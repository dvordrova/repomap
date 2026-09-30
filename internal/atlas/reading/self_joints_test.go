package reading

import (
	"fmt"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
)

// A program's request to what it serves itself joins that input: a post to
// http://localhost/start reaches the program's own POST /start, a unix
// socket it dials possibly reaches the one it listens on (the path is known
// at run time only). The same path on another host, and a route another
// program serves, stay apart.
func TestAProgramJoinsItsRequestsToWhatItServes(t *testing.T) {
	boundary := func(id, target, kind, direction, method string, values ...string) *boundaryState {
		return &boundaryState{kind: kind, place: atlas.Place{ID: id, Kind: atlas.PlaceBoundary, TargetIDs: []string{target},
			Boundary: &atlas.BoundaryFacts{Direction: direction, Method: method, Values: values}}}
	}
	r := &reader{boundaries: map[string]*boundaryState{}}
	for _, state := range []*boundaryState{
		boundary("b1", "t1", atlas.BoundaryClientRequest, atlas.DirectionOut, "POST", "http://localhost/start"),
		boundary("b2", "t1", atlas.BoundaryClientRequest, atlas.DirectionOut, "POST", "https://api.example/start"),
		boundary("b3", "t1", atlas.BoundaryClientRequest, atlas.DirectionOut, "", "unix"),
		boundary("b4", "t1", atlas.BoundaryRequest, atlas.DirectionIn, "POST", "/start"),
		boundary("b5", "t1", atlas.BoundaryListenAddress, atlas.DirectionIn, "", "unix"),
		boundary("b6", "t2", atlas.BoundaryRequest, atlas.DirectionIn, "POST", "/start"),
	} {
		r.boundaries[state.place.ID] = state
	}
	var joined []string
	for _, joint := range r.selfJoints() {
		joined = append(joined, fmt.Sprintf("%s %s->%s %s possible=%v", joint.From.TargetID, joint.From.BoundaryID, joint.To.BoundaryID, joint.Value, joint.Possible))
	}
	if want := []string{"t1 b1->b4 POST /start possible=false", "t1 b3->b5 unix possible=true"}; !slices.Equal(joined, want) {
		t.Fatalf("self joints:\n have %q\n want %q", joined, want)
	}
}
