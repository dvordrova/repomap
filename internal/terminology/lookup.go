package terminology

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Term lookup (owner decision 2026-09-26) finds a name as a whole word or
// phrase whatever its letter case, alone or with an English plural ending:
// "s", or "es" after a name ending in s, x, z, ch or sh. Snapshot finds
// snapshot and snapshots, Class finds classes; Go finds neither good nor goes.
// No other morphology is inferred. Generation's source-backed occurrence
// check and the report's display lookup share this rule.

// FoldTerm folds letter case rune by rune, so a folded name can be found in a
// FoldedText byte for byte.
func FoldTerm(name string) string { return strings.Map(foldRune, name) }

func foldRune(r rune) rune { return unicode.ToLower(unicode.ToUpper(r)) }

// FoldedText is one text folded once, for finding many names in it.
type FoldedText struct {
	text, folded string
	// offsets maps each folded byte to its original byte, plus the end. It is
	// nil when folding kept every rune's length, as it does for most text.
	offsets []int
}

func FoldText(text string) FoldedText {
	result := FoldedText{text: text, folded: strings.Map(foldRune, text)}
	aligned := len(result.folded) == len(text)
	for i, r := range text {
		if !aligned {
			break
		}
		_, size := utf8.DecodeRuneInString(text[i:])
		aligned = utf8.RuneLen(foldRune(r)) == size
	}
	if aligned {
		return result
	}
	result.offsets = make([]int, 0, len(result.folded)+1)
	for i, r := range text {
		for range utf8.RuneLen(foldRune(r)) {
			result.offsets = append(result.offsets, i)
		}
	}
	result.offsets = append(result.offsets, len(text))
	return result
}

func (text FoldedText) original(folded int) int {
	if text.offsets == nil {
		return folded
	}
	return text.offsets[folded]
}

// Occurrence is one whole occurrence in the original text. Ending is the
// folded length of its plural ending, zero when the name stands alone.
type Occurrence struct {
	Start, End, Ending int
}

// Find returns the whole occurrences of a FoldTerm name. word tells the runes
// that continue a word; a letter of another concrete script does not, so
// Matcher를 still names Matcher.
func (text FoldedText) Find(name string, word func(rune) bool) []Occurrence {
	if name == "" {
		return nil
	}
	endings := []string{""}
	if last, _ := utf8.DecodeLastRuneInString(name); unicode.Is(unicode.Latin, last) {
		endings = append(endings, "s")
		for _, sibilant := range []string{"s", "x", "z", "ch", "sh"} {
			if strings.HasSuffix(name, sibilant) {
				endings = append(endings, "es")
				break
			}
		}
	}
	var result []Occurrence
	for from := 0; from < len(text.folded); {
		at := strings.Index(text.folded[from:], name)
		if at < 0 {
			break
		}
		start := from + at
		_, size := utf8.DecodeRuneInString(text.folded[start:])
		from = start + size
		if !wordStart(text.text, text.original(start), word) {
			continue
		}
		end := start + len(name)
		for _, ending := range endings {
			if !strings.HasPrefix(text.folded[end:], ending) || !wordEnd(text.text, text.original(end+len(ending)), word) {
				continue
			}
			result = append(result, Occurrence{Start: text.original(start), End: text.original(end + len(ending)), Ending: len(ending)})
			from = end + len(ending)
			break
		}
	}
	return result
}

func wordStart(text string, start int, word func(rune) bool) bool {
	if start == 0 {
		return true
	}
	before, _ := utf8.DecodeLastRuneInString(text[:start])
	first, _ := utf8.DecodeRuneInString(text[start:])
	return !word(before) || ScriptBoundary(before, first)
}

func wordEnd(text string, end int, word func(rune) bool) bool {
	if end == len(text) {
		return true
	}
	last, _ := utf8.DecodeLastRuneInString(text[:end])
	after, _ := utf8.DecodeRuneInString(text[end:])
	return !word(after) || ScriptBoundary(last, after)
}
