package report

import "testing"

func TestSemanticSearchIndexesPavedPathsWithDirectRoutes(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		RepoName: "fixture",
		Operations: &RepositoryOperations{Paths: []RepositoryPavedPath{{
			ID:    "operate-serve",
			Title: "Run the local server",
			Goal:  "Start the repository-owned development server and verify its endpoint.",
			Actions: []OperationalAction{{
				Instruction: "Start the development server.",
				Command:     "go run ./cmd/server",
				Endpoint:    "http://127.0.0.1:8080/health",
			}},
		}}},
	}

	index, err := BuildSemanticSearchIndex(data)
	if err != nil {
		t.Fatal(err)
	}
	item := findSemanticSearchItem(t, index.Items, "Run the local server")
	if item.Kind != SemanticSearchKindPavedPath || item.Target.Kind != SemanticSearchTargetPavedPath ||
		item.Target.PavedPathID != "operate-serve" {
		t.Fatalf("paved path search item = %#v", item)
	}
	if item.Summary != "Start the repository-owned development server and verify its endpoint." {
		t.Fatalf("paved path summary = %q", item.Summary)
	}
	for _, alias := range []string{
		"go run ./cmd/server",
		"http://127.0.0.1:8080/health",
	} {
		if !stringSliceContains(item.Aliases, alias) {
			t.Fatalf("paved path aliases = %#v, want %q", item.Aliases, alias)
		}
	}
}

func TestSemanticSearchRejectsDanglingPavedPathTarget(t *testing.T) {
	t.Parallel()

	data := &ReportData{RepoName: "fixture"}
	index := SemanticSearchIndex{
		Version: SemanticSearchIndexVersion,
		Items: []SemanticSearchItem{{
			ID:        "paved-path-missing",
			Kind:      SemanticSearchKindPavedPath,
			Title:     "Missing operation",
			Stability: SemanticSearchStabilityRunStable,
			Target: SemanticSearchTarget{
				Kind: SemanticSearchTargetPavedPath, PavedPathID: "missing",
			},
		}},
	}
	if err := index.Validate(data); err == nil {
		t.Fatal("dangling paved path target was accepted")
	}
}
