package table

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// A bare array of rows and a root whose only field wraps the rows are the
// contract's rows in another wrapper; rows still match by key. A response
// with no rows array at all decided nothing.
func TestResponseRowsAcceptWrappersAndRefuseNoRows(t *testing.T) {
	def := Definition{Stage: "atlas_test", Columns: []Column{{Name: "entry", Kind: Choice, Options: []string{"self", "none"}}}}
	window := Window{Rows: []Row{{ID: "first"}, {ID: "second"}}}
	for _, raw := range []string{
		`[{"key":"second","entry":"none"},{"key":"first","entry":"self"}]`,
		`{"result":{"rows":[{"key":"second","entry":"none"},{"key":"first","entry":"self"}]}}`,
		`{"notes":"extra","rows":[{"key":"second","entry":"none"},{"key":"first","entry":"self"}]}`,
	} {
		result, err := DecodeResult(def, window, []byte(raw))
		if err != nil || result.Answers[0]["entry"] != "self" || result.Answers[1]["entry"] != "none" || len(result.Rejections) != 0 {
			t.Fatalf("wrapped rows were refused or read by position: %s -> %+v / %v", raw, result, err)
		}
	}
	for _, raw := range []string{`{"notes":1}`, `{"rows":null}`, `{"rows":{}}`, `null`, `not json`, `{"result":{"rows":null}}`, `{"result":{"rows":[]},"notes":1}`} {
		if _, err := DecodeResult(def, window, []byte(raw)); err == nil {
			t.Fatalf("a response without rows was accepted: %s", raw)
		}
	}
	// A key written with surrounding whitespace is the same exact key; any
	// other spelling is still not asked.
	result, err := DecodeResult(def, window, []byte(`{"rows":[{"key":" first ","entry":"self"},{"key":"Second","entry":"none"}]}`))
	if err != nil || result.Answers[0]["entry"] != "self" || result.Answers[1] != nil {
		t.Fatalf("key matching changed: %+v / %v", result, err)
	}
}

