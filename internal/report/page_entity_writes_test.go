package report

import (
	"encoding/json"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestEntityWritesRequireReachedCallableFieldOwnerAndWriteSite(t *testing.T) {
	b := pageBuilder{data: &ReportData{}, subjects: map[string]subjectRef{}, links: pageLinks{sourceIDs: map[string]string{"state.py": "source"}}}
	for id, kind := range map[string]programindex.ObjectKind{"input": programindex.ObjectFunction, "writer": programindex.ObjectMethod, "reader": programindex.ObjectMethod, "state": programindex.ObjectType, "field": programindex.ObjectVariable, "local": programindex.ObjectVariable} {
		owner := ""
		if id == "field" || id == "writer" || id == "reader" {
			owner = "state"
		}
		b.subjects[id] = subjectRef{subject: groupindex.Subject{ID: id, Object: &groupindex.ObjectFacts{Name: id, Kind: kind, OwnerID: owner, Location: &programindex.Location{Path: "state.py", Line: 3, Column: 1}}}}
	}
	call := groupindex.StructuralEdge{FromSubjectID: "input", ToSubjectID: "writer", Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionAlternatives}
	write := groupindex.StructuralEdge{FromSubjectID: "writer", ToSubjectID: "field", Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationWrites, Resolution: programindex.ResolutionExact, Location: &programindex.Location{Path: "state.py", Line: 17, Column: 5}}
	other := write
	other.FromSubjectID = "reader"
	read := write
	read.RelationKind = programindex.RelationReads
	local := write
	local.ToSubjectID = "local"
	old := write
	old.Location = nil
	index := groupindex.Index{StructuralEdges: []groupindex.StructuralEdge{call, write, other, read, local, old}}
	got := b.operationWrites(&index, "input", reachedSubjects("input", executionAdjacency(&index)), map[string]groupindex.StructuralEdge{"writer": call})
	if len(got) != 1 || got[0].Source.Line != 17 || got[0].Source.Open != "state.py:17:5" || !got[0].Possible || len(got[0].Steps) != 2 || got[0].EntityName != "state" {
		t.Fatalf("effects borrowed membership, reads or lost source/uncertainty: %+v", got)
	}
}

func TestSystemEntityWritesFollowMatchedInputAndPreserveOwnEvidence(t *testing.T) {
	witness, _ := json.Marshal(map[string][]pageCallStep{"post": {{Name: "click"}, {Name: "send", Possible: true}}})
	write := pageEntityWrite{EntityName: "State", Field: "position", Source: pageAnchor{Text: "state.py:17"}, Steps: []pageCallStep{{Name: "post"}, {Name: "move"}}}
	view := pageMap{Nodes: []pageMapNode{{ID: "click", Activation: "interaction", CallPaths: string(witness)}, {ID: "post", Activation: "request", Writes: []pageEntityWrite{write}}, {ID: "get", Activation: "request"}}, Edges: []pageMapEdge{{From: "ui", To: "post", Operations: "click"}, {From: "post", To: "state", Operations: "post"}, {From: "get", To: "state", Operations: "get"}}}
	completeSystemPaths(&view)
	got := view.Nodes[0].Writes
	if len(got) != 1 || !got[0].Possible || len(got[0].Steps) != 4 || !got[0].Steps[2].Integration || got[0].Source.Text != "state.py:17" {
		t.Fatalf("front-end input lost remote write evidence: %+v", got)
	}
	if view.Nodes[1].Writes[0].Possible || view.Nodes[1].Writes[0].Steps[0].Integration || len(view.Nodes[2].Writes) != 0 {
		t.Fatal("composition mutated native evidence or borrowed a sibling's writes")
	}
}
