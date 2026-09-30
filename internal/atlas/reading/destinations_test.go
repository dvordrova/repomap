package reading

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// argumentsAt are the choices of one symbol's decided argument.
func argumentsAt(symbol string, choice ArgumentChoice) DestinationChoices {
	return DestinationChoices{Arguments: map[string]ArgumentChoice{symbol: choice}}
}

// Which value a call's walk follows is its symbol's decided argument, not a
// list of packages: Do follows its request, the request builder the URL it
// was given (a symbol whose result the request is, asked for its own
// argument), WithContext its receiver. What a command-line option's
// declaration or an environment read returns is that setting's value, and
// a builder with no decided argument stops the walk at its call, telling
// whoever asks which symbol it was.
func TestDestinationFollowsTheDecidedArgumentThroughBuilders(t *testing.T) {
	at := func(line int) *sourcevalue.Anchor { return &sourcevalue.Anchor{Path: "main.go", Line: line, Column: 5} }
	result := func(line int) *sourcevalue.Value { return &sourcevalue.Value{Kind: "call_result", Anchor: at(line)} }
	outside := func(pkg, receiver, name, signature string, line int, arguments ...atlas.SourceArgument) atlas.SymbolCall {
		return atlas.SymbolCall{Kind: "invokes_external", Name: name, Line: line, Column: 5, SourceArguments: arguments, API: &atlas.CallAPI{Package: pkg, Receiver: receiver, Name: name, Signature: signature}}
	}
	argument := func(position int, value *sourcevalue.Value) atlas.SourceArgument {
		return atlas.SourceArgument{Position: position, Origin: value}
	}
	flagValue := &sourcevalue.Value{Kind: "concat", Parts: []sourcevalue.Value{*result(2), {Kind: "literal", Text: "/prices"}}}
	calls := []atlas.SymbolCall{
		outside("flag", "", "String", "", 2, argument(1, &sourcevalue.Value{Kind: "literal", Text: "price-endpoint"})),
		outside("net/http", "", "NewRequestWithContext", "func(ctx context.Context, method string, url string, body io.Reader) (*http.Request, error)", 3,
			argument(1, &sourcevalue.Value{Kind: "parameter", Text: "ctx"}), argument(3, flagValue)),
		outside("net/http", "*Request", "WithContext", "func(ctx context.Context) *http.Request", 4, argument(1, &sourcevalue.Value{Kind: "parameter", Text: "ctx"})),
		outside("os", "", "Getenv", "", 6, argument(1, &sourcevalue.Value{Kind: "literal", Text: "AUDIT_URL"})),
		outside("net/url", "", "Parse", "", 7, argument(1, result(6))),
	}
	calls[2].ReceiverValue = result(3)
	do := outside("net/http", "*Client", "Do", "func(req *http.Request) (*http.Response, error)", 5, argument(1, result(4)))
	audit := outside("net/http", "*Client", "Do", "", 8, argument(1, result(7)))
	main := atlas.Place{ID: "main", Path: "main.go", LineNo: 1, TargetIDs: []string{"app"}, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "main"}, Calls: append(calls, do, audit)}}
	env := atlas.Place{ID: "b1", Kind: atlas.PlaceBoundary, Path: "main.go", LineNo: 6, Column: 5, TargetIDs: []string{"app"},
		Boundary: &atlas.BoundaryFacts{Source: "fact", Direction: atlas.DirectionOut, GivenKind: atlas.BoundaryConfig, Values: []string{"AUDIT_URL"}}}
	choices := DestinationChoices{
		Arguments: map[string]ArgumentChoice{
			"net/http.Client.Do": {Position: 1}, "net/http.NewRequestWithContext": {Keyword: "url"}, "net/http.Request.WithContext": {Receiver: true},
		},
		Options: map[sourcevalue.Anchor]string{*at(2): "price-endpoint"},
	}
	reader := NewDestinationReader([]atlas.Place{main, env}, choices)
	var undecided []string
	reader.undecided = func(symbol string, _ atlas.Place, _ atlas.SymbolCall) { undecided = append(undecided, symbol) }
	uses := reader.Read(main, do)
	if len(uses) != 1 || uses[0].Address != "{--price-endpoint}/prices" || len(uses[0].Steps) != 4 {
		t.Fatalf("the request's URL was not followed through its builders: %+v", uses)
	}
	// url.Parse has no decided argument: the walk stops at it, and says so.
	uses = reader.Read(main, audit)
	if len(uses) != 1 || uses[0].Frontier != "Parse()" || !slices.Equal(undecided, []string{"net/url.Parse"}) {
		t.Fatalf("an undecided builder: %+v, %v", uses, undecided)
	}
	choices.Arguments["net/url.Parse"] = ArgumentChoice{Position: 1}
	if uses = NewDestinationReader([]atlas.Place{main, env}, choices).Read(main, audit); len(uses) != 1 || uses[0].Address != "{env:AUDIT_URL}" {
		t.Fatalf("an environment read's value: %+v", uses)
	}
	// None and an argument the call was not given stop at the call.
	choices.Arguments["net/http.Client.Do"] = ArgumentChoice{None: true}
	if uses = NewDestinationReader([]atlas.Place{main}, choices).Read(main, do); len(uses) != 1 || uses[0].Frontier != "Do" {
		t.Fatalf("a call decided none: %+v", uses)
	}
	choices.Arguments["net/http.Client.Do"] = ArgumentChoice{Position: 2}
	if uses = NewDestinationReader([]atlas.Place{main}, choices).Read(main, do); len(uses) != 1 || uses[0].Frontier != "Do" {
		t.Fatalf("a call not given its decided argument: %+v", uses)
	}
	if got := parameterNames("func(ctx context.Context, fn func(a, b int) error, opts ...Option) (int, error)"); !slices.Equal(got, []string{"ctx", "fn", "opts"}) {
		t.Fatalf("parameter names = %v", got)
	}
	if got := parameterNames("int (int, const struct sockaddr *)"); got != nil {
		t.Fatalf("unnamed parameters named %v", got)
	}
}

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
	uses := NewDestinationReader([]atlas.Place{helper, app, sibling}, argumentsAt("net/http.Get", ArgumentChoice{Position: 1})).Read(helper, call)
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

