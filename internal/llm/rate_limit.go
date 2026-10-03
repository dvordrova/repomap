package llm

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// MinimumRateLimitCooldown is the shortest pause of every new attempt
// through the shared gate after a provider's HTTP 429.
const MinimumRateLimitCooldown = time.Minute

// RateLimitCooldown is how long new attempts through the shared gate wait
// after an HTTP 429: at least a minute, longer when the response's
// Retry-After (seconds or an HTTP date) or a "retry after"/"reset after"
// duration in its error message asks for more. A provider passes it to
// BackoffProviderAttempts before it releases the failed attempt's lease and
// waits it before its own retry. Invalid, negative, unitless and past values
// are ignored.
func RateLimitCooldown(retryAfter string, body []byte, now time.Time) time.Duration {
	return max(MinimumRateLimitCooldown, RetryAfterDelay(retryAfter, now), rateLimitBodyDelay(body))
}

// RetryAfterDelay reads an HTTP Retry-After value: whole seconds, or an HTTP
// date measured from now. Anything else, or a date already past, is zero.
func RetryAfterDelay(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.ParseUint(value, 10, 64); err == nil {
		// Saturate before converting seconds to nanoseconds; a long server wait
		// must never overflow into an immediate retry.
		const maxDuration = time.Duration(1<<63 - 1)
		if seconds > uint64(maxDuration/time.Second) {
			return maxDuration
		}
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil && date.After(now) {
		return date.Sub(now)
	}
	return 0
}

// Compatible servers may put relative waits in their 429 error message, for
// example "retry after 9.636307001s, reset after 45.636307001s", as plain
// text, a JSON message, a JSON string error or error.message. The longest
// positive Go duration among them extends the cooldown.
func rateLimitBodyDelay(body []byte) time.Duration {
	messages := []string{string(body)}
	var envelope struct {
		Message string          `json:"message"`
		Error   json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &envelope) == nil {
		messages = []string{envelope.Message}
		var message string
		if json.Unmarshal(envelope.Error, &message) == nil {
			messages = append(messages, message)
		} else {
			var detail struct {
				Message string `json:"message"`
			}
			if json.Unmarshal(envelope.Error, &detail) == nil {
				messages = append(messages, detail.Message)
			}
		}
	}
	var wait time.Duration
	for _, message := range messages {
		words := strings.Fields(message)
		for i := 2; i < len(words); i++ {
			if !(strings.EqualFold(words[i-2], "retry") || strings.EqualFold(words[i-2], "reset")) ||
				!strings.EqualFold(words[i-1], "after") {
				continue
			}
			if delay, err := time.ParseDuration(strings.TrimRight(words[i], ",;.")); err == nil && delay > wait {
				wait = delay
			}
		}
	}
	return wait
}

// RetryProgress is one transport retry of one exact request: a failed
// attempt that will be retried after Delay, then (Starting) the retry's start
// once the shared gate lets it go. It carries the request's digest and closed
// failure facts, never request or response bytes or credentials.
type RetryProgress struct {
	RequestSHA256 string
	Attempt       int
	MaxAttempts   int
	Starting      bool
	Failure       ProviderFailureKind
	HTTPStatus    int
	Delay         time.Duration
	Elapsed       time.Duration
}
