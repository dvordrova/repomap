package facts

import (
	"path"
	"strings"
)

// unanalysedLanguages are the programming languages a file's extension
// names that no adapter of repomap analyses. A file in one of them is code
// the report does not read: Redis's 204 tests live in test-redis.tcl, and
// "No recognized test files" had read as "no tests".
var unanalysedLanguages = map[string]string{
	".tcl": "Tcl", ".rb": "Ruby", ".java": "Java", ".kt": "Kotlin", ".kts": "Kotlin", ".scala": "Scala",
	".groovy": "Groovy", ".rs": "Rust", ".pl": "Perl", ".pm": "Perl", ".lua": "Lua", ".php": "PHP",
	".swift": "Swift", ".cs": "C#", ".fs": "F#", ".vb": "Visual Basic", ".mm": "Objective-C++",
	".cc": "C++", ".cpp": "C++", ".cxx": "C++", ".hh": "C++", ".hpp": "C++", ".hxx": "C++",
	".ex": "Elixir", ".exs": "Elixir", ".erl": "Erlang", ".hrl": "Erlang", ".hs": "Haskell",
	".ml": "OCaml", ".mli": "OCaml", ".dart": "Dart", ".jl": "Julia", ".zig": "Zig", ".nim": "Nim",
	".f90": "Fortran", ".f95": "Fortran", ".ps1": "PowerShell", ".sh": "Shell", ".bash": "Shell",
	".zsh": "Shell", ".s": "Assembly", ".asm": "Assembly", ".sol": "Solidity", ".r": "R",
	".lisp": "Common Lisp", ".el": "Emacs Lisp", ".rkt": "Racket", ".scm": "Scheme",
}

// unanalysedInterpreters name the language of a file without an extension
// by the interpreter its first line runs (utils/redis_init_script).
var unanalysedInterpreters = map[string]string{
	"sh": "Shell", "bash": "Shell", "zsh": "Shell", "dash": "Shell", "ksh": "Shell",
	"ruby": "Ruby", "perl": "Perl", "tclsh": "Tcl", "wish": "Tcl", "lua": "Lua", "php": "PHP",
}

// addUnanalysedFiles records every inspected file written in a language no
// adapter analyses, with its language and its lines: what the report does
// not read stays named, not silent. The file's lines are counted, never
// read into any analysis; a file whose lines cannot be counted keeps its
// language with no count.
func (b *builder) addUnanalysedFiles() {
	if b.input.Repository == nil {
		return
	}
	for _, filePath := range b.input.Repository.VisiblePaths() {
		language := b.unanalysedLanguage(filePath)
		if language == "" {
			continue
		}
		lines, err := b.input.Repository.Lines(filePath)
		if err != nil {
			b.diagnose("unanalysed_file_unreadable", filePath+": "+err.Error())
			lines = 0
		}
		b.add(".", Fact{Kind: KindUnanalysedFile, Anchor: &Anchor{Path: filePath}, Key: language, Lines: lines}, filePath)
	}
}

func (b *builder) unanalysedLanguage(filePath string) string {
	if extension := strings.ToLower(path.Ext(filePath)); extension != "" {
		return unanalysedLanguages[extension]
	}
	// An extensionless script is in the corpus by its #! line.
	file, readable := b.source.file(filePath)
	if !readable || file.binary || len(file.lines) == 0 || !strings.HasPrefix(file.lines[0], "#!") {
		return ""
	}
	words := strings.Fields(strings.TrimPrefix(file.lines[0], "#!"))
	if len(words) == 0 {
		return ""
	}
	interpreter := path.Base(words[0])
	if interpreter == "env" && len(words) > 1 {
		interpreter = words[1]
	}
	return unanalysedInterpreters[interpreter]
}
