package report

import "testing"

// A tiny element for the reading's script: enough of the DOM to build a
// list and read it back as text, without a browser.
const fakeElements = `
class El{constructor(tag){this.tagName=String(tag).toUpperCase();this.children=[];this.className='';this.own='';this.listeners={};this.dataset={};this.hidden=false;}
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

// A chosen declaration had no reading of its own: the panel said "No
// explanation saved", and who calls it and what it calls sat in the part's
// long relation lists. Its reading lists both, from those rows, grouped by
// the part at the other end, one line per declaration with every place the
// call is written; choosing a line reads that declaration.
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
const sides=box.children.map(side=>({title:side.children[0].textContent,parts:side.children.filter(c=>c.className==='map-concept-part').map(p=>p.textContent),
  lines:side.children.filter(c=>c.tagName==='UL').map(ul=>ul.children.map(li=>li.textContent))}));
assert.deepEqual(sides,[
 {title:'Called by',parts:['Client connections','Event loop'],lines:[['readQueryFromClient · redis.c:2414'],['aeMain passes processInputBuffer as a callback · ae.c:335possible']]},
 {title:'Calls',parts:['Client connections'],lines:[['processCommand · redis.c:2353 · redis.c:2380']]}]);
assert.ok(!JSON.stringify(sides).includes('zfree'),'a row with neither end at the declaration is not its relation');
box.find(e=>e.textContent==='processCommand'&&e.tagName==='BUTTON').listeners.click();
assert.deepEqual(map.explained,{key:'h#cmd'},'a callee in the same part is read in place');
box.find(e=>e.className==='map-concept-decl'&&e.textContent.startsWith('aeMain')).listeners.click();
assert.deepEqual(map.revealed,['n-events',false,{key:'h#loop'}],'a caller in another part is read in its part');
const none=rmDeclarationRelations(map,node,'h#nothing',[node]);
assert.equal(none.textContent,"No call to or from it is listed among this part's connections.");
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
`+listen+`
const evidence=block(500,900);
content.click({target:summaryOf(evidence)});evidence.open=true;run();
assert.equal(content.scrollTop,308,'the opened block is brought whole into the column');
const tall=block(500,2000);content.scrollTop=0;
content.click({target:summaryOf(tall)});tall.open=true;run();
assert.equal(content.scrollTop,392,'a block taller than the column stops at its summary');
content.scrollTop=0;content.click({target:summaryOf(tall)});tall.open=false;run();
assert.equal(content.scrollTop,0,'closing scrolls nothing');
const shown=block(200,400);content.click({target:summaryOf(shown)});shown.open=true;run();
assert.equal(content.scrollTop,0,'a block already in view stays');
// Open all: the list comes into view once its folds are open, not when closed.
const folds=[{open:false},{open:false}],list=block(500,1400,true);list.querySelectorAll=s=>s===':scope>.conn-fold'?folds:[];
const button={closest:s=>s==='[data-open-all]'?button:s==='details'||s==='.connection-evidence'?list:null};
content.click({target:button});folds.forEach(f=>f.open=true);run();
assert.equal(content.scrollTop,392,'Open all brings the list to the column top');
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
	projection := systemJSPiece(t, "29-operation-view.js", "function rmSystemProjection(", "// A part's reading goes description")
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
assert.deepEqual(heading.children.map(c=>c.textContent),['Outside this input path'],'the reading says it at once');
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
const byID={strings},projection={inputOwner:{get:'strings',orphan:'strings'}},read=[];
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
assert.equal(stroke.className,'flow-key-stroke flow-key-possible');
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
// callers and its own connections. They are folded under their count, by
// kind, and each still opens its input.
func TestInputsReachingAPartAreFoldedUnderTheirCount(t *testing.T) {
	code := systemJSPiece(t, "29-operation-view.js", "function rmReachingInputs(", "(function(){document.querySelectorAll('[data-map-explorer]')")
	runSystemJS(t, fakeElements+code+`
