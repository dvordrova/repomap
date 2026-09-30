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
const names=el=>el.all(c=>c.has('map-reading-name')).map(c=>c.textContent);
function rmEl(tag,cls,value){const item=document.createElement(tag);if(cls)item.className=cls;if(value!==undefined)item.textContent=value;return item;}
function rmT(key,...values){let used=0;const out=values.reduce((s,v,i)=>{if(s.includes('{'+i+'}'))used++;return s.replace('{'+i+'}',v);},key);if(used!==values.length)throw new Error('Extra report UI parameter: '+key);return out;}
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

// A part's reading stands as the owner chose (2026-09-28, 2026-09-29): who
// calls into it, each caller with what it calls here and a callback said
// so; the part's own box, the model's description marked by style, its
// files; its key declarations bold, one to a line, and the rest under one
// closed fold, in the page data's order, counting nothing; the parts it
// calls, with the variables it uses there apart. A
// declaration's reading: its callers by part, its name as the code link
// with its file and its author's comment, and what it calls. A name reads
// its declaration.
func TestReadingColumnViewsFollowThePreparedData(t *testing.T) {
	code := systemJSPiece(t, "31-reading-column.js", "function rmGroupReading(", "// An Inputs collection's reading")
	runSystemJS(t, readingViewElements+code+`
const part=rmPartView(ctx,nodes['#own'],data).view;
const at=cls=>part.children.findIndex(c=>c.has(cls));
assert.ok(at('map-reading-members')<at('map-reading-in')&&at('map-reading-in')<at('map-reading-out'),'what it is made of first, then who calls it (owner, 2026-09-28)');
assert.deepEqual(part.children.filter(c=>c.has('map-reading-members')).map(names),[['initServer'],['beforeSleep','serverCron','shared']],'its keys, then the rest in the page data\'s order');
const other=part.children.find(c=>c.has('map-reading-other'));
assert.ok(other.tagName==='DETAILS'&&!other.open&&other.children[0].textContent==='Other declarations','the rest wait under a closed fold');
assert.ok(!/\d/.test(part.textContent),'no counts: '+part.textContent);
const incoming=part.children[at('map-reading-in')],outgoing=part.children[at('map-reading-out')];
assert.deepEqual(names(incoming),['main()','initServer()','beforeSleep()'],'each caller with what it calls here');
assert.equal(incoming.all(c=>c.has('map-reading-relation')).length,1,'a callback is said so');
assert.deepEqual(names(outgoing),['dictResize()','server']);
assert.equal(outgoing.all(c=>c.has('map-reading-uses')).length,1,'the variables it uses stand apart from its calls');
const summary=part.all(c=>c.has('map-card-summary'))[0];
assert.ok(summary.has('model')&&summary.title,'model text says so on hover, with no chip');
assert.deepEqual(part.all(c=>c.has('map-reading-key')).map(c=>c.textContent),['initServer'],'only the key is bold, in the members list');
assert.ok(part.all(c=>c.has('map-part-box-entry')).length===1&&part.all(c=>c.has('map-part-box-core')).length===1,'a part box takes its lane');
part.all(c=>c.textContent==='dictResize()'&&c.has('map-reading-name'))[0].listeners.click({preventDefault(){},stopPropagation(){}});
part.all(c=>c.tagName==='BUTTON'&&c.textContent==='main')[0].listeners.click({stopPropagation(){}});
assert.deepEqual(read,['dictResize','main'],'a name reads its declaration and a part box its part');
const view=rmDeclView(ctx,nodes['#own'],data,{name:'serverCron',source:{Href:'h#serverCron',Text:'server.c:1'},explanation:'Runs the cron.',explanation_ref:'e1'});
const one=cls=>view.all(c=>c.has(cls))[0];
assert.ok(view.children[0].has('map-reading-side'),'its callers first');
assert.deepEqual(names(view.children[0]),['initServer()']);
assert.equal(view.children[0].all(c=>c.has('map-reading-relation')).length,1,'a callback is said so');
assert.equal(one('map-decl-code').href,'h#serverCron-L9','the name is the link to all of its code');
assert.equal(one('map-decl-signature').textContent,'(id: long)');
assert.ok(one('map-decl-where').textContent.includes('server.c')&&!/:\d/.test(one('map-decl-where').textContent),'the file alone, no line');
assert.ok(one('map-author-comment').textContent.includes('Called every 100 ms.'),'the author\'s comment stands in the reading, marked as theirs');
assert.ok(one('map-decl-explanation').has('model'),'the model\'s line is marked as the model\'s');
`)
}

