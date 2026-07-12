package componentteach

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/componentprobe"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/sourcecard"
	"github.com/dvordrova/repomap/internal/symbol"
)

const activeBuildCaveat = "Static call hierarchy under the active Go build configuration; this is navigation evidence, not runtime proof."

type buildCandidate struct {
	item       EvidenceItem
	locator    LocatorEntry
	duplicate  bool
	omitReason SelectionReason
	order      int
	tier       int
}

type numberedLine struct {
	number int
	text   string
	origin Origin
}

// Build creates a provider-safe Bundle and a separate local Index from an
// immutable one- or two-round componentprobe chain.
func Build(
	round1 componentprobe.Bundle,
	round2 *componentprobe.Bundle,
	budget Budget,
) (Bundle, Index, SelectionTrace, error) {
	if err := budget.Validate(); err != nil {
		return Bundle{}, Index{}, SelectionTrace{}, err
	}
	if err := round1.Validate(); err != nil {
		return Bundle{}, Index{}, SelectionTrace{}, fmt.Errorf("component teach: invalid round 1: %w", err)
	}
	if round1.Round != componentprobe.RoundInitial {
		return Bundle{}, Index{}, SelectionTrace{}, fmt.Errorf("component teach: first bundle must be round 1")
	}
	rounds := []componentprobe.Bundle{round1}
	if round2 != nil {
		if err := round2.ValidateAgainst(round1); err != nil {
			return Bundle{}, Index{}, SelectionTrace{}, fmt.Errorf("component teach: invalid round 2 chain: %w", err)
		}
		rounds = append(rounds, *round2)
	}

	question := round1.Focus.PrimaryQuestion
	unresolvedFrontier := unresolvedLatestFrontier(rounds)
	bundle := Bundle{
		Version:       BundleVersion,
		GoalObjective: round1.Focus.Goal.Objective,
		Component: Component{
			Name:              round1.Focus.Component.Name,
			PurposeHypothesis: round1.Focus.Component.Purpose,
			SupportBasis:      SupportOrientationHypothesis,
		},
		PrimaryQuestion: PrimaryQuestion{
			ID: question.ID, Question: question.Question, Why: question.Why,
		},
		Evidence:              []EvidenceItem{},
		UnresolvedFrontierIDs: frontierIDs(unresolvedFrontier),
		UnresolvedFrontiers:   frontierHints(unresolvedFrontier),
		Warnings: []string{
			"The component purpose is an orientation hypothesis, not a verified conclusion.",
		},
	}
	index := Index{Version: IndexVersion, Entries: frontierLocators(rounds[len(rounds)-1].Round, unresolvedFrontier)}
	candidates := make([]buildCandidate, 0)
	for _, round := range rounds {
		candidates = append(candidates, roundCandidates(round, &bundle.Warnings)...)
	}
	for index := range candidates {
		candidates[index].order = index
	}
	candidates = markDuplicates(candidates)
	sort.SliceStable(candidates, func(i, j int) bool {
		if evidencePriority(candidates[i].item.Kind) != evidencePriority(candidates[j].item.Kind) {
			return evidencePriority(candidates[i].item.Kind) < evidencePriority(candidates[j].item.Kind)
		}
		if candidates[i].tier != candidates[j].tier {
			return candidates[i].tier < candidates[j].tier
		}
		if candidates[i].order != candidates[j].order {
			return candidates[i].order < candidates[j].order
		}
		return candidates[i].item.ID < candidates[j].item.ID
	})

	trace := SelectionTrace{
		Version:   SelectionTraceVersion,
		Budget:    budget,
		Decisions: make([]SelectionDecision, 0, len(candidates)),
	}
	for _, candidate := range candidates {
		estimated := estimateEvidence(candidate.item)
		decision := SelectionDecision{Kind: candidate.item.Kind, ID: candidate.item.ID, EstimatedBytes: estimated}
		switch {
		case candidate.omitReason != "":
			decision.Reason = candidate.omitReason
		case candidate.duplicate:
			decision.Reason = SelectionDuplicate
		default:
			possible := bundle
			possible.Evidence = appendCopy(bundle.Evidence, candidate.item)
			size, err := modelBytes(possible)
			if err != nil {
				return Bundle{}, Index{}, SelectionTrace{}, err
			}
			if size > budget.MaxModelBytes {
				decision.Reason = SelectionModelBytesLimit
			} else {
				decision.Included = true
				decision.Reason = SelectionWithinBudget
				bundle = possible
				index.Entries = append(index.Entries, candidate.locator)
			}
		}
		trace.Decisions = append(trace.Decisions, decision)
	}
	trace.EstimatedModelBytes, _ = modelBytes(bundle)
	if err := bundle.Validate(); err != nil {
		return Bundle{}, Index{}, SelectionTrace{}, err
	}
	if err := index.Validate(bundle); err != nil {
		return Bundle{}, Index{}, SelectionTrace{}, err
	}
	if err := trace.Validate(); err != nil {
		return Bundle{}, Index{}, SelectionTrace{}, err
	}
	return bundle, index, trace, nil
}

