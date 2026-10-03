// Package typesafe is the llm.Categorizer for TypeSafe's System One models
// (Jev): typed closed decisions with probabilities instead of generated text.
// It writes the evaluation body for the owner's questions, adds the model,
// sends it, and reads the answers back as verdicts for the owner to decide.
package typesafe

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
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
	// providerName names Jev in a failed request's HTTP diagnostics, so a
	// refused key or balance is named as Jev's, not the text model's.
	providerName = "Jev"

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
	// Controller is the run's one attempt gate and set of requests in the
	// air for Jev, shared by every stage that asks it: Jev's rate limit is
	// its account's, so a 429 on one stage's request cools down all of them
	// (llm.AttemptControllerOwner). Nil leaves each batch its executor's.
	Controller *llm.BatchController
	// OnRetry reports each transport retry: the failed attempt with its
	// closed failure, status and the wait before the next one, then the
	// retry's start once the shared gate lets it go. It never sees the
	// request, the response or the key.
	OnRetry func(llm.RetryProgress)
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

var (
	_ llm.Categorizer            = (*Client)(nil)
	_ llm.AttemptControllerOwner = (*Client)(nil)
)

// AttemptController is the run's shared Jev gate, or nil.
func (c *Client) AttemptController() *llm.BatchController { return c.Controller }

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

// Complete sends exactly the prepared bytes. Each attempt holds a lease of
// the shared attempt gate. A network failure, an unreadable body, HTTP 429
// and HTTP 5xx are transport failures and are sent again, at most
// maxAttempts in all, with the same bytes: after a 429 every new attempt
// through the gate waits the shared cooldown (llm.RateLimitCooldown), after
// another failure the short backoff. Nothing else is sent again: a refusal
// of the input's size, another HTTP status and an answer without answers
// end the call. A failed call returns the last attempt's HTTP diagnostics
// and body with a typed failure (llm.ProviderFailureSource), so the executor
// names its class and status and the run can stop on a refused key or
// balance. Cancellation interrupts every wait.
func (c *Client) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	started := time.Now()
	body := prepared.Bytes()
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	var (
		last     attempt
		received int
		wait     time.Duration
	)
	failed := func(attempts int, err error) (llm.Completion, error) {
		response := last.body
		// A provider that echoes the key in its error must not put it in
		// the run's journal: the response is then unavailable.
		if c.APIKey != "" && bytes.Contains(response, []byte(c.APIKey)) {
			response = nil
		}
		return llm.Completion{
			Response: response, HTTPResponse: last.http.Clone(),
			Metrics: llm.Metrics{ProviderResponseBytes: received, Latency: time.Since(started), Attempts: attempts},
		}, err
	}
	for n := 1; n <= maxAttempts; n++ {
		if n > 1 && !sleep(ctx, wait) {
			return failed(n-1, ctx.Err())
		}
		release, err := llm.AcquireProviderAttempt(ctx)
		if err != nil {
			return failed(n-1, err)
		}
		if n > 1 {
			c.retrying(llm.RetryProgress{RequestSHA256: digest, Attempt: n, MaxAttempts: maxAttempts, Starting: true, Elapsed: time.Since(started)})
		}
		last = c.send(ctx, body)
		received += len(last.body)
		if last.status == http.StatusTooManyRequests {
			wait = llm.RateLimitCooldown(last.retryAfter, last.body, time.Now())
			llm.BackoffProviderAttempts(ctx, wait)
		} else {
			wait = backoff(n)
		}
		release()
		if last.err == nil {
			return llm.Completion{
				Response: last.answers, FinishReason: llm.FinishStop, ChoiceCount: 1,
				Metrics: llm.Metrics{
					InputTokens: last.inputTokens, OutputTokens: last.outputTokens, UsageReported: true,
					ProviderResponseBytes: received, Latency: time.Since(started), Attempts: n,
				},
			}, nil
		}
		if err := ctx.Err(); err != nil {
			return failed(n, err)
		}
		if !last.retryable {
			return failed(n, last.err)
		}
		if n < maxAttempts {
			failure := llm.ProviderFailure{Kind: llm.ProviderFailureUnknown}
			var source llm.ProviderFailureSource
			if errors.As(last.err, &source) {
				failure = source.ProviderFailure()
			}
			c.retrying(llm.RetryProgress{RequestSHA256: digest, Attempt: n, MaxAttempts: maxAttempts,
				Failure: failure.Kind, HTTPStatus: failure.HTTPStatus, Delay: wait, Elapsed: time.Since(started)})
		}
	}
	var exhausted *failure
	if errors.As(last.err, &exhausted) {
		copy := *exhausted
		copy.attempts, copy.exhausted = maxAttempts, true
		return failed(maxAttempts, &copy)
	}
	return failed(maxAttempts, last.err)
}

