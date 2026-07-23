package semanticdiscovery

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	LocalSequenceSupportMiss  = "local_sequence_support_missing"
	LocalSequenceScopeWidened = "local_sequence_scope_widened"
)

var (
	localSequenceConditionalPattern = regexp.MustCompile(
		"(?i)\\b(?:if|when|within|inside)\\b",
	)
	localSequenceWideningPattern = regexp.MustCompile(
		"(?i)\\b(?:runtime|system-wide|cross-component|cross-process|distributed|" +
			"always|every|must|guarantees?|regardless|unconditionally|necessarily)\\b",
	)
)

// LocalSequenceClaimError reports why a Golden claim failed the additional
// same-function/same-branch scope check. ClaimIndex is -1 when no qualifying
// sequence claim was returned.
type LocalSequenceClaimError struct {
	Code       string
	ClaimIndex int
}

func (err *LocalSequenceClaimError) Error() string {
	if err.ClaimIndex < 0 {
		return fmt.Sprintf("semantic discovery: %s", err.Code)
	}
	return fmt.Sprintf(
		"semantic discovery: claim %d: %s",
		err.ClaimIndex,
		err.Code,
	)
}

// ValidateLocalSequenceClaims keeps a conditional local sequence from being
// widened after the normal fan-in validator accepts a proposal. Temporal prose
// attached to the entry fact must cite the separate sequence fact, and the
// qualifying sequence claim must remain explicitly conditional and branch
// scoped.
func ValidateLocalSequenceClaims(
	bundle Bundle,
	proposal ArtifactProposal,
	entryFactID string,
	sequenceFactID string,
) error {
	if err := bundle.Validate(); err != nil {
		return err
	}
	known := factIndex(bundle)
	entry, exists := known[entryFactID]
	if !exists {
		return fmt.Errorf("semantic discovery: local sequence entry fact is unavailable")
	}
	sequence, exists := known[sequenceFactID]
	if !exists {
		return fmt.Errorf("semantic discovery: local sequence fact is unavailable")
	}
	if entry.Scope != FactScopeLocal || sequence.Scope != FactScopeLocal ||
		!factSupportsCapability(sequence, CapabilitySequence) ||
		!factSupportsCapability(sequence, CapabilityBranch) ||
		!factSupportsCapability(sequence, CapabilityDirectCall) {
		return fmt.Errorf("semantic discovery: local sequence fact has an invalid capability scope")
	}

	for claimIndex, claim := range proposal.Claims {
		hasEntry := containsID(claim.SupportIDs, entryFactID)
		hasSequence := containsID(claim.SupportIDs, sequenceFactID)
		temporal := sequencePattern.MatchString(claim.Text)
		if hasEntry && temporal && !hasSequence {
			return &LocalSequenceClaimError{
				Code:       LocalSequenceSupportMiss,
				ClaimIndex: claimIndex,
			}
		}
		if !hasSequence {
			continue
		}
		if claim.Basis == ClaimUnresolved || !temporal {
			continue
		}
		lower := strings.ToLower(claim.Text)
		if !strings.Contains(lower, "branch") ||
			!localSequenceConditionalPattern.MatchString(claim.Text) ||
			localSequenceWideningPattern.MatchString(claim.Text) ||
			!hasBoundedLexicalOverlap(claim.Text, []Fact{sequence}) {
			return &LocalSequenceClaimError{
				Code:       LocalSequenceScopeWidened,
				ClaimIndex: claimIndex,
			}
		}
	}
	return nil
}

func containsID(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
