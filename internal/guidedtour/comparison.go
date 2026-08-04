package guidedtour

import (
	"fmt"
	"reflect"
	"sort"
)

const ComparisonVersion = 1

// StoryCoverage is a deterministic, ID-level view of how much of the selected
// candidate one validated story exposes. Evidence coverage includes only
// evidence reachable through selected beats; gap evidence is deliberately not
// mixed into it.
type StoryCoverage struct {
	CandidateID            string   `json:"candidate_id"`
	Steps                  int      `json:"steps"`
	ReferencedBeats        int      `json:"referenced_beats"`
	AvailableBeats         int      `json:"available_beats"`
	ReferencedBeatIDs      []string `json:"referenced_beat_ids"`
	AvailableBeatIDs       []string `json:"available_beat_ids"`
	ReferencedComponents   int      `json:"referenced_components"`
	AvailableComponents    int      `json:"available_components"`
	ReferencedComponentIDs []string `json:"referenced_component_ids"`
	AvailableComponentIDs  []string `json:"available_component_ids"`
	ReachableEvidence      int      `json:"reachable_evidence"`
	AvailableEvidence      int      `json:"available_evidence"`
	ReachableEvidenceIDs   []string `json:"reachable_evidence_ids"`
	AvailableEvidenceIDs   []string `json:"available_evidence_ids"`
	ReferencedGaps         int      `json:"referenced_gaps"`
	AvailableGaps          int      `json:"available_gaps"`
	ReferencedGapIDs       []string `json:"referenced_gap_ids"`
	AvailableGapIDs        []string `json:"available_gap_ids"`
}

// StrategyMetrics records observed experiment behavior. It intentionally has
// no scoring or winner-selection rule: request count is one measurement, not a
// proxy for explanation quality.
type StrategyMetrics struct {
	Strategy              string        `json:"strategy"`
	SemanticCalls         int           `json:"semantic_calls"`
	CacheHits             int           `json:"cache_hits"`
	RequestBytes          int           `json:"request_bytes"`
	ResponseBytes         int           `json:"response_bytes"`
	InputTokens           int           `json:"input_tokens"`
	OutputTokens          int           `json:"output_tokens"`
	PromptCacheHitTokens  int           `json:"prompt_cache_hit_tokens"`
	PromptCacheMissTokens int           `json:"prompt_cache_miss_tokens"`
	UnsupportedClaims     int           `json:"unsupported_claims"`
	WallMillis            int64         `json:"wall_ms"`
	LeafTasks             int           `json:"leaf_tasks"`
	LeafSucceeded         int           `json:"leaf_succeeded"`
	LeafInsufficient      int           `json:"leaf_insufficient"`
	LeafFailed            int           `json:"leaf_failed"`
	ValidationState       string        `json:"validation_state"`
	FailureReason         string        `json:"failure_reason,omitempty"`
	Coverage              StoryCoverage `json:"coverage"`
}

// Comparison is the persisted local comparison of two or more strategies.
// SelectedStrategy and Rationale may both be empty while the raw experiment is
// awaiting human assessment. Once selected, the strategy must name a variant
// and the rationale must be present.
type Comparison struct {
	Version          int               `json:"version"`
	BundleSHA256     string            `json:"bundle_sha256"`
	Model            string            `json:"model"`
	Profile          string            `json:"profile"`
	Variants         []StrategyMetrics `json:"variants"`
	SelectedStrategy string            `json:"selected_strategy,omitempty"`
	Rationale        string            `json:"rationale,omitempty"`
}

