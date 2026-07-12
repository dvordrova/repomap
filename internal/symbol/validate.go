package symbol

import (
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/dvordrova/repomap/internal/evidence"
)

var provenanceMetadataPattern = regexp.MustCompile(`^[A-Za-z0-9._/+:~-]+$`)

// Validate checks the self-contained symbol contract at capability boundaries.
// It does not re-run the analyzer or claim that static calls execute at runtime.
func (b Bundle) Validate() error {
	if b.Version != BundleVersion {
		return fmt.Errorf("symbol: unsupported bundle version %d", b.Version)
	}
	if strings.TrimSpace(b.RepoName) == "" || strings.TrimSpace(b.Query) == "" {
		return fmt.Errorf("symbol: repository name and query are required")
	}
	if b.Target.EvidenceID != "resolution-001" || b.Target.Entity.Name != b.Query {
		return fmt.Errorf("symbol: target is not the exact query resolution")
	}
	if b.Target.Entity.Kind != evidence.EntityFunction && b.Target.Entity.Kind != evidence.EntityMethod {
		return fmt.Errorf("symbol: target kind %q is not callable", b.Target.Entity.Kind)
	}
	if b.Target.Certainty != evidence.CertaintyStatic {
		return fmt.Errorf("symbol: exact target must have static certainty")
	}
	scenarios := make(map[string]struct{}, len(b.Scenarios))
	for index, scenario := range b.Scenarios {
		if !validMetadata(scenario.ID, 128) || scenario.Name != "" {
			return fmt.Errorf("symbol: scenarios[%d] has no id", index)
		}
		if _, exists := scenarios[scenario.ID]; exists {
			return fmt.Errorf("symbol: duplicate scenario %q", scenario.ID)
		}
		scenarios[scenario.ID] = struct{}{}
	}
	if err := validateFact("target", b.Target, scenarios); err != nil {
		return err
	}

	evidenceIDs := map[string]struct{}{b.Target.EvidenceID: {}}
	for index, candidate := range b.Candidates {
		expected := fmt.Sprintf("candidate-%03d", index+1)
		if candidate.EvidenceID != expected {
			return fmt.Errorf("symbol: candidates[%d] has invalid evidence id %q", index, candidate.EvidenceID)
		}
		if err := validateFact(fmt.Sprintf("candidates[%d]", index), candidate, scenarios); err != nil {
			return err
		}
		if err := addEvidenceID(evidenceIDs, candidate.EvidenceID); err != nil {
			return err
		}
	}
	if err := validateCalls("incoming_calls", "call-in", b.IncomingCalls, b.Target.Entity, true, scenarios, evidenceIDs); err != nil {
		return err
	}
	if err := validateCalls("outgoing_calls", "call-out", b.OutgoingCalls, b.Target.Entity, false, scenarios, evidenceIDs); err != nil {
		return err
	}

	for key, count := range b.Truncated {
		if key != "candidates" && key != "incoming_calls" && key != "outgoing_calls" {
			return fmt.Errorf("symbol: unknown truncation field %q", key)
		}
		if count <= 0 {
			return fmt.Errorf("symbol: truncation field %q must be positive", key)
		}
	}
	if len(b.Warnings) != 0 {
		if len(b.Warnings) < 2 || b.Warnings[0] != staticEvidenceWarning || b.Warnings[1] != fuzzyCandidateWarning {
			return fmt.Errorf("symbol: warnings are missing the static evidence contract")
		}
		for _, warning := range b.Warnings[2:] {
			if !allowedAnalyzerWarning(warning) {
				return fmt.Errorf("symbol: warnings contain non-contract analyzer text")
			}
		}
	}

	wantPaths := collectAllowedPaths(b)
	if len(wantPaths) != len(b.AllowedPaths) {
		return fmt.Errorf("symbol: allowed paths do not match retained facts")
	}
	for index, path := range b.AllowedPaths {
		if !validBundlePath(path) {
			return fmt.Errorf("symbol: invalid allowed path %q", path)
		}
		if index > 0 && b.AllowedPaths[index-1] >= path {
			return fmt.Errorf("symbol: allowed paths are not sorted and unique")
		}
		if wantPaths[index] != path {
			return fmt.Errorf("symbol: allowed paths do not match retained facts")
		}
	}
	return nil
}

