package deepseek

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultEndpoint  = "https://api.deepseek.com/chat/completions"
	defaultModel     = "deepseek-v4-flash"
	defaultMaxTokens = 6000
)

type Client struct {
	HTTPClient *http.Client
	APIKey     string
	Model      string
	MaxTokens  int
	Endpoint   string
}

func NewFromEnv() (*Client, error) {
	return newFromEnv(true)
}

// NewPromptFromEnv builds request configuration without requiring an API key.
// It is intended for offline prompt inspection only.
func NewPromptFromEnv() (*Client, error) {
	return newFromEnv(false)
}

func newFromEnv(requireAPIKey bool) (*Client, error) {
	key := strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	if requireAPIKey && key == "" {
		return nil, fmt.Errorf("DEEPSEEK_API_KEY is required unless --snapshot-only is used")
	}
	if !requireAPIKey {
		key = ""
	}
	model := strings.TrimSpace(os.Getenv("DEEPSEEK_MODEL"))
	if model == "" {
		model = defaultModel
	}
	maxTokens := defaultMaxTokens
	if s := strings.TrimSpace(os.Getenv("DEEPSEEK_MAX_TOKENS")); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, fmt.Errorf("DEEPSEEK_MAX_TOKENS must be an integer: %w", err)
		}
		if n <= 0 {
			return nil, fmt.Errorf("DEEPSEEK_MAX_TOKENS must be positive")
		}
		maxTokens = n
	}
	endpoint := strings.TrimSpace(os.Getenv("DEEPSEEK_ENDPOINT"))
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	return &Client{
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
		APIKey:     key,
		Model:      model,
		MaxTokens:  maxTokens,
		Endpoint:   endpoint,
	}, nil
}

type jsonFormat struct {
	Type string `json:"type"`
}

type chatRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	Temperature    float64       `json:"temperature"`
	MaxTokens      int           `json:"max_tokens"`
	ResponseFormat *jsonFormat   `json:"response_format,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

const maxRetries = 3

func (c *Client) buildRequest(bundleJSON []byte) chatRequest {
	return chatRequest{
		Model: c.Model,
		Messages: []chatMessage{
			{
				Role:    "system",
				Content: "You are a senior Go engineer helping orient inside a large unfamiliar Go repository. Use only the provided facts. Do not pretend to have read files that were not provided. Return valid json only.",
			},
			{
				Role: "user",
				Content: `Do not explain the whole repo. Help the developer choose what runtime/event flow to inspect next.

You may only reference file paths listed in allowed_paths. If you think another file probably exists but it is not in allowed_paths, put it in unverified_paths instead of first_files_to_open or likely_files.

Produce a json orientation report with this exact shape:
{
  "project_guess": "short guess what this repo is",
  "confidence": 0.0,
  "high_level_map": [
    {
      "name": "component or subsystem name",
      "evidence": ["facts or paths from the bundle"],
      "why_it_matters": "why this component matters for understanding the repo"
    }
  ],
  "first_files_to_open": [
    {
      "path": "must be from allowed_paths",
      "reason": "why this file is worth opening first"
    }
  ],
  "candidate_flows": [
    {
      "name": "runtime or event flow name",
      "trigger": "what starts this flow",
      "likely_entrypoint": "package or repo-relative file",
      "likely_files": ["all must be from allowed_paths"],
      "why_interesting": "why this flow matters",
      "evidence": ["facts from the bundle supporting this flow"],
      "confidence": 0.0
    }
  ],
  "important_domain_words": [
    {
      "word": "term found in paths or readme",
      "guess": "what it probably means in this repo",
      "evidence": ["paths or readme excerpts from the bundle"]
    }
  ],
  "questions_for_human": [
    "question that helps guide the next analysis step"
  ],
  "unverified_paths": [
    {
      "path": "path model suspects but was not present in allowed_paths",
      "reason": "why it might be relevant"
    }
  ],
  "warnings": [
    "any uncertainty or missing context"
  ]
}

Important rules:
- Candidate flows must be runtime/event-oriented (e.g. "gRPC Put request", "server startup", "watch stream", "raft write path", "lease lifecycle"), not folder-oriented (do not say "server module" or "pkg folder").
- Every candidate flow must include evidence from the bundle.
- Distinguish facts from guesses. If confidence is low, say so in warnings.
- Use only the provided facts bundle. Do not imagine files you cannot see.

