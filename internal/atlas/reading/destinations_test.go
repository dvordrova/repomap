package reading

import (
	"fmt"
	"maps"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

func TestDestinationChainsKeepCallArgumentsTogether(t *testing.T) {
	anchor := &sourcevalue.Anchor{Path: "helper.go", Line: 10, Column: 1}
	parameter := func(n int) sourcevalue.Value { return sourcevalue.Value{Kind: "parameter", Position: n, Owner: anchor} }
	literal := func(text string) *sourcevalue.Value { return &sourcevalue.Value{Kind: "literal", Text: text} }
	helper := atlas.Place{ID: "helper", Path: "helper.go", LineNo: 10, TargetIDs: []string{"app"}, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "Send", Column: 1}}}
	call := atlas.SymbolCall{Name: "http.Get", Line: 11, Column: 5, API: &atlas.CallAPI{Package: "net/http", Name: "Get"}, SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "concat", Parts: []sourcevalue.Value{parameter(1), parameter(2)}}}}}
	helper.Symbol.Calls = []atlas.SymbolCall{call}
	app := atlas.Place{ID: "app", Path: "main.go", LineNo: 1, TargetIDs: []string{"app"}, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "main"}}}
	app.Symbol.Calls = []atlas.SymbolCall{
		{Name: "Send", Line: 2, Column: 3, CalleeIDs: []string{"helper"}, SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: literal("https://first.example")}, {Position: 2, Origin: literal("/a")}}},
		{Name: "Send", Line: 3, Column: 3, CalleeIDs: []string{"helper"}, SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: literal("https://second.example")}, {Position: 2, Origin: literal("/b")}}},
	}
	sibling := atlas.Place{ID: "other", Path: "other.go", LineNo: 1, TargetIDs: []string{"other"}, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "Other"}, Calls: []atlas.SymbolCall{{Name: "Send", Line: 2, Column: 3, CalleeIDs: []string{"helper"}, SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: literal("https://unrelated.example")}, {Position: 2, Origin: literal("/c")}}}}}}
	uses := NewDestinationReader([]atlas.Place{helper, app, sibling}).Read(helper, call)
	var addresses []string
	for _, use := range uses {
		addresses = append(addresses, use.Address)
		if len(use.Steps) != 2 {
			t.Fatalf("chain lost source sites: %+v", use)
		}
	}
	sort.Strings(addresses)
	if !reflect.DeepEqual(addresses, []string{"https://first.example/a", "https://second.example/b"}) {
		t.Fatalf("unobserved cross-product or other target: %v", addresses)
	}
}

func TestDestinationStopsAtUnknownFactoryInsteadOfSelectingNearbyLiteral(t *testing.T) {
	place := atlas.Place{ID: "sender", Path: "pipeline.go", LineNo: 10, TargetIDs: []string{"server"}, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "pipeline.post"}}}
	call := atlas.SymbolCall{Name: "http.RoundTripper.RoundTrip", Line: 30, Column: 5, API: &atlas.CallAPI{Package: "net/http", Name: "RoundTrip"}, SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "call_result", Anchor: &sourcevalue.Anchor{Path: "pipeline.go", Line: 20, Column: 5}}}}}
	place.Symbol.Calls = []atlas.SymbolCall{call, {Name: "picker.pick", Line: 20, Column: 5}, {Name: "log.Printf", Line: 19, Column: 5, Values: []string{"https://unrelated.example"}}}
	uses := NewDestinationReader([]atlas.Place{place}).Read(place, call)
	if len(uses) != 1 || uses[0].Address != "" || uses[0].Frontier != "picker.pick()" || len(uses[0].Steps) != 2 || uses[0].Steps[1].Line != 20 {
		t.Fatalf("frontier changed into guessed destination: %+v", uses)
	}
}

