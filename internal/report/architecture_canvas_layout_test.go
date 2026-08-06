package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestArchitectureCanvasLayoutModes(t *testing.T) {
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
const window = {
  RepomapUI: {
    message(id, params) {
      if (params !== undefined && Object.keys(params).length > 0) throw new Error("unexpected params for " + id);
      return id;
    },
  },
  __REPOMAP_LAYOUT_TEST__: {},
};
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), { window });
const api = window.__REPOMAP_LAYOUT_TEST__;
const mode = (groupCount, primaryCount) => api.landscapeLayoutMode({
  groups: Array.from({ length: groupCount }, (_, index) => ({ id: String(index) })),
  primaryRegion: primaryCount == null ? null : {
    groupIDs: Array.from({ length: primaryCount }, (_, index) => String(index)),
  },
});
process.stdout.write(JSON.stringify({
  modes: [mode(5, null), mode(5, 5), mode(5, 3)],
  columns: [1204, 1044, 884, 600].map((width) => api.boardProfileForWidth(width).columns),
  tieOrder: [
    api.shortestColumnIndex([0, 0, 0, 0], [1, 2, 0, 3]),
    api.shortestColumnIndex([10, 20, 5, 30], [1, 2, 0, 3]),
  ],
  childColumns: [[1, 4], [4, 4], [7, 5], [7, 2]].map(([children, columns]) =>
    api.childGridShape(children, columns).columns
  ),
  singleton: [api.childGridShape(1, 4).singleton, api.childGridShape(2, 4).singleton],
  placements: [
    api.shortestCompatiblePlacement([0, 0, 0, 0], 2).column,
    api.shortestCompatiblePlacement([100, 100, 0, 0], 2).column,
    api.shortestCompatiblePlacement([0, 0, 0, 0], 2).column,
  ],
  diagnostics: [
    api.diagnosticSubsystemIDs([{ id: "a", category: "diagnostic" }, { id: "b" }], []),
    api.diagnosticSubsystemIDs([{ id: "a" }, { id: "b" }], [{ code: "proposal.omitted_members_preserved" }]),
  ],
  partialTruth: api.architecturePartialTruth({
    validation_outcome: "accepted_partial",
    local_remainder_component_id: "remainder",
    diagnostics: [{ code: "noisy-generic-diagnostic" }],
    components: [
      { id: "ordinary-diagnostic", members: [{ name: "must not appear" }] },
      { id: "remainder", members: [
        { name: "cmd/server/main.go" },
        { id: { kind: "package", value: "example.test/internal/local" } },
      ] },
    ],
  }),
  fullTruth: api.architecturePartialTruth({
    validation_outcome: "accepted",
    local_remainder_component_id: "remainder",
    components: [{ id: "remainder", members: [{ name: "must not appear" }] }],
  }),
  fitScales: [
    api.readableFitScale({ x: 28, y: 28, width: 1296, height: 1524 }, { width: 1204, height: 718 }, 28),
    api.readableFitScale({ x: 0, y: 0, width: 500, height: 300 }, { width: 1204, height: 718 }, 28),
    // Decision 234 F1: a HUGE landscape must fit entirely inside the
    // viewport — the fit scale clamps only the upper bound, so a
    // 9000px-wide ELK landscape in a 1280px viewport goes below the old
    // 0.16 floor (0.136) instead of clipping ~108px strips per side.
    api.readableFitScale({ x: 0, y: 0, width: 9000, height: 4000 }, { width: 1224, height: 718 }, 28),
  ],
  focusScales: [
    api.componentFocusScale({ x: 0, y: 0, width: 1300, height: 1000 }, { width: 1204, height: 718 }, 56),
    api.componentFocusScale({ x: 0, y: 0, width: 300, height: 132 }, { width: 1204, height: 718 }, 56),
  ],
  transforms: [
    api.centeredTransform({ x: 28, y: 28, width: 1296, height: 1524 }, { width: 1204, height: 718 }, 0.65),
    api.centeredTransform({ x: 28, y: 28, width: 1296, height: 1524 }, { width: 1204, height: 718 }, 0.65),
  ],
  stepStates: [
    api.architectureStepComponentState(
      { participating_component_ids: ["a", "b", "a"] },
      ["a", "b"]
    ),
    api.architectureStepComponentState(
      { participating_component_ids: ["b"] },
      ["a", "b"]
    ),
    api.architectureStepComponentState(
      { component_id: "a", participating_component_ids: ["a", "b"] },
      ["a", "b"]
    ),
  ],
}));
`
	runnerPath := filepath.Join(t.TempDir(), "layout-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run Landscape layout contract: %v\n%s", err, output)
	}
	var result struct {
		Modes        []string   `json:"modes"`
		Columns      []int      `json:"columns"`
		TieOrder     []int      `json:"tieOrder"`
		ChildColumns []int      `json:"childColumns"`
		Singleton    []bool     `json:"singleton"`
		Placements   []int      `json:"placements"`
		Diagnostics  [][]string `json:"diagnostics"`
		PartialTruth *struct {
			RemainderComponentID string `json:"remainderComponentID"`
			Members              []struct {
				Label string `json:"label"`
			} `json:"members"`
		} `json:"partialTruth"`
		FullTruth   any       `json:"fullTruth"`
		FitScales   []float64 `json:"fitScales"`
		FocusScales []float64 `json:"focusScales"`
		Transforms  []struct {
			X     float64 `json:"x"`
			Y     float64 `json:"y"`
			Scale float64 `json:"scale"`
		} `json:"transforms"`
		StepStates []struct {
			Owner        string   `json:"owner"`
			Participants []string `json:"participants"`
			Related      []string `json:"related"`
			Lane         string   `json:"lane"`
			Selection    string   `json:"selection"`
		} `json:"stepStates"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode Landscape layout contract: %v\n%s", err, output)
	}
	if want := []string{"board", "graph", "hybrid"}; !reflect.DeepEqual(result.Modes, want) {
		t.Errorf("layout modes = %v, want %v", result.Modes, want)
	}
	if want := []int{4, 3, 2, 1}; !reflect.DeepEqual(result.Columns, want) {
		t.Errorf("responsive columns = %v, want %v", result.Columns, want)
	}
	if want := []int{1, 2}; !reflect.DeepEqual(result.TieOrder, want) {
		t.Errorf("shortest-column choices = %v, want %v", result.TieOrder, want)
	}
	if want := []int{1, 2, 3, 2}; !reflect.DeepEqual(result.ChildColumns, want) {
		t.Errorf("child-grid columns = %v, want %v", result.ChildColumns, want)
	}
	if want := []bool{true, false}; !reflect.DeepEqual(result.Singleton, want) {
		t.Errorf("singleton projection = %v, want %v", result.Singleton, want)
	}
	if want := []int{0, 2, 0}; !reflect.DeepEqual(result.Placements, want) {
		t.Errorf("compatible placements = %v, want %v", result.Placements, want)
	}
	if want := [][]string{{"a"}, {"b"}}; !reflect.DeepEqual(result.Diagnostics, want) {
		t.Errorf("diagnostic subsystem ids = %v, want %v", result.Diagnostics, want)
	}
	var partialLabels []string
	if result.PartialTruth != nil {
		for _, member := range result.PartialTruth.Members {
			partialLabels = append(partialLabels, member.Label)
		}
	}
	if result.PartialTruth == nil || result.PartialTruth.RemainderComponentID != "remainder" ||
		!reflect.DeepEqual(partialLabels, []string{
			"cmd/server/main.go", "package:example.test/internal/local",
		}) || result.FullTruth != nil {
		t.Errorf("partial Architecture truth projection = %#v / full=%#v", result.PartialTruth, result.FullTruth)
	}
	// Decision 230 D3: Fit shows every principal node inside the viewport
	// with semantic zoom — the scale may drop below the old readable
	// floor (0.65) when the landscape is taller than the viewport; the
	// computed value depends on the fixture bounds (0.434 here).
	// Decision 234 (F1): a huge landscape fits ENTIRELY inside the viewport
	// — the scale clamps only the upper bound, so the 9000px-wide fixture
	// yields 1168/9000 ≈ 0.1298 (below the removed 0.16 floor), not a
	// clipped 0.16 that would leave edge node centers outside the viewport.
	if want := []float64{0.4343832020997375, 1.35, 0.12977777777777778}; !reflect.DeepEqual(result.FitScales, want) {
		t.Errorf("readable Fit scales = %v, want %v", result.FitScales, want)
	}
	if want := []float64{0.88, 1.05}; !reflect.DeepEqual(result.FocusScales, want) {
		t.Errorf("component focus scales = %v, want %v", result.FocusScales, want)
	}
	if len(result.Transforms) != 2 || result.Transforms[0] != result.Transforms[1] {
		t.Errorf("repeated centered transforms differ: %v", result.Transforms)
	}
	if len(result.StepStates) != 3 {
		t.Fatalf("step component states = %#v", result.StepStates)
	}
	multiple := result.StepStates[0]
	if multiple.Owner != "" || multiple.Lane != "__repomap_unassigned__" || multiple.Selection != "" ||
		!reflect.DeepEqual(multiple.Participants, []string{"a", "b"}) ||
		!reflect.DeepEqual(multiple.Related, []string{"a", "b"}) {
		t.Errorf("multiple-participant step chose or duplicated a component: %#v", multiple)
	}
	single := result.StepStates[1]
	if single.Owner != "" || single.Lane != "b" || single.Selection != "b" ||
		!reflect.DeepEqual(single.Participants, []string{"b"}) {
		t.Errorf("single-participant step state = %#v", single)
	}
	owned := result.StepStates[2]
	if owned.Owner != "a" || owned.Lane != "a" || owned.Selection != "a" ||
		!reflect.DeepEqual(owned.Participants, []string{"a", "b"}) ||
		!reflect.DeepEqual(owned.Related, []string{"a", "b"}) {
		t.Errorf("independently owned step state = %#v", owned)
	}
}

