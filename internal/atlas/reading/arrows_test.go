package reading

import (
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
)

// An arrow folded only from import edges has no witness call. Asked anyway,
// the model invents one: Morfeu arrows r7 and r10 came back as "store or
// fetch cached data". Such an arrow keeps its fallback sentence unasked.
func TestArrowsWithoutWitnessesTakeTheFallbackWithoutAModelRow(t *testing.T) {
	provider := &mutatedTableProvider{}
	var asked []string
	provider.mutate = func(input map[string]any, _ []map[string]any) {
		if input["table"] != lines.StageArrows {
			return
		}
		for _, row := range input["rows"].([]any) {
			fields := row.(map[string]any)
			asked = append(asked, fields["from"].(string)+" -> "+fields["to"].(string))
		}
	}
	r := answerTestReader(t, nil, provider)
	r.opts.Through = ""
	r.opts.Targets = []TargetMeta{{ID: "t"}}
	r.opts.Graph = atlas.Graph{Edges: []atlas.Edge{
		{From: "file:a", To: "file:b", Kind: "calls", Count: 2, Witnesses: []atlas.Witness{{Caller: "Main", Callee: "Help", Path: "a/x.go", LineNo: 4}}},
		{From: "file:a", To: "file:c", Kind: "imports", Count: 3},
	}}
	r.places, r.boxes, r.boxOf = map[string]atlas.Place{}, map[string]*boxState{}, map[string]string{}
	for _, box := range []struct{ id, title string }{{"a", "Alpha"}, {"b", "Beta"}, {"c", "Gamma"}} {
		fileID := "file:" + box.id
		r.places[fileID] = atlas.Place{ID: fileID, Kind: atlas.PlaceFile, Path: box.id + "/x.go", TargetIDs: []string{"t"}}
		r.boxes[box.id] = &boxState{id: box.id, dir: box.id, title: box.title, line: box.title + " does things.", files: []string{fileID}, open: true}
		r.boxOf[fileID] = box.id
	}
	r.knowledge, r.knowledgeSubjects = map[string]*Knowledge{}, map[string]*Knowledge{}
	r.responseTables = map[string]rememberedTable{}
	if err := r.readArrows(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(asked, []string{"Alpha: Alpha does things. -> Beta: Beta does things."}) {
		t.Fatalf("rows sent to the model = %q, want only the witnessed arrow", asked)
	}
	sentences := map[string]string{}
	for _, arrow := range r.arrows["t"] {
		sentences[arrow.from+"->"+arrow.to] = arrow.sentence
	}
	if !strings.HasPrefix(sentences["a->b"], "Text for") {
		t.Fatalf("witnessed arrow lost the model's sentence: %q", sentences["a->b"])
	}
	if sentences["a->c"] != "Alpha uses Gamma." {
		t.Fatalf("unwitnessed arrow sentence = %q, want the fallback", sentences["a->c"])
	}
}

// A call through a function value writes a field, not a declaration: the
// arrow between parts names the functions the fact found stored there
// (getCommand, setCommand), not the field (proc). A sentence the code writes
// names each callee once, whatever number of callers call it.
func TestPartArrowsNameStoredHandlersOnceEach(t *testing.T) {
	provider := &mutatedTableProvider{}
	r := answerTestReader(t, nil, provider)
	r.opts.Through = ""
	r.opts.Targets = []TargetMeta{{ID: "t"}}
	symbol := func(id, name string, calls ...atlas.SymbolCall) atlas.Place {
		return atlas.Place{ID: id, Kind: atlas.PlaceSymbol, Path: "x.c", TargetIDs: []string{"t"}, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: name}, Calls: calls}}
	}
	call := func(name string, callees ...string) atlas.SymbolCall {
		return atlas.SymbolCall{Kind: "calls", Name: name, Line: 1, CalleeIDs: callees}
	}
	through := call("proc", "s:get", "s:set")
	through.Dispatch = "function_value"
	places := []atlas.Place{
		symbol("s:call", "call", through),
		symbol("s:reply", "addReplyBulk", call("addReply", "s:addReply"), call("zmalloc", "s:zmalloc")),
		symbol("s:error", "addReplyError", call("addReply", "s:addReply")),
		symbol("s:get", "getCommand"), symbol("s:set", "setCommand"),
		symbol("s:addReply", "addReply"), symbol("s:zmalloc", "zmalloc"),
	}
	r.opts.Graph = atlas.Graph{Places: places}
	r.places, r.boxes, r.boxOf = map[string]atlas.Place{}, map[string]*boxState{}, map[string]string{}
	for _, place := range places {
		r.places[place.ID] = place
	}
	parts := map[string]string{"s:call": "dispatch", "s:reply": "replies", "s:error": "replies", "s:get": "strings", "s:set": "strings", "s:addReply": "output", "s:zmalloc": "output"}
	r.designBoxOf = map[string]map[string]string{"t": parts}
	for _, box := range []struct{ id, title string }{{"dispatch", "Command dispatch"}, {"replies", "Replies"}, {"strings", "String commands"}, {"output", "Output"}} {
		r.boxes[box.id] = &boxState{id: box.id, title: box.title, line: box.title + " does things."}
	}
	r.knowledge, r.knowledgeSubjects = map[string]*Knowledge{}, map[string]*Knowledge{}
	r.responseTables = map[string]rememberedTable{}
	if err := r.readArrows(t.Context()); err != nil {
		t.Fatal(err)
	}
	sentences := map[string]string{}
	for _, arrow := range r.arrows["t"] {
		sentences[arrow.from+"->"+arrow.to] = arrow.sentence
	}
	want := map[string]string{
		"dispatch->strings": "Command dispatch calls String commands: getCommand, setCommand.",
		"replies->output":   "Replies calls Output: addReply, zmalloc.",
	}
	if !reflect.DeepEqual(sentences, want) {
		t.Fatalf("sentences = %q, want %q", sentences, want)
	}
}

