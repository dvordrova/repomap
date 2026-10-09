package report

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"html/template"
	"io"
	"os/exec"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// A DOM with the child-combinator selectors the component reading uses
// (":scope>.a>.b", "li.x:not([y])", "[id]"), cloning, and sibling
// insertion; no layout.
const shapeElements = `
const camel=k=>k.replace(/^data-/,'').replace(/-(\w)/g,(_,c)=>c.toUpperCase());
class N{
 constructor(tag){this.tagName=String(tag).toUpperCase();this.children=[];this.parent=null;this.className='';this.own='';this.dataset={};this.listeners={};this.id='';this.title='';this.open=false;this.hidden=false;this.type='';
  const self=this;this.classList={add(c){if(!self.has(c))self.className=(self.className+' '+c).trim();},contains(c){return self.has(c);},toggle(){}};}
 has(c){return this.className.split(' ').includes(c);}
 adopt(c){if(typeof c==='string')c=text(c);if(c.parent&&c.parent.children)c.parent.children=c.parent.children.filter(x=>x!==c);c.parent=this;return c;}
 appendChild(c){this.children.push(this.adopt(c));return c;}
 append(...cs){cs.forEach(c=>this.appendChild(c));}
 prepend(...cs){cs.reverse().forEach(c=>this.children.unshift(this.adopt(c)));}
 insertBefore(c,ref){c=this.adopt(c);const at=this.children.indexOf(ref);if(at<0)this.children.push(c);else this.children.splice(at,0,c);return c;}
 after(...cs){const p=this.parent;let at=p.children.indexOf(this);cs.forEach(c=>{c=p.adopt(c);at=p.children.indexOf(this)+1;p.children.splice(at,0,c);});}
 remove(){if(this.parent)this.parent.children=this.parent.children.filter(c=>c!==this);this.parent=null;}
 replaceWith(c){const p=this.parent;c=p.adopt(c);p.children[p.children.indexOf(this)]=c;this.parent=null;}
 replaceChildren(...cs){this.children=[];this.append(...cs);}
 get firstChild(){return this.children[0]||null;}
 get childElementCount(){return this.children.filter(c=>c instanceof N).length;}
 get textContent(){return this.own+this.children.map(c=>c.textContent).join('');}
 set textContent(v){this.own=String(v);this.children=[];}
 set innerHTML(v){this.own=String(v);}
 addEventListener(k,f){this.listeners[k]=f;} click(){if(this.listeners.click)this.listeners.click({stopPropagation(){},preventDefault(){},button:0});}
 setAttribute(k,v){if(k.startsWith('data-'))this.dataset[camel(k)]=v;else this[k]=v;}
 getAttribute(k){return k.startsWith('data-')?this.dataset[camel(k)]??null:this[k]??null;}
 removeAttribute(k){if(k.startsWith('data-'))delete this.dataset[camel(k)];else this[k]='';}
 cloneNode(){const c=new N(this.tagName);Object.assign(c,{className:this.className,own:this.own,id:this.id,title:this.title,open:this.open,dataset:{...this.dataset}});
  const self=c;c.classList={add(x){if(!self.has(x))self.className=(self.className+' '+x).trim();},contains(x){return self.has(x);},toggle(){}};
  this.children.forEach(k=>c.appendChild(k instanceof N?k.cloneNode(true):text(k.textContent)));return c;}
 each(out=[]){for(const c of this.children)if(c instanceof N){out.push(c);c.each(out);}return out;}
 all(test){return this.each().filter(test);}
 is(compound){
  const m=/^([a-z0-9]*)((?:\.[\w-]+)*)((?:\[[\w-]+\])*)(?::not\(\[([\w-]+)\]\))?$/i.exec(compound);if(!m)throw new Error('selector '+compound);
  if(m[1]&&this.tagName!==m[1].toUpperCase())return false;
  if(m[2]&&!m[2].slice(1).split('.').every(c=>this.has(c)))return false;
  const attr=a=>a==='id'?!!this.id:a.startsWith('data-')?this.dataset[camel(a)]!==undefined:!!this[a];
  if(m[3]&&!m[3].slice(1,-1).split('][').every(attr))return false;
  if(m[4]&&attr(m[4]))return false;
  return true;
 }
 matchesFrom(parts,root){let el=this;for(let i=parts.length-1;i>=0;i--){if(parts[i]===':scope'){if(el!==root)return false;continue;}if(!el||!(el instanceof N)||!el.is(parts[i]))return false;if(i>0)el=el.parent;}return true;}
 querySelectorAll(selector){const lists=selector.split(',').map(s=>s.trim().split('>').map(p=>p.trim()));return this.each().filter(el=>lists.some(parts=>el.matchesFrom(parts,this)));}
 querySelector(selector){return this.querySelectorAll(selector)[0]||null;}
}
const text=value=>({textContent:String(value)});
const document={createElement:tag=>new N(tag),createTextNode:text,getElementById:()=>null};
function rmEl(tag,cls,value){const item=document.createElement(tag);if(cls)item.className=cls;if(value!==undefined)item.textContent=value;return item;}
function rmT(key,...values){return values.reduce((s,v,i)=>s.replace('{'+i+'}',v),key);}
const repomapMembers={sourceLink(s){const a=rmEl(s.Href||s.Open?'a':'span','',s.Text);a.href=s.Href;return a;},sourceKey(s){return s.Key||s.Href||s.Open||(s.NoSource?(s.Path?JSON.stringify([s.Path,s.Line||0]):s.Text):'');},symbolKey(s){return s?s.decl_key||s.href||s.open||(s.path?JSON.stringify([s.path,s.line||0]):''):'';},declKey(d){if(!d)return '';if(d.key||d.href||d.open)return d.key||d.href||d.open;const p=d.no_source?/^(.*):(\d+)$/.exec(d.source||d.at||''):null;return p?JSON.stringify([p[1],Number(p[2])]):'';}};
const el=(tag,cls,kids,extra)=>{const e=rmEl(tag,cls);(kids||[]).forEach(k=>e.appendChild(typeof k==='string'?text(k):k));Object.assign(e.dataset,(extra||{}).dataset||{});if(extra&&extra.id)e.id=extra.id;return e;};
`

