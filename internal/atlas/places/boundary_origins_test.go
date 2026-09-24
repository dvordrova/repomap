package places

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/facts"
)

func TestNativeBoundariesKeepTargetOriginsAndSameLineRegistrations(t *testing.T) {
	for _, source := range []string{"routes.go", "routes.py", "routes.ts"} {
		t.Run(source, func(t *testing.T) {
			owners := map[string]struct{}{"app": {}, "lib": {}, "unobserved": {}}
			b := builder{files: map[string]*fileState{source: {targets: owners}},
				dirs: map[string]*dirState{".": {files: map[string]struct{}{source: {}}, count: 1, targets: owners}}, bounds: map[boundaryKey]*boundaryState{},
				factSubjects: map[string]string{"app.app-handler": "same-handler", "lib.lib-handler": "same-handler"}}
			b.input.Facts.Targets = []facts.Target{{ID: "app"}, {ID: "lib"}}
			for _, target := range []string{"app", "lib"} {
				for i, observation := range []struct {
					method, path string
					column       int
				}{
					{"ANY", "/update", 4}, {"ANY", "/metrics", 4}, {"POST", "/update", 4}, {"ANY", "/update", 37},
				} {
					b.input.Facts.Facts = append(b.input.Facts.Facts, facts.Fact{ID: fmt.Sprintf("%s-route-%d", target, i), Kind: facts.KindRegistration,
						TargetID: target, ObjectID: target + "-handler", Method: observation.method, Path: observation.path,
						Anchor: &facts.Anchor{Path: source, Line: 10, Column: observation.column}})
				}
				b.input.Facts.Facts = append(b.input.Facts.Facts, facts.Fact{ID: target + "-listener", Kind: facts.KindRegistration,
					TargetID: target, Key: "Start", Values: []string{":8080"}, Path: ":8080", Text: "echo.Echo.Start", Anchor: &facts.Anchor{Path: source, Line: 10, Column: 4}})
				// Two different calls with the same value at one position, as
				// `s.split("/").join("/")` was anchored when a chain's calls all
				// started at its receiver: split and join stay two places.
				for _, call := range []struct{ key, text string }{{"split", "platform:javascript.String.split"}, {"join", "platform:javascript.Array.join"}} {
					b.input.Facts.Facts = append(b.input.Facts.Facts, facts.Fact{ID: target + "-" + call.key, Kind: facts.KindRegistration,
						TargetID: target, Key: call.key, Values: []string{"/"}, Path: "/", Text: call.text, Anchor: &facts.Anchor{Path: source, Line: 10, Column: 4}})
				}
			}
			b.collectBoundaries()
			if len(b.bounds) != 7 {
				t.Fatalf("paths, methods, columns, callees or listener merged: %+v", b.bounds)
			}
			graph, err := b.graph()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := atlas.EncodeGraph(graph); err != nil {
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
					if origin.ObjectID != "" && origin.ObjectID != origin.TargetID+"."+origin.TargetID+"-handler" {
						t.Fatalf("foreign declaration identity: %+v", origin)
					}
					if origin.FactID[:len(origin.TargetID)] != origin.TargetID {
						t.Fatalf("foreign fact identity: %+v", origin)
					}
				}
				if place.Boundary.Values[0] == ":8080" && (place.Boundary.Direction != atlas.DirectionOut || place.Boundary.Method != "") {
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
		factSubjects: map[string]string{"app.app-handler": "same-handler", "tool.tool-handler": "same-handler"}}
	b.input.Facts.Targets = []facts.Target{{ID: "app"}, {ID: "tool"}}
	for _, target := range []string{"app", "tool"} {
		b.input.Facts.Facts = append(b.input.Facts.Facts, facts.Fact{ID: target + "-route", Kind: facts.KindRegistration,
			TargetID: target, ObjectID: target + "-handler", Method: "POST", Path: "/update",
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
	if place.Boundary.ObjectID != "app.app-handler" {
		t.Fatalf("representative object is not the owner's: %s", place.Boundary.ObjectID)
	}
}

func TestNativeBoundaryWithoutAnObservingOwnerIsDropped(t *testing.T) {
	b := builder{files: map[string]*fileState{"routes.go": {targets: map[string]struct{}{"lib": {}}}},
		dirs: map[string]*dirState{}, bounds: map[boundaryKey]*boundaryState{},
		factSubjects: map[string]string{"tool.tool-handler": "handler"}}
	b.input.Facts.Targets = []facts.Target{{ID: "tool"}}
	b.input.Facts.Facts = append(b.input.Facts.Facts, facts.Fact{ID: "tool-route", Kind: facts.KindRegistration,
		TargetID: "tool", ObjectID: "tool-handler", Method: "GET", Path: "/health",
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

// Two facts of one target identical in every part of the boundary key mean
// an adapter gave two calls one position. The place keeps both and sealing
// refuses it; nothing pairs them with another target's calls by order.
func TestNativeBoundaryRefusesTwoIdenticalFactsOfOneTarget(t *testing.T) {
	owners := map[string]struct{}{"app": {}}
	b := builder{files: map[string]*fileState{"text.ts": {targets: owners}},
		dirs: map[string]*dirState{".": {files: map[string]struct{}{"text.ts": {}}, count: 1, targets: owners}}, bounds: map[boundaryKey]*boundaryState{}, factSubjects: map[string]string{}}
	b.input.Facts.Targets = []facts.Target{{ID: "app"}}
	for _, id := range []string{"app-split-1", "app-split-2"} {
		b.input.Facts.Facts = append(b.input.Facts.Facts, facts.Fact{ID: id, Kind: facts.KindRegistration, TargetID: "app",
			Key: "split", Values: []string{"/"}, Path: "/", Text: "platform:javascript.String.split",
			Anchor: &facts.Anchor{Path: "text.ts", Line: 3, Column: 20}})
	}
	b.collectBoundaries()
	if len(b.bounds) != 1 {
		t.Fatalf("identical observations of one target became %d boundaries", len(b.bounds))
	}
	graph, err := b.graph()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := atlas.EncodeGraph(graph); err == nil || !strings.Contains(err.Error(), "conflicting native origins") {
		t.Fatalf("two facts of one target in one place were sealed: %v", err)
	}
}
