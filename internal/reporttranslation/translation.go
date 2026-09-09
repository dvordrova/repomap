// Package reporttranslation translates the report's explicitly collected model
// prose after analysis, without giving a provider control over report structure.
package reporttranslation

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/report"
)

const StageName = "report_translation"

// Expiry returns to the complete-entry splitter. HTTP 500 may arrive before
// this deadline and has its own explicit split policy on divisible windows.
const attemptTimeout = 4 * time.Minute

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

type requestTerm struct {
	Ref         string `json:"ref"`
	Spelling    string `json:"spelling"`
	Explanation string `json:"explanation"`
}

// The wire projection deliberately omits local term IDs, source links and
// question scopes. Definitions only provide context for the surrounding prose.
type requestEntry struct {
	Ref   string   `json:"ref"`
	Role  string   `json:"role"`
	Text  string   `json:"text"`
	Terms []string `json:"terms,omitempty"`
}

type translationRequest struct {
	Terms   []requestTerm  `json:"terms,omitempty"`
	Entries []requestEntry `json:"entries"`
}

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
	if len(windows) < executor.BatchConcurrency {
		var pending []translationWindow
		for _, window := range windows {
			call, err := translationCall(window, language)
			if err != nil {
				return report.DisplayTranslations{}, err
			}
			cached, err := llm.RecallJSON(ctx, executor, provider, call)
			if err != nil {
				return report.DisplayTranslations{}, err
			}
			if cached.Cached {
				result.Entries = append(result.Entries, cached.Value...)
			} else {
				pending = append(pending, window)
			}
		}
		windows = parallelWindows(pending, executor.BatchConcurrency)
	}
	if len(windows) == 0 {
		return result, result.Validate(catalog)
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
	// Whole cached windows and new windows may interleave. Restore the original
	// catalogue order before validating the single complete display artifact.
	order := make(map[string]int, len(catalog.Entries))
	for i, entry := range catalog.Entries {
		order[entry.Ref] = i
	}
	sort.Slice(result.Entries, func(i, j int) bool { return order[result.Entries[i].Ref] < order[result.Entries[j].Ref] })
	if err := result.Validate(catalog); err != nil {
		return report.DisplayTranslations{}, err
	}
	return result, nil
}

