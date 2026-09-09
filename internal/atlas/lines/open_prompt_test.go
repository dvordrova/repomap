package lines

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/table"
)

func TestLinePromptExamplesMatchRequestedOpenMode(t *testing.T) {
	for _, base := range []table.Definition{Directories(), Files()} {
		t.Run(base.Stage, func(t *testing.T) {
			blocks := strings.Split(base.System, "```json\n")[1:]
			if len(blocks) != 2 {
				t.Fatal("the owner prompt must demonstrate both base and open modes")
			}
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
				wire, _, _ := strings.Cut(blocks[mode], "\n```")
				var example struct {
					Rows []map[string]string `json:"rows"`
				}
				if err := json.Unmarshal([]byte(wire), &example); err != nil || len(example.Rows) != len(rows) {
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
				result, err := table.DecodeResult(def, windows[0], []byte(wire))
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
