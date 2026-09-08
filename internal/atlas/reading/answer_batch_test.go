package reading

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/llm"
)

type answerTestRequest struct {
	Table   string `json:"table"`
	Context struct {
		Candidates []answerSource `json:"candidates"`
	} `json:"context"`
	Rows []map[string]any `json:"rows"`
}

type answerTestProvider struct {
	*tableProvider
	mu             sync.Mutex
	requests       [][]byte
	prepareAnswer  func(answerTestRequest, llm.Prompt) error
	completeAnswer func(answerTestRequest) ([]byte, error)
	invalidState   bool
}

func (p *answerTestProvider) State() []byte {
	if p.invalidState {
		return []byte(`invalid`)
	}
	return p.tableProvider.State()
}

func (p *answerTestProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	var request answerTestRequest
	if err := json.Unmarshal([]byte(prompt.User), &request); err != nil {
		return llm.Prepared{}, err
	}
	if request.Table == lines.StageAnswer && p.prepareAnswer != nil {
		if err := p.prepareAnswer(request, prompt); err != nil {
			return llm.Prepared{}, err
		}
	}
	return p.tableProvider.Prepare(prompt, limits)
}
func (p *answerTestProvider) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var request answerTestRequest
	if err := json.Unmarshal(prepared.Bytes(), &request); err != nil {
		return llm.Completion{}, err
	}
	if request.Table == lines.StageAnswer {
		p.mu.Lock()
		p.requests = append(p.requests, prepared.Bytes())
		p.mu.Unlock()
		if p.completeAnswer != nil {
			raw, err := p.completeAnswer(request)
			if raw != nil || err != nil {
				return llm.Completion{Response: raw, FinishReason: llm.FinishStop, ChoiceCount: 1}, err
			}
		}
	}
	return p.tableProvider.Complete(ctx, prepared)
}

func answerTestRoutes(count int) []atlas.QuestionRoute {
	routes := make([]atlas.QuestionRoute, count)
	for i := range routes {
		routes[i] = atlas.QuestionRoute{Question: fmt.Sprintf("Question %02d?", i), Stops: []atlas.QuestionStop{{PlaceID: fmt.Sprintf("internal:%d", i), Path: fmt.Sprintf("source-%02d.go", i), Line: 1, Name: "Run", Evidence: map[string]any{"signature": "Run()"}, Why: fmt.Sprintf("Reason %d", i)}}}
	}
	return routes
}
func answerTestReader(t *testing.T, routes []atlas.QuestionRoute, provider llm.Provider) *reader {
	t.Helper()
	dir := t.TempDir()
	opts := readOptions(t, testGraph(t), provider, dir)
	if err := os.Mkdir(filepath.Join(opts.OwnerRunDir, atlas.TablesDir), 0700); err != nil {
		t.Fatal(err)
	}
	opts.Through = lines.StageAnswer
	opts.Stage = func(string, ...string) {}
	opts.State = func(string, string, ...string) {}
	return &reader{opts: opts, questions: routes, uses: make(map[string]*atlas.StageUse), started: make(map[string]time.Time)}
}
func resourceRefusal(kind llm.ResourceLimitKind) error {
	return llm.NewResourceLimitError(llm.ResourceLimitError{Stage: lines.StageAnswer, Kind: kind, Limit: 1})
}

