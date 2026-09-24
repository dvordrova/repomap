package table

import (
	"encoding/json"
	"strings"
	"testing"
)

func closedDefinition() Definition {
	return Definition{
		Stage: "atlas_closed", Contract: "repomap.atlas.closed.v1", System: "choose a part", Independent: true,
		Columns: []Column{{Name: "part", Kind: Choice, OptionsFrom: "part_options"}},
	}
}

func closedWindow() Window {
	return Window{
		Context: []Field{{Name: "parts", Value: []map[string]any{{"ref": "c1", "title": "Serving"}}}, {Name: "part_options", Value: []string{"c1", "none"}}},
		Rows:    []Row{{ID: "s1", Fields: []Field{{Name: "name", Value: "Serve"}}}, {ID: "s2", Fields: []Field{{Name: "name", Value: "helper"}}}},
	}
}

// The context is sent once as state; each row is its own question over the
// listed options, keyed by the row's own identity.
func TestClassifierCallAsksOneChoicePerRow(t *testing.T) {
	call, err := ClassifierCall(closedDefinition(), closedWindow(), 0.5)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		State     map[string]any `json:"state"`
		Questions map[string]struct {
			Type     string         `json:"type"`
			Criteria map[string]any `json:"criteria"`
		} `json:"questions"`
	}
	if err := json.Unmarshal([]byte(call.Prompt.User), &body); err != nil {
		t.Fatal(err)
	}
	if body.State["task"] != "choose a part" || body.State["context"] == nil || len(body.Questions) != 2 {
		t.Fatalf("body: %s", call.Prompt.User)
	}
	question := body.Questions["s1|part"]
	if question.Type != "choice" || len(question.Criteria) != 2 || !strings.Contains(call.Prompt.User, `"none":null`) {
		t.Fatalf("question: %+v", question)
	}
}

// A chosen option at or below the probability floor refuses its own row,
// whatever the distribution's confidence says, not its neighbour;
// an unlisted choice is never taken.
func TestDecodeClassifierRefusesUncertainRowsAlone(t *testing.T) {
	window := closedWindow()
	raw := []byte(`{"answers":{"s1|part":{"type":"choice","choice":"Serving","confidence":0.3,"probabilities":{"Serving":0.9,"none":0.1}},"s2|part":{"type":"choice","choice":"Serving","confidence":0.9,"probabilities":{"Serving":0.4,"none":0.35}}}}`)
	result, err := DecodeClassifier(closedDefinition(), window, raw, 0.5)
	if err != nil || result.Answers[0]["part"] != "c1" || result.Answers[1] != nil || len(result.Rejections) != 1 || result.Rejections[0].Key != "s2" || strings.Join(result.AcceptedRowKeys(), " ") != "s1" {
		t.Fatalf("result %+v %v", result, err)
	}
	unlisted := []byte(`{"answers":{"s1|part":{"type":"choice","choice":"c9","confidence":1,"probabilities":{"c9":1}},"s2|part":{"type":"choice","choice":"none","confidence":1,"probabilities":{"none":1}}}}`)
	result, err = DecodeClassifier(closedDefinition(), window, unlisted, 0.5)
	if err != nil || result.Answers[0] != nil || result.Answers[1]["part"] != "none" {
		t.Fatalf("unlisted choice: %+v %v", result, err)
	}
}

func TestOnlyUnconditionalChoicesAreClosed(t *testing.T) {
	def := closedDefinition()
	if !Closed(def) {
		t.Fatal("a closed choice table is not closed")
	}
	def.Columns = append(def.Columns, Column{Name: "line", Kind: Text})
	if Closed(def) {
		t.Fatal("a table with text is closed")
	}
	def.Columns = []Column{{Name: "box", Kind: Choice, OptionsFrom: "box_options", Free: "new: "}}
	if Closed(def) {
		t.Fatal("a choice that allows free text is closed")
	}
}

// An optional yes-only column is a yes/no question, and an optional choice
// has an explicit way to say that nothing applies: a decision model that
// must pick a listed value otherwise answers yes to every row.
func TestOptionalColumnsCanBeLeftEmpty(t *testing.T) {
	def := Definition{Stage: "atlas_optional", Contract: "c", System: "s", Independent: true, Columns: []Column{
		{Name: "explains", Kind: Choice, Options: []string{"yes"}, Optional: true},
		{Name: "talks", Kind: Choice, Options: []string{"db", "sdk"}, Optional: true},
	}}
	window := Window{Rows: []Row{{ID: "s1"}, {ID: "s2"}}}
	call, err := ClassifierCall(def, window, 0.5)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(call.Prompt.User, `"s1|explains":{"instructions"`) || !strings.Contains(call.Prompt.User, `"type":"noul"`) || !strings.Contains(call.Prompt.User, `"none of these"`) {
		t.Fatalf("body: %s", call.Prompt.User)
	}
	raw := []byte(`{"answers":{
		"s1|explains":{"type":"noul","noul":0.8},"s1|talks":{"type":"choice","choice":"db","probabilities":{"db":0.9}},
		"s2|explains":{"type":"noul","noul":0.2},"s2|talks":{"type":"choice","choice":"none of these","probabilities":{"none of these":0.9}}}}`)
	result, err := DecodeClassifier(def, window, raw, 0.5)
	if err != nil || result.Answers[0]["explains"] != "yes" || result.Answers[0]["talks"] != "db" || len(result.Answers[1]) != 0 || result.Answers[1] == nil {
		t.Fatalf("result %+v %v", result, err)
	}
}

// A yes/no cutoff decides every row: yes at the cutoff or above, no below.
func TestYesAtIsACutoffNotAnUncertainBand(t *testing.T) {
	def := Definition{Stage: "atlas_cutoff", Contract: "c", System: "s", Independent: true, YesAt: 0.8,
		Columns: []Column{{Name: "key_symbol", Kind: Choice, Options: []string{"yes", "no"}}}}
	window := Window{Rows: []Row{{ID: "s1"}, {ID: "s2"}}}
	raw := []byte(`{"answers":{"s1|key_symbol":{"type":"choice","choice":"yes","probabilities":{"yes":0.85,"no":0.15}},"s2|key_symbol":{"type":"choice","choice":"yes","probabilities":{"yes":0.7,"no":0.3}}}}`)
	result, err := DecodeClassifier(def, window, raw, 0.5)
	if err != nil || result.Answers[0]["key_symbol"] != "yes" || result.Answers[1]["key_symbol"] != "no" {
		t.Fatalf("result %+v %v", result, err)
	}
}
