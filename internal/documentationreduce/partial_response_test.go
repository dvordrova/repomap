package documentationreduce

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
)

type resourceDocumentationProvider struct {
	documentationPresetProvider
	mu       sync.Mutex
	requests map[string]int
	contents map[string]string
}

func (p *resourceDocumentationProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	var request sourceRequest
	_ = json.Unmarshal([]byte(prompt.User), &request)
	if len(request.Documents) > 2 {
		return llm.Prepared{}, llm.NewResourceLimitError(llm.ResourceLimitError{Kind: llm.ResourceLimitRequestBytes, Limit: 1})
	}
	return p.documentationPresetProvider.Prepare(prompt, limits)
}

func (p *resourceDocumentationProvider) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var prompt documentationPresetPrepared
	_ = json.Unmarshal(prepared.Bytes(), &prompt)
	var request sourceRequest
	_ = json.Unmarshal([]byte(prompt.User), &request)
	if len(request.Documents) == 0 {
		return p.documentationPresetProvider.Complete(ctx, prepared)
	}
	var refs []string
	for _, document := range request.Documents {
		refs = append(refs, document.Ref)
	}
	p.mu.Lock()
	if p.requests == nil {
		p.requests = make(map[string]int)
		p.contents = make(map[string]string)
	}
	p.requests[strings.Join(refs, ",")]++
	if len(request.Documents) == 1 {
		p.contents[request.Documents[0].Path] = request.Documents[0].Content
	}
	p.mu.Unlock()
	if len(request.Documents) > 1 {
		return llm.Completion{}, llm.NewResourceLimitError(llm.ResourceLimitError{Kind: llm.ResourceLimitContextTokens, Limit: 1})
	}
	return p.documentationPresetProvider.Complete(ctx, prepared)
}

func TestDocumentationResourceSplitKeepsExactSiblingAndCompleteChildren(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(map[bool]string{false: "no_cache", true: "warm_cache"}[cached], func(t *testing.T) {
			guidance := guidanceFixture(t, []readmetargetscout.GuidanceDocument{
				{Path: "a/README.md", Kind: readmetargetscout.GuidanceReadme, Content: "Original a."},
				{Path: "b/README.md", Kind: readmetargetscout.GuidanceReadme, Content: "Original b."},
				{Path: "c/README.md", Kind: readmetargetscout.GuidanceReadme, Content: "Original c."},
			})
			provider := &resourceDocumentationProvider{}
			executor := llm.Executor{Enabled: cached, RootDir: t.TempDir(), BatchConcurrency: 2}
			runs := 1
			if cached {
				runs = 2
			}
			for range runs {
				result, err := Run(t.Context(), executor, provider, guidance)
				if err != nil || len(result.Sources) != 3 {
					t.Fatalf("resource split lost accepted sources: %+v / %v", result, err)
				}
			}
			if len(provider.requests) != 4 {
				t.Fatalf("unexpected requests: %v", provider.requests)
			}
			for request, count := range provider.requests {
				if count != 1 {
					t.Fatalf("repeated %s %d times", request, count)
				}
			}
			for _, document := range guidance.Documents {
				if provider.contents[document.Path] != document.Content {
					t.Fatalf("child lost original content for %s", document.Path)
				}
			}
		})
	}
}

type partialDocumentationProvider struct {
	documentationPresetProvider
	raw             []byte
	mu              sync.Mutex
	calls           int
	separateSources bool
	separateMerge   bool
}

func (p *partialDocumentationProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	if p.separateSources {
		var request sourceRequest
		_ = json.Unmarshal([]byte(prompt.User), &request)
		if len(request.Documents) > 1 {
			return llm.Prepared{}, llm.NewResourceLimitError(llm.ResourceLimitError{Kind: llm.ResourceLimitRequestBytes, Limit: 1})
		}
	}
	if p.separateMerge {
		var request mergeRequest
		_ = json.Unmarshal([]byte(prompt.User), &request)
		if len(request.Candidates) > 1 {
			return llm.Prepared{}, llm.NewResourceLimitError(llm.ResourceLimitError{Kind: llm.ResourceLimitRequestBytes, Limit: 1})
		}
	}
	return p.documentationPresetProvider.Prepare(prompt, limits)
}

