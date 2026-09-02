package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
	"github.com/dvordrova/repomap/internal/targetoutcome"
)

func TestReportInputScaleWarningsWritesOneAggregateAcrossTargetPages(t *testing.T) {
	var console bytes.Buffer
	reportInputScaleWarnings(newRunOutput(&console), []report.ReportInputScaleWarning{
		{Kind: report.ReportScaleWarningReportJSONBytes, Retained: 70, AdvisorySize: 64},
		{Kind: report.ReportScaleWarningReportJSONBytes, Retained: 90, AdvisorySize: 64},
		{Kind: report.ReportScaleWarningManifestBytes, Retained: 5, AdvisorySize: 4},
	}, programindex.Target{Language: "go", Name: "api", Selector: "./cmd/api"})
	output := console.String()
	for _, expected := range []string{
		"WARN", "Large report handoff retained",
		`target: language="go"; name="api"; selector="./cmd/api"`,
		"report_json_bytes: largest retained 90; usual size 64",
		"run_manifest_bytes: largest retained 5; usual size 4",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("console output missing %q:\n%s", expected, output)
		}
	}
	if strings.Count(output, "Large report handoff retained") != 1 ||
		strings.Count(output, "report_json_bytes:") != 1 {
		t.Fatalf("report warnings were not aggregated once by kind:\n%s", output)
	}
}

func TestReportInputScaleWarningsDeduplicatesByExactProgramTarget(t *testing.T) {
	var console bytes.Buffer
	output := newRunOutput(&console)
	warnings := []report.ReportInputScaleWarning{{
		Kind:         report.ReportScaleWarningReportJSONBytes,
		Retained:     report.MaxReportJSONBytes + 1,
		AdvisorySize: report.MaxReportJSONBytes,
	}}
	api := programindex.Target{
		ID: "program-api", Language: "go", Name: "api", Selector: "go:./cmd/api",
	}
	worker := programindex.Target{
		ID: "program-worker", Language: "go", Name: "worker", Selector: "go:./cmd/worker",
	}
	reportInputScaleWarnings(output, warnings, api)
	// This represents the later report-input scan for the same target.
	reportInputScaleWarnings(output, warnings, api)
	// A sibling crossing the same kind/advisory remains independently visible.
	reportInputScaleWarnings(output, warnings, worker)

	text := console.String()
	if got := strings.Count(text, report.ReportScaleWarningReportJSONBytes+":"); got != 2 {
		t.Fatalf("target-bound warning count = %d, want 2:\n%s", got, text)
	}
	for _, detail := range []string{
		`target: language="go"; name="api"; selector="go:./cmd/api"; program_id="program-api"`,
		`target: language="go"; name="worker"; selector="go:./cmd/worker"; program_id="program-worker"`,
	} {
		if !strings.Contains(text, detail) {
			t.Fatalf("target-bound warning missing %q:\n%s", detail, text)
		}
	}
}

func TestReportTargetPageRunScaleWarningsDoesNotRepeatAnAlreadyReportedKind(t *testing.T) {
	runDir := t.TempDir()
	path := filepath.Join(runDir, "report.html")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, int64(report.MaxOrdinaryReportHTMLBytes)+1); err != nil {
		t.Fatal(err)
	}
	run := targetPublishedRun{
		RunDir: runDir,
		ReportScaleWarnings: []report.ReportInputScaleWarning{{
			Kind: report.ReportScaleWarningReportHTMLBytes, AdvisorySize: report.MaxOrdinaryReportHTMLBytes,
			Retained: report.MaxOrdinaryReportHTMLBytes + 1,
		}},
		SelectedTargetDisplay: "api",
		SelectedTargetKey:     "go:./cmd/api",
	}
	var console bytes.Buffer
	reportTargetPageRunScaleWarnings(newRunOutput(&console), []targetPublishedRun{run}, nil, nil)
	output := console.String()
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "report_html_bytes:") {
			t.Fatalf("already reported kind was repeated after deferred render:\n%s", output)
		}
	}
	if strings.TrimSpace(output) != "" {
		t.Fatalf("already reported kinds produced new console noise:\n%s", output)
	}
}

