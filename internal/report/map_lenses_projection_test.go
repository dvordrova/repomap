package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The Map lens projection is pure: it consumes only backend-owned Canvas and
// association view-model fields and never performs a browser-side grounding
// join. Canvas 14 entry_handoff_groups are Entrypoints context, not a lens.
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
function transition(path, line, column, symbol, target, componentIDs) {
  return {
    claim_kind: target ? "direct_static_call" : "process_entry",
    support_mode: "resolved_static", label: symbol, path, line, column, symbol,
    component_ids: componentIDs || [],
    evidence: "saved evidence", limitation: "runtime not proven", ordering: "exact_local_order",
    target,
  };
}
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
    // Local operation/surface rows must not masquerade as Integrations.
    { component_id: "c3", family: "internal-operation", imported_family: "example.test/internal", kind: "operation", observation_count: 7, paired: false },
    { component_id: "c3", family: "runtime-surface", imported_family: "example.test/http", kind: "surface", observation_count: 5, paired: false },
  ],
  entryHandoffGroups: [
    {
      version: 1, id: "entry-group-a", component_ids: ["c1", "c3"],
      entry: transition("main.go", 10, 6, "fixture.main", null, ["c1"]),
      entry_handoffs: [transition("main.go", 11, 9, "fixture.Service.Start", {
        label: "fixture.Service.Start", path: "service.go", line: 20, column: 4,
      }, ["c3"])],
      frontier: { ordering: "not_established", limitation: "continuation unknown" },
    },
    {
      version: 1, id: "entry-group-b", component_ids: [],
      entry: transition("worker.go", 5, 6, "fixture.worker.main", null, []),
      entry_handoffs: [transition("worker.go", 6, 3, "fixture.client.Send", {
        label: "fixture.client.Send", path: "client.go", line: 30, column: 2,
      }, [])],
      frontier: { ordering: "not_established", limitation: "continuation unknown" },
    },
  ],
};
const landscape = api.mapLensEmphasisProjection({ lens: "landscape", ...input });
const entrypoints = api.mapLensEmphasisProjection({ lens: "entrypoints", ...input });
const integrations = api.mapLensEmphasisProjection({ lens: "integrations", ...input });
const removedMechanisms = api.mapLensEmphasisProjection({ lens: "mechanisms", ...input });
const structuralEdges = api.mapStructuralEdges({ structural_edges: [
  { id: "import-pair", from_component_id: "c1", to_component_id: "c2", witness_count: 2 },
  { id: "intra", from_component_id: "c1", to_component_id: "c1", witness_count: 1 },
  { id: "unmapped", from_component_ids: ["c1"], to_component_ids: ["c3"], witness_count: 1 },
] });
const full = api.projectArchitectureLens({
  architecture_canvas: { components: input.components, surfaces: input.surfaces },
  architecture_associations: {
    version: 1, total: 4,
    components: [
      { component_id: "c1", name: "C1", associations: input.associations.filter((a) => a.component_id === "c1") },
      { component_id: "c2", name: "C2", associations: input.associations.filter((a) => a.component_id === "c2") },
      { component_id: "c3", name: "C3", associations: input.associations.filter((a) => a.component_id === "c3") },
    ],
  },
}, "integrations");
const groupOnly = api.projectArchitectureLens({
  architecture_canvas: {
    version: 14, components: input.components, surfaces: input.surfaces,
    entry_handoff_groups: input.entryHandoffGroups,
  },
}, "entrypoints");
const unsupportedCanvas = api.projectArchitectureLens({
  architecture_canvas: {
    version: 13, components: input.components, surfaces: [],
    entry_handoff_groups: input.entryHandoffGroups,
  },
}, "entrypoints");
const malformedGroup = api.projectArchitectureLens({
  architecture_canvas: {
    version: 14, components: input.components, surfaces: [],
    entry_handoff_groups: [{
      version: 1, id: "malformed", component_ids: ["c1"],
      entry: transition("main.go", 10, 6, "fixture.main", null, ["c1"]),
      entry_handoffs: [],
      frontier: { ordering: "not_established", limitation: "continuation unknown" },
    }],
  },
}, "entrypoints");
const overlay = api.entryHandoffOverlayProjection(
  input.entryHandoffGroups[0], input.components.map((item) => item.id)
);
const mixedOverlayGroup = {
  version: 1, id: "entry-group-mixed", component_ids: ["c1"],
  entry: transition("main.go", 10, 6, "fixture.main", null, ["c1"]),
  entry_handoffs: [
    transition("main.go", 11, 9, "fixture.local", {
      label: "fixture.local", path: "main.go", line: 30, column: 4,
    }, ["c1"]),
    transition("main.go", 12, 7, "fixture.unjoined", {
      label: "fixture.unjoined", path: "other.go", line: 20, column: 3,
    }, ["unknown-component"]),
  ],
  frontier: { ordering: "not_established", limitation: "continuation unknown" },
};
const mixedOverlay = api.entryHandoffOverlayProjection(
  mixedOverlayGroup, input.components.map((item) => item.id)
);
const noEntryOverlay = api.entryHandoffOverlayProjection(
  input.entryHandoffGroups[1], input.components.map((item) => item.id)
);
const pluralOverlayGroup = {
  version: 1, id: "entry-group-plural", component_ids: ["c1", "c2", "c3"],
  entry: transition("main.go", 10, 6, "fixture.main", null, ["c2", "c1"]),
  entry_handoffs: [transition("main.go", 14, 8, "fixture.plural", {
    label: "fixture.plural", path: "plural.go", line: 20, column: 5,
  }, ["c3", "c2"])],
  frontier: { ordering: "not_established", limitation: "continuation unknown" },
};
const pluralOverlay = api.entryHandoffOverlayProjection(
  pluralOverlayGroup, input.components.map((item) => item.id)
);
const fromBox = { x: 10, y: 20, width: 180, height: 100 };
const toBox = { x: 300, y: 40, width: 180, height: 100 };
const boxesBefore = JSON.stringify([fromBox, toBox]);
const crossGeometry = api.entryHandoffConnectionGeometry(fromBox, toBox, 0);
const loopGeometry = api.entryHandoffConnectionGeometry(fromBox, fromBox, 0);
const boxesAfter = JSON.stringify([fromBox, toBox]);
process.stdout.write(JSON.stringify({
  landscape, entrypoints, integrations, removedMechanisms, structuralEdges,
  full, groupOnly, unsupportedCanvas, malformedGroup, overlay, mixedOverlay,
  noEntryOverlay, pluralOverlay,
  crossGeometry, loopGeometry, boxesUnchanged: boxesBefore === boxesAfter,
}));
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
		Landscape         lensResult `json:"landscape"`
		Entrypoints       lensResult `json:"entrypoints"`
		Integrations      lensResult `json:"integrations"`
		RemovedMechanisms lensResult `json:"removedMechanisms"`
		StructuralEdges   []struct {
			ID           string `json:"id"`
			WitnessCount int    `json:"witness_count"`
		} `json:"structuralEdges"`
		Full              fullLens                  `json:"full"`
		GroupOnly         fullLens                  `json:"groupOnly"`
		UnsupportedCanvas fullLens                  `json:"unsupportedCanvas"`
		MalformedGroup    fullLens                  `json:"malformedGroup"`
		Overlay           entryHandoffOverlayResult `json:"overlay"`
		MixedOverlay      entryHandoffOverlayResult `json:"mixedOverlay"`
		NoEntryOverlay    entryHandoffOverlayResult `json:"noEntryOverlay"`
		PluralOverlay     entryHandoffOverlayResult `json:"pluralOverlay"`
		CrossGeometry     struct {
			Path     string `json:"path"`
			SelfLoop bool   `json:"self_loop"`
		} `json:"crossGeometry"`
		LoopGeometry struct {
			Path     string `json:"path"`
			SelfLoop bool   `json:"self_loop"`
		} `json:"loopGeometry"`
		BoxesUnchanged bool `json:"boxesUnchanged"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode lens projection result: %v\n%s", err, output)
	}
	if len(got.Landscape.Emphasized) != 0 {
		t.Fatalf("landscape emphasized = %#v, want none", got.Landscape.Emphasized)
	}
	if join(got.Entrypoints.Emphasized) != "c1,c3" {
		t.Fatalf("entrypoints emphasized = %#v, want surface + exact handoff participants", got.Entrypoints.Emphasized)
	}
	if len(got.Entrypoints.Objects.Entrypoints) != 2 || len(got.Entrypoints.Objects.EntryHandoffGroups) != 2 {
		t.Fatalf("entrypoint objects = %#v, want two kinds and two exact contexts", got.Entrypoints.Objects)
	}
	if join(got.Integrations.Emphasized) != "c1,c2" || len(got.Integrations.Objects.Touchpoints) != 2 {
		t.Fatalf("integrations = %#v, want only boundary/resource participants", got.Integrations)
	}
	if got.RemovedMechanisms.Lens != "landscape" || len(got.RemovedMechanisms.Emphasized) != 0 {
		t.Fatalf("removed mechanisms lens = %#v, want landscape fallback", got.RemovedMechanisms)
	}
	if len(got.StructuralEdges) != 1 || got.StructuralEdges[0].ID != "import-pair" || got.StructuralEdges[0].WitnessCount != 2 {
		t.Fatalf("Map structural edges = %#v, want the singular cross-component aggregate", got.StructuralEdges)
	}
	if join(got.Full.Emphasized) != "c1,c2" || got.Full.Counts.Touchpoints != 2 || got.Full.Counts.Components != 3 {
		t.Fatalf("full integration projection = %#v", got.Full)
	}
	if join(got.GroupOnly.Emphasized) != "c1,c3" || got.GroupOnly.Dimmed != 1 ||
		got.GroupOnly.Counts.EntryHandoffGroups != 2 || len(got.GroupOnly.EntryHandoffGroups) != 2 {
		t.Fatalf("Canvas14 Entrypoints context = %#v", got.GroupOnly)
	}
	group := got.GroupOnly.EntryHandoffGroups[0]
	if group.Kind != "entry_handoff_group" || group.Group.Version != 1 ||
		group.Group.Entry.Symbol != "fixture.main" || join(group.ComponentIDs) != "c1,c3" {
		t.Fatalf("entry handoff group = %#v", group)
	}
	if got.UnsupportedCanvas.Counts.EntryHandoffGroups != 0 || len(got.UnsupportedCanvas.EntryHandoffGroups) != 0 {
		t.Fatalf("historical Canvas projected current entry context = %#v", got.UnsupportedCanvas)
	}
	if got.MalformedGroup.Counts.EntryHandoffGroups != 0 || len(got.MalformedGroup.EntryHandoffGroups) != 0 {
		t.Fatalf("malformed entry context projected = %#v", got.MalformedGroup)
	}
	if len(got.Overlay.Edges) != 1 || got.Overlay.Edges[0].FromComponentID != "c1" ||
		got.Overlay.Edges[0].ToComponentID != "c3" || len(got.Overlay.SelfLoops) != 0 ||
		len(got.Overlay.Overflow) != 0 {
		t.Fatalf("exact selected-entry overlay = %#v", got.Overlay)
	}
	if len(got.MixedOverlay.Edges) != 0 || len(got.MixedOverlay.SelfLoops) != 1 ||
		got.MixedOverlay.SelfLoops[0].FromComponentID != "c1" || len(got.MixedOverlay.Overflow) != 1 ||
		got.MixedOverlay.Overflow[0].Reason != "target_unjoined" {
		t.Fatalf("self-loop/unjoined overlay = %#v", got.MixedOverlay)
	}
	if len(got.NoEntryOverlay.Overflow) != 1 || got.NoEntryOverlay.Overflow[0].Reason != "entry_unjoined" {
		t.Fatalf("unjoined entry overlay = %#v", got.NoEntryOverlay)
	}
	if len(got.PluralOverlay.Edges) != 0 || len(got.PluralOverlay.SelfLoops) != 0 ||
		len(got.PluralOverlay.Overflow) != 1 || got.PluralOverlay.Overflow[0].Reason != "entry_target_plural" ||
		join(got.PluralOverlay.ComponentIDs) != "c2,c1,c3" {
		t.Fatalf("plural exact owners were collapsed, sorted-first, or Cartesian-expanded: %#v", got.PluralOverlay)
	}
	if !got.BoxesUnchanged || got.CrossGeometry.SelfLoop || got.CrossGeometry.Path == "" ||
		!got.LoopGeometry.SelfLoop || got.LoopGeometry.Path == "" {
		t.Fatalf("overlay geometry mutated layout or lost route: cross=%#v loop=%#v unchanged=%t",
			got.CrossGeometry, got.LoopGeometry, got.BoxesUnchanged)
	}
}

type entryHandoffOverlayResult struct {
	ComponentIDs []string                  `json:"component_ids"`
	Edges        []entryHandoffOverlayEdge `json:"edges"`
	SelfLoops    []entryHandoffOverlayEdge `json:"self_loops"`
	Overflow     []struct {
		Reason string `json:"reason"`
	} `json:"overflow"`
}

type entryHandoffOverlayEdge struct {
	FromComponentID string `json:"from_component_id"`
	ToComponentID   string `json:"to_component_id"`
}

type fullLens struct {
	Lens               string   `json:"lens"`
	Visible            []string `json:"visible"`
	Emphasized         []string `json:"emphasized"`
	Dimmed             int      `json:"dimmed"`
	EntryHandoffGroups []struct {
		Kind         string   `json:"kind"`
		ComponentIDs []string `json:"component_ids"`
		Group        struct {
			Version int `json:"version"`
			Entry   struct {
				Symbol string `json:"symbol"`
			} `json:"entry"`
		} `json:"group"`
	} `json:"entry_handoff_groups"`
	Counts struct {
		Components         int `json:"components"`
		Surfaces           int `json:"surfaces"`
		Entries            int `json:"entries"`
		Touchpoints        int `json:"touchpoints"`
		EntryHandoffGroups int `json:"entry_handoff_groups"`
	} `json:"counts"`
	Omissions struct {
		UnjoinedSurfaces int `json:"unjoined_surfaces"`
	} `json:"omissions"`
}

type lensResult struct {
	Lens       string   `json:"lens"`
	Emphasized []string `json:"emphasized"`
	Objects    struct {
		Entrypoints        []json.RawMessage `json:"entrypoints"`
		Touchpoints        []json.RawMessage `json:"touchpoints"`
		EntryHandoffGroups []json.RawMessage `json:"entry_handoff_groups"`
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
