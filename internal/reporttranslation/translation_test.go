package reporttranslation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/report"
)

type testWire struct {
	Prompt llm.Prompt `json:"prompt"`
	Limits llm.Limits `json:"limits"`
}

// modelResponse is the test provider's ordered answer before responseWire
// writes it as the requested keyed object.
type modelResponse struct {
	Translations []responseEntry
}

type responseEntry struct {
	Ref  string
	Text string
}

// Test-only request reader checks the typed ordered wire and its complete texts.
type modelRequest struct {
	Terms   []requestTerm
	Entries []requestEntry
}

func (request *modelRequest) UnmarshalJSON(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var value translationRequest
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	request.Terms, request.Entries = value.Terms, value.Entries
	if request.Entries == nil {
		return fmt.Errorf("test provider: input has no ordered entry array")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("test provider: input has trailing data")
	}
	return nil
}

type testProvider struct {
	mu             sync.Mutex
	requestBytes   int
	responseRows   int
	resourceKind   llm.ResourceLimitKind
	prepareFailure error
	requests       []testWire
	respond        func(modelRequest) modelResponse
	rawResponse    []byte
	rawRespond     func(modelRequest) []byte
}

func (*testProvider) State() []byte {
	return []byte(`{"provider":"report-translation-test"}`)
}

func (provider *testProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	if provider.prepareFailure != nil {
		return llm.Prepared{}, provider.prepareFailure
	}
	wire, err := json.Marshal(testWire{Prompt: prompt, Limits: limits})
	if err != nil {
		return llm.Prepared{}, err
	}
	if provider.requestBytes > 0 && len(wire) > provider.requestBytes {
		return llm.Prepared{}, llm.NewResourceLimitError(llm.ResourceLimitError{
			Kind: llm.ResourceLimitRequestBytes, Limit: provider.requestBytes,
			Observed: len(wire), ObservedKnown: true,
		})
	}
	return llm.NewPrepared(wire)
}

func (provider *testProvider) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var wire testWire
	if err := json.Unmarshal(prepared.Bytes(), &wire); err != nil {
		return llm.Completion{}, err
	}
	var request modelRequest
	if err := json.Unmarshal([]byte(wire.Prompt.User), &request); err != nil {
		return llm.Completion{}, err
	}
	provider.mu.Lock()
	provider.requests = append(provider.requests, wire)
	provider.mu.Unlock()
	if provider.resourceKind != "" && len(request.Entries) > provider.responseRows {
		return llm.Completion{}, llm.NewResourceLimitError(llm.ResourceLimitError{
			Kind: provider.resourceKind, Limit: provider.responseRows,
			Observed: len(request.Entries), ObservedKnown: true,
		})
	}
	response := translatedResponse(request)
	if provider.respond != nil {
		response = provider.respond(request)
	}
	raw := responseWire(response)
	if provider.rawResponse != nil {
		raw = provider.rawResponse
	}
	if provider.rawRespond != nil {
		raw = provider.rawRespond(request)
	}
	return llm.Completion{
		Response: raw, FinishReason: llm.FinishStop, ChoiceCount: 1,
		Metrics: llm.Metrics{Attempts: 1},
	}, nil
}

// Write members directly so duplicate refs exercise the actual JSON wire,
// rather than disappearing in a test-only map before the decoder sees them.
func responseWire(response modelResponse) []byte {
	var raw bytes.Buffer
	raw.WriteByte('{')
	for i, entry := range response.Translations {
		if i > 0 {
			raw.WriteByte(',')
		}
		ref, _ := json.Marshal(entry.Ref)
		text, _ := json.Marshal(struct {
			Text string `json:"text"`
		}{entry.Text})
		raw.Write(ref)
		raw.WriteByte(':')
		raw.Write(text)
	}
	raw.WriteByte('}')
	return raw.Bytes()
}

func translatedResponse(request modelRequest) modelResponse {
	response := modelResponse{Translations: make([]responseEntry, len(request.Entries))}
	for i, entry := range request.Entries {
		response.Translations[i] = responseEntry{Ref: entry.Ref, Text: "Перевод: " + entry.Text}
	}
	return response
}

func testCatalog(t *testing.T, entries []report.DisplayTextEntry) report.DisplayTextCatalog {
	t.Helper()
	if entries == nil {
		entries = []report.DisplayTextEntry{}
	}
	for i := range entries {
		entries[i].Ref = fmt.Sprintf("t%d", i+1)
	}
	wire, err := json.Marshal(struct {
		Version int                       `json:"version"`
		Entries []report.DisplayTextEntry `json:"entries"`
	}{report.DisplayTextVersion, entries})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(wire)
	catalog := report.DisplayTextCatalog{
		Version: report.DisplayTextVersion, SHA256: hex.EncodeToString(digest[:]), Entries: entries,
	}
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	return catalog
}

func plainEntries(count int) []report.DisplayTextEntry {
	entries := make([]report.DisplayTextEntry, count)
	for i := range entries {
		entries[i] = report.DisplayTextEntry{Role: "answer.explanation", Text: "Quoted \"value\" & <tag> remains qualified.\n\nSecond paragraph: 日本語."}
	}
	return entries
}

