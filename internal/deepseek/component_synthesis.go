package deepseek

import (
	"context"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/componentmap"
)

// ComponentSynthesisPromptJSON returns the exact OpenAI-compatible request
// body used by SynthesizeComponentLandscape without making a provider call.
func (c *Client) ComponentSynthesisPromptJSON(prompt componentmap.SynthesisPrompt) ([]byte, error) {
	if err := validateComponentSynthesisPrompt(prompt); err != nil {
		return nil, err
	}
	return c.FlowExplainPromptJSON(prompt.User, prompt.System)
}

// SynthesizeComponentLandscape asks the provider for one bounded conceptual
// grouping proposal. The returned content is deliberately not decoded here:
// componentmap owns proposal validation and deterministic fallback.
func (c *Client) SynthesizeComponentLandscape(
	ctx context.Context,
	prompt componentmap.SynthesisPrompt,
) ([]byte, error) {
	body, err := c.ComponentSynthesisPromptJSON(prompt)
	if err != nil {
		return nil, err
	}
	result, _, err := doChat(
		ctx,
		c.HTTPClient,
		c.Endpoint,
		c.APIKey,
		c.Auth,
		body,
		false,
	)
	return result, err
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
