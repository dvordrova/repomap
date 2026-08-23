package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/report"
)

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
