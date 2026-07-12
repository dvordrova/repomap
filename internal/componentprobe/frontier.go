package componentprobe

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/componentstudy"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/symbol"
)

func buildFrontier(
	probes []SymbolProbe,
	selected []componentstudy.SymbolCandidate,
	limit int,
) ([]Frontier, int) {
	selectedKeys := make(map[string]struct{}, len(selected))
	for _, candidate := range selected {
		selectedKeys[selectedIdentity(candidate)] = struct{}{}
	}

	byKey := make(map[string]Frontier)
	for _, probe := range probes {
		for _, call := range probe.Structural.IncomingCalls {
			addCallFrontier(byKey, selectedKeys, probe.ID, DirectionIncoming, call.Caller, call)
		}
		for _, call := range probe.Structural.OutgoingCalls {
			addCallFrontier(byKey, selectedKeys, probe.ID, DirectionOutgoing, call.Callee, call)
		}
		for _, reference := range probe.Tests.References {
			location := evidence.Location{
				Path:   reference.Path,
				Line:   reference.Line,
				Column: reference.Column,
			}
			key := frontierKey(FrontierTestReference, DirectionReference, probe.Tests.TargetName, evidence.EntityTest, location)
			origin := EvidenceOrigin{ProbeID: probe.ID, Artifact: ArtifactTests, LocalID: reference.EvidenceID}
			candidate, exists := byKey[key]
			if !exists {
				candidate = Frontier{
					ID:             stableID("frontier", key),
					Kind:           FrontierTestReference,
					Direction:      DirectionReference,
					Name:           probe.Tests.TargetName,
					EntityKind:     evidence.EntityTest,
					Location:       location,
					Certainty:      reference.Certainty,
					Provenance:     cloneProvenance(reference.Provenance),
					Basis:          SupportTestNavigation,
					NavigationOnly: true,
					RuntimeProof:   false,
				}
			}
			candidate.Origins = appendUniqueOrigin(candidate.Origins, origin)
			candidate.Provenance = appendUniqueProvenance(candidate.Provenance, reference.Provenance)
			byKey[key] = candidate
		}
	}

	result := make([]Frontier, 0, len(byKey))
	for _, candidate := range byKey {
		sortOrigins(candidate.Origins)
		result = append(result, candidate)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind != result[j].Kind {
			return result[i].Kind < result[j].Kind
		}
		if result[i].Direction != result[j].Direction {
			return result[i].Direction < result[j].Direction
		}
		if frontierPathTier(result[i]) != frontierPathTier(result[j]) {
			return frontierPathTier(result[i]) < frontierPathTier(result[j])
		}
		if result[i].Location.Path != result[j].Location.Path {
			return result[i].Location.Path < result[j].Location.Path
		}
		if result[i].Location.Line != result[j].Location.Line {
			return result[i].Location.Line < result[j].Location.Line
		}
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].ID < result[j].ID
	})
	omitted := max(0, len(result)-limit)
	return selectFrontierFair(result, probes, limit), omitted
}

func frontierPathTier(candidate Frontier) int {
	if candidate.Kind == FrontierCallEndpoint && strings.HasSuffix(strings.ToLower(candidate.Location.Path), "_test.go") {
		return 1
	}
	return 0
}

func selectFrontierFair(candidates []Frontier, probes []SymbolProbe, limit int) []Frontier {
	if len(candidates) <= limit {
		return candidates
	}
	probeIDs := make([]string, 0, len(probes))
	for _, probe := range probes {
		probeIDs = append(probeIDs, probe.ID)
	}
	sort.Strings(probeIDs)
	selected := make([]Frontier, 0, limit)
	seen := make(map[string]struct{}, limit)
	addFirst := func(probeID string, direction Direction) {
		if len(selected) >= limit {
			return
		}
		for _, candidate := range candidates {
			if candidate.Kind != FrontierCallEndpoint || candidate.Direction != direction || !hasProbeOrigin(candidate.Origins, probeID) {
				continue
			}
			if _, exists := seen[candidate.ID]; exists {
				continue
			}
			selected = append(selected, candidate)
			seen[candidate.ID] = struct{}{}
			return
		}
	}
	for _, probeID := range probeIDs {
		addFirst(probeID, DirectionOutgoing)
	}
	for _, probeID := range probeIDs {
		addFirst(probeID, DirectionIncoming)
	}
	for _, candidate := range candidates {
		if len(selected) >= limit {
			break
		}
		if _, exists := seen[candidate.ID]; exists {
			continue
		}
		selected = append(selected, candidate)
		seen[candidate.ID] = struct{}{}
	}
	return selected
}

func hasProbeOrigin(origins []EvidenceOrigin, probeID string) bool {
	for _, origin := range origins {
		if origin.ProbeID == probeID {
			return true
		}
	}
	return false
}

