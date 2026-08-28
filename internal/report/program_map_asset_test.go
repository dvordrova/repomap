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
		[]byte(`id="rm-system-canvas-graph-js"`),
		[]byte(`id="rm-system-canvas-interaction-js"`),
		[]byte(`id="rm-system-canvas-geometry-js"`),
		[]byte(`id="rm-system-canvas-renderer-js"`),
		[]byte(`id="rm-report-loader-js"`),
		[]byte(`id="rm-report-app-js"`),
		[]byte(`buildRepositoryPresentationModel(repositoryPayload)`),
		[]byte(`buildTargetPresentationModel(payload, state.repositoryModel)`),
		[]byte(`Repository overview`),
		[]byte(`Repository flow`),
		[]byte(`Entrypoints`),
		[]byte(`Integrations`),
		[]byte(`How the code connects`),
		[]byte(`Verify in code`),
		[]byte(`Evidence limits`),
		[]byte(`function renderEvidence(block)`),
		[]byte(`function renderConnections(parent, block, connections)`),
		[]byte(`function connectionOwnerObject(connection)`),
		[]byte(`function groupConnectionsByOwner(connections)`),
		[]byte(`function targetOverviewCounts(model)`),
		[]byte(`element('details', 'rm-target-overview')`),
		[]byte(`displayProgramObjectName(symbol)`),
		[]byte(`id="rm-target-switcher"`),
		[]byte(`id="rm-page-context"`),
		[]byte(`function reportRouteContext(route)`),
		[]byte(`function buildCanvasGraph(presentationModel, rawOptions)`),
		[]byte(`function deriveCanvasEmphasis(rawGraph, rawInteraction)`),
		[]byte(`function portLayoutRequirements(rawGraph)`),
		[]byte(`function measureCanvasNodes(host, nodeElements)`),
		[]byte(`function mountSystemCanvas(host, graph, suppliedOptions, suppliedCallbacks)`),
		[]byte(`function renderEntrypointGroups(nodes, graph, options, callbacks)`),
		[]byte(`function renderAreaSwitcher(selected, activeGroup)`),
		[]byte(`allButton.setAttribute('data-area-selection', allSelectionID)`),
		[]byte(`appendText(allButton, 'span', '', 'All')`),
		[]byte(`navigateToBlock(state.model.blocksByID[group.blockIDs[0]], group, false)`),
		[]byte(`data-canvas-node`),
		[]byte(`data-canvas-edge-port`),
		[]byte(`rm-evidence-disclosure`),
		[]byte(`rm-core-group`),
		[]byte(`function renderCoreGroup(nodes, graph, options, callbacks)`),
		[]byte(`return renderer.mountSystemCanvas(canvasHost, graph`),
		[]byte(`function buildActivityPaths(raw, activitiesByID, integrationUsesByKey)`),
		[]byte(`object(raw, 'target features.activity_paths')`),
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
		[]byte(`function canvasTopology(`),
		[]byte(`function drawCanvasEdges(`),
		[]byte(`function scheduleCanvasDraw(`),
		[]byte(`function highlightCanvasNode(`),
		[]byte(`state.canvasEdges`),
		[]byte(`sourceAction('Open file'`),
		[]byte(`Open source location `),
		[]byte(`rm-connection__meta`),
		[]byte(`id="rm-target-scope"`),
		[]byte(`Show complete map`),
		[]byte(`Focus current grouping selection`),
		[]byte(`rm-canvas-mode`),
		[]byte(`id="rm-report-data"`),
		[]byte(`buildPresentationModel(data)`),
		[]byte(`model.raw`),
	} {
		if bytes.Contains(html, retired) {
			t.Errorf("rendered orientation workspace retained old frontend %q", retired)
		}
	}
}

