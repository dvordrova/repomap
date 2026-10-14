package typesafe

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/llm"
)

// System One's envelope applies to every evaluation, including a direct flow
// choice outside tables. Without a published local tokenizer, each encoded UTF-8
// byte reserves one token; include all framing/options and the one output token.
const (
	RequestTokenLimit  = 64_000
	QuestionTokenLimit = 32_000
)

// checkEnvelope refuses a request over limit bytes (RequestTokenLimit when
// zero) or a question over QuestionTokenLimit or limit, whichever is less.
func checkEnvelope(body map[string]json.RawMessage, exact []byte, limit int) error {
	if limit <= 0 {
		limit = RequestTokenLimit
	}
	refused := func(input, limit int, scope string) error {
		if input+1 <= limit {
			return nil
		}
		return llm.NewResourceLimitError(llm.ResourceLimitError{
			Kind: llm.ResourceLimitContextTokens, Limit: limit,
			Observed: input + 1, ObservedKnown: true,
			ConfiguredMaxTokens: 1, FinishReason: "jev_" + scope + "_utf8_reservation",
		})
	}
	if err := refused(len(exact), limit, "request"); err != nil {
		return err
	}
	var questions map[string]json.RawMessage
	if err := json.Unmarshal(body["questions"], &questions); err != nil {
		return fmt.Errorf("typesafe: evaluation questions: %w", err)
	}
	// Preserve all top-level fields, including the model, when measuring a
	// question beside its complete shared state. Never shrink the sent body.
	single := make(map[string]json.RawMessage, len(body))
	for key, value := range body {
		single[key] = value
	}
	longest := 0
	for key, question := range questions {
		single["questions"], _ = json.Marshal(map[string]json.RawMessage{key: question})
		encoded, err := json.Marshal(single)
		if err != nil {
			return err
		}
		longest = max(longest, len(encoded))
	}
	return refused(longest, min(QuestionTokenLimit, limit), "question")
}

func positiveEnv(name string, fallback int) (int, error) {
	s := strings.TrimSpace(os.Getenv(name))
	if s == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", name, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}
	return n, nil
}
