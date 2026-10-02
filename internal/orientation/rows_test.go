package orientation

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// shownCall is what a provider may learn of one call: every field of the
// graph's call but its column, its callees' identities and its argument
// origins, which stay local; a callee is its declaration's name and place,
// origins are cut to lines.OriginDepth as every evidence row cuts them.
type shownCall struct {
	Name, Kind, Invocation, Dispatch, Detail, Resolution string
	Line                                                 int
	Callees                                              []string // "name path:line"
	Repository                                           bool     // callees the repository indexes, none named
	Arguments, Values                                    []string
	Receiver, Result                                     *sourcevalue.Value
	API                                                  *atlas.CallAPI
	Evidence                                             []atlas.EdgeEvidence
}

func shown(call atlas.SymbolCall, places map[string]atlas.Place) shownCall {
	result := shownCall{
		Name: call.Name, Kind: call.Kind, Invocation: call.Invocation, Dispatch: call.Dispatch, Detail: call.Detail,
		Resolution: call.Resolution, Line: call.Line, Arguments: call.Arguments, Values: call.Values,
		Receiver: cut(call.ReceiverValue, 0), Result: cut(call.ResultValue, 0), API: call.API, Evidence: call.Evidence,
	}
	for _, id := range call.CalleeIDs {
		if place, known := places[id]; known && place.Symbol != nil {
			result.Callees = append(result.Callees, fmt.Sprintf("%s %s:%d", place.Symbol.Decl.Name, place.Path, place.LineNo))
		}
	}
	result.Repository = len(result.Callees) == 0 && len(call.CalleeIDs) > 0
	return result
}

func cut(value *sourcevalue.Value, level int) *sourcevalue.Value {
	if value == nil {
		return nil
	}
	result := &sourcevalue.Value{Kind: value.Kind, Text: value.Text}
	if level >= lines.OriginDepth {
		return result
	}
	result.Initializer = cut(value.Initializer, level+1)
	for i := range value.Parts {
		result.Parts = append(result.Parts, *cut(&value.Parts[i], level+1))
	}
	return result
}

var callHead = regexp.MustCompile(`^(.*)@(-?\d+)(?: -> (.*))?$`)

// readRow reads a member row back from its JSON: each call as shownCall.
// A callee written as a member's ref is that member's name and anchor.
func readRow(t *testing.T, raw []byte, members map[string]memberRow) []shownCall {
	t.Helper()
	var row struct {
		Calls    []any            `json:"calls"`
		Evidence map[string][]any `json:"evidence"`
	}
	if err := json.Unmarshal(raw, &row); err != nil {
		t.Fatal(err)
	}
	var calls []shownCall
	for _, value := range row.Calls {
		tuple, isList := value.([]any)
		if !isList {
			tuple = []any{value}
		}
		head := callHead.FindStringSubmatch(tuple[0].(string))
		if head == nil {
			t.Fatalf("call head %q", tuple[0])
		}
		call := shownCall{Name: head[1], Kind: defaultCallKind, Resolution: defaultResolution}
		call.Line, _ = strconv.Atoi(head[2])
		if head[3] == repositoryCallee {
			call.Repository = true
		} else if head[3] != "" {
			for _, callee := range strings.Split(head[3], " | ") {
				if member, listed := members[callee]; listed {
					call.Callees = append(call.Callees, member.Name+" "+member.Anchor)
					continue
				}
				at := strings.LastIndex(callee, " (")
				call.Callees = append(call.Callees, callee[:at]+" "+strings.TrimSuffix(callee[at+2:], ")"))
			}
		}
		details := map[string]any{}
		for _, part := range tuple[1:] {
			switch part := part.(type) {
			case string:
				for _, word := range strings.Fields(part) {
					switch {
					case slices.Contains(callKinds, word):
						call.Kind = word
					case slices.Contains(callInvocations, word):
						call.Invocation = word
					case slices.Contains(callDispatches, word):
						call.Dispatch = word
					case slices.Contains(callResolutions, word):
						call.Resolution = word
					default:
						t.Fatalf("flag %q names no field", word)
					}
				}
			case map[string]any:
				details = part
			}
		}
		text := func(key string) string { value, _ := details[key].(string); return value }
		if _, set := details["kind"]; set {
			call.Kind = text("kind")
		}
		if _, set := details["resolution"]; set {
			call.Resolution = text("resolution")
		}
		for key, field := range map[string]*string{"invocation": &call.Invocation, "dispatch": &call.Dispatch, "detail": &call.Detail} {
			if _, set := details[key]; set {
				*field = text(key)
			}
		}
		strs := func(key string) []string {
			var out []string
			for _, value := range asList(details[key]) {
				out = append(out, value.(string))
			}
			return out
		}
		call.Arguments = strs("arguments")
		call.Receiver, call.Result = readOrigin(details["receiver"]), readOrigin(details["result"])
		call.Values = strs("values")
		if api := strs("api"); api != nil {
			call.API = &atlas.CallAPI{Package: api[0], Receiver: api[1], Name: api[2], Signature: api[3]}
		}
		for _, ref := range strs("evidence_refs") {
			observed := row.Evidence[ref]
			call.Evidence = append(call.Evidence, atlas.EdgeEvidence{Extractor: observed[0].(string), Label: observed[1].(string), Path: observed[2].(string), LineNo: int(observed[3].(float64))})
		}
		calls = append(calls, call)
	}
	return calls
}

func asList(value any) []any {
	list, _ := value.([]any)
	return list
}

func readOrigin(value any) *sourcevalue.Value {
	tuple, ok := value.([]any)
	if !ok {
		return nil
	}
	origin := &sourcevalue.Value{Kind: tuple[0].(string), Text: tuple[1].(string)}
	if len(tuple) > 2 {
		origin.Initializer = readOrigin(tuple[2])
		for _, part := range tuple[3:] {
			origin.Parts = append(origin.Parts, *readOrigin(part))
		}
	}
	return origin
}

