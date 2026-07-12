package componentstudy

import (
	"encoding/json"
	"fmt"
)

type buildItem struct {
	kind           SelectionKind
	id             string
	rank           int
	limitReason    SelectionReason
	estimatedBytes int
	relatedIDs     []string
	add            func(Bundle) Bundle
}

// Build selects a deterministic, bounded model bundle from one component
// seed. It records every candidate decision, including candidates omitted by
// count, dependency, or byte limits.
func Build(seed Seed, budget Budget) (Bundle, SelectionTrace, error) {
	if err := seed.Validate(); err != nil {
		return Bundle{}, SelectionTrace{}, err
	}
	if err := budget.Validate(); err != nil {
		return Bundle{}, SelectionTrace{}, err
	}

	bundle := emptyBundle(seed)
	baseBytes, err := modelSize(bundle)
	if err != nil {
		return Bundle{}, SelectionTrace{}, err
	}
	if baseBytes > budget.MaxModelBytes {
		return Bundle{}, SelectionTrace{}, fmt.Errorf(
			"component study: model byte budget %d cannot hold bundle framing (%d bytes)",
			budget.MaxModelBytes,
			baseBytes,
		)
	}
	items, err := buildItems(seed)
	if err != nil {
		return Bundle{}, SelectionTrace{}, err
	}

	trace := SelectionTrace{
		Version:   SelectionTraceVersion,
		Budget:    budget,
		Decisions: make([]SelectionDecision, 0, len(items)),
	}
	includedIDs := map[string]struct{}{seed.Goal.ID: {}, seed.Component.ID: {}}
	counts := make(map[SelectionKind]int, 4)
	limits := map[SelectionKind]int{
		SelectionAnchor:   budget.MaxAnchors,
		SelectionFile:     budget.MaxFiles,
		SelectionSymbol:   budget.MaxSymbols,
		SelectionEvidence: budget.MaxEvidence,
	}
	for _, item := range items {
		reason := SelectionWithinBudget
		switch {
		case !referencesIncluded(item.relatedIDs, includedIDs):
			reason = SelectionMissingReference
		case counts[item.kind] >= limits[item.kind]:
			reason = item.limitReason
		}
		if reason != SelectionWithinBudget {
			trace.Decisions = appendDecision(trace.Decisions, item, false, reason)
			continue
		}

		candidateBundle := item.add(bundle)
		candidateBytes, err := modelSize(candidateBundle)
		if err != nil {
			return Bundle{}, SelectionTrace{}, err
		}
		if candidateBytes > budget.MaxModelBytes {
			trace.Decisions = appendDecision(
				trace.Decisions,
				item,
				false,
				SelectionModelBytesLimit,
			)
			continue
		}

		bundle = candidateBundle
		counts[item.kind]++
		includedIDs[item.id] = struct{}{}
		trace.Decisions = appendDecision(trace.Decisions, item, true, SelectionWithinBudget)
	}

	if err := bundle.Validate(); err != nil {
		return Bundle{}, SelectionTrace{}, err
	}
	trace.EstimatedModelBytes, err = modelSize(bundle)
	if err != nil {
		return Bundle{}, SelectionTrace{}, err
	}
	return bundle, trace, nil
}

func emptyBundle(seed Seed) Bundle {
	return Bundle{
		Version:   BundleVersion,
		RepoName:  seed.RepoName,
		Goal:      seed.Goal,
		Component: seed.Component,
		Anchors:   []AnchorCandidate{},
		Files:     []FileCandidate{},
		Symbols:   []SymbolCandidate{},
		Evidence:  []EvidenceCandidate{},
	}
}

