package orientation

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/typesafe"
	"github.com/dvordrova/repomap/internal/typesafe/typesafetest"
)

func flowChoiceDefinition(task string) table.Definition {
	return table.Definition{Stage: flowStage, Contract: "repomap.orientation.flow.v1", System: task, Classifier: true,
		Columns: []table.Column{{Name: "next", Kind: table.Choice, OptionsFrom: "candidates", CriteriaFrom: "criteria", Item: "step",
			Ask: "Which of `candidates` does the path from `step` continue through to do the program's core work once: one run of a command, one request or message a server handles, or one user action carried to its visible result?"}}}
}

func savedFlowChoice(t *testing.T, name string, count int) (table.Definition, table.Window, []map[string]any) {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name+"-flow-choice.json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	raw, err := io.ReadAll(z)
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		SourceRun   string `json:"source_run"`
		SourceScope string `json:"source_scope"`
		Step, Task  string
		Row         table.Row
		Names       []string
	}
	// Source metadata is local fixture provenance, never model input.
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	var entries []map[string]any
	for i, field := range fixture.Row.Fields {
		if field.Name != "candidates" {
			continue
		}
		for _, value := range field.Value.([]any) {
			entries = append(entries, value.(map[string]any))
		}
		fixture.Row.Fields[i].Value = entries
	}
	if fixture.SourceRun == "" || fixture.SourceScope == "" || fixture.Step != name || len(entries) != count || len(fixture.Names) != count {
		t.Fatalf("complete native fixture changed: step=%s rows=%d", fixture.Step, len(entries))
	}
	for i, entry := range entries {
		if entry["ref"] != fmt.Sprintf("c%d", i+1) || entry["title"] != fixture.Names[i] || entry["criteria"] == "" {
			t.Fatalf("original native candidate missing: %+v", entry)
		}
	}
	return flowChoiceDefinition(fixture.Task), table.Window{Stage: flowStage, Rows: []table.Row{fixture.Row}}, entries
}

func denseFlowChoice(question llm.Question, choice string) llm.Verdict {
	probabilities := make(map[string]float64, len(question.Options))
	for _, option := range question.Options {
		probabilities[option.Name] = 0
	}
	probabilities[choice] = 1
	return llm.Verdict{Choice: choice, Probabilities: probabilities}
}

