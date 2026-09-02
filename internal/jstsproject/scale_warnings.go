package jstsproject

import "encoding/json"

type ScaleWarningKind string

const ScaleWarningResultBytes ScaleWarningKind = "jsts_project_result_bytes"

type ScaleWarning struct {
	Kind         ScaleWarningKind
	Retained     int
	AdvisorySize int
}

// ScaleWarnings measures complete retained project authority without changing
// validation or identity.
func ScaleWarnings(result Result) []ScaleWarning {
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil
	}
	return scaleWarningsForResultBytes(len(encoded))
}

func scaleWarningsForResultBytes(retained int) []ScaleWarning {
	if retained <= AdvisoryResultBytes {
		return nil
	}
	return []ScaleWarning{{
		Kind: ScaleWarningResultBytes, Retained: retained,
		AdvisorySize: AdvisoryResultBytes,
	}}
}
