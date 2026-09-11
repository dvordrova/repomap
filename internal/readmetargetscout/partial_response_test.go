package readmetargetscout

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
)

type classificationResponseProvider struct {
	emptyResultProvider
	raw []byte
}

func (provider *classificationResponseProvider) Complete(context.Context, llm.Prepared) (llm.Completion, error) {
	provider.calls.Add(1)
	return llm.Completion{Response: provider.raw, FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, nil
}

func TestClassifierKeepsGoodHypothesesAndFilesWithExactCachedDiagnostics(t *testing.T) {
	repository, _ := testCorpus(t, map[string]string{"README.md": "Use main.go and tools.go.", "main.go": "package main", "tools.go": "package main"})
	compilation, err := compileWithTestHints(t, "sample", repository)
	if err != nil {
		t.Fatal(err)
	}
	mainID, _ := repository.ID("main.go")
	toolID, _ := repository.ID("tools.go")
	raw := []byte(fmt.Sprintf(`{"files":[
 {"file_ref":%q,"score":3,"hypotheses":["  Runs\nthe app.  ",42,""],"confidence":1},
 {"file_ref":%q,"hypotheses":["Builds assets."]},
 {"file_ref":"f999999","hypotheses":"ignored"}, 7
],"notes":"unused"}`, mainID, toolID))
	provider := &classificationResponseProvider{raw: raw}
	var events []llm.Event
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir(), Observer: llm.ObserverFunc(func(event llm.Event) error { events = append(events, event); return nil })}
	for run := range 2 {
		execution, err := Run(t.Context(), executor, provider, compilation)
		if err != nil || execution.UnavailableBatches != 0 || len(execution.Result) != 2 {
			t.Fatalf("accepted catalog: %+v, %v", execution, err)
		}
		if got := execution.Result[0].Classifications[0].Hypotheses; !reflect.DeepEqual(got, []string{"Runs the app."}) {
			t.Fatalf("independent hypothesis lost or guessed: %v", got)
		}
		outcome := execution.Outcomes[0]
		if outcome.Cached != (run == 1) || len(outcome.ResponseRejections) != 4 || !bytes.Equal(outcome.Response, raw) {
			t.Fatalf("exact response/diagnostics: %+v", outcome)
		}
		if !reflect.DeepEqual(outcome.Value.AcceptedRowKeys(), []string{string(toolID)}) {
			t.Fatalf("terms allowed outside a fully accepted file: %v", outcome.Value.AcceptedRowKeys())
		}
	}
	if provider.calls.Load() != 1 || len(events) != 2 || len(events[0].ResponseRejections) != 4 || len(events[1].ResponseRejections) != 4 {
		t.Fatalf("repeated calls/events or lost rejections: %d / %+v", provider.calls.Load(), events)
	}
}

func TestMalformedClassifierDoesNotBlockNativeDiscoveryOrCacheAResult(t *testing.T) {
	repository, _ := testCorpus(t, map[string]string{"README.md": "Run main.go.", "main.go": "package main"})
	compilation, err := compileWithTestHints(t, "sample", repository)
	if err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string]string{
		"wrong object": `{"wrong":"shape"}`,
		"legacy array": `[{"file_ref":"f1","classifications":[{"class":"target_entry","hypotheses":["old shape"]}]}]`,
	} {
		t.Run(name, func(t *testing.T) {
			provider := &classificationResponseProvider{raw: []byte(raw)}
			executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
			for range 2 {
				execution, err := Run(t.Context(), executor, provider, compilation)
				if err != nil || execution.UnavailableBatches != 1 || len(execution.Result) != 0 || len(execution.Result.TargetCandidates()) != 0 || execution.Outcomes[0].Cached {
					t.Fatalf("bad guidance blocked discovery or invented a target: %+v / %v", execution, err)
				}
			}
			if provider.calls.Load() != 2 {
				t.Fatal("malformed response was cached as a successful empty classification")
			}
		})
	}
}
