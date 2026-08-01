package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/flowproof"
)

func TestResticFixtureNavigatesComponentSurfaceTraceEvidence(t *testing.T) {
	t.Parallel()

	fixture, err := os.ReadFile(filepath.Join("testdata", "canvas", "restic-backup-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	var data ReportData
	if err := json.Unmarshal(fixture, &data); err != nil {
		t.Fatal(err)
	}
	data.DiscoveredSurfaces = &DiscoveredSurfaces{Triggers: []DiscoveredTrigger{{
		ID: "restic-backup-command", Kind: "cli_command", Producer: SurfaceProducerCobra,
		Identity:          SurfaceIdentity{Name: "backup", Path: SurfaceValue{Kind: "command", Text: "backup", Known: true}},
		Constructor:       SurfaceSymbol{Name: "newBackupCommand", Location: &SurfaceLocation{Path: "cmd/restic/cmd_backup.go", Line: 74}},
		RegistrationSite:  &SurfaceLocation{Path: "cmd/restic/cmd_backup.go", Line: 74},
		ProcessEntrypoint: SurfaceSymbol{Package: "cmd/restic", Name: "main", Location: &SurfaceLocation{Path: "cmd/restic/main.go", Line: 1}},
		ExecutableRole:    ExecutableRolePrimaryApplication, Certainty: "static", Resolution: "exact", Status: "confirmed_command_registration",
	}}}
	ApplyProductCoherence(&data)
	if data.ArchitectureCanvas == nil || len(data.ArchitectureCanvas.Flows) != 1 {
		t.Fatalf("Restic canvas = %#v", data.ArchitectureCanvas)
	}
	trace := data.ArchitectureCanvas.Flows[0]
	if trace.StartSurfaceID == "" || trace.Status == "" || trace.EvidenceBasis != "static" ||
		len(trace.ParticipatingComponentIDs) < 3 {
		t.Fatalf("Restic trace summary = %#v", trace)
	}
	var start *ArchitectureSurface
	for index := range data.ArchitectureCanvas.Surfaces {
		if data.ArchitectureCanvas.Surfaces[index].ID == trace.StartSurfaceID {
			start = &data.ArchitectureCanvas.Surfaces[index]
			break
		}
	}
	if start == nil || start.RelatedTraceID != trace.ID || len(start.Evidence) == 0 {
		t.Fatalf("Restic start surface = %#v", start)
	}
	associated := 0
	for _, component := range data.ArchitectureCanvas.Components {
		if len(component.ParticipatingFlowIDs) > 0 {
			associated++
		}
	}
	if associated < 3 {
		t.Fatalf("Restic participating components = %d", associated)
	}
	rendered, err := RenderHTML(&data)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rendered), "Guided flows") || !strings.Contains(string(rendered), "Back to architecture") {
		t.Fatal("Restic report did not use coherent Architecture trace navigation")
	}
}

func TestCaddyAdminAnchorsCanHaveZeroSurfacesAndSeparateSuggestion(t *testing.T) {
	t.Parallel()

	const adminComponent = componentmap.ComponentID("component-admin-api")
	data := &ReportData{
		RepoName: "caddy",
		CandidateDirections: []CandidateDirection{{
			ID: "admin-api-investigation", Name: "Admin API request handling",
			LikelyEntrypoint: "admin.go", WhyInteresting: "Inspect the control-plane request boundary.",
		}},
		ArchitectureCanvas: &ArchitectureCanvas{
			Title: "Architecture hypotheses and grounded behavior",
			Components: []ArchitectureComponent{{
				ID: adminComponent, Name: "Admin API", AnchorIDs: []string{"anchor-admin"},
				Members: []componentmap.Candidate{{
					ID: componentmap.MemberID{Kind: componentmap.MemberFile, Value: "admin.go"},
					Facts: []componentmap.LocalFact{{
						Kind: componentmap.FactRepositoryPath, Value: "admin.go",
						Location: &evidence.Location{Path: "admin.go", Line: 1}, Certainty: evidence.CertaintyStatic,
					}},
				}},
			}},
			BehaviorAnchors: []componentmap.BehaviorAnchor{{
				ID: "anchor-admin", Kind: componentmap.AnchorAdminControlPlane, Label: "Admin control plane",
				Location: evidence.Location{Path: "admin.go", Line: 96}, Certainty: evidence.CertaintyStatic,
			}},
		},
		DiscoveredSurfaces: &DiscoveredSurfaces{Triggers: nil},
	}
	ApplyProductCoherence(data)
	component := data.ArchitectureCanvas.Components[0]
	if len(component.OwnedSurfaceIDs) != 0 || len(data.ArchitectureCanvas.Surfaces) != 0 {
		t.Fatalf("Caddy configured-catalog surfaces = %v / %v", component.OwnedSurfaceIDs, data.ArchitectureCanvas.Surfaces)
	}
	if len(component.SuggestedInvestigationIDs) != 1 || component.SuggestedInvestigationIDs[0] != "admin-api-investigation" {
		t.Fatalf("Caddy suggestions = %v", component.SuggestedInvestigationIDs)
	}
	if len(data.ArchitectureCanvas.Flows) != 0 {
		t.Fatalf("Caddy suggestion became a saved trace: %v", data.ArchitectureCanvas.Flows)
	}
	if len(data.ArchitectureCanvas.Suggestions) != 1 || !data.ArchitectureCanvas.Suggestions[0].InvestigationAvailable ||
		len(data.ArchitectureCanvas.Suggestions[0].RelevantAnchorIDs) != 1 {
		t.Fatalf("Caddy typed suggestion = %#v", data.ArchitectureCanvas.Suggestions)
	}
	rendered, err := RenderHTML(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range []string{
		"Zero configured-catalog surfaces is an honest bounded-analysis result",
		"Suggested investigation — not a saved trace",
		"No supported runtime registrations were cataloged.",
	} {
		if !strings.Contains(string(rendered), message) {
			t.Fatalf("Caddy report is missing %q", message)
		}
	}
}

func TestApplyProductCoherenceDoesNotCreateASecondArchitectureCanvas(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		CandidateDirections: []CandidateDirection{{
			ID: "backup", Name: "Backup", Disposition: flowexplain.DirectionAccepted,
			LocalProof: &flowproof.Session{
				Version: flowproof.SessionVersion,
				Proof: flowproof.Proof{
					Version: flowproof.Version, ID: "backup", Archetype: flowproof.ArchetypeCLI,
					Slots: []flowproof.Slot{{Kind: flowproof.SlotTrigger, Status: flowproof.SlotVerified}},
				},
			},
		}},
	}
	ApplyProductCoherence(data)
	if data.ArchitectureCanvas != nil {
		t.Fatalf("presentation coherence created a competing canvas: %#v", data.ArchitectureCanvas)
	}
}

