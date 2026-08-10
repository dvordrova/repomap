package componentmap

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestD241NestedExplicitEmptyMembershipIsItemLocal(t *testing.T) {
	t.Parallel()

	bundle := groundedTwoMemberSynthesisBundle()
	request, _, err := BuildSynthesisRequest(bundle)
	if err != nil {
		t.Fatalf("BuildSynthesisRequest: %v", err)
	}
	if len(request.Candidates) != 2 || len(request.BehaviorAnchors) != 1 {
		t.Fatalf("fixture request = %d candidates, %d anchors", len(request.Candidates), len(request.BehaviorAnchors))
	}
	validRefs := []string{request.Candidates[0].Ref.Ref, request.Candidates[1].Ref.Ref}
	anchorRef := request.BehaviorAnchors[0].Ref.Ref

	for _, test := range []struct {
		name           string
		emptyComponent map[string]any
	}{
		{
			name: "empty member refs",
			emptyComponent: map[string]any{
				"name": "Empty placeholder", "member_refs": []string{}, "anchor_refs": []string{},
			},
		},
		{
			name: "anchor-only empty component",
			emptyComponent: map[string]any{
				"name": "Anchor without membership", "member_refs": []string{}, "anchor_refs": []string{anchorRef},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := d241NestedResponse(t, []map[string]any{
				{
					"name": "Valid runtime", "member_refs": validRefs, "anchor_refs": []string{},
				},
				test.emptyComponent,
			})
			result, err := RecordSynthesisResponse(bundle, "d241-empty", "test", "test", 0, response)
			if err != nil {
				t.Fatalf("RecordSynthesisResponse: %v", err)
			}
			if result.Landscape.Fallback || result.Landscape.ValidationOutcome != ValidationAcceptedPartial {
				t.Fatalf("explicit empty item poisoned valid sibling: outcome=%q fallback=%v diagnostics=%#v",
					result.Landscape.ValidationOutcome, result.Landscape.Fallback, result.Landscape.Diagnostics)
			}
			if !d241HasDiagnostic(result.Landscape.Diagnostics, "proposal.empty_component") {
				t.Fatalf("missing item-local empty diagnostic: %#v", result.Landscape.Diagnostics)
			}
			if d241FindComponent(result.Landscape, "Valid runtime") == nil {
				t.Fatal("valid sibling did not publish")
			}
			if d241FindComponent(result.Landscape, test.emptyComponent["name"].(string)) != nil {
				t.Fatal("empty component published")
			}
			if len(result.Membership.UncoveredMemberIDs) != 0 {
				t.Fatalf("empty item removed sibling coverage: %#v", result.Membership.UncoveredMemberIDs)
			}
		})
	}
}

func TestD241NestedMalformedMembershipStillRejectsWholeResponse(t *testing.T) {
	t.Parallel()

	bundle := groundedTwoMemberSynthesisBundle()
	request, _, err := BuildSynthesisRequest(bundle)
	if err != nil {
		t.Fatalf("BuildSynthesisRequest: %v", err)
	}
	validRefs := []string{request.Candidates[0].Ref.Ref, request.Candidates[1].Ref.Ref}

	for _, test := range []struct {
		name      string
		malformed map[string]any
	}{
		{
			name: "missing",
			malformed: map[string]any{
				"name": "Malformed", "anchor_refs": []string{},
			},
		},
		{
			name: "null",
			malformed: map[string]any{
				"name": "Malformed", "member_refs": nil, "anchor_refs": []string{},
			},
		},
		{
			name: "wrong type",
			malformed: map[string]any{
				"name": "Malformed", "member_refs": "p1", "anchor_refs": []string{},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := d241NestedResponse(t, []map[string]any{
				{
					"name": "Otherwise valid sibling", "member_refs": validRefs, "anchor_refs": []string{},
				},
				test.malformed,
			})
			result, err := RecordSynthesisResponse(bundle, "d241-malformed", "test", "test", 0, response)
			if err != nil {
				t.Fatalf("RecordSynthesisResponse: %v", err)
			}
			if !result.Landscape.Fallback || result.Landscape.ValidationOutcome != ValidationRejected {
				t.Fatalf("malformed membership was not a whole-response rejection: %#v", result.Landscape)
			}
			if !d241HasDiagnostic(result.Landscape.Diagnostics, "response.invalid_proposal") ||
				d241HasDiagnostic(result.Landscape.Diagnostics, "proposal.empty_component") {
				t.Fatalf("malformed membership diagnostics = %#v", result.Landscape.Diagnostics)
			}
		})
	}
}

