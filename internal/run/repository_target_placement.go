package run

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/targetportfolio"
)

type repositoryPlacement struct {
	Target   string `json:"target"`
	Selector string `json:"selector"`
	targetportfolio.Placement
}

func applyRepositoryPlacements(plan repositoryTargetPlan, native []repositoryNativeCandidate, placements []targetportfolio.Placement) (repositoryTargetPlan, error) {
	if len(native) != len(placements) {
		return repositoryTargetPlan{}, fmt.Errorf("incomplete native placement result")
	}
	keys := make(map[string]repositoryTargetKey)
	refs := make(map[repositoryTargetKey]string)
	positions := make(map[repositoryTargetKey]int)
	for _, candidate := range native {
		keys[candidate.Row.Ref] = candidate.Target.Key
		refs[candidate.Target.Key] = candidate.Row.Ref
	}
	for i, target := range plan.Targets {
		positions[target.Key] = i
	}
	for _, placement := range placements {
		key, known := keys[placement.Candidate.Ref]
		if !known {
			return repositoryTargetPlan{}, fmt.Errorf("placement outside native authority")
		}
		position, found := positions[key]
		if !found {
			return repositoryTargetPlan{}, fmt.Errorf("native target missing before placement")
		}
		plan.Targets[position].Placement = placement.Decision
		plan.Outcome.Placements = append(plan.Outcome.Placements, repositoryPlacement{Target: key.String(), Selector: plan.Targets[position].Selector, Placement: placement})
	}
	for i := range plan.Targets {
		target := &plan.Targets[i]
		if ownerRef, seed := strings.CutPrefix(target.Placement, "seed_of:"); seed {
			ownerKey, ok := keys[ownerRef]
			if !ok {
				return repositoryTargetPlan{}, fmt.Errorf("seed owner missing")
			}
			ownerPosition, ok := positions[ownerKey]
			if !ok || plan.Targets[ownerPosition].Placement != "standalone" {
				return repositoryTargetPlan{}, fmt.Errorf("seed owner is not standalone")
			}
			plan.Targets[ownerPosition].Seeds = append(plan.Targets[ownerPosition].Seeds, *target)
			if plan.Default == target.Key {
				plan.Default = ownerKey
			}
		}
	}
	for _, candidate := range native {
		position := positions[candidate.Target.Key]
		if plan.Targets[position].Placement != "shared_code" {
			continue
		}
		// A Go module library whose only consumer is one standalone
		// executable is that executable's own code: cmd/app beside internal/
		// and pkg/ packages is one program, not a program and a library. The
		// model's shared_code decision stays in the journal; the page is the
		// code's to compose. Two or more consumers keep the shared library.
		if owner, folds := soleStandaloneConsumer(plan, positions, candidate); folds {
			if library, isGo := repositoryGoTarget(candidate.Target); isGo && library.Kind == analysistarget.KindModuleLibrary {
				ownerRef := refs[plan.Targets[owner].Key]
				plan.Targets[owner].Absorbed = append(plan.Targets[owner].Absorbed, candidate.Target.Key)
				plan.Targets[owner].AbsorbedRoot = library.ModuleDir
				plan.Targets[position].Placement = "folded_into:" + ownerRef
				plan.Outcome.Placements = append(plan.Outcome.Placements, repositoryPlacement{
					Target: candidate.Target.Key.String(), Selector: plan.Targets[position].Selector,
					Placement: targetportfolio.Placement{Candidate: candidate.Row, Decision: "folded_into:" + ownerRef,
						Reason: "a module library whose only consumer is one standalone executable is that executable's own code"},
				})
				continue
			}
		}
		for _, consumer := range candidate.Consumers {
			if position, ok := positions[consumer]; ok {
				plan.Targets[position].SharedCode = append(plan.Targets[position].SharedCode, candidate.Target.Key)
			}
		}
	}
	retained := make([]repositoryTypedTarget, 0, len(plan.Targets))
	for _, target := range plan.Targets {
		if !strings.HasPrefix(target.Placement, "seed_of:") && !strings.HasPrefix(target.Placement, "folded_into:") {
			retained = append(retained, target)
		}
	}
	plan.Targets = retained
	for i := range plan.Targets {
		if plan.Targets[i].Placement == "" {
			plan.Targets[i].Placement = "standalone"
		}
	}
	// A retained default remains a UI choice. If it is auxiliary, prefer the
	// first accepted standalone target in the same canonical order.
	if selected, ok := plan.DefaultTarget(); ok && selected.Placement != "standalone" {
		for _, target := range plan.Targets {
			if target.Placement == "standalone" {
				plan.Default = target.Key
				break
			}
		}
	}
	plan.Outcome.SelectedRef = plan.Default.String()
	plan.Outcome.SelectedTargets = len(plan.Targets)
	plan.Outcome.SelectedTargetRefs = repositoryTargetRefs(plan.Targets)
	return plan, plan.Validate()
}

// soleStandaloneConsumer returns the position of the one standalone target
// that consumes this library, when there is exactly one.
func soleStandaloneConsumer(plan repositoryTargetPlan, positions map[repositoryTargetKey]int, candidate repositoryNativeCandidate) (int, bool) {
	if len(candidate.Consumers) != 1 {
		return 0, false
	}
	position, ok := positions[candidate.Consumers[0]]
	if !ok || plan.Targets[position].Placement != "standalone" {
		return 0, false
	}
	return position, true
}

func persistRepositoryPlacements(runDir string, placements []repositoryPlacement) error {
	if len(placements) == 0 {
		return nil
	}
	raw, err := json.MarshalIndent(struct {
		Version   int                   `json:"version"`
		Decisions []repositoryPlacement `json:"decisions"`
	}{1, placements}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(runDir, "target-placements.json"), append(raw, '\n'), 0600)
}
