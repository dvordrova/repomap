package lines

import "testing"

// A name is asked its English alias when it holds a letter outside the Latin
// script (owner decision 2026-09-26). One such letter in an otherwise Latin
// name is enough; digits, underscores and punctuation are not letters, and a
// Latin-script name in another language or a transliteration asks none.
func TestNeedsAliasOnlyForALetterOutsideTheLatinScript(t *testing.T) {
	for name, want := range map[string]bool{
		"parse한국":          true,
		"시작":               true,
		"주가정보":             true,
		"Привет":           true,
		"价格_2":             true,
		"ΔPrice":           true,
		"snake_case_ascii": false,
		"parseHTTP2":       false,
		"__init__":         false,
		"$_1":              false,
		"Größe":            false,
		"café":             false,
		"juga_jeongbo":     false,
		"":                 false,
	} {
		if got := NeedsAlias(name); got != want {
			t.Errorf("NeedsAlias(%q) = %v, want %v", name, got, want)
		}
	}
}
