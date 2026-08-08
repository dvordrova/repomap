package report

// EntrypointHandoffGroup is one honest first-hop fan-out:
// process entry → locally supported direct handoffs → explicit unresolved
// frontier. It is not a path and is built entirely from exact D210 entry
// handoffs matched to Canvas process-entry declarations; generic Canvas
// behavior relationships are not direct-entry evidence. No edge is invented
// to make a continuous path, and a disconnected group with an explicit
// frontier is a successful honest result.
import (
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
)

// EntrypointHandoffGroupVersion changes when the persisted group projection
// shape or its exact derivation rules change.
// v1: exact D210 entry handoffs are the sole handoff authority. Every
// transition carries typed producer provenance and an exact relation ref;
// generic Canvas behavior relationships cannot enter this projection.
// v2: an otherwise unjoined exact callee declaration may join through the
// exact RepositoryGraph package whose file inventory contains that declaration
// path, then only to accepted non-remainder Canvas members for that exact
// package. The declaration-member join remains authoritative when present.
const (
	EntrypointHandoffGroupVersion     = 2
	entrypointEvidenceRefEntryHandoff = "architecture_entry_handoff"

	// These output ceilings mirror the validated upstream evidence ceilings:
	// at most 256 behavior anchors, 1,024 local relations, and 256 exact entry
	// handoffs. Exceeding them is a fail-closed contract error, never silent
	// truncation or a partial entry-handoff projection.
	maxEntrypointHandoffGroups    = 256
	maxEntrypointHandoffsPerGroup = 1_280
	maxEntrypointHandoffsTotal    = 1_280
)

// EntrypointHandoffGroup is the bounded provider-free first-hop fan-out
// for one process entry with at least one exact supported handoff.
type EntrypointHandoffGroup struct {
	Version       int                        `json:"version"`
	ID            string                     `json:"id"`
	ComponentIDs  []componentmap.ComponentID `json:"component_ids"`
	Entry         EntrypointTransition       `json:"entry"`
	EntryHandoffs []EntrypointTransition     `json:"entry_handoffs"`
	Frontier      EntrypointFrontier         `json:"frontier"`
}

// EntrypointTransition is one contract-carrying transition (Decision 226).
type EntrypointTransition struct {
	Ordinal      int                        `json:"ordinal"`
	ClaimKind    string                     `json:"claim_kind"`
	SupportMode  string                     `json:"support_mode"`
	Label        string                     `json:"label"`
	Path         string                     `json:"path"`
	Line         int                        `json:"line"`
	Column       int                        `json:"column,omitempty"`
	Symbol       string                     `json:"symbol,omitempty"`
	ComponentIDs []componentmap.ComponentID `json:"component_ids"`
	WitnessCount int                        `json:"witness_count,omitempty"`
	Evidence     string                     `json:"evidence"` // presentation copy; EvidenceRef + Provenance are handoff authority
	EvidenceRef  *EntrypointEvidenceRef     `json:"evidence_ref,omitempty"`
	Provenance   *EntrypointProvenance      `json:"provenance,omitempty"`
	Scenario     string                     `json:"scenario,omitempty"`
	Limitation   string                     `json:"limitation"`
	Ordering     string                     `json:"ordering"`
	Target       *EntrypointHandoffTarget   `json:"target,omitempty"`
}

// EntrypointEvidenceRef identifies the exact saved relation from which a
// handoff was projected. Kind is a closed backend value; ID is the D210
// ArchitectureEntryHandoff ID, never a model-authored or client-supplied join.
type EntrypointEvidenceRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// EntrypointProvenance is the typed, public-safe subset of the validated D210
// producer record. Detail and source location cannot enter this projection.
type EntrypointProvenance struct {
	Provider  string `json:"provider"`
	Version   string `json:"version,omitempty"`
	Operation string `json:"operation"`
}

// EntrypointHandoffTarget is the exact producer-owned callee declaration for a
// first-hop handoff. Symbol is published only when the exact declaration joins
// an accepted non-remainder Canvas member directly or through the exact
// RepositoryGraph package member that owns its declaration file.
type EntrypointHandoffTarget struct {
	Label  string `json:"label"`
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column,omitempty"`
	Symbol string `json:"symbol,omitempty"`
}

// EntrypointFrontier states what is NOT locally supported.
type EntrypointFrontier struct {
	Ordering   string   `json:"ordering"` // not_established | partial_order
	Unresolved []string `json:"unresolved,omitempty"`
	Limitation string   `json:"limitation"`
}

