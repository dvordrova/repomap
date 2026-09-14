package run

import (
	"github.com/dvordrova/repomap/internal/programindex"
	"testing"
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
