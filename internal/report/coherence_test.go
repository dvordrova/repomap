package report

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/flowproof"
	"github.com/dvordrova/repomap/internal/gofacts"
)

func TestLinkArchitectureProductObjectsUsesExactEvidenceJoins(t *testing.T) {
	t.Parallel()

	const componentID = componentmap.ComponentID("component-cli")
	data := &ReportData{
		CandidateDirections: []CandidateDirection{
			{ID: "backup", Name: "Backup", WhyInteresting: "Primary user operation"},
			{ID: "check", Name: "Check", LikelyEntrypoint: "cmd/restic/cmd_check.go"},
		},
		ArchitectureCanvas: &ArchitectureCanvas{
			Components: []ArchitectureComponent{{
				ID: componentID,
				Members: []componentmap.Candidate{{
					ID: componentmap.MemberID{Kind: componentmap.MemberPackage, Value: "github.com/restic/restic/cmd/restic"},
					Facts: []componentmap.LocalFact{{
						Kind: componentmap.FactRepositoryPath, Value: "cmd/restic",
						Location: &evidence.Location{Path: "cmd/restic", Line: 1}, Certainty: evidence.CertaintyStatic,
					}},
				}},
				ParticipatingFlowIDs: []componentmap.FlowID{"backup"},
			}},
			Flows: []ArchitectureFlow{{
				ID: "backup", Name: "Backup", Trigger: "CLI command backup", Command: "backup",
				SeedSurfaceID: "surface-backup",
				Steps: []ArchitectureFlowStep{{
					ID: "register-backup", ComponentID: componentID,
					Location: &evidence.Location{Path: "cmd/restic/cmd_backup.go", Line: 74},
				}},
				Slots: []flowproof.Slot{
					{Kind: flowproof.SlotTrigger, Status: flowproof.SlotVerified, EvidenceIDs: []string{"register-backup"}},
					{Kind: flowproof.SlotEntrypoint, Status: flowproof.SlotVerified, EvidenceIDs: []string{"register-backup"}},
				},
			}},
		},
		DiscoveredSurfaces: &DiscoveredSurfaces{Triggers: []DiscoveredTrigger{{
			ID: "surface-backup", Kind: "worker", Identity: SurfaceIdentity{Name: "backup"},
			ProcessEntrypoint: SurfaceSymbol{Package: "github.com/restic/restic/cmd/restic", Location: &SurfaceLocation{Path: "cmd/restic/main.go", Line: 10}},
			RegistrationSite:  &SurfaceLocation{Path: "cmd/restic/cmd_backup.go", Line: 74},
			Handler:           SurfaceValue{Known: true, Text: "runBackup"}, Certainty: "static", Resolution: "static",
		}}},
	}

	linkArchitectureProductObjects(data)

	component := data.ArchitectureCanvas.Components[0]
	if len(component.OwnedSurfaceIDs) != 1 || component.OwnedSurfaceIDs[0] != "surface-backup" {
		t.Fatalf("owned surfaces = %v", component.OwnedSurfaceIDs)
	}
	if len(component.SuggestedInvestigationIDs) != 1 || component.SuggestedInvestigationIDs[0] != "check" {
		t.Fatalf("suggested investigations = %v", component.SuggestedInvestigationIDs)
	}
	surface := data.ArchitectureCanvas.Surfaces[0]
	if surface.OwningComponentID != componentID || surface.RelatedTraceID != "backup" {
		t.Fatalf("surface join = %#v", surface)
	}
	trace := data.ArchitectureCanvas.Flows[0]
	if trace.Status != "complete" || trace.GroundedAreas != 2 || trace.TotalAreas != 2 ||
		trace.StartSurfaceID != surface.ID || len(trace.ParticipatingComponentIDs) != 1 {
		t.Fatalf("trace summary = %#v", trace)
	}
}

