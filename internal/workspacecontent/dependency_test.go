package workspacecontent

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

func TestProductionImportsStayPresentationNeutral(t *testing.T) {
	t.Parallel()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate workspacecontent package")
	}
	directory := filepath.Dir(currentFile)
	packages, err := parser.ParseDir(
		token.NewFileSet(),
		directory,
		func(info fs.FileInfo) bool {
			return !strings.HasSuffix(info.Name(), "_test.go")
		},
		parser.ImportsOnly,
	)
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{
		"github.com/dvordrova/repomap/internal/reporead":      true,
		"github.com/dvordrova/repomap/internal/sourcecatalog": true,
	}
	for _, parsedPackage := range packages {
		for filename, file := range parsedPackage.Files {
			for _, imported := range file.Imports {
				importPath, err := strconv.Unquote(imported.Path.Value)
				if err != nil {
					t.Fatalf("%s: malformed import: %v", filename, err)
				}
				if strings.HasPrefix(importPath, "github.com/dvordrova/repomap/internal/") &&
					!allowed[importPath] {
					t.Fatalf("%s imports disallowed internal dependency %q", filename, importPath)
				}
			}
		}
	}
}
