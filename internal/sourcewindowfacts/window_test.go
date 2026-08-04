package sourcewindowfacts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/modelresearch"
)

func TestNewWindowDerivesBoundsAndContentHash(t *testing.T) {
	lines := []string{"func serve() {", "\treturn", "}"}
	window, err := NewWindow("evidence-serve", "router.go", 12, lines)
	if err != nil {
		t.Fatal(err)
	}
	lines[0] = "changed after construction"
	if window.EndLine != 14 || window.Lines[0] != "func serve() {" || len(window.ContentSHA256) != 64 {
		t.Fatalf("NewWindow() = %#v", window)
	}
	if err := window.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestLoadRunVerifiesRepositoryLinesAndDeduplicatesIDs(t *testing.T) {
	repo := t.TempDir()
	run := t.TempDir()
	writeSourceWindowTestFile(t, repo, "router.go", strings.Join([]string{
		"package router",
		"",
		"func serve() {",
		"\thelper()",
		"}",
		"",
	}, "\n"))
	item := sourceWindowTestEvidence(
		"evidence-serve",
		"router.go",
		3,
		[]string{"func serve() {", "\thelper()", "}"},
	)
	writeSourceWindowTestBundle(t, run, "research-1", item)
	writeSourceWindowTestBundle(t, run, "research-2", item)

	windows, err := LoadRun(run, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(windows) != 1 {
		t.Fatalf("LoadRun() returned %d windows, want 1: %#v", len(windows), windows)
	}
	window := windows[0]
	if window.EvidenceID != item.ID || window.Path != "router.go" ||
		window.StartLine != 3 || window.EndLine != 5 || len(window.ContentSHA256) != 64 {
		t.Fatalf("LoadRun() window = %#v", window)
	}
}

func TestLoadRunRejectsInvalidOrStaleWindows(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*modelresearch.EvidenceItem)
		repoSource string
		want       string
	}{
		{
			name: "not code bearing",
			mutate: func(item *modelresearch.EvidenceItem) {
				item.Window.CodeBearing = false
			},
			want: "not code-bearing",
		},
		{
			name: "truncated",
			mutate: func(item *modelresearch.EvidenceItem) {
				item.Window.Truncated = true
			},
			want: "truncated",
		},
		{
			name: "inconsistent end",
			mutate: func(item *modelresearch.EvidenceItem) {
				item.Window.EndLine++
			},
			want: "end line",
		},
		{
			name: "location mismatch",
			mutate: func(item *modelresearch.EvidenceItem) {
				item.Location.Line++
			},
			want: "location",
		},
		{
			name: "non portable path",
			mutate: func(item *modelresearch.EvidenceItem) {
				item.Location.Path = "../router.go"
			},
			want: "repository-relative",
		},
		{
			name:       "stale contents",
			repoSource: "package router\n\nfunc serve() {\n\tchanged()\n}\n",
			want:       "differ",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := t.TempDir()
			run := t.TempDir()
			repoSource := "package router\n\nfunc serve() {\n\thelper()\n}\n"
			if test.repoSource != "" {
				repoSource = test.repoSource
			}
			writeSourceWindowTestFile(t, repo, "router.go", repoSource)
			item := sourceWindowTestEvidence(
				"evidence-serve",
				"router.go",
				3,
				[]string{"func serve() {", "\thelper()", "}"},
			)
			if test.mutate != nil {
				test.mutate(&item)
			}
			writeSourceWindowTestBundle(t, run, "research-1", item)
			_, err := LoadRun(run, repo)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("LoadRun() error = %v, want fragment %q", err, test.want)
			}
		})
	}
}

func TestLoadRunForDiscoverySkipsTruncatedAndKeepsValidWindows(t *testing.T) {
	repo := t.TempDir()
	run := t.TempDir()
	writeSourceWindowTestFile(t, repo, "router.go", strings.Join([]string{
		"package router",
		"",
		"func truncated() { oldHelper() }",
		"",
		"func serve() { helper() }",
		"",
	}, "\n"))
	truncated := sourceWindowTestEvidence(
		"evidence-truncated",
		"router.go",
		3,
		[]string{"func truncated() { oldHelper() }"},
	)
	truncated.Window.Truncated = true
	valid := sourceWindowTestEvidence(
		"evidence-serve",
		"router.go",
		5,
		[]string{"func serve() { helper() }"},
	)
	writeSourceWindowTestBundle(t, run, "research-1", truncated, valid)

	if _, err := LoadRun(run, repo); err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("strict LoadRun() error = %v, want truncated", err)
	}
	windows, err := LoadRunForDiscovery(run, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(windows) != 1 || windows[0].EvidenceID != valid.ID {
		t.Fatalf("LoadRunForDiscovery() = %#v, want only %q", windows, valid.ID)
	}
}

func TestLoadRunRejectsConflictingDuplicateEvidenceID(t *testing.T) {
	repo := t.TempDir()
	run := t.TempDir()
	writeSourceWindowTestFile(t, repo, "one.go", "package fixture\n\nfunc one() {}\n")
	writeSourceWindowTestFile(t, repo, "two.go", "package fixture\n\nfunc two() {}\n")
	writeSourceWindowTestBundle(
		t,
		run,
		"research-1",
		sourceWindowTestEvidence("evidence-shared", "one.go", 3, []string{"func one() {}"}),
	)
	writeSourceWindowTestBundle(
		t,
		run,
		"research-2",
		sourceWindowTestEvidence("evidence-shared", "two.go", 3, []string{"func two() {}"}),
	)

	_, err := LoadRun(run, repo)
	if err == nil || !strings.Contains(err.Error(), "conflicting") {
		t.Fatalf("LoadRun() error = %v, want conflicting duplicate", err)
	}
}

func sourceWindowTestEvidence(
	id string,
	path string,
	startLine int,
	lines []string,
) modelresearch.EvidenceItem {
	location := evidence.Location{Path: path, Line: startLine}
	return modelresearch.EvidenceItem{
		ID:        id,
		Kind:      modelresearch.EvidenceSource,
		Statement: "bounded source window selected locally for the research question",
		Location:  &location,
		Certainty: evidence.CertaintyStatic,
		Window: &modelresearch.SourceWindow{
			StartLine:   startLine,
			EndLine:     startLine + len(lines) - 1,
			Lines:       append([]string(nil), lines...),
			CodeBearing: true,
		},
	}
}

func writeSourceWindowTestBundle(
	t *testing.T,
	run string,
	round string,
	items ...modelresearch.EvidenceItem,
) {
	t.Helper()
	dir := filepath.Join(run, "research", round)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	bundle := modelresearch.EvidenceBundle{
		Version:  modelresearch.ContractVersion,
		RoundID:  round,
		Evidence: items,
	}
	raw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "evidence_bundle.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeSourceWindowTestFile(t *testing.T, root, path, contents string) {
	t.Helper()
	filePath := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
