package groupindex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/debugdump"
)

// Persist writes one validated GroupsIndex into an existing ordinary run
// directory. The artifact is replaced only after the saved bytes decode back
// to the same sealed target graph.
func Persist(runDir string, index Index) error {
	return PersistNamed(runDir, ArtifactFilename, index)
}

// PersistNamed writes a GroupsIndex under an explicit page-local artifact
// filename. It exists only for the temporary case where one adapter build
// returns several exact target views; the ordinary selected-target page uses
// ArtifactFilename.
func PersistNamed(runDir string, filename string, index Index) error {
	if strings.TrimSpace(filename) == "" || filepath.Base(filename) != filename {
		return fmt.Errorf("group index: invalid artifact filename %q", filename)
	}
	encoded, err := Encode(index)
	if err != nil {
		return fmt.Errorf("group index: encode: %w", err)
	}
	writer, err := debugdump.OpenWriter(runDir, false)
	if err != nil {
		return fmt.Errorf("group index: open artifact writer: %w", err)
	}
	defer writer.Close()
	if err := writer.WriteValidatedFile(
		filename,
		encoded,
		func(saved []byte) error {
			decoded, decodeErr := Decode(saved)
			if decodeErr != nil {
				return decodeErr
			}
			if decoded.SHA256 != index.SHA256 || decoded.Target.ID != index.Target.ID {
				return fmt.Errorf("group index: persisted authority mismatch")
			}
			return nil
		},
	); err != nil {
		return fmt.Errorf("group index: persist: %w", err)
	}
	return nil
}

// Read restores the canonical GroupsIndex artifact from one ordinary run.
func Read(runDir string) (Index, error) {
	return ReadNamed(runDir, ArtifactFilename)
}

// ReadNamed restores one explicit page-local GroupsIndex artifact.
func ReadNamed(runDir string, filename string) (Index, error) {
	if strings.TrimSpace(filename) == "" || filepath.Base(filename) != filename {
		return Index{}, fmt.Errorf("group index: invalid artifact filename %q", filename)
	}
	raw, err := os.ReadFile(filepath.Join(runDir, filename))
	if err != nil {
		return Index{}, fmt.Errorf("group index: read artifact: %w", err)
	}
	index, err := Decode(raw)
	if err != nil {
		return Index{}, fmt.Errorf("group index: decode artifact: %w", err)
	}
	return index, nil
}
