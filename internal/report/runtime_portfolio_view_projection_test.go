package report

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programpage"
	"github.com/dvordrova/repomap/internal/runtimeportfolio"
	"github.com/dvordrova/repomap/internal/snapshot"
)

func TestExtendRunAuthorityCapturesNewRuntimeEvidenceFromInitialState(t *testing.T) {
	repository := t.TempDir()
	writeTestFile(t, repository, "batch.go", "package fixture\n")
	writeTestFile(t, repository, "runtime.go", "package fixture\n\nfunc Runtime() {}\n")
	runManifestGit(t, repository, "init", "--quiet")
	runManifestGit(t, repository, "add", "batch.go", "runtime.go")
	runManifestGit(t, repository,
		"-c", "user.name=repomap test",
		"-c", "user.email=repomap@example.invalid",
		"-c", "commit.gpgsign=false",
		"commit", "--quiet", "-m", "fixture",
	)
	state := captureRunManifestRepositoryState(t, repository)
	authority, err := ConfirmRunAuthorityScoped(
		context.Background(), repository, state, []string{"batch.go"},
	)
	if err != nil {
		t.Fatal(err)
	}
	extended, err := ExtendRunAuthority(
		context.Background(), authority, []string{"batch.go", "runtime.go"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(extended.inputs) != 2 || extended.inputs[0].Path != "batch.go" ||
		extended.inputs[1].Path != "runtime.go" {
		t.Fatalf("extended captured inputs = %#v", extended.inputs)
	}
}

func TestRuntimePortfolioViewPreservesMappingsEvidenceAndExactTargetCoverage(t *testing.T) {
	result := reportRuntimePortfolioFixture(t, strings.Repeat("a", 64), reportRuntimeTargetsFixture(t))
	view, err := NewRuntimePortfolioView(result)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Roles) != 1 || len(view.Roles[0].Implementations) != 1 ||
		view.Roles[0].Implementations[0].Mode != "admin" || len(view.Roles[0].Evidence) != 1 {
		t.Fatalf("runtime role projection = %#v", view.Roles)
	}
	if view.Roles[0].MappingStatus != runtimeportfolio.MappingMapped ||
		view.Roles[0].Evidence[0].Location.Path != "cmd/app/main.go" {
		t.Fatalf("runtime role authority was not preserved: %#v", view.Roles[0])
	}
	data := &ReportData{RuntimePortfolio: view}
	if err := collectOpenablePaths(data); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(data.OpenablePaths, []string{"cmd/app/main.go"}) {
		t.Fatalf("runtime evidence source authority = %#v", data.OpenablePaths)
	}
	if err := validateTargetNavigation(data, nil); err == nil || !strings.Contains(err.Error(), "requires complete") {
		t.Fatalf("missing final target navigation error = %v", err)
	}

	navigation := reportRuntimeNavigationFixture(result.Targets)
	if err := view.validateTargetNavigation(navigation); err != nil {
		t.Fatalf("exact runtime/navigation join: %v", err)
	}
	dropped := *navigation
	dropped.Targets = append([]TargetNavigationItem(nil), navigation.Targets[:1]...)
	if err := view.validateTargetNavigation(&dropped); err == nil || !strings.Contains(err.Error(), "ProgramTarget set") {
		t.Fatalf("incomplete target navigation error = %v", err)
	}
	substituted := *navigation
	substituted.Targets = append([]TargetNavigationItem(nil), navigation.Targets...)
	substituted.Targets[0].TargetID = "pt-substituted"
	if err := view.validateTargetNavigation(&substituted); err == nil || !strings.Contains(err.Error(), "ProgramTarget set") {
		t.Fatalf("substituted target navigation error = %v", err)
	}
}

func TestRuntimePortfolioViewPreservesFirstClassLibraryRole(t *testing.T) {
	result := reportRuntimePortfolioFixtureWithRoleKind(
		t,
		strings.Repeat("a", 64),
		reportRuntimeTargetsFixture(t),
		runtimeportfolio.RoleKindLibrary,
	)
	view, err := NewRuntimePortfolioView(result)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Roles) != 1 || view.Roles[0].RoleKind != runtimeportfolio.RoleKindLibrary ||
		len(view.UnclassifiedTargets) != 1 || view.UnclassifiedTargets[0].ProgramTargetID != "pt-helper" {
		t.Fatalf("library role projection = roles %#v, unclassified %#v", view.Roles, view.UnclassifiedTargets)
	}
}

