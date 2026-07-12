package report

import (
	"os"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
)

func TestReplaySavedCaddyRun(t *testing.T) {
	runDir := os.Getenv("REPOMAP_SAVED_CADDY_RUN")
	if runDir == "" {
		t.Skip("set REPOMAP_SAVED_CADDY_RUN to exercise the owner-provided offline run")
	}
	data, err := ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	canvas := data.ArchitectureCanvas
	if canvas == nil || canvas.ArchitectureSource != componentmap.SourceValidatedModel || canvas.Fallback {
		t.Fatalf("Caddy architecture source = %#v", canvas)
	}
	want := []string{"Core", "Config", "Admin", "HTTP", "Security", "Entry"}
	primary := make([]ArchitectureSubsystem, 0, len(canvas.Subsystems))
	for _, subsystem := range canvas.Subsystems {
		if subsystem.Category != componentmap.SubsystemCategoryDiagnostic {
			primary = append(primary, subsystem)
		}
	}
	if len(primary) != len(want) {
		t.Fatalf("Caddy primary subsystems = %d, want %d", len(primary), len(want))
	}
	for index, name := range want {
		if primary[index].Name != name {
			t.Fatalf("Caddy subsystem[%d] = %q, want %q", index, primary[index].Name, name)
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
