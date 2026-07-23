package deepseek

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/tasklens"
)

const TaskInvestigationMinMaxTokens = 10_000

// DefaultTaskInvestigationConfig identifies the synthesis route without
// consulting provider environment or retaining credentials. It is used when
// local evidence skips the provider but run metadata still needs a stable
// provider/model identity and comparable request limits.
func DefaultTaskInvestigationConfig() EffectiveConfig {
	maxTokens := defaultMaxTokens
	if maxTokens < TaskInvestigationMinMaxTokens {
		maxTokens = TaskInvestigationMinMaxTokens
	}
	return EffectiveConfig{
		Endpoint:  defaultEndpoint,
		Model:     defaultModel,
		AuthMode:  authBearer,
		Timeout:   defaultTimeout,
		MaxTokens: maxTokens,
	}
}

// TaskInvestigationPromptJSON returns the exact provider envelope for the one
// bounded Task Lens synthesis call without sending it.
func (c *Client) TaskInvestigationPromptJSON(bundle tasklens.Bundle) ([]byte, error) {
	prompt, err := tasklens.BuildSynthesisPrompt(bundle)
	if err != nil {
		return nil, err
	}
	request := c.flowExplainRequest(prompt.User, prompt.System, true)
	if isOfficialDeepSeekEndpoint(c.Endpoint) {
		request.Temperature = nil
		request.Thinking = &thinkingConfig{Type: "enabled"}
		request.ReasoningEffort = prompt.ThinkingProfile
		if request.MaxTokens < TaskInvestigationMinMaxTokens {
			request.MaxTokens = TaskInvestigationMinMaxTokens
		}
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("task lens: marshal provider request: %w", err)
	}
	return body, nil
}

func (c *Client) TaskInvestigationMaxTokens() int {
	if isOfficialDeepSeekEndpoint(c.Endpoint) && c.MaxTokens < TaskInvestigationMinMaxTokens {
		return TaskInvestigationMinMaxTokens
	}
	return c.MaxTokens
}

// InvestigateTaskMeasured makes one semantic attempt. Bounded transport
// retries may resend the same request, but a substantive model response is
// never retried; local decoding and reduction decide whether to publish it.
func (c *Client) InvestigateTaskMeasured(
	ctx context.Context,
	bundle tasklens.Bundle,
) (modelresearch.ProviderResult, error) {
	stopWaiting := c.startWaitProgress(ctx, "task investigation synthesis")
	defer stopWaiting()
	body, err := c.TaskInvestigationPromptJSON(bundle)
	if err != nil {
		return modelresearch.ProviderResult{}, err
	}

	var lastErr error
	var usage modelresearch.ProviderResult
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := backoffDuration(attempt)
			select {
			case <-ctx.Done():
				usage.Attempts = attempt
				return usage, ctx.Err()
			case <-time.After(backoff):
			}
		}

		result, shouldRetry, err := doChatMeasured(
			ctx, c.HTTPClient, c.Endpoint, c.APIKey, c.Auth, body, false,
		)
		usage.RequestBytes += len(body)
		usage.InputTokens += result.InputTokens
		usage.OutputTokens += result.OutputTokens
		usage.PromptCacheHitTokens += result.PromptCacheHitTokens
		usage.PromptCacheMissTokens += result.PromptCacheMissTokens
		if err == nil {
			return modelresearch.ProviderResult{
				Content: result.Content, Attempts: attempt + 1, RequestBytes: usage.RequestBytes,
				InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
				PromptCacheHitTokens:  usage.PromptCacheHitTokens,
				PromptCacheMissTokens: usage.PromptCacheMissTokens,
			}, nil
		}
		lastErr = err
		if !shouldRetry {
			usage.Attempts = attempt + 1
			return usage, err
		}
	}

	usage.Attempts = maxRetries + 1
	return usage, fmt.Errorf(
		"retries exhausted (%d attempts): %w", maxRetries+1, lastErr,
	)
}
