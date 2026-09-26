package targetportfolio

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/llm"
)

// scriptedPortfolioProvider answers each classification batch and default
// comparison from the exact request it received. A classification or default
// request is refused at preparation when separate or separateDefault says its
// candidates must not share a window, so Run forms independent batches.
type scriptedPortfolioProvider struct {
	exhaustivePortfolioProvider
	separate        func(Request) bool
	separateDefault func(DefaultRequest) bool
	classify        func(Request) string
	chooseDefault   func(DefaultRequest) string
	defaultFailure  error
	comparisons     atomic.Int64
}

const (
	classificationPromptPrefix = "Exact bounded classification-batch JSON:\n"
	classificationPromptSuffix = "\n\nEnd of quoted classification-batch JSON."
	defaultPromptPrefix        = "Exact bounded default-comparison JSON:\n"
	defaultPromptSuffix        = "\n\nEnd of quoted default-comparison JSON."
)

func (provider *scriptedPortfolioProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	var request Request
	if provider.separate != nil && strings.Contains(prompt.User, classificationPromptPrefix) &&
		decodePromptRequest(prompt.User, classificationPromptPrefix, classificationPromptSuffix, &request) == nil &&
		provider.separate(request) {
		return llm.Prepared{}, &llm.ResourceLimitError{Kind: llm.ResourceLimitRequestBytes, Limit: 1}
	}
	var comparison DefaultRequest
	if provider.separateDefault != nil && strings.Contains(prompt.User, defaultPromptPrefix) &&
		decodePromptRequest(prompt.User, defaultPromptPrefix, defaultPromptSuffix, &comparison) == nil &&
		provider.separateDefault(comparison) {
		return llm.Prepared{}, &llm.ResourceLimitError{Kind: llm.ResourceLimitRequestBytes, Limit: 1}
	}
	return provider.exhaustivePortfolioProvider.Prepare(prompt, limits)
}

func (provider *scriptedPortfolioProvider) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	provider.calls.Add(1)
	text := string(prepared.Bytes())
	var response string
	if strings.Contains(text, defaultPromptPrefix) {
		provider.comparisons.Add(1)
		if provider.defaultFailure != nil {
			return llm.Completion{}, provider.defaultFailure
		}
		var request DefaultRequest
		if err := decodePromptRequest(text, defaultPromptPrefix, defaultPromptSuffix, &request); err != nil {
			return llm.Completion{}, err
		}
		response = provider.chooseDefault(request)
	} else {
		var request Request
		if err := decodePromptRequest(text, classificationPromptPrefix, classificationPromptSuffix, &request); err != nil {
			return llm.Completion{}, err
		}
		response = provider.classify(request)
	}
	return llm.Completion{
		Response: []byte(response), FinishReason: llm.FinishStop, ChoiceCount: 1,
		Metrics: llm.Metrics{Attempts: 1, ProviderResponseBytes: len(response)},
	}, nil
}

// twoNativeCompilation has two exact native targets (t1 in f1, t2 in f2) and
// one guidance-only candidate, f3.
func twoNativeCompilation(t *testing.T) Compilation {
	t.Helper()
	snapshot := testSnapshot(t, []string{"api.py", "worker.py", "zz_guide.py"})
	rows := []NativeCandidate{
		{Ref: "t1", FileRef: "f1", Language: "python", Kind: "executable", Name: "api"},
		{Ref: "t2", FileRef: "f2", Language: "python", Kind: "executable", Name: "worker"},
	}
	compilation, err := CompileWithNativeAuthority(snapshot, []Candidate{
		{FileRef: "f1", Hypotheses: []string{"native api"}},
		{FileRef: "f2", Hypotheses: []string{"native worker"}},
		{FileRef: "f3", Hypotheses: []string{"guide names this entry"}},
	}, []corpus.FileID{"f1", "f2"}, rows)
	if err != nil {
		t.Fatal(err)
	}
	return compilation
}

