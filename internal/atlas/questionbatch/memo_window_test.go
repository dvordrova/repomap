package questionbatch

import (
	"encoding/json"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/llm"
)

type countingMemoProvider struct {
	*testProvider
	windows atomic.Int64
}

func (p *countingMemoProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	var request modelRequest
	if err := json.Unmarshal([]byte(prompt.User), &request); err == nil && len(request.Evidence) > 0 {
		p.windows.Add(1)
	}
	return p.testProvider.Prepare(prompt, limits)
}

func TestWarmQuestionMemosValidateEachOriginalWindowOnce(t *testing.T) {
	input := testInput(6, 8)
	provider := &countingMemoProvider{testProvider: &testProvider{}}
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir(), BatchConcurrency: 4}
	opts := Options{MaxRows: 2}
	cold, err := Run(t.Context(), executor, provider, input, opts)
	if err != nil || len(provider.requests) != 3 {
		t.Fatalf("cold windows=%d: %v", len(provider.requests), err)
	}
	provider.windows.Store(0)
	warm, err := Run(t.Context(), executor, provider, input, opts)
	if err != nil || len(provider.requests) != 3 || len(warm.Exchanges) != 3 || len(warm.Issues) != 0 {
		t.Fatalf("warm questions did not reuse the complete batch: %+v / %v", warm, err)
	}
	if got := provider.windows.Load(); got != 3 {
		t.Fatalf("8 questions repeated shared-window preparation: got %d, want 3", got)
	}
	for q := range cold.Questions {
		for row := range cold.Questions[q].Chunks {
			cold.Questions[q].Chunks[row].Source = atlas.SourceCache
		}
	}
	if !reflect.DeepEqual(warm.Questions, cold.Questions) {
		t.Fatal("shared-window reuse changed an original question, anchor or provenance")
	}
	for _, exchange := range warm.Exchanges {
		if !exchange.Reused || !reflect.DeepEqual(exchange.QuestionRefs, []string{"q1", "q2", "q3", "q4", "q5", "q6", "q7", "q8"}) {
			t.Fatal("warm window lost its complete original question audit")
		}
	}
}

func TestQuestionMemoValidationCannotAuthorizeDifferentSiblingMetadata(t *testing.T) {
	for _, changed := range []string{"sibling question", "row order"} {
		for _, corrupted := range []int{0, 1} {
			t.Run(changed+"/"+[]string{"invalid first", "invalid after accepted sibling"}[corrupted], func(t *testing.T) {
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
				key, err := data.memoIdentity(provider, data.questions[corrupted])
				if err != nil {
					t.Fatal(err)
				}
				memo, found, err := llm.LoadMemo(executor, key, llm.DecodeJSON[questionMemo](nil))
				if err != nil || !found {
					t.Fatalf("original memo: %v", err)
				}
				// The selected question still matches; another original question in
				// this same request has been altered independently of the request key.
				if changed == "sibling question" {
					memo.Windows[0].Questions[1-corrupted].Question = "Changed original sibling?"
				} else {
					memo.Windows[0].Rows[0], memo.Windows[0].Rows[1] = memo.Windows[0].Rows[1], memo.Windows[0].Rows[0]
				}
				raw, err := json.Marshal(memo)
				if err != nil {
					t.Fatal(err)
				}
				if err := llm.SaveMemo(executor, key, raw); err != nil {
					t.Fatal(err)
				}
				got, err := Run(t.Context(), executor, provider, input, Options{})
				if err != nil || len(got.Issues) != 1 || len(provider.requests) != 2 {
					t.Fatalf("memo mismatch was not isolated: calls=%d issues=%v err=%v", len(provider.requests), got.Issues, err)
				}
				for q := range got.Questions {
					want := atlas.SourceCache
					if q == corrupted {
						want = atlas.SourceModel
					}
					if got.Questions[q].Chunks[0].Source != want {
						t.Fatalf("question %d reused a different input or lost its valid neighbor", q)
					}
				}
			})
		}
	}
}
