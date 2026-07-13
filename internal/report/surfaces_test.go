package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseDiscoveredSurfacesProjectsPairedV2Artifacts(t *testing.T) {
	t.Parallel()

	surfaces, warnings := parseDiscoveredSurfaces(surfaceFixtureDir())
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	if surfaces == nil {
		t.Fatal("surfaces = nil, want the valid paired projection")
	}
	if surfaces.Version != 2 || surfaces.AnalyzerVersion != "surface-ssa-v2" ||
		surfaces.ScenarioID != "go:darwin/amd64:tags=integration" {
		t.Fatalf("surface identity = %#v", surfaces)
	}
	if surfaces.TotalCount != 2 || surfaces.Truncated || len(surfaces.Triggers) != 2 {
		t.Fatalf("trigger bounds = total %d truncated %v triggers %d", surfaces.TotalCount, surfaces.Truncated, len(surfaces.Triggers))
	}
	if surfaces.Triggers[0].ID != "trigger-a-health" || surfaces.Triggers[1].ID != "trigger-z-worker" {
		t.Fatalf("trigger order = %q, %q", surfaces.Triggers[0].ID, surfaces.Triggers[1].ID)
	}
	if surfaces.DirectCount != 1 || surfaces.WrapperCount != 1 ||
		surfaces.WorkerCount != 1 || surfaces.AsyncTaskCount != 0 ||
		surfaces.HTTPRouteCount != 1 || surfaces.DynamicFrontierCount != 1 {
		t.Fatalf("coverage counts = %#v", surfaces)
	}

	httpTrigger := surfaces.Triggers[0]
	if len(httpTrigger.Middleware) != 1 || len(httpTrigger.WrapperChain) != 1 ||
		len(httpTrigger.DynamicFrontier) != 1 || len(httpTrigger.Evidence) != 1 {
		t.Fatalf("http semantic roles were flattened: %#v", httpTrigger)
	}
	if len(surfaces.LoopSignals) != 1 || len(surfaces.DynamicFrontiers) != 1 ||
		len(surfaces.UnsupportedDispatch) != 1 {
		t.Fatalf("coverage evidence was flattened: %#v", surfaces)
	}
	if surfaces.UnsupportedDispatch[0].Location != nil {
		t.Fatalf("absolute unsupported-dispatch location survived: %#v", surfaces.UnsupportedDispatch[0])
	}
	if surfaces.Triggers[1].Evidence[1].Location != nil {
		t.Fatalf("absolute trigger evidence location survived: %#v", surfaces.Triggers[1].Evidence[1])
	}

	encoded, err := json.Marshal(surfaces)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("/private/source/mixed")) {
		t.Fatalf("presentation projection leaked the absolute repository root: %s", encoded)
	}

	var paths []string
	collectDiscoveredSurfacePaths(surfaces, func(value string) {
		paths = append(paths, value)
	})
	wantPaths := []string{
		"cmd/server/main.go",
		"internal/http/registry.go",
		"internal/http/routes.go",
		"internal/worker/queue.go",
	}
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Fatalf("surface paths = %#v, want %#v", paths, wantPaths)
	}
}

func TestProjectDiscoveredSurfacesCollapsesRepeatedCoverageNoise(t *testing.T) {
	t.Parallel()

	frontier := rawSurfaceFrontier{
		Kind: "recursive_wrapper", Detail: "fmt.Sprintf",
		Location: &rawSurfaceLocation{Path: "cmd/example.go", Line: 42, Column: 3},
	}
	projected := projectDiscoveredSurfaces(rawSurfaceCatalog{}, rawSurfaceCoverage{
		DynamicFrontiers:    []rawSurfaceFrontier{frontier, frontier},
		UnsupportedDispatch: []rawSurfaceFrontier{frontier, frontier},
		BudgetsReached:      []string{"depth", "depth", "targets"},
	})

	if projected.DynamicFrontierCount != 1 || len(projected.DynamicFrontiers) != 1 {
		t.Fatalf("dynamic frontiers = count %d items %#v, want one unique item", projected.DynamicFrontierCount, projected.DynamicFrontiers)
	}
	if len(projected.UnsupportedDispatch) != 1 {
		t.Fatalf("unsupported dispatch = %#v, want one unique item", projected.UnsupportedDispatch)
	}
	if !reflect.DeepEqual(projected.BudgetsReached, []string{"depth", "targets"}) {
		t.Fatalf("budgets = %#v, want stable unique values", projected.BudgetsReached)
	}
}

