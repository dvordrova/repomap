package questionbatch

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/llm"
)

func decodeQuestions(t *testing.T, rows, questions int, raw string) (Response, error) {
	t.Helper()
	data, err := prepareCatalogue(testInput(rows, questions), Options{})
	if err != nil {
		t.Fatal(err)
	}
	window := make([]int, rows)
	for i := range window {
		window[i] = i
	}
	return data.decode(window, data.questions, []byte(raw), false)
}

// q1 answers r1 with the selection under test and r2 with a valid one; q2 is
// a valid neighbour. A harmless form difference is accepted, and a defect
// refuses only the (q1,r1) cell.
func TestQuestionCellFormsAreAcceptedAndDefectsRefuseOnlyTheirCell(t *testing.T) {
	const r2 = `{"row":"r2","anchors":["a2"],"relevance":"context","why":"Setup."}`
	const q2 = `{"key":"q2","selections":[]}`
	decode := func(selection string) (Response, error) {
		return decodeQuestions(t, 2, 2, `{"questions":[{"key":"q1","selections":[`+selection+`,`+r2+`]},`+q2+`]}`)
	}
	for _, test := range []struct{ name, selection, why string }{
		{"one anchor written as a string", `{"row":"r1","anchors":"a1","relevance":"direct","why":"Main."}`, "Main."},
		{"relevance in another case", `{"row":"r1","anchors":["a1"],"relevance":" Direct ","why":"Main."}`, "Main."},
		{"blank why", `{"row":"r1","anchors":["a1"],"relevance":"direct","why":" \n "}`, ""},
		{"missing why", `{"row":"r1","anchors":["a1"],"relevance":"direct"}`, ""},
	} {
		t.Run("accepted "+test.name, func(t *testing.T) {
			result, err := decode(test.selection)
			want := []Selection{
				{Row: "r1", Anchors: []string{"a1"}, Relevance: "direct", Why: test.why},
				{Row: "r2", Anchors: []string{"a2"}, Relevance: "context", Why: "Setup."},
			}
			if err != nil || len(result.Rejections) != 0 || len(result.Questions) != 2 || !reflect.DeepEqual(result.Questions[0].Selections, want) {
				t.Fatalf("harmless form was refused or changed: %+v / %v", result, err)
			}
		})
	}
	for _, test := range []struct{ name, selection, reason string }{
		{"unknown anchors", `{"row":"r1","anchors":["a999"],"relevance":"direct","why":"w"}`, "no advertised anchors"},
		{"unknown relevance", `{"row":"r1","anchors":["a1"],"relevance":"maybe","why":"w"}`, "invalid relevance"},
		{"anchors of another shape", `{"row":"r1","anchors":42,"relevance":"direct","why":"w"}`, "invalid selection shape"},
		{"missing relevance", `{"row":"r1","anchors":["a1"],"why":"w"}`, "invalid selection shape"},
		{"why of another shape", `{"row":"r1","anchors":["a1"],"relevance":"direct","why":7}`, "invalid selection shape"},
		{"one anchor as direct and context", `{"row":"r1","anchors":["a1"],"relevance":"direct","why":"w"},{"row":"r1","anchors":["a1"],"relevance":"context","why":"w"}`, "conflicting selection"},
	} {
		t.Run("refused "+test.name, func(t *testing.T) {
			result, err := decode(test.selection)
			if err != nil || len(result.Questions) != 2 || len(result.Rejections) != 1 {
				t.Fatalf("defect refused more than its cell: %+v / %v", result, err)
			}
			rejection := result.Rejections[0]
			if rejection.Question != "q1" || !reflect.DeepEqual(rejection.Rows, []string{"r1"}) || rejection.Chunks != 1 || !strings.Contains(rejection.Reason, test.reason) {
				t.Fatalf("rejection = %+v", rejection)
			}
			if got := result.Questions[0].Selections; len(got) != 1 || got[0].Row != "r2" {
				t.Fatalf("the question's other row lost its decision: %+v", got)
			}
		})
	}
	// A selection without a readable known row decides and refuses nothing.
	for _, selection := range []string{`{"anchors":["a1"],"relevance":"direct","why":"w"}`, `{"row":"r9","anchors":["a1"],"relevance":"direct","why":"w"}`, `"r1"`} {
		result, err := decode(selection)
		if err != nil || len(result.Rejections) != 0 || len(result.Discarded) != 1 || len(result.Questions[0].Selections) != 1 {
			t.Fatalf("selection %s was not discarded and recorded: %+v / %v", selection, result, err)
		}
		if kinds := result.ResponseRejections(); len(kinds) != 1 || kinds[0].Kind != "selection_discarded" {
			t.Fatalf("discarded selection was not journaled: %+v", kinds)
		}
	}
}

