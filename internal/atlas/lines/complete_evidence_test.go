package lines

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/typesafe"
)

// A row carries its complete evidence; a larger row goes alone in its own
// request instead of being cut (review A5's probe: 41 directory files
// reached the model as 40, 21 declarations as 20 with the count still 21,
// 4 caller files of 4 calling declarations as 3 of 3, and a long signature
// lost its last parameter's type).
func TestRowsCarryTheirCompleteEvidence(t *testing.T) {
	signature := "function task(" + strings.Repeat("namedParameter: string, ", 9) + "importantFinalType: PaymentAuthorization): Receipt"
	directory := atlas.Place{ID: "d1", Kind: atlas.PlaceDirectory, Path: "pkg", Directory: &atlas.DirectoryFacts{FileCount: 41}}
	for i := 1; i <= 41; i++ {
		directory.Directory.Files = append(directory.Directory.Files, fmt.Sprintf("file%d.go", i))
	}
	file := atlas.Place{ID: "f1", Kind: atlas.PlaceFile, Path: "pkg/work.go", Parent: "d1", File: &atlas.FileFacts{}}
	for i := 1; i <= 21; i++ {
		file.File.Decls = append(file.File.Decls, atlas.Decl{Name: fmt.Sprintf("work%d", i), Kind: "function", LineNo: i, Signature: signature})
	}
	places, calling := map[string]atlas.Place{}, FileCallers{}
	for i := 1; i <= 4; i++ {
		id := fmt.Sprintf("f%d", i+1)
		places[id] = atlas.Place{ID: id, Kind: atlas.PlaceFile, Path: fmt.Sprintf("pkg/caller%d.go", i), File: &atlas.FileFacts{}}
		file.File.Callers = append(file.File.Callers, id)
		calling[id] = []string{"one", "two", "three", "importantFourthCaller"}
	}
	fields := func(row table.Row) map[string]any {
		result := make(map[string]any)
		for _, field := range row.Fields {
			result[field.Name] = field.Value
		}
		return result
	}
	if files := fields(DirectoryRow(directory))["files"].([]string); len(files) != 41 || files[40] != "file41.go" {
		t.Fatalf("directory files: %d", len(files))
	}
	row := fields(FileRow(file, directory, noLines{}, places, calling))
	declarations := row["declarations"].([]map[string]any)
	if row["declaration_count"] != 21 || len(declarations) != 21 {
		t.Fatalf("declarations: %v of %d", row["declaration_count"], len(declarations))
	}
	for _, declaration := range declarations {
		if declaration["signature"] != signature {
			t.Fatalf("a declaration's signature was cut: %q", declaration["signature"])
		}
	}
	callers := row["callers"].([]map[string]any)
	if len(callers) != 4 {
		t.Fatalf("caller files: %d", len(callers))
	}
	for _, caller := range callers {
		if names := caller["declarations"].([]string); len(names) != 4 || names[3] != "importantFourthCaller" {
			t.Fatalf("calling declarations: %v", names)
		}
	}
	symbol := atlas.Place{ID: "s1", Kind: atlas.PlaceSymbol, Path: file.Path, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "task", Kind: "function", Signature: signature}}}
	if got := fields(SymbolRow(symbol, ""))["signature"]; got != signature {
		t.Fatalf("symbol signature: %q", got)
	}
}

// A row no former cut touched keeps the request bytes main sent before the
// cuts were removed (cc415346), so its memo basis and cached answer stay: a
// leaf directory's absent dirs is still [], never null.
func TestARowNoCutTouchedKeepsItsBytes(t *testing.T) {
	want := map[string]string{
		StageDirectories: "{\n  \"table\": \"atlas_directories\",\n  \"fill\": [{\"kind\":\"text\",\"max_runes\":40,\"name\":\"title\",\"note\":\"two to four words for the box\"}, {\"kind\":\"text\",\"max_runes\":160,\"name\":\"line\",\"note\":\"one sentence, what the directory's code does\"}],\n  \"rows\": [\n    {\"key\": \"dir:pkg/leaf\", \"path\": \"pkg/leaf\", \"name\": \"leaf\", \"dirs\": [], \"files\": [\"a.go\",\"b.go\"], \"file_count\": 2}\n  ]\n}\n",
		StageFiles:       "{\n  \"table\": \"atlas_files\",\n  \"fill\": [{\"kind\":\"text\",\"max_runes\":160,\"name\":\"line\",\"note\":\"one sentence, what the file does\"}],\n  \"rows\": [\n    {\"key\": \"file:pkg/leaf/a.go\", \"path\": \"pkg/leaf/a.go\", \"directory_facts\": \"2 files: a.go, b.go\", \"callers\": [{\"declarations\":[\"main\",\"run\"],\"path\":\"pkg/x.go\"},{\"declarations\":[\"serve\"],\"path\":\"pkg/z.go\"}], \"declaration_count\": 2, \"declarations\": [{\"kind\":\"function\",\"name\":\"Open\",\"signature\":\"func Open(path string) (*Store, error)\"},{\"kind\":\"function\",\"name\":\"close\",\"signature\":\"func close(s *Store)\"}]}\n  ]\n}\n",
		StageSymbols:     "{\n  \"table\": \"atlas_symbols\",\n  \"fill\": [{\"kind\":\"choice\",\"name\":\"key_symbol\",\"note\":\"a declaration a newcomer should look at first\",\"options\":[\"yes\",\"no\"]}],\n  \"rows\": [\n    {\"key\": \"s1\", \"path\": \"pkg/leaf/a.go\", \"name\": \"Open\", \"kind\": \"function\", \"signature\": \"func Open(path string) (*Store, error)\", \"callers\": 2, \"calls\": []}\n  ]\n}\n",
	}
	rows := stableRows()
	for i, def := range []table.Definition{Directories(), Files(), SymbolSelection(false)} {
		request, err := table.Request(def, table.Window{Stage: def.Stage, Rows: []table.Row{rows[i]}})
		if err != nil || string(request) != want[def.Stage] {
			t.Fatalf("%s request changed:\n%s\nwant:\n%s (%v)", def.Stage, request, want[def.Stage], err)
		}
	}
}

// A symbol row with a long complete signature still fits Jev's envelope as
// built: FitClassifierWindows sends it unpacked, with the bytes Request
// writes, so no Pack form or refusal stands in for the signature.
func TestALongSignatureGoesToJevAsWritten(t *testing.T) {
	signature := "func Handle(" + strings.Repeat("argument SomeLongTypeName, ", 60) + "last PaymentAuthorization) (Receipt, error)"
	symbol := atlas.Place{ID: "s1", Kind: atlas.PlaceSymbol, Path: "pkg/a.go", Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "Handle", Kind: "function", Signature: signature}}}
	def := SymbolSelection(false)
	window := table.Window{Stage: def.Stage, Rows: []table.Row{SymbolRow(symbol, "")}}
	windows, err := table.FitClassifierWindows(&typesafe.Client{Model: "jev-test"}, def, []table.Window{window})
	if err != nil || len(windows) != 1 || windows[0].Refused != "" {
		t.Fatalf("windows %+v, %v", windows, err)
	}
	built, _ := table.Request(def, window)
	fitted, _ := table.Request(def, windows[0])
	if string(built) != string(fitted) || !strings.Contains(string(fitted), "last PaymentAuthorization) (Receipt, error)") {
		t.Fatalf("the row was packed or cut:\n%s", fitted)
	}
}
