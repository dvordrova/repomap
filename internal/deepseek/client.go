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
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/secretscan"
)

const (
	defaultEndpoint  = "https://api.deepseek.com/chat/completions"
	defaultModel     = "deepseek-v4-flash"
	defaultMaxTokens = 64_000
	defaultTimeout   = 10 * time.Minute

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

// OrientationPromptVersionJSON identifies the semantic orientation prompt and
// request contract used by Orient and OrientPromptJSON.
const OrientationPromptVersionJSON = "orientation-json-v13"

// SemanticOutputLanguageContractVersion identifies the default shared
// language contract. A stage whose versioned prompt owns a closed output
// language bypasses this wrapper; localization likewise owns its separate
// source/target-locale contract.
const SemanticOutputLanguageContractVersion = "canonical-english-output-v1"

const canonicalEnglishSystemContract = `CANONICAL OUTPUT LANGUAGE CONTRACT (canonical-english-output-v1):
- Write every human-readable prose value in English.
- Keep JSON keys, enum values, exact schema literals, opaque IDs, repository paths, code identifiers, package/module names, API/protocol/product/library names, exact format tags, and quoted source text unchanged.
- Copy values from closed lists of allowed literals exactly, even when their words look human-readable.
- Return exactly the requested response shape.`

const canonicalEnglishUserContract = `OUTPUT LANGUAGE:
The response is the canonical semantic result. Before returning it, verify that every human-readable title, description, explanation, question, reason, warning, summary, and other prose value is English while protected technical values remain unchanged.`

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
	return newFromEnv(true)
}

// NewPromptFromEnv builds request configuration without requiring an API key.
// It is intended for offline prompt inspection only.
func NewPromptFromEnv() (*Client, error) {
	return newFromEnv(false)
}

