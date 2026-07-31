package localization

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const PromptVersion = "localization-projection-json-v1"

const (
	maxPromptBytes         = 1 << 20
	maxPromptFields        = 4 << 10
	maxPromptTermsPerField = 64
)

const russianSystemPrompt = `You translate an allowlisted presentation projection from English to Russian.
Use only the supplied localization input. Do not add facts, explanations, fields, or identifiers.
Return valid json only, with exactly the requested projection shape.`

const russianUserPrefix = `Translate every human-readable text value in localization_input from English to Russian.

Return one json object with exactly this shape:
{"version":1,"canonical_sha256":"<copy canonical_sha256 exactly>","locale":"ru","translations":{"<copy each field id exactly>":"<Russian translation>"}}

Rules:
- Return exactly one translation for every input field ID and no unknown IDs.
- Copy canonical_sha256 and every field ID byte-for-byte.
- Preserve every placeholder such as {{term_01}} byte-for-byte and with the same count.
- Do not translate, alter, remove, duplicate, or invent placeholders.
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
