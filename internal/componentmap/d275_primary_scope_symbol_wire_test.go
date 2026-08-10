package componentmap

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestNestedSymbolsRetainSupportingRoleAndDeriveExactParentScope(t *testing.T) {
	t.Parallel()

	bundle, packageIDs, symbolIDs := d275FreshRepomapShapeBundle()
	request, requestJSON, err := BuildSynthesisRequest(bundle)
	if err != nil {
		t.Fatalf("BuildSynthesisRequest: %v", err)
	}
	if len(packageIDs) != 64 || len(symbolIDs) != 99 || len(request.RequiredMemberRefs) != 163 {
		t.Fatalf("fixture shape packages/symbols/required = %d/%d/%d", len(packageIDs), len(symbolIDs), len(request.RequiredMemberRefs))
	}

	var providerWire struct {
		Candidates []struct {
			Symbols []struct {
				Ref          SynthesisMemberRef    `json:"ref"`
				CoverageRole SynthesisCoverageRole `json:"coverage_role"`
			} `json:"symbols"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(requestJSON, &providerWire); err != nil {
		t.Fatalf("decode provider request: %v", err)
	}
	nestedSymbols := 0
	for _, candidate := range providerWire.Candidates {
		for _, symbol := range candidate.Symbols {
			nestedSymbols++
			if symbol.Ref.Kind != MemberSymbol || symbol.CoverageRole != SynthesisCoverageSupportingEvidence {
				t.Fatalf("nested symbol lost exact supporting role: %#v", symbol)
			}
		}
	}
	if nestedSymbols != len(symbolIDs) {
		t.Fatalf("nested provider symbols = %d, want %d", nestedSymbols, len(symbolIDs))
	}

	catalog, err := buildSynthesisPrivateCatalog(bundle)
	if err != nil {
		t.Fatalf("buildSynthesisPrivateCatalog: %v", err)
	}
	symbolRefs := make([]string, 0, len(symbolIDs))
	for _, symbolID := range symbolIDs {
		symbolRefs = append(symbolRefs, catalog.membersByID[symbolID].Ref)
	}
	response := d275FreshRepomapShapeResponse(t, symbolRefs)
	shape := InspectSynthesisResponseShape(response)
	if !shape.JSONValid || shape.Grammar != "nested" || shape.SubsystemCount != 4 ||
		shape.ComponentCount != 18 || shape.MemberRefCount != 102 {
		t.Fatalf("fresh response shape = %#v", shape)
	}

	parsed, err := decodeSynthesisWireProposalJSON(response)
	if err != nil {
		t.Fatalf("parse fresh response: %v", err)
	}
	ceilingApplied, ceilingDiagnostics, droppedRefs, err := applyMemberRefsCeiling(&parsed)
	if err != nil {
		t.Fatalf("apply member ceiling: %v", err)
	}
	if !ceilingApplied || len(droppedRefs) != 11 ||
		!d275HasDiagnostic(ceilingDiagnostics, "response.member_refs_per_component_ceiling") ||
		d275WireMemberRefCount(parsed) != 91 {
		t.Fatalf("ceiling applied=%v dropped=%d retained=%d diagnostics=%#v",
			ceilingApplied, len(droppedRefs), d275WireMemberRefCount(parsed), ceilingDiagnostics)
	}

	result, err := RecordSynthesisResponse(bundle, "d275-fresh-repomap-shape", "test", "test", 0, response)
	if err != nil {
		t.Fatalf("RecordSynthesisResponse: %v", err)
	}
	if result.Landscape.Fallback || result.Landscape.ValidationOutcome != ValidationAcceptedPartial {
		t.Fatalf("nested-symbol response did not publish model grouping: outcome=%q fallback=%v",
			result.Landscape.ValidationOutcome, result.Landscape.Fallback)
	}
	for _, code := range []string{
		"response.member_refs_per_component_ceiling",
		"proposal.partial_member_coverage",
	} {
		if !d275HasDiagnostic(result.Landscape.Diagnostics, code) {
			t.Fatalf("missing diagnostic %q: %#v", code, result.Landscape.Diagnostics)
		}
	}
	for _, stale := range []string{
		"proposal.supporting_only_unit_coverage_salvaged",
		"proposal.supporting_only_unit_coverage",
		"proposal.empty_primary_scope_coverage",
		"proposal.zero_useful_semantic_components",
		"proposal.invalid_subsystem_count",
	} {
		if d275HasDiagnostic(result.Landscape.Diagnostics, stale) {
			t.Fatalf("nested-symbol response retained stale diagnostic %q: %#v", stale, result.Landscape.Diagnostics)
		}
	}

	counts := result.Membership
	if !counts.Counted || counts.MemberOccurrences != 91 || counts.DistinctMembers != 91 ||
		counts.RequestedPrimaryScope != 64 || counts.CoveredPrimaryScope != 64 ||
		counts.UncoveredPrimaryScope != 0 || counts.CoveredSupportingEvidence != 91 ||
		len(counts.RequestedMemberIDs) != 163 || len(counts.CoveredMemberIDs) != 91 ||
		len(counts.UncoveredMemberIDs) != 72 {
		t.Fatalf("derived primary-scope coverage = %#v", counts)
	}
	for _, packageID := range packageIDs {
		if d241ContainsMember(counts.CoveredMemberIDs, packageID) ||
			!d241ContainsMember(counts.UncoveredMemberIDs, packageID) {
			t.Fatalf("derived parent package became semantic membership: %s", packageID.key())
		}
	}
	for _, symbolID := range symbolIDs[:91] {
		if !d241ContainsMember(counts.CoveredMemberIDs, symbolID) {
			t.Fatalf("selected symbol missing from exact semantic membership: %s", symbolID.key())
		}
	}
	for _, symbolID := range symbolIDs[91:] {
		if !d241ContainsMember(counts.UncoveredMemberIDs, symbolID) {
			t.Fatalf("ceiling-dropped symbol missing from exact remainder: %s", symbolID.key())
		}
	}
	if result.Record.Call == nil ||
		!reflect.DeepEqual(result.Record.Call.Metadata.CoveredMemberIDs, counts.CoveredMemberIDs) ||
		!reflect.DeepEqual(result.Record.Call.Metadata.UncoveredMemberIDs, counts.UncoveredMemberIDs) ||
		result.Record.Call.Metadata.CoveredPrimaryScope != counts.CoveredPrimaryScope ||
		result.Record.Call.Metadata.CoveredSupportingEvidence != counts.CoveredSupportingEvidence {
		t.Fatalf("saved membership/derived scope differs from producer result: %#v", result.Record.Call)
	}
}

func d275FreshRepomapShapeBundle() (CandidateBundle, []MemberID, []MemberID) {
	bundle := CandidateBundle{
		Version: ContractVersion, RepositoryArchetype: ArchetypeCLITool, GroundingMode: GroundingPackages,
	}
	packages := make([]MemberID, 0, 64)
	for index := 0; index < 64; index++ {
		packageID := MemberID{Kind: MemberPackage, Value: fmt.Sprintf("member-package-d275-%03d-private", index)}
		packages = append(packages, packageID)
		packageName := fmt.Sprintf("module/pkg%03d", index)
		bundle.Candidates = append(bundle.Candidates, unitTestCandidate(
			MemberPackage, packageID.Value, packageName, CandidateRoleConceptualMember, nil,
		))
	}
	symbols := make([]MemberID, 0, 99)
	for index := 0; index < 99; index++ {
		parent := packages[index%len(packages)]
		symbolID := MemberID{Kind: MemberSymbol, Value: fmt.Sprintf("member-symbol-d275-%03d-private", index)}
		symbols = append(symbols, symbolID)
		bundle.Candidates = append(bundle.Candidates, unitTestCandidate(
			MemberSymbol, symbolID.Value, fmt.Sprintf("Func%03d", index), CandidateRoleConceptualMember, &parent,
		))
	}
	return bundle, packages, symbols
}

func d275FreshRepomapShapeResponse(t *testing.T, symbolRefs []string) []byte {
	t.Helper()
	if len(symbolRefs) != 99 {
		t.Fatalf("symbol refs = %d, want 99", len(symbolRefs))
	}
	componentRefCounts := []int{2, 4, 4, 7, 3, 3, 2, 4, 5, 3, 12, 2, 7, 4, 2, 1, 2, 35}
	subsystemComponentCounts := []int{4, 6, 5, 3}
	type wireComponent struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		MemberRefs  []string `json:"member_refs"`
		AnchorRefs  []string `json:"anchor_refs"`
	}
	type wireSubsystem struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Components  []wireComponent `json:"components"`
	}
	wire := struct {
		Subsystems []wireSubsystem `json:"subsystems"`
	}{}
	componentIndex := 0
	refCursor := 0
	for subsystemIndex, componentCount := range subsystemComponentCounts {
		subsystem := wireSubsystem{
			Name: fmt.Sprintf("Subsystem %d", subsystemIndex+1), Description: "Bounded production responsibility",
		}
		for localIndex := 0; localIndex < componentCount; localIndex++ {
			refCount := componentRefCounts[componentIndex]
			refs := make([]string, 0, refCount)
			if componentIndex == len(componentRefCounts)-1 {
				refs = append(refs, symbolRefs[refCursor:]...)
				refs = append(refs, symbolRefs[:3]...)
			} else {
				refs = append(refs, symbolRefs[refCursor:refCursor+refCount]...)
				refCursor += refCount
			}
			subsystem.Components = append(subsystem.Components, wireComponent{
				Name:        fmt.Sprintf("Component %d", componentIndex+1),
				Description: "Exact implementation evidence", MemberRefs: refs, AnchorRefs: []string{},
			})
			componentIndex++
		}
		wire.Subsystems = append(wire.Subsystems, subsystem)
	}
	if componentIndex != 18 || refCursor != 67 || len(wire.Subsystems[3].Components[2].MemberRefs) != 35 {
		t.Fatalf("constructed response shape components/cursor/catchall = %d/%d/%d",
			componentIndex, refCursor, len(wire.Subsystems[3].Components[2].MemberRefs))
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("encode response: %v", err)
	}
	return encoded
}

func d275WireMemberRefCount(proposal synthesisWireProposal) int {
	total := 0
	for _, record := range proposal.Records {
		if record.Kind == synthesisWireComponentRecord {
			total += len(record.MemberRefs)
		}
	}
	return total
}

func d275HasDiagnostic(diagnostics []Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
