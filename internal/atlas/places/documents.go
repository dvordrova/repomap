package places

import (
	"fmt"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/atlas"
)

// Documentation enters the same saved graph as code, but never its code-file
// groups or call edges. Selection is by document syntax, not a framework or
// a model's prior opinion of which files matter.
func (b *builder) addDocuments(graph *atlas.Graph) error {
	for _, entry := range b.input.Repository.Entries() {
		if strings.ToLower(path.Ext(entry.Path)) != ".md" {
			continue
		}
		content, err := b.input.Repository.ReadFileAll(entry.ID)
		if err != nil {
			return err
		}
		if !utf8.Valid(content.Bytes) {
			return fmt.Errorf("atlas documents: %s is not UTF-8", entry.Path)
		}
		graph.Places = append(graph.Places, documentSections(entry.Path, string(content.Bytes))...)
	}
	return nil
}

// Split at Markdown headings outside fenced examples. Every source byte,
// including the preamble and reference-link definitions, remains in one section.
// Unsupported Markdown constructs remain ordinary text, never omitted.
func documentSections(filePath, text string) []atlas.Place {
	lines := strings.SplitAfter(text, "\n")
	var result []atlas.Place
	start, title := 0, filePath
	fence := ""
	emit := func(end int) {
		body := strings.Join(lines[start:end], "")
		if body == "" {
			return
		}
		last := start + strings.Count(body, "\n") + 1
		if strings.HasSuffix(body, "\n") {
			last--
		}
		result = append(result, atlas.Place{
			ID: fmt.Sprintf("doc:%s:%d", filePath, start+1), Kind: atlas.PlaceDocument,
			Path: filePath, LineNo: start + 1, TargetIDs: []string{}, Given: title,
			Document: &atlas.DocumentFacts{Title: title, Text: body, EndLine: last},
		})
	}
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(raw, "    ") || strings.HasPrefix(raw, "\t") {
			continue // An indented code example cannot open a heading or fence.
		}
		if fence != "" {
			if strings.HasPrefix(line, fence) && strings.Trim(line, string(fence[0])) == "" {
				fence = ""
			}
			continue
		}
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			fence = line[:len(line)-len(strings.TrimLeft(line, string(line[0])))]
			continue
		}
		prefix := len(line) - len(strings.TrimLeft(line, "#"))
		if prefix < 1 || prefix > 6 || len(line) > prefix && line[prefix] != ' ' && line[prefix] != '\t' {
			continue
		}
		emit(i)
		start = i
		title = truncateRunes(strings.TrimSpace(strings.Trim(line, "#")), maxLineRunes)
		if title == "" {
			title = filePath
		}
	}
	emit(len(lines))
	return result
}
