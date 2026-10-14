package table

import (
	"context"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/typesafe"
)

// linesProvider answers every request with the same lines.
type linesProvider struct{ lines string }

func (p linesProvider) State() []byte { return []byte(`"lines"`) }
func (p linesProvider) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	return llm.NewPrepared([]byte(prompt.User))
}
func (p linesProvider) Complete(context.Context, llm.Prepared) (llm.Completion, error) {
	return llm.Completion{Response: []byte(p.lines)}, nil
}

// Two windows of other contexts are one text-model request, each context
// before its own questions; each window reads its own answers, and a window
// the response left unanswered is refused without its neighbour.
func TestTwoContextsAreOneTextModelRequest(t *testing.T) {
	def := Definition{Stage: "atlas_keys", Contract: "keys", System: "Choose.", Classifier: true,
		Columns: []Column{{Name: "pick", Kind: Choice, Options: []string{"yes", "no"}, Item: "declaration"}}}
	windows := []Window{
		SectionWindow(Window{Context: []Field{{Name: "part", Value: "Orders"}}, Rows: []Row{{ID: "d1", Fields: []Field{{Name: "name", Value: "Place"}}}}}, 0),
		SectionWindow(Window{Context: []Field{{Name: "part", Value: "Users"}}, Rows: []Row{{ID: "d1", Fields: []Field{{Name: "name", Value: "Login"}}}}}, 1),
		SectionWindow(Window{Context: []Field{{Name: "part", Value: "Mail"}}, Rows: []Row{{ID: "d1", Fields: []Field{{Name: "name", Value: "Send"}}}}}, 2),
	}
	model := &typesafe.TextModel{Provider: linesProvider{lines: "1. yes\n2. no"}}
	call, err := ClassifierSectionsCall(model, def, windows, nil)
	if err != nil {
		t.Fatal(err)
	}
	text := call.Prompt.User
	if !strings.Contains(text, "Context 1:\n{\"part\":\"Orders\"}") || !strings.Contains(text, "Context 3:\n{\"part\":\"Mail\"}") ||
		strings.Index(text, "Context 2:") > strings.Index(text, "2. [s2.d1|pick]") || strings.Index(text, "1. [s1.d1|pick]") > strings.Index(text, "Context 2:") {
		t.Fatalf("prompt:\n%s", text)
	}
	prepared, err := model.Prepare(call.Prompt, call.Limits)
	if err != nil {
		t.Fatal(err)
	}
	completion, err := model.Complete(t.Context(), prepared)
	if err != nil {
		t.Fatal(err)
	}
	sections, err := call.DecodeValidate(completion.Response)
	if err != nil {
		t.Fatal(err)
	}
	if sections.Errs[0] != nil || sections.Results[0].Answers[0]["pick"] != "yes" ||
		sections.Errs[1] != nil || sections.Results[1].Answers[0]["pick"] != "no" || sections.Errs[2] == nil {
		t.Fatalf("sections: %+v", sections)
	}
}
