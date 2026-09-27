package groupindex

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
)

// An event loop calls through a field its handlers were stored in under a
// condition: the call stays unresolved, and its witnesses name the handlers
// the stores put there. The map draws each of them as a possible arrow, the
// kind several alternatives draw, and the relation keeps its resolution. A
// store whose function the adapter could not name draws nothing. The part's
// card says what the arrow reaches: the handlers, not the pair's exact calls,
// and not no sentence at all.
func TestUnresolvedCallDrawsItsWitnessedCandidatesAsPossibleArrows(t *testing.T) {
	location := func(file string, line int) *programindex.Location {
		return &programindex.Location{Path: file, Line: line, Column: 1}
	}
	program, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{Language: "c", Kind: "executable", Name: "server", Selector: "c:server", Sources: []programindex.TargetSource{{FileRef: "loop", Path: "loop.c"}, {FileRef: "server", Path: "server.c"}}, AnchorFileRef: "server"},
		Objects: []programindex.ObjectInput{
			{SourceRef: "process", Name: "processEvents", Kind: programindex.ObjectFunction, Visibility: programindex.VisibilityPublic, Location: location("loop.c", 10)},
			{SourceRef: "read", Name: "readQueryFromClient", Kind: programindex.ObjectFunction, Visibility: programindex.VisibilityInternal, Location: location("server.c", 20)},
			{SourceRef: "reply", Name: "sendReplyToClient", Kind: programindex.ObjectFunction, Visibility: programindex.VisibilityInternal, Location: location("server.c", 40)},
			{SourceRef: "tick", Name: "tick", Kind: programindex.ObjectFunction, Visibility: programindex.VisibilityInternal, Location: location("loop.c", 30)},
		},
		Relations: []programindex.RelationInput{
			{SourceRef: "open", Kind: programindex.RelationCalls, FromRef: "process", Resolution: programindex.ResolutionUnresolved, Dispatch: programindex.DispatchFunctionValue,
				TargetsObserved: 1, WitnessesObserved: 4, Location: location("loop.c", 12), Witnesses: []programindex.Witness{
					{Kind: "c_function_value_call", Detail: "call through fileEvent.rfileProc", Location: location("loop.c", 12)},
					{Kind: "c_function_pointer_store", Detail: "readQueryFromClient stored in fileEvent.rfileProc by createFileEvent under a condition", Location: location("server.c", 50), ObjectRef: "read"},
					{Kind: "c_function_pointer_store", Detail: "sendReplyToClient stored in fileEvent.rfileProc by createFileEvent under a condition", Location: location("server.c", 60), ObjectRef: "reply"},
					{Kind: "c_function_pointer_store", Detail: "sendReplyToClient stored in fileEvent.rfileProc by createFileEvent under a condition", Location: location("server.c", 70), ObjectRef: "reply"},
				}},
			// Its stores are unknown: nothing names a function to draw.
			{SourceRef: "blind", Kind: programindex.RelationCalls, FromRef: "process", Resolution: programindex.ResolutionUnresolved, Dispatch: programindex.DispatchFunctionValue,
				TargetsObserved: 1, WitnessesObserved: 1, Location: location("loop.c", 14), Witnesses: []programindex.Witness{
					{Kind: "c_function_value_call", Detail: "call through fileEvent.finalizerProc", Location: location("loop.c", 14)},
				}},
			{SourceRef: "exact", Kind: programindex.RelationCalls, FromRef: "tick", ToRefs: []string{"reply"}, Resolution: programindex.ResolutionExact,
				TargetsObserved: 1, WitnessesObserved: 1, Location: location("loop.c", 31), Witnesses: []programindex.Witness{{Kind: "c_call", Location: location("loop.c", 31)}}},
		},
		Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: 4, RelationsObserved: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, object := range program.Objects {
		ids[object.Name] = object.ID
	}
	var open programindex.Relation
	for _, relation := range program.Relations {
		if relation.Location.Line == 12 {
			open = relation
		}
	}
	if open.Resolution != programindex.ResolutionUnresolved || len(open.ToIDs) != 0 {
		t.Fatalf("the witnessed call gained a target: %+v", open)
	}
	file := func(path string) atlas.File {
		return atlas.File{Path: path, Line: "Preset.", Source: atlas.SourceModel, Symbols: []atlas.Symbol{}}
	}
	// The reading saw only the exact call between the parts and wrote the
	// pair's sentence over it.
	exactSentence := "Event loop calls Client connection handling: sendReplyToClient."
	arrows := []atlas.Arrow{{ID: "a1", From: "loop", To: "clients", Calls: 1, Witnesses: []atlas.Witness{{Caller: "tick", Callee: "sendReplyToClient"}}, Sentence: exactSentence}}
	target := atlas.Target{ID: program.Target.ID, Name: "server", Root: ".", Zones: []atlas.Zone{}, Arrows: arrows, Boundaries: []atlas.Boundary{}, Boxes: []atlas.Box{
		{ID: "loop", Dir: ".", Title: "Event loop", Line: "Waits for sockets.", Side: atlas.SideMid, MemberIDs: []string{ids["processEvents"], ids["tick"]}, Files: []atlas.File{file("loop.c")}},
		{ID: "clients", Dir: ".", Title: "Client connection handling", Line: "Reads queries and writes replies.", Side: atlas.SideMid, MemberIDs: []string{ids["readQueryFromClient"], ids["sendReplyToClient"]}, Files: []atlas.File{file("server.c")}},
	}}
	value := atlas.Atlas{Version: atlas.Version, Targets: []atlas.Target{target}, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{}}
	indexes, err := ProjectAtlas(map[string]programindex.Index{program.Target.ID: program}, value)
	if err != nil {
		t.Fatal(err)
	}
	type drawn struct {
		from, to   string
		resolution programindex.PatternValueResolution
		summary    string
	}
	names := map[string]string{}
	for name, id := range ids {
		names[id] = name
	}
	var got []drawn
	for _, connection := range indexes[0].Connections {
		got = append(got, drawn{names[connection.FromSubjectID], names[connection.ToSubjectID], connection.SupportResolution, connection.Summary})
		if connection.SourceKind != "native_calls" || connection.FromLocation == nil || connection.FromLocation.Line == 14 {
			t.Fatalf("connection lost its call site or came from a call naming nothing: %+v", connection)
		}
	}
	// The open call's connections do not borrow the exact call's sentence:
	// they name the handlers its stores wrote, as the reading's fallback does.
	stored := "Event loop calls Client connection handling: readQueryFromClient, sendReplyToClient."
	want := []drawn{
		{"processEvents", "readQueryFromClient", programindex.PatternValuePossible, stored},
		{"processEvents", "sendReplyToClient", programindex.PatternValuePossible, stored},
		{"tick", "sendReplyToClient", programindex.PatternValueExact, exactSentence},
	}
	if len(got) != len(want) {
		t.Fatalf("connections = %+v, want %+v", got, want)
	}
	for _, expected := range want {
		found := false
		for _, connection := range got {
			found = found || connection == expected
		}
		if !found {
			t.Fatalf("connections = %+v, want %+v", got, want)
		}
	}
}

