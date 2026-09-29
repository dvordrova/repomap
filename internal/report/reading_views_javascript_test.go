package report

import (
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

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
 own:[{decl:0,callers:[{part:'#own',title:'Server lifecycle and cron',own:true,decls:[{decl:1,kind:'passes_callback'}]}],
   callees:[{part:'#core',title:'Server core state',decls:[{decl:5,kind:'calls'}]}]}]};
`

// A part's reading stands as the owner chose (2026-09-28): who calls into
// it, each caller with what it calls here and a callback said so; the
// part's own box, the model's description marked by style, its files; its
// declarations under a heading per kind in the page data's order, a key in
// bold; the parts it calls, with the variables it uses there apart. A
// declaration's reading: its callers by part, its name as the code link
// with its file and its author's comment, and what it calls. A name reads
// its declaration.
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
assert.deepEqual(view.children.map(c=>c.className),['map-reading-side','map-decl-name','map-decl-where','map-author-comment','map-decl-explanation model','map-reading-side']);
assert.equal(view.children[0].textContent,'Called byServer lifecycle and cron1initServer()passes it as a callback');
assert.equal(view.children[1].textContent,'serverCron(id: long)');
assert.equal(view.children[1].children[0].href,'h#serverCron-L9','the name is the link to all of its code');
assert.equal(view.children[2].textContent,'server.c','the file alone, no line');
assert.equal(view.children[3].textContent,"The author's comment in the codeCalled every 100 ms.",'the author\'s comment stands in the reading, marked as theirs');
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
assert.equal(view.children[0].textContent,'Called byServer lifecycle and cron1initServer()passes it as a callbackredis-cli:Command line client1cliConnect()');
assert.equal(view.children[1].className,'map-reading-not-called meta');
assert.equal(view.children[1].textContent,'Not called in redis-benchmark');
`)
}

// Who changes a field and who reads it: a type's field reads "Written by"
// and "Read by" under it, each side its functions in their part's box; a
// global variable lists its fields as the code reaches them through it;
// a function says "Writes:" with each field reading the type declaring it.
// No line numbers.
func TestAFieldsReadingNamesItsWritersAndReaders(t *testing.T) {
	code := systemJSPiece(t, "31-reading-column.js", "function rmGroupReading(", "// An Inputs collection's reading")
	runSystemJS(t, readingViewElements+code+`
data.decls.push(decl('redisDb','type','#core'));
const db=data.decls.length-1;
data.decls.push(decl('redisClient','type','#own',{fields:[{name:'fd',type:'int',href:'h#fd',at:'server.c:91'},{name:'argc',type:'int',href:'h#argc',at:'server.c:92'},{name:'db',type:'redisDb *',href:'h#db',at:'server.c:93',type_decl:db}]}));
const client=data.decls.length-1;
data.own.push({decl:client,fields:[{name:'fd',written:[{part:'#own',title:'Server lifecycle and cron',decls:[0]}],read:[{part:'#own',title:'Server lifecycle and cron',decls:[1]},{part:'#main',title:'main',decls:[3]}]}]});
data.own.push({decl:6,fields:[{name:'shared.crlf',written:[{part:'#own',title:'Server lifecycle and cron',decls:[1]}]}]});
data.own[0].writes=[{path:'redisClient.fd',decl:client},{path:'server.hz'}];
const type=rmDeclView(ctx,nodes['#own'],data,{name:'redisClient',source:{Href:'h#redisClient',Text:'server.c:90'}});
const grid=type.all(c=>c.has('map-reading-field-grid'))[0];
assert.deepEqual(grid.children.map(c=>c.className+': '+c.textContent),[
 ': fd',': int','map-field-uses-row: Written byServer lifecycle and cronserverCron()Read byServer lifecycle and croninitServer()mainmain()',': argc',': int',': db',': redisDb *']);
// A field's repository type reads that type.
const typeReads=read.length;grid.children.at(-1).all(c=>c.has('map-reading-name'))[0].listeners.click({button:0,preventDefault(){},stopPropagation(){}});
assert.deepEqual(read.slice(typeReads),['redisDb'],'the field type links to its type');
assert.ok(!/:\d/.test(grid.textContent),'no line numbers');
const shared=rmDeclView(ctx,nodes['#own'],data,{name:'shared',source:{Href:'h#shared',Text:'server.c:5'}});
assert.equal(shared.all(c=>c.has('map-reading-fields'))[0].textContent,'1 fieldsshared.crlfWritten byServer lifecycle and croninitServer()');
const cron=rmDeclView(ctx,nodes['#own'],data,{name:'serverCron',source:{Href:'h#serverCron',Text:'server.c:1'}});
const writes=cron.all(c=>c.has('map-reading-writes'))[0];
assert.equal(writes.textContent,'Writes: redisClient.fd, server.hz');
const reads=read.length;writes.all(c=>c.has('map-reading-name'))[0].listeners.click({button:0,preventDefault(){},stopPropagation(){}});
assert.deepEqual(read.slice(reads),['redisClient'],'a written field reads the type declaring it');
`)
}

