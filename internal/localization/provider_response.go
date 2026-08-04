package localization

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/modelresearch"
)

const (
	ProviderResponseVersion  = 1
	maxProviderResponseBytes = modelresearch.ProviderResponseByteLimit
)

// ProviderResponse is the compact provider-facing projection envelope. Stable
// presentation addresses remain in Input and in the internal Projection; the
// provider returns short positional bindings so hundreds of long opaque field
// IDs do not consume the bounded completion envelope a second time.
type ProviderResponse struct {
	Version         int                   `json:"version"`
	CanonicalSHA256 string                `json:"canonical_sha256"`
	Locale          string                `json:"locale"`
	Translations    []ProviderTranslation `json:"translations"`
}

type ProviderTranslation struct {
	Index *int
	Text  *string
}

// ProviderResponseDiagnostics contains only closed structural counts. It never
// carries provider-authored text.
type ProviderResponseDiagnostics struct {
	UnrequestedTranslations int
}

func NewProviderTranslation(index int, text string) ProviderTranslation {
	return ProviderTranslation{Index: &index, Text: &text}
}

func (translation ProviderTranslation) MarshalJSON() ([]byte, error) {
	if translation.Index == nil || translation.Text == nil {
		return nil, fmt.Errorf("localization: invalid provider translation")
	}
	return json.Marshal([]any{*translation.Index, *translation.Text})
}

func (translation *ProviderTranslation) UnmarshalJSON(data []byte) error {
	if translation == nil {
		return fmt.Errorf("localization: invalid provider translation")
	}
	var values []json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil || len(values) != 2 {
		return fmt.Errorf("localization: invalid provider translation")
	}
	var index int
	var text string
	if bytes.Equal(bytes.TrimSpace(values[0]), []byte("null")) {
		return fmt.Errorf("localization: invalid provider translation")
	}
	if err := json.Unmarshal(values[0], &index); err != nil {
		return fmt.Errorf("localization: invalid provider translation")
	}
	if bytes.Equal(bytes.TrimSpace(values[1]), []byte("null")) {
		return fmt.Errorf("localization: invalid provider translation")
	}
	if err := json.Unmarshal(values[1], &text); err != nil {
		return fmt.Errorf("localization: invalid provider translation")
	}
	translation.Index = &index
	translation.Text = &text
	return nil
}

// DecodeRussianProviderResponse restores the stable ID-keyed internal
// projection from one strict, complete, position-bound provider response.
// Missing, duplicate, or reordered requested entries and any trailing entry
// inside the requested namespace fail closed before Apply.
func DecodeRussianProviderResponse(
	canonical CanonicalArtifact,
	input Input,
	data []byte,
) (Projection, error) {
	projection, _, err := DecodeRussianProviderResponseDetailed(canonical, input, data)
	return projection, err
}

// DecodeRussianProviderResponseDetailed accepts the exact requested prefix
// once and in order. Provider tuples after that prefix are ignored only when
// every trailing index is outside the requested namespace.
func DecodeRussianProviderResponseDetailed(
	canonical CanonicalArtifact,
	input Input,
	data []byte,
) (Projection, ProviderResponseDiagnostics, error) {
	if err := validateCanonical(canonical); err != nil {
		return Projection{}, ProviderResponseDiagnostics{}, err
	}
	if err := validateInput(canonical, input); err != nil {
		return Projection{}, ProviderResponseDiagnostics{}, err
	}
	if len(data) > maxProviderResponseBytes {
		return Projection{}, ProviderResponseDiagnostics{}, &modelresearch.ResourceLimitError{
			Stage: "localization", Kind: modelresearch.ResourceLimitResponseBytes,
			Limit: maxProviderResponseBytes, Observed: len(data), ObservedKnown: true,
		}
	}
	if input.SourceLocale != LocaleEnglish ||
		input.TargetLocale != LocaleRussian ||
		len(data) == 0 || !utf8.Valid(data) {
		return Projection{}, ProviderResponseDiagnostics{}, fmt.Errorf("localization: invalid provider response")
	}
	var response ProviderResponse
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return Projection{}, ProviderResponseDiagnostics{}, fmt.Errorf("localization: invalid provider response")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Projection{}, ProviderResponseDiagnostics{}, fmt.Errorf("localization: invalid provider response")
	}
	if response.Version != ProviderResponseVersion ||
		response.CanonicalSHA256 != canonical.SHA256 ||
		response.Locale != input.TargetLocale ||
		len(response.Translations) < len(input.Fields) {
		return Projection{}, ProviderResponseDiagnostics{}, fmt.Errorf("localization: invalid provider response")
	}

	translations := make(map[string]string, len(input.Fields))
	for index, translation := range response.Translations[:len(input.Fields)] {
		if translation.Index == nil || *translation.Index != index ||
			translation.Text == nil {
			return Projection{}, ProviderResponseDiagnostics{}, fmt.Errorf("localization: invalid provider response")
		}
		translations[input.Fields[index].ID] = *translation.Text
	}
	for _, translation := range response.Translations[len(input.Fields):] {
		if translation.Index == nil || *translation.Index < len(input.Fields) ||
			translation.Text == nil {
			return Projection{}, ProviderResponseDiagnostics{}, fmt.Errorf("localization: invalid provider response")
		}
	}
	diagnostics := ProviderResponseDiagnostics{
		UnrequestedTranslations: len(response.Translations) - len(input.Fields),
	}
	return Projection{
		Version:         ProjectionVersion,
		CanonicalSHA256: canonical.SHA256,
		Locale:          input.TargetLocale,
		Translations:    translations,
	}, diagnostics, nil
}
