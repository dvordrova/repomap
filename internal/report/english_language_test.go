package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportRenderingIsEnglishOnly(t *testing.T) {
	t.Parallel()

	data := reportProgramShellDataFixture(t, "fixture")
	html, err := RenderHTMLWithOptions(&data, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<html lang="en">`,
		`This report requires JavaScript.`,
		`id="rm-report-app-js"`,
	} {
		if !strings.Contains(string(html), want) {
			t.Fatalf("English HTML is missing %q", want)
		}
	}
	for _, unwanted := range []string{`"report_language"`, "rm-localization-status"} {
		if strings.Contains(string(html), unwanted) {
			t.Fatalf("English HTML retained removed presentation state %q", unwanted)
		}
	}
}

func TestRunMetadataCannotActivateRemovedReportLanguage(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "metadata.json")
	if err := os.WriteFile(
		path,
		[]byte(`{"repo_name":"fixture","effective_options":{"report_language":"ru"}}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	data := reportProgramShellDataFixture(t, "fixture")
	if err := parseRunMetadata(path, &data); err != nil {
		t.Fatal(err)
	}
	html, err := RenderHTMLWithOptions(&data, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(html), `<html lang="en">`) ||
		strings.Contains(string(html), `"report_language"`) {
		t.Fatalf("metadata activated removed report language: %s", html[:min(len(html), 500)])
	}
}
