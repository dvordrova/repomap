package atlas

import (
	"bytes"
	"strings"
	"testing"
)

func TestSealCanonicalizesCompleteBoundaryOriginsWithoutMutatingTheInput(t *testing.T) {
	first := BoundaryOrigin{TargetID: "a", FactID: "fact", ObjectID: "a-handler"}
	second := BoundaryOrigin{TargetID: "b", FactID: "fact-2", ObjectID: "b-handler"}
	graph := Graph{Version: GraphVersion, Places: []Place{{ID: "bnd:route", Kind: PlaceBoundary, Path: "api.go", LineNo: 10, Column: 4, TargetIDs: []string{"a", "b"},
		Boundary: &BoundaryFacts{Source: "fact", Origins: []BoundaryOrigin{second, first, second}, Direction: DirectionIn, GivenKind: BoundaryHTTPServer, Method: "ANY", Values: []string{"/v1/update"}}}}, Edges: []Edge{}, Seeds: []string{}}
	got, err := EncodeGraph(graph)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Places[0].Boundary.Origins) != 3 || graph.Places[0].Boundary.Origins[0] != second {
		t.Fatal("sealing mutated the caller's origins")
	}
	clean := graph
	clean.Places = append([]Place(nil), graph.Places...)
	boundary := *clean.Places[0].Boundary
	boundary.Origins = []BoundaryOrigin{first, second}
	clean.Places[0].Boundary = &boundary
	want, err := EncodeGraph(clean)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("target traversal order or exact duplicate changed the sealed graph")
	}
	if _, err := DecodeGraph(got); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		origins []BoundaryOrigin
		reason  string
	}{
		{"missing", []BoundaryOrigin{first}, "lacks the native origin"},
		{"unknown target", []BoundaryOrigin{first, second, {TargetID: "foreign", FactID: "fact"}}, "invalid native origin"},
		{"conflicting fact", []BoundaryOrigin{first, second, {TargetID: "a", FactID: "other", ObjectID: "a-handler"}}, "conflicting native origins"},
		{"conflicting object", []BoundaryOrigin{first, second, {TargetID: "a", FactID: "fact", ObjectID: "other-handler"}}, "conflicting native origins"},
		{"missing fact", []BoundaryOrigin{first, {TargetID: "b"}}, "invalid native origin"},
	} {
		t.Run(test.name, func(t *testing.T) {
			boundary.Origins = test.origins
			if _, err := EncodeGraph(clean); err == nil || !strings.Contains(err.Error(), test.reason) {
				t.Fatalf("incomplete native ownership was not identified: %v", err)
			}
		})
	}
}
