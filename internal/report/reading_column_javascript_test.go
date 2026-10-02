package report

import "testing"

// A tiny element for the reading's script: enough of the DOM to build a
// list and read it back as text, without a browser.
const fakeElements = `
class El{constructor(tag){this.tagName=String(tag).toUpperCase();this.children=[];this.className='';this.own='';this.listeners={};this.dataset={};this.hidden=false;const self=this;this.classList={add(c){self.className=(self.className+' '+c).trim();},contains(c){return self.className.split(' ').includes(c);}};}
 appendChild(c){this.children.push(c);return c;} append(...c){this.children.push(...c);} replaceChildren(...c){this.children=c;this.own='';}
 get childElementCount(){return this.children.filter(c=>c instanceof El).length;}
 addEventListener(k,f){this.listeners[k]=f;} setAttribute(k,v){this[k]=v;}
 set textContent(v){this.own=String(v);this.children=[];} get textContent(){return this.own+this.children.map(c=>c.textContent).join('');}
 set innerHTML(v){this.html=v;}
 find(test){if(test(this))return this;for(const c of this.children){const f=c instanceof El&&c.find(test);if(f)return f;}return null;}
 all(test,out=[]){if(test(this))out.push(this);for(const c of this.children)if(c instanceof El)c.all(test,out);return out;}
}
const text=value=>({textContent:value});
const document={createElement:tag=>new El(tag),createElementNS:(ns,tag)=>new El(tag),createTextNode:text,createDocumentFragment:()=>new El('fragment')};
function rmEl(tag,cls,value){const item=document.createElement(tag);if(cls)item.className=cls;if(value!==undefined)item.textContent=value;return item;}
function rmT(key,...values){return values.reduce((s,v,i)=>s.replace('{'+i+'}',v),key);}
`

// nameBreaksJS is the column's name writer (rmDotBreaks), which every list
// of names in the column uses.
func nameBreaksJS(t *testing.T) string {
	return systemJSPiece(t, "31-reading-column.js", "var rmLongPiece=", "// One end of a relation: its name")
}

// A chosen declaration had no reading of its own: the panel said "No
// explanation saved", and who calls it and what it calls sat in the part's
// long relation lists. Its reading lists both, from those rows, grouped by
// the part at the other end, one line per declaration; choosing a line
// reads that declaration.
func TestDeclarationReadingListsCallersAndCalleesByPart(t *testing.T) {
	code := systemJSPiece(t, "30-map.js", "function rmDeclarationText(", "// The reading layer over the map")
	runSystemJS(t, fakeElements+`
const K='h#processInputBuffer';
function row(kind,from,to,fromName,toName,site,peer,sentence,possible){
  return {dataset:{kind,fromDecl:from,toDecl:to,fromName,toName},closest:s=>s==='.conn-group'?peer:null,
    querySelector:s=>s===':scope>p'?{firstChild:text(sentence||fromName+' calls '+toName)}:s==='.connection-sources .anchor'?{cloneNode:()=>rmEl('a','anchor',site)}:s===':scope>p>.possible'?(possible?{}:null):null};
}
const peer=(href,title)=>({querySelector:s=>s==='.conn-peer a'?{getAttribute:()=>href,textContent:title}:null});
const events=peer('#events','Event loop'),config=peer('#config','Server configuration');
const rows=[row('calls','h#read',K,'readQueryFromClient','processInputBuffer','redis.c:2414',null),
  row('calls',K,'h#cmd','processInputBuffer','processCommand','redis.c:2353',null),
  row('calls',K,'h#cmd','processInputBuffer','processCommand','redis.c:2380',null),
  row('passes_callback','h#loop',K,'aeMain','processInputBuffer','ae.c:335',events,'aeMain passes processInputBuffer as a callback',true),
  row('calls','h#free','h#zfree','freeClient','zfree','redis.c:1807',config)];
const group={querySelectorAll:s=>s==='.conn[data-kind]'?rows:[]};
document.getElementById=id=>id==='clients'?group:null;
const node={id:'n-clients',dataset:{title:'Client connections'},getAttribute:()=>'#clients'};
const eventNode={id:'n-events',dataset:{title:'Event loop'},getAttribute:()=>'#events'};
const map={explainSource(s){this.explained=s;},revealNode(n,all,s){this.revealed=[n.id,all,s];}};
`+code+`
const box=rmDeclarationRelations(map,node,K,[node,eventNode]);
assert.ok(!box.textContent.includes('zfree'),'a row with neither end at the declaration is not its relation');
box.find(e=>e.textContent==='processCommand'&&e.tagName==='BUTTON').listeners.click();
assert.deepEqual(map.explained,{key:'h#cmd'},'a callee in the same part is read in place');
box.find(e=>e.className==='map-concept-decl'&&e.textContent.startsWith('aeMain')).listeners.click();
assert.deepEqual(map.revealed,['n-events',false,{key:'h#loop'}],'a caller in another part is read in its part');
const none=rmDeclarationRelations(map,node,'h#nothing',[node]);
assert.ok(none.textContent&&!none.find(e=>e.tagName==='BUTTON'),'a declaration with no relation says so and names none');
assert.equal(rmDeclarationText({dataset:{symbols:JSON.stringify([{name:'processInputBuffer',href:'h1',text:'(c: redisClient *)'}])}},{name:'processInputBuffer',source:{Href:'h1'}}),'processInputBuffer(c: redisClient *)');
`)
}

// An evidence list opens all its folded lines at once and closes them
// again, and its button says what it will do also after folds were opened
// by hand. The owner asked for "all".
func TestOpenAllOpensEveryFoldOfItsList(t *testing.T) {
	code := systemJSPiece(t, "30-map.js", "function rmOpenAllWord(", "\n})();") + "\n})();"
	runSystemJS(t, `
function rmT(s){return s;}
const listeners={};
const list={};const folds=[0,1,2].map(()=>({open:false,parentElement:list,matches:s=>s==='.conn-fold'}));
const button={hidden:true,textContent:'Open all',closest:s=>s==='.connection-evidence'?list:null};
list.querySelectorAll=s=>s===':scope>.conn-fold'?folds:[];list.querySelector=s=>s===':scope>[data-open-all]'?button:null;
const document={querySelectorAll:s=>s==='[data-open-all]'?[button]:[],addEventListener:(k,f)=>{listeners[k]=f;}};
`+code+`
assert.equal(button.hidden,false,'the button is shown only with scripting');
const click=()=>listeners.click({target:{closest:s=>s==='[data-open-all]'?button:null}});
click();assert.deepEqual(folds.map(f=>f.open),[true,true,true]);assert.equal(button.textContent,'Close all');
click();assert.deepEqual(folds.map(f=>f.open),[false,false,false]);assert.equal(button.textContent,'Open all');
folds[0].open=true;listeners.toggle({target:folds[0]});assert.equal(button.textContent,'Open all','one open fold is not all of them');
folds[1].open=folds[2].open=true;listeners.toggle({target:folds[2]});assert.equal(button.textContent,'Close all');
`)
}

