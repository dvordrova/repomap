package lines

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

func TestBoundaryOwnerKeepsOriginalReceiverAndArgumentRoles(t *testing.T) {
	origin := func(name string, column int) *sourcevalue.Value {
		return &sourcevalue.Value{Kind: "field", Text: name, Anchor: &sourcevalue.Anchor{Path: "client.go", Line: 29, Column: column}}
	}
	owner := atlas.Place{ID: "private-owner-place", Path: "client.go", LineNo: 20, Symbol: &atlas.SymbolFacts{
		Decl: atlas.Decl{ObjectID: "private-owner-id", Name: "addComment"},
		Calls: []atlas.SymbolCall{{Kind: "invokes_external", Name: "Discussions.Create", Line: 29, Column: 35,
			API:           &atlas.CallAPI{Package: "example.com/sdk", Receiver: "Discussions", Name: "Create"},
			ReceiverValue: origin("issueTrackerClient", 7), SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: origin("projectID", 40)}},
			CalleeIDs: []string{"private-callee-place"}}},
	}}
	before, _ := json.Marshal(owner)
	projected := BoundaryOwner("o1", owner, []int{29}, nil)
	raw, _ := json.Marshal(projected)
	for _, want := range []string{`"receiver_value":{"kind":"field","text":"issueTrackerClient"}`, `"source_arguments":[{"position":1,"origin":{"kind":"field","text":"projectID"}}]`, `"path":"client.go"`, `"line":29`, `"column":35`, `"package":"example.com/sdk"`, `"ref":"o1"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("boundary owner lost source distinction %s: %s", want, raw)
		}
	}
	// The call keeps its own column; origin nodes travel without anchors.
	if strings.Contains(string(raw), "private-") || strings.Contains(string(raw), `"anchor"`) || strings.Contains(string(raw), `"column":40`) {
		t.Fatalf("boundary provider context exposed internal identities or origin anchors: %s", raw)
	}
	after, _ := json.Marshal(owner)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("boundary projection mutated shared original evidence")
	}
}

func TestBoundaryOwnerKeepsCallsBesideRowsAndClientConstructors(t *testing.T) {
	// Morfeu's owner carried twelve calls in every row; the window's owner
	// keeps the calls within three lines of a row and the calls whose
	// literals are address candidates, once.
	call := func(name string, line int, pkg string, values ...string) atlas.SymbolCall {
		return atlas.SymbolCall{Kind: "invokes_external", Name: name, Line: line, API: &atlas.CallAPI{Package: pkg, Name: name}, Values: values}
	}
	owner := atlas.Place{ID: "owner", Path: "client.go", LineNo: 10, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "publish"}, Calls: []atlas.SymbolCall{
		call("amqp.Dial", 12, "github.com/rabbitmq/amqp091-go", "amqp://broker:5672"),
		call("fmt.Errorf", 14, "fmt", "dial broker: %w"),
		call("time.Now", 30, "time"),
		call("ch.Publish", 40, "github.com/rabbitmq/amqp091-go", "morfeu.events"),
		call("log.Printf", 41, "log", "published %s"),
		call("json.Marshal", 43, "encoding/json"),
		call("os.Getenv", 60, "os", "BROKER_URL"),
		call("strings.TrimSpace", 62, "strings", " x "),
	}}}
	projected := BoundaryOwner("o1", owner, []int{40}, nil)
	raw, _ := json.Marshal(projected["calls"])
	for _, want := range []string{`"amqp.Dial"`, `"ch.Publish"`, `"log.Printf"`, `"json.Marshal"`, `"os.Getenv"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("owner lost a call beside the row or a client constructor %s: %s", want, raw)
		}
	}
	for _, forbidden := range []string{`"fmt.Errorf"`, `"time.Now"`, `"strings.TrimSpace"`} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("owner kept a distant call without address candidates %s: %s", forbidden, raw)
		}
	}
	if projected["call_span"] != OwnerCallSpan {
		t.Fatalf("call span not advertised: %v", projected["call_span"])
	}
}

