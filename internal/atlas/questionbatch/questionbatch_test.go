package questionbatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/llm"
)

type testWire struct {
	Prompt llm.Prompt
	Limits llm.Limits
}

type testProvider struct {
	mu           sync.Mutex
	requests     []testWire
	requestBytes int
	complete     func(modelRequest) (Response, error)
}

func (*testProvider) State() []byte { return []byte(`{"provider":"question-batch-test"}`) }
func (provider *testProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	raw, err := json.Marshal(testWire{prompt, limits})
	if err != nil {
		return llm.Prepared{}, err
	}
	if provider.requestBytes > 0 && len(raw) > provider.requestBytes {
		return llm.Prepared{}, llm.NewResourceLimitError(llm.ResourceLimitError{Kind: llm.ResourceLimitRequestBytes, Limit: provider.requestBytes, Observed: len(raw), ObservedKnown: true})
	}
	return llm.NewPrepared(raw)
}
func (provider *testProvider) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var wire testWire
	if err := json.Unmarshal(prepared.Bytes(), &wire); err != nil {
		return llm.Completion{}, err
	}
	var request modelRequest
	if err := json.Unmarshal([]byte(wire.Prompt.User), &request); err != nil {
		return llm.Completion{}, err
	}
	provider.mu.Lock()
	provider.requests = append(provider.requests, wire)
	provider.mu.Unlock()
	response := selectFirst(request, "Inspect the original declaration.")
	var err error
	if provider.complete != nil {
		response, err = provider.complete(request)
	}
	if err != nil {
		return llm.Completion{}, err
	}
	raw, err := json.Marshal(response)
	return llm.Completion{Response: raw, FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, err
}

func testInput(rows, questions int) Input {
	input := Input{Repository: "sample", Chunks: make([]lines.QuestionChunk, rows)}
	for i := range input.Chunks {
		path := fmt.Sprintf("src/file%d.go", i+1)
		input.Chunks[i] = lines.QuestionChunk{
			Row: table.Row{ID: fmt.Sprintf("local-row-id-%d", i), Fields: []table.Field{
				{Name: "path", Value: path},
				{Name: "evidence", Value: []map[string]any{
					{"ref": "a1", "kind": "type", "name": "Response", "anchor_path": path, "anchor_line": 8,
						"owned_declarations": []map[string]any{{"name": "Count", "kind": "variable", "path": path, "line": 9, "signature": "Count int `json:\"count\"`"}}},
					{"ref": "a2", "kind": "documentation", "anchor_path": "README.md", "anchor_line": 20, "author_text": "Read the original instructions.\n\nThen verify the result."},
				}},
				{Name: "anchor_options", Value: []string{"a1", "a2"}},
			}},
			Place: atlas.Place{ID: fmt.Sprintf("local-place-id-%d", i), Path: path},
			Anchors: map[string]lines.QuestionAnchor{
				"a1": {SubjectID: fmt.Sprintf("local-subject-id-%d", i), Path: path, Line: 8, Column: 2, Name: "Response", Kind: "type"},
				"a2": {SubjectID: fmt.Sprintf("local-doc-id-%d", i), Path: "README.md", Line: 20, Column: 1, Kind: "documentation"},
			},
		}
	}
	for i := 0; i < questions; i++ {
		input.Questions = append(input.Questions, fmt.Sprintf("Question %d?", i+1))
	}
	return input
}

func requestRowRef(row json.RawMessage) string {
	var ref struct {
		Key string `json:"key"`
	}
	_ = json.Unmarshal(row, &ref)
	return ref.Key
}
func selectFirst(request modelRequest, why string) Response {
	response := Response{Questions: make([]Decision, len(request.Questions))}
	for i, q := range request.Questions {
		response.Questions[i] = Decision{Key: q.Key, Selections: []Selection{{Row: requestRowRef(request.Evidence[0]), Anchors: []string{"a1"}, Relevance: "direct", Why: why}}}
	}
	return response
}