// Redis's "Source details · 11" under Persistence opened at the column's
// foot with three of its five lines below it, and Open all put four there.
// What the reader opens in the column scrolls into view, no further than
// bringing its summary to the column's top; closing scrolls nothing.
func TestWhatAReaderOpensInTheColumnComesIntoView(t *testing.T) {
	reveal := systemJSPiece(t, "30-map.js", "function rmRevealOpened(", "// An evidence list opens every folded line")
	word := systemJSPiece(t, "30-map.js", "function rmOpenAllWord(", "(function(){")
	listen := systemJSPiece(t, "30-map.js", "    content.addEventListener('click',function(event){", "    function sentences(")
	runSystemJS(t, reveal+word+`
let frames=[];const requestAnimationFrame=f=>frames.push(f),run=()=>{const now=frames;frames=[];now.forEach(f=>f());};
const content={scrollTop:0,getBoundingClientRect:()=>({top:100,bottom:600}),contains:()=>true,addEventListener(kind,f){this[kind]=f;}};
// A block whose summary stands at 500 and foot at 900 before any scroll.
const block=(top,bottom,open=false)=>({tagName:'DETAILS',open,getBoundingClientRect:()=>({top:top-content.scrollTop,bottom:bottom-content.scrollTop})});
const summaryOf=details=>({parentElement:details,closest:s=>s==='summary'?summaryOf(details):null});
// Where a block's top and foot stand in the column (100 to 600) after the scroll.
const whole=(top,bottom)=>top-content.scrollTop>=100&&bottom-content.scrollTop<=600;
const summaryAtTop=top=>top-content.scrollTop>=100&&top-content.scrollTop<=116;
`+listen+`
const evidence=block(500,900);
content.click({target:summaryOf(evidence)});evidence.open=true;run();
assert.ok(whole(500,900),'the opened block is brought whole into the column');
const tall=block(500,2000);content.scrollTop=0;
content.click({target:summaryOf(tall)});tall.open=true;run();
assert.ok(summaryAtTop(500),'a block taller than the column stops at its summary');
content.scrollTop=0;content.click({target:summaryOf(tall)});tall.open=false;run();
assert.equal(content.scrollTop,0,'closing scrolls nothing');
const shown=block(200,400);content.click({target:summaryOf(shown)});shown.open=true;run();
assert.equal(content.scrollTop,0,'a block already in view stays');
// Open all: the list comes into view once its folds are open, not when closed.
const folds=[{open:false},{open:false}],list=block(500,1400,true);list.querySelectorAll=s=>s===':scope>.conn-fold'?folds:[];
const button={closest:s=>s==='[data-open-all]'?button:s==='details'||s==='.connection-evidence'?list:null};
content.click({target:button});folds.forEach(f=>f.open=true);run();
assert.ok(summaryAtTop(500),'Open all brings the list to the column top');
content.scrollTop=0;content.click({target:button});folds.forEach(f=>f.open=false);run();
assert.equal(content.scrollTop,0,'Close all scrolls nothing');
`)
}

// A part read while an input is pinned says, in its heading, that it is off
// the input's path. Drawn from the canvas's emphasis, the line came a frame
// late and again each time the pointer left the canvas for the column,
// pushing the list 26 px under the reader's click: monitorCommand was read
// for pingCommand. It is a fact of the reading, drawn with the reading.
func TestAPartOffThePinnedInputSaysSoWithItsReading(t *testing.T) {
	projection := systemJSPiece(t, "29-operation-view.js", "function rmSystemProjection(", "// An input chosen from Find, a link")
	mark := systemJSPiece(t, "29-operation-view.js", "  function markOutside(n){", "  // A declaration named in the reading")
	runSystemJS(t, fakeElements+projection+`
const nodes=[{id:'server',children:['runtime']},{id:'runtime',children:['clients','strings','debug']},{id:'clients'},{id:'strings'},{id:'debug'},{id:'get',activation:'request'}];
const edges=[{from:'get',to:'strings',label:'implemented in',operations:['get']},{from:'strings',to:'clients',operations:['get']},{from:'clients',to:'debug',operations:[]}];
const projection=rmSystemProjection(nodes,edges);
assert.equal(projection.outside('debug','get'),true,'a part none of the path\'s arrows reach is off it');
assert.equal(projection.outside('clients','get'),false);
assert.equal(projection.outside('runtime','get'),false,'an area holding a part of the path is on it');
assert.equal(projection.outside('get','get'),false,'the input itself');
assert.equal(projection.outside('debug',''),false,'no input pinned');
const heading={children:[],appendChild(c){this.children.push(c);c.remove=()=>{this.children=this.children.filter(x=>x!==c);};return c;},querySelector(s){return this.children.find(c=>'.'+c.className===s)||null;}};
const map={querySelector:s=>s==='.map-inspector-heading'?heading:null};
let operation={id:'get'};
`+mark+`
markOutside({id:'debug'});
assert.equal(heading.children.length,1,'the reading says it at once');
markOutside({id:'debug'});assert.equal(heading.children.length,1,'said once');
markOutside({id:'clients'});assert.equal(heading.children.length,0,'a part on the path says nothing');
operation=null;markOutside({id:'debug'});assert.equal(heading.children.length,0);
`)
}

// "handled by getCommand" in GET's reading opened GitHub, two lines above
// the same name in its path that reads the declaration. The handler is read
// in its part when that part lists it; a modifier-click still opens code.
func TestAnInputsHandlerNameReadsItsDeclaration(t *testing.T) {
	code := systemJSPiece(t, "29-operation-view.js", "  function readsHandler(n,card){", "  map.addEventListener('repomap:inspect'")
	runSystemJS(t, `
const strings={id:'strings',dataset:{symbols:JSON.stringify([{name:'getCommand',href:'h/getCommand'}])}};
const byID={strings},scene={handlerPart:id=>({get:'strings',orphan:'strings'})[id]||''},read=[];
function readDeclaration(part,key){read.push([part.id,key]);}
const link=()=>({listeners:{},addEventListener(k,f){this.listeners[k]=f;}});
const cardWith=name=>({querySelector:s=>s==='.map-card-handler>a'?name:null});
`+code+`
const name=link();readsHandler({id:'get',dataset:{activation:'request',handlerSource:'h/getCommand'}},cardWith(name));
const click=extra=>{let prevented=false;name.listeners.click({button:0,preventDefault(){prevented=true;},stopPropagation(){},...extra});return prevented;};
assert.equal(click({}),true,'a plain click reads the handler instead of opening its code');
assert.deepEqual(read,[['strings','h/getCommand']]);
assert.equal(click({ctrlKey:true}),false,'a modifier-click opens its code');assert.equal(read.length,1);
const other=link();readsHandler({id:'orphan',dataset:{activation:'request',handlerSource:'h/elsewhere'}},cardWith(other));
assert.equal(other.listeners.click,undefined,'a handler its part does not list stays the link it was');
`)
}

