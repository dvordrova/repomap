package terminology

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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
				call, err := namesCall(window, nil)
				if err != nil {
					t.Fatal(err)
				}
				prepared, err := llm.Prepare(provider, call.Prompt, call.Limits)
				if err != nil {
					t.Fatal(err)
				}
				return prepared
			}
			// The explanation request of these names over this prose.
			explain := func(prose []proseSource, found ...string) string {
				t.Helper()
				call, err := explainCall(prose, gatherNames(prose, found))
				if err != nil {
					t.Fatal(err)
				}
				prepared, err := llm.Prepare(provider, call.Prompt, call.Limits)
				if err != nil {
					t.Fatal(err)
				}
				return string(prepared.Bytes())
			}
			parent := prepare(items)
			responses[string(parent.Bytes())] = "refuse"
			responses[string(prepare(items[:1]).Bytes())] = `{"names":["Alpha"]}`
			responses[string(prepare(items[1:]).Bytes())] = `{"names":["Beta"]}`
			both := `{"terms":[{"ref":"t1","explanation":"The first concept."},{"ref":"t2","explanation":"The second concept."}]}`
			responses[explain(items, "Alpha", "Beta")] = both
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
			decider := everyConcept()
			run := func(executor llm.Executor, input []proseSource) []Candidate {
				t.Helper()
				collector := recoveryCollector(input)
				if err := collector.Generate(t.Context(), executor, provider, decider, ""); err != nil {
					t.Fatal(err)
				}
				if len(collector.pending) != len(input) {
					t.Fatal("resource recovery discarded accepted original prose")
				}
				return collector.Snapshot()
			}
			cold := run(executor, items)
			warm := run(executor, items)
			// The refused parent, its two halves and one explanation request.
			if provider.calls != 4 || len(cold) != 2 || !reflect.DeepEqual(cold, warm) {
				t.Fatalf("warm generation repeated refused parent: calls=%d, cold=%+v warm=%+v", provider.calls, cold, warm)
			}
			// Equal provider text rebinds the complete current local source scope.
			rebound := recoveryProse()
			rebound[0].Sources[0].Line = 19
			got := run(executor, rebound)
			if provider.calls != 4 || got[0].Sources[0].Line != 19 || !reflect.DeepEqual(got[0].Origins, []Origin{items[0].Origin}) {
				t.Fatalf("memo supplied old provenance: %+v", got)
			}
			changed := recoveryProse()
			changed[0].Texts[0] = "Alpha changed concept."
			responses[string(prepare(changed).Bytes())] = "refuse"
			responses[string(prepare(changed[:1]).Bytes())] = responses[string(prepare(items[:1]).Bytes())]
			responses[explain(changed, "Alpha", "Beta")] = both
			if len(run(executor, changed)) != 2 || provider.calls != 7 {
				t.Fatalf("changed input inherited old refusal/answer: calls=%d", provider.calls)
			}
			uncached := executor
			uncached.Enabled = false
			if !reflect.DeepEqual(run(uncached, items), cold) || provider.calls != 11 {
				t.Fatalf("NoCache retained refusal or child answers: calls=%d", provider.calls)
			}
			// A replayed whole-parent answer names Alpha alone; it takes
			// precedence over the remembered split, whose halves name both.
			responses[string(parent.Bytes())] = `{"names":["Alpha"]}`
			responses[explain(items, "Alpha")] = `{"terms":[{"ref":"t1","explanation":"Whole-parent replay definition."}]}`
			if _, err := llm.ReplayJSON(t.Context(), executor, provider, parent); err != nil {
				t.Fatal(err)
			}
			got = run(executor, items)
			if provider.calls != 13 || len(got) != 1 || got[0].Explanation != "Whole-parent replay definition." {
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
		if err := collector.Generate(t.Context(), executor, provider, everyConcept(), ""); err != nil || len(collector.pending) != 1 || len(collector.Snapshot()) != 0 {
			t.Fatalf("indivisible refusal changed accepted prose: %+v / %v", collector, err)
		}
	}
	if provider.calls != 2 {
		t.Fatalf("unsplittable request was memoized as splittable: calls=%d", provider.calls)
	}
}

// glossaryAnswers answers the names step with every name it is asked about
// that the prose writes, and the explanation step with one line per term.
func glossaryAnswers(t *testing.T) *testProvider {
	return &testProvider{complete: func(prepared llm.Prepared) (llm.Completion, error) {
		var request struct {
			Prose []struct {
				Text []string `json:"text"`
			} `json:"prose"`
			Terms []struct {
				Ref string `json:"ref"`
			} `json:"terms"`
		}
		if err := json.Unmarshal([]byte(inputUser(t, prepared)), &request); err != nil {
			return llm.Completion{}, err
		}
		if request.Terms == nil {
			names := []string{}
			for _, row := range request.Prose {
				for _, name := range []string{"Alpha", "Beta"} {
					if strings.Contains(strings.Join(row.Text, " "), name) {
						names = append(names, name)
					}
				}
			}
			raw, err := json.Marshal(map[string]any{"names": names})
			if err != nil {
				return llm.Completion{}, err
			}
			return completed(string(raw))
		}
		terms := []map[string]string{}
		for _, term := range request.Terms {
			terms = append(terms, map[string]string{"ref": term.Ref, "explanation": "The concept the prose names."})
		}
		raw, err := json.Marshal(map[string]any{"terms": terms})
		if err != nil {
			return llm.Completion{}, err
		}
		return completed(string(raw))
	}}
}

