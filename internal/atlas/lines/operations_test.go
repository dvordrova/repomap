package lines

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/table"
)

func TestOperationDecisionOwnsRequiredCells(t *testing.T) {
	rows := make([]table.Row, 8)
	result, err := table.DecodeResult(Operations(), table.Window{Rows: rows}, []byte(`{"rows":[
		{"key":"r1","entry":"none","activation":"none","name":"","description":""},
		{"key":"r2","entry":"u1"},
		{"key":"r3","entry":"none","name":42,"description":{}},
		{"key":"r4","entry":"self","name_kind":"label","activation":"continuous","name":"worker","description":"Consumes updates until cancellation."},
		{"key":"r5","entry":"self","name_kind":"label","activation":"continuous","name":null,"description":"Runs periodic event sending batches until context cancellation. Then stops."},
		{"key":"r6","entry":"u2"},
		{"key":"r7","entry":"self","name_kind":"label","activation":"none","name":"worker","description":"Runs."},
		{"key":"r8","entry":"self","name_kind":"label","activation":"continuous","name":"","description":""}
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
	// A provider that fills every schema key returns null for a label the
	// model skipped; the row keeps its decision under its own description.
	if got := result.Answers[4]; got["name"] != "Runs periodic event sending batches until context…" || got["name_from"] != "description" || got["activation"] != "continuous" {
		t.Fatalf("null label did not take the row's own description: %#v", got)
	}
	for _, i := range []int{1, 5, 6, 7} {
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

// A row without registered names has one name kind. It is not asked: the
// decoder reads label and requires the name, whatever the model wrote, and a
// row with registered names that omits the cell reads label the same way.
func TestNameKindIsAskedOnlyWithRegisteredNames(t *testing.T) {
	def := Operations()
	plain := table.Row{Fields: []table.Field{{Name: "path", Value: "worker.go"}}}
	registered := table.Row{Fields: []table.Field{{Name: "registered_name_options", Value: []string{"p1"}}}}
	window := table.Window{Rows: []table.Row{plain, registered, plain, registered}}
	request, err := table.Request(def, window)
	if err != nil || !strings.Contains(string(request), `"when_options_nonempty":"registered_name_options"`) || strings.Contains(string(request), "name_kind_options") {
		t.Fatalf("name_kind is not conditioned on registered names: %s %v", request, err)
	}
	result, err := table.DecodeResult(def, window, []byte(`{"rows":[
		{"key":"r1","entry":"self","activation":"continuous","name":"Send queued notifications","description":"Sends queued notifications until shutdown."},
		{"key":"r2","entry":"self","activation":"request","name_kind":"http","http_method":"GET","http_path":"p1","description":"Returns the status."},
		{"key":"r3","entry":"self","activation":"request","name_kind":"http","http_method":"GET","http_path":"p1","description":"Returns the status of one job. Then logs."},
		{"key":"r4","entry":"self","activation":"request","http_method":"GET","http_path":"p1","description":"Returns the status."}
	]}`))
	if err != nil || len(result.Rejections) != 0 {
		t.Fatalf("unasked name kind refused a row: %+v %v", result, err)
	}
	if got := result.Answers[0]; got["name_kind"] != "label" || got["name"] != "Send queued notifications" || got["name_from"] != "" {
		t.Fatalf("a row without registered names did not read label with its own name: %#v", got)
	}
	if got := result.Answers[1]; got["name_kind"] != "http" || got["http_path"] != "p1" || got["http_method"] != "GET" || got["name"] != "" {
		t.Fatalf("an asked and answered http kind changed: %#v", got)
	}
	if got := result.Answers[2]; got["name_kind"] != "label" || got["http_path"] != "" || got["http_method"] != "" || got["name"] != "Returns the status of one job" || got["name_from"] != "description" {
		t.Fatalf("http written for a row without registered names was not read as label: %#v", got)
	}
	if got := result.Answers[3]; got["name_kind"] != "label" || got["http_path"] != "" || got["name"] != "Returns the status" || got["name_from"] != "description" {
		t.Fatalf("an omitted name kind beside registered names was not read as label: %#v", got)
	}
}

func TestHTTPNamesSelectClosedRegistrationWithoutFreeText(t *testing.T) {
	row := table.Row{Fields: []table.Field{
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
