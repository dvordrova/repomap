package jstsproject

import (
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/programindex/adaptertest"
	"os"
	"path/filepath"
	"testing"
)

func TestCumulativeJSTSQueryOccurrencesRetainNativeOwners(t *testing.T) {
	root := preparedCompilerProject(t)
	tracked := []string{"package.json", "tsconfig.json", "src/data-sources.ts"}
	for _, path := range tracked {
		contents, err := os.ReadFile(filepath.Join("..", "..", "testdata", "repositories", "jsts", filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, root, path, string(contents))
	}
	repository, err := corpus.New(t.Context(), root, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	_, index, _, err := Build(t.Context(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.AssertQueryOccurrenceOwners(t, root, repository, index, "src/data-sources.ts")
}