// The home names every program with its role, entry, the kinds of its
// inputs in words, its connections and the files it is built from, one to
// a line and counting nothing (owner, 2026-09-28, 2026-09-29).
func TestTheHomesProgramsFollowThePageData(t *testing.T) {
	code := systemJSPiece(t, "31-reading-column.js", "function rmGroupReading(", "// An Inputs collection's reading") +
		systemJSPiece(t, "31-reading-column.js", "var rmLanguageNames=", "function rmCollectionView(") +
		systemJSPiece(t, "31-reading-column.js", "var rmPendingKind=", "// The files a component's program reaches") +
		systemJSPiece(t, "31-reading-column.js", "// The home's table of programs", "// </reading-column>")
	runSystemJS(t, readingViewElements+code+`
const all={
 'system-component-t1':{id:'system-component-t1',dataset:{title:'redis-server',owner:'t1',role:'Backend database server',children:'area core',entries:'[{"name":"main","callable":true}]',sources:'["adlist.c","redis.c"]'}},
 'system-inputs-t1':{id:'system-inputs-t1',dataset:{collection:'{"groups":[],"kinds":[{"kind":"request","inputs":["a","b"]},{"kind":"setting","inputs":["c"]}]}'}}};
const opened=[];
const context={nodeByHref:href=>all[href.slice(1)]||null,nodeById:id=>all[id]||null,readNode:n=>opened.push(n.dataset.title)};
const holder=rmEl('div');
rmProgramsTable(context,holder,[all['system-component-t1']],()=>[{incoming:true,title:'redis-cli'},{incoming:false,title:'TCP endpoint'}]);
assert.deepEqual(holder.all(c=>c.has('model')).map(c=>c.textContent),['Backend database server'],'its role is the model\'s');
const lines=holder.all(c=>c.tagName==='LI').map(c=>c.textContent);
assert.deepEqual(lines,['main()','←\u00a0redis-cli','→\u00a0TCP endpoint','adlist.c','redis.c'],'its entry, connections by direction and files, one to a line, an arrow kept with its name');
assert.ok(holder.textContent.includes('Incoming requests · Settings'),'the kinds of its inputs in words');
assert.ok(!/\d/.test(holder.textContent),'no counts: '+holder.textContent);
holder.all(c=>c.tagName==='BUTTON'&&c.textContent==='redis-server')[0].listeners.click();
assert.deepEqual(opened.at(-1),'redis-server','its name reads it');
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
const other=view.children.find(c=>c.has('map-reading-other'));
assert.deepEqual(other.children.filter(c=>c.has('map-reading-file')).map(c=>[c.children[0].textContent,...names(c)]),
 [['adlist.c','listCreate'],['dict.c','dictCreate'],['sds.c','sdsnew']],'file by file under the fold');
const lines=view.all(c=>c.has('map-reading-caller'));
assert.equal(lines.length,1,'the fan-out is one line');
assert.equal(lines[0].tagName,'DETAILS','folded');
const head=lines[0].children[0];
assert.deepEqual(names(head),['loadAppendOnlyFile()','cmdTable'],'the caller and the site it dispatches through');
assert.equal(head.textContent,'loadAppendOnlyFile() calls one of these request handlers through cmdTable','in plain words, not counted: '+head.textContent);
assert.equal(lines[0].all(c=>c.has('possible')||c.has('map-reading-relation')).length,0,'"one of these" says they are possible; no row repeats it');
// A fan through no dispatch site (an interface field's alternatives) names
// none and still reads.
fanned.in[0].lines[0].fan={of:5,noun:'',via:[]};
const plain=rmPartView(ctx,nodes['#own'],fanned).view.all(c=>c.has('map-reading-caller'))[0].children[0];
assert.equal(plain.textContent,'loadAppendOnlyFile() calls one of these','no site: '+plain.textContent);
`)
}

