package terminology

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/dvordrova/repomap/internal/llm"
)

// Glossary work has its own small output allowance and complete prose windows.
// Neither a refusal nor a runaway here can consume an analysis completion.
const glossaryOutputTokens = 8000

//go:embed prompts/generate.md
var generatePrompt string

type proseSource struct {
	Texts   []string `json:"texts"`
	Sources []Source `json:"sources"`
	Origin  Origin   `json:"-"`
}

func (c *Collector) collectProse(request string, textByRow map[string][]string, sources map[string]catalogSource, rows []string) {
	allowed := make(map[string]bool)
	for _, row := range rows {
		allowed[row] = true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for row, texts := range textByRow {
		if rows != nil && !allowed[row] {
			continue
		}
		item := proseSource{Texts: slices.Clone(texts), Origin: Origin{RequestSHA256: request, Row: row}}
		for _, source := range sources {
			if source.Row == "" || source.Row == row {
				item.Sources = append(item.Sources, Source{Path: source.Path, Line: source.Line})
			}
		}
		if len(item.Sources) == 0 || len(item.Texts) == 0 {
			continue
		}
		sort.Strings(item.Texts)
		item.Texts = slices.Compact(item.Texts)
		item.Sources = normalizeSources(item.Sources)
		key, _ := json.Marshal(item.Origin)
		c.pending[string(key)] = item
	}
}

type generationResult struct {
	Terms      []validatedTerm
	Rejections []llm.ResponseRejection
}

func (result generationResult) ResponseRejections() []llm.ResponseRejection { return result.Rejections }

func generationCall(items []proseSource) (llm.Call[generationResult], error) {
	var rows []map[string]any
	var catalogue []catalogSource
	sources := make(map[string]catalogSource)
	for i, item := range items {
		row := fmt.Sprintf("p%d", i+1)
		var refs []string
		for _, source := range item.Sources {
			ref := fmt.Sprintf("g%d", len(catalogue)+1)
			entry := catalogSource{Ref: ref, Path: source.Path, Line: source.Line, Row: row}
			catalogue = append(catalogue, entry)
			sources[ref] = entry
			refs = append(refs, ref)
		}
		rows = append(rows, map[string]any{"key": row, "text": item.Texts, "source_options": refs})
	}
	input, err := json.Marshal(map[string]any{"prose": rows, "sources": catalogue})
	if err != nil {
		return llm.Call[generationResult]{}, err
	}
	// Validation checks occurrence in the original accepted prose; the model
	// cannot supply or rewrite that prose in its glossary response.
	computed := map[string]any{"rows": rows}
	return llm.Call[generationResult]{
		State: []byte(`{"contract":"repomap.glossary.generate.v1"}`),
		Prompt: llm.Prompt{System: generatePrompt, User: string(input), ResponseFormatJSON: true, NoResponseAdjunct: true,
			ResponseExample: `{"terms":[{"name":"<exact name in accepted prose>","explanation":"<source-context definition>","sources":["<supporting g ref>"]}]}`},
		Limits: llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: glossaryOutputTokens},
		DecodeValidate: func(raw []byte) (generationResult, error) {
			normalized, err := llm.NormalizeJSON(raw)
			if err != nil {
				return generationResult{}, err
			}
			var envelope struct {
				Terms []json.RawMessage `json:"terms"`
			}
			if err := json.Unmarshal(normalized, &envelope); err != nil || envelope.Terms == nil {
				return generationResult{}, fmt.Errorf("glossary: a terms array is required")
			}
			// Use the same generic parsed shape as a provider response, including
			// array-valued strings and row ownership.
			encoded, _ := json.Marshal(computed)
			original, _ := jsonValue(encoded)
			terms, rejections := validateTermMetadata(original, sources, envelope.Terms)
			result := generationResult{Terms: terms, Rejections: rejections}
			if len(envelope.Terms) > 0 && len(terms) == 0 {
				return result, fmt.Errorf("glossary: no supported definitions accepted")
			}
			return result, nil
		},
	}, nil
}

func splitProse(items []proseSource) ([]proseSource, []proseSource, bool) {
	if len(items) == 1 {
		if len(items[0].Texts) < 2 {
			return nil, nil, false
		}
		left, right := items[0], items[0]
		middle := len(left.Texts) / 2
		left.Texts, right.Texts = left.Texts[:middle], right.Texts[middle:]
		return []proseSource{left}, []proseSource{right}, true
	}
	if len(items) < 2 {
		return nil, nil, false
	}
	weights, total := make([]int, len(items)), 0
	for i, item := range items {
		raw, _ := json.Marshal(item)
		weights[i] = len(raw)
		total += weights[i]
	}
	middle, weight := 1, weights[0]
	for middle < len(items)-1 && weight < total/2 {
		weight += weights[middle]
		middle++
	}
	return items[:middle], items[middle:], true
}

