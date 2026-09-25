package lines

import (
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/table"
)

// One unlisted value on an optional API decision loses that decision only:
// the symbol's other decisions stand. A part's required role is still one of
// its listed roles or nothing.
func TestAPIDecisionsFailAloneWhileTheCoreRoleIsRequired(t *testing.T) {
	window := table.Window{Rows: []table.Row{{ID: "x1"}, {ID: "x2"}}}
	result, err := table.DecodeResult(API(false), window, []byte(`{"rows":[
		{"key":"x1","publishes":"yes","talks":"grpc","reads_input":"body","validates":true},
		{"key":"x2","talks":"db"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if want := (table.Answer{"publishes": "yes", "reads_input": "body", "validates": "yes"}); !reflect.DeepEqual(result.Answers[0], want) {
		t.Fatalf("one unlisted decision cost the symbol its others: %+v", result.Answers[0])
	}
	if len(result.Rejections) != 1 || result.Rejections[0].Key != "x1" || result.Rejections[0].Cell != "talks" || result.Answers[1]["talks"] != "db" {
		t.Fatalf("the unlisted decision was not recorded alone: %+v", result.Rejections)
	}
	core, err := table.DecodeResult(Core(), table.Window{Rows: []table.Row{{ID: "p1"}, {ID: "p2"}}}, []byte(`{"rows":[{"key":"p1","role":"storage"},{"key":"p2","role":"domain."}]}`))
	if err != nil || core.Answers[0] != nil || core.Answers[1]["role"] != PartDomain {
		t.Fatalf("an unlisted core role was accepted or a punctuated one refused: %+v / %v", core, err)
	}
}

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
}
