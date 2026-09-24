package report

import (
	"fmt"
	"path/filepath"
)

// SourceIDFunc names one openable source path of a run for a local report
// server session. The server owns the naming; publication only borrows it to
// render the served page early.
type SourceIDFunc func(runID, relativePath string) string

// ServedPage is a run's page as the local report server serves it: the
// published report data with a session source ID for every openable path,
// rendered with the run's local roots scrubbed.
type ServedPage struct {
	AnalysisRoot string
	HTML         []byte
	// Sources maps each session source ID the page carries to its path.
	Sources map[string]string
}

// RenderServedPage renders a run's served page from its receipt. It
// validates the report data as every render does.
func RenderServedPage(receipt RunReceipt, sourceID SourceIDFunc) (ServedPage, error) {
	return renderServedPage(receipt, sourceID, RenderHTMLWithOptions)
}

// PreparedServedPage is the served page publication rendered beside
// report.html, when it was asked to.
func (receipt RunReceipt) PreparedServedPage() (ServedPage, bool) {
	if receipt.served == nil {
		return ServedPage{}, false
	}
	return *receipt.served, true
}

func renderServedPage(
	receipt RunReceipt,
	sourceID SourceIDFunc,
	render func(*ReportData, RenderOptions) ([]byte, error),
) (ServedPage, error) {
	if receipt.data == nil || sourceID == nil {
		return ServedPage{}, fmt.Errorf("render report: served page needs report data and source naming")
	}
	runID := filepath.Base(receipt.runDir)
	reportData := *receipt.data
	manifest := receipt.manifest
	analysisRoot, err := manifest.ResolveAnalysisRoot()
	if err != nil {
		return ServedPage{}, fmt.Errorf("resolve analysis root: %w", err)
	}
	sources := make(map[string]string, len(reportData.OpenablePaths))
	sourceIDs := make(map[string]string, len(reportData.OpenablePaths))
	for _, relativePath := range reportData.OpenablePaths {
		id := sourceID(runID, relativePath)
		sources[id] = relativePath
		sourceIDs[relativePath] = id
	}
	reportData.SourceIDs = sourceIDs
	renderOptions := receipt.renderOptions
	renderOptions.LocalRoots = []string{receipt.runDir, analysisRoot, manifest.RepositoryState.Identity}
	rendered, err := render(&reportData, renderOptions)
	if err != nil {
		return ServedPage{}, fmt.Errorf("render report: %w", err)
	}
	return ServedPage{AnalysisRoot: analysisRoot, HTML: rendered, Sources: sources}, nil
}
