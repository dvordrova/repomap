package run

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/pythonprogramindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
	"github.com/dvordrova/repomap/internal/targetportfolio"
)

func TestCumulativePythonLaunchFormsReachPortfolioAndRetainAllSeeds(t *testing.T) {
	_, repository := cumulativeEvidenceRepository(t, "python")
	discovery, err := discoverRepositoryTargets(t.Context(), repositoryTargetRuntimeOptions{Repository: repository, NoModel: true, DiscoverPython: true})
	if err != nil {
		t.Fatal(err)
	}
	guidance, err := readmetargetscout.Compile("python", repository)
	if err != nil {
		t.Fatal(err)
	}
	discovery.guidance, err = guidance.GuidanceSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	native, err := repositoryNativeCandidates(discovery)
	if err != nil {
		t.Fatal(err)
	}
	var owner repositoryNativeCandidate
	var alternatives []repositoryNativeCandidate
	var targets []repositoryTypedTarget
	for _, candidate := range native {
		targets = append(targets, candidate.Target)
		switch candidate.Target.Selector {
		case "python:.:script:repomap-fixture":
			owner = candidate
		case "python:.:module:fixture_app", "python:.:guard:src/fixture_app/cli":
			alternatives = append(alternatives, candidate)
		}
	}
	if owner.Row.Ref == "" || len(alternatives) != 2 {
		t.Fatal("cumulative launch forms missing")
	}
	var rows []targetportfolio.NativeCandidate
	var decisions []targetportfolio.NativeDecision
	for _, candidate := range native {
		rows = append(rows, candidate.Row)
		decisions = append(decisions, targetportfolio.NativeDecision{Ref: candidate.Row.Ref, Decision: "standalone"})
	}
	adapter := discovery.adapters[0]
	compiled, err := targetportfolio.CompileWithNativeAuthority(repository.Snapshot(), adapter.Candidates, adapter.RequiredFileRefs, rows)
	if err != nil {
		t.Fatal(err)
	}
	var choices []targetportfolio.NativeLaunchDecision
	for _, group := range compiled.Request.LaunchGroups {
		if slices.Contains(group.Members, owner.Row.Ref) {
			if len(group.Members) != 3 {
				t.Fatal("exact native launch group lost a form")
			}
			choices = append(choices, targetportfolio.NativeLaunchDecision{Ref: group.Ref, Owner: owner.Row.Ref})
		}
	}
	if len(choices) != 1 {
		t.Fatal("native callable equality did not become one model decision")
	}
	raw, err := json.Marshal(targetportfolio.Response{DefaultFileRef: &owner.Row.FileRef, TargetFileRefs: adapter.RequiredFileRefs, NativeDecisions: decisions, LaunchDecisions: choices})
	if err != nil {
		t.Fatal(err)
	}
	selection, err := targetportfolio.ResolveResponse(compiled, raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, launch := range alternatives {
		advertised, callable, site := false, false, false
		for _, eligible := range launch.Row.SeedOwners {
			advertised = advertised || eligible.Ref == owner.Row.Ref && eligible.SameLaunch
		}
		for _, evidence := range launch.Row.Evidence {
			callable = callable || evidence.Kind == "launch_callable" && evidence.Path == "src/fixture_app/cli.py" && evidence.Line > 0
			site = site || evidence.Kind == "launch_call_site" && evidence.Path != "" && evidence.Line > 0
		}
		if !advertised || !callable || !site {
			t.Fatalf("missing native launch authority: %+v", launch.Row)
		}
	}
	plan, err := repositoryTargetPlanFromDiscovery(discovery, targets, owner.Target.Key, false, targetPortfolioRunOutcome{SelectedRef: owner.Target.Key.String(), SelectedTargets: len(targets), SelectedTargetRefs: repositoryTargetRefs(targets)})
	if err != nil {
		t.Fatal(err)
	}
	plan, err = applyRepositoryPlacements(plan, native, selection.Placements)
	if err != nil {
		t.Fatal(err)
	}
	selected, found := plan.DefaultTarget()
	if !found || len(selected.Seeds) != 2 || len(plan.Targets) != len(native)-2 {
		t.Fatal("accepted launch forms not absorbed exactly once")
	}
	ownerNative, _ := repositoryPythonTarget(selected)
	var seeds []pythontarget.Target
	for _, seed := range selected.Seeds {
		value, ok := repositoryPythonTarget(seed)
		if !ok {
			t.Fatal("lost native seed")
		}
		seeds = append(seeds, value)
	}
	input, err := pythonprogramindex.BuildInput(t.Context(), repository, ownerNative)
	if err != nil {
		t.Fatal(err)
	}
	input, err = pythonprogramindex.WithSeeds(repository, input, ownerNative, seeds...)
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Target.Seeds) != 3 {
		t.Fatalf("source launch anchors disappeared: %+v", input.Target.Seeds)
	}
}
