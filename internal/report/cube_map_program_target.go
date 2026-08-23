package report

import (
	"fmt"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
)

// validateCubeMapProgramTarget proves that the Go-specific semantic cube and
// the language-neutral program target describe the same selected program
// scope. It deliberately joins exact structural facts instead of relying on
// display names, array position, or the fact that both artifacts happened to
// be present in one run directory.
func validateCubeMapProgramTarget(analysis analysistarget.Target, target programindex.Target) error {
	if err := goadapter.ValidateTargetBinding(analysis, target); err != nil {
		return fmt.Errorf("cube map program target: %w", err)
	}
	return nil
}
