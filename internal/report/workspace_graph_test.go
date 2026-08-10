package report

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/workspacegraph"
	"github.com/dvordrova/repomap/internal/workspacepackageselection"
	"github.com/dvordrova/repomap/internal/workspacesnapshot"
)

func TestWorkspacePackageGraphProjectionMaterializesExactEdges(t *testing.T) {
	tests := []struct {
		name            string
		repositoryRoot  string
		analysisRoot    string
		facts           gofacts.Facts
		allowedPaths    []string
		selectedEdges   []EdgeInfo
		moduleSummaries []map[string]any
	}{
		{
			name:           "repository root with nested module",
			repositoryRoot: "/workspacegraph-report-root",
			analysisRoot:   "/workspacegraph-report-root",
			facts:          reportRootFacts(),
			allowedPaths: []string{
				"cmd/app/main.go",
				"internal/core/core.go",
				"tools/cmd/tool/main.go",
			},
			selectedEdges: []EdgeInfo{
				{From: "example.com/repo/tools/cmd/tool", To: "example.com/repo/internal/core"},
				{From: "example.com/repo/cmd/app", To: "example.com/repo/internal/core"},
				{From: "example.com/repo/cmd/app", To: "example.com/repo/internal/core"},
			},
			moduleSummaries: []map[string]any{{
				"module_path": "example.com/compat",
				"module_dir":  "compat",
			}},
		},
		{
			name:           "subdirectory analysis root",
			repositoryRoot: "/workspacegraph-report-subdirectory",
			analysisRoot:   "/workspacegraph-report-subdirectory/service",
			facts:          reportSubdirectoryFacts(),
			allowedPaths: []string{
				"cmd/app/main.go",
				"internal/core/core.go",
			},
			selectedEdges: []EdgeInfo{{
				From: "example.com/service/cmd/app",
				To:   "example.com/service/internal/core",
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := parseLegacyGraphFixture(
				t,
				test.facts,
				test.allowedPaths,
				test.selectedEdges,
				test.moduleSummaries,
			)
			legacy := cloneRepositoryGraph(data.RepositoryGraph)
			if data.repositoryGoFacts == nil {
				t.Fatal("exact graph facts were not captured")
			}
			exactFacts := *data.repositoryGoFacts
			snapshot := reportGraphSnapshot(
				t,
				test.repositoryRoot,
				test.analysisRoot,
				test.allowedPaths,
			)
			graph, err := workspacegraph.New(workspacegraph.Input{
				Snapshot: snapshot,
				GoFacts:  exactFacts,
			})
			if err != nil {
				t.Fatalf("workspacegraph.New: %v", err)
			}
			projected, err := projectWorkspacePackageGraph(legacy, exactFacts, graph)
			if err != nil {
				t.Fatalf("projectWorkspacePackageGraph: %v", err)
			}
			want := cloneRepositoryGraph(legacy)
			want.PackageEdges = reportEdgesFromWorkspaceGraph(graph)
			if !reflect.DeepEqual(projected, want) {
				t.Fatalf("projection differs:\nwant: %#v\nnew:  %#v", want, projected)
			}
			if got, expected := mustJSON(t, projected), mustJSON(t, want); string(got) != string(expected) {
				t.Fatalf("projection bytes differ:\nwant: %s\nnew:  %s", expected, got)
			}
			if projected.Version != 2 {
				t.Fatalf("graph version = %d, want 2", projected.Version)
			}
			if len(projected.Packages) == 0 ||
				projected.Packages[0].DisplayPath != legacy.Packages[0].DisplayPath ||
				projected.Packages[0].Locality != legacy.Packages[0].Locality {
				t.Fatalf("presentation fields changed: %#v", projected.Packages)
			}
			if test.name == "repository root with nested module" {
				if len(projected.Modules) != len(test.facts.Modules)+1 ||
					projected.Modules[len(projected.Modules)-1] != (ModuleInfo{
						Path: "example.com/compat", Dir: "compat",
					}) {
					t.Fatalf("compatibility modules changed: %#v", projected.Modules)
				}
				if len(projected.PackageEdges) != 2 ||
					projected.PackageEdges[0].From != "example.com/repo/cmd/app" {
					t.Fatalf("exact edges were not sorted and deduplicated: %#v", projected.PackageEdges)
				}
			}
		})
	}
}

func TestWorkspacePackageGraphProjectionKeepsCompositePackageIdentities(t *testing.T) {
	const sharedPath = "example.com/shared"
	facts := gofacts.Facts{
		Modules: []gofacts.ModuleFact{
			{ID: "fixture-a", ModulePath: sharedPath, ModuleDir: "fixtures/a"},
			{ID: "fixture-b", ModulePath: sharedPath, ModuleDir: "fixtures/b"},
		},
		Packages: []gofacts.PackageFact{
			{
				CanonicalPath: sharedPath, Name: "main",
				ModuleID: "fixture-a", ModulePath: sharedPath,
				PackageDir: "fixtures/a", ModuleRelativeDir: ".",
			},
			{
				CanonicalPath: sharedPath, Name: "sum",
				ModuleID: "fixture-b", ModulePath: sharedPath,
				PackageDir: "fixtures/b", ModuleRelativeDir: ".",
			},
		},
	}
	data := parseLegacyGraphFixture(t, facts, nil, nil, nil)
	authority := reportGraphAuthority(
		t,
		"/workspacegraph-report-composite",
		"/workspacegraph-report-composite",
		nil,
	)
	attachAuthorizedWorkspacePackageGraph(data, &authority)
	if err := requireCompleteExactWorkspaceGraph(data); err != nil {
		t.Fatalf("requireCompleteExactWorkspaceGraph: %v", err)
	}
	if len(data.RepositoryGraph.Packages) != 2 ||
		data.RepositoryGraph.Packages[0].ModuleID != "fixture-a" ||
		data.RepositoryGraph.Packages[1].ModuleID != "fixture-b" ||
		data.RepositoryGraph.Packages[0].CanonicalPath != sharedPath ||
		data.RepositoryGraph.Packages[1].CanonicalPath != sharedPath {
		t.Fatalf("composite package projection = %#v", data.RepositoryGraph.Packages)
	}
}

func TestDecodeSnapshotExactGoFactsAcceptsBoundedSQLCScale(t *testing.T) {
	t.Parallel()

	const (
		rootPackages    = 92
		fixturePackages = 893
		edgeCount       = 209
	)
	saved := snapshotExactGoFacts{
		Modules: []snapshotExactModuleFact{
			{ID: "root", ModulePath: "example.com/product", ModuleDir: ".", Main: true},
			{ID: "fixture", ModulePath: "example.com/product/endtoend", ModuleDir: "internal/endtoend/testdata", Main: true},
		},
		Packages: make([]snapshotExactPackageFact, 0, rootPackages+fixturePackages),
	}
	rootPaths := make([]string, rootPackages)
	for index := range rootPackages {
		packagePath := fmt.Sprintf("example.com/product/pkg/p%03d", index)
		rootPaths[index] = packagePath
		saved.Packages = append(saved.Packages, snapshotExactPackageFact{
			CanonicalPath:     packagePath,
			Name:              fmt.Sprintf("p%03d", index),
			ModuleID:          "root",
			ModulePath:        "example.com/product",
			PackageDir:        fmt.Sprintf("pkg/p%03d", index),
			ModuleRelativeDir: fmt.Sprintf("pkg/p%03d", index),
		})
	}
	for index := range fixturePackages {
		saved.Packages = append(saved.Packages, snapshotExactPackageFact{
			CanonicalPath:     fmt.Sprintf("example.com/product/endtoend/case%03d", index),
			Name:              fmt.Sprintf("case%03d", index),
			ModuleID:          "fixture",
			ModulePath:        "example.com/product/endtoend",
			PackageDir:        fmt.Sprintf("internal/endtoend/testdata/case%03d", index),
			ModuleRelativeDir: fmt.Sprintf("case%03d", index),
		})
	}
	for offset := 1; len(saved.InternalEdges) < edgeCount; offset++ {
		for index := 0; index < rootPackages && len(saved.InternalEdges) < edgeCount; index++ {
			saved.InternalEdges = append(saved.InternalEdges, gofacts.Edge{
				From: rootPaths[index], To: rootPaths[(index+offset)%rootPackages],
			})
		}
	}

	raw := mustJSON(t, map[string]any{"go_facts": saved})
	facts, err := decodeSnapshotExactGoFacts(raw)
	if err != nil {
		t.Fatalf("decodeSnapshotExactGoFacts: %v", err)
	}
	if len(facts.Modules) != 2 || len(facts.Packages) != rootPackages+fixturePackages ||
		len(facts.InternalEdges) != edgeCount {
		t.Fatalf(
			"decoded facts modules/packages/edges = %d/%d/%d",
			len(facts.Modules), len(facts.Packages), len(facts.InternalEdges),
		)
	}
}

func TestWorkspacePackageGraphCasdoorShapeFeedsArchitectureRelations(t *testing.T) {
	const edgeCount = 90
	facts := casdoorShapedGraphFacts(edgeCount)
	legacy := &RepositoryGraph{
		Version: 2,
		Modules: []ModuleInfo{{
			ID: "root-id", Path: "github.com/casdoor/casdoor", Dir: "",
		}},
		Packages: make([]PackageInfo, len(facts.Packages)),
	}
	for index, fact := range facts.Packages {
		legacy.Packages[index] = PackageInfo{
			CanonicalPath:     fact.CanonicalPath,
			Name:              fact.Name,
			ModuleID:          fact.ModuleID,
			ModulePath:        fact.ModulePath,
			Dir:               fact.PackageDir,
			ModuleRelativeDir: fact.ModuleRelativeDir,
			DisplayPath:       fact.PackageDir,
			Locality:          "local",
		}
	}

	const repositoryRoot = "/workspacegraph-report-casdoor"
	graph, err := workspacegraph.New(workspacegraph.Input{
		Snapshot: reportGraphSnapshot(t, repositoryRoot, repositoryRoot, nil),
		GoFacts:  facts,
	})
	if err != nil {
		t.Fatalf("workspacegraph.New: %v", err)
	}
	projected, err := projectWorkspacePackageGraph(legacy, facts, graph)
	if err != nil {
		t.Fatalf("projectWorkspacePackageGraph: %v", err)
	}
	if got := len(projected.PackageEdges); got != edgeCount {
		t.Fatalf("exact projected edges = %d, want %d", got, edgeCount)
	}

	input, err := BuildArchitectureCanvasInput(&ReportData{
		RepoName:        "github.com/casdoor/casdoor",
		RepositoryGraph: projected,
	})
	if err != nil {
		t.Fatalf("BuildArchitectureCanvasInput: %v", err)
	}
	if got := len(input.CandidateBundle.Relations); got != edgeCount {
		t.Fatalf("Architecture supporting relations = %d, want %d", got, edgeCount)
	}
}

func TestReadRunDirForAuthorizedArchitectureIsExactAndByteStable(t *testing.T) {
	const edgeCount = 90
	facts := casdoorShapedGraphFacts(edgeCount)
	runDir := t.TempDir()
	mkdirAll(t, filepath.Join(runDir, "flows"))
	writeTestFile(t, runDir, "snapshot.json", string(mustJSON(t, map[string]any{
		"repo_name": "github.com/casdoor/casdoor",
		"go_facts":  facts,
	})))
	writeTestFile(t, runDir, "llm_bundle.json", `{}`)
	authority := reportGraphAuthority(
		t,
		"/workspacegraph-authorized-casdoor",
		"/workspacegraph-authorized-casdoor",
		nil,
	)

	first, err := ReadRunDirForAuthorizedArchitecture(runDir, authority)
	if err != nil {
		t.Fatalf("first authorized read: %v", err)
	}
	second, err := ReadRunDirForAuthorizedArchitecture(runDir, authority)
	if err != nil {
		t.Fatalf("second authorized read: %v", err)
	}
	if got := len(first.RepositoryGraph.PackageEdges); got != edgeCount {
		t.Fatalf("authorized exact edges = %d, want %d", got, edgeCount)
	}
	input, err := BuildArchitectureCanvasInput(first)
	if err != nil {
		t.Fatalf("BuildArchitectureCanvasInput: %v", err)
	}
	if got := len(input.CandidateBundle.Relations); got != edgeCount {
		t.Fatalf("authorized Architecture relations = %d, want %d", got, edgeCount)
	}
	if got, want := mustJSON(t, second), mustJSON(t, first); string(got) != string(want) {
		t.Fatalf("authorized replay changed bytes:\nfirst:  %s\nsecond: %s", want, got)
	}
}

func TestReadRunDirForAuthorizedArchitectureRejectsIncompleteExactGraph(t *testing.T) {
	facts := casdoorShapedGraphFacts(1)
	facts.InternalEdges = make([]gofacts.Edge, maxReportGraphFactEdges+1)
	for index := range facts.InternalEdges {
		facts.InternalEdges[index] = gofacts.Edge{
			From: facts.Packages[0].CanonicalPath,
			To:   facts.Packages[1].CanonicalPath,
		}
	}
	runDir := t.TempDir()
	mkdirAll(t, filepath.Join(runDir, "flows"))
	writeTestFile(t, runDir, "snapshot.json", string(mustJSON(t, map[string]any{
		"repo_name": "fixture",
		"go_facts":  facts,
	})))
	writeTestFile(t, runDir, "llm_bundle.json", `{}`)
	authority := reportGraphAuthority(
		t,
		"/workspacegraph-authorized-incomplete",
		"/workspacegraph-authorized-incomplete",
		nil,
	)

	data, err := ReadRunDirForAuthorizedArchitecture(runDir, authority)
	if !IsExactWorkspaceGraphUnavailable(err) {
		t.Fatalf("authorized read error = %T / %v, want exact graph unavailable", err, err)
	}
	if data == nil || data.ArchitectureCanvas == nil || data.RepositoryGraph == nil {
		t.Fatalf("local D177 Canvas did not survive incomplete exact graph: %#v", data)
	}
	if len(data.RepositoryGraph.PackageEdges) != 0 {
		t.Fatalf("incomplete exact graph published partial edges: %#v", data.RepositoryGraph.PackageEdges)
	}
}

func TestWorkspacePackageGraphPreservesArchitectureComponentAndSearchConsumers(t *testing.T) {
	facts := reportRootFacts()
	selected := []EdgeInfo{
		{From: "example.com/repo/tools/cmd/tool", To: "example.com/repo/internal/core"},
		{From: "example.com/repo/cmd/app", To: "example.com/repo/internal/core"},
	}
	data := parseLegacyGraphFixture(t, facts, []string{
		"cmd/app/main.go",
		"internal/core/core.go",
		"tools/cmd/tool/main.go",
	}, selected, nil)
	legacy := cloneRepositoryGraph(data.RepositoryGraph)
	if data.repositoryGoFacts == nil {
		t.Fatal("exact graph facts were not captured")
	}
	exactFacts := *data.repositoryGoFacts
	snapshot := reportGraphSnapshot(
		t,
		"/workspacegraph-report-consumers",
		"/workspacegraph-report-consumers",
		data.OpenablePaths,
	)
	graph, err := workspacegraph.New(workspacegraph.Input{
		Snapshot: snapshot,
		GoFacts:  exactFacts,
	})
	if err != nil {
		t.Fatal(err)
	}
	projected, err := projectWorkspacePackageGraph(legacy, exactFacts, graph)
	if err != nil {
		t.Fatal(err)
	}

	components := []Component{
		{ID: "app", Packages: []string{"example.com/repo/cmd/app"}},
		{ID: "core", Packages: []string{"example.com/repo/internal/core"}},
		{ID: "tool", Packages: []string{"example.com/repo/tools/cmd/tool"}},
	}
	legacyRelations := buildComponentRelations(&ReportData{
		RepositoryGraph: legacy,
		Components:      append([]Component(nil), components...),
	})
	projectedRelations := buildComponentRelations(&ReportData{
		RepositoryGraph: projected,
		Components:      append([]Component(nil), components...),
	})
	if !reflect.DeepEqual(projectedRelations, legacyRelations) {
		t.Fatalf(
			"component relations changed:\nlegacy: %#v\nnew:    %#v",
			legacyRelations,
			projectedRelations,
		)
	}

	legacyData := &ReportData{RepoName: "fixture", RepositoryGraph: legacy}
	projectedData := &ReportData{RepoName: "fixture", RepositoryGraph: projected}
	legacyInput, err := BuildArchitectureCanvasInput(legacyData)
	if err != nil {
		t.Fatalf("legacy BuildArchitectureCanvasInput: %v", err)
	}
	projectedInput, err := BuildArchitectureCanvasInput(projectedData)
	if err != nil {
		t.Fatalf("projected BuildArchitectureCanvasInput: %v", err)
	}
	if !reflect.DeepEqual(projectedInput, legacyInput) {
		t.Fatalf("Architecture input changed:\nlegacy: %#v\nnew:    %#v", legacyInput, projectedInput)
	}
	legacyCanvas, err := ProjectArchitectureCanvas(legacyInput)
	if err != nil {
		t.Fatalf("legacy ProjectArchitectureCanvas: %v", err)
	}
	projectedCanvas, err := ProjectArchitectureCanvas(projectedInput)
	if err != nil {
		t.Fatalf("projected ProjectArchitectureCanvas: %v", err)
	}
	if !reflect.DeepEqual(projectedCanvas, legacyCanvas) {
		t.Fatalf("Architecture canvas changed:\nlegacy: %#v\nnew:    %#v", legacyCanvas, projectedCanvas)
	}

	legacyData.ArchitectureCanvas = &legacyCanvas
	projectedData.ArchitectureCanvas = &projectedCanvas
	legacyTourData := guidedTourReportFixture()
	projectedTourData := guidedTourReportFixture()
	legacyTourData.RepositoryGraph.PackageEdges = append(
		[]EdgeInfo(nil),
		legacy.PackageEdges...,
	)
	projectedTourData.RepositoryGraph.PackageEdges = append(
		[]EdgeInfo(nil),
		projected.PackageEdges...,
	)
	legacyTour, err := BuildGuidedTourBundle(legacyTourData)
	if err != nil {
		t.Fatalf("legacy BuildGuidedTourBundle: %v", err)
	}
	projectedTour, err := BuildGuidedTourBundle(projectedTourData)
	if err != nil {
		t.Fatalf("projected BuildGuidedTourBundle: %v", err)
	}
	if !reflect.DeepEqual(projectedTour, legacyTour) ||
		string(mustJSON(t, projectedTour)) != string(mustJSON(t, legacyTour)) {
		t.Fatalf("guided tour changed:\nlegacy: %#v\nnew:    %#v", legacyTour, projectedTour)
	}

	legacySearch, err := BuildSemanticSearchIndex(legacyData)
	if err != nil {
		t.Fatalf("legacy BuildSemanticSearchIndex: %v", err)
	}
	projectedSearch, err := BuildSemanticSearchIndex(projectedData)
	if err != nil {
		t.Fatalf("projected BuildSemanticSearchIndex: %v", err)
	}
	if !reflect.DeepEqual(projectedSearch, legacySearch) ||
		string(mustJSON(t, projectedSearch)) != string(mustJSON(t, legacySearch)) {
		t.Fatalf("semantic search changed:\nlegacy: %#v\nnew:    %#v", legacySearch, projectedSearch)
	}

	if CurrentFormatVersion != 48 ||
		SemanticSearchIndexVersion != 6 ||
		CurrentRunManifestVersion != 18 {
		t.Fatalf(
			"wire versions changed: report=%d search=%d manifest=%d",
			CurrentFormatVersion,
			SemanticSearchIndexVersion,
			CurrentRunManifestVersion,
		)
	}
}

func TestWorkspacePackageEdgeProjectionIgnoresLegacyNilAndEmptySeed(t *testing.T) {
	facts := reportRootFacts()
	allowed := []string{
		"cmd/app/main.go",
		"internal/core/core.go",
		"tools/cmd/tool/main.go",
	}
	fixture := parseLegacyGraphFixture(t, facts, allowed, nil, nil)
	snapshot := reportGraphSnapshot(
		t,
		"/workspacegraph-report-edge-shape",
		"/workspacegraph-report-edge-shape",
		allowed,
	)
	graph, err := workspacegraph.New(workspacegraph.Input{
		Snapshot: snapshot,
		GoFacts:  facts,
	})
	if err != nil {
		t.Fatalf("workspacegraph.New: %v", err)
	}

	tests := []struct {
		name  string
		edges []EdgeInfo
	}{
		{name: "nil", edges: nil},
		{name: "non-nil empty", edges: []EdgeInfo{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			legacy := cloneRepositoryGraph(fixture.RepositoryGraph)
			legacy.PackageEdges = test.edges
			cloned := cloneRepositoryGraph(legacy)
			if (cloned.PackageEdges == nil) != (test.edges == nil) {
				t.Fatalf("clone PackageEdges = %#v, want shape %#v", cloned.PackageEdges, test.edges)
			}
			projected, err := projectWorkspacePackageGraph(legacy, facts, graph)
			if err != nil {
				t.Fatalf("projectWorkspacePackageGraph: %v", err)
			}
			want := reportEdgesFromWorkspaceGraph(graph)
			if !reflect.DeepEqual(projected.PackageEdges, want) {
				t.Fatalf("projected PackageEdges = %#v, want exact %#v", projected.PackageEdges, want)
			}
		})
	}
}

func TestWorkspacePackageEdgeSelectionPreservesAuthorizedSelfEdge(t *testing.T) {
	facts := reportRootFacts()
	extraGraphEdge := EdgeInfo{
		From: "example.com/repo/tools/cmd/tool",
		To:   "example.com/repo/internal/core",
	}
	self := EdgeInfo{
		From: "example.com/repo/internal/core",
		To:   "example.com/repo/internal/core",
	}
	facts.InternalEdges = append(facts.InternalEdges, gofacts.Edge{
		From: self.From,
		To:   self.To,
	})
	allowed := []string{
		"cmd/app/main.go",
		"internal/core/core.go",
		"tools/cmd/tool/main.go",
	}
	legacy := parseLegacyGraphFixture(t, facts, allowed, []EdgeInfo{
		self,
		{From: "example.com/repo/cmd/app", To: "example.com/repo/internal/core"},
		self,
	}, nil).RepositoryGraph
	graph, err := workspacegraph.New(workspacegraph.Input{
		Snapshot: reportGraphSnapshot(
			t,
			"/workspacegraph-report-self-edge",
			"/workspacegraph-report-self-edge",
			allowed,
		),
		GoFacts: facts,
	})
	if err != nil {
		t.Fatalf("workspacegraph.New: %v", err)
	}
	if _, ok := graph.Edge(extraGraphEdge.From, extraGraphEdge.To); !ok {
		t.Fatal("strict-subset fixture is missing its unselected authorized edge")
	}
	projected, err := projectWorkspacePackageGraph(legacy, facts, graph)
	if err != nil {
		t.Fatalf("projectWorkspacePackageGraph: %v", err)
	}
	want := reportEdgesFromWorkspaceGraph(graph)
	if !reflect.DeepEqual(projected.PackageEdges, want) {
		t.Fatalf("exact self edge set = %#v, want %#v", projected.PackageEdges, want)
	}
}

func TestWorkspacePackageSelectionPreservesOrderDuplicatesFilesAndEditorialFields(
	t *testing.T,
) {
	baseFacts := reportRootFacts()
	facts := baseFacts
	facts.Packages = []gofacts.PackageFact{
		baseFacts.Packages[2],
		baseFacts.Packages[0],
		baseFacts.Packages[1],
		baseFacts.Packages[0],
	}
	allowed := []string{
		"cmd/app/main.go",
		"internal/core/core.go",
		"tools/cmd/tool/main.go",
	}
	legacy := parseLegacyGraphFixture(t, facts, allowed, nil, nil).RepositoryGraph
	for index := range legacy.Packages {
		legacy.Packages[index].DisplayPath = fmt.Sprintf("editorial/%d", index)
		legacy.Packages[index].Locality = fmt.Sprintf("locality-%d", index)
	}
	graph, err := workspacegraph.New(workspacegraph.Input{
		Snapshot: reportGraphSnapshot(
			t,
			"/workspacegraph-report-package-order",
			"/workspacegraph-report-package-order",
			allowed,
		),
		GoFacts: facts,
	})
	if err != nil {
		t.Fatalf("workspacegraph.New: %v", err)
	}

	projected, err := projectWorkspacePackageGraph(legacy, facts, graph)
	if err != nil {
		t.Fatalf("projectWorkspacePackageGraph: %v", err)
	}
	want := cloneRepositoryGraph(legacy)
	want.PackageEdges = reportEdgesFromWorkspaceGraph(graph)
	if !reflect.DeepEqual(projected, want) ||
		string(mustJSON(t, projected)) != string(mustJSON(t, want)) {
		t.Fatalf("package projection changed graph: %#v", projected)
	}
	if projected.Packages[1].CanonicalPath !=
		projected.Packages[3].CanonicalPath ||
		projected.Packages[1].DisplayPath ==
			projected.Packages[3].DisplayPath ||
		projected.Packages[0].CanonicalPath !=
			"example.com/repo/tools/cmd/tool" {
		t.Fatalf(
			"package order, duplicates, or editorial fields changed: %#v",
			projected.Packages,
		)
	}
}

func TestWorkspacePackageSelectionPreservesNilAndEmptyShape(t *testing.T) {
	const repositoryRoot = "/workspacegraph-report-package-shape"
	graph, err := workspacegraph.New(workspacegraph.Input{
		Snapshot: reportGraphSnapshot(t, repositoryRoot, repositoryRoot, nil),
		GoFacts:  gofacts.Facts{},
	})
	if err != nil {
		t.Fatalf("workspacegraph.New: %v", err)
	}
	tests := []struct {
		name     string
		packages []PackageInfo
	}{
		{name: "nil", packages: nil},
		{name: "non-nil empty", packages: []PackageInfo{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			legacy := &RepositoryGraph{Version: 2, Packages: test.packages}
			before := mustJSON(t, legacy)
			projected, err := projectWorkspacePackageGraph(
				legacy,
				gofacts.Facts{},
				graph,
			)
			if err != nil {
				t.Fatalf("projectWorkspacePackageGraph: %v", err)
			}
			if (projected.Packages == nil) != (test.packages == nil) ||
				!reflect.DeepEqual(projected, legacy) ||
				string(mustJSON(t, projected)) != string(before) {
				t.Fatalf("package shape projection changed graph: %#v", projected)
			}
		})
	}
}

func TestWorkspacePackageSelectionPreflightsBeforeGraphProjection(t *testing.T) {
	if maxReportGraphFactPackages != workspacepackageselection.MaxRows {
		t.Fatalf(
			"report/selection row budgets differ: %d != %d",
			maxReportGraphFactPackages,
			workspacepackageselection.MaxRows,
		)
	}
	oversized := strings.Repeat(
		"x",
		workspacepackageselection.MaxScalarBytes+1,
	)
	legacy := &RepositoryGraph{
		Modules: []ModuleInfo{{
			ID: "legacy-id", Path: "example.com/legacy", Dir: "",
		}},
		Packages: []PackageInfo{{
			CanonicalPath:     "example.com/repo/internal/core",
			Name:              oversized,
			ModuleID:          "root-id",
			ModulePath:        "example.com/repo",
			Dir:               "internal/core",
			ModuleRelativeDir: "internal/core",
		}},
	}
	facts := gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "fact-id", ModulePath: "example.com/repo", ModuleDir: ".",
		}},
		Packages: []gofacts.PackageFact{{
			CanonicalPath:     "example.com/repo/internal/core",
			Name:              "core",
			ModuleID:          "root-id",
			ModulePath:        "example.com/repo",
			PackageDir:        "internal/core",
			ModuleRelativeDir: "internal/core",
		}},
	}
	_, err := projectWorkspacePackageGraph(legacy, facts, workspacegraph.Graph{})
	if err == nil || err.Error() != "workspace graph: package projection is unavailable" {
		t.Fatalf("projection error = %v, want package preflight before module lookup", err)
	}
	if strings.Contains(err.Error(), oversized[:64]) {
		t.Fatalf("projection error exposed oversized scalar: %v", err)
	}
}