func buildItems(seed Seed) ([]buildItem, error) {
	items := make([]buildItem, 0, len(seed.Anchors)+len(seed.Files)+len(seed.Symbols)+len(seed.Evidence))
	anchors := sortedCandidates(
		seed.Anchors,
		func(candidate AnchorCandidate) int { return candidate.Rank },
		func(candidate AnchorCandidate) string { return candidate.ID },
	)
	for _, candidate := range anchors {
		estimatedBytes, err := itemSize(candidate)
		if err != nil {
			return nil, err
		}
		items = append(items, buildItem{
			kind: SelectionAnchor, id: candidate.ID, rank: candidate.Rank,
			limitReason: SelectionAnchorLimit, estimatedBytes: estimatedBytes,
			add: func(bundle Bundle) Bundle {
				bundle.Anchors = appendCopy(bundle.Anchors, candidate)
				return bundle
			},
		})
	}
	files := sortedCandidates(
		seed.Files,
		func(candidate FileCandidate) int { return candidate.Rank },
		func(candidate FileCandidate) string { return candidate.ID },
	)
	for _, candidate := range files {
		estimatedBytes, err := itemSize(candidate)
		if err != nil {
			return nil, err
		}
		items = append(items, buildItem{
			kind: SelectionFile, id: candidate.ID, rank: candidate.Rank,
			limitReason: SelectionFileLimit, estimatedBytes: estimatedBytes,
			add: func(bundle Bundle) Bundle {
				bundle.Files = appendCopy(bundle.Files, candidate)
				return bundle
			},
		})
	}
	symbols := sortedCandidates(
		seed.Symbols,
		func(candidate SymbolCandidate) int { return candidate.Rank },
		func(candidate SymbolCandidate) string { return candidate.ID },
	)
	for _, candidate := range symbols {
		estimatedBytes, err := itemSize(candidate)
		if err != nil {
			return nil, err
		}
		items = append(items, buildItem{
			kind: SelectionSymbol, id: candidate.ID, rank: candidate.Rank,
			limitReason: SelectionSymbolLimit, estimatedBytes: estimatedBytes,
			add: func(bundle Bundle) Bundle {
				bundle.Symbols = appendCopy(bundle.Symbols, candidate)
				return bundle
			},
		})
	}
	evidenceItems := sortedCandidates(
		seed.Evidence,
		func(candidate EvidenceCandidate) int { return candidate.Rank },
		func(candidate EvidenceCandidate) string { return candidate.ID },
	)
	for _, candidate := range evidenceItems {
		candidate.RelatedIDs = sortedStrings(candidate.RelatedIDs)
		estimatedBytes, err := itemSize(candidate)
		if err != nil {
			return nil, err
		}
		items = append(items, buildItem{
			kind: SelectionEvidence, id: candidate.ID, rank: candidate.Rank,
			limitReason: SelectionEvidenceLimit, estimatedBytes: estimatedBytes,
			relatedIDs: candidate.RelatedIDs,
			add: func(bundle Bundle) Bundle {
				bundle.Evidence = appendCopy(bundle.Evidence, candidate)
				return bundle
			},
		})
	}
	return items, nil
}

func appendDecision(
	decisions []SelectionDecision,
	item buildItem,
	included bool,
	reason SelectionReason,
) []SelectionDecision {
	return append(decisions, SelectionDecision{
		Kind: item.kind, ID: item.id, Rank: item.rank,
		Included: included, Reason: reason, EstimatedBytes: item.estimatedBytes,
	})
}

func appendCopy[T any](values []T, value T) []T {
	result := make([]T, 0, len(values)+1)
	result = append(result, values...)
	return append(result, value)
}

func referencesIncluded(ids []string, included map[string]struct{}) bool {
	for _, id := range ids {
		if _, exists := included[id]; !exists {
			return false
		}
	}
	return true
}

func modelSize(bundle Bundle) (int, error) {
	data, err := json.Marshal(bundle)
	if err != nil {
		return 0, fmt.Errorf("component study: marshal bundle: %w", err)
	}
	return len(data), nil
}

func itemSize(item any) (int, error) {
	data, err := json.Marshal(item)
	if err != nil {
		return 0, fmt.Errorf("component study: estimate item bytes: %w", err)
	}
	// One byte accounts for the array delimiter or separator around the item.
	return len(data) + 1, nil
}
