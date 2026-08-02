package deepseek

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/modelresearch"
)

// AtlasStudyPromptJSON builds the exact OpenAI-compatible request for the
// single Atlas-backed Brief and Study question without making a provider call.
// The atlasstudy package owns the short-ref catalog and response semantics.
func (c *Client) AtlasStudyPromptJSON(
	prompt atlasstudy.Prompt,
	maxRequestBytes int,
) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("llm: Atlas Study client is required")
	}
	if maxRequestBytes <= 0 {
		return nil, fmt.Errorf("llm: Atlas Study request byte limit must be positive")
	}
	if prompt.Version != atlasstudy.PromptVersion {
		return nil, fmt.Errorf("llm: unsupported Atlas Study prompt version %q", prompt.Version)
	}
	if strings.TrimSpace(prompt.System) == "" || strings.TrimSpace(prompt.User) == "" {
		return nil, fmt.Errorf("llm: Atlas Study prompt is incomplete")
	}
	request := c.canonicalSemanticRequest(prompt.User, prompt.System, true)
	if isOfficialDeepSeekEndpoint(c.Endpoint) {
		request.Thinking = &thinkingConfig{Type: "disabled"}
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("llm: marshal Atlas Study request: %w", err)
	}
	if len(body) > maxRequestBytes {
		return nil, modelresearch.NewResourceLimitError(modelresearch.ResourceLimitError{
			Stage: "atlas_study", Kind: modelresearch.ResourceLimitRequestBytes,
			Limit: maxRequestBytes, Observed: len(body), ObservedKnown: true,
			ConfiguredMaxTokens: c.MaxTokens,
		}, nil)
	}
	return body, nil
}

// AtlasStudyMeasured performs one semantic call. Only transport failures may
// replay the exact immutable body; response decoding and short-ref resolution
// remain a single terminal local decision in atlasstudy.Product.
func (c *Client) AtlasStudyMeasured(
	ctx context.Context,
	prompt atlasstudy.Prompt,
	maxRequestBytes int,
) (modelresearch.ProviderResult, error) {
	if c == nil || c.HTTPClient == nil {
		return modelresearch.ProviderResult{}, fmt.Errorf("llm: Atlas Study request client is required")
	}
	body, err := c.AtlasStudyPromptJSON(prompt, maxRequestBytes)
	if err != nil {
		return modelresearch.ProviderResult{}, err
	}
	stopWaiting := c.startWaitProgress(ctx, "Atlas-backed Brief and Study")
	defer stopWaiting()
	completion, attempts, callErr := executeChatWithTransportRetries(
		ctx, c.HTTPClient, c.Endpoint, c.APIKey, c.Auth, body, true,
	)
	result := providerResultFromCompletion(completion, attempts, len(body)*attempts)
	if callErr != nil {
		return result, annotateResourceLimit(callErr, "atlas_study", c.MaxTokens)
	}
	return result, nil
}
