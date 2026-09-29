package report

import (
	"fmt"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

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