func newFromEnv(requireAPIKey bool) (*Client, error) {
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
	if requireAPIKey && auth == authBearer && key == "" {
		return nil, fmt.Errorf("%s is required when %s=%s", envAPIKey, envAuth, authBearer)
	}
	if !requireAPIKey || auth == authNone {
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

const maxProviderResponseBytes = modelresearch.ProviderResponseByteLimit

var (
	errJSONCompletionInvalid     = errors.New("llm response content is not valid JSON")
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

func (c *Client) buildRequest(bundleJSON []byte) chatRequest {
	request := c.canonicalSemanticRequest(
		`Do not explain the whole repo. Help the developer choose what runtime/event flow to inspect next.

The request-local reference fields embedded in the facts bundle are closed. Return only exact file refs (f0001...) in file-ref fields and exact evidence refs (e0001...) in evidence-ref fields. Never return a repository path, candidate_file_index id, package path, import path, path:line statement, or reference handle in a prose field. Never shorten, extend, prefix, substitute, or repair a ref. Never use a file ref where an evidence ref is required or an evidence ref where a file ref is required. Do not duplicate a ref within one field. Omit a value or flow that cannot use exact refs from this request. There is no unverified_paths response field and no filename inference or fallback from an entrypoint to the first likely file.

Produce a json orientation report with this exact shape:
{
  "project_guess": "short guess what this repo is",
  "confidence": 0.0,
  "high_level_map": [
    {
      "name": "component or subsystem name",
      "role": "entry | boundary | coordination | domain | state | support | unknown",
      "evidence_refs": ["exact e-ref embedded in this request"],
      "why_it_matters": "why this component matters for understanding the repo"
    }
  ],
  "first_files_to_open": [
    {
      "file_ref": "exact f-ref embedded in this request",
      "reason": "why this file is worth opening first"
    }
  ],
  "candidate_flows": [
    {
      "name": "runtime or event flow name",
      "flow_type": "request | operational",
      "trigger": "what starts this flow",
      "likely_entrypoint_ref": "exact f-ref embedded in this request, preferably one of likely_file_refs",
      "likely_file_refs": ["exact f-refs embedded in this request"],
      "why_interesting": "why this flow matters",
      "evidence_refs": ["exact e-refs embedded in this request that support this flow"],
      "confidence": 0.0
    }
  ],
  "important_domain_words": [
    {
      "word": "term found in paths or readme",
      "guess": "what it probably means in this repo",
      "evidence_refs": ["exact e-refs embedded in this request"]
    }
  ],
  "questions_for_human": [
    "question that helps guide the next analysis step"
  ],
  "research_questions": [
    {
      "id": "short question id",
      "purpose": "why resolving this question would improve architecture or a saved trace",
      "question": "one concrete high-value repository question",
      "candidate_file_refs": ["exact f-refs from candidate_file_index in this request"],
      "evidence_categories": ["declaration, callsite, transition, source_window, test, or frontier"]
    }
  ],
  "warnings": [
    "any uncertainty or missing context"
  ]
}

Important rules:
- Candidate flows must be runtime/event-oriented (e.g. "CLI command dispatch", "HTTP request handling", "server startup", "plugin loading", "background job execution"), not folder-oriented (do not say "server module" or "pkg folder").
- Set flow_type to "request" for user/request-driven work and "operational" for background, maintenance, threshold, consensus, or durability work. Prefer the strongest grounded evidence regardless of flow type.
- An operational candidate must cite an evidence_ref attached to a source_signals record. If that evidence is weak or only suggests a possible flow, cap confidence at 0.3 and state the uncertainty.
- Give every high_level_map item one coarse navigation role. Use entry for process or command entrypoints, boundary for external protocols and adapters, coordination for lifecycle and orchestration, domain for core behavior, state for persistence or state ownership, support for configuration/operations/observability/testing, and unknown when the bundle does not support a useful choice. A role is an orientation hypothesis, not static or runtime proof.
- Every candidate flow must include evidence_refs from the facts bundle.
- Propose two to four research_questions only when bounded local evidence could answer them. Select candidates only through exact candidate_file_refs and supplied evidence categories; do not invent or request paths. Treat omitted files as unknown rather than absent. Questions should target user-facing behavior, architecture gaps, or trace frontiers, not prettier names.
- go.command_traces are locally extracted bounded syntax evidence. Preserve their typed relations: calls, registers_command, callback, constructs, registers, and starts_goroutine are not interchangeable. A handler_call with resolved=false is an exact call site but not a resolved concrete target. Prefer a complete command_trace over a filename-only CLI guess.
- Evidence is selected only by exact evidence_refs embedded in this request; do not rewrite referenced evidence as prose.
- Distinguish facts from guesses. If confidence is low, say so in warnings.
- Use only the provided facts bundle and its request-local reference fields. Do not imagine files you cannot see.

Orientation facts bundle JSON:
`+string(bundleJSON),
		"You are a senior software engineer helping orient inside a large unfamiliar repository. Infer the language from language_hints and use only the provided facts. Do not pretend to have read files that were not provided. Return valid json only.",
		true,
	)
	if isOfficialDeepSeekEndpoint(c.Endpoint) {
		// Orientation is a bounded classification over an already compact local
		// facts bundle. DeepSeek V4 otherwise enables thinking by default and can
		// consume the entire JSON completion envelope without returning content.
		request.Thinking = &thinkingConfig{Type: "disabled"}
	}
	return request
}

func (c *Client) OrientPromptJSON(bundleJSON []byte) ([]byte, error) {
	reqPayload := c.buildRequest(bundleJSON)
	return json.Marshal(reqPayload)
}

func (c *Client) FlowExplainPromptJSON(userContent, systemContent string) ([]byte, error) {
	reqPayload := c.flowExplainRequest(userContent, systemContent, true)
	return json.Marshal(reqPayload)
}

func (c *Client) flowExplainPromptText(userContent, systemContent string) ([]byte, error) {
	reqPayload := c.flowExplainRequest(userContent, systemContent, false)
	return json.Marshal(reqPayload)
}

func (c *Client) flowExplainRequest(userContent, systemContent string, jsonMode bool) chatRequest {
	return c.canonicalSemanticRequest(userContent, systemContent, jsonMode)
}

func (c *Client) canonicalSemanticRequest(
	userContent,
	systemContent string,
	jsonMode bool,
) chatRequest {
	return c.semanticRequest(
		withCanonicalEnglishUserContract(userContent),
		withCanonicalEnglishSystemContract(systemContent),
		jsonMode,
	)
}

// semanticRequest builds one provider request without imposing a shared
// output-language contract. It is reserved for stages whose versioned prompt
// owns an explicit closed output language itself.
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

func withCanonicalEnglishSystemContract(systemContent string) string {
	return systemContent + "\n\n" + canonicalEnglishSystemContract
}

func withCanonicalEnglishUserContract(userContent string) string {
	return canonicalEnglishUserContract + "\n\n" + userContent
}

func float64Pointer(value float64) *float64 {
	return &value
}

func (c *Client) FlowExplain(ctx context.Context, userContent, systemContent string) ([]byte, error) {
	return c.flowExplain(ctx, userContent, systemContent, true, true)
}

// BuildResearchRequest returns the exact OpenAI-compatible request body for
// one bounded targeted research prompt.
func (c *Client) BuildResearchRequest(prompt modelresearch.Prompt) ([]byte, error) {
	if prompt.Version != modelresearch.PromptVersion {
		return nil, fmt.Errorf("unsupported model research prompt version %q", prompt.Version)
	}
	return c.FlowExplainPromptJSON(prompt.User, prompt.System)
}

// Research performs one semantic targeted stage. Transport retries remain
// inside the stage and are returned as Attempts rather than extra rounds.
func (c *Client) Research(ctx context.Context, prompt modelresearch.Prompt) (modelresearch.ProviderResult, error) {
	stopWaiting := c.startWaitProgress(ctx, "targeted research")
	defer stopWaiting()
	body, err := c.BuildResearchRequest(prompt)
	if err != nil {
		return modelresearch.ProviderResult{}, err
	}
	completion, attempts, callErr := executeChatWithTransportRetries(
		ctx,
		c.HTTPClient,
		c.Endpoint,
		c.APIKey,
		c.Auth,
		body,
		true,
	)
	if callErr != nil {
		return providerResultFromCompletion(completion, attempts, len(body)*attempts),
			annotateResourceLimit(callErr, "targeted_research", c.MaxTokens)
	}
	return providerResultFromCompletion(completion, attempts, len(body)*attempts), nil
}

// CheckJSONCompatibility makes exactly one small synthetic request. It is used
// by the CLI doctor and deliberately does not inherit normal retry behavior.
func (c *Client) CheckJSONCompatibility(ctx context.Context) error {
	reqPayload := c.flowExplainRequest(
		`Return exactly one JSON object: {"status":"ok"}`,
		"This is a provider compatibility check. Return valid JSON only.",
		true,
	)
	body, err := json.Marshal(reqPayload)
	if err != nil {
		return fmt.Errorf("marshal llm compatibility request: %w", err)
	}
	raw, _, err := doChat(ctx, c.HTTPClient, c.Endpoint, c.APIKey, c.Auth, body, true)
	if err != nil {
		return err
	}
	var response struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &response); err != nil || response.Status != "ok" {
		return fmt.Errorf("llm provider returned JSON but not the expected status")
	}
	return nil
}

func (c *Client) flowExplain(ctx context.Context, userContent, systemContent string, jsonMode, validateJSON bool) ([]byte, error) {
	var (
		body []byte
		err  error
	)
	if jsonMode {
		body, err = c.FlowExplainPromptJSON(userContent, systemContent)
	} else {
		body, err = c.flowExplainPromptText(userContent, systemContent)
	}
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

		result, shouldRetry, err := doChat(ctx, c.HTTPClient, c.Endpoint, c.APIKey, c.Auth, body, validateJSON)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !shouldRetry {
			return nil, annotateResourceLimit(err, "flow_explain", c.MaxTokens)
		}
	}

	return nil, fmt.Errorf("retries exhausted (%d attempts): %w", maxRetries+1, lastErr)
}

func (c *Client) Orient(ctx context.Context, bundleJSON []byte) ([]byte, error) {
	result, err := c.OrientMeasured(ctx, bundleJSON)
	return result.Content, err
}

// OrientMeasured performs one semantic orientation stage and reports bounded
// transport attempts separately from semantic call accounting.
func (c *Client) OrientMeasured(ctx context.Context, bundleJSON []byte) (modelresearch.ProviderResult, error) {
	stopWaiting := c.startWaitProgress(ctx, "orientation")
	defer stopWaiting()
	request := c.buildRequest(bundleJSON)
	body, err := json.Marshal(request)
	if err != nil {
		return modelresearch.ProviderResult{}, fmt.Errorf("marshal llm request: %w", err)
	}
	var (
		measured         modelresearch.ProviderResult
		lastErr          error
		transportRetries int
	)
	for {
		if measured.Attempts > 0 {
			backoff := backoffDuration(measured.Attempts)
			select {
			case <-ctx.Done():
				return measured, ctx.Err()
			case <-time.After(backoff):
			}
		}

		result, shouldRetry, err := doChatMeasured(
			ctx,
			c.HTTPClient,
			c.Endpoint,
			c.APIKey,
			c.Auth,
			body,
			true,
		)
		measured.Attempts++
		measured.RequestBytes += len(body)
		measured.ResponseBytes += result.ResponseBytes
		measured.UsageReported = measured.UsageReported || result.UsageReported
		measured.InputTokens += result.InputTokens
		measured.OutputTokens += result.OutputTokens
		measured.ReasoningTokens += result.ReasoningTokens
		measured.PromptCacheHitTokens += result.PromptCacheHitTokens
		measured.PromptCacheMissTokens += result.PromptCacheMissTokens
		measured.Content = append([]byte(nil), result.Content...)
		measured.FinishReason = result.FinishReason
		measured.ChoiceCount = result.ChoiceCount
		if err == nil {
			return measured, nil
		}
		lastErr = err
		if shouldRetry && transportRetries < maxRetries {
			transportRetries++
			continue
		}
		if shouldRetry {
			return measured, fmt.Errorf(
				"retries exhausted (%d attempts): %w",
				measured.Attempts,
				lastErr,
			)
		}
		return measured, annotateResourceLimit(err, "orientation", request.MaxTokens)
	}
}

func providerResultFromCompletion(
	completion chatCompletion,
	attempts int,
	requestBytes int,
) modelresearch.ProviderResult {
	return modelresearch.ProviderResult{
		Content:  append([]byte(nil), completion.Content...),
		Attempts: attempts, RequestBytes: requestBytes,
		ResponseBytes: completion.ResponseBytes,
		UsageReported: completion.UsageReported,
		InputTokens:   completion.InputTokens, OutputTokens: completion.OutputTokens,
		ReasoningTokens:       completion.ReasoningTokens,
		FinishReason:          completion.FinishReason,
		ChoiceCount:           completion.ChoiceCount,
		PromptCacheHitTokens:  completion.PromptCacheHitTokens,
		PromptCacheMissTokens: completion.PromptCacheMissTokens,
	}
}

func (c *Client) startWaitProgress(ctx context.Context, stage string) func() {
	if c == nil || c.OnWait == nil {
		return func() {}
	}
	interval := c.waitInterval
	if interval <= 0 {
		interval = 10 * time.Second
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

func doOrient(ctx context.Context, httpClient *http.Client, endpoint, apiKey, auth string, body []byte) ([]byte, bool, error) {
	return doChat(ctx, httpClient, endpoint, apiKey, auth, body, true)
}

func doChat(ctx context.Context, httpClient *http.Client, endpoint, apiKey, auth string, body []byte, validateJSON bool) ([]byte, bool, error) {
	result, retry, err := doChatMeasured(ctx, httpClient, endpoint, apiKey, auth, body, validateJSON)
	return result.Content, retry, err
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

func doChatMeasured(ctx context.Context, httpClient *http.Client, endpoint, apiKey, auth string, body []byte, validateJSON bool) (chatCompletion, bool, error) {
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
		return chatCompletion{}, retry, fmt.Errorf("llm request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxProviderResponseBytes+1))
	if err != nil {
		return chatCompletion{}, isRetryableNetworkError(err), fmt.Errorf("read llm response: %w", err)
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
		}, retry, fmt.Errorf("llm request failed with status %d: %s", resp.StatusCode, safeProviderErrorText(respBody))
	}

	var parsed chatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return chatCompletion{
			Content:       append([]byte(nil), respBody...),
			ResponseBytes: len(respBody),
		}, false, fmt.Errorf("%w: %v", errResponseEnvelopeMalformed, err)
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
		return completion, false, modelresearch.NewResourceLimitError(ResourceLimitError{
			Kind:            ResourceLimitOutputTokens,
			Observed:        usage.CompletionTokens,
			ObservedKnown:   usageReported,
			InputTokens:     usage.PromptTokens,
			OutputTokens:    usage.CompletionTokens,
			ReasoningTokens: usage.CompletionTokenDetails.ReasoningTokens,
			FinishReason:    "length",
		}, completion.Content)
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

	if validateJSON {
		var validate json.RawMessage
		if err := json.Unmarshal([]byte(content), &validate); err != nil {
			return completion, false, fmt.Errorf("%w: %v", errJSONCompletionInvalid, err)
		}
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

func backoffDuration(attempt int) time.Duration {
	base := time.Duration(1<<(attempt-1)) * 500 * time.Millisecond
	jitter := time.Duration(float64(base) * (0.5 + rand.Float64()*0.5))
	return jitter
}
