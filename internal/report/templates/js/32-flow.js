// A declaration's flow (page_flow.go, owner-approved 2026-09-29, the
// designer's flow v2): its calls in the order they are written, each run of
// calls into one part under that part's box, drawn as the column draws a
// part (its description on hover, a click reads it). A call opens in place
// to its callee's own flow, grouped the same way; when all of those stay in
// the caller's part, no box repeats. The twist shows under the pointer or
// focus on a call that can open; a name reads its declaration and shows it
// on the canvas; a call its ancestors already make says it is shown above.
// Helper calls (the helper question's decision, into a part most of the
// program's parts call into) stand as one muted line under their step,
// "+ helpers: createListObject, dictAdd", each name reading its
// declaration; the line opens them into rows in place, and "Show helper
// calls" opens every step's. A step whose every call is a helper shows
// them as its calls. No name is hidden (owner, 2026-09-29). Where a call is
// written and what it hands over are on its name's hover. Go ordered every
// list; nothing here sorts.
// <flow>
var rmFlowHelpers=false;
// The part node a declaration stands in: a reading names it by its link
// ("#t1-g6"), an input's path by its node ("n-t1-g6").
function rmFlowPart(ctx,part){return part?(part.charAt(0)==='#'?ctx.nodeByHref(part):ctx.nodeById(part)):null;}
// Where a declaration's own flow is: in the reading at hand (a declaration
// no part holds is read there), else in its part's reading.
function rmFlowOwner(ctx,data,decl){
  var key=decl.key||decl.href||decl.open;if(!key)return null;
  function find(reading){
    if(!reading)return null;
    var position=reading.decls.findIndex(function(other){return (other.key||other.href||other.open)===key;});
    if(position<0)return null;
    var own=(reading.own||[]).find(function(entry){return entry.decl===position&&(entry.flow||[]).length;});
    return own?{data:reading,own:own}:null;
  }
  var here=find(data);if(here)return here;
  var part=rmFlowPart(ctx,decl.part);
  return part?find(rmGroupReading(part)):null;
}
// A call's hover: the part it goes into, what it hands over, and where it
// is written ("called at redis.c:1273 · 1288").
function rmFlowTitle(ctx,decl,call){
  var part=decl&&rmFlowPart(ctx,decl.part),sites=(call.sites||[]).map(function(site,i){return i?site.at.split(':').pop():site.at;});
  var kind=call.kind||'calls',words=kind==='calls'||kind==='invokes_external'?'':rmEndWords.out[kind]||kind.replace(/_/g,' ');
  return [part?part.dataset.title:'',words?rmT(words):'',sites.length?rmT('called at {0}',sites.join(' · ')):''].filter(Boolean).join('\n');
}
// A call's name. A macro's call is the macro as written ("redisAssert"),
// its hover naming what its expansion calls; the name reads that.
function rmFlowName(ctx,decl,call){
  var text=rmCallableName(decl),title=rmFlowTitle(ctx,decl,call);
  if(call.macro&&call.decl!==undefined){text=call.macro;title=[rmT('{0} expands to a call of {1}',call.macro,decl.name),title].filter(Boolean).join('\n');}
  // A declaration no part holds is a plain name.
  if(!decl.part){var plain=rmEl('span','map-reading-name map-flow-plain',text);plain.title=title;return plain;}
  var name=rmDeclName(decl,text,ctx.goDecl(decl),title);
  var part=rmFlowPart(ctx,decl.part);
  if(part){name.addEventListener('mouseenter',function(){ctx.light([part.id]);});name.addEventListener('mouseleave',function(){ctx.light([]);});}
  return name;
}
// One call of a flow: a row that opens to its callee's flow when it has one.
function rmFlowRow(ctx,data,call,opts,helper){
  var decl=call.decl!==undefined?data.decls[call.decl]:null;
  if(!decl&&!call.one){
    // A call into code the report names no declaration for (a library's
    // fork), or a macro whose expansion calls only such code ("assert"):
    // a plain row.
    var lib=rmEl('div','map-flow-row map-flow-lib'+(helper?' map-flow-helper':'')),said=rmEl('span','map-flow-plain',call.macro||call.name+'()');
    said.title=[call.lib,call.macro?rmT('a macro'):'',rmFlowTitle(ctx,null,call)].filter(Boolean).join('\n');lib.appendChild(said);return rmSiteMarks(lib,call.sites);
  }
  if(call.one){
    // A dispatch site: one call, one of the declarations it can call; a
    // macro's call, the macro as written, its declarations under it.
    var site=rmEl('details','map-flow-row map-flow-dispatch'+(helper?' map-flow-helper':'')),head=rmEl('summary','map-flow-head');
    head.appendChild(rmEl('span','map-flow-twist'));
    if(call.macro)head.appendChild(rmEl('span','map-flow-plain',call.macro+(call.every?'':' ')));
    if(!call.every)head.appendChild(rmEl('span','map-flow-one',rmT('one of {0}',call.one.length)));
    rmSiteMarks(head,call.sites);
    head.title=rmFlowTitle(ctx,null,call);site.appendChild(head);
    site.addEventListener('toggle',function(){
      if(!site.open||site.dataset.drawn)return;site.dataset.drawn='1';
      var names=rmEl('p','map-flow-names');
      call.one.forEach(function(at,i){if(i)names.appendChild(document.createTextNode(' '));names.appendChild(rmFlowName(ctx,data.decls[at],{sites:call.sites,kind:call.kind}));});
      site.appendChild(names);
    });
    return site;
  }
  var key=decl.key||decl.href||decl.open,cycle=opts.ancestors.has(key),target=cycle?null:rmFlowOwner(ctx,data,decl),path=opts.path+'>'+(call.kind||'calls')+':'+key;
  var row=rmEl(target?'details':'div','map-flow-row'+(helper?' map-flow-helper':'')),head=rmEl(target?'summary':'div','map-flow-head');
  head.append(rmEl('span','map-flow-twist'),rmFlowName(ctx,decl,call));
  rmSiteMarks(head,call.sites);
  if(cycle)head.appendChild(rmEl('span','map-flow-above meta',rmT('↑ shown above')));
  row.appendChild(head);
  if(target){
    var draw=function(){
      if(row.dataset.drawn)return;row.dataset.drawn='1';
      var ancestors=new Set(opts.ancestors);ancestors.add(key);
      row.appendChild(rmFlowList(ctx,target.data,target.own,{parentPart:decl.part||'',ancestors:ancestors,open:opts.open,path:path,auto:opts.single&&opts.auto>0?opts.auto-1:0}));
    };
    row.addEventListener('toggle',function(){if(row.open){opts.open.add(path);draw();}else opts.open.delete(path);});
    // A handler's single call opens by itself, as far as its calls go one
    // at a time (get → getGenericCommand → lookupKeyReadOrReply).
    if(opts.single&&opts.auto>0)opts.open.add(path);
    if(opts.open.has(path)){draw();row.open=true;}
  }
  return row;
}
// A flow's calls, each run into one part under its box. Top level names
// every part; an opened call names none when all its calls stay in its
// caller's part. Its helper calls stand as one muted line after its rows,
// each name a link, until the line or the toggle opens them in place.
function rmFlowList(ctx,data,own,opts){
  var list=rmEl('div','map-flow-list'),calls=own.flow||[];
  var every=calls.length>0&&calls.every(function(call){return call.helper;});
  var work=calls.filter(function(call){return every||!call.helper;}),helpers=every?[]:calls.filter(function(call){return call.helper;});
  opts=Object.assign({},opts,{single:work.length===1,auto:(opts.auto||0)});
  var key=opts.path+'\u0000helpers';
  function partOf(call){
    if(call.decl!==undefined)return (data.decls[call.decl]||{}).part||'';
    if(call.one)return (data.decls[call.one[0]]||{}).part||'';
    return '\0';
  }
  function draw(){
    list.replaceChildren();
    var opened=rmFlowHelpers||opts.open.has(key),shown=opened?calls:work;
    var stays=opts.parentPart&&shown.every(function(call){var part=partOf(call);return part===opts.parentPart||part==='\0';});
    var group=null,at=null;
    shown.forEach(function(call){
      var part=partOf(call);
      if(!group||part!==at){
        group=rmEl('div','map-flow-group');at=part;
        if(!stays&&part&&part!=='\0'){var box=rmPartBox(ctx,part,'');group.appendChild(box);var node=rmFlowPart(ctx,part);if(node&&node.dataset.summary)box.title=node.dataset.summary;}
        list.appendChild(group);
      }
      group.appendChild(rmFlowRow(ctx,data,call,opts,call.helper&&!every));
    });
    if(!helpers.length||rmFlowHelpers)return;
    // The step's helper calls, one muted line: every name shown and read
    // by a click; "+ helpers" opens them in place, "− helpers" folds them.
    var line=rmEl('p','map-flow-helpers'),more=rmEl('button','map-flow-helpers-toggle',opened?rmT('− helpers'):rmT('+ helpers:'));
    more.type='button';more.setAttribute('aria-expanded',String(opened));
    more.addEventListener('click',function(event){event.stopPropagation();if(opts.open.has(key))opts.open.delete(key);else opts.open.add(key);draw();});
    line.appendChild(more);
    // Each helper keeps where it is called: its code marks show while the
    // name is pointed at or focused.
    if(!opened)helpers.forEach(function(call,i){
      line.appendChild(document.createTextNode(i?', ':' '));
      var decl=call.decl!==undefined?data.decls[call.decl]:null,one=rmEl('span','map-flow-helper-call');
      one.appendChild(decl?rmFlowName(ctx,decl,call):rmEl('span','map-flow-plain',call.macro||(call.one?rmT('one of {0}',call.one.length):call.name||'')));
      line.appendChild(rmSiteMarks(one,call.sites));
    });
    list.appendChild(line);
  }
  draw();
  return list;
}
// A declaration's flow, kept whole across "Show helper calls": what is open
// stays open, a step's helpers opened by their line too.
function rmFlowTree(ctx,data,own,auto){
  var root=rmEl('div','map-flow-root'),open=new Set(),key=(data.decls[own.decl]||{}).key||'';
  root.rmRender=function(){root.replaceChildren(rmFlowList(ctx,data,own,{parentPart:'',ancestors:new Set([key]),open:open,path:'',auto:auto||0}));};
  root.rmRender();
  return root;
}
// The one quiet toggle of a reading's flows.
function rmFlowToggle(card){
  var label=rmEl('label','map-flow-toggle'),box=rmEl('input');box.type='checkbox';box.checked=rmFlowHelpers;
  label.append(box,document.createTextNode(' '+rmT('Show helper calls')));
  box.addEventListener('change',function(){
    rmFlowHelpers=box.checked;
    var scope=card||label.closest('.map-card')||document;
    scope.querySelectorAll('.map-flow-toggle input').forEach(function(other){other.checked=rmFlowHelpers;});
    scope.querySelectorAll('.map-flow-root').forEach(function(root){root.rmRender();});
  });
  return label;
}
// A step of the component's Main flow opens in place to its code flow.
function rmFlowStep(ctx,step,part,key){
  var target=rmFlowOwner(ctx,null,{key:key,part:part.getAttribute('href')||'#'+part.id});
  if(!target)return;
  var twist=rmEl('button','map-flow-step-twist');twist.type='button';twist.setAttribute('aria-expanded','false');twist.setAttribute('aria-label',rmT('Open its calls'));
  var holder=null;
  twist.addEventListener('click',function(event){
    event.stopPropagation();
    if(!holder){holder=rmFlowTree(ctx,target.data,target.own);holder.classList.add('map-flow-step-code');step.appendChild(holder);holder.hidden=true;}
    holder.hidden=!holder.hidden;twist.setAttribute('aria-expanded',String(!holder.hidden));step.classList.toggle('map-flow-step-open',!holder.hidden);
  });
  step.insertBefore(twist,step.firstChild);
}
// How a request reaches an input's handler (page data's ways): the first
// way as one chain grouped by part, a callable handed over read so on its
// hover; the other ways folded on one line, each named by the part where
// it leaves the first; then what the handler does; then who sends it.
function rmInputFlowSection(ctx,path,title,inputNode,choose){
  var ways=path.ways||[],decls=path.decls||[];
  var handlerPart=(path.parts||[]).find(function(part){return part.handler!==undefined;});
  var handler=handlerPart?decls[handlerPart.handler]:null;
  if(!ways.length&&!handler)return null;
  var section=rmEl('section','map-input-flow');
  function readName(decl,hover){
    var part=rmFlowPart(ctx,decl.part),text=rmCallableName({name:decl.name,kind:'function'});
    if(!part){var plain=rmEl('span','map-reading-name map-flow-plain',text);if(hover)plain.title=hover;return plain;}
    var key=decl.href||decl.open;
    return rmDeclName({name:decl.name,href:decl.href,open:decl.open,code:decl.code,key:key},text,function(){ctx.readDeclIn(part,key);},[part.dataset.title,hover].filter(Boolean).join('\n'));
  }
  // A chain in call order, each run in one part under its box.
  function chain(way,last){
    var line=rmEl('div','map-flow-chain'),at=null,run=null;
    var steps=way.chain.map(function(index,i){return {decl:decls[index]||{name:''},i:i};});
    if(last)steps.push({decl:last,i:-1});
    steps.forEach(function(step){
      var part=step.decl.part||'';
      if(!run||part!==at){run=rmEl('div','map-flow-group');at=part;if(part){var node=rmFlowPart(ctx,part);run.appendChild(rmPartBox(ctx,node?node.getAttribute('href')||'#'+node.id:'',''));}line.appendChild(run);}
      else run.appendChild(document.createTextNode(' → '));
      var hover='';
      if(step.i===way.hop&&way.hop>0&&(way.by||[]).length){
        var by=way.by.map(function(index){return (decls[index]||{}).name;});
        hover=by.length>1?rmT('passed as a callback by {0}, which {1} calls',by[by.length-1],by[by.length-2]):rmT('passed as a callback by {0}',by[0]);
      }
      run.appendChild(readName(step.decl,hover));
    });
    return line;
  }
  if(ways.length){
    section.appendChild(rmEl('h5','',rmT('How a request reaches {0}:',title)));
    section.appendChild(chain(ways[0],handler));
    var others=ways.slice(1),also=(path.also||[]).map(function(id){return inputNode(id);}).filter(Boolean);
    if(others.length||also.length){
      var more=rmEl('details','map-flow-other-ways'),head=rmEl('summary');head.appendChild(document.createTextNode(rmT('Other ways in:')+' '));
      others.forEach(function(way,i){
        if(i)head.appendChild(document.createTextNode(', '));
        var from=decls[way.from!==undefined?way.from:way.chain[0]]||{},node=rmFlowPart(ctx,from.part);
        head.appendChild(document.createTextNode(rmT('from')+' '));head.appendChild(rmPartBox(ctx,node?node.getAttribute('href')||'#'+node.id:'',from.name||''));
      });
      if(also.length)head.appendChild(document.createTextNode((others.length?', ':'')+'+'+also.length));
      more.appendChild(head);
      others.forEach(function(way){
        var line=chain(way,null);
        var input=way.input?inputNode(way.input):null;
        if(input){var who=rmEl('button','system-catalogue-member',input.dataset.title);who.type='button';who.addEventListener('click',function(){choose(input);});line.insertBefore(who,line.firstChild);}
        more.appendChild(line);
      });
      if(also.length){
        var runs=rmEl('p','map-flow-also');
        also.forEach(function(input,i){if(i)runs.appendChild(document.createTextNode(' '));var b=rmEl('button','system-catalogue-member',input.dataset.title);b.type='button';b.addEventListener('click',function(){choose(input);});runs.appendChild(b);});
        more.appendChild(runs);
      }
      section.appendChild(more);
    }
  }
  // What the handler does: its own flow.
  if(handler){
    var target=rmFlowOwner(ctx,null,{key:handler.href||handler.open,part:(function(){var node=rmFlowPart(ctx,handler.part);return node?node.getAttribute('href')||'#'+node.id:'';})()});
    if(target){
      var does=rmEl('div','map-flow-does');
      var headline=rmEl('div','map-flow-headline');headline.append(rmEl('h5','',rmT('What it does:')),rmFlowToggle(null));does.appendChild(headline);
      does.appendChild(rmFlowTree(ctx,target.data,target.own,3));section.appendChild(does);
    }
  }
  // Who sends it, a model's match of another program's table row: last,
  // one quiet line, the name reading that program's input.
  (path.sent_by||[]).forEach(function(peer){
    var line=rmModelText('p','map-flow-sent',''),words=rmT('{0} sends {1}.',peer.program,'\u0001').split('\u0001'),input=inputNode(peer.input);
    line.appendChild(document.createTextNode(words[0]));
    if(input){var b=rmEl('button','system-catalogue-member',peer.name);b.type='button';b.addEventListener('click',function(){choose(input);});line.appendChild(b);}else line.appendChild(document.createTextNode(peer.name));
    line.appendChild(document.createTextNode(words[1]||''));
    section.appendChild(line);
  });
  return section;
}
// </flow>
