package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

func TestCurrentPipelineRendersOrientationVerticalSlice(t *testing.T) {
	index := reportProgramIndexFixture(t, "python", "executable")
	portfolio, err := NewProgramPortfolio(index.Target.ID, []programindex.Index{index})
	if err != nil {
		t.Fatal(err)
	}
	coreView, _ := reportCoreMapFixture(t, index)
	activityView, activityRaw := reportActivityEntrypointFixture(t, index)
	integrationUsageView, _, selectedRaw, usageRaw := reportIntegrationUsageFixture(t, index)
	activityPathView, _ := reportActivityPathFixture(t, index, activityRaw, selectedRaw, usageRaw)
	data := &ReportData{
		FormatVersion:          CurrentFormatVersion,
		RepoName:               "fixture",
		CapturedRevision:       strings.Repeat("a", 40),
		ProgramPortfolio:       portfolio,
		CoreMapView:            coreView,
		ActivityEntrypointView: activityView,
		IntegrationUsageView:   integrationUsageView,
		ActivityPathView:       activityPathView,
		OpenablePaths: []string{
			"app/__init__.py", "app/main.py", "scripts/clean.py",
			"storage/__init__.py", "storage/db.py",
		},
	}

	html, err := RenderHTMLWithOptions(data, RenderOptions{})
	if err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	for _, token := range [][]byte{
		[]byte(`id="rm-app" aria-live="polite"`),
		[]byte(`id="rm-report-app-css"`),
		[]byte(`id="rm-report-app-js"`),
		[]byte(`buildPresentationModel(data)`),
		[]byte(`Repository orientation`),
		[]byte(`Repository flow`),
		[]byte(`Entrypoints`),
		[]byte(`Integrations`),
		[]byte(`How the code connects`),
		[]byte(`Verify in code`),
		[]byte(`Evidence limits`),
		[]byte(`sourceAction(symbol.name, symbol.location)`),
		[]byte(`sourceAction('Open file', { path: file.path, line: 0, column: 0 })`),
		[]byte(`function canvasTopology(activeBlockIDs, complete)`),
		[]byte(`function renderAreaSwitcher(selected, activeGroup)`),
		[]byte(`Show complete map · `),
		[]byte(`data-canvas-node`),
		[]byte(`rm-evidence-disclosure`),
		[]byte(`rm-core-group`),
		[]byte(`coreCanvasGroup(group, selected, expanded)`),
		[]byte(`var control = element('a', 'rm-canvas-node rm-canvas-node--' + kind);`),
		[]byte(`function buildActivityPaths(data, target, indexSHA256, activitiesByID, integrationUsesByKey)`),
		[]byte(`function renderActivityDetail(host, activity)`),
		[]byte(`function renderIntegrationDetail(host, integration)`),
		[]byte(`function routeForActivity(id)`),
		[]byte(`Responsibilities`),
		[]byte(`Selected operations`),
		[]byte(`Paths to selected integrations`),
		[]byte(`Mechanism`),
	} {
		if !bytes.Contains(html, token) {
			t.Errorf("rendered orientation workspace is missing %q", token)
		}
	}
	for _, retired := range [][]byte{
		[]byte(`id="rm-program-shell-css"`),
		[]byte(`id="rm-program-map-js"`),
		[]byte(`id="rm-program-contract-js"`),
		[]byte(`id="rm-program-overview-js"`),
		[]byte(`global.RepomapProgramContract`),
		[]byte(`window.RepomapProgramMap`),
		[]byte(`At a glance`),
		[]byte(`Program graphs`),
		[]byte(`Coverage and limits`),
		[]byte(`Raw ProgramIndex`),
		[]byte(`Choose a direction`),
		[]byte(`rm-canvas-popover`),
		[]byte(`element('button', 'rm-canvas-node`),
	} {
		if bytes.Contains(html, retired) {
			t.Errorf("rendered orientation workspace retained old frontend %q", retired)
		}
	}
}

