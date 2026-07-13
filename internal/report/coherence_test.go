package report

import (
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
	if surface.TraceUnavailableReason != "unsupported surface kind" {
		t.Fatalf("unavailable reason = %q", surface.TraceUnavailableReason)
	}
}

func TestRefreshProductCountsKeepsSuggestionsDistinctFromSavedTraces(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		Run: &RunInfo{},
		CandidateDirections: []CandidateDirection{
			{ID: "complete"},
			{ID: "suggested"},
			{ID: "rejected", Disposition: flowexplain.DirectionRejected},
		},
		ArchitectureCanvas: &ArchitectureCanvas{
			Surfaces: []ArchitectureSurface{{ID: "one"}, {ID: "two"}},
			Flows:    []ArchitectureFlow{{ID: "complete", Status: "complete"}, {ID: "partial", Status: "partial"}},
		},
		DiscoveredSurfaces: &DiscoveredSurfaces{Triggers: []DiscoveredTrigger{{ID: "one"}, {ID: "two"}}},
		Flows:              []FlowData{{ID: "failed", Error: "bounded analysis failed"}},
	}

	refreshProductCounts(data)

	if data.Run.SuggestedInvestigationCount != 1 || data.Run.SavedTraceCount != 2 ||
		data.Run.CompleteTraceCount != 1 || data.Run.PartialTraceCount != 1 ||
		data.Run.FailedTraceAttemptCount != 1 || data.Run.DiscoveredSurfaceCount != 2 {
		t.Fatalf("counts = %#v", data.Run)
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
			if surface.RelatedTraceID != "" || surface.ExecutableRole != ExecutableRoleUnknown {
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
