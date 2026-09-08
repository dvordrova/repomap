package report

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/terminology"
)

// DisplayTextTerm is an existing dictionary entry available beside this prose.
// IDs stay local. The translator receives names and definitions, never hint refs.
type DisplayTextTerm struct {
	ID          string `json:"id"`
	Spelling    string `json:"spelling"`
	Explanation string `json:"explanation"`
}

type displayTermSpan struct {
	Start int      `json:"start"`
	End   int      `json:"end"`
	IDs   []string `json:"ids"`
}

type displayTermPlan struct {
	Ref   string            `json:"ref"`
	Text  string            `json:"text"`
	Spans []displayTermSpan `json:"spans"`
}

func (entry DisplayTextEntry) validateTermBindings() error {
	seen := make(map[string]bool)
	definitions := make(map[string]string)
	for _, term := range entry.Terms {
		key := term.ID + "\x00" + term.Spelling
		if term.ID == "" || seen[key] || strings.TrimSpace(term.Spelling) == "" || strings.TrimSpace(term.Explanation) == "" {
			return fmt.Errorf("report: invalid glossary context in %s", entry.Ref)
		}
		seen[key] = true
		if previous, exists := definitions[term.ID]; exists && previous != term.Explanation {
			return fmt.Errorf("report: conflicting glossary definition in %s", entry.Ref)
		}
		definitions[term.ID] = term.Explanation
	}
	seen = make(map[string]bool)
	for _, value := range entry.Protected {
		if seen[value.Ref] || strings.Count(entry.Text, value.Ref) == 0 {
			return fmt.Errorf("report: invalid source placeholder in %s", entry.Ref)
		}
		seen[value.Ref] = true
	}
	return nil
}

// finishDisplayText restores literal source bytes once, then looks up complete
// glossary spellings in the final prose. The model never assigns a hint or its
// position. Equal spellings offer separate definitions, not an inferred sense.
func (entry DisplayTextEntry) finishDisplayText(text string) (string, []displayTermSpan, error) {
	protected := make(map[string]string, len(entry.Protected))
	for _, value := range entry.Protected {
		protected[value.Ref] = value.Text
	}
	plain := displayPlaceholder.ReplaceAllStringFunc(text, func(ref string) string {
		if value, ok := protected[ref]; ok {
			return value
		}
		return ref
	})
	var spans []displayTermSpan
	for _, match := range glossaryMatches(plain, entry.Terms) {
		spans = append(spans, displayTermSpan{
			Start: len(utf16.Encode([]rune(plain[:match.start]))),
			End:   len(utf16.Encode([]rune(plain[:match.end]))),
			IDs:   match.ids,
		})
	}
	return plain, spans, nil
}

type glossaryMatch struct {
	start, end int
	ids        []string
}

// Matches are literal and case-sensitive. Source syntax remains untouched;
// longest complete names win at a shared start, without nested highlights.
func glossaryMatches(text string, terms []DisplayTextTerm) []glossaryMatch {
	blocked := displayVerbatimSyntax.FindAllStringIndex(text, -1)
	for _, bounds := range displayAbsolutePath.FindAllStringSubmatchIndex(text, -1) {
		blocked = append(blocked, []int{bounds[2], bounds[3]})
	}
	byName := make(map[string][]string)
	for _, term := range terms {
		if term.Spelling != "" && !slices.Contains(byName[term.Spelling], term.ID) {
			byName[term.Spelling] = append(byName[term.Spelling], term.ID)
		}
	}
	var matches []glossaryMatch
	for spelling, ids := range byName {
		for from := 0; from < len(text); {
			at := strings.Index(text[from:], spelling)
			if at < 0 {
				break
			}
			start, end := from+at, from+at+len(spelling)
			from = end
			if !wholeGlossaryName(text, start, end) {
				continue
			}
			insideSource := false
			for _, source := range blocked {
				if start < source[1] && end > source[0] {
					insideSource = true
					break
				}
			}
			if !insideSource {
				matches = append(matches, glossaryMatch{start: start, end: end, ids: ids})
			}
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].start != matches[j].start {
			return matches[i].start < matches[j].start
		}
		return matches[i].end > matches[j].end
	})
	var result []glossaryMatch
	end := 0
	for _, match := range matches {
		if match.start >= end {
			result = append(result, match)
			end = match.end
		}
	}
	return result
}

func glossaryInQuestion(term pageGlossaryTerm, scope string) bool {
	for _, question := range term.Questions {
		if question.Href == "#"+scope {
			return true
		}
	}
	return false
}

// A model-supplied English alias and the native code name address the same
// definition. No translated spelling or morphological variant is inferred.
func glossarySpellings(term pageGlossaryTerm) []string {
	names := []string{term.OriginalName}
	if term.Name != "" && term.Name != term.OriginalName {
		names = append(names, term.Name)
	}
	return names
}

func glossaryWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || r == '_' || r == '$'
}

func wholeGlossaryName(text string, start, end int) bool {
	first, _ := utf8.DecodeRuneInString(text[start:end])
	last, _ := utf8.DecodeLastRuneInString(text[start:end])
	if start > 0 {
		r, _ := utf8.DecodeLastRuneInString(text[:start])
		if glossaryWordRune(r) && !terminology.ScriptBoundary(r, first) {
			return false
		}
	}
	if end < len(text) {
		r, _ := utf8.DecodeRuneInString(text[end:])
		if glossaryWordRune(r) && !terminology.ScriptBoundary(last, r) {
			return false
		}
	}
	return true
}

// prepareTerminology uses the same source-owned glossary context for translation
// and local lookup. A shared spelling never merges distinct definitions.
func (page *PreparedPage) prepareTerminology(role, text, scope string, names []string, own *pageGlossaryTerm) DisplayTextEntry {
	entry := DisplayTextEntry{Role: role, Scope: scope}
	byName := make(map[string][]pageGlossaryTerm)
	scoped := make(map[string][]pageGlossaryTerm)
	for _, term := range page.view.Glossary {
		for _, spelling := range glossarySpellings(term) {
			byName[spelling] = append(byName[spelling], term)
			if scope != "" && glossaryInQuestion(term, scope) {
				scoped[spelling] = append(scoped[spelling], term)
			}
		}
	}
	for spelling, terms := range scoped {
		byName[spelling] = terms
	}
	if own != nil {
		entry.Context = own.ID
		for _, spelling := range glossarySpellings(*own) {
			byName[spelling] = []pageGlossaryTerm{*own}
		}
	}
	var all []DisplayTextTerm
	for spelling, values := range byName {
		for _, value := range values {
			all = append(all, DisplayTextTerm{ID: value.ID, Spelling: spelling, Explanation: value.Explanation})
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Spelling != all[j].Spelling {
			return all[i].Spelling < all[j].Spelling
		}
		return all[i].ID < all[j].ID
	})
	used := make(map[string]bool)
	for _, match := range glossaryMatches(text, all) {
		for _, id := range match.ids {
			used[id] = true
		}
	}
	for _, term := range all {
		if used[term.ID] {
			entry.Terms = append(entry.Terms, term)
		}
	}
	entry.Text, entry.Protected = protectedDisplayText(text, names)
	return entry
}
