package deepseek

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// executeChatWithTransportRetries replays only an already-built immutable
// provider request after a retryable transport failure. Semantic response
// validation remains owned by the caller and is never retried here.
func executeChatWithTransportRetries(
	ctx context.Context,
	httpClient *http.Client,
	endpoint, apiKey, auth string,
	body []byte,
	validateJSON bool,
) (chatCompletion, int, error) {
	var (
		lastCompletion chatCompletion
		lastErr        error
	)
	for attempt := 1; attempt <= maxRetries+1; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return lastCompletion, attempt - 1, ctx.Err()
			case <-time.After(backoffDuration(attempt - 1)):
			}
		}

		completion, retryable, err := doChatMeasured(
			ctx,
			httpClient,
			endpoint,
			apiKey,
			auth,
			body,
			validateJSON,
		)
		lastCompletion = completion
		if err == nil {
			return completion, attempt, nil
		}
		lastErr = err
		if !retryable {
			return completion, attempt, err
		}
	}
	return lastCompletion, maxRetries + 1, fmt.Errorf(
		"retries exhausted (%d attempts): %w",
		maxRetries+1,
		lastErr,
	)
}