// ProjectEntrypointHandoffGroups derives one honest first-hop fan-out per exact
// process entry that has a supported handoff. Provider-free and deterministic:
// entries are exact process_entry anchors; transitions are exact D210 entry
// handoffs whose process-entry ID, declaration location and scenario match that
// anchor. Boundary/resource rows remain exclusively owned by the Integrations
// projection; everything beyond direct handoffs is an explicit frontier.
func ProjectEntrypointHandoffGroups(
	canvas *ArchitectureCanvas,
	grounding *ArchitectureGrounding,
	repositoryGraph *RepositoryGraph,
) ([]EntrypointHandoffGroup, error) {
	return projectEntrypointHandoffGroups(canvas, grounding, repositoryGraph)
}

func projectEntrypointHandoffGroups(
	canvas *ArchitectureCanvas,
	grounding *ArchitectureGrounding,
	repositoryGraph *RepositoryGraph,
) ([]EntrypointHandoffGroup, error) {
	return projectEntrypointHandoffGroupsForProduct(canvas, grounding, repositoryGraph)
}

func projectEntrypointHandoffGroupsForProduct(
	canvas *ArchitectureCanvas,
	grounding *ArchitectureGrounding,
	repositoryGraph *RepositoryGraph,
) ([]EntrypointHandoffGroup, error) {
	if canvas == nil {
		return nil, nil
	}
	if canvas.Version != ArchitectureCanvasVersion {
		return nil, fmt.Errorf("entry handoff group: unsupported canvas version %d", canvas.Version)
	}
	if err := validateEntrypointHandoffAuthority(grounding); err != nil {
		return nil, err
	}

	entryAnchors, err := processEntryAnchors(canvas)
	if err != nil {
		return nil, err
	}
	if len(entryAnchors) > maxEntrypointHandoffGroups {
		return nil, fmt.Errorf(
			"entry handoff group: process entry count %d exceeds limit %d",
			len(entryAnchors),
			maxEntrypointHandoffGroups,
		)
	}
	groups := make([]EntrypointHandoffGroup, 0, len(entryAnchors))
	groupIDs := make(map[string]struct{}, len(entryAnchors))
	totalHandoffs := 0
	for _, entryAnchor := range entryAnchors {
		group, err := projectEntrypointHandoffGroupForEntry(canvas, grounding, repositoryGraph, entryAnchor)
		if err != nil {
			return nil, err
		}
		if group == nil {
			continue
		}
		if _, duplicate := groupIDs[group.ID]; duplicate {
			return nil, fmt.Errorf("entry handoff group: duplicate exact entry identity %q", group.ID)
		}
		groupIDs[group.ID] = struct{}{}
		totalHandoffs += len(group.EntryHandoffs)
		if totalHandoffs > maxEntrypointHandoffsTotal {
			return nil, fmt.Errorf(
				"entry handoff group: total handoff count %d exceeds limit %d",
				totalHandoffs,
				maxEntrypointHandoffsTotal,
			)
		}
		groups = append(groups, *group)
	}
	if len(groups) == 0 {
		return nil, nil
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].ID < groups[j].ID })
	return groups, nil
}

// validateEntrypointHandoffAuthority closes the public projection over the
// validated D210 relation grammar without imposing artifact wire order. Stable
// projection tests may permute inputs, but malformed or duplicate relation
// identities fail closed before any producer/scenario data is published.
func validateEntrypointHandoffAuthority(grounding *ArchitectureGrounding) error {
	if grounding == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(grounding.EntryHandoffs))
	for index, handoff := range grounding.EntryHandoffs {
		if err := validateArchitectureEntryHandoffs([]ArchitectureEntryHandoff{handoff}); err != nil {
			return fmt.Errorf("entry handoff group: entry handoff %d is not exact D210 evidence: %w", index, err)
		}
		if _, duplicate := seen[handoff.ID]; duplicate {
			return fmt.Errorf("entry handoff group: duplicate entry handoff identity %q", handoff.ID)
		}
		seen[handoff.ID] = struct{}{}
	}
	return nil
}

type entrypointHandoffTarget struct {
	transition   EntrypointTransition
	componentIDs []componentmap.ComponentID
}