func TestProjectDiscoveredSurfacesCountsHTTPServerRoots(t *testing.T) {
	t.Parallel()

	projected := projectDiscoveredSurfaces(rawSurfaceCatalog{Triggers: []rawSurfaceTrigger{
		{Kind: "http_route"},
		{Kind: "http_server"},
		{Kind: "http_route_descriptor"},
		{Kind: "http_route_frontier"},
	}}, rawSurfaceCoverage{})

	if projected.HTTPRouteCount != 1 || projected.HTTPServerCount != 1 ||
		projected.HTTPRouteDescriptorCount != 1 || projected.HTTPRouteFrontierCount != 1 {
		t.Fatalf("HTTP counts = %#v", projected)
	}
}

func TestProjectDiscoveredSurfacesRetainsProcessEntryAndUnavailablePackage(t *testing.T) {
	t.Parallel()

	projected := projectDiscoveredSurfaces(rawSurfaceCatalog{Triggers: []rawSurfaceTrigger{{
		ID: "process-broken", Kind: "process_entry",
		ProcessEntrypoint: rawSurfaceSymbol{
			ID: "example.com/app/cmd/broken.main", Package: "example.com/app/cmd/broken", Name: "main",
			Location: rawSurfaceLocation{Path: "cmd/broken/main.go", Line: 3},
		},
		RegistrationSite: rawSurfaceLocation{Path: "cmd/broken/main.go", Line: 3},
		Status:           "confirmed_process_entry", Certainty: "static", Resolution: "exact",
		OwningExecutable: "cmd/broken", ExecutableRole: ExecutableRoleSecondaryService,
		Availability:      SurfaceAvailabilityUnavailable,
		UnavailableReason: "package or dependency closure is ill-typed",
	}}}, rawSurfaceCoverage{
		PackageDiagnosticCount: 1, UnavailablePackageCount: 1,
		PackageDiagnostics: []rawSurfacePackageDiagnostic{{
			ID: "diagnostic-1", Kind: "type", Message: "undefined: generatedAsset",
			Package: "example.com/app/cmd/broken", PackageName: "main",
			OwningExecutable: "cmd/broken", ExecutableRole: ExecutableRoleSecondaryService,
			Availability: SurfaceAvailabilityUnavailable,
			Location:     &rawSurfaceLocation{Path: "cmd/broken/main.go", Line: 4, Column: 2},
		}},
		UnavailablePackages: []rawSurfacePackageAvailability{{
			Package: "example.com/app/cmd/broken", PackageName: "main",
			OwningExecutable: "cmd/broken", ExecutableRole: ExecutableRoleSecondaryService,
			Availability: SurfaceAvailabilityUnavailable, Reason: "package_errors",
			DiagnosticIDs: []string{"diagnostic-1"},
		}},
	})

	if projected.ProcessEntryCount != 1 || projected.UnavailableSurfaceCount != 1 ||
		projected.PackageDiagnosticCount != 1 || projected.UnavailablePackageCount != 1 {
		t.Fatalf("process/diagnostic counts = %#v", projected)
	}
	trigger := projected.Triggers[0]
	if trigger.Kind != "process_entry" || trigger.ExecutableRole != ExecutableRoleSecondaryService ||
		trigger.Availability != SurfaceAvailabilityUnavailable || trigger.ProcessEntrypoint.Location == nil {
		t.Fatalf("process entry projection = %#v", trigger)
	}
	if len(projected.PackageDiagnostics) != 1 || projected.PackageDiagnostics[0].Location == nil ||
		projected.PackageDiagnostics[0].Location.Path != "cmd/broken/main.go" ||
		len(projected.UnavailablePackages) != 1 ||
		projected.UnavailablePackages[0].OwningExecutable != "cmd/broken" {
		t.Fatalf("package failure projection = %#v / %#v", projected.PackageDiagnostics, projected.UnavailablePackages)
	}
	data := &ReportData{Run: &RunInfo{}, DiscoveredSurfaces: projected}
	refreshSurfaceCatalogCounts(projected)
	refreshProductCounts(data)
	if data.Run.SecondaryServiceSurfaceCount != 1 || data.Run.UnavailableSurfaceCount != 1 ||
		data.Run.PackageDiagnosticCount != 1 || data.Run.UnavailablePackageCount != 1 {
		t.Fatalf("run-level process/diagnostic counts = %#v", data.Run)
	}
}