// The same answer twice is one answer, cell by cell: rows the entries agree
// on are decided, and only a row they answer differently is refused.
func TestRepeatedQuestionEntriesAreComparedCellByCell(t *testing.T) {
	const agreeing = `{"row":"r1","anchors":["a1"],"relevance":"direct","why":"First hint."}`
	result, err := decodeQuestions(t, 2, 1, `{"questions":[`+
		`{"key":"q1","selections":[`+agreeing+`,{"row":"r2","anchors":["a1"],"relevance":"context","why":"w"}]},`+
		`{"key":"q1","selections":[{"row":"r1","anchors":["a1"],"relevance":"direct","why":"Second hint."}]}]}`)
	if err != nil || len(result.Questions) != 1 || len(result.Rejections) != 1 || !reflect.DeepEqual(result.Rejections[0].Rows, []string{"r2"}) {
		t.Fatalf("disagreement on r2 refused more or less than its cell: %+v / %v", result, err)
	}
	want := []Selection{{Row: "r1", Anchors: []string{"a1"}, Relevance: "direct", Why: "First hint.\n\nSecond hint."}}
	if !reflect.DeepEqual(result.Questions[0].Selections, want) {
		t.Fatalf("agreeing cell lost its decision or a hint: %+v", result.Questions[0].Selections)
	}
	// Different relevance for the same anchor in two entries is refused too.
	result, err = decodeQuestions(t, 1, 2, `{"questions":[{"key":"q1","selections":[`+agreeing+`]},{"key":"q1","selections":[{"row":"r1","anchors":["a1"],"relevance":"context","why":"w"}]},{"key":"q2","selections":[]}]}`)
	if err != nil || len(result.Questions) != 1 || result.Questions[0].Key != "q2" || len(result.Rejections) != 1 || result.Rejections[0].Question != "q1" || result.Rejections[0].Omitted {
		t.Fatalf("a question refused in its only cell = %+v / %v", result, err)
	}
}

// The saved Freqtrade shape: one question selected 18 rows and one of them
// named an anchor its row did not advertise. The other 17 survive.
func TestUnknownAnchorsInOneRowKeepTheQuestionsOtherSelections(t *testing.T) {
	var selections []string
	for row := 1; row <= 18; row++ {
		anchor := "a1"
		if row == 11 {
			anchor = "a4661"
		}
		selections = append(selections, fmt.Sprintf(`{"row":"r%d","anchors":[%q],"relevance":"direct","why":"w"}`, row, anchor))
	}
	result, err := decodeQuestions(t, 18, 1, `{"questions":[{"key":"q1","selections":[`+strings.Join(selections, ",")+`]}]}`)
	if err != nil || len(result.Questions) != 1 || len(result.Questions[0].Selections) != 17 {
		t.Fatalf("one unknown anchor cost the question its other selections: %+v / %v", result, err)
	}
	if len(result.Rejections) != 1 || !reflect.DeepEqual(result.Rejections[0].Rows, []string{"r11"}) || !strings.Contains(result.Rejections[0].Reason, "a4661") {
		t.Fatalf("the unknown anchor was not recorded at its cell: %+v", result.Rejections)
	}
}

func TestQuestionEntriesAcceptWrapperForms(t *testing.T) {
	const entry = `{"key":"q1","selections":[{"row":"r1","anchors":["a1"],"relevance":"direct","why":"w"}]}`
	for _, raw := range []string{`[` + entry + `]`, `{"result":{"questions":[` + entry + `]}}`, `{"questions":[` + entry + `],"note":"extra"}`} {
		result, err := decodeQuestions(t, 1, 1, raw)
		if err != nil || len(result.Questions) != 1 || len(result.Questions[0].Selections) != 1 {
			t.Fatalf("%s: %+v / %v", raw, result, err)
		}
	}
	for _, raw := range []string{`{"questions":null}`, `{"result":{"questions":null}}`, `{"answers":[` + entry + `]}`, `{"result":{"questions":[` + entry + `]},"note":"x"}`} {
		if result, err := decodeQuestions(t, 1, 1, raw); err == nil {
			t.Fatalf("%s: accepted without a questions array: %+v", raw, result)
		}
	}
}