func TestTranslatePreservesCatalogAndUsesSharedCache(t *testing.T) {
	// A serial caller may already have cached the whole 1,310-text catalogue.
	// Enabling parallel execution must reuse that complete validated answer.
	entries := plainEntries(1310)
	entries[0].Text = "__REPOMAP_P1__ may call __REPOMAP_P1__.\n\nOnly if possible."
	entries[0].Protected = []report.DisplayProtectedText{{Ref: "__REPOMAP_P1__", Text: "SOURCE-ORIGINAL-NEVER-SENT"}}
	catalog := testCatalog(t, entries)
	provider := &testProvider{respond: func(request modelRequest) modelResponse {
		response := translatedResponse(request)
		for left, right := 0, len(response.Translations)-1; left < right; left, right = left+1, right-1 {
			response.Translations[left], response.Translations[right] = response.Translations[right], response.Translations[left]
		}
		response.Translations = append(response.Translations,
			response.Translations[0], // A repeated object key keeps its last value.
			responseEntry{Ref: "t999999", Text: "__REPOMAP_P999__"},
		)
		return response
	}}
	var events []llm.Event
	executor := llm.Executor{
		Enabled: true, RootDir: t.TempDir(), BatchConcurrency: 1,
		Observer: llm.ObserverFunc(func(event llm.Event) error { events = append(events, event); return nil }),
	}
	whole, err := translationCall(catalog.Entries, report.Russian)
	if err != nil {
		t.Fatal(err)
	}
	seeded, err := llm.ExecuteJSON(t.Context(), executor, provider, whole)
	if err != nil {
		t.Fatal(err)
	}
	result := report.DisplayTranslations{
		Version: report.DisplayTextVersion, Language: report.Russian,
		CatalogSHA256: catalog.SHA256, Entries: seeded.Value.Entries,
	}
	if len(provider.requests) != 1 {
		t.Fatalf("requests = %d, want all %d complete entries in one request", len(provider.requests), len(entries))
	}
	if err := result.Validate(catalog); err != nil {
		t.Fatal(err)
	}
	for i, entry := range result.Entries {
		if entry.Ref != catalog.Entries[i].Ref || entry.Text != "Перевод: "+catalog.Entries[i].Text {
			t.Fatalf("translated entry %d lost order or original prose: %#v", i, entry)
		}
	}
	wire := provider.requests[0]
	if wire.Prompt.Reasoning || !wire.Prompt.ResponseFormatJSON || wire.Prompt.ResponseLanguage != "ru" ||
		!strings.Contains(wire.Prompt.System, `"ru"`) {
		t.Fatalf("provider prompt did not receive shared target-language policy: %#v", wire.Prompt)
	}
	if wire.Limits != (llm.Limits{
		MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit,
		MaxOutputTokens: llm.DefaultMaxOutputTokens, AttemptTimeout: attemptTimeout,
	}) {
		t.Fatalf("translation introduced a smaller request allowance: %#v", wire.Limits)
	}
	if strings.Contains(wire.Prompt.User, "SOURCE-ORIGINAL-NEVER-SENT") || strings.Contains(wire.Prompt.User, catalog.SHA256) {
		t.Fatal("provider input contains locally restored source or catalogue identity")
	}
	var request modelRequest
	if err := json.Unmarshal([]byte(wire.Prompt.User), &request); err != nil {
		t.Fatal(err)
	}
	if len(request.Entries) != len(entries) {
		t.Fatal("typed provider input lost complete entries")
	}
	for i, entry := range request.Entries {
		if entry.Ref != catalog.Entries[i].Ref || entry.Text != catalog.Entries[i].Text {
			t.Fatalf("typed input reordered a ref or changed original text at %d: %#v", i, entry)
		}
	}
	// Original protected source bytes are restored locally. They do not change
	// identical translation input or authorize copying an old catalogue binding.
	entries[0].Protected[0].Text = "DIFFERENT-SOURCE-ORIGINAL"
	rebound := testCatalog(t, entries)
	executor.BatchConcurrency = 4
	cached, _, err := Translate(t.Context(), executor, provider, rebound, report.Russian)
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 1 || cached.CatalogSHA256 != rebound.SHA256 ||
		cached.CatalogSHA256 == result.CatalogSHA256 || !reflect.DeepEqual(cached.Entries, result.Entries) {
		t.Fatal("exact cached translation was not rebound to current protected source")
	}
	if len(events) != 2 || events[0].Kind != llm.EventLive || events[1].Kind != llm.EventCacheHit {
		t.Fatalf("translation bypassed shared cache/observer: %#v", events)
	}
	executor.BatchConcurrency = 1
	if _, _, err := Translate(t.Context(), executor, provider, rebound, report.English); err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 9 || provider.requests[1].Prompt.ResponseLanguage != "en" {
		t.Fatal("different translation language reused the old exact request")
	}
}

