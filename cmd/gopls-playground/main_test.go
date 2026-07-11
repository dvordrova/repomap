package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
)

func TestWriteMarkdownSummary(t *testing.T) {
	t.Parallel()

	graph := evidence.NewGraph("/repo", "Run")
	graph.Build = evidence.BuildContext{GOOS: "linux", GOARCH: "amd64"}
	graph.AddEntity(evidence.Entity{ID: "query", Kind: evidence.EntityQuery, Name: "Run"})
	graph.AddEntity(evidence.Entity{
		ID:       "run",
		Kind:     evidence.EntityFunction,
		Name:     "Run",
		Location: &evidence.Location{Path: "run.go", Line: 10, Column: 6},
	})
	graph.AddRelation(evidence.Relation{
		From:       "query",
		To:         "run",
		Kind:       evidence.RelationMatchesQuery,
		Certainty:  evidence.CertaintyPossible,
		Provenance: []evidence.Provenance{{Provider: "gopls", Operation: "workspace_symbol"}},
	})

	path := filepath.Join(t.TempDir(), "summary.md")
	if err := writeMarkdownSummary(path, graph); err != nil {
		t.Fatalf("writeMarkdownSummary() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(data)
	for _, want := range []string{"# gopls evidence: Run", "possible`: 1", "`Run`", "`run.go:10:6`"} {
		if !strings.Contains(text, want) {
			t.Errorf("summary missing %q:\n%s", want, text)
		}
	}
}