// The TODOs link of a component's reading landed on its heading at the
// foot of the page, "10 markers in 6 files" still closed under it. A
// heading reached by a link opens the list it heads.
func TestAHeadingReachedByALinkOpensTheListItHeads(t *testing.T) {
	setPage := systemJSPiece(t, "45-modes.js", "function setPage(node){", "  function address(")
	runSystemJS(t, `
const home={id:'overview'},repoMap={id:'repository-map'},questionPage={id:'questions'},page={id:'t1'};
const node=(tag,parent,next,extra={})=>({tagName:tag,parentElement:parent,nextElementSibling:next,textContent:'',closest:()=>null,matches:s=>s.split(',').includes(tag.toLowerCase()),...extra});
let current=home,question=null,term=null,searchIntent='',detailGroup=null,sectionTitle='';
const body={dataset:{}},pages=[home,page],document={querySelectorAll:()=>[]};
function enclosing(){return page;}function revealConcept(){}function placeSearch(){}function showReturn(){}function showLocation(){}function measureToolbar(){}function selectQuestion(){}
`+setPage+`
const reference=node('DETAILS',page,null),list=node('DETAILS',page,null),todos=node('H3',reference,list);
setPage(todos);
assert.equal(reference.open,true,'the section holding the heading opens, as before');
assert.equal(list.open,true,'the list under the heading opens');
const table=node('TABLE',page,null),config=node('H3',page,table);setPage(config);
assert.equal(table.open,undefined,'a heading over no list opens nothing more');
const flow=node('DETAILS',page,null),other=node('DIV',page,flow);setPage(other);
assert.equal(flow.open,undefined,'only a heading opens what follows it');
`)
}

// The top key painted its glyphs in the marks' dark colours and keyed no
// line at all: the solid and dashed arrows went unexplained. It keys a
// stroke only when the map draws one.
func TestKeyNamesTheStrokesTheMapDraws(t *testing.T) {
	code := systemJSPiece(t, "29-operation-view.js", "function rmKey(", "(function(){document.querySelectorAll('[data-map-explorer]')")
	runSystemJS(t, fakeElements+`
const nodes=[{dataset:{lane:'core'}},{dataset:{activation:'request'}}];
const category=n=>n.dataset.activation?'input':'part';
`+code+`
const labels=key=>key.children.map(c=>c.textContent);
assert.deepEqual(labels(rmKey(nodes,[{possible:false},{possible:true}],category)),['Core','Inputs','calls','possible calls']);
assert.deepEqual(labels(rmKey(nodes,[{possible:false}],category)),['Core','Inputs','calls'],'a map with no possible call keys none');
const stroke=rmKey(nodes,[{possible:true}],category).children.at(-1);
assert.ok(stroke.className.includes('flow-key-possible'),'drawn as the possible stroke');
`)
}

// Zoomed into a part, the tiles draw purple dashed links from a function to
// the type it returns and from a type to the function taking it; Redis's
// readers met them with nothing in the key saying what they were. The key
// names them when a part of the map has one, and not otherwise.
func TestKeyNamesTheTilesTypeLinks(t *testing.T) {
	code := systemJSPiece(t, "29-operation-view.js", "function rmKey(", "(function(){document.querySelectorAll('[data-map-explorer]')")
	runSystemJS(t, fakeElements+`
const category=n=>n.dataset.activation?'input':'part';
`+code+`
const labels=key=>key.children.map(c=>c.textContent);
const part=calls=>({dataset:{lane:'core',symbolCalls:JSON.stringify(calls)}});
const key=rmKey([part([[0,1,'calls'],[2,0,'returns']])],[{possible:false}],category);
assert.deepEqual(labels(key),['Core','calls','returns or takes a type']);
assert.ok(key.children.at(-1).className.includes('flow-key-types'),'drawn as the type link');
assert.deepEqual(labels(rmKey([part([[3,0,'takes']])],[],category)),['Core','returns or takes a type'],'a type a function takes is keyed too');
assert.deepEqual(labels(rmKey([part([[0,1,'calls']]),{dataset:{lane:'core'}}],[{possible:false}],category)),['Core','calls'],'a map whose tiles link no type keys none');
`)
}

// Find: "Back to search" came back to the top of a list the reader had
// scrolled, and choosing a component left the results over the map with
// the other component still chosen in them.
func TestFindReturnsToItsPlaceAndFollowsTheChosenComponent(t *testing.T) {
	closing := systemJSPiece(t, "40-find.js", "  var lastPlace=''", "  box.restoreSearch=")
	render := systemJSPiece(t, "40-find.js", "  function render(){", "  box.addEventListener('input',render);")
	runSystemJS(t, fakeElements+`
const box={value:'list'},panel={hidden:true},kind={value:'all'},component={value:'',options:[{value:''},{value:'t1'},{value:'t2'}]};
const results=new El('ol');let scrollTop=0;Object.defineProperty(results,'scrollTop',{get:()=>scrollTop,set:v=>{scrollTop=v;}});
const status=new El('p'),pages=new El('div');pages.querySelector=()=>({disabled:false,textContent:''});
let page=0,lastQuery='',pageSize=12,changes=0;
const entries=Array.from({length:30},(_,i)=>({title:'list'+i,summary:'',path:'adlist.c',component:'redis-server',section:'t1',kind:'part',type:'Part',haystack:'list'+i}));
function expanded(){} function changed(){changes++;} function rank(){return 0;} function action(label){return rmEl('button','',label);} function appendText(p,t,v){p.appendChild(rmEl(t,'',v));}
function description(){} function tileRows(){return null;} function membership(){}
document.documentElement={lang:'en'};
`+closing+render+`
render();results.scrollTop=640;
close(false);assert.equal(panel.hidden,true);
render();assert.equal(results.scrollTop,640,'the same list comes back where the reader left it');
kind.value='part';render();assert.equal(results.scrollTop,0,'another list starts at its top');
box.chooseComponent('t2');assert.equal(panel.hidden,true,'choosing a component closes the results');assert.equal(component.value,'t2','and searches it next');
box.chooseComponent('unknown');assert.equal(component.value,'');
`)
}