// A relation other than a call is said once for a run of names, in plain
// words, and each name stands once on its own line (reviewer, 2026-09-30:
// Server admin commands had read "authCommand() — passed as a callback"
// thirteen times, each wrapping to two lines, and Client I/O's "Calls
// into" had listed each command twice, as a possible call and as a
// callback): a caller whose ends are one run says it on its own line; a
// list of several runs says each above its names, plain calls first; a
// single name keeps its words; callers of a declaration are said alike.
func TestARelationIsSaidOnceForItsRun(t *testing.T) {
	code := systemJSPiece(t, "31-reading-column.js", "function rmGroupReading(", "// An Inputs collection's reading")
	runSystemJS(t, readingViewElements+code+`
const d=(name,part)=>({name,kind:'function',part,key:'h#'+name,href:'h#'+name,file:'redis.c',at:'redis.c:1'});
const commands=['auth','bgsave','echo','ping'].map(n=>d(n+'Command','#own'));
const decls=[Object.assign(d('cmdTable','#main'),{kind:'variable'}),d('deleteKey','#core'),d('lookupCommand','#core')].concat(commands);
const table={decls,files:['redis.c'],members:[{kind:'function',decls:[3,4,5,6]}],
 in:[{part:'',inputs:true,title:'Inputs',count:4,lines:[{caller:0,ends:[3,4,5,6].map(i=>({decl:i,kind:'passes_callback'}))}]}],
 out:[{part:'#core',title:'Server core state',count:5,lines:[{caller:-1,ends:[3,4,5].map(i=>({decl:i,kind:'calls',possible:true})).concat([{decl:1,kind:'calls'}],[3,4,5].map(i=>({decl:i,kind:'passes_callback'})),[{decl:2,kind:'passes_callback'}])}]}]};
const view=rmPartView(ctx,nodes['#own'],table).view;
const incoming=view.children.find(c=>c.has('map-reading-in')),outgoing=view.children.find(c=>c.has('map-reading-out'));
const caller=incoming.all(c=>c.has('map-reading-caller'))[0];
assert.ok(caller.textContent.startsWith('cmdTable passes these as callbacks'),'said once, on the caller\'s line: '+caller.textContent);
assert.equal(caller.all(c=>c.has('map-reading-relation')).length,0,'no name repeats it');
assert.deepEqual(caller.all(c=>c.tagName==='LI').map(c=>c.textContent),['authCommand()','bgsaveCommand()','echoCommand()','pingCommand()'],'one name to a line');
assert.deepEqual(outgoing.all(c=>c.has('map-reading-peer'))[0].children.filter(c=>c.tagName==='UL'||c.has('map-reading-run')).map(c=>c.tagName==='UL'?c.all(x=>x.tagName==='LI').map(x=>x.textContent).join(' '):c.textContent),
 ['deleteKey()','possibly called, passed as callbacks:','authCommand() bgsaveCommand() echoCommand()','lookupCommand()passed as a callback'],
 'plain calls first, then each run said once above its names, each name once; a single name keeps its words');
const callers=decls.concat([d('serverCron','#own')]);
const one={decls:callers,files:['redis.c'],members:[{kind:'function',decls:[7]}],own:[{decl:7,callers:[{part:'#core',title:'Server core state',decls:[{decl:1,kind:'calls'},{decl:2,kind:'calls',possible:true},{decl:0,kind:'calls',possible:true}]}]}]};
const reading=rmDeclView(ctx,nodes['#own'],one,{name:'serverCron',source:{Href:'h#serverCron',Text:'redis.c:1'}});
assert.deepEqual(reading.children[0].all(c=>c.has('map-reading-run')).map(c=>c.textContent),['may call it:'],'its callers are said alike: '+reading.children[0].textContent);
assert.equal(reading.children[0].all(c=>c.has('possible')).length,0,'no caller repeats "possible"');
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
const groups=view.children[0].all(c=>c.has('map-reading-peer'));
assert.deepEqual(groups.map(g=>[g.all(c=>c.has('map-reading-program')).map(c=>c.textContent).join(''),names(g)]),[['',['initServer()']],['redis-cli:',['cliConnect()']]],'its own program\'s callers, then each other program\'s, named');
assert.ok(view.all(c=>c.has('map-reading-not-called'))[0].textContent.includes('redis-benchmark'),'the program never running it is named');
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
assert.deepEqual(grid.all(c=>c.tagName==='DT').map(c=>c.textContent),['fd','argc','db'],'its fields in order');
const uses=grid.all(c=>c.has('map-field-uses-row'));
assert.equal(uses.length,1,'only a field someone writes or reads has its uses');
assert.deepEqual(uses[0].all(c=>c.has('map-field-side')).map(names),[['serverCron()'],['initServer()','main()']],'written by, then read by, each its functions');
// A field's repository type reads that type.
const typeReads=read.length;grid.children.at(-1).all(c=>c.has('map-reading-name'))[0].listeners.click({button:0,preventDefault(){},stopPropagation(){}});
assert.deepEqual(read.slice(typeReads),['redisDb'],'the field type links to its type');
assert.ok(!/:\d/.test(grid.textContent),'no line numbers');
const shared=rmDeclView(ctx,nodes['#own'],data,{name:'shared',source:{Href:'h#shared',Text:'server.c:5'}});
const reached=shared.all(c=>c.has('map-reading-fields'))[0];
assert.deepEqual([reached.all(c=>c.has('map-field-path')).map(c=>c.textContent),names(reached)],[['shared.crlf'],['initServer()']],'a variable lists the fields reached through it');
const cron=rmDeclView(ctx,nodes['#own'],data,{name:'serverCron',source:{Href:'h#serverCron',Text:'server.c:1'}});
const writes=cron.all(c=>c.has('map-reading-writes'));
assert.equal(writes.length,1,'its writes are one line');
assert.deepEqual(names(writes[0]),['redisClient.fd'],'each field once, its name reading its type');
assert.ok(writes[0].textContent.includes('server.hz'),'a field of no known type is still named');
const reads=read.length;writes[0].all(c=>c.has('map-reading-name'))[0].listeners.click({button:0,preventDefault(){},stopPropagation(){}});
assert.deepEqual(read.slice(reads),['redisClient'],'a written field reads the type declaring it');
`)
}

