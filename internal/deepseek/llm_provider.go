package deepseek

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/llm"
)

const (
	llmProviderContract         = "repomap.deepseek.openai-compatible.v1"
	llmProviderStage            = "model_completion"
	llmProviderHeartbeat        = "model completion"
	llmProviderRequestByteLimit = llm.SemanticRecordByteLimit
)

var _ llm.Provider = (*Client)(nil)

// State returns the stable, non-secret configuration that can affect an exact
// prepared request or its live transport. The API key, callbacks, and client
// identity are deliberately excluded.
func (c *Client) State() []byte {
	type providerState struct {
		Contract             string  `json:"contract"`
		Endpoint             string  `json:"endpoint,omitempty"`
		Model                string  `json:"model,omitempty"`
		AuthMode             string  `json:"auth_mode,omitempty"`
		TimeoutNanoseconds   int64   `json:"timeout_nanoseconds,omitempty"`
		ProviderMaxTokens    int     `json:"provider_max_tokens,omitempty"`
		Temperature          float64 `json:"temperature"`
		TransportAttemptsMax int     `json:"transport_attempts_max"`
		RequestByteLimit     int     `json:"request_byte_limit"`
		ResponseByteLimit    int     `json:"response_byte_limit"`
		Invalid              string  `json:"invalid,omitempty"`
	}

	state := providerState{
		Contract:             llmProviderContract,
		Temperature:          0.1,
		TransportAttemptsMax: maxRetries + 1,
		RequestByteLimit:     llmProviderRequestByteLimit,
		ResponseByteLimit:    maxProviderResponseBytes,
	}
	if c == nil {
		state.Invalid = "client_missing"
	} else {
		config := c.EffectiveConfig()
		state.Endpoint = config.Endpoint
		state.Model = config.Model
		state.AuthMode = config.AuthMode
		state.TimeoutNanoseconds = config.Timeout.Nanoseconds()
		state.ProviderMaxTokens = config.MaxTokens
		if err := validateLLMProviderConfig(c); err != nil {
			state.Endpoint = ""
			state.Model = ""
			state.Invalid = "configuration_invalid"
		}
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		// providerState contains only scalar fields, so this is defensive and
		// keeps State's interface total without exposing runtime configuration.
		return []byte(`{"contract":"repomap.deepseek.openai-compatible.v1","invalid":"state_encode_failed"}`)
	}
	return encoded
}

// Prepare deterministically turns cube-owned prompt text into one exact
// OpenAI-compatible request. It adds no semantic or language instructions.
func (c *Client) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	if err := validateLLMProviderConfig(c); err != nil {
		return llm.Prepared{}, err
	}
	if strings.TrimSpace(prompt.System) == "" {
		return llm.Prepared{}, errors.New("deepseek llm provider: system prompt is required")
	}
	if strings.TrimSpace(prompt.User) == "" {
		return llm.Prepared{}, errors.New("deepseek llm provider: user prompt is required")
	}
	if limits.MaxRequestBytes <= 0 || limits.MaxResponseBytes <= 0 || limits.MaxOutputTokens <= 0 {
		return llm.Prepared{}, errors.New("deepseek llm provider: positive request, response, and output limits are required")
	}
	if limits.MaxResponseBytes > maxProviderResponseBytes {
		return llm.Prepared{}, newResourceLimitError(ResourceLimitError{
			Stage: llmProviderStage, Kind: ResourceLimitResponseBytes,
			Limit: maxProviderResponseBytes, Observed: limits.MaxResponseBytes, ObservedKnown: true,
			ConfiguredMaxTokens: c.MaxTokens,
		})
	}

	maxOutputTokens := min(limits.MaxOutputTokens, c.MaxTokens)
	request := c.semanticRequest(prompt.User, prompt.System, prompt.ResponseFormatJSON)
	request.MaxTokens = maxOutputTokens
	if isOfficialDeepSeekEndpoint(c.Endpoint) {
		// Bounded structured cubes need an answer inside their explicit output
		// budget. Official DeepSeek endpoints otherwise enable thinking by
		// default and can consume that budget without returning content.
		request.Thinking = &thinkingConfig{Type: "disabled"}
	}
	body, err := json.Marshal(request)
	if err != nil {
		return llm.Prepared{}, errors.New("deepseek llm provider: encode request failed")
	}
	requestLimit := min(limits.MaxRequestBytes, llmProviderRequestByteLimit)
	if len(body) > requestLimit {
		return llm.Prepared{}, newResourceLimitError(ResourceLimitError{
			Stage: llmProviderStage, Kind: ResourceLimitRequestBytes,
			Limit: requestLimit, Observed: len(body), ObservedKnown: true,
			ConfiguredMaxTokens: c.MaxTokens,
		})
	}
	return llm.NewPrepared(body)
}

