package componentmap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type d238PrimaryScopeFixture struct {
	Bundle      CandidateBundle
	WebPackage  MemberID
	WebSymbol   MemberID
	CorePackage MemberID
	CoreSymbol  MemberID
}

func TestD238BuildSynthesisRequestCarriesClosedPackageContext(t *testing.T) {
	t.Parallel()

	fixture := d238PrimaryScopeBundle()
	request, encoded, err := BuildSynthesisRequest(fixture.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := buildSynthesisPrivateCatalog(fixture.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[MemberID]SynthesisCandidate)
	for _, candidate := range request.Candidates {
		memberID := catalog.membersByRef[candidate.Ref.key()]
		byID[memberID] = candidate
	}
	for _, packageID := range []MemberID{fixture.WebPackage, fixture.CorePackage} {
		candidate := byID[packageID]
		if candidate.CoverageRole != SynthesisCoveragePrimaryScope || candidate.UnitRef == "" {
			t.Fatalf("package context = %#v", candidate)
		}
	}
	for _, pair := range []struct {
		symbol MemberID
		pkg    MemberID
	}{
		{symbol: fixture.WebSymbol, pkg: fixture.WebPackage},
		{symbol: fixture.CoreSymbol, pkg: fixture.CorePackage},
	} {
		candidate := byID[pair.symbol]
		owner := byID[pair.pkg]
		if candidate.CoverageRole != SynthesisCoverageSupportingEvidence ||
			candidate.ParentRef == nil || *candidate.ParentRef != owner.Ref ||
			candidate.UnitRef != owner.UnitRef {
			t.Fatalf("file-mediated symbol context = %#v, owner = %#v", candidate, owner)
		}
	}
	for _, forbidden := range []string{
		fixture.WebPackage.Value,
		fixture.WebSymbol.Value,
		"example.invalid/private",
		"area-web/router.go",
	} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("bounded request leaked %q: %s", forbidden, encoded)
		}
	}
	for _, required := range []string{`"parent_ref":`, `"unit_ref":`, `"coverage_role":"primary_scope"`, `"coverage_role":"supporting_evidence"`} {
		if !bytes.Contains(encoded, []byte(required)) {
			t.Fatalf("bounded request omitted %q: %s", required, encoded)
		}
	}
}

