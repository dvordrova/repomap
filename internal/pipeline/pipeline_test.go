package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/activityentrypoint"
	"github.com/dvordrova/repomap/internal/activitypath"
	"github.com/dvordrova/repomap/internal/coremap"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/dependencydeclaration"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/integrationdependency"
	"github.com/dvordrova/repomap/internal/integrationusage"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
)

func TestRunOwnsCompleteValidatedSemanticChain(t *testing.T) {
	repository, index := pipelineTestAuthorities(t)
	declarations, err := dependencydeclaration.Build(dependencydeclaration.Input{
		CorpusSHA256: repository.Snapshot().SHA256, ProgramIndexSHA256: index.SHA256,
		TargetID: index.Target.ID,
		Scope: dependencydeclaration.Scope{
			Language: "python", Ecosystem: "python", RepositoryPath: "",
			AuthoritySHA256: strings.Repeat("e", 64),
		},
		Sources: []dependencydeclaration.SourceInput{}, Statements: []dependencydeclaration.StatementInput{},
		Includes: []dependencydeclaration.IncludeInput{}, Frontiers: []dependencydeclaration.FrontierInput{},
	})
	if err != nil {
		t.Fatal(err)
	}

	provider := &pipelineTestProvider{responses: [][]byte{
		[]byte(`{"activity_refs":[]}`),
		[]byte(`{"blocks":[{"name":"Execution","purpose":"Runs the selected target.","file_refs":[],"symbol_refs":["s1"]}]}`),
	}}
	observer := &pipelineTestObserver{}
	artifacts := &pipelineTestArtifacts{files: make(map[string][]byte)}
	var progress []ProgressEvent
	var accounting []AccountingEvent
	result, err := Run(
		t.Context(),
		Runtime{
			Provider: provider, Executor: llm.Executor{Enabled: false, Observer: observer},
			Artifacts:  artifacts,
			Progress:   func(event ProgressEvent) { progress = append(progress, event) },
			Accounting: func(event AccountingEvent) { accounting = append(accounting, event) },
		},
		Authorities{
			RepositoryName: "sample", Repository: repository, ProgramIndex: index,
			Dependencies: dependencies.Empty(), Declarations: &declarations,
			ReadmeRoles: readmetargetscout.Result{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.ActivityEntrypoints.ValidateAgainst(index); err != nil {
		t.Fatal(err)
	}
	if err := result.IntegrationDependencies.ValidateAgainstDeclarations(
		dependencies.Empty(), declarations, index.Target,
	); err != nil {
		t.Fatal(err)
	}
	if err := result.IntegrationUsage.ValidateAgainst(index, result.IntegrationDependencies); err != nil {
		t.Fatal(err)
	}
	if err := result.ActivityPaths.ValidateAgainst(
		index, result.ActivityEntrypoints, result.IntegrationDependencies, result.IntegrationUsage,
	); err != nil {
		t.Fatal(err)
	}
	if result.CoreMap.ProgramIndexSHA256 != index.SHA256 || len(result.CoreMap.Refined) != 1 {
		t.Fatalf("core map result = %#v", result.CoreMap)
	}

	wantArtifacts := []string{
		activityentrypoint.ArtifactFilename,
		integrationdependency.ArtifactFilename,
		integrationusage.ArtifactFilename,
		activitypath.ArtifactFilename,
		coremap.ArtifactFilename,
	}
	gotArtifacts := make([]string, 0, len(artifacts.files))
	for filename := range artifacts.files {
		gotArtifacts = append(gotArtifacts, filename)
	}
	slices.Sort(gotArtifacts)
	slices.Sort(wantArtifacts)
	if !slices.Equal(gotArtifacts, wantArtifacts) {
		t.Fatalf("artifacts = %v, want %v", gotArtifacts, wantArtifacts)
	}

	wantStages := []Stage{
		StageActivityEntrypoints,
		StageIntegrationDependencies,
		StageIntegrationUsage,
		StageActivityPaths,
		StageCoreMap,
	}
	if len(progress) != 2*len(wantStages) {
		t.Fatalf("progress events = %d, want %d", len(progress), 2*len(wantStages))
	}
	for position, stage := range wantStages {
		started := progress[position*2]
		ready := progress[position*2+1]
		if started.Stage != stage || started.State != ProgressStarted ||
			ready.Stage != stage || ready.State != ProgressReady ||
			ready.ArtifactFilename == "" || ready.Elapsed < 0 {
			t.Fatalf("stage %q progress = %#v / %#v", stage, started, ready)
		}
	}
	if provider.completeCalls != 2 {
		t.Fatalf("provider completions = %d, want 2", provider.completeCalls)
	}
	if got, want := observer.snapshot(), []string{
		debugdump.SemanticStageActivityEntrypoints,
		debugdump.SemanticStageCoreMapRefined,
	}; !slices.Equal(got, want) {
		t.Fatalf("observed semantic stages = %v, want %v", got, want)
	}
	if !slices.Equal(accounting, result.Accounting) || len(accounting) != 2 ||
		accounting[0].Stage != debugdump.SemanticStageActivityEntrypoints ||
		accounting[1].Stage != debugdump.SemanticStageCoreMapRefined {
		t.Fatalf("pipeline accounting = %#v, result = %#v", accounting, result.Accounting)
	}
	for _, event := range accounting {
		if event.Ordinal != 1 || event.State != AccountingAccepted || event.RequestBytes <= 0 ||
			event.SemanticCalls != 1 || event.TransportAttempts != 1 || event.Metrics.Latency <= 0 {
			t.Fatalf("invalid accounting event = %#v", event)
		}
	}
}

func TestRunStopsOnlyAfterPersistedActivityEntrypoints(t *testing.T) {
	repository, index := pipelineTestAuthorities(t)
	provider := &pipelineTestProvider{responses: [][]byte{
		[]byte(`{"activity_refs":[]}`),
	}}
	artifacts := &pipelineTestArtifacts{files: make(map[string][]byte)}
	var progress []ProgressEvent
	result, err := Run(
		t.Context(),
		Runtime{
			Provider: provider, Executor: llm.Executor{Enabled: false},
			Artifacts: artifacts, StopAfter: StageActivityEntrypoints,
			Progress: func(event ProgressEvent) { progress = append(progress, event) },
		},
		Authorities{
			RepositoryName: "sample", Repository: repository, ProgramIndex: index,
			Dependencies: dependencies.Empty(), ReadmeRoles: readmetargetscout.Result{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.StoppedAfter != StageActivityEntrypoints {
		t.Fatalf("stopped after = %q", result.StoppedAfter)
	}
	if err := result.ActivityEntrypoints.ValidateAgainst(index); err != nil {
		t.Fatal(err)
	}
	if len(artifacts.files) != 1 || artifacts.files[activityentrypoint.ArtifactFilename] == nil {
		t.Fatalf("checkpoint artifacts = %v", artifacts.files)
	}
	if provider.completeCalls != 1 {
		t.Fatalf("provider completions = %d, want only ActivityEntrypoints", provider.completeCalls)
	}
	if len(progress) != 2 || progress[0].State != ProgressStarted ||
		progress[1].State != ProgressReady || progress[1].ArtifactFilename != activityentrypoint.ArtifactFilename {
		t.Fatalf("checkpoint progress = %#v", progress)
	}
}

func TestRunIntegrationDependenciesDoesNotSynthesizeDeclarationArtifact(t *testing.T) {
	_, index := pipelineTestAuthoritiesForLanguage(t, "go", "pkg/main.go")
	provider := &pipelineTestProvider{}
	artifacts := &pipelineTestArtifacts{files: make(map[string][]byte)}
	result, err := runIntegrationDependencies(
		t.Context(),
		Runtime{Provider: provider, Executor: llm.Executor{Enabled: false}, Artifacts: artifacts},
		dependencies.Empty(), nil, index.Target,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.ValidateAgainst(dependencies.Empty()); err != nil {
		t.Fatal(err)
	}
	if provider.completeCalls != 0 {
		t.Fatalf("provider calls = %d, want 0", provider.completeCalls)
	}
	if _, exists := artifacts.files[dependencydeclaration.ArtifactFilename]; exists {
		t.Fatal("pipeline synthesized a declaration artifact for Go")
	}
}

func pipelineTestAuthorities(t *testing.T) (*corpus.Corpus, programindex.Index) {
	return pipelineTestAuthoritiesForLanguage(t, "python", "pkg/main.py")
}

func pipelineTestAuthoritiesForLanguage(
	t *testing.T,
	language string,
	fixturePath string,
) (*corpus.Corpus, programindex.Index) {
	t.Helper()
	root := t.TempDir()
	path := fixturePath
	absolute := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte("fixture source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := corpus.New(t.Context(), root, gitfiles.Listing{
		Paths: []string{path}, RegularPaths: []string{path},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	fileRef, ok := repository.ID(path)
	if !ok {
		t.Fatal("fixture file is absent from corpus")
	}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("c", 64), SourceSHA256: strings.Repeat("d", 64),
		Target: programindex.TargetInput{
			Language: language, Kind: "executable", Name: "pkg.main", Selector: language + ":pkg.main",
			Sources:       []programindex.TargetSource{{FileRef: string(fileRef), Path: path}},
			AnchorFileRef: string(fileRef),
			Seeds: []programindex.TargetSeedInput{{
				ObjectRef: "run", Kind: programindex.SeedCallable,
				Location: &programindex.Location{Path: path, Line: 1, Column: 1},
			}},
		},
		Objects: []programindex.ObjectInput{
			{
				SourceRef: "module", Kind: programindex.ObjectModule, Name: "pkg.main",
				Visibility: programindex.VisibilityPublic,
				Location:   &programindex.Location{Path: path, Line: 1, Column: 1},
			},
			{
				SourceRef: "run", Kind: programindex.ObjectFunction, Name: "run",
				Visibility: programindex.VisibilityPublic, Signature: "run()",
				OwnerRef: "module", ContainerRef: "module",
				Location: &programindex.Location{Path: path, Line: 1, Column: 1},
			},
		},
		Relations: []programindex.RelationInput{},
		Coverage: programindex.CoverageInput{
			Measured: true, ObjectsObserved: 2, RelationsObserved: 0,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return repository, index
}

type pipelineTestArtifacts struct {
	files map[string][]byte
}

func (writer *pipelineTestArtifacts) WriteValidatedFile(
	filename string,
	data []byte,
	validate func([]byte) error,
) error {
	saved := append([]byte(nil), data...)
	if err := validate(saved); err != nil {
		return err
	}
	if _, duplicate := writer.files[filename]; duplicate {
		return fmt.Errorf("duplicate artifact %q", filename)
	}
	writer.files[filename] = saved
	return nil
}

type pipelineTestProvider struct {
	mu            sync.Mutex
	responses     [][]byte
	completeCalls int
}

func (provider *pipelineTestProvider) State() []byte {
	return []byte(`{"endpoint":"https://provider.test/v1/chat","model":"pipeline-fixture"}`)
}

func (provider *pipelineTestProvider) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	return llm.NewPrepared([]byte(prompt.System + "\n" + prompt.User))
}

func (provider *pipelineTestProvider) Complete(
	_ context.Context,
	_ llm.Prepared,
) (llm.Completion, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.completeCalls >= len(provider.responses) {
		return llm.Completion{}, fmt.Errorf("unexpected provider completion")
	}
	response := append([]byte(nil), provider.responses[provider.completeCalls]...)
	provider.completeCalls++
	return llm.Completion{
		Response: response, FinishReason: llm.FinishStop, ChoiceCount: 1,
		Metrics: llm.Metrics{
			InputTokens: 1, OutputTokens: 1, ProviderResponseBytes: len(response),
			UsageReported: true, Latency: time.Millisecond, Attempts: 1,
		},
	}, nil
}

type pipelineTestObserver struct {
	mu     sync.Mutex
	stages []string
}

func (*pipelineTestObserver) Observe(llm.Event) error { return nil }

func (observer *pipelineTestObserver) ObserveStage(stage string, _ llm.Event) error {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	observer.stages = append(observer.stages, stage)
	return nil
}

func (observer *pipelineTestObserver) snapshot() []string {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	return append([]string(nil), observer.stages...)
}
