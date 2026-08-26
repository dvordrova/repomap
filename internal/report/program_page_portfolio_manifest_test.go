package report

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programpage"
	"github.com/dvordrova/repomap/internal/runtimeportfolio"
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
		manifest.MaterialInputs.RuntimePortfolioSHA256 = ""
		if err := manifest.VerifyProgramPagePortfolioArtifact(runDir); err == nil ||
			!strings.Contains(err.Error(), "unbound program page portfolio") {
			t.Fatalf("unbound portfolio error = %v", err)
		}
	})
}

func TestRunManifestPageAuthoritiesAreMutuallyExclusiveAndBindRuntime(t *testing.T) {
	fixture := newProgramPageManifestFixture(t)
	_, neutral := fixture.run(t)

	legacy := neutral
	legacy.MaterialInputs.AnalysisTargetRef = "at-fixture"
	legacy.MaterialInputs.AnalysisTargetSHA256 = strings.Repeat("a", 64)
	legacy.MaterialInputs.TargetRunContainerSHA256 = strings.Repeat("b", 64)
	if err := legacy.Validate(); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("neutral plus Go container error = %v", err)
	}

	legacy.MaterialInputs.TargetPagePortfolioSHA256 = strings.Repeat("c", 64)
	if err := legacy.Validate(); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("neutral plus Go page portfolio error = %v", err)
	}

	missingRuntime := neutral
	missingRuntime.MaterialInputs.RuntimePortfolioSHA256 = ""
	if err := missingRuntime.Validate(); err == nil || !strings.Contains(err.Error(), "runtime portfolio binding") {
		t.Fatalf("neutral page without runtime error = %v", err)
	}

	missingPage := validRunManifestFixture(t)
	missingPage.MaterialInputs.RuntimePortfolioSHA256 = strings.Repeat("d", 64)
	if err := missingPage.Validate(); err == nil || !strings.Contains(err.Error(), "runtime portfolio binding") {
		t.Fatalf("runtime without page authority error = %v", err)
	}
}

func TestRunManifestVerifiesRuntimePortfolioAgainstProgramPages(t *testing.T) {
	fixture := newProgramPageManifestFixture(t)
	runDir, manifest := fixture.run(t)
	targets := runtimeTargetsForProgramPages(fixture.portfolio)
	result := reportRuntimePortfolioFixture(t, fixture.portfolio.SHA256, targets)
	runtimeRaw, err := runtimeportfolio.Encode(result)
	if err != nil {
		t.Fatal(err)
	}
	writeTargetPageManifestArtifact(t, runDir, runtimeportfolio.ArtifactFilename, runtimeRaw)
	manifest.MaterialInputs.RuntimePortfolioSHA256 = manifestSHA256(runtimeRaw)

	index := reportProgramIndexFixture(t, fixture.current.Language, fixture.current.Kind)
	if index.Target.ID != fixture.current.ID {
		t.Fatalf("current fixture target changed: %q != %q", index.Target.ID, fixture.current.ID)
	}
	programPortfolio, err := NewProgramPortfolio(index.Target.ID, []programindex.Index{index})
	if err != nil {
		t.Fatal(err)
	}
	view, err := NewRuntimePortfolioView(result)
	if err != nil {
		t.Fatal(err)
	}
	reportRaw, err := json.Marshal(ReportData{
		FormatVersion: CurrentFormatVersion, RepoName: "fixture",
		CapturedRevision: manifest.RepositoryState.Head, CapturedInputCount: len(manifest.CapturedInputs),
		ProgramPortfolio: programPortfolio, RuntimePortfolio: view,
		OpenablePaths: []string{"batch.go", "cmd/app/main.go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyRuntimePortfolioProjection(runDir, reportRaw); err != nil {
		t.Fatalf("VerifyRuntimePortfolioProjection: %v", err)
	}
	if digest, err := savedRuntimePortfolioSHA256(runDir); err != nil || digest != manifestSHA256(runtimeRaw) {
		t.Fatalf("savedRuntimePortfolioSHA256 = %q, %v", digest, err)
	}

	driftedTargets := append([]runtimeportfolio.Target(nil), targets...)
	for index := range driftedTargets {
		if driftedTargets[index].ProgramTargetID != fixture.sibling.ID {
			continue
		}
		driftedTargets[index].ProgramTargetID = "program-target-substituted"
		break
	}
	drifted := reportRuntimePortfolioFixture(t, fixture.portfolio.SHA256, driftedTargets)
	driftedRaw, err := runtimeportfolio.Encode(drifted)
	if err != nil {
		t.Fatal(err)
	}
	writeTargetPageManifestArtifact(t, runDir, runtimeportfolio.ArtifactFilename, driftedRaw)
	manifest.MaterialInputs.RuntimePortfolioSHA256 = manifestSHA256(driftedRaw)
	driftedView, err := NewRuntimePortfolioView(drifted)
	if err != nil {
		t.Fatal(err)
	}
	var driftedReport ReportData
	if err := json.Unmarshal(reportRaw, &driftedReport); err != nil {
		t.Fatal(err)
	}
	driftedReport.RuntimePortfolio = driftedView
	driftedReportRaw, err := json.Marshal(driftedReport)
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyRuntimePortfolioProjection(runDir, driftedReportRaw); err == nil ||
		!strings.Contains(err.Error(), "does not match program pages") {
		t.Fatalf("substituted runtime target error = %v", err)
	}
}

type programPageManifestFixture struct {
	portfolio    programpage.Portfolio
	raw          []byte
	current      programindex.Target
	sibling      programindex.Target
	currentRunID string
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
	return programPageManifestFixture{
		portfolio: portfolio, raw: raw, current: current, sibling: sibling,
		currentRunID: currentRunID,
	}
}

func (fixture programPageManifestFixture) run(t *testing.T) (string, RunManifest) {
	t.Helper()
	runDir := filepath.Join(t.TempDir(), fixture.currentRunID)
	if err := os.Mkdir(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTargetPageManifestArtifact(t, runDir, programpage.ArtifactFilename, fixture.raw)
	manifest := validRunManifestFixture(t)
	var err error
	manifest.MaterialInputs.ProgramTargetID, manifest.MaterialInputs.ProgramTargetSHA256, err =
		reportProgramTargetMaterial(&fixture.current)
	if err != nil {
		t.Fatal(err)
	}
	manifest.MaterialInputs.ProgramPagePortfolioSHA256 = manifestSHA256(fixture.raw)
	manifest.MaterialInputs.RuntimePortfolioSHA256 = strings.Repeat("7", 64)
	return runDir, manifest
}

func runtimeTargetsForProgramPages(portfolio programpage.Portfolio) []runtimeportfolio.Target {
	result := make([]runtimeportfolio.Target, 0, len(portfolio.Pages))
	for _, page := range portfolio.Pages {
		result = append(result, runtimeportfolio.Target{
			ProgramTargetID: page.Target.ID, DisplayName: page.Target.Name,
			Language: page.Target.Language, Kind: page.Target.Kind, Selector: page.Target.Selector,
			Default: page.Target.ID == portfolio.DefaultTargetID,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ProgramTargetID < result[j].ProgramTargetID
	})
	return result
}
