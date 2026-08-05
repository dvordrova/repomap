package componentmap

import (
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
)

func TestCompileUnitCatalogDeterministicUnderShuffledInput(t *testing.T) {
	t.Parallel()

	bundle := unitFixtureBundle()
	first, err := CompileUnitCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	shuffled := bundle
	// Reverse candidate order deterministically.
	reversed := make([]Candidate, len(bundle.Candidates))
	for index, candidate := range bundle.Candidates {
		reversed[len(bundle.Candidates)-1-index] = candidate
	}
	shuffled.Candidates = reversed
	second, err := CompileUnitCatalog(shuffled)
	if err != nil {
		t.Fatal(err)
	}
	if first.SHA256 != second.SHA256 {
		t.Fatalf("unit catalog digest changed under input reordering: %s vs %s", first.SHA256, second.SHA256)
	}
	if len(first.Units) != len(second.Units) {
		t.Fatalf("unit count changed under input reordering: %d vs %d", len(first.Units), len(second.Units))
	}
}

func TestCompileUnitCatalogCompleteMemberCoverage(t *testing.T) {
	t.Parallel()

	bundle := unitFixtureBundle()
	catalog, err := CompileUnitCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, unit := range catalog.Units {
		total += len(unit.MemberIDs)
	}
	if total != len(bundle.Candidates) {
		t.Fatalf("member coverage = %d, want %d (every exact raw member in exactly one primary unit)", total, len(bundle.Candidates))
	}
	if catalog.CoveredMembers != len(bundle.Candidates) {
		t.Fatalf("covered = %d, want %d", catalog.CoveredMembers, len(bundle.Candidates))
	}
}

func TestCompileUnitCatalogRoleSeparation(t *testing.T) {
	t.Parallel()

	bundle := unitFixtureBundle()
	catalog, err := CompileUnitCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	var testUnits, productionUnits int
	for _, unit := range catalog.Units {
		switch unit.Role {
		case UnitRoleTest:
			testUnits++
		case UnitRoleProduction:
			productionUnits++
		}
	}
	if testUnits == 0 {
		t.Fatalf("no test-role units: %#v", catalog.Units)
	}
	if productionUnits == 0 {
		t.Fatalf("no production-role units: %#v", catalog.Units)
	}
}

func TestCompileUnitCatalogWireRefsAreBoundedAndCanonicalFree(t *testing.T) {
	t.Parallel()

	bundle := unitFixtureBundle()
	catalog, err := CompileUnitCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.WireUnits) != len(catalog.Units) {
		t.Fatalf("wire unit count = %d, want %d", len(catalog.WireUnits), len(catalog.Units))
	}
	// Canonical IDs must never leak into wire labels or representative labels.
	for _, wireUnit := range catalog.WireUnits {
		for _, candidate := range bundle.Candidates {
			if containsSubstring(wireUnit.Label, candidate.ID.Value) {
				t.Fatalf("wire unit label leaked canonical ID %q: %q", candidate.ID.Value, wireUnit.Label)
			}
			for _, label := range wireUnit.RepresentativeLabels {
				if containsSubstring(label, candidate.ID.Value) {
					t.Fatalf("wire representative label leaked canonical ID %q: %q", candidate.ID.Value, label)
				}
			}
		}
	}
}

func TestUnitCatalogUnitMembersByWireRefExpansion(t *testing.T) {
	t.Parallel()

	bundle := unitFixtureBundle()
	catalog, err := CompileUnitCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	membersByRef := unitCatalogUnitMembersByWireRef(catalog)
	if len(membersByRef) != len(catalog.Units) {
		t.Fatalf("expansion map count = %d, want %d", len(membersByRef), len(catalog.Units))
	}
	seen := map[string]bool{}
	total := 0
	for ref, members := range membersByRef {
		if !containsSubstring(ref, "u") {
			t.Fatalf("unit wire ref %q is not a u* ref", ref)
		}
		for _, memberID := range members {
			if seen[memberID.key()] {
				t.Fatalf("member %s expanded into more than one unit", memberID.key())
			}
			seen[memberID.key()] = true
			total++
		}
	}
	if total != len(bundle.Candidates) {
		t.Fatalf("expansion total = %d, want %d", total, len(bundle.Candidates))
	}
}

// unitFixtureBundle builds a small mixed-role bundle for unit compiler tests.
func unitFixtureBundle() CandidateBundle {
	declarationFact := func(value string) []LocalFact {
		return []LocalFact{{Kind: FactDeclaration, Value: value, Certainty: evidence.CertaintyStatic,
			Provenance: []evidence.Provenance{{Provider: "fixture", Version: "v1", Operation: "local_fact"}}}}
	}
	production := Candidate{
		ID:   MemberID{Kind: MemberPackage, Value: "member-package-prod-a"},
		Role: CandidateRoleConceptualMember, Name: "server/api",
		Facts: declarationFact("server/api"),
	}
	productionSymbol := Candidate{
		ID:   MemberID{Kind: MemberSymbol, Value: "member-symbol-prod-handler"},
		Role: CandidateRoleConceptualMember, Name: "server/api/handler",
		ParentID: &production.ID, Facts: declarationFact("server/api/handler"),
	}
	testPkg := Candidate{
		ID:   MemberID{Kind: MemberPackage, Value: "member-package-test-e2e"},
		Role: CandidateRoleConceptualMember, Name: "server/tests/e2e",
		Facts: declarationFact("server/tests/e2e"),
	}
	toolPkg := Candidate{
		ID:   MemberID{Kind: MemberPackage, Value: "member-package-tools-gen"},
		Role: CandidateRoleConceptualMember, Name: "tools/generate",
		Facts: declarationFact("tools/generate"),
	}
	docPkg := Candidate{
		ID:   MemberID{Kind: MemberPackage, Value: "member-package-docs-guide"},
		Role: CandidateRoleConceptualMember, Name: "docs/guide",
		Facts: declarationFact("docs/guide"),
	}
	bundle := CandidateBundle{
		Version:             ContractVersion,
		RepositoryArchetype: ArchetypeModularPlatformServer,
		GroundingMode:       GroundingMixed,
		Candidates:          []Candidate{production, productionSymbol, testPkg, toolPkg, docPkg},
		BehaviorAnchors: []BehaviorAnchor{{
			ID: "anchor-process", Kind: AnchorProcessEntry, ProofMode: AnchorProofProcessEntry,
			Label: "process entry", Certainty: evidence.CertaintyStatic,
			Location:    evidence.Location{Path: "server/main.go", Line: 1, Column: 1},
			Scenario:    ScenarioContext{ID: "scenario-unit-test", Name: "unit test scenario"},
			Producer:    evidence.Provenance{Provider: "fixture", Version: "v1", Operation: "anchor"},
			Limitations: []string{"static fixture anchor"},
			MemberIDs:   []MemberID{production.ID},
		}},
		Relations: []LocalRelation{},
	}
	return bundle
}

func containsSubstring(value, substring string) bool {
	for index := 0; index+len(substring) <= len(value); index++ {
		if value[index:index+len(substring)] == substring {
			return true
		}
	}
	return false
}