func TestTraceAssociationUsesOnlyPersistedSeedAndEvidenceSurfaces(t *testing.T) {
	t.Parallel()

	const componentID = componentmap.ComponentID("runtime")
	processLocation := &SurfaceLocation{Path: "cmd/app/main.go", Line: 20}
	data := &ReportData{
		ArchitectureCanvas: &ArchitectureCanvas{
			Components: []ArchitectureComponent{{
				ID: componentID,
				Members: []componentmap.Candidate{{
					ID: componentmap.MemberID{Kind: componentmap.MemberFile, Value: "cmd/app/main.go"},
					Facts: []componentmap.LocalFact{{
						Kind: componentmap.FactRepositoryPath, Value: "cmd/app/main.go",
						Location:  &evidence.Location{Path: "cmd/app/main.go", Line: 20},
						Certainty: evidence.CertaintyStatic,
					}},
				}},
				ParticipatingFlowIDs: []componentmap.FlowID{"startup"},
			}},
			Flows: []ArchitectureFlow{{
				ID: "startup", Name: "Startup", SeedSurfaceID: "entry",
				TraceEvidenceSurfaceIDs: []string{"server"},
				Steps: []ArchitectureFlowStep{
					{ID: "entry", ComponentID: componentID, Location: &evidence.Location{Path: "cmd/app/main.go", Line: 20}},
					{ID: "server", ComponentID: componentID, Location: &evidence.Location{Path: "internal/admin.go", Line: 40}},
				},
				Slots: []flowproof.Slot{{
					Kind: flowproof.SlotEntrypoint, Status: flowproof.SlotVerified, EvidenceIDs: []string{"entry"},
				}},
			}},
		},
		DiscoveredSurfaces: &DiscoveredSurfaces{Triggers: []DiscoveredTrigger{
			{
				ID: "entry", Kind: "process_entry", ProcessEntrypoint: SurfaceSymbol{Location: processLocation},
				OwningExecutable: "cmd/app", Availability: SurfaceAvailabilityAvailable,
			},
			{
				ID: "server", Kind: "http_server", ProcessEntrypoint: SurfaceSymbol{Location: processLocation},
				ServerStartSite:  &SurfaceLocation{Path: "internal/admin.go", Line: 40},
				OwningExecutable: "cmd/app", Availability: SurfaceAvailabilityAvailable,
			},
			{
				ID: "unrelated-route", Kind: "http_route", ProcessEntrypoint: SurfaceSymbol{Location: processLocation},
				RegistrationSite: &SurfaceLocation{Path: "internal/routes.go", Line: 70},
				OwningExecutable: "cmd/app", Availability: SurfaceAvailabilityAvailable,
			},
		}},
	}

	linkArchitectureProductObjects(data)

	trace := data.ArchitectureCanvas.Flows[0]
	if trace.StartSurfaceID != "entry" || len(trace.TraceEvidenceSurfaceIDs) != 1 ||
		trace.TraceEvidenceSurfaceIDs[0] != "server" || len(trace.RelatedComponentSurfaceIDs) != 1 ||
		trace.RelatedComponentSurfaceIDs[0] != "unrelated-route" {
		t.Fatalf("trace surface relations = %#v", trace)
	}
	for _, surface := range data.ArchitectureCanvas.Surfaces {
		switch surface.ID {
		case "entry", "server":
			if surface.RelatedTraceID != "startup" {
				t.Fatalf("exact trace surface = %#v", surface)
			}
		case "unrelated-route":
			if surface.RelatedTraceID != "" {
				t.Fatalf("component-related surface was mislabeled as trace evidence: %#v", surface)
			}
		}
	}
}

func TestTraceAssociationRejectsEvidenceSurfaceWithoutAnchorOrTransition(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		ArchitectureCanvas: &ArchitectureCanvas{Flows: []ArchitectureFlow{{
			ID: "startup", TraceEvidenceSurfaceIDs: []string{"route"},
		}}},
		DiscoveredSurfaces: &DiscoveredSurfaces{Triggers: []DiscoveredTrigger{{
			ID: "route", Kind: "http_route", RegistrationSite: &SurfaceLocation{Path: "routes.go", Line: 10},
		}}},
	}
	linkArchitectureProductObjects(data)
	if data.ArchitectureCanvas.Surfaces[0].RelatedTraceID != "" ||
		len(data.ArchitectureCanvas.Flows[0].TraceEvidenceSurfaceIDs) != 0 {
		t.Fatalf("unjustified trace evidence survived: %#v", data.ArchitectureCanvas)
	}
	var found bool
	for _, diagnostic := range data.ArchitectureCanvas.Diagnostics {
		found = found || diagnostic.Code == "trace.unjustified_evidence_surface"
	}
	if !found {
		t.Fatalf("diagnostics = %#v", data.ArchitectureCanvas.Diagnostics)
	}
}

