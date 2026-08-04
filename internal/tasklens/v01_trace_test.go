package tasklens

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestRetrievalTraceValidateAgainstBundleRejectsProjectionDrift(t *testing.T) {
	repo := newTaskLensTestRepo(t, "trace-bundle-binding")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and TestCopyConfigEnabled.",
	})
	if err != nil {
		t.Fatal(err)
	}
	trace := bundle.LocalTrace
	if err := trace.ValidateAgainstBundle(bundle); err != nil {
		t.Fatalf("ValidateAgainstBundle() error = %v", err)
	}
	trace.TaskTerms[0].Found = !trace.TaskTerms[0].Found
	if err := trace.ValidateAgainstBundle(bundle); err == nil ||
		!strings.Contains(err.Error(), "task terms differ") {
		t.Fatalf("tampered trace error = %v", err)
	}
}

func TestRetrievalTraceValidateAndRenderCanonicalMarkdown(t *testing.T) {
	t.Parallel()

	trace := validRetrievalTrace(t)
	first, err := RenderRetrievalTraceMarkdown(trace)
	if err != nil {
		t.Fatalf("RenderRetrievalTraceMarkdown() error = %v", err)
	}
	if !strings.Contains(first, "## Source-scope completeness") ||
		!strings.Contains(first, "exact_existing_test") ||
		!strings.Contains(first, "Caused loss") {
		t.Fatalf("rendered Markdown is missing required sections:\n%s", first)
	}

	trace.TaskTerms[0], trace.TaskTerms[1] = trace.TaskTerms[1], trace.TaskTerms[0]
	components := trace.CandidatesBeforeRanking[0].ScoreComponents
	components[0], components[1] = components[1], components[0]
	trace.Limits[0], trace.Limits[1] = trace.Limits[1], trace.Limits[0]
	second, err := RenderRetrievalTraceMarkdown(trace)
	if err != nil {
		t.Fatalf("RenderRetrievalTraceMarkdown(shuffled) error = %v", err)
	}
	if second != first {
		t.Fatalf("canonical Markdown changed after set-like input reordering\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestRetrievalTraceRejectsScoreMismatch(t *testing.T) {
	t.Parallel()

	trace := validRetrievalTrace(t)
	trace.CandidatesBeforeRanking[0].Score++
	if err := trace.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want score-component mismatch")
	}
}

func TestRetrievalTraceRejectsCandidateWithoutOutcome(t *testing.T) {
	t.Parallel()

	trace := validRetrievalTrace(t)
	trace.CandidatesBeforeRanking = append(trace.CandidatesBeforeRanking, RetrievalCandidate{
		ID:             "candidate-dropped",
		Stage:          RetrievalStageInitial,
		DiscoveryOrder: 2,
		Path:           "pkg/other.go",
		Roles:          []AnchorRole{RoleRepresentativeImplementation},
		Score:          1,
		ScoreComponents: []RetrievalScoreComponent{
			{Kind: RetrievalScoreRepositoryRole, Value: 1},
		},
	})
	if err := trace.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing candidate outcome")
	}
}

func TestRetrievalTraceJSONRoundTrip(t *testing.T) {
	t.Parallel()

	input := validRetrievalTrace(t)
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var output RetrievalTrace
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatal(err)
	}
	if err := output.Validate(); err != nil {
		t.Fatalf("round-trip Validate() error = %v", err)
	}
	inputMarkdown, err := RenderRetrievalTraceMarkdown(input)
	if err != nil {
		t.Fatal(err)
	}
	outputMarkdown, err := RenderRetrievalTraceMarkdown(output)
	if err != nil {
		t.Fatal(err)
	}
	if outputMarkdown != inputMarkdown {
		t.Fatal("round-trip Markdown changed")
	}
}

func validRetrievalTrace(t *testing.T) RetrievalTrace {
	t.Helper()

	contract, err := DefaultRoleContract(TaskProfileUnknown)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := EvaluateRoleCoverage(contract, []Anchor{
		{
			ID:        "anchor-test",
			RoleHints: []AnchorRole{RoleRepresentativeImplementation},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return RetrievalTrace{
		Version:     RetrievalTraceVersion,
		TaskKind:    TaskBug,
		TaskProfile: TaskProfileUnknown,
		TaskTerms: []RetrievalTaskTerm{
			{Text: "status", Normalized: "status", Found: true, Weight: 8},
			{Text: "wrong", Normalized: "wrong", Found: false, Weight: 4},
		},
		CandidatesBeforeRanking: []RetrievalCandidate{
			{
				ID:             "candidate-test",
				Stage:          RetrievalStageInitial,
				DiscoveryOrder: 1,
				Path:           "pkg/handler_test.go",
				Symbol:         "TestHandlerStatus",
				Roles:          []AnchorRole{RoleRepresentativeImplementation},
				Score:          12,
				ScoreComponents: []RetrievalScoreComponent{
					{Kind: RetrievalScoreDirectTaskTerm, Value: 8, Detail: "exact status match"},
					{Kind: RetrievalScoreTestFixtureRelevance, Value: 4},
				},
			},
		},
		Relationships: []RetrievalRelationship{},
		SelectedAnchors: []RetrievalSelection{
			{
				CandidateID: "candidate-test",
				AnchorID:    "anchor-test",
				Rank:        1,
				Reason:      "reserved for the only represented supporting role",
			},
		},
		DroppedAnchors: []RetrievalDrop{},
		SourceScopes: []RetrievalSourceScope{
			{
				AnchorID: "anchor-test",
				Scope: SourceScope{
					ScopeKind:             SourceScopeCompleteEnclosingSymbol,
					ScopeStart:            10,
					ScopeEnd:              24,
					SourceTotalLines:      80,
					NegativeEvidenceBasis: NegativeEvidenceNone,
				},
			},
		},
		RoleCoverage: coverage,
		VerificationFrontier: VerificationFrontier{
			DecisiveAnchorID: "anchor-test",
			Anchors: []VerificationItem{
				{
					ID:          "verification-test",
					Authority:   VerificationExactExistingTest,
					AnchorID:    "anchor-test",
					Path:        "pkg/handler_test.go",
					Symbol:      "TestHandlerStatus",
					Text:        "Exact existing test asserts the task-relevant status.",
					EvidenceIDs: []string{"evidence-test"},
				},
			},
		},
		Budgets: Budgets{
			InitialCandidates:       1,
			CandidateItemsFound:     1,
			RetainedAnchors:         1,
			AnchorItemsFound:        1,
			EvidenceFilesConsidered: 1,
			ReadFiles:               1,
			ReadBytes:               512,
			RetainedSourceBytes:     256,
		},
		Limits: []RetrievalLimit{
			{
				Name:     "anchor_limit",
				Limit:    MaxRetainedAnchors,
				Observed: 1,
			},
			{
				Name:       "file_limit",
				Limit:      MaxReadFiles,
				Observed:   MaxReadFiles + 1,
				Applied:    true,
				CausedLoss: true,
				LossReason: "one lower-ranked candidate file was not read",
			},
		},
		GoldAssessment: &GoldAnchorAssessment{
			Disposition: GoldAnchorPresentBeforeRanking,
			CandidateID: "candidate-test",
			Detail:      "known decisive symbol was present before ranking",
		},
	}
}
