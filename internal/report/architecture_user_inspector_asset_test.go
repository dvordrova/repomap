package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchitectureComponentContextsUseTypedNavigationWithoutRepresentativePackageSource(t *testing.T) {
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
function snippet(path, symbol, line) {
  return {
    path, enclosing_symbol: symbol, start_line: line, end_line: line + 1,
    lines: [{ line, text: "func " + symbol + "() {}", highlight: true }],
  };
}
const report = {
  user_mechanisms: [], user_sources: [], source_ids: {},
  openable_paths: ["core/a.go", "core/b.go", "other.go", "shared.go", "shared2.go", "worker/a.go", "worker/b.go"],
  architecture_canvas: {
    behavior_anchors: [
      {
        id: "process-anchor", kind: "process_entry", label: "Process entry main",
        location: { path: "core/a.go", line: 7 },
      },
      {
        id: "process-anchor-only", kind: "process_entry", label: "Process entry worker",
        location: { path: "shared2.go", line: 12 },
      },
      {
        id: "shared-anchor", label: "Shared behavior family",
        location: { path: "shared.go", line: 3 },
      },
      {
        id: "second-anchor", label: "Second anchor-only behavior",
        location: { path: "shared2.go", line: 8 },
      },
    ],
    components: [
      {
        id: "core", anchor_ids: ["process-anchor", "shared-anchor"],
        owned_surface_ids: ["http-get-widgets"],
        participating_surface_ids: ["http-post-widgets", "process-main"], members: [
        {
          id: { kind: "package", value: "opaque-package-id" },
          facts: [{ kind: "declaration", value: "example.test/project/core" }],
        },
        {
          name: "example.test/project/core.A",
          facts: [{ kind: "declaration", value: "example.test/project/core.A", location: { path: "core/a.go", line: 7, column: 9 } }],
        },
        {
          name: "core/a.go",
          facts: [{ kind: "repository_path", value: "core/a.go", location: { path: "core/a.go" } }],
        },
      ] },
      { id: "same-name-only", members: [{
        id: { kind: "package", value: "opaque-package-id-2" }, name: "core", facts: [],
      }] },
      { id: "exact-member-only", anchor_ids: ["shared-anchor"], members: [{
        id: { kind: "symbol", value: "opaque-symbol-id" }, name: "ValidateFunctionURL",
        facts: [{
          kind: "declaration", value: "ValidateFunctionURL",
          location: { path: "other.go", line: 41 },
        }],
      }] },
      { id: "package-only", members: [{
        id: { kind: "package", value: "opaque-package-id-3" }, name: "worker",
        facts: [{ kind: "declaration", value: "example.test/project/worker" }],
      }] },
      { id: "anchor-only", anchor_ids: ["shared-anchor", "second-anchor"], members: [] },
	  { id: "process-anchor-only", anchor_ids: ["process-anchor-only"], members: [] },
	  { id: "relation-only", members: [{
		id: { kind: "symbol", value: "relation-source" }, name: "Request dispatcher", facts: [],
	  }] },
    ],
	structural_edges: [{
	  id: "relation-exact",
	  from_component_ids: ["relation-only"],
	  to_component_ids: ["exact-member-only"],
	  witness: {
		id: "relation-exact", kind: "bounded_direct_call",
		from: { kind: "symbol", value: "relation-source" },
		to: { kind: "symbol", value: "opaque-symbol-id" },
		location: { path: "other.go", line: 41 },
	  },
	}],
    surfaces: [
      {
        id: "http-get-widgets", name: "GET /widgets",
        evidence: [
          { path: "core/a.go", line: 15 },
          { path: "core/b.go", line: 33 },
        ],
      },
      {
        id: "http-post-widgets", name: "POST /widgets",
        evidence: [{ path: "core/a.go", line: 27 }],
      },
      {
        id: "process-main", name: "main", kind: "process_entry",
        evidence: [{ path: "core/a.go", line: 7 }],
      },
    ],
  },
  architecture_component_navigation: {
    version: 1,
    components: [
      {
        component_id: "core", map_target: { kind: "component", component_id: "core" },
        package_participant_ids: [{ kind: "package", value: "opaque-package-id" }],
        symbol_sources: [{
          member_id: { kind: "symbol", value: "core-a" }, symbol: "example.test/project/core.A",
          location: { path: "core/a.go", line: 7, column: 9 },
        }],
      },
      { component_id: "same-name-only", map_target: { kind: "component", component_id: "same-name-only" } },
      {
        component_id: "exact-member-only", map_target: { kind: "component", component_id: "exact-member-only" },
        symbol_sources: [{
          member_id: { kind: "symbol", value: "opaque-symbol-id" }, symbol: "ValidateFunctionURL",
          location: { path: "other.go", line: 41 },
        }],
      },
      {
        component_id: "package-only", map_target: { kind: "component", component_id: "package-only" },
        package_participant_ids: [{ kind: "package", value: "opaque-package-id-3" }],
      },
      { component_id: "anchor-only", map_target: { kind: "component", component_id: "anchor-only" } },
      { component_id: "process-anchor-only", map_target: { kind: "component", component_id: "process-anchor-only" } },
      { component_id: "relation-only", map_target: { kind: "component", component_id: "relation-only" } },
    ],
  },
  discovered_surfaces: {
    total_count: 1,
    triggers: [
      {
        id: "http-get-widgets",
        identity: { method: "GET", path: { text: "/widgets", known: true } },
        handler: { text: "ServeWidget", known: true },
        handler_location: { path: "core/b.go", line: 33, column: 4 },
        registration_site: { path: "core/a.go", line: 15, column: 2 },
        process_entrypoint: { name: "main", location: { path: "core/a.go", line: 7 } },
      },
      {
        id: "http-post-widgets",
        identity: { method: "POST", path: { text: "/widgets", known: true } },
        handler: { text: "CreateWidget", known: true },
        registration_site: { path: "core/a.go", line: 27, column: 6 },
        process_entrypoint: { name: "main", location: { path: "core/a.go", line: 7 } },
      },
      {
        id: "process-main", kind: "process_entry",
        handler: { text: "example.test/project/core.main", known: true },
        registration_site: { path: "core/b.go", line: 99, column: 1 },
        process_entrypoint: { name: "main", location: { path: "core/a.go", line: 7, column: 3 } },
      },
    ],
  },
  repository_graph: { packages: [
    {
      canonical_package_path: "example.test/project/core",
      files: ["core/a.go", "core/b.go"],
    },
    {
      canonical_package_path: "example.test/project/worker",
      files: ["worker/b.go", "worker/a.go"],
    },
  ] },
  study_map: { directions: [
    {
      id: "study-one", question: "How does core work?",
      reading_anchors: [
        { label: "Start here", symbol: "A", location: { path: "core/a.go", line: 11 }, source: snippet("core/a.go", "A", 11) },
        { label: "Then inspect", symbol: "B", location: { path: "core/b.go", line: 21 }, source: snippet("core/b.go", "B", 21) },
      ],
      areas: [{ map_target: { kind: "component", component_id: "core" } }],
    },
    {
      id: "study-other", question: "How does another package work?",
      reading_anchors: [
        { label: "Start here", symbol: "Other", location: { path: "other.go", line: 31 }, source: snippet("other.go", "Other", 31) },
      ],
      areas: [{ map_target: { kind: "component", component_id: "exact-member-only" } }],
    },
    {
      id: "study-shared", question: "How does the shared family work?",
      reading_anchors: [
        { label: "Start here", symbol: "Shared", location: { path: "shared.go", line: 3 }, source: snippet("shared.go", "Shared", 3) },
      ],
      areas: [{ map_target: { kind: "component", component_id: "anchor-only" } }],
    },
    {
      id: "study-worker", question: "How does the worker run?",
      reading_anchors: [
        { label: "Start here", symbol: "Worker", location: { path: "worker/b.go", line: 21 }, source: snippet("worker/b.go", "Worker", 21) },
      ],
      areas: [{ map_target: { kind: "component", component_id: "package-only" } }],
    },
  ] },
  incomplete_study: { directions: [
    {
      id: "incomplete-study-one", question: "How does core work?",
      reading_anchors: [
        { label: "Start here", symbol: "A", location: { path: "core/a.go", line: 11 }, source: snippet("core/a.go", "A", 11) },
      ],
    },
  ] },
};
const window = {
  location: { search: "", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
  __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {},
};
const document = {
  getElementById(id) { return id === "rm-report-data" ? { textContent: JSON.stringify(report) } : null; },
  querySelectorAll() { return []; },
};
document.documentElement = { lang: "en" };
window.document = document;
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), { window });
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window, document, URLSearchParams, Set, Map, AbortController,
});
process.stdout.write(JSON.stringify(window.__REPOMAP_WORKSPACE_TEST__.architectureComponentContexts()));
`
	runnerPath := filepath.Join(t.TempDir(), "architecture-context-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run architecture context projection: %v\n%s", err, output)
	}
	var contexts map[string]struct {
		PackagePaths   []string `json:"package_paths"`
		PackageTargets []struct {
			Path       string `json:"path"`
			Actionable bool   `json:"actionable"`
			Location   struct {
				Path string `json:"path"`
				Line int    `json:"line"`
			} `json:"location"`
		} `json:"package_targets"`
		FileCount int `json:"file_count"`
		Sources   []struct {
			Detail   string `json:"detail"`
			Location struct {
				Path   string `json:"path"`
				Line   int    `json:"line"`
				Column int    `json:"column"`
			} `json:"location"`
		} `json:"sources"`
		SurfaceStarts []struct {
			ID         string `json:"id"`
			Label      string `json:"label"`
			Actionable bool   `json:"actionable"`
			Location   struct {
				Path   string `json:"path"`
				Line   int    `json:"line"`
				Column int    `json:"column"`
			} `json:"location"`
		} `json:"surface_starts"`
		Studies []struct {
			ID string `json:"id"`
		} `json:"studies"`
		StructuralRelations []struct {
			FromLabel string `json:"from_label"`
			ToLabel   string `json:"to_label"`
		} `json:"structural_relations"`
	}
	if err := json.Unmarshal(output, &contexts); err != nil {
		t.Fatalf("decode architecture contexts: %v\n%s", err, output)
	}
	core, ok := contexts["core"]
	if !ok {
		t.Fatalf("exact package component context is absent: %#v", contexts)
	}
	if strings.Join(core.PackagePaths, "|") != "example.test/project/core" || core.FileCount != 2 {
		t.Fatalf("package projection = %#v, file count = %d", core.PackagePaths, core.FileCount)
	}
	if len(core.PackageTargets) != 1 ||
		core.PackageTargets[0].Path != "example.test/project/core" ||
		core.PackageTargets[0].Location.Path != "core/a.go" ||
		core.PackageTargets[0].Location.Line != 1 ||
		core.PackageTargets[0].Actionable {
		t.Fatalf("package targets = %#v", core.PackageTargets)
	}
	if len(core.Sources) != 1 || core.Sources[0].Detail != "example.test/project/core.A" ||
		core.Sources[0].Location.Path != "core/a.go" || core.Sources[0].Location.Line != 7 ||
		core.Sources[0].Location.Column != 9 {
		t.Fatalf("source joins = %#v", core.Sources)
	}
	startsByID := make(map[string]struct {
		Label      string
		Path       string
		Line       int
		Column     int
		Actionable bool
	})
	for _, start := range core.SurfaceStarts {
		startsByID[start.ID] = struct {
			Label      string
			Path       string
			Line       int
			Column     int
			Actionable bool
		}{start.Label, start.Location.Path, start.Location.Line, start.Location.Column, start.Actionable}
	}
	if len(core.SurfaceStarts) != 3 ||
		startsByID["http-get-widgets"].Label != "GET /widgets → ServeWidget" ||
		startsByID["http-get-widgets"].Path != "core/b.go" ||
		startsByID["http-get-widgets"].Line != 33 ||
		startsByID["http-get-widgets"].Column != 4 ||
		startsByID["http-get-widgets"].Actionable ||
		startsByID["http-post-widgets"].Label != "POST /widgets → CreateWidget · registration" ||
		startsByID["http-post-widgets"].Path != "core/a.go" ||
		startsByID["http-post-widgets"].Line != 27 ||
		startsByID["http-post-widgets"].Column != 6 ||
		startsByID["http-post-widgets"].Actionable ||
		startsByID["process-main"].Label != "main · process entry" ||
		startsByID["process-main"].Path != "core/a.go" ||
		startsByID["process-main"].Line != 7 ||
		startsByID["process-main"].Column != 3 ||
		startsByID["process-main"].Actionable {
		t.Fatalf("surface starts = %#v", core.SurfaceStarts)
	}
	if len(core.Studies) != 1 || core.Studies[0].ID != "study-one" {
		t.Fatalf("Study joins = %#v", core.Studies)
	}
	if sameName, ok := contexts["same-name-only"]; !ok || len(sameName.Sources) != 0 || len(sameName.Studies) != 0 {
		t.Fatalf("source-less conceptual component navigation = %#v, present %v", sameName, ok)
	}
	memberOnly, ok := contexts["exact-member-only"]
	if !ok {
		t.Fatalf("exact member-only component context is absent: %#v", contexts)
	}
	if len(memberOnly.PackagePaths) != 0 || memberOnly.FileCount != 1 ||
		len(memberOnly.Sources) != 1 ||
		memberOnly.Sources[0].Location.Path != "other.go" ||
		memberOnly.Sources[0].Location.Line != 41 ||
		len(memberOnly.Studies) != 1 || memberOnly.Studies[0].ID != "study-other" {
		t.Fatalf("exact member-only context = %#v", memberOnly)
	}
	packageOnly, ok := contexts["package-only"]
	if !ok || strings.Join(packageOnly.PackagePaths, "|") != "example.test/project/worker" ||
		packageOnly.FileCount != 2 || len(packageOnly.Sources) != 0 ||
		len(packageOnly.PackageTargets) != 1 ||
		packageOnly.PackageTargets[0].Path != "example.test/project/worker" ||
		packageOnly.PackageTargets[0].Location.Path != "worker/a.go" ||
		packageOnly.PackageTargets[0].Location.Line != 1 ||
		packageOnly.PackageTargets[0].Actionable ||
		len(packageOnly.Studies) != 1 || packageOnly.Studies[0].ID != "study-worker" {
		t.Fatalf("package-only context = %#v, present %v", packageOnly, ok)
	}
	anchorOnly, ok := contexts["anchor-only"]
	if !ok || anchorOnly.FileCount != 2 || len(anchorOnly.Sources) != 0 ||
		len(anchorOnly.Studies) != 1 || anchorOnly.Studies[0].ID != "study-shared" {
		t.Fatalf("anchor-only context = %#v, present %v", anchorOnly, ok)
	}
	processAnchorOnly, ok := contexts["process-anchor-only"]
	if !ok || len(processAnchorOnly.SurfaceStarts) != 1 ||
		processAnchorOnly.SurfaceStarts[0].ID != "process-anchor-only" ||
		processAnchorOnly.SurfaceStarts[0].Label != "Process entry worker" ||
		processAnchorOnly.SurfaceStarts[0].Location.Path != "shared2.go" ||
		processAnchorOnly.SurfaceStarts[0].Location.Line != 12 {
		t.Fatalf("exact process anchor fallback = %#v, present %v", processAnchorOnly, ok)
	}
	relationOnly, ok := contexts["relation-only"]
	if !ok || len(relationOnly.StructuralRelations) != 1 ||
		relationOnly.StructuralRelations[0].FromLabel != "Request dispatcher" ||
		relationOnly.StructuralRelations[0].ToLabel != "ValidateFunctionURL" {
		t.Fatalf("relation-only context lost exact named relation: %#v, present %v", relationOnly, ok)
	}
}

func TestSourceLocationActionAvailabilityMatchesReportAuthority(t *testing.T) {
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
const script = fs.readFileSync(process.argv[2], "utf8");
const messages = fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8");
const revision = "0123456789abcdef0123456789abcdef01234567";
function available(location, report, browserLocation) {
  const window = {
    location: browserLocation,
    __REPOMAP_WORKSPACE_TEST__: {},
    addEventListener() {},
  };
  const document = {
    getElementById(id) { return id === "rm-report-data" ? { textContent: JSON.stringify(report) } : null; },
    querySelectorAll() { return []; },
  };
  document.documentElement = { lang: "en" };
  window.document = document;
  vm.runInNewContext(messages, { window });
  vm.runInNewContext(script, {
    window, document, URLSearchParams, Set, Map, AbortController,
  });
  return window.__REPOMAP_WORKSPACE_TEST__.sourceLocationActionAvailable(location);
}
const base = {
  user_mechanisms: [], user_sources: [],
  openable_paths: ["pkg/file.go"], source_ids: {},
};
process.stdout.write(JSON.stringify({
  staticFile: available(
    {path: "pkg/file.go", line: 8}, base,
    {search: "", hostname: "", protocol: "file:", pathname: "/report.html"}
  ),
  localServer: available(
    {path: "pkg/file.go", line: 8},
    {...base, source_ids: {"pkg/file.go": "opaque-source"}},
    {
      search: "", hostname: "127.0.0.1", protocol: "http:",
      pathname: "/_repomap/token/runs/run/report.html",
    }
  ),
  cleanGitLab: available(
    {path: "pkg/file.go", line: 8},
    {...base, gitlab_source_links: {
      repository_url: "https://gitlab.example/team/project", revision,
      working_tree_paths: [],
    }},
    {search: "", hostname: "", protocol: "file:", pathname: "/report.html"}
  ),
  dirtyGitLab: available(
    {path: "pkg/file.go", line: 8},
    {...base, gitlab_source_links: {
      repository_url: "https://gitlab.example/team/project", revision,
      working_tree_dirty: true, working_tree_paths: ["pkg/file.go"],
    }},
    {search: "", hostname: "", protocol: "file:", pathname: "/report.html"}
  ),
}));
`
	runnerPath := filepath.Join(t.TempDir(), "source-authority-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run source authority projection: %v\n%s", err, output)
	}
	var got struct {
		StaticFile  bool `json:"staticFile"`
		LocalServer bool `json:"localServer"`
		CleanGitLab bool `json:"cleanGitLab"`
		DirtyGitLab bool `json:"dirtyGitLab"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode source authority projection: %v\n%s", err, output)
	}
	if got.StaticFile || !got.LocalServer || !got.CleanGitLab || got.DirtyGitLab {
		t.Fatalf("source authority projection = %#v", got)
	}
}

func TestManifestServerSourceActionRemainsAuthorizedEditorRequest(t *testing.T) {
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
const snippet = {
  path: "pkg/file.go", start_line: 7, end_line: 9,
  lines: [{ line: 8, text: "work()", highlight: true }],
};
const report = {
  user_mechanisms: [], user_sources: [snippet],
  openable_paths: ["pkg/file.go"],
  source_ids: { "pkg/file.go": "opaque-source-id" },
};
const fetchCalls = [];
const fetch = (url, options) => {
  fetchCalls.push({ url, options });
  return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
};
const window = {
  location: {
    hash: "#/map", search: "", hostname: "127.0.0.1", protocol: "http:",
    pathname: "/_repomap/session-token/runs/run-1/report.html",
  },
  history: { state: null, pushState() {}, replaceState() {} },
  __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {},
  setTimeout() { return 1; }, clearTimeout() {},
};
const document = {
  documentElement: { lang: "en" },
  getElementById(id) { return id === "rm-report-data" ? { textContent: JSON.stringify(report) } : null; },
  querySelector() { return null; }, querySelectorAll() { return []; },
};
window.document = document;
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), { window });
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window, document, fetch, URLSearchParams, Set, Map, AbortController, Promise,
});
const api = window.__REPOMAP_WORKSPACE_TEST__;
api.openSourceLocation({ path: "pkg/file.go", line: 8, column: 3 });
const call = fetchCalls[0] || {};
const body = call.options ? JSON.parse(call.options.body) : null;
process.stdout.write(JSON.stringify({
  serverMode: api.serverMode(),
  fetchCount: fetchCalls.length,
  url: call.url,
  action: call.options && call.options.headers["X-Repomap-Action"],
  body,
  sourceLocation: api.workspaceStateSnapshot().sourceLocation,
}));
`
	runnerPath := filepath.Join(t.TempDir(), "manifest-server-source-action-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run manifest-server source action: %v\n%s", err, output)
	}
	var got struct {
		ServerMode     bool   `json:"serverMode"`
		FetchCount     int    `json:"fetchCount"`
		URL            string `json:"url"`
		Action         string `json:"action"`
		SourceLocation any    `json:"sourceLocation"`
		Body           struct {
			RunID    string `json:"run_id"`
			SourceID string `json:"source_id"`
			Line     int    `json:"line"`
			Column   int    `json:"column"`
		} `json:"body"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode manifest-server source action: %v\n%s", err, output)
	}
	if !got.ServerMode || got.FetchCount != 1 ||
		got.URL != "/_repomap/session-token/api/open" || got.Action != "open-file" ||
		got.Body.RunID != "run-1" || got.Body.SourceID != "opaque-source-id" ||
		got.Body.Line != 8 || got.Body.Column != 3 || got.SourceLocation != nil {
		t.Fatalf("manifest-server source action = %#v", got)
	}
}

func TestArchitectureAvailabilityDependsOnlyOnCanonicalCanvas(t *testing.T) {
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
const script = fs.readFileSync(process.argv[2], "utf8");
const messages = fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8");
function available(architectureSynthesis, search) {
  const report = {
    user_mechanisms: [], user_sources: [], openable_paths: [], source_ids: {},
    architecture_canvas: { components: [{ id: "core" }] },
    architecture_synthesis: architectureSynthesis,
  };
  const window = {
    location: { search: search || "", hash: "#/overview", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
    __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {},
  };
  const document = {
    getElementById(id) { return id === "rm-report-data" ? { textContent: JSON.stringify(report) } : null; },
    querySelectorAll() { return []; },
  };
  document.documentElement = { lang: "en" };
  window.document = document;
  vm.runInNewContext(messages, { window });
  vm.runInNewContext(script, { window, document, URLSearchParams, Set, Map, AbortController });
  return window.__REPOMAP_WORKSPACE_TEST__.userArchitectureAvailable();
}
process.stdout.write(JSON.stringify({
  accepted: available({ state: "succeeded", proposal_accepted: true }, ""),
  rejected: available({ state: "succeeded", proposal_rejected: true, fallback_selected: true }, ""),
  failed: available({ state: "failed" }, ""),
  diagnostic: available({ state: "succeeded", proposal_rejected: true, fallback_selected: true }, "?debug=1"),
  localOnly: available(null, ""),
}));
`
	runnerPath := filepath.Join(t.TempDir(), "architecture-publication-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run architecture publication contract: %v\n%s", err, output)
	}
	var got struct {
		Accepted   bool `json:"accepted"`
		Rejected   bool `json:"rejected"`
		Failed     bool `json:"failed"`
		Diagnostic bool `json:"diagnostic"`
		LocalOnly  bool `json:"localOnly"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode architecture publication contract: %v\n%s", err, output)
	}
	if !got.Accepted || !got.Rejected || !got.Failed || !got.Diagnostic || !got.LocalOnly {
		t.Fatalf("architecture publication contract = %#v", got)
	}
}

func TestArchitecturePackageOnlyComponentReachesDedupedExactSourceInTwoClicks(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	canvasPath, err := filepath.Abs(filepath.Join("templates", "architecture_canvas.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs");
const vm = require("vm");
class Element {
  constructor(tag) {
    this.tagName = String(tag || "div").toUpperCase();
    this.children = [];
    this.attributes = {};
    this.className = "";
    this.textContent = "";
    this.hidden = false;
    this.style = {};
    this.dataset = {};
    this.parentNode = null;
    this.listeners = {};
    this.clientWidth = 960;
    this.clientHeight = 640;
    this.classList = {
      add: (...names) => {
        const values = new Set(String(this.className).split(/\s+/).filter(Boolean));
        names.forEach((name) => values.add(name));
        this.className = Array.from(values).join(" ");
      },
      remove: (...names) => {
        const removed = new Set(names);
        this.className = String(this.className).split(/\s+/).filter((name) => name && !removed.has(name)).join(" ");
      },
      toggle: (name, force) => {
        const values = new Set(String(this.className).split(/\s+/).filter(Boolean));
        const enabled = force === undefined ? !values.has(name) : !!force;
        if (enabled) values.add(name); else values.delete(name);
        this.className = Array.from(values).join(" ");
        return enabled;
      },
      contains: (name) => String(this.className).split(/\s+/).includes(name),
    };
  }
  get childNodes() { return this.children; }
  appendChild(child) { if (child) { child.parentNode = this; this.children.push(child); } return child; }
  append(...children) { children.forEach((child) => this.appendChild(child)); }
  prepend(child) { if (child) { child.parentNode = this; this.children.unshift(child); } }
  replaceChildren(...children) { this.children = []; this.textContent = ""; this.append(...children); }
  remove() {
    if (!this.parentNode) return;
    this.parentNode.children = this.parentNode.children.filter((child) => child !== this);
    this.parentNode = null;
  }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return this.attributes[name] == null ? null : this.attributes[name]; }
  removeAttribute(name) { delete this.attributes[name]; }
  addEventListener(type, handler) {
    if (!this.listeners[type]) this.listeners[type] = [];
    this.listeners[type].push(handler);
  }
  removeEventListener() {}
  click() {
    (this.listeners.click || []).forEach((handler) => handler({
      currentTarget: this, target: this, stopPropagation() {}, preventDefault() {},
    }));
  }
  focus() { document.activeElement = this; }
  contains(candidate) {
    if (candidate === this) return true;
    return this.children.some((child) => child && typeof child.contains === "function" && child.contains(candidate));
  }
  getBoundingClientRect() { return { left: 0, top: 0, right: 300, bottom: 180, width: 300, height: 180 }; }
  querySelector() { return null; }
  querySelectorAll() { return []; }
  scrollIntoView() {}
}
function walk(root) {
  const result = [];
  (function visit(node) { if (!node) return; result.push(node); (node.children || []).forEach(visit); })(root);
  return result;
}
function nodeText(root) { return walk(root).map((node) => String(node.textContent || "")).join(""); }
const document = {
  createElement: (tag) => new Element(tag),
  createElementNS: (_ns, tag) => new Element(tag),
  createTextNode(value) { const node = new Element("#text"); node.textContent = String(value); return node; },
  getElementById: () => null,
  querySelector: () => null,
  querySelectorAll: () => [],
  addEventListener() {}, removeEventListener() {},
  body: new Element("body"), documentElement: new Element("html"),
};
document.activeElement = document.body;
const window = {
  document, AbortController, Set, Map, URLSearchParams, Promise,
  requestAnimationFrame: (callback) => callback(),
  clearTimeout, setTimeout, innerWidth: 1440, innerHeight: 1000,
  addEventListener() {}, removeEventListener() {},
  RepomapUI: { message(id) { return id; } },
};
const sandbox = {
  window, document, Element, AbortController, Set, Map, URLSearchParams, Promise,
  requestAnimationFrame: (callback) => callback(), clearTimeout, setTimeout, console,
  addEventListener() {}, removeEventListener() {},
};
sandbox.global = sandbox;
vm.createContext(sandbox);
vm.runInContext(fs.readFileSync(process.argv[2], "utf8"), sandbox);
const host = new Element("div");
const data = {
  components: [{
    id: "package-only", name: "Worker package",
    members: [{ id: { kind: "package", value: "opaque-package" }, name: "worker" }],
  }],
  subsystems: [{ id: "application", name: "Application", component_ids: ["package-only"] }],
  groups: [], structural_edges: [], behavior_anchors: [], relations: [], surfaces: [], flows: [],
};
const opened = [];
const exactTarget = {
  path: "example.test/project/worker", file_count: 2,
  location: { path: "worker/a.go", line: 1, column: 0 }, actionable: true,
};
const app = window.RepomapArchitectureCanvas.mount(host, data, {
  userMode: true,
  message: (id) => id,
  openSourceLocation: (location) => opened.push(location),
  componentContexts: {
    "package-only": {
      package_paths: ["example.test/project/worker"],
      package_targets: [exactTarget, { ...exactTarget }],
      sources: [], surface_starts: [], studies: [], structural_relations: [],
      member_count: 1, authority: "validated", evidence_composition: "package",
    },
  },
});
app.ready.then(() => {
  app.openComponent("package-only"); // first click: cube/component
  const sourceButtons = walk(host).filter((node) =>
    node.tagName === "BUTTON" && String(node.className).split(/\s+/).includes("rm-arch__compact-action") &&
    nodeText(node).includes("example.test/project/worker"));
  if (sourceButtons[0]) sourceButtons[0].click(); // second click: exact source
  process.stdout.write(JSON.stringify({
    sourceButtonCount: sourceButtons.length,
    opened,
    sourceButtonText: sourceButtons[0] ? nodeText(sourceButtons[0]) : "",
  }));
}).catch((error) => {
  process.stdout.write(JSON.stringify({ mountError: String(error && error.stack || error) }));
  process.exit(2);
});
`
	runnerPath := filepath.Join(t.TempDir(), "architecture-package-target-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, canvasPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run package-only Architecture source journey: %v\n%s", err, output)
	}
	var got struct {
		SourceButtonCount int `json:"sourceButtonCount"`
		Opened            []struct {
			Path   string `json:"path"`
			Line   int    `json:"line"`
			Column int    `json:"column"`
		} `json:"opened"`
		SourceButtonText string `json:"sourceButtonText"`
		MountError       string `json:"mountError"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode package-only Architecture source journey: %v\n%s", err, output)
	}
	if got.MountError != "" {
		t.Fatalf("canvas mount failed: %s", got.MountError)
	}
	if got.SourceButtonCount != 1 || len(got.Opened) != 1 ||
		got.Opened[0].Path != "worker/a.go" || got.Opened[0].Line != 1 ||
		got.Opened[0].Column != 0 ||
		!strings.Contains(got.SourceButtonText, "example.test/project/worker") {
		t.Fatalf("package-only two-click source action = %#v", got)
	}
}

func TestArchitectureUserProjectionRetainsOnlyExactPartialTruth(t *testing.T) {
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
const script = fs.readFileSync(process.argv[2], "utf8");
const messages = fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8");
function project(canvas) {
  const report = {
    user_mechanisms: [], user_sources: [], openable_paths: [], source_ids: {},
    architecture_canvas: canvas,
  };
  const window = {
    location: { search: "", hash: "#/overview", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
    __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {},
  };
  const document = {
    getElementById(id) { return id === "rm-report-data" ? { textContent: JSON.stringify(report) } : null; },
    querySelectorAll() { return []; },
  };
  document.documentElement = { lang: "en" };
  window.document = document;
  vm.runInNewContext(messages, { window });
  vm.runInNewContext(script, { window, document, URLSearchParams, Set, Map, AbortController });
  return window.__REPOMAP_WORKSPACE_TEST__.userArchitectureData();
}
const partial = project({
  validation_outcome: "accepted_partial",
  architecture_source: "partial_model",
  local_remainder_component_id: "remainder",
  diagnostics: [{ code: "generic-noise" }],
  components: [{ id: "remainder", members: [
    { name: "cmd/server/main.go" },
    { id: { kind: "package", value: "example.test/internal/local" } },
  ] }],
});
const full = project({
  validation_outcome: "accepted",
  local_remainder_component_id: "stale-remainder",
  components: [{ id: "core", members: [] }],
});
process.stdout.write(JSON.stringify({
  partialOutcome: partial.validation_outcome,
  partialRemainder: partial.local_remainder_component_id,
  partialMembers: partial.components[0].members,
  partialHasDiagnostics: Object.prototype.hasOwnProperty.call(partial, "diagnostics"),
  partialHasSource: Object.prototype.hasOwnProperty.call(partial, "architecture_source"),
  fullHasRemainder: Object.prototype.hasOwnProperty.call(full, "local_remainder_component_id"),
}));
`
	runnerPath := filepath.Join(t.TempDir(), "architecture-partial-user-projection-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run partial Architecture user projection: %v\n%s", err, output)
	}
	var got struct {
		PartialOutcome        string `json:"partialOutcome"`
		PartialRemainder      string `json:"partialRemainder"`
		PartialMembers        []any  `json:"partialMembers"`
		PartialHasDiagnostics bool   `json:"partialHasDiagnostics"`
		PartialHasSource      bool   `json:"partialHasSource"`
		FullHasRemainder      bool   `json:"fullHasRemainder"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode partial Architecture user projection: %v\n%s", err, output)
	}
	if got.PartialOutcome != "accepted_partial" || got.PartialRemainder != "remainder" ||
		len(got.PartialMembers) != 2 || got.PartialHasDiagnostics || got.PartialHasSource ||
		got.FullHasRemainder {
		t.Fatalf("partial Architecture user projection = %#v", got)
	}
}

func TestArchitectureUserInspectorStaysCompactAndSourceBacked(t *testing.T) {
	js := readCanvasAsset(t, "architecture_canvas.js")
	css := readCanvasAsset(t, "architecture_canvas.css")
	reportJS := readCanvasAsset(t, "script.js")

	for _, token := range []string{
		"architecturePackagePathForMember(member, packageByPath)",
		"ARCHITECTURE_COMPONENT_NAVIGATION.components",
		"options.componentContexts = architectureComponentContexts()",
		"message: msg",
		"this.msg(",
		"userComponentActions(component)",
		"array(context.sources).forEach(appendSource)",
		"array(context.package_targets).forEach((target)",
		"sourceLocations.has(key)",
		"array(context.studies).slice(0, 3)",
		"package_targets: packageTargets",
		"surface_starts: surfaceStarts",
		"function sourceLocationActionAvailable(location)",
		"rm-arch__compact-action",
		`this.inspectorSection(this.msg("architecture.section.how_work_enters"))`,
		"this.msg(\"architecture.action.open_launch_point\")",
		"array(context.package_paths).length > 0",
		"(component.members || []).forEach(function (member)",
		".concat(component.participating_surface_ids || [])",
		".concat(array(component && component.participating_surface_ids))",
		"(component.anchor_ids || []).forEach(function (anchorID)",
		"String(anchor.kind || '') !== 'process_entry'",
		"var handlerLocation = trigger.handler_location",
		"trigger.registration_site",
		"surfaceName + ' → ' + handlerName",
		"detail: symbol",
		"lowInformationComponent",
		"has-user-compact-inspector",
		// Decision 229 D1: all nine inspector questions answer or state a
		// truthful empty explanation — no silent section omissions.
		`this.msg("architecture.copy.no_observed_entry")`,
		`this.msg("architecture.copy.no_observed_sources")`,
		`this.msg("architecture.copy.no_observed_used_by")`,
		`this.msg("architecture.copy.no_observed_uses")`,
		`this.msg("architecture.copy.no_observed_studies")`,
		`this.msg("architecture.copy.no_observed_callsites")`,
		"rm-arch__inspector-empty",
		"min-width: 0",
		"overflow-wrap: anywhere",
		".rm-arch__compact-action strong",
		"word-break: break-word",
	} {
		if !strings.Contains(reportJS+js+css, token) {
			t.Errorf("compact architecture inspector is missing %q", token)
		}
	}
	for _, forbidden := range []string{
		"array(context.sources).find((candidate)",
	} {
		if strings.Contains(reportJS+js, forbidden) {
			t.Errorf("compact architecture inspector retains choose-first source logic %q", forbidden)
		}
	}

	start := strings.Index(js, "  inspectUserComponent(component) {")
	end := strings.Index(js, "  inspectUserSurface(surface) {")
	if start < 0 || end <= start {
		t.Fatal("cannot isolate user component inspector")
	}
	componentInspector := js[start:end]
	// P7-B: package rows resolve to a deterministic representative file
	// (the package has no single line); this lives in the component
	// CONTEXTS builder, never in the inspector's per-source rendering.
	if strings.Contains(componentInspector, "packageFiles[0]") {
		t.Errorf("compact architecture inspector retains choose-first source logic %q", "packageFiles[0]")
	}
	for _, forbidden := range []string{"participating_flow_ids", "Mechanism", "Paved", "Runtime surfaces"} {
		if strings.Contains(componentInspector, forbidden) {
			t.Errorf("component inspector infers or expands an unsupported relation through %q", forbidden)
		}
	}
}
