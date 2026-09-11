package adaptertest

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/extractors"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
)

// AssertQueryOccurrenceOwners tests the consuming facts boundary with actual
// cumulative native observations and independently extracted SQL source text.
func AssertQueryOccurrenceOwners(t *testing.T, root string, repository *corpus.Corpus, index programindex.Index, source string) {
	t.Helper()
	response, err := extractors.Database(t.Context(), extractors.Request{Version: extractors.Version, Root: root, Files: []string{source}})
	if err != nil {
		t.Fatal(err)
	}
	layer, err := facts.Build(facts.Input{Repository: repository, Targets: []facts.TargetInput{{Index: index}}, Extractions: []facts.Extraction{{Name: "database", Nodes: response.Nodes, Links: response.Links}}})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, fact := range layer.Facts {
		if fact.Data == nil || fact.Data.Kind != "query" || fact.Path != source {
			continue
		}
		data := fact.Data
		seen[data.SQL] = true
		if strings.Contains(data.SQL, "global_rows") {
			if data.Owner != nil {
				t.Fatalf("global SQL borrowed its consumer's ownership: %+v", data)
			}
			continue
		}
		// Current producers retain value provenance but no declaration owner
		// on these literal leaves (Go SSA constants have no source anchor).
		// Keeping that frontier is preferable to borrowing the call consumer.
		if data.Owner != nil {
			t.Fatalf("SQL without native lexical ownership acquired an owner: %+v", data)
		}
	}
	if len(seen) != 3 {
		t.Fatalf("SQL fixture count=%d: %v", len(seen), seen)
	}
}

func AssertConcreteParameterMethod(t *testing.T, index programindex.Index, source string) {
	t.Helper()
	var caller, method string
	for _, object := range index.Objects {
		if object.Location == nil || object.Location.Path != source {
			continue
		}
		if object.Name == "TypedParameter" {
			caller = object.ID
		}
		if object.Name == "ReadRows" {
			method = object.ID
		}
	}
	if caller == "" || method == "" {
		t.Fatal("native parameter fixture declarations missing")
	}
	for _, relation := range index.Relations {
		if relation.Kind == programindex.RelationCalls && relation.FromID == caller && relation.Resolution == programindex.ResolutionExact && len(relation.ToIDs) == 1 && relation.ToIDs[0] == method {
			return
		}
	}
	t.Fatal("compiler-resolved concrete parameter method call missing")
}