func TestRestoreRuntimePortfolioRequiresAtomicTargetPageBinding(t *testing.T) {
	outer := newTargetPageManifestFixture(t)
	runDir := t.TempDir()
	writeTargetPageManifestArtifact(t, runDir, snapshot.TargetRunContainerArtifactFilename, outer.containerRaw)
	writeTargetPageManifestArtifact(t, runDir, snapshot.TargetPagePortfolioArtifactFilename, outer.portfolioRaw)
	pagePortfolio, err := snapshot.DecodeTargetPagePortfolio(outer.portfolioRaw)
	if err != nil {
		t.Fatal(err)
	}
	index := reportProgramIndexFixture(t, "neutral-test", "executable")
	programPortfolio, err := NewProgramPortfolio(index.Target.ID, []programindex.Index{index})
	if err != nil {
		t.Fatal(err)
	}
	targets := reportRuntimeTargetsForCurrent(index.Target)
	result := reportRuntimePortfolioFixture(t, pagePortfolio.SHA256, targets)
	raw, err := runtimeportfolio.Encode(result)
	if err != nil {
		t.Fatal(err)
	}
	writeTargetPageManifestArtifact(t, runDir, runtimeportfolio.ArtifactFilename, raw)
	data := &ReportData{ProgramPortfolio: programPortfolio}
	if err := restoreRuntimePortfolioView(runDir, data); err != nil {
		t.Fatalf("restore runtime portfolio: %v", err)
	}
	if data.RuntimePortfolio == nil || len(data.RuntimePortfolio.Roles) != 1 {
		t.Fatalf("runtime portfolio projection = %#v", data.RuntimePortfolio)
	}

	if err := os.Remove(filepath.Join(runDir, runtimeportfolio.ArtifactFilename)); err != nil {
		t.Fatal(err)
	}
	if err := restoreRuntimePortfolioView(runDir, &ReportData{ProgramPortfolio: programPortfolio}); err == nil ||
		!strings.Contains(err.Error(), "published together") {
		t.Fatalf("missing runtime artifact error = %v", err)
	}

	wrong := result
	wrong.TargetPagePortfolioSHA256 = strings.Repeat("b", 64)
	wrongRaw, err := runtimeportfolio.Encode(wrong)
	if err != nil {
		t.Fatal(err)
	}
	writeTargetPageManifestArtifact(t, runDir, runtimeportfolio.ArtifactFilename, wrongRaw)
	if err := restoreRuntimePortfolioView(runDir, &ReportData{ProgramPortfolio: programPortfolio}); err == nil ||
		!strings.Contains(err.Error(), "target-page binding mismatch") {
		t.Fatalf("cross-portfolio runtime artifact error = %v", err)
	}
}

