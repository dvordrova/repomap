package table

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
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
	column := Column{Name: "order", Kind: Sequence, OptionsFrom: "options"}
	row := Row{Fields: []Field{{Name: "options", Value: []string{"c1", "c2", "c3"}}}}
	// Unknown refs were never selectable and drop out; a selection of only
	// unknown refs is an empty selection, not a refused row. Commas separate
	// refs as whitespace does. The options list is the only bound: a
	// selection can never hold more refs than it offers.
	for input, expected := range map[string]string{"c3 c1": "c3 c1", "c3 c999 c3 c1": "c3 c1", "none": "", "": "", "c999": "", "c01": "", "c1,c2": "c1 c2", "c1 c2 c3 c1": "c1 c2 c3"} {
		value, err := normalizeCell(column, row, input)
		if err != nil || value != expected {
			t.Fatalf("%q -> %q, %v", input, value, err)
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

func TestResponseExampleDependsOnColumnsNotBatchMembership(t *testing.T) {
	def := testDefinition()
	windows, err := Windows(def, 1, testRows())
	if err != nil {
		t.Fatal(err)
	}
	first, err := Call(def, windows[0])
	if err != nil {
		t.Fatal(err)
	}
	second, err := Call(def, windows[1])
	if err != nil || first.Prompt.ResponseExample != second.Prompt.ResponseExample {
		t.Fatalf("the owner shape changed with batch neighbors: %v", err)
	}
	var example struct {
		Rows []map[string]string `json:"rows"`
	}
	if err := json.Unmarshal([]byte(first.Prompt.ResponseExample), &example); err != nil || len(example.Rows) != 1 {
		t.Fatalf("response example lost its table object: %s / %v", first.Prompt.ResponseExample, err)
	}
	row := example.Rows[0]
	if len(row) != len(def.Columns)+1 || row["key"] != "r1" || row["line"] == "" || row["box"] == "" {
		t.Fatalf("response example does not contain the owner columns: %+v", row)
	}
	def.Columns = append(def.Columns, Column{Name: "activation", Kind: Text})
	changed, err := Call(def, windows[0])
	if err != nil || changed.Prompt.ResponseExample == first.Prompt.ResponseExample || !strings.Contains(changed.Prompt.ResponseExample, `"activation"`) {
		t.Fatalf("response example ignored a contract column change: %s / %v", changed.Prompt.ResponseExample, err)
	}
}

// The example is what the model copies. Its key comes first and the cells
// follow the fill order, on one line, whatever the column names sort to.
func TestResponseExampleListsKeyFirstThenColumnsInFillOrder(t *testing.T) {
	def := Definition{Columns: []Column{
		{Name: "zone", Kind: Choice, Options: []string{"a"}},
		{Name: "entry", Kind: Choice, Options: []string{"self", "none"}},
		{Name: "activation", Kind: Choice, Options: []string{"command"}},
		{Name: `na"me`, Kind: Text},
	}}
	example := ResponseExample(def)
	want := `{"rows":[{"key":"r1","zone":"<computed zone>","entry":"<computed entry>","activation":"<computed activation>","na\"me":"<computed na\"me>"}]}`
	if example != want {
		t.Fatalf("example = %s\nwant      %s", example, want)
	}
	var decoded struct {
		Rows []map[string]string `json:"rows"`
	}
	if err := json.Unmarshal([]byte(example), &decoded); err != nil || len(decoded.Rows) != 1 || len(decoded.Rows[0]) != 5 || strings.Contains(example, "\n") {
		t.Fatalf("example is not one valid JSON line: %v", err)
	}
	call, err := Call(def, Window{})
	if err != nil || call.Prompt.ResponseExample != example {
		t.Fatalf("the prepared call does not carry the ordered example: %v", err)
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
	def.Window = 0
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

func TestWindowsKeepOversizedRowsWholeBetweenOrdinaryBatches(t *testing.T) {
	for _, largeContext := range []bool{false, true} {
		t.Run(fmt.Sprintf("large-context-%t", largeContext), func(t *testing.T) {
			def := testDefinition()
			def.Window = 0
			shared := []Field{{Name: "purpose", Value: "All original evidence."}}
			large := strings.Repeat("ю\"<&\n", 20_000)
			rows := []Row{
				{ID: "before-1", Fields: []Field{{Name: "doc", Value: "before"}}},
				{ID: "before-2", Fields: []Field{{Name: "doc", Value: "before"}}},
				{ID: "large", Fields: []Field{{Name: "doc", Value: large}}},
				{ID: "after-1", Fields: []Field{{Name: "doc", Value: "after"}}},
				{ID: "after-2", Fields: []Field{{Name: "doc", Value: "after"}}},
			}
			wantSizes := []int{2, 1, 2}
			if largeContext {
				shared[0].Value = large
				wantSizes = []int{1, 1, 1, 1, 1}
			}
			windows, err := WindowsWithContext(def, 2, shared, rows)
			if err != nil {
				t.Fatal(err)
			}
			var sizes []int
			offset := 0
			for i, window := range windows {
				sizes = append(sizes, len(window.Rows))
				if window.Index != i || window.Round != 2 || !reflect.DeepEqual(window.Rows, rows[offset:offset+len(window.Rows)]) {
					t.Fatal("packing changed row identity, order or evidence")
				}
				if len(def.System)+len(window.Request) > DefaultInputBytes && len(window.Rows) != 1 {
					t.Fatal("an oversized row absorbed its neighbours")
				}
				var decoded struct {
					Context map[string]string   `json:"context"`
					Rows    []map[string]string `json:"rows"`
				}
				if err := json.Unmarshal(window.Request, &decoded); err != nil {
					t.Fatal(err)
				}
				if decoded.Context["purpose"] != shared[0].Value || len(decoded.Rows) != len(window.Rows) {
					t.Fatal("request lost complete shared context or rows")
				}
				for j, row := range decoded.Rows {
					if row["key"] != Key(j) || row["doc"] != rows[offset+j].Fields[0].Value {
						t.Fatal("request changed escaped UTF-8 evidence or its local key")
					}
				}
				offset += len(window.Rows)
			}
			if offset != len(rows) || !reflect.DeepEqual(sizes, wantSizes) {
				t.Fatalf("window sizes %v, want %v; kept %d rows", sizes, wantSizes, offset)
			}
		})
	}
}

func TestWindowsGreedilyFillByteBudgetWithExactLocalKeys(t *testing.T) {
	doc := strings.Repeat("ю\"<&\n", 20)
	var rows []Row
	for i := 0; i < 25; i++ {
		rows = append(rows, Row{ID: fmt.Sprintf("private-row-%d", i), Fields: []Field{{Name: "doc", Value: doc}}})
	}
	shared := []Field{{Name: "purpose", Value: "Keep complete evidence."}}
	for _, afterRows := range []bool{false, true} {
		def := testDefinition()
		def.ContextAfterRows = afterRows
		ten, err := Request(def, Window{Context: shared, Rows: rows[:10]})
		if err != nil {
			t.Fatal(err)
		}
		budget := len(def.System) + len(ten)
		for _, test := range []struct {
			name   string
			cap    int
			budget int
			sizes  []int
		}{
			{name: "byte only", budget: budget, sizes: []int{10, 10, 5}},
			{name: "one byte short of r10", budget: budget - 1, sizes: []int{9, 9, 7}},
			{name: "explicit row cap", cap: 7, budget: budget, sizes: []int{7, 7, 7, 4}},
		} {
			t.Run(fmt.Sprintf("%s/context-after-%t", test.name, afterRows), func(t *testing.T) {
				def.Window, def.MaxInputBytes = test.cap, test.budget
				windows, err := WindowsWithContext(def, 3, shared, rows)
				if err != nil {
					t.Fatal(err)
				}
				var sizes []int
				offset := 0
				for i, window := range windows {
					sizes = append(sizes, len(window.Rows))
					if window.Stage != def.Stage || window.Round != 3 || window.Index != i || !reflect.DeepEqual(window.Rows, rows[offset:offset+len(window.Rows)]) {
						t.Fatal("rebatching changed row order or round provenance")
					}
					if len(def.System)+len(window.Request) > test.budget {
						t.Fatal("packed request exceeded its exact input budget")
					}
					var input struct {
						Context map[string]string   `json:"context"`
						Rows    []map[string]string `json:"rows"`
					}
					if err := json.Unmarshal(window.Request, &input); err != nil {
						t.Fatal(err)
					}
					if input.Context["purpose"] != shared[0].Value || len(input.Rows) != len(window.Rows) {
						t.Fatal("request lost shared context or complete rows")
					}
					for j, row := range input.Rows {
						if row["key"] != Key(j) || row["doc"] != doc {
							t.Fatal("request key or escaped UTF-8 evidence changed")
						}
					}
					offset += len(window.Rows)
				}
				if !reflect.DeepEqual(sizes, test.sizes) || offset != len(rows) {
					t.Fatalf("underfilled windows or lost rows: %v, want %v", sizes, test.sizes)
				}
			})
		}
	}
	def := testDefinition()
	def.Window = 0
	if windows, err := Windows(def, 0, nil); err != nil || len(windows) != 0 {
		t.Fatalf("empty input produced a window or error: %v", err)
	}
	def.Window = -1
	if _, err := Windows(def, 0, rows); err == nil {
		t.Fatal("negative row budget was accepted")
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

func TestChoiceWithOnlyUnknownAcceptsAnyAnswerAsUnknown(t *testing.T) {
	column := Column{Name: "address", Kind: Choice, OptionsFrom: "address_options"}
	only := Row{Fields: []Field{{Name: "address_options", Value: []string{"unknown"}}}}
	if got, err := normalizeCell(column, only, "{param}/health"); err != nil || got != "unknown" {
		t.Fatalf("the only possible address was refused: %q / %v", got, err)
	}
	offered := Row{Fields: []Field{{Name: "address_options", Value: []string{"a1", "unknown"}}}}
	if _, err := normalizeCell(column, offered, "{param}/health"); err == nil || !strings.Contains(err.Error(), "not one of the options") {
		t.Fatalf("a copied path replaced an offered address ref: %v", err)
	}
	if got, err := normalizeCell(column, offered, "a1"); err != nil || got != "a1" {
		t.Fatalf("an offered ref was refused: %q / %v", got, err)
	}
}

func TestSequenceEmptyOrNullIsAnEmptySelection(t *testing.T) {
	column := Column{Name: "outbound", Kind: Sequence, OptionsFrom: "call_options"}
	row := Row{Fields: []Field{{Name: "call_options", Value: []string{"c1", "c2"}}}}
	for _, cell := range []string{"none", "", "  "} {
		if got, err := normalizeCell(column, row, cell); err != nil || got != "" {
			t.Fatalf("empty selection %q refused: %q / %v", cell, got, err)
		}
	}
	if got, err := normalizeCell(column, row, "c9 c12"); err != nil || got != "" {
		t.Fatalf("refs outside the options did not settle as an empty selection: %q / %v", got, err)
	}
	if got, err := normalizeCell(column, row, "c9 c2"); err != nil || got != "c2" {
		t.Fatalf("a known ref beside an unknown one was lost: %q / %v", got, err)
	}
	def := Definition{Stage: "atlas_symbols", Independent: true, Columns: []Column{column}}
	result, err := DecodeResult(def, Window{Rows: []Row{row}}, []byte(`{"rows":[{"key":"r1","outbound":null}]}`))
	if err != nil || result.Answers[0] == nil || result.Answers[0]["outbound"] != "" {
		t.Fatalf("null selection refused the row: %+v / %v", result, err)
	}
}
