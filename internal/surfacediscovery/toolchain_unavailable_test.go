package surfacediscovery

import (
	"errors"
	"fmt"
	"testing"
)

// Decision 235 (v11) 1C: toolchain acquisition failure is a typed error
// (analysis_toolchain_unavailable) — the generic snapshot/report remains
// available downstream.
func TestAnalysisToolchainUnavailableTyped(t *testing.T) {
	base := errors.New("go: downloading go1.99.0: module go@1.99.0: git ls-remote failed")
	err := &analysisToolchainUnavailableError{cause: base, module: "cmd/app"}
	if !IsAnalysisToolchainUnavailable(err) {
		t.Fatal("typed toolchain error not recognized")
	}
	if !errors.Is(err, base) {
		t.Fatal("typed error does not unwrap the cause")
	}
	if !toolchainAcquisitionError(base) {
		t.Fatal("toolchain acquisition message not detected")
	}
	if toolchainAcquisitionError(fmt.Errorf("surface discovery: load packages: exit status 1")) {
		t.Fatal("ordinary load error misclassified as toolchain acquisition")
	}
	if IsAnalysisToolchainUnavailable(fmt.Errorf("ordinary load failure")) {
		t.Fatal("ordinary load failure misclassified as toolchain unavailable")
	}
}
