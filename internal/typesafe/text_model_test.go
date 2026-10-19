package typesafe

import (
	"context"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
)

// An option list several questions share is written once before the
// questions and named by each; a list one question owns stays inline, and
// the lines still answer each question from its own options.
func TestTheTextModelWritesASharedOptionListOnce(t *testing.T) {
	entry := []llm.Option{{Name: "request", Meaning: "work another program sends"}, {Name: "none", Meaning: "no entry"}}
	model := &TextModel{}
	prompt, err := model.Prompt("task", nil, map[string]llm.Question{
		"c1|enters": {Name: "call", Item: map[string]any{"id": "a"}, Ask: "What does it become?", Options: entry},
		"c2|enters": {Name: "call", Item: map[string]any{"id": "b"}, Ask: "What does it become?", Options: entry},
		"c3|role":   {Item: map[string]any{"id": "c"}, Ask: "Which role?", Options: []llm.Option{{Name: "domain"}, {Name: "adapter"}}},
		"c4|split":  {Item: map[string]any{"id": "d"}, Ask: "Several?"},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := prompt.User
	if strings.Count(text, "- request: work another program sends") != 1 || !strings.Contains(text, "Option lists:\n\nList A:\n- request") ||
		strings.Count(text, "Options: list A") != 2 || !strings.Contains(text, "   Options:\n   - domain\n   - adapter\n") ||
		!strings.Contains(text, "Answer yes or no.") || strings.Index(text, "List A:") > strings.Index(text, "Questions:") {
		t.Fatalf("prompt:\n%s", text)
	}
	asked, _ := model.asked.Load(text)
	answers, err := answersFromLines(asked.([]askedQuestion), []byte("1. request\n2. none\n3. adapter\n4. yes"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"c1|enters":{"choice":"request"`, `"c2|enters":{"choice":"none"`, `"c3|role":{"choice":"adapter"`, `"c4|split":{"noul":1`} {
		if !strings.Contains(string(answers), want) {
			t.Fatalf("answers %s lack %s", answers, want)
		}
	}
}

func TestSharedOptionListsAreLabelledPastZ(t *testing.T) {
	for n, want := range map[int]string{0: "A", 25: "Z", 26: "AA", 27: "AB", 52: "BA", 701: "ZZ", 702: "AAA"} {
		if got := listLabel(n); got != want {
			t.Errorf("listLabel(%d) = %q, want %q", n, got, want)
		}
	}
}

// linesProvider answers every request with the same lines.
type linesProvider struct{ lines string }

func (p linesProvider) State() []byte { return []byte(`"lines"`) }
func (p linesProvider) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	return llm.NewPrepared([]byte(prompt.User))
}
func (p linesProvider) Complete(context.Context, llm.Prepared) (llm.Completion, error) {
	return llm.Completion{Response: []byte(p.lines)}, nil
}

// Two windows asking the same items under other keys are other requests,
// and each reads its answers under its own keys.
func TestTwoWindowsOfTheSameItemsKeepTheirOwnKeys(t *testing.T) {
	model := &TextModel{Provider: linesProvider{lines: "1. [b9|enough] yes"}}
	ask := func(key string) llm.Prepared {
		t.Helper()
		prompt, err := model.Prompt("task", nil, map[string]llm.Question{key: {Name: "box", Item: map[string]any{"name": "api"}, Ask: "Smaller boxes?"}})
		if err != nil {
			t.Fatal(err)
		}
		prepared, err := model.Prepare(prompt, llm.Limits{MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20, MaxOutputTokens: 1})
		if err != nil {
			t.Fatal(err)
		}
		return prepared
	}
	first, second := ask("b4|enough"), ask("b5|enough")
	if string(first.Bytes()) == string(second.Bytes()) {
		t.Fatal("two windows under other keys prepared one request")
	}
	for prepared, key := range map[*llm.Prepared]string{&first: "b4|enough", &second: "b5|enough"} {
		completion, err := model.Complete(t.Context(), *prepared)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(completion.Response), `"`+key+`":{"noul":1`) {
			t.Fatalf("%s read %s", key, completion.Response)
		}
	}
}

// The lines carry only what the format defines; everything else stays
// unknown for the owner to refuse rather than becoming a guessed decision.
func TestTextModelLinesDecideOnlyWhatTheyState(t *testing.T) {
	ask := func(t *testing.T, lines string) map[string]llm.Verdict {
		t.Helper()
		model := &TextModel{Provider: linesProvider{lines: lines}}
		prompt, err := model.Prompt("task", nil, map[string]llm.Question{
			"a|split": {Item: map[string]any{"id": "a"}, Ask: "Several?"},
			"b|kind":  {Item: map[string]any{"id": "b"}, Ask: "Which?", Options: []llm.Option{{Name: "request"}, {Name: "background"}}},
			"c|split": {Item: map[string]any{"id": "c"}, Ask: "Several?"},
		})
		if err != nil {
			t.Fatal(err)
		}
		prepared, err := model.Prepare(prompt, llm.Limits{})
		if err != nil {
			t.Fatal(err)
		}
		completion, err := model.Complete(t.Context(), prepared)
		if err != nil {
			t.Fatal(err)
		}
		verdicts, err := model.Verdicts(completion.Response)
		if err != nil {
			t.Fatal(err)
		}
		return verdicts
	}
	yes := func(v llm.Verdict) bool { return v.Yes != nil && *v.Yes == 1 }
	no := func(v llm.Verdict) bool { return v.Yes != nil && *v.Yes == 0 }

	// Questions are numbered 1 a|split, 2 b|kind, 3 c|split (keys sorted).
	t.Run("two different answers are a conflict, the neighbour survives", func(t *testing.T) {
		v := ask(t, "1. yes\n1. no\n3. no")
		if !v["a|split"].Conflict {
			t.Fatalf("yes then no decided %+v", v["a|split"])
		}
		if !no(v["c|split"]) {
			t.Fatalf("the neighbour lost its answer: %+v", v["c|split"])
		}
	})
	t.Run("the same answer twice is one answer", func(t *testing.T) {
		v := ask(t, "1. yes\n1. Yes.")
		if !yes(v["a|split"]) || v["a|split"].Conflict {
			t.Fatalf("a repeated yes became %+v", v["a|split"])
		}
	})
	t.Run("not sure is no answer, not no", func(t *testing.T) {
		if v, ok := ask(t, "1. not sure")["a|split"]; ok {
			t.Fatalf("not sure decided %+v", v)
		}
	})
	t.Run("a number outside the questions answers nothing", func(t *testing.T) {
		if v := ask(t, "99. yes"); len(v) != 0 {
			t.Fatalf("99. yes decided %+v", v)
		}
	})
	t.Run("an unnumbered line shifts nothing", func(t *testing.T) {
		v := ask(t, "1. yes\nbecause the item splits\n2. request\n3. no")
		if !yes(v["a|split"]) || v["b|kind"].Choice != "request" || !no(v["c|split"]) {
			t.Fatalf("an explanation shifted the answers: %+v", v)
		}
	})
	t.Run("an answer naming two options is not the longer one", func(t *testing.T) {
		v := ask(t, "2. request or background")
		if v["b|kind"].Choice == "request" || v["b|kind"].Choice == "background" {
			t.Fatalf("two named options became one: %+v", v["b|kind"])
		}
		for option, p := range v["b|kind"].Probabilities {
			if p != 0 {
				t.Fatalf("%s at %v for an answer outside the options", option, p)
			}
		}
	})
	t.Run("form around the answer is not the answer", func(t *testing.T) {
		v := ask(t, "1. `YES`\n2. \"Background\".\n3. **no**")
		if !yes(v["a|split"]) || v["b|kind"].Choice != "background" || !no(v["c|split"]) {
			t.Fatalf("quotes, case, bold or a trailing dot refused an answer: %+v", v)
		}
	})
}
