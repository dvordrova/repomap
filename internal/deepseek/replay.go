package deepseek

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/llm"
)

// PrepareReplay restores a saved provider request without rebuilding it from
// today's defaults. Unknown provider options are preserved in the exact bytes.
// Endpoint, authentication and transport policy still belong to Client.
func PrepareReplay(raw []byte) (llm.Prepared, error) {
	var request struct {
		Model       string        `json:"model"`
		Messages    []chatMessage `json:"messages"`
		Temperature *float64      `json:"temperature"`
		MaxTokens   int           `json:"max_tokens"`
		Stream      bool          `json:"stream"`
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		return llm.Prepared{}, fmt.Errorf("replay: invalid provider request JSON: %w", err)
	}
	if strings.TrimSpace(request.Model) == "" || len(request.Messages) == 0 || request.MaxTokens <= 0 || request.Temperature == nil {
		return llm.Prepared{}, fmt.Errorf("replay: request needs model, messages, max_tokens and temperature; use .llm-cache/payloads/<request-sha>.json, not a tables/ input")
	}
	for i, message := range request.Messages {
		if strings.TrimSpace(message.Role) == "" || strings.TrimSpace(message.Content) == "" {
			return llm.Prepared{}, fmt.Errorf("replay: message %d needs role and content", i+1)
		}
	}
	if request.Stream {
		return llm.Prepared{}, fmt.Errorf("replay: streaming requests are not supported by this client")
	}
	return llm.NewPrepared(raw)
}