func TestRunSharesCompleteCatalogueAndPreservesOriginalRefs(t *testing.T) {
	input := testInput(45, 8)
	input.Questions = append(input.Questions, input.Questions[0])
	provider := &testProvider{complete: func(request modelRequest) (Response, error) {
		response := selectFirst(request, "Names suggest a useful place; no test was executed.")
		response.Questions[0].Selections[0].Anchors = []string{"a2", "unknown", "a1", "a2"}
		response.Questions[0].Selections = append(response.Questions[0].Selections, Selection{Row: "unknown", Relevance: "invalid"})
		response.Questions[1].Selections = []Selection{}
		response.Questions = append(response.Questions, response.Questions[0], Decision{Key: "q999"})
		return response, nil
	}}
	result, err := Run(t.Context(), llm.Executor{BatchConcurrency: 4}, provider, input, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 1 || len(result.Exchanges) != 1 {
		t.Fatalf("45 rows and 8 questions need one fitting request, got %d", len(provider.requests))
	}
	wire := provider.requests[0]
	if !wire.Prompt.Reasoning || !wire.Prompt.ResponseFormatJSON || wire.Prompt.ResponseLanguage != "en" || wire.Limits != limits() {
		t.Fatalf("provider policy/envelope = %#v", wire)
	}
	for _, forbidden := range []string{"local-row-id", "local-place-id", "local-subject-id", "local-doc-id"} {
		if strings.Contains(wire.Prompt.User, forbidden) {
			t.Fatalf("canonical identity leaked: %s", forbidden)
		}
	}
	var request modelRequest
	if err := json.Unmarshal([]byte(wire.Prompt.User), &request); err != nil {
		t.Fatal(err)
	}
	if len(request.Evidence) != 45 || len(request.Questions) != 8 || request.Task != Contract {
		t.Fatalf("full independent inputs not preserved: %d/%d", len(request.Evidence), len(request.Questions))
	}
	var row struct {
		Evidence []map[string]any `json:"evidence"`
	}
	if err := json.Unmarshal(request.Evidence[0], &row); err != nil {
		t.Fatal(err)
	}
	if len(row.Evidence) != 2 || row.Evidence[0]["owned_declarations"] == nil || row.Evidence[1]["author_text"] != "Read the original instructions.\n\nThen verify the result." {
		t.Fatal("original owned declarations or paragraphs were lost")
	}
	if !reflect.DeepEqual(result.Questions[0].Chunks[0].Anchors, []string{"a1", "a2"}) {
		t.Fatalf("closed anchor normalization: %#v", result.Questions[0].Chunks[0])
	}
	for q, question := range result.Questions {
		if len(question.Chunks) != 45 {
			t.Fatalf("question %d chunk count", q)
		}
		for row, chunk := range question.Chunks {
			if !chunk.Inspected || chunk.Source != atlas.SourceModel {
				t.Fatalf("unselected row became unavailable at %d/%d", q, row)
			}
		}
	}
	if len(result.Questions[1].Chunks[0].Anchors) != 0 || !result.Questions[1].Chunks[0].Inspected {
		t.Fatal("explicit empty selection is not an inspected result")
	}
	if !reflect.DeepEqual(result.Questions[0].Chunks, result.Questions[8].Chunks) {
		t.Fatal("duplicate exact question did not share its original result")
	}
	if len(result.Exchanges[0].Input) == 0 || result.Exchanges[0].System != Prompt() || !reflect.DeepEqual(result.Exchanges[0].QuestionRefs, []string{"q1", "q2", "q3", "q4", "q5", "q6", "q7", "q8"}) {
		t.Fatal("missing source-request audit metadata")
	}
}

func TestRunEachKeepsRefusedWindowUnavailableAndRecallsAcceptedSiblings(t *testing.T) {
	input := testInput(4, 2)
	provider := &testProvider{complete: func(request modelRequest) (Response, error) {
		response := selectFirst(request, "Inspect this declaration.")
		if requestRowRef(request.Evidence[0]) == "r2" {
			response.Questions = response.Questions[:1]
		}
		return response, nil
	}}
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir(), BatchConcurrency: 4}
	result, err := Run(t.Context(), executor, provider, input, Options{MaxRows: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 4 || len(result.Exchanges) != 4 {
		t.Fatalf("wanted 4 shared calls, got %d/%d", len(provider.requests), len(result.Exchanges))
	}
	for q, question := range result.Questions {
		for row, chunk := range question.Chunks {
			if chunk.Inspected == (q == 1 && row == 1) {
				t.Fatalf("missing q invalidated wrong chunk: row%d %#v", row, chunk)
			}
		}
		if q == 1 && (question.Chunks[1].Relevance != "" || len(question.Chunks[1].Anchors) != 0) {
			t.Fatal("refusal became a none decision")
		}
	}
	provider.complete = nil
	before := len(provider.requests)
	warm, err := Run(t.Context(), executor, provider, input, Options{MaxRows: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests)-before != 1 {
		t.Fatalf("accepted siblings were recalled again: %d calls", len(provider.requests)-before)
	}
	reused := 0
	for _, exchange := range warm.Exchanges {
		if exchange.Reused {
			reused++
			want := 2
			if exchange.ChunkIndexes[0] == 1 {
				want = 1
			}
			if !exchange.Outcome.Cached || len(exchange.QuestionIndexes) != want || len(exchange.QuestionRefs) != want {
				t.Fatal("shared memo exchange lost its current question audit refs")
			}
		}
	}
	if reused != 4 || len(warm.Exchanges) != 5 {
		t.Fatalf("memo exchanges not deduplicated by original request: %#v", warm.Exchanges)
	}
	for _, q := range warm.Questions {
		for _, chunk := range q.Chunks {
			if !chunk.Inspected {
				t.Fatal("missing window was not completed")
			}
		}
	}
}

func TestRunRejectsKnownPositiveWithNoAnchorsAndConflictingScalars(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*Response)
	}{
		{"unknown anchors", func(r *Response) { r.Questions[0].Selections[0].Anchors = []string{"not-advertised"} }},
		{"empty anchors", func(r *Response) { r.Questions[0].Selections[0].Anchors = []string{} }},
		{"missing selections", func(r *Response) { r.Questions[0].Selections = nil }},
		{"bad relevance", func(r *Response) { r.Questions[0].Selections[0].Relevance = "none" }},
		{"blank why", func(r *Response) { r.Questions[0].Selections[0].Why = " \n " }},
		{"conflicting row", func(r *Response) {
			s := r.Questions[0].Selections[0]
			s.Relevance = "context"
			r.Questions[0].Selections = append(r.Questions[0].Selections, s)
		}},
		{"conflicting question", func(r *Response) {
			r.Questions = append(r.Questions, Decision{Key: r.Questions[0].Key, Selections: []Selection{}})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := &testProvider{complete: func(request modelRequest) (Response, error) {
				response := selectFirst(request, "Relevant original declaration.")
				test.edit(&response)
				return response, nil
			}}
			result, err := Run(t.Context(), llm.Executor{}, provider, testInput(1, 2), Options{})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Exchanges) != 1 || result.Exchanges[0].Err != nil || result.Exchanges[0].Superseded || len(result.Exchanges[0].Outcome.Value.Rejections) != 1 {
				t.Fatal("semantic refusal lost its reason, invalidated a neighbour or repartitioned")
			}
			if result.Questions[0].Chunks[0].Inspected || !result.Questions[1].Chunks[0].Inspected || len(result.Questions[1].Chunks[0].Anchors) == 0 {
				t.Fatal("bad question became inspected/none or its accepted neighbour was lost")
			}
		})
	}
}

