package places

import (
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

// freqtrade absorbed its root distribution: the program claims "." beside
// freqtrade/, so tests/ is its own, while build_helpers/ stays the tool's
// and the guard rooted with the program keeps sharing freqtrade/. A target
// that absorbed nothing claims no second root: in the 2026-09-29 run every
// tool took tests/ too, an empty absorbed root read as the repository root.
func TestAnAbsorbedRootIsASecondShallowerRootOfItsProgramOnly(t *testing.T) {
	program := atlasTestIndexTarget(t, "program")
	tool := atlasTestIndexTarget(t, "tool")
	guard := atlasTestIndexTarget(t, "guard")
	rebound, err := programindex.RebindTargetSet([]programindex.Index{program, tool, guard})
	if err != nil {
		t.Fatal(err)
	}
	program, tool, guard = rebound[0], rebound[1], rebound[2]
	all := func() map[string]struct{} {
		return map[string]struct{}{program.Target.ID: {}, tool.Target.ID: {}, guard.Target.ID: {}}
	}
	b := &builder{files: map[string]*fileState{
		"freqtrade/main.py":       {targets: all()},
		"build_helpers/schema.py": {targets: all()},
		"tests/test_main.py":      {targets: all()},
	}}
	b.input.Targets = []TargetInput{{Index: program, Root: "freqtrade", AbsorbedRoot: "."}, {Index: tool, Root: "build_helpers"}, {Index: guard, Root: "freqtrade"}}
	b.claimByRoot()
	holders := func(path string) []string {
		var ids []string
		for _, target := range []programindex.Index{program, tool, guard} {
			if _, ok := b.files[path].targets[target.Target.ID]; ok {
				ids = append(ids, target.Target.Name)
			}
		}
		return ids
	}
	for path, want := range map[string]string{"freqtrade/main.py": "program guard", "build_helpers/schema.py": "tool", "tests/test_main.py": "program"} {
		got := ""
		for i, name := range holders(path) {
			if i > 0 {
				got += " "
			}
			got += name
		}
		if got != want {
			t.Fatalf("%s is held by %q, want %q", path, got, want)
		}
	}
}
