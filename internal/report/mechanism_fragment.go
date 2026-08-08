package report

// MechanismFragment (Decision 226) is one honest first-hop fan-out:
// process entry → locally supported direct handoffs → explicit unresolved
// frontier. It is not a path and is built
// entirely from saved local evidence (canvas behavior-handoff edges,
// behavior anchors, and exact entry handoffs); no edge is invented to make a
// continuous path, and a disconnected fragment with an explicit frontier
// is a successful honest result.
import (
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
)

// MechanismFragmentVersion changes when the persisted fragment projection
// shape or its exact derivation rules change.
// Decision 239 (v2): exact producer-owned entry handoffs remain outside the
// canonical Canvas but participate in this separate mechanism projection.
// Decision 242 (v3): every exact process entry owns one stable fragment and
// exact backend joins publish sorted component participation inside the Canvas.
const (
	MechanismFragmentVersion = 3

	// These output ceilings mirror the validated upstream evidence ceilings:
	// at most 256 behavior anchors, 1,024 local relations, and 256 exact entry
	// handoffs. Exceeding them is a fail-closed contract error, never silent
	// truncation or a partial mechanism projection.
	maxMechanismFragments           = 256
	maxMechanismHandoffsPerFragment = 1_280
	maxMechanismHandoffsTotal       = 1_280
)

// MechanismFragmentProjection is the bounded provider-free first-hop fan-out
// for one process entry with at least one exact supported handoff.
type MechanismFragmentProjection struct {
	Version      int                        `json:"version"`
	ID           string                     `json:"id"`
	ComponentIDs []componentmap.ComponentID `json:"component_ids"`
	Entry        MechanismTransition        `json:"entry"`
	Handoffs     []MechanismTransition      `json:"handoffs"`
	Frontier     MechanismFrontier          `json:"frontier"`
}

// MechanismTransition is one contract-carrying transition (Decision 226).
type MechanismTransition struct {
	Ordinal      int                     `json:"ordinal"`
	ClaimKind    string                  `json:"claim_kind"`
	SupportMode  string                  `json:"support_mode"`
	Label        string                  `json:"label"`
	Path         string                  `json:"path"`
	Line         int                     `json:"line"`
	Column       int                     `json:"column,omitempty"`
	Symbol       string                  `json:"symbol,omitempty"`
	WitnessCount int                     `json:"witness_count,omitempty"`
	Evidence     string                  `json:"evidence"` // provenance provider/version/operation
	Scenario     string                  `json:"scenario,omitempty"`
	Limitation   string                  `json:"limitation"`
	Ordering     string                  `json:"ordering"`
	Target       *MechanismHandoffTarget `json:"target,omitempty"`
}

// MechanismHandoffTarget is the exact producer-owned callee declaration for a
// first-hop handoff. Symbol is published only when that same declaration
// identity and location join an accepted non-remainder Canvas member.
type MechanismHandoffTarget struct {
	Label  string `json:"label"`
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column,omitempty"`
	Symbol string `json:"symbol,omitempty"`
}

// MechanismFrontier states what is NOT locally supported.
type MechanismFrontier struct {
	Ordering   string   `json:"ordering"` // not_established | partial_order
	Unresolved []string `json:"unresolved,omitempty"`
	Limitation string   `json:"limitation"`
}

// ProjectMechanismFragments derives one honest first-hop fan-out per exact
// process entry that has a supported handoff. Provider-free and deterministic:
// entries are exact process_entry anchors; transitions are behavior_handoff
// edges whose from member matches that entry. Boundary/resource rows remain
// exclusively owned by the Integrations projection; everything beyond direct
// handoffs is an explicit frontier.
func ProjectMechanismFragments(
	canvas *ArchitectureCanvas,
) ([]MechanismFragmentProjection, error) {
	return projectMechanismFragments(canvas, nil)
}

func projectMechanismFragments(
	canvas *ArchitectureCanvas,
	grounding *ArchitectureGrounding,
) ([]MechanismFragmentProjection, error) {
	return projectMechanismFragmentsForProduct(canvas, grounding)
}

