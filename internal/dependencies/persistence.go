package dependencies

import (
	"fmt"
	"reflect"

	"github.com/dvordrova/repomap/internal/debugdump"
)

// Persist writes one validated dependency catalog into an existing ordinary
// run directory. Persistence belongs to the language-neutral catalog owner;
// language adapters remain responsible for constructing the catalog.
func Persist(runDir string, catalog Catalog) error {
	encoded, err := Encode(catalog)
	if err != nil {
		return fmt.Errorf("dependency catalog: encode: %w", err)
	}
	writer, err := debugdump.OpenWriter(runDir, false)
	if err != nil {
		return fmt.Errorf("dependency catalog: open artifact writer: %w", err)
	}
	defer writer.Close()
	if err := writer.WriteValidatedFile(
		ArtifactFilename,
		encoded,
		func(saved []byte) error {
			decoded, decodeErr := Decode(saved)
			if decodeErr != nil {
				return decodeErr
			}
			if !reflect.DeepEqual(decoded, catalog) {
				return fmt.Errorf("dependency catalog: persisted authority mismatch")
			}
			return nil
		},
	); err != nil {
		return fmt.Errorf("dependency catalog: persist: %w", err)
	}
	return nil
}