func TestNestedSupportingMemberDerivesScopeWithoutChangingProposalOrGrounding(t *testing.T) {
	t.Parallel()

	fixture := d241SupportingOnlyFixture(t)
	original := Proposal{
		Version: ProposalVersion,
		Subsystems: []ProposedSubsystem{{
			Name: "Application",
			Components: []ProposedComponent{
				{
					Name: "Mixed component", MemberIDs: []MemberID{fixture.SupportingChild, fixture.ValidPrimary},
					AnchorIDs: []string{fixture.ChildAnchorID},
				},
				{Name: "Valid sibling", MemberIDs: []MemberID{fixture.SiblingPrimary}},
			},
		}},
	}
	wantOriginal := cloneProposal(original)
	salvaged, membersDropped, componentsAffected, componentsDropped, anchorsDropped :=
		salvageSupportingOnlyProductionUnits(
			fixture.Bundle,
			fixture.Catalog,
			fixture.Units,
			fixture.Contexts,
			nil,
			original,
		)
	if !reflect.DeepEqual(original, wantOriginal) {
		t.Fatal("derived parent scope mutated the resolved provider proposal")
	}
	if membersDropped != 0 || componentsAffected != 0 || componentsDropped != 0 || anchorsDropped != 0 {
		t.Fatalf("derived parent scope triggered salvage = members %d affected %d components %d anchors %d",
			membersDropped, componentsAffected, componentsDropped, anchorsDropped)
	}
	if !reflect.DeepEqual(salvaged, original) {
		t.Fatalf("exact model membership changed: %#v", salvaged)
	}
	mixed := salvaged.Subsystems[0].Components[0]
	if !reflect.DeepEqual(mixed.MemberIDs, []MemberID{fixture.SupportingChild, fixture.ValidPrimary}) ||
		!reflect.DeepEqual(mixed.AnchorIDs, []string{fixture.ChildAnchorID}) {
		t.Fatalf("nested member or its exact anchor changed: %#v", mixed)
	}
	if !reflect.DeepEqual(salvaged.Subsystems[0].Components[1], original.Subsystems[0].Components[1]) {
		t.Fatalf("valid sibling changed: %#v", salvaged.Subsystems[0].Components[1])
	}
	counts := synthesisMembershipCounts(fixture.Bundle, fixture.Contexts, salvaged)
	if d241ContainsMember(counts.UncoveredMemberIDs, fixture.SupportingChild) ||
		!d241ContainsMember(counts.CoveredMemberIDs, fixture.SupportingChild) ||
		d241ContainsMember(counts.CoveredMemberIDs, fixture.SupportingPrimary) ||
		!d241ContainsMember(counts.UncoveredMemberIDs, fixture.SupportingPrimary) ||
		counts.RequestedPrimaryScope != 3 || counts.CoveredPrimaryScope != 3 ||
		counts.UncoveredPrimaryScope != 0 || counts.CoveredSupportingEvidence != 1 {
		t.Fatalf("derived parent scope changed semantic member partition: %#v", counts)
	}

	response := d241NestedResponse(t, []map[string]any{
		{
			"name": "Mixed component",
			"member_refs": []string{
				fixture.Catalog.membersByID[fixture.SupportingChild].Ref,
				fixture.Catalog.membersByID[fixture.ValidPrimary].Ref,
			},
			"anchor_refs": []string{fixture.Catalog.anchorsByID[fixture.ChildAnchorID].Ref},
		},
		{
			"name":        "Valid sibling",
			"member_refs": []string{fixture.Catalog.membersByID[fixture.SiblingPrimary].Ref},
			"anchor_refs": []string{},
		},
	})
	result, err := RecordSynthesisResponse(fixture.Bundle, "d241-supporting", "test", "test", 0, response)
	if err != nil {
		t.Fatalf("RecordSynthesisResponse: %v", err)
	}
	if result.Landscape.Fallback || result.Landscape.ValidationOutcome != ValidationAcceptedPartial ||
		d241HasDiagnostic(result.Landscape.Diagnostics, "proposal.supporting_only_unit_coverage_salvaged") {
		t.Fatalf("nested semantic choice was not published directly: %#v", result.Landscape)
	}
	if d241FindComponent(result.Landscape, "Valid sibling") == nil ||
		!d241ContainsMember(result.Membership.CoveredMemberIDs, fixture.SupportingChild) ||
		d241ContainsMember(result.Membership.CoveredMemberIDs, fixture.SupportingPrimary) {
		t.Fatalf("published grouping changed sibling/member accounting: landscape=%#v membership=%#v",
			result.Landscape, result.Membership)
	}
	publishedMixed := d241FindComponent(result.Landscape, "Mixed component")
	if publishedMixed == nil || !reflect.DeepEqual(publishedMixed.AnchorIDs, []string{fixture.ChildAnchorID}) ||
		!d241ContainsCandidate(publishedMixed.Members, fixture.SupportingChild) ||
		d241ContainsCandidate(publishedMixed.Members, fixture.SupportingPrimary) {
		t.Fatalf("published mixed component lost exact child/anchor or gained parent: %#v", publishedMixed)
	}
}

