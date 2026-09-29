package lines

import (
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/table"
)

// A missing address is the declared unknown and names no address; the line
// stands. What the call reaches is not the row's cell: a destination is
// named once for all of its calls.
func TestOutboundAddressMissingReadsUnknown(t *testing.T) {
	def := FixedBoundaries(true)
	window := table.Window{
		Rows: []table.Row{
			{ID: "b1", Fields: []table.Field{{Name: "address_options", Value: []string{"unknown", "a1"}}}},
			{ID: "b2", Fields: []table.Field{{Name: "address_options", Value: []string{"unknown", "a1"}}}},
		},
	}
	result, err := table.DecodeResult(def, window, []byte(`{"rows":[
		{"key":"b1","line":"Sends the order.","destination":"d1"},
		{"key":"b2","line":"Sends the order.","address":"a7"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if want := (table.Answer{"line": "Sends the order.", "address": "unknown"}); !reflect.DeepEqual(result.Answers[0], want) || len(result.AcceptedRowKeys()) != 1 {
		t.Fatalf("a missing address refused its row, chose one or kept a destination: %+v", result.Answers[0])
	}
	if want := (table.Answer{"line": "Sends the order."}); !reflect.DeepEqual(result.Answers[1], want) || len(result.Rejections) != 1 || result.Rejections[0].Cell != "address" {
		t.Fatalf("an unlisted address was not refused alone: %+v / %+v", result.Answers[1], result.Rejections)
	}
	// The unknown a missing address reads as is no answer of its own: a row
	// that wrote nothing valid is refused, not remembered as answered, while
	// an address the model wrote still keeps its row.
	for _, raw := range []string{`{"rows":[{"key":"b1"}]}`, `{"rows":[{"key":"b1","line":""}]}`, `{"rows":[{"key":"b1","line":" ","address":null}]}`} {
		if refused, err := table.DecodeResult(def, table.Window{Rows: window.Rows[:1]}, []byte(raw)); err == nil || refused.Answers[0] != nil {
			t.Fatalf("a row with only its missing-address value was accepted: %s -> %+v", raw, refused.Answers)
		}
	}
	written, err := table.DecodeResult(def, table.Window{Rows: window.Rows[:1]}, []byte(`{"rows":[{"key":"b1","line":"","address":"unknown"}]}`))
	if want := (table.Answer{"address": "unknown"}); err != nil || !reflect.DeepEqual(written.Answers[0], want) {
		t.Fatalf("a written address lost its row: %+v / %v", written, err)
	}
}

// A destination's one cell: an unlisted ref, or a free name without its
// name, refuses that destination alone; its neighbours stand.
func TestDestinationNameRefusesAnUnlistedOrEmptyNameAlone(t *testing.T) {
	def := DestinationNames()
	window := table.Window{
		Context: []table.Field{{Name: "destination_options", Value: []string{"d1"}}},
		Rows:    []table.Row{{ID: "g1"}, {ID: "g2"}, {ID: "g3"}},
	}
	result, err := table.DecodeResult(def, window, []byte(`{"rows":[
		{"key":"g1","destination":"d1"},
		{"key":"g2","destination":"d7"},
		{"key":"g3","destination":"other: "}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Answers[0]["destination"] != "d1" || result.Answers[1] != nil || result.Answers[2] != nil || len(result.Rejections) != 2 {
		t.Fatalf("an unlisted or empty destination was not refused alone: %+v / %+v", result.Answers, result.Rejections)
	}
}