func TestOrientationClientKeepsCompleteEvidenceBehindDisclosure(t *testing.T) {
	for _, required := range []string{
		"starts.forEach(function (start)",
		"connections.forEach(function (connection)",
		"related.forEach(function (candidate)",
		"block.symbols.forEach(function (symbol)",
		"block.files.forEach(function (file)",
		"element('details', 'rm-disclosure')",
		"element('details', 'rm-evidence-disclosure')",
	} {
		if !strings.Contains(reportAppJS, required) {
			t.Errorf("orientation progressive disclosure is missing %q", required)
		}
	}
	for _, truncated := range []string{
		"connections.slice(0, 7)",
		"filter(Boolean).slice(0, 4)",
		"starts.slice(0, 5)",
	} {
		if strings.Contains(reportAppJS, truncated) {
			t.Errorf("orientation client truncates navigable evidence with %q", truncated)
		}
	}
}

func TestOrientationClientRendersRuntimePortfolioBeforeProgramMap(t *testing.T) {
	for _, required := range []string{
		"object(data.runtime_portfolio, 'runtime_portfolio')",
		"The runtime portfolio version is not supported",
		"function buildTargetDirectory(data, currentTarget)",
		"href: '#/program'",
		"link.href = implementation.target.href",
		"sourceAction(evidence.label, evidence.location)",
		"function renderRuntimePortfolio(host)",
		"Primary runtime roles",
		"Supporting and optional roles",
		"Uncertain runtime roles",
		"Unclassified targets",
		"Target mapping unresolved",
		"hash === '#/repository'",
		"if (hash === '#/program')",
		"#/program/responsibility/",
		"Back to runtime overview",
	} {
		if !strings.Contains(reportAppJS, required) {
			t.Errorf("runtime portfolio client is missing %q", required)
		}
	}
	for _, truncated := range []string{
		"runtime.roles.slice(",
		"role.implementations.slice(",
		"role.evidence.slice(",
		"runtime.unclassified.slice(",
	} {
		if strings.Contains(reportAppJS, truncated) {
			t.Errorf("runtime portfolio client truncates complete contract rows with %q", truncated)
		}
	}
	for _, selector := range []string{
		".rm-runtime-section", ".rm-runtime-grid", ".rm-runtime-card",
		".rm-runtime-target", ".rm-runtime-unclassified__item",
	} {
		if !strings.Contains(reportAppCSS, selector) {
			t.Errorf("runtime portfolio CSS is missing %q", selector)
		}
	}
	if !strings.Contains(programTemplateHTML, `href="#/repository"`) {
		t.Error("report brand does not return to the repository runtime overview")
	}
}

func TestOrientationClientRendersExactJSTSSurfacesAndCrossSurfacePaths(t *testing.T) {
	for _, required := range []string{
		"function buildJSTSSurfaceCatalog(data, target, indexSHA256, openable)",
		"js_ts_surface_catalog_view.version') !== 2",
		"data.format_version, 'format_version') !== 65",
		"function buildCrossSurfacePaths(data, target, indexSHA256, openable, surfaceCatalog)",
		"['browser_application', 'node_server', 'command_line_application', 'shared_contracts', 'tool', 'unknown']",
		"if (value === 'command_line_application') return 'Command-line application'",
		"['product_surface', 'supporting_code', 'tool', 'unknown']",
		"http_method_path_match",
		"['exact_static', 'resolved_indirect', 'possible', 'unresolved_frontier']",
		"function routeForSurface(id)",
		"function routeForCrossSurfacePath(id)",
		"#/program/surface/",
		"#/program/path/",
		"function renderJSTSRepositoryOverview(host)",
		"function renderSurfaceDetail(host, surface)",
		"function renderCrossSurfacePathDetail(host, path)",
		"sourceAction('Open surface evidence', surface.location)",
		"sourceAction('Open exact step', step.location)",
		"Open semantic program map →",
		"A cross-surface path must cite exact browser and server product surfaces",
		"JavaScript/TypeScript surface and path authority must be published together",
	} {
		if !strings.Contains(reportAppJS, required) {
			t.Errorf("JavaScript/TypeScript report client is missing %q", required)
		}
	}
	for _, truncated := range []string{
		"catalog.surfaces.slice(",
		"crossSurfacePaths.paths.slice(",
		"path.steps.slice(",
		"surface.entryRefs.slice(",
		"surface.evidenceRefs.slice(",
	} {
		if strings.Contains(reportAppJS, truncated) {
			t.Errorf("JavaScript/TypeScript client truncates exact contract rows with %q", truncated)
		}
	}
	for _, selector := range []string{
		".rm-surface-grid", ".rm-surface-card", ".rm-cross-path-card",
		".rm-path-timeline", ".rm-path-step--http-boundary",
	} {
		if !strings.Contains(reportAppCSS, selector) {
			t.Errorf("JavaScript/TypeScript report CSS is missing %q", selector)
		}
	}
}

