package deepseek

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentteach"
)

// Replaceable experiment-contract test: keep only while the component teacher
// wire contract is being challenged independently of the product UI.
func TestComponentTeacherPromptJSONUsesGroundedContract(t *testing.T) {
	t.Parallel()

	if ComponentTeachPromptVersionJSON != "component-teach-json-v2" {
		t.Fatalf("ComponentTeachPromptVersionJSON = %q", ComponentTeachPromptVersionJSON)
	}
	bundle := componentteach.Bundle{
		Version:       componentteach.BundleVersion,
		GoalObjective: "Understand how startup errors reach the command boundary.",
		Component: componentteach.Component{
			Name:              "Server lifecycle",
			PurposeHypothesis: "Prior orientation suggests this area owns startup.",
			SupportBasis:      componentteach.SupportOrientationHypothesis,
		},
		PrimaryQuestion: componentteach.PrimaryQuestion{
			ID:       "q-startup",
			Question: "How does startup report an immediate service error?",
			Why:      "This distinguishes written error propagation from runtime convergence.",
		},
		Evidence: []componentteach.EvidenceItem{
			{
				ID:           "teach-0123456789abcdef0123",
				Kind:         componentteach.EvidenceSourceSlice,
				SupportBasis: componentteach.SupportSource,
				Summary:      "Bounded source slice for startup.",
				Content:      []string{"if err := start(); err != nil {", "return err", "}"},
			},
		},
		UnresolvedFrontierIDs: []string{"frontier-abcdef0123456789abcd"},
		UnresolvedFrontiers: []componentteach.FrontierHint{
			{
				ID:             "frontier-abcdef0123456789abcd",
				Kind:           "call_endpoint",
				Direction:      "outgoing",
				Name:           "Server.Start",
				EntityKind:     "method",
				SupportBasis:   componentteach.SupportStaticActiveBuild,
				NavigationOnly: false,
			},
		},
		Warnings: []string{},
	}
	bundleJSON, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	bundleJSON = []byte(strings.TrimSuffix(string(bundleJSON), "}") +
		`,"api_key":"must-not-survive","index":{"path":"private/location.go"}}`)

	client := &Client{Model: "deepseek-v4-flash", MaxTokens: 6000}
	requestJSON, err := client.TeacherPromptJSON(bundleJSON)
	if err != nil {
		t.Fatalf("TeacherPromptJSON() error = %v", err)
	}
	var request chatRequest
	if err := json.Unmarshal(requestJSON, &request); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if request.Model != client.Model || request.MaxTokens != client.MaxTokens {
		t.Fatalf("request config = model %q, max_tokens %d", request.Model, request.MaxTokens)
	}
	if request.Temperature != 0 {
		t.Fatalf("temperature = %v, want 0", request.Temperature)
	}
	if request.ResponseFormat == nil || request.ResponseFormat.Type != "json_object" {
		t.Fatalf("response format = %#v", request.ResponseFormat)
	}
	if len(request.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(request.Messages))
	}
	prompt := request.Messages[0].Content + "\n" + request.Messages[1].Content
	for _, fragment := range []string{
		"Teach only the supplied primary question",
		"Every returned item must cite at least one exact evidence id",
		"frontier ID and its unresolved_frontiers hint are navigation leads",
		"Use that frontier's supplied name, direction, kind, entity_kind, and support_basis",
		"static_active_build",
		"Bounded caller/callee evidence cannot prove absence",
		`Never say "only", "never", or "not in the call chain"`,
		"source_supported",
		"test_navigation_only",
		"No runtime-observed evidence is supplied",
		"Inspect all supplied source_slice and callsite_slice content before declaring code",
		"only when its ID remains in both unresolved_frontier_ids and unresolved_frontiers",
		"Never copy or reconstruct repository paths",
		"teach-0123456789abcdef0123",
		"frontier-abcdef0123456789abcd",
	} {
		if !strings.Contains(prompt, fragment) {
			t.Errorf("prompt missing %q", fragment)
		}
	}
	for _, forbidden := range []string{"must-not-survive", "private/location.go", `"api_key"`} {
		if strings.Contains(prompt, forbidden) {
			t.Errorf("prompt retained unknown local field %q", forbidden)
		}
	}
}
