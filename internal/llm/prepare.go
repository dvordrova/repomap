package llm

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed prompts/response-language.md
var responseLanguagePrompt string

// Prepare applies the shared prose-language instruction before the provider
// encodes its request. Execution, fit checks and memo identities all use this
// boundary. Replay deliberately bypasses it to send the saved bytes unchanged.
func Prepare(provider Provider, prompt Prompt, limits Limits) (Prepared, error) {
	if provider == nil {
		return Prepared{}, fmt.Errorf("llm: provider is nil")
	}
	language := strings.ToLower(prompt.ResponseLanguage)
	if language == "" {
		language = "en"
	}
	// This is a language tag, never free-form instructions. Locale support is
	// owned by the presentation stage; preparation only checks its syntax.
	for i, part := range strings.Split(language, "-") {
		if len(part) == 0 || len(part) > 8 || i == 0 && len(part) < 2 {
			return Prepared{}, fmt.Errorf("llm: invalid response language tag")
		}
		for _, char := range part {
			if char < 'a' || char > 'z' {
				if i == 0 || char < '0' || char > '9' {
					return Prepared{}, fmt.Errorf("llm: invalid response language tag")
				}
			}
		}
	}
	name := fmt.Sprintf("the language identified by BCP 47 tag %q", language)
	if language == "en" {
		name = "English"
	}
	prompt.System = strings.ReplaceAll(strings.TrimSpace(responseLanguagePrompt), "{{language}}", name) + "\n\n" + prompt.System
	prompt.ResponseLanguage = language
	return provider.Prepare(prompt, limits)
}
