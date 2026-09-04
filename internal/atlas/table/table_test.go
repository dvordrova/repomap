package table

import (
	"bytes"
	"strings"
	"testing"
)

func testDefinition() Definition {
	return Definition{
		Stage: "atlas_test", Contract: "repomap.atlas.test.v1", Window: 2, System: "fill the table",
		Columns: []Column{
			{Name: "line", Kind: Text, MaxRunes: 20},
			{Name: "box", Kind: Choice, OptionsFrom: "box_options", Free: "new: ", FreeMaxRunes: 10},
		},
	}
}

func testRows() []Row {
	return []Row{
		{ID: "file:a.go", Fields: []Field{{Name: "path", Value: "a.go"}, {Name: "box_options", Value: []string{"here", "pkg/b"}}}},
		{ID: "file:b.go", Fields: []Field{{Name: "path", Value: "b.go"}, {Name: "box_options", Value: []string{"here"}}}},
		{ID: "file:c.go", Fields: []Field{{Name: "path", Value: "c.go"}, {Name: "box_options", Value: []string{"here"}}}},
	}
}

func TestWindowsKeepOrderAndBytesAreStable(t *testing.T) {
	def := testDefinition()
	first, err := Windows(def, 1, testRows())
	if err != nil {
		t.Fatal(err)
	}
	second, _ := Windows(def, 1, testRows())
	if len(first) != 2 || len(first[0].Rows) != 2 || len(first[1].Rows) != 1 {
		t.Fatalf("windows: %d, sizes %d/%d", len(first), len(first[0].Rows), len(first[1].Rows))
	}
	if !bytes.Equal(first[0].Request, second[0].Request) {
		t.Fatal("the same rows produced different request bytes")
	}
	request := string(first[0].Request)
	if !strings.Contains(request, `{"key": "r1", "path": "a.go", "box_options": ["here","pkg/b"]}`) {
		t.Fatalf("request does not carry the row in field order:\n%s", request)
	}
	if strings.Index(request, `"path": "a.go"`) > strings.Index(request, `"path": "b.go"`) {
		t.Fatal("rows are out of order")
	}
	if strings.Contains(request, "file:a.go") {
		t.Fatal("the request carries the code-side row ID")
	}
}

func TestDecodeAcceptsEveryKeyOnce(t *testing.T) {
	def := testDefinition()
	windows, _ := Windows(def, 1, testRows())
	raw := []byte("```json\n{\"rows\":[{\"key\":\"r2\",\"line\":\"  reads   b \",\"box\":\"HERE\"},{\"key\":\"r1\",\"line\":\"writes a\",\"box\":\"new:  Config loading and parsing \"}]}\n```")
	answers, err := Decode(def, windows[0], raw)
	if err != nil {
		t.Fatal(err)
	}
	if answers[0]["line"] != "writes a" || answers[1]["line"] != "reads b" {
		t.Fatalf("lines: %v", answers)
	}
	if answers[1]["box"] != "here" {
		t.Fatalf("choice was not normalized: %q", answers[1]["box"])
	}
	if got := answers[0]["box"]; !strings.HasPrefix(got, "new: ") || strings.Count(got, " ") > 3 {
		t.Fatalf("free choice was not bounded: %q", got)
	}
	if title, ok := IsFree(def.Columns[1], answers[0]["box"]); !ok || title == "" {
		t.Fatalf("IsFree: %q %v", title, ok)
	}
}

func TestDecodeRefusesBadWindows(t *testing.T) {
	def := testDefinition()
	windows, _ := Windows(def, 1, testRows())
	cases := map[string]string{
		"missing key":    `{"rows":[{"key":"r1","line":"a","box":"here"}]}`,
		"duplicate key":  `{"rows":[{"key":"r1","line":"a","box":"here"},{"key":"r1","line":"b","box":"here"}]}`,
		"unknown key":    `{"rows":[{"key":"r1","line":"a","box":"here"},{"key":"r9","line":"b","box":"here"}]}`,
		"extra cell":     `{"rows":[{"key":"r1","line":"a","box":"here","why":"x"},{"key":"r2","line":"b","box":"here"}]}`,
		"missing cell":   `{"rows":[{"key":"r1","line":"a"},{"key":"r2","line":"b","box":"here"}]}`,
		"bad choice":     `{"rows":[{"key":"r1","line":"a","box":"pkg/z"},{"key":"r2","line":"b","box":"here"}]}`,
		"empty text":     `{"rows":[{"key":"r1","line":"   ","box":"here"},{"key":"r2","line":"b","box":"here"}]}`,
		"empty free":     `{"rows":[{"key":"r1","line":"a","box":"new: "},{"key":"r2","line":"b","box":"here"}]}`,
		"extra envelope": `{"rows":[{"key":"r1","line":"a","box":"here"},{"key":"r2","line":"b","box":"here"}],"notes":"x"}`,
		"not json":       `rows: r1 a here`,
	}
	for name, raw := range cases {
		if _, err := Decode(def, windows[0], []byte(raw)); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestTextIsCutAtAWord(t *testing.T) {
	def := testDefinition()
	windows, _ := Windows(def, 1, testRows()[:1])
	raw := `{"rows":[{"key":"r1","line":"this line is much longer than twenty runes","box":"here"}]}`
	answers, err := Decode(def, windows[0], []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if got := answers[0]["line"]; len([]rune(got)) > 20 || !strings.HasSuffix(got, "…") {
		t.Fatalf("line was not cut: %q", got)
	}
}

func TestStateIgnoresTheClock(t *testing.T) {
	def := testDefinition()
	windows, _ := Windows(def, 1, testRows())
	a, _ := State(def, windows[0])
	b, _ := State(def, windows[0])
	if !bytes.Equal(a, b) {
		t.Fatal("state is not stable")
	}
	other := def
	other.System = "another prompt"
	c, _ := State(other, windows[0])
	if bytes.Equal(a, c) {
		t.Fatal("a changed prompt kept the cache identity")
	}
}
