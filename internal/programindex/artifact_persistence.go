package programindex

import (
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/debugdump"
)

// Persist writes one validated ProgramIndex artifact into an existing run
// directory. An empty filename selects the canonical default artifact name.
//
// Encode validates the sealed index, seal included, and returns its one
// canonical encoding; those bytes are written as they are. Decoding them
// again here would only repeat what the adapter conformance kit proves for
// every adapter's real output (adaptertest.AssertSharedArtifact), and every
// reader still decodes and validates what it reads.
func Persist(runDir string, filename string, index Index) error {
	if strings.TrimSpace(filename) == "" {
		filename = ArtifactFilename
	}
	encoded, err := Encode(index)
	if err != nil {
		return fmt.Errorf("program index: encode %s: %w", filename, err)
	}
	writer, err := debugdump.OpenWriter(runDir)
	if err != nil {
		return fmt.Errorf("program index: open artifact writer: %w", err)
	}
	defer writer.Close()
	if err := writer.WriteFile(filename, encoded); err != nil {
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
	writer, err := debugdump.OpenWriter(runDir)
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