func projectEntrypointHandoffGroupForEntry(
	canvas *ArchitectureCanvas,
	grounding *ArchitectureGrounding,
	repositoryGraph *RepositoryGraph,
	entryAnchor *componentmap.BehaviorAnchor,
) (*EntrypointHandoffGroup, error) {
	entryMembers := processEntryMemberIDs(entryAnchor)
	if len(entryMembers) == 0 {
		return nil, nil
	}

	handoffs := []EntrypointTransition{}
	frontier := EntrypointFrontier{
		Ordering:   "not_established",
		Unresolved: []string{"continuation beyond the first-hop handoffs"},
		Limitation: "No further transition is locally proven to continue from these first-hop handoffs; execution order beyond them is not established.",
	}
	// Exact D210 handoffs are the sole authoritative first-hop projection.
	var handoffTargets []entrypointHandoffTarget
	if grounding != nil {
		for _, handoff := range grounding.EntryHandoffs {
			if handoff.Scenario.ID != entryAnchor.Scenario.ID ||
				!architectureDeclarationLocationsCompatible(handoff.ProcessEntrypoint.Location, entryAnchor.Location) ||
				!entrypointHandoffMatchesAnchor(canvas, entryAnchor, handoff) {
				continue
			}
			componentIDs := componentIDsForEntryHandoffCallee(canvas, repositoryGraph, handoff)
			transition := transitionFromEntryHandoff(handoff, componentIDs)
			handoffTargets = append(handoffTargets, entrypointHandoffTarget{
				transition:   transition,
				componentIDs: componentIDs,
			})
		}
	}
	if len(handoffTargets) > maxEntrypointHandoffsPerGroup {
		return nil, fmt.Errorf(
			"entry handoff group: entry %q handoff count %d exceeds limit %d",
			entryAnchor.ID,
			len(handoffTargets),
			maxEntrypointHandoffsPerGroup,
		)
	}
	handoffTargets = compactEntrypointHandoffTargets(handoffTargets)
	if len(handoffTargets) == 0 {
		// Zero-hop process entries remain authoritative surface Entrypoints;
		// this joined handoff projection has nothing to add for them.
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
		leftTarget := entrypointHandoffTargetKey(left.Target)
		rightTarget := entrypointHandoffTargetKey(right.Target)
		if leftTarget != rightTarget {
			return leftTarget < rightTarget
		}
		if left.Label != right.Label {
			return left.Label < right.Label
		}
		return entrypointEvidenceRefKey(left.EvidenceRef) < entrypointEvidenceRefKey(right.EvidenceRef)
	})
	entryComponentIDs := componentIDsForAnchor(canvas, entryAnchor)
	componentSet := make(map[componentmap.ComponentID]struct{}, len(entryComponentIDs))
	for _, componentID := range entryComponentIDs {
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
		entrySymbol = exactEntrypointSymbol(canvas, entryMembers[0], entryAnchor.Location)
	}
	entryLabel := "process entry"
	if entrySymbol != "" {
		entryLabel += " " + entrySymbol
	}
	entry := EntrypointTransition{
		Ordinal:      0,
		ClaimKind:    "process_entry",
		SupportMode:  "resolved_static",
		Label:        entryLabel,
		Path:         entryAnchor.Location.Path,
		Line:         entryAnchor.Location.Line,
		Column:       entryAnchor.Location.Column,
		Symbol:       entrySymbol,
		ComponentIDs: mergeEntrypointComponentIDs(nil, entryComponentIDs),
		Evidence:     "behavior anchor proof_mode " + string(entryAnchor.ProofMode),
		Scenario:     entryAnchor.Scenario.ID,
		Limitation:   "process entry identity only; runtime reachability not proven",
		Ordering:     "exact_local_order",
	}
	componentIDs := make([]componentmap.ComponentID, 0, len(componentSet))
	for componentID := range componentSet {
		componentIDs = append(componentIDs, componentID)
	}
	sort.Slice(componentIDs, func(i, j int) bool { return componentIDs[i] < componentIDs[j] })
	return &EntrypointHandoffGroup{
		Version:       EntrypointHandoffGroupVersion,
		ID:            entrypointHandoffGroupID(entryAnchor),
		ComponentIDs:  componentIDs,
		Entry:         entry,
		EntryHandoffs: handoffs,
		Frontier:      frontier,
	}, nil
}

func entrypointHandoffMatchesAnchor(
	canvas *ArchitectureCanvas,
	entryAnchor *componentmap.BehaviorAnchor,
	handoff ArchitectureEntryHandoff,
) bool {
	if entryAnchor == nil || handoff.ProcessEntrypoint.ID == "" {
		return false
	}
	for _, memberID := range processEntryMemberIDs(entryAnchor) {
		if exactEntrypointSymbol(canvas, memberID, entryAnchor.Location) ==
			handoff.ProcessEntrypoint.ID {
			return true
		}
	}
	return false
}

