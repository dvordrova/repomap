package surfacediscovery

import (
	"fmt"
)

type AnalysisTargetSSAUnavailableReason string

const (
	AnalysisTargetPackageNotSSASafe     AnalysisTargetSSAUnavailableReason = "target_package_not_ssa_safe"
	AnalysisTargetExactRootsUnavailable AnalysisTargetSSAUnavailableReason = "target_exact_roots_unavailable"
)

// AnalysisTargetSSAUnavailableError is fatal: the selected product target
// itself cannot own SSA-backed analysis under the requested build scenario.
// A safe sibling package must not make that run look ready.
type AnalysisTargetSSAUnavailableError struct {
	Reason        AnalysisTargetSSAUnavailableReason
	Package       string
	ExpectedRoots int
	ResolvedRoots int
	// Diagnostic is one complete, repository-relative package diagnostic from
	// the selected target's local dependency closure. It explains a concrete
	// build-input failure without weakening the fatal
	// target contract or attempting repository setup.
	Diagnostic *PackageDiagnostic
	// programCoverage is a process-local snapshot of the complete preparation
	// diagnostics and phase metrics. It is intentionally not public state on
	// the error: callers can only receive an independently owned snapshot.
	programCoverage *ProgramCoverage
}

// ProgramCoverageSnapshot returns complete process-local preparation coverage
// when this error was raised while validating the loaded target packages.
func (e *AnalysisTargetSSAUnavailableError) ProgramCoverageSnapshot() (ProgramCoverage, bool) {
	if e == nil || e.programCoverage == nil {
		return ProgramCoverage{}, false
	}
	return e.programCoverage.Snapshot(), true
}

func (e *AnalysisTargetSSAUnavailableError) bindProgramCoverage(coverage ProgramCoverage) {
	if e == nil {
		return
	}
	snapshot := coverage.Snapshot()
	e.programCoverage = &snapshot
}

func (e *AnalysisTargetSSAUnavailableError) Error() string {
	if e == nil {
		return "selected Go analysis target is unavailable for SSA"
	}
	switch e.Reason {
	case AnalysisTargetPackageNotSSASafe:
		if diagnostic := analysisTargetDiagnosticText(e.Diagnostic); diagnostic != "" {
			return fmt.Sprintf(
				"selected Go analysis target package %s is unavailable for SSA because package %s failed at %s; prepare missing generated/build inputs, choose another --target, or override the platform with --force-platform",
				e.Package, e.Diagnostic.Package, diagnostic,
			)
		}
		return fmt.Sprintf(
			"selected Go analysis target package %s is unavailable for SSA; choose another --target or correct --force-platform",
			e.Package,
		)
	case AnalysisTargetExactRootsUnavailable:
		return fmt.Sprintf(
			"selected Go analysis target package %s resolved %d of %d exact process roots in SSA; choose another --target or correct --force-platform",
			e.Package, e.ResolvedRoots, e.ExpectedRoots,
		)
	default:
		return fmt.Sprintf("selected Go analysis target package %s is unavailable for SSA", e.Package)
	}
}

func analysisTargetDiagnosticText(diagnostic *PackageDiagnostic) string {
	if diagnostic == nil || diagnostic.Location == nil ||
		diagnostic.Location.Path == "" || diagnostic.Location.Line <= 0 ||
		diagnostic.Message == "" {
		return ""
	}
	location := fmt.Sprintf("%s:%d", diagnostic.Location.Path, diagnostic.Location.Line)
	if diagnostic.Location.Column > 0 {
		location += fmt.Sprintf(":%d", diagnostic.Location.Column)
	}
	return location + ": " + diagnostic.Message
}
