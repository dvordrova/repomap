package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSavedHTMLUsesOrdinaryRendererAndPreservesPublication(t *testing.T) {
	for _, host := range []string{"local", "GitHub", "GitLab", "GitHub unavailable", "GitLab unavailable"} {
		t.Run(host, func(t *testing.T) {
			runDir := t.TempDir()
			data := reportProgramShellDataFixture(t, "example.com/team/server")
			data.ArtifactsDir = runDir
			data.defaultProgramIndexArtifactFilename = "program-index.json"
			manifest := validRunManifestFixture(t)
			// The saved report is portable; these repository directories need not exist.
			manifest.AnalysisRoot = "/repo/nested"
			source, err := NewRunSource(manifest.AnalysisRoot, manifest.RepositoryState)
			if err != nil {
				t.Fatal(err)
			}
			options := GenerateOptions{Data: &data, PublishHTML: true}
			if strings.HasSuffix(host, " unavailable") {
				source.UnavailableSourcePaths = append([]string(nil), data.OpenablePaths...)
			}
			switch strings.TrimSuffix(host, " unavailable") {
			case "GitHub":
				options.GitHubURL = "https://github.com/example/project"
			case "GitLab":
				options.GitLabURL = "https://gitlab.com/example/project"
			}
			if _, err := Generate(runDir, source, options); err != nil {
				t.Fatal(err)
			}
			before := map[string][]byte{}
			for _, name := range []string{"report.html", "report.json", RunManifestFilename} {
				before[name], err = os.ReadFile(filepath.Join(runDir, name))
				if err != nil {
					t.Fatal(err)
				}
			}
			html, err := RenderSavedHTML(runDir)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(html, before["report.html"]) {
				t.Fatal("saved rendering differs from ordinary publication, including exact source links and data stamp")
			}
			for name, want := range before {
				got, err := os.ReadFile(filepath.Join(runDir, name))
				if err != nil || !bytes.Equal(got, want) {
					t.Fatalf("saved rendering changed %s: %v", name, err)
				}
			}
		})
	}
}

func TestSavedHTMLReusesExactTranslationsAfterDisplayTraversalChanges(t *testing.T) {
	runDir := t.TempDir()
	data := reportProgramShellDataFixture(t, "example.com/team/server")
	data.ArtifactsDir, data.defaultProgramIndexArtifactFilename = runDir, "program-index.json"
	data.ReadmeOverview = "This server prepares a response."
	manifest := validRunManifestFixture(t)
	source, err := NewRunSource(manifest.AnalysisRoot, manifest.RepositoryState)
	if err != nil {
		t.Fatal(err)
	}
	page, err := PreparePage(&data, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	current := page.TextCatalog()
	if len(current.Entries) < 2 {
		t.Fatal("fixture needs distinct display texts")
	}
	translations := DisplayTranslations{Version: DisplayTextVersion, Language: Russian, CatalogSHA256: current.SHA256, Entries: []DisplayTranslationEntry{}}
	for i, entry := range current.Entries {
		text := fmt.Sprintf("Перевод %d: %s", i+1, entry.Text)
		translations.Entries = append(translations.Entries, DisplayTranslationEntry{Ref: entry.Ref, Text: text})
	}
	receipt, err := Generate(runDir, source, GenerateOptions{Data: &data, PublishHTML: true, Render: RenderOptions{Language: Russian, Translations: &translations}})
	if err != nil {
		t.Fatal(err)
	}
	// A saved publication can have discovered these identical texts in the
	// opposite order. Its translated bytes remain attached to those texts.
	entries := append([]DisplayTextEntry{}, current.Entries...)
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	saved := reorderedDisplayCatalog(entries)
	savedTranslations, err := rebindDisplayTranslations(current, saved, translations)
	if err != nil {
		t.Fatal(err)
	}
	for filename, value := range map[string]any{"report-display-texts.json": saved, "report-translations.ru.json": savedTranslations} {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(runDir, filename), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	before := map[string][]byte{}
	for _, name := range []string{"report.json", RunManifestFilename, receipt.HTMLFilename(), "report-display-texts.json", "report-translations.ru.json"} {
		before[name], err = os.ReadFile(filepath.Join(runDir, name))
		if err != nil {
			t.Fatal(err)
		}
	}
	rendered, err := RenderSavedHTML(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rendered, before[receipt.HTMLFilename()]) {
		t.Fatal("reordered saved translations changed ordinary HTML")
	}
	for name, want := range before {
		got, err := os.ReadFile(filepath.Join(runDir, name))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("render changed saved publication %s: %v", name, err)
		}
	}
}
