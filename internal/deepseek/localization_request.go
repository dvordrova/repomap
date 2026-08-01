package deepseek

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/localization"
	"github.com/dvordrova/repomap/internal/secretscan"
)

const LocalizationRequestVersion = "localization-openai-request-v1"

// MaxLocalizationRequestBodyBytes is the shared upper bound for the exact
// provider-free localization request and its immutable projection record.
const MaxLocalizationRequestBodyBytes = 2 << 20

const localizationProvider = "openai-compatible"

const (
	maxLocalizationRequestScalarBytes = 4 << 10
)

// LocalizationRequestEvidence contains the exact provider-free request bytes
// and the non-secret provider identity needed to bind a future localization
// projection record. It never contains an API key or Authorization header.
type LocalizationRequestEvidence struct {
	Version         string   `json:"version"`
	Provider        string   `json:"provider"`
	Endpoint        string   `json:"endpoint"`
	AuthMode        string   `json:"auth_mode"`
	Model           string   `json:"model"`
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxTokens       int      `json:"max_tokens"`
	ResponseFormat  string   `json:"response_format"`
	Thinking        string   `json:"thinking,omitempty"`
	ReasoningEffort string   `json:"reasoning_effort,omitempty"`
	Body            []byte   `json:"body"`
}

// BuildLocalizationRequest returns the exact OpenAI-compatible JSON request
// body for a validated provider-neutral localization prompt. It performs no
// provider call and does not apply the canonical semantic-English contract.
func (c *Client) BuildLocalizationRequest(
	prompt localization.Prompt,
) (LocalizationRequestEvidence, error) {
	if c == nil {
		return LocalizationRequestEvidence{}, fmt.Errorf(
			"llm: localization request client is required",
		)
	}
	if _, err := localization.MarshalPrompt(prompt); err != nil {
		return LocalizationRequestEvidence{}, fmt.Errorf(
			"llm: invalid localization prompt",
		)
	}
	if localizationPromptContainsCredential(prompt) {
		return LocalizationRequestEvidence{}, fmt.Errorf(
			"llm: localization prompt contains an obvious credential",
		)
	}
	if !validLocalizationRequestScalar(c.Endpoint, false) {
		return LocalizationRequestEvidence{}, fmt.Errorf(
			"llm: invalid localization endpoint",
		)
	}
	endpoint, err := canonicalLocalizationEndpoint(c.Endpoint)
	if err != nil {
		return LocalizationRequestEvidence{}, fmt.Errorf(
			"llm: invalid localization endpoint: %w",
			err,
		)
	}
	if !validLocalizationRequestScalar(c.Auth, true) {
		return LocalizationRequestEvidence{}, fmt.Errorf(
			"llm: invalid localization auth mode",
		)
	}
	authMode := strings.ToLower(strings.TrimSpace(c.Auth))
	if authMode == "" {
		authMode = authBearer
	}
	if authMode != authBearer && authMode != authNone {
		return LocalizationRequestEvidence{}, fmt.Errorf(
			"llm: invalid localization auth mode",
		)
	}
	if !validLocalizationRequestScalar(c.Model, false) ||
		strings.TrimSpace(c.Model) == "" ||
		strings.TrimSpace(c.Model) != c.Model {
		return LocalizationRequestEvidence{}, fmt.Errorf(
			"llm: invalid localization model",
		)
	}
	if c.MaxTokens <= 0 {
		return LocalizationRequestEvidence{}, fmt.Errorf(
			"llm: localization max tokens must be positive",
		)
	}

	maxTokens := c.MaxTokens
	temperature := float64(0)
	request := chatRequest{
		Model: c.Model,
		Messages: []chatMessage{
			{Role: "system", Content: prompt.System},
			{Role: "user", Content: prompt.User},
		},
		Temperature:    &temperature,
		MaxTokens:      maxTokens,
		ResponseFormat: &jsonFormat{Type: "json_object"},
	}
	thinking := ""
	if isOfficialDeepSeekEndpoint(endpoint) {
		thinking = "disabled"
		request.Thinking = &thinkingConfig{Type: thinking}
	}
	body, err := json.Marshal(request)
	if err != nil {
		return LocalizationRequestEvidence{}, fmt.Errorf(
			"llm: encode localization request: %w",
			err,
		)
	}
	if len(body) == 0 || len(body) > MaxLocalizationRequestBodyBytes {
		return LocalizationRequestEvidence{}, &ResourceLimitError{
			Stage: "localization", Kind: ResourceLimitRequestBytes,
			Limit: MaxLocalizationRequestBodyBytes, Observed: len(body),
			ObservedKnown: true, ConfiguredMaxTokens: c.MaxTokens,
		}
	}
	if _, found := secretscan.DetectAlways(string(body)); found {
		return LocalizationRequestEvidence{}, fmt.Errorf(
			"llm: localization request contains an obvious credential",
		)
	}

	evidence := LocalizationRequestEvidence{
		Version: LocalizationRequestVersion, Provider: localizationProvider,
		Endpoint: endpoint, AuthMode: authMode, Model: c.Model,
		Temperature: &temperature, MaxTokens: maxTokens,
		ResponseFormat: "json_object", Thinking: thinking,
		Body: body,
	}
	if err := evidence.Validate(prompt); err != nil {
		return LocalizationRequestEvidence{}, err
	}
	return evidence, nil
}

