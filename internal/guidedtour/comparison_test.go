package guidedtour

import (
	"reflect"
	"strings"
	"testing"
)

func TestEvaluateStoryCoverage(t *testing.T) {
	t.Parallel()

	bundle := testBundle(t)
	proposal := testProposal()
	proposal.Steps[2].BeatIDs = []string{"beat-4"}
	story, err := MaterializeStory(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := EvaluateStoryCoverage(bundle, story)
	if err != nil {
		t.Fatalf("EvaluateStoryCoverage() error = %v", err)
	}

	assertCoverageIDs(t, "referenced beats", coverage.ReferencedBeatIDs, []string{"beat-1", "beat-2", "beat-3", "beat-4"})
	assertCoverageIDs(t, "available beats", coverage.AvailableBeatIDs, []string{"beat-1", "beat-2", "beat-3", "beat-4"})
	assertCoverageIDs(t, "referenced components", coverage.ReferencedComponentIDs, []string{"component-1", "component-2"})
	assertCoverageIDs(t, "available components", coverage.AvailableComponentIDs, []string{"component-1", "component-2"})
	assertCoverageIDs(t, "reachable evidence", coverage.ReachableEvidenceIDs, []string{
		"evidence-1", "evidence-2", "evidence-3", "evidence-4", "evidence-shared",
	})
	assertCoverageIDs(t, "available evidence", coverage.AvailableEvidenceIDs, []string{
		"evidence-1", "evidence-2", "evidence-3", "evidence-4", "evidence-shared",
	})
	assertCoverageIDs(t, "referenced gaps", coverage.ReferencedGapIDs, []string{"gap-1"})
	assertCoverageIDs(t, "available gaps", coverage.AvailableGapIDs, []string{"gap-1"})
	if coverage.CandidateID != "trace-1" || coverage.Steps != 3 ||
		coverage.ReferencedBeats != 4 || coverage.AvailableBeats != 4 ||
		coverage.ReferencedComponents != 2 || coverage.AvailableComponents != 2 ||
		coverage.ReachableEvidence != 5 || coverage.AvailableEvidence != 5 ||
		coverage.ReferencedGaps != 1 || coverage.AvailableGaps != 1 {
		t.Fatalf("EvaluateStoryCoverage() counts = %#v", coverage)
	}

	reordered := cloneBundle(t, bundle)
	reverseComponents(reordered.Components)
	reverseBeats(reordered.Candidates[0].Beats)
	for index := range reordered.Candidates[0].Beats {
		reverseEvidence(reordered.Candidates[0].Beats[index].Evidence)
	}
	reorderedCoverage, err := EvaluateStoryCoverage(reordered, story)
	if err != nil {
		t.Fatalf("EvaluateStoryCoverage() reordered error = %v", err)
	}
	if !reflect.DeepEqual(reorderedCoverage, coverage) {
		t.Fatalf("EvaluateStoryCoverage() depends on input order\ngot:  %#v\nwant: %#v", reorderedCoverage, coverage)
	}
}

func TestEvaluateStoryCoverageReportsPartialSelection(t *testing.T) {
	t.Parallel()

	bundle := testBundle(t)
	proposal := testProposal()
	proposal.Steps = []ProposedStep{
		{Title: "Enter", Explanation: "Start at the exact entry", BeatIDs: []string{"beat-1"}},
		{Title: "Dispatch", Explanation: "Continue through dispatch", BeatIDs: []string{"beat-2"}},
		{Title: "Work", Explanation: "End at the selected behavior", BeatIDs: []string{"beat-3"}},
	}
	story, err := MaterializeStory(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := EvaluateStoryCoverage(bundle, story)
	if err != nil {
		t.Fatal(err)
	}
	assertCoverageIDs(t, "partial beats", coverage.ReferencedBeatIDs, []string{"beat-1", "beat-2", "beat-3"})
	assertCoverageIDs(t, "partial evidence", coverage.ReachableEvidenceIDs, []string{
		"evidence-1", "evidence-2", "evidence-3", "evidence-shared",
	})
	if coverage.ReferencedBeats != 3 || coverage.AvailableBeats != 4 ||
		coverage.ReferencedGaps != 1 || coverage.AvailableGaps != 1 {
		t.Fatalf("EvaluateStoryCoverage() partial counts = %#v", coverage)
	}
}

func TestEvaluateStoryCoverageRejectsNonMaterializedStory(t *testing.T) {
	t.Parallel()

	bundle := testBundle(t)
	story, err := MaterializeStory(bundle, testProposal())
	if err != nil {
		t.Fatal(err)
	}
	story.Steps[0].ComponentIDs = []string{"component-1"}
	if _, err := EvaluateStoryCoverage(bundle, story); err == nil ||
		!strings.Contains(err.Error(), "does not match") {
		t.Fatalf("EvaluateStoryCoverage() error = %v, want materialization mismatch", err)
	}
}

func TestComparisonValidate(t *testing.T) {
	t.Parallel()

	bundle := testBundle(t)
	story, err := MaterializeStory(bundle, testProposal())
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := EvaluateStoryCoverage(bundle, story)
	if err != nil {
		t.Fatal(err)
	}
	hash, _, err := BundleHash(bundle)
	if err != nil {
		t.Fatal(err)
	}
	valid := Comparison{
		Version:      ComparisonVersion,
		BundleSHA256: hash,
		Model:        "deepseek-chat",
		Profile:      "default",
		Variants: []StrategyMetrics{
			{
				Strategy: "monolithic", SemanticCalls: 1, RequestBytes: 100,
				ResponseBytes: 20, InputTokens: 30, OutputTokens: 10,
				WallMillis: 12, ValidationState: "accepted", Coverage: coverage,
			},
			{
				Strategy: "fanout", SemanticCalls: 3, CacheHits: 1, RequestBytes: 110,
				ResponseBytes: 30, InputTokens: 32, OutputTokens: 12, WallMillis: 9,
				LeafTasks: 3, LeafSucceeded: 2, LeafFailed: 1,
				ValidationState: "accepted", Coverage: coverage,
			},
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Comparison.Validate() raw error = %v", err)
	}
	valid.SelectedStrategy = "fanout"
	valid.Rationale = "Broader exact beat coverage at comparable token cost"
	if err := valid.Validate(); err != nil {
		t.Fatalf("Comparison.Validate() final error = %v", err)
	}

	tests := []struct {
		name    string
		mutate  func(*Comparison)
		wantErr string
	}{
		{
			name: "duplicate strategy",
			mutate: func(value *Comparison) {
				value.Variants[1].Strategy = value.Variants[0].Strategy
			},
			wantErr: "duplicate strategy",
		},
		{
			name: "unknown selected strategy",
			mutate: func(value *Comparison) {
				value.SelectedStrategy = "missing"
			},
			wantErr: "is not a variant",
		},
		{
			name: "unaccounted leaf",
			mutate: func(value *Comparison) {
				value.Variants[1].LeafFailed = 0
			},
			wantErr: "leaf outcome counts",
		},
		{
			name: "negative unsupported claims",
			mutate: func(value *Comparison) {
				value.Variants[0].UnsupportedClaims = -1
			},
			wantErr: "metrics cannot be negative",
		},
		{
			name: "coverage count mismatch",
			mutate: func(value *Comparison) {
				value.Variants[0].Coverage.ReferencedBeats++
			},
			wantErr: "count does not match",
		},
		{
			name: "unsorted ids",
			mutate: func(value *Comparison) {
				value.Variants[0].Coverage.AvailableBeatIDs[0], value.Variants[0].Coverage.AvailableBeatIDs[1] =
					value.Variants[0].Coverage.AvailableBeatIDs[1], value.Variants[0].Coverage.AvailableBeatIDs[0]
			},
			wantErr: "must be sorted",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			value := valid
			value.Variants = append([]StrategyMetrics{}, valid.Variants...)
			for index := range value.Variants {
				value.Variants[index].Coverage = cloneStoryCoverage(valid.Variants[index].Coverage)
			}
			test.mutate(&value)
			err := value.Validate()
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Comparison.Validate() error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func assertCoverageIDs(t *testing.T, field string, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %#v, want %#v", field, got, want)
	}
}

func cloneStoryCoverage(value StoryCoverage) StoryCoverage {
	value.ReferencedBeatIDs = append([]string{}, value.ReferencedBeatIDs...)
	value.AvailableBeatIDs = append([]string{}, value.AvailableBeatIDs...)
	value.ReferencedComponentIDs = append([]string{}, value.ReferencedComponentIDs...)
	value.AvailableComponentIDs = append([]string{}, value.AvailableComponentIDs...)
	value.ReachableEvidenceIDs = append([]string{}, value.ReachableEvidenceIDs...)
	value.AvailableEvidenceIDs = append([]string{}, value.AvailableEvidenceIDs...)
	value.ReferencedGapIDs = append([]string{}, value.ReferencedGapIDs...)
	value.AvailableGapIDs = append([]string{}, value.AvailableGapIDs...)
	return value
}
