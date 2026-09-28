package report

import "testing"

// A DOM small enough to render the reading column's views and read them
// back: classes, datasets, text, listeners and `.class` queries.
const readingViewElements = `
class El{
 constructor(tag){this.tagName=String(tag).toUpperCase();this.children=[];this.className='';this.own='';this.listeners={};this.dataset={};this.hidden=false;this.title='';this.parent=null;
  const self=this;this.classList={add(c){if(!self.has(c))self.className=(self.className+' '+c).trim();},contains(c){return self.has(c);}};}
 has(c){return this.className.split(' ').includes(c);}
 appendChild(c){if(c instanceof El)c.parent=this;this.children.push(c);return c;}
 append(...c){c.forEach(x=>this.appendChild(typeof x==='string'?text(x):x));}
 remove(){if(this.parent)this.parent.children=this.parent.children.filter(c=>c!==this);}
 get childElementCount(){return this.children.filter(c=>c instanceof El).length;}
 get firstChild(){return this.children[0];}
 addEventListener(k,f){this.listeners[k]=f;} setAttribute(k,v){this[k]=v;}
 set textContent(v){this.own=String(v);this.children=[];} get textContent(){return this.own+this.children.map(c=>c.textContent).join('');}
 set innerHTML(v){this.html=v;}
 all(test,out=[]){for(const c of this.children)if(c instanceof El){if(test(c))out.push(c);c.all(test,out);}return out;}
 querySelectorAll(selector){return this.all(c=>c.has(selector.slice(1)));}
}
const text=value=>({textContent:value});
const document={createElement:tag=>new El(tag),createTextNode:text};
function rmEl(tag,cls,value){const item=document.createElement(tag);if(cls)item.className=cls;if(value!==undefined)item.textContent=value;return item;}
function rmT(key,...values){return values.reduce((s,v,i)=>s.replace('{'+i+'}',v),key);}
const repomapMembers={sourceLink(s){const a=rmEl(s.Href||s.Open?'a':'span','',s.Text);a.href=s.Href;return a;},sourceKey(s){return s.Href||s.Open||'';}};
const nodes={'#own':{dataset:{title:'Server lifecycle and cron',summary:'Keeps the server running.',summaryRef:'s1',lane:'',symbols:'[{"name":"serverCron","kind":"function","text":"(id: long)","href":"h#serverCron"}]'},getAttribute:()=>'#own'},
 '#main':{dataset:{title:'main',lane:'triggers'},getAttribute:()=>'#main'},
 '#core':{dataset:{title:'Server core state',lane:'core'},getAttribute:()=>'#core'}};
const read=[];
const ctx={nodeByHref:href=>nodes[href]||null,nodeById:()=>null,goDecl:decl=>nodes[decl.part]?()=>read.push(decl.name):null,readNode:n=>read.push(n.dataset.title),light(){}};
const decl=(name,kind,part,extra)=>Object.assign({name,kind,part,key:'h#'+name,href:'h#'+name,file:'server.c',at:'server.c:1'},extra||{});
const data={decls:[decl('serverCron','function','#own',{doc:'Called every 100 ms.',code:'h#serverCron-L9'}),decl('initServer','function','#own',{bold:true}),decl('beforeSleep','function','#own'),
  decl('main','function','#main'),decl('server','variable','#core'),decl('dictResize','function','#core'),decl('shared','variable','#own')],
 files:['server.c'],
 members:[{kind:'function',decls:[2,1,0]},{kind:'variable',decls:[6]}],
 in:[{part:'#main',title:'main',count:2,lines:[{caller:3,ends:[{decl:1,kind:'calls'},{decl:2,kind:'passes_callback'}]}]}],
 out:[{part:'#core',title:'Server core state',count:2,lines:[{caller:-1,ends:[{decl:5,kind:'calls'},{decl:4,kind:'reads'}]}]}],
 own:[{decl:0,callers:[{part:'#own',title:'Server lifecycle and cron',own:true,decls:[{decl:1,kind:'passes_callback',sites:[{at:'server.c:30',href:'h#30'}]}]}],
   callees:[{part:'#core',title:'Server core state',decls:[{decl:5,kind:'calls',sites:[{at:'server.c:22',href:'h#22'}]}]}],uses:[{decl:4,kind:'reads',sites:[{at:'server.c:21',href:'h#21'}]}]}]};
`

