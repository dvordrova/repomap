package places

import (
	"fmt"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/facts"
)

func TestNativeBoundariesKeepTargetOriginsAndSameLineRegistrations(t *testing.T) {
	for _, source := range []string{"routes.go", "routes.py", "routes.ts"} {
		t.Run(source, func(t *testing.T) {
			b := builder{files: map[string]*fileState{source: {targets: map[string]struct{}{"app": {}, "lib": {}, "unobserved": {}}}},
				dirs: map[string]*dirState{}, bounds: map[boundaryKey]*boundaryState{},
				factSubjects: map[string]string{"app-handler": "same-handler", "lib-handler": "same-handler"}}
			b.input.Facts.Targets = []facts.Target{{ID: "facts-app", ProgramTargetID: "app"}, {ID: "facts-lib", ProgramTargetID: "lib"}}
			for _, target := range []string{"app", "lib"} {
				for i, observation := range []struct {
					method, path string
					column       int
				}{
					{"ANY", "/update", 4}, {"ANY", "/metrics", 4}, {"POST", "/update", 4}, {"ANY", "/update", 37},
				} {
					b.input.Facts.Facts = append(b.input.Facts.Facts, facts.Fact{ID: fmt.Sprintf("%s-route-%d", target, i), Kind: facts.KindHTTPRoute,
						TargetID: "facts-" + target, ObjectID: target + "-handler", Method: observation.method, Path: observation.path,
						Anchor: &facts.Anchor{Path: source, Line: 10, Column: observation.column}})
				}
				b.input.Facts.Facts = append(b.input.Facts.Facts, facts.Fact{ID: target + "-listener", Kind: facts.KindListenAddress,
					TargetID: "facts-" + target, ObjectID: target + "-handler", Value: ":8080", Anchor: &facts.Anchor{Path: source, Line: 10, Column: 4}})
			}
			b.collectBoundaries()
			if len(b.bounds) != 5 {
				t.Fatalf("paths, methods, columns or listener merged: %+v", b.bounds)
			}
			graph, err := b.graph()
			if err != nil {
				t.Fatal(err)
			}
			ids := map[string]bool{}
			for _, place := range graph.Places {
				if place.Boundary == nil {
					continue
				}
				if ids[place.ID] {
					t.Fatalf("distinct native observations share an ID: %s", place.ID)
				}
				ids[place.ID] = true
				if len(place.TargetIDs) != 2 || place.TargetIDs[0] != "app" || place.TargetIDs[1] != "lib" {
					t.Fatalf("borrowed file ownership: %+v", place)
				}
				if len(place.Boundary.Origins) != 2 {
					t.Fatalf("lost target facts: %+v", place)
				}
				for _, origin := range place.Boundary.Origins {
					if origin.ObjectID != origin.TargetID+"-handler" {
						t.Fatalf("foreign declaration identity: %+v", origin)
					}
					if origin.FactID[:len(origin.TargetID)] != origin.TargetID {
						t.Fatalf("foreign fact identity: %+v", origin)
					}
				}
				if place.Boundary.Values[0] == ":8080" && (place.Boundary.GivenKind != atlas.BoundaryListenAddress || place.Boundary.Method != "") {
					t.Fatalf("listener became a request: %+v", place)
				}
			}
		})
	}
}

func TestNativeBoundariesStayWithinFileOwnership(t *testing.T) {
	// One route fact observed from the owner's view and from a tool's view
	// whose page does not hold the file. Freqtrade observed one fact from
	// seven views while one product held the file; the atlas then refused
	// the boundary outside every box of the other six targets.
	b := builder{files: map[string]*fileState{"routes.go": {targets: map[string]struct{}{"app": {}}}},
		dirs: map[string]*dirState{}, bounds: map[boundaryKey]*boundaryState{},
		factSubjects: map[string]string{"app-handler": "same-handler", "tool-handler": "same-handler"}}
	b.input.Facts.Targets = []facts.Target{{ID: "facts-app", ProgramTargetID: "app"}, {ID: "facts-tool", ProgramTargetID: "tool"}}
	for _, target := range []string{"app", "tool"} {
		b.input.Facts.Facts = append(b.input.Facts.Facts, facts.Fact{ID: target + "-route", Kind: facts.KindHTTPRoute,
			TargetID: "facts-" + target, ObjectID: target + "-handler", Method: "POST", Path: "/update",
			Anchor: &facts.Anchor{Path: "routes.go", Line: 33, Column: 4}})
	}
	b.collectBoundaries()
	if len(b.bounds) != 1 {
		t.Fatalf("one anchored observation became %d boundaries", len(b.bounds))
	}
	graph, err := b.graph()
	if err != nil {
		t.Fatal(err)
	}
	var boundaries []atlas.Place
	for _, place := range graph.Places {
		if place.Boundary != nil {
			boundaries = append(boundaries, place)
		}
	}
	if len(boundaries) != 1 {
		t.Fatalf("expected one boundary place: %+v", boundaries)
	}
	place := boundaries[0]
	if len(place.TargetIDs) != 1 || place.TargetIDs[0] != "app" {
		t.Fatalf("boundary left the targets holding its file: %+v", place.TargetIDs)
	}
	if len(place.Boundary.Origins) != 1 || place.Boundary.Origins[0].TargetID != "app" || place.Boundary.Origins[0].FactID != "app-route" {
		t.Fatalf("origins do not match the retained scopes: %+v", place.Boundary.Origins)
	}
	if place.Boundary.ObjectID != "app-handler" {
		t.Fatalf("representative object is not the owner's: %s", place.Boundary.ObjectID)
	}
}

func TestNativeBoundaryWithoutAnObservingOwnerIsDropped(t *testing.T) {
	b := builder{files: map[string]*fileState{"routes.go": {targets: map[string]struct{}{"lib": {}}}},
		dirs: map[string]*dirState{}, bounds: map[boundaryKey]*boundaryState{},
		factSubjects: map[string]string{"tool-handler": "handler"}}
	b.input.Facts.Targets = []facts.Target{{ID: "facts-tool", ProgramTargetID: "tool"}}
	b.input.Facts.Facts = append(b.input.Facts.Facts, facts.Fact{ID: "tool-route", Kind: facts.KindHTTPRoute,
		TargetID: "facts-tool", ObjectID: "tool-handler", Method: "GET", Path: "/health",
		Anchor: &facts.Anchor{Path: "routes.go", Line: 12, Column: 4}})
	b.collectBoundaries()
	graph, err := b.graph()
	if err != nil {
		t.Fatal(err)
	}
	for _, place := range graph.Places {
		if place.Boundary != nil {
			t.Fatalf("a boundary no page can hold reached the graph: %+v", place)
		}
	}
}
