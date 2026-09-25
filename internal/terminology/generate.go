package terminology

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/llm"
)

// Glossary work keeps complete prose windows but its own output allowance.
// A legitimate generation window produced 1,276 to 15,278 output tokens on
// the Syn/issue-bot/Watchtower series, while two Watchtower windows fell
// into a repetition loop and consumed the full 128,000-token shared
// allowance (318 s and 519 s). This ceiling bounds that loop at roughly one
// quarter of the cost; the ordinary resource-refusal split then retries the
// halves, exactly as it does today for context refusals.
// It remains a separate optional completion with independent validation.
const glossaryOutputTokens = 32768

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

// code is the exact-name set of the owning collector. It never enters the
// request; it only decides which validated terms are published.
func generationCall(items []proseSource, code map[string]CodeNameKind) (llm.Call[generationResult], error) {
	var rows []map[string]any
	for i, item := range items {
		row := fmt.Sprintf("p%d", i+1)
		rows = append(rows, map[string]any{"ref": row, "text": item.Texts})
	}
	input, err := json.Marshal(map[string]any{"prose": rows})
	if err != nil {
		return llm.Call[generationResult]{}, err
	}
	// Validation checks occurrence in the original accepted prose; the model
	// cannot supply or rewrite that prose in its glossary response.
	return llm.Call[generationResult]{
		State: []byte(`{"contract":"repomap.glossary.generate.v5"}`),
		Prompt: llm.Prompt{System: generatePrompt, User: string(input), ResponseFormatJSON: true, NoResponseAdjunct: true,
			ResponseExample: `{"terms":[{"name":"<exact name in accepted prose>","kind":"<acronym, domain, protocol or format>","explanation":"<prose-context definition>","rows":["<supporting p ref>"]}]}`},
		Limits: llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: glossaryOutputTokens},
		DecodeValidate: func(raw []byte) (generationResult, error) {
			normalized, err := llm.NormalizeJSON(raw)
			if err != nil {
				return generationResult{}, err
			}
			wire, err := decodeTerms(normalized)
			if err != nil {
				return generationResult{}, err
			}
			terms, accepted, rejections := validateGeneration(items, wire, code)
			result := generationResult{Terms: terms, Rejections: rejections}
			if len(wire) > 0 && accepted == 0 {
				return result, fmt.Errorf("glossary: no supported definitions accepted")
			}
			return result, nil
		},
	}, nil
}