func TestRunSplitsRealResourceFailuresByWholeEvidenceBytes(t *testing.T) {
	input := testInput(10, 2)
	input.Chunks[0].Row.Fields = append(input.Chunks[0].Row.Fields, table.Field{Name: "file_author_doc", Value: strings.Repeat("large original paragraph\n", 230)})
	data, err := prepareCatalogue(input, Options{})
	if err != nil {
		t.Fatal(err)
	}
	whole := window{rows: []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, questions: []int{0, 1}}
	left, right, ok := data.split(whole)
	if !ok || !reflect.DeepEqual(left.rows, []int{0}) || len(right.rows) != 9 {
		t.Fatalf("uneven rows split by count instead of bytes: %#v/%#v", left, right)
	}
	for _, kind := range []llm.ResourceLimitKind{llm.ResourceLimitContextTokens, llm.ResourceLimitRequestBytes} {
		t.Run(string(kind), func(t *testing.T) {
			provider := &testProvider{complete: func(request modelRequest) (Response, error) {
				if len(request.Evidence) > 1 && requestRowRef(request.Evidence[0]) == "r1" {
					return Response{}, llm.NewResourceLimitError(llm.ResourceLimitError{Kind: kind, Limit: 1})
				}
				return selectFirst(request, "Original source survives the split."), nil
			}}
			result, err := Run(t.Context(), llm.Executor{BatchConcurrency: 4}, provider, input, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Exchanges) != 3 || !result.Exchanges[0].Superseded || result.Exchanges[0].Err == nil {
				t.Fatalf("resource attempts = %#v", result.Exchanges)
			}
			if !reflect.DeepEqual(result.Exchanges[1].ChunkIndexes, []int{0}) || len(result.Exchanges[2].ChunkIndexes) != 9 {
				t.Fatal("whole weighted source partitions changed")
			}
			for _, q := range result.Questions {
				for _, chunk := range q.Chunks {
					if !chunk.Inspected {
						t.Fatal("resource split lost source coverage")
					}
				}
			}
		})
	}
	atomic := &testProvider{complete: func(modelRequest) (Response, error) {
		return Response{}, llm.NewResourceLimitError(llm.ResourceLimitError{Kind: llm.ResourceLimitContextTokens})
	}}
	result, err := Run(t.Context(), llm.Executor{}, atomic, testInput(1, 1), Options{})
	if err != nil || result.Questions[0].Chunks[0].Inspected || len(result.Exchanges) != 1 || result.Exchanges[0].Superseded {
		t.Fatalf("atomic refusal changed authority: %#v %v", result, err)
	}
}

