// Package questionbatch selects original source anchors for independent
// questions from one shared catalogue. It owns request partitions and their
// completeness; callers restore the selected refs through their current chunks.
package questionbatch

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/llm"
)

const Contract = "repomap.atlas.question-batch.v4"

//go:embed prompt.md
var systemPrompt string

//go:embed response-example.json
var responseExample string

func Prompt() string { return strings.TrimSpace(systemPrompt) }

type Input struct {
	Repository string
	Chunks     []lines.QuestionChunk
	Questions  []string
}

type Options struct {
	// MaxInputBytes is an explicit development override on system + user UTF-8
	// bytes. Zero relies only on the actual prepared provider envelope.
	MaxInputBytes int
	// MaxRows is an explicit development override on evidence rows per call.
	// Zero imposes no row count ceiling; questions still share each window.
	MaxRows int
	// System replaces this cube's static instruction for an isolated reading.
	System string
}

type Selection struct {
	Row       string   `json:"row"`
	Anchors   []string `json:"anchors"`
	Relevance string   `json:"relevance"`
	Why       string   `json:"why"`
}

type Decision struct {
	Key        string      `json:"key"`
	Selections []Selection `json:"selections"`
}

type QuestionRejection struct {
	Question string `json:"question"`
	Reason   string `json:"reason"`
	Chunks   int    `json:"chunks"`
	// Rows names the refused (question,row) cells of a question whose other
	// rows were decided. Those cells stay uninspected; empty Rows refuses the
	// whole question in this window.
	Rows []string `json:"rows,omitempty"`
	// Omitted marks a question a first-round response made no decision for:
	// named nowhere, or only without selections. That is not yet a loss: the
	// cube asks it once more over the same rows in a request without the
	// questions it did decide, and the re-asking window's own result counts.
	Omitted bool `json:"omitted,omitempty"`
	// Recovered marks an omitted question whose second round then decided
	// every row of this window.
	Recovered bool `json:"recovered,omitempty"`
}

type Response struct {
	Questions  []Decision          `json:"questions"`
	Rejections []QuestionRejection `json:"rejections,omitempty"`
	// Discarded records selections that name no readable or known row. They
	// decide and refuse nothing.
	Discarded    []QuestionRejection `json:"discarded,omitempty"`
	metadataRows []string
}

// Retrieval terms bind to evidence rows, which may be shared by questions.
// A row of a refused cell, or mentioned by a refused question, cannot
// authorize optional metadata.
func (response Response) AcceptedRowKeys() []string { return response.metadataRows }

// An omission is journaled under its own kind so rejected.jsonl does not
// count a question the second round answers as a loss.
func (response Response) ResponseRejections() []llm.ResponseRejection {
	var result []llm.ResponseRejection
	for _, rejection := range response.Rejections {
		kind := "question_rejected"
		if rejection.Omitted {
			kind = "question_omitted"
		}
		result = append(result, llm.ResponseRejection{Kind: kind, Count: rejection.Chunks, Reason: rejection.Reason, Samples: []string{rejection.Question}})
	}
	for _, discarded := range response.Discarded {
		result = append(result, llm.ResponseRejection{Kind: "selection_discarded", Count: 1, Reason: discarded.Reason, Samples: []string{discarded.Question}})
	}
	return result
}

// refusedRows returns the refused cells of one decided question.
func (response Response) refusedRows(key string) map[string]bool {
	refused := make(map[string]bool)
	for _, rejection := range response.Rejections {
		if rejection.Question == key {
			for _, row := range rejection.Rows {
				refused[row] = true
			}
		}
	}
	return refused
}

// questionRefusal refuses a response that decided no asked question. It
// keeps each question's rejection, so the questions it made no decision for
// can be asked again in a request that differs from the refused one.
type questionRefusal struct {
	rejections []QuestionRejection
}

func (refusal *questionRefusal) Error() string {
	return "question batch: no questions accepted: " + refusal.rejections[0].Reason
}

// ChunkResult has one slot per original input chunk. Only an inspected chunk
// with an empty selection means no anchors were selected. An uninspected slot
// is unavailable and must never become a negative relevance decision.
type ChunkResult struct {
	Inspected      bool
	Selections     []Selection
	Source         string
	QuestionRef    string
	RequestKey     string
	RequestSHA256  string
	ResponseSHA256 string
}

type QuestionResult struct {
	Question string
	Chunks   []ChunkResult
}

// Exchange includes failed resource attempts as operational history.
// Superseded exchanges have no semantic authority; only their later children
// can inspect chunks. QuestionIndexes refer to the original input positions.
// A Reask exchange asked only questions an earlier accepted response omitted,
// over that response's rows.
type Exchange struct {
	ChunkIndexes    []int
	QuestionIndexes []int
	QuestionRefs    []string
	Input           json.RawMessage
	System          string
	Outcome         llm.Outcome[Response]
	Err             error
	Superseded      bool
	Reused          bool
	Reask           bool
}

type Result struct {
	Questions []QuestionResult
	Exchanges []Exchange
	// Issues are memo/cache diagnostics. A stale or unreadable memo is a miss,
	// never an alternative source of accepted cells.
	Issues []error
}

type modelQuestion struct {
	Key      string `json:"key"`
	Question string `json:"question"`
}

type modelRequest struct {
	Task       string            `json:"task"`
	Repository string            `json:"repository"`
	Evidence   []json.RawMessage `json:"evidence"`
	Questions  []modelQuestion   `json:"questions"`
}

