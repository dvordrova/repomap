package gofacts

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/reporead"
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

func TestResolveMainEntrypointsUsesDeclarationsInBuildSelectedFiles(t *testing.T) {
	tests := []struct {
		name      string
		files     map[string]string
		goFiles   []string
		wantPath  string
		wantLine  int
		wantCount int
	}{
		{
			name: "func main in a non-main filename",
			files: map[string]string{
				"command.go": "package main\n\nfunc main() {}\n",
			},
			goFiles:   []string{"command.go"},
			wantPath:  "cmd/app/command.go",
			wantLine:  3,
			wantCount: 1,
		},
		{
			name: "main filename without func main",
			files: map[string]string{
				"main.go": "package main\n\nfunc run() {}\n",
			},
			goFiles:   []string{"main.go"},
			wantCount: 0,
		},
		{
			name: "method named main is not a process entrypoint",
			files: map[string]string{
				"runner.go": "package main\n\ntype runner struct{}\n\nfunc (runner) main() {}\n",
			},
			goFiles:   []string{"runner.go"},
			wantCount: 0,
		},
		{
			name: "only go-list-selected files are inspected",
			files: map[string]string{
				"selected.go":     "package main\n\nfunc main() {}\n",
				"not_selected.go": "package main\n\n\n\nfunc main() {}\n",
			},
			goFiles:   []string{"selected.go"},
			wantPath:  "cmd/app/selected.go",
			wantLine:  3,
			wantCount: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			repo := t.TempDir()
			packageDir := filepath.Join(repo, "cmd", "app")
			if err := os.MkdirAll(packageDir, 0o700); err != nil {
				t.Fatal(err)
			}
			for name, source := range test.files {
				if err := os.WriteFile(filepath.Join(packageDir, name), []byte(source), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			reader, err := reporead.New(repo)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()

			entrypoints, warnings, scaleWarnings := resolveMainEntrypoints(reader, []Entrypoint{{
				ImportPath: "example.com/project/cmd/app",
				PackageDir: "cmd/app",
				GoFiles:    test.goFiles,
			}})
			if len(warnings) != 0 {
				t.Fatalf("warnings = %v, want none", warnings)
			}
			if len(scaleWarnings) != 0 {
				t.Fatalf("scale warnings = %v, want none", scaleWarnings)
			}
			if len(entrypoints) != test.wantCount {
				t.Fatalf("entrypoints = %#v, want %d", entrypoints, test.wantCount)
			}
			if test.wantCount == 0 {
				return
			}

			anchors := entrypoints[0].Anchors
			if len(anchors) != 1 {
				t.Fatalf("anchors = %#v, want one", anchors)
			}
			anchor := anchors[0]
			if anchor.Version != EntrypointAnchorVersion || anchor.Kind != EntrypointAnchorGoMain ||
				anchor.Path != test.wantPath || anchor.Line != test.wantLine {
				t.Fatalf("anchor = %#v, want %s:%d go main v%d", anchor, test.wantPath, test.wantLine, EntrypointAnchorVersion)
			}
		})
	}
}

func TestResolveMainEntrypointsRetainsFilesPastUsualSize(t *testing.T) {
	repo := t.TempDir()
	packageDir := filepath.Join(repo, "cmd", "app")
	if err := os.MkdirAll(packageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	goFiles := make([]string, 0, advisoryEntrypointFilesPerPackage+1)
	for index := 0; index < advisoryEntrypointFilesPerPackage; index++ {
		name := fmt.Sprintf("source_%03d.go", index)
		if err := os.WriteFile(filepath.Join(packageDir, name), []byte("package main\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		goFiles = append(goFiles, name)
	}
	const mainFile = "zz_main.go"
	if err := os.WriteFile(filepath.Join(packageDir, mainFile), []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	goFiles = append(goFiles, mainFile)

	reader, err := reporead.New(repo)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	entrypoints, failures, scaleWarnings := resolveMainEntrypoints(reader, []Entrypoint{{
		ImportPath: "example.com/project/cmd/app", PackageDir: "cmd/app", GoFiles: goFiles,
	}})
	if len(failures) != 0 {
		t.Fatalf("failures = %v", failures)
	}
	if len(entrypoints) != 1 || len(entrypoints[0].Anchors) != 1 || entrypoints[0].Anchors[0].Path != "cmd/app/"+mainFile {
		t.Fatalf("entrypoint after usual file count = %#v", entrypoints)
	}
	if len(scaleWarnings) != 1 || !strings.Contains(scaleWarnings[0], "retained all 257") {
		t.Fatalf("scale warnings = %v", scaleWarnings)
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
