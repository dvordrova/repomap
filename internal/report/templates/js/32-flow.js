// A declaration's flow (page_flow.go, owner-approved 2026-09-29, the
// designer's flow v2): its calls in the order they are written, each run of
// calls into one part under that part's box, drawn as the column draws a
// part (its description on hover, a click reads it). A call opens in place
// to its callee's own flow, grouped the same way; when all of those stay in
// the caller's part, no box repeats. The twist shows under the pointer or
// focus on a call that can open; a name reads its declaration and shows it
// on the canvas; a call its ancestors already make says it is shown above.
// Helper calls (the helper question's decision, into a part most of the
// program's parts call into, never the caller's own) fold under one muted
// "+ helpers" under their step only when there are more than three of
// them: three or fewer stand as rows, and so does a step whose every call
// is a helper (owner, 2026-09-29: processCommand's lookupCommand and
// queueMultiCommand had waited among eighteen names). The fold opens them
// into rows in place, and "Show helper calls" opens every step's. Names
// stand one to a line; where a call is written and what it hands over are
// on its name's hover. Go ordered every list; nothing here sorts.
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
  if(!decl.part){var plain=rmDotBreaks(rmEl('span','map-reading-name map-flow-plain',text));plain.title=title;return plain;}
  var name=rmDeclName(decl,text,ctx.goDecl(decl),title);
  var part=rmFlowPart(ctx,decl.part);
  if(part){name.addEventListener('mouseenter',function(){ctx.light([part.id]);});name.addEventListener('mouseleave',function(){ctx.light([]);});}
  return name;
}
// One call of a flow: a row that opens to its callee's flow when it has one.
// Its name is its link; no code mark follows it (owner, 2026-09-29).
function rmFlowRow(ctx,data,call,opts,helper){
  var decl=call.decl!==undefined?data.decls[call.decl]:null;
  if(call.one){
    // A dispatch site: one call, one of the declarations it can call; a
    // macro's call, the macro as written, its declarations under it, one
    // to a line.
    var site=rmEl('details','map-flow-row map-flow-dispatch'+(helper?' map-flow-helper':'')),head=rmEl('summary','map-flow-head');
    head.appendChild(rmEl('span','map-flow-twist'));
    if(call.macro)head.appendChild(rmEl('span','map-flow-plain',call.macro+(call.every?'':' ')));
    if(!call.every)head.appendChild(rmEl('span','map-flow-one',rmT('one of these')));
    head.title=rmFlowTitle(ctx,null,call);site.appendChild(head);
    site.addEventListener('toggle',function(){
      if(!site.open||site.dataset.drawn)return;site.dataset.drawn='1';
      var names=rmEl('ul','map-flow-names');
      call.one.forEach(function(at){var item=rmEl('li');item.appendChild(rmFlowName(ctx,data.decls[at],{sites:call.sites,kind:call.kind}));names.appendChild(item);});
      site.appendChild(names);
    });
    return site;
  }
  var key=decl.key||decl.href||decl.open,cycle=opts.ancestors.has(key),target=cycle?null:rmFlowOwner(ctx,data,decl),path=opts.path+'>'+(call.kind||'calls')+':'+key;
  var row=rmEl(target?'details':'div','map-flow-row'+(helper?' map-flow-helper':'')),head=rmEl(target?'summary':'div','map-flow-head');
  head.append(rmEl('span','map-flow-twist'),rmFlowName(ctx,decl,call));
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
// caller's part. More than three helper calls fold under one muted
// "+ helpers" after its rows until it or the toggle opens them in place.
// Its calls into code the report names no declaration for (a library's
// strerror, close or fork, a macro calling only such code: "assert") are
// no rows: one muted line ends the step, "also calls: strerror, close",
// each name once in the order written (owner, 2026-09-29: as rows they
// had split one part's calls under two boxes, "Replication" twice).
var rmFlowFoldAbove=3;
function rmFlowList(ctx,data,own,opts){
  var list=rmEl('div','map-flow-list'),all=own.flow||[];
  var outside=all.filter(function(call){return call.decl===undefined&&!call.one;}),calls=all.filter(function(call){return outside.indexOf(call)<0;});
  var marked=calls.filter(function(call){return call.helper;}),fold=marked.length>rmFlowFoldAbove&&marked.length<calls.length;
  var work=fold?calls.filter(function(call){return !call.helper;}):calls,helpers=fold?marked:[];
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
      group.appendChild(rmFlowRow(ctx,data,call,opts,call.helper&&fold));
    });
    if(helpers.length&&!rmFlowHelpers)list.appendChild(helperLine(opened));
    if(outside.length)list.appendChild(alsoLine());
  }
  function alsoLine(){
    var line=rmEl('p','map-flow-also'),names=[];line.appendChild(rmEl('span','map-flow-also-label',rmT('also calls:')));
    outside.forEach(function(call){
      var name=call.macro||call.name||'';if(!name||names.indexOf(name)>=0)return;
      line.appendChild(document.createTextNode(names.length?', ':' '));names.push(name);
      var said=rmDotBreaks(rmEl('span','map-flow-plain',name));said.title=[call.lib,call.macro?rmT('a macro'):''].filter(Boolean).join('\n');line.appendChild(said);
    });
    return line;
  }
  function helperLine(opened){
    // The step's helper calls, folded: "+ helpers" opens them into rows in
    // place, "− helpers" folds them.
    var line=rmEl('p','map-flow-helpers'),more=rmEl('button','map-flow-helpers-toggle',opened?rmT('− helpers'):rmT('+ helpers'));
    more.type='button';more.setAttribute('aria-expanded',String(opened));
    more.addEventListener('click',function(event){event.stopPropagation();if(opts.open.has(key))opts.open.delete(key);else opts.open.add(key);draw();});
    line.appendChild(more);
    return line;
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
      more.appendChild(head);
      others.forEach(function(way){
        var line=chain(way,null);
        var input=way.input?inputNode(way.input):null;
        if(input){var who=rmEl('button','system-catalogue-member',input.dataset.title);who.type='button';who.addEventListener('click',function(){choose(input);});line.insertBefore(who,line.firstChild);}
        more.appendChild(line);
      });
      // The inputs that run the site themselves, one to a line.
      if(also.length){
        var runs=rmEl('ul','map-reading-ends map-flow-also-inputs');
        also.forEach(function(input){var item=rmEl('li'),b=rmEl('button','system-catalogue-member',input.dataset.title);b.type='button';b.addEventListener('click',function(){choose(input);});item.appendChild(b);runs.appendChild(item);});
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
