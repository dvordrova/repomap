package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/studymap"
)

func TestWritePublicationTraceExplainsBoundedReductionsWithoutProse(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	writeDecisionTraceFixture(t, filepath.Join(runDir, studymap.DirectionsAttemptFile), `{
  "version": 1,
  "direction_diagnostics": {
    "received": 6,
    "accepted": 4,
    "rejected": 2,
    "issues": [
      {"position": 1, "code": "invalid_reading_copy"},
      {"position": 5, "code": "invalid_anchor_selection"}
    ]
  },
  "raw_response": "MODEL PROSE /private/repository/path SHOULD NOT APPEAR"
}`)
	writeDecisionTraceFixture(t, filepath.Join(runDir, studyMapReviewsFile), `{
  "version": 1,
  "attempts": [
    {"validation_state": "accepted"},
    {"validation_state": "failed_provider", "failure_reason": "unexpected EOF /private/provider"},
    {"validation_state": "rejected", "issue_code": "model_direction_id"},
    {"validation_state": "canceled"}
  ],
  "reduction": {
    "version": 1,
    "proposed": 4,
    "reviewed": 3,
    "selected": 2,
    "issues": [
      {"direction_id": "private-id", "code": "review_missing", "detail": "MODEL DETAIL SHOULD NOT APPEAR"},
      {"direction_id": "another-private-id", "code": "question_scope_broader", "detail": "/private/path"}
    ]
  }
}`)

	research := modelresearch.NewState(
		modelresearch.DefaultPolicy(),
		modelresearch.RepositoryContext{Identity: "private identity"},
	)
	research.Rounds = []modelresearch.ResearchRound{
		{
			Status:                modelresearch.RoundCompleted,
			ValidatedFindings:     make([]modelresearch.ValidatedFinding, 3),
			RejectedFindings:      make([]modelresearch.RejectedFinding, 1),
			NewGroundedFactsCount: 4,
			UnresolvedFrontiers:   make([]modelresearch.Frontier, 2),
		},
		{Status: modelresearch.RoundNoNewEvidence},
	}
	research.SkippedRounds = []modelresearch.ResearchRound{{
		Status: modelresearch.RoundSkipped,
		Gate:   modelresearch.GateDecision{Reason: "targeted_round_limit"},
	}}
	data := &report.ReportData{
		CandidateDirections: []report.CandidateDirection{
			{Disposition: "accepted"},
			{Disposition: "accepted"},
			{Disposition: "rejected"},
		},
		Run: &report.RunInfo{
			CandidateDirectionCount: 3,
			AcceptedDirectionCount:  2,
			RejectedDirectionCount:  1,
		},
		ModelResearch: &research,
		DiscoveredSurfaces: &report.DiscoveredSurfaces{
			TotalCount:              12,
			ApplicationCount:        7,
			UnavailableSurfaceCount: 2,
			UnresolvedHandlerCount:  3,
			PackagesInspected:       8,
			FunctionsInspected:      240,
			PackageDiagnosticCount:  1,
			BudgetsReached:          []string{"detached_functions", "detached_functions"},
			Triggers:                make([]report.DiscoveredTrigger, 10),
		},
		ArchitectureSynthesis: &report.ArchitectureSynthesisStatus{
			State:                 report.ArchitectureSynthesisSucceeded,
			ProposalAccepted:      true,
			ProposalNormalized:    true,
			ArchitectureSource:    string(componentmap.SourceNormalizedModel),
			NormalizationCount:    2,
			ProviderCallSucceeded: true,
		},
		ArchitectureCanvas: &report.ArchitectureCanvas{
			ValidationOutcome:  componentmap.ValidationAcceptedNormalized,
			ArchitectureSource: componentmap.SourceNormalizedModel,
			BehaviorAnchors:    make([]componentmap.BehaviorAnchor, 5),
			Components: []report.ArchitectureComponent{
				{Members: make([]componentmap.Candidate, 2)},
				{Members: make([]componentmap.Candidate, 3)},
			},
			Subsystems: make([]report.ArchitectureSubsystem, 2),
			Surfaces:   make([]report.ArchitectureSurface, 7),
		},
		StudyPublication: &report.StudyPublicationStatus{
			Version: 1, State: "published", Candidates: 6, Selected: 2,
		},
		StudyMap: &report.RepositoryStudyMap{
			Shape:            make([]report.RepositoryStudyArea, 6),
			Directions:       make([]report.StudyDirection, 2),
			HiddenDirections: make([]report.StudyDirection, 1),
		},
		UserMechanisms: make([]report.UserMechanism, 1),
		UserTopics:     make([]report.UserTopic, 2),
		OpenablePaths:  make([]string, 19),
		Warnings:       []string{"warning with /private/path that must not be copied"},
	}

	var output bytes.Buffer
	writePublicationTrace(&output, runDir, data, true, "gitlab_static", 0)
	got := output.String()
	for _, want := range []string{
		"publication decision summary (bounded counts only): cache=disabled",
		"decision orientation: found=3 accepted=2 rejected=1 unresolved=0",
		"decision direction expansion: requested=0 eligible=2 expanded=0 not_expanded=2 state=not_requested",
		"decision targeted research: selected=2 skipped=1 validated_findings=3 rejected_findings=1 new_grounded_facts=4 unresolved_frontiers=2",
		"outcomes=completed=1,no_new_evidence=1 skip_reasons=targeted_round_limit=1",
		"decision surfaces: generic_scheduled=false found=12 published=10 hidden=2",
		"budgets=detached_functions=2",
		"decision architecture: anchors=5 members=5 grouped_components=2 groups=2 surfaces=7",
		"outcome=accepted_normalized source=normalized_model normalizations=2",
		"decision study shape: areas=6 canonical_directions=2 hidden_directions=1",
		"decision study drafts: received=6 accepted=4 rejected=2 reasons=invalid_anchor_selection=1,invalid_reading_copy=1",
		"decision study reviews: proposed=4 reviewed=3 rejected=1 selected=2 reduced_after_review=1",
		"reasons=question_scope_broader=1,review_missing=1",
		"decision study review attempts: outcomes=accepted=1,canceled=1,failed_provider=1,rejected=1 reasons=model_direction_id=1",
		"decision study publication: state=published failure=none candidates=6 selected=2 not_selected=4 published=2 hidden=1 projection=canonical",
		"decision publication: mechanisms=1 topics=2 study=2 guided_tour=0 architecture_components=2 architecture_surfaces=7",
		"source_targets=19 warnings=1 authority=gitlab_static",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("decision trace missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"MODEL PROSE",
		"MODEL DETAIL",
		"/private/",
		"private-id",
		"warning with",
	} {
		if strings.Contains(got, forbidden) {
			t.Errorf("decision trace leaked %q:\n%s", forbidden, got)
		}
	}
}

func TestFormatDecisionReasonCountsIsSortedAndBounded(t *testing.T) {
	t.Parallel()

	got := formatDecisionReasonCounts([]string{
		"z", "a", "b", "c", "d", "e", "f", "g", "unsafe code", strings.Repeat("x", 65),
	})
	if got != "a=1,b=1,c=1,d=1,e=1,f=1,other=4" {
		t.Fatalf("bounded reasons = %q", got)
	}
}

func TestWritePublicationTraceExplainsFailedStudyWithoutEchoingFailure(t *testing.T) {
	t.Parallel()

	data := &report.ReportData{
		StudyPublication: &report.StudyPublicationStatus{
			Version: 1,
			State:   "failed",
			FailureReason: "study map: reviewed selection has 1 direction; need at least 3 " +
				"/private/repository/path",
			Candidates: 4,
		},
	}
	var output bytes.Buffer
	writePublicationTrace(&output, t.TempDir(), data, false, "local", 0)
	got := output.String()
	if !strings.Contains(
		got,
		"decision study publication: state=failed failure=insufficient_reviewed_directions candidates=4 selected=0 not_selected=4 published=0 hidden=0 projection=none",
	) {
		t.Fatalf("failed Study trace is missing the stable reason:\n%s", got)
	}
	if strings.Contains(got, "/private/") || strings.Contains(got, "reviewed selection has") {
		t.Fatalf("failed Study trace leaked raw failure text:\n%s", got)
	}
}

func TestStudyDecisionArtifactsFailClosedOnInvalidCountsAndUnknownCodes(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	writeDecisionTraceFixture(t, filepath.Join(runDir, studymap.DirectionsAttemptFile), `{
  "version": 1,
  "direction_diagnostics": {
    "received": 999999,
    "accepted": 1,
    "rejected": 999998,
    "issues": [{"position": 0, "code": "token_like_but_not_local"}]
  }
}`)
	writeDecisionTraceFixture(t, filepath.Join(runDir, studyMapReviewsFile), `{
  "version": 1,
  "attempts": [
    {"validation_state": "future_state", "issue_code": "token_like_but_not_local"}
  ],
  "reduction": {
    "version": 1,
    "proposed": 2,
    "reviewed": 2,
    "selected": 1,
    "issues": [{"code": "token_like_but_not_local"}]
  }
}`)
	var output bytes.Buffer
	writePublicationTrace(&output, runDir, &report.ReportData{}, false, "local", 0)
	got := output.String()
	if !strings.Contains(got, "decision study drafts: artifact=invalid") {
		t.Fatalf("invalid direction counts were trusted:\n%s", got)
	}
	if !strings.Contains(got, "decision study reviews: proposed=2 reviewed=2 rejected=0 selected=1 reduced_after_review=1 reasons=unknown_code=1") {
		t.Fatalf("unknown review reason was not reduced:\n%s", got)
	}
	if !strings.Contains(got, "decision study review attempts: outcomes=unknown_state=1 reasons=unknown_code=1") {
		t.Fatalf("unknown attempt metadata was not reduced:\n%s", got)
	}
	if strings.Contains(got, "token_like_but_not_local") {
		t.Fatalf("unknown reason was echoed:\n%s", got)
	}
}

func writeDecisionTraceFixture(t *testing.T, path, value string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}
