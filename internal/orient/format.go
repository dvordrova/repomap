package orient

import (
	"fmt"
	"strings"
)

func formatHumanReadable(report combinedReport, debugDir string, runID string) string {
	var b strings.Builder

	if report.Orientation != nil {
		b.WriteString(fmt.Sprintf("Project: %s\n", report.Orientation.ProjectGuess))
		b.WriteString(fmt.Sprintf("Confidence: %.0f%%\n", report.Orientation.Confidence*100))
		if len(report.Orientation.HighLevelMap) > 0 {
			b.WriteString("High-level map:\n")
			for _, item := range report.Orientation.HighLevelMap {
				b.WriteString(fmt.Sprintf("  - %s", item.Name))
				if item.WhyItMatters != "" {
					b.WriteString(": " + item.WhyItMatters)
				}
				b.WriteByte('\n')
			}
		}
		if len(report.Orientation.FirstFilesToOpen) > 0 {
			b.WriteString("Start here:\n")
			for _, file := range report.Orientation.FirstFilesToOpen {
				b.WriteString(fmt.Sprintf("  - %s", file.Path))
				if file.Reason != "" {
					b.WriteString(": " + file.Reason)
				}
				b.WriteByte('\n')
			}
		}
		if len(report.Orientation.QuestionsForHuman) > 0 {
			b.WriteString(fmt.Sprintf("Questions to guide the next step: %v\n", report.Orientation.QuestionsForHuman))
		}
		if len(report.Orientation.Warnings) > 0 {
			b.WriteString(fmt.Sprintf("Orientation warnings: %v\n", report.Orientation.Warnings))
		}
		if len(report.Orientation.CandidateFlows) > 0 {
			b.WriteString("Candidate directions:\n")
			for _, flow := range report.Orientation.CandidateFlows {
				b.WriteString(fmt.Sprintf("  - %s (%.0f%%)", flow.Name, flow.Confidence*100))
				if flow.Trigger != "" {
					b.WriteString(": " + flow.Trigger)
				}
				b.WriteByte('\n')
			}
		}
	}
	b.WriteString(fmt.Sprintf("\n%d direction(s) expanded:\n\n", len(report.ExplainedFlows)))

	for i, ef := range report.ExplainedFlows {
		b.WriteString(fmt.Sprintf("━ %s ━\n", ef.FlowSeed.Name))
		b.WriteString(fmt.Sprintf("  (selected %d files, %d tests, %d docs)\n",
			ef.FlowBundleSummary.SelectedFilesCount,
			ef.FlowBundleSummary.SelectedTestsCount,
			ef.FlowBundleSummary.SelectedDocsCount))

		if len(ef.FlowBundleSummary.UnverifiedSeeds) > 0 {
			b.WriteString(fmt.Sprintf("  Unverified seeds: %v\n", ef.FlowBundleSummary.UnverifiedSeeds))
		}

		if i < len(report.ExplainedFlows)-1 {
			b.WriteString("\n")
		}
	}

	if len(report.Warnings) > 0 {
		b.WriteString(fmt.Sprintf("\nWarnings: %v\n", report.Warnings))
	}

	if debugDir != "" {
		b.WriteString(fmt.Sprintf("\nArtifacts: %s/%s\n", debugDir, runID))
	}

	return b.String()
}