// A component's reading is at most five sections, counting nothing
// (owner, 2026-09-29, after the critic: nine sections had run to ten
// screens with up to fifty-three standalone digits): its summary, entry and
// the kinds of its inputs; its Main flow closed by what it runs on its own;
// its files; its connections (mounted beside it); one line of links. Its
// areas and parts are the canvas's; what its entrypoints do not reach, its
// TODOs and its analysis coverage are the "What is missing" page's.
func TestAComponentsReadingIsAtMostFiveSectionsWithNoDigits(t *testing.T) {
	code := systemJSPiece(t, "31-reading-column.js", "function rmGroupReading(", "// An Inputs collection's reading") +
		systemJSPiece(t, "31-reading-column.js", "var rmLanguageNames=", "function rmCollectionView(") +
		systemJSPiece(t, "31-reading-column.js", "// Pointing at inputs in the column lights", "// Where a catalogue's inputs are declared") +
		systemJSPiece(t, "31-reading-column.js", "var rmPendingKind=", "// The home's table of programs") +
		systemJSPiece(t, "32-flow.js", "var rmFlowHelpers", "// </flow>")
	runSystemJS(t, shapeElements+code+`
const part={id:'t1-g1',dataset:{title:'Server lifecycle and cron'},getAttribute:()=>'#t1-g1'};
const ctx={nodeByHref:h=>h==='#t1-g1'?part:null,nodeById:()=>null,goDecl:()=>null,readDeclIn(){},readNode(){},light(){}};
const map={readingContext:()=>ctx};
// The component's page keeps only the original saved reading reference.
const details=el('section','',[el('header','component-intro'),
 el('details','component-reference',[el('summary','',['Code, entrypoints and sources']),el('h3','',['Runs code it is given'])])],{dataset:{componentFlow:JSON.stringify({
 Flow:{Parts:[{Title:'Startup and event loop'}],Steps:[{Label:'main',Part:'#t1-g1',Key:'h#main',Explanation:'It runs main.'},{Label:'initServer',Part:'#t1-g1',Key:'h#init',Explanation:'It runs initServer.'}]},
 Own:[{Label:'serverCron',Input:'t1-o3'}]})}});
const card=el('div','map-card',[el('div','map-card-intro',[el('p','model map-card-summary',['Serves clients.']),el('div','map-card-actions',[el('a','map-details-link',['Open component'])])])]);
const n={id:'system-component-t1',dataset:{owner:'t1',entries:JSON.stringify([{name:'main',callable:true,part:'#t1-g1',key:'h#main',href:'h#main'}]),
 files:JSON.stringify({decls:[{name:'rdbSave',kind:'function',part:'#t1-g1',href:'h#save'}],files:[{path:'dump.rdb',by:[{part:'#t1-g1',title:'Server lifecycle and cron',decls:[0]}]},{by:[{part:'#t1-g1',decls:[0]}]}]})}};
const collection={dataset:{collection:JSON.stringify({groups:[],kinds:[{kind:'request',inputs:Array.from({length:96},(_,i)=>'r'+i)},{kind:'setting',inputs:['s1','s2']}]})}};
rmComponentReading(map,n,card,details,collection,false);
// Its sections as the column lays them out: the intro's blocks, then the
// card's; a run of blocks with no heading of their own is one section.
const blocks=[];for(const c of card.children){if(c.has('map-card-intro'))blocks.push(...c.children.filter(k=>k instanceof N));else blocks.push(c);}
let sections=0,plain=false;
for(const b of blocks){const own=['DETAILS','SECTION','UL','OL','DL','NAV'].includes(b.tagName)||/^H[1-6]$/.test((b.children[0]||{}).tagName||'');if(own){sections++;plain=false;}else if(!plain){sections++;plain=true;}}
assert.ok(sections+1<=5,'at most five sections, the connections included: '+(sections+1));
const said=card.textContent;
for(const gone of ['Areas and parts','Not reachable','Analysis coverage','TODOs','Runs code it is given','Read or written by','Path not established'])assert.ok(!said.includes(gone),'no '+gone);
const digits=said.split(/\s+/).map(t=>t.replace(/^[·×+(\[{"']+|[)\]}"':;,.·%]+$/g,'')).filter(t=>/^\d[\d,]*$/.test(t));
assert.deepEqual(digits,[],'no standalone digits: '+said);
assert.ok(said.includes('Incoming requests')&&said.includes('Settings'),'its inputs by kind, in words');
const flow=card.all(c=>c.tagName==='DETAILS'&&c.children[0]&&c.children[0].textContent==='Main flow')[0];
assert.ok(flow&&flow.all(c=>c.has('map-component-own')).length===1,'what it runs on its own closes its Main flow');
const files=card.all(c=>c.has('map-component-files'))[0];
assert.equal(files.all(c=>c.has('map-file')).length,1,'a file whose path is not established is not listed');
assert.equal(card.all(c=>c.has('map-component-page')).length,1,'its whole page leads the line of links');
`)
}

