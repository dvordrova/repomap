package orient

import (
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/flowproof"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/llmbundle"
	"github.com/dvordrova/repomap/internal/snapshot"
)

func TestApplyOrientationConfidenceGateCapsIncompleteCLIFlow(t *testing.T) {
	t.Parallel()

	bundle := llmbundle.Build(snapshot.Snapshot{
		RepoName: "tool",
		Readme:   "Run tool backup to save files.",
		GoFacts: &gofacts.Facts{
			Modules: []gofacts.ModuleFact{{ModulePath: "example.com/tool", ModuleDir: "."}},
			EntrypointPackages: []gofacts.Entrypoint{{
				ImportPath: "example.com/tool/cmd/tool",
				PackageDir: "cmd/tool",
				Kind:       "cli",
				GoFiles:    []string{"main.go", "backup.go"},
				Anchors: []gofacts.EntrypointAnchor{{
					Version: gofacts.EntrypointAnchorVersion,
					Kind:    gofacts.EntrypointAnchorGoMain,
					Path:    "cmd/tool/main.go",
					Line:    10,
				}},
			}},
			CommandTraces: []gofacts.CommandTrace{{
				Version:           gofacts.CommandTraceVersion,
				Framework:         "cobra",
				EntrypointPackage: "example.com/tool/cmd/tool",
				Command:           "backup",
				Complete:          true,
				Steps: []gofacts.CommandTraceStep{
					{Symbol: "main", Relation: "entrypoint", TargetLocation: evidence.Location{Path: "cmd/tool/main.go", Line: 10}},
					{Symbol: "newRootCommand", Relation: "calls", CallsiteLocation: confidenceLocation("cmd/tool/main.go", 11), TargetLocation: evidence.Location{Path: "cmd/tool/main.go", Line: 20}},
					{Symbol: "newBackupCommand", Relation: "registers_command", CallsiteLocation: confidenceLocation("cmd/tool/main.go", 25), TargetLocation: evidence.Location{Path: "cmd/tool/backup.go", Line: 5}},
					{Symbol: "runBackup", Relation: "callback", CallsiteLocation: confidenceLocation("cmd/tool/backup.go", 12), TargetLocation: evidence.Location{Path: "cmd/tool/backup.go", Line: 30}},
				},
				HandlerCalls: []gofacts.CommandTraceCall{{
					Symbol: "repo.Save", Path: "cmd/tool/backup.go", Line: 40, Relation: "calls",
				}},
			}},
		},
	}, []string{"cmd/tool/main.go", "cmd/tool/backup.go"}, llmbundle.Options{MaxFiles: 20})

	report := orientationPart{
		ProjectGuess: "backup tool",
		Confidence:   0.95,
		CandidateFlows: []flowexplain.CandidateFlow{{
			Name:             "Backup command",
			Trigger:          "user runs backup subcommand",
			LikelyEntrypoint: "cmd/tool/main.go",
			LikelyFiles:      []string{"cmd/tool/main.go", "cmd/tool/backup.go"},
			Evidence:         []string{"cmd/tool/main.go"},
			Confidence:       0.9,
		}},
		UnverifiedPaths: unverifiedPathList{{Path: "internal/repository", Reason: "not retrieved"}},
	}

	applyOrientationConfidenceGate(&report, bundle)

	flow := report.CandidateFlows[0]
	if flow.Confidence != incompleteContextCap {
		t.Fatalf("flow confidence = %.2f, want %.2f", flow.Confidence, incompleteContextCap)
	}
	if report.Confidence != incompleteContextCap {
		t.Fatalf("orientation confidence = %.2f, want %.2f", report.Confidence, incompleteContextCap)
	}
	if flow.LocalVerification == nil || flow.LocalVerification.Status != "partial" {
		t.Fatalf("local verification = %#v", flow.LocalVerification)
	}
	if !containsVerification(flow.LocalVerification.Verified, "verified command dispatch") {
		t.Fatalf("verified evidence = %v", flow.LocalVerification.Verified)
	}
	if !containsVerification(flow.LocalVerification.Missing, "complete core-package retrieval") {
		t.Fatalf("missing evidence = %v", flow.LocalVerification.Missing)
	}

	report.CandidateFlows[0].Confidence = 0.9
	report.CandidateFlows[0].LocalProof = &flowproof.Session{
		Version: flowproof.SessionVersion,
		Proof: flowproof.Proof{
			Version:   flowproof.Version,
			Archetype: flowproof.ArchetypeCLI,
			Slots: []flowproof.Slot{
				{Kind: flowproof.SlotTrigger, Status: flowproof.SlotVerified},
				{Kind: flowproof.SlotEntrypoint, Status: flowproof.SlotVerified},
				{Kind: flowproof.SlotDispatch, Status: flowproof.SlotVerified},
				{Kind: flowproof.SlotApplicationCallable, Status: flowproof.SlotVerified},
				{Kind: flowproof.SlotCoreOperation, Status: flowproof.SlotVerified},
				{Kind: flowproof.SlotIOBoundary, Status: flowproof.SlotVerified},
				{
					Kind: flowproof.SlotConcurrency, Status: flowproof.SlotNotApplicable,
					ApplicabilityReason: flowproof.ApplicabilityNoConcurrentLifecycleInScope,
					Provenance: []evidence.Provenance{{
						Provider: "fixture", Operation: "inspect_handler_concurrency",
					}},
				},
				{Kind: flowproof.SlotTermination, Status: flowproof.SlotVerified},
			},
		},
		Stop: &flowproof.Stop{Reason: flowproof.StopComplete},
	}
	applyOrientationConfidenceGate(&report, bundle)
	proved := report.CandidateFlows[0]
	if proved.Confidence != 0.9 {
		t.Fatalf("proof-scoped confidence = %.2f, want 0.9", proved.Confidence)
	}
	if proved.LocalVerification == nil || proved.LocalVerification.Status != "verified" ||
		containsVerification(proved.LocalVerification.Missing, "complete core-package retrieval") {
		t.Fatalf("proof-scoped verification = %#v", proved.LocalVerification)
	}
}

