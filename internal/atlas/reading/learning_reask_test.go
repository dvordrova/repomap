package reading

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/modeldiag"
)

func learningIntentIDs(intents []learningIntent) []string {
	ids := make([]string, 0, len(intents))
	for _, intent := range intents {
		ids = append(ids, intent.ID)
	}
	return ids
}

// learningOmittingProvider answers each Learn window with an unknown review
// for the asked intents its script names and leaves the others out, the way
// the Morfeu 20260911 windows came back with the purpose review only.
type learningOmittingProvider struct {
	mu       sync.Mutex
	requests []learningRequest
	// review returns the intent IDs to review in the call-th request.
	review func(call int, asked []string) []string
}

func (*learningOmittingProvider) State() []byte {
	return []byte(`{"provider":"learning-omitting-test"}`)
}

func (*learningOmittingProvider) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	raw, err := json.Marshal(prompt)
	if err != nil {
		return llm.Prepared{}, err
	}
	return llm.NewPrepared(raw)
}

func (p *learningOmittingProvider) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var prompt llm.Prompt
	if err := json.Unmarshal(prepared.Bytes(), &prompt); err != nil {
		return llm.Completion{}, err
	}
	var pool learningRequest
	if err := json.Unmarshal([]byte(prompt.User), &pool); err != nil {
		return llm.Completion{}, err
	}
	p.mu.Lock()
	call := len(p.requests)
	p.requests = append(p.requests, pool)
	p.mu.Unlock()
	response := learningResponse{Reviews: []learningReview{}}
	for _, id := range p.review(call, learningIntentIDs(pool.Intents)) {
		response.Reviews = append(response.Reviews, learningReview{Intent: id, State: "unknown", Reason: "The original evidence needs further reading.", Sources: []string{"e1"}})
	}
	raw, err := json.Marshal(response)
	if err != nil {
		return llm.Completion{}, err
	}
	return llm.Completion{Response: raw, FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, nil
}

// learningReaskRun runs the Learn stage over the test graph and returns the
// reader with the console messages it reported.
func learningReaskRun(t *testing.T, cache string, provider llm.Provider, journal *debugdump.Writer) (*reader, []string) {
	t.Helper()
	var messages []string
	r := isolatedLearningReader(t, cache, provider)
	if journal != nil {
		r.opts.Executor.Observer = debugdump.NewSemanticObserver(journal)
	}
	r.opts.State = func(stage, state string, details ...string) {
		messages = append(messages, stage+": "+state+"\n"+strings.Join(details, "\n"))
	}
	if err := r.readLearning(t.Context()); err != nil {
		t.Fatal(err)
	}
	return r, messages
}

func learningWindowResult(t *testing.T, r *reader, window int) learningResponse {
	t.Helper()
	saved, err := os.ReadFile(filepath.Join(r.opts.OwnerRunDir, atlas.TablesDir, fmt.Sprintf("atlas_learn-r0-w%d.result.json", window)))
	var result learningResponse
	if err != nil || json.Unmarshal(saved, &result) != nil {
		t.Fatalf("window %d result: %v", window, err)
	}
	return result
}

