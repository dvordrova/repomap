package lines

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/llm"
)

func TestLinePromptExamplesMatchRequestedOpenMode(t *testing.T) {
	for _, base := range []table.Definition{Directories(), Files()} {
		t.Run(base.Stage, func(t *testing.T) {
			rows := []table.Row{
				{ID: "first", Fields: []table.Field{{Name: "box_options", Value: []string{BoxHere}}}},
				{ID: "second", Fields: []table.Field{{Name: "box_options", Value: []string{BoxHere}}}},
			}
			for mode, def := range []table.Definition{base, WithOpen(base)} {
				windows, err := table.Windows(def, 1, rows)
				if err != nil || len(windows) != 1 {
					t.Fatalf("prepare owner rows: %v", err)
				}
				call, err := table.Call(def, windows[0])
				if err != nil || call.Prompt.System != base.System {
					t.Fatalf("ordinary table call changed the owner instructions: %v", err)
				}
				var request struct {
					Fill []struct{ Name string } `json:"fill"`
				}
				if err := json.Unmarshal([]byte(call.Prompt.User), &request); err != nil {
					t.Fatal(err)
				}
				client := &deepseek.Client{HTTPClient: &http.Client{}, Endpoint: "https://api.deepseek.com/chat/completions", Model: "format-test", Auth: "none", MaxTokens: llm.DefaultMaxOutputTokens}
				prepared, err := llm.Prepare(client, call.Prompt, call.Limits)
				if err != nil {
					t.Fatal(err)
				}
				var sent struct{ Messages []struct{ Content string } }
				if err := json.Unmarshal(prepared.Bytes(), &sent); err != nil {
					t.Fatal(err)
				}
				if strings.Count(sent.Messages[0].Content, `"rows"`) != 1 {
					t.Fatal("prepared prompt must contain just the current mode's response example")
				}
				wire := call.Prompt.ResponseExample
				var example struct {
					Rows []map[string]string `json:"rows"`
				}
				if err := json.Unmarshal([]byte(wire), &example); err != nil || len(example.Rows) != 1 {
					t.Fatalf("invalid owner response example: %v", err)
				}
				for _, row := range example.Rows {
					if len(row) != len(request.Fill)+1 {
						t.Fatalf("prompt example disagrees with advertised fill: row=%+v fill=%+v", row, request.Fill)
					}
					for _, column := range request.Fill {
						if _, ok := row[column.Name]; !ok {
							t.Fatalf("prompt example omits requested cell %q", column.Name)
						}
					}
				}
				// Actual decisions, independent of the illustrative response values.
				example.Rows = []map[string]string{
					{"key": "r1", "title": "Command entry", "line": "Starts the analysis.", "box": "here"},
					{"key": "r2", "title": "Report rendering", "line": "Renders the report.", "box": "here"},
				}
				if mode == 1 {
					for _, row := range example.Rows {
						row["open"] = "yes"
					}
				}
				answer, _ := json.Marshal(example)
				result, err := table.DecodeResult(def, windows[0], answer)
				if err != nil || len(result.Rejections) != 0 || len(result.Answers) != len(rows) {
					t.Fatalf("owner prompt example was refused: %+v / %v", result, err)
				}
				if mode == 1 {
					delete(example.Rows[0], "open")
					incomplete, _ := json.Marshal(example)
					refused, err := table.DecodeResult(def, windows[0], incomplete)
					if err != nil || len(refused.Rejections) != 1 || refused.Answers[0] != nil || refused.Answers[1]["open"] != "yes" {
						t.Fatalf("missing open was invented or a complete sibling was lost: %+v / %v", refused, err)
					}
				}
			}
		})
	}
}