func TestFlowChoiceCompleteSavedNativeWindowsUseActualPrepareAndOriginalEvidence(t *testing.T) {
	for _, fixture := range []struct {
		name        string
		count       int
		partitioned bool
	}{{"sqlite3VdbeExec", 237, true}, {"sqlite3RegisterBuiltinFunctions", 93, false}} {
		t.Run(fixture.name, func(t *testing.T) {
			def, window, entries := savedFlowChoice(t, fixture.name, fixture.count)
			original, _ := encodeWire(window)
			client := &typesafe.Client{Model: "jev-1.13.0"}
			call, err := table.ClassifierCall(client, def, window)
			if err != nil {
				t.Fatal(err)
			}
			prepared, prepareErr := client.Prepare(call.Prompt, call.Limits)
			if flowInputRefusal(prepareErr) != fixture.partitioned || prepareErr != nil && !flowInputRefusal(prepareErr) {
				t.Fatalf("saved whole envelope: %v", prepareErr)
			}
			t.Logf("native_candidates=%d whole_user_bytes=%d prepared_bytes=%d refusal=%v", len(entries), len(call.Prompt.User), prepared.Len(), prepareErr)
			byRef, byTitle := map[string]map[string]any{}, map[string]map[string]any{}
			for _, entry := range entries {
				byRef[entry["ref"].(string)] = entry
				byTitle[entry["title"].(string)] = entry
			}
			var mu sync.Mutex
			seen := map[string]bool{}
			preferred := entries[len(entries)/3]["title"].(string)
			categorizer := &typesafetest.Categorizer{Client: *client, Decide: func(key string, question llm.Question) (llm.Verdict, bool) {
				if key != window.Rows[0].ID+"|next" {
					t.Errorf("unknown question %q", key)
					return llm.Verdict{}, false
				}
				for _, value := range question.Item["candidates"].([]any) {
					entry := value.(map[string]any)
					original := byRef[entry["ref"].(string)]
					if original == nil {
						t.Errorf("invented candidate: %+v", entry)
						continue
					}
					for field, value := range original {
						if field != "criteria" && !reflect.DeepEqual(value, entry[field]) {
							t.Errorf("native field %s changed: %+v", field, entry)
						}
					}
				}
				chosen := question.Options[0].Name
				for _, option := range question.Options {
					entry := byTitle[option.Name]
					if entry == nil || option.Meaning != entry["criteria"] {
						t.Errorf("candidate lost complete source criteria: %+v", option)
					}
					mu.Lock()
					seen[option.Name] = true
					mu.Unlock()
					if option.Name == preferred {
						chosen = preferred
					}
				}
				return denseFlowChoice(question, chosen), true
			}}
			outcome, verdicts, selected, err := executeFlowChoice(t.Context(), llm.Executor{BatchConcurrency: 4}, categorizer, def, window)
			if err != nil || outcome.Value.Answers[0]["next"] != byTitle[preferred]["ref"] || len(verdicts) != 1 {
				t.Fatalf("full native selection: %v %+v %v", err, outcome.Value, verdicts)
			}
			if len(seen) != len(entries) {
				t.Fatalf("source sampling: read %d of %d original candidates", len(seen), len(entries))
			}
			if (len(selected) > 0) != fixture.partitioned {
				t.Fatalf("fitting whole request unnecessarily partitioned: %v", selected)
			}
			requests := categorizer.Requests()
			for _, request := range requests {
				if _, err := client.Prepare(llm.Prompt{User: string(request)}, call.Limits); err != nil {
					t.Fatalf("actual request exceeds Jev envelope: %v", err)
				}
			}
			if fixture.partitioned {
				var final struct {
					State     struct{ Task string }
					Questions map[string]struct{ Criteria map[string]string }
				}
				if err := json.Unmarshal(requests[len(requests)-1], &final); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(final.State.Task, flowComparisonPrompt) {
					t.Fatal("local confidence promoted without independent final comparison")
				}
				for _, question := range final.Questions {
					for title, criteria := range question.Criteria {
						if criteria != byTitle[title]["criteria"] {
							t.Fatal("final comparison replaced original candidate criteria")
						}
					}
				}
			} else if categorizer.Calls() != 1 || string(requests[0]) != string(prepared.Bytes()) {
				t.Fatal("fitting ordinary request changed")
			}
			after, _ := encodeWire(window)
			if string(original) != string(after) {
				t.Fatal("model selection mutated original native evidence")
			}
			t.Logf("actual_model_requests=%d final_original_candidates=%d", categorizer.Calls(), len(selected))
		})
	}
}

func TestFlowChoiceLocalTiesRequireIndependentComparisonAndNonProgressIsExplicit(t *testing.T) {
	def, window, _ := savedFlowChoice(t, "sqlite3VdbeExec", 237)
	for _, stalled := range []bool{false, true} {
		t.Run(fmt.Sprintf("nonprogress=%t", stalled), func(t *testing.T) {
			categorizer := &typesafetest.Categorizer{Client: typesafe.Client{Model: "jev-1.13.0"}, Decide: func(_ string, question llm.Question) (llm.Verdict, bool) {
				probabilities := map[string]float64{}
				for _, option := range question.Options {
					probabilities[option.Name] = 0
				}
				if stalled {
					for _, option := range question.Options {
						probabilities[option.Name] = 1 / float64(len(question.Options))
					}
				} else {
					probabilities[question.Options[0].Name], probabilities[question.Options[1].Name] = .52, .48
				}
				return llm.Verdict{Choice: question.Options[0].Name, Probabilities: probabilities}, true
			}}
			outcome, verdicts, selected, err := executeFlowChoice(t.Context(), llm.Executor{BatchConcurrency: 4}, categorizer, def, window)
			if stalled {
				if err == nil || !strings.Contains(err.Error(), "cannot reduce") || verdicts != nil || len(selected) != 237 {
					t.Fatalf("unchanged context retried/invented: %v %v %d", err, verdicts, len(selected))
				}
			} else if err != nil || !outcome.Value.Uncertain(0) || len(selected) < 4 || len(verdicts) != 1 {
				t.Fatalf("local ties disappeared or became global without comparison: %v %+v %d", err, outcome.Value, len(selected))
			}
			var last struct{ State struct{ Task string } }
			_ = json.Unmarshal(categorizer.Requests()[categorizer.Calls()-1], &last)
			if strings.Contains(last.State.Task, flowComparisonPrompt) == stalled {
				t.Fatal("wrong final comparison scope")
			}
		})
	}
}

