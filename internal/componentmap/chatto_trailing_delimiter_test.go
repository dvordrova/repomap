package componentmap

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Archive 12 P0 (Chatto): a provider response that is one COMPLETE valid
// JSON proposal followed by exactly `]}` must normalize, run ORDINARY
// semantic validation (same member_refs contract), publish the exact
// normalization diagnostic, and continue — never a whole fallback and
// never a terminal rejection. This is the same bounded defect class as
// Gotify, now pinned at the RecordSynthesisResponse level.
func TestChattoTrailingClosingDelimiterRecordsAccepted(t *testing.T) {
	bundle := unitFixtureBundle()
	request, _, err := BuildSynthesisRequest(bundle)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if len(request.Candidates) == 0 {
		t.Fatal("fixture has no candidates")
	}
	// Pick 4 production refs and 1 symbol ref from the request catalog.
	var productionRefs []string
	var symbolRef string
	for _, candidate := range request.Candidates {
		if strings.HasPrefix(candidate.Ref.Ref, "p") && len(productionRefs) < 4 {
			productionRefs = append(productionRefs, candidate.Ref.Ref)
		}
		if strings.HasPrefix(candidate.Ref.Ref, "s") && symbolRef == "" {
			symbolRef = candidate.Ref.Ref
		}
	}
	if len(productionRefs) < 4 || symbolRef == "" {
		t.Fatalf("fixture lacks p*/s* refs: p=%v s=%q", productionRefs, symbolRef)
	}
	wire := synthesisWireProposal{Records: []synthesisWireRecord{
		{Kind: synthesisWireSubsystemRecord, Ref: "g1", Name: "Core", Description: "core"},
		{Kind: synthesisWireSubsystemRecord, Ref: "g2", Name: "Auth", Description: "auth"},
		{Kind: synthesisWireSubsystemRecord, Ref: "g3", Name: "API", Description: "api"},
		{Kind: synthesisWireSubsystemRecord, Ref: "g4", Name: "Storage", Description: "storage"},
		{
			Kind: synthesisWireComponentRecord, SubsystemRef: "g1",
			Name: "Core component", Description: "responsibility",
			MemberRefs: []SynthesisMemberRef{{Ref: productionRefs[0]}, {Ref: productionRefs[1]}},
			AnchorRefs: []SynthesisAnchorRef{{Ref: "a1"}},
			Hypothesis: false,
		},
		{
			Kind: synthesisWireComponentRecord, SubsystemRef: "g2",
			Name: "Auth component", Description: "responsibility",
			MemberRefs: []SynthesisMemberRef{{Ref: productionRefs[2]}},
			AnchorRefs: []SynthesisAnchorRef{}, // honest empty array
			Hypothesis: false,
		},
		{
			Kind: synthesisWireComponentRecord, SubsystemRef: "g3",
			Name: "API component", Description: "responsibility",
			MemberRefs: []SynthesisMemberRef{{Ref: productionRefs[3]}},
			AnchorRefs: []SynthesisAnchorRef{{Ref: "a2"}},
			Hypothesis: false,
		},
		{
			Kind: synthesisWireComponentRecord, SubsystemRef: "g4",
			Name: "Storage component", Description: "responsibility",
			MemberRefs: []SynthesisMemberRef{{Ref: symbolRef}},
			AnchorRefs: []SynthesisAnchorRef{},
			Hypothesis: false,
		},
	}}
	response, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	// Chatto corpus case: the complete valid object is followed by exactly
	// `]}` — nothing else.
	raw := append(append([]byte(nil), response...), []byte("]}")...)
	result, err := RecordSynthesisResponse(bundle, "v11-chatto-"+revisionSuffix(), "test", "test", time.Millisecond, raw)
	if err != nil {
		t.Fatalf("record (Chatto trailing ]}): %v", err)
	}
	if result.Landscape.Fallback {
		t.Fatalf("Chatto trailing ]} must not whole-fallback: %#v", result.Landscape.Diagnostics)
	}
	if result.Landscape.ValidationOutcome != ValidationAccepted &&
		result.Landscape.ValidationOutcome != ValidationAcceptedPartial &&
		result.Landscape.ValidationOutcome != ValidationAcceptedNormalized {
		t.Fatalf("outcome = %s, want accepted/partial/normalized", result.Landscape.ValidationOutcome)
	}
	found := false
	for _, d := range result.Landscape.Diagnostics {
		if d.Code == "response.trailing_closing_delimiters_normalized" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing exact normalization diagnostic, got: %#v", result.Landscape.Diagnostics)
	}
	if len(result.Landscape.Subsystems) != 4 {
		t.Fatalf("subsystems = %d, want 4 (all records accepted)", len(result.Landscape.Subsystems))
	}
}

func revisionSuffix() string {
	return "archive12"
}