// Morfeu 20260911 with a 524,288-token provider window: Learn windows w4 and
// w7 came back with the purpose review only and the other seven intents
// stayed unavailable. An intent an accepted response did not name at all is
// asked again over the same evidence, the omitted intents together and then
// each still-omitted one alone. The first ask's omissions reach the shared
// journal as intent_omitted, never as a loss, carry their recovered mark in
// the window's result, and the console says how many were re-asked.
func TestLearningReasksOmittedIntentsTogetherThenAlone(t *testing.T) {
	intents := learningIntentIDs(learningIntents())
	provider := &learningOmittingProvider{review: func(call int, asked []string) []string {
		switch call {
		case 0:
			return []string{"purpose"}
		case 1:
			return slices.DeleteFunc(slices.Clone(asked), func(id string) bool { return id == "failures" || id == "change" })
		default:
			return asked
		}
	}}
	base := t.TempDir()
	writer, err := debugdump.NewWriter(base, "reading")
	if err != nil {
		t.Fatal(err)
	}
	cache := t.TempDir()
	r, messages := learningReaskRun(t, cache, provider, writer)
	want := [][]string{intents, intents[1:], {"failures"}, {"change"}}
	if len(provider.requests) != len(want) {
		t.Fatalf("%d requests, want %d", len(provider.requests), len(want))
	}
	for i, request := range provider.requests {
		if !reflect.DeepEqual(learningIntentIDs(request.Intents), want[i]) || len(request.Evidence) != len(provider.requests[0].Evidence) || request.PartialContext {
			t.Fatalf("request %d asked %v over %d evidence items, want %v over the first ask's evidence", i+1, learningIntentIDs(request.Intents), len(request.Evidence), want[i])
		}
	}
	windows := map[string]int{}
	for _, review := range r.learning.Reviews {
		if review.State == "unavailable" {
			t.Fatalf("a recovered intent stayed unavailable: %+v", review)
		}
		windows[review.Intent] = review.Window
	}
	if r.learning.State != "ready" || len(r.learning.Reviews) != len(intents) || windows["purpose"] != 1 || windows["run"] != 2 || windows["failures"] != 3 || windows["change"] != 4 {
		t.Fatalf("plan: state %q, windows %v", r.learning.State, windows)
	}
	if use := r.use(stageLearn); len(r.rejected) != 0 || use.Windows != 4 || use.Rows != 17 || use.Live != 4 || use.Rejected != 0 || use.Given != 0 {
		t.Fatalf("use: %+v, rejected %+v", use, r.rejected)
	}
	for window, omitted := range map[int][]string{1: intents[1:], 2: {"failures", "change"}} {
		var got []string
		for _, rejection := range learningWindowResult(t, r, window).Rejections {
			if !rejection.Omitted || !rejection.Recovered || !strings.HasPrefix(rejection.Reason, "learn: missing intent review") {
				t.Fatalf("window %d rejection: %+v", window, rejection)
			}
			got = append(got, rejection.Intent)
		}
		if !reflect.DeepEqual(got, omitted) {
			t.Fatalf("window %d omitted %v, want %v", window, got, omitted)
		}
	}
	journal := r.tables.String()
	for _, line := range []string{
		"## atlas_learn · window 2 · model\n\nRe-asks only the intents an earlier response over this evidence omitted.",
		"- Intent run omitted by the model and re-asked: recovered",
		"- Intent change omitted by the model and re-asked: recovered",
	} {
		if !strings.Contains(journal, line) {
			t.Fatalf("tables journal lacks %q:\n%s", line, journal)
		}
	}
	log := strings.Join(messages, "\n")
	if !strings.Contains(log, "atlas_learn: re-asked\nre-asked 7 intents omitted by the model in 3 windows; 7 recovered") || strings.Contains(log, "still unavailable") {
		t.Fatalf("console:\n%s", log)
	}
	rows, err := modeldiag.Read(filepath.Join(base, "reading"))
	kinds := map[string]int{}
	for _, row := range rows {
		kinds[row.Kind] += row.Count
	}
	if err != nil || !reflect.DeepEqual(kinds, map[string]int{"intent_omitted": 9}) {
		t.Fatalf("shared journal: %v / %v", kinds, err)
	}
	calls := len(provider.requests)
	warm, _ := learningReaskRun(t, cache, provider, nil)
	if len(provider.requests) != calls || warm.use(stageLearn).Cached != 4 || warm.learning.State != "ready" || len(warm.learning.Reviews) != len(intents) {
		t.Fatalf("warm run did not replay the re-asks from the cache: %d requests, %+v", len(provider.requests), warm.use(stageLearn))
	}
}

