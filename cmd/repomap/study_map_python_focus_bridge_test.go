package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/report"
)

func TestRecordedBeetsPythonFocusReachesStudyAnchor(t *testing.T) {
	repoPath := os.Getenv("REPOMAP_BEETS_REPO")
	if repoPath == "" {
		t.Skip("set REPOMAP_BEETS_REPO to run the recorded Beets Study bridge discriminator")
	}

	raw, err := os.ReadFile(filepath.Join(
		"..", "..", "experiments", "semantic-map", "beets.python-selection.json",
	))
	if err != nil {
		t.Fatal(err)
	}
	var recorded struct {
		Repository struct {
			Revision string `json:"revision"`
		} `json:"repository"`
		SelectedSymbols []struct {
			Name      string `json:"name"`
			Path      string `json:"path"`
			StartLine int    `json:"start_line"`
		} `json:"selected_symbols"`
	}
	if err := json.Unmarshal(raw, &recorded); err != nil {
		t.Fatal(err)
	}
	revision, err := exec.Command("git", "-C", repoPath, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(revision)); got != recorded.Repository.Revision {
		t.Fatalf("Beets revision = %q, want recorded %q", got, recorded.Repository.Revision)
	}

	var focusPath string
	var focusLine int
	for _, symbol := range recorded.SelectedSymbols {
		if symbol.Name == "_get_plugin" {
			focusPath = symbol.Path
			focusLine = symbol.StartLine
			break
		}
	}
	if focusPath != "beets/plugins.py" || focusLine != 406 {
		t.Fatalf("recorded Python focus = %s:%d, want beets/plugins.py:406", focusPath, focusLine)
	}

	plan, err := modelresearch.PlanTargetedRounds(
		context.Background(),
		modelresearch.PlanningInput{
			RepoPath: repoPath,
			Questions: []modelresearch.ProposedQuestion{{
				ID:           "plugin-loading",
				Purpose:      "Understand the plugin interface and loading mechanism.",
				Question:     "How does a configured Beets plugin become an executable command?",
				CandidateIDs: []string{"plugins"},
			}},
			Candidates: []modelresearch.FileCandidate{{
				ID: "plugins", Path: focusPath, Kind: "source", Score: 50,
				FocusLocations: []evidence.Location{{Path: focusPath, Line: focusLine}},
			}},
			InitialProviderPaths: []string{focusPath},
			Universe: modelresearch.LocalRepositoryUniverse{
				AuthorizedPaths: []string{focusPath},
			},
			Policy: modelresearch.DefaultPolicy(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Selected) != 1 {
		t.Fatalf("selected rounds = %#v", plan)
	}

	var focused modelresearch.EvidenceItem
	for _, item := range plan.Selected[0].Bundle.Evidence {
		if item.Kind == modelresearch.EvidenceSource && item.Window != nil {
			focused = item
			break
		}
	}
	if focused.Window == nil {
		t.Fatal("focused source window was not emitted")
	}
	sourceText := strings.Join(focused.Window.Lines, "\n")
	if focused.Window.StartLine != 366 || focused.Window.EndLine != 445 ||
		!strings.Contains(sourceText, "def _get_plugin") ||
		!strings.Contains(sourceText, "issubclass(obj, BeetsPlugin)") {
		t.Fatalf(
			"focused source window = %d-%d\n%s",
			focused.Window.StartLine,
			focused.Window.EndLine,
			sourceText,
		)
	}

	runDir := t.TempDir()
	state := modelresearch.NewState(
		modelresearch.DefaultPolicy(),
		modelresearch.RepositoryContext{
			Identity: "beets-recorded-focus", Revision: recorded.Repository.Revision, Scenario: "default",
		},
	)
	state.Theory.GroundedFacts = []modelresearch.EvidenceItem{focused}
	if err := modelresearch.WriteState(runDir, state); err != nil {
		t.Fatal(err)
	}
	bundle, err := buildStudyMapBundle(runDir, repoPath, &report.ReportData{
		RepoName: "beets-recorded-focus", OpenablePaths: []string{focusPath},
	})
	if err != nil {
		t.Fatal(err)
	}

	var found bool
	for _, anchor := range bundle.Anchors {
		if anchor.Path == focusPath && anchor.Symbol == "_get_plugin" && anchor.Line == focusLine {
			if anchor.ExactSource == nil || anchor.Function.Path != "" {
				t.Fatalf("Python Study anchor selected the wrong source arm: %#v", anchor)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Study anchors = %#v, want exact _get_plugin:406", bundle.Anchors)
	}
	if len(bundle.Mechanisms) != 0 {
		t.Fatalf("provider-free bridge invented complete mechanisms: %#v", bundle.Mechanisms)
	}
}