func planProse(ctx context.Context, provider llm.Provider, items []proseSource) ([][]proseSource, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	call, err := generationCall(items)
	if err != nil {
		return nil, err
	}
	_, err = llm.Prepare(provider, call.Prompt, call.Limits)
	if err == nil {
		return [][]proseSource{items}, nil
	}
	if err != nil && !reductionResource(err) {
		return nil, err
	}
	left, right, ok := splitProse(items)
	if !ok {
		// An indivisible original text remains complete. The actual provider
		// envelope decides whether it fits.
		return [][]proseSource{items}, nil
	}
	a, err := planProse(ctx, provider, left)
	if err != nil {
		return nil, err
	}
	b, err := planProse(ctx, provider, right)
	return append(a, b...), err
}

// Generate explains names in already accepted analysis prose. All original
// prose is considered; disjoint windows own their optional glossary results.
// Provider/response refusals preserve the main analysis and accepted siblings.
func (c *Collector) Generate(ctx context.Context, executor llm.Executor, provider llm.Provider) error {
	c.mu.Lock()
	keys := make([]string, 0, len(c.pending))
	for key := range c.pending {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	items := make([]proseSource, 0, len(keys))
	for _, key := range keys {
		items = append(items, c.pending[key])
	}
	c.mu.Unlock()
	if len(items) == 0 {
		return nil
	}
	windows, err := planProse(ctx, provider, items)
	if err != nil {
		return err
	}
	for len(windows) > 0 {
		calls := make([]llm.Call[generationResult], len(windows))
		for i, window := range windows {
			calls[i], err = generationCall(window)
			if err != nil {
				return err
			}
		}
		if executor.PlanNotice != nil {
			executor.PlanNotice(len(calls))
		}
		failures := make(map[string]llm.FailureKind)
		eachExecutor := executor
		eachExecutor.Observer = llm.ObserverFunc(func(event llm.Event) error {
			if event.Kind == llm.EventFailure && event.Source == llm.SourceLive {
				failures[event.RequestSHA256] = event.Failure
			}
			if executor.Observer != nil {
				return executor.Observer.Observe(event)
			}
			return nil
		})
		outcomes := llm.ExecuteJSONEach(ctx, eachExecutor, provider, calls)
		if err := ctx.Err(); err != nil {
			return err
		}
		var pending [][]proseSource
		for i, outcome := range outcomes {
			for _, issue := range outcome.Outcome.Issues {
				if issue.Kind != llm.IssueCacheValidate {
					return fmt.Errorf("glossary: %w", issue)
				}
			}
			if outcome.Err == nil {
				c.acceptDefinitions(windows[i], outcome.Outcome.Value.Terms)
				continue
			}
			if errors.Is(outcome.Err, context.Canceled) || errors.Is(outcome.Err, context.DeadlineExceeded) {
				return outcome.Err
			}
			if reductionResource(outcome.Err) {
				if left, right, ok := splitProse(windows[i]); ok {
					for _, child := range [][]proseSource{left, right} {
						parts, err := planProse(ctx, provider, child)
						if err != nil {
							return err
						}
						pending = append(pending, parts...)
					}
				}
			} else {
				switch failures[outcome.Outcome.RequestSHA256] {
				case llm.FailureProvider, llm.FailureResponse, llm.FailureValidation:
				default:
					return outcome.Err
				}
			}
			// The shared executor records the refused optional response. It
			// supplies no definitions and never revokes an analysis result.
		}
		windows = pending
	}
	return nil
}

func (c *Collector) acceptDefinitions(items []proseSource, terms []validatedTerm) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, term := range terms {
		candidate := term.candidate
		for _, source := range term.sources {
			candidate.Sources = append(candidate.Sources, Source{Path: source.Path, Line: source.Line})
		}
		for _, row := range term.rows {
			for i, item := range items {
				if row == fmt.Sprintf("p%d", i+1) {
					candidate.Origins = append(candidate.Origins, item.Origin)
				}
			}
		}
		candidate.Sources = normalizeSources(candidate.Sources)
		candidate.Origins = normalizeOrigins(candidate.Origins)
		identity := candidate
		identity.Origins = nil
		key, _ := json.Marshal(identity)
		previous := c.values[string(key)]
		candidate.Origins = normalizeOrigins(append(candidate.Origins, previous.Origins...))
		c.values[string(key)] = candidate
	}
}
