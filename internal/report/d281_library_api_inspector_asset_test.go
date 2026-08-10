package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestD281ComponentInspectorUsesExactLibraryAPIInsteadOfPackageLineFallback(t *testing.T) {
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
class Element{
 constructor(tag){this.tagName=String(tag||"div").toUpperCase();this.children=[];this.attributes={};this.className="";this._text="";this.hidden=false;this.dataset={};this.style={setProperty(k,v){this[k]=String(v);}};this.listeners={};this.parentNode=null;this.clientWidth=1120;this.clientHeight=720;this.classList={add:(...xs)=>{const s=new Set(cls(this));xs.forEach(x=>s.add(x));this.className=[...s].join(" ");},remove:(...xs)=>{const d=new Set(xs);this.className=cls(this).filter(x=>!d.has(x)).join(" ");},toggle:(x,f)=>{const s=new Set(cls(this)),v=f===undefined?!s.has(x):!!f;if(v)s.add(x);else s.delete(x);this.className=[...s].join(" ");return v;},contains:x=>cls(this).includes(x)};}
 get textContent(){return this._text+this.children.map(x=>x.textContent||"").join("");}set textContent(v){this._text=String(v||"");this.children=[];}get childNodes(){return this.children;}
 appendChild(x){if(x){x.parentNode=this;this.children.push(x);}return x;}append(...xs){xs.forEach(x=>this.appendChild(x));}prepend(x){if(x){x.parentNode=this;this.children.unshift(x);}}replaceChildren(...xs){this.children=[];this._text="";this.append(...xs);}remove(){}
 setAttribute(k,v){this.attributes[k]=String(v);}getAttribute(k){return this.attributes[k]==null?null:this.attributes[k];}removeAttribute(k){delete this.attributes[k];}
 addEventListener(k,f){(this.listeners[k]||(this.listeners[k]=[])).push(f);}removeEventListener(){}click(){(this.listeners.click||[]).forEach(f=>f({target:this,currentTarget:this,preventDefault(){},stopPropagation(){}}));}focus(){document.activeElement=this;}contains(x){return x===this||this.children.some(c=>c&&c.contains&&c.contains(x));}
 getBoundingClientRect(){return{left:0,top:0,right:1120,bottom:720,width:1120,height:720};}querySelector(){return null;}querySelectorAll(){return [];}scrollIntoView(){}
}
const cls=n=>String(n&&n.className||"").split(/\s+/).filter(Boolean);const walk=root=>{const out=[];(function visit(n){if(!n)return;out.push(n);(n.children||[]).forEach(visit);})(root);return out;};
const document={createElement:t=>new Element(t),createElementNS:(_n,t)=>new Element(t),createTextNode:v=>{const n=new Element("#text");n.textContent=String(v);return n;},getElementById:()=>null,querySelector:()=>null,querySelectorAll:()=>[],addEventListener(){},removeEventListener(){},body:new Element("body"),documentElement:new Element("html")};document.activeElement=document.body;
const window={document,location:{hash:"#/map",pathname:"/report.html",search:""},history:{replaceState(){}},AbortController,Set,Map,URLSearchParams,Promise,ELK:function(){},requestAnimationFrame:f=>f(),clearTimeout,setTimeout,innerWidth:1440,innerHeight:1000,addEventListener(){},removeEventListener(){},RepomapUI:{message:id=>id}};
const sandbox={window,document,Element,AbortController,Set,Map,URLSearchParams,Promise,requestAnimationFrame:f=>f(),clearTimeout,setTimeout,console,addEventListener(){},removeEventListener(){},WheelEvent:{DOM_DELTA_LINE:1,DOM_DELTA_PAGE:2}};sandbox.global=sandbox;vm.createContext(sandbox);vm.runInContext(fs.readFileSync(process.argv[2],"utf8"),sandbox);
const host=new Element("div"),opened=[];
const context={sources:[],surface_starts:[],studies:[],package_targets:[],package_paths:["gopkg.in/telebot.v3/react"],structural_relations:[],library_api_packages:[{package_path:"gopkg.in/telebot.v3/react",display_path:"react",declarations:[{kind:"func",name:"React",path:"react/react.go",line:10,column:6},{kind:"type",name:"Reaction",path:"react/react.go",line:20,column:6}]}]};
const app=window.RepomapArchitectureCanvas.mount(host,{components:[{id:"react",name:"React",description:"React integration",subsystem_id:"extensions",members:[]}],subsystems:[{id:"extensions",name:"Extensions",component_ids:["react"]}],groups:[],structural_edges:[],behavior_anchors:[],relations:[],surfaces:[],flows:[]},{userMode:true,message:id=>id,componentContexts:{react:context},associations:{components:[]},openSourceLocation:location=>opened.push(location)});
app.ready.then(()=>{app.openComponent("react");const nodes=walk(host);const tabs=nodes.filter(n=>n.getAttribute&&n.getAttribute("role")==="tab");const labels=tabs.map(n=>n.textContent);if(tabs[1])tabs[1].click();const after=walk(host);const apiPanel=after.find(n=>n.getAttribute&&n.getAttribute("role")==="tabpanel"&&String(n.getAttribute("id")||"").includes("-api-panel"));const rows=apiPanel?walk(apiPanel).filter(n=>cls(n).includes("rm-arch__api-declaration")):[];if(rows[0])rows[0].click();process.stdout.write(JSON.stringify({labels,apiPanelVisible:!!apiPanel&&!apiPanel.hidden,rows:rows.map(n=>n.textContent),opened}));}).catch(e=>{process.stdout.write(JSON.stringify({error:String(e&&e.stack||e)}));process.exit(2);});
`
	runnerPath := filepath.Join(t.TempDir(), "d281-library-api-inspector.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, canvasPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run D281 library API inspector: %v\n%s", err, output)
	}
	var got struct {
		Labels          []string
		APIPanelVisible bool
		Rows            []string
		Opened          []struct {
			Path         string
			Line, Column int
		}
		Error string
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode D281 library API inspector: %v\n%s", err, output)
	}
	if got.Error != "" {
		t.Fatal(got.Error)
	}
	if strings.Join(got.Labels, ",") != "architecture.tab.summary,main.map.mode.api,architecture.tab.connections" || !got.APIPanelVisible {
		t.Fatalf("component API tabs = %#v visible=%t", got.Labels, got.APIPanelVisible)
	}
	if len(got.Rows) != 2 || !strings.Contains(got.Rows[0], "React") || !strings.Contains(got.Rows[1], "Reaction") {
		t.Fatalf("component API rows = %#v", got.Rows)
	}
	if len(got.Opened) != 1 || got.Opened[0].Path != "react/react.go" || got.Opened[0].Line != 10 || got.Opened[0].Column != 6 {
		t.Fatalf("component API exact source action = %#v", got.Opened)
	}
}
