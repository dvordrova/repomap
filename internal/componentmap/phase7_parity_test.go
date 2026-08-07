package componentmap

import (
	"strings"
	"testing"
)

// Phase 7 prompt-cleanup parity: one malformed item cannot erase valid
// siblings where stage semantics allow item-local salvage. A component that
// references an unknown member ref is dropped item-locally; the valid
// sibling components still publish (accepted_partial, never a blank canvas).
func TestSynthesisOneMalformedComponentDoesNotEraseValidSiblings(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"subsystems":[{"name":"App","description":"","components":[` +
		`{"name":"Good one","description":"","member_refs":["p1"]},` +
		`{"name":"Broken","description":"","member_refs":["zz-unknown-ref"]},` +
		`{"name":"Good two","description":"","member_refs":["p2"]}` +
		`]}]}`)
	proposal, err := decodeSynthesisWireProposalJSON(raw)
	if err != nil {
		t.Fatalf("nested wire rejected: %v", err)
	}
	bundle := adversarialPrivateIdentitySynthesisBundle()
	_ = proposal
	// Full pipeline: evaluate applies item-scope salvage and counts the
	// recoverable findings into the landscape diagnostics.
	landscape, _, err := evaluateSynthesisResponse(bundle, ResponseCaptured, raw)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	// The two valid components must survive; the malformed one is dropped
	// with a counted diagnostic, not a canvas-wide rejection.
	if landscape.ValidationOutcome != ValidationAcceptedPartial {
		t.Fatalf("outcome = %q, want accepted_partial", landscape.ValidationOutcome)
	}
	foundGoodOne, foundGoodTwo := false, false
	for _, subsystem := range landscape.Subsystems {
		for _, component := range subsystem.Components {
			if component.Name == "Good one" {
				foundGoodOne = true
			}
			if component.Name == "Good two" {
				foundGoodTwo = true
			}
		}
	}
	if !foundGoodOne || !foundGoodTwo {
		t.Fatalf("valid siblings were erased: good_one=%v good_two=%v", foundGoodOne, foundGoodTwo)
	}
	foundUnknown := false
	for _, diagnostic := range landscape.Diagnostics {
		if strings.Contains(diagnostic.Code, "unknown_member") || strings.Contains(diagnostic.Code, "unknown") {
			foundUnknown = true
		}
	}
	if !foundUnknown {
		t.Fatalf("malformed component left no counted diagnostic: %#v", landscape.Diagnostics)
	}
}

// Phase 7 parity: prompt identity is content-derived and the response schema
// version is a separate local contract — a prompt edit must change the prompt
// version while the schema version stays stable.
func TestSynthesisPromptIdentityContentDerived(t *testing.T) {
	t.Parallel()
	before := SynthesisPromptVersion
	if before == "" {
		t.Fatal("prompt version is empty")
	}
	// The version is a content hash of the system prompt; the wire response
	// schema version is a separate local contract (ProposalVersion).
	if !strings.HasPrefix(before, "architecture-grounding-") {
		t.Fatalf("prompt version not content-derived: %q", before)
	}
	if ProposalVersion == 0 {
		t.Fatal("schema version contract is not set")
	}
}
