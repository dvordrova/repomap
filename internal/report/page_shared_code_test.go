package report

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestSharedJoinKeepsFileOwnershipDeclarationIdentityAndViewScope(t *testing.T) {
	object := func(id, path string, line int) programindex.Object {
		return programindex.Object{ID: id, Kind: programindex.ObjectFunction, Name: "same",
			Location: &programindex.Location{Path: path, Line: line, Column: 1}}
	}
	objects := []programindex.Object{
		object("n1", "pkg/shared.py", 10), object("n2", "tests/example.py", 20),
		{ID: "n3", Kind: programindex.ObjectFunction, Name: "no_location"},
		object("n4", "pkg/other.py", 10), object("n5", "off.py", 30),
		object("n6", "unused.py", 40), object("n7", "pkg/shared.py", 50),
	}
	entries := make([]programindex.Index, 3)
	for i := range entries {
		entries[i] = programindex.Index{Target: programindex.Target{ID: fmt.Sprintf("t%d", i+1)}, Objects: slices.Clone(objects)}
	}
	entries[0].Objects[0].Unreachable = true
	graphs := []groupindex.Index{
		{Target: entries[0].Target, Groups: []groupindex.Group{{ID: "g1", MemberSubjectIDs: []string{"n1", "n2", "n3"}}},
			OffMap: []groupindex.OffMapFile{{Path: "off.py"}}},
		{Target: entries[1].Target, OffMap: []groupindex.OffMapFile{{Path: "off.py"}}},
	}
	newBuilder := func(indexes []groupindex.Index) *pageBuilder {
		return &pageBuilder{data: &ReportData{ProgramPortfolio: &ProgramPortfolio{Entries: entries}}, indexes: indexes,
			sections: []*pageSection{{programTargetID: "t3"}, {programTargetID: "t1"}, {programTargetID: "t2"}}}
	}
	builder := newBuilder(graphs)
	join := builder.sharedJoin()
	want := &sharedCode{holders: map[string][]sharedHolder{}, keys: map[string]string{}, unreachable: map[string]bool{},
		held: map[string]map[string]bool{"t1": {"pkg/shared.py": true, "tests/example.py": true, "off.py": true}, "t2": {"off.py": true}, "t3": nil},
		into: map[string]map[string][]groupindex.StructuralEdge{}}
	// These are the reader's expected declarations, including helpers in a
	// held file and every declaration in a program without a mapped inventory.
	for _, row := range []struct {
		target string
		ids    []int
	}{{"t3", []int{0, 1, 3, 4, 5, 6}}, {"t1", []int{0, 1, 4, 6}}, {"t2", []int{4}}} {
		for _, i := range row.ids {
			key := groupindex.DeclarationKey(objects[i])
			qualified := subjectKey(row.target, objects[i].ID)
			want.keys[qualified] = key
			want.unreachable[qualified] = row.target == "t1" && i == 0
			want.holders[key] = append(want.holders[key], sharedHolder{targetID: row.target, objectID: objects[i].ID})
		}
	}
	if !reflect.DeepEqual(join, want) {
		t.Fatalf("shared identity, ownership or order changed:\ngot %#v\nwant %#v", join, want)
	}
	if builder.sharedJoin() != join {
		t.Fatal("shared join was rebuilt")
	}
	// The test-free view has different file ownership. Its join must not
	// borrow the full page's holders merely because native evidence is shared.
	withoutTests := slices.Clone(graphs)
	withoutTests[0].Groups = []groupindex.Group{{ID: "g1", MemberSubjectIDs: []string{"n1", "n3"}}}
	view := newBuilder(withoutTests)
	if view.sharedJoin().holds("t1", objects[1].Location) || !join.holds("t1", objects[1].Location) {
		t.Fatal("full and test-free views lost their independent held files")
	}
	if files := heldFiles(&groupindex.Index{Target: entries[0].Target, Groups: []groupindex.Group{{MemberSubjectIDs: []string{"n3"}}}}, entries[:1]); files == nil || len(files) != 0 {
		t.Fatalf("a mapped inventory with no located members became a no-map inventory: %v", files)
	}
	if files := heldFiles(&groupindex.Index{Target: entries[0].Target}, entries[:1]); files != nil {
		t.Fatalf("a no-map inventory no longer holds all native declarations: %v", files)
	}
}

