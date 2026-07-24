package workspacecontent_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/sourcecatalog"
	"github.com/dvordrova/repomap/internal/workspacecontent"
	"github.com/dvordrova/repomap/internal/workspacesearch"
)

func TestExactSearchSelectionFeedsAuthorizedContentWithoutPresentationTypes(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	analysisRoot := filepath.Join(repository, "service")
	if err := os.Mkdir(analysisRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"README.md": []byte("# Service\r\n\r\nExact local guide.\r\n"),
		"service.go": []byte(
			"package service\n\nfunc Run() {\n\tprintln(\"run\")\n}\n",
		),
	}
	allowed := make([]string, 0, len(files))
	captured := make([]freshness.CapturedInput, 0, len(files))
	for sourcePath, content := range files {
		if err := os.WriteFile(
			filepath.Join(analysisRoot, filepath.FromSlash(sourcePath)),
			content,
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		allowed = append(allowed, sourcePath)
		repositoryPath := filepath.ToSlash(filepath.Join("service", filepath.FromSlash(sourcePath)))
		id := sha256.Sum256([]byte("workspacecontent-consumer\x00" + repositoryPath))
		captured = append(captured, freshness.CapturedInput{
			Version:       freshness.CapturedInputVersion,
			ID:            fmt.Sprintf("%x", id[:]),
			Path:          repositoryPath,
			Kind:          freshness.FileRegular,
			Mode:          "100644",
			ContentSHA256: fmt.Sprintf("%x", sha256.Sum256(content)),
			Stages:        []string{"report_evidence"},
		})
	}
	catalog, err := sourcecatalog.New(sourcecatalog.Input{
		RepositoryRoot: repository,
		AnalysisRoot:   analysisRoot,
		AllowedPaths:   allowed,
		CapturedInputs: captured,
	})
	if err != nil {
		t.Fatal(err)
	}
	index, err := workspacesearch.New(workspacesearch.Input{Catalog: catalog})
	if err != nil {
		t.Fatal(err)
	}
	service, err := workspacecontent.New(catalog)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	selectPath := func(query string, kind workspacesearch.Kind) string {
		t.Helper()
		matches, searchErr := index.Search(workspacesearch.Query{Text: query})
		if searchErr != nil {
			t.Fatal(searchErr)
		}
		if len(matches) == 0 || matches[0].Entry.Kind != kind {
			t.Fatalf("Search(%q) = %#v", query, matches)
		}
		return matches[0].Entry.Path
	}

	readmePath := selectPath("README.md", workspacesearch.KindDocument)
	readme, err := service.Read(context.Background(), workspacecontent.Request{
		Path: readmePath, Range: workspacecontent.Range{StartLine: 1, EndLine: 3, FocusLine: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if readme.Path != "README.md" || filepath.IsAbs(readme.Path) ||
		readme.Lines[0].Text != "# Service" {
		t.Fatalf("README result = %#v", readme)
	}

	sourcePath := selectPath("service.go", workspacesearch.KindFile)
	source, err := service.Read(context.Background(), workspacecontent.Request{
		Path: sourcePath, Range: workspacecontent.Range{StartLine: 3, EndLine: 5, FocusLine: 3},
		Limits: workspacecontent.Limits{MaxLines: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	if source.Path != "service.go" || filepath.IsAbs(source.Path) ||
		len(source.Lines) > 4 || !strings.Contains(source.Text, "func Run") {
		t.Fatalf("source result = %#v", source)
	}

	if err := os.WriteFile(
		filepath.Join(analysisRoot, filepath.FromSlash(sourcePath)),
		[]byte("package changed\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	changed, err := service.Read(context.Background(), workspacecontent.Request{
		Path: sourcePath, Range: workspacecontent.Range{StartLine: 1, EndLine: 1, FocusLine: 1},
	})
	if workspacecontent.ErrorKindOf(err) != workspacecontent.ErrorSourceChanged ||
		changed.Path != "" || strings.Contains(err.Error(), analysisRoot) {
		t.Fatalf("changed result=%#v err=%v", changed, err)
	}
}
