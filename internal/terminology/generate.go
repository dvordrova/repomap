package terminology

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode"

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

// The glossary is made in three steps, each one decision:
//
//  1. names (text model): which names of the report's prose are glossary
//     terms, as a list of names written in the prose;
//  2. term (categorizer): for each name, one closed choice of what it is for
//     a newcomer; only a decided domain concept goes on;
//  3. explanations (text model): one definition per accepted name.
//
// Code, not a model, finds every prose row in which a name is written, folds
// the names the term lookup treats as one, and attaches each name's rows and
// their complete source scope. The model never selects rows, so a name is
// never answered once per row.
//
//go:embed prompts/names.md
var namesPrompt string

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

// NameDecision is what the glossary decided about one name the names step
// found: its categorizer decision, or undecided for a near-tie, or
// unanswered when no accepted answer reached it. Rows is how many prose rows
// write it.
type NameDecision struct {
	Name     string `json:"name"`
	Decision string `json:"decision"`
	Rows     int    `json:"rows"`
}

// The outcomes of a name that the categorizer did not decide.
const (
	NameUndecided  = "undecided"
	NameUnanswered = "unanswered"
)

// Generate explains the domain concepts of already accepted analysis prose,
// in the three steps above. All original prose is considered; disjoint
// windows own their optional results. Provider and response refusals
// preserve the main analysis and accepted siblings. program is the report's
// own summary of the program, shared by every categorizer question; it may
// be empty.
func (c *Collector) Generate(ctx context.Context, executor llm.Executor, provider llm.Provider, categorizer llm.Categorizer, program string) error {
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
	if categorizer == nil {
		return fmt.Errorf("glossary: no categorizer decides which names are terms")
	}
	code := c.codeNames()
	var found []string
	err := runWindows(ctx, executor, provider, items, c.progress, runSpec[proseSource, namesResult]{
		call:  func(window []proseSource) (llm.Call[namesResult], error) { return namesCall(window, code) },
		split: splitProse,
		unit:  "prose sources",
		accept: func(_ []proseSource, result namesResult) {
			found = append(found, result.Names...)
		},
	})
	if err != nil {
		return err
	}
	names := gatherNames(items, found)
	decisions, err := decideNames(ctx, executor, categorizer, items, names, program)
	if err != nil {
		return err
	}
	var accepted []glossaryName
	counts := make(map[string]int)
	record := make([]NameDecision, len(names))
	for i, name := range names {
		record[i] = NameDecision{Name: name.Name, Decision: decisions[i], Rows: len(name.Rows)}
		counts[decisions[i]]++
		if decisions[i] == TermDomainConcept {
			accepted = append(accepted, name)
		}
	}
	c.mu.Lock()
	c.decisions = record
	c.mu.Unlock()
	c.progress("decided", fmt.Sprintf("%d names in %d prose rows: %d domain concepts, %d general vocabulary, %d code elements, %d undecided, %d unanswered",
		len(names), len(items), counts[TermDomainConcept], counts[TermGeneralVocabulary], counts[TermCodeElement], counts[NameUndecided], counts[NameUnanswered]))
	if len(accepted) == 0 {
		return nil
	}
	return runWindows(ctx, executor, provider, accepted, c.progress, runSpec[glossaryName, explanations]{
		call:  func(window []glossaryName) (llm.Call[explanations], error) { return explainCall(items, window) },
		split: splitNames,
		unit:  "glossary terms",
		accept: func(window []glossaryName, result explanations) {
			c.acceptDefinitions(items, window, result.Explanations)
		},
	})
}

// Decisions lists what the last Generate decided about every name found,
// in name order.
func (c *Collector) Decisions() []NameDecision {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.decisions)
}

// runSpec is one text-model step over windows of units: its call, how a
// refused window splits, and what an accepted answer does.
type runSpec[U any, T any] struct {
	call   func([]U) (llm.Call[T], error)
	split  func([]U) ([]U, []U, bool)
	accept func([]U, T)
	unit   string
}

