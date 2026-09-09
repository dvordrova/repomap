package reporttranslation

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/report"
)

type parallelTranslationProvider struct {
	*testProvider
	started chan struct{}
	release chan struct{}
}

func (provider *parallelTranslationProvider) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	provider.started <- struct{}{}
	select {
	case <-ctx.Done():
		return llm.Completion{}, ctx.Err()
	case <-provider.release:
		return provider.testProvider.Complete(ctx, prepared)
	}
}

func TestTranslateStartsFourCompletePartitionsAndReusesTheirCache(t *testing.T) {
	entries := plainEntries(1585)
	entries[0].Text = "Use __REPOMAP_P1__.\n\nKeep its full qualification."
	entries[0].Protected = []report.DisplayProtectedText{{Ref: "__REPOMAP_P1__", Text: "private-source"}}
	for i := range entries {
		entries[i].Terms = []report.DisplayTextTerm{{ID: "local-term", Spelling: "bank", Explanation: "A data bank."}}
	}
	catalog := testCatalog(t, entries)
	provider := &parallelTranslationProvider{testProvider: &testProvider{}, started: make(chan struct{}, 4), release: make(chan struct{})}
	executor := llm.Executor{RootDir: t.TempDir(), Enabled: true, BatchConcurrency: 4}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	type result struct {
		value report.DisplayTranslations
		err   error
	}
	finished := make(chan result, 1)
	go func() {
		value, err := Translate(ctx, executor, provider, catalog, report.Russian)
		finished <- result{value, err}
	}()
	for i := 0; i < 4; i++ {
		select {
		case <-provider.started:
		case <-ctx.Done():
			t.Fatalf("only %d independent requests started before any response", i)
		}
	}
	close(provider.release)
	cold := <-finished
	if cold.err != nil || cold.value.Validate(catalog) != nil || len(provider.requests) != 4 {
		t.Fatalf("parallel translation failed: %v; requests %d", cold.err, len(provider.requests))
	}
	seen := make(map[string]int)
	for _, wire := range provider.requests {
		var request modelRequest
		if err := json.Unmarshal([]byte(wire.Prompt.User), &request); err != nil {
			t.Fatal(err)
		}
		assertRequestTerms(t, request, catalog.Entries)
		if wire.Limits.AttemptTimeout != 4*time.Minute || strings.Contains(wire.Prompt.User, "private-source") {
			t.Fatal("deadline or protected local source boundary changed")
		}
		for _, entry := range request.Entries {
			seen[entry.Ref]++
		}
	}
	for i, entry := range cold.value.Entries {
		if seen[entry.Ref] != 1 || entry.Ref != catalog.Entries[i].Ref || entry.Text != "Перевод: "+catalog.Entries[i].Text {
			t.Fatal("parallel partition omitted, duplicated, reordered or cut a text")
		}
	}
	warmProvider := &testProvider{}
	warm, err := Translate(t.Context(), executor, warmProvider, catalog, report.Russian)
	if err != nil || len(warmProvider.requests) != 0 || !reflect.DeepEqual(warm, cold.value) {
		t.Fatal("warm execution retranslated an accepted child or changed its binding")
	}
}

func TestParallelWindowsBalanceTextVolumeWithoutCuttingLongEntries(t *testing.T) {
	entries := plainEntries(9)
	entries[0].Text = strings.Repeat("Long original paragraph.\n", 500)
	catalog := testCatalog(t, entries)
	windows := parallelWindows([]translationWindow{catalog.Entries}, 4)
	if len(windows) != 4 || len(windows[0]) != 1 {
		t.Fatalf("one long text was cut or balanced by row count: %+v", windows)
	}
	var flattened []report.DisplayTextEntry
	for _, window := range windows {
		flattened = append(flattened, window...)
	}
	if !reflect.DeepEqual(flattened, catalog.Entries) {
		t.Fatal("balancing changed the complete ordered input")
	}
	if got := parallelWindows([]translationWindow{catalog.Entries[:1]}, 4); len(got) != 1 {
		t.Fatal("an indivisible text became multiple requests")
	}
}
