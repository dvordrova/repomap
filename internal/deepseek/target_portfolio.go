package deepseek

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/targetportfolio"
)

const targetPortfolioSelectionStage = "target_portfolio_selection"

// TargetPortfolioPromptJSON builds the exact OpenAI-compatible request for
// one bounded target-portfolio selection without making a provider call.
func (c *Client) TargetPortfolioPromptJSON(prompt targetportfolio.Prompt) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("llm: target portfolio client is required")
	}
	if err := validateTargetPortfolioPrompt(prompt); err != nil {
		return nil, err
	}
	request := c.canonicalSemanticRequest(prompt.User, prompt.System, true)
	if isOfficialDeepSeekEndpoint(c.Endpoint) {
		request.Thinking = &thinkingConfig{Type: "disabled"}
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("llm: marshal target portfolio request: %w", err)
	}
	if len(body) > targetportfolio.MaxProviderRequestBytes {
		return nil, modelresearch.NewResourceLimitError(modelresearch.ResourceLimitError{
			Stage: targetPortfolioSelectionStage, Kind: modelresearch.ResourceLimitRequestBytes,
			Limit: targetportfolio.MaxProviderRequestBytes, Observed: len(body), ObservedKnown: true,
			ConfiguredMaxTokens: c.MaxTokens,
		}, nil)
	}
	return body, nil
}

// TargetPortfolioMeasured builds and sends one target-portfolio request. The
// returned provider content is deliberately not decoded here: targetportfolio
// owns the one strict local decision and restoration of backend-owned targets.
func (c *Client) TargetPortfolioMeasured(
	ctx context.Context,
	prompt targetportfolio.Prompt,
) (modelresearch.ProviderResult, error) {
	body, err := c.TargetPortfolioPromptJSON(prompt)
	if err != nil {
		return modelresearch.ProviderResult{}, err
	}
	return c.TargetPortfolioBodyMeasured(ctx, body)
}

// TargetPortfolioBodyMeasured sends an already-built exact provider body.
// Retryable transport failures may replay only these immutable bytes. Invalid
// semantic content is preserved for the local targetportfolio reducer and is
// never repaired or retried by the provider transport.
func (c *Client) TargetPortfolioBodyMeasured(
	ctx context.Context,
	exactBody []byte,
) (modelresearch.ProviderResult, error) {
	if c == nil || c.HTTPClient == nil {
		return modelresearch.ProviderResult{}, fmt.Errorf("llm: target portfolio request client is required")
	}
	if len(exactBody) == 0 {
		return modelresearch.ProviderResult{}, fmt.Errorf("llm: target portfolio request body is required")
	}
	if len(exactBody) > targetportfolio.MaxProviderRequestBytes {
		return modelresearch.ProviderResult{}, modelresearch.NewResourceLimitError(modelresearch.ResourceLimitError{
			Stage: targetPortfolioSelectionStage, Kind: modelresearch.ResourceLimitRequestBytes,
			Limit: targetportfolio.MaxProviderRequestBytes, Observed: len(exactBody), ObservedKnown: true,
			ConfiguredMaxTokens: c.MaxTokens,
		}, nil)
	}

	stopWaiting := c.startWaitProgress(ctx, "target portfolio selection")
	defer stopWaiting()
	completion, attempts, callErr := executeChatWithTransportRetries(
		ctx, c.HTTPClient, c.Endpoint, c.APIKey, c.Auth, exactBody, false,
	)
	result := providerResultFromCompletion(completion, attempts, len(exactBody)*attempts)
	if callErr != nil {
		callErr = annotateIncompleteCompletion(callErr, targetPortfolioSelectionStage)
		return result, annotateResourceLimit(callErr, targetPortfolioSelectionStage, c.MaxTokens)
	}
	if err := requireSingleStoppedCompletion(targetPortfolioSelectionStage, completion); err != nil {
		return result, err
	}
	if len(result.Content) > targetportfolio.MaxResponseBytes {
		return result, modelresearch.NewResourceLimitError(modelresearch.ResourceLimitError{
			Stage: targetPortfolioSelectionStage, Kind: modelresearch.ResourceLimitResponseBytes,
			Limit: targetportfolio.MaxResponseBytes, Observed: len(result.Content), ObservedKnown: true,
			ConfiguredMaxTokens: c.MaxTokens,
			InputTokens:         result.InputTokens, OutputTokens: result.OutputTokens,
			ReasoningTokens: result.ReasoningTokens, FinishReason: result.FinishReason,
		}, result.Content)
	}
	return result, nil
}

func validateTargetPortfolioPrompt(prompt targetportfolio.Prompt) error {
	if prompt.Version != targetportfolio.PromptVersion {
		return fmt.Errorf("llm: unsupported target portfolio prompt version %q", prompt.Version)
	}
	if strings.TrimSpace(prompt.System) == "" {
		return fmt.Errorf("llm: target portfolio system prompt is required")
	}
	if strings.TrimSpace(prompt.User) == "" {
		return fmt.Errorf("llm: target portfolio user prompt is required")
	}
	return nil
}
