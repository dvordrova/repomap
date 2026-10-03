package pythonprogramindex

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

// A name assigned under a branch, a loop, a try or a comprehension may hold
// any value the stores reaching a call put there, so a call through it calls
// each function they store, as alternatives, and names each store. A store
// of anything else, or a name some value no store names may be in, leaves
// the call open. The cumulative fixture's run_chosen_handler checks the plain
// branch and run_defaulted_handler a parameter; these are their other shapes
// and the exact controls.
func TestBuildCallsWhatTheStoresOfANameAssignedUnderABranchPutThere(t *testing.T) {
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


def defaulted(handler=None):
    if handler is None:
        handler = accept
    handler()


hook = flush
if json:
    hook = accept


def rebinds_hook():
    global hook
    hook = flush


def calls_hook():
    hook()


if json:
    compat = accept


def calls_compat():
    compat()


class Chosen:
    if json:
        handle = accept
    else:
        handle = flush

    def run(self):
        handle()


def aliased(flag):
    picked = flush
    if flag:
        picked = accept
    handler = flush
    if flag:
        handler = picked
    handler()


def rebound_by_call(flag):
    handler = flush
    if flag:
        handler = accept
    handler = handler()


def none_then_chosen(flag):
    handler = None
    if flag:
        handler = accept
    else:
        handler = flush
    handler()


def stored_after(flag):
    handler = flush
    handler()
    if flag:
        handler = accept


def stored_after_in_loop(items):
    handler = flush
    for item in items:
        handler()
        handler = accept


def overwritten(flag):
    handler = accept
    if flag:
        handler = flush
    handler = accept
    handler()


def counted(flag):
    handler = flush
    if flag:
        handler = accept
    handler += 1
    handler()


def chosen_class(flag):
    kind = Base
    if flag:
        kind = Other
    kind()
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
	// Each caller's one call: its targets, and the function stores it names.
	want := map[string]string{
		"declared_under_branch":  "exact accept",
		"reassigned":             "exact accept",
		"looped":                 "alternatives accept,flush 44:15 flush stored in handler|46:19 accept stored in handler under a condition",
		"declared_then_assigned": "alternatives accept,handler 51:5 handler stored in handler|54:19 accept stored in handler under a condition",
		// A nested def runs when it is called: every store may be there.
		"inner": "alternatives accept,flush 59:15 flush stored in handler|61:19 accept stored in handler under a condition",
		// One store, the name unbound otherwise: its function.
		"comprehension": "exact accept",
		// A call of an attribute names each module stored in the name, not
		// pickle.dumps.
		"codec_alias":            "unresolved 74:13 json stored in codec|76:17 pickle stored in codec under a condition",
		"assigned_then_declared": "alternatives flush,handler 81:15 flush stored in handler|83:9 handler stored in handler under a condition",
		// An if's condition and the first operand of `and` always run.
		"tested": "exact accept",
		// A parameter holds what its caller handed when the branch is skipped.
		"defaulted": "unresolved 96:19 accept stored in handler under a condition",
		// Another def's global store is no store of the module's.
		"calls_hook": "unresolved 100:8 flush stored in hook|102:12 accept stored in hook under a condition",
		// A module name no store always binds falls back to a builtin.
		"calls_compat": "unresolved 115:14 accept stored in compat under a condition",
		// A method does not see its class body's names: its call is none
		// of their stores' (Scope.owner skips the class body).
		"run": "unresolved",
		// picked holds either function where handler = picked runs; the
		// witness names the one its last store binds.
		"aliased": "unresolved 136:15 flush stored in handler|138:19 accept stored in handler under a condition",
		// handler = handler() takes effect after its own call.
		"rebound_by_call": "alternatives accept,flush 143:15 flush stored in handler|145:19 accept stored in handler under a condition",
		// None is no function: open.
		"none_then_chosen": "unresolved 152:19 accept stored in handler under a condition|154:19 flush stored in handler under a condition",
		// A store after the call reaches it only through a loop around it.
		"stored_after":         "exact flush",
		"stored_after_in_loop": "alternatives accept,flush 166:15 flush stored in handler|169:19 accept stored in handler under a condition",
		// The store no branch skips overwrites what came before it.
		"overwritten": "exact accept",
		// An augmented assignment stores what no store names.
		"counted": "unresolved 181:15 flush stored in handler|183:19 accept stored in handler under a condition",
		// Classes a branch chooses are constructed, each a target.
		"chosen_class": "alternatives Base,Other 189:12 Base stored in kind|191:16 Other stored in kind under a condition",
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
		var targets []string
		for _, id := range relation.ToIDs {
			targets = append(targets, names[id])
		}
		slices.Sort(targets)
		if len(targets) > 0 {
			got += " " + strings.Join(targets, ",")
		}
		if (relation.Resolution == programindex.ResolutionAlternatives) != (relation.Dispatch == programindex.DispatchFunctionValue) {
			t.Fatalf("%s call dispatch %q for %s", from, relation.Dispatch, relation.Resolution)
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
