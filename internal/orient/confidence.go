package orient

import (
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/flowproof"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/llmbundle"
)

const (
	missingEntrypointCap = 0.3
	missingDispatchCap   = 0.3
	missingCoreCallCap   = 0.5
	incompleteContextCap = 0.6
)

// applyOrientationConfidenceGate treats provider confidence as a proposal.
// Exact local entrypoint and command-trace facts determine the maximum value
// that can reach the report.
func applyOrientationConfidenceGate(report *orientationPart, bundle llmbundle.Bundle) {
	anchorPaths := make(map[string]struct{})
	for _, entrypoint := range bundle.Go.Entrypoints {
		for _, anchor := range entrypoint.Anchors {
			anchorPaths[anchor.Path] = struct{}{}
		}
	}

	contextIncomplete := len(report.UnverifiedPaths) > 0 || hasBundleWarning(bundle.Warnings, "truncated candidate_file_index")
	maxFlowConfidence := 0.0
	for index := range report.CandidateFlows {
		flow := &report.CandidateFlows[index]
		verification := &flowexplain.FlowVerification{Status: "verified", ConfidenceCap: 1}
		proofComplete := localProofComplete(flow.LocalProof)

		if _, ok := anchorPaths[flow.LikelyEntrypoint]; ok {
			verification.Verified = append(verification.Verified, "exact entrypoint declaration")
		} else if flowLooksLikeCLI(*flow, bundle.Go.CommandTraces) {
			verification.Missing = append(verification.Missing, "exact CLI entrypoint")
			verification.ConfidenceCap = minConfidence(verification.ConfidenceCap, missingEntrypointCap)
		}

		if flowLooksLikeCLI(*flow, bundle.Go.CommandTraces) {
			trace, ok := commandTraceForFlow(*flow, bundle.Go.CommandTraces)
			if !ok || !trace.Complete {
				verification.Missing = append(verification.Missing, "command dispatch: root command → registration → Run/RunE handler")
				verification.ConfidenceCap = minConfidence(verification.ConfidenceCap, missingDispatchCap)
			} else {
				verification.Verified = append(verification.Verified, commandTraceSummary(trace))
				if len(trace.HandlerCalls) == 0 {
					verification.Missing = append(verification.Missing, "first domain-level call from command handler")
					verification.ConfidenceCap = minConfidence(verification.ConfidenceCap, missingCoreCallCap)
				} else {
					verification.Verified = append(verification.Verified, "bounded handler call sites")
				}
			}
			if proofComplete {
				verification.Verified = append(verification.Verified, "bounded CLI proof completed locally")
			}
		}

		if contextIncomplete && !proofComplete {
			verification.Missing = append(verification.Missing, "complete core-package retrieval")
			verification.ConfidenceCap = minConfidence(verification.ConfidenceCap, incompleteContextCap)
		}
		if len(verification.Missing) > 0 {
			verification.Status = "partial"
		}
		if flow.Confidence > verification.ConfidenceCap {
			report.Warnings = append(report.Warnings, fmt.Sprintf(
				"local confidence gate capped candidate_flows[%d] from %.2f to %.2f",
				index,
				flow.Confidence,
				verification.ConfidenceCap,
			))
			flow.Confidence = verification.ConfidenceCap
		}
		flow.LocalVerification = verification
		if flow.Confidence > maxFlowConfidence {
			maxFlowConfidence = flow.Confidence
		}
	}
	if contextIncomplete && report.Confidence > incompleteContextCap {
		report.Warnings = append(report.Warnings, fmt.Sprintf(
			"local confidence gate capped orientation from %.2f to %.2f because focused retrieval is incomplete",
			report.Confidence,
			incompleteContextCap,
		))
		report.Confidence = incompleteContextCap
	}
	if maxFlowConfidence > 0 && report.Confidence > maxFlowConfidence && contextIncomplete {
		report.Confidence = maxFlowConfidence
	}
}

func localProofComplete(session *flowproof.Session) bool {
	return session != nil && session.Stop != nil &&
		session.Stop.Reason == flowproof.StopComplete && session.Proof.Satisfied()
}

func flowLooksLikeCLI(flow flowexplain.CandidateFlow, traces []gofacts.CommandTrace) bool {
	text := strings.ToLower(strings.Join([]string{flow.Name, flow.Trigger, flow.WhyInteresting}, " "))
	for _, word := range []string{"cli", "command", "subcommand", "cobra"} {
		if strings.Contains(text, word) {
			return true
		}
	}
	for _, trace := range traces {
		if len(trace.Steps) > 0 && trace.Steps[0].TargetLocation.Path == flow.LikelyEntrypoint {
			return true
		}
	}
	return false
}

func commandTraceForFlow(flow flowexplain.CandidateFlow, traces []gofacts.CommandTrace) (gofacts.CommandTrace, bool) {
	text := strings.ToLower(strings.Join([]string{flow.Name, flow.Trigger, flow.WhyInteresting}, " "))
	for _, trace := range traces {
		command := strings.ToLower(strings.TrimSpace(trace.Command))
		if command != "" && containsToken(text, command) {
			return trace, true
		}
	}
	for _, trace := range traces {
		if trace.Complete {
			return trace, true
		}
	}
	return gofacts.CommandTrace{}, false
}

func commandTraceSummary(trace gofacts.CommandTrace) string {
	parts := make([]string, 0, len(trace.Steps))
	for _, step := range trace.Steps {
		parts = append(parts, fmt.Sprintf("%s:%d %s", step.TargetLocation.Path, step.TargetLocation.Line, step.Symbol))
	}
	return "verified command dispatch: " + strings.Join(parts, " → ")
}

func hasBundleWarning(warnings []string, target string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, target) {
			return true
		}
	}
	return false
}

func containsToken(text, token string) bool {
	for _, part := range strings.FieldsFunc(text, func(r rune) bool {
		return r < 'a' || r > 'z'
	}) {
		if part == token {
			return true
		}
	}
	return false
}

func minConfidence(left, right float64) float64 {
	if right < left {
		return right
	}
	return left
}