func TestReportTargetPageRunScaleWarningsReachesConsoleAfterDeferredRender(t *testing.T) {
	runs := make([]targetPublishedRun, 2)
	for index := range runs {
		runs[index].RunDir = t.TempDir()
		runs[index].SelectedTargetDisplay = []string{"api", "worker"}[index]
		runs[index].SelectedTargetKey = []string{"go:./cmd/api", "go:./cmd/worker"}[index]
		path := filepath.Join(runs[index].RunDir, "report.html")
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Truncate(path, int64(report.MaxOrdinaryReportHTMLBytes)+1); err != nil {
			t.Fatal(err)
		}
	}
	var console bytes.Buffer
	reportTargetPageRunScaleWarnings(
		newRunOutput(&console),
		runs,
		[]report.ReportInputScaleWarning{{
			Kind: report.ReportScaleWarningBundleBytes, Retained: int(report.AdvisoryStandaloneTargetBundlePayloadBytes) + 1,
			AdvisorySize: int(report.AdvisoryStandaloneTargetBundlePayloadBytes),
		}},
		nil,
	)
	output := console.String()
	for _, expected := range []string{
		"WARN", "Large report handoff retained",
		"report_html_bytes: largest retained",
		"standalone_bundle_bytes: largest retained",
		"targets portfolio-wide",
		"targets api (go:./cmd/api), worker (go:./cmd/worker)",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("deferred report warning missing %q:\n%s", expected, output)
		}
	}
	if strings.Count(output, "Large report handoff retained") != 1 {
		t.Fatalf("deferred report warnings were noisy:\n%s", output)
	}
}

func TestReportTargetPageRunScaleWarningsKeepsTransportWarningOnExactTarget(t *testing.T) {
	runs := []targetPublishedRun{
		{
			SelectedTargetDisplay: "api", SelectedTargetKey: "go:./cmd/api",
			ProgramPage: report.TargetNavigationPage{ProgramTarget: programindex.Target{ID: "program-api"}},
		},
		{
			SelectedTargetDisplay: "worker", SelectedTargetKey: "python:worker",
			ProgramPage: report.TargetNavigationPage{ProgramTarget: programindex.Target{ID: "program-worker"}},
		},
	}
	var console bytes.Buffer
	reportTargetPageRunScaleWarnings(
		newRunOutput(&console), runs, nil,
		[]report.TargetReportScaleWarning{{
			SelectedTargetID: "selected-worker", ProgramTargetID: "program-worker",
			Warning: report.ReportInputScaleWarning{
				Kind:         report.ReportScaleWarningTargetBundleRawBytes,
				Retained:     int(report.AdvisoryStandaloneTargetPayloadBytes) + 1,
				AdvisorySize: int(report.AdvisoryStandaloneTargetPayloadBytes),
			},
		}},
	)
	output := console.String()
	if !strings.Contains(output, "targets worker (python:worker)") ||
		strings.Contains(output, "targets api (go:./cmd/api)") {
		t.Fatalf("target-bound warning was smeared across runs:\n%s", output)
	}
}

func TestReportSelectedTargetOutcomeScaleWarningsIncludesExactIdentity(t *testing.T) {
	languages := make([]string, targetoutcome.MaxAllowedProgramLanguages+1)
	for index := range languages {
		languages[index] = fmt.Sprintf("language-%02d", index)
	}
	selected, err := targetoutcome.NewSelectedTargetWithLanguages(
		targetoutcome.LanguageGroupPython,
		languages,
		targetoutcome.ScopeExecutable,
		"API",
		"python:api",
	)
	if err != nil {
		t.Fatal(err)
	}
	var console bytes.Buffer
	reportSelectedTargetOutcomeScaleWarnings(
		newRunOutput(&console), []targetoutcome.SelectedTarget{selected},
	)
	output := console.String()
	for _, expected := range []string{
		"Large selected-target language authority retained",
		`target: selected_id="` + selected.ID + `"; language_group="python"; name="API"; selector="python:api"`,
		"target_allowed_program_languages: affected collections 1",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("selected-target warning missing %q:\n%s", expected, output)
		}
	}
}

func TestReportGoFactScaleWarningsWritesOneNonFatalAggregate(t *testing.T) {
	var console bytes.Buffer
	reportGoFactScaleWarnings(newRunOutput(&console), &gofacts.Facts{Warnings: []string{
		"large retained Go facts: retained all 51 external import summaries; usual size is 50",
		"large retained Go facts: module . retained the complete 1048577-byte go.mod; usual size is 1048576 bytes",
		"unrelated warning",
	}})
	output := console.String()
	for _, expected := range []string{
		"WARN",
		"Large Go source facts retained",
		"all locally observed Go facts were retained",
		"retained all 51 external import summaries",
		"module . retained the complete 1048577-byte go.mod",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("console output missing %q:\n%s", expected, output)
		}
	}
	if strings.Contains(output, "unrelated warning") || strings.Count(output, "Large Go source facts retained") != 1 {
		t.Fatalf("console warning was not one scale-only aggregate:\n%s", output)
	}
}

