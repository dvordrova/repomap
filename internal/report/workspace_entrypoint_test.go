package report

import (
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/workspaceentrypoint"
	"github.com/dvordrova/repomap/internal/workspacegraph"
)

func TestWorkspaceEntrypointProjectionPreservesLegacyBytesAndConsumers(t *testing.T) {
	index := reportEntrypointIndex(t)
	legacy := reportEntrypointSurfaces()
	before := mustJSON(t, legacy)

	projected, err := projectWorkspaceEntrypoints(legacy, index)
	if err != nil {
		t.Fatalf("projectWorkspaceEntrypoints: %v", err)
	}
	if projected == legacy {
		t.Fatal("successful projection retained the legacy pointer")
	}
	if !reflect.DeepEqual(projected, legacy) {
		t.Fatalf("projection changed surfaces:\nlegacy: %#v\nnew:    %#v", legacy, projected)
	}
	if got := string(mustJSON(t, projected)); got != string(before) {
		t.Fatalf("projection changed serialized bytes:\nlegacy: %s\nnew:    %s", before, got)
	}
	if len(projected.Triggers) != 3 ||
		projected.Triggers[0].ID != "surface-worker" ||
		projected.Triggers[1].ID != "process-first" ||
		projected.Triggers[2].ID != "process-duplicate" ||
		projected.Triggers[1].ProcessEntrypoint.ID != projected.Triggers[2].ProcessEntrypoint.ID ||
		projected.TotalCount != 9 ||
		!projected.Truncated {
		t.Fatalf("order, duplicate identity, selection, or truncation changed: %#v", projected)
	}
	if projected.Triggers[1].Identity.Path.Candidates != nil ||
		projected.Triggers[2].Identity.Path.Candidates == nil {
		t.Fatalf(
			"nil/empty distinction changed: %#v / %#v",
			projected.Triggers[1].Identity.Path.Candidates,
			projected.Triggers[2].Identity.Path.Candidates,
		)
	}
	if len(projected.EntrypointsConsidered) != 1 ||
		projected.EntrypointsConsidered[0].Location == nil ||
		projected.EntrypointsConsidered[0].Location.Column != 7 {
		t.Fatalf("SSA entrypoint subset changed: %#v", projected.EntrypointsConsidered)
	}

	legacyData := &ReportData{
		RepoName:           "fixture",
		DiscoveredSurfaces: legacy,
		RepositoryGraph: &RepositoryGraph{
			Version: 2,
			Modules: []ModuleInfo{{
				ID: "root-id", Path: "example.com/repo", Dir: "",
			}},
			Packages: []PackageInfo{{
				CanonicalPath: "example.com/repo/cmd/app", Name: "main",
				ModuleID: "root-id", ModulePath: "example.com/repo",
				Dir: "cmd/app", ModuleRelativeDir: "cmd/app",
				DisplayPath: "cmd/app", Locality: "local",
				Files: []string{"cmd/app/main.go"},
			}},
		},
		Components:          []Component{{ID: "component-app", Name: "App"}},
		ArchitectureCanvas:  &ArchitectureCanvas{Version: ArchitectureCanvasVersion},
		SemanticSearch:      &SemanticSearchIndex{Version: SemanticSearchIndexVersion},
		Run:                 &RunInfo{PromptVersion: "saved-run"},
		OpenablePaths:       []string{"cmd/app/main.go"},
		CandidateFlows:      []string{},
		Flows:               []FlowData{},
		ComponentRelations:  []ComponentRelation{},
		CandidateDirections: []CandidateDirection{},
	}
	projectedData := *legacyData
	projectedData.DiscoveredSurfaces = projected
	legacyArchitecture, err := BuildArchitectureCanvasInput(legacyData)
	if err != nil {
		t.Fatalf("legacy BuildArchitectureCanvasInput: %v", err)
	}
	projectedArchitecture, err := BuildArchitectureCanvasInput(&projectedData)
	if err != nil {
		t.Fatalf("projected BuildArchitectureCanvasInput: %v", err)
	}
	if !reflect.DeepEqual(projectedArchitecture, legacyArchitecture) ||
		string(mustJSON(t, projectedArchitecture)) != string(mustJSON(t, legacyArchitecture)) {
		t.Fatal("Architecture input or deterministic candidate bundle changed")
	}
	legacySearch, err := BuildSemanticSearchIndex(legacyData)
	if err != nil {
		t.Fatalf("legacy BuildSemanticSearchIndex: %v", err)
	}
	projectedSearch, err := BuildSemanticSearchIndex(&projectedData)
	if err != nil {
		t.Fatalf("projected BuildSemanticSearchIndex: %v", err)
	}
	if !reflect.DeepEqual(projectedSearch, legacySearch) ||
		string(mustJSON(t, projectedSearch)) != string(mustJSON(t, legacySearch)) {
		t.Fatal("semantic Search projection changed")
	}
	if CurrentFormatVersion != 30 ||
		SemanticSearchIndexVersion != 6 ||
		CurrentRunManifestVersion != 11 {
		t.Fatalf(
			"wire versions changed: report=%d search=%d manifest=%d",
			CurrentFormatVersion,
			SemanticSearchIndexVersion,
			CurrentRunManifestVersion,
		)
	}

	// The successful replacement owns every nested mutable value.
	projected.Triggers[0].Middleware[0].Candidates[0] = "mutated"
	projected.Triggers[1].ProcessEntrypoint.Location.Path = "mutated.go"
	projected.EntrypointsConsidered[0].Location.Column = 99
	projected.UnavailablePackages[0].DiagnosticIDs[0] = "mutated"
	if legacy.Triggers[0].Middleware[0].Candidates[0] != "worker-candidate" ||
		legacy.Triggers[1].ProcessEntrypoint.Location.Path != "cmd/app/main.go" ||
		legacy.EntrypointsConsidered[0].Location.Column != 7 ||
		legacy.UnavailablePackages[0].DiagnosticIDs[0] != "diagnostic-1" {
		t.Fatal("successful clone shares mutable state with legacy surfaces")
	}
}

