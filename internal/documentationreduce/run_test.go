package documentationreduce

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
)

func TestRunLosslesslyBatchesAndConvergentlyReducesGuidance(t *testing.T) {
	guidance := guidanceFixture(t, []readmetargetscout.GuidanceDocument{
		{
			Path: "AGENTS.md", Kind: readmetargetscout.GuidanceAgents,
			Content: "The Order Ledger owns order reconciliation and operator workflows.\n",
		},
		{
			Path: "README.md", Kind: readmetargetscout.GuidanceReadme,
			Content: strings.Repeat("The service records orders for merchants and produces settlement reports. α\n", 180),
		},
	})
	provider := &documentationPresetProvider{maximumPreparedBytes: 4_500}

	result, err := Run(t.Context(), llm.Executor{
		Enabled: false, BatchConcurrency: 2, BatchController: &llm.BatchController{},
	}, provider, guidance)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := result.ValidateAgainst(guidance); err != nil {
		t.Fatalf("ValidateAgainst: %v", err)
	}
	if result.GuidanceSHA256 != guidance.SHA256 || result.ReductionSHA256 == "" {
		t.Fatalf("result digests = %q / %q", result.GuidanceSHA256, result.ReductionSHA256)
	}
	if result.Overview != "Order Ledger records and reconciles merchant orders." {
		t.Fatalf("overview = %q", result.Overview)
	}
	if len(result.Sources) != 2 || result.Sources[0].Path != "AGENTS.md" || result.Sources[1].Path != "README.md" {
		t.Fatalf("restored sources = %#v", result.Sources)
	}
	for _, source := range result.Sources {
		if !reflect.DeepEqual(source.Concepts, []string{"Order Ledger", "Settlement report"}) {
			t.Fatalf("canonical source = %#v", source)
		}
	}
	if provider.sourceCalls < 2 || provider.mergeCalls == 0 {
		t.Fatalf("preset calls source=%d merge=%d, want exhaustive source shards and a merge", provider.sourceCalls, provider.mergeCalls)
	}
	provider.assertLossless(t, guidance)

	snapshot, err := result.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	snapshot.Sources[0].Concepts[0] = "mutated"
	if result.Sources[0].Concepts[0] == "mutated" {
		t.Fatal("Snapshot aliased result")
	}
}

func TestRunAcceptsSparseEmptyReduction(t *testing.T) {
	guidance := guidanceFixture(t, []readmetargetscout.GuidanceDocument{{
		Path: "README.md", Kind: readmetargetscout.GuidanceReadme, Content: "Build notes only.\n",
	}})
	provider := &documentationPresetProvider{maximumPreparedBytes: 1 << 20, sparse: true}

	result, err := Run(context.Background(), llm.Executor{Enabled: false}, provider, guidance)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.GuidanceSHA256 != guidance.SHA256 || result.ReductionSHA256 == "" ||
		result.Overview != "" || result.Sources == nil || len(result.Sources) != 0 {
		t.Fatalf("sparse result = %#v", result)
	}
	if err := result.ValidateAgainst(guidance); err != nil {
		t.Fatalf("ValidateAgainst: %v", err)
	}
}

