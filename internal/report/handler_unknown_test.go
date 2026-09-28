package report

import (
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// An input whose handler is not established (an option a call declares)
// is read where it is declared and binds to no part: no arrow into its
// caller's part, no place among that part's operations, no route of a
// handler. Its node carries the mark the reading says so by. An input with
// a handler in the same part keeps its implementation arrow.
func TestAnInputWhoseHandlerIsNotEstablishedBindsToNoPart(t *testing.T) {
	const target = "tool"
	location := func(line int) programindex.Location {
		return programindex.Location{Path: "cli.go", Line: line, Column: 5}
	}
	at := location(3)
	index := groupindex.Index{Target: programindex.Target{ID: target},
		Subjects: []groupindex.Subject{{ID: "run", Object: &groupindex.ObjectFacts{Name: "run", Location: &at}}},
		Groups:   []groupindex.Group{{ID: "g1", Title: "Command line", Lane: groupindex.LaneCore, MemberSubjectIDs: []string{"run"}, EvidenceSubjectIDs: []string{}}},
		Operations: []groupindex.Operation{
			{ID: "o1", GroupID: "g1", Name: "-v --verbose", Kind: "command", Source: "model", Location: location(7), HandlerUnknown: true},
			{ID: "o2", GroupID: "g1", SubjectID: "run", Name: "run", Kind: "command", Source: "fact", Location: location(3)},
		},
	}
	section := &pageSection{ID: "tool", programTargetID: target, ShortLabel: "Tool"}
	builder := &pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{index}, byProgram: map[string]*pageSection{target: section}, subjects: map[string]subjectRef{}}
	for _, subject := range index.Subjects {
		builder.subjects[subject.ID] = subjectRef{subject: subject}
	}
	overview := builder.overviewBuilder()
	section.Map = overview.buildMap(section)
	overview.fillSectionOperations(section)
	var option, handled pageMapNode
	for _, node := range section.Map.Nodes {
		switch node.FullTitle {
		case "-v --verbose":
			option = node
		case "run":
			handled = node
		}
	}
	if !option.HandlerUnknown || option.InputOwner != "" || option.OperationGroup != "Command line" {
		t.Fatalf("the option's node: %+v", option)
	}
	if handled.HandlerUnknown || handled.InputOwner == "" {
		t.Fatalf("the handled input lost its part: %+v", handled)
	}
	for _, edge := range section.Map.Edges {
		if edge.From == option.ID && edge.Label == "implemented in" {
			t.Fatalf("the option acquired an implementation arrow: %+v", edge)
		}
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
