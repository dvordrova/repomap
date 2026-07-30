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

	"github.com/dvordrova/repomap/internal/gofacts"
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
	if httpTrigger.SurfaceRole != SurfaceRoleEntrySurface || httpTrigger.TraceReadiness != SurfaceTraceReady ||
		httpTrigger.TraceReadinessReason == "" || httpTrigger.Quality.Identity != surfaceQualityExact ||
		httpTrigger.Quality.Traceability != SurfaceTraceReady {
		t.Fatalf("projected route semantics = %#v", httpTrigger)
	}
	workerTrigger := surfaces.Triggers[1]
	if workerTrigger.SurfaceRole != SurfaceRoleRuntimeActivity || workerTrigger.TraceReadiness != SurfaceTraceUnsupported {
		t.Fatalf("projected worker semantics = %#v", workerTrigger)
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

func TestProjectedSurfaceSemanticsUseDynamicRoleAndLocalWrapperStart(t *testing.T) {
	t.Parallel()

	dynamicRoute := DiscoveredTrigger{
		Kind: "http_route", Availability: SurfaceAvailabilityAvailable,
		RegistrationSite: &SurfaceLocation{Path: "internal/routes.go", Line: 20},
		Identity:         SurfaceIdentity{Path: SurfaceValue{Kind: "unknown", Text: "runtime route"}},
	}
	ensureProjectedSurfaceSemantics(&dynamicRoute)
	if dynamicRoute.SurfaceRole != SurfaceRoleDynamicFrontier ||
		dynamicRoute.TraceReadiness != SurfaceTraceUnsupported {
		t.Fatalf("dynamic route semantics = %#v", dynamicRoute)
	}

	wrappedServer := DiscoveredTrigger{
		Kind: "http_server", Availability: SurfaceAvailabilityAvailable,
		WrapperChain: []SurfaceWrapper{{Callsite: &SurfaceLocation{Path: "internal/server.go", Line: 44}}},
	}
	ensureProjectedSurfaceSemantics(&wrappedServer)
	if wrappedServer.SurfaceRole != SurfaceRoleEntrySurface ||
		wrappedServer.TraceReadiness != SurfaceTracePartialReady ||
		wrappedServer.Quality.RegistrationStart != surfaceQualityPartial {
		t.Fatalf("wrapped server semantics = %#v", wrappedServer)
	}
}

func TestProjectDiscoveredSurfacesRetainsTypedCobraLocationsAndProducer(t *testing.T) {
	t.Parallel()

	projected := projectDiscoveredSurfaces(rawSurfaceCatalog{Triggers: []rawSurfaceTrigger{{
		ID: "typed-put", Kind: "cli_command", Producer: SurfaceProducerCobra,
		Identity: rawSurfaceIdentity{
			Name: "put",
			Path: rawSurfaceValue{Kind: "command_path", Text: "put", Known: true},
		},
		Transport: "cli", Framework: "cobra",
		ProcessEntrypoint: rawSurfaceSymbol{
			ID: "example.com/app.main", Package: "example.com/app", Name: "main",
			Location: rawSurfaceLocation{Path: "main.go", Line: 8},
		},
		Dispatcher: rawSurfaceValue{Kind: "function", Text: "example.com/app/cli.Start", Known: true},
		Constructor: rawSurfaceSymbol{
			ID: "example.com/app/command.NewPutCommand", Package: "example.com/app/command",
			Name: "NewPutCommand", Location: rawSurfaceLocation{Path: "command/put.go", Line: 20},
		},
		RegistrationSite: rawSurfaceLocation{Path: "cli/root.go", Line: 42},
		DescriptorSite:   &rawSurfaceLocation{Path: "command/put.go", Line: 21},
		Handler: rawSurfaceValue{
			Kind: "function", Text: "example.com/app/command.putCommandFunc", Known: true,
		},
		HandlerLocation: &rawSurfaceLocation{Path: "command/put.go", Line: 55},
		DiscoveryBasis:  "build_selected_typed_cobra_registration",
		Certainty:       "static", Resolution: "exact", Availability: SurfaceAvailabilityAvailable,
		ApplicationClass: SurfaceApplicationOwned,
		Provenance: []rawSurfaceProvenance{{
			Provider: "go_types_ssa", Operation: "discover_typed_cobra_command",
		}},
	}}}, rawSurfaceCoverage{})

	if len(projected.Triggers) != 1 {
		t.Fatalf("typed projection count = %d", len(projected.Triggers))
	}
	trigger := projected.Triggers[0]
	if trigger.Producer != SurfaceProducerCobra ||
		trigger.Constructor.Location == nil ||
		trigger.Constructor.Location.Path != "command/put.go" ||
		trigger.Constructor.Location.Line != 20 ||
		trigger.HandlerLocation == nil ||
		trigger.HandlerLocation.Path != "command/put.go" ||
		trigger.HandlerLocation.Line != 55 ||
		trigger.SurfaceRole != SurfaceRoleEntrySurface ||
		trigger.TraceReadiness != SurfaceTraceReady {
		t.Fatalf("typed Cobra projection = %#v", trigger)
	}
}

func TestMergeCommandSurfaceCatalogPrefersEquivalentTypedCobraRecord(t *testing.T) {
	t.Parallel()

	typed := DiscoveredTrigger{
		ID: "typed-backup", Kind: "cli_command", Producer: SurfaceProducerCobra,
		Identity: SurfaceIdentity{
			Name: "backup",
			Path: SurfaceValue{Kind: "command_path", Text: "backup", Known: true},
		},
		Transport: "cli", Framework: "cobra",
		ProcessEntrypoint: SurfaceSymbol{
			ID: "example.com/restic/cmd/restic.main", Package: "example.com/restic/cmd/restic",
			Name: "main", Location: &SurfaceLocation{Path: "cmd/restic/main.go", Line: 10},
		},
		Dispatcher: SurfaceValue{Kind: "function", Text: "example.com/restic/cmd/restic.Start", Known: true},
		Constructor: SurfaceSymbol{
			ID:      "example.com/restic/cmd/restic.newBackupCommand",
			Package: "example.com/restic/cmd/restic", Name: "newBackupCommand",
			Location: &SurfaceLocation{Path: "cmd/restic/cmd_backup.go", Line: 35},
		},
		RegistrationSite: &SurfaceLocation{Path: "cmd/restic/cmd_backup.go", Line: 35, Column: 4},
		Handler: SurfaceValue{
			Kind: "function", Text: "example.com/restic/cmd/restic.run-backup", Known: true,
		},
		HandlerLocation:  &SurfaceLocation{Path: "cmd/restic/cmd_backup.go", Line: 45},
		DiscoveryBasis:   "build_selected_typed_cobra_registration",
		Certainty:        "static",
		Resolution:       "exact",
		Availability:     SurfaceAvailabilityAvailable,
		ApplicationClass: SurfaceApplicationOwned,
		Provenance: []SurfaceProvenance{{
			Provider: "go_types_ssa", Operation: "discover_typed_cobra_command",
		}},
	}
	data := &ReportData{
		CommandTraces: []gofacts.CommandTrace{
			testCommandTrace("backup", "newBackupCommand", "cmd/restic/cmd_backup.go", 35),
		},
		DiscoveredSurfaces: &DiscoveredSurfaces{Triggers: []DiscoveredTrigger{typed}},
	}

	mergeCommandSurfaceCatalog(data)

	if len(data.DiscoveredSurfaces.Triggers) != 1 ||
		data.DiscoveredSurfaces.Triggers[0].ID != "typed-backup" ||
		data.DiscoveredSurfaces.CLICommandCount != 1 ||
		data.DiscoveredSurfaces.Triggers[0].TraceReadiness != SurfaceTraceReady {
		t.Fatalf("merged typed/legacy commands = %#v", data.DiscoveredSurfaces)
	}
}

func TestEquivalentCobraCommandSurfaceRequiresQualifiedOwnership(t *testing.T) {
	t.Parallel()

	typed := DiscoveredTrigger{
		Kind: "cli_command",
		Identity: SurfaceIdentity{
			Name: "lease grant",
		},
		Framework: "cobra",
		Constructor: SurfaceSymbol{
			ID:      "example.com/app/commands.newGrantCommand",
			Package: "example.com/app/commands",
			Name:    "newGrantCommand",
		},
	}
	same := typed
	same.Identity.Name = "grant"
	if !equivalentCobraCommandSurface(typed, same) {
		t.Fatal("same qualified constructor did not suppress nested typed/legacy duplicate")
	}

	other := same
	other.Constructor.ID = "example.com/other/commands.newGrantCommand"
	other.Constructor.Package = "example.com/other/commands"
	if equivalentCobraCommandSurface(typed, other) {
		t.Fatal("same constructor leaf in another package was treated as equivalent")
	}
}

func TestMergeCommandSurfaceCatalogPreservesFullCountsWhenLegacyTracesOverflow(t *testing.T) {
	t.Parallel()

	const legacyCount = 10
	genericCount := maxDiscoveredSurfaceTriggers - 6
	triggers := make([]DiscoveredTrigger, 0, genericCount)
	for index := range genericCount {
		triggers = append(triggers, DiscoveredTrigger{
			ID:       fmt.Sprintf("generic-%03d", index),
			Kind:     "worker",
			Producer: SurfaceProducerGeneric,
		})
	}
	traces := make([]gofacts.CommandTrace, 0, legacyCount)
	for index := range legacyCount {
		traces = append(traces, testCommandTrace(
			fmt.Sprintf("command-%02d", index),
			fmt.Sprintf("newCommand%02d", index),
			fmt.Sprintf("cmd/app/command_%02d.go", index),
			index+1,
		))
	}
	data := &ReportData{
		CommandTraces:      traces,
		DiscoveredSurfaces: &DiscoveredSurfaces{Triggers: triggers},
	}

	mergeCommandSurfaceCatalog(data)

	catalog := data.DiscoveredSurfaces
	wantTotal := genericCount + legacyCount
	if !catalog.Truncated ||
		len(catalog.Triggers) != maxDiscoveredSurfaceTriggers ||
		catalog.TotalCount != wantTotal ||
		catalog.CLICommandCount != legacyCount ||
		catalog.GenericSurfaceCount != genericCount {
		t.Fatalf("overflowed merged catalog = %#v", catalog)
	}
	refreshSurfaceCatalogCounts(catalog)
	if catalog.TotalCount != wantTotal ||
		catalog.CLICommandCount != legacyCount ||
		catalog.GenericSurfaceCount != genericCount {
		t.Fatalf("refresh rewrote full merged counts from retained subset: %#v", catalog)
	}
}

func TestMergeCommandSurfaceCatalogDoesNotEvictAuthoritativeFacts(t *testing.T) {
	t.Parallel()

	const legacyCount = 10
	authoritativeCount := maxDiscoveredSurfaceTriggers - 6
	triggers := make([]DiscoveredTrigger, 0, authoritativeCount)
	for index := range authoritativeCount {
		triggers = append(triggers, DiscoveredTrigger{
			ID:       fmt.Sprintf("typed-%03d", index),
			Kind:     "cli_command",
			Producer: SurfaceProducerCobra,
			Identity: SurfaceIdentity{
				Name: fmt.Sprintf("typed-%03d", index),
				Path: SurfaceValue{
					Kind:  "command_segment",
					Text:  fmt.Sprintf("typed-%03d", index),
					Known: true,
				},
			},
			Framework:      "cobra",
			DiscoveryBasis: "build_selected_typed_cobra_descriptor",
		})
	}
	traces := make([]gofacts.CommandTrace, 0, legacyCount)
	for index := range legacyCount {
		traces = append(traces, testCommandTrace(
			fmt.Sprintf("legacy-%02d", index),
			fmt.Sprintf("newLegacy%02dCommand", index),
			fmt.Sprintf("cmd/app/legacy_%02d.go", index),
			index+1,
		))
	}
	data := &ReportData{
		CommandTraces:      traces,
		DiscoveredSurfaces: &DiscoveredSurfaces{Triggers: triggers},
	}

	mergeCommandSurfaceCatalog(data)

	catalog := data.DiscoveredSurfaces
	if len(catalog.Triggers) != maxDiscoveredSurfaceTriggers ||
		catalog.TotalCount != authoritativeCount+legacyCount {
		t.Fatalf("bounded merged catalog = %#v", catalog)
	}
	seen := make(map[string]struct{}, len(catalog.Triggers))
	for _, trigger := range catalog.Triggers {
		seen[trigger.ID] = struct{}{}
	}
	for _, trigger := range triggers {
		if _, ok := seen[trigger.ID]; !ok {
			t.Fatalf("authoritative fact %q was evicted by a legacy trace", trigger.ID)
		}
	}
}

func TestMergeCommandSurfaceCatalogSkipsLegacyMergeForTruncatedTypedCatalog(t *testing.T) {
	t.Parallel()

	total := maxDiscoveredSurfaceTriggers + 20
	retained := DiscoveredTrigger{
		ID: "typed-backup", Kind: "cli_command",
		Identity: SurfaceIdentity{
			Name: "backup",
			Path: SurfaceValue{Kind: "command_path", Text: "backup", Known: true},
		},
		Framework:      "cobra",
		DiscoveryBasis: "build_selected_typed_cobra_registration",
		Provenance: []SurfaceProvenance{{
			Provider: "go_types_ssa", Operation: "discover_typed_cobra_command",
		}},
	}
	data := &ReportData{
		CommandTraces: []gofacts.CommandTrace{
			testCommandTrace("backup", "newBackupCommand", "cmd/app/backup.go", 10),
			testCommandTrace("restore", "newRestoreCommand", "cmd/app/restore.go", 20),
		},
		DiscoveredSurfaces: &DiscoveredSurfaces{
			TotalCount:      total,
			Truncated:       true,
			CLICommandCount: total,
			Triggers:        []DiscoveredTrigger{retained},
		},
	}

	mergeCommandSurfaceCatalog(data)

	catalog := data.DiscoveredSurfaces
	if len(catalog.Triggers) != 1 || catalog.Triggers[0].ID != retained.ID {
		t.Fatalf("legacy traces were merged into truncated typed catalog: %#v", catalog.Triggers)
	}
	if catalog.TotalCount != total ||
		catalog.CLICommandCount != total ||
		catalog.GenericSurfaceCount != 0 ||
		!catalog.Truncated {
		t.Fatalf("truncated typed counts changed during legacy merge: %#v", catalog)
	}
	if catalog.Triggers[0].Producer != SurfaceProducerCobra ||
		catalog.Triggers[0].Availability != SurfaceAvailabilityAvailable ||
		catalog.Triggers[0].ExecutableRole != ExecutableRoleUnknown {
		t.Fatalf("retained typed trigger was not normalized: %#v", catalog.Triggers[0])
	}
}

func TestMergeCommandSurfaceCatalogPreservesLegacyCardsForTruncatedGenericCatalog(t *testing.T) {
	t.Parallel()

	total := maxDiscoveredSurfaceTriggers + 20
	genericCount := total
	retained := make([]DiscoveredTrigger, 0, maxDiscoveredSurfaceTriggers)
	for index := range maxDiscoveredSurfaceTriggers {
		retained = append(retained, DiscoveredTrigger{
			ID:       fmt.Sprintf("generic-%03d", index),
			Kind:     "worker",
			Producer: SurfaceProducerGeneric,
		})
	}
	data := &ReportData{
		CommandTraces: []gofacts.CommandTrace{
			testCommandTrace("backup", "newBackupCommand", "cmd/app/backup.go", 10),
			testCommandTrace("restore", "newRestoreCommand", "cmd/app/restore.go", 20),
		},
		DiscoveredSurfaces: &DiscoveredSurfaces{
			Version:             6,
			TotalCount:          total,
			Truncated:           true,
			GenericSurfaceCount: genericCount,
			Triggers:            retained,
		},
	}

	mergeCommandSurfaceCatalog(data)

	catalog := data.DiscoveredSurfaces
	if len(catalog.Triggers) != maxDiscoveredSurfaceTriggers ||
		catalog.TotalCount != total+2 ||
		catalog.CLICommandCount != 2 ||
		catalog.GenericSurfaceCount != genericCount ||
		!catalog.Truncated {
		t.Fatalf("truncated generic catalog did not retain legacy command counts: %#v", catalog)
	}
	var retainedLegacy int
	for _, trigger := range catalog.Triggers {
		if trigger.Identity.Name == "backup" || trigger.Identity.Name == "restore" {
			retainedLegacy++
		}
	}
	if retainedLegacy == 0 {
		t.Fatalf("legacy CLI cards were not retained in bounded generic catalog: %#v", catalog.Triggers)
	}
}

func TestMergeCommandSurfaceCatalogSkipsLegacyWhenTruncatedCountMayHideTypedCommands(t *testing.T) {
	t.Parallel()

	total := maxDiscoveredSurfaceTriggers + 20
	retained := DiscoveredTrigger{
		ID:       "generic-worker",
		Kind:     "worker",
		Producer: SurfaceProducerGeneric,
	}
	data := &ReportData{
		CommandTraces: []gofacts.CommandTrace{
			testCommandTrace("backup", "newBackupCommand", "cmd/app/backup.go", 10),
		},
		DiscoveredSurfaces: &DiscoveredSurfaces{
			TotalCount:          total,
			Truncated:           true,
			CLICommandCount:     1,
			GenericSurfaceCount: total - 1,
			Triggers:            []DiscoveredTrigger{retained},
		},
	}

	mergeCommandSurfaceCatalog(data)

	catalog := data.DiscoveredSurfaces
	if len(catalog.Triggers) != 1 || catalog.Triggers[0].ID != retained.ID {
		t.Fatalf("legacy trace was merged despite a possibly omitted typed command: %#v", catalog.Triggers)
	}
	if catalog.TotalCount != total ||
		catalog.CLICommandCount != 1 ||
		catalog.GenericSurfaceCount != total-1 {
		t.Fatalf("hidden typed command counts changed during legacy merge: %#v", catalog)
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
		trigger.Availability != SurfaceAvailabilityUnavailable || trigger.ProcessEntrypoint.Location == nil ||
		trigger.SurfaceRole != SurfaceRoleRejected || trigger.TraceReadiness != SurfaceTraceRejected ||
		trigger.Quality.Traceability != SurfaceTraceRejected {
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

func TestProjectDiscoveredSurfacesPreservesUnavailableProcessProducerSemantics(t *testing.T) {
	t.Parallel()

	projected := projectDiscoveredSurfaces(rawSurfaceCatalog{Triggers: []rawSurfaceTrigger{{
		ID: "process-primary", Kind: "process_entry", Resolution: "exact",
		ProcessEntrypoint: rawSurfaceSymbol{Location: rawSurfaceLocation{Path: "cmd/app/main.go", Line: 7}},
		Availability:      SurfaceAvailabilityUnavailable, ApplicationClass: SurfaceApplicationOwned,
		SurfaceRole: SurfaceRoleEntrySurface, TraceReadiness: SurfaceTracePartialReady,
		TraceReadinessReason: "exact process entry can seed a one-anchor partial trace; typed downstream closure is unavailable",
		Quality: rawSurfaceQuality{Identity: surfaceQualityExact, RegistrationStart: surfaceQualityNotApplicable,
			HandlerCallback: surfaceQualityNotApplicable, Reachability: surfaceQualityPartial,
			Ownership: surfaceQualityExact, Traceability: SurfaceTracePartialReady},
	}}}, rawSurfaceCoverage{})
	trigger := projected.Triggers[0]
	if trigger.SurfaceRole != SurfaceRoleEntrySurface || trigger.TraceReadiness != SurfaceTracePartialReady ||
		trigger.Quality.Identity != surfaceQualityExact || trigger.Quality.Traceability != SurfaceTracePartialReady {
		t.Fatalf("producer process semantics were rewritten: %#v", trigger)
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
					catalog.Version = 8
				})
			},
			warning: "unsupported trigger_catalog.json version 8",
		},
		{
			name: "unsupported coverage version",
			mutate: func(t *testing.T, runDir string) {
				mutateSurfaceCoverage(t, runDir, func(coverage *rawSurfaceCoverage) {
					coverage.Version = 8
				})
			},
			warning: "unsupported surface_coverage.json version 8",
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
	beforeTotal := surfaces.TotalCount
	beforeCLICommands := surfaces.CLICommandCount
	beforeUnassigned := surfaces.UnassignedCount
	refreshSurfaceCatalogCounts(surfaces)
	if surfaces.TotalCount != beforeTotal ||
		surfaces.CLICommandCount != beforeCLICommands ||
		surfaces.HTTPRouteCount != beforeRoutes ||
		surfaces.UnassignedCount != beforeUnassigned {
		t.Fatalf("truncated counts changed to displayed subset: %#v", surfaces)
	}
}

func TestProjectDiscoveredSurfacesCountsCobraBeforeMixedCatalogTruncation(t *testing.T) {
	t.Parallel()

	total := maxDiscoveredSurfaceTriggers + 20
	raw := make([]rawSurfaceTrigger, 0, total)
	for index := range total {
		producer := SurfaceProducerGeneric
		kind := "http_route"
		if index%3 == 0 {
			producer = SurfaceProducerCobra
			kind = "cli_command"
		}
		raw = append(raw, rawSurfaceTrigger{
			ID:             fmt.Sprintf("trigger-%03d", total-index-1),
			Kind:           kind,
			Producer:       producer,
			ScenarioID:     "scenario",
			Availability:   SurfaceAvailabilityAvailable,
			ExecutableRole: ExecutableRoleUnknown,
		})
	}

	projected := projectDiscoveredSurfaces(rawSurfaceCatalog{Triggers: raw}, rawSurfaceCoverage{})
	wantCLICommands := (total + 2) / 3
	if !projected.Truncated ||
		len(projected.Triggers) != maxDiscoveredSurfaceTriggers ||
		projected.TotalCount != total ||
		projected.CLICommandCount != wantCLICommands ||
		projected.GenericSurfaceCount != total-wantCLICommands {
		t.Fatalf("mixed truncated counts = %#v", projected)
	}

	refreshSurfaceCatalogCounts(projected)
	if projected.TotalCount != total ||
		projected.CLICommandCount != wantCLICommands ||
		projected.GenericSurfaceCount != total-wantCLICommands ||
		projected.UnassignedCount != total {
		t.Fatalf("refresh rewrote raw mixed counts from retained subset: %#v", projected)
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
