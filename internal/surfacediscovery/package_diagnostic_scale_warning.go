package surfacediscovery

// PackageDiagnosticScaleWarningKind names one former local diagnostic
// threshold. Both are log-only measurements over complete retained evidence.
type PackageDiagnosticScaleWarningKind string

const (
	PackageDiagnosticScaleWarningCount        PackageDiagnosticScaleWarningKind = "package_diagnostics"
	PackageDiagnosticScaleWarningMessageBytes PackageDiagnosticScaleWarningKind = "package_diagnostic_message_bytes"
)

// PackageDiagnosticScaleWarning reports a complete retained measurement.
type PackageDiagnosticScaleWarning struct {
	Kind         PackageDiagnosticScaleWarningKind
	AdvisorySize int
	Retained     int
}

// PackageDiagnosticScaleWarnings cannot fail and cannot affect analysis. It
// reports only former local sizes after all diagnostics and message bytes have
// already been retained.
func PackageDiagnosticScaleWarnings(coverage ProgramCoverage) []PackageDiagnosticScaleWarning {
	measurements := []PackageDiagnosticScaleWarning{{
		Kind: PackageDiagnosticScaleWarningCount, AdvisorySize: MaxPackageDiagnostics,
		Retained: len(coverage.PackageDiagnostics),
	}}
	maximumMessageBytes := 0
	for _, diagnostic := range coverage.PackageDiagnostics {
		if len(diagnostic.Message) > maximumMessageBytes {
			maximumMessageBytes = len(diagnostic.Message)
		}
	}
	measurements = append(measurements, PackageDiagnosticScaleWarning{
		Kind: PackageDiagnosticScaleWarningMessageBytes, AdvisorySize: advisoryDiagnosticBytes,
		Retained: maximumMessageBytes,
	})
	warnings := make([]PackageDiagnosticScaleWarning, 0, len(measurements))
	for _, measurement := range measurements {
		if measurement.Retained > measurement.AdvisorySize {
			warnings = append(warnings, measurement)
		}
	}
	return warnings
}