func TestRunEmptyGuidanceDoesNotRequireProvider(t *testing.T) {
	result, err := Run(context.Background(), llm.Executor{}, nil, readmetargetscout.GuidanceSnapshot{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.GuidanceSHA256 != "" || result.ReductionSHA256 == "" || result.Sources == nil || len(result.Sources) != 0 {
		t.Fatalf("empty result = %#v", result)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("Validate empty result: %v", err)
	}
}

func TestNormalizeResponseDiscardsUnknownRefsBeforeTheirValues(t *testing.T) {
	authority := map[string]documentAuthority{
		"d1": {path: "README.md", kind: readmetargetscout.GuidanceReadme},
	}
	result, err := normalizeResponse([]byte(`{
  "overview":"",
  "sources":[
    {"ref":"d999","concepts":null},
    {"ref":"d1","concepts":["Useful concept","Useful concept"]}
  ]
}`), authority)
	if err != nil {
		t.Fatalf("normalizeResponse: %v", err)
	}
	if len(result.sources) != 1 || len(result.sources[0].Concepts) != 1 || result.sources[0].Ref != "d1" {
		t.Fatalf("normalized response = %#v", result)
	}
}

func TestNormalizeResponseKeepsEveryValidConceptOfADocument(t *testing.T) {
	authority := map[string]documentAuthority{
		"d1": {path: "README.md", kind: readmetargetscout.GuidanceReadme},
		"d2": {path: "docs/README.md", kind: readmetargetscout.GuidanceReadme},
	}
	// Sixteen entries: the repeated leading concept counts once, so all
	// fifteen distinct concepts are kept.
	supplied := []string{"Zeta ledger", "Zeta ledger"}
	for index := range 14 {
		supplied = append(supplied, fmt.Sprintf("Concept %02d", index))
	}
	raw, err := json.Marshal(map[string]any{"overview": "Ledger.", "sources": []map[string]any{
		{"ref": "d1", "concepts": supplied},
		{"ref": "d2", "concepts": []string{"Kept"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := normalizeResponse(raw, authority)
	if err != nil {
		t.Fatalf("normalizeResponse: %v", err)
	}
	want := append([]string(nil), supplied[1:]...)
	sort.Strings(want)
	if len(result.sources) != 2 || !reflect.DeepEqual(result.sources[0].Concepts, want) || result.sources[0].Ref != "d1" {
		t.Fatalf("concepts = %#v, want all %d: %v", result.sources, len(want), want)
	}
	if len(result.rejected) != 0 || result.overview != "Ledger." || result.AcceptedRowKeys() != nil || len(result.refusedSources) != 0 {
		t.Fatalf("a long document lost concepts or acceptance: %+v", result)
	}
}

func TestNormalizeResponseAcceptsHarmlessFormsAndRefusesOnlyTheBadPart(t *testing.T) {
	authority := map[string]documentAuthority{
		"d1": {path: "README.md", kind: readmetargetscout.GuidanceReadme},
		"d2": {path: "docs/README.md", kind: readmetargetscout.GuidanceReadme},
	}
	for name, test := range map[string]struct {
		raw          string
		wantOverview string
		wantSources  map[string][]string
		wantAccepted []string
		wantRejected int
	}{
		"bare sources array": {
			raw:         `[{"ref":"d1","concepts":["Ledger"]}]`,
			wantSources: map[string][]string{"d1": {"Ledger"}},
		},
		"absent overview": {
			raw:         `{"sources":[{"ref":"d1","concepts":["Ledger"]}]}`,
			wantSources: map[string][]string{"d1": {"Ledger"}},
		},
		"null or absent concepts": {
			raw:          `{"overview":"Ledger service.","sources":[{"ref":"d1","concepts":null},{"ref":"d2"},{"ref":"d1","concepts":["Ledger"]}]}`,
			wantOverview: "Ledger service.", wantSources: map[string][]string{"d1": {"Ledger"}},
		},
		"one concept string": {
			raw:         `{"overview":"","sources":[{"ref":"d1","concepts":"  Ledger "}]}`,
			wantSources: map[string][]string{"d1": {"Ledger"}},
		},
		"one bad concept": {
			raw:         `{"overview":"","sources":[{"ref":"d1","concepts":[42,"Ledger"]},{"ref":"d2","concepts":["Order"]}]}`,
			wantSources: map[string][]string{"d1": {"Ledger"}, "d2": {"Order"}}, wantAccepted: []string{"d1", "d2"}, wantRejected: 1,
		},
		"row with only bad concepts": {
			raw:         `{"overview":"","sources":[{"ref":"d1","concepts":[42]},{"ref":"d2","concepts":["Order"]}]}`,
			wantSources: map[string][]string{"d2": {"Order"}}, wantAccepted: []string{"d2"}, wantRejected: 1,
		},
		"bad concepts field": {
			raw:         `{"overview":"","sources":[{"ref":"d1","concepts":{"name":"Ledger"}},{"ref":"d2","concepts":["Order"]}]}`,
			wantSources: map[string][]string{"d2": {"Order"}}, wantAccepted: []string{"d2"}, wantRejected: 1,
		},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := normalizeResponse([]byte(test.raw), authority)
			if err != nil {
				t.Fatalf("harmless form refused: %v", err)
			}
			got := make(map[string][]string)
			for _, source := range result.sources {
				got[source.Ref] = source.Concepts
			}
			if result.overview != test.wantOverview || !reflect.DeepEqual(got, test.wantSources) {
				t.Fatalf("result = %q %v, want %q %v", result.overview, got, test.wantOverview, test.wantSources)
			}
			if !reflect.DeepEqual(result.AcceptedRowKeys(), test.wantAccepted) || len(result.rejected) != test.wantRejected {
				t.Fatalf("accepted %v / rejected %+v, want %v / %d", result.AcceptedRowKeys(), result.rejected, test.wantAccepted, test.wantRejected)
			}
		})
	}
	for name, raw := range map[string]string{
		"missing sources":         `{"overview":"Ledger service."}`,
		"null sources":            `{"overview":"Ledger service.","sources":null}`,
		"sources object":          `{"overview":"Ledger service.","sources":{"d1":["Ledger"]}}`,
		"only bad concepts":       `{"overview":"","sources":[{"ref":"d1","concepts":[42,""]}]}`,
		"overview without source": `{"overview":"Ledger service.","sources":[]}`,
	} {
		if result, err := normalizeResponse([]byte(raw), authority); err == nil {
			t.Fatalf("%s accepted: %+v", name, result)
		}
	}
}

func TestPromptsKeepDocumentationUntrustedAndOutOfGraphClassification(t *testing.T) {
	for name, prompt := range map[string]string{"source": sourcePrompt, "merge": mergePrompt} {
		for _, fragment := range []string{"untrusted", "Do not", "entrypoints", "graph edges", "`sources`", "`concepts`", "12"} {
			if !strings.Contains(prompt, fragment) {
				t.Fatalf("%s prompt does not contain %q", name, fragment)
			}
		}
		if strings.Contains(prompt, "claim") {
			t.Fatalf("%s prompt still asks for claims", name)
		}
	}
	if strings.Contains(responseExample, "claims") {
		t.Fatalf("response example still shows claims: %s", responseExample)
	}
}

func TestSplitUTF8PrefersParagraphBoundaryAndIsLossless(t *testing.T) {
	original := "first α paragraph\n\nsecond β paragraph with more words"
	left, right, ok := splitUTF8(original)
	if !ok {
		t.Fatal("splitUTF8 did not split")
	}
	if left+right != original {
		t.Fatal("splitUTF8 did not preserve exact content")
	}
	if !strings.HasSuffix(left, "\n\n") {
		t.Fatalf("split boundary = %q | %q, want paragraph boundary", left, right)
	}
}

type capturedDocumentPart struct {
	ordinal int
	count   int
	content string
}

type documentationPresetProvider struct {
	maximumPreparedBytes int
	sparse               bool

	mu          sync.Mutex
	sourceCalls int
	mergeCalls  int
	parts       map[string][]capturedDocumentPart
}

type documentationPresetPrepared struct {
	System string `json:"system"`
	User   string `json:"user"`
}

func (provider *documentationPresetProvider) State() []byte {
	return []byte(`{"provider":"documentation-reduce-preset-v1"}`)
}

func (provider *documentationPresetProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	if !prompt.ResponseFormatJSON || prompt.System == "" || prompt.User == "" ||
		!strings.HasSuffix(prompt.System, prompt.ResponseExample) || strings.Count(prompt.System, `"overview"`) != 1 ||
		limits.MaxRequestBytes != llm.SemanticRecordByteLimit ||
		limits.MaxResponseBytes != llm.ProviderResponseByteLimit || limits.MaxOutputTokens != maxOutputTokens {
		return llm.Prepared{}, fmt.Errorf("preset received invalid request contract")
	}
	wire, err := json.Marshal(documentationPresetPrepared{System: prompt.System, User: prompt.User})
	if err != nil {
		return llm.Prepared{}, err
	}
	if provider.maximumPreparedBytes > 0 && len(wire) > provider.maximumPreparedBytes {
		return llm.Prepared{}, llm.NewResourceLimitError(llm.ResourceLimitError{
			Stage: "preset_prepare", Kind: llm.ResourceLimitRequestBytes,
			Limit: provider.maximumPreparedBytes, Observed: len(wire), ObservedKnown: true,
		})
	}
	return llm.NewPrepared(wire)
}

func (provider *documentationPresetProvider) Complete(
	_ context.Context,
	prepared llm.Prepared,
) (llm.Completion, error) {
	var prompt documentationPresetPrepared
	if err := json.Unmarshal(prepared.Bytes(), &prompt); err != nil {
		return llm.Completion{}, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(prompt.User), &raw); err != nil {
		return llm.Completion{}, err
	}
	var response modelResponse
	switch {
	case raw["documents"] != nil:
		if !strings.Contains(prompt.System, "\n\n"+strings.TrimSpace(sourcePrompt)) || !strings.Contains(prompt.System, "prose in English.") {
			return llm.Completion{}, fmt.Errorf("preset source prompt mismatch")
		}
		var request sourceRequest
		if err := json.Unmarshal([]byte(prompt.User), &request); err != nil {
			return llm.Completion{}, err
		}
		if request.ContentTrust != "untrusted_repository_text" || len(request.Documents) == 0 {
			return llm.Completion{}, fmt.Errorf("preset source authority mismatch")
		}
		provider.captureSourceRequest(request)
		if provider.sparse {
			response = modelResponse{Sources: []responseSource{}}
			break
		}
		response.Overview = "Order Ledger records and reconciles merchant orders."
		seen := make(map[string]struct{})
		for _, document := range request.Documents {
			if _, duplicate := seen[document.Ref]; duplicate {
				continue
			}
			seen[document.Ref] = struct{}{}
			response.Sources = append(response.Sources,
				responseSource{
					Ref:      document.Ref,
					Concepts: []string{"Order Ledger", "Order Ledger", "Settlement report"},
				},
				responseSource{
					Ref:      document.Ref,
					Concepts: []string{"Order Ledger"},
				},
			)
		}
		response.Sources = append(response.Sources, responseSource{Ref: "d999"})
	case raw["candidates"] != nil:
		if !strings.Contains(prompt.System, "\n\n"+strings.TrimSpace(mergePrompt)) || !strings.Contains(prompt.System, "prose in English.") {
			return llm.Completion{}, fmt.Errorf("preset merge prompt mismatch")
		}
		var request mergeRequest
		if err := json.Unmarshal([]byte(prompt.User), &request); err != nil {
			return llm.Completion{}, err
		}
		if request.ContentTrust != "untrusted_repository_summary" || len(request.Candidates) == 0 {
			return llm.Completion{}, fmt.Errorf("preset merge authority mismatch")
		}
		provider.mu.Lock()
		provider.mergeCalls++
		provider.mu.Unlock()
		response.Overview = "Order Ledger records and reconciles merchant orders."
		refs := make(map[string]struct{})
		for _, candidate := range request.Candidates {
			for _, source := range candidate.Sources {
				refs[source.Ref] = struct{}{}
			}
		}
		ordered := make([]string, 0, len(refs))
		for ref := range refs {
			ordered = append(ordered, ref)
		}
		sort.Strings(ordered)
		for _, ref := range ordered {
			response.Sources = append(response.Sources, responseSource{
				Ref: ref, Concepts: []string{"Order Ledger", "Settlement report"},
			})
		}
	default:
		return llm.Completion{}, fmt.Errorf("preset received unknown request")
	}
	wire, err := json.Marshal(response)
	if err != nil {
		return llm.Completion{}, err
	}
	return llm.Completion{
		Response: wire, FinishReason: llm.FinishStop, ChoiceCount: 1,
		Metrics: llm.Metrics{Attempts: 1},
	}, nil
}

func (provider *documentationPresetProvider) captureSourceRequest(request sourceRequest) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.sourceCalls++
	if provider.parts == nil {
		provider.parts = make(map[string][]capturedDocumentPart)
	}
	for _, document := range request.Documents {
		provider.parts[document.Path] = append(provider.parts[document.Path], capturedDocumentPart{
			ordinal: document.Part.Ordinal, count: document.Part.Count, content: document.Content,
		})
	}
}

func (provider *documentationPresetProvider) assertLossless(
	t *testing.T,
	guidance readmetargetscout.GuidanceSnapshot,
) {
	t.Helper()
	provider.mu.Lock()
	defer provider.mu.Unlock()
	for _, document := range guidance.Documents {
		parts := append([]capturedDocumentPart(nil), provider.parts[document.Path]...)
		sort.Slice(parts, func(i, j int) bool { return parts[i].ordinal < parts[j].ordinal })
		var joined strings.Builder
		for position, part := range parts {
			if part.ordinal != position+1 || part.count != len(parts) {
				t.Fatalf("%s part %d = %d/%d, total %d", document.Path, position, part.ordinal, part.count, len(parts))
			}
			joined.WriteString(part.content)
		}
		if joined.String() != document.Content {
			t.Fatalf("%s content was not covered losslessly", document.Path)
		}
	}
}

func guidanceFixture(
	t *testing.T,
	documents []readmetargetscout.GuidanceDocument,
) readmetargetscout.GuidanceSnapshot {
	t.Helper()
	wire, err := json.Marshal(documents)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(wire)
	result := readmetargetscout.GuidanceSnapshot{
		SHA256: hex.EncodeToString(digest[:]), Documents: documents,
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("guidance fixture: %v", err)
	}
	return result
}