func TestWorkspacePackageEdgeSelectionPreflightsBeforeGraphProjection(t *testing.T) {
	oversized := strings.Repeat("x", maxReportGraphScalarBytes+1)
	legacy := &RepositoryGraph{
		Modules: []ModuleInfo{{
			ID: "legacy-id", Path: "example.com/legacy", Dir: "",
		}},
	}
	facts := gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "fact-id", ModulePath: "example.com/repo", ModuleDir: ".",
		}},
		InternalEdges: []gofacts.Edge{{
			From: oversized,
			To:   "example.com/repo/internal/core",
		}},
	}
	_, err := projectWorkspacePackageGraph(legacy, facts, workspacegraph.Graph{})
	if err == nil || err.Error() != "workspace graph: edge projection is unavailable" {
		t.Fatalf("projection error = %v, want edge preflight before module lookup", err)
	}
	if strings.Contains(err.Error(), oversized[:64]) {
		t.Fatalf("projection error exposed oversized endpoint: %v", err)
	}
}

func TestWorkspacePackageExactEdgeBudgetIsTransactional(t *testing.T) {
	const (
		fromPackage = "example.com/repo/a"
		toPackage   = "example.com/repo/b"
	)
	facts := gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "root-id", ModulePath: "example.com/repo", ModuleDir: ".",
		}},
		Packages: []gofacts.PackageFact{
			{
				CanonicalPath: fromPackage, Name: "a",
				ModuleID: "root-id", ModulePath: "example.com/repo",
				PackageDir: "a", ModuleRelativeDir: "a",
			},
			{
				CanonicalPath: toPackage, Name: "b",
				ModuleID: "root-id", ModulePath: "example.com/repo",
				PackageDir: "b", ModuleRelativeDir: "b",
			},
		},
		InternalEdges: []gofacts.Edge{{From: fromPackage, To: toPackage}},
	}
	original := &RepositoryGraph{
		Version: 2,
		Modules: []ModuleInfo{{
			ID: "root-id", Path: "example.com/repo", Dir: "",
		}},
		Packages: []PackageInfo{
			{
				CanonicalPath: fromPackage, Name: "a",
				ModuleID: "root-id", ModulePath: "example.com/repo",
				Dir: "a", ModuleRelativeDir: "a",
			},
			{
				CanonicalPath: toPackage, Name: "b",
				ModuleID: "root-id", ModulePath: "example.com/repo",
				Dir: "b", ModuleRelativeDir: "b",
			},
		},
	}
	const repositoryRoot = "/workspacegraph-report-edge-budget"
	snapshot := reportGraphSnapshot(t, repositoryRoot, repositoryRoot, nil)
	graph, err := workspacegraph.New(workspacegraph.Input{
		Snapshot: snapshot,
		GoFacts:  facts,
	})
	if err != nil {
		t.Fatalf("workspacegraph.New: %v", err)
	}
	exactlyAtLimit := facts
	exactlyAtLimit.InternalEdges = make([]gofacts.Edge, maxReportGraphFactEdges)
	for index := range exactlyAtLimit.InternalEdges {
		exactlyAtLimit.InternalEdges[index] = facts.InternalEdges[0]
	}
	projected, err := projectWorkspacePackageGraph(original, exactlyAtLimit, graph)
	if err != nil {
		t.Fatalf("projectWorkspacePackageGraph at exact edge bound: %v", err)
	}
	if got := projected.PackageEdges; !reflect.DeepEqual(got, []EdgeInfo{{
		From: fromPackage,
		To:   toPackage,
	}}) {
		t.Fatalf("deduplicated exact edge set = %#v", got)
	}

	overLimit := exactlyAtLimit
	overLimit.InternalEdges = append(
		append([]gofacts.Edge(nil), exactlyAtLimit.InternalEdges...),
		facts.InternalEdges[0],
	)
	before := mustJSON(t, original)
	if _, err := projectWorkspacePackageGraph(original, overLimit, graph); err == nil {
		t.Fatal("projectWorkspacePackageGraph unexpectedly accepted N+1 exact edges")
	} else if strings.Contains(err.Error(), fromPackage) ||
		strings.Contains(err.Error(), toPackage) ||
		strings.Contains(err.Error(), repositoryRoot) {
		t.Fatalf("projection error exposed caller scalar or absolute root: %v", err)
	}
	if string(mustJSON(t, original)) != string(before) {
		t.Fatal("N+1 projection failure mutated the original graph")
	}
}

