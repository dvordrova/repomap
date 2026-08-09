package componentmap

import (
	"encoding/json"
	"strings"
	"testing"
)

// Phase 1 prompt-contract cleanup: the active Architecture response is the
// nested subsystems grammar. The model chooses meaning only — names,
// descriptions, and representative member/anchor refs. It never invents g*
// IDs, never emits kind/subsystem_ref, never maintains parent/child foreign
// keys, and never counts refs/components. The backend assigns subsystem refs
// in wire order and derives adjacency from nesting.
func TestSynthesisNestedWireDecodesBackendOwnedIdentity(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"subsystems":[` +
		`{"name":"Application","description":"","components":[` +
		`{"name":"Runtime","description":"","member_refs":["p1","s2"],"anchor_refs":["a1"]},` +
		`{"name":"Storage","description":"","member_refs":["p3"]}` +
		`]},` +
		`{"name":"Tooling","description":"","components":[` +
		`{"name":"Builder","description":"","member_refs":["p4"],"anchor_refs":[]}` +
		`]}]}`)
	proposal, err := decodeSynthesisWireProposalJSON(raw)
	if err != nil {
		t.Fatalf("nested wire rejected: %v", err)
	}
	if len(proposal.Records) != 5 {
		t.Fatalf("records = %d, want 5 (2 subsystems + 3 components)", len(proposal.Records))
	}
	// Backend assigns g1/g2 in wire order; components derive subsystem_ref
	// from nesting — the model never carries them.
	if proposal.Records[0].Kind != synthesisWireSubsystemRecord || proposal.Records[0].Ref != "g1" {
		t.Fatalf("first record = %#v, want subsystem g1", proposal.Records[0])
	}
	if proposal.Records[1].Kind != synthesisWireComponentRecord || proposal.Records[1].SubsystemRef != "g1" {
		t.Fatalf("second record = %#v, want component of g1", proposal.Records[1])
	}
	if proposal.Records[3].Kind != synthesisWireSubsystemRecord || proposal.Records[3].Ref != "g2" {
		t.Fatalf("third record = %#v, want subsystem g2", proposal.Records[3])
	}
	if proposal.Records[4].SubsystemRef != "g2" {
		t.Fatalf("fifth record subsystem_ref = %q, want g2", proposal.Records[4].SubsystemRef)
	}
	// Plain-string refs carry no kind; the backend restores kinds locally.
	if len(proposal.Records[1].MemberRefs) != 2 || proposal.Records[1].MemberRefs[0].Ref != "p1" ||
		proposal.Records[1].MemberRefs[0].Kind != "" {
		t.Fatalf("member refs not plain strings: %#v", proposal.Records[1].MemberRefs)
	}
	if len(proposal.Records[1].AnchorRefs) != 1 || proposal.Records[1].AnchorRefs[0].Ref != "a1" {
		t.Fatalf("anchor refs = %#v", proposal.Records[1].AnchorRefs)
	}
	// The component without an anchor_refs field is normalized, never rejected.
	if !proposal.Records[2].NormalizedMissingAnchorRefs {
		t.Fatalf("missing anchor_refs must be a counted normalization")
	}
	if proposal.Records[4].NormalizedMissingAnchorRefs {
		t.Fatalf("explicit empty anchor_refs must not be treated as missing")
	}
}

func TestSynthesisNestedWireRejectsNullAnchorRefs(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"subsystems":[{"name":"Application","components":[` +
		`{"name":"Runtime","member_refs":["p1"],"anchor_refs":null}` +
		`]}]}`)
	if _, err := decodeSynthesisWireProposalJSON(raw); err == nil ||
		!strings.Contains(err.Error(), "must not be null") {
		t.Fatalf("null anchor_refs error = %v, want bounded null rejection", err)
	}
}

