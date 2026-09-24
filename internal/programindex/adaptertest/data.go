package adaptertest

import (
	"reflect"
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

// AssertSQLQueryFacts builds the fact layer from a real native index and checks
// the sql_query facts anchored in source: exactly the statements given, each
// with its tables (nil: none). Every ordinary text must reach a call outside the
// repository as a literal argument in source and still become no fact, so a
// message that only starts with an SQL verb is refused, not merely unseen.
func AssertSQLQueryFacts(t *testing.T, index programindex.Index, source string, statements map[string]string, ordinary ...string) {
	t.Helper()
	layer, err := facts.Build(facts.Input{Targets: []facts.TargetInput{{Index: index}}})
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]string{}
	for _, fact := range layer.OfKind(facts.KindSQLQuery) {
		if fact.Anchor != nil && fact.Anchor.Path == source {
			have[fact.Value] = fact.Key
		}
	}
	if statements == nil {
		statements = map[string]string{}
	}
	if !reflect.DeepEqual(have, statements) {
		t.Fatalf("sql_query facts in %s = %v, want %v", source, have, statements)
	}
	objects := map[string]programindex.Object{}
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	outside := func(relation programindex.Relation) bool {
		for _, id := range relation.ToIDs {
			if object, ok := objects[id]; ok && (object.Kind != programindex.ObjectExternalSymbol || object.External != nil && object.External.RepositoryPath != "") {
				return false
			}
		}
		return true
	}
	for _, text := range ordinary {
		reached := false
		for _, relation := range index.Relations {
			for _, pattern := range relation.Patterns {
				if pattern.Location == nil || pattern.Location.Path != source || !outside(relation) {
					continue
				}
				for _, argument := range pattern.Arguments {
					reached = reached || argument.Kind == programindex.PatternLiteralString && argument.Value == text
				}
			}
		}
		if !reached {
			t.Fatalf("%q is not a literal argument of a call outside the repository in %s", text, source)
		}
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
