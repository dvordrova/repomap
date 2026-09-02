package readmetargetscout

type ScaleWarningKind string

const (
	ScaleWarningAggregateRequest  ScaleWarningKind = "aggregate_request_bytes"
	ScaleWarningAggregateResponse ScaleWarningKind = "aggregate_response_bytes"
	ScaleWarningClassifications   ScaleWarningKind = "classifications_per_file"
	ScaleWarningHypotheses        ScaleWarningKind = "hypotheses_per_classification"
	ScaleWarningHypothesisBytes   ScaleWarningKind = "hypothesis_bytes"
	ScaleWarningArtifactBytes     ScaleWarningKind = "artifact_bytes"
)

// ScaleWarning is diagnostic only. AdvisorySize is the former local cutoff;
// Retained is the complete observed value that remains authoritative.
type ScaleWarning struct {
	Kind         ScaleWarningKind
	Retained     int
	AdvisorySize int
}

func InputScaleWarnings(compilation Compilation) []ScaleWarning {
	if compilation.State != StateReady || len(compilation.wire) <= AdvisoryAtomicRequestBytes {
		return nil
	}
	return []ScaleWarning{{
		Kind: ScaleWarningAggregateRequest, Retained: len(compilation.wire),
		AdvisorySize: AdvisoryAtomicRequestBytes,
	}}
}

func ResultScaleWarnings(result Result) []ScaleWarning {
	warnings := make([]ScaleWarning, 0)
	maxClasses, maxHypotheses, maxHypothesisBytes := 0, 0, 0
	for _, file := range result {
		if len(file.Classifications) > maxClasses {
			maxClasses = len(file.Classifications)
		}
		for _, classification := range file.Classifications {
			if len(classification.Hypotheses) > maxHypotheses {
				maxHypotheses = len(classification.Hypotheses)
			}
			for _, hypothesis := range classification.Hypotheses {
				if len(hypothesis) > maxHypothesisBytes {
					maxHypothesisBytes = len(hypothesis)
				}
			}
		}
	}
	if maxClasses > AdvisoryClassificationsPerFile {
		warnings = append(warnings, ScaleWarning{
			Kind: ScaleWarningClassifications, Retained: maxClasses,
			AdvisorySize: AdvisoryClassificationsPerFile,
		})
	}
	if maxHypotheses > AdvisoryHypothesesPerClass {
		warnings = append(warnings, ScaleWarning{
			Kind: ScaleWarningHypotheses, Retained: maxHypotheses,
			AdvisorySize: AdvisoryHypothesesPerClass,
		})
	}
	if maxHypothesisBytes > AdvisoryHypothesisBytes {
		warnings = append(warnings, ScaleWarning{
			Kind: ScaleWarningHypothesisBytes, Retained: maxHypothesisBytes,
			AdvisorySize: AdvisoryHypothesisBytes,
		})
	}
	return warnings
}

// ExecutionScaleWarnings measures provider response size from the exact
// decoded assistant-content byte counts retained by each accepted outcome.
// The merged Result remains a separate semantic/artifact measurement: its
// canonical JSON size is not a substitute for bytes the provider returned.
func ExecutionScaleWarnings(execution Execution) []ScaleWarning {
	warnings := ResultScaleWarnings(execution.Result)
	responseBytes := 0
	for _, outcome := range execution.Outcomes {
		if outcome.ResponseBytes <= 0 {
			continue
		}
		maximum := int(^uint(0) >> 1)
		if responseBytes > maximum-outcome.ResponseBytes {
			responseBytes = maximum
			break
		}
		responseBytes += outcome.ResponseBytes
	}
	if responseBytes > AdvisoryResponseBytes {
		warnings = append(warnings, ScaleWarning{
			Kind: ScaleWarningAggregateResponse, Retained: responseBytes,
			AdvisorySize: AdvisoryResponseBytes,
		})
	}
	return warnings
}

func ArtifactScaleWarnings(retainedBytes int) []ScaleWarning {
	if retainedBytes <= AdvisoryArtifactBytes {
		return nil
	}
	return []ScaleWarning{{
		Kind: ScaleWarningArtifactBytes, Retained: retainedBytes,
		AdvisorySize: AdvisoryArtifactBytes,
	}}
}
