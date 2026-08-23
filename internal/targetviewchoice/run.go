package targetviewchoice

import (
	"context"
	"fmt"

	"github.com/dvordrova/repomap/internal/llm"
)

// Run executes one already compiled cube through the shared provider layer.
// It never supplies a local fallback when provider execution or validation
// fails.
func Run(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	cube Cube,
) (Selection, error) {
	state, err := cube.State()
	if err != nil {
		return Selection{}, err
	}
	prompt, err := cube.BuildPrompt()
	if err != nil {
		return Selection{}, err
	}
	outcome, err := llm.ExecuteJSON(ctx, executor, provider, llm.Call[Selection]{
		State:  state,
		Prompt: prompt,
		Limits: llm.Limits{
			MaxRequestBytes:  MaxProviderRequestBytes,
			MaxResponseBytes: MaxResponseBytes,
			MaxOutputTokens:  MaxOutputTokens,
		},
		DecodeValidate: cube.ResolveResponse,
	})
	if err != nil {
		return Selection{}, fmt.Errorf("target view choice: model cube: %w", err)
	}
	return outcome.Value, nil
}
