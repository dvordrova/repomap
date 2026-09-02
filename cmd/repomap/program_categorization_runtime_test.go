package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/documentationreduce"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programcategorization"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
)

func TestReduceRepositoryDocumentationRunsOneOwnedRepositoryHandoff(t *testing.T) {
	controller := &llm.BatchController{}
	calls := 0
	runner := func(
		ctx context.Context,
		executor llm.Executor,
		provider llm.Provider,
		guidance readmetargetscout.GuidanceSnapshot,
	) (documentationreduce.Result, error) {
		calls++
		if provider != nil || executor.BatchController != controller || executor.BatchConcurrency != 3 {
			t.Fatalf("documentation runtime wiring = provider %#v, executor %#v", provider, executor)
		}
		return documentationreduce.Run(ctx, executor, nil, guidance)
	}
	result, err := reduceRepositoryDocumentationForRun(
		t.Context(), t.TempDir(), true, 3, controller, nil, runner, nil,
		readmetargetscout.GuidanceSnapshot{}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || result.ReductionSHA256 == "" || result.Sources == nil {
		t.Fatalf("documentation handoff = %#v, calls = %d", result, calls)
	}
	snapshot, err := result.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Sources = append(snapshot.Sources, documentationreduce.Source{})
	if len(result.Sources) != 0 {
		t.Fatal("documentation runtime result aliases its returned snapshot")
	}
}

func TestEnrichProgramIndexReturnsOwnedPageAuthorityAndPersistsDocumentation(t *testing.T) {
	documentation := emptyRuntimeDocumentation(t)
	base := runtimeCategorizationIndex(t, "api", "go:./cmd/api", "cmd/api/main.go", "f-api")
	controller := &llm.BatchController{}
	calls := 0
	runner := func(
		_ context.Context,
		executor llm.Executor,
		_ llm.Provider,
		index programindex.Index,
		reduced documentationreduce.Result,
	) (programcategorization.Result, error) {
		calls++
		if executor.BatchController != controller || executor.BatchConcurrency != 2 ||
			reduced.ReductionSHA256 != documentation.ReductionSHA256 {
			t.Fatalf("categorization runtime wiring = %#v / %#v", executor, reduced)
		}
		return programcategorization.Result{
			ProgramTargetID: index.Target.ID, BaseProgramIndexSHA256: index.SHA256,
			ReducedDocumentationSHA256: reduced.ReductionSHA256,
			Assignments:                []programcategorization.Assignment{},
			Diagnostics:                []programcategorization.Diagnostic{},
		}, nil
	}
	runDir := t.TempDir()
	enriched, err := enrichProgramIndexForRun(
		t.Context(), runDir, t.TempDir(), true, 2, controller, nil, runner,
		documentation, base, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("categorization calls = %d", calls)
	}
	if enriched.Categorization == nil ||
		enriched.Categorization.BaseIndexSHA256 != base.SHA256 ||
		enriched.Categorization.ReducedDocumentationSHA256 != documentation.ReductionSHA256 ||
		enriched.SHA256 == base.SHA256 {
		t.Fatalf("enriched index = %#v", enriched.Categorization)
	}
	if base.Categorization != nil {
		t.Fatal("base index was mutated")
	}
	if _, err := os.Stat(filepath.Join(runDir, programindex.ArtifactFilename)); !os.IsNotExist(err) {
		t.Fatalf("categorization helper persisted ProgramIndex: %v", err)
	}
	assertPersistedRuntimeDocumentation(t, runDir, documentation)
}

func TestEnrichProgramIndexRejectsFailedPageWithoutProgramPersistence(t *testing.T) {
	documentation := emptyRuntimeDocumentation(t)
	base := runtimeCategorizationIndex(t, "api", "go:./cmd/api", "cmd/api/main.go", "f-api")
	calls := 0
	runner := func(
		_ context.Context,
		_ llm.Executor,
		_ llm.Provider,
		index programindex.Index,
		reduced documentationreduce.Result,
	) (programcategorization.Result, error) {
		calls++
		return programcategorization.Result{}, errors.New("rejected target")
	}
	runDir := t.TempDir()
	enriched, err := enrichProgramIndexForRun(
		t.Context(), runDir, t.TempDir(), true, 1, &llm.BatchController{}, nil,
		runner, documentation, base, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "rejected target") || enriched.Target.ID != "" || calls != 1 {
		t.Fatalf("partial categorization = %#v, error = %v", enriched, err)
	}
	if _, err := os.Stat(filepath.Join(runDir, programindex.ArtifactFilename)); !os.IsNotExist(err) {
		t.Fatalf("failed categorization persisted ProgramIndex: %v", err)
	}
	assertPersistedRuntimeDocumentation(t, runDir, documentation)
}

func assertPersistedRuntimeDocumentation(
	t *testing.T,
	runDir string,
	want documentationreduce.Result,
) {
	t.Helper()
	wantBytes, err := documentationreduce.Encode(want)
	if err != nil {
		t.Fatal(err)
	}
	gotBytes, err := os.ReadFile(filepath.Join(runDir, documentationreduce.ArtifactFilename))
	if err != nil {
		t.Fatalf("read reduced documentation artifact: %v", err)
	}
	if !bytes.Equal(gotBytes, wantBytes) {
		t.Fatalf("reduced documentation artifact = %s, want %s", gotBytes, wantBytes)
	}
	restored, err := documentationreduce.Read(runDir)
	if err != nil {
		t.Fatalf("restore reduced documentation artifact: %v", err)
	}
	if restored.GuidanceSHA256 != want.GuidanceSHA256 ||
		restored.ReductionSHA256 != want.ReductionSHA256 {
		t.Fatalf("restored reduced documentation = %#v, want %#v", restored, want)
	}
}

func emptyRuntimeDocumentation(t *testing.T) documentationreduce.Result {
	t.Helper()
	result, err := documentationreduce.Run(
		t.Context(), llm.Executor{}, nil, readmetargetscout.GuidanceSnapshot{},
	)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func runtimeCategorizationIndex(
	t *testing.T,
	name string,
	selector string,
	path string,
	fileRef string,
) programindex.Index {
	t.Helper()
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64),
		SourceSHA256:   strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: "go", Kind: "executable", Name: name, Selector: selector,
			Sources:       []programindex.TargetSource{{FileRef: fileRef, Path: path}},
			AnchorFileRef: fileRef,
		},
		Objects: []programindex.ObjectInput{}, Relations: []programindex.RelationInput{},
		Coverage: programindex.CoverageInput{Measured: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return index
}
