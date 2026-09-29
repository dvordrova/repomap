package places

import (
	"strings"
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

// A script holds its file and what it imports in its directory, and the
// deepest root covering a file decides it: freqtrade's build_helpers holds
// three script programs and two CI scripts no program imports, which had
// fallen to freqtrade through the root distribution it absorbed. Such a
// file is no program's, unless a shallower program's entry imports it.
func TestAFileInAScriptsDirectoryNoOneImportsIsNoProgramsFile(t *testing.T) {
	program := atlasTestIndexTarget(t, "program")
	script := atlasScriptIndexTarget(t, "build_helpers/create.py")
	rebound, err := programindex.RebindTargetSet([]programindex.Index{program, script})
	if err != nil {
		t.Fatal(err)
	}
	program, script = rebound[0], rebound[1]
	all := func() map[string]struct{} {
		return map[string]struct{}{program.Target.ID: {}, script.Target.ID: {}}
	}
	b := &builder{files: map[string]*fileState{
		"build_helpers/create.py":     {targets: all()},
		"build_helpers/shared.py":     {targets: all()},
		"build_helpers/pre_commit.py": {targets: all()},
		"build_helpers/schema.py":     {targets: all()},
		"tests/test_main.py":          {targets: all()},
	}}
	b.scriptImports = map[string]map[string]bool{script.Target.ID: {"build_helpers/create.py": true, "build_helpers/shared.py": true}}
	b.entryImports = map[string]map[string]bool{program.Target.ID: {"program/main.go": true, "build_helpers/schema.py": true}}
	b.input.Targets = []TargetInput{{Index: program, Root: "program", AbsorbedRoot: "."}, {Index: script, Root: "build_helpers", Script: true}}
	b.claimByRoot()
	for path, want := range map[string][]string{
		"build_helpers/create.py":     {script.Target.ID},
		"build_helpers/shared.py":     {script.Target.ID},
		"build_helpers/pre_commit.py": nil,
		"build_helpers/schema.py":     {program.Target.ID},
		"tests/test_main.py":          {program.Target.ID},
	} {
		var got []string
		for id := range b.files[path].targets {
			got = append(got, id)
		}
		if len(got) != len(want) || len(want) == 1 && got[0] != want[0] {
			t.Errorf("%s is held by %v, want %v", path, got, want)
		}
	}
	// With no other root covering it, a file the script does not import
	// keeps every program indexing it: nothing else claims it (a Django
	// project's manage.py beside apps it reaches only by strings).
	b.files["build_helpers/pre_commit.py"].targets = all()
	b.input.Targets[0].AbsorbedRoot = ""
	b.claimByRoot()
	if got := len(b.files["build_helpers/pre_commit.py"].targets); got != 2 {
		t.Errorf("a file only a script's root covers is held by %d programs, want 2", got)
	}
}

func atlasScriptIndexTarget(t *testing.T, file string) programindex.Index {
	t.Helper()
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: "python", Kind: "executable", Name: file, Selector: file,
			Sources: []programindex.TargetSource{{FileRef: "f1", Path: file}}, AnchorFileRef: "f1",
		},
		Objects: []programindex.ObjectInput{{
			SourceRef: "o", Kind: programindex.ObjectFunction, Name: "main", Visibility: programindex.VisibilityPublic,
			Location: &programindex.Location{Path: file, Line: 1, Column: 1},
		}},
		Relations: []programindex.RelationInput{},
		Coverage:  programindex.CoverageInput{Measured: true, ObjectsObserved: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	return index
}
