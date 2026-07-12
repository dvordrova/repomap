package surfacediscovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteArtifacts(t *testing.T) {
	result := analyzeFixture(t, "direct")
	directory := t.TempDir()
	if err := WriteArtifacts(directory, result); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		TriggerCatalogFilename,
		SurfaceCoverageFilename,
		SemanticSummaryFilename,
		ArchitectureGroundingFilename,
		SurfaceSummaryFilename,
	} {
		data, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if len(data) == 0 {
			t.Fatalf("%s is empty", name)
		}
	}
	markdown, err := os.ReadFile(filepath.Join(directory, SurfaceSummaryFilename))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(markdown), "/health") || !strings.Contains(string(markdown), "not a runtime trace") {
		t.Fatalf("markdown = %s", markdown)
	}
}