// A function's reading lists its calls as its flow, under "Calls", in the
// order Go wrote them, each run into one part under that part's box, with
// no caption repeating its name ("serverCron calls, in order:"). More than
// three helper calls fold under one muted "+ helpers" naming none of them;
// three or fewer stand as rows, as do the calls of a step whose every call
// is a helper (owner, 2026-09-29: processCommand's own calls had waited
// among eighteen names). The fold and "Show helper calls" open them in
// place. A call opens in place to its callee's flow, a call its ancestors
// make says it is shown above, a call of itself is no row but one quiet
// line (reviewer, 2026-09-30: othello.ai/move's calls had opened with
// "othello.ai/move() ↑ shown above"), a call of its other arity names the
// form it calls, and a library's call is a plain row.
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
d.push(decl('redisLog','function','#core'),decl('rdbSave','function','#persist'),decl('lookupKeyRead','function',''),decl('zmallocUsed','function','#core'),decl('dictRandom','function','#core'),decl('freeOne','function','#core'));
const log=d.length-6,save=d.length-5,lookup=d.length-4,used=d.length-3,random=d.length-2,free=d.length-1;
data.own[0].flow=[{decl:log,helper:true,sites:[{at:'server.c:1273'},{at:'server.c:1288'}]},{decl:2,sites:[{at:'server.c:1284'}]},{name:'wait3',lib:'sys/wait.h',kind:'invokes_external',sites:[{at:'server.c:1304'}]},
  {decl:used,helper:true,sites:[{at:'server.c:1310'}]},{decl:save,sites:[{at:'server.c:1322'}]},{decl:save,kind:'passes_callback',sites:[{at:'server.c:1330'}]},{decl:random,helper:true,sites:[{at:'server.c:1340'}]},
  {decl:lookup,sites:[{at:'server.c:1350'}]},{decl:0,sites:[{at:'server.c:1360'}]},{decl:0,arity:'[b n]',sites:[{at:'server.c:1362'}]},{decl:free,helper:true,sites:[{at:'server.c:1365'}]},
  {macro:'assert',lib:'assert.h',sites:[{at:'server.c:1370'}]},{decl:1,macro:'redisAssert',sites:[{at:'server.c:1380'}]}];
