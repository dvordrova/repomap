package dependencies

import "encoding/json"

type ScaleWarningKind string

const ScaleWarningArtifactBytes ScaleWarningKind = "dependency_catalog_artifact_bytes"

type ScaleWarning struct {
	Kind         ScaleWarningKind
	Retained     int
	AdvisorySize int
}

// ScaleWarnings measures the complete accepted dependency catalog. It cannot
// validate, truncate, rewrite, or reject dependency authority.
func ScaleWarnings(catalog Catalog) []ScaleWarning {
	encoded, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return nil
	}
	return scaleWarningsForEncodedBytes(len(encoded) + 1) // Encode appends one canonical newline.
}

func scaleWarningsForEncodedBytes(retained int) []ScaleWarning {
	if retained <= AdvisoryArtifactBytes {
		return nil
	}
	return []ScaleWarning{{
		Kind: ScaleWarningArtifactBytes, Retained: retained,
		AdvisorySize: AdvisoryArtifactBytes,
	}}
}
