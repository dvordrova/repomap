package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRussianReportLanguageReachesDataAndHTML(t *testing.T) {
	t.Parallel()

	html, err := buildHTML(&ReportData{
		FormatVersion:  CurrentFormatVersion,
		ReportLanguage: "ru",
		RepoName:       "fixture",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<html lang="ru">`,
		`"report_language":"ru"`,
		`"main.what.to.study"`,
		`"Что изучать"`,
		`id="rm-ui-messages-js"`,
		`<noscript>`,
		`Для этого отчёта нужен JavaScript.`,
		`body > :not(noscript)`,
	} {
		if !strings.Contains(string(html), want) {
			t.Fatalf("Russian HTML is missing %q", want)
		}
	}
	if strings.Contains(string(html), "This report requires JavaScript.") {
		t.Fatal("Russian no-JS presentation contains the English notice")
	}
}

func TestDefaultReportLanguageRemainsEnglish(t *testing.T) {
	t.Parallel()

	html, err := buildHTML(&ReportData{
		FormatVersion: CurrentFormatVersion,
		RepoName:      "fixture",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(html), `<html lang="en">`) ||
		!strings.Contains(string(html), `<noscript>`) ||
		!strings.Contains(string(html), `This report requires JavaScript.`) ||
		strings.Contains(string(html), `Для этого отчёта нужен JavaScript.`) ||
		strings.Contains(string(html), `"report_language"`) {
		t.Fatalf("default report language changed: %s", html[:min(len(html), 500)])
	}
}

func TestRussianUICatalogPreservesOpaqueRepositoryTerms(t *testing.T) {
	t.Parallel()

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	assetPath, err := filepath.Abs(filepath.Join("templates", "ui_messages.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs");
const vm = require("vm");
const sandbox = {
  window: {},
  document: {documentElement: {lang: "ru"}},
};
sandbox.window.document = sandbox.document;
vm.runInNewContext(fs.readFileSync(process.argv[1], "utf8"), sandbox);
const message = sandbox.window.RepomapUI.message;
process.stdout.write(JSON.stringify([
  message("main.what.to.study"),
  message("main.purpose"),
  message("main.action.start_with", {target: "runServer"}),
  message("main.source.location_lines", {path: "cli/cmd/run.go", start: 85, end: 144}),
  message("main.toast.opened_vscode", {location: "pglogrepl.go:74"}),
  message("main.map.context", {title: "Как работает репликация?"}),
]));
`
	output, err := exec.Command(node, "-e", runner, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run language asset: %v: %s", err, output)
	}
	var got []string
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"Что изучать",
		"Назначение",
		"Начать с runServer →",
		"cli/cmd/run.go · строки 85–144",
		"Открыто в VS Code: pglogrepl.go:74",
		"Контекст на карте: «Как работает репликация?»",
	}
	if len(got) != len(want) {
		t.Fatalf("translations = %#v", got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("translation %d = %q, want %q", index, got[index], want[index])
		}
	}
}

func TestParseRunMetadataDoesNotLeakRequestedLocaleIntoCanonicalReport(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "metadata.json")
	if err := os.WriteFile(
		path,
		[]byte(`{"effective_options":{"report_language":"ru"}}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	var data ReportData
	if warning := parseRunMetadata(path, &data); warning != "" {
		t.Fatal(warning)
	}
	if data.ReportLanguage != "" {
		t.Fatalf("ReportLanguage = %q, want canonical English", data.ReportLanguage)
	}
	if data.requestedPresentationLocale != "ru" {
		t.Fatalf(
			"requestedPresentationLocale = %q, want transient ru render intent",
			data.requestedPresentationLocale,
		)
	}
}