// A walk never passes through test code: the URL a test hands the
// program's sender is not where the program's call goes, so the walk from
// the sender reaches only the program's own caller. freqtrade's webhook
// calls, once its tests were its own files, walked only into
// tests/rpc/test_rpc_webhook.py.
func TestDestinationWalksPassNoTestCaller(t *testing.T) {
	anchor := &sourcevalue.Anchor{Path: "webhook.py", Line: 10, Column: 1}
	literal := func(text string) *sourcevalue.Value { return &sourcevalue.Value{Kind: "literal", Text: text} }
	sender := atlas.Place{ID: "send", Path: "webhook.py", LineNo: 10, Parent: "file:webhook.py", TargetIDs: []string{"app"}, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "send", Column: 1}}}
	call := atlas.SymbolCall{Name: "requests.post", Line: 11, Column: 5, API: &atlas.CallAPI{Package: "requests", Name: "post"},
		SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "parameter", Position: 1, Owner: anchor}}}}
	sender.Symbol.Calls = []atlas.SymbolCall{call}
	caller := func(id, path string, url string) atlas.Place {
		return atlas.Place{ID: id, Path: path, LineNo: 1, Parent: "file:" + path, TargetIDs: []string{"app"}, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: id},
			Calls: []atlas.SymbolCall{{Name: "send", Line: 2, Column: 3, CalleeIDs: []string{"send"}, SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: literal(url)}}}}}}
	}
	places := []atlas.Place{
		{ID: "file:webhook.py", Kind: atlas.PlaceFile, Path: "webhook.py", File: &atlas.FileFacts{}},
		{ID: "file:main.py", Kind: atlas.PlaceFile, Path: "main.py", File: &atlas.FileFacts{}},
		{ID: "file:tests/test_webhook.py", Kind: atlas.PlaceFile, Path: "tests/test_webhook.py", File: &atlas.FileFacts{Test: true}},
		sender, caller("main", "main.py", "https://hooks.example/notify"), caller("test_send", "tests/test_webhook.py", "https://test.example"),
	}
	uses := NewDestinationReader(places, argumentsAt("requests.post", ArgumentChoice{Position: 1})).Read(sender, call)
	if len(uses) != 1 || uses[0].Address != "https://hooks.example/notify" {
		t.Fatalf("the walk passed a test: %+v", uses)
	}
}