func TestAttachAuthorizedWorkspacePackageGraphIsTransactional(t *testing.T) {
	facts := reportRootFacts()
	allowed := []string{
		"cmd/app/main.go",
		"internal/core/core.go",
		"tools/cmd/tool/main.go",
	}
	selected := []EdgeInfo{{
		From: "example.com/repo/cmd/app",
		To:   "example.com/repo/internal/core",
	}}
	legacyData := parseLegacyGraphFixture(t, facts, allowed, selected, nil)
	authority := reportGraphAuthority(
		t,
		"/workspacegraph-report-transaction",
		"/workspacegraph-report-transaction",
		allowed,
	)

	t.Run("construction failure retains original pointer and bytes", func(t *testing.T) {
		invalid := facts
		invalid.Modules = append([]gofacts.ModuleFact(nil), facts.Modules...)
		invalid.Modules[0].ModulePath = "../private-module"
		original := cloneRepositoryGraph(legacyData.RepositoryGraph)
		data := &ReportData{
			RepositoryGraph:   original,
			OpenablePaths:     append([]string(nil), allowed...),
			repositoryGoFacts: &invalid,
		}
		before := mustJSON(t, data.RepositoryGraph)
		attachAuthorizedWorkspacePackageGraph(data, &authority)
		if data.RepositoryGraph != original ||
			string(mustJSON(t, data.RepositoryGraph)) != string(before) ||
			!reflect.DeepEqual(data.Warnings, []string{workspaceGraphUnavailableWarning}) {
			t.Fatalf("construction failure did not remain explicit and transactional: graph=%#v warnings=%#v", data.RepositoryGraph, data.Warnings)
		}
	})

	t.Run("stale legacy edge seed is ignored", func(t *testing.T) {
		original := cloneRepositoryGraph(legacyData.RepositoryGraph)
		original.PackageEdges = []EdgeInfo{{
			From: "example.com/repo/cmd/app",
			To:   "example.com/repo/not-collected",
		}}
		data := &ReportData{
			RepositoryGraph:   original,
			OpenablePaths:     append([]string(nil), allowed...),
			repositoryGoFacts: &facts,
		}
		attachAuthorizedWorkspacePackageGraph(data, &authority)
		if data.RepositoryGraph == original {
			t.Fatal("successful attachment retained original pointer")
		}
		want := cloneRepositoryGraph(original)
		want.PackageEdges = exactReportEdges(t, facts, authority)
		if !reflect.DeepEqual(data.RepositoryGraph, want) || len(data.Warnings) != 0 {
			t.Fatalf("stale legacy edge seed affected exact graph: graph=%#v warnings=%#v", data.RepositoryGraph, data.Warnings)
		}
	})

	t.Run("package identity failure retains pointer bytes and warnings", func(t *testing.T) {
		original := cloneRepositoryGraph(legacyData.RepositoryGraph)
		original.Packages[0].Name = "private_conflicting_name"
		warnings := []string{"existing warning"}
		data := &ReportData{
			RepositoryGraph:   original,
			OpenablePaths:     append([]string(nil), allowed...),
			Warnings:          append([]string(nil), warnings...),
			repositoryGoFacts: &facts,
		}
		before := mustJSON(t, data.RepositoryGraph)
		attachAuthorizedWorkspacePackageGraph(data, &authority)
		wantWarnings := append(append([]string(nil), warnings...), workspaceGraphUnavailableWarning)
		if data.RepositoryGraph != original ||
			string(mustJSON(t, data.RepositoryGraph)) != string(before) ||
			!reflect.DeepEqual(data.Warnings, wantWarnings) {
			t.Fatalf("package identity failure changed pointer, bytes, or warnings")
		}
	})

	t.Run("package file failure retains pointer bytes and warnings", func(t *testing.T) {
		original := cloneRepositoryGraph(legacyData.RepositoryGraph)
		original.Packages[0].Files = []string{"cmd/app/private.go"}
		warnings := []string{"existing warning"}
		data := &ReportData{
			RepositoryGraph:   original,
			OpenablePaths:     append([]string(nil), allowed...),
			Warnings:          append([]string(nil), warnings...),
			repositoryGoFacts: &facts,
		}
		before := mustJSON(t, data.RepositoryGraph)
		attachAuthorizedWorkspacePackageGraph(data, &authority)
		wantWarnings := append(append([]string(nil), warnings...), workspaceGraphUnavailableWarning)
		if data.RepositoryGraph != original ||
			string(mustJSON(t, data.RepositoryGraph)) != string(before) ||
			!reflect.DeepEqual(data.Warnings, wantWarnings) {
			t.Fatalf("package file failure changed pointer, bytes, or warnings")
		}
	})

	t.Run("success attaches complete equal replacement", func(t *testing.T) {
		original := cloneRepositoryGraph(legacyData.RepositoryGraph)
		data := &ReportData{
			RepositoryGraph:   original,
			OpenablePaths:     append([]string(nil), allowed...),
			repositoryGoFacts: &facts,
		}
		attachAuthorizedWorkspacePackageGraph(data, &authority)
		if data.RepositoryGraph == original {
			t.Fatal("successful attachment retained original pointer")
		}
		want := cloneRepositoryGraph(original)
		want.PackageEdges = exactReportEdges(t, facts, authority)
		if !reflect.DeepEqual(data.RepositoryGraph, want) ||
			string(mustJSON(t, data.RepositoryGraph)) != string(mustJSON(t, want)) {
			t.Fatalf("successful attachment did not materialize exact graph: %#v", data.RepositoryGraph)
		}
	})
}

