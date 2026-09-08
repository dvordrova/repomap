package terminology

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
)

type reductionProvider struct {
	mu            sync.Mutex
	calls         int
	merge         bool
	maxGroups     int
	prepareLimit  int
	refusal       llm.ResourceLimitKind
	response      string
	requests      []string
	rejectName    string
	prepareError  error
	completeError error
	invalidState  bool
	cancel        context.CancelFunc
}

func (provider *reductionProvider) State() []byte {
	if provider.invalidState {
		return []byte(`invalid`)
	}
	return []byte(`{"provider":"glossary-test"}`)
}
func (provider *reductionProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	if provider.prepareError != nil {
		return llm.Prepared{}, provider.prepareError
	}
	if limits.MaxOutputTokens != llm.DefaultMaxOutputTokens || limits.MaxRequestBytes != llm.SemanticRecordByteLimit || !prompt.ResponseFormatJSON {
		return llm.Prepared{}, fmt.Errorf("wrong shared request envelope")
	}
	var request struct {
		Groups []wireGroup `json:"groups"`
	}
	if err := json.Unmarshal([]byte(prompt.User), &request); err != nil {
		return llm.Prepared{}, err
	}
	if provider.prepareLimit > 0 && len(request.Groups) > provider.prepareLimit {
		return llm.Prepared{}, llm.NewResourceLimitError(llm.ResourceLimitError{Kind: llm.ResourceLimitRequestBytes, Limit: provider.prepareLimit, Observed: len(request.Groups), ObservedKnown: true})
	}
	return llm.NewPrepared([]byte(prompt.System + "\n\n" + prompt.User))
}
func (provider *reductionProvider) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.calls++
	if provider.cancel != nil {
		provider.cancel()
	}
	_, body, found := strings.Cut(string(prepared.Bytes()), reducePrompt+"\n\n")
	if !found {
		return llm.Completion{}, fmt.Errorf("missing owning prompt")
	}
	provider.requests = append(provider.requests, body)
	if provider.completeError != nil {
		return llm.Completion{}, provider.completeError
	}
	var request struct {
		Groups []wireGroup `json:"groups"`
	}
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		return llm.Completion{}, err
	}
	if provider.refusal != "" && len(request.Groups) > provider.maxGroups {
		return llm.Completion{}, llm.NewResourceLimitError(llm.ResourceLimitError{Kind: provider.refusal, Limit: provider.maxGroups, Observed: len(request.Groups), ObservedKnown: true})
	}
	response := provider.response
	for _, group := range request.Groups {
		for _, variant := range group.Variants {
			if variant.Name == provider.rejectName {
				response = `{"groups":[]}`
			}
		}
	}
	if response == "" {
		type group struct {
			Members        []string `json:"members"`
			Representative string   `json:"representative"`
		}
		result := struct {
			Groups []group `json:"groups"`
		}{}
		if provider.merge {
			joined := group{Representative: request.Groups[0].Variants[0].Ref}
			for _, item := range request.Groups {
				joined.Members = append(joined.Members, item.Ref)
			}
			result.Groups = append(result.Groups, joined)
		} else {
			for _, item := range request.Groups {
				result.Groups = append(result.Groups, group{[]string{item.Ref}, item.Variants[0].Ref})
			}
		}
		encoded, _ := json.Marshal(result)
		response = string(encoded)
	}
	return llm.Completion{Response: []byte(response), FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, nil
}

func termCandidate(name, explanation, file string) Candidate {
	return Candidate{Name: name, Explanation: explanation, Sources: []Source{{Path: file, Line: 12}}, Origins: []Origin{{RequestSHA256: "original-private-request-reference", Row: "r1"}}}
}