func (p *partialDocumentationProvider) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	raw := p.raw
	if p.separateSources {
		var prompt documentationPresetPrepared
		_ = json.Unmarshal(prepared.Bytes(), &prompt)
		var request sourceRequest
		_ = json.Unmarshal([]byte(prompt.User), &request)
		if len(request.Documents) == 1 && request.Documents[0].Path == "README.md" {
			raw, _ = json.Marshal(modelResponse{Overview: "A source-local overview.", Sources: []responseSource{{Ref: request.Documents[0].Ref, Concepts: []string{"A valid author concept."}}}})
		}
	}
	if p.separateMerge {
		var prompt documentationPresetPrepared
		_ = json.Unmarshal(prepared.Bytes(), &prompt)
		var request mergeRequest
		_ = json.Unmarshal([]byte(prompt.User), &request)
		if request.Candidates[0].Sources[0].Ref == "d0002" {
			raw = []byte(`{"overview":"","sources":[]}`)
		}
	}
	return llm.Completion{Response: raw, FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, nil
}

func TestDocumentationKeepsGoodConceptsAndSourcesWithOriginalCacheAndRejections(t *testing.T) {
	authority := map[string]documentAuthority{"d1": {path: "AGENTS.md", kind: readmetargetscout.GuidanceAgents}, "d2": {path: "README.md", kind: readmetargetscout.GuidanceReadme}}
	raw := []byte(`{"overview":17,"sources":[
 {"ref":"d1","concepts":["  Good concept.  ",9,""],"claims":["Retired field."],"unused":true},
 {"ref":"d2","concepts":["Another good concept."]},
 {"ref":"d999","concepts":null}],"notes":"unused"}`)
	provider := &partialDocumentationProvider{raw: raw}
	var events []llm.Event
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir(), Observer: llm.ObserverFunc(func(event llm.Event) error { events = append(events, event); return nil })}
	call := reductionCall("source", "guidance", sourcePrompt, []byte(`{"documents":[]}`), authority)
	for run := range 2 {
		outcome, err := llm.ExecuteJSON(t.Context(), executor, provider, call)
		if err != nil || len(outcome.Value.sources) != 2 || outcome.Value.overview != "" || outcome.Cached != (run == 1) || !bytes.Equal(outcome.Response, raw) {
			t.Fatalf("partial reduction: %+v / %v", outcome, err)
		}
		if !reflect.DeepEqual(outcome.Value.sources[0].Concepts, []string{"Good concept."}) || !reflect.DeepEqual(outcome.Value.AcceptedRowKeys(), []string{"d2"}) {
			t.Fatalf("wrong accepted concepts or term scopes: %+v", outcome.Value)
		}
		if len(outcome.ResponseRejections) != 4 {
			t.Fatalf("lost local rejection diagnostics: %+v", outcome.ResponseRejections)
		}
	}
	if provider.calls != 1 || len(events) != 2 || len(events[0].ResponseRejections) != 4 || len(events[1].ResponseRejections) != 4 {
		t.Fatalf("repeated calls/events or missing journal rejections: %d / %+v", provider.calls, events)
	}
}

func TestDocumentationRefusedSourceKeepsSiblingWithoutInventingOverview(t *testing.T) {
	guidance := guidanceFixture(t, []readmetargetscout.GuidanceDocument{
		{Path: "AGENTS.md", Kind: readmetargetscout.GuidanceAgents, Content: "A different part of the repository."},
		{Path: "README.md", Kind: readmetargetscout.GuidanceReadme, Content: "The author describes this part."},
	})
	provider := &partialDocumentationProvider{raw: []byte(`{}`), separateSources: true}
	executor := llm.Executor{BatchConcurrency: 2, Enabled: true, RootDir: t.TempDir()}
	for range 2 {
		result, err := Run(t.Context(), executor, provider, guidance)
		if err != nil || result.Overview != "" || len(result.Sources) != 1 || result.Sources[0].Path != "README.md" {
			t.Fatalf("refused source erased sibling or supplied complete overview: %+v / %v", result, err)
		}
		if err := result.ValidateAgainst(guidance); err != nil {
			t.Fatal(err)
		}
	}
	if provider.calls != 3 {
		t.Fatalf("malformed source was cached or good sibling was repeated: %d calls", provider.calls)
	}
}