func TestLinkArchitectureProductObjectsLeavesAmbiguousSurfaceUnassigned(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		ArchitectureCanvas: &ArchitectureCanvas{Components: []ArchitectureComponent{
			architecturePathComponent("a", "shared/start.go"),
			architecturePathComponent("b", "shared/start.go"),
		}},
		DiscoveredSurfaces: &DiscoveredSurfaces{Triggers: []DiscoveredTrigger{{
			ID: "shared", Kind: "async_task", Identity: SurfaceIdentity{Name: "shared"},
			RegistrationSite: &SurfaceLocation{Path: "shared/start.go", Line: 20},
			Handler:          SurfaceValue{Known: true, Text: "start"}, Certainty: "static", Resolution: "static",
		}}},
	}

	linkArchitectureProductObjects(data)

	surface := data.ArchitectureCanvas.Surfaces[0]
	if surface.OwningComponentID != "" || surface.Category != surfaceCategoryUnassigned {
		t.Fatalf("ambiguous surface = %#v", surface)
	}
	if surface.TraceUnavailableReason != "runtime activity is nested asynchronous work and cannot independently establish a top-level trace" {
		t.Fatalf("unavailable reason = %q", surface.TraceUnavailableReason)
	}
}

func TestSuggestionMapsPackageDeclarationToRepositoryPackageFiles(t *testing.T) {
	t.Parallel()

	const (
		canonical = "example.com/project/internal/service"
		component = componentmap.ComponentID("service-component")
	)
	data := &ReportData{
		RepositoryGraph: &RepositoryGraph{Packages: []PackageInfo{{
			CanonicalPath: canonical, Dir: "internal/service",
			Files: []string{"internal/service/service.go", "internal/service/worker.go"},
		}}},
		CandidateDirections: []CandidateDirection{{
			ID: "service", Name: "Service", LikelyEntrypoint: canonical,
		}},
		ArchitectureCanvas: &ArchitectureCanvas{Components: []ArchitectureComponent{{
			ID: component,
			Members: []componentmap.Candidate{{
				ID: componentmap.MemberID{Kind: componentmap.MemberPackage, Value: "opaque-package-member"},
				Facts: []componentmap.LocalFact{{
					Kind: componentmap.FactDeclaration, Value: canonical, Certainty: evidence.CertaintyStatic,
				}},
			}},
		}}},
	}

	linkArchitectureProductObjects(data)

	if len(data.ArchitectureCanvas.Suggestions) != 1 {
		t.Fatalf("suggestions = %#v", data.ArchitectureCanvas.Suggestions)
	}
	suggestion := data.ArchitectureCanvas.Suggestions[0]
	if len(suggestion.RelevantComponentIDs) != 1 || suggestion.RelevantComponentIDs[0] != component ||
		!suggestion.InvestigationAvailable || suggestion.StartLocation == nil ||
		suggestion.StartLocation.Path != "internal/service/service.go" {
		t.Fatalf("package suggestion = %#v", suggestion)
	}
	if suggestion.CanStartTrace || suggestion.TraceUnavailableReason == "" {
		t.Fatalf("package source was conflated with a trace seed: %#v", suggestion)
	}
	if len(data.OpenablePaths) != 1 || data.OpenablePaths[0] != suggestion.StartLocation.Path {
		t.Fatalf("package source was not authorized for opening: %v", data.OpenablePaths)
	}
}

