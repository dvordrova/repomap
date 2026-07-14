package deepseek

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/modelresearch"
)

// ComponentSynthesisPromptJSON returns the exact OpenAI-compatible request
// body used by SynthesizeComponentLandscape without making a provider call.
func (c *Client) ComponentSynthesisPromptJSON(prompt componentmap.SynthesisPrompt) ([]byte, error) {
	if err := validateComponentSynthesisPrompt(prompt); err != nil {
		return nil, err
	}
	request := c.flowExplainRequest(prompt.User, prompt.System, true)
	if isOfficialDeepSeekEndpoint(c.Endpoint) {
		request.Thinking = &thinkingConfig{Type: "disabled"}
	}
	return json.Marshal(request)
}

// SynthesizeComponentLandscape asks the provider for one bounded conceptual
// grouping proposal. The returned content is deliberately not decoded here:
// componentmap owns proposal validation and deterministic fallback.
func (c *Client) SynthesizeComponentLandscape(
	ctx context.Context,
	prompt componentmap.SynthesisPrompt,
) ([]byte, error) {
	result, err := c.SynthesizeComponentLandscapeMeasured(ctx, prompt)
	return result.Content, err
}

func (c *Client) SynthesizeComponentLandscapeMeasured(
	ctx context.Context,
	prompt componentmap.SynthesisPrompt,
) (modelresearch.ProviderResult, error) {
	stopWaiting := c.startWaitProgress(ctx, "architecture synthesis")
	defer stopWaiting()
	body, err := c.ComponentSynthesisPromptJSON(prompt)
	if err != nil {
		return modelresearch.ProviderResult{}, err
	}
	result, _, err := doChatMeasured(
		ctx,
		c.HTTPClient,
		c.Endpoint,
		c.APIKey,
		c.Auth,
		body,
		false,
	)
	return modelresearch.ProviderResult{
		Content: result.Content, Attempts: 1,
		InputTokens: result.InputTokens, OutputTokens: result.OutputTokens,
	}, err
}

func validateComponentSynthesisPrompt(prompt componentmap.SynthesisPrompt) error {
	if prompt.Version != componentmap.SynthesisPromptVersion {
		return fmt.Errorf(
			"llm: unsupported component synthesis prompt version %q",
			prompt.Version,
		)
	}
	if strings.TrimSpace(prompt.System) == "" {
		return fmt.Errorf("llm: component synthesis system prompt is required")
	}
	if strings.TrimSpace(prompt.User) == "" {
		return fmt.Errorf("llm: component synthesis user prompt is required")
	}
	return nil
}

func isOfficialDeepSeekEndpoint(endpoint string) bool {
	parsed, err := url.Parse(endpoint)
	return err == nil && strings.EqualFold(parsed.Hostname(), "api.deepseek.com")
}
