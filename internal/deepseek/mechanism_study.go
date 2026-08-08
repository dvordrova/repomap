package deepseek

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/mechanismstudy"
	"github.com/dvordrova/repomap/internal/modelresearch"
)

const mechanismStudyStage = "mechanism_study"

// MechanismStudyPromptJSON builds the exact OpenAI-compatible request for one
// bounded mechanism-study batch without making a provider call.
func (c *Client) MechanismStudyPromptJSON(prompt mechanismstudy.Prompt) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("llm: mechanism study client is required")
	}
	if err := validateMechanismStudyPrompt(prompt); err != nil {
		return nil, err
	}
	request := c.canonicalSemanticRequest(prompt.User, prompt.System, true)
	if isOfficialDeepSeekEndpoint(c.Endpoint) {
		request.Thinking = &thinkingConfig{Type: "disabled"}
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("llm: marshal mechanism study request: %w", err)
	}
	return body, nil
}

// MechanismStudyBodyMeasured sends an already-built exact provider body. Only
// retryable transport failures may replay these immutable bytes; semantic
// response validation remains a single terminal decision in mechanismstudy.
func (c *Client) MechanismStudyBodyMeasured(
	ctx context.Context,
	exactBody []byte,
) (modelresearch.ProviderResult, error) {
	if c == nil || c.HTTPClient == nil {
		return modelresearch.ProviderResult{}, fmt.Errorf("llm: mechanism study request client is required")
	}
	if len(exactBody) == 0 {
		return modelresearch.ProviderResult{}, fmt.Errorf("llm: mechanism study request body is required")
	}
	stopWaiting := c.startWaitProgress(ctx, "Study mechanism identification")
	defer stopWaiting()
	completion, attempts, callErr := executeChatWithTransportRetries(
		ctx, c.HTTPClient, c.Endpoint, c.APIKey, c.Auth, exactBody, false,
	)
	result := providerResultFromCompletion(completion, attempts, len(exactBody)*attempts)
	if callErr != nil {
		callErr = annotateIncompleteCompletion(callErr, mechanismStudyStage)
		return result, annotateResourceLimit(callErr, mechanismStudyStage, c.MaxTokens)
	}
	if err := requireSingleStoppedCompletion(mechanismStudyStage, completion); err != nil {
		return result, err
	}
	return result, nil
}

func validateMechanismStudyPrompt(prompt mechanismstudy.Prompt) error {
	if prompt.Version != mechanismstudy.PromptVersion {
		return fmt.Errorf("llm: unsupported mechanism study prompt version %q", prompt.Version)
	}
	if strings.TrimSpace(prompt.System) == "" {
		return fmt.Errorf("llm: mechanism study system prompt is required")
	}
	if strings.TrimSpace(prompt.User) == "" {
		return fmt.Errorf("llm: mechanism study user prompt is required")
	}
	return nil
}