func requestHas(request Request, ref corpus.FileID) bool {
	return slices.ContainsFunc(request.Candidates, func(candidate VisibleCandidate) bool { return candidate.FileRef == ref })
}

func TestRefusedClassificationBatchKeepsItsSiblingAndItsOwnAuthority(t *testing.T) {
	provider := &scriptedPortfolioProvider{
		// f1 is classified alone; f2 and f3 share the second batch.
		separate: func(request Request) bool { return requestHas(request, "f1") && len(request.Candidates) > 1 },
		classify: func(request Request) string {
			if requestHas(request, "f1") {
				return `{"default_file_ref":"f1","target_file_refs":["f1"],"native_decisions":[{"ref":"t1","decision":"standalone"}]}`
			}
			return `["f2","f3"]`
		},
		chooseDefault: func(request DefaultRequest) string {
			return `{"default_file_ref":"` + string(request.Candidates[0].FileRef) + `","reason":"the api is the product"}`
		},
	}
	execution, err := Run(t.Context(), llm.Executor{Enabled: false, BatchConcurrency: 2}, provider, twoNativeCompilation(t))
	if err != nil {
		t.Fatalf("one refused classification answer ended the stage: %v", err)
	}
	selection := execution.Selection
	if !slices.Equal(candidateRefs(selection.Targets), []corpus.FileID{"f1", "f2"}) ||
		!slices.Equal(candidateRefs(selection.Unclassified), []corpus.FileID{"f3"}) {
		t.Fatalf("sibling targets or refused batch's required ref lost, or guidance promoted: %#v", selection)
	}
	if selection.Default == nil || selection.Default.FileRef != "f1" || provider.comparisons.Load() != 1 {
		t.Fatalf("default not chosen by the closed comparison: %#v after %d comparisons", selection.Default, provider.comparisons.Load())
	}
	if len(selection.Placements) != 2 ||
		selection.Placements[0].Decision != "standalone" || selection.Placements[0].Reason != "" ||
		selection.Placements[1].Decision != "standalone" ||
		!strings.Contains(selection.Placements[1].Reason, "classification answer was refused") {
		t.Fatalf("placements = %+v", selection.Placements)
	}
	if len(execution.Outcomes) != 3 {
		t.Fatalf("outcomes = %d, want two classifications and one comparison", len(execution.Outcomes))
	}
	refused := 0
	for _, outcome := range execution.Outcomes {
		for _, rejection := range outcome.ResponseRejections {
			if rejection.Kind == "response_validation" {
				refused++
			}
		}
	}
	if refused != 1 {
		t.Fatalf("refused classification answer not journaled: %+v", execution.Outcomes)
	}
}

func TestPortfolioAnswerExtraFieldsAndUnusableDefaultsAreDiscarded(t *testing.T) {
	compilation := twoNativeCompilation(t)
	for name, test := range map[string]struct {
		raw          string
		wantTargets  []corpus.FileID
		wantDefault  corpus.FileID
		wantRejected int
	}{
		"extra fields": {
			raw:         `{"reasoning":"both are products","default_file_ref":"f2","target_file_refs":["f1","f2","f3"],"native_decisions":[{"ref":"t1","decision":"standalone","why":"api"},{"ref":"t2","decision":"standalone"}]}`,
			wantTargets: []corpus.FileID{"f1", "f2", "f3"}, wantDefault: "f2",
		},
		"unadvertised default": {
			raw:         `{"default_file_ref":"f99","target_file_refs":["f1","f2"]}`,
			wantTargets: []corpus.FileID{"f1", "f2"}, wantRejected: 1,
		},
		"default outside targets": {
			raw:         `{"default_file_ref":"f3","target_file_refs":["f1","f2"]}`,
			wantTargets: []corpus.FileID{"f1", "f2"}, wantRejected: 1,
		},
		"missing default": {
			raw:         `{"target_file_refs":["f1","f2"]}`,
			wantTargets: []corpus.FileID{"f1", "f2"},
		},
		"non-text members": {
			raw:         `{"default_file_ref":7,"target_file_refs":["f1",{"ref":"f3"},3],"native_decisions":"standalone"}`,
			wantTargets: []corpus.FileID{"f1", "f2"}, wantRejected: 4,
		},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := resolveResponse(compilation, []byte(test.raw))
			if err != nil {
				t.Fatalf("harmless answer refused: %v", err)
			}
			if !slices.Equal(candidateRefs(result.Targets), test.wantTargets) {
				t.Fatalf("targets = %v, want %v", candidateRefs(result.Targets), test.wantTargets)
			}
			if (test.wantDefault == "") != (result.Default == nil) ||
				(result.Default != nil && result.Default.FileRef != test.wantDefault) {
				t.Fatalf("default = %#v, want %q", result.Default, test.wantDefault)
			}
			if len(result.ResponseRejections()) != test.wantRejected {
				t.Fatalf("rejections = %+v, want %d", result.ResponseRejections(), test.wantRejected)
			}
		})
	}
}

