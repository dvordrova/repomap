package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/report"
)

func TestGlobalPublicationFailureKeepsCompletedTargetPagesAnalyzed(t *testing.T) {
	var stderr bytes.Buffer
	output := newRunOutput(&stderr)
	targets := []targetPageConsoleContext{
		{DisplayPath: "server", Scope: "go:server", RunID: "run-server", Role: "default"},
		{DisplayPath: "client", Scope: "go:client", RunID: "run-client", Role: "sibling"},
	}

	reportAnalyzedTargetPagePublicationFailure(output, targets)

	got := stderr.String()
	if strings.Count(got, "state: analyzed") != len(targets) {
		t.Fatalf("completed target states =\n%s", got)
	}
	if strings.Contains(got, "Target not analyzed") || strings.Count(got, "state: failed") != 1 {
		t.Fatalf("global failure was reported as a target-local failure:\n%s", got)
	}
	publication := strings.Index(got, "Report publication:")
	lastTarget := strings.LastIndex(got, "Target page:")
	if publication < 0 || publication < lastTarget ||
		!strings.Contains(got[publication:], "state: failed") ||
		!strings.Contains(got[publication:], "analyzed target pages: 2") ||
		!strings.Contains(got[publication:], "final report was not published") {
		t.Fatalf("publication failure summary =\n%s", got)
	}
}

func TestQuarantineTargetPagePublicationRemovesAuthorityAndProductNames(t *testing.T) {
	runDir := t.TempDir()
	for _, name := range []string{report.RunManifestFilename, "report.html", "report.json", "program-index.json"} {
		if err := os.WriteFile(filepath.Join(runDir, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := quarantineTargetPagePublication([]string{runDir, runDir}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{report.RunManifestFilename, "report.html", "report.json"} {
		if _, err := os.Stat(filepath.Join(runDir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s remains product-addressable: %v", name, err)
		}
	}
	for _, name := range []string{"report.html.failed", "report.json.failed", "program-index.json"} {
		if _, err := os.Stat(filepath.Join(runDir, name)); err != nil {
			t.Fatalf("%s was not preserved: %v", name, err)
		}
	}
}
