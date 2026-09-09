package report

import (
	"bytes"
	"fmt"
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

	if err := installAuthorizedReport(runDir, reportJSON, reportHTML, manifest, nil); err != nil {
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
	// The manifest is a record of the run, installed last; reading it back
	// is reading it back, not verifying the report against it.
	installed, err := ReadRunManifest(runDir)
	if err != nil {
		t.Fatalf("read installed manifest: %v", err)
	}
	if installed.ProgramTargetID != manifest.ProgramTargetID || installed.AnalysisRoot != manifest.AnalysisRoot {
		t.Fatalf("installed manifest = %#v, want %#v", installed, manifest)
	}
	assertNoReportStages(t, runDir)
}

func TestInstallAuthorizedBackingDataRemovesTargetLocalHTML(t *testing.T) {
	runDir := t.TempDir()
	htmlPath := filepath.Join(runDir, "report.html")
	if err := os.WriteFile(htmlPath, []byte("stale target page\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reportJSON := []byte("backing-report-json\n")
	if err := installAuthorizedReport(
		runDir, reportJSON, nil, validRunManifestFixture(t), nil,
	); err != nil {
		t.Fatalf("installAuthorizedReport backing data: %v", err)
	}
	if _, err := os.Lstat(htmlPath); !os.IsNotExist(err) {
		t.Fatalf("backing data retained target-local report.html: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(reportJSON) {
		t.Fatalf("report.json = %q, want %q", got, reportJSON)
	}
	if _, err := os.Lstat(filepath.Join(runDir, RunManifestFilename)); err != nil {
		t.Fatalf("backing manifest is missing: %v", err)
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
		nil,
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
			strings.HasPrefix(entry.Name(), ".report-html-") ||
			strings.HasPrefix(entry.Name(), ".report-translations-") {
			t.Fatalf("staged report artifact remains: %s", entry.Name())
		}
	}
}

func TestLocalizedPublicationRestoresDisplayAndReplacesPreviousHTML(t *testing.T) {
	runDir := t.TempDir()
	data := reportProgramShellDataFixture(t, "example.com/team/server/v5")
	data.ArtifactsDir = runDir
	data.ReadmeOverview = "This server prepares a response."
	data.defaultProgramIndexArtifactFilename = "program-index.json"
	manifest := validRunManifestFixture(t)
	manifest.AnalysisRoot = filepath.Join(manifest.RepositoryState.Identity, "tutorial-game")
	source, err := NewRunSource(manifest.AnalysisRoot, manifest.RepositoryState)
	if err != nil {
		t.Fatal(err)
	}
	options := GenerateOptions{Data: &data, PublishHTML: true}
	if _, err := Generate(runDir, source, options); err != nil {
		t.Fatal(err)
	}
	englishJSON, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PreparePage(&data, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	catalog := prepared.TextCatalog()
	translations := DisplayTranslations{Version: DisplayTextVersion, Language: Russian, CatalogSHA256: catalog.SHA256, Entries: []DisplayTranslationEntry{}}
	for i, entry := range catalog.Entries {
		translated := fmt.Sprintf("Русский текст %d", i+1)
		for _, protected := range entry.Protected {
			translated += " " + protected.Ref
		}
		translations.Entries = append(translations.Entries, DisplayTranslationEntry{Ref: entry.Ref, Text: translated})
	}
	options.Render = RenderOptions{Language: Russian, Translations: &translations}
	receipt, err := Generate(runDir, source, options)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.HTMLFilename() != "report.tutorial-game.ru.html" {
		t.Fatalf("localized filename = %q", receipt.HTMLFilename())
	}
	if _, err := os.Stat(filepath.Join(runDir, "report.html")); !os.IsNotExist(err) {
		t.Fatalf("localized publication retained an English HTML: %v", err)
	}
	russianJSON, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil || !bytes.Equal(englishJSON, russianJSON) {
		t.Fatalf("localization changed canonical JSON: %v", err)
	}
	localizedHTML, err := os.ReadFile(filepath.Join(runDir, receipt.HTMLFilename()))
	if err != nil || !bytes.Contains(localizedHTML, []byte(`<html lang="ru">`)) || !bytes.Contains(localizedHTML, []byte("Русский текст")) {
		t.Fatalf("localized static HTML is incomplete: %v", err)
	}
	restored, err := ReadRunReceipt(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if restored.HTMLFilename() != receipt.HTMLFilename() || restored.RenderOptions().Translations == nil {
		t.Fatal("receipt lost display publication")
	}
	rerendered, err := RenderSavedHTML(runDir)
	if err != nil || !bytes.Contains(rerendered, []byte("Русский текст")) {
		t.Fatalf("restored display failed: %v", err)
	}
	if !bytes.Equal(rerendered, localizedHTML) {
		t.Fatal("saved rendering differs from ordinary localized publication")
	}
	translationPath := filepath.Join(runDir, receipt.manifest.Display.TranslationsFilename)
	savedTranslation, err := os.ReadFile(translationPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(translationPath); err != nil {
		t.Fatal(err)
	}
	if _, err := RenderSavedHTML(runDir); err == nil {
		t.Fatal("missing saved translation was silently regenerated or replaced")
	}
	if err := os.WriteFile(translationPath, savedTranslation, 0o600); err != nil {
		t.Fatal(err)
	}
	// A later English publication replaces the chosen display artifact too.
	options.Render = RenderOptions{}
	if _, err := Generate(runDir, source, options); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{receipt.HTMLFilename(), "report-translations.ru.json"} {
		if _, err := os.Stat(filepath.Join(runDir, name)); !os.IsNotExist(err) {
			t.Fatalf("English replacement retained %s: %v", name, err)
		}
	}
	assertNoReportStages(t, runDir)
}

func TestLocalizedPublicationFailureLeavesNoFinishedArtifacts(t *testing.T) {
	runDir := t.TempDir()
	manifest := validRunManifestFixture(t)
	manifest.Display = &DisplayPublication{Language: Russian, HTMLFilename: "report.server.ru.html", TranslationsFilename: "report-translations.ru.json"}
	manifest.Version = 0 // Fail the final manifest installation, after all renames.
	err := installAuthorizedReport(runDir, []byte("{}"), []byte("<html/>"), manifest, []byte("{}"))
	if err == nil {
		t.Fatal("invalid manifest was installed")
	}
	for _, name := range []string{"report.json", "report.html", manifest.Display.HTMLFilename, manifest.Display.TranslationsFilename, RunManifestFilename} {
		if _, err := os.Stat(filepath.Join(runDir, name)); !os.IsNotExist(err) {
			t.Fatalf("failed publication left %s: %v", name, err)
		}
	}
	assertNoReportStages(t, runDir)
}