// exactEntrypointSymbol restores readable entry identity only from one
// exact declaration fact at the process-entry location. Member IDs are opaque
// local join keys and must never become product copy.
func exactEntrypointSymbol(
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

func transitionFromEntryHandoff(
	handoff ArchitectureEntryHandoff,
	componentIDs []componentmap.ComponentID,
) EntrypointTransition {
	target := &EntrypointHandoffTarget{
		Label:  handoff.Callee.Name,
		Path:   handoff.Callee.Location.Path,
		Line:   handoff.Callee.Location.Line,
		Column: handoff.Callee.Location.Column,
	}
	if len(componentIDs) > 0 {
		target.Symbol = handoff.Callee.ID
	}
	transition := EntrypointTransition{
		ClaimKind:    "direct_static_call",
		SupportMode:  "resolved_static",
		Label:        "handoff to " + handoff.Callee.Name,
		Path:         handoff.RepresentativeCallsite.Path,
		Line:         handoff.RepresentativeCallsite.Line,
		Column:       handoff.RepresentativeCallsite.Column,
		Symbol:       handoff.Callee.Name,
		ComponentIDs: mergeEntrypointComponentIDs(nil, componentIDs),
		WitnessCount: max(handoff.WitnessCount, 1),
		Evidence:     handoff.Producer.Provider + " " + handoff.Producer.Version + " " + handoff.Producer.Operation,
		EvidenceRef: &EntrypointEvidenceRef{
			Kind: entrypointEvidenceRefEntryHandoff,
			ID:   handoff.ID,
		},
		Provenance: &EntrypointProvenance{
			Provider:  handoff.Producer.Provider,
			Version:   handoff.Producer.Version,
			Operation: handoff.Producer.Operation,
		},
		Scenario:   handoff.Scenario.ID,
		Limitation: architectureEntryHandoffLimitation,
		Ordering:   "resolved_path_order",
		Target:     target,
	}
	if len(handoff.Limitations) > 0 {
		transition.Limitation = handoff.Limitations[0]
	}
	return transition
}

func compactEntrypointHandoffTargets(targets []entrypointHandoffTarget) []entrypointHandoffTarget {
	seen := make(map[string]int, len(targets))
	result := targets[:0]
	for _, target := range targets {
		transition := target.transition
		key := fmt.Sprintf(
			"%s\x00%s\x00%d\x00%d\x00%s\x00%s\x00%s\x00%s",
			transition.ClaimKind,
			transition.Path,
			transition.Line,
			transition.Column,
			transition.Symbol,
			transition.Scenario,
			entrypointEvidenceRefKey(transition.EvidenceRef),
			entrypointHandoffTargetKey(transition.Target),
		)
		if existing, duplicate := seen[key]; duplicate {
			if transition.WitnessCount > result[existing].transition.WitnessCount {
				result[existing].transition.WitnessCount = transition.WitnessCount
			}
			result[existing].componentIDs = mergeEntrypointComponentIDs(
				result[existing].componentIDs,
				target.componentIDs,
			)
			result[existing].transition.ComponentIDs = mergeEntrypointComponentIDs(
				result[existing].transition.ComponentIDs,
				target.componentIDs,
			)
			continue
		}
		seen[key] = len(result)
		target.componentIDs = mergeEntrypointComponentIDs(nil, target.componentIDs)
		result = append(result, target)
	}
	return result
}

func entrypointHandoffTargetKey(target *EntrypointHandoffTarget) string {
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

func mergeEntrypointComponentIDs(
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

func entrypointEvidenceRefKey(reference *EntrypointEvidenceRef) string {
	if reference == nil {
		return ""
	}
	return reference.Kind + "\x00" + reference.ID
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
				"entry handoff group: process entry anchor %q is not exact",
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

func componentIDsForAnchor(
	canvas *ArchitectureCanvas,
	anchor *componentmap.BehaviorAnchor,
) []componentmap.ComponentID {
	if anchor == nil {
		return []componentmap.ComponentID{}
	}
	var result []componentmap.ComponentID
	for _, memberID := range anchor.MemberIDs {
		result = mergeEntrypointComponentIDs(result, componentIDsForMemberID(canvas, memberID))
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
	return mergeEntrypointComponentIDs(nil, result)
}

func componentIDsForEntryHandoffCallee(
	canvas *ArchitectureCanvas,
	repositoryGraph *RepositoryGraph,
	handoff ArchitectureEntryHandoff,
) []componentmap.ComponentID {
	return architectureComponentIDsForExactDeclaration(
		canvas,
		repositoryGraph,
		handoff.Callee.ID,
		handoff.Callee.Location,
	)
}

func entrypointHandoffGroupID(anchor *componentmap.BehaviorAnchor) string {
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
	return architectureStableID("entry-handoff-group", parts...)
}
