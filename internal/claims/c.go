package claims

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// cComment is one C comment block: a /* */ comment, or a run of // comments
// on consecutive lines of their own. Lines are 0-based.
type cComment struct {
	start, end int
	prose      []string // the words on each line, without comment markers
	ownsLine   bool     // only whitespace precedes it on its first line
	codeAfter  bool     // code follows it on its last line
	lineRun    bool     // a run of // comments
}

var (
	// cLicence marks a licence or copyright block. At the top of a file it is
	// not the authors' description of the code (owner decision D5).
	cLicence = regexp.MustCompile(`(?i)\bcopyright\b|\blicen[cs]e[ds]?\b|spdx-license-identifier|all rights reserved|permission is hereby granted|redistribution and use`)
	// cVersionStamp is a version-control keyword such as $Id: ... $ or
	// $NetBSD: ... $, which licence headers carry beside the licence.
	cVersionStamp = regexp.MustCompile(`^\$[A-Z][A-Za-z]*(?::[^$]*)?\$$`)
	// cDecoration is a run of one repeated decoration character.
	cDecoration = regexp.MustCompile(`={4,}|-{4,}|\*{4,}|#{4,}|~{4,}|_{4,}|\+{4,}|/{4,}`)
)

const (
	// cBannerWords is the most a section banner's title says.
	cBannerWords = 8
	// cHeaderLines is how many lines below its first line a declaration's
	// header may end: static / int / foo(void).
	cHeaderLines = 2
)

// CPath reports a C source or header file, whose comments cQuotes reads.
func CPath(filePath string) bool {
	return classifyPath(filePath) == kindC
}

// CDocstring is the text of the C docstring in docs that describes the
// declaration located at line, or "" when none does; declarations are the
// lines the file's other declarations are located at. A C docstring names the
// line where the declaration directly below it ends its header, and a
// declaration is located at its name, on that line or on the few above it
// (static int / foo(void), struct foo / {). So the file's own description,
// which names no line, describes no declaration, and neither does a comment
// above a prototype or a macro, on whose line no declaration is located.
func CDocstring(docs []Claim, line int, declarations []int) string {
	for _, doc := range docs {
		if doc.DeclarationLine == 0 || line <= doc.Line || line > doc.DeclarationLine || doc.DeclarationLine-line > cHeaderLines {
			continue
		}
		between := false
		for _, declared := range declarations {
			if declared > doc.Line && declared < line {
				between = true
				break
			}
		}
		if !between {
			return doc.Text
		}
	}
	return ""
}

// cQuotes returns a C file's author quotes. A comment block that ends directly
// above a top-level declaration (a line that starts at column 0 with a name)
// is a docstring whose DeclarationLine is the line where that declaration's
// header ends and its name is written. A comment block before
// the file's first line of code is a docstring too, the file's own
// description when no declaration follows it directly, except a licence,
// copyright or version-control stamp there (D5). Section banners such
// as /* ==== Lists ==== */ are the author's layout, not a claim about the code
// (D4). Marker comments (NOTE:, WARNING:, ...) are quoted line by line from
// every other comment. Comment markers inside strings and character
// constants open nothing.
func cQuotes(lines []string) (docs, markers []quote) {
	comments, code := cLex(lines)
	firstCode := len(lines)
	for index, hasCode := range code {
		if hasCode {
			firstCode = index
			break
		}
	}
	for _, comment := range comments {
		leading := comment.end < firstCode
		if leading && (comment.hasLicence() || comment.versionStamp()) || comment.banner() {
			continue
		}
		for offset, prose := range comment.prose {
			if commentMarker.MatchString(prose) {
				if text := quoteWithin(prose); text != "" {
					markers = append(markers, quote{Line: comment.start + offset + 1, Text: text})
				}
			}
		}
		if !comment.ownsLine || comment.codeAfter {
			continue
		}
		next := comment.end + 1
		above := next < len(lines) && cTopLevelDeclaration(lines[next])
		if !leading && !above {
			continue
		}
		if item, ok := docstringQuote(comment.start, strings.Join(comment.prose, "\n")); ok {
			// Where the declaration writes its name; none for the file's own
			// description, which no declaration follows directly.
			if above {
				item.DeclarationLine = cHeaderEnd(lines, next) + 1
			}
			docs = append(docs, item)
		}
	}
	return docs, markers
}

func (comment cComment) hasLicence() bool {
	return cLicence.MatchString(strings.Join(comment.prose, "\n"))
}

func (comment cComment) versionStamp() bool {
	return cVersionStamp.MatchString(strings.TrimSpace(strings.Join(comment.prose, " ")))
}

