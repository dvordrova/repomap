package contracttest

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythonprogramindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

// conditionalCalleeLine is what one call line of a conditional-callee example
// holds: who calls what, how certain, through what and how invoked.
type conditionalCalleeLine struct {
	kind       programindex.RelationKind
	from       string
	to         string
	resolution programindex.Resolution
	dispatch   string
	invocation string
}

// assertConditionalCallees checks each listed line's one call relation and,
// for alternatives, the witness naming the condition's choice.
func assertConditionalCallees(t *testing.T, index programindex.Index, path, witness string, want map[int]conditionalCalleeLine) {
	t.Helper()
	got := make(map[int][]string)
	for _, view := range storedCallbackRelations(index, path) {
		relation := view.relation
		if _, ok := want[view.line]; !ok || (relation.Kind != programindex.RelationCalls && relation.Kind != programindex.RelationInvokesExternal) {
			continue
		}
		got[view.line] = append(got[view.line], fmt.Sprintf("%s %s->%s %s %s %s", relation.Kind, view.from, strings.Join(view.to, ","),
			relation.Resolution, relation.Dispatch, relation.Invocation))
		chose := slices.ContainsFunc(relation.Witnesses, func(value programindex.Witness) bool {
			return value.Kind == witness && value.Location != nil && value.Location.Path == path && value.Location.Line == view.line
		})
		if chose != (relation.Resolution == programindex.ResolutionAlternatives) {
			t.Fatalf("line %d: %s witness present=%v for a %s call: %+v", view.line, witness, chose, relation.Resolution, relation)
		}
	}
	for number, expected := range want {
		wanted := fmt.Sprintf("%s %s->%s %s %s %s", expected.kind, expected.from, expected.to, expected.resolution, expected.dispatch, expected.invocation)
		if !slices.Equal(got[number], []string{wanted}) {
			t.Fatalf("line %d: calls %q, want [%q]", number, got[number], wanted)
		}
	}
}

// A Python callee chosen by a conditional expression whose branches all name
// callables calls one of them: alternatives through a function value, the
// equivalent of C's conditional callee (src/fixture_app/stored_callbacks.py).
func TestCumulativePythonACalleeChosenByAConditionCallsOneOfItsCallables(t *testing.T) {
	_, repository := materializeFixtureRepository(t, "python")
	catalog, err := pythontarget.Discover(t.Context(), repository)
	if err != nil {
		t.Fatal(err)
	}
	indexes, err := pythonprogramindex.BuildMany(t.Context(), repository, []pythontarget.Target{pythonFixtureTarget(t, catalog)})
	if err != nil {
		t.Fatal(err)
	}
	calls, external := programindex.RelationCalls, programindex.RelationInvokesExternal
	exact, alternatives, unresolved := programindex.ResolutionExact, programindex.ResolutionAlternatives, programindex.ResolutionUnresolved
	value := programindex.DispatchFunctionValue
	assertConditionalCallees(t, indexes[0], "src/fixture_app/stored_callbacks.py", "python_conditional_callee", map[int]conditionalCalleeLine{
		130: {calls, "watch_tick", "tick_millis,tick_seconds", alternatives, value, ""},
		// A nested condition and a parenthesised name: three alternatives.
		131: {calls, "watch_tick", "tick_millis,tick_seconds,tick_tenths", alternatives, value, ""},
		// A parameter is no callable the index knows: the call stays open.
		132: {calls, "watch_tick", "", unresolved, "", ""},
		// A constant condition takes its branch; one function is the call.
		136: {calls, "watch_tick_again", "tick_seconds", exact, "", ""},
		137: {calls, "watch_tick_again", "tick_millis", exact, "", ""},
		138: {external, "watch_tick_again", "time.monotonic,time.perf_counter", alternatives, value, ""},
		139: {calls, "watch_tick_again", "EventLoop,Loop", alternatives, value, "construct"},
	})
}