func TestEmitProgramIndexScaleWarningsWritesOneRetainedDataWarningPerTarget(t *testing.T) {
	var console bytes.Buffer
	index := programindex.Index{Target: programindex.Target{Language: "go", Selector: "./cmd/server"}}
	emitProgramIndexScaleWarnings(newRunOutput(&console), index, []programindex.ScaleWarning{
		{
			Kind: programindex.ScaleWarningRelationPatterns, AdvisorySize: 64,
			AffectedCollections: 3, MaximumRetained: 247,
		},
		{
			Kind: programindex.ScaleWarningPatternArguments, AdvisorySize: 128,
			AffectedCollections: 1, MaximumRetained: 131,
		},
	})

	output := console.String()
	for _, text := range []string{
		"WARN",
		"Large ProgramIndex collections retained",
		`target: language="go"; name=""; selector="./cmd/server"`,
		"all rows retained; no local truncation was applied",
		"relation_patterns: affected collections 3; largest retained 247; usual size 64",
		"pattern_arguments: affected collections 1; largest retained 131; usual size 128",
	} {
		if !strings.Contains(output, text) {
			t.Fatalf("console output missing %q:\n%s", text, output)
		}
	}
	if strings.Count(output, "Large ProgramIndex collections retained") != 1 {
		t.Fatalf("console warning was not aggregated once:\n%s", output)
	}
}

func TestEmitProgramViewScaleWarningsWritesOneNonFatalAggregate(t *testing.T) {
	var console bytes.Buffer
	index := programindex.Index{Target: programindex.Target{Language: "python", Selector: "src/app"}}
	emitProgramViewScaleWarnings(newRunOutput(&console), index, []report.ProgramViewScaleWarning{
		{
			Kind: report.ProgramViewScaleWarningObjects, AdvisorySize: report.MaxProgramViewObjects,
			AffectedCollections: 1, MaximumRetained: report.MaxProgramViewObjects + 1,
		},
		{
			Kind: report.ProgramViewScaleWarningWitnesses, AdvisorySize: report.MaxProgramViewWitnessesPerRelation,
			AffectedCollections: 7, MaximumRetained: report.MaxProgramViewWitnessesPerRelation + 3,
		},
	})

	output := console.String()
	for _, expected := range []string{
		"WARN",
		"Large ProgramView retained",
		`target: language="python"; name=""; selector="src/app"`,
		"all ProgramView rows and relation witnesses retained",
		"objects: affected collections 1; largest retained 2049; usual size 2048",
		"witnesses_per_relation: affected collections 7; largest retained 7; usual size 4",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("console output missing %q:\n%s", expected, output)
		}
	}
	if strings.Count(output, "Large ProgramView retained") != 1 {
		t.Fatalf("console warning was not aggregated once:\n%s", output)
	}
}

func TestEmitDirectCallIndexScaleWarningsReportsCompleteRetainedGraph(t *testing.T) {
	var console bytes.Buffer
	emitDirectCallIndexScaleWarnings(newRunOutput(&console), "example.com/server", []surfacediscovery.DirectCallScaleWarning{
		{Kind: surfacediscovery.DirectCallScaleWarningDepth, AdvisorySize: 10, Retained: 13},
		{Kind: surfacediscovery.DirectCallScaleWarningEdges, AdvisorySize: 10_000, Retained: 12_345},
	})

	output := console.String()
	for _, expected := range []string{
		"WARN",
		"Large Go call graph retained",
		"target: example.com/server",
		"all rows admitted by the selected controls retained",
		"traversal_depth: retained 13; usual size 10",
		"edges: retained 12345; usual size 10000",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("console output missing %q:\n%s", expected, output)
		}
	}
}

func TestReportPackageDiagnosticScaleWarningsNamesExactTarget(t *testing.T) {
	diagnostics := make([]surfacediscovery.PackageDiagnostic, 129)
	diagnostics[0].Message = strings.Repeat("x", 513)
	var console bytes.Buffer
	reportPackageDiagnosticScaleWarnings(
		newRunOutput(&console),
		"server (go:example.com/server)",
		surfacediscovery.ProgramCoverage{PackageDiagnostics: diagnostics},
	)
	output := console.String()
	for _, expected := range []string{
		"WARN", "Large Go package diagnostics retained",
		"target: server (go:example.com/server)",
		"package_diagnostics: retained 129; usual size 128",
		"package_diagnostic_message_bytes: retained 513; usual size 512",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("console output missing %q:\n%s", expected, output)
		}
	}
}
