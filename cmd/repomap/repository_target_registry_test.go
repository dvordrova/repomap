package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/targetoutcome"
)

type syntheticJVMFacts struct {
	SourceSHA256 string
	FileRef      corpus.FileID
}

func syntheticJVMDescriptor() repositoryTargetAdapterDescriptor {
	return repositoryTargetAdapterDescriptor{
		Key: "jvm", Rank: 0, Label: "JVM",
		AllowedLanguages: []string{"java", "kotlin", "scala"},
		SelectorPrefixes: []string{"jvm:"},
		Discover: func(context.Context, repositoryTargetRuntimeOptions) (repositoryTargetAdapterDiscovery, bool, error) {
			return repositoryTargetAdapterDiscovery{}, false, nil
		},
		PrepareDispatchPlan: func(repositoryTargetPlan, []repositoryTypedTarget) (any, error) {
			return struct{}{}, nil
		},
		PrepareDispatchTarget: func(
			_ context.Context, _ repositoryTargetDispatchOptions,
			target repositoryTypedTarget, _ any,
		) (repositoryTargetDispatchBinding, error) {
			return repositoryTargetDispatchBinding{
				Target: target, ProgramFacts: target.native, ProgramFactsBound: true,
			}, nil
		},
		ValidateNative: func(target repositoryTypedTarget) error {
			if _, ok := target.native.(syntheticJVMFacts); !ok {
				return &syntheticAdapterError{"expected one JVM compiler snapshot"}
			}
			return nil
		},
		MatchProgramTarget: func(target repositoryTypedTarget, programTarget programindex.Target) bool {
			return programTarget.Selector == target.Selector && programTarget.Name == target.Display
		},
		ValidatePlanAuthority: func(any, repositoryTypedTarget) error { return nil },
		BuildProgramInput: func(request repositoryProgramBuildRequest) (programindex.Input, error) {
			facts, ok := request.Facts.(syntheticJVMFacts)
			if !ok {
				return programindex.Input{}, &syntheticAdapterError{"compiler facts have the wrong type"}
			}
			fileRef := facts.FileRef
			if fileRef == "" {
				fileRef = "f1"
			}
			return programindex.Input{
				ScenarioSHA256: strings.Repeat("1", 64),
				SourceSHA256:   facts.SourceSHA256,
				Target: programindex.TargetInput{
					Language: "java", Kind: "library", Name: request.Target.Display,
					Selector:      request.Target.Selector,
					Sources:       []programindex.TargetSource{{FileRef: string(fileRef), Path: "src/App.java"}},
					AnchorFileRef: string(fileRef),
					Seeds: []programindex.TargetSeedInput{{
						ObjectRef: "method:App.run", Kind: programindex.SeedCallable,
						Location: &programindex.Location{Path: "src/App.java", Line: 3, Column: 5},
					}},
				},
				Objects: []programindex.ObjectInput{
					{
						SourceRef: "package:example", Kind: programindex.ObjectPackage,
						Name: "example", Visibility: programindex.VisibilityPublic,
					},
					{
						SourceRef: "class:App", Kind: programindex.ObjectType,
						Name: "App", Visibility: programindex.VisibilityPublic,
						OwnerRef: "package:example", ContainerRef: "package:example",
						Location: &programindex.Location{Path: "src/App.java", Line: 1, Column: 1},
					},
					{
						SourceRef: "method:App.run", Kind: programindex.ObjectMethod,
						Name: "run", Signature: "void run()", Visibility: programindex.VisibilityPublic,
						OwnerRef: "class:App", ContainerRef: "package:example",
						Location: &programindex.Location{Path: "src/App.java", Line: 3, Column: 5},
					},
				},
				Relations: []programindex.RelationInput{},
				Coverage: programindex.CoverageInput{
					Measured: true, ObjectsObserved: 3, RelationsObserved: 0,
				},
			}, nil
		},
		BuildDependencies: func(repositoryDependencyBuildRequest) (dependencies.Catalog, error) {
			return dependencies.Empty(), nil
		},
	}
}

type syntheticAdapterError struct{ message string }

func (err *syntheticAdapterError) Error() string { return err.message }

