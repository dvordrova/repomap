package claims

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dvordrova/repomap/internal/debugdump"
)

// Encode validates and returns the exact canonical JSON of one claims artifact.
func Encode(result Result) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("claims: encode artifact: %w", err)
	}
	return encoded, nil
}

// Decode strictly restores one canonical claims artifact.
func Decode(encoded []byte) (Result, error) {
	if len(encoded) == 0 {
		return Result{}, fmt.Errorf("claims: invalid artifact size")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var result Result
	if err := decoder.Decode(&result); err != nil {
		return Result{}, fmt.Errorf("claims: decode artifact: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Result{}, fmt.Errorf("claims: trailing JSON value")
		}
		return Result{}, fmt.Errorf("claims: trailing artifact data: %w", err)
	}
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	canonical, err := Encode(result)
	if err != nil {
		return Result{}, err
	}
	if !bytes.Equal(encoded, canonical) {
		return Result{}, fmt.Errorf("claims: artifact is not canonical")
	}
	return result, nil
}

// Persist atomically writes the claims artifact into one existing run directory.
func Persist(runDir string, result Result) error {
	owned, err := result.Snapshot()
	if err != nil {
		return fmt.Errorf("claims: own artifact: %w", err)
	}
	encoded, err := Encode(owned)
	if err != nil {
		return err
	}
	writer, err := debugdump.OpenWriter(runDir, false)
	if err != nil {
		return fmt.Errorf("claims: open artifact writer: %w", err)
	}
	defer writer.Close()
	if err := writer.WriteValidatedFile(
		ArtifactFilename,
		encoded,
		func(saved []byte) error {
			restored, decodeErr := Decode(saved)
			if decodeErr != nil {
				return decodeErr
			}
			if restored.SHA256 != owned.SHA256 || !bytes.Equal(saved, encoded) {
				return fmt.Errorf("claims: persisted authority mismatch")
			}
			return nil
		},
	); err != nil {
		return fmt.Errorf("claims: persist artifact: %w", err)
	}
	return nil
}

// Read restores the canonical claims artifact from one run directory.
func Read(runDir string) (Result, error) {
	encoded, err := os.ReadFile(filepath.Join(runDir, ArtifactFilename))
	if err != nil {
		return Result{}, fmt.Errorf("claims: read artifact: %w", err)
	}
	result, err := Decode(encoded)
	if err != nil {
		return Result{}, fmt.Errorf("claims: decode persisted artifact: %w", err)
	}
	return result, nil
}
