package reading

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/llm"
)

func TestLearningMalformedIntentKeepsSiblingReviewsAndExactCache(t *testing.T) {
	pool := learningRequest{Evidence: []learningEvidence{{Ref: "e1"}}, Intents: learningIntents()}
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

// Freqtrade run 20260910-144751, Learn window w3: the provider returned eight
// questions reviews with 26 questions and "reason": "" on every review. The
// decoder refused all eight, the window became unavailable and no retry
// followed. A review's reason and its questions' why explain the same
// choice, so the reviews are read and the reason is taken from the first
// accepted why, with the substitution recorded in the plan and the journal.
// The prompt still asks for the reason: request bytes are unchanged.
func TestLearningQuestionReviewWithoutReasonTakesItsFirstQuestionWhy(t *testing.T) {
	pool := learningRequest{Evidence: []learningEvidence{{Ref: "e1"}}, Intents: learningIntents()}
	reply := learningReply()
	for i := range reply.Reviews {
		reply.Reviews[i].State, reply.Reviews[i].Reason = "questions", " \n "
		reply.Reviews[i].Questions = []learningProposal{
			{Question: fmt.Sprintf("How does part %d start?", i), Why: fmt.Sprintf("Part %d owns the startup path. Its options are read first.", i), Sources: []string{"e1"}},
			{Question: fmt.Sprintf("What does part %d keep?", i), Why: "It keeps the state between runs.", Sources: []string{"e1"}},
		}
	}
	raw, _ := json.Marshal(reply)
	decoded, err := decodeLearning(raw, pool)
	if err != nil || len(decoded.Reviews) != len(learningIntents()) || len(decoded.Rejections) != 0 || len(decoded.QuestionRejections) != 0 {
		t.Fatalf("empty reasons refused questions reviews: %+v / %v", decoded, err)
	}
	for i, review := range decoded.Reviews {
		if review.ReasonFrom != "why" || review.Reason != fmt.Sprintf("Part %d owns the startup path", i) || len(review.Questions) != 2 {
			t.Fatalf("review %d did not take its first question's why: %+v", i, review)
		}
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
	r := isolatedLearningReader(t, t.TempDir(), &learningProvider{reply: func() learningResponse { return reply }})
	if err := r.readLearning(t.Context()); err != nil {
		t.Fatal(err)
	}
	if r.learning.State != "ready" || len(r.learning.Reviews) != len(learningIntents()) || len(r.learning.Questions) != 2*len(learningIntents()) || len(r.rejected) != 0 {
		t.Fatalf("plan lost reviews whose reason came from a question: %+v, %d rejected rows", r.learning, len(r.rejected))
	}
	for _, review := range r.learning.Reviews {
		if review.State != "questions" || review.ReasonFrom != "why" || review.Reason == "" {
			t.Fatalf("plan review hides the substitution: %+v", review)
		}
	}
	if !strings.Contains(r.tables.String(), "- Reason for purpose taken from its first question's why") {
		t.Fatalf("tables journal omits the substitution:\n%s", r.tables.String())
	}
	saved, err := os.ReadFile(filepath.Join(r.opts.OwnerRunDir, "learning-plan.json"))
	if err != nil || !strings.Contains(string(saved), `"reason_from": "why"`) {
		t.Fatalf("saved plan hides the substitution: %v", err)
	}
}

// One malformed proposal no longer takes its review's accepted neighbours
// with it: the review keeps the questions that passed, the plan stays ready,
// and the journal names the dropped question by intent, wording and rule.
func TestLearningDropsOneBadQuestionAndKeepsItsReview(t *testing.T) {
	r := isolatedLearningReader(t, t.TempDir(), &learningProvider{reply: func() learningResponse {
		reply := learningReply()
		reply.Reviews[3].Questions[1].Sources = []string{"h1"} // a context ref, not an advertised e ref
		return reply
	}})
	if err := r.readLearning(t.Context()); err != nil {
		t.Fatal(err)
	}
	if r.learning.State != "ready" || len(r.learning.Reviews) != len(learningIntents()) {
		t.Fatalf("one bad question refused its review or the plan: %+v", r.learning)
	}
	data := r.learning.Reviews[3]
	if data.Intent != "data" || data.State != "questions" || data.ReasonFrom != "" || data.Reason != "Two different concepts matter." {
		t.Fatalf("surviving review changed: %+v", data)
	}
	for _, question := range r.learning.Questions {
		if question.Question == "What does a revision identify?" {
			t.Fatal("rejected question reached the plan")
		}
	}
	var rows []string
	for _, row := range r.rejected {
		if row.Stage != stageLearn || row.Kind != "question_rejected" || row.Count != 1 || row.ResponseRef == "" || !reflect.DeepEqual(row.Samples, []string{"data"}) {
			t.Fatalf("unexpected journal row: %+v", row)
		}
		rows = append(rows, row.Reason)
	}
	want := `learn: data question 2 "What does a revision identify?" names no advertised source in [h1]`
	if !reflect.DeepEqual(rows, []string{want}) {
		t.Fatalf("journal rows: %q", rows)
	}
	if !strings.Contains(r.tables.String(), "- Rejected question: "+want) {
		t.Fatalf("tables journal omits the dropped question:\n%s", r.tables.String())
	}
}

// A review whose every proposal fails is refused as before, and its
// rejections still name each proposal's rule; a review that wrote its own
// reason keeps it with nothing recorded; a not_applicable or unknown review
// has no content besides its reason and still needs one; a rejection quotes
// only the beginning of a long question.
func TestLearningReviewRefusalsAfterPerQuestionValidation(t *testing.T) {
	pool := learningRequest{Evidence: []learningEvidence{{Ref: "e1"}}, Intents: learningIntents()}
	own := learningReply()
	raw, _ := json.Marshal(own)
	decoded, err := decodeLearning(raw, pool)
	if err != nil || decoded.Reviews[0].Reason != own.Reviews[0].Reason || decoded.Reviews[0].ReasonFrom != "" || len(decoded.QuestionRejections) != 0 {
		t.Fatalf("a written reason was replaced or marked: %+v / %v", decoded, err)
	}
	long := "What does the revision identify when the lease expires and the state is rewritten by a later revision?"
	for name, test := range map[string]struct {
		change func(*learningResponse)
		intent string // refused intent, or "" when every review is read
		reason string
		rows   []string
	}{
		"every-question-bad": {change: func(reply *learningResponse) {
			reply.Reviews[3].Reason = ""
			reply.Reviews[3].Questions[0].Question = " "
			reply.Reviews[3].Questions[1].Why = ""
		}, intent: "data", reason: "learn: questions review kept none of its 2 proposed questions", rows: []string{
			`learn: data question 1 "" needs wording`,
			`learn: data question 2 "What does a revision identify?" needs a reason`,
		}},
		"no-sources": {change: func(reply *learningResponse) {
			reply.Reviews[0].Questions[0].Sources = nil
		}, intent: "purpose", reason: "learn: questions review kept none of its 1 proposed questions", rows: []string{`learn: purpose question 1 "What does a lease control?" needs original sources`}},
		"unknown-blank-reason": {change: func(reply *learningResponse) { reply.Reviews[1].Reason = " " }, intent: "run", reason: "learn: a review needs a reason"},
		"inapplicable-blank-reason": {change: func(reply *learningResponse) {
			reply.Reviews[1] = learningReview{Intent: "run", State: "not_applicable", Sources: []string{"e1"}}
		}, intent: "run", reason: "learn: a review needs a reason"},
		"long-question-unknown-sources": {change: func(reply *learningResponse) {
			reply.Reviews[3].Questions[1].Question = long
			reply.Reviews[3].Questions[1].Sources = []string{"e9", "e8", "e7", "e6", "e5"}
		}, rows: []string{`learn: data question 2 "What does the revision identify when the lease expires and…" names no advertised source in [e9 e8 e7 e6]`}},
	} {
		t.Run(name, func(t *testing.T) {
			reply := learningReply()
			test.change(&reply)
			raw, _ := json.Marshal(reply)
			decoded, err := decodeLearning(raw, pool)
			if err != nil {
				t.Fatal(err)
			}
			if test.intent == "" {
				if len(decoded.Reviews) != len(learningIntents()) || len(decoded.Rejections) != 0 || len(decoded.Reviews[3].Questions) != 1 || decoded.Reviews[3].Reason != own.Reviews[3].Reason {
					t.Fatalf("a dropped question refused its review: %+v", decoded)
				}
			} else if len(decoded.Reviews) != len(learningIntents())-1 || len(decoded.Rejections) != 1 || decoded.Rejections[0].Intent != test.intent || decoded.Rejections[0].Reason != test.reason {
				t.Fatalf("refusal: %+v", decoded.Rejections)
			}
			var rows []string
			for _, rejection := range decoded.QuestionRejections {
				rows = append(rows, rejection.Reason)
			}
			if !reflect.DeepEqual(rows, test.rows) {
				t.Fatalf("question rejections: %q, want %q", rows, test.rows)
			}
		})
	}
}
