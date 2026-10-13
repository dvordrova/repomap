package typesafe

import (
	"bufio"
	"bytes"
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
	reader   Client
	// asked keeps each prompt's questions by its text, and each prepared
	// request's by its bytes, so Complete knows which line answers which.
	asked    sync.Map // prompt text → []askedQuestion
	prepared sync.Map // sha256 of prepared bytes → []askedQuestion
}

type askedQuestion struct {
	key     string
	options []string // none for yes/no
}

var _ llm.Categorizer = (*TextModel)(nil)

func (m *TextModel) State() []byte {
	state, _ := json.Marshal(map[string]json.RawMessage{
		"categorizer": json.RawMessage(`"text_model.lines.v1"`),
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
		if len(question.Options) == 0 {
			text.WriteString("   Answer yes or no.\n")
		} else {
			text.WriteString("   Options:\n")
			for _, option := range question.Options {
				one.options = append(one.options, option.Name)
				fmt.Fprintf(&text, "   - %s%s\n", option.Name, optionMeaning(option))
			}
		}
		asked = append(asked, one)
	}
	m.asked.Store(text.String(), asked)
	return llm.Prompt{System: textModelSystem, User: text.String(), NoResponseAdjunct: true}, nil
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
	// Keep Jev's request envelope, so windows pack as they do for Jev.
	if len(prompt.User)+1 > RequestTokenLimit {
		return llm.Prepared{}, llm.NewResourceLimitError(llm.ResourceLimitError{
			Kind: llm.ResourceLimitContextTokens, Limit: RequestTokenLimit,
			Observed: len(prompt.User) + 1, ObservedKnown: true,
			ConfiguredMaxTokens: 1, FinishReason: "jev_request_utf8_reservation",
		})
	}
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
	completion.Response, err = answersFromLines(asked.([]askedQuestion), completion.Response)
	return completion, err
}

func (m *TextModel) Verdicts(response []byte) (map[string]llm.Verdict, error) {
	return m.reader.Verdicts(response)
}

// answersFromLines reads "N. answer" lines into Jev's answers object. A line
// for no asked number is ignored; a question without a line stays unanswered.
func answersFromLines(asked []askedQuestion, response []byte) ([]byte, error) {
	answers := map[string]any{}
	scanner := bufio.NewScanner(bytes.NewReader(response))
	scanner.Buffer(make([]byte, 0, 64*1024), len(response)+1)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		digits := 0
		for digits < len(line) && line[digits] >= '0' && line[digits] <= '9' {
			digits++
		}
		if digits == 0 {
			continue
		}
		n, _ := strconv.Atoi(line[:digits])
		if n < 1 || n > len(asked) {
			continue
		}
		answer := strings.Trim(strings.TrimSpace(strings.TrimLeft(line[digits:], ".:)")), "`\"'*")
		question := asked[n-1]
		if len(question.options) == 0 {
			switch strings.ToLower(answer) {
			case "yes":
				answers[question.key] = map[string]any{"type": "noul", "noul": 1}
			case "no":
				answers[question.key] = map[string]any{"type": "noul", "noul": 0}
			}
			continue
		}
		probabilities := make(map[string]float64, len(question.options))
		for _, option := range question.options {
			probabilities[option] = 0
			if strings.EqualFold(option, answer) {
				answer, probabilities[option] = option, 1
			}
		}
		answers[question.key] = map[string]any{"type": "choice", "choice": answer, "probabilities": probabilities}
	}
	if len(answers) == 0 {
		return nil, errors.New("typesafe: text model wrote no numbered answers")
	}
	return json.Marshal(map[string]any{"answers": answers})
}
