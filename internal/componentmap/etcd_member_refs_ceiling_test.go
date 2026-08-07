package componentmap

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Archive 12 P0 (etcd): without a membership ceiling, a large repository
// makes the provider serialize an unbounded member list, the output
// degenerates into repeated p* refs and dies at the provider output limit
// (finish_reason length → provider_output_limit). The single member_refs
// grammar stays, but response membership is BOUNDED deterministically in
// the backend: per component and total. The report continues with the
// bounded membership and the exact ceiling diagnostics; unreturned members
// stay in the local remainder.
func TestEtcdStyleMemberRefsCeilingKeepsReportAlive(t *testing.T) {
	bundle := unitFixtureBundle()
	request, _, err := BuildSynthesisRequest(bundle)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if len(request.Candidates) == 0 {
		t.Fatal("fixture has no candidates")
	}
	// Collect every p* ref from the catalog to simulate a large repository.
	var allRefs []string
	for _, candidate := range request.Candidates {
		if strings.HasPrefix(candidate.Ref.Ref, "p") {
			allRefs = append(allRefs, candidate.Ref.Ref)
		}
	}
	if len(allRefs) < 3 {
		t.Fatalf("fixture has too few p* refs: %d", len(allRefs))
	}
	// Simulate the etcd degeneration: 10 components, each repeating its own
	// refs ~64 times (like the 10,448 p* occurrences on 290 unique refs).
	// A ref repeated within one component must not double-count, but the
	// raw wire volume is what blows the provider budget.
	records := []synthesisWireRecord{
		{Kind: synthesisWireSubsystemRecord, Ref: "g1", Name: "Core", Description: "core"},
	}
	const componentCount = 10
	const repeatsPerComponent = 64
	refsPerComponent := (len(allRefs) + componentCount - 1) / componentCount
	for index := 0; index < componentCount; index++ {
		start := index * refsPerComponent
		if start >= len(allRefs) {
			break
		}
		end := start + refsPerComponent
		if end > len(allRefs) {
			end = len(allRefs)
		}
		componentRefs := allRefs[start:end]
		if len(componentRefs) == 0 {
			componentRefs = allRefs[:1]
		}
		members := make([]SynthesisMemberRef, 0, len(componentRefs)*repeatsPerComponent)
		for repeat := 0; repeat < repeatsPerComponent; repeat++ {
			for _, ref := range componentRefs {
				members = append(members, SynthesisMemberRef{Ref: ref})
			}
		}
		records = append(records, synthesisWireRecord{
			Kind: synthesisWireComponentRecord, SubsystemRef: "g1",
			Name:        "component-" + string(rune('a'+index)),
			Description: "responsibility",
			MemberRefs:  members,
			AnchorRefs:  []SynthesisAnchorRef{},
			Hypothesis:  false,
		})
	}
	wire := synthesisWireProposal{Records: records}
	response, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RecordSynthesisResponse(bundle, "v11-etcd-ceiling", "test", "test", time.Millisecond, response)
	if err != nil {
		t.Fatalf("record (etcd-style oversized membership): %v", err)
	}
	if result.Landscape.Fallback {
		t.Fatalf("etcd-style membership must not whole-fallback: %#v", result.Landscape.Diagnostics)
	}
	if result.Landscape.ValidationOutcome != ValidationAccepted &&
		result.Landscape.ValidationOutcome != ValidationAcceptedPartial &&
		result.Landscape.ValidationOutcome != ValidationAcceptedNormalized {
		t.Fatalf("outcome = %s, want accepted/partial/normalized", result.Landscape.ValidationOutcome)
	}
	// The exact per-component ceiling diagnostic must be present.
	perComponent := false
	for _, d := range result.Landscape.Diagnostics {
		if d.Code == "response.member_refs_per_component_ceiling" {
			perComponent = true
		}
	}
	if !perComponent {
		t.Fatalf("missing exact per-component ceiling diagnostic: %#v", result.Landscape.Diagnostics)
	}
	// Every accepted component must be within the per-component bound.
	for _, subsystem := range result.Landscape.Subsystems {
		for _, component := range subsystem.Components {
			if len(component.Members) > maxMemberRefsPerComponent {
				t.Fatalf("component %s has %d members, ceiling is %d", component.Name, len(component.Members), maxMemberRefsPerComponent)
			}
		}
	}
	// The whole accepted landscape membership stays within the total bound.
	totalMembers := 0
	for _, subsystem := range result.Landscape.Subsystems {
		for _, component := range subsystem.Components {
			totalMembers += len(component.Members)
		}
	}
	if totalMembers > maxTotalMemberRefs {
		t.Fatalf("total accepted membership = %d, ceiling is %d", totalMembers, maxTotalMemberRefs)
	}
}

// The TOTAL ceiling is a second deterministic bound: with many components
// each under the per-component maximum, the whole response must still stay
// within maxTotalMemberRefs (drop from the end, earliest components keep
// their membership).
func TestMemberRefsTotalCeilingDropsFromEnd(t *testing.T) {
	proposal := &synthesisWireProposal{}
	for index := 0; index < 30; index++ {
		members := make([]SynthesisMemberRef, 0, maxMemberRefsPerComponent)
		for member := 0; member < maxMemberRefsPerComponent; member++ {
			members = append(members, SynthesisMemberRef{Ref: string(rune('a'+index)) + string(rune('a'+(member%26))) + "p"})
		}
		proposal.Records = append(proposal.Records, synthesisWireRecord{
			Kind: synthesisWireComponentRecord, SubsystemRef: "g1",
			Name:        "component-" + string(rune('a'+index)),
			Description: "responsibility",
			MemberRefs:  members,
			AnchorRefs:  []SynthesisAnchorRef{},
			Hypothesis:  false,
		})
	}
	applied, diagnostics, err := applyMemberRefsCeiling(proposal)
	if err != nil {
		t.Fatalf("ceiling: %v", err)
	}
	if !applied {
		t.Fatalf("total ceiling must apply for 30 components x %d refs", maxMemberRefsPerComponent)
	}
	totalFound := false
	for _, d := range diagnostics {
		if d.Code == "response.member_refs_total_ceiling" {
			totalFound = true
		}
	}
	if !totalFound {
		t.Fatalf("missing total ceiling diagnostic: %#v", diagnostics)
	}
	total := 0
	for _, record := range proposal.Records {
		total += len(record.MemberRefs)
	}
	if total != maxTotalMemberRefs {
		t.Fatalf("total after ceiling = %d, want %d", total, maxTotalMemberRefs)
	}
	// Earliest components keep their full membership; the drop came from
	// the end (last component is empty after dropping all its refs).
	if len(proposal.Records[0].MemberRefs) != maxMemberRefsPerComponent {
		t.Fatalf("first component lost refs (%d), earliest must keep membership", len(proposal.Records[0].MemberRefs))
	}
	if len(proposal.Records[len(proposal.Records)-1].MemberRefs) != 0 {
		t.Fatalf("last component still has %d refs, drop must come from the end", len(proposal.Records[len(proposal.Records)-1].MemberRefs))
	}
}