// EvaluateStoryCoverage verifies that story is exactly the locally
// materialized replay of a valid proposal for bundle before counting any IDs.
// It performs no repository reads or analysis.
func EvaluateStoryCoverage(bundle Bundle, story Story) (StoryCoverage, error) {
	proposal := proposalFromStory(story)
	record, err := EncodeRecord(bundle, proposal)
	if err != nil {
		return StoryCoverage{}, fmt.Errorf("guided tour comparison: validate story proposal: %w", err)
	}
	replayed, err := ReplayRecord(bundle, record)
	if err != nil {
		return StoryCoverage{}, fmt.Errorf("guided tour comparison: replay story record: %w", err)
	}
	if !reflect.DeepEqual(replayed, story) {
		return StoryCoverage{}, fmt.Errorf("guided tour comparison: story does not match its locally materialized record")
	}

	candidate, exists := findCandidate(bundle, story.CandidateID)
	if !exists {
		return StoryCoverage{}, fmt.Errorf("guided tour comparison: story candidate %q is unavailable", story.CandidateID)
	}

	referencedBeats := make(map[string]struct{})
	referencedComponents := make(map[string]struct{})
	reachableEvidence := make(map[string]struct{})
	for _, step := range story.Steps {
		addStrings(referencedBeats, step.BeatIDs)
		addStrings(referencedComponents, step.ComponentIDs)
		addEvidenceIDs(reachableEvidence, step.Evidence)
	}

	availableBeats := make(map[string]struct{}, len(candidate.Beats))
	availableComponents := make(map[string]struct{})
	availableEvidence := make(map[string]struct{})
	for _, beat := range candidate.Beats {
		availableBeats[beat.ID] = struct{}{}
		addStrings(availableComponents, beat.ComponentIDs)
		addEvidenceIDs(availableEvidence, beat.Evidence)
	}

	referencedGaps := make(map[string]struct{})
	for _, summary := range story.GapSummary {
		addStrings(referencedGaps, summary.GapIDs)
	}
	availableGaps := make(map[string]struct{}, len(candidate.Gaps))
	for _, gap := range candidate.Gaps {
		availableGaps[gap.ID] = struct{}{}
	}

	coverage := StoryCoverage{
		CandidateID:            candidate.ID,
		Steps:                  len(story.Steps),
		ReferencedBeatIDs:      sortedSet(referencedBeats),
		AvailableBeatIDs:       sortedSet(availableBeats),
		ReferencedComponentIDs: sortedSet(referencedComponents),
		AvailableComponentIDs:  sortedSet(availableComponents),
		ReachableEvidenceIDs:   sortedSet(reachableEvidence),
		AvailableEvidenceIDs:   sortedSet(availableEvidence),
		ReferencedGapIDs:       sortedSet(referencedGaps),
		AvailableGapIDs:        sortedSet(availableGaps),
	}
	coverage.ReferencedBeats = len(coverage.ReferencedBeatIDs)
	coverage.AvailableBeats = len(coverage.AvailableBeatIDs)
	coverage.ReferencedComponents = len(coverage.ReferencedComponentIDs)
	coverage.AvailableComponents = len(coverage.AvailableComponentIDs)
	coverage.ReachableEvidence = len(coverage.ReachableEvidenceIDs)
	coverage.AvailableEvidence = len(coverage.AvailableEvidenceIDs)
	coverage.ReferencedGaps = len(coverage.ReferencedGapIDs)
	coverage.AvailableGaps = len(coverage.AvailableGapIDs)
	return coverage, nil
}

func (comparison Comparison) Validate() error {
	if comparison.Version != ComparisonVersion {
		return fmt.Errorf("guided tour comparison: unsupported version %d", comparison.Version)
	}
	if !validSHA256(comparison.BundleSHA256) {
		return fmt.Errorf("guided tour comparison: bundle hash is malformed")
	}
	if err := validateText("comparison model", comparison.Model, maxNameBytes, true); err != nil {
		return fmt.Errorf("guided tour comparison: %w", err)
	}
	if err := validateText("comparison profile", comparison.Profile, maxNameBytes, true); err != nil {
		return fmt.Errorf("guided tour comparison: %w", err)
	}
	if len(comparison.Variants) < 2 {
		return fmt.Errorf("guided tour comparison: at least two variants are required")
	}
	strategies := make(map[string]struct{}, len(comparison.Variants))
	for index, variant := range comparison.Variants {
		if err := variant.validate(); err != nil {
			return fmt.Errorf("guided tour comparison: variants[%d]: %w", index, err)
		}
		if _, duplicate := strategies[variant.Strategy]; duplicate {
			return fmt.Errorf("guided tour comparison: duplicate strategy %q", variant.Strategy)
		}
		strategies[variant.Strategy] = struct{}{}
	}
	if comparison.SelectedStrategy == "" {
		if comparison.Rationale != "" {
			return fmt.Errorf("guided tour comparison: rationale requires a selected strategy")
		}
		return nil
	}
	if _, exists := strategies[comparison.SelectedStrategy]; !exists {
		return fmt.Errorf("guided tour comparison: selected strategy %q is not a variant", comparison.SelectedStrategy)
	}
	if err := validateText("comparison rationale", comparison.Rationale, maxSummaryBytes, true); err != nil {
		return fmt.Errorf("guided tour comparison: %w", err)
	}
	return nil
}

