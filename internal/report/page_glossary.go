package report

import (
	"fmt"
	"slices"
	"sort"
	"strconv"

	"github.com/dvordrova/repomap/internal/terminology"
)

// pageGlossaryTerm is one accepted explanation with its original context.
// Equal spellings never establish equal meanings. The catalogue joins only
// identical records and retains every answer and map destination that uses one.
// Name may be an accepted English alias; OriginalName always names the code.
type pageGlossaryTerm struct {
	ID, Name, OriginalName, Explanation string
	Sources                             []pageAnchor
	Questions                           []pageLearnLink
	Places                              []pageLearnLink
	Code                                bool
}

type pageGlossarySourceFile struct {
	Path      string
	Locations []pageAnchor
}

// SourceFiles groups the saved context without selecting evidence for a
// definition. Every distinct destination remains available in the static HTML.
func (term pageGlossaryTerm) SourceFiles() []pageGlossarySourceFile {
	var files []pageGlossarySourceFile
	byPath := make(map[string]int)
	seen := make(map[pageAnchor]bool)
	for _, source := range term.Sources {
		if seen[source] {
			continue
		}
		seen[source] = true
		path := source.Path
		if path == "" {
			path = source.Text
		}
		at, exists := byPath[path]
		if !exists {
			at = len(files)
			byPath[path] = at
			files = append(files, pageGlossarySourceFile{Path: path})
		}
		if source.Line > 0 {
			source.Text = strconv.Itoa(source.Line)
		}
		files[at].Locations = append(files[at].Locations, source)
	}
	for _, file := range files {
		sort.SliceStable(file.Locations, func(i, j int) bool {
			return file.Locations[i].Line < file.Locations[j].Line
		})
	}
	return files
}

func collectGlossary(view *pageView) {
	view.Glossary = nil
	byID := make(map[string]int)
	add := func(value pageGlossaryTerm) {
		if at, exists := byID[value.ID]; exists {
			previous := &view.Glossary[at]
			for _, question := range value.Questions {
				if !slices.Contains(previous.Questions, question) {
					previous.Questions = append(previous.Questions, question)
				}
			}
			for _, place := range value.Places {
				if !slices.Contains(previous.Places, place) {
					previous.Places = append(previous.Places, place)
				}
			}
			return
		}
		byID[value.ID] = len(view.Glossary)
		view.Glossary = append(view.Glossary, value)
	}
	for _, concept := range view.LearnConcepts {
		name := concept.Name
		if concept.Alias != "" {
			name = concept.Alias
		}
		add(pageGlossaryTerm{ID: concept.ID, Name: name, OriginalName: concept.Name,
			Explanation: concept.Explanation, Code: true,
			Sources: []pageAnchor{concept.Source}, Places: concept.Places})
	}
	for _, question := range view.Questions {
		for _, answer := range question.Answers {
			for _, concept := range answer.Terms {
				if at, exists := byID[concept.ID]; exists {
					link := pageLearnLink{Title: question.Question, Href: "#" + question.ID}
					if !slices.Contains(view.Glossary[at].Questions, link) {
						view.Glossary[at].Questions = append(view.Glossary[at].Questions, link)
					}
				}
			}
		}
	}
	sort.SliceStable(view.Glossary, func(i, j int) bool {
		if view.Glossary[i].Name != view.Glossary[j].Name {
			return view.Glossary[i].Name < view.Glossary[j].Name
		}
		return view.Glossary[i].ID < view.Glossary[j].ID
	})
}

func (builder *pageBuilder) reducedGlossary(view *pageView) error {
	catalog := builder.data.Glossary
	if catalog == nil {
		return nil
	}
	if err := catalog.Validate(); err != nil {
		return fmt.Errorf("report: glossary: %w", err)
	}
	view.GlossaryPartialComparison = catalog.PartialComparison
	// Native declaration entries already have their complete anchors and links.
	// Domain reduction only adds its own accepted definitions.
	usedIDs := make(map[string]bool)
	for _, original := range view.Glossary {
		usedIDs[original.ID] = true
	}
	for _, entry := range catalog.Entries {
		for index, spelling := range entry.Names {
			term := pageGlossaryTerm{ID: fmt.Sprintf("term-%s-%d", entry.ID, index), Name: spelling, OriginalName: spelling,
				Explanation: entry.Explanation}
			for _, source := range entry.Sources {
				term.Sources = append(term.Sources, builder.links.anchor(source.Path, source.Line, 0))
			}
			// Exact request and row tie a definition to the question that supplied
			// its context. Equal words or neighbouring files do not bind senses.
			for _, question := range view.Questions {
				linked := false
				for _, answer := range question.Answers {
					for _, variant := range entry.Variants {
						if answer.RequestSHA256 != "" && answer.OriginRow != "" && slices.Contains(variant.Origins, terminology.Origin{RequestSHA256: answer.RequestSHA256, Row: answer.OriginRow}) {
							linked = true
						}
					}
				}
				if linked {
					link := pageLearnLink{Title: question.Question, Href: "#" + question.ID}
					if !slices.Contains(term.Questions, link) {
						term.Questions = append(term.Questions, link)
					}
				}
			}
			if usedIDs[term.ID] {
				return fmt.Errorf("report: glossary repeats a declaration destination")
			}
			usedIDs[term.ID] = true
			sortGlossaryLinks(term.Places)
			sortGlossaryLinks(term.Questions)
			view.Glossary = append(view.Glossary, term)
		}
	}
	sort.Slice(view.Glossary, func(i, j int) bool {
		if view.Glossary[i].OriginalName != view.Glossary[j].OriginalName {
			return view.Glossary[i].OriginalName < view.Glossary[j].OriginalName
		}
		return view.Glossary[i].ID < view.Glossary[j].ID
	})
	return nil
}

func sortGlossaryLinks(links []pageLearnLink) {
	sort.Slice(links, func(i, j int) bool {
		if links[i].Href != links[j].Href {
			return links[i].Href < links[j].Href
		}
		return links[i].Title < links[j].Title
	})
}
