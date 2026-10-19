package typesafe

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/dvordrova/repomap/internal/llm"
)

//go:embed text_model.md
var textModelSystem string

// TextModel answers the closed questions with the run's text model when Jev
// is not configured. The model reads numbered questions as plain text and
// writes one answer per line in order; Complete turns those lines into Jev's
// answers object (the chosen option at 1, the others at 0; yes at 1 or 0),
// so the executor, the cache and Verdicts read it as Jev's.
type TextModel struct {
	Provider llm.Provider
	// Controller is the gate the run's text-model stages use: the closed
	// questions go to the same account, so one 429 cools down both.
	Controller *llm.BatchController
	reader     Client
	// asked keeps each prompt's questions by its text, and each prepared
	// request's by its bytes, so Complete knows which line answers which.
	asked    sync.Map // prompt text → []askedQuestion
	prepared sync.Map // sha256 of prepared bytes → []askedQuestion
}

type askedQuestion struct {
	key     string
	options []string // none for yes/no
}

var (
	_ llm.SectionedCategorizer   = (*TextModel)(nil)
	_ llm.AttemptControllerOwner = (*TextModel)(nil)
)

// AttemptController is the run's text-model gate, or nil.
func (m *TextModel) AttemptController() *llm.BatchController { return m.Controller }

func (m *TextModel) State() []byte {
	state, _ := json.Marshal(map[string]json.RawMessage{
		"categorizer": json.RawMessage(`"text_model.lines.v7"`),
		"provider":    m.Provider.State(),
	})
	return state
}

func (m *TextModel) Prompt(task string, shared map[string]any, questions map[string]llm.Question) (llm.Prompt, error) {
	return m.prompt(task, []llm.Section{{Context: shared, Questions: questions}})
}

// PromptSections asks several windows in one request: each context is
// written before its own questions, numbered on across the request. One
// section is Prompt's request.
func (m *TextModel) PromptSections(task string, sections []llm.Section) (llm.Prompt, error) {
	return m.prompt(task, sections)
}

func (m *TextModel) prompt(task string, sections []llm.Section) (llm.Prompt, error) {
	type numbered struct {
		key      string
		question llm.Question
		section  int
	}
	var flat []numbered
	for s, section := range sections {
		keys := make([]string, 0, len(section.Questions))
		for key := range section.Questions {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			flat = append(flat, numbered{key: key, question: section.Questions[key], section: s})
		}
	}
	single := len(sections) == 1
	var text strings.Builder
	text.WriteString("Task:\n" + task + "\n")
	if single && len(sections[0].Context) > 0 {
		encoded, err := json.Marshal(sections[0].Context)
		if err != nil {
			return llm.Prompt{}, err
		}
		text.WriteString("\nContext:\n" + string(encoded) + "\n")
	}
	// An option list several questions share is written once, before the
	// questions, and each of them names it: the list with its meanings is
	// most of a question, and a window's questions usually share one.
	lists := make([]string, len(flat))
	uses := map[string]int{}
	for i, one := range flat {
		var list strings.Builder
		for _, option := range one.question.Options {
			fmt.Fprintf(&list, "- %s%s\n", option.Name, optionMeaning(option))
		}
		lists[i] = list.String()
		uses[lists[i]]++
	}
	labels := map[string]string{}
	for _, list := range lists {
		if list != "" && uses[list] > 1 && labels[list] == "" {
			labels[list] = listLabel(len(labels))
			if len(labels) == 1 {
				text.WriteString("\nOption lists:\n")
			}
			fmt.Fprintf(&text, "\nList %s:\n%s", labels[list], list)
		}
	}
	text.WriteString("\nQuestions:\n")
	asked := make([]askedQuestion, 0, len(flat))
	for i, one := range flat {
		if !single && (i == 0 || flat[i-1].section != one.section) {
			context := "none"
			if shared := sections[one.section].Context; len(shared) > 0 {
				encoded, err := json.Marshal(shared)
				if err != nil {
					return llm.Prompt{}, err
				}
				context = string(encoded)
			}
			fmt.Fprintf(&text, "\nContext %d:\n%s\n", one.section+1, context)
		}
		question := one.question
		name := question.Name
		if name == "" {
			name = "row"
		}
		item, err := json.Marshal(question.Item)
		if err != nil {
			return llm.Prompt{}, err
		}
		// The key is written beside the number: two windows asking the same
		// items under other keys are other requests, never one cached answer
		// read under the other window's keys.
		fmt.Fprintf(&text, "\n%d. [%s] %s\n   %s: %s\n", i+1, one.key, question.Ask, name, item)
		asking := askedQuestion{key: one.key}
		for _, option := range question.Options {
			asking.options = append(asking.options, option.Name)
		}
		switch {
		case len(question.Options) == 0:
			text.WriteString("   Answer yes or no.\n")
		case labels[lists[i]] != "":
			fmt.Fprintf(&text, "   Options: list %s\n", labels[lists[i]])
		default:
			text.WriteString("   Options:\n")
			for _, line := range strings.SplitAfter(lists[i], "\n") {
				if line != "" {
					text.WriteString("   " + line)
				}
			}
		}
		asked = append(asked, asking)
	}
	m.asked.Store(text.String(), asked)
	return llm.Prompt{System: textModelSystem, User: text.String(), NoResponseAdjunct: true}, nil
}