func TestReduceAliasesRetainsEveryOriginalAndUsesExactCache(t *testing.T) {
	items := []Candidate{termCandidate("DAG", "A directed acyclic graph.", "graph.py"), termCandidate("directed acyclic graph", "A graph with no directed cycles.", "graph.py")}
	provider := &reductionProvider{merge: true}
	executor := llm.Executor{RootDir: t.TempDir(), Enabled: true}
	got, err := Reduce(t.Context(), executor, provider, items)
	if err != nil || len(got.Entries) != 1 || got.PartialComparison || len(got.Entries[0].Names) != 2 || len(got.Entries[0].Variants) != 2 {
		t.Fatalf("reduction = %+v, %v", got, err)
	}
	if got.Entries[0].Explanation != items[0].Explanation && got.Entries[0].Explanation != items[1].Explanation {
		t.Fatal("reduction invented representative prose")
	}
	if len(got.Requests) != 1 || got.Validate() != nil {
		t.Fatal("accepted reduction lacks sealed request provenance")
	}
	for _, wire := range provider.requests {
		if strings.Contains(wire, "original-private-request-reference") || strings.Contains(wire, got.Entries[0].ID) || strings.Contains(wire, `"origins"`) || strings.Contains(wire, `"request_sha256"`) || strings.Contains(wire, `"id"`) {
			t.Fatal("local canonical identity or request provenance leaked into provider input")
		}
	}
	warm, err := Reduce(t.Context(), executor, provider, []Candidate{items[1], items[0]})
	if err != nil || provider.calls != 1 || !reflect.DeepEqual(got, warm) {
		t.Fatalf("reorder lost exact cache or canonical identity: calls=%d err=%v", provider.calls, err)
	}
	items[0].Origins = []Origin{{RequestSHA256: "new-local-owner-reference", Row: "r2"}}
	rebound, err := Reduce(t.Context(), executor, provider, items)
	if err != nil || provider.calls != 1 || rebound.Entries[0].ID != got.Entries[0].ID || rebound.SHA256 == got.SHA256 {
		t.Fatalf("local provenance changed the provider request or semantic ID: calls=%d err=%v", provider.calls, err)
	}
	if _, err := rebound.CanonicalJSON(); err != nil {
		t.Fatal(err)
	}
	rebound.Entries[0].Explanation = "Unsaved synthesized prose."
	if rebound.Validate() == nil {
		t.Fatal("tampered explanation was accepted")
	}
}