// Client connections' 95 reaching inputs stood open between the part's
// callers and its own connections. They are folded, by kind, counting
// nothing (owner, 2026-09-29: no digits in the column), and each still
// opens its input.
func TestInputsReachingAPartAreFolded(t *testing.T) {
	code := systemJSPiece(t, "31-reading-column.js", "var rmInputKindTitles=", "function rmCollectionView(") +
		systemJSPiece(t, "29-operation-view.js", "function rmReachingInputs(", "(function(){document.querySelectorAll('[data-map-explorer]')")
	runSystemJS(t, fakeElements+code+`
const get={dataset:{activation:'request',title:'get'}},cron={dataset:{activation:'continuous',title:'serverCron'}};
let chosen=null;
const inputs=rmReachingInputs({dataset:{}},[get,cron],()=>'redis-server',input=>{chosen=input;});
assert.equal(inputs.tagName,'DETAILS');assert.ok(!inputs.open,'the list is folded');
assert.equal(inputs.children[0].tagName,'SUMMARY');assert.ok(!/\d/.test(inputs.children[0].textContent),'counting nothing: '+inputs.children[0].textContent);
assert.equal(inputs.all(e=>e.tagName==='H6').length,2,'by kind');
inputs.find(e=>e.tagName==='BUTTON'&&e.textContent==='serverCron').listeners.click();
assert.equal(chosen,cron,'each input by its own name, the program said once');
// Past twelve, a kind's inputs stand in the canvas's groups, each closed
// under its part's name (reviewer, 2026-09-30: ninety-six rows in one list);
// several programs are each named once.
const many=Array.from({length:14},(_,i)=>({dataset:{activation:'request',title:'cmd'+i}}));
const grouped=rmReachingInputs({dataset:{}},many,input=>input.dataset.title==='cmd13'?'redis-cli':'redis-server',()=>{},input=>Number(input.dataset.title.slice(3))<7?'String commands':'List commands');
assert.deepEqual(grouped.all(e=>e.tagName==='H5').map(e=>e.textContent),['redis-server','redis-cli'],'each program once');
assert.deepEqual(grouped.all(e=>e.className==='system-reaching-group').map(e=>e.children[0].textContent),['String commands','List commands'],'the canvas groups, each closed');
assert.equal(grouped.all(e=>e.tagName==='BUTTON').length,14,'no input dropped');
`)
}

// A key type the model explained is listed from its concept before the
// part's code rows, and the concept carries neither the key mark nor the
// type's fields: redisClient read as a plain name with no fields.
func TestCodeInThisPartKeepsAKeyTypesMarkAndFields(t *testing.T) {
	code := systemJSPiece(t, "26-map-members.js", "var repomapMembers = (function () {", "\n})();") + "\n})();"
	runSystemJS(t, `
const reading={decls:[{name:'createClient',key:'h#2440',href:'h#2440',at:'redis.c:2440',kind:'function'},{name:'redisClient',key:'h#303',href:'h#303',at:'redis.c:303',kind:'type',bold:true,fields:[{name:'fd',href:'h#303#fd',at:'redis.c:304'},{name:'db',href:'h#303#db',at:'redis.c:305'}]}],
  members:[{kind:'function',decls:[0]},{kind:'type',decls:[1]}]};
function rmGroupReading(){return reading;}
const node={dataset:{concepts:'c',explained:'e'},getAttribute:()=>'#clients'};
rmPage.data=(n,name)=>name==='concepts'?[{name:'redisClient',explanation:'One connected client.',source:{Href:'h#303',Text:'redis.c:303'}}]:
  name==='explained'?[{name:'createClient',alias:'Client factory',explanation:'Makes a client.',source:{Href:'h#2440',Text:'redis.c:2440'}}]:null;
`+code+`
const items=repomapMembers.items(node);
assert.deepEqual(items.map(i=>[i.name,!!i.key,(i.fields||[]).map(f=>f.name)]),[['redisClient',true,['fd','db']],['createClient',false,[]]]);
assert.equal(items[0].explanation,'One connected client.','the model line stays');
assert.deepEqual([items[1].alias,items[1].explanation,items[1].source.Path,items[1].source.Line],['Client factory','Makes a client.','redis.c',2440],'a declaration keeps the model\'s alias and line, and its place');
`)
}

// Find: a declaration that no map holds had its title as the way to its
// row; one result per declaration had made that title plain text.
func TestFindOpensADeclarationNoMapHoldsAtItsRow(t *testing.T) {
	render := systemJSPiece(t, "40-find.js", "  function render(){", "  box.addEventListener('input',render);")
	runSystemJS(t, fakeElements+`
const box={value:'list'},panel={hidden:true},kind={value:'all'},component={value:'',options:[{value:''}]};
const results=new El('ol'),status=new El('p'),pages=new El('div');pages.querySelector=()=>({disabled:false,textContent:''});
let page=0,lastQuery='',pageSize=12,lastPlace='',lastScroll=0;
const source={tagName:'SPAN',querySelector:()=>null};
const code=(title,memberships)=>({title,summary:'',path:'adlist.c',component:'',section:'',sections:[],kind:'code',type:'Code',haystack:'list',source,memberships,destination:{}});
const entries=[code('listCreate:41',[]),code('listDup:90',[{title:'a'},{title:'b'}])];
function expanded(){} function changed(){} function close(){} function rank(){return 0;} function appendText(p,t,v){return p.appendChild(rmEl(t,'',v));}
function action(label,entry){const b=rmEl('button','',label);b.entry=entry;b.classList={add(){}};return b;} function description(){} function tileRows(e){return {head:e.title.replace(/:\d+$/,''),fields:[]};} function membership(e,m){return m;}
document.documentElement={lang:'en'};
`+render+`
render();
const heads=results.children.map(li=>li.children[0].children[0]);
assert.equal(heads[0].tagName,'BUTTON');assert.equal(heads[0].entry,entries[0],'the title opens the declaration where the report lists it');
assert.equal(heads[1].tagName,'STRONG','a declaration in several places is chosen by its In links');
`)
}

// A reading starts with what its subject is made of (owner's 3a): List
// commands read "Made of 18 functions" over its declarations by file and
// line, a column of line numbers. The list is the model's keys first, then
// every declaration by name, whatever its case, with no line number; the
// kinds its tiles carry are counted and its files named once.
func TestAPartsCompositionIsCountedAndItsCodeListedKeysFirstThenByName(t *testing.T) {
	code := systemJSPiece(t, "26-map-members.js", "var repomapMembers = (function () {", "\n})();") + "\n})();"
	runSystemJS(t, `
const document={getElementById:()=>null};
function rmT(key,...values){return values.reduce((s,v,i)=>s.replace('{'+i+'}',v),key);}
const concept=(name,href,path,key)=>({name,key,source:{Href:href,Path:path,Text:path+':1'}});
const node={getAttribute:()=>'',dataset:{
  concepts:JSON.stringify([concept('lpushCommand','h/lpush','redis.c'),concept('pushGenericCommand','h/push','redis.c'),concept('LZF_STATE','h/lzf','lzfP.h'),
    concept('blockingPopGenericCommand','h/block','redis.c',true),concept('_dictPanic','h/panic','dict.c'),concept('server','h/server','redis.c')]),
  symbols:JSON.stringify([{name:'lpushCommand',kind:'function',href:'h/lpush'},{name:'pushGenericCommand',kind:'function',href:'h/push'},{name:'LZF_STATE',kind:'type',href:'h/lzf'},
    {name:'blockingPopGenericCommand',kind:'function',href:'h/block',key:true},{name:'_dictPanic',kind:'method',href:'h/panic'},{name:'server',kind:'variable',href:'h/server'}])}};
`+code+`
assert.deepEqual(repomapMembers.sorted(node).map(i=>i.name),['blockingPopGenericCommand','_dictPanic','lpushCommand','LZF_STATE','pushGenericCommand','server']);
assert.deepEqual(repomapMembers.items(node).map(i=>i.name).slice(0,2),['lpushCommand','pushGenericCommand'],'the page\'s own order is kept for the tiles');
assert.deepEqual(repomapMembers.composition(node),{total:6,counts:'4 functions, 1 types, 1 variables',files:['dict.c','lzfP.h','redis.c']});
`)
}