func TestDestinationStopsAtUnknownFactoryInsteadOfSelectingNearbyLiteral(t *testing.T) {
	place := atlas.Place{ID: "sender", Path: "pipeline.go", LineNo: 10, TargetIDs: []string{"server"}, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "pipeline.post"}}}
	call := atlas.SymbolCall{Name: "http.RoundTripper.RoundTrip", Line: 30, Column: 5, API: &atlas.CallAPI{Package: "net/http", Name: "RoundTrip"}, SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "call_result", Anchor: &sourcevalue.Anchor{Path: "pipeline.go", Line: 20, Column: 5}}}}}
	place.Symbol.Calls = []atlas.SymbolCall{call, {Name: "picker.pick", Line: 20, Column: 5}, {Name: "log.Printf", Line: 19, Column: 5, Values: []string{"https://unrelated.example"}}}
	uses := NewDestinationReader([]atlas.Place{place}, argumentsAt("net/http.RoundTrip", ArgumentChoice{Position: 1})).Read(place, call)
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
	uses := NewDestinationReader([]atlas.Place{app}, argumentsAt("requests.api.get", ArgumentChoice{Keyword: "url"})).Read(app, call)
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
	uses := NewDestinationReader([]atlas.Place{p}, argumentsAt("platform:javascript.fetch", ArgumentChoice{Position: 1})).Read(p, call)
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
	for _, use := range NewDestinationReader([]atlas.Place{connect, cli, replicate}, argumentsAt("net/http.Get", ArgumentChoice{Position: 1})).Read(connect, call) {
		targets[use.Address] = use.TargetIDs
	}
	if want := map[string][]string{"https://cli.example": {"cli"}, "https://master.example": {"server"}}; !reflect.DeepEqual(targets, want) {
		t.Fatalf("addresses by target = %v, want %v", targets, want)
	}
}

