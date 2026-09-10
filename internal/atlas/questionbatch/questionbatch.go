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
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/llm"
)

const Contract = "repomap.atlas.question-batch.v2"

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
}

type Response struct {
	Questions    []Decision          `json:"questions"`
	Rejections   []QuestionRejection `json:"rejections,omitempty"`
	metadataRows []string
}

// Retrieval terms bind to evidence rows, which may be shared by questions.
// A row mentioned by a refused question cannot authorize optional metadata.
func (response Response) AcceptedRowKeys() []string { return response.metadataRows }

func (response Response) ResponseRejections() []llm.ResponseRejection {
	var result []llm.ResponseRejection
	for _, rejection := range response.Rejections {
		result = append(result, llm.ResponseRejection{Kind: "question_rejected", Count: rejection.Chunks, Reason: rejection.Reason, Samples: []string{rejection.Question}})
	}
	return result
}

// ChunkResult has one slot per original input chunk. Only an inspected chunk
// with an empty selection means no anchors were selected. An uninspected slot
// is unavailable and must never become a negative relevance decision.
type ChunkResult struct {
	Inspected      bool
	Anchors        []string
	Relevance      string
	Why            string
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
}

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
	for len(planned) > 0 {
		calls := make([]llm.Call[Response], len(planned))
		for i, part := range planned {
			calls[i], err = data.call(part)
			if err != nil {
				return Result{}, err
			}
		}
		outcomes := llm.ExecuteJSONEach(ctx, executor, provider, calls)
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		var next []window
		for i, outcome := range outcomes {
			part := planned[i]
			exchange := Exchange{ChunkIndexes: append([]int(nil), part.rows...), Outcome: outcome.Outcome, Err: outcome.Err,
				Input: json.RawMessage(calls[i].Prompt.User), System: calls[i].Prompt.System}
			for _, q := range part.questions {
				exchange.QuestionIndexes = append(exchange.QuestionIndexes, data.questionInputs[q])
				exchange.QuestionRefs = append(exchange.QuestionRefs, data.questions[q].Key)
			}
			if outcome.Err == nil {
				for _, q := range part.questions {
					accepted := data.apply(&result.Questions[q], part.rows, data.questions[q].Key, outcome.Outcome)
					if accepted && outcome.Outcome.CacheKey != "" {
						remembered[q] = append(remembered[q], data.reference(part, q, outcome.Outcome.CacheKey))
					}
				}
			} else if splittableResource(outcome.Err) {
				if left, right, ok := data.splitResource(part, outcome.Err); ok {
					exchange.Superseded = true
					for _, child := range []window{left, right} {
						parts, err := data.plan(ctx, provider, child)
						if err != nil {
							return Result{}, err
						}
						next = append(next, parts...)
					}
				}
			}
			result.Exchanges = append(result.Exchanges, exchange)
		}
		planned = next
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	data.remember(executor, keys, remembered, &result)
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	return data.expand(result), nil
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
	return data.requestCall(part.rows, data.windowQuestions(part))
}

func (data catalogue) requestCall(rows []int, questions []modelQuestion) (llm.Call[Response], error) {
	encoded, err := json.Marshal(data.request(rows, questions))
	if err != nil {
		return llm.Call[Response]{}, fmt.Errorf("question batch: encode request: %w", err)
	}
	return llm.Call[Response]{
		State:          []byte(`{"contract":"` + Contract + `"}`),
		Prompt:         llm.Prompt{System: data.opts.System, User: string(encoded), ResponseFormatJSON: true, ResponseExample: responseExample, Reasoning: true},
		Limits:         limits(),
		DecodeValidate: func(raw []byte) (Response, error) { return data.decode(rows, questions, raw) },
	}, nil
}

func limits() llm.Limits {
	// Closed source selections and short relevance explanations use a smaller
	// output allowance. Actual refusals split questions first, without dropping
	// their complete original evidence or limiting the number of questions.
	return llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: 16000}
}