func (metrics StrategyMetrics) validate() error {
	if err := validateOpaque("comparison strategy", metrics.Strategy); err != nil {
		return err
	}
	if err := validateOpaque("comparison validation state", metrics.ValidationState); err != nil {
		return err
	}
	if metrics.SemanticCalls < 0 || metrics.CacheHits < 0 || metrics.RequestBytes < 0 ||
		metrics.ResponseBytes < 0 || metrics.InputTokens < 0 || metrics.OutputTokens < 0 ||
		metrics.PromptCacheHitTokens < 0 || metrics.PromptCacheMissTokens < 0 ||
		metrics.UnsupportedClaims < 0 || metrics.WallMillis < 0 ||
		metrics.LeafTasks < 0 || metrics.LeafSucceeded < 0 ||
		metrics.LeafInsufficient < 0 || metrics.LeafFailed < 0 {
		return fmt.Errorf("metrics cannot be negative")
	}
	if metrics.LeafSucceeded+metrics.LeafFailed != metrics.LeafTasks {
		return fmt.Errorf("leaf outcome counts do not match leaf task count")
	}
	if metrics.LeafInsufficient > metrics.LeafSucceeded {
		return fmt.Errorf("insufficient leaf count exceeds valid leaf count")
	}
	if metrics.FailureReason != "" {
		if err := validateText("comparison failure reason", metrics.FailureReason, maxSummaryBytes, false); err != nil {
			return err
		}
	}
	if metrics.Coverage.CandidateID == "" {
		if !reflect.DeepEqual(metrics.Coverage, StoryCoverage{}) {
			return fmt.Errorf("empty coverage candidate must use zero coverage")
		}
		if metrics.FailureReason == "" {
			return fmt.Errorf("zero coverage requires a failure reason")
		}
		return nil
	}
	return metrics.Coverage.validate()
}

func (coverage StoryCoverage) validate() error {
	if err := validateOpaque("coverage candidate id", coverage.CandidateID); err != nil {
		return err
	}
	if coverage.Steps < minProposalSteps || coverage.Steps > maxProposalSteps {
		return fmt.Errorf("coverage steps must be between %d and %d", minProposalSteps, maxProposalSteps)
	}
	if coverage.ReferencedBeats < minReferencedProposalBeats || coverage.AvailableBeats < minReferencedProposalBeats {
		return fmt.Errorf("coverage must contain at least %d referenced and available beats", minReferencedProposalBeats)
	}
	sets := []struct {
		name      string
		count     int
		ids       []string
		available []string
	}{
		{name: "referenced beats", count: coverage.ReferencedBeats, ids: coverage.ReferencedBeatIDs, available: coverage.AvailableBeatIDs},
		{name: "available beats", count: coverage.AvailableBeats, ids: coverage.AvailableBeatIDs},
		{name: "referenced components", count: coverage.ReferencedComponents, ids: coverage.ReferencedComponentIDs, available: coverage.AvailableComponentIDs},
		{name: "available components", count: coverage.AvailableComponents, ids: coverage.AvailableComponentIDs},
		{name: "reachable evidence", count: coverage.ReachableEvidence, ids: coverage.ReachableEvidenceIDs, available: coverage.AvailableEvidenceIDs},
		{name: "available evidence", count: coverage.AvailableEvidence, ids: coverage.AvailableEvidenceIDs},
		{name: "referenced gaps", count: coverage.ReferencedGaps, ids: coverage.ReferencedGapIDs, available: coverage.AvailableGapIDs},
		{name: "available gaps", count: coverage.AvailableGaps, ids: coverage.AvailableGapIDs},
	}
	for _, set := range sets {
		if set.count != len(set.ids) {
			return fmt.Errorf("%s count does not match exact ids", set.name)
		}
		if err := validateSortedIDs(set.name, set.ids); err != nil {
			return err
		}
		if set.available != nil && !isSubset(set.ids, set.available) {
			return fmt.Errorf("%s are not a subset of available ids", set.name)
		}
	}
	return nil
}

func proposalFromStory(story Story) Proposal {
	proposal := Proposal{
		Version:     ProposalVersion,
		CandidateID: story.CandidateID,
		Title:       story.Title,
		Summary:     story.Summary,
		Steps:       make([]ProposedStep, 0, len(story.Steps)),
		GapSummary:  make([]ProposedGapSummary, 0, len(story.GapSummary)),
	}
	for _, step := range story.Steps {
		proposal.Steps = append(proposal.Steps, ProposedStep{
			Title:       step.Title,
			Explanation: step.Explanation,
			BeatIDs:     append([]string{}, step.BeatIDs...),
		})
	}
	for _, summary := range story.GapSummary {
		proposal.GapSummary = append(proposal.GapSummary, ProposedGapSummary{
			Explanation: summary.Explanation,
			GapIDs:      append([]string{}, summary.GapIDs...),
		})
	}
	return proposal
}

func addEvidenceIDs(destination map[string]struct{}, values []EvidenceRef) {
	for _, value := range values {
		destination[value.ID] = struct{}{}
	}
}

func validateSortedIDs(field string, values []string) error {
	if err := validateIDList(field, values); err != nil {
		return err
	}
	if !sort.StringsAreSorted(values) {
		return fmt.Errorf("%s must be sorted", field)
	}
	return nil
}

func isSubset(values, available []string) bool {
	known := make(map[string]struct{}, len(available))
	for _, value := range available {
		known[value] = struct{}{}
	}
	for _, value := range values {
		if _, exists := known[value]; !exists {
			return false
		}
	}
	return true
}