func TestDestinationConstructorBindsItsOwnKeywordArguments(t *testing.T) {
	formal := &sourcevalue.Anchor{Path: "adapter.py", Line: 3, Column: 5}
	parameter := func(name string, position int) sourcevalue.Value {
		return sourcevalue.Value{Kind: "parameter", Text: name, Position: position, Owner: formal}
	}
	result := &sourcevalue.Value{Kind: "record", Owner: formal, Parts: []sourcevalue.Value{{Kind: "field_value", Text: "endpoint", Parts: []sourcevalue.Value{{Kind: "concat", Parts: []sourcevalue.Value{parameter("base", 1), parameter("suffix", 2)}}}}}}
	app := atlas.Place{ID: "app", Path: "main.py", LineNo: 1, TargetIDs: []string{"app"}, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "main"}}}
	for i, values := range [][2]string{{"https://first.example", "/a"}, {"https://second.example", "/b"}} {
		app.Symbol.Calls = append(app.Symbol.Calls, atlas.SymbolCall{Name: "Adapter", Line: 2 + i, Column: 5, CalleeIDs: []string{"native-class"}, ResultValue: result, SourceArguments: []atlas.SourceArgument{
			{Keyword: "base", Origin: &sourcevalue.Value{Kind: "literal", Text: values[0]}},
			{Keyword: "suffix", Origin: &sourcevalue.Value{Kind: "literal", Text: values[1]}},
		}})
	}
	call := atlas.SymbolCall{Name: "requests.get", Line: 5, Column: 5, API: &atlas.CallAPI{Package: "requests.api", Name: "get"}, SourceArguments: []atlas.SourceArgument{{Keyword: "url", Origin: &sourcevalue.Value{Kind: "field", Text: "endpoint", Parts: []sourcevalue.Value{{Kind: "call_result", Anchor: &sourcevalue.Anchor{Path: "main.py", Line: 2, Column: 5}}}}}}}
	app.Symbol.Calls = append(app.Symbol.Calls, call)
	uses := NewDestinationReader([]atlas.Place{app}).Read(app, call)
	if len(uses) != 1 || uses[0].Address != "https://first.example/a" {
		t.Fatalf("constructor lost formal ownership or mixed independent instances: %+v", uses)
	}
}

func TestDestinationAlternativeFieldsKeepTheirSourcePair(t *testing.T) {
	record := func(base, path string) sourcevalue.Value {
		return sourcevalue.Value{Kind: "record", Parts: []sourcevalue.Value{
			{Kind: "field_value", Text: "base", Parts: []sourcevalue.Value{{Kind: "literal", Text: base}}},
			{Kind: "field_value", Text: "path", Parts: []sourcevalue.Value{{Kind: "literal", Text: path}}},
		}}
	}
	choice := sourcevalue.Value{Kind: "alternatives", Anchor: &sourcevalue.Anchor{Path: "app.ts", Line: 3, Column: 1}, Parts: []sourcevalue.Value{record("https://first.example", "/a"), record("https://second.example", "/b")}}
	url := &sourcevalue.Value{Kind: "concat", Parts: []sourcevalue.Value{{Kind: "field", Text: "base", Parts: []sourcevalue.Value{choice}}, {Kind: "field", Text: "path", Parts: []sourcevalue.Value{choice}}}}
	call := atlas.SymbolCall{Name: "fetch", Line: 5, Column: 1, API: &atlas.CallAPI{Package: "platform:javascript", Name: "fetch"}, SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: url}}}
	p := atlas.Place{ID: "main", Path: "app.ts", LineNo: 1, TargetIDs: []string{"app"}, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "main"}, Calls: []atlas.SymbolCall{call}}}
	uses := NewDestinationReader([]atlas.Place{p}).Read(p, call)
	if len(uses) != 2 {
		t.Fatalf("lost source alternatives: %+v", uses)
	}
	for _, use := range uses {
		if use.Address == "https://first.example/b" || use.Address == "https://second.example/a" {
			t.Fatalf("unobserved address: %+v", uses)
		}
	}
}

