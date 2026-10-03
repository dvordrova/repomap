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

const initialWindows = 8

//go:embed prompt.md
var translationPrompt string

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

// Untranslated names one display text that keeps its source language: it was
// refused again after being re-asked in a smaller request, or its one-text
// request was still refused after every split. The executor has journaled
// that refusal. Reason is that final refusal.
type Untranslated struct {
	Ref    string
	Reason string
}

// Translate returns a complete presentation-only translation bound to catalog.
// The caller decides whether its selected display language needs translation;
// an empty catalogue needs no provider. Texts are accepted independently: a
// text refused or missing in an accepted answer is re-asked once, with the
// other refused texts of its window, in a smaller request, and keeps its
// source language if that answer refuses it again. An answer that translates
// nothing divides until one text remains, and a text refused on its own keeps
// its source language. Kept texts are named in the returned list. Exact requests,
// cache, journals and concurrency belong to the supplied shared executor.
func Translate(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	catalog report.DisplayTextCatalog,
	language report.DisplayLanguage,
) (report.DisplayTranslations, []Untranslated, error) {
	if err := catalog.Validate(); err != nil {
		return report.DisplayTranslations{}, nil, err
	}
	language, err := report.NormalizeDisplayLanguage(string(language))
	if err != nil {
		return report.DisplayTranslations{}, nil, err
	}
	if err := ctx.Err(); err != nil {
		return report.DisplayTranslations{}, nil, err
	}
	result := report.DisplayTranslations{
		Version: report.DisplayTextVersion, Language: language,
		CatalogSHA256: catalog.SHA256,
		Entries:       make([]report.DisplayTranslationEntry, 0, len(catalog.Entries)),
	}
	if len(catalog.Entries) == 0 {
		return result, nil, result.Validate(catalog)
	}
	windows, indivisible, err := planWindows(ctx, provider, catalog.Entries, language)
	if err != nil {
		return report.DisplayTranslations{}, nil, err
	}
	var untranslated []Untranslated
	keep := func(entry report.DisplayTextEntry, reason string) {
		result.Entries = append(result.Entries, report.DisplayTranslationEntry{Ref: entry.Ref, Text: entry.Text})
		untranslated = append(untranslated, Untranslated{Ref: entry.Ref, Reason: reason})
	}
	// A text whose request alone does not fit the provider keeps its source
	// language and is named; its neighbours are still asked.
	for _, oversized := range indivisible {
		keep(oversized.entry, oversized.reason)
	}
	var followUps []translationWindow
	accept := func(window translationWindow, value windowTranslation) {
		result.Entries = append(result.Entries, value.Entries...)
		if retry := value.retry(window); len(retry) > 0 {
			followUps = append(followUps, retry)
		}
	}
	windowCount := max(initialWindows, executor.BatchConcurrency)
	if len(windows) < windowCount {
		var pending []translationWindow
		for _, window := range windows {
			call, err := translationCall(window, language)
			if err != nil {
				return report.DisplayTranslations{}, nil, err
			}
			cached, err := llm.RecallJSON(ctx, executor, provider, call)
			if err != nil {
				return report.DisplayTranslations{}, nil, err
			}
			if cached.Cached {
				accept(window, cached.Value)
			} else {
				pending = append(pending, window)
			}
		}
		windows = parallelWindows(pending, windowCount)
	}
	if err := translateWindows(ctx, executor, provider, windows, language, accept, keep); err != nil {
		return report.DisplayTranslations{}, nil, err
	}
	// Texts refused or missing in accepted answers are asked once more, in
	// requests of only those texts. A text refused in that follow-up answer
	// keeps its source language; only a follow-up answer that translates
	// nothing halves, as any window does, before its texts are kept.
	err = translateWindows(ctx, executor, provider, followUps, language,
		func(window translationWindow, value windowTranslation) {
			result.Entries = append(result.Entries, value.Entries...)
			refused := make(map[string]string, len(value.Refused))
			for _, entry := range value.Refused {
				refused[entry.Ref] = entry.Reason
			}
			for _, entry := range window {
				if reason, found := refused[entry.Ref]; found {
					keep(entry, reason)
				}
			}
		}, keep)
	if err != nil {
		return report.DisplayTranslations{}, nil, err
	}
	// Whole cached windows and new windows may interleave. Restore the original
	// catalogue order before validating the single complete display artifact.
	order := make(map[string]int, len(catalog.Entries))
	for i, entry := range catalog.Entries {
		order[entry.Ref] = i
	}
	sort.Slice(result.Entries, func(i, j int) bool { return order[result.Entries[i].Ref] < order[result.Entries[j].Ref] })
	sort.Slice(untranslated, func(i, j int) bool { return order[untranslated[i].Ref] < order[untranslated[j].Ref] })
	if err := result.Validate(catalog); err != nil {
		return report.DisplayTranslations{}, nil, err
	}
	return result, untranslated, nil
}

