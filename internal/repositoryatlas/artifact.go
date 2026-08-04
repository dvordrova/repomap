package repositoryatlas

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	ArtifactFilename = "repository_atlas.v1.json"
	MaxArtifactBytes = 32 << 20
)

// DecodeCanonicalJSON strictly decodes one bounded canonical Atlas artifact.
// Artifacts are required to use CanonicalJSON byte-for-byte so ordering cannot
// become a second, implicit identity contract.
func DecodeCanonicalJSON(data []byte) (Atlas, error) {
	if len(data) == 0 || len(data) > MaxArtifactBytes {
		return Atlas{}, fmt.Errorf(
			"repository atlas artifact: size must be between 1 and %d bytes",
			MaxArtifactBytes,
		)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var atlas Atlas
	if err := decoder.Decode(&atlas); err != nil {
		return Atlas{}, fmt.Errorf("repository atlas artifact: decode: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return Atlas{}, fmt.Errorf("repository atlas artifact: multiple json values")
		}
		return Atlas{}, fmt.Errorf("repository atlas artifact: trailing data: %w", err)
	}
	canonical, err := CanonicalJSON(atlas)
	if err != nil {
		return Atlas{}, fmt.Errorf("repository atlas artifact: %w", err)
	}
	if !bytes.Equal(data, canonical) {
		return Atlas{}, fmt.Errorf("repository atlas artifact: bytes are not canonical")
	}
	return atlas, nil
}