func confidenceLocation(path string, line int) *evidence.Location {
	return &evidence.Location{Path: path, Line: line}
}

func TestApplyOrientationConfidenceGateRejectsModelOnlyCLIDispatch(t *testing.T) {
	t.Parallel()

	report := orientationPart{
		ProjectGuess: "tool",
		Confidence:   0.9,
		CandidateFlows: []flowexplain.CandidateFlow{{
			Name:             "CLI command dispatch",
			Trigger:          "user runs a command",
			LikelyEntrypoint: "cmd/tool/main.go",
			LikelyFiles:      []string{"cmd/tool/main.go"},
			Evidence:         []string{"cmd/tool/main.go"},
			Confidence:       0.9,
		}},
	}
	applyOrientationConfidenceGate(&report, llmbundle.Bundle{})

	flow := report.CandidateFlows[0]
	if flow.Confidence != missingDispatchCap {
		t.Fatalf("flow confidence = %.2f, want %.2f", flow.Confidence, missingDispatchCap)
	}
	if flow.LocalVerification == nil || !containsVerification(flow.LocalVerification.Missing, "command dispatch") {
		t.Fatalf("local verification = %#v", flow.LocalVerification)
	}
}

func TestApplyOrientationConfidenceGateCapsUnprovedOperationalFlow(t *testing.T) {
	t.Parallel()

	report := orientationPart{
		ProjectGuess: "service",
		Confidence:   0.8,
		CandidateFlows: []flowexplain.CandidateFlow{{
			Name:       "Lease expiry background loop",
			FlowType:   flowexplain.FlowTypeOperational,
			Confidence: 0.8,
		}},
	}

	applyOrientationConfidenceGate(&report, llmbundle.Bundle{})

	flow := report.CandidateFlows[0]
	if flow.Confidence != 0.3 {
		t.Fatalf("flow confidence = %.2f, want 0.30", flow.Confidence)
	}
	if flow.LocalVerification == nil ||
		!containsVerification(flow.LocalVerification.Missing, "observed operational execution") {
		t.Fatalf("local verification = %#v", flow.LocalVerification)
	}
}

func TestRejectedDirectionDoesNotEnterAcceptedFlowSelection(t *testing.T) {
	t.Parallel()

	flows := []flowexplain.CandidateFlow{
		{Name: "Startup", Confidence: 0.3, LocalVerification: &flowexplain.FlowVerification{Verified: []string{"exact entrypoint"}}},
		{Name: "Admin", Confidence: 0.6},
		{Name: "Threshold", Confidence: 0.3},
	}
	for index := range flows {
		flowexplain.ClassifyCandidateFlow(&flows[index])
	}
	accepted := acceptedCandidateFlows(flows)
	if len(accepted) != 2 || accepted[0].Name != "Startup" || accepted[1].Name != "Admin" {
		t.Fatalf("accepted directions = %#v", accepted)
	}
	if flows[2].Disposition != flowexplain.DirectionRejected || flows[2].DispositionReason == "" {
		t.Fatalf("threshold disposition = %#v", flows[2])
	}
}

func containsVerification(values []string, fragment string) bool {
	for _, value := range values {
		if len(value) >= len(fragment) && value[:len(fragment)] == fragment {
			return true
		}
	}
	return false
}
