package llmbundle

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/sourcesignals"
)

func TestBuildCompactBundle(t *testing.T) {
	s := snapshot.Snapshot{
		RepoName:      "test-repo",
		Readme:        "# Hello World\n\nThis is a test repo\n",
		FileTree:      []string{"cmd/app/main.go", "go.mod", "README.md", "pkg/lib.go", "pkg/util.go"},
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
					ModulePath:       "example.com/test",
					ModuleDir:        ".",
					RoleGuess:        "repository_root",
					PackagesCount:    5,
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
					Anchors: []gofacts.EntrypointAnchor{{
						Version: gofacts.EntrypointAnchorVersion,
						Kind:    gofacts.EntrypointAnchorGoMain,
						Path:    "cmd/app/main.go",
						Line:    7,
					}},
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
	if anchors := bundle.Go.Entrypoints[0].Anchors; len(anchors) != 1 ||
		anchors[0].Path != "cmd/app/main.go" || anchors[0].Line != 7 ||
		anchors[0].Kind != gofacts.EntrypointAnchorGoMain ||
		anchors[0].Version != gofacts.EntrypointAnchorVersion {
		t.Fatalf("entrypoint anchors = %#v, want compact func main location", anchors)
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

func TestBuildTracePreservesBelowCapEdgeBundleJSONAndWarnings(t *testing.T) {
	edges := []gofacts.Edge{
		{From: "example.com/fixture/z", To: "example.com/fixture/b"},
		{From: "example.com/fixture/a", To: "example.com/fixture/c"},
	}
	s := snapshot.Snapshot{
		RepoName: "edge-policy",
		GoFacts: &gofacts.Facts{
			PackagesCount: 2,
			InternalEdges: edges,
		},
	}
	opts := Options{MaxEdges: 3, SourceSignals: []sourcesignals.Signal{}}
	bundle := Build(s, nil, opts)
	traced, trace := BuildWithTrace(s, nil, opts)
	bundleJSON, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	tracedJSON, err := json.Marshal(traced)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(Bundle{
		RepoName: "edge-policy",
		Go: goSection{
			PackagesCount:         2,
			ModuleSummaries:       []moduleSummaryCompact{},
			Entrypoints:           []entrypointCompact{},
			OrientationCandidates: []gofacts.OrientationCandidate{},
			ImportantEdges: []gofacts.Edge{
				{From: "example.com/fixture/a", To: "example.com/fixture/c"},
				{From: "example.com/fixture/z", To: "example.com/fixture/b"},
			},
		},
		KnownDocs:            []string{},
		ProviderAllowedPaths: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(bundleJSON) != string(wantJSON) || string(tracedJSON) != string(wantJSON) {
		t.Fatalf("below-cap edge bundle bytes changed:\nordinary: %s\ntraced:   %s\nwant:     %s", bundleJSON, tracedJSON, wantJSON)
	}
	if len(bundle.Warnings) != 0 || len(traced.Warnings) != 0 || trace.Counts.Edges.Omitted != 0 {
		t.Fatalf("below-cap edge warnings/trace changed: ordinary=%#v traced=%#v counts=%#v", bundle.Warnings, traced.Warnings, trace.Counts.Edges)
	}
}

func TestBuildUsesProvidedSourceSignalsWithoutRescanning(t *testing.T) {
	t.Parallel()

	provided := []sourcesignals.Signal{{
		Path:     "internal/worker.go",
		Line:     12,
		Category: "background_loop",
		Match:    "NewTicker",
		Weight:   40,
	}}
	bundle := Build(
		snapshot.Snapshot{
			RepoName: "fixture",
			GoFacts: &gofacts.Facts{OrientationCandidates: []gofacts.OrientationCandidate{{
				Name:      "Background loop",
				Kind:      gofacts.OrientationKindSignalFlow,
				OpenFiles: []string{"internal/worker.go"},
			}}},
		},
		[]string{"internal/worker.go"},
		Options{
			RepoPath:      filepath.Join(t.TempDir(), "missing"),
			SourceSignals: provided,
			MaxFiles:      10,
		},
	)

	if len(bundle.SourceSignals) != 1 || bundle.SourceSignals[0].Path != provided[0].Path {
		t.Fatalf("source signals = %#v, want provided bounded signal", bundle.SourceSignals)
	}
}

func TestBundleRespectsMaxLimits(t *testing.T) {
	var entries []gofacts.Entrypoint
	for i := 0; i < 50; i++ {
		entries = append(entries, gofacts.Entrypoint{
			ModulePath: "m", ImportPath: "m/p", PackageDir: "cmd/app", Kind: "unknown", GoFiles: []string{"main.go"},
		})
	}
	var candidates []gofacts.OrientationCandidate
	for i := 0; i < 100; i++ {
		candidates = append(candidates, gofacts.OrientationCandidate{
			Name: "c", Kind: "unknown", EntrypointPackage: "m/p", OpenFiles: []string{"cmd/app/main.go"},
		})
	}
	var edges []gofacts.Edge
	for i := 0; i < 100; i++ {
		edges = append(edges, gofacts.Edge{
			From: fmt.Sprintf("m/from/%03d", i),
			To:   fmt.Sprintf("m/to/%03d", i),
		})
	}

	s := snapshot.Snapshot{
		RepoName: "t",
		GoFacts: &gofacts.Facts{
			Modules:               []gofacts.ModuleFact{{ModulePath: "m", ModuleDir: "."}},
			EntrypointPackages:    entries,
			OrientationCandidates: candidates,
			InternalEdges:         edges,
			ModuleSummaries:       []gofacts.ModuleSummary{{ModulePath: "m", ModuleDir: "."}},
		},
	}

	bundle := Build(s, []string{"cmd/app/main.go"}, Options{
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
		"docs/operator-guide.rst",
		"docs/Makefile",
		"docs/assets/custom.css",
		"Documentation/etcd-internals/README.md",
		"cmd/app/main.go",
		"go.mod",
	}

	docs := findKnownDocs(files)

	hasArch := false
	hasDocsReadme := false
	hasRST := false
	for _, d := range docs {
		if d == "Documentation/etcd-internals/README.md" {
			hasDocsReadme = true
		}
		if d == "docs/architecture.md" {
			hasArch = true
		}
		if d == "docs/operator-guide.rst" {
			hasRST = true
		}
		if d == "docs/Makefile" || d == "docs/assets/custom.css" {
			t.Fatalf("non-document %q leaked into known_docs", d)
		}
	}
	if !hasArch {
		t.Fatalf("expected docs/architecture.md in known_docs, got: %v", docs)
	}
	if !hasDocsReadme {
		t.Fatalf("expected Documentation/etcd-internals/README.md in known_docs, got: %v", docs)
	}
	if !hasRST {
		t.Fatalf("expected docs/operator-guide.rst in known_docs, got: %v", docs)
	}
}

func TestFindKnownDocsKeepsCurrentDocumentationAheadOfDecisionHistory(t *testing.T) {
	files := []string{
		"docs/agent-room/CURRENT.md",
		"docs/CORE_IDEA.md",
	}
	for index := 0; index < 40; index++ {
		files = append(files, fmt.Sprintf("docs/agent-room/%03d-old-decision.md", index))
	}

	docs := findKnownDocs(files)
	if len(docs) != 30 {
		t.Fatalf("known docs = %d, want unchanged limit 30", len(docs))
	}
	if !containsString(docs, "docs/agent-room/CURRENT.md") || !containsString(docs, "docs/CORE_IDEA.md") {
		t.Fatalf("current documentation was displaced by decision history: %v", docs)
	}
	if docs[0] != "docs/CORE_IDEA.md" || docs[1] != "docs/agent-room/CURRENT.md" {
		t.Fatalf("known docs order = %v, want current docs first", docs[:2])
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

func TestBuildPythonRepositoryWithoutGoFacts(t *testing.T) {
	t.Parallel()

	files := []string{
		"README.md",
		"pyproject.toml",
		"src/tool/__main__.py",
		"src/tool/service.py",
		"tests/test_service.py",
	}
	bundle := Build(snapshot.Snapshot{
		RepoName: "python-tool",
		Readme:   "# Python tool",
		LanguageHints: []snapshot.LanguageHint{
			{Language: "Python", Count: 3},
		},
	}, files, Options{MaxFiles: 20})

	for _, path := range files {
		if !containsString(bundle.ProviderAllowedPaths, path) {
			t.Fatalf("allowed_paths = %v, want %q", bundle.ProviderAllowedPaths, path)
		}
	}
	if bundle.Go.ModulesCount != 0 || len(bundle.Go.Entrypoints) != 0 {
		t.Fatalf("Go section = %#v, want empty", bundle.Go)
	}
	for _, entry := range bundle.CandidateFileIndex {
		switch entry.Path {
		case "src/tool/__main__.py":
			if entry.Kind != "source" || !containsString(entry.Signals, "entrypoint") {
				t.Fatalf("Python entrypoint = %#v", entry)
			}
		case "tests/test_service.py":
			if entry.Kind != "test" {
				t.Fatalf("Python test kind = %q", entry.Kind)
			}
		}
	}
}

func TestFileIndexIncludesEntrypointOpenFiles(t *testing.T) {
	facts := &gofacts.Facts{
		EntrypointPackages: []gofacts.Entrypoint{
			{ModulePath: "m", ImportPath: "m/cmd/app", PackageDir: "cmd/app", GoFiles: []string{"main.go"}},
		},
		OrientationCandidates: []gofacts.OrientationCandidate{
			{
				EntrypointPackage: "m/cmd/app",
				OpenFiles:         []string{"server/main.go", "etcdctl/main.go"},
			},
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

func TestFileIndexMarksOnlyVerifiedMainAnchorAsEntrypoint(t *testing.T) {
	facts := &gofacts.Facts{
		EntrypointPackages: []gofacts.Entrypoint{{
			ImportPath: "example.com/project/cmd/app",
			PackageDir: "cmd/app",
			Kind:       "unknown",
			GoFiles:    []string{"config.go", "start.go"},
			Anchors: []gofacts.EntrypointAnchor{{
				Version: gofacts.EntrypointAnchorVersion,
				Kind:    gofacts.EntrypointAnchorGoMain,
				Path:    "cmd/app/start.go",
				Line:    9,
			}},
		}},
		OrientationCandidates: []gofacts.OrientationCandidate{{
			EntrypointPackage: "example.com/project/cmd/app",
			OpenFiles:         []string{"cmd/app/config.go", "cmd/app/start.go"},
		}},
	}

	entries := buildFileIndex(
		[]string{"cmd/app/config.go", "cmd/app/start.go"},
		facts,
		nil,
		nil,
	)
	foundStart := false
	foundConfig := false
	for _, entry := range entries {
		switch entry.Path {
		case "cmd/app/start.go":
			foundStart = true
			if !containsString(entry.Signals, "entrypoint") {
				t.Fatalf("verified main anchor signals = %v, want entrypoint", entry.Signals)
			}
		case "cmd/app/config.go":
			foundConfig = true
			if containsString(entry.Signals, "entrypoint") {
				t.Fatalf("non-anchor package file signals = %v, must not be an entrypoint", entry.Signals)
			}
		}
	}
	if !foundStart || !foundConfig {
		t.Fatalf("file index paths = %#v, want both package files", entries)
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

func TestFileIndexPrefersEntrypointDependencySourceOverTests(t *testing.T) {
	facts := &gofacts.Facts{
		Modules: []gofacts.ModuleFact{
			{ModulePath: "example.com/project", ModuleDir: "."},
		},
		EntrypointPackages: []gofacts.Entrypoint{
			{
				ModulePath: "example.com/project",
				ImportPath: "example.com/project/cmd/app",
				PackageDir: "cmd/app",
				GoFiles:    []string{"main.go"},
			},
		},
		InternalEdges: []gofacts.Edge{
			{From: "example.com/project/cmd/app", To: "example.com/project/pkg/runtime"},
		},
	}
	fileList := []string{
		"cmd/app/main.go",
		"pkg/runtime/client.go",
		"pkg/runtime/accounts_test.go",
		"pkg/runtime/client_test.go",
	}

	bundle := Build(snapshot.Snapshot{RepoName: "test", GoFacts: facts}, fileList, Options{MaxFiles: 2})

	if len(bundle.CandidateFileIndex) != 2 {
		t.Fatalf("candidate files = %d, want 2", len(bundle.CandidateFileIndex))
	}
	if got := bundle.CandidateFileIndex[1].Path; got != "pkg/runtime/client.go" {
		t.Fatalf("second candidate = %q, want entrypoint dependency source", got)
	}
	if !containsString(bundle.CandidateFileIndex[1].Signals, "entrypoint-dependency") {
		t.Fatalf("dependency signals = %v, want entrypoint-dependency", bundle.CandidateFileIndex[1].Signals)
	}
	for _, entry := range buildFileIndex(fileList, facts, nil, nil) {
		if entry.Kind == "test" && containsString(entry.Signals, "entrypoint-dependency") {
			t.Fatalf("test %q received runtime dependency signal: %v", entry.Path, entry.Signals)
		}
	}
}

func TestFileIndexMarksSecondHopEntrypointDependencySources(t *testing.T) {
	facts := &gofacts.Facts{
		Modules: []gofacts.ModuleFact{
			{ModulePath: "example.com/project", ModuleDir: "."},
		},
		EntrypointPackages: []gofacts.Entrypoint{
			{
				ModulePath: "example.com/project",
				ImportPath: "example.com/project/cmd/app",
				PackageDir: "cmd/app",
				GoFiles:    []string{"main.go"},
			},
		},
		InternalEdges: []gofacts.Edge{
			{From: "example.com/project/cmd/app", To: "example.com/project/pkg/runtime"},
			{From: "example.com/project/pkg/runtime", To: "example.com/project/pkg/engine"},
		},
	}
	fileList := []string{
		"cmd/app/main.go",
		"pkg/runtime/client.go",
		"pkg/engine/engine.go",
		"pkg/engine/engine_test.go",
	}
	entries := buildFileIndex(fileList, facts, nil, nil)
	for _, entry := range entries {
		hasSecondHop := containsString(entry.Signals, "entrypoint-second-hop")
		switch entry.Path {
		case "pkg/engine/engine.go":
			if !hasSecondHop {
				t.Fatalf("second-hop source signals = %v", entry.Signals)
			}
		case "pkg/engine/engine_test.go":
			if hasSecondHop {
				t.Fatalf("second-hop test signals = %v", entry.Signals)
			}
		}
	}
}

func TestFileIndexRetainsPackageNamedSourceAmongEqualDependencyFiles(t *testing.T) {
	facts := &gofacts.Facts{
		Modules: []gofacts.ModuleFact{
			{ModulePath: "example.com/project", ModuleDir: "."},
		},
		EntrypointPackages: []gofacts.Entrypoint{
			{
				ModulePath: "example.com/project",
				ImportPath: "example.com/project",
				PackageDir: ".",
				Kind:       "unknown",
				GoFiles:    []string{"main.go"},
			},
		},
		InternalEdges: []gofacts.Edge{
			{From: "example.com/project", To: "example.com/project/server"},
		},
	}
	fileList := []string{"main.go", "server/server.go"}
	for index := 0; index < 70; index++ {
		fileList = append(fileList, fmt.Sprintf("server/a%03d.go", index))
	}

	bundle := Build(
		snapshot.Snapshot{RepoName: "test", GoFacts: facts},
		fileList,
		Options{MaxFiles: 60},
	)
	if !containsFileIndexPath(bundle.CandidateFileIndex, "server/server.go") {
		t.Fatal("package-named source was displaced by equal-scored sibling files")
	}
	for _, entry := range bundle.CandidateFileIndex {
		if entry.Path == "server/server.go" &&
			!containsString(entry.Signals, "directory-anchor") {
			t.Fatalf("package anchor signals = %v", entry.Signals)
		}
	}
}

func TestRepositoryDirForImportUsesLongestModuleMatch(t *testing.T) {
	modules := []gofacts.ModuleFact{
		{ModulePath: "example.com/project", ModuleDir: "."},
		{ModulePath: "example.com/project/server/v2", ModuleDir: "server"},
	}

	got, ok := repositoryDirForImport("example.com/project/server/v2/runtime/transport", modules)
	if !ok {
		t.Fatal("repositoryDirForImport() did not match nested module")
	}
	if got != "server/runtime/transport" {
		t.Fatalf("repository directory = %q, want server/runtime/transport", got)
	}
}

func TestSelectOrientationEntrypointsIgnoresAuxiliaryMains(t *testing.T) {
	entrypoints := []gofacts.Entrypoint{
		{ImportPath: "example.com/project/cmd/app", PackageDir: "cmd/app", Kind: "unknown"},
		{ImportPath: "example.com/project/scripts/docs", PackageDir: "scripts/docs", Kind: "unknown"},
		{ImportPath: "example.com/project/test/testdata/tool", PackageDir: "test/testdata/tool", Kind: "unknown"},
	}

	selected := selectOrientationEntrypoints(entrypoints)
	if len(selected) != 1 || selected[0].ImportPath != "example.com/project/cmd/app" {
		t.Fatalf("selected entrypoints = %#v, want only cmd/app", selected)
	}
}

func TestSelectOrientationEntrypointsPrefersProductionCommandOverPreviewAndTest(t *testing.T) {
	entrypoints := []gofacts.Entrypoint{
		{ImportPath: "example.com/project/cmd/app-preview", PackageDir: "cmd/app-preview", Kind: "unknown"},
		{ImportPath: "example.com/project/cmd/app-test", PackageDir: "cmd/app-test", Kind: "unknown"},
		{ImportPath: "example.com/project/cmd/app", PackageDir: "cmd/app", Kind: "unknown"},
	}

	selected := selectOrientationEntrypoints(entrypoints)
	if len(selected) != 1 || selected[0].ImportPath != "example.com/project/cmd/app" {
		t.Fatalf("selected entrypoints = %#v, want only production command", selected)
	}
}

func TestBuildSelectsProductionModulesBeforeFixtureModules(t *testing.T) {
	moduleSummaries := []gofacts.ModuleSummary{
		{ModulePath: "example.com/project/testdata/fixture", ModuleDir: "testdata/fixture", EntrypointsCount: 1},
		{ModulePath: "example.com/project/api", ModuleDir: "api", RoleGuess: "api_definitions"},
		{ModulePath: "example.com/project", ModuleDir: ".", EntrypointsCount: 1},
	}
	for index := 0; index < 24; index++ {
		moduleSummaries = append(moduleSummaries, gofacts.ModuleSummary{
			ModulePath: fmt.Sprintf("example.com/project/testdata/fixture-%02d", index),
			ModuleDir:  fmt.Sprintf("testdata/fixture-%02d", index),
		})
	}

	bundle := Build(snapshot.Snapshot{
		RepoName: "project",
		GoFacts:  &gofacts.Facts{ModuleSummaries: moduleSummaries},
	}, []string{"go.mod", "api/api.go"}, Options{MaxModules: 2})
	if len(bundle.Go.ModuleSummaries) != 2 {
		t.Fatalf("module summaries = %#v, want two", bundle.Go.ModuleSummaries)
	}
	if bundle.Go.ModuleSummaries[0].ModuleDir != "." || bundle.Go.ModuleSummaries[1].ModuleDir != "api" {
		t.Fatalf("selected module summaries = %#v, want production root and public api", bundle.Go.ModuleSummaries)
	}
}

func TestSelectImportantEdgesRetainsPrimaryEntryToCoreConnectivity(t *testing.T) {
	modules := []gofacts.ModuleFact{{ModulePath: "example.com/project", ModuleDir: "."}}
	entrypoints := []gofacts.Entrypoint{{
		ImportPath: "example.com/project/cmd/app",
		PackageDir: "cmd/app",
		Kind:       "primary_binary",
	}}
	edges := []gofacts.Edge{
		{From: "example.com/project/cmd/app", To: "example.com/project/testdata/fixture"},
		{From: "example.com/project/cmd/app", To: "example.com/project/bootstrap"},
		{From: "example.com/project/bootstrap", To: "example.com/project/core"},
		{From: "example.com/project/testdata/fixture", To: "example.com/project/testdata/helper"},
	}

	selected := selectImportantEdges(edges, entrypoints, modules, 2)
	want := []gofacts.Edge{
		{From: "example.com/project/cmd/app", To: "example.com/project/bootstrap"},
		{From: "example.com/project/bootstrap", To: "example.com/project/core"},
	}
	if !reflect.DeepEqual(selected, want) {
		t.Fatalf("important edges = %#v, want exact entry-to-core path %#v", selected, want)
	}
}

func TestSelectCommandTracesDoesNotLetPreviewDisplaceProductionCommand(t *testing.T) {
	preview := gofacts.CommandTrace{
		Command: "preview",
		Steps: []gofacts.CommandTraceStep{{
			TargetLocation: evidence.Location{Path: "cmd/app-preview/main.go", Line: 1},
		}},
	}
	production := gofacts.CommandTrace{
		Command: "serve",
		Steps: []gofacts.CommandTraceStep{{
			TargetLocation: evidence.Location{Path: "cmd/app/main.go", Line: 1},
		}},
	}

	selected := selectCommandTraces([]gofacts.CommandTrace{preview, production}, "run preview", 1)
	if len(selected) != 1 || selected[0].Command != "serve" {
		t.Fatalf("selected command traces = %#v, want production command", selected)
	}
	pins := selectedCommandTracePaths([]gofacts.CommandTrace{preview, production})
	if _, ok := pins["cmd/app/main.go"]; !ok {
		t.Fatalf("production command path was not pinned: %v", pins)
	}
	if _, ok := pins["cmd/app-preview/main.go"]; ok {
		t.Fatalf("preview command path was pinned beside production: %v", pins)
	}
}

func TestSelectOrientationEntrypointsFallsBackForLibraryTools(t *testing.T) {
	entrypoints := []gofacts.Entrypoint{
		{ImportPath: "example.com/project/tools/four", PackageDir: "tools/four", Kind: "tool"},
		{ImportPath: "example.com/project/tools/three", PackageDir: "tools/three", Kind: "tool"},
		{ImportPath: "example.com/project/tools/two", PackageDir: "tools/two", Kind: "tool"},
		{ImportPath: "example.com/project/tools/one", PackageDir: "tools/one", Kind: "tool"},
	}

	selected := selectOrientationEntrypoints(entrypoints)
	if len(selected) != 3 || selected[0].ImportPath != "example.com/project/tools/four" ||
		selected[2].ImportPath != "example.com/project/tools/three" {
		t.Fatalf("fallback entrypoints = %#v, want lexicographically first three", selected)
	}
}

func TestSelectFileIndexReservesDiverseEvidenceAndFillsShortages(t *testing.T) {
	entries := []fileIndexEntry{
		{Path: "api/service.proto", Kind: "proto", Score: 120},
		{Path: "api/events.proto", Kind: "proto", Score: 119},
		{Path: "config/default.yaml", Kind: "config", Score: 118},
		{Path: "config/secure.yaml", Kind: "config", Score: 117},
		{Path: "api/service.pb.go", Kind: "generated", Score: 116},
		{Path: "api/events.pb.go", Kind: "generated", Score: 115},
	}
	for i := 0; i < 70; i++ {
		entries = append(entries, fileIndexEntry{Path: fmt.Sprintf("source/%03d.go", i), Kind: "source", Score: 100})
	}
	for i := 0; i < 70; i++ {
		entries = append(entries, fileIndexEntry{Path: fmt.Sprintf("source/%03d_test.go", i), Kind: "test", Score: 90})
	}
	entries = append(entries,
		fileIndexEntry{Path: "docs/architecture.md", Kind: "doc", Score: 80},
		fileIndexEntry{Path: "README.md", Kind: "doc", Score: 70},
	)

	selected := selectFileIndex(entries, 60)
	counts := make(map[string]int)
	for _, entry := range selected {
		counts[fileIndexGroup(entry.Kind)]++
	}

	if len(selected) != 60 {
		t.Fatalf("selected files = %d, want 60", len(selected))
	}
	if counts["source"] != 48 || counts["test"] != 6 || counts["doc"] != 2 || counts["support"] != 4 {
		t.Fatalf("selected groups = %v, want production sources to displace generated flex entries", counts)
	}
	if selected[0].Path != "api/service.proto" || selected[len(selected)-1].Path != "README.md" {
		t.Fatalf("selection order changed: first=%q last=%q", selected[0].Path, selected[len(selected)-1].Path)
	}
}

func TestSelectFileIndexFlexCanChooseAdditionalSources(t *testing.T) {
	var entries []fileIndexEntry
	for i := 0; i < 50; i++ {
		entries = append(entries, fileIndexEntry{Path: fmt.Sprintf("source/%03d.go", i), Kind: "source", Score: 120})
	}
	for i := 0; i < 6; i++ {
		entries = append(entries, fileIndexEntry{Path: fmt.Sprintf("api/%03d.proto", i), Kind: "proto", Score: 110})
	}
	for i := 0; i < 6; i++ {
		entries = append(entries, fileIndexEntry{Path: fmt.Sprintf("source/%03d_test.go", i), Kind: "test", Score: 100})
	}
	for i := 0; i < 4; i++ {
		entries = append(entries, fileIndexEntry{Path: fmt.Sprintf("docs/%03d.md", i), Kind: "doc", Score: 90})
	}

	selected := selectFileIndex(entries, 60)
	counts := make(map[string]int)
	seen := make(map[string]struct{})
	for _, entry := range selected {
		counts[fileIndexGroup(entry.Kind)]++
		if _, duplicate := seen[entry.Path]; duplicate {
			t.Fatalf("duplicate selected path %q", entry.Path)
		}
		seen[entry.Path] = struct{}{}
	}
	if len(selected) != 60 || counts["source"] != 50 || counts["test"] != 6 || counts["doc"] != 4 {
		t.Fatalf("selected files/groups = %d/%v, want source flex before support", len(selected), counts)
	}
}

func TestSelectFileIndexPinsUserFacingEntrypoint(t *testing.T) {
	var entries []fileIndexEntry
	for i := 0; i < 70; i++ {
		entries = append(entries, fileIndexEntry{
			Path:  fmt.Sprintf("runtime/%03d.go", i),
			Kind:  "source",
			Score: 200,
		})
	}
	entries = append(entries, fileIndexEntry{
		Path:    "main.go",
		Kind:    "source",
		Score:   100,
		Signals: []string{"entrypoint", "source"},
	})

	selected := selectFileIndex(entries, 60)
	if len(selected) != 60 {
		t.Fatalf("selected files = %d, want 60", len(selected))
	}
	if !containsFileIndexPath(selected, "main.go") {
		t.Fatal("user-facing entrypoint was displaced by higher-scored runtime sources")
	}
	selected = selectFileIndex(entries, 1)
	if len(selected) != 1 || selected[0].Path != "main.go" {
		t.Fatalf("one-file selection = %#v, want pinned main.go", selected)
	}
}

func TestBuildFileIndexDoesNotLetPreviewSignalsConsumeProductionSlots(t *testing.T) {
	facts := &gofacts.Facts{
		Modules: []gofacts.ModuleFact{{ModulePath: "example.com/project", ModuleDir: "."}},
		EntrypointPackages: []gofacts.Entrypoint{{
			ImportPath: "example.com/project/cmd/app",
			PackageDir: "cmd/app",
			Kind:       "primary_binary",
			GoFiles:    []string{"main.go"},
		}},
	}
	files := []string{
		"cmd/app/main.go",
		"internal/core/core.go",
		"internal/storage/write.go",
	}
	signals := []sourcesignals.Signal{
		{Path: "internal/storage/write.go", Category: "storage_durability", Weight: 40},
	}
	for index := 0; index < 20; index++ {
		filePath := fmt.Sprintf("cmd/tool-preview/%02d.go", index)
		files = append(files, filePath)
		signals = append(signals, sourcesignals.Signal{
			Path: filePath, Category: "background_loop", Weight: 200,
		})
	}

	bundle := Build(
		snapshot.Snapshot{RepoName: "project", GoFacts: facts},
		files,
		Options{MaxFiles: 3, SourceSignals: signals},
	)
	want := []string{"cmd/app/main.go", "internal/core/core.go", "internal/storage/write.go"}
	for _, filePath := range want {
		if !containsFileIndexPath(bundle.CandidateFileIndex, filePath) {
			t.Fatalf("candidate anchors = %#v, missing production role %q", bundle.CandidateFileIndex, filePath)
		}
	}
	for _, entry := range bundle.CandidateFileIndex {
		if strings.Contains(entry.Path, "preview") {
			t.Fatalf("preview consumed a production candidate slot: %#v", bundle.CandidateFileIndex)
		}
	}
}

func TestBuildFileIndexAuxiliaryPythonEntrypointDoesNotConsumeProductionSlot(t *testing.T) {
	bundle := Build(
		snapshot.Snapshot{RepoName: "project"},
		[]string{
			"cmd/app-preview/main.py",
			"app/main.py",
			"internal/core/engine.py",
		},
		Options{MaxFiles: 2},
	)
	for _, required := range []string{"app/main.py", "internal/core/engine.py"} {
		if !containsFileIndexPath(bundle.CandidateFileIndex, required) {
			t.Fatalf("candidate anchors = %#v, missing production path %q", bundle.CandidateFileIndex, required)
		}
	}
	if containsFileIndexPath(bundle.CandidateFileIndex, "cmd/app-preview/main.py") {
		t.Fatalf("preview Python entrypoint consumed a production slot: %#v", bundle.CandidateFileIndex)
	}
}

// Retrieval-regression fixture: this protects the observed onboarding outcome,
// not the current scoring or quota implementation. Replace or remove it when
// selection becomes package-first, as long as dominant packages still cannot
// hide useful core anchors.
func TestSelectFileIndexKeepsCoreAnchorsWhenOnePackageDominates(t *testing.T) {
	entries := []fileIndexEntry{
		{
			Path:    "server/main.go",
			Kind:    "source",
			Score:   200,
			Signals: []string{"entrypoint", "source"},
		},
	}
	for i := 0; i < 20; i++ {
		entries = append(entries, fileIndexEntry{
			Path:  fmt.Sprintf("server/transport/%02d.go", i),
			Kind:  "source",
			Score: 190 - i,
		})
	}
	entries = append(entries,
		fileIndexEntry{Path: "server/etcdserver/server.go", Kind: "source", Score: 120},
		fileIndexEntry{Path: "server/storage/wal.go", Kind: "source", Score: 110},
	)

	selected := selectFileIndex(entries, 7)
	if !containsFileIndexPath(selected, "server/etcdserver/server.go") {
		t.Fatalf("core package anchor was crowded out: %#v", selected)
	}
	if !containsFileIndexPath(selected, "server/storage/wal.go") {
		t.Fatalf("storage package was crowded out: %#v", selected)
	}

}

// Retrieval-regression fixture: this scenario may be replaced when the
// selector becomes package-first. The durable product expectation is that a
// bounded survey can retain a distinctive core file reached through startup
// wiring instead of spending the whole budget on nearer sibling packages.
func TestBuildKeepsDistinctiveCoreFileThroughStartupWiring(t *testing.T) {
	facts := &gofacts.Facts{
		Modules: []gofacts.ModuleFact{{ModulePath: "example.com/project", ModuleDir: "."}},
		EntrypointPackages: []gofacts.Entrypoint{{
			ImportPath: "example.com/project/cmd/app",
			PackageDir: "cmd/app",
			Kind:       "primary_binary",
			GoFiles:    []string{"main.go"},
		}},
		InternalEdges: []gofacts.Edge{
			{From: "example.com/project/cmd/app", To: "example.com/project/bootstrap"},
			{From: "example.com/project/bootstrap", To: "example.com/project/embed"},
			{From: "example.com/project/embed", To: "example.com/project/core"},
		},
	}
	files := []string{
		"cmd/app/main.go",
		"bootstrap/config.go",
		"bootstrap/start.go",
		"embed/config.go",
		"embed/start.go",
		"core/raft.go",
	}

	bundle := Build(
		snapshot.Snapshot{RepoName: "test", GoFacts: facts},
		files,
		Options{MaxFiles: 3},
	)
	if !containsString(bundle.ProviderAllowedPaths, "core/raft.go") {
		t.Fatalf("distinctive third-hop core file was omitted: %v", bundle.ProviderAllowedPaths)
	}
}

func TestBundleFiltersEveryModelVisibleFilePathToAllowedPaths(t *testing.T) {
	facts := &gofacts.Facts{
		Modules: []gofacts.ModuleFact{{ModulePath: "example.com/project", ModuleDir: "."}},
		EntrypointPackages: []gofacts.Entrypoint{
			{ImportPath: "example.com/project/cmd/app", PackageDir: "cmd/app", Kind: "unknown", GoFiles: []string{"main.go"}},
			{ImportPath: "example.com/project/scripts/docs", PackageDir: "scripts/docs", Kind: "unknown", GoFiles: []string{"main.go"}},
		},
		OrientationCandidates: []gofacts.OrientationCandidate{
			{EntrypointPackage: "example.com/project/cmd/app", OpenFiles: []string{"cmd/app/main.go"}},
			{EntrypointPackage: "example.com/project/scripts/docs", OpenFiles: []string{"scripts/docs/main.go"}},
		},
		InternalEdges: []gofacts.Edge{
			{From: "example.com/project/cmd/app", To: "example.com/project/pkg/runtime"},
		},
	}
	fileList := []string{
		"cmd/app/main.go",
		"pkg/runtime/runtime.go",
		"scripts/docs/main.go",
		"docs/architecture.md",
	}

	bundle := Build(snapshot.Snapshot{RepoName: "test", GoFacts: facts}, fileList, Options{MaxFiles: 2})
	allowed := makePathSet(bundle.ProviderAllowedPaths)
	if len(bundle.KnownDocs) != 0 {
		t.Fatalf("known docs outside selected allowlist survived: %v", bundle.KnownDocs)
	}
	if len(bundle.Go.Entrypoints) != 1 || len(bundle.Go.OrientationCandidates) != 1 {
		t.Fatalf("model entrypoints/candidates = %d/%d, want only user-facing selection",
			len(bundle.Go.Entrypoints), len(bundle.Go.OrientationCandidates))
	}
	for _, entrypoint := range bundle.Go.Entrypoints {
		for _, openFile := range entrypoint.OpenFiles {
			if _, ok := allowed[openFile]; !ok {
				t.Fatalf("entrypoint path %q is outside allowed_paths", openFile)
			}
		}
	}
	for _, candidate := range bundle.Go.OrientationCandidates {
		for _, openFile := range candidate.OpenFiles {
			if _, ok := allowed[openFile]; !ok {
				t.Fatalf("candidate path %q is outside allowed_paths", openFile)
			}
		}
	}

	signals := filterSourceSignals([]sourcesignals.Signal{
		{Path: "cmd/app/main.go"},
		{Path: "scripts/docs/main.go"},
	}, allowed)
	if len(signals) != 1 || signals[0].Path != "cmd/app/main.go" {
		t.Fatalf("filtered source signals = %#v, want only allowed path", signals)
	}
}

func TestScoreFileUsesComponentTokenBoundaries(t *testing.T) {
	empty := map[string]struct{}{}

	releaseScore, releaseSignals, _ := scoreFile(
		"scripts/testdata/all-releases.json",
		"config",
		empty,
		empty,
		empty,
		empty,
		empty,
	)
	leaseScore, leaseSignals, _ := scoreFile(
		"server/lease/lessor.go",
		"source",
		empty,
		empty,
		empty,
		empty,
		empty,
	)

	if containsString(releaseSignals, "lease") || releaseScore != 20 {
		t.Fatalf("release file score/signals = %d/%v, want config only", releaseScore, releaseSignals)
	}
	if !containsString(leaseSignals, "lease") || leaseScore <= releaseScore {
		t.Fatalf("lease source score/signals = %d/%v, want lease component", leaseScore, leaseSignals)
	}
}

// Heuristic unit test: delete or replace this table with the package-first
// selector. It only prevents generic filenames from regaining anchor weight in
// the current transitional scorer.
func TestPackageAnchorSourceUsesArchitectureRoles(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "server role", path: "server/etcdserver/server.go", want: true},
		{name: "generic main", path: "server/etcdmain/main.go", want: false},
		{name: "generic utility", path: "client/pathutil/util.go", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isPackageAnchorSource(test.path); got != test.want {
				t.Fatalf("isPackageAnchorSource(%q) = %t, want %t", test.path, got, test.want)
			}
		})
	}
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

	if len(bundle.ProviderAllowedPaths) == 0 {
		t.Fatal("allowed_paths is empty")
	}

	allowedSet := make(map[string]bool)
	for _, p := range bundle.ProviderAllowedPaths {
		allowedSet[p] = true
	}

	for _, e := range bundle.CandidateFileIndex {
		if !allowedSet[e.Path] {
			t.Fatalf("file index path %q not in allowed_paths", e.Path)
		}
	}
}

func TestMaxFilesLimitsOnlyProviderVisibleEvidence(t *testing.T) {
	trace := gofacts.CommandTrace{
		Version:           gofacts.CommandTraceVersion,
		Framework:         "cobra",
		EntrypointPackage: "example.com/project/cmd/app",
		Command:           "backup",
		Steps: []gofacts.CommandTraceStep{
			{Symbol: "main", Relation: "entrypoint", TargetLocation: evidence.Location{Path: "cmd/app/main.go", Line: 3}},
			{Symbol: "newRoot", Relation: "calls", TargetLocation: evidence.Location{Path: "cmd/app/root.go", Line: 5}},
			{Symbol: "newBackup", Relation: "registers_command", TargetLocation: evidence.Location{Path: "cmd/app/backup.go", Line: 7}},
			{Symbol: "runBackup", Relation: "callback", TargetLocation: evidence.Location{Path: "cmd/app/run.go", Line: 9}},
		},
	}
	facts := &gofacts.Facts{CommandTraces: []gofacts.CommandTrace{trace}}
	files := []string{"cmd/app/main.go", "cmd/app/root.go", "cmd/app/backup.go", "cmd/app/run.go"}

	bundle := Build(snapshot.Snapshot{RepoName: "project", GoFacts: facts}, files, Options{MaxFiles: 1})

	if len(bundle.ProviderAllowedPaths) != 1 {
		t.Fatalf("provider allowed paths = %v, want one path", bundle.ProviderAllowedPaths)
	}
	if len(bundle.Go.CommandTraces) != 0 {
		t.Fatalf("provider command traces = %#v, want trace omitted when all exact paths do not fit", bundle.Go.CommandTraces)
	}
	if len(facts.CommandTraces) != 1 || len(facts.CommandTraces[0].Steps) != 4 {
		t.Fatalf("local command traces were mutated by provider selection: %#v", facts.CommandTraces)
	}
}

func TestBuildUsesSerializedBytesAsPrimaryProviderLimit(t *testing.T) {
	files := make([]string, 0, 180)
	for index := 0; index < 180; index++ {
		files = append(files, fmt.Sprintf("internal/service/handler_%03d.go", index))
	}

	bundle := Build(snapshot.Snapshot{RepoName: "large"}, files, Options{
		MaxFiles: 180,
		MaxBytes: 12 << 10,
	})
	encoded, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > 12<<10 {
		t.Fatalf("provider bundle bytes = %d, want at most %d", len(encoded), 12<<10)
	}
	if len(bundle.CandidateFileIndex) >= len(files) {
		t.Fatalf("provider summaries = %d, want byte fitting below %d candidates", len(bundle.CandidateFileIndex), len(files))
	}
	if bundle.LocalAuthorizedFiles != len(files) {
		t.Fatalf("local authorized files = %d, want %d", bundle.LocalAuthorizedFiles, len(files))
	}
	if len(bundle.CandidateFileIndex) == 0 || bundle.CandidateFileIndex[0].ID == "" {
		t.Fatalf("provider candidate IDs are missing: %#v", bundle.CandidateFileIndex)
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
		{"src/widget.ts", "source"},
		{"web/client.js", "source"},
		{"tool/worker.py", "source"},
		{"src/Worker.java", "source"},
		{"engine/lib.rs", "source"},
		{"native/hash.c", "source"},
		{"native/hash.cpp", "source"},
		{"server/etcdserver_impl_test.go", "test"},
		{"docs/design.md", "doc"},
		{"docs/operator-guide.rst", "doc"},
		{"README", "doc"},
		{"docs/Makefile", "unknown"},
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

func TestBuildFileIndexKeepsSmallRepositorySourcesWithoutConventionalNames(t *testing.T) {
	t.Parallel()

	sourcePaths := []string{
		"src/widget.ts",
		"web/client.js",
		"tool/worker.py",
		"src/Worker.java",
		"engine/lib.rs",
		"native/hash.c",
		"native/hash.cpp",
	}
	files := append(append([]string(nil), sourcePaths...), "README.md", "package.json")
	index := buildFileIndex(files, nil, nil, nil)
	for _, filePath := range sourcePaths {
		if !containsFileIndexPath(index, filePath) {
			t.Errorf("small repository source %q disappeared from candidate_file_index: %#v", filePath, index)
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

	if len(bundle.ProviderAllowedPaths) == 0 {
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

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsFileIndexPath(entries []fileIndexEntry, target string) bool {
	for _, entry := range entries {
		if entry.Path == target {
			return true
		}
	}
	return false
}
