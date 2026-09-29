package facts

import (
	"go/scanner"
	"go/token"
	"path"
	"regexp"
	"slices"
	"strings"
)

var todoMarker = regexp.MustCompile(`\b(TODO|FIXME|XXX|HACK)\b`)

// addTODOs scans the comments of every code file of the corpus: a TODO is
// a marker a comment in source code writes (owner, 2026-09-29: Redis's ten
// "TODOs" were <a name="TODO"> anchors of doc/*.html). A code file is one
// written in a language an adapter analyses or one it does not (the
// unanalysed_file languages); documents, markup and data are none. Rows under
// a target root are attributed to that target; the rest stay
// repository-level.
func (b *builder) addTODOs() {
	for _, filePath := range b.source.paths() {
		file, ok := b.source.file(filePath)
		if !ok || file.binary || !slices.ContainsFunc(file.lines, hasTODOWord) {
			continue
		}
		style, code := b.commentStyleOf(filePath)
		if !code {
			continue
		}
		targetID := b.targetForPath(filePath)
		root := b.rootForTarget(targetID)
		for number, line := range todoLines(filePath, file.lines, style) {
			if !hasTODOWord(line) {
				continue
			}
			match := todoMarker.FindStringSubmatchIndex(line)
			if match == nil {
				continue
			}
			marker := line[match[2]:match[3]]
			text := clipText(strings.TrimLeft(strings.TrimSpace(line[match[3]:]), ":- "))
			text = strings.TrimSpace(strings.TrimSuffix(text, "*/"))
			if text == "" {
				text = marker
			}
			b.add(root, Fact{
				Kind:     KindTODO,
				TargetID: targetID,
				Anchor:   &Anchor{Path: filePath, Line: number + 1},
				Key:      marker,
				Text:     text,
			}, marker, text)
		}
	}
}

// hasTODOWord is whether a line holds a marker's letters at all: a marker
// needs them, and a comment is part of its source lines, so a file without
// them is neither lexed nor matched.
func hasTODOWord(line string) bool {
	return strings.Contains(line, "TODO") || strings.Contains(line, "FIXME") ||
		strings.Contains(line, "XXX") || strings.Contains(line, "HACK")
}

// todoLines are the comments of a code file, one entry per physical line:
// Go's lexer tells actual comments from context.TODO(), identifiers and
// strings; other languages are read by their comment syntax, strings
// skipped. A language whose syntax is not known keeps its whole lines.
func todoLines(filePath string, lines []string, style commentStyle) []string {
	if path.Ext(filePath) != ".go" {
		if style.line == nil && style.open == "" {
			return lines
		}
		return style.comments(lines)
	}
	source := []byte(strings.Join(lines, "\n"))
	file := token.NewFileSet().AddFile(filePath, -1, len(source))
	var lexer scanner.Scanner
	lexer.Init(file, source, func(token.Position, string) {}, scanner.ScanComments)
	comments := make([]string, len(lines))
	for {
		pos, kind, text := lexer.Scan()
		if kind == token.EOF {
			break
		}
		if kind != token.COMMENT {
			continue
		}
		start := file.PositionFor(pos, false).Line - 1
		for offset, line := range strings.Split(text, "\n") {
			comments[start+offset] += " " + line
		}
	}
	return comments
}

// commentStyle is a language's comment syntax: its line comment openers, its
// block comment delimiters and its string quotes.
type commentStyle struct {
	line        []string
	open, close string
	quotes      string
}

var (
	slashComments = commentStyle{line: []string{"//"}, open: "/*", close: "*/", quotes: "\"'`"}
	hashComments  = commentStyle{line: []string{"#"}, quotes: "\"'"}
	lispComments  = commentStyle{line: []string{";"}, quotes: "\""}
	dashComments  = commentStyle{line: []string{"--"}, quotes: "\"'"}
)

// analysedStyles are the comment syntaxes of the languages the adapters
// analyse, by extension.
var analysedStyles = map[string]commentStyle{
	".go": slashComments, ".c": slashComments, ".h": slashComments,
	".js": slashComments, ".jsx": slashComments, ".mjs": slashComments, ".cjs": slashComments,
	".ts": slashComments, ".tsx": slashComments, ".mts": slashComments, ".cts": slashComments,
	".py": hashComments, ".pyi": hashComments,
	".clj": lispComments, ".cljs": lispComments, ".cljc": lispComments,
}

// unanalysedStyles are the comment syntaxes of the languages no adapter
// analyses, by the language name unanalysed_file gives them; a language
// missing here keeps its whole lines.
var unanalysedStyles = map[string]commentStyle{
	"Tcl": hashComments, "Ruby": hashComments, "Perl": hashComments, "Shell": hashComments, "R": hashComments,
	"Elixir": hashComments, "Julia": hashComments, "PowerShell": hashComments, "Nim": hashComments,
	"Java": slashComments, "Kotlin": slashComments, "Scala": slashComments, "Groovy": slashComments,
	"Rust": slashComments, "Swift": slashComments, "C#": slashComments, "F#": slashComments,
	"C++": slashComments, "Objective-C++": slashComments, "Dart": slashComments, "Zig": slashComments,
	"Solidity": slashComments, "PHP": {line: []string{"//", "#"}, open: "/*", close: "*/", quotes: "\"'"},
	"Lua": dashComments, "Haskell": dashComments,
	"Common Lisp": lispComments, "Emacs Lisp": lispComments, "Racket": lispComments, "Scheme": lispComments,
	"Assembly": lispComments,
}

// commentStyleOf is a file's comment syntax and whether it is code at all.
func (b *builder) commentStyleOf(filePath string) (commentStyle, bool) {
	if style, analysed := analysedStyles[strings.ToLower(path.Ext(filePath))]; analysed {
		return style, true
	}
	language := b.unanalysedLanguage(filePath)
	if language == "" {
		return commentStyle{}, false
	}
	return unanalysedStyles[language], true
}

// comments are the comment text of each physical line, read left to right:
// a string's contents are no comment, a line comment runs to the line's end
// and a block comment across lines.
func (style commentStyle) comments(lines []string) []string {
	out := make([]string, len(lines))
	inBlock := false
	for number, line := range lines {
		var said strings.Builder
		quote := byte(0)
		for at := 0; at < len(line); {
			switch {
			case inBlock:
				end := strings.Index(line[at:], style.close)
				if end < 0 {
					said.WriteString(" " + line[at:])
					at = len(line)
					continue
				}
				said.WriteString(" " + line[at:at+end])
				at += end + len(style.close)
				inBlock = false
			case quote != 0:
				if line[at] == '\\' {
					at += 2
					continue
				}
				if line[at] == quote {
					quote = 0
				}
				at++
			case style.open != "" && strings.HasPrefix(line[at:], style.open):
				inBlock = true
				at += len(style.open)
			case slices.ContainsFunc(style.line, func(opener string) bool { return strings.HasPrefix(line[at:], opener) }):
				said.WriteString(" " + line[at:])
				at = len(line)
			default:
				if strings.IndexByte(style.quotes, line[at]) >= 0 {
					quote = line[at]
				}
				at++
			}
		}
		out[number] = said.String()
	}
	return out
}