// decodeTerms reads the terms list. For optional glossary work "no terms" is
// a legitimate answer: a missing or null terms member is an empty list, and a
// bare top-level array is the list itself. Any other shape is refused.
func decodeTerms(raw []byte) ([]json.RawMessage, error) {
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err == nil && list != nil {
		return list, nil
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope == nil {
		return nil, fmt.Errorf("glossary: a terms array is required")
	}
	terms, present := envelope["terms"]
	if !present {
		return []json.RawMessage{}, nil
	}
	list = nil
	if err := json.Unmarshal(terms, &list); err != nil {
		return nil, fmt.Errorf("glossary: a terms array is required")
	}
	if list == nil {
		list = []json.RawMessage{}
	}
	return list, nil
}

// A definition selects accepted prose, whose source scope remains complete.
// Neither a row-number coincidence nor a rejected neighbour supplies provenance.
// The returned count includes valid terms whose exact name the code already
// owns. Each is journaled with that name and omitted from the published terms:
// a window of nothing but code names is an accepted answer, not a refusal.
func validateGeneration(items []proseSource, wire []json.RawMessage, code map[string]CodeNameKind) ([]validatedTerm, int, []llm.ResponseRejection) {
	var rejections []llm.ResponseRejection
	rejectionByReason := make(map[string]int)
	journal := func(kind, reason, position string) {
		index, found := rejectionByReason[kind+"\x00"+reason]
		if !found {
			index = len(rejections)
			rejectionByReason[kind+"\x00"+reason] = index
			rejections = append(rejections, llm.ResponseRejection{Kind: kind, Reason: reason})
		}
		rejections[index].Count++
		if len(rejections[index].Samples) < 5 {
			rejections[index].Samples = append(rejections[index].Samples, position)
		}
	}
	reject := func(reason, position string) { journal("glossary_term_rejected", reason, position) }
	rows := make(map[string]proseSource, len(items))
	for i, item := range items {
		rows[fmt.Sprintf("p%d", i+1)] = item
	}
	var terms []validatedTerm
	accepted := 0
	for index, raw := range wire {
		position := fmt.Sprintf("terms[%d]", index)
		// Extra members are ignored, and kind is neither stored nor shown, so
		// it gates nothing except a self-declared retired identifier kind.
		var term termWire
		if err := json.Unmarshal(raw, &term); err != nil || term.Name == nil || term.Explanation == nil || term.Rows == nil {
			reject("invalid optional term shape", position)
			continue
		}
		var kind string
		if json.Unmarshal(term.Kind, &kind) == nil && strings.EqualFold(strings.TrimSpace(kind), retiredIdentifierKind) {
			reject("term declares itself an identifier, not a concept", position)
			continue
		}
		name, explanation := strings.TrimSpace(*term.Name), strings.TrimSpace(*term.Explanation)
		if name == "" || explanation == "" || explanation == "none" {
			reject("invalid optional term fields", position)
			continue
		}
		validated := validatedTerm{candidate: Candidate{Name: name, Explanation: explanation}}
		seen := make(map[string]bool)
		for _, ref := range term.Rows {
			if ref == nil {
				reject("invalid optional prose row ref", position)
				continue
			}
			item, known := rows[*ref]
			if !known {
				reject("unsupported optional prose row ref", position)
				continue
			}
			if seen[*ref] {
				continue
			}
			seen[*ref] = true
			if !slices.ContainsFunc(item.Texts, func(text string) bool { return mentionsTerm(text, name) }) {
				continue
			}
			validated.rows = append(validated.rows, *ref)
			validated.sources = append(validated.sources, item.Sources...)
		}
		if len(validated.rows) == 0 || len(validated.sources) == 0 {
			reject("term has no source-backed occurrence in the computed result", position)
			continue
		}
		accepted++
		if owner, found := code[name]; found {
			// The prompt already asks for concepts only. An exact code
			// spelling is still dropped here, visibly and by name.
			journal("glossary_code_name_omitted", fmt.Sprintf("term names a code %s: %s", owner, name), position)
			continue
		}
		sort.Strings(validated.rows)
		validated.sources = normalizeSources(validated.sources)
		terms = append(terms, validated)
	}
	return terms, accepted, rejections
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
	// Planning prepares requests only; code names never change their bytes.
	call, err := generationCall(items, nil)
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
	code := c.codeNames()
	for len(windows) > 0 {
		var calls []llm.Call[generationResult]
		for i := 0; i < len(windows); i++ {
			call, err := generationCall(windows[i], code)
			if err != nil {
				return err
			}
			refused, err := llm.RecallAdaptiveSplit(executor, provider, call)
			if err != nil {
				return err
			}
			if refused {
				if left, right, ok := splitProse(windows[i]); ok {
					windows = slices.Concat(windows[:i], [][]proseSource{left, right}, windows[i+1:])
					i-- // Rebuild complete children through the current owner.
					continue
				}
			}
			calls = append(calls, call)
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
			// A provider-local timeout can wrap DeadlineExceeded while the run
			// remains alive. It takes the same optional refusal path as other
			// exhausted provider failures; only this run's context cancels it.
			if err := ctx.Err(); err != nil {
				return err
			}
			if reductionResource(outcome.Err) {
				if left, right, ok := splitProse(windows[i]); ok {
					if _, err := llm.RememberAdaptiveSplit(executor, provider, calls[i], outcome.Outcome, outcome.Err); err != nil {
						return err
					}
					c.progress("partitioned", fmt.Sprintf("the provider refused %d prose sources in one request by resources; the complete set continues in 2 partitions", len(windows[i])))
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
