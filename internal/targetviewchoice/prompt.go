package targetviewchoice

import (
	"fmt"

	"github.com/dvordrova/repomap/internal/llm"
)

// PromptVersion changes whenever either readable static prompt changes.
var PromptVersion = "target-view-choice-prompt-" + shortSHA256(promptSystem+"\x00"+promptUserShape)

// BuildPrompt binds the complete provider-visible catalog to the static cube
// instructions. No private target identity is included.
func (cube Cube) BuildPrompt() (llm.Prompt, error) {
	wire, err := cube.ProviderVisibleJSON()
	if err != nil {
		return llm.Prompt{}, err
	}
	return llm.Prompt{
		System:             promptSystem,
		User:               fmt.Sprintf(promptUserShape, wire),
		ResponseFormatJSON: true,
	}, nil
}

func shortSHA256(value string) string {
	return sha256Hex([]byte(value))[:12]
}
