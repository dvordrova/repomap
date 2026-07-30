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
		`'What to study': 'Что изучать'`,
	} {
		if !strings.Contains(string(html), want) {
			t.Fatalf("Russian HTML is missing %q", want)
		}
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
		strings.Contains(string(html), `"report_language"`) {
		t.Fatalf("default report language changed: %s", html[:min(len(html), 500)])
	}
}

func TestRussianUITranslationPreservesRepositoryTerms(t *testing.T) {
	t.Parallel()

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	assetPath, err := filepath.Abs(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs");
const vm = require("vm");
const sandbox = {
  URLSearchParams,
  navigator: {},
  window: {
    location: {search: "", hostname: "localhost", protocol: "file:", pathname: "/report.html", hash: ""},
    __REPOMAP_WORKSPACE_TEST__: {},
    addEventListener() {},
  },
  document: {
    createElement(tag) { return {tagName: tag.toUpperCase(), className: "", textContent: ""}; },
    getElementById(id) {
      if (id === "rm-report-data") return {textContent: JSON.stringify({report_language: "ru"})};
      return null;
    },
  },
};
vm.runInNewContext(fs.readFileSync(process.argv[1], "utf8"), sandbox);
const translate = sandbox.window.__REPOMAP_WORKSPACE_TEST__.translateUIString;
const txt = sandbox.window.__REPOMAP_WORKSPACE_TEST__.txt;
process.stdout.write(JSON.stringify([
  translate("What to study"),
  translate("Purpose"),
  translate("Start with runServer →"),
  translate("cli/cmd/run.go · runServer"),
  translate("NATS JetStream handler"),
  translate("1 exact anchor"),
  translate("2 exact anchors"),
  translate("5 exact anchors"),
  translate("cli/cmd/run.go · lines 85–144"),
  translate("1 surface · 1 code path · 2 exact anchors"),
  translate("1 exact member"),
  translate("Code paths (1)"),
  translate("Code paths (1) ▾"),
  translate("Production package github.com/example/project/v0/pkg/service."),
  translate("Open an anchor at its exact repository location."),
  translate("Core data type"),
  translate("Public boundary"),
  translate("Surfaces"),
  translate("1 surface · 1 exact anchor"),
  translate("← Back to reading path"),
  translate("Opened pglogrepl.go:74 in VS Code"),
  translate("Map context for “Как работает репликация?”"),
  translate("Open a concrete source location or continue on the architecture map."),
  translate("System story"),
  translate("How the parts fit together"),
  translate("Inspect the wider repository"),
  translate("Explore architecture"),
  txt("code", "", "Open").textContent,
  txt("span", "", "Open").textContent,
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
		"cli/cmd/run.go · runServer",
		"NATS JetStream handler",
		"1 точная опора",
		"2 точные опоры",
		"5 точных опор",
		"cli/cmd/run.go · строки 85–144",
		"1 точка запуска · 1 путь в коде · 2 точные опоры",
		"1 точный элемент",
		"Пути в коде (1)",
		"Пути в коде (1) ▾",
		"Пакет production-кода github.com/example/project/v0/pkg/service.",
		"Откройте опору в её точной позиции в репозитории.",
		"Основной тип данных",
		"Публичная граница",
		"Точки запуска",
		"1 точка запуска · 1 точная опора",
		"← Назад к пути изучения",
		"Открыто в VS Code: pglogrepl.go:74",
		"Контекст на карте: «Как работает репликация?»",
		"Откройте конкретную позицию в исходниках или перейдите к карте архитектуры.",
		"Как устроена система",
		"Как связаны основные части",
		"Изучить репозиторий шире",
		"Исследовать архитектуру",
		"Open",
		"Открыть",
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

func TestParseRunMetadataRestoresReportLanguage(t *testing.T) {
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
	if data.ReportLanguage != "ru" {
		t.Fatalf("ReportLanguage = %q, want ru", data.ReportLanguage)
	}
}
