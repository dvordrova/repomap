package reading

import (
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
)

func TestTargetBoundaryRestoresItsOriginalFactAndObject(t *testing.T) {
	boundary := atlas.Place{ID: "bnd:shared", Kind: atlas.PlaceBoundary, Path: "api.go", LineNo: 43, Column: 7, Parent: "file:api",
		TargetIDs: []string{"main", "library", "unobserved"}, Boundary: &atlas.BoundaryFacts{Source: "fact", ObjectID: "main-handler", GivenKind: atlas.BoundaryHTTPServer,
			Direction: atlas.DirectionIn, Method: "ANY", Values: []string{"/v1/update"}, Origins: []atlas.BoundaryOrigin{
				{TargetID: "main", FactID: "original-route", ObjectID: "main-handler"}, {TargetID: "library", FactID: "original-route-2", ObjectID: "library-handler"},
			}}}
	r := reader{boundaries: map[string]*boundaryState{boundary.ID: {place: boundary, kind: atlas.BoundaryHTTPServer, line: "Handles updates."}}, boxOf: map[string]string{boundary.Parent: "api"}}
	for _, target := range []string{"main", "library", "unobserved"} {
		got := r.target(TargetMeta{ID: target}).Boundaries
		if target == "unobserved" {
			if len(got) != 0 {
				t.Fatalf("borrowed a sibling fact: %+v", got)
			}
			continue
		}
		want := "original-route"
		if target == "library" {
			want += "-2"
		}
		if len(got) != 1 || got[0].FactID != want || got[0].ObjectID != target+"-handler" || got[0].Method != "ANY" || len(got[0].Values) != 1 || got[0].Values[0] != "/v1/update" || got[0].LineNo != 43 || got[0].Column != 7 {
			t.Fatalf("wrong native target projection for %s: %+v", target, got)
		}
	}
	if boundary.Boundary.ObjectID != "main-handler" || boundary.Boundary.Origins[1].FactID != "original-route-2" {
		t.Fatal("projection mutated the shared source")
	}
}

func TestSharedBoundaryContextUsesItsNativeSubjectBeforeRepresentativeObject(t *testing.T) {
	shared := atlas.Place{ID: "sym:handler", Kind: atlas.PlaceSymbol, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "other-view-handler", Name: "Handle"}}}
	incidental := atlas.Place{ID: "sym:other", Kind: atlas.PlaceSymbol, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "other", Name: "Handle"}}}
	owners := map[string]atlas.Place{shared.ID: shared, "other-view-handler": shared, "unrelated": incidental}
	native := &atlas.BoundaryFacts{Source: "fact", SubjectID: shared.ID, ObjectID: "first-view-handler"}
	if got := boundaryOwner(native, owners); got.ID != shared.ID {
		t.Fatalf("exact shared declaration lost: %+v", got)
	}
	native.ObjectID = "unrelated"
	if got := boundaryOwner(native, owners); got.ID != shared.ID {
		t.Fatalf("native subject lost precedence: %+v", got)
	}
	native.SubjectID = "unknown"
	native.ObjectID = "first-view-handler"
	if got := boundaryOwner(native, owners); got.Symbol != nil {
		t.Fatal("same-name declaration was guessed")
	}
}

func TestTargetOmitsBoundariesWhoseFileItDoesNotHold(t *testing.T) {
	// Membership is decided in places; the projection still refuses to name
	// a box the target does not have, so a foreign scope cannot fail atlas
	// validation at publication.
	boundary := atlas.Place{ID: "bnd:shared", Kind: atlas.PlaceBoundary, Path: "api.go", LineNo: 33, Column: 4, Parent: "file:api",
		TargetIDs: []string{"main", "tool"}, Boundary: &atlas.BoundaryFacts{Source: "fact", ObjectID: "main-handler", GivenKind: atlas.BoundaryHTTPServer,
			Direction: atlas.DirectionIn, Method: "POST", Values: []string{"/update"}, Origins: []atlas.BoundaryOrigin{
				{TargetID: "main", FactID: "main-route", ObjectID: "main-handler"}, {TargetID: "tool", FactID: "tool-route", ObjectID: "tool-handler"},
			}}}
	r := reader{
		places:     map[string]atlas.Place{"file:api": {ID: "file:api", Kind: atlas.PlaceFile, Path: "api.go", TargetIDs: []string{"main"}}},
		boundaries: map[string]*boundaryState{boundary.ID: {place: boundary, kind: atlas.BoundaryHTTPServer, line: "Handles updates."}},
		boxOf:      map[string]string{boundary.Parent: "api"},
	}
	if got := r.target(TargetMeta{ID: "tool"}).Boundaries; len(got) != 0 {
		t.Fatalf("boundary named a box the tool target does not have: %+v", got)
	}
	if got := r.target(TargetMeta{ID: "main"}).Boundaries; len(got) != 1 || got[0].BoxID != "api" || got[0].FactID != "main-route" {
		t.Fatalf("owner lost its boundary: %+v", got)
	}
}
