package run

import (
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythonprogramindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/dvordrova/repomap/internal/targetportfolio"
)

const (
	facetConsole = "python:client:script:fixture-client"
	facetScript  = "python:client:script-file:fixture_client/cli"
	facetLibrary = "python:client:library:library"
	facetBench   = "python:client:guard:fixture_client/bench"
)

// facetPlan places every cumulative Python target as decisions says, standalone
// by default (as the model left freqtrade's three client targets on
// 2026-09-29), and applies the placements.
func facetPlan(t *testing.T, decisions map[string]string) (*corpus.Corpus, repositoryTargetPlan, map[string]repositoryNativeCandidate) {
	t.Helper()
	_, repository := cumulativeEvidenceRepository(t, "python")
	discovery, err := discoverRepositoryTargets(t.Context(), repositoryTargetRuntimeOptions{Repository: repository, NoModel: true, DiscoverPython: true})
	if err != nil {
		t.Fatal(err)
	}
	native, err := repositoryNativeCandidates(repository, discovery)
	if err != nil {
		t.Fatal(err)
	}
	var targets []repositoryTypedTarget
	var placements []targetportfolio.Placement
	bySelector := map[string]repositoryNativeCandidate{}
	for _, candidate := range native {
		targets = append(targets, candidate.Target)
		bySelector[candidate.Target.Selector] = candidate
		decision := decisions[candidate.Target.Selector]
		if decision == "" {
			decision = "standalone"
		}
		placements = append(placements, targetportfolio.Placement{Candidate: candidate.Row, Decision: decision})
	}
	for _, selector := range []string{facetConsole, facetScript, facetLibrary, facetBench, "python:.:library:library", "python:.:script-file:src/fixture_app/script_context"} {
		if bySelector[selector].Row.Ref == "" {
			t.Fatalf("fixture target %s missing", selector)
		}
	}
	library := bySelector[facetLibrary].Target.Key
	plan, err := repositoryTargetPlanFromDiscovery(discovery, targets, library, false, targetPortfolioRunOutcome{SelectedRef: library.String(), SelectedTargets: len(targets), SelectedTargetRefs: repositoryTargetRefs(targets)})
	if err != nil {
		t.Fatal(err)
	}
	if plan, err = applyRepositoryPlacements(plan, native, placements); err != nil {
		t.Fatal(err)
	}
	return repository, plan, bySelector
}

