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

// Test-only request reader retains member order so the fake provider can
// verify complete catalogue packing without a production input decoder.
type modelRequest struct {
	Entries []responseEntry
}

func (request *modelRequest) UnmarshalJSON(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return fmt.Errorf("test provider: input is not a flat object")
	}
	request.Entries = nil
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		ref, ok := token.(string)
		if !ok {
			return fmt.Errorf("test provider: input key is not a string")
		}
		var text string
		if err := decoder.Decode(&text); err != nil {
			return err
		}
		request.Entries = append(request.Entries, responseEntry{Ref: ref, Text: text})
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return fmt.Errorf("test provider: input object is incomplete")
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
		text, _ := json.Marshal(entry.Text)
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
	entries := plainEntries(600)
	entries[0].Text = "__REPOMAP_P1__ may call __REPOMAP_P1__.\n\nOnly if possible."
	entries[0].Protected = []report.DisplayProtectedText{{Ref: "__REPOMAP_P1__", Text: "SOURCE-ORIGINAL-NEVER-SENT"}}
	catalog := testCatalog(t, entries)
	provider := &testProvider{respond: func(request modelRequest) modelResponse {
		response := translatedResponse(request)
		for left, right := 0, len(response.Translations)-1; left < right; left, right = left+1, right-1 {
			response.Translations[left], response.Translations[right] = response.Translations[right], response.Translations[left]
		}
		response.Translations = append(response.Translations,
			response.Translations[0], // Exact duplicates are one set member.
			responseEntry{Ref: "t999", Text: "__REPOMAP_P999__"},
		)
		return response
	}}
	var events []llm.Event
	executor := llm.Executor{
		Enabled: true, RootDir: t.TempDir(), BatchConcurrency: 4,
		Observer: llm.ObserverFunc(func(event llm.Event) error { events = append(events, event); return nil }),
	}
	result, err := Translate(t.Context(), executor, provider, catalog, report.Russian)
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 1 {
		t.Fatalf("requests = %d, want all 600 complete entries in one request", len(provider.requests))
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
		MaxOutputTokens: llm.DefaultMaxOutputTokens,
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
		t.Fatal("flat provider input lost complete entries")
	}
	for i, entry := range request.Entries {
		if entry.Ref != catalog.Entries[i].Ref || entry.Text != catalog.Entries[i].Text {
			t.Fatalf("flat input reordered a ref or changed original text at %d: %#v", i, entry)
		}
	}
	// Original protected source bytes are restored locally. They do not change
	// identical translation input or authorize copying an old catalogue binding.
	entries[0].Protected[0].Text = "DIFFERENT-SOURCE-ORIGINAL"
	entries[0].Role = "different-local-display-role"
	rebound := testCatalog(t, entries)
	cached, err := Translate(t.Context(), executor, provider, rebound, report.Russian)
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
	if _, err := Translate(t.Context(), executor, provider, rebound, report.English); err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 2 || provider.requests[1].Prompt.ResponseLanguage != "en" {
		t.Fatal("different translation language reused the old exact request")
	}
}

func TestTranslateRejectsIncompleteAmbiguousOrChangedPlaceholders(t *testing.T) {
	catalog := testCatalog(t, []report.DisplayTextEntry{
		{Role: "answer", Text: "Use __REPOMAP_P1__ if possible.", Protected: []report.DisplayProtectedText{{Ref: "__REPOMAP_P1__", Text: "code"}}},
		{Role: "label", Text: "Start here"},
	})
	for _, test := range []struct {
		name string
		edit func(*modelResponse)
	}{
		{"missing", func(response *modelResponse) { response.Translations = response.Translations[:1] }},
		{"blank", func(response *modelResponse) { response.Translations[1].Text = " \n " }},
		{"removed placeholder", func(response *modelResponse) { response.Translations[0].Text = "Translated text" }},
		{"repeated placeholder", func(response *modelResponse) { response.Translations[0].Text += " __REPOMAP_P1__" }},
		{"invented placeholder", func(response *modelResponse) { response.Translations[1].Text += " __REPOMAP_P2__" }},
		{"conflicting duplicate", func(response *modelResponse) {
			response.Translations = append(response.Translations, responseEntry{Ref: "t2", Text: "Different text"})
		}},
		{"unknown cannot replace known", func(response *modelResponse) { response.Translations[1].Ref = "t999" }},
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
			result, err := Translate(t.Context(), executor, provider, catalog, report.Russian)
			if err == nil || !reflect.DeepEqual(result, report.DisplayTranslations{}) {
				t.Fatalf("failed translation became a partial artifact: %#v, %v", result, err)
			}
			if len(events) != 1 || events[0].Failure != llm.FailureValidation || len(provider.requests) != 1 {
				t.Fatalf("failure was not rejected by shared executor before caching: %#v", events)
			}
			provider.respond = nil
			if _, err := Translate(t.Context(), executor, provider, catalog, report.Russian); err != nil {
				t.Fatal(err)
			}
			if len(provider.requests) != 2 || len(events) != 2 || events[1].Kind != llm.EventLive {
				t.Fatal("refused translation was reused instead of obtaining a complete answer")
			}
		})
	}
}

