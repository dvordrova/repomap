package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/runtimeportfolio"
	"github.com/dvordrova/repomap/internal/snapshot"
)

type runtimePortfolioRunner func(
	context.Context,
	llm.Executor,
	llm.Provider,
	runtimeportfolio.Input,
) (runtimeportfolio.RunOutcome, error)

// runtimePortfolioArtifactSetValidator performs the expensive semantic check
// once for a set of artifacts that must be byte-identical. Callers still read
// and compare every run's bytes; a later run can never substitute a different
// (even independently valid) repository artifact.
type runtimePortfolioArtifactSetValidator struct {
	canonical []byte
}

func (validator *runtimePortfolioArtifactSetValidator) validate(
	raw []byte,
	validateFirst func([]byte) error,
) error {
	if validator.canonical != nil {
		if !bytes.Equal(raw, validator.canonical) {
			return fmt.Errorf("runtime portfolio: target pages carry different repository artifacts")
		}
		return nil
	}
	if err := validateFirst(raw); err != nil {
		return err
	}
	validator.canonical = append([]byte(nil), raw...)
	return nil
}

func fullyValidateRuntimePortfolioArtifact(raw []byte, input runtimeportfolio.Input) error {
	decoded, err := runtimeportfolio.Decode(raw)
	if err != nil {
		return err
	}
	if err := decoded.ValidateAgainst(input); err != nil {
		return fmt.Errorf("runtime portfolio: stale repository authority: %w", err)
	}
	want, err := runtimeportfolio.Encode(decoded)
	if err != nil || !bytes.Equal(raw, want) {
		return fmt.Errorf("runtime portfolio: artifact is not canonical")
	}
	return nil
}

