package run

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/terminology"
)

// The glossary is optional and the response cache is not a run artifact: a
// cache that cannot be written is a notice, and the accepted analysis is
// published with its glossary (review A7: such a recovered cache issue once
// returned from the glossary before translation and HTML). A glossary
// artifact the run cannot write is still an explicit error, and nothing is
// published.
func TestAGlossaryCacheIssuePublishesTheReportAndAnUnwrittenArtifactStopsIt(t *testing.T) {
	publish := func(t *testing.T, blockArtifact bool) (string, string, error) {
		t.Helper()
		ctx := context.Background()
		_, runs, inventory := repositoryReportRunsFixture(t, 1)
		assembled, err := assembleRepositoryReport(ctx, inventory, runs, atlasOutcome{})
		if err != nil {
			t.Fatal(err)
		}
		provider := &terminologyRuntimeProvider{}
		collector := terminology.NewCollector([]string{"part-00/main.go"})
		// One accepted analysis answer writes the prose the glossary explains.
		if _, err := llm.ExecuteJSON(ctx, llm.Executor{}, collector.Wrap(provider), llm.Call[map[string]any]{
			Prompt: llm.Prompt{System: "Describe each row.", ResponseFormatJSON: true,
				User: `{"table":"captions","fill":[{"name":"caption","kind":"text"}],"rows":[{"key":"r1","path":"part-00/main.go","line":1}]}`},
			Limits: llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: 1024},
		}); err != nil {
			t.Fatal(err)
		}
		// Every accepted answer's payloads, and so its cache record, fail to
		// save: a regular file stands where the payload directory goes.
		debugDir := t.TempDir()
		if err := os.Mkdir(filepath.Join(debugDir, llm.CacheDirectoryName), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(debugDir, llm.CacheDirectoryName, "payloads"), []byte("not a directory"), 0o600); err != nil {
			t.Fatal(err)
		}
		runDir := runs[0].RunDir
		if blockArtifact {
			if err := os.Mkdir(filepath.Join(runDir, "terminology.json"), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		categorizer, err := glossaryConcepts()
		if err != nil {
			t.Fatal(err)
		}
		var console bytes.Buffer
		_, err = publishRepositoryReport(ctx, assembled, nil, repositoryTargetDispatchOptions{
			Output: newRunOutput(&console), DebugDir: debugDir, Categorizer: categorizer, NoServe: true,
			Deps: defaultRunDeps{terminology: collector, newDisplayProvider: func() (llm.Provider, error) { return provider, nil }},
		})
		return runDir, console.String(), err
	}

	runDir, console, err := publish(t, false)
	if err != nil {
		t.Fatalf("a recoverable cache issue stopped publication: %v\n%s", err, console)
	}
	if !strings.Contains(console, "cache write failed") {
		t.Fatalf("the run output did not say the cache was not written:\n%s", console)
	}
	if _, err := os.Stat(filepath.Join(runDir, "report.html")); err != nil {
		t.Fatalf("the report was not published: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(runDir, terminology.CatalogFilename))
	if err != nil {
		t.Fatal(err)
	}
	var catalog terminology.Catalog
	if err := json.Unmarshal(raw, &catalog); err != nil || len(catalog.Entries) != 1 || catalog.Entries[0].Names[0] != "OHLCV" {
		t.Fatalf("the published glossary lost its definition: %s / %v", raw, err)
	}

	runDir, console, err = publish(t, true)
	if err == nil || !strings.Contains(err.Error(), "terminology.json") {
		t.Fatalf("an unwritten glossary artifact was not an error: %v\n%s", err, console)
	}
	if _, err := os.Stat(filepath.Join(runDir, "report.html")); !os.IsNotExist(err) {
		t.Fatalf("a report was published without its glossary artifact: %v", err)
	}
}
