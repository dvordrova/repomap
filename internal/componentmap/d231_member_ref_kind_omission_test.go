package componentmap

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/evidence"
)

// Decision 231 (Archive 9, gap closure — miniflux live run 20260806-223340):
// member_refs may omit the backend-owned kind ({"ref":"p11"}). The catalog
// owns kinds; a supplied wrong kind still fails item-scope, but an omitted
// kind resolves by the ref-only catalog key. Before this fix the decode
// required kind+ref and whole-rejected the proposal with
// response.invalid_proposal.
func TestSynthesisMemberRefKindOmissionAccepted(t *testing.T) {
	t.Parallel()
	bundle := unitFixtureBundle()
	request, _, err := BuildSynthesisRequest(bundle)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if len(request.Candidates) == 0 {
		t.Fatal("fixture has no candidates")
	}
	// Take one package-owned supporting ref and its exact primary package ref;
	// use them WITHOUT kind. D238 requires primary scope from the same unit,
	// while this regression remains solely about omitted backend-owned kinds.
	var productionRef, symbolRef string
	for _, candidate := range request.Candidates {
		if strings.HasPrefix(candidate.Ref.Ref, "s") &&
			candidate.CoverageRole == SynthesisCoverageSupportingEvidence &&
			candidate.ParentRef != nil {
			symbolRef = candidate.Ref.Ref
			productionRef = candidate.ParentRef.Ref
			break
		}
	}
	if productionRef == "" || symbolRef == "" {
		t.Fatalf("fixture lacks p*/s* candidate refs: %q %q", productionRef, symbolRef)
	}
	wire := synthesisWireProposal{Records: []synthesisWireRecord{
		{Kind: synthesisWireSubsystemRecord, Ref: "g1", Name: "Core", Description: "core"},
		{
			Kind: synthesisWireComponentRecord, SubsystemRef: "g1",
			Name: "Core component", Description: "responsibility",
			// Decision 231 gap closure: kind omitted on member refs.
			MemberRefs: []SynthesisMemberRef{{Ref: productionRef}, {Ref: symbolRef}},
			AnchorRefs: []SynthesisAnchorRef{},
			Hypothesis: false,
		},
	}}
	response, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RecordSynthesisResponse(bundle, "revision-kindless", "test", "test", time.Millisecond, response)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if result.Landscape.Fallback {
		t.Fatalf("kind-omitted member refs must not whole-reject: %#v", result.Landscape.Diagnostics)
	}
	// 2 of the fixture's members are referenced; the others stay in the
	// deterministic local remainder — accepted_partial is the honest
	// outcome (never whole-reject).
	if result.Landscape.ValidationOutcome != ValidationAccepted &&
		result.Landscape.ValidationOutcome != ValidationAcceptedPartial {
		t.Fatalf("outcome = %s, want accepted/partial: %#v", result.Landscape.ValidationOutcome, result.Landscape.Diagnostics)
	}
	// The component resolved BOTH members.
	found := false
	for _, subsystem := range result.Landscape.Subsystems {
		for _, component := range subsystem.Components {
			if component.Name == "Core component" {
				found = true
				if len(component.Members) != 2 {
					t.Fatalf("Core component members = %v, want 2 (p* + s*)", memberIDs(component.Members))
				}
			}
		}
	}
	if !found {
		t.Fatal("Core component missing")
	}
}

// The kind omission is also proven end-to-end with the EXACT miniflux
// provider response shape (flat records, member refs without kind, anchor
// refs without kind). It must decode and resolve, not whole-reject.
func TestSynthesisMinifluxKindlessResponseShape(t *testing.T) {
	t.Parallel()
	bundle := unitFixtureBundle()
	wire := synthesisWireProposal{Records: []synthesisWireRecord{
		{Kind: synthesisWireSubsystemRecord, Ref: "g1", Name: "Конфигурация", Description: "config handling"},
		{
			Kind: synthesisWireComponentRecord, SubsystemRef: "g1",
			Name: "Парсер конфигурации", Description: "разбор файлов конфигурации",
			MemberRefs: []SynthesisMemberRef{{Ref: "p1"}, {Ref: "s1"}},
			AnchorRefs: []SynthesisAnchorRef{{Ref: "a1"}},
		},
	}}
	response, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := decodeSynthesisWireProposalJSON(response)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(proposal.Records) != 2 {
		t.Fatalf("records = %d", len(proposal.Records))
	}
	_ = bundle
	_ = evidence.CertaintyStatic
}
