package deepseek

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRussianOutputLanguageReinforcesEveryModelMessage(t *testing.T) {
	client := &Client{Model: "test", MaxTokens: 1000, OutputLanguage: "ru"}

	orientationJSON, err := client.OrientPromptJSON([]byte(`{"allowed_paths":["cmd/server/main.go"]}`))
	if err != nil {
		t.Fatal(err)
	}
	flowJSON, err := client.FlowExplainPromptJSON(
		`{"opaque_id":"direction-1","path":"cmd/server/main.go"}`,
		"Return valid JSON only.",
	)
	if err != nil {
		t.Fatal(err)
	}

	for name, raw := range map[string][]byte{
		"orientation": orientationJSON,
		"flow":        flowJSON,
	} {
		var request chatRequest
		if err := json.Unmarshal(raw, &request); err != nil {
			t.Fatalf("%s request: %v", name, err)
		}
		if len(request.Messages) != 2 {
			t.Fatalf("%s messages = %#v", name, request.Messages)
		}
		system := request.Messages[0].Content
		for _, want := range []string{
			"human-readable prose value in Russian",
			"repository paths",
			"code identifiers",
			"API",
		} {
			if !strings.Contains(system, want) {
				t.Fatalf("%s system prompt is missing %q: %s", name, want, system)
			}
		}
		user := request.Messages[1].Content
		for _, want := range []string{
			"OUTPUT LANGUAGE CONTRACT",
			"human-readable prose value in Russian",
			"protected technical identifiers",
		} {
			if !strings.Contains(user, want) {
				t.Fatalf("%s user prompt is missing %q: %s", name, want, user)
			}
		}
	}

	if !strings.Contains(string(orientationJSON), `cmd/server/main.go`) {
		t.Fatal("orientation request changed the repository path")
	}
	var flowRequest chatRequest
	if err := json.Unmarshal(flowJSON, &flowRequest); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`{"opaque_id":"direction-1","path":"cmd/server/main.go"}`,
		"Do not return English explanatory sentences",
	} {
		if !strings.Contains(flowRequest.Messages[1].Content, want) {
			t.Fatalf("flow user input is missing %q: %q", want, flowRequest.Messages[1].Content)
		}
	}
}

func TestDefaultOutputLanguagePreservesPromptBytes(t *testing.T) {
	client := &Client{Model: "test", MaxTokens: 1000}
	got, err := client.FlowExplainPromptJSON("user", "system")
	if err != nil {
		t.Fatal(err)
	}
	var request chatRequest
	if err := json.Unmarshal(got, &request); err != nil {
		t.Fatal(err)
	}
	if request.Messages[0].Content != "system" ||
		request.Messages[1].Content != "user" {
		t.Fatalf("default messages = %#v", request.Messages)
	}
}