type flowRefusalCategorizer struct {
	*typesafetest.Categorizer
	refuse func([]byte) (llm.Completion, error, bool)
}

func (categorizer flowRefusalCategorizer) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	if response, err, refused := categorizer.refuse(prepared.Bytes()); refused {
		return response, err
	}
	return categorizer.Categorizer.Complete(ctx, prepared)
}

func TestFlowChoiceActualInputRefusalSplitsButOtherFailuresDoNot(t *testing.T) {
	def, window, entries := savedFlowChoice(t, "sqlite3RegisterBuiltinFunctions", 93)
	for _, kind := range []llm.ResourceLimitKind{llm.ResourceLimitContextTokens, llm.ResourceLimitOutputTokens, llm.ResourceLimitResponseBytes} {
		t.Run(string(kind), func(t *testing.T) {
			var requests [][]byte
			var mu sync.Mutex
			base := &typesafetest.Categorizer{Client: typesafe.Client{Model: "jev-1.13.0"}, Decide: func(_ string, question llm.Question) (llm.Verdict, bool) {
				return denseFlowChoice(question, question.Options[0].Name), true
			}}
			categorizer := flowRefusalCategorizer{Categorizer: base, refuse: func(raw []byte) (llm.Completion, error, bool) {
				mu.Lock()
				requests = append(requests, append([]byte(nil), raw...))
				first := len(requests) == 1
				mu.Unlock()
				if first {
					return llm.Completion{}, llm.NewResourceLimitError(llm.ResourceLimitError{Kind: kind}), true
				}
				return llm.Completion{}, nil, false
			}}
			outcome, verdicts, selected, err := executeFlowChoice(t.Context(), llm.Executor{BatchConcurrency: 4}, categorizer, def, window)
			if kind == llm.ResourceLimitContextTokens {
				if err != nil || outcome.Value.Answers[0]["next"] == "" || len(selected) != 2 || len(verdicts) != 1 || len(requests) != 4 {
					t.Fatalf("actual input refusal not divided: %v %d %v", err, len(requests), selected)
				}
				for _, request := range requests[1:] {
					if string(request) == string(requests[0]) {
						t.Fatal("identical refused input retried")
					}
				}
				var question struct {
					Questions map[string]struct{ Criteria map[string]any }
				}
				_ = json.Unmarshal(requests[1], &question)
				for _, q := range question.Questions {
					if len(q.Criteria) >= len(entries) {
						t.Fatal("actual whole refusal did not force changed complete children")
					}
				}
			} else if err == nil || verdicts != nil || len(selected) != 0 || len(requests) != 1 {
				t.Fatalf("non-input failure changed native choice scope: %v %d %v", err, len(requests), selected)
			}
		})
	}
}

