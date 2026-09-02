package main

import (
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/programindex"
)

// reportSemanticOrdinalScaleWarnings is shared by the active model-backed
// repository selection, categorization, grouping, matching, and portfolio
// stages. Ordinal thresholds are diagnostics only.
func reportSemanticOrdinalScaleWarnings(
	output *runOutput,
	label string,
	contextDetails []string,
	warnings []debugdump.SemanticOrdinalScaleWarning,
) {
	if output == nil || len(warnings) == 0 {
		return
	}
	details := []string{
		"all semantic journal attempts and accepted instances were retained; former ordinal ceilings are diagnostic only",
	}
	details = append(details, contextDetails...)
	for _, warning := range warnings {
		details = append(details, fmt.Sprintf(
			"%s: largest retained %d; former usual size %d",
			warning.Kind, warning.Retained, warning.AdvisorySize,
		))
	}
	if strings.TrimSpace(label) == "" {
		label = "Semantic"
	}
	output.Warn(label+" model journal scale", details...)
}

func semanticWarningTargetDetail(target programindex.Target) string {
	language := strings.TrimSpace(target.Language)
	name := strings.TrimSpace(target.Name)
	selector := strings.TrimSpace(target.Selector)
	if language == "" && name == "" && selector == "" {
		return ""
	}
	return fmt.Sprintf(
		"target: language=%q; name=%q; selector=%q; program_id=%q",
		language, name, selector, target.ID,
	)
}