func TestWorkspaceEntrypointProjectionRejectsMismatchTransactionally(t *testing.T) {
	index := reportEntrypointIndex(t)
	tests := []struct {
		name   string
		mutate func(*DiscoveredTrigger)
	}{
		{
			name: "symbol id",
			mutate: func(trigger *DiscoveredTrigger) {
				trigger.ProcessEntrypoint.ID = "wrong.main"
			},
		},
		{
			name: "symbol package",
			mutate: func(trigger *DiscoveredTrigger) {
				trigger.ProcessEntrypoint.Package = "example.com/repo/other"
			},
		},
		{
			name: "symbol name",
			mutate: func(trigger *DiscoveredTrigger) {
				trigger.ProcessEntrypoint.Name = "notMain"
			},
		},
		{
			name: "location column",
			mutate: func(trigger *DiscoveredTrigger) {
				trigger.ProcessEntrypoint.Location.Column = 1
			},
		},
		{
			name: "identity",
			mutate: func(trigger *DiscoveredTrigger) {
				trigger.Identity.Path.Text = "cmd/app/other.go"
			},
		},
		{
			name: "registration",
			mutate: func(trigger *DiscoveredTrigger) {
				trigger.RegistrationSite.Line++
			},
		},
		{
			name: "handler",
			mutate: func(trigger *DiscoveredTrigger) {
				trigger.Handler.Text = "wrong.main"
			},
		},
		{
			name: "evidence",
			mutate: func(trigger *DiscoveredTrigger) {
				trigger.Evidence = nil
			},
		},
		{
			name: "provenance",
			mutate: func(trigger *DiscoveredTrigger) {
				trigger.Provenance = nil
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			legacy := reportEntrypointSurfaces()
			test.mutate(&legacy.Triggers[1])
			before := mustJSON(t, legacy)
			data := &ReportData{DiscoveredSurfaces: legacy}
			err := attachWorkspaceEntrypointIndex(data, index)
			if err == nil {
				t.Fatal("attachWorkspaceEntrypointIndex unexpectedly succeeded")
			}
			if data.DiscoveredSurfaces != legacy ||
				string(mustJSON(t, data.DiscoveredSurfaces)) != string(before) {
				t.Fatalf("failed adapter mutated surfaces: %#v", data.DiscoveredSurfaces)
			}
		})
	}
}

