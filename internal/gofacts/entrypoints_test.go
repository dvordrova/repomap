package gofacts

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRecordsOnlyBuildSelectedMainFunctionAnchors(t *testing.T) {
	repo := t.TempDir()
	files := map[string]string{
		"go.mod":                  "module example.com/project\n\ngo 1.24\n",
		"cmd/app/start.go":        "package main\n\nfunc main() {}\n",
		"cmd/app/not_selected.go": "//go:build repomap_never\n\npackage main\n\nfunc main() {}\n",
		"cmd/decoy/main.go":       "package main\n\nfunc helper() {}\n",
	}
	fileList := make([]string, 0, len(files))
	for name, source := range files {
		absolute := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
		fileList = append(fileList, name)
	}

	facts, err := loadForHost(context.Background(), repo, fileList)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.EntrypointPackages) != 1 {
		t.Fatalf("entrypoints = %#v, want only cmd/app", facts.EntrypointPackages)
	}
	entrypoint := facts.EntrypointPackages[0]
	if entrypoint.ImportPath != "example.com/project/cmd/app" {
		t.Fatalf("entrypoint import path = %q", entrypoint.ImportPath)
	}
	if len(entrypoint.GoFiles) != 1 || entrypoint.GoFiles[0] != "start.go" {
		t.Fatalf("build-selected GoFiles = %v, want [start.go]", entrypoint.GoFiles)
	}
	if len(entrypoint.Anchors) != 1 || entrypoint.Anchors[0].Path != "cmd/app/start.go" || entrypoint.Anchors[0].Line != 3 {
		t.Fatalf("entrypoint anchors = %#v, want cmd/app/start.go:3", entrypoint.Anchors)
	}
}

func TestIsMainFunctionRequiresProcessSignature(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   bool
	}{
		{name: "no parameters or results", source: "package main\nfunc main() {}\n", want: true},
		{name: "parameter", source: "package main\nfunc main(value int) {}\n", want: false},
		{name: "result", source: "package main\nfunc main() int { return 0 }\n", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), "entry.go", test.source, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			function, ok := file.Decls[0].(*ast.FuncDecl)
			if !ok {
				t.Fatalf("declaration = %T, want *ast.FuncDecl", file.Decls[0])
			}
			if got := isMainFunction(function); got != test.want {
				t.Fatalf("isMainFunction() = %t, want %t", got, test.want)
			}
		})
	}
}
