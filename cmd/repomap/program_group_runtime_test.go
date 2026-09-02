package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/report"
)

func TestGroupProgramIndexPersistsOnePageLocalGraph(t *testing.T) {
	program := runtimeGroupedProgramIndex(t, "api", "go:./cmd/api", "cmd/api/main.go", "f-api")
	runner := func(
		_ context.Context,
		executor llm.Executor,
		provider llm.Provider,
		program programindex.Index,
	) (groupindex.Index, []groupindex.Diagnostic, error) {
		if provider != nil || executor.BatchConcurrency != 2 {
			t.Fatalf("grouping runtime wiring = provider %#v, executor %#v", provider, executor)
		}
		return groupindex.Build(program, groupindex.Proposals{})
	}
	runDir := t.TempDir()
	grouped, err := groupProgramIndexForRun(
		t.Context(), runDir, t.TempDir(), true, 2, &llm.BatchController{}, nil,
		runner, program, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if grouped.Target.ID != program.Target.ID || grouped.ProgramIndexSHA256 != program.SHA256 {
		t.Fatalf("page-local GroupsIndex = %#v", grouped)
	}
	if _, err := os.Stat(filepath.Join(runDir, groupindex.ArtifactFilename)); err != nil {
		t.Fatalf("missing %s: %v", groupindex.ArtifactFilename, err)
	}
}

func TestGroupProgramIndexRejectsFailedPageWithoutPersistence(t *testing.T) {
	program := runtimeGroupedProgramIndex(t, "api", "go:./cmd/api", "cmd/api/main.go", "f-api")
	calls := 0
	runner := func(
		_ context.Context,
		_ llm.Executor,
		_ llm.Provider,
		program programindex.Index,
	) (groupindex.Index, []groupindex.Diagnostic, error) {
		calls++
		return groupindex.Index{}, nil, errors.New("rejected graph")
	}
	runDir := t.TempDir()
	grouped, err := groupProgramIndexForRun(
		t.Context(), runDir, t.TempDir(), true, 1, &llm.BatchController{}, nil,
		runner, program, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "rejected graph") || grouped.Target.ID != "" || calls != 1 {
		t.Fatalf("partial grouping = %#v, error = %v", grouped, err)
	}
	if _, err := os.Stat(filepath.Join(runDir, groupindex.ArtifactFilename)); !os.IsNotExist(err) {
		t.Fatalf("failed grouping persisted %s: %v", groupindex.ArtifactFilename, err)
	}
}

func TestMatchPublishedRunGroupsOwnsAndPersistsCompleteSet(t *testing.T) {
	programs := []programindex.Index{
		runtimeGroupedProgramIndex(t, "api", "go:./cmd/api", "cmd/api/main.go", "f-api"),
		runtimeGroupedProgramIndex(t, "worker", "python:worker", "worker.py", "f-worker"),
	}
	runs := make([]targetPublishedRun, len(programs))
	for position, program := range programs {
		grouped, _, err := groupindex.Build(program, groupindex.Proposals{})
		if err != nil {
			t.Fatal(err)
		}
		runDir := t.TempDir()
		runs[position] = targetPublishedRun{
			RunID: "run-" + program.Target.Name, RunDir: runDir, GroupIndex: grouped,
			ProgramPage: report.TargetNavigationPage{ProgramTarget: program.Target},
		}
	}
	calls := 0
	runner := func(
		_ context.Context,
		executor llm.Executor,
		provider llm.Provider,
		indexes []groupindex.Index,
	) ([]groupindex.Index, []groupindex.Diagnostic, error) {
		calls++
		if provider != nil || executor.BatchConcurrency != 3 {
			t.Fatalf("matching runtime wiring = provider %#v, executor %#v", provider, executor)
		}
		result := make([]groupindex.Index, len(indexes))
		for position := range indexes {
			result[position] = indexes[position].Snapshot()
		}
		return result, nil, nil
	}
	matched, err := matchPublishedRunGroups(
		t.Context(), t.TempDir(), true, 3, &llm.BatchController{}, nil,
		runner, runs, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(matched) != len(runs) {
		t.Fatalf("matched count = %d, calls = %d", len(matched), calls)
	}
	for position, run := range matched {
		restored, err := groupindex.Read(run.RunDir)
		if err != nil {
			t.Fatal(err)
		}
		if restored.SHA256 != run.GroupIndex.SHA256 || runs[position].GroupIndex.SHA256 != run.GroupIndex.SHA256 {
			t.Fatalf("matched run %d changed unexpected authority", position)
		}
	}
}

func TestMatchPublishedRunGroupsNeedsNoProviderForCandidateFreeGraphs(t *testing.T) {
	indexes := []groupindex.Index{
		runtimeCandidateFreeGroupIndex(t, "go", "api", "go:./cmd/api", "cmd/api/main.go", "f-api"),
		runtimeCandidateFreeGroupIndex(t, "python", "worker", "python:worker", "worker.py", "f-worker"),
	}
	runs := make([]targetPublishedRun, len(indexes))
	for position, index := range indexes {
		runs[position] = targetPublishedRun{
			RunID: "run-" + index.Target.Name, RunDir: t.TempDir(), GroupIndex: index,
			ProgramPage: report.TargetNavigationPage{ProgramTarget: index.Target},
		}
	}
	providerFactoryCalls := 0
	matched, err := matchPublishedRunGroups(
		t.Context(), t.TempDir(), true, 2, &llm.BatchController{}, func() (llm.Provider, error) {
			providerFactoryCalls++
			return nil, errors.New("candidate-free matching must not configure a provider")
		},
		nil, runs, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(matched) != len(runs) {
		t.Fatalf("candidate-free matched runs = %d, want %d", len(matched), len(runs))
	}
	if providerFactoryCalls != 0 {
		t.Fatalf("candidate-free provider factory calls = %d, want 0", providerFactoryCalls)
	}
	for position := range matched {
		if matched[position].GroupIndex.SHA256 != indexes[position].SHA256 {
			t.Fatalf("candidate-free graph %d changed authority", position)
		}
	}
}

func runtimeGroupedProgramIndex(
	t *testing.T,
	name string,
	selector string,
	path string,
	fileRef string,
) programindex.Index {
	t.Helper()
	base := runtimeCategorizationIndex(t, name, selector, path, fileRef)
	enriched, err := programindex.Enrich(base, strings.Repeat("c", 64), nil)
	if err != nil {
		t.Fatal(err)
	}
	return enriched
}

func runtimeCandidateFreeGroupIndex(
	t *testing.T,
	language string,
	name string,
	selector string,
	path string,
	fileRef string,
) groupindex.Index {
	t.Helper()
	base, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: language, Kind: "application", Name: name, Selector: selector,
			Sources: []programindex.TargetSource{{FileRef: fileRef, Path: path}}, AnchorFileRef: fileRef,
		},
		Objects: []programindex.ObjectInput{{
			SourceRef: "entry", Kind: programindex.ObjectFunction, Name: "entry",
			Visibility: programindex.VisibilityPublic,
			Location:   &programindex.Location{Path: path, Line: 1, Column: 1},
		}},
		Relations: []programindex.RelationInput{},
		Coverage:  programindex.CoverageInput{Measured: true, ObjectsObserved: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	enriched, err := programindex.Enrich(base, strings.Repeat("c", 64), []programindex.CategoryAssignment{{
		SubjectID: base.Objects[0].ID, Categories: []programindex.Category{programindex.CategoryCore},
	}})
	if err != nil {
		t.Fatal(err)
	}
	grouped, diagnostics, err := groupindex.Build(enriched, groupindex.Proposals{Groups: []groupindex.GroupProposal{{
		Key: "core", Title: name + " core", Summary: "Owns local work.", Lane: groupindex.LaneCore,
		MemberSubjectIDs: []string{enriched.Objects[0].ID},
	}}})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("candidate-free group: diagnostics=%#v err=%v", diagnostics, err)
	}
	return grouped
}