// A part's reading stands as the owner chose (2026-09-28): who calls into
// it, each caller with what it calls here and a callback said so; the
// part's own box, the model's description marked by style, its files; its
// declarations under a heading per kind in the page data's order, a key in
// bold; the parts it calls, with the variables it uses there apart. A
// declaration's reading: its callers by part, its name as the code link
// with its file and its author's comment, what it calls and the variables
// it uses, each held by a part named on hover. A name reads its
// declaration.
func TestReadingColumnViewsFollowThePreparedData(t *testing.T) {
	code := systemJSPiece(t, "31-reading-column.js", "function rmGroupReading(", "// An Inputs collection's reading")
	runSystemJS(t, readingViewElements+code+`
const part=rmPartView(ctx,nodes['#own'],data).view;
const lines=part.children.map(c=>c.className+': '+c.textContent);
assert.deepEqual(lines,[
 'map-reading-side map-reading-in: Called frommain2main()initServer()beforeSleep()passed as a callback',
 'map-part-title: Server lifecycle and cron',
 'map-card-summary model: Keeps the server running.',
 'map-part-files meta: server.c',
 'map-reading-members: 3 functionsbeforeSleepinitServerserverCron',
 'map-reading-members: 1 variablesshared',
 'map-reading-side map-reading-out: Calls intoServer core state2dictResize()Uses variablesserver',
]);
assert.equal(part.all(c=>c.has('map-card-summary'))[0].title,'written by the model','model text says so on hover, with no chip');
assert.deepEqual(part.all(c=>c.has('map-reading-key')).map(c=>c.textContent),['initServer'],'only the key is bold, in the members list');
assert.ok(part.all(c=>c.has('map-part-box-entry')).length===1&&part.all(c=>c.has('map-part-box-core')).length===1,'a part box takes its lane');
part.all(c=>c.textContent==='dictResize()'&&c.has('map-reading-name'))[0].listeners.click({preventDefault(){},stopPropagation(){}});
part.all(c=>c.tagName==='BUTTON'&&c.textContent==='main')[0].listeners.click({stopPropagation(){}});
assert.deepEqual(read,['dictResize','main'],'a name reads its declaration and a part box its part');
const view=rmDeclView(ctx,nodes['#own'],data,{name:'serverCron',source:{Href:'h#serverCron',Text:'server.c:1'},explanation:'Runs the cron.',explanation_ref:'e1'});
assert.deepEqual(view.children.map(c=>c.className),['map-reading-side','map-decl-name','map-decl-where','map-author-comment','map-decl-explanation model','map-reading-side','map-reading-side']);
assert.equal(view.children[0].textContent,'Called byServer lifecycle and cron1initServer()passes it as a callback:30');
assert.equal(view.children[0].all(c=>c.tagName==='A'&&c.textContent===':30')[0].href,'h#30','the relation links to its own line');
assert.equal(view.children[1].textContent,'serverCron(id: long)');
assert.equal(view.children[1].children[0].href,'h#serverCron-L9','the name is the link to all of its code');
assert.equal(view.children[2].textContent,'server.c','the file alone, no line');
assert.equal(view.children[3].textContent,"The author's comment in the codeCalled every 100 ms.",'the author\'s comment stands in the reading, marked as theirs');
assert.equal(view.children[6].textContent,'Uses variablesserver:21');
assert.equal(view.children[6].all(c=>c.has('map-reading-name'))[0].title,'A global variable of Server core state\nserver.c:21');
`)
}
