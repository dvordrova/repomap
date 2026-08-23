package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

func TestMixedTargetSelectionStopsOnEveryPositivePythonSubset(t *testing.T) {
	repository, facts, goCatalog, pythonCatalog := mixedTargetRuntimeFixture(t)
	goFileRef, _ := repository.ID("cmd/api/main.go")
	pythonFileRef, _ := repository.ID("python/main.py")
	goResolver, err := analysistarget.NewGoFileTargetResolver(repository, facts, goCatalog)
	if err != nil {
		t.Fatal(err)
	}
	goRefs, err := goResolver.Resolve([]corpus.FileID{goFileRef})
	if err != nil {
		t.Fatal(err)
	}
	pythonResolver, err := pythontarget.NewFileTargetResolver(repository, pythonCatalog)
	if err != nil {
		t.Fatal(err)
	}
	pythonTargets, err := pythonResolver.Resolve([]corpus.FileID{pythonFileRef})
	if err != nil {
		t.Fatal(err)
	}
	pythonRefs := pythonTargetRefs(pythonTargets)

	for _, test := range []struct {
		name           string
		defaultFileRef corpus.FileID
		targetFileRefs []corpus.FileID
		wantRefs       []string
		wantDefaultRef string
	}{
		{
			name: "Python selected beside Go default", defaultFileRef: goFileRef,
			targetFileRefs: []corpus.FileID{goFileRef, pythonFileRef},
			wantRefs:       append(append([]string(nil), goRefs...), pythonRefs...),
			wantDefaultRef: goRefs[0],
		},
		{
			name: "Python default", defaultFileRef: pythonFileRef,
			targetFileRefs: []corpus.FileID{pythonFileRef},
			wantRefs:       append([]string(nil), pythonRefs...),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			response, marshalErr := json.Marshal(map[string]any{
				"default_file_ref": test.defaultFileRef,
				"target_file_refs": test.targetFileRefs,
			})
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			provider := &targetPortfolioClientStub{response: response}
			selection, runErr := selectMixedTargetsForRun(
				context.Background(), "example.com/repomap", goCatalog, facts, pythonCatalog,
				repository, nil, func() (llm.Provider, error) { return provider, nil },
				llm.Executor{Enabled: false},
			)
			if runErr == nil || !strings.Contains(runErr.Error(), "cannot publish complete semantics") ||
				!strings.Contains(runErr.Error(), "--target") ||
				!strings.Contains(runErr.Error(), "exact choice: Go") ||
				!strings.Contains(runErr.Error(), "or Python") {
				t.Fatalf("mixed Python selection error = %v", runErr)
			}
			if len(selection.Go.TargetRefs) != 0 {
				t.Fatalf("terminal mixed selection exposed Go execution refs: %#v", selection.Go)
			}
			outcome := selection.Outcome
			if outcome.SelectedFileRefs != len(test.targetFileRefs) ||
				outcome.SelectedTargets != len(test.wantRefs) ||
				!exactRefSet(outcome.SelectedTargetRefs, test.wantRefs) ||
				outcome.SelectedRef != test.wantDefaultRef {
				t.Fatalf("terminal mixed outcome = %#v, want refs %#v", outcome, test.wantRefs)
			}
		})
	}
}

func TestMixedTargetSelectionAllowsExactGoOnlyPositiveSubset(t *testing.T) {
	repository, facts, goCatalog, pythonCatalog := mixedTargetRuntimeFixture(t)
	goFileRef, _ := repository.ID("cmd/api/main.go")
	response, err := json.Marshal(map[string]any{
		"default_file_ref": goFileRef,
		"target_file_refs": []corpus.FileID{goFileRef},
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := &targetPortfolioClientStub{response: response}
	selection, err := selectMixedTargetsForRun(
		context.Background(), "example.com/repomap", goCatalog, facts, pythonCatalog,
		repository, nil, func() (llm.Provider, error) { return provider, nil },
		llm.Executor{Enabled: false},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection.Go.TargetRefs) != 1 ||
		selection.Go.DefaultTargetRef != selection.Go.TargetRefs[0] ||
		selection.Outcome.SelectedTargets != 1 ||
		selection.Outcome.SelectedFileRefs != 1 ||
		!exactRefSet(selection.Outcome.SelectedTargetRefs, selection.Go.TargetRefs) {
		t.Fatalf("Go-only mixed selection = %#v", selection)
	}
}

func mixedTargetRuntimeFixture(
	t *testing.T,
) (*corpus.Corpus, gofacts.Facts, analysistarget.TargetCatalog, pythontarget.Catalog) {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod":                  "module example.com/repomap\n",
		"cmd/api/main.go":         "package main\nfunc main() {}\n",
		"cmd/worker/main.go":      "package main\nfunc main() {}\n",
		"pkg/client/client.go":    "package client\nfunc NewClient() {}\n",
		"python/requirements.txt": "requests==2.0\n",
		"python/main.py":          "if __name__ == '__main__':\n    pass\n",
	}
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
	facts := targetPortfolioRuntimeFacts()
	goCatalog := targetPortfolioRuntimeCatalog(t, facts)
	pythonCatalog, err := pythontarget.Discover(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	if len(pythonCatalog.Entries) == 0 {
		t.Fatal("mixed fixture discovered no Python target")
	}
	return repository, facts, goCatalog, pythonCatalog
}
