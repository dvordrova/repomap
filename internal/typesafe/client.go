// Package typesafe is the llm.Categorizer for TypeSafe's System One models
// (Jev): typed closed decisions with probabilities instead of generated text.
// It writes the evaluation body for the owner's questions, adds the model,
// sends it, and reads the answers back as verdicts for the owner to decide.
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

// Client sends System One evaluations. Its zero value writes and reads
// them but cannot send; build it with NewFromEnv.
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

var _ llm.Categorizer = (*Client)(nil)

// NewFromEnv requires JEV_KEY: the closed decisions have no other model.
func NewFromEnv() (*Client, error) {
	key := strings.TrimSpace(os.Getenv(envAPIKey))
	if key == "" {
		return nil, fmt.Errorf("%s is required: keys, part roles and key declarations are decided by Jev; run with --no-model --target … to skip models", envAPIKey)
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

// Prompt is the evaluation body: the task and the shared context are the
// state, and each question is keyed by its owner's key and names its item
// as the question does ("row" unless it says otherwise). An option carries
// its structured criteria, else its meaning, else null, since the state
// already defines it.
func (c *Client) Prompt(task string, context map[string]any, questions map[string]llm.Question) (llm.Prompt, error) {
	asked := make(map[string]any, len(questions))
	for key, question := range questions {
		name := question.Name
		if name == "" {
			name = "row"
		}
		if name == "question" {
			return llm.Prompt{}, fmt.Errorf("typesafe: question %s names its item \"question\"", key)
		}
		instructions := map[string]any{name: question.Item, "question": question.Ask}
		if len(question.Options) == 0 {
			asked[key] = map[string]any{"type": "noul", "instructions": instructions}
			continue
		}
		criteria := make(map[string]any, len(question.Options))
		for _, option := range question.Options {
			switch {
			case option.Criteria != nil:
				criteria[option.Name] = option.Criteria
			case option.Meaning != "":
				criteria[option.Name] = option.Meaning
			default:
				criteria[option.Name] = nil
			}
		}
		asked[key] = map[string]any{"type": "choice", "instructions": instructions, "criteria": criteria}
	}
	body, err := json.Marshal(map[string]any{
		"state":     map[string]any{"task": task, "context": context},
		"questions": asked,
	})
	if err != nil {
		return llm.Prompt{}, err
	}
	return llm.Prompt{User: string(body), NoResponseAdjunct: true}, nil
}

// Verdicts reads the answers once, each on its own: a malformed one leaves
// only its question unanswered. A response without answers decided nothing.
func (c *Client) Verdicts(response []byte) (map[string]llm.Verdict, error) {
	var envelope struct {
		Answers map[string]json.RawMessage `json:"answers"`
	}
	if err := json.Unmarshal(response, &envelope); err != nil || envelope.Answers == nil {
		return nil, fmt.Errorf("response has no answers")
	}
	verdicts := make(map[string]llm.Verdict, len(envelope.Answers))
	for key, raw := range envelope.Answers {
		var answer struct {
			Type          string             `json:"type"`
			Choice        string             `json:"choice"`
			Probabilities map[string]float64 `json:"probabilities"`
			Noul          *float64           `json:"noul"`
		}
		if json.Unmarshal(raw, &answer) != nil {
			continue
		}
		switch answer.Type {
		case "choice":
			verdicts[key] = llm.Verdict{Choice: answer.Choice, Probabilities: answer.Probabilities}
		case "noul":
			verdicts[key] = llm.Verdict{Yes: answer.Noul}
		}
	}
	return verdicts, nil
}

// Prepare takes the evaluation body Prompt wrote and adds the model.
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
