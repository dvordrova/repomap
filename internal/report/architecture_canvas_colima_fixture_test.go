package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowproof"
)

// TestColimaRuntimeV2FixturePreservesBranching is a fixture acceptance test.
// It protects the semantic cases for which Colima was selected; it is not a
// golden test for renderer wording or coordinates.
func TestColimaRuntimeV2FixturePreservesBranching(t *testing.T) {
	t.Parallel()

	encoded, err := os.ReadFile(filepath.Join("testdata", "canvas", "colima-runtime-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		ArchitectureCanvas *ArchitectureCanvas `json:"architecture_canvas"`
	}
	if err := json.Unmarshal(encoded, &saved); err != nil {
		t.Fatal(err)
	}
	canvas := saved.ArchitectureCanvas
	if canvas == nil {
		t.Fatal("architecture_canvas is missing")
	}
	const savedLandscapeVersion = componentmap.ContractVersion
	if canvas.Version != ArchitectureCanvasVersion ||
		canvas.LandscapeVersion != savedLandscapeVersion ||
		canvas.FlowProofVersion != flowproof.Version {
		t.Fatalf(
			"canvas versions = %d/%d/%d, want %d/%d/%d",
			canvas.Version, canvas.LandscapeVersion, canvas.FlowProofVersion,
			ArchitectureCanvasVersion, savedLandscapeVersion, flowproof.Version,
		)
	}
	if got := len(canvas.Subsystems); got != 4 {
		t.Fatalf("subsystems = %d, want 4", got)
	}
	if got := len(canvas.Components); got != 10 {
		t.Fatalf("components = %d, want 10", got)
	}
	if len(canvas.Flows) != 1 || canvas.Flows[0].ID != "colima-start" {
		t.Fatalf("flows = %#v, want colima-start", canvas.Flows)
	}

	flow := canvas.Flows[0]
	if len(flow.Branches) != 1 || flow.Branches[0].Kind != "main" {
		t.Fatalf("branches = %#v, want one synchronous main branch", flow.Branches)
	}
	for _, step := range flow.Steps {
		if step.Binding == nil || step.ComponentID == "" {
			t.Fatalf("step %q lost its exact member/component binding: %#v", step.ID, step)
		}
		if step.Binding.FlowID != flow.ID || step.Binding.AnchorID != step.ID {
			t.Fatalf("step %q has mismatched binding %#v", step.ID, step.Binding)
		}
	}

	edges := make(map[string]ArchitectureFlowEdge, len(canvas.FlowEdges))
	for _, edge := range canvas.FlowEdges {
		edges[edge.ID] = edge
	}
	selection := edges["select-runtime-factory"]
	if selection.Resolution != evidence.ResolutionUnresolved ||
		selection.From != "new-container" || selection.To != "runtime-factory-dispatch" {
		t.Fatalf("runtime selection = %#v, want explicit unresolved map dispatch", selection)
	}
	for _, edgeID := range []string{
		"candidate-docker-factory",
		"candidate-containerd-factory",
		"candidate-kubernetes-factory",
		"dispatch-docker-start",
		"dispatch-containerd-start",
		"dispatch-kubernetes-start",
	} {
		edge, ok := edges[edgeID]
		if !ok || edge.Resolution != evidence.ResolutionFrameworkRule || edge.Certainty != evidence.CertaintyPossible {
			t.Fatalf("registered alternative %q = %#v, want possible framework-resolved edge", edgeID, edge)
		}
	}
	vz := edges["select-vz-driver"]
	if vz.Condition == nil ||
		vz.Condition.Expression != "util.MacOS13OrNewer() && conf.VMType == VZ && sameArchitecture" ||
		vz.Evidence.Path != "environment/vm/lima/yaml.go" || vz.Evidence.Line != 30 {
		t.Fatalf("VZ branch = %#v, want the exact guarded source branch", vz)
	}
	qemu := edges["select-qemu-driver"]
	if qemu.Condition != nil || qemu.Evidence.Line != 25 {
		t.Fatalf("QEMU default = %#v, want the unconditional default assignment", qemu)
	}
	limaStart := edges["call-lima-start"]
	if limaStart.Resolution != evidence.ResolutionTypeInferred || limaStart.Provider != "go_types" {
		t.Fatalf("VM interface dispatch = %#v, want a type-inferred Lima target", limaStart)
	}
	for _, forbidden := range [][2]string{
		{"docker-provision", "docker-start"},
		{"containerd-provision", "containerd-start"},
		{"kubernetes-provision", "kubernetes-start"},
	} {
		for _, edge := range canvas.FlowEdges {
			if edge.From == forbidden[0] && edge.To == forbidden[1] {
				t.Fatalf("source order was promoted to a call edge: %#v", edge)
			}
		}
	}

	var unresolvedFrontier bool
	for _, frontier := range canvas.Frontiers {
		if frontier.Kind == "unresolved_transition" && frontier.TransitionID == "select-runtime-factory" {
			unresolvedFrontier = true
		}
	}
	if !unresolvedFrontier {
		t.Fatalf("frontiers = %#v, want the dynamic runtime dispatch frontier", canvas.Frontiers)
	}
	var concurrency flowproof.Slot
	for _, slot := range flow.Slots {
		if slot.Kind == flowproof.SlotConcurrency {
			concurrency = slot
		}
	}
	if concurrency.Status != flowproof.SlotNotApplicable ||
		concurrency.ApplicabilityReason != flowproof.ApplicabilityNoConcurrentLifecycleInScope ||
		len(concurrency.Provenance) == 0 {
		t.Fatalf("concurrency slot = %#v, want witnessed not_applicable", concurrency)
	}

	if len(canvas.StructuralFacts) != 11 || len(canvas.StructuralEdges) != 10 {
		t.Fatalf(
			"structural projection = %d facts/%d edges, want 11 exact facts/10 cross-component edges",
			len(canvas.StructuralFacts), len(canvas.StructuralEdges),
		)
	}
	for _, edge := range canvas.StructuralEdges {
		witness := edge.Witness
		if witness.Kind != componentmap.StructuralRelationPackageImport ||
			witness.From.Kind != componentmap.MemberPackage || witness.To.Kind != componentmap.MemberPackage ||
			len(witness.Provenance) == 0 || len(witness.Scenarios) != 1 {
			t.Fatalf("structural edge lost its exact package witness: %#v", edge)
		}
	}
}