func TestD238SynthesisRequestContextValidationFailsClosed(t *testing.T) {
	t.Parallel()

	fixture := d238PrimaryScopeBundle()
	request, _, err := BuildSynthesisRequest(fixture.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := buildSynthesisPrivateCatalog(fixture.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	indexByID := make(map[MemberID]int)
	for index, candidate := range request.Candidates {
		indexByID[catalog.membersByRef[candidate.Ref.key()]] = index
	}
	tests := []struct {
		name   string
		mutate func(*SynthesisRequest)
	}{
		{
			name: "open coverage role",
			mutate: func(value *SynthesisRequest) {
				value.Candidates[indexByID[fixture.WebPackage]].CoverageRole = "other"
			},
		},
		{
			name: "unadvertised unit",
			mutate: func(value *SynthesisRequest) {
				value.Candidates[indexByID[fixture.WebPackage]].UnitRef = "u-missing"
			},
		},
		{
			name: "supporting evidence without package",
			mutate: func(value *SynthesisRequest) {
				value.Candidates[indexByID[fixture.WebSymbol]].ParentRef = nil
			},
		},
		{
			name: "non-package parent",
			mutate: func(value *SynthesisRequest) {
				parent := value.Candidates[indexByID[fixture.CoreSymbol]].Ref
				value.Candidates[indexByID[fixture.WebSymbol]].ParentRef = &parent
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutated := request
			mutated.Candidates = append([]SynthesisCandidate(nil), request.Candidates...)
			test.mutate(&mutated)
			if err := validateSynthesisRequestCoverage(mutated); err == nil {
				t.Fatal("invalid request context was accepted")
			}
		})
	}
	collidingUnitRef := string(request.Candidates[0].UnitRef)
	identityCatalog := synthesisPrivateCatalog{canonicalOpaqueIDs: map[string]struct{}{collidingUnitRef: {}}}
	if err := validateSynthesisRequestIdentityFields(identityCatalog, request); err == nil ||
		!strings.Contains(err.Error(), "unit_ref") || strings.Contains(err.Error(), collidingUnitRef) {
		t.Fatalf("candidate unit identity collision error = %v", err)
	}
}

func TestD238SynthesisPromptPrioritizesPrimaryScopeAndKeepsMemberOnlyOutput(t *testing.T) {
	t.Parallel()

	prompt, err := BuildSynthesisPrompt(d238PrimaryScopeBundle().Bundle)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"Candidates marked primary_scope form the top-level production conceptual repository surface.",
		"Cover defensible primary_scope across the supplied production units before selecting supporting_evidence.",
		"coverage of them never compensates for uncovered production primary_scope.",
		"a non-empty member_refs array",
		"Candidate parent_ref, unit_ref, and coverage_role fields are read-only context and must never be returned.",
	} {
		if !strings.Contains(prompt.System, required) {
			t.Fatalf("prompt omitted %q:\n%s", required, prompt.System)
		}
	}
	if strings.Contains(prompt.System, "unit_refs") {
		t.Fatalf("live prompt still advertises historical unit_refs output:\n%s", prompt.System)
	}
}

func TestD238GHZLikeAllSupportingResponseFailsPrimaryScopeQuality(t *testing.T) {
	t.Parallel()

	bundle, packageIDs, symbolIDs := d238GHZLikeBundle()
	request, _, err := BuildSynthesisRequest(bundle)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := buildSynthesisPrivateCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	primary, supporting := 0, 0
	refs := make([]SynthesisMemberRef, 0, len(symbolIDs))
	for _, candidate := range request.Candidates {
		switch candidate.CoverageRole {
		case SynthesisCoveragePrimaryScope:
			primary++
		case SynthesisCoverageSupportingEvidence:
			supporting++
			refs = append(refs, candidate.Ref)
		}
	}
	if primary != len(packageIDs) || primary != 18 || supporting != len(symbolIDs) || supporting != 10 {
		t.Fatalf("request roles primary/supporting = %d/%d", primary, supporting)
	}
	for _, symbolID := range symbolIDs {
		ref := catalog.membersByID[symbolID]
		found := false
		for _, candidate := range request.Candidates {
			if candidate.Ref == ref {
				found = candidate.ParentRef != nil && candidate.UnitRef != ""
				break
			}
		}
		if !found {
			t.Fatalf("symbol %q lacks bounded package context", symbolID.key())
		}
	}
	response := d238NestedResponse(t, refs)
	result, err := RecordSynthesisResponse(bundle, "d238-ghz-like", "test", "test", 0, response)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Landscape.Fallback || result.Landscape.ValidationOutcome != ValidationRejected {
		t.Fatalf("all-supporting proposal was not rejected: %#v", result.Landscape)
	}
	for _, code := range []string{"proposal.empty_primary_scope_coverage", "proposal.supporting_only_unit_coverage"} {
		if !d238HasDiagnostic(result.Landscape.Diagnostics, code) {
			t.Fatalf("quality diagnostics omitted %q: %#v", code, result.Landscape.Diagnostics)
		}
	}
	counts := result.Membership
	if !counts.Counted || counts.RequestedPrimaryScope != 18 || counts.CoveredPrimaryScope != 0 ||
		counts.UncoveredPrimaryScope != 18 || counts.CoveredSupportingEvidence != 10 {
		t.Fatalf("quality rejection coverage = %#v", counts)
	}
	metadata := result.Record.Call.Metadata
	if metadata.RequestedPrimaryScope != 18 || metadata.CoveredPrimaryScope != 0 ||
		metadata.UncoveredPrimaryScope != 18 || metadata.CoveredSupportingEvidence != 10 {
		t.Fatalf("quality rejection metadata = %#v", metadata)
	}
}

func TestD238PackageAndSeparateSupportingEvidenceRemainAcceptedPartial(t *testing.T) {
	t.Parallel()

	fixture := d238PrimaryScopeBundle()
	catalog, err := buildSynthesisPrivateCatalog(fixture.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	response := d238NestedResponse(t, []SynthesisMemberRef{
		catalog.membersByID[fixture.WebPackage],
		catalog.membersByID[fixture.WebSymbol],
	})
	result, err := RecordSynthesisResponse(fixture.Bundle, "d238-valid-partial", "test", "test", 0, response)
	if err != nil {
		t.Fatal(err)
	}
	if result.Landscape.Fallback || result.Landscape.ValidationOutcome != ValidationAcceptedPartial {
		t.Fatalf("defensible partial proposal rejected: %#v", result.Landscape)
	}
	if d238HasDiagnostic(result.Landscape.Diagnostics, "proposal.empty_primary_scope_coverage") ||
		d238HasDiagnostic(result.Landscape.Diagnostics, "proposal.supporting_only_unit_coverage") {
		t.Fatalf("valid package/supporting coverage received quality finding: %#v", result.Landscape.Diagnostics)
	}
	counts := result.Membership
	if counts.RequestedPrimaryScope != 2 || counts.CoveredPrimaryScope != 1 ||
		counts.UncoveredPrimaryScope != 1 || counts.CoveredSupportingEvidence != 1 {
		t.Fatalf("accepted partial coverage = %#v", counts)
	}
	metadata := result.Record.Call.Metadata
	if metadata.RequestedPrimaryScope != counts.RequestedPrimaryScope ||
		metadata.CoveredPrimaryScope != counts.CoveredPrimaryScope ||
		metadata.UncoveredPrimaryScope != counts.UncoveredPrimaryScope ||
		metadata.CoveredSupportingEvidence != counts.CoveredSupportingEvidence {
		t.Fatalf("record coverage does not match producer result: metadata=%#v counts=%#v", metadata, counts)
	}
}

func TestD238SupportingEvidenceFromUncoveredUnitFailsQuality(t *testing.T) {
	t.Parallel()

	fixture := d238PrimaryScopeBundle()
	catalog, err := buildSynthesisPrivateCatalog(fixture.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	response := d238NestedResponse(t, []SynthesisMemberRef{
		catalog.membersByID[fixture.WebPackage],
		catalog.membersByID[fixture.CoreSymbol],
	})
	result, err := RecordSynthesisResponse(fixture.Bundle, "d238-supporting-only", "test", "test", 0, response)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Landscape.Fallback || !d238HasDiagnostic(result.Landscape.Diagnostics, "proposal.supporting_only_unit_coverage") {
		t.Fatalf("supporting-only unit coverage was accepted: %#v", result.Landscape)
	}
	if d238HasDiagnostic(result.Landscape.Diagnostics, "proposal.empty_primary_scope_coverage") {
		t.Fatalf("non-empty primary coverage received empty-primary finding: %#v", result.Landscape.Diagnostics)
	}
}

func TestD238BehaviorOnlyCandidatesRemainPrimaryAndUsable(t *testing.T) {
	t.Parallel()

	first := MemberID{Kind: MemberSymbol, Value: "member-symbol-d238-behavior-first"}
	second := MemberID{Kind: MemberSymbol, Value: "member-symbol-d238-behavior-second"}
	bundle := CandidateBundle{
		Version: ContractVersion, RepositoryArchetype: ArchetypeLibraryFramework, GroundingMode: GroundingPackages,
		Candidates: []Candidate{
			{ID: first, Role: CandidateRoleConceptualMember, Name: "First", Facts: []LocalFact{testLocalFact(FactDeclaration, "First", "first.go", 1)}},
			{ID: second, Role: CandidateRoleConceptualMember, Name: "Second", Facts: []LocalFact{testLocalFact(FactDeclaration, "Second", "second.go", 1)}},
		},
	}
	request, _, err := BuildSynthesisRequest(bundle)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range request.Candidates {
		if candidate.CoverageRole != SynthesisCoveragePrimaryScope || candidate.ParentRef != nil {
			t.Fatalf("behavior-only context = %#v", candidate)
		}
	}
	catalog, err := buildSynthesisPrivateCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RecordSynthesisResponse(
		bundle,
		"d238-behavior-only",
		"test",
		"test",
		0,
		d238NestedResponse(t, []SynthesisMemberRef{catalog.membersByID[first]}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Landscape.Fallback || result.Membership.RequestedPrimaryScope != 2 ||
		result.Membership.CoveredPrimaryScope != 1 || result.Membership.CoveredSupportingEvidence != 0 {
		t.Fatalf("behavior-only primary coverage = landscape:%#v counts:%#v", result.Landscape, result.Membership)
	}
}

func d238PrimaryScopeBundle() d238PrimaryScopeFixture {
	webPackage := MemberID{Kind: MemberPackage, Value: "member-package-d238-web-private"}
	webFile := MemberID{Kind: MemberFile, Value: "member-file-d238-web-private"}
	webSymbol := MemberID{Kind: MemberSymbol, Value: "member-symbol-d238-web-private"}
	corePackage := MemberID{Kind: MemberPackage, Value: "member-package-d238-core-private"}
	coreFile := MemberID{Kind: MemberFile, Value: "member-file-d238-core-private"}
	coreSymbol := MemberID{Kind: MemberSymbol, Value: "member-symbol-d238-core-private"}
	return d238PrimaryScopeFixture{
		WebPackage: webPackage, WebSymbol: webSymbol, CorePackage: corePackage, CoreSymbol: coreSymbol,
		Bundle: CandidateBundle{
			Version: ContractVersion, RepositoryArchetype: ArchetypeCLITool, GroundingMode: GroundingPackages,
			Candidates: []Candidate{
				{ID: webPackage, Role: CandidateRoleConceptualMember, Name: "area-web/router", Facts: []LocalFact{testLocalFact(FactDeclaration, "router", "area-web/router.go", 1)}},
				{ID: webFile, Role: CandidateRoleStructuralLocator, Name: "area-web/router.go", ParentID: &webPackage, Facts: []LocalFact{testLocalFact(FactRepositoryPath, "area-web/router.go", "area-web/router.go", 1)}},
				{ID: webSymbol, Role: CandidateRoleConceptualMember, Name: "example.invalid/private/web.Router", ParentID: &webFile, Facts: []LocalFact{testLocalFact(FactDeclaration, "Router", "area-web/router.go", 2)}},
				{ID: corePackage, Role: CandidateRoleConceptualMember, Name: "area-core/runtime", Facts: []LocalFact{testLocalFact(FactDeclaration, "runtime", "area-core/runtime.go", 1)}},
				{ID: coreFile, Role: CandidateRoleStructuralLocator, Name: "area-core/runtime.go", ParentID: &corePackage, Facts: []LocalFact{testLocalFact(FactRepositoryPath, "area-core/runtime.go", "area-core/runtime.go", 1)}},
				{ID: coreSymbol, Role: CandidateRoleConceptualMember, Name: "example.invalid/private/core.Start", ParentID: &coreFile, Facts: []LocalFact{testLocalFact(FactDeclaration, "Start", "area-core/runtime.go", 2)}},
			},
		},
	}
}

func d238GHZLikeBundle() (CandidateBundle, []MemberID, []MemberID) {
	bundle := CandidateBundle{
		Version: ContractVersion, RepositoryArchetype: ArchetypeCLITool, GroundingMode: GroundingPackages,
	}
	packages := make([]MemberID, 0, 18)
	symbols := make([]MemberID, 0, 10)
	for index := 0; index < 18; index++ {
		packageID := MemberID{Kind: MemberPackage, Value: fmt.Sprintf("member-package-d238-ghz-%02d-private", index)}
		packages = append(packages, packageID)
		packageName := fmt.Sprintf("area-%02d/package", index)
		bundle.Candidates = append(bundle.Candidates, Candidate{
			ID: packageID, Role: CandidateRoleConceptualMember, Name: packageName,
			Facts: []LocalFact{testLocalFact(FactDeclaration, "package", packageName+"/package.go", 1)},
		})
		if index >= 10 {
			continue
		}
		fileID := MemberID{Kind: MemberFile, Value: fmt.Sprintf("member-file-d238-ghz-%02d-private", index)}
		symbolID := MemberID{Kind: MemberSymbol, Value: fmt.Sprintf("member-symbol-d238-ghz-%02d-private", index)}
		symbols = append(symbols, symbolID)
		fileName := fmt.Sprintf("area-%02d/package/member.go", index)
		bundle.Candidates = append(bundle.Candidates,
			Candidate{
				ID: fileID, Role: CandidateRoleStructuralLocator, Name: fileName, ParentID: &packageID,
				Facts: []LocalFact{testLocalFact(FactRepositoryPath, fileName, fileName, 1)},
			},
			Candidate{
				ID: symbolID, Role: CandidateRoleConceptualMember, Name: fmt.Sprintf("example.invalid/private/area%d.Member", index), ParentID: &fileID,
				Facts: []LocalFact{testLocalFact(FactDeclaration, "Member", fileName, 2)},
			},
		)
	}
	return bundle, packages, symbols
}

func d238NestedResponse(t *testing.T, refs []SynthesisMemberRef) []byte {
	t.Helper()
	memberRefs := make([]string, len(refs))
	for index, ref := range refs {
		memberRefs[index] = ref.Ref
	}
	wire := struct {
		Subsystems []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Components  []struct {
				Name        string   `json:"name"`
				Description string   `json:"description"`
				MemberRefs  []string `json:"member_refs"`
				AnchorRefs  []string `json:"anchor_refs"`
			} `json:"components"`
		} `json:"subsystems"`
	}{}
	subsystem := struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Components  []struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			MemberRefs  []string `json:"member_refs"`
			AnchorRefs  []string `json:"anchor_refs"`
		} `json:"components"`
	}{Name: "Repository", Description: "Bounded repository scope"}
	for index, memberRef := range memberRefs {
		subsystem.Components = append(subsystem.Components, struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			MemberRefs  []string `json:"member_refs"`
			AnchorRefs  []string `json:"anchor_refs"`
		}{
			Name: fmt.Sprintf("Responsibility %d", index+1), Description: "Exact bounded responsibility",
			MemberRefs: []string{memberRef}, AnchorRefs: []string{},
		})
	}
	wire.Subsystems = append(wire.Subsystems, subsystem)
	encoded, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func d238HasDiagnostic(diagnostics []Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
