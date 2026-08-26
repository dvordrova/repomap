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
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/llm"
)

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

func TestTargetSelectionRunsREADMEAndGoScoutsThenSelectsByFileRef(t *testing.T) {
	repository := targetPortfolioCorpus(t, true)
	facts := targetPortfolioRuntimeFacts()
	catalog := targetPortfolioRuntimeCatalog(t, facts)

	readmeProvider := &targetPortfolioClientStub{
		response: []byte(`[{"file_ref":"f3","classifications":[{"class":"target_entry","hypotheses":["README documents the worker command"]}]},{"file_ref":"f5","classifications":[{"class":"target_entry","hypotheses":["README documents a Python helper product"]}]}]`),
	}
	portfolioProvider := &targetPortfolioClientStub{
		response: []byte(`{"default_file_ref":"f3","target_file_refs":["f3"]}`),
	}
	providerCalls := 0
	selection, outcome, err := selectTargetsForRun(
		context.Background(),
		"example.com/repomap",
		catalog,
		facts,
		repository,
		"",
		nil,
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
	worker := targetPortfolioRuntimeEntry(t, catalog, "cmd/worker")
	if selection.DefaultTargetRef != worker.Candidate.Target.Ref ||
		len(selection.TargetRefs) != 1 || selection.TargetRefs[0] != worker.Candidate.Target.Ref {
		t.Fatalf("selection = %#v, want worker target", selection)
	}
	if outcome.SelectedTargets != 1 || len(outcome.ReadmeRoles) != 2 {
		t.Fatalf("outcome = %#v", outcome)
	}
	if readmeProvider.calls != 1 || portfolioProvider.calls != 1 || providerCalls != 2 {
		t.Fatalf("provider calls = README %d, portfolio %d, factories %d", readmeProvider.calls, portfolioProvider.calls, providerCalls)
	}
	if readmeProvider.prompt.ResponseFormatJSON {
		t.Fatal("direct README array response incorrectly requested json_object mode")
	}
	if !portfolioProvider.prompt.ResponseFormatJSON ||
		!strings.Contains(portfolioProvider.prompt.User, "README documents the worker command") ||
		strings.Contains(portfolioProvider.prompt.User, "Python helper product") {
		t.Fatalf("portfolio prompt did not receive merged README hypothesis: %#v", portfolioProvider.prompt)
	}
}

func TestExplicitTargetRunsReadmeCubeButBypassesPortfolioCube(t *testing.T) {
	repository := targetPortfolioCorpus(t, true)
	facts := targetPortfolioRuntimeFacts()
	catalog := targetPortfolioRuntimeCatalog(t, facts)
	providerCalls := 0
	readmeProvider := &targetPortfolioClientStub{response: []byte(`[]`)}

	selection, _, err := selectTargetsForRun(
		context.Background(), "example.com/repomap", catalog, facts, repository,
		"cmd/api", nil,
		func() (llm.Provider, error) {
			providerCalls++
			return readmeProvider, nil
		},
		llm.Executor{Enabled: false},
	)
	if err != nil {
		t.Fatal(err)
	}
	api := targetPortfolioRuntimeEntry(t, catalog, "cmd/api")
	if selection.DefaultTargetRef != api.Candidate.Target.Ref ||
		len(selection.TargetRefs) != 1 || providerCalls != 1 || readmeProvider.calls != 1 {
		t.Fatalf("explicit selection = %#v, provider calls = %d", selection, providerCalls)
	}
}

func TestTargetPortfolioInvalidJSONFailsClosed(t *testing.T) {
	repository := targetPortfolioCorpus(t, false)
	facts := targetPortfolioRuntimeFacts()
	catalog := targetPortfolioRuntimeCatalog(t, facts)
	provider := &targetPortfolioClientStub{response: []byte(`{"target_file_refs":[]}`)}

	_, _, err := selectTargetsForRun(
		context.Background(), "example.com/repomap", catalog, facts, repository,
		"", nil, func() (llm.Provider, error) { return provider, nil },
		llm.Executor{Enabled: false},
	)
	if err == nil || !strings.Contains(err.Error(), "required target portfolio selection") ||
		!strings.Contains(err.Error(), "exact --target choices: Go:") {
		t.Fatalf("invalid response error = %v", err)
	}
}

func TestTargetPortfolioMayHonestlyRejectEveryCandidate(t *testing.T) {
	repository := targetPortfolioCorpus(t, false)
	facts := targetPortfolioRuntimeFacts()
	catalog := targetPortfolioRuntimeCatalog(t, facts)
	provider := &targetPortfolioClientStub{
		response: []byte(`{"default_file_ref":null,"target_file_refs":[]}`),
	}

	_, outcome, err := selectTargetsForRun(
		context.Background(), "example.com/repomap", catalog, facts, repository,
		"", nil, func() (llm.Provider, error) { return provider, nil },
		llm.Executor{Enabled: false},
	)
	if err == nil || !strings.Contains(err.Error(), "no positively supported target entry") ||
		!strings.Contains(err.Error(), "--target TARGET") {
		t.Fatalf("empty positive selection error = %v", err)
	}
	if outcome.SemanticState != debugdump.SemanticStateAccepted ||
		outcome.UnclassifiedFiles == 0 || outcome.SelectedFileRefs != 0 || provider.calls != 1 {
		t.Fatalf("empty positive selection outcome = %#v, calls = %d", outcome, provider.calls)
	}
}

func TestTargetPortfolioRejectsLibraryOnlyReductionWhenExactExecutableExists(t *testing.T) {
	repository := targetPortfolioCorpus(t, false)
	facts := targetPortfolioRuntimeFacts()
	catalog := targetPortfolioRuntimeCatalog(t, facts)
	provider := &targetPortfolioClientStub{
		response: []byte(`{"default_file_ref":"f3","target_file_refs":["f3"]}`),
	}

	_, outcome, err := selectTargetsForRun(
		context.Background(), "example.com/repomap", catalog, facts, repository,
		"", nil, func() (llm.Provider, error) { return provider, nil },
		llm.Executor{Enabled: false},
	)
	if err == nil || !strings.Contains(err.Error(), "exact executable authority") {
		t.Fatalf("library-only reduction error = %v", err)
	}
	if outcome.SelectedFileRefs != 0 || provider.calls != 1 {
		t.Fatalf("library-only reduction outcome = %#v, calls = %d", outcome, provider.calls)
	}
}

func TestTargetPortfolioRequiresExecutableDefaultAndPreservesSupportingLibrary(t *testing.T) {
	for _, test := range []struct {
		name     string
		response string
		wantErr  bool
	}{
		{
			name:     "library default with executable selected",
			response: `{"default_file_ref":"f3","target_file_refs":["f1","f3"]}`,
			wantErr:  true,
		},
		{
			name:     "executable default with library selected in reverse response order",
			response: `{"default_file_ref":"f1","target_file_refs":["f3","f1"]}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := targetPortfolioCorpus(t, false)
			facts := targetPortfolioRuntimeFacts()
			catalog := targetPortfolioRuntimeCatalog(t, facts)
			provider := &targetPortfolioClientStub{response: []byte(test.response)}

			selection, outcome, err := selectTargetsForRun(
				context.Background(), "example.com/repomap", catalog, facts, repository,
				"", nil, func() (llm.Provider, error) { return provider, nil },
				llm.Executor{Enabled: false},
			)
			if test.wantErr {
				if err == nil || !strings.Contains(err.Error(), "executable") {
					t.Fatalf("authority error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			api := targetPortfolioRuntimeEntry(t, catalog, "cmd/api")
			library := targetPortfolioRuntimeEntry(t, catalog, ".")
			if selection.DefaultTargetRef != api.Candidate.Target.Ref ||
				!exactRefSet(selection.TargetRefs, []string{
					api.Candidate.Target.Ref, library.Candidate.Target.Ref,
				}) || outcome.SelectedTargets != 2 || outcome.SelectedFileRefs != 2 {
				t.Fatalf("selection = %#v, outcome = %#v", selection, outcome)
			}
		})
	}
}

func TestTargetPortfolioLibraryOnlyCatalogRetainsLibraryAuthority(t *testing.T) {
	repository := targetPortfolioCorpus(t, false)
	facts := targetPortfolioRuntimeLibraryOnlyFacts()
	catalog := targetPortfolioRuntimeCatalog(t, facts)
	provider := &targetPortfolioClientStub{
		response: []byte(`{"default_file_ref":"f3","target_file_refs":["f3"]}`),
	}

	selection, outcome, err := selectTargetsForRun(
		context.Background(), "example.com/repomap", catalog, facts, repository,
		"", nil, func() (llm.Provider, error) { return provider, nil },
		llm.Executor{Enabled: false},
	)
	if err != nil {
		t.Fatal(err)
	}
	library := targetPortfolioRuntimeEntry(t, catalog, ".")
	if selection.DefaultTargetRef != library.Candidate.Target.Ref ||
		len(selection.TargetRefs) != 1 || selection.TargetRefs[0] != library.Candidate.Target.Ref ||
		outcome.SelectedTargets != 1 {
		t.Fatalf("library-only selection = %#v, outcome = %#v", selection, outcome)
	}
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

func targetPortfolioRuntimeLibraryOnlyFacts() gofacts.Facts {
	modulePath := "example.com/repomap"
	return gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "module-root", ModulePath: modulePath, ModuleDir: ".", Main: true,
		}},
		Packages: []gofacts.PackageFact{
			targetPortfolioRuntimePackage(modulePath, "pkg/client", "client", []gofacts.PackageDeclaration{{
				Kind: gofacts.PackageDeclarationFunc, Name: "NewClient",
				Path: "pkg/client/client.go", Line: 1, Column: 6, ExecutableBody: true,
			}}),
		},
		EntrypointPackages: []gofacts.Entrypoint{},
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