// Programs sharing one project index (Python's) each declare every file of
// the project, but a program holds only the files its map claims: a script
// holds its own file, never the package its sibling is built from. A
// declaration only one program holds is that program's own code, and a
// caller is listed under a program only when that program holds the
// caller. freqtrade's FreqtradeBot.process had listed Worker._process_running
// under its own part and again under each of five script programs.
func TestACallerIsListedOnlyUnderTheProgramsHoldingIt(t *testing.T) {
	at := func(path string, line int) *programindex.Location {
		return &programindex.Location{Path: path, Line: line, Column: 5}
	}
	function := func(id, name, path string, line int) programindex.Object {
		return programindex.Object{ID: id, Kind: programindex.ObjectFunction, Name: name, Location: at(path, line)}
	}
	call := func(from, to string) groupindex.StructuralEdge {
		return groupindex.StructuralEdge{FromSubjectID: from, ToSubjectID: to, Role: groupindex.EdgeRelationTarget,
			RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionExact}
	}
	project := []programindex.Object{
		function("n1", "Worker._process_running", "freqtrade/worker.py", 180),
		function("n2", "FreqtradeBot.process", "freqtrade/freqtradebot.py", 220),
		function("n3", "main", "scripts/rest_client.py", 10),
		function("n4", "create_partials", "build_helpers/partials.py", 20),
		function("n5", "Arguments.get_parsed_arg", "freqtrade/commands/arguments.py", 90),
	}
	edges := []groupindex.StructuralEdge{call("n1", "n2"), call("n4", "n5"), call("n3", "n2")}
	var entries []programindex.Index
	for _, id := range []string{"t1", "t2", "t3"} {
		entries = append(entries, programindex.Index{Target: programindex.Target{ID: id}, Objects: slices.Clone(project)})
	}
	builder := &pageBuilder{data: &ReportData{ProgramPortfolio: &ProgramPortfolio{Entries: entries}}, byProgram: map[string]*pageSection{}, subjects: map[string]subjectRef{}, indexes: []groupindex.Index{
		{Target: programindex.Target{ID: "t1"}, StructuralEdges: edges, Groups: []groupindex.Group{{ID: "g1", MemberSubjectIDs: []string{"n1", "n2"}}},
			OffMap: []groupindex.OffMapFile{{Path: "freqtrade/commands/arguments.py", Reason: "left_out"}}},
		{Target: programindex.Target{ID: "t2"}, StructuralEdges: edges, Groups: []groupindex.Group{{ID: "g1", MemberSubjectIDs: []string{"n4"}}}},
		{Target: programindex.Target{ID: "t3"}, StructuralEdges: edges, Groups: []groupindex.Group{{ID: "g1", MemberSubjectIDs: []string{"n3"}}}},
	}}
	for i, name := range []string{"freqtrade", "build_helpers/partials.py", "scripts/rest_client.py"} {
		section := &pageSection{ID: fmt.Sprintf("t%d", i+1), programTargetID: fmt.Sprintf("t%d", i+1), ShortLabel: name}
		builder.sections = append(builder.sections, section)
		builder.byProgram[section.programTargetID] = section
	}
	for _, index := range entries {
		for _, object := range index.Objects {
			builder.subjects[subjectKey(index.Target.ID, object.ID)] = subjectRef{programTargetID: index.Target.ID,
				subject: groupindex.Subject{ID: object.ID, Object: &groupindex.ObjectFacts{Name: object.Name, Kind: object.Kind, Location: object.Location}}}
		}
	}
	if got := builder.callersElsewhere("t1", "n2"); len(got) != 0 {
		t.Fatalf("FreqtradeBot.process is called elsewhere by %+v, want no other program: none holds it", got)
	}
	if got := builder.callersElsewhere("t1", "n5"); len(got) != 0 {
		t.Fatalf("a declaration off freqtrade's map is called elsewhere by %+v: the script holds its file, not arguments.py", got)
	}
	join := builder.sharedJoin()
	if !join.own("t1", "n2") || !join.own("t1", "n5") || !join.own("t2", "n4") {
		t.Fatal("a declaration one program holds is not its own code")
	}
	// A declaration calling itself in another program holding it too is no
	// caller elsewhere.
	shared := &pageBuilder{data: builder.data, byProgram: builder.byProgram, sections: builder.sections, subjects: builder.subjects, indexes: []groupindex.Index{
		{Target: programindex.Target{ID: "t1"}, StructuralEdges: []groupindex.StructuralEdge{call("n2", "n2")}},
		{Target: programindex.Target{ID: "t2"}, StructuralEdges: []groupindex.StructuralEdge{call("n2", "n2"), call("n1", "n2")}},
	}}
	if got := shared.callersElsewhere("t1", "n2"); len(got) != 1 || got[0].targetID != "t2" || got[0].caller != "n1" {
		t.Fatalf("a shared declaration calling itself is called elsewhere by %+v, want t2's n1 alone", got)
	}
	if len(join.holders[join.keys[subjectKey("t1", "n2")]]) != 1 {
		t.Fatalf("FreqtradeBot.process has holders %+v, want freqtrade alone", join.holders[join.keys[subjectKey("t1", "n2")]])
	}
}

