package facts

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

func TestTODOsKeepCumulativeFixtureAnchorsAcrossLargeFiles(t *testing.T) {
	for _, fixture := range []struct {
		language, path, comment string
	}{
		{"go", "internal/storefixture/level_responses.go", "//"},
		{"python", "src/fixture_app/models.py", "#"},
		{"jsts", "src/type-members.ts", "//"},
	} {
		t.Run(fixture.language, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(repositoryRoot(t), "testdata", "repositories", fixture.language, filepath.FromSlash(fixture.path)))
			if err != nil {
				t.Fatal(err)
			}
			marker := fixture.comment + " TODO: document count validation."
			for _, size := range []struct {
				name, newline string
				bytes         int
			}{
				{"below former cutoff", "\n", (1 << 20) - 1},
				{"at former cutoff", "\n", 1 << 20},
				{"marker beyond former cutoff", "\n", (1 << 20) + len(marker) + 8},
				{"CRLF beyond former cutoff", "\r\n", (1 << 20) + len(marker) + 8},
			} {
				t.Run(size.name, func(t *testing.T) {
					source := strings.ReplaceAll(string(raw), "\n", size.newline)
					if strings.Count(source, marker) != 1 {
						t.Fatal("fixture must contain exactly one original task")
					}
					firstLine := strings.Count(source[:strings.Index(source, marker)], "\n") + 1
					lastLine := strings.Count(source, "\n") + 2
					padding := size.bytes - len(source) - len(fixture.comment) - len(size.newline) - len(marker) - len(size.newline)
					source += fixture.comment + strings.Repeat(" ", padding) + size.newline + marker + size.newline
					if len(source) != size.bytes {
						t.Fatalf("fixture bytes = %d, want %d", len(source), size.bytes)
					}
					repository := newCorpus(t, map[string]string{fixture.path: source})
					result := mustBuild(t, Input{Repository: repository})
					rows := result.OfKind(KindTODO)
					if len(rows) != 2 {
						t.Fatalf("%d-byte file retained %d/2 tasks: %+v", len(source), len(rows), rows)
					}
					for _, line := range []int{firstLine, lastLine} {
						requireFact(t, result, KindTODO, "original count task", func(row Fact) bool {
							return row.Anchor.Path == fixture.path && row.Anchor.Line == line && row.Key == "TODO" && row.Text == "document count validation."
						})
					}
					if rows[0].ID == rows[1].ID {
						t.Fatal("source-distinct tasks lost their separate identities")
					}
				})
			}
		})
	}
}

func TestLargeGoTODOScanStillIgnoresCallsAndStrings(t *testing.T) {
	source := "package sample\n" +
		"var ctx = context.TODO()\n" +
		"var text = `// TODO not a comment`\n" +
		"var escaped = \"TODO: not work\"\n" +
		"//line fake.go:900\n" +
		"func f() { context.TODO() /* TODO: retry later */ }\n" +
		"/* notes\n * FIXME: preserve this anchor\n */\n" +
		"//" + strings.Repeat(" ", 1<<20) + "\n"
	result := mustBuild(t, Input{Repository: newCorpus(t, map[string]string{"sample.go": source})})
	if rows := result.OfKind(KindTODO); len(rows) != 2 {
		t.Fatalf("large-file calls or strings became tasks: %+v", rows)
	}
	for _, expected := range []struct {
		line int
		text string
	}{{6, "retry later"}, {8, "preserve this anchor"}} {
		requireFact(t, result, KindTODO, expected.text, func(row Fact) bool {
			return row.Anchor.Path == "sample.go" && row.Anchor.Line == expected.line && row.Text == expected.text
		})
	}
}

// A TODO is a marker a comment in source code writes (owner, 2026-09-29:
// Redis's ten "TODOs" were <a name="TODO"> anchors of its doc/*.html). A
// document's, a page's or a data file's marker is none; a marker in a
// string is none; a comment's is one in every code language, whether an
// adapter analyses it (C, Python, Clojure) or not (Tcl).
func TestTODOsAreCommentsOfCodeFiles(t *testing.T) {
	repository := newCorpus(t, map[string]string{
		"doc/README.html":   "<a name=\"TODO\">TODO</a>\n",
		"NOTES.md":          "TODO: write the notes\n",
		"data/todo.json":    "{\"TODO\": 1}\n",
		"server.c":          "char *s = \"TODO: not a comment\";\n/* TODO: free the list */\nint x; // FIXME: overflow\n/* notes\n * XXX: retry\n */\n",
		"tool.py":           "text = \"TODO: data\"  # TODO: split\n",
		"core.clj":          "(def x \"TODO\") ; HACK: cache it\n",
		"tests/test-kv.tcl": "puts \"TODO\"\n# TODO: cover expiry\n",
	})
	result := mustBuild(t, Input{Repository: repository})
	var said []string
	for _, row := range result.OfKind(KindTODO) {
		said = append(said, fmt.Sprintf("%s:%d %s %s", row.Anchor.Path, row.Anchor.Line, row.Key, row.Text))
	}
	sort.Strings(said)
	want := []string{"core.clj:1 HACK cache it", "server.c:2 TODO free the list", "server.c:3 FIXME overflow", "server.c:5 XXX retry",
		"tests/test-kv.tcl:2 TODO cover expiry", "tool.py:1 TODO split"}
	if !slices.Equal(said, want) {
		t.Fatalf("TODOs = %q\nwant %q", said, want)
	}
}
