package table

import (
	"reflect"
	"strings"
	"testing"
)

func TestIndependentRowsKeepValidNeighbours(t *testing.T) {
	def := Definition{Stage: "atlas_operations", Columns: []Column{
		{Name: "entry", Kind: Choice, OptionsFrom: "entry_options"},
	}}
	window := Window{Rows: []Row{
		{ID: "first", Fields: []Field{{Name: "entry_options", Value: []string{"self", "none"}}}},
		{ID: "second", Fields: []Field{{Name: "entry_options", Value: []string{"self", "none"}}}},
		{ID: "third", Fields: []Field{{Name: "entry_options", Value: []string{"self", "none"}}}},
	}}
	for name, middle := range map[string]string{
		"unknown scalar":     `{"key":"second","entry":"u1"}`,
		"missing cell":       `{"key":"second"}`,
		"wrong type":         `{"key":"second","entry":{"value":"self"}}`,
		"conflicting repeat": `{"key":"second","entry":"self"},{"key":"second","entry":"none"}`,
		"missing row":        `{"key":"unknown","entry":"self"}`,
	} {
		t.Run(name, func(t *testing.T) {
			raw := []byte(`{"notes":{"ignored":true},"rows":[{"key":"first","entry":"self","extra":[1,2]},` + middle + `,{"key":"third","entry":"none"}]}`)
			result, err := DecodeResult(def, window, raw)
			if err != nil || len(result.Answers) != 3 || result.Answers[0]["entry"] != "self" || result.Answers[1] != nil || result.Answers[2]["entry"] != "none" {
				t.Fatalf("one refused row lost valid neighbours: %+v / %v", result, err)
			}
			if !reflect.DeepEqual(result.AcceptedRowKeys(), []string{"first", "third"}) {
				t.Fatalf("optional metadata retained refused rows: %v", result.AcceptedRowKeys())
			}
			found := false
			for _, rejection := range result.Rejections {
				found = found || rejection.Key == "second" && rejection.Reason != ""
			}
			if !found {
				t.Fatal("refused row has no specific reason")
			}
		})
	}
	// A model that repeats the same answer for a row gave one answer.
	repeated, err := DecodeResult(def, window, []byte(`{"rows":[{"key":"first","entry":"self"},{"key":"second","entry":"none"},{"key":"second","entry":"none"},{"key":"third","entry":"none"}]}`))
	if err != nil || len(repeated.Rejections) != 0 || repeated.Answers[1]["entry"] != "none" {
		t.Fatalf("an identical repeat refused its row: %+v / %v", repeated, err)
	}
	if _, err := DecodeResult(def, window, []byte(`{"rows":[{"key":"second","entry":"u1"}]}`)); err == nil || !strings.Contains(err.Error(), "no rows accepted") {
		t.Fatalf("a wholly refused response became a cacheable success: %v", err)
	}
	for _, raw := range []string{`{"rows":null}`, `{"notes":"no rows"}`, `{"rows":{}}`, `not json`} {
		if _, err := DecodeResult(def, window, []byte(raw)); err == nil {
			t.Fatalf("invalid envelope accepted: %s", raw)
		}
	}
}

func TestIndependentRowsIgnoreAdditionalFields(t *testing.T) {
	def := testDefinition()
	window := Window{Rows: testRows()[:1]}
	result, err := DecodeResult(def, window, []byte(`{"additional":{"anything":true},"rows":[{"key":"f1","line":"Reads input.","box":"here","additional":[1,2]}]}`))
	if err != nil || len(result.Rejections) != 0 || result.Answers[0]["line"] != "Reads input." || len(result.Answers[0]) != 2 {
		t.Fatalf("extra fields changed an independent answer: %+v / %v", result, err)
	}
}

func TestIndependentRowsRequireTheirArtifactIDs(t *testing.T) {
	def := Definition{Stage: "atlas_answer", Columns: []Column{
		{Name: "entry", Kind: Choice, OptionsFrom: "entry_options"},
	}}
	one := Window{Rows: []Row{{ID: "only", Fields: []Field{{Name: "entry_options", Value: []string{"self", "none"}}}}}}
	if _, err := DecodeResult(def, one, []byte(`{"rows":[{"entry":"self"}]}`)); err == nil || !strings.Contains(err.Error(), "no string key") {
		t.Fatalf("a keyless answer was associated by position: %v", err)
	}
	two := Window{Rows: []Row{one.Rows[0], {ID: "second", Fields: one.Rows[0].Fields}}}
	result, err := DecodeResult(def, two, []byte(`{"rows":[{"key":"second","entry":"none"},{"entry":"self"}]}`))
	if err != nil || result.Answers[0] != nil || result.Answers[1]["entry"] != "none" || len(result.Rejections) != 2 {
		t.Fatalf("a partly keyed response was read positionally: %+v / %v", result, err)
	}
}

func TestIndependentMissingChoiceTakesItsDeclaredNoDecisionValue(t *testing.T) {
	def := Definition{Stage: "atlas_symbols", Columns: []Column{
		{Name: "key_symbol", Kind: Choice, Options: []string{"yes", "no"}},
		{Name: "activation", Kind: Choice, Options: []string{"none", "unassessed", "request"}, Missing: "unassessed"},
	}}
	window := Window{Rows: []Row{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}}}
	result, err := DecodeResult(def, window, []byte(`{"rows":[
		{"key":"a","key_symbol":"no"},
		{"key":"b","key_symbol":"no","activation":null},
		{"key":"c","key_symbol":"yes","activation":"request"},
		{"key":"d","activation":"request"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Answers[0]["activation"] != "unassessed" || result.Answers[1]["activation"] != "unassessed" || result.Answers[2]["activation"] != "request" {
		t.Fatalf("missing or null activation did not settle as unassessed: %+v", result.Answers[:3])
	}
	if result.Answers[3] != nil || len(result.Rejections) != 1 || !strings.Contains(result.Rejections[0].Reason, `missing "key_symbol" cell`) {
		t.Fatalf("a missing choice without a declared no-decision value was accepted: %+v / %+v", result.Answers[3], result.Rejections)
	}
}
