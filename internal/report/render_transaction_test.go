package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallAuthorizedReportCommitsManifestLast(t *testing.T) {
	runDir := t.TempDir()
	reportJSON := []byte("report-json\n")
	reportHTML := []byte("<html>report</html>\n")
	manifest := validRunManifestFixture(t)

	if err := installAuthorizedReport(runDir, reportJSON, reportHTML, manifest); err != nil {
		t.Fatalf("installAuthorizedReport: %v", err)
	}
	for name, want := range map[string][]byte{
		"report.json": reportJSON,
		"report.html": reportHTML,
	} {
		got, err := os.ReadFile(filepath.Join(runDir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if string(got) != string(want) {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
	if _, err := ReadRunManifest(runDir); err == nil {
		// The low-level fixture deliberately supplies arbitrary report bytes;
		// installation proves ordering, not semantic fixture construction.
		t.Fatal("arbitrary report unexpectedly passed canonical manifest restore")
	}
	if _, err := os.Lstat(filepath.Join(runDir, RunManifestFilename)); err != nil {
		t.Fatalf("ready manifest is missing: %v", err)
	}
	assertNoReportStages(t, runDir)
}

func TestInstallAuthorizedReportFailureRemovesEveryProductName(t *testing.T) {
	runDir := t.TempDir()
	manifest := validRunManifestFixture(t)
	manifest.Version = 0

	err := installAuthorizedReport(
		runDir,
		[]byte("report-json\n"),
		[]byte("<html>report</html>\n"),
		manifest,
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported version") {
		t.Fatalf("installAuthorizedReport error = %v", err)
	}
	for _, name := range []string{"report.json", "report.html", RunManifestFilename} {
		if _, statErr := os.Lstat(filepath.Join(runDir, name)); !os.IsNotExist(statErr) {
			t.Fatalf("failed publication left %s: %v", name, statErr)
		}
	}
	assertNoReportStages(t, runDir)
}

func assertNoReportStages(t *testing.T, runDir string) {
	t.Helper()
	entries, err := os.ReadDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".report-json-") ||
			strings.HasPrefix(entry.Name(), ".report-html-") {
			t.Fatalf("staged report artifact remains: %s", entry.Name())
		}
	}
}