func TestSuggestionLeavesAmbiguousPackageDeclarationUnassigned(t *testing.T) {
	t.Parallel()

	const canonical = "example.com/project/internal/shared"
	packageMember := func(id componentmap.ComponentID) ArchitectureComponent {
		return ArchitectureComponent{
			ID: id,
			Members: []componentmap.Candidate{{
				ID: componentmap.MemberID{Kind: componentmap.MemberPackage, Value: "opaque-" + string(id)},
				Facts: []componentmap.LocalFact{{
					Kind: componentmap.FactDeclaration, Value: canonical, Certainty: evidence.CertaintyStatic,
				}},
			}},
		}
	}
	data := &ReportData{
		RepositoryGraph: &RepositoryGraph{Packages: []PackageInfo{{
			CanonicalPath: canonical, Dir: "internal/shared", Files: []string{"internal/shared/shared.go"},
		}}},
		CandidateDirections: []CandidateDirection{{
			ID: "shared", Name: "Shared", LikelyFiles: []string{"internal/shared/shared.go"},
		}},
		ArchitectureCanvas: &ArchitectureCanvas{Components: []ArchitectureComponent{
			packageMember("component-a"), packageMember("component-b"),
		}},
	}

	linkArchitectureProductObjects(data)

	suggestion := data.ArchitectureCanvas.Suggestions[0]
	if len(suggestion.RelevantComponentIDs) != 0 ||
		len(data.ArchitectureCanvas.Components[0].SuggestedInvestigationIDs) != 0 ||
		len(data.ArchitectureCanvas.Components[1].SuggestedInvestigationIDs) != 0 {
		t.Fatalf("ambiguous package suggestion received ownership: %#v", suggestion)
	}
	if !suggestion.InvestigationAvailable || suggestion.StartLocation == nil {
		t.Fatalf("ambiguous ownership hid exact source availability: %#v", suggestion)
	}
}

func TestSuggestionsKeepSourceAndTypedTraceAvailabilityDistinct(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		CandidateDirections: []CandidateDirection{
			{ID: "route", Name: "Route", LikelyFiles: []string{"server/route.go"}, CandidateBasis: flowexplain.CandidateBasisModelOrientation},
			{ID: "aggregate", Name: "Aggregate", LikelyFiles: []string{"signals/aggregate.go"}, CandidateBasis: flowexplain.CandidateBasisSourceSignalAggregate},
			{ID: "activity", Name: "Activity", LikelyFiles: []string{"worker/run.go"}, CandidateBasis: flowexplain.CandidateBasisRuntimeActivity},
			{ID: "broken", Name: "Broken process", LikelyEntrypoint: "cmd/broken/main.go", CandidateBasis: flowexplain.CandidateBasisLocalEntrypoint},
		},
		ArchitectureCanvas: &ArchitectureCanvas{Components: []ArchitectureComponent{
			architecturePathComponent("route-component", "server/route.go"),
			architecturePathComponent("aggregate-component", "signals/aggregate.go"),
			architecturePathComponent("worker-component", "worker/run.go"),
			architecturePathComponent("broken-component", "cmd/broken/main.go"),
		}},
		DiscoveredSurfaces: &DiscoveredSurfaces{Triggers: []DiscoveredTrigger{
			{
				ID: "route-surface", Kind: "http_route", Identity: SurfaceIdentity{Path: SurfaceValue{Known: true, Text: "/ready"}},
				RegistrationSite:  &SurfaceLocation{Path: "server/route.go", Line: 20},
				ProcessEntrypoint: SurfaceSymbol{Location: &SurfaceLocation{Path: "cmd/server/main.go", Line: 10}},
				Handler:           SurfaceValue{Known: true, Text: "ready"}, Resolution: "exact",
			},
			{
				ID: "worker-surface", Kind: "worker", RegistrationSite: &SurfaceLocation{Path: "worker/run.go", Line: 30},
				Handler: SurfaceValue{Known: true, Text: "run"}, Resolution: "exact",
			},
			{
				ID: "broken-process", Kind: "process_entry", Availability: SurfaceAvailabilityUnavailable,
				UnavailableReason: "package failed to load under the recorded build scenario",
				ProcessEntrypoint: SurfaceSymbol{Location: &SurfaceLocation{Path: "cmd/broken/main.go", Line: 5}},
				Resolution:        "exact", ApplicationClass: SurfaceApplicationOwned,
				SurfaceRole: SurfaceRoleEntrySurface, TraceReadiness: SurfaceTracePartialReady,
				TraceReadinessReason: "exact process entry can seed a one-anchor partial trace; typed downstream closure is unavailable",
				Quality: SurfaceQuality{Identity: surfaceQualityExact, RegistrationStart: surfaceQualityNotApplicable,
					HandlerCallback: surfaceQualityNotApplicable, Reachability: surfaceQualityPartial,
					Ownership: surfaceQualityExact, Traceability: SurfaceTracePartialReady},
			},
		}},
	}

	linkArchitectureProductObjects(data)
	suggestions := make(map[string]ArchitectureSuggestion)
	for _, suggestion := range data.ArchitectureCanvas.Suggestions {
		suggestions[suggestion.ID] = suggestion
	}
	if !suggestions["route"].InvestigationAvailable || !suggestions["route"].CanStartTrace ||
		suggestions["route"].StartLocation == nil || suggestions["route"].StartLocation.Line != 20 {
		t.Fatalf("route suggestion = %#v", suggestions["route"])
	}
	for _, id := range []string{"aggregate", "activity"} {
		suggestion := suggestions[id]
		if !suggestion.InvestigationAvailable || suggestion.CanStartTrace || suggestion.StartLocation == nil ||
			suggestion.TraceUnavailableReason == "" || suggestion.UnavailableReason != "" {
			t.Errorf("source-only suggestion %q = %#v", id, suggestion)
		}
	}
	if !suggestions["broken"].InvestigationAvailable || !suggestions["broken"].CanStartTrace ||
		suggestions["broken"].TraceUnavailableReason != "" {
		t.Fatalf("exact unavailable process suggestion = %#v", suggestions["broken"])
	}
	if suggestions["aggregate"].TraceUnavailableReason == suggestions["activity"].TraceUnavailableReason {
		t.Fatalf("aggregate/activity trace reasons are not distinct: %q", suggestions["aggregate"].TraceUnavailableReason)
	}
	rendered, err := RenderHTML(data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rendered), suggestions["aggregate"].TraceUnavailableReason) ||
		!strings.Contains(string(rendered), "suggestion.trace_unavailable_reason") {
		t.Fatal("rendered report omitted the suggestion trace-unavailable reason")
	}
}

