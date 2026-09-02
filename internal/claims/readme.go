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
		texts = append(texts, part.Text)
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
		for _, piece := range splitWithin(part.Text) {
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
		text := collapseSpace(match[1])
		if body := firstParagraphBelow(lines, index+1); body != "" {
			text = strings.TrimSpace(text + " — " + body)
		}
		for _, piece := range splitWithin(text) {
			result = append(result, quote{Line: index + 1, Text: piece})
		}
	}
	if result == nil {
		if parts := paragraphs(lines); len(parts) > 0 {
			for _, piece := range splitWithin(parts[0].Text) {
				result = append(result, quote{Line: parts[0].Line, Text: piece})
			}
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
