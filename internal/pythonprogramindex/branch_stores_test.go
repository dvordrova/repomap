package pythonprogramindex

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

// A name assigned under a branch, a loop, a try or a comprehension may hold
// any value stored in it, so a call through it is unresolved and names each
// function stored there. The cumulative fixture's run_chosen_handler checks
// the plain branch; these are its other shapes and the exact controls.
func TestBuildKeepsNamesAssignedUnderABranchUnresolved(t *testing.T) {
	const path = "stores/runtime.py"
	repository := pythonCorpus(t, map[string]string{
		"pyproject.toml":     "[project]\nname = \"stores\"\nversion = \"1.0.0\"\n",
		"stores/__init__.py": "",
		path: `import json
import pickle


def accept():
    pass


def flush():
    pass


class Base:
    pass


class Other:
    pass


try:
    Parent = Base
except ImportError:
    Parent = Other


class Child(Parent):
    pass


if json:
    def declared_under_branch():
        handler = accept
        handler()


def reassigned():
    handler = flush
    handler = accept
    handler()


def looped(items):
    handler = flush
    for item in items:
        handler = accept
    handler()


def declared_then_assigned(flag):
    def handler():
        pass
    if flag:
        handler = accept
    handler()


def enclosing(flag):
    handler = flush
    if flag:
        handler = accept

    def inner():
        handler()
    return inner


def comprehension(items):
    [picked := accept for item in items]
    picked()


def codec_alias(flag):
    codec = json
    if flag:
        codec = pickle
    codec.dumps({})


def assigned_then_declared(flag):
    handler = flush
    if flag:
        def handler():
            pass
    handler()


def tested(flag):
    if (handler := accept) and flag:
        pass
    handler()
`,
	})
	index, err := buildOneForTest(context.Background(), repository, targetOfKind(t, repository, pythontarget.KindLibrary))
	if err != nil {
		t.Fatal(err)
	}
	if err := index.Validate(); err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, object := range index.Objects {
		names[object.ID] = object.Name
	}
	// Each caller's one call: its target when exact, or the function stores
	// it names when unresolved.
	want := map[string]string{
		"declared_under_branch":  "exact accept",
		"reassigned":             "exact accept",
		"looped":                 "unresolved 44:15 flush stored in handler|46:19 accept stored in handler under a condition",
		"declared_then_assigned": "unresolved 51:5 handler stored in handler|54:19 accept stored in handler under a condition",
		"inner":                  "unresolved 59:15 flush stored in handler|61:19 accept stored in handler under a condition",
		"comprehension":          "unresolved 69:16 accept stored in picked under a condition",
		// A call of an attribute names each module stored in the name, not
		// pickle.dumps.
		"codec_alias":            "unresolved 74:13 json stored in codec|76:17 pickle stored in codec under a condition",
		"assigned_then_declared": "unresolved 81:15 flush stored in handler|83:9 handler stored in handler under a condition",
		// An if's condition and the first operand of `and` always run.
		"tested": "exact accept",
	}
	seen := map[string]bool{}
	for _, relation := range index.Relations {
		from := names[relation.FromID]
		if relation.Kind == programindex.RelationImplements && from == "Child" {
			if relation.Resolution != programindex.ResolutionUnresolved || len(relation.ToIDs) != 0 {
				t.Fatalf("a base class assigned under a try became its last value: %#v", relation)
			}
			seen["Child"] = true
		}
		expected, ok := want[from]
		if !ok || relation.Kind != programindex.RelationCalls && relation.Kind != programindex.RelationInvokesExternal {
			continue
		}
		got := string(relation.Resolution)
		if len(relation.ToIDs) == 1 {
			got += " " + names[relation.ToIDs[0]]
		}
		var stores []string
		for _, witness := range relation.Witnesses {
			if witness.Kind == "function_value_store" && witness.Location != nil && witness.Location.Path == path {
				stores = append(stores, fmt.Sprintf("%d:%d %s", witness.Location.Line, witness.Location.Column, witness.Detail))
			}
		}
		if len(stores) > 0 {
			got += " " + strings.Join(stores, "|")
		}
		if got != expected || seen[from] {
			t.Fatalf("%s call = %q, want %q (once): %#v", from, got, expected, relation)
		}
		seen[from] = true
	}
	if len(seen) != len(want)+1 {
		t.Fatalf("checked %v, want every caller of %v and Child", seen, want)
	}
}
