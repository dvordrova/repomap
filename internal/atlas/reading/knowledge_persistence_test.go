package reading

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
)

func TestKnowledgeMemoWriteFailureKeepsAcceptedBindingsAndArtifacts(t *testing.T) {
	graph := knowledgeGraph(t)
	firstID := atlas.SymbolID("pkg/a/y.go", 3, "help")
	secondID := atlas.SymbolID("pkg/a/y.go", 8, "help")
	for _, place := range graph.Places {
		if place.ID != firstID {
			continue
		}
		other, facts := place, *place.Symbol
		other.ID, other.LineNo, other.Symbol = secondID, 8, &facts
		other.Symbol.Decl.LineNo = 8
		other.Symbol.Decl.ObjectID = "second-native-object"
		graph.Places = append(graph.Places, other)
		break
	}
	atlas.SortPlaces(graph.Places)
	cache := t.TempDir()
	initial := readOptions(t, graph, &tableProvider{}, cache)
	initial.Through, initial.WindowRows = lines.StageSymbols, 1
	before := readKnowledge(t, initial)
	if before[firstID].BasisID != before[secondID].BasisID {
		t.Fatal("fixture must share one exact input across two native subjects")
	}

	// Refuse only the optional memo's atomic rename. The shared response cache
	// and the new run's mandatory artifacts remain writable.
	memoPath := filepath.Join(cache, ".llm-cache", "memo-"+before[firstID].BasisID+".json")
	if err := os.Remove(memoPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(memoPath, 0o700); err != nil {
		t.Fatal(err)
	}
	provider := &tableProvider{}
	opts := readOptions(t, graph, provider, cache)
	opts.Through, opts.WindowRows = lines.StageSymbols, 1
	warnings := 0
	opts.State = func(stage, state string, details ...string) {
		if state != "cache write failed" {
			return
		}
		warnings++
		if stage != lines.StageSymbols || len(details) != 1 || !strings.Contains(details[0], memoPath) {
			t.Errorf("memo diagnostic lost its stage or exact path: %s / %s / %v", stage, state, details)
		}
	}
	after := readKnowledge(t, opts)
	if warnings != 1 || provider.calls != 0 {
		t.Fatalf("one failed memo should warn once and reuse the exact response: warnings=%d, calls=%d", warnings, provider.calls)
	}
	if len(after) != len(before) {
		t.Fatalf("memo failure lost accepted knowledge: %d -> %d records", len(before), len(after))
	}
	for id, expected := range before {
		expected.Source = atlas.SourceCache
		if !reflect.DeepEqual(after[id], expected) {
			t.Fatalf("memo failure changed accepted cells or provenance for %s:\nwant %+v\ngot  %+v", id, expected, after[id])
		}
	}
	if info, err := os.Stat(memoPath); err != nil || !info.IsDir() {
		t.Fatalf("failed memo was repaired or removed: %v", err)
	}
	journal, err := os.ReadFile(filepath.Join(opts.OwnerRunDir, atlas.TablesFilename))
	if err != nil || !strings.Contains(string(journal), "Shared exact input "+secondID) {
		t.Fatalf("mandatory tables artifact lost the shared binding: %v", err)
	}
	windows, err := filepath.Glob(filepath.Join(opts.OwnerRunDir, atlas.TablesDir, lines.StageSymbols+"-*.result.json"))
	if err != nil || len(windows) != 1 {
		t.Fatalf("accepted window artifact missing: %v, %v", windows, err)
	}
	for _, suffix := range []string{"request.ref.json", "response.ref.json"} {
		ref := strings.TrimSuffix(windows[0], "result.json") + suffix
		raw, err := readWindowPayload(ref)
		if err != nil || len(raw) == 0 {
			t.Fatalf("accepted window lost its exact %s payload: %v", suffix, err)
		}
	}
}

func TestKnowledgeMandatoryArtifactWriteFailureRemainsFatal(t *testing.T) {
	for _, artifact := range []string{
		KnowledgeFilename,
		filepath.Join(atlas.TablesDir, lines.StageDirectories+"-r1-w0.result.json"),
	} {
		t.Run(artifact, func(t *testing.T) {
			provider := &tableProvider{}
			opts := readOptions(t, knowledgeGraph(t), provider, t.TempDir())
			opts.Through = lines.StageDirectories
			blocked := filepath.Join(opts.OwnerRunDir, artifact)
			if err := os.MkdirAll(blocked, 0o700); err != nil {
				t.Fatal(err)
			}
			result, err := Read(t.Context(), opts)
			var pathErr *os.PathError
			if !errors.As(err, &pathErr) || pathErr.Path != blocked {
				t.Fatalf("mandatory artifact write was not returned: result=%+v, err=%v", result, err)
			}
			if result.Complete || provider.calls == 0 {
				t.Fatalf("expected an accepted model answer followed by fatal persistence: result=%+v, calls=%d", result, provider.calls)
			}
		})
	}
}