func TestDiscardedDefaultIsChosenByTheSingleEligibleRuleOrTheComparison(t *testing.T) {
	answer := func(defaultRef string) func(Request) string {
		return func(Request) string {
			return `{"default_file_ref":` + defaultRef + `,"target_file_refs":["f1","f2"]}`
		}
	}
	for name, defaultRef := range map[string]string{"unadvertised": `"f99"`, "outside targets": `"f3"`, "missing": `null`} {
		t.Run(name, func(t *testing.T) {
			provider := &scriptedPortfolioProvider{
				classify: answer(defaultRef),
				chooseDefault: func(request DefaultRequest) string {
					return `{"default_file_ref":"` + string(request.Candidates[1].FileRef) + `"}`
				},
			}
			execution, err := Run(t.Context(), llm.Executor{Enabled: false}, provider, twoNativeCompilation(t))
			if err != nil {
				t.Fatalf("unusable default ended the stage: %v", err)
			}
			if execution.Selection.Default == nil || execution.Selection.Default.FileRef != "f2" ||
				provider.comparisons.Load() != 1 || slices.Contains(candidateRefs(execution.Selection.Targets), "f3") {
				t.Fatalf("default = %#v after %d comparisons, targets %v", execution.Selection.Default,
					provider.comparisons.Load(), candidateRefs(execution.Selection.Targets))
			}
		})
	}

	// With one eligible target the discarded default needs no comparison.
	single := &scriptedPortfolioProvider{
		classify: func(Request) string {
			return `{"default_file_ref":"f99","target_file_refs":["f1"]}`
		},
	}
	snapshot := testSnapshot(t, []string{"api.py", "zz_guide.py"})
	compilation, err := CompileWithRequiredTargetAuthority(snapshot, []Candidate{
		{FileRef: "f1", Hypotheses: []string{"native api"}},
		{FileRef: "f2", Hypotheses: []string{"guide names this entry"}},
	}, []corpus.FileID{"f1"})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := Run(t.Context(), llm.Executor{Enabled: false}, single, compilation)
	if err != nil || execution.Selection.Default == nil || execution.Selection.Default.FileRef != "f1" ||
		single.comparisons.Load() != 0 {
		t.Fatalf("single eligible default = %#v / %v after %d comparisons", execution.Selection.Default, err, single.comparisons.Load())
	}
}

func TestDefaultComparisonAcceptsExtraFieldsButNotAnUnknownRef(t *testing.T) {
	compilation := twoNativeCompilation(t)
	batch, err := compileDefaultBatch(compilation, []corpus.FileID{"f1", "f2"})
	if err != nil {
		t.Fatal(err)
	}
	selection, err := batch.resolve([]byte(`{"default_file_ref":"f2","reason":"the worker is the product"}`))
	if err != nil || selection.Default == nil || selection.Default.FileRef != "f2" {
		t.Fatalf("comparison with a reason = %#v / %v", selection.Default, err)
	}
	for _, raw := range []string{`{"default_file_ref":"f3"}`, `{"default_file_ref":"f99"}`, `{"reason":"none"}`, `{"default_file_ref":2}`, `null`} {
		if _, err := batch.resolve([]byte(raw)); err == nil {
			t.Fatalf("comparison accepted %s", raw)
		}
	}
}