func TestRenderRejectsProgramViewForAnotherTarget(t *testing.T) {
	index := reportProgramIndexFixture(t, "python", "executable")
	portfolio, err := NewProgramPortfolio(index.Target.ID, []programindex.Index{index})
	if err != nil {
		t.Fatal(err)
	}
	portfolio.Entries[0].View.TargetID = "program-target-" + strings.Repeat("0", 64)

	_, err = RenderHTMLWithOptions(&ReportData{
		FormatVersion:      CurrentFormatVersion,
		RepoName:           "fixture",
		CapturedRevision:   strings.Repeat("a", 40),
		CapturedInputCount: 0,
		ProgramPortfolio:   portfolio,
	}, RenderOptions{})
	if err == nil || !strings.Contains(err.Error(), "target/view identity mismatch") {
		t.Fatalf("RenderHTML error = %v", err)
	}
}

func TestRenderHTMLRequiresProgramPortfolio(t *testing.T) {
	_, err := RenderHTMLWithOptions(&ReportData{
		FormatVersion: CurrentFormatVersion,
		RepoName:      "legacy-fallback-is-forbidden",
	}, RenderOptions{})
	if err == nil || !strings.Contains(err.Error(), "requires a complete program portfolio") {
		t.Fatalf("RenderHTML error = %v", err)
	}
}