// A function's reading lists its calls as its flow, in the order Go wrote
// them, each run into one part under that part's box, with no caption
// repeating its name ("serverCron calls, in order:"). Helper calls stand as
// one muted line of names under their step, each name reading its
// declaration, unless every call of a step is one; the line and "Show
// helper calls" open them in place. A call opens in place to its callee's
// flow, a call its ancestors make says it is shown above, and a library's
// call is a plain row.
func TestAFunctionsReadingIsItsFlow(t *testing.T) {
	reading := systemJSPiece(t, "31-reading-column.js", "function rmGroupReading(", "// An Inputs collection's reading")
	flow := systemJSPiece(t, "32-flow.js", "var rmFlowHelpers", "// </flow>")
	runSystemJS(t, readingViewElements+reading+flow+`
El.prototype.querySelector=function(s){return this.querySelectorAll(s)[0]||null;};
El.prototype.closest=function(){return null;};
El.prototype.insertBefore=function(c,ref){c.parent=this;const at=this.children.indexOf(ref);if(at<0)this.children.push(c);else this.children.splice(at,0,c);return c;};
Object.defineProperty(El.prototype,'open',{get(){return !!this._open;},set(v){const was=!!this._open;this._open=!!v;if(was!==this._open&&this.listeners.toggle)this.listeners.toggle();}});
nodes['#persist']={id:'persist',dataset:{title:'Persistence',summary:'Saves the dataset.'},getAttribute:()=>'#persist'};
ctx.nodeById=()=>null;document.getElementById=()=>null;
const d=data.decls;
d.push(decl('redisLog','function','#core'),decl('rdbSave','function','#persist'),decl('lookupKeyRead','function',''));
const log=d.length-3,save=d.length-2,lookup=d.length-1;
data.own[0].flow=[{decl:log,helper:true,sites:[{at:'server.c:1273'},{at:'server.c:1288'}]},{decl:2,sites:[{at:'server.c:1284'}]},{name:'wait3',lib:'sys/wait.h',kind:'invokes_external',sites:[{at:'server.c:1304'}]},
  {decl:save,sites:[{at:'server.c:1322'}]},{decl:save,kind:'passes_callback',sites:[{at:'server.c:1330'}]},{decl:lookup,sites:[{at:'server.c:1350'}]},{decl:0,sites:[{at:'server.c:1360'}]},
  {macro:'assert',lib:'assert.h',sites:[{at:'server.c:1370'}]},{decl:1,macro:'redisAssert',sites:[{at:'server.c:1380'}]}];
data.own.push({decl:save,flow:[{decl:log,helper:true,sites:[{at:'server.c:3010'}]}]});
data.own.push({decl:lookup,flow:[{decl:1,sites:[{at:'server.c:905'}]}]});
const view=rmDeclView(ctx,nodes['#own'],data,{name:'serverCron',source:{Href:'h#serverCron',Text:'server.c:1'}});
const said=view.textContent;
assert.ok(!/serverCron calls|in order|steps|\d+ helpers|only helpers|not on the map|also from/.test(said),'no caption repeats its name and no meta word: '+said);
const root=view.all(c=>c.has('map-flow-root'))[0];
const rows=()=>root.all(c=>c.has('map-flow-row')&&!c.has('map-flow-helper')).map(c=>c.all(x=>x.has('map-reading-name')||x.has('map-flow-plain'))[0].textContent);
assert.deepEqual(rows(),['beforeSleep()','wait3()','rdbSave()','rdbSave()','lookupKeyRead()','serverCron()','assert','redisAssert'],'the calls in the order Go wrote them, helpers folded, a macro as written');
const macro=root.all(c=>c.has('map-reading-name')&&c.textContent==='redisAssert')[0];
assert.ok(macro.title.startsWith('redisAssert expands to a call of initServer'),'a macro names what its expansion calls on its hover: '+macro.title);
assert.equal(root.all(c=>c.has('map-flow-plain')&&c.textContent==='assert')[0].title,'assert.h\na macro\ncalled at server.c:1370');
assert.deepEqual(root.all(c=>c.has('map-part-box')).map(c=>c.textContent),['Server lifecycle and cron','Persistence','Server lifecycle and cron','Server lifecycle and cron'],'each run into one part stands under its box');
assert.ok(root.all(c=>c.has('map-flow-above')).length===1,'a call its ancestors make is shown above');
const helperLine=root.all(c=>c.has('map-flow-helpers'));
assert.equal(helperLine.length,1,'one line of helper names under the step');
assert.equal(helperLine[0].textContent,'+ helpers: redisLog()','the helper is named, not hidden');
const helperReads=read.length;helperLine[0].all(c=>c.has('map-reading-name'))[0].listeners.click({button:0,preventDefault(){},stopPropagation(){}});
assert.deepEqual(read.slice(helperReads),['redisLog'],'a helper name reads its declaration');
// The line opens the step's helpers into rows in place, and folds them again.
helperLine[0].children[0].listeners.click({stopPropagation(){}});
assert.deepEqual(root.all(c=>c.has('map-flow-row')&&c.has('map-flow-helper')).map(c=>c.textContent),['redisLog()'],'opened, the helper is a row in its place');
assert.equal(root.all(c=>c.has('map-flow-helpers'))[0].textContent,'− helpers');
root.all(c=>c.has('map-flow-helpers'))[0].children[0].listeners.click({stopPropagation(){}});
assert.equal(root.all(c=>c.has('map-flow-helpers'))[0].textContent,'+ helpers: redisLog()');
// Opening rdbSave shows its only call, a helper, as its call.
const saveRow=root.all(c=>c.tagName==='DETAILS'&&c.textContent.startsWith('rdbSave()'))[0];
saveRow.open=true;
assert.ok(saveRow.all(c=>c.has('map-flow-row')).some(r=>r.textContent==='redisLog()'&&!r.has('map-flow-helper')),'a step whose every call is a helper shows them');
// The declaration no part holds is a plain name that still opens.
const lookupRow=root.all(c=>c.tagName==='DETAILS'&&c.textContent.startsWith('lookupKeyRead()'))[0];
assert.ok(lookupRow&&lookupRow.all(c=>c.has('map-flow-plain')).length>0,'a declaration no part holds keeps its call, a plain name');
// The toggle shows the helper calls, the open ones staying open.
rmFlowHelpers=true;root.rmRender();
const shownHelpers=root.all(c=>c.has('map-flow-helper'));
assert.ok(shownHelpers.length===1&&shownHelpers[0].textContent==='redisLog()','the toggle shows the helper call, lighter, in its place');
assert.equal(root.all(c=>c.has('map-flow-helpers')).length,0,'with the toggle on, no step keeps its line');
assert.ok(root.all(c=>c.tagName==='DETAILS'&&c.textContent.startsWith('rdbSave()')&&c.open).length===1,'what was open stays open');
`)
}