func TestBoundaryAddressesLeaveOutTemplatesAndFormattingPackages(t *testing.T) {
	call := func(name string, line int, pkg string, values ...string) atlas.SymbolCall {
		return atlas.SymbolCall{Kind: "invokes_external", Name: name, Line: line, API: &atlas.CallAPI{Package: pkg, Name: name}, Values: values}
	}
	owner := atlas.Place{ID: "owner", Path: "client.go", Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "send"}, Calls: []atlas.SymbolCall{
		call("fmt.Errorf", 14, "fmt", "send: %w"),
		call("time.Format", 15, "time", "2006-01-02"),
		call("log.Printf", 16, "log", "sent"),
		call("logrus.Infof", 17, "github.com/sirupsen/logrus", "sent to %s"),
		call("errors.Wrap", 18, "github.com/pkg/errors", "wrapped"),
		call("strconv.Quote", 19, "strconv", "quoted"),
		call("zap.String", 20, "go.uber.org/zap", "url", "node-%03d"),
		call("http.Get", 21, "net/http", "https://api.example/%2Fescaped"),
		{Kind: "invokes_external", Name: "sdk.New", Line: 22, Values: []string{"https://sdk.example"}},
		{Kind: "invokes_external", Name: "strings.Join", Line: 23, Values: []string{"joined"}},
	}}}
	place := atlas.Place{ID: "candidate", Path: "client.go", LineNo: 21, Boundary: &atlas.BoundaryFacts{Direction: atlas.DirectionOut, External: "http.Get", Values: []string{"https://api.example/%2Fescaped", "%(name)s"}}}
	var values []string
	for _, address := range BoundaryAddresses(place, owner) {
		values = append(values, address.Value)
	}
	want := []string{"https://api.example/%2Fescaped", "url", "https://sdk.example"}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("address candidates = %v, want %v", values, want)
	}
	for value, template := range map[string]bool{"%s: %w": true, "node-%-5d": true, "%.2f": true, "%(name)s": true, "https://x/%2F": false, "100%": false, "50%% done": false, "plain": false} {
		if FormatTemplate(value) != template {
			t.Fatalf("FormatTemplate(%q) = %t", value, !template)
		}
	}
}

func TestBoundaryRowAsksAddressOnlyWithCandidatesAndNamesItsOwner(t *testing.T) {
	place := atlas.Place{ID: "candidate", Path: "client.go", LineNo: 21, Boundary: &atlas.BoundaryFacts{
		Direction: atlas.DirectionOut, Caller: "send", CallerDoc: "Sends the request.", External: "http.Client.Do", Values: []string{}}}
	addresses := []BoundaryAddress{{Ref: "a1", Value: "https://api.example"}}
	fields := func(row table.Row) map[string]any {
		result := make(map[string]any)
		for _, field := range row.Fields {
			result[field.Name] = field.Value
		}
		return result
	}
	asked := fields(BoundaryRow(place, "o1", addresses, true))
	if asked["owner_ref"] != "o1" || asked["address_catalog"] == nil || !reflect.DeepEqual(asked["address_options"], []string{"unknown", "a1"}) {
		t.Fatalf("asked row lost its owner ref or catalogue: %+v", asked)
	}
	if _, present := asked["caller_doc"]; present {
		t.Fatalf("caller_doc repeated beside the window's owner: %+v", asked)
	}
	for name, row := range map[string]table.Row{
		"known address": BoundaryRow(place, "o1", addresses, false),
		"no candidates": BoundaryRow(place, "o1", nil, true),
		"fixed incoming": BoundaryRow(atlas.Place{ID: "route", Path: "routes.go", LineNo: 3, Boundary: &atlas.BoundaryFacts{
			Source: "fact", GivenKind: atlas.BoundaryRequest, Direction: atlas.DirectionIn, Method: "GET", Values: []string{"/levels"}}}, "", addresses, false),
	} {
		got := fields(row)
		if got["address_catalog"] != nil || got["address_options"] != nil {
			t.Fatalf("%s: address catalogue rendered where no address decision exists: %+v", name, got)
		}
	}
	ownerless := fields(BoundaryRow(place, "", nil, true))
	if ownerless["caller_doc"] != "Sends the request." || ownerless["owner_ref"] != nil {
		t.Fatalf("row without an owner lost its documentation: %+v", ownerless)
	}
}

