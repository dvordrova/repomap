package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/report"
)

func TestRunOutputExplainsPartialMapConnectivityOutsideReport(t *testing.T) {
	t.Parallel()

	var console bytes.Buffer
	output := newRunOutput(&console)
	output.MapConnectivity(report.ArchitectureStructuralConnectivity{
		PackageImportFactCount:          44,
		ProjectedWitnessCount:           12,
		ProjectedPairEdgeCount:          7,
		SuppressedIntraComponentCount:   20,
		SuppressedUnjoinedEndpointCount: 12,
	})

	got := console.String()
	for _, want := range []string{
		"WARN",
		"Map package connectivity is partial",
		"exact package-import facts: 44",
		"projected: 12 witness(es) across 7 component-pair edge(s)",
		"retained inside one component (no Map edge): 20",
		"suppressed because an endpoint has no final component: 12",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("console output %q does not contain %q", got, want)
		}
	}
}

func TestRunOutputKeepsExpectedIntraComponentImportsInformational(t *testing.T) {
	t.Parallel()

	var console bytes.Buffer
	output := newRunOutput(&console)
	output.MapConnectivity(report.ArchitectureStructuralConnectivity{
		PackageImportFactCount:        5,
		SuppressedIntraComponentCount: 5,
	})

	got := console.String()
	if strings.Contains(got, "WARN") || !strings.Contains(got, "Map connectivity") ||
		!strings.Contains(got, "state: complete") {
		t.Fatalf("console output = %q", got)
	}
}

func TestRunOutputExplainsArchitectureScopeOmissionsInConsole(t *testing.T) {
	t.Parallel()

	var console bytes.Buffer
	output := newRunOutput(&console)
	output.ArchitectureScope(report.ArchitectureProductScope{
		ObservedModules: 2, RetainedModules: 1, OmittedModules: 1,
		ObservedPackages: 985, RetainedPackages: 92,
		ObservedEdges: 209, RetainedEdges: 206,
	})

	got := console.String()
	for _, want := range []string{
		"WARN",
		"Architecture model scope omits non-production modules",
		"modules retained: 1/2",
		"packages retained: 92/985",
		"exact package edges retained: 206/209",
		"whole non-production modules omitted: 1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("console output %q does not contain %q", got, want)
		}
	}
}