// Owner decision 2026-09-26: a comparison answer naming an unknown ref, or
// none, does not end the run. The targets stay, the default is unresolved and
// nothing picks one in its place.
func TestRefusedDefaultComparisonLeavesTheDefaultUnresolved(t *testing.T) {
	for name, answer := range map[string]string{
		"unknown ref":          `{"default_file_ref":"f3"}`,
		"unadvertised ref":     `{"default_file_ref":"f99"}`,
		"missing ref":          `{"reason":"both are products"}`,
		"null ref":             `{"default_file_ref":null}`,
		"non-text ref":         `{"default_file_ref":2}`,
		"not an answer at all": `null`,
	} {
		t.Run(name, func(t *testing.T) {
			provider := &scriptedPortfolioProvider{
				classify: func(Request) string {
					return `{"target_file_refs":["f1","f2"],"native_decisions":[{"ref":"t1","decision":"standalone"},{"ref":"t2","decision":"tool"}]}`
				},
				chooseDefault: func(DefaultRequest) string { return answer },
			}
			execution, err := Run(t.Context(), llm.Executor{Enabled: false}, provider, twoNativeCompilation(t))
			if err != nil {
				t.Fatalf("a refused default comparison ended the stage: %v", err)
			}
			selection := execution.Selection
			if selection.Default != nil {
				t.Fatalf("default = %#v, want it unresolved", selection.Default)
			}
			if !slices.Equal(candidateRefs(selection.Targets), []corpus.FileID{"f1", "f2"}) ||
				!slices.Equal(candidateRefs(selection.Unclassified), []corpus.FileID{"f3"}) {
				t.Fatalf("targets or unclassified changed: %#v", selection)
			}
			if len(selection.Placements) != 2 || selection.Placements[0].Decision != "standalone" ||
				selection.Placements[1].Decision != "tool" {
				t.Fatalf("placements = %+v", selection.Placements)
			}
			if provider.comparisons.Load() != 1 || len(execution.Outcomes) != 2 ||
				len(execution.Outcomes[1].ResponseRejections) != 1 ||
				execution.Outcomes[1].ResponseRejections[0].Kind != "response_validation" {
				t.Fatalf("refused comparison not journaled: %d comparisons, outcomes %+v",
					provider.comparisons.Load(), execution.Outcomes)
			}
		})
	}
}

// A refused comparison in a round with several leaves the default unresolved
// even though a sibling comparison chose: that winner was never weighed
// against the refused comparison's candidates. No later round is asked.
func TestOneRefusedComparisonOfARoundLeavesTheDefaultUnresolved(t *testing.T) {
	snapshot := testSnapshot(t, []string{"a.py", "b.py", "c.py", "d.py"})
	compilation, err := CompileWithRequiredTargetAuthority(snapshot, []Candidate{
		{FileRef: "f1", Hypotheses: []string{"native a"}},
		{FileRef: "f2", Hypotheses: []string{"native b"}},
		{FileRef: "f3", Hypotheses: []string{"native c"}},
		{FileRef: "f4", Hypotheses: []string{"native d"}},
	}, []corpus.FileID{"f1", "f2", "f3", "f4"})
	if err != nil {
		t.Fatal(err)
	}
	provider := &scriptedPortfolioProvider{
		separateDefault: func(request DefaultRequest) bool { return len(request.Candidates) > 2 },
		classify:        func(Request) string { return `{"target_file_refs":[]}` },
		chooseDefault: func(request DefaultRequest) string {
			if request.Candidates[0].FileRef == "f1" {
				return `{"default_file_ref":"f99"}`
			}
			return `{"default_file_ref":"` + string(request.Candidates[0].FileRef) + `"}`
		},
	}
	execution, err := Run(t.Context(), llm.Executor{Enabled: false, BatchConcurrency: 2}, provider, compilation)
	if err != nil {
		t.Fatalf("one refused comparison ended the stage: %v", err)
	}
	if execution.Selection.Default != nil || len(execution.Selection.Targets) != 4 {
		t.Fatalf("selection = %#v, want four targets and an unresolved default", execution.Selection)
	}
	if provider.comparisons.Load() != 2 || len(execution.Outcomes) != 3 {
		t.Fatalf("comparisons = %d, outcomes = %d; want both first-round comparisons and no second round",
			provider.comparisons.Load(), len(execution.Outcomes))
	}
}

