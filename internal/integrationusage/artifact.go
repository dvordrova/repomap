package integrationusage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
)

const (
	ArtifactFilename = "integration-usage.json"
	MaxArtifactBytes = 32 << 20
)

// Encode returns the exact canonical standalone artifact bytes.
func Encode(result Result) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("integration usage: encode artifact: %w", err)
	}
	if len(encoded) == 0 || len(encoded) > MaxArtifactBytes {
		return nil, fmt.Errorf(
			"integration usage: artifact is %d bytes, limit is %d",
			len(encoded), MaxArtifactBytes,
		)
	}
	return encoded, nil
}

// Decode rejects unknown fields, trailing values, invalid identities, and any
// representation other than the exact canonical bytes produced by Encode.
func Decode(encoded []byte) (Result, error) {
	if len(encoded) == 0 || len(encoded) > MaxArtifactBytes {
		return Result{}, fmt.Errorf("integration usage: invalid artifact size")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var result Result
	if err := decoder.Decode(&result); err != nil {
		return Result{}, fmt.Errorf("integration usage: decode artifact: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Result{}, fmt.Errorf("integration usage: trailing JSON value")
		}
		return Result{}, fmt.Errorf("integration usage: trailing artifact data: %w", err)
	}
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	canonical, err := Encode(result)
	if err != nil {
		return Result{}, err
	}
	if !bytes.Equal(encoded, canonical) {
		return Result{}, fmt.Errorf("integration usage: artifact is not canonical")
	}
	return result, nil
}

// ArtifactSHA256 returns the digest of the exact canonical artifact bytes.
func (result Result) ArtifactSHA256() (string, error) {
	encoded, err := Encode(result)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
