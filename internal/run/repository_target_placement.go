package run

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	positions := make(map[repositoryTargetKey]int)
	for _, candidate := range native {
		keys[candidate.Row.Ref] = candidate.Target.Key
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
		if plan.Targets[positions[candidate.Target.Key]].Placement != "shared_code" {
			continue
		}
		for _, consumer := range candidate.Consumers {
			if position, ok := positions[consumer]; ok {
				plan.Targets[position].SharedCode = append(plan.Targets[position].SharedCode, candidate.Target.Key)
			}
		}
	}
	retained := make([]repositoryTypedTarget, 0, len(plan.Targets))
	for _, target := range plan.Targets {
		if !strings.HasPrefix(target.Placement, "seed_of:") {
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