func TestRunSplitsOutputLimitsByQuestionsBeforeRepeatingEvidence(t *testing.T) {
	for _, kind := range []llm.ResourceLimitKind{llm.ResourceLimitOutputTokens, llm.ResourceLimitResponseBytes} {
		t.Run(string(kind), func(t *testing.T) {
			input := testInput(10, 8)
			input.Chunks[0].Row.Fields = append(input.Chunks[0].Row.Fields, table.Field{Name: "original_excerpt", Value: strings.Repeat("large original paragraph\n", 230)})
			provider := &testProvider{complete: func(request modelRequest) (Response, error) {
				if len(request.Questions) > 4 {
					return Response{}, llm.NewResourceLimitError(llm.ResourceLimitError{Kind: kind, Limit: 1})
				}
				return selectFirst(request, "Each question still reads the complete evidence."), nil
			}}
			result, err := Run(t.Context(), llm.Executor{BatchConcurrency: 4}, provider, input, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Exchanges) != 3 || !result.Exchanges[0].Superseded || result.Exchanges[0].Err == nil {
				t.Fatalf("output resource attempts = %#v", result.Exchanges)
			}
			var original modelRequest
			if err := json.Unmarshal(result.Exchanges[0].Input, &original); err != nil {
				t.Fatal(err)
			}
			seen := make(map[string]bool)
			for _, exchange := range result.Exchanges[1:] {
				var child modelRequest
				if err := json.Unmarshal(exchange.Input, &child); err != nil {
					t.Fatal(err)
				}
				if exchange.Err != nil || len(child.Questions) != 4 || !reflect.DeepEqual(child.Evidence, original.Evidence) {
					t.Fatal("output split must reduce questions and preserve every original evidence byte")
				}
				for _, question := range child.Questions {
					if seen[question.Key] {
						t.Fatal("question repeated after output split")
					}
					seen[question.Key] = true
				}
			}
			if len(seen) != len(input.Questions) {
				t.Fatal("output split lost a question")
			}
			for _, question := range result.Questions {
				for _, chunk := range question.Chunks {
					if !chunk.Inspected {
						t.Fatal("output split lost question/source coverage")
					}
				}
			}
			// Once only one question remains, complete source rows can still
			// split; an atomic question/source pair cannot retry unchanged.
			data, err := prepareCatalogue(input, Options{})
			if err != nil {
				t.Fatal(err)
			}
			failure := llm.NewResourceLimitError(llm.ResourceLimitError{Kind: kind})
			left, right, ok := data.splitResource(window{rows: []int{0, 1}, questions: []int{0}}, failure)
			if !ok || len(left.rows) != 1 || len(right.rows) != 1 || !reflect.DeepEqual(left.questions, right.questions) {
				t.Fatal("single question cannot reduce its complete evidence rows")
			}
			if _, _, ok := data.splitResource(window{rows: []int{0}, questions: []int{0}}, failure); ok {
				t.Fatal("atomic output failure retried unchanged")
			}
		})
	}
}

