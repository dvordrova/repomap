package facts

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/corpus"
)

const (
	// textMaxRunes bounds the quoted line of a TODO or risk row so a
	// minified line cannot flood the report.
	textMaxRunes = 120
	// binaryProbeBytes is how much of a file is inspected for NUL bytes
	// before it is treated as binary and skipped by regex passes.
	binaryProbeBytes = 8192
)

// sourceReader reads corpus files once and serves their lines to every
// extractor. Read failures become diagnostics, never partial facts.
type sourceReader struct {
	repository *corpus.Corpus
	cache      map[string]*sourceFile
	diagnose   func(kind, detail string)
}

type sourceFile struct {
	lines  []string
	binary bool
	size   int
}

func newSourceReader(repository *corpus.Corpus, diagnose func(kind, detail string)) *sourceReader {
	return &sourceReader{repository: repository, cache: make(map[string]*sourceFile), diagnose: diagnose}
}

// paths lists every readable corpus path in canonical order.
func (reader *sourceReader) paths() []string {
	if reader.repository == nil {
		return nil
	}
	entries := reader.repository.Entries()
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry.Path)
	}
	return result
}

func (reader *sourceReader) has(filePath string) bool {
	if reader.repository == nil {
		return false
	}
	_, ok := reader.repository.ID(filePath)
	return ok
}

// file returns the cached content of one corpus path, or false when the path
// is not part of the corpus or could not be read.
func (reader *sourceReader) file(filePath string) (*sourceFile, bool) {
	if cached, ok := reader.cache[filePath]; ok {
		return cached, cached != nil
	}
	if reader.repository == nil {
		return nil, false
	}
	id, ok := reader.repository.ID(filePath)
	if !ok {
		reader.cache[filePath] = nil
		return nil, false
	}
	content, err := reader.repository.ReadFileAll(id)
	if err != nil {
		reader.cache[filePath] = nil
		reader.diagnose("source_unreadable", filePath+": "+err.Error())
		return nil, false
	}
	result := &sourceFile{size: len(content.Bytes), binary: looksBinary(content.Bytes)}
	if !result.binary {
		result.lines = splitLines(content.Bytes)
	}
	reader.cache[filePath] = result
	return result, true
}

// line returns the trimmed content of one 1-based line, or "" when unknown.
func (reader *sourceReader) line(filePath string, number int) string {
	file, ok := reader.file(filePath)
	if !ok || file.binary || number <= 0 || number > len(file.lines) {
		return ""
	}
	return strings.TrimSpace(file.lines[number-1])
}

func looksBinary(content []byte) bool {
	probe := content
	if len(probe) > binaryProbeBytes {
		probe = probe[:binaryProbeBytes]
	}
	return bytes.IndexByte(probe, 0) >= 0
}

func splitLines(content []byte) []string {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// clipText trims a quoted source line to the report bound and strips bytes
// that would make the row invalid.
func clipText(value string) string {
	value = strings.TrimSpace(strings.ToValidUTF8(value, ""))
	value = strings.ReplaceAll(value, "\x00", "")
	if utf8.RuneCountInString(value) <= textMaxRunes {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:textMaxRunes]))
}

// isCommentLine reports a line that is only a comment, so a regex pass does
// not turn "# never call exec()" into a risk.
func isCommentLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	for _, prefix := range []string{"#", "//", "/*", "*", "<!--"} {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func sourceExtension(filePath string) string {
	dot := strings.LastIndex(filePath, ".")
	slash := strings.LastIndex(filePath, "/")
	if dot < 0 || dot < slash {
		return ""
	}
	return strings.ToLower(filePath[dot:])
}

func isSourceFile(filePath string) bool {
	switch sourceExtension(filePath) {
	case ".py", ".pyi", ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs", ".go":
		return true
	default:
		return false
	}
}

func isPythonFile(filePath string) bool {
	extension := sourceExtension(filePath)
	return extension == ".py" || extension == ".pyi"
}

func isJavaScriptFile(filePath string) bool {
	switch sourceExtension(filePath) {
	case ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs":
		return true
	default:
		return false
	}
}