func TestBoundaryWindowSharesOwnerAndDestinationsAndDecodesClosedDestination(t *testing.T) {
	owner := atlas.Place{ID: "owner", Path: "client.go", LineNo: 10, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "publish", Doc: "Publishes events."},
		Calls: []atlas.SymbolCall{{Kind: "invokes_external", Name: "ch.Publish", Line: 40, Values: []string{"morfeu.events"}}}}}
	place := func(id string, line int) atlas.Place {
		return atlas.Place{ID: id, Path: "client.go", LineNo: line, Boundary: &atlas.BoundaryFacts{Direction: atlas.DirectionOut, Caller: "publish", External: "ch.Publish", Values: []string{"morfeu.events"}}}
	}
	catalog := Destinations(map[string]string{"github.com/rabbitmq/amqp091-go": "RabbitMQ", "net/http": ""})
	shared := append([]table.Field{BoundaryOwnerContext(BoundaryOwner("o1", owner, []int{40, 41}, nil))}, DestinationFields(catalog)...)
	rows := []table.Row{
		BoundaryRow(place("first", 40), "o1", BoundaryAddresses(place("first", 40), owner), true),
		BoundaryRow(place("second", 41), "o1", nil, true),
	}
	def := FixedBoundaries(true)
	windows, err := table.WindowsWithContext(def, 1, shared, rows)
	if err != nil || len(windows) != 1 {
		t.Fatalf("windows: %d, %v", len(windows), err)
	}
	request := string(windows[0].Request)
	if strings.Count(request, `"Publishes events."`) != 1 || strings.Count(request, `"destination_catalog"`) != 1 || strings.Count(request, `"owner_ref": "o1"`) != 2 {
		t.Fatalf("owner or catalogue not shared once per window:\n%s", request)
	}
	if !strings.Contains(request, `"destination_catalog": [{"ref":"d1","value":"RabbitMQ","packages":["github.com/rabbitmq/amqp091-go"]}]`) {
		t.Fatalf("catalogue lost its named package or offered an unnamed one:\n%s", request)
	}
	result, err := table.DecodeResult(def, windows[0], []byte(`{"rows":[
		{"key":"first","line":"publishes catalog events to morfeu.events","destination":"d1","address":"a1"},
		{"key":"second","line":"notifies the operator","destination":"other: Twilio"}]}`))
	if err != nil || len(result.Rejections) != 0 {
		t.Fatalf("closed destination refused: %+v / %v", result, err)
	}
	if result.Answers[0]["destination"] != "d1" || DestinationValue(catalog, result.Answers[0]["destination"]) != "RabbitMQ" || result.Answers[0]["address"] != "a1" {
		t.Fatalf("ref did not resolve to the listed system: %+v", result.Answers[0])
	}
	if free, ok := table.IsFree(def.Columns[1], result.Answers[1]["destination"]); !ok || free != "Twilio" {
		t.Fatalf("free destination lost: %+v", result.Answers[1])
	}
	if _, asked := result.Answers[1]["address"]; asked {
		t.Fatalf("a row without candidates acquired an address cell: %+v", result.Answers[1])
	}
	refused, err := table.DecodeResult(def, windows[0], []byte(`{"rows":[
		{"key":"first","line":"publishes","destination":"RabbitMQ broker","address":"a1"},
		{"key":"second","line":"notifies","destination":"other: Twilio"}]}`))
	if err != nil || len(refused.Rejections) != 1 || refused.Rejections[0].Key != "first" || refused.Answers[1]["destination"] != "other: Twilio" {
		t.Fatalf("free text without the prefix was accepted or its neighbour lost: %+v / %v", refused, err)
	}
}

func TestFixedBoundaryRequestAsksForProseNotNativeExistenceOrKind(t *testing.T) {
	for _, kind := range []string{atlas.BoundaryConfig, atlas.BoundaryRequest, atlas.BoundaryClientRequest, atlas.BoundaryListenAddress} {
		t.Run(kind, func(t *testing.T) {
			outgoing := kind == atlas.BoundaryClientRequest
			place := atlas.Place{ID: "native", Path: "service.py", LineNo: 12, Boundary: &atlas.BoundaryFacts{
				Source: "fact", GivenKind: kind, Direction: atlas.DirectionOut, Values: []string{"SOURCE_VALUE", "OTHER_VALUE"}}}
			addresses := BoundaryAddresses(place)
			row := BoundaryRow(place, "", addresses, outgoing)
			def := FixedBoundaries(outgoing)
			var shared []table.Field
			if outgoing {
				shared = DestinationFields(Destinations(map[string]string{"example.com/client": "Example service"}))
			}
			windows, err := table.WindowsWithContext(def, 1, shared, []table.Row{row})
			if err != nil || len(windows) != 1 {
				t.Fatalf("fixed request: %v", err)
			}
			var request struct {
				Fill []struct{ Name string }
				Rows []map[string]any
			}
			if err := json.Unmarshal(windows[0].Request, &request); err != nil {
				t.Fatal(err)
			}
			var columns []string
			for _, column := range request.Fill {
				columns = append(columns, column.Name)
			}
			want := []string{"line"}
			if outgoing {
				want = append(want, "destination", "address")
			} else {
				// An incoming entry is named from its words; this row wrote
				// none, so its name is not asked.
				want = append(want, "name")
			}
			if !reflect.DeepEqual(columns, want) || request.Rows[0]["kind_given"] != kind || request.Rows[0]["decision_options"] != nil || request.Rows[0]["kind_options"] != nil {
				t.Fatalf("native property became a model decision: %s", windows[0].Request)
			}
			if (request.Rows[0]["address_catalog"] != nil) != outgoing {
				t.Fatalf("address catalogue presence does not follow the asked cells: %s", windows[0].Request)
			}
			result, err := table.DecodeResult(def, windows[0], []byte(`{"rows":[{"key":"native","line":"Reads the configured value.","decision":"none","kind":"invented","basis":"configuration","destination":"d1","address":"unknown"}]}`))
			answered := len(want)
			if !outgoing {
				answered--
			}
			if err != nil || len(result.Rejections) != 0 || len(result.Answers[0]) != answered || result.Answers[0]["decision"] != "" || result.Answers[0]["kind"] != "" {
				t.Fatalf("unrequested cells acquired authority: %+v / %v", result, err)
			}
			if outgoing {
				// An unlisted address loses only the address: the line and the
				// destination stand, and no address is taken.
				bad, err := table.DecodeResult(def, windows[0], []byte(`{"rows":[{"key":"native","line":"Sends a request.","destination":"d1","address":"a999"}]}`))
				if err != nil || len(bad.Rejections) != 1 || bad.Rejections[0].Cell != "address" || !strings.Contains(bad.Rejections[0].Reason, "a999") {
					t.Fatalf("an unlisted address was not refused alone: %+v / %v", bad, err)
				}
				if _, taken := bad.Answers[0]["address"]; taken || bad.Answers[0]["line"] != "Sends a request." || bad.Answers[0]["destination"] != "d1" || len(bad.AcceptedRowKeys()) != 0 {
					t.Fatalf("fixed fact weakened closed address refs: %+v", bad)
				}
				// A row whose every cell fails is still refused.
				if refused, err := table.DecodeResult(def, windows[0], []byte(`{"rows":[{"key":"native","line":"","destination":"d999","address":"a999"}]}`)); err == nil || refused.Answers[0] != nil {
					t.Fatalf("a row without one valid cell was accepted: %+v / %v", refused, err)
				}
			}
		})
	}
}

