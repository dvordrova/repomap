package jstsproject

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
)

func TestCumulativeVitestFilesRequireAuthoredLiteralConfig(t *testing.T) {
	root := preparedCompilerProject(t)
	names := []string{"package.json", "tsconfig.json", "vitest.config.ts", "src/market.test.ts", "src/test-setup.ts", "src/testing/real-service.ts", "src/excluded/retained.test.ts"}
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "repositories", "jsts", filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, root, name, string(raw))
	}
	repository, err := corpus.New(t.Context(), root, gitfiles.Listing{Paths: names, RegularPaths: names})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	read := func() map[string]bool {
		t.Helper()
		_, index, _, err := Build(t.Context(), repository, root)
		if err != nil {
			t.Fatal(err)
		}
		files := make(map[string]bool)
		for _, source := range index.Target.Sources {
			if filepath.Ext(source.Path) == ".ts" && source.Path != "vitest.config.ts" {
				files[source.Path] = slices.Contains(index.Target.TestSources, source.Path)
			}
		}
		return files
	}
	want := map[string]bool{"src/market.test.ts": true, "src/test-setup.ts": true, "src/testing/real-service.ts": false, "src/excluded/retained.test.ts": false}
	if got := read(); !reflect.DeepEqual(got, want) {
		t.Fatalf("configured test discovery lost source or invented a filename role: %+v", got)
	}
	// An arbitrary include variable is not an observed literal list. Keep all
	// declarations, and do not guess the previously configured test partition.
	writeTestFile(t, root, "vitest.config.ts", `import {defineConfig} from "vitest/config"; const include = ["src/**/*.test.ts"]; export default defineConfig({test:{include}});`)
	for file := range want {
		want[file] = false
	}
	if got := read(); !reflect.DeepEqual(got, want) {
		t.Fatalf("unsupported dynamic config hid source: %+v", got)
	}
}
