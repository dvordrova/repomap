package report

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf16"

	"github.com/dvordrova/repomap/internal/terminology"
)

// DisplayTextTerm is an existing dictionary entry available beside this prose.
// IDs stay local. The translator receives names and definitions, never hint refs.
type DisplayTextTerm struct {
	ID          string `json:"id"`
	Spelling    string `json:"spelling"`
	Explanation string `json:"explanation"`
	// Code marks a code declaration's name, matched exactly as written.
	Code bool `json:"code,omitempty"`
}

// exactSpelling keys a code declaration's spelling apart from every folded
// spelling, so the display lookup finds it only as written; acronymSpelling
// keys an acronym, whose plural ending must be written in lower case.
const (
	exactSpelling   = "\x00"
	acronymSpelling = "\x01"
)

func spellingKey(spelling string, code bool) string {
	switch {
	case code:
		return exactSpelling + spelling
	case terminology.IsAcronym(spelling):
		return acronymSpelling + terminology.FoldTerm(spelling)
	}
	return terminology.FoldTerm(spelling)
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

// finishDisplayText restores literal source bytes once, then looks up whole
// glossary names in the final prose. The model never assigns a hint or its
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
	start, end, ending int
	ids                []string
}

// Matches follow the shared term lookup (owner decision 2026-09-26): whole
// words in any letter case, alone or with an English plural ending. Spellings
// equal but for case offer their definitions together. Source syntax remains
// untouched; the longest complete name wins at a shared start, a name that
// needs no plural ending wins an equal span, and highlights never nest.
func glossaryMatches(text string, terms []DisplayTextTerm) []glossaryMatch {
	return glossaryMatchesBySpelling(text, glossarySpellingIDs(terms))
}

// glossarySpellingIDs keys each spelling's sorted term IDs by its folded case.
func glossarySpellingIDs(terms []DisplayTextTerm) map[string][]string {
	byName := make(map[string][]string)
	for _, term := range terms {
		name := spellingKey(term.Spelling, term.Code)
		if term.Spelling != "" && !slices.Contains(byName[name], term.ID) {
			byName[name] = append(byName[name], term.ID)
		}
	}
	for _, ids := range byName {
		slices.Sort(ids)
	}
	return byName
}

func glossaryMatchesBySpelling(text string, byName map[string][]string) []glossaryMatch {
	if len(byName) == 0 {
		return nil
	}
	return glossaryMatchesWith(text, findDisplaySyntax(text), byName)
}

