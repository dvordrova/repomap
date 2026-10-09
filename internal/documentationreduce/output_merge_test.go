package documentationreduce

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
)

func TestDocumentationAcceptedMergeFramingOverflowKeepsOriginals(t *testing.T) {
	_, originals, authority := packingEvidence(2)
	for i := range originals {
		originals[i].sources[0].Concepts[0] = strings.Repeat(originals[i].sources[0].Concepts[0], 16)
	}
	real, transport := realDocumentationProvider(6000)
	got, err := mergeTournament(t.Context(), llm.Executor{}, real, "accepted", authority, originals)
	if err != nil || !reflect.DeepEqual(canonicalCandidates(originals), got) {
		t.Fatalf("accepted originals lost on framing overflow: %v", err)
	}
	if real.contextRefusals == 0 || real.preset.mergeCalls != 0 || transport.calls.Load() != 0 || joinReductions(got).overview != "" {
		t.Fatal("missing actual Prepare refusal, redundant completion, HTTP or invented overview")
	}
	// A genuine configuration/payment failure while preparing a merge remains fatal.
	cause := documentationPaymentFailure{}
	_, err = mergeTournament(t.Context(), llm.Executor{}, documentationPrepareFailure{cause}, "accepted", authority, originals)
	if !errors.Is(err, cause) {
		t.Fatalf("payment failure swallowed: %v", err)
	}
}

type outputMergeProvider struct {
	*contextDocumentationProvider
	mu                 sync.Mutex
	seen               map[string]int
	sourceWholeRefused bool
	mergeRefusals      int
	sourceAtom         bool
}