// What an outgoing call reaches is named once per destination, never per
// call (F3: litestream's seven subcommands each dialled the control socket
// and each asked its own name, ten names in all). A call whose outside
// package atlas_systems named takes that name with no question: the lone
// Azure DeleteBlob and its sibling beside an S3 call alike. Calls whose
// walks end at the same place are one destination: two requests to one
// server from two declarations are asked once, with both calls, their
// callers and where the program reaches them from by name; a query fact
// at a database call's site shares that call's walk and takes its
// package's name. A fact without a call, and the SQL text formatted where
// a query fact stands, are their own destinations.
func TestADestinationIsNamedOnceForAllItsCalls(t *testing.T) {
	const azblob, s3, gateway = "github.com/Azure/azure-sdk-for-go/sdk/storage/azblob", "github.com/aws/aws-sdk-go-v2/service/s3", "example.com/gateway"
	call := func(pkg, receiver, name string, line int, arguments ...atlas.SourceArgument) atlas.SymbolCall {
		return atlas.SymbolCall{Kind: "invokes_external", Name: name, Line: line, Column: 9, API: &atlas.CallAPI{Package: pkg, Receiver: receiver, Name: name}, SourceArguments: arguments}
	}
	literal := func(text string) atlas.SourceArgument {
		return atlas.SourceArgument{Position: 1, Origin: &sourcevalue.Value{Kind: "literal", Text: text}}
	}
	symbol := func(id, path, target string, line int, calls ...atlas.SymbolCall) atlas.Place {
		return atlas.Place{ID: "symbol:" + id, Kind: atlas.PlaceSymbol, Path: path, LineNo: line, Parent: "file:" + id, TargetIDs: []string{target},
			Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "object:" + id, Name: id}, Calls: calls}}
	}
	alone := symbol("DeleteLTXFiles", "abs/replica_client.go", "service", 293, call(azblob, "*Client", "DeleteBlob", 303))
	beside := symbol("DeleteAll", "abs/all.go", "service", 30, call(azblob, "*Client", "DeleteBlob", 40), call(s3, "*Client", "GetObject", 41))
	send := symbol("Send", "gateway/send.go", "tool", 5, call(gateway, "*Client", "Send", 7))
	// Two subcommands ask one server for two things; main reaches both.
	// Info's item carries the calls beside its request, as written, and
	// the closure it hands its client's transport.
	info := symbol("Info", "control/info.go", "service", 10, call("net/http", "*Client", "Get", 12, literal("http://localhost/info")),
		atlas.SymbolCall{Kind: "invokes_external", Name: "fmt.Errorf", Line: 13, Column: 10, API: &atlas.CallAPI{Package: "fmt", Name: "Errorf"}, Values: []string{"failed to connect to control socket: %w"}})
	info.Symbol.Bindings = []atlas.SymbolBinding{{From: "Info", To: "Info$1", Kind: "passes_callback", Detail: "net/http.Transport.DialContext <- func(ctx context.Context, network string, addr string) (net.Conn, error)"}}
	list := symbol("List", "control/list.go", "service", 10, call("net/http", "*Client", "Get", 14, literal("http://localhost/list")))
	main := symbol("main", "main.go", "service", 1,
		atlas.SymbolCall{Kind: "calls", Name: "Info", Line: 3, Column: 2, CalleeIDs: []string{info.ID}, Resolution: "exact"},
		atlas.SymbolCall{Kind: "calls", Name: "List", Line: 4, Column: 2, CalleeIDs: []string{list.ID}, Resolution: "exact"})
	for _, callee := range []*atlas.Place{&info, &list} {
		callee.Symbol.CalledBy = []atlas.SymbolCaller{{Kind: "calls", Resolution: "exact", PlaceID: main.ID, Path: main.Path, Name: "main", Line: 3}}
	}
	// A query fact claims the Exec call at its site: it names no external,
	// and takes the package of the talking call it stands on. The next Exec
	// names database/sql.
	database := &sourcevalue.Value{Kind: "parameter", Text: "db", Position: 1, Anchor: &sourcevalue.Anchor{Path: "db/wal.go", Line: 20, Column: 14}}
	exec, vacuum := call("database/sql", "*DB", "Exec", 22), call("database/sql", "*DB", "Exec", 24)
	exec.ReceiverValue, vacuum.ReceiverValue = database, database
	checkpoint := symbol("Checkpoint", "db/wal.go", "service", 20, exec, vacuum)
	pragma := atlas.Place{ID: "fact:pragma", Kind: atlas.PlaceBoundary, Path: "db/wal.go", LineNo: 22, Column: 9, Parent: "file:Checkpoint", TargetIDs: []string{"service"},
		Boundary: &atlas.BoundaryFacts{Source: "fact", ObjectID: "object:Checkpoint", Origins: []atlas.BoundaryOrigin{{TargetID: "service", FactID: "pragma"}}, Values: []string{"PRAGMA wal_checkpoint(TRUNCATE)"}, Direction: atlas.DirectionOut, GivenKind: atlas.BoundaryDB}}
	// One URL given to an S3 call, an Azure call and a plain request: the
	// packages name two systems, so the facts show no one destination and
	// the request, named by no package, is asked alone.
	mirror := symbol("Mirror", "store/mirror.go", "service", 60,
		call(s3, "*Client", "GetObject", 61, literal("https://store.example/a")), call(azblob, "*Client", "DeleteBlob", 62, literal("https://store.example/b")),
		call("net/http", "*Client", "Get", 63, literal("https://store.example/c")))
	// A fact without a declaration names no package and walks nothing.
	upload := atlas.Place{ID: "fact:upload", Kind: atlas.PlaceBoundary, Path: "abs/upload.go", LineNo: 5, Parent: "file:upload", TargetIDs: []string{"service"},
		Boundary: &atlas.BoundaryFacts{Source: "external_call", Origins: []atlas.BoundaryOrigin{{TargetID: "service", FactID: "upload"}}, External: "azblob.Client.UploadStream", Direction: atlas.DirectionOut, GivenKind: atlas.BoundarySDK}}
	// A query fact at the fmt.Sprintf formatting its text, handed to
	// db.Exec: the Exec sends it, so its destination is the Exec's.
	formatted := &sourcevalue.Value{Kind: "call_result", Text: "Sprintf", Anchor: &sourcevalue.Anchor{Path: "db/vacuum.go", Line: 12, Column: 20}}
	handed := call("database/sql", "*DB", "Exec", 12, atlas.SourceArgument{Position: 1, Origin: formatted})
	handed.Column, handed.ReceiverValue = 9, database
	compactor := symbol("Vacuum", "db/vacuum.go", "service", 10, handed,
		atlas.SymbolCall{Kind: "invokes_external", Name: "fmt.Sprintf", Line: 12, Column: 20, API: &atlas.CallAPI{Package: "fmt", Name: "Sprintf"}})
	vacuumed := atlas.Place{ID: "fact:vacuum", Kind: atlas.PlaceBoundary, Path: "db/vacuum.go", LineNo: 12, Column: 20, Parent: "file:Vacuum", TargetIDs: []string{"service"},
		Boundary: &atlas.BoundaryFacts{Source: "fact", ObjectID: "object:Vacuum", Origins: []atlas.BoundaryOrigin{{TargetID: "service", FactID: "vacuum"}}, Values: []string{"VACUUM INTO %s"}, Direction: atlas.DirectionOut, GivenKind: atlas.BoundaryDB}}
	// A query fact whose site holds the fmt.Sprintf formatting its text,
	// handed to no call that talks: its own destination.
	shrink := symbol("Shrink", "db/shrink.go", "service", 250, atlas.SymbolCall{Kind: "invokes_external", Name: "fmt.Sprintf", Line: 252, Column: 22, API: &atlas.CallAPI{Package: "fmt", Name: "Sprintf"}})
	query := atlas.Place{ID: "fact:query", Kind: atlas.PlaceBoundary, Path: "db/shrink.go", LineNo: 252, Column: 22, Parent: "file:Shrink", TargetIDs: []string{"service"},
		Boundary: &atlas.BoundaryFacts{Source: "fact", ObjectID: "object:Shrink", Origins: []atlas.BoundaryOrigin{{TargetID: "service", FactID: "query"}}, Values: []string{"PRAGMA wal_checkpoint(%s)"}, Direction: atlas.DirectionOut, GivenKind: atlas.BoundaryDB}}
	graph := []atlas.Place{alone, beside, send, info, list, main, checkpoint, pragma, mirror, compactor, vacuumed, upload, shrink, query}
	systems := map[string]string{azblob: "Azure Blob Storage", s3: "Amazon S3", "net/http": "none", gateway: "Example gateway", "database/sql": "SQLite"}
	asked := make(map[string]int)
	var destinations []map[string]any
	var mu sync.Mutex
	provider := &mutatedTableProvider{}
	provider.mutate = func(input map[string]any, rows []map[string]any) {
		mu.Lock()
		defer mu.Unlock()
		if input["table"] == "atlas_systems" {
			for i, source := range input["rows"].([]any) {
				pkg := source.(map[string]any)["package"].(string)
				asked[pkg]++
				rows[i]["system"] = systems[pkg]
			}
			return
		}
		named := false
		for _, column := range input["fill"].([]any) {
			named = named || column.(map[string]any)["name"] == "destination"
		}
		for i, source := range input["rows"].([]any) {
			row := source.(map[string]any)
			if !named {
				if context, _ := input["context"].(map[string]any); row["reached_from"] != nil || context["destination_catalog"] != nil {
					t.Errorf("a call's own row was offered destinations: %v", row)
				}
				rows[i]["line"], rows[i]["address"] = "sends", "unknown"
				continue
			}
			destinations = append(destinations, row)
			switch text := string(mustJSON(row)); {
			case strings.Contains(text, "store.example"):
				rows[i]["destination"] = "other: Mirror"
			case strings.Contains(text, "net/http"):
				rows[i]["destination"] = "other: Control socket"
			case strings.Contains(text, "wal_checkpoint(%s)"):
				rows[i]["destination"] = "other: SQLite"
			default:
				rows[i]["destination"] = destinationRef(input, "Azure Blob Storage")
			}
		}
	}
	r := answerTestReader(t, nil, provider)
	r.opts.Through, r.opts.Graph.Places = "", graph
	r.opts.Targets = []TargetMeta{
		{ID: "service", Dependencies: []Dependency{{Package: azblob, Module: azblob, Version: "v1.6.2"}, {Package: s3, Module: s3, Version: "v1.97.3"}}},
		{ID: "tool", Dependencies: []Dependency{{Package: gateway, Module: gateway, Version: "v0.1.0"}}},
	}
	sources := map[string]string{
		"control/info.go": strings.Repeat("\n", 11) + `	resp, err := client.Get("http://localhost/info")` + "\n" + `		return fmt.Errorf("failed to connect to control socket: %w", err)` + "\n",
		"control/list.go": strings.Repeat("\n", 13) + `	resp, err := client.Get("http://localhost/list")` + "\n",
	}
	r.opts.ReadSource = func(path string) ([]byte, error) { return []byte(sources[path]), nil }
	r.places = map[string]atlas.Place{}
	for _, place := range graph {
		r.places[place.ID] = place
	}
	r.knowledge, r.knowledgeSubjects = map[string]*Knowledge{}, map[string]*Knowledge{}
	r.responseTables = map[string]rememberedTable{}
	r.api = map[string]apiRole{}
	for _, api := range []string{azblob + ".Client.DeleteBlob", s3 + ".Client.GetObject", gateway + ".Client.Send", "net/http.Client.Get", "database/sql.DB.Exec"} {
		r.api[api] = apiRole{talks: atlas.BoundarySDK}
	}
	r.arguments = map[string]ArgumentChoice{"net/http.Client.Get": {Position: 1}, "database/sql.DB.Exec": {Receiver: true},
		s3 + ".Client.GetObject": {Position: 1}, azblob + ".Client.DeleteBlob": {Position: 1}}
	if err := r.readBoundaries(t.Context()); err != nil {
		t.Fatal(err)
	}
	// One question per package, and one per destination no package names.
	if want := map[string]int{azblob: 1, s3: 1, "net/http": 1, gateway: 1, "database/sql": 1}; !maps.Equal(asked, want) {
		t.Fatalf("packages asked %v, want %v", asked, want)
	}
	if len(destinations) != 4 {
		t.Fatalf("destinations asked %d times, want 4 (the server, the mirror's request, the upload, the formatted query): %s", len(destinations), mustJSON(destinations))
	}
	for _, row := range destinations {
		text := string(mustJSON(row))
		if !strings.Contains(text, "localhost") {
			continue
		}
		for _, want := range []string{`"address":"http://localhost/info"`, `"address":"http://localhost/list"`, `client.Get(\"http://localhost/info\")`, `client.Get(\"http://localhost/list\")`, `"reached_from":["main"]`,
			`"hands_over":["Info$1 to net/http.Transport.DialContext"]`, `"calls":["fmt.Errorf(\"failed to connect to control socket: %w\", err)"]`, `"name":"List","path":"control/list.go"`} {
			if !strings.Contains(text, want) {
				t.Fatalf("the server's item lost %s: %s", want, text)
			}
		}
	}
	got := make(map[string]string)
	for _, state := range r.boundaries {
		got[fmt.Sprintf("%s:%d", state.place.Path, state.place.LineNo)] = state.destinationOf(state.place.TargetIDs[0])
	}
	want := map[string]string{
		"abs/replica_client.go:303": "Azure Blob Storage", "abs/all.go:40": "Azure Blob Storage", "abs/all.go:41": "Amazon S3", "gateway/send.go:7": "Example gateway",
		"control/info.go:12": "Control socket", "control/list.go:14": "Control socket",
		"db/wal.go:22": "SQLite", "db/wal.go:24": "SQLite", "abs/upload.go:5": "Azure Blob Storage", "db/shrink.go:252": "SQLite",
		"store/mirror.go:61": "Amazon S3", "store/mirror.go:62": "Azure Blob Storage", "store/mirror.go:63": "Mirror",
		"db/vacuum.go:12": "SQLite",
	}
	if len(got) != len(want) {
		t.Fatalf("destinations = %v", got)
	}
	for site, name := range want {
		if got[site] != name {
			t.Fatalf("destinations = %v, want %v", got, want)
		}
	}
}