data.own.push({decl:save,flow:[{decl:log,helper:true,sites:[{at:'server.c:3010'}]}]});
data.own.push({decl:lookup,flow:[{decl:log,helper:true,sites:[{at:'server.c:904'}]},{decl:1,sites:[{at:'server.c:905'}]},{decl:0,sites:[{at:'server.c:906'}]},{name:'strerror',lib:'string.h',kind:'invokes_external',sites:[{at:'server.c:907'}]}]});
const view=rmDeclView(ctx,nodes['#own'],data,{name:'serverCron',source:{Href:'h#serverCron',Text:'server.c:1'}});
const said=view.textContent;
assert.ok(!/serverCron calls|in order|steps|\d+ helpers|only helpers|not on the map|also from/.test(said),'no caption repeats its name and no meta word: '+said);
const root=view.all(c=>c.has('map-flow-root'))[0];
const rows=()=>root.all(c=>c.has('map-flow-row')&&!c.has('map-flow-helper')).map(c=>c.all(x=>x.has('map-reading-name')||x.has('map-flow-plain'))[0].textContent);
assert.deepEqual(rows(),['beforeSleep()','rdbSave()','rdbSave()','lookupKeyRead()','redisAssert'],'the calls in the order Go wrote them, helpers folded, a macro as written, and never itself');
const itself=root.all(c=>c.has('map-flow-itself'));
assert.deepEqual(itself.map(c=>c.textContent),['calls itself','calls its [b n] form'],'its call of itself is said once, plainly, and a call of its other arity names that form');
const macro=root.all(c=>c.has('map-reading-name')&&c.textContent==='redisAssert')[0];
assert.ok(macro.title.includes('initServer'),'a macro names what its expansion calls on its hover: '+macro.title);
// A call into code the report names no declaration for (wait3, the
// assert macro) is no row: the step ends in one line naming each once,
// in the order written, and one part's calls stay under one box (owner,
// 2026-09-29: as rows they had split Replication's calls under two).
const also=root.all(c=>c.has('map-flow-also'));
assert.equal(also.length,1,'one line of the calls outside the report');
assert.deepEqual(also[0].all(c=>c.has('map-flow-plain')).map(c=>c.textContent),['wait3','assert']);
assert.deepEqual(root.all(c=>c.has('map-part-box')).map(c=>c.textContent),['Server lifecycle and cron','Persistence','Server lifecycle and cron'],'each run into one part stands under its box');
const lists=[root,...root.all(c=>c.has('map-flow-list'))].map(l=>l.has('map-flow-list')?l:l.children[0]);
for(const list of lists){const boxes=list.children.filter(c=>c.has&&c.has('map-flow-group')).map(g=>(g.children[0]&&g.children[0].has('map-part-box'))?g.children[0].textContent:'');
  boxes.forEach((box,i)=>assert.ok(!box||box!==boxes[i-1],'a part\'s box repeats only with another group between: '+boxes));}
