package deepseek

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/secretscan"
)

const (
	defaultEndpoint             = "https://api.deepseek.com/chat/completions"
	defaultModel                = "deepseek-v4-flash"
	defaultMaxTokens            = 128_000
	defaultTimeout              = 10 * time.Minute
	defaultWaitProgressInterval = 10 * time.Second

	authBearer = "bearer"
	authNone   = "none"

	envEndpoint  = "REPOMAP_LLM_ENDPOINT"
	envModel     = "REPOMAP_LLM_MODEL"
	envAPIKey    = "REPOMAP_LLM_API_KEY"
	envMaxTokens = "REPOMAP_LLM_MAX_TOKENS"
	envTimeout   = "REPOMAP_LLM_TIMEOUT"
	envAuth      = "REPOMAP_LLM_AUTH"

	legacyEnvEndpoint = "DEEPSEEK_ENDPOINT"
	legacyEnvModel    = "DEEPSEEK_MODEL"
	legacyEnvAPIKey   = "DEEPSEEK_API_KEY"
	legacyEnvTimeout  = "DEEPSEEK_TIMEOUT"
	legacyEnvAuth     = "DEEPSEEK_AUTH"
)

// Client is safe for concurrent llm.Provider calls after configuration. Its
// exported fields and OnWait hook must not be mutated once execution starts.
type Client struct {
	HTTPClient *http.Client
	APIKey     string
	Model      string
	MaxTokens  int
	Endpoint   string
	Auth       string
	// OnWait is called from a heartbeat goroutine during long semantic stages.
	// Set it before starting a request; it must be concurrency-safe, return
	// promptly, and never log prompt, response, source, or credential content.
	OnWait       func(WaitProgress)
	waitInterval time.Duration
}

type WaitProgress struct {
	Stage   string
	Elapsed time.Duration
}

type EffectiveConfig struct {
	Endpoint  string
	Model     string
	AuthMode  string
	Timeout   time.Duration
	MaxTokens int
}

func (c *Client) EffectiveConfig() EffectiveConfig {
	auth := c.Auth
	if auth == "" {
		auth = authBearer
	}
	var timeout time.Duration
	if c.HTTPClient != nil {
		timeout = c.HTTPClient.Timeout
	}
	return EffectiveConfig{
		Endpoint:  c.Endpoint,
		Model:     c.Model,
		AuthMode:  auth,
		Timeout:   timeout,
		MaxTokens: c.MaxTokens,
	}
}

func NewFromEnv() (*Client, error) {
	useGenericConfig := anyEnvSet(
		envEndpoint,
		envModel,
		envAPIKey,
		envMaxTokens,
		envTimeout,
		envAuth,
	)
	value := func(primary, legacy string) string {
		if useGenericConfig {
			return strings.TrimSpace(os.Getenv(primary))
		}
		return strings.TrimSpace(os.Getenv(legacy))
	}

	auth := value(envAuth, legacyEnvAuth)
	if auth == "" {
		auth = authBearer
	}
	if auth != authBearer && auth != authNone {
		return nil, fmt.Errorf("%s must be %q or %q", envAuth, authBearer, authNone)
	}

	model := value(envModel, legacyEnvModel)
	if model == "" {
		model = defaultModel
	}

	maxTokens := defaultMaxTokens
	if s := strings.TrimSpace(os.Getenv(envMaxTokens)); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, fmt.Errorf("%s must be an integer: %w", envMaxTokens, err)
		}
		if n <= 0 {
			return nil, fmt.Errorf("%s must be positive", envMaxTokens)
		}
		maxTokens = n
	}

	timeout := defaultTimeout
	if s := value(envTimeout, legacyEnvTimeout); s != "" {
		parsed, err := time.ParseDuration(s)
		if err != nil {
			return nil, fmt.Errorf("%s must be a duration: %w", envTimeout, err)
		}
		if parsed <= 0 {
			return nil, fmt.Errorf("%s must be positive", envTimeout)
		}
		timeout = parsed
	}

	endpoint := value(envEndpoint, legacyEnvEndpoint)
	if endpoint == "" {
		if useGenericConfig {
			return nil, fmt.Errorf("%s is required when using REPOMAP_LLM_* configuration", envEndpoint)
		}
		if auth == authNone {
			return nil, fmt.Errorf("%s is required when unauthenticated mode is enabled", legacyEnvEndpoint)
		}
		endpoint = defaultEndpoint
	}
	if err := validateEndpoint(endpoint); err != nil {
		return nil, fmt.Errorf("%s is invalid: %w", envEndpoint, err)
	}

	key := value(envAPIKey, legacyEnvAPIKey)
	if auth == authBearer && key == "" {
		return nil, fmt.Errorf("%s is required when %s=%s", envAPIKey, envAuth, authBearer)
	}
	if auth == authNone {
		key = ""
	}

	return &Client{
		HTTPClient: &http.Client{Timeout: timeout},
		APIKey:     key,
		Model:      model,
		MaxTokens:  maxTokens,
		Endpoint:   endpoint,
		Auth:       auth,
	}, nil
}

