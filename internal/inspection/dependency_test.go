package inspection

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestProductionDependencyDirection(t *testing.T) {
	t.Parallel()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate inspection package")
	}
	allowedInternal := map[string]struct{}{
		"github.com/dvordrova/repomap/internal/analyzer":      {},
		"github.com/dvordrova/repomap/internal/evidence":      {},
		"github.com/dvordrova/repomap/internal/reporead":      {},
		"github.com/dvordrova/repomap/internal/sourcecard":    {},
		"github.com/dvordrova/repomap/internal/sourcecatalog": {},
		"github.com/dvordrova/repomap/internal/symbol":        {},
	}
	packages, err := parser.ParseDir(
		token.NewFileSet(),
		filepath.Dir(currentFile),
		func(info fs.FileInfo) bool { return !strings.HasSuffix(info.Name(), "_test.go") },
		parser.ImportsOnly,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, parsedPackage := range packages {
		for filename, file := range parsedPackage.Files {
			for _, imported := range file.Imports {
				importPath, err := strconv.Unquote(imported.Path.Value)
				if err != nil {
					t.Fatalf("%s: malformed import: %v", filename, err)
				}
				if !strings.HasPrefix(importPath, "github.com/dvordrova/repomap/internal/") {
					continue
				}
				if _, ok := allowedInternal[importPath]; !ok {
					t.Fatalf("%s imports disallowed internal dependency %q", filename, importPath)
				}
			}
		}
	}
}
