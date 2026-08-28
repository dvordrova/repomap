package report

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestReportAppHashRoutesCanvasTargetsWithoutInertControls(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}

	tempDir := t.TempDir()
	appPath := filepath.Join(tempDir, "report_app.js")
	if err := os.WriteFile(appPath, []byte(reportAppJS), 0o600); err != nil {
		t.Fatal(err)
	}
	graphPath := filepath.Join(tempDir, "system_canvas_graph.js")
	if err := os.WriteFile(graphPath, []byte(systemCanvasGraphJS), 0o600); err != nil {
		t.Fatal(err)
	}
	runnerPath := filepath.Join(tempDir, "report_app_behavior.js")
	if err := os.WriteFile(runnerPath, []byte(reportAppBehaviorRunner), 0o600); err != nil {
		t.Fatal(err)
	}

	output, err := exec.Command(node, runnerPath, appPath, graphPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run report application behavior contract: %v\n%s", err, output)
	}
}

const reportAppBehaviorRunner = `
const fs = require('fs');
const vm = require('vm');

const failures = [];
function check(condition, message) {
  if (!condition) failures.push(message);
}

class TestElement {
  constructor(tagName) {
    this.tagName = String(tagName).toUpperCase();
    this.children = [];
    this.attributes = Object.create(null);
    this.listeners = Object.create(null);
    this.className = '';
    this.href = '';
    this.parentNode = null;
    this._textContent = '';
    this.scrollCalls = [];
    this.focusCalls = [];
    this._bounds = { left: 0, top: 0, right: 100, bottom: 40, width: 100, height: 40 };
  }

  appendChild(child) {
    child.parentNode = this;
    this.children.push(child);
    return child;
  }

  removeChild(child) {
    this.children = this.children.filter((candidate) => candidate !== child);
    child.parentNode = null;
    return child;
  }

  replaceChildren(...children) {
    this.children = [];
    children.forEach((child) => this.appendChild(child));
  }

  set textContent(value) {
    this._textContent = String(value);
    this.children = [];
  }

  get textContent() {
    return this._textContent + this.children.map((child) => child.textContent).join('');
  }

  setAttribute(name, value) {
    this.attributes[name] = String(value);
    if (name === 'class') this.className = String(value);
  }

  getAttribute(name) {
    return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null;
  }

  removeAttribute(name) {
    delete this.attributes[name];
    if (name === 'class') this.className = '';
  }

  addEventListener(type, listener) {
    if (!this.listeners[type]) this.listeners[type] = [];
    this.listeners[type].push(listener);
  }

  querySelector(selector) {
    return this.querySelectorAll(selector)[0] || null;
  }

  querySelectorAll(selector) {
    return descendants(this).filter((node) => node !== this && matchesSelector(node, selector));
  }

  dispatch(type, values) {
    const event = Object.assign({
      type, target: this, currentTarget: this, defaultPrevented: false,
      propagationStopped: false,
      preventDefault() { this.defaultPrevented = true; },
      stopPropagation() { this.propagationStopped = true; }
    }, values || {});
    (this.listeners[type] || []).forEach((listener) => listener.call(this, event));
    return event;
  }

  scrollIntoView(options) { this.scrollCalls.push(options); }
  focus(options) { this.focusCalls.push(options); }
  getBoundingClientRect() { return this._bounds; }

  click() {
    const event = {
      type: 'click', target: this, currentTarget: this, defaultPrevented: false,
      propagationStopped: false,
      preventDefault() { this.defaultPrevented = true; },
      stopPropagation() { this.propagationStopped = true; }
    };
    (this.listeners.click || []).forEach((listener) => listener.call(this, event));
    if (!event.defaultPrevented && this.tagName === 'A' && this.href.startsWith('#')) {
      context.window.location.hash = this.href;
    }
    return event;
  }
}

function matchesSelector(node, selector) {
  selector = String(selector);
  if (selector.startsWith('.')) {
    return String(node.className).split(/\s+/).includes(selector.slice(1));
  }
  const attribute = selector.match(/^\[([^=\]]+)(?:="([^"]*)")?\]$/);
  if (attribute) {
    const value = node.getAttribute(attribute[1]);
    return value !== null && (attribute[2] === undefined || value === attribute[2]);
  }
  return node.tagName === selector.toUpperCase();
}

function descendants(root) {
  const result = [];
  function visit(node) {
    result.push(node);
    node.children.forEach(visit);
  }
  visit(root);
  return result;
}

const document = {
  createElement(tagName) { return new TestElement(tagName); },
  createElementNS(namespace, tagName) { return new TestElement(tagName); },
  elementsByID: Object.create(null),
  getElementById(id) { return this.elementsByID[id] || null; },
  queryResult: null,
  querySelector() { return this.queryResult; }
};
const context = {
  console,
  document,
  window: {
    location: { hash: '#/repository' },
    listeners: Object.create(null),
    addEventListener(type, listener) {
      if (!this.listeners[type]) this.listeners[type] = [];
      this.listeners[type].push(listener);
    },
    dispatch(type) { (this.listeners[type] || []).forEach((listener) => listener()); },
    requestAnimationFrame(callback) { callback(); },
    setTimeout(callback) { callback(); }
  },
  URL,
  encodeURIComponent,
  decodeURIComponent
};
context.globalThis = context;

const graphSource = fs.readFileSync(process.argv[3], 'utf8');
vm.runInNewContext(graphSource, context, { filename: process.argv[3] });

const source = fs.readFileSync(process.argv[2], 'utf8');
const exposureBoundary = '\n  var api = Object.freeze({ boot: boot, fail: failApplication });';
if (source.split(exposureBoundary).length !== 2) {
  throw new Error('report_app.js does not have one recognizable application exposure boundary');
}
const exposure = [
  '  globalThis.__reportAppBehavior = {',
  '    setState: function (value) { state = value; },',
  '    selectedReportRoute: selectedReportRoute,',
  '    renderRepositoryFallback: renderRepositoryFallback,',
  '    renderActivityDetail: renderActivityDetail,',
  '    renderExecutionStory: renderExecutionStory,',
  '    renderStarts: renderStarts,',
  '    compactDisplayText: compactDisplayText,',
  '    renderIntegrationDetail: renderIntegrationDetail,',
  '    renderEvidence: renderEvidence,',
  '    connectionsFor: connectionsFor,',
  '    connectionOwnerObject: connectionOwnerObject,',
  '    groupConnectionsByOwner: groupConnectionsByOwner,',
  '    renderConnections: renderConnections,',
  '    renderSurvey: renderSurvey,',
  '    renderTargetSurfaceInventory: renderTargetSurfaceInventory,',
  '    renderSurfaceDetail: renderSurfaceDetail,',
  '    renderCrossSurfacePathDetail: renderCrossSurfacePathDetail,',
  '    renderRuntimePortfolio: renderRuntimePortfolio,',
  '    buildTargetDirectory: buildTargetDirectory,',
  '    buildRepositoryPresentationModel: buildRepositoryPresentationModel,',
  '    buildTargetPresentationModel: buildTargetPresentationModel,',
  '    targetOutcomeForLocation: targetOutcomeForLocation,',
  '    renderNotAnalyzedTargets: renderNotAnalyzedTargets,',
  '    renderHeader: renderHeader,',
  '    reportRouteContext: reportRouteContext,',
  '    updateHeaderContext: updateHeaderContext,',
  '    crossSurfaceEmptyReason: crossSurfaceEmptyReason,',
  '    renderFlowCanvas: renderFlowCanvas,',
  '    renderAreaSwitcher: renderAreaSwitcher,',
  '    buildSystemCanvasGraph: buildSystemCanvasGraph,',
  '    canvasNodeHref: canvasNodeHref,',
  '    navigateCanvasNode: navigateCanvasNode,',
  '    scheduleResponsibilityScroll: scheduleResponsibilityScroll,',
  '    transition: transition,',
  '    getState: function () { return state; },',
  '    resetBoot: function () { state = null; navigationToken = 0; bootStarted = false; },',
  '    unmountSystemCanvas: unmountSystemCanvas',
  '  };'
].join('\n');
const instrumented = source.replace(exposureBoundary, '\n' + exposure + exposureBoundary);
vm.runInNewContext(instrumented, context, { filename: process.argv[2] });

const api = context.__reportAppBehavior;
const activity = {
  id: 'entry/start here', name: 'app/main#Start here', kind: 'function', signature: 'start()',
  location: { path: 'app/main.py', line: 7, column: 1 }
};

const oversizedSignature = '(): {\n' + Array.from({ length: 400 }, (_, index) =>
  'field' + index + ': number;').join(' ') + '\n}';
const compactSignature = api.compactDisplayText(oversizedSignature, 120);
check(Array.from(compactSignature).length <= 120 && compactSignature.endsWith('…') &&
  !compactSignature.includes('\n'),
  'oversized signatures must become an explicit bounded one-line preview');
const block = {
  id: 'core/execution', name: 'Execution core', purpose: 'Runs the application.',
  files: [{ path: 'app/execution.py' }], symbols: [], depth: 0
};
const storageBlock = {
  id: 'core/storage', name: 'Storage core', purpose: 'Persists application state.',
  files: [], symbols: [], depth: 1
};
const integration = {
  id: 'dependency:http client', name: 'HTTP client', kind: 'external',
  packagePath: 'http-client', modulePath: 'http_client', uses: []
};
const sourceObject = { id: 'object:execution', name: 'app/execution#execute', kind: 'function' };
const targetObject = { id: 'object:storage', name: 'app/storage#store', kind: 'function' };
const platformObject = {
  id: 'object:platform-date', name: 'platform:javascript.Date', kind: 'external_symbol',
  external: { package_path: 'platform:javascript', name: 'Date' }
};
const packageObject = {
  id: 'object:react-effect', name: 'react.useEffect', kind: 'external_symbol',
  external: { package_path: 'react', receiver: 'react', name: 'useEffect' }
};
const directRoute = {
  id: 'route:direct',
  activityID: activity.id, callerID: activity.id, status: 'exact', distance: 0, steps: [], frontier: [],
  outcomes: [{
    dependencyID: integration.id,
    use: {
      authority: 'exact_external_symbol', label: 'send request', callee: 'http-client.send',
      mechanism: 'resolved external call',
      callsite: { path: 'app/execution.py', line: 30, column: 3 }
    }
  }]
};
const model = {
  repoName: 'github.com/fukict/fukict', revision: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
  target: {
    id: 'target:fukict', language: 'typescript', kind: 'library', name: 'fukict',
    selector: 'jsts:package.json'
  },
  targets: [
    {
      id: 'target:fukict', language: 'typescript', kind: 'library', displayName: 'fukict',
      href: '#/program'
    },
    {
      id: 'target:babel', language: 'typescript', kind: 'library', displayName: '@fukict/babel-preset',
      href: '../babel/report.html#/program'
    }
  ],
  defaultTargetID: 'target:babel',
  surfaceCatalog: null,
  runtimePortfolio: null,
  openable: {
    'app/main.py': true, 'app/execution.py': true,
    'scripts/check-changeset.ts': true, 'scripts/publish.ts': true, 'scripts/tag.ts': true
  },
  activities: [activity],
  blocks: [block, storageBlock],
  integrations: [integration],
  relations: [
    {
      id: 'relation:execution-storage', from_id: sourceObject.id, to_ids: [targetObject.id],
      kind: 'calls', resolution: 'exact'
    },
    {
      id: 'relation:execution-storage-possible', from_id: sourceObject.id, to_ids: [targetObject.id],
      kind: 'calls', resolution: 'alternatives'
    }
  ],
  objectsByID: {
    [sourceObject.id]: sourceObject, [targetObject.id]: targetObject,
    [platformObject.id]: platformObject, [packageObject.id]: packageObject
  },
  blocksBySymbol: {
    [activity.id]: [], [sourceObject.id]: [block.id], [targetObject.id]: [storageBlock.id]
  },
  activityPaths: { routes: [directRoute], objectsByID: { [activity.id]: activity } },
  blocksByID: { [block.id]: block, [storageBlock.id]: storageBlock },
  activityByID: { [activity.id]: activity },
  integrationsByID: { [integration.id]: integration },
  crossSurfacePaths: null,
  groups: [], groupByBlock: Object.create(null), modelGroupCount: 0, unassignedBlockCount: 0,
  groupsByBlock: { [block.id]: [], [storageBlock.id]: [] }
};
api.setState({
  model, focusGroupID: null, completeCanvas: false, canvasMount: null,
  pendingResponsibilityID: '', pendingAreaGroupFocusID: '',
  source: {
    mode: 'static', host: 'GitHub', repositoryURL: 'https://github.com/example/fixture',
    revision: model.revision, pathPrefix: ''
  }
});

function registerHeaderElement(id, tagName) {
  const value = new TestElement(tagName);
  document.elementsByID[id] = value;
  return value;
}
const targetSwitcher = registerHeaderElement('rm-target-switcher', 'details');
const targetSummary = new TestElement('summary');
targetSwitcher.appendChild(targetSummary);
const targetRepository = registerHeaderElement('rm-target-repository', 'span');
const targetCurrent = registerHeaderElement('rm-target-current', 'span');
const targetCount = registerHeaderElement('rm-target-count', 'span');
const targetPanelCount = registerHeaderElement('rm-target-panel-count', 'span');
const targetNavigation = registerHeaderElement('rm-target-navigation', 'nav');
const pageContext = registerHeaderElement('rm-page-context', 'div');
api.renderHeader();
api.updateHeaderContext({ kind: 'repository' });
const targetLinks = descendants(targetNavigation).filter((node) =>
  node.tagName === 'A' && String(node.className).includes('rm-target-switcher__target')
);
const currentTargetBadge = descendants(targetLinks[0]).find((node) =>
  String(node.className).split(/\s+/).includes('rm-target-switcher__badge--current')
);
check(targetRepository.textContent === 'fukict/fukict' && targetCurrent.textContent === 'All targets' &&
  targetCount.textContent === '2' && targetPanelCount.textContent === '2 targets',
  'the repository route must identify the repository and complete target count without claiming a current target');
check(targetLinks.length === 2 && targetLinks.every((link) => link.getAttribute('aria-current') === 'false') &&
  currentTargetBadge && currentTargetBadge.hidden === true && !targetLinks[0].textContent.includes('Default') &&
  targetLinks[1].textContent.includes('@fukict/babel-preset') && targetLinks[1].textContent.includes('Default'),
  'the repository route must distinguish the repository default without presenting any target as the current page');
targetSwitcher.open = true;
targetLinks[1].click();
check(targetSwitcher.open === false,
  'choosing a target must close the compact target switcher');
targetSwitcher.open = true;
targetSwitcher.dispatch('keydown', { key: 'Escape' });
check(targetSwitcher.open === false && targetSummary.focusCalls.length === 1,
  'Escape must close the target switcher and restore focus to its summary');
check(api.reportRouteContext({ kind: 'repository' }) === 'Repository overview' &&
  api.reportRouteContext({ kind: 'surface' }) === 'Surface' &&
  api.reportRouteContext({ kind: 'path' }) === 'Full-stack path' &&
  api.reportRouteContext({ kind: 'entrypoint' }) === 'Entrypoint' &&
  api.reportRouteContext({ kind: 'integration' }) === 'Integration' &&
  api.reportRouteContext({ kind: 'responsibility' }) === 'Responsibility' &&
  api.reportRouteContext({ kind: 'program' }) === 'Program map',
  'every report route must expose a concise page context');
api.updateHeaderContext({ kind: 'responsibility' });
check(pageContext.textContent === 'Responsibility · TypeScript · Library · aaaaaaaaaa',
  'the header context must identify the selected page, target kind, and captured revision');
check(targetCurrent.textContent === 'Target · fukict',
  'a target detail route must retain the selected target context');
check(targetLinks.every((link) => link.getAttribute('aria-current') === 'false'),
  'a target detail route must not mark the program-map target link as the current page');
check(currentTargetBadge.hidden === true,
  'a target detail route must not show the program-map Current badge');
api.updateHeaderContext({ kind: 'program' });
check(targetLinks[0].getAttribute('aria-current') === 'page' &&
  targetLinks[1].getAttribute('aria-current') === 'false' && currentTargetBadge.hidden === false,
  'only the exact selected target program route may mark its target link as the current page');
api.updateHeaderContext({ kind: 'repository' });
check(targetCurrent.textContent === 'All targets' &&
  targetLinks.every((link) => link.getAttribute('aria-current') === 'false') &&
  currentTargetBadge.hidden === true,
  'returning to the repository route must clear both current-page and current-target claims');

const failedSelectedTargetID = 'selected-target:failed-worker';
const targetOutcomePortfolio = api.buildTargetDirectory({
  logical_default_selected_target_id: failedSelectedTargetID,
  targets: [
    {
      selected_target_id: 'selected-target:fukict', language: 'javascript_typescript',
      kind: 'package', display_name: 'fukict', href: '?target=0#/program',
      state: 'analyzed', program_target_id: model.targets[0].id
    },
    {
      selected_target_id: 'selected-target:babel', language: 'javascript_typescript',
      kind: 'package', display_name: '@fukict/babel-preset', href: '?target=1#/program',
      state: 'analyzed', program_target_id: model.targets[1].id
    },
    {
      selected_target_id: failedSelectedTargetID, language: 'go', kind: 'executable',
      display_name: 'worker', state: 'not_analyzed',
      failure_stage: 'program_analysis', failure_reason: 'source_not_analyzable'
    }
  ]
}).outcomes;
check(targetOutcomePortfolio.outcomes.length === 3 && targetOutcomePortfolio.analyzed.length === 2 &&
  targetOutcomePortfolio.notAnalyzed.length === 1 &&
  targetOutcomePortfolio.defaultSelectedTargetID === failedSelectedTargetID,
  'the browser projection must retain every selected target while keeping analyzed and failed outcomes distinct');
model.targetOutcomePortfolio = targetOutcomePortfolio;
targetNavigation.replaceChildren();
api.renderHeader();
const exhaustiveTargetRows = descendants(targetNavigation).filter((node) =>
  String(node.className).split(/\s+/).includes('rm-target-switcher__target')
);
const exhaustiveTargetLinks = exhaustiveTargetRows.filter((node) => node.tagName === 'A');
const failedTargetRow = exhaustiveTargetRows.find((node) => node.getAttribute('aria-disabled') === 'true');
check(targetCount.textContent === '3' && targetPanelCount.textContent === '3 targets' &&
  exhaustiveTargetRows.length === 3 && exhaustiveTargetLinks.length === 2,
  'the target picker must count every selected target but link only analyzed targets');
check(failedTargetRow && failedTargetRow.tagName === 'DIV' && failedTargetRow.href === '' &&
  failedTargetRow.textContent.includes('worker') && failedTargetRow.textContent.includes('Not analyzed') &&
  failedTargetRow.textContent.includes('Default') &&
  failedTargetRow.textContent.includes('The selected source could not be analyzed.'),
  'a failed logical default must remain visible, disabled, red-state ready, and explained without a raw error');
const hashBeforeFailedTargetClick = context.window.location.hash;
if (failedTargetRow) failedTargetRow.click();
check(context.window.location.hash === hashBeforeFailedTargetClick,
  'a not-analyzed target row must not navigate to an invented target page');
const failedTargetOverviewHost = new TestElement('main');
api.renderRepositoryFallback(failedTargetOverviewHost);
check(failedTargetOverviewHost.textContent.includes('2 / 3 selected targets analyzed.') &&
  failedTargetOverviewHost.textContent.includes('Targets not analyzed') &&
  failedTargetOverviewHost.textContent.includes('Program analysis') &&
  failedTargetOverviewHost.textContent.includes('The selected source could not be analyzed.'),
  'the repository overview must account for analyzed targets and explain failed targets in a separate section');
model.targetOutcomePortfolio = null;
targetNavigation.replaceChildren();
api.renderHeader();

const targetOverviewHost = new TestElement('main');
api.renderSurvey(targetOverviewHost);
const targetOverview = descendants(targetOverviewHost).find((node) =>
  String(node.className).split(/\s+/).includes('rm-target-overview')
);
check(targetOverview && targetOverview.tagName === 'DETAILS' && targetOverview.open !== true,
  'the selected target overview must be a compact disclosure that starts closed');
check(targetOverviewHost.textContent.includes('fukict') &&
  targetOverviewHost.textContent.includes('2 responsibilities') &&
  targetOverviewHost.textContent.includes('1 entrypoint') &&
  targetOverviewHost.textContent.includes('1 integration') &&
  targetOverviewHost.textContent.includes('TypeScript · Library'),
  'the closed target overview summary must retain the short target identity and useful semantic counts');
check(!descendants(targetOverviewHost).some((node) =>
  String(node.className).split(/\s+/).includes('rm-survey')
), 'the selected target overview must not retain the oversized repository-survey hero');

const noCrossSurfaceCoverage = { http_uses_observed: 0, routes_observed: 0 };
check(api.crossSurfaceEmptyReason({ surfaces: [{ disposition: 'tool', kind: 'tool' }] },
  noCrossSurfaceCoverage).includes('neither a product browser surface nor a product Node server'),
  'a tool-only target must explain why it cannot establish a full-stack path');
check(api.crossSurfaceEmptyReason({ surfaces: [
  { disposition: 'product_surface', kind: 'browser_application' },
  { disposition: 'product_surface', kind: 'node_server' }
] }, noCrossSurfaceCoverage).includes('no retained client HTTP use'),
  'a product browser/server pair must report the next exact missing path condition');

const oversizedActivity = {
  id: 'entry:oversized-signature', name: 'src/index#LargeInferredType', kind: 'function',
  signature: oversizedSignature, location: { path: 'app/main.py', line: 30, column: 1 }
};
model.activityByID[oversizedActivity.id] = oversizedActivity;
const startsHost = new TestElement('main');
api.renderStarts(startsHost, { symbols: [{ id: oversizedActivity.id }] });
const renderedSignature = descendants(startsHost).find((node) => node.className === 'rm-start__signature');
check(!!renderedSignature && renderedSignature.textContent === api.compactDisplayText(oversizedSignature, 72),
  'responsibility entrypoint rows must render only the bounded signature preview');
check(startsHost.textContent.includes('LargeInferredType →') &&
  !startsHost.textContent.includes('src/index#LargeInferredType'),
  'responsibility entrypoint labels must omit redundant file and module prefixes');
delete model.activityByID[oversizedActivity.id];

check(api.selectedReportRoute().kind === 'repository',
  'an explicit #/repository route must render the repository fallback');

const fallbackHost = new TestElement('main');
api.renderRepositoryFallback(fallbackHost);
const fallback = descendants(fallbackHost).find((node) =>
  node.tagName === 'A' && node.href === '#/program'
);
check(!!fallback, 'the repository fallback must expose an Open program map link');
check(!!fallback && fallback.textContent.includes('Open program map'),
  'the repository fallback link must explain its destination');
if (fallback) fallback.click();
check(context.window.location.hash === '#/program',
  'the repository fallback link must navigate to #/program');
check(api.selectedReportRoute().kind === 'program',
  'the fallback target must resolve as the program route');

context.window.location.hash = '';
model.surfaceCatalog = { surfaces: [], surfacesByID: Object.create(null) };
model.crossSurfacePaths = { paths: [], pathsByID: Object.create(null) };
check(api.selectedReportRoute().kind === 'repository',
  'an empty location hash must remain on the repository overview without loading a target');
model.surfaceCatalog.surfaces.push({ id: 'surface:browser' });
check(api.selectedReportRoute().kind === 'repository',
  'target-local JavaScript/TypeScript surfaces must not turn the repository route into a selected-target page');
model.surfaceCatalog.surfaces = [];
model.crossSurfacePaths.paths.push({ id: 'path:request' });
check(api.selectedReportRoute().kind === 'repository',
  'target-local full-stack paths must remain on the selected target program page');
const browserSurface = {
  id: 'surface:browser/app', name: 'Browser app', kind: 'browser_application', role: 'client',
  disposition: 'product_surface', entryRefs: ['fact:browser-root'], evidenceRefs: [],
  location: { path: 'scripts/check-changeset.ts', line: 1, column: 1 }
};
const toolSurface = {
  id: 'surface:build/tool', name: 'Build tool', kind: 'tool', role: 'build',
  disposition: 'tool', entryRefs: [], evidenceRefs: [],
  location: { path: 'scripts/tag.ts', line: 1, column: 1 }
};
const crossSurfacePath = {
  id: 'path:http/request', name: 'Load data', outcome: 'Browser reaches the server route.',
  frontier: '', steps: [{
    ordinal: 1, kind: 'page_route', label: 'Browser root reaches the route',
    sourceRef: 'fact:browser-root', targetRefs: ['fact:route-handler'],
    authority: 'exact_static', resolution: 'exact',
    location: { path: 'scripts/check-changeset.ts', line: 94, column: 1 }
  }]
};
const browserRootFact = {
  ref: 'fact:browser-root', category: 'declaration', kind: 'function', label: 'src/index#root',
  location: { path: 'scripts/check-changeset.ts', line: 94, column: 1 }
};
const routeHandlerFact = {
  ref: 'fact:route-handler', category: 'declaration', kind: 'function', label: 'server/routes#handleRequest',
  location: { path: 'scripts/publish.ts', line: 20, column: 1 }
};
model.surfaceCatalog = {
  project: { name: 'fukict', moduleResolution: 'bundler' },
  surfaces: [browserSurface, toolSurface],
  surfacesByID: { [browserSurface.id]: browserSurface, [toolSurface.id]: toolSurface },
  factsByRef: {
    [browserRootFact.ref]: browserRootFact,
    [routeHandlerFact.ref]: routeHandlerFact
  }
};
model.crossSurfacePaths = {
  paths: [crossSurfacePath], pathsByID: { [crossSurfacePath.id]: crossSurfacePath },
  factsByRef: {
    [browserRootFact.ref]: browserRootFact,
    [routeHandlerFact.ref]: routeHandlerFact
  },
  coverage: { http_uses_observed: 1, routes_observed: 1 }
};
const surfaceInventoryHost = new TestElement('main');
api.renderTargetSurfaceInventory(surfaceInventoryHost);
const surfaceInventoryLinks = descendants(surfaceInventoryHost).filter((node) => node.tagName === 'A');
check([browserSurface, toolSurface].every((surface) => surfaceInventoryLinks.some((link) =>
  link.href === '#/program/surface/' + encodeURIComponent(surface.id)
)) && surfaceInventoryLinks.some((link) =>
  link.href === '#/program/path/' + encodeURIComponent(crossSurfacePath.id)
), 'the selected target program page must expose a reachable link for every exact surface and full-stack path');
check(surfaceInventoryHost.textContent.includes('Product surfaces') &&
  surfaceInventoryHost.textContent.includes('Tools and scripts') &&
  surfaceInventoryHost.textContent.includes('Full-stack paths'),
  'the selected target program page must keep the complete JavaScript/TypeScript surface inventory discoverable');
const surfaceDetailHost = new TestElement('main');
api.renderSurfaceDetail(surfaceDetailHost, browserSurface);
const surfaceBack = descendants(surfaceDetailHost).find((node) =>
  node.tagName === 'A' && node.textContent.includes('Back to target overview')
);
const pathDetailHost = new TestElement('main');
api.renderCrossSurfacePathDetail(pathDetailHost, crossSurfacePath);
const pathBack = descendants(pathDetailHost).find((node) =>
  node.tagName === 'A' && node.textContent.includes('Back to target overview')
);
check(surfaceBack && surfaceBack.href === '#/program' && pathBack && pathBack.href === '#/program',
  'surface and path details must return to the selected target program page');
check(surfaceDetailHost.textContent.includes('root') &&
  !surfaceDetailHost.textContent.includes('src/index#root') &&
  pathDetailHost.textContent.includes('handleRequest') &&
  !pathDetailHost.textContent.includes('server/routes#handleRequest'),
  'surface and path declaration facts must not repeat canonical module prefixes beside exact source locations');
model.surfaceCatalog = null;
model.crossSurfacePaths = null;
model.runtimePortfolio = { roles: [], unclassified: [] };
check(api.selectedReportRoute().kind === 'repository',
  'an empty runtime portfolio must still leave the empty hash on the repository overview');
model.runtimePortfolio.roles.push({ id: 'role:service' });
check(api.selectedReportRoute().kind === 'repository',
  'a runtime role must make the repository overview useful');
model.runtimePortfolio.roles = [];
model.runtimePortfolio.unclassified.push({ id: 'target:unknown' });
check(api.selectedReportRoute().kind === 'repository',
  'an unclassified target must make the repository overview useful');
model.runtimePortfolio = {
  roles: [
    {
      id: 'role:service', name: 'Example application', purpose: 'Demonstrates the packages.',
      prominence: 'primary', roleKind: 'service', requiredness: 'optional', confidence: 'high',
      implementations: [{ target: model.targets[0], mode: 'dev' }], evidence: []
    },
    {
      id: 'role:tool', name: 'Release tooling', purpose: 'Publishes the packages.',
      prominence: 'supporting', roleKind: 'supporting_tool', requiredness: 'experimental', confidence: 'low',
      implementations: [{ target: model.targets[0], mode: 'cli' }], evidence: []
    },
    {
      id: 'role:example', name: 'Complete example', purpose: 'Demonstrates the complete product.',
      prominence: 'supporting', roleKind: 'example', requiredness: 'optional', confidence: 'high',
      implementations: [{ target: model.targets[0], mode: 'dev' }], evidence: [
        { label: 'Selected activity start: src/App#App.render', location: { path: 'scripts/check-changeset.ts', line: 94, column: 1 } },
        { label: 'Responsibility representative: src/App#App', location: { path: 'scripts/check-changeset.ts', line: 94, column: 1 } },
        { label: 'Repository guidance example entry', location: { path: 'scripts/publish.ts', line: 20, column: 2 } }
      ]
    },
    {
      id: 'role:library', name: 'Router API', purpose: 'Provides reusable routing APIs.',
      prominence: 'supporting', roleKind: 'library', requiredness: 'required', confidence: 'high',
      implementations: [{ target: model.targets[1], mode: '' }], evidence: []
    },
    {
      id: 'role:unknown', name: 'Unclear package role', purpose: 'Has evidence but no established product kind.',
      prominence: 'primary', roleKind: 'unknown', requiredness: 'optional', confidence: 'high',
      implementations: [{ target: model.targets[1], mode: '' }], evidence: []
    }
  ],
  unclassified: [{
    target: { displayName: 'Unknown package', href: '../unknown/report.html#/program' },
    reason: 'No repository role maps this analyzed target.'
  }]
};
model.targetOutcomePortfolio = targetOutcomePortfolio;
const repositoryOverviewHost = new TestElement('main');
api.renderRuntimePortfolio(repositoryOverviewHost);
const repositoryOverviewText = repositoryOverviewHost.textContent;
const librarySectionOffset = repositoryOverviewText.indexOf('Libraries and product APIs');
const primarySectionOffset = repositoryOverviewText.indexOf('Primary runtime roles');
const exampleSectionOffset = repositoryOverviewText.indexOf('Examples');
const toolSectionOffset = repositoryOverviewText.indexOf('Supporting tools');
const uncertainSectionOffset = repositoryOverviewText.indexOf('Uncertain roles');
const unclassifiedSectionOffset = repositoryOverviewText.indexOf('Unclassified targets');
const notAnalyzedSectionOffset = repositoryOverviewText.indexOf('Targets not analyzed');
check(repositoryOverviewText.includes('Repository overview') &&
  repositoryOverviewText.includes('5 repository roles across 2 analyzed targets.') &&
  repositoryOverviewText.includes('2 / 3 selected targets analyzed.'),
  'the repository hero must describe the analyzed coverage of the complete selected-target portfolio');
check(primarySectionOffset >= 0 && primarySectionOffset < librarySectionOffset &&
  librarySectionOffset < exampleSectionOffset && exampleSectionOffset < toolSectionOffset &&
  toolSectionOffset < uncertainSectionOffset && uncertainSectionOffset < unclassifiedSectionOffset &&
  unclassifiedSectionOffset < notAnalyzedSectionOffset,
  'runtime roles, analyzed-but-unclassified targets, and targets not analyzed must remain distinct and ordered');
check(repositoryOverviewText.includes('Router API') && repositoryOverviewText.includes('Library'),
  'the repository overview must render a first-class library role and its kind');
const libraryRoleCard = descendants(repositoryOverviewHost).find((node) =>
  node.tagName === 'ARTICLE' && node.textContent.includes('Router API')
);
check(libraryRoleCard && libraryRoleCard.textContent.includes('Required role') &&
  libraryRoleCard.textContent.includes('Supporting'),
  'a library card must retain exceptional requiredness and its supporting product prominence');
check(libraryRoleCard && !libraryRoleCard.textContent.includes('No repository role maps'),
  'a semantic library role must not be rendered as an unclassified target');
check(repositoryOverviewText.includes('Complete example') && repositoryOverviewText.includes('Release tooling') &&
  !repositoryOverviewText.includes('Supporting and optional roles'),
  'examples and supporting tools must not be mixed into one generic optional-role section');
check(repositoryOverviewText.includes('Unclear package role') &&
  repositoryOverviewText.indexOf('Unclear package role') > uncertainSectionOffset,
  'a role with no established kind must remain visibly uncertain rather than being presented as runtime authority');
check(!repositoryOverviewText.includes('Implemented by') && !repositoryOverviewText.includes('Optional') &&
  !repositoryOverviewText.includes('High confidence') &&
  repositoryOverviewText.includes('Experimental') && repositoryOverviewText.includes('Low confidence'),
  'role cards must hide baseline model bookkeeping while retaining exceptional status warnings');
check(repositoryOverviewText.includes('Evidence · 3 facts · 2 files') &&
  repositoryOverviewText.split('scripts/check-changeset.ts').length === 2 &&
  repositoryOverviewText.split('scripts/publish.ts').length === 2 &&
  !repositoryOverviewText.includes('Selected activity start:'),
  'repository-role evidence must retain fact accounting but group exact locations once per file without raw semantic labels');
const repositoryEvidenceLinks = descendants(repositoryOverviewHost).filter((node) =>
  node.tagName === 'A' && node.href.includes('/blob/') &&
  (node.href.includes('#L94') || node.href.includes('#L20'))
);
check(repositoryEvidenceLinks.length === 2,
  'grouped repository-role evidence must preserve one exact action for every distinct source location');
model.runtimePortfolio = null;
model.targetOutcomePortfolio = null;

const focusedGraph = api.buildSystemCanvasGraph([block.id], false);
const directEdge = focusedGraph.edges.find((edge) =>
  edge.sourceID === 'entry:' + activity.id && edge.targetID === 'integration:' + integration.id
);
check(!!directEdge && directEdge.blockIDs.length === 0,
  'focused graph must retain a direct ungrouped entrypoint-to-integration frontier');
check(focusedGraph.accounting.directFrontierEdges === 1,
  'focused graph must account for its direct frontier separately from core grouping');
check(focusedGraph.nodesByLane.entry.some((node) => node.entityID === activity.id) &&
  focusedGraph.nodesByLane.integration.some((node) => node.entityID === integration.id),
  'direct frontier endpoints must remain visible in the focused graph');
check(focusedGraph.accounting.hiddenConnectedIntegrations === 0,
  'a visible direct-frontier integration must not be reported as belonging to another group');
check(!focusedGraph.edges.some((edge) =>
  edge.sourceID === 'core:' + block.id && edge.targetID === 'core:' + storageBlock.id
), 'focused graph must not retain a core edge whose target card is not visible');
check(focusedGraph.accounting.hiddenPlacedStarts >= 0 &&
  focusedGraph.accounting.hiddenConnectedIntegrations >= 0,
  'focused hidden connection counts must never become negative');
const unconnectedActivity = {
  id: 'entry:unconnected', name: 'Unconnected start', kind: 'function', signature: 'unused()',
  location: { path: 'app/main.py', line: 20, column: 1 }
};
model.activities.push(unconnectedActivity);
const completeGraphWithUnconnected = api.buildSystemCanvasGraph([block.id], true);
check(completeGraphWithUnconnected.nodesByLane.entry.length === 2 &&
  completeGraphWithUnconnected.accounting.connectedStartCount === 1 &&
  completeGraphWithUnconnected.accounting.hiddenPlacedStarts === 0,
  'complete graph must distinguish all shown entrypoints from the connected subset without negative hidden counts');
model.activities.pop();

let mountedCanvasContract = null;
let canvasUnmounted = false;
context.RepomapSystemCanvasRenderer = {
  mountSystemCanvas(host, graph, options, callbacks) {
    mountedCanvasContract = { host, graph, options, callbacks };
    return { unmount() { canvasUnmounted = true; } };
  }
};
const focusedCanvas = api.renderFlowCanvas(block);
check(focusedCanvas.element.textContent.includes('visible as a direct frontier outside core grouping'),
  'focused canvas must explain why an ungrouped direct route remains visible');
check(!focusedCanvas.element.textContent.includes('belong to other grouping selections'),
  'focused canvas must not assign direct-frontier integrations to another grouping');
const ungroupedAreaButtons = descendants(focusedCanvas.element).filter((node) =>
  node.getAttribute('data-area-selection') !== null
);
check(ungroupedAreaButtons.length === 1 &&
  ungroupedAreaButtons[0].getAttribute('data-area-selection') === 'all' &&
  ungroupedAreaButtons[0].querySelector('span').textContent === 'All' &&
  ungroupedAreaButtons[0].querySelector('small').textContent === '2',
  'an empty grouping must retain one All selection with complete responsibility accounting');
check(!descendants(focusedCanvas.element).some((node) =>
  String(node.className).includes('rm-canvas-mode')
), 'the canvas header must not retain a separate complete-map toggle');
check(source.includes('completeCanvas: false') &&
  !source.includes('Only exact selected facts are connected.'),
  'large reports must start focused and complete-mode copy must not hide possible/runtime authority');
const mountedController = focusedCanvas.mount();
check(mountedController && mountedCanvasContract &&
  mountedCanvasContract.options.laneSummaries.core.includes('2 directional core links'),
  'the shell must pass complete no-group core accounting into the isolated renderer');
const visibleCoreEdges = mountedCanvasContract.graph.edges.filter((edge) =>
  edge.sourceID === 'core:' + block.id && edge.targetID === 'core:' + storageBlock.id
);
check(visibleCoreEdges.length === 2 && visibleCoreEdges.some((edge) => edge.authority === 'exact') &&
  visibleCoreEdges.some((edge) => edge.authority === 'possible'),
  'exact and possible authority for one endpoint pair must remain separate');
check(typeof mountedCanvasContract.callbacks.hrefForNode === 'function' &&
  typeof mountedCanvasContract.callbacks.navigateNode === 'function',
  'the report shell must supply explicit node-link and navigation callbacks to the isolated renderer');
api.getState().canvasMount = mountedController;
api.unmountSystemCanvas();
check(canvasUnmounted && api.getState().canvasMount === null,
  'the report shell must explicitly unmount the isolated canvas controller');
const storageArea = {
  id: 'group:storage', name: 'Storage area', purpose: 'Owns persistence behavior.',
  authority: 'model', blockIDs: [storageBlock.id]
};
model.groups = [storageArea];
model.groupByBlock = { [storageBlock.id]: storageArea };
model.groupsByBlock = { [block.id]: [], [storageBlock.id]: [storageArea] };
model.modelGroupCount = 1;
const areaSwitcher = api.renderAreaSwitcher(block, null);
const areaSelections = descendants(areaSwitcher).filter((node) =>
  node.getAttribute('data-area-selection') !== null
);
check(areaSelections.length === 2 &&
  areaSelections[0].getAttribute('data-area-selection') === 'all' &&
  areaSelections[1].getAttribute('data-area-selection') === storageArea.id,
  'All must be the first architecture selection before every exact grouping');
const areaButton = descendants(areaSwitcher).find((node) => node.getAttribute('data-area-group') === storageArea.id);
check(!!areaButton, 'the architecture switcher must expose every exact grouping selection as a button');
if (areaButton) areaButton.click();
check(context.window.location.hash === '#/program/responsibility/core%2Fstorage' &&
  api.getState().focusGroupID === storageArea.id && api.getState().pendingResponsibilityID === '',
  'an architecture-area selection must update the focused map without scheduling a scroll to responsibility detail');
const activeAreaSwitcher = api.renderAreaSwitcher(storageBlock, storageArea);
const activeAreaButton = descendants(activeAreaSwitcher).find((node) =>
  node.getAttribute('data-area-group') === storageArea.id
);
check(activeAreaButton && activeAreaButton.getAttribute('aria-current') === 'true' &&
  activeAreaButton.focusCalls.length === 1 &&
  activeAreaButton.focusCalls[0] && activeAreaButton.focusCalls[0].preventScroll === true,
  'the replacement active area button must regain focus without moving the viewport away from the canvas');
api.getState().completeCanvas = true;
api.getState().pendingAreaGroupFocusID = 'all';
const completeAreaSwitcher = api.renderAreaSwitcher(storageBlock, storageArea);
const activeAllButton = descendants(completeAreaSwitcher).find((node) =>
  node.getAttribute('data-area-selection') === 'all'
);
const inactiveAreaButton = descendants(completeAreaSwitcher).find((node) =>
  node.getAttribute('data-area-group') === storageArea.id
);
check(activeAllButton && activeAllButton.getAttribute('aria-current') === 'true' &&
  activeAllButton.focusCalls.length === 1 &&
  activeAllButton.focusCalls[0] && activeAllButton.focusCalls[0].preventScroll === true &&
  inactiveAreaButton && inactiveAreaButton.getAttribute('aria-current') === null,
  'All must become the sole current selection and regain focus in complete-map mode');
model.groups = [];
model.groupByBlock = Object.create(null);
model.groupsByBlock = { [block.id]: [], [storageBlock.id]: [] };
model.modelGroupCount = 0;
api.getState().focusGroupID = null;
api.getState().completeCanvas = false;
context.window.location.hash = '#/program';

const completeGraph = api.buildSystemCanvasGraph([block.id], true);
const entryNode = completeGraph.nodesByID['entry:' + activity.id];
const coreNode = completeGraph.nodesByID['core:' + block.id];
const integrationNode = completeGraph.nodesByID['integration:' + integration.id];
check(entryNode && coreNode && integrationNode,
  'the extracted graph must expose stable exact identities for every canvas lane');
check(api.canvasNodeHref(entryNode) === '#/program/entrypoint/entry%2Fstart%20here' &&
  api.canvasNodeHref(coreNode) === '#/program/responsibility/core%2Fexecution' &&
  api.canvasNodeHref(integrationNode) === '#/program/integration/dependency%3Ahttp%20client',
  'the report shell must retain exact program routes for isolated canvas nodes');
check(entryNode.name === 'Start here' && entryNode.data.location.path === 'app/main.py' &&
  !entryNode.name.includes('app/main#'),
  'the graph projection must keep compact entrypoint display text beside exact location data');
check(!source.includes('function canvasTopology(') &&
  !source.includes('function drawCanvasEdges(') &&
  !source.includes('function highlightCanvasNode('),
  'the report shell must not retain the legacy graph, SVG, or interaction implementation');
const evidenceBlock = {
  id: 'core:release-scripts', name: 'Release scripts', purpose: 'Validates and publishes releases.',
  files: [
    { path: 'scripts/check-changeset.ts' },
    { path: 'scripts/tag.ts' }
  ],
  symbols: [
    {
      id: 'object:check-precommit', name: 'scripts/check-changeset#checkPrecommit', kind: 'function',
      location: { path: 'scripts/check-changeset.ts', line: 94, column: 1 }, unresolvedOutgoing: 0
    },
    {
      id: 'object:publish', name: 'scripts/publish#publish', kind: 'function',
      location: { path: 'scripts/publish.ts', line: 20, column: 1 }, unresolvedOutgoing: 0
    }
  ],
  depth: 0
};
const evidenceHost = api.renderEvidence(evidenceBlock);
const evidenceDetails = descendants(evidenceHost).filter((node) => node.tagName === 'DETAILS');
const evidenceLinks = descendants(evidenceHost).filter((node) =>
  node.tagName === 'A' && node.href.includes('/blob/')
);
const evidenceNames = descendants(evidenceHost).filter((node) =>
  String(node.className).includes('rm-source-action__name')
).map((node) => node.textContent);
const evidenceFileGroups = descendants(evidenceHost).filter((node) =>
  String(node.className).split(/\s+/).includes('rm-evidence-file')
);
check(evidenceDetails.length === 1 && evidenceDetails[0].open === true,
  'Verify in code must use one disclosure that is open by default');
check(!evidenceHost.textContent.includes('Files ·') && !evidenceHost.textContent.includes('Open file'),
  'grouped evidence must not repeat a separate Files section or Open file labels');
check(evidenceNames.includes('checkPrecommit') && evidenceNames.includes('publish') &&
  !evidenceHost.textContent.includes('scripts/check-changeset#') &&
  !evidenceHost.textContent.includes('scripts/publish#'),
  'declaration labels must omit redundant file and module prefixes');
check((evidenceHost.textContent.match(/scripts\/check-changeset\.ts/g) || []).length === 1 &&
  (evidenceHost.textContent.match(/scripts\/publish\.ts/g) || []).length === 1 &&
  (evidenceHost.textContent.match(/scripts\/tag\.ts/g) || []).length === 1,
  'each exact evidence file must render once as its group heading');
check(evidenceFileGroups.length === 3 &&
  evidenceFileGroups.some((group) => group.textContent.includes('scripts/check-changeset.ts') &&
    group.textContent.includes('checkPrecommit') && group.textContent.includes('L94')) &&
  evidenceFileGroups.some((group) => group.textContent.includes('scripts/publish.ts') &&
    group.textContent.includes('publish') && group.textContent.includes('L20')) &&
  evidenceFileGroups.some((group) => group.textContent === 'scripts/tag.ts'),
  'each declaration must render inside its exact file group');
check(evidenceLinks.length === 5 &&
  evidenceLinks.some((link) => link.href.endsWith('/scripts/check-changeset.ts')) &&
  evidenceLinks.some((link) => link.href.endsWith('/scripts/check-changeset.ts#L94')) &&
  evidenceLinks.some((link) => link.href.endsWith('/scripts/publish.ts')) &&
  evidenceLinks.some((link) => link.href.endsWith('/scripts/publish.ts#L20')) &&
  evidenceLinks.some((link) => link.href.endsWith('/scripts/tag.ts')),
  'file headings, declarations, symbol-only files, and file-only evidence must retain exact source links');

context.window.location.hash = api.canvasNodeHref(entryNode);
check(context.window.location.hash === '#/program/entrypoint/entry%2Fstart%20here',
  'the entrypoint canvas link must preserve its exact encoded identity');
let route = api.selectedReportRoute();
check(route.kind === 'entrypoint' && route.activity === activity,
  'the entrypoint hash must resolve to its exact selected activity');
const activityHost = new TestElement('main');
model.blocksBySymbol[activity.id] = [block.id];
api.renderActivityDetail(activityHost, route.activity);
check(activityHost.textContent.includes('Start here') &&
  activityHost.textContent.includes('Follow execution'),
  'the entrypoint route must render a useful semantic detail page');
check(descendants(activityHost).some((node) => node.className === 'rm-entrypoint-header') &&
  !descendants(activityHost).some((node) => node.className === 'rm-detail-hero') &&
  activityHost.textContent.includes('Core responsibility touchpoints') &&
  activityHost.textContent.includes(block.purpose) &&
  activityHost.textContent.includes('Starts here') &&
  activityHost.textContent.includes('Exact selected path'),
  'entrypoint detail must use a compact header and expose existing responsibility and route facts');
const exactStory = descendants(activityHost).find((node) =>
  String(node.className).split(/\s+/).includes('rm-execution-story')
);
const exactStorySourceActions = exactStory ? descendants(exactStory).filter((node) =>
  String(node.className).split(/\s+/).includes('rm-source-action')
) : [];
check(exactStory && exactStory.textContent.includes('Deterministic program facts') &&
  exactStory.textContent.includes('Exact route') && exactStory.textContent.includes('External outcome') &&
  exactStory.textContent.includes('http-client.send'),
  'Follow execution must reframe the already joined exact route and endpoint without model prose');
check(exactStorySourceActions.length === 2 &&
  exactStorySourceActions[0].textContent.includes('Start here') &&
  exactStorySourceActions[1].textContent.includes('Open exact endpoint'),
  'one execution route must expose only its selected activity and one exact endpoint source action');

const possibleCaller = {
  id: 'object:possible-caller', name: 'app/execution#sendPossibly', kind: 'function'
};
model.activityPaths.objectsByID[possibleCaller.id] = possibleCaller;
const possibleRoute = {
  id: 'route:possible', activityID: activity.id, callerID: possibleCaller.id,
  status: 'possible', distance: 1, frontier: [],
  steps: [{
    fromID: activity.id, toID: possibleCaller.id, kind: 'passes_callback', authority: 'possible',
    location: { path: 'app/execution.py', line: 24, column: 3 }
  }],
  outcomes: [directRoute.outcomes[0], {
    dependencyID: integration.id,
    use: {
      authority: 'exact_external_symbol', label: 'send fallback', callee: 'http-client.sendFallback',
      mechanism: 'resolved external call',
      callsite: { path: 'app/execution.py', line: 31, column: 3 }
    }
  }]
};
const possibleStory = api.renderExecutionStory(activity, [possibleRoute]);
const possibleSourceActions = descendants(possibleStory).filter((node) =>
  String(node.className).split(/\s+/).includes('rm-source-action')
);
check(possibleStory.textContent.includes('Possible route') &&
  descendants(possibleStory).some((node) =>
    String(node.className).split(/\s+/).includes('rm-execution-story-step--possible')) &&
  (possibleStory.textContent.match(/External outcome/g) || []).length === 2,
  'Follow execution must keep a possible edge visibly distinct and retain every exact joined outcome');
check(possibleSourceActions.length === 2,
  'multiple exact outcomes on one route must not expand beyond the two-source-action cap');

const unresolvedRoute = {
  id: 'route:unresolved-endpoint', activityID: activity.id, callerID: activity.id,
  status: 'exact', distance: 0, steps: [], frontier: [], outcomes: [{
    dependencyID: integration.id,
    use: {
      authority: 'syntactic_unresolved', label: 'dynamic send', callee: 'client.send',
      mechanism: 'syntactic callsite',
      callsite: { path: 'app/execution.py', line: 32, column: 3 }
    }
  }]
};
const unresolvedStory = api.renderExecutionStory(activity, [unresolvedRoute]);
const unresolvedSourceActions = descendants(unresolvedStory).filter((node) =>
  String(node.className).split(/\s+/).includes('rm-source-action')
);
check(unresolvedStory.textContent.includes('not represented in available facts') &&
  unresolvedStory.textContent.includes('Inspect integration') &&
  !unresolvedStory.textContent.includes('client.send') && unresolvedSourceActions.length === 1,
  'an unresolved callsite must remain an explicit endpoint gap without a callee or endpoint source action');

const routesBeforeEmptyStory = model.activityPaths.routes;
model.activityPaths.routes = [];
const noStoryHost = new TestElement('main');
api.renderActivityDetail(noStoryHost, activity);
check(api.renderExecutionStory(activity, []) === null &&
  !descendants(noStoryHost).some((node) =>
    String(node.className).split(/\s+/).includes('rm-execution-story')) &&
  noStoryHost.textContent.includes('Paths to selected integrations'),
  'an entrypoint without a joined path must omit Follow execution and preserve the existing empty-path page');
model.activityPaths.routes = routesBeforeEmptyStory;
delete model.activityPaths.objectsByID[possibleCaller.id];

const coreClick = {
  defaultPrevented: false,
  preventDefault() { this.defaultPrevented = true; }
};
api.navigateCanvasNode(coreNode, coreClick, null);
check(context.window.location.hash === '#/program/responsibility/core%2Fexecution',
  'the core canvas link must preserve its exact encoded identity');
check(coreClick && coreClick.defaultPrevented && api.getState().pendingResponsibilityID === block.id,
  'a core canvas click must request navigation that lands on the responsibility detail');
route = api.selectedReportRoute();
check(route.kind === 'responsibility' && route.block === block,
  'the core hash must resolve to its exact selected responsibility');
const responsibilityDetail = new TestElement('article');
responsibilityDetail.setAttribute('data-responsibility-detail', block.id);
document.queryResult = responsibilityDetail;
api.scheduleResponsibilityScroll(block.id);
check(responsibilityDetail.scrollCalls.length === 1 && responsibilityDetail.focusCalls.length === 1,
  'responsibility navigation must scroll and move focus to the exact detail article');
document.queryResult = null;

context.window.location.hash = api.canvasNodeHref(integrationNode);
check(context.window.location.hash === '#/program/integration/dependency%3Ahttp%20client',
  'the integration canvas link must preserve its exact encoded identity');
route = api.selectedReportRoute();
check(route.kind === 'integration' && route.integration === integration,
  'the integration hash must resolve to its exact selected dependency');
integration.uses = [{
  callerID: sourceObject.id, callerName: 'app/execution#execute', callee: 'http-client.send',
  label: 'Send request', mechanism: 'resolved external call', authority: 'exact_external_symbol',
  callsite: { path: 'app/execution.py', line: 30, column: 3 }, route: null
}];
const integrationHost = new TestElement('main');
api.renderIntegrationDetail(integrationHost, route.integration);
check(integrationHost.textContent.includes('HTTP client') &&
  integrationHost.textContent.includes('Selected operations'),
  'the integration route must render a useful semantic detail page');
check(integrationHost.textContent.includes('execute → http-client.send') &&
  !integrationHost.textContent.includes('app/execution#execute'),
  'integration operations must not repeat a canonical caller path beside the exact selected callsite');
['frontier', 'unconnected'].forEach((status) => {
  integration.uses[0].route = {
    status, activityID: '', frontier: status === 'frontier' ? ['program_objects_omitted'] : []
  };
  const gapHost = new TestElement('main');
  api.renderIntegrationDetail(gapHost, integration);
  check(gapHost.textContent.includes('Activity path: ' + status) &&
    gapHost.textContent.includes('not represented in available facts') &&
    !gapHost.textContent.includes('from Start here'),
    status + ' integration routes must remain explicit missing activity authority, not synthesized entrypoint stories');
});
integration.uses = [];

const routerOwner = {
  id: 'object:router-owner', name: 'packages/router/src/Router#Router', kind: 'type',
  location: { path: 'app/execution.py', line: 4, column: 1 }
};
const routerConstructor = {
  id: 'object:router-constructor', name: 'packages/router/src/Router#Router.constructor', kind: 'method',
  owner_id: routerOwner.id, container_id: routerOwner.id,
  location: { path: 'app/execution.py', line: 9, column: 1 }
};
const sameNameRouterOwner = {
  id: 'object:other-router-owner', name: 'packages/other/src/Router#Router', kind: 'type',
  location: { path: 'app/execution.py', line: 5, column: 1 }
};
const sameNameRouterConstructor = {
  id: 'object:other-router-constructor', name: 'packages/other/src/Router#Router.constructor', kind: 'method',
  owner_id: sameNameRouterOwner.id, container_id: sameNameRouterOwner.id,
  location: { path: 'app/execution.py', line: 17, column: 1 }
};
const helperModule = {
  id: 'object:helper-container', name: 'packages/router/src/history#History', kind: 'type',
  location: { path: 'app/execution.py', line: 6, column: 1 }
};
const createHistory = {
  id: 'object:create-history', name: 'packages/router/src/history#createHistory', kind: 'function',
  container_id: helperModule.id,
  location: { path: 'app/execution.py', line: 18, column: 1 }
};
[routerOwner, routerConstructor, sameNameRouterOwner, sameNameRouterConstructor,
  helperModule, createHistory].forEach((candidate) => {
  model.objectsByID[candidate.id] = candidate;
});
const connectionBlock = {
  id: block.id, name: block.name, purpose: block.purpose,
  files: block.files, symbols: [routerConstructor, sameNameRouterConstructor, createHistory], depth: block.depth
};
function accountedRelation(relation, counts) {
  if (!relation.witnesses) {
    relation.witnesses = [{ kind: 'fixture', location: relation.location }];
  } else {
    relation.witnesses.forEach((witness) => { if (!witness.kind) witness.kind = 'fixture'; });
  }
  counts = counts || {};
  relation.witnesses_indexed = counts.indexed == null ? relation.witnesses.length : counts.indexed;
  relation.witnesses_observed = counts.observed == null ? relation.witnesses_indexed : counts.observed;
  relation.witnesses_omitted = relation.witnesses_observed - relation.witnesses_indexed;
  relation.witnesses_projection_omitted = relation.witnesses_indexed - relation.witnesses.length;
  return relation;
}
model.relations = [
  accountedRelation({
    id: 'relation:exact-one', from_id: routerConstructor.id, to_ids: [targetObject.id],
    kind: 'calls', resolution: 'exact',
    location: { path: 'app/execution.py', line: 10, column: 3 },
    witnesses: [
      { kind: 'fixture', location: { path: 'app/execution.py', line: 10, column: 3 } },
      { kind: 'fixture', location: { path: 'app/execution.py', line: 16, column: 3 } }
    ]
  }),
  accountedRelation({
    id: 'relation:exact-two', from_id: routerConstructor.id, to_ids: [targetObject.id],
    kind: 'calls', resolution: 'exact',
    location: { path: 'app/execution.py', line: 11, column: 3 }
  }),
  accountedRelation({
    id: 'relation:unresolved-send', from_id: routerConstructor.id, to_ids: [],
    kind: 'calls', resolution: 'unresolved',
    location: { path: 'app/execution.py', line: 12, column: 3 },
    witnesses: [{ source_expression: 'client.send', location: { path: 'app/execution.py', line: 12, column: 3 } }]
  }),
  accountedRelation({
    id: 'relation:unresolved-flush', from_id: routerConstructor.id, to_ids: [],
    kind: 'calls', resolution: 'unresolved',
    location: { path: 'app/execution.py', line: 13, column: 3 },
    witnesses: [{ source_expression: 'client.flush', location: { path: 'app/execution.py', line: 13, column: 3 } }]
  }),
  accountedRelation({
    id: 'relation:platform-date', from_id: routerConstructor.id, to_ids: [platformObject.id],
    kind: 'invokes_external', resolution: 'exact', invocation: 'construct',
    location: { path: 'app/execution.py', line: 14, column: 3 }
  }, { observed: 3, indexed: 1 }),
  accountedRelation({
    id: 'relation:package-effect', from_id: routerConstructor.id, to_ids: [packageObject.id],
    kind: 'invokes_external', resolution: 'exact', invocation: 'call',
    location: { path: 'app/execution.py', line: 15, column: 3 }
  }, { observed: 2, indexed: 2 }),
  accountedRelation({
    id: 'relation:same-name-owner', from_id: sameNameRouterConstructor.id, to_ids: [targetObject.id],
    kind: 'calls', resolution: 'exact', invocation: 'declared_interface_dispatch:synchronous',
    location: { path: 'app/execution.py', line: 17, column: 3 }
  }),
  accountedRelation({
    id: 'relation:container-owner', from_id: createHistory.id, to_ids: [targetObject.id],
    kind: 'calls', resolution: 'exact',
    location: { path: 'app/execution.py', line: 18, column: 3 }
  })
];
const connections = api.connectionsFor(connectionBlock);
const exactConnections = connections.filter((connection) => connection.resolution === 'exact');
const localExactConnections = exactConnections.filter((connection) =>
  connection.targetIDs.includes(targetObject.id)
);
const unresolvedConnections = connections.filter((connection) => connection.resolution === 'unresolved');
check(exactConnections.length === 5 && localExactConnections.length === 3 &&
  localExactConnections.some((connection) => connection.relationIDs.length === 2),
  'relations with the same resolved target must remain one compact connection group');
check(unresolvedConnections.length === 2 && unresolvedConnections.every((connection) =>
  connection.relationIDs.length === 1
), 'different unresolved relation records must remain separate connection rows');
check(unresolvedConnections.some((connection) => connection.to === 'unresolved call: client.send') &&
  unresolvedConnections.some((connection) => connection.to === 'unresolved call: client.flush'),
  'every unresolved connection row must retain its exact witnessed expression');
const ownerGroups = api.groupConnectionsByOwner(connections);
check(ownerGroups.length === 3 &&
  ownerGroups.some((group) => group.owner.id === routerOwner.id) &&
  ownerGroups.some((group) => group.owner.id === sameNameRouterOwner.id) &&
  ownerGroups.some((group) => group.owner.id === helperModule.id),
  'connection hierarchy must group by exact owner and container identities without merging equal display names');
const routerOwnerGroup = ownerGroups.find((group) => group.owner.id === routerOwner.id);
check(routerOwnerGroup && routerOwnerGroup.local.length === 1 &&
  routerOwnerGroup.platform.length === 1 && routerOwnerGroup.external.length === 1 &&
  routerOwnerGroup.unresolved.length === 2,
  'the exact owner hierarchy must keep local, platform, external, and unresolved relation authorities separate');
const containerConnection = connections.find((connection) => connection.fromID === createHistory.id);
check(containerConnection && api.connectionOwnerObject(containerConnection).id === helperModule.id,
  'a declaration without owner_id must use its exact non-module/package container_id as hierarchy authority');
const adapterInvocationConnection = connections.find((connection) =>
  connection.fromID === sameNameRouterConstructor.id
);
check(adapterInvocationConnection &&
  adapterInvocationConnection.invocation === 'declared_interface_dispatch:synchronous',
  'connection projection must preserve adapter-owned invocation text without treating it as a cross-language enum');

const connectionsHost = new TestElement('main');
api.renderConnections(connectionsHost, connectionBlock, connections);
const connectionLinks = descendants(connectionsHost).filter((node) =>
  node.tagName === 'A' && node.href.includes('/blob/')
);
const expectedConnectionLocations = [10, 11, 12, 13, 14, 15, 16, 17, 18].map((line) =>
  'app/execution.py:' + String(line)
);
check(expectedConnectionLocations.every((location) =>
    connectionLinks.some((link) => link.href.endsWith('#L' + location.split(':')[1]))
  ),
  'every grouped relation and witness location must remain available through an exact source link');
check(connectionsHost.textContent.includes('7 relation groups · 8 relation records') &&
  connectionsHost.textContent.includes('unresolved call: client.send') &&
  connectionsHost.textContent.includes('unresolved call: client.flush') &&
  !connectionsHost.textContent.includes('exact records'),
  'the disclosure summary must continue to describe condensed relation records honestly');
const platformConnection = exactConnections.find((connection) => connection.targetIDs.includes(platformObject.id));
const packageConnection = exactConnections.find((connection) => connection.targetIDs.includes(packageObject.id));
check(platformConnection && platformConnection.platformTarget && platformConnection.invocation === 'construct' &&
  platformConnection.witnessesObserved === 3 && platformConnection.witnessesOmitted === 2 &&
  packageConnection && !packageConnection.platformTarget && packageConnection.invocation === 'call' &&
  packageConnection.witnessesObserved === 2 && packageConnection.witnessesProjectionOmitted === 1,
  'connection condensation must preserve platform, invocation, and witness accounting even when its footer is hidden');
check(connectionsHost.textContent.includes('constructor') &&
  connectionsHost.textContent.includes('store()') &&
  connectionsHost.textContent.includes('new JavaScript Date()') &&
  connectionsHost.textContent.includes('react.useEffect()') &&
  !connectionsHost.textContent.includes('— calls →') &&
  !connectionsHost.textContent.includes('packages/router/src/Router#'),
  'owned calls must render as a compact owner, member, and callable-target hierarchy');
const connectionPaths = descendants(connectionsHost).filter((node) =>
  String(node.className).includes('rm-connection__path')
);
check(connectionPaths.some((path) => path.textContent === 'store()') &&
  connectionPaths.some((path) => path.textContent === 'new JavaScript Date()') &&
  connectionPaths.some((path) => path.textContent === 'react.useEffect()'),
  'compact connection targets must remain directly source-linked');
const renderedOwnerGroups = descendants(connectionsHost).filter((node) =>
  String(node.className).split(/\s+/).includes('rm-connection-owner')
);
const renderedOwnerTitles = descendants(connectionsHost).filter((node) =>
  String(node.className).split(/\s+/).includes('rm-connection-owner__title')
);
const runtimeBuckets = descendants(connectionsHost).filter((node) =>
  node.tagName === 'DETAILS' && String(node.className).split(/\s+/).includes('rm-connection-runtime')
);
const renderedMemberGroups = descendants(connectionsHost).filter((node) =>
  String(node.className).split(/\s+/).includes('rm-connection-member-group')
);
const renderedMemberHeadings = descendants(connectionsHost).filter((node) =>
  String(node.className).split(/\s+/).includes('rm-connection-member-group__heading')
);
check(renderedOwnerGroups.length === 3 && renderedOwnerTitles.filter((title) =>
  title.textContent.includes('Router')
).length === 2,
  'the rendered hierarchy must retain two distinct exact Router owners even when their display names match');
check(renderedMemberGroups.length === 3 && renderedMemberHeadings.filter((heading) =>
  heading.textContent === 'constructor'
).length === 2 && renderedMemberHeadings.some((heading) => heading.textContent === 'createHistory'),
  'each exact source member must be shown once beneath its exact owner');
check(runtimeBuckets.length === 3 && runtimeBuckets.every((details) => details.open !== true) &&
  runtimeBuckets.some((details) => String(details.className).includes('rm-connection-runtime--platform')) &&
  runtimeBuckets.some((details) => String(details.className).includes('rm-connection-runtime--external')) &&
  runtimeBuckets.some((details) => String(details.className).includes('rm-connection-runtime--unresolved')),
  'platform, external, and unresolved rows must remain complete behind separate closed native disclosures');
const connectionSites = descendants(connectionsHost).filter((node) =>
  node.tagName === 'DETAILS' && String(node.className).split(/\s+/).includes('rm-connection-sites')
);
const resolutionBadges = descendants(connectionsHost).filter((node) =>
  String(node.className).split(/\s+/).includes('rm-resolution')
);
check(connectionSites.length === 1 && connectionSites[0].open !== true &&
  connectionSites[0].textContent.includes('3 callsites') &&
  expectedConnectionLocations.slice(0, 2).every((location) => connectionSites[0].textContent.includes(location)) &&
  resolutionBadges.every((badge) => !String(badge.className).includes('rm-resolution--exact')) &&
  resolutionBadges.some((badge) => String(badge.className).includes('rm-resolution--unresolved')),
  'exactness and repeated paths must stay implicit while multiple callsites and unresolved authority remain available');
check(!connectionsHost.textContent.includes('Open source location') &&
  !descendants(connectionsHost).some((node) => String(node.className).includes('rm-connection__meta')) &&
  !connectionsHost.textContent.includes('relation records · 3 source locations') &&
  connectionsHost.textContent.includes('3 relation witness details are not shown'),
  'connection rows must omit redundant location labels and counters while retaining omission-only diagnostics');

const renderedRoots = [focusedCanvas.element, evidenceHost, activityHost, integrationHost, connectionsHost];
check(renderedRoots.flatMap(descendants).every((node) =>
  node.tagName !== 'DIALOG' && !String(node.className).includes('rm-canvas-popover')
), 'canvas and detail interactions must not create a blocking dialog or popover');

(async function verifyAsyncLoaderBoundary() {
  function analyzedTarget(selectedID, programID, name, href) {
    return {
      selected_target_id: selectedID, program_target_id: programID,
      language: 'go', kind: 'library', display_name: name, state: 'analyzed', href
    };
  }
  function targetPayload(programID, name) {
    const emptyProjection = { eligible: 0, omitted: 0 };
    return {
      version: 1,
      target: { id: programID, language: 'go', kind: 'library', name, selector: 'go:' + name },
      openable_paths: [],
      features: {
        program: {
          objects: [], relations: [],
          projection: { seeds: emptyProjection, objects: emptyProjection, relations: emptyProjection }
        }
      }
    };
  }

  const repositoryPayload = {
    version: 1,
    repository: { name: 'example/async', captured_revision: 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb' },
    source: { kind: 'github', repository_url: 'https://github.com/example/async' },
    logical_default_selected_target_id: 'selected:a',
    targets: [
      analyzedTarget('selected:a', 'program:a', 'alpha', '?target=0#/program'),
      analyzedTarget('selected:b', 'program:b', 'beta', '?target=1#/program'),
      analyzedTarget('selected:c', 'program:c', 'slow', '?target=2#/program'),
      analyzedTarget('selected:d', 'program:d', 'broken', '?target=3#/program')
    ],
    openable_paths: [], warnings: []
  };

  const strictRepositoryModel = api.buildRepositoryPresentationModel(repositoryPayload);
  const unknownRepositoryPayload = JSON.parse(JSON.stringify(repositoryPayload));
  unknownRepositoryPayload.repository.unexpected = true;
  let nestedRepositoryUnknownRejected = false;
  try {
    api.buildRepositoryPresentationModel(unknownRepositoryPayload);
  } catch (error) {
    nestedRepositoryUnknownRejected = /unknown field/.test(String(error && error.message));
  }
  const unknownTargetPayload = targetPayload('program:a', 'alpha');
  unknownTargetPayload.features.program.unexpected = true;
  let nestedTargetUnknownRejected = false;
  try {
    api.buildTargetPresentationModel(unknownTargetPayload, strictRepositoryModel);
  } catch (error) {
    nestedTargetUnknownRejected = /unknown field/.test(String(error && error.message));
  }
  check(nestedRepositoryUnknownRejected && nestedTargetUnknownRejected,
    'feature-aware model boundaries must reject nested unknown browser payload fields');

  const partialTargetPayload = targetPayload('program:a', 'alpha');
  partialTargetPayload.features.core = {};
  let partialTargetRejected = false;
  try {
    api.buildTargetPresentationModel(partialTargetPayload, strictRepositoryModel);
  } catch (error) {
    partialTargetRejected = /partial semantic feature family/.test(String(error && error.message));
  }
  check(partialTargetRejected,
    'target model must reject a partial core/entrypoint/integration/activity-path family');

  const invalidSourceUnion = JSON.parse(JSON.stringify(repositoryPayload));
  invalidSourceUnion.source = {
    kind: 'github', repository_url: 'https://github.com/example/async',
    source_ids: { 'valid/path.go': 'a'.repeat(43) }
  };
  const invalidSourceMap = JSON.parse(JSON.stringify(repositoryPayload));
  invalidSourceMap.source = {
    kind: 'served',
    source_ids: { '../outside.go': 'a'.repeat(43), 'valid/path.go': 'a'.repeat(43) }
  };
  const invalidSourceID = JSON.parse(JSON.stringify(repositoryPayload));
  invalidSourceID.source = {
    kind: 'served', source_ids: { 'valid/path.go': 'too-short' }
  };
  const duplicateSourceID = JSON.parse(JSON.stringify(repositoryPayload));
  duplicateSourceID.source = {
    kind: 'served',
    source_ids: { 'valid/one.go': 'a'.repeat(43), 'valid/two.go': 'a'.repeat(43) }
  };
  let invalidSourceUnionRejected = false;
  let invalidSourceMapRejected = false;
  let invalidSourceIDRejected = false;
  let duplicateSourceIDRejected = false;
  try {
    api.buildRepositoryPresentationModel(invalidSourceUnion);
  } catch (error) {
    invalidSourceUnionRejected = /static source authority is invalid/.test(String(error && error.message));
  }
  try {
    api.buildRepositoryPresentationModel(invalidSourceMap);
  } catch (error) {
    invalidSourceMapRejected = /canonical repository-relative path|not unique/.test(
      String(error && error.message)
    );
  }
  try {
    api.buildRepositoryPresentationModel(invalidSourceID);
  } catch (error) {
    invalidSourceIDRejected = /source ID is invalid/.test(String(error && error.message));
  }
  try {
    api.buildRepositoryPresentationModel(duplicateSourceID);
  } catch (error) {
    duplicateSourceIDRejected = /source IDs are not unique/.test(String(error && error.message));
  }
  check(invalidSourceUnionRejected && invalidSourceMapRejected &&
    invalidSourceIDRejected && duplicateSourceIDRejected,
    'repository source shape must enforce its kind union and every dynamic source-ID entry');

  let incompleteRuntimeRejected = false;
  try {
    api.buildRepositoryPresentationModel(Object.assign({}, repositoryPayload, {
      runtime: {
        roles: [{
          name: 'partial runtime', purpose: 'Maps only one analyzed target.',
          prominence: 'primary', role_kind: 'service', requiredness: 'required', confidence: 'high',
          implementations: [{ program_target_id: 'program:a' }], evidence: []
        }],
        unclassified_targets: []
      }
    }));
  } catch (error) {
    incompleteRuntimeRejected = /cover every analyzed ProgramTarget/.test(String(error && error.message));
  }
  check(incompleteRuntimeRejected,
    'repository model must preserve exhaustive mapped-or-unclassified runtime target coverage');

  const ordinaryRepositoryPayload = Object.assign({}, repositoryPayload, {
    targets: [
      analyzedTarget('selected:a', 'program:a', 'alpha', '#/program'),
      analyzedTarget('selected:b', 'program:b', 'beta', '../sibling/report.html#/program')
    ]
  });
  const ordinaryRepositoryModel = api.buildRepositoryPresentationModel(ordinaryRepositoryPayload);
  context.window.location = {
    hash: '#/program', search: '', pathname: '/runs/owner/report.html', protocol: 'https:',
    hostname: 'example.test'
  };
  check(api.targetOutcomeForLocation(ordinaryRepositoryModel).selectedTargetID === 'selected:a',
    'ordinary current target href must match the exact current report pathname');
  const siblingRepositoryModel = api.buildRepositoryPresentationModel(Object.assign({}, repositoryPayload, {
    targets: [
      analyzedTarget('selected:a', 'program:a', 'alpha', '../owner/report.html#/program'),
      analyzedTarget('selected:b', 'program:b', 'beta', '#/program')
    ]
  }));
  context.window.location.pathname = '/runs/sibling/report.html';
  check(api.targetOutcomeForLocation(siblingRepositoryModel).selectedTargetID === 'selected:b',
    'ordinary sibling href must match pathname plus query instead of ambiguous empty query text');
  const payloads = {
    'selected:a': targetPayload('program:a', 'alpha'),
    'selected:b': targetPayload('program:b', 'beta'),
    'selected:c': targetPayload('program:c', 'slow')
  };
  const targetCalls = Object.create(null);
  let repositoryCalls = 0;
  let resolveSlow;
  const slowPayload = new Promise((resolve) => { resolveSlow = resolve; });
  const reportSource = {
    loadRepository() { repositoryCalls += 1; return Promise.resolve(repositoryPayload); },
    loadTarget(selectedID) {
      targetCalls[selectedID] = (targetCalls[selectedID] || 0) + 1;
      if (selectedID === 'selected:c') return slowPayload;
      if (selectedID === 'selected:d') return Promise.reject(new Error('x'.repeat(1000)));
      return Promise.resolve(payloads[selectedID]);
    }
  };

  targetNavigation.replaceChildren();
  const appHost = registerHeaderElement('rm-app', 'main');
  const fatalHost = registerHeaderElement('rm-fatal', 'section');
  fatalHost.hidden = true;
  registerHeaderElement('rm-fatal-message', 'p');
  const toastHost = registerHeaderElement('rm-toast', 'div');
  toastHost.hidden = true;
  context.window.location = {
    hash: '#/repository', search: '', pathname: '/report.html', protocol: 'https:',
    hostname: 'example.test'
  };

  await context.RepomapReportApp.boot(reportSource);
  check(repositoryCalls === 1 && !Object.keys(targetCalls).length,
    'repository boot must load repository data once and perform zero target loads');
  check(api.getState().model.target === null &&
    !Object.prototype.hasOwnProperty.call(api.getState().model, 'raw'),
    'repository presentation state must contain no target payload or raw model escape hatch');

  context.window.location.search = '?target=0';
  context.window.location.hash = '#/program';
  await api.transition();
  check(targetCalls['selected:a'] === 1 && api.getState().model.target.id === 'program:a' &&
    !Object.prototype.hasOwnProperty.call(api.getState().model, 'raw'),
    'program route must load exactly the target selected by its repository href');

  context.window.location.search = '?target=1';
  await api.transition();
  check(targetCalls['selected:b'] === 1 && api.getState().model.target.id === 'program:b',
    'a direct ?target=N handoff must load the exact matching selected target');

  context.window.location.search = '?target=2';
  const staleTransition = api.transition();
  context.window.location.hash = '#/repository';
  await api.transition();
  resolveSlow(payloads['selected:c']);
  await staleTransition;
  check(api.getState().model.target === null && appHost.textContent.includes('Repository overview'),
    'a slower target load must not replace a newer repository navigation');

  context.window.location.search = '?target=3';
  context.window.location.hash = '#/program';
  await api.transition();
  check(appHost.textContent.includes('This target could not be opened') &&
    appHost.textContent.length < 1000 && fatalHost.hidden === true,
    'target load failure must render a bounded local error instead of disabling the application');
  context.window.location.hash = '#/repository';
  await api.transition();
  check(appHost.textContent.includes('Repository overview') && repositoryCalls === 1,
    'repository overview must remain usable after a target load failure');

  api.resetBoot();
  let coldRepositoryCalls = 0;
  const coldTargetCalls = Object.create(null);
  const coldSource = {
    loadRepository() {
      coldRepositoryCalls += 1;
      return Promise.resolve(repositoryPayload);
    },
    loadTarget(selectedID) {
      coldTargetCalls[selectedID] = (coldTargetCalls[selectedID] || 0) + 1;
      return Promise.resolve(payloads[selectedID]);
    }
  };
  context.window.location = {
    hash: '#/program', search: '?target=1', pathname: '/report.html', protocol: 'https:',
    hostname: 'example.test'
  };
  await context.RepomapReportApp.boot(coldSource);
  check(coldRepositoryCalls === 1 && coldTargetCalls['selected:b'] === 1 &&
    Object.keys(coldTargetCalls).length === 1 && api.getState().model.target.id === 'program:b',
    'cold direct target boot must restore repository authority and load exactly its one selected target');

  if (failures.length) throw new Error(failures.join('\n'));
})().catch((error) => {
  process.stderr.write((error && error.stack ? error.stack : String(error)) + '\n');
  process.exitCode = 1;
});
`
