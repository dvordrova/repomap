package llmbundle

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

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
		edges = append(edges, gofacts.Edge{})

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
	if counts["source"] != 46 || counts["test"] != 6 || counts["doc"] != 2 || counts["support"] != 6 {
		t.Fatalf("selected groups = %v, want source=46 test=6 doc=2 support=6 after flex fill", counts)
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
	allowed := makePathSet(bundle.AllowedPaths)
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
	)
	leaseScore, leaseSignals, _ := scoreFile(
		"server/lease/lessor.go",
		"source",
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
