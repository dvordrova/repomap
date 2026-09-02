package main

import (
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/programcategorization"
)

// formatCategorizationCoverage prints the categorization result with its
// denominators. An absolute count of accepted rows cannot distinguish a
// healthy run from one that lost most of its signal, and a category carried
// by nearly every subject routes no attention, so both the fraction covered
// and the per-category split are printed on every run.
func formatCategorizationCoverage(coverage programcategorization.Coverage) []string {
	details := []string{
		"subjects covered: " + formatCoverageFraction(coverage.CoveredSubjects(), coverage.Subjects()) +
			" (objects " + formatCoverageFraction(coverage.CoveredObjects, coverage.Objects) +
			", connections " + formatCoverageFraction(coverage.CoveredPatterns, coverage.Patterns) + ")",
		"categories: " + formatCoverageCategories(coverage),
	}
	if coverage.Requests > 0 {
		details = append(details, fmt.Sprintf(
			"requests: %d, of which %d assigned nothing", coverage.Requests, coverage.Empty,
		))
	}
	details = append(details, fmt.Sprintf(
		"accepted rows naming a subject outside the index: %d", coverage.OutsideIndex,
	))
	return details
}

func formatCoverageFraction(covered, total int) string {
	if total == 0 {
		return fmt.Sprintf("%d/0", covered)
	}
	return fmt.Sprintf("%d/%d (%d%%)", covered, total, covered*100/total)
}

// formatCoverageCategories prints every category in the closed vocabulary,
// including the ones nothing landed on: a zero is the finding.
func formatCoverageCategories(coverage programcategorization.Coverage) string {
	subjects := coverage.Subjects()
	parts := make([]string, 0, 4)
	for _, category := range []programcategorization.Category{
		programcategorization.CategoryInbound,
		programcategorization.CategoryBackgroundActivity,
		programcategorization.CategoryDependency,
		programcategorization.CategoryCore,
	} {
		count := coverage.ByCategory[category]
		part := strings.ReplaceAll(string(category), "_", " ") + " " + fmt.Sprint(count)
		if subjects > 0 {
			part += fmt.Sprintf(" (%d%%)", count*100/subjects)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ", ")
}
