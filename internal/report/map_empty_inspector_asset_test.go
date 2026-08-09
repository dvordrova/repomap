package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMapRouteEmptyInspectorIsBoundedActionableAndSelectionAware(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	assetPath, err := filepath.Abs(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs"), vm = require("vm");
class Element {
 constructor(tag) {
  this.tagName=String(tag||"div").toUpperCase(); this.children=[]; this.attributes={}; this.className="";
  this._text=""; this.hidden=false; this.open=false; this.style={}; this.dataset={}; this.parentNode=null;
  this.id=""; this.listeners={}; this.onclick=null;
  this.classList={
   add:(...xs)=>{const s=new Set(String(this.className).split(/\s+/).filter(Boolean));xs.forEach(x=>s.add(x));this.className=Array.from(s).join(" ");},
   remove:(...xs)=>{const s=new Set(xs);this.className=String(this.className).split(/\s+/).filter(x=>x&&!s.has(x)).join(" ");},
   toggle:(x,f)=>{const s=new Set(String(this.className).split(/\s+/).filter(Boolean)),v=f===undefined?!s.has(x):!!f;if(v)s.add(x);else s.delete(x);this.className=Array.from(s).join(" ");return v;},
   contains:(x)=>String(this.className).split(/\s+/).includes(x),
  };
 }
 get childNodes(){return this.children;}
 get childElementCount(){return this.children.length;}
 get textContent(){return this._text+this.children.map(x=>x.textContent||"").join("");}
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
 dispatchEvent(e){(this.listeners[e&&e.type]||[]).forEach(f=>f.call(this,e));}
 click(){if(typeof this.onclick==="function")this.onclick({preventDefault(){},stopPropagation(){}});this.dispatchEvent({type:"click",preventDefault(){},stopPropagation(){}});}
 focus(){document.activeElement=this;}
 contains(x){return x===this||this.children.some(c=>c&&c.contains&&c.contains(x));}
 querySelector(s){return walk(this).find(x=>matches(x,s))||null;}
 querySelectorAll(s){return walk(this).filter(x=>matches(x,s));}
 scrollIntoView(){}
}
function walk(root,out){out=out||[];(root&&root.children||[]).forEach(x=>{out.push(x);walk(x,out);});return out;}
function has(node,name){return String(node&&node.className||"").split(/\s+/).includes(name);}
function matches(node,selector){
 if(!node||!selector)return false;
 if(selector[0]===".")return has(node,selector.slice(1));
 if(selector[0]==="#")return node.id===selector.slice(1);
 const attr=selector.match(/^\[([^=\]]+)(?:=["']?([^"'\]]+)["']?)?\]$/);
 if(attr)return Object.prototype.hasOwnProperty.call(node.attributes||{},attr[1])&&(attr[2]==null||node.attributes[attr[1]]===attr[2]);
 return node.tagName===String(selector).toUpperCase();
}
const roots={};
["rm-task-investigation","rm-study-overview","rm-study-detail","rm-operate-detail","rm-architecture","rm-provenance"].forEach(id=>{
 roots[id]=new Element("section");roots[id].id=id;roots[id].className="rm-tab-content";
});
const workspace=new Element("main");workspace.className="rm-workspace";
const sourcePath="cmd/main.go", revision="a".repeat(40);
const kinds=[
 ["process_entry","primary_application"],
 ["process_entry","secondary_service"],
 ["cli_command","tooling"],
 ["library_api","public_api"],
 ["async_task","runtime_activity"],
];
const triggers=Array.from({length:500},(_,i)=>({
 id:"entry-"+i, kind:kinds[i%kinds.length][0], executable_role:kinds[i%kinds.length][1],
 surface_role:"entry_surface", application_classification:"application_surface", availability:"available",
 identity:{name:"Entry "+i}, handler_location:{path:sourcePath,line:10+i,column:1},
 handler:{text:"Entry"+i,known:true},
}));
const components=Array.from({length:12},(_,i)=>({id:"component-"+i,name:"Component "+i,description:"Responsibility "+i,members:[]}));
components.push({id:"local-remainder",name:"Local remainder",members:Array.from({length:17},(_,i)=>({name:"pkg/"+i}))});
const report={
 repo_name:"fixture",report_language:"en",repository_archetype:"application",
 repository_guide:{purpose:"Orient readers around the repository's runtime shape."},
 user_mechanisms:[],user_topics:[],source_ids:{},openable_paths:[sourcePath],
 user_sources:[{path:sourcePath,enclosing_symbol:"main",start_line:1,end_line:600,lines:[{line:10,text:"func main() {}",highlight:true}]}],
 repository_atlas:{version:1,units:Array.from({length:5},(_,i)=>({id:"unit-"+i,name:"unit/"+i,kind:i?"package":"repository"})),entities:[],evidence:[],relations:[]},
 discovered_surfaces:{total_count:triggers.length,triggers},
 architecture_canvas:{version:15,repository_archetype:"application",validation_outcome:"accepted_partial",local_remainder_component_id:"local-remainder",components,subsystems:[],groups:[],structural_edges:[],behavior_anchors:[],surfaces:[],flows:[],flow_edges:[],entry_handoff_groups:[]},
 architecture_associations:{components:[{component_id:"component-0",associations:[
  {kind:"boundary",paired:true,owning_unit:"unit-0",imported_family:"database/sql"},
  {kind:"resource",paired:true,owning_unit:"unit-0",imported_family:"database/sql"},
  {kind:"operation",paired:false,owning_unit:"unit-0",imported_family:"local.operation"},
  {kind:"surface",paired:false,owning_unit:"unit-0",imported_family:"local.surface"},
 ]}]},
};
if(process.argv[3]!=="unavailable")report.github_source_links={repository_url:"https://github.com/acme/fixture",revision};
const mobileMedia={matches:false,listeners:[],addEventListener(type,f){if(type==="change")this.listeners.push(f);},addListener(f){this.listeners.push(f);},set(value){this.matches=!!value;this.listeners.forEach(f=>f({matches:this.matches}));}};
const document={
 createElement:t=>new Element(t),createElementNS:(_n,t)=>new Element(t),createTextNode:v=>{const n=new Element("#text");n.textContent=String(v);return n;},
 getElementById(id){if(id==="rm-report-data")return{textContent:JSON.stringify(report)};if(roots[id])return roots[id];return Object.values(roots).flatMap(r=>[r].concat(walk(r))).find(n=>n.id===id)||null;},
 querySelector(s){return s===".rm-workspace"?workspace:null;},
 querySelectorAll(s){if(s===".rm-main-content > .rm-tab-content")return Object.values(roots);return Object.values(roots).flatMap(r=>walk(r)).filter(n=>matches(n,s));},
 body:new Element("body"),documentElement:{lang:"en"},activeElement:null,
};
const history={state:null,pushState(state,_title,hash){this.state=state;window.location.hash=hash;},replaceState(state,_title,hash){this.state=state;window.location.hash=hash;},back(){}};
let inspectorVisibility=null,canvasTransform="translate(41px, 29px) scale(.77)";
const window={document,location:{hash:"#/map",search:"",hostname:"fixture.test",protocol:"file:",pathname:"/report.html"},history,__REPOMAP_WORKSPACE_TEST__:{},addEventListener(){},removeEventListener(){},scrollTo(){},open(){},matchMedia(){return mobileMedia;},setTimeout,clearTimeout};
window.Element=Element;document.activeElement=document.body;
window.RepomapArchitectureCanvas={
 projectArchitectureLens(){return{objects:{entrypoints:[],touchpoints:[],entry_handoff_groups:[]}};},
 mount(_host,_data,options){inspectorVisibility=options.onInspectorVisibilityChange;inspectorVisibility(false);return{ready:Promise.resolve(),destroy(){},openComponent(){},openTrace(){},openFlowStep(){},openSurface(){},setLens(){},setStudyMechanismOverlay(){return false;},clearStudyMechanismOverlay(){}};},
};
const context={window,document,Element,URLSearchParams,Set,Map,AbortController,Promise,setTimeout,clearTimeout};
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js","ui_messages.js"),"utf8"),context);
vm.runInNewContext(fs.readFileSync(process.argv[2],"utf8"),context);
const api=window.__REPOMAP_WORKSPACE_TEST__;
(async()=>{
 api.restoreWorkspaceFromRoute({replace:true});
 await new Promise(resolve=>setImmediate(resolve));
 const mapRoot=roots["rm-architecture"],rails=walk(mapRoot).filter(n=>has(n,"rm-map-empty-inspector")),rail=rails[0];
 const nodes=rail?walk(rail):[];
 const starts=nodes.filter(n=>Object.prototype.hasOwnProperty.call(n.attributes||{},"data-rm-map-start-here"));
 const start=starts[0]||null;
 const metricValues=nodes.filter(n=>has(n,"rm-map-empty-inspector__metric")).map(n=>{const dd=walk(n).find(x=>x.tagName==="DD");return dd?dd.textContent:"";});
 const primary=nodes.find(n=>has(n,"rm-map-empty-inspector__entry-list--primary"));
 const disclosures=nodes.filter(n=>n.tagName==="DETAILS");
 const interactive=nodes.filter(n=>n.tagName==="A"||n.tagName==="BUTTON");
 const nestedInteractive=interactive.filter(n=>{for(let p=n.parentNode;p&&p!==rail;p=p.parentNode)if(p.tagName==="A"||p.tagName==="BUTTON")return true;return false;});
 const layout=walk(mapRoot).find(n=>has(n,"rm-map-primary-layout"));
 const stage=walk(mapRoot).find(n=>has(n,"rm-architecture-canvas-stage"));
 const componentDisclosure=walk(mapRoot).find(n=>has(n,"rm-architecture-list-disclosure"));
 const componentDisclosureCount=componentDisclosure&&walk(componentDisclosure).find(n=>has(n,"rm-architecture-disclosure__count"));
 const componentListRows=componentDisclosure?walk(componentDisclosure).filter(n=>has(n,"rm-architecture-list__item")).length:0;
 mobileMedia.set(true);
 const hashBefore=window.location.hash,transformBefore=canvasTransform;
 inspectorVisibility(true);
 const hiddenOnSelection=rail.hidden&&layout.classList.contains("has-detail-inspector");
 inspectorVisibility(false);
 const restoredOnClose=!rail.hidden&&!layout.classList.contains("has-detail-inspector");
 process.stdout.write(JSON.stringify({
  routeView:api.workspaceStateSnapshot().view,hash:window.location.hash,hashStable:hashBefore===window.location.hash,
  transformStable:transformBefore===canvasTransform,railCount:rails.length,purpose:(nodes.find(n=>has(n,"rm-map-empty-inspector__purpose"))||{}).textContent||"",
  startCount:starts.length,startTag:start&&start.tagName,startHref:start&&start.getAttribute("href"),startTarget:start&&start.getAttribute("target"),startRel:start&&start.getAttribute("rel"),
  unavailableCount:nodes.filter(n=>has(n,"rm-map-empty-inspector__unavailable")).length,metricValues,
  primaryRows:primary?primary.children.length:0,allRows:nodes.filter(n=>has(n,"rm-map-empty-inspector__entry-row")).length,
  disclosureCount:disclosures.length,openDisclosureCount:disclosures.filter(n=>n.open).length,railNodeCount:nodes.length,
  forbiddenWallCount:nodes.filter(n=>has(n,"rm-overview-object-card")||has(n,"rm-study-direction-card")||has(n,"rm-architecture-list__item")||has(n,"rm-architecture-truth-strip")).length,
  nestedInteractive:nestedInteractive.length,hiddenOnSelection,restoredOnClose,
  railPrecedesStage:!!(layout&&rail&&stage&&layout.children.indexOf(rail)<layout.children.indexOf(stage)),
  componentListReachable:!!componentDisclosure,componentListOpenOnMobile:!!(componentDisclosure&&componentDisclosure.open),
  componentDisclosureCount:componentDisclosureCount?componentDisclosureCount.textContent:"",componentListRows,
 }));
})().catch(error=>{process.stdout.write(JSON.stringify({error:String(error&&error.stack||error)}));process.exit(2);});
`
	runnerPath := filepath.Join(t.TempDir(), "map-empty-inspector-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	type result struct {
		RouteView, Hash, Purpose, StartTag, StartHref, StartTarget, StartRel string
		HashStable, TransformStable                                          bool
		RailCount, StartCount, UnavailableCount                              int
		MetricValues                                                         []string
		PrimaryRows, AllRows, DisclosureCount, OpenDisclosureCount           int
		RailNodeCount, ForbiddenWallCount, NestedInteractive                 int
		HiddenOnSelection, RestoredOnClose, RailPrecedesStage                bool
		ComponentListReachable, ComponentListOpenOnMobile                    bool
		ComponentDisclosureCount                                             string
		ComponentListRows                                                    int
		Error                                                                string
	}
	run := func(mode string) result {
		t.Helper()
		output, err := exec.Command(node, runnerPath, assetPath, mode).CombinedOutput()
		if err != nil {
			t.Fatalf("run %s Map empty inspector fixture: %v\n%s", mode, err, output)
		}
		var got result
		if err := json.Unmarshal(output, &got); err != nil {
			t.Fatalf("decode %s Map empty inspector fixture: %v\n%s", mode, err, output)
		}
		if got.Error != "" {
			t.Fatalf("%s Map empty inspector failed: %s", mode, got.Error)
		}
		return got
	}

	actionable := run("actionable")
	if actionable.RouteView != "map" || actionable.Hash != "#/map" || !actionable.HashStable || !actionable.TransformStable {
		t.Errorf("actual Map route or stable Canvas state = %#v", actionable)
	}
	if actionable.RailCount != 1 || actionable.Purpose != "Orient readers around the repository's runtime shape." {
		t.Errorf("repository empty inspector lead = %#v", actionable)
	}
	wantHref := "https://github.com/acme/fixture/blob/" + strings.Repeat("a", 40) + "/cmd/main.go#L10"
	if actionable.StartCount != 1 || actionable.StartTag != "A" || actionable.StartHref != wantHref ||
		actionable.StartTarget != "_blank" || actionable.StartRel != "noopener noreferrer" || actionable.UnavailableCount != 0 {
		t.Errorf("exact pinned Start Here = %#v", actionable)
	}
	if strings.Join(actionable.MetricValues, ",") != "500,12,5,1" {
		t.Errorf("compact counts or paired interaction count = %#v", actionable.MetricValues)
	}
	if actionable.PrimaryRows != 3 || actionable.AllRows != 5 || actionable.DisclosureCount < 2 ||
		actionable.OpenDisclosureCount != 0 || actionable.RailNodeCount > 70 {
		t.Errorf("bounded/collapsed entry summary = %#v", actionable)
	}
	if actionable.ForbiddenWallCount != 0 || actionable.NestedInteractive != 0 ||
		!actionable.HiddenOnSelection || !actionable.RestoredOnClose || !actionable.RailPrecedesStage ||
		!actionable.ComponentListReachable || !actionable.ComponentListOpenOnMobile ||
		actionable.ComponentDisclosureCount != "· 12" || actionable.ComponentListRows != 12 {
		t.Errorf("empty/detail inspector or mobile reachability contract = %#v", actionable)
	}

	unavailable := run("unavailable")
	if unavailable.StartCount != 0 || unavailable.UnavailableCount != 1 || unavailable.RailCount != 1 {
		t.Errorf("unavailable Start Here must be explicit, never an inert source = %#v", unavailable)
	}

	css := readCanvasAsset(t, "style.css")
	for _, token := range []string{
		".rm-map-primary-layout { align-items: stretch; display: grid",
		"grid-template-columns: minmax(0, 1fr) minmax(270px, 310px)",
		".rm-map-primary-layout > .rm-architecture-canvas-stage { grid-column: 1; grid-row: 1; }",
		"grid-column: 2; grid-row: 1",
		".rm-map-empty-inspector[hidden] { display: none; }",
		"@media (min-width: 641px) and (max-height: 1080px)",
		"--rm-map-first-viewport-height: clamp(360px, calc(100vh - 525px), 555px)",
		".rm-map-primary-layout .rm-arch:not(.has-selected-flow) .rm-arch__viewport { height: var(--rm-map-first-viewport-height); }",
		".rm-architecture-canvas-card { display: flex; flex-direction: column; }",
		".rm-map-primary-layout { display: contents; }",
		".rm-map-empty-inspector { max-height: none; order: 0; overflow: visible; }",
		".rm-map-lens-control { order: 1; }",
		".rm-architecture-canvas-stage { order: 3; }",
	} {
		if !strings.Contains(css, token) {
			t.Errorf("Map empty-inspector desktop/mobile containment missing %q", token)
		}
	}
}
