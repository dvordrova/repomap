package terminology

import (
	"sort"
	"strings"
	"unicode"
)

// TermOccurrences are the places in the repository's files where one
// spelling of a glossary entry is written. They are code facts found by the
// shared term lookup, not the definition's evidence: the glossary once listed
// every file of the analysis its prose came from, lzf.h and solarisfixes.h
// among the files "in which" event loop appeared.
type TermOccurrences struct {
	Entry   string   `json:"entry"`
	Name    string   `json:"name"`
	Sources []Source `json:"sources"`
}

// Paths lists the corpus files the collector knows, in order.
func (c *Collector) Paths() []string {
	paths := make([]string, 0, len(c.paths))
	for path := range c.paths {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

// Occurrences finds every line of the given files on which a spelling of a
// catalogue entry is written, as a whole word or phrase in any letter case
// with an English plural ending (the shared term lookup). read returns a
// file's text; a file it cannot read is left out. Spellings no file writes
// have no row.
func Occurrences(catalog Catalog, paths []string, read func(string) (string, bool)) []TermOccurrences {
	word := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) || r == '_' }
	type spelling struct{ entry, name string }
	found := make(map[spelling][]Source)
	var order []spelling
	for _, entry := range catalog.Entries {
		for _, name := range entry.Names {
			order = append(order, spelling{entry.ID, name})
		}
	}
	sorted := append([]string(nil), paths...)
	sort.Strings(sorted)
	for _, path := range sorted {
		text, ok := read(path)
		if !ok {
			continue
		}
		folded := FoldText(text)
		for _, key := range order {
			last := 0
			for _, occurrence := range folded.Find(FoldTerm(key.name), IsAcronym(key.name), word) {
				line := strings.Count(text[:occurrence.Start], "\n") + 1
				if line != last {
					found[key] = append(found[key], Source{Path: path, Line: line})
					last = line
				}
			}
		}
	}
	var result []TermOccurrences
	for _, key := range order {
		if sources := found[key]; len(sources) > 0 {
			result = append(result, TermOccurrences{Entry: key.entry, Name: key.name, Sources: sources})
		}
	}
	return result
}