// A list of refs is a selection, and true or false on a yes/no choice is
// that choice. Anything else on a required cell still refuses the row; on an
// optional cell it discards only that cell.
func TestCellFormsReadListsAndBooleans(t *testing.T) {
	def := Definition{Stage: "atlas_test", Columns: []Column{
		{Name: "sources", Kind: Sequence, OptionsFrom: "options"},
		{Name: "same", Kind: Choice, Options: []string{"yes", "no"}},
		{Name: "validates", Kind: Choice, Options: []string{"yes"}, Optional: true},
		{Name: "talks", Kind: Choice, Options: []string{"db", "sdk"}, Optional: true},
	}}
	options := []Field{{Name: "options", Value: []string{"s1", "s2", "s3"}}}
	window := Window{Rows: []Row{{ID: "a", Fields: options}, {ID: "b", Fields: options}, {ID: "c", Fields: options}}}
	result, err := DecodeResult(def, window, []byte(`{"rows":[
		{"key":"a","sources":["s2","s9","s1"],"same":true,"validates":false,"talks":"grpc"},
		{"key":"b","sources":"s3","same":false,"validates":true,"talks":{"kind":"db"}},
		{"key":"c","sources":"s1","same":{"value":"yes"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if want := (Answer{"sources": "s2 s1", "same": "yes"}); !reflect.DeepEqual(result.Answers[0], want) {
		t.Fatalf("list or boolean forms were not read: %+v", result.Answers[0])
	}
	if want := (Answer{"sources": "s3", "same": "no", "validates": "yes"}); !reflect.DeepEqual(result.Answers[1], want) {
		t.Fatalf("false or a mistyped optional cell changed the row: %+v", result.Answers[1])
	}
	if result.Answers[2] != nil {
		t.Fatalf("an object on a required choice was accepted: %+v", result.Answers[2])
	}
	var cells []string
	for _, rejection := range result.Rejections {
		if rejection.Cell != "" {
			cells = append(cells, rejection.Key+"."+rejection.Cell)
		}
	}
	if !reflect.DeepEqual(cells, []string{"a.talks", "b.talks"}) || !reflect.DeepEqual(result.AcceptedRowKeys(), []string{}) {
		t.Fatalf("discarded optional cells were not recorded or their rows authorized prose: %+v / %v", result.Rejections, result.AcceptedRowKeys())
	}
}

// Surrounding quotes, backticks and one final punctuation mark are form; a
// ref followed by a label is not the ref.
func TestChoiceStripsQuotesAndFinalPunctuation(t *testing.T) {
	column := Column{Name: "peer", Kind: Choice, OptionsFrom: "peer_options"}
	row := Row{Fields: []Field{{Name: "peer_options", Value: []string{"none", "p3", "d2", "yes"}}}}
	for input, want := range map[string]string{"yes.": "yes", `"p3"`: "p3", "`d2`": "d2", "'p3';": "p3", `"p3".`: "p3", "P3!": "p3"} {
		if got, err := normalizeCell(column, nil, row, input); err != nil || got != want {
			t.Fatalf("%q -> %q / %v, want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"p3: Storage", "p3 Storage", "p3..", `"p3`, "p"} {
		if got, err := normalizeCell(column, nil, row, input); err == nil {
			t.Fatalf("%q was read as %q", input, got)
		}
	}
}

// A column's own spelling of absence reads the same in any case, with a
// final period, empty, null or missing. Without one, empty text is refused.
func TestAbsenceSpellingsReadAsTheColumnsEmptyValue(t *testing.T) {
	def := Definition{Stage: "atlas_test", Columns: []Column{
		{Name: "line", Kind: Text},
		{Name: "remaining", Kind: Prose, EmptyValue: "none"},
		{Name: "label", Kind: Text, Missing: "-"},
	}}
	window := Window{Rows: []Row{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}, {ID: "e"}}}
	result, err := DecodeResult(def, window, []byte(`{"rows":[
		{"key":"a","line":"Reads.","remaining":"None.","label":""},
		{"key":"b","line":"Reads.","remaining":"","label":null},
		{"key":"c","line":"Reads.","remaining":null},
		{"key":"d","line":"Reads.","remaining":"NONE","label":"Loads config"},
		{"key":"e","line":"  ","remaining":"A gap remains.","label":"x"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if result.Answers[i]["remaining"] != "none" {
			t.Fatalf("row %d absence spelling was not the empty value: %+v", i, result.Answers[i])
		}
	}
	if result.Answers[0]["label"] != "-" || result.Answers[1]["label"] != "-" || result.Answers[2]["label"] != "-" || result.Answers[3]["label"] != "Loads config" {
		t.Fatalf("an empty label did not read as its missing value: %+v", result.Answers)
	}
	if result.Answers[4] != nil || len(result.Rejections) != 1 || !strings.Contains(result.Rejections[0].Reason, `"line" is empty`) {
		t.Fatalf("empty text without an empty value was accepted: %+v / %+v", result.Answers[4], result.Rejections)
	}
}

// An Alone cell fails by itself: the row keeps its other decisions, the
// refusal is recorded and the row's text is not accepted as glossary prose.
// A cell without Alone, or a row where no cell survives, still refuses it.
func TestAloneCellsFailByThemselves(t *testing.T) {
	def := Definition{Stage: "atlas_directories", Columns: []Column{
		{Name: "title", Kind: Text, MaxRunes: 40, Alone: true},
		{Name: "line", Kind: Text, MaxRunes: 160, Alone: true},
		{Name: "open", Kind: Choice, Options: []string{"yes", "no"}, Alone: true},
	}}
	window := Window{Rows: []Row{{ID: "d1"}, {ID: "d2"}, {ID: "d3"}, {ID: "d4"}, {ID: "d5"}}}
	result, err := DecodeResult(def, window, []byte(`{"rows":[
		{"key":"d1","title":"Command entry","line":"Starts the analysis."},
		{"key":"d2","title":"Report","line":"Renders the report.","open":"maybe"},
		{"key":"d3","title":"","line":"Keeps fixtures.","open":"no"},
		{"key":"d4","title":"","line":" ","open":"perhaps"},
		{"key":"d5","title":"Complete","line":"Answers every cell.","open":"yes"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if want := (Answer{"title": "Command entry", "line": "Starts the analysis."}); !reflect.DeepEqual(result.Answers[0], want) {
		t.Fatalf("a missing open refused its row or was invented: %+v", result.Answers[0])
	}
	if want := (Answer{"title": "Report", "line": "Renders the report."}); !reflect.DeepEqual(result.Answers[1], want) {
		t.Fatalf("an unlisted open refused its row: %+v", result.Answers[1])
	}
	if want := (Answer{"line": "Keeps fixtures.", "open": "no"}); !reflect.DeepEqual(result.Answers[2], want) {
		t.Fatalf("an empty title refused its row: %+v", result.Answers[2])
	}
	if result.Answers[3] != nil || result.Answers[4]["open"] != "yes" {
		t.Fatalf("a row with no surviving cell was accepted, or a complete neighbour lost: %+v", result.Answers)
	}
	if !reflect.DeepEqual(result.AcceptedRowKeys(), []string{"d5"}) {
		t.Fatalf("a row with a refused cell authorized its text: %v", result.AcceptedRowKeys())
	}
	kinds := map[string]int{}
	for _, rejection := range result.ResponseRejections() {
		kinds[rejection.Kind]++
	}
	if kinds["cell_rejected"] != 3 || kinds["row_rejected"] != 1 {
		t.Fatalf("cell and row refusals were not recorded apart: %+v", result.Rejections)
	}
	// Without Alone the same missing decision refuses the row.
	strict := def
	strict.Columns = append([]Column{}, def.Columns...)
	strict.Columns[2].Alone = false
	if refused, err := DecodeResult(strict, Window{Rows: window.Rows[:1]}, []byte(`{"rows":[{"key":"d1","title":"Command entry","line":"Starts the analysis."}]}`)); err == nil || refused.Answers[0] != nil {
		t.Fatalf("a missing required decision was accepted: %+v / %v", refused, err)
	}
}

// Two copies of a row that differ only in an Alone caption keep the row and
// lose that caption; a difference in a closed decision leaves no decision.
func TestRepeatedRowsLoseOnlyDifferingAloneCells(t *testing.T) {
	def := Definition{Stage: "atlas_targets", Columns: []Column{
		{Name: "same", Kind: Choice, Options: []string{"yes", "no"}},
		{Name: "line", Kind: Text, Alone: true},
	}}
	window := Window{Rows: []Row{{ID: "a"}, {ID: "b"}}}
	result, err := DecodeResult(def, window, []byte(`{"rows":[
		{"key":"a","same":"yes","line":"Calls the service."},{"key":"a","same":"yes","line":"Talks to the service."},
		{"key":"b","same":"yes","line":"Same."},{"key":"b","same":"no","line":"Same."}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if want := (Answer{"same": "yes"}); !reflect.DeepEqual(result.Answers[0], want) {
		t.Fatalf("copies differing in a caption lost the row or kept one wording: %+v", result.Answers[0])
	}
	if result.Answers[1] != nil || len(result.Rejections) != 2 || result.Rejections[0].Cell != "line" || result.Rejections[1].Key != "b" {
		t.Fatalf("copies differing in a decision were accepted: %+v / %+v", result.Answers[1], result.Rejections)
	}
}

// A response with no accepted row keeps every row's reason for the journal.
func TestNoRowsAcceptedCarriesEveryRowReason(t *testing.T) {
	def := Definition{Stage: "atlas_test", Columns: []Column{{Name: "entry", Kind: Choice, Options: []string{"self", "none"}}}}
	window := Window{Rows: []Row{{ID: "a"}, {ID: "b"}}}
	_, err := DecodeResult(def, window, []byte(`{"rows":[{"key":"a","entry":"u1"},{"key":"b","entry":"u2"}]}`))
	var refused *NoRowsAccepted
	if !errors.As(err, &refused) || len(refused.Rejections) != 2 || !strings.Contains(err.Error(), "no rows accepted") || !strings.Contains(refused.Rejections[1].Reason, "u2") {
		t.Fatalf("a wholly refused response lost its row reasons: %v", err)
	}
}

// A value the column fills in for an absent cell is no answer: a row whose
// every written cell was refused is refused, while an explicit empty list is
// a written empty selection that keeps its row.
func TestFilledInAbsenceIsNoAnswer(t *testing.T) {
	def := Definition{Stage: "atlas_learn", Columns: []Column{
		{Name: "questions", Kind: Sequence, OptionsFrom: "options"},
		{Name: "reason", Kind: Text, Optional: true},
		{Name: "label", Kind: Text, Missing: "-"},
	}}
	options := []Field{{Name: "options", Value: []string{"q1"}}}
	window := Window{Rows: []Row{{ID: "a", Fields: options}, {ID: "b", Fields: options}, {ID: "c", Fields: options}}}
	result, err := DecodeResult(def, window, []byte(`{"rows":[
		{"key":"a","questions":[],"reason":5},
		{"key":"b","questions":null,"reason":5},
		{"key":"c","questions":"q1"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if want := (Answer{"questions": "", "label": "-"}); !reflect.DeepEqual(result.Answers[0], want) {
		t.Fatalf("an explicit empty selection lost its row: %+v", result.Answers[0])
	}
	if result.Answers[1] != nil || result.Answers[2]["questions"] != "q1" {
		t.Fatalf("a row of filled-in absences was accepted, or a neighbour lost: %+v", result.Answers)
	}
}
