package run

import (
	"bytes"
	"context"
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

// runtimeProgramIndex is a one-object program index for tests that need a
// target and nothing more.
func runtimeProgramIndex(t *testing.T, name, selector, path, fileRef string) programindex.Index {
	t.Helper()
	base, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: "go", Kind: "application", Name: name, Selector: selector,
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
// groups, and the report, with no provider anywhere.
func TestAtlasPathPublishesAReportWithoutTheModel(t *testing.T) {
	repositoryRoot := ordinaryGraphGoRepository(t)
	debugDir := t.TempDir()
	served := 0
	deps := defaultRunDeps{
		ctx: context.Background(), stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{},
		llmBatchConcurrency: 1, llmBatchController: &llm.BatchController{},
		serveReport: func(context.Context, reportserver.Options) error { served++; return nil },
		openReport:  func(string) error { return nil },
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
	if len(value.Targets) != 1 || len(value.Targets[0].Boxes) != 2 || value.Targets[0].Files != 2 {
		t.Fatalf("atlas: %+v", value.Targets)
	}
	for _, box := range value.Targets[0].Boxes {
		for _, file := range box.Files {
			if file.Source != atlas.SourceGiven {
				t.Errorf("%s: source %q without the model", file.Path, file.Source)
			}
		}
	}
	index, err := groupindex.Read(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Groups) != 2 || len(index.Connections) != 1 {
		t.Fatalf("projected groups %d connections %d", len(index.Groups), len(index.Connections))
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