func projectMechanismFragmentsForProduct(
	canvas *ArchitectureCanvas,
	grounding *ArchitectureGrounding,
) ([]MechanismFragmentProjection, error) {
	if canvas == nil {
		return nil, nil
	}
	if canvas.Version != ArchitectureCanvasVersion {
		return nil, fmt.Errorf("mechanism fragment: unsupported canvas version %d", canvas.Version)
	}

	entryAnchors, err := processEntryAnchors(canvas)
	if err != nil {
		return nil, err
	}
	if len(entryAnchors) > maxMechanismFragments {
		return nil, fmt.Errorf(
			"mechanism fragment: process entry count %d exceeds limit %d",
			len(entryAnchors),
			maxMechanismFragments,
		)
	}
	fragments := make([]MechanismFragmentProjection, 0, len(entryAnchors))
	fragmentIDs := make(map[string]struct{}, len(entryAnchors))
	totalHandoffs := 0
	for _, entryAnchor := range entryAnchors {
		fragment, err := projectMechanismFragmentForEntry(canvas, grounding, entryAnchor)
		if err != nil {
			return nil, err
		}
		if fragment == nil {
			continue
		}
		if _, duplicate := fragmentIDs[fragment.ID]; duplicate {
			return nil, fmt.Errorf("mechanism fragment: duplicate exact entry identity %q", fragment.ID)
		}
		fragmentIDs[fragment.ID] = struct{}{}
		totalHandoffs += len(fragment.Handoffs)
		if totalHandoffs > maxMechanismHandoffsTotal {
			return nil, fmt.Errorf(
				"mechanism fragment: total handoff count %d exceeds limit %d",
				totalHandoffs,
				maxMechanismHandoffsTotal,
			)
		}
		fragments = append(fragments, *fragment)
	}
	if len(fragments) == 0 {
		return nil, nil
	}
	sort.Slice(fragments, func(i, j int) bool { return fragments[i].ID < fragments[j].ID })
	return fragments, nil
}

type mechanismHandoffTarget struct {
	transition   MechanismTransition
	componentIDs []componentmap.ComponentID
}

