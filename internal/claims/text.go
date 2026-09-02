package claims

import (
	"strings"
	"unicode/utf8"
)

// quote is one located quote produced by a scanner.
type quote struct {
	Line int
	Text string
}

func splitLines(text string) []string {
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		lines[index] = strings.TrimSuffix(line, "\r")
	}
	return lines
}

// collapseSpace keeps the original words and folds every whitespace run.
func collapseSpace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func fits(text string) bool {
	return utf8.RuneCountInString(text) <= MaxTextRunes
}

// quoteWithin bounds a docstring-like quote: the whole text when it fits,
// otherwise its first paragraph, then its first sentence, then a
// word-boundary cut. The cut is the last resort for a single huge sentence.
func quoteWithin(raw string) string {
	whole := collapseSpace(raw)
	if fits(whole) {
		return whole
	}
	first := collapseSpace(firstParagraph(raw))
	if first != "" && fits(first) {
		return first
	}
	sentence := firstSentence(first)
	if sentence != "" && fits(sentence) {
		return sentence
	}
	return cutAtWord(first)
}

func firstParagraph(raw string) string {
	var lines []string
	for _, line := range splitLines(raw) {
		if strings.TrimSpace(line) == "" {
			if len(lines) > 0 {
				break
			}
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func firstSentence(text string) string {
	if index := strings.Index(text, ". "); index >= 0 {
		return text[:index+1]
	}
	return text
}

// cutAtWord returns the longest word-aligned prefix within MaxTextRunes.
func cutAtWord(text string) string {
	if fits(text) {
		return text
	}
	runes := []rune(text)
	prefix := string(runes[:MaxTextRunes])
	if space := strings.LastIndex(prefix, " "); space > 0 {
		prefix = prefix[:space]
	}
	return strings.TrimSpace(prefix)
}

// splitWithin splits collapsed text into word-aligned pieces that each fit;
// no words are lost.
func splitWithin(text string) []string {
	var pieces []string
	words := strings.Fields(text)
	current, currentRunes := make([]string, 0, len(words)), 0
	for _, word := range words {
		wordRunes := utf8.RuneCountInString(word)
		if len(current) > 0 && currentRunes+1+wordRunes > MaxTextRunes {
			pieces = append(pieces, strings.Join(current, " "))
			current, currentRunes = current[:0], 0
		}
		if len(current) > 0 {
			currentRunes++
		}
		current = append(current, word)
		currentRunes += wordRunes
	}
	if len(current) > 0 {
		pieces = append(pieces, strings.Join(current, " "))
	}
	return pieces
}