func TestReduceClosedReferencesAndCompleteCoverage(t *testing.T) {
	items := []Candidate{termCandidate("alpha", "Alpha meaning.", "a.py"), termCandidate("beta", "Beta meaning.", "b.py")}
	var originals []Entry
	for _, item := range items {
		entry, err := makeEntry(item.Explanation, []Candidate{item})
		if err != nil {
			t.Fatal(err)
		}
		originals = append(originals, entry)
	}
	call, err := reductionCall(originals)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, response string
		valid          bool
	}{
		{"unknown members and duplicate set values", `{"groups":[{"members":["g1","g1","g999"],"representative":"v1"},{"members":["unknown"],"representative":"unknown"},{"members":["g2"],"representative":"v2"}]}`, true},
		{"missing original", `{"groups":[{"members":["g1"],"representative":"v1"}]}`, false},
		{"unknown representative", `{"groups":[{"members":["g1","g2"],"representative":"unknown"}]}`, false},
		{"representative outside group", `{"groups":[{"members":["g1"],"representative":"v2"},{"members":["g2"],"representative":"v2"}]}`, false},
		{"conflicting group assignment", `{"groups":[{"members":["g1","g2"],"representative":"v1"},{"members":["g2"],"representative":"v2"}]}`, false},
		{"identical repeated assignment", `{"groups":[{"members":["g1"],"representative":"v1"},{"members":["g1"],"representative":"v1"},{"members":["g2"],"representative":"v2"}]}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, decodeErr := call.DecodeValidate([]byte(test.response))
			if (decodeErr == nil) != test.valid {
				t.Fatalf("decoder valid=%v, err=%v", test.valid, decodeErr)
			}
			provider := &reductionProvider{response: test.response}
			var failures []llm.FailureKind
			executor := llm.Executor{RootDir: t.TempDir(), Enabled: true, Observer: llm.ObserverFunc(func(event llm.Event) error {
				if event.Kind == llm.EventFailure {
					failures = append(failures, event.Failure)
				}
				return nil
			})}
			got, err := Reduce(t.Context(), executor, provider, items)
			if err != nil || len(got.Entries) != 2 || got.PartialComparison == test.valid || provider.calls != 1 {
				t.Fatalf("valid=%v, got=%+v err=%v", test.valid, got, err)
			}
			if !test.valid {
				if !reflect.DeepEqual(failures, []llm.FailureKind{llm.FailureValidation}) || len(got.Requests) != 0 {
					t.Fatal("refused response was not rejected or gained accepted provenance")
				}
				for _, original := range originals {
					found := false
					for _, entry := range got.Entries {
						found = found || reflect.DeepEqual(original, entry)
					}
					if !found {
						t.Fatal("refused response modified an original entry")
					}
				}
				if _, err := Reduce(t.Context(), executor, provider, items); err != nil || provider.calls != 2 {
					t.Fatal("refused consolidation was cached")
				}
			}
		})
	}
}

func TestReduceAdaptiveWindowsRetainEvidenceAndReportFixedPoint(t *testing.T) {
	var items []Candidate
	for i := range 4 {
		items = append(items, termCandidate(fmt.Sprintf("name-%d", i), fmt.Sprintf("Original explanation %d.", i), "terms.py"))
	}
	for _, kind := range []llm.ResourceLimitKind{llm.ResourceLimitContextTokens, llm.ResourceLimitOutputTokens, llm.ResourceLimitResponseBytes} {
		t.Run(string(kind), func(t *testing.T) {
			provider := &reductionProvider{merge: true, maxGroups: 2, refusal: kind}
			got, err := Reduce(t.Context(), llm.Executor{RootDir: t.TempDir(), Enabled: true, BatchConcurrency: 2}, provider, items)
			if err != nil || len(got.Entries) != 1 || len(got.Entries[0].Variants) != 4 || got.PartialComparison {
				t.Fatalf("adaptive lossless reduction = %+v, %v", got, err)
			}
			var final struct {
				Groups []wireGroup `json:"groups"`
			}
			if err := json.Unmarshal([]byte(provider.requests[len(provider.requests)-1]), &final); err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, group := range final.Groups {
				count += len(group.Variants)
			}
			if len(final.Groups) != 2 || count != 4 {
				t.Fatal("next round discarded original evidence instead of carrying its full groups")
			}
		})
	}
	provider := &reductionProvider{maxGroups: 1, refusal: llm.ResourceLimitContextTokens}
	got, err := Reduce(t.Context(), llm.Executor{RootDir: t.TempDir(), Enabled: true}, provider, items)
	if err != nil || len(got.Entries) != 4 || !got.PartialComparison {
		t.Fatalf("fixed point = %+v, %v", got, err)
	}
	provider = &reductionProvider{prepareLimit: 1}
	got, err = Reduce(t.Context(), llm.Executor{}, provider, items)
	if err != nil || len(got.Entries) != 4 || !got.PartialComparison || provider.calls != 4 {
		t.Fatalf("prepared envelope did not partition complete evidence: %+v, %v, calls=%d", got, err, provider.calls)
	}
}

func TestReduceEmptyAndAtomicResourceFailure(t *testing.T) {
	empty, err := Reduce(t.Context(), llm.Executor{}, nil, nil)
	if err != nil || len(empty.Entries) != 0 || empty.Validate() != nil {
		t.Fatalf("empty catalogue = %+v %v", empty, err)
	}
	items := []Candidate{termCandidate("Only", "Indivisible original evidence.", "only.py")}
	single, err := Reduce(t.Context(), llm.Executor{}, nil, items)
	if err != nil || len(single.Entries) != 1 || single.PartialComparison || single.Validate() != nil {
		t.Fatal("a single accepted definition required a provider comparison")
	}
	items = append(items, termCandidate("Second", "Another complete original.", "second.py"))
	provider := &reductionProvider{refusal: llm.ResourceLimitOutputTokens}
	got, err := Reduce(t.Context(), llm.Executor{}, provider, items)
	if err != nil || len(got.Entries) != 2 || !got.PartialComparison || provider.calls != 3 {
		t.Fatalf("atomic refusals failed to preserve input after lossless partition: %+v, %v, calls=%d", got, err, provider.calls)
	}
	provider = &reductionProvider{prepareError: llm.NewResourceLimitError(llm.ResourceLimitError{Kind: llm.ResourceLimitRequestBytes})}
	got, err = Reduce(t.Context(), llm.Executor{}, provider, items)
	if err != nil || len(got.Entries) != 2 || !got.PartialComparison || provider.calls != 0 {
		t.Fatalf("atomic preparation refusal = %+v, %v, calls=%d", got, err, provider.calls)
	}
}

func TestReduceRefusedWindowPreservesOriginalsAndAcceptedSibling(t *testing.T) {
	var items []Candidate
	for _, name := range []string{"aa", "bb", "cc", "dd"} {
		item := termCandidate(name, "An accepted original definition.", name+".py")
		item.Origins = []Origin{{RequestSHA256: "original-request-" + name, Row: "r1"}}
		items = append(items, item)
	}
	provider := &reductionProvider{prepareLimit: 2, merge: true, rejectName: "aa"}
	var failures, accepted int
	executor := llm.Executor{RootDir: t.TempDir(), Enabled: true, BatchConcurrency: 2, Observer: llm.ObserverFunc(func(event llm.Event) error {
		if event.Kind == llm.EventFailure {
			failures++
		}
		if event.Kind == llm.EventLive {
			accepted++
		}
		return nil
	})}
	got, err := Reduce(t.Context(), executor, provider, items)
	if err != nil || len(got.Entries) != 3 || !got.PartialComparison || provider.calls != 2 || failures != 1 || accepted != 1 || len(got.Requests) != 1 {
		t.Fatalf("sibling outcomes = %+v, %v, calls=%d failures=%d accepted=%d", got, err, provider.calls, failures, accepted)
	}
	var variants []Candidate
	for _, entry := range got.Entries {
		variants = append(variants, entry.Variants...)
	}
	normalized, err := normalizeCandidates(variants)
	if err != nil || !reflect.DeepEqual(normalized, items) {
		t.Fatalf("lost original source/request evidence: %+v %v", normalized, err)
	}
	for _, original := range items[:2] {
		expected, err := makeEntry(original.Explanation, []Candidate{original})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, entry := range got.Entries {
			found = found || reflect.DeepEqual(entry, expected)
		}
		if !found {
			t.Fatal("refused window changed an accepted input entry")
		}
	}
	if _, err := Reduce(t.Context(), executor, provider, items); err != nil || provider.calls != 3 {
		t.Fatal("accepted sibling was not cached independently of the refused window")
	}
}

func TestReduceStageRefusalDoesNotSwallowLocalOrPersistenceFailures(t *testing.T) {
	items := []Candidate{termCandidate("alpha", "Alpha definition.", "a.py"), termCandidate("beta", "Beta definition.", "b.py")}
	t.Run("provider failure preserves accepted definitions", func(t *testing.T) {
		provider := &reductionProvider{completeError: errors.New("service unavailable")}
		got, err := Reduce(t.Context(), llm.Executor{}, provider, items)
		if err != nil || len(got.Entries) != 2 || !got.PartialComparison || provider.calls != 1 {
			t.Fatalf("provider refusal = %+v %v", got, err)
		}
	})
	t.Run("cancelled complete", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		got, err := Reduce(ctx, llm.Executor{}, &reductionProvider{cancel: cancel}, items)
		if !errors.Is(err, context.Canceled) || len(got.Entries) != 0 {
			t.Fatalf("cancelled run = %+v %v", got, err)
		}
	})
	t.Run("configuration failure", func(t *testing.T) {
		cause := errors.New("provider is not configured")
		provider := &reductionProvider{prepareError: cause}
		got, err := Reduce(t.Context(), llm.Executor{}, provider, items)
		if !errors.Is(err, cause) || len(got.Entries) != 0 || provider.calls != 0 {
			t.Fatalf("configuration error = %+v %v", got, err)
		}
	})
	t.Run("invalid prepared provider state", func(t *testing.T) {
		provider := &reductionProvider{invalidState: true}
		got, err := Reduce(t.Context(), llm.Executor{RootDir: t.TempDir(), Enabled: true}, provider, items)
		if err == nil || len(got.Entries) != 0 || provider.calls != 0 {
			t.Fatalf("invalid provider identity = %+v %v", got, err)
		}
	})
	t.Run("journal failure", func(t *testing.T) {
		cause := errors.New("journal unavailable")
		executor := llm.Executor{Observer: llm.ObserverFunc(func(llm.Event) error { return cause })}
		got, err := Reduce(t.Context(), executor, &reductionProvider{}, items)
		if !errors.Is(err, cause) || len(got.Entries) != 0 {
			t.Fatalf("journal failure = %+v %v", got, err)
		}
	})
	t.Run("cache persistence failure", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "file-blocks-cache-directory")
		if err := os.WriteFile(root, []byte("occupied"), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := Reduce(t.Context(), llm.Executor{RootDir: root, Enabled: true}, &reductionProvider{}, items)
		if err == nil || len(got.Entries) != 0 {
			t.Fatalf("cache failure = %+v %v", got, err)
		}
	})
}

func TestReduceCompleteCatalogueHasNoCandidateCountQuota(t *testing.T) {
	var items []Candidate
	for i := range 605 {
		items = append(items, termCandidate(fmt.Sprintf("Term%03d", i), "An original source-bound explanation.", fmt.Sprintf("sources/%03d.py", i)))
	}
	provider := &reductionProvider{}
	got, err := Reduce(t.Context(), llm.Executor{}, provider, items)
	if err != nil || len(got.Entries) != len(items) || provider.calls != 1 || got.PartialComparison {
		t.Fatalf("complete fitting catalogue split or lost entries: entries=%d calls=%d partial=%v err=%v", len(got.Entries), provider.calls, got.PartialComparison, err)
	}
}

func TestReduceRejectsNoncanonicalSavedSourcesBeforeProvider(t *testing.T) {
	for _, source := range []string{"/absolute/host/source.py", "..", "a\x00b.py"} {
		t.Run(fmt.Sprintf("%q", source), func(t *testing.T) {
			invalid := termCandidate("Invalid source", "Must stay local.", source)
			provider := &reductionProvider{}
			if _, err := Reduce(t.Context(), llm.Executor{}, provider, []Candidate{invalid}); err == nil || provider.calls != 0 {
				t.Fatal("a noncanonical source reached the provider")
			}
			catalog := Catalog{Version: CatalogVersion, Entries: []Entry{{Explanation: invalid.Explanation, Variants: []Candidate{invalid}}}}
			if err := catalog.Seal(); err == nil {
				t.Fatal("a noncanonical saved source acquired catalogue authority")
			}
		})
	}
}
