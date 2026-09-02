package secretscan

import "unicode/utf8"

const (
	// The UTF-8 encodings of the only two runes that fold to an ASCII letter.
	longSFirstByte      = 0xC5
	longSSecondByte     = 0xBF
	kelvinFirstByte     = 0xE2
	kelvinSecondByte    = 0x84
	kelvinThirdByte     = 0xAA
	continuationMask    = 0xC0
	continuationPattern = 0x80
)

// A regexp in persistencePatterns can only match text that contains one of a
// few fixed words. Checking for those words first is far cheaper than running
// the pattern, and the always-on persistence guard runs over every provider
// request and response, so it dominates a cached run.
//
// The gate must never hide a match. Each entry lists every literal an
// alternative of its pattern requires, folded the way the pattern folds, so a
// text the pattern could match always passes the gate.
var persistenceLiterals = [][]string{
	{"-----begin"},
	{"bearer"},
	{"authorization"},
	{"sk-"},
	{"ghp_", "github_pat_"},
	{"akia"},
	{"apikey", "api_key", "api-key"},
	{"apikey", "api_key", "api-key"},
}

func anyLiteralPresent[Text string | []byte](text Text, literals []string) bool {
	for _, literal := range literals {
		if ContainsFold(text, literal) {
			return true
		}
	}
	return false
}

// ContainsFold reports whether the already-lowercase needle occurs in text
// under the same simple folding the patterns use. Beyond ASCII it accepts the
// only two runes that fold to an ASCII letter, U+212A KELVIN SIGN and U+017F
// LATIN SMALL LETTER LONG S, so a case-insensitive pattern can never match
// text this gate rejected.
//
// It is exported because the provider boundary gates its own structured scan
// the same way, and it takes bytes or a string so neither caller has to copy
// a payload to ask the question.
func ContainsFold[Text string | []byte](text Text, needle string) bool {
	if needle == "" {
		return true
	}
	for index := 0; index < len(text); {
		folded, size := foldAt(text, index)
		if folded == needle[0] && matchesFoldAt(text, index, needle) {
			return true
		}
		index += size
	}
	return false
}

func matchesFoldAt[Text string | []byte](text Text, index int, needle string) bool {
	for position := 0; position < len(needle); position++ {
		if index >= len(text) {
			return false
		}
		folded, size := foldAt(text, index)
		if folded != needle[position] {
			return false
		}
		index += size
	}
	return true
}

// foldAt returns the ASCII byte the character at index folds to, or 0 when it
// folds to nothing in ASCII, together with that character's byte width.
func foldAt[Text string | []byte](text Text, index int) (byte, int) {
	value := text[index]
	if value < utf8.RuneSelf {
		if value >= 'A' && value <= 'Z' {
			return value + ('a' - 'A'), 1
		}
		return value, 1
	}
	// U+017F LATIN SMALL LETTER LONG S folds to "s", U+212A KELVIN SIGN to
	// "k". No other non-ASCII rune folds to an ASCII letter.
	if value == longSFirstByte && index+1 < len(text) && text[index+1] == longSSecondByte {
		return 's', 2
	}
	if value == kelvinFirstByte && index+2 < len(text) &&
		text[index+1] == kelvinSecondByte && text[index+2] == kelvinThirdByte {
		return 'k', 3
	}
	return 0, continuationWidth(text, index)
}

func continuationWidth[Text string | []byte](text Text, index int) int {
	width := 1
	for index+width < len(text) && text[index+width]&continuationMask == continuationPattern {
		width++
	}
	return width
}
