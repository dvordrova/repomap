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
}

func (e *AnalysisTargetSSAUnavailableError) Error() string {
	if e == nil {
		return "selected Go analysis target is unavailable for SSA"
	}
	switch e.Reason {
	case AnalysisTargetPackageNotSSASafe:
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

func IsAnalysisTargetSSAUnavailable(err error) bool {
	var target *AnalysisTargetSSAUnavailableError
	return errors.As(err, &target)
}