func TestRefreshProductCountsKeepsSuggestionsDistinctFromSavedTraces(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		Run: &RunInfo{},
		CandidateDirections: []CandidateDirection{
			{ID: "complete", LocalProof: testSavedTraceSession("complete", true)},
			{ID: "partial", LocalProof: testSavedTraceSession("partial", false)},
			{ID: "suggested"},
			{ID: "rejected", Disposition: flowexplain.DirectionRejected},
		},
		ArchitectureCanvas: &ArchitectureCanvas{
			Surfaces:    []ArchitectureSurface{{ID: "one"}, {ID: "two"}},
			Suggestions: []ArchitectureSuggestion{{ID: "suggested"}},
			Flows:       []ArchitectureFlow{{ID: "complete", Status: "complete"}, {ID: "partial", Status: "partial"}},
		},
		DiscoveredSurfaces: &DiscoveredSurfaces{Triggers: []DiscoveredTrigger{{ID: "one"}, {ID: "two"}}},
		Flows: []FlowData{
			{ID: "evidence", EvidenceOnly: true, FlowStatus: "local_only"},
			{ID: "failed", Error: "bounded analysis failed"},
		},
	}

	refreshProductCounts(data)

	if data.Run.SuggestedInvestigationCount != 1 || data.Run.SavedTraceCount != 2 ||
		data.Run.CompleteTraceCount != 1 || data.Run.PartialTraceCount != 1 ||
		data.Run.FailedTraceAttemptCount != 1 || data.Run.EvidenceBundleCount != 1 ||
		data.Run.DiscoveredSurfaceCount != 2 {
		t.Fatalf("counts = %#v", data.Run)
	}
}

func testSavedTraceSession(id string, complete bool) *flowproof.Session {
	slots := make([]flowproof.Slot, 0, 8)
	for _, kind := range []flowproof.SlotKind{
		flowproof.SlotTrigger,
		flowproof.SlotEntrypoint,
		flowproof.SlotDispatch,
		flowproof.SlotApplicationCallable,
		flowproof.SlotCoreOperation,
		flowproof.SlotIOBoundary,
		flowproof.SlotConcurrency,
		flowproof.SlotTermination,
	} {
		status := flowproof.SlotMissing
		missing := "not collected"
		if complete || len(slots) == 0 {
			status = flowproof.SlotVerified
			missing = ""
		}
		slots = append(slots, flowproof.Slot{Kind: kind, Status: status, Missing: missing})
	}
	return &flowproof.Session{
		Version: flowproof.SessionVersion,
		Proof: flowproof.Proof{
			Version: flowproof.Version,
			ID:      id, Archetype: flowproof.ArchetypeProcess,
			Slots: slots,
		},
	}
}

