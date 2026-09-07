// Package reporttranslation translates the report's explicitly collected model
// prose after analysis, without giving a provider control over report structure.
package reporttranslation

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/report"
)

const StageName = "report_translation"

//go:embed prompt.md
var translationPrompt string

type modelResponse struct {
	Translations []responseEntry
}

type responseEntry struct {
	Ref  string
	Text string
}

type translationWindow []report.DisplayTextEntry

// Translate returns a complete presentation-only translation bound to catalog.
// The caller decides whether its selected display language needs translation;
// an empty catalogue needs no provider. Failed windows never become a partial
// display artifact. Exact requests, cache, journals and concurrency belong to
// the supplied shared executor.
func Translate(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	catalog report.DisplayTextCatalog,
	language report.DisplayLanguage,
) (report.DisplayTranslations, error) {
	if err := catalog.Validate(); err != nil {
		return report.DisplayTranslations{}, err
	}
	language, err := report.NormalizeDisplayLanguage(string(language))
	if err != nil {
		return report.DisplayTranslations{}, err
	}
	if err := ctx.Err(); err != nil {
		return report.DisplayTranslations{}, err
	}
	result := report.DisplayTranslations{
		Version: report.DisplayTextVersion, Language: language,
		CatalogSHA256: catalog.SHA256,
		Entries:       make([]report.DisplayTranslationEntry, 0, len(catalog.Entries)),
	}
	if len(catalog.Entries) == 0 {
		return result, result.Validate(catalog)
	}
	windows, err := planWindows(ctx, provider, catalog.Entries, language)
	if err != nil {
		return report.DisplayTranslations{}, err
	}
	_, outcomes, err := llm.ExecuteAdaptiveJSONBatch(
		ctx, executor, provider, windows,
		func(plan []translationWindow) ([]llm.Call[[]report.DisplayTranslationEntry], error) {
			calls := make([]llm.Call[[]report.DisplayTranslationEntry], len(plan))
			for i, window := range plan {
				call, err := translationCall(window, language)
				if err != nil {
					return nil, err
				}
				calls[i] = call
			}
			return calls, nil
		},
		func(window translationWindow) (translationWindow, translationWindow, bool) {
			if len(window) <= 1 {
				return nil, nil, false
			}
			middle := len(window) / 2
			return window[:middle], window[middle:], true
		},
	)
	if err != nil {
		return report.DisplayTranslations{}, fmt.Errorf("report translation: %w", err)
	}
	for _, outcome := range outcomes {
		result.Entries = append(result.Entries, outcome.Value...)
	}
	if err := result.Validate(catalog); err != nil {
		return report.DisplayTranslations{}, err
	}
	return result, nil
}

// planWindows grows each consecutive prefix until the actual prepared request
// stops fitting, then locates the last fitting prefix. There is no row ceiling;
// exponential growth avoids preparing every intermediate catalogue prefix.
// Every accepted prefix is checked through the same language policy and
// provider preparation that execution uses.
func planWindows(
	ctx context.Context,
	provider llm.Provider,
	entries []report.DisplayTextEntry,
	language report.DisplayLanguage,
) ([]translationWindow, error) {
	var windows []translationWindow
	for start := 0; start < len(entries); {
		remaining := len(entries) - start
		fits := func(count int) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			call, err := translationCall(entries[start:start+count], language)
			if err != nil {
				return err
			}
			prepared, err := llm.Prepare(provider, call.Prompt, call.Limits)
			if err != nil {
				return err
			}
			if prepared.Len() > call.Limits.MaxRequestBytes {
				return llm.NewResourceLimitError(llm.ResourceLimitError{
					Stage: StageName, Kind: llm.ResourceLimitRequestBytes,
					Limit: call.Limits.MaxRequestBytes, Observed: prepared.Len(), ObservedKnown: true,
				})
			}
			return nil
		}
		if err := fits(1); err != nil {
			return nil, fmt.Errorf("report translation: prepare %s: %w", entries[start].Ref, err)
		}
		good, bad := 1, remaining+1
		for good < remaining {
			next := remaining
			if good <= remaining/2 {
				next = good * 2
			}
			if err := fits(next); err != nil {
				if !requestTooLarge(err) {
					return nil, fmt.Errorf("report translation: prepare window: %w", err)
				}
				bad = next
				break
			}
			good = next
		}
		for bad-good > 1 {
			middle := good + (bad-good)/2
			if err := fits(middle); err != nil {
				if !requestTooLarge(err) {
					return nil, fmt.Errorf("report translation: prepare window: %w", err)
				}
				bad = middle
			} else {
				good = middle
			}
		}
		windows = append(windows, translationWindow(entries[start:start+good]))
		start += good
	}
	return windows, nil
}