func TestD241SupportingOnlySalvageRespectsSharedAndCeilingPrimaryEvidence(t *testing.T) {
	t.Parallel()

	fixture := d241SupportingOnlyFixture(t)
	childOnly := ProposedComponent{Name: "Supporting child", MemberIDs: []MemberID{fixture.SupportingChild}}

	t.Run("shared unit scope", func(t *testing.T) {
		proposal := Proposal{
			Version: ProposalVersion,
			Subsystems: []ProposedSubsystem{{
				Name: "Application",
				Components: []ProposedComponent{
					childOnly,
					{Name: "Shared production scope", SharedUnitRefs: []string{string(fixture.SupportingUnit)}},
				},
			}},
		}
		got, membersDropped, componentsAffected, componentsDropped, anchorsDropped :=
			salvageSupportingOnlyProductionUnits(
				fixture.Bundle, fixture.Catalog, fixture.Units, fixture.Contexts, nil, proposal,
			)
		if membersDropped != 0 || componentsAffected != 0 || componentsDropped != 0 || anchorsDropped != 0 ||
			!reflect.DeepEqual(got, proposal) {
			t.Fatalf("shared primary scope was treated as supporting-only: %#v", got)
		}
	})

	t.Run("primary ref removed by backend ceiling", func(t *testing.T) {
		proposal := Proposal{
			Version: ProposalVersion,
			Subsystems: []ProposedSubsystem{{
				Name:       "Application",
				Components: []ProposedComponent{childOnly},
			}},
		}
		ceilingDropped := []SynthesisMemberRef{fixture.Catalog.membersByID[fixture.SupportingPrimary]}
		got, membersDropped, componentsAffected, componentsDropped, anchorsDropped :=
			salvageSupportingOnlyProductionUnits(
				fixture.Bundle, fixture.Catalog, fixture.Units, fixture.Contexts, ceilingDropped, proposal,
			)
		if membersDropped != 0 || componentsAffected != 0 || componentsDropped != 0 || anchorsDropped != 0 ||
			!reflect.DeepEqual(got, proposal) {
			t.Fatalf("ceiling-created supporting-only condition was salvaged: %#v", got)
		}
	})
}