func TestProjectDiscoveredSurfacesSanitizesEmbeddedExternalLocations(t *testing.T) {
	t.Parallel()

	projected := projectDiscoveredSurfaces(rawSurfaceCatalog{Triggers: []rawSurfaceTrigger{{
		ID: "external-value", Kind: "http_route",
		Dispatcher: rawSurfaceValue{
			Kind: "allocation", Text: "*echo.Echo@/Users/example/go/pkg/mod/echo.go:1:2", Known: true,
			Candidates: []string{"*echo.Echo@/Users/example/go/pkg/mod/echo.go:1:2"},
		},
	}}}, rawSurfaceCoverage{})
	encoded, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("/Users/example")) ||
		projected.Triggers[0].Dispatcher.Text != "*echo.Echo@<external>" {
		t.Fatalf("external location sanitization failed: %s", encoded)
	}
}

func TestParseDiscoveredSurfacesMissingArtifacts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		file        string
		wantWarning string
	}{
		{name: "both missing"},
		{name: "catalog only", file: surfaceCatalogFilename, wantWarning: surfaceCoverageFilename},
		{name: "coverage only", file: surfaceCoverageFilename, wantWarning: surfaceCatalogFilename},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runDir := t.TempDir()
			if test.file != "" {
				copySurfaceFixtureFile(t, runDir, test.file)
			}

			surfaces, warnings := parseDiscoveredSurfaces(runDir)
			if surfaces != nil {
				t.Fatalf("surfaces = %#v, want nil", surfaces)
			}
			if test.wantWarning == "" && len(warnings) != 0 {
				t.Fatalf("warnings = %#v, want silent legacy omission", warnings)
			}
			if test.wantWarning != "" && (len(warnings) != 1 || !strings.Contains(warnings[0], test.wantWarning)) {
				t.Fatalf("warnings = %#v, want one incomplete-pair warning containing %q", warnings, test.wantWarning)
			}
		})
	}
}

func TestParseDiscoveredSurfacesRejectsUnusablePairs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*testing.T, string)
		warning string
	}{
		{
			name: "malformed catalog",
			mutate: func(t *testing.T, runDir string) {
				writeSurfaceTestFile(t, runDir, surfaceCatalogFilename, []byte(`{"version":`))
			},
			warning: "invalid json",
		},
		{
			name: "unsupported catalog version",
			mutate: func(t *testing.T, runDir string) {
				mutateSurfaceCatalog(t, runDir, func(catalog *rawSurfaceCatalog) {
					catalog.Version = 4
				})
			},
			warning: "unsupported trigger_catalog.json version 4",
		},
		{
			name: "unsupported coverage version",
			mutate: func(t *testing.T, runDir string) {
				mutateSurfaceCoverage(t, runDir, func(coverage *rawSurfaceCoverage) {
					coverage.Version = 4
				})
			},
			warning: "unsupported surface_coverage.json version 4",
		},
		{
			name: "repository mismatch",
			mutate: func(t *testing.T, runDir string) {
				mutateSurfaceCoverage(t, runDir, func(coverage *rawSurfaceCoverage) {
					coverage.Repository.ModulePath = "example.com/other"
				})
			},
			warning: "repository identities do not match",
		},
		{
			name: "scenario mismatch",
			mutate: func(t *testing.T, runDir string) {
				mutateSurfaceCoverage(t, runDir, func(coverage *rawSurfaceCoverage) {
					coverage.Scenario.GOARCH = "arm64"
				})
			},
			warning: "scenarios do not match",
		},
		{
			name: "trigger scenario mismatch",
			mutate: func(t *testing.T, runDir string) {
				mutateSurfaceCatalog(t, runDir, func(catalog *rawSurfaceCatalog) {
					catalog.Triggers[0].ScenarioID = "go:linux/amd64:tags="
				})
			},
			warning: "trigger scenario does not match",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runDir := copySurfaceFixture(t)
			test.mutate(t, runDir)

			surfaces, warnings := parseDiscoveredSurfaces(runDir)
			if surfaces != nil {
				t.Fatalf("surfaces = %#v, want nil", surfaces)
			}
			if len(warnings) != 1 || !strings.Contains(warnings[0], test.warning) {
				t.Fatalf("warnings = %#v, want one containing %q", warnings, test.warning)
			}
		})
	}
}

