package contracttest

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/groupindex/flowtest"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/typesafe/typesafetest"
)

// kvd is read end to end, without captions as an ordinary run reads it, with
// a preset in place of the model that answers each registrar and each entry
// from its row alone. The command table's rows reach the registrar table as
// one field of their record type, and the entries they make are named by the
// word each row wrote. Nothing in the pipeline names kvd, Redis or a command
// table; the same two decisions name an HTTP route in the Echo preset. Each
// word call is asked on its own with where its arguments come from, the
// callables kvd's own event loop keeps are asked once each with while what
// they are kept, and no question is left unanswered.
func TestCFixturePresetReadingTurnsTableRowsIntoNamedRequests(t *testing.T) {
	fixture := loadCFixture(t)
	index := buildCIndex(t, fixture, "c:kvd")
	layer, err := facts.Build(facts.Input{Repository: fixture.repository, Targets: []facts.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: fixture.repository, Targets: []places.TargetInput{{Index: index, Root: "."}}, Facts: layer})
	if err != nil {
		t.Fatal(err)
	}
	preset := &kvdPreset{}
	result, err := reading.Read(context.Background(), reading.Options{
		Graph: graph, Repository: "kvd", Revision: "test", NoCaptions: true,
		Targets:  []reading.TargetMeta{{ID: index.Target.ID, Language: "c", Kind: "executable", Name: index.Target.Name, Root: "."}},
		Executor: llm.Executor{BatchConcurrency: 1, BatchController: &llm.BatchController{}},
		Provider: preset, Categorizer: preset.categorizer(), OwnerRunDir: t.TempDir(),
		ReadSource: func(path string) ([]byte, error) { return fixture.source(t, path), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	indexes, err := groupindex.ProjectAtlas(map[string]programindex.Index{index.Target.ID: index}, result.Atlas)
	if err != nil {
		t.Fatal(err)
	}
	if len(indexes) != 1 {
		t.Fatalf("indexes = %d", len(indexes))
	}
	// Every registrar the program hands a callable to was asked once, the
	// command table's field among them; fopen and the socket calls, which
	// are handed none, were not asked what a callable becomes.
	preset.mu.Lock()
	handed := slices.Clone(preset.handed)
	named := slices.Clone(preset.named)
	unnamed, entryWindows := preset.unnamed, preset.entryWindows
	preset.mu.Unlock()
	if unnamed != 0 {
		t.Fatalf("%d incoming rows with nothing to decide were sent without captions", unnamed)
	}
	// Seven entries of seven handlers share one request: a handler's calls
	// say nothing about the words that name its entry.
	if entryWindows != 1 {
		t.Fatalf("entries were named in %d requests, want one", entryWindows)
	}
	sort.Strings(handed)
	if want := []string{"kvd.h.kvCommand.proc", "pthread.h.pthread_create", "signal.h.struct sigaction", "stdlib.h.qsort"}; !slices.Equal(handed, want) {
		t.Fatalf("registrars asked what a callable becomes = %v, want %v", handed, want)
	}
	// Every call kvd's code gives words to is asked on its own what they
	// become, with where each argument comes from as the C adapter records
	// it: main's strcmp compares an element of its own argument vector, and
	// loadConfig's strcasecmp the first word of the array splitLine returns,
	// followed through the local argv. A getenv call is a setting read the
	// facts already name: not asked.
	preset.mu.Lock()
	entered := slices.Clone(preset.entered)
	kept := slices.Clone(preset.kept)
	tables := slices.Clone(preset.tables)
	preset.mu.Unlock()
	for _, call := range []string{
		`string.h.strcmp in main: 1: element "1" of parameter #2 argv of main; 2: "--symbols"`,
		`string.h.strcasecmp in loadConfig: 1: element "0" of result of calling splitLine(line, &words); 2: "port"`,
		`string.h.strcasecmp in loadConfig: 1: element "0" of result of calling splitLine(line, &words); 2: "dbfilename"`,
	} {
		if !slices.Contains(entered, call) {
			t.Fatalf("%s was not asked what its words become: %v", call, entered)
		}
	}
	for _, call := range entered {
		if strings.HasPrefix(call, "stdlib.h.getenv") {
			t.Fatalf("a setting read the facts name was asked what its words become: %v", call)
		}
	}
	// Each callable kvd's own functions keep is asked once, with while what
	// it is kept: the walk back from the keeping call stops at the program's
	// start and at every callable another registration hands over (the
	// command handlers a table row hands to proc, the reader acceptHandler
	// keeps).
	slices.Sort(kept)
	wantKept := []string{
		"acceptHandler by loopCreateFileEvent during [from the program's start at main]",
		"beforeSleep by loopSetBeforeSleep during [from the program's start at main]",
		"readQueryFromClient by loopCreateFileEvent during [while acceptHandler run (handed to loopCreateFileEvent)]",
		"sendReplyToClient by loopCreateFileEvent during [while pingCommand, bgsaveCommand, getCommand, setCommand, keysCommand, delCommand run (handed to kvd.h.kvCommand.proc), through addReply, addReplyLong while readQueryFromClient run (handed to loopCreateFileEvent), through processInputBuffer, processCommand, addReply]",
	}
	if !slices.Equal(kept, wantKept) {
		t.Fatalf("kept callables asked = %v\nwant %v", kept, wantKept)
	}
	// The table of names kvd declares, its symbols for --symbols, is asked
	// with the function reading it; answered none, it makes no input.
	if !slices.Equal(tables, []string{"symsTable read by [printSymbols]"}) {
		t.Fatalf("tables of names asked = %v", tables)
	}
	for _, row := range result.Rejected {
		if strings.HasSuffix(row.Kind, "window_rejected") || row.Kind == "window_rejected" {
			t.Fatalf("a question was left unanswered: %+v", row)
		}
	}
	// Before its entry is decided, a word a call gives an outside symbol
	// registers nothing in another question: the role split's and the
	// helper question's items list only the words of registrations.
	preset.mu.Lock()
	leaked := slices.Clone(preset.leaked)
	preset.mu.Unlock()
	if len(leaked) > 0 {
		t.Fatalf("an undecided word reached other questions: %v", leaked)
	}
	for _, row := range result.Rejected {
		if row.Stage == "atlas_api" {
			t.Fatalf("an outside symbol's question was refused: %+v", row)
		}
	}
	handlers := map[string]string{}
	for _, object := range index.Objects {
		if object.Kind == programindex.ObjectFunction && object.Location != nil && object.Location.Path == "kvd.c" {
			handlers[object.ID] = object.Name
		}
	}
	type input struct{ kind, name, handler, source string }
	var got []input
	for _, operation := range indexes[0].Operations {
		got = append(got, input{kind: operation.Kind, name: operation.Name, handler: handlers[operation.SubjectID], source: operation.Source})
		// Without captions no entry's line is written: the registration's
		// given text ("kvd.h.kvCommand.proc get in getCommand") restates
		// the row, so the entry has no line and its reading names its
		// handler.
		if operation.Summary != "" {
			t.Fatalf("%s keeps a line no model wrote: %q", operation.Name, operation.Summary)
		}
	}
	sort.Slice(got, func(i, j int) bool { return got[i].kind+got[i].name < got[j].kind+got[j].name })
	// Besides the table's commands and the thread: the connection the
	// event loop accepts, a request; --symbols, an option main compares
	// its arguments with; and the two directives of the configuration file,
	// settings. The last three have no handler established.
	want := []input{
		{kind: "continuous", name: "statsWorker", handler: "statsWorker", source: "fact"},
		{kind: "request", name: "acceptHandler", handler: "acceptHandler", source: "fact"},
		{kind: "command", name: "--symbols", source: "model"},
		{kind: "setting", name: "port", source: "model"},
		{kind: "setting", name: "dbfilename", source: "model"},
	}
	for _, row := range cCommandRows {
		want = append(want, input{kind: "request", name: row.name, handler: row.function, source: "fact"})
	}
	sort.Slice(want, func(i, j int) bool { return want[i].kind+want[i].name < want[j].kind+want[j].name })
	if !slices.Equal(got, want) {
		t.Fatalf("inputs = %+v\nwant %+v", got, want)
	}
	// Binding, listening on and accepting from the server's socket are its
	// listening side: they serve, and the connection accept takes in is no
	// outgoing request. Nothing else kvd calls talks to another system.
	var serving []string
	for _, role := range result.Atlas.API {
		if role.Publishes {
			serving = append(serving, role.Symbol)
		}
		if role.Talks != "" {
			t.Fatalf("%s talks %s", role.Symbol, role.Talks)
		}
	}
	if want := []string{"sys/socket.h.accept", "sys/socket.h.bind", "sys/socket.h.listen"}; !slices.Equal(serving, want) {
		t.Fatalf("symbols serving = %v, want %v", serving, want)
	}
	if len(indexes[0].Outbound) != 0 {
		t.Fatalf("kvd talks to another system: %+v", indexes[0].Outbound)
	}
	// Each command's entry was offered the words its row wrote and chose the
	// command's own. The thread's registration wrote only its call word: no
	// word named it, and it keeps its handler's name.
	// The event loop's calls through fe->rfileProc and fe->wfileProc stay
	// unresolved, and the handlers their stores name reach the map as the
	// possible arrows of the loop's part, as several alternatives would.
	processEvents := cObject(t, index, programindex.ObjectFunction, "loopProcessEvents", "loop.c")
	possible := map[string]bool{}
	for _, connection := range indexes[0].Connections {
		if connection.FromSubjectID != processEvents.ID || connection.SupportResolution != programindex.PatternValuePossible {
			continue
		}
		for _, relation := range index.Relations {
			if relation.ID == connection.SourceID && (relation.Resolution != programindex.ResolutionUnresolved || len(relation.ToIDs) != 0) {
				t.Fatalf("a witnessed call became resolved: %+v", relation)
			}
		}
		possible[handlers[connection.ToSubjectID]] = true
	}
	for _, handler := range []string{"acceptHandler", "readQueryFromClient", "sendReplyToClient"} {
		if !possible[handler] {
			t.Fatalf("the event loop draws no possible arrow to %s: %v", handler, possible)
		}
	}
	sort.Strings(named)
	wantNamed := []string{"pthread_create"}
	for _, row := range cCommandRows {
		wantNamed = append(wantNamed, "kvCommand "+row.name)
	}
	sort.Strings(wantNamed)
	if !slices.Equal(named, wantNamed) {
		t.Fatalf("entries asked for a name = %v, want %v", named, wantNamed)
	}
	checkKvdReach(t, index, indexes[0])
}

// checkKvdReach holds kvd's derived reach to its code. processCommand's
// cmd->proc(c) dispatches every command; its cmd->preload is one exact
// call and no dispatch. No command handler calls back into processCommand,
// and main's loop reaches it only through calls left unresolved, so no
// input reaches the site. get's reach is its handler's own work: the
// reading, accepting and replying callbacks the loop stores are handed
// over, never called, so they are not in it. What main reaches and no
// input does is the launch; what both reach is both.
func checkKvdReach(t *testing.T, program programindex.Index, index groupindex.Index) {
	t.Helper()
	flowtest.Check(t, program, index)
	names := map[string]string{}
	for _, subject := range index.Subjects {
		if subject.Object != nil {
			names[subject.ID] = subject.Object.Name
		}
	}
	var sites []groupindex.DispatchSite
	for _, site := range index.Dispatch {
		if len(site.OperationIDs) > 0 {
			sites = append(sites, site)
		}
	}
	if len(sites) != 1 || names[sites[0].FromSubjectID] != "processCommand" || sites[0].Location == nil || sites[0].Location.Path != "kvd.c" || sites[0].Location.Line != 176 ||
		len(sites[0].Alternatives) != len(cCommandRows) || len(sites[0].OperationIDs) != len(cCommandRows) || len(sites[0].ReachedFrom) != 0 {
		t.Fatalf("dispatch sites of kvd's inputs: %+v", sites)
	}
	operations := map[string]string{}
	for _, operation := range index.Operations {
		operations[operation.ID] = operation.Name
	}
	// A request dispatched there arrives from acceptHandler, the input
	// the loop runs on a new connection: it registers readQueryFromClient,
	// whose code reaches processCommand. No other code calls into it.
	site := sites[0]
	if len(site.Outer) != 1 || operations[site.Outer[0].OperationID] != "acceptHandler" || names[site.Outer[0].Registered] != "readQueryFromClient" || site.Unexplained {
		t.Fatalf("outer inputs of processCommand: %+v (unexplained %v)", site.Outer, site.Unexplained)
	}
	var route []string
	for _, edge := range site.Outer[0].Edges {
		route = append(route, names[index.StructuralEdges[edge].FromSubjectID]+"→"+names[index.StructuralEdges[edge].ToSubjectID])
	}
	if slices.Sort(route); !slices.Equal(route, []string{"processInputBuffer→processCommand", "readQueryFromClient→processInputBuffer"}) {
		t.Fatalf("acceptHandler's registered reader reaches processCommand by %v", route)
	}
	// The inputs whose handler is not established are one catalogue per
	// declaring function and kind: main's --symbols, and loadConfig's two
	// directives, called from main.
	catalogues := map[string][]string{}
	for _, catalogue := range index.Catalogues {
		key := names[catalogue.DeclaredBy] + " " + catalogue.Kind
		for _, id := range catalogue.OperationIDs {
			catalogues[key] = append(catalogues[key], operations[id])
		}
		if key == "loadConfig setting" && (len(catalogue.Calls) != 1 || names[index.StructuralEdges[catalogue.Calls[0]].FromSubjectID] != "main") {
			t.Fatalf("loadConfig's catalogue is called from %v", catalogue.Calls)
		}
	}
	if want := map[string][]string{"main command": {"--symbols"}, "loadConfig setting": {"port", "dbfilename"}}; !reflect.DeepEqual(catalogues, want) {
		t.Fatalf("catalogues = %v, want %v", catalogues, want)
	}
	// The launch walk from main finds them where they are declared.
	found := map[string][]string{}
	for _, function := range index.Launch.Functions {
		for _, id := range function.Found {
			found[names[function.SubjectID]] = append(found[names[function.SubjectID]], operations[id])
		}
	}
	if slices.Sort(found["loadConfig"]); !slices.Equal(found["loadConfig"], []string{"dbfilename", "port"}) || !slices.Contains(found["main"], "--symbols") {
		t.Fatalf("the launch finds %v", found)
	}
	for position, operation := range index.Operations {
		if operation.Name == "acceptHandler" {
			// acceptHandler hands readQueryFromClient to the loop: no
			// input handles that reader, so it is no hand-over between
			// inputs.
			if len(index.Reach[position].HandedOverBy) != 0 {
				t.Fatalf("acceptHandler is handed over: %+v", index.Reach[position])
			}
			continue
		}
		if len(index.Reach[position].HandsOver) != 0 || len(index.Reach[position].HandedOverBy) != 0 {
			t.Fatalf("%s hands over or is handed over: %+v", operation.Name, index.Reach[position])
		}
		if operation.Name != "get" {
			continue
		}
		reached := flowtest.Reached(index, index.Reach[position])
		for _, name := range []string{"getCommand", "addReplyBulk", "addReply", "sbAppend", "loopCreateFileEvent"} {
			if reached[name] == "" {
				t.Fatalf("get does not reach %s: %v", name, reached)
			}
		}
		for _, name := range []string{"readQueryFromClient", "sendReplyToClient", "acceptHandler", "processCommand"} {
			if reached[name] != "" {
				t.Fatalf("get reaches %s: %v", name, reached)
			}
		}
	}
	// The launch walks from main by the same rule: loopProcessEvents's calls
	// through fe->rfileProc and fe->wfileProc, which resolve to nothing, are
	// the calls the code cannot follow there, at their own lines.
	closed := map[string][]int{}
	for _, function := range index.Launch.Functions {
		if function.Outcome() != "closed" {
			continue
		}
		for _, position := range function.Closed {
			closed[names[function.SubjectID]] = append(closed[names[function.SubjectID]], index.Unresolved[position].Location.Line)
		}
	}
	if len(closed) != 1 || !slices.Equal(closed["loopProcessEvents"], []int{58, 59}) {
		t.Fatalf("launch functions that could not be looked inside: %v", closed)
	}
	phases := map[string]string{}
	for _, subject := range index.Subjects {
		if subject.Object != nil && subject.Object.Location != nil {
			phases[subject.Object.Name] = subject.Phase
		}
	}
	for name, want := range map[string]string{
		"main": groupindex.PhaseInit, "setupSignals": groupindex.PhaseInit, "netListen": groupindex.PhaseInit, "loopCreate": groupindex.PhaseInit,
		"loopCreateFileEvent": groupindex.PhaseBoth,
		"getCommand":          groupindex.PhaseRuntime, "addReply": groupindex.PhaseRuntime, "statsWorker": groupindex.PhaseRuntime, "reportStats": groupindex.PhaseRuntime,
	} {
		if phases[name] != want {
			t.Fatalf("%s is %q, want %q", name, phases[name], want)
		}
	}
}

// kvdPreset answers the text-model tables of a caption-less reading of kvd
// the way a reader would from each row, and fails any other request, so a
// table the reading starts to ask is noticed rather than answered by chance.
type kvdPreset struct {
	// roles are what an outside symbol talks to, by its api row, beside
	// the socket calls that serve; any other symbol is none.
	roles         map[string]map[string]string
	mu            sync.Mutex
	handed, named []string
	// entered are the word calls asked what their words become, each as
	// "symbol in declaration: arguments" (see categorizer).
	entered []string
	// kept are the kept callables asked what they become, each with the
	// function keeping it and while what it is kept; tables the tables of
	// names asked, each with its readers.
	kept, tables []string
	// peers are the table rows asked which peer input they send, each with
	// the peer the preset chose ("none" for del: a reader who is unsure).
	peers []string
	// leaked are the questions whose item lists, among what registers a
	// declaration, a word a call gives an outside symbol before its entry
	// was decided.
	leaked []string
	// unnamed counts rows of the incoming boundaries table that had no words
	// to name them by: without captions such a row has nothing to decide.
	unnamed int
	// entryWindows counts the requests that named entries.
	entryWindows int
}

func (*kvdPreset) State() []byte { return []byte(`{"provider":"kvd-preset"}`) }

func (*kvdPreset) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	return llm.NewPrepared([]byte(prompt.User))
}

func (preset *kvdPreset) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var request struct {
		Task    string           `json:"task"`
		Table   string           `json:"table"`
		Fill    []map[string]any `json:"fill"`
		Context map[string]any   `json:"context"`
		Rows    []map[string]any `json:"rows"`
		Units   []struct {
			Ref  string `json:"ref"`
			Path string `json:"path"`
			Box  string `json:"box"`
		} `json:"units"`
	}
	if err := json.Unmarshal(prepared.Bytes(), &request); err != nil {
		return llm.Completion{}, err
	}
	var answer any
	switch {
	case request.Task == "repomap.atlas.parts.v2":
		// One part per source file, or per box of a split file, is as good
		// a map as any for this test.
		var groups []map[string]any
		for _, unit := range request.Units {
			name := unit.Path
			if unit.Box != "" {
				name = unit.Path + ": " + unit.Box
			}
			groups = append(groups, map[string]any{"name": name, "units": []string{unit.Ref}})
		}
		answer = map[string]any{"groups": groups}
	case request.Task == "repomap.atlas.describe.v1":
		answer = map[string]any{"description": "Preset description."}
	case request.Task == "repomap.atlas.areas.v1":
		answer = map[string]any{"areas": []any{}}
	case request.Table == "atlas_boundaries", request.Table == "atlas_systems", request.Table == "atlas_joints", request.Table == "atlas_targets", request.Table == "atlas_symbols":
		outgoing := false
		for _, column := range request.Fill {
			outgoing = outgoing || column["name"] == "destination"
		}
		if request.Table == "atlas_boundaries" && !outgoing {
			preset.mu.Lock()
			preset.entryWindows++
			preset.mu.Unlock()
		}
		rows := make([]map[string]any, 0, len(request.Rows))
		for _, row := range request.Rows {
			if request.Table == "atlas_boundaries" && !outgoing && row["word_options"] == nil {
				preset.mu.Lock()
				preset.unnamed++
				preset.mu.Unlock()
			}
			cells, err := preset.answer(request.Table, request.Fill, request.Context, row)
			if err != nil {
				return llm.Completion{}, err
			}
			rows = append(rows, cells)
		}
		answer = map[string]any{"rows": rows}
	default:
		return llm.Completion{}, fmt.Errorf("kvd preset: no answer for task %q table %q", request.Task, request.Table)
	}
	response, err := json.Marshal(answer)
	if err != nil {
		return llm.Completion{}, err
	}
	return llm.Completion{Response: response, FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, nil
}

func (preset *kvdPreset) answer(table string, fill []map[string]any, context, row map[string]any) (map[string]any, error) {
	answer := map[string]any{"key": row["key"]}
	for _, column := range fill {
		name, _ := column["name"].(string)
		if !conditionHolds(column, answer, row) {
			continue
		}
		switch {
		case table == "atlas_boundaries" && name == "name":
			words, _ := row["words"].([]any)
			var values []string
			for _, item := range words {
				word, _ := item.(map[string]any)
				value, _ := word["value"].(string)
				values = append(values, value)
			}
			preset.mu.Lock()
			preset.named = append(preset.named, strings.Join(values, " "))
			preset.mu.Unlock()
			// The command a client sends; the record type names nothing.
			var refs []string
			for _, command := range cCommandRows {
				refs = append(refs, wordRefs(row, command.name)...)
			}
			answer["name"] = refs
		case table == "atlas_boundaries" && name == "destination":
			// kvcli's connect reaches the server: no package names a
			// system, so the reader writes it.
			answer[name] = "other: kvd server"
		case table == "atlas_boundaries" && name == "address":
			answer[name] = "unknown"
		case table == "atlas_systems" && name == "system":
			// A socket or libc header is no system of its own.
			answer[name] = "none"
		case table == "atlas_joints" && name == "same":
			answer[name] = "yes"
		case table == "atlas_joints" && name == "peer":
			answer[name] = preset.peer(context, row)
		case table == "atlas_joints" && name == "label":
			answer[name] = "sends a command"
		case table == "atlas_targets" && name == "role":
			answer[name] = "product"
		case table == "atlas_symbols" && name == "line":
			// A type's line is asked with or without captions.
			answer[name] = "A preset line."
		default:
			return nil, fmt.Errorf("kvd preset: no answer for %s.%s", table, name)
		}
	}
	return answer, nil
}

// peer chooses the peer a row names, as a reader would: the client's
// connect reaches the server's listening socket; a row of the client's
// command table names the server's input of the same command, but for del,
// which this reader leaves unmatched, so that the code's own linking by
// equal words would show.
func (preset *kvdPreset) peer(context, row map[string]any) string {
	side, _ := row["a"].(map[string]any)
	values := stringsOf(side["values"])
	peers, _ := context["peers"].([]any)
	choose := func(match func(map[string]any) bool) string {
		for _, item := range peers {
			if peer, _ := item.(map[string]any); match(peer) {
				return fmt.Sprint(peer["ref"])
			}
		}
		return "none"
	}
	if external, _ := side["external"].(string); strings.HasSuffix(external, ".connect") {
		return choose(func(peer map[string]any) bool { external, _ := peer["external"].(string); return strings.HasSuffix(external, ".listen") })
	}
	chosen := "none"
	if len(values) > 0 && values[0] != "del" {
		chosen = choose(func(peer map[string]any) bool { return slices.Contains(stringsOf(peer["values"]), values[0]) })
	}
	preset.mu.Lock()
	preset.peers = append(preset.peers, strings.Join(values, " ")+" -> "+chosen)
	preset.mu.Unlock()
	return chosen
}

// categorizer answers the closed tables: every declaration and candidate is
// a key, and the part of the server's commands is what kvd exists for. Of
// the outside symbols, a command table row binds a request, the thread's
// start binds work that runs as long as the server; binding, listening on
// and accepting from the server's socket serve; every other symbol, such as
// fopen or the socket's creation, is none. Each word call is answered from
// where its compared argument comes from, as a reader of the call would: an
// element of main's argument vector is an option (command), the first word
// of a line the configuration file splits into is a setting, anything else
// none. Of the callables kvd's own event loop keeps, the one it runs when
// the listening socket has a connection to accept is a request, the others
// none; the client's table of command names makes commands.
func (preset *kvdPreset) categorizer() *typesafetest.Categorizer {
	decide := typesafetest.ByColumn(map[string]llm.Verdict{"explains": typesafetest.Yes(0.9), "key_symbol": typesafetest.Choose("yes")})
	return &typesafetest.Categorizer{Decide: func(key string, question llm.Question) (llm.Verdict, bool) {
		symbol, _ := question.Item["symbol"].(string)
		usage, _ := question.Item["usage"].(string)
		column := key[strings.LastIndex(key, "|")+1:]
		if registered, _ := json.Marshal(question.Item["registered"]); strings.Contains(string(registered), "--symbols") {
			preset.mu.Lock()
			preset.leaked = append(preset.leaked, key)
			preset.mu.Unlock()
		}
		switch column {
		case "binds":
			preset.mu.Lock()
			preset.handed = append(preset.handed, symbol)
			preset.mu.Unlock()
			switch {
			case strings.Contains(usage, `{"get", getCommand`), strings.Contains(usage, `{"ping", pingCommand`):
				// A row of the table a client's first word is looked up in.
				return typesafetest.Choose("request"), true
			case strings.Contains(usage, "pthread_create"):
				// The thread's body loops for as long as the server runs.
				return typesafetest.Choose("continuous"), true
			}
			return typesafetest.Choose("none"), true
		case "publishes":
			return typesafetest.Choose("none"), true
		case "enters":
			arguments := strings.Join(stringsOf(question.Item["arguments"]), "; ")
			in, _ := question.Item["in"].(string)
			preset.mu.Lock()
			preset.entered = append(preset.entered, symbol+" in "+strings.Fields(in+" ?")[0]+": "+arguments)
			preset.mu.Unlock()
			switch {
			case strings.Contains(arguments, "result of calling splitLine"):
				return typesafetest.Choose("setting"), true
			case strings.Contains(arguments, "of parameter #2 argv of main"):
				return typesafetest.Choose("command"), true
			}
			return typesafetest.Choose("none"), true
		case "becomes":
			candidate := fmt.Sprint(question.Item["callable"], question.Item["table"])
			preset.mu.Lock()
			if question.Item["table"] != nil {
				preset.tables = append(preset.tables, fmt.Sprintf("%v read by %v", question.Item["table"], question.Item["read_by"]))
			} else {
				preset.kept = append(preset.kept, fmt.Sprintf("%v by %v during %v", strings.Fields(fmt.Sprint(question.Item["callable"]))[0], strings.Fields(fmt.Sprint(question.Item["kept_by"]))[0], question.Item["during"]))
			}
			preset.mu.Unlock()
			switch {
			case strings.HasPrefix(candidate, "acceptHandler"):
				return typesafetest.Choose("request"), true
			case question.Item["table"] == "cmdTable":
				return typesafetest.Choose("command"), true
			}
			return typesafetest.Choose("none"), true
		case "talks":
			if value := preset.roles[symbol]["talks"]; value != "" {
				return typesafetest.Choose(value), true
			}
			for _, serving := range []string{".bind", ".listen", ".accept"} {
				if strings.HasSuffix(symbol, serving) {
					return typesafetest.Choose("serves"), true
				}
			}
			return typesafetest.Choose("none"), true
		case "role":
			return typesafetest.Choose("domain"), true
		case "boxes":
			// Every file stays whole: its parts are its files.
			return typesafetest.Choose("one box"), true
		case "helper":
			return typesafetest.Choose("responsibility"), true
		}
		return decide(key, question)
	}}
}
