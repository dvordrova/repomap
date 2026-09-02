package programpage

import "encoding/json"

type ScaleWarningKind string

const (
	ScaleWarningPages ScaleWarningKind = "program_page_portfolio_pages"
	ScaleWarningBytes ScaleWarningKind = "program_page_portfolio_bytes"
)

type ScaleWarning struct {
	Kind         ScaleWarningKind
	Retained     int
	AdvisorySize int
}

func ScaleWarnings(portfolio Portfolio) []ScaleWarning {
	result := []ScaleWarning{}
	if len(portfolio.Pages) > MaxPages {
		result = append(result, ScaleWarning{Kind: ScaleWarningPages, Retained: len(portfolio.Pages), AdvisorySize: MaxPages})
	}
	if encoded, err := json.Marshal(portfolio); err == nil && len(encoded) > AdvisoryArtifactBytes {
		result = append(result, ScaleWarning{Kind: ScaleWarningBytes, Retained: len(encoded), AdvisorySize: AdvisoryArtifactBytes})
	}
	return result
}
