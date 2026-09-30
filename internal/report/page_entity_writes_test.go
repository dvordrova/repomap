package report

import (
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// What an input changes in the program's data (critic, 2026-09-30: redis's
// set had listed 80 field writes, its helpers' internals among them; the
// milestone review: litestream's config and metrics, freqtrade's reads). Only
// writes, of the data the report's inventory establishes: its work's writes
// of a table owner's field (Trade.amount); a field the program's file is
// written from (rdbSave reads db.dict) handed to a helper writing its type,
// as handed (dictAdd); a database call a writing statement is written at;
// an outside system the reach sends to, named once. Never a config or
// counter field (Server.dirty, Client.argc), a helper's own writes, a query
// that only reads, a GET, a constructor setting up its own object, or a
// call only possibly made.
func TestAnInputChangesOnlyTheProgramsDataItsOwnWorkWrites(t *testing.T) {
	at := func(line int) *programindex.Location {
		return &programindex.Location{Path: "server.c", Line: line, Column: 5}
	}
	b := pageBuilder{data: &ReportData{ProgramPortfolio: &ProgramPortfolio{}}, subjects: map[string]subjectRef{}, subjectAt: map[string]string{},
		links: pageLinks{sourceIDs: map[string]string{"server.c": "source"}}}
	objects := map[string]*groupindex.ObjectFacts{}
	var native []programindex.Object
	add := func(id, name string, kind programindex.ObjectKind, owner string, line int, helper bool, types ...int) {
		object := &groupindex.ObjectFacts{Name: name, Kind: kind, OwnerID: owner, Location: at(line)}
		objects[id] = object
		declared := programindex.Object{ID: id, Name: name, Kind: kind, OwnerID: owner, Location: at(line)}
		for _, typeLine := range types {
			declared.Types = append(declared.Types, *at(typeLine))
		}
		native = append(native, declared)
		subject := groupindex.Subject{ID: id, Kind: groupindex.SubjectObject, Object: object}
		if helper {
			subject.Interpretation = &groupindex.Interpretation{Helper: true}
		}
		b.subjects[subjectKey("t1", id)] = subjectRef{programTargetID: "t1", subject: subject}
		if kind == programindex.ObjectType {
			b.subjectAt[subjectLocationKey("t1", "server.c", line)] = id
		}
	}
	add("set", "setCommand", programindex.ObjectFunction, "", 10, false)
	add("generic", "setGenericCommand", programindex.ObjectFunction, "", 20, true)
	add("dictAdd", "dictAdd", programindex.ObjectFunction, "", 30, true)
	add("addReply", "addReply", programindex.ObjectFunction, "", 40, true)
	add("other", "otherCommand", programindex.ObjectFunction, "", 50, false)
	add("maybe", "maybeCommand", programindex.ObjectFunction, "", 60, false)
	add("Server", "Server", programindex.ObjectType, "", 100, false)
	add("dirty", "dirty", programindex.ObjectVariable, "Server", 101, false)
	add("Db", "Db", programindex.ObjectType, "", 110, false)
	add("dict", "dict", programindex.ObjectVariable, "Db", 111, false, 120)
	add("Dict", "Dict", programindex.ObjectType, "", 120, false)
	add("used", "used", programindex.ObjectVariable, "Dict", 121, false)
	add("Shared", "Shared", programindex.ObjectType, "", 130, false)
	add("ok", "ok", programindex.ObjectVariable, "Shared", 131, false, 140)
	add("Reply", "Reply", programindex.ObjectType, "", 140, false)
	add("file", "server.c", programindex.ObjectModule, "", 1, false)
	add("server", "server", programindex.ObjectVariable, "file", 2, false)
	add("db", "db", programindex.ObjectVariable, "Server", 102, false, 110)
	add("rdbSave", "rdbSave", programindex.ObjectFunction, "", 70, false)
	add("Client", "Client", programindex.ObjectType, "", 160, false)
	add("argc", "argc", programindex.ObjectVariable, "Client", 161, false)
	add("Trade", "Trade", programindex.ObjectType, "", 150, false)
	add("init", "__init__", programindex.ObjectMethod, "Trade", 151, false)
	add("amount", "amount", programindex.ObjectVariable, "Trade", 152, false)
	edge := func(id, from, to string, kind programindex.RelationKind, resolution programindex.Resolution, line int, path ...string) groupindex.StructuralEdge {
		return groupindex.StructuralEdge{RelationID: id, FromSubjectID: from, ToSubjectID: to, Role: groupindex.EdgeRelationTarget, RelationKind: kind, Resolution: resolution, Location: at(line), FieldPath: strings.Join(path, "")}
	}
	exact := programindex.ResolutionExact
	index := groupindex.Index{Target: programindex.Target{ID: "t1"},
		Groups: []groupindex.Group{{ID: "g1", Title: "Strings", MemberSubjectIDs: []string{"set", "generic", "init", "maybe"}}, {ID: "g2", Title: "Structures", MemberSubjectIDs: []string{"dictAdd", "addReply"}},
			{ID: "g3", Title: "Other", MemberSubjectIDs: []string{"other"}}},
		StructuralEdges: []groupindex.StructuralEdge{
			edge("c1", "set", "generic", programindex.RelationCalls, exact, 11),
			edge("c2", "generic", "dictAdd", programindex.RelationCalls, exact, 21),
			edge("w1", "generic", "dirty", programindex.RelationWrites, exact, 22, "server.dirty"),
			edge("w5", "generic", "argc", programindex.RelationWrites, exact, 26, "Client.argc"),
			edge("w6", "set", "dirty", programindex.RelationWrites, exact, 13, "server.dirty"),
			edge("c3", "generic", "addReply", programindex.RelationCalls, exact, 23),
			edge("w2", "dictAdd", "used", programindex.RelationWrites, exact, 31, "Dict.used"),
			edge("c4", "other", "dictAdd", programindex.RelationCalls, exact, 51),
			edge("c5", "generic", "Trade", programindex.RelationCalls, exact, 24),
			edge("c6", "generic", "init", programindex.RelationCalls, exact, 24),
			edge("w3", "init", "amount", programindex.RelationWrites, exact, 153),
			edge("w7", "generic", "amount", programindex.RelationWrites, exact, 27),
			edge("c7", "set", "maybe", programindex.RelationCalls, programindex.ResolutionAlternatives, 12),
			edge("w4", "maybe", "dirty", programindex.RelationWrites, exact, 61, "server.dirty"),
			edge("r1", "rdbSave", "dict", programindex.RelationReads, exact, 71, "Db.dict"),
			edge("c9", "generic", "Trade", programindex.RelationCalls, exact, 20),
		},
		Operations: []groupindex.Operation{{ID: "o1", SubjectID: "set", Kind: "request", Name: "set"}},
		Outbound: []groupindex.OutboundCall{{ID: "d1", SubjectID: "generic", Kind: "db", Destination: "Database", DataIDs: []string{"y1"}, Location: *at(25)},
			{ID: "d2", SubjectID: "generic", Kind: "db", Destination: "Database", Location: *at(28)},
			{ID: "d3", SubjectID: "generic", Kind: "sdk", Destination: "Amazon S3", Location: *at(29)},
			{ID: "d4", SubjectID: "generic", Kind: "client_request", Destination: "Status API", Method: "GET", Location: *at(30)}},
		Data: []groupindex.DataRecord{{DataRecord: atlas.DataRecord{ID: "y1", Path: "server.c", Line: 150, Data: &facts.DataObject{Kind: "table", Name: "trades"}}, OwnerSubjectID: "Trade"},
			{DataRecord: atlas.DataRecord{ID: "q1", Path: "server.c", Line: 25, Data: &facts.DataObject{Kind: "query", Name: "trades", Statement: "INSERT", SQL: "INSERT INTO trades"}}},
			{DataRecord: atlas.DataRecord{ID: "q2", Path: "server.c", Line: 28, Data: &facts.DataObject{Kind: "query", Name: "trades", Statement: "SELECT", SQL: "SELECT * FROM trades"}}},
			{DataRecord: atlas.DataRecord{ID: "w1", Path: "server.c", Line: 72, Data: &facts.DataObject{Kind: "file", Name: "{server.dbfilename}", File: &facts.DataFile{Calls: []facts.DataCall{{}}}}}, CallSubjectIDs: []string{"rdbSave"}}},
	}
	b.data.ProgramPortfolio.Entries = []programindex.Index{{Target: programindex.Target{ID: "t1"}, Objects: native, Relations: []programindex.Relation{
		{ID: "c2", Kind: programindex.RelationCalls, Patterns: []programindex.RelationPattern{{Arguments: []programindex.PatternArgument{{ObjectIDs: []string{"dict"}}}}}},
		{ID: "c3", Kind: programindex.RelationCalls, Patterns: []programindex.RelationPattern{{Arguments: []programindex.PatternArgument{{ObjectIDs: []string{"ok"}}}}}},
		{ID: "c9", Kind: programindex.RelationCalls, Invocation: programindex.InvocationConstruct},
	}}}
	for id := range objects {
		index.Subjects = append(index.Subjects, b.subjects[subjectKey("t1", id)].subject)
	}
	slices.SortFunc(index.Subjects, func(a, b groupindex.Subject) int { return strings.Compare(a.ID, b.ID) })
	groupindex.Derive(&index)
	var said []string
	changes := b.operationWrites(&index, index.Reach[0])
	if len(changes) == 0 || changes[0].Deeper {
		t.Fatalf("the row its own path creates is not first: %+v", changes)
	}
	for _, change := range changes {
		line := change.Kind + " " + change.EntityName + "." + change.Field
		if change.Kind == "creates" {
			line = "creates " + change.EntityName
		}
		if len(change.Via) > 0 {
			line += " via " + strings.Join(change.Via, ", ")
		}
		if change.Destination != "" {
			line = strings.Join(slices.DeleteFunc([]string{change.Kind, change.Destination, strings.Join(change.Tables, ",")}, func(word string) bool { return word == "" }), " ")
			if change.EntityName != "" {
				line += " of " + change.EntityName
			}
		}
		var by []string
		for _, caller := range change.Callers {
			by = append(by, caller.Name)
		}
		line += " by " + strings.Join(by, ", ")
		said = append(said, line)
	}
	want := []string{"creates Trade by setGenericCommand", "call Db.dict via dictAdd by setGenericCommand", "write Trade.amount by setGenericCommand", "db Database trades of Trade by setGenericCommand", "sends Amazon S3 by setGenericCommand"}
	if !slices.Equal(said, want) {
		t.Fatalf("changes = %q\nwant %q", said, want)
	}
}

func TestSystemEntityWritesFollowMatchedInputAndPreserveOwnEvidence(t *testing.T) {
	write := pageEntityWrite{Kind: "write", EntityName: "State", Field: "position", Source: pageAnchor{Text: "state.py:17"}, Callers: []pageCallStep{{Name: "post"}}}
	view := pageMap{Nodes: []pageMapNode{{ID: "click", Activation: "interaction"}, {ID: "post", Activation: "request", Writes: []pageEntityWrite{write}}, {ID: "get", Activation: "request"}}, Edges: []pageMapEdge{{From: "ui", To: "post", Operations: "click"}, {From: "post", To: "state", Operations: "post"}, {From: "get", To: "state", Operations: "get"}}}
	completeSystemPaths(&view)
	got := view.Nodes[0].Writes
	if len(got) != 1 || !got[0].Possible || !got[0].Integration || len(got[0].Callers) != 1 || got[0].Source.Text != "state.py:17" {
		t.Fatalf("front-end input lost remote write evidence: %+v", got)
	}
	if view.Nodes[1].Writes[0].Possible || view.Nodes[1].Writes[0].Integration || len(view.Nodes[2].Writes) != 0 {
		t.Fatal("composition mutated native evidence or borrowed a sibling's writes")
	}
}
