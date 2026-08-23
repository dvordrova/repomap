package coremap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Encode returns the validated CoreMap artifact produced by ordinary analysis.
func Encode(result Result) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("coremap: encode artifact: %w", err)
	}
	if len(encoded) > MaxArtifactBytes {
		return nil, fmt.Errorf("coremap: artifact is %d bytes, limit is %d", len(encoded), MaxArtifactBytes)
	}
	return append(encoded, '\n'), nil
}

// Decode strictly restores one standalone CoreMap artifact.
func Decode(encoded []byte) (Result, error) {
	if len(encoded) == 0 || len(encoded) > MaxArtifactBytes {
		return Result{}, fmt.Errorf("coremap: invalid artifact size")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var result Result
	if err := decoder.Decode(&result); err != nil {
		return Result{}, fmt.Errorf("coremap: decode artifact: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Result{}, fmt.Errorf("coremap: trailing JSON value")
		}
		return Result{}, fmt.Errorf("coremap: trailing artifact data: %w", err)
	}
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	return result, nil
}