// An entry is named by the words its registration wrote, as the model
// chooses them: restored as written, in the order it wrote them, whatever
// protocol they belong to. A listener names no entry.
func TestEntryNamesAreChosenWordsRestoredAsWritten(t *testing.T) {
	entry := func(kind string, words ...string) atlas.Place {
		return atlas.Place{ID: "b1", Path: "server.go", LineNo: 3, Boundary: &atlas.BoundaryFacts{
			Source: "fact", Direction: atlas.DirectionIn, GivenKind: kind, Words: words}}
	}
	for _, tc := range []struct {
		place atlas.Place
		cell  string
		want  string
	}{
		{entry(atlas.BoundaryRequest, "GET", "/users/:id"), "w1 w2", "GET /users/:id"},
		{entry(atlas.BoundaryRequest, "get", "/items"), "w1 w2", "get /items"},
		{entry(atlas.BoundaryRequest, "redisCommand", "get"), "w2", "get"},
		{entry(atlas.BoundaryRequest, "Handle", "POST", "/items"), "w2 w3", "POST /items"},
		{entry(atlas.BoundaryQueueConsumer, "subscribe", "orders.created"), "w2", "orders.created"},
		{entry(atlas.BoundaryRequest, "RegisterService", "billing.Invoices", "Create"), "w2 w3", "billing.Invoices Create"},
		{entry(atlas.BoundaryRequest, "on", "chat message"), "w2", "chat message"},
		// A word that cannot stand in a one-line name as written is not
		// offered, and never trimmed into one: w2 is the next word.
		{entry(atlas.BoundaryRequest, "on", " padded ", "line\nbreak", "join"), "w2", "join"},
		{entry(atlas.BoundaryRequest, "GET", "/users/:id"), "w2 w1", "/users/:id GET"},
		{entry(atlas.BoundaryContinuous, "pthread_create"), "", ""},
		{entry(atlas.BoundaryRequest, "GET", "/users/:id"), "w3 w1", "GET"},
	} {
		if got := EntryName(EntryWords(tc.place), tc.cell); got != tc.want {
			t.Fatalf("%v chose %q: %q, want %q", tc.place.Boundary.Words, tc.cell, got, tc.want)
		}
	}
	listener := entry(atlas.BoundaryListenAddress, "Start", ":8080")
	if words := EntryWords(listener); len(words) != 0 {
		t.Fatalf("a listener was offered a name: %+v", words)
	}
	row := BoundaryRow(entry(atlas.BoundaryRequest, "GET", "/users/:id"), "", nil, false)
	fields := map[string]any{}
	for _, field := range row.Fields {
		fields[field.Name] = field.Value
	}
	if fields["method"] != nil || fields["values"] != nil || !reflect.DeepEqual(fields["word_options"], []string{"w1", "w2"}) {
		t.Fatalf("an entry row shows a method or values beside its words: %+v", fields)
	}
}