Facts bundle JSON:
` + string(bundleJSON),
			},
		},
		Temperature:    0.1,
		MaxTokens:      c.MaxTokens,
		ResponseFormat: &jsonFormat{Type: "json_object"},
	}
}

func (c *Client) OrientPromptJSON(bundleJSON []byte) ([]byte, error) {
	reqPayload := c.buildRequest(bundleJSON)
	return json.MarshalIndent(reqPayload, "", "  ")
}

func (c *Client) FlowExplainPromptJSON(userContent, systemContent string) ([]byte, error) {
	reqPayload := c.flowExplainRequest(userContent, systemContent, true)
	return json.MarshalIndent(reqPayload, "", "  ")
}

func (c *Client) flowExplainPromptText(userContent, systemContent string) ([]byte, error) {
	reqPayload := c.flowExplainRequest(userContent, systemContent, false)
	return json.MarshalIndent(reqPayload, "", "  ")
}

func (c *Client) flowExplainRequest(userContent, systemContent string, jsonMode bool) chatRequest {
	request := chatRequest{
		Model: c.Model,
		Messages: []chatMessage{
			{Role: "system", Content: systemContent},
			{Role: "user", Content: userContent},
		},
		Temperature: 0.1,
		MaxTokens:   c.MaxTokens,
	}
	if jsonMode {
		request.ResponseFormat = &jsonFormat{Type: "json_object"}
	}
	return request
}

func (c *Client) FlowExplain(ctx context.Context, userContent, systemContent string) ([]byte, error) {
	return c.flowExplain(ctx, userContent, systemContent, true, true)
}

func (c *Client) flowExplain(ctx context.Context, userContent, systemContent string, jsonMode, validateJSON bool) ([]byte, error) {
	reqPayload := c.flowExplainRequest(userContent, systemContent, jsonMode)

	body, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal flow explain request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := backoffDuration(attempt)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		result, shouldRetry, err := doChat(ctx, c.HTTPClient, c.Endpoint, c.APIKey, body, validateJSON)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !shouldRetry {
			return nil, err
		}
	}

	return nil, fmt.Errorf("retries exhausted (%d attempts): %w", maxRetries+1, lastErr)
}

func (c *Client) Orient(ctx context.Context, bundleJSON []byte) ([]byte, error) {
	reqPayload := c.buildRequest(bundleJSON)

	body, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal deepseek request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := backoffDuration(attempt)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		result, shouldRetry, err := doOrient(ctx, c.HTTPClient, c.Endpoint, c.APIKey, body)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !shouldRetry {
			return nil, err
		}
	}

	return nil, fmt.Errorf("retries exhausted (%d attempts): %w", maxRetries+1, lastErr)
}

func doOrient(ctx context.Context, httpClient *http.Client, endpoint, apiKey string, body []byte) ([]byte, bool, error) {
	return doChat(ctx, httpClient, endpoint, apiKey, body, true)
}

func doChat(ctx context.Context, httpClient *http.Client, endpoint, apiKey string, body []byte, validateJSON bool) ([]byte, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("build deepseek request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		retry := isRetryableNetworkError(err)
		return nil, retry, fmt.Errorf("deepseek request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, fmt.Errorf("read deepseek response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retry := isRetryableHTTP(resp.StatusCode)
		return nil, retry, fmt.Errorf("deepseek request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed chatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, false, fmt.Errorf("parse deepseek response envelope: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return nil, false, fmt.Errorf("deepseek response contains no choices")
	}
	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if content == "" {
		return nil, false, fmt.Errorf("deepseek response content is empty")
	}

	if validateJSON {
		var validate json.RawMessage
		if err := json.Unmarshal([]byte(content), &validate); err != nil {
			return nil, false, fmt.Errorf("deepseek response content is not valid JSON:\n%s", content)
		}
	}

	return []byte(content), false, nil
}

func isRetryableHTTP(status int) bool {
	return status == 429 || status >= 500
}

func isRetryableNetworkError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return true
}

func backoffDuration(attempt int) time.Duration {
	base := time.Duration(1<<(attempt-1)) * 500 * time.Millisecond
	jitter := time.Duration(float64(base) * (0.5 + rand.Float64()*0.5))
	return jitter
}
