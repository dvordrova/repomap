package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStudyInvestigationAssetGroupsSiblingMechanismsIntoSourceTraceAndKeepsMapOverlay(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	asset, err := filepath.Abs(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs");
const vm = require("vm");
class Element {
  constructor(tag) {
    this.tagName = String(tag || "div").toUpperCase(); this.children = []; this.attributes = {};
    this.className = ""; this._text = ""; this.hidden = false; this.style = {}; this.dataset = {};
    this.parentNode = null; this.id = ""; this.listeners = {};
    this.classList = {
      add: (...names) => { const values = new Set(String(this.className).split(/\s+/).filter(Boolean)); names.forEach((name) => values.add(name)); this.className = Array.from(values).join(" "); },
      remove: (...names) => { const removed = new Set(names); this.className = String(this.className).split(/\s+/).filter((name) => name && !removed.has(name)).join(" "); },
      toggle: (name, force) => { const values = new Set(String(this.className).split(/\s+/).filter(Boolean)); const active = force === undefined ? !values.has(name) : !!force; if (active) values.add(name); else values.delete(name); this.className = Array.from(values).join(" "); return active; },
      contains: (name) => String(this.className).split(/\s+/).includes(name),
    };
  }
  get childNodes() { return this.children; }
  get textContent() { return this._text + this.children.map((child) => child.textContent).join(""); }
  set textContent(value) { this._text = value == null ? "" : String(value); this.children = []; }
  appendChild(child) { if (child) { child.parentNode = this; this.children.push(child); } return child; }
  append(...children) { children.forEach((child) => this.appendChild(child)); }
  prepend(...children) { children.reverse().forEach((child) => { if (child) { child.parentNode = this; this.children.unshift(child); } }); }
  replaceChildren(...children) { this.children = []; this._text = ""; children.forEach((child) => this.appendChild(child)); }
  remove() { if (this.parentNode) this.parentNode.children = this.parentNode.children.filter((child) => child !== this); }
  setAttribute(name, value) { this.attributes[name] = String(value); if (name === "id") this.id = String(value); }
  getAttribute(name) { return this.attributes[name] == null ? null : String(this.attributes[name]); }
  removeAttribute(name) { delete this.attributes[name]; }
  querySelector(selector) { return walk(this).find((node) => matches(node, selector)) || null; }
  querySelectorAll(selector) { return walk(this).filter((node) => matches(node, selector)); }
  addEventListener(type, listener) { (this.listeners[type] ||= []).push(listener); }
  dispatchEvent(event) { (this.listeners[event && event.type] || []).forEach((listener) => listener.call(this, event)); }
  contains(node) { return this === node || walk(this).includes(node); }
  focus() {}
  scrollIntoView() {}
}
function walk(node, out) { out = out || []; (node.children || []).forEach((child) => { out.push(child); walk(child, out); }); return out; }
function matches(node, selector) {
  if (selector[0] === ".") return String(node.className || "").split(/\s+/).includes(selector.slice(1));
  const attr = selector.match(/^\[([^=\]]+)(?:=["']?([^"'\]]+)["']?)?\]$/);
  return !!(attr && Object.prototype.hasOwnProperty.call(node.attributes || {}, attr[1]) && (attr[2] == null || node.attributes[attr[1]] === attr[2]));
}
const roots = {};
[
  "rm-task-investigation", "rm-operate-detail", "rm-architecture", "rm-provenance", "rm-tabs",
].forEach((id) => { roots[id] = new Element("section"); roots[id].id = id; });
const workspace = new Element("div"); workspace.className = "rm-workspace";
const language = process.argv[3] || "en";
const paths = ["main.go", "start.go", "local.go", "offmap.go", "shared.go", "finish.go", "sibling.go", "tail.go"];
function location(path, line, column) { return { path, line, column }; }
const nodes = [
  { id: "node-1", ordinal: 1, label: "telebot.v3 · NewBot", declaration: location("main.go", 10, 2), component_ids: ["c1"] },
  { id: "node-2", ordinal: 2, label: "fixture.start", declaration: location("start.go", 11, 2), component_ids: ["c2"] },
  { id: "node-3", ordinal: 3, label: "fixture.local", declaration: location("local.go", 12, 2), component_ids: ["c2"] },
  { id: "node-4", ordinal: 4, label: "fixture.offmap", declaration: location("offmap.go", 13, 2), component_ids: [] },
  { id: "node-5", ordinal: 5, label: "fixture.shared", declaration: location("shared.go", 14, 2), component_ids: ["c1", "c2"] },
  { id: "node-6", ordinal: 6, label: "fixture.finish", declaration: location("finish.go", 15, 2), component_ids: ["c1"] },
];
const invocations = ["synchronous", "goroutine", "deferred", "synchronous", "synchronous"];
const edges = invocations.map((invocation, index) => ({
  id: "edge-" + (index + 1), ordinal: index + 1,
  from_node_id: nodes[index].id, to_node_id: nodes[index + 1].id,
  invocation, witness_count: index + 1,
  callsite: location(paths[index], 30 + index, 5),
}));
const mechanism = {
  id: "study-investigation-theme-1-mechanism-1", ordinal: 1,
  reading_ordinals: [1], nodes, edges,
  model_prose: "SECRET MODEL EXPLANATION",
};
const fourEdgeMechanism = {
  id: "study-investigation-theme-3-mechanism-1", ordinal: 1,
  reading_ordinals: [1], nodes: nodes.slice(0, 5), edges: edges.slice(0, 4),
};
const siblingNodes = [
  { id: "sibling-root", ordinal: 1, label: "telebot.v3 · NewBot", declaration: location("main.go", 10, 2), component_ids: ["c1"] },
  { id: "sibling-middle", ordinal: 2, label: "fixture.sibling", declaration: location("sibling.go", 20, 2), component_ids: ["c2"] },
  { id: "sibling-tail", ordinal: 3, label: "fixture.tail", declaration: location("tail.go", 21, 2), component_ids: ["c1"] },
];
const siblingMechanism = {
  id: "study-investigation-theme-1-mechanism-2", ordinal: 2,
  reading_ordinals: [1], nodes: siblingNodes,
  edges: [
    { id: "sibling-edge-1", ordinal: 1, from_node_id: "sibling-root", to_node_id: "sibling-middle", invocation: "synchronous", witness_count: 1, callsite: location("main.go", 40, 5) },
    { id: "sibling-edge-2", ordinal: 2, from_node_id: "sibling-middle", to_node_id: "sibling-tail", invocation: "synchronous", witness_count: 1, callsite: location("sibling.go", 50, 5) },
  ],
};
const cards = [
  {
    ordinal: 1, final_title: "How startup reaches work", final_question: "What calls what?",
    why_it_matters: "Follow the exact implementation path.", expected_learning: "Confirm each direct call.",
    badge: "source_backed", theme_kind: "user_journey",
    readings: [{ path: "main.go", line: 10, symbol: "gitlab.com/acme/fixture/pkg.(*Runner).main", role: "direct", what_to_look_for: "Start here." }],
    investigation: {
      version: 1, id: "study-investigation-theme-1", outcome: "mechanism",
      reading_ordinals: [1], mechanisms: [siblingMechanism, mechanism],
      status_code: "SECRET STATUS", provider_ref: "SECRET REF",
    },
  },
  {
    ordinal: 2, final_title: "Prepared theme", final_question: "Where should I begin?",
    badge: "source_backed", theme_kind: "learning",
    readings: [{ path: "finish.go", line: 15, symbol: "fixture.finish", role: "direct" }],
    investigation: {
      version: 1, id: "study-investigation-theme-2", outcome: "prepared_investigation",
      reading_ordinals: [1], mechanisms: [], status_code: "SECRET PREPARED STATUS",
    },
  },
  {
    ordinal: 3, final_title: "Four-edge trace", final_question: "Does every exact edge stay visible?",
    badge: "source_backed", theme_kind: "learning",
    readings: [{ path: "main.go", line: 10, symbol: "fixture.main", role: "direct" }],
    investigation: {
      version: 1, id: "study-investigation-theme-3", outcome: "mechanism",
      reading_ordinals: [1], mechanisms: [fourEdgeMechanism],
    },
  },
];
const report = {
  repo_name: "fixture", report_language: language,
  user_mechanisms: [], user_topics: [], user_sources: [], openable_paths: paths, source_ids: {},
  github_source_links: { repository_url: "https://github.com/acme/fixture", revision: "a".repeat(40) },
  repository_atlas: { version: 1, units: [], entities: [], evidence: [], relations: [] },
  atlas_study: { themes: { cards } },
  architecture_canvas: {
    version: 15, components: [{ id: "c1", name: "Entry" }, { id: "c2", name: "Worker" }],
    subsystems: [{ id: "s1", name: "Application", component_ids: ["c1", "c2"] }],
    structural_edges: [], behavior_anchors: [], flows: [], flow_edges: [], surfaces: [], entry_handoff_groups: [],
  },
};
const document = {
  createElement: (tag) => new Element(tag),
  createElementNS: (_, tag) => new Element(tag),
  createTextNode: (value) => { const node = new Element("#text"); node.textContent = String(value); return node; },
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
    if (roots[id]) return roots[id];
    return Object.values(roots).flatMap((root) => [root].concat(walk(root))).find((node) => node.id === id) || null;
  },
  querySelector(selector) { return selector === ".rm-workspace" ? workspace : null; },
  querySelectorAll(selector) {
    return Object.values(roots).flatMap((root) => walk(root)).filter((node) => matches(node, selector));
  },
  body: new Element("body"), documentElement: { lang: language }, activeElement: null,
};
const history = {
  state: null,
  pushState(state, _, hash) { this.state = state; window.location.hash = hash; },
  replaceState(state, _, hash) { this.state = state; window.location.hash = hash; },
  back() {},
};
const mobileMedia = {
  matches: process.argv[4] === "mobile", listeners: [],
  addEventListener(type, listener) { if (type === "change") this.listeners.push(listener); },
  addListener(listener) { this.listeners.push(listener); },
  set(value) { this.matches = !!value; this.listeners.forEach((listener) => listener({ matches: this.matches })); },
};
const window = {
  document, location: { hash: "#study-theme-1", search: "", hostname: "fixture.test", protocol: "file:", pathname: "/report.html" },
  history, __REPOMAP_WORKSPACE_TEST__: {}, __REPOMAP_LAYOUT_TEST__: {},
  addEventListener() {}, removeEventListener() {}, open() {}, scrollTo() {},
  matchMedia() { return mobileMedia; }, setTimeout, clearTimeout,
};
document.activeElement = document.body; window.Element = Element;
const context = { window, document, Element, URLSearchParams, Set, Map, AbortController, Promise, setTimeout, clearTimeout };
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), context);
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "architecture_canvas.js"), "utf8"), context);
const realCanvas = window.RepomapArchitectureCanvas;
const overlays = [];
const lenses = [];
window.RepomapArchitectureCanvas = {
  projectArchitectureLens: realCanvas.projectArchitectureLens,
  projectEntrypointHandoffOverlay: realCanvas.projectEntrypointHandoffOverlay,
  projectStudyMechanismOverlay: realCanvas.projectStudyMechanismOverlay,
  mount() { return {
    ready: Promise.resolve(), destroy() {}, openComponent() {}, openTrace() {}, openFlowStep() {}, openSurface() {},
    setLens(value) { lenses.push(value); }, setStudyMechanismOverlay(value) { overlays.push(value && value.id || ""); return true; },
    clearStudyMechanismOverlay() { overlays.push(""); },
  }; },
};
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), context);
const api = window.__REPOMAP_WORKSPACE_TEST__;
api.restoreWorkspaceFromRoute({ replace: true });
const detail = document.getElementById("study-theme-1");
if (!detail) throw new Error("inline Study theme missing");
const detailText = detail.textContent.replace(/\s+/g, " ").trim();
const detailSummaryCount = walk(detail).filter((node) => node.tagName === "SUMMARY").length;
const detailGroups = detail.querySelectorAll(".rm-study-investigation__group").length;
const detailRoots = detail.querySelectorAll(".rm-study-investigation__root").length;
const detailMechanisms = detail.querySelectorAll(".rm-study-investigation__mechanism");
const detailRows = detail.querySelectorAll(".rm-study-investigation__row");
const detailSources = detail.querySelectorAll(".rm-study-investigation__source");
const firstRow = detailRows[0];
const firstRowSources = firstRow ? firstRow.querySelectorAll(".rm-study-investigation__source") : [];
const mapButton = detail.querySelector(".rm-study-investigation__map");
if (!mapButton || typeof mapButton.onclick !== "function") throw new Error("Study map action missing: " + detailText);
const prepared = api.renderStudyInvestigation(cards[1]);
const preparedAbsent = prepared === null;
const preparedText = prepared ? prepared.textContent.replace(/\s+/g, " ").trim() : "";
const fourEdge = api.renderStudyInvestigation(cards[2]);
const fourEdgeRows = fourEdge.querySelectorAll(".rm-study-investigation__row");
const fourEdgeCallsites = fourEdge.querySelectorAll(".rm-study-investigation__line");
const fourEdgeCallees = fourEdge.querySelectorAll(".rm-study-investigation__callee");
const fourEdgeRoots = fourEdge.querySelectorAll(".rm-study-investigation__root-declaration");
mapButton.onclick();
Promise.resolve().then(() => {}).then(() => {
  const mapRoot = roots["rm-architecture"];
  const mapContext = mapRoot.querySelector(".rm-study-investigation-map");
  const returnBanner = mapRoot.querySelector(".rm-architecture-return");
  const desktopMapContext = mapContext && mapContext.querySelector(".rm-study-investigation-map__desktop");
  const mobileMapContext = mapContext && mapContext.querySelector(".rm-study-investigation-map__mobile-path");
  const mapText = desktopMapContext ? desktopMapContext.textContent.replace(/\s+/g, " ").trim() : "";
  const mapNodes = desktopMapContext ? desktopMapContext.querySelectorAll(".rm-study-investigation__node").length : 0;
  const mapTransitions = desktopMapContext ? desktopMapContext.querySelectorAll(".rm-study-investigation__transition").length : 0;
  const sideRows = desktopMapContext ? desktopMapContext.querySelectorAll(".rm-study-investigation-map__side-row").length : 0;
  const mapSources = desktopMapContext ? desktopMapContext.querySelectorAll(".rm-study-investigation__source") : [];
  const mobileMapText = mobileMapContext ? mobileMapContext.textContent.replace(/\s+/g, " ").trim() : "";
  const mobileMapRoots = mobileMapContext ? mobileMapContext.querySelectorAll(".rm-study-investigation__root").length : 0;
  const mobileMapMechanisms = mobileMapContext ? mobileMapContext.querySelectorAll(".rm-study-investigation__mechanism").length : 0;
  const mobileMapRows = mobileMapContext ? mobileMapContext.querySelectorAll(".rm-study-investigation__row").length : 0;
  const mobileMapSources = mobileMapContext ? mobileMapContext.querySelectorAll(".rm-study-investigation__source") : [];
  const mapStage = mapRoot.querySelector(".rm-architecture-canvas-stage");
  const mapContextFloatsInStage = !!(mapStage && mapContext && mapContext.parentNode && mapContext.parentNode.parentNode === mapStage);
  const mapHash = window.location.hash;
  const historyTarget = window.history.state && window.history.state.mapTarget;
  const componentDisclosure = mapRoot.querySelector(".rm-architecture-list-disclosure");
  const componentSummary = componentDisclosure && componentDisclosure.querySelector(".rm-architecture-disclosure__summary");
  const componentDisclosureInitiallyOpen = !!(componentDisclosure && componentDisclosure.open);
  let componentDisclosureOpensOnMobile = componentDisclosureInitiallyOpen;
  let componentDisclosureClosesOnDesktop = !componentDisclosureInitiallyOpen;
  let componentDisclosurePreservesUserClose = false;
  if (componentDisclosure && componentSummary && !mobileMedia.matches) {
    mobileMedia.set(true);
    componentDisclosureOpensOnMobile = !!componentDisclosure.open;
    mobileMedia.set(false);
    componentDisclosureClosesOnDesktop = !componentDisclosure.open;
    mobileMedia.set(true);
    componentSummary.dispatchEvent({ type: "click" });
    componentDisclosure.open = false;
    mobileMedia.set(false);
    mobileMedia.set(true);
    componentDisclosurePreservesUserClose = !componentDisclosure.open;
  }
  process.stdout.write(JSON.stringify({
    detailText, detailTag: detail.tagName, detailOpen: !!detail.open, detailSummaryCount,
    detailGroups, detailRoots, detailRows: detailRows.length,
    detailMechanismOrder: detailMechanisms.map((node) => node.getAttribute("data-study-mechanism-id") || ""),
    firstRowSourceCount: firstRowSources.length,
    firstRowSourceHrefs: firstRowSources.map((node) => node.getAttribute("href") || ""),
    fourEdgeRows: fourEdgeRows.length,
    fourEdgeCallsites: fourEdgeCallsites.length,
    fourEdgeCallsiteHrefs: fourEdgeCallsites.map((node) => node.getAttribute("href") || ""),
    fourEdgeFunctionLabels: fourEdgeRoots.length + fourEdgeCallees.length,
    mapButtonCount: detail.querySelectorAll(".rm-study-investigation__map").length,
    detailSourceCount: detailSources.length,
    sourceHrefs: detailSources.map((node) => node.getAttribute("href") || ""),
    preparedAbsent, preparedText, mapText, mapNodes, mapTransitions, sideRows,
    mobileMapText, mobileMapRoots, mobileMapMechanisms, mobileMapRows,
    mapSourceHrefs: mapSources.map((node) => node.getAttribute("href") || ""),
    mobileMapSourceHrefs: mobileMapSources.map((node) => node.getAttribute("href") || ""),
    mapContextFloatsInStage, returnBannerCount: returnBanner ? 1 : 0, overlays, lenses,
    componentDisclosureInitiallyOpen, componentDisclosureOpensOnMobile,
    componentDisclosureClosesOnDesktop, componentDisclosurePreservesUserClose,
    mapHash, historyTarget,
    returned: api.workspaceStateSnapshot(), returnHash: window.location.hash,
  }));
}).catch((error) => { process.stderr.write(String(error && error.stack || error)); process.exit(2); });
`
	runnerPath := filepath.Join(t.TempDir(), "study-investigation-asset-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}

	run := func(language string, viewport string) studyInvestigationAssetResult {
		t.Helper()
		output, err := exec.Command(node, runnerPath, asset, language, viewport).CombinedOutput()
		if err != nil {
			t.Fatalf("run Study investigation asset (%s): %v\n%s", language, err, output)
		}
		var got studyInvestigationAssetResult
		if err := json.Unmarshal(output, &got); err != nil {
			t.Fatalf("decode Study investigation asset (%s): %v\n%s", language, err, output)
		}
		return got
	}

	en := run("en", "desktop")
	if en.DetailGroups != 1 || en.DetailRoots != 1 || en.DetailRows != 7 ||
		en.DetailSourceCount != 16 || en.FirstRowSourceCount != 2 || en.MapButtonCount != 2 {
		t.Fatalf("Study source trace = groups %d roots %d rows %d sources %d first-row sources %d map actions %d",
			en.DetailGroups, en.DetailRoots, en.DetailRows, en.DetailSourceCount,
			en.FirstRowSourceCount, en.MapButtonCount)
	}
	if strings.Join(en.DetailMechanismOrder, ",") !=
		"study-investigation-theme-1-mechanism-1,study-investigation-theme-1-mechanism-2" {
		t.Fatalf("sibling mechanisms are not in first-callsite source order: %#v", en.DetailMechanismOrder)
	}
	for index, exact := range []string{"/main.go#L30", "/start.go#L11"} {
		if index >= len(en.FirstRowSourceHrefs) || !strings.Contains(en.FirstRowSourceHrefs[index], exact) {
			t.Fatalf("source-trace row action %d = %q, want exact %q in callsite/callee order",
				index, en.FirstRowSourceHrefs[index], exact)
		}
	}
	if en.FourEdgeRows != 4 || en.FourEdgeCallsites != 4 || en.FourEdgeFunctionLabels != 5 {
		t.Fatalf("four-edge source trace = rows %d callsites %d function labels %d, want 4/4/5",
			en.FourEdgeRows, en.FourEdgeCallsites, en.FourEdgeFunctionLabels)
	}
	for index, exact := range []string{"/main.go#L30", "/start.go#L31", "/local.go#L32", "/offmap.go#L33"} {
		if index >= len(en.FourEdgeCallsiteHrefs) || !strings.Contains(en.FourEdgeCallsiteHrefs[index], exact) {
			t.Fatalf("four-edge source trace callsite %d = %q, want exact %q",
				index, en.FourEdgeCallsiteHrefs[index], exact)
		}
	}
	if en.MapNodes != 0 || en.MapTransitions != 0 || en.SideRows != 4 ||
		len(en.MapSourceHrefs) != 4 || !en.MapContextFloatsInStage {
		t.Fatalf("Map context = nodes %d transitions %d side rows %d sources %d floats=%t",
			en.MapNodes, en.MapTransitions, en.SideRows, len(en.MapSourceHrefs), en.MapContextFloatsInStage)
	}
	for _, href := range append(append(append([]string{}, en.SourceHrefs...), en.FirstRowSourceHrefs...), en.MapSourceHrefs...) {
		if !strings.Contains(href, "https://github.com/acme/fixture/blob/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/") {
			t.Fatalf("Study source is not pinned to the captured revision: %q", href)
		}
	}
	for _, required := range []string{
		"Source trace", "Exact direct calls in source order", "not a runtime trace",
		"Goroutine start", "Deferred call", "main.go", "10 NewBot()", "30", "start()", "local()",
		"Show on map",
	} {
		if !strings.Contains(en.DetailText, required) {
			t.Fatalf("English Study detail missing %q: %s", required, en.DetailText)
		}
	}
	for _, removed := range []string{"Path 1", "Path 2", "Direct call", "static witness", "Declaration ·"} {
		if strings.Contains(en.DetailText, removed) {
			t.Fatalf("Study source trace retained removed chrome %q: %s", removed, en.DetailText)
		}
	}
	if strings.Contains(en.DetailText, "gitlab.com/acme/fixture/pkg") {
		t.Fatalf("Study detail leaked a fully-qualified Go receiver: %s", en.DetailText)
	}
	if strings.Contains(en.DetailText+en.MapText, "v3 ·") {
		t.Fatalf("Study trace leaked the package disambiguation prefix: detail=%s map=%s", en.DetailText, en.MapText)
	}
	if !en.PreparedAbsent || en.PreparedText != "" ||
		!strings.Contains(en.MapText, "Exact transitions without a map arrow") ||
		!strings.Contains(en.MapText, "Both endpoints are inside “Worker”.") ||
		!strings.Contains(en.MapText, "no single joined map area") ||
		!strings.Contains(en.MapText, "several exact map areas") {
		t.Fatalf("prepared investigation leaked empty product chrome or map copy is incomplete: absent=%t prepared=%q map=%s",
			en.PreparedAbsent, en.PreparedText, en.MapText)
	}
	for _, duplicated := range []string{"Study path on the map", "The complete path stays visible here", "Source trace"} {
		if strings.Contains(en.MapText, duplicated) {
			t.Fatalf("Map context duplicated the Study detail path %q: %s", duplicated, en.MapText)
		}
	}
	for _, leaked := range []string{"SECRET MODEL EXPLANATION", "SECRET STATUS", "SECRET REF", "prepared_investigation", "provider_ref", "status_code"} {
		if strings.Contains(en.DetailText+en.MapText+en.PreparedText, leaked) {
			t.Fatalf("private/status field %q leaked into Study HTML", leaked)
		}
	}
	if en.DetailTag != "DETAILS" || !en.DetailOpen || en.DetailSummaryCount != 1 || en.ReturnBannerCount != 0 {
		t.Fatalf("inline Study disclosure = tag %q open %t summaries %d return banners %d",
			en.DetailTag, en.DetailOpen, en.DetailSummaryCount, en.ReturnBannerCount)
	}
	if !strings.Contains(strings.Join(en.Overlays, ","), "study-investigation-theme-1-mechanism-1") ||
		strings.Contains(strings.Join(en.Lenses, ","), "entrypoints") ||
		en.MapHash != "#canvas" || en.HistoryTarget.Kind != "study_mechanism" ||
		en.HistoryTarget.ThemeOrdinal != 1 || en.HistoryTarget.InvestigationID != "study-investigation-theme-1" ||
		en.HistoryTarget.MechanismID != "study-investigation-theme-1-mechanism-1" {
		t.Fatalf("inline map selection = overlays %#v lenses %#v hash %q target %#v",
			en.Overlays, en.Lenses, en.MapHash, en.HistoryTarget)
	}
	if en.Returned.View != "map" || en.Returned.ThemeCardOrdinal != 0 || en.ReturnHash != "#canvas" {
		t.Fatalf("mechanism stays on the unified target page = state %#v hash %q", en.Returned, en.ReturnHash)
	}
	if en.ComponentDisclosureInitiallyOpen || !en.ComponentDisclosureOpensOnMobile ||
		!en.ComponentDisclosureClosesOnDesktop || !en.ComponentDisclosurePreservesUserClose {
		t.Fatalf("responsive component disclosure = initial %t mobile %t desktop %t preserved %t",
			en.ComponentDisclosureInitiallyOpen, en.ComponentDisclosureOpensOnMobile,
			en.ComponentDisclosureClosesOnDesktop, en.ComponentDisclosurePreservesUserClose)
	}
	mobile := run("en", "mobile")
	if !mobile.ComponentDisclosureInitiallyOpen {
		t.Fatal("component disclosure is closed on an initially mobile Map")
	}
	if mobile.MobileMapRoots != 1 || mobile.MobileMapMechanisms != 1 || mobile.MobileMapRows != 5 ||
		len(mobile.MobileMapSourceHrefs) != 12 ||
		!strings.Contains(mobile.MobileMapText, "Source trace") {
		t.Fatalf("mobile Map lost the complete source trace: roots=%d mechanisms=%d rows=%d sources=%d text=%q",
			mobile.MobileMapRoots, mobile.MobileMapMechanisms, mobile.MobileMapRows,
			len(mobile.MobileMapSourceHrefs), mobile.MobileMapText)
	}

	ru := run("ru", "desktop")
	for _, required := range []string{
		"След по коду", "Точные прямые вызовы в порядке исходников", "Это не трассировка выполнения",
		"Запуск goroutine", "Отложенный вызов", "10 NewBot()", "30", "start()", "local()", "Показать на карте",
		"Точные переходы без стрелки на карте",
	} {
		if !strings.Contains(ru.DetailText+ru.PreparedText+ru.MapText, required) {
			t.Fatalf("Russian Study UI missing %q: detail=%s prepared=%s map=%s",
				required, ru.DetailText, ru.PreparedText, ru.MapText)
		}
	}
	if ru.DetailGroups != en.DetailGroups || ru.DetailRoots != en.DetailRoots ||
		ru.DetailRows != en.DetailRows || ru.MapNodes != en.MapNodes || ru.SideRows != en.SideRows ||
		len(ru.MapSourceHrefs) != len(en.MapSourceHrefs) || ru.MapContextFloatsInStage != en.MapContextFloatsInStage {
		t.Fatalf("EN/RU Study structure diverged: EN %#v RU %#v", en, ru)
	}
}

func TestStudyInvestigationAssetsKeepTheCompletePathOnMobile(t *testing.T) {
	css, err := os.ReadFile(filepath.Join("templates", "style.css"))
	if err != nil {
		t.Fatal(err)
	}
	canvasCSS, err := os.ReadFile(filepath.Join("templates", "architecture_canvas.css"))
	if err != nil {
		t.Fatal(err)
	}
	canvasJS, err := os.ReadFile(filepath.Join("templates", "architecture_canvas.js"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		`.rm-study-investigation__row { align-items: baseline; display: flex; flex-wrap: wrap;`,
		`.rm-study-investigation__root { align-items: baseline; background: #f8fafc; display: flex; flex-wrap: wrap;`,
		`.rm-study-investigation__line { color: #64748b; flex: 0 0 auto;`,
		`.rm-study-investigation__map { background: transparent; border: 0;`,
		`.rm-architecture-canvas-stage { min-width: 0; position: relative; }`,
		`position: absolute; right: .75rem; top: .75rem;`,
		`.rm-study-investigation-map--mobile-only, .rm-study-investigation-map__mobile-path { display: none; }`,
		`.rm-study-investigation-map, .rm-study-investigation-map--mobile-only { display: grid; margin: .65rem 0 0; max-height: none; overflow: visible; position: static; width: auto; }`,
		`.rm-study-investigation-map__desktop { display: none; }`,
		`.rm-study-investigation-map__mobile-path { display: grid; gap: .5rem; }`,
	} {
		if !strings.Contains(string(css), required) {
			t.Errorf("mobile Study path CSS is missing %q", required)
		}
	}
	for _, required := range []string{
		`.rm-arch__edge--study-mechanism .rm-arch__edge-visible`,
		`data-study-mechanism-overlay="true"`,
		`.rm-arch__is-study-mechanism-participant`,
	} {
		if !strings.Contains(string(canvasCSS), required) {
			t.Errorf("Study Canvas overlay CSS is missing %q", required)
		}
	}
	for _, required := range []string{
		`projectStudyMechanismOverlay: studyMechanismOverlayProjection`,
		`setStudyMechanismOverlay: (mechanism) => app.setStudyMechanismOverlay(mechanism)`,
		`clearStudyMechanismOverlay: () => app.clearStudyMechanismOverlay()`,
	} {
		if !strings.Contains(string(canvasJS), required) {
			t.Errorf("Study Canvas overlay API is missing %q", required)
		}
	}
}

