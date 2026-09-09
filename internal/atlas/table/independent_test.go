package table

import (
	"reflect"
	"strings"
	"testing"
)

func TestIndependentRowsKeepValidNeighbours(t *testing.T) {
	def := Definition{Stage: "atlas_operations", Independent: true, Columns: []Column{
		{Name: "entry", Kind: Choice, OptionsFrom: "entry_options"},
	}}
	window := Window{Rows: []Row{
		{ID: "first", Fields: []Field{{Name: "entry_options", Value: []string{"self", "none"}}}},
		{ID: "second", Fields: []Field{{Name: "entry_options", Value: []string{"self", "none"}}}},
		{ID: "third", Fields: []Field{{Name: "entry_options", Value: []string{"self", "none"}}}},
	}}
	for name, middle := range map[string]string{
		"unknown scalar": `{"key":"r2","entry":"u1"}`,
		"missing cell":   `{"key":"r2"}`,
		"wrong type":     `{"key":"r2","entry":{"value":"self"}}`,
		"duplicate key":  `{"key":"r2","entry":"self"},{"key":"r2","entry":"self"}`,
		"missing row":    `{"key":"unknown","entry":"self"}`,
	} {
		t.Run(name, func(t *testing.T) {
			raw := []byte(`{"notes":{"ignored":true},"rows":[{"key":"r1","entry":"self","extra":[1,2]},` + middle + `,{"key":"r3","entry":"none"}]}`)
			result, err := DecodeResult(def, window, raw)
			if err != nil || len(result.Answers) != 3 || result.Answers[0]["entry"] != "self" || result.Answers[1] != nil || result.Answers[2]["entry"] != "none" {
				t.Fatalf("one refused row lost valid neighbours: %+v / %v", result, err)
			}
			if !reflect.DeepEqual(result.AcceptedRowKeys(), []string{"r1", "r3"}) {
				t.Fatalf("optional metadata retained refused rows: %v", result.AcceptedRowKeys())
			}
			found := false
			for _, rejection := range result.Rejections {
				found = found || rejection.Key == "r2" && rejection.Reason != ""
			}
			if !found {
				t.Fatal("refused row has no specific reason")
			}
			coupled := def
			coupled.Independent = false
			if _, err := DecodeResult(coupled, window, raw); err == nil {
				t.Fatal("coupled table accepted an incomplete response")
			}
		})
	}
	if _, err := DecodeResult(def, window, []byte(`{"rows":[{"key":"r2","entry":"u1"}]}`)); err == nil || !strings.Contains(err.Error(), "no rows accepted") {
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
	def.Independent = true
	window := Window{Rows: testRows()[:1]}
	result, err := DecodeResult(def, window, []byte(`{"additional":{"anything":true},"rows":[{"key":"r1","line":"Reads input.","box":"here","additional":[1,2]}]}`))
	if err != nil || len(result.Rejections) != 0 || result.Answers[0]["line"] != "Reads input." || len(result.Answers[0]) != 2 {
		t.Fatalf("extra fields changed an independent answer: %+v / %v", result, err)
	}
	def.Independent = false
	result, err = DecodeResult(def, window, []byte(`{"rows":[{"key":"r1","line":"Reads input.","box":"here"}]}`))
	if err != nil || result.AcceptedRowKeys() != nil {
		t.Fatal("coupled result narrowed its original whole-response metadata")
	}
}