// planWindows keeps all units in one window unless the provider envelope
// refuses to prepare it; then it splits complete units.
func planWindows[U any, T any](ctx context.Context, provider llm.Provider, units []U, spec runSpec[U, T]) ([][]U, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	call, err := spec.call(units)
	if err != nil {
		return nil, err
	}
	_, err = llm.Prepare(provider, call.Prompt, call.Limits)
	if err == nil {
		return [][]U{units}, nil
	}
	if !reductionResource(err) {
		return nil, err
	}
	left, right, ok := spec.split(units)
	if !ok {
		// An indivisible original unit remains complete. The actual provider
		// envelope decides whether it fits.
		return [][]U{units}, nil
	}
	a, err := planWindows(ctx, provider, left, spec)
	if err != nil {
		return nil, err
	}
	b, err := planWindows(ctx, provider, right, spec)
	return append(a, b...), err
}

// runWindows asks one text-model step over every unit. A resource refusal
// continues the complete units in two partitions and remembers the split;
// a provider, response or validation refusal leaves that window without an
// answer and never revokes an analysis result or an accepted sibling.
func runWindows[U any, T any](ctx context.Context, executor llm.Executor, provider llm.Provider, units []U, progress func(state, detail string), spec runSpec[U, T]) error {
	windows, err := planWindows(ctx, provider, units, spec)
	if err != nil {
		return err
	}
	for len(windows) > 0 {
		var calls []llm.Call[T]
		for i := 0; i < len(windows); i++ {
			call, err := spec.call(windows[i])
			if err != nil {
				return err
			}
			refused, err := llm.RecallAdaptiveSplit(executor, provider, call)
			if err != nil {
				return err
			}
			if refused {
				if left, right, ok := spec.split(windows[i]); ok {
					windows = slices.Concat(windows[:i], [][]U{left, right}, windows[i+1:])
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
		var pending [][]U
		for i, outcome := range outcomes {
			for _, issue := range outcome.Outcome.Issues {
				if issue.Kind != llm.IssueCacheValidate && issue.Kind != llm.IssueMetrics {
					return fmt.Errorf("glossary: %w", issue)
				}
			}
			if outcome.Err == nil {
				spec.accept(windows[i], outcome.Outcome.Value)
				continue
			}
			// A provider-local timeout can wrap DeadlineExceeded while the run
			// remains alive. It takes the same optional refusal path as other
			// exhausted provider failures; only this run's context cancels it.
			if err := ctx.Err(); err != nil {
				return err
			}
			if reductionResource(outcome.Err) {
				if left, right, ok := spec.split(windows[i]); ok {
					if _, err := llm.RememberAdaptiveSplit(executor, provider, calls[i], outcome.Outcome, outcome.Err); err != nil {
						return err
					}
					if progress != nil {
						progress("partitioned", fmt.Sprintf("the provider refused %d %s in one request by resources; the complete set continues in 2 partitions", len(windows[i]), spec.unit))
					}
					for _, child := range [][]U{left, right} {
						parts, err := planWindows(ctx, provider, child, spec)
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
			// supplies nothing and never revokes an analysis result.
		}
		windows = pending
	}
	return nil
}

type namesResult struct {
	Names      []string
	Rejections []llm.ResponseRejection
}

func (result namesResult) ResponseRejections() []llm.ResponseRejection { return result.Rejections }

// proseRows is the prose catalogue of a request: one p ref per row, with
// the row's accepted texts as written.
func proseRows(items []proseSource) []map[string]any {
	rows := make([]map[string]any, 0, len(items))
	for i, item := range items {
		rows = append(rows, map[string]any{"ref": fmt.Sprintf("p%d", i+1), "text": item.Texts})
	}
	return rows
}

// namesCall asks which names of these prose rows are glossary terms. code is
// the exact-name set of the owning collector. It never enters the request;
// it only decides which found names go on.
func namesCall(items []proseSource, code map[string]CodeNameKind) (llm.Call[namesResult], error) {
	input, err := json.Marshal(map[string]any{"prose": proseRows(items)})
	if err != nil {
		return llm.Call[namesResult]{}, err
	}
	return llm.Call[namesResult]{
		State: []byte(`{"contract":"repomap.glossary.names.v1"}`),
		Prompt: llm.Prompt{System: namesPrompt, User: string(input), ResponseFormatJSON: true, NoResponseAdjunct: true,
			ResponseExample: `{"names":["<a name as the prose writes it>"]}`},
		Limits: llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: glossaryOutputTokens},
		DecodeValidate: func(raw []byte) (namesResult, error) {
			normalized, err := llm.NormalizeJSON(raw)
			if err != nil {
				return namesResult{}, err
			}
			wire, err := decodeNames(normalized)
			if err != nil {
				return namesResult{}, err
			}
			result, accepted := validateNames(items, wire, code)
			if len(wire) > 0 && accepted == 0 {
				return result, fmt.Errorf("glossary: no name of the answer is written in the prose")
			}
			return result, nil
		},
	}, nil
}

// decodeNames reads the names list. "No names" is a legitimate answer: a
// missing or null names member is an empty list, a bare top-level array is
// the list itself and one string is a list of one. Any other shape is
// refused. An entry that is an object with a name member is that name.
func decodeNames(raw []byte) ([]json.RawMessage, error) {
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err == nil && list != nil {
		return list, nil
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope == nil {
		return nil, fmt.Errorf("glossary: a names array is required")
	}
	names, present := envelope["names"]
	if !present || string(names) == "null" {
		return []json.RawMessage{}, nil
	}
	var one string
	if json.Unmarshal(names, &one) == nil {
		return []json.RawMessage{names}, nil
	}
	if err := json.Unmarshal(names, &list); err != nil {
		return nil, fmt.Errorf("glossary: a names array is required")
	}
	if list == nil {
		list = []json.RawMessage{}
	}
	return list, nil
}

// validateNames keeps each name written in this window's prose, once. The
// count includes names whose exact spelling the code already owns: each is
// journaled with that name and not kept, and a window of only code names is
// an accepted answer, not a refusal.
func validateNames(items []proseSource, wire []json.RawMessage, code map[string]CodeNameKind) (namesResult, int) {
	var result namesResult
	byReason := make(map[string]int)
	journal := func(kind, reason, position string) {
		index, found := byReason[kind+"\x00"+reason]
		if !found {
			index = len(result.Rejections)
			byReason[kind+"\x00"+reason] = index
			result.Rejections = append(result.Rejections, llm.ResponseRejection{Kind: kind, Reason: reason})
		}
		result.Rejections[index].Count++
		if len(result.Rejections[index].Samples) < 5 {
			result.Rejections[index].Samples = append(result.Rejections[index].Samples, position)
		}
	}
	seen := make(map[string]bool)
	accepted := 0
	for index, raw := range wire {
		position := fmt.Sprintf("names[%d]", index)
		var name string
		if json.Unmarshal(raw, &name) != nil {
			var object struct {
				Name *string `json:"name"`
			}
			if json.Unmarshal(raw, &object) != nil || object.Name == nil {
				journal("glossary_name_rejected", "a name is not a string", position)
				continue
			}
			name = *object.Name
		}
		name = strings.TrimSpace(name)
		if name == "" {
			journal("glossary_name_rejected", "empty name", position)
			continue
		}
		if !slices.ContainsFunc(items, func(item proseSource) bool {
			return slices.ContainsFunc(item.Texts, func(text string) bool { return mentionsTerm(text, name) })
		}) {
			journal("glossary_name_rejected", "the name is not written in the prose", position)
			continue
		}
		accepted++
		if seen[name] {
			continue // An identical repeat is one answer.
		}
		seen[name] = true
		if owner, found := code[name]; found {
			// The prompt already asks for concepts only. An exact code
			// spelling is still dropped here, visibly and by name.
			journal("glossary_code_name_omitted", fmt.Sprintf("name is a code %s: %s", owner, name), position)
			continue
		}
		result.Names = append(result.Names, name)
	}
	return result, accepted
}

// glossaryName is one found name with every prose row, by index into the
// collected rows, whose text writes it by the shared term lookup.
type glossaryName struct {
	Name string
	Rows []int
}

// gatherNames makes one term of each name the term lookup treats as one:
// spellings equal but for case, and a name with its English plural ending
// ("WAL segments" is "WAL segment"). The term keeps the spelling the prose
// writes most often, the first in order on a tie, and the rows of every
// spelling. Rows come from all collected prose, not only the window in which
// the model found the name.
func gatherNames(items []proseSource, found []string) []glossaryName {
	spellings := make(map[string][]string) // folded name -> spellings
	for _, name := range found {
		folded := FoldTerm(name)
		if !slices.Contains(spellings[folded], name) {
			spellings[folded] = append(spellings[folded], name)
		}
	}
	folds := make([]string, 0, len(spellings))
	for folded := range spellings {
		folds = append(folds, folded)
	}
	// Shorter folded names first, so a plural finds its singular.
	sort.Slice(folds, func(i, j int) bool {
		if len(folds[i]) != len(folds[j]) {
			return len(folds[i]) < len(folds[j])
		}
		return folds[i] < folds[j]
	})
	word := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) || r == '_' }
	var texts []string
	for _, item := range items {
		texts = append(texts, item.Texts...)
	}
	written := func(spelling string) int {
		count := 0
		for _, text := range texts {
			count += len(FoldText(text).FindExact(spelling, word))
		}
		return count
	}
	var bases []string
	base := make(map[string]string) // folded name -> its base's folded name
	for _, folded := range folds {
		base[folded] = folded
		for _, candidate := range bases {
			spelling := spellings[candidate][0]
			whole := spellings[folded][0]
			for _, occurrence := range FoldText(whole).Find(FoldTerm(spelling), IsAcronym(spelling), word) {
				if occurrence.Start == 0 && occurrence.End == len(whole) && occurrence.Ending > 0 {
					base[folded] = candidate
				}
			}
			if base[folded] != folded {
				break
			}
		}
		if base[folded] == folded {
			bases = append(bases, folded)
		}
	}
	var names []glossaryName
	for _, folded := range bases {
		choices := slices.Clone(spellings[folded])
		sort.Strings(choices)
		chosen, most := choices[0], -1
		for _, spelling := range choices {
			if count := written(spelling); count > most {
				chosen, most = spelling, count
			}
		}
		// validateNames kept only names the prose writes, so every one has
		// a row.
		name := glossaryName{Name: chosen}
		for i, item := range items {
			if slices.ContainsFunc(item.Texts, func(text string) bool { return mentionsTerm(text, chosen) }) {
				name.Rows = append(name.Rows, i)
			}
		}
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := FoldTerm(names[i].Name), FoldTerm(names[j].Name)
		if a != b {
			return a < b
		}
		return names[i].Name < names[j].Name
	})
	return names
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

// acceptDefinitions records one candidate per explained name: its name, its
// explanation, and the complete source scope and origin of every prose row
// that writes it. The same name with the same explanation is one definition.
func (c *Collector) acceptDefinitions(items []proseSource, names []glossaryName, explanations map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, name := range names {
		explanation, found := explanations[name.Name]
		if !found {
			continue
		}
		candidate := Candidate{Name: name.Name, Explanation: explanation}
		for _, row := range name.Rows {
			candidate.Sources = append(candidate.Sources, items[row].Sources...)
			candidate.Origins = append(candidate.Origins, items[row].Origin)
		}
		key, _ := json.Marshal([]string{candidate.Name, candidate.Explanation})
		previous := c.values[string(key)]
		candidate.Sources = normalizeSources(append(candidate.Sources, previous.Sources...))
		candidate.Origins = normalizeOrigins(append(candidate.Origins, previous.Origins...))
		c.values[string(key)] = candidate
	}
}