// The Main flow in a component's reading says what its steps are (external
// review, 2026-10-02): each run of steps in one part stands under that
// part's box, its description on hover and a click reading it; a type's
// line is clicked open; an input a step handles reads that input, named as
// the map names it; the line saying where the path stops closes the flow.
func TestAComponentsMainFlowStandsStepsUnderTheirPartsAndNamesTheirInputs(t *testing.T) {
	code := systemJSPiece(t, "31-reading-column.js", "function rmGroupReading(", "// An Inputs collection's reading") +
		systemJSPiece(t, "31-reading-column.js", "// Pointing at inputs in the column lights", "// Where a catalogue's inputs are declared") +
		systemJSPiece(t, "31-reading-column.js", "var rmPendingKind=", "// The home's table of programs") +
		systemJSPiece(t, "32-flow.js", "var rmFlowHelpers", "// </flow>") +
		systemJSPiece(t, "29-operation-view.js", "// A type's line, the model's, reads its first sentence", "// Where a component's \"Entrypoints\" link lands")
	runSystemJS(t, shapeElements+code+`
const part={id:'t1-g1',dataset:{title:'Trading bot core',summary:'Runs the trading loop.'},getAttribute:()=>'#t1-g1'};
const input={id:'t1-o9',dataset:{title:'trade'}};
const read=[];
const ctx={nodeByHref:h=>h==='#t1-g1'?part:null,nodeById:id=>id==='t1-o9'?input:null,goDecl:()=>null,readDeclIn(){},readNode(node){read.push(node.id);},light(){}};
const map={readingContext:()=>ctx};
const details=el('section','',[el('header','component-intro')],{dataset:{componentFlow:JSON.stringify({Flow:{Steps:[
 {Label:'start_trading',Part:'#t1-g1',Key:'h#start_trading',PartHead:'#t1-g1',Handles:[{Words:'handles the command',Names:[{Name:'trade',Input:'t1-o9'}]}]},
 {Label:'FreqtradeBot.process',Part:'#t1-g1',Key:'h#FreqtradeBot.process',TypeName:'FreqtradeBot',TypeLine:'the main trading bot'},
 {Label:'IStrategy.adjust',Part:'#t1-g1',Key:'h#IStrategy.adjust',PartHead:'#t1-g7'}]}})}});
const card=el('div','map-card',[el('div','map-card-intro',[el('p','model map-card-summary',['Trades.'])])]);
rmComponentReading(map,{id:'system-component-t1',dataset:{owner:'t1'}},card,details,null,false);
const flow=card.all(c=>c.tagName==='DETAILS'&&c.children[0]&&c.children[0].textContent==='Main flow')[0];
const heads=flow.all(c=>c.has('flow-part-head'));
assert.equal(heads.length,1,'a part the map does not draw stands no head');
const box=heads[0].children[0];
assert.ok(box.has('map-part-box')&&box.textContent==='Trading bot core'&&box.title==='Runs the trading loop.','the part by its title, its description on hover');
const name=flow.all(c=>c.has('flow-input'))[0];
assert.equal(name.tagName,'BUTTON');assert.equal(name.textContent,'trade');
name.click();assert.deepEqual(read,['t1-o9'],'the input reads its own reading');
assert.ok(flow.all(c=>c.has('flow-type'))[0].listeners.click,'a type line opens on a click');
const order=flow.children.map(c=>c.className);
assert.ok(order.indexOf('meta flow-end')>order.indexOf('flow'),'the closing line follows the steps: '+order);
`)
}