func TestUnifiedSurfaceCatalogCountsProducersRolesAndUntracedCommands(t *testing.T) {
	t.Parallel()

	backupComponent := componentmap.ComponentID("backup-component")
	restoreComponent := componentmap.ComponentID("restore-component")
	data := &ReportData{
		Run: &RunInfo{},
		RepositoryGraph: &RepositoryGraph{Packages: []PackageInfo{{
			CanonicalPath: "example.com/restic/cmd/restic", DisplayPath: "cmd/restic",
		}}},
		CommandTraces: []gofacts.CommandTrace{
			testCommandTrace("backup", "newBackupCommand", "cmd/restic/cmd_backup.go", 35),
			testCommandTrace("restore", "newRestoreCommand", "cmd/restic/cmd_restore.go", 25),
		},
		DiscoveredSurfaces: &DiscoveredSurfaces{Triggers: []DiscoveredTrigger{
			{ID: "build-task-1", Kind: "worker", Producer: SurfaceProducerGeneric, ProcessEntrypoint: SurfaceSymbol{Location: &SurfaceLocation{Path: "helpers/build-release-binaries/main.go", Line: 10}}},
			{ID: "build-task-2", Kind: "async_task", Producer: SurfaceProducerGeneric, ProcessEntrypoint: SurfaceSymbol{Location: &SurfaceLocation{Path: "helpers/build-release-binaries/main.go", Line: 10}}},
		}},
		ArchitectureCanvas: &ArchitectureCanvas{
			BehaviorAnchors: []componentmap.BehaviorAnchor{{
				ID: "restic-process", Kind: componentmap.AnchorProcessEntry,
				Location: evidence.Location{Path: "cmd/restic/main.go", Line: 10},
			}},
			Components: []ArchitectureComponent{
				architecturePathComponent(backupComponent, "cmd/restic/cmd_backup.go"),
				architecturePathComponent(restoreComponent, "cmd/restic/cmd_restore.go"),
			},
			Flows: []ArchitectureFlow{{
				ID: "backup-flow", Command: "backup", Status: "partial",
				Steps: []ArchitectureFlowStep{{ID: "backup", Location: &evidence.Location{Path: "cmd/restic/cmd_backup.go", Line: 45}}},
			}},
		},
	}
	mergeCommandSurfaceCatalog(data)
	linkArchitectureProductObjects(data)
	refreshProductCounts(data)

	if len(data.DiscoveredSurfaces.Triggers) != 4 || len(data.ArchitectureCanvas.Surfaces) != 4 || data.Run.DiscoveredSurfaceCount != 4 {
		t.Fatalf("catalog/canvas/headline counts = %d/%d/%d", len(data.DiscoveredSurfaces.Triggers), len(data.ArchitectureCanvas.Surfaces), data.Run.DiscoveredSurfaceCount)
	}
	if data.Run.ApplicationSurfaceCount != 2 || data.Run.ToolingSurfaceCount != 2 || data.Run.GenericSurfaceCount != 2 || data.Run.CLICommandSurfaceCount != 2 {
		t.Fatalf("surface breakdown = %#v", data.Run)
	}
	for _, surface := range data.DiscoveredSurfaces.Triggers {
		switch surface.Identity.Name {
		case "backup":
			if surface.RelatedTraceID != "backup-flow" || surface.OwningComponentID != backupComponent {
				t.Fatalf("backup surface = %#v", surface)
			}
		case "restore":
			if surface.RelatedTraceID != "" || surface.OwningComponentID != restoreComponent {
				t.Fatalf("untraced restore surface = %#v", surface)
			}
		default:
			if surface.ExecutableRole != ExecutableRoleSecondaryTooling {
				t.Fatalf("helper surface role = %q", surface.ExecutableRole)
			}
		}
	}
}