// The walk's ends are a destination's identity, within one program: an
// absolute URL by its scheme and host as written, a setting's value by the
// setting, any other address or an unresolved value by where the walk
// stopped. A row the walk never read, or whose site holds no reaching call,
// is its own. A row written in code two programs share is each program's
// own destination, walked to that program's values: Redis's anet.c connect
// is reached from the server's master host and the clients' server host.
func TestDestinationKeyIsWhereTheWalksEnd(t *testing.T) {
	use := func(address, frontier string, steps ...atlas.DestinationStep) atlas.DestinationUse {
		return atlas.DestinationUse{Address: address, Frontier: frontier, Steps: steps, TargetIDs: []string{"app"}}
	}
	state := func(id string, uses ...atlas.DestinationUse) *boundaryState {
		return &boundaryState{place: atlas.Place{ID: id, TargetIDs: []string{"app"}}, uses: uses, reaching: true}
	}
	key := func(state *boundaryState) string {
		return destinationKey(destinationMember{state: state, target: state.place.TargetIDs[0]})
	}
	stop := func(path string, line int) atlas.DestinationStep {
		return atlas.DestinationStep{Path: path, Line: line, Column: 3}
	}
	other := state("b", use("http://localhost/info", ""))
	other.place.TargetIDs = []string{"tool"}
	other.uses[0].TargetIDs = []string{"tool"}
	formatted := state("b", use("", "?.PageSize", stop("main.go", 55)))
	formatted.reaching = false
	for _, same := range [][2]*boundaryState{
		{state("a", use("http://localhost/info", "")), state("b", use("http://localhost/list?x=1", ""))},
		{state("a", use("http://{--host}:{--port}/x", "")), state("b", use("http://{--host}:{--port}/y", ""))},
		{state("a", use("{--socket}", "", stop("info.go", 22))), state("b", use("{--socket}", "", stop("list.go", 22)))},
		{state("a", use("{env:API}/users", "")), state("b", use("{env:API}/orders", ""))},
		{state("a", use("", "?.DB", stop("main.go", 55))), state("b", use("", "?.DB", stop("load.go", 9), stop("main.go", 55)))},
		{state("a", use("/health", "", stop("a.go", 4))), state("b", use("/health", "", stop("a.go", 4)))},
		{state("a", use("x.db", "", stop("a.go", 4)), use("x.db", "", stop("a.go", 4))), state("b", use("x.db", "", stop("a.go", 4)))},
	} {
		if key(same[0]) != key(same[1]) {
			t.Fatalf("one destination split: %q / %q", key(same[0]), key(same[1]))
		}
	}
	for _, apart := range [][2]*boundaryState{
		{state("a", use("http://localhost:8080/info", "")), state("b", use("http://localhost/info", ""))},
		{state("a", use("http://localhost/info", "")), other},
		{state("a", use("/health", "", stop("a.go", 4))), state("b", use("/health", "", stop("b.go", 9)))},
		{state("a", use("", "?.DB", stop("main.go", 55))), state("b", use("", "?.DB", stop("main.go", 57)))},
		{state("a", use("", "computed value", stop("a.go", 1))), state("b", use("", "unresolved argument", stop("a.go", 1)))},
		{state("a", use("", "?.PageSize", stop("main.go", 55))), formatted},
		{state("a"), state("b")},
		{state("a", use("http://x.example", "")), state("b", use("http://x.example", ""), use("http://y.example", ""))},
	} {
		if key(apart[0]) == key(apart[1]) {
			t.Fatalf("two destinations joined: %q", key(apart[0]))
		}
	}
	// One shared row, two programs, each walked to its own value.
	shared := &boundaryState{place: atlas.Place{ID: "connect", TargetIDs: []string{"server", "client"}}, reaching: true, uses: []atlas.DestinationUse{
		{Frontier: "server.masterhost", Steps: []atlas.DestinationStep{stop("server.c", 7)}, TargetIDs: []string{"server"}},
		{Frontier: "config.hostip", Steps: []atlas.DestinationStep{stop("cli.c", 9)}, TargetIDs: []string{"client"}},
	}}
	server, client := destinationKey(destinationMember{shared, "server"}), destinationKey(destinationMember{shared, "client"})
	if server == client || !strings.Contains(server, "masterhost") || strings.Contains(server, "hostip") || !strings.Contains(client, "hostip") {
		t.Fatalf("the shared row's destinations are %q and %q", server, client)
	}
}

