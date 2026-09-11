package run

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/terminology"
)

// This provider understands the original table and separate prose-row glossary,
// as the configured online provider must. No production runtime seam is replaced.
type terminologyRuntimeProvider struct {
	calls  int
	stages map[string]int
}

func (*terminologyRuntimeProvider) State() []byte {
	return []byte(`{"model":"terminology-runtime-test"}`)
}
func (*terminologyRuntimeProvider) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	if strings.Contains(prompt.System, "# Response envelope and repository terminology") {
		return llm.Prepared{}, fmt.Errorf("main analysis must not request inline glossary metadata")
	}
	raw, err := json.Marshal(map[string]string{"system": prompt.System, "user": prompt.User})
	if err != nil {
		return llm.Prepared{}, err
	}
	return llm.NewPrepared(raw)
}
func (p *terminologyRuntimeProvider) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var message map[string]string
	if err := json.Unmarshal(prepared.Bytes(), &message); err != nil {
		return llm.Completion{}, err
	}
	var input struct {
		Table string `json:"table"`
		Fill  []struct {
			Name, Kind  string
			Options     []string
			OptionsFrom string `json:"options_from"`
		} `json:"fill"`
		Rows []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal([]byte(message["user"]), &input); err != nil {
		return llm.Completion{}, err
	}
	if input.Table == "" {
		var request struct {
			Prose []struct {
				Ref  string
				Text []string
			}
		}
		if err := json.Unmarshal([]byte(message["user"]), &request); err != nil {
			return llm.Completion{}, err
		}
		p.calls++
		if p.stages == nil {
			p.stages = make(map[string]int)
		}
		p.stages["glossary"]++
		terms := []map[string]any{}
		for _, row := range request.Prose {
			if !strings.Contains(strings.Join(row.Text, " "), "OHLCV") {
				continue
			}
			terms = append(terms, map[string]any{"name": "OHLCV", "kind": "acronym", "explanation": "The named group of market-data values described here.", "rows": []string{row.Ref}})
		}
		raw, err := json.Marshal(map[string]any{"terms": terms})
		return llm.Completion{Response: raw, ChoiceCount: 1, FinishReason: llm.FinishStop, Metrics: llm.Metrics{Attempts: 1}}, err
	}
	p.calls++
	if p.stages == nil {
		p.stages = make(map[string]int)
	}
	p.stages[input.Table]++
	rows := make([]map[string]string, 0, len(input.Rows))
	for _, row := range input.Rows {
		key, _ := row["key"].(string)
		answer := map[string]string{"key": key}
		for _, column := range input.Fill {
			switch column.Kind {
			case "text", "prose":
				answer[column.Name] = "OHLCV describes the market data fields."
			case "sequence":
				answer[column.Name] = "none"
			case "choice":
				options := append([]string(nil), column.Options...)
				if column.OptionsFrom != "" {
					options = nil
					if values, ok := row[column.OptionsFrom].([]any); ok {
						for _, value := range values {
							options = append(options, value.(string))
						}
					}
				}
				if len(options) > 0 {
					answer[column.Name] = options[0]
				}
			}
		}
		rows = append(rows, answer)
	}
	raw, err := json.Marshal(map[string]any{"rows": rows})
	return llm.Completion{Response: raw, ChoiceCount: 1, FinishReason: llm.FinishStop, Metrics: llm.Metrics{Attempts: 1}}, err
}

