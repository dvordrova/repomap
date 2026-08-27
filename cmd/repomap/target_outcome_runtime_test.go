package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/dvordrova/repomap/internal/targetoutcome"
)

func TestRepositorySelectedTargetKeepsPreanalysisGoIdentity(t *testing.T) {
	repository, source, project := repositoryTargetRuntimeInlineInputs(t)
	entry := targetPortfolioRuntimeEntry(t, *source.TargetCatalog, ".")
	target := repositoryTypedTarget{
		Key: repositoryTargetKey{
			Adapter: repositoryTargetAdapterGo,
			Ref:     entry.Candidate.Target.Ref,
		},
		Selector: entry.Candidate.Key,
		Go:       &entry.Candidate.Target,
	}
	selected, err := repositorySelectedTarget(target)
	if err != nil {
		t.Fatal(err)
	}
	if selected.LanguageGroup != targetoutcome.LanguageGroupGo ||
		selected.ScopeKind != targetoutcome.ScopeLibrary ||
		selected.DisplayName != entry.DisplayPath || selected.Selector != entry.Candidate.Key {
		t.Fatalf("selected target = %#v", selected)
	}

	pythonCatalog, err := pythontarget.Discover(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	if len(pythonCatalog.Entries) == 0 {
		t.Fatal("inline repository has no Python target")
	}
	python := pythonCatalog.Entries[0]
	pythonSelected, err := repositorySelectedTarget(repositoryTypedTarget{
		Key:      repositoryTargetKey{Adapter: repositoryTargetAdapterPython, Ref: python.Ref},
		Selector: python.Selector,
		Python:   &python,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pythonSelected.LanguageGroup != targetoutcome.LanguageGroupPython ||
		(pythonSelected.ScopeKind != targetoutcome.ScopeExecutable &&
			pythonSelected.ScopeKind != targetoutcome.ScopeLibrary) {
		t.Fatalf("Python selected target = %#v", pythonSelected)
	}

	jsts, err := jstsproject.TargetFromResult(project)
	if err != nil {
		t.Fatal(err)
	}
	jstsSelected, err := repositorySelectedTarget(repositoryTypedTarget{
		Key:      repositoryTargetKey{Adapter: repositoryTargetAdapterJSTS, Ref: jsts.Ref},
		Selector: jsts.Selector,
		JSTS:     &jsts,
	})
	if err != nil {
		t.Fatal(err)
	}
	if jstsSelected.LanguageGroup != targetoutcome.LanguageGroupJavaScriptTypeScript ||
		jstsSelected.ScopeKind != targetoutcome.ScopePackage {
		t.Fatalf("JSTS selected target = %#v", jstsSelected)
	}
}

func TestClassifyRepositoryTargetFailureUsesClosedTypedCauses(t *testing.T) {
	stage, reason := classifyRepositoryTargetFailure(
		targetoutcome.StageTargetPreparation,
		errors.Join(errors.New("prepare"), jstsproject.ErrTypeScriptCompilerUnavailable),
	)
	if stage != targetoutcome.StageTargetPreparation ||
		reason != targetoutcome.ReasonRequiredToolUnavailable {
		t.Fatalf("compiler failure = %s/%s", stage, reason)
	}

	stage, reason = classifyRepositoryTargetFailure(
		targetoutcome.StageSemanticAnalysis,
		llm.NewResourceLimitError(llm.ResourceLimitError{Kind: llm.ResourceLimitOutputTokens}),
	)
	if stage != targetoutcome.StageSemanticAnalysis || reason != targetoutcome.ReasonResourceLimit {
		t.Fatalf("resource failure = %s/%s", stage, reason)
	}
}

func TestPersistTargetOutcomePortfolioForRunDirsKeepsCanonicalBytes(t *testing.T) {
	selected, err := targetoutcome.NewSelectedTarget(
		targetoutcome.LanguageGroupGo,
		targetoutcome.ScopeLibrary,
		"module library",
		"go:module-library",
	)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := targetoutcome.NewNotAnalyzed(
		selected,
		targetoutcome.StageProgramAnalysis,
		targetoutcome.ReasonSourceNotAnalyzable,
	)
	if err != nil {
		t.Fatal(err)
	}
	portfolio, err := targetoutcome.Build(selected.ID, []targetoutcome.Outcome{outcome})
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(t.TempDir(), "20260827-120000-target-a1b2c3")
	if err := persistTargetOutcomePortfolioForRunDirs(portfolio, []string{runDir}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(runDir, targetoutcome.ArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	want, err := portfolio.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("persisted bytes changed: %q", got)
	}
}