// An intent the model leaves out three times stays unavailable: in the first
// ask, in the second round beside the other omitted intent, and alone in the
// third, whose refused window is the loss the journal counts. Nothing is
// asked a fourth time.
func TestLearningIntentOmittedThreeTimesStaysUnavailable(t *testing.T) {
	provider := &learningOmittingProvider{review: func(call int, asked []string) []string {
		switch call {
		case 0:
			return slices.DeleteFunc(slices.Clone(asked), func(id string) bool { return id == "failures" || id == "change" })
		case 1:
			return []string{"failures"}
		default:
			return nil
		}
	}}
	r, messages := learningReaskRun(t, t.TempDir(), provider, nil)
	want := [][]string{learningIntentIDs(learningIntents()), {"failures", "change"}, {"change"}}
	if len(provider.requests) != len(want) {
		t.Fatalf("%d requests, want %d", len(provider.requests), len(want))
	}
	for i, request := range provider.requests {
		if !reflect.DeepEqual(learningIntentIDs(request.Intents), want[i]) {
			t.Fatalf("request %d asked %v, want %v", i+1, learningIntentIDs(request.Intents), want[i])
		}
	}
	var unavailable []atlas.LearningReview
	windows := map[string]int{}
	for _, review := range r.learning.Reviews {
		if review.State == "unavailable" {
			unavailable = append(unavailable, review)
			continue
		}
		windows[review.Intent] = review.Window
	}
	if r.learning.State != "partial" || len(unavailable) != 1 || unavailable[0].Intent != "change" || unavailable[0].Window != 1 ||
		!strings.HasPrefix(unavailable[0].Reason, "learn: missing intent review") || windows["failures"] != 2 || len(windows) != 7 {
		t.Fatalf("plan: state %q, unavailable %+v, windows %v", r.learning.State, unavailable, windows)
	}
	if use := r.use(stageLearn); len(r.rejected) != 1 || r.rejected[0].Kind != "window_rejected" || r.rejected[0].Count != 1 ||
		!strings.Contains(r.rejected[0].Reason, "learn: no intent reviews accepted; 1 of 1 intents missing") ||
		use.Windows != 3 || use.Rows != 11 || use.Rejected != 1 || use.Given != 1 {
		t.Fatalf("use: %+v, rejected %+v", use, r.rejected)
	}
	first, second := learningWindowResult(t, r, 1), learningWindowResult(t, r, 2)
	if len(first.Rejections) != 2 || !first.Rejections[0].Recovered || first.Rejections[1].Recovered ||
		len(second.Rejections) != 1 || second.Rejections[0].Intent != "change" || !second.Rejections[0].Omitted || second.Rejections[0].Recovered {
		t.Fatalf("results: %+v / %+v", first.Rejections, second.Rejections)
	}
	if journal := r.tables.String(); !strings.Contains(journal, "- Intent failures omitted by the model and re-asked: recovered") ||
		!strings.Contains(journal, "- Intent change omitted by the model and re-asked: still unavailable") {
		t.Fatalf("tables journal:\n%s", journal)
	}
	if log := strings.Join(messages, "\n"); !strings.Contains(log, "re-asked 2 intents omitted by the model in 2 windows; 1 recovered, 1 still unavailable") {
		t.Fatalf("console:\n%s", log)
	}
}

// Morfeu 20260911, windows w5 and w6: no review at all. A first ask that
// reviewed none of its intents is refused as before, and its intents are
// then asked one at a time; a second round together would repeat the
// refused request byte for byte.
func TestLearningRefusedFirstAskIsReaskedOneIntentAtATime(t *testing.T) {
	provider := &learningOmittingProvider{review: func(call int, asked []string) []string {
		if call == 0 {
			return nil
		}
		return asked
	}}
	r, messages := learningReaskRun(t, t.TempDir(), provider, nil)
	intents := learningIntentIDs(learningIntents())
	if len(provider.requests) != 1+len(intents) {
		t.Fatalf("%d requests, want %d", len(provider.requests), 1+len(intents))
	}
	for i, request := range provider.requests[1:] {
		if !reflect.DeepEqual(learningIntentIDs(request.Intents), []string{intents[i]}) {
			t.Fatalf("re-ask %d asked %v", i+1, learningIntentIDs(request.Intents))
		}
	}
	if r.learning.State != "ready" || len(r.learning.Reviews) != len(intents) || slices.ContainsFunc(r.learning.Reviews, func(review atlas.LearningReview) bool {
		return review.State == "unavailable" || review.Window != slices.Index(intents, review.Intent)+2
	}) {
		t.Fatalf("plan: state %q, %+v", r.learning.State, r.learning.Reviews)
	}
	if len(r.rejected) != 1 || r.rejected[0].Kind != "window_rejected" || r.rejected[0].Count != len(intents) ||
		!strings.Contains(r.rejected[0].Reason, fmt.Sprintf("learn: no intent reviews accepted; %d of %d intents missing", len(intents), len(intents))) {
		t.Fatalf("journal: %+v", r.rejected)
	}
	if log := strings.Join(messages, "\n"); !strings.Contains(log, fmt.Sprintf("re-asked %d intents omitted by the model in %d windows; %d recovered", len(intents), len(intents), len(intents))) {
		t.Fatalf("console:\n%s", log)
	}
}