type window struct {
	rows      []int
	questions []int
	// reask marks the second round: only questions an accepted shared
	// response omitted, over its rows. A question this window omits is refused.
	reask bool
}

func (part window) with(rows, questions []int) window {
	return window{rows: rows, questions: questions, reask: part.reask}
}

// maxQuestionsPerWindow bounds the independent decisions one request asks
// for. Freqtrade 20260910-144751, atlas_question window w1: 64 questions ×
// 460 code rows × 4,661 advertised anchors (2.14 MB) came back with q1, q2,
// q3 and q64 only at finish=stop (33,963 output tokens, about 31,000 of them
// reasoning); the decoder refused 60 questions and 60 × 460 = 27,600 cells
// stayed unavailable while the answer stage worked on its fallback. Window
// w2 of the same run (64 questions × 1,241 document rows × 2,575 anchors)
// answered 64/64, and w1 with reasoning off enumerated anchors for q3
// (a3 … a31892 …) up to the 128,000-token ceiling: the cause is the number
// of decisions per answer, not the keys. Probes on the saved w1 with
// reasoning on: 8 questions × all 460 rows gave 8/8 answers, 97 selections,
// 48 s, 9,970 output tokens (4,745 reasoning), finish=stop; 8 questions ×
// the rows up to 3,000 anchors (318 rows) gave 8/8, 76 selections, 64 s.
// The question ceiling alone repairs the window, so no anchor ceiling is
// imposed: none was measured to be needed.
const maxQuestionsPerWindow = 8

type catalogue struct {
	input          Input
	opts           Options
	rows           []json.RawMessage
	anchorOptions  [][]string
	rowByRef       map[string]int
	questions      []modelQuestion
	questionInputs []int
	inputQuestions []int
	evidenceSHA    string
}

