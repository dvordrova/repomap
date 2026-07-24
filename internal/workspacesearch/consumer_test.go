package workspacesearch_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/inspection"
	"github.com/dvordrova/repomap/internal/sourcecatalog"
	"github.com/dvordrova/repomap/internal/workspacesearch"
)

func TestConsumerSelectsExactFactsForInspection(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	readmeID := sha256.Sum256([]byte("workspacesearch-consumer\x00README.md"))
	serviceID := sha256.Sum256([]byte("workspacesearch-consumer\x00service.go"))
	catalog, err := sourcecatalog.New(sourcecatalog.Input{
		RepositoryRoot: root,
		AnalysisRoot:   root,
		AllowedPaths:   []string{"README.md", "service.go"},
		CapturedInputs: []freshness.CapturedInput{
			{
				Version: freshness.CapturedInputVersion, ID: fmt.Sprintf("%x", readmeID[:]),
				Path: "README.md", Kind: freshness.FileRegular, Mode: "100644",
				ContentSHA256: strings.Repeat("a", 64), Stages: []string{"report_evidence"},
			},
			{
				Version: freshness.CapturedInputVersion, ID: fmt.Sprintf("%x", serviceID[:]),
				Path: "service.go", Kind: freshness.FileRegular, Mode: "100644",
				ContentSHA256: strings.Repeat("b", 64), Stages: []string{"report_evidence"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	symbol := evidence.Entity{
		ID:       "service.Run",
		Kind:     evidence.EntityFunction,
		Name:     "Run",
		Language: "go",
		Scope:    evidence.SourceScopeRepository,
		Location: &evidence.Location{Path: "service.go", Line: 3, Column: 1},
	}
	index, err := workspacesearch.New(workspacesearch.Input{
		Catalog: catalog,
		Symbols: []evidence.Entity{symbol},
	})
	if err != nil {
		t.Fatal(err)
	}

	assertMatch := func(query string, kind workspacesearch.Kind) workspacesearch.Match {
		t.Helper()
		result, searchErr := index.Search(workspacesearch.Query{Text: query})
		if searchErr != nil {
			t.Fatal(searchErr)
		}
		if len(result) == 0 || result[0].Entry.Kind != kind {
			t.Fatalf("Search(%q) = %#v", query, result)
		}
		return result[0]
	}
	assertMatch("service.go", workspacesearch.KindFile)
	assertMatch("README.md", workspacesearch.KindDocument)
	selected := assertMatch("Run", workspacesearch.KindSymbol)

	resolveRequest := inspection.ResolveRequest{
		Location:  *selected.Entry.Location,
		RankTerms: []string{selected.Entry.Name},
	}
	inspectRequest := inspection.InspectRequest{Target: *selected.Entry.Entity}
	service, err := inspection.New(catalog, inspection.Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Resolve(context.Background(), resolveRequest); inspection.ErrorKindOf(err) != inspection.ErrorAnalyzerUnavailable {
		t.Fatalf("selected location did not enter Resolve authority: %v", err)
	}
	if _, err := service.Inspect(context.Background(), inspectRequest); inspection.ErrorKindOf(err) != inspection.ErrorAnalyzerUnavailable {
		t.Fatalf("selected entity did not enter Inspect authority: %v", err)
	}
	if filepath.IsAbs(selected.Entry.Path) || filepath.IsAbs(selected.Entry.Location.Path) {
		t.Fatalf("selected result leaked an absolute path: %#v", selected)
	}
}
