package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowproof"
	"github.com/dvordrova/repomap/internal/report"
)

func TestLoadPreviewHTMLUsesSavedArchitectureCanvas(t *testing.T) {
	fixturePath := filepath.Join(t.TempDir(), "report.json")
	data, err := json.Marshal(report.ReportData{
		FormatVersion: 5,
		RepoName:      "saved-fixture",
		ProjectGuess:  "offline canvas preview",
		ArchitectureCanvas: &report.ArchitectureCanvas{
			Version: report.ArchitectureCanvasVersion,
			Flows: []report.ArchitectureFlow{{
				ID: "saved-flow", Name: "Saved flow",
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixturePath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	html, err := loadPreviewHTML(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	output := string(html)
	for _, expected := range []string{
		"saved-fixture",
		"saved-flow",
		"rm-report-data",
		"rm-architecture-canvas-js",
		"RepomapArchitectureCanvas",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("preview HTML does not contain %q", expected)
		}
	}
}

func TestLoadPreviewHTMLRejectsMissingOrStaleCanvas(t *testing.T) {
	tests := []struct {
		name string
		data report.ReportData
		want string
	}{
		{name: "missing", data: report.ReportData{}, want: "no architecture_canvas"},
		{
			name: "stale",
			data: report.ReportData{ArchitectureCanvas: &report.ArchitectureCanvas{
				Version: report.ArchitectureCanvasVersion + 1,
			}},
			want: "version",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixturePath := filepath.Join(t.TempDir(), "report.json")
			encoded, err := json.Marshal(test.data)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(fixturePath, encoded, 0o600); err != nil {
				t.Fatal(err)
			}
			_, err = loadPreviewHTML(fixturePath)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("loadPreviewHTML() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestResticBackupV2FixturePreservesAuditedTopology(t *testing.T) {
	t.Parallel()

	fixturePath := filepath.Join("..", "..", defaultFixture)
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	var saved report.ReportData
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	canvas := saved.ArchitectureCanvas
	if canvas == nil {
		t.Fatal("architecture_canvas is missing")
	}
	if canvas.Version != 6 || canvas.LandscapeVersion != 6 || canvas.FlowProofVersion != 3 {
		t.Fatalf("canvas versions = %d/%d/%d", canvas.Version, canvas.LandscapeVersion, canvas.FlowProofVersion)
	}
	if len(canvas.Components) != 9 || len(canvas.Subsystems) != 4 {
		t.Fatalf("landscape = %d components in %d subsystems, want 9 in 4", len(canvas.Components), len(canvas.Subsystems))
	}
	if len(canvas.Flows) != 1 || canvas.Flows[0].ID != "restic-backup" {
		t.Fatalf("flows = %#v, want the restic Backup flow", canvas.Flows)
	}

	flow := canvas.Flows[0]
	branches := make(map[string]report.ArchitectureFlowBranch, len(flow.Branches))
	for _, branch := range flow.Branches {
		branches[branch.Kind] = branch
	}
	mainBranch, hasMain := branches["main"]
	taskBranch, hasTask := branches["task"]
	sharedBranch, hasShared := branches["shared"]
	if !hasMain || !hasTask || !hasShared || taskBranch.RootAnchorID != "scanner-task" {
		t.Fatalf("branches = %#v, want distinct main, scanner task, and shared context branches", flow.Branches)
	}
	if contains(mainBranch.AnchorIDs, "scanner-scan") || !contains(taskBranch.AnchorIDs, "scanner-scan") {
		t.Fatalf("Scanner.Scan must exist only on task branch: main=%v task=%v", mainBranch.AnchorIDs, taskBranch.AnchorIDs)
	}
	if !contains(sharedBranch.AnchorIDs, "scanner-context") {
		t.Fatalf("scanner context is not explicit shared state: %v", sharedBranch.AnchorIDs)
	}

	edges := make(map[string]report.ArchitectureFlowEdge, len(canvas.FlowEdges))
	for _, edge := range canvas.FlowEdges {
		edges[edge.ID] = edge
		if edge.Relation == evidence.RelationKind("source_order") {
			t.Fatalf("source order was promoted to a call edge: %#v", edge)
		}
	}
	cancel := edges["cancel-scanner-context"]
	join := edges["join-scanner"]
	if cancel.Relation != evidence.RelationCancels || cancel.To != "scanner-context" {
		t.Fatalf("cancel edge = %#v, want cancellation target", cancel)
	}
	if join.Relation != evidence.RelationJoins || join.To != "scanner-task" {
		t.Fatalf("join edge = %#v, want scanner task", join)
	}
	if cancel.To == "wait-scanner" || cancel.ID == join.ID {
		t.Fatalf("cancel and join were conflated: cancel=%#v join=%#v", cancel, join)
	}

	var terminationFrontier bool
	for _, frontier := range canvas.Frontiers {
		if frontier.Slot == flowproof.SlotTermination && strings.Contains(frontier.Reason, "process termination") {
			terminationFrontier = true
		}
	}
	if !terminationFrontier {
		t.Fatalf("frontiers = %#v, want explicit process-termination frontier", canvas.Frontiers)
	}

	canvas.Version = report.ArchitectureCanvasVersion
	canvas.LandscapeVersion = componentmap.ContractVersion
	canvas.FlowProofVersion = flowproof.Version
	currentData, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	currentFixturePath := filepath.Join(t.TempDir(), "restic-backup-v2.json")
	if err := os.WriteFile(currentFixturePath, currentData, 0o600); err != nil {
		t.Fatal(err)
	}
	html, err := loadPreviewHTML(currentFixturePath)
	if err != nil {
		t.Fatal(err)
	}
	output := string(html)
	for _, expected := range []string{"restic-backup", "starts_goroutine", "join-scanner", "Architecture"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("preview HTML does not contain %q", expected)
		}
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
