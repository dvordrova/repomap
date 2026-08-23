package readmetargetscout

import "fmt"

var PromptVersion = "readme-target-scout-prompt-" + shortSHA256(promptSystem+"\x00"+promptUserShape)

// ProviderVisibleJSON returns an independently owned copy of the complete
// atomic semantic request.
func ProviderVisibleJSON(compilation Compilation) ([]byte, error) {
	if err := validateReadyCompilation(compilation); err != nil {
		return nil, err
	}
	return append([]byte(nil), compilation.wire...), nil
}

func BuildPrompt(compilation Compilation) (Prompt, error) {
	wire, err := ProviderVisibleJSON(compilation)
	if err != nil {
		return Prompt{}, err
	}
	return Prompt{
		Version: PromptVersion,
		System:  promptSystem,
		User:    fmt.Sprintf(promptUserShape, wire),
	}, nil
}

func shortSHA256(value string) string {
	return sha256Hex([]byte(value))[:12]
}