// placeholderEntries holds sixteen texts of equal weight, so the initial plan
// is eight windows of two: t1 (with a source placeholder) beside t2, and so on.
func placeholderEntries() []report.DisplayTextEntry {
	entries := plainEntries(16)
	entries[0].Text = strings.Replace(entries[0].Text, "value", "__REPOMAP_P1__", 1)
	entries[0].Protected = []report.DisplayProtectedText{{Ref: "__REPOMAP_P1__", Text: "code"}}
	return entries
}

func requestSizes(t *testing.T, wires []testWire) map[int]int {
	t.Helper()
	sizes := make(map[int]int)
	for _, wire := range wires {
		var request modelRequest
		if err := json.Unmarshal([]byte(wire.Prompt.User), &request); err != nil {
			t.Fatal(err)
		}
		sizes[len(request.Entries)]++
	}
	return sizes
}

// One refused text does not refuse its window: its neighbour is published at
// once, with no halving, and only the refused text is asked again, alone. A
// text refused again keeps its source language, named with its refusal.
func TestTranslateRejectsIncompleteOrChangedPlaceholders(t *testing.T) {
	catalog := testCatalog(t, placeholderEntries())
	for _, test := range []struct {
		name string
		kept string
		edit func(*modelResponse)
	}{
		{"missing", "t2", func(response *modelResponse) {
			for i, entry := range response.Translations {
				if entry.Ref == "t2" {
					response.Translations = append(response.Translations[:i], response.Translations[i+1:]...)
					break
				}
			}
		}},
		{"blank", "t2", editTranslationRef("t2", func(entry *responseEntry) { entry.Text = " \n " })},
		{"removed placeholder", "t1", editTranslationRef("t1", func(entry *responseEntry) { entry.Text = "Translated text" })},
		{"invented placeholder", "t2", editTranslationRef("t2", func(entry *responseEntry) { entry.Text += " __REPOMAP_P2__" })},
		{"unknown cannot replace known", "t2", editTranslationRef("t2", func(entry *responseEntry) { entry.Ref = "t999" })},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := &testProvider{respond: func(request modelRequest) modelResponse {
				response := translatedResponse(request)
				test.edit(&response)
				return response
			}}
			var events []llm.Event
			executor := llm.Executor{Enabled: true, RootDir: t.TempDir(), Observer: llm.ObserverFunc(func(event llm.Event) error {
				events = append(events, event)
				return nil
			})}
			// The refused text is neither repaired nor published as a translation:
			// it keeps its source language, named with its refusal, while its
			// accepted neighbour and the complete artifact survive.
			result, untranslated, err := Translate(t.Context(), executor, provider, catalog, report.Russian)
			if err != nil || result.Validate(catalog) != nil {
				t.Fatalf("a refused text failed the whole translation: %#v, %v", result, err)
			}
			if len(untranslated) != 1 || untranslated[0].Ref != test.kept || untranslated[0].Reason == "" {
				t.Fatalf("kept text was not named with its refusal: %+v", untranslated)
			}
			for i, entry := range result.Entries {
				want := "Перевод: " + catalog.Entries[i].Text
				if entry.Ref == test.kept {
					want = catalog.Entries[i].Text
				}
				if entry.Ref != catalog.Entries[i].Ref || entry.Text != want {
					t.Fatalf("refused text was repaired or its neighbour was lost: %#v", entry)
				}
			}
			// Eight windows of two, then one request of only the refused text.
			if sizes := requestSizes(t, provider.requests); len(provider.requests) != 9 || sizes[2] != 8 || sizes[1] != 1 {
				t.Fatalf("the refused text halved its window or was not asked again alone: %v", sizes)
			}
			var retry modelRequest
			if err := json.Unmarshal([]byte(provider.requests[8].Prompt.User), &retry); err != nil || retry.Entries[0].Ref != test.kept {
				t.Fatalf("the follow-up asked for another text: %+v %v", retry, err)
			}
			refused := 0
			for _, event := range events {
				if event.Failure == llm.FailureValidation {
					refused++
					if _, found, err := llm.CachedExchange(executor.RootDir, event.CacheKey); err != nil || found {
						t.Fatalf("refused translation entered accepted cache: %v", err)
					}
				}
			}
			if refused != 1 {
				t.Fatalf("expected only the refused follow-up, got %d validation failures", refused)
			}
			provider.respond = nil
			before := len(provider.requests)
			if _, kept, err := Translate(t.Context(), executor, provider, catalog, report.Russian); err != nil || len(kept) != 0 {
				t.Fatalf("a later valid answer did not translate the kept text: %+v, %v", kept, err)
			}
			if sizes := requestSizes(t, provider.requests[before:]); len(provider.requests) != before+1 || sizes[1] != 1 {
				t.Fatal("only the refused text should be requested again")
			}
		})
	}
}