func TestAuthorizedReadRunDirUsesExactGraphWithoutLegacyEdgeSeed(t *testing.T) {
	facts := reportRootFacts()
	allowed := []string{
		"cmd/app/main.go",
		"internal/core/core.go",
		"tools/cmd/tool/main.go",
	}
	selected := []EdgeInfo{
		{From: "example.com/repo/tools/cmd/tool", To: "example.com/repo/internal/core"},
		{From: "example.com/repo/cmd/app", To: "example.com/repo/internal/core"},
	}
	dir := t.TempDir()
	mkdirAll(t, filepath.Join(dir, "flows"))
	writeTestFile(t, dir, "snapshot.json", string(mustJSON(t, map[string]any{
		"repo_name": "fixture",
		"go_facts":  facts,
	})))
	writeTestFile(t, dir, "llm_bundle.json", string(mustJSON(t, map[string]any{
		"allowed_paths": allowed,
		"go": map[string]any{
			"important_edges": selected,
		},
	})))

	legacy, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("legacy ReadRunDir: %v", err)
	}
	if legacy.repositoryGoFacts != nil {
		t.Fatal("plain ReadRunDir retained neutral-only exact facts")
	}
	authority := reportGraphAuthority(
		t,
		"/workspacegraph-report-read-run-dir",
		"/workspacegraph-report-read-run-dir",
		allowed,
	)
	adapted, err := readRunDir(dir, authority.analysisRoot, &authority, nil)
	if err != nil {
		t.Fatalf("authorized readRunDir: %v", err)
	}
	if adapted.repositoryGoFacts == nil {
		t.Fatal("authorized readRunDir did not retain exact facts for attachment")
	}
	wantGraph := cloneRepositoryGraph(legacy.RepositoryGraph)
	wantGraph.PackageEdges = exactReportEdges(t, facts, authority)
	if !reflect.DeepEqual(adapted.RepositoryGraph, wantGraph) {
		t.Fatalf(
			"authorized graph differs:\nwant: %#v\nnew:  %#v",
			wantGraph,
			adapted.RepositoryGraph,
		)
	}
	legacy.RepositoryGraph = wantGraph
	if got, want := string(mustJSON(t, adapted)), string(mustJSON(t, legacy)); got != want {
		t.Fatalf("authorized report bytes differ beyond exact graph:\nwant: %s\nnew:  %s", want, got)
	}
}

