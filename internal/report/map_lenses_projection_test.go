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
// components/surfaces/associations/mechanism-fragments, which component IDs each
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
  // Adversarial saved flows: Decision 242 requires that they do not change
  // Mechanisms objects or emphasis.
  flowEdges: [
    { flow_id: "flow-1", from_component_id: "c1", to_component_id: "c3" },
    { flow_id: "flow-1", from_component_id: "c3", to_component_id: "c1" },
  ],
  mechanismFragments: [
    {
      version: 3, id: "fragment-a", component_ids: ["c1", "c3"],
      entry: {
        claim_kind: "process_entry", support_mode: "resolved_static",
        label: "process entry fixture.main", path: "main.go", line: 10,
        evidence: "exact entry", limitation: "runtime not proven", ordering: "exact_local_order",
      },
      handoffs: [{
        claim_kind: "direct_static_call", support_mode: "resolved_static",
        label: "handoff to Service.Start", path: "main.go", line: 11,
        evidence: "exact handoff", limitation: "runtime not proven", ordering: "resolved_path_order",
      }],
      frontier: { ordering: "not_established", unresolved: ["continuation unknown"], limitation: "continuation unknown" },
    },
    {
      version: 3, id: "fragment-b", component_ids: [],
      entry: {
        claim_kind: "process_entry", support_mode: "resolved_static",
        label: "process entry other.main", path: "other.go", line: 5,
        evidence: "exact entry", limitation: "runtime not proven", ordering: "exact_local_order",
      },
      handoffs: [{
        claim_kind: "direct_static_call", support_mode: "resolved_static",
        label: "handoff to unknown", path: "other.go", line: 6,
        evidence: "exact handoff", limitation: "runtime not proven", ordering: "resolved_path_order",
      }],
      frontier: { ordering: "not_established", limitation: "continuation unknown" },
    },
  ],
};
const landscape = api.mapLensEmphasisProjection({ lens: "landscape", ...input });
const entrypoints = api.mapLensEmphasisProjection({ lens: "entrypoints", ...input });
const integrations = api.mapLensEmphasisProjection({ lens: "integrations", ...input });
const mechanisms = api.mapLensEmphasisProjection({ lens: "mechanisms", ...input });
const structuralEdges = api.mapStructuralEdges({ structural_edges: [
  { id: "import-pair", from_component_id: "c1", to_component_id: "c2", witness_count: 2 },
  { id: "intra", from_component_id: "c1", to_component_id: "c1", witness_count: 1 },
  { id: "unmapped", from_component_ids: ["c1"], to_component_ids: ["c3"], witness_count: 1 },
] });
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
const fragmentOnly = api.projectArchitectureLens({
  architecture_canvas: {
    version: 13,
    components: input.components,
    surfaces: input.surfaces,
    flow_edges: [],
    mechanism_fragments: input.mechanismFragments,
  },
}, "mechanisms");
const unsupportedFragment = api.projectArchitectureLens({
  architecture_canvas: {
    version: 13, components: input.components, surfaces: [], flow_edges: [],
    mechanism_fragments: [{ version: 2, id: "historical", entry: { label: "historical" } }],
  },
}, "mechanisms");
const malformedFragment = api.projectArchitectureLens({
  architecture_canvas: {
    version: 13, components: input.components, surfaces: [], flow_edges: [],
    mechanism_fragments: [{
      version: 3, id: "malformed", component_ids: [],
      entry: {
        claim_kind: "process_entry", support_mode: "resolved_static",
        label: "process entry valid", path: "main.go", line: 10,
        evidence: "exact entry", limitation: "runtime not proven", ordering: "exact_local_order",
      },
      handoffs: [],
      frontier: { ordering: "not_established", limitation: "continuation unknown" },
    }],
  },
}, "mechanisms");
const historicalCanvas = api.projectArchitectureLens({
  architecture_canvas: {
    version: 12, components: input.components, surfaces: [], flow_edges: [],
    mechanism_fragments: input.mechanismFragments,
  },
}, "mechanisms");
process.stdout.write(JSON.stringify({ landscape, entrypoints, integrations, mechanisms, structuralEdges, full, fragmentOnly, unsupportedFragment, malformedFragment, historicalCanvas }));
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
		Landscape       lensResult `json:"landscape"`
		Entrypoints     lensResult `json:"entrypoints"`
		Integrations    lensResult `json:"integrations"`
		Mechanisms      lensResult `json:"mechanisms"`
		StructuralEdges []struct {
			ID           string `json:"id"`
			WitnessCount int    `json:"witness_count"`
		} `json:"structuralEdges"`
		Full                fullLens `json:"full"`
		FragmentOnly        fullLens `json:"fragmentOnly"`
		UnsupportedFragment fullLens `json:"unsupportedFragment"`
		MalformedFragment   fullLens `json:"malformedFragment"`
		HistoricalCanvas    fullLens `json:"historicalCanvas"`
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
	if len(got.Mechanisms.Objects.Mechanisms) != 2 {
		t.Fatalf("mechanisms objects = %#v, want the two exact Canvas fragments", got.Mechanisms.Objects.Mechanisms)
	}
	if len(got.StructuralEdges) != 1 || got.StructuralEdges[0].ID != "import-pair" || got.StructuralEdges[0].WitnessCount != 2 {
		t.Fatalf("Map structural edges = %#v, want the singular cross-component aggregate", got.StructuralEdges)
	}
	// projectArchitectureLens(reportData, lens): the DOM-free entry point —
	// normalizes the nested association view-model and returns counts.
	if join(got.Full.Emphasized) != "c1,c2" {
		t.Fatalf("full projection emphasized = %#v, want [c1 c2]", got.Full.Emphasized)
	}
	if got.Full.Counts.Touchpoints != 2 || got.Full.Counts.Components != 3 {
		t.Fatalf("full projection counts = %#v, want components=3 touchpoints=2", got.Full.Counts)
	}
	// Decision 242: Canvas v13 fragments are the sole Mechanisms authority.
	// Exact component IDs are emphasized; a second unjoined fragment remains
	// visible without guessing. Empty flow_edges are ordinary, and the
	// adversarial flow rows above cannot change this result.
	if join(got.FragmentOnly.Emphasized) != "c1,c3" || got.FragmentOnly.Dimmed != 1 {
		t.Fatalf("fragment-only emphasis = %#v dimmed=%d, want exact [c1 c3] with only c2 dimmed",
			got.FragmentOnly.Emphasized, got.FragmentOnly.Dimmed)
	}
	if got.FragmentOnly.Counts.Mechanisms != 2 || len(got.FragmentOnly.Mechanisms) != 2 {
		t.Fatalf("fragment-only mechanisms = %#v counts=%#v, want two", got.FragmentOnly.Mechanisms, got.FragmentOnly.Counts)
	}
	fragment := got.FragmentOnly.Mechanisms[0]
	if fragment.Kind != "mechanism_fragment" || fragment.Fragment.Version != 3 ||
		fragment.Fragment.Entry.Label != "process entry fixture.main" || join(fragment.ComponentIDs) != "c1,c3" {
		t.Fatalf("fragment-only object = %#v, want exact v3 fragment and backend IDs [c1 c3]", fragment)
	}
	if got.UnsupportedFragment.Counts.Mechanisms != 0 || len(got.UnsupportedFragment.Mechanisms) != 0 {
		t.Fatalf("unsupported fragment projected = %#v", got.UnsupportedFragment.Mechanisms)
	}
	if got.MalformedFragment.Counts.Mechanisms != 0 || len(got.MalformedFragment.Mechanisms) != 0 {
		t.Fatalf("malformed current fragment projected = %#v", got.MalformedFragment.Mechanisms)
	}
	if got.HistoricalCanvas.Counts.Mechanisms != 0 || len(got.HistoricalCanvas.Mechanisms) != 0 {
		t.Fatalf("historical Canvas projected current mechanism fragments = %#v", got.HistoricalCanvas.Mechanisms)
	}
}

type fullLens struct {
	Lens       string   `json:"lens"`
	Visible    []string `json:"visible"`
	Emphasized []string `json:"emphasized"`
	Dimmed     int      `json:"dimmed"`
	Mechanisms []struct {
		Kind         string   `json:"kind"`
		ComponentIDs []string `json:"component_ids"`
		Fragment     struct {
			Version int `json:"version"`
			Entry   struct {
				Label string `json:"label"`
			} `json:"entry"`
		} `json:"fragment"`
	} `json:"mechanisms"`
	Counts struct {
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
