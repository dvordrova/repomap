package report

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestInputPathIncludesDataReadsWithoutExecutingTheirNeighbours(t *testing.T) {
	b := pageBuilder{data: &ReportData{}, subjects: map[string]subjectRef{}, links: pageLinks{sourceIDs: map[string]string{"app.py": "source"}}}
	index := groupindex.Index{}
	for id, kind := range map[string]programindex.ObjectKind{"input": programindex.ObjectFunction, "helper": programindex.ObjectFunction, "sibling": programindex.ObjectFunction, "data": programindex.ObjectVariable, "other-data": programindex.ObjectVariable, "callback": programindex.ObjectFunction, "state": programindex.ObjectType, "field": programindex.ObjectVariable} {
		owner := ""
		if id == "field" {
			owner = "state"
		}
		subject := groupindex.Subject{ID: id, Object: &groupindex.ObjectFacts{Name: id, Kind: kind, OwnerID: owner, Location: &programindex.Location{Path: "app.py", Line: 3, Column: 1}}}
		index.Subjects = append(index.Subjects, subject)
		b.subjects[id] = subjectRef{subject: subject}
		index.Groups = append(index.Groups, groupindex.Group{ID: id + "-group", Title: id, MemberSubjectIDs: []string{id}})
	}
	edge := func(from, to string, kind programindex.RelationKind) groupindex.StructuralEdge {
		return groupindex.StructuralEdge{FromSubjectID: from, ToSubjectID: to, Role: groupindex.EdgeRelationTarget, RelationKind: kind, Resolution: programindex.ResolutionAlternatives, Location: &programindex.Location{Path: "app.py", Line: 17, Column: 9}}
	}
	index.StructuralEdges = []groupindex.StructuralEdge{
		edge("input", "helper", programindex.RelationCalls),
		edge("helper", "data", programindex.RelationReads),
		edge("sibling", "other-data", programindex.RelationReads),
		edge("helper", "callback", programindex.RelationReads),
		edge("data", "callback", programindex.RelationCalls),
		edge("input", "state", programindex.RelationCalls),
		edge("state", "other-data", programindex.RelationReads), // Class declaration metadata is not its constructor body.
		edge("callback", "field", programindex.RelationWrites),
	}
	index.Operations = []groupindex.Operation{{ID: "request", SubjectID: "input", GroupID: "input-group", Name: "GET /levels", Kind: "request", Location: programindex.Location{Path: "app.py", Line: 3, Column: 1}}}
	result := b.buildOperationMap(&pageSection{ID: "test"}, &index)
	operation := result.Nodes[0]
	var paths map[string][]pageCallStep
	if err := json.Unmarshal([]byte(operation.CallPaths), &paths); err != nil {
		t.Fatal(err)
	}
	steps := paths[mapNodeID("data-group")]
	if len(steps) != 3 || steps[0].Name != "input" || steps[1].Name != "helper" || !steps[2].Read || !steps[2].Possible || steps[2].ReadAt == nil || steps[2].ReadAt.Open != "app.py:17:9" {
		t.Fatalf("data lost its call/read explanation or source: %+v", steps)
	}
	if len(operation.Writes) != 0 {
		t.Fatal("reading data executed a callback's effects")
	}
	for _, id := range []string{"sibling", "other-data", "callback", "field"} {
		if _, found := paths[mapNodeID(id+"-group")]; found {
			t.Fatalf("read fabricated path to %s", id)
		}
	}
	found := false
	for _, e := range result.Edges {
		if e.From == mapNodeID("helper-group") && e.To == mapNodeID("data-group") && strings.Contains(e.Operations, operation.ID) && e.Label == "reads" {
			found = true
		}
	}
	if !found {
		t.Fatal("input map omitted its terminal read edge")
	}
}