// A refused follow-up is the only second request: when the smaller request
// translates the text, nothing is kept in the source language.
func TestTranslateReasksARefusedTextOnceInASmallerRequest(t *testing.T) {
	catalog := testCatalog(t, placeholderEntries())
	provider := &testProvider{respond: func(request modelRequest) modelResponse {
		response := translatedResponse(request)
		if len(request.Entries) > 1 {
			editTranslationRef("t1", func(entry *responseEntry) { entry.Text = "Translated text" })(&response)
		}
		return response
	}}
	result, untranslated, err := Translate(t.Context(), llm.Executor{Enabled: true, RootDir: t.TempDir()}, provider, catalog, report.Russian)
	if err != nil || result.Validate(catalog) != nil || len(untranslated) != 0 {
		t.Fatalf("the smaller request did not translate the refused text: %+v %v", untranslated, err)
	}
	for i, entry := range result.Entries {
		if entry.Text != "Перевод: "+catalog.Entries[i].Text {
			t.Fatalf("entry %d was not translated: %#v", i, entry)
		}
	}
	if sizes := requestSizes(t, provider.requests); len(provider.requests) != 9 || sizes[2] != 8 || sizes[1] != 1 {
		t.Fatalf("requests: %v", sizes)
	}
}

// An answer that translates nothing, such as a bare number, still halves its
// window; an entry introducing a placeholder it does not own is still refused.
func TestTranslateUnreadableAnswerHalvesAndForeignPlaceholderStaysRefused(t *testing.T) {
	catalog := testCatalog(t, placeholderEntries())
	provider := &testProvider{rawRespond: func(request modelRequest) []byte {
		if len(request.Entries) > 1 {
			return []byte(`42`)
		}
		return responseWire(translatedResponse(request))
	}}
	result, untranslated, err := Translate(t.Context(), llm.Executor{Enabled: true, RootDir: t.TempDir()}, provider, catalog, report.Russian)
	if err != nil || result.Validate(catalog) != nil || len(untranslated) != 0 {
		t.Fatalf("halved children did not translate the catalogue: %+v %v", untranslated, err)
	}
	if sizes := requestSizes(t, provider.requests); len(provider.requests) != 24 || sizes[2] != 8 || sizes[1] != 16 {
		t.Fatalf("an unreadable answer did not halve its window: %v", sizes)
	}
	provider = &testProvider{respond: func(request modelRequest) modelResponse {
		response := translatedResponse(request)
		editTranslationRef("t2", func(entry *responseEntry) { entry.Text += " __REPOMAP_P1__" })(&response)
		return response
	}}
	result, untranslated, err = Translate(t.Context(), llm.Executor{Enabled: true, RootDir: t.TempDir()}, provider, catalog, report.Russian)
	if err != nil || result.Validate(catalog) != nil || len(untranslated) != 1 || untranslated[0].Ref != "t2" ||
		!strings.Contains(untranslated[0].Reason, "introduced a source placeholder") || result.Entries[1].Text != catalog.Entries[1].Text {
		t.Fatalf("a placeholder owned by another text was accepted: %+v %v", untranslated, err)
	}
}

func editTranslationRef(ref string, edit func(*responseEntry)) func(*modelResponse) {
	return func(response *modelResponse) {
		for i := range response.Translations {
			if response.Translations[i].Ref == ref {
				edit(&response.Translations[i])
			}
		}
	}
}

func TestTranslateKeyedWirePreservesOnlyOriginalPlaceholderAuthority(t *testing.T) {
	catalog := testCatalog(t, []report.DisplayTextEntry{
		{Role: "answer", Text: "Use __REPOMAP_P1__ if possible.", Protected: []report.DisplayProtectedText{{Ref: "__REPOMAP_P1__", Text: "source.go"}}},
		{Role: "label", Text: "Start here"},
	})
	provider := &testProvider{rawResponse: []byte(`{
		"t2":{"text":"Начало"}, "t1":{"text":"Используйте __REPOMAP_P1__ и затем __REPOMAP_P1__ по возможности."},
		"t\u0031":{"text":"Используйте __REPOMAP_P1__ и затем __REPOMAP_P1__ по возможности."},
		"t999":{"protected":["__REPOMAP_P999__"],"text":null}, "unknown":null
	}`)}
	result, _, err := Translate(t.Context(), llm.Executor{}, provider, catalog, report.Russian)
	if err != nil || len(result.Entries) != 2 || result.Entries[0].Text != "Используйте __REPOMAP_P1__ и затем __REPOMAP_P1__ по возможности." || result.Entries[1].Text != "Начало" {
		t.Fatalf("equal parsed duplicates or unknown refs changed the known translations: %#v %v", result, err)
	}
	raw, err := json.Marshal(result)
	if err != nil || strings.Contains(string(raw), `"protected"`) || strings.Contains(string(raw), "P999") {
		t.Fatal("unknown value acquired display or placeholder authority")
	}
	provider.rawResponse = []byte(`{"t1":{"text":"Без обязательного плейсхолдера."},"t2":{"text":"Начало"},"protected":["__REPOMAP_P1__"]}`)
	var untranslated []Untranslated
	result, untranslated, err = Translate(t.Context(), llm.Executor{}, provider, catalog, report.Russian)
	if err != nil || len(untranslated) != 1 || untranslated[0].Ref != "t1" || len(result.Entries) != 2 ||
		result.Entries[0].Text != "Use __REPOMAP_P1__ if possible." || result.Entries[1].Text != "Начало" {
		t.Fatalf("unadvertised output metadata authorized a missing source placeholder: %#v %+v %v", result, untranslated, err)
	}
}

