package deepseek

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/themestudy"
)

// ThemeScoutPromptJSON builds the exact OpenAI-compatible request for the
// bounded Theme Scout question without making a provider call. The themestudy
// package owns the short-ref catalog and response semantics. Both theme stages
// are bounded classification, so the official DeepSeek endpoint receives
// explicit disabled thinking (docs/DEEPSEEK_API_NOTES.md).
func (c *Client) ThemeScoutPromptJSON(
	prompt themestudy.ScoutPrompt,
	maxRequestBytes int,
) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("llm: theme scout client is required")
	}
	if maxRequestBytes <= 0 {
		return nil, fmt.Errorf("llm: theme scout request byte limit must be positive")
	}
	if prompt.Version != themestudy.ScoutPromptVersion {
		return nil, fmt.Errorf("llm: unsupported theme scout prompt version %q", prompt.Version)
	}
	if prompt.Language != themestudy.LanguageEnglish && prompt.Language != themestudy.LanguageRussian {
		return nil, fmt.Errorf("llm: unsupported theme scout prompt language %q", prompt.Language)
	}
	if strings.TrimSpace(prompt.System) == "" || strings.TrimSpace(prompt.User) == "" {
		return nil, fmt.Errorf("llm: theme scout prompt is incomplete")
	}
	request := c.semanticRequest(prompt.User, prompt.System, true)
	if isOfficialDeepSeekEndpoint(c.Endpoint) {
		request.Thinking = &thinkingConfig{Type: "disabled"}
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("llm: marshal theme scout request: %w", err)
	}
	if len(body) > maxRequestBytes {
		return nil, modelresearch.NewResourceLimitError(modelresearch.ResourceLimitError{
			Stage: themestudy.ScoutStage, Kind: modelresearch.ResourceLimitRequestBytes,
			Limit: maxRequestBytes, Observed: len(body), ObservedKnown: true,
			ConfiguredMaxTokens: c.MaxTokens,
		}, nil)
	}
	return body, nil
}

// ThemeScoutMeasured performs one semantic Theme Scout call. Only transport
// failures may replay the exact immutable body; response decoding and short-ref
// resolution remain a single terminal local decision in themestudy.
func (c *Client) ThemeScoutMeasured(
	ctx context.Context,
	prompt themestudy.ScoutPrompt,
	maxRequestBytes int,
) (modelresearch.ProviderResult, error) {
	if c == nil || c.HTTPClient == nil {
		return modelresearch.ProviderResult{}, fmt.Errorf("llm: theme scout request client is required")
	}
	body, err := c.ThemeScoutPromptJSON(prompt, maxRequestBytes)
	if err != nil {
		return modelresearch.ProviderResult{}, err
	}
	stopWaiting := c.startWaitProgress(ctx, "Theme Scout")
	defer stopWaiting()
	completion, attempts, callErr := executeChatWithTransportRetries(
		ctx, c.HTTPClient, c.Endpoint, c.APIKey, c.Auth, body, false,
	)
	result := providerResultFromCompletion(completion, attempts, len(body)*attempts)
	if callErr != nil {
		callErr = annotateIncompleteCompletion(callErr, themestudy.ScoutStage)
		return result, annotateResourceLimit(callErr, themestudy.ScoutStage, c.MaxTokens)
	}
	if err := requireSingleStoppedCompletion(themestudy.ScoutStage, completion); err != nil {
		return result, err
	}
	return result, nil
}

// ThemeAdjudicationPromptJSON builds the exact OpenAI-compatible request for
// the bounded Source Review / Theme Adjudication question without making a
// provider call.
func (c *Client) ThemeAdjudicationPromptJSON(
	prompt themestudy.AdjudicationPrompt,
	maxRequestBytes int,
) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("llm: theme adjudication client is required")
	}
	if maxRequestBytes <= 0 {
		return nil, fmt.Errorf("llm: theme adjudication request byte limit must be positive")
	}
	if prompt.Version != themestudy.AdjudicationPromptVersion {
		return nil, fmt.Errorf("llm: unsupported theme adjudication prompt version %q", prompt.Version)
	}
	if prompt.Language != themestudy.LanguageEnglish && prompt.Language != themestudy.LanguageRussian {
		return nil, fmt.Errorf("llm: unsupported theme adjudication prompt language %q", prompt.Language)
	}
	if strings.TrimSpace(prompt.System) == "" || strings.TrimSpace(prompt.User) == "" {
		return nil, fmt.Errorf("llm: theme adjudication prompt is incomplete")
	}
	request := c.semanticRequest(prompt.User, prompt.System, true)
	if isOfficialDeepSeekEndpoint(c.Endpoint) {
		request.Thinking = &thinkingConfig{Type: "disabled"}
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("llm: marshal theme adjudication request: %w", err)
	}
	if len(body) > maxRequestBytes {
		return nil, modelresearch.NewResourceLimitError(modelresearch.ResourceLimitError{
			Stage: themestudy.AdjudicationStage, Kind: modelresearch.ResourceLimitRequestBytes,
			Limit: maxRequestBytes, Observed: len(body), ObservedKnown: true,
			ConfiguredMaxTokens: c.MaxTokens,
		}, nil)
	}
	return body, nil
}

// ThemeAdjudicationMeasured performs one semantic Theme Adjudication call.
func (c *Client) ThemeAdjudicationMeasured(
	ctx context.Context,
	prompt themestudy.AdjudicationPrompt,
	maxRequestBytes int,
) (modelresearch.ProviderResult, error) {
	if c == nil || c.HTTPClient == nil {
		return modelresearch.ProviderResult{}, fmt.Errorf("llm: theme adjudication request client is required")
	}
	body, err := c.ThemeAdjudicationPromptJSON(prompt, maxRequestBytes)
	if err != nil {
		return modelresearch.ProviderResult{}, err
	}
	stopWaiting := c.startWaitProgress(ctx, "Source Review / Theme Adjudication")
	defer stopWaiting()
	completion, attempts, callErr := executeChatWithTransportRetries(
		ctx, c.HTTPClient, c.Endpoint, c.APIKey, c.Auth, body, false,
	)
	result := providerResultFromCompletion(completion, attempts, len(body)*attempts)
	if callErr != nil {
		callErr = annotateIncompleteCompletion(callErr, themestudy.AdjudicationStage)
		return result, annotateResourceLimit(callErr, themestudy.AdjudicationStage, c.MaxTokens)
	}
	if err := requireSingleStoppedCompletion(themestudy.AdjudicationStage, completion); err != nil {
		return result, err
	}
	return result, nil
}