// Only a refused answer leaves the default unresolved; a provider failure is
// not an answer and still ends the stage.
func TestDefaultComparisonProviderFailureStillEndsTheStage(t *testing.T) {
	provider := &scriptedPortfolioProvider{
		classify:       func(Request) string { return `{"target_file_refs":["f1","f2"]}` },
		defaultFailure: errors.New("provider unavailable"),
	}
	if _, err := Run(t.Context(), llm.Executor{Enabled: false}, provider, twoNativeCompilation(t)); err == nil {
		t.Fatal("a failed default comparison call left the default unresolved instead of ending the stage")
	}
}

func TestMalformedNativeDecisionLosesOnlyItsOwnPlacement(t *testing.T) {
	c := launchGroupCompilation(t)
	raw := `{"default_file_ref":"f1","target_file_refs":["f1"],
		"launch_decisions":[{"ref":"g1","owner":"t1"},{"ref":"g2","owner":"t4"}],
		"native_decisions":[{"ref":"t6","decision":42},"t4",{"ref":7,"decision":"tool"}]}`
	result, err := resolveResponse(c, []byte(raw))
	if err != nil {
		t.Fatalf("malformed decision row refused its neighbours: %v", err)
	}
	if result.Placements[5].Decision != "standalone" || result.Placements[5].Reason == "" ||
		result.Placements[5].Rejected != "42" {
		t.Fatalf("a malformed decision became an answer: %+v", result.Placements[5])
	}
	for i, want := range []string{"standalone", "seed_of:t1", "seed_of:t1", "standalone", "seed_of:t4"} {
		if result.Placements[i].Decision != want || result.Placements[i].Reason != "" {
			t.Fatalf("neighbour %d = %+v, want %s", i, result.Placements[i], want)
		}
	}
	if len(result.ResponseRejections()) != 2 {
		t.Fatalf("unlocated decision rows not journaled: %+v", result.ResponseRejections())
	}

	for _, test := range []struct {
		decision string
		want     string
		accepted bool
	}{
		{`"Standalone "`, "standalone", true},
		{`" SHARED_CODE"`, "shared_code", true},
		{`"owner"`, "standalone", false},
		{`null`, "standalone", false},
		{`["tool"]`, "standalone", false},
	} {
		raw := `{"target_file_refs":[],"launch_decisions":[{"ref":"g1","owner":"t1"},{"ref":"g2","owner":"t4"}],"native_decisions":[{"ref":"t6","decision":` + test.decision + `}]}`
		result, err := resolveResponse(c, []byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		placement := result.Placements[5]
		if placement.Decision != test.want || (placement.Reason == "") != test.accepted {
			t.Fatalf("decision %s = %+v, want %s accepted=%v", test.decision, placement, test.want, test.accepted)
		}
	}
}

func TestNativeDecisionFoldsOnlyCaseAndSpace(t *testing.T) {
	for value, want := range map[string]string{
		"Standalone ":     "standalone",
		"TOOL":            "tool",
		" seed_of: t1 ":   "seed_of:t1",
		"Seed_Of:T1":      "seed_of:T1",
		"shared-code":     "shared-code",
		"example please":  "example please",
		`{"decision":42}`: `{"decision":42}`,
	} {
		if got := normalizeDecision(value); got != want {
			t.Fatalf("normalizeDecision(%q) = %q, want %q", value, got, want)
		}
	}
}