func TestAttachAuthorizedWorkspaceEntrypointIndexIsTransactional(t *testing.T) {
	graphFacts := reportRootFacts()
	entrypointFacts := reportRootEntrypointFacts()
	allowed := []string{
		"cmd/app/main.go",
		"internal/core/core.go",
		"tools/cmd/tool/main.go",
	}
	authority := reportGraphAuthority(
		t,
		"/workspace-entrypoint-report-transaction",
		"/workspace-entrypoint-report-transaction",
		allowed,
	)

	t.Run("construction failure", func(t *testing.T) {
		invalid := entrypointFacts
		invalid.EntrypointPackages = append(
			[]gofacts.Entrypoint(nil),
			entrypointFacts.EntrypointPackages...,
		)
		invalid.EntrypointPackages[0].ModulePath = strings.Repeat(
			"x",
			workspaceentrypoint.MaxScalarBytes+1,
		)
		original := reportEntrypointSurfaces()
		before := mustJSON(t, original)
		data := &ReportData{
			DiscoveredSurfaces:        original,
			OpenablePaths:             append([]string(nil), allowed...),
			repositoryGoFacts:         &graphFacts,
			repositoryEntrypointFacts: &invalid,
			Warnings:                  []string{"legacy warning"},
		}
		attachAuthorizedWorkspaceEntrypointIndex(data, &authority)
		if data.DiscoveredSurfaces != original ||
			string(mustJSON(t, data.DiscoveredSurfaces)) != string(before) ||
			!reflect.DeepEqual(data.Warnings, []string{"legacy warning"}) {
			t.Fatalf("construction failure mutated report: %#v", data)
		}
	})

	t.Run("adapter failure after valid prefix", func(t *testing.T) {
		original := reportEntrypointSurfaces()
		original.Triggers[2].Handler.Text = "wrong.main"
		before := mustJSON(t, original)
		data := &ReportData{
			DiscoveredSurfaces:        original,
			OpenablePaths:             append([]string(nil), allowed...),
			repositoryGoFacts:         &graphFacts,
			repositoryEntrypointFacts: &entrypointFacts,
		}
		attachAuthorizedWorkspaceEntrypointIndex(data, &authority)
		if data.DiscoveredSurfaces != original ||
			string(mustJSON(t, data.DiscoveredSurfaces)) != string(before) {
			t.Fatalf("adapter failure partially mutated report: %#v", data.DiscoveredSurfaces)
		}
	})

	t.Run("success replaces with equal complete clone", func(t *testing.T) {
		original := reportEntrypointSurfaces()
		before := mustJSON(t, original)
		data := &ReportData{
			DiscoveredSurfaces:        original,
			OpenablePaths:             append([]string(nil), allowed...),
			repositoryGoFacts:         &graphFacts,
			repositoryEntrypointFacts: &entrypointFacts,
		}
		attachAuthorizedWorkspaceEntrypointIndex(data, &authority)
		if data.DiscoveredSurfaces == original {
			t.Fatal("successful attachment retained original pointer")
		}
		if !reflect.DeepEqual(data.DiscoveredSurfaces, original) ||
			string(mustJSON(t, data.DiscoveredSurfaces)) != string(before) {
			t.Fatalf("successful attachment changed report: %#v", data.DiscoveredSurfaces)
		}
	})
}