// An entry with null or missing selections made no decision: it is asked
// once more over the same rows, and a second such answer is refused.
func TestNullSelectionsAreAnOmissionAskedOnce(t *testing.T) {
	nullFor := func(times int) func(modelRequest) (Response, error) {
		var mu sync.Mutex
		asked := 0
		return func(request modelRequest) (Response, error) {
			response := selectFirst(request, "Inspect the original declaration.")
			mu.Lock()
			defer mu.Unlock()
			for i := range response.Questions {
				if response.Questions[i].Key == "q1" && asked < times {
					asked++
					response.Questions[i].Selections = nil
				}
			}
			return response, nil
		}
	}
	provider := &testProvider{complete: nullFor(1)}
	result, err := Run(t.Context(), llm.Executor{}, provider, testInput(2, 2), Options{})
	if err != nil || len(result.Exchanges) != 2 || !result.Exchanges[1].Reask || !reflect.DeepEqual(result.Exchanges[1].QuestionRefs, []string{"q1"}) {
		t.Fatalf("null selections were not re-asked once: %+v / %v", result.Exchanges, err)
	}
	if rejection := result.Exchanges[0].Outcome.Value.Rejections[0]; !rejection.Omitted || !rejection.Recovered || !strings.Contains(rejection.Reason, "missing selections for q1") {
		t.Fatalf("first-round rejection = %+v", rejection)
	}
	for row := range 2 {
		if !result.Questions[0].Chunks[row].Inspected {
			t.Fatalf("the re-asked question lost row %d", row)
		}
	}
	provider = &testProvider{complete: nullFor(2)}
	result, err = Run(t.Context(), llm.Executor{}, provider, testInput(2, 2), Options{})
	if err != nil || len(result.Exchanges) != 2 || result.Exchanges[1].Err == nil || !strings.Contains(result.Exchanges[1].Err.Error(), "missing selections for q1") {
		t.Fatalf("a second answer without selections was not refused: %+v / %v", result.Exchanges, err)
	}
	for row := range 2 {
		if result.Questions[0].Chunks[row].Inspected || !result.Questions[1].Chunks[row].Inspected {
			t.Fatalf("coverage at row %d: %+v", row, result.Questions)
		}
	}
}

// A response that decided no question still said which ones it omitted.
// They are asked again in a request without the refused question; a window
// whose every question was omitted is not repeated byte for byte.
func TestRefusedWindowAsksItsOmittedQuestionsAgain(t *testing.T) {
	provider := &testProvider{complete: func(request modelRequest) (Response, error) {
		response := selectFirst(request, "Inspect the original declaration.")
		kept := []Decision{}
		for _, decision := range response.Questions {
			switch {
			case decision.Key == "q1":
				decision.Selections[0].Anchors = []string{"a999"}
				kept = append(kept, decision)
			case len(request.Questions) == 1:
				kept = append(kept, decision)
			}
		}
		response.Questions = kept
		return response, nil
	}}
	result, err := Run(t.Context(), llm.Executor{}, provider, testInput(1, 2), Options{})
	if err != nil || len(result.Exchanges) != 2 {
		t.Fatalf("exchanges = %+v / %v", result.Exchanges, err)
	}
	first, second := result.Exchanges[0], result.Exchanges[1]
	if first.Err == nil || !strings.Contains(first.Err.Error(), "no questions accepted") {
		t.Fatalf("a response that decided nothing was accepted: %v", first.Err)
	}
	if !second.Reask || second.Err != nil || !reflect.DeepEqual(second.QuestionRefs, []string{"q2"}) {
		t.Fatalf("the omitted question was not asked again alone: %+v / %v", second, second.Err)
	}
	if result.Questions[0].Chunks[0].Inspected || !result.Questions[1].Chunks[0].Inspected {
		t.Fatalf("coverage = %+v", result.Questions)
	}

	silent := &testProvider{complete: func(modelRequest) (Response, error) { return Response{Questions: []Decision{}}, nil }}
	result, err = Run(t.Context(), llm.Executor{}, silent, testInput(1, 2), Options{})
	if err != nil || len(result.Exchanges) != 1 || result.Exchanges[0].Err == nil || len(silent.requests) != 1 {
		t.Fatalf("a window that omitted every question was repeated: %+v / %v", result.Exchanges, err)
	}
}

