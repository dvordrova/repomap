package orient

import (
	"context"

	flowproofassemble "github.com/dvordrova/repomap/internal/flowproof/assemble"
	"github.com/dvordrova/repomap/internal/llmbundle"
)

func attachLocalFlowProofs(ctx context.Context, repoPath string, report *orientationPart, bundle llmbundle.Bundle) {
	report.Warnings = append(report.Warnings,
		flowproofassemble.Attach(ctx, repoPath, report.CandidateFlows, bundle.Go.CommandTraces)...,
	)
}