func validateFact(field string, fact Fact, scenarios map[string]struct{}) error {
	if fact.EvidenceID == "" || fact.Entity.ID == "" || fact.Entity.Name == "" {
		return fmt.Errorf("symbol: %s identity is incomplete", field)
	}
	if !fact.Certainty.Valid() {
		return fmt.Errorf("symbol: %s has invalid certainty %q", field, fact.Certainty)
	}
	if fact.Entity.Location == nil || !validLocation(*fact.Entity.Location) {
		return fmt.Errorf("symbol: %s has no usable repository location", field)
	}
	if len(fact.Provenance) == 0 {
		return fmt.Errorf("symbol: %s has no provenance", field)
	}
	if err := validateProvenance(field, fact.Provenance); err != nil {
		return err
	}
	return validateScenarioRefs(field, fact.Scenarios, scenarios)
}

func validateCalls(field, prefix string, calls []CallFact, target evidence.Entity, incoming bool, scenarios map[string]struct{}, evidenceIDs map[string]struct{}) error {
	for index, call := range calls {
		expected := fmt.Sprintf("%s-%03d", prefix, index+1)
		if call.EvidenceID != expected {
			return fmt.Errorf("symbol: %s[%d] has invalid evidence id %q", field, index, call.EvidenceID)
		}
		if call.Caller.ID == "" || call.Caller.Name == "" || call.Callee.ID == "" || call.Callee.Name == "" {
			return fmt.Errorf("symbol: %s[%d] identity is incomplete", field, index)
		}
		if incoming && !reflect.DeepEqual(call.Callee, target) {
			return fmt.Errorf("symbol: %s[%d] does not call the target", field, index)
		}
		if !incoming && !reflect.DeepEqual(call.Caller, target) {
			return fmt.Errorf("symbol: %s[%d] does not originate at the target", field, index)
		}
		if call.Certainty != evidence.CertaintyStatic || len(call.Provenance) == 0 {
			return fmt.Errorf("symbol: %s[%d] is not a static provenanced call", field, index)
		}
		if err := validateProvenance(fmt.Sprintf("%s[%d]", field, index), call.Provenance); err != nil {
			return err
		}
		for _, entity := range []evidence.Entity{call.Caller, call.Callee} {
			if entity.Location == nil || !validLocation(*entity.Location) {
				return fmt.Errorf("symbol: %s[%d] has invalid entity location", field, index)
			}
		}
		if call.Callsite != nil && !validLocation(*call.Callsite) {
			return fmt.Errorf("symbol: %s[%d] has invalid callsite", field, index)
		}
		if err := validateScenarioRefs(fmt.Sprintf("%s[%d]", field, index), call.Scenarios, scenarios); err != nil {
			return err
		}
		if err := addEvidenceID(evidenceIDs, call.EvidenceID); err != nil {
			return err
		}
	}
	return nil
}

func validateProvenance(field string, values []evidence.Provenance) error {
	for index, provenance := range values {
		if !validMetadata(provenance.Provider, 64) ||
			(provenance.Version != "" && !validMetadata(provenance.Version, 128)) ||
			!validMetadata(provenance.Operation, 64) ||
			provenance.Detail != "" {
			return fmt.Errorf("symbol: %s provenance[%d] is incomplete", field, index)
		}
		if provenance.Location != nil && !validLocation(*provenance.Location) {
			return fmt.Errorf("symbol: %s provenance[%d] has invalid location", field, index)
		}
	}
	return nil
}

func validMetadata(value string, limit int) bool {
	return value != "" && len(value) <= limit && provenanceMetadataPattern.MatchString(value)
}

func validateScenarioRefs(field string, values []string, known map[string]struct{}) error {
	if known == nil && len(values) > 0 {
		return fmt.Errorf("symbol: %s references scenarios before scenario validation", field)
	}
	seen := make(map[string]struct{}, len(values))
	for _, scenarioID := range values {
		if _, exists := known[scenarioID]; !exists {
			return fmt.Errorf("symbol: %s references unknown scenario %q", field, scenarioID)
		}
		if _, exists := seen[scenarioID]; exists {
			return fmt.Errorf("symbol: %s repeats scenario %q", field, scenarioID)
		}
		seen[scenarioID] = struct{}{}
	}
	return nil
}

func addEvidenceID(known map[string]struct{}, id string) error {
	if _, exists := known[id]; exists {
		return fmt.Errorf("symbol: duplicate evidence id %q", id)
	}
	known[id] = struct{}{}
	return nil
}

func validLocation(location evidence.Location) bool {
	return validBundlePath(location.Path) && location.Line > 0 && location.Column >= 0
}

func validBundlePath(path string) bool {
	if path == "" {
		return false
	}
	local := filepath.FromSlash(path)
	return !filepath.IsAbs(local) && filepath.IsLocal(local) && filepath.ToSlash(filepath.Clean(local)) == path
}
