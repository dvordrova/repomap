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