// The names open calls' stores wrote tie; they go in source order, never in
// the alphabet's: the call written first, then, at one call, the order the
// code stored them, before where the functions happen to be declared.
func TestStoredNamesTieInSourceOrder(t *testing.T) {
	location := func(file string, line int) *programindex.Location {
		return &programindex.Location{Path: file, Line: line, Column: 1}
	}
	store := func(name, ref string, line int) programindex.Witness {
		return programindex.Witness{Kind: "c_function_pointer_store", Detail: name + " stored in fileEvent.rfileProc under a condition", Location: location("server.c", line), ObjectRef: ref}
	}
	program, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{Language: "c", Kind: "executable", Name: "server", Selector: "c:server", Sources: []programindex.TargetSource{{FileRef: "loop", Path: "loop.c"}, {FileRef: "server", Path: "server.c"}}, AnchorFileRef: "server"},
		Objects: []programindex.ObjectInput{
			{SourceRef: "process", Name: "processEvents", Kind: programindex.ObjectFunction, Visibility: programindex.VisibilityPublic, Location: location("loop.c", 10)},
			{SourceRef: "zap", Name: "zapHandler", Kind: programindex.ObjectFunction, Visibility: programindex.VisibilityInternal, Location: location("server.c", 30)},
			{SourceRef: "accept", Name: "acceptHandler", Kind: programindex.ObjectFunction, Visibility: programindex.VisibilityInternal, Location: location("server.c", 10)},
			{SourceRef: "timer", Name: "timerHandler", Kind: programindex.ObjectFunction, Visibility: programindex.VisibilityInternal, Location: location("server.c", 40)},
		},
		Relations: []programindex.RelationInput{
			{SourceRef: "open", Kind: programindex.RelationCalls, FromRef: "process", Resolution: programindex.ResolutionUnresolved, Dispatch: programindex.DispatchFunctionValue,
				TargetsObserved: 1, WitnessesObserved: 3, Location: location("loop.c", 12), Witnesses: []programindex.Witness{
					{Kind: "c_function_value_call", Detail: "call through fileEvent.rfileProc", Location: location("loop.c", 12)},
					store("acceptHandler", "accept", 60), store("zapHandler", "zap", 50),
				}},
			{SourceRef: "timers", Kind: programindex.RelationCalls, FromRef: "process", Resolution: programindex.ResolutionUnresolved, Dispatch: programindex.DispatchFunctionValue,
				TargetsObserved: 1, WitnessesObserved: 2, Location: location("loop.c", 11), Witnesses: []programindex.Witness{
					{Kind: "c_function_value_call", Detail: "call through timeEvent.timeProc", Location: location("loop.c", 11)},
					store("timerHandler", "timer", 70),
				}},
		},
		Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: 4, RelationsObserved: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, object := range program.Objects {
		ids[object.Name] = object.ID
	}
	file := func(path string) atlas.File {
		return atlas.File{Path: path, Line: "Preset.", Source: atlas.SourceModel, Symbols: []atlas.Symbol{}}
	}
	target := atlas.Target{ID: program.Target.ID, Name: "server", Root: ".", Zones: []atlas.Zone{}, Arrows: []atlas.Arrow{}, Boundaries: []atlas.Boundary{}, Boxes: []atlas.Box{
		{ID: "loop", Dir: ".", Title: "Event loop", Line: "Waits for sockets.", Side: atlas.SideMid, MemberIDs: []string{ids["processEvents"]}, Files: []atlas.File{file("loop.c")}},
		{ID: "clients", Dir: ".", Title: "Clients", Line: "Serves clients.", Side: atlas.SideMid, MemberIDs: []string{ids["zapHandler"], ids["acceptHandler"], ids["timerHandler"]}, Files: []atlas.File{file("server.c")}},
	}}
	indexes, err := ProjectAtlas(map[string]programindex.Index{program.Target.ID: program}, atlas.Atlas{Version: atlas.Version, Targets: []atlas.Target{target}, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, connection := range indexes[0].Connections {
		if want := "Event loop calls Clients: timerHandler, zapHandler, acceptHandler."; connection.Summary != want {
			t.Fatalf("summary = %q, want %q", connection.Summary, want)
		}
	}
	if len(indexes[0].Connections) != 3 {
		t.Fatalf("connections = %+v", indexes[0].Connections)
	}
}
