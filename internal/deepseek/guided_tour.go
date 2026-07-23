package deepseek

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/guidedtour"
	"github.com/dvordrova/repomap/internal/modelresearch"
)

const guidedTourFanInMinMaxTokens = 12_000

// GuidedTourPromptJSON returns the exact OpenAI-compatible request used by
// EditGuidedTourMeasured without making a provider call.
func (c *Client) GuidedTourPromptJSON(prompt guidedtour.Prompt) ([]byte, error) {
	if err := validateGuidedTourPrompt(prompt); err != nil {
		return nil, err
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
		if prompt.Version == guidedtour.FanInPromptVersion && request.MaxTokens < guidedTourFanInMinMaxTokens {
			request.MaxTokens = guidedTourFanInMinMaxTokens
		}
	}
	return json.Marshal(request)
}

// EditGuidedTourMeasured asks for one bounded guided-tour semantic response.
// Validation, repository references, and materialization remain local to guidedtour.
func (c *Client) EditGuidedTourMeasured(
	ctx context.Context,
	prompt guidedtour.Prompt,
) (modelresearch.ProviderResult, error) {
	stopWaiting := c.startWaitProgress(ctx, "guided tour editing")
	defer stopWaiting()
	body, err := c.GuidedTourPromptJSON(prompt)
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
	return modelresearch.ProviderResult{
		Content: result.Content, Attempts: 1,
		InputTokens: result.InputTokens, OutputTokens: result.OutputTokens,
		PromptCacheHitTokens:  result.PromptCacheHitTokens,
		PromptCacheMissTokens: result.PromptCacheMissTokens,
	}, err
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