func roundCandidates(round componentprobe.Bundle, warnings *[]string) []buildCandidate {
	result := make([]buildCandidate, 0)
	for _, probe := range round.SymbolProbes {
		result = append(result, relationCandidates(round.Round, probe)...)
		result = append(result, sourceCandidates(round.Round, probe, warnings)...)
		result = append(result, testCandidates(round.Round, probe)...)
	}
	for _, window := range round.CallsiteWindows {
		result = append(result, callsiteCandidates(round.Round, window, warnings)...)
	}
	return result
}

func relationCandidates(round int, probe componentprobe.SymbolProbe) []buildCandidate {
	result := make([]buildCandidate, 0, len(probe.Structural.IncomingCalls)+len(probe.Structural.OutgoingCalls))
	for _, call := range probe.Structural.IncomingCalls {
		if candidate, ok := relationCandidate(round, probe.ID, call); ok {
			result = append(result, candidate)
		}
	}
	for _, call := range probe.Structural.OutgoingCalls {
		if candidate, ok := relationCandidate(round, probe.ID, call); ok {
			result = append(result, candidate)
		}
	}
	return result
}

func relationCandidate(round int, probeID string, call symbol.CallFact) (buildCandidate, bool) {
	location, ok := relationLocation(call)
	if !ok {
		return buildCandidate{}, false
	}
	key := fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%d", call.Caller.Name, call.Callee.Name, location.Path, location.Line, location.Column)
	id := stableID("teach", "relation", key)
	return buildCandidate{
		item: EvidenceItem{
			ID: id, Kind: EvidenceStaticRelation, SupportBasis: SupportStaticActiveBuild,
			Summary: fmt.Sprintf("Static call relation: %s calls %s.", call.Caller.Name, call.Callee.Name),
			Caller:  call.Caller.Name, Callee: call.Callee.Name, Direction: "caller_to_callee",
			ActiveBuildCaveat: activeBuildCaveat,
		},
		locator: LocatorEntry{
			ID: id, Kind: LocatorEvidence, Path: location.Path,
			StartLine: location.Line, EndLine: max(location.Line, location.EndLine), Column: location.Column,
			Origins: []Origin{{Round: round, ProbeID: probeID, Artifact: componentprobe.ArtifactStructural, LocalID: call.EvidenceID}},
		},
	}, true
}

func relationLocation(call symbol.CallFact) (evidence.Location, bool) {
	if call.Callsite != nil {
		return *call.Callsite, true
	}
	if call.Caller.Location != nil {
		return *call.Caller.Location, true
	}
	if call.Callee.Location != nil {
		return *call.Callee.Location, true
	}
	return evidence.Location{}, false
}

func sourceCandidates(round int, probe componentprobe.SymbolProbe, warnings *[]string) []buildCandidate {
	if err := sourcecard.ValidateForRemote(probe.Source); err != nil {
		id := stableID("teach", "source-card-rejected", probe.ID)
		*warnings = append(*warnings, "A source card was omitted because outbound source validation failed.")
		return []buildCandidate{{
			item:       EvidenceItem{ID: id, Kind: EvidenceSourceSlice, SupportBasis: SupportSource, Summary: "Omitted source card"},
			omitReason: SelectionRemoteValidationFail,
		}}
	}
	lines := make([]numberedLine, 0, len(probe.Source.Lines))
	for _, line := range probe.Source.Lines {
		lines = append(lines, numberedLine{
			number: line.Line, text: line.Text,
			origin: Origin{Round: round, ProbeID: probe.ID, Artifact: componentprobe.ArtifactSource, LocalID: line.EvidenceID},
		})
	}
	return sliceCandidates(EvidenceSourceSlice, SupportSource, "Source slice for "+probe.Source.Target.Name, probe.Source.Target.Path, probe.Source.Target.Column, lines)
}

