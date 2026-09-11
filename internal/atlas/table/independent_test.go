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

func TestIndependentKeylessRowsAreReadInAskedOrderOnlyWhenComplete(t *testing.T) {
	def := Definition{Stage: "atlas_answer", Independent: true, Columns: []Column{
		{Name: "entry", Kind: Choice, OptionsFrom: "entry_options"},
	}}
	one := Window{Rows: []Row{{ID: "only", Fields: []Field{{Name: "entry_options", Value: []string{"self", "none"}}}}}}
	result, err := DecodeResult(def, one, []byte(`{"rows":[{"entry":"self"}]}`))
	if err != nil || len(result.Answers) != 1 || result.Answers[0]["entry"] != "self" || len(result.Rejections) != 0 {
		t.Fatalf("the only asked row did not receive the only answer: %+v / %v", result, err)
	}
	if _, err := DecodeResult(def, one, []byte(`{"rows":[{"entry":"self"},{"entry":"none"}]}`)); err == nil || !strings.Contains(err.Error(), "no string key") {
		t.Fatalf("two keyless rows for one asked row were guessed: %v", err)
	}
	two := Window{Rows: []Row{one.Rows[0], {ID: "second", Fields: one.Rows[0].Fields}}}
	result, err = DecodeResult(def, two, []byte(`{"rows":[{"entry":"self"},{"entry":"none"}]}`))
	if err != nil || result.Answers[0]["entry"] != "self" || result.Answers[1]["entry"] != "none" || len(result.Rejections) != 0 {
		t.Fatalf("a complete keyless response was not read in asked order: %+v / %v", result, err)
	}
	if _, err := DecodeResult(def, two, []byte(`{"rows":[{"entry":"self"}]}`)); err == nil || !strings.Contains(err.Error(), "no string key") {
		t.Fatalf("an incomplete keyless response was guessed: %v", err)
	}
	result, err = DecodeResult(def, two, []byte(`{"rows":[{"key":"r2","entry":"none"},{"entry":"self"}]}`))
	if err != nil || result.Answers[0] != nil || result.Answers[1]["entry"] != "none" || len(result.Rejections) != 2 {
		t.Fatalf("a partly keyed response was read positionally: %+v / %v", result, err)
	}
}

func TestIndependentEmptyLabelFallsBackToItsOwnDescription(t *testing.T) {
	def := Definition{Stage: "atlas_operations", Independent: true, Columns: []Column{
		{Name: "entry", Kind: Choice, Options: []string{"self", "none"}},
		{Name: "name", Kind: Text, MaxRunes: 20, When: map[string]string{"entry": "self"}, EmptyFrom: "description"},
		{Name: "description", Kind: Text, MaxRunes: 180, When: map[string]string{"entry": "self"}},
	}}
	window := Window{Rows: []Row{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}}}
	raw := []byte(`{"rows":[
		{"key":"r1","entry":"self","name":null,"description":"Runs periodic event aggregation batches until context cancellation. Emits status events."},
		{"key":"r2","entry":"self","description":"Handles incoming WebSocket test-launcher events, starting or stopping tests."},
		{"key":"r3","entry":"self","name":"Send batches","description":"Runs periodic event sending batches."},
		{"key":"r4","entry":"self","name":"","description":""}]}`)
	result, err := DecodeResult(def, window, raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Answers[0]; got["name"] != "Runs periodic event…" || got["name_from"] != "description" {
		t.Fatalf("null label did not take the first sentence of its description: %+v", got)
	}
	if got := result.Answers[1]; got["name"] != "Handles incoming…" || got["name_from"] != "description" {
		t.Fatalf("missing label did not take its description: %+v", got)
	}
	if got := result.Answers[2]; got["name"] != "Send batches" || got["name_from"] != "" {
		t.Fatalf("a written label was replaced: %+v", got)
	}
	if result.Answers[3] != nil || len(result.Rejections) != 1 || !strings.Contains(result.Rejections[0].Reason, "is empty") {
		t.Fatalf("an empty description invented a label: %+v / %+v", result.Answers[3], result.Rejections)
	}
	loose := Definition{Stage: "atlas_operations", Independent: true, Columns: []Column{
		{Name: "name", Kind: Text, MaxRunes: 20, EmptyFrom: "description"},
		{Name: "description", Kind: Prose},
	}}
	result, err = DecodeResult(loose, Window{Rows: []Row{{ID: "a"}}}, []byte(`{"rows":[{"key":"r1","name":null,"description":"   "}]}`))
	if err == nil || result.Answers[0] != nil || !strings.Contains(err.Error(), "is empty") {
		t.Fatalf("blank prose became a label: %+v / %v", result, err)
	}
}

func TestIndependentMissingChoiceTakesItsDeclaredNoDecisionValue(t *testing.T) {
	def := Definition{Stage: "atlas_symbols", Independent: true, Columns: []Column{
		{Name: "key_symbol", Kind: Choice, Options: []string{"yes", "no"}},
		{Name: "activation", Kind: Choice, Options: []string{"none", "unassessed", "request"}, Missing: "unassessed"},
	}}
	window := Window{Rows: []Row{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}}}
	result, err := DecodeResult(def, window, []byte(`{"rows":[
		{"key":"r1","key_symbol":"no"},
		{"key":"r2","key_symbol":"no","activation":null},
		{"key":"r3","key_symbol":"yes","activation":"request"},
		{"key":"r4","activation":"request"}]}`))
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