// Existing native-flow regressions execute the actual selected-reading DOM
// formatter. The component template must carry data, never a static flow copy.
func executeComponentFlowDisplay(t *testing.T, parsed *template.Template, out *bytes.Buffer, language DisplayLanguage, section *pageSection) error {
	t.Helper()
	var html bytes.Buffer
	if err := parsed.ExecuteTemplate(&html, "target.html", section); err != nil {
		return err
	}
	if bytes.Contains(html.Bytes(), []byte(`class="component-flow"`)) || bytes.Contains(html.Bytes(), []byte(`class="component-own-work"`)) {
		t.Fatal("the component still prints a second flow reading")
	}
	raw, err := section.FlowJSON()
	if err != nil {
		return err
	}
	if raw == "" {
		return nil
	}
	return executeFlowDisplayJS(t, out, language, `rmFlowDisplay.flow(saved,into);(saved.Own||[]).forEach(function(row){into.appendChild(rmFlowDisplay.own(row));});`, raw)
}

func executeFlowDisplayJS(t *testing.T, out *bytes.Buffer, language DisplayLanguage, draw, raw string) error {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node required")
	}
	vocabulary, err := uiVocabulary(language)
	if err != nil {
		return err
	}
	words, err := json.Marshal(vocabulary)
	if err != nil {
		return err
	}
	factory := systemJSPiece(t, "31-reading-column.js", "var rmFlowDisplay=", "function rmComponentFlowData(")
	script := shapeElements + `
rmT=function(key,...values){if(!Object.prototype.hasOwnProperty.call(vocabulary,key))throw Error('missing UI '+key);return values.reduce((s,v,i)=>s.replace('{'+i+'}',v),vocabulary[key]);};
const vocabulary=` + string(words) + ";\n" + factory + `
const saved=` + raw + `,into=rmEl('div');
` + draw + `
function escaped(value){return String(value).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&#34;',"'":'&#39;'}[c]));}
function write(node){
 if(!(node instanceof N))return escaped(node.textContent);
 let attrs='';if(node.className)attrs+=' class="'+escaped(node.className)+'"';
 for(const k of ['id','href','target','title'])if(node[k])attrs+=' '+k+'="'+escaped(node[k])+'"';
 for(const [key,value] of Object.entries(node.dataset)){const k='data-'+key.replace(/[A-Z]/g,c=>'-'+c.toLowerCase());attrs+=' '+k+(k==='data-folded'?'':'="'+escaped(value)+'"');}
 return '<'+node.tagName.toLowerCase()+attrs+'>'+escaped(node.own)+node.children.map(write).join('')+'</'+node.tagName.toLowerCase()+'>';
}
process.stdout.write(into.children.map(write).join(''));
`
	encoded, err := exec.CommandContext(t.Context(), node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("selected flow DOM: %v\n%s", err, encoded)
	}
	out.Write(encoded)
	return nil
}