// Validate proves that request evidence is complete, bounded, canonical in its
// non-secret identity, and semantically matches the exact localization prompt.
func (evidence LocalizationRequestEvidence) Validate(
	prompt localization.Prompt,
) error {
	if _, err := localization.MarshalPrompt(prompt); err != nil {
		return fmt.Errorf("llm: invalid localization prompt")
	}
	if len(evidence.Body) > MaxLocalizationRequestBodyBytes {
		return &ResourceLimitError{
			Stage: "localization", Kind: ResourceLimitRequestBytes,
			Limit: MaxLocalizationRequestBodyBytes, Observed: len(evidence.Body),
			ObservedKnown: true, ConfiguredMaxTokens: evidence.MaxTokens,
		}
	}
	if evidence.Version != LocalizationRequestVersion ||
		evidence.Provider != localizationProvider ||
		!validLocalizationRequestScalar(evidence.Endpoint, false) ||
		!validLocalizationRequestScalar(evidence.AuthMode, false) ||
		!validLocalizationRequestScalar(evidence.Model, false) ||
		evidence.Model != strings.TrimSpace(evidence.Model) ||
		evidence.Temperature == nil ||
		*evidence.Temperature != 0 ||
		evidence.MaxTokens <= 0 ||
		evidence.ResponseFormat != "json_object" ||
		evidence.ReasoningEffort != "" ||
		len(evidence.Body) == 0 {
		return fmt.Errorf("llm: invalid localization request evidence")
	}
	canonicalEndpoint, err := canonicalLocalizationEndpoint(evidence.Endpoint)
	if err != nil || canonicalEndpoint != evidence.Endpoint {
		return fmt.Errorf("llm: invalid localization request evidence")
	}
	if evidence.AuthMode != authBearer && evidence.AuthMode != authNone {
		return fmt.Errorf("llm: invalid localization request evidence")
	}
	wantThinking := ""
	if isOfficialDeepSeekEndpoint(evidence.Endpoint) {
		wantThinking = "disabled"
	}
	if evidence.Thinking != wantThinking {
		return fmt.Errorf("llm: invalid localization request evidence")
	}
	if localizationPromptContainsCredential(prompt) {
		return fmt.Errorf("llm: localization prompt contains an obvious credential")
	}
	if _, found := secretscan.DetectAlways(string(evidence.Body)); found {
		return fmt.Errorf("llm: localization request contains an obvious credential")
	}

	var request chatRequest
	decoder := json.NewDecoder(bytes.NewReader(evidence.Body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return fmt.Errorf("llm: invalid localization request evidence")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("llm: invalid localization request evidence")
	}
	if request.Model != evidence.Model ||
		len(request.Messages) != 2 ||
		request.Messages[0] != (chatMessage{Role: "system", Content: prompt.System}) ||
		request.Messages[1] != (chatMessage{Role: "user", Content: prompt.User}) ||
		request.Temperature == nil ||
		*request.Temperature != *evidence.Temperature ||
		request.MaxTokens != evidence.MaxTokens ||
		request.ResponseFormat == nil ||
		request.ResponseFormat.Type != evidence.ResponseFormat ||
		request.ReasoningEffort != evidence.ReasoningEffort {
		return fmt.Errorf("llm: invalid localization request evidence")
	}
	switch {
	case evidence.Thinking == "" && request.Thinking != nil:
		return fmt.Errorf("llm: invalid localization request evidence")
	case evidence.Thinking != "" &&
		(request.Thinking == nil || request.Thinking.Type != evidence.Thinking):
		return fmt.Errorf("llm: invalid localization request evidence")
	}
	return nil
}

func localizationPromptContainsCredential(prompt localization.Prompt) bool {
	if _, found := secretscan.DetectAlways(prompt.System); found {
		return true
	}
	_, found := secretscan.DetectAlways(prompt.User)
	return found
}

func canonicalLocalizationEndpoint(raw string) (string, error) {
	if raw == "" || strings.TrimSpace(raw) != raw {
		return "", fmt.Errorf("endpoint is empty or has surrounding whitespace")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("endpoint cannot be parsed")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("scheme must be http or https")
	}
	if parsed.Host == "" || parsed.Hostname() == "" {
		return "", fmt.Errorf("host is required")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("userinfo is not allowed")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("query and fragment are not allowed")
	}

	hostname := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if (scheme == "http" && port == "80") ||
		(scheme == "https" && port == "443") {
		port = ""
	}
	host := hostname
	if port != "" {
		host = net.JoinHostPort(hostname, port)
	} else if strings.Contains(hostname, ":") {
		host = "[" + hostname + "]"
	}
	return scheme + "://" + host + parsed.EscapedPath(), nil
}

func validLocalizationRequestScalar(value string, allowEmpty bool) bool {
	if len(value) > maxLocalizationRequestScalarBytes ||
		!utf8.ValidString(value) ||
		strings.ContainsAny(value, "\x00\r\n\t") {
		return false
	}
	return allowEmpty || value != ""
}
