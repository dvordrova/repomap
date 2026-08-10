package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchitectureComponentInspectorDeduplicatesEntriesAndPrefersExactStarts(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	canvasPath, err := filepath.Abs(filepath.Join("templates", "architecture_canvas.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs"), vm = require("vm");
class Element {
 constructor(tag) {
  this.tagName=String(tag||"div").toUpperCase();this.children=[];this.attributes={};this.className="";
  this.textContent="";this.hidden=false;this.style={};this.dataset={};this.listeners={};this.parentNode=null;
  this.clientWidth=960;this.clientHeight=640;
  this.classList={
   add:(...xs)=>{const s=new Set(String(this.className).split(/\s+/).filter(Boolean));xs.forEach(x=>s.add(x));this.className=Array.from(s).join(" ");},
   remove:(...xs)=>{const s=new Set(xs);this.className=String(this.className).split(/\s+/).filter(x=>x&&!s.has(x)).join(" ");},
   toggle:(x,f)=>{const s=new Set(String(this.className).split(/\s+/).filter(Boolean)),v=f===undefined?!s.has(x):!!f;if(v)s.add(x);else s.delete(x);this.className=Array.from(s).join(" ");return v;},
   contains:x=>String(this.className).split(/\s+/).includes(x),
  };
 }
 get childNodes(){return this.children;}appendChild(x){if(x){x.parentNode=this;this.children.push(x);}return x;}
 append(...xs){xs.forEach(x=>this.appendChild(x));}prepend(x){if(x){x.parentNode=this;this.children.unshift(x);}}
 replaceChildren(...xs){this.children=[];this.textContent="";this.append(...xs);}remove(){}
 setAttribute(k,v){this.attributes[k]=String(v);}getAttribute(k){return this.attributes[k]==null?null:this.attributes[k];}
 removeAttribute(k){delete this.attributes[k];}addEventListener(k,f){(this.listeners[k]||(this.listeners[k]=[])).push(f);}
 removeEventListener(){}focus(){document.activeElement=this;}contains(x){return x===this||this.children.some(c=>c&&c.contains&&c.contains(x));}
 getBoundingClientRect(){return {left:0,top:0,right:300,bottom:180,width:300,height:180};}
 querySelector(){return null;}querySelectorAll(){return [];}scrollIntoView(){}
}
const walk=root=>{const out=[];(function visit(x){if(!x)return;out.push(x);(x.children||[]).forEach(visit);})(root);return out;};
const has=(node,name)=>String(node&&node.className||"").split(/\s+/).includes(name);
const nodeText=root=>walk(root).map(node=>String(node.textContent||"")).join("");
const document={createElement:t=>new Element(t),createElementNS:(_n,t)=>new Element(t),createTextNode:v=>{const n=new Element("#text");n.textContent=String(v);return n;},getElementById:()=>null,querySelector:()=>null,querySelectorAll:()=>[],addEventListener(){},removeEventListener(){},body:new Element("body"),documentElement:new Element("html")};
document.activeElement=document.body;
const labels={
 "main.overview.anatomy.surface_kind.http_route":"HTTP",
 "main.overview.anatomy.surface_kind.process_entry":"PROCESS",
 "main.overview.anatomy.surface_kind.other":"OTHER",
};
const window={document,location:{hash:"#/map"},AbortController,Set,Map,URLSearchParams,Promise,requestAnimationFrame:f=>f(),clearTimeout,setTimeout,innerWidth:1440,innerHeight:1000,addEventListener(){},removeEventListener(){}};
const sandbox={window,document,Element,AbortController,Set,Map,URLSearchParams,Promise,requestAnimationFrame:f=>f(),clearTimeout,setTimeout,console,addEventListener(){},removeEventListener(){}};
sandbox.global=sandbox;vm.createContext(sandbox);vm.runInContext(fs.readFileSync(process.argv[2],"utf8"),sandbox);
const host=new Element("div");
const data={
 components:[{id:"core",subsystem_id:"system",name:"Core",description:"Core work",owned_surface_ids:["trigger-route"],anchor_ids:["anchor-main"],members:[]}],
 subsystems:[{id:"system",name:"System",component_ids:["core"]}],groups:[],structural_edges:[],relations:[],flows:[],
 surfaces:[{id:"trigger-route",name:"/jobs",kind:"http_route",evidence:[{path:"pkg/route.go",line:11,column:2}]}],
 behavior_anchors:[{id:"anchor-main",kind:"process_entry",label:"main",location:{path:"cmd/app/main.go",line:7,column:1}}],
};
const app=window.RepomapArchitectureCanvas.mount(host,data,{
 userMode:true,
 message:(id)=>labels[id]||id,
 openSourceLocation(){},openStudyTheme(){},openStudyDirection(){},openComponent(){},
 componentContexts:{core:{
  sources:[],studies:[],structural_relations:[],package_paths:["example.test/project/pkg"],member_count:1,authority:"validated",evidence_composition:"exact",
  surface_starts:[
   {id:"trigger-route",label:"/jobs",location:{path:"pkg/route.go",line:11,column:2},actionable:true},
   {id:"anchor-main",label:"main",location:{path:"cmd/app/main.go",line:7,column:1},actionable:true},
  ],
  package_targets:[{path:"example.test/project/pkg",location:{path:"pkg/aaa.go",line:1,column:0},actionable:true}],
 }},
});
app.ready.then(()=>{
 app.openComponent("core");
 const panels=walk(host).filter(node=>node.getAttribute&&node.getAttribute("role")==="tabpanel");
 const summary=panels[0],connections=panels[1];
 const groupTitles=walk(connections).filter(node=>has(node,"rm-arch__entry-group-title")).map(nodeText);
 const connectionStarts=walk(connections).filter(node=>has(node,"rm-arch__source-start")).map(nodeText);
 const readStarts=walk(summary).filter(node=>has(node,"rm-arch__source-start")).map(nodeText);
 const primary=walk(summary).find(node=>has(node,"rm-arch__component-primary-source"));
 process.stdout.write(JSON.stringify({
  groupTitles,
  routeConnectionCount:connectionStarts.filter(value=>value.includes("pkg/route.go:11")).length,
  primary:primary?nodeText(primary):"",
  readStarts,
 }));
}).catch(error=>{process.stdout.write(JSON.stringify({error:String(error&&error.stack||error)}));process.exit(2);});
`
	runnerPath := filepath.Join(t.TempDir(), "architecture-component-causal-regression.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, canvasPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run Architecture component causal regression: %v\n%s", err, output)
	}
	var got struct {
		GroupTitles          []string `json:"groupTitles"`
		RouteConnectionCount int      `json:"routeConnectionCount"`
		Primary              string   `json:"primary"`
		ReadStarts           []string `json:"readStarts"`
		Error                string   `json:"error"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode Architecture component causal regression: %v\n%s", err, output)
	}
	if got.Error != "" {
		t.Fatalf("Architecture component causal regression failed: %s", got.Error)
	}
	if strings.Join(got.GroupTitles, ",") != "HTTP,PROCESS" || got.RouteConnectionCount != 1 {
		t.Fatalf("entry groups duplicated a typed start or lost the unique anchor: groups=%#v route count=%d", got.GroupTitles, got.RouteConnectionCount)
	}
	if !strings.Contains(got.Primary, "pkg/route.go:11") || strings.Contains(got.Primary, "pkg/aaa.go:1") {
		t.Fatalf("Summary primary source did not prefer the exact surface start: %q", got.Primary)
	}
	if len(got.ReadStarts) != 3 || !strings.Contains(got.ReadStarts[0], "pkg/route.go:11") ||
		!strings.Contains(got.ReadStarts[1], "cmd/app/main.go:7") ||
		!strings.Contains(got.ReadStarts[2], "pkg/aaa.go:1") {
		t.Fatalf("Read code order = %#v, want exact surfaces before package fallback", got.ReadStarts)
	}
}

func TestInlineStudyThemeHistoryHasNoParentReturnRoute(t *testing.T) {
	scriptPath, err := filepath.Abs(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, deleted := range []string{
		"navigateWorkspace('study_overview', { replace: true })",
		"rm-study-detail",
		"rm-study-theme-card__open",
	} {
		if strings.Contains(string(script), deleted) {
			t.Errorf("unified target page retained deleted parent-route control %q", deleted)
		}
	}
	if strings.Count(string(script), "renderArchitectureReturn()") != 1 {
		t.Error("deleted parent-return banner is still invoked")
	}
	for _, required := range []string{
		"action.setAttribute('href', '#study-theme-' + ordinal)",
		"writeWorkspaceHistory('#study-theme-' + ordinal",
		"detail.open = true",
		"detail.scrollIntoView({ block: 'start' })",
	} {
		if !strings.Contains(string(script), required) {
			t.Errorf("inline Study deep-link/history contract is missing %q", required)
		}
	}
}
