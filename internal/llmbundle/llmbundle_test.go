package llmbundle

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/snapshot"
)

func TestBuildCompactBundle(t *testing.T) {
	s := snapshot.Snapshot{
		RepoName:  "test-repo",
		Readme:    "# Hello World\n\nThis is a test repo\n",
		FileTree:  []string{"cmd/app/main.go", "go.mod", "README.md", "pkg/lib.go", "pkg/util.go"},
		TopLevelStats: map[string]int{"cmd": 1, "pkg": 2, ".": 2},
		LanguageHints: []snapshot.LanguageHint{
			{Language: "Go", Count: 3},
		},
		GoFacts: &gofacts.Facts{
			Modules: []gofacts.ModuleFact{
				{ModulePath: "example.com/test", ModuleDir: "."},
			},
			PackagesCount: 5,
			ModuleSummaries: []gofacts.ModuleSummary{
				{
					ModulePath:  "example.com/test",
					ModuleDir:   ".",
					RoleGuess:   "repository_root",
					PackagesCount: 5,
					EntrypointsCount: 1,
				},
			},
			EntrypointPackages: []gofacts.Entrypoint{
				{
					ModulePath: "example.com/test",
					ImportPath: "example.com/test/cmd/app",
					PackageDir: "cmd/app",
					Kind:       "unknown",
					GoFiles:    []string{"main.go"},
				},
			},
			OrientationCandidates: []gofacts.OrientationCandidate{
				{
					Name:              "example.com/test/cmd/app",
					Kind:              "unknown",
					EntrypointPackage: "example.com/test/cmd/app",
					OpenFiles:         []string{"cmd/app/main.go"},
					Why:               "entrypoint of unknown role",
					Priority:          0,
				},
			},
			InternalEdges: []gofacts.Edge{
				{From: "example.com/test/cmd/app", To: "example.com/test/pkg/lib"},
			},
		},
	}

	bundle := Build(s, s.FileTree, Options{})

	if bundle.RepoName != "test-repo" {
		t.Fatalf("repo_name = %q", bundle.RepoName)
	}
	if !strings.Contains(bundle.ReadmeExcerpt, "Hello") {
		t.Fatalf("readme_excerpt should contain Hello: %q", bundle.ReadmeExcerpt)
	}
	if bundle.Go.ModulesCount != 1 {
		t.Fatalf("modules_count = %d, want 1", bundle.Go.ModulesCount)
	}
	if len(bundle.Go.Entrypoints) != 1 {
		t.Fatalf("entrypoints = %d, want 1", len(bundle.Go.Entrypoints))
	}
	if bundle.Go.Entrypoints[0].OpenFiles[0] != "cmd/app/main.go" {
		t.Fatalf("open_files[0] = %q, want cmd/app/main.go", bundle.Go.Entrypoints[0].OpenFiles[0])
	}
	if len(bundle.Go.ImportantEdges) != 1 {
		t.Fatalf("important_edges = %d, want 1", len(bundle.Go.ImportantEdges))
	}
	if len(bundle.Go.OrientationCandidates) != 1 {
		t.Fatalf("orientation_candidates = %d, want 1", len(bundle.Go.OrientationCandidates))
	}

	jsonBytes, _ := json.Marshal(bundle)
	jsonStr := string(jsonBytes)

	if strings.Contains(jsonStr, `"file_tree"`) {
		t.Fatal("bundle must not contain full file_tree")
	}
	if !strings.Contains(jsonStr, `"repo_name"`) {
		t.Fatal("bundle must contain repo_name")
	}
	if !strings.Contains(jsonStr, `"orientation_candidates"`) {
		t.Fatal("bundle must contain orientation_candidates")
	}
}

