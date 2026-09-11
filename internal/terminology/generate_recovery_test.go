package terminology

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
)

func recoveryProse() []proseSource {
	return []proseSource{
		{Texts: []string{"Alpha concept."}, Sources: []Source{{Path: "a.py", Line: 3}}, Origin: Origin{RequestSHA256: strings.Repeat("a", 64), Row: "r1"}},
		{Texts: []string{"Beta concept."}, Sources: []Source{{Path: "b.py", Line: 7}}, Origin: Origin{RequestSHA256: strings.Repeat("b", 64), Row: "r2"}},
	}
}

func recoveryCollector(items []proseSource) *Collector {
	collector := NewCollector([]string{"a.py", "b.py"})
	for i, item := range items {
		collector.pending[string(rune('a'+i))] = item
	}
	return collector
}

func TestGenerationResourceMemoKeepsOriginalProseAndWholeParentReplay(t *testing.T) {
	for _, kind := range []llm.ResourceLimitKind{llm.ResourceLimitOutputTokens, llm.ResourceLimitContextTokens, llm.ResourceLimitResponseBytes} {
		t.Run(string(kind), func(t *testing.T) {
			items := recoveryProse()
			executor := llm.Executor{RootDir: t.TempDir(), Enabled: true, BatchConcurrency: 2}
			provider := &testProvider{}
			// Every reply is bound to one complete prepared input. Unknown
			// requests fail the test instead of receiving a generic answer.
			responses := make(map[string]string)
			prepare := func(window []proseSource) llm.Prepared {
				t.Helper()
				call, err := generationCall(window)
				if err != nil {
					t.Fatal(err)
				}
				prepared, err := llm.Prepare(provider, call.Prompt, call.Limits)
				if err != nil {
					t.Fatal(err)
				}
				return prepared
			}
			parent := prepare(items)
			responses[string(parent.Bytes())] = "refuse"
			responses[string(prepare(items[:1]).Bytes())] = `{"terms":[{"name":"Alpha","explanation":"The first concept.","rows":["p1"]}]}`
			responses[string(prepare(items[1:]).Bytes())] = `{"terms":[{"name":"Beta","explanation":"The second concept.","rows":["p1"]}]}`
			provider.complete = func(prepared llm.Prepared) (llm.Completion, error) {
				response, known := responses[string(prepared.Bytes())]
				if !known {
					t.Error("unadvertised glossary request")
					return llm.Completion{}, context.Canceled
				}
				if response == "refuse" {
					return llm.Completion{}, llm.NewResourceLimitError(llm.ResourceLimitError{Kind: kind})
				}
				return completed(response)
			}
			run := func(executor llm.Executor, input []proseSource) []Candidate {
				t.Helper()
				collector := recoveryCollector(input)
				if err := collector.Generate(t.Context(), executor, provider); err != nil {
					t.Fatal(err)
				}
				if len(collector.pending) != len(input) {
					t.Fatal("resource recovery discarded accepted original prose")
				}
				return collector.Snapshot()
			}
			cold := run(executor, items)
			warm := run(executor, items)
			if provider.calls != 3 || len(cold) != 2 || !reflect.DeepEqual(cold, warm) {
				t.Fatalf("warm generation repeated refused parent: calls=%d, cold=%+v warm=%+v", provider.calls, cold, warm)
			}
			// Equal provider text rebinds the complete current local source scope.
			rebound := recoveryProse()
			rebound[0].Sources[0].Line = 19
			got := run(executor, rebound)
			if provider.calls != 3 || got[0].Sources[0].Line != 19 || !reflect.DeepEqual(got[0].Origins, []Origin{items[0].Origin}) {
				t.Fatalf("memo supplied old provenance: %+v", got)
			}
			changed := recoveryProse()
			changed[0].Texts[0] = "Alpha changed concept."
			responses[string(prepare(changed).Bytes())] = "refuse"
			responses[string(prepare(changed[:1]).Bytes())] = responses[string(prepare(items[:1]).Bytes())]
			if len(run(executor, changed)) != 2 || provider.calls != 5 {
				t.Fatalf("changed input inherited old refusal/answer: calls=%d", provider.calls)
			}
			uncached := executor
			uncached.Enabled = false
			if !reflect.DeepEqual(run(uncached, items), cold) || provider.calls != 8 {
				t.Fatalf("NoCache retained refusal or child answers: calls=%d", provider.calls)
			}
			responses[string(parent.Bytes())] = `{"terms":[{"name":"Alpha","explanation":"Whole-parent replay definition.","rows":["p1"]},{"name":"Beta","explanation":"The second concept.","rows":["p2"]}]}`
			if _, err := llm.ReplayJSON(t.Context(), executor, provider, parent); err != nil {
				t.Fatal(err)
			}
			got = run(executor, items)
			if provider.calls != 9 || got[0].Explanation != "Whole-parent replay definition." {
				t.Fatalf("split memo overruled exact parent replay: calls=%d, %+v", provider.calls, got)
			}
		})
	}
}

func TestGenerationIndivisibleResourceRefusalIsOptionalAndNotMemoized(t *testing.T) {
	provider := &testProvider{complete: func(llm.Prepared) (llm.Completion, error) {
		return llm.Completion{}, llm.NewResourceLimitError(llm.ResourceLimitError{Kind: llm.ResourceLimitOutputTokens})
	}}
	executor := llm.Executor{RootDir: t.TempDir(), Enabled: true}
	for range 2 {
		collector := recoveryCollector(recoveryProse()[:1])
		if err := collector.Generate(t.Context(), executor, provider); err != nil || len(collector.pending) != 1 || len(collector.Snapshot()) != 0 {
			t.Fatalf("indivisible refusal changed accepted prose: %+v / %v", collector, err)
		}
	}
	if provider.calls != 2 {
		t.Fatalf("unsplittable request was memoized as splittable: calls=%d", provider.calls)
	}
}