func TestCommandSurfaceTraceJoinIncludesExecutableIdentity(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		CommandTraces: []gofacts.CommandTrace{
			testCommandTraceFor("example.com/app-a", "cmd/app-a", "serve", "newServeCommand", "cmd/app-a/serve.go", 20, 30),
			testCommandTraceFor("example.com/app-b", "cmd/app-b", "serve", "newServeCommand", "cmd/app-b/serve.go", 20, 30),
		},
		ArchitectureCanvas: &ArchitectureCanvas{
			BehaviorAnchors: []componentmap.BehaviorAnchor{{
				ID: "app-a-process", Kind: componentmap.AnchorProcessEntry,
				Location: evidence.Location{Path: "cmd/app-a/main.go", Line: 10},
			}},
			Components: []ArchitectureComponent{
				architecturePathComponent("app-a", "cmd/app-a/serve.go"),
				architecturePathComponent("app-b", "cmd/app-b/serve.go"),
			},
			Flows: []ArchitectureFlow{{
				ID: "serve-flow", Command: "serve",
				Steps: []ArchitectureFlowStep{{ID: "serve", Location: &evidence.Location{Path: "cmd/app-a/serve.go", Line: 30}}},
			}},
		},
	}
	mergeCommandSurfaceCatalog(data)
	linkArchitectureProductObjects(data)

	for _, surface := range data.DiscoveredSurfaces.Triggers {
		switch surface.OwningExecutable {
		case "cmd/app-a":
			if surface.RelatedTraceID != "serve-flow" || surface.ExecutableRole != ExecutableRolePrimaryApplication {
				t.Fatalf("app-a surface = %#v", surface)
			}
		case "cmd/app-b":
			if surface.RelatedTraceID != "" || surface.ExecutableRole != ExecutableRoleSecondaryService {
				t.Fatalf("app-b surface = %#v", surface)
			}
		}
	}
}

func TestNonCobraPrimarySurfaceUsesSameRoleForCountsAndGrouping(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		Run: &RunInfo{},
		DiscoveredSurfaces: &DiscoveredSurfaces{Triggers: []DiscoveredTrigger{{
			ID: "worker", Kind: "worker", Producer: SurfaceProducerGeneric,
			ProcessEntrypoint: SurfaceSymbol{Location: &SurfaceLocation{Path: "cmd/app/main.go", Line: 10}},
		}}},
		ArchitectureCanvas: &ArchitectureCanvas{
			BehaviorAnchors: []componentmap.BehaviorAnchor{{
				ID: "app-process", Kind: componentmap.AnchorProcessEntry,
				Location: evidence.Location{Path: "cmd/app/main.go", Line: 10},
			}},
			Components: []ArchitectureComponent{architecturePathComponent("app", "cmd/app/main.go")},
		},
	}
	linkArchitectureProductObjects(data)
	refreshProductCounts(data)

	trigger := data.DiscoveredSurfaces.Triggers[0]
	surface := data.ArchitectureCanvas.Surfaces[0]
	if trigger.ExecutableRole != ExecutableRolePrimaryApplication || surface.Category != surfaceCategoryApplication ||
		data.Run.ApplicationSurfaceCount != 1 {
		t.Fatalf("trigger=%#v surface=%#v counts=%#v", trigger, surface, data.Run)
	}
}

func TestRepositoryNamedMainClassifiesPrimaryWithoutArchitectureCanvas(t *testing.T) {
	t.Parallel()

	data := &ReportData{RepoName: "caddy"}
	trigger := DiscoveredTrigger{
		Kind:     "http_route",
		Producer: SurfaceProducerGeneric,
		ProcessEntrypoint: SurfaceSymbol{
			Name: "main",
			Location: &SurfaceLocation{
				Path: "cmd/caddy/main.go",
				Line: 40,
			},
		},
	}
	classifySurfaceExecutable(data, &trigger)
	if trigger.OwningExecutable != "cmd/caddy" || trigger.ExecutableRole != ExecutableRolePrimaryApplication {
		t.Fatalf("repository-named main executable = %#v", trigger)
	}
}

