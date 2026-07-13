package orient

import (
	"context"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/llmbundle"
	"github.com/dvordrova/repomap/internal/snapshot"
)

func TestLocalFlowProofKeepsTraceOutsideInitialProviderBundle(t *testing.T) {
	trace := gofacts.CommandTrace{
		Version:           gofacts.CommandTraceVersion,
		Framework:         "cobra",
		EntrypointPackage: "example.com/project/cmd/app",
		Command:           "backup",
		Steps: []gofacts.CommandTraceStep{
			{Symbol: "main", Relation: "entrypoint", TargetLocation: evidence.Location{Path: "cmd/app/main.go", Line: 3}},
			{Symbol: "newRoot", Relation: "calls", TargetLocation: evidence.Location{Path: "cmd/app/root.go", Line: 5}},
			{Symbol: "newBackup", Relation: "registers_command", TargetLocation: evidence.Location{Path: "cmd/app/backup.go", Line: 7}},
			{Symbol: "runBackup", Relation: "callback", TargetLocation: evidence.Location{Path: "cmd/app/run.go", Line: 9}},
		},
		HandlerCalls: []gofacts.CommandTraceCall{{
			Symbol: "archive", Path: "cmd/app/run.go", Line: 12, Relation: "calls",
			Resolved: true, TargetPath: "pkg/z.go", TargetLine: 21,
		}},
	}
	facts := &gofacts.Facts{CommandTraces: []gofacts.CommandTrace{trace}}
	snap := snapshot.Snapshot{RepoName: "project", GoFacts: facts}
	bundle := llmbundle.Build(snap, []string{
		"cmd/app/main.go", "cmd/app/root.go", "cmd/app/backup.go", "cmd/app/run.go",
		"pkg/z.go",
	}, llmbundle.Options{MaxFiles: 1})
	if len(bundle.Go.CommandTraces) != 0 {
		t.Fatalf("provider command traces = %#v, want filtered provider copy", bundle.Go.CommandTraces)
	}

	report := orientationPart{CandidateFlows: []flowexplain.CandidateFlow{{
		Name: "Backup command", Trigger: "CLI backup command", LikelyEntrypoint: "cmd/app/main.go",
	}}}
	attachLocalFlowProofs(context.Background(), t.TempDir(), &report, localFlowProofInput(snap))

	proof := report.CandidateFlows[0].LocalProof
	if proof == nil {
		t.Fatal("local proof is nil; provider truncation must not remove the full local trace")
	}
	foundOutsideInitialBundle := false
	for _, anchor := range proof.Proof.Anchors {
		if anchor.Location != nil && anchor.Location.Path == "pkg/z.go" {
			foundOutsideInitialBundle = true
			break
		}
	}
	if !foundOutsideInitialBundle {
		t.Fatalf("local proof anchors = %#v, want exact local target omitted from initial provider paths", proof.Proof.Anchors)
	}
	for _, providerPath := range bundle.ProviderAllowedPaths {
		if providerPath == "pkg/z.go" {
			t.Fatalf("test precondition failed: local target was initially provider-visible: %v", bundle.ProviderAllowedPaths)
		}
	}
}
