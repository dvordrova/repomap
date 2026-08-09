package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStudyInvestigationAssetRendersPathsMapOverlayAndPreparedCopy(t *testing.T) {
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
  "rm-overview", "rm-task-investigation", "rm-mechanisms", "rm-mechanism-detail",
  "rm-study-overview", "rm-study-detail", "rm-operate-detail", "rm-architecture",
  "rm-provenance", "rm-tabs",
].forEach((id) => { roots[id] = new Element("section"); roots[id].id = id; });
const workspace = new Element("div"); workspace.className = "rm-workspace";
const language = process.argv[3] || "en";
const paths = ["main.go", "start.go", "local.go", "offmap.go", "shared.go", "finish.go"];
function location(path, line, column) { return { path, line, column }; }
const nodes = [
  { id: "node-1", ordinal: 1, label: "fixture.main", declaration: location("main.go", 10, 2), component_ids: ["c1"] },
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
const cards = [
  {
    ordinal: 1, final_title: "How startup reaches work", final_question: "What calls what?",
    why_it_matters: "Follow the exact implementation path.", expected_learning: "Confirm each direct call.",
    badge: "source_backed", theme_kind: "user_journey",
    readings: [{ path: "main.go", line: 10, symbol: "gitlab.com/acme/fixture/pkg.(*Runner).main", role: "direct", what_to_look_for: "Start here." }],
    investigation: {
      version: 1, id: "study-investigation-theme-1", outcome: "mechanism",
      reading_ordinals: [1], mechanisms: [mechanism],
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
  document, location: { hash: "#/study/theme/1", search: "", hostname: "fixture.test", protocol: "file:", pathname: "/report.html" },
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
const detail = roots["rm-study-detail"];
const detailText = detail.textContent.replace(/\s+/g, " ").trim();
const detailNodes = detail.querySelectorAll(".rm-study-investigation__node").length;
const detailTransitions = detail.querySelectorAll(".rm-study-investigation__transition").length;
const detailSources = detail.querySelectorAll(".rm-study-investigation__source");
const mapButton = detail.querySelector(".rm-study-investigation__map");
if (!mapButton || typeof mapButton.onclick !== "function") throw new Error("Study map action missing: " + detailText);
const prepared = api.renderStudyInvestigation(cards[1]);
const preparedText = prepared.textContent.replace(/\s+/g, " ").trim();
mapButton.onclick();
Promise.resolve().then(() => {}).then(() => {
  const mapRoot = roots["rm-architecture"];
  const mapContext = mapRoot.querySelector(".rm-study-investigation-map");
  const returnBanner = mapRoot.querySelector(".rm-architecture-return");
  const returnButton = returnBanner && walk(returnBanner).find((node) => node.tagName === "BUTTON");
  const desktopMapContext = mapContext && mapContext.querySelector(".rm-study-investigation-map__desktop");
  const mobileMapContext = mapContext && mapContext.querySelector(".rm-study-investigation-map__mobile-path");
  const mapText = desktopMapContext ? desktopMapContext.textContent.replace(/\s+/g, " ").trim() : "";
  const mapNodes = desktopMapContext ? desktopMapContext.querySelectorAll(".rm-study-investigation__node").length : 0;
  const mapTransitions = desktopMapContext ? desktopMapContext.querySelectorAll(".rm-study-investigation__transition").length : 0;
  const sideRows = desktopMapContext ? desktopMapContext.querySelectorAll(".rm-study-investigation-map__side-row").length : 0;
  const mapSources = desktopMapContext ? desktopMapContext.querySelectorAll(".rm-study-investigation__source") : [];
  const mobileMapText = mobileMapContext ? mobileMapContext.textContent.replace(/\s+/g, " ").trim() : "";
  const mobileMapNodes = mobileMapContext ? mobileMapContext.querySelectorAll(".rm-study-investigation__node").length : 0;
  const mobileMapTransitions = mobileMapContext ? mobileMapContext.querySelectorAll(".rm-study-investigation__transition").length : 0;
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
  const entrypointsLens = mapRoot.querySelector('[data-map-lens="entrypoints"]');
  const landscapeLens = mapRoot.querySelector('[data-map-lens="landscape"]');
  if (!entrypointsLens || !landscapeLens) throw new Error("Map lens controls missing");
  entrypointsLens.onclick();
  const contextHiddenOutsideLandscape = !mapRoot.querySelector(".rm-study-investigation-map");
  const targetOutsideLandscape = api.workspaceStateSnapshot().mapTarget;
  landscapeLens.onclick();
  const contextRestoredInLandscape = !!mapRoot.querySelector(".rm-study-investigation-map");
  const targetRestoredInLandscape = api.workspaceStateSnapshot().mapTarget;
  if (returnButton && typeof returnButton.onclick === "function") returnButton.onclick();
  process.stdout.write(JSON.stringify({
    detailText, detailNodes, detailTransitions,
    detailSourceCount: detailSources.length,
    sourceHrefs: detailSources.map((node) => node.getAttribute("href") || ""),
    preparedText, mapText, mapNodes, mapTransitions, sideRows,
    mobileMapText, mobileMapNodes, mobileMapTransitions,
    mapSourceHrefs: mapSources.map((node) => node.getAttribute("href") || ""),
    mobileMapSourceHrefs: mobileMapSources.map((node) => node.getAttribute("href") || ""),
    mapContextFloatsInStage, contextHiddenOutsideLandscape, contextRestoredInLandscape,
    targetOutsideLandscape, targetRestoredInLandscape, overlays, lenses,
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
	if en.DetailNodes != 6 || en.DetailTransitions != 5 || en.DetailSourceCount != 11 {
		t.Fatalf("Study detail path = nodes %d transitions %d sources %d",
			en.DetailNodes, en.DetailTransitions, en.DetailSourceCount)
	}
	if en.MapNodes != 0 || en.MapTransitions != 0 || en.SideRows != 4 ||
		len(en.MapSourceHrefs) != 4 || !en.MapContextFloatsInStage {
		t.Fatalf("Map context = nodes %d transitions %d side rows %d sources %d floats=%t",
			en.MapNodes, en.MapTransitions, en.SideRows, len(en.MapSourceHrefs), en.MapContextFloatsInStage)
	}
	for _, href := range append(append([]string{}, en.SourceHrefs...), en.MapSourceHrefs...) {
		if !strings.Contains(href, "https://github.com/acme/fixture/blob/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/") {
			t.Fatalf("Study source is not pinned to the captured revision: %q", href)
		}
	}
	for _, required := range []string{
		"Connected code paths", "Direct call", "Goroutine start", "Deferred call",
		"Declaration · main.go:10:2", "Call site · main.go:30:5", "Show on map", "(*Runner).main",
	} {
		if !strings.Contains(en.DetailText, required) {
			t.Fatalf("English Study detail missing %q: %s", required, en.DetailText)
		}
	}
	if strings.Contains(en.DetailText, "gitlab.com/acme/fixture/pkg") {
		t.Fatalf("Study detail leaked a fully-qualified Go receiver: %s", en.DetailText)
	}
	if !strings.Contains(en.PreparedText, "Exact starting points are ready") ||
		!strings.Contains(en.MapText, "Exact transitions without a map arrow") ||
		!strings.Contains(en.MapText, "Both endpoints are inside “Worker”.") ||
		!strings.Contains(en.MapText, "no single joined map area") ||
		!strings.Contains(en.MapText, "several exact map areas") {
		t.Fatalf("English prepared/map copy incomplete: prepared=%s map=%s", en.PreparedText, en.MapText)
	}
	for _, duplicated := range []string{"Study path on the map", "The complete path stays visible here", "Connected code paths"} {
		if strings.Contains(en.MapText, duplicated) {
			t.Fatalf("Map context duplicated the Study detail path %q: %s", duplicated, en.MapText)
		}
	}
	for _, leaked := range []string{"SECRET MODEL EXPLANATION", "SECRET STATUS", "SECRET REF", "prepared_investigation", "provider_ref", "status_code"} {
		if strings.Contains(en.DetailText+en.MapText+en.PreparedText, leaked) {
			t.Fatalf("private/status field %q leaked into Study HTML", leaked)
		}
	}
	if strings.Join(en.Overlays, ",") != "study-investigation-theme-1-mechanism-1,,study-investigation-theme-1-mechanism-1" ||
		strings.Join(en.Lenses, ",") != "landscape,entrypoints,landscape" ||
		!en.ContextHiddenOutsideLandscape || !en.ContextRestoredInLandscape ||
		en.TargetOutsideLandscape != en.HistoryTarget || en.TargetRestoredInLandscape != en.HistoryTarget ||
		en.MapHash != "#/map" || en.HistoryTarget.Kind != "study_mechanism" ||
		en.HistoryTarget.ThemeOrdinal != 1 || en.HistoryTarget.InvestigationID != "study-investigation-theme-1" ||
		en.HistoryTarget.MechanismID != "study-investigation-theme-1-mechanism-1" {
		t.Fatalf("transient map selection = overlays %#v lenses %#v hidden=%t restored=%t outside=%#v restoredTarget=%#v hash %q target %#v",
			en.Overlays, en.Lenses, en.ContextHiddenOutsideLandscape, en.ContextRestoredInLandscape,
			en.TargetOutsideLandscape, en.TargetRestoredInLandscape, en.MapHash, en.HistoryTarget)
	}
	if en.Returned.View != "study" || en.Returned.ThemeCardOrdinal != 1 || en.ReturnHash != "#/study/theme/1" {
		t.Fatalf("return to Study = state %#v hash %q", en.Returned, en.ReturnHash)
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
	if mobile.MobileMapNodes != 6 || mobile.MobileMapTransitions != 5 ||
		len(mobile.MobileMapSourceHrefs) != 11 ||
		!strings.Contains(mobile.MobileMapText, "Connected code paths") {
		t.Fatalf("mobile Map lost the complete ordered mechanism: nodes=%d transitions=%d sources=%d text=%q",
			mobile.MobileMapNodes, mobile.MobileMapTransitions, len(mobile.MobileMapSourceHrefs), mobile.MobileMapText)
	}

	ru := run("ru", "desktop")
	for _, required := range []string{
		"Связанные пути по коду", "Прямой вызов", "Запуск goroutine", "Отложенный вызов",
		"Объявление · main.go:10:2", "Место вызова · main.go:30:5", "Показать на карте",
		"Точные места в коде найдены", "Точные переходы без стрелки на карте",
	} {
		if !strings.Contains(ru.DetailText+ru.PreparedText+ru.MapText, required) {
			t.Fatalf("Russian Study UI missing %q: detail=%s prepared=%s map=%s",
				required, ru.DetailText, ru.PreparedText, ru.MapText)
		}
	}
	if ru.DetailNodes != en.DetailNodes || ru.MapNodes != en.MapNodes || ru.SideRows != en.SideRows ||
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
		`.rm-study-investigation__path { flex-direction: column; overflow: visible;`,
		`.rm-study-investigation__node, .rm-study-investigation__transition { box-sizing: border-box; flex-basis: auto; width: 100%; }`,
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
	DetailNodes                           int                         `json:"detailNodes"`
	DetailTransitions                     int                         `json:"detailTransitions"`
	DetailSourceCount                     int                         `json:"detailSourceCount"`
	SourceHrefs                           []string                    `json:"sourceHrefs"`
	PreparedText                          string                      `json:"preparedText"`
	MapText                               string                      `json:"mapText"`
	MapNodes                              int                         `json:"mapNodes"`
	MapTransitions                        int                         `json:"mapTransitions"`
	SideRows                              int                         `json:"sideRows"`
	MapSourceHrefs                        []string                    `json:"mapSourceHrefs"`
	MobileMapText                         string                      `json:"mobileMapText"`
	MobileMapNodes                        int                         `json:"mobileMapNodes"`
	MobileMapTransitions                  int                         `json:"mobileMapTransitions"`
	MobileMapSourceHrefs                  []string                    `json:"mobileMapSourceHrefs"`
	MapContextFloatsInStage               bool                        `json:"mapContextFloatsInStage"`
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
