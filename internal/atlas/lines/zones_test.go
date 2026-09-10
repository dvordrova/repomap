package lines

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/terminology"
)

func TestZoneModesKeepRowsAndCellsInPreparedRequests(t *testing.T) {
	boxes := []BoxSummary{
		{ID: "local-render", Title: "Report rendering", Line: "Renders the report.", Files: 3},
		{ID: "local-index", Title: "Program indexing", Line: "Indexes declarations.", Files: 5},
	}
	parts := []string{"Report rendering", "Program indexing"}
	cases := []struct {
		mode     string
		def      table.Definition
		context  []table.Field
		rows     []table.Row
		columns  []string
		response string
	}{
		{"names", ZoneNames(7), []table.Field{{Name: "question", Value: "names"}, {Name: "want", Value: 7}},
			[]table.Row{ZoneNamesRow("local-target", boxes)},
			[]string{"part_1", "part_2", "part_3", "part_4", "part_5", "part_6", "part_7"},
			`{"rows":[{"key":"r1","part_1":"Report rendering","part_2":"Program indexing","part_3":"Source reading","part_4":"Model requests","part_5":"Report serving","part_6":"Dependency analysis","part_7":"Question answering"}]}`},
		{"assign", ZoneAssign(parts), []table.Field{{Name: "question", Value: "assign"}, {Name: "parts", Value: parts}},
			[]table.Row{ZoneBoxRow(boxes[0]), ZoneBoxRow(boxes[1])}, []string{"part"},
			`{"rows":[{"key":"r1","part":"Report rendering"},{"key":"r2","part":"Program indexing"}]}`},
		{"lines", ZoneLines(), []table.Field{{Name: "question", Value: "lines"}},
			[]table.Row{ZoneLineRow("local-render-part", parts[0], boxes[:1]), ZoneLineRow("local-index-part", parts[1], boxes[1:])}, []string{"line"},
			`{"rows":[{"key":"r1","line":"Renders the report."},{"key":"r2","line":"Indexes declarations."}]}`},
	}
	client := &deepseek.Client{HTTPClient: &http.Client{}, Endpoint: "https://api.deepseek.com/chat/completions", Model: "zones-contract-test", Auth: "none", MaxTokens: llm.DefaultMaxOutputTokens}
	for _, test := range cases {
		t.Run(test.mode, func(t *testing.T) {
			windows, err := table.WindowsWithContext(test.def, 1, test.context, test.rows)
			if err != nil || len(windows) != 1 {
				t.Fatalf("windows: %d, %v", len(windows), err)
			}
			call, err := table.Call(test.def, windows[0])
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := llm.Prepare(client, call.Prompt, call.Limits)
			if err != nil {
				t.Fatal(err)
			}
			wrapped, err := llm.Prepare(terminology.NewCollector(nil).Wrap(client), call.Prompt, call.Limits)
			if err != nil || string(wrapped.Bytes()) != string(prepared.Bytes()) {
				t.Fatalf("source-free Zones acquired an unrelated response envelope: %v", err)
			}
			var wire struct {
				Messages []struct{ Role, Content string }
			}
			if err := json.Unmarshal(prepared.Bytes(), &wire); err != nil {
				t.Fatal(err)
			}
			if len(wire.Messages) != 2 || wire.Messages[0].Role != "system" || !strings.Contains(wire.Messages[0].Content, call.Prompt.System) || !strings.HasSuffix(wire.Messages[0].Content, call.Prompt.ResponseExample) || wire.Messages[1].Content != string(windows[0].Request) {
				t.Fatal("provider request changed the owning prompt or complete table input")
			}
			var input struct {
				Context map[string]any
				Fill    []struct{ Name string }
				Rows    []map[string]any
			}
			if err := json.Unmarshal([]byte(wire.Messages[1].Content), &input); err != nil {
				t.Fatal(err)
			}
			var columns []string
			for _, field := range input.Fill {
				columns = append(columns, field.Name)
			}
			if input.Context["question"] != test.mode || len(input.Rows) != len(test.rows) || !reflect.DeepEqual(columns, test.columns) {
				t.Fatalf("wrong mode/rows/columns: %+v", input)
			}
			for i, row := range input.Rows {
				if row["key"] != table.Key(i) {
					t.Fatalf("input row %d lost its exact key", i)
				}
			}
			var example struct{ Rows []map[string]string }
			if err := json.Unmarshal([]byte(call.Prompt.ResponseExample), &example); err != nil {
				t.Fatal(err)
			}
			if len(example.Rows) != 1 || example.Rows[0]["key"] != "r1" || len(example.Rows[0]) != len(test.columns)+1 {
				t.Fatalf("response example disagrees with one complete row: %+v", example)
			}
			for _, column := range test.columns {
				if _, ok := example.Rows[0][column]; !ok {
					t.Fatalf("response example omitted cell %q", column)
				}
			}
			answers, err := call.DecodeValidate([]byte(test.response))
			if err != nil || len(answers) != len(test.rows) {
				t.Fatalf("complete mode response refused: %+v, %v", answers, err)
			}
			if test.mode == "names" {
				var split []map[string]string
				for _, column := range test.columns {
					split = append(split, map[string]string{"key": "r1", column: answers[0][column]})
				}
				raw, _ := json.Marshal(map[string]any{"rows": split})
				if _, err := call.DecodeValidate(raw); err == nil || !strings.Contains(err.Error(), "7 rows answered, 1 asked") {
					t.Fatalf("repeated row keys were accepted: %v", err)
				}
			}
			t.Logf("prepared %s: %d input/output rows, columns %v, %d bytes, sha256 %s", test.mode, len(test.rows), columns, prepared.Len(), fmt.Sprintf("%x", sha256.Sum256(prepared.Bytes())))
		})
	}
}
