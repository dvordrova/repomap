package lines

import (
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/table"
)

// A missing address is the declared unknown and names no address; the line
// and destination stand. An unlisted destination, or a free destination
// without its name, loses only itself.
func TestOutboundAddressMissingReadsUnknown(t *testing.T) {
	def := FixedBoundaries(true)
	window := table.Window{
		Context: []table.Field{{Name: "destination_options", Value: []string{"d1"}}},
		Rows: []table.Row{
			{ID: "b1", Fields: []table.Field{{Name: "address_options", Value: []string{"unknown", "a1"}}}},
			{ID: "b2", Fields: []table.Field{{Name: "address_options", Value: []string{"unknown", "a1"}}}},
		},
	}
	result, err := table.DecodeResult(def, window, []byte(`{"rows":[
		{"key":"b1","line":"Sends the order.","destination":"d1"},
		{"key":"b2","line":"Sends the order.","destination":"d7","address":"a1"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if want := (table.Answer{"line": "Sends the order.", "destination": "d1", "address": "unknown"}); !reflect.DeepEqual(result.Answers[0], want) || len(result.AcceptedRowKeys()) != 1 {
		t.Fatalf("a missing address refused its row or chose one: %+v", result.Answers[0])
	}
	if want := (table.Answer{"line": "Sends the order.", "address": "a1"}); !reflect.DeepEqual(result.Answers[1], want) || len(result.Rejections) != 1 || result.Rejections[0].Cell != "destination" {
		t.Fatalf("an unlisted destination was not refused alone: %+v / %+v", result.Answers[1], result.Rejections)
	}
	free, err := table.DecodeResult(def, table.Window{Context: window.Context, Rows: window.Rows[:1]}, []byte(`{"rows":[{"key":"b1","line":"Sends the order.","destination":"other: ","address":"a1"}]}`))
	if want := (table.Answer{"line": "Sends the order.", "address": "a1"}); err != nil || !reflect.DeepEqual(free.Answers[0], want) || len(free.Rejections) != 1 || free.Rejections[0].Cell != "destination" {
		t.Fatalf("a free destination without its name was not refused alone: %+v / %v", free, err)
	}
	// The unknown a missing address reads as is no answer of its own: a row
	// that wrote nothing valid is refused, not remembered as answered, while
	// an address the model wrote still keeps its row.
	for _, raw := range []string{`{"rows":[{"key":"b1"}]}`, `{"rows":[{"key":"b1","line":"","destination":"d9"}]}`, `{"rows":[{"key":"b1","line":" ","destination":null,"address":null}]}`} {
		if refused, err := table.DecodeResult(def, table.Window{Context: window.Context, Rows: window.Rows[:1]}, []byte(raw)); err == nil || refused.Answers[0] != nil {
			t.Fatalf("a row with only its missing-address value was accepted: %s -> %+v", raw, refused.Answers)
		}
	}
	written, err := table.DecodeResult(def, table.Window{Context: window.Context, Rows: window.Rows[:1]}, []byte(`{"rows":[{"key":"b1","line":"","destination":"d9","address":"unknown"}]}`))
	if want := (table.Answer{"address": "unknown"}); err != nil || !reflect.DeepEqual(written.Answers[0], want) {
		t.Fatalf("a written address lost its row: %+v / %v", written, err)
	}
}