func TestTranslateKeyedWirePreservesOnlyOriginalPlaceholderAuthority(t *testing.T) {
	catalog := testCatalog(t, []report.DisplayTextEntry{
		{Role: "answer", Text: "Use __REPOMAP_P1__ if possible.", Protected: []report.DisplayProtectedText{{Ref: "__REPOMAP_P1__", Text: "source.go"}}},
		{Role: "label", Text: "Start here"},
	})
	provider := &testProvider{rawResponse: []byte(`{
		"t2":"Начало", "t1":"Используйте __REPOMAP_P1__ по возможности.",
		"t\u0031":"Используйте __REPOMAP_P1__ по возможности.",
		"t999":{"protected":["__REPOMAP_P999__"],"text":null}, "unknown":null
	}`)}
	result, err := Translate(t.Context(), llm.Executor{}, provider, catalog, report.Russian)
	if err != nil || len(result.Entries) != 2 || result.Entries[0].Text != "Используйте __REPOMAP_P1__ по возможности." || result.Entries[1].Text != "Начало" {
		t.Fatalf("equal parsed duplicates or unknown refs changed the known translations: %#v %v", result, err)
	}
	raw, err := json.Marshal(result)
	if err != nil || strings.Contains(string(raw), `"protected"`) || strings.Contains(string(raw), "P999") {
		t.Fatal("unknown value acquired display or placeholder authority")
	}
	provider.rawResponse = []byte(`{"t1":"Без обязательного плейсхолдера.","t2":"Начало","protected":["__REPOMAP_P1__"]}`)
	result, err = Translate(t.Context(), llm.Executor{}, provider, catalog, report.Russian)
	if err == nil || !reflect.DeepEqual(result, report.DisplayTranslations{}) {
		t.Fatal("unadvertised output metadata authorized a missing source placeholder")
	}
}

