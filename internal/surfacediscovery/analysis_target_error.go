package surfacediscovery

import (
	"errors"
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
	// Diagnostic is one already-bounded, repository-relative package
	// diagnostic from the selected target's local dependency closure. It
	// explains a concrete build-input failure without weakening the fatal
	// target contract or attempting repository setup.
	Diagnostic *PackageDiagnostic
}

func (e *AnalysisTargetSSAUnavailableError) Error() string {
	if e == nil {
		return "selected Go analysis target is unavailable for SSA"
	}
	switch e.Reason {
	case AnalysisTargetPackageNotSSASafe:
		if diagnostic := analysisTargetDiagnosticText(e.Diagnostic); diagnostic != "" {
			return fmt.Sprintf(
				"selected Go analysis target package %s is unavailable for SSA because package %s failed at %s; prepare missing generated/build inputs, or choose another --target/--go-target",
				e.Package, e.Diagnostic.Package, diagnostic,
			)
		}
		return fmt.Sprintf(
			"selected Go analysis target package %s is unavailable for SSA; choose another --target or correct --go-target",
			e.Package,
		)
	case AnalysisTargetExactRootsUnavailable:
		return fmt.Sprintf(
			"selected Go analysis target package %s resolved %d of %d exact process roots in SSA; choose another --target or correct --go-target",
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

func IsAnalysisTargetSSAUnavailable(err error) bool {
	var target *AnalysisTargetSSAUnavailableError
	return errors.As(err, &target)
}