func TestFlowChoiceActualSelectionInputRefusalKeepsAcceptedSiblingInMemory(t *testing.T) {
	def, window, _ := savedFlowChoice(t, "sqlite3VdbeExec", 237)
	var mu sync.Mutex
	var attempts [][]byte
	base := &typesafetest.Categorizer{Client: typesafe.Client{Model: "jev-1.13.0"}, Decide: func(_ string, question llm.Question) (llm.Verdict, bool) {
		return denseFlowChoice(question, question.Options[0].Name), true
	}}
	categorizer := flowRefusalCategorizer{Categorizer: base, refuse: func(raw []byte) (llm.Completion, error, bool) {
		mu.Lock()
		attempts = append(attempts, append([]byte(nil), raw...))
		mu.Unlock()
		var request struct {
			State     struct{ Task string }
			Questions map[string]struct {
				Instructions struct {
					Step struct{ Candidates []struct{ Ref string } }
				}
			}
		}
		_ = json.Unmarshal(raw, &request)
		if strings.Contains(request.State.Task, flowSelectionPrompt) {
			for _, question := range request.Questions {
				rows := question.Instructions.Step.Candidates
				if len(rows) > 100 && rows[0].Ref == "c1" {
					return llm.Completion{}, llm.NewResourceLimitError(llm.ResourceLimitError{Kind: llm.ResourceLimitContextTokens}), true
				}
			}
		}
		return llm.Completion{}, nil, false
	}}
	outcome, verdicts, selected, err := executeFlowChoice(t.Context(), llm.Executor{BatchConcurrency: 4}, categorizer, def, window)
	if err != nil || len(selected) != 3 || len(verdicts) != 1 || outcome.Value.Answers[0]["next"] == "" || len(attempts) != 5 {
		t.Fatalf("actual refused leaf lost/repeated accepted sibling: %v selected=%v calls=%d", err, selected, len(attempts))
	}
	seen := map[string]bool{}
	for _, request := range attempts {
		if seen[string(request)] {
			t.Fatal("exact paid request repeated while only its neighbour split")
		}
		seen[string(request)] = true
	}
}

func TestFlowChoiceMissingConflictingAndUnknownDecisionsKeepAcceptedNeighbours(t *testing.T) {
	def, window, _ := savedFlowChoice(t, "sqlite3VdbeExec", 237)
	for _, failure := range []string{"missing", "conflict", "unknown", "nonfinite"} {
		t.Run(failure, func(t *testing.T) {
			base := &typesafetest.Categorizer{Client: typesafe.Client{Model: "jev-1.13.0"}, Decide: func(_ string, question llm.Question) (llm.Verdict, bool) {
				return denseFlowChoice(question, question.Options[0].Name), true
			}}
			categorizer := flowRefusalCategorizer{Categorizer: base, refuse: func(raw []byte) (llm.Completion, error, bool) {
				var request struct {
					Questions map[string]struct {
						Instructions struct {
							Step struct{ Candidates []struct{ Ref string } }
						}
						Criteria map[string]any
					}
				}
				_ = json.Unmarshal(raw, &request)
				for key, q := range request.Questions {
					if len(q.Instructions.Step.Candidates) == 0 || q.Instructions.Step.Candidates[0].Ref != "c1" {
						continue
					}
					encodedKey, _ := json.Marshal(key)
					var response string
					switch failure {
					case "missing":
						response = `{"answers":{}}`
					case "conflict":
						response = `{"answers":{` + string(encodedKey) + `:{"type":"choice","choice":"one","probabilities":{"one":1}},` + string(encodedKey) + `:{"type":"choice","choice":"two","probabilities":{"two":1}}}}`
					case "unknown":
						response = `{"answers":{` + string(encodedKey) + `:{"type":"choice","choice":"unknown","probabilities":{"unknown":1}}}}`
					case "nonfinite":
						response = `{"answers":{` + string(encodedKey) + `:{"type":"choice","choice":"unknown","probabilities":{"unknown":1e309}}}}`
					}
					return llm.Completion{Response: []byte(response), FinishReason: llm.FinishStop, ChoiceCount: 1}, nil, true
				}
				return llm.Completion{}, nil, false
			}}
			executor := llm.Executor{Enabled: true, RootDir: t.TempDir(), BatchConcurrency: 4}
			_, verdicts, selected, err := executeFlowChoice(t.Context(), executor, categorizer, def, window)
			if err == nil || verdicts != nil || len(selected) > 0 || base.Calls() != 1 {
				t.Fatalf("failed dependent choice repaired from accepted neighbours: %v %v %v calls=%d", err, verdicts, selected, base.Calls())
			}
			acceptedCalls := base.Calls()
			_, _, _, err = executeFlowChoice(t.Context(), executor, categorizer, def, window)
			if err == nil || base.Calls() != acceptedCalls {
				t.Fatal("independent accepted selection window repeated after its neighbour failed")
			}
		})
	}
}
