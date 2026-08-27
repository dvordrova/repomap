package main

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/dependencydeclaration"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/pipeline"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythondeclareddependencies"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

// runPythonDeclaredDependenciesForRun is the adapter-owned preparation that
// remains outside the language-neutral semantic pipeline. It reads exact
// Python package declarations and persists their target-bound authority.
func runPythonDeclaredDependenciesForRun(
	ctx context.Context,
	runDir string,
	repository *corpus.Corpus,
	targets pythontarget.Catalog,
	selected pythontarget.Target,
	index programindex.Index,
	output *runOutput,
) (dependencydeclaration.Result, error) {
	started := time.Now()
	if output != nil {
		output.Stage("Analysis cubes", "reading exact Python package declarations")
	}
	result, err := pythondeclareddependencies.Build(ctx, repository, targets, selected, index)
	if err != nil {
		return dependencydeclaration.Result{}, fmt.Errorf("Python declared dependencies: %w", err)
	}
	encoded, err := dependencydeclaration.Encode(result)
	if err != nil {
		return dependencydeclaration.Result{}, fmt.Errorf("Python declared dependencies: encode result: %w", err)
	}
	wantArtifactSHA256, err := result.ArtifactSHA256()
	if err != nil {
		return dependencydeclaration.Result{}, fmt.Errorf("Python declared dependencies: digest result: %w", err)
	}
	writer, err := debugdump.OpenWriter(runDir, false)
	if err != nil {
		return dependencydeclaration.Result{}, fmt.Errorf("Python declared dependencies: open artifact writer: %w", err)
	}
	defer writer.Close()
	snapshot := repository.Snapshot()
	if err := writer.WriteValidatedFile(
		dependencydeclaration.ArtifactFilename,
		encoded,
		func(saved []byte) error {
			decoded, decodeErr := pythondeclareddependencies.Decode(
				saved, snapshot, targets, selected, index,
			)
			if decodeErr != nil {
				return decodeErr
			}
			if !reflect.DeepEqual(decoded, result) {
				return fmt.Errorf("Python declared dependencies: persisted authority mismatch")
			}
			savedSHA256, digestErr := decoded.ArtifactSHA256()
			if digestErr != nil {
				return digestErr
			}
			if savedSHA256 != wantArtifactSHA256 {
				return fmt.Errorf("Python declared dependencies: persisted artifact digest mismatch")
			}
			return nil
		},
	); err != nil {
		return dependencydeclaration.Result{}, fmt.Errorf("Python declared dependencies: persist result: %w", err)
	}
	if output != nil {
		output.State(
			"Python declared dependencies", "ready",
			fmt.Sprintf("declaration sources: %d", len(result.Sources)),
			fmt.Sprintf("declared packages: %d", len(result.Packages)),
			fmt.Sprintf("explicit coverage boundaries: %d", result.Coverage.Boundaries),
			"coverage: "+string(result.Coverage.State),
			formatRunOutputWallDuration(time.Since(started)),
			"artifact: "+dependencydeclaration.ArtifactFilename,
		)
	}
	return result, nil
}

func pythonDependencyCoverageError(catalog dependencies.Catalog) error {
	if catalog.Coverage.State == dependencies.CoverageComplete {
		return nil
	}
	detail := "unclassified direct import"
	if len(catalog.Coverage.Omissions) > 0 {
		first := catalog.Coverage.Omissions[0]
		detail = fmt.Sprintf("%s for %s", first.Reason, first.PackagePath)
		if len(catalog.Coverage.Omissions) > 1 {
			detail += fmt.Sprintf(" and %d more", len(catalog.Coverage.Omissions)-1)
		}
	}
	return fmt.Errorf(
		"Python dependency authority is incomplete: %s; replace or remove the unresolved import before analysis",
		detail,
	)
}

// runSemanticPipelineForRun is the command's one bridge into the
// language-neutral semantic chain. Language adapters have already prepared
// every authority supplied here; this bridge only configures the shared
// provider/executor, journal, persistence writer, accounting, and console
// projection.
func runSemanticPipelineForRun(
	ctx context.Context,
	runDir string,
	cacheRoot string,
	noCache bool,
	stopAfter pipeline.Stage,
	language string,
	authorities pipeline.Authorities,
	output *runOutput,
	batchConcurrency int,
	batchController *llm.BatchController,
	providers targetPortfolioProviderFactory,
) (pipeline.Result, error) {
	if providers == nil {
		return pipeline.Result{}, fmt.Errorf("%s semantic pipeline: model provider is unavailable", language)
	}
	provider, err := providers()
	if err != nil {
		return pipeline.Result{}, fmt.Errorf("%s semantic pipeline: configure provider: %w", language, err)
	}
	if provider == nil {
		return pipeline.Result{}, fmt.Errorf("%s semantic pipeline: configured model provider is unavailable", language)
	}
	writer, err := debugdump.OpenWriter(runDir, false)
	if err != nil {
		return pipeline.Result{}, fmt.Errorf("%s semantic pipeline: open artifact writer: %w", language, err)
	}
	defer writer.Close()
	writer.SetWarningWriter(semanticPipelineWarningSink{output: output, language: language})
	executor := llm.Executor{
		RootDir: cacheRoot, Enabled: !noCache,
		Observer:         debugdump.NewSemanticObserver(writer),
		BatchConcurrency: batchConcurrency,
		BatchController:  batchController,
	}
	accounting := []pipeline.AccountingEvent{}
	result, runErr := pipeline.Run(
		ctx,
		pipeline.Runtime{
			Provider: provider, Executor: executor, Artifacts: writer,
			Progress:  semanticPipelineProgress(output, language),
			StopAfter: stopAfter,
			Accounting: func(event pipeline.AccountingEvent) {
				accounting = append(accounting, event)
			},
		},
		authorities,
	)
	if accountingErr := recordSemanticPipelineAccounting(runDir, accounting); accountingErr != nil && output != nil {
		output.Warn(language+" semantic pipeline accounting unavailable", accountingErr.Error())
	}
	if runErr != nil {
		return pipeline.Result{}, runErr
	}
	return result, nil
}

