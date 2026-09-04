package groupmatching

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/groupindex"
)

// TestCompileAtRepositoryScale compiles the group graphs of a real run when
// REPOMAP_GROUPS_INDEX_DIR names a directory of run directories, and reports
// how long it took. It exists because repomap on itself — nineteen targets,
// eleven hundred groups, forty-eight thousand edges — spent seventy minutes
// of CPU in compilePair before the incidence indexes and had not finished.
func TestCompileAtRepositoryScale(t *testing.T) {
	root := os.Getenv("REPOMAP_GROUPS_INDEX_DIR")
	if root == "" {
		t.Skip("set REPOMAP_GROUPS_INDEX_DIR to a runs directory to measure")
	}
	matches, err := filepath.Glob(filepath.Join(root, "*", groupindex.ArtifactFilename))
	if err != nil || len(matches) == 0 {
		t.Fatalf("no %s under %s", groupindex.ArtifactFilename, root)
	}
	var indexes []groupindex.Index
	for _, match := range matches {
		raw, err := os.ReadFile(match)
		if err != nil {
			t.Fatal(err)
		}
		index, err := groupindex.Decode(raw)
		if err != nil {
			t.Fatalf("%s: %v", match, err)
		}
		indexes = append(indexes, index)
	}
	groups, edges := 0, 0
	for _, index := range indexes {
		groups += len(index.Groups)
		edges += len(index.StructuralEdges)
	}
	started := time.Now()
	compilation, err := Compile(indexes)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("targets %d, groups %d, structural edges %d, pairs %d: compiled in %s",
		len(indexes), groups, edges, len(compilation.pairs), time.Since(started).Round(time.Millisecond))
}
