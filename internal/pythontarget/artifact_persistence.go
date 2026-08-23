package pythontarget

import (
	"fmt"

	"github.com/dvordrova/repomap/internal/debugdump"
)

// PersistCatalog writes one validated Python target catalog into an existing
// run directory.
func PersistCatalog(runDir string, catalog Catalog) error {
	encoded, err := catalog.CanonicalJSON()
	if err != nil {
		return fmt.Errorf("Python target catalog: encode: %w", err)
	}
	writer, err := debugdump.OpenWriter(runDir, false)
	if err != nil {
		return fmt.Errorf("Python target catalog: open artifact writer: %w", err)
	}
	defer writer.Close()
	if err := writer.WriteValidatedFile(
		ArtifactFilename,
		encoded,
		func(saved []byte) error {
			decoded, err := DecodeCatalog(saved)
			if err != nil {
				return err
			}
			if decoded.Ref != catalog.Ref {
				return fmt.Errorf("Python target catalog: persisted authority mismatch")
			}
			return nil
		},
	); err != nil {
		return fmt.Errorf("Python target catalog: persist: %w", err)
	}
	return nil
}
