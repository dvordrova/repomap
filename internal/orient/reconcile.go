package orient

import (
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowproof"
)

// reconcileResolvedUnknownPaths removes only exact file paths that the local
// proof has since resolved. It deliberately leaves unresolved, model-suggested,
// unrelated, and merely similar paths untouched.
func reconcileResolvedUnknownPaths(report *orientationPart) {
	if len(report.UnverifiedPaths) == 0 {
		return
	}
	resolved := make(map[string]struct{})
	for _, flow := range report.CandidateFlows {
		if flow.LocalProof == nil {
			continue
		}
		proof := flow.LocalProof.Proof
		anchors := make(map[string]flowproof.Anchor, len(proof.Anchors))
		for _, anchor := range proof.Anchors {
			anchors[anchor.ID] = anchor
		}

		for _, slot := range proof.Slots {
			if slot.Status != flowproof.SlotVerified {
				continue
			}
			for _, evidenceID := range slot.EvidenceIDs {
				addResolvedAnchorPath(resolved, anchors[evidenceID])
			}
		}
		for _, transition := range proof.Transitions {
			if !resolvedTargetResolution(transition.Resolution) {
				continue
			}
			addResolvedAnchorPath(resolved, anchors[transition.To])
		}
	}

	if len(resolved) == 0 {
		return
	}
	kept := make(unverifiedPathList, 0, len(report.UnverifiedPaths))
	for _, unknown := range report.UnverifiedPaths {
		if _, ok := resolved[unknown.Path]; !ok {
			kept = append(kept, unknown)
		}
	}
	report.UnverifiedPaths = kept
}

func addResolvedAnchorPath(resolved map[string]struct{}, anchor flowproof.Anchor) {
	if anchor.Location == nil || anchor.Location.Path == "" {
		return
	}
	resolved[anchor.Location.Path] = struct{}{}
}

func resolvedTargetResolution(resolution evidence.ResolutionKind) bool {
	switch resolution {
	case evidence.ResolutionStatic,
		evidence.ResolutionFrameworkRule,
		evidence.ResolutionRuntimeObserved:
		return true
	default:
		return false
	}
}
