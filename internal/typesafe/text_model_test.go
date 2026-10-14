package typesafe

import (
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
