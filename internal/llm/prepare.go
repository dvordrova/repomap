package llm

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed prompts/response-language.md
var responseLanguagePrompt string

//go:embed prompts/response-format.md
var responseFormatPrompt string

// Prepare applies optional metadata, the final response shape and the shared
// prose language before provider encoding. Execution, fit checks and memo
// identities all use this boundary. Replay sends saved bytes unchanged.
func Prepare(provider Provider, prompt Prompt, limits Limits) (Prepared, error) {
	if provider == nil {
		return Prepared{}, fmt.Errorf("llm: provider is nil")
	}
	if adapter, ok := provider.(PromptAdapter); ok && !prompt.NoResponseAdjunct {
		var err error
		prompt, err = adapter.AdaptPrompt(prompt)
		if err != nil {
			return Prepared{}, err
		}
	}
	if prompt.ResponseExample != "" {
		if !json.Valid([]byte(prompt.ResponseExample)) {
			return Prepared{}, fmt.Errorf("llm: invalid response example")
		}
		prompt.System += "\n\n" + strings.TrimSpace(responseFormatPrompt) + "\n" + prompt.ResponseExample
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
