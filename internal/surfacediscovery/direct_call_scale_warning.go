package surfacediscovery

// DirectCallScaleWarningKind names one aggregate graph measurement whose
// former ordinary threshold is now diagnostic only.
type DirectCallScaleWarningKind string

const (
	DirectCallScaleWarningDepth DirectCallScaleWarningKind = "traversal_depth"
	DirectCallScaleWarningEdges DirectCallScaleWarningKind = "edges"
	DirectCallScaleWarningNodes DirectCallScaleWarningKind = "nodes"
)

// DirectCallScaleWarning reports a complete retained measurement. AdvisorySize
// is never a filtering or validation authority.
type DirectCallScaleWarning struct {
	Kind         DirectCallScaleWarningKind
	AdvisorySize int
	Retained     int
}

// DirectCallScaleWarnings derives at most one warning for each formerly
// bounded graph dimension. It is a pure diagnostic: malformed input produces
// no error and can never affect target analysis or publication.
func DirectCallScaleWarnings(index DirectCallIndex) []DirectCallScaleWarning {
	if index.State != DirectCallIndexReady {
		return nil
	}
	measurements := [...]DirectCallScaleWarning{
		{Kind: DirectCallScaleWarningDepth, AdvisorySize: AdvisoryDirectCallMaxDepth, Retained: index.Coverage.TraversalDepthReached},
		{Kind: DirectCallScaleWarningEdges, AdvisorySize: AdvisoryDirectCallMaxEdges, Retained: len(index.Edges)},
		{Kind: DirectCallScaleWarningNodes, AdvisorySize: AdvisoryDirectCallMaxNodes, Retained: len(index.Nodes)},
	}
	warnings := make([]DirectCallScaleWarning, 0, len(measurements))
	for _, measurement := range measurements {
		if measurement.Retained > measurement.AdvisorySize {
			warnings = append(warnings, measurement)
		}
	}
	return warnings
}
