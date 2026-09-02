package targetoutcome

import "encoding/json"

type ScaleWarningKind string

const (
	ScaleWarningOutcomes         ScaleWarningKind = "target_outcome_portfolio_outcomes"
	ScaleWarningAllowedLanguages ScaleWarningKind = "target_allowed_program_languages"
	ScaleWarningBytes            ScaleWarningKind = "target_outcome_portfolio_bytes"
)

type ScaleWarning struct {
	Kind                ScaleWarningKind
	AdvisorySize        int
	AffectedCollections int
	MaximumRetained     int
}

func ScaleWarnings(portfolio Portfolio) []ScaleWarning {
	warnings := []ScaleWarning{
		{Kind: ScaleWarningOutcomes, AdvisorySize: MaxOutcomes},
		{Kind: ScaleWarningAllowedLanguages, AdvisorySize: MaxAllowedProgramLanguages},
		{Kind: ScaleWarningBytes, AdvisorySize: AdvisoryArtifactBytes},
	}
	record := func(position, retained int) {
		if retained <= warnings[position].AdvisorySize {
			return
		}
		warnings[position].AffectedCollections++
		if retained > warnings[position].MaximumRetained {
			warnings[position].MaximumRetained = retained
		}
	}
	record(0, len(portfolio.Outcomes))
	for _, outcome := range portfolio.Outcomes {
		record(1, len(outcome.SelectedTarget.AllowedProgramLanguages))
	}
	if encoded, err := json.Marshal(portfolio); err == nil {
		record(2, len(encoded))
	}
	result := make([]ScaleWarning, 0, len(warnings))
	for _, warning := range warnings {
		if warning.AffectedCollections > 0 {
			result = append(result, warning)
		}
	}
	return result
}

// SelectedTargetScaleWarnings reports the only target-outcome scale
// measurement that is complete before analysis starts. It lets orchestration
// log the exact selected-target language authority even when later target
// work fails; it never changes that authority.
func SelectedTargetScaleWarnings(targets []SelectedTarget) []ScaleWarning {
	warning := ScaleWarning{
		Kind: ScaleWarningAllowedLanguages, AdvisorySize: MaxAllowedProgramLanguages,
	}
	for _, target := range targets {
		retained := len(target.AllowedProgramLanguages)
		if retained <= warning.AdvisorySize {
			continue
		}
		warning.AffectedCollections++
		if retained > warning.MaximumRetained {
			warning.MaximumRetained = retained
		}
	}
	if warning.AffectedCollections == 0 {
		return nil
	}
	return []ScaleWarning{warning}
}
