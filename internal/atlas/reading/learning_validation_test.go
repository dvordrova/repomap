package reading

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/llm"
)

func TestLearningMalformedIntentKeepsSiblingReviewsAndExactCache(t *testing.T) {
	pool := learningRequest{Evidence: []learningEvidence{{Ref: "e1"}}}
	reply := learningReply()
	raw, _ := json.Marshal(reply)
	var envelope map[string][]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope["reviews"][0] = json.RawMessage(`{"intent":"purpose","questions":7}`)
	raw, _ = json.Marshal(envelope)
	provider := &learningRawProvider{response: raw}
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
	call, err := learningCall(pool, learningPrompt)
	if err != nil {
		t.Fatal(err)
	}
	for run := 0; run < 2; run++ {
		outcome, err := llm.ExecuteJSON(t.Context(), executor, provider, call)
		if err != nil || outcome.Cached != (run == 1) || len(outcome.Value.Reviews) != 7 || len(outcome.Value.Rejections) != 1 || outcome.Value.Rejections[0].Intent != "purpose" || slices.Contains(outcome.Value.AcceptedRowKeys(), "purpose") {
			t.Fatalf("malformed intent changed siblings on run %d: %+v, %v", run, outcome.Value, err)
		}
		if !reflect.DeepEqual(outcome.Value.Reviews[2], reply.Reviews[3]) || string(outcome.Response) != string(raw) {
			t.Fatal("partial acceptance changed a valid review or the original response")
		}
	}
	if provider.calls != 1 {
		t.Fatal("accepted sibling reviews were requested again")
	}
}

type learningRawProvider struct {
	learningProvider
	response []byte
}

func (p *learningRawProvider) Complete(_ context.Context, _ llm.Prepared) (llm.Completion, error) {
	p.calls++
	return llm.Completion{Response: p.response, FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, nil
}

func TestLearningRefusedMergeKeepsOriginalQuestionsWithoutInventingLinks(t *testing.T) {
	provider := &learningProvider{refuseMerge: true}
	r := isolatedLearningReader(t, t.TempDir(), provider)
	questions := []atlas.LearningQuestion{
		{Question: "How does Alpha work?", Origins: []atlas.LearningOrigin{{Intent: "purpose", Question: "How does Alpha work?", Why: "Original Alpha reason."}}},
		{Question: "How does Beta work?", Origins: []atlas.LearningOrigin{{Intent: "data", Question: "How does Beta work?", Why: "Original Beta reason."}}},
		{Question: "How does Gamma work?", Origins: []atlas.LearningOrigin{{Intent: "run", Question: "How does Gamma work?", Why: "Original Gamma reason."}}},
	}
	r.learning = &atlas.LearningPlan{State: "ready", Questions: questions}
	if err := r.mergeLearning(t.Context()); err != nil {
		t.Fatal(err)
	}
	if r.learning.State != "partial" || !reflect.DeepEqual(r.learning.Questions, questions) || len(r.rejected) == 0 {
		t.Fatal("invalid representatives erased, rewrote or joined original questions")
	}
}

func TestLearningQuestionReviewNeedsItsOwnReasonApartFromQuestionWhy(t *testing.T) {
	pool := learningRequest{Evidence: []learningEvidence{{Ref: "e1"}}}
	reply := learningReply()
	reply.Reviews[0].State = "questions"
	reply.Reviews[0].Reason = ""
	reply.Reviews[0].Questions = []learningProposal{{Question: "How does the service start?", Why: "A newcomer needs the startup path.", Sources: []string{"e1"}}}
	raw, _ := json.Marshal(reply)
	decoded, err := decodeLearning(raw, pool)
	if err != nil || len(decoded.Reviews) != 7 || len(decoded.Rejections) != 1 || decoded.Rejections[0].Intent != reply.Reviews[0].Intent || decoded.Rejections[0].Reason != "learn: a review needs a reason" {
		t.Fatalf("a question's reason substituted for its review or refused siblings: %+v / %v", decoded, err)
	}
	call, err := learningCall(pool, learningPrompt)
	if err != nil {
		t.Fatal(err)
	}
	for _, instruction := range []string{"Every review must contain a nonempty", "including a review whose state is `questions`", "per-question\nreasons do not replace the review's reason"} {
		if !strings.Contains(call.Prompt.System, instruction) {
			t.Fatalf("actual proposal prompt omits reason contract %q", instruction)
		}
	}
}
