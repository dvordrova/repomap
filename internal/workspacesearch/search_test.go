package workspacesearch

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/sourcecatalog"
)

func TestNewAndSearchExactWorkspaceFacts(t *testing.T) {
	t.Parallel()

	catalog := testCatalog(t, "service", []string{"z.go", "README.md", "cmd/main.go"})
	symbol := testSymbol("run", "Run", "cmd/main.go", 12)
	index, err := New(Input{
		Catalog: catalog,
		Symbols: []evidence.Entity{symbol, symbol},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	entries := index.Entries()
	if len(entries) != 4 {
		t.Fatalf("entries = %d, want three paths and one deduplicated symbol", len(entries))
	}
	if entries[0].Kind != KindDocument || entries[0].Path != "README.md" {
		t.Fatalf("first entry = %#v, want deterministic document entry", entries[0])
	}

	tests := []struct {
		name  string
		query string
		kind  Kind
		path  string
		match MatchKind
	}{
		{name: "full path", query: "cmd/main.go", kind: KindFile, path: "cmd/main.go", match: MatchExactPath},
		{name: "document basename", query: "README.md", kind: KindDocument, path: "README.md", match: MatchExactPath},
		{name: "symbol", query: "Run", kind: KindSymbol, path: "cmd/main.go", match: MatchExactName},
		{name: "prefix", query: "REA", kind: KindDocument, path: "README.md", match: MatchPrefix},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, searchErr := index.Search(Query{Text: test.query})
			if searchErr != nil {
				t.Fatal(searchErr)
			}
			if len(result) == 0 || result[0].Entry.Kind != test.kind ||
				result[0].Entry.Path != test.path || result[0].Match != test.match {
				t.Fatalf("Search(%q) = %#v", test.query, result)
			}
		})
	}

	again, err := New(Input{Catalog: catalog, Symbols: []evidence.Entity{symbol}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(index.Entries(), again.Entries()) {
		t.Fatal("construction is not deterministic")
	}
}

func TestNewUsesAnalysisRootRelativeCatalogScope(t *testing.T) {
	t.Parallel()

	catalog := testCatalog(t, "repository/service", []string{"cmd/main.go"})
	index, err := New(Input{
		Catalog: catalog,
		Symbols: []evidence.Entity{
			testSymbol("main", "main", "cmd/main.go", 3),
			testSymbol("outside", "Outside", "../outside.go", 1),
			testSymbol("absolute", "Absolute", "/private/tmp/outside.go", 1),
			{ID: "missing", Kind: evidence.EntityFunction, Name: "Missing"},
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := index.Entries(); len(got) != 2 {
		t.Fatalf("entries = %#v, want one path and one authorized symbol", got)
	}
	for _, query := range []string{"Outside", "Absolute", "Missing"} {
		result, searchErr := index.Search(Query{Text: query})
		if searchErr != nil {
			t.Fatal(searchErr)
		}
		if len(result) != 0 {
			t.Fatalf("Search(%q) leaked outside-scope entry: %#v", query, result)
		}
	}
}

func TestNewRejectsRawCardinalityAndOversizedScalars(t *testing.T) {
	catalog := testCatalog(t, "repository", []string{"main.go"})

	catalogPaths := make([]string, 0, maxCatalogEntries+1)
	for index := 0; index <= maxCatalogEntries; index++ {
		catalogPaths = append(catalogPaths, fmt.Sprintf("pkg/%04d/file.go", index))
	}
	oversizedCatalog := testCatalog(t, "repository", catalogPaths)
	if _, err := New(Input{Catalog: oversizedCatalog}); err == nil ||
		!strings.Contains(err.Error(), "source catalog exceeds") {
		t.Fatalf("oversized catalog error = %v", err)
	}

	tooMany := make([]evidence.Entity, maxSymbolEntries+1)
	if _, err := New(Input{Catalog: catalog, Symbols: tooMany}); err == nil ||
		!strings.Contains(err.Error(), "symbols exceed") {
		t.Fatalf("oversized symbol collection error = %v", err)
	}

	huge := testSymbol("huge", strings.Repeat("x", maxNameBytes+1), "main.go", 1)
	if _, err := New(Input{Catalog: catalog, Symbols: []evidence.Entity{huge}}); err == nil ||
		!strings.Contains(err.Error(), "oversized scalar") {
		t.Fatalf("oversized scalar error = %v", err)
	}
}

func TestNewFiltersMalformedExactEntities(t *testing.T) {
	t.Parallel()

	catalog := testCatalog(t, "repository", []string{"main.go"})
	symbols := []evidence.Entity{
		{ID: "query", Kind: evidence.EntityQuery, Name: "Query", Location: testLocation("main.go", 1)},
		{ID: "external", Kind: evidence.EntityFunction, Name: "External", Scope: evidence.SourceScopeDependency, Location: testLocation("main.go", 1)},
		{ID: "control", Kind: evidence.EntityFunction, Name: "Bad\nName", Location: testLocation("main.go", 1)},
		{ID: "space", Kind: evidence.EntityFunction, Name: " Padded", Location: testLocation("main.go", 1)},
		{ID: "range", Kind: evidence.EntityFunction, Name: "Range", Location: &evidence.Location{Path: "main.go", Line: 4, EndLine: 3}},
	}
	index, err := New(Input{Catalog: catalog, Symbols: symbols})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := index.Entries(); len(got) != 1 || got[0].Kind != KindFile {
		t.Fatalf("malformed exact entities survived: %#v", got)
	}
}

func TestSearchValidatesQueryAndBoundsResults(t *testing.T) {
	paths := make([]string, 0, 40)
	for index := 0; index < 40; index++ {
		paths = append(paths, filepath.ToSlash(filepath.Join("pkg", "match", fmt.Sprintf("%02d", index), "file.go")))
	}
	catalog := testCatalog(t, "repository", paths)
	index, err := New(Input{Catalog: catalog})
	if err != nil {
		t.Fatal(err)
	}

	for _, query := range []Query{
		{},
		{Text: strings.Repeat("x", maxQueryBytes+1)},
		{Text: "bad\nquery"},
		{Text: string([]byte{0xff})},
		{Text: "match", MaxResults: maxResults + 1},
	} {
		if _, searchErr := index.Search(query); searchErr == nil {
			t.Fatalf("Search(%#v) succeeded", query)
		}
	}

	result, err := index.Search(Query{Text: "match"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != maxResults || cap(result) > maxResults {
		t.Fatalf("default results len/cap = %d/%d, want %d bounded", len(result), cap(result), maxResults)
	}
	narrowed, err := index.Search(Query{Text: "match", MaxResults: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(narrowed) != 3 || cap(narrowed) > 3 {
		t.Fatalf("narrowed results len/cap = %d/%d", len(narrowed), cap(narrowed))
	}
}

func TestEntriesAndMatchesAreDefensiveCopies(t *testing.T) {
	t.Parallel()

	catalog := testCatalog(t, "repository", []string{"main.go"})
	index, err := New(Input{
		Catalog: catalog,
		Symbols: []evidence.Entity{testSymbol("run", "Run", "main.go", 2)},
	})
	if err != nil {
		t.Fatal(err)
	}
	entries := index.Entries()
	for index := range entries {
		if entries[index].Entity != nil {
			entries[index].Entity.Name = "mutated"
			entries[index].Entity.Location.Path = "outside.go"
			entries[index].Location.Path = "outside.go"
		}
	}
	result, err := index.Search(Query{Text: "Run"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Entry.Name != "Run" ||
		result[0].Entry.Location.Path != "main.go" ||
		result[0].Entry.Entity.Location.Path != "main.go" {
		t.Fatalf("index changed through caller copy: %#v", result)
	}
	result[0].Entry.Entity.Name = "changed again"
	again, err := index.Search(Query{Text: "Run"})
	if err != nil || len(again) != 1 || again[0].Entry.Entity.Name != "Run" {
		t.Fatalf("match was not defensively copied: %#v, %v", again, err)
	}
}

func testCatalog(t *testing.T, rootSuffix string, paths []string) sourcecatalog.Catalog {
	t.Helper()
	repositoryRoot := filepath.Join(t.TempDir(), "repository")
	analysisRoot := repositoryRoot
	if rootSuffix != "repository" {
		analysisRoot = filepath.Join(repositoryRoot, filepath.FromSlash(strings.TrimPrefix(rootSuffix, "repository/")))
	}
	inputs := make([]freshness.CapturedInput, 0, len(paths))
	for _, sourcePath := range paths {
		repositoryPath := sourcePath
		if analysisRoot != repositoryRoot {
			prefix, err := filepath.Rel(repositoryRoot, analysisRoot)
			if err != nil {
				t.Fatal(err)
			}
			repositoryPath = filepath.ToSlash(filepath.Join(prefix, filepath.FromSlash(sourcePath)))
		}
		id := sha256.Sum256([]byte("workspacesearch-test\x00" + repositoryPath))
		inputs = append(inputs, freshness.CapturedInput{
			Version:       freshness.CapturedInputVersion,
			ID:            fmt.Sprintf("%x", id[:]),
			Path:          repositoryPath,
			Kind:          freshness.FileRegular,
			Mode:          "100644",
			ContentSHA256: strings.Repeat("a", 64),
			Stages:        []string{"report_evidence"},
		})
	}
	catalog, err := sourcecatalog.New(sourcecatalog.Input{
		RepositoryRoot: repositoryRoot,
		AnalysisRoot:   analysisRoot,
		AllowedPaths:   paths,
		CapturedInputs: inputs,
	})
	if err != nil {
		t.Fatalf("sourcecatalog.New: %v", err)
	}
	return catalog
}

func testSymbol(id, name, sourcePath string, line int) evidence.Entity {
	return evidence.Entity{
		ID:       id,
		Kind:     evidence.EntityFunction,
		Name:     name,
		Language: "go",
		Scope:    evidence.SourceScopeRepository,
		Location: testLocation(sourcePath, line),
	}
}

func testLocation(sourcePath string, line int) *evidence.Location {
	return &evidence.Location{Path: sourcePath, Line: line, Column: 1}
}
