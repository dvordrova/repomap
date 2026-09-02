package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/targetportfolio"
)

func TestReportTargetPortfolioScaleWarningsKeepsCompleteReservoir(t *testing.T) {
	repository := targetPortfolioCorpus(t, false)
	snapshot := repository.Snapshot()
	long := strings.Repeat("complete target evidence ", 4000)
	compilation, err := targetportfolio.Compile(snapshot, []targetportfolio.Candidate{
		{FileRef: snapshot.Entries[0].ID, Hypotheses: []string{long + "one"}},
		{FileRef: snapshot.Entries[1].ID, Hypotheses: []string{long + "two"}},
		{FileRef: snapshot.Entries[2].ID, Hypotheses: []string{long + "three"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var console strings.Builder
	reportTargetPortfolioScaleWarnings(newRunOutput(&console), compilation)
	output := console.String()
	for _, expected := range []string{
		"WARN", "Large target portfolio retained",
		"all merged file candidates, required target refs, executable refs, and hypotheses were retained",
		"bounded disjoint model batches; no local aggregate threshold removed data",
		"complete_candidate_request_bytes",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("console warning missing %q:\n%s", expected, output)
		}
	}
}

type targetPortfolioClientStub struct {
	response []byte
	calls    int
	prompt   llm.Prompt
}

func (stub *targetPortfolioClientStub) State() []byte {
	return []byte(`{"provider":"target-portfolio-test"}`)
}

func (stub *targetPortfolioClientStub) Prepare(
	prompt llm.Prompt,
	_ llm.Limits,
) (llm.Prepared, error) {
	stub.prompt = prompt
	envelope, err := json.Marshal(map[string]any{
		"model": "test",
		"messages": []any{
			map[string]any{"role": "system", "content": prompt.System},
			map[string]any{"role": "user", "content": prompt.User},
		},
	})
	if err != nil {
		return llm.Prepared{}, err
	}
	return llm.NewPrepared(envelope)
}

func (stub *targetPortfolioClientStub) Complete(
	context.Context,
	llm.Prepared,
) (llm.Completion, error) {
	stub.calls++
	return llm.Completion{
		Response:     append([]byte(nil), stub.response...),
		FinishReason: llm.FinishStop,
		ChoiceCount:  1,
		Metrics: llm.Metrics{
			Attempts: 1, ProviderResponseBytes: len(stub.response), Latency: time.Millisecond,
		},
	}, nil
}

func TestTargetPortfolioRepoNameUsesModuleBeforeSemanticMajorSuffix(t *testing.T) {
	for _, test := range []struct {
		identity string
		want     string
	}{
		{identity: "go.etcd.io/etcd/v3", want: "etcd"},
		{identity: "github.com/acme/tool/v2/", want: "tool"},
		{identity: "example.com/tool/v1", want: "v1"},
		{identity: "example.com/tool", want: "tool"},
	} {
		if got := targetPortfolioRepoName(test.identity); got != test.want {
			t.Fatalf("targetPortfolioRepoName(%q) = %q, want %q", test.identity, got, test.want)
		}
	}
}

func targetPortfolioCorpus(t *testing.T, withReadme bool) *corpus.Corpus {
	t.Helper()
	repository := t.TempDir()
	paths := []string{"cmd/api/main.go", "cmd/worker/main.go", "pkg/client/client.go", "scripts/tool.py"}
	if withReadme {
		paths = append(paths, "README.md")
	}
	for _, filePath := range paths {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(repository, filePath)), 0o700); err != nil {
			t.Fatal(err)
		}
		content := "package fixture\n"
		if filePath == "scripts/tool.py" {
			content = "def main(): pass\n"
		}
		if filePath == "README.md" {
			content = "# repomap\n\nRun the worker command.\n"
		}
		if err := os.WriteFile(filepath.Join(repository, filePath), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	opened, err := corpus.New(context.Background(), repository, gitfiles.Listing{
		Paths: paths, RegularPaths: paths,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = opened.Close() })
	return opened
}

func targetPortfolioRuntimeFacts() gofacts.Facts {
	modulePath := "example.com/repomap"
	return gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "module-root", ModulePath: modulePath, ModuleDir: ".", Main: true,
		}},
		Packages: []gofacts.PackageFact{
			targetPortfolioRuntimePackage(modulePath, "cmd/api", "main", nil),
			targetPortfolioRuntimePackage(modulePath, "cmd/worker", "main", nil),
			targetPortfolioRuntimePackage(modulePath, "pkg/client", "client", []gofacts.PackageDeclaration{{
				Kind: gofacts.PackageDeclarationFunc, Name: "NewClient",
				Path: "pkg/client/client.go", Line: 1, Column: 6, ExecutableBody: true,
			}}),
		},
		EntrypointPackages: []gofacts.Entrypoint{
			targetPortfolioRuntimeEntrypoint(modulePath, "cmd/api", 1),
			targetPortfolioRuntimeEntrypoint(modulePath, "cmd/worker", 1),
		},
	}
}

func targetPortfolioRuntimePackage(
	modulePath string,
	dir string,
	name string,
	declarations []gofacts.PackageDeclaration,
) gofacts.PackageFact {
	fileName := "main.go"
	if dir == "pkg/client" {
		fileName = "client.go"
	}
	return gofacts.PackageFact{
		CanonicalPath: modulePath + "/" + dir,
		Name:          name, ModuleID: "module-root", ModulePath: modulePath,
		PackageDir: dir, ModuleRelativeDir: dir, DisplayPath: dir,
		Locality: "local", Files: []string{dir + "/" + fileName},
		Declarations: declarations, DeclarationsScanned: true,
		LoadCompleteness: completeGoPackageLoad(),
	}
}

func completeGoPackageLoad() *gofacts.PackageLoadCompleteness {
	return &gofacts.PackageLoadCompleteness{
		Version: gofacts.PackageLoadCompletenessVersion,
		State:   gofacts.PackageLoadComplete,
	}
}

func targetPortfolioRuntimeEntrypoint(modulePath, dir string, line int) gofacts.Entrypoint {
	return gofacts.Entrypoint{
		ModulePath: modulePath, ImportPath: modulePath + "/" + dir,
		PackageDir: dir, ModuleRelativeDir: dir, ModuleDir: ".", Kind: "unknown",
		GoFiles: []string{dir + "/main.go"},
		Anchors: []gofacts.EntrypointAnchor{{
			Version: gofacts.EntrypointAnchorVersion,
			Kind:    gofacts.EntrypointAnchorGoMain,
			Path:    dir + "/main.go", Line: line,
		}},
	}
}

func targetPortfolioRuntimeCatalog(t *testing.T, facts gofacts.Facts) analysistarget.TargetCatalog {
	t.Helper()
	catalog, err := analysistarget.BuildCatalog(facts)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func targetPortfolioRuntimeEntry(
	t *testing.T,
	catalog analysistarget.TargetCatalog,
	displayPath string,
) analysistarget.TargetCatalogEntry {
	t.Helper()
	for _, entry := range catalog.Entries {
		if entry.DisplayPath == displayPath {
			return entry
		}
	}
	t.Fatalf("catalog has no display path %q", displayPath)
	return analysistarget.TargetCatalogEntry{}
}
