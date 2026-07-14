package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowproof"
)

// TestSoftServeDaemonV2FixturePreservesConcurrentLifecycle is an MVP fixture
// acceptance test. It is intentionally replaceable with the fixture and does
// not freeze renderer wording, coordinates, or every discovered task.
func TestSoftServeDaemonV2FixturePreservesConcurrentLifecycle(t *testing.T) {
	t.Parallel()

	encoded, err := os.ReadFile(filepath.Join("testdata", "canvas", "soft-serve-daemon-v2.json"))
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
	if canvas == nil || len(canvas.Flows) != 1 {
		t.Fatalf("architecture canvas/flow missing: %#v", canvas)
	}
	flow := canvas.Flows[0]
	if flow.ID != "soft-serve-daemon" {
		t.Fatalf("flow id = %q, want soft-serve-daemon", flow.ID)
	}

	branchKinds := map[string]int{}
	for _, branch := range flow.Branches {
		branchKinds[branch.Kind]++
	}
	if branchKinds["main"] != 1 || branchKinds["shared"] != 1 || branchKinds["task"] < 2 {
		t.Fatalf("branch kinds = %#v, want main, shared, and multiple concrete tasks", branchKinds)
	}

	edges := make(map[string]ArchitectureFlowEdge, len(canvas.FlowEdges))
	for _, edge := range canvas.FlowEdges {
		edges[edge.ID] = edge
	}
	start := edges["start-outer-server-task"]
	if start.Relation != evidence.RelationStartsGoroutine || !start.CrossBranch ||
		start.From != "serve-handler" || start.To != "outer-server-task" {
		t.Fatalf("outer task start = %#v", start)
	}
	cancel := edges["cancel-shutdown-context"]
	if cancel.Relation != evidence.RelationCancels || !cancel.CrossBranch || cancel.To != "shutdown-context" {
		t.Fatalf("shutdown cancellation = %#v", cancel)
	}
	for _, edge := range canvas.FlowEdges {
		if edge.Relation == evidence.RelationJoins && edge.To == "outer-server-task" {
			t.Fatalf("outer task completion was invented as a join: %#v", edge)
		}
	}
	if join := edges["join-shutdown-http"]; join.Relation != evidence.RelationJoins ||
		join.From != "shutdown-wait" || join.To != "shutdown-http-task" {
		t.Fatalf("shutdown join = %#v", join)
	}

	var concurrency, termination flowproof.Slot
	for _, slot := range flow.Slots {
		switch slot.Kind {
		case flowproof.SlotConcurrency:
			concurrency = slot
		case flowproof.SlotTermination:
			termination = slot
		}
	}
	if concurrency.Status != flowproof.SlotPartial || concurrency.Missing == "" {
		t.Fatalf("concurrency slot = %#v, want an explicit partial frontier", concurrency)
	}
	if termination.Status != flowproof.SlotPartial || termination.Missing == "" {
		t.Fatalf("termination slot = %#v, want an explicit partial frontier", termination)
	}
}
