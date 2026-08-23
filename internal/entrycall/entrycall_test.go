package entrycall

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCompileKeepsExactSubstratePrivateAndBuildsSurfaceAuthority(t *testing.T) {
	substrate := surfaceSubstrateFixture()
	accidental, err := json.Marshal(substrate)
	if err != nil || string(accidental) != "{}" {
		t.Fatalf("exact substrate became JSON-visible: %s, %v", accidental, err)
	}

	compiled, err := Compile(substrate)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if compiled.SubstrateSHA256 == "" || len(compiled.Request.SurfaceCatalog.Candidates) != 1 {
		t.Fatalf("compiled exact authority = %#v", compiled)
	}
	candidate := compiled.Request.SurfaceCatalog.Candidates[0]
	rootNodeID, known := compiled.RootNodeID(candidate.RootRef)
	if !known || rootNodeID != "canonical:root" {
		t.Fatalf("root authority = %q, %v", rootNodeID, known)
	}
	wire, err := json.Marshal(compiled.Request)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{
		"canonical:root", "canonical:route", "routes.go",
		`"entries"`, `"nodes"`, `"families"`, `"frontier"`,
	} {
		if strings.Contains(string(wire), private) {
			t.Fatalf("bounded request leaked private authority %q: %s", private, wire)
		}
	}
	if !strings.Contains(string(wire), `"value":"/v1/jobs"`) {
		t.Fatalf("bounded surface fact disappeared: %s", wire)
	}

	snapshot := substrate.Snapshot()
	snapshot.SurfaceCandidates[0].Facts[0].Value = "changed"
	if substrate.SurfaceCandidates[0].Facts[0].Value == "changed" {
		t.Fatal("Substrate.Snapshot shares exact surface facts with its source")
	}
}

func TestReduceRestoresOnlyExactSurfaceFacts(t *testing.T) {
	compiled, err := Compile(surfaceSubstrateFixture())
	if err != nil {
		t.Fatal(err)
	}
	candidate := compiled.Request.SurfaceCatalog.Candidates[0]
	factRef := func(kind SurfaceFactKind) string {
		t.Helper()
		for _, fact := range candidate.Facts {
			if fact.Kind == kind {
				return fact.Ref
			}
		}
		t.Fatalf("candidate has no %q fact: %#v", kind, candidate)
		return ""
	}
	response := Response{
		Version: ResponseVersion,
		SurfaceProposals: []ResponseSurfaceProposal{{
			CandidateRef: candidate.Ref,
			KindRef:      SurfaceKindRefHTTPRoute,
			Bindings: []ResponseSurfaceBinding{
				{SlotRef: SurfaceSlotRefMethod, FactRef: factRef(SurfaceFactToken)},
				{SlotRef: SurfaceSlotRefPath, FactRef: factRef(SurfaceFactString)},
				{SlotRef: SurfaceSlotRefHandler, FactRef: factRef(SurfaceFactCallable)},
			},
		}},
	}
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Reduce(compiled, raw)
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	if len(result.SurfaceProposals) != 1 || len(result.RejectedSurfaceProposals) != 0 {
		t.Fatalf("restored surfaces = %#v", result)
	}
	surface := result.SurfaceProposals[0]
	if surface.Kind != SurfaceKindHTTPRoute || surface.Role != SurfaceRoleEntrySurface ||
		surface.Path == nil || surface.Path.Text != "/v1/jobs" || surface.Path.Location == nil ||
		surface.Path.Location.Path != "routes.go" || surface.Handler == nil ||
		surface.Handler.Text != "createJob" {
		t.Fatalf("restored exact surface = %#v", surface)
	}
}

func TestReduceRejectsUnknownRefsWithoutPartialResult(t *testing.T) {
	compiled, err := Compile(surfaceSubstrateFixture())
	if err != nil {
		t.Fatal(err)
	}
	response := Response{
		Version: ResponseVersion,
		SurfaceProposals: []ResponseSurfaceProposal{{
			CandidateRef: "c999", KindRef: SurfaceKindRefHTTPRoute,
			Bindings: []ResponseSurfaceBinding{{SlotRef: SurfaceSlotRefPath, FactRef: "x999"}},
		}},
	}
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Reduce(compiled, raw)
	if err == nil || result.SurfaceProposals != nil || result.RejectedSurfaceProposals != nil {
		t.Fatalf("unknown ref returned partial authority: %#v, %v", result, err)
	}
}

func TestCompileDoesNotDropCandidateRootsBeforeCatalogBounds(t *testing.T) {
	substrate := surfaceSubstrateFixture()
	substrate.Roots = []ExactRoot{
		{NodeID: "canonical:root-1"}, {NodeID: "canonical:root-2"},
		{NodeID: "canonical:root-3"}, {NodeID: "canonical:root-4"},
		{NodeID: "canonical:root-5"},
	}
	substrate.Coverage.RootsConsidered = len(substrate.Roots)
	substrate.SurfaceCandidates[0].RootNodeID = "canonical:root-5"

	compiled, err := Compile(substrate)
	if err != nil {
		t.Fatal(err)
	}
	candidate := compiled.Request.SurfaceCatalog.Candidates[0]
	rootNodeID, known := compiled.RootNodeID(candidate.RootRef)
	if !known || rootNodeID != "canonical:root-5" || compiled.SurfaceCoverage().OmittedCandidates != 0 {
		t.Fatalf("candidate root/coverage = %q, %v, %#v", rootNodeID, known, compiled.SurfaceCoverage())
	}
}

func surfaceSubstrateFixture() Substrate {
	registration := Location{Path: "routes.go", Line: 12, Column: 3}
	handler := Location{Path: "handlers.go", Line: 25, Column: 1}
	return Substrate{
		Version: SubstrateVersion,
		State:   StateReady,
		Roots:   []ExactRoot{{NodeID: "canonical:root"}},
		SurfaceCandidates: []ExactSurfaceCandidate{{
			ID: "canonical:route", RootNodeID: "canonical:root",
			Form: SurfaceCandidateDirectCall, Sketch: "POST /v1/jobs", Site: registration,
			Facts: []ExactSurfaceFact{
				{ID: "canonical:method", Kind: SurfaceFactToken, Position: 1, Label: "method", Value: "POST", Location: registration},
				{ID: "canonical:path", Kind: SurfaceFactString, Position: 2, Label: "path", Value: "/v1/jobs", Location: registration},
				{ID: "canonical:handler", Kind: SurfaceFactCallable, Position: 3, Label: "handler", Value: "createJob", Location: handler},
			},
		}},
		Coverage: Coverage{
			RootsConsidered:             1,
			SurfaceCandidatesConsidered: 1, SurfaceCandidatesIndexed: 1,
			SurfaceCandidateFactsConsidered: 3, SurfaceCandidateFactsIndexed: 3,
		},
	}
}
