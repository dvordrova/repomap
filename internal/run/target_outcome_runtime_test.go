package run

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
	target, err := newGoRepositoryTypedTarget(entry.Candidate.Target, entry.Candidate.Key)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := repositorySelectedTarget("t1", target)
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
	pythonTarget, err := newPythonRepositoryTypedTarget(python)
	if err != nil {
		t.Fatal(err)
	}
	pythonSelected, err := repositorySelectedTarget("t2", pythonTarget)
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
	jstsTarget, err := newJSTSRepositoryTypedTarget(jsts)
	if err != nil {
		t.Fatal(err)
	}
	jstsSelected, err := repositorySelectedTarget("t3", jstsTarget)
	if err != nil {
		t.Fatal(err)
	}
	if jstsSelected.LanguageGroup != targetoutcome.LanguageGroup(repositoryTargetAdapterJSTS) ||
		jstsSelected.ScopeKind != targetoutcome.ScopePackage {
		t.Fatalf("JSTS selected target = %#v", jstsSelected)
	}
}

func TestClassifyRepositoryTargetFailureUsesClosedTypedCauses(t *testing.T) {
	stage, reason := classifyRepositoryTargetFailure(
		targetoutcome.StageProgramAnalysis,
		errors.New("unresolved project reference ./load prepared TypeScript compiler"),
	)
	if stage != targetoutcome.StageProgramAnalysis || reason != targetoutcome.ReasonAnalysisFailed {
		t.Fatalf("source diagnostic was treated as missing compiler: %s/%s", stage, reason)
	}
	stage, reason = classifyRepositoryTargetFailure(
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
		"t1",
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
		"module library: no Go files",
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

// A failed target's saved reason reads with the repository's paths relative,
// whichever spelling of the repository the error used: macOS resolves a
// temporary /var path to /private/var. A failure with no text says so.
func TestFailureDetailReadsTheRepositoryRelative(t *testing.T) {
	repository := t.TempDir()
	resolved, err := filepath.EvalSymlinks(repository)
	if err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{repository, resolved} {
		got := failureDetail(errors.New("clang could not parse "+filepath.Join(root, "src", "a.c")+": "+root+" is read-only"), repository)
		if got != "clang could not parse src/a.c: . is read-only" {
			t.Fatalf("failure detail for %s = %q", root, got)
		}
	}
	if got := failureDetail(errors.New("  "), repository); got != "the failure gave no text to show" || !targetoutcome.ValidFailureDetail(got) {
		t.Fatalf("an error without text reads %q", got)
	}
}