const get={dataset:{activation:'request',title:'get'}},cron={dataset:{activation:'continuous',title:'serverCron'}};
let chosen=null;
const inputs=rmReachingInputs({dataset:{}},[get,cron],()=>'redis-server',input=>{chosen=input;});
assert.equal(inputs.tagName,'DETAILS');assert.ok(!inputs.open,'the list is folded');
assert.equal(inputs.children[0].tagName,'SUMMARY');assert.equal(inputs.children[0].textContent,'Inputs reaching this part · 2');
assert.deepEqual(inputs.all(e=>e.tagName==='H6').map(e=>e.textContent),['Incoming requests','Background work']);
inputs.find(e=>e.tagName==='BUTTON'&&e.textContent==='redis-server / serverCron').listeners.click();
assert.equal(chosen,cron);
const none=rmReachingInputs({dataset:{itemKind:'External communication'}},[],()=>'',()=>{});
assert.equal(none.children[0].textContent,'Inputs reaching this communication · 0');
`)
}

// A key type the model explained is listed from its concept before the
// part's code rows, and the concept carries neither the key mark nor the
// type's fields: redisClient read as a plain name with no fields.
func TestCodeInThisPartKeepsAKeyTypesMarkAndFields(t *testing.T) {
	code := systemJSPiece(t, "26-map-members.js", "var repomapMembers = (function () {", "\n})();") + "\n})();"
	runSystemJS(t, `
const chip=(name,href)=>({textContent:name,dataset:{},getAttribute:k=>k==='href'?href:null,cloneNode(){return {textContent:name,querySelectorAll:()=>[]};},closest:()=>null,querySelector:()=>null});
const field=(name,href)=>({dataset:{},querySelector:s=>s==='.chip'?chip(name,href):null});
const row=(name,href,key,fields)=>({dataset:{alias:'',key:String(key)},querySelector:s=>s===':scope>strong>.chip'&&key||s===':scope>.chip'&&!key?chip(name,href):s.startsWith(':scope>.anchor')?{textContent:'redis.c:1'}:null,
  querySelectorAll:s=>s===':scope>.symbol-fields>li'?fields.map(f=>field(f,href+'#'+f)):[]});
const rows=[row('redisClient','h#303',true,['fd','db']),row('createClient','h#2440',false,[])];
const group={querySelectorAll:s=>s==='.group-highlights .key-symbol'?rows:[]};
const document={getElementById:id=>id==='clients'?group:null};
const node={dataset:{concepts:JSON.stringify([{name:'redisClient',explanation:'One connected client.',source:{Href:'h#303',Text:'redis.c:303'}}])},getAttribute:()=>'#clients'};
`+code+`
const items=repomapMembers.items(node);
assert.deepEqual(items.map(i=>[i.name,!!i.key,(i.fields||[]).map(f=>f.name)]),[['redisClient',true,['fd','db']],['createClient',false,[]]]);
assert.equal(items[0].explanation,'One connected client.','the model line stays');
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