type d241SupportingFixture struct {
	Bundle            CandidateBundle
	Catalog           synthesisPrivateCatalog
	Units             UnitCatalog
	Contexts          map[MemberID]synthesisCandidateContext
	SupportingPrimary MemberID
	SupportingChild   MemberID
	SupportingUnit    UnitWireRef
	ValidPrimary      MemberID
	SiblingPrimary    MemberID
	ChildAnchorID     string
}

func d241SupportingOnlyFixture(t *testing.T) d241SupportingFixture {
	t.Helper()

	supportingPrimary := unitTestCandidate(
		MemberPackage, "package-alpha-private", "example.invalid/project/alpha", CandidateRoleConceptualMember, nil,
	)
	supportingFile := unitTestCandidate(
		MemberFile, "file-alpha-private", "alpha/service.go", CandidateRoleStructuralLocator, &supportingPrimary.ID,
	)
	supportingChild := unitTestCandidate(
		MemberSymbol, "symbol-alpha-private", "example.invalid/project/alpha.Run", CandidateRoleConceptualMember, &supportingFile.ID,
	)
	validPrimary := unitTestCandidate(
		MemberPackage, "package-beta-private", "example.invalid/project/beta", CandidateRoleConceptualMember, nil,
	)
	siblingPrimary := unitTestCandidate(
		MemberPackage, "package-gamma-private", "example.invalid/project/gamma", CandidateRoleConceptualMember, nil,
	)
	const childAnchorID = "anchor-alpha-child-private"
	bundle := unitTestBundle(
		[]Candidate{supportingPrimary, supportingFile, supportingChild, validPrimary, siblingPrimary},
		[]BehaviorAnchor{unitTestAnchor(childAnchorID, AnchorExtensionFamily, supportingChild.ID)},
	)
	catalog, err := buildSynthesisPrivateCatalog(bundle)
	if err != nil {
		t.Fatalf("buildSynthesisPrivateCatalog: %v", err)
	}
	units, err := CompileUnitCatalog(bundle)
	if err != nil {
		t.Fatalf("CompileUnitCatalog: %v", err)
	}
	contexts, err := synthesisCandidateContexts(bundle, units)
	if err != nil {
		t.Fatalf("synthesisCandidateContexts: %v", err)
	}
	supportingUnit := units.MemberToWireUnit[supportingPrimary.ID]
	if supportingUnit == "" || supportingUnit == units.MemberToWireUnit[validPrimary.ID] {
		t.Fatalf("fixture did not create distinct production units: %#v", units.MemberToWireUnit)
	}
	return d241SupportingFixture{
		Bundle: bundle, Catalog: catalog, Units: units, Contexts: contexts,
		SupportingPrimary: supportingPrimary.ID, SupportingChild: supportingChild.ID,
		SupportingUnit: supportingUnit, ValidPrimary: validPrimary.ID, SiblingPrimary: siblingPrimary.ID,
		ChildAnchorID: childAnchorID,
	}
}

func d241NestedResponse(t *testing.T, components []map[string]any) []byte {
	t.Helper()
	response, err := json.Marshal(map[string]any{
		"subsystems": []map[string]any{{
			"name": "Application", "description": "", "components": components,
		}},
	})
	if err != nil {
		t.Fatalf("encode nested response: %v", err)
	}
	return response
}

func d241HasDiagnostic(diagnostics []Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func d241FindComponent(landscape Landscape, name string) *Component {
	for subsystemIndex := range landscape.Subsystems {
		for componentIndex := range landscape.Subsystems[subsystemIndex].Components {
			component := &landscape.Subsystems[subsystemIndex].Components[componentIndex]
			if component.Name == name {
				return component
			}
		}
	}
	return nil
}

func d241ContainsMember(memberIDs []MemberID, want MemberID) bool {
	for _, memberID := range memberIDs {
		if memberID == want {
			return true
		}
	}
	return false
}

func d241ContainsCandidate(candidates []Candidate, want MemberID) bool {
	for _, candidate := range candidates {
		if candidate.ID == want {
			return true
		}
	}
	return false
}