func TestMalformedNewExactFieldsKeepLegacySnapshotProjection(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "snapshot.json", `{
		"repo_name":"fixture",
		"go_facts":{
			"modules":[{
				"id":"root-id",
				"module_path":"example.com/repo",
				"module_dir":".",
				"display_name":".",
				"main":"not-a-boolean"
			}],
			"packages":[],
			"internal_edges":"not-an-edge-array"
		}
	}`)
	data := &ReportData{}
	if warning := parseSnapshotWithExactFacts(
		filepath.Join(dir, "snapshot.json"),
		data,
		true,
	); warning != "" {
		t.Fatalf("legacy parse changed: %s", warning)
	}
	if data.RepositoryGraph == nil || len(data.RepositoryGraph.Modules) != 1 {
		t.Fatalf("legacy graph missing: %#v", data.RepositoryGraph)
	}
	if data.repositoryGoFacts != nil {
		t.Fatal("malformed exact extension was retained")
	}
	original := data.RepositoryGraph
	before := mustJSON(t, original)
	authority := reportGraphAuthority(
		t,
		"/workspacegraph-report-malformed",
		"/workspacegraph-report-malformed",
		nil,
	)
	attachAuthorizedWorkspacePackageGraph(data, &authority)
	if data.RepositoryGraph != original ||
		string(mustJSON(t, data.RepositoryGraph)) != string(before) ||
		!reflect.DeepEqual(data.Warnings, []string{workspaceGraphUnavailableWarning}) {
		t.Fatalf("malformed exact extension changed legacy graph: %#v", data.RepositoryGraph)
	}
}