// Run uses the shared executor's isolation for independent windows. A refused
// window leaves its question/chunk cells unavailable; accepted neighbours
// survive. Provider resource failures alone allow lossless repartitioning.
// Invalid preparation and cancellation remain errors for the caller.
func Run(ctx context.Context, executor llm.Executor, provider llm.Provider, input Input, opts Options) (Result, error) {
	data, err := prepareCatalogue(input, opts)
	if err != nil {
		return Result{}, err
	}
	result := Result{Questions: make([]QuestionResult, len(data.questions))}
	for i, question := range data.questions {
		result.Questions[i] = QuestionResult{Question: question.Question, Chunks: make([]ChunkResult, len(input.Chunks))}
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if len(data.questions) == 0 || len(data.rows) == 0 {
		return data.expand(result), nil
	}
	if provider == nil {
		return Result{}, fmt.Errorf("question batch: provider is required")
	}
	keys, remembered, err := data.recall(ctx, executor, provider, &result)
	if err != nil {
		return Result{}, err
	}
	rounds := &execution{data: data, executor: executor, provider: provider, result: &result, remembered: remembered, warm: make(map[string]bool)}
	var planned []window
	for _, missing := range data.missing(result) {
		parts, err := data.plan(ctx, provider, missing)
		if err != nil {
			return Result{}, err
		}
		planned = append(planned, parts...)
	}
	if executor.PlanNotice != nil {
		executor.PlanNotice(len(planned))
	}
	first := len(result.Exchanges)
	if err := rounds.run(ctx, planned); err != nil {
		return Result{}, err
	}
	// An accepted response that named only some of its questions decided
	// nothing about the others. Ask those once more over the same rows, without
	// the questions the model did answer; a question omitted twice stays refused.
	planned = nil
	for _, omitted := range data.omittedWindows(result.Exchanges[first:]) {
		parts, err := data.plan(ctx, provider, omitted)
		if err != nil {
			return Result{}, err
		}
		planned = append(planned, parts...)
	}
	if len(planned) > 0 {
		if executor.PlanNotice != nil {
			executor.PlanNotice(len(planned))
		}
		if err := rounds.run(ctx, planned); err != nil {
			return Result{}, err
		}
	}
	data.markRecovered(&result, first)
	data.remember(executor, keys, remembered, &result)
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	return data.expand(result), nil
}

// execution carries one reading's state across rounds: accepted decisions
// land in result, an accepted question remembers its window, and warm names
// the row sets whose request prefix the provider has already processed.
type execution struct {
	data       catalogue
	executor   llm.Executor
	provider   llm.Provider
	result     *Result
	remembered [][]rememberedWindow
	warm       map[string]bool
}

// run executes windows until every resource refusal has been repartitioned.
func (e *execution) run(ctx context.Context, planned []window) error {
	for len(planned) > 0 {
		var next []window
		for _, wave := range e.waves(planned) {
			children, err := e.execute(ctx, wave)
			if err != nil {
				return err
			}
			next = append(next, children...)
		}
		planned = next
	}
	return ctx.Err()
}

// waves orders one round for the provider's prompt cache. Windows over the
// same rows share their system + evidence prefix (the questions come last),
// and DeepSeek serves that prefix from its cache only once a request carrying
// it has completed: in the probes of the saved Freqtrade w1 the windows ran
// in parallel and prompt_cache_hit_tokens was 1,408 of 516,842, so eight
// windows cost eight times the input of one. The first window of each row
// set not yet seen by the provider goes alone; its siblings and every window
// over already-processed rows follow together, in parallel as before.
func (e *execution) waves(planned []window) [][]window {
	var leads, rest []window
	led := make(map[string]bool)
	for _, part := range planned {
		key := rowsKey(part.rows)
		if e.warm[key] || led[key] {
			rest = append(rest, part)
			continue
		}
		led[key] = true
		leads = append(leads, part)
	}
	var waves [][]window
	for _, wave := range [][]window{leads, rest} {
		if len(wave) > 0 {
			waves = append(waves, wave)
		}
	}
	return waves
}

// execute runs one wave and returns the complete partitions that replace
// its resource refusals.
func (e *execution) execute(ctx context.Context, wave []window) ([]window, error) {
	data := e.data
	calls := make([]llm.Call[Response], len(wave))
	for i, part := range wave {
		var err error
		if calls[i], err = data.call(part); err != nil {
			return nil, err
		}
	}
	outcomes := llm.ExecuteJSONEach(ctx, e.executor, e.provider, calls)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var next []window
	for i, outcome := range outcomes {
		part := wave[i]
		if !outcome.Outcome.Cached && outcome.Outcome.RequestBytes > 0 {
			e.warm[rowsKey(part.rows)] = true
		}
		exchange := Exchange{ChunkIndexes: append([]int(nil), part.rows...), Reask: part.reask, Outcome: outcome.Outcome, Err: outcome.Err,
			Input: json.RawMessage(calls[i].Prompt.User), System: calls[i].Prompt.System}
		for _, q := range part.questions {
			exchange.QuestionIndexes = append(exchange.QuestionIndexes, data.questionInputs[q])
			exchange.QuestionRefs = append(exchange.QuestionRefs, data.questions[q].Key)
		}
		if outcome.Err == nil {
			for _, q := range part.questions {
				accepted := data.apply(&e.result.Questions[q], part.rows, data.questions[q].Key, outcome.Outcome)
				if accepted && outcome.Outcome.CacheKey != "" {
					e.remembered[q] = append(e.remembered[q], data.reference(part, q, outcome.Outcome.CacheKey))
				}
			}
		} else if splittableResource(outcome.Err) {
			if left, right, ok := data.splitResource(part, outcome.Err); ok {
				exchange.Superseded = true
				for _, child := range []window{left, right} {
					parts, err := data.plan(ctx, e.provider, child)
					if err != nil {
						return nil, err
					}
					next = append(next, parts...)
				}
			}
		}
		e.result.Exchanges = append(e.result.Exchanges, exchange)
	}
	return next, nil
}

func prepareCatalogue(input Input, opts Options) (catalogue, error) {
	data := catalogue{input: input, opts: opts, rowByRef: make(map[string]int)}
	if opts.MaxInputBytes < 0 || opts.MaxRows < 0 {
		return data, fmt.Errorf("question batch: input and row budgets must be nonnegative")
	}
	if opts.System == "" {
		data.opts.System = Prompt()
	}
	data.rows = make([]json.RawMessage, len(input.Chunks))
	data.anchorOptions = make([][]string, len(input.Chunks))
	for i, chunk := range input.Chunks {
		key := rowRef(i)
		var encoded bytes.Buffer
		encoded.WriteString(`{"key":`)
		ref, _ := json.Marshal(key)
		encoded.Write(ref)
		seen := map[string]bool{"key": true}
		var options []string
		for _, field := range chunk.Row.Fields {
			if field.Name == "" || seen[field.Name] {
				return data, fmt.Errorf("question batch: duplicate or empty row field at %s", key)
			}
			seen[field.Name] = true
			name, _ := json.Marshal(field.Name)
			value, err := json.Marshal(field.Value)
			if err != nil {
				return data, fmt.Errorf("question batch: encode %s: %w", key, err)
			}
			if field.Name == "anchor_options" {
				if err := json.Unmarshal(value, &options); err != nil {
					return data, fmt.Errorf("question batch: invalid anchor options at %s", key)
				}
			}
			encoded.WriteByte(',')
			encoded.Write(name)
			encoded.WriteByte(':')
			encoded.Write(value)
		}
		if options == nil {
			return data, fmt.Errorf("question batch: missing anchor options at %s", key)
		}
		for _, ref := range options {
			if _, known := chunk.Anchors[ref]; !known {
				return data, fmt.Errorf("question batch: advertised anchor has no source at %s", key)
			}
		}
		encoded.WriteByte('}')
		data.rows[i] = encoded.Bytes()
		data.anchorOptions[i] = options
		data.rowByRef[key] = i
	}
	known := make(map[string]int)
	for i, question := range input.Questions {
		if strings.TrimSpace(question) == "" {
			return data, fmt.Errorf("question batch: question %d is empty", i+1)
		}
		q, exists := known[question]
		if !exists {
			q = len(data.questions)
			known[question] = q
			data.questions = append(data.questions, modelQuestion{Key: "q" + strconv.Itoa(q+1), Question: question})
			data.questionInputs = append(data.questionInputs, i)
		}
		data.inputQuestions = append(data.inputQuestions, q)
	}
	encoded, err := json.Marshal(data.rows)
	if err != nil {
		return data, err
	}
	data.evidenceSHA = digest(encoded)
	return data, nil
}

func (data catalogue) request(rows []int, questions []modelQuestion) modelRequest {
	request := modelRequest{Task: Contract, Repository: data.input.Repository, Questions: questions}
	for _, row := range rows {
		request.Evidence = append(request.Evidence, data.rows[row])
	}
	return request
}

func (data catalogue) windowQuestions(part window) []modelQuestion {
	questions := make([]modelQuestion, len(part.questions))
	for i, q := range part.questions {
		questions[i] = data.questions[q]
	}
	return questions
}

func (data catalogue) call(part window) (llm.Call[Response], error) {
	return data.requestCall(part.rows, data.windowQuestions(part), part.reask)
}

// requestCall prepares one window. The request bytes do not depend on reask;
// only the decoder's diagnostics do, so a first-round omission is re-asked
// while a re-asking window's omission is refused.
func (data catalogue) requestCall(rows []int, questions []modelQuestion, reask bool) (llm.Call[Response], error) {
	encoded, err := json.Marshal(data.request(rows, questions))
	if err != nil {
		return llm.Call[Response]{}, fmt.Errorf("question batch: encode request: %w", err)
	}
	return llm.Call[Response]{
		State:          []byte(`{"contract":"` + Contract + `"}`),
		Prompt:         llm.Prompt{System: data.opts.System, User: string(encoded), ResponseFormatJSON: true, ResponseExample: responseExample, Reasoning: true, ProseFields: []string{"questions[].selections[].why"}},
		Limits:         limits(),
		DecodeValidate: func(raw []byte) (Response, error) { return data.decode(rows, questions, raw, reask) },
	}, nil
}

func limits() llm.Limits {
	// Reasoning shares the output allowance with the source selections. Short
	// visible answers do not justify a smaller reservation for a complete
	// evidence catalogue. The provider still applies its configured ceiling.
	return llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: llm.DefaultMaxOutputTokens}
}

