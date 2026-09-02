package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/programpage"
	"github.com/dvordrova/repomap/internal/report"
)

func TestPreparePublishedTargetAuthorityRetainsOnlyProgramPage(t *testing.T) {
	program := runtimeGroupedProgramIndex(
		t, "api", "go:./cmd/api", "cmd/api/main.go", "f-api",
	)
	want := report.TargetNavigationPage{RunID: "run-api", ProgramTarget: program.Target}
	calls := 0
	got, err := preparePublishedTargetAuthority(func() (report.TargetNavigationPage, error) {
		calls++
		return want, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || got.RunID != want.RunID || got.ProgramTarget.ID != want.ProgramTarget.ID {
		t.Fatalf("prepared page = %#v, calls = %d", got, calls)
	}
}

func TestProgramPagePortfolioPersistsIdenticallyForEveryGraphRun(t *testing.T) {
	programs := []struct {
		name     string
		selector string
		path     string
		fileRef  string
	}{
		{name: "api", selector: "go:./cmd/api", path: "cmd/api/main.go", fileRef: "f-api"},
		{name: "worker", selector: "python:worker", path: "worker.py", fileRef: "f-worker"},
	}
	runs := make([]targetPublishedRun, len(programs))
	for position, fixture := range programs {
		program := runtimeGroupedProgramIndex(
			t, fixture.name, fixture.selector, fixture.path, fixture.fileRef,
		)
		runID := "run-" + fixture.name
		runs[position] = targetPublishedRun{
			RunID: runID, RunDir: filepath.Join(t.TempDir(), runID),
			ProgramPage: report.TargetNavigationPage{RunID: runID, ProgramTarget: program.Target},
		}
		if err := os.MkdirAll(runs[position].RunDir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	portfolio, err := buildProgramPagePortfolio(runs, runs[0].RunID)
	if err != nil {
		t.Fatal(err)
	}
	if err := persistProgramPagePortfolioForRuns(portfolio, runs); err != nil {
		t.Fatal(err)
	}
	for _, run := range runs {
		raw, err := os.ReadFile(filepath.Join(run.RunDir, programpage.ArtifactFilename))
		if err != nil {
			t.Fatal(err)
		}
		restored, err := programpage.Decode(raw)
		if err != nil {
			t.Fatal(err)
		}
		if restored.SHA256 != portfolio.SHA256 {
			t.Fatalf("run %s portfolio digest = %s, want %s", run.RunID, restored.SHA256, portfolio.SHA256)
		}
	}
}
