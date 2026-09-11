package run

import (
	"runtime"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/readmetargetscout"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/targetportfolio"
)

// A Go module library whose only consumer is one standalone executable is
// folded into that executable; a library shared by two executables stays a
// shared_code target beside them.
func TestSoleConsumerModuleLibraryFoldsIntoItsExecutable(t *testing.T) {
	root, repository := cumulativeEvidenceRepository(t, "go")
	source, err := snapshot.BuildContext(t.Context(), snapshot.Options{RepoPath: root, GoTarget: runtime.GOOS + "/" + runtime.GOARCH, RepositoryCorpus: repository})
	if err != nil {
		t.Fatal(err)
	}
	discovery, err := discoverRepositoryTargets(t.Context(), repositoryTargetRuntimeOptions{Repository: repository, NoModel: true, GoSnapshot: &source})
	if err != nil {
		t.Fatal(err)
	}
	guidance, err := readmetargetscout.Compile("go", repository)
	if err != nil {
		t.Fatal(err)
	}
	if discovery.guidance, err = guidance.GuidanceSnapshot(); err != nil {
		t.Fatal(err)
	}
	native, err := repositoryNativeCandidates(repository, discovery)
	if err != nil {
		t.Fatal(err)
	}
	library := -1
	var targets []repositoryTypedTarget
	var placements []targetportfolio.Placement
	for i, candidate := range native {
		targets = append(targets, candidate.Target)
		decision := "standalone"
		if candidate.Row.Kind == "library" && candidate.Row.Root == "." {
			library, decision = i, "shared_code"
		}
		placements = append(placements, targetportfolio.Placement{Candidate: candidate.Row, Decision: decision})
	}
	if library < 0 || len(native[library].Consumers) != 2 {
		t.Fatalf("fixture library with two consumers missing: %d", library)
	}
	consumer := native[library].Consumers[0]
	build := func() repositoryTargetPlan {
		plan, err := repositoryTargetPlanFromDiscovery(discovery, targets, consumer, false, targetPortfolioRunOutcome{SelectedRef: consumer.String(), SelectedTargets: len(targets), SelectedTargetRefs: repositoryTargetRefs(targets)})
		if err != nil {
			t.Fatal(err)
		}
		return plan
	}
	shared, err := applyRepositoryPlacements(build(), native, placements)
	if err != nil {
		t.Fatal(err)
	}
	owner, found := shared.DefaultTarget()
	if !found || len(shared.Targets) != len(native) || len(owner.SharedCode) != 1 || owner.AbsorbedRoot != "" {
		t.Fatalf("a library with two consumers did not stay shared: targets=%d shared=%v absorbed=%q", len(shared.Targets), owner.SharedCode, owner.AbsorbedRoot)
	}
	sole := append([]repositoryNativeCandidate(nil), native...)
	sole[library].Consumers = sole[library].Consumers[:1]
	folded, err := applyRepositoryPlacements(build(), sole, placements)
	if err != nil {
		t.Fatal(err)
	}
	owner, found = folded.DefaultTarget()
	if !found || len(folded.Targets) != len(native)-1 || len(owner.SharedCode) != 0 || len(owner.Absorbed) != 1 || owner.Absorbed[0] != native[library].Target.Key || owner.AbsorbedRoot != "." {
		t.Fatalf("a library with one consumer did not fold into it: targets=%d shared=%v absorbed=%v root=%q", len(folded.Targets), owner.SharedCode, owner.Absorbed, owner.AbsorbedRoot)
	}
	for _, target := range folded.Targets {
		if target.Key == native[library].Target.Key {
			t.Fatal("folded library kept its own page")
		}
	}
	journaled := false
	for _, row := range folded.Outcome.Placements {
		if row.Target == native[library].Target.Key.String() && strings.HasPrefix(row.Decision, "folded_into:") {
			journaled = true
		}
	}
	if !journaled {
		t.Fatalf("fold not journaled beside the model decision: %+v", folded.Outcome.Placements)
	}
}