// Equally observed callees go in source order, never in the alphabet's: the
// call written first; among the functions one call reaches through a field,
// the one stored there first (the command table's first row); without a
// known store, the one declared first. Alphabetically Redis's dispatcher
// "calls String commands: appendCommand, decrCommand, decrbyCommand" and hid
// get and set.
func TestPartArrowTiesGoInSourceOrder(t *testing.T) {
	provider := &mutatedTableProvider{}
	r := answerTestReader(t, nil, provider)
	r.opts.Through = ""
	r.opts.Targets = []TargetMeta{{ID: "t"}}
	symbol := func(id, name, path string, line int, calls ...atlas.SymbolCall) atlas.Place {
		return atlas.Place{ID: id, Kind: atlas.PlaceSymbol, Path: path, LineNo: line, TargetIDs: []string{"t"}, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: name}, Calls: calls}}
	}
	through := atlas.SymbolCall{Kind: "calls", Name: "proc", Line: 7, Column: 5, Dispatch: "function_value", CalleeIDs: []string{"s:append", "s:decr", "s:get", "s:set"},
		Stores: []atlas.CallStore{{CalleeID: "s:append", Path: "server.c", LineNo: 3}, {CalleeID: "s:decr", Path: "server.c", LineNo: 4}, {CalleeID: "s:get", Path: "server.c", LineNo: 1}, {CalleeID: "s:set", Path: "server.c", LineNo: 2}}}
	handler := atlas.SymbolCall{Kind: "calls", Name: "handler", Line: 60, Column: 5, Dispatch: "function_value", CalleeIDs: []string{"s:accept", "s:write"}}
	places := []atlas.Place{
		symbol("s:call", "call", "server.c", 5, through),
		symbol("s:loop", "loop", "loop.c", 58, handler),
		symbol("s:write", "writeReply", "net.c", 20), symbol("s:accept", "acceptClient", "net.c", 50),
		symbol("s:main", "main", "main.c", 1,
			atlas.SymbolCall{Kind: "calls", Name: "zeta", Line: 3, Column: 5, CalleeIDs: []string{"s:zeta"}},
			atlas.SymbolCall{Kind: "calls", Name: "alpha", Line: 4, Column: 5, CalleeIDs: []string{"s:alpha"}}),
		symbol("s:set", "setCommand", "server.c", 10), symbol("s:get", "getCommand", "server.c", 12),
		symbol("s:decr", "decrCommand", "server.c", 30), symbol("s:append", "appendCommand", "server.c", 40),
		symbol("s:alpha", "alpha", "util.c", 1), symbol("s:zeta", "zeta", "util.c", 2),
	}
	r.opts.Graph = atlas.Graph{Places: places}
	r.places, r.boxes, r.boxOf = map[string]atlas.Place{}, map[string]*boxState{}, map[string]string{}
	for _, place := range places {
		r.places[place.ID] = place
	}
	parts := map[string]string{"s:call": "dispatch", "s:set": "strings", "s:get": "strings", "s:decr": "strings", "s:append": "strings", "s:main": "main", "s:alpha": "util", "s:zeta": "util",
		"s:loop": "loop", "s:write": "net", "s:accept": "net"}
	r.designBoxOf = map[string]map[string]string{"t": parts}
	for _, box := range []struct{ id, title string }{{"dispatch", "Command dispatch"}, {"strings", "String commands"}, {"main", "Entry"}, {"util", "Utilities"}, {"loop", "Event loop"}, {"net", "Networking"}} {
		r.boxes[box.id] = &boxState{id: box.id, title: box.title, line: box.title + " does things."}
	}
	r.knowledge, r.knowledgeSubjects = map[string]*Knowledge{}, map[string]*Knowledge{}
	r.responseTables = map[string]rememberedTable{}
	if err := r.readArrows(t.Context()); err != nil {
		t.Fatal(err)
	}
	sentences := map[string]string{}
	for _, arrow := range r.arrows["t"] {
		sentences[arrow.from+"->"+arrow.to] = arrow.sentence
	}
	want := map[string]string{
		"dispatch->strings": "Command dispatch calls String commands: getCommand, setCommand, appendCommand.",
		"main->util":        "Entry calls Utilities: zeta, alpha.",
		"loop->net":         "Event loop calls Networking: writeReply, acceptClient.",
	}
	if !reflect.DeepEqual(sentences, want) {
		t.Fatalf("sentences = %q, want %q", sentences, want)
	}
}
