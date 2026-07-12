package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRenderInputScopedFreshnessFixture(t *testing.T) {
	fixture := filepath.Join("testdata", "canvas", "input-scoped-freshness-v3.json")
	encoded, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	var data ReportData
	if err := json.Unmarshal(encoded, &data); err != nil {
		t.Fatal(err)
	}
	html, err := RenderHTML(&data)
	if err != nil {
		t.Fatal(err)
	}
	if out := os.Getenv("REPOMAP_FRESHNESS_REPLAY_HTML"); out != "" {
		if err := os.WriteFile(out, html, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote freshness replay HTML: %s", out)
	}
}
