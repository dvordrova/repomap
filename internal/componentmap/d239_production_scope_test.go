package componentmap

import (
	"strings"
	"testing"
)

type d239ProductionScopeFixture struct {
	Bundle            CandidateBundle
	ProductionPackage MemberID
	ProductionSymbol  MemberID
	TestPackage       MemberID
	TestSymbol        MemberID
	ToolingPackage    MemberID
	DocsPackage       MemberID
}

func TestD239SynthesisRequestUsesProductionAwareCoverageRoles(t *testing.T) {
	t.Parallel()
	if SynthesisRequestVersion != 21 || SynthesisRecordVersion != 17 || ContractVersion != 15 || ProposalVersion != 15 {
		t.Fatalf(
			"production-aware identities request/record/contract/proposal = %d/%d/%d/%d",
			SynthesisRequestVersion,
			SynthesisRecordVersion,
			ContractVersion,
			ProposalVersion,
		)
	}

	fixture := d239ProductionScopeBundle()
	request, _, err := BuildSynthesisRequest(fixture.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := buildSynthesisPrivateCatalog(fixture.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[MemberID]SynthesisCandidate, len(request.Candidates))
	for _, candidate := range request.Candidates {
		byID[catalog.membersByRef[candidate.Ref.key()]] = candidate
	}
	unitRoles := make(map[UnitWireRef]UnitRole, len(request.Units))
	for _, unit := range request.Units {
		unitRoles[unit.Ref] = unit.Role
	}

	production := byID[fixture.ProductionPackage]
	if production.CoverageRole != SynthesisCoveragePrimaryScope || production.ParentRef != nil ||
		unitRoles[production.UnitRef] != UnitRoleProduction {
		t.Fatalf("production package context = %#v, unit role = %q", production, unitRoles[production.UnitRef])
	}
	for _, item := range []struct {
		id       MemberID
		unitRole UnitRole
	}{
		{id: fixture.TestPackage, unitRole: UnitRoleTest},
		{id: fixture.ToolingPackage, unitRole: UnitRoleTooling},
		{id: fixture.DocsPackage, unitRole: UnitRoleDocumentation},
	} {
		candidate := byID[item.id]
		if candidate.CoverageRole != SynthesisCoverageSupportingEvidence || candidate.ParentRef != nil ||
			unitRoles[candidate.UnitRef] != item.unitRole {
			t.Fatalf("non-production package context = %#v, unit role = %q", candidate, unitRoles[candidate.UnitRef])
		}
	}
	for _, item := range []struct {
		child MemberID
		owner MemberID
	}{
		{child: fixture.ProductionSymbol, owner: fixture.ProductionPackage},
		{child: fixture.TestSymbol, owner: fixture.TestPackage},
	} {
		child := byID[item.child]
		owner := byID[item.owner]
		if child.CoverageRole != SynthesisCoverageSupportingEvidence || child.ParentRef == nil ||
			*child.ParentRef != owner.Ref || child.UnitRef != owner.UnitRef {
			t.Fatalf("package-owned context = %#v, owner = %#v", child, owner)
		}
	}

	tests := []struct {
		name   string
		mutate func(*SynthesisRequest)
	}{
		{
			name: "production package demoted",
			mutate: func(value *SynthesisRequest) {
				value.Candidates[d239CandidateIndex(value.Candidates, production.Ref)].CoverageRole = SynthesisCoverageSupportingEvidence
			},
		},
		{
			name: "test package promoted",
			mutate: func(value *SynthesisRequest) {
				testPackage := byID[fixture.TestPackage]
				value.Candidates[d239CandidateIndex(value.Candidates, testPackage.Ref)].CoverageRole = SynthesisCoveragePrimaryScope
			},
		},
		{
			name: "test child loses package parent",
			mutate: func(value *SynthesisRequest) {
				testSymbol := byID[fixture.TestSymbol]
				value.Candidates[d239CandidateIndex(value.Candidates, testSymbol.Ref)].ParentRef = nil
			},
		},
		{
			name: "unit role is open",
			mutate: func(value *SynthesisRequest) {
				value.Units = append([]SynthesisUnit(nil), value.Units...)
				value.Units[0].Role = "other"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutated := request
			mutated.Candidates = append([]SynthesisCandidate(nil), request.Candidates...)
			test.mutate(&mutated)
			if err := validateSynthesisRequestCoverage(mutated); err == nil {
				t.Fatal("invalid production-aware request context was accepted")
			}
		})
	}

	prompt, err := BuildSynthesisPrompt(fixture.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"top-level production conceptual repository surface",
		"Top-level package candidates in test, tooling, or documentation units",
		"automatically represents its exact enclosing package and production unit",
	} {
		if !strings.Contains(prompt.System, required) {
			t.Fatalf("production-aware prompt omitted %q:\n%s", required, prompt.System)
		}
	}
}

func TestD239NonProductionCoverageCannotReplaceProductionScope(t *testing.T) {
	t.Parallel()

	fixture := d239ProductionScopeBundle()
	catalog, err := buildSynthesisPrivateCatalog(fixture.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	nonProductionRefs := []SynthesisMemberRef{
		catalog.membersByID[fixture.TestPackage],
		catalog.membersByID[fixture.TestSymbol],
		catalog.membersByID[fixture.ToolingPackage],
		catalog.membersByID[fixture.DocsPackage],
	}
	result, err := RecordSynthesisResponse(
		fixture.Bundle,
		"d239-non-production-only",
		"test",
		"test",
		0,
		d238NestedResponse(t, nonProductionRefs),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Landscape.Fallback || result.Landscape.ValidationOutcome != ValidationRejected ||
		!d238HasDiagnostic(result.Landscape.Diagnostics, "proposal.empty_primary_scope_coverage") {
		t.Fatalf("non-production-only proposal was not rejected: %#v", result.Landscape)
	}
	if d238HasDiagnostic(result.Landscape.Diagnostics, "proposal.supporting_only_unit_coverage") {
		t.Fatalf("all-supporting non-production units triggered the production per-unit gate: %#v", result.Landscape.Diagnostics)
	}
	d239AssertCoverageCounts(t, result, 1, 0, 1, 4)

	withProduction := append([]SynthesisMemberRef{catalog.membersByID[fixture.ProductionPackage]}, nonProductionRefs...)
	accepted, err := RecordSynthesisResponse(
		fixture.Bundle,
		"d239-production-and-supporting",
		"test",
		"test",
		0,
		d238NestedResponse(t, withProduction),
	)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Landscape.Fallback || accepted.Landscape.ValidationOutcome != ValidationAcceptedPartial {
		t.Fatalf("production-backed partial proposal was rejected: %#v", accepted.Landscape)
	}
	if d238HasDiagnostic(accepted.Landscape.Diagnostics, "proposal.empty_primary_scope_coverage") ||
		d238HasDiagnostic(accepted.Landscape.Diagnostics, "proposal.supporting_only_unit_coverage") {
		t.Fatalf("production-backed proposal received a quality rejection: %#v", accepted.Landscape.Diagnostics)
	}
	d239AssertCoverageCounts(t, accepted, 1, 1, 0, 4)
}

func d239AssertCoverageCounts(
	t *testing.T,
	result SynthesisResult,
	requestedPrimary int,
	coveredPrimary int,
	uncoveredPrimary int,
	coveredSupporting int,
) {
	t.Helper()
	counts := result.Membership
	if !counts.Counted || counts.RequestedPrimaryScope != requestedPrimary ||
		counts.CoveredPrimaryScope != coveredPrimary || counts.UncoveredPrimaryScope != uncoveredPrimary ||
		counts.CoveredSupportingEvidence != coveredSupporting {
		t.Fatalf("producer coverage = %#v", counts)
	}
	metadata := result.Record.Call.Metadata
	if metadata.RequestedPrimaryScope != counts.RequestedPrimaryScope ||
		metadata.CoveredPrimaryScope != counts.CoveredPrimaryScope ||
		metadata.UncoveredPrimaryScope != counts.UncoveredPrimaryScope ||
		metadata.CoveredSupportingEvidence != counts.CoveredSupportingEvidence {
		t.Fatalf("record coverage does not match producer result: metadata=%#v counts=%#v", metadata, counts)
	}
}

func d239CandidateIndex(candidates []SynthesisCandidate, ref SynthesisMemberRef) int {
	for index, candidate := range candidates {
		if candidate.Ref == ref {
			return index
		}
	}
	return -1
}

func d239ProductionScopeBundle() d239ProductionScopeFixture {
	productionPackage := MemberID{Kind: MemberPackage, Value: "member-package-d239-production-private"}
	productionFile := MemberID{Kind: MemberFile, Value: "member-file-d239-production-private"}
	productionSymbol := MemberID{Kind: MemberSymbol, Value: "member-symbol-d239-production-private"}
	testPackage := MemberID{Kind: MemberPackage, Value: "member-package-d239-test-private"}
	testFile := MemberID{Kind: MemberFile, Value: "member-file-d239-test-private"}
	testSymbol := MemberID{Kind: MemberSymbol, Value: "member-symbol-d239-test-private"}
	toolingPackage := MemberID{Kind: MemberPackage, Value: "member-package-d239-tooling-private"}
	docsPackage := MemberID{Kind: MemberPackage, Value: "member-package-d239-docs-private"}

	return d239ProductionScopeFixture{
		ProductionPackage: productionPackage,
		ProductionSymbol:  productionSymbol,
		TestPackage:       testPackage,
		TestSymbol:        testSymbol,
		ToolingPackage:    toolingPackage,
		DocsPackage:       docsPackage,
		Bundle: CandidateBundle{
			Version: ContractVersion, RepositoryArchetype: ArchetypeApplication, GroundingMode: GroundingPackages,
			Candidates: []Candidate{
				{ID: productionPackage, Role: CandidateRoleConceptualMember, Name: "server/api", Facts: []LocalFact{testLocalFact(FactDeclaration, "example.invalid/repo/server/api", "server/api/api.go", 1)}},
				{ID: productionFile, Role: CandidateRoleStructuralLocator, Name: "server/api/api.go", ParentID: &productionPackage, Facts: []LocalFact{testLocalFact(FactRepositoryPath, "server/api/api.go", "server/api/api.go", 1)}},
				{ID: productionSymbol, Role: CandidateRoleConceptualMember, Name: "example.invalid/repo/server/api.Start", ParentID: &productionFile, Facts: []LocalFact{testLocalFact(FactDeclaration, "Start", "server/api/api.go", 2)}},
				{ID: testPackage, Role: CandidateRoleConceptualMember, Name: "server/tests/e2e", Facts: []LocalFact{testLocalFact(FactDeclaration, "example.invalid/repo/server/tests/e2e", "server/tests/e2e/e2e.go", 1)}},
				{ID: testFile, Role: CandidateRoleStructuralLocator, Name: "server/tests/e2e/e2e.go", ParentID: &testPackage, Facts: []LocalFact{testLocalFact(FactRepositoryPath, "server/tests/e2e/e2e.go", "server/tests/e2e/e2e.go", 1)}},
				{ID: testSymbol, Role: CandidateRoleConceptualMember, Name: "example.invalid/repo/server/tests/e2e.Run", ParentID: &testFile, Facts: []LocalFact{testLocalFact(FactDeclaration, "Run", "server/tests/e2e/e2e.go", 2)}},
				{ID: toolingPackage, Role: CandidateRoleConceptualMember, Name: "tools/generate", Facts: []LocalFact{testLocalFact(FactDeclaration, "example.invalid/repo/tools/generate", "tools/generate/main.go", 1)}},
				{ID: docsPackage, Role: CandidateRoleConceptualMember, Name: "docs/guide", Facts: []LocalFact{testLocalFact(FactDeclaration, "example.invalid/repo/docs/guide", "docs/guide/guide.go", 1)}},
			},
		},
	}
}
