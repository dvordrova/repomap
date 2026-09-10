package lines

import (
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/table"
)

func TestOperationDecisionOwnsRequiredCells(t *testing.T) {
	rows := make([]table.Row, 7)
	for i := range rows {
		rows[i].Fields = []table.Field{{Name: "entry_options", Value: []string{"self", "none", "u1"}}}
	}
	result, err := table.DecodeResult(Operations(), table.Window{Rows: rows}, []byte(`{"rows":[
		{"key":"r1","entry":"none","activation":"none","name":"","description":""},
		{"key":"r2","entry":"u1"},
		{"key":"r3","entry":"none","name":42,"description":{}},
		{"key":"r4","entry":"self","activation":"continuous","name":"worker","description":"Consumes updates until cancellation."},
		{"key":"r5","entry":"self","activation":"continuous","name":"","description":"Runs."},
		{"key":"r6","entry":"u2"},
		{"key":"r7","entry":"self","activation":"none","name":"worker","description":"Runs."}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	for i, entry := range []string{"none", "u1", "none"} {
		if len(result.Answers[i]) != 1 || result.Answers[i]["entry"] != entry {
			t.Fatalf("negative decision lost or unused cells retained: %#v", result.Answers[i])
		}
	}
	if result.Answers[3]["name"] != "worker" || len(result.Rejections) != 3 {
		t.Fatalf("self or rejection scope changed: %#v", result)
	}
	for _, i := range []int{4, 5, 6} {
		if result.Answers[i] != nil {
			t.Fatalf("invalid operation accepted: %#v", result.Answers[i])
		}
	}
}

func TestNegativeOperationWindowIsAccepted(t *testing.T) {
	result, err := table.DecodeResult(Operations(), table.Window{Rows: []table.Row{{Fields: []table.Field{{Name: "entry_options", Value: []string{"self", "none"}}}}}}, []byte(`{"rows":[{"key":"r1","entry":"none"}]}`))
	if err != nil || len(result.AcceptedRowKeys()) != 1 {
		t.Fatalf("negative window refused: %#v, %v", result, err)
	}
}
