// Package typesafetest answers Jev's requests in tests. The real client
// writes every request and reads every response; only the network is
// replaced, by the test's own decision for each question.
package typesafetest

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/typesafe"
)

// Categorizer is Jev with a local answer.
type Categorizer struct {
	typesafe.Client
	// Decide answers one question by its key. A question it does not know
	// fails the whole request, so no test is answered by accident.
	Decide func(key string, question llm.Question) (llm.Verdict, bool)

	mu       sync.Mutex
	calls    int
	requests [][]byte
}

var _ llm.Categorizer = (*Categorizer)(nil)

// Requests are the request bodies, as the client wrote them.
func (c *Categorizer) Requests() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.requests)
}

// Calls is the number of requests answered.
func (c *Categorizer) Calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func (c *Categorizer) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var body struct {
		Questions map[string]struct {
			Instructions map[string]json.RawMessage `json:"instructions"`
			Criteria     map[string]json.RawMessage `json:"criteria"`
		} `json:"questions"`
	}
	if err := json.Unmarshal(prepared.Bytes(), &body); err != nil {
		return llm.Completion{}, err
	}
	c.mu.Lock()
	c.requests = append(c.requests, slices.Clone(prepared.Bytes()))
	c.mu.Unlock()
	answers := make(map[string]any, len(body.Questions))
	for key, asked := range body.Questions {
		// The instructions hold the question and one item under its name.
		var question llm.Question
		for name, raw := range asked.Instructions {
			if name == "question" {
				if err := json.Unmarshal(raw, &question.Ask); err != nil {
					return llm.Completion{}, fmt.Errorf("typesafetest: question %s: %w", key, err)
				}
				continue
			}
			if question.Item != nil {
				return llm.Completion{}, fmt.Errorf("typesafetest: question %s has two items", key)
			}
			if err := json.Unmarshal(raw, &question.Item); err != nil {
				return llm.Completion{}, fmt.Errorf("typesafetest: question %s item: %w", key, err)
			}
			if name != "row" {
				question.Name = name
			}
		}
		for name, raw := range asked.Criteria {
			option := llm.Option{Name: name}
			var meaning string
			var criteria llm.Criteria
			switch {
			case string(raw) == "null":
			case json.Unmarshal(raw, &meaning) == nil:
				option.Meaning = meaning
			case json.Unmarshal(raw, &criteria) == nil:
				option.Criteria = &criteria
			default:
				return llm.Completion{}, fmt.Errorf("typesafetest: question %s option %s has unreadable criteria", key, name)
			}
			question.Options = append(question.Options, option)
		}
		sort.Slice(question.Options, func(i, j int) bool { return question.Options[i].Name < question.Options[j].Name })
		verdict, known := llm.Verdict{}, false
		if c.Decide != nil {
			verdict, known = c.Decide(key, question)
		}
		if !known {
			return llm.Completion{}, fmt.Errorf("typesafetest: no decision for question %s", key)
		}
		if verdict.Yes != nil {
			answers[key] = map[string]any{"type": "noul", "noul": *verdict.Yes}
			continue
		}
		// Choose's certainty shorthand is completed only here, against this
		// exact prepared question. Sparse model responses are never completed
		// by the production decoder or by a made-up option inventory.
		if len(verdict.Probabilities) == 1 && verdict.Probabilities[verdict.Choice] == 1 && len(verdict.InvalidProbabilities) == 0 {
			probabilities := make(map[string]float64, len(question.Options))
			known := false
			for _, option := range question.Options {
				probabilities[option.Name] = 0
				if strings.EqualFold(strings.TrimSpace(option.Name), strings.TrimSpace(verdict.Choice)) {
					probabilities[option.Name], known = 1, true
				}
			}
			if known {
				verdict.Probabilities = probabilities
			}
		}
		answers[key] = map[string]any{"type": "choice", "choice": verdict.Choice, "probabilities": verdict.Probabilities}
	}
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	raw, err := json.Marshal(map[string]any{"answers": answers})
	return llm.Completion{Response: raw, FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, err
}

// ByColumn decides by the column a question asks about, the end of its key
// after the last "|"; a question about any other column is not known.
func ByColumn(verdicts map[string]llm.Verdict) func(string, llm.Question) (llm.Verdict, bool) {
	return func(key string, _ llm.Question) (llm.Verdict, bool) {
		verdict, known := verdicts[key[strings.LastIndex(key, "|")+1:]]
		return verdict, known
	}
}

// Yes answers a yes/no question with this probability of yes.
func Yes(p float64) llm.Verdict { return llm.Verdict{Yes: &p} }

// Choose picks one option with certainty. The local preset writes explicit
// zeroes for its actual prepared question's other options.
func Choose(option string) llm.Verdict {
	return llm.Verdict{Choice: option, Probabilities: map[string]float64{option: 1}}
}
