package orient

import (
	"context"

	"github.com/dvordrova/repomap/internal/flowproof"
	flowproofassemble "github.com/dvordrova/repomap/internal/flowproof/assemble"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/snapshot"
)

func localFlowProofInput(snapshot snapshot.Snapshot) flowproofassemble.Input {
	var traces []gofacts.CommandTrace
	if snapshot.GoFacts != nil {
		traces = snapshot.GoFacts.CommandTraces
	}
	return flowproofassemble.Input{
		CommandTraces: traces,
		ProofBudget:   flowproof.DefaultBudget(),
	}
}

func attachLocalFlowProofs(ctx context.Context, repoPath string, report *orientationPart, input flowproofassemble.Input) {
	report.Warnings = append(report.Warnings,
		flowproofassemble.Attach(ctx, repoPath, report.CandidateFlows, input)...,
	)
}