// Each text's value is judged on its own. The requested keyed object, a bare
// string, one wrapper member and a {ref, text} list (including the echoed
// entries shape) carry the same ref identity; a wrong value refuses only its
// own text, and an answer with no usable text is refused whole.
func TestTranslateRejectsInvalidKeyedWireBeforeCache(t *testing.T) {
	catalog := testCatalog(t, plainEntries(2))
	for _, test := range []struct {
		name, raw string
		kept      []string
	}{
		{"keyed text objects", `{"t1":{"text":"one"},"t2":{"text":"two"}}`, nil},
		{"bare strings", `{"t1":"one","t2":"two"}`, nil},
		{"wrapped list", `{"translations":[{"ref":"t1","text":"one"},{"ref":"t2","text":"two"}]}`, nil},
		{"wrapped keyed object", `{"translations":{"t1":"one","t2":"two"}}`, nil},
		{"echoed entries", `{"entries":[{"ref":"t1","role":"answer.explanation","text":"one"},{"ref":"t2","role":"answer.explanation","text":"two"}]}`, nil},
		{"root list", `[{"ref":"t1","text":"one"},{"ref":"t2","text":"two"}]`, nil},
		{"identical list repeat", `[{"ref":"t1","text":"one"},{"ref":"t1","text":"one"},{"ref":"t2","text":"two"}]`, nil},
		{"different list repeat", `[{"ref":"t1","text":"one"},{"ref":"t1","text":"uno"},{"ref":"t2","text":"two"}]`, []string{"t1"}},
		{"known null", `{"t1":null,"t2":"two"}`, []string{"t1"}},
		{"known number", `{"t1":1,"t2":"two"}`, []string{"t1"}},
		{"known bool", `{"t1":true,"t2":"two"}`, []string{"t1"}},
		{"known array", `{"t1":["one"],"t2":"two"}`, []string{"t1"}},
		{"text not a string", `{"t1":{"text":7},"t2":"two"}`, []string{"t1"}},
		{"missing text", `{"t1":{"translation":"one"},"t2":"two"}`, []string{"t1"}},
		{"missing mandatory ref", `{"t1":{"text":"one"},"t999":{"text":"two"}}`, []string{"t2"}},
		{"last parsed duplicate is invalid", `{"t1":{"text":"one"},"t2":{"text":"two"},"t1":null}`, []string{"t1"}},
		{"root array of keyed objects", `[{"t1":"one","t2":"two"}]`, []string{"t1", "t2"}},
		{"root null", `null`, []string{"t1", "t2"}},
		{"root number", `42`, []string{"t1", "t2"}},
		{"missing object", `{}`, []string{"t1", "t2"}},
		{"two wrappers", `{"a":{"t1":"one","t2":"two"},"b":{"t1":"uno","t2":"dos"}}`, []string{"t1", "t2"}},
		{"unclosed ref quote", `{"t1":"one","t2:"two"}`, []string{"t1", "t2"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			kept := make(map[string]bool)
			for _, ref := range test.kept {
				kept[ref] = true
			}
			// One window of both texts: the refused text alone is named.
			call, err := translationCall(catalog.Entries, report.Russian)
			if err != nil {
				t.Fatal(err)
			}
			value, decodeErr := call.DecodeValidate([]byte(test.raw))
			if len(test.kept) == len(catalog.Entries) {
				if decodeErr == nil {
					t.Fatalf("an answer with no usable text was accepted: %+v", value)
				}
			} else if decodeErr != nil || len(value.Entries)+len(value.Refused) != 2 || len(value.Refused) != len(test.kept) {
				t.Fatalf("window value: %+v %v", value, decodeErr)
			} else {
				for _, refused := range value.Refused {
					if !kept[refused.Ref] || refused.Reason == "" {
						t.Fatalf("the wrong text was refused: %+v", value.Refused)
					}
				}
			}

			provider := &testProvider{rawResponse: []byte(test.raw)}
			executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
			result, untranslated, err := Translate(t.Context(), executor, provider, catalog, report.Russian)
			if err != nil || result.Validate(catalog) != nil || len(untranslated) != len(test.kept) {
				t.Fatalf("keyed wire was repaired, over-refused or failed the translation: %#v %+v %v", result, untranslated, err)
			}
			for i, entry := range result.Entries {
				want := []string{"one", "two"}[i]
				if kept[entry.Ref] {
					want = catalog.Entries[i].Text
				}
				if entry.Text != want {
					t.Fatalf("kept and translated texts disagree with the refusal list: %#v %+v", entry, untranslated)
				}
			}
			before := len(provider.requests)
			provider.rawResponse = nil
			if _, again, err := Translate(t.Context(), executor, provider, catalog, report.Russian); err != nil || len(again) != 0 {
				t.Fatalf("valid answers did not replace the kept texts: %+v %v", again, err)
			}
			if len(provider.requests) != before+len(untranslated) {
				t.Fatal("a refused answer was cached or an accepted text was asked again")
			}
		})
	}
}