// banner reports a decorated section title: a decoration run and at most a
// few words besides it.
func (comment cComment) banner() bool {
	text := strings.Join(comment.prose, " ")
	if !cDecoration.MatchString(text) {
		return false
	}
	return len(strings.Fields(cDecoration.ReplaceAllString(text, " "))) <= cBannerWords
}

// cHeaderEnd is the line that ends a declaration's header, where it writes
// its name: the first line from first on with a parenthesis, a brace, a
// semicolon or an initializer (static int / foo(void) ends on foo's line).
// A header longer than cHeaderLines ends on its first line.
func cHeaderEnd(lines []string, first int) int {
	for index := first; index < len(lines) && index <= first+cHeaderLines; index++ {
		if strings.ContainsAny(lines[index], "({;=") {
			return index
		}
	}
	return first
}

// cTopLevelDeclaration is a line that starts at column 0 with a name: a
// function, a prototype, a type or a file-scope variable. Statements inside
// a body are indented; directives start with #.
func cTopLevelDeclaration(line string) bool {
	first, _ := utf8.DecodeRuneInString(line)
	return first == '_' || unicode.IsLetter(first)
}

// cLex finds the comments of a C file and which lines hold code.
func cLex(lines []string) ([]cComment, []bool) {
	source := strings.Join(lines, "\n")
	code := make([]bool, len(lines))
	var comments []cComment
	line, lineStart := 0, 0
	// codeBefore reports code on the current line before position at.
	codeBefore := func(at int) bool {
		return strings.TrimSpace(source[lineStart:at]) != "" && !cOnlyComments(source[lineStart:at])
	}
	for i := 0; i < len(source); {
		switch c := source[i]; {
		case c == '\n':
			line++
			i++
			lineStart = i
		case strings.HasPrefix(source[i:], "//"):
			end := strings.IndexByte(source[i:], '\n')
			if end < 0 {
				end = len(source) - i
			}
			owns := !codeBefore(i)
			prose := strings.TrimSpace(strings.TrimLeft(source[i+2:i+end], "/!"))
			last := len(comments) - 1
			if owns && last >= 0 && comments[last].lineRun && comments[last].end == line-1 && comments[last].ownsLine {
				comments[last].end = line
				comments[last].prose = append(comments[last].prose, prose)
			} else {
				comments = append(comments, cComment{start: line, end: line, prose: []string{prose}, ownsLine: owns, lineRun: true})
			}
			i += end
		case strings.HasPrefix(source[i:], "/*"):
			end := strings.Index(source[i+2:], "*/")
			body := ""
			next := len(source)
			if end >= 0 {
				body = source[i+2 : i+2+end]
				next = i + 2 + end + 2
			} else {
				body = source[i+2:]
			}
			comment := cComment{start: line, ownsLine: !codeBefore(i)}
			for _, text := range strings.Split(body, "\n") {
				comment.prose = append(comment.prose, cBlockProse(text))
			}
			if newlines := strings.Count(source[i:next], "\n"); newlines > 0 {
				line += newlines
				lineStart = i + strings.LastIndexByte(source[i:next], '\n') + 1
			}
			comment.end = line
			rest := source[next:]
			if eol := strings.IndexByte(rest, '\n'); eol >= 0 {
				rest = rest[:eol]
			}
			comment.codeAfter = strings.TrimSpace(rest) != "" && !cOnlyComments(rest)
			comments = append(comments, comment)
			i = next
		case c == '"' || c == '\'':
			// A string or character constant ends at its quote or, left
			// open, at the end of its line; an escaped newline continues it.
			code[line] = true
			j := i + 1
			for j < len(source) && source[j] != c && source[j] != '\n' {
				if source[j] == '\\' && j+1 < len(source) {
					if source[j+1] == '\n' {
						line++
						lineStart = j + 2
						code[line] = true
					}
					j += 2
					continue
				}
				j++
			}
			if j < len(source) && source[j] == c {
				j++
			}
			i = j
		default:
			if !unicode.IsSpace(rune(c)) {
				code[line] = true
			}
			i++
		}
	}
	return comments, code
}

// cOnlyComments reports text made of comments and whitespace only.
func cOnlyComments(text string) bool {
	text = strings.TrimSpace(text)
	for text != "" {
		switch {
		case strings.HasPrefix(text, "//"):
			return true
		case strings.HasPrefix(text, "/*"):
			end := strings.Index(text[2:], "*/")
			if end < 0 {
				return true
			}
			text = strings.TrimSpace(text[2+end+2:])
		default:
			return false
		}
	}
	return true
}

// cBlockProse strips the leading * of a block comment's line (the second *
// of /** too); a longer run of stars is decoration and stays.
func cBlockProse(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "*") && !strings.HasPrefix(text, "**") {
		text = text[1:]
	}
	return strings.TrimSpace(text)
}
