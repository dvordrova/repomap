package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestDevRenderSavedRunForUIReview renders a saved run's report.json with the
// current templates into /tmp for browser-based UI acceptance (Decision 217).
// Provider-free: reads only the saved artifacts; never writes back into the
// run directory. It mirrors the report server's parse path (report.json is
// the persisted canonical projection; the synthesis record is re-validated
// only when the run is replayed end-to-end).
func TestDevRenderSavedRunForUIReview(t *testing.T) {
	runDir := os.Getenv("D217_RUN_DIR")
	if runDir == "" {
		t.Skip("D217_RUN_DIR unset")
	}
	out := os.Getenv("D217_OUT")
	if out == "" {
		out = filepath.Join(os.TempDir(), "d217-rendered.html")
	}
	raw, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatalf("read report.json: %v", err)
	}
	var data ReportData
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	html, err := RenderHTML(&data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if err := os.WriteFile(out, html, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Logf("rendered %d bytes to %s", len(html), out)
}
