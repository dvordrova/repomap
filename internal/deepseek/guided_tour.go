package deepseek

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/guidedtour"
	"github.com/dvordrova/repomap/internal/modelresearch"
)

// GuidedTourPromptJSON returns the exact OpenAI-compatible request used by
// EditGuidedTourMeasured without making a provider call.
func (c *Client) GuidedTourPromptJSON(prompt guidedtour.Prompt) ([]byte, error) {
	request, err := c.guidedTourRequest(prompt)
	if err != nil {
		return nil, err
	}
	return json.Marshal(request)
}

func (c *Client) guidedTourRequest(prompt guidedtour.Prompt) (chatRequest, error) {
	if err := validateGuidedTourPrompt(prompt); err != nil {
		return chatRequest{}, err
	}
	request := c.flowExplainRequest(prompt.User, prompt.System, true)
	if isOfficialDeepSeekEndpoint(c.Endpoint) {
		// DeepSeek thinking mode ignores temperature. Omit it so the request
		// makes clear that determinism comes from the local JSON contract.
		request.Temperature = nil
		request.Thinking = &thinkingConfig{Type: "enabled"}
		if prompt.Version == guidedtour.LeafPromptVersion {
			request.ReasoningEffort = "high"
		} else {
			request.ReasoningEffort = "max"
		}
	}
	return request, nil
}

// EditGuidedTourMeasured asks for one bounded guided-tour semantic response.
// Validation, repository references, and materialization remain local to guidedtour.
func (c *Client) EditGuidedTourMeasured(
	ctx context.Context,
	prompt guidedtour.Prompt,
) (modelresearch.ProviderResult, error) {
	stopWaiting := c.startWaitProgress(ctx, "guided tour editing")
	defer stopWaiting()
	request, err := c.guidedTourRequest(prompt)
	if err != nil {
		return modelresearch.ProviderResult{}, err
	}
	body, err := json.Marshal(request)
	if err != nil {
		return modelresearch.ProviderResult{}, fmt.Errorf("llm: encode guided tour request: %w", err)
	}
	var measured modelresearch.ProviderResult
	for attempt := 1; attempt <= 2; attempt++ {
		result, retryableTransport, callErr := doChatMeasured(
			ctx,
			c.HTTPClient,
			c.Endpoint,
			c.APIKey,
			c.Auth,
			body,
			true,
		)
		measured.Attempts = attempt
		measured.RequestBytes += len(body)
		measured.ResponseBytes += result.ResponseBytes
		measured.UsageReported = measured.UsageReported || result.UsageReported
		measured.InputTokens += result.InputTokens
		measured.OutputTokens += result.OutputTokens
		measured.ReasoningTokens += result.ReasoningTokens
		measured.PromptCacheHitTokens += result.PromptCacheHitTokens
		measured.PromptCacheMissTokens += result.PromptCacheMissTokens
		measured.Content = append([]byte(nil), result.Content...)
		measured.FinishReason = result.FinishReason
		if callErr == nil {
			return measured, nil
		}
		if !retryableTransport || attempt == 2 {
			return measured, annotateResourceLimit(callErr, "guided_tour", request.MaxTokens)
		}
		select {
		case <-ctx.Done():
			return measured, ctx.Err()
		case <-time.After(backoffDuration(attempt)):
		}
	}
	return measured, fmt.Errorf("llm: guided tour retry loop ended unexpectedly")
}

func validateGuidedTourPrompt(prompt guidedtour.Prompt) error {
	switch prompt.Version {
	case guidedtour.PromptVersion, guidedtour.LeafPromptVersion, guidedtour.FanInPromptVersion:
		// All guided-tour stages use the same bounded chat request contract.
	default:
		return fmt.Errorf("llm: unsupported guided tour prompt version %q", prompt.Version)
	}
	if strings.TrimSpace(prompt.System) == "" {
		return fmt.Errorf("llm: guided tour system prompt is required")
	}
	if strings.TrimSpace(prompt.User) == "" {
		return fmt.Errorf("llm: guided tour user prompt is required")
	}
	return nil
}
