package dependencies

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const (
	ArtifactFilename = "dependency-catalog.json"
	MaxArtifactBytes = 32 << 20
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
	if len(encoded) > MaxArtifactBytes {
		return nil, fmt.Errorf("dependencies: artifact is %d bytes, limit is %d", len(encoded), MaxArtifactBytes)
	}
	return append(encoded, '\n'), nil
}

// Decode rejects unknown fields, trailing values, invalid identities and
// artifacts outside the same bound used by Encode.
func Decode(encoded []byte) (Catalog, error) {
	if len(encoded) == 0 || len(encoded) > MaxArtifactBytes {
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
