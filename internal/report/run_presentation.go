package report

import (
	"errors"
	"fmt"
	"strings"
)

// PrepareRunPresentation builds the complete deterministic render projection
// from one canonical report. It never mutates canonical: transient metadata,
// product coherence, and exact workspace search are attached only to a deep
// clone used by presentation adapters and localization identity. Opaque
// navigation indexes and source IDs are attached after this seam so they
// cannot change translation requests or cache identity.
func PrepareRunPresentation(
	runDir string,
	canonical *ReportData,
	sourceEpisodeJSON []byte,
) (*ReportData, error) {
	if strings.TrimSpace(runDir) == "" {
		return nil, fmt.Errorf("report presentation: run directory is required")
	}
	if canonical == nil {
		return nil, fmt.Errorf("report presentation: canonical report is required")
	}
	prepared, err := cloneReportData(canonical)
	if err != nil {
		return nil, fmt.Errorf("report presentation: clone canonical report: %w", err)
	}
	var preparationErrors []error
	if err := HydrateRunPresentationMetadata(runDir, prepared); err != nil {
		preparationErrors = append(
			preparationErrors,
			fmt.Errorf("hydrate transient metadata: %w", err),
		)
	}
	ApplyProductCoherence(prepared)
	if len(sourceEpisodeJSON) > 0 {
		if err := AttachSourceEpisodePresentation(prepared, sourceEpisodeJSON); err != nil {
			preparationErrors = append(
				preparationErrors,
				fmt.Errorf("attach source episode presentation: %w", err),
			)
		}
	}
	return prepared, errors.Join(preparationErrors...)
}
