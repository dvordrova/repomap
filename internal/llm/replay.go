package llm

import (
	"context"
	"encoding/json"
	"fmt"
)

// ReplayJSON always makes a live call and refreshes the exact-request cache.
// It knows JSON and the transport contract, not a repository stage's schema.
// ExecuteJSON revalidates this response against its owning stage on reuse.
func ReplayJSON(ctx context.Context, executor Executor, provider Provider, prepared Prepared) (Outcome[json.RawMessage], error) {
	var outcome Outcome[json.RawMessage]
	if provider == nil || !executor.Enabled {
		return outcome, fmt.Errorf("llm: replay requires a provider and an enabled cache")
	}
	limits := Limits{MaxRequestBytes: SemanticRecordByteLimit, MaxResponseBytes: ProviderResponseByteLimit, MaxOutputTokens: DefaultMaxOutputTokens}
	request := prepared.Bytes()
	if len(request) == 0 || len(request) > limits.MaxRequestBytes {
		return outcome, fmt.Errorf("llm: replay request is empty or exceeds the provider envelope")
	}
	state, err := canonicalProviderState(provider.State())
	if err != nil {
		return outcome, err
	}
	setOutcomeRequest(&outcome, request)
	outcome.CacheKey = executionCacheKey(state, nil, request)
	if _, err := SavePayload(executor.RootDir, request); err != nil {
		return outcome, err
	}
	outcome, callErr := executeLive(bindExecutorAttemptGate(ctx, executor), executor, provider, prepared, DecodeJSON[json.RawMessage](nil), limits, outcome)
	if len(outcome.Response) > 0 {
		if _, err := SavePayload(executor.RootDir, outcome.Response); err != nil {
			return outcome, err
		}
	}
	for _, issue := range outcome.Issues {
		if issue.Kind == IssueCacheWrite {
			return outcome, fmt.Errorf("llm: replay could not refresh cache: %w", issue.Err)
		}
	}
	return outcome, callErr
}