// A member row's call tuples hold every field of every call the graph keeps
// for a provider: read back, each is the call as the evidence rows would
// show it, in the order written. No argument origin is among them
// (2026-09-30: othello's arguments were half its member bytes, and a change
// of their form alone had moved its main flow from 9 steps to 47), while a
// call's literal words stay its values.
func TestAMemberRowReadsBackEveryCallLosslessly(t *testing.T) {
	deep := &sourcevalue.Value{Kind: "call", Text: "f(g(h(x)))", Parts: []sourcevalue.Value{
		{Kind: "call", Text: "g(h(x))", Parts: []sourcevalue.Value{{Kind: "call", Text: "h(x)", Parts: []sourcevalue.Value{{Kind: "parameter", Text: "x"}}}}},
	}}
	field := &sourcevalue.Value{Kind: "field", Text: "self.freqtrade", Initializer: &sourcevalue.Value{Kind: "call", Text: "FreqtradeBot(config)"}}
	evidence := atlas.EdgeEvidence{Extractor: "c", Label: "table row", Path: "cmd.c", LineNo: 40}
	callee := atlas.Place{ID: "sym:work", Path: "work.py", LineNo: 30, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "Worker.run", ObjectID: "t1.n2"}}}
	outside := atlas.Place{ID: "sym:other", Path: "other.py", LineNo: 5, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "other (odd)", ObjectID: "t1.n3"}}}
	calls := []atlas.SymbolCall{
		{Name: "late", Line: 0, Kind: "calls"},
		{Name: "loadConfig", Line: 12, Column: 9, Kind: "calls", Resolution: "exact", CalleeIDs: []string{"sym:work", "sym:other"},
			SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "index", Text: "argv[1]"}}}},
		{Name: "Worker", Line: 12, Column: 2, Kind: "calls", Invocation: "construct", Resolution: "exact", ResultValue: &sourcevalue.Value{Kind: "name", Text: "worker"}},
		{Name: "os.Getenv", Line: 14, Kind: "calls", Resolution: "exact", Values: []string{"LITESTREAM_CONFIG"},
			SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "literal", Text: "LITESTREAM_CONFIG"}}},
			API:             &atlas.CallAPI{Package: "os", Name: "Getenv", Signature: "func(key string) string"}},
		{Name: "flag.String", Line: 15, Kind: "calls", Resolution: "exact", Values: []string{"config", "{param}"},
			SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "literal", Text: "config"}}, {Position: 3, Keyword: "usage", Origin: nil}},
			API:             &atlas.CallAPI{Package: "flag", Receiver: "*FlagSet", Name: "String"}},
		{Name: "_throttle", Line: 20, Kind: "passes_callback", Resolution: "alternatives", Dispatch: "function_value", Invocation: "async_task",
			SourceArguments: []atlas.SourceArgument{{Keyword: "func", Origin: field}}, ReceiverValue: deep, Arguments: []string{"Worker._process_running"},
			Evidence: []atlas.EdgeEvidence{evidence, evidence}, CalleeIDs: []string{"sym:gone"}},
		{Name: "proc", Line: 21, Kind: "", Resolution: "", Detail: "via cmd->proc", Dispatch: "table", Evidence: []atlas.EdgeEvidence{evidence},
			Stores: []atlas.CallStore{{CalleeID: "sym:work", Path: "cmd.c", LineNo: 40}}},
		{Name: "odd@name -> x", Line: 22, Kind: "reads", Resolution: "unresolved"},
	}
	member := atlas.Place{ID: "sym:main", Path: "main.py", LineNo: 3, Symbol: &atlas.SymbolFacts{
		Decl: atlas.Decl{Name: "main", Kind: "function", Signature: "def main(argv)", Doc: "Start the bot.", ObjectID: "t1.n1"}, Calls: calls,
	}}
	graph := atlas.Graph{Places: []atlas.Place{member, callee, outside}}
	writer := newRowWriter(graph)
	writer.refs = map[string]string{"sym:main": "t1.n1", "sym:work": "t1.n2"}
	rows := map[string]memberRow{
		"t1.n1": writer.row("t1.n1", memberWire{}, &member),
		"t1.n2": writer.row("t1.n2", memberWire{}, &callee),
	}
	raw, err := json.Marshal(rows["t1.n1"])
	if err != nil {
		t.Fatal(err)
	}
	for _, local := range []string{"sym:", `"column"`, "callee_id", "stores", `"t1.n3"`, `"args"`, "argv[1]", "self.freqtrade"} {
		if strings.Contains(string(raw), local) {
			t.Fatalf("the row carries a local identity %q: %s", local, raw)
		}
	}
	got := readRow(t, raw, rows)
	var want []shownCall
	for _, call := range lines.WrittenOrder(calls) {
		want = append(want, shown(call, writer.places))
	}
	if len(got) != len(want) {
		t.Fatalf("read %d calls, want %d: %s", len(got), len(want), raw)
	}
	for i := range want {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Fatalf("call %d reads back differently:\n got %+v\nwant %+v\nrow %s", i, got[i], want[i], raw)
		}
	}
	if row := rows["t1.n1"]; row.Name != "main" || row.Anchor != "main.py:3" || row.Signature != "def main(argv)" || row.Kind != "function" {
		t.Fatalf("the row lost its declaration: %+v", row)
	}
	// Code structure only: the docstring is the author's claim, never sent.
	if strings.Contains(string(raw), "Start the bot.") || strings.Contains(string(raw), "author_doc") {
		t.Fatalf("the row carries the author's docstring: %s", raw)
	}
}
