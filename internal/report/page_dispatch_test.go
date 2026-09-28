package report

import (
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// Redis's call reaches `c->cmd->proc`, one of 94 command functions, and
// cmdTable hands all 94 over as callbacks: the arrow card listed "call →
// xCommand" and "cmdTable passes callback xCommand" 77 times each. The page
// data marks both sets of rows with the one set they belong to, so the card
// can say each in one line; a caller that calls every member itself, and a
// set a caller hands over only in part, stay ordinary rows.
func TestDispatchRowsCarryTheSetTheyBelongTo(t *testing.T) {
	at := func(line int) *programindex.Location {
		return &programindex.Location{Path: "redis.c", Line: line, Column: 5}
	}
	edge := func(relation, from, to string, kind programindex.RelationKind, resolution programindex.Resolution, line int) groupindex.StructuralEdge {
		return groupindex.StructuralEdge{FromSubjectID: from, ToSubjectID: to, Role: groupindex.EdgeRelationTarget, RelationID: relation,
			RelationKind: kind, Resolution: resolution, Location: at(line)}
	}
	calls, callback := programindex.RelationCalls, programindex.RelationPassesCallback
	alternatives, exact := programindex.ResolutionAlternatives, programindex.ResolutionExact
	// The replay site comes first among the edges and later in the source:
	// the fold is named by the first site in source order (B2).
	index := groupindex.Index{Target: programindex.Target{ID: "server"}, StructuralEdges: []groupindex.StructuralEdge{
		edge("replay", "load", "get", calls, alternatives, 7587), edge("replay", "load", "set", calls, alternatives, 7587), edge("replay", "load", "del", calls, alternatives, 7587),
		edge("dispatch", "call", "get", calls, alternatives, 2054), edge("dispatch", "call", "set", calls, alternatives, 2054), edge("dispatch", "call", "del", calls, alternatives, 2054),
		edge("row1", "table", "get", callback, exact, 704), edge("row2", "table", "set", callback, exact, 705), edge("row3", "table", "del", callback, exact, 706),
		edge("row4", "table", "ping", callback, exact, 707),
		// Calling every member directly is its own three calls.
		edge("c1", "debug", "get", calls, exact, 9000), edge("c2", "debug", "set", calls, exact, 9001), edge("c3", "debug", "del", calls, exact, 9002),
		// Handing over two of the three is no set.
		edge("p1", "partial", "get", callback, exact, 800), edge("p2", "partial", "set", callback, exact, 801),
		// One alternative is no dispatch.
		edge("single", "one", "get", calls, alternatives, 50),
	}}
	builder := pageBuilder{indexes: []groupindex.Index{index}, subjects: map[string]subjectRef{},
		byProgram: map[string]*pageSection{"server": {ID: "server", Language: "c"}}}
	for line, name := range []string{"call", "load", "table", "debug", "partial", "one", "get", "set", "del", "ping"} {
		subject := groupindex.Subject{ID: name, Object: &groupindex.ObjectFacts{Name: name, Kind: programindex.ObjectFunction, Location: at(line + 1)}}
		builder.subjects[subjectKey("server", name)] = subjectRef{subject: subject}
		builder.indexes[0].Subjects = append(builder.indexes[0].Subjects, subject)
	}
	groupindex.Derive(&builder.indexes[0])
	row := func(relation, from, to string, kind programindex.RelationKind) pageEdgeCall {
		connection := groupindex.Connection{ID: relation + to, From: groupindex.Endpoint{TargetID: "server", GroupID: "a"}, To: groupindex.Endpoint{TargetID: "server", GroupID: "b"},
			SourceKind: "native_" + string(kind), SourceID: relation, FromSubjectID: from, ToSubjectID: to, FromLocation: at(1)}
		call := builder.connectionCall(connection)
		if call == nil {
			t.Fatalf("%s %s %s has no call", from, kind, to)
		}
		return *call
	}
	dispatch, replay := row("dispatch", "call", "get", calls), row("replay", "load", "set", calls)
	if dispatch.Fold == "" || !dispatch.One || dispatch.Of != 3 || dispatch.Same != "" {
		t.Fatalf("a call at a dispatch site is not one of its set: %+v", dispatch)
	}
	if replay.Fold != dispatch.Fold || !replay.One {
		t.Fatalf("two sites calling one of the same set are not one set: %+v %+v", dispatch, replay)
	}
	if handed := row("row1", "table", "get", callback); handed.Fold != dispatch.Fold || handed.One || handed.Of != 3 || handed.Same != "call" {
		t.Fatalf("a table handing the whole set over is not said as that set: %+v", handed)
	}
	for _, plain := range []pageEdgeCall{row("row4", "table", "ping", callback), row("c1", "debug", "get", calls), row("p1", "partial", "get", callback), row("single", "one", "get", calls)} {
		if plain.Fold != "" || plain.One || plain.Of != 0 || plain.Same != "" {
			t.Fatalf("an ordinary call was folded: %+v", plain)
		}
	}
}
