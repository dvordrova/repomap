package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPickerLabelUsesReportLanguageAndSecondPrecision(t *testing.T) {
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
const language = process.argv[3];
const report = {
  report_language: language,
  user_mechanisms: [],
  user_sources: [],
  openable_paths: [],
  source_ids: {},
};
const window = {
  location: { search: "", hostname: "127.0.0.1", protocol: "http:", pathname: "/report.html" },
  __REPOMAP_WORKSPACE_TEST__: {},
  addEventListener() {},
};
const document = {
  documentElement: { lang: language },
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
    return null;
  },
  querySelectorAll() { return []; },
};
window.document = document;
const sandbox = {
  window,
  document,
  URLSearchParams,
  Set,
  Map,
  AbortController,
  Intl,
};
vm.createContext(sandbox);
vm.runInContext(
  fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"),
  sandbox
);
vm.runInContext(fs.readFileSync(process.argv[2], "utf8"), sandbox);
const api = window.__REPOMAP_WORKSPACE_TEST__;
const run = {
  id: "20260731-140506-fixture-abcdef012345",
  repo_name: "fixture",
  created_at: "2026-07-31T14:05:06Z",
  report_language: "ru",
  cache_mode: "no-cache",
  short_id: "abcdef012345",
};
const legacy = {
  id: "20260731-140506-fixture-legacy",
  repo_name: "fixture",
  report_language: "en",
  cache_mode: "cache",
};
process.stdout.write(JSON.stringify({
  date: api.runPickerDate(run),
  label: api.runPickerLabel(run),
  legacyDate: api.runPickerDate(legacy),
  legacyLabel: api.runPickerLabel(legacy),
}));
`
	runnerPath := filepath.Join(t.TempDir(), "run-picker-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}

	type result struct {
		Date        string `json:"date"`
		Label       string `json:"label"`
		LegacyDate  string `json:"legacyDate"`
		LegacyLabel string `json:"legacyLabel"`
	}
	run := func(t *testing.T, language string) result {
		t.Helper()
		command := exec.Command(node, runnerPath, assetPath, language)
		command.Env = append(os.Environ(), "LANG=de_DE.UTF-8", "TZ=UTC")
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("run picker asset test: %v\n%s", err, output)
		}
		var got result
		if err := json.Unmarshal(output, &got); err != nil {
			t.Fatalf("decode run picker result: %v\n%s", err, output)
		}
		got.Date = normalizeRunPickerWhitespace(got.Date)
		got.Label = normalizeRunPickerWhitespace(got.Label)
		got.LegacyDate = normalizeRunPickerWhitespace(got.LegacyDate)
		got.LegacyLabel = normalizeRunPickerWhitespace(got.LegacyLabel)
		return got
	}

	english := run(t, "en")
	russian := run(t, "ru")
	if english.Date != "07/31/2026, 14:05:06" ||
		russian.Date != "31.07.2026, 14:05:06" {
		t.Fatalf(
			"picker dates do not follow report language: en=%q ru=%q",
			english.Date,
			russian.Date,
		)
	}
	for language, got := range map[string]result{
		"en": english,
		"ru": russian,
	} {
		for _, part := range []string{
			"fixture · ru · no-cache · ",
			"14:05:06",
			" · abcdef012345",
		} {
			if !strings.Contains(got.Label, part) {
				t.Errorf("%s picker label %q is missing %q", language, got.Label, part)
			}
		}
		if got.LegacyDate != got.Date ||
			!strings.Contains(got.LegacyLabel, "fixture · en · cache · ") ||
			!strings.HasSuffix(got.LegacyLabel, " · legacy") {
			t.Errorf(
				"%s legacy picker fallback = date %q label %q",
				language,
				got.LegacyDate,
				got.LegacyLabel,
			)
		}
	}
}

func normalizeRunPickerWhitespace(value string) string {
	value = strings.ReplaceAll(value, "\u202f", " ")
	return strings.ReplaceAll(value, "\u00a0", " ")
}
