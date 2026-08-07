package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Decision 236 (v11), owner corrective: the lens vertical is separated into
// (A) a PURE projection test — no DOM, no fake Element, no mount, no
// geometry — and (B) a real-browser product test (see BROWSER_ACCEPTANCE).
// This test verifies the pure projection only: given backend-owned
// components/surfaces/associations/flow-edges, which component IDs each
// lens emphasizes and which first-class objects it makes visible.
func TestArchitectureCanvasMapLensPureProjection(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	assetPath, err := filepath.Abs(filepath.Join("templates", "architecture_canvas.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs");
const vm = require("vm");
const window = { __REPOMAP_LAYOUT_TEST__: {} };
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), { window });
const api = window.__REPOMAP_LAYOUT_TEST__;
const input = {
  components: [
    { id: "c1", name: "C1", participating_surface_ids: ["surf-a"] },
    { id: "c2", name: "C2" },
    { id: "c3", name: "C3" },
  ],
  surfaces: [
    { id: "surf-a", kind: "cli_command", name: "run", participating_component_ids: ["c1"] },
    { id: "surf-b", kind: "http_route", name: "GET /api", participating_component_ids: ["c2"] },
  ],
  associations: [
    { component_id: "c1", family: "database", imported_family: "github.com/jackc/pgx/v5", kind: "boundary", observation_count: 2, paired: true },
    { component_id: "c2", family: "cache-lock", imported_family: "github.com/redis/go-redis/v9", kind: "boundary", observation_count: 1, paired: false },
  ],
  flowEdges: [
    { flow_id: "flow-1", from_component_id: "c1", to_component_id: "c3" },
    { flow_id: "flow-1", from_component_id: "c3", to_component_id: "c1" },
  ],
};
const landscape = api.mapLensEmphasisProjection({ lens: "landscape", ...input });
const entrypoints = api.mapLensEmphasisProjection({ lens: "entrypoints", ...input });
const integrations = api.mapLensEmphasisProjection({ lens: "integrations", ...input });
const mechanisms = api.mapLensEmphasisProjection({ lens: "mechanisms", ...input });
const full = api.projectArchitectureLens({
  architecture_canvas: {
    components: input.components,
    surfaces: input.surfaces,
    flow_edges: input.flowEdges,
  },
  architecture_associations: {
    version: 1,
    total: 2,
    components: [
      { component_id: "c1", name: "C1", associations: input.associations.filter((a) => a.component_id === "c1") },
      { component_id: "c2", name: "C2", associations: input.associations.filter((a) => a.component_id === "c2") },
    ],
  },
}, "integrations");
process.stdout.write(JSON.stringify({ landscape, entrypoints, integrations, mechanisms, full }));
`
	runnerPath := filepath.Join(t.TempDir(), "map-lens-projection-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run lens pure projection: %v\n%s", err, output)
	}
	var got struct {
		Landscape    lensResult `json:"landscape"`
		Entrypoints  lensResult `json:"entrypoints"`
		Integrations lensResult `json:"integrations"`
		Mechanisms   lensResult `json:"mechanisms"`
		Full         fullLens   `json:"full"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode lens projection result: %v\n%s", err, output)
	}
	if len(got.Landscape.Emphasized) != 0 {
		t.Fatalf("landscape emphasized = %#v, want none", got.Landscape.Emphasized)
	}
	if join(got.Entrypoints.Emphasized) != "c1" {
		t.Fatalf("entrypoints emphasized = %#v, want [c1] (only c1 owns an entry surface)", got.Entrypoints.Emphasized)
	}
	// Entry objects must be visible as first-class catalog entries grouped
	// by kind — not merely inferred from highlighted components.
	if len(got.Entrypoints.Objects.Entrypoints) != 2 {
		t.Fatalf("entrypoints objects = %#v, want 2 kind groups", got.Entrypoints.Objects.Entrypoints)
	}
	if join(got.Integrations.Emphasized) != "c1,c2" {
		t.Fatalf("integrations emphasized = %#v, want [c1 c2]", got.Integrations.Emphasized)
	}
	if len(got.Integrations.Objects.Touchpoints) != 2 {
		t.Fatalf("integrations touchpoint objects = %#v, want 2", got.Integrations.Objects.Touchpoints)
	}
	if join(got.Mechanisms.Emphasized) != "c1,c3" {
		t.Fatalf("mechanisms emphasized = %#v, want [c1 c3]", got.Mechanisms.Emphasized)
	}
	if len(got.Mechanisms.Objects.Mechanisms) != 1 {
		t.Fatalf("mechanisms flow objects = %#v, want 1 connected flow", got.Mechanisms.Objects.Mechanisms)
	}
	// projectArchitectureLens(reportData, lens): the DOM-free entry point —
	// normalizes the nested association view-model and returns counts.
	if join(got.Full.Emphasized) != "c1,c2" {
		t.Fatalf("full projection emphasized = %#v, want [c1 c2]", got.Full.Emphasized)
	}
	if got.Full.Counts.Touchpoints != 2 || got.Full.Counts.Components != 3 {
		t.Fatalf("full projection counts = %#v, want components=3 touchpoints=2", got.Full.Counts)
	}
}

type fullLens struct {
	Lens       string   `json:"lens"`
	Visible    []string `json:"visible"`
	Emphasized []string `json:"emphasized"`
	Dimmed     int      `json:"dimmed"`
	Counts     struct {
		Components  int `json:"components"`
		Surfaces    int `json:"surfaces"`
		Entries     int `json:"entries"`
		Touchpoints int `json:"touchpoints"`
		Mechanisms  int `json:"mechanisms"`
	} `json:"counts"`
	Omissions struct {
		UnjoinedSurfaces int `json:"unjoined_surfaces"`
	} `json:"omissions"`
}

type lensResult struct {
	Lens       string   `json:"lens"`
	Emphasized []string `json:"emphasized"`
	Objects    struct {
		Entrypoints []json.RawMessage `json:"entrypoints"`
		Touchpoints []json.RawMessage `json:"touchpoints"`
		Mechanisms  []json.RawMessage `json:"mechanisms"`
	} `json:"objects"`
}

func join(values []string) string {
	out := ""
	for index, value := range values {
		if index > 0 {
			out += ","
		}
		out += value
	}
	return out
}
