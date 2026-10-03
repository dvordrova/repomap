package reporttranslation

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/report"
)

// A text answered twice differently is refused alone, as every closed
// answer is (control review B3, 2026-10-03: the decoder had kept a repeated
// object key's last value): its neighbour is published and the answer
// cached, the text asked once more on its own and, refused again, kept in
// its source language. An identical repeat is one answer.
func TestATextAnsweredTwiceDifferentlyIsRefusedAlone(t *testing.T) {
	catalog := testCatalog(t, []report.DisplayTextEntry{
		{Role: "answer", Text: "Use __REPOMAP_P1__.", Protected: []report.DisplayProtectedText{{Ref: "__REPOMAP_P1__", Text: "original.go"}}},
		{Role: "label", Text: "Start here"},
	})
	const final = `{"text":"Итог __REPOMAP_P1__."}`
	for _, test := range []struct {
		name, members string
		accepted      bool
	}{
		{"different text", `"t1":{"text":"Первый __REPOMAP_P1__."},"t1":` + final, false},
		{"shadowed null", `"t1":null,"t1":` + final, false},
		{"shadowed wrong shape", `"t1":{"text":7,"protected":[]},"t1":` + final, false},
		{"shadowed missing placeholder", `"t1":{"text":"Без ссылки."},"t1":` + final, false},
		{"shadowed unknown placeholder", `"t1":{"text":"__REPOMAP_P999__"},"t\u0031":` + final, false},
		{"nested different text", `"t1":{"text":"Первый __REPOMAP_P1__.","text":"Итог __REPOMAP_P1__."}`, false},
		{"nested wrong type", `"t1":{"text":null,"text":7,"text":"Итог __REPOMAP_P1__."}`, false},
		{"nested escaped key", `"t1":{"text":"__REPOMAP_P999__","\u0074ext":"Итог __REPOMAP_P1__."}`, false},
		{"identical repeat", `"t1":` + final + `,"t\u0031":` + final, true},
		{"nested identical repeat", `"t1":{"text":"Итог __REPOMAP_P1__.","text":"Итог __REPOMAP_P1__."}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Unknown refs have no authority, whatever they repeat.
			raw := []byte(`{"t2":{"text":"Начало"},"t999":{"text":"__REPOMAP_P999__"},"t999":null,` + test.members + `}`)
			provider := &testProvider{rawResponse: raw}
			var events []llm.Event
			executor := llm.Executor{Enabled: true, RootDir: t.TempDir(), Observer: llm.ObserverFunc(func(event llm.Event) error {
				events = append(events, event)
				return nil
			})}
			result, untranslated, err := Translate(t.Context(), executor, provider, catalog, report.Russian)
			if err != nil || result.Validate(catalog) != nil {
				t.Fatalf("translation failed: %+v, %v", result, err)
			}
			want := []report.DisplayTranslationEntry{{Ref: "t1", Text: "Use __REPOMAP_P1__."}, {Ref: "t2", Text: "Начало"}}
			if test.accepted {
				want[0].Text = "Итог __REPOMAP_P1__."
			}
			if !reflect.DeepEqual(result.Entries, want) || (len(untranslated) == 1 && untranslated[0].Ref == "t1") == test.accepted {
				t.Fatalf("entries %+v, untranslated %+v", result.Entries, untranslated)
			}
			// The neighbour's answer is accepted and cached with its exact
			// bytes; a refused one is not.
			accepted := 0
			for _, event := range events {
				cached, found, err := llm.CachedExchange(executor.RootDir, event.CacheKey)
				if err != nil {
					t.Fatal(err)
				}
				if event.Failure != "" {
					if found {
						t.Fatalf("a refused answer was cached: %+v", event)
					}
					continue
				}
				accepted++
				if !found || !bytes.Equal(cached.Response, raw) || !bytes.Equal(cached.Request, event.Request) {
					t.Fatalf("cache changed the original exchange: found=%v", found)
				}
			}
			if accepted == 0 {
				t.Fatal("no answer was accepted")
			}
		})
	}
}

// A repeated value never repairs another: a valid first answer beside a
// different or invalid one refuses the text, and a singleton refused keeps
// its source language, its answer never cached.
func TestARepeatedTextNeverRepairsAnother(t *testing.T) {
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
			result, untranslated, err := Translate(t.Context(), executor, provider, catalog, report.Russian)
			if err != nil || len(untranslated) != 1 || untranslated[0].Ref != "t1" || len(result.Entries) != 1 || result.Entries[0].Text != "Use __REPOMAP_P1__." {
				t.Fatalf("a valid value repaired another: %+v, %+v, %v", result, untranslated, err)
			}
			if len(events) != 1 || events[0].Failure != llm.FailureValidation || !bytes.Equal(events[0].Response, raw) {
				t.Fatalf("singleton rejection lost original response: %+v", events)
			}
			if _, found, err := llm.CachedExchange(executor.RootDir, events[0].CacheKey); err != nil || found {
				t.Fatalf("a refused answer entered the accepted cache: %v", err)
			}
		})
	}
}