func (data catalogue) plan(ctx context.Context, provider llm.Provider, part window) ([]window, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if maximum := data.opts.MaxRows; maximum > 0 && len(part.rows) > maximum {
		var planned []window
		for start := 0; start < len(part.rows); start += maximum {
			children, err := data.plan(ctx, provider, part.with(part.rows[start:min(start+maximum, len(part.rows))], part.questions))
			if err != nil {
				return nil, err
			}
			planned = append(planned, children...)
		}
		return planned, nil
	}
	// Every question group reads the same complete rows; only the decisions
	// asked of one answer are fewer.
	if len(part.questions) > maxQuestionsPerWindow {
		var planned []window
		for start := 0; start < len(part.questions); start += maxQuestionsPerWindow {
			children, err := data.plan(ctx, provider, part.with(part.rows, part.questions[start:min(start+maxQuestionsPerWindow, len(part.questions))]))
			if err != nil {
				return nil, err
			}
			planned = append(planned, children...)
		}
		return planned, nil
	}
	call, err := data.call(part)
	if err != nil {
		return nil, err
	}
	var tooLarge error
	if limit := data.opts.MaxInputBytes; limit > 0 && len(call.Prompt.System)+len(call.Prompt.User) > limit {
		tooLarge = llm.NewResourceLimitError(llm.ResourceLimitError{Stage: lines.StageQuestion, Kind: llm.ResourceLimitRequestBytes,
			Limit: limit, Observed: len(call.Prompt.System) + len(call.Prompt.User), ObservedKnown: true})
	} else {
		prepared, err := llm.Prepare(provider, call.Prompt, call.Limits)
		if err != nil {
			if !splittableResource(err) {
				return nil, fmt.Errorf("question batch: provider preparation: %w", err)
			}
			tooLarge = err
		} else if prepared.Len() > call.Limits.MaxRequestBytes {
			tooLarge = llm.NewResourceLimitError(llm.ResourceLimitError{Stage: lines.StageQuestion, Kind: llm.ResourceLimitRequestBytes,
				Limit: call.Limits.MaxRequestBytes, Observed: prepared.Len(), ObservedKnown: true})
		}
	}
	if tooLarge == nil {
		return []window{part}, nil
	}
	left, right, ok := data.split(part)
	if !ok {
		return nil, fmt.Errorf("question batch: complete evidence/question singleton cannot fit: %w", tooLarge)
	}
	first, err := data.plan(ctx, provider, left)
	if err != nil {
		return nil, err
	}
	second, err := data.plan(ctx, provider, right)
	return append(first, second...), err
}

// Output limits constrain the decisions for independent questions. Reduce that
// dimension first while each child retains the complete source catalogue.
// Input/context limits still follow the prepared input's relative byte weight.
func (data catalogue) splitResource(part window, err error) (window, window, bool) {
	var resourceErr *llm.ResourceLimitError
	if errors.As(err, &resourceErr) && len(part.questions) > 1 &&
		(resourceErr.Kind == llm.ResourceLimitOutputTokens || resourceErr.Kind == llm.ResourceLimitResponseBytes) {
		weights := make([]int, len(part.questions))
		for i, q := range part.questions {
			encoded, _ := json.Marshal(data.questions[q])
			weights[i] = len(encoded)
		}
		at := balanced(weights)
		return part.with(part.rows, part.questions[:at]), part.with(part.rows, part.questions[at:]), true
	}
	return data.split(part)
}