func TestSnapshotExactGoFactsPreflightRejectsOversizedEdgeBeforeCapture(t *testing.T) {
	oversized := strings.Repeat("x", 2*1024*1024)
	dir := t.TempDir()
	writeTestFile(t, dir, "snapshot.json", `{
		"repo_name":"fixture",
		"go_facts":{
			"modules":[{
				"id":"root-id",
				"module_path":"example.com/repo",
				"module_dir":".",
				"display_name":".",
				"main":true
			}],
			"packages":[{
				"canonical_package_path":"example.com/repo/internal/core",
				"name":"core",
				"owning_module_id":"root-id",
				"module_path":"example.com/repo",
				"package_directory":"internal/core",
				"module_relative_path":"internal/core",
				"display_path":"internal/core",
				"locality":"local",
				"files":["internal/core/core.go"]
			}],
			"internal_edges":[{
				"from":"`+oversized+`",
				"to":"example.com/repo/internal/core"
			}]
		}
	}`)

	data := &ReportData{}
	if warning := parseSnapshotWithExactFacts(
		filepath.Join(dir, "snapshot.json"),
		data,
		true,
	); warning != "" {
		t.Fatalf("legacy parse changed: %s", warning)
	}
	if data.repositoryGoFacts != nil {
		t.Fatal("oversized exact facts were retained")
	}
	if data.RepositoryGraph == nil || len(data.RepositoryGraph.Packages) != 1 {
		t.Fatalf("legacy graph missing: %#v", data.RepositoryGraph)
	}
	original := data.RepositoryGraph
	before := mustJSON(t, original)
	authority := reportGraphAuthority(
		t,
		"/workspacegraph-report-oversized",
		"/workspacegraph-report-oversized",
		[]string{"internal/core/core.go"},
	)
	attachAuthorizedWorkspacePackageGraph(data, &authority)
	after := mustJSON(t, data.RepositoryGraph)
	if data.RepositoryGraph != original || string(after) != string(before) ||
		!reflect.DeepEqual(data.Warnings, []string{workspaceGraphUnavailableWarning}) {
		t.Fatalf("oversized exact facts changed legacy graph: %s", after)
	}
	if strings.Contains(string(after), oversized[:maxReportGraphScalarBytes+1]) ||
		strings.Contains(string(after), authority.analysisRoot) {
		t.Fatalf("unsafe scalar or absolute root reached encoded graph: %s", after)
	}
}

