package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/documentationreduce"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/orientation"
	"github.com/dvordrova/repomap/internal/programcategorization"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programpage"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
	"github.com/dvordrova/repomap/internal/targetoutcome"
)

func TestExactJSTSSelectorBypassesFailingGoPlanningPrerequisite(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve repository-target test source path")
	}
	repositoryRoot := filepath.Join(filepath.Dir(filename), "..", "..")
	tracked := []string{
		"testdata/repositories/go/go.mod",
		"testdata/repositories/jsts/package.json",
		"testdata/repositories/jsts/src/platform.ts",
		"testdata/repositories/jsts/src/server.ts",
		"testdata/repositories/jsts/tsconfig.json",
	}
	repository, err := corpus.New(
		t.Context(), repositoryRoot,
		gitfiles.Listing{Paths: tracked, RegularPaths: tracked},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })

	evidence := repositoryLanguages(repository)
	if !evidence.Go || !evidence.JavaScriptTypeScript {
		t.Fatalf("combined cumulative fixture evidence = %#v", evidence)
	}
	const selector = "jsts:testdata/repositories/jsts/package.json"
	target, err := jstsproject.ScoutSelected(t.Context(), repository, selector)
	if err != nil {
		t.Fatalf("cumulative JSTS fixture does not own %q: %v", selector, err)
	}
	goPreparationCalls := 0
	goSource, err := prepareRepositoryPlanningGoSource(
		evidence.Go,
		target.Selector,
		func() (snapshot.Snapshot, error) {
			goPreparationCalls++
			return snapshot.Snapshot{}, errors.New("unrelated Go facts are incomplete")
		},
	)
	if err != nil || goSource != nil || goPreparationCalls != 0 {
		t.Fatalf(
			"exact JSTS Go prerequisite = source %#v, calls %d, error %v",
			goSource, goPreparationCalls, err,
		)
	}

	providerCalls := 0
	plan, err := selectRepositoryTargetPlanForRun(t.Context(), repositoryTargetRuntimeOptions{
		RepoName: "repomap-cumulative-jsts-fixture", Repository: repository,
		DiscoverJSTS: evidence.JavaScriptTypeScript, TargetOverride: target.Selector,
		ScoutJSTSFn: jstsproject.ScoutTargets,
		Providers: func() (llm.Provider, error) {
			providerCalls++
			return nil, errors.New("exact JSTS selection must not call a provider")
		},
		Executor: llm.Executor{Enabled: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 0 || !plan.Explicit || len(plan.Targets) != 1 ||
		plan.Targets[0].Key.Adapter != repositoryTargetAdapterJSTS ||
		plan.Targets[0].Selector != selector {
		t.Fatalf("exact JSTS plan after skipped Go prerequisite = %#v; provider calls = %d", plan, providerCalls)
	}
	if _, ok := repositoryPlanGoSource(plan); ok {
		t.Fatal("exact JSTS plan unexpectedly retained Go source authority")
	}
}

func TestRepositoryPlanningGoSourcePreservesDefaultAndExactGoBehavior(t *testing.T) {
	for _, selector := range []string{"", "example.com/service@.::module_library"} {
		t.Run(selector, func(t *testing.T) {
			calls := 0
			_, err := prepareRepositoryPlanningGoSource(
				true,
				selector,
				func() (snapshot.Snapshot, error) {
					calls++
					return snapshot.Snapshot{}, errors.New("Go prerequisite failed")
				},
			)
			if err == nil || calls != 1 {
				t.Fatalf("Go planning prerequisite = calls %d, error %v", calls, err)
			}
		})
	}

	for _, selector := range []string{"jsts:package.json", "python:.:library:library"} {
		t.Run(selector, func(t *testing.T) {
			calls := 0
			source, err := prepareRepositoryPlanningGoSource(
				true,
				selector,
				func() (snapshot.Snapshot, error) {
					calls++
					return snapshot.Snapshot{}, errors.New("irrelevant Go prerequisite failed")
				},
			)
			if err != nil || source != nil || calls != 0 {
				t.Fatalf("non-Go planning prerequisite = source %#v, calls %d, error %v", source, calls, err)
			}
		})
	}
}

func TestRepositoryGoWorkspaceStateSharesOneUnionAcrossExactTargets(t *testing.T) {
	repositoryRoot, repository, selected := repositoryGoWorkspaceFixture(t, false)
	options := surfacediscovery.DefaultOptions(
		repositoryRoot, runtime.GOOS+"/"+runtime.GOARCH,
	)
	var state repositoryGoWorkspaceState
	first, err := state.analyze(t.Context(), options, selected[0], selected)
	if err != nil {
		t.Fatal(err)
	}
	workspace := state.workspace
	if workspace == nil || state.unionUnavailable {
		t.Fatalf("successful union state = workspace %p, unavailable %t", workspace, state.unionUnavailable)
	}
	second, err := state.analyze(t.Context(), options, selected[1], selected)
	if err != nil {
		t.Fatal(err)
	}
	if state.workspace != workspace {
		t.Fatal("second exact target replaced the shared prepared workspace")
	}
	if first.DirectCallIndex == nil || second.DirectCallIndex == nil ||
		first.DirectCallIndex.Scope.TargetRef != selected[0].AnalysisTarget.Ref ||
		second.DirectCallIndex.Scope.TargetRef != selected[1].AnalysisTarget.Ref {
		t.Fatalf("shared workspace target scopes = %#v / %#v", first.DirectCallIndex, second.DirectCallIndex)
	}
	if directCallIndexContainsPackage(*first.DirectCallIndex, "example.com/atomic-workspace/cmd/worker") ||
		directCallIndexContainsPackage(*second.DirectCallIndex, "example.com/atomic-workspace/cmd/app") {
		t.Fatal("shared workspace leaked one sibling target into the other's exact projection")
	}
	for index, scoped := range selected {
		if scoped.GoFacts == nil || scoped.GoFacts.Dependencies == nil || scoped.AnalysisTarget == nil {
			t.Fatalf("scoped Go target %d omitted dependency authority", index)
		}
		typed := repositoryTypedTarget{
			Key: repositoryTargetKey{
				Adapter: repositoryTargetAdapterGo,
				Ref:     scoped.AnalysisTarget.Ref,
			},
			native: scoped.AnalysisTarget.Snapshot(),
		}
		catalog, err := buildGoRepositoryDependencies(repositoryDependencyBuildRequest{
			Target: typed,
			Facts: goRepositoryProgramFacts{
				Target:       scoped.AnalysisTarget.Snapshot(),
				Dependencies: *scoped.GoFacts.Dependencies,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, importer := range catalog.Importers {
			for siblingIndex, siblingPath := range []string{
				"example.com/atomic-workspace/cmd/app",
				"example.com/atomic-workspace/cmd/worker",
			} {
				if siblingIndex != index && importer.PackagePath == siblingPath {
					t.Fatalf("target %d dependency catalog retained sibling importer %#v", index, importer)
				}
			}
		}
	}
	_ = repository
}

func TestRepositoryGoWorkspaceStateFallsBackExactlyAndStopsReusingFailedUnion(t *testing.T) {
	repositoryRoot, _, selected := repositoryGoWorkspaceFixture(t, true)
	options := surfacediscovery.DefaultOptions(
		repositoryRoot, runtime.GOOS+"/"+runtime.GOARCH,
	)
	var state repositoryGoWorkspaceState
	healthy, err := state.analyze(t.Context(), options, selected[0], selected)
	if err != nil {
		t.Fatalf("healthy exact fallback: %v", err)
	}
	if !state.unionUnavailable || state.workspace != nil {
		t.Fatalf("failed union remained reusable: workspace %p, unavailable %t", state.workspace, state.unionUnavailable)
	}
	if healthy.DirectCallIndex == nil ||
		healthy.DirectCallIndex.Scope.TargetRef != selected[0].AnalysisTarget.Ref ||
		directCallIndexContainsPackage(*healthy.DirectCallIndex, "example.com/atomic-workspace/cmd/worker") {
		t.Fatalf("healthy exact fallback leaked broken sibling: %#v", healthy.DirectCallIndex)
	}
	if _, err := state.analyze(t.Context(), options, selected[1], selected); err == nil {
		t.Fatal("later broken target reused healthy target-local fallback authority")
	}
	if state.workspace != nil || !state.unionUnavailable {
		t.Fatalf("broken sibling changed failed-union state: workspace %p, unavailable %t", state.workspace, state.unionUnavailable)
	}
}

func repositoryGoWorkspaceFixture(
	t *testing.T,
	brokenWorker bool,
) (string, *corpus.Corpus, []snapshot.Snapshot) {
	t.Helper()
	repositoryRoot := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/atomic-workspace\n\ngo 1.24\n",
		"cmd/app/main.go": `package main
import "example.com/atomic-workspace/internal/shared"
func main() { shared.FromApp() }
`,
		"cmd/worker/main.go": `package main
import "example.com/atomic-workspace/internal/shared"
func main() { shared.FromWorker() }
`,
		"internal/shared/shared.go": `package shared
func FromApp() { common() }
func FromWorker() { common() }
func common() {}
`,
	}
	if brokenWorker {
		files["cmd/worker/main.go"] = "package main\nfunc main() { missing() }\n"
	}
	paths := make([]string, 0, len(files))
	for filePath := range files {
		paths = append(paths, filePath)
	}
	sort.Strings(paths)
	for _, filePath := range paths {
		absolute := filepath.Join(repositoryRoot, filepath.FromSlash(filePath))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(files[filePath]), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	repository, err := corpus.New(t.Context(), repositoryRoot, gitfiles.Listing{
		Paths: paths, RegularPaths: paths,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	deferred, err := snapshot.BuildContext(t.Context(), snapshot.Options{
		RepoPath: repositoryRoot, RepositoryCorpus: repository,
		GoTarget: runtime.GOOS + "/" + runtime.GOARCH,
	})
	if err != nil {
		t.Fatal(err)
	}
	refs := make(map[string]string)
	if deferred.TargetCatalog != nil {
		for _, entry := range deferred.TargetCatalog.Entries {
			refs[entry.DisplayPath] = entry.Candidate.Target.Ref
		}
	}
	selected := make([]snapshot.Snapshot, 0, 2)
	for _, displayPath := range []string{"cmd/app", "cmd/worker"} {
		if refs[displayPath] == "" {
			t.Fatalf("Go workspace fixture target refs = %#v", refs)
		}
		scoped, scopeErr := snapshot.ScopeAnalysisTarget(deferred, refs[displayPath])
		if scopeErr != nil {
			t.Fatal(scopeErr)
		}
		selected = append(selected, scoped)
	}
	return repositoryRoot, repository, selected
}

func directCallIndexContainsPackage(index surfacediscovery.DirectCallIndex, packagePath string) bool {
	for _, node := range index.Nodes {
		if node.Package == packagePath {
			return true
		}
	}
	return false
}

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
			requireRepositoryPlanGuidance(t, plan)
			_, hasGoSource := repositoryPlanGoSource(plan)
			_, hasPythonCatalog := repositoryPlanPythonCatalog(plan)
			if plan.Explicit || len(plan.Targets) != 5 || plan.Outcome.SelectedTargets != 5 ||
				plan.Outcome.SelectedFileRefs != 5 || !hasGoSource || !hasPythonCatalog {
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
			if test.wantAdapter == repositoryTargetAdapterJSTS {
				jstsTarget, ok := repositoryJSTSTarget(defaultTarget)
				if !ok || jstsTarget.Selector != project.Project.Selector {
					t.Fatalf("JavaScript/TypeScript default scout target = %#v", defaultTarget)
				}
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
				if target.native == nil {
					t.Fatalf("typed target lost its opaque adapter handle: %#v", target)
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

func TestRepositoryTargetPlanRetainsEveryJSTSPackageByDefault(t *testing.T) {
	repository := pythonTargetCorpus(t, map[string]string{
		"package.json":          `{"private":true}`,
		"admin/package.json":    `{"name":"admin-app"}`,
		"admin/src/main.ts":     "export const admin = true\n",
		"frontend/package.json": `{"name":"frontend-app"}`,
		"frontend/src/main.tsx": "export const frontend = true\n",
		"tools/package.json":    `{"private":true,"scripts":{"dev":"bun run --cwd ../.. dev"}}`,
	})
	adminRef := repositoryTargetRuntimeFileRef(t, repository, "admin/package.json")
	frontendRef := repositoryTargetRuntimeFileRef(t, repository, "frontend/package.json")
	toolsRef := repositoryTargetRuntimeFileRef(t, repository, "tools/package.json")
	for _, test := range []struct {
		name                string
		defaultRef          corpus.FileID
		wantDefaultSelector string
	}{
		{name: "admin default", defaultRef: adminRef, wantDefaultSelector: "jsts:admin/package.json"},
		{name: "frontend default", defaultRef: frontendRef, wantDefaultSelector: "jsts:frontend/package.json"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response, err := json.Marshal(map[string]any{
				"default_file_ref": test.defaultRef,
				"target_file_refs": []corpus.FileID{adminRef, frontendRef},
			})
			if err != nil {
				t.Fatal(err)
			}
			provider := &targetPortfolioClientStub{response: response}
			plan, err := selectRepositoryTargetPlanForRun(context.Background(), repositoryTargetRuntimeOptions{
				RepoName: "multi-jsts", Repository: repository, DiscoverJSTS: true,
				ScoutJSTSFn: jstsproject.ScoutTargets,
				Providers: func() (llm.Provider, error) {
					return provider, nil
				},
				Executor: llm.Executor{Enabled: false},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := plan.Validate(); err != nil {
				t.Fatal(err)
			}
			if len(plan.Targets) != 2 || plan.Outcome.SelectedTargets != 2 ||
				plan.Outcome.SelectedFileRefs != 2 {
				t.Fatalf("multi-JSTS plan = %#v", plan)
			}
			wantSelectors := []string{"jsts:admin/package.json", "jsts:frontend/package.json"}
			for index, target := range plan.Targets {
				if _, ok := repositoryJSTSTarget(target); target.Key.Adapter != repositoryTargetAdapterJSTS || !ok ||
					target.Selector != wantSelectors[index] || len(target.FileRefs) != 1 {
					t.Fatalf("multi-JSTS target %d = %#v", index, target)
				}
			}
			defaultTarget, ok := plan.DefaultTarget()
			if !ok || defaultTarget.Selector != test.wantDefaultSelector {
				t.Fatalf("multi-JSTS default = %#v, want %q", defaultTarget, test.wantDefaultSelector)
			}
			if strings.Contains(provider.prompt.User, "tools/package.json") ||
				strings.Contains(provider.prompt.User, string(toolsRef)) {
				t.Fatalf("source-less tooling manifest leaked into target portfolio: %s", provider.prompt.User)
			}
		})
	}

	for _, omitted := range []corpus.FileID{adminRef, frontendRef} {
		t.Run("cannot omit "+string(omitted), func(t *testing.T) {
			kept := frontendRef
			if omitted == frontendRef {
				kept = adminRef
			}
			response, err := json.Marshal(map[string]any{
				"default_file_ref": kept,
				"target_file_refs": []corpus.FileID{kept},
			})
			if err != nil {
				t.Fatal(err)
			}
			provider := &targetPortfolioClientStub{response: response}
			_, err = selectRepositoryTargetPlanForRun(context.Background(), repositoryTargetRuntimeOptions{
				RepoName: "multi-jsts", Repository: repository, DiscoverJSTS: true,
				ScoutJSTSFn: jstsproject.ScoutTargets,
				Providers: func() (llm.Provider, error) {
					return provider, nil
				},
				Executor: llm.Executor{Enabled: false},
			})
			if err == nil || !strings.Contains(err.Error(), "omits exact required target authority") {
				t.Fatalf("omitted JSTS package error = %v", err)
			}
		})
	}
}

func TestRepositoryTargetPlanExactJSTSFromManyBypassesPortfolio(t *testing.T) {
	repository := pythonTargetCorpus(t, map[string]string{
		"admin/package.json":    `{"name":"admin-app"}`,
		"admin/src/main.ts":     "export const admin = true\n",
		"frontend/package.json": `{"name":"frontend-app"}`,
		"frontend/src/main.tsx": "export const frontend = true\n",
	})
	for _, selector := range []string{"jsts:admin/package.json", "jsts:frontend/package.json"} {
		t.Run(selector, func(t *testing.T) {
			providerCalls := 0
			plan, err := selectRepositoryTargetPlanForRun(context.Background(), repositoryTargetRuntimeOptions{
				RepoName: "multi-jsts", Repository: repository, DiscoverJSTS: true,
				TargetOverride: selector, ScoutJSTSFn: jstsproject.ScoutTargets,
				Providers: func() (llm.Provider, error) {
					providerCalls++
					return nil, errors.New("TargetPortfolio must be bypassed for exact --target")
				},
				Executor: llm.Executor{Enabled: false},
			})
			if err != nil {
				t.Fatal(err)
			}
			if !plan.Explicit || len(plan.Targets) != 1 || plan.Targets[0].Selector != selector ||
				plan.Default != plan.Targets[0].Key || len(plan.Targets[0].FileRefs) != 0 {
				t.Fatalf("exact multi-package plan = %#v", plan)
			}
			if providerCalls != 0 || plan.Outcome.SemanticCalls != 0 || len(plan.Outcome.Request) != 0 {
				t.Fatalf("exact selector invoked model portfolio: calls=%d outcome=%#v", providerCalls, plan.Outcome)
			}
		})
	}
}

func TestRepositoryTargetPlanKeepsPythonBesideEveryJSTSPackage(t *testing.T) {
	repository := pythonTargetCorpus(t, map[string]string{
		"pyproject.toml": `[project]
name = "service"
version = "0.1.0"
`,
		"service.py": `def main():
	return 0

if __name__ == "__main__":
	main()
`,
		"admin/package.json": `{"name":"admin-app"}`,
		"admin/src/main.ts":  "export const admin = true\n",
		"front/package.json": `{"name":"front-app"}`,
		"front/src/main.ts":  "export const front = true\n",
	})
	discoveryOptions := repositoryTargetRuntimeOptions{
		RepoName: "python-with-multi-jsts", Repository: repository,
		DiscoverPython: true, DiscoverJSTS: true, ScoutJSTSFn: jstsproject.ScoutTargets,
	}
	discovery, err := discoverRepositoryTargets(context.Background(), discoveryOptions)
	if err != nil {
		t.Fatal(err)
	}
	candidateSets := make([][]analysistarget.FileCandidate, 0, len(discovery.adapters))
	for _, adapter := range discovery.adapters {
		candidateSets = append(candidateSets, adapter.Candidates)
	}
	nativeCandidates, err := analysistarget.MergeFileCandidates(repository.Snapshot(), candidateSets...)
	if err != nil {
		t.Fatal(err)
	}
	requiredRefs, err := repositoryRequiredTargetFileRefs(discovery, nativeCandidates)
	if err != nil {
		t.Fatal(err)
	}
	frontRef := repositoryTargetRuntimeFileRef(t, repository, "front/package.json")
	response, err := json.Marshal(map[string]any{
		"default_file_ref": frontRef,
		"target_file_refs": requiredRefs,
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := &targetPortfolioClientStub{response: response}
	options := discoveryOptions
	options.Providers = func() (llm.Provider, error) { return provider, nil }
	options.Executor = llm.Executor{Enabled: false}
	plan, err := selectRepositoryTargetPlanForRun(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[repositoryTargetAdapter]int{}
	for _, target := range plan.Targets {
		counts[target.Key.Adapter]++
	}
	pythonDiscovery := discovery.byKey[repositoryTargetAdapterPython]
	pythonCatalog, ok := pythonDiscovery.Authority.(pythontarget.Catalog)
	if !ok {
		t.Fatal("Python adapter discovery lost its exact catalog authority")
	}
	if counts[repositoryTargetAdapterJSTS] != 2 ||
		counts[repositoryTargetAdapterPython] != len(pythonCatalog.Entries) ||
		plan.Outcome.SelectedFileRefs != len(requiredRefs) {
		t.Fatalf("Python plus multi-JSTS plan = %#v; adapter counts = %#v", plan, counts)
	}
	defaultTarget, ok := plan.DefaultTarget()
	if !ok || defaultTarget.Selector != "jsts:front/package.json" {
		t.Fatalf("Python plus multi-JSTS default = %#v", defaultTarget)
	}

	pythonSelector := pythonCatalog.Entries[0].Selector
	providerCalls := 0
	options.TargetOverride = pythonSelector
	options.Providers = func() (llm.Provider, error) {
		providerCalls++
		return nil, errors.New("TargetPortfolio must be bypassed for exact Python --target")
	}
	exact, err := selectRepositoryTargetPlanForRun(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 0 || !exact.Explicit || len(exact.Targets) != 1 ||
		exact.Targets[0].Key.Adapter != repositoryTargetAdapterPython ||
		exact.Targets[0].Selector != pythonSelector {
		t.Fatalf("exact Python plan beside multi-JSTS = %#v; provider calls = %d", exact, providerCalls)
	}
}

func TestRepositoryDefaultMixedPlanDefersJSTSCompilerUntilTargetDispatch(t *testing.T) {
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
	planned, err := newJSTSRepositoryTypedTarget(scout)
	if err != nil {
		t.Fatal(err)
	}
	ordered := []repositoryTypedTarget{planned}
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
	target, err := newJSTSRepositoryTypedTarget(scout)
	if err != nil {
		t.Fatal(err)
	}
	rebound, err := rebindMaterializedJSTSTarget(target, project)
	if err != nil {
		t.Fatal(err)
	}
	reboundTarget, ok := repositoryJSTSTarget(rebound)
	if !ok || reboundTarget.Name != project.Project.Name ||
		reboundTarget.Name == scout.Name || rebound.Key != target.Key {
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
	pythonTarget, ok := repositoryPythonTarget(got)
	if !ok {
		t.Fatalf("typed module-execution target %q is absent from %#v", want.Ref, plan.Targets)
	}
	if got.Key.Ref != want.Ref || got.Selector != want.Selector || pythonTarget.IdentityRef != want.IdentityRef ||
		pythonTarget.ScopeRef != want.ScopeRef || len(pythonTarget.Roots) != 1 ||
		pythonTarget.Roots[0].Kind != pythontarget.RootModuleExecution ||
		len(got.FileRefs) != 1 || got.FileRefs[0] != mainRef {
		t.Fatalf("resolver-derived target lost exact authority: got %#v, want %#v", got, want)
	}
	pythonCatalog, hasPythonCatalog := repositoryPlanPythonCatalog(plan)
	if !hasPythonCatalog || !pythonCatalog.OwnsTarget(pythonTarget) {
		t.Fatalf("resolver-derived target escaped retained catalog authority: %#v", pythonTarget)
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
			requireRepositoryPlanGuidance(t, plan)
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
			if _, ok := repositoryPlanGoSource(plan); !ok {
				t.Fatalf("explicit %s target lost active Go scout authority", test.wantAdapter)
			}
			if _, ok := repositoryPlanPythonCatalog(plan); !ok {
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

func TestRepositoryTargetPlanAcceptsExactPythonNativeAnchorPath(t *testing.T) {
	repository := pythonTargetCorpus(t, map[string]string{
		"pyproject.toml": `[project]
name = "kongctl"
version = "0.1.0"
`,
		"kongctl/__init__.py": "",
		"kongctl/__main__.py": `def main():
    return 0

if __name__ == "__main__":
    raise SystemExit(main())
`,
	})
	anchorRef := repositoryTargetRuntimeFileRef(t, repository, "kongctl/__main__.py")
	catalog, err := pythontarget.Discover(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := pythontarget.NewFileTargetResolver(repository, catalog)
	if err != nil {
		t.Fatal(err)
	}
	resolverChoices, _, err := resolver.ModuleExecutionChoices(100)
	if err != nil {
		t.Fatal(err)
	}
	resolverCandidate := false
	for _, choice := range resolverChoices {
		if choice.Path == "kongctl/__main__.py" {
			resolverCandidate = true
			break
		}
	}
	if !resolverCandidate {
		t.Fatal("fixture has no competing resolver-only module-execution candidate")
	}

	plan, err := selectRepositoryTargetPlanForRun(context.Background(), repositoryTargetRuntimeOptions{
		RepoName: "kongctl", Repository: repository, DiscoverPython: true,
		TargetOverride: "kongctl/__main__.py", Executor: llm.Executor{Enabled: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("validate native-anchor plan: %v", err)
	}
	selectedPython, selectedPythonOK := repositoryPythonTarget(plan.Targets[0])
	if !plan.Explicit || len(plan.Targets) != 1 || !selectedPythonOK {
		t.Fatalf("native-anchor plan = %#v", plan)
	}
	selected := plan.Targets[0]
	if selected.Selector != "python:.:module:kongctl" || selectedPython.Selector != selected.Selector ||
		selectedPython.AnchorFileRef != anchorRef || selectedPython.ScopeRef != "" ||
		(len(selectedPython.Roots) == 1 && selectedPython.Roots[0].Kind == pythontarget.RootModuleExecution) {
		t.Fatalf("native anchor did not retain exact catalog target authority: %#v", selected)
	}
	if len(selected.FileRefs) != 0 || plan.Outcome.SelectedFileRefs != 0 || plan.Outcome.SemanticCalls != 0 {
		t.Fatalf("native path alias acquired portfolio authority: %#v", plan)
	}
}

func TestRepositoryTargetPlanRejectsAmbiguousPythonNativeAnchorPath(t *testing.T) {
	repository := pythonTargetCorpus(t, map[string]string{
		"setup.py": `from setuptools import setup
setup(
    name="kongctl",
    packages=["kongctl"],
    entry_points={"console_scripts": ["kongctl = kongctl.__main__:main"]},
)
`,
		"kongctl/__init__.py": "",
		"kongctl/__main__.py": `def main():
    return 0

if __name__ == "__main__":
    raise SystemExit(main())
`,
	})

	_, err := selectRepositoryTargetPlanForRun(context.Background(), repositoryTargetRuntimeOptions{
		RepoName: "kongctl", Repository: repository, DiscoverPython: true,
		TargetOverride: "kongctl/__main__.py", Executor: llm.Executor{Enabled: false},
	})
	if err == nil || !strings.Contains(err.Error(), "matches more than one exact repository target") {
		t.Fatalf("ambiguous native anchor error = %v", err)
	}
	matchedClause, _, _ := strings.Cut(err.Error(), "; exact --target choices:")
	if !strings.Contains(matchedClause, "matching exact selectors: python:.:module:kongctl, python:.:script:kongctl") ||
		strings.Contains(matchedClause, "python:module-execution:") {
		t.Fatalf("ambiguous native anchor match clause = %q", matchedClause)
	}
}

func TestRepositoryTargetPlanRetainsSharedPythonDefaultViews(t *testing.T) {
	repository := pythonTargetCorpus(t, map[string]string{
		"setup.py": `from setuptools import setup
setup(
    name="kongctl",
    packages=["kongctl"],
    entry_points={"console_scripts": ["kongctl = kongctl.__main__:main"]},
)
`,
		"kongctl/__init__.py": "",
		"kongctl/__main__.py": `def main():
    return 0

if __name__ == "__main__":
    raise SystemExit(main())
`,
	})
	setupRef := repositoryTargetRuntimeFileRef(t, repository, "setup.py")
	mainRef := repositoryTargetRuntimeFileRef(t, repository, "kongctl/__main__.py")
	response, err := json.Marshal(map[string]any{
		"default_file_ref": mainRef,
		"target_file_refs": []corpus.FileID{setupRef, mainRef},
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := &targetPortfolioClientStub{response: response}
	providerCalls := 0
	plan, err := selectRepositoryTargetPlanForRun(context.Background(), repositoryTargetRuntimeOptions{
		RepoName: "kongctl", Repository: repository, DiscoverPython: true,
		Providers: func() (llm.Provider, error) {
			providerCalls++
			return provider, nil
		},
		Executor: llm.Executor{Enabled: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("validate shared-default plan: %v", err)
	}
	if plan.Explicit || providerCalls != 1 || provider.calls != 1 {
		t.Fatalf(
			"shared-default orchestration = explicit %t, provider factory %d, calls %d",
			plan.Explicit, providerCalls, provider.calls,
		)
	}
	wantShared := map[string]bool{
		"python:.:module:kongctl": false,
		"python:.:script:kongctl": false,
	}
	for _, target := range plan.Targets {
		if _, wanted := wantShared[target.Selector]; !wanted {
			continue
		}
		if _, ok := repositoryPythonTarget(target); !ok || len(target.FileRefs) != 1 || target.FileRefs[0] != mainRef {
			t.Fatalf("shared representative authority = %#v", target)
		}
		wantShared[target.Selector] = true
	}
	for selector, found := range wantShared {
		if !found {
			t.Fatalf("shared default omitted exact target %q: %#v", selector, plan.Targets)
		}
	}
	defaultTarget, ok := plan.DefaultTarget()
	if !ok || defaultTarget.Selector != "python:.:module:kongctl" ||
		plan.Outcome.SelectedRef != defaultTarget.Key.String() ||
		plan.Outcome.SelectedTargets != len(plan.Targets) || plan.Outcome.SelectedFileRefs != 2 {
		t.Fatalf("shared landing-page default = %#v; outcome = %#v", defaultTarget, plan.Outcome)
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
		) ([]jstsproject.Target, error) {
			discoveredSelector = selector
			target, targetErr := jstsproject.TargetFromResult(project)
			return []jstsproject.Target{target}, targetErr
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	jstsDiscovery, ok := discovery.byKey[repositoryTargetAdapterJSTS]
	if !ok || discoveredSelector != project.Project.Selector || len(jstsDiscovery.Candidates) != 1 {
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
		ScoutJSTSFn: func(context.Context, *corpus.Corpus, string) ([]jstsproject.Target, error) {
			*discoveryCalls++
			return []jstsproject.Target{jstsTarget}, nil
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

func requireRepositoryPlanGuidance(t *testing.T, plan repositoryTargetPlan) {
	t.Helper()
	if err := plan.guidance.Validate(); err != nil {
		t.Fatalf("validate repository plan guidance: %v", err)
	}
	if len(plan.guidance.Documents) != 1 || plan.guidance.Documents[0].Path != "README.md" ||
		plan.guidance.Documents[0].Kind != readmetargetscout.GuidanceReadme ||
		plan.guidance.Documents[0].Content != "# Universal runtime\n\nGo API, Python runtime, and TypeScript client.\n" {
		t.Fatalf("repository plan guidance = %#v", plan.guidance)
	}
	owned, err := plan.guidance.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	owned.Documents[0].Content = "mutated"
	if plan.guidance.Documents[0].Content != "# Universal runtime\n\nGo API, Python runtime, and TypeScript client.\n" {
		t.Fatal("repository plan guidance shares storage with returned snapshot")
	}
}

func TestRepositoryTargetDispatcherPublishesMatchedGraphForCumulativePythonAndJSTS(t *testing.T) {
	repositoryRoot := ordinaryGraphCumulativeRepository(t)
	repository, err := corpus.Open(t.Context(), repositoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	repositoryState, err := freshness.CaptureRepository(t.Context(), repositoryRoot, repository)
	if err != nil {
		t.Fatal(err)
	}

	pythonCatalog, err := pythontarget.Discover(t.Context(), repository)
	if err != nil {
		t.Fatal(err)
	}
	var pythonTarget pythontarget.Target
	for _, target := range pythonCatalog.Entries {
		if target.Selector == "python:.:script:repomap-fixture" {
			pythonTarget = target
			break
		}
	}
	if pythonTarget.Selector == "" {
		t.Fatalf("cumulative Python fixture target is absent: %#v", pythonCatalog.Entries)
	}
	pythonPlanned, err := newPythonRepositoryTypedTarget(pythonTarget)
	if err != nil {
		t.Fatal(err)
	}

	// The ordinary dispatcher still owns the selected-project boundary. This
	// fixture supplies a sealed compiler result so the regression exercises the
	// real JSTS ProgramIndex projection without requiring a machine-local npm
	// installation merely to test command orchestration.
	jstsProject := jsTSTestProjectAt(
		t, repository, "typescript", "package.json", "src/server.ts",
	)
	jstsTarget, err := jstsproject.TargetFromResult(jstsProject)
	if err != nil {
		t.Fatal(err)
	}
	jstsPlanned, err := newJSTSRepositoryTypedTarget(jstsTarget)
	if err != nil {
		t.Fatal(err)
	}

	targets := []repositoryTypedTarget{pythonPlanned, jstsPlanned}
	sort.Slice(targets, func(i, j int) bool {
		return repositoryTypedTargetLess(targets[i], targets[j])
	})
	plan := repositoryTargetPlan{
		Targets: targets,
		Default: pythonPlanned.Key,
		Authorities: map[repositoryTargetAdapter]any{
			repositoryTargetAdapterPython: pythonCatalog.Snapshot(),
		},
		Outcome: targetPortfolioRunOutcome{
			SelectedRef:        pythonPlanned.Key.String(),
			SelectedTargets:    len(targets),
			SelectedTargetRefs: repositoryTargetRefs(targets),
		},
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("validate cumulative fixture plan: %v", err)
	}

	provider := &ordinaryGraphNoNetworkProvider{}
	providerFactoryCalls := 0
	documentationCalls := 0
	categorizationLanguages := make(map[string]int)
	groupingLanguages := make(map[string]int)
	matchingCalls := 0
	deps := defaultRunDeps{
		ctx:                 t.Context(),
		stdout:              &bytes.Buffer{},
		stderr:              &bytes.Buffer{},
		llmBatchConcurrency: 2,
		llmBatchController:  &llm.BatchController{},
		newCubeProvider: func() (llm.Provider, error) {
			providerFactoryCalls++
			return provider, nil
		},
		runDocumentationReduce: func(
			ctx context.Context,
			executor llm.Executor,
			provider llm.Provider,
			guidance readmetargetscout.GuidanceSnapshot,
		) (documentationreduce.Result, error) {
			documentationCalls++
			if executor.Enabled || provider != nil || len(guidance.Documents) != 0 {
				return documentationreduce.Result{}, fmt.Errorf(
					"unexpected documentation preset authority: executor=%#v provider=%#v guidance=%#v",
					executor, provider, guidance,
				)
			}
			return documentationreduce.Run(ctx, executor, provider, guidance)
		},
		runProgramCategorization: func(
			_ context.Context,
			executor llm.Executor,
			actualProvider llm.Provider,
			base programindex.Index,
			documentation documentationreduce.Result,
		) (programcategorization.Result, error) {
			if executor.Enabled || actualProvider != provider || len(base.Objects) == 0 {
				return programcategorization.Result{}, fmt.Errorf(
					"unexpected categorization preset authority for %s", base.Target.ID,
				)
			}
			categorizationLanguages[base.Target.Language]++
			return programcategorization.Result{
				ProgramTargetID:            base.Target.ID,
				BaseProgramIndexSHA256:     base.SHA256,
				ReducedDocumentationSHA256: documentation.ReductionSHA256,
				Assignments: []programcategorization.Assignment{{
					SubjectID:  base.Objects[0].ID,
					Categories: []programcategorization.Category{programcategorization.CategoryCore},
				}},
				Diagnostics: []programcategorization.Diagnostic{},
			}, nil
		},
		runProgramGrouping: func(
			_ context.Context,
			executor llm.Executor,
			actualProvider llm.Provider,
			program programindex.Index,
		) (groupindex.Index, []groupindex.Diagnostic, error) {
			if executor.Enabled || actualProvider != provider || program.Categorization == nil ||
				len(program.Categorization.Assignments) != 1 {
				return groupindex.Index{}, nil, fmt.Errorf(
					"unexpected grouping preset authority for %s", program.Target.ID,
				)
			}
			groupingLanguages[program.Target.Language]++
			subjectID := program.Categorization.Assignments[0].SubjectID
			return groupindex.Build(program, groupindex.Proposals{Groups: []groupindex.GroupProposal{{
				Key: "core", Title: program.Target.Name + " core",
				Summary: "Owns the selected target's core behavior.", Lane: groupindex.LaneCore,
				MemberSubjectIDs: []string{subjectID}, EvidenceSubjectIDs: []string{subjectID},
			}}})
		},
		runOrientation: func(
			_ context.Context,
			executor llm.Executor,
			_ llm.Provider,
			input orientation.Input,
		) (orientation.Result, []orientation.RejectedRow, error) {
			if executor.Enabled {
				return orientation.Result{}, nil, fmt.Errorf("orientation reused the live cache")
			}
			result, sealErr := orientation.Empty(
				input.Facts.SHA256, input.Claims.SHA256, orientationGroupDigests(input.Groups), 0,
			)
			return result, nil, sealErr
		},
		runGroupMatching: func(
			_ context.Context,
			executor llm.Executor,
			actualProvider llm.Provider,
			indexes []groupindex.Index,
		) ([]groupindex.Index, []groupindex.Diagnostic, error) {
			matchingCalls++
			if executor.Enabled || actualProvider != provider || len(indexes) != 2 ||
				len(indexes[0].Groups) != 1 || len(indexes[1].Groups) != 1 {
				return nil, nil, fmt.Errorf("matching did not receive the complete GroupsIndex set")
			}
			matched, diagnostics, matchErr := groupindex.WithConnections(
				indexes,
				[]groupindex.ConnectionInput{{
					From: groupindex.Endpoint{
						TargetID: indexes[0].Target.ID, GroupID: indexes[0].Groups[0].ID,
					},
					To: groupindex.Endpoint{
						TargetID: indexes[1].Target.ID, GroupID: indexes[1].Groups[0].ID,
					},
					SemanticKind: "coordinates_with", Label: "Cross-target fixture link",
					Summary:           "Connects the Python and TypeScript fixture responsibilities.",
					SupportResolution: programindex.PatternValueExact,
					Evidence: []groupindex.SubjectEndpoint{{
						TargetID:  indexes[0].Target.ID,
						SubjectID: indexes[0].Groups[0].EvidenceSubjectIDs[0],
					}},
				}},
			)
			if matchErr != nil || len(diagnostics) != 0 {
				return nil, diagnostics, errors.Join(matchErr, fmt.Errorf("matching diagnostics: %#v", diagnostics))
			}
			return matched, nil, nil
		},
	}

	debugDir := t.TempDir()
	var verifiedRuns []report.VerifiedRunReceipt
	var console bytes.Buffer
	assessment, reportPath, err := dispatchRepositoryTargetPlan(
		t.Context(),
		repositoryTargetDispatchOptions{
			Repo: repositoryRoot,
			ExtraArgs: []string{
				"--debug-dir", debugDir, "--no-cache", "--no-open",
			},
			Deps: deps, Corpus: repository, RepositoryState: repositoryState, Plan: plan,
			RunID: "ordinary-graph", DebugDir: debugDir, NoCache: true, NoOpen: true,
			Output: newRunOutput(&console),
			DiscoverJSTSFn: func(
				_ context.Context, _ *corpus.Corpus, repo, selector string,
			) (jstsproject.Result, error) {
				if repo != repositoryRoot || selector != jstsProject.Project.Selector {
					return jstsproject.Result{}, fmt.Errorf(
						"unexpected selected JSTS materialization %q in %q", selector, repo,
					)
				}
				return jstsProject.Snapshot(), nil
			},
			VerifiedRunsSink: func(receipts []report.VerifiedRunReceipt) {
				verifiedRuns = append([]report.VerifiedRunReceipt(nil), receipts...)
			},
		},
	)
	if err != nil {
		t.Fatalf("ordinary cumulative graph dispatch: %v\n%s", err, console.String())
	}
	if assessment.Status != report.PublicationReady {
		t.Fatalf("ordinary cumulative graph publication = %#v", assessment)
	}
	if documentationCalls != 1 || categorizationLanguages["python"] != 1 ||
		categorizationLanguages["typescript"] != 1 || groupingLanguages["python"] != 1 ||
		groupingLanguages["typescript"] != 1 || matchingCalls != 1 ||
		len(verifiedRuns) != 2 {
		t.Fatalf(
			"ordinary graph calls = docs %d, categorization %#v, grouping %#v, matching %d, verified %d",
			documentationCalls, categorizationLanguages, groupingLanguages, matchingCalls,
			len(verifiedRuns),
		)
	}
	if providerFactoryCalls == 0 || provider.callCount() != 0 {
		t.Fatalf(
			"request-bound local presets = provider factories %d, transport calls %d",
			providerFactoryCalls, provider.callCount(),
		)
	}
	if _, err := os.Stat(reportPath); err != nil {
		t.Fatalf("owner report %q: %v", reportPath, err)
	}

	runDirs := ordinaryGraphRunDirs(t, debugDir)
	if len(runDirs) != 2 {
		t.Fatalf("ordinary graph run directories = %v", runDirs)
	}
	htmlCount := 0
	localConnectionCount := 0
	for _, runDir := range runDirs {
		if _, err := os.Stat(filepath.Join(runDir, "report.html")); err == nil {
			htmlCount++
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		documentation, err := documentationreduce.Read(runDir)
		if err != nil {
			t.Fatal(err)
		}
		setRaw, err := os.ReadFile(filepath.Join(runDir, programindex.ArtifactSetFilename))
		if err != nil {
			t.Fatal(err)
		}
		set, err := programindex.DecodeArtifactSet(setRaw)
		if err != nil || len(set.Entries) != 1 {
			t.Fatalf("ProgramIndex set in %s = %#v, %v", runDir, set, err)
		}
		if set.Entries[0].Filename != programindex.ArtifactFilename {
			t.Fatalf(
				"page-local ProgramIndex filename in %s = %q, want %q",
				runDir, set.Entries[0].Filename, programindex.ArtifactFilename,
			)
		}
		indexRaw, err := os.ReadFile(filepath.Join(runDir, set.Entries[0].Filename))
		if err != nil {
			t.Fatal(err)
		}
		index, err := programindex.Decode(indexRaw)
		if err != nil {
			t.Fatal(err)
		}
		if index.Categorization == nil ||
			index.Categorization.ReducedDocumentationSHA256 != documentation.ReductionSHA256 ||
			len(index.Categorization.Assignments) != 1 {
			t.Fatalf("persisted ProgramIndex is not the enriched authority: %#v", index.Categorization)
		}
		groups, err := groupindex.Read(runDir)
		if err != nil {
			t.Fatal(err)
		}
		if groups.ProgramIndexSHA256 != index.SHA256 || len(groups.Groups) != 1 {
			t.Fatalf("persisted GroupsIndex does not bind ProgramIndex: %#v", groups)
		}
		localConnectionCount += len(groups.Connections)

		dependencyRaw, err := os.ReadFile(filepath.Join(runDir, dependencies.ArtifactFilename))
		if err != nil {
			t.Fatalf("read common dependency catalog in %s: %v", runDir, err)
		}
		dependencyCatalog, err := dependencies.Decode(dependencyRaw)
		if err != nil {
			t.Fatalf("decode common dependency catalog in %s: %v", runDir, err)
		}
		if dependencyCatalog.Coverage.State != dependencies.CoverageComplete {
			t.Fatalf(
				"common dependency catalog coverage in %s = %#v",
				runDir, dependencyCatalog.Coverage,
			)
		}
		for _, legacyArtifact := range []string{
			"declared-dependencies.json",
			"jsts-project.json",
			"program-index-jsts.json",
			"python-target-catalog.json",
		} {
			if _, err := os.Lstat(filepath.Join(runDir, legacyArtifact)); err == nil {
				t.Fatalf("legacy adapter artifact %q was published in %s", legacyArtifact, runDir)
			} else if !os.IsNotExist(err) {
				t.Fatalf("inspect legacy adapter artifact %q in %s: %v", legacyArtifact, runDir, err)
			}
		}

		manifest, err := report.ReadRunManifest(runDir)
		if err != nil {
			t.Fatal(err)
		}
		documentationRaw, err := os.ReadFile(filepath.Join(runDir, documentationreduce.ArtifactFilename))
		if err != nil {
			t.Fatal(err)
		}
		groupsRaw, err := os.ReadFile(filepath.Join(runDir, groupindex.ArtifactFilename))
		if err != nil {
			t.Fatal(err)
		}
		if manifest.MaterialInputs.ProgramIndexSetSHA256 != ordinaryGraphSHA256(setRaw) ||
			manifest.MaterialInputs.ReducedDocumentationSHA256 != ordinaryGraphSHA256(documentationRaw) ||
			manifest.MaterialInputs.GroupsIndexSHA256 != ordinaryGraphSHA256(groupsRaw) {
			t.Fatalf("manifest did not bind graph artifacts in %s: %#v", runDir, manifest.MaterialInputs)
		}
		reportRaw, err := os.ReadFile(filepath.Join(runDir, "report.json"))
		if err != nil {
			t.Fatal(err)
		}
		var data report.ReportData
		if err := json.Unmarshal(reportRaw, &data); err != nil {
			t.Fatalf("decode final report in %s: %v", runDir, err)
		}
		if data.GroupGraph == nil || len(data.GroupGraph.Indexes) != 2 {
			t.Fatalf("final report omitted complete matched graph set in %s: %#v", runDir, data.GroupGraph)
		}
		if err := data.GroupGraph.Validate(); err != nil {
			t.Fatalf("validate final report graph in %s: %v", runDir, err)
		}
		publishedConnections := 0
		for _, graphIndex := range data.GroupGraph.Indexes {
			publishedConnections += len(graphIndex.Connections)
		}
		if publishedConnections != 1 {
			t.Fatalf("final report graph in %s has %d cross-target connections, want 1", runDir, publishedConnections)
		}
	}
	if htmlCount != 1 || localConnectionCount != 1 {
		t.Fatalf("ordinary graph publication = physical HTML %d, cross-target connections %d", htmlCount, localConnectionCount)
	}
}

func TestRepositoryTargetDispatcherRoutesGoThroughCommonPageAndSemanticPipeline(t *testing.T) {
	repositoryRoot := ordinaryGraphGoRepository(t)
	repository, err := corpus.Open(t.Context(), repositoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	repositoryState, err := freshness.CaptureRepository(t.Context(), repositoryRoot, repository)
	if err != nil {
		t.Fatal(err)
	}

	goTarget := runtime.GOOS + "/" + runtime.GOARCH
	source, err := snapshot.BuildContext(t.Context(), snapshot.Options{
		RepoPath: repositoryRoot, RepositoryCorpus: repository, GoTarget: goTarget,
	})
	if err != nil {
		t.Fatal(err)
	}
	if source.TargetCatalog == nil {
		t.Fatal("Go fixture target catalog is absent")
	}
	var selected analysistarget.Target
	selector := ""
	for _, entry := range source.TargetCatalog.Entries {
		if entry.Candidate.Target.Kind == analysistarget.KindExecutablePackage &&
			entry.DisplayPath == "cmd/app" {
			selected = entry.Candidate.Target.Snapshot()
			selector = entry.Candidate.Key
			break
		}
	}
	if selector == "" {
		t.Fatalf("Go fixture executable target is absent: %#v", source.TargetCatalog.Entries)
	}
	planned, err := newGoRepositoryTypedTarget(selected, selector)
	if err != nil {
		t.Fatal(err)
	}
	plan := repositoryTargetPlan{
		Targets: []repositoryTypedTarget{planned}, Default: planned.Key,
		Authorities: map[repositoryTargetAdapter]any{
			repositoryTargetAdapterGo: source,
		},
		Outcome: targetPortfolioRunOutcome{
			SelectedRef: planned.Key.String(), SelectedTargets: 1,
			SelectedTargetRefs: []string{planned.Key.String()},
		},
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("validate Go common-seam plan: %v", err)
	}

	semantic := newOrdinaryGraphSemanticPreset(0)
	debugDir := t.TempDir()
	var console bytes.Buffer
	assessment, reportPath, err := dispatchRepositoryTargetPlan(
		t.Context(),
		repositoryTargetDispatchOptions{
			Repo: repositoryRoot,
			ExtraArgs: []string{
				"--debug-dir", debugDir, "--no-cache", "--no-open",
			},
			Deps: semantic.deps(t.Context()), GoTarget: goTarget,
			Corpus: repository, RepositoryState: repositoryState, Plan: plan,
			RunID: "go-common-page-seam", DebugDir: debugDir, NoCache: true, NoOpen: true,
			Output: newRunOutput(&console),
		},
	)
	if err != nil {
		t.Fatalf("dispatch Go through common page seam: %v\n%s", err, console.String())
	}
	if assessment.Status != report.PublicationReady {
		t.Fatalf("Go common-seam publication = %#v", assessment)
	}
	if semantic.documentationCalls != 1 || semantic.categorizationLanguages["go"] != 1 ||
		semantic.groupingLanguages["go"] != 1 || semantic.matchingCalls != 0 {
		t.Fatalf(
			"Go common semantic calls = docs %d, categorization %#v, grouping %#v, matching %d",
			semantic.documentationCalls, semantic.categorizationLanguages,
			semantic.groupingLanguages, semantic.matchingCalls,
		)
	}
	if semantic.providerFactoryCalls == 0 || semantic.provider.callCount() != 0 {
		t.Fatalf(
			"Go request-bound presets = provider factories %d, transport calls %d",
			semantic.providerFactoryCalls, semantic.provider.callCount(),
		)
	}
	if _, err := os.Stat(reportPath); err != nil {
		t.Fatalf("Go common-seam report %q: %v", reportPath, err)
	}
	runDirs := ordinaryGraphRunDirs(t, debugDir)
	if len(runDirs) != 1 {
		t.Fatalf("Go common-seam run directories = %v", runDirs)
	}
	index := ordinaryGraphReadProgramIndex(t, runDirs[0])
	if index.Target.Language != "go" || index.Categorization == nil ||
		len(index.Categorization.Assignments) != 1 {
		t.Fatalf("persisted Go common ProgramIndex = target %#v, categorization %#v", index.Target, index.Categorization)
	}
	foundPlatform := false
	for _, object := range index.Objects {
		if object.External == nil || object.External.PackagePath != "fmt" || object.External.Name != "Println" {
			continue
		}
		foundPlatform = object.External.AuthorityKind == programindex.ExternalAuthorityPlatform
	}
	if !foundPlatform {
		t.Fatalf("Go dispatcher did not preserve fmt.Println platform authority: %#v", index.Objects)
	}
	groups, err := groupindex.Read(runDirs[0])
	if err != nil {
		t.Fatal(err)
	}
	if groups.ProgramIndexSHA256 != index.SHA256 || len(groups.Groups) != 1 {
		t.Fatalf("persisted Go GroupsIndex does not bind the common ProgramIndex: %#v", groups)
	}
	dependencyRaw, err := os.ReadFile(filepath.Join(runDirs[0], dependencies.ArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	dependencyCatalog, err := dependencies.Decode(dependencyRaw)
	if err != nil {
		t.Fatal(err)
	}
	if dependencyCatalog.Coverage.State != dependencies.CoverageComplete {
		t.Fatalf("persisted Go dependency coverage = %#v", dependencyCatalog.Coverage)
	}
	pageRaw, err := os.ReadFile(filepath.Join(runDirs[0], programpage.ArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	pagePortfolio, err := programpage.Decode(pageRaw)
	if err != nil {
		t.Fatal(err)
	}
	if pagePortfolio.DefaultTargetID != index.Target.ID || len(pagePortfolio.Pages) != 1 ||
		pagePortfolio.Pages[0].Target.ID != index.Target.ID ||
		pagePortfolio.Pages[0].RunID != filepath.Base(runDirs[0]) {
		t.Fatalf("single-target ProgramPagePortfolio = %#v", pagePortfolio)
	}
	outcomeRaw, err := os.ReadFile(filepath.Join(runDirs[0], targetoutcome.ArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	outcomePortfolio, err := targetoutcome.Decode(outcomeRaw)
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomePortfolio.Outcomes) != 1 ||
		outcomePortfolio.Outcomes[0].State != targetoutcome.StateAnalyzed ||
		outcomePortfolio.Outcomes[0].Analysis == nil ||
		outcomePortfolio.Outcomes[0].Analysis.ProgramTarget.ID != index.Target.ID ||
		outcomePortfolio.Outcomes[0].Analysis.RunID != filepath.Base(runDirs[0]) {
		t.Fatalf("single-target TargetOutcomePortfolio = %#v", outcomePortfolio)
	}
	manifest, err := report.ReadRunManifest(runDirs[0])
	if err != nil {
		t.Fatal(err)
	}
	if manifest.MaterialInputs.ProgramPagePortfolioSHA256 != ordinaryGraphSHA256(pageRaw) ||
		manifest.MaterialInputs.TargetOutcomePortfolioSHA256 != ordinaryGraphSHA256(outcomeRaw) {
		t.Fatalf("single-target manifest omitted portfolio authority: %#v", manifest.MaterialInputs)
	}
	reportRaw, err := os.ReadFile(filepath.Join(runDirs[0], "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var reportData report.ReportData
	if err := json.Unmarshal(reportRaw, &reportData); err != nil {
		t.Fatal(err)
	}
	if reportData.TargetOutcomePortfolio == nil ||
		len(reportData.TargetOutcomePortfolio.Outcomes) != 1 {
		t.Fatalf("single-target report omitted outcome projection: %#v", reportData.TargetOutcomePortfolio)
	}
}

func TestRepositoryTargetDispatcherContainsMissingJSTSCompilerAndPublishesHealthySibling(t *testing.T) {
	repositoryRoot := ordinaryGraphCumulativeRepository(t)
	repository, err := corpus.Open(t.Context(), repositoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	repositoryState, err := freshness.CaptureRepository(t.Context(), repositoryRoot, repository)
	if err != nil {
		t.Fatal(err)
	}

	pythonCatalog, err := pythontarget.Discover(t.Context(), repository)
	if err != nil {
		t.Fatal(err)
	}
	var pythonTarget pythontarget.Target
	for _, target := range pythonCatalog.Entries {
		if target.Selector == "python:.:script:repomap-fixture" {
			pythonTarget = target
			break
		}
	}
	if pythonTarget.Selector == "" {
		t.Fatalf("cumulative Python fixture target is absent: %#v", pythonCatalog.Entries)
	}
	pythonPlanned, err := newPythonRepositoryTypedTarget(pythonTarget)
	if err != nil {
		t.Fatal(err)
	}
	jstsProject := jsTSTestProjectAt(
		t, repository, "typescript", "package.json", "src/server.ts",
	)
	jstsTarget, err := jstsproject.TargetFromResult(jstsProject)
	if err != nil {
		t.Fatal(err)
	}
	jstsPlanned, err := newJSTSRepositoryTypedTarget(jstsTarget)
	if err != nil {
		t.Fatal(err)
	}
	targets := []repositoryTypedTarget{pythonPlanned, jstsPlanned}
	sort.Slice(targets, func(i, j int) bool {
		return repositoryTypedTargetLess(targets[i], targets[j])
	})
	plan := repositoryTargetPlan{
		Targets: targets, Default: pythonPlanned.Key,
		Authorities: map[repositoryTargetAdapter]any{
			repositoryTargetAdapterPython: pythonCatalog.Snapshot(),
		},
		Outcome: targetPortfolioRunOutcome{
			SelectedRef: pythonPlanned.Key.String(), SelectedTargets: len(targets),
			SelectedTargetRefs: repositoryTargetRefs(targets),
		},
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("validate partial-failure plan: %v", err)
	}

	semantic := newOrdinaryGraphSemanticPreset(1)
	debugDir := t.TempDir()
	var verifiedRuns []report.VerifiedRunReceipt
	var console bytes.Buffer
	assessment, reportPath, err := dispatchRepositoryTargetPlan(
		t.Context(),
		repositoryTargetDispatchOptions{
			Repo: repositoryRoot,
			ExtraArgs: []string{
				"--debug-dir", debugDir, "--no-cache", "--no-open",
			},
			Deps: semantic.deps(t.Context()), Corpus: repository,
			RepositoryState: repositoryState, Plan: plan,
			RunID: "partial-target-failure", DebugDir: debugDir, NoCache: true, NoOpen: true,
			Output: newRunOutput(&console),
			DiscoverJSTSFn: func(
				_ context.Context, _ *corpus.Corpus, repo, selector string,
			) (jstsproject.Result, error) {
				if repo != repositoryRoot || selector != jstsTarget.Selector {
					return jstsproject.Result{}, fmt.Errorf(
						"unexpected selected JSTS materialization %q in %q", selector, repo,
					)
				}
				return jstsproject.Result{}, fmt.Errorf(
					"%w: fixture compiler is unavailable", jstsproject.ErrTypeScriptCompilerUnavailable,
				)
			},
			VerifiedRunsSink: func(receipts []report.VerifiedRunReceipt) {
				verifiedRuns = append([]report.VerifiedRunReceipt(nil), receipts...)
			},
		},
	)
	if err != nil {
		t.Fatalf("partial target failure dispatch: %v\n%s", err, console.String())
	}
	if assessment.Status != report.PublicationReady {
		t.Fatalf("partial target failure publication = %#v", assessment)
	}
	if semantic.documentationCalls != 1 || semantic.categorizationLanguages["python"] != 1 ||
		semantic.categorizationLanguages["typescript"] != 0 ||
		semantic.groupingLanguages["python"] != 1 ||
		semantic.groupingLanguages["typescript"] != 0 || semantic.matchingCalls != 1 ||
		len(verifiedRuns) != 1 {
		t.Fatalf(
			"partial failure calls = docs %d, categorization %#v, grouping %#v, matching %d, verified %d",
			semantic.documentationCalls, semantic.categorizationLanguages,
			semantic.groupingLanguages, semantic.matchingCalls, len(verifiedRuns),
		)
	}
	if semantic.providerFactoryCalls == 0 || semantic.provider.callCount() != 0 {
		t.Fatalf(
			"partial failure request-bound presets = provider factories %d, transport calls %d",
			semantic.providerFactoryCalls, semantic.provider.callCount(),
		)
	}
	if _, err := os.Stat(reportPath); err != nil {
		t.Fatalf("partial failure owner report %q: %v", reportPath, err)
	}
	runDirs := ordinaryGraphRunDirs(t, debugDir)
	if len(runDirs) != 1 {
		t.Fatalf("partial failure successful run directories = %v", runDirs)
	}
	outcomeRaw, err := os.ReadFile(filepath.Join(runDirs[0], targetoutcome.ArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	outcomePortfolio, err := targetoutcome.Decode(outcomeRaw)
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomePortfolio.Outcomes) != 2 {
		t.Fatalf("partial failure outcomes = %#v", outcomePortfolio.Outcomes)
	}
	outcomesBySelector := make(map[string]targetoutcome.Outcome, len(outcomePortfolio.Outcomes))
	for _, outcome := range outcomePortfolio.Outcomes {
		outcomesBySelector[outcome.SelectedTarget.Selector] = outcome
	}
	pythonOutcome, ok := outcomesBySelector[pythonTarget.Selector]
	if !ok || pythonOutcome.State != targetoutcome.StateAnalyzed || pythonOutcome.Analysis == nil ||
		pythonOutcome.Analysis.ProgramTarget.Language != "python" || pythonOutcome.Failure != nil {
		t.Fatalf("healthy Python sibling outcome = %#v", pythonOutcome)
	}
	jstsOutcome, ok := outcomesBySelector[jstsTarget.Selector]
	if !ok || jstsOutcome.State != targetoutcome.StateNotAnalyzed || jstsOutcome.Analysis != nil ||
		jstsOutcome.Failure == nil ||
		jstsOutcome.Failure.Stage != targetoutcome.StageTargetPreparation ||
		jstsOutcome.Failure.Reason != targetoutcome.ReasonRequiredToolUnavailable {
		t.Fatalf("missing-compiler JSTS outcome = %#v", jstsOutcome)
	}
	reportRaw, err := os.ReadFile(filepath.Join(runDirs[0], "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var data report.ReportData
	if err := json.Unmarshal(reportRaw, &data); err != nil {
		t.Fatal(err)
	}
	if data.GroupGraph == nil || len(data.GroupGraph.Indexes) != 1 ||
		data.GroupGraph.Indexes[0].Target.Language != "python" {
		t.Fatalf("partial failure report graph = %#v", data.GroupGraph)
	}
	if !strings.Contains(console.String(), "analyzed: 1/2") ||
		!strings.Contains(console.String(), "not analyzed: 1") {
		t.Fatalf("partial failure console omitted exhaustive coverage:\n%s", console.String())
	}
}

type ordinaryGraphSemanticPreset struct {
	provider                *ordinaryGraphNoNetworkProvider
	providerFactoryCalls    int
	documentationCalls      int
	categorizationLanguages map[string]int
	groupingLanguages       map[string]int
	matchingCalls           int
	wantMatchingIndexCount  int
}

func newOrdinaryGraphSemanticPreset(wantMatchingIndexCount int) *ordinaryGraphSemanticPreset {
	return &ordinaryGraphSemanticPreset{
		provider:                &ordinaryGraphNoNetworkProvider{},
		categorizationLanguages: make(map[string]int),
		groupingLanguages:       make(map[string]int),
		wantMatchingIndexCount:  wantMatchingIndexCount,
	}
}

func (preset *ordinaryGraphSemanticPreset) deps(ctx context.Context) defaultRunDeps {
	return defaultRunDeps{
		ctx: ctx, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{},
		llmBatchConcurrency: 2, llmBatchController: &llm.BatchController{},
		newCubeProvider: func() (llm.Provider, error) {
			preset.providerFactoryCalls++
			return preset.provider, nil
		},
		runDocumentationReduce: func(
			ctx context.Context,
			executor llm.Executor,
			provider llm.Provider,
			guidance readmetargetscout.GuidanceSnapshot,
		) (documentationreduce.Result, error) {
			preset.documentationCalls++
			if executor.Enabled || provider != nil || len(guidance.Documents) != 0 {
				return documentationreduce.Result{}, fmt.Errorf(
					"unexpected documentation preset authority: executor=%#v provider=%#v guidance=%#v",
					executor, provider, guidance,
				)
			}
			return documentationreduce.Run(ctx, executor, provider, guidance)
		},
		runProgramCategorization: func(
			_ context.Context,
			executor llm.Executor,
			provider llm.Provider,
			base programindex.Index,
			documentation documentationreduce.Result,
		) (programcategorization.Result, error) {
			if executor.Enabled || provider != preset.provider || len(base.Objects) == 0 {
				return programcategorization.Result{}, fmt.Errorf(
					"unexpected categorization preset authority for %s", base.Target.ID,
				)
			}
			preset.categorizationLanguages[base.Target.Language]++
			return programcategorization.Result{
				ProgramTargetID:            base.Target.ID,
				BaseProgramIndexSHA256:     base.SHA256,
				ReducedDocumentationSHA256: documentation.ReductionSHA256,
				Assignments: []programcategorization.Assignment{{
					SubjectID:  base.Objects[0].ID,
					Categories: []programcategorization.Category{programcategorization.CategoryCore},
				}},
				Diagnostics: []programcategorization.Diagnostic{},
			}, nil
		},
		runProgramGrouping: func(
			_ context.Context,
			executor llm.Executor,
			provider llm.Provider,
			program programindex.Index,
		) (groupindex.Index, []groupindex.Diagnostic, error) {
			if executor.Enabled || provider != preset.provider || program.Categorization == nil ||
				len(program.Categorization.Assignments) != 1 {
				return groupindex.Index{}, nil, fmt.Errorf(
					"unexpected grouping preset authority for %s", program.Target.ID,
				)
			}
			preset.groupingLanguages[program.Target.Language]++
			subjectID := program.Categorization.Assignments[0].SubjectID
			return groupindex.Build(program, groupindex.Proposals{Groups: []groupindex.GroupProposal{{
				Key: "core", Title: program.Target.Name + " core",
				Summary: "Owns the selected target's core behavior.", Lane: groupindex.LaneCore,
				MemberSubjectIDs: []string{subjectID}, EvidenceSubjectIDs: []string{subjectID},
			}}})
		},
		runOrientation: func(
			_ context.Context,
			executor llm.Executor,
			_ llm.Provider,
			input orientation.Input,
		) (orientation.Result, []orientation.RejectedRow, error) {
			if executor.Enabled {
				return orientation.Result{}, nil, fmt.Errorf("orientation reused the live cache")
			}
			result, sealErr := orientation.Empty(
				input.Facts.SHA256, input.Claims.SHA256, orientationGroupDigests(input.Groups), 0,
			)
			return result, nil, sealErr
		},
		runGroupMatching: func(
			_ context.Context,
			executor llm.Executor,
			provider llm.Provider,
			indexes []groupindex.Index,
		) ([]groupindex.Index, []groupindex.Diagnostic, error) {
			preset.matchingCalls++
			if executor.Enabled || provider != preset.provider ||
				len(indexes) != preset.wantMatchingIndexCount {
				return nil, nil, fmt.Errorf(
					"matching received %d GroupsIndexes, want %d",
					len(indexes), preset.wantMatchingIndexCount,
				)
			}
			matched := make([]groupindex.Index, len(indexes))
			for index := range indexes {
				matched[index] = indexes[index].Snapshot()
			}
			return matched, nil, nil
		},
	}
}

func ordinaryGraphReadProgramIndex(t *testing.T, runDir string) programindex.Index {
	t.Helper()
	setRaw, err := os.ReadFile(filepath.Join(runDir, programindex.ArtifactSetFilename))
	if err != nil {
		t.Fatal(err)
	}
	set, err := programindex.DecodeArtifactSet(setRaw)
	if err != nil || len(set.Entries) != 1 || set.Entries[0].Filename != programindex.ArtifactFilename {
		t.Fatalf("page-local ProgramIndex set = %#v, %v", set, err)
	}
	indexRaw, err := os.ReadFile(filepath.Join(runDir, set.Entries[0].Filename))
	if err != nil {
		t.Fatal(err)
	}
	index, err := programindex.Decode(indexRaw)
	if err != nil {
		t.Fatal(err)
	}
	return index
}

func ordinaryGraphGoRepository(t *testing.T) string {
	t.Helper()
	repositoryRoot := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/common-page\n\ngo 1.22\n",
		"cmd/app/main.go": `package main

import (
	"fmt"

	"example.com/common-page/internal/work"
)

func main() {
	fmt.Println("run")
	work.Run()
}
`,
		"internal/work/work.go": `package work

func Run() { execute() }

func execute() {}
`,
	}
	paths := make([]string, 0, len(files))
	for filePath := range files {
		paths = append(paths, filePath)
	}
	sort.Strings(paths)
	for _, filePath := range paths {
		absolute := filepath.Join(repositoryRoot, filepath.FromSlash(filePath))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(files[filePath]), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ordinaryGraphGit(t, repositoryRoot, "init", "--quiet")
	ordinaryGraphGit(t, repositoryRoot, "config", "user.email", "repomap@example.test")
	ordinaryGraphGit(t, repositoryRoot, "config", "user.name", "repomap fixture")
	ordinaryGraphGit(t, repositoryRoot, "add", ".")
	ordinaryGraphGit(t, repositoryRoot, "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "Go common page fixture")
	return repositoryRoot
}

func ordinaryGraphCumulativeRepository(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve repository-target test source path")
	}
	sourceRoot := filepath.Join(filepath.Dir(filename), "..", "..", "testdata", "repositories")
	repositoryRoot := t.TempDir()
	for _, fixture := range []string{"python", "jsts"} {
		fixtureRoot := filepath.Join(sourceRoot, fixture)
		err := filepath.WalkDir(fixtureRoot, func(sourcePath string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relative, err := filepath.Rel(fixtureRoot, sourcePath)
			if err != nil || relative == "." {
				return err
			}
			destination := filepath.Join(repositoryRoot, relative)
			if entry.IsDir() {
				return os.MkdirAll(destination, 0o755)
			}
			if !entry.Type().IsRegular() {
				return fmt.Errorf("cumulative fixture contains unsupported file %q", relative)
			}
			contents, err := os.ReadFile(sourcePath)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
				return err
			}
			return os.WriteFile(destination, contents, 0o644)
		})
		if err != nil {
			t.Fatalf("copy cumulative %s fixture: %v", fixture, err)
		}
	}
	ordinaryGraphGit(t, repositoryRoot, "init", "--quiet")
	ordinaryGraphGit(t, repositoryRoot, "config", "user.email", "repomap@example.test")
	ordinaryGraphGit(t, repositoryRoot, "config", "user.name", "repomap fixture")
	ordinaryGraphGit(t, repositoryRoot, "add", ".")
	ordinaryGraphGit(t, repositoryRoot, "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "cumulative fixtures")
	return repositoryRoot
}

func ordinaryGraphGit(t *testing.T, directory string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, arguments...)...)
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
}

func ordinaryGraphRunDirs(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	runDirs := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runDir := filepath.Join(root, entry.Name())
		if _, err := os.Stat(filepath.Join(runDir, report.RunManifestFilename)); err == nil {
			runDirs = append(runDirs, runDir)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	sort.Strings(runDirs)
	return runDirs
}

func ordinaryGraphSHA256(raw []byte) string {
	digest := sha256.Sum256(raw)
	return fmt.Sprintf("%x", digest[:])
}

type ordinaryGraphNoNetworkProvider struct {
	completeCalls int
}

func (*ordinaryGraphNoNetworkProvider) State() []byte {
	return []byte(`{"endpoint":"https://provider.test/v1/chat","model":"ordinary-graph-preset"}`)
}

func (*ordinaryGraphNoNetworkProvider) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	wire := []byte(prompt.System + "\n" + prompt.User)
	if len(wire) == 1 {
		wire = []byte("ordinary graph preset")
	}
	return llm.NewPrepared(wire)
}

func (provider *ordinaryGraphNoNetworkProvider) Complete(
	context.Context,
	llm.Prepared,
) (llm.Completion, error) {
	provider.completeCalls++
	return llm.Completion{}, errors.New("ordinary graph preset forbids network completion")
}

func (provider *ordinaryGraphNoNetworkProvider) callCount() int {
	return provider.completeCalls
}

// orientationGroupDigests lists the sealed graph digests an orientation
// result must bind.
func orientationGroupDigests(indexes []groupindex.Index) []string {
	digests := make([]string, 0, len(indexes))
	for _, index := range indexes {
		digests = append(digests, index.SHA256)
	}
	return digests
}
