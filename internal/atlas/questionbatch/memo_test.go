package questionbatch

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/llm"
)

func TestQuestionMemoReusesAddedQuestionIndependentlyAndRefreshesReplay(t *testing.T) {
	input := testInput(3, 2)
	provider := &testProvider{}
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir(), BatchConcurrency: 4}
	first, err := Run(t.Context(), executor, provider, input, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 1 {
		t.Fatal("initial questions did not share a call")
	}
	data, err := prepareCatalogue(input, Options{})
	if err != nil {
		t.Fatal(err)
	}
	key, err := data.memoIdentity(provider, data.questions[1])
	if err != nil {
		t.Fatal(err)
	}
	memo, found, err := llm.LoadMemo(executor, key, llm.DecodeJSON[questionMemo](nil))
	if err != nil || !found || len(memo.Windows) != 1 {
		t.Fatalf("question reference memo: %#v %v", memo, err)
	}
	raw, _ := json.Marshal(memo)
	for _, forbidden := range []string{"owned_declarations", "signature", "local-subject-id", "Inspect the original declaration.", "selections"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("memo copied evidence/cells: %s", forbidden)
		}
	}
	if memo.Windows[0].QuestionRef != "q2" || len(memo.Windows[0].Questions) != 2 {
		t.Fatal("original batch question metadata missing")
	}

	// A newly added question is the only missing interpretation. Existing
	// questions retain their old q refs instead of being silently renumbered.
	added := input
	added.Questions = append(append([]string{}, input.Questions...), "Additional question?")
	warm, err := Run(t.Context(), executor, provider, added, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 2 || len(warm.Exchanges) != 2 || !warm.Exchanges[0].Reused {
		t.Fatalf("added question reran existing questions: calls=%d exchanges=%d", len(provider.requests), len(warm.Exchanges))
	}
	var request modelRequest
	if err := json.Unmarshal([]byte(provider.requests[1].Prompt.User), &request); err != nil {
		t.Fatal(err)
	}
	if len(request.Questions) != 1 || request.Questions[0].Question != "Additional question?" {
		t.Fatalf("missing question packing: %#v", request.Questions)
	}

	// Exact replay changes the current response behind both original question
	// references. Recalling only original q2 still validates the whole old batch.
	provider.complete = func(request modelRequest) (Response, error) {
		return selectFirst(request, "New reason from the current replay."), nil
	}
	prepared, err := llm.NewPrepared(first.Exchanges[0].Outcome.Request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := llm.ReplayJSON(t.Context(), executor, provider, prepared); err != nil {
		t.Fatal(err)
	}
	onlySecond := input
	onlySecond.Questions = []string{input.Questions[1]}
	for i := range onlySecond.Chunks {
		onlySecond.Chunks[i].Place.ID = "new-owner-local-id"
		anchor := onlySecond.Chunks[i].Anchors["a1"]
		anchor.SubjectID = "new-subject-local-id"
		onlySecond.Chunks[i].Anchors["a1"] = anchor
	}
	before := len(provider.requests)
	refreshed, err := Run(t.Context(), executor, provider, onlySecond, Options{})
	if err != nil {
		t.Fatal(err)
	}
	chunk := refreshed.Questions[0].Chunks[0]
	if len(provider.requests) != before || chunk.Source != atlas.SourceCache || chunk.QuestionRef != "q2" || chunk.Why != "New reason from the current replay." {
		t.Fatalf("memo did not read refreshed exact response: %#v", chunk)
	}
	if len(refreshed.Exchanges) != 1 || !reflect.DeepEqual(refreshed.Exchanges[0].QuestionIndexes, []int{0}) || !reflect.DeepEqual(refreshed.Exchanges[0].QuestionRefs, []string{"q2"}) {
		t.Fatal("recalled audit did not bind old ref to current question index")
	}
	if err := json.Unmarshal(refreshed.Exchanges[0].Input, &request); err != nil || len(request.Questions) != 2 {
		t.Fatal("recalled input lost full original window")
	}

	// A replay that omits another required original question invalidates this
	// entire old window, even when the currently requested q2 is present.
	provider.complete = func(request modelRequest) (Response, error) {
		response := selectFirst(request, "Not complete.")
		response.Questions = response.Questions[1:]
		return response, nil
	}
	if _, err := llm.ReplayJSON(t.Context(), executor, provider, prepared); err != nil {
		t.Fatal(err)
	}
	provider.complete = nil
	before = len(provider.requests)
	revalidated, err := Run(t.Context(), executor, provider, onlySecond, Options{})
	if err != nil || len(provider.requests) != before+1 || len(revalidated.Issues) == 0 {
		t.Fatalf("partial original response was promoted: calls=%d issues=%v err=%v", len(provider.requests)-before, revalidated.Issues, err)
	}
	if revalidated.Questions[0].Chunks[0].Why == "Not complete." {
		t.Fatal("invalid replay became question evidence")
	}
}

func TestQuestionMemoChecksCompletePreparedRequestAndChangedEvidence(t *testing.T) {
	input := testInput(2, 2)
	provider := &testProvider{}
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
	if _, err := Run(t.Context(), executor, provider, input, Options{}); err != nil {
		t.Fatal(err)
	}
	data, err := prepareCatalogue(input, Options{})
	if err != nil {
		t.Fatal(err)
	}
	key, err := data.memoIdentity(provider, data.questions[1])
	if err != nil {
		t.Fatal(err)
	}
	memo, found, err := llm.LoadMemo(executor, key, llm.DecodeJSON[questionMemo](nil))
	if err != nil || !found {
		t.Fatal(err)
	}
	// This is syntactically legitimate metadata for the selected question,
	// but it would reconstruct a different sibling question in the old batch.
	memo.Windows[0].Questions[0].Question = "Changed original sibling?"
	raw, _ := json.Marshal(memo)
	if err := llm.SaveMemo(executor, key, raw); err != nil {
		t.Fatal(err)
	}
	onlySecond := input
	onlySecond.Questions = []string{input.Questions[1]}
	before := len(provider.requests)
	result, err := Run(t.Context(), executor, provider, onlySecond, Options{})
	if err != nil || len(result.Issues) == 0 || len(provider.requests) != before+1 {
		t.Fatalf("memo request mismatch was trusted: %#v %v", result.Issues, err)
	}
	if result.Exchanges[0].Reused {
		t.Fatal("mismatched full request became a reused exchange")
	}

	// A changed source input must invalidate the question basis even when all
	// local source IDs and short anchor refs stayed the same.
	input.Chunks[0].Row.Fields[0].Value = "src/changed.go"
	before = len(provider.requests)
	changed, err := Run(t.Context(), executor, provider, input, Options{})
	if err != nil || len(provider.requests) != before+1 || changed.Exchanges[0].Reused {
		t.Fatalf("changed evidence used old question basis: %v", err)
	}
}
