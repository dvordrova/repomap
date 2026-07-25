package workspacesnapshot_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/workspacecontent"
	"github.com/dvordrova/repomap/internal/workspacesearch"
	"github.com/dvordrova/repomap/internal/workspacesnapshot"
)

func TestSnapshotSupportsSearchAndExactContentWithoutReport(t *testing.T) {
	root := canonicalTempDir(t)
	content := []byte("package sample\n\nfunc Start() {}\n")
	if err := os.WriteFile(filepath.Join(root, "main.go"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	contentDigest := fmt.Sprintf("%x", sha256.Sum256(content))
	repository := freshness.RepositoryState{
		Version:  freshness.RepositoryStateVersion,
		Identity: root,
		Head:     strings.Repeat("a", 40),
		Dirty:    []freshness.DirtyFile{},
	}
	captured := freshness.CapturedInput{
		Version:       freshness.CapturedInputVersion,
		ID:            strings.Repeat("b", 64),
		Path:          "main.go",
		Kind:          freshness.FileRegular,
		Mode:          "100644",
		ContentSHA256: contentDigest,
		Stages:        []string{"report_evidence"},
	}
	snapshot, err := workspacesnapshot.New(workspacesnapshot.Input{
		AnalysisRoot:   root,
		Repository:     repository,
		CapturedInputs: []freshness.CapturedInput{captured},
		AllowedPaths:   []string{"main.go"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := snapshot.Catalog().Paths(); !reflect.DeepEqual(got, []string{"main.go"}) {
		t.Fatalf("catalog paths = %#v", got)
	}

	index, err := workspacesearch.New(workspacesearch.Input{Catalog: snapshot.Catalog()})
	if err != nil {
		t.Fatalf("workspacesearch.New: %v", err)
	}
	matches, err := index.Search(workspacesearch.Query{Text: "main.go"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) != 1 || matches[0].Entry.Path != "main.go" ||
		matches[0].Match != workspacesearch.MatchExactPath {
		t.Fatalf("search matches = %#v", matches)
	}

	contentService, err := workspacecontent.New(snapshot.Catalog())
	if err != nil {
		t.Fatalf("workspacecontent.New: %v", err)
	}
	defer contentService.Close()
	result, err := contentService.Read(context.Background(), workspacecontent.Request{
		Path: "main.go",
		Range: workspacecontent.Range{
			StartLine: 1,
			EndLine:   3,
			FocusLine: 3,
		},
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if result.Path != "main.go" || result.ContentSHA256 != contentDigest ||
		result.Text != strings.TrimSuffix(string(content), "\n") {
		t.Fatalf("content result = %#v", result)
	}

	unrelated := repository
	unrelated.Dirty = []freshness.DirtyFile{externalDirtyFile("notes.txt", strings.Repeat("d", 64))}
	if assessed := snapshot.Assess(unrelated); assessed.State != freshness.FreshnessUnrelatedChanges {
		t.Fatalf("unrelated Assess = %#v", assessed)
	}
	if err := snapshot.Verify(unrelated); err != nil {
		t.Fatalf("Verify unrelated: %v", err)
	}

	stale := repository
	stale.Dirty = []freshness.DirtyFile{externalDirtyFile("main.go", strings.Repeat("e", 64))}
	if assessed := snapshot.Assess(stale); assessed.State != freshness.FreshnessPartiallyStale {
		t.Fatalf("stale Assess = %#v", assessed)
	}
	if err := snapshot.Verify(stale); err == nil {
		t.Fatal("Verify stale unexpectedly succeeded")
	}
}

func canonicalTempDir(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(root)
}

func externalDirtyFile(path, digest string) freshness.DirtyFile {
	return freshness.DirtyFile{
		Status:        "modified",
		Path:          path,
		Kind:          freshness.FileRegular,
		Mode:          "100644",
		ContentSHA256: digest,
	}
}
