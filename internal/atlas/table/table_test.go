package table

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
)

type memoTestProvider struct{}

func (memoTestProvider) State() []byte { return []byte(`{"model":"test"}`) }
func (memoTestProvider) Prepare(llm.Prompt, llm.Limits) (llm.Prepared, error) {
	return llm.Prepared{}, fmt.Errorf("memo identity prepared a pretend provider request")
}
func (memoTestProvider) Complete(context.Context, llm.Prepared) (llm.Completion, error) {
	return llm.Completion{}, fmt.Errorf("unexpected provider call")
}

func testDefinition() Definition {
	return Definition{
		Stage: "atlas_test", Contract: "repomap.atlas.test.v1", Window: 2, System: "fill the table",
		Columns: []Column{
			{Name: "line", Kind: Text, MaxRunes: 20},
			{Name: "box", Kind: Choice, OptionsFrom: "box_options", Free: "new: ", FreeMaxRunes: 10},
		},
	}
}

func TestArtifactIDsReachProviderButDoNotSplitEqualEvidenceMemos(t *testing.T) {
	def := testDefinition()
	first := Window{Rows: []Row{{ID: "f1", Fields: []Field{{Name: "path", Value: "same.go"}}}}}
	second := Window{Rows: []Row{{ID: "f9", Fields: []Field{{Name: "path", Value: "same.go"}}}}}
	firstRequest, err := Request(def, first)
	if err != nil {
		t.Fatal(err)
	}
	secondRequest, err := Request(def, second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(firstRequest, []byte(`"key": "f1"`)) || !bytes.Contains(secondRequest, []byte(`"key": "f9"`)) || bytes.Contains(firstRequest, []byte(`"key": "r1"`)) {
		t.Fatalf("provider rows were renamed: %s / %s", firstRequest, secondRequest)
	}
	a, err := MemoIdentity(memoTestProvider{}, def, first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := MemoIdentity(memoTestProvider{}, def, second)
	if err != nil || a != b {
		t.Fatalf("owner ID split equal evidence: %s / %s / %v", a, b, err)
	}
	second.Rows[0].Fields[0].Value = "changed.go"
	c, err := MemoIdentity(memoTestProvider{}, def, second)
	if err != nil || c == a {
		t.Fatalf("changed evidence reused a memo: %s / %s / %v", a, c, err)
	}
}

func TestSequencePreservesOrderAndFiltersOnlyExactKnownRefs(t *testing.T) {
	column := Column{Name: "order", Kind: Sequence, OptionsFrom: "options", LimitFrom: "limit"}
	row := Row{Fields: []Field{{Name: "options", Value: []string{"c1", "c2", "c3"}}, {Name: "limit", Value: 2}}}
	// Unknown refs were never selectable and drop out; a selection of only
	// unknown refs is an empty selection, not a refused row. Commas separate
	// refs as whitespace does. The limit is guidance: a selection past it
	// keeps every distinct advertised ref in the written order, never the
	// first N.
	for input, expected := range map[string]string{"c3 c1": "c3 c1", "c3 c999 c3 c1": "c3 c1", "none": "", "": "", "c999": "", "c01": "", "c1,c2": "c1 c2", "c2 c3 c1": "c2 c3 c1"} {
		value, err := normalizeCell(column, nil, row, input)
		if err != nil || value != expected {
			t.Fatalf("%q -> %q, %v", input, value, err)
		}
	}
	// The limit the model is told is the owner's to supply: a row without a
	// positive integer limit is a preparation error, not a refused answer.
	def := Definition{Stage: "atlas_learn", Columns: []Column{column}}
	for _, fields := range [][]Field{{row.Fields[0]}, {row.Fields[0], {Name: "limit", Value: "2"}}, {row.Fields[0], {Name: "limit", Value: 0}}} {
		if _, err := Request(def, Window{Rows: []Row{{ID: "menu", Fields: fields}}}); err == nil || !strings.Contains(err.Error(), `"limit"`) {
			t.Fatalf("a menu without a usable limit was prepared: %v / %v", fields, err)
		}
	}
	if _, err := Request(def, Window{Rows: []Row{{ID: "menu", Fields: row.Fields}}}); err != nil {
		t.Fatal(err)
	}
	// An options list without a limit is its own bound: a selection can never
	// hold more refs than it offers, and a repeated ref counts once.
	unbounded := Column{Name: "outbound", Kind: Sequence, OptionsFrom: "options"}
	if value, err := normalizeCell(unbounded, nil, row, "c1 c2 c3 c1"); err != nil || value != "c1 c2 c3" {
		t.Fatalf("unbounded selection changed: %q / %v", value, err)
	}
}

// A name chosen among an entry's words may come back as the words
// themselves: litestream's window of 87 flag and route names answered
// "socket" and "POST /start" for w1 and w2, and every one had been
// discarded. A member written as exactly one word's value is that word; the
// whole cell first, so a value holding a space stays one word. A value two
// words share names neither, and a word nothing offers stays unknown.
func TestSequenceTakesAWordWrittenAsItsValue(t *testing.T) {
	type word struct {
		Ref   string `json:"ref"`
		Value string `json:"value"`
	}
	column := Column{Name: "name", Kind: Sequence, OptionsFrom: "word_options", ValuesFrom: "words"}
	row := Row{Fields: []Field{
		{Name: "words", Value: []word{{"w1", "HandleFunc"}, {"w2", "POST /start"}, {"w3", "/start"}, {"w4", "socket"}, {"w5", "dup"}, {"w6", "dup"}}},
		{Name: "word_options", Value: []string{"w1", "w2", "w3", "w4", "w5", "w6"}},
	}}
	for input, expected := range map[string]string{
		"socket": "w4", "POST /start": "w2", "/start": "w3", "w2": "w2", "socket w3": "w4 w3",
		"dup": "", "unknown": "", "w4 socket": "w4", "none": "",
	} {
		if value, err := normalizeCell(column, nil, row, input); err != nil || value != expected {
			t.Fatalf("%q -> %q, %v; want %q", input, value, err, expected)
		}
	}
	// Without ValuesFrom a value is no ref.
	column.ValuesFrom = ""
	if value, err := normalizeCell(column, nil, row, "socket"); err != nil || value != "" {
		t.Fatalf("a value was taken without ValuesFrom: %q, %v", value, err)
	}
}

func testRows() []Row {
	return []Row{
		{ID: "f1", Fields: []Field{{Name: "path", Value: "a.go"}, {Name: "box_options", Value: []string{"here", "pkg/b"}}}},
		{ID: "f2", Fields: []Field{{Name: "path", Value: "b.go"}, {Name: "box_options", Value: []string{"here"}}}},
		{ID: "f3", Fields: []Field{{Name: "path", Value: "c.go"}, {Name: "box_options", Value: []string{"here"}}}},
	}
}

// A real row's key in the example reads, to a model without reasoning, as
// "answer this row": litestream's windows of near-identical rows came back
// holding only the row the example showed. The example's key is a
// placeholder no row may carry, and a row answered under it answers nothing.
func TestResponseExampleShowsAPlaceholderKeyNoRowCarries(t *testing.T) {
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
		t.Fatalf("the example follows its window's rows: %v", err)
	}
	var example struct {
		Rows []map[string]string `json:"rows"`
	}
	if err := json.Unmarshal([]byte(first.Prompt.ResponseExample), &example); err != nil || len(example.Rows) != 1 {
		t.Fatalf("response example lost its table object: %s / %v", first.Prompt.ResponseExample, err)
	}
	row := example.Rows[0]
	if len(row) != len(def.Columns)+1 || row["key"] != ExampleKey || row["line"] == "" || row["box"] == "" {
		t.Fatalf("response example does not show the placeholder key and the owner columns: %+v", row)
	}
	for _, window := range windows {
		for _, asked := range window.Rows {
			if strings.Contains(first.Prompt.ResponseExample, `"`+asked.ID+`"`) {
				t.Fatalf("the example shows row %s's key: %s", asked.ID, first.Prompt.ResponseExample)
			}
		}
	}
	def.Columns = append(def.Columns, Column{Name: "activation", Kind: Text})
	changed, err := Call(def, windows[0])
	if err != nil || changed.Prompt.ResponseExample == first.Prompt.ResponseExample || !strings.Contains(changed.Prompt.ResponseExample, `"activation"`) {
		t.Fatalf("response example ignored a contract column change: %s / %v", changed.Prompt.ResponseExample, err)
	}

	if _, err := Windows(def, 1, []Row{{ID: ExampleKey}}); err == nil {
		t.Fatal("a row carrying the example's placeholder key was prepared")
	}
	result, err := DecodeResult(testDefinition(), windows[0], []byte(`{"rows":[{"key":"<each row's key>","line":"reads a","box":"here"},{"key":"f2","line":"reads b","box":"here"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Answers[0] != nil || result.Answers[1]["line"] != "reads b" {
		t.Fatalf("answers = %+v", result.Answers)
	}
	reasons := map[string]string{}
	for _, rejection := range result.Rejections {
		reasons[rejection.Key] = rejection.Reason
	}
	if reasons[ExampleKey] != "response row copied the example's placeholder key" || reasons["f1"] != "row was not answered" {
		t.Fatalf("rejections = %+v", result.Rejections)
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
	window := Window{Rows: []Row{{ID: "s7"}}}
	example := ResponseExample(def)
	want := `{"rows":[{"key":"<each row's key>","zone":"<computed zone>","entry":"<computed entry>","activation":"<computed activation>","na\"me":"<computed na\"me>"}]}`
	if example != want {
		t.Fatalf("example = %s\nwant      %s", example, want)
	}
	var decoded struct {
		Rows []map[string]string `json:"rows"`
	}
	if err := json.Unmarshal([]byte(example), &decoded); err != nil || len(decoded.Rows) != 1 || len(decoded.Rows[0]) != 5 || strings.Contains(example, "\n") {
		t.Fatalf("example is not one valid JSON line: %v", err)
	}
	call, err := Call(def, window)
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
	if !strings.Contains(request, `{"key": "f1", "path": "a.go", "box_options": ["here","pkg/b"]}`) {
		t.Fatalf("request does not carry the row in field order:\n%s", request)
	}
	if strings.Index(request, `"path": "a.go"`) > strings.Index(request, `"path": "b.go"`) {
		t.Fatal("rows are out of order")
	}
	if !strings.Contains(request, `"key": "f1"`) {
		t.Fatal("the request lost the artifact row ID")
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
    {"key": "f1", "path": "a.go", "box_options": ["here","pkg/b"]},
    {"key": "f2", "path": "b.go", "box_options": ["here"]}
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
	raw := []byte("```json\n{\"rows\":[{\"key\":\"f2\",\"line\":\"  reads   b \",\"box\":\"HERE\"},{\"key\":\"f1\",\"line\":\"writes a\",\"box\":\"new:  Config loading and parsing \"}]}\n```")
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

func TestChoiceAcceptsAUniquePrefix(t *testing.T) {
	def := testDefinition()
	def.Columns[1] = Column{Name: "box", Kind: Choice, Options: []string{"Utilities and configuration", "Utilities and logging", "Storage"}}
	windows, _ := Windows(def, 1, testRows()[:1])
	if _, err := Decode(def, windows[0], []byte(`{"rows":[{"key":"f1","line":"a","box":"Utilities and"}]}`)); err == nil {
		t.Fatal("an ambiguous prefix was accepted")
	}
	answers, err := Decode(def, windows[0], []byte(`{"rows":[{"key":"f1","line":"a","box":"Stor"}]}`))
	if err != nil || answers[0]["box"] != "Storage" {
		t.Fatalf("unique prefix: %v %v", answers, err)
	}
}

func TestTextIsCutAtAWord(t *testing.T) {
	def := testDefinition()
	windows, _ := Windows(def, 1, testRows()[:1])
	raw := `{"rows":[{"key":"f1","line":"this line is much longer than twenty runes","box":"here"}]}`
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
	raw, _ := json.Marshal(map[string]any{"rows": []map[string]string{{"key": "f1", "line": " \n" + text + "\n ", "box": "here"}}})
	answers, err := Decode(def, windows[0], raw)
	if err != nil || answers[0]["line"] != text {
		t.Fatalf("prose was changed: %v, %v", answers, err)
	}
	if _, err := Decode(def, windows[0], []byte(`{"rows":[{"key":"f1","line":" \n ","box":"here"}]}`)); err == nil {
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
	if len(windows) != 3 || strings.Join(ids, ",") != "f1,f2,f3" {
		t.Fatalf("lost or reordered rows: %v", ids)
	}
	def.MaxInputBytes--
	if _, err := WindowsWithContext(def, 1, shared, rows); err == nil || !strings.Contains(err.Error(), "row f1") {
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
					if row["key"] != rows[offset+j].ID || row["doc"] != rows[offset+j].Fields[0].Value {
						t.Fatal("request changed escaped UTF-8 evidence or its row ID")
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
		rows = append(rows, Row{ID: fmt.Sprintf("k%d", i+1), Fields: []Field{{Name: "doc", Value: doc}}})
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
			{name: "byte only", budget: budget, sizes: []int{10, 9, 6}},
			{name: "one byte short of ten rows", budget: budget - 1, sizes: []int{9, 9, 7}},
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
						if row["key"] != window.Rows[j].ID || row["doc"] != doc {
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
	if got, err := normalizeCell(column, nil, only, "{param}/health"); err != nil || got != "unknown" {
		t.Fatalf("the only possible address was refused: %q / %v", got, err)
	}
	offered := Row{Fields: []Field{{Name: "address_options", Value: []string{"a1", "unknown"}}}}
	if _, err := normalizeCell(column, nil, offered, "{param}/health"); err == nil || !strings.Contains(err.Error(), "not one of the options") {
		t.Fatalf("a copied path replaced an offered address ref: %v", err)
	}
	if got, err := normalizeCell(column, nil, offered, "a1"); err != nil || got != "a1" {
		t.Fatalf("an offered ref was refused: %q / %v", got, err)
	}
}

func TestSequenceEmptyOrNullIsAnEmptySelection(t *testing.T) {
	column := Column{Name: "outbound", Kind: Sequence, OptionsFrom: "call_options"}
	row := Row{ID: "s1", Fields: []Field{{Name: "call_options", Value: []string{"c1", "c2"}}}}
	for _, cell := range []string{"none", "", "  "} {
		if got, err := normalizeCell(column, nil, row, cell); err != nil || got != "" {
			t.Fatalf("empty selection %q refused: %q / %v", cell, got, err)
		}
	}
	if got, err := normalizeCell(column, nil, row, "c9 c12"); err != nil || got != "" {
		t.Fatalf("refs outside the options did not settle as an empty selection: %q / %v", got, err)
	}
	if got, err := normalizeCell(column, nil, row, "c9 c2"); err != nil || got != "c2" {
		t.Fatalf("a known ref beside an unknown one was lost: %q / %v", got, err)
	}
	def := Definition{Stage: "atlas_symbols", Columns: []Column{column}}
	result, err := DecodeResult(def, Window{Rows: []Row{row}}, []byte(`{"rows":[{"key":"s1","outbound":null}]}`))
	if err != nil || result.Answers[0] == nil || result.Answers[0]["outbound"] != "" {
		t.Fatalf("null selection refused the row: %+v / %v", result, err)
	}
}

func TestOptionsFromReadsTheWindowContextWhenTheRowHasNoList(t *testing.T) {
	// A catalogue every row chooses from is sent once, in the context; the
	// row's own field still wins when both carry the name.
	column := Column{Name: "destination", Kind: Choice, OptionsFrom: "destination_options", Free: "other: ", FreeMaxRunes: 10}
	context := []Field{{Name: "destination_options", Value: []string{"d1", "d2"}}}
	bare := Row{ID: "b1", Fields: []Field{{Name: "path", Value: "client.go"}}}
	if got, err := normalizeCell(column, context, bare, "d2"); err != nil || got != "d2" {
		t.Fatalf("context options were not consulted: %q / %v", got, err)
	}
	for _, input := range []string{"other: Twilio", "other:Twilio", "OTHER :\n Twilio", "other:\tTwilio"} {
		if got, err := normalizeCell(column, context, bare, input); err != nil || got != "other: Twilio" {
			t.Fatalf("free text beside context options was refused: %q -> %q / %v", input, got, err)
		}
	}
	for _, input := range []string{"other:", "other: \n", "another: Twilio", "Twilio"} {
		if got, err := normalizeCell(column, context, bare, input); err == nil {
			t.Fatalf("missing name or unrequested tag became a free choice: %q -> %q", input, got)
		}
	}
	if _, err := normalizeCell(column, context, bare, "d9"); err == nil {
		t.Fatal("a ref outside the context list was accepted")
	}
	own := Row{ID: "b3", Fields: []Field{{Name: "destination_options", Value: []string{"d9"}}}}
	if got, err := normalizeCell(column, context, own, "d9"); err != nil || got != "d9" {
		t.Fatalf("the row's own list did not shadow the context: %q / %v", got, err)
	}
	address := Column{Name: "address", Kind: Choice, OptionsFrom: "address_options", WhenOptionsFrom: "address_options"}
	def := Definition{Stage: "atlas_boundaries", Columns: []Column{column, address}}
	window := Window{Context: context, Rows: []Row{bare, {ID: "b2", Fields: []Field{{Name: "address_options", Value: []string{"unknown", "a1"}}}}}}
	result, err := DecodeResult(def, window, []byte(`{"rows":[{"key":"b1","destination":"d1"},{"key":"b2","destination":"d1","address":"a1"}]}`))
	if err != nil || len(result.Rejections) != 0 {
		t.Fatalf("a row without address candidates was required to choose one: %+v / %v", result, err)
	}
	if _, asked := result.Answers[0]["address"]; asked || result.Answers[1]["address"] != "a1" {
		t.Fatalf("inactive address cell gained authority or the active one lost it: %+v", result.Answers)
	}
}

// Every row answers under "key"; a column of that name could never be
// filled, and each window would be refused.
func TestColumnCannotBeNamedLikeTheRowIdentity(t *testing.T) {
	def := testDefinition()
	def.Columns = append(def.Columns, Column{Name: "key", Kind: Choice, Options: []string{"yes"}, Optional: true})
	if _, err := Request(def, Window{Rows: []Row{{ID: "s1"}}}); err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("Request error = %v, want the reserved column name refused", err)
	}
}
