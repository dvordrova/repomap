package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestD272ComponentSelectionUsesLocalDirectedGraphAndActionableRelations(t *testing.T) {
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
class Element {
 constructor(tag){this.tagName=String(tag||"div").toUpperCase();this.children=[];this.attributes={};this.className="";this.textContent="";this.hidden=false;this.style={setProperty(k,v){this[k]=String(v);}};this.dataset={};this.listeners={};this.parentNode=null;this.clientWidth=1120;this.clientHeight=720;
  this.classList={add:(...xs)=>{const s=new Set(String(this.className).split(/\s+/).filter(Boolean));xs.forEach(x=>s.add(x));this.className=[...s].join(" ");},remove:(...xs)=>{const s=new Set(xs);this.className=String(this.className).split(/\s+/).filter(x=>x&&!s.has(x)).join(" ");},toggle:(x,f)=>{const s=new Set(String(this.className).split(/\s+/).filter(Boolean)),v=f===undefined?!s.has(x):!!f;if(v)s.add(x);else s.delete(x);this.className=[...s].join(" ");return v;},contains:x=>String(this.className).split(/\s+/).includes(x)};
 }
 get childNodes(){return this.children;} appendChild(x){if(x){x.parentNode=this;this.children.push(x);}return x;} append(...xs){xs.forEach(x=>this.appendChild(x));} prepend(x){if(x){x.parentNode=this;this.children.unshift(x);}} replaceChildren(...xs){this.children=[];this.textContent="";this.append(...xs);} remove(){}
 setAttribute(k,v){this.attributes[k]=String(v);} getAttribute(k){return this.attributes[k]==null?null:this.attributes[k];} removeAttribute(k){delete this.attributes[k];}
 addEventListener(k,f){(this.listeners[k]||(this.listeners[k]=[])).push(f);} removeEventListener(){} click(){(this.listeners.click||[]).forEach(f=>f({target:this,currentTarget:this,preventDefault(){},stopPropagation(){}}));} focus(){document.activeElement=this;} contains(x){return x===this||this.children.some(c=>c&&c.contains&&c.contains(x));}
 getBoundingClientRect(){return {left:0,top:0,right:1120,bottom:720,width:1120,height:720};} querySelector(){return null;} querySelectorAll(){return [];} scrollIntoView(){}
}
const walk=root=>{const out=[];(function visit(x){if(!x)return;out.push(x);(x.children||[]).forEach(visit);})(root);return out;};
const cls=(node,name)=>String(node&&node.className||"").split(/\s+/).includes(name);
const svgCls=(node,name)=>String(node&&node.getAttribute&&node.getAttribute("class")||"").split(/\s+/).includes(name);
const nodeText=root=>walk(root).map(x=>String(x.textContent||"")).join("");
const document={createElement:t=>new Element(t),createElementNS:(_n,t)=>new Element(t),createTextNode:v=>{const n=new Element("#text");n.textContent=String(v);return n;},getElementById:()=>null,querySelector:()=>null,querySelectorAll:()=>[],addEventListener(){},removeEventListener(){},body:new Element("body"),documentElement:new Element("html")};document.activeElement=document.body;
const window={document,location:{hash:"#/map",pathname:"/report.html",search:""},history:{replaceState(){}},AbortController,Set,Map,URLSearchParams,Promise,ELK:function(){},requestAnimationFrame:f=>f(),clearTimeout,setTimeout,innerWidth:1440,innerHeight:1000,addEventListener(){},removeEventListener(){},RepomapUI:{message:(id,p)=>id+(p&&p.count!=null?":"+p.count:"")}};
const sandbox={window,document,Element,AbortController,Set,Map,URLSearchParams,Promise,requestAnimationFrame:f=>f(),clearTimeout,setTimeout,console,addEventListener(){},removeEventListener(){},WheelEvent:{DOM_DELTA_LINE:1,DOM_DELTA_PAGE:2}};sandbox.global=sandbox;vm.createContext(sandbox);vm.runInContext(fs.readFileSync(process.argv[2],"utf8"),sandbox);
const incoming=Array.from({length:5},(_,i)=>({component_id:"in"+i,name:"Incoming "+i,kind:"incoming",relation_count:i+1}));
const outgoing=Array.from({length:5},(_,i)=>({component_id:"out"+i,name:"Outgoing "+i,kind:"outgoing",relation_count:i+2}));
const ids=["center",...incoming.map(x=>x.component_id),...outgoing.map(x=>x.component_id)];
const components=ids.map(id=>({id,name:id==="center"?"Storage and WAL":id,subsystem_id:"system",members:[]}));
const structural_edges=[
 ...incoming.map((n,i)=>({id:"incoming-edge-"+i,from_component_id:n.component_id,to_component_id:"center",witness:{kind:"package_import"}})),
 ...outgoing.map((n,i)=>({id:"outgoing-edge-"+i,from_component_id:"center",to_component_id:n.component_id,witness:{kind:"package_import"}})),
 {id:"unrelated-0",from_component_id:"in0",to_component_id:"out0",witness:{kind:"package_import"}},
 {id:"unrelated-1",from_component_id:"in1",to_component_id:"out1",witness:{kind:"package_import"}},
];
const structural_relations=Array.from({length:7},(_,i)=>({id:"op-"+i,from_label:"from"+i,to_label:"to"+i,location:i<6?{path:"pkg/op"+i+".go",line:i+10,column:2}:null}));
const contexts={};ids.forEach(id=>contexts[id]={sources:[],surface_starts:[],studies:[],package_targets:[],package_paths:[],structural_relations:id==="center"?structural_relations:[]});
const associationComponents=ids.map(id=>({component_id:id,incoming:[],outgoing:[],associations:[]}));
associationComponents[0].incoming=incoming;associationComponents[0].outgoing=outgoing;
associationComponents.find(x=>x.component_id==="in0").outgoing=[{component_id:"center",name:"Storage and WAL",kind:"outgoing",relation_count:1},{component_id:"out0",name:"out0",kind:"outgoing",relation_count:1}];
const host=new Element("div"),opened=[];
const app=window.RepomapArchitectureCanvas.mount(host,{components,subsystems:[{id:"system",name:"System",component_ids:ids}],groups:[],structural_edges,behavior_anchors:[],relations:[],surfaces:[],flows:[]},{userMode:true,message:window.RepomapUI.message,componentContexts:contexts,associations:{components:associationComponents},openLocation:(path,line,column)=>opened.push({path,line,column})});
app.ready.then(()=>{
 app.openComponent("center");
 let nodes=walk(host),edges=nodes.filter(n=>svgCls(n,"rm-arch__edge--structural"));
 const centerVisible=edges.filter(n=>n.style.display!=="none").length,centerHidden=edges.filter(n=>n.style.display==="none").length;
 const markerCount=nodes.filter(n=>n.tagName==="PATH"&&n.getAttribute("marker-end")==="url(#rm-arch-structural-arrow)").length;
 let tabs=nodes.filter(n=>n.getAttribute&&n.getAttribute("role")==="tab");tabs[1].click();
 nodes=walk(host);
 const relationButtons=nodes.filter(n=>cls(n,"rm-arch__component-relation"));
 const relationOverflow=nodes.filter(n=>cls(n,"rm-arch__relation-overflow")&&walk(n).some(x=>cls(x,"rm-arch__component-relation")));
 const operationRows=nodes.filter(n=>cls(n,"rm-arch__operation-row"));
 const operationActions=operationRows.filter(n=>n.tagName==="BUTTON"&&cls(n,"is-action"));
 const operationStatic=operationRows.filter(n=>n.tagName!=="BUTTON"&&cls(n,"is-static"));
 if(operationActions[0])operationActions[0].click();
 if(relationButtons[0])relationButtons[0].click();
 nodes=walk(host);edges=nodes.filter(n=>svgCls(n,"rm-arch__edge--structural"));tabs=nodes.filter(n=>n.getAttribute&&n.getAttribute("role")==="tab");
 const selected=nodes.find(n=>cls(n,"rm-arch__component")&&cls(n,"is-selected"));
 process.stdout.write(JSON.stringify({centerVisible,centerHidden,markerCount,relationButtonCount:relationButtons.length,relationOverflowCount:relationOverflow.length,operationRowCount:operationRows.length,operationActionCount:operationActions.length,operationStaticCount:operationStatic.length,opened,selected:nodeText(selected),neighborVisible:edges.filter(n=>n.style.display!=="none").length,connectionsRetained:tabs[1]&&tabs[1].getAttribute("aria-selected")==="true"}));
}).catch(e=>{process.stdout.write(JSON.stringify({error:String(e&&e.stack||e)}));process.exit(2);});
`
	runnerPath := filepath.Join(t.TempDir(), "d272-component-ego-graph.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, canvasPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run D272 component journey: %v\n%s", err, output)
	}
	var got struct {
		CenterVisible, CenterHidden, MarkerCount                      int
		RelationButtonCount, RelationOverflowCount                    int
		OperationRowCount, OperationActionCount, OperationStaticCount int
		Opened                                                        []struct {
			Path         string
			Line, Column int
		}
		Selected            string
		NeighborVisible     int
		ConnectionsRetained bool
		Error               string
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode D272 component journey: %v\n%s", err, output)
	}
	if got.Error != "" {
		t.Fatalf("D272 component journey failed: %s", got.Error)
	}
	if got.CenterVisible != 10 || got.CenterHidden != 2 || got.MarkerCount != 12 {
		t.Fatalf("selected center edge projection = visible %d hidden %d markers %d, want 10/2/12", got.CenterVisible, got.CenterHidden, got.MarkerCount)
	}
	if got.RelationButtonCount != 10 || got.RelationOverflowCount != 2 {
		t.Fatalf("component relation rows = %d with %d disclosures, want all 10 behind two bounded sections", got.RelationButtonCount, got.RelationOverflowCount)
	}
	if got.OperationRowCount != 7 || got.OperationActionCount != 6 || got.OperationStaticCount != 1 {
		t.Fatalf("operation rows = total %d action %d static %d, want 7/6/1", got.OperationRowCount, got.OperationActionCount, got.OperationStaticCount)
	}
	if len(got.Opened) != 1 || got.Opened[0].Path != "pkg/op0.go" || got.Opened[0].Line != 10 || got.Opened[0].Column != 2 {
		t.Fatalf("exact operation action = %#v", got.Opened)
	}
	if !strings.Contains(got.Selected, "in0") || got.NeighborVisible != 2 || !got.ConnectionsRetained {
		t.Fatalf("related-component navigation selected %q with %d local edges, connections retained=%v", got.Selected, got.NeighborVisible, got.ConnectionsRetained)
	}
}

func TestD272RelationAndStaticOperationAffordancesAreDistinct(t *testing.T) {
	css := readCanvasAsset(t, "architecture_canvas.css")
	for _, token := range []string{
		".rm-arch__component-relation {",
		"cursor: pointer",
		".rm-arch__relation-overflow {",
		".rm-arch__operation-row.is-static {",
		"border-left: 2px solid",
	} {
		if !strings.Contains(css, token) {
			t.Errorf("D272 affordance contract is missing %q", token)
		}
	}
}
