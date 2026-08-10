package deepseek

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/entrycall"
	"github.com/dvordrova/repomap/internal/modelresearch"
)

const entryCallCompressionStage = "entry_call_compression"

// EntryCallCompressionPromptJSON builds the exact OpenAI-compatible request
// for one bounded entry-call compression without making a provider call.
func (c *Client) EntryCallCompressionPromptJSON(prompt entrycall.Prompt) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("llm: entry call compression client is required")
	}
	if err := validateEntryCallCompressionPrompt(prompt); err != nil {
		return nil, err
	}
	request := c.canonicalSemanticRequest(prompt.User, prompt.System, true)
	if isOfficialDeepSeekEndpoint(c.Endpoint) {
		request.Thinking = &thinkingConfig{Type: "disabled"}
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("llm: marshal entry call compression request: %w", err)
	}
	if len(body) > entrycall.MaxRequestBytes {
		return nil, modelresearch.NewResourceLimitError(modelresearch.ResourceLimitError{
			Stage: entryCallCompressionStage, Kind: modelresearch.ResourceLimitRequestBytes,
			Limit: entrycall.MaxRequestBytes, Observed: len(body), ObservedKnown: true,
			ConfiguredMaxTokens: c.MaxTokens,
		}, nil)
	}
	return body, nil
}

// EntryCallCompressionBodyMeasured sends an already-built exact provider
// body. Only retryable transport failures may replay these immutable bytes;
// semantic response validation remains a single terminal decision in
// entrycall.Reduce.
func (c *Client) EntryCallCompressionBodyMeasured(
	ctx context.Context,
	exactBody []byte,
) (modelresearch.ProviderResult, error) {
	if c == nil || c.HTTPClient == nil {
		return modelresearch.ProviderResult{}, fmt.Errorf("llm: entry call compression request client is required")
	}
	if len(exactBody) == 0 {
		return modelresearch.ProviderResult{}, fmt.Errorf("llm: entry call compression request body is required")
	}
	if len(exactBody) > entrycall.MaxRequestBytes {
		return modelresearch.ProviderResult{}, modelresearch.NewResourceLimitError(modelresearch.ResourceLimitError{
			Stage: entryCallCompressionStage, Kind: modelresearch.ResourceLimitRequestBytes,
			Limit: entrycall.MaxRequestBytes, Observed: len(exactBody), ObservedKnown: true,
			ConfiguredMaxTokens: c.MaxTokens,
		}, nil)
	}
	stopWaiting := c.startWaitProgress(ctx, "Entrypoint call compression")
	defer stopWaiting()
	completion, attempts, callErr := executeChatWithTransportRetries(
		ctx, c.HTTPClient, c.Endpoint, c.APIKey, c.Auth, exactBody, false,
	)
	result := providerResultFromCompletion(completion, attempts, len(exactBody)*attempts)
	if callErr != nil {
		callErr = annotateIncompleteCompletion(callErr, entryCallCompressionStage)
		return result, annotateResourceLimit(callErr, entryCallCompressionStage, c.MaxTokens)
	}
	if err := requireSingleStoppedCompletion(entryCallCompressionStage, completion); err != nil {
		return result, err
	}
	if len(result.Content) > entrycall.MaxResponseBytes {
		return result, modelresearch.NewResourceLimitError(modelresearch.ResourceLimitError{
			Stage: entryCallCompressionStage, Kind: modelresearch.ResourceLimitResponseBytes,
			Limit: entrycall.MaxResponseBytes, Observed: len(result.Content), ObservedKnown: true,
			ConfiguredMaxTokens: c.MaxTokens,
			InputTokens:         result.InputTokens, OutputTokens: result.OutputTokens,
			ReasoningTokens: result.ReasoningTokens, FinishReason: result.FinishReason,
		}, result.Content)
	}
	return result, nil
}

func validateEntryCallCompressionPrompt(prompt entrycall.Prompt) error {
	if prompt.Version != entrycall.PromptVersion {
		return fmt.Errorf("llm: unsupported entry call compression prompt version %q", prompt.Version)
	}
	if strings.TrimSpace(prompt.System) == "" {
		return fmt.Errorf("llm: entry call compression system prompt is required")
	}
	if strings.TrimSpace(prompt.User) == "" {
		return fmt.Errorf("llm: entry call compression user prompt is required")
	}
	return nil
}