func TestRestoreRuntimePortfolioAcceptsNeutralProgramPageBinding(t *testing.T) {
	runDir := t.TempDir()
	current := reportProgramIndexFixture(t, "neutral-current", "executable")
	sibling := reportProgramIndexFixture(t, "neutral-sibling", "application")
	portfolio, err := programpage.Build(current.Target.ID, []programpage.Page{
		{Target: current.Target.Snapshot(), RunID: "run-current"},
		{Target: sibling.Target.Snapshot(), RunID: "run-sibling"},
	})
	if err != nil {
		t.Fatal(err)
	}
	portfolioRaw, err := portfolio.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	writeTargetPageManifestArtifact(t, runDir, programpage.ArtifactFilename, portfolioRaw)
	programPortfolio, err := NewProgramPortfolio(current.Target.ID, []programindex.Index{current})
	if err != nil {
		t.Fatal(err)
	}
	targets := []runtimeportfolio.Target{
		{
			ProgramTargetID: current.Target.ID, DisplayName: current.Target.Name,
			Language: current.Target.Language, Kind: current.Target.Kind,
			Selector: current.Target.Selector, Default: true,
		},
		{
			ProgramTargetID: sibling.Target.ID, DisplayName: sibling.Target.Name,
			Language: sibling.Target.Language, Kind: sibling.Target.Kind,
			Selector: sibling.Target.Selector,
		},
	}
	sort.Slice(targets, func(left, right int) bool {
		return targets[left].ProgramTargetID < targets[right].ProgramTargetID
	})
	result := reportRuntimePortfolioFixture(t, portfolio.SHA256, targets)
	runtimeRaw, err := runtimeportfolio.Encode(result)
	if err != nil {
		t.Fatal(err)
	}
	writeTargetPageManifestArtifact(t, runDir, runtimeportfolio.ArtifactFilename, runtimeRaw)
	data := &ReportData{ProgramPortfolio: programPortfolio}
	if err := restoreRuntimePortfolioView(runDir, data); err != nil {
		t.Fatalf("restore neutral runtime portfolio: %v", err)
	}
	if data.RuntimePortfolio == nil || len(data.RuntimePortfolio.Roles) != 1 {
		t.Fatalf("neutral runtime portfolio projection = %#v", data.RuntimePortfolio)
	}

	if err := os.Remove(filepath.Join(runDir, runtimeportfolio.ArtifactFilename)); err != nil {
		t.Fatal(err)
	}
	if err := restoreRuntimePortfolioView(runDir, &ReportData{ProgramPortfolio: programPortfolio}); err == nil ||
		!strings.Contains(err.Error(), "published together") {
		t.Fatalf("missing neutral runtime artifact error = %v", err)
	}

	wrong := result
	wrong.TargetPagePortfolioSHA256 = strings.Repeat("b", 64)
	wrongRaw, err := runtimeportfolio.Encode(wrong)
	if err != nil {
		t.Fatal(err)
	}
	writeTargetPageManifestArtifact(t, runDir, runtimeportfolio.ArtifactFilename, wrongRaw)
	if err := restoreRuntimePortfolioView(runDir, &ReportData{ProgramPortfolio: programPortfolio}); err == nil ||
		!strings.Contains(err.Error(), "program-page binding mismatch") {
		t.Fatalf("cross-neutral-portfolio runtime artifact error = %v", err)
	}

	writeTargetPageManifestArtifact(t, runDir, runtimeportfolio.ArtifactFilename, runtimeRaw)
	writeTargetPageManifestArtifact(t, runDir, snapshot.TargetPagePortfolioArtifactFilename, []byte(`{}`))
	if err := restoreRuntimePortfolioView(runDir, &ReportData{ProgramPortfolio: programPortfolio}); err == nil ||
		!strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("mixed legacy/neutral portfolio error = %v", err)
	}
}