func TestSyntheticJVMAdapterDispatchesToSharedProgramPageWithoutCoreSwitch(t *testing.T) {
	registry, err := newRepositoryTargetAdapterRegistry(syntheticJVMDescriptor())
	if err != nil {
		t.Fatal(err)
	}
	repositoryRoot := t.TempDir()
	sourcePath := filepath.Join(repositoryRoot, "src", "App.java")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("public class App { void run() {} }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := corpus.New(t.Context(), repositoryRoot, gitfiles.Listing{
		Paths: []string{"src/App.java"}, RegularPaths: []string{"src/App.java"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	fileRef, ok := repository.ID("src/App.java")
	if !ok {
		t.Fatal("synthetic JVM source is absent from the corpus")
	}
	facts := syntheticJVMFacts{SourceSHA256: strings.Repeat("2", 64), FileRef: fileRef}
	target, err := newRepositoryTypedTarget(
		registry,
		repositoryTargetKey{Adapter: "jvm", Ref: "compiler-target-1"},
		"jvm:app", "app", targetoutcome.ScopeLibrary, facts,
	)
	if err != nil {
		t.Fatal(err)
	}
	target.FileRefs = []corpus.FileID{fileRef}
	page, err := buildRepositoryProgramPageAuthority(registry, repositoryProgramBuildRequest{
		Target: target, Facts: facts,
	})
	if err != nil {
		t.Fatal(err)
	}
	index := page.ProgramIndex
	if index.Target.Language != "java" ||
		!repositoryTypedTargetMatchesProgramTargetWithRegistry(registry, target, index.Target) {
		t.Fatalf("synthetic JVM target = %#v", index.Target)
	}
	descriptor, _ := registry.descriptor("jvm")
	dispatch, err := descriptor.PrepareDispatchTarget(
		context.Background(), repositoryTargetDispatchOptions{}, target, struct{}{},
	)
	if err != nil {
		t.Fatal(err)
	}
	dispatchedPage, err := buildRepositoryProgramPageAuthority(registry, repositoryProgramBuildRequest{
		Target: dispatch.Target, Facts: dispatch.ProgramFacts,
	})
	if err != nil {
		t.Fatal(err)
	}
	ownedPage, err := ownRepositoryProgramPageAuthority(registry, dispatch.Target, dispatchedPage)
	if err != nil {
		t.Fatal(err)
	}
	if ownedPage.ProgramIndex.Target.Language != "java" ||
		ownedPage.Dependencies.Coverage.State != "complete" {
		t.Fatalf("synthetic JVM shared page authority = %#v", ownedPage)
	}
	methodID := ""
	for _, object := range ownedPage.ProgramIndex.Objects {
		if object.Name == "run" {
			methodID = object.ID
			break
		}
	}
	if methodID == "" {
		t.Fatal("synthetic JVM method is absent from ProgramIndex")
	}
	enriched, err := programindex.Enrich(
		ownedPage.ProgramIndex,
		strings.Repeat("3", 64),
		[]programindex.CategoryAssignment{{
			SubjectID: methodID, Categories: []programindex.Category{programindex.CategoryCore},
		}},
	)
	if err != nil {
		t.Fatalf("synthetic JVM shared categorization: %v", err)
	}
	grouped, diagnostics, err := groupindex.Build(enriched, groupindex.Proposals{Groups: []groupindex.GroupProposal{{
		Key: "execution", Title: "Execution", Summary: "Runs the synthetic JVM target.",
		Lane: groupindex.LaneCore, MemberSubjectIDs: []string{methodID},
	}}})
	if err != nil {
		t.Fatalf("synthetic JVM shared grouping: %v", err)
	}
	if len(diagnostics) != 0 || len(grouped.Groups) != 1 ||
		grouped.Groups[0].Lane != groupindex.LaneCore ||
		len(grouped.Groups[0].MemberSubjectIDs) != 1 ||
		grouped.Groups[0].MemberSubjectIDs[0] != methodID ||
		grouped.ProgramIndexSHA256 != enriched.SHA256 {
		t.Fatalf("synthetic JVM ProgramIndex-to-GroupsIndex contract = %#v / %#v", grouped, diagnostics)
	}
	plan := repositoryTargetPlan{
		Targets: []repositoryTypedTarget{target}, Default: target.Key, Explicit: true,
		Authorities: map[repositoryTargetAdapter]any{"jvm": facts},
		Outcome: targetPortfolioRunOutcome{
			SelectedRef: target.Key.String(), SelectedTargets: 1,
			SelectedTargetRefs: []string{target.Key.String()},
		},
	}
	if err := plan.validateWith(registry); err != nil {
		t.Fatalf("synthetic JVM planned target: %v", err)
	}
}

func TestRepositoryTargetRegistryRejectsAmbiguousNamespaceAndNoncanonicalLanguages(t *testing.T) {
	first := syntheticJVMDescriptor()
	second := syntheticJVMDescriptor()
	second.Key = "other-jvm"
	second.Rank = 1
	if _, err := newRepositoryTargetAdapterRegistry(first, second); err == nil ||
		!strings.Contains(err.Error(), "selector prefix") {
		t.Fatalf("selector prefix collision error = %v", err)
	}
	second.SelectorPrefixes = []string{"jvm:child:"}
	if _, err := newRepositoryTargetAdapterRegistry(first, second); err == nil ||
		!strings.Contains(err.Error(), "overlap") {
		t.Fatalf("selector prefix overlap error = %v", err)
	}

	registry, err := newRepositoryTargetAdapterRegistry(first)
	if err != nil {
		t.Fatal(err)
	}
	target, err := newRepositoryTypedTarget(
		registry,
		repositoryTargetKey{Adapter: "jvm", Ref: "compiler-target-1"},
		"jvm:app", "app", targetoutcome.ScopeLibrary,
		syntheticJVMFacts{SourceSHA256: strings.Repeat("2", 64)},
	)
	if err != nil {
		t.Fatal(err)
	}
	target.AllowedLanguages = []string{"scala", "java", "kotlin"}
	if err := target.validateWith(registry); err == nil || !strings.Contains(err.Error(), "canonical") {
		t.Fatalf("noncanonical allowed languages error = %v", err)
	}
}

func TestSyntheticJVMAdapterRejectsProgramLanguageOutsideDescriptor(t *testing.T) {
	descriptor := syntheticJVMDescriptor()
	build := descriptor.BuildProgramInput
	descriptor.BuildProgramInput = func(request repositoryProgramBuildRequest) (programindex.Input, error) {
		input, err := build(request)
		input.Target.Language = "groovy"
		return input, err
	}
	registry, err := newRepositoryTargetAdapterRegistry(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	facts := syntheticJVMFacts{SourceSHA256: strings.Repeat("2", 64)}
	target, err := newRepositoryTypedTarget(
		registry,
		repositoryTargetKey{Adapter: "jvm", Ref: "compiler-target-1"},
		"jvm:app", "app", targetoutcome.ScopeLibrary, facts,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buildRepositoryProgramPageAuthority(registry, repositoryProgramBuildRequest{
		Target: target, Facts: facts,
	}); err == nil || !strings.Contains(err.Error(), "different ProgramTarget") {
		t.Fatalf("unregistered ProgramTarget language error = %v", err)
	}
}