func glossaryMatchesWith(text string, syntax displaySyntax, byName map[string][]string) []glossaryMatch {
	if len(byName) == 0 {
		return nil
	}
	blocked := slices.Clip(syntax.verbatim)
	for _, bounds := range syntax.paths {
		blocked = append(blocked, []int{bounds[0], bounds[1]})
	}
	folded := terminology.FoldText(text)
	var matches []glossaryMatch
	for name, ids := range byName {
		var occurrences []terminology.Occurrence
		if exact, ok := strings.CutPrefix(name, exactSpelling); ok {
			occurrences = folded.FindExact(exact, glossaryWordRune)
		} else if acronym, ok := strings.CutPrefix(name, acronymSpelling); ok {
			occurrences = folded.Find(acronym, true, glossaryWordRune)
		} else {
			occurrences = folded.Find(name, false, glossaryWordRune)
		}
		for _, found := range occurrences {
			insideSource := false
			for _, source := range blocked {
				if found.Start < source[1] && found.End > source[0] {
					insideSource = true
					break
				}
			}
			if !insideSource {
				matches = append(matches, glossaryMatch{start: found.Start, end: found.End, ending: found.Ending, ids: ids})
			}
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].start != matches[j].start {
			return matches[i].start < matches[j].start
		}
		if matches[i].end != matches[j].end {
			return matches[i].end > matches[j].end
		}
		return matches[i].ending < matches[j].ending
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
// definition. No translated spelling is inferred; lookup adds only letter case
// and the English plural ending.
func glossarySpellings(term pageGlossaryTerm) []string {
	names := []string{term.OriginalName}
	if term.Name != "" && term.Name != term.OriginalName {
		names = append(names, term.Name)
	}
	return names
}

// sameGlossaryDefinition reports whether two glossary terms would show the
// same definition: equal explanation, sources, questions and places.
func sameGlossaryDefinition(a, b pageGlossaryTerm) bool {
	return a.Explanation == b.Explanation && a.Code == b.Code && slices.Equal(a.Sources, b.Sources) &&
		slices.Equal(a.Questions, b.Questions) && slices.Equal(a.Places, b.Places)
}

// A letter, digit, mark, underscore or dollar sign continues a name, so a
// glossary word never lights up inside HMMish, _HMM or $HMM.
func glossaryWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || r == '_' || r == '$'
}

// prepareTerminology uses the same source-owned glossary context for translation
// and local lookup. A shared spelling never merges distinct definitions.
func (page *PreparedPage) prepareTerminology(role, text, scope string, names []string, own *pageGlossaryTerm) DisplayTextEntry {
	entry := DisplayTextEntry{Role: role, Scope: scope}
	if own != nil {
		entry.Context = own.ID
	}
	terms := page.terminologyFor(scope, own)
	all := terms.all
	syntax := page.displaySyntax(text)
	used := make(map[string]bool)
	for _, match := range glossaryMatchesWith(text, syntax, terms.bySpelling) {
		for _, id := range match.ids {
			used[id] = true
		}
	}
	for _, term := range all {
		if used[term.ID] {
			entry.Terms = append(entry.Terms, term)
		}
	}
	entry.Text, entry.Protected = protectedDisplayTextWith(text, syntax, page.nameIndex(names))
	return entry
}

// displaySyntax finds a text's source syntax once per page: the same label
// stands on both cards of a connection and on its map edge.
func (page *PreparedPage) displaySyntax(text string) displaySyntax {
	if syntax, ok := page.syntax[text]; ok {
		return syntax
	}
	syntax := findDisplaySyntax(text)
	if page.syntax == nil {
		page.syntax = make(map[string]displaySyntax)
	}
	page.syntax[text] = syntax
	return syntax
}

// nameIndex builds the protected-name index once for the page's name list.
func (page *PreparedPage) nameIndex(names []string) *displayNameIndex {
	if len(names) == 0 {
		return nil
	}
	if page.names == nil || page.namesFrom != &names[0] || page.namesCount != len(names) {
		page.names, page.namesFrom, page.namesCount = newDisplayNameIndex(names), &names[0], len(names)
	}
	return page.names
}

// pageTerms are the glossary terms one scope and one own term see, sorted,
// with each spelling's term IDs; they do not depend on the text.
type pageTerms struct {
	all        []DisplayTextTerm
	bySpelling map[string][]string
}

// terminologyFor builds the terms a text in this scope sees once per scope
// and own term instead of once per display text.
func (page *PreparedPage) terminologyFor(scope string, own *pageGlossaryTerm) pageTerms {
	key := scope + "\x00"
	if own != nil {
		key += own.ID
	}
	if cached, ok := page.terms[key]; ok {
		return cached
	}
	// A question's own sense, or a glossary entry's own definition, replaces
	// the other definitions of every spelling that lookup cannot tell apart.
	type spelled struct {
		spelling string
		term     pageGlossaryTerm
	}
	byName := make(map[string][]spelled)
	scoped := make(map[string][]spelled)
	for _, term := range page.view.Glossary {
		for _, spelling := range glossarySpellings(term) {
			name := spellingKey(spelling, term.Code)
			byName[name] = append(byName[name], spelled{spelling, term})
			if scope != "" && glossaryInQuestion(term, scope) {
				scoped[name] = append(scoped[name], spelled{spelling, term})
			}
		}
	}
	for name, terms := range scoped {
		byName[name] = terms
	}
	if own != nil {
		spellings := glossarySpellings(*own)
		for _, spelling := range spellings {
			delete(byName, spellingKey(spelling, own.Code))
		}
		for _, spelling := range spellings {
			name := spellingKey(spelling, own.Code)
			byName[name] = append(byName[name], spelled{spelling, *own})
		}
	}
	// A reduced entry keeps one glossary term per spelling, so Zipkin and
	// zipkin carry one identical definition. Lookup cannot tell them apart:
	// the name offers that definition once, under its first ID.
	for name, values := range byName {
		slices.SortStableFunc(values, func(a, b spelled) int { return strings.Compare(a.term.ID, b.term.ID) })
		var kept []pageGlossaryTerm
		byName[name] = slices.DeleteFunc(values, func(value spelled) bool {
			for _, other := range kept {
				if other.ID == value.term.ID {
					return false
				}
				if sameGlossaryDefinition(other, value.term) {
					return true
				}
			}
			kept = append(kept, value.term)
			return false
		})
	}
	var all []DisplayTextTerm
	for _, values := range byName {
		for _, value := range values {
			all = append(all, DisplayTextTerm{ID: value.term.ID, Spelling: value.spelling, Explanation: value.term.Explanation, Code: value.term.Code})
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Spelling != all[j].Spelling {
			return all[i].Spelling < all[j].Spelling
		}
		return all[i].ID < all[j].ID
	})
	terms := pageTerms{all: all, bySpelling: glossarySpellingIDs(all)}
	if page.terms == nil {
		page.terms = make(map[string]pageTerms)
	}
	page.terms[key] = terms
	return terms
}
