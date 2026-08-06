package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type overviewGroup struct {
	Label   string `json:"label"`
	ShowAll int    `json:"showAll"`
	Visible bool   `json:"visible"`
	Cards   int    `json:"cards"`
}

// TestOverviewEntrySurfaceShapeAwareClassification executes the real
// templates/script.js + ui_messages.js in Node and verifies the Decision 233
// contract: the Overview entry surface classification is repository-shape +
// product-role aware. A modular_platform_server repository renders exact HTTP
// routes as a PRIMARY category summary (visible, representatives + Show all
// N), while CLI commands stay reachable tooling; a cli_tool repository
// promotes its command tree to primary. The golden HTML cannot exercise this
// (its fixture has no triggers/archetype), so this is the F8 regression.
func TestOverviewEntrySurfaceShapeAwareClassification(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	assetPath, err := filepath.Abs(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs");
const vm = require("vm");
class Element {
  constructor(tag) {
    this.tagName = tag;
    this.className = "";
    this.textContent = "";
    this.children = [];
    this.attributes = {};
    this.hidden = false;
    this.parentNode = null;
    this.classList = {
      add: (name) => { if (!this.className.split(/\s+/).includes(name)) this.className = (this.className + " " + name).trim(); },
      remove: (name) => { this.className = this.className.split(/\s+/).filter((value) => value && value !== name).join(" "); },
      toggle: (name, force) => { if (force) this.classList.add(name); else this.classList.remove(name); },
    };
  }
  get childNodes() { return this.children; }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return this.attributes[name] || null; }
  removeAttribute(name) { delete this.attributes[name]; }
  appendChild(child) { child.parentNode = this; this.children.push(child); return child; }
  append(...children) { children.forEach((child) => this.appendChild(child)); }
  prepend(child) { child.parentNode = this; this.children.unshift(child); }
  replaceChildren(...children) { this.children = []; this.append(...children); }
  remove() {
    if (!this.parentNode) return;
    this.parentNode.children = this.parentNode.children.filter((child) => child !== this);
    this.parentNode = null;
  }
  querySelector(selector) {
    if (!selector.startsWith(".")) return null;
    const className = selector.slice(1);
    return walk(this).find((node) => node !== this && String(node.className).split(/\s+/).includes(className)) || null;
  }
  querySelectorAll(selector) {
    if (!selector.startsWith(".")) return [];
    const className = selector.slice(1);
    return walk(this).filter((node) => node !== this && String(node.className).split(/\s+/).includes(className));
  }
  addEventListener() {}
  contains(node) { return walk(this).includes(node); }
  focus() { this.focused = true; }
  onclick = null;
  scrollIntoView() {}
}
function walk(root) {
  const result = [];
  (function visit(node) { result.push(node); (node.children || []).forEach(visit); })(root);
  return result;
}
function text(root) { return walk(root).map((node) => String(node.textContent || "")).join(""); }
function byClass(root, className) {
  return walk(root).filter((node) => String(node.className).split(/\s+/).includes(className));
}
function snippet(path, symbol, line) {
  return {
    path, enclosing_symbol: symbol, start_line: line, end_line: line + 1,
    content_sha256: "a".repeat(64), presentation_sha256: ("x" + line).padStart(64, "0"),
    lines: [{ line, text: "func " + symbol + "() {}", highlight: true }],
  };
}
function triggerService(kind, id, title, path, line, symbol) {
  const trigger = {
    id, kind, surface_role: "entry_surface", application_classification: "application_surface",
    availability: "available", executable_role: "primary_application", status: "resolved",
    handler: { kind: "symbol", text: symbol, known: true },
    handler_location: { path, line },
  };
  if (kind === "http_route") {
    trigger.identity = { method: "GET", path: { kind: "known", text: title, known: true } };
  } else if (kind === "cli_command") {
    trigger.identity = { name: title };
  } else {
    trigger.identity = { name: title };
    trigger.process_entrypoint = { name: symbol, location: { path, line } };
  }
  return trigger;
}
function reportFor(archetype, triggers) {
  return {
    repo_name: "fixture", report_language: "en",
    user_sources: [], user_mechanisms: [], user_topics: [],
    openable_paths: ["cmd/service/main.go", "internal/api/handlers/users.go", "internal/api/handlers/orders.go", "cmd/service/cli.go", "cmd/cli/main.go"],
    github_source_links: { repository_url: "https://github.com/example/fixture", revision: "a".repeat(40) },
    navigator: { version: 2, state: "unavailable" },
    repository_atlas: {
      version: 1,
      units: [{ id: "repo", kind: "repository", name: "fixture" }, { id: "app", kind: "app", parent_id: "repo", name: "fixture" }],
      entities: [], observations: [], relations: [], evidence: [],
    },
    architecture_canvas: {
      version: 10, repository_archetype: archetype,
      validation_outcome: "accepted", architecture_source: "validated_model", architecture_level: 2,
      title: "Fixture", components: [], behavior_anchors: [], surfaces: [], flows: [],
    },
    discovered_surfaces: { version: 1, total_count: triggers.length, triggers },
  };
}
function journey(report) {
  const roots = {};
  ["rm-overview"].forEach((id) => {
    roots[id] = new Element("section");
    roots[id].className = "rm-tab-content rm-active";
  });
  roots["rm-tabs"] = new Element("nav");
  roots["rm-source-drawer"] = new Element("aside");
  roots["rm-source-drawer"].hidden = true;
  roots["rm-source-drawer-content"] = new Element("div");
  roots["rm-source-drawer-close"] = new Element("button");
  const workspace = new Element("main");
  const window = {
    location: { search: "", hash: "#/overview", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
    history: { state: null, pushState(state, _, hash) { this.state = state; window.location.hash = hash; }, replaceState(state, _, hash) { this.state = state; window.location.hash = hash; }, back() {} },
    __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {}, open() {}, scrollTo() {},
  };
  window.RepomapArchitectureCanvas = {
    mount() { return { ready: Promise.resolve(), destroy() {}, openComponent() {}, openTrace() {}, openFlowStep() {}, openSurface() {} }; },
  };
  const document = {
    createElement(tag) { return new Element(tag); },
    createTextNode(value) { const node = new Element("#text"); node.textContent = String(value); return node; },
    getElementById(id) {
      if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
      return roots[id] || null;
    },
    querySelector(selector) {
      if (selector === ".rm-workspace") return workspace;
      if (selector.startsWith(".")) {
        const wanted = selector.slice(1);
        for (const root of Object.values(roots)) {
          const found = walk(root).find((node) => String(node.className).split(/\s+/).includes(wanted));
          if (found) return found;
        }
      }
      return null;
    },
    querySelectorAll(selector) {
      if (selector === ".rm-main-content > .rm-tab-content") return Object.values(roots);
      return [];
    },
  };
  document.documentElement = { lang: "en" };
  window.document = document;
  vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), { window });
  vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
    window, document, URLSearchParams, Set, Map, AbortController, Promise,
  });
  const api = window.__REPOMAP_WORKSPACE_TEST__;
  api.renderWorkspaceTabs();
  api.renderOverviewWorkspace();
  const groups = byClass(roots["rm-overview"], "rm-overview-entry-group").map((group) => {
    const label = byClass(group, "rm-overview-entry-group__label").map((node) => text(node)).join("");
    const showAll = byClass(group, "rm-overview-entry-group__show-all").length;
    const visible = group.parentNode === roots["rm-overview"] || (group.parentNode && String(group.parentNode.className).split(/\s+/).includes("rm-overview-entry-disclosure")) === false;
    const cards = byClass(group, "rm-overview-object-card").length;
    return { label, showAll, visible, cards };
  });
  const overviewText = text(roots["rm-overview"]);
  return { groups, overviewText };
}
const service = reportFor("modular_platform_server", [
  triggerService("http_route", "t-users", "/api/users", "internal/api/handlers/users.go", 31, "handleUsers"),
  triggerService("http_route", "t-orders", "/api/orders", "internal/api/handlers/orders.go", 44, "handleOrders"),
  triggerService("http_route", "t-health", "/healthz", "internal/api/handlers/users.go", 55, "handleHealth"),
  triggerService("http_route", "t-metrics", "/metrics", "internal/api/handlers/orders.go", 66, "handleMetrics"),
  triggerService("cli_command", "t-cli", "serve", "cmd/service/cli.go", 90, "serve"),
  triggerService("process_entry", "t-main", "main", "cmd/service/main.go", 12, "main"),
]);
const cli = reportFor("cli_tool", [
  triggerService("cli_command", "t-run", "run", "cmd/cli/main.go", 20, "run"),
  triggerService("cli_command", "t-status", "status", "cmd/cli/main.go", 42, "status"),
  triggerService("cli_command", "t-init", "init", "cmd/cli/main.go", 70, "init"),
  triggerService("process_entry", "t-climain", "main", "cmd/cli/main.go", 12, "main"),
]);
process.stdout.write(JSON.stringify({ service: journey(service), cli: journey(cli) }));
`
	runnerPath := filepath.Join(t.TempDir(), "overview-classification-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	type group struct {
		Label   string `json:"label"`
		ShowAll int    `json:"showAll"`
		Visible bool   `json:"visible"`
		Cards   int    `json:"cards"`
	}
	type result struct {
		Groups       []overviewGroup `json:"groups"`
		OverviewText string  `json:"overviewText"`
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run overview classification workspace: %v\n%s", err, output)
	}
	var out struct {
		Service result `json:"service"`
		CLI     result `json:"cli"`
	}
	if err := json.Unmarshal(output, &out); err != nil {
		t.Fatalf("decode overview classification: %v\n%s", err, output)
	}

	// Service shape: exact HTTP routes render inside the PRIMARY category
	// (visible, representative cards + Show-all disclosure); the CLI command
	// stays reachable but collapsed as tooling (not primary).
	primaryGroup := findGroup(out.Service.Groups, "Primary product entry")
	if primaryGroup == nil {
		t.Fatalf("service-shaped Overview must render a primary category, groups=%#v", out.Service.Groups)
	}
	if primaryGroup.Cards < 3 {
		t.Fatalf("primary category must show representative cards, got %d", primaryGroup.Cards)
	}
	if primaryGroup.ShowAll != 1 {
		t.Fatalf("primary category (4 routes) must expose Show all N, got %d", primaryGroup.ShowAll)
	}
	toolingGroup := findGroup(out.Service.Groups, "CLI and tooling")
	if toolingGroup == nil {
		t.Fatalf("CLI commands must remain reachable as tooling, groups=%#v", out.Service.Groups)
	}
	if toolingGroup.Visible {
		t.Fatalf("tooling must be collapsed on a service-shaped repository")
	}
	if !strings.Contains(out.Service.OverviewText, "/api/users") || !strings.Contains(out.Service.OverviewText, "/api/orders") {
		t.Fatalf("exact HTTP route representatives missing from the Overview text")
	}
	if !strings.Contains(out.Service.OverviewText, "serve") {
		t.Fatalf("CLI command disappeared from the service Overview")
	}

	// CLI shape: the command tree is the PRIMARY category (visible with
	// representatives), never collapsed under tooling.
	cliPrimary := findGroup(out.CLI.Groups, "Primary product entry")
	if cliPrimary == nil {
		t.Fatalf("CLI-shaped Overview must render a primary category, groups=%#v", out.CLI.Groups)
	}
	if tooling := findGroup(out.CLI.Groups, "CLI and tooling"); tooling != nil && tooling.Visible {
		t.Fatalf("CLI-shaped repository must not collapse its own command tree as tooling")
	}
	if !strings.Contains(out.CLI.OverviewText, "run") || !strings.Contains(out.CLI.OverviewText, "status") {
		t.Fatalf("CLI-shaped repository must show its command tree")
	}
}

func findGroup(groups []overviewGroup, needle string) *overviewGroup {
	for index := range groups {
		if strings.Contains(groups[index].Label, needle) {
			return &groups[index]
		}
	}
	return nil
}
