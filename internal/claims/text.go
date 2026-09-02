package claims

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// minProseLetters is the shortest run of letters that makes a fragment read
// as a sentence rather than as badges or punctuation.
const minProseLetters = 3

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
// markupOnly matches a line whose visible content is markup rather than
// prose: an HTML tag, a badge image, or a bare link. Such a line says nothing
// a reader can act on, so it never becomes a claim.
var (
	htmlTag       = regexp.MustCompile(`<[^>]+>`)
	inlineImage   = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	referenceImg  = regexp.MustCompile(`!\[[^\]]*\]`)
	referenceLink = regexp.MustCompile(`\[([^\]]*)\]\[[^\]]*\]`)
	inlineLink    = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	emphasis      = regexp.MustCompile("[*_`]{1,3}")
	headingMark   = regexp.MustCompile(`(?m)^\s{0,3}#{1,6}\s*`)
	trailingMark  = regexp.MustCompile(`(?m)\s*#+\s*$`)
)

// readable turns one Markdown fragment into the prose a reader would see:
// images and tags drop out, links keep their text, emphasis markers go away.
// It returns the empty string when nothing but markup remains.
func readable(text string) string {
	// Images carry no prose at all; links keep the words a reader sees. Both
	// reference and inline forms appear in real README files.
	text = inlineImage.ReplaceAllString(text, " ")
	text = referenceImg.ReplaceAllString(text, " ")
	text = referenceLink.ReplaceAllString(text, "$1")
	text = inlineLink.ReplaceAllString(text, "$1")
	text = htmlTag.ReplaceAllString(text, " ")
	// Heading and emphasis markers are formatting, not words: a quote reads as
	// the sentence the author wrote.
	text = headingMark.ReplaceAllString(text, "")
	text = trailingMark.ReplaceAllString(text, "")
	text = emphasis.ReplaceAllString(text, "")
	text = collapseSpace(text)
	if !hasProse(text) {
		return ""
	}
	return text
}

// hasProse requires at least a few letters in a row, so a line of badges,
// punctuation, or a bare URL is not quoted as if it said something.
func hasProse(text string) bool {
	run := 0
	for _, r := range text {
		if unicode.IsLetter(r) {
			run++
			if run >= minProseLetters {
				return true
			}
			continue
		}
		run = 0
	}
	return false
}

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