// A name is its link: no row carries a separate code mark, a link with no
// name of its own (owner, 2026-09-29).
assert.deepEqual(view.all(c=>c.tagName==='A'&&c.textContent==='').length,0,'no code mark beside a name');
const helperLine=root.all(c=>c.has('map-flow-helpers'));
assert.equal(helperLine.length,1,'one fold of helper calls under the step');
assert.deepEqual(names(helperLine[0]),[],'the fold names none of them: several names to a line were a wall');
assert.equal(helperLine[0].children[0].textContent,'+ helpers');
// The fold opens the step's helpers into rows in place, and folds them again.
helperLine[0].children[0].listeners.click({stopPropagation(){}});
const helperRows=root.all(c=>c.has('map-flow-row')&&c.has('map-flow-helper'));
assert.deepEqual(helperRows.map(c=>c.textContent),['redisLog()','zmallocUsed()','dictRandom()','freeOne()'],'opened, each helper is a row in its place');
const helperReads=read.length;helperRows[0].all(c=>c.has('map-reading-name'))[0].listeners.click({button:0,preventDefault(){},stopPropagation(){}});
assert.deepEqual(read.slice(helperReads),['redisLog'],'a helper name reads its declaration');
root.all(c=>c.has('map-flow-helpers'))[0].children[0].listeners.click({stopPropagation(){}});
assert.equal(root.all(c=>c.has('map-flow-row')&&c.has('map-flow-helper')).length,0,'and folds them again');
// Opening rdbSave shows its only call, a helper, as its call.
const saveRow=root.all(c=>c.tagName==='DETAILS'&&c.textContent.startsWith('rdbSave()'))[0];
saveRow.open=true;
assert.ok(saveRow.all(c=>c.has('map-flow-row')).some(r=>r.textContent==='redisLog()'&&!r.has('map-flow-helper')),'a step whose every call is a helper shows them');
// Three helper calls or fewer stand as rows: lookupKeyRead's one.
const lookupOpen=root.all(c=>c.tagName==='DETAILS'&&c.textContent.startsWith('lookupKeyRead()'))[0];
lookupOpen.open=true;
assert.ok(lookupOpen.all(c=>c.has('map-flow-row')).some(r=>r.textContent==='redisLog()'&&!r.has('map-flow-helper'))&&lookupOpen.all(c=>c.has('map-flow-helpers')).length===0,'a lone helper call stands as a row');
assert.ok(lookupOpen.all(c=>c.has('map-flow-above')).length===1&&lookupOpen.all(c=>c.has('map-flow-itself')).length===0,'a call back into an ancestor is shown above, not "itself"');
// One "also calls:" line under the whole flow, gathering what the opened
// calls reach outside the report, its hover naming who calls it
// (reviewer, 2026-09-30: othello's key-pressed had stacked three).
const alsoLines=root.all(c=>c.has('map-flow-also'));
assert.equal(alsoLines.length,1,'one line for the whole flow');
assert.deepEqual(alsoLines[0].all(c=>c.has('map-flow-plain')).map(c=>c.textContent),['wait3','assert','strerror'],'its own, then the opened call\'s');
assert.ok(alsoLines[0].all(c=>c.textContent==='strerror')[0].title.includes('lookupKeyRead'),'the opened call\'s names say who calls them');
lookupOpen.open=false;
assert.deepEqual(root.all(c=>c.has('map-flow-also'))[0].all(c=>c.has('map-flow-plain')).map(c=>c.textContent),['wait3','assert'],'closed, its names go');
// The declaration no part holds is a plain name that still opens.
const lookupRow=root.all(c=>c.tagName==='DETAILS'&&c.textContent.startsWith('lookupKeyRead()'))[0];
assert.ok(lookupRow&&lookupRow.all(c=>c.has('map-flow-plain')).length>0,'a declaration no part holds keeps its call, a plain name');
// The toggle shows the helper calls, the open ones staying open.
rmFlowHelpers=true;root.rmRender();
const shownHelpers=root.all(c=>c.has('map-flow-helper'));
assert.deepEqual(shownHelpers.map(c=>c.textContent),['redisLog()','zmallocUsed()','dictRandom()','freeOne()'],'the toggle shows the folded helper calls, lighter, in their place');
assert.equal(root.all(c=>c.has('map-flow-helpers')).length,0,'with the toggle on, no step keeps its line');
assert.ok(root.all(c=>c.tagName==='DETAILS'&&c.textContent.startsWith('rdbSave()')&&c.open).length===1,'what was open stays open');
`)
}

// An input's reading opens at how a request reaches it: the first way as
// one chain, a line per part, the handler last in its part, the callable
// handed over saying how on its hover; the other ways folded on one line
// named by the part where each leaves the first, the inputs that run the
// site themselves listed inside it, not counted; then who sends it, last.
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
const first=section.children.find(c=>c.has('map-flow-chain'));
assert.deepEqual(first.children.map(run=>[run.all(c=>c.has('map-part-box'))[0].textContent,names(run)]),
 [['Server core state',['acceptHandler()','readQueryFromClient()','call()']],['Server lifecycle and cron',['getCommand()']]],'the first way, a line per part, the handler last');
const hop=first.all(c=>c.textContent==='readQueryFromClient()'&&c.has('map-reading-name'))[0];
assert.ok(hop.title.includes('createClient')&&hop.title.includes('acceptHandler'),'the callable handed over says how on its hover: '+hop.title);
const others=section.children.find(c=>c.has('map-flow-other-ways'));
assert.ok(others.tagName==='DETAILS'&&!others.open,'the other ways are folded');
assert.deepEqual(others.children[0].all(c=>c.has('map-part-box')).map(c=>c.textContent),['Replication'],'named by the part where each leaves the first');
assert.ok(!/\d/.test(others.children[0].textContent),'the fold counts nothing: '+others.children[0].textContent);
const sent=section.children.at(-1);
assert.ok(sent.has('map-flow-sent')&&sent.textContent.includes('redis-cli'),'who sends it, last');
`)
}