func TestBundleRespectsMaxLimits(t *testing.T) {
	var entries []gofacts.Entrypoint
	for i := 0; i < 50; i++ {
		entries = append(entries, gofacts.Entrypoint{
			ModulePath: "m", ImportPath: "m/p", Kind: "unknown",
		})
	}
	var candidates []gofacts.OrientationCandidate
	for i := 0; i < 100; i++ {
		candidates = append(candidates, gofacts.OrientationCandidate{
			Name: "c", Kind: "unknown", EntrypointPackage: "c",
		})
	}
	var edges []gofacts.Edge
	for i := 0; i < 100; i++ {
		edges = append(edges, gofacts.Edge{})

	}

	s := snapshot.Snapshot{
		RepoName: "t",
		GoFacts: &gofacts.Facts{
			Modules:              []gofacts.ModuleFact{{ModulePath: "m", ModuleDir: "."}},
			EntrypointPackages:    entries,
			OrientationCandidates: candidates,
			InternalEdges:         edges,
			ModuleSummaries:       []gofacts.ModuleSummary{{ModulePath: "m", ModuleDir: "."}},
		},
	}

	bundle := Build(s, nil, Options{
		MaxEntrypoints: 10,
		MaxFiles:       15,
		MaxEdges:       20,
	})

	if len(bundle.Go.Entrypoints) != 10 {
		t.Fatalf("entrypoints = %d, want 10", len(bundle.Go.Entrypoints))
	}
	if len(bundle.Go.OrientationCandidates) != 15 {
		t.Fatalf("orientation_candidates = %d, want 15", len(bundle.Go.OrientationCandidates))
	}
	if len(bundle.Go.ImportantEdges) != 20 {
		t.Fatalf("important_edges = %d, want 20", len(bundle.Go.ImportantEdges))
	}
	if len(bundle.Warnings) < 3 {
		t.Fatalf("expected at least 3 truncation warnings, got: %v", bundle.Warnings)
	}
}

func TestFindKnownDocs(t *testing.T) {
	files := []string{
		"README.md",
		"docs/architecture.md",
		"Documentation/etcd-internals/README.md",
		"cmd/app/main.go",
		"go.mod",
	}

	docs := findKnownDocs(files)

	hasArch := false
	hasDocsReadme := false
	for _, d := range docs {
		if d == "Documentation/etcd-internals/README.md" {
			hasDocsReadme = true
		}
		if d == "docs/architecture.md" {
			hasArch = true
		}
	}
	if !hasArch {
		t.Fatalf("expected docs/architecture.md in known_docs, got: %v", docs)
	}
	if !hasDocsReadme {
		t.Fatalf("expected Documentation/etcd-internals/README.md in known_docs, got: %v", docs)
	}
}

func TestBuildEmptyGoFacts(t *testing.T) {
	s := snapshot.Snapshot{
		RepoName: "test",
		GoFacts:  nil,
	}
	bundle := Build(s, nil, Options{})
	if bundle.Go.ModulesCount != 0 {
		t.Fatalf("modules_count = %d, want 0", bundle.Go.ModulesCount)
	}
	if len(bundle.Go.Entrypoints) != 0 {
		t.Fatalf("entrypoints = %d, want 0", len(bundle.Go.Entrypoints))
	}
}

func TestFileIndexIncludesEntrypointOpenFiles(t *testing.T) {
	facts := &gofacts.Facts{
		EntrypointPackages: []gofacts.Entrypoint{
			{ModulePath: "m", ImportPath: "m/cmd/app", PackageDir: "cmd/app", GoFiles: []string{"main.go"}},
		},
		OrientationCandidates: []gofacts.OrientationCandidate{
			{OpenFiles: []string{"server/main.go", "etcdctl/main.go"}},
		},
	}
	fileList := []string{
		"server/main.go",
		"etcdctl/main.go",
		"server/etcdserver/etcdserver.go",
		"go.mod",
		"README.md",
	}
	files := buildFileIndex(fileList, facts, nil, nil)

	paths := make(map[string]bool)
	for _, e := range files {
		paths[e.Path] = true
	}

	if !paths["server/main.go"] {
		t.Fatal("missing entrypoint open_file server/main.go")
	}
	if !paths["etcdctl/main.go"] {
		t.Fatal("missing entrypoint open_file etcdctl/main.go")
	}
}

