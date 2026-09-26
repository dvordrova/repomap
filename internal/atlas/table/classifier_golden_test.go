package table

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/typesafe"
)

var updateGolden = flag.Bool("update", false, "rewrite the Jev request golden")

// goldenWindows cover every form a closed question takes: a catalogue choice
// shown by title, a row's own list, an optional choice with its "none of
// these", a yes-only noul, a yes/no choice, an explicit Ask and a Note.
func goldenWindows() (Definition, []Window) {
	def := Definition{
		Stage: "atlas_golden", Contract: "repomap.atlas.golden.v1", System: "Decide each row <as a reader> & say so.",
		YesAt: 0.8,
		Columns: []Column{
			{Name: "part", Kind: Choice, OptionsFrom: "part_options"},
			{Name: "talks", Kind: Choice, Options: []string{"db", "sdk"}, Optional: true, Note: "the system this row talks to"},
			{Name: "explains", Kind: Choice, Options: []string{"yes"}, Optional: true, Ask: "Does `row` explain `context.part`?"},
			{Name: "key_symbol", Kind: Choice, Options: []string{"yes", "no"}},
		},
	}
	context := []Field{
		{Name: "part", Value: "Serving <HTTP> & co"},
		{Name: "parts", Value: []map[string]any{{"ref": "c1", "title": "Serving"}, {"ref": "c2", "title": "Storage"}}},
		{Name: "part_options", Value: []string{"c1", "c2", "none"}},
		{Name: "count", Value: 3},
	}
	return def, []Window{
		{Context: context, Rows: []Row{
			{ID: "s1", Fields: []Field{{Name: "name", Value: "Serve"}, {Name: "signature", Value: "func(<-chan int) error"}, {Name: "entry", Value: true}}},
			{ID: "s2", Fields: []Field{{Name: "name", Value: "helper"}, {Name: "calls", Value: []string{"Open", "Close"}}, {Name: "part_options", Value: []string{"c2"}}}},
		}},
		{Context: context[:1], Rows: []Row{{ID: "s3", Fields: []Field{{Name: "name", Value: "Main"}, {Name: "line", Value: 12}, {Name: "part_options", Value: []any{"c1", "none"}}}}}},
	}
}

// The exact prepared Jev request is the cache key of every closed decision:
// a changed byte asks Jev again for every row of every run. The golden was
// taken from main before the categorizer interface existed; a deliberate
// change of the request shape rewrites it with -update and says why.
func TestJevRequestBytesMatchTheGolden(t *testing.T) {
	def, windows := goldenWindows()
	client := &typesafe.Client{Model: "jev-1.13.0"}
	var got bytes.Buffer
	for _, window := range windows {
		call, err := ClassifierCall(client, def, window)
		if err != nil {
			t.Fatal(err)
		}
		prepared, err := client.Prepare(call.Prompt, call.Limits)
		if err != nil {
			t.Fatal(err)
		}
		got.Write(prepared.Bytes())
		got.WriteByte('\n')
	}
	golden := filepath.Join("testdata", "jev_request.golden")
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, got.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatalf("the Jev request changed, so every cached decision is asked again:\n got %s\nwant %s", got.Bytes(), want)
	}
}
