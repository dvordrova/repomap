package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const (
	GoldenMechanismFactsFile        = "golden_mechanism_facts.json"
	GoldenMechanismRecordFile       = "golden_mechanism_semantic.json"
	semanticSupplementLegacyVersion = 1
	semanticSupplementVersion       = 2
	maxSemanticSupplementFileBytes  = 256 << 10
)

var semanticSupplementOpaqueID = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,127}$`)

// SemanticSupplementCandidateBinding binds one replayed candidate to its
// locally saved probe and the exact supplemental facts that probe supplied.
type SemanticSupplementCandidateBinding struct {
	CandidateID string   `json:"candidate_id"`
	ProbeSHA256 string   `json:"probe_sha256"`
	FactIDs     []string `json:"fact_ids"`
}

// SemanticSupplement is the publishable bridge between bounded locally saved
// probes and the existing Semantic Discovery replay bundle. Raw source and
// model output stay outside this record; only validated local Facts cross the
// boundary. CandidateID and ProbeSHA256 are retained solely for version 1
// replay compatibility.
type SemanticSupplement struct {
	Version              int                                  `json:"version"`
	CandidateID          string                               `json:"candidate_id,omitempty"`
	ProbeSHA256          string                               `json:"probe_sha256,omitempty"`
	CandidateBindings    []SemanticSupplementCandidateBinding `json:"candidate_bindings,omitempty"`
	BaseBundleSHA256     string                               `json:"base_bundle_sha256"`
	EnrichedBundleSHA256 string                               `json:"enriched_bundle_sha256"`
	Facts                []semanticdiscovery.Fact             `json:"facts"`
}

// PrepareSemanticSupplement validates facts first against the saved report
// without enrichment and then against the exact locally focused bundle that
// replay will rebuild. Failed preparation restores the previous enrichment.
func PrepareSemanticSupplement(
	data *ReportData,
	candidateID string,
	probeSHA256 string,
	facts []semanticdiscovery.Fact,
) (SemanticSupplement, semanticdiscovery.Bundle, error) {
	factIDs := make([]string, 0, len(facts))
	for _, fact := range facts {
		factIDs = append(factIDs, fact.ID)
	}
	return PrepareSemanticSupplementSet(
		data,
		[]SemanticSupplementCandidateBinding{{
			CandidateID: candidateID,
			ProbeSHA256: probeSHA256,
			FactIDs:     factIDs,
		}},
		facts,
	)
}

// PrepareSemanticSupplementSet validates a bounded set of candidate bindings
// first against the saved report without enrichment and then against the exact
// locally focused bundle that replay will rebuild. Failed preparation restores
// the previous enrichment; successful preparation leaves the enriched facts in
// place for record generation.
func PrepareSemanticSupplementSet(
	data *ReportData,
	bindings []SemanticSupplementCandidateBinding,
	facts []semanticdiscovery.Fact,
) (SemanticSupplement, semanticdiscovery.Bundle, error) {
	if data == nil {
		return SemanticSupplement{}, semanticdiscovery.Bundle{}, fmt.Errorf(
			"semantic supplement: report data is required",
		)
	}
	facts = cloneSemanticSupplementFacts(facts)
	bindings = canonicalSemanticSupplementBindings(bindings)
	if err := validateSemanticSupplementFacts(facts); err != nil {
		return SemanticSupplement{}, semanticdiscovery.Bundle{}, err
	}
	if err := validateSemanticSupplementBindings(bindings, facts); err != nil {
		return SemanticSupplement{}, semanticdiscovery.Bundle{}, err
	}

	previous := data.SemanticSupplementalFacts
	data.SemanticSupplementalFacts = nil
	base, err := BuildSemanticDiscoveryBundle(data)
	if err != nil {
		data.SemanticSupplementalFacts = previous
		return SemanticSupplement{}, semanticdiscovery.Bundle{}, err
	}
	baseSHA, _, err := semanticdiscovery.BundleHash(base)
	if err != nil {
		data.SemanticSupplementalFacts = previous
		return SemanticSupplement{}, semanticdiscovery.Bundle{}, err
	}

	data.SemanticSupplementalFacts = cloneSemanticSupplementFacts(facts)
	enriched, err := BuildSemanticDiscoveryBundle(data)
	if err != nil {
		data.SemanticSupplementalFacts = previous
		return SemanticSupplement{}, semanticdiscovery.Bundle{}, err
	}
	enrichedSHA, _, err := semanticdiscovery.BundleHash(enriched)
	if err != nil {
		data.SemanticSupplementalFacts = previous
		return SemanticSupplement{}, semanticdiscovery.Bundle{}, err
	}
	record := SemanticSupplement{
		Version:              semanticSupplementVersion,
		CandidateBindings:    bindings,
		BaseBundleSHA256:     baseSHA,
		EnrichedBundleSHA256: enrichedSHA,
		Facts:                cloneSemanticSupplementFacts(facts),
	}
	if err := validateSemanticSupplement(record); err != nil {
		data.SemanticSupplementalFacts = previous
		return SemanticSupplement{}, semanticdiscovery.Bundle{}, err
	}
	return record, enriched, nil
}

func loadSemanticSupplement(data *ReportData, path string) string {
	_, warning := loadSemanticSupplementRecord(data, path)
	return warning
}

func loadSemanticSupplementRecord(
	data *ReportData,
	path string,
) (SemanticSupplement, string) {
	if data == nil {
		return SemanticSupplement{}, "golden mechanism unavailable: report data is required"
	}
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return SemanticSupplement{}, ""
		}
		return SemanticSupplement{}, "golden mechanism unavailable: cannot inspect saved facts"
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxSemanticSupplementFileBytes {
		return SemanticSupplement{}, "golden mechanism unavailable: saved facts are not a bounded regular file"
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return SemanticSupplement{}, "golden mechanism unavailable: cannot read saved facts"
	}
	var record SemanticSupplement
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return SemanticSupplement{}, "golden mechanism unavailable: saved facts contain invalid json"
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return SemanticSupplement{}, "golden mechanism unavailable: saved facts contain invalid json"
	}
	if err := validateSemanticSupplement(record); err != nil {
		return SemanticSupplement{}, "golden mechanism unavailable: saved facts are invalid"
	}

	previous := data.SemanticSupplementalFacts
	data.SemanticSupplementalFacts = nil
	base, err := BuildSemanticDiscoveryBundle(data)
	if err != nil {
		data.SemanticSupplementalFacts = previous
		return SemanticSupplement{}, "golden mechanism unavailable: current base facts are invalid"
	}
	baseSHA, _, err := semanticdiscovery.BundleHash(base)
	if err != nil || baseSHA != record.BaseBundleSHA256 {
		data.SemanticSupplementalFacts = previous
		return SemanticSupplement{}, "golden mechanism unavailable: saved probe facts are stale"
	}
	data.SemanticSupplementalFacts = cloneSemanticSupplementFacts(record.Facts)
	enriched, err := BuildSemanticDiscoveryBundle(data)
	if err != nil {
		data.SemanticSupplementalFacts = previous
		return SemanticSupplement{}, "golden mechanism unavailable: enriched facts are invalid"
	}
	enrichedSHA, _, err := semanticdiscovery.BundleHash(enriched)
	if err != nil || enrichedSHA != record.EnrichedBundleSHA256 {
		data.SemanticSupplementalFacts = previous
		return SemanticSupplement{}, "golden mechanism unavailable: enriched fact projection changed"
	}
	return record, ""
}

func validateSemanticSupplement(record SemanticSupplement) error {
	if record.Version != semanticSupplementLegacyVersion &&
		record.Version != semanticSupplementVersion {
		return fmt.Errorf("semantic supplement: unsupported version %d", record.Version)
	}
	if !validSemanticSupplementSHA256(record.BaseBundleSHA256) ||
		!validSemanticSupplementSHA256(record.EnrichedBundleSHA256) {
		return fmt.Errorf("semantic supplement: invalid sha256")
	}
	if err := validateSemanticSupplementFacts(record.Facts); err != nil {
		return err
	}
	switch record.Version {
	case semanticSupplementLegacyVersion:
		if len(record.CandidateBindings) != 0 {
			return fmt.Errorf("semantic supplement: legacy record has candidate bindings")
		}
		if !semanticSupplementOpaqueID.MatchString(record.CandidateID) {
			return fmt.Errorf("semantic supplement: invalid candidate id")
		}
		if !validSemanticSupplementSHA256(record.ProbeSHA256) {
			return fmt.Errorf("semantic supplement: invalid probe sha256")
		}
	case semanticSupplementVersion:
		if record.CandidateID != "" || record.ProbeSHA256 != "" {
			return fmt.Errorf("semantic supplement: current record has legacy candidate binding")
		}
		if err := validateSemanticSupplementBindings(record.CandidateBindings, record.Facts); err != nil {
			return err
		}
	}
	return nil
}

func validateSemanticSupplementFacts(facts []semanticdiscovery.Fact) error {
	if len(facts) == 0 || len(facts) > maxSemanticDiscoverySupplementalFacts {
		return fmt.Errorf(
			"semantic supplement: fact count must be between 1 and %d",
			maxSemanticDiscoverySupplementalFacts,
		)
	}
	seen := make(map[string]struct{}, len(facts))
	for _, fact := range facts {
		if _, duplicate := seen[fact.ID]; duplicate {
			return fmt.Errorf("semantic supplement: duplicate fact id")
		}
		seen[fact.ID] = struct{}{}
	}
	return nil
}

func validateSemanticSupplementBindings(
	bindings []SemanticSupplementCandidateBinding,
	facts []semanticdiscovery.Fact,
) error {
	if len(bindings) == 0 || len(bindings) > semanticdiscovery.MaxSelectedCandidates {
		return fmt.Errorf(
			"semantic supplement: candidate binding count must be between 1 and %d",
			semanticdiscovery.MaxSelectedCandidates,
		)
	}
	knownFacts := make(map[string]struct{}, len(facts))
	for _, fact := range facts {
		knownFacts[fact.ID] = struct{}{}
	}
	seenCandidates := make(map[string]struct{}, len(bindings))
	coveredFacts := make(map[string]struct{}, len(facts))
	for _, binding := range bindings {
		if !semanticSupplementOpaqueID.MatchString(binding.CandidateID) {
			return fmt.Errorf("semantic supplement: invalid candidate id")
		}
		if _, duplicate := seenCandidates[binding.CandidateID]; duplicate {
			return fmt.Errorf("semantic supplement: duplicate candidate id")
		}
		seenCandidates[binding.CandidateID] = struct{}{}
		if !validSemanticSupplementSHA256(binding.ProbeSHA256) {
			return fmt.Errorf("semantic supplement: invalid probe sha256")
		}
		if len(binding.FactIDs) == 0 || len(binding.FactIDs) > len(facts) {
			return fmt.Errorf("semantic supplement: invalid candidate fact count")
		}
		seenBindingFacts := make(map[string]struct{}, len(binding.FactIDs))
		for _, factID := range binding.FactIDs {
			if _, duplicate := seenBindingFacts[factID]; duplicate {
				return fmt.Errorf("semantic supplement: duplicate candidate fact id")
			}
			seenBindingFacts[factID] = struct{}{}
			if _, known := knownFacts[factID]; !known {
				return fmt.Errorf("semantic supplement: candidate references unknown fact id")
			}
			coveredFacts[factID] = struct{}{}
		}
	}
	if len(coveredFacts) != len(knownFacts) {
		return fmt.Errorf("semantic supplement: candidate bindings do not cover all facts")
	}
	return nil
}

func semanticSupplementCandidateIDs(record SemanticSupplement) ([]string, error) {
	if err := validateSemanticSupplement(record); err != nil {
		return nil, err
	}
	if record.Version == semanticSupplementLegacyVersion {
		return []string{record.CandidateID}, nil
	}
	result := make([]string, 0, len(record.CandidateBindings))
	for _, binding := range record.CandidateBindings {
		result = append(result, binding.CandidateID)
	}
	sort.Strings(result)
	return result, nil
}

func canonicalSemanticSupplementBindings(
	bindings []SemanticSupplementCandidateBinding,
) []SemanticSupplementCandidateBinding {
	result := make([]SemanticSupplementCandidateBinding, len(bindings))
	for index, binding := range bindings {
		result[index] = binding
		result[index].FactIDs = append([]string(nil), binding.FactIDs...)
		sort.Strings(result[index].FactIDs)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CandidateID < result[j].CandidateID
	})
	return result
}

func cloneSemanticSupplementFacts(facts []semanticdiscovery.Fact) []semanticdiscovery.Fact {
	result := make([]semanticdiscovery.Fact, len(facts))
	for index, fact := range facts {
		result[index] = fact
		result[index].Keywords = append([]string(nil), fact.Keywords...)
		result[index].Capabilities = append([]semanticdiscovery.Capability(nil), fact.Capabilities...)
		result[index].Focus.ComponentIDs = append([]string(nil), fact.Focus.ComponentIDs...)
		result[index].Focus.FlowIDs = append([]string(nil), fact.Focus.FlowIDs...)
		result[index].Focus.SurfaceIDs = append([]string(nil), fact.Focus.SurfaceIDs...)
		result[index].Evidence = append([]semanticdiscovery.EvidenceRef(nil), fact.Evidence...)
		if fact.Source != nil {
			source := *fact.Source
			result[index].Source = &source
		}
	}
	return result
}

func validSemanticSupplementSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range strings.ToLower(value) {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