// attempt is one HTTP exchange: its diagnostics and body, and either Jev's
// answers or the failure and whether it is a transport failure.
type attempt struct {
	http       *llm.HTTPResponse
	body       []byte
	status     int
	retryAfter string
	err        error
	retryable  bool

	answers                   []byte
	inputTokens, outputTokens int
}

func (c *Client) send(ctx context.Context, body []byte) attempt {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return attempt{err: fmt.Errorf("typesafe: build request: %w", err)}
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+c.APIKey)
	response, err := c.HTTPClient.Do(request)
	if err != nil {
		return attempt{err: transportFailure(err), retryable: true}
	}
	defer response.Body.Close()
	result := attempt{http: llm.DiagnosticHTTPResponse(response), status: response.StatusCode}
	result.http.Provider = providerName
	raw, err := io.ReadAll(io.LimitReader(response.Body, llm.ProviderResponseByteLimit+1))
	if err != nil {
		result.err, result.retryable = transportFailure(err), true
		return result
	}
	if len(raw) > llm.ProviderResponseByteLimit {
		limit := &llm.ResourceLimitError{Kind: llm.ResourceLimitResponseBytes, Limit: llm.ProviderResponseByteLimit,
			Observed: len(raw), ObservedKnown: true, ObservedAtLeast: true}
		if response.StatusCode != http.StatusOK {
			limit.HTTPStatus = response.StatusCode
		}
		result.err = limit
		return result
	}
	result.body = raw
	switch {
	case response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500:
		result.retryAfter = response.Header.Get("Retry-After")
		result.err, result.retryable = &failure{kind: llm.ProviderFailureHTTPStatus, status: response.StatusCode}, true
		return result
	case response.StatusCode == http.StatusBadRequest && bytes.Contains(raw, []byte("max_tokens_exceeded")):
		result.err = llm.NewResourceLimitError(llm.ResourceLimitError{
			Kind: llm.ResourceLimitContextTokens, HTTPStatus: response.StatusCode,
		})
		return result
	case response.StatusCode != http.StatusOK:
		result.err = &failure{kind: llm.ProviderFailureHTTPStatus, status: response.StatusCode}
		return result
	}
	var envelope struct {
		Answers json.RawMessage `json:"answers"`
		Usage   struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.Answers) == 0 {
		result.err = &failure{kind: llm.ProviderFailureResponse}
		return result
	}
	result.answers, _ = json.Marshal(map[string]json.RawMessage{"answers": envelope.Answers})
	result.inputTokens, result.outputTokens = envelope.Usage.InputTokens, envelope.Usage.OutputTokens
	return result
}

// failure is a closed transport fact about a failed call: its class, HTTP
// status and attempts. Its text never carries the response body.
type failure struct {
	kind      llm.ProviderFailureKind
	status    int
	attempts  int
	exhausted bool
	cause     error
}

func transportFailure(err error) *failure {
	var timeout net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &timeout) && timeout.Timeout() {
		return &failure{kind: llm.ProviderFailureTimeout, cause: err}
	}
	return &failure{kind: llm.ProviderFailureNetwork, cause: err}
}

func (err *failure) Error() string {
	message := "typesafe: "
	switch err.kind {
	case llm.ProviderFailureHTTPStatus:
		message += fmt.Sprintf("HTTP %d", err.status)
	case llm.ProviderFailureTimeout:
		message += "timeout"
	case llm.ProviderFailureNetwork:
		message += "network failure"
	case llm.ProviderFailureResponse:
		message += "response has no answers"
	default:
		message += "request failed"
	}
	if err.exhausted {
		message += fmt.Sprintf(" after %d attempts", err.attempts)
	}
	return message
}

func (err *failure) Unwrap() error { return err.cause }

func (err *failure) ProviderFailure() llm.ProviderFailure {
	return llm.ProviderFailure{Kind: err.kind, HTTPStatus: err.status, Attempts: err.attempts, RetryExhausted: err.exhausted}
}

func (c *Client) retrying(progress llm.RetryProgress) {
	if c.OnRetry != nil {
		c.OnRetry(progress)
	}
}

// backoff is the short wait after a transport failure other than a 429.
func backoff(attempt int) time.Duration {
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
