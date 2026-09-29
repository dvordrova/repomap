package run

import (
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestClojureCumulativeOrdinaryAdapter(t *testing.T) {
	root, repository := cumulativeEvidenceRepository(t, "clojure")
	discovery, enabled, err := discoverClojureRepositoryTargets(t.Context(), repositoryTargetRuntimeOptions{RepoName: "clojure", Repository: repository, NoModel: true})
	if err != nil || !enabled {
		t.Fatalf("discovery: %v %v", enabled, err)
	}
	targets, err := discovery.ResolveExplicit(repository, "clojure:deps.edn")
	if err != nil || len(targets) != 1 {
		t.Fatalf("selection: %v %v", targets, err)
	}
	adapter := clojureRepositoryTargetAdapterDescriptor()
	binding, err := adapter.PrepareDispatchTarget(t.Context(), repositoryTargetDispatchOptions{Repo: root, Corpus: repository}, targets[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	input, err := adapter.BuildProgramInput(repositoryProgramBuildRequest{Context: t.Context(), Corpus: repository, Target: binding.Target, Facts: binding.ProgramFacts})
	if err != nil {
		t.Fatal(err)
	}
	index, err := programindex.New(input)
	if err != nil {
		t.Fatal(err)
	}
	if !adapter.MatchProgramTarget(binding.Target, index.Target) {
		t.Fatal("ordinary target binding failed")
	}
	deps, err := adapter.BuildDependencies(repositoryDependencyBuildRequest{Target: binding.Target, ProgramIndex: index, Facts: binding.ProgramFacts})
	if err != nil {
		t.Fatal(err)
	}
	if len(deps.Dependencies) == 0 {
		t.Fatal("dependency projection is empty")
	}
}

// The fixture's shadow-cljs.edn restores its :app build, a ClojureScript
// program beside the JVM project that deps.edn restores; an exact --target
// names the build by its manifest and id.
func TestClojureShadowBuildIsDiscoveredBesideTheJVMProject(t *testing.T) {
	root, repository := cumulativeEvidenceRepository(t, "clojure")
	discovery, enabled, err := discoverClojureRepositoryTargets(t.Context(), repositoryTargetRuntimeOptions{RepoName: "clojure", Repository: repository, NoModel: true})
	if err != nil || !enabled {
		t.Fatalf("discovery: %v %v", enabled, err)
	}
	deps, _ := repository.ID("deps.edn")
	shadow, _ := repository.ID("shadow-cljs.edn")
	if len(discovery.RequiredFileRefs) != 2 || !discovery.ResolvesFile(deps) || !discovery.ResolvesFile(shadow) {
		t.Fatalf("required files: %v", discovery.RequiredFileRefs)
	}
	restored, err := discovery.RestoreFiles([]corpus.FileID{shadow})
	if err != nil || len(restored) != 1 || restored[0].Target.Selector != "clojure:shadow-cljs.edn:app" {
		t.Fatalf("restored: %+v %v", restored, err)
	}
	targets, err := discovery.ResolveExplicit(repository, "clojure:shadow-cljs.edn:app")
	if err != nil || len(targets) != 1 {
		t.Fatalf("selection: %v %v", targets, err)
	}
	evidence, err := discovery.NativeEvidence(targets[0])
	if err != nil || len(evidence.Observations) != 1 || evidence.Observations[0].Values[0] != "example.web/init" {
		t.Fatalf("evidence: %+v %v", evidence, err)
	}
	adapter := clojureRepositoryTargetAdapterDescriptor()
	binding, err := adapter.PrepareDispatchTarget(t.Context(), repositoryTargetDispatchOptions{Repo: root, Corpus: repository}, targets[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	input, err := adapter.BuildProgramInput(repositoryProgramBuildRequest{Context: t.Context(), Corpus: repository, Target: binding.Target, Facts: binding.ProgramFacts})
	if err != nil {
		t.Fatal(err)
	}
	index, err := programindex.New(input)
	if err != nil {
		t.Fatal(err)
	}
	if !adapter.MatchProgramTarget(binding.Target, index.Target) || index.Target.Name != "app" || len(index.Target.Seeds) != 1 {
		t.Fatalf("build target: %+v", index.Target)
	}
}