func (data catalogue) plan(ctx context.Context, provider llm.Provider, part window) ([]window, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if maximum := data.opts.MaxRows; maximum > 0 && len(part.rows) > maximum {
		var planned []window
		for start := 0; start < len(part.rows); start += maximum {
			children, err := data.plan(ctx, provider, window{rows: part.rows[start:min(start+maximum, len(part.rows))], questions: part.questions})
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
		return window{rows: part.rows, questions: part.questions[:at]}, window{rows: part.rows, questions: part.questions[at:]}, true
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
		return window{rows: part.rows[:at], questions: part.questions}, window{rows: part.rows[at:], questions: part.questions}, true
	}
	if len(part.questions) > 1 {
		at := balanced(questionWeights)
		return window{rows: part.rows, questions: part.questions[:at]}, window{rows: part.rows, questions: part.questions[at:]}, true
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

func (data catalogue) decode(rows []int, questions []modelQuestion, raw []byte) (Response, error) {
	normalized, err := llm.NormalizeJSON(raw)
	if err != nil {
		return Response{}, err
	}
	var envelope struct {
		Questions []json.RawMessage `json:"questions"`
	}
	if err := json.Unmarshal(normalized, &envelope); err != nil || envelope.Questions == nil {
		return Response{}, fmt.Errorf("question batch: response must contain a questions array")
	}
	allowed := make(map[string]bool, len(questions))
	for _, question := range questions {
		allowed[question.Key] = true
	}
	byQuestion := make(map[string][]Decision)
	failures := make(map[string]string)
	original := make(map[string][]json.RawMessage)
	var unknown []json.RawMessage
	for _, rawQuestion := range envelope.Questions {
		var key struct {
			Key string `json:"key"`
		}
		if json.Unmarshal(rawQuestion, &key) != nil || !allowed[key.Key] {
			unknown = append(unknown, rawQuestion)
			continue
		}
		original[key.Key] = append(original[key.Key], rawQuestion)
		var decision Decision
		if err := json.Unmarshal(rawQuestion, &decision); err != nil {
			failures[key.Key] = "question has an invalid selection shape"
			continue
		}
		byQuestion[key.Key] = append(byQuestion[key.Key], decision)
	}
	result := Response{Questions: []Decision{}}
	unsafe := append([]json.RawMessage(nil), unknown...)
	for _, question := range questions {
		reason := failures[question.Key]
		if reason == "" {
			// Strip unknown optional fields before the existing closed-ref and
			// scalar checks. Each question still needs one coherent selection.
			encoded, _ := json.Marshal(Response{Questions: byQuestion[question.Key]})
			accepted, err := data.decodeComplete(rows, []modelQuestion{question}, encoded)
			if err == nil {
				result.Questions = append(result.Questions, accepted.Questions...)
				continue
			}
			reason = err.Error()
		}
		result.Rejections = append(result.Rejections, QuestionRejection{Question: question.Key, Reason: reason, Chunks: len(rows)})
		unsafe = append(unsafe, original[question.Key]...)
	}
	if len(result.Questions) == 0 {
		return result, fmt.Errorf("question batch: no questions accepted: %s", result.Rejections[0].Reason)
	}
	if len(result.Rejections) > 0 || len(unknown) > 0 {
		result.metadataRows = safeQuestionMetadataRows(rows, unsafe)
	}
	return result, nil
}

// The glossary observes the exact raw result. Suppress source-row metadata
// touched by a refused question, even if an accepted question shares that row.
func safeQuestionMetadataRows(rows []int, refused []json.RawMessage) []string {
	blocked := make(map[string]bool)
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

func (data catalogue) decodeComplete(rows []int, questions []modelQuestion, raw []byte) (Response, error) {
	response, err := llm.DecodeJSON[Response](nil)(raw)
	if err != nil {
		return Response{}, err
	}
	allowedRows := make(map[string]int, len(rows))
	for _, row := range rows {
		allowedRows[rowRef(row)] = row
	}
	allowedQuestions := make(map[string]bool, len(questions))
	for _, question := range questions {
		allowedQuestions[question.Key] = true
	}
	byQuestion := make(map[string]Decision, len(questions))
	for _, decision := range response.Questions {
		if !allowedQuestions[decision.Key] {
			continue
		}
		if decision.Selections == nil {
			return Response{}, fmt.Errorf("question batch: missing selections for %s", decision.Key)
		}
		byRow := make(map[string]Selection)
		reasons := make(map[string]map[string]bool)
		for _, selection := range decision.Selections {
			row, known := allowedRows[selection.Row]
			if !known {
				continue
			}
			allowedAnchors := make(map[string]bool, len(data.anchorOptions[row]))
			for _, ref := range data.anchorOptions[row] {
				allowedAnchors[ref] = true
			}
			anchors := make(map[string]bool)
			for _, ref := range selection.Anchors {
				if allowedAnchors[ref] {
					anchors[ref] = true
				}
			}
			if len(anchors) == 0 {
				return Response{}, fmt.Errorf("question batch: positive selection has no advertised anchors for %s/%s", decision.Key, selection.Row)
			}
			if selection.Relevance != "direct" && selection.Relevance != "context" || strings.TrimSpace(selection.Why) == "" {
				return Response{}, fmt.Errorf("question batch: invalid relevance or reason for %s/%s", decision.Key, selection.Row)
			}
			selection.Anchors = nil
			// Preserve the source row's advertised order, independent of output
			// ordering or duplicate set members.
			for _, ref := range data.anchorOptions[row] {
				if anchors[ref] {
					selection.Anchors = append(selection.Anchors, ref)
					delete(anchors, ref)
				}
			}
			if previous, duplicate := byRow[selection.Row]; duplicate {
				if previous.Relevance != selection.Relevance {
					return Response{}, fmt.Errorf("question batch: conflicting selection for %s/%s", decision.Key, selection.Row)
				}
				for _, ref := range previous.Anchors {
					anchors[ref] = true
				}
				for _, ref := range selection.Anchors {
					anchors[ref] = true
				}
				selection.Anchors = nil
				for _, ref := range data.anchorOptions[row] {
					if anchors[ref] {
						selection.Anchors = append(selection.Anchors, ref)
						delete(anchors, ref)
					}
				}
			}
			if reasons[selection.Row] == nil {
				reasons[selection.Row] = make(map[string]bool)
			}
			reasons[selection.Row][selection.Why] = true
			byRow[selection.Row] = selection
		}
		normalized := Decision{Key: decision.Key, Selections: []Selection{}}
		for _, row := range rows {
			if selection, selected := byRow[rowRef(row)]; selected {
				// Different explanations of the same relevance decision are
				// independent hints. Preserve every original hint in stable order;
				// neither the first nor the last response row wins.
				var hints []string
				for reason := range reasons[selection.Row] {
					hints = append(hints, reason)
				}
				sort.Strings(hints)
				selection.Why = strings.Join(hints, "\n\n")
				normalized.Selections = append(normalized.Selections, selection)
			}
		}
		if previous, duplicate := byQuestion[decision.Key]; duplicate {
			first, _ := json.Marshal(previous)
			second, _ := json.Marshal(normalized)
			if !bytes.Equal(first, second) {
				return Response{}, fmt.Errorf("question batch: conflicting question result %s", decision.Key)
			}
		}
		byQuestion[decision.Key] = normalized
	}
	result := Response{Questions: make([]Decision, 0, len(questions))}
	for _, question := range questions {
		decision, found := byQuestion[question.Key]
		if !found {
			return Response{}, fmt.Errorf("question batch: missing question %s", question.Key)
		}
		result.Questions = append(result.Questions, decision)
	}
	return result, nil
}

func (data catalogue) apply(question *QuestionResult, rows []int, ref string, outcome llm.Outcome[Response]) bool {
	accepted := false
	byRow := make(map[string]Selection)
	for _, decision := range outcome.Value.Questions {
		if decision.Key == ref {
			accepted = true
			for _, selection := range decision.Selections {
				byRow[selection.Row] = selection
			}
		}
	}
	if !accepted {
		return false
	}
	source := atlas.SourceModel
	if outcome.Cached {
		source = atlas.SourceCache
	}
	for _, row := range rows {
		selection := byRow[rowRef(row)]
		question.Chunks[row] = ChunkResult{
			Inspected: true, Anchors: append([]string{}, selection.Anchors...), Relevance: selection.Relevance, Why: selection.Why,
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
		encoded, _ := json.Marshal(rows)
		key := string(encoded)
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

func (data catalogue) expand(result Result) Result {
	questions := make([]QuestionResult, len(data.inputQuestions))
	for i, q := range data.inputQuestions {
		questions[i] = QuestionResult{Question: data.input.Questions[i], Chunks: append([]ChunkResult(nil), result.Questions[q].Chunks...)}
		for j := range questions[i].Chunks {
			questions[i].Chunks[j].Anchors = append([]string(nil), questions[i].Chunks[j].Anchors...)
		}
	}
	result.Questions = questions
	return result
}

func rowRef(index int) string  { return "r" + strconv.Itoa(index+1) }
func digest(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