func TestFileIndexEntrypointsGetHighScore(t *testing.T) {
	facts := &gofacts.Facts{
		EntrypointPackages: []gofacts.Entrypoint{
			{ModulePath: "m", ImportPath: "m/server", PackageDir: "server", GoFiles: []string{"main.go"}},
		},
		OrientationCandidates: []gofacts.OrientationCandidate{
			{OpenFiles: []string{"server/main.go"}},
		},
	}
	fileList := []string{"server/main.go", "README.md"}

	files := buildFileIndex(fileList, facts, nil, nil)

	for _, e := range files {
		if e.Path == "server/main.go" {
			if e.Score < 100 {
				t.Fatalf("entrypoint score = %d, want >= 100", e.Score)
			}
			if e.Kind != "source" {
				t.Fatalf("kind = %q, want source", e.Kind)
			}
			return
		}
	}
	t.Fatal("server/main.go not found in file index")
}

func TestAllowedPathsContainsAllIndexPaths(t *testing.T) {
	facts := &gofacts.Facts{
		EntrypointPackages: []gofacts.Entrypoint{
			{ModulePath: "m", ImportPath: "m/server", PackageDir: "server", GoFiles: []string{"main.go"}},
		},
		OrientationCandidates: []gofacts.OrientationCandidate{
			{OpenFiles: []string{"server/main.go", "etcdctl/main.go"}},
		},
	}
	fileList := []string{
		"server/main.go",
		"etcdctl/main.go",
		"server/etcdserver/etcdserver.go",
		"docs/architecture.md",
	}

	s := snapshot.Snapshot{
		RepoName: "test",
		GoFacts:  facts,
	}
	bundle := Build(s, fileList, Options{})

	if len(bundle.AllowedPaths) == 0 {
		t.Fatal("allowed_paths is empty")
	}

	allowedSet := make(map[string]bool)
	for _, p := range bundle.AllowedPaths {
		allowedSet[p] = true
	}

	for _, e := range bundle.CandidateFileIndex {
		if !allowedSet[e.Path] {
			t.Fatalf("file index path %q not in allowed_paths", e.Path)
		}
	}
}

func TestFileIndexIncludesDocs(t *testing.T) {
	fileList := []string{"docs/architecture.md", "docs/workflow.md", "cmd/app/main.go", "README.md"}
	knownDocs := []string{"docs/architecture.md"}

	files := buildFileIndex(fileList, &gofacts.Facts{}, knownDocs, nil)

	found := false
	for _, e := range files {
		if e.Path == "docs/architecture.md" {
			found = true
			if e.Kind != "doc" {
				t.Fatalf("kind = %q, want doc", e.Kind)
			}
		}
	}
	if !found {
		t.Fatal("docs/architecture.md not found in file index")
	}
}

func TestDetectFileKind(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"server/main.go", "source"},
		{"server/etcdserver_impl_test.go", "test"},
		{"docs/design.md", "doc"},
		{"api/etcdserverpb/rpc.proto", "proto"},
		{"config/config.yaml", "config"},
		{"api/etcdserverpb/rpc.pb.go", "generated"},
		{"Makefile", "unknown"},
	}
	for _, tc := range cases {
		got := detectFileKind(tc.path)
		if got != tc.want {
			t.Errorf("detectFileKind(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestLLMBundleOnlyIncludesAllowedPaths(t *testing.T) {
	s := snapshot.Snapshot{
		RepoName: "test",
		GoFacts: &gofacts.Facts{
			EntrypointPackages: []gofacts.Entrypoint{
				{ModulePath: "m", ImportPath: "m/server", PackageDir: "server", GoFiles: []string{"main.go"}},
			},
			OrientationCandidates: []gofacts.OrientationCandidate{
				{OpenFiles: []string{"server/main.go"}},
			},
		},
	}
	fileList := []string{"server/main.go", "server/etcdserver/etcdserver.go", "go.mod"}

	bundle := Build(s, fileList, Options{})

	if len(bundle.AllowedPaths) == 0 {
		t.Fatal("--llm-bundle-only must include allowed_paths")
	}
	if len(bundle.CandidateFileIndex) == 0 {
		t.Fatal("--llm-bundle-only must include candidate_file_index")
	}

	foundEntrypoint := false
	for _, e := range bundle.CandidateFileIndex {
		if e.Path == "server/main.go" {
			foundEntrypoint = true
			break
		}
	}
	if !foundEntrypoint {
		t.Fatal("candidate_file_index must include entrypoint open_file")
	}
}
