package terminology

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
)

// Resolve only this request's advertised catalogues, as a reader must. This
// also catches children accidentally depending on parent refs or partial sets.
func reductionRequestCandidates(t *testing.T, body string) ([]Candidate, reductionRequest) {
	t.Helper()
	var request reductionRequest
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		t.Fatal(err)
	}
	sources := map[string]Source{}
	for _, item := range request.Sources {
		if _, exists := sources[item.Ref]; exists || item.Ref == "" {
			t.Fatalf("duplicate/empty source ref: %+v", item)
		}
		sources[item.Ref] = item.Source
	}
	sets := map[string][]Source{}
	usedSources, usedSets := map[string]bool{}, map[string]bool{}
	for _, item := range request.SourceSets {
		if _, exists := sets[item.Ref]; exists || item.Ref == "" {
			t.Fatalf("duplicate/empty source set: %+v", item)
		}
		var resolved []Source
		for _, ref := range item.Sources {
			source, exists := sources[ref]
			if !exists {
				t.Fatalf("source set %s uses unadvertised source %s", item.Ref, ref)
			}
			resolved = append(resolved, source)
			usedSources[ref] = true
		}
		sets[item.Ref] = resolved
	}
	var result []Candidate
	for _, group := range request.Groups {
		for _, variant := range group.Variants {
			scope, exists := sets[variant.SourceSet]
			if !exists {
				t.Fatalf("variant %s uses an unadvertised source set %s", variant.Ref, variant.SourceSet)
			}
			usedSets[variant.SourceSet] = true
			result = append(result, Candidate{Name: variant.Name, Explanation: variant.Explanation, Sources: scope})
		}
	}
	if len(usedSources) != len(sources) || len(usedSets) != len(sets) {
		t.Fatal("request carries source context from an unrelated window")
	}
	return result, request
}

func TestReductionSourceCataloguesPreserveEveryScopeAcrossChildren(t *testing.T) {
	shared := []Source{{Path: "README.md", Line: 0}, {Path: "src/시세.py", Line: 12}, {Path: "src/시세.py", Line: 21}}
	items := []Candidate{
		{Name: "Price", Explanation: "The current quoted price.", Sources: shared, Origins: []Origin{{RequestSHA256: "original-price", Row: "r1"}}},
		{Name: "Quote", Explanation: "The exchange's quote.\nIts price may change.", Sources: shared, Origins: []Origin{{RequestSHA256: "original-quote", Row: "r2"}}},
		{Name: "LastTrade", Explanation: "One completed trade, not a live quote.", Sources: []Source{{Path: "src/시세.py", Line: 12}}, Origins: []Origin{{RequestSHA256: "original-trade", Row: "r3"}}},
		{Name: "TradeHistory", Explanation: "The exchange's completed trades.", Sources: []Source{{Path: "src/시세.py", Line: 12}, {Path: "src/시세.py", Line: 38}}, Origins: []Origin{{RequestSHA256: "original-history", Row: "r4"}}},
	}
	originals, err := normalizeCandidates(items)
	if err != nil {
		t.Fatal(err)
	}
	var entries []Entry
	want := map[string]Candidate{}
	for _, item := range originals {
		entry, err := makeEntry(item.Explanation, []Candidate{item})
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, entry)
		item.Origins = nil
		want[item.Name] = item
	}
	call, err := reductionCall(entries)
	if err != nil {
		t.Fatal(err)
	}
	resolved, request := reductionRequestCandidates(t, call.Prompt.User)
	if len(request.Sources) != 4 || len(request.SourceSets) != 3 || len(resolved) != len(items) {
		t.Fatalf("repeated source scopes not factored exactly: %+v", request)
	}
	for _, item := range resolved {
		if !reflect.DeepEqual(item, want[item.Name]) {
			t.Fatalf("full original variant changed: got %+v want %+v", item, want[item.Name])
		}
	}
	// Force an actual resource refusal followed by independent child windows.
	provider := &reductionProvider{maxGroups: 2, refusal: llm.ResourceLimitContextTokens}
	got, err := Reduce(t.Context(), llm.Executor{RootDir: t.TempDir(), Enabled: true}, provider, items)
	if err != nil || !got.PartialComparison || len(got.Entries) != len(items) {
		t.Fatalf("split altered original entries: %+v / %v", got, err)
	}
	if len(provider.requests) < 3 {
		t.Fatalf("actual resource refusal did not produce child requests: %d", len(provider.requests))
	}
	for _, body := range provider.requests {
		resolved, _ := reductionRequestCandidates(t, body)
		for _, item := range resolved {
			if !reflect.DeepEqual(item, want[item.Name]) {
				t.Fatalf("child lost its original full source scope: %+v", item)
			}
		}
	}
	var retained []Candidate
	for _, entry := range got.Entries {
		retained = append(retained, entry.Variants...)
	}
	retained, err = normalizeCandidates(retained)
	if err != nil || !reflect.DeepEqual(retained, originals) {
		t.Fatal("source factoring changed accepted variants or their local origins")
	}
}

func TestReductionRepeatedProvenanceIsEncodedOnce(t *testing.T) {
	scope := []Source{{Path: "service.py", Line: 0}, {Path: "service.py", Line: 30}}
	var entries []Entry
	for i := range 100 {
		candidate := Candidate{Name: fmt.Sprintf("Concept%d", i), Explanation: "An original explanation.", Sources: scope}
		entry, err := makeEntry(candidate.Explanation, []Candidate{candidate})
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, entry)
	}
	call, err := reductionCall(entries)
	if err != nil {
		t.Fatal(err)
	}
	resolved, request := reductionRequestCandidates(t, call.Prompt.User)
	if len(request.Sources) != len(scope) || len(request.SourceSets) != 1 || len(request.Groups) != len(entries) || len(resolved) != len(entries) {
		t.Fatal("factoring introduced a quota or copied shared provenance for each variant")
	}
}