// An area's reading starts with its parts, each in its box with its
// description and only its key declarations, one to a line, counting
// nothing (owner, 2026-09-29: "123 functions, 4 types" and 127 names
// several to a line had been a wall); a part leads to its reading and a key
// to its own, in its part, without moving the camera.
func TestAnAreasCompositionListsItsPartsAndTheirKeys(t *testing.T) {
	code := nameBreaksJS(t) + systemJSPiece(t, "29-operation-view.js", "  function areaComposition(", "  map.addEventListener('repomap:inspect'")
	runSystemJS(t, fakeElements+`
const part=(id,title,branch,summary)=>({id,dataset:{title,branch:branch||'',summary:summary||''}});
const byID={a:part('a','Compression','','Compresses dumps.'),b:part('b','Data structures'),c:part('c','Nested','area')};
function rmModelText(tag,cls,text){const e=rmEl(tag,cls+' model',text);return e;}
const members={a:[{name:'lzf_compress',key:true,source:{Href:'h/c'}},{name:'u8',source:{Href:'h/u8'}}],b:[{name:'listNext',source:{Href:'h/n'}}]};
const repomapMembers={sorted:p=>members[p.id]||[],composition:p=>({total:(members[p.id]||[]).length,counts:(members[p.id]||[]).length+' functions',files:p.id==='a'?['lzf.c']:[]}),
  sourceLink:s=>{const a=rmEl('a');a.href=s.Href;return a;},displayName:i=>i.name,sourceKey:s=>s.Href};
const chosen=[];function select(n,navigate,source,focus){chosen.push([n.id,source?source.key:undefined,focus]);}
`+code+`
const section=areaComposition({dataset:{children:'a b c'}});
assert.ok(!/\d/.test(section.textContent),'no counts: '+section.textContent);
const parts=section.all(e=>e.className==='map-area-part');
const keys=p=>p.all(e=>e.tagName==='LI').map(li=>li.textContent);
assert.deepEqual(parts.map(p=>[p.children[0].textContent,keys(p)]),[['Compression',['lzf_compress']],['Data structures',[]]],'each part with its keys alone, one to a line');
assert.equal(parts[0].children[1].textContent,'Compresses dumps.','its description');
assert.equal(parts[0].all(e=>e.tagName==='LI')[0].children[0].className,'map-member-key','a key is bold');
parts[0].children[0].listeners.click();
parts[0].all(e=>e.tagName==='LI')[0].children[0].listeners.click({preventDefault(){},stopPropagation(){}});
assert.deepEqual(chosen,[['a',undefined,false],['a','h/c',false]]);
assert.equal(areaComposition({dataset:{children:'c'}}),null,'an area of no parts has no composition');
`)
}

// A click on an arrow end reads the frame it stands on without moving the
// camera, and the reading knows which of its connections to open while it
// is drawn; afterwards a plain choice of the frame opens none.
func TestAnArrowEndOpensItsFramesConnectionInTheReading(t *testing.T) {
	code := systemJSPiece(t, "29-operation-view.js", "  var pendingConnection=null;", "  function reset(){")
	runSystemJS(t, `
const byID={'t1-area-k3':{id:'t1-area-k3'}},map={},seen=[];
function select(n,navigate,source,focus){seen.push([n.id,navigate,source,focus,pendingConnection&&pendingConnection.key]);return Promise.resolve(true);}
`+code+`
(async()=>{
  assert.equal(map.openConnection,openConnection);
  await openConnection('t1-area-k3','in:t1-area-k2');
  assert.deepEqual(seen,[['t1-area-k3',true,null,false,'in:t1-area-k2']],'the frame is read in place with that connection pending');
  assert.equal(pendingConnection,null,'once drawn, nothing is pending');
  assert.equal(await openConnection('gone','in:x'),false);
})().catch(error=>{console.error(error);process.exit(1);});
`)
}

