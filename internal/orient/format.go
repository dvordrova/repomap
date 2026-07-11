package orient

import (
	"encoding/json"
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
		if ef.Error != "" {
			b.WriteString(fmt.Sprintf("  Model explanation rejected: %s\n", ef.Error))
		}

		var summary string
		var confidence float64
		var likelyChain []flowChainStep
		var readFiles []fileToOpen
		var testsToRead []fileToOpen
		var unknowns []string
		var warnings []string

		if ef.FlowReport != nil {
			var fr flowReportFields
			if json.Unmarshal(ef.FlowReport, &fr) == nil {
				summary = fr.Summary
				confidence = fr.Confidence
				likelyChain = fr.LikelyChain
				readFiles = fr.FilesToReadInOrder
				testsToRead = fr.TestsToRead
				unknowns = fr.Unknowns
				warnings = fr.Warnings
			}
		}

		if summary != "" {
			b.WriteString(fmt.Sprintf("  %s\n", summary))
			b.WriteString(fmt.Sprintf("  Confidence: %.0f%%\n", confidence*100))
		} else {
			b.WriteString(fmt.Sprintf("  (selected %d files, %d tests, %d docs)\n",
				ef.FlowBundleSummary.SelectedFilesCount,
				ef.FlowBundleSummary.SelectedTestsCount,
				ef.FlowBundleSummary.SelectedDocsCount))
		}

		if len(likelyChain) > 0 {
			b.WriteString("  Evidence-backed likely chain:\n")
			for _, step := range likelyChain {
				b.WriteString(fmt.Sprintf("    %d. %s", step.Step, step.WhatHappens))
				if len(step.EvidenceFiles) > 0 {
					b.WriteString(fmt.Sprintf(" [%s]", strings.Join(step.EvidenceFiles, ", ")))
				}
				b.WriteByte('\n')
			}
		}

		if len(readFiles) > 0 {
			b.WriteString(fmt.Sprintf("  Files to read (%d):\n", len(readFiles)))
			for _, f := range readFiles {
				b.WriteString(fmt.Sprintf("    %s\n", f.Path))
				if f.Reason != "" {
					b.WriteString(fmt.Sprintf("      %s\n", f.Reason))
				}
			}
		}

		if len(testsToRead) > 0 {
			b.WriteString(fmt.Sprintf("  Tests (%d):\n", len(testsToRead)))
			for _, t := range testsToRead {
				b.WriteString(fmt.Sprintf("    %s\n", t.Path))
			}
		}

		if len(ef.FlowBundleSummary.UnverifiedSeeds) > 0 {
			b.WriteString(fmt.Sprintf("  Unverified seeds: %v\n", ef.FlowBundleSummary.UnverifiedSeeds))
		}

		if len(unknowns) > 0 {
			b.WriteString(fmt.Sprintf("  Unknowns: %v\n", unknowns))
		}

		if len(warnings) > 0 {
			b.WriteString(fmt.Sprintf("  Warnings: %v\n", warnings))
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
