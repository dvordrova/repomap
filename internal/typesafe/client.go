// Package typesafe is the llm.Provider for TypeSafe's System One models
// (Jev): typed closed decisions with probabilities instead of generated text.
// The owner prepares the exact evaluation body; this client adds the model,
// sends it, and returns the answers object for the owner to validate.
package typesafe

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/llm"
)

const (
	defaultEndpoint = "https://api.typesafe.ai/v1/systemone"
	// The version is pinned: an alias moves with a release and would change
	// cached decisions and tuned confidence thresholds silently.
	defaultModel   = "jev-1.13.0"
	defaultTimeout = 2 * time.Minute
	maxAttempts    = 5

	envAPIKey   = "JEV_KEY"
	envModel    = "REPOMAP_JEV_MODEL"
	envEndpoint = "REPOMAP_JEV_ENDPOINT"
)

// Client sends System One evaluations. Its zero value is not usable; build
// it with NewFromEnv.
type Client struct {
	HTTPClient *http.Client
	Endpoint   string
	Model      string
	APIKey     string
	// OnRetry reports a failed attempt that will be retried: its status
	// (0 for a transport error), reason and the wait before the next one.
	// It never sees the request or the key.
	OnRetry func(attempt, status int, reason string, wait time.Duration)
}

// transport keeps one HTTP/1.1 connection per request in flight. Over the
// default single HTTP/2 connection, 57 concurrent 100 KB evaluations took
// 42 s (15 s each) because their bodies shared one send window; on separate
// connections the same requests took 9.2 s (2 s each).
func transport() *http.Transport {
	return &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		ForceAttemptHTTP2:   false,
		TLSNextProto:        map[string]func(string, *tls.Conn) http.RoundTripper{},
		MaxIdleConns:        64,
		MaxIdleConnsPerHost: 64,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
}

// NewFromEnv returns nil without error when no key is configured: closed
// decisions then stay with the ordinary provider.
func NewFromEnv() (*Client, error) {
	key := strings.TrimSpace(os.Getenv(envAPIKey))
	if key == "" {
		return nil, nil
	}
	client := &Client{HTTPClient: &http.Client{Timeout: defaultTimeout, Transport: transport()}, Endpoint: defaultEndpoint, Model: defaultModel, APIKey: key}
	if value := strings.TrimSpace(os.Getenv(envModel)); value != "" {
		client.Model = value
	}
	if value := strings.TrimSpace(os.Getenv(envEndpoint)); value != "" {
		client.Endpoint = value
	}
	return client, nil
}

func (c *Client) State() []byte {
	state, _ := json.Marshal(map[string]string{"endpoint": c.Endpoint, "model": c.Model, "protocol": "systemone.v1"})
	return state
}

// Prepare takes the owner's evaluation body (state and questions) from the
// prompt's user part; the system part is the owner's and is already inside
// that body.
func (c *Client) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal([]byte(prompt.User), &body); err != nil {
		return llm.Prepared{}, fmt.Errorf("typesafe: evaluation body: %w", err)
	}
	if body["state"] == nil || body["questions"] == nil {
		return llm.Prepared{}, fmt.Errorf("typesafe: evaluation body needs state and questions")
	}
	body["model"], _ = json.Marshal(c.Model)
	exact, err := json.Marshal(body)
	if err != nil {
		return llm.Prepared{}, err
	}
	if limits.MaxRequestBytes > 0 && len(exact) > limits.MaxRequestBytes {
		return llm.Prepared{}, llm.NewResourceLimitError(llm.ResourceLimitError{
			Kind: llm.ResourceLimitRequestBytes, Limit: limits.MaxRequestBytes, Observed: len(exact), ObservedKnown: true,
		})
	}
	return llm.NewPrepared(exact)
}

func (c *Client) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	started := time.Now()
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(prepared.Bytes()))
		if err != nil {
			return llm.Completion{}, err
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+c.APIKey)
		response, err := c.HTTPClient.Do(request)
		if err != nil {
			if ctx.Err() != nil {
				return llm.Completion{}, ctx.Err()
			}
			lastErr = err
			wait := backoff(attempt, "")
			c.retrying(attempt, 0, err.Error(), wait)
			if !sleep(ctx, wait) {
				return llm.Completion{}, ctx.Err()
			}
			continue
		}
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, llm.ProviderResponseByteLimit+1))
		response.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 {
			lastErr = fmt.Errorf("typesafe: HTTP %d: %s", response.StatusCode, truncate(raw))
			wait := backoff(attempt, response.Header.Get("Retry-After"))
			c.retrying(attempt, response.StatusCode, truncate(raw), wait)
			if !sleep(ctx, wait) {
				return llm.Completion{}, ctx.Err()
			}
			continue
		}
		if response.StatusCode == http.StatusBadRequest && bytes.Contains(raw, []byte("max_tokens_exceeded")) {
			return llm.Completion{}, llm.NewResourceLimitError(llm.ResourceLimitError{
				Kind: llm.ResourceLimitContextTokens, HTTPStatus: response.StatusCode,
			})
		}
		if response.StatusCode != http.StatusOK {
			return llm.Completion{}, fmt.Errorf("typesafe: HTTP %d: %s", response.StatusCode, truncate(raw))
		}
		var envelope struct {
			Answers json.RawMessage `json:"answers"`
			Usage   struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.Answers) == 0 {
			return llm.Completion{}, fmt.Errorf("typesafe: response has no answers: %s", truncate(raw))
		}
		answers, _ := json.Marshal(map[string]json.RawMessage{"answers": envelope.Answers})
		return llm.Completion{
			Response: answers, FinishReason: llm.FinishStop, ChoiceCount: 1,
			Metrics: llm.Metrics{
				InputTokens: envelope.Usage.InputTokens, OutputTokens: envelope.Usage.OutputTokens, UsageReported: true,
				ProviderResponseBytes: len(raw), Latency: time.Since(started), Attempts: attempt,
			},
		}, nil
	}
	return llm.Completion{}, fmt.Errorf("typesafe: %d attempts failed: %w", maxAttempts, lastErr)
}

func (c *Client) retrying(attempt, status int, reason string, wait time.Duration) {
	if c.OnRetry != nil && attempt < maxAttempts {
		c.OnRetry(attempt, status, reason, wait)
	}
}

func backoff(attempt int, retryAfter string) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * time.Second
}

func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func truncate(raw []byte) string {
	if len(raw) > 300 {
		return string(raw[:300]) + "…"
	}
	return string(raw)
}