// An input's reading opens at how a request reaches it: the first way as
// one chain, a line per part, the handler last in its part, the callable
// handed over saying how on its hover; the other ways folded on one line
// named by the part where each leaves the first, "+N" for the inputs that
// run the site themselves; then who sends it, last.
func TestAnInputsReadingOpensAtHowARequestReachesIt(t *testing.T) {
	reading := systemJSPiece(t, "31-reading-column.js", "function rmGroupReading(", "// An Inputs collection's reading")
	flow := systemJSPiece(t, "32-flow.js", "var rmFlowHelpers", "// </flow>")
	runSystemJS(t, readingViewElements+reading+flow+`
El.prototype.insertBefore=function(c,ref){c.parent=this;const at=this.children.indexOf(ref);if(at<0)this.children.push(c);else this.children.splice(at,0,c);return c;};
El.prototype.closest=function(){return null;};
nodes['#repl']={id:'repl',dataset:{title:'Replication'},getAttribute:()=>'#repl'};
const byId={'n-core':nodes['#core'],'n-main':nodes['#main'],'n-repl':nodes['#repl'],'n-own':nodes['#own']};
ctx.nodeById=id=>byId[id]||null;document.getElementById=()=>null;
const p=(name,part)=>({name,part,href:'h#'+name});
const path={decls:[p('acceptHandler','n-core'),p('readQueryFromClient','n-core'),p('call','n-core'),p('getCommand','n-own'),p('createClient','n-core'),p('serverCron','n-main'),p('syncWithMaster','n-repl')],
  ways:[{input:'accept',chain:[0,1,2],hop:1,by:[0,4]},{input:'cron',chain:[5,1,2],hop:1,by:[5,6,4],from:6}],also:['exec','lpush'],
  parts:[{depth:0,handler:3,part:'n-own'}],sent_by:[{program:'redis-cli',input:'cli-get',name:'get',in:'cmdTable'}]};
const inputs={accept:{dataset:{title:'acceptHandler'}},cron:{dataset:{title:'serverCron'}},exec:{dataset:{title:'exec'}},lpush:{dataset:{title:'lpush'}},'cli-get':{dataset:{title:'get'}}};
const section=rmInputFlowSection(ctx,path,'get',id=>inputs[id]||null,()=>{});
const lines=section.children.map(c=>c.textContent);
assert.equal(lines[0],'How a request reaches get:');
assert.equal(lines[1],'Server core stateacceptHandler() → readQueryFromClient() → call()Server lifecycle and crongetCommand()','the first way, a line per part, the handler last');
const hop=section.all(c=>c.textContent==='readQueryFromClient()'&&c.has('map-reading-name'))[0];
assert.equal(hop.title,'Server core state\npassed as a callback by createClient, which acceptHandler calls');
assert.ok(lines[2].startsWith('Other ways in: from Replication, +2'),'the other ways, folded, by where they leave the first: '+lines[2]);
assert.equal(section.children.at(-1).textContent,'redis-cli sends get.','who sends it, last');
`)
}