func callsiteCandidates(round int, window componentprobe.CallsiteWindow, warnings *[]string) []buildCandidate {
	remoteLines := make([]sourcecard.Line, 0, len(window.Lines))
	for _, line := range window.Lines {
		remoteLines = append(remoteLines, sourcecard.Line{EvidenceID: line.EvidenceID, Line: line.Line, Text: line.Text})
	}
	if err := sourcecard.ValidateLinesForRemote(remoteLines); err != nil {
		id := stableID("teach", "callsite-rejected", window.EvidenceID)
		*warnings = append(*warnings, "A callsite slice was omitted because outbound source validation failed.")
		return []buildCandidate{{
			item:       EvidenceItem{ID: id, Kind: EvidenceCallsiteSlice, SupportBasis: SupportSource, Summary: "Omitted callsite slice"},
			omitReason: SelectionRemoteValidationFail,
		}}
	}
	lines := make([]numberedLine, 0, len(window.Lines))
	for _, line := range window.Lines {
		lines = append(lines, numberedLine{
			number: line.Line, text: line.Text,
			origin: Origin{Round: round, ProbeID: window.Origin.ProbeID, Artifact: window.Origin.Artifact, LocalID: window.Origin.LocalID},
		})
	}
	summary := fmt.Sprintf("Callsite slice for %s calling %s.", window.Caller.Name, window.Callee.Name)
	return sliceCandidates(EvidenceCallsiteSlice, SupportSource, summary, window.Callsite.Path, window.Callsite.Column, lines)
}

func sliceCandidates(kind EvidenceKind, basis SupportBasis, summary, path string, column int, lines []numberedLine) []buildCandidate {
	chunks := chunkLines(lines)
	result := make([]buildCandidate, 0, len(chunks))
	for chunkIndex, chunk := range chunks {
		content := make([]string, 0, len(chunk))
		origins := make([]Origin, 0, len(chunk))
		for _, line := range chunk {
			content = append(content, line.text)
			origins = appendUniqueOrigin(origins, line.origin)
		}
		start, end := chunk[0].number, chunk[len(chunk)-1].number
		id := stableID("teach", string(kind), path, fmt.Sprint(start), fmt.Sprint(end), strings.Join(content, "\n"))
		result = append(result, buildCandidate{
			item: EvidenceItem{ID: id, Kind: kind, SupportBasis: basis, Summary: summary, Content: content},
			tier: chunkIndex,
			locator: LocatorEntry{
				ID: id, Kind: LocatorEvidence, Path: path, StartLine: start, EndLine: end,
				Column: column, Origins: origins,
			},
		})
	}
	return result
}

func chunkLines(lines []numberedLine) [][]numberedLine {
	result := make([][]numberedLine, 0)
	current := make([]numberedLine, 0, maxSliceLines)
	bytes := 0
	flush := func() {
		if len(current) > 0 {
			result = append(result, current)
			current = make([]numberedLine, 0, maxSliceLines)
			bytes = 0
		}
	}
	for _, line := range lines {
		line.text = truncateUTF8(line.text, maxSliceBytes)
		cost := len(line.text)
		if len(current) > 0 {
			cost++
		}
		if len(current) >= maxSliceLines || (len(current) > 0 && bytes+cost > maxSliceBytes) {
			flush()
			cost = len(line.text)
		}
		current = append(current, line)
		bytes += cost
	}
	flush()
	return result
}

func testCandidates(round int, probe componentprobe.SymbolProbe) []buildCandidate {
	result := make([]buildCandidate, 0, len(probe.Tests.References))
	for _, reference := range probe.Tests.References {
		key := fmt.Sprintf("%s\x00%s\x00%d\x00%d", probe.Tests.TargetName, reference.Path, reference.Line, reference.Column)
		id := stableID("teach", "test-reference", key)
		result = append(result, buildCandidate{
			item: EvidenceItem{
				ID: id, Kind: EvidenceTestReference, SupportBasis: SupportTestNavigation,
				Summary:        "A Go test references " + probe.Tests.TargetName + "; use this only as a navigation lead.",
				NavigationOnly: true,
			},
			locator: LocatorEntry{
				ID: id, Kind: LocatorEvidence, Path: reference.Path,
				StartLine: reference.Line, EndLine: reference.Line, Column: reference.Column,
				Origins: []Origin{{Round: round, ProbeID: probe.ID, Artifact: componentprobe.ArtifactTests, LocalID: reference.EvidenceID}},
			},
		})
	}
	return result
}

func unresolvedLatestFrontier(rounds []componentprobe.Bundle) []componentprobe.Frontier {
	probed := make(map[string]struct{})
	for _, round := range rounds {
		for _, probe := range round.SymbolProbes {
			selected := probe.SelectedSymbol
			probed[callableIdentity(selected.Path, selected.Line, selected.Column, selected.Name)] = struct{}{}
		}
	}
	latest := rounds[len(rounds)-1]
	result := make([]componentprobe.Frontier, 0, len(latest.Frontier))
	for _, candidate := range latest.Frontier {
		identity := callableIdentity(
			candidate.Location.Path,
			candidate.Location.Line,
			candidate.Location.Column,
			candidate.Name,
		)
		if _, alreadyProbed := probed[identity]; alreadyProbed {
			continue
		}
		result = append(result, candidate)
	}
	return result
}