// An input a case of its handler's comparison declares does what the case's
// lines call (critic, 2026-09-30: litestream's replicate had read all of
// Main.Run): its "What it does" is that case's flow, never the handler's
// other cases, and an input handled by the whole function reads its flow.
func TestACaseInputDoesWhatItsCaseCalls(t *testing.T) {
	reading := systemJSPiece(t, "31-reading-column.js", "function rmGroupReading(", "// An Inputs collection's reading")
	flow := systemJSPiece(t, "32-flow.js", "var rmFlowHelpers", "// </flow>")
	runSystemJS(t, readingViewElements+reading+flow+`
El.prototype.insertBefore=function(c,ref){c.parent=this;const at=this.children.indexOf(ref);if(at<0)this.children.push(c);else this.children.splice(at,0,c);return c;};
El.prototype.closest=function(){return null;};
const d=data.decls;
d.push(decl('Run','function','#own'),decl('NewReplicateCommand','function','#core'),decl('ReplicateRun','function','#core'),decl('DatabasesRun','function','#core'));
const run=d.length-4,make=d.length-3,replicate=d.length-2,databases=d.length-1;
data.own.push({decl:run,flow:[{decl:make,sites:[{at:'main.go:120'}]},{decl:replicate,sites:[{at:'main.go:124'}]},{decl:databases,sites:[{at:'main.go:131'}]}],
  cases:[{line:119,flow:[{decl:make,sites:[{at:'main.go:120'}]},{decl:replicate,sites:[{at:'main.go:124'}]}]},{line:130,flow:[{decl:databases,sites:[{at:'main.go:131'}]}]}]});
document.getElementById=id=>id==='own'?{dataset:{reading:'1'},rmReading:data}:null;
ctx.nodeById=id=>id==='n-own'?nodes['#own']:null;
const does=path=>{const section=rmInputFlowSection(ctx,path,'replicate',()=>null,()=>{});const box=section&&section.children.find(c=>c.has('map-flow-does'));
  return box?box.all(c=>c.has('map-flow-row')).map(c=>c.all(x=>x.has('map-reading-name'))[0].textContent):null;};
const path={decls:[{name:'Run',part:'n-own',href:'h#Run'}],parts:[{depth:0,handler:0,part:'n-own'}]};
assert.deepEqual(does(Object.assign({case:119},path)),['NewReplicateCommand()','ReplicateRun()'],'the replicate case\'s calls alone');
assert.deepEqual(does(Object.assign({case:130},path)),['DatabasesRun()'],'the databases case\'s');
assert.deepEqual(does(path),['NewReplicateCommand()','ReplicateRun()','DatabasesRun()'],'an input the whole function handles reads its flow');
assert.equal(does(Object.assign({case:200},path)),null,'a case with no flow of its own shows none, never the whole handler');
`)
}