// synthesizeRuntimePortfolio runs one exhaustive repository-level semantic cube
// after every selected target page has completed and the exact page portfolio
// has been sealed. The default page owns its provider/cache observations;
// every page receives the same validated semantic artifact bytes.
func synthesizeRuntimePortfolio(
	ctx context.Context,
	cacheRoot string,
	noCache bool,
	batchConcurrency int,
	batchController *llm.BatchController,
	providerFactory targetPortfolioProviderFactory,
	runner runtimePortfolioRunner,
	container snapshot.TargetRunContainer,
	portfolio snapshot.TargetPagePortfolio,
	runs []targetPublishedRun,
	output *runOutput,
) error {
	input, owner, err := runtimePortfolioInputForRuns(container, portfolio, runs)
	if err != nil {
		return err
	}
	if providerFactory == nil {
		return fmt.Errorf("runtime portfolio: model provider is unavailable")
	}
	provider, err := providerFactory()
	if err != nil {
		return fmt.Errorf("runtime portfolio: configure provider: %w", err)
	}
	if provider == nil {
		return fmt.Errorf("runtime portfolio: configured model provider is unavailable")
	}
	if runner == nil {
		runner = runtimeportfolio.Run
	}

	writer, err := debugdump.OpenWriter(owner.RunDir, false)
	if err != nil {
		return fmt.Errorf("runtime portfolio: open semantic artifact writer: %w", err)
	}
	executor := llm.Executor{
		RootDir:          cacheRoot,
		Enabled:          !noCache,
		Observer:         debugdump.NewSemanticObserver(writer),
		BatchConcurrency: batchConcurrency,
		BatchController:  batchController,
	}
	if output != nil {
		output.Stage("Repository overview", "synthesizing runtime roles across completed target pages")
	}
	started := time.Now()
	outcome, runErr := runner(
		ctx,
		debugdump.BindStage(executor, debugdump.SemanticStageRuntimePortfolio),
		provider,
		input,
	)
	closeErr := writer.Close()
	if runErr != nil {
		return errors.Join(fmt.Errorf("runtime portfolio: synthesize repository roles: %w", runErr), closeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("runtime portfolio: close semantic artifact writer: %w", closeErr)
	}
	if err := outcome.Value.ValidateAgainst(input); err != nil {
		return fmt.Errorf("runtime portfolio: accepted result does not match exact repository input: %w", err)
	}
	if err := persistTargetPagePortfolioArtifacts(container, portfolio, runs); err != nil {
		return err
	}
	if err := persistRuntimePortfolioForRuns(input, outcome.Value, runs); err != nil {
		return err
	}
	state := debugdump.SemanticStateAccepted
	semanticCalls := outcome.SemanticCalls
	transportAttempts := outcome.Metrics.Attempts
	latencyMillis := outcome.Metrics.Latency.Milliseconds()
	if outcome.Cached {
		state = debugdump.SemanticStateCacheHit
		transportAttempts = 0
		latencyMillis = 0
	}
	if err := recordSemanticStageDiagnostic(owner.RunDir, semanticStageDiagnostic{
		Stage: debugdump.SemanticStageRuntimePortfolio, State: state,
		RequestBytes: outcome.RequestBytes, SemanticCalls: semanticCalls,
		TransportAttempts: transportAttempts, LatencyMillis: latencyMillis,
	}); err != nil {
		return fmt.Errorf("runtime portfolio: record semantic diagnostics: %w", err)
	}
	if output != nil {
		source := "live provider"
		if outcome.Cached {
			source = "validated cache"
		}
		output.State(
			"Repository overview", "ready",
			"source: "+source,
			fmt.Sprintf("runtime roles: %d", len(outcome.Value.Roles)),
			fmt.Sprintf("mapped targets: %d/%d", outcome.Value.Coverage.TargetsMapped, outcome.Value.Coverage.TargetsObserved),
			formatRunOutputWallDuration(time.Since(started)),
			"artifact: "+runtimeportfolio.ArtifactFilename,
		)
	}
	return nil
}

func persistRuntimePortfolioForRuns(
	input runtimeportfolio.Input,
	result runtimeportfolio.Result,
	runs []targetPublishedRun,
) error {
	encoded, err := runtimeportfolio.Encode(result)
	if err != nil {
		return fmt.Errorf("runtime portfolio: encode artifact: %w", err)
	}
	if len(encoded) == 0 || len(encoded) > runtimeportfolio.MaxArtifactBytes {
		return fmt.Errorf("runtime portfolio: encoded artifact exceeds its byte bound")
	}
	validator := runtimePortfolioArtifactSetValidator{}
	for _, run := range runs {
		writer, writerErr := debugdump.OpenWriter(run.RunDir, true)
		if writerErr != nil {
			return fmt.Errorf("runtime portfolio: open run %s: %w", run.RunID, writerErr)
		}
		writeErr := writer.WriteValidatedFile(
			runtimeportfolio.ArtifactFilename,
			encoded,
			func(saved []byte) error {
				if !bytes.Equal(saved, encoded) {
					return fmt.Errorf("runtime portfolio: persisted bytes changed")
				}
				return validator.validate(saved, func(raw []byte) error {
					return fullyValidateRuntimePortfolioArtifact(raw, input)
				})
			},
		)
		closeErr := writer.Close()
		if writeErr != nil {
			return fmt.Errorf("runtime portfolio: persist run %s: %w", run.RunID, writeErr)
		}
		if closeErr != nil {
			return fmt.Errorf("runtime portfolio: close run %s: %w", run.RunID, closeErr)
		}
	}
	return nil
}

// validateRuntimePortfolioArtifactsForFinalization makes the repository cube
// a hard prerequisite of target-page finalization and recovery. Every page
// must carry the exact same canonical bytes, and those bytes must still bind
// the freshly rebuilt completed-page input and exact TargetPagePortfolio.
func validateRuntimePortfolioArtifactsForFinalization(
	container snapshot.TargetRunContainer,
	portfolio snapshot.TargetPagePortfolio,
	runs []targetPublishedRun,
) error {
	input, _, err := runtimePortfolioInputForRuns(container, portfolio, runs)
	if err != nil {
		return err
	}
	validator := runtimePortfolioArtifactSetValidator{}
	for _, run := range runs {
		raw, found, readErr := readTargetPageRunFile(
			run.RunDir,
			runtimeportfolio.ArtifactFilename,
			runtimeportfolio.MaxArtifactBytes,
		)
		if readErr != nil {
			return readErr
		}
		if !found {
			return fmt.Errorf("runtime portfolio: run %s is missing the repository artifact", run.RunID)
		}
		if err := validator.validate(raw, func(candidate []byte) error {
			return fullyValidateRuntimePortfolioArtifact(candidate, input)
		}); err != nil {
			return fmt.Errorf("runtime portfolio: run %s: %w", run.RunID, err)
		}
	}
	return nil
}

func runtimePortfolioInputForRuns(
	container snapshot.TargetRunContainer,
	portfolio snapshot.TargetPagePortfolio,
	runs []targetPublishedRun,
) (runtimeportfolio.Input, targetPublishedRun, error) {
	if err := portfolio.ValidateAgainstContainer(container); err != nil {
		return runtimeportfolio.Input{}, targetPublishedRun{}, err
	}
	pages, programTargetByOuterRef, _, err := targetNavigationPagesForRuns(container, portfolio, runs)
	if err != nil {
		return runtimeportfolio.Input{}, targetPublishedRun{}, err
	}
	pageByTargetID := make(map[string]report.TargetNavigationPage, len(pages))
	for _, page := range pages {
		pageByTargetID[page.ProgramTarget.ID] = page
	}
	runByOuterRef := make(map[string]targetPublishedRun, len(runs))
	for _, run := range runs {
		if _, duplicate := runByOuterRef[run.Target.Ref]; duplicate {
			return runtimeportfolio.Input{}, targetPublishedRun{}, fmt.Errorf("runtime portfolio: duplicate published target ref")
		}
		runByOuterRef[run.Target.Ref] = run
	}
	result := runtimeportfolio.Input{
		TargetPagePortfolioSHA256: portfolio.SHA256,
		Targets:                   []runtimeportfolio.TargetInput{},
		RepositoryEvidence:        []runtimeportfolio.EvidenceInput{},
	}
	var owner targetPublishedRun
	for _, projection := range container.Targets {
		run, found := runByOuterRef[projection.Target.Ref]
		if !found {
			return runtimeportfolio.Input{}, targetPublishedRun{}, fmt.Errorf("runtime portfolio: completed run coverage is incomplete")
		}
		programTargetID := programTargetByOuterRef[projection.Target.Ref]
		page, found := pageByTargetID[programTargetID]
		if !found {
			return runtimeportfolio.Input{}, targetPublishedRun{}, fmt.Errorf("runtime portfolio: exact program target page is missing")
		}
		data, readErr := report.ReadRunDir(run.RunDir)
		if readErr != nil {
			return runtimeportfolio.Input{}, targetPublishedRun{}, fmt.Errorf("runtime portfolio: read completed page %s: %w", run.RunID, readErr)
		}
		if result.RepositoryName == "" {
			result.RepositoryName = data.RepoName
		} else if result.RepositoryName != data.RepoName {
			return runtimeportfolio.Input{}, targetPublishedRun{}, fmt.Errorf("runtime portfolio: target pages disagree on repository identity")
		}
		if result.CapturedRevision == "" {
			result.CapturedRevision = run.SelectedRevision
		} else if result.CapturedRevision != run.SelectedRevision {
			return runtimeportfolio.Input{}, targetPublishedRun{}, fmt.Errorf("runtime portfolio: target pages disagree on captured revision")
		}
		input, inputErr := runtimePortfolioTargetInput(data, page, projection.Target.Ref == container.DefaultTargetRef)
		if inputErr != nil {
			return runtimeportfolio.Input{}, targetPublishedRun{}, fmt.Errorf("runtime portfolio: completed page %s: %w", run.RunID, inputErr)
		}
		result.Targets = append(result.Targets, input)
		if projection.Target.Ref == container.DefaultTargetRef {
			owner = run
		}
	}
	if owner.RunDir == "" {
		return runtimeportfolio.Input{}, targetPublishedRun{}, fmt.Errorf("runtime portfolio: default target page is missing")
	}
	readmeRows, err := readRuntimePortfolioReadmeRoles(owner.RunDir)
	if err != nil {
		return runtimeportfolio.Input{}, targetPublishedRun{}, err
	}
	result.RepositoryEvidence = runtimePortfolioRepositoryEvidence(readmeRows)
	if _, err := runtimeportfolio.Compile(result); err != nil {
		return runtimeportfolio.Input{}, targetPublishedRun{}, fmt.Errorf("runtime portfolio: compile completed-page input: %w", err)
	}
	return result, owner, nil
}

func runtimePortfolioTargetInput(
	data *report.ReportData,
	page report.TargetNavigationPage,
	isDefault bool,
) (runtimeportfolio.TargetInput, error) {
	if data == nil || data.ProgramPortfolio == nil || data.CoreMapView == nil ||
		data.ActivityEntrypointView == nil || data.IntegrationUsageView == nil {
		return runtimeportfolio.TargetInput{}, fmt.Errorf("required ProgramIndex semantic views are incomplete")
	}
	var entry *report.ProgramPortfolioEntry
	for index := range data.ProgramPortfolio.Entries {
		candidate := &data.ProgramPortfolio.Entries[index]
		if candidate.Target.ID == page.ProgramTarget.ID {
			entry = candidate
			break
		}
	}
	if entry == nil || !reflect.DeepEqual(entry.Target, page.ProgramTarget) ||
		data.ProgramPortfolio.DefaultTargetID != page.ProgramTarget.ID ||
		data.CoreMapView.ProgramTargetID != page.ProgramTarget.ID ||
		data.ActivityEntrypointView.ProgramTargetID != page.ProgramTarget.ID ||
		data.IntegrationUsageView.ProgramTargetID != page.ProgramTarget.ID {
		return runtimeportfolio.TargetInput{}, fmt.Errorf("completed semantic views do not bind the page ProgramTarget")
	}
	target := runtimeportfolio.TargetInput{
		ProgramTargetID:  page.ProgramTarget.ID,
		DisplayName:      page.ProgramTarget.Name,
		Language:         page.ProgramTarget.Language,
		Kind:             page.ProgramTarget.Kind,
		Selector:         page.ProgramTarget.Selector,
		Default:          isDefault,
		ProgramObjects:   entry.View.IndexCoverage.ObjectsObserved,
		ProgramRelations: entry.View.IndexCoverage.RelationsObserved,
		ActivityStarts:   len(data.ActivityEntrypointView.Entrypoints),
		IntegrationUses:  data.IntegrationUsageView.Coverage.Selected,
		Responsibilities: []runtimeportfolio.ResponsibilityInput{},
		Evidence:         []runtimeportfolio.EvidenceInput{},
	}
	for _, source := range page.ProgramTarget.Sources {
		target.Evidence = append(target.Evidence, runtimeportfolio.EvidenceInput{
			Kind:            runtimeportfolio.EvidenceTargetEntrypoint,
			Label:           runtimePortfolioEvidenceLabel("Program target source", page.ProgramTarget.Name),
			Location:        runtimeportfolio.Location{Path: source.Path},
			ProgramTargetID: page.ProgramTarget.ID,
		})
	}
	for _, seed := range entry.View.Seeds {
		location := seed.LaunchLocation
		if location == nil {
			location = seed.DeclarationLocation
		}
		if location == nil {
			continue
		}
		target.Evidence = append(target.Evidence, runtimeportfolio.EvidenceInput{
			Kind: runtimeportfolio.EvidenceTargetEntrypoint,
			Label: runtimePortfolioEvidenceLabel(
				"Program target "+string(seed.Kind), seed.Name, seed.Signature,
			),
			Location:        runtimeportfolio.Location{Path: location.Path, Line: location.Line, Column: location.Column},
			ProgramTargetID: page.ProgramTarget.ID,
		})
	}
	for _, activity := range data.ActivityEntrypointView.Entrypoints {
		target.Evidence = append(target.Evidence, runtimeportfolio.EvidenceInput{
			Kind: runtimeportfolio.EvidenceActivityStart,
			Label: runtimePortfolioEvidenceLabel(
				"Selected activity start", activity.ContainerName, activity.OwnerName, activity.Name, activity.Signature,
			),
			Location: runtimeportfolio.Location{
				Path: activity.Location.Path, Line: activity.Location.Line, Column: activity.Location.Column,
			},
			ProgramTargetID: page.ProgramTarget.ID,
		})
	}
	for _, block := range data.CoreMapView.RefinedCore {
		appendRuntimePortfolioResponsibility(&target, block)
	}
	return target, nil
}

func appendRuntimePortfolioResponsibility(
	target *runtimeportfolio.TargetInput,
	block report.CoreMapViewBlock,
) {
	if target == nil {
		return
	}
	responsibility := runtimeportfolio.ResponsibilityInput{
		Name:     runtimePortfolioEvidenceLabel(block.Name),
		Purpose:  runtimePortfolioEvidenceLabel(block.Purpose),
		Evidence: []runtimeportfolio.EvidenceInput{},
	}
	for _, file := range block.Files {
		responsibility.Evidence = append(responsibility.Evidence, runtimeportfolio.EvidenceInput{
			Kind:            runtimeportfolio.EvidenceResponsibility,
			Label:           runtimePortfolioEvidenceLabel("Responsibility file", block.Name),
			Location:        runtimeportfolio.Location{Path: file.Path},
			ProgramTargetID: target.ProgramTargetID,
		})
	}
	for _, representative := range block.RepresentativeSymbols {
		responsibility.Evidence = append(responsibility.Evidence, runtimeportfolio.EvidenceInput{
			Kind: runtimeportfolio.EvidenceProgramFact,
			Label: runtimePortfolioEvidenceLabel(
				"Responsibility representative", block.Name, representative.Symbol.Package, representative.Symbol.Name,
			),
			Location: runtimeportfolio.Location{
				Path:   representative.Symbol.Location.Path,
				Line:   representative.Symbol.Location.Line,
				Column: representative.Symbol.Location.Column,
			},
			ProgramTargetID: target.ProgramTargetID,
		})
	}
	target.Responsibilities = append(target.Responsibilities, responsibility)
	for _, child := range block.Children {
		appendRuntimePortfolioResponsibility(target, child)
	}
}

func readRuntimePortfolioReadmeRoles(runDir string) ([]readmeRoleLogRow, error) {
	raw, found, err := readTargetPageRunFile(
		runDir,
		readmetargetscout.ArtifactFilename,
		readmetargetscout.MaxRequestBytes,
	)
	if err != nil || !found {
		return []readmeRoleLogRow{}, err
	}
	var artifact struct {
		Version int                `json:"version"`
		Files   []readmeRoleLogRow `json:"files"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&artifact); err != nil {
		return nil, fmt.Errorf("runtime portfolio: decode README role authority: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("runtime portfolio: README role authority has trailing data")
	}
	if artifact.Version != 1 || artifact.Files == nil {
		return nil, fmt.Errorf("runtime portfolio: README role authority has invalid identity")
	}
	for _, row := range artifact.Files {
		if row.Path == "" || row.Classifications == nil {
			return nil, fmt.Errorf("runtime portfolio: README role authority is incomplete")
		}
		for _, classification := range row.Classifications {
			if !validReadmeRoleClass(classification.Class) || classification.Hypotheses == nil {
				return nil, fmt.Errorf("runtime portfolio: README role authority has an invalid classification")
			}
		}
	}
	return artifact.Files, nil
}

func runtimePortfolioRepositoryEvidence(rows []readmeRoleLogRow) []runtimeportfolio.EvidenceInput {
	result := []runtimeportfolio.EvidenceInput{}
	for _, row := range rows {
		for _, classification := range row.Classifications {
			kind := runtimeportfolio.EvidenceRepositoryGuidance
			switch classification.Class {
			case readmetargetscout.ClassConfiguration:
				kind = runtimeportfolio.EvidenceConfiguration
			case readmetargetscout.ClassDeployment:
				kind = runtimeportfolio.EvidenceDeployment
			}
			for _, hypothesis := range classification.Hypotheses {
				result = append(result, runtimeportfolio.EvidenceInput{
					Kind: kind,
					Label: runtimePortfolioEvidenceLabel(
						"Repository guidance "+string(classification.Class), hypothesis,
					),
					Location: runtimeportfolio.Location{Path: row.Path},
				})
			}
		}
	}
	return result
}

func runtimePortfolioEvidenceLabel(parts ...string) string {
	meaningful := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			meaningful = append(meaningful, trimmed)
		}
	}
	joined := strings.Join(meaningful, ": ")
	joined = strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return ' '
		}
		return character
	}, joined)
	joined = strings.Join(strings.Fields(joined), " ")
	if joined == "" {
		joined = "Exact program evidence"
	}
	if len(joined) <= runtimeportfolio.MaxTextBytes {
		return joined
	}
	limit := runtimeportfolio.MaxTextBytes - len("...")
	for limit > 0 && !utf8.RuneStart(joined[limit]) {
		limit--
	}
	return strings.TrimSpace(joined[:limit]) + "..."
}
