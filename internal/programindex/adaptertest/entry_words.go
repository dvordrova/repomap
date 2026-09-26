package adaptertest

import (
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
)

// AssertEntryWords checks the words an incoming registration offers to name
// its entry, as written: the call word, its literals and the composed address.
// A route's verb and path, a command's name and a topic arrive the same way,
// so each language's route reaches the one naming choice the reading makes.
func AssertEntryWords(t testing.TB, graph atlas.Graph, source string, line int, want ...string) {
	t.Helper()
	found := 0
	for _, place := range graph.Places {
		if place.Boundary == nil || place.Path != source || place.LineNo != line || place.Boundary.Direction != atlas.DirectionIn {
			continue
		}
		found++
		if !slices.Equal(place.Boundary.Words, want) {
			t.Fatalf("%s:%d entry words = %q, want %q", source, line, place.Boundary.Words, want)
		}
	}
	if found != 1 {
		t.Fatalf("%s:%d has %d incoming registrations, want one", source, line, found)
	}
}
