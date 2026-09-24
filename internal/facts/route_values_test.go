package facts

import (
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// A method that calls itself on a field of its own receiver (`n.next.walk()`)
// makes the receiver's origin that same field one level deeper. Reading it is
// recursion and ends as unknown instead of growing the field path forever.
func TestRouteValuesStopAtRecursiveReceiverFields(t *testing.T) {
	owner := sourcevalue.Anchor{Path: "list.go", Line: 3, Column: 1}
	receiver := sourcevalue.Value{Kind: "receiver", Owner: &owner}
	next := sourcevalue.Value{Kind: "field", Text: "next", Parts: []sourcevalue.Value{receiver}}
	index := programindex.Index{
		Objects: []programindex.Object{{
			ID: "walk", Kind: programindex.ObjectMethod, Name: "walk",
			Location: &programindex.Location{Path: "list.go", Line: 3, Column: 1},
		}},
		Relations: []programindex.Relation{{
			ID: "self", Kind: programindex.RelationCalls, FromID: "walk", ToIDs: []string{"walk"},
			Resolution: programindex.ResolutionExact,
			Patterns: []programindex.RelationPattern{{
				ID: "p", Form: programindex.PatternCall, Selector: "walk",
				Location:      &programindex.Location{Path: "list.go", Line: 5, Column: 2},
				ReceiverValue: &next,
			}},
		}},
	}
	target, err := newTargetContext(TargetInput{Index: index, Root: "."})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan []routeLiteral, 1)
	go func() {
		done <- newRouteValueReader(target).value(&next, []string{"path"}, routeLiteral{}, make(map[string]bool))
	}()
	select {
	case got := <-done:
		if len(got) != 0 {
			t.Fatalf("recursive field read produced %v, want unknown", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("recursive field read did not stop")
	}
}