func anyEnvSet(names ...string) bool {
	for _, name := range names {
		if _, ok := os.LookupEnv(name); ok {
			return true
		}
	}
	return false
}

func validateEndpoint(endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("scheme must be http or https")
	}
	if parsed.Host == "" || parsed.Hostname() == "" {
		return fmt.Errorf("host is required")
	}
	if parsed.User != nil {
		return fmt.Errorf("userinfo is not allowed")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("query and fragment are not allowed")
	}
	return nil
}

type jsonFormat struct {
	Type string `json:"type"`
}

type thinkingConfig struct {
	Type string `json:"type"`
}

type chatRequest struct {
	Model           string          `json:"model"`
	Messages        []chatMessage   `json:"messages"`
	Temperature     *float64        `json:"temperature,omitempty"`
	MaxTokens       int             `json:"max_tokens"`
	ResponseFormat  *jsonFormat     `json:"response_format,omitempty"`
	Thinking        *thinkingConfig `json:"thinking,omitempty"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"`
}

type chatMessage struct {
	Role             string `json:"role"`
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		FinishReason string      `json:"finish_reason"`
		Message      chatMessage `json:"message"`
	} `json:"choices"`
	Usage *chatUsage `json:"usage"`
}

type chatUsage struct {
	PromptTokens           int `json:"prompt_tokens"`
	CompletionTokens       int `json:"completion_tokens"`
	PromptCacheHitTokens   int `json:"prompt_cache_hit_tokens"`
	PromptCacheMissTokens  int `json:"prompt_cache_miss_tokens"`
	CompletionTokenDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

const maxRetries = 3

const maxProviderErrorBytes = 8 * 1024

const maxProviderResponseBytes = llm.ProviderResponseByteLimit

var (
	errResponseEnvelopeMalformed = errors.New("llm response envelope is malformed")
	// ErrResponseContentEmpty distinguishes an HTTP-successful, parsed
	// provider envelope with no usable assistant content from transport or
	// provider-call failures. Stage owners use it for closed response-decode
	// diagnostics without matching provider error text.
	ErrResponseContentEmpty = errors.New("llm response content is empty")
)

// IncompleteCompletionError reports an envelope that cannot represent one
// complete semantic answer. FinishReason is a closed diagnostic value; an
// unknown provider value is never echoed.
type IncompleteCompletionError struct {
	Stage        string
	ChoiceCount  int
	FinishReason string
}

type providerTransportError struct {
	failure llm.ProviderFailure
	cause   error
}

func newProviderTransportError(kind llm.ProviderFailureKind, status int, cause error) error {
	return &providerTransportError{
		failure: llm.ProviderFailure{Kind: kind, HTTPStatus: status},
		cause:   cause,
	}
}

func (err *providerTransportError) Error() string {
	if err == nil {
		return "llm provider transport failed"
	}
	if err.failure.Kind == llm.ProviderFailureHTTPStatus {
		return fmt.Sprintf("llm provider transport failed (class=http_status status=%d)", err.failure.HTTPStatus)
	}
	switch err.failure.Kind {
	case llm.ProviderFailureTimeout:
		return "llm provider transport failed (class=timeout)"
	case llm.ProviderFailureNetwork:
		return "llm provider transport failed (class=network)"
	case llm.ProviderFailureResponse:
		return "llm provider transport failed (class=response)"
	default:
		return "llm provider transport failed"
	}
}

func (err *providerTransportError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func (err *providerTransportError) ProviderFailure() llm.ProviderFailure {
	if err == nil {
		return llm.ProviderFailure{Kind: llm.ProviderFailureUnknown}
	}
	return err.failure
}

func (err *IncompleteCompletionError) Error() string {
	if err == nil {
		return "llm response completion is incomplete"
	}
	stage := strings.TrimSpace(err.Stage)
	if stage == "" {
		stage = "semantic"
	}
	reason := err.FinishReason
	if reason == "" {
		reason = "unavailable"
	}
	return fmt.Sprintf(
		"llm %s response completion is incomplete (choices=%d, finish_reason=%s)",
		stage,
		err.ChoiceCount,
		reason,
	)
}

// semanticRequest builds one provider request from cube-owned prompt text. It
// adds no semantic or output-language instructions.
func (c *Client) semanticRequest(
	userContent,
	systemContent string,
	jsonMode bool,
) chatRequest {
	request := chatRequest{
		Model: c.Model,
		Messages: []chatMessage{
			{Role: "system", Content: systemContent},
			{Role: "user", Content: userContent},
		},
		Temperature: float64Pointer(0.1),
		MaxTokens:   c.MaxTokens,
	}
	if jsonMode {
		request.ResponseFormat = &jsonFormat{Type: "json_object"}
	}
	return request
}

func float64Pointer(value float64) *float64 {
	return &value
}

func isOfficialDeepSeekEndpoint(endpoint string) bool {
	parsed, err := url.Parse(endpoint)
	return err == nil && strings.EqualFold(parsed.Hostname(), "api.deepseek.com")
}

func (c *Client) startWaitProgress(ctx context.Context, stage string) func() {
	if c == nil || c.OnWait == nil {
		return func() {}
	}
	interval := c.waitInterval
	if interval <= 0 {
		interval = defaultWaitProgressInterval
	}
	done := make(chan struct{})
	var once sync.Once
	var wait sync.WaitGroup
	wait.Add(1)
	started := time.Now()
	go func() {
		defer wait.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				c.OnWait(WaitProgress{Stage: stage, Elapsed: time.Since(started)})
			}
		}
	}()
	return func() {
		once.Do(func() { close(done) })
		wait.Wait()
	}
}