func TestTranslateIgnoresExtraEntryMetadataWithoutRepeatingTheRequest(t *testing.T) {
	catalog := testCatalog(t, []report.DisplayTextEntry{termEntry()})
	raw := []byte(`{"t1":{"text":"Загрузите custom dictionary через __REPOMAP_P1__.","terms":["d1"],"mentions":{},"protected":["__REPOMAP_P999__"]}}`)
	provider := &testProvider{rawResponse: raw}
	var events []llm.Event
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir(), Observer: llm.ObserverFunc(func(event llm.Event) error {
		events = append(events, event)
		return nil
	})}
	for run := 0; run < 2; run++ {
		result, _, err := Translate(t.Context(), executor, provider, catalog, report.Russian)
		if err != nil || result.Validate(catalog) != nil || len(result.Entries) != 1 || result.Entries[0].Text != "Загрузите custom dictionary через __REPOMAP_P1__." {
			t.Fatalf("extra metadata changed valid translation on run %d: %+v, %v", run, result, err)
		}
	}
	if len(provider.requests) != 1 || len(events) != 2 || events[0].Kind != llm.EventLive || events[1].Kind != llm.EventCacheHit {
		t.Fatalf("extra terms caused repeated translation: requests=%d events=%d", len(provider.requests), len(events))
	}
	for _, event := range events {
		if event.Failure != "" || !bytes.Equal(event.Response, raw) {
			t.Fatal("extra metadata rejected or changed the original cached response")
		}
	}
}

func TestTranslatePacksByPreparedProviderEnvelope(t *testing.T) {
	entries := plainEntries(7)
	for i := range entries {
		entries[i].Terms = termEntry().Terms
	}
	catalog := testCatalog(t, entries)
	provider := &testProvider{}
	call, err := translationCall(catalog.Entries[:3], report.Russian)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := llm.Prepare(provider, call.Prompt, call.Limits)
	if err != nil {
		t.Fatal(err)
	}
	provider.requestBytes = prepared.Len()
	windows, err := planWindows(t.Context(), provider, catalog.Entries, report.Russian)
	if err != nil {
		t.Fatal(err)
	}
	if len(windows) != 3 || len(windows[0]) != 3 || len(windows[1]) != 3 || len(windows[2]) != 1 {
		t.Fatalf("provider envelope did not pack complete entries as 3+3+1: %v", windows)
	}
	result, _, err := Translate(t.Context(), llm.Executor{}, provider, catalog, report.Russian)
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 7 {
		t.Fatalf("windows = %d, want seven initial singleton partitions", len(provider.requests))
	}
	var refs []string
	for i, wire := range provider.requests {
		var request modelRequest
		if err := json.Unmarshal([]byte(wire.Prompt.User), &request); err != nil {
			t.Fatal(err)
		}
		assertRequestTerms(t, request, catalog.Entries)
		if len(request.Entries) != 1 {
			t.Fatalf("window %d contains %d entries, want one", i, len(request.Entries))
		}
		for _, entry := range request.Entries {
			refs = append(refs, entry.Ref)
		}
		encoded, err := json.Marshal(wire)
		if err != nil || len(encoded) > provider.requestBytes {
			t.Fatalf("executed request exceeds actual provider envelope: %d, %v", len(encoded), err)
		}
	}
	if !reflect.DeepEqual(refs, []string{"t1", "t2", "t3", "t4", "t5", "t6", "t7"}) {
		t.Fatalf("window refs lost source identity or complete coverage: %#v", refs)
	}
	if err := result.Validate(catalog); err != nil {
		t.Fatal(err)
	}

	// A text that remains absent at singleton scope is not repaired from its
	// neighbours: it keeps its source language, named with the refusal, and
	// the accepted texts are published beside it.
	provider.requests = nil
	provider.respond = func(request modelRequest) modelResponse {
		response := translatedResponse(request)
		for i, entry := range response.Translations {
			if entry.Ref == "t5" {
				response.Translations = append(response.Translations[:i], response.Translations[i+1:]...)
				break
			}
		}
		return response
	}
	kept, untranslated, err := Translate(t.Context(), llm.Executor{Enabled: true, RootDir: t.TempDir()}, provider, catalog, report.Russian)
	if err != nil || kept.Validate(catalog) != nil || len(untranslated) != 1 || untranslated[0].Ref != "t5" || !strings.Contains(untranslated[0].Reason, "missing translation for t5") {
		t.Fatalf("a refused singleton was not kept in the source language: %#v, %+v, %v", kept, untranslated, err)
	}
	if kept.Entries[4].Text != catalog.Entries[4].Text || kept.Entries[3].Text != "Перевод: "+catalog.Entries[3].Text {
		t.Fatalf("kept text or its neighbour changed: %#v", kept.Entries)
	}
	provider.respond = nil
	provider.requests = nil
	provider.requestBytes = 1
	result, _, err = Translate(t.Context(), llm.Executor{}, provider, catalog, report.Russian)
	var resourceErr *llm.ResourceLimitError
	if !errors.As(err, &resourceErr) || resourceErr.Kind != llm.ResourceLimitRequestBytes ||
		len(provider.requests) != 0 || !reflect.DeepEqual(result, report.DisplayTranslations{}) {
		t.Fatalf("oversized complete entry was not a preparation failure: %#v, %v", result, err)
	}
}