func TestSynthesisNestedWireUsesDescriptionBoundNotNameBound(t *testing.T) {
	t.Parallel()
	description := strings.Repeat("я", maxNameBytes/2+1)
	raw, err := json.Marshal(synthesisWireNested{Subsystems: []synthesisWireNestedSubsystem{{
		Name:        "Application",
		Description: description,
		Components: []synthesisWireNestedComponent{{
			Name:        "Runtime",
			Description: description,
			MemberRefs:  json.RawMessage(`["p1"]`),
		}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeSynthesisWireProposalJSON(raw); err != nil {
		t.Fatalf("nested descriptions within %d bytes rejected by the %d-byte name limit: %v", maxDescriptionBytes, maxNameBytes, err)
	}
}

func TestSynthesisNestedExplicitEmptyAnchorRefsPreservesFullCoverage(t *testing.T) {
	t.Parallel()

	const candidateCount = 28
	bundle := candidateBundleWithPackages(candidateCount)
	request, _, err := BuildSynthesisRequest(bundle)
	if err != nil {
		t.Fatalf("BuildSynthesisRequest: %v", err)
	}
	if len(request.Candidates) != candidateCount {
		t.Fatalf("request candidates = %d, want %d", len(request.Candidates), candidateCount)
	}

	type nestedComponent struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		MemberRefs  []string `json:"member_refs"`
		AnchorRefs  []string `json:"anchor_refs"`
	}
	type nestedSubsystem struct {
		Name        string            `json:"name"`
		Description string            `json:"description"`
		Components  []nestedComponent `json:"components"`
	}

	componentNames := []string{
		"Runtime", "Storage", "Commands", "Transport",
		"Configuration", "Clients", "Workers", "Utilities",
	}
	components := make([]nestedComponent, 0, len(componentNames))
	for index, name := range componentNames {
		start := index * len(request.Candidates) / len(componentNames)
		end := (index + 1) * len(request.Candidates) / len(componentNames)
		memberRefs := make([]string, 0, end-start)
		for _, candidate := range request.Candidates[start:end] {
			memberRefs = append(memberRefs, candidate.Ref.Ref)
		}
		components = append(components, nestedComponent{
			Name:        name,
			Description: "Distinct responsibility over exact request-local members.",
			MemberRefs:  memberRefs,
			AnchorRefs:  []string{},
		})
	}
	response, err := json.Marshal(struct {
		Subsystems []nestedSubsystem `json:"subsystems"`
	}{Subsystems: []nestedSubsystem{{
		Name:        "Repository",
		Description: "Complete conceptual grouping.",
		Components:  components,
	}}})
	if err != nil {
		t.Fatalf("marshal nested response: %v", err)
	}
	if got := strings.Count(string(response), `"anchor_refs":[]`); got != len(componentNames) {
		t.Fatalf("explicit empty anchor arrays = %d, want %d: %s", got, len(componentNames), response)
	}

	landscape, membership, err := evaluateSynthesisResponse(bundle, ResponseCaptured, response)
	if err != nil {
		t.Fatalf("full-coverage nested response with explicit empty anchors: %v", err)
	}
	if landscape.Fallback || landscape.ValidationOutcome != ValidationAccepted ||
		landscape.Source != SourceValidatedModel {
		t.Fatalf("full-coverage response = fallback:%v outcome:%s source:%s", landscape.Fallback, landscape.ValidationOutcome, landscape.Source)
	}
	if !membership.Counted || membership.DistinctMembers != candidateCount ||
		membership.MemberOccurrences != candidateCount || !membership.CoverageComplete() {
		t.Fatalf("full-coverage membership = %#v, want %d/%d", membership, candidateCount, candidateCount)
	}
	if hasLandscapeDiagnostic(landscape.Diagnostics, "proposal.normalized_missing_anchor_refs") {
		t.Fatalf("explicit empty anchor arrays produced a missing-field normalization: %#v", landscape.Diagnostics)
	}
}

// Phase 1: the model must not be asked to return response-local IDs, kind
// tags, or adjacency foreign keys — the prompt grammar is nested and the
// counts/cardinality instructions are gone from the provider contract.
func TestSynthesisPromptNestedGrammarRemovesForeignKeysAndCounting(t *testing.T) {
	t.Parallel()
	bundle := adversarialPrivateIdentitySynthesisBundle()
	prompt, err := BuildSynthesisPrompt(bundle)
	if err != nil {
		t.Fatalf("BuildSynthesisPrompt: %v", err)
	}
	for _, forbidden := range []string{
		`"records"`, `"kind":"subsystem"`, `"ref":"g1"`, `"subsystem_ref"`,
		"twenty-four", "three hundred twenty", "forty-eight",
		"cap each component", "cap the entire response",
	} {
		if strings.Contains(prompt.System, forbidden) {
			t.Errorf("prompt still exposes backend-owned foreign key/counting %q", forbidden)
		}
	}
	for _, required := range []string{
		`{"subsystems":[`, `"components":[`, `"member_refs":["p1","s2"]`,
		"Choose representative supplied members needed to distinguish each component",
		"do not wrap refs in objects and do not add kind fields",
		"Do not emit response-local IDs, kind tags, parent references",
	} {
		if !strings.Contains(prompt.System, required) {
			t.Errorf("prompt misses nested grammar %q", required)
		}
	}
}

// Phase 1: required_member_refs is absent from the provider-visible request
// wire; it was exactly the candidates checklist (duplicate bytes).
func TestSynthesisRequestWireOmitsRequiredMemberRefs(t *testing.T) {
	t.Parallel()
	bundle := adversarialPrivateIdentitySynthesisBundle()
	request, encoded, err := BuildSynthesisRequest(bundle)
	if err != nil {
		t.Fatalf("BuildSynthesisRequest: %v", err)
	}
	if len(request.RequiredMemberRefs) != len(request.Candidates) {
		t.Fatalf("local checklist = %d, candidates = %d", len(request.RequiredMemberRefs), len(request.Candidates))
	}
	if strings.Contains(string(encoded), `"required_member_refs"`) {
		t.Fatalf("provider wire leaks required_member_refs: %s", encoded)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode wire: %v", err)
	}
	if _, exists := decoded["required_member_refs"]; exists {
		t.Fatalf("provider wire carries the duplicated checklist")
	}
}