type chatCompletion struct {
	Content               []byte
	ResponseBytes         int
	UsageReported         bool
	InputTokens           int
	OutputTokens          int
	ReasoningTokens       int
	FinishReason          string
	ChoiceCount           int
	PromptCacheHitTokens  int
	PromptCacheMissTokens int
	finishReasonClass     string
}

func doChatMeasured(ctx context.Context, httpClient *http.Client, endpoint, apiKey, auth string, body []byte) (chatCompletion, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return chatCompletion{}, false, fmt.Errorf("build llm request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	switch auth {
	case "", authBearer:
		if apiKey == "" {
			return chatCompletion{}, false, fmt.Errorf("llm bearer authentication requires an API key")
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
	case authNone:
		// Explicit no-auth endpoints must not receive even an empty Authorization header.
	default:
		return chatCompletion{}, false, fmt.Errorf("unsupported llm authentication mode %q", auth)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		retry := isRetryableNetworkError(err)
		return chatCompletion{}, retry, newProviderTransportError(
			providerNetworkFailureKind(err), 0, fmt.Errorf("llm request failed: %w", err),
		)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxProviderResponseBytes+1))
	if err != nil {
		return chatCompletion{}, isRetryableNetworkError(err), newProviderTransportError(
			providerNetworkFailureKind(err), 0, fmt.Errorf("read llm response: %w", err),
		)
	}
	if len(respBody) > maxProviderResponseBytes {
		resourceErr := &ResourceLimitError{
			Kind:            ResourceLimitResponseBytes,
			Limit:           maxProviderResponseBytes,
			Observed:        len(respBody),
			ObservedKnown:   true,
			ObservedAtLeast: true,
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resourceErr.HTTPStatus = resp.StatusCode
		}
		return chatCompletion{ResponseBytes: len(respBody)}, false, resourceErr
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retry := isRetryableHTTP(resp.StatusCode)
		return chatCompletion{
			Content:       append([]byte(nil), respBody...),
			ResponseBytes: len(respBody),
		}, retry, newProviderTransportError(
			llm.ProviderFailureHTTPStatus,
			resp.StatusCode,
			fmt.Errorf("llm request failed with status %d: %s", resp.StatusCode, safeProviderErrorText(respBody)),
		)
	}

	var parsed chatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return chatCompletion{
			Content:       append([]byte(nil), respBody...),
			ResponseBytes: len(respBody),
		}, false, newProviderTransportError(
			llm.ProviderFailureResponse, 0, fmt.Errorf("%w: %v", errResponseEnvelopeMalformed, err),
		)
	}
	usage := chatUsage{}
	usageReported := parsed.Usage != nil
	if usageReported {
		usage = *parsed.Usage
	}
	completion := chatCompletion{
		ResponseBytes:         len(respBody),
		ChoiceCount:           len(parsed.Choices),
		UsageReported:         usageReported,
		InputTokens:           usage.PromptTokens,
		OutputTokens:          usage.CompletionTokens,
		ReasoningTokens:       usage.CompletionTokenDetails.ReasoningTokens,
		PromptCacheHitTokens:  usage.PromptCacheHitTokens,
		PromptCacheMissTokens: usage.PromptCacheMissTokens,
	}
	if completion.ChoiceCount != 1 {
		return completion, false, &IncompleteCompletionError{
			ChoiceCount: completion.ChoiceCount, FinishReason: "unavailable",
		}
	}
	choice := parsed.Choices[0]
	completion.FinishReason = knownFinishReason(choice.FinishReason)
	completion.finishReasonClass = closedFinishReason(choice.FinishReason)
	content := strings.TrimSpace(choice.Message.Content)
	completion.Content = []byte(content)
	if choice.FinishReason == "length" {
		return completion, false, newResourceLimitError(ResourceLimitError{
			Kind:            ResourceLimitOutputTokens,
			Observed:        usage.CompletionTokens,
			ObservedKnown:   usageReported,
			InputTokens:     usage.PromptTokens,
			OutputTokens:    usage.CompletionTokens,
			ReasoningTokens: usage.CompletionTokenDetails.ReasoningTokens,
			FinishReason:    "length",
		})
	}
	if content == "" {
		details := make([]string, 0, 4)
		if finishReason := knownFinishReason(choice.FinishReason); finishReason != "" {
			details = append(details, "finish_reason="+finishReason)
		}
		if usage.CompletionTokens > 0 {
			details = append(details, fmt.Sprintf("completion_tokens=%d", usage.CompletionTokens))
		}
		if usage.CompletionTokenDetails.ReasoningTokens > 0 {
			details = append(details, fmt.Sprintf(
				"reasoning_tokens=%d",
				usage.CompletionTokenDetails.ReasoningTokens,
			))
		} else if strings.TrimSpace(choice.Message.ReasoningContent) != "" {
			details = append(details, "reasoning_content_present")
		}
		if len(details) > 0 {
			return completion, false, fmt.Errorf("%w (%s)", ErrResponseContentEmpty, strings.Join(details, ", "))
		}
		return completion, false, ErrResponseContentEmpty
	}

	return completion, false, nil
}

