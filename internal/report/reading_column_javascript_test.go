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
