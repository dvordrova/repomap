package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/runtimeportfolio"
)

func TestRuntimePortfolioTargetInputRetainsValidatedRuntimeEvidence(t *testing.T) {
	target := programindex.Target{
		ID: "program-target-runtime", Language: "go", Kind: "executable_package",
		Name: "cmd/dragonfly", Selector: "./cmd/dragonfly",
		Sources: []programindex.TargetSource{{FileRef: "f-main", Path: "cmd/dragonfly/main.go"}},
	}
	data := &report.ReportData{
		ProgramPortfolio: &report.ProgramPortfolio{
			DefaultTargetID: target.ID,
			Entries: []report.ProgramPortfolioEntry{{
				Target: target,
				View: report.ProgramView{
					IndexCoverage: programindex.Coverage{ObjectsObserved: 41, RelationsObserved: 73},
					Seeds: []report.ProgramViewSeed{{
						Kind: programindex.SeedCallable, Name: "main", Signature: "func main()",
						LaunchLocation: &programindex.Location{Path: "cmd/dragonfly/main.go", Line: 12, Column: 1},
					}},
				},
			}},
		},
		CoreMapView: &report.CoreMapView{
			ProgramTargetID: target.ID,
			RefinedCore: []report.CoreMapViewBlock{{
				Name: "Peer runtime configuration", Purpose: "Configures seed-peer execution mode.",
				Files: []report.CoreMapViewFile{{Path: "client/config/peerhost.go"}},
				RepresentativeSymbols: []report.CoreMapViewRepresentativeSymbol{{
					Symbol: report.CoreMapViewSymbol{
						Package: "config", Name: "SeedPeerOption",
						Location: report.CoreMapViewLocation{Path: "client/config/peerhost.go", Line: 27, Column: 2},
					},
				}},
			}},
		},
		ActivityEntrypointView: &report.ActivityEntrypointView{
			ProgramTargetID: target.ID,
			Entrypoints: []report.ActivityEntrypointViewObject{{
				Name: "daemon", Signature: "func daemon()",
				Location: programindex.Location{Path: "cmd/dfget/cmd/daemon.go", Line: 19, Column: 1},
			}},
		},
		IntegrationUsageView: &report.IntegrationUsageView{
			ProgramTargetID: target.ID,
			Coverage:        report.IntegrationUsageViewCoverage{Selected: 9},
		},
	}
	page := report.TargetNavigationPage{ProgramTarget: target}

	input, err := runtimePortfolioTargetInput(data, page, true)
	if err != nil {
		t.Fatal(err)
	}
	if !input.Default || input.ProgramTargetID != target.ID || input.ProgramObjects != 41 ||
		input.ProgramRelations != 73 || input.ActivityStarts != 1 || input.IntegrationUses != 9 {
		t.Fatalf("target input summary = %#v", input)
	}
	if len(input.Responsibilities) != 1 || input.Responsibilities[0].Name != "Peer runtime configuration" ||
		len(input.Responsibilities[0].Evidence) != 2 {
		t.Fatalf("responsibility evidence = %#v", input.Responsibilities)
	}
	encoded := fmt.Sprintf("%#v", input)
	for _, want := range []string{"SeedPeerOption", "client/config/peerhost.go", "daemon", "cmd/dfget/cmd/daemon.go"} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("target input omitted %q: %s", want, encoded)
		}
	}
}