func (data catalogue) split(part window) (window, window, bool) {
	rowWeights := make([]int, len(part.rows))
	questionWeights := make([]int, len(part.questions))
	rowBytes, questionBytes := 0, 0
	for i, row := range part.rows {
		rowWeights[i] = len(data.rows[row])
		rowBytes += rowWeights[i]
	}
	for i, q := range part.questions {
		encoded, _ := json.Marshal(data.questions[q])
		questionWeights[i] = len(encoded)
		questionBytes += questionWeights[i]
	}
	if len(part.rows) > 1 && (len(part.questions) <= 1 || rowBytes >= questionBytes) {
		at := balanced(rowWeights)
		return part.with(part.rows[:at], part.questions), part.with(part.rows[at:], part.questions), true
	}
	if len(part.questions) > 1 {
		at := balanced(questionWeights)
		return part.with(part.rows, part.questions[:at]), part.with(part.rows, part.questions[at:]), true
	}
	return window{}, window{}, false
}

func balanced(weights []int) int {
	total := 0
	for _, weight := range weights {
		total += weight
	}
	best, distance, prefix := 1, total, 0
	for i, weight := range weights[:len(weights)-1] {
		prefix += weight
		delta := prefix - (total - prefix)
		if delta < 0 {
			delta = -delta
		}
		if delta < distance {
			best, distance = i+1, delta
		}
	}
	return best
}

func splittableResource(err error) bool {
	var resourceErr *llm.ResourceLimitError
	if !errors.As(err, &resourceErr) {
		return false
	}
	switch resourceErr.Kind {
	case llm.ResourceLimitRequestBytes, llm.ResourceLimitResponseBytes, llm.ResourceLimitOutputTokens, llm.ResourceLimitContextTokens:
		return true
	default:
		return false
	}
}

// decode reads each asked question independently, and within a question each
// (question,row) cell. A defect refuses the smallest scope it touches: an
// unknown entry or row is discarded, a malformed or conflicting cell stays
// uninspected, and the question's other rows and the other questions keep
// their decisions.
func (data catalogue) decode(rows []int, questions []modelQuestion, raw []byte, reask bool) (Response, error) {
	normalized, err := llm.NormalizeJSON(raw)
	if err != nil {
		return Response{}, err
	}
	entries, err := questionEntriesOf(normalized)
	if err != nil {
		return Response{}, err
	}
	allowed := make(map[string]bool, len(questions))
	for _, question := range questions {
		allowed[question.Key] = true
	}
	byQuestion := make(map[string][]json.RawMessage)
	var unknown []json.RawMessage
	for _, entry := range entries {
		var named struct {
			Key string `json:"key"`
		}
		if json.Unmarshal(entry, &named) != nil || !allowed[named.Key] {
			unknown = append(unknown, entry)
			continue
		}
		byQuestion[named.Key] = append(byQuestion[named.Key], entry)
	}
	// A missing question says what the response did contain, so the
	// journal and the console show the provider's shape, not only the gap.
	shape := responseShape(unknown)
	result := Response{Questions: []Decision{}}
	unsafe := append([]json.RawMessage(nil), unknown...)
	refusedCells := make(map[string]bool)
	for _, question := range questions {
		reading := data.readQuestion(rows, question.Key, byQuestion[question.Key])
		result.Discarded = append(result.Discarded, reading.discarded...)
		if reading.decided {
			result.Questions = append(result.Questions, reading.decision)
			if len(reading.refused) > 0 {
				result.Rejections = append(result.Rejections, QuestionRejection{Question: question.Key, Reason: reading.reason, Chunks: len(reading.refused), Rows: reading.refused})
				for _, row := range reading.refused {
					refusedCells[row] = true
				}
			}
			continue
		}
		reason, omitted := reading.reason, false
		if reading.undecided {
			// No decision was made. The first round asks it again; a
			// re-asking window's gap is final.
			omitted = !reask
			if len(byQuestion[question.Key]) == 0 && shape != "" {
				reason += " (" + shape + ")"
			}
		}
		result.Rejections = append(result.Rejections, QuestionRejection{Question: question.Key, Reason: reason, Chunks: len(rows), Omitted: omitted})
		unsafe = append(unsafe, byQuestion[question.Key]...)
	}
	if len(result.Questions) == 0 {
		return result, &questionRefusal{rejections: result.Rejections}
	}
	if len(result.Rejections) > 0 || len(unknown) > 0 {
		result.metadataRows = safeQuestionMetadataRows(rows, unsafe, refusedCells)
	}
	return result, nil
}

// questionReading is one question's decision in one response.
type questionReading struct {
	decision Decision
	// decided is true when at least one row of the window was decided.
	decided bool
	// refused lists the refused cells of a decided question, in row order.
	refused []string
	reason  string
	// undecided marks a question named nowhere or only without selections.
	undecided bool
	discarded []QuestionRejection
}

// questionCells is one entry's reading of a question: each row's selected
// anchors with their relevance, their hints, and the rows it refused.
type questionCells struct {
	relevance map[string]map[string]string
	hints     map[string]map[string]bool
	refused   map[string]string
}

