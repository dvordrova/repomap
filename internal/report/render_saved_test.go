package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/terminology"
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

// Rendering one saved run again gives the same bytes. Glossary names that read
// the same words ("LOAD" as an acronym and "load" in any case in "clean
// calls load") had been settled by map order, so freqtrade's display refs
// shifted (t1235/t1236) between two renders of one saved run with one binary.
func TestRenderingASavedRunAgainGivesTheSameBytes(t *testing.T) {
	runDir := t.TempDir()
	data := reportProgramShellDataFixture(t, "example.com/team/server")
	data.ArtifactsDir, data.defaultProgramIndexArtifactFilename = runDir, "program-index.json"
	source := []terminology.Source{{Path: "main.py", Line: 1}}
	data.Glossary = pageGlossaryCatalog(t,
		[]terminology.Candidate{{Name: "LOAD", Explanation: "The loader command.", Sources: source}},
		[]terminology.Candidate{{Name: "load", Explanation: "Reading saved input.", Sources: source}})
	manifest := validRunManifestFixture(t)
	runSource, err := NewRunSource(manifest.AnalysisRoot, manifest.RepositoryState)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(runDir, runSource, GenerateOptions{Data: &data, PublishHTML: true}); err != nil {
		t.Fatal(err)
	}
	published, err := os.ReadFile(filepath.Join(runDir, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(published, []byte("The loader command.")) || !bytes.Contains(published, []byte("Reading saved input.")) {
		t.Fatal("the fixture's two glossary names are not on the page")
	}
	for render := 1; render <= 8; render++ {
		again, err := RenderSavedHTML(runDir)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(again, published) {
			t.Fatalf("render %d of the same saved run differs from its publication", render)
		}
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

// A rendering only presents what analysis saved (owner, 2026-10-01: "у
// html должна быть простая задача — вот данные, показываю"): rendering a
// saved run, and rendering a report's data in memory, never runs
// GroupsIndex's Derive. Its reaches, spines, dispatch sites, catalogues,
// launch walk and phases, and those of the overview's test-free views,
// are hydrated from the saved index.
func TestARenderingNeverDerivesTheGroupsIndex(t *testing.T) {
	runDir := t.TempDir()
	data := reportProgramShellDataFixture(t, "example.com/team/server")
	data.ArtifactsDir, data.defaultProgramIndexArtifactFilename = runDir, "program-index.json"
	manifest := validRunManifestFixture(t)
	runSource, err := NewRunSource(manifest.AnalysisRoot, manifest.RepositoryState)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(runDir, runSource, GenerateOptions{Data: &data, PublishHTML: true}); err != nil {
		t.Fatal(err)
	}
	before := groupindex.Derivations()
	if _, err := RenderSavedHTML(runDir); err != nil {
		t.Fatal(err)
	}
	if derived := groupindex.Derivations() - before; derived != 0 {
		t.Fatalf("rendering a saved run derived the GroupsIndex %d times", derived)
	}
	memory := reportProgramShellDataFixture(t, "example.com/team/server")
	options := reportSingleTargetRenderOptionsFixture(t, &memory)
	before = groupindex.Derivations()
	if _, err := RenderHTMLWithOptions(&memory, options); err != nil {
		t.Fatal(err)
	}
	if derived := groupindex.Derivations() - before; derived != 0 {
		t.Fatalf("rendering a report's data derived the GroupsIndex %d times", derived)
	}
}
