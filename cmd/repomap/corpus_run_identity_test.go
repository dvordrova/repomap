package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCorpusAssessesExactLatestDefaultInsteadOfNewerSibling(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repo := corpusRepo{Repository: "github/example", Kind: "service"}
	runsDir := corpusRunIdentityFixture(t, root, repo,
		"20260809-120000-example-default",
		"20260809-120001-example-sibling",
	)
	writeCorpusIdentityReport(t,
		filepath.Join(runsDir, "20260809-120000-example-default"),
		"default-revision",
	)
	writeCorpusIdentityReport(t,
		filepath.Join(runsDir, "20260809-120001-example-sibling"),
		"sibling-revision",
	)
	if err := os.Symlink("20260809-120000-example-default", filepath.Join(runsDir, "latest")); err != nil {
		t.Fatal(err)
	}
	if got := latestCorpusRun(root, repo.Repository); got != filepath.Join(runsDir, "20260809-120000-example-default") {
		t.Fatalf("latest corpus run = %q, want selected default", got)
	}

	matrixPath := writeCorpusIdentityMatrix(t, corpusMatrix{Repositories: []corpusRepo{repo}})
	var stdout bytes.Buffer
	err := runCorpusCLI([]string{root, "--matrix", matrixPath}, &stdout)
	if err == nil || !strings.Contains(err.Error(), "1 publication(s) failed integrity") {
		t.Fatalf("corpus error = %v, want expected fixture integrity failure", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, "corpus_acceptance.json"))
	if err != nil {
		t.Fatal(err)
	}
	var facts []corpusRunFacts
	if err := json.Unmarshal(raw, &facts); err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 ||
		facts[0].RunID != "20260809-120000-example-default" ||
		facts[0].Revision != "default-revision" {
		t.Fatalf("assessed facts = %#v, want exact latest default", facts)
	}
	if !strings.Contains(stdout.String(), "run=20260809-120000-example-default") ||
		strings.Contains(stdout.String(), "run=20260809-120001-example-sibling") {
		t.Fatalf("corpus output did not attribute the exact default:\n%s", stdout.String())
	}
}

func TestCorpusRunIdentityLatestToMatrixRoundTrip(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repo := corpusRepo{Repository: "github/example", Kind: "service"}
	runsDir := corpusRunIdentityFixture(t, root, repo,
		"20260809-120000-example-default",
		"20260809-120001-example-sibling",
	)
	if err := os.Symlink("20260809-120000-example-default", filepath.Join(runsDir, "latest")); err != nil {
		t.Fatal(err)
	}

	selected, err := resolveCorpusRun(root, repo)
	if err != nil {
		t.Fatalf("resolve legacy latest: %v", err)
	}
	repo.RunID = selected.RunID
	raw, err := json.Marshal(corpusMatrix{Repositories: []corpusRepo{repo}})
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip corpusMatrix
	if err := json.Unmarshal(raw, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if len(roundTrip.Repositories) != 1 || roundTrip.Repositories[0].RunID != selected.RunID {
		t.Fatalf("matrix round trip = %#v", roundTrip)
	}

	// Changing latest after the launcher recorded the default must not retarget
	// the acceptance gate to a sibling page.
	if err := os.Remove(filepath.Join(runsDir, "latest")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("20260809-120001-example-sibling", filepath.Join(runsDir, "latest")); err != nil {
		t.Fatal(err)
	}
	restored, err := resolveCorpusRun(root, roundTrip.Repositories[0])
	if err != nil {
		t.Fatalf("resolve exact matrix run: %v", err)
	}
	if restored != selected {
		t.Fatalf("restored selection = %#v, want %#v", restored, selected)
	}
}

func TestCorpusRunIdentityRejectsTraversalUnknownAndDuplicate(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repo := corpusRepo{Repository: "github/example", Kind: "service"}
	corpusRunIdentityFixture(t, root, repo, "20260809-120000-example-default")

	for _, test := range []struct {
		name  string
		runID string
		want  string
	}{
		{name: "traversal", runID: "../outside", want: "invalid run_id"},
		{name: "unknown", runID: "20260809-120999-example-unknown", want: "run id is unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := repo
			candidate.RunID = test.runID
			if _, err := resolveCorpusRun(root, candidate); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("resolve error = %v, want %q", err, test.want)
			}
		})
	}

	duplicate := corpusMatrix{Repositories: []corpusRepo{
		{Repository: repo.Repository, Kind: "service", RunID: "20260809-120000-example-default"},
		{Repository: repo.Repository, Kind: "service", RunID: "20260809-120000-example-default"},
	}}
	matrixPath := writeCorpusIdentityMatrix(t, duplicate)
	var stdout bytes.Buffer
	if err := runCorpusCLI([]string{root, "--matrix", matrixPath}, &stdout); err == nil ||
		!strings.Contains(err.Error(), "duplicate repository") {
		t.Fatalf("duplicate matrix error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("duplicate matrix emitted partial acceptance output: %q", stdout.String())
	}
}

func corpusRunIdentityFixture(t *testing.T, root string, repo corpusRepo, runIDs ...string) string {
	t.Helper()
	encoded := strings.ReplaceAll(repo.Repository, "/", "__")
	runsDir := filepath.Join(root, repo.Kind+"__"+encoded, "runs")
	for _, runID := range runIDs {
		if err := os.MkdirAll(filepath.Join(runsDir, runID), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return runsDir
}

func writeCorpusIdentityReport(t *testing.T, runDir, revision string) {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"captured_revision": revision})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "report.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeCorpusIdentityMatrix(t *testing.T, matrix corpusMatrix) string {
	t.Helper()
	raw, err := json.Marshal(matrix)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "matrix.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
