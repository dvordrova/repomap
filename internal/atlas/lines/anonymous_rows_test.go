package lines

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

func rowText(t *testing.T, row table.Row) string {
	t.Helper()
	raw, err := json.Marshal(row.Fields)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// A function literal reaches the file and symbol rows as the adapter's fact,
// never as a name's shape (control review, A3's prompt remnant): Go's
// serve$1, which go/ssa named after serve, is marked anonymous, and a public
// JavaScript price$1 is a declaration like any other, unmarked.
func TestRowsStateAFunctionLiteralFromItsAnonymityNeverFromItsName(t *testing.T) {
	directory := atlas.Place{ID: atlas.DirectoryID("pkg"), Kind: atlas.PlaceDirectory, Path: "pkg", Directory: &atlas.DirectoryFacts{Files: []string{"a.go"}, FileCount: 1}}
	file := atlas.Place{ID: atlas.FileID("pkg/a.go"), Kind: atlas.PlaceFile, Path: "pkg/a.go", Parent: directory.ID, File: &atlas.FileFacts{Decls: []atlas.Decl{
		{Name: "serve", Kind: "function", Exported: true, LineNo: 3},
		{Name: "serve$1", Kind: "function", Anonymous: true, LineNo: 5},
		{Name: "price$1", Kind: "function", Exported: true, LineNo: 9},
	}}}
	text := rowText(t, FileRow(file, directory, noLines{}, map[string]atlas.Place{directory.ID: directory}, nil))
	if !strings.Contains(text, `{"anonymous":true,"kind":"function","name":"serve$1"}`) || !strings.Contains(text, `{"kind":"function","name":"price$1"}`) {
		t.Fatalf("file row = %s", text)
	}
	literal := atlas.Place{ID: "s1", Kind: atlas.PlaceSymbol, Path: file.Path, Parent: file.ID, Symbol: &atlas.SymbolFacts{
		Decl: atlas.Decl{Name: "serve$1", Kind: "function", Anonymous: true},
		Calls: []atlas.SymbolCall{
			{Kind: "calls", Name: "serve$1$1", Line: 6, Resolution: DefaultResolution, CalleeIDs: []string{"s2"}, CalleeAnonymous: true},
			{Kind: "calls", Name: "price$1", Line: 7, Resolution: DefaultResolution, CalleeIDs: []string{"s3"}},
		},
		Bindings: []atlas.SymbolBinding{{From: "serve", To: "serve$1", ToAnonymous: true, Detail: "net/http.HandleFunc", Kind: "passes_callback", Resolution: "exact", Path: file.Path, Line: 4}},
	}}
	text = rowText(t, SymbolRow(literal, ""))
	for _, want := range []string{`{"Name":"anonymous","Value":true}`, `"serve$1$1@6 anonymous"`, `"price$1@7"`, `"to_anonymous":true`} {
		if !strings.Contains(text, want) {
			t.Fatalf("symbol row lacks %s: %s", want, text)
		}
	}
	if strings.Contains(text, "from_anonymous") {
		t.Fatalf("a named end was marked: %s", text)
	}
}