func TestTranslateSplitsRealResponseResourcesWithoutPartialPublication(t *testing.T) {
	entries := plainEntries(32)
	for i := range entries {
		entries[i].Terms = []report.DisplayTextTerm{{ID: fmt.Sprintf("local-%d", i), Spelling: "bank", Explanation: fmt.Sprintf("Definition %d.", i%3)}}
	}
	catalog := testCatalog(t, entries)
	for _, kind := range []llm.ResourceLimitKind{llm.ResourceLimitResponseBytes, llm.ResourceLimitOutputTokens} {
		t.Run(string(kind), func(t *testing.T) {
			provider := &testProvider{resourceKind: kind, responseRows: 2}
			executor := llm.Executor{Enabled: true, RootDir: t.TempDir(), BatchConcurrency: 1}
			result, _, err := Translate(t.Context(), executor, provider, catalog, report.Russian)
			if err != nil {
				t.Fatal(err)
			}
			if err := result.Validate(catalog); err != nil {
				t.Fatal(err)
			}
			for i, entry := range result.Entries {
				if entry.Ref != catalog.Entries[i].Ref || entry.Text != "Перевод: "+catalog.Entries[i].Text {
					t.Fatalf("adaptive window lost complete source identity at %d: %#v", i, entry)
				}
			}
			if len(provider.requests) <= 16 {
				t.Fatal("test did not exercise real failed resource envelopes and adaptive splits")
			}
			for _, wire := range provider.requests {
				var request modelRequest
				if err := json.Unmarshal([]byte(wire.Prompt.User), &request); err != nil {
					t.Fatal(err)
				}
				assertRequestTerms(t, request, catalog.Entries)
			}
			// One text the provider cannot answer within its resources is not
			// retried or repaired: it keeps its source language with that refusal.
			atomicProvider := &testProvider{resourceKind: kind}
			atomicCatalog := testCatalog(t, plainEntries(1))
			kept, untranslated, err := Translate(t.Context(), llm.Executor{}, atomicProvider, atomicCatalog, report.Russian)
			if err != nil || len(atomicProvider.requests) != 1 || kept.Validate(atomicCatalog) != nil || kept.Entries[0].Text != atomicCatalog.Entries[0].Text ||
				len(untranslated) != 1 || untranslated[0].Ref != "t1" || !strings.Contains(untranslated[0].Reason, "resource="+string(kind)) {
				t.Fatalf("atomic resource failure was repaired, retried or failed the translation: %#v, %+v, %v", kept, untranslated, err)
			}
		})
	}
}

// The owner's run ended after every stage had completed: three
// "missing translation for t38" refusals, the last on a one-text request. A
// text refused on its own now keeps its source language; the rest is published.
func TestTranslateKeepsARefusedSingletonInTheSourceLanguage(t *testing.T) {
	catalog := testCatalog(t, plainEntries(32))
	withoutT5 := func(request modelRequest) modelResponse {
		response := translatedResponse(request)
		for i, entry := range response.Translations {
			if entry.Ref == "t5" {
				response.Translations = append(response.Translations[:i], response.Translations[i+1:]...)
				break
			}
		}
		return response
	}
	provider := &testProvider{respond: withoutT5}
	var events []llm.Event
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir(), BatchConcurrency: 1,
		Observer: llm.ObserverFunc(func(event llm.Event) error { events = append(events, event); return nil })}
	result, untranslated, err := Translate(t.Context(), executor, provider, catalog, report.Russian)
	if err != nil || result.Validate(catalog) != nil {
		t.Fatalf("one text without a translation failed the complete catalogue: %v", err)
	}
	if len(untranslated) != 1 || untranslated[0].Ref != "t5" || !strings.Contains(untranslated[0].Reason, "missing translation for t5") {
		t.Fatalf("kept text was not named with its refusal: %+v", untranslated)
	}
	for i, entry := range result.Entries {
		want := "Перевод: " + catalog.Entries[i].Text
		if entry.Ref == "t5" {
			want = catalog.Entries[i].Text
		}
		if entry.Ref != catalog.Entries[i].Ref || entry.Text != want {
			t.Fatalf("entry %d was repaired, lost or reordered: %#v", i, entry)
		}
	}
	// Eight windows of four. The window holding t5 is accepted for its other
	// three texts, with no halving; t5 alone is asked once more and refused.
	refused := 0
	for _, event := range events {
		if event.Failure == llm.FailureValidation {
			refused++
			if _, found, err := llm.CachedExchange(executor.RootDir, event.CacheKey); err != nil || found {
				t.Fatalf("refused window entered accepted cache: %v", err)
			}
		}
	}
	if len(provider.requests) != 9 || refused != 1 {
		t.Fatalf("requests=%d refused=%d; want eight windows and one follow-up of t5", len(provider.requests), refused)
	}
	if sizes := requestSizes(t, provider.requests); sizes[4] != 8 || sizes[1] != 1 {
		t.Fatalf("window sizes changed: %v", sizes)
	}
	// A warm run reuses the accepted windows; only the refused text is asked
	// again, and it is kept again without an error.
	warmProvider := &testProvider{respond: withoutT5}
	warm, warmUntranslated, err := Translate(t.Context(), executor, warmProvider, catalog, report.Russian)
	if err != nil || !reflect.DeepEqual(warm, result) || !reflect.DeepEqual(warmUntranslated, untranslated) || len(warmProvider.requests) != 1 {
		t.Fatalf("warm run changed the kept text or repeated accepted windows: requests=%d, %v", len(warmProvider.requests), err)
	}
}

