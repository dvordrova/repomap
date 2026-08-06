package componentmap

import (
	"encoding/json"
	"testing"
	"time"
)

// D230 review B1 regression: a small bundle where D4 equivalence coalescing
// absorbs EVERY component leaves no local remainder; the recoverable
// collision finding must still satisfy the partial-model provenance check
// (previously: "partial model source has inconsistent state").
func TestApplyD4FullCoalescenceKeepsPartialModelValid(t *testing.T) {
	t.Parallel()

	bundle := candidateBundleWithPackages(2)
	ids := candidateIDs(bundle.Candidates)
	result, err := Apply(bundle, Proposal{
		Version: ContractVersion,
		Subsystems: []ProposedSubsystem{{
			Name: "Small",
			Components: []ProposedComponent{
				{Name: "Role A", MemberIDs: ids[:1]},
				{Name: "Role B", MemberIDs: ids[:1]},
			},
		}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if result.Fallback {
		t.Fatalf("fallback fired: %#v", result.Diagnostics)
	}
	if err := result.Validate(bundle); err != nil {
		t.Fatalf("landscape validate after full coalescence: %v", err)
	}
	if !hasLandscapeDiagnostic(result.Diagnostics, "proposal.equivalent_member_set_collision") {
		t.Fatalf("collision not counted: %#v", result.Diagnostics)
	}
}

// D230 review B2 regression: same scenario through the wire synthesis path.
func TestSynthesisD4FullCoalescenceWirePath(t *testing.T) {
	t.Parallel()

	bundle := candidateBundleWithPackages(2)
	ids := candidateIDs(bundle.Candidates)
	catalog, err := buildSynthesisPrivateCatalog(bundle)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	refs := make([]SynthesisMemberRef, 0, len(ids[:1]))
	for _, id := range ids[:1] {
		ref, exists := catalog.membersByID[id]
		if !exists {
			t.Fatalf("no wire ref for %q", id.key())
		}
		refs = append(refs, ref)
	}
	wire := synthesisWireProposal{Records: []synthesisWireRecord{
		{Kind: synthesisWireSubsystemRecord, Ref: "g1", Name: "Small", Description: ""},
		{Kind: synthesisWireComponentRecord, SubsystemRef: "g1", Name: "Role A", Description: "", MemberRefs: refs, AnchorRefs: []SynthesisAnchorRef{}, Hypothesis: true},
		{Kind: synthesisWireComponentRecord, SubsystemRef: "g1", Name: "Role B", Description: "", MemberRefs: refs, AnchorRefs: []SynthesisAnchorRef{}, Hypothesis: true},
	}}
	response, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RecordSynthesisResponse(bundle, "revision-d4", "test", "test", time.Millisecond, response)
	if err != nil {
		t.Fatalf("RecordSynthesisResponse error (D4 wire-path regression): %v", err)
	}
	if result.Landscape.Fallback {
		t.Fatalf("fallback fired: %#v", result.Landscape.Diagnostics)
	}
	if err := result.Landscape.Validate(bundle); err != nil {
		t.Fatalf("landscape validate: %v", err)
	}
}
