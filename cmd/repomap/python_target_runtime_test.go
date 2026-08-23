package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

func TestPythonTargetSelectionResolvesREADMEFileToFrameworkNeutralModuleView(t *testing.T) {
	repository := pythonModuleTargetCorpus(t)
	mainRef, ok := repository.ID("src/main.py")
	if !ok {
		t.Fatal("fixture has no main source ref")
	}
	portfolioResponse, err := json.Marshal(map[string]any{
		"default_file_ref": mainRef,
		"target_file_refs": []corpus.FileID{mainRef},
	})
	if err != nil {
		t.Fatal(err)
	}
	readmeResponse, err := json.Marshal([]any{map[string]any{
		"file_ref": mainRef,
		"classifications": []any{map[string]any{
			"class": "target_entry", "hypotheses": []string{"README names this module as the service start"},
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	readmeProvider := &targetPortfolioClientStub{response: readmeResponse}
	portfolioProvider := &targetPortfolioClientStub{response: portfolioResponse}
	providerCalls := 0
	selection, err := selectPythonTargetsForRun(
		context.Background(), "python-service", repository, "", nil,
		func() (llm.Provider, error) {
			providerCalls++
			if providerCalls == 1 {
				return readmeProvider, nil
			}
			return portfolioProvider, nil
		},
		llm.Executor{Enabled: false},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(selection.Default.Selector, "python:module-execution:") ||
		len(selection.Default.Roots) != 1 || selection.Default.Roots[0].Kind != "module_execution" ||
		len(selection.Targets) != 1 || selection.Targets[0].Ref != selection.Default.Ref {
		t.Fatalf("README-selected module did not become one exact module view: %#v", selection)
	}
	if !selection.Catalog.OwnsTarget(selection.Default) {
		t.Fatalf("derived module view is not bound to selection catalog: %#v", selection.Default)
	}
	if selection.Outcome.SelectedFileRefs != 1 || selection.Outcome.SelectedTargets != 1 ||
		providerCalls != 2 || readmeProvider.calls != 1 || portfolioProvider.calls != 1 {
		t.Fatalf(
			"module selection outcome = %#v, provider factory calls = %d, completions = %d/%d",
			selection.Outcome, providerCalls, readmeProvider.calls, portfolioProvider.calls,
		)
	}
	if !strings.Contains(portfolioProvider.prompt.User, `"path":"src/main.py"`) ||
		!strings.Contains(portfolioProvider.prompt.User, `"path":"pyproject.toml"`) {
		t.Fatalf("portfolio did not receive README module and native library hypotheses: %s", portfolioProvider.prompt.User)
	}
}

func TestPythonExplicitTargetAcceptsOnlyExactDerivedModuleSelector(t *testing.T) {
	repository := pythonModuleTargetCorpus(t)
	catalog, err := pythontarget.Discover(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := pythontarget.NewFileTargetResolver(repository, catalog)
	if err != nil {
		t.Fatal(err)
	}
	mainRef, ok := repository.ID("src/main.py")
	if !ok {
		t.Fatal("fixture has no main source ref")
	}
	want, err := resolver.ResolveOne(mainRef)
	if err != nil {
		t.Fatal(err)
	}
	got, err := resolvePythonTargetOverride(catalog, resolver, want.Selector)
	if err != nil || got.Ref != want.Ref {
		t.Fatalf("exact derived selector = %#v, %v; want %q", got, err, want.Ref)
	}
	if _, err := resolvePythonTargetOverride(catalog, resolver, "src/main.py"); err == nil ||
		!strings.Contains(err.Error(), "use one exact selector") ||
		!strings.Contains(err.Error(), want.Selector+" (src/main.py)") {
		t.Fatalf("derived module path became an implicit target alias: %v; catalog=%#v", err, catalog.Entries)
	}
}

func TestPythonExactTargetSurvivesUnrelatedDiscoveryOmission(t *testing.T) {
	repository := pythonTargetCorpus(t, map[string]string{
		"README.md": "# App\n\nRun `python app.py`.\n",
		"app.py": `def main():
    return 0

if __name__ == "__main__":
    raise SystemExit(main())
`,
		"setup.py": `from setuptools import setup

ENTRY_POINTS = load_entry_points()
setup(name="partial-fixture", entry_points=ENTRY_POINTS)
`,
	})
	catalog, err := pythontarget.Discover(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Coverage != pythontarget.CoveragePartial || len(catalog.Omissions) == 0 {
		t.Fatalf("fixture did not retain its typed omission: %#v", catalog)
	}
	selector := ""
	for _, target := range catalog.Entries {
		if strings.Contains(target.Selector, ":guard:app") {
			selector = target.Selector
			break
		}
	}
	if selector == "" {
		t.Fatalf("fixture has no exact app target: %#v", catalog.Entries)
	}
	readmeProvider := &targetPortfolioClientStub{response: []byte(`[]`)}
	providerCalls := 0
	selection, err := selectPythonTargetsForRun(
		context.Background(), "partial-python", repository, selector, nil,
		func() (llm.Provider, error) {
			providerCalls++
			return readmeProvider, nil
		},
		llm.Executor{Enabled: false},
	)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Catalog.Coverage != pythontarget.CoveragePartial ||
		selection.Default.Selector != selector || providerCalls != 1 || readmeProvider.calls != 1 {
		t.Fatalf("partial exact selection = %#v; provider calls = %d/%d", selection, providerCalls, readmeProvider.calls)
	}
}

func pythonModuleTargetCorpus(t *testing.T) *corpus.Corpus {
	t.Helper()
	return pythonTargetCorpus(t, map[string]string{
		"README.md": "# Web\n\nRun `src/main.py` to start the service.\n",
		"pyproject.toml": `[tool.poetry]
name = "web"
version = "0.1.0"
`,
		"src/__init__.py": "",
		"src/config.py":   "VALUE = 1\n",
		"src/main.py": `from runtime import Whatever

app = Whatever()
`,
	})
}

func pythonTargetCorpus(t *testing.T, files map[string]string) *corpus.Corpus {
	t.Helper()
	root := t.TempDir()
	paths := make([]string, 0, len(files))
	for filePath := range files {
		paths = append(paths, filePath)
	}
	sort.Strings(paths)
	for _, filePath := range paths {
		absolute := filepath.Join(root, filepath.FromSlash(filePath))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(files[filePath]), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	repository, err := corpus.New(context.Background(), root, gitfiles.Listing{
		Paths: paths, RegularPaths: paths,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	return repository
}
