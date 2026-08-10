package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The Map lens projection is pure: it consumes only backend-owned Canvas and
// association view-model fields and never performs a browser-side grounding
// join. Canvas 15 entry_handoff_groups are Entrypoints context, not a lens.
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
    { id: "c-remainder", name: "Local remainder" },
  ],
  localRemainderComponentID: "c-remainder",
  surfaces: [
    { id: "surf-a", surface_role: "entry_surface", kind: "cli_command", name: "run", participating_component_ids: ["c1"] },
    { id: "surf-b", surface_role: "entry_surface", kind: "http_route", name: "GET /api", participating_component_ids: ["c2"] },
    { id: "surf-zero", surface_role: "entry_surface", kind: "process_entry", name: "zero.main", participating_component_ids: [],
      evidence: [{ path: "cmd/zero/main.go", line: 7, column: 6 }] },
    { id: "surf-off-map", surface_role: "entry_surface", kind: "scheduled_job", name: "sweep", participating_component_ids: ["c-outside"] },
    // Runtime activities remain useful elsewhere, but are not user entrypoints.
    { id: "surf-runtime", surface_role: "runtime_activity", kind: "worker", name: "internal worker", participating_component_ids: ["c3"] },
  ],
  associations: [
    { component_id: "c1", family: "database", owning_unit: "pkg/store", imported_family: "github.com/jackc/pgx/v5", kind: "boundary", observation_count: 2, paired: true,
      witnesses: [
        { path: "store.go", line: 20, symbol: "Query", role: "production" },
        { path: "store.go", line: 30, symbol: "Exec", role: "production" },
      ] },
    { component_id: "c1", family: "database", owning_unit: "pkg/store", imported_family: "github.com/jackc/pgx/v5", kind: "resource", observation_count: 1, paired: true,
      witnesses: [{ path: "store.go", line: 20, symbol: "Query", role: "production" }] },
    // Same component/family, different owning unit is a separate touchpoint.
    { component_id: "c1", family: "database", owning_unit: "pkg/archive", imported_family: "github.com/jackc/pgx/v5", kind: "boundary", observation_count: 1, paired: false,
      witnesses: [{ path: "archive.go", line: 8, symbol: "Archive", role: "production" }] },
    { component_id: "c2", family: "cache-lock", owning_unit: "pkg/cache", imported_family: "github.com/redis/go-redis/v9", kind: "boundary", observation_count: 1, paired: false,
      witnesses: [{ path: "cache.go", line: 11, symbol: "Lock", role: "production" }] },
    // Association evidence can outlive a component's inclusion in the bounded map.
    { component_id: "c-outside", family: "broker/pub-sub", owning_unit: "pkg/events", imported_family: "example.test/broker", kind: "resource", observation_count: 1, paired: false,
      witnesses: [{ path: "publish.go", line: 9, symbol: "Publish", role: "production" }] },
    // The local remainder is present in Canvas storage but is not a visible
    // principal; its exact evidence must survive as off-map.
    { component_id: "c-remainder", family: "filesystem", owning_unit: "pkg/tmp", imported_family: "os", kind: "resource", observation_count: 1, paired: false,
      witnesses: [{ path: "tmp.go", line: 12, symbol: "CreateTemp", role: "production" }] },
    // Local operation/surface rows must not masquerade as Integrations.
    { component_id: "c3", family: "internal-operation", owning_unit: "pkg/internal", imported_family: "example.test/internal", kind: "operation", observation_count: 7, paired: false },
    { component_id: "c3", family: "runtime-surface", owning_unit: "pkg/http", imported_family: "example.test/http", kind: "surface", observation_count: 5, paired: false },
  ],
  entryHandoffGroups: [
    {
      version: 2, id: "entry-group-a", component_ids: ["c1", "c3"],
      entry: transition("main.go", 10, 6, "fixture.main", null, ["c1"]),
      entry_handoffs: [transition("main.go", 11, 9, "fixture.Service.Start", {
        label: "fixture.Service.Start", path: "service.go", line: 20, column: 4,
      }, ["c3"])],
      frontier: { ordering: "not_established", limitation: "continuation unknown" },
    },
    {
      version: 2, id: "entry-group-b", component_ids: [],
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
const unownedOnly = api.mapLensEmphasisProjection({
  lens: "entrypoints",
  components: input.components.map((component) => ({ id: component.id, name: component.name })),
  localRemainderComponentID: input.localRemainderComponentID,
  surfaces: [input.surfaces[2]],
  associations: [], entryHandoffGroups: [],
});
const integrations = api.mapLensEmphasisProjection({ lens: "integrations", ...input });
const removedMechanisms = api.mapLensEmphasisProjection({ lens: "mechanisms", ...input });
const structuralEdges = api.mapStructuralEdges({ structural_edges: [
  { id: "import-pair", from_component_id: "c1", to_component_id: "c2", witness_count: 2 },
  { id: "intra", from_component_id: "c1", to_component_id: "c1", witness_count: 1 },
  { id: "unmapped", from_component_ids: ["c1"], to_component_ids: ["c3"], witness_count: 1 },
] });
const full = api.projectArchitectureLens({
  architecture_canvas: { components: input.components, surfaces: input.surfaces, local_remainder_component_id: input.localRemainderComponentID },
  architecture_associations: {
    version: 1, total: 8,
    components: [
      { component_id: "c1", name: "C1", associations: input.associations.filter((a) => a.component_id === "c1") },
      { component_id: "c2", name: "C2", associations: input.associations.filter((a) => a.component_id === "c2") },
      { component_id: "c3", name: "C3", associations: input.associations.filter((a) => a.component_id === "c3") },
      { component_id: "c-outside", name: "Outside", associations: input.associations.filter((a) => a.component_id === "c-outside") },
      { component_id: "c-remainder", name: "Local remainder", associations: input.associations.filter((a) => a.component_id === "c-remainder") },
    ],
  },
}, "integrations");
const groupOnly = api.projectArchitectureLens({
  architecture_canvas: {
    version: 15, components: input.components, surfaces: input.surfaces,
    local_remainder_component_id: input.localRemainderComponentID,
    entry_handoff_groups: input.entryHandoffGroups,
  },
}, "entrypoints");
const unsupportedCanvas = api.projectArchitectureLens({
  architecture_canvas: {
    version: 13, components: input.components, surfaces: [],
    local_remainder_component_id: input.localRemainderComponentID,
    entry_handoff_groups: input.entryHandoffGroups,
  },
}, "entrypoints");
const malformedGroup = api.projectArchitectureLens({
  architecture_canvas: {
    version: 15, components: input.components, surfaces: [],
    local_remainder_component_id: input.localRemainderComponentID,
    entry_handoff_groups: [{
      version: 2, id: "malformed", component_ids: ["c1"],
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
  version: 2, id: "entry-group-mixed", component_ids: ["c1"],
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
  version: 2, id: "entry-group-plural", component_ids: ["c1", "c2", "c3"],
  entry: transition("main.go", 10, 6, "fixture.main", null, ["c2", "c1"]),
  entry_handoffs: [transition("main.go", 14, 8, "fixture.plural", {
    label: "fixture.plural", path: "plural.go", line: 20, column: 5,
  }, ["c3", "c2"])],
  frontier: { ordering: "not_established", limitation: "continuation unknown" },
};
const pluralOverlay = api.entryHandoffOverlayProjection(
  pluralOverlayGroup, input.components.map((item) => item.id)
);
const ageLikeGroup = {
  version: 2, id: "entry-group-age-like", component_ids: ["c1", "c2"],
  entry: transition("cmd/age/age.go", 105, 6, "fixture.main", null, ["c1"]),
  entry_handoffs: Array.from({ length: 13 }, (_, index) => transition(
    "cmd/age/age.go", 170 + index, 4, "fixture.local" + index,
    { label: "fixture.local" + index, path: "cmd/age/age.go", line: 300 + index, column: 6 },
    ["c1"]
  )).concat([transition(
    "cmd/age/age.go", 253, 5, "fixture.term.IsTerminal",
    { label: "fixture.term.IsTerminal", path: "internal/term/term.go", line: 120, column: 6 },
    ["c2"]
  )]),
  frontier: { ordering: "not_established", limitation: "continuation unknown" },
};
const ageLikeOverlay = api.entryHandoffOverlayProjection(
  ageLikeGroup, input.components.map((item) => item.id)
);
const casdoorLikeGroup = {
  version: 2, id: "entry-group-casdoor-like", component_ids: ["c1", "c2"],
  entry: transition("main.go", 36, 6, "fixture.main", null, ["c1"]),
  entry_handoffs: Array.from({ length: 19 }, (_, index) => transition(
    "main.go", 130 + index, 2, "fixture.http.Register" + index,
    { label: "fixture.http.Register" + index, path: "http/register" + index + ".go", line: 20 + index, column: 6 },
    ["c2"]
  )),
  frontier: { ordering: "not_established", limitation: "continuation unknown" },
};
const casdoorLikeOverlay = api.entryHandoffOverlayProjection(
  casdoorLikeGroup, input.components.map((item) => item.id)
);
const fromBox = { x: 10, y: 20, width: 180, height: 100 };
const toBox = { x: 300, y: 40, width: 180, height: 100 };
const boxesBefore = JSON.stringify([fromBox, toBox]);
const crossGeometry = api.entryHandoffConnectionGeometry(fromBox, toBox, 0);
const boxesAfter = JSON.stringify([fromBox, toBox]);
process.stdout.write(JSON.stringify({
  landscape, entrypoints, unownedOnly, integrations, removedMechanisms, structuralEdges,
  full, groupOnly, unsupportedCanvas, malformedGroup, overlay, mixedOverlay,
  noEntryOverlay, pluralOverlay, ageLikeOverlay, casdoorLikeOverlay,
  crossGeometry, boxesUnchanged: boxesBefore === boxesAfter,
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
		UnownedOnly       lensResult `json:"unownedOnly"`
		Integrations      lensResult `json:"integrations"`
		RemovedMechanisms lensResult `json:"removedMechanisms"`
		StructuralEdges   []struct {
			ID           string `json:"id"`
			WitnessCount int    `json:"witness_count"`
		} `json:"structuralEdges"`
		Full               fullLens                  `json:"full"`
		GroupOnly          fullLens                  `json:"groupOnly"`
		UnsupportedCanvas  fullLens                  `json:"unsupportedCanvas"`
		MalformedGroup     fullLens                  `json:"malformedGroup"`
		Overlay            entryHandoffOverlayResult `json:"overlay"`
		MixedOverlay       entryHandoffOverlayResult `json:"mixedOverlay"`
		NoEntryOverlay     entryHandoffOverlayResult `json:"noEntryOverlay"`
		PluralOverlay      entryHandoffOverlayResult `json:"pluralOverlay"`
		AgeLikeOverlay     entryHandoffOverlayResult `json:"ageLikeOverlay"`
		CasdoorLikeOverlay entryHandoffOverlayResult `json:"casdoorLikeOverlay"`
		CrossGeometry      struct {
			Path string `json:"path"`
		} `json:"crossGeometry"`
		BoxesUnchanged bool `json:"boxesUnchanged"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode lens projection result: %v\n%s", err, output)
	}
	if len(got.Landscape.Emphasized) != 0 {
		t.Fatalf("landscape emphasized = %#v, want none", got.Landscape.Emphasized)
	}
	if join(got.Entrypoints.Emphasized) != "c1,c2,c3" ||
		join(got.Entrypoints.Participants.VisibleComponentIDs) != "c1,c2,c3" ||
		join(got.Entrypoints.Participants.OffMapComponentIDs) != "c-outside" {
		t.Fatalf("entrypoint participants = %#v, want visible and off-map exact joins", got.Entrypoints)
	}
	if len(got.Entrypoints.Objects.Entrypoints) != 4 ||
		entryCount(got.Entrypoints.Objects.Entrypoints) != 4 ||
		len(got.Entrypoints.Objects.EntryHandoffGroups) != 2 ||
		containsEntrypoint(got.Entrypoints.Objects.Entrypoints, "surf-runtime") {
		t.Fatalf("entrypoint objects = %#v, want only four entry_surface records and two exact contexts", got.Entrypoints.Objects)
	}
	offMapEntry := findEntrypoint(got.Entrypoints.Objects.Entrypoints, "surf-off-map")
	if offMapEntry == nil || len(offMapEntry.ComponentIDs) != 0 || len(offMapEntry.VisibleComponentIDs) != 0 ||
		join(offMapEntry.OffMapComponentIDs) != "c-outside" {
		t.Fatalf("off-map entrypoint distinction = %#v", offMapEntry)
	}
	if len(got.UnownedOnly.Emphasized) != 0 || len(got.UnownedOnly.Objects.Entrypoints) != 1 {
		t.Fatalf("unowned exact entry = %#v, want visible with no component emphasis", got.UnownedOnly)
	}
	if join(got.Integrations.Emphasized) != "c1,c2" ||
		join(got.Integrations.Participants.VisibleComponentIDs) != "c1,c2" ||
		join(got.Integrations.Participants.OffMapComponentIDs) != "c-outside,c-remainder" ||
		len(got.Integrations.Objects.Touchpoints) != 5 {
		t.Fatalf("integrations = %#v, want boundary/resource participants partitioned by map visibility", got.Integrations)
	}
	paired := findTouchpoint(got.Integrations.Objects.Touchpoints, "c1", "database", "pkg/store")
	if paired == nil || paired.Kind != "boundary" || join(paired.Kinds) != "boundary,resource" ||
		!paired.Paired || paired.ObservationCount != 3 || paired.WitnessCount != 2 ||
		len(paired.Witnesses) != 2 || join(paired.ComponentIDs) != "c1" ||
		len(paired.OffMapComponentIDs) != 0 {
		t.Fatalf("paired database touchpoint = %#v, want one exact coalesced object", paired)
	}
	if findTouchpoint(got.Integrations.Objects.Touchpoints, "c1", "database", "pkg/archive") == nil {
		t.Fatalf("same component/family across owning units was incorrectly collapsed: %#v", got.Integrations.Objects.Touchpoints)
	}
	offMap := findTouchpoint(got.Integrations.Objects.Touchpoints, "c-outside", "broker/pub-sub", "pkg/events")
	if offMap == nil || len(offMap.ComponentIDs) != 0 || len(offMap.VisibleComponentIDs) != 0 ||
		join(offMap.OffMapComponentIDs) != "c-outside" {
		t.Fatalf("off-map touchpoint distinction = %#v", offMap)
	}
	remainder := findTouchpoint(got.Integrations.Objects.Touchpoints, "c-remainder", "filesystem", "pkg/tmp")
	if remainder == nil || len(remainder.VisibleComponentIDs) != 0 ||
		join(remainder.OffMapComponentIDs) != "c-remainder" {
		t.Fatalf("local remainder leaked into visible principals or lost its evidence: %#v", remainder)
	}
	if got.RemovedMechanisms.Lens != "landscape" || len(got.RemovedMechanisms.Emphasized) != 0 {
		t.Fatalf("removed mechanisms lens = %#v, want landscape fallback", got.RemovedMechanisms)
	}
	if len(got.StructuralEdges) != 1 || got.StructuralEdges[0].ID != "import-pair" || got.StructuralEdges[0].WitnessCount != 2 {
		t.Fatalf("Map structural edges = %#v, want the singular cross-component aggregate", got.StructuralEdges)
	}
	if join(got.Full.Emphasized) != "c1,c2" || got.Full.Counts.Touchpoints != 5 ||
		got.Full.Counts.Components != 3 || got.Full.Dimmed != 0 ||
		join(got.Full.Visible) != "c1,c2,c3" ||
		join(got.Full.Participants.OffMapComponentIDs) != "c-outside,c-remainder" {
		t.Fatalf("full integration projection = %#v", got.Full)
	}
	if join(got.GroupOnly.Emphasized) != "c1,c2,c3" || got.GroupOnly.Dimmed != 0 ||
		got.GroupOnly.Counts.EntryHandoffGroups != 2 || len(got.GroupOnly.EntryHandoffGroups) != 2 ||
		got.GroupOnly.Counts.Entries != 4 || got.GroupOnly.Omissions.UnjoinedSurfaces != 2 ||
		join(got.GroupOnly.Participants.OffMapComponentIDs) != "c-outside" {
		t.Fatalf("Canvas15 Entrypoints context = %#v", got.GroupOnly)
	}
	group := got.GroupOnly.EntryHandoffGroups[0]
	if group.Kind != "entry_handoff_group" || group.Group.Version != 2 ||
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
		got.Overlay.Edges[0].ToComponentID != "c3" ||
		len(got.Overlay.Overflow) != 0 {
		t.Fatalf("exact selected-entry overlay = %#v", got.Overlay)
	}
	if len(got.MixedOverlay.Edges) != 0 || len(got.MixedOverlay.Overflow) != 2 ||
		got.MixedOverlay.Overflow[0].Reason != "same_component" ||
		got.MixedOverlay.Overflow[1].Reason != "target_unjoined" {
		t.Fatalf("same-component/unjoined overflow = %#v", got.MixedOverlay)
	}
	if len(got.NoEntryOverlay.Overflow) != 1 || got.NoEntryOverlay.Overflow[0].Reason != "entry_unjoined" {
		t.Fatalf("unjoined entry overlay = %#v", got.NoEntryOverlay)
	}
	if len(got.PluralOverlay.Edges) != 0 ||
		len(got.PluralOverlay.Overflow) != 1 || got.PluralOverlay.Overflow[0].Reason != "entry_target_plural" ||
		join(got.PluralOverlay.ComponentIDs) != "c2,c1,c3" {
		t.Fatalf("plural exact owners were collapsed, sorted-first, or Cartesian-expanded: %#v", got.PluralOverlay)
	}
	if len(got.AgeLikeOverlay.Edges) != 1 || len(got.AgeLikeOverlay.Overflow) != 13 {
		t.Fatalf("Age-like overlay = %#v, want one cross-cube arrow and 13 compact side rows", got.AgeLikeOverlay)
	}
	for index, item := range got.AgeLikeOverlay.Overflow {
		if item.Reason != "same_component" {
			t.Fatalf("Age-like overflow[%d] reason = %q, want same_component", index, item.Reason)
		}
	}
	if len(got.CasdoorLikeOverlay.Edges) != 1 || got.CasdoorLikeOverlay.Edges[0].CallCount != 19 ||
		len(got.CasdoorLikeOverlay.Edges[0].Handoffs) != 19 || len(got.CasdoorLikeOverlay.Overflow) != 19 {
		t.Fatalf("Casdoor-like component-pair aggregation = %#v, want one 19-call edge and 19 exact context rows",
			got.CasdoorLikeOverlay)
	}
	for index, item := range got.CasdoorLikeOverlay.Overflow {
		if item.Reason != "parallel_component_pair" || item.Handoff.Path != "main.go" ||
			item.Handoff.Line != 130+index {
			t.Fatalf("Casdoor-like exact row[%d] = %#v", index, item)
		}
	}
	if got.CasdoorLikeOverlay.Overflow[0].Handoff.Target.Path != "http/register0.go" ||
		got.CasdoorLikeOverlay.Overflow[18].Handoff.Target.Path != "http/register18.go" {
		t.Fatalf("Casdoor-like exact callee rows lost source order: %#v", got.CasdoorLikeOverlay.Overflow)
	}
	if !got.BoxesUnchanged || got.CrossGeometry.Path == "" {
		t.Fatalf("overlay geometry mutated layout or lost cross route: cross=%#v unchanged=%t",
			got.CrossGeometry, got.BoxesUnchanged)
	}
}

func TestArchitectureCanvasMapLensCSSKeepsLandscapeNeutral(t *testing.T) {
	asset, err := os.ReadFile(filepath.Join("templates", "architecture_canvas.css"))
	if err != nil {
		t.Fatal(err)
	}
	css := string(asset)
	for _, forbidden := range []string{
		`[data-lens-has-emphasis="true"][data-lens="entrypoints"]`,
		`[data-lens-has-emphasis="true"][data-lens="integrations"]`,
	} {
		if strings.Contains(css, forbidden) {
			t.Fatalf("Map lens globally dims the neutral landscape through %q", forbidden)
		}
	}
	if !strings.Contains(css, `.rm-arch__is-entry-handoff-participant .rm-arch__component-card`) {
		t.Fatal("explicit entry handoff selection lost its focused participant treatment")
	}
}

type entryHandoffOverlayResult struct {
	ComponentIDs []string                  `json:"component_ids"`
	Edges        []entryHandoffOverlayEdge `json:"edges"`
	Overflow     []struct {
		Reason  string `json:"reason"`
		Handoff struct {
			Line   int    `json:"line"`
			Path   string `json:"path"`
			Target struct {
				Path string `json:"path"`
			} `json:"target"`
		} `json:"handoff"`
	} `json:"overflow"`
}

type entryHandoffOverlayEdge struct {
	FromComponentID string `json:"from_component_id"`
	ToComponentID   string `json:"to_component_id"`
	CallCount       int    `json:"call_count"`
	Handoffs        []struct {
		Path string `json:"path"`
		Line int    `json:"line"`
	} `json:"handoffs"`
}

type fullLens struct {
	Lens               string                  `json:"lens"`
	Visible            []string                `json:"visible"`
	Emphasized         []string                `json:"emphasized"`
	Participants       lensParticipants        `json:"participants"`
	Dimmed             int                     `json:"dimmed"`
	Touchpoints        []integrationTouchpoint `json:"touchpoints"`
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
	Lens         string           `json:"lens"`
	Emphasized   []string         `json:"emphasized"`
	Participants lensParticipants `json:"participants"`
	Objects      struct {
		Entrypoints        []entrypointCategory    `json:"entrypoints"`
		Touchpoints        []integrationTouchpoint `json:"touchpoints"`
		EntryHandoffGroups []json.RawMessage       `json:"entry_handoff_groups"`
	} `json:"objects"`
}

type lensParticipants struct {
	VisibleComponentIDs []string `json:"visible_component_ids"`
	OffMapComponentIDs  []string `json:"off_map_component_ids"`
}

type entrypointCategory struct {
	Kind    string                `json:"kind"`
	Entries []entrypointLensEntry `json:"entries"`
}

type entrypointLensEntry struct {
	ID                  string   `json:"id"`
	ComponentIDs        []string `json:"component_ids"`
	VisibleComponentIDs []string `json:"visible_component_ids"`
	OffMapComponentIDs  []string `json:"off_map_component_ids"`
}

type integrationTouchpoint struct {
	ComponentID      string   `json:"component_id"`
	Family           string   `json:"family"`
	OwningUnit       string   `json:"owning_unit"`
	Kind             string   `json:"kind"`
	Kinds            []string `json:"kinds"`
	Paired           bool     `json:"paired"`
	ObservationCount int      `json:"observation_count"`
	WitnessCount     int      `json:"witness_count"`
	Witnesses        []struct {
		Path   string `json:"path"`
		Line   int    `json:"line"`
		Symbol string `json:"symbol"`
		Role   string `json:"role"`
	} `json:"witnesses"`
	ComponentIDs        []string `json:"component_ids"`
	VisibleComponentIDs []string `json:"visible_component_ids"`
	OffMapComponentIDs  []string `json:"off_map_component_ids"`
}

func entryCount(categories []entrypointCategory) int {
	total := 0
	for _, category := range categories {
		total += len(category.Entries)
	}
	return total
}

func containsEntrypoint(categories []entrypointCategory, id string) bool {
	return findEntrypoint(categories, id) != nil
}

func findEntrypoint(categories []entrypointCategory, id string) *entrypointLensEntry {
	for _, category := range categories {
		for index := range category.Entries {
			if category.Entries[index].ID == id {
				return &category.Entries[index]
			}
		}
	}
	return nil
}

func findTouchpoint(touchpoints []integrationTouchpoint, componentID, family, owningUnit string) *integrationTouchpoint {
	for index := range touchpoints {
		if touchpoints[index].ComponentID == componentID && touchpoints[index].Family == family &&
			touchpoints[index].OwningUnit == owningUnit {
			return &touchpoints[index]
		}
	}
	return nil
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