func TestSnapshotExactGoFactsPreflightRejectsCaseInsensitiveKnownKeyAliases(t *testing.T) {
	const canonical = `{
		"repo_name":"fixture",
		"go_facts":{
			"modules":[{
				"id":"root-id",
				"module_path":"example.com/repo",
				"module_dir":".",
				"display_name":".",
				"main":true
			}],
			"packages":[{
				"canonical_package_path":"example.com/repo",
				"name":"repo",
				"owning_module_id":"root-id",
				"module_path":"example.com/repo",
				"package_directory":".",
				"module_relative_path":".",
				"display_path":"repo",
				"locality":"local",
				"files":["main.go"]
			}],
			"internal_edges":[{
				"from":"example.com/repo",
				"to":"example.com/repo"
			}]
		}
	}`
	edge := `{"from":"example.com/repo","to":"example.com/repo"}`
	oversizedEdges := strings.Repeat(edge+",", maxReportGraphFactEdges) + edge

	tests := []struct {
		name     string
		snapshot string
	}{
		{
			name:     "GO_FACTS",
			snapshot: strings.Replace(canonical, `"go_facts"`, `"GO_FACTS"`, 1),
		},
		{
			name:     "MODULES",
			snapshot: strings.Replace(canonical, `"modules"`, `"MODULES"`, 1),
		},
		{
			name:     "PACKAGES",
			snapshot: strings.Replace(canonical, `"packages"`, `"PACKAGES"`, 1),
		},
		{
			name: "INTERNAL_EDGES beyond collection limit",
			snapshot: strings.Replace(
				canonical,
				`"internal_edges":[{`+
					"\n\t\t\t\t"+`"from":"example.com/repo",`+
					"\n\t\t\t\t"+`"to":"example.com/repo"`+
					"\n\t\t\t"+`}]`,
				`"INTERNAL_EDGES":[`+oversizedEdges+`]`,
				1,
			),
		},
		{
			name:     "FILES",
			snapshot: strings.Replace(canonical, `"files"`, `"FILES"`, 1),
		},
		{
			name:     "MODULE_PATH",
			snapshot: strings.Replace(canonical, `"module_path"`, `"MODULE_PATH"`, 1),
		},
		{
			name:     "FROM",
			snapshot: strings.Replace(canonical, `"from"`, `"FROM"`, 1),
		},
		{
			name:     "TO",
			snapshot: strings.Replace(canonical, `"to"`, `"TO"`, 1),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertSnapshotExactGoFactsAliasRejected(t, test.snapshot)
		})
	}
}

func TestSnapshotExactGoFactsPreflightRejectsEscapedKnownKeyAliases(t *testing.T) {
	const canonical = `{
		"repo_name":"fixture",
		"go_facts":{
			"modules":[{
				"id":"root-id",
				"module_path":"example.com/repo",
				"module_dir":".",
				"display_name":".",
				"main":true
			}],
			"packages":[{
				"canonical_package_path":"example.com/repo",
				"name":"repo",
				"owning_module_id":"root-id",
				"module_path":"example.com/repo",
				"package_directory":".",
				"module_relative_path":".",
				"display_path":"repo",
				"locality":"local",
				"files":["main.go"]
			}],
			"internal_edges":[{
				"from":"example.com/repo",
				"to":"example.com/repo"
			}]
		}
	}`
	tests := []struct {
		name     string
		snapshot string
	}{
		{
			name:     `go\u005ffacts`,
			snapshot: strings.Replace(canonical, `"go_facts"`, `"go\u005ffacts"`, 1),
		},
		{
			name:     `internal\u005fedges`,
			snapshot: strings.Replace(canonical, `"internal_edges"`, `"internal\u005fedges"`, 1),
		},
		{
			name:     `module\u005fpath`,
			snapshot: strings.Replace(canonical, `"module_path"`, `"module\u005fpath"`, 1),
		},
		{
			name:     `f\u0069les`,
			snapshot: strings.Replace(canonical, `"files"`, `"f\u0069les"`, 1),
		},
		{
			name:     `fr\u006fm`,
			snapshot: strings.Replace(canonical, `"from"`, `"fr\u006fm"`, 1),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertSnapshotExactGoFactsAliasRejected(t, test.snapshot)
		})
	}
}

func assertSnapshotExactGoFactsAliasRejected(t *testing.T, snapshot string) {
	t.Helper()
	input := []byte(snapshot)
	if _, err := preflightSnapshotExactGoFacts(input); !errors.Is(
		err,
		errReportGraphJSONUnavailable,
	) {
		t.Fatalf("preflight error = %v, want unavailable", err)
	}
	facts, err := decodeSnapshotExactGoFacts(input)
	if !errors.Is(err, errReportGraphJSONUnavailable) {
		t.Fatalf("decode error = %v, want unavailable", err)
	}
	if !reflect.DeepEqual(facts, gofacts.Facts{}) {
		t.Fatalf("decode returned facts: %#v", facts)
	}

	dir := t.TempDir()
	writeTestFile(t, dir, "snapshot.json", snapshot)
	data := &ReportData{}
	if warning := parseSnapshotWithExactFacts(
		filepath.Join(dir, "snapshot.json"),
		data,
		true,
	); warning != "" {
		t.Fatalf("legacy parse changed: %s", warning)
	}
	if data.repositoryGoFacts != nil {
		t.Fatal("alias exact facts were retained")
	}
	if data.RepositoryGraph == nil ||
		len(data.RepositoryGraph.Modules) == 0 ||
		len(data.RepositoryGraph.Packages) == 0 {
		t.Fatalf("legacy graph missing: %#v", data.RepositoryGraph)
	}
	original := data.RepositoryGraph
	before := mustJSON(t, original)
	attachAuthorizedWorkspacePackageGraph(data, &RunAuthority{})
	if data.RepositoryGraph != original ||
		string(mustJSON(t, data.RepositoryGraph)) != string(before) ||
		!reflect.DeepEqual(data.Warnings, []string{workspaceGraphUnavailableWarning}) {
		t.Fatalf("alias changed legacy graph: %#v", data.RepositoryGraph)
	}
}

func TestSnapshotExactGoFactsPreflightStopsBeforeExcessEdge(t *testing.T) {
	var input strings.Builder
	input.WriteString(`{"go_facts":{"modules":[],"packages":[],"internal_edges":[`)
	for index := 0; index < maxReportGraphFactEdges; index++ {
		if index > 0 {
			input.WriteByte(',')
		}
		input.WriteString(`{"from":"a","to":"b"}`)
	}
	// The element beyond the fixed edge budget is intentionally incomplete.
	// A bounded preflight must reject before scanning or decoding it.
	input.WriteString(`,{"from":"`)
	input.WriteString(strings.Repeat("x", 2*1024*1024))

	_, err := preflightSnapshotExactGoFacts([]byte(input.String()))
	if !errors.Is(err, errReportGraphJSONBounds) {
		t.Fatalf("preflight error = %v, want bounds", err)
	}
}

func BenchmarkSnapshotExactGoFactsPreflightOversizedScalar(b *testing.B) {
	input := []byte(
		`{"go_facts":{"modules":[],"packages":[],"internal_edges":[{"from":"` +
			strings.Repeat("x", 2*1024*1024),
	)
	b.ReportAllocs()
	b.SetBytes(maxReportGraphScalarBytes + 1)
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		_, _ = preflightSnapshotExactGoFacts(input)
	}
}

func parseLegacyGraphFixture(
	t *testing.T,
	facts gofacts.Facts,
	allowedPaths []string,
	selectedEdges []EdgeInfo,
	moduleSummaries []map[string]any,
) *ReportData {
	t.Helper()
	dir := t.TempDir()
	snapshotJSON := mustJSON(t, map[string]any{
		"repo_name": "fixture",
		"go_facts":  facts,
	})
	writeTestFile(t, dir, "snapshot.json", string(snapshotJSON))
	bundleJSON := mustJSON(t, map[string]any{
		"allowed_paths": allowedPaths,
		"go": map[string]any{
			"module_summaries": moduleSummaries,
			"important_edges":  selectedEdges,
		},
	})
	writeTestFile(t, dir, "llm_bundle.json", string(bundleJSON))

	data := &ReportData{}
	if warning := parseSnapshotWithExactFacts(
		filepath.Join(dir, "snapshot.json"),
		data,
		true,
	); warning != "" {
		t.Fatalf("parseSnapshot: %s", warning)
	}
	if warning := parseLLMBundle(filepath.Join(dir, "llm_bundle.json"), data); warning != "" {
		t.Fatalf("parseLLMBundle: %s", warning)
	}
	return data
}

