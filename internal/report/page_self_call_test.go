package report

import (
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// A declaration calling itself is no relation between two declarations of
// its part: othello.ai/move's two-argument form calls its three-argument
// form, and its reading had said "Called by … othello.ai/move()" and
// "Calls othello.ai/move()". Its call to another member stays.
func TestADeclarationCallingItselfIsNoInternalConnection(t *testing.T) {
	part := groupindex.Group{ID: "search", Title: "AI search", MemberSubjectIDs: []string{"move", "choose"}}
	index := groupindex.Index{Groups: []groupindex.Group{part}}
	b := pageBuilder{subjects: map[string]subjectRef{}, links: pageLinks{sourceIDs: map[string]string{"src/othello/ai.cljc": "ai-file"}}}
	for _, item := range []struct{ id, name string }{{"move", "othello.ai/move"}, {"choose", "othello.ai.search/choose"}} {
		b.subjects[item.id] = subjectRef{subject: groupindex.Subject{ID: item.id, Object: &groupindex.ObjectFacts{Name: item.name, Location: &programindex.Location{Path: "src/othello/ai.cljc", Line: 4, Column: 1}}}}
	}
	call := func(id, to string, line int) groupindex.StructuralEdge {
		return groupindex.StructuralEdge{Role: groupindex.EdgeRelationTarget, RelationID: id, RelationKind: programindex.RelationCalls, FromSubjectID: "move", ToSubjectID: to,
			Resolution: programindex.ResolutionExact, Location: &programindex.Location{Path: "src/othello/ai.cljc", Line: line, Column: 4}}
	}
	index.StructuralEdges = []groupindex.StructuralEdge{call("arity", "move", 6), call("search", "choose", 8)}
	rows := b.internalGroupConnections(index, part)
	if len(rows) != 1 || rows[0].Label != "othello.ai/move calls othello.ai.search/choose" {
		t.Fatalf("internal connections = %+v, want move's call to choose alone", rows)
	}
}

func TestArityCallUsesOriginalSourceExpressionAndExistingDetail(t *testing.T) {
	const expression = "[значение\n\tstatus]"
	builder := pageBuilder{data: &ReportData{ProgramPortfolio: &ProgramPortfolio{Entries: []programindex.Index{
		{Target: programindex.Target{ID: "t1"}, Relations: []programindex.Relation{
			{ID: "multiline", Witnesses: []programindex.Witness{{Kind: "arity", Detail: "selected arity", SourceExpression: expression}}},
			{ID: "existing", Witnesses: []programindex.Witness{{Kind: "arity", Detail: "[name times]"}}},
		}},
		{Target: programindex.Target{ID: "t2"}, Relations: []programindex.Relation{{ID: "other", Witnesses: []programindex.Witness{{Kind: "arity", SourceExpression: "[other]"}}}}},
	}}}}
	calls := builder.arityCalls("t1")
	if len(calls) != 2 || calls["multiline"] != expression || calls["existing"] != "[name times]" {
		t.Fatalf("selected parameters lost or substituted by caption: %+v", calls)
	}
}
