package typesafe

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
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
	_ llm.Categorizer            = (*TextModel)(nil)
	_ llm.AttemptControllerOwner = (*TextModel)(nil)
)

// AttemptController is the run's text-model gate, or nil.
func (m *TextModel) AttemptController() *llm.BatchController { return m.Controller }

func (m *TextModel) State() []byte {
	state, _ := json.Marshal(map[string]json.RawMessage{
		"categorizer": json.RawMessage(`"text_model.lines.v5"`),
		"provider":    m.Provider.State(),
	})
	return state
}

func (m *TextModel) Prompt(task string, shared map[string]any, questions map[string]llm.Question) (llm.Prompt, error) {
	keys := make([]string, 0, len(questions))
	for key := range questions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var text strings.Builder
	text.WriteString("Task:\n" + task + "\n")
	if len(shared) > 0 {
		encoded, err := json.Marshal(shared)
		if err != nil {
			return llm.Prompt{}, err
		}
		text.WriteString("\nContext:\n" + string(encoded) + "\n")
	}
	// An option list several questions share is written once, before the
	// questions, and each of them names it: the list with its meanings is
	// most of a question, and a window's questions usually share one.
	lists := make([]string, len(keys))
	uses := map[string]int{}
	for i, key := range keys {
		var list strings.Builder
		for _, option := range questions[key].Options {
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
	asked := make([]askedQuestion, 0, len(keys))
	for i, key := range keys {
		question := questions[key]
		name := question.Name
		if name == "" {
			name = "row"
		}
		item, err := json.Marshal(question.Item)
		if err != nil {
			return llm.Prompt{}, err
		}
		fmt.Fprintf(&text, "\n%d. %s\n   %s: %s\n", i+1, question.Ask, name, item)
		one := askedQuestion{key: key}
		for _, option := range question.Options {
			one.options = append(one.options, option.Name)
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
		asked = append(asked, one)
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

// answersFromLines reads the model's lines softly into Jev's answers object:
// a numbered line answers its number, an unnumbered or out-of-range one the
// next question in order; bullets, quotes and a trailing dot are dropped,
// and an option named inside a longer answer is that option. A question
// without a readable line stays unanswered for its owner to refuse.
func answersFromLines(asked []askedQuestion, response []byte) ([]byte, error) {
	answers := map[string]any{}
	next := 0
	for _, line := range strings.Split(string(response), "\n") {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "-*•>#"))
		if line == "" {
			continue
		}
		digits := 0
		for digits < len(line) && line[digits] >= '0' && line[digits] <= '9' {
			digits++
		}
		at := next
		if digits > 0 && digits < len(line) && strings.ContainsRune(".:)", rune(line[digits])) {
			if n, _ := strconv.Atoi(line[:digits]); n >= 1 && n <= len(asked) {
				at = n - 1
			}
			line = line[digits+1:]
		}
		if at >= len(asked) {
			continue
		}
		next = at + 1
		answer := strings.TrimSpace(line)
		question := asked[at]
		if len(question.options) == 0 {
			switch word := strings.ToLower(strings.Trim(answer, "`\"'*")); {
			case strings.HasPrefix(word, "yes"):
				answers[question.key] = map[string]any{"type": "noul", "noul": 1}
			case strings.HasPrefix(word, "no"):
				answers[question.key] = map[string]any{"type": "noul", "noul": 0}
			}
			continue
		}
		answer = matchOption(question.options, answer)
		probabilities := make(map[string]float64, len(question.options))
		for _, option := range question.options {
			probabilities[option] = 0
			if option == answer {
				probabilities[option] = 1
			}
		}
		answers[question.key] = map[string]any{"type": "choice", "choice": answer, "probabilities": probabilities}
	}
	return json.Marshal(map[string]any{"answers": answers})
}

// matchOption is the option the answer names: the same text in any case,
// as written or without quotes, backticks, bold and a trailing dot; else the
// longest option the answer contains; else the answer as written.
func matchOption(options []string, answer string) string {
	bare := strings.TrimRight(strings.Trim(answer, "`\"'*"), ".")
	for _, option := range options {
		if strings.EqualFold(option, answer) {
			return option
		}
	}
	for _, option := range options {
		if strings.EqualFold(option, bare) {
			return option
		}
	}
	folded := strings.ToLower(answer)
	best := ""
	for _, option := range options {
		if strings.EqualFold(option, answer) {
			return option
		}
		if option != "" && strings.Contains(folded, strings.ToLower(option)) && len(option) > len(best) {
			best = option
		}
	}
	if best != "" {
		return best
	}
	return answer
}