// Independent snapshot of the pre-migration component flow/own-work template.
// The test oracle is frozen; the ordinary target never prints this reading.
const componentFlowHTMLOracle = "{{/* The Main flow is read at the top of the component's reading; this copy is its source there, never shown twice (owner, 2026-09-29). */}}{{if or .Flow .Start}}<section class=\"component-flow\" hidden><h3 id=\"{{$.ID}}-flow\">{{t \"Main flow\"}}</h3>\n{{if .Flow}}{{with .Flow.Parts}}<p class=\"model flow-title flow-parts\">{{range $i, $part := .}}{{if $part.Or}} {{t \"or\"}} {{else if $i}} → {{end}}{{if $part.Href}}<a href=\"{{$part.Href}}\" data-display-ref=\"{{$part.TitleRef}}\">{{$part.Title}}</a>{{else}}{{$part.Title}}{{end}}{{end}}</p>{{end}}<ol class=\"flow\">{{range .Flow.Steps}}{{template \"flow-step\" .}}{{end}}</ol><p class=\"meta flow-end\">{{t \"The path stops here. At each step it follows one call the step before may make; open a step for all of its calls.\"}}</p>{{else}}{{if .Start}}<p class=\"meta\">{{t \"Read forward from where it starts:\"}}</p><ol class=\"flow fact\">{{range .Start}}<li class=\"flow-step\"{{with .Part}} data-step-part=\"{{.}}\"{{end}}{{with .Key}} data-step-key=\"{{.}}\"{{end}}{{with .Code}} data-step-code=\"{{pagedataLink .}}\"{{end}}><span class=\"flow-what\"><code>{{.Symbol}}</code>{{if .Group}} <span class=\"flow-where\">{{t \"in\"}} <a href=\"{{.Href}}\">{{.Group}}</a></span>{{end}}</span>{{if .Anchor}} {{template \"anchor\" .Anchor}}{{end}}{{if .Reaches}}<span class=\"flow-why flow-reaches\">{{range .Reaches}}<span class=\"flow-reach\">{{.Label}} <a href=\"{{.Href}}\">{{.Title}}</a></span>{{end}}</span>{{else if .Silent}}<span class=\"flow-why flow-calls-nothing\">{{t \"calls nothing\"}}</span>{{else if .Calls}}<span class=\"flow-why flow-reaches flow-reaches-own\">{{t \"it calls\"}} {{range $i, $call := .Calls}}{{if $i}}, {{end}}{{if $call.More}}…{{else}}<code>{{template \"anchor\" $call.Anchor}}</code>{{if $call.Unresolved}} <span class=\"meta\">({{t \"implementation not established\"}})</span>{{end}}{{with $call.Implementers}} <span class=\"meta\">({{t \"implemented in this repository by\"}} {{range $j, $name := .}}{{if $j}}, {{end}}<code>{{$name}}</code>{{end}}; {{t \"by method set, not a traced call\"}})</span>{{end}}{{end}}{{end}}</span>{{end}}</li>{{end}}</ol>{{range .StartElsewhere}}<p class=\"meta flow-elsewhere\">{{t \"Elsewhere in\"}} <a href=\"{{.Href}}\">{{.Group}}</a>, {{t \"not from these entries:\"}}{{range $i, $row := .Rows}}{{if $i}};{{end}} {{if $row.FromSource}}{{template \"anchor\" $row.FromSource}}{{else}}{{$row.Label}}{{end}} → <a href=\"{{$row.Href}}\">{{$row.Title}}</a>{{end}}{{if .Uses}}{{if .Rows}};{{end}} {{t \"uses the headers or types of\"}} {{range $i, $row := .Uses}}{{if $i}}, {{end}}<a href=\"{{$row.Href}}\">{{$row.Title}}</a>{{with $row.OtherTarget}} ({{.}}){{end}}{{end}}{{end}}</p>{{end}}{{end}}{{end}}</section>{{end}}\n{{/* What the program runs on its own, read after its Main flow in the component's reading (ownWork). */}}{{with .OwnWork}}<section class=\"component-own-work\" hidden><p class=\"flow-own-title\">{{t \"Also runs on its own:\"}}</p><ul class=\"flow-own\">{{range .}}<li class=\"flow-own-step\" data-input=\"{{.Input}}\"{{template \"step-attrs\" .}}><span class=\"flow-what\"><code>{{.Label}}</code></span>{{if or .Registers .RunBy}} — <span class=\"flow-how\">{{$said := false}}{{range .Registers}}{{if $said}}; {{end}}{{$said = true}}{{template \"registration-chain\" .}}{{end}}{{range .RunBy}}{{if $said}}; {{end}}{{$said = true}}{{template \"runner-chain\" .}}{{end}}</span>{{end}}</li>{{end}}</ul></section>{{end}}\n"