// Two windows over the same evidence with different intent sets share their
// request bytes up to "intents": the system prompt carries no catalogue and
// the intents follow the evidence in the payload, so a re-ask reuses the
// provider's cached prefix of the first ask. The decoder reads exactly the
// asked intents.
func TestLearningRequestSharesItsPrefixAcrossIntentSets(t *testing.T) {
	pool := newLearningPool(learningContextFixture(), false)
	whole, err := learningCall(pool, learningPrompt)
	if err != nil {
		t.Fatal(err)
	}
	subset := pool
	subset.Intents, subset.Reask = pool.Intents[5:], 1
	part, err := learningCall(subset, learningPrompt)
	if err != nil {
		t.Fatal(err)
	}
	at := strings.Index(whole.Prompt.User, `"intents"`)
	if at < 0 || whole.Prompt.System != part.Prompt.System || whole.Prompt.User[:at] != part.Prompt.User[:at] || !strings.Contains(whole.Prompt.User[:at], `"evidence"`) {
		t.Fatalf("requests over the same evidence do not share their prefix:\n%s\n%s", whole.Prompt.User, part.Prompt.User)
	}
	entry, _ := json.Marshal(learningIntents()[0])
	if string(entry) != `{"id":"purpose","title":"Purpose and main parts","guidance":"What problem does this repository solve, for whom, and how do its main parts divide the work? What should a newcomer open first for a concrete need?"}` ||
		!strings.Contains(whole.Prompt.User[at:], string(entry)) || strings.Contains(part.Prompt.User, `"id":"purpose"`) {
		t.Fatalf("intent catalogue on the wire: %s", whole.Prompt.User[at:])
	}
	for _, intent := range learningIntents() {
		if strings.Contains(whole.Prompt.System, intent.Title) || strings.Contains(whole.Prompt.System, "\n## ") {
			t.Fatalf("system prompt carries the catalogue: %s", intent.Title)
		}
	}
	if !strings.Contains(whole.Prompt.System, "context refs (h1, h2, …) are\nnot sources") || !strings.Contains(whole.Prompt.System, "exactly one review per listed intent") {
		t.Fatalf("system prompt:\n%s", whole.Prompt.System)
	}
	// A review for an intent this window did not ask is unmatched; an asked
	// intent no review named is omitted, with the unmatched shape beside the
	// gap, and journaled under its own kind.
	reply := learningReply()
	raw, _ := json.Marshal(reply)
	got, err := part.DecodeValidate(raw)
	if err != nil || len(got.Reviews) != 3 || len(got.Rejections) != 0 || got.Reviews[0].Intent != "configuration" {
		t.Fatalf("subset decode: %+v / %v", got, err)
	}
	reply.Reviews = reply.Reviews[:7]
	raw, _ = json.Marshal(reply)
	got, err = part.DecodeValidate(raw)
	if err != nil || len(got.Reviews) != 2 || len(got.Rejections) != 1 || got.Rejections[0].Intent != "change" || !got.Rejections[0].Omitted || !strings.Contains(got.Rejections[0].Reason, "5 unmatched reviews") {
		t.Fatalf("omitted decode: %+v / %v", got, err)
	}
	if rows := got.ResponseRejections(); len(rows) != 1 || rows[0].Kind != "intent_omitted" || rows[0].Samples[0] != "change" {
		t.Fatalf("journal kind: %+v", rows)
	}
	// The last round's omission is final: a window asked one intent and
	// naming no review is refused, with the missing intent named for the
	// executor and the count for the journal.
	single := subset
	single.Intents, single.Reask = pool.Intents[7:], learningReaskRounds
	call, err := learningCall(single, learningPrompt)
	if err != nil {
		t.Fatal(err)
	}
	_, err = call.DecodeValidate([]byte(`{"reviews":[]}`))
	var refusal *learningRefusal
	if !errors.As(err, &refusal) || !reflect.DeepEqual(refusal.Missing, []string{"change"}) || err.Error() != "learn: no intent reviews accepted; 1 of 1 intents missing" {
		t.Fatalf("final omission: %v", err)
	}
}
