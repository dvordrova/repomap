package terminology

import (
	"reflect"
	"testing"
)

func namesFormsCall(t *testing.T, text string) func(string) (namesResult, error) {
	t.Helper()
	call, err := namesCall([]proseSource{{Texts: []string{text}, Sources: []Source{{Path: "a.py", Line: 3}}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return func(raw string) (namesResult, error) { return call.DecodeValidate([]byte(raw)) }
}

// "No names" is a legitimate optional glossary answer however it is written;
// a bare list, one string or a name object are the names they carry, and a
// names member of another type is still refused.
func TestGlossaryNamesAnswerForms(t *testing.T) {
	decode := namesFormsCall(t, "Each OHLCV candle closes the bucket.")
	for _, raw := range []string{`{"names":null}`, `{}`, `{"names":[]}`, `[]`} {
		got, err := decode(raw)
		if err != nil || len(got.Names) != 0 || len(got.Rejections) != 0 {
			t.Fatalf("%s: an empty answer was refused: %+v %v", raw, got, err)
		}
	}
	for raw, want := range map[string][]string{
		`["OHLCV","candle"]`:                      {"OHLCV", "candle"},
		`{"names":"OHLCV"}`:                       {"OHLCV"},
		`{"names":[{"name":"OHLCV","kind":"x"}]}`: {"OHLCV"},
		`{"names":[" OHLCV "],"note":"extra"}`:    {"OHLCV"},
	} {
		got, err := decode(raw)
		if err != nil || !reflect.DeepEqual(got.Names, want) {
			t.Fatalf("%s: %+v %v", raw, got, err)
		}
	}
	for _, raw := range []string{`{"names":{"name":"OHLCV"}}`, `{"names":7}`, `null`, `"names"`} {
		if _, err := decode(raw); err == nil {
			t.Fatalf("%s: a names member of the wrong type was accepted", raw)
		}
	}
}

// Owner decision 2026-09-26: prose that says "snapshots" or "classes" writes
// the name Snapshot or class. Go is still not written in "good" or "goes",
// nor Snap in "snapshots".
func TestGlossaryNameIsWrittenInAnyCaseAndPluralForm(t *testing.T) {
	decode := namesFormsCall(t, "Replicas exchange snapshots of classes. A good cache goes stale.")
	got, err := decode(`{"names":["Snapshot","class","Replica","Go","Snap"]}`)
	if err != nil || !reflect.DeepEqual(got.Names, []string{"Snapshot", "class", "Replica"}) {
		t.Fatalf("case and plural forms: %+v %v", got, err)
	}
	if len(got.Rejections) != 1 || got.Rejections[0].Reason != "the name is not written in the prose" || got.Rejections[0].Count != 2 {
		t.Fatalf("a partial word wrote a name: %+v", got.Rejections)
	}
}

// Each closed ref takes one explanation. A missing, empty or conflicting one
// leaves only that name unexplained; an identical repeat, a padded or
// upper-case ref and extra members are harmless; an unknown ref is dropped.
func TestExplanationsAreReadPerClosedRef(t *testing.T) {
	byRef := map[string]string{"t1": "Alpha", "t2": "Beta", "t3": "Gamma", "t4": "Delta"}
	got, err := decodeExplanations([]byte(`{"terms":[
		{"ref":"t1","explanation":"The first concept.","extra":1},
		{"ref":" T1 ","explanation":"The first concept."},
		{"ref":"t2","explanation":"One meaning."},
		{"ref":"t2","explanation":"Another meaning."},
		{"ref":"t4","explanation":"none"},
		{"ref":"t9","explanation":"Unknown."}]}`), byRef)
	if err != nil || !reflect.DeepEqual(got.Explanations, map[string]string{"Alpha": "The first concept."}) {
		t.Fatalf("explanations: %+v %v", got, err)
	}
	reasons := make(map[string]int)
	for _, rejection := range got.Rejections {
		reasons[rejection.Reason] += rejection.Count
	}
	if !reflect.DeepEqual(reasons, map[string]int{"unknown term ref": 1, "a term answered twice with different explanations": 1, "a term was not explained": 2}) {
		t.Fatalf("journal: %+v", got.Rejections)
	}
	if got, err := decodeExplanations([]byte(`[{"ref":"t3","explanation":"The third concept."}]`), byRef); err != nil || got.Explanations["Gamma"] != "The third concept." {
		t.Fatalf("a bare terms list was not read: %+v %v", got, err)
	}
	for _, raw := range []string{`{}`, `{"terms":null}`, `{"terms":[{"ref":"t9","explanation":"Unknown."}]}`} {
		if _, err := decodeExplanations([]byte(raw), byRef); err == nil {
			t.Fatalf("%s: a window that explained no name was accepted", raw)
		}
	}
	if _, err := decodeExplanations([]byte(`{"terms":"none"}`), byRef); err == nil {
		t.Fatal("a terms member of the wrong type was accepted")
	}
}