// A lookup of a host's addresses and the connection made after it are two
// exchanges even on one value: the lookup (sdk) is answered by the
// resolver, the connect (client_request) by the host (redis's
// gethostbyname(server.masterhost) had read "Primary" beside the connect
// to the master). Each is its own destination, and a reader told that a
// lookup ends where it is answered names the resolver for the lookup.
func TestALookupOfAnAddressEndsAtTheResolver(t *testing.T) {
	call := func(pkg, name string, line int) atlas.SymbolCall {
		return atlas.SymbolCall{Kind: "invokes_external", Name: name, Line: line, Column: 9, API: &atlas.CallAPI{Package: pkg, Name: name},
			SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "literal", Text: "redis://primary.example:6379"}}}}
	}
	sync := atlas.Place{ID: "symbol:syncWithMaster", Kind: atlas.PlaceSymbol, Path: "replication.c", LineNo: 10, Parent: "file:replication", TargetIDs: []string{"server"},
		Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "object:syncWithMaster", Name: "syncWithMaster"}, Calls: []atlas.SymbolCall{call("netdb.h", "gethostbyname", 12), call("sys/socket.h", "connect", 13)}}}
	resolver := strings.Contains(strings.Join(strings.Fields(lines.DestinationNames().System), " "), "A lookup of an address ends where the lookup is answered")
	provider := &mutatedTableProvider{}
	provider.mutate = func(input map[string]any, rows []map[string]any) {
		if input["table"] == "atlas_systems" {
			// The libc headers name no system: the destination is asked.
			for i := range rows {
				rows[i]["system"] = "none"
			}
			return
		}
		named := false
		for _, column := range input["fill"].([]any) {
			named = named || column.(map[string]any)["name"] == "destination"
		}
		for i, source := range input["rows"].([]any) {
			row := source.(map[string]any)
			if !named {
				rows[i]["line"], rows[i]["address"] = "sends", "unknown"
				continue
			}
			rows[i]["destination"] = "other: Primary"
			if resolver && strings.Contains(string(mustJSON(row)), `"kind":"sdk"`) {
				rows[i]["destination"] = "other: Name resolver"
			}
		}
	}
	r := answerTestReader(t, nil, provider)
	r.opts.Through, r.opts.Graph.Places = "", []atlas.Place{sync}
	r.opts.Targets = []TargetMeta{{ID: "server"}}
	r.opts.ReadSource = func(string) ([]byte, error) { return nil, nil }
	r.places = map[string]atlas.Place{sync.ID: sync}
	r.knowledge, r.knowledgeSubjects = map[string]*Knowledge{}, map[string]*Knowledge{}
	r.responseTables = map[string]rememberedTable{}
	r.api = map[string]apiRole{"netdb.h.gethostbyname": {talks: atlas.BoundarySDK}, "sys/socket.h.connect": {talks: atlas.BoundaryClientRequest}}
	r.arguments = map[string]ArgumentChoice{"netdb.h.gethostbyname": {Position: 1}, "sys/socket.h.connect": {Position: 1}}
	if err := r.readBoundaries(t.Context()); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, state := range r.boundaries {
		got[fmt.Sprintf("%s:%d", state.place.Path, state.place.LineNo)] = state.destinationOf("server")
	}
	if want := map[string]string{"replication.c:12": "Name resolver", "replication.c:13": "Primary"}; !maps.Equal(got, want) {
		t.Fatalf("destinations = %v, want %v", got, want)
	}
}