// listLabel names the nth shared option list: A … Z, then AA, AB and on.
func listLabel(n int) string {
	label := ""
	for n++; n > 0; n = (n - 1) / 26 {
		label = string(rune('A'+(n-1)%26)) + label
	}
	return label
}

func optionMeaning(option llm.Option) string {
	if option.Criteria == nil {
		if option.Meaning == "" {
			return ""
		}
		return ": " + option.Meaning
	}
	parts := []string{option.Criteria.What}
	if option.Criteria.Includes != "" {
		parts = append(parts, "includes "+option.Criteria.Includes)
	}
	if option.Criteria.NotFor != "" {
		parts = append(parts, "not for "+option.Criteria.NotFor)
	}
	if len(option.Criteria.Examples) > 0 {
		parts = append(parts, "e.g. "+strings.Join(option.Criteria.Examples, ", "))
	}
	return ": " + strings.Join(parts, "; ")
}

func (m *TextModel) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	asked, ok := m.asked.Load(prompt.User)
	if !ok {
		return llm.Prepared{}, errors.New("typesafe: text model prompt was not built by Prompt")
	}
	// The text model's request size is its provider's (REPOMAP_LLM_BYTES_LIMIT),
	// not Jev's envelope: the provider refuses a larger window for its owner
	// to partition.
	// Owners size Jev's output as one token; a text model writes its lines.
	limits.MaxOutputTokens = llm.DefaultMaxOutputTokens
	prepared, err := m.Provider.Prepare(prompt, limits)
	if err != nil {
		return llm.Prepared{}, err
	}
	m.prepared.Store(sha256.Sum256(prepared.Bytes()), asked)
	return prepared, nil
}

func (m *TextModel) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	asked, ok := m.prepared.Load(sha256.Sum256(prepared.Bytes()))
	if !ok {
		return llm.Completion{}, errors.New("typesafe: text model request was not prepared by Prepare")
	}
	completion, err := m.Provider.Complete(ctx, prepared)
	if err != nil {
		return completion, err
	}
	answers, err := answersFromLines(asked.([]askedQuestion), completion.Response)
	if err != nil {
		return completion, err
	}
	completion.Response = answers
	return completion, nil
}

func (m *TextModel) Verdicts(response []byte) (map[string]llm.Verdict, error) {
	return m.reader.Verdicts(response)
}

// answersFromLines reads the model's lines into Jev's answers object, taking
// only what the format defines: a line is "N. answer" with N a question's
// number; any other line, and a number outside the questions, is no answer
// and shifts nothing. The answer is yes or no, or one option written as
// listed (case, quotes, backticks, bold and a trailing dot aside); any other
// text is written as the choice for its owner to refuse. A question answered
// twice the same way is answered once; two different answers are both
// written under the key, which Verdicts reads as a conflict. A question
// without a readable line stays unanswered for its owner to refuse.
func answersFromLines(asked []askedQuestion, response []byte) ([]byte, error) {
	given := make([][]string, len(asked))
	for _, line := range strings.Split(string(response), "\n") {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "-*•>#"))
		digits := 0
		for digits < len(line) && line[digits] >= '0' && line[digits] <= '9' {
			digits++
		}
		if digits == 0 || digits >= len(line) || !strings.ContainsRune(".:)", rune(line[digits])) {
			continue
		}
		n, err := strconv.Atoi(line[:digits])
		if err != nil || n < 1 || n > len(asked) {
			continue
		}
		rest := strings.TrimSpace(line[digits+1:])
		// A line may repeat the question's bracketed key; it is no answer.
		if strings.HasPrefix(rest, "[") {
			if end := strings.Index(rest, "]"); end > 0 {
				rest = strings.TrimSpace(rest[end+1:])
			}
		}
		question := asked[n-1]
		var value map[string]any
		if len(question.options) == 0 {
			switch strings.ToLower(bareAnswer(rest)) {
			case "yes":
				value = map[string]any{"type": "noul", "noul": 1}
			case "no":
				value = map[string]any{"type": "noul", "noul": 0}
			default:
				continue
			}
		} else {
			choice := matchOption(question.options, rest)
			probabilities := make(map[string]float64, len(question.options))
			for _, option := range question.options {
				probabilities[option] = 0
				if option == choice {
					probabilities[option] = 1
				}
			}
			value = map[string]any{"type": "choice", "choice": choice, "probabilities": probabilities}
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(given[n-1], string(encoded)) {
			given[n-1] = append(given[n-1], string(encoded))
		}
	}
	var answers bytes.Buffer
	answers.WriteString(`{"answers":{`)
	first := true
	for i, question := range asked {
		key, err := json.Marshal(question.key)
		if err != nil {
			return nil, err
		}
		for _, value := range given[i] {
			if !first {
				answers.WriteByte(',')
			}
			first = false
			answers.Write(key)
			answers.WriteByte(':')
			answers.WriteString(value)
		}
	}
	answers.WriteString("}}")
	return answers.Bytes(), nil
}

// bareAnswer is the answer without surrounding quotes, backticks, bold
// marks, spaces and a trailing dot.
func bareAnswer(answer string) string {
	return strings.Trim(answer, " `\"'*.")
}

// matchOption is the option the answer names, as written or bare, in any
// letter case; else the bare answer as written, for its owner to refuse.
func matchOption(options []string, answer string) string {
	bare := bareAnswer(answer)
	for _, option := range options {
		if strings.EqualFold(option, answer) || strings.EqualFold(option, bare) {
			return option
		}
	}
	return bare
}
