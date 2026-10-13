package run

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/typesafe"
)

// runTestLLM checks the model configuration of an ordinary run with one tiny
// request to the text model and one to the categorizer, and prints what each
// resolved to. The key is never printed.
func runTestLLM(ctx context.Context, stdout io.Writer) error {
	provider, err := deepseek.NewFromEnv()
	if err != nil {
		return fmt.Errorf("text model configuration: %w", err)
	}
	config := provider.EffectiveConfig()
	fmt.Fprintf(stdout, "text model: %s %s (auth %s, timeout %s, max tokens %d)\n",
		config.Endpoint, config.Model, config.AuthMode, config.Timeout, config.MaxTokens)
	answer, err := testLLMCall(ctx, provider, llm.Prompt{System: "Reply with the single word ok.", User: "Are you there?"}, 16)
	if err != nil {
		return fmt.Errorf("text model request: %w", err)
	}
	fmt.Fprintf(stdout, "text model answered: %q\n", strings.TrimSpace(answer))

	categorizer, err := newJevCategorizer()
	if err != nil {
		return fmt.Errorf("categorizer configuration: %w", err)
	}
	switch categorizer.(type) {
	case *typesafe.Client:
		fmt.Fprintln(stdout, "categorizer: Jev")
	case *typesafe.TextModel:
		fmt.Fprintln(stdout, "categorizer: the text model (JEV_KEY is not set)")
	}
	prompt, err := categorizer.Prompt("Classify files of a Go repository.", nil, map[string]llm.Question{
		"test": {Name: "file", Item: map[string]any{"path": "server/handler_test.go"}, Ask: "Is this file a test?"},
		"kind": {Name: "file", Item: map[string]any{"path": "cmd/server/main.go"}, Ask: "What is this file?",
			Options: []llm.Option{{Name: "entry", Meaning: "a program's main"}, {Name: "library", Meaning: "code other files import"}}},
	})
	if err != nil {
		return fmt.Errorf("categorizer prompt: %w", err)
	}
	response, err := testLLMCall(ctx, categorizer, prompt, 1)
	if err != nil {
		return fmt.Errorf("categorizer request: %w", err)
	}
	verdicts, err := categorizer.Verdicts([]byte(response))
	if err != nil {
		return fmt.Errorf("categorizer answers: %w", err)
	}
	test, kind := verdicts["test"], verdicts["kind"]
	if test.Yes == nil || kind.Choice == "" {
		return fmt.Errorf("categorizer left a question unanswered: %s", response)
	}
	fmt.Fprintf(stdout, "categorizer answered: handler_test.go is a test at %.2f; main.go is %q\n", *test.Yes, kind.Choice)
	fmt.Fprintln(stdout, "ok")
	return nil
}

func testLLMCall(ctx context.Context, provider llm.Provider, prompt llm.Prompt, outputTokens int) (string, error) {
	prepared, err := provider.Prepare(prompt, llm.Limits{
		MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: outputTokens,
	})
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	completion, err := provider.Complete(ctx, prepared)
	if err != nil {
		return "", err
	}
	return string(completion.Response), nil
}
