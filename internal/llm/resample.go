package llm

import (
	"context"
	"errors"
)

// executeJSONResampled makes one more draw of an opted-in call whose live
// answer was refused whole. A refused answer is never cached, so the second
// draw sends the same prepared bytes; an accepted second draw is cached under
// the same key. Both exchanges reach the observer, and the outcome carries
// their summed measurements and every issue. Replay calls executeLive
// directly and is never resampled.
func executeJSONResampled[T any](ctx context.Context, executor Executor, provider Provider, call Call[T]) (Outcome[T], error) {
	first, err := executeJSON(ctx, executor, provider, call, false)
	if err == nil || !resamples(call, first, err) {
		return first, err
	}
	second, err := executeJSON(ctx, executor, provider, call, false)
	second.Metrics = addMetrics(first.Metrics, second.Metrics)
	second.Issues = append(first.Issues, second.Issues...)
	return second, err
}

// resamples says whether a refused live answer is worth a second draw: the
// model's answer, not the request or the transport, was at fault, and the
// owner has no other recovery for it.
func resamples[T any](call Call[T], outcome Outcome[T], err error) bool {
	if !call.Resample || outcome.Cached || call.SplitRejectedResponse || call.SplitHTTP500 || call.Limits.AttemptTimeout > 0 {
		return false
	}
	if rejectedAdaptiveResponse(outcome) {
		return true // the owner's decoder or validator refused the whole answer
	}
	if outcome.FinishReason == FinishLength {
		return !call.divisible // cut at the output-token cap
	}
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr.Operation != "complete" {
		return false
	}
	failure := providerErr.ProviderFailure()
	switch failure.Kind {
	case ProviderFailureResource:
		return failure.ResourceKind == ResourceLimitOutputTokens && !call.divisible
	case ProviderFailureResponse:
		// An empty answer, an undecodable provider envelope or a completion
		// that did not stop; a content filter would refuse the same bytes.
		return outcome.FinishReason != FinishContentFilter
	default:
		return false
	}
}

func addMetrics(first, second Metrics) Metrics {
	return Metrics{
		InputTokens:           first.InputTokens + second.InputTokens,
		OutputTokens:          first.OutputTokens + second.OutputTokens,
		ReasoningTokens:       first.ReasoningTokens + second.ReasoningTokens,
		PromptCacheHitTokens:  first.PromptCacheHitTokens + second.PromptCacheHitTokens,
		PromptCacheMissTokens: first.PromptCacheMissTokens + second.PromptCacheMissTokens,
		ProviderResponseBytes: first.ProviderResponseBytes + second.ProviderResponseBytes,
		UsageReported:         first.UsageReported && second.UsageReported,
		Latency:               first.Latency + second.Latency,
		Attempts:              first.Attempts + second.Attempts,
	}
}
