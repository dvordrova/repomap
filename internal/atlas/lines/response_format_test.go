package lines

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/terminology"
)

func TestPreparedTablesHaveOneResponseShapeWithCurrentColumns(t *testing.T) {
	client := &deepseek.Client{HTTPClient: &http.Client{}, Endpoint: "https://api.deepseek.com/chat/completions", Model: "response-contract-test", Auth: "none", MaxTokens: llm.DefaultMaxOutputTokens}
	for _, def := range []table.Definition{
		Directories(), WithOpen(Directories()), Files(), WithOpen(Files()),
		Symbols(), Types(), SymbolSelection(false), SymbolSelection(true),
		Operations(), Boundaries(), Targets(), Targets(true), Arrows(),
		Joints(), Peers(), ZoneNames(7), ZoneAssign([]string{"Rendering"}), ZoneLines(), Answer(),
	} {
		for _, withTerms := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/terms=%t", def.Contract, withTerms), func(t *testing.T) {
				rows := []table.Row{{ID: "local-subject", Fields: []table.Field{
					{Name: "path", Value: "main.go"},
					{Name: "box_options", Value: []string{"here"}},
					{Name: "entry_options", Value: []string{"self", "none"}},
					{Name: "peer_options", Value: []string{"p1", "none"}},
					{Name: "candidate_options", Value: []string{"c1"}},
					{Name: "call_options", Value: []string{"c1"}},
				}}}
				windows, err := table.Windows(def, 1, rows)
				if err != nil || len(windows) != 1 {
					t.Fatalf("windows: %v", err)
				}
				call, err := table.Call(def, windows[0])
				if err != nil {
					t.Fatal(err)
				}
				var provider llm.Provider = client
				if withTerms {
					provider = terminology.NewCollector([]string{"main.go"}).Wrap(client)
				}
				prepared, err := llm.Prepare(provider, call.Prompt, call.Limits)
				if err != nil {
					t.Fatal(err)
				}
				var sent struct {
					Messages []struct{ Role, Content string }
				}
				if err := json.Unmarshal(prepared.Bytes(), &sent); err != nil {
					t.Fatal(err)
				}
				system, user := sent.Messages[0].Content, sent.Messages[1].Content
				if strings.Count(system, `"rows"`) != 1 || strings.Contains(system, `"terms"`) || strings.Contains(system, `"result"`) {
					t.Fatal("main response gained an optional metadata envelope")
				}
				if strings.Contains(user, "REPOMAP_PROSE_SOURCES_V1") || user != call.Prompt.User {
					t.Fatal("local prose context entered provider input")
				}
				if (len(prepared.ResponseContext()) > 0) != (withTerms && !call.Prompt.NoResponseAdjunct) {
					t.Fatal("local prose context eligibility changed")
				}
				example := json.RawMessage(system[strings.LastIndex(system, "\n")+1:])
				var result struct{ Rows []map[string]string }
				if json.Unmarshal(example, &result) != nil || len(result.Rows) != 1 {
					t.Fatal("invalid final result example")
				}
				want := []string{"key"}
				for _, column := range def.Columns {
					want = append(want, column.Name)
				}
				var got []string
				for key := range result.Rows[0] {
					got = append(got, key)
				}
				sort.Strings(got)
				sort.Strings(want)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("final response fields %v disagree with current mode %v", got, want)
				}
			})
		}
	}
}

func TestTableOwnerKeepsProseMetadataAndOmitsClosedDecisionMetadata(t *testing.T) {
	for _, test := range []struct {
		name    string
		def     table.Definition
		omitted bool
	}{
		{"selection", SymbolSelection(false), true},
		{"type-selection", SymbolSelection(true), true},
		{"closed-assignments", ZoneAssign([]string{"Storage"}), true},
		{"captions", Symbols(), false},
		{"long-prose", Types(), false},
		{"conditional-operation-prose", Operations(), false},
		{"conditional-boundary-prose", Boundaries(true), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			windows, err := table.Windows(test.def, 1, []table.Row{{ID: "local", Fields: []table.Field{{Name: "path", Value: "service.go"}}}})
			if err != nil {
				t.Fatal(err)
			}
			call, err := table.Call(test.def, windows[0])
			if err != nil || call.Prompt.NoResponseAdjunct != test.omitted {
				t.Fatalf("owner metadata eligibility changed: %+v / %v", call.Prompt, err)
			}
		})
	}
}
