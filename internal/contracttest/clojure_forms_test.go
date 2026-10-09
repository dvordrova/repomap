package contracttest

import (
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/clojureproject"
	"github.com/dvordrova/repomap/internal/programindex"
)

// A special form (if, recur) is the language's syntax, no call, as an if
// or a loop is in Go, Python, JS/TS and C, where no call fact is ever
// made of one; clojure.core's functions (=, str, dec) stay calls of the
// platform, as a C program's libc calls are. A call from one arity of a
// definition that its argument count sends to another (greet-times'
// [name] to [name times]) is that arity's, not a recursion: its relation
// carries an `arity` witness naming the parameters called. Only Clojure
// has arities of one definition: Python, Go, JS/TS and C have none.
func TestAClojureSpecialFormIsNoCallAndAnotherArityIsNoRecursion(t *testing.T) {
	root, repository := materializeFixtureRepository(t, "clojure")
	targets, err := clojureproject.Scout(repository, "clojure")
	if err != nil || len(targets) != 1 {
		t.Fatalf("Clojure discovery: %v %v", targets, err)
	}
	result, err := sharedClojureFixture(t, root, repository, targets[0])
	if err != nil {
		t.Fatal(err)
	}
	index, err := programindex.New(result.Input)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, object := range index.Objects {
		names[object.ID] = object.Name
	}
	var calls, arities []string
	for _, relation := range index.Relations {
		if names[relation.FromID] != "example.core/greet-times" || relation.Kind == programindex.RelationReads {
			continue
		}
		for _, to := range relation.ToIDs {
			calls = append(calls, names[to])
		}
		for _, witness := range relation.Witnesses {
			if witness.Kind == "arity" {
				arities = append(arities, names[relation.ToIDs[0]]+" "+witness.SourceExpression)
			}
		}
	}
	slices.Sort(calls)
	if want := []string{"clojure.core/=", "clojure.core/dec", "clojure.core/str", "example.core/greet-times"}; !slices.Equal(calls, want) {
		t.Fatalf("greet-times calls %q, want %q", calls, want)
	}
	if want := []string{"example.core/greet-times [name times]"}; !slices.Equal(arities, want) {
		t.Fatalf("greet-times' arity calls %q, want %q", arities, want)
	}
	for _, object := range index.Objects {
		if object.Name == "clojure.core/if" || object.Name == "clojure.core/recur" || object.Name == "cljs.core/if" {
			t.Fatalf("a special form is an outside symbol: %s", object.Name)
		}
	}
}

// Metabase's update-with-change-log selects a parameter vector written across
// several lines. The expression is source, not a one-line identity or caption.
func TestCumulativeClojureMultilineArityWitnessIsOriginalSource(t *testing.T) {
	root, repository := materializeFixtureRepository(t, "clojure")
	targets, err := clojureproject.Scout(repository, "clojure")
	if err != nil || len(targets) != 1 {
		t.Fatalf("discovery: %v / %v", targets, err)
	}
	result, err := sharedClojureFixture(t, root, repository, targets[0])
	if err != nil {
		t.Fatal(err)
	}
	index, err := programindex.New(result.Input)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]programindex.Object{}
	for _, object := range index.Objects {
		names[object.ID] = object
	}
	const source = "[значение\n\tstatus]"
	selected, recursive := 0, 0
	for _, relation := range index.Relations {
		from := names[relation.FromID]
		if from.Name != "example.core/greet-multiline" || len(relation.ToIDs) != 1 || names[relation.ToIDs[0]].Name != from.Name {
			continue
		}
		sawArity := false
		for _, witness := range relation.Witnesses {
			if witness.Kind != "arity" {
				continue
			}
			sawArity = true
			selected++
			if witness.SourceExpression != source || witness.Detail != "selected arity" || witness.Location == nil || relation.Location == nil || *witness.Location != *relation.Location || witness.Location.Path != "src/example/core.clj" || witness.Location.Line != from.Location.Line+1 {
				t.Fatalf("selected arity lost original source or call coordinates: %+v", witness)
			}
		}
		if !sawArity {
			recursive++
		}
	}
	if selected != 1 || recursive != 1 {
		t.Fatalf("cross-arity vs same-arity selfcalls: %d / %d", selected, recursive)
	}
	assertProgramIndexRoundTrip(t, index)
}
