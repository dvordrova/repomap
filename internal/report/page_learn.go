package report

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
)

type pageLearnLink struct {
	Title string
	Href  string
}

type pageLearnQuestionTopic struct {
	Title     string
	Questions []pageLearnLink
}

type pageLearnPart struct {
	Title    string
	Href     string
	Summary  string
	Kind     string
	Language string
	Areas    []pageLearnLink
}

type pageLearnBand struct {
	Title string
	Parts []pageLearnPart
}

type pageLearnConcept struct {
	pageMapConcept
	ID      string
	Context string
	Places  []pageLearnLink
}

// Learn is an entrance to the existing maps. It uses their roles, hierarchy
// and explanations; no second classification or knowledge graph is created.
func (builder *pageBuilder) learn(view *pageView) {
	bands := map[string][]pageLearnPart{}
	conceptAt := map[string]int{}
	for _, section := range view.Sections {
		part := pageLearnPart{Title: section.ShortLabel, Href: "#" + section.ID, Kind: section.Kind, Language: section.Language}
		if index := builder.graphIndex(section.programTargetID); index != nil {
			part.Summary = index.Summary
		}
		if section.Map != nil {
			children := map[string]bool{}
			for _, node := range section.Map.Nodes {
				for _, id := range strings.Fields(node.Children) {
					children[id] = true
				}
			}
			for _, node := range section.Map.Nodes {
				if node.Remote || node.Activation != "" {
					continue
				}
				if !children[node.ID] {
					part.Areas = append(part.Areas, pageLearnLink{Title: node.FullTitle, Href: "#" + node.ID})
				}
				// This is the same bounded type explanation already shipped to
				// the inspector. Repeated memberships retain all destinations.
				var concepts []pageMapConcept
				if node.Concepts == "" || json.Unmarshal([]byte(node.Concepts), &concepts) != nil {
					continue
				}
				for _, concept := range concepts {
					key := concept.Source.Path + ":" + strconv.Itoa(concept.Source.Line) + "|" + concept.Name + "|" + concept.Explanation
					at, exists := conceptAt[key]
					if !exists {
						at = len(view.LearnConcepts)
						conceptAt[key] = at
						view.LearnConcepts = append(view.LearnConcepts, pageLearnConcept{pageMapConcept: concept, ID: fmt.Sprintf("concept-%x", sha256.Sum256([]byte(key)))})
					}
					place := pageLearnLink{Title: section.ShortLabel + " / " + node.FullTitle, Href: "#" + node.ID}
					view.LearnConcepts[at].Places = append(view.LearnConcepts[at].Places, place)
				}
			}
		}
		sort.SliceStable(part.Areas, func(i, j int) bool { return part.Areas[i].Title < part.Areas[j].Title })
		band := builder.repoRole(section)
		bands[band] = append(bands[band], part)
	}
	var names []string
	for name := range bands {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		view.LearnBands = append(view.LearnBands, pageLearnBand{Title: name[2:], Parts: bands[name]})
	}
	sort.SliceStable(view.LearnConcepts, func(i, j int) bool { return view.LearnConcepts[i].Name < view.LearnConcepts[j].Name })
	conceptNames := map[string]int{}
	for _, concept := range view.LearnConcepts {
		conceptNames[concept.Name]++
	}
	for i := range view.LearnConcepts {
		concept := &view.LearnConcepts[i]
		if conceptNames[concept.Name] > 1 {
			concept.Context = concept.Source.Text
		}
	}
	questionConcepts(view)
	learningQuestionTopics(view, builder.data.Learning)
}

func learningQuestionTopics(view *pageView, plan *atlas.LearningPlan) {
	add := func(title string, question *pageQuestion) {
		at := -1
		for i, topic := range view.LearnQuestionTopics {
			if topic.Title == title {
				at = i
				break
			}
		}
		if at < 0 {
			at = len(view.LearnQuestionTopics)
			view.LearnQuestionTopics = append(view.LearnQuestionTopics, pageLearnQuestionTopic{Title: title})
		}
		for _, link := range view.LearnQuestionTopics[at].Questions {
			if link.Href == "#"+question.ID {
				return
			}
		}
		view.LearnQuestionTopics[at].Questions = append(view.LearnQuestionTopics[at].Questions, pageLearnLink{Title: question.Question, Href: "#" + question.ID})
	}
	for _, q := range view.Questions {
		if q.UserQuestion || len(q.Origins) == 0 {
			add("Your questions", q)
		}
	}
	if plan == nil {
		return
	}
	for _, review := range plan.Reviews {
		for _, q := range view.Questions {
			for _, origin := range q.Origins {
				if origin.Title == review.Title {
					add(review.Title, q)
				}
			}
		}
	}
}

// A definition is offered only when the answer selected that declaration.
// Sharing a word, file or group is insufficient: another type named Lease
// can mean something else. Explanations and all memberships are reused intact.
func questionConcepts(view *pageView) {
	for _, question := range view.Questions {
		for i := range question.Answers {
			answer := &question.Answers[i]
			for _, concept := range view.LearnConcepts {
				for _, check := range answer.Checks {
					if check.Name == concept.Name && check.Source.Path == concept.Source.Path && check.Source.Line == concept.Source.Line {
						answer.Terms = append(answer.Terms, concept)
						break
					}
				}
			}
		}
	}
}