// An sdk call reaches the service behind its package whatever value it
// hands it: two programs looking up two hosts with gethostbyname reach one
// resolver, asked once and named alike in both (redis-server's had read
// "Resolver", redis-cli's "System Resolver"). Their connects, to two ends,
// stay two destinations, each asked.
func TestOneServiceBehindAPackageIsOneDestinationInEveryProgram(t *testing.T) {
	call := func(pkg, name string, line int, host string) atlas.SymbolCall {
		return atlas.SymbolCall{Kind: "invokes_external", Name: name, Line: line, Column: 9, API: &atlas.CallAPI{Package: pkg, Name: name},
			SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "literal", Text: host}}}}
	}
	symbol := func(id, path, target string, host string) atlas.Place {
		return atlas.Place{ID: "symbol:" + id, Kind: atlas.PlaceSymbol, Path: path, LineNo: 10, Parent: "file:" + id, TargetIDs: []string{target},
			Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "object:" + id, Name: id}, Calls: []atlas.SymbolCall{call("netdb.h", "gethostbyname", 12, host), call("sys/socket.h", "connect", 13, host)}}}
	}
	server, client := symbol("syncWithMaster", "replication.c", "server", "primary.example"), symbol("cliConnect", "redis-cli.c", "cli", "127.0.0.1")
	var mu sync.Mutex
	resolvers := 0
	provider := &mutatedTableProvider{}
	provider.mutate = func(input map[string]any, rows []map[string]any) {
		mu.Lock()
		defer mu.Unlock()
		if input["table"] == "atlas_systems" {
			for i := range rows {
				rows[i]["system"] = "none"
			}
			return
		}
		named := false
		for _, column := range input["fill"].([]any) {
			named = named || column.(map[string]any)["name"] == "destination"
		}
		for i, source := range input["rows"].([]any) {
			row := source.(map[string]any)
			if !named {
				rows[i]["line"], rows[i]["address"] = "sends", "unknown"
				continue
			}
			rows[i]["destination"] = "other: Server"
			if strings.Contains(string(mustJSON(row)), `"kind":"sdk"`) {
				// Each window names the resolver its own way.
				resolvers++
				rows[i]["destination"] = []string{"other: Resolver", "other: System Resolver"}[min(resolvers-1, 1)]
			}
		}
	}
	r := answerTestReader(t, nil, provider)
	r.opts.Through, r.opts.Graph.Places = "", []atlas.Place{server, client}
	r.opts.Targets = []TargetMeta{{ID: "cli"}, {ID: "server"}}
	r.opts.ReadSource = func(string) ([]byte, error) { return nil, nil }
	r.places = map[string]atlas.Place{server.ID: server, client.ID: client}
	r.knowledge, r.knowledgeSubjects = map[string]*Knowledge{}, map[string]*Knowledge{}
	r.responseTables = map[string]rememberedTable{}
	r.api = map[string]apiRole{"netdb.h.gethostbyname": {talks: atlas.BoundarySDK}, "sys/socket.h.connect": {talks: atlas.BoundaryClientRequest}}
	r.arguments = map[string]ArgumentChoice{"netdb.h.gethostbyname": {Position: 1}, "sys/socket.h.connect": {Position: 1}}
	if err := r.readBoundaries(t.Context()); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, state := range r.boundaries {
		for _, target := range rowTargets(state) {
			got[fmt.Sprintf("%s:%d", state.place.Path, state.place.LineNo)] = state.destinationOf(target)
		}
	}
	want := map[string]string{"redis-cli.c:12": "Resolver", "replication.c:12": "Resolver", "redis-cli.c:13": "Server", "replication.c:13": "Server"}
	if !maps.Equal(got, want) || resolvers != 1 {
		t.Fatalf("destinations = %v, resolver asked %d times; want %v, asked once", got, resolvers, want)
	}
}
