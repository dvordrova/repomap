package programindex

import "encoding/json"

type ArtifactSetScaleWarningKind string

const (
	ArtifactSetScaleWarningBytes ArtifactSetScaleWarningKind = "program_index_set_bytes"
)

type ArtifactSetScaleWarning struct {
	Kind         ArtifactSetScaleWarningKind
	Retained     int
	AdvisorySize int
}

// ArtifactSetScaleWarnings reports the former local byte threshold without
// changing or validating the page-local binding.
func ArtifactSetScaleWarnings(set ArtifactSet) []ArtifactSetScaleWarning {
	result := []ArtifactSetScaleWarning{}
	if encoded, err := json.Marshal(set); err == nil && len(encoded) > AdvisoryArtifactSetBytes {
		result = append(result, ArtifactSetScaleWarning{
			Kind: ArtifactSetScaleWarningBytes, Retained: len(encoded),
			AdvisorySize: AdvisoryArtifactSetBytes,
		})
	}
	return result
}
