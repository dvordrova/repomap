package targetviewchoice

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/llm"
)

func TestCubeCanonicalRefsStateAndOwnership(t *testing.T) {
	input := fixtureViews()
	hypotheses := fixtureFileHypotheses()
	cube, err := Compile(input, hypotheses)
	if err != nil {
		t.Fatal(err)
	}
	request := providerRequest(t, cube)
	if len(request.Views) != 2 || request.Views[0].Ref != "v1" || request.Views[1].Ref != "v2" {
		t.Fatalf("unexpected request refs: %#v", request.Views)
	}
	if request.Views[0].Language != "python" || request.Views[0].Kind != "executable" {
		t.Fatalf("views were not canonically ordered: %#v", request.Views)
	}
	if !reflect.DeepEqual(request.SelectedFileHypotheses, fixtureFileHypotheses()) {
		t.Fatalf("selected file hypotheses were lost: %#v", request.SelectedFileHypotheses)
	}
	wire, err := cube.ProviderVisibleJSON()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), "pyt-") || strings.Contains(string(wire), "target_id") {
		t.Fatalf("provider request leaked canonical identity: %s", wire)
	}
	stateBefore, err := cube.State()
	if err != nil {
		t.Fatal(err)
	}
	input[1].BasisSummaries[0] = "mutated caller input"
	hypotheses[0] = "mutated caller hypothesis"
	request.SelectedFileHypotheses[0] = "mutated returned request"
	request.Views[0].BasisSummaries[0] = "mutated returned request"
	stateAfter, err := cube.State()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stateBefore, stateAfter) {
		t.Fatal("caller mutation changed cube state")
	}

	reordered := fixtureViews()
	reordered[0], reordered[1] = reordered[1], reordered[0]
	other, err := Compile(reordered, fixtureFileHypotheses())
	if err != nil {
		t.Fatal(err)
	}
	otherState, err := other.State()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stateBefore, otherState) {
		t.Fatal("input order changed canonical cube state")
	}
}

func TestResolveResponseIsStrictAndRestoresAuthority(t *testing.T) {
	cube, err := Compile(fixtureViews(), fixtureFileHypotheses())
	if err != nil {
		t.Fatal(err)
	}
	request := providerRequest(t, cube)
	selection, err := cube.ResolveResponse([]byte(`{"default_view_ref":"v2"}`))
	if err != nil {
		t.Fatal(err)
	}
	if selection.DefaultViewRef != "v2" || selection.DefaultView.Selector != request.Views[1].Selector {
		t.Fatalf("selection was not restored from authority: %#v", selection)
	}

	for name, raw := range map[string]string{
		"unknown ref":     `{"default_view_ref":"v99"}`,
		"duplicate field": `{"default_view_ref":"v1","default_view_ref":"v2"}`,
		"extra field":     `{"default_view_ref":"v1","reason":"guess"}`,
		"missing field":   `{}`,
		"wrong type":      `{"default_view_ref":["v1"]}`,
		"trailing value":  `{"default_view_ref":"v1"}{}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := cube.ResolveResponse([]byte(raw)); err == nil {
				t.Fatalf("accepted invalid response %s", raw)
			}
		})
	}
}

func providerRequest(t *testing.T, cube Cube) Request {
	t.Helper()
	wire, err := cube.ProviderVisibleJSON()
	if err != nil {
		t.Fatal(err)
	}
	var request Request
	if err := json.Unmarshal(wire, &request); err != nil {
		t.Fatal(err)
	}
	return request
}

func TestCompileRejectsNonAmbiguousAndDuplicateAuthority(t *testing.T) {
	views := fixtureViews()
	if _, err := Compile(views[:1], fixtureFileHypotheses()); err == nil {
		t.Fatal("accepted a non-ambiguous one-view request")
	}
	views[1] = cloneView(views[0])
	if _, err := Compile(views, fixtureFileHypotheses()); err == nil {
		t.Fatal("accepted duplicate exact views")
	}
	if _, err := Compile(fixtureViews(), nil); err == nil {
		t.Fatal("accepted missing selected file hypotheses")
	}
}

func TestRunUsesSharedLLMExecutor(t *testing.T) {
	cube, err := Compile(fixtureViews(), fixtureFileHypotheses())
	if err != nil {
		t.Fatal(err)
	}
	provider := &choiceProvider{response: []byte(`{"default_view_ref":"v1"}`)}
	selection, err := Run(context.Background(), llm.Executor{Enabled: false}, provider, cube)
	if err != nil {
		t.Fatal(err)
	}
	if selection.DefaultViewRef != "v1" || provider.completeCalls != 1 {
		t.Fatalf("unexpected run result: %#v; calls=%d", selection, provider.completeCalls)
	}
	if !provider.prompt.ResponseFormatJSON || !strings.Contains(provider.prompt.User, `"views"`) {
		t.Fatalf("shared provider received wrong prompt: %#v", provider.prompt)
	}
}

func fixtureViews() []View {
	return []View{
		{
			Language: "python", Kind: "library", DisplayName: "Acme package",
			Selector: "python:acme", AnchorPath: "src/acme/__init__.py",
			RootSummaries:  []string{},
			BasisSummaries: []string{"pyproject declares the acme import package"},
		},
		{
			Language: "python", Kind: "executable", DisplayName: "Acme API",
			Selector: "python:acme-api", AnchorPath: "src/acme/api.py",
			RootSummaries:  []string{"FastAPI application acme.api:app"},
			BasisSummaries: []string{"project script launches acme.api:main"},
		},
	}
}

func fixtureFileHypotheses() []string {
	return []string{
		"README launch example names this file",
		"native discovery found exact executable and library views",
	}
}

type choiceProvider struct {
	response      []byte
	prompt        llm.Prompt
	completeCalls int
}

func (provider *choiceProvider) State() []byte {
	return []byte(`{"endpoint":"https://provider.test","model":"fixture"}`)
}

func (provider *choiceProvider) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	provider.prompt = prompt
	wire, err := json.Marshal(prompt)
	if err != nil {
		return llm.Prepared{}, err
	}
	return llm.NewPrepared(wire)
}

func (provider *choiceProvider) Complete(context.Context, llm.Prepared) (llm.Completion, error) {
	provider.completeCalls++
	return llm.Completion{
		Response: provider.response, FinishReason: llm.FinishStop, ChoiceCount: 1,
		Metrics: llm.Metrics{Attempts: 1, Latency: time.Millisecond},
	}, nil
}