func projectMechanismFragmentForEntry(
	canvas *ArchitectureCanvas,
	grounding *ArchitectureGrounding,
	entryAnchor *componentmap.BehaviorAnchor,
) (*MechanismFragmentProjection, error) {
	entryMembers := processEntryMemberIDs(entryAnchor)
	if len(entryMembers) == 0 {
		return nil, nil
	}

	handoffs := []MechanismTransition{}
	frontier := MechanismFrontier{
		Ordering:   "not_established",
		Unresolved: []string{"continuation beyond the first-hop handoffs"},
		Limitation: "No further transition is locally proven to continue from these first-hop handoffs; execution order beyond them is not established.",
	}
	// Exact D210 handoffs are the authoritative first-hop projection. Record
	// their callsites before consulting the older Canvas relation store so the
	// same callsite is not published twice under two different target labels.
	var handoffTargets []mechanismHandoffTarget
	entryHandoffCallsites := make(map[string]struct{})
	if grounding != nil {
		for _, handoff := range grounding.EntryHandoffs {
			if handoff.Scenario.ID != entryAnchor.Scenario.ID ||
				!architectureDeclarationLocationsCompatible(handoff.ProcessEntrypoint.Location, entryAnchor.Location) ||
				!mechanismEntryHandoffMatchesAnchor(canvas, entryAnchor, handoff) {
				continue
			}
			componentIDs := componentIDsForEntryHandoffCallee(canvas, handoff)
			transition := transitionFromEntryHandoff(handoff, len(componentIDs) > 0)
			handoffTargets = append(handoffTargets, mechanismHandoffTarget{
				transition:   transition,
				componentIDs: componentIDs,
			})
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
		if !containsMechanismMemberID(entryMembers, witness.From) ||
			!mechanismRelationHasScenario(witness, entryAnchor.Scenario.ID) {
			continue
		}
		transition := transitionFromWitness(canvas, edge, witness, entryAnchor.Scenario.ID)
		if _, exactEntryHandoff := entryHandoffCallsites[mechanismCallsiteKey(transition.Path, transition.Line, transition.Column)]; exactEntryHandoff {
			continue
		}
		handoffTargets = append(handoffTargets, mechanismHandoffTarget{
			transition:   transition,
			componentIDs: componentIDsForMemberID(canvas, witness.To),
		})
	}
	if len(handoffTargets) > maxMechanismHandoffsPerFragment {
		return nil, fmt.Errorf(
			"mechanism fragment: entry %q handoff count %d exceeds limit %d",
			entryAnchor.ID,
			len(handoffTargets),
			maxMechanismHandoffsPerFragment,
		)
	}
	handoffTargets = compactMechanismHandoffTargets(handoffTargets)
	if len(handoffTargets) == 0 {
		// Zero-hop process entries remain authoritative Entrypoints, but are not
		// promoted into fake Mechanisms without an exact first-hop handoff.
		return nil, nil
	}
	sort.Slice(handoffTargets, func(i, j int) bool {
		left := handoffTargets[i].transition
		right := handoffTargets[j].transition
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.Line != right.Line {
			return left.Line < right.Line
		}
		if left.Column != right.Column {
			return left.Column < right.Column
		}
		if left.Symbol != right.Symbol {
			return left.Symbol < right.Symbol
		}
		if left.Scenario != right.Scenario {
			return left.Scenario < right.Scenario
		}
		leftTarget := mechanismHandoffTargetKey(left.Target)
		rightTarget := mechanismHandoffTargetKey(right.Target)
		if leftTarget != rightTarget {
			return leftTarget < rightTarget
		}
		if left.Label != right.Label {
			return left.Label < right.Label
		}
		return left.Evidence < right.Evidence
	})
	componentIDs := componentIDsForAnchor(canvas, entryAnchor)
	componentSet := make(map[componentmap.ComponentID]struct{}, len(componentIDs))
	for _, componentID := range componentIDs {
		componentSet[componentID] = struct{}{}
	}
	for index := range handoffTargets {
		target := &handoffTargets[index]
		target.transition.Ordinal = index + 1
		handoffs = append(handoffs, target.transition)
		if len(target.componentIDs) == 0 {
			continue
		}
		for _, componentID := range target.componentIDs {
			componentSet[componentID] = struct{}{}
		}
	}
	// Entry: a process entry is a declaration/entry anchor — claim_kind
	// process_entry, support_mode resolved_static, and evidence derived from
	// the anchor's actual process_entry proof mode. The label is derived only
	// from an exact declaration fact, never from the presentation-localizable
	// anchor label; otherwise localization would make this persisted projection
	// drift from the same local evidence. Symbol carries that readable exact
	// declaration only when the exact anchor has one member; plural membership is
	// represented without selecting a hidden representative.
	entrySymbol := ""
	if len(entryMembers) == 1 {
		entrySymbol = exactMechanismEntrySymbol(canvas, entryMembers[0], entryAnchor.Location)
	}
	entryLabel := "process entry"
	if entrySymbol != "" {
		entryLabel += " " + entrySymbol
	}
	entry := MechanismTransition{
		Ordinal:     0,
		ClaimKind:   "process_entry",
		SupportMode: "resolved_static",
		Label:       entryLabel,
		Path:        entryAnchor.Location.Path,
		Line:        entryAnchor.Location.Line,
		Symbol:      entrySymbol,
		Evidence:    "behavior anchor proof_mode " + string(entryAnchor.ProofMode),
		Scenario:    entryAnchor.Scenario.ID,
		Limitation:  "process entry identity only; runtime reachability not proven",
		Ordering:    "exact_local_order",
	}
	componentIDs = componentIDs[:0]
	for componentID := range componentSet {
		componentIDs = append(componentIDs, componentID)
	}
	sort.Slice(componentIDs, func(i, j int) bool { return componentIDs[i] < componentIDs[j] })
	return &MechanismFragmentProjection{
		Version:      MechanismFragmentVersion,
		ID:           mechanismFragmentID(entryAnchor),
		ComponentIDs: componentIDs,
		Entry:        entry,
		Handoffs:     handoffs,
		Frontier:     frontier,
	}, nil
}

func mechanismEntryHandoffMatchesAnchor(
	canvas *ArchitectureCanvas,
	entryAnchor *componentmap.BehaviorAnchor,
	handoff ArchitectureEntryHandoff,
) bool {
	if entryAnchor == nil || handoff.ProcessEntrypoint.ID == "" {
		return false
	}
	for _, memberID := range processEntryMemberIDs(entryAnchor) {
		if exactMechanismEntrySymbol(canvas, memberID, entryAnchor.Location) ==
			handoff.ProcessEntrypoint.ID {
			return true
		}
	}
	return false
}

// exactMechanismEntrySymbol restores readable entry identity only from one
// exact declaration fact at the process-entry location. Member IDs are opaque
// local join keys and must never become product copy.
func exactMechanismEntrySymbol(
	canvas *ArchitectureCanvas,
	memberID componentmap.MemberID,
	entryLocation evidence.Location,
) string {
	if canvas == nil || memberID.Kind != componentmap.MemberSymbol || memberID.Value == "" {
		return ""
	}
	result := ""
	for _, component := range canvas.Components {
		if canvas.LocalRemainderComponentID != "" && component.ID == canvas.LocalRemainderComponentID {
			continue
		}
		for _, members := range [][]componentmap.Candidate{component.Members, component.SharedMembers} {
			for _, member := range members {
				if !memberIDEquals(member.ID, memberID) {
					continue
				}
				for _, fact := range member.Facts {
					if fact.Kind != componentmap.FactDeclaration || fact.Value == "" ||
						fact.Location == nil ||
						!architectureDeclarationLocationsCompatible(*fact.Location, entryLocation) {
						continue
					}
					if result == "" {
						result = fact.Value
						continue
					}
					if fact.Value != result {
						return ""
					}
				}
			}
		}
	}
	return result
}

func architectureDeclarationLocationsCompatible(left, right evidence.Location) bool {
	if left.Path != right.Path || left.Line != right.Line {
		return false
	}
	return left.Column == 0 || right.Column == 0 || left.Column == right.Column
}

func transitionFromEntryHandoff(
	handoff ArchitectureEntryHandoff,
	locallyAdmissibleSymbol bool,
) MechanismTransition {
	target := &MechanismHandoffTarget{
		Label:  handoff.Callee.Name,
		Path:   handoff.Callee.Location.Path,
		Line:   handoff.Callee.Location.Line,
		Column: handoff.Callee.Location.Column,
	}
	if locallyAdmissibleSymbol {
		target.Symbol = handoff.Callee.ID
	}
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
		Target:       target,
	}
	if len(handoff.Limitations) > 0 {
		transition.Limitation = handoff.Limitations[0]
	}
	return transition
}

