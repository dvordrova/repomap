package contracttest

import (
	"context"
	"encoding/json"
	"fmt"
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
// table; the same two decisions name an HTTP route in the Echo preset.
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
	// Every call kvd's code gives words to is asked what they become,
	// strcmp's "--symbols" among them; answered none, they make no input.
	// A getenv call is a setting read the facts already name: not asked.
	preset.mu.Lock()
	entered := slices.Clone(preset.entered)
	preset.mu.Unlock()
	for _, symbol := range []string{"string.h.strcmp", "stdio.h.fprintf"} {
		if !slices.Contains(entered, symbol) {
			t.Fatalf("%s was not asked what its words become: %v", symbol, entered)
		}
	}
	if slices.Contains(entered, "stdlib.h.getenv") {
		t.Fatalf("a setting read the facts name was asked what its words become: %v", entered)
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
	want := []input{{kind: "continuous", name: "statsWorker", handler: "statsWorker", source: "fact"}}
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
	for position, operation := range index.Operations {
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
	// entered are the symbols asked what the words their calls are given
	// become; each is answered none, as a reader would for kvd's calls.
	entered []string
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
		Task  string           `json:"task"`
		Table string           `json:"table"`
		Fill  []map[string]any `json:"fill"`
		Rows  []map[string]any `json:"rows"`
		Units []struct {
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
	case request.Table == "atlas_boundaries":
		outgoing := false
		for _, column := range request.Fill {
			outgoing = outgoing || column["name"] == "destination"
		}
		if !outgoing {
			preset.mu.Lock()
			preset.entryWindows++
			preset.mu.Unlock()
		}
		rows := make([]map[string]any, 0, len(request.Rows))
		for _, row := range request.Rows {
			if !outgoing && row["word_options"] == nil {
				preset.mu.Lock()
				preset.unnamed++
				preset.mu.Unlock()
			}
			cells, err := preset.answer(request.Table, request.Fill, row)
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

func (preset *kvdPreset) answer(table string, fill []map[string]any, row map[string]any) (map[string]any, error) {
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
		default:
			return nil, fmt.Errorf("kvd preset: no answer for %s.%s", table, name)
		}
	}
	return answer, nil
}

// categorizer answers the closed tables: every declaration and candidate is
// a key, and the part of the server's commands is what kvd exists for. Of
// the outside symbols, a command table row binds a request, the thread's
// start binds work that runs as long as the server; binding, listening on
// and accepting from the server's socket serve; every other symbol, such as
// fopen or the socket's creation, is none.
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
			// strcmp compares a word, fprintf prints a format: none of
			// kvd's word calls is an entry.
			preset.mu.Lock()
			preset.entered = append(preset.entered, symbol)
			preset.mu.Unlock()
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
		}
		return decide(key, question)
	}}
}
