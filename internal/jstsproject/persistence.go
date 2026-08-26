package jstsproject

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/programindex"
)

// Persist writes one validated adapter artifact to an existing ordinary run.
func Persist(runDir string, result Result) error {
	encoded, err := Encode(result)
	if err != nil {
		return fmt.Errorf("jsts project: encode persisted artifact: %w", err)
	}
	writer, err := debugdump.OpenWriter(runDir, false)
	if err != nil {
		return fmt.Errorf("jsts project: open artifact writer: %w", err)
	}
	defer writer.Close()
	if err := writer.WriteValidatedFile(ArtifactFilename, encoded, func(saved []byte) error {
		decoded, decodeErr := Decode(saved)
		if decodeErr != nil {
			return decodeErr
		}
		if !reflect.DeepEqual(decoded, result) {
			return fmt.Errorf("jsts project: persisted authority mismatch")
		}
		return nil
	}); err != nil {
		return fmt.Errorf("jsts project: persist: %w", err)
	}
	return nil
}

// PersistAll writes the bound adapter artifact, ProgramIndex, and dependency
// catalog using their canonical owners and filenames.
func PersistAll(runDir string, result Result, index programindex.Index, catalog dependencies.Catalog) error {
	if err := ValidateProgramIndex(result, index); err != nil {
		return err
	}
	if err := catalog.Validate(); err != nil {
		return err
	}
	if err := Persist(runDir, result); err != nil {
		return err
	}
	if err := programindex.Persist(runDir, ProgramIndexFilename, index); err != nil {
		return err
	}
	if err := dependencies.Persist(runDir, catalog); err != nil {
		return err
	}
	return nil
}

// Load reads and validates the canonical adapter artifact from a run directory.
func Load(runDir string) (Result, error) {
	file, err := os.Open(filepath.Join(runDir, ArtifactFilename))
	if err != nil {
		return Result{}, fmt.Errorf("jsts project: open artifact: %w", err)
	}
	defer file.Close()
	encoded, err := io.ReadAll(io.LimitReader(file, MaxArtifactBytes+1))
	if err != nil {
		return Result{}, fmt.Errorf("jsts project: read artifact: %w", err)
	}
	return Decode(encoded)
}
