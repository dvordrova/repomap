package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func TestD278RootTargetLabelAndFreshnessTruth(t *testing.T) {
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
class Element {
  constructor() { this.hidden = true; this.textContent = ""; this.children = []; }
  appendChild(child) { this.children.push(child); return child; }
  replaceChildren(...children) { this.children = children; }
  setAttribute() {}
}
function evaluate(language, dirty, repoName) {
  const elements = {
    "rm-report-data": {textContent:JSON.stringify({
      repo_name:repoName, report_language:language, captured_revision:"a".repeat(40),
      freshness:{version:1,state:"fresh"},
      github_source_links:{repository_url:"https://github.com/dvordrova/telebot",revision:"a".repeat(40),working_tree_dirty:dirty},
      analysis_target:{version:2,ref:"at-root",kind:"executable_package",module_dir:".",module_path:repoName,package_dir:".",package_path:repoName},
      target_navigation:{version:2,default_target_ref:"at-root",current_target_ref:"at-root",targets:[
        {target_ref:"at-root",kind:"executable_package",module_path:repoName,module_dir:".",display_path:".",available:true,href:"#/map"},
        {target_ref:"at-layout",kind:"executable_package",module_path:repoName,module_dir:".",display_path:"layout",available:true,href:"../layout/report.html#/map"},
      ]},
      user_sources:[],openable_paths:[],source_ids:{},
    })},
    "rm-run-details":new Element(), "rm-provenance-row":new Element(),
    "rm-freshness-detail":new Element(), "rm-artifacts-dir":new Element(),
    "rm-feedback-path":new Element(), "rm-snapshot-detail":new Element(),
    "rm-submodule-detail":new Element(),
  };
  const document = {
    documentElement:{lang:language}, createElement(){return new Element();},
    createTextNode(text){const node=new Element();node.textContent=String(text);return node;},
    getElementById(id){return elements[id] || null;}, querySelector(){return null;},querySelectorAll(){return[];},
  };
  const window = {document,location:{search:"",hash:"#canvas",protocol:"file:",pathname:"/report.html"},__REPOMAP_WORKSPACE_TEST__:{},addEventListener(){}};
  const context={window,document,URLSearchParams,Set,Map,AbortController,Promise};
  vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js","ui_messages.js"),"utf8"),context);
  vm.runInNewContext(fs.readFileSync(process.argv[2],"utf8"),context);
  const api=window.__REPOMAP_WORKSPACE_TEST__;
  api.renderRunDetails();
  return {
    labels:api.analysisTargetMenuItems().map((item)=>item.label),
    chips:elements["rm-provenance-row"].children.map((item)=>item.textContent),
    detail:elements["rm-freshness-detail"].textContent,
  };
}
process.stdout.write(JSON.stringify({
  enClean:evaluate("en",false,"gopkg.in/telebot.v3"),
  enDirty:evaluate("en",true,"gopkg.in/telebot.v3"),
  ruDirty:evaluate("ru",true,"gopkg.in/telebot.v3"),
  etcd:evaluate("en",false,"go.etcd.io/etcd/v3"),
  moby:evaluate("en",false,"github.com/moby/moby/v2"),
}));
`
	runnerPath := filepath.Join(t.TempDir(), "d278-target-freshness.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, scriptPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run D278 asset: %v\n%s", err, output)
	}
	var wire map[string]struct {
		Labels []string `json:"labels"`
		Chips  []string `json:"chips"`
		Detail string   `json:"detail"`
	}
	if err := json.Unmarshal(output, &wire); err != nil {
		t.Fatalf("decode D278 asset: %v\n%s", err, output)
	}
	if !slices.Equal(wire["enClean"].Labels, []string{"telebot", "layout"}) ||
		!slices.Contains(wire["enClean"].Chips, "repository current") ||
		slices.Contains(wire["enClean"].Chips, "local changes captured") {
		t.Fatalf("clean current presentation = %#v", wire["enClean"])
	}
	if !slices.Contains(wire["enDirty"].Chips, "repository current") ||
		!slices.Contains(wire["enDirty"].Chips, "local changes captured") {
		t.Fatalf("stable dirty EN presentation = %#v", wire["enDirty"])
	}
	if !slices.Contains(wire["ruDirty"].Chips, "репозиторий актуален") ||
		!slices.Contains(wire["ruDirty"].Chips, "локальные изменения учтены") ||
		wire["ruDirty"].Detail != "Актуальность: актуален" {
		t.Fatalf("stable dirty RU presentation = %#v", wire["ruDirty"])
	}
	if !slices.Equal(wire["etcd"].Labels, []string{"etcd", "layout"}) ||
		!slices.Equal(wire["moby"].Labels, []string{"moby", "layout"}) {
		t.Fatalf("semantic-import root labels: etcd=%#v moby=%#v", wire["etcd"], wire["moby"])
	}
}
