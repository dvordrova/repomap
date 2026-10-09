package contracttest

import (
	"fmt"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythonprogramindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

// callGuard is what one call line of a guard example runs under: its
// callee, the guard's kind ("" for none) and the line of the construct.
type callGuard struct {
	callee string
	kind   string
	at     int
}

// assertCallGuards checks each listed line's call (or calls, folded) of the
// named callee: its guard's kind and the construct's line, in path.
func assertCallGuards(t *testing.T, index programindex.Index, path, from string, want map[int]callGuard) {
	t.Helper()
	seen := map[int]bool{}
	for _, view := range storedCallbackRelations(index, path) {
		expected, ok := want[view.line]
		if !ok || view.from != from || !slices.Contains(view.to, expected.callee) ||
			view.relation.Kind != programindex.RelationCalls && view.relation.Kind != programindex.RelationInvokesExternal {
			continue
		}
		seen[view.line] = true
		guard, got := view.relation.Guard, ""
		if guard != nil {
			got = fmt.Sprintf("%s@%d", guard.Kind, guard.Location.Line)
		}
		wanted := ""
		if expected.kind != "" {
			wanted = fmt.Sprintf("%s@%d", expected.kind, expected.at)
		}
		if got != wanted || guard != nil && guard.Location.Path != path {
			t.Errorf("%s:%d %s -> %s runs under %q, want %q", path, view.line, from, expected.callee, got, wanted)
		}
	}
	for line, expected := range want {
		if !seen[line] {
			t.Errorf("%s:%d: no call of %s from %s", path, line, expected.callee, from)
		}
	}
}

// assertCallCondition checks the actual native adapter's saved condition at
// a source site, including its branch polarity. Two calls on one line may
// have opposite arms, so both are checked independently.
func assertCallCondition(t *testing.T, index programindex.Index, path, from, callee string, line int, condition, when string) {
	t.Helper()
	for _, view := range storedCallbackRelations(index, path) {
		if view.from != from || !slices.Contains(view.to, callee) {
			continue
		}
		atSite := view.line == line
		for _, pattern := range view.relation.Patterns {
			atSite = atSite || pattern.Location != nil && pattern.Location.Path == path && pattern.Location.Line == line
		}
		for _, witness := range view.relation.Witnesses {
			atSite = atSite || witness.Location != nil && witness.Location.Path == path && witness.Location.Line == line
		}
		if !atSite {
			continue
		}
		if guard := view.relation.Guard; guard != nil && guard.Condition == condition && guard.When == when {
			return
		}
	}
	t.Errorf("%s:%d %s -> %s: no guard with condition %q, outcome %q", path, line, from, callee, condition, when)
}

// A C call's guard: oom ends in abort, so its calls and what the arm ending
// in one does first never return; sbDrainForever is declared _Noreturn but
// loops, and sbCheckOrAbort returns when its check holds, so neither call
// fails; growing is unguarded (strbuf.c's sbReserve).
func TestCFixtureACallSaysWhatItRunsUnder(t *testing.T) {
	index := buildCIndex(t, sharedCFixture(t), "c:kvd")
	noreturn, branch := programindex.GuardNoReturn, programindex.GuardBranch
	assertCallGuards(t, index, "strbuf.c", "sbReserve", map[int]callGuard{
		73: {"oom", noreturn, 73},
		75: {"sbTrace", noreturn, 74},
		76: {"oom", noreturn, 76},
		78: {"sbTrace", branch, 74},
		80: {"sbDrainForever", branch, 80},
		// sbCheckOrAbort returns when its check holds: no failing path.
		81: {"sbCheckOrAbort", "", 0},
		82: {"sbAppend", "", 0},
	})
	assertCallGuards(t, index, "strbuf.c", "sbInit", map[int]callGuard{21: {"oom", noreturn, 21}})
	assertCallCondition(t, index, "strbuf.c", "sbReserve", "sbTrace", 75, "len > SB_LIMIT", programindex.GuardWhenHolds)
	assertCallCondition(t, index, "strbuf.c", "sbReserve", "sbTrace", 78, "len > SB_LIMIT", programindex.GuardWhenFails)
	for _, row := range []struct {
		line                    int
		callee, condition, when string
	}{
		{96, "sbAvail", "ready", programindex.GuardWhenHolds},
		{98, "sbTrace", "ready", programindex.GuardWhenFails},
		{100, "sbAvail", "ready && retry", programindex.GuardWhenHolds},
		{101, "sbAvail", "ready || retry", programindex.GuardWhenFails},
		{102, "sbAvail", "ready", programindex.GuardWhenHolds},
		{102, "sbAvail", "ready", programindex.GuardWhenFails},
		{105, "sbTrace", "ready", programindex.GuardWhenMatches},
	} {
		assertCallCondition(t, index, "strbuf.c", "sbCheckedBranches", row.callee, row.line, row.condition, row.when)
	}
}

// A Python call's guard: an arm ending in a raise, what a raise raises and
// an except body fail; any other arm is a branch; a try body and the rest
// run unguarded (src/fixture_app/iteration.py's checked_store).
func TestCumulativePythonACallSaysWhatItRunsUnder(t *testing.T) {
	_, repository := materializeFixtureRepository(t, "python")
	catalog, err := pythontarget.Discover(t.Context(), repository)
	if err != nil {
		t.Fatal(err)
	}
	indexes, err := pythonprogramindex.BuildMany(t.Context(), repository, []pythontarget.Target{pythonFixtureTarget(t, catalog)})
	if err != nil {
		t.Fatal(err)
	}
	failing, branch := programindex.GuardError, programindex.GuardBranch
	assertCallGuards(t, indexes[0], "src/fixture_app/iteration.py", "checked_store", map[int]callGuard{
		116: {"report_failure", failing, 115},
		117: {"describe", failing, 117},
		119: {"report_long", branch, 118},
		123: {"report_failure", failing, 122},
		124: {"count", "", 0},
	})
	// A return before the raise: the arm does not only fail.
	assertCallGuards(t, indexes[0], "src/fixture_app/iteration.py", "checked_long", map[int]callGuard{131: {"report_long", branch, 130}})
	path := "src/fixture_app/iteration.py"
	assertCallCondition(t, indexes[0], path, "checked_store", "report_failure", 116, "error is not None", programindex.GuardWhenHolds)
	for _, row := range []struct {
		line                    int
		callee, condition, when string
	}{
		{141, "count", "ready", programindex.GuardWhenHolds},
		{143, "report_long", "ready", programindex.GuardWhenFails},
		{144, "count", "ready and retry", programindex.GuardWhenHolds},
		{145, "report_long", "ready or retry", programindex.GuardWhenFails},
		{146, "report_failure", "ready", programindex.GuardWhenHolds},
		{146, "describe", "ready", programindex.GuardWhenFails},
		{149, "count", "ready", programindex.GuardWhenMatches},
	} {
		assertCallCondition(t, indexes[0], path, "checked_branches", row.callee, row.line, row.condition, row.when)
	}
}

// A Go call's guard: the arm a non-nil error takes, what panic is handed and
// an arm ending in panic fail; any other arm is a branch; the rest runs
// unguarded (storefixture/handoff_flow.go's CheckedStore). Called from the
// cumulative Go contract with its index.
func assertGoCallGuards(t *testing.T, index programindex.Index) {
	t.Helper()
	failing, branch := programindex.GuardError, programindex.GuardBranch
	assertCallGuards(t, index, "internal/storefixture/handoff_flow.go", "CheckedStore", map[int]callGuard{
		163: {"reportStoreFailure", failing, 162},
		167: {"describeStore", failing, 167},
		170: {"reportLongKey", branch, 169},
		172: {"countStore", "", 0},
	})
	// A return before the panic: the arm does not only fail.
	assertCallGuards(t, index, "internal/storefixture/handoff_flow.go", "CheckedLongKey", map[int]callGuard{183: {"reportLongKey", branch, 182}})
	path := "internal/storefixture/handoff_flow.go"
	assertCallCondition(t, index, path, "CheckedStore", "reportStoreFailure", 163, "err != nil", programindex.GuardWhenHolds)
	assertCallCondition(t, index, path, "CheckedBranches", "countStore", 196, "ready", programindex.GuardWhenHolds)
	assertCallCondition(t, index, path, "CheckedBranches", "reportLongKey", 198, "ready", programindex.GuardWhenFails)
	assertCallCondition(t, index, path, "CheckedBranches", "describeStore", 201, "", "")
	assertCallCondition(t, index, path, "CheckedBranches", "describeStore", 204, "", "")
	assertCallCondition(t, index, path, "CheckedBranches", "reportStoreFailure", 207, "ready", programindex.GuardWhenHolds)
	assertCallCondition(t, index, path, "CheckedBranches", "reportStoreFailure", 210, "ready", programindex.GuardWhenHolds)
	assertCallCondition(t, index, path, "CheckedEither", "reportLongKey", 216, "", "")
	assertCallCondition(t, index, path, "CheckedEither", "reportLongKey", 218, "", "")
}