// A cache that cannot be written or read back is a notice on the run output,
// not a glossary failure: the executor already answered live, so the
// definitions are the same as with a working cache (review A7: a recovered
// cache issue in this optional stage once stopped the report before its
// HTML). A required run artifact that was not written still stops it.
func TestRecoverableCacheIssuesLeaveTheGlossaryItsDefinitions(t *testing.T) {
	generate := func(t *testing.T, executor llm.Executor, provider llm.Provider) ([]Candidate, []string, error) {
		t.Helper()
		var notices []string
		collector := recoveryCollector(recoveryProse())
		collector.Progress = func(state, detail string) { notices = append(notices, state+": "+detail) }
		err := collector.Generate(t.Context(), executor, provider, everyConcept(), "")
		return collector.Snapshot(), notices, err
	}
	noticed := func(notices []string, state string) bool {
		for _, notice := range notices {
			if strings.HasPrefix(notice, state+": ") {
				return true
			}
		}
		return false
	}
	reference, _, err := generate(t, llm.Executor{}, glossaryAnswers(t))
	if err != nil || len(reference) != 2 {
		t.Fatalf("reference glossary: %+v / %v", reference, err)
	}

	t.Run("a cache that cannot be written", func(t *testing.T) {
		root := t.TempDir()
		cache := filepath.Join(root, llm.CacheDirectoryName)
		if err := os.Mkdir(cache, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(cache, 0o700) })
		if file, err := os.CreateTemp(cache, "probe-*"); err == nil {
			file.Close()
			t.Skip("current user can write into a read-only directory")
		}
		executor := llm.Executor{RootDir: root, Enabled: true}
		got, notices, err := generate(t, executor, glossaryAnswers(t))
		if err != nil || !reflect.DeepEqual(got, reference) {
			t.Fatalf("an unwritable cache changed the glossary: %+v / %v", got, err)
		}
		// The answers' records and the term decisions' memos both failed.
		if !noticed(notices, "cache write failed") || !slices.ContainsFunc(notices, func(notice string) bool {
			return strings.Contains(notice, "remember term decision")
		}) {
			t.Fatalf("the run output did not say the cache was not written: %q", notices)
		}
		catalog, err := Reduce(t.Context(), executor, &reductionProvider{merge: true}, []Candidate{
			termCandidate("Alpha", "Alpha meaning.", "a.py"), termCandidate("alpha", "The same name in lower case.", "b.py"),
		}, func(state, detail string) { notices = append(notices, state+": "+detail) })
		if err != nil || len(catalog.Entries) != 1 || catalog.PartialComparison {
			t.Fatalf("an unwritable cache changed the reduction: %+v / %v", catalog, err)
		}
	})

	t.Run("a damaged cache read back", func(t *testing.T) {
		executor := llm.Executor{RootDir: t.TempDir(), Enabled: true}
		provider := glossaryAnswers(t)
		if got, _, err := generate(t, executor, provider); err != nil || !reflect.DeepEqual(got, reference) {
			t.Fatalf("cold glossary: %+v / %v", got, err)
		}
		records, err := filepath.Glob(filepath.Join(executor.RootDir, llm.CacheDirectoryName, "*.json"))
		if err != nil {
			t.Fatal(err)
		}
		for _, record := range records {
			if strings.HasPrefix(filepath.Base(record), "memo-") {
				continue
			}
			if err := os.WriteFile(record, []byte(`{"damaged"`), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		calls := provider.calls
		got, notices, err := generate(t, executor, provider)
		if err != nil || !reflect.DeepEqual(got, reference) || provider.calls == calls || !noticed(notices, "cache read failed") {
			t.Fatalf("a damaged cache stopped the glossary: %+v / %v, notices %q", got, err, notices)
		}
	})

	t.Run("a required artifact that was not written", func(t *testing.T) {
		cause := errors.New("rejected.jsonl unavailable")
		executor := llm.Executor{Observer: llm.ObserverFunc(func(llm.Event) error { return cause })}
		if _, _, err := generate(t, executor, glossaryAnswers(t)); !errors.Is(err, cause) {
			t.Fatalf("an unwritten required artifact did not stop the glossary: %v", err)
		}
	})
}
