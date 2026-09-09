package reporttranslation

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/report"
)

func TestTranslateLastObjectValueWinsBeforeValidationAndCache(t *testing.T) {
	catalog := testCatalog(t, []report.DisplayTextEntry{
		{Role: "answer", Text: "Use __REPOMAP_P1__.", Protected: []report.DisplayProtectedText{{Ref: "__REPOMAP_P1__", Text: "original.go"}}},
		{Role: "label", Text: "Start here"},
	})
	const final = `{"text":"Итог __REPOMAP_P1__."}`
	for _, test := range []struct{ name, members string }{
		{"different text", `"t1":{"text":"Первый __REPOMAP_P1__."},"t1":` + final},
		{"shadowed null", `"t1":null,"t1":` + final},
		{"shadowed wrong shape", `"t1":{"text":7,"protected":[]},"t1":` + final},
		{"shadowed missing placeholder", `"t1":{"text":"Без ссылки."},"t1":` + final},
		{"shadowed unknown placeholder", `"t1":{"text":"__REPOMAP_P999__"},"t\u0031":` + final},
		{"nested different text", `"t1":{"text":"Первый __REPOMAP_P1__.","text":"Итог __REPOMAP_P1__."}`},
		{"nested wrong type", `"t1":{"text":null,"text":7,"text":"Итог __REPOMAP_P1__."}`},
		{"nested escaped key", `"t1":{"text":"__REPOMAP_P999__","\u0074ext":"Итог __REPOMAP_P1__."}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Unknown refs have no authority, including after their own overwrite.
			raw := []byte(`{"t2":{"text":"Начало"},"t999":{"text":"__REPOMAP_P999__"},"t999":null,` + test.members + `}`)
			provider := &testProvider{rawResponse: raw}
			var events []llm.Event
			executor := llm.Executor{Enabled: true, RootDir: t.TempDir(), Observer: llm.ObserverFunc(func(event llm.Event) error {
				events = append(events, event)
				return nil
			})}
			want := []report.DisplayTranslationEntry{{Ref: "t1", Text: "Итог __REPOMAP_P1__."}, {Ref: "t2", Text: "Начало"}}
			for run := 0; run < 2; run++ {
				result, err := Translate(t.Context(), executor, provider, catalog, report.Russian)
				if err != nil || !reflect.DeepEqual(result.Entries, want) || result.Validate(catalog) != nil {
					t.Fatalf("run %d did not validate final values in catalogue order: %+v, %v", run, result, err)
				}
			}
			if len(provider.requests) != 2 || len(events) != 4 || events[0].Kind != llm.EventLive || events[2].Kind != llm.EventCacheHit {
				t.Fatalf("last values did not reuse accepted requests: calls=%d events=%d", len(provider.requests), len(events))
			}
			cached, found, err := llm.CachedExchange(executor.RootDir, events[0].CacheKey)
			if err != nil || !found || !bytes.Equal(cached.Response, raw) || !bytes.Equal(cached.Request, events[0].Request) {
				t.Fatalf("cache changed the original exchange: found=%v, %v", found, err)
			}
			for i, event := range events {
				if !bytes.Equal(event.Response, raw) || event.CacheKey != events[i%2].CacheKey || event.Failure != "" {
					t.Fatal("observer lost the original response or used a shadowed value")
				}
			}
		})
	}
}

func TestTranslateLastObjectValueStillMustValidate(t *testing.T) {
	catalog := testCatalog(t, []report.DisplayTextEntry{
		{Role: "answer", Text: "Use __REPOMAP_P1__.", Protected: []report.DisplayProtectedText{{Ref: "__REPOMAP_P1__", Text: "original.go"}}},
	})
	for _, test := range []struct{ name, last string }{
		{"null", `null`},
		{"missing text", `{}`},
		{"missing placeholder", `{"text":"Нет ссылки."}`},
		{"unknown placeholder", `{"text":"__REPOMAP_P1__ __REPOMAP_P999__"}`},
		{"empty text", `{"text":""}`},
		{"nested null", `{"text":"__REPOMAP_P1__","text":null}`},
		{"nested wrong type", `{"text":"__REPOMAP_P1__","text":7}`},
		{"nested missing placeholder", `{"text":"__REPOMAP_P1__","text":"Нет ссылки."}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw := []byte(`{"t1":{"text":"Первый __REPOMAP_P1__."},"t\u0031":` + test.last + `}`)
			provider := &testProvider{rawResponse: raw}
			var events []llm.Event
			executor := llm.Executor{Enabled: true, RootDir: t.TempDir(), Observer: llm.ObserverFunc(func(event llm.Event) error {
				events = append(events, event)
				return nil
			})}
			result, err := Translate(t.Context(), executor, provider, catalog, report.Russian)
			if err == nil || !reflect.DeepEqual(result, report.DisplayTranslations{}) {
				t.Fatalf("earlier valid value repaired invalid final value: %+v, %v", result, err)
			}
			if len(events) != 1 || events[0].Failure != llm.FailureValidation || !bytes.Equal(events[0].Response, raw) {
				t.Fatalf("singleton rejection lost original response: %+v", events)
			}
			if _, found, err := llm.CachedExchange(executor.RootDir, events[0].CacheKey); err != nil || found {
				t.Fatalf("invalid final value entered accepted cache: %v", err)
			}
		})
	}
}