// A function's reading prints no line number (owner, 2026-09-29:
// "человек будет видеть код"): a caller calling from two places is its
// name once, the name its link and no place kept beside it, and
// "Uses variables" ("argv :4248 :4250 …", fields reached
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
	// A read keeps no place; a caller keeps where it makes the call, its
	// first place in source order, for its name's code link (owner,
	// 2026-09-29: the caller's name had opened processCommand at its top,
	// above the call). The page prints no separate code marks.
	for _, site := range []string{"#L22", "#L27", "#L305"} {
		if strings.Contains(raw, site) {
			t.Fatalf("the reading keeps the site %s: %s", site, raw)
		}
	}
	var site *pageReadingSite
	for _, group := range own.Callers {
		for _, end := range group.Decls {
			if reading.Decls[end.Decl].Name == "processTimeEvents" {
				site = end.Site
			}
		}
	}
	if site == nil || !strings.HasSuffix(site.Href, "#L301") || !strings.Contains(site.At, "305") {
		t.Fatalf("processTimeEvents calls serverCron at %+v, want its first line linked and both named", site)
	}
	code := systemJSPiece(t, "31-reading-column.js", "function rmGroupReading(", "// An Inputs collection's reading")
	runSystemJS(t, readingViewElements+code+`
const reading=`+raw+`;
reading.decls.forEach(decl=>{if(decl.key===undefined)decl.key=decl.href;});
const cron=reading.decls.find(decl=>decl.name==='serverCron');
const view=rmDeclView(ctx,nodes['#own'],reading,{name:'serverCron',source:{Href:cron.href,Text:cron.at}});
const said=view.all(()=>true).map(c=>c.textContent).filter(text=>/:\d/.test(text));
assert.deepEqual(said,[],'no line number anywhere in the reading');
assert.deepEqual(view.all(c=>c.has('map-reading-reads')).map(names),[['redisClient.argv','server','shared.czero.ptr']],'one line, each read once, in the order first used');
assert.deepEqual(view.all(c=>c.has('map-reading-writes')).map(names),[['redisClient.db']]);
assert.equal(view.all(c=>c.has('map-reading-name')&&c.textContent==='processTimeEvents()').length,1,'a caller calling from two places is one name');
assert.equal(view.all(c=>c.tagName==='A'&&c.textContent==='').length,0,'and no code mark beside it');
assert.ok(!view.textContent.includes('Uses variables'));
`)
}

// A kind chosen in an Inputs collection's frame, or a count in its
// component's reading, opens the collection at that kind's section,
// Background work at the first of its scheduled and continuous sections
// (owner, 2026-09-29: Settings had opened the reading at its 96 requests).
func TestAKindChosenOpensItsCollectionAtItsSection(t *testing.T) {
	code := nameBreaksJS(t) + systemJSPiece(t, "31-reading-column.js", "var rmLanguageNames=", "// Where a catalogue's inputs are declared") + "\nvar rmPendingKind='';\n"
	runSystemJS(t, readingViewElements+code+`
const inputs={a:{dataset:{title:'get'}},b:{dataset:{title:'dir'}},c:{dataset:{title:'IOThreadEntryPoint'}},d:{dataset:{title:'serverCron'}}};
const node={dataset:{owner:'t1',collection:JSON.stringify({groups:[{kind:'request',inputs:['a']},{kind:'setting',inputs:['b']},{kind:'continuous',inputs:['c']},{kind:'scheduled',inputs:['d']}]})}};
const context={nodeById:id=>inputs[id]||null,nodeByHref:()=>null,light(){},readNode(){}};
const at=()=>rmCollectionView(context,node,rmPage.data(node,'collection')).children.filter(c=>c.dataset&&'readingAnchor' in c.dataset).map(c=>c.dataset.kind);
rmPendingKind='setting';assert.deepEqual(at(),['setting']);
rmPendingKind=['scheduled','continuous'];assert.deepEqual(at(),['continuous'],'Background work lands on the first of its sections');
assert.deepEqual(at(),[],'a later reading opens at its top');
`)
}