// A chosen input's reading lists its path (owner's 3c): each dispatch site
// that dispatches it, the first open, "Dispatched from call · one of 94",
// saying that how a request for the input gets to the site is not
// established; not the inputs whose own code reaches the site, which a
// reader took for GET's route. A site the input's own handler calls reads
// apart ("exec's handler itself calls call"), so the two lines no longer
// read as one contradicting the other. Then the inputs registering it, and the parts it enters by depth,
// each with every call entering it from an earlier part, five and the rest
// folded, and the other calls counted, with no line number. A name reads its
// declaration in the report (getCommand and addReply had opened GitHub); a
// modifier-click still opens its code.
func TestAnInputsPathNamesItsDispatchThenItsOwnSteps(t *testing.T) {
	code := nameBreaksJS(t) + systemJSPiece(t, "29-operation-view.js", "function rmInputPathSection(", "(function(){document.querySelectorAll('[data-map-explorer]')") +
		systemJSPiece(t, "30-map.js", "function rmSiteHandlers(", "// A dispatch site read with its declaration")
	runSystemJS(t, fakeElements+`
const repomapMembers={sourceLink:s=>{const a=rmEl('a','',s.Text);a.href=s.Href;return a;}};
const decl=(name,part)=>({name,href:'h/'+name,source:'redis.c:1',part});
const path={dispatched:[{site:0,of:94,handlers:94,inputs:95,shared:[{handler:11,inputs:['t1-sinter','t1-smembers']}],reached_from:['t1-exec']},{site:1,of:94,handlers:94,inputs:95}],registered_by:['t1-accept'],
  spine:{steps:[{decl:2,part:'n-strings'},{decl:3,part:'n-strings'}],branches:[{decl:4,part:'n-keys'},{decl:5,part:'n-clients',helper:true},{decl:6,part:'n-clients',helper:true}]},
  parts:[{part:'n-strings',title:'String commands',depth:0,handler:2},{part:'n-dispatch',title:'Command dispatch',depth:1,entered:[[2,3,0]]},{part:'n-keys',title:'Keyspace',depth:2,entered:[[3,4,1]]},
    {part:'n-clients',title:'Client connections',depth:2,entered:[[3,5,0],[3,6,0],[3,7,0],[3,8,0],[3,9,0],[3,10,2]],others:3}],
  decls:[decl('call','n-clients'),decl('loadAppendOnlyFile','n-persist'),decl('getCommand','n-strings'),decl('getGenericCommand','n-strings'),decl('lookupKeyRead','n-keys'),
    decl('addReply','n-clients'),decl('addReplyBulk','n-clients'),decl('addReplyLong','n-clients'),decl('addReplySds','n-clients'),decl('addReplyDouble','n-clients'),decl('shared','n-clients'),decl('sinterCommand','n-sets')]};
const node=(id,title)=>({id,dataset:{title}});
const parts={'n-strings':node('n-strings','String commands'),'n-clients':node('n-clients','Client connections')},inputs={'t1-exec':node('t1-exec','exec'),'t1-accept':node('t1-accept','accept'),'t1-sinter':node('t1-sinter','sinter'),'t1-smembers':node('t1-smembers','smembers')},chosen=[],read=[];
`+code+`
const section=rmInputPathSection(path,'get',id=>parts[id]||null,id=>inputs[id]||null,part=>chosen.push(part.id),(part,key)=>read.push([part.id,key]));
const boxes=section.all(e=>e.className==='system-shared-path');
assert.deepEqual(boxes.map(b=>[b.tagName,!!b.open]),[['DETAILS',true],['DETAILS',false]],'each site dispatching it, the first open');
assert.ok(boxes[0].children[0].textContent.includes('call')&&boxes[1].children[0].textContent.includes('loadAppendOnlyFile'),'each named by its site');
assert.ok(boxes[0].textContent.includes('not established'),'how a request gets to the site is said to be unknown');
// Each count says what it counts: 94 handlers, 95 inputs, and why they differ.
const counts=boxes[0].children[1].title;
assert.ok(counts.includes('95')&&counts.includes('sinterCommand')&&counts.includes('sinter, smembers'),counts);
assert.ok(/94\D+90/.test(rmSiteHandlers({of:94,handlers:90})),'a site whose alternatives are not all handlers says so');
assert.ok(!section.textContent.includes('exec'),'the inputs reaching call are its reading, not get\'s');
section.find(e=>e.tagName==='BUTTON'&&e.textContent==='accept').listeners.click();
assert.deepEqual(chosen,['t1-accept'],'the input registering get leads to its reading');
const steps=section.children.at(-1);
const titles=box=>box.children.filter(c=>c.className==='system-path-part').map(c=>c.textContent);
const said=box=>box.all(e=>e.className==='system-path-step').map(c=>c.textContent);
// The handler's flow reads as its spine: getCommand, then getGenericCommand,
// whose work splits into lookupKeyRead, its helpers named on one line.
const spine=section.find(e=>e.className==='system-path-spine');
assert.deepEqual(spine.all(e=>e.className==='system-path-spine-step').map(c=>c.textContent),['String commandsgetCommand','String commandsgetGenericCommand']);
assert.deepEqual(spine.all(e=>e.className==='system-path-spine-branch').map(c=>c.textContent),['lookupKeyRead']);
assert.ok(spine.find(e=>e.className==='system-path-spine-helpers').textContent.includes('addReply, addReplyBulk'),'the helpers are named, never counted');
// The parts the path enters fold under one line that names them all.
const deeper=steps.children.find(c=>c.className==='system-path-deeper');
assert.equal(deeper.tagName,'DETAILS');assert.ok(!deeper.open,'the parts start folded');
assert.ok(deeper.children[0].textContent.includes('String commands, Command dispatch, Keyspace, Client connections'),'the fold names its parts: '+deeper.children[0].textContent);
assert.deepEqual(titles(deeper),['String commands','Command dispatch','Keyspace','Client connections']);
assert.ok(said(deeper)[0].includes('getCommand'),'the handler is named in its own part');
assert.ok(said(deeper).includes('getCommand → getGenericCommand'));
assert.equal(said(deeper).length,9,'every call entering a part is kept');
assert.equal(deeper.all(e=>e.className==='system-path-more').length,1,'past five, the rest of a part\'s calls fold');
assert.equal(deeper.all(e=>e.className==='possible').length,2,'a possible call and a read say so');
assert.ok(deeper.textContent.includes('3 more'),'the other calls into a part are counted');
assert.ok(!section.textContent.includes('redis.c'),'no line number is shown');
const name=text=>steps.find(e=>e.tagName!=='DIV'&&e.textContent===text);
const click=(element,extra={})=>{let prevented=false;element.listeners.click({button:0,preventDefault(){prevented=true;},stopPropagation(){},...extra});return prevented;};
assert.equal(click(name('addReply')),true,'a plain click reads the declaration instead of opening its code');
assert.deepEqual(read,[['n-clients','h/addReply']]);
assert.equal(click(name('addReply'),{metaKey:true}),false,'a modifier-click opens its code');
assert.equal(click(name('addReply'),{button:1}),false);
assert.deepEqual(read,[['n-clients','h/addReply']]);
assert.equal(name('lookupKeyRead').tagName,'SPAN','a declaration in a part the map does not draw is only named');
steps.find(e=>e.tagName==='BUTTON'&&e.textContent==='String commands').listeners.click();
assert.deepEqual(chosen,['t1-accept','n-strings'],'a part on the path leads to its reading');
assert.equal(steps.find(e=>e.className==='system-path-part'&&e.textContent==='Keyspace').tagName,'DIV','a part the map does not draw is only named');
// exec is dispatched from call and its handler calls call again: the two
// lines say different things and do not read as a contradiction.
const exec=rmInputPathSection({dispatched:[{site:0,of:94,handlers:94,inputs:95}],reaches:[{site:0,inputs:95,calls:[[1,0,0]]}],decls:[decl('call','n-clients'),decl('execCommand','n-strings')]},
  'exec',id=>parts[id]||null,id=>inputs[id]||null,()=>{},()=>{});
assert.ok(exec.all(e=>e.className==='system-shared-path')[0].textContent.includes('not established'));
const own=exec.find(e=>e.className==='system-path-reaches');
assert.ok(own&&own.all(e=>e.className==='system-path-step').map(c=>c.textContent).includes('execCommand → call'),'what its handler itself calls there reads apart');
`)
}