// A shared helper is linked into the server and the client, like Redis's
// anetTcpGenericConnect; the client runs it through its own caller, while
// the other caller, linked into both, runs only in the server. The address
// that caller passes is the server's alone.
func TestDestinationChainsStayWithTheTargetsThatRunEveryStep(t *testing.T) {
	anchor := &sourcevalue.Anchor{Path: "net.go", Line: 10, Column: 1}
	literal := func(text string) []atlas.SourceArgument {
		return []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "literal", Text: text}}}
	}
	call := atlas.SymbolCall{Name: "http.Get", Line: 12, Column: 5, API: &atlas.CallAPI{Package: "net/http", Name: "Get"},
		SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "parameter", Position: 1, Owner: anchor}}}}
	connect := atlas.Place{ID: "connect", Path: "net.go", LineNo: 10, TargetIDs: []string{"cli", "server"},
		Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "Connect", Column: 1}, Calls: []atlas.SymbolCall{call}}}
	cli := atlas.Place{ID: "cli", Path: "cli.go", LineNo: 1, TargetIDs: []string{"cli"}, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "main"},
		Calls: []atlas.SymbolCall{{Name: "Connect", Line: 2, Column: 3, CalleeIDs: []string{"connect"}, SourceArguments: literal("https://cli.example")}}}}
	replicate := atlas.Place{ID: "replicate", Path: "net.go", LineNo: 30, TargetIDs: []string{"cli", "server"}, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "Replicate"}, Unreached: []string{"cli"},
		Calls: []atlas.SymbolCall{{Name: "Connect", Line: 31, Column: 3, CalleeIDs: []string{"connect"}, SourceArguments: literal("https://master.example")}}}}
	targets := map[string][]string{}
	for _, use := range NewDestinationReader([]atlas.Place{connect, cli, replicate}).Read(connect, call) {
		targets[use.Address] = use.TargetIDs
	}
	if want := map[string][]string{"https://cli.example": {"cli"}, "https://master.example": {"server"}}; !reflect.DeepEqual(targets, want) {
		t.Fatalf("addresses by target = %v, want %v", targets, want)
	}
}

