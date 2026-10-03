package groupindex

import (
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

// A callable written inline is named as a reader names it, never by the
// number its compiler gives it: by the repository function it only wraps,
// else by the function whose lines hold it, "(inline)", a method with its
// type. A helper called beside outside calls, or beside another repository
// function, names nothing; a callable no function holds keeps its name, and
// so does every other declaration.
func TestInlineCallablesAreNamedByTheirCalleeOrTheirHome(t *testing.T) {
	at := func(line int) *programindex.Location {
		return &programindex.Location{Path: "cmd/app/main.go", Line: line, Column: 2}
	}
	object := func(id, name string, kind programindex.ObjectKind, line, end int) programindex.Object {
		return programindex.Object{ID: id, Name: name, Kind: kind, Location: at(line), EndLine: end}
	}
	calls := func(from string, to ...string) []programindex.Relation {
		var result []programindex.Relation
		for _, id := range to {
			result = append(result, programindex.Relation{Kind: programindex.RelationCalls, Resolution: programindex.ResolutionExact, FromID: from, ToIDs: []string{id}})
		}
		return result
	}
	run := object("run", "Run", programindex.ObjectMethod, 10, 40)
	run.OwnerID = "command"
	program := programindex.Index{Target: programindex.Target{Language: "go"}, Objects: []programindex.Object{
		{ID: "command", Name: "ReplicateCommand", Kind: programindex.ObjectType, Location: at(5), EndLine: 8},
		run,
		literal(object("metrics", "Run$1", programindex.ObjectFunction, 20, 25)),
		object("tool", "DatabasesTool", programindex.ObjectFunction, 50, 70),
		literal(object("handler", "DatabasesTool$1", programindex.ObjectFunction, 55, 65)),
		object("list", "listDatabases", programindex.ObjectFunction, 80, 90),
		literal(object("both", "Run$2", programindex.ObjectFunction, 30, 35)),
		object("lambda", "<lambda>", programindex.ObjectLambda, 60, 60),
		literal(object("loose", "init$1", programindex.ObjectFunction, 100, 101)),
		literal(object("wrapper", "Tool$2", programindex.ObjectFunction, 66, 68)),
		{ID: "serve", Name: "ListenAndServe", Kind: programindex.ObjectFunction, External: &programindex.ExternalSymbol{PackagePath: "net/http", Name: "ListenAndServe"}},
	}}
	program.Relations = append(program.Relations, calls("metrics", "serve")...)
	program.Relations = append(program.Relations, calls("handler", "list", "serve")...)
	program.Relations = append(program.Relations, calls("both", "list", "tool")...)
	program.Relations = append(program.Relations, calls("wrapper", "list")...)
	got := said(inlineNames(program))
	// Two a function holds alike are each one of two (the test below).
	want := map[string]string{
		"metrics": "ReplicateCommand.Run (inline, 2)",
		"handler": "DatabasesTool (inline, 2)",
		"wrapper": "listDatabases",
		"both":    "ReplicateCommand.Run (inline, 2)",
		"lambda":  "DatabasesTool (inline, 2)",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("inline names = %v\nwant %v", got, want)
	}
}

// Callables one function writes alike are told apart by the word each
// one's hand-over gives it (headscale's cmd/hi commands, Name = "doctor"),
// else each said as one of how many they are (casdoor's two goroutines of
// Start, whose hand-overs give no word; two handed over under one word):
// never by a number or an ordinal of one's own. One alone keeps its name.
func TestCallablesOneFunctionWritesAlikeAreToldApart(t *testing.T) {
	at := func(path string, line int) *programindex.Location {
		return &programindex.Location{Path: path, Line: line, Column: 2}
	}
	function := func(id, name, path string, line, end int) programindex.Object {
		return programindex.Object{ID: id, Name: name, Kind: programindex.ObjectFunction, Location: at(path, line), EndLine: end}
	}
	field := func(detail string) programindex.Witness {
		return programindex.Witness{Kind: "callable_receiver_field", Detail: detail}
	}
	handed := func(from, to string, witnesses ...programindex.Witness) programindex.Relation {
		return programindex.Relation{Kind: programindex.RelationPassesCallback, Resolution: programindex.ResolutionExact, FromID: from, ToIDs: []string{to}, Witnesses: witnesses}
	}
	program := programindex.Index{Target: programindex.Target{Language: "go"}, Objects: []programindex.Object{
		function("main", "main", "cmd/hi/main.go", 20, 80),
		literal(function("doctor", "main$1", "cmd/hi/main.go", 28, 30)),
		literal(function("networks", "main$2", "cmd/hi/main.go", 46, 48)),
		function("start", "Start", "service/proxy.go", 306, 372),
		literal(function("http", "Start$1", "service/proxy.go", 321, 327)),
		literal(function("https", "Start$2", "service/proxy.go", 329, 370)),
		function("run", "Run", "run.go", 1, 40),
		literal(function("first", "Run$1", "run.go", 10, 12)),
		literal(function("second", "Run$2", "run.go", 20, 22)),
		function("serve", "Serve", "serve.go", 1, 20),
		literal(function("alone", "Serve$1", "serve.go", 5, 9)),
	}, Relations: []programindex.Relation{
		handed("main", "doctor", field(`Name = "doctor"`), field(`Help = "Check system requirements"`)),
		handed("main", "networks", field(`Name = "networks"`), field(`Help = "Prune unused Docker networks"`)),
		handed("run", "first", field(`Name = "same"`), field(`Timeout = 10`)),
		handed("run", "second", field(`Name = "same"`), field(`Timeout = 20`)),
	}}
	want := map[string]string{
		"doctor": "main (inline for doctor)", "networks": "main (inline for networks)",
		"http": "Start (inline, 2)", "https": "Start (inline, 2)",
		"first": "Run (inline, 2)", "second": "Run (inline, 2)",
		"alone": "Serve (inline)",
	}
	for name, got := range map[string]map[string]string{"names": said(inlineNames(program)), "holders": said(inlineHolders(program))} {
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s = %v, want %v", name, got, want)
		}
	}
}