func TestArchitectureCanvasGroupsResticLifecycleByTaskRoot(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}

	assetPath, err := filepath.Abs(filepath.Join("templates", "architecture_canvas.js"))
	if err != nil {
		t.Fatal(err)
	}
	fixturePath, err := filepath.Abs(filepath.Join("testdata", "canvas", "restic-backup-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs");
const vm = require("vm");
const messages = {
  "architecture.value.saved_cli_trace": "Saved CLI trace",
  "architecture.value.saved_process_trace": "Saved process trace",
  "architecture.label.saved_trace": "Saved trace",
  "architecture.value.lifecycle_started_by": "Started by",
  "architecture.value.lifecycle_callback": "Callback",
  "architecture.value.lifecycle_cancellation": "Cancellation",
  "architecture.value.lifecycle_join": "Join",
};
function message(id, params) {
  if (!Object.prototype.hasOwnProperty.call(messages, id)) throw new Error("unknown message " + id);
  if (params !== undefined && Object.keys(params).length > 0) throw new Error("unexpected params for " + id);
  return messages[id];
}
const window = {
  RepomapUI: { message },
  __REPOMAP_LAYOUT_TEST__: {},
};
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), { window });
const api = window.__REPOMAP_LAYOUT_TEST__;
const canvas = JSON.parse(fs.readFileSync(process.argv[3], "utf8")).architecture_canvas;
const flow = canvas.flows[0];
const grouped = api.groupLifecycleRelations(flow, canvas.flow_edges);
const fallback = api.groupLifecycleRelations(
  { branches: [] },
  canvas.flow_edges.concat(canvas.flow_edges)
);
process.stdout.write(JSON.stringify({
  archetype: flow.archetype,
  traceLabels: [
    api.savedTraceLabel("cli", message),
    api.savedTraceLabel("process", message),
    api.savedTraceLabel("", message),
  ],
  groups: grouped.groups.map((group) => {
    const relations = {};
    group.relations.forEach((edge) => {
      const label = api.lifecycleRelationHeading(edge, message);
      if (!relations[label]) relations[label] = [];
      relations[label].push(edge.id);
    });
    return { root: group.rootAnchorID, relations };
  }),
  ungroupedTotal: grouped.ungroupedTotal,
  fallbackShown: fallback.ungrouped.length,
  fallbackTotal: fallback.ungroupedTotal,
}));
`
	runnerPath := filepath.Join(t.TempDir(), "lifecycle-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath, fixturePath).CombinedOutput()
	if err != nil {
		t.Fatalf("run lifecycle grouping contract: %v\n%s", err, output)
	}
	var result struct {
		Archetype   string   `json:"archetype"`
		TraceLabels []string `json:"traceLabels"`
		Groups      []struct {
			Root      string              `json:"root"`
			Relations map[string][]string `json:"relations"`
		} `json:"groups"`
		UngroupedTotal int `json:"ungroupedTotal"`
		FallbackShown  int `json:"fallbackShown"`
		FallbackTotal  int `json:"fallbackTotal"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode lifecycle grouping contract: %v\n%s", err, output)
	}
	if result.Archetype != "cli" {
		t.Fatalf("fixture archetype = %q, want cli", result.Archetype)
	}
	if want := []string{"Saved CLI trace", "Saved process trace", "Saved trace"}; !reflect.DeepEqual(result.TraceLabels, want) {
		t.Errorf("saved trace labels = %v, want %v", result.TraceLabels, want)
	}
	if len(result.Groups) != 1 || result.Groups[0].Root != "scanner-task" {
		t.Fatalf("lifecycle groups = %#v, want one scanner task card", result.Groups)
	}
	wantRelations := map[string][]string{
		"Started by":   {"start-scanner"},
		"Callback":     {"run-scanner"},
		"Cancellation": {"cancel-scanner-context", "scanner-uses-context"},
		"Join":         {"join-scanner"},
	}
	if !reflect.DeepEqual(result.Groups[0].Relations, wantRelations) {
		t.Errorf("grouped lifecycle relations = %#v, want %#v", result.Groups[0].Relations, wantRelations)
	}
	if result.UngroupedTotal != 0 {
		t.Errorf("grouped Restic relations left %d ungrouped", result.UngroupedTotal)
	}
	if result.FallbackShown != 6 || result.FallbackTotal != 10 {
		t.Errorf(
			"ungrouped fallback = %d shown/%d total, want 6/10",
			result.FallbackShown,
			result.FallbackTotal,
		)
	}
}
