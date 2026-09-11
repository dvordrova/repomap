package reading

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/sourcevalue"
	"github.com/dvordrova/repomap/internal/terminology"
)

func TestQuestionCallableFactsReachFinalAnswerWithoutInventingWiring(t *testing.T) {
	decl := func(id, path, name string, line int) atlas.Place {
		return atlas.Place{ID: "private-place-" + id, Kind: atlas.PlaceSymbol, Path: path, LineNo: line, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "private-object-" + id, Name: name, Kind: "function", Signature: "func()", LineNo: line, Column: 1}}}
	}
	main := decl("main", "main.go", "main", 13)
	init := decl("init", "handlers.go", "InitHandler", 12)
	reinit := decl("reinit", "handlers.go", "ReinitHandler", 30)
	trace := decl("trace", "otel.go", "newTraceProvider", 91)
	other := decl("other", "otel.go", "unrelatedNeighbour", 120)
	main.Symbol.Calls = []atlas.SymbolCall{
		{Name: "InitHandler", Kind: "calls", Line: 27, Column: 20, Invocation: "synchronous", Resolution: "exact", CalleeIDs: []string{init.ID}},
		{Name: "ReinitHandler", Kind: "calls", Line: 28, Column: 22, Invocation: "synchronous", Resolution: "exact", CalleeIDs: []string{reinit.ID}},
		{Name: "Dispatch", Kind: "calls", Line: 40, Column: 10, Resolution: "alternatives", CalleeIDs: []string{init.ID, reinit.ID}},
		{Name: "InitHandler", Kind: "calls", Line: 55, Column: 10, Invocation: "synchronous", Resolution: "exact", CalleeIDs: []string{init.ID}},
		{Name: "InitHandler", Kind: "calls", Line: 55, Column: 35, Invocation: "synchronous", Resolution: "exact", CalleeIDs: []string{init.ID}},
		{Name: "InitHandler", Kind: "calls", Line: 55, Column: 65, Invocation: "goroutine", Resolution: "exact", CalleeIDs: []string{init.ID}},
		{Name: "Unknown", Kind: "calls", Line: 60, Column: 11, Resolution: "unresolved"},
	}
	caller := atlas.SymbolCaller{PlaceID: main.ID, ObjectID: main.Symbol.Decl.ObjectID, Name: "main", Signature: "func()", Path: main.Path, Line: 27, Kind: "calls", Invocation: "synchronous", Resolution: "exact"}
	init.Symbol.CalledBy = []atlas.SymbolCaller{caller}
	caller.Line = 55
	init.Symbol.CalledBy = append(init.Symbol.CalledBy, caller)
	caller.Line = 28
	reinit.Symbol.CalledBy = []atlas.SymbolCaller{caller}
	trace.Symbol.Calls = []atlas.SymbolCall{{Name: "otlptracehttp.New", Kind: "invokes_external", Line: 97, Column: 41,
		API:             &atlas.CallAPI{Package: "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp", Name: "New"},
		Evidence:        []atlas.EdgeEvidence{{Extractor: "compiler", Path: "otel.go", LineNo: 97, Label: "selected observation"}},
		ReceiverValue:   &sourcevalue.Value{Kind: "unknown", Text: "traceExporter", Anchor: &sourcevalue.Anchor{Path: "otel.go", Line: 97, Column: 3}},
		SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "field", Text: "TraceEndpoint", Anchor: &sourcevalue.Anchor{Path: "otel.go", Line: 97, Column: 52}, Parts: []sourcevalue.Value{{Kind: "unknown", Text: "config"}}}}},
		ResultValue:     &sourcevalue.Value{Kind: "call_result", Anchor: &sourcevalue.Anchor{Path: "otel.go", Line: 97, Column: 41}}},
		{Name: "otlptracehttp.WithEndpoint", Kind: "invokes_external", Line: 100, Column: 29, Values: []string{"observed-endpoint"}, API: &atlas.CallAPI{Package: "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp", Name: "WithEndpoint"}}}
	other.Symbol.Calls = []atlas.SymbolCall{{Name: "NeighbourCall", Line: 121, Column: 12, Evidence: []atlas.EdgeEvidence{{Extractor: "compiler", Path: "otel.go", LineNo: 121, Label: "unselected-neighbour-evidence"}}}}
	symbols := []atlas.Place{main, init, reinit, trace, other}
	graph := atlas.Graph{Places: append([]atlas.Place(nil), symbols...)}
	for _, path := range []string{"main.go", "handlers.go", "otel.go"} {
		file := atlas.Place{ID: atlas.FileID(path), Kind: atlas.PlaceFile, Path: path, File: &atlas.FileFacts{}}
		for _, symbol := range symbols {
			if symbol.Path == path {
				file.File.Decls = append(file.File.Decls, symbol.Symbol.Decl)
			}
		}
		graph.Places = append(graph.Places, file)
	}
	before, _ := json.Marshal(graph)
	chunks := lines.QuestionRows(graph)
	selected := func(names ...string) (string, answerTestRequest) {
		t.Helper()
		wanted := make(map[string]bool)
		for _, name := range names {
			wanted[name] = true
		}
		route := atlas.QuestionRoute{Question: "How are these declarations connected?"}
		for _, chunk := range chunks {
			for ref, anchor := range chunk.Anchors {
				if wanted[anchor.Name] {
					route.Stops = append(route.Stops, atlas.QuestionStop{SubjectID: anchor.SubjectID, PlaceID: chunk.Place.ID, Path: anchor.Path, Line: anchor.Line, Column: anchor.Column, Name: anchor.Name, Kind: anchor.Kind, Evidence: lines.AnchorEvidence(chunk, ref)})
				}
			}
		}
		window, err := makeAnswerWindow(lines.Answer(), []atlas.QuestionRoute{route}, []answerQuestion{{index: 0, candidates: uniqueRouteAnchors(route.Stops), complete: true}})
		if err != nil {
			t.Fatal(err)
		}
		raw := string(window.table.Request)
		for _, forbidden := range []string{"private-", "callee_ids", "object_id", "place_id"} {
			if strings.Contains(raw, forbidden) {
				t.Fatalf("provider source exposed %q: %s", forbidden, raw)
			}
		}
		var request answerTestRequest
		if err := json.Unmarshal(window.table.Request, &request); err != nil {
			t.Fatal(err)
		}
		call, err := answerCall(lines.Answer(), window)
		if err != nil {
			t.Fatal(err)
		}
		collector := terminology.NewCollector([]string{"main.go", "handlers.go", "otel.go"})
		prepared, err := llm.Prepare(collector.Wrap(&tableProvider{}), call.Prompt, call.Limits)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(prepared.Bytes()), "REPOMAP_PROSE_SOURCES_V1") {
			t.Fatal("local source context leaked into provider input")
		}
		var sourceCatalog struct {
			Sources []struct{ Ref, Path, Row string }
		}
		if err := json.Unmarshal(prepared.ResponseContext(), &sourceCatalog); err != nil {
			t.Fatal(err)
		}
		for _, source := range sourceCatalog.Sources {
			if !strings.HasPrefix(source.Ref, "g") || source.Row != "r1" {
				t.Fatalf("declaration refs confused glossary result ownership: %+v", source)
			}
		}
		return raw, request
	}
	unit := func(request answerTestRequest) map[string]any {
		t.Helper()
		if len(request.Context.Candidates) != 1 || len(request.Context.Candidates[0].Observations) != 1 {
			t.Fatal("selected declaration lost source isolation")
		}
		evidence := request.Context.Candidates[0].Observations[0]["evidence"].(map[string]any)
		return evidence["evidence"].([]any)[0].(map[string]any)
	}
	_, mainRequest := selected("main")
	calls := unit(mainRequest)["calls"].([]any)
	if len(calls) != len(main.Symbol.Calls) {
		t.Fatal("own call reservoir was selected or truncated")
	}
	for i, original := range main.Symbol.Calls {
		call := calls[i].(map[string]any)
		if call["line"] != float64(original.Line) || call["column"] != float64(original.Column) || call["resolution"] != original.Resolution {
			t.Fatalf("source site or dispatch certainty changed: %v", call)
		}
	}
	if len(calls[2].(map[string]any)["callee_candidates"].([]any)) != 2 {
		t.Fatal("possible native targets were merged or discarded")
	}
	initRaw, initRequest := selected("InitHandler")
	if strings.Contains(initRaw, "ReinitHandler") || strings.Contains(initRaw, "Dispatch") || unit(initRequest)["calls"] != nil {
		t.Fatal("one callee inherited its caller's other calls")
	}
	callers := unit(initRequest)["called_by"].([]any)
	var columns []int
	for _, value := range callers {
		call := value.(map[string]any)
		columns = append(columns, int(call["column"].(float64)))
		declaration := call["caller_declaration"].(map[string]any)
		if declaration["path"] != "main.go" || declaration["line"] != float64(13) || call["resolution"] != "exact" {
			t.Fatal("incoming source authority changed")
		}
	}
	if !reflect.DeepEqual(columns, []int{20, 10, 35}) {
		t.Fatalf("same-line native callsites collapsed: %v", columns)
	}
	traceRaw, traceRequest := selected("newTraceProvider")
	for _, want := range []string{"otlptracehttp", "selected observation", "observed-endpoint", `"line":97`, `"column":41`, `"line":100`, `"column":29`, `"receiver_value"`, `"source_arguments"`, `"result_value"`, `"TraceEndpoint"`, `"kind":"unknown"`, `"kind":"call_result"`} {
		if !strings.Contains(traceRaw, want) {
			t.Fatalf("final source lost %q", want)
		}
	}
	for _, forbidden := range []string{"unselected-neighbour-evidence", "NeighbourCall", "unrelatedNeighbour"} {
		if strings.Contains(traceRaw, forbidden) {
			t.Fatalf("declaration's local evidence catalogue leaked %q", forbidden)
		}
	}
	traceUnit := unit(traceRequest)
	catalog := traceUnit["source_evidence"].(map[string]any)["by_ref"].(map[string]any)
	ref := traceUnit["calls"].([]any)[0].(map[string]any)["evidence_refs"].([]any)[0].(string)
	if catalog[ref].(map[string]any)["label"] != "selected observation" || len(catalog) != 1 {
		t.Fatal("source refs lost their declaration-local catalogue")
	}
	after, _ := json.Marshal(graph)
	if string(before) != string(after) {
		t.Fatal("question preparation mutated the sealed source graph")
	}
}