// freqtrade's trade had read its handler's first calls, then "Reaches 65
// more parts deeper", the bot loop behind the count (critic, 2026-09-30).
// An input's path reads its handler's flow as its spine: the steps whose
// work is one call into the next, a class with the methods of it the step
// before calls, then the branches its work splits into, helpers named on
// one line; the parts it enters fold under one line naming every one.
func TestAnInputsPathReadsItsSpineAndNamesItsParts(t *testing.T) {
	code := nameBreaksJS(t) + systemJSPiece(t, "29-operation-view.js", "function rmInputPathSection(", "(function(){document.querySelectorAll('[data-map-explorer]')") +
		systemJSPiece(t, "30-map.js", "function rmSiteHandlers(", "// A dispatch site read with its declaration")
	runSystemJS(t, fakeElements+`
const repomapMembers={sourceLink:s=>{const a=rmEl('a','',s.Text);a.href=s.Href;return a;}};
const decls=['start_trading','Worker','run','exit','__init__','FreqtradeBot','process','startup','Configuration','State'].map(name=>({name}));
`+code+`
const section=rmInputPathSection({decls,parts:[{part:'p0',title:'CLI',depth:0,handler:0},{part:'p1',title:'Trading bot core',depth:1,entered:[[0,1,0]]},{part:'p2',title:'Configuration',depth:3,entered:[[1,8,0]]}],
  spine:{steps:[{decl:0,part:'p0'},{decl:1,part:'p1',members:[4,2,3]}],branches:[{decl:5,part:'p1',members:[6,7]},{decl:8,part:'p2'},{decl:9,part:'p3',helper:true}]}},'trade',()=>null,()=>null,()=>{},()=>{});
const spine=section.find(e=>e.className==='system-path-spine');
assert.deepEqual(spine.all(e=>e.className==='system-path-spine-step').map(c=>c.textContent),['start_trading','Worker (__init__, run, exit)'],'a constructor call and its class are one step');
assert.deepEqual(spine.all(e=>e.className==='system-path-spine-branch').map(c=>c.textContent),['FreqtradeBot (process, startup)','Configuration'],'the bot loop is one expand away');
assert.equal(spine.find(e=>e.className==='system-path-spine-helpers').textContent.trim(),'helpers: State');
const fold=section.children.at(-1).children.find(c=>c.className==='system-path-deeper');
assert.ok(fold.children[0].textContent.includes('CLI, Trading bot core, Configuration'),'the fold names its parts, never counts them');
assert.ok(!/\d+ more parts/.test(section.textContent),'no part is hidden behind a count');
// The line names the program's parts only, each once; the calls outside the
// program stand under "Outside" in the fold, one head per destination with
// its calls (critic, 2026-09-30: litestream's replicate had named "init"
// four times and raw calls among its parts).
const outside=rmInputPathSection({decls:decls.concat([{name:'sql.Open'},{name:'sql.DB.BeginTx'},{name:'exec.CommandContext'}]),
  parts:[{part:'p0',title:'CLI',depth:0,handler:0},{part:'p1',title:'Trading bot core',depth:1,entered:[[0,1,0]]},{part:'p1',title:'Trading bot core',depth:2},
    {part:'t1',title:'Database',depth:3,entered:[[5,10,0]],outside:true},{part:'t2',title:'Database',depth:4,entered:[[5,11,0]],outside:true},{part:'t3',title:'exec.CommandContext',depth:2,entered:[[1,12,0]],outside:true}],
  spine:{steps:[{decl:0,part:'p0'},{decl:1,part:'p1'}]}},'trade',()=>null,()=>null,()=>{},()=>{});
const folded=outside.children.at(-1).children.find(c=>c.className==='system-path-deeper');
assert.equal(folded.children[0].textContent,'Parts on this path: CLI, Trading bot core','the parts only, each once: '+folded.children[0].textContent);
const heads=folded.all(e=>e.className==='system-path-part').map(e=>e.textContent);
assert.deepEqual(heads,['CLI','Trading bot core','Trading bot core','Database','exec.CommandContext'],'each outside destination once, after the parts');
assert.ok(folded.children.some(c=>c.className==='map-reading-label'&&c.textContent==='Outside'),'the destinations stand under Outside');
`)
}

// A dispatch site is read with its declaration: named, how many it chooses
// between and how many inputs are dispatched there on its hover (no digits
// in the column), and the inputs whose own code reaches it, each with its
// calls to it and a button to its reading, with the line that none of them
// is established as leading to an input dispatched there; or that no input
// reaches it by calls.
func TestADispatchSitesReadingListsTheInputsReachingIt(t *testing.T) {
	code := systemJSPiece(t, "30-map.js", "// The inputs dispatched at a site", "// Who calls a declaration and what it calls")
	runSystemJS(t, fakeElements+`
const readings={sites:[{site:0,of:94,handlers:94,inputs:95,shared:[{handler:5,inputs:['t1-sinter','t1-smembers']}],reached_from:[{input:'t1-exec',calls:[[2,0,0]]},{input:'t1-lpush',calls:[[3,4,0],[4,0,1]]}]},{site:1,of:94,handlers:94,inputs:95}],
  decls:[{name:'call',href:'h/call'},{name:'loadAppendOnlyFile',href:'h/load'},{name:'execCommand'},{name:'lpushCommand'},{name:'handleClientsWaitingListPush'},{name:'sinterCommand'}]};
const node={dataset:{dispatch:JSON.stringify(readings)}},chosen=[];
const map={chooseOperation:id=>chosen.push(id)};
function rmDeclName(decl,text,go,title){const a=rmEl('a','map-reading-name',text);a.href=decl.code||decl.href;a.title=title;if(go)a.listeners.click=go;return a;}
function rmFlowPart(){return null;}
document.getElementById=id=>({'t1-exec':{dataset:{title:'exec'}},'t1-lpush':{dataset:{title:'lpush'}},'t1-sinter':{dataset:{title:'sinter'}},'t1-smembers':{dataset:{title:'smembers'}}})[id]||null;
`+code+`
const box=rmSiteReading(map,node,'h/call');
// Every name with code is its link, as every name in the column is.
assert.equal(box.find(e=>e.tagName==='A'&&e.textContent==='call').href,'h/call');
// The site's counts say what they count on its hover, and why its 95
// inputs outnumber its 94 handlers.
assert.ok(!/\d/.test(box.children[0].textContent),'the heading counts nothing: '+box.children[0].textContent);
assert.ok(/94.*95/.test(box.children[0].title),'how many it chooses between and how many inputs are dispatched there, on its hover');
assert.ok(box.children[0].title.includes('sinterCommand')&&box.children[0].title.includes('sinter, smembers'),'a handler several inputs share, with those inputs');
const reached=box.all(e=>e.className==='map-concept-reached');
assert.deepEqual(reached.map(r=>[r.children[0].textContent,r.all(e=>e.tagName==='LI').length]),[['exec',1],['lpush',2]],'each input reaching it, with its calls to it');
assert.equal(reached[1].all(e=>e.className==='possible').length,1,'a possible call says so');
assert.ok(box.children.at(-1).textContent.includes('not established'),'none of them is said to lead to an input dispatched there');
box.find(e=>e.tagName==='BUTTON'&&e.textContent==='lpush').listeners.click();
assert.deepEqual(chosen,['t1-lpush']);
const load=rmSiteReading(map,node,'h/load');
assert.equal(load.all(e=>e.className==='map-concept-reached').length,0);
assert.ok(load.children.at(-1).textContent.includes('loadAppendOnlyFile'),'a site no input reaches says so');
assert.equal(rmSiteReading(map,node,'h/other').childElementCount,0,'another declaration has no site reading');
// The inputs dispatched at a site are part of the column's own scroll,
// folded under their count when long (owner, 2026-09-29), each input's name
// reading the input and each handler's name its declaration's link.
const handlers=Array.from({length:13},(_,i)=>({name:'h'+i+'Command',href:'h/h'+i}));
const many={sites:[{site:0,of:94,handlers:94,inputs:13,dispatched:handlers.map((_,i)=>({input:'t1-exec',handler:i+1}))}],decls:[{name:'call',href:'h/call'}].concat(handlers)};
const list=rmSiteReading(map,{dataset:{dispatch:JSON.stringify(many)}},'h/call').find(e=>e.className==='map-dispatched');
assert.equal(list.tagName,'DETAILS');assert.equal(list.open,false,'thirteen inputs fold under their count');
const rows=list.find(e=>e.className==='map-dispatched-list').children;
assert.equal(rows.length,13);
assert.equal(rows[0].children[0].tagName,'BUTTON');assert.equal(rows[0].children[2].tagName,'A');assert.equal(rows[0].children[2].href,'h/h0');
`)
}