// Callables are counted alike by the function whose lines hold them, never
// by that function's name: two packages' main functions, each holding one
// closure, are each "main (inline)", and two types' Start methods, each
// holding one goroutine, are each "Server.Start (inline)" (review
// 2026-10-02: one closure in cmd/a's main and one in cmd/b's were each "one
// of two anonymous functions in main"). Two in one of them are still one
// of two.
func TestCallablesOfSameNamedFunctionsAreNotCountedTogether(t *testing.T) {
	at := func(path string, line int) *programindex.Location {
		return &programindex.Location{Path: path, Line: line, Column: 2}
	}
	object := func(id, name string, kind programindex.ObjectKind, path string, line, end int) programindex.Object {
		return programindex.Object{ID: id, Name: name, Kind: kind, Location: at(path, line), EndLine: end}
	}
	startA := object("startA", "Start", programindex.ObjectMethod, "a/server.go", 10, 30)
	startA.OwnerID = "serverA"
	startB := object("startB", "Start", programindex.ObjectMethod, "b/server.go", 10, 30)
	startB.OwnerID = "serverB"
	program := programindex.Index{Target: programindex.Target{Language: "go"}, Objects: []programindex.Object{
		object("a", "main", programindex.ObjectFunction, "cmd/a/main.go", 5, 20),
		literal(object("ac", "main$1", programindex.ObjectFunction, "cmd/a/main.go", 8, 10)),
		object("b", "main", programindex.ObjectFunction, "cmd/b/main.go", 5, 20),
		literal(object("bc", "main$1", programindex.ObjectFunction, "cmd/b/main.go", 8, 10)),
		object("serverA", "Server", programindex.ObjectType, "a/server.go", 3, 6),
		object("serverB", "Server", programindex.ObjectType, "b/server.go", 3, 6),
		startA,
		literal(object("ag", "Start$1", programindex.ObjectFunction, "a/server.go", 12, 14)),
		startB,
		literal(object("bg", "Start$1", programindex.ObjectFunction, "b/server.go", 12, 14)),
		literal(object("bh", "Start$2", programindex.ObjectFunction, "b/server.go", 20, 22)),
	}}
	want := map[string]string{
		"ac": "main (inline)", "bc": "main (inline)",
		"ag": "Server.Start (inline)",
		"bg": "Server.Start (inline, 2)", "bh": "Server.Start (inline, 2)",
	}
	for name, got := range map[string]map[string]string{"names": said(inlineNames(program)), "holders": said(inlineHolders(program))} {
		t.Logf("%s: %v", name, got)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
}

// A name is no evidence that a callable was written inline: its adapter
// says so, a lambda by its kind and a Go function literal by ProgramIndex's
// Anonymous (go/ssa names it Run$1). JavaScript's `export function
// price$1()` and `price$`, which no adapter calls anonymous, keep their
// names and are no Anonymous subjects, which GroupsIndex compiles beside
// Inline for the report to read (external review, 2026-10-03: the report
// had hidden every name with "$").
func TestOnlyWhatItsAdapterCallsAnonymousIsWrittenInline(t *testing.T) {
	at := func(line int) *programindex.Location {
		return &programindex.Location{Path: "src/price.js", Line: line, Column: 1}
	}
	objects := []programindex.Object{
		{ID: "holder", Name: "active", Kind: programindex.ObjectFunction, Location: at(1), EndLine: 9},
		{ID: "numbered", Name: "price$1", Kind: programindex.ObjectFunction, Location: at(3), EndLine: 4},
		{ID: "dollar", Name: "price$", Kind: programindex.ObjectFunction, Location: at(20), EndLine: 21},
		{ID: "arrow", Name: "lambda@5:3", Kind: programindex.ObjectLambda, Location: at(5), EndLine: 5},
		{ID: "literal", Name: "active$1", Kind: programindex.ObjectFunction, Anonymous: true, Location: at(6), EndLine: 7},
	}
	program := programindex.Index{Objects: objects}
	written := map[string]bool{}
	for _, object := range objects {
		written[object.ID] = writtenInline(object)
	}
	if want := map[string]bool{"holder": false, "numbered": false, "dollar": false, "arrow": true, "literal": true}; !reflect.DeepEqual(written, want) {
		t.Fatalf("written inline %v, want %v", written, want)
	}
	names := said(inlineNames(program))
	if _, named := names["numbered"]; named {
		t.Fatalf("price$1 is renamed %q", names["numbered"])
	}
	if names["literal"] != "active (inline, 2)" || names["arrow"] != "active (inline, 2)" {
		t.Fatalf("the callables written inline in active read %v", names)
	}
	// Compiled into GroupsIndex beside Inline, never persisted.
	retained := map[string]struct{}{"numbered": {}, "arrow": {}, "dollar": {}, "literal": {}}
	anonymous := map[string]bool{}
	for _, subject := range compileRetainedSubjects(program, retained) {
		anonymous[subject.ID] = subject.Object.Anonymous
	}
	if want := map[string]bool{"numbered": false, "arrow": true, "dollar": false, "literal": true}; !reflect.DeepEqual(anonymous, want) {
		t.Fatalf("Anonymous %v, want %v", anonymous, want)
	}
}

// literal is a function its Go adapter calls anonymous (Run$1).
func literal(object programindex.Object) programindex.Object {
	object.Anonymous = true
	return object
}

// said is each inline name as analysis writes it into saved text.
func said(names map[string]InlineName) map[string]string {
	result := make(map[string]string, len(names))
	for id, name := range names {
		result[id] = name.String()
	}
	return result
}

// The fields say the name; their text is what saved labels and entry names
// carry, as before.
func TestAnInlineNameIsItsFields(t *testing.T) {
	for _, check := range []struct {
		name InlineName
		want string
	}{
		{InlineName{Wraps: "listDatabases"}, "listDatabases"},
		{InlineName{In: "ReplicateCommand.Run"}, "ReplicateCommand.Run (inline)"},
		{InlineName{In: "main", For: "doctor"}, "main (inline for doctor)"},
		{InlineName{In: "Start", Of: 2}, "Start (inline, 2)"},
		{InlineName{}, ""},
	} {
		if got := check.name.String(); got != check.want || check.name.IsZero() != (check.want == "") {
			t.Fatalf("%+v reads %q, want %q", check.name, got, check.want)
		}
	}
}

// Callables one function writes alike, no hand-over word telling them
// apart, are told apart by the first thing only each uses, in its source
// order, a repository declaration before an outside one: etcd's startPeer
// goroutines read recvc and propc (and the first calls String), casdoor's
// Start goroutines call http.ListenAndServe and Config.GetCertificate.
// Two using only what the other uses stay one of how many.
func TestCallablesWrittenAlikeReadApartByWhatOnlyEachUses(t *testing.T) {
	at := func(path string, line, column int) *programindex.Location {
		return &programindex.Location{Path: path, Line: line, Column: column}
	}
	function := func(id, name, path string, line, end int) programindex.Object {
		return programindex.Object{ID: id, Name: name, Kind: programindex.ObjectFunction, Location: at(path, line, 2), EndLine: end}
	}
	external := func(id, receiver, name string) programindex.Object {
		return programindex.Object{ID: id, Name: "go." + receiver + "." + name, Kind: programindex.ObjectExternalSymbol,
			External: &programindex.ExternalSymbol{PackagePath: "net/http", Receiver: receiver, Name: name}}
	}
	relation := func(kind programindex.RelationKind, from, to, path string, line, column int) programindex.Relation {
		return programindex.Relation{Kind: kind, Resolution: programindex.ResolutionExact, FromID: from, ToIDs: []string{to}, Location: at(path, line, column)}
	}
	const peer, proxy, same = "peer.go", "proxy.go", "same.go"
	program := programindex.Index{Target: programindex.Target{Language: "go"}, Objects: []programindex.Object{
		function("startPeer", "startPeer", peer, 131, 210),
		literal(function("p1", "startPeer$1", peer, 135, 140)),
		literal(function("p2", "startPeer$2", peer, 174, 188)),
		literal(function("p3", "startPeer$3", peer, 192, 206)),
		{ID: "recvc", Name: "recvc", Kind: programindex.ObjectVariable, Location: at(peer, 20, 2)},
		{ID: "propc", Name: "propc", Kind: programindex.ObjectVariable, Location: at(peer, 21, 2)},
		{ID: "logger", Name: "Logger", Kind: programindex.ObjectVariable, Location: at(peer, 22, 2)},
		function("process", "Process", peer, 300, 310),
		function("string", "String", peer, 320, 322),
		function("start", "Start", proxy, 306, 372),
		literal(function("http", "Start$1", proxy, 321, 327)),
		literal(function("https", "Start$2", proxy, 329, 370)),
		external("printf", "", "Printf"),
		external("listen", "", "ListenAndServe"),
		external("certificate", "*Config", "GetCertificate"),
		function("run", "Run", same, 1, 40),
		literal(function("first", "Run$1", same, 10, 12)),
		literal(function("second", "Run$2", same, 20, 22)),
	}, Relations: []programindex.Relation{
		relation(programindex.RelationReads, "p1", "logger", peer, 136, 8),
		relation(programindex.RelationCalls, "p1", "string", peer, 137, 83),
		relation(programindex.RelationReads, "p2", "recvc", peer, 177, 19),
		relation(programindex.RelationCalls, "p2", "process", peer, 178, 24),
		relation(programindex.RelationReads, "p2", "logger", peer, 179, 11),
		relation(programindex.RelationReads, "p3", "propc", peer, 195, 19),
		relation(programindex.RelationCalls, "p3", "process", peer, 196, 24),
		relation(programindex.RelationInvokesExternal, "http", "printf", proxy, 322, 3),
		relation(programindex.RelationInvokesExternal, "http", "listen", proxy, 323, 3),
		relation(programindex.RelationInvokesExternal, "https", "printf", proxy, 330, 3),
		relation(programindex.RelationInvokesExternal, "https", "certificate", proxy, 331, 3),
		relation(programindex.RelationCalls, "first", "process", same, 11, 3),
		relation(programindex.RelationCalls, "second", "process", same, 21, 3),
		// Each also calls outside code, so none only wraps a callee.
		relation(programindex.RelationInvokesExternal, "p1", "printf", peer, 137, 17),
		relation(programindex.RelationInvokesExternal, "p2", "printf", peer, 180, 20),
		relation(programindex.RelationInvokesExternal, "p3", "printf", peer, 198, 20),
		relation(programindex.RelationInvokesExternal, "first", "printf", same, 11, 20),
		relation(programindex.RelationInvokesExternal, "second", "printf", same, 21, 20),
	}}
	want := map[string]string{
		"p1": "startPeer (inline calling String)", "p2": "startPeer (inline reading recvc)", "p3": "startPeer (inline reading propc)",
		"http": "Start (inline calling http.ListenAndServe)", "https": "Start (inline calling Config.GetCertificate)",
		"first": "Run (inline, 2)", "second": "Run (inline, 2)",
	}
	if got := said(inlineNames(program)); !reflect.DeepEqual(got, want) {
		t.Fatalf("names = %v, want %v", got, want)
	}
}