func TestReadEnabledTerminologyUsesOrdinaryFactoryAndSeparateAcceptedProsePass(t *testing.T) {
	source := t.TempDir()
	opts := reading.Options{OwnerRunDir: source, Repository: "example", Revision: "abc",
		Targets: []reading.TargetMeta{{ID: "t1", Language: "go", Kind: "library", Name: "example", Root: "."}},
		Graph: atlas.Graph{Version: atlas.GraphVersion, Revision: "abc", Places: []atlas.Place{
			{ID: "dir:.", Kind: atlas.PlaceDirectory, Path: ".", TargetIDs: []string{"t1"}, Given: "one file", Directory: &atlas.DirectoryFacts{Files: []string{"main.go"}, FileCount: 1}},
			{ID: "file:main.go", Kind: atlas.PlaceFile, Path: "main.go", TargetIDs: []string{"t1"}, Parent: "dir:.", Given: "one function", File: &atlas.FileFacts{}},
		}}}
	if _, err := reading.SaveInput(opts); err != nil {
		t.Fatal(err)
	}
	provider := &terminologyRuntimeProvider{}
	factoryCalls := 0
	factory := func() (llm.Provider, error) { factoryCalls++; return provider, nil }
	cache := t.TempDir()
	output := filepath.Join(t.TempDir(), "reading")
	args := []string{filepath.Join(source, reading.InputFilename), "--through", "files", "--output", output, "--debug-dir", cache}
	var stdout bytes.Buffer
	if err := runReadConfigured(context.Background(), args, &stdout, factory, true); err != nil {
		t.Fatal(err)
	}
	if factoryCalls != 1 || provider.calls == 0 || provider.stages["atlas_directories"] == 0 || provider.stages["atlas_files"] == 0 || provider.stages["glossary"] == 0 {
		t.Fatalf("ordinary stages were not wrapped: calls=%d stages=%v factories=%d", provider.calls, provider.stages, factoryCalls)
	}
	raw, err := os.ReadFile(filepath.Join(output, "terminology.json"))
	if err != nil {
		t.Fatal(err)
	}
	var terms []terminology.Candidate
	if err := json.Unmarshal(raw, &terms); err != nil || len(terms) == 0 {
		t.Fatalf("accepted analysis did not persist terms: %s / %v", raw, err)
	}
	for _, term := range terms {
		if term.Name != "OHLCV" || len(term.Origins) == 0 {
			t.Fatalf("term lost accepted-call provenance: %+v", term)
		}
		for _, source := range term.Sources {
			if source.Path != "main.go" {
				t.Fatalf("term gained a directory or unadvertised source: %+v", term)
			}
		}
	}
	for _, name := range []string{"report.html", "report.json", "glossary.json"} {
		if _, err := os.Stat(filepath.Join(output, name)); !os.IsNotExist(err) {
			t.Fatalf("saved reading added a publication/reduction stage: %s", name)
		}
	}
	calls := provider.calls
	args[4] = filepath.Join(t.TempDir(), "warm-reading")
	if err := runReadConfigured(context.Background(), args, &stdout, factory, true); err != nil {
		t.Fatal(err)
	}
	if provider.calls != calls {
		t.Fatalf("warm metadata recollection added calls: %d -> %d", calls, provider.calls)
	}
	warm, err := os.ReadFile(filepath.Join(args[4], "terminology.json"))
	if err != nil || !bytes.Equal(raw, warm) {
		t.Fatalf("warm run lost original terms: %s / %v", warm, err)
	}
}

func TestReadingTerminologyPathsRequireSourceAuthority(t *testing.T) {
	graph := atlas.Graph{Places: []atlas.Place{
		{Kind: atlas.PlaceDirectory, Path: "pkg"},
		{Kind: atlas.PlaceEntity, Path: "generated/missing.go", Entity: &atlas.EntityFacts{Status: "not_in_corpus", Files: []string{}}},
		{Kind: atlas.PlaceEntity, Path: "logical-table-name", Entity: &atlas.EntityFacts{Files: []string{"main.go"}}},
		{Kind: atlas.PlaceSymbol, Path: "symbol-only.go"},
		{Kind: atlas.PlaceBoundary, Path: "boundary-only.go"},
		{Kind: atlas.PlaceFile, Path: "main.go"},
		{Kind: atlas.PlaceDocument, Path: "docs/usage.md"},
		{Kind: atlas.PlaceSourceFact, Path: "package.json"},
		{Kind: atlas.PlaceFile},
	}, Edges: []atlas.Edge{
		{Kind: "observation", Evidence: &atlas.EdgeEvidence{Path: "sql/queries.sql", LineNo: 12}},
		{Kind: "inventory", Evidence: &atlas.EdgeEvidence{Path: "generated/present.go", LineNo: 1}},
		{Kind: "calls"},
	}}
	want := []string{"main.go", "docs/usage.md", "package.json", "sql/queries.sql", "generated/present.go"}
	if got := readingTerminologyPaths(graph); !reflect.DeepEqual(got, want) {
		t.Fatalf("saved reading source authority: got %v, want %v", got, want)
	}
}
