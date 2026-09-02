package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/programindex"
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

func TestActiveSemanticOrdinalWarningKeepsExactTargetIdentity(t *testing.T) {
	var console bytes.Buffer
	target := programindex.Target{
		ID: "program-api", Language: "go", Name: "api", Selector: "go:./cmd/api",
	}
	reportSemanticOrdinalScaleWarnings(
		newRunOutput(&console),
		"Program grouping",
		[]string{semanticWarningTargetDetail(target)},
		[]debugdump.SemanticOrdinalScaleWarning{{
			Kind:         debugdump.SemanticScaleWarningAttemptOrdinal,
			Retained:     debugdump.MaxSemanticAttemptOrdinal + 1,
			AdvisorySize: debugdump.MaxSemanticAttemptOrdinal,
		}},
	)

	output := console.String()
	for _, expected := range []string{
		"Program grouping model journal scale",
		`target: language="go"; name="api"; selector="go:./cmd/api"; program_id="program-api"`,
		"semantic_attempt_ordinal: largest retained 257; former usual size 256",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("semantic ordinal warning missing %q:\n%s", expected, output)
		}
	}
}
