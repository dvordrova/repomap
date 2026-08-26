package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/dvordrova/repomap/internal/snapshot"
)

func TestRepositoryTargetPlanRestoresAllThreeAdaptersWithEachTypedDefault(t *testing.T) {
	for _, test := range []struct {
		name        string
		defaultPath string
		wantAdapter repositoryTargetAdapter
	}{
		{name: "Go default", defaultPath: "cmd/api/main.go", wantAdapter: repositoryTargetAdapterGo},
		{name: "Go library default", defaultPath: "pkg/client/client.go", wantAdapter: repositoryTargetAdapterGo},
		{name: "Python default", defaultPath: "native/runtime.py", wantAdapter: repositoryTargetAdapterPython},
		{name: "JavaScript TypeScript default", defaultPath: "package.json", wantAdapter: repositoryTargetAdapterJSTS},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository, goSource, project := repositoryTargetRuntimeInlineInputs(t)
			goFileRef := repositoryTargetRuntimeFileRef(t, repository, "cmd/api/main.go")
			goWorkerFileRef := repositoryTargetRuntimeFileRef(t, repository, "cmd/worker/main.go")
			goLibraryFileRef := repositoryTargetRuntimeFileRef(t, repository, "pkg/client/client.go")
			pythonFileRef := repositoryTargetRuntimeFileRef(t, repository, "native/runtime.py")
			manifestRef := repositoryTargetRuntimeFileRef(t, repository, "package.json")
			defaultRef := repositoryTargetRuntimeFileRef(t, repository, test.defaultPath)
			response, err := json.Marshal(map[string]any{
				"default_file_ref": defaultRef,
				"target_file_refs": []corpus.FileID{
					manifestRef, pythonFileRef, goFileRef, goWorkerFileRef, goLibraryFileRef,
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			readmeProvider := &targetPortfolioClientStub{response: []byte(`[]`)}
			portfolioProvider := &targetPortfolioClientStub{response: response}
			providerCalls := 0
			discoveryCalls := 0

			plan, err := selectRepositoryTargetPlanForRun(
				context.Background(),
				repositoryTargetRuntimeTestOptions(
					t, repository, &goSource, project, "", &providerCalls,
					&discoveryCalls, readmeProvider, portfolioProvider,
				),
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := plan.Validate(); err != nil {
				t.Fatalf("validate returned plan: %v", err)
			}
			if plan.Explicit || len(plan.Targets) != 5 || plan.Outcome.SelectedTargets != 5 ||
				plan.Outcome.SelectedFileRefs != 5 || plan.GoSource == nil || plan.PythonCatalog == nil {
				t.Fatalf("repository target plan = %#v", plan)
			}
			if providerCalls != 2 || readmeProvider.calls != 1 || portfolioProvider.calls != 1 {
				t.Fatalf(
					"model requests = factory %d, README %d, portfolio %d; want one of each",
					providerCalls, readmeProvider.calls, portfolioProvider.calls,
				)
			}
			if discoveryCalls != 1 {
				t.Fatalf("JavaScript/TypeScript discovery calls = %d, want 1", discoveryCalls)
			}
			defaultTarget, ok := plan.DefaultTarget()
			if !ok || defaultTarget.Key.Adapter != test.wantAdapter || plan.Default != defaultTarget.Key {
				t.Fatalf("typed default = %#v / %#v, want %q", plan.Default, defaultTarget, test.wantAdapter)
			}
			if test.wantAdapter == repositoryTargetAdapterJSTS &&
				(defaultTarget.JSTS == nil || defaultTarget.JSTS.Selector != project.Project.Selector) {
				t.Fatalf("JavaScript/TypeScript default scout target = %#v", defaultTarget.JSTS)
			}
			ordered, err := repositoryTargetExecutionOrder(plan)
			if err != nil {
				t.Fatalf("order repository targets: %v", err)
			}
			if len(ordered) != len(plan.Targets) || ordered[0].Key != plan.Default {
				t.Fatalf("repository execution order = %#v", ordered)
			}
			orderedKeys := make(map[repositoryTargetKey]int, len(ordered))
			for _, target := range ordered {
				orderedKeys[target.Key]++
			}
			for _, target := range plan.Targets {
				if orderedKeys[target.Key] != 1 {
					t.Fatalf("repository execution coverage for %s = %d, want 1", target.Key, orderedKeys[target.Key])
				}
			}

			adapterCounts := map[repositoryTargetAdapter]int{}
			for _, target := range plan.Targets {
				adapterCounts[target.Key.Adapter]++
				if target.Selector == "" || len(target.FileRefs) != 1 {
					t.Fatalf("typed target lost exact selector/file authority: %#v", target)
				}
				payloads := boolCount(target.Go != nil) + boolCount(target.Python != nil) + boolCount(target.JSTS != nil)
				if payloads != 1 {
					t.Fatalf("typed target payload count = %d: %#v", payloads, target)
				}
			}
			if adapterCounts[repositoryTargetAdapterGo] != 3 ||
				adapterCounts[repositoryTargetAdapterPython] != 1 ||
				adapterCounts[repositoryTargetAdapterJSTS] != 1 {
				t.Fatalf("adapter counts = %#v", adapterCounts)
			}
			for _, ref := range []corpus.FileID{
				goFileRef, goWorkerFileRef, goLibraryFileRef, pythonFileRef, manifestRef,
			} {
				if !strings.Contains(portfolioProvider.prompt.User, string(ref)) {
					t.Fatalf("shared portfolio prompt omitted file_ref %q: %s", ref, portfolioProvider.prompt.User)
				}
			}
		})
	}
}

func TestRepositoryDefaultMixedPlanDefersJSTSCompilerUntilDispatchPreflight(t *testing.T) {
	repository, goSource, project := repositoryTargetRuntimeInlineInputs(t)
	response, err := json.Marshal(map[string]any{
		"default_file_ref": repositoryTargetRuntimeFileRef(t, repository, "cmd/api/main.go"),
		"target_file_refs": []corpus.FileID{
			repositoryTargetRuntimeFileRef(t, repository, "cmd/api/main.go"),
			repositoryTargetRuntimeFileRef(t, repository, "cmd/worker/main.go"),
			repositoryTargetRuntimeFileRef(t, repository, "pkg/client/client.go"),
			repositoryTargetRuntimeFileRef(t, repository, "native/runtime.py"),
			repositoryTargetRuntimeFileRef(t, repository, "package.json"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	readmeProvider := &targetPortfolioClientStub{response: []byte(`[]`)}
	portfolioProvider := &targetPortfolioClientStub{response: response}
	providerCalls := 0
	scoutCalls := 0
	plan, err := selectRepositoryTargetPlanForRun(
		context.Background(),
		repositoryTargetRuntimeTestOptions(
			t, repository, &goSource, project, "", &providerCalls,
			&scoutCalls, readmeProvider, portfolioProvider,
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	if scoutCalls != 1 {
		t.Fatalf("mixed JSTS scout calls = %d, want 1", scoutCalls)
	}
	ordered, err := repositoryTargetExecutionOrder(plan)
	if err != nil {
		t.Fatal(err)
	}
	materializationCalls := 0
	materialized, err := materializeSelectedJSTSProjects(
		context.Background(),
		repositoryTargetDispatchOptions{
			Repo: t.TempDir(), Corpus: repository,
			DiscoverJSTSFn: func(
				context.Context, *corpus.Corpus, string, string,
			) (jstsproject.Result, error) {
				materializationCalls++
				return project.Snapshot(), nil
			},
		},
		ordered,
	)
	if err != nil {
		t.Fatal(err)
	}
	if materializationCalls != 1 || len(materialized) != 1 {
		t.Fatalf(
			"mixed JSTS materialization = %d calls / %d results, want one deferred call",
			materializationCalls, len(materialized),
		)
	}
}

func TestJSTSMaterializationProgressAndErrorsNameExactManifest(t *testing.T) {
	repository, _, project := repositoryTargetRuntimeInlineInputs(t)
	scout, err := jstsproject.TargetFromResult(project)
	if err != nil {
		t.Fatal(err)
	}
	ordered := []repositoryTypedTarget{{
		Key:      repositoryTargetKey{Adapter: repositoryTargetAdapterJSTS, Ref: scout.Ref},
		Selector: scout.Selector,
		JSTS:     &scout,
	}}
	for _, test := range []struct {
		name         string
		discoveryErr error
		wantGuidance bool
	}{
		{
			name:         "generic helper failure keeps its own diagnosis",
			discoveryErr: errors.New("jsts project: TypeScript helper output exceeds 67108864 bytes"),
		},
		{
			name:         "typed compiler prerequisite adds owner action",
			discoveryErr: fmt.Errorf("%w: typescript-api package is not installed", jstsproject.ErrTypeScriptCompilerUnavailable),
			wantGuidance: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var console bytes.Buffer
			_, err := materializeSelectedJSTSProjects(
				context.Background(),
				repositoryTargetDispatchOptions{
					Repo: t.TempDir(), Corpus: repository, Output: newRunOutput(&console),
					DiscoverJSTSFn: func(
						context.Context, *corpus.Corpus, string, string,
					) (jstsproject.Result, error) {
						return jstsproject.Result{}, test.discoveryErr
					},
				},
				ordered,
			)
			if err == nil || !strings.Contains(err.Error(), scout.Selector) ||
				!strings.Contains(err.Error(), "manifest "+scout.ManifestPath) {
				t.Fatalf("materialization error = %v", err)
			}
			if strings.Contains(err.Error(), "owner must prepare") != test.wantGuidance {
				t.Fatalf("owner guidance mismatch: %v", err)
			}
			output := console.String()
			for _, want := range []string{
				"JavaScript/TypeScript project materialization",
				"state: started",
				"target: " + scout.Name,
				"manifest: " + scout.ManifestPath,
				"selector: " + scout.Selector,
			} {
				if !strings.Contains(output, want) {
					t.Fatalf("materialization progress omitted %q:\n%s", want, output)
				}
			}
		})
	}
}

func TestJSTSMaterializationRebindsContentDerivedNameWithoutFreshnessGate(t *testing.T) {
	repository, _, project := repositoryTargetRuntimeInlineInputs(t)
	scout, err := jstsproject.TargetFromResult(project)
	if err != nil {
		t.Fatal(err)
	}
	scout.Name = "name-before-content-change"
	if err := validateJSTSTargetMaterialization(repository, scout, project); err != nil {
		t.Fatalf("validate materialization after name drift: %v", err)
	}
	target := repositoryTypedTarget{
		Key:      repositoryTargetKey{Adapter: repositoryTargetAdapterJSTS, Ref: scout.Ref},
		Selector: scout.Selector,
		JSTS:     &scout,
	}
	rebound, err := rebindMaterializedJSTSTarget(target, project)
	if err != nil {
		t.Fatal(err)
	}
	if rebound.JSTS == nil || rebound.JSTS.Name != project.Project.Name ||
		rebound.JSTS.Name == scout.Name || rebound.Key != target.Key {
		t.Fatalf("rebound JavaScript/TypeScript target = %#v", rebound)
	}
}

func TestCanonicalNativeTargetFileRefsUsesOneRepresentativePerExactTarget(t *testing.T) {
	candidates := []analysistarget.FileCandidate{
		{FileRef: "f1", Hypotheses: []string{"first library API file"}},
		{FileRef: "f2", Hypotheses: []string{"second library API file"}},
		{FileRef: "f3", Hypotheses: []string{"executable root"}},
	}
	resolved := map[corpus.FileID][]string{
		"f1": {"library"},
		"f2": {"library"},
		"f3": {"command"},
	}
	refs, err := canonicalNativeTargetFileRefs(
		"test",
		candidates,
		[]string{"library", "command"},
		func(fileRef corpus.FileID) ([]string, error) {
			return append([]string(nil), resolved[fileRef]...), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 || refs[0] != "f1" || refs[1] != "f3" {
		t.Fatalf("canonical target representatives = %v, want [f1 f3]", refs)
	}
}

func TestRepositoryTargetPlanPreservesResolverDerivedPythonModuleExecutionTarget(t *testing.T) {
	repository := pythonTargetCorpus(t, map[string]string{
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
	mainRef := repositoryTargetRuntimeFileRef(t, repository, "src/main.py")
	manifestRef := repositoryTargetRuntimeFileRef(t, repository, "pyproject.toml")
	catalog, err := pythontarget.Discover(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := pythontarget.NewFileTargetResolver(repository, catalog)
	if err != nil {
		t.Fatal(err)
	}
	want, err := resolver.ResolveOne(mainRef)
	if err != nil {
		t.Fatal(err)
	}
	if len(want.Roots) != 1 || want.Roots[0].Kind != pythontarget.RootModuleExecution {
		t.Fatalf("resolver control target = %#v, want one module-execution root", want)
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
	portfolioResponse, err := json.Marshal(map[string]any{
		"default_file_ref": mainRef,
		"target_file_refs": []corpus.FileID{manifestRef, mainRef},
	})
	if err != nil {
		t.Fatal(err)
	}
	readmeProvider := &targetPortfolioClientStub{response: readmeResponse}
	portfolioProvider := &targetPortfolioClientStub{response: portfolioResponse}
	providerCalls := 0
	plan, err := selectRepositoryTargetPlanForRun(context.Background(), repositoryTargetRuntimeOptions{
		RepoName: "web", Repository: repository, DiscoverPython: true,
		Executor: llm.Executor{Enabled: false},
		Providers: func() (llm.Provider, error) {
			providerCalls++
			if providerCalls == 1 {
				return readmeProvider, nil
			}
			if providerCalls == 2 {
				return portfolioProvider, nil
			}
			return nil, errors.New("unexpected additional model request")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("validate module-execution plan: %v", err)
	}
	if len(plan.Targets) != 2 || plan.Outcome.SelectedTargets != 2 ||
		plan.Outcome.SelectedFileRefs != 2 || plan.Default.Adapter != repositoryTargetAdapterPython {
		t.Fatalf("typed module-execution plan = %#v", plan)
	}
	var got repositoryTypedTarget
	for _, target := range plan.Targets {
		if target.Key.Ref == want.Ref {
			got = target
			break
		}
	}
	if got.Python == nil {
		t.Fatalf("typed module-execution target %q is absent from %#v", want.Ref, plan.Targets)
	}
	if got.Key.Ref != want.Ref || got.Selector != want.Selector || got.Python.IdentityRef != want.IdentityRef ||
		got.Python.ScopeRef != want.ScopeRef || len(got.Python.Roots) != 1 ||
		got.Python.Roots[0].Kind != pythontarget.RootModuleExecution ||
		len(got.FileRefs) != 1 || got.FileRefs[0] != mainRef {
		t.Fatalf("resolver-derived target lost exact authority: got %#v, want %#v", got, want)
	}
	if plan.PythonCatalog == nil || !plan.PythonCatalog.OwnsTarget(*got.Python) {
		t.Fatalf("resolver-derived target escaped retained catalog authority: %#v", got.Python)
	}
	if providerCalls != 2 || readmeProvider.calls != 1 || portfolioProvider.calls != 1 {
		t.Fatalf("model requests = factory %d, README %d, portfolio %d; want one of each", providerCalls, readmeProvider.calls, portfolioProvider.calls)
	}
}

func TestRepositoryTargetPlanExplicitExactSelectorsBypassPortfolio(t *testing.T) {
	repository, goSource, project := repositoryTargetRuntimeInlineInputs(t)
	pythonCatalog, err := pythontarget.Discover(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	if len(pythonCatalog.Entries) != 1 {
		t.Fatalf("inline Python targets = %d, want 1", len(pythonCatalog.Entries))
	}
	goEntry := targetPortfolioRuntimeEntry(t, *goSource.TargetCatalog, "cmd/api")

	for _, test := range []struct {
		name        string
		selector    string
		wantAdapter repositoryTargetAdapter
	}{
		{name: "Go", selector: goEntry.Candidate.Key, wantAdapter: repositoryTargetAdapterGo},
		{name: "Python", selector: pythonCatalog.Entries[0].Selector, wantAdapter: repositoryTargetAdapterPython},
		{name: "JavaScript TypeScript", selector: project.Project.Selector, wantAdapter: repositoryTargetAdapterJSTS},
	} {
		t.Run(test.name, func(t *testing.T) {
			readmeProvider := &targetPortfolioClientStub{response: []byte(`[]`)}
			providerCalls := 0
			discoveryCalls := 0
			options := repositoryTargetRuntimeTestOptions(
				t, repository, &goSource, project, test.selector, &providerCalls,
				&discoveryCalls, readmeProvider, nil,
			)
			plan, err := selectRepositoryTargetPlanForRun(context.Background(), options)
			if err != nil {
				t.Fatal(err)
			}
			if err := plan.Validate(); err != nil {
				t.Fatalf("validate explicit plan: %v", err)
			}
			if !plan.Explicit || len(plan.Targets) != 1 || plan.Default != plan.Targets[0].Key ||
				plan.Targets[0].Key.Adapter != test.wantAdapter || plan.Targets[0].Selector != test.selector {
				t.Fatalf("explicit typed plan = %#v", plan)
			}
			if len(plan.Targets[0].FileRefs) != 0 || plan.Outcome.SelectedFileRefs != 0 ||
				len(plan.Outcome.Request) != 0 || plan.Outcome.SemanticCalls != 0 {
				t.Fatalf("explicit selector acquired portfolio authority: %#v", plan)
			}
			if providerCalls != 1 || readmeProvider.calls != 1 {
				t.Fatalf("explicit model requests = factory %d, README %d", providerCalls, readmeProvider.calls)
			}
			if discoveryCalls != 1 {
				t.Fatalf("explicit JavaScript/TypeScript scout calls = %d, want 1", discoveryCalls)
			}
			if plan.GoSource == nil {
				t.Fatalf("explicit %s target lost active Go scout authority", test.wantAdapter)
			}
			if plan.PythonCatalog == nil {
				t.Fatalf("explicit %s target lost active Python scout authority", test.wantAdapter)
			}
			ordered, err := repositoryTargetExecutionOrder(plan)
			if err != nil {
				t.Fatal(err)
			}
			materializationCalls := 0
			materialized, err := materializeSelectedJSTSProjects(
				context.Background(),
				repositoryTargetDispatchOptions{
					Repo: t.TempDir(), Corpus: repository,
					DiscoverJSTSFn: func(
						context.Context, *corpus.Corpus, string, string,
					) (jstsproject.Result, error) {
						materializationCalls++
						return project.Snapshot(), nil
					},
				},
				ordered,
			)
			if err != nil {
				t.Fatal(err)
			}
			wantMaterializationCalls := 0
			if test.wantAdapter == repositoryTargetAdapterJSTS {
				wantMaterializationCalls = 1
			}
			if materializationCalls != wantMaterializationCalls ||
				len(materialized) != wantMaterializationCalls {
				t.Fatalf(
					"explicit %s JSTS materialization = %d calls / %d results, want %d",
					test.wantAdapter, materializationCalls, len(materialized), wantMaterializationCalls,
				)
			}
		})
	}
}

func TestRepositoryTargetDiscoveryPassesExactJSTSSelectorBeforeProjectDiscovery(t *testing.T) {
	repository := pythonTargetCorpus(t, map[string]string{
		"package.json": `{"name":"web"}`,
		"src/main.ts":  "export const start = true\n",
	})
	project := jsTSTestProject(t, repository, "typescript")
	discoveredSelector := "not-called"
	discovery, err := discoverRepositoryTargets(context.Background(), repositoryTargetRuntimeOptions{
		RepoName: "universal-runtime", Repository: repository,
		DiscoverJSTS: true, TargetOverride: project.Project.Selector,
		ScoutJSTSFn: func(
			_ context.Context,
			_ *corpus.Corpus,
			selector string,
		) (jstsproject.Target, error) {
			discoveredSelector = selector
			return jstsproject.TargetFromResult(project)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if discoveredSelector != project.Project.Selector || discovery.jstsTarget == nil ||
		discovery.jstsTarget.Selector != project.Project.Selector {
		t.Fatalf("early JavaScript/TypeScript selector = %q; discovery = %#v", discoveredSelector, discovery)
	}
}

func TestRepositoryTargetPlanRejectsSuppressionOfNativeGoTargets(t *testing.T) {
	repository, goSource, project := repositoryTargetRuntimeInlineInputs(t)
	libraryFileRef := repositoryTargetRuntimeFileRef(t, repository, "pkg/client/client.go")
	pythonFileRef := repositoryTargetRuntimeFileRef(t, repository, "native/runtime.py")
	manifestRef := repositoryTargetRuntimeFileRef(t, repository, "package.json")
	response, err := json.Marshal(map[string]any{
		"default_file_ref": pythonFileRef,
		"target_file_refs": []corpus.FileID{libraryFileRef, pythonFileRef, manifestRef},
	})
	if err != nil {
		t.Fatal(err)
	}
	readmeProvider := &targetPortfolioClientStub{response: []byte(`[]`)}
	portfolioProvider := &targetPortfolioClientStub{response: response}
	providerCalls := 0
	discoveryCalls := 0
	_, err = selectRepositoryTargetPlanForRun(
		context.Background(),
		repositoryTargetRuntimeTestOptions(
			t, repository, &goSource, project, "", &providerCalls,
			&discoveryCalls, readmeProvider, portfolioProvider,
		),
	)
	if err == nil || !strings.Contains(err.Error(), "omits exact required target authority") {
		t.Fatalf("mixed native-target suppression error = %v", err)
	}
	if providerCalls != 2 || readmeProvider.calls != 1 || portfolioProvider.calls != 1 {
		t.Fatalf("mixed authority model requests = %d / %d / %d", providerCalls, readmeProvider.calls, portfolioProvider.calls)
	}
}

func repositoryTargetRuntimeInlineInputs(
	t *testing.T,
) (*corpus.Corpus, snapshot.Snapshot, jstsproject.Result) {
	t.Helper()
	// This is deliberately an inline, run-local corpus rather than a new
	// repository fixture. The regression is selector/plan authority, not a new
	// language-adapter discovery invariant.
	repository := pythonTargetCorpus(t, map[string]string{
		"README.md":            "# Universal runtime\n\nGo API, Python runtime, and TypeScript client.\n",
		"go.mod":               "module example.com/repomap\n",
		"cmd/api/main.go":      "package main\nfunc main() {}\n",
		"cmd/worker/main.go":   "package main\nfunc main() {}\n",
		"pkg/client/client.go": "package client\nfunc NewClient() {}\n",
		"native/runtime.py": `def main():
	return 0

if __name__ == "__main__":
	main()
`,
		"package.json":  `{"name":"universal-runtime"}`,
		"tsconfig.json": `{}`,
		"src/main.ts":   "export const start = true\n",
	})
	facts := targetPortfolioRuntimeFacts()
	catalog := targetPortfolioRuntimeCatalog(t, facts)
	goSource := snapshot.Snapshot{
		RepoName: "example.com/repomap", DisplayName: "universal-runtime",
		GoFacts: &facts, TargetCatalog: &catalog,
		FilesConsidered: len(repository.Entries()), FilteredFiles: repository.VisiblePaths(),
	}
	project := jsTSTestProject(t, repository, "typescript")
	return repository, goSource, project
}

func repositoryTargetRuntimeTestOptions(
	t *testing.T,
	repository *corpus.Corpus,
	goSource *snapshot.Snapshot,
	project jstsproject.Result,
	override string,
	providerCalls *int,
	discoveryCalls *int,
	readmeProvider *targetPortfolioClientStub,
	portfolioProvider *targetPortfolioClientStub,
) repositoryTargetRuntimeOptions {
	t.Helper()
	jstsTarget, err := jstsproject.TargetFromResult(project)
	if err != nil {
		t.Fatal(err)
	}
	return repositoryTargetRuntimeOptions{
		RepoName: "example.com/repomap", Repository: repository,
		GoSnapshot: goSource, DiscoverPython: true, DiscoverJSTS: true,
		TargetOverride: override, Executor: llm.Executor{Enabled: false},
		Providers: func() (llm.Provider, error) {
			*providerCalls++
			switch *providerCalls {
			case 1:
				return readmeProvider, nil
			case 2:
				if portfolioProvider == nil {
					return nil, errors.New("TargetPortfolio must be bypassed for exact --target")
				}
				return portfolioProvider, nil
			default:
				return nil, errors.New("unexpected additional model request")
			}
		},
		ScoutJSTSFn: func(context.Context, *corpus.Corpus, string) (jstsproject.Target, error) {
			*discoveryCalls++
			return jstsTarget, nil
		},
	}
}

func repositoryTargetRuntimeFileRef(
	t *testing.T,
	repository *corpus.Corpus,
	filePath string,
) corpus.FileID {
	t.Helper()
	fileRef, ok := repository.ID(filePath)
	if !ok {
		t.Fatalf("inline corpus has no file %q", filePath)
	}
	return fileRef
}

// Keep compile-time coverage on the exact native Go payload type. This avoids
// accidentally replacing it with a display-only wrapper during integration.
var _ *analysistarget.Target = repositoryTypedTarget{}.Go