func TestSavedResticCoherence(t *testing.T) {
	runDir := os.Getenv("REPOMAP_SAVED_RESTIC_RUN")
	if runDir == "" {
		t.Skip("set REPOMAP_SAVED_RESTIC_RUN to exercise the owner-provided model-backed run")
	}
	data, err := ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	ApplyProductCoherence(data)
	if data.ArchitectureCanvas == nil {
		input, buildErr := BuildArchitectureCanvasInput(data)
		t.Fatalf("saved Restic canvas is unavailable: build=%v candidates=%d flows=%d", buildErr, len(input.CandidateBundle.Candidates), len(input.Flows))
	}
	if len(data.ArchitectureCanvas.Flows) != 4 {
		t.Fatalf("saved Restic traces = %d, want 4", len(data.ArchitectureCanvas.Flows))
	}
	traceIDs := make(map[componentmap.FlowID]bool, len(data.ArchitectureCanvas.Flows))
	for _, trace := range data.ArchitectureCanvas.Flows {
		traceIDs[trace.ID] = true
	}
	for _, id := range []componentmap.FlowID{"backup-command", "check-command", "init-command", "restore-command"} {
		if !traceIDs[id] {
			t.Errorf("saved Restic trace is missing %q", id)
		}
	}
	if len(data.DiscoveredSurfaces.Triggers) != 56 || len(data.ArchitectureCanvas.Surfaces) != 56 || data.Run.DiscoveredSurfaceCount != 56 {
		t.Fatalf("saved Restic catalog/canvas/headline = %d/%d/%d, want 56 retained records", len(data.DiscoveredSurfaces.Triggers), len(data.ArchitectureCanvas.Surfaces), data.Run.DiscoveredSurfaceCount)
	}
	if data.Run.CLICommandSurfaceCount != 28 || data.Run.GenericSurfaceCount != 28 ||
		data.Run.ApplicationSurfaceCount != 29 || data.Run.ToolingSurfaceCount != 4 || data.Run.TestHelperSurfaceCount != 0 ||
		data.Run.SupportingDependencyCount != 23 || data.Run.DependencyOnlySurfaceCount != 0 {
		t.Fatalf("saved Restic surface breakdown = %#v", data.Run)
	}
	required := map[string]bool{"backup": false, "check": false, "init": false, "restore": false, "snapshots": false, "list": false, "prune": false, "find": false}
	for _, surface := range data.DiscoveredSurfaces.Triggers {
		if surface.Kind == "http_route" || surface.Kind == "http_server" {
			if surface.ApplicationClass != SurfaceSupportingDependency ||
				surface.TraceReadiness != SurfaceTraceUnsupported || surface.RelatedTraceID != "" {
				t.Fatalf("dependency-derived HTTP behavior was promoted: %#v", surface)
			}
		}
		if surface.Producer == SurfaceProducerCobra {
			if _, wanted := required[surface.Identity.Name]; wanted {
				required[surface.Identity.Name] = true
			}
		}
		if strings.Contains(surface.OwningExecutable, "helpers/build-release-binaries") && surface.ExecutableRole != ExecutableRoleSecondaryTooling {
			t.Fatalf("build-release surface role = %q", surface.ExecutableRole)
		}
	}
	for command, found := range required {
		if !found {
			t.Errorf("saved Restic catalog is missing %q", command)
		}
	}
	for _, trace := range data.ArchitectureCanvas.Flows {
		if trace.StartSurfaceID == "" {
			t.Errorf("saved trace %q command=%q has no deterministic command surface; first steps=%#v", trace.ID, trace.Command, trace.Steps[:min(4, len(trace.Steps))])
		}
	}
	for _, command := range []string{"backup", "check", "init", "restore"} {
		mapped := false
		for _, surface := range data.ArchitectureCanvas.Surfaces {
			if surface.Name == command && surface.OwningComponentID != "" {
				mapped = true
				break
			}
		}
		if !mapped {
			t.Errorf("saved Restic command surface %q has no exact component mapping", command)
		}
	}
	for _, trace := range data.ArchitectureCanvas.Flows {
		if len(trace.ParticipatingComponentIDs) == 0 {
			t.Errorf("saved Restic trace %q has no participating component", trace.ID)
		}
	}
}
