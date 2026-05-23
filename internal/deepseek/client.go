package deepseek

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultEndpoint = "https://api.deepseek.com/chat/completions"
	defaultModel    = "deepseek-chat"
)

type Client struct {
	HTTPClient *http.Client
	APIKey     string
	Model      string
	Endpoint   string
}

func NewFromEnv() (*Client, error) {
	key := strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	if key == "" {
		return nil, fmt.Errorf("DEEPSEEK_API_KEY is required unless --snapshot-only is used")
	}
	model := strings.TrimSpace(os.Getenv("DEEPSEEK_MODEL"))
	if model == "" {
		model = defaultModel
	}
	return &Client{
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
		APIKey:     key,
		Model:      model,
		Endpoint:   defaultEndpoint,
	}, nil
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
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

func (c *Client) Orient(ctx context.Context, snapshotJSON []byte) ([]byte, error) {
	reqPayload := chatRequest{
		Model: c.Model,
		Messages: []chatMessage{
			{
				Role:    "system",
				Content: "You are an expert software repository orientation assistant. Respond with JSON only, no markdown fences, no extra text.",
			},
			{
				Role: "user",
				Content: `Produce an orientation report JSON with this exact shape:
{
  "project_guess": "short guess what this repo is",
  "confidence": 0.0,
  "first_files_to_open": [{"path":"file path","reason":"why this file is useful"}],
  "detected_entrypoints": [{"name":"entrypoint name","evidence_files":["paths"],"why_it_matters":"..."}],
  "candidate_flows": [{"name":"flow name","trigger":"what starts the flow","likely_files":["paths"],"why_interesting":"...","confidence":0.0}],
  "important_domain_words": [{"word":"term","guess":"what it probably means in this repo"}],
  "questions_for_human": ["question that would help choose next analysis step"]
}

Use only evidence from this local repository snapshot. If uncertain, lower confidence.
Snapshot JSON:
` + string(snapshotJSON),
			},
		},
		Temperature: 0.1,
	}

	body, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal deepseek request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build deepseek request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deepseek request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read deepseek response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("deepseek request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed chatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("parse deepseek response envelope: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("deepseek response contains no choices")
	}
	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if content == "" {
		return nil, fmt.Errorf("deepseek response content is empty")
	}
	return []byte(content), nil
}