// translateWindows executes complete windows through the shared adaptive
// executor. An answer that translates nothing divides until one text
// remains. After every split only one complete text can remain in a refused
// request. Its final refusal (a missing or invalid translation, an unusable
// response, a failed provider call) is already journaled by the executor; the
// text keeps its source language instead of costing the whole report. A
// failure before any provider answer, or of a window that could still divide,
// remains the stage's error.
func translateWindows(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	windows []translationWindow,
	language report.DisplayLanguage,
	accept func(translationWindow, windowTranslation),
	keep func(report.DisplayTextEntry, string),
) error {
	if len(windows) == 0 {
		return nil
	}
	leaves, err := llm.ExecuteAdaptiveJSONEachResults(
		ctx, executor, provider, windows,
		func(window translationWindow) (llm.Call[windowTranslation], error) {
			return translationCall(window, language)
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
		return fmt.Errorf("report translation: %w", err)
	}
	for _, leaf := range leaves {
		if leaf.Err == nil {
			accept(leaf.Item, leaf.Outcome.Value)
			continue
		}
		if len(leaf.Item) != 1 || len(leaf.Outcome.ResponseRejections) == 0 {
			return fmt.Errorf("report translation: %w", leaf.Err)
		}
		keep(leaf.Item[0], leaf.Err.Error())
	}
	return nil
}

// Plan smaller requests before asking for new translations. UTF-8
// text bytes balance generation work; they are not a token estimate or a size
// cutoff. Every entry stays whole and translationCall rebuilds each child's
// complete term dictionary. Provider-envelope limits still apply separately.
func parallelWindows(windows []translationWindow, count int) []translationWindow {
	weight := func(window translationWindow) int {
		total := 0
		for _, entry := range window {
			total += max(1, len(entry.Text))
		}
		return total
	}
	for len(windows) < count {
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

// indivisibleText is a text whose request alone does not fit the provider,
// with the preparation's refusal.
type indivisibleText struct {
	entry  report.DisplayTextEntry
	reason string
}

// planWindows grows each consecutive prefix until the actual prepared request
// stops fitting, then locates the last fitting prefix. There is no row ceiling;
// exponential growth avoids preparing every intermediate catalogue prefix.
// Every accepted prefix is checked through the same language policy and
// provider preparation that execution uses. Either preparation limit, the
// request's bytes or the provider's context window, means "smaller": with the
// real DeepSeek preparation one or two texts fit where four did not, and the
// context refusal had ended the stage with none of seven translated (control
// review B3, 2026-10-03). A text that does not fit alone is indivisible and
// returned as such; each text is in exactly one window or indivisible.
func planWindows(
	ctx context.Context,
	provider llm.Provider,
	entries []report.DisplayTextEntry,
	language report.DisplayLanguage,
) ([]translationWindow, []indivisibleText, error) {
	var windows []translationWindow
	var indivisible []indivisibleText
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
			if !requestTooLarge(err) {
				return nil, nil, fmt.Errorf("report translation: prepare %s: %w", entries[start].Ref, err)
			}
			indivisible = append(indivisible, indivisibleText{entry: entries[start], reason: fmt.Sprintf("report translation: %s does not fit one request: %v", entries[start].Ref, err)})
			start++
			continue
		}
		good, bad := 1, remaining+1
		for good < remaining {
			next := remaining
			if good <= remaining/2 {
				next = good * 2
			}
			if err := fits(next); err != nil {
				if !requestTooLarge(err) {
					return nil, nil, fmt.Errorf("report translation: prepare window: %w", err)
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
					return nil, nil, fmt.Errorf("report translation: prepare window: %w", err)
				}
				bad = middle
			} else {
				good = middle
			}
		}
		windows = append(windows, translationWindow(entries[start:start+good]))
		start += good
	}
	return windows, indivisible, nil
}

// requestTooLarge says a preparation refused a request for its size: its
// bytes or the provider's context window.
func requestTooLarge(err error) bool {
	var resourceErr *llm.ResourceLimitError
	return errors.As(err, &resourceErr) && (resourceErr.Kind == llm.ResourceLimitRequestBytes || resourceErr.Kind == llm.ResourceLimitContextTokens)
}

func translationCall(
	window translationWindow,
	language report.DisplayLanguage,
) (llm.Call[windowTranslation], error) {
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
		return llm.Call[windowTranslation]{}, err
	}
	return llm.Call[windowTranslation]{
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
		DecodeValidate: func(raw []byte) (windowTranslation, error) {
			return decodeWindowTranslation(raw, window)
		},
	}, nil
}