func TestParseDiscoveredSurfacesSortsAndCapsStableIDs(t *testing.T) {
	t.Parallel()

	runDir := copySurfaceFixture(t)
	mutateSurfaceCatalog(t, runDir, func(catalog *rawSurfaceCatalog) {
		template := catalog.Triggers[0]
		total := maxDiscoveredSurfaceTriggers + 3
		catalog.Triggers = make([]rawSurfaceTrigger, 0, total)
		for index := range total {
			trigger := template
			trigger.ID = fmt.Sprintf("trigger-%03d", total-index-1)
			catalog.Triggers = append(catalog.Triggers, trigger)
		}
	})

	surfaces, warnings := parseDiscoveredSurfaces(runDir)
	if len(warnings) != 0 || surfaces == nil {
		t.Fatalf("surfaces = %#v, warnings = %#v", surfaces, warnings)
	}
	if surfaces.TotalCount != maxDiscoveredSurfaceTriggers+3 || !surfaces.Truncated ||
		len(surfaces.Triggers) != maxDiscoveredSurfaceTriggers {
		t.Fatalf("bounded triggers = total %d truncated %v displayed %d", surfaces.TotalCount, surfaces.Truncated, len(surfaces.Triggers))
	}
	if surfaces.Triggers[0].ID != "trigger-000" ||
		surfaces.Triggers[len(surfaces.Triggers)-1].ID != "trigger-255" {
		t.Fatalf(
			"bounded trigger range = %q ... %q",
			surfaces.Triggers[0].ID,
			surfaces.Triggers[len(surfaces.Triggers)-1].ID,
		)
	}
	beforeRoutes := surfaces.HTTPRouteCount
	surfaces.Triggers = append(surfaces.Triggers, DiscoveredTrigger{
		ID: "cobra-extra", Kind: "cli_command", Producer: SurfaceProducerCobra,
	})
	refreshSurfaceCatalogCounts(surfaces)
	if surfaces.TotalCount != maxDiscoveredSurfaceTriggers+4 || surfaces.HTTPRouteCount != beforeRoutes {
		t.Fatalf("truncated counts changed to displayed subset: %#v", surfaces)
	}
}

func TestParseDiscoveredSurfacesRejectsOversizedArtifact(t *testing.T) {
	t.Parallel()

	runDir := copySurfaceFixture(t)
	writeSurfaceTestFile(
		t,
		runDir,
		surfaceCatalogFilename,
		bytes.Repeat([]byte{' '}, maxSurfaceArtifactBytes+1),
	)

	surfaces, warnings := parseDiscoveredSurfaces(runDir)
	if surfaces != nil {
		t.Fatalf("surfaces = %#v, want nil", surfaces)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "exceeds") {
		t.Fatalf("warnings = %#v, want one size-limit warning", warnings)
	}
}

func surfaceFixtureDir() string {
	return filepath.Join("testdata", "surfaces", "mixed-v2")
}

func copySurfaceFixture(t *testing.T) string {
	t.Helper()
	runDir := t.TempDir()
	copySurfaceFixtureFile(t, runDir, surfaceCatalogFilename)
	copySurfaceFixtureFile(t, runDir, surfaceCoverageFilename)
	return runDir
}

func copySurfaceFixtureFile(t *testing.T, runDir, name string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(surfaceFixtureDir(), name))
	if err != nil {
		t.Fatal(err)
	}
	writeSurfaceTestFile(t, runDir, name, data)
}

func mutateSurfaceCatalog(t *testing.T, runDir string, mutate func(*rawSurfaceCatalog)) {
	t.Helper()
	var catalog rawSurfaceCatalog
	readSurfaceTestJSON(t, runDir, surfaceCatalogFilename, &catalog)
	mutate(&catalog)
	writeSurfaceTestJSON(t, runDir, surfaceCatalogFilename, catalog)
}

func mutateSurfaceCoverage(t *testing.T, runDir string, mutate func(*rawSurfaceCoverage)) {
	t.Helper()
	var coverage rawSurfaceCoverage
	readSurfaceTestJSON(t, runDir, surfaceCoverageFilename, &coverage)
	mutate(&coverage)
	writeSurfaceTestJSON(t, runDir, surfaceCoverageFilename, coverage)
}

func readSurfaceTestJSON(t *testing.T, runDir, name string, target any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(runDir, name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}

func writeSurfaceTestJSON(t *testing.T, runDir, name string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	writeSurfaceTestFile(t, runDir, name, data)
}

func writeSurfaceTestFile(t *testing.T, runDir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(runDir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}
