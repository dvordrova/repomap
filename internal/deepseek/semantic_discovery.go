package deepseek

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/pavedpath"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

// SemanticDiscoveryPromptJSON returns the exact OpenAI-compatible request used
// by DiscoverSemanticsMeasured without making a provider call.
func (c *Client) SemanticDiscoveryPromptJSON(prompt semanticdiscovery.Prompt) ([]byte, error) {
	if err := validateSemanticDiscoveryPrompt(prompt); err != nil {
		return nil, err
	}
	request := c.flowExplainRequest(prompt.User, prompt.System, true)
	if isOfficialDeepSeekEndpoint(c.Endpoint) {
		if prompt.ThinkingProfile == semanticdiscovery.ThinkingDisabled {
			// A reading-pack review is a bounded classification over three to
			// five exact anchors. Hidden reasoning can otherwise consume the
			// entire response envelope before emitting the small JSON verdict.
			request.Thinking = &thinkingConfig{Type: "disabled"}
		} else {
			// DeepSeek thinking mode ignores temperature. The strict local JSON
			// contract and replay cache own reproducibility for these stages.
			request.Temperature = nil
			request.Thinking = &thinkingConfig{Type: "enabled"}
			request.ReasoningEffort = string(prompt.ThinkingProfile)
		}
	}
	return json.Marshal(request)
}

// DiscoverSemanticsMeasured executes one bounded semantic-discovery prompt.
// Response parsing, opaque-ID validation, and materialization remain local to
// semanticdiscovery.
func (c *Client) DiscoverSemanticsMeasured(
	ctx context.Context,
	prompt semanticdiscovery.Prompt,
) (modelresearch.ProviderResult, error) {
	stopWaiting := c.startWaitProgress(ctx, semanticDiscoveryProgressLabel(prompt))
	defer stopWaiting()
	body, err := c.SemanticDiscoveryPromptJSON(prompt)
	if err != nil {
		return modelresearch.ProviderResult{}, err
	}
	result, _, err := doChatMeasured(
		ctx,
		c.HTTPClient,
		c.Endpoint,
		c.APIKey,
		c.Auth,
		body,
		false,
	)
	return providerResultFromCompletion(result, 1, len(body)),
		annotateResourceLimit(err, "semantic_discovery", c.MaxTokens)
}

func semanticDiscoveryProgressLabel(prompt semanticdiscovery.Prompt) string {
	if label := strings.TrimSpace(prompt.ProgressLabel); label != "" &&
		len(label) <= 96 && !strings.ContainsAny(label, "\r\n") {
		return label
	}
	switch prompt.Version {
	case semanticdiscovery.OpportunityPromptVersion:
		return "semantic opportunity scan"
	case semanticdiscovery.LeafPromptVersion:
		return "semantic evidence leaf"
	case semanticdiscovery.FanInPromptVersion:
		return "semantic fan-in synthesis"
	case semanticdiscovery.MonolithicPromptVersion:
		return "semantic monolithic baseline"
	case semanticdiscovery.GoldenMechanismPromptVersion,
		semanticdiscovery.GoldenMechanismPromptVersionV3:
		return "golden mechanism synthesis"
	case semanticdiscovery.OnboardingEditorPromptVersion:
		return "repository onboarding editing"
	case semanticdiscovery.StudyMapPromptVersion:
		return "repository study map editing"
	case semanticdiscovery.StudyBriefPromptVersion:
		return "repository brief and shape editing"
	case semanticdiscovery.StudyCandidatesPromptVersion:
		return "repository study direction planning"
	case semanticdiscovery.ReadingPackReviewPromptVersion:
		return "repository reading pack review"
	case pavedpath.PromptVersion:
		return "repository operating guide editing"
	default:
		return "semantic discovery"
	}
}

func validateSemanticDiscoveryPrompt(prompt semanticdiscovery.Prompt) error {
	var expectedProfile semanticdiscovery.ThinkingProfile
	switch prompt.Version {
	case semanticdiscovery.OpportunityPromptVersion:
		expectedProfile = semanticdiscovery.ThinkingMax
	case semanticdiscovery.LeafPromptVersion:
		expectedProfile = semanticdiscovery.ThinkingHigh
	case semanticdiscovery.FanInPromptVersion:
		expectedProfile = semanticdiscovery.ThinkingMax
	case semanticdiscovery.MonolithicPromptVersion:
		expectedProfile = semanticdiscovery.ThinkingMax
	case semanticdiscovery.GoldenMechanismPromptVersion,
		semanticdiscovery.GoldenMechanismPromptVersionV3:
		expectedProfile = semanticdiscovery.ThinkingMax
	case semanticdiscovery.OnboardingEditorPromptVersion:
		expectedProfile = semanticdiscovery.ThinkingMax
	case semanticdiscovery.StudyMapPromptVersion:
		expectedProfile = semanticdiscovery.ThinkingMax
	case semanticdiscovery.StudyBriefPromptVersion:
		expectedProfile = semanticdiscovery.ThinkingMax
	case semanticdiscovery.StudyCandidatesPromptVersion:
		expectedProfile = semanticdiscovery.ThinkingMax
	case semanticdiscovery.ReadingPackReviewPromptVersion:
		expectedProfile = semanticdiscovery.ThinkingDisabled
	case pavedpath.PromptVersion:
		expectedProfile = semanticdiscovery.ThinkingHigh
	default:
		return fmt.Errorf("llm: unsupported semantic discovery prompt version %q", prompt.Version)
	}
	if prompt.ThinkingProfile != expectedProfile {
		return fmt.Errorf(
			"llm: semantic discovery prompt %q requires thinking profile %q, got %q",
			prompt.Version,
			expectedProfile,
			prompt.ThinkingProfile,
		)
	}
	if strings.TrimSpace(prompt.System) == "" {
		return fmt.Errorf("llm: semantic discovery system prompt is required")
	}
	if strings.TrimSpace(prompt.User) == "" {
		return fmt.Errorf("llm: semantic discovery user prompt is required")
	}
	return nil
}