// windowTranslation is one accepted translation answer. Entries passed their
// own validation; Refused names every other text of the window and why. The
// texts are independent: one refused text never refuses its neighbours.
type windowTranslation struct {
	Entries []report.DisplayTranslationEntry
	Refused []refusedEntry
}

type refusedEntry struct {
	Ref    string
	Reason string
}

func (value windowTranslation) ResponseRejections() []llm.ResponseRejection {
	rejections := make([]llm.ResponseRejection, 0, len(value.Refused))
	for _, refused := range value.Refused {
		rejections = append(rejections, llm.ResponseRejection{
			Kind: "translation_entry_refused", Count: 1, Samples: []string{refused.Ref}, Reason: refused.Reason,
		})
	}
	return rejections
}

// retry lists, in window order, the texts to ask again in a smaller request.
func (value windowTranslation) retry(window translationWindow) translationWindow {
	refused := make(map[string]bool, len(value.Refused))
	for _, entry := range value.Refused {
		refused[entry.Ref] = true
	}
	var texts translationWindow
	for _, entry := range window {
		if refused[entry.Ref] {
			texts = append(texts, entry)
		}
	}
	return texts
}

// decodeWindowTranslation accepts every text whose own value is a usable
// translation. A missing, malformed, twice differently answered or invalid
// text is refused alone; its neighbours stand. An answer that is not a
// readable translation shape, or that translates nothing, is refused whole.
func decodeWindowTranslation(raw []byte, window translationWindow) (windowTranslation, error) {
	known := make(map[string]bool, len(window))
	for _, entry := range window {
		known[entry.Ref] = true
	}
	candidates, err := translationCandidates(raw, known)
	if err != nil {
		return windowTranslation{}, err
	}
	var value windowTranslation
	for _, entry := range window {
		text, reason := candidateText(candidates[entry.Ref], entry.Ref)
		if reason == "" {
			if err := entry.ValidateTranslation(text); err != nil {
				reason = err.Error()
			}
		}
		if reason != "" {
			value.Refused = append(value.Refused, refusedEntry{Ref: entry.Ref, Reason: reason})
			continue
		}
		value.Entries = append(value.Entries, report.DisplayTranslationEntry{Ref: entry.Ref, Text: text})
	}
	if len(value.Entries) == 0 {
		if len(value.Refused) == 1 {
			return windowTranslation{}, errors.New(value.Refused[0].Reason)
		}
		return windowTranslation{}, fmt.Errorf("report translation: no text translated: %s", value.Refused[0].Reason)
	}
	return value, nil
}

// translationCandidates finds each window ref's raw values. The requested form
// is one object keyed by refs. One member wrapping such an object, or a list of
// {ref, text} objects (including the echoed entries shape), at the root or under
// one member, carries the same ref identity. Only refs of the window count.
func translationCandidates(raw []byte, known map[string]bool) (map[string][]json.RawMessage, error) {
	if members, err := objectMembers(raw); err == nil {
		// Every copy of a repeated key is kept: an identical repeat is one
		// answer, two different ones answer the text twice differently and
		// refuse it alone (candidateText), never the last one winning.
		if keyed := keyedCandidates(members, known); len(keyed) > 0 {
			return keyed, nil
		}
		object := make(map[string]json.RawMessage, len(members))
		for _, member := range members {
			object[member.key] = member.value
		}
		names := make([]string, 0, len(object))
		for name := range object {
			names = append(names, name)
		}
		sort.Strings(names)
		var wrapped map[string][]json.RawMessage
		for _, name := range names {
			candidates := wrappedCandidates(object[name], known)
			if len(candidates) == 0 {
				continue
			}
			if wrapped != nil {
				return nil, fmt.Errorf("report translation: several members wrap translations")
			}
			wrapped = candidates
		}
		return wrapped, nil
	}
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) == nil && list != nil {
		return listCandidates(list, known), nil
	}
	return nil, fmt.Errorf("report translation: expected a JSON object keyed by text refs")
}

