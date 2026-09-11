package orientation

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/terminology"
)

type terminologyProvider struct {
	result map[string]any
	names  []string
	calls  int
}

func (*terminologyProvider) State() []byte { return []byte(`{"provider":"orientation-terms"}`) }
func (*terminologyProvider) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	wire, err := json.Marshal(presetPrepared{System: prompt.System, User: prompt.User})
	if err != nil {
		return llm.Prepared{}, err
	}
	return llm.NewPrepared(wire)
}
func (p *terminologyProvider) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	p.calls++
	var prompt presetPrepared
	if err := json.Unmarshal(prepared.Bytes(), &prompt); err != nil {
		return llm.Completion{}, err
	}
	if !strings.Contains(prompt.User, `"prose":`) {
		raw, err := json.Marshal(p.result)
		return llm.Completion{Response: raw, FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, err
	}
	var request struct {
		Prose []struct {
			Ref  string
			Text []string
		}
	}
	if err := json.Unmarshal([]byte(prompt.User), &request); err != nil {
		return llm.Completion{}, err
	}
	terms := []map[string]any{}
	for _, name := range p.names {
		for _, row := range request.Prose {
			if !strings.Contains(strings.Join(row.Text, " "), name) {
				continue
			}
			terms = append(terms, map[string]any{"name": name, "kind": "domain", "explanation": "The source-backed meaning of " + name + ".", "rows": []string{row.Ref}})
		}
	}
	raw, err := json.Marshal(map[string]any{"terms": terms})
	return llm.Completion{Response: raw, FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, err
}

func TestOrientationTermsUseOnlyAcceptedOriginalSlotsLiveAndCached(t *testing.T) {
	fixture := newFixture(t)
	refs := fixture.refs(t)
	provider := &terminologyProvider{result: map[string]any{
		"key": "roles[1]", "summary": "BadSummary", "summary_refs": []string{"f999"},
		"roles": []any{
			map[string]any{"target": refs.target("alpha"), "role": "BadRole", "purpose": "BadRole", "refs": []string{"f999"}, "key": "roles[1]", "extra": map[string]string{"key": "roles[1]", "text": "NestedBadRole"}},
			map[string]any{"target": refs.target("beta"), "role": "GoodRole", "purpose": "GoodRole handles requests.", "refs": []string{refs.fact("call")}},
		},
		"run_recipe": []any{
			map[string]any{"command": "BadRecipe", "refs": []string{"f999"}},
			map[string]any{"command": "GoodRecipe", "refs": []string{refs.fact("entrypoint")}},
		},
		"main_flow": map[string]any{"title": "GoodTitle", "steps": []any{
			map[string]any{"target": refs.target("alpha"), "ref": "f999", "explanation": "BadStep", "key": "main_flow.steps[1]"},
			map[string]any{"target": refs.target("beta"), "ref": refs.fact("call"), "explanation": "GoodStep"},
		}},
	}, names: []string{"BadSummary", "BadRole", "NestedBadRole", "GoodRole", "BadRecipe", "GoodRecipe", "BadStep", "GoodStep", "GoodTitle"}}
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
	var first []terminology.Candidate
	for run := 0; run < 2; run++ {
		collector := terminology.NewCollector([]string{"README.md"})
		result, rejected, err := Run(t.Context(), executor, collector.Wrap(provider), fixture.input)
		if err != nil || len(rejected) != 4 || result.Summary != "" || len(result.Roles) != 1 || len(result.RunRecipe) != 1 || len(result.MainFlow.Steps) != 1 {
			t.Fatalf("partial orientation lost valid siblings: %+v, %v", result, err)
		}
		if err := collector.Generate(t.Context(), executor, provider); err != nil {
			t.Fatal(err)
		}
		terms := collector.Snapshot()
		want := map[string]string{"GoodRole": "roles[1]", "GoodRecipe": "run_recipe[1]", "GoodStep": "main_flow.steps[1]", "GoodTitle": "main_flow.title"}
		if len(terms) != len(want) {
			t.Fatalf("refused text supplied a term or accepted term was lost: %+v", terms)
		}
		for _, term := range terms {
			if len(term.Origins) != 1 || term.Origins[0].Row != want[term.Name] || term.Origins[0].RequestSHA256 == "" {
				t.Fatalf("term lost its accepted original slot: %+v", term)
			}
		}
		if run == 0 {
			first = terms
		} else if !reflect.DeepEqual(first, terms) {
			t.Fatal("cache replay changed accepted term origins")
		}
	}
	if provider.calls != 2 {
		t.Fatal("cached partial orientation made another provider call")
	}
}

func TestOrientationEquivalentRoleSlotsSurviveUntilAnActualConflict(t *testing.T) {
	fixture := newFixture(t)
	refs := fixture.refs(t)
	_, catalogue, err := buildRequest(fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	for _, conflict := range []bool{false, true} {
		rows := []any{
			map[string]any{"target": refs.target("alpha"), "role": "Backend", "purpose": "Serves items.", "refs": []string{refs.fact("route")}},
			map[string]any{"target": refs.target("alpha"), "role": "Backend", "purpose": "Serves items.", "refs": []string{refs.fact("entrypoint")}},
			map[string]any{"target": refs.target("beta"), "role": "Client", "purpose": "Fetches items.", "refs": []string{refs.fact("call")}},
		}
		if conflict {
			rows = append(rows, map[string]any{"target": refs.target("alpha"), "role": "Worker", "purpose": "Handles background jobs.", "refs": []string{refs.fact("route")}})
		}
		raw, _ := json.Marshal(map[string]any{"roles": rows})
		result, err := normalize(raw, catalogue)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"roles[0]", "roles[1]", "roles[2]"}
		if conflict {
			want = []string{"roles[2]"}
		}
		if !slices.Equal(result.AcceptedRowKeys(), want) {
			t.Fatalf("equivalent/conflicting role term scope: %v, want %v", result.AcceptedRowKeys(), want)
		}
	}
}

func TestOrientationCachesEmptyResponseButNeverWhollyRejectedOutput(t *testing.T) {
	fixture := newFixture(t)
	refs := fixture.refs(t)
	for _, test := range []struct {
		name     string
		response map[string]any
		accepted bool
	}{
		{"empty", map[string]any{}, true},
		{"bad section types", map[string]any{"summary": 42, "roles": "wrong", "run_recipe": true, "main_flow": []string{"wrong"}}, false},
		{"all rows refused", map[string]any{
			"summary": "Unsupported summary", "summary_refs": []string{"f999"},
			"roles":      []any{map[string]any{"target": refs.target("alpha"), "role": "Unsupported role", "purpose": "Unsupported purpose", "refs": []string{"f999"}}},
			"run_recipe": []any{map[string]any{"command": "unsupported", "refs": []string{"f999"}}},
			"main_flow":  map[string]any{"title": "Unsupported flow", "steps": []any{map[string]any{"target": refs.target("alpha"), "ref": "f999", "explanation": "Unsupported step"}}},
		}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := &presetProvider{respond: func([]byte) []byte { return encodeResponse(t, test.response) }}
			executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
			for range 2 {
				result, rejected, err := Run(t.Context(), executor, provider, fixture.input)
				if err != nil || result.Validate() != nil || result.Summary != "" || len(result.Roles) != 0 || len(result.RunRecipe) != 0 || len(result.MainFlow.Steps) != 0 {
					t.Fatalf("empty or refused output invented orientation or aborted: %+v, %v", result, err)
				}
				if test.accepted != (len(rejected) == 0) {
					t.Fatal("legitimate empty and wholly refused response were conflated")
				}
			}
			wantCalls := 2
			if test.accepted {
				wantCalls = 1
			}
			if provider.completions != wantCalls {
				t.Fatalf("provider calls=%d, want %d", provider.completions, wantCalls)
			}
		})
	}
}