func TestRunManifestVerifiesRuntimePortfolioArtifactProjectionAndEvidenceAuthority(t *testing.T) {
	outer := newTargetPageManifestFixture(t)
	runDir := t.TempDir()
	writeTargetPageManifestArtifact(t, runDir, snapshot.TargetPagePortfolioArtifactFilename, outer.portfolioRaw)
	pagePortfolio, err := snapshot.DecodeTargetPagePortfolio(outer.portfolioRaw)
	if err != nil {
		t.Fatal(err)
	}
	index := reportProgramIndexFixture(t, "neutral-test", "executable")
	programPortfolio, err := NewProgramPortfolio(index.Target.ID, []programindex.Index{index})
	if err != nil {
		t.Fatal(err)
	}
	result := reportRuntimePortfolioFixture(t, pagePortfolio.SHA256, reportRuntimeTargetsForCurrent(index.Target))
	view, err := NewRuntimePortfolioView(result)
	if err != nil {
		t.Fatal(err)
	}
	runtimeRaw, err := runtimeportfolio.Encode(result)
	if err != nil {
		t.Fatal(err)
	}
	writeTargetPageManifestArtifact(t, runDir, runtimeportfolio.ArtifactFilename, runtimeRaw)

	manifest := validRunManifestFixture(t)
	target := index.Target.Snapshot()
	manifest.MaterialInputs.ProgramTargetID, manifest.MaterialInputs.ProgramTargetSHA256, err =
		reportProgramTargetMaterial(&target)
	if err != nil {
		t.Fatal(err)
	}
	manifest.MaterialInputs.AnalysisTargetRef = outer.appRef
	manifest.MaterialInputs.AnalysisTargetSHA256 = outer.targetSHA256(t, outer.appRef)
	manifest.MaterialInputs.TargetRunContainerSHA256 = manifestSHA256(outer.containerRaw)
	manifest.MaterialInputs.TargetPagePortfolioSHA256 = manifestSHA256(outer.portfolioRaw)
	manifest.MaterialInputs.RuntimePortfolioSHA256 = manifestSHA256(runtimeRaw)
	data := ReportData{
		FormatVersion: CurrentFormatVersion, RepoName: "fixture",
		CapturedRevision: manifest.RepositoryState.Head, CapturedInputCount: len(manifest.CapturedInputs),
		ProgramPortfolio: programPortfolio, RuntimePortfolio: view,
		OpenablePaths: []string{"batch.go", "cmd/app/main.go"},
	}
	navigation := reportRuntimeNavigationFixture(result.Targets)
	if err := validateTargetNavigation(&data, navigation); err != nil {
		t.Fatalf("final runtime target navigation: %v", err)
	}
	reportRaw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyRuntimePortfolioProjection(runDir, reportRaw); err != nil {
		t.Fatalf("VerifyRuntimePortfolioProjection: %v", err)
	}

	missingEvidence := data
	missingEvidence.OpenablePaths = []string{"batch.go"}
	missingEvidenceRaw, err := json.Marshal(missingEvidence)
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyRuntimePortfolioProjection(runDir, missingEvidenceRaw); err == nil ||
		!strings.Contains(err.Error(), "outside source authority") {
		t.Fatalf("unbound runtime evidence error = %v", err)
	}

	drifted := data
	drifted.RuntimePortfolio = cloneRuntimePortfolioView(t, view)
	drifted.RuntimePortfolio.UnclassifiedTargets[0].Reason = "Guessed locally."
	driftedRaw, err := json.Marshal(drifted)
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyRuntimePortfolioProjection(runDir, driftedRaw); err == nil ||
		!strings.Contains(err.Error(), "does not match artifact") {
		t.Fatalf("drifted runtime projection error = %v", err)
	}
}

func reportRuntimePortfolioFixture(
	t *testing.T,
	portfolioSHA string,
	targets []runtimeportfolio.Target,
) runtimeportfolio.Result {
	return reportRuntimePortfolioFixtureWithRoleKind(
		t,
		portfolioSHA,
		targets,
		runtimeportfolio.RoleKindService,
	)
}