func TestOrientationClientReframesJoinedActivityPathsAsOptionalExecutionStory(t *testing.T) {
	for _, required := range []string{
		"function renderExecutionStory(activity, routes)",
		"if (!routes.length) return null",
		"route.activityID === activity.id",
		"outcome.use.authority === 'exact_external_symbol'",
		"if (outcomeIndex === 0)",
		"Follow execution",
		"Possible callback handoff",
		"Available facts do not prove an exact transfer",
		"Observed exact external callsites from this caller",
		"data-execution-node",
		"data-execution-edge",
		"not represented in available facts",
	} {
		if !strings.Contains(reportAppJS, required) {
			t.Errorf("optional execution-story presentation is missing %q", required)
		}
	}
	for _, selector := range []string{
		".rm-execution-story__header",
		".rm-execution-story__key--exact",
		".rm-execution-story__key--possible",
		".rm-execution-story-node__body",
		".rm-execution-story-connector--possible",
		".rm-execution-story__boundary",
		".rm-execution-story__outcomes--fanout",
		".rm-execution-story .rm-source-action__location",
		".rm-execution-story__not-represented",
		".rm-execution-story-outcome__actions",
	} {
		if !strings.Contains(reportAppCSS, selector) {
			t.Errorf("optional execution-story presentation is missing %q", selector)
		}
	}
	for _, duplicateAuthority := range []string{"features.execution_story", "ExecutionStoryView"} {
		if strings.Contains(reportAppJS, duplicateAuthority) {
			t.Errorf("execution-story presentation introduced duplicate browser authority %q", duplicateAuthority)
		}
	}
	if strings.Contains(reportAppJS, "External outcome") {
		t.Error("execution-story presentation must not promote an exact external callsite into a runtime outcome")
	}
}

func TestSystemCanvasAssetsKeepIsolatedOwnership(t *testing.T) {
	scriptIDs := []string{
		`id="rm-report-loader-js"`,
		`id="rm-system-canvas-graph-js"`,
		`id="rm-system-canvas-interaction-js"`,
		`id="rm-system-canvas-geometry-js"`,
		`id="rm-system-canvas-renderer-js"`,
		`id="rm-report-app-js"`,
	}
	previous := -1
	for _, scriptID := range scriptIDs {
		position := strings.Index(programTemplateHTML, scriptID)
		if position < 0 {
			t.Fatalf("program report template is missing %q", scriptID)
		}
		if position <= previous {
			t.Fatalf("program report script %q is not embedded after the preceding System canvas layer", scriptID)
		}
		previous = position
	}

	assets := map[string]string{
		"report loader": reportLoaderJS,
		"graph":         systemCanvasGraphJS,
		"interaction":   systemCanvasInteractionJS,
		"geometry":      systemCanvasGeometryJS,
		"renderer":      systemCanvasRendererJS,
		"report app":    reportAppJS,
	}
	for name, source := range assets {
		if strings.Contains(strings.ToLower(source), "</script") {
			t.Errorf("embedded %s asset contains an inline-script breakout", name)
		}
	}

	assertOmits := func(name, source string, forbidden ...string) {
		t.Helper()
		for _, token := range forbidden {
			if strings.Contains(source, token) {
				t.Errorf("%s asset crosses its ownership boundary with %q", name, token)
			}
		}
	}
	for name, source := range map[string]string{
		"graph":       systemCanvasGraphJS,
		"interaction": systemCanvasInteractionJS,
	} {
		assertOmits(name, source,
			"document.",
			"window.location",
			"location.hash",
			"state.model",
		)
	}
	assertOmits("geometry", systemCanvasGeometryJS,
		"presentationModel",
		"blocksBySymbol",
		"buildCanvasGraph",
		"state.model",
		"document.",
		"window.location",
		"location.hash",
		"rm-report-data",
	)
	assertOmits("renderer", systemCanvasRendererJS,
		"presentationModel",
		"blocksBySymbol",
		"buildCanvasGraph",
		"buildPresentationModel",
		"state.model",
		"window.location",
		"location.hash",
		"hashchange",
		"routeFor",
		"navigateTo",
		"rm-report-data",
	)
	assertOmits("report app", reportAppJS,
		"function canvasTopology(",
		"function drawCanvasEdges(",
		"function scheduleCanvasDraw(",
		"rm-report-data",
		"rm-bundle-index",
		"rm-repository-payload",
		"rm-target-chunk-",
		"model.raw",
	)
}

