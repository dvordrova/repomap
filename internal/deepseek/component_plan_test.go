package deepseek

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentstudy"
)

func TestComponentPlanPromptJSONUsesBoundedPlanningContract(t *testing.T) {
	t.Parallel()

	if ComponentPlanPromptVersionJSON != "component-plan-json-v3" {
		t.Fatalf("ComponentPlanPromptVersionJSON = %q", ComponentPlanPromptVersionJSON)
	}
	bundleJSON := componentPlanBundleJSON(t)
	bundleJSON = []byte(strings.TrimSuffix(string(bundleJSON), "}") + `,"api_key":"must-not-survive"}`)

	client := &Client{Model: "deepseek-v4-flash", MaxTokens: 6000}
	requestJSON, err := client.ComponentPlanPromptJSON(bundleJSON)
	if err != nil {
		t.Fatalf("ComponentPlanPromptJSON() error = %v", err)
	}
	var request chatRequest
	if err := json.Unmarshal(requestJSON, &request); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if request.Model != client.Model || request.MaxTokens != client.MaxTokens {
		t.Fatalf("request config = model %q, max_tokens %d", request.Model, request.MaxTokens)
	}
	if request.ResponseFormat == nil || request.ResponseFormat.Type != "json_object" {
		t.Fatalf("response format = %#v", request.ResponseFormat)
	}
	if len(request.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(request.Messages))
	}
	prompt := request.Messages[0].Content + "\n" + request.Messages[1].Content
	for _, fragment := range []string{
		"research plan, not a component explanation",
		"Ask 2-4 concrete research questions",
		"selected_files contains at most 2 exact IDs",
		"selected_symbols contains at most 3 exact IDs",
		"single question to investigate now",
		"primary_question_id",
		"Cover the explicit clauses of the research objective",
		"opaque candidate IDs only",
		"Static and navigation evidence is not runtime truth",
		"file-serve",
		"symbol-start",
	} {
		if !strings.Contains(prompt, fragment) {
			t.Errorf("prompt missing %q", fragment)
		}
	}
	if strings.Contains(prompt, "must-not-survive") || strings.Contains(prompt, "api_key") {
		t.Fatal("prompt retained an unknown bundle field")
	}
}

func TestComponentPlanPromptJSONRejectsInvalidBundle(t *testing.T) {
	t.Parallel()

	client := &Client{Model: "deepseek-v4-flash", MaxTokens: 1200}
	tests := []struct {
		name       string
		bundleJSON []byte
	}{
		{name: "not json", bundleJSON: []byte("not json")},
		{name: "invalid contract", bundleJSON: []byte(`{"version":1}`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := client.ComponentPlanPromptJSON(test.bundleJSON); err == nil {
				t.Fatal("ComponentPlanPromptJSON() error = nil")
			}
		})
	}
}

func componentPlanBundleJSON(t *testing.T) []byte {
	t.Helper()

	provenance := componentstudy.Provenance{
		Source:    "local",
		Operation: "package_scan",
	}
	bundle := componentstudy.Bundle{
		Version:  componentstudy.BundleVersion,
		RepoName: "soft-serve",
		Goal: componentstudy.Goal{
			ID:        "goal-onboarding",
			Kind:      componentstudy.GoalOnboarding,
			Objective: "Trace startup and shutdown without assuming runtime order.",
		},
		Component: componentstudy.Component{
			ID:      "component-server",
			Name:    "Server lifecycle",
			Purpose: "Navigation hypothesis for the selected onboarding area.",
		},
		Files: []componentstudy.FileCandidate{
			{
				ID:         "file-serve",
				Rank:       1,
				Path:       "cmd/soft/serve/serve.go",
				Reason:     "Selected component anchor package.",
				Provenance: provenance,
				Certainty:  componentstudy.CertaintyNavigation,
			},
			{
				ID:         "file-server",
				Rank:       2,
				Path:       "cmd/soft/serve/server.go",
				Reason:     "Build-selected file in the same Go package.",
				Provenance: provenance,
				Certainty:  componentstudy.CertaintyStatic,
			},
		},
		Symbols: []componentstudy.SymbolCandidate{
			{
				ID:         "symbol-start",
				Rank:       1,
				Name:       "Start",
				Kind:       "method",
				Path:       "cmd/soft/serve/server.go",
				Line:       120,
				Reason:     "Bounded document-symbol candidate.",
				Provenance: provenance,
				Certainty:  componentstudy.CertaintyNavigation,
			},
		},
		Evidence: []componentstudy.EvidenceCandidate{
			{
				ID:         "evidence-startup",
				Rank:       1,
				Kind:       componentstudy.EvidenceDirection,
				Statement:  "The orientation report suggests server startup as an area to inspect.",
				RelatedIDs: []string{"component-server", "file-serve"},
				Reason:     "Preserve the prior model direction as a hypothesis.",
				Provenance: provenance,
				Certainty:  componentstudy.CertaintyHypothesis,
			},
		},
	}
	data, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
