package claims

import (
	"path"
	"regexp"
	"strings"
)

// MaxWholeReadmeBytes is the README size up to which the whole text is one
// quote; larger files are quoted heading by heading.
const MaxWholeReadmeBytes = 2048

var (
	readmeName      = regexp.MustCompile(`(?i)^readme(\.|$)`)
	markdownHeading = regexp.MustCompile(`^#{1,6}\s+(.*?)\s*#*\s*$`)
)

func isReadmePath(filePath string) bool {
	return readmeName.MatchString(path.Base(filePath))
}

// paragraph is a run of non-blank prose lines; Line is its first line.
type paragraph struct {
	Line int
	Text string
}

func readmeQuotes(size int, lines []string) []quote {
	prose := proseLines(lines)
	if size <= MaxWholeReadmeBytes {
		return wholeReadmeQuotes(prose)
	}
	return headingQuotes(prose)
}

// proseLines blanks every line inside a fenced code block so the fence never
// becomes a quote and always ends a paragraph.
func proseLines(lines []string) []string {
	result := make([]string, len(lines))
	inFence := false
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if !inFence {
			result[index] = line
		}
	}
	return result
}

func paragraphs(lines []string) []paragraph {
	var result []paragraph
	var current []string
	start := 0
	flush := func() {
		if len(current) > 0 {
			result = append(result, paragraph{Line: start + 1, Text: collapseSpace(strings.Join(current, " "))})
			current = nil
		}
	}
	for index, line := range lines {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		if len(current) == 0 {
			start = index
		}
		current = append(current, line)
	}
	flush()
	return result
}

// wholeReadmeQuotes quotes a short README as one claim at line 1, falling
// back to per-paragraph pieces when the whole text would not fit.
func wholeReadmeQuotes(lines []string) []quote {
	parts := paragraphs(lines)
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if readablePart := readable(part.Text); readablePart != "" {
			texts = append(texts, readablePart)
		}
	}
	whole := strings.Join(texts, " ")
	if whole == "" {
		return nil
	}
	if fits(whole) {
		return []quote{{Line: 1, Text: whole}}
	}
	var result []quote
	for _, part := range parts {
		readablePart := readable(part.Text)
		if readablePart == "" {
			continue
		}
		for _, piece := range splitWithin(readablePart) {
			result = append(result, quote{Line: part.Line, Text: piece})
		}
	}
	return result
}

// headingQuotes quotes each heading with the first paragraph beneath it. A
// long README without headings falls back to its first paragraph.
func headingQuotes(lines []string) []quote {
	var result []quote
	for index, line := range lines {
		match := markdownHeading.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		heading := readable(match[1])
		body := readable(firstParagraphBelow(lines, index+1))
		// A heading with no prose beneath it states nothing; the section it
		// opens is still reachable through the anchor of a later claim.
		if body == "" {
			continue
		}
		text := body
		if heading != "" {
			text = heading + " — " + body
		}
		for _, piece := range splitWithin(text) {
			result = append(result, quote{Line: index + 1, Text: piece})
		}
	}
	if result == nil {
		for _, part := range paragraphs(lines) {
			readablePart := readable(part.Text)
			if readablePart == "" {
				continue
			}
			for _, piece := range splitWithin(readablePart) {
				result = append(result, quote{Line: part.Line, Text: piece})
			}
			break
		}
	}
	return result
}

func firstParagraphBelow(lines []string, from int) string {
	var collected []string
	for _, line := range lines[from:] {
		blank := strings.TrimSpace(line) == ""
		if markdownHeading.MatchString(line) || (blank && len(collected) > 0) {
			break
		}
		if !blank {
			collected = append(collected, line)
		}
	}
	return collapseSpace(strings.Join(collected, " "))
}