func TestAuthorizedReadRunDirUsesEntrypointIndexWithoutChangingReportBytes(t *testing.T) {
	graphFacts := reportRootFacts()
	entrypointFacts := reportRootEntrypointFacts()
	graphFacts.EntrypointPackages = entrypointFacts.EntrypointPackages
	allowed := []string{
		"cmd/app/main.go",
		"internal/core/core.go",
		"tools/cmd/tool/main.go",
	}
	dir := t.TempDir()
	mkdirAll(t, dir+"/flows")
	writeTestFile(t, dir, "snapshot.json", string(mustJSON(t, map[string]any{
		"repo_name": "fixture",
		"go_facts":  graphFacts,
	})))
	writeTestFile(t, dir, "llm_bundle.json", string(mustJSON(t, map[string]any{
		"allowed_paths": allowed,
		"go": map[string]any{
			"important_edges": []EdgeInfo{{
				From: "example.com/repo/cmd/app",
				To:   "example.com/repo/internal/core",
			}},
		},
	})))
	writeEntrypointSurfaceArtifacts(t, dir)

	legacy, err := ReadRunDir(dir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	if legacy.repositoryGoFacts != nil || legacy.repositoryEntrypointFacts != nil {
		t.Fatal("plain ReadRunDir retained neutral-only exact facts")
	}
	authority := reportGraphAuthority(
		t,
		"/workspace-entrypoint-report-read-run-dir",
		"/workspace-entrypoint-report-read-run-dir",
		allowed,
	)
	adapted, err := readRunDir(dir, authority.analysisRoot, &authority, nil)
	if err != nil {
		t.Fatalf("authorized readRunDir: %v", err)
	}
	if adapted.repositoryGoFacts == nil || adapted.repositoryEntrypointFacts == nil {
		t.Fatal("authorized read did not capture both independent exact fact sets")
	}
	legacyComparable := *legacy
	adaptedComparable := *adapted
	adaptedComparable.repositoryGoFacts = nil
	adaptedComparable.repositoryEntrypointFacts = nil
	adaptedComparable.studyDocumentSourceRoot = legacyComparable.studyDocumentSourceRoot
	if !reflect.DeepEqual(&adaptedComparable, &legacyComparable) {
		t.Fatalf(
			"authorized public report state changed:\nlegacy: %#v\nnew:    %#v",
			&legacyComparable,
			&adaptedComparable,
		)
	}
	if got, want := string(mustJSON(t, adapted)), string(mustJSON(t, legacy)); got != want {
		t.Fatalf("authorized report bytes changed:\nlegacy: %s\nnew:    %s", want, got)
	}
	if adapted.DiscoveredSurfaces == nil ||
		len(adapted.DiscoveredSurfaces.Triggers) != 1 ||
		adapted.DiscoveredSurfaces.Triggers[0].ProcessEntrypoint.ID !=
			"example.com/repo/cmd/app.main" {
		t.Fatalf("entrypoint surface missing: %#v", adapted.DiscoveredSurfaces)
	}
}

func reportEntrypointIndex(t *testing.T) workspaceentrypoint.Index {
	t.Helper()
	graphFacts := reportRootFacts()
	snapshot := reportGraphSnapshot(
		t,
		"/workspace-entrypoint-report-index",
		"/workspace-entrypoint-report-index",
		[]string{
			"cmd/app/main.go",
			"internal/core/core.go",
			"tools/cmd/tool/main.go",
		},
	)
	graph, err := workspacegraph.New(workspacegraph.Input{
		Snapshot: snapshot,
		GoFacts:  graphFacts,
	})
	if err != nil {
		t.Fatalf("workspacegraph.New: %v", err)
	}
	index, err := workspaceentrypoint.New(workspaceentrypoint.Input{
		GoFacts: reportRootEntrypointFacts(),
		Graph:   graph,
	})
	if err != nil {
		t.Fatalf("workspaceentrypoint.New: %v", err)
	}
	return index
}

func reportRootEntrypointFacts() gofacts.Facts {
	return gofacts.Facts{EntrypointPackages: []gofacts.Entrypoint{
		{
			ModulePath: "example.com/repo", ImportPath: "example.com/repo/cmd/app",
			PackageDir: "cmd/app", ModuleRelativeDir: "cmd/app", ModuleDir: ".",
			Kind: "primary_binary", GoFiles: []string{"main.go"},
			Anchors: []gofacts.EntrypointAnchor{{
				Version: gofacts.EntrypointAnchorVersion, Kind: gofacts.EntrypointAnchorGoMain,
				Path: "cmd/app/main.go", Line: 9,
			}},
		},
		{
			ModulePath: "example.com/repo/tools",
			ImportPath: "example.com/repo/tools/cmd/tool",
			PackageDir: "tools/cmd/tool", ModuleRelativeDir: "cmd/tool", ModuleDir: "tools",
			Kind: "tool", GoFiles: []string{"main.go"},
			Anchors: []gofacts.EntrypointAnchor{{
				Version: gofacts.EntrypointAnchorVersion, Kind: gofacts.EntrypointAnchorGoMain,
				Path: "tools/cmd/tool/main.go", Line: 5,
			}},
		},
	}}
}

func reportEntrypointSurfaces() *DiscoveredSurfaces {
	first := exactReportProcessTrigger(
		"process-first",
		"example.com/repo/cmd/app",
		"cmd/app/main.go",
		9,
	)
	first.Identity.Path.Candidates = nil
	first.Middleware = nil
	second := exactReportProcessTrigger(
		"process-duplicate",
		"example.com/repo/cmd/app",
		"cmd/app/main.go",
		9,
	)
	second.Identity.Path.Candidates = []string{}
	second.Middleware = []SurfaceValue{}
	return &DiscoveredSurfaces{
		Version:           surfaceArtifactVersion,
		AnalyzerVersion:   "surface-ssa-v6",
		ScenarioID:        "go:test/test",
		ScopeStatement:    "saved scope",
		TotalCount:        9,
		Truncated:         true,
		ProcessEntryCount: 2,
		EntrypointsConsidered: []SurfaceSymbol{{
			ID: "example.com/repo/cmd/app.main", Package: "example.com/repo/cmd/app",
			Name: "main", Location: &SurfaceLocation{Path: "cmd/app/main.go", Line: 9, Column: 7},
		}},
		ConfiguredSeedsMatched: []string{},
		Triggers: []DiscoveredTrigger{
			{
				ID: "surface-worker", Kind: "worker",
				Middleware: []SurfaceValue{{
					Kind: "function", Known: true, Candidates: []string{"worker-candidate"},
				}},
				Evidence: []SurfaceEvidence{},
			},
			first,
			second,
		},
		LoopSignals:         []SurfaceLoopSignal{},
		DynamicFrontiers:    nil,
		UnsupportedDispatch: []SurfaceFrontier{},
		PackageDiagnostics:  nil,
		UnavailablePackages: []SurfacePackageAvailability{{
			DiagnosticIDs: []string{"diagnostic-1"},
		}},
		BudgetsReached: []string{},
	}
}

func exactReportProcessTrigger(
	id,
	packagePath,
	filePath string,
	line int,
) DiscoveredTrigger {
	symbolID := packagePath + ".main"
	location := &SurfaceLocation{Path: filePath, Line: line}
	return DiscoveredTrigger{
		ID: id, Kind: "process_entry", Transport: "process", Framework: "go",
		Identity: SurfaceIdentity{
			Name: "main",
			Path: SurfaceValue{
				Kind: "declaration", Text: filePath, Known: true, Candidates: []string{},
			},
		},
		ProcessEntrypoint: SurfaceSymbol{
			ID: symbolID, Package: packagePath, Name: "main",
			Location: &SurfaceLocation{Path: filePath, Line: line},
		},
		RegistrationSite: location,
		Handler: SurfaceValue{
			Kind: "declaration", Text: symbolID, Known: true, Candidates: []string{},
		},
		Evidence: []SurfaceEvidence{{
			ID:       "process-entry:" + filePath + ":" + integerString(line) + ":0",
			Kind:     "process_entry_declaration",
			Location: &SurfaceLocation{Path: filePath, Line: line},
			Detail:   "exact declaration",
		}},
		Provenance: []SurfaceProvenance{{
			Provider: "gofacts", Version: "entrypoint-anchor-v1",
			Operation: "build_selected_main_declaration",
		}},
		ExecutableRole:    ExecutableRoleSecondaryService,
		Availability:      SurfaceAvailabilityAvailable,
		OwningComponentID: componentmap.ComponentID("component-app"),
	}
}

func integerString(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [32]byte
	position := len(digits)
	for value > 0 {
		position--
		digits[position] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[position:])
}

func writeEntrypointSurfaceArtifacts(t *testing.T, dir string) {
	t.Helper()
	repository := rawSurfaceRepository{
		Root: "/saved/repository", ModulePath: "example.com/repo",
	}
	scenario := rawSurfaceScenario{
		ID: "go:test/test", GOOS: "test", GOARCH: "test", Tags: []string{},
	}
	trigger := rawSurfaceTrigger{
		ID: "process-app", Kind: "process_entry", ScenarioID: scenario.ID,
		Identity: rawSurfaceIdentity{
			Name: "main",
			Path: rawSurfaceValue{
				Kind: "declaration", Text: "cmd/app/main.go", Known: true,
				Candidates: []string{},
			},
		},
		Transport: "process", Framework: "go",
		ProcessEntrypoint: rawSurfaceSymbol{
			ID: "example.com/repo/cmd/app.main", Package: "example.com/repo/cmd/app",
			Name: "main", Location: rawSurfaceLocation{Path: "cmd/app/main.go", Line: 9},
		},
		RegistrationSite: rawSurfaceLocation{Path: "cmd/app/main.go", Line: 9},
		Handler: rawSurfaceValue{
			Kind: "declaration", Text: "example.com/repo/cmd/app.main",
			Known: true, Candidates: []string{},
		},
		Evidence: []rawSurfaceEvidence{{
			ID: "process-entry:cmd/app/main.go:9:0", Kind: "process_entry_declaration",
			Location: rawSurfaceLocation{Path: "cmd/app/main.go", Line: 9},
		}},
		Provenance: []rawSurfaceProvenance{{
			Provider: "gofacts", Version: "entrypoint-anchor-v1",
			Operation: "build_selected_main_declaration",
		}},
		Availability: SurfaceAvailabilityAvailable,
	}
	writeTestFile(t, dir, surfaceCatalogFilename, string(mustJSON(t, rawSurfaceCatalog{
		Version: surfaceArtifactVersion, AnalyzerVersion: "surface-ssa-v6",
		CatalogVersion: surfaceSemanticCatalogVersion,
		Repository:     repository, Scenario: scenario,
		Triggers: []rawSurfaceTrigger{trigger},
	})))
	writeTestFile(t, dir, surfaceCoverageFilename, string(mustJSON(t, rawSurfaceCoverage{
		Version: surfaceArtifactVersion, Repository: repository, Scenario: scenario,
		ProcessEntries: 1, AvailableProcessEntries: 1,
		EntrypointsConsidered: []rawSurfaceSymbol{},
		ScopeStatement:        "saved scope",
	})))
}
