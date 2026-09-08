package run

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/documentationreduce"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/reportserver"
	"github.com/dvordrova/repomap/internal/targetoutcome"
)

// No ProgramIndex, GroupsIndex, dependency or documentation files exist here.
// Publication and serving must consume the values the producer already owns.
func TestRepositoryReportPublishesOnceAndServesFromMemory(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repo, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source, err := report.NewRunSource(repo, freshness.RepositoryState{
		Version: freshness.RepositoryStateVersion, Identity: repo, Head: strings.Repeat("a", 40),
	})
	if err != nil {
		t.Fatal(err)
	}
	docs, err := documentationreduce.Run(ctx, llm.Executor{}, nil, readmetargetscout.GuidanceSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	var runs []targetPublishedRun
	var outcomes []targetoutcome.Outcome
	for i := 0; i < 27; i++ {
		name := fmt.Sprintf("part-%02d", i)
		index := runtimeProgramIndex(t, name, "go:"+name, name+"/main.go", name)
		groups, err := groupindex.Empty(index)
		if err != nil {
			t.Fatal(err)
		}
		runID := "run-" + name
		dir := filepath.Join(root, runID)
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(`{"repo_name":"fixture"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		page, err := report.TargetNavigationPageFor(dir, index.Target)
		if err != nil {
			t.Fatal(err)
		}
		catalog := dependencies.Empty()
		runs = append(runs, targetPublishedRun{
			RunID: runID, RunDir: dir, ProgramPage: page, ProgramIndex: &index,
			GroupIndex: groups, Dependencies: &catalog, Documentation: &docs,
			RepoName: "fixture", Source: source,
		})
		selected, err := targetoutcome.NewSelectedTarget(targetoutcome.LanguageGroupGo, targetoutcome.ScopeLibrary, name, index.Target.Selector)
		if err != nil {
			t.Fatal(err)
		}
		analyzed, err := targetoutcome.NewAnalyzed(selected, index.Target, runID)
		if err != nil {
			t.Fatal(err)
		}
		outcomes = append(outcomes, analyzed)
	}
	portfolio, err := buildProgramPagePortfolio(runs, runs[0].RunID)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := targetoutcome.Build(outcomes[0].SelectedTarget.ID, outcomes)
	if err != nil {
		t.Fatal(err)
	}
	question := &atlas.QuestionRoute{Version: 7, Question: "Where is state stored?", Revision: strings.Repeat("a", 40), Stops: []atlas.QuestionStop{{Path: "part-00/main.go", Line: 1, Name: "Open", Why: "Inspect the storage entry."}}, Guide: &atlas.QuestionGuide{State: "partial", Steps: []atlas.QuestionStep{{Path: "part-00/main.go", Line: 1, StopIndexes: []int{0}}}}}
	receipt, err := publishRepositoryReport(ctx, portfolio, inventory, runs, atlasOutcome{Questions: []atlas.QuestionRoute{*question, {Version: 7, Question: "How do I run it?", Revision: question.Revision}}}, repositoryTargetDispatchOptions{Output: newRunOutput(io.Discard)})
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt.Data().GroupGraph.Indexes) != len(runs) {
		t.Fatal("report lost target graphs")
	}
	for i, run := range runs {
		for _, name := range []string{"report.json", "report.html", report.RunManifestFilename} {
			_, err := os.Stat(filepath.Join(run.RunDir, name))
			if i == 0 && err != nil {
				t.Fatal(err)
			}
			if i != 0 && !os.IsNotExist(err) {
				t.Fatalf("duplicated repository artifact: %s/%s", run.RunID, name)
			}
		}
	}
	// A new process only needs the common report and its manifest.
	restored, err := report.ReadRunReceipt(runs[0].RunDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.Data().GroupGraph.Indexes) != 27 {
		t.Fatal("saved report lost targets")
	}
	if len(restored.Data().Questions) != 2 || restored.Data().Questions[0].Question != question.Question {
		t.Fatal("question result was not handed through memory into the common report")
	}
	html, err := os.ReadFile(filepath.Join(runs[0].RunDir, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{question.Question, "How do I run it?", "class=\"reading-guide\"", "Inspect the storage entry.", "This reading route is incomplete"} {
		if !strings.Contains(string(html), expected) {
			t.Fatalf("reading guide omits %q", expected)
		}
	}
	t.Run("render command reuses the complete saved report without provider setup", func(t *testing.T) {
		// Invalid provider settings must be irrelevant to rendering saved data.
		t.Setenv("REPOMAP_LLM_ENDPOINT", ":invalid")
		t.Setenv("REPOMAP_LLM_CHAT_TEMPLATE_KWARGS", "invalid JSON")
		output := filepath.Join(t.TempDir(), "current.html")
		if err := runRender([]string{runs[0].RunDir, "--output", output}, io.Discard); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(output)
		if err != nil || !bytes.Equal(got, html) {
			t.Fatalf("CLI rendering changed the ordinary 27-component page: %v", err)
		}
		if err := runRender([]string{t.TempDir(), "--output", output}, io.Discard); err == nil {
			t.Fatal("missing report was silently regenerated")
		}
		got, err = os.ReadFile(output)
		if err != nil || !bytes.Equal(got, html) {
			t.Fatalf("failed rendering changed the previous HTML: %v", err)
		}
	})
	t.Run("Russian no-model publication keeps the common page and never opens a provider", func(t *testing.T) {
		providerCalls := 0
		localized, err := publishRepositoryReport(ctx, portfolio, inventory, runs, atlasOutcome{}, repositoryTargetDispatchOptions{
			Output: newRunOutput(io.Discard), DisplayLanguage: report.Russian, NoModel: true,
			Deps: defaultRunDeps{newCubeProvider: func() (llm.Provider, error) {
				providerCalls++
				return nil, fmt.Errorf("no-model publication opened a provider")
			}},
		})
		if err != nil || providerCalls != 0 {
			t.Fatalf("no-model translation: calls=%d, error=%v", providerCalls, err)
		}
		expectedFilename, filenameErr := report.ReportHTMLFilename(filepath.Base(runs[0].Source.Repository.Identity), report.Russian)
		if filenameErr != nil || localized.HTMLFilename() != expectedFilename || len(localized.RenderOptions().Translations.Entries) != 0 {
			t.Fatal("no-model publication lost its localized dictionary-only display")
		}
		for i, run := range runs {
			matches, err := filepath.Glob(filepath.Join(run.RunDir, "report*.html"))
			if err != nil || i == 0 && len(matches) != 1 || i != 0 && len(matches) != 0 {
				t.Fatalf("localized common-page count in %s: %v, %v", run.RunID, matches, err)
			}
		}
		restored, err := report.ReadRunReceipt(runs[0].RunDir)
		if err != nil || !restored.RenderOptions().NoModel || restored.RenderOptions().Language != report.Russian {
			t.Fatalf("restored dictionary-only display: %v", err)
		}
		html, err := report.RenderHTMLWithOptions(restored.Data(), restored.RenderOptions())
		if err != nil || !strings.Contains(string(html), `<html lang="ru">`) {
			t.Fatalf("Russian no-model rerender: %v", err)
		}
	})
	if err := os.RemoveAll(runs[0].RunDir); err != nil {
		t.Fatal(err)
	}
	_, err = reportserver.NewHandler(reportserver.Options{
		RunsDir: root, InitialRunID: runs[0].RunID, Runs: []report.RunReceipt{receipt},
		OpenFile: func(context.Context, string, int, int) error { return nil },
	})
	if err != nil {
		t.Fatalf("server reread deleted publication: %v", err)
	}
}

func TestRestoredTargetIndexIsLoadedOnlyOnce(t *testing.T) {
	index := runtimeProgramIndex(t, "api", "go:api", "main.go", "main")
	run := targetPublishedRun{RunDir: t.TempDir()}
	if err := programindex.Persist(run.RunDir, programindex.ArtifactFilename, index); err != nil {
		t.Fatal(err)
	}
	first, err := run.programIndex()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(run.RunDir, programindex.ArtifactFilename)); err != nil {
		t.Fatal(err)
	}
	second, err := run.programIndex()
	if err != nil || first.SHA256 != second.SHA256 {
		t.Fatalf("second read: %s, %v", second.SHA256, err)
	}
}