// Complete sends exactly the immutable bytes returned by Prepare. Only
// retryable transport failures are replayed, and every retry reuses those same
// bytes. Provider envelope decoding remains transport-owned; domain JSON
// normalization and validation remain llm.Executor and cube responsibilities.
func (c *Client) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	if err := validateLLMProviderConfig(c); err != nil {
		return llm.Completion{}, err
	}
	body := prepared.Bytes()
	if len(body) == 0 {
		return llm.Completion{}, errors.New("deepseek llm provider: prepared request is empty")
	}
	if len(body) > llmProviderRequestByteLimit {
		return llm.Completion{}, newResourceLimitError(ResourceLimitError{
			Stage: llmProviderStage, Kind: ResourceLimitRequestBytes,
			Limit: llmProviderRequestByteLimit, Observed: len(body), ObservedKnown: true,
			ConfiguredMaxTokens: c.MaxTokens,
		})
	}
	if !json.Valid(body) {
		return llm.Completion{}, errors.New("deepseek llm provider: prepared request is not valid JSON")
	}
	if err := ctx.Err(); err != nil {
		return llm.Completion{}, err
	}

	stopWaiting := c.startWaitProgress(ctx, llmProviderHeartbeat)
	defer stopWaiting()
	started := time.Now()
	var (
		last          chatCompletion
		lastErr       error
		attempts      int
		responseBytes int
	)
	for attempt := 1; attempt <= maxRetries+1; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				completion := llmCompletion(last, attempts, responseBytes, time.Since(started))
				return completion, closedLLMProviderError("complete", ctx.Err())
			case <-time.After(backoffDuration(attempt - 1)):
			}
		}

		completion, retryable, err := doChatMeasured(
			ctx, c.HTTPClient, c.Endpoint, c.APIKey, c.Auth, body,
		)
		attempts = attempt
		responseBytes += completion.ResponseBytes
		last = completion
		if err == nil {
			if completionErr := requireSingleStoppedCompletion(llmProviderStage, completion); completionErr != nil {
				result := llmCompletion(completion, attempts, responseBytes, time.Since(started))
				return result, closedLLMProviderError("complete", completionErr)
			}
			return llmCompletion(completion, attempts, responseBytes, time.Since(started)), nil
		}
		lastErr = annotateIncompleteCompletion(err, llmProviderStage)
		lastErr = annotateResourceLimit(lastErr, llmProviderStage, c.MaxTokens)
		if !retryable {
			result := llmCompletion(completion, attempts, responseBytes, time.Since(started))
			return result, closedLLMProviderError("complete", lastErr)
		}
	}

	result := llmCompletion(last, attempts, responseBytes, time.Since(started))
	exhausted := fmt.Errorf("transport retries exhausted after %d attempts: %w", attempts, lastErr)
	return result, closedLLMProviderError("complete", exhausted)
}

func validateLLMProviderConfig(c *Client) error {
	if c == nil {
		return errors.New("deepseek llm provider: client is required")
	}
	if c.HTTPClient == nil {
		return errors.New("deepseek llm provider: HTTP client is required")
	}
	if strings.TrimSpace(c.Model) == "" {
		return errors.New("deepseek llm provider: model is required")
	}
	if c.MaxTokens <= 0 {
		return errors.New("deepseek llm provider: provider output-token ceiling must be positive")
	}
	if err := validateEndpoint(c.Endpoint); err != nil {
		return errors.New("deepseek llm provider: endpoint is invalid")
	}
	auth := c.Auth
	if auth == "" {
		auth = authBearer
	}
	switch auth {
	case authBearer:
		if c.APIKey == "" {
			return errors.New("deepseek llm provider: bearer API key is required")
		}
	case authNone:
		// No Authorization header is sent.
	default:
		return errors.New("deepseek llm provider: authentication mode is invalid")
	}
	return nil
}

func llmCompletion(
	completion chatCompletion,
	attempts int,
	providerResponseBytes int,
	latency time.Duration,
) llm.Completion {
	return llm.Completion{
		Response:     append([]byte(nil), completion.Content...),
		FinishReason: llmFinishReason(completion.finishReasonClass),
		ChoiceCount:  completion.ChoiceCount,
		Metrics: llm.Metrics{
			InputTokens: completion.InputTokens, OutputTokens: completion.OutputTokens,
			ReasoningTokens:       completion.ReasoningTokens,
			PromptCacheHitTokens:  completion.PromptCacheHitTokens,
			PromptCacheMissTokens: completion.PromptCacheMissTokens,
			ProviderResponseBytes: providerResponseBytes,
			UsageReported:         completion.UsageReported,
			Latency:               latency, Attempts: attempts,
		},
	}
}

func llmFinishReason(reason string) llm.FinishReason {
	switch reason {
	case "stop":
		return llm.FinishStop
	case "length":
		return llm.FinishLength
	case "content_filter":
		return llm.FinishContentFilter
	case "tool_calls":
		return llm.FinishToolCalls
	case "insufficient_system_resource":
		return llm.FinishInsufficientSystemResource
	default:
		return llm.FinishUnknown
	}
}

type llmProviderError struct {
	operation string
	cause     error
}

func closedLLMProviderError(operation string, cause error) error {
	if cause == nil {
		return nil
	}
	return &llmProviderError{operation: operation, cause: cause}
}

func (err *llmProviderError) Error() string {
	if err == nil || err.operation == "" {
		return "deepseek llm provider operation failed"
	}
	return "deepseek llm provider " + err.operation + " failed"
}

func (err *llmProviderError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}