// Two calls through one outside package, asked in different windows beside
// different neighbours, are offered one and the same catalogue and take the
// same name: litestream's lone Azure DeleteBlob was named S3 while its
// siblings' window named Azure Blob Storage. The name is asked once per
// package; a package that reaches no outside system gives no entry.
func TestOneOutsidePackageIsOfferedTheSameDestinationsInEveryWindow(t *testing.T) {
	const azblob, s3, gateway = "github.com/Azure/azure-sdk-for-go/sdk/storage/azblob", "github.com/aws/aws-sdk-go-v2/service/s3", "example.com/gateway"
	call := func(pkg, receiver, name string, line int) atlas.SymbolCall {
		return atlas.SymbolCall{Kind: "invokes_external", Name: name, Line: line, Column: 9, API: &atlas.CallAPI{Package: pkg, Receiver: receiver, Name: name}}
	}
	symbol := func(id, path, target string, line int, calls ...atlas.SymbolCall) atlas.Place {
		return atlas.Place{ID: "symbol:" + id, Kind: atlas.PlaceSymbol, Path: path, LineNo: line, Parent: "file:" + id, TargetIDs: []string{target},
			Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "object:" + id, Name: id}, Calls: calls}}
	}
	// A lone call in one declaration; another beside an S3 call in a
	// second; a request through the standard HTTP client in a third; a
	// gateway call of another target.
	alone := symbol("DeleteLTXFiles", "abs/replica_client.go", "service", 293, call(azblob, "*Client", "DeleteBlob", 303))
	beside := symbol("DeleteAll", "abs/all.go", "service", 30, call(azblob, "*Client", "DeleteBlob", 40), call(s3, "*Client", "GetObject", 41))
	ping := symbol("Ping", "control/ping.go", "service", 10, call("net/http", "*Client", "Get", 12))
	send := symbol("Send", "gateway/send.go", "tool", 5, call(gateway, "*Client", "Send", 7))
	// A fact without a declaration names no package and is offered its
	// target's catalogue all the same.
	upload := atlas.Place{ID: "fact:upload", Kind: atlas.PlaceBoundary, Path: "abs/upload.go", LineNo: 5, Parent: "file:upload", TargetIDs: []string{"service"},
		Boundary: &atlas.BoundaryFacts{Source: "external_call", Origins: []atlas.BoundaryOrigin{{TargetID: "service", FactID: "upload"}}, External: "azblob.Client.UploadStream", Direction: atlas.DirectionOut, GivenKind: atlas.BoundarySDK}}
	graph := []atlas.Place{alone, beside, ping, send, upload}
	systems := map[string]string{azblob: "Azure Blob Storage", s3: "Amazon S3", "net/http": "none", gateway: "Example gateway"}
	asked := make(map[string]int)
	var packageRows []map[string]any
	offered := make(map[string]string)
	windows := 0
	var mu sync.Mutex
	provider := &mutatedTableProvider{}
	provider.mutate = func(input map[string]any, rows []map[string]any) {
		mu.Lock()
		defer mu.Unlock()
		if input["table"] == "atlas_systems" {
			for i, source := range input["rows"].([]any) {
				row := source.(map[string]any)
				pkg := row["package"].(string)
				asked[pkg]++
				packageRows = append(packageRows, row)
				rows[i]["system"] = systems[pkg]
			}
			return
		}
		windows++
		catalog := input["context"].(map[string]any)["destination_catalog"]
		for i, source := range input["rows"].([]any) {
			row := source.(map[string]any)
			offered[fmt.Sprintf("%v:%v", row["path"], row["line"])] = string(mustJSON(catalog))
			rows[i]["line"], rows[i]["address"] = "sends", "unknown"
			// The entry that lists the row's package, as a reader would
			// choose; a row whose package reaches no named system names
			// its own, and a row without a package takes the target's
			// object store.
			switch pkg, _ := row["package"].(string); {
			case pkg == "net/http":
				rows[i]["destination"] = "other: Control socket"
			case pkg == "":
				rows[i]["destination"] = destinationRef(input, "Azure Blob Storage")
			default:
				rows[i]["destination"] = destinationRef(input, systems[pkg])
			}
		}
	}
	r := answerTestReader(t, nil, provider)
	r.opts.Through, r.opts.Graph.Places = "", graph
	r.opts.Targets = []TargetMeta{
		{ID: "service", Dependencies: []Dependency{{Package: azblob, Module: azblob, Version: "v1.6.2"}, {Package: s3, Module: s3, Version: "v1.97.3"}}},
		{ID: "tool", Dependencies: []Dependency{{Package: gateway, Module: gateway, Version: "v0.1.0"}}},
	}
	r.places = map[string]atlas.Place{}
	for _, place := range graph {
		r.places[place.ID] = place
	}
	r.knowledge, r.knowledgeSubjects = map[string]*Knowledge{}, map[string]*Knowledge{}
	r.responseTables = map[string]rememberedTable{}
	r.api = map[string]apiRole{}
	for _, api := range []string{azblob + ".Client.DeleteBlob", s3 + ".Client.GetObject", "net/http.Client.Get", gateway + ".Client.Send"} {
		r.api[api] = apiRole{talks: atlas.BoundarySDK}
	}
	if err := r.readBoundaries(t.Context()); err != nil {
		t.Fatal(err)
	}
	// One question per package, with its manifest record and its calls.
	if want := map[string]int{azblob: 1, s3: 1, "net/http": 1, gateway: 1}; !maps.Equal(asked, want) {
		t.Fatalf("packages asked %v, want %v", asked, want)
	}
	for _, row := range packageRows {
		if row["package"] == azblob && (!reflect.DeepEqual(row["dependency"], []any{azblob + " v1.6.2"}) || !strings.Contains(string(mustJSON(row["calls"])), `"symbol":"Client.DeleteBlob"`)) {
			t.Fatalf("the azblob item lost its record or its call: %v", row)
		}
	}
	service := offered["abs/replica_client.go:303"]
	if len(offered) != 6 {
		t.Fatalf("rows lost: %d windows, %v", windows, offered)
	}
	for _, site := range []string{"abs/all.go:40", "abs/all.go:41", "abs/upload.go:5", "control/ping.go:12"} {
		if offered[site] != service {
			t.Fatalf("%s was offered other destinations than the lone call:\n%s\n%s", site, offered[site], service)
		}
	}
	entry := func(ref, value, pkg string) map[string]any {
		return map[string]any{"ref": ref, "value": value, "packages": []string{pkg}}
	}
	if want := string(mustJSON([]any{entry("d1", "Amazon S3", s3), entry("d2", "Azure Blob Storage", azblob)})); service != want {
		t.Fatalf("the service's catalogue is not its named packages (none gives no entry):\n%s\nwant %s", service, want)
	}
	if tool := offered["gateway/send.go:7"]; tool != string(mustJSON([]any{entry("d1", "Example gateway", gateway)})) {
		t.Fatalf("another target's row is not offered its own target's catalogue: %s", tool)
	}
	got := make(map[string]string)
	for _, state := range r.boundaries {
		got[fmt.Sprintf("%s:%d", state.place.Path, state.place.LineNo)] = state.destination
	}
	want := map[string]string{"abs/replica_client.go:303": "Azure Blob Storage", "abs/all.go:40": "Azure Blob Storage", "abs/all.go:41": "Amazon S3", "abs/upload.go:5": "Azure Blob Storage", "control/ping.go:12": "Control socket", "gateway/send.go:7": "Example gateway"}
	if !maps.Equal(got, want) {
		t.Fatalf("destinations = %v, want %v", got, want)
	}
}
