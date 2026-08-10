package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// D281 restores Integrations as an explicit context on the same stable Canvas.
// The fixture intentionally exercises both failure modes seen in ordinary
// reports: Telebot-like observations owned only by LocalRemainder, and a
// Restic-like mix of one mapped owner and one LocalRemainder owner. The latter
// must remain visible as evidence without inventing a remainder box on the map.
func TestD281IntegrationsWorkspaceKeepsMappedAndOffMapEvidenceActionable(t *testing.T) {
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
 constructor(tag) {
  this.tagName=String(tag||"div").toUpperCase(); this.children=[]; this.attributes={};
  this.className=""; this._text=""; this.hidden=false; this.open=false; this.style={};
  this.dataset={}; this.parentNode=null; this.id=""; this.listeners={}; this.onclick=null;
  this.value=""; this.placeholder=""; this.type="";
  this.classList={
   add:(...xs)=>{const s=new Set(classes(this));xs.forEach(x=>s.add(x));this.className=Array.from(s).join(" ");},
   remove:(...xs)=>{const drop=new Set(xs);this.className=classes(this).filter(x=>!drop.has(x)).join(" ");},
   toggle:(x,force)=>{const s=new Set(classes(this)),v=force===undefined?!s.has(x):!!force;if(v)s.add(x);else s.delete(x);this.className=Array.from(s).join(" ");return v;},
   contains:(x)=>classes(this).includes(x),
  };
 }
 get childNodes(){return this.children;}
 get childElementCount(){return this.children.length;}
 get textContent(){return this._text+this.children.map(x=>x&&x.textContent||"").join("");}
 set textContent(v){this._text=v==null?"":String(v);this.children=[];}
 appendChild(x){if(x){x.parentNode=this;this.children.push(x);}return x;}
 append(...xs){xs.forEach(x=>this.appendChild(x));}
 prepend(...xs){xs.reverse().forEach(x=>{if(x){x.parentNode=this;this.children.unshift(x);}});}
 replaceChildren(...xs){this.children=[];this._text="";this.append(...xs);}
 remove(){if(this.parentNode)this.parentNode.children=this.parentNode.children.filter(x=>x!==this);}
 setAttribute(k,v){this.attributes[k]=String(v);if(k==="id")this.id=String(v);}
 getAttribute(k){return this.attributes[k]==null?null:String(this.attributes[k]);}
 removeAttribute(k){delete this.attributes[k];}
 addEventListener(k,f){(this.listeners[k]||(this.listeners[k]=[])).push(f);}
 removeEventListener(k,f){this.listeners[k]=(this.listeners[k]||[]).filter(x=>x!==f);}
 dispatchEvent(e){(this.listeners[e&&e.type]||[]).forEach(f=>f.call(this,e));}
 click(){const e={type:"click",preventDefault(){},stopPropagation(){}};if(typeof this.onclick==="function")this.onclick(e);this.dispatchEvent(e);}
 focus(){document.activeElement=this;}
 contains(x){return x===this||this.children.some(c=>c&&c.contains&&c.contains(x));}
 querySelector(s){return walk(this).find(x=>matches(x,s))||null;}
 querySelectorAll(s){return walk(this).filter(x=>matches(x,s));}
 scrollIntoView(){}
}
function classes(node){return String(node&&node.className||"").split(/\s+/).filter(Boolean);}
function walk(root,out){out=out||[];(root&&root.children||[]).forEach(x=>{out.push(x);walk(x,out);});return out;}
function matches(node,selector){
 if(!node||!selector)return false;
 selector=String(selector).trim();
 if(selector.includes(","))return selector.split(",").some(part=>matches(node,part));
 if(selector.includes(" ")||selector.includes(">"))return false;
 const id=(selector.match(/#([A-Za-z0-9_-]+)/)||[])[1];if(id&&node.id!==id)return false;
 const tag=(selector.match(/^[A-Za-z][A-Za-z0-9_-]*/)||[])[0];if(tag&&node.tagName!==tag.toUpperCase())return false;
 const requiredClasses=Array.from(selector.matchAll(/\.([A-Za-z0-9_-]+)/g),m=>m[1]);
 if(requiredClasses.some(name=>!classes(node).includes(name)))return false;
 const attrs=Array.from(selector.matchAll(/\[([^=\]]+)(?:=["']?([^"'\]]+)["']?)?\]/g));
 for(const match of attrs){
  if(!Object.prototype.hasOwnProperty.call(node.attributes||{},match[1]))return false;
  if(match[2]!=null&&String(node.attributes[match[1]])!==String(match[2]))return false;
 }
 return !!(id||tag||requiredClasses.length||attrs.length);
}

const mode=process.argv[4];
const language=mode==="telebot"?"ru":"en";
const revision="b".repeat(40);
const telebotPaths=["bot.go","download.go","react/react.go"];
const resticPaths=["backend/local.go","internal/backend/http.go"];
function row(kind,family,unit,path,line,symbol,paired){
 return {kind,family,imported_family:family,owning_unit:unit,paired:!!paired,
  observation_count:1,witnesses:[{path,line,symbol,role:"production"}],
  source_roles:{production:1,test:0,tooling:0}};
}
function pair(family,unit,path,line,symbol){
 return [row("boundary",family,unit,path,line,symbol,true),row("resource",family,unit,path,line,symbol,true)];
}
const report={
 repo_name:mode==="telebot"?"gopkg.in/telebot.v3":"github.com/restic/restic",
 report_language:language,source_ids:{},user_sources:[],repository_atlas:{version:1,units:[],entities:[],relations:[],evidence:[]},
 captured_revision:revision,github_source_links:{repository_url:mode==="telebot"?"https://github.com/tucnak/telebot":"https://github.com/restic/restic",revision},
 architecture_canvas:{version:15,validation_outcome:"accepted_partial",architecture_source:"partial_model",
  local_remainder_component_id:"local-remainder",subsystems:[],groups:[],structural_edges:[],behavior_anchors:[],flows:[],flow_edges:[],entry_handoff_groups:[]},
 architecture_associations:{version:2,components:[],total:4},
};
if(mode==="telebot"){
 report.analysis_target={version:2,ref:"module-library",kind:"module_library",module_dir:".",module_path:"gopkg.in/telebot.v3"};
 report.openable_paths=telebotPaths;
 report.library_api={version:1,module_path:"gopkg.in/telebot.v3",module_dir:".",total_declarations:5,shown_declarations:5,packages:[
  {package_path:"gopkg.in/telebot.v3",package_dir:".",display_path:".",total_declarations:3,shown_declarations:3,counts:{functions:1,methods:1,types:0,consts:1,vars:0},declarations:[
   {kind:"const",name:"ModeDefault",path:"bot.go",line:10,column:2},
   {kind:"func",name:"NewBot",path:"bot.go",line:19,column:6},
   {kind:"method",receiver:"Bot",name:"Start",path:"bot.go",line:58,column:15},
  ]},
  {package_path:"gopkg.in/telebot.v3/react",package_dir:"react",display_path:"react",total_declarations:2,shown_declarations:2,counts:{functions:1,methods:0,types:1,consts:0,vars:0},declarations:[{kind:"func",name:"React",path:"react/react.go",line:10,column:6},{kind:"type",name:"Reaction",path:"react/react.go",line:20,column:6}]},
 ]};
 report.atlas_study={themes:{cards:[
  {ordinal:1,final_title:"Bot lifecycle",why_it_matters:"Construct and run a bot.",readings:[{path:"bot.go",line:19,column:6,symbol:"NewBot"}]},
  {ordinal:2,final_title:"React bridge",why_it_matters:"Connect UI handlers.",readings:[{path:"react/react.go",line:10,column:6,symbol:"React"}]},
  {ordinal:3,final_title:"Downloads",why_it_matters:"Follow file transfer.",readings:[{path:"download.go",line:80,column:3,symbol:"(*Bot).Download"}]},
  {ordinal:4,final_title:"Hidden fourth card",why_it_matters:"Outside the compact shelf.",readings:[]},
 ]}};
 report.architecture_canvas.components=[
  {id:"react",name:"React",description:"React integration",members:[]},
  {id:"local-remainder",name:"Unclassified by model",members:[{id:{kind:"package",value:"gopkg.in/telebot.v3"},name:"gopkg.in/telebot.v3"}]},
 ];
 report.architecture_canvas.surfaces=[];
 report.architecture_associations.components=[
  {component_id:"local-remainder",name:"Unclassified by model",associations:[
   ...pair("HTTP-gRPC-SDK","gopkg.in/telebot.v3","bot.go",19,"NewBot"),
   ...pair("process-OS","gopkg.in/telebot.v3","download.go",80,"(*Bot).Download"),
   row("operation","other","gopkg.in/telebot.v3","bot.go",90,"internalCall",false),
  ]},
 ];
}else{
 report.analysis_target={version:2,ref:"cmd-restic",kind:"executable_package",module_dir:".",package_dir:"cmd/restic",package_path:"github.com/restic/restic/cmd/restic",roots:[{path:"cmd/restic/main.go",line:161}]};
 report.openable_paths=resticPaths.concat(["cmd/restic/main.go","cmd/restic/worker.go"]);
 report.architecture_canvas.components=[
  {id:"backend",name:"Repository and data",description:"Storage backends",members:[]},
  {id:"local-remainder",name:"Unclassified by model",members:[{id:{kind:"package",value:"github.com/restic/restic/internal/backend"},name:"internal/backend"}]},
 ];
 report.architecture_canvas.surfaces=[
  {id:"restic-main",kind:"process_entry",name:"main",surface_role:"entry_surface",participating_component_ids:["backend"],evidence:[{path:"cmd/restic/main.go",line:161,column:6}]},
  {id:"restic-worker",kind:"async_task",name:"worker",surface_role:"runtime_activity",participating_component_ids:["backend"],evidence:[{path:"cmd/restic/worker.go",line:20,column:3}]},
 ];
 report.architecture_associations.components=[
  {component_id:"backend",name:"Repository and data",associations:[
   ...pair("filesystem","github.com/restic/restic/backend/local","backend/local.go",20,"local.Open"),
  ]},
  {component_id:"local-remainder",name:"Unclassified by model",associations:[
   ...pair("HTTP-gRPC-SDK","github.com/restic/restic/internal/backend","internal/backend/http.go",42,"http.NewRequest"),
   row("surface","other","github.com/restic/restic/internal/backend","internal/backend/http.go",50,"handler",false),
  ]},
 ];
}

const roots={};
["rm-task-investigation","rm-study-overview","rm-study-detail","rm-operate-detail","rm-architecture","rm-provenance"].forEach(id=>{
 roots[id]=new Element("section");roots[id].id=id;roots[id].className="rm-tab-content";
});
const workspace=new Element("main");workspace.className="rm-workspace";
const body=new Element("body");body.appendChild(workspace);
function documentNodes(){return Object.values(roots).flatMap(root=>[root].concat(walk(root))).concat([workspace].concat(walk(workspace)));}
const document={
 createElement:t=>new Element(t),createElementNS:(_n,t)=>new Element(t),
 createTextNode:v=>{const n=new Element("#text");n.textContent=String(v);return n;},
 getElementById(id){if(id==="rm-report-data")return{textContent:JSON.stringify(report)};return documentNodes().find(n=>n.id===id)||null;},
 querySelector(s){if(s===".rm-workspace")return workspace;return documentNodes().find(n=>matches(n,s))||null;},
 querySelectorAll(s){if(s===".rm-main-content > .rm-tab-content")return Object.values(roots);return documentNodes().filter(n=>matches(n,s));},
 body,documentElement:{lang:language},activeElement:body,addEventListener(){},removeEventListener(){},
};
const history={state:null,pushState(state,_title,hash){this.state=state;window.location.hash=hash;},replaceState(state,_title,hash){this.state=state;window.location.hash=hash;},back(){}};
const window={document,history,location:{hash:"#/map",search:"",hostname:"fixture.test",protocol:"file:",pathname:"/report.html"},
 __REPOMAP_WORKSPACE_TEST__:{},__REPOMAP_LAYOUT_TEST__:{},addEventListener(){},removeEventListener(){},scrollTo(){},open(){},
 matchMedia(){return{matches:false,addEventListener(){},addListener(){}};},setTimeout,clearTimeout};
window.Element=Element;
const context={window,document,Element,URLSearchParams,Set,Map,AbortController,Promise,setTimeout,clearTimeout};
vm.createContext(context);
vm.runInContext(fs.readFileSync(process.argv[2].replace("script.js","ui_messages.js"),"utf8"),context);
vm.runInContext(fs.readFileSync(process.argv[3],"utf8"),context);
const realCanvas=window.RepomapArchitectureCanvas;
const projection=realCanvas.projectArchitectureLens(report,"integrations");
let mountCount=0,openComponentCalls=0,selectedGroupCalls=0,canvasHost=null;
const lensCalls=[];
window.RepomapArchitectureCanvas=Object.assign({},realCanvas,{
 projectUserPresentation:data=>data,
 mount(host){
  mountCount++;canvasHost=host;host.style.transform="translate(17px, 11px) scale(.81)";
  return new Proxy({
   ready:Promise.resolve(),destroy(){},setLens(value){lensCalls.push(value);if(value==="integrations")throw new Error("canvas lens is not ready");},
   openComponent(){openComponentCalls++;},openTrace(){},openFlowStep(){},openSurface(){},
   selectEntrypointHandoffGroup(){selectedGroupCalls++;},setStudyMechanismOverlay(){return false;},clearStudyMechanismOverlay(){},
  },{get(target,key){return key in target?target[key]:(()=>{});}});
 },
});
vm.runInContext(fs.readFileSync(process.argv[2],"utf8"),context);
const api=window.__REPOMAP_WORKSPACE_TEST__;
(async()=>{
 api.restoreWorkspaceFromRoute({replace:true});
 await new Promise(resolve=>setImmediate(resolve));
 const mapRoot=roots["rm-architecture"];
 const initialNodes=[mapRoot].concat(walk(mapRoot));
 const initialModes=initialNodes.filter(n=>Object.prototype.hasOwnProperty.call(n.attributes||{},"data-map-mode"));
 const initialModeControl=initialNodes.find(n=>classes(n).includes("rm-map-mode-control"));
 const initialPressed=initialModes.map(n=>n.getAttribute("aria-pressed"));
 const initialContextText=initialNodes.filter(n=>classes(n).includes("rm-map-mode-context")).map(n=>n.textContent).join("\n");
 const initialAPIPackages=initialNodes.filter(n=>classes(n).includes("rm-library-api-package"));
 const initialAPIPackageLabels=initialNodes.filter(n=>classes(n).includes("rm-library-api-package__summary")).map(n=>n.children[0]&&n.children[0].textContent||"");
 const initialAPIPackageTitles=initialAPIPackages.map(n=>n.getAttribute("title")||"");
 const initialAPISources=initialNodes.filter(n=>classes(n).includes("rm-map-entry-context__source")&&n.parentNode&&classes(n.parentNode).includes("rm-library-api-declaration"));
 const initialAPISourceCount=initialAPISources.length;
 const initialAPISourceTitles=initialAPISources.map(n=>n.getAttribute("title")||"");
 const initialAPIDeclarationTexts=initialNodes.filter(n=>classes(n).includes("rm-library-api-declaration")).map(n=>n.textContent);
 const initialAPIStudyTitles=initialNodes.filter(n=>classes(n).includes("rm-library-api-study-pick__title")).map(n=>n.textContent);
 const initialAPIStudyJoinStates=initialNodes.filter(n=>classes(n).includes("rm-library-api-study-pick__source")).map(n=>n.getAttribute("data-api-study-joined"));
 const initialAPIStudySourceTitles=initialNodes.filter(n=>classes(n).includes("rm-library-api-study-pick__source")).map(n=>n.getAttribute("title")||"");
 const initialAPICollapsed=initialNodes.filter(n=>classes(n).includes("rm-library-api-section--collapsed"));
 const initialAPIReceivers=initialNodes.filter(n=>classes(n).includes("rm-library-api-receiver"));
 const initialAPIReceiverStates=initialAPIReceivers.map(n=>!!n.open);
 const initialAPIReceiverTexts=initialAPIReceivers.map(n=>n.children[0]&&n.children[0].textContent||"");
 const apiSearch=initialNodes.find(n=>classes(n).includes("rm-library-api-search"));
 if(apiSearch){apiSearch.value="Start";apiSearch.dispatchEvent({type:"input"});}
 const searchedNodes=[mapRoot].concat(walk(mapRoot));
 const searchedAPIReceivers=searchedNodes.filter(n=>classes(n).includes("rm-library-api-receiver"));
 const searchedAPIReceiverStates=searchedAPIReceivers.map(n=>!!n.open);
 if(apiSearch){apiSearch.value="no-declaration-can-match";apiSearch.dispatchEvent({type:"input"});}
 const zeroSearchNodes=[mapRoot].concat(walk(mapRoot));
 const zeroAPIEmpty=zeroSearchNodes.filter(n=>classes(n).includes("rm-library-api-empty"));
 const initialLensCalls=lensCalls.slice();
 const integrationButton=initialModes.find(n=>n.getAttribute("data-map-mode")==="integrations");
 const hostBefore=canvasHost,transformBefore=canvasHost&&canvasHost.style.transform;
 if(integrationButton)integrationButton.click();
 await new Promise(resolve=>setImmediate(resolve));
 const nodes=[mapRoot].concat(walk(mapRoot));
 const modes=nodes.filter(n=>Object.prototype.hasOwnProperty.call(n.attributes||{},"data-map-mode"));
 const shelves=nodes.filter(n=>classes(n).includes("rm-map-integrations-context"));
 const modeContexts=nodes.filter(n=>classes(n).includes("rm-map-mode-context"));
 const rows=nodes.filter(n=>classes(n).includes("rm-map-integration-row"));
 const outside=rows.filter(n=>classes(n).includes("rm-map-integration-row--outside"));
 const sources=nodes.filter(n=>classes(n).includes("rm-map-integration-source"));
 const activePressed=modes.map(n=>n.getAttribute("aria-pressed"));
 const integrationContextText=modeContexts.map(n=>n.textContent).join("\n");
 const activeContextHidden=!!(modeContexts[0]&&modeContexts[0].hidden);
 const closeContext=nodes.find(n=>classes(n).includes("rm-map-mode-context__close"));
 if(closeContext)closeContext.click();
 const closedNodes=[mapRoot].concat(walk(mapRoot));
 const closedModes=closedNodes.filter(n=>Object.prototype.hasOwnProperty.call(n.attributes||{},"data-map-mode"));
 const closedContext=closedNodes.find(n=>classes(n).includes("rm-map-mode-context"));
 process.stdout.write(JSON.stringify({
  modeIDs:modes.map(n=>n.getAttribute("data-map-mode")),modeLabels:modes.map(n=>n.textContent),initialPressed,initialContextText,initialAPIPackageLabels,initialAPIPackageTitles,initialAPISourceCount,initialAPISourceTitles,
  initialAPIDeclarationTexts,initialAPIStudyTitles,initialAPIStudyJoinStates,initialAPIStudySourceTitles,
  initialAPICollapsedStates:initialAPICollapsed.map(n=>!!n.open),initialAPICollapsedTexts:initialAPICollapsed.map(n=>n.children[0]&&n.children[0].textContent||""),
  initialAPIReceiverStates,initialAPIReceiverTexts,searchedAPIReceiverStates,
  modeControlRole:initialModeControl&&initialModeControl.getAttribute("role")||"",modeControlLabel:initialModeControl&&initialModeControl.getAttribute("aria-label")||"",
  zeroAPIEmptyCount:zeroAPIEmpty.length,zeroAPIEmptyText:zeroAPIEmpty.map(n=>n.textContent).join("\n"),
  zeroAPIEmptyRole:zeroAPIEmpty[0]&&zeroAPIEmpty[0].getAttribute("role")||"",zeroAPIEmptyLive:zeroAPIEmpty[0]&&zeroAPIEmpty[0].getAttribute("aria-live")||"",
  zeroAPIPackageCount:zeroSearchNodes.filter(n=>classes(n).includes("rm-library-api-package")).length,
  zeroAPIStudyCount:zeroSearchNodes.filter(n=>classes(n).includes("rm-library-api-study-picks")).length,
  zeroAPISearchCount:zeroSearchNodes.filter(n=>classes(n).includes("rm-library-api-search")).length,
  activePressed,activeContextHidden,shelfCount:shelves.length,shelfText:shelves.map(n=>n.textContent).join("\n"),
  contextText:integrationContextText,
  rowCount:rows.length,rowTexts:rows.map(n=>n.textContent),rowTags:rows.map(n=>n.tagName),rowOpen:rows.map(n=>!!n.open),outsideCount:outside.length,
  sourceCount:sources.length,sourceTags:sources.map(n=>n.tagName),sourceHrefs:sources.map(n=>n.getAttribute("href")||""),
  closePresent:!!closeContext,closedPressed:closedModes.map(n=>n.getAttribute("aria-pressed")),closedContextHidden:!!(closedContext&&closedContext.hidden),
  initialLensCalls,lensCalls,mountCount,openComponentCalls,selectedGroupCalls,
  transformStable:hostBefore===canvasHost&&transformBefore===(canvasHost&&canvasHost.style.transform),
  projectionDimmed:projection.dimmed,projectionTouchpoints:projection.counts.touchpoints,
  visibleParticipants:projection.participants&&projection.participants.visible_component_ids||[],
  offMapParticipants:projection.participants&&projection.participants.off_map_component_ids||[],
 }));
})().catch(error=>{process.stdout.write(JSON.stringify({error:String(error&&error.stack||error)}));process.exit(2);});
`
	runnerPath := filepath.Join(t.TempDir(), "d281-integrations-workspace.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	canvasPath, err := filepath.Abs(filepath.Join("templates", "architecture_canvas.js"))
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		ModeIDs, ModeLabels, InitialPressed, ActivePressed, ClosedPressed []string
		InitialAPIPackageLabels, InitialAPIPackageTitles                  []string
		InitialAPISourceTitles                                            []string
		InitialAPIDeclarationTexts, InitialAPIStudyTitles                 []string
		InitialAPIStudyJoinStates, InitialAPIStudySourceTitles            []string
		InitialAPICollapsedTexts                                          []string
		InitialAPIReceiverTexts                                           []string
		InitialAPICollapsedStates, InitialAPIReceiverStates               []bool
		SearchedAPIReceiverStates, RowOpen                                []bool
		ShelfText, ContextText, InitialContextText, Error                 string
		ModeControlRole, ModeControlLabel                                 string
		ZeroAPIEmptyText, ZeroAPIEmptyRole, ZeroAPIEmptyLive               string
		ShelfCount, RowCount, OutsideCount, SourceCount                   int
		ZeroAPIEmptyCount, ZeroAPIPackageCount                            int
		ZeroAPIStudyCount, ZeroAPISearchCount                             int
		RowTexts, RowTags, SourceTags, SourceHrefs                        []string
		InitialLensCalls, LensCalls                                       []string
		MountCount, OpenComponentCalls, SelectedGroupCalls                int
		TransformStable, ClosePresent, ActiveContextHidden                bool
		ClosedContextHidden                                               bool
		ProjectionDimmed, ProjectionTouchpoints                           int
		InitialAPISourceCount                                             int
		VisibleParticipants, OffMapParticipants                           []string
	}
	run := func(mode string) result {
		t.Helper()
		output, runErr := exec.Command(node, runnerPath, scriptPath, canvasPath, mode).CombinedOutput()
		if runErr != nil {
			t.Fatalf("run %s D281 integrations fixture: %v\n%s", mode, runErr, output)
		}
		var got result
		if err := json.Unmarshal(output, &got); err != nil {
			t.Fatalf("decode %s D281 integrations fixture: %v\n%s", mode, err, output)
		}
		if got.Error != "" {
			t.Fatalf("%s D281 integrations fixture failed: %s", mode, got.Error)
		}
		return got
	}

	telebot := run("telebot")
	if len(telebot.ModeIDs) == 0 {
		t.Fatalf("D281 production marker missing: [data-map-mode]")
	}
	if strings.Join(telebot.ModeIDs, ",") != "api,integrations" ||
		strings.Join(telebot.ModeLabels, ",") != "API,Интеграции · 2" {
		t.Errorf("Telebot compact modes = %#v / %#v", telebot.ModeIDs, telebot.ModeLabels)
	}
	if strings.Join(telebot.InitialPressed, ",") != "true,false" ||
		strings.Join(telebot.ActivePressed, ",") != "false,true" || telebot.ActiveContextHidden {
		t.Errorf("Telebot explicit mode selection = initial %#v active %#v", telebot.InitialPressed, telebot.ActivePressed)
	}
	if telebot.ModeControlRole != "group" || telebot.ModeControlLabel != "Контексты карты" {
		t.Errorf("Telebot map context switch accessible name = role %q label %q", telebot.ModeControlRole, telebot.ModeControlLabel)
	}
	for _, exact := range []string{"Публичный API библиотеки", "Что посмотреть сначала", "telebot", "NewBot()", "Bot.Start()", "react", "React()", "Reaction", "1 константа"} {
		if !strings.Contains(telebot.InitialContextText, exact) {
			t.Errorf("Telebot API launchpad is missing %q in %q", exact, telebot.InitialContextText)
		}
	}
	if !slices.Equal(telebot.InitialAPIPackageLabels, []string{"telebot", "react"}) {
		t.Errorf("Telebot API package labels = %#v, want semantic package labels without import major suffix", telebot.InitialAPIPackageLabels)
	}
	if strings.Contains(telebot.InitialContextText, "gopkg.in/telebot.v3") {
		t.Errorf("Telebot API repeats the full import path in visible package chrome: %q", telebot.InitialContextText)
	}
	if !slices.Equal(telebot.InitialAPIPackageTitles, []string{"", ""}) {
		t.Errorf("Telebot package summary repeats the full import path through hover chrome: %#v", telebot.InitialAPIPackageTitles)
	}
	if telebot.InitialAPISourceCount != 5 {
		t.Errorf("Telebot API exact source actions = %d, want 5", telebot.InitialAPISourceCount)
	}
	for _, exact := range []string{"bot.go:19:6", "bot.go:58:15", "bot.go:10:2", "react/react.go:10:6", "react/react.go:20:6"} {
		if !slices.ContainsFunc(telebot.InitialAPISourceTitles, func(title string) bool { return strings.Contains(title, exact) }) {
			t.Errorf("Telebot symbol-only API source actions lost hidden exact location %q in %#v", exact, telebot.InitialAPISourceTitles)
		}
	}
	if strings.Join(telebot.InitialAPIDeclarationTexts, ",") != "NewBot(),Bot.Start(),ModeDefault,React(),Reaction" {
		t.Errorf("Telebot API symbol-only declaration syntax = %#v", telebot.InitialAPIDeclarationTexts)
	}
	if slices.ContainsFunc(telebot.InitialAPIDeclarationTexts, func(text string) bool {
		return strings.Contains(text, ".go") || strings.Contains(text, "Функция") || strings.Contains(text, "Метод")
	}) {
		t.Errorf("Telebot API declaration rows repeat file/kind chrome: %#v", telebot.InitialAPIDeclarationTexts)
	}
	if !slices.Equal(telebot.InitialAPIStudyTitles, []string{"Bot lifecycle", "React bridge", "Downloads"}) ||
		strings.Join(telebot.InitialAPIStudyJoinStates, ",") != "true,true,false" {
		t.Errorf("Telebot compact Study/API exact join = titles %#v joins %#v", telebot.InitialAPIStudyTitles, telebot.InitialAPIStudyJoinStates)
	}
	for index, exact := range []string{"bot.go:19:6", "react/react.go:10:6", "download.go:80:3"} {
		if index >= len(telebot.InitialAPIStudySourceTitles) || !strings.Contains(telebot.InitialAPIStudySourceTitles[index], exact) {
			t.Errorf("Telebot Study source %d does not preserve exact hidden location %q in %#v", index, exact, telebot.InitialAPIStudySourceTitles)
		}
	}
	if strings.Join(telebot.InitialAPICollapsedTexts, ",") != "1 константа,1 тип" ||
		!slices.Equal(telebot.InitialAPICollapsedStates, []bool{false, false}) {
		t.Errorf("Telebot heavy API kinds are not initially collapsed: %#v / %#v", telebot.InitialAPICollapsedTexts, telebot.InitialAPICollapsedStates)
	}
	if !slices.Equal(telebot.InitialAPIReceiverTexts, []string{"Bot · 1 метод"}) ||
		!slices.Equal(telebot.InitialAPIReceiverStates, []bool{false}) ||
		!slices.Equal(telebot.SearchedAPIReceiverStates, []bool{true}) {
		t.Errorf("Telebot receiver methods are not collapsed until active search: text %#v initial %#v searched %#v",
			telebot.InitialAPIReceiverTexts, telebot.InitialAPIReceiverStates, telebot.SearchedAPIReceiverStates)
	}
	if telebot.ZeroAPIEmptyCount != 1 || telebot.ZeroAPIEmptyText != "По этому запросу объявления API не найдены." ||
		telebot.ZeroAPIEmptyRole != "status" || telebot.ZeroAPIEmptyLive != "polite" || telebot.ZeroAPIPackageCount != 0 ||
		telebot.ZeroAPIStudyCount != 1 || telebot.ZeroAPISearchCount != 1 {
		t.Errorf("Telebot zero-result API search is silent or replaces persistent orientation controls: %#v", telebot)
	}
	if telebot.ShelfCount == 0 {
		t.Fatalf("D281 production marker missing after Integrations selection: .rm-map-integrations-context")
	}
	if telebot.ShelfCount != 1 || telebot.RowCount != 2 || telebot.OutsideCount != 2 || telebot.SourceCount != 2 {
		t.Errorf("Telebot off-map integrations shelf = %#v", telebot)
	}
	for _, copy := range []string{
		"Наблюдаемые внешние вызовы и состояние",
		"1 место вызова",
		"Вне концептуальной группировки",
		"Это не полный перечень интеграций",
	} {
		if !strings.Contains(telebot.ContextText, copy) {
			t.Errorf("Telebot integrations context is missing %q in %q", copy, telebot.ContextText)
		}
	}
	if !slices.Equal(telebot.SourceTags, []string{"A", "A"}) {
		t.Errorf("Telebot exact source actions = %#v", telebot.SourceTags)
	}
	if !slices.Equal(telebot.RowTags, []string{"DETAILS", "DETAILS"}) || !slices.Equal(telebot.RowOpen, []bool{false, false}) {
		t.Errorf("Telebot integrations are not one flat disclosure list: tags %#v open %#v", telebot.RowTags, telebot.RowOpen)
	}
	wantTelebotHrefs := []string{
		"https://github.com/tucnak/telebot/blob/" + strings.Repeat("b", 40) + "/bot.go#L19",
		"https://github.com/tucnak/telebot/blob/" + strings.Repeat("b", 40) + "/download.go#L80",
	}
	if !slices.Equal(telebot.SourceHrefs, wantTelebotHrefs) {
		t.Errorf("Telebot exact source hrefs = %#v, want %#v", telebot.SourceHrefs, wantTelebotHrefs)
	}
	if telebot.ProjectionDimmed != 0 || telebot.ProjectionTouchpoints != 2 ||
		len(telebot.VisibleParticipants) != 0 || strings.Join(telebot.OffMapParticipants, ",") != "local-remainder" {
		t.Errorf("Telebot neutral/off-map projection = %#v", telebot)
	}

	restic := run("restic")
	if strings.Join(restic.ModeIDs, ",") != "entrypoints,integrations" ||
		strings.Join(restic.ModeLabels, ",") != "Entrypoints · 1,Integrations · 2" {
		t.Errorf("Restic compact modes = %#v / %#v", restic.ModeIDs, restic.ModeLabels)
	}
	if strings.Join(restic.InitialPressed, ",") != "false,false" ||
		strings.Join(restic.ActivePressed, ",") != "false,true" || restic.ActiveContextHidden {
		t.Errorf("Restic explicit mode selection = initial %#v active %#v", restic.InitialPressed, restic.ActivePressed)
	}
	if restic.ModeControlRole != "group" || restic.ModeControlLabel != "Map contexts" {
		t.Errorf("Restic map context switch accessible name = role %q label %q", restic.ModeControlRole, restic.ModeControlLabel)
	}
	if restic.ShelfCount != 1 || restic.RowCount != 2 || restic.OutsideCount != 1 || restic.SourceCount != 2 {
		t.Errorf("Restic mapped/off-map integrations shelf = %#v", restic)
	}
	for _, copy := range []string{
		"Observed external calls and state",
		"1 callsite",
		"Outside conceptual grouping",
		"This is not a complete integration inventory",
	} {
		if !strings.Contains(restic.ContextText, copy) {
			t.Errorf("Restic integrations context is missing %q in %q", copy, restic.ContextText)
		}
	}
	if !slices.Equal(restic.SourceTags, []string{"A", "A"}) {
		t.Errorf("Restic exact source actions = %#v", restic.SourceTags)
	}
	if !slices.Equal(restic.RowTags, []string{"DETAILS", "DETAILS"}) || !slices.Equal(restic.RowOpen, []bool{false, false}) {
		t.Errorf("Restic integrations are not one flat disclosure list: tags %#v open %#v", restic.RowTags, restic.RowOpen)
	}
	wantResticHrefs := []string{
		"https://github.com/restic/restic/blob/" + strings.Repeat("b", 40) + "/backend/local.go#L20",
		"https://github.com/restic/restic/blob/" + strings.Repeat("b", 40) + "/internal/backend/http.go#L42",
	}
	if !slices.Equal(restic.SourceHrefs, wantResticHrefs) {
		t.Errorf("Restic exact source hrefs = %#v, want %#v", restic.SourceHrefs, wantResticHrefs)
	}
	if restic.ProjectionDimmed != 0 || restic.ProjectionTouchpoints != 2 ||
		strings.Join(restic.VisibleParticipants, ",") != "backend" || strings.Join(restic.OffMapParticipants, ",") != "local-remainder" {
		t.Errorf("Restic neutral mapped/off-map projection = %#v", restic)
	}

	for name, got := range map[string]result{"telebot": telebot, "restic": restic} {
		if got.MountCount != 1 || got.OpenComponentCalls != 0 || got.SelectedGroupCalls != 0 || !got.TransformStable {
			t.Errorf("%s context switch mutated Canvas selection/layout = %#v", name, got)
		}
		if slices.Contains(got.InitialLensCalls, "entrypoints") || slices.Contains(got.InitialLensCalls, "integrations") {
			t.Errorf("%s automatically selected a focused Canvas context = %#v", name, got.InitialLensCalls)
		}
		if !slices.Contains(got.LensCalls, "integrations") || len(got.LensCalls) == 0 || got.LensCalls[len(got.LensCalls)-1] != "landscape" ||
			!got.ClosePresent || !got.ClosedContextHidden || slices.Contains(got.ClosedPressed, "true") {
			t.Errorf("%s context close did not return to neutral permanent map: %#v", name, got)
		}
	}
	if css := readCanvasAsset(t, "style.css"); !strings.Contains(css, ".rm-library-api-empty {") {
		t.Error("zero-result API status has no bounded visual treatment")
	}
}