func TestRenderHTMLRequiresExactPersistedAuthority(t *testing.T) {
	base := reportProgramShellDataFixture(t, "fixture")
	tests := map[string]struct {
		mutate func(*ReportData)
		want   string
	}{
		"blank repository": {
			mutate: func(data *ReportData) { data.RepoName = "" },
			want:   "repository name",
		},
		"spaced repository": {
			mutate: func(data *ReportData) { data.RepoName = " fixture " },
			want:   "repository name",
		},
		"missing revision": {
			mutate: func(data *ReportData) { data.CapturedRevision = "" },
			want:   "captured revision",
		},
		"negative input count": {
			mutate: func(data *ReportData) { data.CapturedInputCount = -1 },
			want:   "captured input count",
		},
		"noncanonical revision": {
			mutate: func(data *ReportData) { data.CapturedRevision = strings.Repeat("A", 40) },
			want:   "canonical lowercase",
		},
		"empty warning": {
			mutate: func(data *ReportData) { data.Warnings = []string{""} },
			want:   "warning 0",
		},
		"invalid openable path": {
			mutate: func(data *ReportData) { data.OpenablePaths = []string{"../escape.go"} },
			want:   "openable path",
		},
		"unsorted openable paths": {
			mutate: func(data *ReportData) { data.OpenablePaths = []string{"z.go", "a.go"} },
			want:   "uniquely sorted",
		},
		"invalid served source ID": {
			mutate: func(data *ReportData) { data.SourceIDs = map[string]string{"app/main.py": "invalid"} },
			want:   "served source authority",
		},
		"incomplete served source IDs": {
			mutate: func(data *ReportData) {
				data.SourceIDs = map[string]string{"app/main.py": strings.Repeat("a", 43)}
			},
			want: "does not cover every openable path",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := base
			test.mutate(&candidate)
			if _, err := RenderHTMLWithOptions(&candidate, RenderOptions{}); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("RenderHTML authority error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestOrientationClientKeepsStrictSourceActionsWithoutLegacyFallbacks(t *testing.T) {
	for _, required := range []string{
		"multiple source authorities",
		"The source authority revision does not match the report",
		"/api/open",
		"X-Repomap-Action",
		"source_id: sourceID",
		"Open the exact captured revision in",
		"Opened the exact source location in VS Code",
		"Source evidence is outside publication authority",
	} {
		if !strings.Contains(reportAppJS, required) {
			t.Errorf("orientation source contract is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"navigator.clipboard",
		"innerHTML",
		"rm-source-drawer",
		"embeddedSourceForLocation",
		"artifact_filename",
		"validateProgramCoverage",
		"makeProgramContext",
		"RepomapProgramMap",
	} {
		if strings.Contains(reportAppJS, forbidden) {
			t.Errorf("orientation client retained old fallback or full browser contract %q", forbidden)
		}
	}
}

func TestOrientationClientBuildsMeaningBeforeRenderingEvidence(t *testing.T) {
	for _, required := range []string{
		"flattenBlocks(array(core.refined_core",
		"activityByID[id]",
		"blocksBySymbol[symbol.id]",
		"array(core.refined_groups",
		"groupByBlock[blockID] = group",
		"activityByID[id] = start",
		"integrations.push(integration)",
		"state.model.blocksBySymbol[use.callerID]",
		"connectionsFor(selected)",
		"relatedBlocksFor(selected, connections)",
		"runtime target unresolved",
		"A semantic responsibility map is not available for this target",
	} {
		if !strings.Contains(reportAppJS, required) {
			t.Errorf("orientation presentation model is missing %q", required)
		}
	}
	for _, datasetUI := range []string{
		"renderRawProgramIndex",
		"renderCoreMapCoverage",
		"renderIntegrationUsageCoverage",
		"producerCoverageCard",
		"metricChip",
	} {
		if strings.Contains(reportAppJS, datasetUI) {
			t.Errorf("orientation client exposes a pipeline dataset UI %q", datasetUI)
		}
	}
}

func TestOrientationClientSupportsOverlappingCoreGroups(t *testing.T) {
	for _, required := range []string{
		"groupsByBlock[blockID].push(group)",
		"state.model.groupsByBlock[block.id]",
		"local_unassigned",
	} {
		if !strings.Contains(reportAppJS, required) {
			t.Errorf("group browser model is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"A responsibility belongs to several refined core groups.",
		"complete responsibility partition",
	} {
		if strings.Contains(reportAppJS, forbidden) {
			t.Errorf("overlapping group browser model retains disjoint-partition rejection %q", forbidden)
		}
	}
	if !strings.Contains(reportAppCSS, ".rm-core-group--local-unassigned") {
		t.Error("local unassigned group presentation is missing")
	}
}

func reportProgramShellDataFixture(t *testing.T, repoName string) ReportData {
	t.Helper()
	index := reportProgramIndexFixture(t, "python", "executable")
	portfolio, err := NewProgramPortfolio(index.Target.ID, []programindex.Index{index})
	if err != nil {
		t.Fatal(err)
	}
	coreView, _ := reportCoreMapFixture(t, index)
	activityView, activityRaw := reportActivityEntrypointFixture(t, index)
	integrationUsageView, _, selectedRaw, usageRaw := reportIntegrationUsageFixture(t, index)
	activityPathView, _ := reportActivityPathFixture(t, index, activityRaw, selectedRaw, usageRaw)
	return ReportData{
		FormatVersion:          CurrentFormatVersion,
		RepoName:               repoName,
		CapturedRevision:       strings.Repeat("a", 40),
		CapturedInputCount:     0,
		ProgramPortfolio:       portfolio,
		CoreMapView:            coreView,
		ActivityEntrypointView: activityView,
		IntegrationUsageView:   integrationUsageView,
		ActivityPathView:       activityPathView,
		OpenablePaths: []string{
			"app/__init__.py", "app/main.py", "scripts/clean.py",
			"storage/__init__.py", "storage/db.py",
		},
	}
}
