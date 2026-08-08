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
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

// MechanismFragmentVersion changes when the persisted fragment projection
// shape or its exact derivation rules change.
// Decision 239 (v2): exact producer-owned entry handoffs remain outside the
// canonical Canvas but participate in this separate mechanism projection.
const MechanismFragmentVersion = 2

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
	Ordinal      int    `json:"ordinal"`
	ClaimKind    string `json:"claim_kind"`
	SupportMode  string `json:"support_mode"`
	Label        string `json:"label"`
	Path         string `json:"path"`
	Line         int    `json:"line"`
	Column       int    `json:"column,omitempty"`
	Symbol       string `json:"symbol,omitempty"`
	WitnessCount int    `json:"witness_count,omitempty"`
	Evidence     string `json:"evidence"` // provenance provider/version/operation
	Scenario     string `json:"scenario,omitempty"`
	Limitation   string `json:"limitation"`
	Ordering     string `json:"ordering"`
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
	return projectMechanismFragment(canvas, associations, atlas, nil)
}

func projectMechanismFragment(
	canvas *ArchitectureCanvas,
	associations *ArchitectureAssociationProjection,
	atlas *repositoryatlas.Atlas,
	grounding *ArchitectureGrounding,
) (*MechanismFragmentProjection, error) {
	return projectMechanismFragmentForProduct(canvas, associations, atlas, grounding, nil)
}

