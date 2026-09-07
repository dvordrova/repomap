package table

import (
	"bytes"
	"encoding/json"
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

func TestSequencePreservesOrderAndFiltersOnlyExactKnownRefs(t *testing.T) {
	column := Column{Name: "order", Kind: Sequence, OptionsFrom: "options", LimitFrom: "limit"}
	row := Row{Fields: []Field{{Name: "options", Value: []string{"c1", "c2", "c3"}}, {Name: "limit", Value: 2}}}
	for input, expected := range map[string]string{"c3 c1": "c3 c1", "c3 c999 c3 c1": "c3 c1", "none": ""} {
		value, err := normalizeCell(column, row, input)
		if err != nil || value != expected {
			t.Fatalf("%q -> %q, %v", input, value, err)
		}
	}
	for _, input := range []string{"", "c999", "c01", "c1 c2 c3", "c1,c2"} {
		if _, err := normalizeCell(column, row, input); err == nil {
			t.Fatalf("accepted %q", input)
		}
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
	withContext := first[0]
	withContext.Context = []Field{{Name: "question", Value: "Where is state?"}, {Name: "repository", Value: "example/repository"}}
	raw, err := Request(def, withContext)
	if err != nil {
		t.Fatal(err)
	}
	const existingRequest = `{
  "table": "atlas_test",
  "fill": [{"kind":"text","max_runes":20,"name":"line"}, {"free_prefix":"new: ","kind":"choice","name":"box","options_from":"box_options"}],
  "context": {"question": "Where is state?", "repository": "example/repository"},
  "rows": [
    {"key": "r1", "path": "a.go", "box_options": ["here","pkg/b"]},
    {"key": "r2", "path": "b.go", "box_options": ["here"]}
  ]
}
`
	if string(raw) != existingRequest {
		t.Fatalf("default request bytes changed:\n%s", raw)
	}
	def.ContextAfterRows = true
	withoutContext, err := Request(def, first[0])
	if err != nil || !bytes.Equal(withoutContext, first[0].Request) {
		t.Fatalf("context placement changed a request without context: %v", err)
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

func TestChoiceAcceptsAUniquePrefix(t *testing.T) {
	def := testDefinition()
	def.Columns[1] = Column{Name: "box", Kind: Choice, Options: []string{"Utilities and configuration", "Utilities and logging", "Storage"}}
	windows, _ := Windows(def, 1, testRows()[:1])
	if _, err := Decode(def, windows[0], []byte(`{"rows":[{"key":"r1","line":"a","box":"Utilities and"}]}`)); err == nil {
		t.Fatal("an ambiguous prefix was accepted")
	}
	answers, err := Decode(def, windows[0], []byte(`{"rows":[{"key":"r1","line":"a","box":"Stor"}]}`))
	if err != nil || answers[0]["box"] != "Storage" {
		t.Fatalf("unique prefix: %v %v", answers, err)
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

func TestProsePreservesParagraphsAndTheCompleteQualification(t *testing.T) {
	def := testDefinition()
	def.Columns[0] = Column{Name: "line", Kind: Prose}
	windows, err := Windows(def, 1, testRows()[:1])
	if err != nil {
		t.Fatal(err)
	}
	text := "The declared default is 'two  spaces'.\n\n" + strings.Repeat("A supported observation. ", 45) + "\n\nThis does not establish runtime behavior."
	raw, _ := json.Marshal(map[string]any{"rows": []map[string]string{{"key": "r1", "line": " \n" + text + "\n ", "box": "here"}}})
	answers, err := Decode(def, windows[0], raw)
	if err != nil || answers[0]["line"] != text {
		t.Fatalf("prose was changed: %v, %v", answers, err)
	}
	if _, err := Decode(def, windows[0], []byte(`{"rows":[{"key":"r1","line":" \n ","box":"here"}]}`)); err == nil {
		t.Fatal("accepted empty prose")
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
	other = def
	other.Reasoning = true
	d, _ := State(other, windows[0])
	if bytes.Equal(a, d) {
		t.Fatal("a changed reasoning preference kept the table identity")
	}
	call, err := Call(other, windows[0])
	if err != nil || !call.Prompt.Reasoning {
		t.Fatalf("table reasoning preference did not reach request preparation: %v", err)
	}
	if call.Limits.MaxOutputTokens != 128000 {
		t.Fatalf("table narrowed the shared output envelope: %d", call.Limits.MaxOutputTokens)
	}
}

func TestWindowsSplitByInputBytesWithoutLosingRowsOrContext(t *testing.T) {
	def := testDefinition()
	def.Window = 40
	rows := testRows()
	for i := range rows {
		rows[i].Fields = append(rows[i].Fields, Field{Name: "doc", Value: strings.Repeat("ю", 700)})
	}
	shared := []Field{{Name: "purpose", Value: "one question shared by all rows"}}
	one, err := Request(def, Window{Rows: rows[:1], Context: shared})
	if err != nil {
		t.Fatal(err)
	}
	def.MaxInputBytes = len(def.System) + len(one)
	windows, err := WindowsWithContext(def, 1, shared, rows)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, window := range windows {
		if len(window.Request)+len(def.System) > def.MaxInputBytes {
			t.Fatal("oversized request")
		}
		if !strings.Contains(string(window.Request), "one question shared by all rows") {
			t.Fatal("shared context disappeared")
		}
		for _, row := range window.Rows {
			ids = append(ids, row.ID)
		}
	}
	if len(windows) != 3 || strings.Join(ids, ",") != "file:a.go,file:b.go,file:c.go" {
		t.Fatalf("lost or reordered rows: %v", ids)
	}
	def.MaxInputBytes--
	if _, err := WindowsWithContext(def, 1, shared, rows); err == nil || !strings.Contains(err.Error(), "row file:a.go") {
		t.Fatalf("an oversized single row was hidden: %v", err)
	}
}

func TestRequestDoesNotReplaceUnencodableEvidenceWithNull(t *testing.T) {
	rows := testRows()
	rows[0].Fields = append(rows[0].Fields, Field{Name: "evidence", Value: make(chan int)})
	if _, err := Windows(testDefinition(), 1, rows); err == nil {
		t.Fatal("invalid evidence silently became null")
	}
	for _, afterRows := range []bool{false, true} {
		def := testDefinition()
		def.ContextAfterRows = afterRows
		if _, err := WindowsWithContext(def, 1, []Field{{Name: "context", Value: make(chan int)}}, testRows()); err == nil {
			t.Fatalf("invalid context silently became null with ContextAfterRows=%t", afterRows)
		}
	}
}
