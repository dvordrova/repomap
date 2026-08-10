package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalysisTargetOwnsPersistentWorkspacePurpose(t *testing.T) {
	template, err := os.ReadFile(filepath.Join("templates", "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	markup := string(template)
	if strings.Count(markup, `id="rm-workspace-purpose"`) != 1 {
		t.Fatal("workspace target scope must have one persistent header owner")
	}
	if strings.Index(markup, `id="rm-workspace-purpose"`) > strings.Index(markup, `class="rm-workspace"`) {
		t.Fatal("workspace target scope is inside one routed Map/Study view instead of the persistent header")
	}
	if !strings.Contains(markup, `data-rm-aria-message="main.analysis_targets.navigation"`) ||
		!strings.Contains(markup, `id="rm-architecture" class="rm-tab-content rm-active"`) ||
		strings.Contains(markup, `id="rm-study-overview"`) || strings.Contains(markup, `id="rm-study-detail"`) {
		t.Fatal("report template must expose one persistent target rail and one Canvas→Study target page")
	}
	cssBytes, err := os.ReadFile(filepath.Join("templates", "style.css"))
	if err != nil {
		t.Fatal(err)
	}
	css := string(cssBytes)
	for _, token := range []string{
		`.rm-target-link__default-dot { background: #16a34a`,
		`.rm-repository-nav .rm-tab.rm-active { background: #eff6ff; border-left-color: var(--rm-accent)`,
		`.rm-repository-nav .rm-target-link { max-width: 100%; min-width: 0; white-space: normal; }`,
		`.rm-source-action-link { align-items: center; box-sizing: border-box; display: inline-flex; gap: .35rem;`,
		`.rm-study-reading-anchor__open--plain { align-items: baseline; display: inline-flex; flex-wrap: wrap; gap: .35rem; }`,
		`.rm-workspace, .rm-workspace.has-source-drawer { display: grid; grid-template-columns: clamp(104px, 30vw, 160px)`,
	} {
		if !strings.Contains(css, token) {
			t.Errorf("persistent target rail CSS missing %q", token)
		}
	}

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	scriptPath, err := filepath.Abs(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs"), vm = require("vm");
const scriptPath = process.argv[2];
const messagesPath = scriptPath.replace("script.js", "ui_messages.js");
function purpose(language, target) {
  const report = { repo_name: "repo", user_sources: [], openable_paths: [], source_ids: {} };
  if (target) report.analysis_target = target;
  const document = {
    documentElement: { lang: language },
    getElementById(id) { return id === "rm-report-data" ? { textContent: JSON.stringify(report) } : null; },
    querySelector() { return null; }, querySelectorAll() { return []; },
    addEventListener() {}, removeEventListener() {},
  };
  const window = {
    document,
    location: { search: "", hash: "#/map", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
    __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {}, removeEventListener() {},
  };
  const context = { window, document, URLSearchParams, Set, Map, AbortController, Promise };
  vm.createContext(context);
  vm.runInContext(fs.readFileSync(messagesPath, "utf8"), context);
  vm.runInContext(fs.readFileSync(scriptPath, "utf8"), context);
  return window.__REPOMAP_WORKSPACE_TEST__.workspacePurposeText();
}
const executable = { version: 2, kind: "executable_package", package_dir: "cmd/repomap", package_path: "github.com/dvordrova/repomap/cmd/repomap" };
const rootLibrary = { version: 2, kind: "module_library", module_dir: ".", module_path: "gopkg.in/telebot.v3" };
process.stdout.write(JSON.stringify({
  enExecutable: purpose("en", executable), ruExecutable: purpose("ru", executable),
  enLibrary: purpose("en", rootLibrary), ruLibrary: purpose("ru", rootLibrary),
  enNonGo: purpose("en", null), ruNonGo: purpose("ru", null),
}));
`
	runnerPath := filepath.Join(t.TempDir(), "analysis-target-workspace.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, scriptPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run analysis target workspace asset: %v\n%s", err, output)
	}
	var got map[string]string
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode analysis target workspace asset: %v\n%s", err, output)
	}
	want := map[string]string{
		"enExecutable": "Report scope: executable package cmd/repomap",
		"ruExecutable": "Область отчёта: исполняемый пакет cmd/repomap",
		"enLibrary":    "Report scope: public library API of module gopkg.in/telebot.v3",
		"ruLibrary":    "Область отчёта: публичный библиотечный API модуля gopkg.in/telebot.v3",
		"enNonGo":      "Repository onboarding",
		"ruNonGo":      "Знакомство с репозиторием",
	}
	for name, expected := range want {
		if got[name] != expected {
			t.Errorf("%s = %q, want %q", name, got[name], expected)
		}
	}
}

func TestSingleTargetRailAndExecutableStartsOnNeutralMap(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	scriptPath, err := filepath.Abs(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs"), vm = require("vm");
const target = {
  version: 2, ref: "target-ref", kind: "executable_package", module_dir: ".",
  package_dir: ".", package_path: "github.com/casdoor/casdoor",
  roots: [{path:"main_linux.go",line:12},{path:"main_windows.go",line:15}],
};
const report = {
  repo_name: "casdoor", report_language: "en", analysis_target: target,
  user_sources: [], openable_paths: [], source_ids: {},
  architecture_canvas: { entry_handoff_groups: [
    {id:"windows",entry:{path:"main_windows.go",line:15}},
    {id:"linux",entry:{path:"main_linux.go",line:12}},
  ]},
};
const document = {
  documentElement: {lang:"en"},
  getElementById(id) { return id === "rm-report-data" ? {textContent:JSON.stringify(report)} : null; },
  querySelector() { return null; }, querySelectorAll() { return []; },
};
const window = {document, location:{search:"",hash:"#canvas",protocol:"file:",pathname:"/report.html"}, __REPOMAP_WORKSPACE_TEST__:{}, addEventListener(){}};
const context = {window,document,URLSearchParams,Set,Map,AbortController,Promise};
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js","ui_messages.js"),"utf8"),context);
vm.runInNewContext(fs.readFileSync(process.argv[2],"utf8"),context);
const api = window.__REPOMAP_WORKSPACE_TEST__;
process.stdout.write(JSON.stringify({items:api.analysisTargetMenuItems(),defaultGroup:api.defaultEntrypointHandoffGroupID()}));
`
	runnerPath := filepath.Join(t.TempDir(), "single-target-rail.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, scriptPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run single-target rail contract: %v\n%s", err, output)
	}
	var got struct {
		Items []struct {
			Ref, Label, Title, GoMod string
			IsDefault, IsActive      bool
		}
		DefaultGroup string `json:"defaultGroup"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode single-target rail contract: %v\n%s", err, output)
	}
	if len(got.Items) != 1 || got.Items[0].Ref != "target-ref" ||
		got.Items[0].Label != "casdoor" || got.Items[0].Title != "github.com/casdoor/casdoor" || got.Items[0].GoMod != "go.mod" ||
		!got.Items[0].IsDefault || !got.Items[0].IsActive {
		t.Fatalf("single target rail = %#v", got.Items)
	}
	if got.DefaultGroup != "" {
		t.Fatalf("executable preselected entry handoff group = %q, want none", got.DefaultGroup)
	}
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(script), "function renderMapModeControl") ||
		!strings.Contains(string(script), "entryObjects.id = 'rm-map-lens-objects'") {
		t.Fatal("executable Map must expose explicit mode controls and one contextual objects host")
	}
}

func TestSingleModuleLibraryTargetRailUsesLocalizedLibraryAPILabel(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	scriptPath, err := filepath.Abs(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs"), vm = require("vm");
function items(language) {
  const report = {
    repo_name:"telebot", report_language:language,
    analysis_target:{version:2,ref:"module-library",kind:"module_library",module_id:"root",module_path:"gopkg.in/telebot.v3",module_dir:"."},
    user_sources:[],openable_paths:[],source_ids:{},
  };
  const document = {
    documentElement:{lang:language},
    getElementById(id){return id === "rm-report-data" ? {textContent:JSON.stringify(report)} : null;},
    querySelector(){return null;},querySelectorAll(){return[];},
  };
  const window={document,location:{search:"",hash:"#/map",protocol:"file:",pathname:"/report.html"},__REPOMAP_WORKSPACE_TEST__:{},addEventListener(){}};
  const context={window,document,URLSearchParams,Set,Map,AbortController,Promise};
  vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js","ui_messages.js"),"utf8"),context);
  vm.runInNewContext(fs.readFileSync(process.argv[2],"utf8"),context);
  return window.__REPOMAP_WORKSPACE_TEST__.analysisTargetMenuItems();
}
process.stdout.write(JSON.stringify({en:items("en"),ru:items("ru")}));
`
	runnerPath := filepath.Join(t.TempDir(), "single-module-library-rail.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, scriptPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run single module-library rail contract: %v\n%s", err, output)
	}
	var got map[string][]struct {
		Ref, Label, Title, GoMod string
		IsDefault, IsActive      bool
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode single module-library rail contract: %v\n%s", err, output)
	}
	for language, label := range map[string]string{"en": "Library API", "ru": "Библиотечный API"} {
		items := got[language]
		if len(items) != 1 || items[0].Ref != "module-library" || items[0].Label != label ||
			items[0].Title != "gopkg.in/telebot.v3" || items[0].GoMod != "go.mod" ||
			!items[0].IsDefault || !items[0].IsActive {
			t.Fatalf("%s single module-library rail = %#v", language, items)
		}
	}
}
