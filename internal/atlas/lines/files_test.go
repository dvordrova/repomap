package lines

import (
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

type noLines struct{}

func (noLines) Line(string) (string, bool) { return "", false }

func TestFileRowDescribesEvidenceWithoutDecidingArchitectureMembership(t *testing.T) {
	caller := atlas.Place{ID: atlas.FileID("pkg/a/x.go"), Kind: atlas.PlaceFile, Path: "pkg/a/x.go", File: &atlas.FileFacts{Doc: "Entry.", Decls: []atlas.Decl{
		{Name: "Exported", Kind: "function", Exported: true, Doc: "Leading."}, {Name: "Main", Kind: "function"}}}}
	silent := atlas.Place{ID: atlas.FileID("pkg/a/z.go"), Kind: atlas.PlaceFile, Path: "pkg/a/z.go", File: &atlas.FileFacts{Decls: []atlas.Decl{{Name: "Z", Kind: "type", Exported: true}}}}
	directory := atlas.Place{ID: atlas.DirectoryID("pkg/a"), Kind: atlas.PlaceDirectory, Path: "pkg/a", Given: "2 files"}
	file := atlas.Place{ID: atlas.FileID("pkg/a/y.go"), Kind: atlas.PlaceFile, Path: "pkg/a/y.go", Parent: directory.ID,
		File: &atlas.FileFacts{Decls: []atlas.Decl{{Name: "help", Kind: "function"}}, Callers: []string{caller.ID, silent.ID}}}
	places := map[string]atlas.Place{caller.ID: caller, silent.ID: silent, directory.ID: directory, file.ID: file}
	calling := FileCallers{caller.ID: {"Main", "Main$1", "run", "fourth"}}
	fields := func(row table.Row) map[string]any {
		result := make(map[string]any)
		for _, field := range row.Fields {
			result[field.Name] = field.Value
		}
		return result
	}
	alone := fields(FileRow(file, directory, noLines{}, places, calling))
	if alone["box_options"] != nil {
		t.Fatalf("a file without a sibling box was offered a placement: %+v", alone)
	}
	callers := alone["callers"].([]map[string]any)
	if len(callers) != 2 || !reflect.DeepEqual(callers[0]["declarations"], []string{"Main", "Main$1", "run"}) || callers[0]["path"] != "pkg/a/x.go" {
		t.Fatalf("calling declarations are not the graph's witnesses, bounded: %+v", callers)
	}
	if _, listed := callers[1]["declarations"]; listed || callers[1]["path"] != "pkg/a/z.go" {
		t.Fatalf("a caller without witnesses borrowed leading declarations: %+v", callers[1])
	}
	def := Files()
	first, second := FileRow(file, directory, noLines{}, places, nil), FileRow(file, directory, noLines{}, places, nil)
	second.ID = "f2"
	windows, err := table.Windows(def, 1, []table.Row{first, second})
	if err != nil || len(windows) != 1 {
		t.Fatalf("windows: %v", err)
	}
	result, err := table.DecodeResult(def, windows[0], []byte(`{"rows":[{"key":"file:pkg/a/y.go","line":"Helps."},{"key":"f2","line":"Helps too."}]}`))
	if err != nil || len(result.Rejections) != 0 {
		t.Fatalf("omitted box refused: %+v / %v", result, err)
	}
	if _, asked := result.Answers[0]["box"]; asked || result.Answers[1]["box"] != "" {
		t.Fatalf("file descriptions acquired membership: %+v", result.Answers)
	}
	moved, err := table.DecodeResult(def, windows[0], []byte(`{"rows":[{"key":"file:pkg/a/y.go","line":"Helps.","box":"pkg/b"},{"key":"f2","line":"Helps too.","box":"new: Helpers"}]}`))
	if err != nil || len(moved.Rejections) != 0 || moved.Answers[1]["box"] != "" {
		t.Fatalf("unsolicited box choice gained authority: %+v / %v", moved, err)
	}
	if _, asked := moved.Answers[0]["box"]; asked {
		t.Fatalf("a box answer on a row without options gained authority: %+v", moved.Answers[0])
	}
}

// A directory is said by what it holds: its README's first line is the
// reader's fallback (the place's Given), never a model input, in its own
// row, as its children's parent or as a file's directory facts.
func TestDirectoryRowLeavesTheParentToTheWindowContext(t *testing.T) {
	parent := atlas.Place{ID: atlas.DirectoryID("pkg"), Kind: atlas.PlaceDirectory, Path: "pkg", Given: "Packages for the service.",
		Directory: &atlas.DirectoryFacts{Readme: "Packages for the service.", Dirs: []string{"a", "b"}, Files: []string{"doc.go", "x.go"}, FileCount: 4}}
	child := atlas.Place{ID: atlas.DirectoryID("pkg/a"), Kind: atlas.PlaceDirectory, Path: "pkg/a", Parent: parent.ID, Directory: &atlas.DirectoryFacts{Dirs: []string{}, Files: []string{"x.go"}, FileCount: 1}}
	for _, field := range DirectoryRow(child).Fields {
		if field.Name == "parent" {
			t.Fatal("parent repeated in the row")
		}
	}
	for _, field := range DirectoryRow(parent).Fields {
		if field.Name == "readme" || field.Value == parent.Directory.Readme {
			t.Fatalf("a directory row carries its README line: %+v", field)
		}
	}
	context := DirectoryContext(&parent)
	if len(context) != 1 || context[0].Name != "parent" || !reflect.DeepEqual(context[0].Value, map[string]any{"path": "pkg", "line": "4 files: a, b, doc.go"}) {
		t.Fatalf("parent context: %+v", context)
	}
	file := atlas.Place{ID: atlas.FileID("pkg/doc.go"), Kind: atlas.PlaceFile, Path: "pkg/doc.go", Parent: parent.ID, File: &atlas.FileFacts{}}
	var facts any
	for _, field := range FileRow(file, parent, noLines{}, nil, nil).Fields {
		if field.Name == "directory_facts" {
			facts = field.Value
		}
	}
	if facts != "4 files: a, b, doc.go" {
		t.Fatalf("a file's directory facts: %+v", facts)
	}
	if DirectoryContext(nil) != nil {
		t.Fatal("a root row gained a parent")
	}
}