func callableIdentity(path string, line, column int, name string) string {
	if index := strings.LastIndexByte(name, '.'); index >= 0 {
		name = name[index+1:]
	}
	return fmt.Sprintf("%s\x00%d\x00%d\x00%s", path, line, column, strings.TrimSpace(name))
}

func frontierIDs(frontier []componentprobe.Frontier) []string {
	ids := make([]string, 0, len(frontier))
	for _, candidate := range frontier {
		ids = append(ids, candidate.ID)
	}
	sort.Strings(ids)
	return ids
}

func frontierHints(frontier []componentprobe.Frontier) []FrontierHint {
	hints := make([]FrontierHint, 0, len(frontier))
	for _, candidate := range frontier {
		basis := SupportStaticActiveBuild
		if candidate.NavigationOnly {
			basis = SupportTestNavigation
		}
		hints = append(hints, FrontierHint{
			ID: candidate.ID, Kind: string(candidate.Kind), Direction: string(candidate.Direction),
			Name: candidate.Name, EntityKind: string(candidate.EntityKind), SupportBasis: basis,
			NavigationOnly: candidate.NavigationOnly,
		})
	}
	sort.Slice(hints, func(i, j int) bool { return hints[i].ID < hints[j].ID })
	return hints
}

func frontierLocators(round int, frontier []componentprobe.Frontier) []LocatorEntry {
	entries := make([]LocatorEntry, 0, len(frontier))
	for _, candidate := range frontier {
		origins := make([]Origin, 0, len(candidate.Origins))
		for _, origin := range candidate.Origins {
			origins = append(origins, Origin{Round: round, ProbeID: origin.ProbeID, Artifact: origin.Artifact, LocalID: origin.LocalID})
		}
		entries = append(entries, LocatorEntry{
			ID: candidate.ID, Kind: LocatorFrontier, Path: candidate.Location.Path,
			StartLine: candidate.Location.Line, EndLine: max(candidate.Location.Line, candidate.Location.EndLine),
			Column: candidate.Location.Column, Origins: origins,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries
}

func markDuplicates(candidates []buildCandidate) []buildCandidate {
	first := make(map[string]int, len(candidates))
	for index := range candidates {
		candidate := &candidates[index]
		if candidate.omitReason != "" {
			continue
		}
		prior, exists := first[candidate.item.ID]
		if !exists {
			first[candidate.item.ID] = index
			continue
		}
		candidate.duplicate = true
		candidates[prior].locator.Origins = mergeOrigins(candidates[prior].locator.Origins, candidate.locator.Origins)
	}
	return candidates
}

func evidencePriority(kind EvidenceKind) int {
	switch kind {
	case EvidenceOrientationNote:
		return 0
	case EvidenceStaticRelation:
		return 1
	case EvidenceSourceSlice:
		return 2
	case EvidenceCallsiteSlice:
		return 3
	case EvidenceTestReference:
		return 4
	default:
		return 9
	}
}

func estimateEvidence(item EvidenceItem) int {
	data, _ := json.Marshal(item)
	return len(data) + 1
}

func modelBytes(bundle Bundle) (int, error) {
	data, err := json.Marshal(bundle)
	if err != nil {
		return 0, fmt.Errorf("component teach: marshal model bundle: %w", err)
	}
	return len(data), nil
}

func stableID(prefix string, parts ...string) string {
	hash := sha256.New()
	var length [8]byte
	for _, part := range parts {
		binary.BigEndian.PutUint64(length[:], uint64(len(part)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write([]byte(part))
	}
	return fmt.Sprintf("%s-%x", prefix, hash.Sum(nil)[:10])
}

func appendCopy[T any](values []T, value T) []T {
	result := make([]T, 0, len(values)+1)
	result = append(result, values...)
	return append(result, value)
}

func appendUniqueOrigin(values []Origin, value Origin) []Origin {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func mergeOrigins(left, right []Origin) []Origin {
	for _, origin := range right {
		left = appendUniqueOrigin(left, origin)
	}
	sort.Slice(left, func(i, j int) bool {
		if left[i].Round != left[j].Round {
			return left[i].Round < left[j].Round
		}
		if left[i].ProbeID != left[j].ProbeID {
			return left[i].ProbeID < left[j].ProbeID
		}
		if left[i].Artifact != left[j].Artifact {
			return left[i].Artifact < left[j].Artifact
		}
		return left[i].LocalID < left[j].LocalID
	})
	return left
}

func truncateUTF8(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