func TestBuildProgramPagePortfolioUsesValidatedPublishedPageIdentity(t *testing.T) {
	buildTarget := func(selector, path, fileRef string) programindex.Target {
		t.Helper()
		index, err := programindex.New(programindex.Input{
			ScenarioSHA256: strings.Repeat("a", 64),
			SourceSHA256:   strings.Repeat("b", 64),
			Target: programindex.TargetInput{
				Language: "go", Kind: "executable", Name: selector, Selector: selector,
				Sources:       []programindex.TargetSource{{FileRef: fileRef, Path: path}},
				AnchorFileRef: fileRef,
			},
			Objects: []programindex.ObjectInput{}, Relations: []programindex.RelationInput{},
			Coverage: programindex.CoverageInput{Measured: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		return index.Target
	}
	base := t.TempDir()
	defaultRunID := "20260827-120000-api-a1b2c3"
	workerRunID := "20260827-120000-worker-a1b2c3"
	api := buildTarget("./cmd/api", "cmd/api/main.go", "f-api")
	worker := buildTarget("./cmd/worker", "cmd/worker/main.go", "f-worker")
	runs := []targetPublishedRun{
		{
			RunID: defaultRunID, RunDir: filepath.Join(base, defaultRunID),
			ProgramPage: report.TargetNavigationPage{
				RunID: defaultRunID, ProgramTarget: api,
				ArtifactFilename: "program-index-api.json",
			},
		},
		{
			RunID: workerRunID, RunDir: filepath.Join(base, workerRunID),
			ProgramPage: report.TargetNavigationPage{
				RunID: workerRunID, ProgramTarget: worker,
				ArtifactFilename: "program-index-worker.json",
			},
		},
	}

	portfolio, err := buildProgramPagePortfolio(runs, defaultRunID)
	if err != nil {
		t.Fatal(err)
	}
	runByTargetID := make(map[string]string, len(portfolio.Pages))
	for _, page := range portfolio.Pages {
		runByTargetID[page.Target.ID] = page.RunID
	}
	if portfolio.DefaultTargetID != api.ID || len(portfolio.Pages) != 2 ||
		runByTargetID[api.ID] != defaultRunID || runByTargetID[worker.ID] != workerRunID {
		t.Fatalf("program page portfolio = %#v", portfolio)
	}
	if _, err := os.Stat(runs[0].RunDir); !os.IsNotExist(err) {
		t.Fatalf("test unexpectedly created backing run directory: %v", err)
	}

	runs[1].ProgramPage.RunID = defaultRunID
	if _, err := buildProgramPagePortfolio(runs, defaultRunID); err == nil ||
		!strings.Contains(err.Error(), "completed page identity is invalid") {
		t.Fatalf("mismatched cached page error = %v", err)
	}
}

func runtimePortfolioArtifactFixture(
	t *testing.T,
) (runtimeportfolio.Input, runtimeportfolio.Result, []byte) {
	t.Helper()
	headerWordLabel := runtimePortfolioEvidenceLabel(
		"Responsibility representative", "Authentication and authorization",
		"go.etcd.io/etcd/server/v3/auth", "Authenticate",
	)
	if want := "Responsibility representative: Authentication and authorization: go.etcd.io/etcd/server/v3/auth: Authenticate"; headerWordLabel != want {
		t.Fatalf("runtime evidence label = %q, want %q", headerWordLabel, want)
	}
	input := runtimeportfolio.Input{
		RepositoryName: "example", CapturedRevision: strings.Repeat("a", 40),
		TargetPagePortfolioSHA256: strings.Repeat("b", 64),
		Targets: []runtimeportfolio.TargetInput{{
			ProgramTargetID: "program-target-a", DisplayName: "server", Language: "go",
			Kind: "executable_package", Selector: "./cmd/server", Default: true,
			Responsibilities: []runtimeportfolio.ResponsibilityInput{},
			Evidence: []runtimeportfolio.EvidenceInput{{
				Kind: runtimeportfolio.EvidenceTargetEntrypoint, Label: headerWordLabel,
				Location:        runtimeportfolio.Location{Path: "cmd/server/main.go", Line: 1},
				ProgramTargetID: "program-target-a",
			}},
		}},
		RepositoryEvidence: []runtimeportfolio.EvidenceInput{},
	}
	compilation, err := runtimeportfolio.Compile(input)
	if err != nil {
		t.Fatal(err)
	}
	evidenceRef := compilation.Request.Targets[0].EvidenceRefs[0]
	response := []byte(fmt.Sprintf(`{"roles":[{"name":"Server","purpose":"Runs the primary service.","prominence":"primary","role_kind":"service","requiredness":"required","confidence":"high","mapping_status":"mapped","implementations":[{"target_ref":"t1"}],"evidence_refs":[%q]}]}`, evidenceRef))
	result, err := runtimeportfolio.ResolveResponse(compilation, response)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := runtimeportfolio.Encode(result)
	if err != nil {
		t.Fatal(err)
	}
	return input, result, encoded
}

func TestRuntimePortfolioArtifactSetValidatesOnceAndChecksEveryRunBytes(t *testing.T) {
	input, _, encoded := runtimePortfolioArtifactFixture(t)
	for tamperedIndex := 0; tamperedIndex < 4; tamperedIndex++ {
		t.Run(fmt.Sprintf("tamper-run-%d", tamperedIndex), func(t *testing.T) {
			validator := runtimePortfolioArtifactSetValidator{}
			fullValidations := 0
			var gotErr error
			for runIndex := 0; runIndex < 4; runIndex++ {
				raw := append([]byte(nil), encoded...)
				if runIndex == tamperedIndex {
					raw[len(raw)-2] ^= 1
				}
				gotErr = validator.validate(raw, func(candidate []byte) error {
					fullValidations++
					return fullyValidateRuntimePortfolioArtifact(candidate, input)
				})
				if gotErr != nil {
					break
				}
			}
			if gotErr == nil {
				t.Fatal("tampered run bytes were accepted")
			}
			if fullValidations != 1 {
				t.Fatalf("full semantic validations = %d, want 1", fullValidations)
			}
		})
	}

	validator := runtimePortfolioArtifactSetValidator{}
	fullValidations := 0
	for runIndex := 0; runIndex < 4; runIndex++ {
		if err := validator.validate(encoded, func(candidate []byte) error {
			fullValidations++
			return fullyValidateRuntimePortfolioArtifact(candidate, input)
		}); err != nil {
			t.Fatalf("validate identical run %d: %v", runIndex, err)
		}
	}
	if fullValidations != 1 {
		t.Fatalf("full semantic validations for identical runs = %d, want 1", fullValidations)
	}
}

func TestPersistRuntimePortfolioWritesByteIdenticalArtifacts(t *testing.T) {
	input, result, _ := runtimePortfolioArtifactFixture(t)
	base := t.TempDir()
	runs := []targetPublishedRun{
		{RunID: "run-a", RunDir: filepath.Join(base, "run-a")},
		{RunID: "run-b", RunDir: filepath.Join(base, "run-b")},
	}
	for _, run := range runs {
		if err := os.Mkdir(run.RunDir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := persistRuntimePortfolioForRuns(input, result, runs); err != nil {
		t.Fatal(err)
	}
	left, err := os.ReadFile(filepath.Join(runs[0].RunDir, runtimeportfolio.ArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	right, err := os.ReadFile(filepath.Join(runs[1].RunDir, runtimeportfolio.ArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(left, right) {
		t.Fatal("repository runtime artifacts differ between target pages")
	}
	decoded, err := runtimeportfolio.Decode(left)
	if err != nil {
		t.Fatal(err)
	}
	if err := decoded.ValidateAgainst(input); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimePortfolioEvidenceLabelIsBoundedUTF8(t *testing.T) {
	label := runtimePortfolioEvidenceLabel("entry\npoint", strings.Repeat("é", runtimeportfolio.MaxTextBytes))
	if !utf8.ValidString(label) || len(label) > runtimeportfolio.MaxTextBytes || strings.Contains(label, "\n") {
		t.Fatalf("bounded label = %q (%d bytes)", label, len(label))
	}
}
