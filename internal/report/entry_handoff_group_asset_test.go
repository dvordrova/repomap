package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Executable targets open on a neutral landscape. Entrypoints is an explicit
// first-class mode; choosing it reveals source-backed cards without selecting
// a handoff group until the reader asks to explore one entry.
func TestEntrypointModeStartsNeutralAndExploresOneExactEntry(t *testing.T) {
	if ArchitectureCanvasVersion != 15 || EntrypointHandoffGroupVersion != 2 {
		t.Fatalf("fixture requires Canvas15/group v2, got %d/%d", ArchitectureCanvasVersion, EntrypointHandoffGroupVersion)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	asset := filepath.Join("templates", "script.js")
	runner := `
const fs = require("fs");
const vm = require("vm");
function Element(tag) {
  this.tagName = String(tag).toUpperCase(); this._text = ""; this.children = [];
  this.attributes = {}; this.hidden = false; this.className = "";
  const self = this;
  this.classList = {
    add(value) { const parts = String(self.className || "").split(/\s+/).filter(Boolean); if (!parts.includes(value)) parts.push(value); self.className = parts.join(" "); },
    remove(value) { self.className = String(self.className || "").split(/\s+/).filter((part) => part && part !== value).join(" "); },
    toggle(value, active) { if (active) this.add(value); else this.remove(value); },
  };
}
Object.defineProperty(Element.prototype, "childNodes", { get() { return this.children; } });
Object.defineProperty(Element.prototype, "textContent", {
  get() { return this._text + this.children.map((child) => child.textContent).join(""); },
  set(value) { this._text = value == null ? "" : String(value); },
});
Element.prototype.setAttribute = function (name, value) { this.attributes[name] = String(value); if (name === "id") this.id = String(value); };
Element.prototype.getAttribute = function (name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; };
Element.prototype.removeAttribute = function (name) { delete this.attributes[name]; };
Element.prototype.appendChild = function (child) { child.parentNode = this; this.children.push(child); return child; };
Element.prototype.append = function (...children) { children.forEach((child) => this.appendChild(child)); };
Element.prototype.replaceChildren = function (...children) { this.children = []; children.forEach((child) => this.appendChild(child)); };
Element.prototype.remove = function () { if (this.parentNode) this.parentNode.children = this.parentNode.children.filter((child) => child !== this); };
Element.prototype.prepend = function (...children) { children.reverse().forEach((child) => { child.parentNode = this; this.children.unshift(child); }); };
function walk(node, out) { out = out || []; (node.children || []).forEach((child) => { out.push(child); walk(child, out); }); return out; }
function matches(node, selector) {
  if (selector[0] === ".") return String(node.className || "").split(/\s+/).includes(selector.slice(1));
  const attr = selector.match(/^\[([^=\]]+)(?:=["']?([^"'\]]+)["']?)?\]$/);
  if (attr) return Object.prototype.hasOwnProperty.call(node.attributes || {}, attr[1]) && (attr[2] == null || node.attributes[attr[1]] === attr[2]);
  return false;
}
Element.prototype.querySelector = function (selector) { return walk(this).find((node) => matches(node, selector)) || null; };
Element.prototype.querySelectorAll = function (selector) { return walk(this).filter((node) => matches(node, selector)); };
const roots = {
  "rm-architecture": new Element("section"),
  "rm-tabs": new Element("div"),
};
const workspace = new Element("div");
function transition(label, path, line, column, target, componentIDs) {
  return {
    claim_kind: target ? "direct_static_call" : "process_entry",
    support_mode: "resolved_static", label, path, line, column,
    component_ids: componentIDs || [],
    symbol: label, evidence: "go_ssa secret diagnostic detail",
    evidence_ref: { kind: "architecture_entry_handoff", id: "entry-handoff-secret" },
    provenance: { provider: "go_ssa", version: "surface-ssa-v12", operation: "collect_entry_direct_static_handoff" },
    scenario: "go:linux/amd64:tags=", limitation: "raw repeated limitation",
    ordering: "resolved_path_order", target,
  };
}
function group(id, entrySymbol, entryPath, componentIDs, handoffs) {
  const entry = transition(entrySymbol || "process entry", entryPath, 10, 6, null, [componentIDs[0]]);
  entry.symbol = entrySymbol;
  return {
    version: 2, id, component_ids: componentIDs,
    entry,
    entry_handoffs: Array.isArray(handoffs) ? handoffs : [handoffs],
    frontier: { ordering: "not_established", unresolved: ["continuation beyond first hop"], limitation: "raw frontier limitation" },
  };
}
const sameComponentHandoffs = Array.from({ length: 13 }, (_, index) =>
  transition(
    "fixture.local" + index, "main.go", 30 + index, 4,
    { label: "fixture.local" + index, path: "main.go", line: 200 + index, column: 6 },
    ["c1"]
  )
);
const groupA = group("entry-group-a", "fixture.main", "main.go", ["c1", "c2"], [
  transition("fixture.service.Start", "main.go", 15, 9,
    { label: "fixture.service.Start", path: "service.go", line: 20, column: 4 }, ["c2"]),
].concat(sameComponentHandoffs));
const groupB = group("entry-group-b", "fixture.main", "worker.go", ["c3"],
  transition("fixture.client.Send", "worker.go", 25, 8,
    { label: "fixture.client.Send", path: "client.go", line: 30, column: 3 }, []));
// Zero/plural entry ownership must fall back to the exact path leaf, never a
// guessed component from group.component_ids.
groupB.entry.component_ids = [];
const report = {
  repo_name: "fixture", report_language: process.argv[3] || "en",
  analysis_target: {
    version: 2, ref: "fixture-target", kind: "executable_package", module_dir: ".",
    package_dir: ".", package_path: "fixture", roots: [{ path: "main.go", line: 10 }],
  },
  github_source_links: { repository_url: "https://github.com/acme/fixture", revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" },
  user_mechanisms: [], user_topics: [],
  // Mixed snippets must not suppress exact pinned source actions.
  user_sources: [{ path: "other.go", start_line: 1, end_line: 1, lines: ["package fixture"] }],
  openable_paths: ["client.go", "cmd/zero/main.go", "main.go", "other.go", "service.go", "worker.go"], source_ids: {},
  architecture_canvas: {
    version: 15, local_remainder_component_id: "component-r",
    components: [
      { id: "c1", name: "Entry", participating_surface_ids: ["surface-main"] },
      { id: "c2", name: "Service" }, { id: "c3", name: "Client" },
    ],
    subsystems: [], behavior_anchors: [], flows: [], structural_edges: [],
    surfaces: [
      { id: "surface-main", surface_role: "entry_surface", kind: "process_entry", name: "main", participating_component_ids: ["c1"],
        evidence: [{ path: "main.go", line: 10, column: 6 }] },
      { id: "surface-zero", surface_role: "entry_surface", kind: "process_entry", name: "zero.main", participating_component_ids: [],
        evidence: [{ path: "cmd/zero/main.go", line: 7, column: 6 }] },
    ],
    entry_handoff_groups: [groupA, groupB],
  },
  entry_call: { version: 1, families: [{
    caller_label: "fixture.main", callee_label: "fixture.service.Start", witness_count: 1,
    root_declaration: { path: "main.go", line: 10, column: 6 },
    callsites: [{ path: "main.go", line: 15, column: 9 }],
  }] },
  repository_atlas: { version: 1, units: [], entities: [], observations: [], evidence: [], relations: [] },
};
if (process.argv[4] === "moby") {
  const processLocation = { path: "cmd/dockerd/main.go", line: 16, column: 6 };
  function mobySurface(id, kind, name, path, line, column, componentIDs) {
    return {
      id, kind, name, surface_role: "entry_surface", participating_component_ids: componentIDs || [],
      evidence: [{ path, line, column }, processLocation, { path: "daemon/command/docker.go", line: 32, column: 9 }],
    };
  }
  const mobyGroup = group("entry-group-dockerd", "github.com/moby/moby/v2/cmd/dockerd.main",
    "cmd/dockerd/main.go", ["c1", "c2"], transition(
      "github.com/moby/moby/v2/daemon/command.NewDaemonRunner", "cmd/dockerd/main.go", 30, 35,
      { label: "github.com/moby/moby/v2/daemon/command.NewDaemonRunner", path: "daemon/command/docker.go", line: 94, column: 6 },
      ["c2"]
    ));
  mobyGroup.entry.line = 16;
  mobyGroup.entry.column = 6;
  report.repo_name = "github.com/moby/moby";
  report.analysis_target.package_dir = "cmd/dockerd";
  report.analysis_target.package_path = "github.com/moby/moby/v2/cmd/dockerd";
  report.analysis_target.roots = [processLocation];
  report.openable_paths = [
    "cmd/dockerd/main.go", "daemon/command/daemon.go", "daemon/command/docker.go",
    "daemon/command/metrics.go", "daemon/daemon_unix.go", "daemon/internal/metrics/plugin_unix.go",
    "daemon/libnetwork/diagnostic/server.go", "daemon/server/server.go",
  ];
  report.architecture_canvas.surfaces = [
    mobySurface("trigger-root", "http_route", "/", "daemon/libnetwork/diagnostic/server.go", 35, 14, ["c2"]),
    mobySurface("trigger-metrics", "http_route", "/metrics", "daemon/command/metrics.go", 26, 12, []),
    mobySurface("trigger-metrics-server", "http_server", "HTTP server", "daemon/command/metrics.go", 33, 22, []),
    mobySurface("trigger-ready", "http_route", "/ready", "daemon/libnetwork/diagnostic/server.go", 37, 14, ["c2"]),
    mobySurface("trigger-api", "http_route", "/v{version:[0-9.]+}/{path:.*}", "daemon/server/server.go", 146, 14, []),
    mobySurface("trigger-diagnostic-server", "http_server", "HTTP server", "daemon/libnetwork/diagnostic/server.go", 97, 31, ["c2"]),
    mobySurface("trigger-daemon-server", "http_server", "HTTP server", "daemon/command/daemon.go", 377, 30, ["c1", "c2"]),
    mobySurface("trigger-plugin-server", "http_server", "HTTP server", "daemon/internal/metrics/plugin_unix.go", 125, 22, ["c2"]),
    mobySurface("trigger-plugin-metrics", "http_route", "/metrics", "daemon/internal/metrics/plugin_unix.go", 118, 12, ["c2"]),
    mobySurface("trigger-help", "http_route", "/help", "daemon/libnetwork/diagnostic/server.go", 36, 14, ["c2"]),
    { id: "trigger-main", kind: "process_entry", name: "main", surface_role: "entry_surface",
      participating_component_ids: ["c1"], evidence: [processLocation] },
    { id: "trigger-worker", kind: "worker", name: "background sync", surface_role: "runtime_activity",
      participating_component_ids: ["c2"], evidence: [{ path: "daemon/command/daemon.go", line: 400, column: 2 }, processLocation] },
  ];
  report.architecture_canvas.entry_handoff_groups = [mobyGroup];
  function triggerValue(kind, text, known) {
    return { kind, text, known: known !== false, candidates: [] };
  }
  function mobyTrigger(id, kind, identity, handler, registration, handlerLocation) {
    return {
      id, kind, surface_role: "entry_surface", provisional_id: false,
      identity, handler, registration_site: registration,
      server_start_site: kind === "http_server" ? registration : null,
      handler_location: handlerLocation || null,
      process_entrypoint: { name: "main", location: processLocation },
    };
  }
  report.discovered_surfaces = { triggers: [
    mobyTrigger("trigger-root", "http_route", {path:triggerValue("constant", "/", true)},
      triggerValue("function", "github.com/moby/moby/v2/daemon/libnetwork/diagnostic.notImplemented", true),
      {path:"daemon/libnetwork/diagnostic/server.go",line:35,column:14}),
    // Ambiguous stable IDs fail closed even when one candidate has the same
    // kind, label and location as the Canvas entry.
    mobyTrigger("trigger-root", "cli_command", {path:triggerValue("command_segment", "poison", true)},
      triggerValue("function", "fixture.poison", true),
      {path:"daemon/libnetwork/diagnostic/server.go",line:35,column:14}),
    mobyTrigger("trigger-metrics", "http_route", {path:triggerValue("constant", "/metrics", true)},
      {kind:"unknown",text:"result of github.com/docker/go-metrics.Handler",known:false,candidates:[]},
      {path:"daemon/command/metrics.go",line:26,column:12}),
    mobyTrigger("trigger-ready", "http_route", {method:"get",path:triggerValue("constant", "/ready", true)},
      triggerValue("function", "github.com/moby/moby/v2/daemon/libnetwork/diagnostic.ready", true),
      {path:"daemon/libnetwork/diagnostic/server.go",line:37,column:14}),
    mobyTrigger("trigger-api", "http_route", {path:triggerValue("constant", "/v{version:[0-9.]+}/{path:.*}", true)},
      triggerValue("function", "github.com/moby/moby/v2/daemon/server.CreateMux$1", true),
      {path:"daemon/server/server.go",line:146,column:14}),
    mobyTrigger("trigger-help", "http_route", {path:triggerValue("constant", "/help", true)},
      triggerValue("function", "github.com/moby/moby/v2/daemon/libnetwork/diagnostic.help", true),
      {path:"daemon/libnetwork/diagnostic/server.go",line:36,column:14}),
    mobyTrigger("trigger-metrics-server", "http_server", {name:"HTTP server",path:triggerValue("allocation", "listener", true)},
      triggerValue("allocation", "**net/http.ServeMux@daemon/command/metrics.go:25:2", true),
      {path:"daemon/command/metrics.go",line:33,column:22}),
    mobyTrigger("trigger-diagnostic-server", "http_server", {name:"HTTP server",path:triggerValue("allocation", "listener", true)},
      triggerValue("allocation", "server.Handler", true),
      {path:"daemon/libnetwork/diagnostic/server.go",line:97,column:31}),
    mobyTrigger("trigger-daemon-server", "http_server", {name:"HTTP server",path:triggerValue("allocation", "listener", true)},
      triggerValue("field", "server.Handler", true),
      {path:"daemon/command/daemon.go",line:377,column:30}),
    mobyTrigger("trigger-plugin-server", "http_server", {name:"HTTP server",path:triggerValue("allocation", "listener", true)},
      triggerValue("allocation", "server.Handler", true),
      {path:"daemon/internal/metrics/plugin_unix.go",line:125,column:22}),
    mobyTrigger("trigger-main", "process_entry", {name:"main",path:triggerValue("declaration", "cmd/dockerd/main.go", true)},
      triggerValue("declaration", "github.com/moby/moby/v2/cmd/dockerd.main", true), processLocation),
    // Similar text without the Canvas stable ID is never rendered.
    mobyTrigger("trigger-poison", "http_route", {path:triggerValue("constant", "/ready", true)},
      triggerValue("function", "fixture.poison", true),
      {path:"daemon/libnetwork/diagnostic/server.go",line:37,column:14}),
  ] };
  report.entry_call.families = Array.from({ length: 6 }, (_, index) => ({
    caller_label: index === 0 ? "dockerd · main" : "command · newDaemonCommand",
    callee_label: "command · family" + String(index + 1), witness_count: 1,
    root_declaration: processLocation,
    callsites: [{ path: index === 0 ? "cmd/dockerd/main.go" : "daemon/command/docker.go", line: 30 + index, column: 9 }],
  }));
  report.entry_call.version = 2;
  report.entry_call.surfaces = [{
    id: "model-surface-000000000000000000000099", kind: "http_route", role: "entry_surface",
    form: "direct_call", site: { path: "daemon/libnetwork/diagnostic/server.go", line: 37, column: 14 },
    method: { kind: "token", text: "GET", location: { path: "daemon/libnetwork/diagnostic/server.go", line: 37, column: 14 } },
    path: { kind: "string", text: "/ready", location: { path: "daemon/libnetwork/diagnostic/server.go", line: 37, column: 14 } },
    handler: { kind: "callable", text: "github.com/moby/moby/v2/daemon/libnetwork/diagnostic.ready", location: { path: "daemon/libnetwork/diagnostic/server.go", line: 88, column: 6 } },
    origin: "model_assisted", state: "exact_registration", runtime_reachability: "not_established",
  }];
  report.entry_call.surface_coverage = {
    considered_candidates: 1, advertised_candidates: 1, omitted_candidates: 0,
    considered_facts: 3, advertised_facts: 3, omitted_facts: 0,
    unsafe_facts_excluded: 0, unreachable_candidates_excluded: 0,
    selected_proposals: 1, rejected_proposals: 0,
  };
}
if (process.argv[4] === "legacy-cli") {
  const processLocation = { path: "cmd/restic/main.go", line: 20, column: 6 };
  const cliSurfaces = [];
  const cliTriggers = [];
  const cliFiles = [];
  for (let index = 0; index < 37; index++) {
    const ordinal = String(index).padStart(2, "0");
    const id = "legacy-cli-" + ordinal;
    const exactRoot = index === 36;
    const name = exactRoot ? "restic" : index === 0 ? "ls" : "command-" + ordinal;
    const path = "cmd/restic/cmd_" + ordinal + ".go";
    const registration = { path: "cmd/restic/main.go", line: 76 + index, column: 16 };
    // One same-location record proves the UI deduplicates identical exact
    // registration/handler actions without collapsing distinct locations.
    const handlerLocation = index === 1 ? registration : { path, line: 65, column: 9 };
    cliFiles.push(path);
    cliSurfaces.push({
      id, kind: "cli_command", name: "wrong canvas label " + ordinal,
      surface_role: "entry_surface", participating_component_ids: ["c1"],
      evidence: [registration, handlerLocation],
    });
    cliTriggers.push({
      id, kind: "cli_command", surface_role: "entry_surface", provisional_id: !exactRoot,
      identity: {
        path: { kind: "command_segment", text: name, known: exactRoot, candidates: exactRoot ? [] : [name] },
        name,
      },
      handler: {
        kind: "function",
        text: "github.com/restic/restic/cmd/restic.func_literal@" + path + ":65:9",
        known: true,
        candidates: [],
      },
      registration_site: registration,
      handler_location: handlerLocation,
      process_entrypoint: { name: "main", location: processLocation },
    });
  }
  report.repo_name = "github.com/restic/restic";
  report.openable_paths = ["cmd/restic/main.go"].concat(cliFiles);
  report.architecture_canvas.surfaces = [{
    id: "legacy-main", kind: "process_entry", name: "main", surface_role: "entry_surface",
    participating_component_ids: ["c1"], evidence: [processLocation],
  }].concat(cliSurfaces);
  report.architecture_canvas.entry_handoff_groups = [];
  report.entry_call.families = [];
  report.discovered_surfaces = {
    triggers: cliTriggers.concat([{
      id: "legacy-cli-poison", kind: "cli_command", surface_role: "entry_surface",
      identity: { path: { kind: "command_segment", text: "poison", known: true, candidates: [] }, name: "poison" },
      handler: { kind: "function", text: "fixture.poison", known: true, candidates: [] },
      registration_site: { path: "cmd/restic/main.go", line: 999, column: 1 },
    }]),
  };
}
if (String(process.argv[4] || "").indexOf("model-assisted") === 0) {
  const processLocation = { path: "main.go", line: 10, column: 6 };
  const localSite = { path: "routes.go", line: 20, column: 2 };
  const localHandler = { path: "handlers.go", line: 40, column: 6 };
  const commandIdentities = [
    "backup [flags]", "repair", "index", "key", "add", "get [key]", "health", "txn",
  ];
  function exactValue(kind, text, location) {
    return { kind, text, location };
  }
  function modelRoute(id, method, routePath, site, handlerName, handlerLocation) {
    return {
      id, kind: "http_route", role: "entry_surface", form: "direct_call", site,
      method: exactValue("token", method, site), path: exactValue("string", routePath, site),
      handler: exactValue("callable", handlerName, handlerLocation),
      origin: "model_assisted", state: "exact_registration", runtime_reachability: "not_established",
    };
  }
  function modelCommand(index) {
    const ordinal = String(index).padStart(2, "0");
    const site = { path: "cmd/serve/cmd_" + ordinal + ".go", line: 20 + index, column: 2 };
    const handler = { path: site.path, line: 60 + index, column: 6 };
    return {
      id: "model-surface-" + String(index + 10).padStart(24, "0"),
      kind: "cli_command", role: "descriptor", form: "keyed_composite", site,
      identity: exactValue("string", commandIdentities[index], site),
      handler: index % 2 === 0 ? exactValue("callable", "fixture.run" + ordinal, handler) : null,
      origin: "model_assisted", state: "declared_descriptor", runtime_reachability: "not_established",
    };
  }
  const localTrigger = {
    id: "local-route", kind: "http_route", surface_role: "entry_surface", provisional_id: false,
    resolution: "exact", identity: { method: "GET", path: { kind: "constant", text: "/local", known: true, candidates: [] } },
    handler: { kind: "function", text: "fixture.local", known: true, candidates: [] },
    registration_site: localSite, handler_location: localHandler,
  };
  const localDescriptor = {
    id: "local-cli-descriptor", kind: "cli_command", surface_role: "descriptor", provisional_id: false,
    resolution: "exact", identity: {
      name: "backup [flags]",
      path: { kind: "command_segment", text: "backup [flags]", known: true, candidates: [] },
    },
    handler: { kind: "function", text: "fixture.localBackup", known: true, candidates: [] },
    descriptor_site: { path: "cmd/serve/cmd_00.go", line: 20, column: 2 },
    handler_location: { path: "cmd/serve/cmd_00.go", line: 90, column: 6 },
  };
  const unmatchedDescriptor = {
    id: "local-cli-unmatched", kind: "cli_command", surface_role: "descriptor", provisional_id: false,
    resolution: "exact", identity: {
      name: "hidden-local-only",
      path: { kind: "command_segment", text: "hidden-local-only", known: true, candidates: [] },
    },
    descriptor_site: { path: "cmd/serve/cmd_00.go", line: 21, column: 2 },
  };
  report.openable_paths = ["main.go", "routes.go", "handlers.go"];
  for (let index = 0; index < 8; index++) report.openable_paths.push("cmd/serve/cmd_" + String(index).padStart(2, "0") + ".go");
  report.architecture_canvas.surfaces = [
    { id: "model-main", kind: "process_entry", name: "main", surface_role: "entry_surface", participating_component_ids: ["c1"], evidence: [processLocation] },
    { id: "local-route", kind: "http_route", name: "wrong local label", surface_role: "entry_surface", participating_component_ids: ["c2"], evidence: [localSite, localHandler] },
  ];
  report.architecture_canvas.entry_handoff_groups = [];
  report.discovered_surfaces = { triggers: [localTrigger, localDescriptor, unmatchedDescriptor] };
  report.entry_call = {
    version: 2, families: [],
    surfaces: [
      // Equivalent exact local TriggerRecord wins this duplicate key.
      modelRoute("model-surface-000000000000000000000001", "GET", "/local", localSite, "fixture.local", localHandler),
      modelRoute("model-surface-000000000000000000000003", "GET", "/",
        { path: "main.go", line: 98, column: 2 }, "main.root", { path: "main.go", line: 98, column: 17 }),
      modelRoute("model-surface-000000000000000000000002", "GET", "/ws",
        { path: "main.go", line: 208, column: 2 }, "main.hello", { path: "main.go", line: 143, column: 6 }),
    ].concat(Array.from({ length: 8 }, (_, index) => modelCommand(index))),
    surface_coverage: {
      considered_candidates: 18, advertised_candidates: 15, omitted_candidates: 3,
      considered_facts: 60, advertised_facts: 54, omitted_facts: 6,
      unsafe_facts_excluded: 2, unreachable_candidates_excluded: 1,
      selected_proposals: 11, rejected_proposals: 2,
    },
  };
  if (String(process.argv[4] || "").endsWith("-shuffled")) {
    report.architecture_canvas.surfaces.reverse();
    report.discovered_surfaces.triggers.reverse();
    report.entry_call.surfaces.reverse();
  }
}
const window = {
  location: { hash: "#canvas", host: "fixture.test", pathname: "/index.html", search: "" },
  history: { state: null, pushState(state, _, hash) { this.state = state; window.location.hash = hash; }, replaceState(state, _, hash) { this.state = state; window.location.hash = hash; }, back() {} },
  __REPOMAP_WORKSPACE_TEST__: {}, __REPOMAP_LAYOUT_TEST__: {}, addEventListener() {}, open() {}, scrollTo() {},
};
const document = {
  createElement(tag) { return new Element(tag); },
  createTextNode(value) { const node = new Element("#text"); node.textContent = String(value); return node; },
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
    if (roots[id]) return roots[id];
    return Object.values(roots).flatMap((root) => [root].concat(walk(root))).find((node) => node.id === id) || null;
  },
  querySelector(selector) {
    if (selector === ".rm-workspace") return workspace;
    return Object.values(roots).flatMap((root) => walk(root)).find((node) => matches(node, selector)) || null;
  },
  querySelectorAll(selector) {
    return Object.values(roots).flatMap((root) => walk(root)).filter((node) => matches(node, selector));
  },
};
document.documentElement = { lang: report.report_language };
window.document = document;
const context = { window, document, Element, URLSearchParams, Set, Map, AbortController, Promise };
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), context);
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "architecture_canvas.js"), "utf8"), context);
const realCanvas = window.RepomapArchitectureCanvas;
const selectedGroups = [];
const selectedLenses = [];
window.RepomapArchitectureCanvas = {
  projectArchitectureLens: realCanvas.projectArchitectureLens,
  projectEntrypointHandoffOverlay: realCanvas.projectEntrypointHandoffOverlay,
  mount() { return {
    ready: Promise.resolve(), destroy() {}, openComponent() {}, openTrace() {}, openFlowStep() {}, openSurface() {},
    clearStudyMechanismOverlay() {}, setLens(id) { selectedLenses.push(id); },
    selectEntrypointHandoffGroup(id) { selectedGroups.push(id); },
  }; },
};
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), context);
const api = window.__REPOMAP_WORKSPACE_TEST__;
api.renderWorkspaceTabs();
api.restoreWorkspaceFromRoute({ replace: true });
Promise.resolve().then(() => Promise.resolve()).then(() => {
  const root = roots["rm-architecture"];
  const targetLinks = roots["rm-tabs"].querySelectorAll(".rm-target-link");
  const lensHost = document.getElementById("rm-map-lens-objects");
  const modeContext = root.querySelector(".rm-map-mode-context");
  const modes = walk(root).filter((node) => node.attributes["data-map-mode"]);
  const initialModeIDs = modes.map((node) => node.attributes["data-map-mode"]);
  const initialModeStates = modes.map((node) => node.getAttribute("aria-pressed"));
  const initialLensHostMode = lensHost && lensHost.getAttribute("data-lens") || "";
  const initialContextHidden = !!(modeContext && modeContext.hidden);
  const initialEntryCardCount = root.querySelectorAll(".rm-map-entry-card").length;
  const initialSelectedGroups = selectedGroups.slice();
  const initialSelectedLenses = selectedLenses.slice();
  const entryMode = modes.find((node) => node.attributes["data-map-mode"] === "entrypoints");
  if (entryMode && typeof entryMode.onclick === "function") entryMode.onclick();
  const cards = root.querySelectorAll(".rm-map-entry-card");
  const explore = root.querySelectorAll(".rm-map-entry-card__explore");
  const sourceActions = lensHost ? lensHost.querySelectorAll(".rm-source-action-link") : [];
  const familyDetails = lensHost ? lensHost.querySelectorAll(".rm-map-entry-families") : [];
  const familyRows = lensHost ? lensHost.querySelectorAll(".rm-map-entry-family-row") : [];
  const selectedBeforeExplore = selectedGroups.slice();
  if (explore[0] && typeof explore[0].onclick === "function") explore[0].onclick();
  const cardLabels = cards.map((card) => {
    const identity = card.querySelector(".rm-map-entry-card__identity");
    return identity ? identity.textContent : "";
  });
  const cardCallbacks = cards.map((card) => {
    const callback = card.querySelector(".rm-map-entry-card__callback");
    return callback ? callback.textContent : "";
  });
  const cardSourceText = cards.map((card) => {
    const sources = card.querySelector(".rm-map-entry-card__sources");
    return sources ? sources.textContent : "";
  });
  const cardSourceHrefs = cards.map((card) => {
    const source = card.querySelector(".rm-source-action-link");
    return source && source.getAttribute("href") || "";
  });
  const cardAllSourceHrefs = cards.map((card) =>
    card.querySelectorAll(".rm-source-action-link").map((source) => source.getAttribute("href") || ""));
  const cardFamilyRowCounts = cards.map((card) => card.querySelectorAll(".rm-map-entry-family-row").length);
  const cardExploreCounts = cards.map((card) => card.querySelectorAll(".rm-map-entry-card__explore").length);
  const cardModelBadgeCounts = cards.map((card) => card.querySelectorAll(".rm-map-entry-card__origin").length);
  const entryGroups = root.querySelectorAll(".rm-map-entry-group");
  const cliGroup = entryGroups.find((group) => group.getAttribute("data-entry-surface-kind") === "cli_command");
  const cliPrimaryList = cliGroup && cliGroup.children.find((child) =>
    String(child.className || "").split(/\s+/).includes("rm-map-entry-group__list") &&
    !String(child.className || "").split(/\s+/).includes("rm-map-entry-group__list--remainder"));
  const cliMore = cliGroup && cliGroup.querySelector(".rm-map-entry-group__more");
  const modelBadges = root.querySelectorAll(".rm-map-entry-card__origin");
  const modelStates = root.querySelectorAll(".rm-map-entry-card__state");
  const modelFrontier = root.querySelector(".rm-map-entry-model-frontier");
  process.stdout.write(JSON.stringify({
    targetCount: targetLinks.length,
    targetHref: targetLinks[0] && targetLinks[0].getAttribute("href") || "",
    targetCurrent: targetLinks[0] && targetLinks[0].getAttribute("aria-current") || "",
    initialModeIDs, initialModeStates, initialLensHostMode, initialContextHidden,
    initialEntryCardCount, initialSelectedGroups, initialSelectedLenses,
    modeStates: modes.map((node) => node.getAttribute("aria-pressed")),
    lensHostPresent: !!lensHost,
    lensHostMode: lensHost && lensHost.getAttribute("data-lens") || "",
    contextHidden: !!(modeContext && modeContext.hidden),
    entryCardCount: cards.length, exploreCount: explore.length,
    familyDisclosureCount: familyDetails.length,
    familyRowText: familyRows.map((node) => node.textContent),
    cardLabels, cardCallbacks, cardSourceText, cardSourceHrefs, cardAllSourceHrefs,
    cardFamilyRowCounts, cardExploreCounts, cardModelBadgeCounts,
    groupKinds: entryGroups.map((group) => group.getAttribute("data-entry-surface-kind") || ""),
    groupText: entryGroups.map((group) => group.children[0] ? group.children[0].textContent : ""),
    cliVisibleCount: cliPrimaryList ? cliPrimaryList.children.length : 0,
    cliMoreCount: cliMore ? cliMore.querySelectorAll(".rm-map-entry-card").length : 0,
    cliMoreText: cliMore ? cliMore.textContent : "",
    cliMoreOpen: !!(cliMore && cliMore.open),
    modelBadgeCount: modelBadges.length,
    modelBadgeText: modelBadges.map((item) => item.textContent),
    modelStateText: modelStates.map((item) => item.textContent),
    modelFrontierText: modelFrontier ? modelFrontier.textContent : "",
    sourceActionCount: sourceActions.length,
    sourceHrefs: sourceActions.map((item) => item.getAttribute("href") || ""),
    selectedBeforeExplore,
    selectedGroups,
    selectedLenses,
    selectedStates: explore.map((item) => item.getAttribute("aria-pressed")),
    hash: window.location.hash,
    text: root.textContent.replace(/\s+/g, " ").trim(),
  }));
}).catch((error) => { process.stderr.write(String(error && error.stack || error)); process.exit(2); });
`
	runnerPath := filepath.Join(t.TempDir(), "entry-handoff-group-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}

	run := func(language, fixture string) entryHandoffAssetResult {
		t.Helper()
		output, err := exec.Command(node, runnerPath, asset, language, fixture).CombinedOutput()
		if err != nil {
			t.Fatalf("run entry handoff workspace (%s): %v\n%s", language, err, output)
		}
		var got entryHandoffAssetResult
		if err := json.Unmarshal(output, &got); err != nil {
			t.Fatalf("decode entry handoff workspace (%s): %v\n%s", language, err, output)
		}
		return got
	}

	en := run("en", "baseline")
	if en.TargetCount != 1 || en.TargetHref != "#canvas" || en.TargetCurrent != "page" || en.Hash != "#canvas" {
		t.Fatalf("single executable target rail = %#v", en)
	}
	if strings.Join(en.InitialModeIDs, ",") != "entrypoints,integrations" ||
		strings.Join(en.InitialModeStates, ",") != "false,false" ||
		!en.LensHostPresent || en.InitialLensHostMode != "landscape" || !en.InitialContextHidden ||
		en.InitialEntryCardCount != 0 || len(en.InitialSelectedGroups) != 0 ||
		strings.Join(en.InitialSelectedLenses, ",") != "landscape" {
		t.Fatalf("executable did not start on a neutral, unselected landscape: %#v", en)
	}
	if strings.Join(en.ModeStates, ",") != "true,false" || en.LensHostMode != "entrypoints" ||
		en.ContextHidden || en.EntryCardCount != 3 || en.ExploreCount != 2 ||
		en.FamilyDisclosureCount != 1 || len(en.FamilyRowText) != 1 ||
		!strings.Contains(en.FamilyRowText[0], "Start") || en.SourceActionCount < 4 {
		t.Fatalf("explicit Entrypoints launchpad = %#v", en)
	}
	if !strings.Contains(en.Text, "14 direct calls · 2 mapped areas · 0 off-map") ||
		!strings.Contains(en.Text, "1 direct calls · 0 mapped areas · 1 off-map") {
		t.Fatalf("Entrypoints map summary conflated missing source ownership or same-component calls with off-map targets: %s", en.Text)
	}
	if len(en.SelectedBeforeExplore) != 0 || strings.Join(en.SelectedGroups, ",") != "entry-group-a" ||
		strings.Join(en.SelectedStates, ",") != "true,false" ||
		len(en.SelectedLenses) < 2 || en.SelectedLenses[len(en.SelectedLenses)-1] != "entrypoints" {
		t.Fatalf("entry exploration was automatic or not exclusive: %#v", en)
	}
	if !strings.Contains(strings.Join(en.SourceHrefs, ","),
		"/blob/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/cmd/zero/main.go#L7") {
		t.Fatalf("unowned zero-hop process entry is not visible and source-backed: %#v", en)
	}
	for _, href := range en.SourceHrefs {
		if !strings.Contains(href, "github.com/acme/fixture/blob/") {
			t.Fatalf("Entrypoints source is not pinned: %q", href)
		}
	}
	for _, leaked := range []string{"claim_kind", "resolved_static", "architecture_entry_handoff", "entry-handoff-secret", "go:linux", "raw repeated limitation", "raw frontier limitation"} {
		if strings.Contains(en.Text, leaked) {
			t.Fatalf("diagnostic field %q leaked into ordinary HTML: %s", leaked, en.Text)
		}
	}

	ru := run("ru", "baseline")
	if strings.Join(ru.InitialModeIDs, ",") != strings.Join(en.InitialModeIDs, ",") ||
		ru.EntryCardCount != en.EntryCardCount || ru.ExploreCount != en.ExploreCount ||
		ru.FamilyDisclosureCount != en.FamilyDisclosureCount ||
		ru.SourceActionCount != en.SourceActionCount || ru.LensHostMode != "entrypoints" ||
		len(ru.InitialSelectedGroups) != 0 {
		t.Fatalf("neutral start or explicit Entrypoints behavior changed across locales: EN %#v RU %#v", en, ru)
	}

	moby := run("ru", "moby")
	if moby.InitialEntryCardCount != 0 || !moby.InitialContextHidden || len(moby.InitialSelectedGroups) != 0 ||
		moby.EntryCardCount != 11 || moby.ExploreCount != 1 || moby.FamilyDisclosureCount != 1 ||
		len(moby.FamilyRowText) != 6 || len(moby.SelectedBeforeExplore) != 0 ||
		strings.Join(moby.SelectedGroups, ",") != "entry-group-dockerd" ||
		strings.Join(moby.SelectedStates, ",") != "true" {
		t.Fatalf("Moby-shaped Entrypoints launchpad duplicated or auto-selected process context: %#v", moby)
	}
	mainCards := 0
	uniqueCards := make(map[string]struct{}, len(moby.CardLabels))
	for index, label := range moby.CardLabels {
		if label == "main" {
			mainCards++
			if moby.CardFamilyRowCounts[index] != 6 || moby.CardExploreCounts[index] != 1 ||
				!strings.Contains(moby.CardSourceHrefs[index], "/cmd/dockerd/main.go#L16") {
				t.Fatalf("Moby process card did not exclusively own source, calls and Explore: %#v", moby)
			}
		} else {
			if moby.CardFamilyRowCounts[index] != 0 || moby.CardExploreCounts[index] != 0 ||
				strings.Contains(moby.CardSourceHrefs[index], "/cmd/dockerd/main.go#L16") {
				t.Fatalf("Moby route/server card inherited process context: label=%q result=%#v", label, moby)
			}
		}
		uniqueCards[label+"\x00"+moby.CardSourceHrefs[index]] = struct{}{}
	}
	if mainCards != 1 || len(uniqueCards) != 11 || !containsString(moby.CardLabels, "/metrics") ||
		!containsString(moby.CardLabels, "GET /ready") || !containsString(moby.CardLabels, "/help") ||
		!containsString(moby.CardLabels, "/v{version:[0-9.]+}/{path:.*}") || containsString(moby.CardLabels, "*}") ||
		containsString(moby.CardLabels, "background sync") {
		t.Fatalf("Moby exact entry surfaces were collapsed, duplicated or polluted by runtime activity: %#v", moby)
	}
	if strings.Join(moby.GroupKinds, ",") != "process_entry,http_route,http_server" ||
		!strings.Contains(strings.Join(moby.GroupText, " "), "HTTP-маршруты6") ||
		strings.Contains(moby.Text, "fixture.poison") || strings.Contains(moby.Text, "ServeMux") ||
		strings.Contains(moby.Text, "result of github.com/docker/go-metrics.Handler") {
		t.Fatalf("Moby surface groups or fail-closed callbacks = %#v", moby)
	}
	readyIndex := -1
	metricsUnresolved := 0
	for index, label := range moby.CardLabels {
		if label == "GET /ready" {
			readyIndex = index
		}
		if label == "/metrics" && moby.CardCallbacks[index] == "обработчик не разрешён" {
			metricsUnresolved++
		}
	}
	if readyIndex < 0 || moby.CardCallbacks[readyIndex] != "ready()" ||
		!strings.Contains(moby.CardSourceText[readyIndex], "Регистрация") ||
		strings.Contains(moby.CardSourceText[readyIndex], "Обработчик") ||
		!strings.Contains(moby.CardSourceHrefs[readyIndex], "/daemon/libnetwork/diagnostic/server.go#L37") ||
		metricsUnresolved != 1 {
		t.Fatalf("fresh Moby route callback was not causally restored from its exact catalog ID: %#v", moby)
	}

	legacyCLI := run("ru", "legacy-cli")
	if strings.Join(legacyCLI.GroupKinds, ",") != "process_entry,cli_command" ||
		legacyCLI.EntryCardCount != 38 || legacyCLI.CLIVisibleCount != 6 || legacyCLI.CLIMoreCount != 31 ||
		legacyCLI.CLIMoreOpen || !strings.Contains(legacyCLI.CLIMoreText, "Показать ещё 31") ||
		!strings.Contains(strings.Join(legacyCLI.GroupText, " "), "CLI-команды37") {
		t.Fatalf("legacy CLI catalog was not bounded behind one honest native disclosure: %#v", legacyCLI)
	}
	if len(legacyCLI.CardLabels) < 7 || legacyCLI.CardLabels[1] != "restic" ||
		containsString(legacyCLI.CardLabels, "wrong canvas label 00") ||
		strings.Contains(legacyCLI.Text, "poison") ||
		!strings.Contains(legacyCLI.Text, "анонимный обработчик") ||
		!strings.Contains(legacyCLI.Text, "частичный путь") {
		t.Fatalf("legacy CLI identity/callback enrichment did not follow the exact stable ID: %#v", legacyCLI)
	}
	// The second CLI fixture gives registration and handler the identical
	// exact location. It must render one action, while the first keeps both.
	if len(legacyCLI.CardSourceText) < 4 ||
		strings.Count(legacyCLI.CardSourceText[2], "Регистрация") != 1 ||
		strings.Contains(legacyCLI.CardSourceText[2], "Обработчик") ||
		!strings.Contains(legacyCLI.CardSourceText[3], "Регистрация") ||
		!strings.Contains(legacyCLI.CardSourceText[3], "Обработчик") {
		t.Fatalf("legacy CLI exact registration/handler source actions were duplicated or lost: %#v", legacyCLI)
	}

	modelAssistedEN := run("en", "model-assisted")
	modelAssisted := run("ru", "model-assisted")
	modelAssistedShuffledEN := run("en", "model-assisted-shuffled")
	modelAssistedShuffledRU := run("ru", "model-assisted-shuffled")
	for name, candidate := range map[string]entryHandoffAssetResult{
		"ru":          modelAssisted,
		"shuffled-en": modelAssistedShuffledEN,
		"shuffled-ru": modelAssistedShuffledRU,
	} {
		if strings.Join(candidate.CardLabels, "\x00") != strings.Join(modelAssistedEN.CardLabels, "\x00") ||
			strings.Join(candidate.CardCallbacks, "\x00") != strings.Join(modelAssistedEN.CardCallbacks, "\x00") {
			t.Fatalf("model-assisted semantic order changed for %s: EN %#v candidate %#v", name, modelAssistedEN, candidate)
		}
	}
	if strings.Join(modelAssisted.GroupKinds, ",") != "process_entry,http_route,cli_command" ||
		modelAssisted.EntryCardCount != 12 || modelAssisted.ExploreCount != 0 ||
		modelAssisted.ModelBadgeCount != 9 || modelAssisted.CLIVisibleCount != 6 ||
		modelAssisted.CLIMoreCount != 2 || modelAssisted.CLIMoreOpen ||
		!strings.Contains(modelAssisted.CLIMoreText, "Показать ещё 2") ||
		!strings.Contains(modelAssisted.Text, "Точки входа · 12") {
		t.Fatalf("model-assisted Entrypoints union/count/disclosure = %#v", modelAssisted)
	}
	if len(modelAssisted.CardLabels) < 10 ||
		strings.Join(modelAssisted.CardLabels[:4], "\x00") != "main\x00GET /\x00GET /local\x00GET /ws" ||
		strings.Join(modelAssisted.CardLabels[4:10], "\x00") !=
			"add\x00backup [flags]\x00get [key]\x00health\x00index\x00key" {
		t.Fatalf("model-assisted root-first HTTP or semantic CLI order = %#v", modelAssisted.CardLabels)
	}
	if countString(modelAssisted.CardLabels, "GET /local") != 1 ||
		countString(modelAssisted.CardLabels, "GET /ws") != 1 ||
		containsString(modelAssisted.CardLabels, "wrong local label") ||
		countString(modelAssisted.CardLabels, "backup [flags]") != 1 ||
		!containsString(modelAssisted.CardLabels, "txn") ||
		strings.Contains(modelAssisted.Text, "hidden-local-only") ||
		strings.Contains(modelAssisted.Text, "model-surface-") {
		t.Fatalf("deterministic precedence or restored identities failed: %#v", modelAssisted)
	}
	if !allEntryHandoffStringsEqual(modelAssisted.ModelBadgeText, "С участием модели") ||
		countString(modelAssisted.ModelStateText, "точный статический вызов регистрации") != 2 ||
		countString(modelAssisted.ModelStateText, "объявленный дескриптор") != 7 ||
		!strings.Contains(modelAssisted.ModelFrontierText, "11 точек входа восстановлено") ||
		!strings.Contains(modelAssisted.ModelFrontierText, "2 предложения отклонены") ||
		!strings.Contains(modelAssisted.ModelFrontierText, "2 структурных кандидата оставлены без классификации") ||
		!strings.Contains(modelAssisted.ModelFrontierText, "3 кандидатов и 6 фактов") ||
		!strings.Contains(modelAssisted.ModelFrontierText, "2 небезопасных фактов") ||
		!strings.Contains(modelAssisted.ModelFrontierText, "1 недостижимых кандидатов") {
		t.Fatalf("model-assisted truth/frontier copy = %#v", modelAssisted)
	}
	if !strings.Contains(modelAssisted.Text, "каталог не исчерпывает все регистрации") ||
		!strings.Contains(modelAssisted.Text, "Внешние префиксы маршрута") {
		t.Fatalf("model-assisted non-exhaustive/prefix limitation is absent: %#v", modelAssisted)
	}
	echoIndex := -1
	for index, label := range modelAssisted.CardLabels {
		if label == "GET /ws" {
			echoIndex = index
			break
		}
	}
	if echoIndex < 0 || modelAssisted.CardCallbacks[echoIndex] != "hello()" ||
		!strings.Contains(modelAssisted.CardSourceText[echoIndex], "Регистрация") ||
		!strings.Contains(modelAssisted.CardSourceText[echoIndex], "Обработчик") ||
		!strings.Contains(modelAssisted.CardSourceHrefs[echoIndex], "/main.go#L208") ||
		modelAssisted.CardExploreCounts[echoIndex] != 0 {
		t.Fatalf("model-assisted exact route actions/callback = %#v", modelAssisted)
	}
	backupIndex := -1
	for index, label := range modelAssisted.CardLabels {
		if label == "backup [flags]" {
			backupIndex = index
			break
		}
	}
	if backupIndex < 0 || modelAssisted.CardCallbacks[backupIndex] != "localBackup()" ||
		modelAssisted.CardModelBadgeCounts[backupIndex] != 0 ||
		!strings.Contains(modelAssisted.CardSourceText[backupIndex], "Дескриптор") ||
		!strings.Contains(modelAssisted.CardSourceText[backupIndex], "Обработчик") ||
		!containsString(modelAssisted.CardAllSourceHrefs[backupIndex],
			"https://github.com/acme/fixture/blob/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/cmd/serve/cmd_00.go#L90") ||
		containsString(modelAssisted.CardAllSourceHrefs[backupIndex],
			"https://github.com/acme/fixture/blob/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/cmd/serve/cmd_00.go#L60") {
		t.Fatalf("local exact CLI descriptor did not override same-site model proposal: %#v", modelAssisted)
	}
}

func TestEntrypointHandoffCanvasOverlayKeepsLayoutAndOneSelection(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	asset, err := filepath.Abs(filepath.Join("templates", "architecture_canvas.js"))
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
    this.parentNode = null; this.handlers = {};
    this.classList = {
      add: (...names) => { const values = new Set(String(this.className).split(/\s+/).filter(Boolean)); names.forEach((name) => values.add(name)); this.className = Array.from(values).join(" "); },
      remove: (...names) => { const removed = new Set(names); this.className = String(this.className).split(/\s+/).filter((name) => name && !removed.has(name)).join(" "); },
      toggle: (name, force) => { const values = new Set(String(this.className).split(/\s+/).filter(Boolean)); const active = force === undefined ? !values.has(name) : !!force; if (active) values.add(name); else values.delete(name); this.className = Array.from(values).join(" "); return active; },
      contains: (name) => String(this.className).split(/\s+/).includes(name),
    };
  }
  get childNodes() { return this.children; }
  get childElementCount() { return this.children.length; }
  get textContent() { return this._text + this.children.map((child) => child.textContent).join(""); }
  set textContent(value) { this._text = value == null ? "" : String(value); }
  appendChild(child) { if (child) { child.parentNode = this; this.children.push(child); } return child; }
  append(...children) { children.forEach((child) => this.appendChild(child)); }
  prepend(child) { if (child) { child.parentNode = this; this.children.unshift(child); } }
  replaceChildren(...children) { this.children = []; this._text = ""; children.forEach((child) => this.appendChild(child)); }
  remove() { if (this.parentNode) this.parentNode.children = this.parentNode.children.filter((child) => child !== this); }
  setAttribute(name, value) { this.attributes[name] = String(value); if (name === "class") this.className = String(value); }
  getAttribute(name) { return this.attributes[name] == null ? null : String(this.attributes[name]); }
  removeAttribute(name) { delete this.attributes[name]; }
  addEventListener(name, handler) { (this.handlers[name] ||= []).push(handler); }
  removeEventListener() {}
  dispatch(name) { (this.handlers[name] || []).forEach((handler) => handler({ target: this, stopPropagation() {}, preventDefault() {} })); }
  focus() {}
  contains(node) { if (node === this) return true; return this.children.some((child) => child.contains && child.contains(node)); }
  closest() { return null; }
  getBoundingClientRect() { return { left: 0, top: 0, right: 900, bottom: 640, width: 900, height: 640 }; }
  querySelector(selector) { return walk(this).find((node) => matches(node, selector)) || null; }
  querySelectorAll(selector) { return walk(this).filter((node) => matches(node, selector)); }
  scrollIntoView() {}
}
function walk(node, out) { out = out || []; (node.children || []).forEach((child) => { out.push(child); walk(child, out); }); return out; }
function matches(node, selector) {
  if (selector[0] === ".") return String(node.className || "").split(/\s+/).includes(selector.slice(1));
  const attr = selector.match(/^\[([^=\]]+)(?:=["']?([^"'\]]+)["']?)?\]$/);
  return !!(attr && Object.prototype.hasOwnProperty.call(node.attributes || {}, attr[1]) && (attr[2] == null || node.attributes[attr[1]] === attr[2]));
}
const document = {
  createElement: (tag) => new Element(tag), createElementNS: (_, tag) => new Element(tag),
  createTextNode: (value) => { const node = new Element("#text"); node.textContent = value; return node; },
  querySelector: () => null, querySelectorAll: () => [], getElementById: () => null,
  addEventListener() {}, removeEventListener() {}, body: new Element("body"), documentElement: new Element("html"),
};
function ELK() {}
const window = {
  document, ELK, AbortController, Set, Map, URLSearchParams, Promise,
  requestAnimationFrame: (fn) => fn(), setTimeout, clearTimeout,
  innerWidth: 1200, innerHeight: 800, location: { hash: "" }, history: { replaceState() {} },
  addEventListener() {}, removeEventListener() {},
  RepomapUI: { message(id) { return id; } },
};
const sandbox = {
  window, document, Element, ELK, AbortController, Set, Map, URLSearchParams, Promise,
  requestAnimationFrame: (fn) => fn(), setTimeout, clearTimeout, console,
  addEventListener() {}, removeEventListener() {},
};
sandbox.global = sandbox; vm.createContext(sandbox);
vm.runInContext(fs.readFileSync(process.argv[2], "utf8"), sandbox);
function transition(path, line, column, symbol, componentIDs, target) {
  return { claim_kind: target ? "direct_static_call" : "process_entry", support_mode: "resolved_static",
    label: symbol, path, line, column, symbol, component_ids: componentIDs, target };
}
function group(id, entryIDs, handoffs) {
  return { version: 2, id, component_ids: Array.from(new Set(entryIDs.concat(...handoffs.map((item) => item.component_ids)))),
    entry: transition("main.go", 10, 6, "fixture.main", entryIDs, null), entry_handoffs: handoffs,
    frontier: { ordering: "not_established", limitation: "continuation unknown" } };
}
const cross = transition("main.go", 15, 9, "fixture.service.Start", ["c2"],
  { label: "fixture.service.Start", path: "service.go", line: 20, column: 4 });
