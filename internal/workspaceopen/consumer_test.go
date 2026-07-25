package workspaceopen_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/workspaceopen"
	"github.com/dvordrova/repomap/internal/workspacesearch"
	"github.com/dvordrova/repomap/internal/workspacesnapshot"
)

func TestSearchSelectionResolvesWithoutReportOrEditor(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root = filepath.Clean(root)
	content := []byte("package sample\n\nfunc Start() {}\n")
	filePath := filepath.Join(root, "main.go")
	if err := os.WriteFile(filePath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	contentDigest := fmt.Sprintf("%x", sha256.Sum256(content))
	snapshot, err := workspacesnapshot.New(workspacesnapshot.Input{
		AnalysisRoot: root,
		Repository: freshness.RepositoryState{
			Version:  freshness.RepositoryStateVersion,
			Identity: root,
			Head:     strings.Repeat("a", 40),
		},
		CapturedInputs: []freshness.CapturedInput{{
			Version:       freshness.CapturedInputVersion,
			ID:            strings.Repeat("b", 64),
			Path:          "main.go",
			Kind:          freshness.FileRegular,
			Mode:          "100644",
			ContentSHA256: contentDigest,
			Stages:        []string{"report_evidence"},
		}},
		AllowedPaths: []string{"main.go"},
	})
	if err != nil {
		t.Fatalf("workspacesnapshot.New: %v", err)
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
		t.Fatalf("matches = %#v", matches)
	}

	service, err := workspaceopen.New(snapshot)
	if err != nil {
		t.Fatalf("workspaceopen.New: %v", err)
	}
	target, err := service.Resolve(context.Background(), workspaceopen.Request{
		Path: matches[0].Entry.Path,
	})
	if err != nil {
		t.Fatalf("Resolve unchanged: %v", err)
	}
	info, statErr := os.Stat(target.AbsolutePath)
	if statErr != nil || !info.Mode().IsRegular() || !filepath.IsAbs(target.AbsolutePath) ||
		target.Path != "main.go" || target.SourceChanged {
		t.Fatalf("target = %#v stat=%v", target, statErr)
	}

	if err := os.WriteFile(filePath, []byte("package changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	target, err = service.Resolve(context.Background(), workspaceopen.Request{
		Path: matches[0].Entry.Path,
	})
	if err != nil || !target.SourceChanged {
		t.Fatalf("Resolve changed = %#v, %v", target, err)
	}

	_, err = service.Resolve(context.Background(), workspaceopen.Request{Path: "private.go"})
	if workspaceopen.ErrorKindOf(err) != workspaceopen.ErrorUnauthorized ||
		strings.Contains(err.Error(), root) {
		t.Fatalf("private error = %q kind=%q", err, workspaceopen.ErrorKindOf(err))
	}
}
