package documentationreduce

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dvordrova/repomap/internal/debugdump"
)

const ArtifactFilename = "reduced-documentation.json"

// Encode validates and returns the exact canonical JSON representation of one
// reduced_documentation handoff.
func Encode(result Result) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("documentation reduce: encode artifact: %w", err)
	}
	return encoded, nil
}

// Decode strictly restores one canonical reduced_documentation artifact. It
// rejects unknown fields, trailing values, non-canonical JSON, and a broken
// reduction seal.
func Decode(encoded []byte) (Result, error) {
	if len(encoded) == 0 {
		return Result{}, fmt.Errorf("documentation reduce: invalid artifact size")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var result Result
	if err := decoder.Decode(&result); err != nil {
		return Result{}, fmt.Errorf("documentation reduce: decode artifact: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Result{}, fmt.Errorf("documentation reduce: trailing JSON value")
		}
		return Result{}, fmt.Errorf("documentation reduce: trailing artifact data: %w", err)
	}
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	canonical, err := Encode(result)
	if err != nil {
		return Result{}, err
	}
	if !bytes.Equal(encoded, canonical) {
		return Result{}, fmt.Errorf("documentation reduce: artifact is not canonical")
	}
	return result, nil
}

// Persist atomically replaces the reduced_documentation artifact in one
// existing ordinary run directory. The exact bytes are decoded and compared
// with the owned reduction before they become visible.
func Persist(runDir string, result Result) error {
	owned, err := result.Snapshot()
	if err != nil {
		return fmt.Errorf("documentation reduce: own artifact: %w", err)
	}
	encoded, err := Encode(owned)
	if err != nil {
		return fmt.Errorf("documentation reduce: encode artifact: %w", err)
	}
	writer, err := debugdump.OpenWriter(runDir, false)
	if err != nil {
		return fmt.Errorf("documentation reduce: open artifact writer: %w", err)
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
			if restored.GuidanceSHA256 != owned.GuidanceSHA256 ||
				restored.ReductionSHA256 != owned.ReductionSHA256 ||
				!bytes.Equal(saved, encoded) {
				return fmt.Errorf("documentation reduce: persisted authority mismatch")
			}
			return nil
		},
	); err != nil {
		return fmt.Errorf("documentation reduce: persist artifact: %w", err)
	}
	return nil
}

// Read restores the canonical reduced_documentation artifact from one
// ordinary run directory.
func Read(runDir string) (Result, error) {
	encoded, err := os.ReadFile(filepath.Join(runDir, ArtifactFilename))
	if err != nil {
		return Result{}, fmt.Errorf("documentation reduce: read artifact: %w", err)
	}
	result, err := Decode(encoded)
	if err != nil {
		return Result{}, fmt.Errorf("documentation reduce: decode persisted artifact: %w", err)
	}
	return result, nil
}