const local = Array.from({ length: 13 }, (_, index) => transition(
  "main.go", 30 + index, 5, "fixture.local" + index, ["c1"],
  { label: "fixture.local" + index, path: "main.go", line: 100 + index, column: 2 }
));
const second = transition("worker.go", 12, 7, "fixture.worker.cross", ["c1"],
  { label: "fixture.worker.local", path: "worker.go", line: 22, column: 3 });
const repeated = Array.from({ length: 19 }, (_, index) => transition(
  "main.go", 130 + index, 2, "fixture.http.Register" + index, ["c2"],
  { label: "fixture.http.Register" + index, path: "http/register" + index + ".go", line: 20 + index, column: 6 }
));
const data = {
  version: 15,
  components: [{ id: "c1", name: "Entry" }, { id: "c2", name: "Service" }],
  subsystems: [{ id: "s1", name: "Infrastructure and integration services",
    description: "Exact HTTP and provider wiring", component_ids: ["c1", "c2"] }],
  structural_edges: [], behavior_anchors: [], flows: [], flow_edges: [], surfaces: [],
  entry_handoff_groups: [
    group("group-a", ["c1"], [cross].concat(local)),
    group("group-b", ["c2"], [second]),
    group("group-c", ["c1"], repeated),
  ],
};
const opened = [];
const host = new Element("div");
const app = window.RepomapArchitectureCanvas.mount(host, data, {
  userMode: true, message: (id) => id === "architecture.relation.calls" ? "calls" : id,
  openLocation(path, line, column) { opened.push([path, line, column]); },
});
app.ready.then(() => {
  app.setLens("entrypoints");
  const surface = host.querySelector(".rm-arch__surface");
  const components = host.querySelectorAll(".rm-arch__component");
  const groupTitle = host.querySelector(".rm-arch__group-title");
  const groupShell = host.querySelector(".rm-arch__group");
  const before = JSON.stringify({ transform: surface.style.transform, nodes: components.map((node) => [node.style.left, node.style.top]) });
  const firstCard = components[0].querySelector(".rm-arch__component-card");
  firstCard.dispatch("click");
  const selectedBeforeEntry = components.filter((node) => node.classList.contains("is-selected")).length;
  const dimmedBeforeEntry = components.filter((node) => node.classList.contains("is-dimmed")).length;
  app.selectEntrypointHandoffGroup("group-a");
  const selectedWithEntry = components.filter((node) => node.classList.contains("is-selected")).length;
  const dimmedWithEntry = components.filter((node) => node.classList.contains("is-dimmed")).length;
  const edgesA = host.querySelectorAll(".rm-arch__edge--entry-handoff");
  const badgesA = host.querySelectorAll(".rm-arch__entry-handoff-source");
  const markersA = edgesA.map((edge) => edge.querySelector(".rm-arch__edge-visible")).filter(Boolean)
    .map((path) => path.getAttribute("marker-end"));
  const participantsA = components.filter((node) => String(node.className).split(/\s+/).includes("rm-arch__is-entry-handoff-participant"));
  if (badgesA[0]) badgesA[0].dispatch("click");
  const after = JSON.stringify({ transform: surface.style.transform, nodes: components.map((node) => [node.style.left, node.style.top]) });
  app.selectEntrypointHandoffGroup("group-b");
  const edgesB = host.querySelectorAll(".rm-arch__edge--entry-handoff");
  app.selectEntrypointHandoffGroup("group-c");
  const aggregateEdges = host.querySelectorAll(".rm-arch__edge--entry-handoff");
  const aggregateBadges = host.querySelectorAll(".rm-arch__entry-handoff-source");
  const aggregateBadgeCounts = aggregateBadges.map((item) => item.getAttribute("data-entry-handoff-call-count") || "");
  const aggregateBadgeText = aggregateBadges.map((item) => item.textContent);
  firstCard.dispatch("click");
  const edgesAfterComponent = host.querySelectorAll(".rm-arch__edge--entry-handoff").length;
  const badgesAfterComponent = host.querySelectorAll(".rm-arch__entry-handoff-source").length;
  const participantsAfterComponent = components.filter((node) => node.classList.contains("rm-arch__is-entry-handoff-participant")).length;
  const selectedAfterComponent = components.filter((node) => node.classList.contains("is-selected")).length;
  const dimmedAfterComponent = components.filter((node) => node.classList.contains("is-dimmed")).length;
  const studyMechanism = {
    id: "study-mechanism-1", ordinal: 1,
    nodes: [
      { id: "n1", label: "fixture.main", component_ids: ["c1"] },
      { id: "n2", label: "fixture.start", component_ids: ["c2"] },
      { id: "n3", label: "fixture.local", component_ids: ["c2"] },
      { id: "n4", label: "fixture.offmap", component_ids: [] },
      { id: "n5", label: "fixture.shared", component_ids: ["c1", "c2"] },
      { id: "n6", label: "fixture.finish", component_ids: ["c1"] },
    ],
    edges: [
      { id: "e1", from_node_id: "n1", to_node_id: "n2", invocation: "synchronous" },
      { id: "e2", from_node_id: "n2", to_node_id: "n3", invocation: "goroutine" },
      { id: "e3", from_node_id: "n3", to_node_id: "n4", invocation: "deferred" },
      { id: "e4", from_node_id: "n4", to_node_id: "n5", invocation: "synchronous" },
      { id: "e5", from_node_id: "n5", to_node_id: "n6", invocation: "synchronous" },
    ],
  };
  app.setLens("landscape");
  const studyBefore = JSON.stringify({ transform: surface.style.transform, nodes: components.map((node) => [node.style.left, node.style.top]) });
  app.setStudyMechanismOverlay(studyMechanism);
  const studyEdges = host.querySelectorAll(".rm-arch__edge--study-mechanism");
  const studyParticipants = components.filter((node) => String(node.className).split(/\s+/).includes("rm-arch__is-study-mechanism-participant"));
  const studyMarkers = studyEdges.map((edge) => edge.querySelector(".rm-arch__edge-visible")).filter(Boolean)
    .map((path) => path.getAttribute("marker-end"));
  const studyProjection = window.RepomapArchitectureCanvas.projectStudyMechanismOverlay(studyMechanism, ["c1", "c2"]);
  const studyAfter = JSON.stringify({ transform: surface.style.transform, nodes: components.map((node) => [node.style.left, node.style.top]) });
  app.clearStudyMechanismOverlay();
  process.stdout.write(JSON.stringify({
    edgesA: edgesA.length, badgesA: badgesA.length, markersA,
    participantsA: participantsA.length, badgeText: badgesA.map((item) => item.textContent),
    opened, layoutUnchanged: before === after, edgesB: edgesB.length,
    aggregateEdges: aggregateEdges.length, aggregateBadges: aggregateBadges.length,
    aggregateBadgeCounts, aggregateBadgeText,
    selectedBeforeEntry, dimmedBeforeEntry, selectedWithEntry, dimmedWithEntry,
    edgesAfterComponent, badgesAfterComponent, participantsAfterComponent,
    selectedAfterComponent, dimmedAfterComponent,
    groupTitleText: groupTitle && groupTitle.textContent || "",
    groupTitleTooltip: groupTitle && groupTitle.title || "",
    groupTooltip: groupShell && groupShell.title || "",
    studyEdges: studyEdges.length, studyParticipants: studyParticipants.length,
    studyMarkers, studySideReasons: studyProjection.side_rows.map((row) => row.reason),
    studyComponentIDs: studyProjection.component_ids,
    studyLayoutUnchanged: studyBefore === studyAfter,
    studyEdgesAfterClear: host.querySelectorAll(".rm-arch__edge--study-mechanism").length,
    studyParticipantsAfterClear: components.filter((node) => String(node.className).split(/\s+/).includes("rm-arch__is-study-mechanism-participant")).length,
  }));
}).catch((error) => { process.stderr.write(String(error && error.stack || error)); process.exit(2); });
`
	runnerPath := filepath.Join(t.TempDir(), "entry-handoff-overlay-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, asset).CombinedOutput()
	if err != nil {
		t.Fatalf("run entry handoff overlay: %v\n%s", err, output)
	}
	var got struct {
		EdgesA                      int      `json:"edgesA"`
		BadgesA                     int      `json:"badgesA"`
		MarkersA                    []string `json:"markersA"`
		ParticipantsA               int      `json:"participantsA"`
		BadgeText                   []string `json:"badgeText"`
		Opened                      [][]any  `json:"opened"`
		LayoutUnchanged             bool     `json:"layoutUnchanged"`
		EdgesB                      int      `json:"edgesB"`
		AggregateEdges              int      `json:"aggregateEdges"`
		AggregateBadges             int      `json:"aggregateBadges"`
		AggregateBadgeCounts        []string `json:"aggregateBadgeCounts"`
		AggregateBadgeText          []string `json:"aggregateBadgeText"`
		SelectedBeforeEntry         int      `json:"selectedBeforeEntry"`
		DimmedBeforeEntry           int      `json:"dimmedBeforeEntry"`
		SelectedWithEntry           int      `json:"selectedWithEntry"`
		DimmedWithEntry             int      `json:"dimmedWithEntry"`
		EdgesAfterComponent         int      `json:"edgesAfterComponent"`
		BadgesAfterComponent        int      `json:"badgesAfterComponent"`
		ParticipantsAfterComponent  int      `json:"participantsAfterComponent"`
		SelectedAfterComponent      int      `json:"selectedAfterComponent"`
		DimmedAfterComponent        int      `json:"dimmedAfterComponent"`
		GroupTitleText              string   `json:"groupTitleText"`
		GroupTitleTooltip           string   `json:"groupTitleTooltip"`
		GroupTooltip                string   `json:"groupTooltip"`
		StudyEdges                  int      `json:"studyEdges"`
		StudyParticipants           int      `json:"studyParticipants"`
		StudyMarkers                []string `json:"studyMarkers"`
		StudySideReasons            []string `json:"studySideReasons"`
		StudyComponentIDs           []string `json:"studyComponentIDs"`
		StudyLayoutUnchanged        bool     `json:"studyLayoutUnchanged"`
		StudyEdgesAfterClear        int      `json:"studyEdgesAfterClear"`
		StudyParticipantsAfterClear int      `json:"studyParticipantsAfterClear"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode entry handoff overlay: %v\n%s", err, output)
	}
	if got.EdgesA != 1 || got.BadgesA != 1 || got.ParticipantsA != 2 || !got.LayoutUnchanged {
		t.Fatalf("selected overlay = edges:%d badges:%d participants:%d layout unchanged:%t",
			got.EdgesA, got.BadgesA, got.ParticipantsA, got.LayoutUnchanged)
	}
	if got.EdgesB != 1 {
		t.Fatalf("second selection rendered %d edges, want exactly its one edge", got.EdgesB)
	}
	if got.AggregateEdges != 1 || got.AggregateBadges != 1 ||
		strings.Join(got.AggregateBadgeCounts, ",") != "19" ||
		strings.Join(got.AggregateBadgeText, ",") != "19 calls" {
		t.Fatalf("19 parallel component-pair calls = edges:%d badges:%d counts:%#v labels:%#v",
			got.AggregateEdges, got.AggregateBadges, got.AggregateBadgeCounts, got.AggregateBadgeText)
	}
	if got.SelectedBeforeEntry != 1 || got.DimmedBeforeEntry != 1 ||
		got.SelectedWithEntry != 0 || got.DimmedWithEntry != 0 {
		t.Fatalf("entry activation did not exclusively clear component focus: selected/dimmed %d/%d -> %d/%d",
			got.SelectedBeforeEntry, got.DimmedBeforeEntry, got.SelectedWithEntry, got.DimmedWithEntry)
	}
	if got.EdgesAfterComponent != 0 || got.BadgesAfterComponent != 0 ||
		got.ParticipantsAfterComponent != 0 || got.SelectedAfterComponent != 1 || got.DimmedAfterComponent != 1 {
		t.Fatalf("component click did not exclusively clear entry overlay: edges %d badges %d participants %d selected %d dimmed %d",
			got.EdgesAfterComponent, got.BadgesAfterComponent, got.ParticipantsAfterComponent,
			got.SelectedAfterComponent, got.DimmedAfterComponent)
	}
	if got.GroupTitleText != "Infrastructure and integration services" ||
		got.GroupTitleTooltip != "Infrastructure and integration services — Exact HTTP and provider wiring" ||
		got.GroupTooltip != got.GroupTitleTooltip {
		t.Fatalf("full group title/tooltip lost: text %q title %q shell %q",
			got.GroupTitleText, got.GroupTitleTooltip, got.GroupTooltip)
	}
	if len(got.MarkersA) != 1 || got.MarkersA[0] != "url(#rm-arch-entry-handoff-arrow)" ||
		len(got.BadgeText) != 1 || !strings.Contains(strings.Join(got.BadgeText, " "), "main.go:15:9") {
		t.Fatalf("overlay arrows/source badges = markers %#v labels %#v", got.MarkersA, got.BadgeText)
	}
	if len(got.Opened) != 1 || len(got.Opened[0]) != 3 || got.Opened[0][0] != "main.go" ||
		got.Opened[0][1] != float64(15) || got.Opened[0][2] != float64(9) {
		t.Fatalf("overlay source action lost exact column: %#v", got.Opened)
	}
	if got.StudyEdges != 1 || got.StudyParticipants != 2 || !got.StudyLayoutUnchanged {
		t.Fatalf("Study overlay = edges:%d participants:%d layout unchanged:%t",
			got.StudyEdges, got.StudyParticipants, got.StudyLayoutUnchanged)
	}
	if strings.Join(got.StudyMarkers, ",") != "url(#rm-arch-study-mechanism-arrow)" ||
		strings.Join(got.StudySideReasons, ",") != "same_component,zero_component,zero_component,plural_components" ||
		strings.Join(got.StudyComponentIDs, ",") != "c1,c2" {
		t.Fatalf("Study projection = markers %#v sides %#v participants %#v",
			got.StudyMarkers, got.StudySideReasons, got.StudyComponentIDs)
	}
	if got.StudyEdgesAfterClear != 0 || got.StudyParticipantsAfterClear != 0 {
		t.Fatalf("cleared Study overlay retained edges %d or participants %d",
			got.StudyEdgesAfterClear, got.StudyParticipantsAfterClear)
	}
}

