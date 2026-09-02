package pythontarget

import "encoding/json"

type ScaleWarningKind string

const ScaleWarningCatalogBytes ScaleWarningKind = "python_target_catalog_bytes"

type ScaleWarning struct {
	Kind         ScaleWarningKind
	Retained     int
	AdvisorySize int
}

// ScaleWarnings measures the complete sealed catalog without changing its
// targets, identity, or validation.
func ScaleWarnings(catalog Catalog) []ScaleWarning {
	encoded, err := json.Marshal(catalog)
	if err != nil {
		return nil
	}
	return scaleWarningsForCatalogBytes(len(encoded))
}

func scaleWarningsForCatalogBytes(retained int) []ScaleWarning {
	if retained <= AdvisoryCatalogBytes {
		return nil
	}
	return []ScaleWarning{{
		Kind: ScaleWarningCatalogBytes, Retained: retained,
		AdvisorySize: AdvisoryCatalogBytes,
	}}
}