func reportRuntimePortfolioFixtureWithRoleKind(
	t *testing.T,
	portfolioSHA string,
	targets []runtimeportfolio.Target,
	roleKind runtimeportfolio.RoleKind,
) runtimeportfolio.Result {
	t.Helper()
	inputs := make([]runtimeportfolio.TargetInput, 0, len(targets))
	for _, target := range targets {
		inputs = append(inputs, runtimeportfolio.TargetInput{
			ProgramTargetID: target.ProgramTargetID, DisplayName: target.DisplayName,
			Language: target.Language, Kind: target.Kind, Selector: target.Selector, Default: target.Default,
			Responsibilities: []runtimeportfolio.ResponsibilityInput{}, Evidence: []runtimeportfolio.EvidenceInput{},
		})
	}
	defaultTargetID := ""
	for _, target := range targets {
		if target.Default {
			defaultTargetID = target.ProgramTargetID
			break
		}
	}
	evidence := runtimeportfolio.EvidenceInput{
		Kind: runtimeportfolio.EvidenceRepositoryGuidance, Label: "Runtime role documentation",
		Location:        runtimeportfolio.Location{Path: "cmd/app/main.go", Line: 1, Column: 1},
		ProgramTargetID: defaultTargetID,
	}
	repositoryEvidence := []runtimeportfolio.EvidenceInput{evidence}
	implementationMode := "admin"
	if roleKind == runtimeportfolio.RoleKindLibrary {
		evidence.Kind = runtimeportfolio.EvidenceProgramFact
		evidence.Label = "Exports a reusable library API"
		for index := range inputs {
			if inputs[index].ProgramTargetID == defaultTargetID {
				inputs[index].Evidence = append(inputs[index].Evidence, evidence)
				break
			}
		}
		repositoryEvidence = []runtimeportfolio.EvidenceInput{}
		implementationMode = ""
	}
	compilation, err := runtimeportfolio.Compile(runtimeportfolio.Input{
		RepositoryName: "fixture", CapturedRevision: strings.Repeat("c", 40),
		TargetPagePortfolioSHA256: portfolioSHA,
		Targets:                   inputs,
		RepositoryEvidence:        repositoryEvidence,
	})
	if err != nil {
		t.Fatal(err)
	}
	requestRaw, err := json.Marshal(compilation.Request)
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Targets []struct {
			Ref     string `json:"ref"`
			Default bool   `json:"default"`
		} `json:"targets"`
		Evidence []struct {
			Ref string `json:"ref"`
		} `json:"evidence_catalog"`
	}
	if err := json.Unmarshal(requestRaw, &request); err != nil {
		t.Fatal(err)
	}
	targetRef := ""
	for _, target := range request.Targets {
		if target.Default {
			targetRef = target.Ref
		}
	}
	if targetRef == "" || len(request.Evidence) != 1 {
		t.Fatalf("compiled runtime refs = %#v", request)
	}
	responseRaw, err := json.Marshal(map[string]any{
		"roles": []any{map[string]any{
			"name": "Administrative service", "purpose": "Serves the repository's administrative runtime.",
			"prominence": "primary", "role_kind": roleKind, "requiredness": "required",
			"confidence": "high", "mapping_status": "mapped",
			"implementations": []any{map[string]any{"target_ref": targetRef, "mode": implementationMode}},
			"evidence_refs":   []string{request.Evidence[0].Ref},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runtimeportfolio.ResolveResponse(compilation, responseRaw)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func reportRuntimeTargetsFixture(t *testing.T) []runtimeportfolio.Target {
	t.Helper()
	return []runtimeportfolio.Target{
		{ProgramTargetID: "pt-app", DisplayName: "app", Language: "go", Kind: "executable", Selector: "./cmd/app", Default: true},
		{ProgramTargetID: "pt-helper", DisplayName: "helper", Language: "go", Kind: "executable", Selector: "./cmd/helper"},
	}
}

func reportRuntimeTargetsForCurrent(current programindex.Target) []runtimeportfolio.Target {
	targets := []runtimeportfolio.Target{
		{ProgramTargetID: current.ID, DisplayName: current.Name, Language: current.Language, Kind: current.Kind, Selector: current.Selector, Default: true},
		{ProgramTargetID: "pt-supporting-runtime", DisplayName: "supporting", Language: current.Language, Kind: current.Kind, Selector: "supporting-runtime"},
	}
	sort.Slice(targets, func(left, right int) bool { return targets[left].ProgramTargetID < targets[right].ProgramTargetID })
	return targets
}

func reportRuntimeNavigationFixture(targets []runtimeportfolio.Target) *TargetNavigationPortfolio {
	result := &TargetNavigationPortfolio{
		Version: TargetNavigationVersion, Targets: make([]TargetNavigationItem, 0, len(targets)),
	}
	for _, target := range targets {
		if target.Default {
			result.DefaultTargetID = target.ProgramTargetID
			result.CurrentTargetID = target.ProgramTargetID
		}
		href := "../run-supporting/report.html#/program"
		if target.ProgramTargetID == result.CurrentTargetID {
			href = "#/program"
		}
		result.Targets = append(result.Targets, TargetNavigationItem{
			TargetID: target.ProgramTargetID, Language: target.Language, Kind: target.Kind,
			DisplayName: target.DisplayName, Href: href,
		})
	}
	return result
}

func cloneRuntimePortfolioView(t *testing.T, value *RuntimePortfolioView) *RuntimePortfolioView {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result RuntimePortfolioView
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return &result
}