func keyedCandidates(members []objectMember, known map[string]bool) map[string][]json.RawMessage {
	candidates := make(map[string][]json.RawMessage)
	for _, member := range members {
		if known[member.key] {
			candidates[member.key] = append(candidates[member.key], member.value)
		}
	}
	return candidates
}

func wrappedCandidates(raw json.RawMessage, known map[string]bool) map[string][]json.RawMessage {
	if members, err := objectMembers(raw); err == nil {
		return keyedCandidates(members, known)
	}
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) == nil {
		return listCandidates(list, known)
	}
	return nil
}

// listCandidates reads {ref, text} rows; every row naming a window ref is kept,
// so a repeated ref must agree with itself to be accepted.
func listCandidates(list []json.RawMessage, known map[string]bool) map[string][]json.RawMessage {
	candidates := make(map[string][]json.RawMessage)
	for _, item := range list {
		var row map[string]json.RawMessage
		var ref string
		if json.Unmarshal(item, &row) != nil || json.Unmarshal(row["ref"], &ref) != nil || !known[ref] {
			continue
		}
		candidates[ref] = append(candidates[ref], item)
	}
	return candidates
}

// candidateText reads one text's translation: a string, or an object whose
// text member is a string. Only text is consumed; echoed input metadata such
// as terms has no bearing on whether that text is a usable translation.
func candidateText(values []json.RawMessage, ref string) (string, string) {
	if len(values) == 0 {
		return "", fmt.Sprintf("report translation: missing translation for %s", ref)
	}
	text := ""
	for i, value := range values {
		one, reason := oneCandidateText(value, ref)
		if reason != "" {
			return "", reason
		}
		if i > 0 && one != text {
			return "", fmt.Sprintf("report translation: %s was translated twice differently", ref)
		}
		text = one
	}
	return text, ""
}

func oneCandidateText(raw json.RawMessage, ref string) (string, string) {
	trimmed := bytes.TrimSpace(raw)
	switch {
	case len(trimmed) > 0 && trimmed[0] == '"':
		var text string
		if json.Unmarshal(trimmed, &text) == nil {
			return text, ""
		}
	case len(trimmed) > 0 && trimmed[0] == '{':
		members, err := objectMembers(trimmed)
		if err != nil {
			break
		}
		// Every "text" member counts: two different ones answer the text
		// twice differently, an identical repeat is one answer.
		var texts []json.RawMessage
		for _, member := range members {
			if member.key == "text" {
				texts = append(texts, member.value)
			}
		}
		if len(texts) == 0 {
			return "", fmt.Sprintf("report translation: missing text for %s", ref)
		}
		said := ""
		for i, value := range texts {
			var text *string
			if json.Unmarshal(value, &text) != nil || text == nil {
				return "", fmt.Sprintf("report translation: text for %s must be a string", ref)
			}
			if i > 0 && *text != said {
				return "", fmt.Sprintf("report translation: %s was translated twice differently", ref)
			}
			said = *text
		}
		return said, ""
	}
	return "", fmt.Sprintf("report translation: translation for %s must be text or an object with text", ref)
}

type objectMember struct {
	key   string
	value json.RawMessage
}

// objectMembers reads one JSON object's members in order, a repeated key's
// every copy included; it fails on anything but one object.
func objectMembers(raw []byte) ([]objectMember, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if open, err := decoder.Token(); err != nil || open != json.Delim('{') {
		return nil, errors.New("not a JSON object")
	}
	var members []objectMember
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, _ := token.(string)
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		members = append(members, objectMember{key: key, value: value})
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("more than one JSON value")
	}
	return members, nil
}