// readQuestion combines the entries of one question cell by cell. The same
// answer given twice is one answer; a row the entries answer differently,
// including a selection against an explicit empty list, is refused alone.
func (data catalogue) readQuestion(rows []int, key string, entries []json.RawMessage) questionReading {
	var reading questionReading
	if len(entries) == 0 {
		reading.undecided = true
		reading.reason = fmt.Sprintf("question batch: missing question %s", key)
		return reading
	}
	allowedRows := make(map[string]int, len(rows))
	for _, row := range rows {
		allowedRows[rowRef(row)] = row
	}
	var readings []questionCells
	for _, entry := range entries {
		var wire struct {
			Selections json.RawMessage `json:"selections"`
		}
		_ = json.Unmarshal(entry, &wire)
		if len(wire.Selections) == 0 || bytes.Equal(wire.Selections, []byte("null")) {
			continue // an entry without selections made no decision
		}
		var selections []json.RawMessage
		if json.Unmarshal(wire.Selections, &selections) != nil {
			// No cell of this entry can be read or compared, so no other
			// entry for the question may stand in for it.
			return questionReading{reason: fmt.Sprintf("question batch: question %s has an invalid selection shape", key)}
		}
		cells, discarded := data.readCells(allowedRows, key, selections)
		readings = append(readings, cells)
		reading.discarded = append(reading.discarded, discarded...)
	}
	if len(readings) == 0 {
		reading.undecided = true
		reading.reason = fmt.Sprintf("question batch: missing selections for %s", key)
		return reading
	}
	reading.decision = Decision{Key: key, Selections: []Selection{}}
	var reasons []string
	for _, row := range rows {
		ref := rowRef(row)
		reason := ""
		for _, cells := range readings {
			if refused, ok := cells.refused[ref]; ok {
				reason = refused
				break
			}
		}
		for _, other := range readings[1:] {
			if reason == "" && !maps.Equal(readings[0].relevance[ref], other.relevance[ref]) {
				reason = fmt.Sprintf("question batch: conflicting answers for %s/%s", key, ref)
			}
		}
		if reason != "" {
			reading.refused = append(reading.refused, ref)
			reasons = append(reasons, reason)
			continue
		}
		reading.decided = true
		for _, anchor := range data.anchorOptions[row] {
			relevance, selected := readings[0].relevance[ref][anchor]
			if !selected {
				continue
			}
			// Different explanations of the same relevance decision are
			// independent hints. Preserve every original hint in stable order;
			// neither the first nor the last response row wins.
			seen := make(map[string]bool)
			var hints []string
			for _, cells := range readings {
				for hint := range cells.hints[ref+"/"+anchor] {
					if !seen[hint] {
						seen[hint] = true
						hints = append(hints, hint)
					}
				}
			}
			sort.Strings(hints)
			reading.decision.Selections = append(reading.decision.Selections, Selection{Row: ref, Anchors: []string{anchor}, Relevance: relevance, Why: strings.Join(hints, "\n\n")})
		}
	}
	if len(reasons) > 0 {
		reading.reason = reasons[0]
		if len(reasons) > 1 {
			reading.reason += fmt.Sprintf("; %d cells refused", len(reasons))
		}
	}
	if !reading.decided {
		reading.refused = nil
	}
	return reading
}

// readCells reads one entry's selections field by field, so a malformed
// selection refuses only its own (question,row) cell. A selection without a
// readable known row is discarded and recorded.
func (data catalogue) readCells(allowedRows map[string]int, key string, selections []json.RawMessage) (questionCells, []QuestionRejection) {
	cells := questionCells{relevance: make(map[string]map[string]string), hints: make(map[string]map[string]bool), refused: make(map[string]string)}
	var discarded []QuestionRejection
	refuse := func(ref, reason string) {
		if _, done := cells.refused[ref]; !done {
			cells.refused[ref] = reason
		}
	}
	for _, raw := range selections {
		var wire struct {
			Row       json.RawMessage `json:"row"`
			Anchors   json.RawMessage `json:"anchors"`
			Relevance json.RawMessage `json:"relevance"`
			Why       json.RawMessage `json:"why"`
		}
		var ref string
		if json.Unmarshal(raw, &wire) != nil || json.Unmarshal(wire.Row, &ref) != nil || ref == "" {
			// Discarding a selection that still names a known row would turn
			// the model's positive into a negative: that cell is refused.
			named := knownRowsNamed(raw, allowedRows)
			for _, row := range named {
				refuse(row, fmt.Sprintf("question batch: invalid selection shape for %s/%s", key, row))
			}
			if len(named) == 0 {
				discarded = append(discarded, QuestionRejection{Question: key, Reason: fmt.Sprintf("question batch: a selection of %s has no readable row", key)})
			}
			continue
		}
		row, known := allowedRows[ref]
		if !known {
			discarded = append(discarded, QuestionRejection{Question: key, Reason: fmt.Sprintf("question batch: a selection of %s names an unknown row", key)})
			continue
		}
		anchors, anchorsOK := selectionAnchors(wire.Anchors)
		var relevance, why string
		relevanceOK := json.Unmarshal(wire.Relevance, &relevance) == nil
		whyOK := len(wire.Why) == 0 || bytes.Equal(wire.Why, []byte("null")) || json.Unmarshal(wire.Why, &why) == nil
		if !anchorsOK || !relevanceOK || !whyOK {
			refuse(ref, fmt.Sprintf("question batch: invalid selection shape for %s/%s", key, ref))
			continue
		}
		advertised := make(map[string]bool, len(data.anchorOptions[row]))
		for _, anchor := range data.anchorOptions[row] {
			advertised[anchor] = true
		}
		var selected, unknownAnchors []string
		for _, anchor := range anchors {
			if advertised[anchor] {
				selected = append(selected, anchor)
			} else {
				unknownAnchors = append(unknownAnchors, anchor)
			}
		}
		if len(selected) == 0 {
			refuse(ref, fmt.Sprintf("question batch: positive selection has no advertised anchors for %s/%s %q", key, ref, unknownAnchors))
			continue
		}
		relevance = strings.ToLower(strings.TrimSpace(relevance))
		if relevance != "direct" && relevance != "context" {
			refuse(ref, fmt.Sprintf("question batch: invalid relevance for %s/%s", key, ref))
			continue
		}
		if cells.relevance[ref] == nil {
			cells.relevance[ref] = make(map[string]string)
		}
		// Relevance belongs to the selected source, not the arbitrary file
		// chunk containing it. Preserve each hint only at its own anchors.
		for _, anchor := range selected {
			if previous, seen := cells.relevance[ref][anchor]; seen && previous != relevance {
				refuse(ref, fmt.Sprintf("question batch: conflicting selection for %s/%s/%s", key, ref, anchor))
			}
			cells.relevance[ref][anchor] = relevance
			if hint := ref + "/" + anchor; strings.TrimSpace(why) != "" {
				if cells.hints[hint] == nil {
					cells.hints[hint] = make(map[string]bool)
				}
				cells.hints[hint][why] = true
			}
		}
	}
	return cells, discarded
}

