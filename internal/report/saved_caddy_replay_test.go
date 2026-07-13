package report

import (
	"os"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
)

func TestReplaySavedCaddyRun(t *testing.T) {
	runDir := os.Getenv("REPOMAP_SAVED_CADDY_RUN")
	if runDir == "" {
		t.Skip("set REPOMAP_SAVED_CADDY_RUN to exercise the owner-provided model-backed run")
	}
	data, err := ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	canvas := data.ArchitectureCanvas
	if canvas == nil || canvas.Fallback ||
		(canvas.ArchitectureSource != componentmap.SourceValidatedModel && canvas.ArchitectureSource != componentmap.SourceNormalizedModel) {
		input, buildErr := BuildArchitectureCanvasInput(data)
		saved, readErr := os.ReadFile(runDir + "/" + ArchitectureSynthesisFile)
		_, replayErr := ReplayArchitectureSynthesis(input, saved)
		t.Fatalf("Caddy architecture source = %#v; build=%v read=%v replay=%v", canvas, buildErr, readErr, replayErr)
	}
	if len(canvas.Subsystems) == 0 || len(canvas.Components) == 0 {
		t.Fatal("Caddy architecture replay is empty")
	}
	if data.DiscoveredSurfaces == nil || len(data.DiscoveredSurfaces.Triggers) != 0 || len(canvas.Surfaces) != 0 {
		t.Fatalf("Caddy gained unsupported surfaces: catalog=%#v canvas=%#v", data.DiscoveredSurfaces, canvas.Surfaces)
	}
	if len(canvas.Suggestions) != 3 {
		t.Fatalf("Caddy suggestions = %#v, want three accepted untraced directions", canvas.Suggestions)
	}
	for _, suggestion := range canvas.Suggestions {
		if len(suggestion.RelevantComponentIDs) == 0 {
			t.Errorf("suggestion %q has no exact component mapping", suggestion.ID)
		}
		switch suggestion.ID {
		case "admin-api-request-handling", "cli-command-dispatch":
			if !suggestion.InvestigationAvailable || len(suggestion.RelevantAnchorIDs) == 0 {
				t.Errorf("anchored suggestion %q = %#v", suggestion.ID, suggestion)
			}
		case "http-request-handling":
			if !suggestion.InvestigationAvailable || suggestion.StartLocation == nil ||
				suggestion.CanStartTrace || suggestion.TraceUnavailableReason == "" {
				t.Errorf("member-only HTTP suggestion = %#v", suggestion)
			}
		}
	}
	for _, component := range canvas.Components {
		if component.Name == "Other locally known members" && len(component.SuggestedInvestigationIDs) > 0 {
			t.Fatalf("diagnostic remainder received suggestions: %v", component.SuggestedInvestigationIDs)
		}
	}
	if out := os.Getenv("REPOMAP_REPLAY_HTML"); out != "" {
		html, err := RenderHTML(data)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(out, html, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote replay HTML: %s", out)
	}
}
