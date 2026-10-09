package claims

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/dvordrova/repomap/internal/programindex"
)

func nativeDocstringIndex(t *testing.T, nameLines ...int) programindex.Index {
	t.Helper()
	sites := make([]programindex.Location, len(nameLines))
	for i, line := range nameLines {
		sites[i] = programindex.Location{Line: line, Column: 3}
	}
	return nativeDocstringIndexSites(t, sites...)
}
func nativeDocstringIndexSites(t *testing.T, sites ...programindex.Location) programindex.Index {
	t.Helper()
	input := programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{Language: "typescript", Kind: "library", Name: "author-claims", Selector: "jsts:package.json", AnchorFileRef: "f1",
			Sources: []programindex.TargetSource{{FileRef: "f1", Path: "src/claims.ts"}}},
		Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: len(sites)},
	}
	for position, site := range sites {
		input.Objects = append(input.Objects, programindex.ObjectInput{SourceRef: strings.Repeat("d", position+1), Kind: programindex.ObjectVariable, Name: "count", Visibility: programindex.VisibilityPublic,
			Location: &programindex.Location{Path: "src/claims.ts", Line: site.Line, Column: site.Column}, DocstringRanges: []programindex.LineRange{{Line: 1, EndLine: 1, Column: 1, EndColumn: len("/** Counts an authored level. */")}}})
	}
	index, err := programindex.New(input)
	if err != nil {
		t.Fatal(err)
	}
	return index
}

func TestNativeJSDocOwnerUsesNameLineAndKeepsAmbiguityUnknown(t *testing.T) {
	for _, row := range []struct {
		name  string
		lines []int
		want  int
	}{{"one exact name", []int{3}, 3}, {"identical native observations", []int{3, 3}, 3}, {"different native name lines", []int{3, 4}, 0}} {
		t.Run(row.name, func(t *testing.T) {
			index := nativeDocstringIndex(t, row.lines...)
			owners, err := nativeDocstringOwners(t.Context(), []func() (programindex.Index, error){func() (programindex.Index, error) { return index, nil }}, nil)
			if err != nil {
				t.Fatal(err)
			}
			quotes, err := jsDocBlocksWithOwners(splitLines("/** Counts an authored level. */\nexport const\n count = 1;\n"), owners["src/claims.ts"])
			if err != nil || len(quotes) != 1 || quotes[0].Line != 1 || quotes[0].DeclarationLine != row.want || quotes[0].Text != "Counts an authored level." {
				t.Fatalf("source quote/native exact owner: %+v", quotes)
			}
		})
	}
	quotes := jsDocBlocks(splitLines("/** Counts an authored level. */\nexport const\n count = 1;\n"))
	if len(quotes) != 1 || quotes[0].DeclarationLine != 0 {
		t.Fatal("missing native metadata was reconstructed from the keyword line")
	}
}

func TestNativeDocstringReaderMustRetainItsSealAndErrors(t *testing.T) {
	index := nativeDocstringIndex(t, 3)
	index.Objects[0].DocstringRanges[0].Column = 2
	if _, err := nativeDocstringOwners(context.Background(), []func() (programindex.Index, error){func() (programindex.Index, error) { return index, nil }}, nil); err == nil {
		t.Fatal("unsealed attachment became author ownership")
	}
	if _, err := nativeDocstringOwners(context.Background(), []func() (programindex.Index, error){nil}, nil); err == nil {
		t.Fatal("missing native reader was accepted")
	}
}

func TestNativeDocstringOwnerKeepsNameColumnsAndConflictingViewsUnknown(t *testing.T) {
	for _, sites := range [][]programindex.Location{
		{{Line: 3, Column: 3}, {Line: 3, Column: 3}},
		{{Line: 3, Column: 3}, {Line: 3, Column: 9}, {Line: 3, Column: 3}},
	} {
		index := nativeDocstringIndexSites(t, sites...)
		owners, err := nativeDocstringOwners(t.Context(), []func() (programindex.Index, error){func() (programindex.Index, error) { return index, nil }}, nil)
		if err != nil {
			t.Fatal(err)
		}
		got, err := jsDocBlocksWithOwners(splitLines("/** Counts an authored level. */\nexport const\n count=1,other=2;"), owners["src/claims.ts"])
		if err != nil || len(got) != 1 {
			t.Fatalf("native quote: %+v %v", got, err)
		}
		want := declarationSite{Line: 3, Column: 3}
		if len(sites) == 3 {
			want = declarationSite{}
		}
		if got[0].DeclarationLine != want.Line || got[0].DeclarationColumn != want.Column {
			t.Fatalf("native name location: %+v, want %+v", got[0], want)
		}
	}
}

