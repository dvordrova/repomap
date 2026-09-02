package targetportfolio

import "fmt"

var PromptVersion = "target-portfolio-prompt-" + shortSHA256(
	promptSystem+promptUserShape+defaultPromptSystem+defaultPromptUserShape,
)

// ProviderVisibleJSON returns the complete retained fact reservoir. It is the
// sole shape permitted to cross the provider boundary, but a large compilation
// is an authority/debug projection: Run sends deterministic bounded subsets.
// Corpus and cache identities stay in Compilation's private authority.
func ProviderVisibleJSON(compilation Compilation) ([]byte, error) {
	if err := validateCompilation(compilation); err != nil {
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
