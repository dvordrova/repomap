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
	runnerPath := filepath.Join(tempDir, "report_app_behavior.js")
	if err := os.WriteFile(runnerPath, []byte(reportAppBehaviorRunner), 0o600); err != nil {
		t.Fatal(err)
	}

	output, err := exec.Command(node, runnerPath, appPath).CombinedOutput()
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
  }

  appendChild(child) {
    child.parentNode = this;
    this.children.push(child);
    return child;
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

  addEventListener(type, listener) {
    if (!this.listeners[type]) this.listeners[type] = [];
    this.listeners[type].push(listener);
  }

  querySelector(selector) {
    const tagName = String(selector).toUpperCase();
    return descendants(this).find((node) => node !== this && node.tagName === tagName) || null;
  }

  dispatch(type, values) {
    const event = Object.assign({
      type, target: this, currentTarget: this, defaultPrevented: false,
      preventDefault() { this.defaultPrevented = true; }
    }, values || {});
    (this.listeners[type] || []).forEach((listener) => listener.call(this, event));
    return event;
  }

  scrollIntoView(options) { this.scrollCalls.push(options); }
  focus(options) { this.focusCalls.push(options); }

  click() {
    const event = {
      type: 'click', target: this, currentTarget: this, defaultPrevented: false,
      preventDefault() { this.defaultPrevented = true; }
    };
    (this.listeners.click || []).forEach((listener) => listener.call(this, event));
    if (!event.defaultPrevented && this.tagName === 'A' && this.href.startsWith('#')) {
      context.window.location.hash = this.href;
    }
    return event;
  }
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
    requestAnimationFrame(callback) { callback(); }
  },
  URL,
  encodeURIComponent,
  decodeURIComponent
};
context.globalThis = context;

const source = fs.readFileSync(process.argv[2], 'utf8');
const bootBoundary = /\n\s*boot\(\);\s*\n\}\)\(\);\s*$/;
if (!bootBoundary.test(source)) {
  throw new Error('report_app.js does not have one recognizable boot boundary');
}
const exposure = [
  '  globalThis.__reportAppBehavior = {',
  '    setState: function (value) { state = value; },',
  '    selectedReportRoute: selectedReportRoute,',
  '    renderRepositoryFallback: renderRepositoryFallback,',
  '    renderActivityDetail: renderActivityDetail,',
  '    renderStarts: renderStarts,',
  '    compactDisplayText: compactDisplayText,',
  '    renderIntegrationDetail: renderIntegrationDetail,',
  '    renderEvidence: renderEvidence,',
  '    connectionsFor: connectionsFor,',
  '    renderConnections: renderConnections,',
  '    renderHeader: renderHeader,',
  '    reportRouteContext: reportRouteContext,',
  '    updateHeaderContext: updateHeaderContext,',
  '    crossSurfaceEmptyReason: crossSurfaceEmptyReason,',
  '    renderFlowCanvas: renderFlowCanvas,',
  '    canvasTopology: canvasTopology,',
  '    scheduleResponsibilityScroll: scheduleResponsibilityScroll,',
  '    getState: function () { return state; },',
  '    activityCanvasNode: activityCanvasNode,',
  '    coreCanvasNode: coreCanvasNode,',
  '    integrationCanvasNode: integrationCanvasNode',
  '  };',
  '})();'
].join('\n');
const instrumented = source.replace(bootBoundary, '\n' + exposure);
vm.runInNewContext(instrumented, context, { filename: process.argv[2] });