func TestOrientationClientKeepsCompleteEvidenceBehindDisclosure(t *testing.T) {
	for _, required := range []string{
		"starts.forEach(function (start)",
		"groupConnectionsByOwner(connections)",
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
		"ownerGroups.slice(",
		"group.local.slice(",
		"group.platform.slice(",
		"group.external.slice(",
		"group.unresolved.slice(",
		"filter(Boolean).slice(0, 4)",
		"starts.slice(0, 5)",
	} {
		if strings.Contains(reportAppJS, truncated) {
			t.Errorf("orientation client truncates navigable evidence with %q", truncated)
		}
	}
	for _, retired := range []string{
		"sourceAction('Open file'",
		"Open source location ",
		"rm-connection__meta",
	} {
		if strings.Contains(reportAppJS, retired) {
			t.Errorf("orientation evidence retained redundant presentation %q", retired)
		}
	}
	for _, selector := range []string{
		".rm-source-action--compact", ".rm-evidence-files", ".rm-evidence-file",
		".rm-evidence-file__path", ".rm-evidence-symbols", ".rm-connection-sites",
		".rm-connection-owner-list", ".rm-connection-owner__members", ".rm-connection-member-group__heading",
		".rm-connection-member__targets", ".rm-connection-target__link", ".rm-connection-runtime__summary",
		".rm-flow-canvas[data-canvas-highlight] .rm-canvas-edge-group", ".rm-canvas-edge-port--related", ".rm-canvas-node--edge-related",
		".rm-canvas-node-wrap:hover > .rm-canvas-node",
		".rm-canvas-entry-file__header", ".rm-canvas-entry-file__nodes", ".rm-canvas-node--entry",
		".rm-canvas-edge-port--active::before", ".rm-canvas-arrow--exact",
	} {
		if !strings.Contains(reportAppCSS, selector) {
			t.Errorf("grouped exact-source presentation is missing %q", selector)
		}
	}
	for _, compactSignatureRule := range []string{
		".rm-start__signature", "max-height: 2.9em", "-webkit-line-clamp: 2", "line-clamp: 2",
	} {
		if !strings.Contains(reportAppCSS, compactSignatureRule) {
			t.Errorf("compact entrypoint signature presentation is missing %q", compactSignatureRule)
		}
	}
}

func TestOrientationClientUsesCompactTargetSwitcherAndRouteContext(t *testing.T) {
	for _, required := range []string{
		`<details class="rm-target-switcher" id="rm-target-switcher">`,
		`<summary>`,
		`id="rm-target-repository"`,
		`id="rm-target-current"`,
		`id="rm-target-count"`,
		`id="rm-target-panel-count"`,
		`id="rm-target-navigation"`,
		`id="rm-page-context"`,
	} {
		if !strings.Contains(programTemplateHTML, required) {
			t.Errorf("compact target switcher template is missing %q", required)
		}
	}
	for _, required := range []string{
		"function shortRepositoryName(value)",
		"function reportRouteContext(route)",
		"function updateHeaderContext(route)",
		"function renderHeader()",
		"state.model.defaultTargetID",
		"rm-target-switcher__badge--current",
		"if (event.key !== 'Escape' || !switcher.open) return",
		"updateHeaderContext(route)",
	} {
		if !strings.Contains(reportAppJS, required) {
			t.Errorf("compact target switcher client is missing %q", required)
		}
	}
	for _, selector := range []string{
		".rm-target-switcher", ".rm-target-switcher__panel", ".rm-target-switcher__targets",
		".rm-target-switcher__target", ".rm-target-switcher__badges", ".rm-target-switcher__badge",
		".rm-target-overview", ".rm-target-overview__summary", ".rm-target-overview__body",
	} {
		if !strings.Contains(reportAppCSS, selector) {
			t.Errorf("compact target switcher CSS is missing %q", selector)
		}
	}
	for _, retired := range []string{
		`id="rm-target-scope"`, `class="rm-site-header__targets"`,
		`aria-label="Choose repository target"`,
	} {
		if strings.Contains(programTemplateHTML, retired) {
			t.Errorf("compact target switcher retained flat target navigation %q", retired)
		}
	}
}

