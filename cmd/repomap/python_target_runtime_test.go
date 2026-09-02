package main

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

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
