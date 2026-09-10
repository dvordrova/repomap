package reading

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/llm"
)

type aliasProvider struct {
	tableProvider
	aliases map[string]string
}

func (provider *aliasProvider) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	completion, err := provider.tableProvider.Complete(ctx, prepared)
	if err != nil {
		return completion, err
	}
	var request struct {
		Table string
		Rows  []struct{ Key, Name string }
	}
	if err := json.Unmarshal(prepared.Bytes(), &request); err != nil || request.Table != lines.StageSymbols {
		return completion, err
	}
	byKey := make(map[string]string)
	for _, row := range request.Rows {
		byKey[row.Key] = "none"
		if alias := provider.aliases[row.Name]; alias != "" {
			byKey[row.Key] = alias
		}
	}
	var response struct{ Rows []map[string]string }
	if err := json.Unmarshal(completion.Response, &response); err != nil {
		return completion, err
	}
	for _, row := range response.Rows {
		row["alias"] = byKey[row["key"]]
	}
	completion.Response, err = json.Marshal(map[string]any{"rows": response.Rows})
	return completion, err
}

func TestSymbolAndTypeAliasesFollowExistingKnowledgeWithoutRenamingDeclarations(t *testing.T) {
	graph := knowledgeGraph(t)
	rename := func(decl *atlas.Decl) {
		switch decl.Name {
		case "Main":
			decl.Name, decl.Doc = "시작", "Starts the program."
		case "Z":
			decl.Name, decl.Doc = "주가정보", "Represents a daily stock quote."
		}
	}
	want := make(map[string]atlas.Decl)
	for i := range graph.Places {
		place := &graph.Places[i]
		if place.File != nil {
			for j := range place.File.Decls {
				rename(&place.File.Decls[j])
			}
		}
		if place.Symbol != nil {
			rename(&place.Symbol.Decl)
			place.ID = atlas.SymbolID(place.Path, place.LineNo, place.Symbol.Decl.Name)
			if place.Symbol.Decl.Name != "Gen" {
				want[place.ID] = place.Symbol.Decl
			}
		}
	}
	atlas.SortPlaces(graph.Places)
	before, _ := json.Marshal(graph)
	aliases := map[string]string{"시작": "program start", "주가정보": "stock quote"}
	provider := &aliasProvider{aliases: aliases}
	cache := t.TempDir()
	opts := readOptions(t, graph, provider, cache)
	first, err := Read(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	assertAliases := func(result Result) {
		t.Helper()
		seen := make(map[string]bool)
		for _, target := range result.Atlas.Targets {
			for _, box := range target.Boxes {
				for _, file := range box.Files {
					for _, symbol := range file.Symbols {
						decl, known := want[symbol.ID]
						if !known {
							if symbol.Alias != "" {
								t.Fatal("unreviewed declaration acquired an alias")
							}
							continue
						}
						seen[symbol.ID] = true
						if symbol.Alias != aliases[decl.Name] || symbol.Name != decl.Name || symbol.ObjectID != decl.ObjectID || symbol.LineNo != decl.LineNo || symbol.Column != decl.Column || symbol.Signature != decl.Signature || symbol.Doc != decl.Doc {
							t.Fatalf("English label replaced a native declaration or lost its model value: %+v", symbol)
						}
					}
				}
			}
		}
		if len(seen) != len(want) {
			t.Fatalf("only %d of %d reviewed declarations were published", len(seen), len(want))
		}
	}
	assertAliases(first)
	for _, use := range first.Uses {
		if use.Stage == lines.StageSymbols && (use.Rows != 6 || use.Windows != 4) {
			t.Fatalf("selection and caption accounting: %+v", use)
		}
	}
	raw, err := os.ReadFile(filepath.Join(opts.OwnerRunDir, KnowledgeFilename))
	if err != nil {
		t.Fatal(err)
	}
	var knowledge struct{ Records []Knowledge }
	if err := json.Unmarshal(raw, &knowledge); err != nil {
		t.Fatal(err)
	}
	for _, record := range knowledge.Records {
		if decl, known := want[record.PlaceID]; known {
			expected := aliases[decl.Name]
			if expected == "" {
				expected = "none"
			}
			if record.Cells["alias"] != expected || record.SubjectID != decl.ObjectID || record.OriginRequest == "" {
				t.Fatalf("alias lost its accepted knowledge provenance: %+v", record)
			}
		}
	}
	// The empty provider would answer none. An exact warm replay must restore
	// the original accepted aliases along with their descriptions, without calls.
	warm := &aliasProvider{}
	opts.Provider, opts.OwnerRunDir = warm, t.TempDir()
	second, err := Read(t.Context(), opts)
	if err != nil || warm.calls != 0 {
		t.Fatalf("alias did not reuse existing knowledge: calls %d, %v", warm.calls, err)
	}
	assertAliases(second)
	after, _ := json.Marshal(graph)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("alias interpretation changed the source graph")
	}
}
