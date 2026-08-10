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
const context={sources:[],surface_starts:[],studies:[],package_targets:[],package_paths:["gopkg.in/telebot.v3"],structural_relations:[],library_api_packages:[{package_path:"gopkg.in/telebot.v3",display_path:".",declarations:[
 {kind:"func",name:"NewBot",path:"bot.go",line:26,column:6},
 {kind:"method",receiver:"(*Bot)",name:"Start",path:"bot.go",line:58,column:17},
 {kind:"method",receiver:"(*Bot)",name:"Stop",path:"bot.go",line:72,column:17},
 {kind:"method",receiver:"(*Bot)",name:"Handle",path:"bot.go",line:86,column:17},
 {kind:"type",name:"Bot",path:"bot.go",line:12,column:6},
 {kind:"const",name:"DefaultLimit",path:"settings.go",line:8,column:7},
 {kind:"var",name:"ErrClosed",path:"errors.go",line:5,column:5},
]}]};
const app=window.RepomapArchitectureCanvas.mount(host,{components:[{id:"bot-api",name:"Bot API",description:"Public bot API",subsystem_id:"library",members:[]}],subsystems:[{id:"library",name:"Library",component_ids:["bot-api"]}],groups:[],structural_edges:[],behavior_anchors:[],relations:[],surfaces:[],flows:[]},{userMode:true,message:id=>id,componentContexts:{"bot-api":context},associations:{components:[]},openSourceLocation:location=>opened.push(location)});
app.ready.then(()=>{
 app.openComponent("bot-api");
 const initial=walk(host),tabs=initial.filter(n=>n.getAttribute&&n.getAttribute("role")==="tab"),labels=tabs.map(n=>n.textContent);
 const summaryPanel=initial.find(n=>n.getAttribute&&n.getAttribute("role")==="tabpanel"&&String(n.getAttribute("id")||"").includes("-summary-panel"));
 const summaryNodes=summaryPanel?walk(summaryPanel):[];
 const summaryReceivers=summaryNodes.filter(n=>cls(n).includes("rm-arch__api-receiver"));
 const summaryCollapsed=summaryNodes.filter(n=>cls(n).includes("rm-arch__api-section--collapsed"));
 if(tabs[1])tabs[1].click();
 const after=walk(host),apiPanel=after.find(n=>n.getAttribute&&n.getAttribute("role")==="tabpanel"&&String(n.getAttribute("id")||"").includes("-api-panel"));
 const apiNodes=apiPanel?walk(apiPanel):[],rows=apiNodes.filter(n=>cls(n).includes("rm-arch__api-declaration"));
 const receivers=apiNodes.filter(n=>cls(n).includes("rm-arch__api-receiver"));
 const collapsed=apiNodes.filter(n=>cls(n).includes("rm-arch__api-section--collapsed"));
 const packageHeaders=apiNodes.filter(n=>n.tagName==="H4").map(n=>n.textContent);
 const packageGroups=apiNodes.filter(n=>cls(n).includes("rm-arch__api-package"));
 const kinds=apiNodes.filter(n=>cls(n).includes("rm-arch__api-kind"));
 const paths=apiNodes.filter(n=>n.tagName==="CODE");
 const newBot=rows.find(n=>n.textContent==="NewBot()"),start=rows.find(n=>n.textContent==="(*Bot).Start()");
 if(newBot)newBot.click();if(start)start.click();
 process.stdout.write(JSON.stringify({
  labels,apiPanelVisible:!!apiPanel&&!apiPanel.hidden,packageHeaders,packageTitles:packageGroups.map(n=>n.getAttribute("title")||""),
  rows:rows.map(n=>n.textContent),rowTitles:rows.map(n=>n.getAttribute("title")||""),rowAria:rows.map(n=>n.getAttribute("aria-label")||""),
  receiverCount:receivers.length,receiverOpen:receivers.map(n=>!!n.open),receiverSummaries:receivers.map(n=>n.children[0]&&n.children[0].textContent||""),
  collapsedCount:collapsed.length,collapsedOpen:collapsed.map(n=>!!n.open),collapsedSummaries:collapsed.map(n=>n.children[0]&&n.children[0].textContent||""),
  summaryReceiverCount:summaryReceivers.length,summaryReceiverOpen:summaryReceivers.map(n=>!!n.open),summaryCollapsedOpen:summaryCollapsed.map(n=>!!n.open),
  kindCount:kinds.length,pathCount:paths.length,opened
 }));
}).catch(e=>{process.stdout.write(JSON.stringify({error:String(e&&e.stack||e)}));process.exit(2);});
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
		Labels, PackageHeaders, PackageTitles               []string
		Rows, RowTitles, RowAria                            []string
		ReceiverOpen, CollapsedOpen                         []bool
		ReceiverSummaries, CollapsedSummaries               []string
		SummaryReceiverOpen, SummaryCollapsedOpen           []bool
		APIPanelVisible                                     bool
		ReceiverCount, CollapsedCount, SummaryReceiverCount int
		KindCount, PathCount                                int
		Opened                                              []struct {
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
	if strings.Join(got.PackageHeaders, ",") != "telebot.v3" || strings.Join(got.PackageTitles, ",") != "gopkg.in/telebot.v3" {
		t.Fatalf("component API package identity = %#v / %#v", got.PackageHeaders, got.PackageTitles)
	}
	wantRows := "NewBot(),(*Bot).Start(),(*Bot).Stop(),(*Bot).Handle(),Bot,DefaultLimit,ErrClosed"
	if strings.Join(got.Rows, ",") != wantRows || got.KindCount != 0 || got.PathCount != 0 {
		t.Fatalf("component API rows = %#v", got.Rows)
	}
	if got.ReceiverCount != 1 || strings.Join(got.ReceiverSummaries, ",") != "main.map.api.section.receiver" ||
		len(got.ReceiverOpen) != 1 || got.ReceiverOpen[0] || got.SummaryReceiverCount != 1 ||
		len(got.SummaryReceiverOpen) != 1 || got.SummaryReceiverOpen[0] {
		t.Fatalf("component API receiver groups = full %#v/%#v summary %#v", got.ReceiverSummaries, got.ReceiverOpen, got.SummaryReceiverOpen)
	}
	if got.CollapsedCount != 3 || len(got.CollapsedOpen) != 3 || got.CollapsedOpen[0] || got.CollapsedOpen[1] || got.CollapsedOpen[2] ||
		len(got.SummaryCollapsedOpen) == 0 || got.SummaryCollapsedOpen[0] {
		t.Fatalf("component API collapsed sections = full %#v/%#v summary %#v", got.CollapsedSummaries, got.CollapsedOpen, got.SummaryCollapsedOpen)
	}
	if len(got.RowTitles) != 7 || got.RowTitles[0] != "NewBot() · bot.go:26:6" || got.RowTitles[1] != "(*Bot).Start() · bot.go:58:17" ||
		len(got.RowAria) != 7 || !strings.Contains(got.RowAria[0], got.RowTitles[0]) {
		t.Fatalf("component API exact hidden source identity = titles %#v aria %#v", got.RowTitles, got.RowAria)
	}
	if len(got.Opened) != 2 || got.Opened[0].Path != "bot.go" || got.Opened[0].Line != 26 || got.Opened[0].Column != 6 ||
		got.Opened[1].Path != "bot.go" || got.Opened[1].Line != 58 || got.Opened[1].Column != 17 {
		t.Fatalf("component API exact source action = %#v", got.Opened)
	}
}
