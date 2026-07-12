package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
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
	rendered, err := RenderHTML(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range []string{
		"Zero configured-catalog surfaces is an honest bounded-analysis result",
		"Suggested investigation — not a saved trace",
		"No surfaces matched the configured terminal catalog under this build scenario.",
	} {
		if !strings.Contains(string(rendered), message) {
			t.Fatalf("Caddy report is missing %q", message)
		}
	}
}
