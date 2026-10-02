package report

import "testing"

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
const repomapMembers={sourceLink(s){const a=rmEl(s.Href||s.Open?'a':'span','',s.Text);a.href=s.Href;return a;},sourceKey(s){return s.Href||s.Open||'';}};
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
const step=(name,dataset)=>el('li','flow-step',[el('span','flow-what',[el('code','',[name])]),el('span','model flow-why',['It runs '+name+'.'])],{dataset});
// The component's page: its Main flow and own work (hidden sources the
// reading copies), and its reference with what the reading leaves out.
const details=el('section','',[
 el('header','component-intro'),
 el('section','component-flow',[el('p','model flow-title',['Startup and event loop']),el('ol','flow',[step('main',{stepPart:'#t1-g1',stepKey:'h#main'}),step('initServer',{stepPart:'#t1-g1',stepKey:'h#init'})])]),
 el('section','component-own-work',[el('p','flow-own-title',['Also runs on its own:']),el('ul','flow-own',[el('li','flow-own-step',[el('span','flow-what',[el('code','',['serverCron'])])],{dataset:{input:'t1-o3'}})])]),
 el('details','component-reference',[el('summary','',['Code, entrypoints and sources']),el('h3','',['Runs code it is given'])])]);
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
const step=(name,kids)=>el('li','flow-step',[el('span','flow-what',[el('code','',[name])])].concat(kids||[]),{dataset:{stepPart:'#t1-g1',stepKey:'h#'+name}});
const details=el('section','',[el('header','component-intro'),
 el('section','component-flow',[el('ol','flow',[el('li','flow-part-head',[],{dataset:{flowPart:'#t1-g1'}}),
  step('start_trading',[el('span','flow-via flow-handles',['handles the command ',el('code','flow-input',['trade'],{dataset:{input:'t1-o9'}})])]),
  step('FreqtradeBot.process',[el('span','model flow-type',[el('code','',['FreqtradeBot']),' — the main trading bot'])]),
  el('li','flow-part-head',[],{dataset:{flowPart:'#t1-g7'}}),step('IStrategy.adjust')]),
  el('p','meta flow-end',['The path stops here.'])])]);
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