// The component's "Entrypoints" link lands on the program's entry: the part
// whose reading the link names, with the seed read there, or the component
// itself, read at its entry line, when no part holds the seed; with neither
// on this map it is left to the page it names.
func TestTheEntrypointsLinkLandsOnThePartOrTheComponentsEntryLine(t *testing.T) {
	code := systemJSPiece(t, "29-operation-view.js", "function rmEntryLanding(", "(function(){document.querySelectorAll('[data-map-explorer]')")
	runSystemJS(t, code+`
const node=(id,href)=>({id,getAttribute:k=>k==='href'?href:null});
const nodes=[node('n-t1-g1','#t1-g1'),node('n-t1-g2','#t1-g2')],component=node('system-component-t1','#t1');
const link=data=>({dataset:data});
let landing=rmEntryLanding(link({entryPart:'t1-g2',entrySource:'h/kvd.c#L302'}),nodes,()=>component);
assert.deepEqual([landing.node.id,landing.source,landing.entry],['n-t1-g2',{key:'h/kvd.c#L302'},false],'the seed is read in its part');
landing=rmEntryLanding(link({entryPart:'t1-g1'}),nodes,()=>component);
assert.deepEqual([landing.node.id,landing.source,landing.entry],['n-t1-g1',null,false],'seeds in one part land on the part');
landing=rmEntryLanding(link({}),nodes,()=>component);
assert.deepEqual([landing.node.id,landing.source,landing.entry],['system-component-t1',null,true],'a seed off the map reads the component at its entry line');
assert.equal(rmEntryLanding(link({}),nodes,()=>null),null,'with no component on this map the link keeps its page');
`)
}

// The toolbar's breadcrumb was one link, "redis-server (executable) /
// Server runtime / Replication · syncCommand", whose click only re-read the
// current reading: Redis's readers clicked "Server runtime" in it three
// times and stayed on syncCommand's tiles. Each segment is its own link, the
// label still reads as one, and each goes up to its level: a frame is read
// and framed, the part is read and entered without its declaration, the
// declaration is read in its part, the input is entered as its path.
func TestEachBreadcrumbSegmentGoesUpToItsLevel(t *testing.T) {
	path := systemJSPiece(t, "29-operation-view.js", "function rmExplorationPath(", "(function(){document.querySelectorAll('[data-map-explorer]')")
	levels := systemJSPiece(t, "29-operation-view.js", "  map.explorationPath=function(){", "  map.resumeExploration=function(){")
	crumbs := systemJSPiece(t, "45-modes.js", "function rmCrumbs(", "// One entrance, one report")
	runSystemJS(t, fakeElements+`
const node=(id,title,extra={})=>({id,dataset:{title,...extra}});
const byID={get:node('t1-o7','get',{activation:'request'}),server:node('system-component-t1','redis-server (executable)'),
  runtime:node('t1-area-k2','Server runtime'),replication:node('n-t1-g18','Replication')};
for(const key of Object.keys(byID))byID[byID[key].id]=byID[key];
const parents={'n-t1-g18':'t1-area-k2','t1-area-k2':'system-component-t1'};
function path(id){const out=[];while(id){out.unshift(id);id=parents[id];}return out;}
let scope='n-t1-g18',operation=byID.get;const calls=[];
const map={explorerMember:{owner:'n-t1-g18',name:'syncCommand',key:'h#sync',href:'h#sync',open:''},revealNode(n,all,source){calls.push(['reveal',n.id,all,source]);return Promise.resolve(true);}};
const surface={clearMember(){calls.push(['clearMember']);}};
function select(n,navigate,source,focus){calls.push(['select',n.id,navigate,source,focus]);return Promise.resolve(true);}
`+path+levels+crumbs+`
const segments=map.explorationPath();
const container=rmEl('span','reading-map-context');
rmCrumbs(container,segments,{href:'#overview',title:'System map'});
const links=container.children.filter(c=>c.tagName==='A');
assert.deepEqual(links.map(a=>[a.textContent,a.href]),[['get','#t1-o7'],['redis-server (executable)','#system-component-t1'],['Server runtime','#t1-area-k2'],['Replication','#n-t1-g18'],['syncCommand','#n-t1-g18']],'one link per segment');
assert.equal(container.textContent,map.explorationLabel(),'the separators stand between the links');
assert.deepEqual(links.map(a=>a['aria-current']||''),['','','','','location']);
for(const link of links)map.goToLevel(segments[Number(link.dataset.crumb)]);
assert.deepEqual(calls,[
  ['clearMember'],['select','t1-o7',true,null,true],
  ['clearMember'],['select','system-component-t1',true,null,'center'],
  ['clearMember'],['select','t1-area-k2',true,null,'center'],
  ['clearMember'],['select','n-t1-g18',true,null,'center'],
  ['reveal','n-t1-g18',false,{key:'h#sync',href:'h#sync',open:''}]]);
scope='';operation=null;map.explorerMember=null;
rmCrumbs(container,map.explorationPath(),{href:'#overview',title:map.explorationLabel()});
assert.deepEqual(container.children.map(a=>[a.textContent,a.href]),[['System map','#overview']],'with nothing read it names the map');
`)
}
