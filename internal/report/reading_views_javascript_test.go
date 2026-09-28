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
 replaceChildren(...c){this.children=[];this.append(...c);}
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
 'map-part-title: Server lifecycle and cron',
 'map-card-summary model: Keeps the server running.',
 'map-part-files meta: server.c',
 'map-reading-members: 3 functionsbeforeSleepinitServerserverCron',
 'map-reading-members: 1 variablesshared',
 'map-reading-side map-reading-in: Called frommain2main()initServer()beforeSleep()passed as a callback',
 'map-reading-side map-reading-out: Calls intoServer core state2dictResize()Uses variablesserver',
],'what it is made of first, then who calls it (owner, 2026-09-28)');
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

// A component's reading names every area and part, each reading it, with
// its description on one line; the home names every program with its role,
// entry, inputs, connections and the files it is built from (owner,
// 2026-09-28).
func TestAComponentsOutlineAndTheHomesProgramsFollowThePageData(t *testing.T) {
	code := systemJSPiece(t, "31-reading-column.js", "function rmGroupReading(", "// An Inputs collection's reading") +
		systemJSPiece(t, "31-reading-column.js", "var rmLanguageNames=", "function rmCollectionView(") +
		systemJSPiece(t, "31-reading-column.js", "// The kind whose section a count", "function rmComponentReading(") +
		systemJSPiece(t, "31-reading-column.js", "// The home's table of programs", "// </reading-column>")
	runSystemJS(t, readingViewElements+code+`
const all={
 'system-component-t1':{id:'system-component-t1',dataset:{title:'redis-server',owner:'t1',role:'Backend database server',children:'area core',entries:'[{"name":"main","callable":true}]',sources:'["adlist.c","redis.c"]'}},
 area:{id:'area',dataset:{title:'Core infrastructure',summary:'Provides runtime services.',children:'core'},getAttribute:()=>'#area'},
 core:{id:'core',dataset:{title:'Server core state',summary:'Manages core state.',lane:'core'},getAttribute:()=>'#core'},
 'system-inputs-t1':{id:'system-inputs-t1',dataset:{collection:'{"groups":[],"kinds":[{"kind":"request","inputs":["a","b"]},{"kind":"setting","inputs":["c"]}]}'}}};
const opened=[];
const context={nodeByHref:href=>all[href.slice(1)]||null,nodeById:id=>all[id]||null,readNode:n=>opened.push(n.dataset.title)};
const outline=rmOutline(context,all['system-component-t1']);
assert.equal(outline.textContent,'Areas and partsCore infrastructureProvides runtime services.Server core stateManages core state.Server core stateManages core state.');
outline.all(c=>c.tagName==='BUTTON'&&c.textContent==='Core infrastructure')[0].listeners.click({stopPropagation(){}});
assert.deepEqual(opened,['Core infrastructure'],'a name reads its area');
const holder=rmEl('div');
rmProgramsTable(context,holder,[all['system-component-t1']],()=>[{incoming:true,title:'redis-cli'},{incoming:false,title:'TCP endpoint'}]);
assert.equal(holder.textContent,'Programsredis-serverBackend database serverEntrymain()Inputs2 requests · 1 settingsConnections← redis-cli · → TCP endpointBuilt fromadlist.c redis.c');
`)
}

// A part of several files lists its declarations file by file; a caller
// reaching many of its declarations through one dispatch site is one line,
// folded (owner, 2026-09-28: Sorted set commands' "Called from" opened with
// 17 loadAppendOnlyFile() → z*Command() possible rows).
func TestAPartReadsFileByFileAndAFanOutAsOneLine(t *testing.T) {
	code := systemJSPiece(t, "31-reading-column.js", "function rmGroupReading(", "// An Inputs collection's reading")
	runSystemJS(t, readingViewElements+code+`
const d=(name,file)=>({name,kind:'function',part:'#own',key:'h#'+name,href:'h#'+name,file,at:file+':1'});
const decls=[d('listCreate','adlist.c'),d('dictCreate','dict.c'),d('sdsnew','sds.c'),d('loadAppendOnlyFile','redis.c'),d('cmdTable','redis.c')]
 .concat(['zadd','zrem','zrank','zcard','zscore'].map(n=>d(n+'Command','redis.c')));
const fanned={decls,files:['adlist.c','dict.c','sds.c'],members:[{kind:'function',decls:[0,1,2]}],
 in:[{part:'#main',title:'main',count:5,lines:[{caller:3,ends:[5,6,7,8,9].map(i=>({decl:i,kind:'calls',possible:true})),fan:{of:94,noun:'request',via:[4]}}]}]};
const view=rmPartView(ctx,nodes['#own'],fanned).view;
assert.deepEqual(view.children.filter(c=>c.has('map-reading-file')).map(c=>c.textContent),
 ['adlist.c1 functionslistCreate','dict.c1 functionsdictCreate','sds.c1 functionssdsnew'],'file by file');
const line=view.all(c=>c.has('map-reading-caller'))[0];
assert.equal(line.tagName,'DETAILS','the fan-out is one folded line');
assert.equal(line.children[0].textContent,'loadAppendOnlyFile() → 5 request handlers, possible, via cmdTable');
`)
}

// A declaration other programs hold too lists their calls into it after its
// own program's, each group named by its program, and says when its own
// program never runs it (owner, 2026-09-28: anetTcpConnect).
func TestASharedDeclarationNamesTheProgramsCallingIt(t *testing.T) {
	code := systemJSPiece(t, "31-reading-column.js", "function rmGroupReading(", "// An Inputs collection's reading")
	runSystemJS(t, readingViewElements+code+`
data.decls.push(decl('cliConnect','function','#cli'));
data.own[0].callers.push({part:'#cli',title:'Command line client',program:'redis-cli',decls:[{decl:7,kind:'calls'}]});
data.own[0].not_called_in='redis-benchmark';
nodes['#cli']={dataset:{title:'Command line client',lane:'entry'},getAttribute:()=>'#cli'};
const view=rmDeclView(ctx,nodes['#own'],data,{name:'serverCron',source:{Href:'h#serverCron',Text:'server.c:1'}});
assert.equal(view.children[0].textContent,'Called byServer lifecycle and cron1initServer()passes it as a callback:30redis-cli:Command line client1cliConnect()');
assert.equal(view.children[1].className,'map-reading-not-called meta');
assert.equal(view.children[1].textContent,'Not called in redis-benchmark');
`)
}
