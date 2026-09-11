package table

import "testing"

// Missing settles a cell the model was asked for and left out; Unasked
// settles a cell the row never asked because its WhenOptionsFrom field had
// no choices, so a branch conditioned on it can still follow. A column with
// only Missing leaves an unasked cell without a value, and nothing the
// model writes into an unasked cell gains authority.
func TestUnaskedCellTakesItsOwnValueWhileMissingSettlesOnlyAskedCells(t *testing.T) {
	def := Definition{Stage: "atlas_test", Independent: true, Columns: []Column{
		{Name: "entry", Kind: Choice, Options: []string{"self", "none"}},
		{Name: "name_kind", Kind: Choice, Options: []string{"http", "label"}, When: map[string]string{"entry": "self"}, WhenOptionsFrom: "registered_name_options", Missing: "label", Unasked: "label"},
		{Name: "http_path", Kind: Choice, OptionsFrom: "registered_name_options", When: map[string]string{"name_kind": "http"}},
		{Name: "name", Kind: Text, MaxRunes: 20, When: map[string]string{"name_kind": "label"}},
		{Name: "box", Kind: Choice, OptionsFrom: "box_options", WhenOptionsFrom: "box_options", Missing: "here"},
	}}
	plain := Row{ID: "plain"}
	offered := Row{ID: "offered", Fields: []Field{{Name: "registered_name_options", Value: []string{"p1"}}, {Name: "box_options", Value: []string{"here", "pkg/b"}}}}
	window := Window{Rows: []Row{plain, offered, plain, offered}}
	result, err := DecodeResult(def, window, []byte(`{"rows":[
		{"key":"r1","entry":"self","name":"Send mail","box":"pkg/b"},
		{"key":"r2","entry":"self","http_path":"p1","name":"ignored"},
		{"key":"r3","entry":"self","name_kind":"http","http_path":"p1","name":"Sync"},
		{"key":"r4","entry":"none","name_kind":"label","name":"x","box":"pkg/b"}]}`))
	if err != nil || len(result.Rejections) != 0 {
		t.Fatalf("rows refused: %+v / %v", result, err)
	}
	if got := result.Answers[0]; got["name_kind"] != "label" || got["name"] != "Send mail" || got["box"] != "" {
		t.Fatalf("unasked name_kind did not read label, or an unasked box gained a value: %#v", got)
	}
	if got := result.Answers[1]; got["name_kind"] != "label" || got["name"] != "ignored" || got["http_path"] != "" || got["box"] != "here" {
		t.Fatalf("an asked and omitted name_kind did not read label with its name, or an omitted asked box lost here: %#v", got)
	}
	if got := result.Answers[2]; got["name_kind"] != "label" || got["name"] != "Sync" || got["http_path"] != "" {
		t.Fatalf("a written choice on an unasked cell gained authority: %#v", got)
	}
	if got := result.Answers[3]; len(got) != 2 || got["entry"] != "none" || got["box"] != "pkg/b" {
		t.Fatalf("an inactive branch was settled by Unasked: %#v", got)
	}
}