// knownRowsNamed returns the known row refs a selection without a readable
// row still names in any of its strings: written as the selection itself, as
// a list, or under another field.
func knownRowsNamed(raw json.RawMessage, allowedRows map[string]int) []string {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	seen := make(map[string]bool)
	var named []string
	var visit func(any)
	visit = func(value any) {
		switch value := value.(type) {
		case string:
			if _, known := allowedRows[value]; known && !seen[value] {
				seen[value] = true
				named = append(named, value)
			}
		case []any:
			for _, child := range value {
				visit(child)
			}
		case map[string]any:
			for _, child := range value {
				visit(child)
			}
		}
	}
	visit(value)
	return named
}

// selectionAnchors reads a list of anchor refs; one ref written as a single
// string is the same list.
func selectionAnchors(raw json.RawMessage) ([]string, bool) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, true
	}
	var anchors []string
	if json.Unmarshal(raw, &anchors) == nil {
		return anchors, true
	}
	var anchor string
	if json.Unmarshal(raw, &anchor) == nil {
		return []string{anchor}, true
	}
	return nil, false
}

// questionEntriesOf reads the `questions` array of a response. A bare array of
// entries, or a root whose only field is an object holding `questions`, is the
// same answer in another form; missing or null questions are no answer.
func questionEntriesOf(normalized []byte) ([]json.RawMessage, error) {
	var entries []json.RawMessage
	if json.Unmarshal(normalized, &entries) == nil {
		return entries, nil
	}
	var envelope struct {
		Questions []json.RawMessage `json:"questions"`
	}
	if json.Unmarshal(normalized, &envelope) == nil && envelope.Questions != nil {
		return envelope.Questions, nil
	}
	var wrapper map[string]json.RawMessage
	if json.Unmarshal(normalized, &wrapper) == nil && len(wrapper) == 1 {
		for _, inner := range wrapper {
			if json.Unmarshal(inner, &envelope) == nil && envelope.Questions != nil {
				return envelope.Questions, nil
			}
		}
	}
	return nil, fmt.Errorf("question batch: response must contain a questions array")
}

// responseShape describes entries that named no asked question: the field
// names they carry and the values under key-like fields. A missing
// question then says what the response held, not only that it lacked one:
// the Freqtrade run that asked 64 questions in one window got q1, q2, q3
// and q64 back with the right keys and nothing else.
func responseShape(unknown []json.RawMessage) string {
	if len(unknown) == 0 {
		return ""
	}
	names := map[string]bool{}
	var values []string
	for _, raw := range unknown {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil {
			continue
		}
		for name := range fields {
			names[name] = true
		}
		for _, name := range []string{"key", "question", "question_key", "question_id", "ref", "id", "q"} {
			var value string
			if json.Unmarshal(fields[name], &value) == nil && len(values) < 4 {
				values = append(values, name+"="+value)
			}
		}
	}
	var fieldNames []string
	for name := range names {
		fieldNames = append(fieldNames, name)
	}
	sort.Strings(fieldNames)
	shape := fmt.Sprintf("%d unmatched entries with fields %v", len(unknown), fieldNames)
	if len(values) > 0 {
		shape += fmt.Sprintf(", named %v", values)
	}
	return shape
}

// The glossary observes the exact raw result. Suppress source-row metadata
// of a refused cell or touched by a refused question, even if an accepted
// question shares that row.
func safeQuestionMetadataRows(rows []int, refused []json.RawMessage, refusedCells map[string]bool) []string {
	blocked := make(map[string]bool, len(refusedCells))
	for row := range refusedCells {
		blocked[row] = true
	}
	var visit func(any)
	visit = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			for _, field := range []string{"row", "key"} {
				if ref, ok := value[field].(string); ok {
					blocked[ref] = true
				}
			}
			for _, child := range value {
				visit(child)
			}
		case []any:
			for _, child := range value {
				visit(child)
			}
		}
	}
	for _, raw := range refused {
		var value any
		if json.Unmarshal(raw, &value) == nil {
			visit(value)
		}
	}
	accepted := make([]string, 0, len(rows))
	for _, row := range rows {
		if !blocked[rowRef(row)] {
			accepted = append(accepted, rowRef(row))
		}
	}
	return accepted
}