type studyInvestigationAssetResult struct {
	DetailText                            string                      `json:"detailText"`
	DetailTag                             string                      `json:"detailTag"`
	DetailOpen                            bool                        `json:"detailOpen"`
	DetailSummaryCount                    int                         `json:"detailSummaryCount"`
	DetailGroups                          int                         `json:"detailGroups"`
	DetailRoots                           int                         `json:"detailRoots"`
	DetailRows                            int                         `json:"detailRows"`
	DetailMechanismOrder                  []string                    `json:"detailMechanismOrder"`
	FirstRowSourceCount                   int                         `json:"firstRowSourceCount"`
	FirstRowSourceHrefs                   []string                    `json:"firstRowSourceHrefs"`
	FourEdgeRows                          int                         `json:"fourEdgeRows"`
	FourEdgeCallsites                     int                         `json:"fourEdgeCallsites"`
	FourEdgeCallsiteHrefs                 []string                    `json:"fourEdgeCallsiteHrefs"`
	FourEdgeFunctionLabels                int                         `json:"fourEdgeFunctionLabels"`
	MapButtonCount                        int                         `json:"mapButtonCount"`
	DetailSourceCount                     int                         `json:"detailSourceCount"`
	SourceHrefs                           []string                    `json:"sourceHrefs"`
	PreparedAbsent                        bool                        `json:"preparedAbsent"`
	PreparedText                          string                      `json:"preparedText"`
	MapText                               string                      `json:"mapText"`
	MapNodes                              int                         `json:"mapNodes"`
	MapTransitions                        int                         `json:"mapTransitions"`
	SideRows                              int                         `json:"sideRows"`
	MapSourceHrefs                        []string                    `json:"mapSourceHrefs"`
	MobileMapText                         string                      `json:"mobileMapText"`
	MobileMapRoots                        int                         `json:"mobileMapRoots"`
	MobileMapMechanisms                   int                         `json:"mobileMapMechanisms"`
	MobileMapRows                         int                         `json:"mobileMapRows"`
	MobileMapSourceHrefs                  []string                    `json:"mobileMapSourceHrefs"`
	MapContextFloatsInStage               bool                        `json:"mapContextFloatsInStage"`
	ReturnBannerCount                     int                         `json:"returnBannerCount"`
	ContextHiddenOutsideLandscape         bool                        `json:"contextHiddenOutsideLandscape"`
	ContextRestoredInLandscape            bool                        `json:"contextRestoredInLandscape"`
	TargetOutsideLandscape                studyInvestigationMapTarget `json:"targetOutsideLandscape"`
	TargetRestoredInLandscape             studyInvestigationMapTarget `json:"targetRestoredInLandscape"`
	Overlays                              []string                    `json:"overlays"`
	Lenses                                []string                    `json:"lenses"`
	ComponentDisclosureInitiallyOpen      bool                        `json:"componentDisclosureInitiallyOpen"`
	ComponentDisclosureOpensOnMobile      bool                        `json:"componentDisclosureOpensOnMobile"`
	ComponentDisclosureClosesOnDesktop    bool                        `json:"componentDisclosureClosesOnDesktop"`
	ComponentDisclosurePreservesUserClose bool                        `json:"componentDisclosurePreservesUserClose"`
	MapHash                               string                      `json:"mapHash"`
	HistoryTarget                         studyInvestigationMapTarget `json:"historyTarget"`
	Returned                              struct {
		View             string `json:"view"`
		ThemeCardOrdinal int    `json:"themeCardOrdinal"`
	} `json:"returned"`
	ReturnHash string `json:"returnHash"`
}

type studyInvestigationMapTarget struct {
	Kind            string `json:"kind"`
	ThemeOrdinal    int    `json:"theme_ordinal"`
	InvestigationID string `json:"investigation_id"`
	MechanismID     string `json:"mechanism_id"`
}