// An area's reading starts with its parts, each with what it is made of and
// its declarations, keys first; a part leads to its reading and a
// declaration to its own, in its part, without moving the camera.
func TestAnAreasCompositionListsItsPartsAndTheirCode(t *testing.T) {
	code := systemJSPiece(t, "29-operation-view.js", "  function areaComposition(", "  map.addEventListener('repomap:inspect'")
	runSystemJS(t, fakeElements+`
const part=(id,title,branch)=>({id,dataset:{title,branch:branch||''}});
const byID={a:part('a','Compression'),b:part('b','Data structures'),c:part('c','Nested','area')};
const members={a:[{name:'lzf_compress',key:true,source:{Href:'h/c'}},{name:'u8',source:{Href:'h/u8'}}],b:[{name:'listNext',source:{Href:'h/n'}}]};
const repomapMembers={sorted:p=>members[p.id]||[],composition:p=>({total:(members[p.id]||[]).length,counts:(members[p.id]||[]).length+' functions',files:p.id==='a'?['lzf.c']:[]}),
  sourceLink:s=>{const a=rmEl('a');a.href=s.Href;return a;},displayName:i=>i.name,sourceKey:s=>s.Href};
const chosen=[];function select(n,navigate,source,focus){chosen.push([n.id,source?source.key:undefined,focus]);}
`+code+`
const section=areaComposition({dataset:{children:'a b c'}});
assert.equal(section.children[0].textContent,'Made of 2 parts');
const parts=section.all(e=>e.className==='map-area-part');
assert.deepEqual(parts.map(p=>p.children.map(c=>c.textContent)),[['Compression','2 functions · lzf.c','lzf_compressu8'],['Data structures','1 functions','listNext']]);
assert.equal(parts[0].children[2].children[0].className,'map-member-key','a key is marked');
parts[0].children[0].listeners.click();
parts[0].children[2].children[1].listeners.click({preventDefault(){},stopPropagation(){}});
assert.deepEqual(chosen,[['a',undefined,false],['a','h/u8',false]]);
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

// A chosen input's reading lists its path (owner's 3c): the chain it
// shares with the other inputs its dispatch chooses between, folded into
// one box, then its own steps, a callee under its caller, with no line
// number and the part heading it where the part changes. A name of its own
// steps reads that declaration in the report (getCommand and addReply had
// opened GitHub); a modifier-click still opens its code, which is one
// explicit link on the line. The shared box is as it was.
func TestAnInputsPathIsTheSharedChainThenItsOwnSteps(t *testing.T) {
	code := systemJSPiece(t, "29-operation-view.js", "function rmInputPathSection(", "(function(){document.querySelectorAll('[data-map-explorer]')")
	runSystemJS(t, fakeElements+`
const plain=document.createElement;document.createElement=tag=>{const element=plain(tag);element.style={};return element;};
const repomapMembers={sourceLink:s=>{const a=rmEl('a','',s.Text);a.href=s.Href;return a;}};
const step=(name,part,title,depth,extra)=>({name,href:'h/'+name,source:'redis.c:1',part,part_title:title,depth,...extra});
const path={shared:[{inputs:95,all:false,through:'call',of:94,steps:[step('main','n-config','Server configuration',0),step('aeMain','n-loop','Event loop',0),step('call','n-clients','Client connections',0)]},
  {inputs:95,through:'loadAppendOnlyFile',of:94,steps:[step('main','n-config','Server configuration',0),step('loadAppendOnlyFile','n-persist','Persistence',0)]}],
  own:[step('getCommand','n-strings','String commands',0),step('getGenericCommand','n-strings','String commands',1),step('lookupKeyRead','n-keys','Keyspace',2,{possible:true}),step('addReply','n-clients','Client connections',2)]};
const parts={'n-strings':{id:'n-strings'},'n-clients':{id:'n-clients'}},chosen=[],read=[];
`+code+`
const section=rmInputPathSection(path,id=>parts[id]||null,part=>chosen.push(part.id),(part,key)=>read.push([part.id,key]));
assert.equal(section.children[0].textContent,'Path');
const boxes=section.all(e=>e.className==='system-shared-path');
assert.deepEqual(boxes.map(b=>[b.tagName,!!b.open,b.children[0].textContent]),[['DETAILS',true,'Shared by {0} inputs, through {1}'.replace('{0}',95).replace('{1}','call')],['DETAILS',false,'Shared by 95 inputs, through loadAppendOnlyFile']]);
const lines=list=>list.children.map(c=>c.className==='system-path-part'?'['+c.textContent+']':c.textContent);
assert.deepEqual(lines(boxes[0].find(e=>e.className==='system-path-steps')),['[Server configuration]','main','[Event loop]','aeMain','[Client connections]','call']);
assert.equal(boxes[0].children.at(-1).textContent,'call → one of 94');
const own=section.children.at(-1);
assert.deepEqual(lines(own),['[String commands]','getCommand · Open code ↗','getGenericCommand · Open code ↗','[Keyspace]','lookupKeyRead · possible · Open code ↗','[Client connections]','addReply · Open code ↗']);
const name=text=>own.find(e=>e.tagName!=='DIV'&&e.textContent===text);
const click=(element,extra={})=>{let prevented=false;element.listeners.click({button:0,preventDefault(){prevented=true;},stopPropagation(){},...extra});return prevented;};
assert.equal(click(name('addReply')),true,'a plain click reads the declaration instead of opening its code');
assert.deepEqual(read,[['n-clients','h/addReply']]);
assert.equal(click(name('getCommand'),{metaKey:true}),false,'a modifier-click opens its code');
assert.equal(click(name('getCommand'),{button:1}),false);
assert.deepEqual(read,[['n-clients','h/addReply']]);
assert.equal(name('lookupKeyRead').tagName,'SPAN','a step in a part the map does not draw is only named');
assert.deepEqual(own.all(e=>e.className==='system-path-code').map(e=>[e.textContent,e.href]),[['Open code ↗','h/getCommand'],['Open code ↗','h/getGenericCommand'],['Open code ↗','h/lookupKeyRead'],['Open code ↗','h/addReply']]);
assert.ok(!boxes[0].find(e=>e.className==='system-path-code')&&!boxes[0].find(e=>e.listeners&&e.listeners.click&&e.tagName==='A'),'the shared box is unchanged');
assert.deepEqual(own.all(e=>e.className==='system-path-step').map(e=>e.style.marginLeft),['0px','10px','20px','20px'],'a callee stands under its caller');
assert.ok(!JSON.stringify(lines(own)).includes('redis.c'),'no line number is shown');
own.find(e=>e.tagName==='BUTTON'&&e.textContent==='String commands').listeners.click();
assert.deepEqual(chosen,['n-strings'],'a part on the path leads to its reading');
assert.equal(own.find(e=>e.textContent==='Keyspace').tagName,'DIV','a part the map does not draw is only named');
`)
}
