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

func TestPersistRuntimePortfolioWritesByteIdenticalArtifacts(t *testing.T) {
	input := runtimeportfolio.Input{
		RepositoryName: "example", CapturedRevision: strings.Repeat("a", 40),
		TargetPagePortfolioSHA256: strings.Repeat("b", 64),
		Targets: []runtimeportfolio.TargetInput{{
			ProgramTargetID: "program-target-a", DisplayName: "server", Language: "go",
			Kind: "executable_package", Selector: "./cmd/server", Default: true,
			Responsibilities: []runtimeportfolio.ResponsibilityInput{},
			Evidence: []runtimeportfolio.EvidenceInput{{
				Kind: runtimeportfolio.EvidenceTargetEntrypoint, Label: "Server entrypoint",
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