func TestNativeDocstringRangesSliceUTF16SourceExactly(t *testing.T) {
	prefix := `export const lead="😀"; export const `
	comment := "/** Counts Unicode levels. */"
	line := prefix + comment + " count=1;"
	column := len(utf16.Encode([]rune(prefix))) + 1
	span := programindex.LineRange{Line: 1, EndLine: 1, Column: column, EndColumn: column + len(comment) - 1}
	for _, ending := range []string{"\n", "\r\n"} {
		got, err := jsDocBlocksWithOwners(splitLines(line+ending), map[programindex.LineRange]declarationSite{span: {Line: 1, Column: span.EndColumn + 2}})
		if err != nil || len(got) != 1 || got[0].Text != "Counts Unicode levels." || got[0].Column != column {
			t.Fatalf("exact UTF16 quote: %+v %v", got, err)
		}
	}
	if _, err := utf16ByteOffset("😀", 2); err == nil {
		t.Fatal("surrogate-pair interior became a source boundary")
	}
	for _, bad := range []programindex.LineRange{
		{Line: 1, EndLine: 1, Column: column + 1, EndColumn: span.EndColumn},
		{Line: 1, EndLine: 1, Column: column, EndColumn: span.EndColumn + 1},
		{Line: 1, EndLine: 2, Column: column, EndColumn: 1},
	} {
		if _, err := jsDocBlocksWithOwners([]string{line}, map[programindex.LineRange]declarationSite{bad: {Line: 1, Column: 50}}); err == nil {
			t.Fatalf("invalid complete native range accepted: %+v", bad)
		}
	}
}

func TestNativeClaimPositionsPreserveAbsentBytesAndSealExactOwnership(t *testing.T) {
	row := Claim{ID: "raw", Source: SourceDocstring, Path: "src/claims.ts", Line: 1, DeclarationLine: 2, Text: "Counts authored levels."}
	absent, err := Seal(Result{Claims: []Claim{row}})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(absent)
	row.Column, row.DeclarationColumn = 0, 0
	empty, err := Seal(Result{Claims: []Claim{row}})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(empty)
	if !bytes.Equal(before, after) || bytes.Contains(before, []byte("column")) || empty.SHA256 != absent.SHA256 {
		t.Fatal("absent optional positions changed existing canonical bytes")
	}
	row.Column, row.DeclarationColumn = 1, 14
	owned, err := Seal(Result{Claims: []Claim{row}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(owned)
	var restored Result
	if err := json.Unmarshal(encoded, &restored); err != nil || restored.Validate() != nil || restored.Claims[0].DeclarationColumn != 14 {
		t.Fatal("exact native ownership did not roundtrip")
	}
	snapshot, err := owned.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Claims[0].DeclarationColumn = 25
	if snapshot.Validate() == nil || owned.Validate() != nil {
		t.Fatal("mutated native owner preserved seal or changed its source")
	}
	for _, bad := range []Claim{
		{ID: "bad", Source: SourceDocstring, Path: "src/claims.ts", Line: 1, DeclarationColumn: 3, Text: row.Text},
		{ID: "bad", Source: SourceDocstring, Path: "src/claims.ts", Line: 1, DeclarationLine: 2, DeclarationColumn: -1, Text: row.Text},
		{ID: "bad", Source: SourceComment, Path: "src/claims.ts", Line: 1, DeclarationLine: 2, DeclarationColumn: 3, Text: row.Text},
	} {
		if _, err := Seal(Result{Claims: []Claim{bad}}); err == nil {
			t.Fatalf("invalid owner accepted: %+v", bad)
		}
	}
}