func TestShallowCobraDescriptorKeepsSourceOwnershipWithoutRuntimeRole(t *testing.T) {
	t.Parallel()

	trigger := DiscoveredTrigger{
		Kind:      "cli_command",
		Producer:  SurfaceProducerCobra,
		Framework: "cobra",
		Constructor: SurfaceSymbol{
			Package: "example.com/app/command",
			Name:    "newServeCommand",
			Location: &SurfaceLocation{
				Path: "command/serve.go",
				Line: 20,
			},
		},
		DescriptorSite: &SurfaceLocation{
			Path: "command/serve.go",
			Line: 21,
		},
	}

	classifySurfaceExecutable(nil, &trigger)

	if trigger.OwningExecutable != "example.com/app/command" {
		t.Fatalf("shallow descriptor owner = %q", trigger.OwningExecutable)
	}
	if trigger.ExecutableRole != ExecutableRoleUnknown {
		t.Fatalf("shallow descriptor role = %q, want unknown", trigger.ExecutableRole)
	}
	if trigger.ProcessEntrypoint.Location != nil || trigger.ProcessEntrypoint.Package != "" {
		t.Fatalf("shallow descriptor invented process reachability: %#v", trigger.ProcessEntrypoint)
	}
}

func TestCommandSurfaceIDIgnoresRegistrationLineMovement(t *testing.T) {
	t.Parallel()

	first := commandTraceSurface(nil, testCommandTraceFor(
		"example.com/app", "cmd/app", "serve", "newServeCommand", "cmd/app/serve.go", 20, 30,
	))
	second := commandTraceSurface(nil, testCommandTraceFor(
		"example.com/app", "cmd/app", "serve", "newServeCommand", "cmd/app/serve.go", 120, 130,
	))
	if first.ID == "" || first.ID != second.ID {
		t.Fatalf("surface IDs changed after line movement: %q != %q", first.ID, second.ID)
	}
}

func TestConstructorDerivedCommandIdentityIsNotKnown(t *testing.T) {
	t.Parallel()

	trace := testCommandTraceFor(
		"example.com/app", "cmd/app", "newRepoListCommand", "newRepoListCommand", "cmd/app/list.go", 20, 30,
	)
	trace.Complete = false
	trace.Missing = []string{"command name"}
	surface := commandTraceSurface(nil, trace)
	if surface.Identity.Name != "repoList" || surface.Identity.Path.Known ||
		surface.Identity.Path.Kind != "constructor_derived_command" || surface.Resolution != "partial" {
		t.Fatalf("constructor-derived identity = %#v", surface)
	}
}

func testCommandTrace(command, constructor, sourcePath string, line int) gofacts.CommandTrace {
	return testCommandTraceFor(
		"example.com/restic/cmd/restic", "cmd/restic", command, constructor, sourcePath, line, line+10,
	)
}

func testCommandTraceFor(packagePath, executable, command, constructor, sourcePath string, line, callbackLine int) gofacts.CommandTrace {
	return gofacts.CommandTrace{
		Version: 2, Framework: "cobra", EntrypointPackage: packagePath, Command: command, Complete: true,
		Steps: []gofacts.CommandTraceStep{
			{Symbol: "main", Relation: "entrypoint", TargetLocation: evidence.Location{Path: executable + "/main.go", Line: 10}},
			{Symbol: "newRootCommand", Relation: "calls", TargetLocation: evidence.Location{Path: executable + "/main.go", Line: 20}},
			{Symbol: constructor, Relation: "registers_command", CallsiteLocation: &evidence.Location{Path: executable + "/main.go", Line: line}, TargetLocation: evidence.Location{Path: sourcePath, Line: line}},
			{Symbol: "run-" + command, Relation: "callback", TargetLocation: evidence.Location{Path: sourcePath, Line: callbackLine}},
		},
	}
}

func architecturePathComponent(id componentmap.ComponentID, repositoryPath string) ArchitectureComponent {
	return ArchitectureComponent{
		ID: id,
		Members: []componentmap.Candidate{{
			ID: componentmap.MemberID{Kind: componentmap.MemberFile, Value: repositoryPath},
			Facts: []componentmap.LocalFact{{
				Kind: componentmap.FactRepositoryPath, Value: repositoryPath,
				Location: &evidence.Location{Path: repositoryPath, Line: 1}, Certainty: evidence.CertaintyStatic,
			}},
		}},
	}
}