// Fill the existing worker pool before asking for new translations. UTF-8
// text bytes balance generation work; they are not a token estimate or a size
// cutoff. Every entry stays whole and translationCall rebuilds each child's
// complete term dictionary. Provider-envelope limits still apply separately.
func parallelWindows(windows []translationWindow, workers int) []translationWindow {
	weight := func(window translationWindow) int {
		total := 0
		for _, entry := range window {
			total += max(1, len(entry.Text))
		}
		return total
	}
	for len(windows) < workers {
		largest, size := -1, 0
		for i, window := range windows {
			if len(window) > 1 {
				if n := weight(window); n > size {
					largest, size = i, n
				}
			}
		}
		if largest < 0 {
			break
		}
		window := windows[largest]
		middle, distance, left := 1, size, 0
		for i := 1; i < len(window); i++ {
			left += max(1, len(window[i-1].Text))
			delta := size - 2*left
			if delta < 0 {
				delta = -delta
			}
			if delta < distance {
				middle, distance = i, delta
			}
		}
		next := make([]translationWindow, 0, len(windows)+1)
		next = append(next, windows[:largest]...)
		next = append(next, window[:middle], window[middle:])
		windows = append(next, windows[largest+1:]...)
	}
	return windows
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
	// Keep catalogue order and definition context in this same request. Term
	// spellings stay visible; only existing source syntax uses placeholders.
	request := translationRequest{Entries: make([]requestEntry, len(window))}
	termRefs := make(map[[2]string]string)
	for i, entry := range window {
		request.Entries[i] = requestEntry{Ref: entry.Ref, Role: entry.Role, Text: entry.Text}
		seen := make(map[string]bool)
		for _, term := range entry.Terms {
			key := [2]string{term.Spelling, term.Explanation}
			ref, exists := termRefs[key]
			if !exists {
				ref = fmt.Sprintf("d%d", len(request.Terms)+1)
				termRefs[key] = ref
				request.Terms = append(request.Terms, requestTerm{Ref: ref, Spelling: term.Spelling, Explanation: term.Explanation})
			}
			if !seen[ref] {
				request.Entries[i].Terms = append(request.Entries[i].Terms, ref)
				seen[ref] = true
			}
		}
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return llm.Call[[]report.DisplayTranslationEntry]{}, err
	}
	return llm.Call[[]report.DisplayTranslationEntry]{
		SplitRejectedResponse: true,
		SplitHTTP500:          len(window) > 1,
		State:                 []byte(`{"contract":"repomap.report-display-translation.v9"}`),
		Prompt: llm.Prompt{
			System: strings.TrimSpace(translationPrompt), User: string(raw),
			ResponseFormatJSON: true, ResponseLanguage: string(language), Reasoning: false,
		},
		Limits: llm.Limits{
			MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit,
			MaxOutputTokens: llm.DefaultMaxOutputTokens, AttemptTimeout: attemptTimeout,
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

type wireField struct {
	name  string
	value json.RawMessage
}

func objectFields(raw []byte) ([]wireField, error) {
	// Keep only the last raw value of each object key before validating its
	// shape or text. A shadowed value has no translation authority.
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	if values == nil {
		return nil, fmt.Errorf("report translation: expected JSON object")
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	fields := make([]wireField, 0, len(names))
	for _, name := range names {
		fields = append(fields, wireField{name, values[name]})
	}
	return fields, nil
}

func decodeTranslationResponse(raw []byte, window translationWindow) (modelResponse, error) {
	var response modelResponse
	known := make(map[string]report.DisplayTextEntry, len(window))
	for _, entry := range window {
		known[entry.Ref] = entry
	}
	fields, err := objectFields(raw)
	if err != nil {
		return response, err
	}
	for _, field := range fields {
		entry, ok := known[field.name]
		if !ok {
			continue
		}
		translation, err := decodeTranslationValue(field.value, entry)
		if err != nil {
			return response, err
		}
		response.Translations = append(response.Translations, translation)
	}
	return response, nil
}

func decodeTranslationValue(raw []byte, entry report.DisplayTextEntry) (responseEntry, error) {
	result := responseEntry{Ref: entry.Ref}
	fields, err := objectFields(raw)
	if err != nil {
		return result, err
	}
	textSeen := false
	for _, field := range fields {
		switch field.name {
		case "text":
			var text *string
			if err := json.Unmarshal(field.value, &text); err != nil || text == nil {
				return result, fmt.Errorf("report translation: text for %s must be a string", entry.Ref)
			}
			result.Text, textSeen = *text, true
		default:
			return result, fmt.Errorf("report translation: unsupported translated entry field %q", field.name)
		}
	}
	if !textSeen {
		return result, fmt.Errorf("report translation: missing text for %s", entry.Ref)
	}
	return result, nil
}

func normalizeTranslations(window translationWindow, response modelResponse) ([]report.DisplayTranslationEntry, error) {
	allowed := make(map[string]report.DisplayTextEntry, len(window))
	for _, entry := range window {
		allowed[entry.Ref] = entry
	}
	byRef := make(map[string]report.DisplayTranslationEntry, len(window))
	for _, translation := range response.Translations {
		entry, known := allowed[translation.Ref]
		if !known {
			continue
		}
		if err := entry.ValidateTranslation(translation.Text); err != nil {
			return nil, err
		}
		value := report.DisplayTranslationEntry{Ref: translation.Ref, Text: translation.Text}
		byRef[translation.Ref] = value
	}
	translations := make([]report.DisplayTranslationEntry, 0, len(window))
	for _, entry := range window {
		value, present := byRef[entry.Ref]
		if !present {
			return nil, fmt.Errorf("report translation: missing translation for %s", entry.Ref)
		}
		translations = append(translations, value)
	}
	return translations, nil
}
