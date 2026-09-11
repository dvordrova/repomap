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
	place := atlas.Place{ID: "private-boundary-place", Path: "client.go", LineNo: 29,
		Boundary: &atlas.BoundaryFacts{ObjectID: "private-owner-id", Direction: atlas.DirectionOut, Caller: "addComment"}}
	before, _ := json.Marshal(owner)
	row := BoundaryRow(place, "", owner)
	var projected any
	for _, field := range row.Fields {
		if field.Name == "owner" {
			projected = field.Value
		}
	}
	raw, _ := json.Marshal(projected)
	for _, want := range []string{`"receiver_value":{"kind":"field","text":"issueTrackerClient"}`, `"source_arguments":[{"position":1,"origin":{"kind":"field","text":"projectID"}}]`, `"path":"client.go"`, `"line":29`, `"column":35`, `"package":"example.com/sdk"`} {
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

func TestFixedBoundaryRequestAsksForProseNotNativeExistenceOrKind(t *testing.T) {
	for _, kind := range []string{atlas.BoundaryConfig, atlas.BoundaryHTTPServer, atlas.BoundaryHTTPClient, atlas.BoundaryListenAddress} {
		t.Run(kind, func(t *testing.T) {
			outgoing := kind == atlas.BoundaryHTTPClient
			place := atlas.Place{ID: "native", Path: "service.py", LineNo: 12, Boundary: &atlas.BoundaryFacts{
				Source: "fact", GivenKind: kind, Direction: atlas.DirectionOut, Values: []string{"SOURCE_VALUE"}}}
			row := BoundaryRow(place, "")
			def := FixedBoundaries(outgoing)
			windows, err := table.Windows(def, 1, []table.Row{row})
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
			result, err := table.DecodeResult(def, windows[0], []byte(`{"rows":[{"key":"r1","line":"Reads the configured value.","decision":"none","kind":"invented","basis":"configuration","destination":"Peer service","address":"unknown"}]}`))
			if err != nil || len(result.Rejections) != 0 || len(result.Answers[0]) != len(want) || result.Answers[0]["decision"] != "" || result.Answers[0]["kind"] != "" {
				t.Fatalf("unrequested cells acquired authority: %+v / %v", result, err)
			}
			if outgoing {
				bad, err := table.DecodeResult(def, windows[0], []byte(`{"rows":[{"key":"r1","line":"Sends a request.","destination":"Peer","address":"a999"}]}`))
				if err == nil || len(bad.Rejections) != 1 || !strings.Contains(bad.Rejections[0].Reason, "address") {
					t.Fatalf("fixed fact weakened closed address refs: %+v / %v", bad, err)
				}
			}
		})
	}
}
