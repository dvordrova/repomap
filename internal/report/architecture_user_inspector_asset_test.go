package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchitectureComponentContextsUseExactPackageFileStudyJoin(t *testing.T) {
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
  openable_paths: ["core/a.go", "core/b.go", "other.go", "worker/a.go", "worker/b.go"],
  architecture_canvas: {
    components: [
      { id: "core", members: [
        {
          id: { kind: "package", value: "opaque-package-id" },
          facts: [{ kind: "declaration", value: "example.test/project/core" }],
        },
        {
          name: "example.test/project/core.A",
          facts: [{ kind: "declaration", value: "example.test/project/core.A", location: { path: "core/a.go", line: 7 } }],
        },
        {
          name: "core/a.go",
          facts: [{ kind: "repository_path", value: "core/a.go", location: { path: "core/a.go" } }],
        },
      ] },
      { id: "same-name-only", members: [{
        id: { kind: "package", value: "opaque-package-id-2" }, name: "core", facts: [],
      }] },
      { id: "exact-member-only", members: [{
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
        { label: "Start here", location: { path: "core/a.go", line: 11 }, source: snippet("core/a.go", "A", 11) },
        { label: "Then inspect", location: { path: "core/b.go", line: 21 }, source: snippet("core/b.go", "B", 21) },
      ],
    },
    {
      id: "study-other", question: "How does another package work?",
      reading_anchors: [
        { label: "Start here", location: { path: "other.go", line: 31 }, source: snippet("other.go", "Other", 31) },
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
		PackagePaths []string `json:"package_paths"`
		FileCount    int      `json:"file_count"`
		Sources      []struct {
			Detail   string `json:"detail"`
			Location struct {
				Path string `json:"path"`
				Line int    `json:"line"`
			} `json:"location"`
		} `json:"sources"`
		Studies []struct {
			ID string `json:"id"`
		} `json:"studies"`
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
	if len(core.Sources) != 4 || core.Sources[0].Detail != "example.test/project/core.A" ||
		core.Sources[0].Location.Path != "core/a.go" || core.Sources[0].Location.Line != 7 ||
		core.Sources[1].Location.Path != "core/a.go" || core.Sources[1].Location.Line != 0 ||
		core.Sources[2].Location.Path != "core/a.go" || core.Sources[3].Location.Path != "core/b.go" {
		t.Fatalf("source joins = %#v", core.Sources)
	}
	if len(core.Studies) != 1 || core.Studies[0].ID != "study-one" {
		t.Fatalf("Study joins = %#v", core.Studies)
	}
	if _, ok := contexts["same-name-only"]; ok {
		t.Fatalf("package-name similarity created a non-exact join: %#v", contexts["same-name-only"])
	}
	memberOnly, ok := contexts["exact-member-only"]
	if !ok {
		t.Fatalf("exact member-only component context is absent: %#v", contexts)
	}
	if len(memberOnly.PackagePaths) != 0 || memberOnly.FileCount != 1 ||
		len(memberOnly.Sources) != 2 ||
		memberOnly.Sources[0].Location.Path != "other.go" ||
		memberOnly.Sources[0].Location.Line != 41 ||
		len(memberOnly.Studies) != 1 || memberOnly.Studies[0].ID != "study-other" {
		t.Fatalf("exact member-only context = %#v", memberOnly)
	}
	packageOnly, ok := contexts["package-only"]
	if !ok || strings.Join(packageOnly.PackagePaths, "|") != "example.test/project/worker" ||
		packageOnly.FileCount != 2 || len(packageOnly.Sources) != 1 ||
		packageOnly.Sources[0].Location.Path != "worker/a.go" ||
		packageOnly.Sources[0].Location.Line != 0 {
		t.Fatalf("package-only context = %#v, present %v", packageOnly, ok)
	}
}

func TestRejectedArchitectureFallbackIsDiagnosticOnly(t *testing.T) {
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
	if !got.Accepted || got.Rejected || got.Failed || !got.Diagnostic || !got.LocalOnly {
		t.Fatalf("architecture publication contract = %#v", got)
	}
}

func TestArchitectureUserInspectorStaysCompactAndSourceBacked(t *testing.T) {
	js := readCanvasAsset(t, "architecture_canvas.js")
	css := readCanvasAsset(t, "architecture_canvas.css")
	reportJS := readCanvasAsset(t, "script.js")

	for _, token := range []string{
		"architecturePackagePathForMember(member, packageByPath)",
		"componentFiles[String(location.path || '')]",
		"options.componentContexts = architectureComponentContexts()",
		"translateUI: translateUIString",
		"this.translateUI(",
		"userComponentActions(component)",
		"return actions.slice(0, 3)",
		"array(context.package_paths).length > 0",
		"(component.members || []).forEach(function (member)",
		"detail: member.name || filePath",
		"lowInformationComponent",
		"has-user-compact-inspector",
	} {
		if !strings.Contains(reportJS+js+css, token) {
			t.Errorf("compact architecture inspector is missing %q", token)
		}
	}

	start := strings.Index(js, "  inspectUserComponent(component) {")
	end := strings.Index(js, "  inspectUserSurface(surface) {")
	if start < 0 || end <= start {
		t.Fatal("cannot isolate user component inspector")
	}
	componentInspector := js[start:end]
	for _, forbidden := range []string{"participating_flow_ids", "Mechanism", "Paved", "Runtime surfaces"} {
		if strings.Contains(componentInspector, forbidden) {
			t.Errorf("component inspector infers or expands an unsupported relation through %q", forbidden)
		}
	}
}
