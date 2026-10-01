package report

import (
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// An input whose handler is not established (an option a call declares)
// is read where it is declared and is handled by no part: no implementation
// arrow, no place among its part's operations, no route of a handler. Its
// node carries the mark the reading says so by. Its arrow goes into the part
// taking it in: the option's into its declaring function's part ("declared
// in"), a table row's into each part a reader of the table stands in
// ("looked up in"), never the table's own. An input with a handler in the
// same part keeps its implementation arrow.
func TestAnInputWhoseHandlerIsNotEstablishedIsTakenInWhereItsCodeReadsIt(t *testing.T) {
	const target = "tool"
	location := func(line int) programindex.Location {
		return programindex.Location{Path: "cli.go", Line: line, Column: 5}
	}
	at, parseAt, tableAt, lookupAt := location(3), location(6), location(20), location(30)
	index := groupindex.Index{Target: programindex.Target{ID: target},
		Subjects: []groupindex.Subject{
			{ID: "run", Object: &groupindex.ObjectFacts{Name: "run", Kind: programindex.ObjectFunction, Location: &at}},
			{ID: "parse", Object: &groupindex.ObjectFacts{Name: "parse", Kind: programindex.ObjectFunction, Location: &parseAt}},
			{ID: "table", Object: &groupindex.ObjectFacts{Name: "table", Kind: programindex.ObjectVariable, Location: &tableAt}},
			{ID: "lookup", Object: &groupindex.ObjectFacts{Name: "lookup", Kind: programindex.ObjectFunction, Location: &lookupAt}},
		},
		Groups: []groupindex.Group{
			{ID: "g1", Title: "Command line", Lane: groupindex.LaneCore, MemberSubjectIDs: []string{"run", "parse", "table"}, EvidenceSubjectIDs: []string{}},
			{ID: "g2", Title: "Lookup", Lane: groupindex.LaneCore, MemberSubjectIDs: []string{"lookup"}, EvidenceSubjectIDs: []string{}},
		},
		Operations: []groupindex.Operation{
			{ID: "o1", GroupID: "g1", Name: "-v --verbose", Kind: "command", Source: "model", Location: location(7), HandlerUnknown: true, DeclaredBy: "parse",
				Aliases: []groupindex.OperationAlias{{Name: "--loud", Location: location(9)}, {Name: "--loud", Location: location(10)}, {Name: "-V", Location: location(11)}}},
			{ID: "o2", GroupID: "g1", SubjectID: "run", Name: "run", Kind: "command", Source: "fact", Location: location(3)},
			{ID: "o3", GroupID: "g1", Name: "get", Kind: "request", Source: "model", Location: location(21), HandlerUnknown: true, DeclaredBy: "table"},
		},
		// lookup reads the table its rows are declared in.
		StructuralEdges: []groupindex.StructuralEdge{{FromSubjectID: "lookup", ToSubjectID: "table", Role: groupindex.EdgeRelationTarget,
			RelationKind: programindex.RelationReads, Resolution: programindex.ResolutionExact, Location: &lookupAt}},
	}
	section := &pageSection{ID: "tool", programTargetID: target, ShortLabel: "Tool"}
	builder := &pageBuilder{data: &ReportData{}, indexes: analyzedIndexes(index), byProgram: map[string]*pageSection{target: section}, subjects: map[string]subjectRef{}}
	for _, subject := range index.Subjects {
		builder.subjects[subject.ID] = subjectRef{subject: subject}
	}
	overview := builder.overviewBuilder()
	section.Map = overview.buildMap(section)
	overview.fillSectionOperations(section)
	var option, handled, row pageMapNode
	parts := map[string]string{}
	for _, node := range section.Map.Nodes {
		switch node.FullTitle {
		case "-v --verbose":
			option = node
		case "run":
			handled = node
		case "get":
			row = node
		case "Command line", "Lookup":
			parts[node.ID] = node.FullTitle
		}
	}
	if !option.HandlerUnknown || option.InputOwner != "" || option.OperationGroup != "Command line" {
		t.Fatalf("the option's node: %+v", option)
	}
	// Its other spellings of one value are read with it, each once, in
	// source order ("also written --loud, -V").
	if option.Spellings != "--loud, -V" {
		t.Fatalf("the option's other spellings: %q", option.Spellings)
	}
	if handled.HandlerUnknown || handled.InputOwner == "" {
		t.Fatalf("the handled input lost its part: %+v", handled)
	}
	taken := map[string][]string{}
	for _, edge := range section.Map.Edges {
		if edge.From != option.ID && edge.From != row.ID {
			continue
		}
		if edge.Label == "implemented in" {
			t.Fatalf("an input whose handler is not established acquired an implementation arrow: %+v", edge)
		}
		name := ""
		for _, call := range edge.Calls {
			name = call.Name
		}
		taken[edge.From] = append(taken[edge.From], edge.Label+" "+name+" → "+parts[edge.To])
	}
	if got := taken[option.ID]; len(got) != 1 || got[0] != "declared in parse → Command line" {
		t.Fatalf("the option is taken in at %v", got)
	}
	if got := taken[row.ID]; len(got) != 1 || got[0] != "looked up in lookup → Lookup" {
		t.Fatalf("the table's row is taken in at %v", got)
	}
	if row.InputOwner != "" || !row.HandlerUnknown {
		t.Fatalf("the table's row gained a handler: %+v", row)
	}
	card := builder.groupCard(section.ID, index, index.Groups[0])
	if len(card.Operations) != 1 || card.Operations[0].Name != "run" {
		t.Fatalf("the part lists %+v as its operations", card.Operations)
	}
	commands := 0
	for _, row := range section.Activities {
		if row.Kind == "command" {
			commands++
		}
	}
	if commands != 2 {
		t.Fatalf("the component's commands are %+v", section.Activities)
	}
}
