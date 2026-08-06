package report

// MechanismFragment (Decision 226) is the one honest vertical fragment:
// surface/process entry → locally supported operation transitions →
// observed boundary/resource → explicit unresolved frontier. It is built
// entirely from saved local evidence (canvas behavior-handoff edges,
// behavior anchors, Atlas observations); no edge is invented to make a
// continuous path, and a disconnected fragment with an explicit frontier
// is a successful honest result.
import (
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

// MechanismFragmentVersion changes when the persisted fragment projection
// shape or its exact derivation rules change.
const MechanismFragmentVersion = 1

// MechanismFragmentProjection is the bounded provider-free vertical
// fragment for a selected mechanism/process entry.
type MechanismFragmentProjection struct {
	Version     int                   `json:"version"`
	Entry       MechanismTransition   `json:"entry"`
	Transitions []MechanismTransition `json:"transitions"`
	Frontier    MechanismFrontier     `json:"frontier"`
}

// MechanismTransition is one contract-carrying transition (Decision 226).
type MechanismTransition struct {
	Ordinal     int    `json:"ordinal"`
	ClaimKind   string `json:"claim_kind"`
	SupportMode string `json:"support_mode"`
	Label       string `json:"label"`
	Path        string `json:"path"`
	Line        int    `json:"line"`
	Symbol      string `json:"symbol,omitempty"`
	Evidence    string `json:"evidence"` // provenance provider/version/operation
	Scenario    string `json:"scenario,omitempty"`
	Limitation  string `json:"limitation"`
	Ordering    string `json:"ordering"`
}

// MechanismFrontier states what is NOT locally supported.
type MechanismFrontier struct {
	Ordering   string   `json:"ordering"` // not_established | partial_order
	Unresolved []string `json:"unresolved,omitempty"`
	Limitation string   `json:"limitation"`
}

// ProjectMechanismFragment derives the honest vertical fragment for the
// repository's primary process entry from saved local evidence only.
// Provider-free and deterministic: entries are exact process_entry anchors
// with call_target proof; transitions are behavior_handoff edges whose from
// member matches the entry symbol; boundary/resource rows come from the
// Decision 225 association join; everything else is an explicit frontier.
func ProjectMechanismFragment(
	canvas *ArchitectureCanvas,
	associations *ArchitectureAssociationProjection,
	atlas *repositoryatlas.Atlas,
) (*MechanismFragmentProjection, error) {
	if canvas == nil {
		return nil, nil
	}
	if canvas.Version != ArchitectureCanvasVersion {
		return nil, fmt.Errorf("mechanism fragment: unsupported canvas version %d", canvas.Version)
	}

	// Exact entry: first process_entry anchor with call_target proof.
	entryAnchor := findProcessEntryAnchor(canvas)
	if entryAnchor == nil {
		return nil, nil
	}
	entryMember := symbolMemberID(entryAnchor)
	if entryMember.Value == "" {
		return nil, nil
	}

	transitions := []MechanismTransition{}
	frontier := MechanismFrontier{Ordering: "not_established", Limitation: "No locally saved transitions beyond the observed handoffs; execution order beyond them is not established."}
	// Behavior-handoff edges from the entry symbol (exact, SSA-supported).
	var handoffTargets []MechanismTransition
	for _, edge := range canvas.StructuralEdges {
		witness := edge.Witness
		if witness.Kind != componentmap.StructuralRelationBehaviorHandoff {
			continue
		}
		if !memberIDEquals(witness.From, entryMember) {
			continue
		}
		transition := transitionFromWitness(edge, witness, len(handoffTargets)+1)
		handoffTargets = append(handoffTargets, transition)
	}
	sort.Slice(handoffTargets, func(i, j int) bool {
		if handoffTargets[i].Path != handoffTargets[j].Path {
			return handoffTargets[i].Path < handoffTargets[j].Path
		}
		return handoffTargets[i].Line < handoffTargets[j].Line
	})
	transitions = append(transitions, handoffTargets...)
	frontier.Unresolved = []string{"further locally saved transitions beyond the observed handoffs"}

	// Observed boundary/resource rows in the entry component scope
	// (Decision 225 association join) follow the transitions.
	entryComponentID := componentForAnchor(canvas, entryAnchor)
	if entryComponentID != "" && associations != nil {
		for _, component := range associations.Components {
			if string(component.ComponentID) != string(entryComponentID) {
				continue
			}
			for _, row := range component.Associations {
				transitions = append(transitions, MechanismTransition{
					Ordinal:     len(transitions) + 1,
					ClaimKind:   mechanismClaimKindForRow(row),
					SupportMode: "observed_local",
					Label:       row.Kind + " " + row.ImportedFamily,
					Path:        row.OwningUnit,
					Evidence:    "atlas boundary/resource observation",
					Scenario:    "Recorded build scenario",
					Limitation:  "physical target unknown; runtime reachability not proven; read/write/order semantics not proven",
					Ordering:    "not_established",
				})
			}
			break
		}
	}

	// The unresolved continuation is an explicit contract transition
	// (Decision 226): claim_kind unresolved_continuation, support_mode
	// unknown, ordering not_established. It is always last — it states the
	// honest disconnect beyond every locally observed transition and is
	// never fabricated into a continuous path.
	transitions = append(transitions, MechanismTransition{
		Ordinal:     len(transitions) + 1,
		ClaimKind:   "unresolved_continuation",
		SupportMode: "unknown",
		Label:       "beyond the observed handoffs",
		Evidence:    "no locally saved transition",
		Limitation:  "execution order and further transitions not established",
		Ordering:    "not_established",
	})

	entry := MechanismTransition{
		Ordinal:     0,
		ClaimKind:   "exact_registration",
		SupportMode: "resolved_static",
		Label:       entryAnchor.Label,
		Path:        entryAnchor.Location.Path,
		Line:        entryAnchor.Location.Line,
		Symbol:      entryAnchor.Label,
		Evidence:    "behavior anchor call_target",
		Scenario:    entryAnchor.Scenario.ID,
		Limitation:  "process entry identity only; runtime reachability not proven",
		Ordering:    "exact_local_order",
	}
	return &MechanismFragmentProjection{
		Version:     MechanismFragmentVersion,
		Entry:       entry,
		Transitions: transitions,
		Frontier:    frontier,
	}, nil
}

func mechanismClaimKindForRow(row ArchitectureBoundaryResourceRow) string {
	if row.Kind == "boundary" {
		return "storage_boundary_callsite"
	}
	return "outbound_client_callsite"
}

func findProcessEntryAnchor(canvas *ArchitectureCanvas) *componentmap.BehaviorAnchor {
	for index := range canvas.BehaviorAnchors {
		anchor := &canvas.BehaviorAnchors[index]
		if anchor.Kind != componentmap.AnchorProcessEntry {
			continue
		}
		if anchor.Location.Path == "" {
			continue
		}
		return anchor
	}
	return nil
}

func symbolMemberID(anchor *componentmap.BehaviorAnchor) componentmap.MemberID {
	if anchor == nil || len(anchor.MemberIDs) == 0 {
		return componentmap.MemberID{}
	}
	return anchor.MemberIDs[0]
}

func memberIDEquals(left, right componentmap.MemberID) bool {
	return left.Kind == right.Kind && left.Value == right.Value
}

func componentForAnchor(canvas *ArchitectureCanvas, anchor *componentmap.BehaviorAnchor) componentmap.ComponentID {
	if anchor == nil {
		return ""
	}
	for _, component := range canvas.Components {
		for _, member := range component.Members {
			for _, anchorMember := range anchor.MemberIDs {
				if memberIDEquals(member.ID, anchorMember) {
					return component.ID
				}
			}
		}
	}
	return ""
}

func transitionFromWitness(
	edge ArchitectureStructuralEdge,
	witness componentmap.LocalRelation,
	ordinal int,
) MechanismTransition {
	transition := MechanismTransition{
		Ordinal:     ordinal,
		ClaimKind:   "direct_static_call",
		SupportMode: "resolved_static",
		Label:       "handoff",
		Evidence:    "go_ssa behavior handoff",
		Scenario:    "Recorded Go build scenario",
		Limitation:  "runtime dispatch beyond the recorded build scenario not proven",
		Ordering:    "resolved_path_order",
	}
	if witness.Location != nil {
		transition.Path = witness.Location.Path
		transition.Line = witness.Location.Line
	}
	if len(witness.Provenance) > 0 {
		prov := witness.Provenance[0]
		transition.Evidence = prov.Provider + " " + prov.Version + " " + prov.Operation
	}
	if len(witness.Scenarios) > 0 {
		transition.Scenario = witness.Scenarios[0].Name
	}
	transition.Symbol = witness.To.Value
	return transition
}