// The cumulative fixture's client/ is freqtrade's ft_client: a console
// script (fixture-client = fixture_client.cli:main), a shebang on cli.py with
// no __main__ guard, and the package the same manifest installs. Three
// targets, one component: the script file is the console script's launch
// form (its seed) and the library its facet, whose root the program claims
// beside its own. bench.py, a guard in the same directory placed a tool,
// is a script: it holds its own file and shares only what it imports there;
// the tests only the library held become the program's. The root project's library declares acme beside fixture_app,
// code outside the console script's root, so it stays; script_context.py's
// shebang defines no program's launch callable, so it stays too.
func TestClientPackageIsOneProgramWithItsLaunchFormAndLibraryFacet(t *testing.T) {
	repository, plan, bySelector := facetPlan(t, map[string]string{facetBench: "tool"})
	var program, bench repositoryTypedTarget
	for _, target := range plan.Targets {
		switch target.Selector {
		case facetScript, facetLibrary:
			t.Fatalf("%s kept a component of its own", target.Selector)
		case facetConsole:
			program = target
		case facetBench:
			bench = target
		}
	}
	if len(plan.Targets) != len(bySelector)-2 || bench.Placement != "tool" {
		t.Fatalf("want exactly the script file and the library folded, got %d of %d targets", len(plan.Targets), len(bySelector))
	}
	if plan.Default != program.Key {
		t.Fatalf("the default stayed on the folded library: %v", plan.Default)
	}
	if len(program.Seeds) != 1 || program.Seeds[0].Selector != facetScript {
		t.Fatalf("the shebang is not the console script's launch form: %+v", program.Seeds)
	}
	if program.AbsorbedRoot != "client" || !slices.Equal(program.Libraries, []string{"fixture_client"}) || len(program.Absorbed) != 1 || program.Absorbed[0] != bySelector[facetLibrary].Target.Key {
		t.Fatalf("the library is not the program's facet: root=%q libraries=%v absorbed=%v", program.AbsorbedRoot, program.Libraries, program.Absorbed)
	}
	journaled := map[string]string{}
	for _, row := range plan.Outcome.Placements {
		if row.Reason != "" && (strings.HasPrefix(row.Decision, "seed_of:") || strings.HasPrefix(row.Decision, "folded_into:")) {
			journaled[row.Selector] = row.Decision
		}
	}
	ref := bySelector[facetConsole].Row.Ref
	if journaled[facetScript] != "seed_of:"+ref || journaled[facetLibrary] != "folded_into:"+ref || len(journaled) != 2 {
		t.Fatalf("folds not journaled beside the model's decisions: %v", journaled)
	}

	// The page keeps the shebang's anchor as a seed and names the facet.
	build := func(target repositoryTypedTarget, id string) programindex.Index {
		native, _ := repositoryPythonTarget(target)
		input, err := pythonprogramindex.BuildInput(t.Context(), repository, native)
		if err != nil {
			t.Fatal(err)
		}
		for _, seed := range target.Seeds {
			value, _ := repositoryPythonTarget(seed)
			if input, err = pythonprogramindex.WithSeeds(repository, input, native, value); err != nil {
				t.Fatal(err)
			}
		}
		input.Target.ID, input.Target.Libraries = id, target.Libraries
		index, err := programindex.New(input)
		if err != nil {
			t.Fatal(err)
		}
		return index
	}
	index := build(program, "t1")
	anchors := map[string]bool{}
	for _, seed := range index.Target.Seeds {
		anchors[seed.Location.Path+":"+string(seed.Kind)] = true
	}
	if !anchors["client/fixture_client/cli.py:callable"] || !anchors["client/fixture_client/cli.py:script"] || !slices.Equal(index.Target.Libraries, []string{"fixture_client"}) {
		t.Fatalf("the program lost the script anchor or the facet: seeds=%+v libraries=%v", index.Target.Seeds, index.Target.Libraries)
	}

	// The atlas: the program claims client/ beside its own directory; the
	// tool rooted there holds its file and shares the files it imports.
	var inputs []places.TargetInput
	for _, pair := range []struct {
		target repositoryTypedTarget
		id     string
	}{{program, "t1"}, {bench, "t2"}} {
		inputs = append(inputs, atlasPlacesTarget(build(pair.target, pair.id), pair.target))
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: inputs})
	if err != nil {
		t.Fatal(err)
	}
	held := map[string][]string{}
	for _, place := range graph.Places {
		if place.Kind == atlas.PlaceFile {
			held[place.Path] = place.TargetIDs
		}
	}
	if !slices.Equal(held["client/fixture_client/rest.py"], []string{"t1", "t2"}) || !slices.Equal(held["client/tests/test_rest.py"], []string{"t1"}) {
		t.Fatalf("claims: rest.py %v, test_rest.py %v", held["client/fixture_client/rest.py"], held["client/tests/test_rest.py"])
	}
	// bench.py is a script: it holds its own file and, in its directory,
	// only what it imports; cli.py, which it does not import, is the
	// console script's alone.
	if !slices.Equal(held["client/fixture_client/bench.py"], []string{"t2"}) || !slices.Equal(held["client/fixture_client/cli.py"], []string{"t1"}) {
		t.Fatalf("claims: bench.py %v, cli.py %v", held["client/fixture_client/bench.py"], held["client/fixture_client/cli.py"])
	}
}

// With the console script placed a tool, the standalone programs the
// library's packages build are bench.py, a declared guard the model was
// offered as the library's seed and kept apart, and cli.py's shebang, which
// runs a file, not the distribution's command: the library stays its own
// component, and the shebang, whose console script is no longer
// standalone, too.
func TestALibraryIsNoFacetOfAGuardItCouldHaveSeeded(t *testing.T) {
	_, plan, _ := facetPlan(t, map[string]string{facetConsole: "tool"})
	kept := map[string]string{}
	for _, target := range plan.Targets {
		kept[target.Selector] = target.Placement
		if target.AbsorbedRoot != "" || len(target.Libraries) != 0 {
			t.Fatalf("%s absorbed a library: %q %v", target.Selector, target.AbsorbedRoot, target.Libraries)
		}
	}
	if kept[facetLibrary] != "standalone" || kept[facetBench] != "standalone" || kept[facetScript] != "standalone" {
		t.Fatalf("a fold happened: %v", kept)
	}
	if !pythontarget.CanSeed(nativeOf(t, plan, facetLibrary), nativeOf(t, plan, facetBench)) {
		t.Fatal("bench.py is not a guard its library could seed")
	}
}

func nativeOf(t *testing.T, plan repositoryTargetPlan, selector string) pythontarget.Target {
	t.Helper()
	for _, target := range plan.Targets {
		if target.Selector == selector {
			value, _ := repositoryPythonTarget(target)
			return value
		}
	}
	t.Fatalf("%s not retained", selector)
	return pythontarget.Target{}
}
