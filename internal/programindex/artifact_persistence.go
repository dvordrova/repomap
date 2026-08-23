package programindex

import (
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/debugdump"
)

// Persist writes one validated ProgramIndex artifact into an existing run
// directory. An empty filename selects the canonical default artifact name.
func Persist(runDir string, filename string, index Index) error {
	if strings.TrimSpace(filename) == "" {
		filename = ArtifactFilename
	}
	encoded, err := Encode(index)
	if err != nil {
		return fmt.Errorf("program index: encode %s: %w", filename, err)
	}
	writer, err := debugdump.OpenWriter(runDir, false)
	if err != nil {
		return fmt.Errorf("program index: open artifact writer: %w", err)
	}
	defer writer.Close()
	if err := writer.WriteValidatedFile(filename, encoded, func(saved []byte) error {
		decoded, decodeErr := Decode(saved)
		if decodeErr != nil {
			return decodeErr
		}
		if decoded.SHA256 != index.SHA256 || decoded.Target.ID != index.Target.ID {
			return fmt.Errorf("program index: persisted authority mismatch")
		}
		return nil
	}); err != nil {
		return fmt.Errorf("program index: persist %s: %w", filename, err)
	}
	return nil
}

// PersistArtifactSet writes one validated ProgramIndex artifact-set handoff
// into an existing run directory.
func PersistArtifactSet(runDir string, set ArtifactSet) error {
	encoded, err := EncodeArtifactSet(set)
	if err != nil {
		return fmt.Errorf("program index set: encode: %w", err)
	}
	writer, err := debugdump.OpenWriter(runDir, false)
	if err != nil {
		return fmt.Errorf("program index set: open artifact writer: %w", err)
	}
	defer writer.Close()
	if err := writer.WriteValidatedFile(
		ArtifactSetFilename,
		encoded,
		func(saved []byte) error {
			decoded, decodeErr := DecodeArtifactSet(saved)
			if decodeErr != nil {
				return decodeErr
			}
			if decoded.SHA256 != set.SHA256 || decoded.DefaultTargetID != set.DefaultTargetID {
				return fmt.Errorf("program index set: persisted authority mismatch")
			}
			return nil
		},
	); err != nil {
		return fmt.Errorf("program index set: persist: %w", err)
	}
	return nil
}