// apply records one window's decided cells of a question. Its refused cells
// stay uninspected, never a negative. A window never overwrites a cell another
// window already decided: a later window over a refused cell is its own
// decision, and two decisions for one cell leave the later window unused.
func (data catalogue) apply(question *QuestionResult, rows []int, ref string, outcome llm.Outcome[Response]) bool {
	accepted := false
	byRow := make(map[string][]Selection)
	for _, decision := range outcome.Value.Questions {
		if decision.Key == ref {
			accepted = true
			for _, selection := range decision.Selections {
				byRow[selection.Row] = append(byRow[selection.Row], selection)
			}
		}
	}
	if !accepted {
		return false
	}
	refused := outcome.Value.refusedRows(ref)
	for _, row := range rows {
		if !refused[rowRef(row)] && question.Chunks[row].Inspected {
			return false
		}
	}
	source := atlas.SourceModel
	if outcome.Cached {
		source = atlas.SourceCache
	}
	for _, row := range rows {
		if refused[rowRef(row)] {
			continue
		}
		question.Chunks[row] = ChunkResult{
			Inspected: true, Selections: cloneSelections(byRow[rowRef(row)]),
			Source: source, QuestionRef: ref, RequestKey: outcome.CacheKey, RequestSHA256: outcome.RequestSHA256, ResponseSHA256: outcome.ResponseSHA256,
		}
	}
	return true
}

func (data catalogue) missing(result Result) []window {
	byRows := make(map[string]int)
	var windows []window
	for q, question := range result.Questions {
		var rows []int
		for i, chunk := range question.Chunks {
			if !chunk.Inspected {
				rows = append(rows, i)
			}
		}
		if len(rows) == 0 {
			continue
		}
		key := rowsKey(rows)
		at, found := byRows[key]
		if !found {
			at = len(windows)
			byRows[key] = at
			windows = append(windows, window{rows: rows})
		}
		windows[at].questions = append(windows[at].questions, q)
	}
	return windows
}

// omittedWindows plans the second round: for every first-round response,
// its rows with only the questions it made no decision for. A response that
// decided no question still names them, unless re-asking them all would
// repeat its request.
func (data catalogue) omittedWindows(exchanges []Exchange) []window {
	byRows := make(map[string]int)
	seen := make(map[string]bool)
	var windows []window
	for _, exchange := range exchanges {
		rejections := exchange.Outcome.Value.Rejections
		if exchange.Err != nil {
			var refusal *questionRefusal
			if !errors.As(exchange.Err, &refusal) {
				continue
			}
			rejections = refusal.rejections
			omitted := 0
			for _, rejection := range rejections {
				if rejection.Omitted {
					omitted++
				}
			}
			if omitted == len(exchange.QuestionRefs) {
				continue
			}
		}
		for _, rejection := range rejections {
			q, known := data.exchangeQuestion(exchange, rejection.Question)
			key := rowsKey(exchange.ChunkIndexes)
			if !rejection.Omitted || !known || seen[key+" "+rejection.Question] {
				continue
			}
			seen[key+" "+rejection.Question] = true
			at, found := byRows[key]
			if !found {
				at = len(windows)
				byRows[key] = at
				windows = append(windows, window{rows: append([]int(nil), exchange.ChunkIndexes...), reask: true})
			}
			windows[at].questions = append(windows[at].questions, q)
		}
	}
	for i := range windows {
		sort.Ints(windows[i].questions)
	}
	return windows
}

// markRecovered annotates each first-round omission whose question the
// second round then decided for every row of that window, so a journal
// reader can tell a repaired gap from a loss.
func (data catalogue) markRecovered(result *Result, first int) {
	for i := first; i < len(result.Exchanges); i++ {
		exchange := &result.Exchanges[i]
		if exchange.Err != nil {
			continue
		}
		for j := range exchange.Outcome.Value.Rejections {
			rejection := &exchange.Outcome.Value.Rejections[j]
			q, known := data.exchangeQuestion(*exchange, rejection.Question)
			if !rejection.Omitted || !known {
				continue
			}
			rejection.Recovered = true
			for _, row := range exchange.ChunkIndexes {
				if !result.Questions[q].Chunks[row].Inspected {
					rejection.Recovered = false
					break
				}
			}
		}
	}
}

// exchangeQuestion resolves a response key to the catalogue question this
// exchange asked under it.
func (data catalogue) exchangeQuestion(exchange Exchange, key string) (int, bool) {
	for i, ref := range exchange.QuestionRefs {
		if ref == key && i < len(exchange.QuestionIndexes) {
			return data.inputQuestions[exchange.QuestionIndexes[i]], true
		}
	}
	return 0, false
}

func (data catalogue) expand(result Result) Result {
	questions := make([]QuestionResult, len(data.inputQuestions))
	for i, q := range data.inputQuestions {
		questions[i] = QuestionResult{Question: data.input.Questions[i], Chunks: append([]ChunkResult(nil), result.Questions[q].Chunks...)}
		for j := range questions[i].Chunks {
			questions[i].Chunks[j].Selections = cloneSelections(questions[i].Chunks[j].Selections)
		}
	}
	result.Questions = questions
	return result
}

func cloneSelections(selections []Selection) []Selection {
	result := make([]Selection, len(selections))
	for i, selection := range selections {
		result[i] = selection
		result[i].Anchors = append([]string(nil), selection.Anchors...)
	}
	return result
}

func rowRef(index int) string  { return "r" + strconv.Itoa(index+1) }
func digest(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }

// rowsKey identifies one ordered row set, the shared prefix of every window
// over it.
func rowsKey(rows []int) string { encoded, _ := json.Marshal(rows); return string(encoded) }
