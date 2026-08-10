package componentmap

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestD254MobyLikePackagesNestExactSymbols(t *testing.T) {
	t.Parallel()

	daemon := MemberID{Kind: MemberPackage, Value: "member-package-d254-daemon-router"}
	integration := MemberID{Kind: MemberPackage, Value: "member-package-d254-integration-router"}
	api := MemberID{Kind: MemberPackage, Value: "member-package-d254-api-router"}
	client := MemberID{Kind: MemberPackage, Value: "member-package-d254-client-router"}
	symbol := MemberID{Kind: MemberSymbol, Value: "member-symbol-d254-handler"}
	bundle := CandidateBundle{
		Version: ContractVersion, RepositoryArchetype: ArchetypeModularPlatformServer,
		GroundingMode: GroundingPackages,
		Candidates: []Candidate{
			{
				ID: daemon, Role: CandidateRoleConceptualMember, Name: "daemon/server/router",
				Facts: []LocalFact{
					testLocalFact(FactDeclaration, "github.com/moby/moby/v2/daemon/server/router", "daemon/server/router/router.go", 1),
					testLocalFact(FactExecutableRole, "dockerd runtime", "daemon/server/router/router.go", 1),
				},
			},
			{
				ID: integration, Role: CandidateRoleConceptualMember, Name: "integration/network/router",
				Facts: []LocalFact{testLocalFact(FactDeclaration, "github.com/moby/moby/v2/integration/network/router", "integration/network/router/router.go", 1)},
			},
			{
				ID: api, Role: CandidateRoleConceptualMember, Name: "api/types/router",
				Facts: []LocalFact{testLocalFact(FactDeclaration, "github.com/moby/moby/api/types/router", "api/types/router/router.go", 1)},
			},
			{
				ID: client, Role: CandidateRoleConceptualMember, Name: "client/internal/router",
				Facts: []LocalFact{testLocalFact(FactDeclaration, "github.com/moby/moby/client/internal/router", "client/internal/router/router.go", 1)},
			},
			{
				ID: symbol, Role: CandidateRoleConceptualMember, Name: "Handle", ParentID: &daemon,
				Facts: []LocalFact{testLocalFact(FactDeclaration, "github.com/moby/moby/v2/daemon/server/router.Handle", "daemon/server/router/router.go", 20)},
			},
		},
	}

	request, encoded, err := BuildSynthesisRequest(bundle)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := buildSynthesisPrivateCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	byRef := make(map[string]SynthesisCandidate, len(request.Candidates))
	for _, candidate := range request.Candidates {
		byRef[candidate.Ref.key()] = candidate
	}
	for id, wantPath := range map[MemberID]string{
		daemon: "v2/daemon/server/router", integration: "v2/integration/network/router",
		api: "api/types/router", client: "client/internal/router",
	} {
		candidate := byRef[catalog.membersByID[id].key()]
		if candidate.Label != "" || candidate.PackagePath != wantPath {
			t.Fatalf("package %s context = label:%q path:%q, want no label and %q", id.key(), candidate.Label, candidate.PackagePath, wantPath)
		}
	}
	if got := byRef[catalog.membersByID[symbol].key()]; got.Label != "Handle" || got.PackagePath != "" ||
		got.ParentRef == nil || *got.ParentRef != catalog.membersByID[daemon] {
		t.Fatalf("flat local symbol context changed: %#v", got)
	}

	type wireSymbol struct {
		Ref          SynthesisMemberRef    `json:"ref"`
		Label        string                `json:"label"`
		CoverageRole SynthesisCoverageRole `json:"coverage_role"`
		ParentRef    json.RawMessage       `json:"parent_ref"`
	}
	type wireCandidate struct {
		Ref         SynthesisMemberRef `json:"ref"`
		Label       *string            `json:"label"`
		PackagePath string             `json:"package_path"`
		Symbols     []wireSymbol       `json:"symbols"`
		Facts       []SynthesisFact    `json:"facts"`
	}
	var wire struct {
		Candidates []wireCandidate `json:"candidates"`
	}
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire.Candidates) != 4 {
		t.Fatalf("provider top-level candidates = %d, want four packages with no flat symbol duplicate: %s", len(wire.Candidates), encoded)
	}
	seenSymbol := 0
	for _, candidate := range wire.Candidates {
		if candidate.Label != nil || candidate.PackagePath == "" || candidate.Symbols == nil {
			t.Fatalf("package wire shape is not exact: %#v", candidate)
		}
		if candidate.Ref == catalog.membersByID[daemon] {
			if len(candidate.Symbols) != 1 || candidate.Symbols[0].Ref != catalog.membersByID[symbol] ||
				candidate.Symbols[0].Label != "Handle" ||
				candidate.Symbols[0].CoverageRole != SynthesisCoverageSupportingEvidence ||
				len(candidate.Symbols[0].ParentRef) != 0 {
				t.Fatalf("daemon nested symbols = %#v", candidate.Symbols)
			}
			seenSymbol++
			if len(candidate.Facts) != 1 || candidate.Facts[0].Kind != FactExecutableRole {
				t.Fatalf("nonredundant package facts were not preserved: %#v", candidate.Facts)
			}
		} else if len(candidate.Facts) != 0 {
			t.Fatalf("redundant static package declaration survived: %#v", candidate.Facts)
		}
	}
	if seenSymbol != 1 || strings.Count(string(encoded), catalog.membersByID[symbol].Ref) != 1 {
		t.Fatalf("nested symbol was lost or duplicated: %s", encoded)
	}
	if len(encoded) >= maxSynthesisRequestBytes {
		t.Fatalf("request bytes = %d, limit %d", len(encoded), maxSynthesisRequestBytes)
	}

	prompt, err := BuildSynthesisPrompt(bundle)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"package-owned symbols once under symbols",
		"every nested symbol explicitly carries coverage_role supporting_evidence",
		"package ref and every nested symbol ref are independently valid member_refs",
		"nesting proves only exact containment",
		"never requires co-selection or placement in the same component",
		"supporting_evidence never substitutes for a defensible p* primary_scope ref somewhere in the same production unit",
		"that p* ref may appear in a different component",
	} {
		if !strings.Contains(prompt.System, required) {
			t.Fatalf("nested package prompt omitted %q", required)
		}
	}
}