func TestRunPartitionsQuestionsWhenTheyDominateAndHonorsExplicitBudgets(t *testing.T) {
	input := testInput(1, 4)
	for i := range input.Questions {
		input.Questions[i] += strings.Repeat(" complete question context", 100)
	}
	data, err := prepareCatalogue(input, Options{})
	if err != nil {
		t.Fatal(err)
	}
	call, err := data.call(window{rows: []int{0}, questions: []int{0, 1}})
	if err != nil {
		t.Fatal(err)
	}
	provider := &testProvider{}
	prepared, err := llm.Prepare(provider, call.Prompt, call.Limits)
	if err != nil {
		t.Fatal(err)
	}
	provider.requestBytes = prepared.Len()
	result, err := Run(t.Context(), llm.Executor{}, provider, input, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Exchanges) != 2 {
		t.Fatalf("question partitions = %d", len(result.Exchanges))
	}
	for _, exchange := range result.Exchanges {
		if len(exchange.ChunkIndexes) != 1 || len(exchange.QuestionRefs) != 2 {
			t.Fatal("question partition lost complete evidence")
		}
	}
	provider.requestBytes = 0
	provider.requests = nil
	result, err = Run(t.Context(), llm.Executor{}, provider, input, Options{MaxInputBytes: len(call.Prompt.System) + len(call.Prompt.User)})
	if err != nil || len(result.Exchanges) != 2 {
		t.Fatalf("explicit input budget = %d windows, %v", len(result.Exchanges), err)
	}
	if _, err := Run(t.Context(), llm.Executor{}, provider, input, Options{MaxInputBytes: 1}); err == nil {
		t.Fatal("complete oversized singleton was truncated")
	}
	if _, err := Run(t.Context(), llm.Executor{}, provider, input, Options{MaxRows: -1}); err == nil {
		t.Fatal("negative row budget accepted")
	}
}

func TestRunEmptyAndCancellationNeedNoSemanticFallback(t *testing.T) {
	for _, input := range []Input{testInput(0, 2), testInput(2, 0)} {
		result, err := Run(t.Context(), llm.Executor{}, nil, input, Options{})
		if err != nil || len(result.Exchanges) != 0 || len(result.Questions) != len(input.Questions) {
			t.Fatalf("legitimate empty input: %#v %v", result, err)
		}
	}
	if _, err := Run(t.Context(), llm.Executor{}, nil, testInput(1, 1), Options{}); err == nil {
		t.Fatal("missing provider accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Run(ctx, llm.Executor{}, &testProvider{}, testInput(1, 1), Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}
