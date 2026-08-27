package report

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/coremap"
)

func TestSystemCanvasPureModulesPreserveGraphAndInteractionContracts(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	tempDir := t.TempDir()
	assets := []string{
		systemCanvasGraphJS,
		systemCanvasInteractionJS,
		systemCanvasGeometryJS,
	}
	assetPaths := make([]string, 0, len(assets))
	for index, source := range assets {
		path := filepath.Join(tempDir, fmt.Sprintf("system_canvas_%d.js", index))
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
		assetPaths = append(assetPaths, path)
	}
	runnerPath := filepath.Join(tempDir, "system_canvas_modules.js")
	if err := os.WriteFile(runnerPath, []byte(systemCanvasPureModuleRunner), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(node, append([]string{runnerPath}, assetPaths...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("run System canvas pure-module contract: %v\n%s", err, output)
	}
}

func TestSystemCanvasRealBrowserInteractionDoesNotRebuildGeometry(t *testing.T) {
	browser := systemCanvasHeadlessBrowser()
	if browser == "" {
		t.Skip("Chrome or Chromium is unavailable")
	}
	html := systemCanvasGeneratedReportHTML(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(html))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	profile := t.TempDir()
	command := exec.CommandContext(ctx, browser,
		"--headless=new",
		"--disable-background-networking",
		"--disable-component-update",
		"--disable-default-apps",
		"--disable-dev-shm-usage",
		"--disable-gpu",
		"--hide-scrollbars",
		"--no-first-run",
		"--no-sandbox",
		"--window-size=1400,1000",
		"--virtual-time-budget=8000",
		"--dump-dom",
		"--user-data-dir="+profile,
		server.URL+"/#/program",
	)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := runSystemCanvasHeadlessDump(ctx, command)
	if bytes.Contains(output, []byte(`id="test-result" data-status="pass"`)) {
		return
	}
	if ctx.Err() != nil {
		t.Fatalf("headless browser timed out before the contract passed: %v\n%s\n%s", ctx.Err(),
			systemCanvasBrowserResult(output), stderr.String())
	}
	if err != nil {
		t.Fatalf("run headless browser System canvas contract: %v\n%s\n%s", err,
			systemCanvasBrowserResult(output), stderr.String())
	}
	t.Fatalf("real-browser System canvas contract did not pass:\n%s", systemCanvasBrowserResult(output))
}

func runSystemCanvasHeadlessDump(ctx context.Context, command *exec.Cmd) ([]byte, error) {
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if command.Stderr == nil {
		command.Stderr = io.Discard
	}
	if err := command.Start(); err != nil {
		return nil, err
	}
	type readResult struct {
		chunk []byte
		err   error
	}
	reads := make(chan readResult, 1)
	go func() {
		buffer := make([]byte, 32*1024)
		for {
			count, readErr := stdout.Read(buffer)
			if count > 0 {
				chunk := append([]byte(nil), buffer[:count]...)
				reads <- readResult{chunk: chunk}
			}
			if readErr != nil {
				reads <- readResult{err: readErr}
				return
			}
		}
	}()
	var output bytes.Buffer
	for {
		select {
		case read := <-reads:
			if len(read.chunk) > 0 {
				_, _ = output.Write(read.chunk)
				if bytes.Contains(output.Bytes(), []byte("</html>")) {
					_ = command.Process.Kill()
					_ = command.Wait()
					return output.Bytes(), nil
				}
			}
			if read.err != nil {
				waitErr := command.Wait()
				if read.err != io.EOF {
					return output.Bytes(), read.err
				}
				return output.Bytes(), waitErr
			}
		case <-ctx.Done():
			_ = command.Process.Kill()
			_ = command.Wait()
			return output.Bytes(), ctx.Err()
		}
	}
}

func systemCanvasBrowserResult(output []byte) string {
	const marker = `id="test-result"`
	position := bytes.Index(output, []byte(marker))
	if position < 0 {
		if len(output) > 2000 {
			return string(output[len(output)-2000:])
		}
		return string(output)
	}
	start := position - 200
	if start < 0 {
		start = 0
	}
	end := position + 2000
	if end > len(output) {
		end = len(output)
	}
	return string(output[start:end])
}

func systemCanvasHeadlessBrowser() string {
	for _, candidate := range []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
	} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

func systemCanvasGeneratedReportHTML(t *testing.T) string {
	t.Helper()
	data := reportProgramShellDataFixture(t, "system-canvas-browser-fixture")
	data.GitHubSourceLinks = &GitHubSourceLinks{
		RepositoryURL: "https://github.com/example/system-canvas-fixture",
		Revision:      data.CapturedRevision,
	}
	view := data.ProgramPortfolio.Entries[0].View
	objectsBySourceRef := make(map[string]ProgramViewObject, len(view.Objects))
	for _, object := range view.Objects {
		objectsBySourceRef[object.SourceRef] = object
	}

	block := func(id, name, purpose, sourceRef string, incoming, outgoing int) CoreMapViewBlock {
		object, ok := objectsBySourceRef[sourceRef]
		if !ok || object.Location == nil {
			t.Fatalf("generated report fixture is missing exact object %q", sourceRef)
		}
		return CoreMapViewBlock{
			ID: id, Name: name, Purpose: purpose,
			Files: []CoreMapViewFile{},
			RepresentativeSymbols: []CoreMapViewRepresentativeSymbol{{
				Kind: object.Kind, Visibility: object.Visibility,
				IncomingCalls: incoming, OutgoingCalls: outgoing,
				Symbol: CoreMapViewSymbol{
					NodeID: object.ID, Package: "fixture", Name: object.Name,
					Location: CoreMapViewLocation{
						Path: object.Location.Path, Line: object.Location.Line, Column: object.Location.Column,
					},
				},
			}},
			Children: []CoreMapViewBlock{},
		}
	}

	coreView := *data.CoreMapView
	coreView.RefinedCore = []CoreMapViewBlock{
		block("alpha", "Alpha", "Owns the selected activity start.", "fn-start", 0, 0),
		block("beta", "Beta", "Calls the final responsibility.", "fn-clean", 0, 1),
		block("gamma", "Gamma", "Receives the exact local call.", "fn-load", 1, 0),
	}
	coreView.RefinedGroups = []CoreMapViewGroup{
		{
			ID: "complete-flow", Authority: coremap.GroupAuthorityModel,
			Name: "Complete flow", Purpose: "Keeps all browser-test responsibilities visible.",
			CoreBlockIDs: []string{"alpha", "beta", "gamma"},
		},
		{
			ID: "alternate-flow", Authority: coremap.GroupAuthorityModel,
			Name: "Alternate flow", Purpose: "Exercises shell-owned grouping navigation without scrolling.",
			CoreBlockIDs: []string{"alpha", "beta", "gamma"},
		},
	}
	coreView.Coverage.SymbolsAvailable = 3
	coreView.Coverage.RefinedBlocks = 3
	coreView.Coverage.RefinedFilesSelected = 0
	coreView.Coverage.RefinedSymbolsSelected = 3
	coreView.Coverage.RefinedGroups = 2
	coreView.Coverage.RefinedModelGroups = 2
	coreView.Coverage.RefinedLocalGroups = 0
	coreView.Coverage.RefinedUnassignedBlocks = 0
	coreView.Coverage.RefinedGroupCalls = 1
	data.CoreMapView = &coreView

	rendered, err := RenderHTMLWithOptions(&data, RenderOptions{})
	if err != nil {
		t.Fatalf("render generated System canvas report: %v", err)
	}
	closingBody := bytes.LastIndex(rendered, []byte("</body>"))
	if closingBody < 0 {
		t.Fatal("generated System canvas report has no closing body")
	}
	var driver strings.Builder
	driver.WriteString(`<style>.rm-canvas-edge-group,.rm-canvas-edge-port,.rm-canvas-node{transition:none!important}</style>`)
	driver.WriteString(`<pre id="test-result" data-status="pending"></pre><script>`)
	driver.WriteString(strings.ReplaceAll(systemCanvasGeneratedReportBrowserRunner, "</script", `<\/script`))
	driver.WriteString(`</script>`)
	return string(rendered[:closingBody]) + driver.String() + string(rendered[closingBody:])
}

const systemCanvasFixture = `{
  activities: [
    {id: 'start', name: 'src/main#start', kind: 'function', signature: '()', location: {path: 'src/main.ts', line: 1, column: 1}}
  ],
  blocks: [
    {id: 'alpha', name: 'Alpha', purpose: 'Starts the flow.', symbols: ['symbol-alpha']},
    {id: 'beta', name: 'Beta', purpose: 'Coordinates the flow.', symbols: ['symbol-beta']},
    {id: 'gamma', name: 'Gamma', purpose: 'Finishes the flow.', symbols: ['symbol-gamma']},
    {id: 'a:b', name: 'Colon source', purpose: 'Exercises stable identity.', symbols: ['symbol-a-b']},
    {id: 'c', name: 'Colon target', purpose: 'Exercises stable identity.', symbols: ['symbol-c']},
    {id: 'a', name: 'Short source', purpose: 'Exercises stable identity.', symbols: ['symbol-a']},
    {id: 'b:c', name: 'Long target', purpose: 'Exercises stable identity.', symbols: ['symbol-b-c']}
  ],
  integrations: [
    {id: 'exact-dependency', name: 'Exact dependency', kind: 'package', packagePath: 'exact-package', uses: [
      {callerID: 'symbol-beta', authority: 'exact_external_symbol'}
    ]},
    {id: 'runtime-dependency', name: 'Runtime dependency', kind: 'package', packagePath: 'runtime-package', uses: [
      {callerID: 'symbol-gamma', authority: 'syntactic_unresolved'}
    ]}
  ],
  relations: [
    {kind: 'calls', resolution: 'exact', from_id: 'symbol-alpha', to_ids: ['symbol-beta']},
    {kind: 'writes', resolution: 'exact', from_id: 'symbol-alpha', to_ids: ['symbol-beta']},
    {kind: 'calls', resolution: 'alternatives', from_id: 'symbol-beta', to_ids: ['symbol-gamma']},
    {kind: 'calls', resolution: 'exact', from_id: 'symbol-a-b', to_ids: ['symbol-c']},
    {kind: 'calls', resolution: 'exact', from_id: 'symbol-a', to_ids: ['symbol-b-c']}
  ],
  blocksBySymbol: {
    start: ['alpha'],
    'symbol-alpha': ['alpha'],
    'symbol-beta': ['beta'],
    'symbol-gamma': ['gamma'],
    'symbol-a-b': ['a:b'],
    'symbol-c': ['c'],
    'symbol-a': ['a'],
    'symbol-b-c': ['b:c']
  },
  groupsByBlock: {
    alpha: ['flow'], beta: ['flow'], gamma: ['flow'],
    'a:b': ['identity'], c: ['identity'], a: ['identity'], 'b:c': ['identity']
  }
}`

const systemCanvasPureModuleRunner = `
'use strict';
const fs = require('fs');
const vm = require('vm');
for (const path of process.argv.slice(2)) vm.runInThisContext(fs.readFileSync(path, 'utf8'), {filename: path});

function assert(condition, message) {
  if (!condition) throw new Error(message);
}
function equal(actual, expected, message) {
  const left = JSON.stringify(actual);
  const right = JSON.stringify(expected);
  if (left !== right) throw new Error(message + ': got ' + left + ', want ' + right);
}
const model = ` + systemCanvasFixture + `;
const Graph = globalThis.RepomapSystemCanvasGraph;
const Interaction = globalThis.RepomapSystemCanvasInteraction;
const Geometry = globalThis.RepomapSystemCanvasGeometry;

const focused = Graph.buildCanvasGraph(model, {activeBlockIDs: ['alpha', 'beta'], complete: false});
const complete = Graph.buildCanvasGraph(model, {activeBlockIDs: ['alpha', 'beta'], complete: true});
const repeated = Graph.buildCanvasGraph(model, {activeBlockIDs: ['alpha', 'beta'], complete: true});
equal(complete.nodes.map((node) => node.id), repeated.nodes.map((node) => node.id), 'node IDs are stable');
equal(complete.edges.map((edge) => edge.id), repeated.edges.map((edge) => edge.id), 'edge IDs are stable');
assert(new Set(complete.nodes.map((node) => node.id)).size === complete.nodes.length, 'node IDs are unique');
assert(new Set(complete.edges.map((edge) => edge.id)).size === complete.edges.length, 'edge IDs are unique');

const alphaBeta = complete.edges.filter((edge) => edge.sourceID === 'core:alpha' && edge.targetID === 'core:beta');
assert(alphaBeta.length === 1, 'duplicate semantic rows condense exactly once');
assert(alphaBeta[0].authority === 'exact', 'exact authority survives projection');
const betaGamma = complete.edges.find((edge) => edge.sourceID === 'core:beta' && edge.targetID === 'core:gamma');
assert(betaGamma && betaGamma.authority === 'possible', 'possible authority survives projection');
const gammaRuntime = complete.edges.find((edge) => edge.sourceID === 'core:gamma' && edge.targetID === 'integration:runtime-dependency');
assert(gammaRuntime && gammaRuntime.authority === 'runtime', 'runtime authority survives projection');
const exactDependency = complete.edges.find((edge) => edge.sourceID === 'core:beta' && edge.targetID === 'integration:exact-dependency');
assert(exactDependency && exactDependency.authority === 'exact', 'exact integration authority survives projection');

const collisionLeft = complete.edges.find((edge) => edge.sourceID === 'core:a:b' && edge.targetID === 'core:c');
const collisionRight = complete.edges.find((edge) => edge.sourceID === 'core:a' && edge.targetID === 'core:b:c');
assert(collisionLeft && collisionRight && collisionLeft.id !== collisionRight.id, 'length-framed edge IDs resist separator collisions');

assert(focused.nodesByID['core:alpha'] && focused.nodesByID['core:beta'], 'focused blocks remain visible');
assert(!focused.nodesByID['core:gamma'], 'inactive blocks are hidden in focused mode');
assert(!focused.edgesByID[betaGamma.id] && !focused.edgesByID[gammaRuntime.id], 'focused mode hides edges outside the selected blocks');
assert(complete.nodesByID['core:gamma'] && complete.edgesByID[betaGamma.id] && complete.edgesByID[gammaRuntime.id], 'complete mode restores hidden nodes and edges');
assert(complete.incomingByNodeID['core:beta'].some((edge) => edge.id === alphaBeta[0].id), 'incoming adjacency is exact');
assert(complete.outgoingByNodeID['core:beta'].some((edge) => edge.id === betaGamma.id), 'outgoing adjacency is exact');
assert(complete.incidentEdgesByNodeID['core:beta'].length === 3, 'incident adjacency contains each visible edge once');

let state = Interaction.createInteractionState();
state = Interaction.reduceCanvasInteraction(state, {type: 'NODE_POINTER_ENTER', nodeID: 'core:beta'}, complete);
let emphasis = Interaction.deriveCanvasEmphasis(complete, state);
assert(emphasis.mode === 'node' && emphasis.activeNodeID === 'core:beta', 'node hover activates the node');
equal(new Set(emphasis.emphasizedEdgeIDs).size, 3, 'node hover emphasizes its incident edges');
assert(emphasis.visibleEndpointIDs.length === 6, 'node hover exposes both endpoints of every incident edge');
state = Interaction.reduceCanvasInteraction(state, {type: 'NODE_POINTER_LEAVE', nodeID: 'core:beta'}, complete);
assert(Interaction.deriveCanvasEmphasis(complete, state).mode === 'none', 'node leave clears hover emphasis');
state = Interaction.reduceCanvasInteraction(state, {type: 'NODE_FOCUS', nodeID: 'core:beta'}, complete);
assert(Interaction.deriveCanvasEmphasis(complete, state).activeNodeID === 'core:beta', 'keyboard focus activates the node');
state = Interaction.reduceCanvasInteraction(state, {type: 'NODE_POINTER_ENTER', nodeID: 'core:gamma'}, complete);
assert(Interaction.deriveCanvasEmphasis(complete, state).activeNodeID === 'core:gamma',
  'pointer hover takes precedence over retained keyboard focus');
state = Interaction.reduceCanvasInteraction(state, {type: 'NODE_POINTER_LEAVE', nodeID: 'core:gamma'}, complete);
assert(Interaction.deriveCanvasEmphasis(complete, state).activeNodeID === 'core:beta',
  'leaving the pointer restores retained keyboard focus');
state = Interaction.reduceCanvasInteraction(state, {type: 'NODE_BLUR', nodeID: 'core:beta'}, complete);

const endpointID = Interaction.endpointID(betaGamma.id, 'core:beta', 'source');
state = Interaction.reduceCanvasInteraction(state, {type: 'ENDPOINT_POINTER_ENTER', endpointID}, complete);
emphasis = Interaction.deriveCanvasEmphasis(complete, state);
assert(emphasis.mode === 'edge' && emphasis.activeEdgeID === betaGamma.id, 'endpoint hover activates exactly one edge');
assert(emphasis.emphasizedEdgeIDs.length === 1 && emphasis.oppositeNodeID === 'core:gamma', 'endpoint hover identifies the opposite node');
assert(Interaction.oppositeNodeID(complete, betaGamma.id, 'core:beta') === 'core:gamma', 'opposite node resolves from source');
assert(Interaction.oppositeNodeID(complete, betaGamma.id, 'core:gamma') === 'core:beta', 'opposite node resolves from target');
state = Interaction.reduceCanvasInteraction(state, {type: 'ENDPOINT_POINTER_LEAVE', endpointID}, complete);
state = Interaction.reduceCanvasInteraction(state, {type: 'ENDPOINT_FOCUS', endpointID}, complete);
assert(Interaction.deriveCanvasEmphasis(complete, state).activeEdgeID === betaGamma.id, 'endpoint focus activates exactly one edge');
state = Interaction.reduceCanvasInteraction(state, {type: 'ENDPOINT_CLICK', endpointID}, complete);
assert(state.pinnedNodeID === '' && state.activatedEndpointID === endpointID,
  'endpoint click records activation without inventing persistent pin behavior');
state = Interaction.reduceCanvasInteraction(state, {type: 'ENDPOINT_BLUR', endpointID}, complete);
state = Interaction.reduceCanvasInteraction(state, {type: 'NODE_PIN', nodeID: 'core:gamma'}, complete);
assert(state.pinnedNodeID === 'core:gamma', 'explicit node pinning remains available to the renderer');
state = Interaction.reduceCanvasInteraction(state, {type: 'ESCAPE'}, complete);
assert(state.pinnedNodeID === '' && state.activatedEndpointID === '', 'Escape clears pinned interaction');
state = Interaction.reduceCanvasInteraction(state, {type: 'EDGE_POINTER_ENTER', edgeID: alphaBeta[0].id}, complete);
assert(Interaction.deriveCanvasEmphasis(complete, state).activeEdgeID === alphaBeta[0].id, 'edge hover activates one edge');
state = Interaction.reduceCanvasInteraction(state, {type: 'CLEAR_INTERACTION'}, complete);
assert(Interaction.deriveCanvasEmphasis(complete, state).mode === 'none', 'clear resets all interaction');

const laneX = {entry: 10, core: 300, integration: 650};
const lanePositions = {entry: 0, core: 0, integration: 0};
const nodesByID = Object.create(null);
for (const node of complete.nodes) {
  const top = 20 + lanePositions[node.lane]++ * 80;
  nodesByID[node.id] = {left: laneX[node.lane], top, width: 180, height: 48};
}
const measurements = {width: 900, height: 900, nodesByID};
const ports = Geometry.assignStablePorts(complete, measurements);
const repeatedPorts = Geometry.assignStablePorts(complete, measurements);
equal(ports, repeatedPorts, 'port identities and slots are stable');
assert(ports.ports.length === complete.edges.length * 2, 'every edge owns two stable ports');
assert(new Set(ports.ports.map((port) => port.id)).size === ports.ports.length, 'port IDs are unique');
for (const edge of complete.edges) {
  assert(ports.portsByEdgeID[edge.id].source && ports.portsByEdgeID[edge.id].target, 'edge port roles are complete');
}
const geometry = Geometry.buildEdgeGeometry(complete, measurements, ports);
const repeatedGeometry = Geometry.buildEdgeGeometry(complete, measurements, repeatedPorts);
equal(geometry, repeatedGeometry, 'edge geometry is deterministic for the same numeric measurements');
assert(geometry.edges.every((edge) => edge.path.startsWith('M ') && edge.authority === complete.edgesByID[edge.id].authority), 'geometry retains path and authority');
assert(geometry.edges.every((edge) => {
  const sameLane = complete.nodesByID[edge.sourceID].lane === complete.nodesByID[edge.targetID].lane;
  return edge.target.y === edge.targetPort.y &&
    edge.target.x - edge.targetPort.x === (sameLane ? 12 : -12);
}), 'arrow tips stop outside endpoint circles with a stable lane-aware clearance');
assert(geometry.edgesByID[alphaBeta[0].id].track !== geometry.edgesByID[betaGamma.id].track,
  'overlapping same-lane relations use separate deterministic outer tracks');

const denseEntries = Array.from({length: 8}, (_, index) => ({
  id: 'entry:dense-' + String(index), lane: 'entry', kind: 'entrypoint',
  data: {name: 'dense-' + String(index)}
}));
const denseCore = {id: 'core:dense', lane: 'core', kind: 'core', data: {name: 'Dense core'}};
const denseEdges = denseEntries.map((entry, index) => ({
  id: 'dense-edge-' + String(index), sourceID: entry.id, targetID: denseCore.id,
  authority: 'exact', description: 'dense edge ' + String(index)
}));
const denseNodes = denseEntries.concat([denseCore]);
const denseGraph = {
  nodes: denseNodes,
  edges: denseEdges,
  nodesByID: Object.fromEntries(denseNodes.map((node) => [node.id, node])),
  edgesByID: Object.fromEntries(denseEdges.map((edge) => [edge.id, edge])),
  incidentEdgesByNodeID: Object.fromEntries(denseNodes.map((node) => [node.id,
    denseEdges.filter((edge) => edge.sourceID === node.id || edge.targetID === node.id)]))
};
const denseLayout = Geometry.portLayoutRequirements(denseGraph);
assert(denseLayout.slotSpacing === 32 && denseLayout.minHeightByNodeID[denseCore.id] === 260,
  'eight endpoint hit targets reserve a 260px card with a non-overlapping 32px pitch');
const denseMeasurements = {
  width: 700, height: 700,
  nodesByID: Object.fromEntries(denseNodes.map((node, index) => [node.id, node === denseCore ?
    {left: 400, top: 120, width: 200, height: denseLayout.minHeightByNodeID[denseCore.id]} :
    {left: 20, top: 20 + index * 60, width: 180, height: 40}]))
};
const densePorts = Geometry.assignStablePorts(denseGraph, denseMeasurements);
const denseTargetPorts = densePorts.portsByNodeID[denseCore.id];
equal(denseTargetPorts.map((port) => port.oppositeNodeID), denseEntries.map((entry) => entry.id),
  'dense target ports follow opposite-node vertical order instead of arbitrary edge IDs');
assert(denseTargetPorts.every((port, index) => index === 0 ||
  port.offset - denseTargetPorts[index - 1].offset >= 32),
  'dense endpoint centers retain enough distance for their 29px pointer targets');

const orderSource = complete.nodesByID['core:alpha'];
const orderTarget = complete.nodesByID['core:beta'];
const orderEdges = ['edge:a', 'edge:Z', 'edge:ä'].map((id) => ({
  id, sourceID: orderSource.id, targetID: orderTarget.id, authority: 'exact', description: id
}));
const orderEdgesByID = Object.fromEntries(orderEdges.map((edge) => [edge.id, edge]));
const orderingGraph = {
  nodes: [orderSource, orderTarget], edges: orderEdges,
  nodesByID: {[orderSource.id]: orderSource, [orderTarget.id]: orderTarget},
  edgesByID: orderEdgesByID,
  incidentEdgesByNodeID: {[orderSource.id]: orderEdges, [orderTarget.id]: orderEdges}
};
const orderingMeasurements = {
  width: 500, height: 200,
  nodesByID: {
    [orderSource.id]: {left: 10, top: 20, width: 180, height: 48},
    [orderTarget.id]: {left: 300, top: 20, width: 180, height: 48}
  }
};
const expectedCodeUnitOrder = ['edge:Z', 'edge:a', 'edge:ä'];
const orderingPorts = Geometry.assignStablePorts(orderingGraph, orderingMeasurements);
equal(orderingPorts.portsByNodeID[orderSource.id].map((port) => port.edgeID), expectedCodeUnitOrder,
  'port assignment uses locale-independent code-unit ordering');
const orderingEmphasis = Interaction.deriveCanvasEmphasis(orderingGraph,
  Interaction.reduceCanvasInteraction(Interaction.createInteractionState(),
    {type: 'NODE_POINTER_ENTER', nodeID: orderSource.id}, orderingGraph));
equal(orderingEmphasis.emphasizedEdgeIDs, expectedCodeUnitOrder,
  'incident-edge emphasis uses the same locale-independent ordering');
`

const systemCanvasGeneratedReportBrowserRunner = `
(function () {
  'use strict';
  var result = document.getElementById('test-result');
  function assert(condition, message) {
    if (!condition) throw new Error(message);
  }
  function node(id) {
    return document.querySelector('[data-canvas-node="' + id + '"]');
  }
  function edge(sourceID, targetID) {
    return document.querySelector('g[data-canvas-edge-from="' + sourceID + '"][data-canvas-edge-to="' + targetID + '"]');
  }
  function port(nodeID, oppositeNodeID) {
    return document.querySelector('[data-canvas-edge-port][data-canvas-node-id="' + nodeID + '"]' +
      '[data-canvas-opposite-node-id="' + oppositeNodeID + '"]');
  }
  function delay(milliseconds) {
    return new Promise(function (resolve) { window.setTimeout(resolve, milliseconds); });
  }
  async function waitFor(predicate, timeout, message) {
    var deadline = Date.now() + timeout;
    while (!predicate()) {
      if (Date.now() >= deadline) {
        var fatal = document.getElementById('rm-fatal-message');
        throw new Error((typeof message === 'function' ? message() : message) + ': edges=' +
          String(document.querySelectorAll('g[data-canvas-edge-id]').length) +
          ' ports=' + String(document.querySelectorAll('[data-canvas-edge-port]').length) +
          ' fatal=' + String(fatal && fatal.textContent || 'none'));
      }
      await delay(20);
    }
  }
  function nodeBoundarySnapshot() {
    return Array.prototype.map.call(document.querySelectorAll('[data-canvas-node]'), function (item) {
      var style = getComputedStyle(item);
      return [
        item.getAttribute('data-canvas-node'),
        style.borderTopColor,
        style.borderRightColor,
        style.borderBottomColor,
        style.borderLeftColor,
        style.boxShadow
      ].join('|');
    }).join('\n');
  }
  function emphasisClassSnapshot(selector) {
    return Array.prototype.map.call(document.querySelectorAll(selector), function (item) {
      return item.className && typeof item.className.baseVal === 'string'
        ? item.className.baseVal
        : item.className;
    }).join('\n');
  }
  function edgePathSnapshot() {
    return Array.prototype.map.call(document.querySelectorAll('g[data-canvas-edge-id] path'), function (item) {
      return item.getAttribute('d');
    }).join('\n');
  }
  async function run() {
    var scriptIDs = [
      'rm-system-canvas-graph-js',
      'rm-system-canvas-interaction-js',
      'rm-system-canvas-geometry-js',
      'rm-system-canvas-renderer-js',
      'rm-report-app-js'
    ];
    scriptIDs.forEach(function (id, index) {
      var script = document.getElementById(id);
      assert(script, 'generated report embeds ' + id);
      if (index) {
        assert(document.getElementById(scriptIDs[index - 1]).compareDocumentPosition(script) & Node.DOCUMENT_POSITION_FOLLOWING,
          'generated report executes System canvas assets in ownership order');
      }
    });
    assert(window.RepomapSystemCanvasGraph && window.RepomapSystemCanvasInteraction &&
      window.RepomapSystemCanvasGeometry && window.RepomapSystemCanvasRenderer,
      'generated report executed every embedded System canvas layer');

    await delay(350);
    await waitFor(function () {
      return document.querySelectorAll('g[data-canvas-edge-id]').length === 2 &&
        document.querySelectorAll('[data-canvas-edge-port]').length === 4;
    }, 3000, 'generated System canvas did not finish its initial geometry');
    var host = document.querySelector('.rm-system-canvas-host');
    var canvas = document.querySelector('[data-system-canvas]');
    var fatal = document.getElementById('rm-fatal-message');
    assert(host && canvas && host.contains(canvas), 'the report shell mounted the real System canvas: ' +
      (fatal && fatal.textContent ? fatal.textContent : 'no fatal diagnostic'));
    assert(document.querySelectorAll('g[data-canvas-edge-id]').length === 2, 'generated semantic payload renders both exact edges');
    assert(document.querySelectorAll('[data-canvas-edge-port]').length === 4, 'stable endpoint controls were created eagerly');
    var entryFileGroups = document.querySelectorAll('.rm-canvas-entry-file');
    assert(entryFileGroups.length === 1 &&
      entryFileGroups[0].querySelector('.rm-canvas-entry-file__header h4').textContent === 'app/main.py',
      'entrypoints are grouped under their exact file path');
    var compactEntryMeta = entryFileGroups[0].querySelector('.rm-canvas-node--entry .rm-canvas-node__meta');
    assert(compactEntryMeta && compactEntryMeta.textContent === 'L1',
      'entrypoint cards keep only a compact line label instead of repeating the file path');
    var diagnostics = window.RepomapSystemCanvasRenderer.diagnostics(host);
    assert(diagnostics, 'the report shell owns a live renderer controller');
    var initialGeometryBuildCount = diagnostics.geometryBuildCount;
    assert(initialGeometryBuildCount > 0, 'initial browser geometry was built');

    var beta = node('core:beta');
    var gamma = node('core:gamma');
    var incident = edge('core:beta', 'core:gamma');
    var nonIncident = document.querySelector('g[data-canvas-edge-from^="entry:"][data-canvas-edge-to="core:alpha"]');
    assert(beta && gamma && incident && nonIncident, 'generated report exposes the intended graph');
    assert(beta.getAttribute('href').indexOf('#/program/responsibility/beta') >= 0,
      'shell-owned node navigation remains available');

    var initialNodeBoundaries = nodeBoundarySnapshot();
    beta.dispatchEvent(new PointerEvent('pointerover', {bubbles: true}));
    await delay(60);
    assert(canvas.getAttribute('data-canvas-highlight') === 'node', 'pointer hover activates node emphasis');
    assert(incident.classList.contains('rm-canvas-edge-group--related'), 'incident edge remains emphasized');
    assert(!nonIncident.classList.contains('rm-canvas-edge-group--related'), 'non-incident edge is not emphasized');
    assert(Number(getComputedStyle(nonIncident).opacity) < Number(getComputedStyle(incident).opacity), 'non-incident edge visibly fades');
    assert(nodeBoundarySnapshot() === initialNodeBoundaries,
      'relationship emphasis does not repaint card borders or halos');
    var betaPortRects = Array.prototype.map.call(
      document.querySelectorAll('[data-canvas-edge-port][data-canvas-node-id="core:beta"]'),
      function (item) { return item.getBoundingClientRect(); }
    ).sort(function (left, right) { return left.top - right.top; });
    assert(betaPortRects.length > 0 && betaPortRects.every(function (rect) {
      return Math.abs(rect.width - 29) <= 0.5 && Math.abs(rect.height - 29) <= 0.5;
    }), 'visible endpoint controls expose stable 29px pointer targets: ' + betaPortRects.map(function (rect) {
      return String(rect.width) + 'x' + String(rect.height);
    }).join(', '));
    assert(betaPortRects.every(function (rect, index) {
      return index === 0 || betaPortRects[index - 1].bottom <= rect.top;
    }), 'endpoint pointer targets on a dense card do not overlap');
    assert(Array.prototype.every.call(document.querySelectorAll('marker'), function (marker) {
      return marker.getAttribute('markerUnits') === 'userSpaceOnUse' &&
        !/[zZ]/.test(marker.querySelector('path').getAttribute('d'));
    }), 'arrowheads keep a fixed open shape when relation stroke emphasis changes');
    assert(Array.prototype.every.call(document.querySelectorAll('g[data-canvas-edge-id]'), function (group) {
      var path = group.querySelector('path');
      var targetPort = document.querySelector('[data-canvas-edge-port][data-canvas-edge-id="' +
        group.getAttribute('data-canvas-edge-id') + '"][data-canvas-node-id="' +
        group.getAttribute('data-canvas-edge-to') + '"]');
      if (!path || !targetPort || typeof path.getTotalLength !== 'function') return false;
      var tip = path.getPointAtLength(path.getTotalLength());
      var svgBounds = path.ownerSVGElement.getBoundingClientRect();
      var portBounds = targetPort.getBoundingClientRect();
      var scaleX = svgBounds.width / path.ownerSVGElement.viewBox.baseVal.width;
      var scaleY = svgBounds.height / path.ownerSVGElement.viewBox.baseVal.height;
      var tipX = svgBounds.left + tip.x * scaleX;
      var tipY = svgBounds.top + tip.y * scaleY;
      var portX = portBounds.left + portBounds.width / 2;
      var portY = portBounds.top + portBounds.height / 2;
      return Math.hypot(tipX - portX, tipY - portY) >= 9;
    }), 'every arrow tip remains visibly outside its circular endpoint control');

    var denseHost = document.createElement('div');
    denseHost.style.width = '1100px';
    document.body.appendChild(denseHost);
    var denseActivities = Array.from({length: 8}, function (_, index) {
      return {
        id: 'dense-' + String(index), name: 'dense.ts#entry' + String(index), kind: 'function',
        signature: '()', location: {path: 'dense.ts', line: index + 1, column: 1}
      };
    });
    var denseMemberships = Object.create(null);
    denseActivities.forEach(function (activity) { denseMemberships[activity.id] = ['dense-core']; });
    var denseGraph = window.RepomapSystemCanvasGraph.buildCanvasGraph({
      activities: denseActivities,
      blocks: [{id: 'dense-core', name: 'Dense core', purpose: 'Exercises dense endpoint layout.', symbols: ['dense-symbol']}],
      integrations: [], relations: [], blocksBySymbol: denseMemberships,
      groupsByBlock: {'dense-core': ['dense-group']}
    }, {activeBlockIDs: ['dense-core'], complete: true});
    // Chrome's finite virtual-time budget may stop issuing a second animation
    // frame after the asynchronously booted report has settled. Exercise the
    // renderer's supported timer scheduler for this additional synthetic mount.
    var requestAnimationFrame = window.requestAnimationFrame;
    window.requestAnimationFrame = undefined;
    var denseController = window.RepomapSystemCanvasRenderer.mountSystemCanvas(denseHost, denseGraph, {}, {});
    window.requestAnimationFrame = requestAnimationFrame;
    await waitFor(function () {
      return denseHost.querySelectorAll('[data-canvas-edge-port]').length === 16;
    }, 3000, function () { return 'dense System canvas did not finish geometry; dense edges=' +
      String(denseHost.querySelectorAll('g[data-canvas-edge-id]').length) + ' dense ports=' +
      String(denseHost.querySelectorAll('[data-canvas-edge-port]').length) + ' builds=' +
      String(denseController.diagnostics.geometryBuildCount); });
    var denseCore = denseHost.querySelector('[data-canvas-node="core:dense-core"]');
    denseCore.dispatchEvent(new PointerEvent('pointerover', {bubbles: true}));
    await delay(40);
    var denseRects = Array.prototype.map.call(
      denseHost.querySelectorAll('[data-canvas-edge-port][data-canvas-node-id="core:dense-core"]'),
      function (item) { return item.getBoundingClientRect(); }
    ).sort(function (left, right) { return left.top - right.top; });
    assert(denseCore.getBoundingClientRect().height >= 260 && denseRects.length === 8,
      'eight connections expand their core card before geometry is measured');
    assert(denseRects.every(function (rect, index) {
      return Math.abs(rect.width - 29) <= 0.5 && Math.abs(rect.height - 29) <= 0.5 &&
        (index === 0 || denseRects[index - 1].bottom <= rect.top);
    }), 'dense endpoint pointer targets remain individually reachable without overlap');
    denseController.unmount();
    denseHost.remove();

    var endpoint = port('core:beta', 'core:gamma');
    assert(endpoint && endpoint.classList.contains('rm-canvas-edge-port--related'), 'related endpoint button becomes visible');
    var betaChild = beta.querySelector('.rm-canvas-node__name') || beta;
    var nodeClassSnapshot = emphasisClassSnapshot('[data-canvas-node]');
    var edgeClassSnapshot = emphasisClassSnapshot('g[data-canvas-edge-id]');
    var pathSnapshot = edgePathSnapshot();
    endpoint.dispatchEvent(new PointerEvent('pointerover', {bubbles: true, relatedTarget: betaChild}));
    await delay(40);
    assert(canvas.getAttribute('data-canvas-highlight') === 'node',
      'moving from a card onto its endpoint keeps stable node emphasis');
    assert(emphasisClassSnapshot('[data-canvas-node]') === nodeClassSnapshot &&
      emphasisClassSnapshot('g[data-canvas-edge-id]') === edgeClassSnapshot,
      'endpoint pointer crossing does not churn node or edge emphasis classes');
    assert(nodeBoundarySnapshot() === initialNodeBoundaries && edgePathSnapshot() === pathSnapshot,
      'endpoint pointer crossing does not repaint card boundaries or replace edge paths');
    endpoint.dispatchEvent(new PointerEvent('pointerout', {bubbles: true, relatedTarget: betaChild}));
    await delay(40);
    assert(canvas.getAttribute('data-canvas-highlight') === 'node' &&
      emphasisClassSnapshot('[data-canvas-node]') === nodeClassSnapshot &&
      emphasisClassSnapshot('g[data-canvas-edge-id]') === edgeClassSnapshot,
      'moving from an endpoint back into its card keeps stable node emphasis');
    assert(diagnostics.geometryBuildCount === initialGeometryBuildCount,
      'card and endpoint pointer movement does not rebuild geometry');

    beta.dispatchEvent(new PointerEvent('pointerout', {bubbles: true, relatedTarget: document.body}));
    beta.focus();
    await delay(40);
    assert(document.activeElement === beta && canvas.getAttribute('data-canvas-highlight') === 'node',
      'keyboard focus activates the shell-mounted card');

    assert(endpoint.tabIndex === 0 && endpoint.getAttribute('aria-hidden') === 'false', 'visible endpoint is keyboard focusable');
    endpoint.focus();
    await delay(40);
    assert(document.activeElement === endpoint, 'endpoint receives keyboard focus');
    assert(incident.classList.contains('rm-canvas-edge-group--active'), 'endpoint focus highlights exactly its edge');
    assert(document.querySelectorAll('.rm-canvas-edge-group--active').length === 1, 'only one edge is active');
    assert(gamma.classList.contains('rm-canvas-node--edge-opposite'), 'opposite node is marked');
    assert(endpoint.querySelector('.rm-canvas-edge-port__label').textContent === 'Gamma', 'endpoint reveals the opposite node name');
    var hashBefore = window.location.hash;
    endpoint.click();
    await delay(60);
    assert(document.activeElement === gamma, 'endpoint click focuses and centers the opposite node');
    assert(window.location.hash === hashBefore, 'endpoint navigation does not change report routing');

    canvas.dispatchEvent(new KeyboardEvent('keydown', {key: 'Escape', bubbles: true}));
    await delay(40);
    assert(document.activeElement === gamma && window.location.hash === hashBefore,
      'Escape is handled without stealing keyboard focus or report navigation');
    gamma.blur();
    await delay(40);
    assert(!canvas.hasAttribute('data-canvas-highlight'), 'leaving the focused node clears transient emphasis');
    assert(diagnostics.geometryBuildCount === initialGeometryBuildCount,
      'hover, focus, endpoint click, and Escape do not rebuild geometry');

    await delay(600);
    var areaButtons = document.querySelectorAll('.rm-area-switcher__item');
    assert(areaButtons.length === 3 && areaButtons[0].querySelector('span').textContent === 'All',
      'generated report renders All before the shell-owned architecture grouping controls');
    assert(!document.querySelector('.rm-canvas-mode'),
      'generated report does not retain a separate complete-map toggle');
    var maximumScroll = Math.max(0, document.documentElement.scrollHeight - window.innerHeight);
    var requestedScroll = Math.min(160, maximumScroll);
    window.scrollTo(0, requestedScroll);
    await delay(40);
    var scrollBefore = window.scrollY;
    areaButtons[0].click();
    await delay(160);
    var activeAll = document.querySelector('.rm-area-switcher__item[aria-current="true"]');
    assert(activeAll && activeAll.querySelector('span').textContent === 'All',
      'All becomes the current complete-map selection after the shell re-renders');
    assert(document.activeElement === activeAll,
      'All regains focus after the shell re-renders; active element is ' +
        (document.activeElement ? document.activeElement.outerHTML : 'missing'));
    if (maximumScroll >= 80) {
      assert(Math.abs(window.scrollY - scrollBefore) <= 1,
        'All does not scroll away from the updated map: before=' + String(scrollBefore) +
          ' after=' + String(window.scrollY) + ' maximum=' + String(maximumScroll));
    }
    var alternateArea = document.querySelector('[data-area-group="alternate-flow"]');
    assert(alternateArea, 'generated report retains the exact alternate grouping control');
    alternateArea.click();
    await delay(160);
    assert(window.location.hash.indexOf('/responsibility/alpha') >= 0,
      'area switch keeps existing responsibility navigation behavior');
    var activeAlternate = document.querySelector('.rm-area-switcher__item[aria-current="true"]');
    assert(activeAlternate && activeAlternate.querySelector('span').textContent === 'Alternate flow' &&
      document.activeElement === activeAlternate,
      'area switch re-renders the requested grouping');
    if (maximumScroll >= 80) {
      assert(Math.abs(window.scrollY - scrollBefore) <= 1,
        'area switch does not scroll away from the updated map: before=' + String(scrollBefore) +
        ' after=' + String(window.scrollY) + ' maximum=' + String(maximumScroll));
    }
    result.setAttribute('data-status', 'pass');
    result.textContent = 'pass';
  }
  run().catch(function (error) {
    result.setAttribute('data-status', 'fail');
    result.textContent = error && error.stack ? error.stack : String(error);
  });
})();
`