// A refused cell is asked again on the next run in its own smaller window,
// and both windows are then recalled without another provider call.
func TestRefusedCellIsAskedAgainOnTheNextRunAndRemembered(t *testing.T) {
	input := testInput(2, 2)
	provider := &testProvider{complete: func(request modelRequest) (Response, error) {
		response := selectFirst(request, "Inspect the original declaration.")
		if len(request.Evidence) == 2 {
			response.Questions[0].Selections[0].Anchors = []string{"a999"}
		}
		return response, nil
	}}
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
	first, err := Run(t.Context(), executor, provider, input, Options{})
	if err != nil || len(provider.requests) != 1 || first.Questions[0].Chunks[0].Inspected || !first.Questions[0].Chunks[1].Inspected {
		t.Fatalf("first run: %+v / %v", first.Questions, err)
	}
	second, err := Run(t.Context(), executor, provider, input, Options{})
	if err != nil || len(provider.requests) != 2 {
		t.Fatalf("the refused cell was not asked again in its own window: %d calls, %v", len(provider.requests), err)
	}
	var asked modelRequest
	if err := json.Unmarshal([]byte(provider.requests[1].Prompt.User), &asked); err != nil || len(asked.Evidence) != 1 || len(asked.Questions) != 1 || asked.Questions[0].Key != "q1" {
		t.Fatalf("second request = %+v / %v", asked, err)
	}
	third, err := Run(t.Context(), executor, provider, input, Options{})
	if err != nil || len(provider.requests) != 2 {
		t.Fatalf("remembered windows were not recalled: %d calls, %v", len(provider.requests), err)
	}
	for _, result := range []Result{second, third} {
		for q, question := range result.Questions {
			for row, chunk := range question.Chunks {
				if !chunk.Inspected {
					t.Fatalf("cell %d/%d stayed unavailable after its own window: %+v", q, row, chunk)
				}
			}
		}
	}
	if chunk := third.Questions[0].Chunks[0]; chunk.Source != atlas.SourceCache || len(chunk.Selections) != 1 || chunk.Selections[0].Row != "r1" {
		t.Fatalf("recalled cell = %+v", chunk)
	}
}

// Two windows never both decide one cell: the later one is left unused rather
// than choosing between two model answers.
func TestApplyNeverOverwritesADecidedCell(t *testing.T) {
	data, err := prepareCatalogue(testInput(2, 1), Options{})
	if err != nil {
		t.Fatal(err)
	}
	question := QuestionResult{Chunks: make([]ChunkResult, 2)}
	first := llm.Outcome[Response]{Value: Response{
		Questions:  []Decision{{Key: "q1", Selections: []Selection{}}},
		Rejections: []QuestionRejection{{Question: "q1", Rows: []string{"r1"}, Chunks: 1}},
	}}
	if !data.apply(&question, []int{0, 1}, "q1", first) || question.Chunks[0].Inspected || !question.Chunks[1].Inspected {
		t.Fatalf("first window = %+v", question.Chunks)
	}
	overlapping := llm.Outcome[Response]{Value: Response{Questions: []Decision{{Key: "q1", Selections: []Selection{{Row: "r2", Anchors: []string{"a1"}, Relevance: "direct"}}}}}}
	if data.apply(&question, []int{0, 1}, "q1", overlapping) || question.Chunks[0].Inspected || len(question.Chunks[1].Selections) != 0 {
		t.Fatalf("a second decision overwrote a decided cell: %+v", question.Chunks)
	}
	refusedCell := llm.Outcome[Response]{Value: Response{Questions: []Decision{{Key: "q1", Selections: []Selection{{Row: "r1", Anchors: []string{"a1"}, Relevance: "direct"}}}}}}
	if !data.apply(&question, []int{0}, "q1", refusedCell) || !question.Chunks[0].Inspected || len(question.Chunks[0].Selections) != 1 {
		t.Fatalf("the refused cell's own window was not applied: %+v", question.Chunks)
	}
}