func TestDocumentationRefusedMergeKeepsOriginalConcepts(t *testing.T) {
	_, candidates, authority := packingEvidence(2)
	provider := &partialDocumentationProvider{raw: []byte(`{}`)}
	result, err := mergeTournament(t.Context(), llm.Executor{}, provider, "guidance", authority, candidates)
	if err != nil || len(result) != 2 || provider.calls != 1 {
		t.Fatalf("merge erased originals or retried: %+v / %v / %d", result, err, provider.calls)
	}
	combined := joinReductions(result)
	if combined.overview != "" || len(combined.sources) != 2 {
		t.Fatalf("invented merged overview or lost sources: %+v", combined)
	}
}

func TestDocumentationRefusedMergeSingletonDoesNotBecomeWholeOverview(t *testing.T) {
	_, candidates, authority := packingEvidence(2)
	provider := &partialDocumentationProvider{raw: []byte(`{}`), separateMerge: true}
	result, err := mergeTournament(t.Context(), llm.Executor{BatchConcurrency: 2}, provider, "guidance", authority, candidates)
	if err != nil || len(result) != 1 || provider.calls != 2 {
		t.Fatalf("failed merge erased original or retried: %+v / %v", result, err)
	}
	combined := joinReductions(result)
	if combined.overview != "" || len(combined.sources) != 1 || combined.sources[0].Ref != "d0001" {
		t.Fatalf("partial retained source became whole overview: %+v", combined)
	}
}

func TestDocumentationMergeUnlocatedMalformedSourceKeepsOriginalContext(t *testing.T) {
	_, candidates, authority := packingEvidence(2)
	for i := range candidates {
		candidates[i].sources[0].Concepts[0] = strings.TrimSpace(candidates[i].sources[0].Concepts[0])
	}
	provider := &partialDocumentationProvider{raw: []byte(`{"overview":"A new partial overview.","sources":[
		{"ref":42,"concepts":["An unlocatable concept."]},
		{"ref":"d0002","concepts":["Accepted reduction of second source."]}]}`)}
	result, err := mergeTournament(t.Context(), llm.Executor{}, provider, "guidance", authority, candidates)
	if err != nil || len(result) != 1 || provider.calls != 1 {
		t.Fatalf("partial merge failed: %+v / %v", result, err)
	}
	if result[0].overview != "" || len(result[0].sources) != 2 {
		t.Fatalf("unlocated failure erased context or supplied whole overview: %+v", result)
	}
	for i, original := range candidates {
		found := false
		for _, concept := range result[0].sources[i].Concepts {
			if concept == original.sources[0].Concepts[0] {
				found = true
			}
			if concept == "An unlocatable concept." {
				t.Fatal("unknown identity acquired source authority")
			}
		}
		if !found {
			t.Fatal("accepted original concept disappeared after unlocatable malformed source")
		}
	}
}

func TestDocumentationPartialMergeKeepsOriginalsOnlyForRefusedSource(t *testing.T) {
	_, candidates, authority := packingEvidence(2)
	for i := range candidates {
		candidates[i].sources[0].Concepts[0] = strings.TrimSpace(candidates[i].sources[0].Concepts[0])
	}
	provider := &partialDocumentationProvider{raw: []byte(`{"overview":"","sources":[
		{"ref":"d0001","concepts":[42,"New accepted concept."]},
		{"ref":"d0002","concepts":["Accepted reduction of second source."]}]}`)}
	result, err := mergeTournament(t.Context(), llm.Executor{}, provider, "guidance", authority, candidates)
	if err != nil || len(result) != 1 || provider.calls != 1 {
		t.Fatalf("partial merge failed: %+v / %v", result, err)
	}
	sources := result[0].sources
	if len(sources) != 2 || len(sources[0].Concepts) != 2 || len(sources[1].Concepts) != 1 || sources[1].Concepts[0] != "Accepted reduction of second source." {
		t.Fatalf("discarded refused original or overwrote good reduction: %+v", sources)
	}
	found := false
	for _, concept := range sources[0].Concepts {
		if concept == candidates[0].sources[0].Concepts[0] {
			found = true
		}
	}
	if !found {
		t.Fatal("already accepted source concept disappeared after malformed merge concept")
	}
}