func compactMechanismHandoffTargets(targets []mechanismHandoffTarget) []mechanismHandoffTarget {
	seen := make(map[string]int, len(targets))
	result := targets[:0]
	for _, target := range targets {
		transition := target.transition
		key := fmt.Sprintf(
			"%s\x00%s\x00%d\x00%d\x00%s\x00%s\x00%s",
			transition.ClaimKind,
			transition.Path,
			transition.Line,
			transition.Column,
			transition.Symbol,
			transition.Scenario,
			mechanismHandoffTargetKey(transition.Target),
		)
		if existing, duplicate := seen[key]; duplicate {
			if transition.WitnessCount > result[existing].transition.WitnessCount {
				result[existing].transition.WitnessCount = transition.WitnessCount
			}
			result[existing].componentIDs = mergeMechanismComponentIDs(
				result[existing].componentIDs,
				target.componentIDs,
			)
			continue
		}
		seen[key] = len(result)
		target.componentIDs = mergeMechanismComponentIDs(nil, target.componentIDs)
		result = append(result, target)
	}
	return result
}

func mechanismHandoffTargetKey(target *MechanismHandoffTarget) string {
	if target == nil {
		return ""
	}
	return fmt.Sprintf(
		"%s\x00%s\x00%d\x00%d\x00%s",
		target.Label,
		target.Path,
		target.Line,
		target.Column,
		target.Symbol,
	)
}

