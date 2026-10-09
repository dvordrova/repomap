package typesafe

import (
	"encoding/json"
	"fmt"

	"github.com/dvordrova/repomap/internal/llm"
)

// System One's envelope applies to every evaluation, including a direct flow
// choice outside tables. Without a published local tokenizer, each encoded UTF-8
// byte reserves one token; include all framing/options and the one output token.
const (
	RequestTokenLimit  = 64_000
	QuestionTokenLimit = 32_000
)

func checkEnvelope(body map[string]json.RawMessage, exact []byte) error {
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
	if err := refused(len(exact), RequestTokenLimit, "request"); err != nil {
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
	return refused(longest, QuestionTokenLimit, "question")
}