func (p *outputMergeProvider) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	p.contextDocumentationProvider.mu.Lock()
	local, known := p.ready[string(prepared.Bytes())]
	p.contextDocumentationProvider.mu.Unlock()
	if !known {
		panic("unbound preparation")
	}
	var prompt documentationPresetPrepared
	if err := json.Unmarshal(local.Bytes(), &prompt); err != nil {
		return llm.Completion{}, err
	}
	var request struct {
		Documents  []documentWire       `json:"documents"`
		Candidates []mergeCandidateWire `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(prompt.User), &request); err != nil {
		return llm.Completion{}, err
	}
	p.mu.Lock()
	if p.seen == nil {
		p.seen = map[string]int{}
	}
	p.seen[string(prepared.Bytes())]++
	fail := false
	if len(request.Documents) > 0 && (p.sourceAtom || !p.sourceWholeRefused && len(request.Documents) > 1) {
		p.sourceWholeRefused = true
		fail = true
	}
	if len(request.Candidates) > 0 {
		if len(request.Candidates) == 1 {
			p.mu.Unlock()
			panic("already accepted singleton called provider")
		}
		p.mergeRefusals++
		fail = true
	}
	p.mu.Unlock()
	if fail {
		return llm.Completion{}, llm.NewResourceLimitError(llm.ResourceLimitError{Kind: llm.ResourceLimitOutputTokens, Limit: 128000})
	}
	return p.contextDocumentationProvider.Complete(ctx, prepared)
}

func TestSavedPublicDocumentationOutputRefusalKeepsAcceptedSingletons(t *testing.T) {
	sourceWire, err := os.ReadFile("testdata/repomap-public-documentation-source.json")
	if err != nil {
		t.Fatal(err)
	}
	var source sourceRequest
	if err := json.Unmarshal(sourceWire, &source); err != nil {
		t.Fatal(err)
	}
	docs := make([]readmetargetscout.GuidanceDocument, len(source.Documents))
	for i, d := range source.Documents {
		docs[i] = readmetargetscout.GuidanceDocument{Path: d.Path, Kind: d.Kind, Content: d.Content}
	}
	guidance := guidanceFixture(t, docs)
	for _, cached := range []bool{false, true} {
		t.Run(map[bool]string{false: "cold", true: "warm"}[cached], func(t *testing.T) {
			real, transport := realDocumentationProvider(1000000)
			real.client.Endpoint = "https://api.deepseek.com/chat/completions"
			real.client.Model = "deepseek-v4-flash"
			real.client.MaxTokens = 128000
			p := &outputMergeProvider{contextDocumentationProvider: real}
			executor := llm.Executor{Enabled: cached, RootDir: t.TempDir(), BatchConcurrency: 2}
			runs := 1
			if cached {
				runs = 2
			}
			for range runs {
				result, err := Run(t.Context(), executor, p, guidance)
				if err != nil {
					t.Fatal(err)
				}
				if result.Overview != "" || len(result.Sources) != len(docs) {
					t.Fatalf("independent accepted source reductions/global overview: %+v", result)
				}
				if err := result.ValidateAgainst(guidance); err != nil {
					t.Fatal(err)
				}
			}
			if p.mergeRefusals != 1 || len(p.seen) != 4 {
				t.Fatalf("unexpected calls: merge refusals=%d distinct=%d", p.mergeRefusals, len(p.seen))
			}
			for _, count := range p.seen {
				if count != 1 {
					t.Fatal("identical request repeated")
				}
			}
			real.preset.assertLossless(t, guidance)
			if transport.calls.Load() != 0 {
				t.Fatal("HTTP")
			}
		})
	}
	// The exact accepted one-candidate projection that failed live needs no
	// completion or semantic concatenation; its source statements survive.
	mergeWire, err := os.ReadFile("testdata/repomap-public-documentation-merge.json")
	if err != nil {
		t.Fatal(err)
	}
	singletonWire, err := os.ReadFile("testdata/repomap-public-documentation-singleton.json")
	if err != nil {
		t.Fatal(err)
	}
	var parent, child mergeRequest
	if json.Unmarshal(mergeWire, &parent) != nil || json.Unmarshal(singletonWire, &child) != nil {
		t.Fatal("saved complete merge")
	}
	if len(parent.Candidates) != 2 || len(child.Candidates) != 1 || !reflect.DeepEqual(parent.Candidates[0], child.Candidates[0]) {
		t.Fatal("saved singleton is not the exact already accepted parent contribution")
	}
}

func TestDocumentationOutputAtomStaysExplicit(t *testing.T) {
	guidance := guidanceFixture(t, []readmetargetscout.GuidanceDocument{{Path: "README.md", Kind: readmetargetscout.GuidanceReadme, Content: "α"}})
	real, transport := realDocumentationProvider(1000000)
	p := &outputMergeProvider{contextDocumentationProvider: real, sourceAtom: true}
	_, err := Run(t.Context(), llm.Executor{}, p, guidance)
	if err == nil || !strings.Contains(err.Error(), "output_tokens") || len(p.seen) != 1 {
		t.Fatalf("indivisible output refusal: %v / %d", err, len(p.seen))
	}
	if transport.calls.Load() != 0 {
		t.Fatal("HTTP")
	}
}

func TestDocumentationPreparedSingleMergeCarryPreservesExactSources(t *testing.T) {
	real, transport := realDocumentationProvider(1000000)
	real.client.Endpoint = "https://api.deepseek.com/chat/completions"
	real.client.Model = "deepseek-v4-flash"
	real.client.MaxTokens = 128000
	wire, err := os.ReadFile("testdata/repomap-public-documentation-merge.json")
	if err != nil {
		t.Fatal(err)
	}
	var request mergeRequest
	if err := json.Unmarshal(wire, &request); err != nil {
		t.Fatal(err)
	}
	var original []normalizedReduction
	authority := map[string]documentAuthority{}
	for _, candidate := range request.Candidates {
		original = append(original, normalizedReduction{overview: candidate.Overview, sources: cloneResponseSources(candidate.Sources)})
		for _, source := range candidate.Sources {
			authority[source.Ref] = documentAuthority{path: source.Ref + "/README.md", kind: readmetargetscout.GuidanceReadme}
		}
	}
	p := &outputMergeProvider{contextDocumentationProvider: real}
	got, err := mergeTournament(t.Context(), llm.Executor{}, p, "saved-original", authority, original)
	if err != nil || !reflect.DeepEqual(canonicalCandidates(original), got) {
		t.Fatalf("accepted original reductions changed: %v", err)
	}
	if p.mergeRefusals != 1 || len(p.seen) != 1 || joinReductions(got).overview != "" {
		t.Fatal("redundant singleton call or invented global overview")
	}
	if transport.calls.Load() != 0 {
		t.Fatal("HTTP")
	}
}
