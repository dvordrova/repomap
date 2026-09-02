package report

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programpage"
	"github.com/dvordrova/repomap/internal/targetoutcome"
)

func TestRunManifestVerifiesProgramPagePortfolioArtifact(t *testing.T) {
	fixture := newProgramPageManifestFixture(t)

	t.Run("portfolio binds exact current ProgramTarget and run", func(t *testing.T) {
		runDir, manifest := fixture.run(t)
		if err := manifest.Validate(); err != nil {
			t.Fatalf("neutral page manifest: %v", err)
		}
		if err := manifest.VerifyProgramPagePortfolioArtifact(runDir); err != nil {
			t.Fatalf("VerifyProgramPagePortfolioArtifact: %v", err)
		}
	})

	t.Run("manifest digest rejects changed bytes", func(t *testing.T) {
		runDir, manifest := fixture.run(t)
		changed := append(append([]byte(nil), fixture.raw...), '\n')
		writeTargetPageManifestArtifact(t, runDir, programpage.ArtifactFilename, changed)
		if err := manifest.VerifyProgramPagePortfolioArtifact(runDir); err == nil ||
			!strings.Contains(err.Error(), "sha256 mismatch") {
			t.Fatalf("changed portfolio error = %v", err)
		}
	})

	t.Run("portfolio self-seal rejects tamper even when manifest follows bytes", func(t *testing.T) {
		runDir, manifest := fixture.run(t)
		tampered := bytes.Replace(fixture.raw, []byte(fixture.currentRunID), []byte("run-current-2"), 1)
		writeTargetPageManifestArtifact(t, runDir, programpage.ArtifactFilename, tampered)
		manifest.MaterialInputs.ProgramPagePortfolioSHA256 = manifestSHA256(tampered)
		if err := manifest.VerifyProgramPagePortfolioArtifact(runDir); err == nil ||
			!strings.Contains(err.Error(), "sha256 mismatch") {
			t.Fatalf("tampered portfolio error = %v", err)
		}
	})

	t.Run("current target must own this exact run", func(t *testing.T) {
		runDir, manifest := fixture.run(t)
		manifest.MaterialInputs.ProgramTargetID = fixture.sibling.ID
		_, manifest.MaterialInputs.ProgramTargetSHA256, _ = reportProgramTargetMaterial(&fixture.sibling)
		if err := manifest.VerifyProgramPagePortfolioArtifact(runDir); err == nil ||
			!strings.Contains(err.Error(), "no exact published program page") {
			t.Fatalf("other sibling page error = %v", err)
		}

		otherDir := filepath.Join(t.TempDir(), "run-current-other")
		if err := os.Mkdir(otherDir, 0o700); err != nil {
			t.Fatal(err)
		}
		writeTargetPageManifestArtifact(t, otherDir, programpage.ArtifactFilename, fixture.raw)
		manifest.MaterialInputs.ProgramTargetID = fixture.current.ID
		_, manifest.MaterialInputs.ProgramTargetSHA256, _ = reportProgramTargetMaterial(&fixture.current)
		if err := manifest.VerifyProgramPagePortfolioArtifact(otherDir); err == nil ||
			!strings.Contains(err.Error(), "no exact published program page") {
			t.Fatalf("other run page error = %v", err)
		}
	})

	t.Run("present unbound portfolio is rejected", func(t *testing.T) {
		runDir, manifest := fixture.run(t)
		manifest.MaterialInputs.ProgramPagePortfolioSHA256 = ""
		manifest.MaterialInputs.TargetOutcomePortfolioSHA256 = ""
		if err := manifest.VerifyProgramPagePortfolioArtifact(runDir); err == nil ||
			!strings.Contains(err.Error(), "unbound program page portfolio") {
			t.Fatalf("unbound portfolio error = %v", err)
		}
	})
}

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

func TestRunManifestVerifiesTargetOutcomePortfolioProjection(t *testing.T) {
	fixture := newProgramPageManifestFixture(t)
	runDir, manifest := fixture.run(t)
	view, err := NewTargetOutcomePortfolioView(fixture.outcomes, fixture.portfolio)
	if err != nil {
		t.Fatal(err)
	}
	reportRaw, err := json.Marshal(ReportData{TargetOutcomePortfolio: view})
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyTargetOutcomePortfolioProjection(runDir, reportRaw); err != nil {
		t.Fatalf("VerifyTargetOutcomePortfolioProjection: %v", err)
	}

	tampered := *view
	tampered.Outcomes = append([]TargetOutcomeView(nil), view.Outcomes...)
	tampered.Outcomes[0].DisplayName += " changed"
	tamperedRaw, err := json.Marshal(ReportData{TargetOutcomePortfolio: &tampered})
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyTargetOutcomePortfolioProjection(runDir, tamperedRaw); err == nil ||
		!strings.Contains(err.Error(), "does not match artifacts") {
		t.Fatalf("tampered projection error = %v", err)
	}

	if err := os.Remove(filepath.Join(runDir, targetoutcome.ArtifactFilename)); err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyTargetOutcomePortfolioProjection(runDir, reportRaw); err == nil ||
		!strings.Contains(err.Error(), "artifact or projection is missing") {
		t.Fatalf("missing outcome artifact error = %v", err)
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

func (fixture programPageManifestFixture) run(t *testing.T) (string, RunManifest) {
	t.Helper()
	runDir := filepath.Join(t.TempDir(), fixture.currentRunID)
	if err := os.Mkdir(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTargetPageManifestArtifact(t, runDir, programpage.ArtifactFilename, fixture.raw)
	writeTargetPageManifestArtifact(t, runDir, targetoutcome.ArtifactFilename, fixture.outcomeRaw)
	manifest := validRunManifestFixture(t)
	var err error
	manifest.MaterialInputs.ProgramTargetID, manifest.MaterialInputs.ProgramTargetSHA256, err =
		reportProgramTargetMaterial(&fixture.current)
	if err != nil {
		t.Fatal(err)
	}
	manifest.MaterialInputs.ProgramPagePortfolioSHA256 = manifestSHA256(fixture.raw)
	manifest.MaterialInputs.TargetOutcomePortfolioSHA256 = manifestSHA256(fixture.outcomeRaw)
	return runDir, manifest
}
