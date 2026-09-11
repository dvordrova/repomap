package lines

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/destinations"
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
	for _, want := range []string{`"receiver_value"`, `"source_arguments"`, `"issueTrackerClient"`, `"projectID"`, `"path":"client.go"`, `"line":29`, `"column":40`, `"package":"example.com/sdk"`, `"ref":"o1"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("boundary owner lost source distinction %s: %s", want, raw)
		}
	}
	if strings.Contains(string(raw), "private-") {
		t.Fatalf("boundary provider context exposed internal identities: %s", raw)
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
			Source: "fact", GivenKind: atlas.BoundaryHTTPServer, Direction: atlas.DirectionIn, Method: "GET", Values: []string{"/levels"}}}, "", addresses, false),
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

func TestBoundaryDefinitionsCarryClosedListsAsColumnOptions(t *testing.T) {
	out := Boundaries(true)
	names := map[string]table.Column{}
	for _, column := range out.Columns {
		names[column.Name] = column
	}
	if !reflect.DeepEqual(names["decision"].Options, []string{"boundary", "none", "unassessed"}) || !reflect.DeepEqual(names["kind"].Options, atlas.OutgoingBoundaryKinds()) {
		t.Fatalf("closed lists are not column options: %+v", names)
	}
	for _, kind := range names["kind"].Options {
		if kind == atlas.BoundaryHTTPServer || kind == atlas.BoundaryConfig {
			t.Fatalf("an outgoing candidate may choose a kind the group index drops: %v", names["kind"].Options)
		}
	}
	if names["line"].Kind != table.Text || names["line"].MaxRunes != ShortLineRunes {
		t.Fatalf("outgoing line is not a bounded text cell: %+v", names["line"])
	}
	if names["destination"].OptionsFrom != "destination_options" || names["destination"].Free != DestinationOther {
		t.Fatalf("destination is not a closed choice with a free prefix: %+v", names["destination"])
	}
	if names["address"].WhenOptionsFrom != "address_options" {
		t.Fatalf("address cell is asked without candidates: %+v", names["address"])
	}
	if !strings.HasPrefix(out.Contract, "repomap.atlas.boundaries.v7") || !strings.HasPrefix(FixedBoundaries(true).Contract, "repomap.atlas.boundaries.v7.fixed") {
		t.Fatalf("contract not raised: %s / %s", out.Contract, FixedBoundaries(true).Contract)
	}
	if FixedBoundaries(false).System == Boundaries().System || strings.Count(FixedBoundaries(false).System, "\n") > 25 {
		t.Fatal("fixed facts share the candidate prompt or the fixed prompt is not short")
	}
	if !reflect.DeepEqual(Boundaries().Columns[1].Options, atlas.BoundaryKinds()) {
		t.Fatalf("incoming candidates lost the full kind list: %v", Boundaries().Columns[1].Options)
	}
}

func TestBoundaryWindowSharesOwnerAndDestinationsAndDecodesClosedDestination(t *testing.T) {
	owner := atlas.Place{ID: "owner", Path: "client.go", LineNo: 10, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "publish", Doc: "Publishes events."},
		Calls: []atlas.SymbolCall{{Kind: "invokes_external", Name: "ch.Publish", Line: 40, Values: []string{"morfeu.events"}}}}}
	place := func(id string, line int) atlas.Place {
		return atlas.Place{ID: id, Path: "client.go", LineNo: line, Boundary: &atlas.BoundaryFacts{Direction: atlas.DirectionOut, Caller: "publish", External: "ch.Publish", Values: []string{"morfeu.events"}}}
	}
	catalog := destinations.Catalog([]string{"github.com/rabbitmq/amqp091-go"})
	shared := append([]table.Field{BoundaryOwnerContext(BoundaryOwner("o1", owner, []int{40, 41}, nil))}, DestinationFields(catalog)...)
	rows := []table.Row{
		BoundaryRow(place("first", 40), "o1", BoundaryAddresses(place("first", 40), owner), true),
		BoundaryRow(place("second", 41), "o1", nil, true),
	}
	def := Boundaries(true)
	windows, err := table.WindowsWithContext(def, 1, shared, rows)
	if err != nil || len(windows) != 1 {
		t.Fatalf("windows: %d, %v", len(windows), err)
	}
	request := string(windows[0].Request)
	if strings.Count(request, `"Publishes events."`) != 1 || strings.Count(request, `"destination_catalog"`) != 1 || strings.Count(request, `"owner_ref": "o1"`) != 2 {
		t.Fatalf("owner or catalogue not shared once per window:\n%s", request)
	}
	if !strings.Contains(request, `"dependencies":["github.com/rabbitmq/amqp091-go"]`) {
		t.Fatalf("catalogue lost the target's dependency annotation:\n%s", request)
	}
	result, err := table.DecodeResult(def, windows[0], []byte(`{"rows":[
		{"key":"r1","decision":"boundary","kind":"queue_producer","line":"publishes catalog events to morfeu.events","destination":"d1","basis":"dispatch","address":"a1"},
		{"key":"r2","decision":"boundary","kind":"sdk","line":"notifies the operator","destination":"other: Twilio","basis":"dispatch"}]}`))
	if err != nil || len(result.Rejections) != 0 {
		t.Fatalf("closed destination refused: %+v / %v", result, err)
	}
	if result.Answers[0]["destination"] != "d1" || destinations.Value(catalog, result.Answers[0]["destination"]) != "RabbitMQ" || result.Answers[0]["address"] != "a1" {
		t.Fatalf("ref did not resolve to the listed system: %+v", result.Answers[0])
	}
	if free, ok := table.IsFree(def.Columns[3], result.Answers[1]["destination"]); !ok || free != "Twilio" {
		t.Fatalf("free destination lost: %+v", result.Answers[1])
	}
	if _, asked := result.Answers[1]["address"]; asked {
		t.Fatalf("a row without candidates acquired an address cell: %+v", result.Answers[1])
	}
	refused, err := table.DecodeResult(def, windows[0], []byte(`{"rows":[
		{"key":"r1","decision":"boundary","kind":"queue_producer","line":"publishes","destination":"RabbitMQ broker","basis":"dispatch","address":"a1"},
		{"key":"r2","decision":"none","kind":"","line":"","destination":"","basis":""}]}`))
	if err != nil || len(refused.Rejections) != 1 || refused.Rejections[0].Key != "r1" || refused.Answers[1]["decision"] != "none" {
		t.Fatalf("free text without the prefix was accepted or a negative row lost: %+v / %v", refused, err)
	}
}

func TestFixedBoundaryRequestAsksForProseNotNativeExistenceOrKind(t *testing.T) {
	for _, kind := range []string{atlas.BoundaryConfig, atlas.BoundaryHTTPServer, atlas.BoundaryHTTPClient, atlas.BoundaryListenAddress} {
		t.Run(kind, func(t *testing.T) {
			outgoing := kind == atlas.BoundaryHTTPClient
			place := atlas.Place{ID: "native", Path: "service.py", LineNo: 12, Boundary: &atlas.BoundaryFacts{
				Source: "fact", GivenKind: kind, Direction: atlas.DirectionOut, Values: []string{"SOURCE_VALUE", "OTHER_VALUE"}}}
			addresses := BoundaryAddresses(place)
			row := BoundaryRow(place, "", addresses, outgoing)
			def := FixedBoundaries(outgoing)
			var shared []table.Field
			if outgoing {
				shared = DestinationFields(destinations.Catalog(nil))
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
			}
			if !reflect.DeepEqual(columns, want) || request.Rows[0]["kind_given"] != kind || request.Rows[0]["decision_options"] != nil || request.Rows[0]["kind_options"] != nil {
				t.Fatalf("native property became a model decision: %s", windows[0].Request)
			}
			if (request.Rows[0]["address_catalog"] != nil) != outgoing {
				t.Fatalf("address catalogue presence does not follow the asked cells: %s", windows[0].Request)
			}
			result, err := table.DecodeResult(def, windows[0], []byte(`{"rows":[{"key":"r1","line":"Reads the configured value.","decision":"none","kind":"invented","basis":"configuration","destination":"d1","address":"unknown"}]}`))
			if err != nil || len(result.Rejections) != 0 || len(result.Answers[0]) != len(want) || result.Answers[0]["decision"] != "" || result.Answers[0]["kind"] != "" {
				t.Fatalf("unrequested cells acquired authority: %+v / %v", result, err)
			}
			if outgoing {
				bad, err := table.DecodeResult(def, windows[0], []byte(`{"rows":[{"key":"r1","line":"Sends a request.","destination":"d1","address":"a999"}]}`))
				if err == nil || len(bad.Rejections) != 1 || !strings.Contains(bad.Rejections[0].Reason, "address") {
					t.Fatalf("fixed fact weakened closed address refs: %+v / %v", bad, err)
				}
			}
		})
	}
}
