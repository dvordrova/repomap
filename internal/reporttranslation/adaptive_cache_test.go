package reporttranslation

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/report"
)

func TestTranslateReusesResourcePartitionsFromPersistentCache(t *testing.T) {
	for _, kind := range []llm.ResourceLimitKind{llm.ResourceLimitOutputTokens, llm.ResourceLimitContextTokens, llm.ResourceLimitAttemptTime} {
		t.Run(string(kind), func(t *testing.T) {
			entries := plainEntries(32)
			for i := range entries {
				entries[i].Text = fmt.Sprintf("Entry %d. %s", i+1, entries[i].Text)
			}
			catalog := testCatalog(t, entries)
			cacheRoot := t.TempDir()
			provider := &testProvider{resourceKind: kind, responseRows: 2}
			cold, err := Translate(t.Context(), llm.Executor{
				Enabled: true, RootDir: cacheRoot, BatchConcurrency: 1,
			}, provider, catalog, report.Russian)
			if err != nil {
				t.Fatal(err)
			}
			if len(provider.requests) != 24 {
				t.Fatalf("cold calls = %d, want eight refused windows and sixteen accepted children", len(provider.requests))
			}
			// The owner must rebuild complete child requests. Check their union,
			// not just the output length, so duplicated or missing texts fail.
			seen := make(map[string]int, len(entries))
			original := make(map[string]string, len(entries))
			for _, entry := range catalog.Entries {
				original[entry.Ref] = entry.Text
			}
			refused := 0
			for _, wire := range provider.requests {
				var request modelRequest
				if err := json.Unmarshal([]byte(wire.Prompt.User), &request); err != nil {
					t.Fatal(err)
				}
				if len(request.Entries) > provider.responseRows {
					refused++
					continue
				}
				for _, entry := range request.Entries {
					text, known := original[entry.Ref]
					if !known || entry.Text != text {
						t.Fatalf("child changed original %q: %q", entry.Ref, entry.Text)
					}
					seen[entry.Ref]++
				}
			}
			if refused != 8 {
				t.Fatalf("initial resource refusals = %d, want eight", refused)
			}
			if err := cold.Validate(catalog); err != nil {
				t.Fatal(err)
			}
			for i, entry := range cold.Entries {
				if seen[entry.Ref] != 1 || entry.Ref != catalog.Entries[i].Ref || entry.Text != "Перевод: "+catalog.Entries[i].Text {
					t.Fatalf("entry %d did not survive once and intact: %+v; inspected %d times", i, entry, seen[entry.Ref])
				}
			}

			// New provider and executor values rule out reuse of process-local
			// state. Only persisted split observations and exact child responses
			// can avoid repeating the known oversized parent.
			warmProvider := &testProvider{resourceKind: kind, responseRows: 2}
			warm, err := Translate(t.Context(), llm.Executor{
				Enabled: true, RootDir: cacheRoot, BatchConcurrency: 1,
			}, warmProvider, catalog, report.Russian)
			if err != nil {
				t.Fatal(err)
			}
			if len(warmProvider.requests) != 0 {
				t.Fatalf("warm Translate made %d Complete calls", len(warmProvider.requests))
			}
			if !reflect.DeepEqual(warm, cold) {
				t.Fatal("warm Translate changed the complete translated catalogue")
			}
		})
	}
}