func addCallFrontier(
	byKey map[string]Frontier,
	selected map[string]struct{},
	probeID string,
	direction Direction,
	endpoint evidence.Entity,
	call symbol.CallFact,
) {
	if endpoint.Location == nil {
		return
	}
	if _, alreadySelected := selected[entityIdentity(endpoint)]; alreadySelected {
		return
	}
	key := frontierKey(FrontierCallEndpoint, direction, endpoint.Name, endpoint.Kind, *endpoint.Location)
	origin := EvidenceOrigin{ProbeID: probeID, Artifact: ArtifactStructural, LocalID: call.EvidenceID}
	candidate, exists := byKey[key]
	if !exists {
		candidate = Frontier{
			ID:             stableID("frontier", key),
			Kind:           FrontierCallEndpoint,
			Direction:      direction,
			Name:           endpoint.Name,
			EntityKind:     endpoint.Kind,
			Location:       *endpoint.Location,
			Certainty:      call.Certainty,
			Provenance:     cloneProvenance(call.Provenance),
			Basis:          SupportStaticNavigation,
			NavigationOnly: false,
			RuntimeProof:   false,
		}
	}
	candidate.Origins = appendUniqueOrigin(candidate.Origins, origin)
	candidate.Provenance = appendUniqueProvenance(candidate.Provenance, call.Provenance)
	byKey[key] = candidate
}

func selectedIdentity(candidate componentstudy.SymbolCandidate) string {
	return fmt.Sprintf("%s\x00%d\x00%d\x00%s", candidate.Path, candidate.Line, candidate.Column, callableName(candidate.Name))
}

func entityIdentity(entity evidence.Entity) string {
	if entity.Location == nil {
		return ""
	}
	return fmt.Sprintf("%s\x00%d\x00%d\x00%s", entity.Location.Path, entity.Location.Line, entity.Location.Column, callableName(entity.Name))
}

func frontierKey(
	kind FrontierKind,
	direction Direction,
	name string,
	entityKind evidence.EntityKind,
	location evidence.Location,
) string {
	return fmt.Sprintf(
		"%s\x00%s\x00%s\x00%s\x00%d\x00%d\x00%s",
		kind,
		direction,
		entityKind,
		location.Path,
		location.Line,
		location.Column,
		callableName(name),
	)
}

func appendUniqueOrigin(values []EvidenceOrigin, value EvidenceOrigin) []EvidenceOrigin {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func sortOrigins(values []EvidenceOrigin) {
	sort.Slice(values, func(i, j int) bool {
		if values[i].ProbeID != values[j].ProbeID {
			return values[i].ProbeID < values[j].ProbeID
		}
		if values[i].Artifact != values[j].Artifact {
			return values[i].Artifact < values[j].Artifact
		}
		return values[i].LocalID < values[j].LocalID
	})
}

func appendUniqueProvenance(dst, values []evidence.Provenance) []evidence.Provenance {
	for _, value := range values {
		duplicate := false
		for _, existing := range dst {
			if sameProvenance(existing, value) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			dst = append(dst, cloneProvenance([]evidence.Provenance{value})[0])
		}
	}
	return dst
}

func sameProvenance(left, right evidence.Provenance) bool {
	if left.Provider != right.Provider || left.Version != right.Version ||
		left.Operation != right.Operation || left.Detail != right.Detail {
		return false
	}
	if left.Location == nil || right.Location == nil {
		return left.Location == nil && right.Location == nil
	}
	return *left.Location == *right.Location
}

func deriveStatus(probes []SymbolProbe, selected []componentstudy.SymbolCandidate) Status {
	if len(probes) == 0 {
		return StatusBlocked
	}
	// One point is useful evidence but not yet a connected lifecycle slice.
	if len(probes) != len(selected) || len(selected) < 2 {
		return StatusFrontier
	}

	index := make(map[string]int, len(selected))
	for i, candidate := range selected {
		index[selectedIdentity(candidate)] = i
	}
	adjacency := make([]map[int]struct{}, len(selected))
	for i := range adjacency {
		adjacency[i] = make(map[int]struct{})
	}
	for _, probe := range probes {
		calls := append(append([]symbol.CallFact{}, probe.Structural.IncomingCalls...), probe.Structural.OutgoingCalls...)
		for _, call := range calls {
			from, fromSelected := index[entityIdentity(call.Caller)]
			to, toSelected := index[entityIdentity(call.Callee)]
			if !fromSelected || !toSelected || call.Certainty != evidence.CertaintyStatic {
				continue
			}
			adjacency[from][to] = struct{}{}
			adjacency[to][from] = struct{}{}
		}
	}

	seen := map[int]struct{}{0: {}}
	queue := []int{0}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for next := range adjacency[current] {
			if _, exists := seen[next]; exists {
				continue
			}
			seen[next] = struct{}{}
			queue = append(queue, next)
		}
	}
	if len(seen) == len(selected) {
		return StatusConnected
	}
	return StatusFrontier
}
