package reading

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/llm"
)

// Reconstruct the advertised context independently of the production encoder.
// This is a test assertion, not an alternate product input format.
func expandedLearningTestRequest(t *testing.T, raw []byte) learningRequest {
	t.Helper()
	var wire struct {
		PartialContext bool                      `json:"partial_context"`
		Contexts       map[string]map[string]any `json:"contexts"`
		Evidence       []struct {
			Ref        string         `json:"ref"`
			ContextRef string         `json:"context_ref"`
			Context    map[string]any `json:"context"`
		} `json:"evidence"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	result := learningRequest{PartialContext: wire.PartialContext}
	used := map[string]bool{}
	for i, row := range wire.Evidence {
		if row.Ref != fmt.Sprintf("e%d", i+1) {
			t.Fatalf("nonlocal evidence ref %q", row.Ref)
		}
		if row.ContextRef != "" {
			shared, found := wire.Contexts[row.ContextRef]
			if !found {
				t.Fatalf("missing context %q", row.ContextRef)
			}
			if !used[row.ContextRef] && row.ContextRef != fmt.Sprintf("h%d", len(used)+1) {
				t.Fatal("context refs did not restart in first-appearance order")
			}
			used[row.ContextRef] = true
			for name, value := range shared {
				if name != "components" && name != "area_model_hypothesis" {
					t.Fatalf("original evidence moved into shared context: %s", name)
				}
				if _, duplicate := row.Context[name]; duplicate {
					t.Fatalf("context field %s remains duplicated", name)
				}
				row.Context[name] = value
			}
		}
		result.Evidence = append(result.Evidence, learningEvidence{Ref: row.Ref, Context: row.Context})
	}
	if len(used) != len(wire.Contexts) {
		t.Fatal("window carried context outside its evidence")
	}
	return result
}

func learningContextFixture() []learningEvidence {
	var evidence []learningEvidence
	for i := 0; i < 6; i++ {
		original := map[string]any{"anchor_path": fmt.Sprintf("source%d.py", i), "anchor_line": i + 2,
			"author_doc": fmt.Sprintf("Original section %d.\n\nIts distinct later paragraph.", i)}
		evidence = append(evidence, learningEvidence{Context: map[string]any{
			"components": []string{fmt.Sprintf("Component %d", i/3)}, "ordinal": i,
			"area_model_hypothesis": strings.Repeat("Shared interpreted purpose. ", 30), "original": original,
		}, Source: atlas.QuestionStop{PlaceID: fmt.Sprintf("private-place-%d", i), SubjectID: fmt.Sprintf("private-subject-%d", i),
			Path: fmt.Sprintf("source%d.py", i), Line: i + 2, Column: 3, Kind: "documentation", Name: "Original section",
			TargetIDs: []string{"private-target"}, KnowledgeIDs: []string{"private-knowledge"}, Evidence: original}})
	}
	return evidence
}

func TestLearningRequestSharesContextWithoutChangingOriginals(t *testing.T) {
	evidence := learningContextFixture()
	pool := newLearningPool(evidence, false)
	before, err := json.Marshal(pool)
	if err != nil {
		t.Fatal(err)
	}
	call, err := learningCall(pool, learningPrompt)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(call.Prompt.User)
	if strings.Count(string(raw), `"area_model_hypothesis"`) != 2 || len(raw) >= len(before) {
		t.Fatal("repeated context was not shared once per distinct header")
	}
	for _, forbidden := range []string{"private-place", "private-subject", "private-target", "private-knowledge"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("local provenance leaked: %s", forbidden)
		}
	}
	reconstructed := expandedLearningTestRequest(t, raw)
	got, _ := json.Marshal(reconstructed)
	if string(got) != string(before) {
		t.Fatal("context reconstruction changed or omitted an original entry")
	}
	after, _ := json.Marshal(pool)
	if string(after) != string(before) {
		t.Fatal("encoding mutated the original pool")
	}
	again, err := learningCall(pool, learningPrompt)
	if err != nil || again.Prompt.User != call.Prompt.User {
		t.Fatal("identical inputs produced different request bytes")
	}
	// Explicit development budgets use the compact representation actually sent.
	budget := len(raw) + len(learningPrompt)
	pools, err := learningPools(evidence, budget, learningPrompt)
	if err != nil || len(pools) != 1 {
		t.Fatalf("planner measured repeated headers instead of wire: %v, %d pools", err, len(pools))
	}
	// The encoder also preserves optional-field presence and null values.
	evidence[0].Context["components"] = nil
	delete(evidence[1].Context, "components")
	pool = newLearningPool(evidence, false)
	want, _ := json.Marshal(pool)
	encoded, err := encodeLearningPool(pool)
	if err != nil {
		t.Fatal(err)
	}
	expanded, _ := json.Marshal(expandedLearningTestRequest(t, encoded))
	if string(expanded) != string(want) {
		t.Fatal("missing and null shared fields were conflated")
	}
	evidence[0].Context["components"] = make(chan int)
	if _, err := learningCall(newLearningPool(evidence, false), learningPrompt); err == nil {
		t.Fatal("invalid local context was silently omitted")
	}
}

func TestLearningSharedContextRefsDoNotRejectHTMLHeadings(t *testing.T) {
	pool := newLearningPool([]learningEvidence{{Context: map[string]any{
		"components": []string{"Documentation renderer"},
		"original":   "The document renderer emits h1 for its title and h2 for section headings.",
	}}}, false)
	call, err := learningCall(pool, learningPrompt)
	if err != nil {
		t.Fatal(err)
	}
	reply := learningReply()
	reply.Reviews[0] = learningReview{Intent: "purpose", State: "questions",
		Reason: "The renderer distinguishes h1 titles from h2 section headings.", Sources: []string{"e1"},
		Questions: []learningProposal{{Question: "How does the renderer choose h1 and h2 headings?",
			Why: "Explain the h1 and h2 document hierarchy.", Sources: []string{"h1", "e1"}}}}
	raw, _ := json.Marshal(reply)
	got, err := call.DecodeValidate(raw)
	if err != nil {
		t.Fatalf("supported HTML heading question was rejected: %v", err)
	}
	question := got.Reviews[0].Questions[0]
	if question.Question != reply.Reviews[0].Questions[0].Question ||
		!reflect.DeepEqual(question.Sources, []string{"e1"}) {
		t.Fatal("heading prose changed or context ref gained source authority")
	}
	reply.Reviews[0].Questions[0].Sources = []string{"h1"}
	raw, _ = json.Marshal(reply)
	partial, err := call.DecodeValidate(raw)
	if err != nil || len(partial.Rejections) != 1 || partial.Rejections[0].Intent != "purpose" || slices.Contains(partial.AcceptedRowKeys(), "purpose") {
		t.Fatal("question supported only by a shared context ref was accepted or rejected valid siblings")
	}
}

func TestLearningSharedContextSurvivesResourcePartitionsAndRecall(t *testing.T) {
	evidence := learningContextFixture()
	provider := &learningResourceProvider{refuse: func(pool learningRequest) error {
		if len(pool.Evidence) > 2 {
			return &llm.ResourceLimitError{Kind: llm.ResourceLimitContextTokens}
		}
		return nil
	}}
	cache := t.TempDir()
	run := func() *reader {
		r := isolatedLearningReader(t, cache, provider)
		r.learning = &atlas.LearningPlan{Version: 1, GraphSHA256: r.opts.Graph.SHA256, State: "ready"}
		if err := r.executeLearning(t.Context(), []learningRequest{newLearningPool(evidence, false)}, learningPrompt); err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, review := range r.learning.Reviews {
			if review.Intent != "purpose" {
				continue
			}
			if !review.PartialContext {
				t.Fatal("partition lost its partial scope")
			}
			for _, source := range review.Sources {
				if seen[source.SubjectID] {
					t.Fatal("source retained twice")
				}
				seen[source.SubjectID] = true
				source.Source = ""
				found := false
				for _, original := range evidence {
					found = found || reflect.DeepEqual(source, original.Source)
				}
				if !found {
					t.Fatal("source location, evidence or local identity changed")
				}
			}
		}
		if len(seen) != len(evidence) {
			t.Fatal("partition omitted original evidence")
		}
		return r
	}
	run()
	for _, prompt := range provider.prompts {
		pool := expandedLearningTestRequest(t, []byte(prompt.User))
		for _, item := range pool.Evidence {
			ordinal := int(item.Context["ordinal"].(float64))
			want, _ := json.Marshal(evidence[ordinal].Context)
			got, _ := json.Marshal(item.Context)
			if string(got) != string(want) {
				t.Fatal("child request did not retain the complete original context")
			}
		}
	}
	calls := len(provider.requests)
	warm := run()
	if len(provider.requests) != calls || warm.use(stageLearn).Live != 0 || warm.use(stageLearn).Cached == 0 {
		t.Fatal("regenerated child contexts did not reuse exact accepted requests")
	}
}
