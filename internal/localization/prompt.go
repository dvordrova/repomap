package localization

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const PromptVersion = "localization-projection-json-v4"

const (
	maxPromptBytes         = 1 << 20
	maxPromptFields        = 4 << 10
	maxPromptTermsPerField = 64
)

const russianSystemPrompt = `You translate a complete bounded presentation inventory from English to Russian.
Use only the supplied localization input. Do not add facts, explanations, fields, or identifiers.
Return valid json only, with exactly the requested projection shape.`

const russianUserPrefix = `Translate every human-readable text value in localization_input from English to Russian.

Return one json object with exactly this shape:
{"version":1,"canonical_sha256":"<copy canonical_sha256 exactly>","locale":"ru","translations":[[0,"<Russian translation of localization_input.fields[0].text>"]]}

Rules:
- Return exactly one translation entry for every input field, in the same order as localization_input.fields.
- Each translation entry is exactly [index, text]. Start indexes at 0. Every entry index must equal its array position, with no missing, duplicate, reordered, or extra entries.
- Copy canonical_sha256 byte-for-byte. Field IDs are opaque input-only addresses: do not copy or translate them into the response.
- Preserve every placeholder such as {{term_01}} byte-for-byte and with the same count.
- Do not translate, alter, remove, duplicate, or invent placeholders.
- Translate every English prose word outside placeholders, including short headings, labels, names, summaries, diagnostics, and technical explanations.
- Latin-script technical identities are allowed only where the input supplies a placeholder. Translate unprotected descriptive technical words into natural Russian instead of copying them.
- Every value that contains prose outside placeholders must contain Cyrillic Russian text. A value made only of placeholders must remain only those placeholders.
- Before returning, check every value and remove any leftover unprotected English prose.
- Values in translations contain only presentation prose. Do not return markdown, code fences, commentary, or additional json fields.

localization_input:
`

// Prompt is a provider-neutral exact localization prompt. A provider adapter
// may later map System and User into its own request envelope; this contract
// does not contain transport, model, endpoint, or generation parameters.
type Prompt struct {
	Version string `json:"version"`
	System  string `json:"system"`
	User    string `json:"user"`
}

// BuildRussianPrompt builds one deterministic English-to-Russian prompt from
// an already validated canonical artifact and its exact localization input.
func BuildRussianPrompt(canonical CanonicalArtifact, input Input) (Prompt, error) {
	if err := preflightRussianPromptInput(canonical, input); err != nil {
		return Prompt{}, err
	}
	if err := validateInput(canonical, input); err != nil {
		return Prompt{}, fmt.Errorf("localization: build prompt: %w", err)
	}
	if input.SourceLocale != LocaleEnglish || input.TargetLocale != LocaleRussian {
		return Prompt{}, fmt.Errorf("localization: build prompt: unsupported locale direction")
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return Prompt{}, fmt.Errorf("localization: encode prompt input: %w", err)
	}
	if len(inputJSON) == 0 ||
		len(inputJSON) > maxPromptBytes-len(russianUserPrefix) {
		return Prompt{}, fmt.Errorf("localization: prompt exceeds its byte limit")
	}
	var user strings.Builder
	user.Grow(len(russianUserPrefix) + len(inputJSON))
	user.WriteString(russianUserPrefix)
	user.Write(inputJSON)
	prompt := Prompt{
		Version: PromptVersion,
		System:  russianSystemPrompt,
		User:    user.String(),
	}
	if _, err := MarshalPrompt(prompt); err != nil {
		return Prompt{}, err
	}
	return prompt, nil
}

// MarshalPrompt returns the compact deterministic provider-neutral prompt
// bytes and enforces the stage boundary before they can reach an adapter.
func MarshalPrompt(prompt Prompt) ([]byte, error) {
	if len(prompt.System) > maxPromptBytes ||
		len(prompt.User) > maxPromptBytes-len(prompt.System) {
		return nil, fmt.Errorf("localization: prompt exceeds its byte limit")
	}
	if prompt.Version != PromptVersion ||
		strings.TrimSpace(prompt.System) == "" ||
		strings.TrimSpace(prompt.User) == "" ||
		!utf8.ValidString(prompt.System) ||
		!utf8.ValidString(prompt.User) {
		return nil, fmt.Errorf("localization: invalid prompt")
	}
	encoded, err := json.Marshal(prompt)
	if err != nil {
		return nil, fmt.Errorf("localization: encode prompt: %w", err)
	}
	if len(encoded) == 0 || len(encoded) > maxPromptBytes {
		return nil, fmt.Errorf("localization: prompt exceeds its byte limit")
	}
	return encoded, nil
}

func preflightRussianPromptInput(canonical CanonicalArtifact, input Input) error {
	if len(canonical.Fields) > maxPromptFields ||
		len(input.Fields) > maxPromptFields {
		return fmt.Errorf("localization: prompt exceeds its byte limit")
	}
	boundedAdd := func(total *int, values ...string) bool {
		for _, value := range values {
			if len(value) > maxPromptBytes-*total {
				return false
			}
			*total += len(value)
		}
		return true
	}
	canonicalBytes := 0
	if !boundedAdd(
		&canonicalBytes,
		canonical.Locale,
		canonical.SHA256,
	) {
		return fmt.Errorf("localization: prompt exceeds its byte limit")
	}
	for _, field := range canonical.Fields {
		if len(field.ProtectedTerms) > maxPromptTermsPerField ||
			!boundedAdd(
				&canonicalBytes,
				field.ID,
				string(field.OwnerKind),
				field.OwnerID,
				string(field.Name),
				field.Text,
			) {
			return fmt.Errorf("localization: prompt exceeds its byte limit")
		}
		for _, term := range field.ProtectedTerms {
			if !boundedAdd(
				&canonicalBytes,
				term.Token,
				string(term.Kind),
				term.Value,
			) {
				return fmt.Errorf("localization: prompt exceeds its byte limit")
			}
		}
	}
	inputBytes := 0
	if !boundedAdd(
		&inputBytes,
		input.CanonicalSHA256,
		input.SourceLocale,
		input.TargetLocale,
	) {
		return fmt.Errorf("localization: prompt exceeds its byte limit")
	}
	for _, field := range input.Fields {
		if len(field.Placeholders) > maxPromptTermsPerField ||
			!boundedAdd(&inputBytes, field.ID, field.Text) {
			return fmt.Errorf("localization: prompt exceeds its byte limit")
		}
		for _, placeholder := range field.Placeholders {
			if !boundedAdd(
				&inputBytes,
				placeholder.Token,
				string(placeholder.Kind),
			) {
				return fmt.Errorf("localization: prompt exceeds its byte limit")
			}
		}
	}
	return nil
}