// Finding a shared declaration's callers in the other programs costs each
// program's calls into it, not all of that program's calls: beets' render
// had scanned every sharing program's every edge once per member of every
// part (quadratic in its declarations, 30 of its render's 64 s). Thirty
// thousand functions shared by four programs, each calling the next, are
// read for all their callers in well under the bound; the quadratic scan
// took minutes.
func TestCallersElsewhereScaleWithTheCallsIntoADeclaration(t *testing.T) {
	const functions, programs = 30000, 4
	objects := make([]programindex.Object, functions)
	edges := make([]groupindex.StructuralEdge, 0, functions)
	for i := range objects {
		objects[i] = programindex.Object{ID: fmt.Sprintf("n%d", i), Kind: programindex.ObjectFunction, Name: fmt.Sprintf("f%d", i),
			Location: &programindex.Location{Path: fmt.Sprintf("pkg/f%d.py", i/50), Line: i%50 + 1, Column: 1}}
		if i > 0 {
			edges = append(edges, groupindex.StructuralEdge{FromSubjectID: objects[i-1].ID, ToSubjectID: objects[i].ID, Role: groupindex.EdgeRelationTarget,
				RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionExact})
		}
	}
	builder := &pageBuilder{data: &ReportData{ProgramPortfolio: &ProgramPortfolio{}}, byProgram: map[string]*pageSection{}, subjects: map[string]subjectRef{}}
	for p := range programs {
		id := fmt.Sprintf("t%d", p+1)
		builder.data.ProgramPortfolio.Entries = append(builder.data.ProgramPortfolio.Entries, programindex.Index{Target: programindex.Target{ID: id}, Objects: objects})
		builder.indexes = append(builder.indexes, groupindex.Index{Target: programindex.Target{ID: id}, StructuralEdges: edges})
		section := &pageSection{ID: id, programTargetID: id, ShortLabel: id}
		builder.sections = append(builder.sections, section)
		builder.byProgram[id] = section
	}
	started := time.Now()
	listed := 0
	for _, object := range objects {
		listed += len(builder.callersElsewhere("t1", object.ID))
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("callers elsewhere of %d shared functions took %s", functions, elapsed)
	}
	// Every function but the first is called by its predecessor in each of
	// the three other programs.
	if listed != (functions-1)*(programs-1) {
		t.Fatalf("listed %d callers elsewhere, want %d", listed, (functions-1)*(programs-1))
	}
}
