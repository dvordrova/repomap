package deepseek

import (
	"context"
	"fmt"

	"github.com/dvordrova/repomap/internal/localization"
	"github.com/dvordrova/repomap/internal/modelresearch"
)

// ExecuteLocalizationRequest executes one already-built, exact localization
// request. The prompt is required only to revalidate that the immutable
// evidence still describes the intended translation request before any
// network activity occurs.
func (c *Client) ExecuteLocalizationRequest(
	ctx context.Context,
	prompt localization.Prompt,
	evidence LocalizationRequestEvidence,
) (modelresearch.ProviderResult, error) {
	if err := evidence.Validate(prompt); err != nil {
		return modelresearch.ProviderResult{}, err
	}
	if c == nil || c.HTTPClient == nil {
		return modelresearch.ProviderResult{}, fmt.Errorf(
			"llm: localization request client is required",
		)
	}

	stopWaiting := c.startWaitProgress(ctx, "localization")
	defer stopWaiting()

	completion, attempts, err := executeChatWithTransportRetries(
		ctx,
		c.HTTPClient,
		evidence.Endpoint,
		c.APIKey,
		evidence.AuthMode,
		evidence.Body,
		true,
	)
	if err != nil {
		return modelresearch.ProviderResult{
			Attempts:     attempts,
			RequestBytes: len(evidence.Body) * attempts,
		}, err
	}
	return modelresearch.ProviderResult{
		Content:               completion.Content,
		Attempts:              attempts,
		RequestBytes:          len(evidence.Body) * attempts,
		InputTokens:           completion.InputTokens,
		OutputTokens:          completion.OutputTokens,
		PromptCacheHitTokens:  completion.PromptCacheHitTokens,
		PromptCacheMissTokens: completion.PromptCacheMissTokens,
	}, nil
}
