package semanticdiscovery

import (
	"fmt"
	"sort"
)

// EncodeRecord creates the metrics-free, canonical replay contract. Provider
// timings and token counters belong to the orchestration stage artifact, not
// to semantic truth or replay identity.
func EncodeRecord(
	bundle Bundle,
	opportunity OpportunityProposal,
	selected []OpportunityCandidate,
	leaves []LeafResult,
	fanIn FanInArtifact,
) ([]byte, error) {
	_, fanInContext, err := validateLeafResults(bundle, leaves)
	if err != nil {
		return nil, err
	}
	canonicalFanIn, err := canonicalizeFanInArtifact(fanInContext, fanIn)
	if err != nil {
		return nil, fmt.Errorf("semantic discovery: canonicalize record verdicts: %w", err)
	}
	selectedIDs := make([]string, 0, len(selected))
	for _, candidate := range selected {
		selectedIDs = append(selectedIDs, candidate.ID)
	}
	record := Record{
		Version:              RecordVersion,
		Opportunity:          opportunity,
		SelectedCandidateIDs: selectedIDs,
		Leaves:               append([]LeafResult(nil), leaves...),
		FanIn:                canonicalFanIn,
	}
	bundleHash, _, err := BundleHash(bundle)
	if err != nil {
		return nil, err
	}
	record.BundleSHA256 = bundleHash
	if _, err := validateRecord(bundle, record); err != nil {
		return nil, err
	}
	record = canonicalRecord(record)
	_, encoded, err := hashJSON("semantic record", record)
	if err != nil {
		return nil, err
	}
	if len(encoded) > maxRecordBytes {
		return nil, fmt.Errorf("semantic discovery: record is too large")
	}
	return encoded, nil
}

func DecodeRecord(raw []byte) (Record, error) {
	var record Record
	if err := decodeStrict(raw, &record, maxRecordBytes); err != nil {
		return Record{}, fmt.Errorf("semantic discovery: invalid replay record json: %w", err)
	}
	return record, nil
}

// ReplayRecord accepts only a record still bound to the exact current bundle
// and re-runs every local opportunity, leaf, lineage, and retained proposal
// validator. A record may contain a validated partial fan-in result.
func ReplayRecord(bundle Bundle, raw []byte) ([]Artifact, error) {
	record, err := DecodeRecord(raw)
	if err != nil {
		return nil, err
	}
	if _, err := validateRecord(bundle, record); err != nil {
		return nil, err
	}
	return MaterializePartialArtifacts(bundle, record.Leaves, record.FanIn)
}

func RecordHash(record Record) (string, []byte, error) {
	if record.Version != RecordVersion {
		return "", nil, fmt.Errorf("semantic discovery: unsupported record version %d", record.Version)
	}
	return hashJSON("semantic record", canonicalRecord(record))
}

func validateRecord(bundle Bundle, record Record) ([]OpportunityCandidate, error) {
	if record.Version != RecordVersion {
		return nil, fmt.Errorf("semantic discovery: unsupported record version %d", record.Version)
	}
	bundleHash, _, err := BundleHash(bundle)
	if err != nil {
		return nil, err
	}
	if record.BundleSHA256 != bundleHash {
		return nil, fmt.Errorf("semantic discovery: replay bundle hash does not match current facts")
	}
	if err := ValidateOpportunityProposal(bundle, record.Opportunity); err != nil {
		return nil, err
	}
	if len(record.SelectedCandidateIDs) == 0 || len(record.SelectedCandidateIDs) > MaxSelectedCandidates {
		return nil, fmt.Errorf("semantic discovery: record selected candidate count is invalid")
	}
	if err := validateIDList("record selected candidate ids", record.SelectedCandidateIDs, true); err != nil {
		return nil, err
	}
	expected, err := SelectOpportunities(bundle, record.Opportunity, len(record.SelectedCandidateIDs))
	if err != nil {
		return nil, err
	}
	expectedIDs := make([]string, 0, len(expected))
	selectedByID := make(map[string]OpportunityCandidate, len(expected))
	for _, candidate := range expected {
		expectedIDs = append(expectedIDs, candidate.ID)
		selectedByID[candidate.ID] = candidate
	}
	if !equalOrderedStrings(record.SelectedCandidateIDs, expectedIDs) {
		return nil, fmt.Errorf("semantic discovery: record selection does not match deterministic selection")
	}
	if len(record.Leaves) == 0 {
		return nil, fmt.Errorf("semantic discovery: record contains no validated leaves")
	}
	for _, result := range record.Leaves {
		if _, selected := selectedByID[result.Task.Candidate.ID]; !selected {
			return nil, fmt.Errorf("semantic discovery: record leaf candidate was not selected")
		}
	}
	if _, _, err := validateLeafResults(bundle, record.Leaves); err != nil {
		return nil, err
	}
	if err := ValidatePartialFanInArtifact(bundle, record.Leaves, record.FanIn); err != nil {
		return nil, err
	}
	return expected, nil
}

func canonicalRecord(record Record) Record {
	result := record
	result.Opportunity.Candidates = append(
		[]OpportunityCandidate(nil),
		record.Opportunity.Candidates...,
	)
	for index := range result.Opportunity.Candidates {
		result.Opportunity.Candidates[index] = canonicalOpportunityCandidate(
			result.Opportunity.Candidates[index],
		)
	}
	sort.Slice(result.Opportunity.Candidates, func(i, j int) bool {
		return result.Opportunity.Candidates[i].ID < result.Opportunity.Candidates[j].ID
	})
	result.SelectedCandidateIDs = append([]string(nil), record.SelectedCandidateIDs...)
	result.Leaves = append([]LeafResult(nil), record.Leaves...)
	for index := range result.Leaves {
		result.Leaves[index].Task = canonicalLeafTask(result.Leaves[index].Task)
		result.Leaves[index].Artifact = NormalizeLeafArtifact(result.Leaves[index].Artifact)
	}
	sort.Slice(result.Leaves, func(i, j int) bool {
		return result.Leaves[i].Task.ID < result.Leaves[j].Task.ID
	})
	result.FanIn = NormalizeFanInArtifact(record.FanIn)
	return result
}

func equalOrderedStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