func TestTranslateRejectedWindowHalvesAndWarmRunReusesOnlyCompleteChildren(t *testing.T) {
	entries := plainEntries(32)
	catalog := testCatalog(t, entries)
	bad := []byte(`{"t1":{"text":"Do not salvage this prefix"},"t2,"}`)
	provider := &testProvider{rawRespond: func(request modelRequest) []byte {
		if len(request.Entries) > 2 {
			return bad
		}
		return responseWire(translatedResponse(request))
	}}
	var events []llm.Event
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir(), BatchConcurrency: 1,
		Observer: llm.ObserverFunc(func(event llm.Event) error { events = append(events, event); return nil })}
	for run := 0; run < 2; run++ {
		result, _, err := Translate(t.Context(), executor, provider, catalog, report.Russian)
		if err != nil || result.Validate(catalog) != nil {
			t.Fatalf("run %d did not produce the complete validated catalogue: %v", run, err)
		}
		for i, entry := range result.Entries {
			if entry.Ref != catalog.Entries[i].Ref || entry.Text != "Перевод: "+catalog.Entries[i].Text {
				t.Fatalf("refused parent supplied a value or a child lost identity: %+v", entry)
			}
		}
		if len(provider.requests) != 24 {
			t.Fatalf("run %d used %d calls; want eight refused windows and sixteen accepted halves in total", run, len(provider.requests))
		}
	}
	refused, warm := 0, 0
	for _, event := range events {
		if event.Kind == llm.EventCacheHit {
			warm++
		}
		if event.Failure == llm.FailureValidation {
			refused++
			if !bytes.Equal(event.Response, bad) {
				t.Fatal("validation failure lost the original refused response")
			}
			if _, found, err := llm.CachedExchange(executor.RootDir, event.CacheKey); err != nil || found {
				t.Fatalf("refused parent became an accepted cache entry: %v", err)
			}
		}
	}
	if refused != 8 || warm < 16 {
		t.Fatalf("raw refusals or warm child cache provenance changed: refused=%d cached=%d", refused, warm)
	}
}

func TestTranslateValidatesBeforeProviderAndAllowsEmptyCatalog(t *testing.T) {
	empty := testCatalog(t, nil)
	result, _, err := Translate(t.Context(), llm.Executor{}, nil, empty, report.Russian)
	if err != nil || result.Entries == nil || len(result.Entries) != 0 || result.Validate(empty) != nil {
		t.Fatalf("empty catalogue should need no provider: %#v, %v", result, err)
	}
	provider := &testProvider{}
	catalog := testCatalog(t, plainEntries(1))
	badCatalog := catalog
	badCatalog.SHA256 = "wrong"
	if _, _, err := Translate(t.Context(), llm.Executor{}, provider, badCatalog, report.Russian); err == nil {
		t.Fatal("invalid catalogue was accepted")
	}
	if _, _, err := Translate(t.Context(), llm.Executor{}, provider, catalog, "other"); err == nil {
		t.Fatal("unsupported display language was accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := Translate(ctx, llm.Executor{}, provider, catalog, report.Russian); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled translation = %v", err)
	}
	if _, _, err := Translate(t.Context(), llm.Executor{}, nil, catalog, report.Russian); err == nil {
		t.Fatal("nonempty catalogue accepted a missing provider")
	}
	provider.prepareFailure = errors.New("provider configuration failure")
	if _, _, err := Translate(t.Context(), llm.Executor{}, provider, catalog, report.Russian); !errors.Is(err, provider.prepareFailure) {
		t.Fatalf("non-resource prepare error was split or lost: %v", err)
	}
	if len(provider.requests) != 0 {
		t.Fatal("invalid input reached the provider")
	}
}