func TestAnswerBatchSharesEvidenceKeepsRowsAndCanonicalExactCache(t *testing.T) {
	opts, base := questionFixture(t)
	opts.Through, opts.WindowRows = lines.StageAnswer, 0
	opts.Questions = []string{"Z question?", "A question?"}
	provider := &answerTestProvider{tableProvider: base}
	opts.Provider = provider
	provider.prepareAnswer = func(_ answerTestRequest, prompt llm.Prompt) error {
		if !prompt.Reasoning {
			t.Fatal("final answers lost reasoning")
		}
		return nil
	}
	first, err := Read(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 1 || len(first.Questions) != 2 {
		t.Fatalf("requests=%d questions=%d", len(provider.requests), len(first.Questions))
	}
	var request answerTestRequest
	if err := json.Unmarshal(provider.requests[0], &request); err != nil {
		t.Fatal(err)
	}
	if len(request.Rows) != 2 || len(request.Context.Candidates) != 4 || request.Rows[0]["question"] != "A question?" {
		t.Fatalf("not a canonical shared catalogue: %+v", request)
	}
	for _, source := range request.Context.Candidates {
		if !reflect.DeepEqual(source.ResultRows, []string{"r1", "r2"}) {
			t.Fatalf("source row origins=%v", source.ResultRows)
		}
	}
	a, b := first.Questions[0].Answer.Parts[0], first.Questions[1].Answer.Parts[0]
	if a.OriginRequest == "" || a.OriginRequest != b.OriginRequest || a.OriginRow != "r2" || b.OriginRow != "r1" {
		t.Fatalf("shared origin lost exact row: %+v / %+v", a, b)
	}
	for i, q := range first.Questions {
		if q.Question != opts.Questions[i] || !reflect.DeepEqual(q.Guide.Steps, q.Answer.Parts[0].Steps) {
			t.Fatal("presentation order or derived reading changed")
		}
	}
	opts.OwnerRunDir = t.TempDir()
	opts.Questions = []string{"A question?", "Z question?"}
	warm, err := Read(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 1 || warm.Questions[0].Answer.Parts[0].Source != atlas.SourceCache {
		t.Fatal("reordering invalidated the exact common request")
	}
	opts.OwnerRunDir = t.TempDir()
	opts.Questions = append(opts.Questions, "Another question?")
	expanded, err := Read(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 2 {
		t.Fatal("a changed full request did not generate one new complete batch")
	}
	for _, question := range expanded.Questions {
		if question.Answer.Parts[0].Source != atlas.SourceModel {
			t.Fatal("exact cache invented independent answer reuse")
		}
	}
	files, err := filepath.Glob(filepath.Join(opts.OwnerRunDir, atlas.TablesDir, "atlas_route*"))
	if err != nil || len(files) > 0 {
		t.Fatal("selector artifacts survived")
	}
}

func TestAnswerSourcesKeepIndependentObservationsAndQuestionHints(t *testing.T) {
	routes := answerTestRoutes(2)
	routes[0].Stops = append(routes[0].Stops, atlas.QuestionStop{PlaceID: "another:producer", Path: routes[0].Stops[0].Path, Line: 1, Evidence: map[string]any{"extractor": "sqlc"}, Why: "A separate original observation"})
	routes[1].Stops = append(routes[1].Stops, routes[0].Stops[0])
	routes[1].Stops[1].Why = "Different question hint"
	parts := []answerQuestion{{index: 0, candidates: uniqueRouteAnchors(routes[0].Stops), complete: true}, {index: 1, candidates: uniqueRouteAnchors(routes[1].Stops), complete: true}}
	window, err := makeAnswerWindow(lines.Answer(), routes, parts)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(window.table.Request)
	for _, retained := range []string{"sqlc", "Different question hint", "A separate original observation", "Run()"} {
		if !strings.Contains(raw, retained) {
			t.Fatalf("lost %q", retained)
		}
	}
	if strings.Contains(raw, "internal:") || strings.Contains(raw, "another:producer") {
		t.Fatal("internal source identity entered the request")
	}
	if len(window.bindings[0]) != 1 || len(window.bindings[1]) != 2 {
		t.Fatal("question source sets were broadened")
	}
	call, err := answerCall(lines.Answer(), window)
	if err != nil {
		t.Fatal(err)
	}
	// A source belonging only to the other row cannot substantiate an answer.
	forbidden := ""
	for ref := range window.bindings[1] {
		if _, ok := window.bindings[0][ref]; !ok {
			forbidden = ref
			break
		}
	}
	response := []table.Answer{{"key": "r1", "answer": "A claim.", "basis": "Observed.", "sources": forbidden, "remaining": "none", "state": "answered"}, {"key": "r2", "answer": "none", "basis": "none", "sources": "none", "remaining": "Missing evidence.", "state": "unanswered"}}
	encoded, _ := json.Marshal(map[string]any{"rows": response})
	if _, err := call.DecodeValidate(encoded); err == nil {
		t.Fatal("another question's sources became authority")
	}
}

func TestAnswerBatchResourceRefusalsSplitQuestionsBeforeSources(t *testing.T) {
	for _, kind := range []llm.ResourceLimitKind{llm.ResourceLimitContextTokens, llm.ResourceLimitOutputTokens, llm.ResourceLimitResponseBytes} {
		t.Run(string(kind), func(t *testing.T) {
			provider := &answerTestProvider{tableProvider: &tableProvider{}}
			provider.completeAnswer = func(request answerTestRequest) ([]byte, error) {
				if len(request.Rows) > 1 {
					return nil, resourceRefusal(kind)
				}
				return nil, nil
			}
			r := answerTestReader(t, answerTestRoutes(4), provider)
			if err := r.readAnswers(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(provider.requests) != 7 || len(r.rejected) != 0 {
				t.Fatalf("attempts=%d rejected=%v", len(provider.requests), r.rejected)
			}
			for _, raw := range provider.requests {
				var request answerTestRequest
				json.Unmarshal(raw, &request)
				if len(request.Context.Candidates) != len(request.Rows) {
					t.Fatal("child retained unrelated sibling sources")
				}
				for _, row := range request.Rows {
					if row["evidence_complete"] != true {
						t.Fatal("question split fragmented its own evidence")
					}
				}
			}
			for _, question := range r.questions {
				if len(question.Answer.Parts) != 1 || len(question.Answer.Parts[0].Steps) != 1 {
					t.Fatalf("lost complete answer: %+v", question.Answer)
				}
			}
		})
	}
}

func TestAnswerBatchSingletonPreservesEveryOriginalSourceInParts(t *testing.T) {
	routes := answerTestRoutes(1)
	for i := 1; i < 5; i++ {
		routes[0].Stops = append(routes[0].Stops, atlas.QuestionStop{Path: fmt.Sprintf("part-%d.go", i), Line: 1, Evidence: map[string]any{"text": strings.Repeat("original ", 100)}})
	}
	provider := &answerTestProvider{tableProvider: &tableProvider{}}
	provider.completeAnswer = func(request answerTestRequest) ([]byte, error) {
		if len(request.Context.Candidates) > 1 {
			return nil, resourceRefusal(llm.ResourceLimitContextTokens)
		}
		return nil, nil
	}
	r := answerTestReader(t, routes, provider)
	if err := r.readAnswers(context.Background()); err != nil {
		t.Fatal(err)
	}
	answer := r.questions[0].Answer
	if len(answer.Parts) != 5 || answer.State != "partial" || len(r.questions[0].Guide.Parts) != 5 {
		t.Fatalf("partition result: %+v", r.questions[0])
	}
	seen := make(map[string]bool)
	for _, part := range answer.Parts {
		for _, step := range part.Steps {
			if seen[step.Path] {
				t.Fatal("duplicated source part")
			}
			seen[step.Path] = true
		}
	}
	if len(seen) != 5 {
		t.Fatal("lost original source")
	}
	for _, raw := range provider.requests {
		var request answerTestRequest
		json.Unmarshal(raw, &request)
		if len(request.Context.Candidates) == 1 && request.Rows[0]["evidence_complete"] != false {
			t.Fatal("partial evidence claimed a whole-question comparison")
		}
	}
}

func TestAnswerBatchMalformedWindowKeepsAcceptedSibling(t *testing.T) {
	provider := &answerTestProvider{tableProvider: &tableProvider{}}
	provider.completeAnswer = func(request answerTestRequest) ([]byte, error) {
		if len(request.Rows) > 1 {
			return nil, resourceRefusal(llm.ResourceLimitOutputTokens)
		}
		if request.Rows[0]["question"] == "Question 00?" {
			return []byte(`{"rows":[`), nil
		}
		return nil, nil
	}
	r := answerTestReader(t, answerTestRoutes(2), provider)
	if err := r.readAnswers(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 3 || r.questions[0].Answer.State != "unavailable" || r.questions[1].Answer.Parts[0].Source != atlas.SourceModel || len(r.rejected) != 1 {
		t.Fatalf("failed window altered sibling: %+v", r.questions)
	}
}

func TestAnswerBatchUsesPreparedEnvelopeWithoutDefaultByteCap(t *testing.T) {
	routes := answerTestRoutes(2)
	for i := range routes {
		routes[i].Stops[0].Evidence["text"] = strings.Repeat("original ", 10000)
	}
	provider := &answerTestProvider{tableProvider: &tableProvider{}}
	r := answerTestReader(t, routes, provider)
	if err := r.readAnswers(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 1 || len(provider.requests[0]) < 2*table.DefaultInputBytes {
		t.Fatal("ordinary answer was divided by the old local budget")
	}
	provider.requests = nil
	provider.prepareAnswer = func(request answerTestRequest, _ llm.Prompt) error {
		if len(request.Rows) > 1 {
			return resourceRefusal(llm.ResourceLimitRequestBytes)
		}
		return nil
	}
	r = answerTestReader(t, routes, provider)
	if err := r.readAnswers(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 2 {
		t.Fatal("prepared envelope refusal did not divide complete questions")
	}
}

func TestAnswerBatchLocalPreparationCancellationAndPersistenceAreTerminal(t *testing.T) {
	provider := &answerTestProvider{tableProvider: &tableProvider{}}
	localErr := errors.New("invalid local request")
	provider.prepareAnswer = func(answerTestRequest, llm.Prompt) error { return localErr }
	r := answerTestReader(t, answerTestRoutes(1), provider)
	if err := r.readAnswers(context.Background()); !errors.Is(err, localErr) {
		t.Fatalf("local prepare error became unavailable: %v", err)
	}
	provider.prepareAnswer = nil
	provider.invalidState = true
	if err := r.readAnswers(context.Background()); err == nil {
		t.Fatal("invalid provider state became an unavailable answer")
	}
	if len(provider.requests) != 0 {
		t.Fatal("invalid local state reached the provider")
	}
	provider.invalidState = false
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.readAnswers(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation became unavailable: %v", err)
	}
	if err := os.Remove(filepath.Join(r.opts.OwnerRunDir, atlas.TablesDir)); err != nil {
		t.Fatal(err)
	}
	if err := r.readAnswers(context.Background()); err == nil {
		t.Fatal("persistence failure became a successful reading")
	}
}

func TestAnswerBatchIndivisibleProviderResourceFailureIsTerminal(t *testing.T) {
	provider := &answerTestProvider{tableProvider: &tableProvider{}}
	provider.completeAnswer = func(answerTestRequest) ([]byte, error) { return nil, resourceRefusal(llm.ResourceLimitOutputTokens) }
	r := answerTestReader(t, answerTestRoutes(1), provider)
	err := r.readAnswers(context.Background())
	if !answerResourceFailure(err) || len(provider.requests) != 1 {
		t.Fatalf("indivisible refusal did not stop: %v; requests=%d", err, len(provider.requests))
	}
	if len(r.questions[0].Answer.Parts) != 0 {
		t.Fatal("indivisible refusal gained an unavailable semantic part")
	}
}

func TestAnswerConnectionsKeepExactRowsAndOriginalWitnesses(t *testing.T) {
	routes := answerTestRoutes(2)
	routes[0].Stops = append(routes[0].Stops, routes[1].Stops...)
	routes[0].Connections = []atlas.QuestionConnection{{FromID: routes[0].Stops[0].PlaceID, ToID: routes[0].Stops[1].PlaceID, Kind: "calls", Count: 4000, Evidence: &atlas.EdgeEvidence{Path: "schema.sql", LineNo: 8, Label: "producer relation"}, Witnesses: []atlas.Witness{{Caller: "UnrelatedCaller", Callee: "UnrelatedCallee", Path: "source-00.go", LineNo: 500}}}}
	parts := []answerQuestion{{index: 0, candidates: uniqueRouteAnchors(routes[0].Stops), complete: true}, {index: 1, candidates: uniqueRouteAnchors(routes[1].Stops), complete: true}}
	window, err := makeAnswerWindow(lines.Answer(), routes, parts)
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Context struct {
			Connections []struct {
				ResultRows  []string            `json:"result_rows"`
				Count       int                 `json:"count"`
				Declaration *atlas.EdgeEvidence `json:"declaration"`
			} `json:"connections"`
		} `json:"context"`
	}
	if err := json.Unmarshal(window.table.Request, &request); err != nil {
		t.Fatal(err)
	}
	if len(request.Context.Connections) != 1 || !reflect.DeepEqual(request.Context.Connections[0].ResultRows, []string{"r1"}) || request.Context.Connections[0].Count != 4000 || request.Context.Connections[0].Declaration.Path != "schema.sql" {
		t.Fatalf("connection scope broadened: %+v", request)
	}
	if strings.Contains(string(window.table.Request), "UnrelatedCaller") || !strings.Contains(string(window.table.Request), "files_or_observed_entities") || routes[0].Connections[0].Witnesses[0].Caller != "UnrelatedCaller" {
		t.Fatal("compact projection lost original witness authority")
	}
}

func TestAnswerUnavailablePartCannotBecomeAnUnansweredWhole(t *testing.T) {
	routes := answerTestRoutes(1)
	routes[0].Stops = append(routes[0].Stops, atlas.QuestionStop{Path: "other.go", Line: 1, Evidence: map[string]any{"signature": "Other()"}})
	base := &tableProvider{answerFor: func(map[string]any) table.Answer {
		return table.Answer{"state": "unanswered", "answer": "none", "basis": "none", "sources": "none", "remaining": "The available declarations do not answer this question."}
	}}
	provider := &answerTestProvider{tableProvider: base}
	provider.completeAnswer = func(request answerTestRequest) ([]byte, error) {
		if len(request.Context.Candidates) > 1 {
			return nil, resourceRefusal(llm.ResourceLimitContextTokens)
		}
		if request.Context.Candidates[0].Path == "source-00.go" {
			return []byte(`{"rows":[`), nil
		}
		return nil, nil
	}
	r := answerTestReader(t, routes, provider)
	if err := r.readAnswers(context.Background()); err != nil {
		t.Fatal(err)
	}
	answer := r.questions[0].Answer
	if answer.State != "unavailable" || len(answer.Parts) != 2 || r.questions[0].Guide.State != "unavailable" {
		t.Fatalf("uninspected evidence became a negative answer: %+v", answer)
	}
	if answerState([]atlas.QuestionAnswerPart{{State: "unavailable"}, {State: "partial", Text: "Useful supported finding."}}) != "partial" {
		t.Fatal("useful sibling answer lost its partial state")
	}
}