const api = context.__reportAppBehavior;
const activity = {
  id: 'entry/start here', name: 'Start here', kind: 'function', signature: 'start()',
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
  activityID: activity.id, callerID: activity.id, status: 'exact', steps: [], frontier: [],
  outcomes: [{
    dependencyID: integration.id,
    use: { authority: 'exact_external_symbol', label: 'send request' }
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
  model, focusGroupID: null, completeCanvas: false, canvasEdges: [],
  pendingResponsibilityID: '',
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
const targetLinks = descendants(targetNavigation).filter((node) =>
  node.tagName === 'A' && String(node.className).includes('rm-target-switcher__target')
);
check(targetRepository.textContent === 'fukict/fukict' && targetCurrent.textContent === 'Target · fukict' &&
  targetCount.textContent === '2' && targetPanelCount.textContent === '2 targets',
  'the compact target switcher must identify the repository, current target, and complete target count');
check(targetLinks.length === 2 && targetLinks[0].getAttribute('aria-current') === 'true' &&
  targetLinks[0].textContent.includes('Current') && !targetLinks[0].textContent.includes('Default') &&
  targetLinks[1].textContent.includes('@fukict/babel-preset') && targetLinks[1].textContent.includes('Default'),
  'the compact target switcher must distinguish current and repository-default targets');
targetSwitcher.open = true;
targetLinks[1].click();
check(targetSwitcher.open === false,
  'choosing a target must close the compact target switcher');
targetSwitcher.open = true;
targetSwitcher.dispatch('keydown', { key: 'Escape' });
check(targetSwitcher.open === false && targetSummary.focusCalls.length === 1,
  'Escape must close the target switcher and restore focus to its summary');
check(api.reportRouteContext({ kind: 'repository' }) === 'Target overview' &&
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
check(api.selectedReportRoute().kind === 'program',
  'an empty JavaScript/TypeScript surface overview must default to the useful program map');
model.surfaceCatalog.surfaces.push({ id: 'surface:browser' });
check(api.selectedReportRoute().kind === 'repository',
  'a non-empty JavaScript/TypeScript surface overview must remain the default');
model.surfaceCatalog.surfaces = [];
model.crossSurfacePaths.paths.push({ id: 'path:request' });
check(api.selectedReportRoute().kind === 'repository',
  'a path-only JavaScript/TypeScript overview must remain the default');
model.surfaceCatalog = null;
model.crossSurfacePaths = null;
model.runtimePortfolio = { roles: [], unclassified: [] };
check(api.selectedReportRoute().kind === 'program',
  'an empty runtime portfolio must not shadow the useful program map');
model.runtimePortfolio.roles.push({ id: 'role:service' });
check(api.selectedReportRoute().kind === 'repository',
  'a runtime role must make the repository overview useful');
model.runtimePortfolio.roles = [];
model.runtimePortfolio.unclassified.push({ id: 'target:unknown' });
check(api.selectedReportRoute().kind === 'repository',
  'an unclassified target must make the repository overview useful');
model.runtimePortfolio = null;

const focusedTopology = api.canvasTopology([block.id], false);
const directEdge = focusedTopology.edges.find((edge) =>
  edge.from === 'entry:' + activity.id && edge.to === 'integration:' + integration.id
);
check(!!directEdge && directEdge.blockIDs.length === 0,
  'focused topology must retain a direct ungrouped entrypoint-to-integration frontier');
check(focusedTopology.directFrontierEdges === 1,
  'focused topology must account for its direct frontier separately from core grouping');
check(focusedTopology.starts.includes(activity) && focusedTopology.integrations.includes(integration),
  'direct frontier endpoints must remain visible in focused topology');
check(focusedTopology.hiddenConnectedIntegrations === 0,
  'a visible direct-frontier integration must not be reported as belonging to another group');
check(!focusedTopology.edges.some((edge) =>
  edge.from === 'core:' + block.id && edge.to === 'core:' + storageBlock.id
), 'focused topology must not retain a core edge whose target card is not visible');
check(focusedTopology.hiddenPlacedStarts >= 0 && focusedTopology.hiddenConnectedIntegrations >= 0,
  'focused hidden connection counts must never become negative');
const unconnectedActivity = {
  id: 'entry:unconnected', name: 'Unconnected start', kind: 'function', signature: 'unused()',
  location: { path: 'app/main.py', line: 20, column: 1 }
};
model.activities.push(unconnectedActivity);
const completeTopology = api.canvasTopology([block.id], true);
check(completeTopology.starts.length === 2 && completeTopology.connectedStartCount === 1 &&
  completeTopology.hiddenPlacedStarts === 0,
  'complete topology must distinguish all shown entrypoints from the connected subset without negative hidden counts');
model.activities.pop();

const focusedCanvas = api.renderFlowCanvas(block);
check(focusedCanvas.textContent.includes('visible as a direct frontier outside core grouping'),
  'focused canvas must explain why an ungrouped direct route remains visible');
check(!focusedCanvas.textContent.includes('belong to other grouping selections'),
  'focused canvas must not assign direct-frontier integrations to another grouping');
check(source.includes('completeCanvas: false') &&
  !source.includes('Only exact selected facts are connected.'),
  'large reports must start focused and complete-mode copy must not hide possible/runtime authority');
check(focusedCanvas.textContent.includes('2 directional core links'),
  'the no-group canvas must show all core cards and account for separately authoritative directed links');
const visibleCoreEdges = api.getState().canvasEdges.filter((edge) =>
  edge.from === 'core:' + block.id && edge.to === 'core:' + storageBlock.id
);
check(visibleCoreEdges.length === 2 && visibleCoreEdges.some((edge) => edge.resolution === 'exact') &&
  visibleCoreEdges.some((edge) => edge.resolution === 'possible'),
  'exact and possible authority for one endpoint pair must remain separate');

const wrappers = [
  api.activityCanvasNode(activity),
  api.coreCanvasNode(block, false, null),
  api.integrationCanvasNode(integration)
];
const controls = wrappers.flatMap((wrapper) => descendants(wrapper).filter((node) =>
  node.getAttribute('data-canvas-node') !== null
));
check(controls.length === wrappers.length,
  'every canvas node must expose exactly one interactive control');
check(controls.every((control) => control && control.tagName === 'A'),
  'canvas nodes must be links, not inert buttons');
check(controls.every((control) => control && control.href.startsWith('#/program/')),
  'every canvas node must have a program hash target');
check(wrappers[1].textContent.includes('→ 1 exact downstream') &&
  wrappers[1].textContent.includes('⇢ 1 possible downstream') &&
  wrappers[1].textContent.includes('Inspect responsibility ↓'),
  'a core card must expose separate directed authorities and explain that it opens the detail below');
check(source.includes("marker-end', 'url(#rm-canvas-arrow-") && source.includes('sameLane ? targetBounds.right') &&
  source.includes('authorityOffset'),
  'canvas edges must draw arrowheads and offset parallel exact/possible/runtime authority');

function renderedCoreSelection(selectedID) {
  return [block, storageBlock].flatMap((candidate) => descendants(
    api.coreCanvasNode(candidate, candidate.id === selectedID, null)
  )).filter((node) => node.getAttribute('data-canvas-node') !== null);
}
let selectedControls = renderedCoreSelection(block.id);
check(selectedControls.filter((control) => control.getAttribute('aria-current') === 'true').length === 1 &&
  selectedControls.find((control) => control.getAttribute('aria-current') === 'true').getAttribute('data-canvas-node') ===
    'core:' + block.id,
  'the first core selection must expose exactly one current canvas control');
selectedControls = renderedCoreSelection(storageBlock.id);
check(selectedControls.filter((control) => control.getAttribute('aria-current') === 'true').length === 1 &&
  selectedControls.find((control) => control.getAttribute('aria-current') === 'true').getAttribute('data-canvas-node') ===
    'core:' + storageBlock.id,
  'a sequential core selection must replace, not accumulate, the current canvas control');

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

const entryControl = controls[0];
if (entryControl) entryControl.click();
check(context.window.location.hash === '#/program/entrypoint/entry%2Fstart%20here',
  'the entrypoint canvas link must preserve its exact encoded identity');
let route = api.selectedReportRoute();
check(route.kind === 'entrypoint' && route.activity === activity,
  'the entrypoint hash must resolve to its exact selected activity');
const activityHost = new TestElement('main');
model.blocksBySymbol[activity.id] = [block.id];
api.renderActivityDetail(activityHost, route.activity);
check(activityHost.textContent.includes('Start here') &&
  activityHost.textContent.includes('Paths to selected integrations'),
  'the entrypoint route must render a useful semantic detail page');
check(descendants(activityHost).some((node) => node.className === 'rm-entrypoint-header') &&
  !descendants(activityHost).some((node) => node.className === 'rm-detail-hero') &&
  activityHost.textContent.includes('Core responsibility touchpoints') &&
  activityHost.textContent.includes(block.purpose) &&
  activityHost.textContent.includes('Starts here') &&
  activityHost.textContent.includes('Exact selected path'),
  'entrypoint detail must use a compact header and expose existing responsibility and route facts');

const coreControl = controls[1];
const coreClick = coreControl ? coreControl.click() : null;
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

const integrationControl = controls[2];
if (integrationControl) integrationControl.click();
check(context.window.location.hash === '#/program/integration/dependency%3Ahttp%20client',
  'the integration canvas link must preserve its exact encoded identity');
route = api.selectedReportRoute();
check(route.kind === 'integration' && route.integration === integration,
  'the integration hash must resolve to its exact selected dependency');
const integrationHost = new TestElement('main');
api.renderIntegrationDetail(integrationHost, route.integration);
check(integrationHost.textContent.includes('HTTP client') &&
  integrationHost.textContent.includes('Selected operations'),
  'the integration route must render a useful semantic detail page');

const connectionBlock = {
  id: block.id, name: block.name, purpose: block.purpose,
  files: block.files, symbols: [sourceObject], depth: block.depth
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
    id: 'relation:exact-one', from_id: sourceObject.id, to_ids: [targetObject.id],
    kind: 'calls', resolution: 'exact',
    location: { path: 'app/execution.py', line: 10, column: 3 },
    witnesses: [
      { kind: 'fixture', location: { path: 'app/execution.py', line: 10, column: 3 } },
      { kind: 'fixture', location: { path: 'app/execution.py', line: 16, column: 3 } }
    ]
  }),
  accountedRelation({
    id: 'relation:exact-two', from_id: sourceObject.id, to_ids: [targetObject.id],
    kind: 'calls', resolution: 'exact',
    location: { path: 'app/execution.py', line: 11, column: 3 }
  }),
  accountedRelation({
    id: 'relation:unresolved-send', from_id: sourceObject.id, to_ids: [],
    kind: 'calls', resolution: 'unresolved',
    location: { path: 'app/execution.py', line: 12, column: 3 },
    witnesses: [{ source_expression: 'client.send', location: { path: 'app/execution.py', line: 12, column: 3 } }]
  }),
  accountedRelation({
    id: 'relation:unresolved-flush', from_id: sourceObject.id, to_ids: [],
    kind: 'calls', resolution: 'unresolved',
    location: { path: 'app/execution.py', line: 13, column: 3 },
    witnesses: [{ source_expression: 'client.flush', location: { path: 'app/execution.py', line: 13, column: 3 } }]
  }),
  accountedRelation({
    id: 'relation:platform-date', from_id: sourceObject.id, to_ids: [platformObject.id],
    kind: 'invokes_external', resolution: 'exact', invocation: 'construct',
    location: { path: 'app/execution.py', line: 14, column: 3 }
  }, { observed: 3, indexed: 1 }),
  accountedRelation({
    id: 'relation:package-effect', from_id: sourceObject.id, to_ids: [packageObject.id],
    kind: 'invokes_external', resolution: 'exact', invocation: 'call',
    location: { path: 'app/execution.py', line: 15, column: 3 }
  }, { observed: 2, indexed: 2 })
];
const connections = api.connectionsFor(connectionBlock);
const exactConnections = connections.filter((connection) => connection.resolution === 'exact');
const localExactConnections = exactConnections.filter((connection) =>
  connection.targetIDs.includes(targetObject.id)
);
const unresolvedConnections = connections.filter((connection) => connection.resolution === 'unresolved');
check(exactConnections.length === 3 && localExactConnections.length === 1 &&
  localExactConnections[0].relationIDs.length === 2,
  'relations with the same resolved target must remain one compact connection group');
check(unresolvedConnections.length === 2 && unresolvedConnections.every((connection) =>
  connection.relationIDs.length === 1
), 'different unresolved relation records must remain separate connection rows');
check(unresolvedConnections.some((connection) => connection.to === 'unresolved call: client.send') &&
  unresolvedConnections.some((connection) => connection.to === 'unresolved call: client.flush'),
  'every unresolved connection row must retain its exact witnessed expression');

const connectionsHost = new TestElement('main');
api.renderConnections(connectionsHost, connectionBlock, connections);
const connectionLinks = descendants(connectionsHost).filter((node) =>
  node.tagName === 'A' && node.href.includes('/blob/')
);
const expectedConnectionLocations = [10, 11, 12, 13, 14, 15, 16].map((line) =>
  'app/execution.py:' + String(line)
);
check(connectionLinks.length === expectedConnectionLocations.length &&
  expectedConnectionLocations.every((location) =>
    connectionLinks.some((link) => link.textContent === location && link.href.endsWith('#L' + location.split(':')[1]))
  ),
  'every grouped relation and witness location must render as one exact path:line source link');
check(connectionsHost.textContent.includes('5 relation groups · 6 relation records') &&
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
check(connectionsHost.textContent.includes('execute — creates a JavaScript platform value → JavaScript Date') &&
  connectionsHost.textContent.includes('execute — uses a resolved external/runtime API → react.useEffect') &&
  !connectionsHost.textContent.includes('app/execution#'),
  'relation titles must expose their semantic verb, omit the redundant declaration path, and retain exact source links');
const connectionPaths = descendants(connectionsHost).filter((node) =>
  String(node.className).includes('rm-connection__path')
);
check(connectionPaths.some((path) => path.textContent.includes('creates a JavaScript platform value')) &&
  connectionPaths.some((path) => path.textContent.includes('uses a resolved external/runtime API')),
  'compact connection rows must keep relation semantics visible');
check(!connectionsHost.textContent.includes('Open source location') &&
  !descendants(connectionsHost).some((node) => String(node.className).includes('rm-connection__meta')) &&
  !connectionsHost.textContent.includes('relation records · 3 source locations') &&
  connectionsHost.textContent.includes('3 relation witness details are not shown'),
  'connection rows must omit redundant location labels and counters while retaining omission-only diagnostics');

const renderedRoots = wrappers.concat([focusedCanvas, evidenceHost, activityHost, integrationHost, connectionsHost]);
check(renderedRoots.flatMap(descendants).every((node) =>
  node.tagName !== 'DIALOG' && !String(node.className).includes('rm-canvas-popover')
), 'canvas and detail interactions must not create a blocking dialog or popover');

if (failures.length) {
  throw new Error(failures.join('\n'));
}
`
