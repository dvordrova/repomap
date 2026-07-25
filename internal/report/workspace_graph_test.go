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
	"github.com/dvordrova/repomap/internal/workspacesnapshot"
)

func TestWorkspacePackageGraphProjectionPreservesLegacyBytes(t *testing.T) {
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
			legacyJSON := mustJSON(t, legacy)

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
			if !reflect.DeepEqual(projected, legacy) {
				t.Fatalf("projection changed graph:\nlegacy: %#v\nnew:    %#v", legacy, projected)
			}
			if got := mustJSON(t, projected); string(got) != string(legacyJSON) {
				t.Fatalf("projection changed serialized bytes:\nlegacy: %s\nnew:    %s", legacyJSON, got)
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
				if len(projected.PackageEdges) != 3 ||
					projected.PackageEdges[1] != projected.PackageEdges[2] {
					t.Fatalf("selected edge order/duplicates changed: %#v", projected.PackageEdges)
				}
			}
		})
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

	if CurrentFormatVersion != 26 ||
		SemanticSearchIndexVersion != 5 ||
		CurrentRunManifestVersion != 4 {
		t.Fatalf(
			"wire versions changed: report=%d search=%d manifest=%d",
			CurrentFormatVersion,
			SemanticSearchIndexVersion,
			CurrentRunManifestVersion,
		)
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
			string(mustJSON(t, data.RepositoryGraph)) != string(before) {
			t.Fatalf("construction failure mutated graph: %#v", data.RepositoryGraph)
		}
	})

	t.Run("adapter failure retains original pointer and bytes", func(t *testing.T) {
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
		before := mustJSON(t, data.RepositoryGraph)
		attachAuthorizedWorkspacePackageGraph(data, &authority)
		if data.RepositoryGraph != original ||
			string(mustJSON(t, data.RepositoryGraph)) != string(before) {
			t.Fatalf("adapter failure mutated graph: %#v", data.RepositoryGraph)
		}
	})

	t.Run("success attaches complete equal replacement", func(t *testing.T) {
		original := cloneRepositoryGraph(legacyData.RepositoryGraph)
		data := &ReportData{
			RepositoryGraph:   original,
			OpenablePaths:     append([]string(nil), allowed...),
			repositoryGoFacts: &facts,
		}
		before := mustJSON(t, data.RepositoryGraph)
		attachAuthorizedWorkspacePackageGraph(data, &authority)
		if data.RepositoryGraph == original {
			t.Fatal("successful attachment retained original pointer")
		}
		if !reflect.DeepEqual(data.RepositoryGraph, original) ||
			string(mustJSON(t, data.RepositoryGraph)) != string(before) {
			t.Fatalf("successful attachment changed graph: %#v", data.RepositoryGraph)
		}
	})
}

func TestAuthorizedReadRunDirUsesGraphWithoutChangingReportBytes(t *testing.T) {
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
	adapted, err := readRunDir(dir, authority.analysisRoot, &authority)
	if err != nil {
		t.Fatalf("authorized readRunDir: %v", err)
	}
	if adapted.repositoryGoFacts == nil {
		t.Fatal("authorized readRunDir did not retain exact facts for attachment")
	}
	if !reflect.DeepEqual(adapted.RepositoryGraph, legacy.RepositoryGraph) {
		t.Fatalf(
			"authorized graph changed:\nlegacy: %#v\nnew:    %#v",
			legacy.RepositoryGraph,
			adapted.RepositoryGraph,
		)
	}
	if got, want := string(mustJSON(t, adapted)), string(mustJSON(t, legacy)); got != want {
		t.Fatalf("authorized report bytes changed:\nlegacy: %s\nnew:    %s", want, got)
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
		string(mustJSON(t, data.RepositoryGraph)) != string(before) {
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
	if data.RepositoryGraph != original || string(after) != string(before) {
		t.Fatalf("oversized exact facts changed legacy graph: %s", after)
	}
	if strings.Contains(string(after), oversized[:maxReportGraphScalarBytes+1]) ||
		strings.Contains(string(after), authority.analysisRoot) {
		t.Fatalf("unsafe scalar or absolute root reached encoded graph: %s", after)
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
