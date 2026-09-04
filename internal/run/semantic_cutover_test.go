package run

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/dependencies"
)

func TestPythonDependencyCoverageErrorKeepsExactFirstOmission(t *testing.T) {
	if err := pythonDependencyCoverageError(dependencies.Catalog{
		Coverage: dependencies.Coverage{State: dependencies.CoverageComplete},
	}); err != nil {
		t.Fatalf("complete coverage: %v", err)
	}

	err := pythonDependencyCoverageError(dependencies.Catalog{Coverage: dependencies.Coverage{
		State: dependencies.CoveragePartial,
		Omissions: []dependencies.Omission{
			{PackagePath: "runtime.dynamic", Reason: dependencies.OmissionDependencyIdentityMissing},
			{PackagePath: "runtime.optional", Reason: dependencies.OmissionDependencyMetadataMissing},
		},
	}})
	if err == nil || !strings.Contains(err.Error(), "dependency_identity_missing for runtime.dynamic") ||
		!strings.Contains(err.Error(), "and 1 more") {
		t.Fatalf("partial coverage error = %v", err)
	}
}
