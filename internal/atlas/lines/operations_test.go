package lines

import (
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/table"
)

func TestOperationDecisionOwnsRequiredCells(t *testing.T) {
	rows := make([]table.Row, 7)
	for i := range rows {
		rows[i].Fields = []table.Field{{Name: "name_kind_options", Value: []string{"label"}}}
	}
	result, err := table.DecodeResult(Operations(), table.Window{Rows: rows}, []byte(`{"rows":[
		{"key":"r1","entry":"none","activation":"none","name":"","description":""},
		{"key":"r2","entry":"u1"},
		{"key":"r3","entry":"none","name":42,"description":{}},
		{"key":"r4","entry":"self","name_kind":"label","activation":"continuous","name":"worker","description":"Consumes updates until cancellation."},
		{"key":"r5","entry":"self","name_kind":"label","activation":"continuous","name":"","description":"Runs."},
		{"key":"r6","entry":"u2"},
		{"key":"r7","entry":"self","name_kind":"label","activation":"none","name":"worker","description":"Runs."}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range []int{0, 2} {
		if len(result.Answers[i]) != 1 || result.Answers[i]["entry"] != "none" {
			t.Fatalf("negative decision lost or unused cells retained: %#v", result.Answers[i])
		}
	}
	if result.Answers[3]["name"] != "worker" || len(result.Rejections) != 4 {
		t.Fatalf("self or rejection scope changed: %#v", result)
	}
	for _, i := range []int{1, 4, 5, 6} {
		if result.Answers[i] != nil {
			t.Fatalf("invalid operation accepted: %#v", result.Answers[i])
		}
	}
}

func TestNegativeOperationWindowIsAccepted(t *testing.T) {
	result, err := table.DecodeResult(Operations(), table.Window{Rows: []table.Row{{}}}, []byte(`{"rows":[{"key":"r1","entry":"none"}]}`))
	if err != nil || len(result.AcceptedRowKeys()) != 1 {
		t.Fatalf("negative window refused: %#v, %v", result, err)
	}
}

func TestHTTPNamesSelectClosedRegistrationWithoutFreeText(t *testing.T) {
	row := table.Row{Fields: []table.Field{
		{Name: "name_kind_options", Value: []string{"label", "http"}},
		{Name: "registered_name_options", Value: []string{"p1"}},
	}}
	result, err := table.DecodeResult(Operations(), table.Window{Rows: []table.Row{row, row, row}}, []byte(`{"rows":[
		{"key":"r1","entry":"self","activation":"request","name_kind":"http","http_method":"GET","http_path":"p1","name":"/invented","description":"Returns a greeting."},
		{"key":"r2","entry":"self","activation":"request","name_kind":"http","http_method":"GET","http_path":"/invented","description":"Returns a greeting."},
		{"key":"r3","entry":"none","name_kind":42,"http_path":[]}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Answers[0]["http_path"] != "p1" || result.Answers[0]["name"] != "" ||
		result.Answers[1] != nil || len(result.Answers[2]) != 1 || len(result.Rejections) != 1 {
		t.Fatalf("HTTP ref validation or independent negative decision changed: %#v", result)
	}
}
