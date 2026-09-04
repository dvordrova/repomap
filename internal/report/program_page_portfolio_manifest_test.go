package report

import (
	"bytes"
	"encoding/json"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programpage"
	"github.com/dvordrova/repomap/internal/targetoutcome"
	"strings"
	"testing"
)

func TestTargetOutcomePortfolioViewRequiresExactAnalyzedPageBijection(t *testing.T) {
	fixture := newProgramPageManifestFixture(t)
	failedSelected, err := targetoutcome.NewSelectedTarget(
		targetoutcome.LanguageGroupGo, targetoutcome.ScopeLibrary,
		"unavailable module", "go:example.test/unavailable",
	)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := targetoutcome.NewNotAnalyzed(
		failedSelected, targetoutcome.StageProgramAnalysis, targetoutcome.ReasonSourceNotAnalyzable,
	)
	if err != nil {
		t.Fatal(err)
	}
	outcomes := append([]targetoutcome.Outcome(nil), fixture.outcomes.Outcomes...)
	outcomes = append(outcomes, failed)
	portfolio, err := targetoutcome.Build(failedSelected.ID, outcomes)
	if err != nil {
		t.Fatal(err)
	}
	view, err := NewTargetOutcomePortfolioView(portfolio, fixture.portfolio)
	if err != nil {
		t.Fatalf("NewTargetOutcomePortfolioView: %v", err)
	}
	if err := view.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(`"run_id"`)) || bytes.Contains(encoded, []byte(`"program_target"`)) ||
		!bytes.Contains(encoded, []byte(`"state":"not_analyzed"`)) ||
		!bytes.Contains(encoded, []byte(`"failure_reason":"source_not_analyzable"`)) {
		t.Fatalf("target outcome browser projection = %s", encoded)
	}

	drifted := make([]targetoutcome.Outcome, 0, len(fixture.outcomes.Outcomes))
	for _, outcome := range fixture.outcomes.Outcomes {
		if outcome.Analysis != nil && outcome.Analysis.ProgramTarget.ID == fixture.current.ID {
			outcome, err = targetoutcome.NewAnalyzed(
				outcome.SelectedTarget, outcome.Analysis.ProgramTarget, "run-current-other",
			)
			if err != nil {
				t.Fatal(err)
			}
		}
		drifted = append(drifted, outcome)
	}
	driftedPortfolio, err := targetoutcome.Build(fixture.outcomes.DefaultSelectedTargetID, drifted)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewTargetOutcomePortfolioView(driftedPortfolio, fixture.portfolio); err == nil ||
		!strings.Contains(err.Error(), "no exact program page") {
		t.Fatalf("drifted run binding error = %v", err)
	}
}

type programPageManifestFixture struct {
	portfolio    programpage.Portfolio
	raw          []byte
	current      programindex.Target
	sibling      programindex.Target
	currentRunID string
	outcomes     targetoutcome.Portfolio
	outcomeRaw   []byte
}

func newProgramPageManifestFixture(t *testing.T) programPageManifestFixture {
	t.Helper()
	current := reportProgramIndexFixture(t, "python", "executable").Target
	sibling := reportProgramIndexFixture(t, "typescript", "application").Target
	const currentRunID = "run-current-1"
	portfolio, err := programpage.Build(current.ID, []programpage.Page{
		{Target: current, RunID: currentRunID},
		{Target: sibling, RunID: "run-sibling-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := portfolio.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	currentSelected, err := targetoutcome.NewSelectedTarget(
		targetoutcome.LanguageGroupPython, targetoutcome.ScopeExecutable,
		current.Name, current.Selector,
	)
	if err != nil {
		t.Fatal(err)
	}
	siblingSelected, err := targetoutcome.NewSelectedTarget(
		targetoutcome.LanguageGroupJavaScriptTypeScript, targetoutcome.ScopePackage,
		sibling.Name, sibling.Selector,
	)
	if err != nil {
		t.Fatal(err)
	}
	currentOutcome, err := targetoutcome.NewAnalyzed(currentSelected, current, currentRunID)
	if err != nil {
		t.Fatal(err)
	}
	siblingOutcome, err := targetoutcome.NewAnalyzed(siblingSelected, sibling, "run-sibling-1")
	if err != nil {
		t.Fatal(err)
	}
	outcomes, err := targetoutcome.Build(currentSelected.ID, []targetoutcome.Outcome{
		currentOutcome, siblingOutcome,
	})
	if err != nil {
		t.Fatal(err)
	}
	outcomeRaw, err := outcomes.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	return programPageManifestFixture{
		portfolio: portfolio, raw: raw, current: current, sibling: sibling,
		currentRunID: currentRunID, outcomes: outcomes, outcomeRaw: outcomeRaw,
	}
}