func reportRootFacts() gofacts.Facts {
	return gofacts.Facts{
		Modules: []gofacts.ModuleFact{
			{
				ID: "root-id", ModulePath: "example.com/repo", ModuleDir: ".",
				Main: true, GoMod: "go.mod", DisplayName: ".",
			},
			{
				ID: "tools-id", ModulePath: "example.com/repo/tools", ModuleDir: "tools",
				Main: true, GoMod: "tools/go.mod", DisplayName: "tools",
			},
		},
		Packages: []gofacts.PackageFact{
			{
				CanonicalPath: "example.com/repo/cmd/app", Name: "main",
				ModuleID: "root-id", ModulePath: "example.com/repo",
				PackageDir: "cmd/app", ModuleRelativeDir: "cmd/app",
				DisplayPath: "cmd/app", Locality: "local",
				Files: []string{"cmd/app/main.go"},
			},
			{
				CanonicalPath: "example.com/repo/internal/core", Name: "core",
				ModuleID: "root-id", ModulePath: "example.com/repo",
				PackageDir: "internal/core", ModuleRelativeDir: "internal/core",
				DisplayPath: "internal/core", Locality: "local",
				Files: []string{"internal/core/core.go"},
			},
			{
				CanonicalPath: "example.com/repo/tools/cmd/tool", Name: "main",
				ModuleID: "tools-id", ModulePath: "example.com/repo/tools",
				PackageDir: "tools/cmd/tool", ModuleRelativeDir: "cmd/tool",
				DisplayPath: "cmd/tool", Locality: "local",
				Files: []string{"tools/cmd/tool/main.go"},
			},
		},
		InternalEdges: []gofacts.Edge{
			{From: "example.com/repo/cmd/app", To: "example.com/repo/internal/core"},
			{From: "example.com/repo/tools/cmd/tool", To: "example.com/repo/internal/core"},
		},
	}
}

func reportSubdirectoryFacts() gofacts.Facts {
	return gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "service-id", ModulePath: "example.com/service", ModuleDir: ".",
			Main: true, GoMod: "go.mod", DisplayName: ".",
		}},
		Packages: []gofacts.PackageFact{
			{
				CanonicalPath: "example.com/service/cmd/app", Name: "main",
				ModuleID: "service-id", ModulePath: "example.com/service",
				PackageDir: "cmd/app", ModuleRelativeDir: "cmd/app",
				DisplayPath: "cmd/app", Locality: "local",
				Files: []string{"cmd/app/main.go"},
			},
			{
				CanonicalPath: "example.com/service/internal/core", Name: "core",
				ModuleID: "service-id", ModulePath: "example.com/service",
				PackageDir: "internal/core", ModuleRelativeDir: "internal/core",
				DisplayPath: "internal/core", Locality: "local",
				Files: []string{"internal/core/core.go"},
			},
		},
		InternalEdges: []gofacts.Edge{{
			From: "example.com/service/cmd/app",
			To:   "example.com/service/internal/core",
		}},
	}
}

func casdoorShapedGraphFacts(edgeCount int) gofacts.Facts {
	const modulePath = "github.com/casdoor/casdoor"
	facts := gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "root-id", ModulePath: modulePath, ModuleDir: ".", Main: true,
		}},
		Packages:      make([]gofacts.PackageFact, edgeCount+1),
		InternalEdges: make([]gofacts.Edge, edgeCount),
	}
	for index := range facts.Packages {
		directory := fmt.Sprintf("pkg%03d", index)
		canonicalPath := modulePath + "/" + directory
		facts.Packages[index] = gofacts.PackageFact{
			CanonicalPath:     canonicalPath,
			Name:              directory,
			ModuleID:          "root-id",
			ModulePath:        modulePath,
			PackageDir:        directory,
			ModuleRelativeDir: directory,
		}
		if index > 0 {
			facts.InternalEdges[index-1] = gofacts.Edge{
				From: facts.Packages[index-1].CanonicalPath,
				To:   canonicalPath,
			}
		}
	}
	return facts
}

func reportEdgesFromWorkspaceGraph(graph workspacegraph.Graph) []EdgeInfo {
	edges := graph.Edges()
	if edges == nil {
		return nil
	}
	result := make([]EdgeInfo, len(edges))
	for index, edge := range edges {
		result[index] = EdgeInfo{From: edge.FromPackage, To: edge.ToPackage}
	}
	return result
}

func exactReportEdges(
	t *testing.T,
	facts gofacts.Facts,
	authority RunAuthority,
) []EdgeInfo {
	t.Helper()
	snapshot, err := workspacesnapshot.New(workspacesnapshot.Input{
		AnalysisRoot:   authority.analysisRoot,
		Repository:     authority.repository,
		CapturedInputs: authority.inputs,
	})
	if err != nil {
		t.Fatalf("workspacesnapshot.New: %v", err)
	}
	graph, err := workspacegraph.New(workspacegraph.Input{Snapshot: snapshot, GoFacts: facts})
	if err != nil {
		t.Fatalf("workspacegraph.New: %v", err)
	}
	return reportEdgesFromWorkspaceGraph(graph)
}

func reportGraphSnapshot(
	t *testing.T,
	repositoryRoot,
	analysisRoot string,
	allowedPaths []string,
) workspacesnapshot.Snapshot {
	t.Helper()
	authority := reportGraphAuthority(t, repositoryRoot, analysisRoot, allowedPaths)
	snapshot, err := workspacesnapshot.New(workspacesnapshot.Input{
		AnalysisRoot:   authority.analysisRoot,
		Repository:     authority.repository,
		CapturedInputs: authority.inputs,
		AllowedPaths:   allowedPaths,
	})
	if err != nil {
		t.Fatalf("workspacesnapshot.New: %v", err)
	}
	return snapshot
}

func reportGraphAuthority(
	t *testing.T,
	repositoryRoot,
	analysisRoot string,
	allowedPaths []string,
) RunAuthority {
	t.Helper()
	repositoryRoot = filepath.Clean(repositoryRoot)
	analysisRoot = filepath.Clean(analysisRoot)
	analysisRelative, err := filepath.Rel(repositoryRoot, analysisRoot)
	if err != nil {
		t.Fatal(err)
	}
	analysisPrefix := ""
	if analysisRelative != "." {
		analysisPrefix = filepath.ToSlash(analysisRelative)
	}
	inputs := make([]freshness.CapturedInput, 0, len(allowedPaths))
	for _, allowedPath := range allowedPaths {
		repositoryPath := allowedPath
		if analysisPrefix != "" {
			repositoryPath = path.Join(analysisPrefix, allowedPath)
		}
		id := sha256.Sum256([]byte("id:" + repositoryPath))
		content := sha256.Sum256([]byte("content:" + repositoryPath))
		inputs = append(inputs, freshness.CapturedInput{
			Version:       freshness.CapturedInputVersion,
			ID:            fmt.Sprintf("%x", id),
			Path:          repositoryPath,
			Kind:          freshness.FileRegular,
			Mode:          "100644",
			ContentSHA256: fmt.Sprintf("%x", content),
			Stages:        []string{"workspace_graph_report_test"},
		})
	}
	return RunAuthority{
		analysisRoot: analysisRoot,
		repository: freshness.RepositoryState{
			Version:  freshness.RepositoryStateVersion,
			Identity: repositoryRoot,
			Head:     strings.Repeat("a", 40),
			Dirty:    []freshness.DirtyFile{},
		},
		inputs:    inputs,
		freshness: freshness.NewFreshnessResult(freshness.FreshnessFresh),
		confirmed: true,
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