func requireSingleStoppedCompletion(stage string, completion chatCompletion) error {
	if completion.ChoiceCount == 1 && completion.finishReasonClass == "stop" {
		return nil
	}
	return &IncompleteCompletionError{
		Stage: stage, ChoiceCount: completion.ChoiceCount,
		FinishReason: completion.finishReasonClass,
	}
}

func annotateIncompleteCompletion(err error, stage string) error {
	var incomplete *IncompleteCompletionError
	if !errors.As(err, &incomplete) {
		return err
	}
	return &IncompleteCompletionError{
		Stage: stage, ChoiceCount: incomplete.ChoiceCount,
		FinishReason: incomplete.FinishReason,
	}
}

func closedFinishReason(reason string) string {
	if known := knownFinishReason(reason); known != "" {
		return known
	}
	if strings.TrimSpace(reason) == "" {
		return "missing"
	}
	return "unknown"
}

func knownFinishReason(reason string) string {
	switch reason {
	case "stop", "length", "content_filter", "tool_calls", "insufficient_system_resource":
		return reason
	default:
		return ""
	}
}

func safeProviderErrorText(body []byte) string {
	text := strings.TrimSpace(string(body))
	if kind, found := secretscan.DetectPersistenceSensitive(text); found {
		return fmt.Sprintf("[redacted: %s detected in provider response]", kind)
	}
	if kind, found := secretscan.Detect(text); found {
		return fmt.Sprintf("[redacted: %s detected in provider response]", kind)
	}
	if len(text) <= maxProviderErrorBytes {
		return text
	}
	cut := maxProviderErrorBytes
	for cut > 0 && !utf8.ValidString(text[:cut]) {
		cut--
	}
	return text[:cut] + "...[truncated]"
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

func providerNetworkFailureKind(err error) llm.ProviderFailureKind {
	if errors.Is(err, context.DeadlineExceeded) {
		return llm.ProviderFailureTimeout
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) && networkErr.Timeout() {
		return llm.ProviderFailureTimeout
	}
	return llm.ProviderFailureNetwork
}

func backoffDuration(attempt int) time.Duration {
	base := time.Duration(1<<(attempt-1)) * 500 * time.Millisecond
	jitter := time.Duration(float64(base) * (0.5 + rand.Float64()*0.5))
	return jitter
}
