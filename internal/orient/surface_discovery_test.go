package orient

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
	"github.com/dvordrova/repomap/internal/repositoryatlas/goadapter"
)

func TestRunPersistsOptInSurfaceArtifactsBesideReportRun(t *testing.T) {
	repository := t.TempDir()
	writeSurfaceTestFile(t, repository, "go.mod", "module example.com/orient-surface\n\ngo 1.24\n")
	writeSurfaceTestFile(t, repository, "main.go", `package main

import "net/http"

func health(http.ResponseWriter, *http.Request) {}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", health)
	_ = http.ListenAndServe(":8080", mux)
}
`)
	runOrientGit(t, repository, "init", "--quiet")
	runOrientGit(t, repository, "add", "--", "go.mod", "main.go")

	debugDirectory := t.TempDir()
	_, err := Run(context.Background(), Options{
		RepoPath: repository, Offline: true, OutputJSON: true,
		DebugDir: debugDirectory, RunID: "surface-run", RequireArtifacts: true,
		DiscoverSurfaces: true,
		MaxReadmeBytes:   1024, MaxReadmeLLMBytes: 512, MaxTreeLines: 50,
		MaxInterestingFiles: 50, MaxGoPkgs: 50, MaxGoEdges: 50,
		MaxLLMEntrypoints: 10, MaxLLMModules: 10, MaxLLMFiles: 20,
		MaxLLMEdges: 20, MaxLLMSignals: 10, MaxLLMSignalsPerFile: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	runDirectory := filepath.Join(debugDirectory, "surface-run")
	for _, name := range []string{
		"trigger_catalog.json", "surface_coverage.json",
		"semantic_summaries.json", "surface_summary.md",
		repositoryatlas.ArtifactFilename,
	} {
		data, err := os.ReadFile(filepath.Join(runDirectory, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !strings.Contains(string(data), "/health") && name == "trigger_catalog.json" {
			t.Fatalf("catalog does not contain route: %s", data)
		}
		if name == "trigger_catalog.json" && (!strings.Contains(string(data), `"kind": "process_entry"`) ||
			!strings.Contains(string(data), `"kind": "process_entry_declaration"`)) {
			t.Fatalf("catalog does not contain the gofacts process-entry surface: %s", data)
		}
		if name == repositoryatlas.ArtifactFilename {
			atlas, err := repositoryatlas.DecodeCanonicalJSON(data)
			if err != nil {
				t.Fatalf("decode repository Atlas: %v", err)
			}
			entityKinds := map[repositoryatlas.EntityKind]int{}
			for _, entity := range atlas.Entities {
				entityKinds[entity.Kind]++
			}
			processEvidence, packageEvidence := 0, 0
			for _, item := range atlas.Evidence {
				switch item.Provenance.Operation {
				case "build_selected_main_declaration":
					processEvidence++
				case goadapter.PackageDeclarationEvidenceOperation:
					packageEvidence++
				}
			}
			if len(atlas.Entities) != 2 || processEvidence != 1 || packageEvidence != 1 ||
				len(atlas.Relations) != 1 ||
				entityKinds[repositoryatlas.EntitySurface] != 1 ||
				entityKinds[repositoryatlas.EntityOperation] != 1 ||
				atlas.Relations[0].Authority != repositoryatlas.AuthorityResolved {
				t.Fatalf("repository Atlas process entry projection = %#v", atlas)
			}
		}
	}
}

func TestRunPersistsUnitTopologyWithoutSurfaceDiscovery(t *testing.T) {
	repository := t.TempDir()
	writeSurfaceTestFile(t, repository, "go.mod", "module example.com/orient-atlas\n\ngo 1.24\n")
	writeSurfaceTestFile(t, repository, "main.go", "package main\n\nfunc main() {}\n")
	runOrientGit(t, repository, "init", "--quiet")
	runOrientGit(t, repository, "add", "--", "go.mod", "main.go")

	debugDirectory := t.TempDir()
	_, err := Run(context.Background(), Options{
		RepoPath: repository, Offline: true, OutputJSON: true,
		DebugDir: debugDirectory, RunID: "atlas-units", RequireArtifacts: true,
		MaxReadmeBytes: 1024, MaxReadmeLLMBytes: 512, MaxTreeLines: 50,
		MaxInterestingFiles: 50, MaxGoPkgs: 50, MaxGoEdges: 50,
		MaxLLMEntrypoints: 10, MaxLLMModules: 10, MaxLLMFiles: 20,
		MaxLLMEdges: 20, MaxLLMSignals: 10, MaxLLMSignalsPerFile: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := os.ReadFile(filepath.Join(debugDirectory, "atlas-units", repositoryatlas.ArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	atlas, err := repositoryatlas.DecodeCanonicalJSON(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(atlas.Units) != 4 || len(atlas.Entities) != 0 || len(atlas.Relations) != 0 {
		t.Fatalf("unit-only repository Atlas = %#v", atlas)
	}
}

func TestSurfaceDiscoveryInputPreservesExactEntrypointAnchors(t *testing.T) {
	input := surfaceDiscoveryInput("fixture", &gofacts.Facts{
		Modules: []gofacts.ModuleFact{{ModuleDir: "service"}},
		EntrypointPackages: []gofacts.Entrypoint{{
			ImportPath: "example.com/fixture/cmd/fixture", PackageDir: "service/cmd/fixture",
			ModuleDir: "service", Kind: "unknown",
			Anchors: []gofacts.EntrypointAnchor{{
				Version: gofacts.EntrypointAnchorVersion, Kind: gofacts.EntrypointAnchorGoMain,
				Path: "service/cmd/fixture/main.go", Line: 23,
			}},
		}},
	}, nil)
	if input.RepositoryName != "fixture" || len(input.Entrypoints) != 1 ||
		!reflect.DeepEqual(input.ModuleDirs, []string{"service"}) ||
		len(input.Entrypoints[0].Anchors) != 1 ||
		input.Entrypoints[0].Anchors[0].Kind != "go_main_function" ||
		input.Entrypoints[0].ModuleDir != "service" ||
		input.Entrypoints[0].Anchors[0].Path != "service/cmd/fixture/main.go" ||
		input.Entrypoints[0].Anchors[0].Line != 23 {
		t.Fatalf("surface discovery input = %#v", input)
	}
}

func TestRunSurfaceDiscoveryWithoutArtifactDirectoryIsSkipped(t *testing.T) {
	repository := t.TempDir()
	writeSurfaceTestFile(t, repository, "README.md", "fixture\n")
	runOrientGit(t, repository, "init", "--quiet")
	runOrientGit(t, repository, "add", "--", "README.md")

	_, err := Run(context.Background(), Options{
		RepoPath: repository, SnapshotOnly: true, DiscoverSurfaces: true,
	})
	if err != nil {
		t.Fatalf("surface discovery without an artifact directory: %v", err)
	}
}

func TestRunSurfaceDiscoverySkipsNonGoRepository(t *testing.T) {
	repository := t.TempDir()
	writeSurfaceTestFile(t, repository, "README.md", "fixture\n")
	runOrientGit(t, repository, "init", "--quiet")
	runOrientGit(t, repository, "add", "--", "README.md")

	debugDirectory := t.TempDir()
	_, err := Run(context.Background(), Options{
		RepoPath: repository, Offline: true, OutputJSON: true,
		DebugDir: debugDirectory, RunID: "non-go", RequireArtifacts: true,
		DiscoverSurfaces: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(debugDirectory, "non-go", "trigger_catalog.json")); !os.IsNotExist(err) {
		t.Fatalf("non-Go run created a surface catalog: %v", err)
	}
}

func writeSurfaceTestFile(t *testing.T, root, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