func projectMechanismFragmentForProduct(
	canvas *ArchitectureCanvas,
	associations *ArchitectureAssociationProjection,
	atlas *repositoryatlas.Atlas,
	grounding *ArchitectureGrounding,
	navigatorProduct *NavigatorReportProduct,
) (*MechanismFragmentProjection, error) {
	if canvas == nil {
		return nil, nil
	}
	if canvas.Version != ArchitectureCanvasVersion {
		return nil, fmt.Errorf("mechanism fragment: unsupported canvas version %d", canvas.Version)
	}

	// Exact entry: when ordinary Navigator selected a startup, use its
	// backend-owned evidence to select the matching process-entry anchor.
	// Falling back to a different entry would make the product's horizontal
	// and vertical views disagree. Runs without a selected Navigator retain the
	// historical deterministic first-entry behavior.
	entryAnchor := findProcessEntryAnchor(canvas, navigatorProduct, atlas)
	if entryAnchor == nil {
		return nil, nil
	}
	entryMember := symbolMemberID(entryAnchor)
	if entryMember.Value == "" {
		return nil, nil
	}

	transitions := []MechanismTransition{}
	frontier := MechanismFrontier{Ordering: "not_established", Limitation: "No locally saved transitions beyond the observed handoffs; execution order beyond them is not established."}
	// Exact D210 handoffs are the authoritative first-hop projection. Record
	// their callsites before consulting the older Canvas relation store so the
	// same callsite is not published twice under two different target labels.
	var handoffTargets []MechanismTransition
	entryHandoffCallsites := make(map[string]struct{})
	if grounding != nil {
		for _, handoff := range grounding.EntryHandoffs {
			if !architectureLocationsEqual(handoff.ProcessEntrypoint.Location, entryAnchor.Location) {
				continue
			}
			transition := transitionFromEntryHandoff(handoff)
			handoffTargets = append(handoffTargets, transition)
			entryHandoffCallsites[mechanismCallsiteKey(transition.Path, transition.Line, transition.Column)] = struct{}{}
		}
	}
	// Behavior-handoff edges from the entry symbol (exact, SSA-supported).
	// Ordinals are assigned AFTER sorting so wire order is 1..N.
	for _, edge := range canvas.StructuralEdges {
		witness := edge.Witness
		if witness.Kind != componentmap.StructuralRelationBehaviorHandoff {
			continue
		}
		if !memberIDEquals(witness.From, entryMember) {
			continue
		}
		transition := transitionFromWitness(edge, witness)
		if _, exactEntryHandoff := entryHandoffCallsites[mechanismCallsiteKey(transition.Path, transition.Line, transition.Column)]; exactEntryHandoff {
			continue
		}
		handoffTargets = append(handoffTargets, transition)
	}
	handoffTargets = compactMechanismTransitions(handoffTargets)
	sort.Slice(handoffTargets, func(i, j int) bool {
		if handoffTargets[i].Path != handoffTargets[j].Path {
			return handoffTargets[i].Path < handoffTargets[j].Path
		}
		if handoffTargets[i].Line != handoffTargets[j].Line {
			return handoffTargets[i].Line < handoffTargets[j].Line
		}
		if handoffTargets[i].Column != handoffTargets[j].Column {
			return handoffTargets[i].Column < handoffTargets[j].Column
		}
		return handoffTargets[i].Symbol < handoffTargets[j].Symbol
	})
	for index := range handoffTargets {
		handoffTargets[index].Ordinal = index + 1
	}
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

	// Entry: a process entry is a declaration/entry anchor — claim_kind
	// process_entry, support_mode resolved_static, and evidence derived from
	// the anchor's actual proof mode (process_entry or call_target), never a
	// hardcoded call_target. The label keeps the anchor label; the Symbol
	// carries the bare entry identity (member id), consistent with
	// transitions.
	entrySymbol := entryMember.Value
	entry := MechanismTransition{
		Ordinal:     0,
		ClaimKind:   "process_entry",
		SupportMode: "resolved_static",
		Label:       entryAnchor.Label,
		Path:        entryAnchor.Location.Path,
		Line:        entryAnchor.Location.Line,
		Symbol:      entrySymbol,
		Evidence:    "behavior anchor proof_mode " + string(entryAnchor.ProofMode),
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

func architectureLocationsEqual(left, right evidence.Location) bool {
	return left.Path == right.Path && left.Line == right.Line && left.Column == right.Column
}

func transitionFromEntryHandoff(handoff ArchitectureEntryHandoff) MechanismTransition {
	transition := MechanismTransition{
		ClaimKind:    "direct_static_call",
		SupportMode:  "resolved_static",
		Label:        "handoff to " + handoff.Callee.Name,
		Path:         handoff.RepresentativeCallsite.Path,
		Line:         handoff.RepresentativeCallsite.Line,
		Column:       handoff.RepresentativeCallsite.Column,
		Symbol:       handoff.Callee.Name,
		WitnessCount: max(handoff.WitnessCount, 1),
		Evidence:     handoff.Producer.Provider + " " + handoff.Producer.Version + " " + handoff.Producer.Operation,
		Scenario:     handoff.Scenario.ID,
		Limitation:   architectureEntryHandoffLimitation,
		Ordering:     "resolved_path_order",
	}
	if len(handoff.Limitations) > 0 {
		transition.Limitation = handoff.Limitations[0]
	}
	return transition
}

func compactMechanismTransitions(transitions []MechanismTransition) []MechanismTransition {
	seen := make(map[string]int, len(transitions))
	result := transitions[:0]
	for _, transition := range transitions {
		key := fmt.Sprintf(
			"%s\x00%s\x00%d\x00%d\x00%s",
			transition.ClaimKind,
			transition.Path,
			transition.Line,
			transition.Column,
			transition.Symbol,
		)
		if existing, duplicate := seen[key]; duplicate {
			if transition.WitnessCount > result[existing].WitnessCount {
				result[existing].WitnessCount = transition.WitnessCount
			}
			continue
		}
		seen[key] = len(result)
		result = append(result, transition)
	}
	return result
}

func mechanismCallsiteKey(path string, line, column int) string {
	return fmt.Sprintf("%s\x00%d\x00%d", path, line, column)
}

func mechanismClaimKindForRow(row ArchitectureBoundaryResourceRow) string {
	if row.Kind == "boundary" {
		return "storage_boundary_callsite"
	}
	return "outbound_client_callsite"
}

func findProcessEntryAnchor(
	canvas *ArchitectureCanvas,
	navigatorProduct *NavigatorReportProduct,
	atlas *repositoryatlas.Atlas,
) *componentmap.BehaviorAnchor {
	if navigatorProduct != nil && navigatorProduct.Recommendation != nil {
		if atlas == nil {
			return nil
		}
		selectedEvidence := make(map[string]struct{}, len(navigatorProduct.Recommendation.EvidenceIDs))
		for _, evidenceID := range navigatorProduct.Recommendation.EvidenceIDs {
			selectedEvidence[evidenceID] = struct{}{}
		}
		selectedLocations := make([]evidence.Location, 0, len(selectedEvidence))
		for _, item := range atlas.Evidence {
			if _, selected := selectedEvidence[item.ID]; selected {
				selectedLocations = append(selectedLocations, item.Location)
			}
		}
		for index := range canvas.BehaviorAnchors {
			anchor := &canvas.BehaviorAnchors[index]
			if anchor.Kind != componentmap.AnchorProcessEntry || anchor.Location.Path == "" {
				continue
			}
			for _, location := range selectedLocations {
				if architectureLocationsEqual(anchor.Location, location) {
					return anchor
				}
			}
		}
		return nil
	}
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
) MechanismTransition {
	transition := MechanismTransition{
		ClaimKind:   "direct_static_call",
		SupportMode: "resolved_static",
		Label:       "handoff",
		Evidence:    "go_ssa behavior handoff",
		Scenario:    "Recorded Go build scenario",
		Limitation:  "runtime dispatch beyond the recorded build scenario not proven",
		Ordering:    "resolved_path_order",
	}
	transition.WitnessCount = edge.WitnessCount
	if transition.WitnessCount == 0 {
		transition.WitnessCount = len(edge.WitnessIDs)
	}
	if transition.WitnessCount == 0 {
		transition.WitnessCount = 1
	}
	if witness.Location != nil {
		transition.Path = witness.Location.Path
		transition.Line = witness.Location.Line
		transition.Column = witness.Location.Column
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