// The old formatter is an independent display oracle: order, element roles,
// exact sources/keys and attribution refs agree, not merely selected substrings.
func TestComponentFlowDOMMatchesThePreviousCompleteReading(t *testing.T) {
	for _, language := range []DisplayLanguage{English, Russian} {
		parsed, err := template.New("report").Funcs(pageTemplateFuncs(language)).ParseFS(reportTemplateFS, "templates/html/*.html")
		if err != nil {
			t.Fatal(err)
		}
		parsed, err = parsed.New("old-component-flow").Parse(componentFlowHTMLOracle)
		if err != nil {
			t.Fatal(err)
		}
		at := &pageAnchor{Path: "flow.c", Line: 7, Text: "flow.c:7", Href: "https://github.com/o/r/blob/abc/flow.c#L7", Code: "https://github.com/o/r/blob/abc/flow.c#L7-L19", key: "flow.c:7:12:function:run"}
		local := &pageAnchor{Path: "worker.py", Line: 3, Text: "worker.py:3", Open: "source-3:3", key: "worker.py:3:8:function:worker"}
		missing := &pageAnchor{Path: "missing.c", Line: 4, Text: "missing.c:4", NoSource: true, key: "missing.c:4:2:function:lost"}
		condition := &pageGuard{Key: "on an error path", ConditionKey: "only if not", Condition: `ready && size < limit`, At: at}
		name := pageStepName{Name: `run <&"`, Part: "#t1-g1", Key: at.Key(), Code: at.Code, Possible: true, Implemented: true, Guard: condition, Loop: at}
		helper := pageStepName{Name: "helper", Part: "#t1-g2", Key: local.Key(), Open: local.Open, Handed: true}
		name.Through = []pageStepName{helper}
		passed := &pageFlowFork{From: &name, Names: []pageStepName{name, helper}, OneOf: true}
		handed := *passed
		handed.Names = append([]pageStepName{}, passed.Names...)
		handed.Names[0].HandedTo = "scheduler"
		leaf := pageFlowStep{Label: "run", Target: "worker", Anchor: at, Part: "#t1-g1", Key: at.Key(), Explanation: "Works <with> inputs & state.", ExplanationRef: "explain-1", TypeName: "Worker", TypeLine: "An independent worker.", TypeLineRef: "type-1", Through: []pageStepName{name}, ViaKey: "handed to {0}", ViaArg: "scheduler", ViaFrom: &name, Implemented: true, Guard: condition, Loop: local, Registers: []pageStepRegistration{{By: []pageStepName{name, helper}, At: at}, {By: []pageStepName{helper}, At: local}, {By: []pageStepName{name}, At: missing}}, RunBy: [][]pageStepName{{helper, name}, {name}}, Handles: []pageFlowHandles{{Words: "handles the request", Names: []pageFlowInput{{Name: "GET /", Input: "t1-o1"}}}, {Words: "handles the requests:", Names: []pageFlowInput{{Name: "GET /a", Input: "t1-o2"}, {Name: "GET /b", Input: "t1-o3"}}, Folded: true}}}
		root := leaf
		root.PartHead = "#t1-g1"
		root.Back = &helper
		root.BackIf = condition
		root.Guard = nil
		root.OneOf = true
		root.Ways = []pageFlowWay{{Head: leaf, Rest: []pageFlowStep{leaf, {Label: "lost", Anchor: missing}}, Folded: true}, {Head: leaf, Rest: []pageFlowStep{{Label: "local", Anchor: local}}}}
		root.Passed = passed
		root.Handed = &handed
		root.Fork = passed
		root.Stop = "From here this route also goes on as the way from {0}."
		root.Joins = []pageStepName{name, helper}
		root.OpenAt = missing
		plain := leaf
		plain.ViaKey = ""
		plain.Via = "saved native handover"
		plain.Fork = &pageFlowFork{Label: "saved alternatives", From: &helper, Names: []pageStepName{name}}
		plain.Back = nil
		own := []pageOwnWork{{pageFlowStep: leaf, Input: "t1-o4"}, {pageFlowStep: pageFlowStep{Label: "lost", Anchor: missing}, Input: "t1-o5"}}
		fixtures := []*pageSection{
			{ID: "t1", Flow: &pageFlow{Parts: []pageFlowPart{{Title: "Workers", TitleRef: "part-1", Href: "#t1-g1"}, {Title: "Alternatives", Or: true}}, Steps: []pageFlowStep{root, plain}}, OwnWork: own},
			{ID: "t1", Start: []pageStart{{Symbol: "run", Anchor: at, Group: "Workers", Href: "#t1-g1", Part: "#t1-g1", Key: at.Key(), Code: at.Code, Reaches: []pageConnection{{Label: "calls", Href: "#t1-g2", Title: "Other"}}}, {Symbol: "local", Anchor: local, Calls: []pageOwnCall{{Anchor: local, Unresolved: true, Implementers: []string{"Worker.run", "Other.run"}}, {More: true}}}, {Symbol: "lost", Anchor: missing, Silent: true}}, StartElsewhere: []pageElsewhere{{Group: "Workers", Href: "#t1-g1", Rows: []pageConnection{{FromSource: local, Label: "Calls", Href: "#t1-g2", Title: "Other"}, {Label: "unplaced", Href: "#t1-g3", Title: "Worker"}}, Uses: []pageConnection{{Href: "#t2-g1", Title: "Types", OtherTarget: "Library"}}}}, OwnWork: own},
			{ID: "t1", OwnWork: own},
		}
		for i, section := range fixtures {
			var old bytes.Buffer
			if err := parsed.ExecuteTemplate(&old, "old-component-flow", section); err != nil {
				t.Fatal(err)
			}
			source := old.String()
			flowPart := ""
			ownPart := ""
			if start := strings.Index(source, `class="component-flow"`); start >= 0 {
				after := strings.Index(source[start:], "</h3>") + start + len("</h3>")
				end := strings.Index(source[after:], "</section>") + after
				flowPart = source[after:end]
			}
			if start := strings.Index(source, `<ul class="flow-own">`); start >= 0 {
				after := start + len(`<ul class="flow-own">`)
				end := strings.Index(source[after:], "</ul>") + after
				ownPart = source[after:end]
			}
			var current bytes.Buffer
			if err := executeComponentFlowDisplay(t, parsed, &current, language, section); err != nil {
				t.Fatal(err)
			}
			want, got := completeFlowTokens(t, flowPart+ownPart), completeFlowTokens(t, current.String())
			if !reflect.DeepEqual(got, want) {
				a, _ := json.MarshalIndent(want, "", " ")
				b, _ := json.MarshalIndent(got, "", " ")
				t.Fatalf("%s fixture %d complete reading changed:\nold%s\nnew%s", language, i, a, b)
			}
		}
	}
}

type flowDisplayToken struct {
	Kind, Name string
	Attributes []string
}

func completeFlowTokens(t *testing.T, source string) []flowDisplayToken {
	t.Helper()
	source = strings.ReplaceAll(source, " data-folded>", ` data-folded="">`)
	decoder := xml.NewDecoder(strings.NewReader("<div>" + source + "</div>"))
	decoder.Strict = false
	decoder.Entity = xml.HTMLEntity
	var tokens []flowDisplayToken
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			row := flowDisplayToken{Kind: "start", Name: value.Name.Local}
			for _, attr := range value.Attr {
				row.Attributes = append(row.Attributes, attr.Name.Local+"="+strings.Join(strings.Fields(attr.Value), " "))
			}
			sort.Strings(row.Attributes)
			tokens = append(tokens, row)
		case xml.EndElement:
			tokens = append(tokens, flowDisplayToken{Kind: "end", Name: value.Name.Local})
		case xml.CharData:
			if text := strings.Join(strings.Fields(string(value)), " "); text != "" {
				tokens = append(tokens, flowDisplayToken{Kind: "text", Name: text})
			}
		}
	}
	return tokens
}