type entryHandoffAssetResult struct {
	TargetCount           int        `json:"targetCount"`
	TargetHref            string     `json:"targetHref"`
	TargetCurrent         string     `json:"targetCurrent"`
	InitialModeIDs        []string   `json:"initialModeIDs"`
	InitialModeStates     []string   `json:"initialModeStates"`
	InitialLensHostMode   string     `json:"initialLensHostMode"`
	InitialContextHidden  bool       `json:"initialContextHidden"`
	InitialEntryCardCount int        `json:"initialEntryCardCount"`
	InitialSelectedGroups []string   `json:"initialSelectedGroups"`
	InitialSelectedLenses []string   `json:"initialSelectedLenses"`
	ModeStates            []string   `json:"modeStates"`
	LensHostPresent       bool       `json:"lensHostPresent"`
	LensHostMode          string     `json:"lensHostMode"`
	ContextHidden         bool       `json:"contextHidden"`
	EntryCardCount        int        `json:"entryCardCount"`
	ExploreCount          int        `json:"exploreCount"`
	FamilyDisclosureCount int        `json:"familyDisclosureCount"`
	FamilyRowText         []string   `json:"familyRowText"`
	CardLabels            []string   `json:"cardLabels"`
	CardCallbacks         []string   `json:"cardCallbacks"`
	CardSourceText        []string   `json:"cardSourceText"`
	CardSourceHrefs       []string   `json:"cardSourceHrefs"`
	CardAllSourceHrefs    [][]string `json:"cardAllSourceHrefs"`
	CardFamilyRowCounts   []int      `json:"cardFamilyRowCounts"`
	CardExploreCounts     []int      `json:"cardExploreCounts"`
	CardModelBadgeCounts  []int      `json:"cardModelBadgeCounts"`
	GroupKinds            []string   `json:"groupKinds"`
	GroupText             []string   `json:"groupText"`
	CLIVisibleCount       int        `json:"cliVisibleCount"`
	CLIMoreCount          int        `json:"cliMoreCount"`
	CLIMoreText           string     `json:"cliMoreText"`
	CLIMoreOpen           bool       `json:"cliMoreOpen"`
	ModelBadgeCount       int        `json:"modelBadgeCount"`
	ModelBadgeText        []string   `json:"modelBadgeText"`
	ModelStateText        []string   `json:"modelStateText"`
	ModelFrontierText     string     `json:"modelFrontierText"`
	SourceActionCount     int        `json:"sourceActionCount"`
	SourceHrefs           []string   `json:"sourceHrefs"`
	SelectedBeforeExplore []string   `json:"selectedBeforeExplore"`
	SelectedGroups        []string   `json:"selectedGroups"`
	SelectedLenses        []string   `json:"selectedLenses"`
	SelectedStates        []string   `json:"selectedStates"`
	Hash                  string     `json:"hash"`
	Text                  string     `json:"text"`
}

func allEntryHandoffStringsEqual(values []string, want string) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if value != want {
			return false
		}
	}
	return true
}