func requestTooLarge(err error) bool {
	var resourceErr *llm.ResourceLimitError
	return errors.As(err, &resourceErr) && resourceErr.Kind == llm.ResourceLimitRequestBytes
}

func translationCall(
	window translationWindow,
	language report.DisplayLanguage,
) (llm.Call[[]report.DisplayTranslationEntry], error) {
	// Keep the catalogue's order, including t2 before t10. A Go map would
	// reorder these keys lexicographically. Role and protected-source metadata
	// stay local; the text already contains every required placeholder token.
	var request bytes.Buffer
	request.WriteByte('{')
	for i, entry := range window {
		if i > 0 {
			request.WriteByte(',')
		}
		ref, _ := json.Marshal(entry.Ref)
		text, _ := json.Marshal(entry.Text)
		request.Write(ref)
		request.WriteByte(':')
		request.Write(text)
	}
	request.WriteByte('}')
	return llm.Call[[]report.DisplayTranslationEntry]{
		State: []byte(`{"contract":"repomap.report-display-translation.v4"}`),
		Prompt: llm.Prompt{
			System: strings.TrimSpace(translationPrompt), User: request.String(),
			ResponseFormatJSON: true, ResponseLanguage: string(language), Reasoning: false,
		},
		Limits: llm.Limits{
			MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit,
			MaxOutputTokens: llm.DefaultMaxOutputTokens,
		},
		DecodeValidate: func(raw []byte) ([]report.DisplayTranslationEntry, error) {
			response, err := decodeTranslationResponse(raw, window)
			if err != nil {
				return nil, err
			}
			return normalizeTranslations(window, response)
		},
	}, nil
}

// Keep duplicate object members until normalization can compare their values.
// Decoding straight into a map would silently give the last occurrence authority.
func decodeTranslationResponse(raw []byte, window translationWindow) (modelResponse, error) {
	var response modelResponse
	known := make(map[string]bool, len(window))
	for _, entry := range window {
		known[entry.Ref] = true
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	expect := func(delimiter json.Delim) error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		if token != delimiter {
			return fmt.Errorf("report translation: expected JSON %q", delimiter)
		}
		return nil
	}
	if err := expect('{'); err != nil {
		return response, err
	}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return response, err
		}
		ref, ok := token.(string)
		if !ok {
			return response, fmt.Errorf("report translation: expected a translation ref")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return response, err
		}
		if !known[ref] {
			continue
		}
		if len(value) == 0 || value[0] != '"' {
			return response, fmt.Errorf("report translation: translation for %s must be a string", ref)
		}
		var text string
		if err := json.Unmarshal(value, &text); err != nil {
			return response, err
		}
		response.Translations = append(response.Translations, responseEntry{Ref: ref, Text: text})
	}
	if err := expect('}'); err != nil {
		return response, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return response, fmt.Errorf("report translation: unexpected data after translations object")
	}
	return response, nil
}

func normalizeTranslations(window translationWindow, response modelResponse) ([]report.DisplayTranslationEntry, error) {
	allowed := make(map[string]report.DisplayTextEntry, len(window))
	for _, entry := range window {
		allowed[entry.Ref] = entry
	}
	byRef := make(map[string]string, len(window))
	for _, translation := range response.Translations {
		entry, known := allowed[translation.Ref]
		if !known {
			continue
		}
		if previous, duplicate := byRef[translation.Ref]; duplicate {
			if previous != translation.Text {
				return nil, fmt.Errorf("report translation: conflicting translations for %s", translation.Ref)
			}
			continue
		}
		if err := entry.ValidateTranslation(translation.Text); err != nil {
			return nil, err
		}
		byRef[translation.Ref] = translation.Text
	}
	translations := make([]report.DisplayTranslationEntry, 0, len(window))
	for _, entry := range window {
		text, present := byRef[entry.Ref]
		if !present {
			return nil, fmt.Errorf("report translation: missing translation for %s", entry.Ref)
		}
		translations = append(translations, report.DisplayTranslationEntry{Ref: entry.Ref, Text: text})
	}
	return translations, nil
}
