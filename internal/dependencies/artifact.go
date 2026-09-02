package dependencies

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const (
	ArtifactFilename = "dependency-catalog.json"

	// AdvisoryArtifactBytes is the former catalog persistence ceiling. A
	// complete validated catalog now crosses it unchanged and reports a scale
	// warning on the ordinary run path. MaxArtifactBytes is retained as a
	// compatibility sentinel for manifest readers; zero means no local cutoff.
	AdvisoryArtifactBytes = 32 << 20
	MaxArtifactBytes      = 0
)

// Encode returns the canonical, validated dependency artifact.
func Encode(catalog Catalog) ([]byte, error) {
	if err := catalog.Validate(); err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("dependencies: encode artifact: %w", err)
	}
	return append(encoded, '\n'), nil
}

// Decode rejects unknown fields, trailing values, and invalid identities.
func Decode(encoded []byte) (Catalog, error) {
	if len(encoded) == 0 {
		return Catalog{}, fmt.Errorf("dependencies: invalid artifact size")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var catalog Catalog
	if err := decoder.Decode(&catalog); err != nil {
		return Catalog{}, fmt.Errorf("dependencies: decode artifact: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Catalog{}, fmt.Errorf("dependencies: trailing JSON value")
		}
		return Catalog{}, fmt.Errorf("dependencies: trailing artifact data: %w", err)
	}
	if err := catalog.Validate(); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}