func mergeMechanismComponentIDs(
	left, right []componentmap.ComponentID,
) []componentmap.ComponentID {
	seen := make(map[componentmap.ComponentID]struct{}, len(left)+len(right))
	for _, values := range [][]componentmap.ComponentID{left, right} {
		for _, value := range values {
			if value != "" {
				seen[value] = struct{}{}
			}
		}
	}
	result := make([]componentmap.ComponentID, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func mechanismCallsiteKey(path string, line, column int) string {
	return fmt.Sprintf("%s\x00%d\x00%d", path, line, column)
}

func processEntryAnchors(canvas *ArchitectureCanvas) ([]*componentmap.BehaviorAnchor, error) {
	if canvas == nil {
		return nil, nil
	}
	result := make([]*componentmap.BehaviorAnchor, 0)
	for index := range canvas.BehaviorAnchors {
		anchor := &canvas.BehaviorAnchors[index]
		if anchor.Kind != componentmap.AnchorProcessEntry {
			continue
		}
		entryMembers := processEntryMemberIDs(anchor)
		if anchor.ProofMode != componentmap.AnchorProofProcessEntry ||
			anchor.ID == "" || anchor.Scenario.ID == "" ||
			!validGroundingLocation(anchor.Location) ||
			len(entryMembers) == 0 || len(entryMembers) != len(anchor.MemberIDs) {
			return nil, fmt.Errorf(
				"mechanism fragment: process entry anchor %q is not exact",
				anchor.ID,
			)
		}
		result = append(result, anchor)
	}
	sort.Slice(result, func(i, j int) bool {
		left := result[i]
		right := result[j]
		if left.Location.Path != right.Location.Path {
			return left.Location.Path < right.Location.Path
		}
		if left.Location.Line != right.Location.Line {
			return left.Location.Line < right.Location.Line
		}
		if left.Location.Column != right.Location.Column {
			return left.Location.Column < right.Location.Column
		}
		return left.ID < right.ID
	})
	return result, nil
}

func processEntryMemberIDs(anchor *componentmap.BehaviorAnchor) []componentmap.MemberID {
	if anchor == nil {
		return nil
	}
	seen := make(map[componentmap.MemberID]struct{}, len(anchor.MemberIDs))
	for _, memberID := range anchor.MemberIDs {
		if memberID.Kind != componentmap.MemberSymbol || memberID.Value == "" {
			continue
		}
		seen[memberID] = struct{}{}
	}
	result := make([]componentmap.MemberID, 0, len(seen))
	for memberID := range seen {
		result = append(result, memberID)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind != result[j].Kind {
			return result[i].Kind < result[j].Kind
		}
		return result[i].Value < result[j].Value
	})
	return result
}

func memberIDEquals(left, right componentmap.MemberID) bool {
	return left.Kind == right.Kind && left.Value == right.Value
}

func containsMechanismMemberID(values []componentmap.MemberID, target componentmap.MemberID) bool {
	for _, value := range values {
		if memberIDEquals(value, target) {
			return true
		}
	}
	return false
}

func mechanismRelationHasScenario(relation componentmap.LocalRelation, scenarioID string) bool {
	if scenarioID == "" {
		return false
	}
	for _, scenario := range relation.Scenarios {
		if scenario.ID == scenarioID {
			return true
		}
	}
	return false
}

func componentIDsForAnchor(
	canvas *ArchitectureCanvas,
	anchor *componentmap.BehaviorAnchor,
) []componentmap.ComponentID {
	if anchor == nil {
		return []componentmap.ComponentID{}
	}
	var result []componentmap.ComponentID
	for _, memberID := range anchor.MemberIDs {
		result = mergeMechanismComponentIDs(result, componentIDsForMemberID(canvas, memberID))
	}
	if result == nil {
		return []componentmap.ComponentID{}
	}
	return result
}

func componentIDsForMemberID(
	canvas *ArchitectureCanvas,
	memberID componentmap.MemberID,
) []componentmap.ComponentID {
	if canvas == nil || memberID.Value == "" {
		return nil
	}
	var result []componentmap.ComponentID
	for _, component := range canvas.Components {
		if canvas.LocalRemainderComponentID != "" && component.ID == canvas.LocalRemainderComponentID {
			continue
		}
		for _, members := range [][]componentmap.Candidate{component.Members, component.SharedMembers} {
			matched := false
			for _, member := range members {
				if memberIDEquals(member.ID, memberID) {
					matched = true
					break
				}
			}
			if matched {
				result = append(result, component.ID)
				break
			}
		}
	}
	return mergeMechanismComponentIDs(nil, result)
}

func componentIDsForEntryHandoffCallee(
	canvas *ArchitectureCanvas,
	handoff ArchitectureEntryHandoff,
) []componentmap.ComponentID {
	if canvas == nil || handoff.Callee.ID == "" || handoff.Callee.Location.Path == "" {
		return nil
	}
	var result []componentmap.ComponentID
	for _, component := range canvas.Components {
		if canvas.LocalRemainderComponentID != "" && component.ID == canvas.LocalRemainderComponentID {
			continue
		}
		matched := false
		for _, members := range [][]componentmap.Candidate{component.Members, component.SharedMembers} {
			for _, member := range members {
				if exactMechanismDeclarationMember(member, handoff.Callee) {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if matched {
			result = append(result, component.ID)
		}
	}
	return mergeMechanismComponentIDs(nil, result)
}

func exactMechanismDeclarationMember(
	candidate componentmap.Candidate,
	callee ArchitectureAnchorMember,
) bool {
	if candidate.ID.Kind != componentmap.MemberSymbol {
		return false
	}
	for _, fact := range candidate.Facts {
		if fact.Kind != componentmap.FactDeclaration || fact.Value != callee.ID ||
			fact.Location == nil {
			continue
		}
		if architectureDeclarationLocationsCompatible(*fact.Location, callee.Location) {
			return true
		}
	}
	return false
}

func mechanismFragmentID(anchor *componentmap.BehaviorAnchor) string {
	memberKeys := make([]string, 0, len(anchor.MemberIDs))
	for _, memberID := range anchor.MemberIDs {
		memberKeys = append(memberKeys, string(memberID.Kind)+"\x00"+memberID.Value)
	}
	sort.Strings(memberKeys)
	parts := []string{
		anchor.ID,
		anchor.Location.Path,
		fmt.Sprintf("%d:%d", anchor.Location.Line, anchor.Location.Column),
		anchor.Scenario.ID,
	}
	parts = append(parts, memberKeys...)
	return architectureStableID("mechanism-fragment", parts...)
}

func transitionFromWitness(
	canvas *ArchitectureCanvas,
	edge ArchitectureStructuralEdge,
	witness componentmap.LocalRelation,
	scenarioID string,
) MechanismTransition {
	transition := MechanismTransition{
		ClaimKind:   "direct_static_call",
		SupportMode: "resolved_static",
		Label:       "handoff",
		Evidence:    "go_ssa behavior handoff",
		Scenario:    scenarioID,
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
	transition.Symbol = witness.To.Value
	transition.Target = exactMechanismTargetForMemberID(canvas, witness.To)
	if transition.Target != nil {
		transition.Label = "handoff to " + transition.Target.Label
	}
	return transition
}

// exactMechanismTargetForMemberID restores a Canvas handoff target only when
// the accepted non-remainder component membership owns one unambiguous exact
// declaration for the witness target. It never selects among locations or
// presentation labels.
func exactMechanismTargetForMemberID(
	canvas *ArchitectureCanvas,
	memberID componentmap.MemberID,
) *MechanismHandoffTarget {
	if canvas == nil || memberID.Kind != componentmap.MemberSymbol || memberID.Value == "" {
		return nil
	}
	var result *MechanismHandoffTarget
	resultKey := ""
	for _, component := range canvas.Components {
		if canvas.LocalRemainderComponentID != "" && component.ID == canvas.LocalRemainderComponentID {
			continue
		}
		for _, members := range [][]componentmap.Candidate{component.Members, component.SharedMembers} {
			for _, member := range members {
				if !memberIDEquals(member.ID, memberID) {
					continue
				}
				for _, fact := range member.Facts {
					if fact.Kind != componentmap.FactDeclaration || fact.Value == "" ||
						fact.Location == nil || !validGroundingLocation(*fact.Location) {
						continue
					}
					candidate := &MechanismHandoffTarget{
						Label:  fact.Value,
						Path:   fact.Location.Path,
						Line:   fact.Location.Line,
						Column: fact.Location.Column,
						Symbol: fact.Value,
					}
					key := mechanismHandoffTargetKey(candidate)
					if result == nil {
						result, resultKey = candidate, key
						continue
					}
					if key != resultKey {
						return nil
					}
				}
			}
		}
	}
	return result
}