// pythonSemanticPipelineProgress is the presentation adapter for neutral
// pipeline events. It deliberately owns all Python wording outside the shared
// semantic execution package.
func semanticPipelineProgress(output *runOutput, language string) func(pipeline.ProgressEvent) {
	if output == nil {
		return nil
	}
	return func(event pipeline.ProgressEvent) {
		label := strings.TrimSpace(language)
		if label == "" {
			label = "Program"
		}
		if event.State == pipeline.ProgressStarted {
			switch event.Stage {
			case pipeline.StageActivityEntrypoints:
				output.Stage("Analysis cubes", "selecting "+label+" activity starts from the exact ProgramIndex")
			case pipeline.StageIntegrationDependencies:
				output.Stage("Analysis cubes", "classifying "+label+" integration candidates from exact dependency authority")
			case pipeline.StageIntegrationUsage:
				output.Stage("Analysis cubes", "classifying concrete "+label+" integration operations")
			case pipeline.StageActivityPaths:
				output.Stage("Analysis cubes", "connecting exact integration callers to selected activity starts")
			case pipeline.StageCoreMap:
				output.Stage("Analysis cubes", "building "+label+" core responsibilities from the exact ProgramIndex")
			}
			return
		}
		if event.State != pipeline.ProgressReady {
			return
		}

		duration := formatRunOutputWallDuration(event.Elapsed)
		artifact := "artifact: " + event.ArtifactFilename
		switch event.Stage {
		case pipeline.StageActivityEntrypoints:
			value := event.Result.ActivityEntrypoints
			output.State(
				label+" activity entrypoints", "ready",
				fmt.Sprintf("selected activity starts: %d", len(value.Objects)),
				fmt.Sprintf("advertised callables: %d", value.Coverage.CandidatesAdvertised),
				duration,
				artifact,
			)
		case pipeline.StageIntegrationDependencies:
			value := event.Result.IntegrationDependencies
			declared := 0
			if value.Declarations != nil {
				declared = len(value.Declarations.Packages)
			}
			output.State(
				label+" integration dependencies", "ready",
				fmt.Sprintf("selected observed candidates: %d", len(value.Dependencies)),
				fmt.Sprintf("selected declared candidates: %d", declared),
				duration,
				artifact,
			)
		case pipeline.StageIntegrationUsage:
			output.State(
				label+" integration usage", "ready",
				fmt.Sprintf("selected operations: %d", len(event.Result.IntegrationUsage.Uses)),
				duration,
				artifact,
			)
		case pipeline.StageActivityPaths:
			value := event.Result.ActivityPaths
			output.State(
				label+" activity paths", "ready",
				fmt.Sprintf("integration callers: %d", len(value.Routes)),
				fmt.Sprintf("integration outcomes with exact caller path: %d", value.Coverage.ExactOutcomes),
				fmt.Sprintf("integration outcomes with possible caller path: %d", value.Coverage.PossibleOutcomes),
				fmt.Sprintf("frontier or unconnected: %d", value.Coverage.FrontierOutcomes+value.Coverage.UnconnectedOutcomes),
				duration,
				artifact,
			)
		case pipeline.StageCoreMap:
			output.State(label+" core map", "ready", duration, artifact)
		}
	}
}

// pythonSemanticPipelineWarningSink retains the existing stage-specific
// console summaries while one shared writer and observer own the whole chain.
type semanticPipelineWarningSink struct {
	output   *runOutput
	language string
}

func (writer semanticPipelineWarningSink) Write(data []byte) (int, error) {
	if writer.output == nil {
		return len(data), nil
	}
	detail := strings.TrimSpace(string(data))
	detail = strings.TrimPrefix(detail, "warning: ")
	summary := writer.language + " semantic pipeline exchange journal unavailable"
	switch {
	case strings.Contains(detail, "stage="+debugdump.SemanticStageActivityEntrypoints):
		summary = writer.language + " activity entrypoint semantic exchange journal unavailable"
	case strings.Contains(detail, "stage="+debugdump.SemanticStageIntegrationDependencies):
		summary = writer.language + " integration dependency semantic exchange journal unavailable"
	case strings.Contains(detail, "stage="+debugdump.SemanticStageIntegrationUsage):
		summary = writer.language + " integration usage semantic exchange journal unavailable"
	case strings.Contains(detail, "stage="+debugdump.SemanticStageCoreMapBaseline),
		strings.Contains(detail, "stage="+debugdump.SemanticStageCoreMapRefined):
		summary = writer.language + " core semantic exchange journal unavailable"
	}
	writer.output.Warn(summary, detail)
	return len(data), nil
}
