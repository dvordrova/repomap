package terminology

import (
	"testing"
)

func generationFormsCall(t *testing.T) func(string) (generationResult, error) {
	t.Helper()
	call, err := generationCall([]proseSource{{Texts: []string{"Each OHLCV candle closes the bucket."}, Sources: []Source{{Path: "a.py", Line: 3}}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return func(raw string) (generationResult, error) { return call.DecodeValidate([]byte(raw)) }
}

// "No terms" is a legitimate optional glossary answer however it is written;
// a terms member of another type is still refused.
func TestGlossaryNoTermsIsAnEmptyAnswer(t *testing.T) {
	decode := generationFormsCall(t)
	for _, raw := range []string{`{"terms":null}`, `{}`, `{"terms":[]}`, `[]`} {
		got, err := decode(raw)
		if err != nil || len(got.Terms) != 0 || len(got.Rejections) != 0 {
			t.Fatalf("%s: an empty glossary answer was refused: %+v %v", raw, got, err)
		}
	}
	got, err := decode(`[{"name":"OHLCV","kind":"acronym","explanation":"Open, high, low, close and volume.","rows":["p1"]}]`)
	if err != nil || len(got.Terms) != 1 || got.Terms[0].candidate.Name != "OHLCV" {
		t.Fatalf("a bare terms array was not read as the terms list: %+v %v", got, err)
	}
	for _, raw := range []string{`{"terms":"none"}`, `{"terms":{"name":"OHLCV"}}`, `null`, `"terms"`} {
		if _, err := decode(raw); err == nil {
			t.Fatalf("%s: a terms member of the wrong type was accepted", raw)
		}
	}
}

// Extra members and a missing or unknown kind are harmless; name, explanation
// and rows stay required, and a self-declared identifier is still dropped.
func TestGlossaryTermNeedsNameExplanationAndRowsOnly(t *testing.T) {
	decode := generationFormsCall(t)
	got, err := decode(`{"terms":[
		{"name":" OHLCV ","explanation":"Open, high, low, close and volume.","rows":["p1"],"confidence":0.9},
		{"name":"candle","kind":"concept","explanation":"One time bucket of prices.","rows":["p1"]},
		{"name":"bucket","kind":7,"explanation":"A fixed time interval.","rows":["p1"]}]}`)
	if err != nil || len(got.Terms) != 3 || len(got.Rejections) != 0 || got.Terms[0].candidate.Name != "OHLCV" {
		t.Fatalf("a term with extra members, no kind or another kind was refused: %+v %v", got, err)
	}
	got, err = decode(`{"terms":[
		{"name":"OHLCV","kind":"acronym","explanation":"Open, high, low, close and volume.","rows":["p1"]},
		{"name":"candle","kind":"domain","explanation":"One time bucket of prices."},
		{"name":"  ","kind":"domain","explanation":"Nothing is named.","rows":["p1"]},
		{"name":"bucket","kind":"domain","explanation":"none","rows":["p1"]},
		{"name":"bucket","kind":" Identifier ","explanation":"A self-declared code name.","rows":["p1"]}]}`)
	if err != nil || len(got.Terms) != 1 || got.Terms[0].candidate.Name != "OHLCV" {
		t.Fatalf("a term without rows, name or explanation, or a self-declared identifier was kept: %+v %v", got, err)
	}
	reasons := make(map[string]int)
	for _, rejection := range got.Rejections {
		reasons[rejection.Reason] += rejection.Count
	}
	if reasons["invalid optional term shape"] != 1 || reasons["invalid optional term fields"] != 2 || reasons["term declares itself an identifier, not a concept"] != 1 {
		t.Fatalf("refused terms were not journaled: %+v", got.Rejections)
	}
}