func TestOrientationClientRendersRuntimePortfolioBeforeProgramMap(t *testing.T) {
	for _, required := range []string{
		"function buildRepositoryPresentationModel(data)",
		"integer(data.version, 'repository payload.version') !== 1",
		"The repository payload version is not supported",
		"function buildTargetDirectory(data)",
		"data.runtime == null ? null : buildRuntimePortfolio(data.runtime, directory, openable)",
		"object(raw, 'repository runtime')",
		"rawOutcome.selected_target_id",
		"rawOutcome.program_target_id",
		"optionalText(rawOutcome.href, 'repository target.href')",
		"link.href = implementation.target.href",
		"function runtimeEvidenceGroups(evidence)",
		"function renderRuntimeEvidence(role)",
		"sourceAction(label, location, { compact: true, locationLabel: '' })",
		"function renderRuntimePortfolio(host)",
		"Libraries and product APIs",
		"Primary runtime roles",
		"Examples",
		"Supporting tools",
		"Other supporting roles",
		"Uncertain roles",
		"Unclassified targets",
		"Target mapping unresolved",
		"hash === '#/repository'",
		"if (hash === '#/program')",
		"#/program/responsibility/",
		"← Repository overview",
	} {
		if !strings.Contains(reportAppJS, required) {
			t.Errorf("runtime portfolio client is missing %q", required)
		}
	}
	primarySection := strings.Index(reportAppJS, "renderRuntimeRoleSection(host, 'primary'")
	librarySection := strings.Index(reportAppJS, "renderRuntimeRoleSection(host, 'library'")
	exampleSection := strings.Index(reportAppJS, "renderRuntimeRoleSection(host, 'example'")
	toolSection := strings.Index(reportAppJS, "renderRuntimeRoleSection(host, 'tool'")
	supportingSection := strings.Index(reportAppJS, "renderRuntimeRoleSection(host, 'supporting'")
	if librarySection < 0 || primarySection < 0 || exampleSection < 0 || toolSection < 0 || supportingSection < 0 ||
		primarySection >= librarySection || librarySection >= exampleSection || exampleSection >= toolSection ||
		toolSection >= supportingSection {
		t.Error("repository overview does not render library, runtime, example, tool, and other supporting roles distinctly")
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
		".rm-runtime-section--library", ".rm-runtime-section--example", ".rm-runtime-section--tool",
		".rm-runtime-target", ".rm-runtime-evidence__files", ".rm-runtime-evidence-file__locations",
		".rm-runtime-unclassified__item",
	} {
		if !strings.Contains(reportAppCSS, selector) {
			t.Errorf("runtime portfolio CSS is missing %q", selector)
		}
	}
	for _, retired := range []string{
		"Implemented by", "Repository evidence", "Supporting and optional roles",
		"data.runtime_portfolio", "runtime_portfolio.version", "function buildTargetDirectory(data, currentTarget)",
	} {
		if strings.Contains(reportAppJS, retired) {
			t.Errorf("runtime portfolio retained internal presentation copy %q", retired)
		}
	}
	if !strings.Contains(programTemplateHTML, `href="#/repository"`) {
		t.Error("report brand does not return to the repository runtime overview")
	}
}

func TestOrientationClientRendersExactJSTSSurfacesAndCrossSurfacePaths(t *testing.T) {
	for _, required := range []string{
		"function buildTargetPresentationModel(data, repositoryModel)",
		"integer(data.version, 'target payload.version') !== 1",
		"The target payload version is not supported",
		"var features = object(data.features, 'target payload.features')",
		"function buildJSTSSurfaceCatalog(raw, openable)",
		"object(raw, 'target features.surfaces')",
		"function buildCrossSurfacePaths(raw, openable, surfaceCatalog)",
		"object(raw, 'target features.cross_surface_paths')",
		"features.cross_surface_paths, openable, surfaceCatalog",
		"['browser_application', 'node_server', 'command_line_application', 'shared_contracts', 'tool', 'unknown']",
		"if (value === 'command_line_application') return 'Command-line application'",
		"['product_surface', 'supporting_code', 'tool', 'unknown']",
		"http_method_path_match",
		"['exact_static', 'resolved_indirect', 'possible', 'unresolved_frontier']",
		"function routeForSurface(id)",
		"function routeForCrossSurfacePath(id)",
		"#/program/surface/",
		"#/program/path/",
		"function renderTargetSurfaceInventory(host)",
		"renderTargetSurfaceInventory(orientation)",
		"function crossSurfaceEmptyReason(catalog, coverage)",
		"function renderSurfaceDetail(host, surface)",
		"function renderCrossSurfacePathDetail(host, path)",
		"← Back to target overview",
		"back.href = '#/program'",
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
		"object(sourceSpec, 'repository payload.source')",
		"closedText(sourceSpec.kind, ['github', 'gitlab', 'served', 'none']",
		"sourceSpec.repository_url",
		"revision: model.revision",
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
		"multiple source authorities",
		"The source authority revision does not match the report",
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
		"connectionsFor(selected)",
		"relatedBlocksFor(selected, connections)",
		"runtime target unresolved",
		"A semantic responsibility map is not available for this target",
	} {
		if !strings.Contains(reportAppJS, required) {
			t.Errorf("orientation presentation model is missing %q", required)
		}
	}
	if !strings.Contains(systemCanvasGraphJS,
		"mapList(blocksBySymbol, string(use.callerID, 'integration use.callerID')") {
		t.Error("System canvas graph projection no longer binds integration callers through exact block membership")
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
		"state.model.groupsByBlock[selected.id]",
		"local_unassigned",
	} {
		if !strings.Contains(reportAppJS, required) {
			t.Errorf("group browser model is missing %q", required)
		}
	}
	if !strings.Contains(systemCanvasGraphJS, "groupsByBlock[entityID]") {
		t.Error("System canvas graph projection no longer preserves overlapping group memberships")
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