func TestD254ProviderProjectionRejectsMissingNestedPackage(t *testing.T) {
	t.Parallel()

	request := SynthesisRequest{
		RequiredMemberRefs: []SynthesisMemberRef{{Kind: MemberSymbol, Ref: "s1"}},
		Candidates: []SynthesisCandidate{{
			Ref:       SynthesisMemberRef{Kind: MemberSymbol, Ref: "s1"},
			ParentRef: &SynthesisMemberRef{Kind: MemberPackage, Ref: "p1"},
			Label:     "Handle", Facts: []SynthesisFact{},
		}},
	}
	if _, err := synthesisProviderRequestWire(request); err == nil || !strings.Contains(err.Error(), "parent") {
		t.Fatalf("symbol with an absent package parent reached provider projection: %v", err)
	}
}

func TestD275ProviderProjectionRejectsNestedPrimarySymbol(t *testing.T) {
	t.Parallel()

	packageRef := SynthesisMemberRef{Kind: MemberPackage, Ref: "p1"}
	symbolRef := SynthesisMemberRef{Kind: MemberSymbol, Ref: "s1"}
	request := SynthesisRequest{
		RequiredMemberRefs: []SynthesisMemberRef{packageRef, symbolRef},
		Candidates: []SynthesisCandidate{
			{Ref: packageRef, UnitRef: "u1", CoverageRole: SynthesisCoveragePrimaryScope, PackagePath: "runtime", Facts: []SynthesisFact{}},
			{Ref: symbolRef, ParentRef: &packageRef, UnitRef: "u1", CoverageRole: SynthesisCoveragePrimaryScope, Label: "Start", Facts: []SynthesisFact{}},
		},
	}
	if _, err := synthesisProviderRequestWire(request); err == nil || !strings.Contains(err.Error(), "supporting evidence") {
		t.Fatalf("nested primary symbol reached provider projection: %v", err)
	}
}
