package adaptertest

import (
	"testing"

	"github.com/dvordrova/repomap/internal/facts"
)

// AssertParameterHolders checks registrations in source whose receiver is a
// parameter of their function: each path in want is registered once, held by
// the call at that line of source, or by nothing when the line is 0. A
// parameter handed to its own function comes around to itself; following it
// must end, and the cycle adds no value of its own.
func AssertParameterHolders(t *testing.T, layer facts.Result, source string, want map[string]int) {
	t.Helper()
	seen := map[string]int{}
	for _, fact := range layer.OfKind(facts.KindRegistration) {
		line, ok := want[fact.Path]
		if !ok || fact.Anchor == nil || fact.Anchor.Path != source {
			continue
		}
		seen[fact.Path]++
		if line == 0 && fact.Holder != nil || line != 0 && (fact.Holder == nil || fact.Holder.Path != source || fact.Holder.Line != line) {
			t.Fatalf("%s %s is held by %+v, want line %d (0: none)", source, fact.Path, fact.Holder, line)
		}
	}
	for path := range want {
		if seen[path] != 1 {
			t.Fatalf("%s %s registered %d times, want once", source, path, seen[path])
		}
	}
}
