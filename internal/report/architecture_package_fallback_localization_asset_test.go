package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The fixture mirrors the one-package offline issue-bot Canvas that exposed
// English deterministic backend copy in a Russian report. It mounts the real
// Canvas asset (so the singleton group label and cube description are tested)
// and also exercises the report's sanitizing projection used by the component
// list. Model-authored and anchor-first text are explicit negative controls.
func TestArchitecturePackageFallbackHasTypedLocalizedDOMCopy(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	canvasPath, err := filepath.Abs(filepath.Join("templates", "architecture_canvas.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs=require("fs"),vm=require("vm");
const canvasSource=fs.readFileSync(process.argv[2],"utf8");
const messageSource=fs.readFileSync(process.argv[2].replace("architecture_canvas.js","ui_messages.js"),"utf8");
const reportSource=fs.readFileSync(process.argv[2].replace("architecture_canvas.js","script.js"),"utf8");
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
 removeEventListener(){}focus(){}contains(x){return x===this||this.children.some(c=>c&&c.contains&&c.contains(x));}
 getBoundingClientRect(){return {left:0,top:0,right:300,bottom:180,width:300,height:180};}
 querySelector(){return null;}querySelectorAll(){return [];}scrollIntoView(){}
}
const walk=root=>{const out=[];(function visit(x){if(!x)return;out.push(x);(x.children||[]).forEach(visit);})(root);return out;};
const has=(node,name)=>String(node&&node.className||"").split(/\s+/).includes(name);
const nodeText=root=>walk(root).map(node=>String(node.textContent||"")).join("");
function packageCanvas(source,mode) {return {
 version:15,architecture_source:source,architecture_level:4,grounding_mode:mode||"package_landscape",validation_outcome:"accepted",
 components:[{id:"component-issue-bot-main",subsystem_id:"subsystem-packages",name:"main",description:"Deterministic grouping from exact local package candidates.",members:[
  {id:{kind:"package",value:"member-package-opaque"},name:"main",parent_id:null},
  {id:{kind:"symbol",value:"member-symbol-opaque"},name:"gitlab.example/issue-bot.main",parent_id:{kind:"file",value:"member-file-opaque"}},
 ]}],
 subsystems:[{id:"subsystem-packages",name:"Packages",description:"Deterministic local package landscape.",component_ids:["component-issue-bot-main"]}],
 structural_edges:[],behavior_anchors:[],relations:[],surfaces:[],flows:[],
};}
function authoredCanvas(source,level,mode) {return {
 version:15,architecture_source:source,architecture_level:level,grounding_mode:mode,validation_outcome:"accepted",
 components:[{id:"authored",subsystem_id:"authored-area",name:"Command dispatch",description:"Author-owned component description.",members:[{id:{kind:"package",value:"opaque-authored-package"},name:"technical-package"}]}],
 subsystems:[{id:"authored-area",name:"Entry and dispatch",description:"Author-owned subsystem description.",component_ids:["authored"]}],
 structural_edges:[],behavior_anchors:[],relations:[],surfaces:[],flows:[],
};}

const domDocument={createElement:t=>new Element(t),createElementNS:(_n,t)=>new Element(t),createTextNode:v=>{const n=new Element("#text");n.textContent=String(v);return n;},getElementById:()=>null,querySelector:()=>null,querySelectorAll:()=>[],addEventListener(){},removeEventListener(){},body:new Element("body"),documentElement:new Element("html")};
const domWindow={document:domDocument,location:{hash:"#/map"},ELK:function(){},AbortController,Set,Map,URLSearchParams,Promise,requestAnimationFrame:f=>f(),clearTimeout,setTimeout,innerWidth:1440,innerHeight:1000,addEventListener(){},removeEventListener(){}};
const domSandbox={window:domWindow,document:domDocument,Element,AbortController,Set,Map,URLSearchParams,Promise,requestAnimationFrame:f=>f(),clearTimeout,setTimeout,console,addEventListener(){},removeEventListener(){}};
domSandbox.global=domSandbox;vm.createContext(domSandbox);vm.runInContext(messageSource,domSandbox);vm.runInContext(canvasSource,domSandbox);
async function render(locale,data) {
 domDocument.documentElement.lang=locale;const host=new Element("div");const original=JSON.stringify(data);
 const message=(id,params)=>domWindow.__REPOMAP_UI_MESSAGES_TEST__.messageForLocale(locale,id,params);
 const app=domWindow.RepomapArchitectureCanvas.mount(host,data,{userMode:true,message});await app.ready;
 const nodes=walk(host),card=nodes.find(node=>has(node,"rm-arch__component-card"));
 return {card:nodeText(card),group:nodeText(nodes.find(node=>has(node,"rm-arch__component-group"))),name:nodeText(nodes.find(node=>has(node,"rm-arch__component-name"))),description:nodeText(nodes.find(node=>has(node,"rm-arch__component-description"))),inputUnchanged:original===JSON.stringify(data)};
}
function project(locale,canvas) {
 const report={report_language:locale,user_mechanisms:[],user_sources:[],openable_paths:[],source_ids:{},architecture_canvas:canvas};
 const document={documentElement:{lang:locale},getElementById:id=>id==="rm-report-data"?{textContent:JSON.stringify(report)}:null,querySelector(){return null;},querySelectorAll(){return [];},addEventListener(){},removeEventListener(){}};
 const window={document,location:{search:"",hash:"#/map",hostname:"example.test",protocol:"file:",pathname:"/report.html"},history:{state:null,pushState(){},replaceState(){}},__REPOMAP_WORKSPACE_TEST__:{},addEventListener(){},removeEventListener(){},setTimeout(){return 1;},clearTimeout(){}};
 const sandbox={window,document,Element,AbortController,Set,Map,URLSearchParams,Promise,console,setTimeout,clearTimeout,requestAnimationFrame:f=>f()};
 sandbox.global=sandbox;vm.createContext(sandbox);vm.runInContext(messageSource,sandbox);vm.runInContext(canvasSource,sandbox);vm.runInContext(reportSource,sandbox);
 return window.__REPOMAP_WORKSPACE_TEST__.userArchitectureData();
}
(async()=>{process.stdout.write(JSON.stringify({
 dom:{ruLocal:await render("ru",packageCanvas("local_packages")),ruRejected:await render("ru",packageCanvas("package_fallback")),ruMixed:await render("ru",packageCanvas("package_fallback","mixed")),enLocal:await render("en",packageCanvas("local_packages")),ruModel:await render("ru",authoredCanvas("validated_model",1,"package_landscape")),ruAnchors:await render("ru",authoredCanvas("local_anchors",3,"behavior_grounded"))},
 projection:{ruPackage:project("ru",packageCanvas("local_packages")),enPackage:project("en",packageCanvas("local_packages")),ruModel:project("ru",authoredCanvas("validated_model",1,"package_landscape")),ruAnchors:project("ru",authoredCanvas("local_anchors",3,"behavior_grounded"))},
}));})().catch(error=>{process.stdout.write(JSON.stringify({error:String(error&&error.stack||error)}));process.exit(2);});
`
	runnerPath := filepath.Join(t.TempDir(), "architecture-package-fallback-localization.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, canvasPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run package fallback localization: %v\n%s", err, output)
	}
	type rendered struct {
		Card, Group, Name, Description string
		InputUnchanged                 bool
	}
	type canvas struct {
		ArchitectureSource string `json:"architecture_source"`
		Components         []struct{ Name, Description string }
		Subsystems         []struct{ Name, Description string }
	}
	var got struct {
		DOM struct {
			RULocal, RURejected, RUMixed, ENLocal, RUModel, RUAnchors rendered
		}
		Projection struct {
			RUPackage, ENPackage, RUModel, RUAnchors canvas
		}
		Error string
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode package fallback localization: %v\n%s", err, output)
	}
	if got.Error != "" {
		t.Fatalf("package fallback localization failed: %s", got.Error)
	}
	for name, value := range map[string]rendered{
		"ru local": got.DOM.RULocal, "ru rejected": got.DOM.RURejected, "ru mixed": got.DOM.RUMixed,
	} {
		if value.Group != "Пакеты" || value.Name != "main" ||
			value.Description != "Точные локальные элементы, сгруппированные по пакету." ||
			strings.Contains(value.Card, "Packages") || strings.Contains(value.Card, "Deterministic grouping") || !value.InputUnchanged {
			t.Errorf("%s package DOM = %#v", name, value)
		}
	}
	if got.DOM.ENLocal.Group != "Packages" || got.DOM.ENLocal.Name != "main" ||
		got.DOM.ENLocal.Description != "Exact local members grouped by package." ||
		strings.Contains(got.DOM.ENLocal.Card, "Deterministic grouping from exact local package candidates.") || !got.DOM.ENLocal.InputUnchanged {
		t.Errorf("EN package DOM = %#v", got.DOM.ENLocal)
	}
	for name, value := range map[string]rendered{"model": got.DOM.RUModel, "anchor": got.DOM.RUAnchors} {
		if value.Group != "Entry and dispatch" || value.Name != "Command dispatch" ||
			value.Description != "Author-owned component description." || !value.InputUnchanged {
			t.Errorf("%s-authored Canvas copy changed = %#v", name, value)
		}
	}
	if got.Projection.RUPackage.ArchitectureSource != "" || len(got.Projection.RUPackage.Components) != 1 ||
		got.Projection.RUPackage.Components[0].Name != "main" ||
		got.Projection.RUPackage.Components[0].Description != "Точные локальные элементы, сгруппированные по пакету." ||
		len(got.Projection.RUPackage.Subsystems) != 1 || got.Projection.RUPackage.Subsystems[0].Name != "Пакеты" {
		t.Errorf("RU report package projection = %#v", got.Projection.RUPackage)
	}
	if len(got.Projection.ENPackage.Components) != 1 || got.Projection.ENPackage.Components[0].Description != "Exact local members grouped by package." ||
		len(got.Projection.ENPackage.Subsystems) != 1 || got.Projection.ENPackage.Subsystems[0].Name != "Packages" {
		t.Errorf("EN report package projection = %#v", got.Projection.ENPackage)
	}
	if len(got.Projection.RUModel.Components) != 1 || got.Projection.RUModel.Components[0].Name != "Command dispatch" ||
		got.Projection.RUModel.Components[0].Description != "Author-owned component description." ||
		len(got.Projection.RUModel.Subsystems) != 1 || got.Projection.RUModel.Subsystems[0].Name != "Entry and dispatch" {
		t.Errorf("model-authored report copy changed = %#v", got.Projection.RUModel)
	}
	if len(got.Projection.RUAnchors.Components) != 1 || got.Projection.RUAnchors.Components[0].Name != "Диспетчеризация команд" ||
		len(got.Projection.RUAnchors.Subsystems) != 1 || got.Projection.RUAnchors.Subsystems[0].Name != "Вход и диспетчеризация" {
		t.Errorf("typed local-anchor report copy regressed = %#v", got.Projection.RUAnchors)
	}
}