func TestTranslateRejectsInvalidKeyedWireBeforeCache(t *testing.T) {
	catalog := testCatalog(t, plainEntries(2))
	for _, test := range []struct{ name, raw string }{
		{"root array", `[{"t1":"one","t2":"two"}]`},
		{"root null", `null`},
		{"missing object", `{}`},
		{"legacy array", `{"translations":[{"ref":"t1","text":"one"},{"ref":"t2","text":"two"}]}`},
		{"legacy wrapper", `{"translations":{"t1":"one","t2":"two"}}`},
		{"known null", `{"t1":null,"t2":"two"}`},
		{"known number", `{"t1":1,"t2":"two"}`},
		{"known bool", `{"t1":true,"t2":"two"}`},
		{"known array", `{"t1":["one"],"t2":"two"}`},
		{"legacy entry metadata", `{"t1":{"text":"one","protected":[]},"t2":"two"}`},
		{"missing mandatory ref", `{"t1":"one","t999":"two"}`},
		{"conflicting parsed duplicate", `{"t1":"one","t2":"two","t\u0031":"different"}`},
		{"unclosed ref quote", `{"t1":"one","t2:"two"}`},
		{"extra closing brace", `{"t1":"one","t2":"two"}}`},
		{"trailing JSON", `{"t1":"one","t2":"two"} {}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := &testProvider{rawResponse: []byte(test.raw)}
			executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
			result, err := Translate(t.Context(), executor, provider, catalog, report.Russian)
			if err == nil || !reflect.DeepEqual(result, report.DisplayTranslations{}) || len(provider.requests) != 1 {
				t.Fatalf("invalid wire became a partial result, JSON repair or retry: %#v %v", result, err)
			}
			provider.rawResponse = nil
			if _, err := Translate(t.Context(), executor, provider, catalog, report.Russian); err != nil {
				t.Fatal(err)
			}
			if len(provider.requests) != 2 {
				t.Fatal("invalid keyed response entered the accepted cache")
			}
		})
	}
}

func TestTranslatePacksByPreparedProviderEnvelope(t *testing.T) {
	catalog := testCatalog(t, plainEntries(7))
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
	result, err := Translate(t.Context(), llm.Executor{}, provider, catalog, report.Russian)
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 3 {
		t.Fatalf("windows = %d, want 3+3+1 under exact prepared byte budget", len(provider.requests))
	}
	var refs []string
	for i, wire := range provider.requests {
		var request modelRequest
		if err := json.Unmarshal([]byte(wire.Prompt.User), &request); err != nil {
			t.Fatal(err)
		}
		if want := []int{3, 3, 1}[i]; len(request.Entries) != want {
			t.Fatalf("window %d contains %d entries, want %d", i, len(request.Entries), want)
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

	// An accepted first window is not enough for a publishable translation.
	// A missing scalar in the next window must not return those earlier texts.
	provider.requests = nil
	provider.respond = func(request modelRequest) modelResponse {
		response := translatedResponse(request)
		if request.Entries[0].Ref == "t4" {
			response.Translations = response.Translations[:1]
		}
		return response
	}
	failed, err := Translate(t.Context(), llm.Executor{Enabled: true, RootDir: t.TempDir()}, provider, catalog, report.Russian)
	if err == nil || !reflect.DeepEqual(failed, report.DisplayTranslations{}) || len(provider.requests) != 2 {
		t.Fatalf("a refused sibling window produced a partial translation or a semantic retry: %#v, %v", failed, err)
	}
	provider.respond = nil
	provider.requests = nil
	provider.requestBytes = 1
	result, err = Translate(t.Context(), llm.Executor{}, provider, catalog, report.Russian)
	var resourceErr *llm.ResourceLimitError
	if !errors.As(err, &resourceErr) || resourceErr.Kind != llm.ResourceLimitRequestBytes ||
		len(provider.requests) != 0 || !reflect.DeepEqual(result, report.DisplayTranslations{}) {
		t.Fatalf("oversized complete entry was not a preparation failure: %#v, %v", result, err)
	}
}

func TestTranslateSplitsOnlyRealResponseResourcesWithoutPartialPublication(t *testing.T) {
	catalog := testCatalog(t, plainEntries(7))
	for _, kind := range []llm.ResourceLimitKind{llm.ResourceLimitResponseBytes, llm.ResourceLimitOutputTokens} {
		t.Run(string(kind), func(t *testing.T) {
			provider := &testProvider{resourceKind: kind, responseRows: 2}
			executor := llm.Executor{Enabled: true, RootDir: t.TempDir(), BatchConcurrency: 4}
			result, err := Translate(t.Context(), executor, provider, catalog, report.Russian)
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
			if len(provider.requests) <= 4 {
				t.Fatal("test did not exercise real failed resource envelopes and adaptive splits")
			}
			atomicProvider := &testProvider{resourceKind: kind}
			atomicCatalog := testCatalog(t, plainEntries(1))
			failed, err := Translate(t.Context(), llm.Executor{}, atomicProvider, atomicCatalog, report.Russian)
			var resourceErr *llm.ResourceLimitError
			if !errors.As(err, &resourceErr) || resourceErr.Kind != kind || len(atomicProvider.requests) != 1 ||
				!reflect.DeepEqual(failed, report.DisplayTranslations{}) {
				t.Fatalf("atomic resource failure was repaired or partially published: %#v, %v", failed, err)
			}
		})
	}
}

func TestTranslateValidatesBeforeProviderAndAllowsEmptyCatalog(t *testing.T) {
	empty := testCatalog(t, nil)
	result, err := Translate(t.Context(), llm.Executor{}, nil, empty, report.Russian)
	if err != nil || result.Entries == nil || len(result.Entries) != 0 || result.Validate(empty) != nil {
		t.Fatalf("empty catalogue should need no provider: %#v, %v", result, err)
	}
	provider := &testProvider{}
	catalog := testCatalog(t, plainEntries(1))
	badCatalog := catalog
	badCatalog.SHA256 = "wrong"
	if _, err := Translate(t.Context(), llm.Executor{}, provider, badCatalog, report.Russian); err == nil {
		t.Fatal("invalid catalogue was accepted")
	}
	if _, err := Translate(t.Context(), llm.Executor{}, provider, catalog, "other"); err == nil {
		t.Fatal("unsupported display language was accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Translate(ctx, llm.Executor{}, provider, catalog, report.Russian); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled translation = %v", err)
	}
	if _, err := Translate(t.Context(), llm.Executor{}, nil, catalog, report.Russian); err == nil {
		t.Fatal("nonempty catalogue accepted a missing provider")
	}
	provider.prepareFailure = errors.New("provider configuration failure")
	if _, err := Translate(t.Context(), llm.Executor{}, provider, catalog, report.Russian); !errors.Is(err, provider.prepareFailure) {
		t.Fatalf("non-resource prepare error was split or lost: %v", err)
	}
	if len(provider.requests) != 0 {
		t.Fatal("invalid input reached the provider")
	}
}