// A function's reading prints no line number (owner, 2026-09-29:
// "человек будет видеть код"): a caller calling from two places is its
// name once, and "Uses variables" ("argv :4248 :4250 …", fields reached
// through a parameter printed as variables) is one line, "Reads: …", each
// field by the path the code reaches it by and each global variable once,
// in the order first used. A field it also writes stays under "Writes:", a
// name leading to a longer path it reads is said by that path (shared and
// shared.czero by shared.czero.ptr), and a local is not listed.
func TestAFunctionsReadingSaysEachReadOnceWithNoLineNumbers(t *testing.T) {
	b, index, part, anchors := readingFixture(t)
	add := func(id, name string, kind programindex.ObjectKind, line int, owner string) {
		b.subjects[subjectKey("t1", id)] = subjectRef{subject: groupindex.Subject{ID: id, Object: &groupindex.ObjectFacts{Name: name, Kind: kind, OwnerID: owner,
			Location: &programindex.Location{Path: "server.go", Line: line, Column: 1}}}}
		anchors[id] = b.links.anchorPointer("server.go", line, 1)
	}
	add("objects", "sharedObjectsStruct", programindex.ObjectType, 600, "module")
	add("f-czero", "czero", programindex.ObjectVariable, 601, "objects")
	add("shared", "shared", programindex.ObjectVariable, 610, "module")
	add("robj", "robj", programindex.ObjectType, 620, "module")
	add("f-ptr", "ptr", programindex.ObjectVariable, 621, "robj")
	add("local", "len", programindex.ObjectVariable, 21, "cron")
	edge := func(to, kind, path string, line int) groupindex.StructuralEdge {
		return groupindex.StructuralEdge{FromSubjectID: "cron", ToSubjectID: to, Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationKind(kind),
			Resolution: programindex.ResolutionExact, FieldPath: path, Location: &programindex.Location{Path: "server.go", Line: line, Column: 9}}
	}
	index.StructuralEdges = []groupindex.StructuralEdge{
		edge("f-argv", "reads", "redisClient.argv", 21),
		edge("local", "reads", "", 21),
		edge("f-argv", "reads", "redisClient.argv", 22),
		edge("shared", "reads", "", 23),
		edge("f-czero", "reads", "shared.czero", 23),
		edge("state", "reads", "", 24),
		edge("f-db", "writes", "redisClient.db", 25),
		edge("f-db", "reads", "redisClient.db", 26),
		edge("f-argv", "reads", "redisClient.argv", 27),
		edge("f-ptr", "reads", "shared.czero.ptr", 28),
	}
	b.indexes[0] = index
	caller := func(line int) pageConnection {
		row := readingRow(anchors, "←", "#t1-g6", "Event loop and networking", "calls", "events", "processTimeEvents", "cron", "serverCron")
		row.FromSource = b.links.anchorPointer("server.go", line, 5)
		return row
	}
	raw := b.groupReading(index, part, pageGroup{ID: "t1-g14", Title: part.Title, Connections: []pageConnection{caller(301), caller(305)},
		InternalConnections: []pageConnection{
			readingRow(anchors, "", "", "", "reads", "cron", "serverCron", "f-argv", "argv"),
			readingRow(anchors, "", "", "", "reads", "cron", "serverCron", "state", "server"),
		}})
	reading := decodeReading(t, raw)
	cron := slices.IndexFunc(reading.Decls, func(decl pageReadingDecl) bool { return decl.Name == "serverCron" })
	own := reading.Own[slices.IndexFunc(reading.Own, func(owner pageReadingOwner) bool { return owner.Decl == cron })]
	var reads []string
	for _, read := range own.Reads {
		if read.Decl == nil {
			t.Fatalf("%s reads no declaration", read.Path)
		}
		reads = append(reads, read.Path+" "+reading.Decls[*read.Decl].Name)
	}
	if want := []string{"redisClient.argv redisClient", "server server", "shared.czero.ptr robj"}; !slices.Equal(reads, want) {
		t.Fatalf("serverCron reads %q, want %q", reads, want)
	}
	if len(own.Writes) != 1 || own.Writes[0].Path != "redisClient.db" {
		t.Fatalf("serverCron writes %+v", own.Writes)
	}
	for _, site := range []string{"#L301", "#L305", "#L22", "#L27"} {
		if strings.Contains(raw, site) {
			t.Fatalf("the reading keeps the use site %s: %s", site, raw)
		}
	}
	code := systemJSPiece(t, "31-reading-column.js", "function rmGroupReading(", "// An Inputs collection's reading")
	runSystemJS(t, readingViewElements+code+`
const reading=`+raw+`;
reading.decls.forEach(decl=>{if(decl.key===undefined)decl.key=decl.href;});
const cron=reading.decls.find(decl=>decl.name==='serverCron');
const view=rmDeclView(ctx,nodes['#own'],reading,{name:'serverCron',source:{Href:cron.href,Text:cron.at}});
const said=view.all(()=>true).map(c=>c.textContent).filter(text=>/:\d/.test(text));
assert.deepEqual(said,[],'no line number anywhere in the reading');
assert.equal(view.all(c=>c.has('map-reading-reads'))[0].textContent,'Reads: redisClient.argv, server, shared.czero.ptr');
assert.equal(view.all(c=>c.has('map-reading-writes'))[0].textContent,'Writes: redisClient.db');
assert.equal(view.all(c=>c.has('map-reading-name')&&c.textContent==='processTimeEvents()').length,1,'a caller calling from two places is one name');
assert.ok(!view.textContent.includes('Uses variables'));
`)
}
