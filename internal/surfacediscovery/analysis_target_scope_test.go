package surfacediscovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalysisTargetPackageAllowlistExcludesOtherCommandsAndFixtures(t *testing.T) {
	repository := t.TempDir()
	writeTargetScopeFile(t, repository, "go.mod", "module example.com/repomap\n\ngo 1.24\n")
	writeTargetScopeFile(t, repository, "cmd/repomap/main.go", `package main
import "net/http"
func main() { http.HandleFunc("/product", func(http.ResponseWriter, *http.Request) {}) }
`)
	writeTargetScopeFile(t, repository, "cmd/symbol-playground/main.go", `package main
import "net/http"
func main() { http.HandleFunc("/noise", func(http.ResponseWriter, *http.Request) {}) }
`)
	writeTargetScopeFile(t, repository, "testdata/fixture/main.go", `package main
import "net/http"
func main() { http.HandleFunc("/fixture", func(http.ResponseWriter, *http.Request) {}) }
`)

	result, err := AnalyzeWithInput(DefaultOptions(repository), Input{
		RepositoryName: "repomap",
		ModuleDirs:     []string{"."},
		Packages:       []PackageInput{{Path: "example.com/repomap/cmd/repomap", ModuleDir: "."}},
		Entrypoints: []EntrypointInput{{
			Package: "example.com/repomap/cmd/repomap", PackageDir: "cmd/repomap", ModuleDir: ".",
			Anchors: []EntrypointAnchorInput{{Kind: ProcessEntryAnchorGoMain, Path: "cmd/repomap/main.go", Line: 3}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(resultJSON)
	if !strings.Contains(encoded, "/product") {
		t.Fatalf("selected product surface unavailable: %s", encoded)
	}
	for _, forbidden := range []string{"/noise", "/fixture", "symbol-playground", "testdata/fixture"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("off-target package leaked through surface discovery (%q): %s", forbidden, encoded)
		}
	}
}

func writeTargetScopeFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
