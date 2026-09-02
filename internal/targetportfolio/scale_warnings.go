package targetportfolio

// ScaleWarning reports an aggregate that crossed a former one-call usual
// size. It is diagnostic only and cannot reject or shorten a compilation.
type ScaleWarning struct {
	Kind         string
	Retained     int
	AdvisorySize int
}

const ScaleWarningCompleteRequestBytes = "complete_candidate_request_bytes"

func ScaleWarnings(compilation Compilation) []ScaleWarning {
	if validateCompilation(compilation) != nil || len(compilation.wire) <= AdvisoryCompleteRequestBytes {
		return nil
	}
	return []ScaleWarning{{
		Kind:     ScaleWarningCompleteRequestBytes,
		Retained: len(compilation.wire), AdvisorySize: AdvisoryCompleteRequestBytes,
	}}
}
