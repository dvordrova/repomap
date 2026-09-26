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
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/reportserver"
)

// The closed decisions have no model but Jev: a model run without JEV_KEY,
// or without any categorizer, stops before any artifact, corpus or model
// call instead of degrading.
func TestAModelRunWithoutJevKeyStopsBeforeAnalysis(t *testing.T) {
	t.Setenv("JEV_KEY", "")
	repositoryRoot := t.TempDir()
	for want, factory := range map[string]func() (llm.Categorizer, error){
		"JEV_KEY is required":                         newJevCategorizer,
		"no categorizer answers the closed decisions": nil,
	} {
		debugDir := filepath.Join(t.TempDir(), "runs")
		calls := 0
		provider := func() (llm.Provider, error) { calls++; return nil, fmt.Errorf("a model was configured") }
		err := runDefaultWithDeps(repositoryRoot, []string{"--no-open", "--debug-dir", debugDir}, defaultRunDeps{
			ctx: t.Context(), stdout: io.Discard, stderr: io.Discard,
			newTargetPortfolioProvider: provider, newCubeProvider: provider, newCategorizer: factory,
		})
		if err == nil || !strings.Contains(err.Error(), want) || calls != 0 {
			t.Fatalf("%s: error %v after %d model factories", want, err, calls)
		}
		if _, err := os.Stat(debugDir); !os.IsNotExist(err) {
			t.Fatalf("%s: the run created %s: %v", want, debugDir, err)
		}
	}
}

// runtimeProgramIndex is a one-object program index for tests that need a
// target and nothing more.
func runtimeProgramIndex(t *testing.T, targetID, name, selector, path, fileRef string) programindex.Index {
	t.Helper()
	base, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			ID: targetID, Language: "go", Kind: "application", Name: name, Selector: selector,
			Sources: []programindex.TargetSource{{FileRef: fileRef, Path: path}}, AnchorFileRef: fileRef,
		},
		Objects: []programindex.ObjectInput{{
			SourceRef: "entry", Kind: programindex.ObjectFunction, Name: "entry",
			Visibility: programindex.VisibilityPublic,
			Location:   &programindex.Location{Path: path, Line: 1, Column: 1},
		}},
		Relations: []programindex.RelationInput{},
		Coverage:  programindex.CoverageInput{Measured: true, ObjectsObserved: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	return base
}

// TestAtlasPathPublishesAReportWithoutTheModel drives the whole ordinary
// path in process: target selection by exact selector, the Go index, facts
// and claims, the atlas tables on their fallback lines, the projected
// groups, and the report, with no provider anywhere and no JEV_KEY.
func TestAtlasPathPublishesAReportWithoutTheModel(t *testing.T) {
	t.Setenv("JEV_KEY", "")
	repositoryRoot := ordinaryGraphGoRepository(t)
	debugDir := t.TempDir()
	served := 0
	deps := defaultRunDeps{
		ctx: context.Background(), stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{},
		llmBatchConcurrency: 1, llmBatchController: &llm.BatchController{},
		serveReport: func(context.Context, reportserver.Options) error { served++; return nil },
		openReport:  func(string) error { return nil },
		newCategorizer: func() (llm.Categorizer, error) {
			t.Error("a --no-model run built the categorizer")
			return newJevCategorizer()
		},
	}
	err := runDefaultWithDeps(repositoryRoot, []string{
		"--no-model", "--target", "example.com/common-page@.::example.com/common-page/cmd/app",
		"--no-open", "--debug-dir", debugDir,
	}, deps)
	if err != nil {
		t.Fatalf("atlas run: %v\n%s", err, deps.stderr.(*bytes.Buffer).String())
	}
	runDirs := ordinaryGraphRunDirs(t, debugDir)
	if len(runDirs) != 1 {
		t.Fatalf("run dirs: %v", runDirs)
	}
	runDir := runDirs[0]
	for _, name := range []string{atlas.GraphFilename, atlas.ArtifactFilename, atlas.TablesFilename, groupindex.ArtifactFilename, "report.html"} {
		if _, err := os.Stat(filepath.Join(runDir, name)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	value, err := atlas.Read(runDir)
	if err != nil {
		t.Fatal(err)
	}
	// Without a model there is no map of parts: an explicit map failure with
	// every file off the map, never an invented grouping.
	if len(value.Targets) != 1 || len(value.Targets[0].Boxes) != 0 || len(value.Targets[0].OffMap) != 2 || value.Targets[0].Files != 2 || value.Targets[0].MapFailure == "" {
		t.Fatalf("atlas: %+v", value.Targets)
	}
	for _, entry := range value.Targets[0].OffMap {
		if entry.File.Source != atlas.SourceGiven || entry.Reason != atlas.OffMapFailure {
			t.Errorf("%s: source %q reason %q without the model", entry.File.Path, entry.File.Source, entry.Reason)
		}
	}
	index, err := groupindex.Read(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Groups) != 0 || len(index.OffMap) != 2 || index.MapFailure == "" {
		t.Fatalf("projected groups %d off-map %d failure %q", len(index.Groups), len(index.OffMap), index.MapFailure)
	}
	page, err := os.ReadFile(filepath.Join(runDir, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"The map of parts is unavailable: ", "Not on the map", "internal/work/work.go", "cmd/app/main.go"} {
		if !strings.Contains(string(page), want) {
			t.Errorf("report lacks %q", want)
		}
	}
	data, err := report.ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if data.GroupGraph == nil || data.Orientation != nil {
		t.Fatalf("report data: group graph %v orientation %v", data.GroupGraph != nil, data.Orientation != nil)
	}
	if served != 1 {
		t.Fatalf("served %d times", served)
	}
}
