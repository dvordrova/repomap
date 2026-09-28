// One repository drawing. Selection changes emphasis and reading, never layout.
// All IDs, containment and operation paths come from the rendered report.
function rmSystemProjection(nodes, edges) {
  var byID={},parents={},inputOwner={};
  nodes.forEach(function(n){byID[n.id]=n;});
  nodes.forEach(function(n){if(n.inputOwner&&byID[n.inputOwner])inputOwner[n.id]=n.inputOwner;});
  nodes.forEach(function(n){(n.children||[]).forEach(function(id){parents[id]=n.id;});});
  edges.forEach(function(e){if(byID[e.from]?.activation && e.label==='implemented in' && byID[e.to] && !byID[e.to].activation)inputOwner[e.from]=e.to;});
  function leaves(id,seen){seen=seen||new Set();if(seen.has(id)||!byID[id])return [];seen.add(id);var n=byID[id];return n.children?.length?n.children.flatMap(function(c){return leaves(c,new Set(seen));}):[id];}
  var visible=nodes.filter(function(n){return !n.children?.length;}).map(function(n){return n.id;});
  var representatives={};nodes.forEach(function(n){representatives[n.id]=[n.id];});
  // Only the input catalogue contains input cards. Implementation and original
  // ownership remain available independently for breadcrumbs and reading.
  var areas=nodes.filter(function(n){return n.children?.length;}).map(function(n){return {id:n.id,nodes:n.children.filter(function(id){return n.branch==='inputs'||!byID[id]?.activation;})};});
  function selection(id,operation){
    var selected=new Set(leaves(id));if(byID[id])selected.add(id);var active=new Set(operation?[]:selected),path=[];
    if(operation){
      path=edges.filter(function(e){return (e.operations||[]).includes(operation);});
      path.forEach(function(e){active.add(e.from);active.add(e.to);});
      active.add(operation);
    }else if(id){edges.forEach(function(e){if(selected.has(e.from)||selected.has(e.to)){active.add(e.from);active.add(e.to);}});}
    return {selected:selected,active:active,path:path,entry:operation||''};
  }
  // Whether a thing read while an input is pinned stands off that input's
  // path: neither it nor a part inside it is an end of the path's arrows.
  // It is a fact of the reading, not of what the pointer is over.
  function outside(id,operation){
    if(!id||!operation||id===operation)return false;
    var active=selection('',operation).active;
    return !active.has(id)&&!leaves(id).some(function(leaf){return active.has(leaf);});
  }
  return {visible:visible,areas:areas,representatives:representatives,parents:parents,inputOwner:inputOwner,leaves:leaves,selection:selection,outside:outside};
}
// An input chosen from Find, a link, a reading or its own tile is entered as
// its path: the canvas frames the part holding its handler and the path's
// parts nearest it, the trace dark from there, while the reading column
// reads the input. Framing its tile showed a wall of inputs and no route: the
// collection stands outside its component, and no one camera shows the tile
// and the parts it reaches readably. The path is the drawn parts of its
// saved trace; an input without one is entered as its tile.
function rmInputPath(n,byID){
  if(!n?.dataset?.activation)return [];
  return (n.dataset.inputTrace||'').split(/\s+/).filter(function(id){return byID[id]&&!byID[id].dataset.activation;});
}
// The map's one key: each kind of card as a small card in the fill and
// border the canvas paints it with, its mark on its border, then the
// strokes arrows are drawn with: calls, possible calls, and the purple
// dashed links between a part's declarations from a function to the type it
// returns or from a type to the function taking it, which Redis's readers
// met in tiles with nothing saying what they were. Its glyphs had been
// painted in the marks' dark colours, so a pale green card matched nothing
// in it. A kind the map does not draw is not keyed. The line saying what the
// numbers on a frame's border are stands above it, beside the map's
// controls, where the row had room; it had been folded into a legend under
// the map.
function rmKey(nodes,edges,category){
  var key=rmEl('span','flow-color-key');
  [['','Parts'],['entry','Entrypoints'],['core','Core'],['input','Inputs'],['external','External communication']].filter(function(item){return nodes.some(function(n){return ['core','entry'].includes(item[0])?!n.dataset.branch&&!n.dataset.activation&&n.dataset.lane===(item[0]==='entry'?'triggers':'core'):item[0]?category(n)===item[0]:!n.dataset.branch&&category(n)==='part'&&!['core','triggers'].includes(n.dataset.lane);});}).forEach(function(item){var label=rmEl('span',item[0]);label.append(rmEl('i'),document.createTextNode(rmT(item[1])));key.appendChild(label);});
  var types=nodes.some(function(n){try{return (rmPage.data(n,'symbolCalls')||[]).some(function(link){return link[2]==='returns'||link[2]==='takes';});}catch(_){return false;}});
  [[false,'calls',edges.some(function(e){return !e.possible;})],[true,'possible calls',edges.some(function(e){return e.possible;})],['types','returns or takes a type',types]].filter(function(item){return item[2];}).forEach(function(item){
    var label=rmEl('span','flow-key-stroke'+(item[0]==='types'?' flow-key-types':item[0]?' flow-key-possible':'')),svg=document.createElementNS('http://www.w3.org/2000/svg','svg');
    svg.setAttribute('viewBox','0 0 30 10');svg.setAttribute('width','30');svg.setAttribute('height','10');svg.setAttribute('aria-hidden','true');
    svg.innerHTML='<path d="M1 5H24"/><path d="M23 1.5 29 5 23 8.5z"/>';
    label.append(svg,document.createTextNode(rmT(item[1])));key.appendChild(label);
  });
  return key;
}
// The inputs that reach a part, by kind, folded under their count: most
// commands reach most parts through the one dispatcher, and Client
// connections' list of 95 stood open between the part's callers and its
// own connections.
function rmReachingInputs(n,reaching,owner,choose){
  var inputs=rmEl('details','system-reaching-inputs');inputs.appendChild(rmEl('summary','',rmT(n.dataset.itemKind==='External communication'?'Inputs reaching this communication':'Inputs reaching this part')+' · '+reaching.length));
  if(reaching.length){
    var types=new Map();reaching.forEach(function(input){var type=input.dataset.activation;if(!types.has(type))types.set(type,[]);types.get(type).push(input);});
    types.forEach(function(choices,type){inputs.appendChild(rmEl('h6','',rmT(({request:'Incoming requests',command:'Commands',setting:'Settings',interaction:'User interactions',scheduled:'Scheduled tasks',continuous:'Background work'})[type]||'Inputs')));var links=rmEl('div','system-neighbours');choices.forEach(function(input){var b=rmEl('button','',owner(input)+' / '+input.dataset.title);b.type='button';b.addEventListener('click',function(){choose(input);});links.appendChild(b);});inputs.appendChild(links);});
  }else inputs.appendChild(rmEl('p','meta',rmT('No input path to this item is recorded.')));
  return inputs;
}
// An input's reading of its saved reach (page_input_path.go; owner's 3c).
// First each dispatch site that dispatches it, the first open: "Dispatched
// from call · one of 94", saying that how the input reaches the site is not
// established. The inputs whose own code reaches the site are the site's
// reading, never this one's: a benchmark reader had taken a route to call
// for GET's path. Then the sites its own code reaches, with its calls to
// them; the inputs it registers or is registered by; then the parts it
// enters, nearest the handler first, each with every call entering it from
// a part reached earlier (five, the rest folded under their count) and a
// count of the other calls into it, the parts past the handler's own calls
// folded under one line. No route is chosen and no line number is written. A name in a drawn part is read there (`read(part,key)`), as
// its tile is; a modifier-click opens its code.
function rmInputPathSection(path,title,partNode,inputNode,choose,read){
  var section=rmEl('section','system-input-path'),decls=path.decls||[];
  section.appendChild(rmEl('h5','',rmT('Path')));
  function name(index){
    var decl=decls[index]||{name:''},key=decl.href||decl.open,at=key&&partNode(decl.part);
    var link=at?repomapMembers.sourceLink({Href:decl.code||decl.href,Open:decl.open,Text:decl.name,NoSource:decl.no_source}):rmEl('span','',decl.name);
    if(decl.source)link.title=decl.source;
    if(at)link.addEventListener('click',function(event){
      if(event.button||event.metaKey||event.ctrlKey||event.shiftKey||event.altKey)return;
      event.preventDefault();event.stopPropagation();read(at,key);
    });
    return link;
  }
  function call(entry){
    var line=rmEl('div','system-path-step');
    line.append(name(entry[0]),document.createTextNode(' → '),name(entry[1]));
    if(entry[2]&4)line.appendChild(rmEl('span','possible',' · '+rmT('possible integration')));
    else if(entry[2]&2)line.appendChild(rmEl('span','possible',' · '+rmT('read')));
    else if(entry[2]&1)line.appendChild(rmEl('span','possible',' · '+rmT('possible')));
    return line;
  }
  function calls(list,box){
    list.slice(0,5).forEach(function(entry){box.appendChild(call(entry));});
    if(list.length>5){
      var more=rmEl('details','system-path-more');more.appendChild(rmEl('summary','','+'+(list.length-5)));
      list.slice(5).forEach(function(entry){more.appendChild(call(entry));});box.appendChild(more);
    }
  }
  function inputs(ids,label){
    var box=rmEl('div','system-neighbours');box.appendChild(rmEl('span','meta',label));
    ids.forEach(function(id){
      var input=inputNode(id);if(!input)return;
      var button=rmEl('button','',input.dataset.title);button.type='button';button.addEventListener('click',function(){choose(input);});box.appendChild(button);
    });
    return box;
  }
  (path.dispatched||[]).forEach(function(site,index){
    var box=rmEl('details','system-shared-path'),site_name=(decls[site.site]||{}).name||'';box.open=index===0;
    box.appendChild(rmEl('summary','',rmT('Dispatched from {0}',site_name)+' · '+rmSiteHandlers(site)));
    // "one of 94 handlers" reads the dispatcher, whose reading lists them
    // by input with a filter.
    var at=rmEl('p','meta'),handlers=rmEl('button','system-path-handlers',rmSiteHandlers(site)),siteDecl=decls[site.site]||{},sitePart=partNode(siteDecl.part);handlers.type='button';
    if(sitePart)handlers.addEventListener('click',function(){read(sitePart,siteDecl.href||siteDecl.open);});
    at.append(name(site.site),document.createTextNode(' → '),handlers);box.appendChild(at);
    // How many inputs are dispatched there and which handler serves
    // several of them, where a reader compares the counts.
    at.title=[rmT('{0} inputs are dispatched here',site.inputs)].concat(rmSharedHandlers(site,decls,function(id){return inputNode(id)?.dataset.title||'';})).join('\n');
    // The outer inputs a request dispatched here arrives from (u6): each
    // listed, none chosen; through a callable one registers, that hop is
    // named. Without any, how it gets there is not established.
    (site.outer||[]).forEach(function(outer){
      // One closed fold per outer input: its name and the callables it
      // registers on the line, its route and each hop inside.
      var input=inputNode(outer.input),fold=rmEl('details','system-path-outer'),head=rmEl('summary');
      var who=input?rmEl('button','system-catalogue-member',input.dataset.title):rmEl('span','',outer.input);
      if(input){who.type='button';who.addEventListener('click',function(event){event.preventDefault();choose(input);});}
      var text=rmT('A request for {0} arrives at {1} from {2}:',title,site_name,'\u0001').split('\u0001');
      head.append(document.createTextNode(text[0]),who,document.createTextNode((text[1]||'').replace(/:\s*$/,'')));
      var registered=(outer.hops||[]).map(function(hop){return (decls[hop.registers]||{}).name||'';}).filter(Boolean);
      if(registered.length)head.append(document.createTextNode(' · '+rmT('registers {0}',registered.join(', '))));
      fold.appendChild(head);
      calls(outer.calls||[],fold);
      (outer.hops||[]).forEach(function(hop){
        var reg=rmEl('p','system-path-step');var parts=rmT('{0} registers {1}','\u0001','\u0002').split(/[\u0001\u0002]/);
        reg.append(document.createTextNode(parts[0]),document.createTextNode(input?input.dataset.title:''),document.createTextNode(parts[1]),name(hop.registers),document.createTextNode(parts[2]||''));
        fold.appendChild(reg);
        if((hop.registering||[]).length){var how=rmEl('details','system-path-more');how.appendChild(rmEl('summary','',rmT('How it registers it')));calls(hop.registering,how);fold.appendChild(how);}
        calls(hop.calls||[],fold);
      });
      box.appendChild(fold);
    });
    if(!(site.outer||[]).length)box.appendChild(rmEl('p','meta',rmT('How a request for {0} gets to {1} is not established.',title,site_name)));
    else if(site.unexplained)box.appendChild(rmEl('p','meta',rmT('Other ways to {0} are not established.',site_name)));
    section.appendChild(box);
  });
  (path.reaches||[]).forEach(function(site){
    var box=rmEl('div','system-path-reaches');
    box.appendChild(rmEl('h6','',rmT("{0}'s handler itself calls {1}, where {2} inputs are dispatched:",title,(decls[site.site]||{}).name||'',site.inputs)));
    calls(site.calls||[],box);section.appendChild(box);
  });
  if((path.checks||[]).length){
    // Words only the handler's own code checks: the input's sub-arguments.
    var checks=rmEl('p','system-path-checks');checks.appendChild(rmEl('span','meta',rmT(path.values?'Its values':'Words its handler checks')+': '));
    path.checks.forEach(function(check,i){if(i)checks.append(document.createTextNode(', '));var link=check.href||check.open?repomapMembers.sourceLink({Href:check.href,Open:check.open,Text:check.name,NoSource:check.no_source}):rmEl('span','',check.name);if(check.source)link.title=check.source;checks.appendChild(link);});
    section.appendChild(checks);
  }
  // A model match between a table row of one program and an input of
  // another: named, never drawn.
  function peer(entry,key){
    var line=rmEl('p','system-path-peer'),input=inputNode(entry.input);
    line.appendChild(document.createTextNode(rmT(key,entry.program)+' '));
    if(input){var b=rmEl('button','system-catalogue-member',entry.name);b.type='button';b.addEventListener('click',function(){choose(input);});line.appendChild(b);}
    else line.appendChild(rmEl('span','',entry.name));
    if(entry.in)line.appendChild(document.createTextNode(' ('+entry.in+')'));
    line.appendChild(rmEl('span','possible',' · '+rmT('model match')));
    return line;
  }
  (path.sent_to||[]).forEach(function(entry){section.appendChild(peer(entry,'Sent to {0} as'));});
  (path.sent_by||[]).forEach(function(entry){section.appendChild(peer(entry,'Sent by {0}:'));});
  if((path.registered_by||[]).length)section.appendChild(inputs(path.registered_by,rmT('Registered by')));
  if((path.registers||[]).length)section.appendChild(inputs(path.registers,rmT('Registers')));
  // The part holding the handler names it; the parts its handler calls
  // directly (depth 1) stand open, and the parts reached deeper are folded
  // under one line that opens them as they are. The fold is by depth alone,
  // the handler's own calls against the rest: it chooses no route and
  // drops no call. GET had listed thirteen parts down to VM swap-in's
  // rdbLoadObject → zslInsert.
  var parts=rmEl('div','system-path-steps'),all=path.parts||[];
  function step(part,into){
    var node=partNode(part.part),head=rmEl(node?'button':'div','system-path-part',node?node.dataset.title:part.title||'');
    if(node){head.type='button';head.addEventListener('click',function(){choose(node);});}
    into.appendChild(head);
    if(part.handler!=null){var own=rmEl('div','system-path-step');own.append(rmEl('span','meta',rmT('handled by')+' '),name(part.handler));into.appendChild(own);}
    calls(part.entered||[],into);
    if(part.others)into.appendChild(rmEl('p','meta',rmT('{0} more calls into this part come from other code on this path',part.others)));
  }
  var deep=all.filter(function(part){return part.depth>1;});
  all.filter(function(part){return !(part.depth>1);}).forEach(function(part){step(part,parts);});
  if(deep.length){
    var deeper=rmEl('details','system-path-deeper');deeper.appendChild(rmEl('summary','',rmT('Reaches {0} more parts deeper',deep.length)));
    deep.forEach(function(part){step(part,deeper);});parts.appendChild(deeper);
  }
  if(parts.childElementCount)section.appendChild(parts);
  return section;
}
// <catalogue>
// A catalogue's reading (page_catalogue.go), shared by its members: where
// they are declared and where that code is called from, what else it uses
// and who else uses that, then once that where these inputs take effect is
// not established. "Uses" is never read as "takes effect".
function rmCatalogueSection(catalogue,title,inputNode,choose,read,partNode){
  var section=rmEl('section','system-catalogue'),decls=catalogue.decls||[];
  function name(index){
    var decl=decls[index]||{name:''},key=decl.href||decl.open,at=key&&partNode(decl.part);
    var link=key?repomapMembers.sourceLink({Href:decl.code||decl.href,Open:decl.open,Text:decl.name,NoSource:decl.no_source}):rmEl('span','',decl.name);
    if(decl.source)link.title=decl.source;
    if(at)link.addEventListener('click',function(event){
      if(event.button||event.metaKey||event.ctrlKey||event.shiftKey||event.altKey)return;
      event.preventDefault();event.stopPropagation();read(at,key);
    });
    return link;
  }
  // A translated line with nodes in its {n} places.
  function line(cls,key){
    var args=Array.prototype.slice.call(arguments,2),marks=args.map(function(_,i){return '\u0001'+i+'\u0002';});
    var text=rmT.apply(null,[key].concat(marks)),p=rmEl('p',cls);
    text.split(/(\u0001\d+\u0002)/).forEach(function(piece){
      var m=/^\u0001(\d+)\u0002$/.exec(piece);
      if(m){var arg=args[Number(m[1])];p.append(arg instanceof Node?arg:document.createTextNode(String(arg)));}
      else if(piece)p.append(document.createTextNode(piece));
    });
    return p;
  }
  function inline(key){return Array.from(line.apply(null,['',key].concat(Array.prototype.slice.call(arguments,1))).childNodes);}
  var head=rmEl('p','system-catalogue-declared');
  // The object they are declared on: the input declared at its call, when
  // there is one, else that call as written.
  var onInput=catalogue.on_input&&inputNode(catalogue.on_input),on=null;
  if(onInput){on=rmEl('button','system-catalogue-member',onInput.dataset.title);on.type='button';on.addEventListener('click',function(){choose(onInput);});}
  else if(catalogue.on!=null)on=name(catalogue.on);
  if(on&&catalogue.declarer>=0)head.append.apply(head,inline('Declared on {0} in {1}',on,name(catalogue.declarer)));
  else if(on)head.append.apply(head,inline('Declared on {0}',on));
  else if(catalogue.declarer>=0)head.append.apply(head,inline(catalogue.table?'In {0}':'Declared in {0}',name(catalogue.declarer)));
  var members=catalogue.members||[];
  if(members.length>=2){
    var of=({command:'one of {0} commands',request:'one of {0} requests',setting:'one of {0} settings'})[catalogue.kind]||'one of {0} inputs';
    head.append(document.createTextNode(' · '+rmT(of,members.length)+': '));
    members.forEach(function(id,i){
      var input=inputNode(id);if(!input)return;
      if(i)head.append(document.createTextNode(' '));
      if(input.dataset.title===title){head.append(rmEl('b','',input.dataset.title));return;}
      var b=rmEl('button','system-catalogue-member',input.dataset.title);b.type='button';b.addEventListener('click',function(){choose(input);});head.append(b);
    });
  }
  function callers(calls){
    var from=document.createElement('span');
    calls.forEach(function(call,i){
      if(i)from.append(document.createTextNode(', '));
      from.append(name(call.caller));
      if(call.line){var at=call.href?repomapMembers.sourceLink({Href:call.href,Open:call.open,Text:':'+call.line}):rmEl('span','',':'+call.line);from.append(document.createTextNode(' '),at);}
      if(call.possible)from.append(rmEl('span','possible',' · '+rmT('possible')));
    });
    return from;
  }
  if((catalogue.calls||[]).length){head.append(document.createTextNode(' · '));head.append.apply(head,inline('called from {0}',callers(catalogue.calls)));}
  // A table's rows are looked up where the table is read.
  (catalogue.readers||[]).forEach(function(reader){
    head.append(document.createTextNode(' · '));head.append.apply(head,inline('looked up in {0}',name(reader.reader)));
    if((reader.calls||[]).length){head.append(document.createTextNode(', '));head.append.apply(head,inline('called from {0}',callers(reader.calls)));}
  });
  section.appendChild(head);
  if(onInput&&catalogue.on_handler!=null)section.appendChild(line('system-catalogue-handled','{0} is handled by {1}',onInput.dataset.title,name(catalogue.on_handler)));
  (catalogue.uses||[]).forEach(function(use){
    if(catalogue.declarer<0)return;
    var box=rmEl('div','system-catalogue-uses');
    var p=line('system-catalogue-use','{0} also uses {1}',name(catalogue.declarer),name(use.decl));
    box.appendChild(p);
    if((use.users||[]).length){
      var more=rmEl('details','system-catalogue-users');
      var summary=rmEl('summary');
      summary.textContent=rmT('{0} is also used by',(decls[use.decl]||{}).name||'')+' ('+rmT('{0} functions in {1} parts',use.users.length,use.parts||0)+')';
      more.appendChild(summary);
      var byPart=new Map();use.users.forEach(function(user){var part=(decls[user]||{}).part||'';if(!byPart.has(part))byPart.set(part,[]);byPart.get(part).push(user);});
      byPart.forEach(function(users,part){
        var row=rmEl('div','system-neighbours'),node=partNode(part);
        if(node){var b=rmEl('button','',node.dataset.title);b.type='button';b.addEventListener('click',function(){choose(node);});row.appendChild(b);row.append(document.createTextNode(': '));}
        users.forEach(function(user,i){if(i)row.append(document.createTextNode(', '));row.append(name(user));});
        more.appendChild(row);
      });
      box.appendChild(more);
    }
    section.appendChild(box);
  });
  section.appendChild(rmEl('p','meta',rmT('Where these take effect is not established.')));
  return section;
}
// </catalogue>
// <launch>
// The Inputs reading's fold "How these were found" (page_launch.go): the
// launch functions that hold inputs, each by its chain of calls from where
// the program starts; each symbol's idiom line, a model answer; the calls
// that may declare an input and were not decided; the calls the code cannot
// follow; and the other functions the launch reaches, counted only.
function rmLaunchSection(launch,inputNode,choose,read,partNode){
  var box=rmEl('details','system-launch'),decls=launch.decls||[];
  box.appendChild(rmEl('summary','',rmT('How these were found')));
  function name(index){
    var decl=decls[index]||{name:''},key=decl.href||decl.open,at=key&&partNode(decl.part);
    var link=key?repomapMembers.sourceLink({Href:decl.code||decl.href,Open:decl.open,Text:decl.name,NoSource:decl.no_source}):rmEl('span','',decl.name);
    if(decl.source)link.title=decl.source;
    if(at)link.addEventListener('click',function(event){
      if(event.button||event.metaKey||event.ctrlKey||event.shiftKey||event.altKey)return;
      event.preventDefault();event.stopPropagation();read(at,key);
    });
    return link;
  }
  function site(line,href,open){return href||open?repomapMembers.sourceLink({Href:href,Open:open,Text:':'+line}):rmEl('span','',':'+line);}
  (launch.found||[]).forEach(function(found){
    var row=rmEl('div','system-path-step');
    found.chain.forEach(function(index,i){if(i)row.append(document.createTextNode(' → '));row.append(name(index));});
    row.append(document.createTextNode(' ('+found.inputs.length+') '));
    found.inputs.slice(0,8).forEach(function(id){var input=inputNode(id);if(!input)return;var b=rmEl('button','system-catalogue-member',input.dataset.title);b.type='button';b.addEventListener('click',function(){choose(input);});row.append(b,document.createTextNode(' '));});
    if(found.inputs.length>8)row.append(document.createTextNode('+'+(found.inputs.length-8)));
    box.appendChild(row);
  });
  (launch.idioms||[]).forEach(function(idiom){
    var row=rmEl('div','system-path-step system-launch-idiom');
    var functions=document.createElement('span');(idiom.functions||[]).forEach(function(index,i){if(i)functions.append(document.createTextNode(', '));functions.append(name(index));});
    row.append(document.createTextNode(rmT('{0}: {1} of {2} word calls declare inputs ({3}), in',idiom.symbol,idiom.entries,idiom.calls,rmT(idiom.kind))+' '),functions);
    row.appendChild(rmEl('span','possible',' · '+rmT('model')));
    box.appendChild(row);
  });
  if((launch.unsure||[]).length){
    box.appendChild(rmEl('h6','',rmT('Unsure')));
    // One line per symbol and reason, its calls folded under it: every
    // call listed, none dropped.
    var groups=new Map();
    launch.unsure.forEach(function(call){var key=call.symbol+'\u0000'+call.reason;if(!groups.has(key))groups.set(key,[]);groups.get(key).push(call);});
    groups.forEach(function(calls){
      var first=calls[0],text=rmT(first.reason==='no_words'?'calls {0} with words the code computes':'calls {0} with words; whether they are inputs is not decided',first.symbol);
      if(calls.length===1){
        var row=rmEl('div','system-path-step');row.append(name(first.function),document.createTextNode(' '+text+' '),site(first.line,first.href,first.open));box.appendChild(row);return;
      }
      var fold=rmEl('details','system-path-more');fold.appendChild(rmEl('summary','',first.symbol+' · '+rmT('{0} calls',calls.length)+' · '+text));
      calls.forEach(function(call){var row=rmEl('div','system-path-step');row.append(name(call.function),document.createTextNode(' '),site(call.line,call.href,call.open));fold.appendChild(row);});
      box.appendChild(fold);
    });
  }
  if((launch.closed||[]).length){
    box.appendChild(rmEl('h6','',rmT('Could not look inside')));
    launch.closed.forEach(function(closed){
      var row=rmEl('div','system-path-step');
      row.append(name(closed.function),document.createTextNode(' · '+rmT('{0} calls the code cannot follow',closed.sites.length)+' '));
      closed.sites.slice(0,5).forEach(function(at){if(at.line)row.append(site(at.line,at.href,at.open),document.createTextNode(' '));});
      box.appendChild(row);
    });
  }
  if(launch.nothing)box.appendChild(rmEl('p','meta',rmT('{0} more functions the launch reaches declare none',launch.nothing)));
  return box;
}
// </launch>
// An input's reading leads to its program's Main flow, the model's reading
// of a request through the program, with the title the model wrote kept as
// model text: a reader had found that page only by chance. Null when the
// component has no flow section.
function rmInputFlow(page){
  var flow=page&&page.querySelector(':scope>.component-flow'),heading=flow&&flow.querySelector(':scope>h3[id]');
  if(!heading)return null;
  var box=rmEl('div','system-input-flow'),link=rmEl('a','map-details-link',heading.textContent);link.href='#'+heading.id;box.appendChild(link);
  var title=flow.querySelector(':scope>.flow-title');if(title)box.appendChild(title.cloneNode(true));
  return box;
}
// Where the reading is, as the toolbar's breadcrumb names it: the pinned
// input, the frames from the component down to what is read, and the
// declaration named in it. Each segment is a level to go back up to; it had
// been one link naming them all that only re-read the current reading, and
// Redis's readers clicked "Server runtime" in it and stayed on syncCommand's
// tiles. `sep` is what stands before a segment, as the one label read.
function rmExplorationPath(operation,frames,member){
  var segments=[];
  if(operation)segments.push({id:operation.id,title:operation.dataset.title,kind:'input'});
  frames.forEach(function(n,i){segments.push({id:n.id,title:n.dataset.title,kind:'frame',sep:i?' / ':' · '});});
  if(member&&member.name&&frames.length)segments.push({id:frames[frames.length-1].id,title:member.name,kind:'member',sep:' · ',source:{key:member.key,href:member.href,open:member.open}});
  return segments;
}
// Where a component's "Entrypoints" link lands (page_sections.go): the part
// holding the program's seed, the seed read there, or, when no part holds
// it, the component, read at its entry line. It had landed on the inputs.
// Null leaves the link to the page it names.
function rmEntryLanding(link,nodes,component){
  var part=link.dataset.entryPart;
  if(part){
    var node=nodes.find(function(n){return n.getAttribute('href')==='#'+part;});
    if(node)return {node:node,source:link.dataset.entrySource?{key:link.dataset.entrySource}:null,entry:false};
  }
  var owner=component();
  return owner?{node:owner,source:null,entry:true}:null;
}
// An input's row in a component's catalogue names the input by where it is
// registered and its handler by its code, and both opened GitHub on a plain
// click: a reader of Redis's 95 commands meant to read flushdb, not its
// line in redis.c. When the row's own link to the input (its "To
// explanation", or its title's anchor) names one input, a plain click on
// the name or the handler reads that input in the report; a modifier-click
// still opens the code. Nothing is matched by name.
function rmCatalogInputClick(event,reveal){
  if(event.button||event.ctrlKey||event.metaKey||event.shiftKey||event.altKey)return false;
  var name=event.target.closest&&event.target.closest('.route-path,.route-symbol,.input-handler a');
  var row=name&&name.closest('.input-catalog [data-input-item]');if(!row)return false;
  var ids=new Set(Array.from(row.querySelectorAll('a.input-explanation[href^="#"],.input-title>a[href^="#"]:not(.route-path)')).map(function(a){return a.getAttribute('href').slice(1);}).filter(Boolean));
  if(ids.size!==1||!reveal(Array.from(ids)[0]))return false;
  event.preventDefault();event.stopImmediatePropagation();return true;
}
(function(){document.querySelectorAll('[data-map-explorer]').forEach(function(map){
  var svg=map.querySelector('svg'),stage=map.querySelector('[data-map-stage]');
  var nodes=Array.from(map.querySelectorAll('[data-node]')),byID={},aliases={};
  nodes.forEach(function(n){byID[n.id]=n;});
  map.querySelectorAll('[data-map-alias]').forEach(function(n){aliases[n.id]=n.dataset.mapAlias;});
  var rawEdges=Array.from(svg.querySelectorAll('.map-edge')).filter(function(e){return e.dataset.scope!=='static';}).map(function(e){return {from:e.dataset.from,to:e.dataset.to,scope:e.dataset.scope,summary:e.dataset.summary,summaryRef:e.dataset.summaryRef,labelRef:e.dataset.labelRef,fromSource:e.dataset.fromSource,fromText:e.dataset.fromText,fromNoSource:e.dataset.fromNoSource==='true',toSource:e.dataset.toSource,toText:e.dataset.toText,toNoSource:e.dataset.toNoSource==='true',operations:(e.dataset.operations||'').split(/\s+/).filter(Boolean),possible:e.classList.contains('map-edge-possible'),init:e.classList.contains('map-edge-init'),calls:(rmPage.data(e,'calls')||[]).map(function(call){if(!('callee' in call))call.callee=call.to||'';return call;}),label:e.dataset.label||''};});
  var model=nodes.map(function(n){return {id:n.id,branch:n.dataset.branch,children:(n.dataset.children||'').split(/\s+/).filter(Boolean),activation:n.dataset.activation,inputOwner:n.dataset.inputOwner};});
  var projection=rmSystemProjection(model,rawEdges),scope='',operation=null,surface=null,ready=null;
  // Whether the camera may stand away from the pinned input's own tile: on its
  // path's start, or on a part read since.
  var inputAway=false;
  // What was read, and where the camera stood, before an input's path was
  // entered: "Leave input path" returns there (owner, 2026-09-28), as Back
  // would. It had gone to the repository's summary, and a reader needed
  // the Components list to get back to redis-server.
  var beforeInput=null;
  var searchValue='',filterValue='',selectionRevision=0;
  map.querySelector('[data-operation-controls]')?.remove();
  var bar=rmEl('div','system-controls'),search=rmEl('input','system-search'),filter=rmEl('select','system-filter'),clear=rmEl('button','',rmT('Clear selection'));
  search.type='search';search.placeholder=rmT('Find');search.setAttribute('aria-label',rmT('Find'));clear.type='button';
  [['','Everything'],['component','Components'],['part','Parts'],['input','Inputs'],['external','External communication']].forEach(function(item){var option=rmEl('option','',rmT(item[1]));option.value=item[0];filter.appendChild(option);});filter.setAttribute('aria-label',rmT('Show on map'));
  bar.append(search);map.prepend(bar);bar.appendChild(map.querySelector('[data-map-controls]'));
  if(map.hasAttribute('data-system-map'))search.hidden=true;
  var results=rmEl('div','system-results');results.hidden=true;bar.after(results);
  var colorKey=rmKey(nodes,rawEdges,category);
  var caption=rmEl('div','system-selection');caption.setAttribute('aria-live','polite');results.after(caption);
  var measureControls=new ResizeObserver(function(){map.style.setProperty('--system-controls-height',(bar.offsetHeight+results.offsetHeight+caption.offsetHeight+24)+'px');});
  [bar,results,caption].forEach(function(n){measureControls.observe(n);});
  function kind(n){return n.dataset.itemKind||(n.dataset.activation?'Inputs':n.dataset.branch==='component'?'Component':n.dataset.branch?'Area':n.dataset.lane==='core'?'Core':n.dataset.lane==='dependencies'?'Code dependencies':n.dataset.lane==='triggers'?'Entrypoints':'Part');}
  map.itemKind=kind;
  function category(n){return n.dataset.activation||n.dataset.branch==='inputs'?'input':n.dataset.itemKind==='External communication'?'external':n.dataset.branch==='component'||n.dataset.itemKind==='Component'?'component':'part';}
  function owner(n){return byID['system-component-'+n.dataset.owner]?.dataset.title||document.getElementById(n.dataset.owner)?.dataset.componentName||'';}
  function emit(){map.dispatchEvent(new Event('repomap:reading'));}
  function address(n,newVisit){document.dispatchEvent(new CustomEvent('repomap:navigate',{detail:{destination:n||document.getElementById('overview'),newVisit:!!newVisit}}));}
  function path(id){var result=[],seen=new Set();while(id&&!seen.has(id)){seen.add(id);result.unshift(id);id=projection.parents[id];}return result;}
  function matches(n){return (!filterValue||category(n)===filterValue)&&(!searchValue||(path(n.id).map(function(id){return byID[id].dataset.title;}).join(' ')+' '+n.dataset.summary+' '+owner(n)).toLowerCase().includes(searchValue.toLowerCase()));}
  function updateResults(){
    results.replaceChildren();results.hidden=!searchValue&&!filterValue;
    if(results.hidden)return;
    nodes.filter(matches).forEach(function(n){var b=rmEl('button','',n.dataset.title);b.type='button';b.dataset.selectNode=n.id;b.appendChild(rmEl('small','',owner(n)+' · '+rmT(kind(n))));b.addEventListener('click',function(){select(n,true,null,true).then(function(selected){if(selected)rmScrollToReading(map);});});results.appendChild(b);});
    if(!results.childElementCount)results.appendChild(rmEl('p','',rmT('No matches.')));
  }
  function emphasize(){
    var state=projection.selection(scope,operation?.id),has=!!scope||!!operation||!!searchValue||!!filterValue;
    var matched=new Set();if(searchValue||filterValue)nodes.filter(matches).forEach(function(n){projection.leaves(n.id).forEach(function(id){matched.add(id);});});
    surface?.update({scope:scope,operation:operation?.id||'',entry:state.entry,selected:state.selected,matched:matched,searching:!!searchValue||!!filterValue});
    map.classList.toggle('system-has-selection',has);clear.disabled=!scope&&!operation;
    renderCaption();
    map.explorerScope=scope;map.explorerOperation=operation;map.inspectedOperation=operation;map.dataset.operationPinned=operation?'true':'false';
  }
  function renderCaption(){
    caption.replaceChildren();
    var inspector=map.querySelector('.map-inspector'),controls=map.querySelector('.map-input-context');
    if(inspector&&!controls){controls=rmEl('div','map-input-context');inspector.prepend(controls);}
    if(controls){controls.replaceChildren();controls.hidden=!operation;}
    if(operation){
      var context=rmEl('span','system-reading-context',rmT('Input')+': '+operation.dataset.title);
      // From its path, or from a part read since, to the input's own tile
      // among the inputs its handler's part takes; once the tile is framed
      // there is nowhere to go. Choosing the tile again returns to the path.
      if(scope||inputAway){var start=rmEl('button','system-input-start',rmT('Show input'));start.type='button';start.addEventListener('click',function(){surface?.clearHover();select(operation,true,null,'input');});context.appendChild(start);}
      clear.textContent=rmT('Leave input path');controls?.append(context,clear);
    }
    caption.appendChild(colorKey);
  }
  function focusNode(n,center){surface?.focus(n.id,center);}
  async function select(n,navigate,source,focus){
    if(!n)return;
    var ticket=++selectionRevision;map.explorerMember=null;map.clearMapPreview?.();surface?.light?.([]);
    // Search is a chooser. Once a destination is chosen it must not continue
    // highlighting every other result or covering the destination's drawing.
    search.value=searchValue='';filter.value=filterValue='';updateResults();
    // Choosing an input, even its tile on the canvas, moves to its path; only
    // Show input frames the tile.
    var path=focus!=='input'&&rmInputPath(n,byID).length>0;
    // Reading anything else leaves the input's path (owner, 2026-09-28):
    // "serverCron ·" had prefixed every breadcrumb for four questions.
    if(n.dataset.activation){if(!operation)beforeInput=map.readingState();operation=n;scope='';inputAway=path;}
    else{scope=n.id;operation=null;inputAway=false;beforeInput=null;}
    emphasize();if(navigate)address(n,!!(focus||path)&&!!map.captureViewport?.()?.overview);
    await ready;if(ticket!==selectionRevision)return false;map.showNode?.(n);if(source)map.explainSource?.(source);
    if(focus==='input')surface?.showInput(n.id);
    else if(focus||path)focusNode(n,!!n.dataset.activation||focus==='center');
    emit();return true;
  }
  // A click on an arrow end reads the frame it stands on, scrolled to its
  // Connections with that connection open; the camera stays (owner's 3b).
  var pendingConnection=null;
  function openConnection(id,key){
    var frame=byID[id];if(!frame)return Promise.resolve(false);
    pendingConnection={id:id,key:key};
    return select(frame,true,null,false).finally(function(){pendingConnection=null;});
  }
  map.openConnection=openConnection;
  function reset(){selectionRevision++;scope='';operation=null;inputAway=false;beforeInput=null;surface?.clearHover();emphasize();map.clearInspection?.();emit();}
  map.showWholeMap=async function(){
    search.value=searchValue='';filter.value=filterValue='';updateResults();reset();
    address(null,true);document.querySelector('.nav .find')?.restoreSearch?.();
    await ready;await surface?.overview();emit();
  };
  map.componentSelection=function(){
    var selected=byID[scope||operation?.id];
    if(selected&&(selected.dataset.activation||selected.dataset.branch==='inputs')){
      var owner=selected.dataset.owner,component=nodes.find(function(n){return owner&&n.dataset.branch==='component'&&n.dataset.owner===owner;});
      return component?.dataset.owner||'';
    }
    if(map.captureViewport?.()?.componentsOpen===false)return '';
    var ids=path(scope||operation?.id),component=ids.map(function(id){return byID[id];}).find(function(n){return n.dataset.branch==='component';});
    return component?.dataset.owner||'';
  };
  map.selectComponent=function(id){return select(byID['system-component-'+id],true,null,'center');};
  map.closeDetails=function(){
    selectionRevision++;scope='';surface?.clearHover();emphasize();map.clearInspection?.();
    address(operation||null);emit();
  };
  clear.addEventListener('click',function(){
    var saved=operation&&beforeInput;
    if(!saved){reset();address(null);return;}
    beforeInput=null;
    map.restoreReadingState(Object.assign({},saved,{operation:''})).then(function(){address(byID[saved.scope]||null,true);});
  });
  function filterChanged(){searchValue=search.value;filterValue=filter.value;updateResults();emphasize();emit();}
  search.addEventListener('input',filterChanged);filter.addEventListener('change',filterChanged);
  nodes.forEach(function(n){n.addEventListener('click',function(e){e.preventDefault();e.stopImmediatePropagation();select(n,true);});});
  var inputWrites=nodes.filter(function(n){return n.dataset.activation;}).map(function(n){return {input:n,writes:rmPage.data(n,'writes')||[]};});
  var writeReading=null;
  map.addEventListener('repomap:reading',function(){
    if(!writeReading||writeReading.card.hidden)return;
    var key=map.explorerMember?.key||'';if(key===writeReading.key)return;
    writeReading.key=key;entityWrites(writeReading.node,writeReading.card);
  });
  function entityWrites(n,card){
    var entityKeys=new Set(repomapMembers.items(n).map(function(item){return repomapMembers.sourceKey(item.source);}));
    var selectedKey=map.explorerMember?.owner===n.id?map.explorerMember.key:'';
    var rows=inputWrites.flatMap(function(row){return row.writes.filter(function(write){return n.dataset.activation?row.input===n:entityKeys.has(repomapMembers.sourceKey(write.entity))&&(!selectedKey||repomapMembers.sourceKey(write.entity)===selectedKey);}).map(function(write){return {input:row.input,write:write};});});
    card.querySelector('.system-entity-writes')?.remove();
    if(!rows.length)return;
    // Folded under its count, after what the part is made of: 97 inputs'
    // writes into one type had stood above the part's own reading.
    var section=rmEl('details','system-entity-writes'),head=rmEl('summary');head.append(rmEl('span','',rmT('State changes')),rmEl('span','map-reading-peer-count',String(rows.length)));
    section.appendChild(head);section.open=rows.length<=rmShortSection;
    section.appendChild(rmEl('p','meta',rmT('Writes reachable in code; a call path does not prove they execute on every run.')));
    var entities=new Map();rows.forEach(function(row){var key=repomapMembers.sourceKey(row.write.entity);if(!entities.has(key))entities.set(key,[]);entities.get(key).push(row);});
    entities.forEach(function(changes){
      var heading=rmEl('h6'),entity=changes[0].write;
      var source=repomapMembers.sourceLink(entity.entity);source.textContent=entity.entity_name;heading.appendChild(source);section.appendChild(heading);
      var inputs=new Map();changes.forEach(function(row){if(!inputs.has(row.input.id))inputs.set(row.input.id,[]);inputs.get(row.input.id).push(row);});
      inputs.forEach(function(evidence){
        var input=evidence[0].input;
        if(!n.dataset.activation){var jump=rmEl('button','system-write-input',owner(input)+' / '+input.dataset.title);jump.type='button';jump.addEventListener('click',function(){select(input,true,null,true);});section.appendChild(jump);}
        var fields=rmEl('ul','plain');evidence.forEach(function(row){
          var write=row.write,field=rmEl('li');field.appendChild(rmEl('strong','',write.field));
          if(write.possible)field.appendChild(rmEl('span','possible',' · '+rmT('possible')));
          field.appendChild(document.createElement('br'));field.appendChild(repomapMembers.sourceLink(write.source));
          if(write.integration)field.appendChild(rmEl('span','possible',' · '+rmT('possible integration')));
          // Its writer's callers on this input's path: every call into it,
          // none chosen as its route.
          if((write.callers||[]).length){
            var callers=rmEl('details','call-path');callers.appendChild(rmEl('summary','',rmT('Called by')));var list=rmEl('ul');
            write.callers.forEach(function(step){var li=rmEl('li');li.appendChild(rmEl('strong','',step.name));if(step.possible)li.appendChild(rmEl('span','possible',' · '+rmT('possible call')));li.appendChild(document.createElement('br'));li.appendChild(repomapMembers.sourceLink({Href:step.href,Open:step.open,Text:step.source,NoSource:step.no_source}));list.appendChild(li);});
            callers.appendChild(list);field.appendChild(callers);
          }
          fields.appendChild(field);
        });section.appendChild(fields);
      });
    });
    var partReading=!n.dataset.activation&&card.querySelector('.map-part-reading');
    if(partReading)partReading.appendChild(section);else card.querySelector('.map-card-intro').after(section);
  }
  function nodeByHref(href){return href?nodes.find(function(n){return n.getAttribute('href')===href||'#'+n.id===href;})||null:null;}
  // What the reading column reads with (31-reading-column.js): every name
  // it reads is read in the report and shown on the canvas, the camera
  // moving only when it is out of sight.
  map.readingContext=function(){return {
    nodeByHref:nodeByHref,
    nodeById:function(id){return byID[id]||null;},
    goDecl:function(decl){var part=nodeByHref(decl.part);return part&&!part.dataset.activation&&decl.key?function(){readDeclaration(part,decl.key);}:null;},
    readDeclIn:readDeclaration,
    readNode:function(n){surface?.clearMember?.();select(n,true,null,true);},
    light:function(ids){surface?.light?.(ids);}
  };};
  // The frame holding what is read, and going up to it: from a declaration
  // to its part, which is read without it; from a part to its area or
  // component.
  map.parentFrame=function(n){var id=projection.parents[n.id];return id?byID[id]||null:null;};
  map.readUp=function(n,member){
    surface?.clearMember?.();
    if(member){document.dispatchEvent(new Event('repomap:visit'));map.inspectConcept?.(-1);return;}
    select(n,true,null,true);
  };
  // What an area is made of: its parts in their order, each with how many
  // declarations of each kind in which files, and those declarations, the
  // model's keys first and bold, the rest by name. A part leads to its
  // reading; a declaration to its own, in its part.
  function areaComposition(n){
    var parts=(n.dataset.children||'').split(/\s+/).map(function(id){return byID[id];}).filter(function(part){return part&&!part.dataset.activation&&!part.dataset.branch;});
    if(!parts.length)return null;
    var section=rmEl('section','map-area-composition'),made=parts.map(function(part){return repomapMembers.composition(part);});
    var heading=rmEl('h5','map-made-of');heading.appendChild(rmEl('span','',rmT('Made of {0} parts',parts.length)));section.appendChild(heading);
    parts.forEach(function(part,index){
      var entry=rmEl('div','map-area-part'),go=rmEl('button','',part.dataset.title);go.type='button';
      go.addEventListener('click',function(){select(part,true,null,false);});entry.appendChild(go);
      var counts=made[index];if(counts.total)entry.appendChild(rmEl('small','',counts.counts+(counts.files.length?' · '+counts.files.join(', '):'')));
      var members=rmEl('div','map-area-members');
      repomapMembers.sorted(part).forEach(function(item){
        var link=repomapMembers.sourceLink(item.source);link.textContent=repomapMembers.displayName(item);if(item.key)link.className='map-member-key';
        link.title=[item.source.Text,item.source.NoSource?rmT('No source'):''].filter(Boolean).join('\n');
        link.addEventListener('click',function(event){event.preventDefault();event.stopPropagation();select(part,true,{href:item.source.Href,open:item.source.Open,key:repomapMembers.sourceKey(item.source)},false);});
        members.appendChild(link);
      });
      if(members.childElementCount)entry.appendChild(members);
      section.appendChild(entry);
    });
    return section;
  }
  // A part read while an input is pinned says, in its heading, that it is
  // off that input's path, drawn with the reading itself. Drawn from the
  // canvas's emphasis it came a frame late, and again each time the pointer
  // left the canvas for the column: Introspection and debugging's list moved
  // 26 px under a reader's click, and monitorCommand was read for pingCommand.
  function markOutside(n){
    var heading=map.querySelector('.map-inspector-heading');if(!heading)return;
    heading.querySelector('.map-reading-outside')?.remove();
    if(projection.outside(n.id,operation?.id))heading.appendChild(rmEl('small','map-reading-outside',rmT('Outside this input path')));
  }
  // A declaration named in the reading is read in its part, as a click on
  // its tile reads it: in place when that part is the one being read.
  // A new declaration in the same part is a new visit: Back returns to the
  // one read before it.
  function readDeclaration(part,key){
    if(!part)return;
    if(scope===part.id){document.dispatchEvent(new Event('repomap:visit'));map.explainSource?.({key:key});return;}
    map.revealNode?.(part,false,{key:key});
  }
  // The handler an input's reading names ("handled by getCommand") is read
  // in its part, as the path's steps are, when that part lists it; a
  // modifier-click still opens its code.
  function readsHandler(n,card){
    var name=card.querySelector('.map-card-handler>a'),part=byID[projection.inputOwner[n.id]],key=n.dataset.handlerSource||n.dataset.handlerOpen;
    if(!name||!part||part.dataset.activation||!key)return;
    var symbols=rmPage.data(part,'symbols')||[];
    if(!symbols.some(function(symbol){return symbol.href===key||symbol.open===key;}))return;
    name.addEventListener('click',function(event){
      if(event.button||event.metaKey||event.ctrlKey||event.shiftKey||event.altKey)return;
      event.preventDefault();event.stopPropagation();readDeclaration(part,key);
    });
  }
  map.addEventListener('repomap:inspect',function(e){
    var n=e.detail.node,card=e.detail.card;
    markOutside(n);
    if(n.dataset.activation)readsHandler(n,card);
    entityWrites(n,card);
    // What an area is made of comes first, under its description (owner's 3a).
    if(n.dataset.branch==='area'){var composition=areaComposition(n),intro=card.querySelector('.map-card-intro');var actions=intro?.querySelector(':scope>.map-card-actions');if(composition&&actions)actions.before(composition);else if(composition&&intro)intro.appendChild(composition);}
    // A frame's Connections are its arrow ends as the canvas groups them,
    // each opening to the calls its card lists; they replace the list of
    // neighbours by name.
    // A part's are its arrow ends when it is the frame looked at: to the
    // parts and areas beside it, as its plaques stand for them.
    var frameConnections=null,partConnections=card.querySelector('.map-part-reading');
    if(n.dataset.branch==='area'||n.dataset.branch==='component'||partConnections){
      frameConnections=rmEl('div','map-frame-connections-holder');
      if(surface?.mountConnections&&surface.mountConnections(frameConnections,n.id,pendingConnection?.id===n.id?pendingConnection.key:'',function(part,key){readDeclaration(byID[part],key);})){
        if(partConnections)partConnections.insertBefore(frameConnections,partConnections.querySelector(':scope>.map-reading-in')||partConnections.querySelector(':scope>.map-reading-out'));
        else (card.querySelector('.map-area-composition')||card.querySelector('.map-card-intro'))?.after(frameConnections);
      }else frameConnections=null;
    }
    writeReading={node:n,card:card,key:map.explorerMember?.key||''};
    var group=document.getElementById((n.getAttribute('href')||'').slice(1));
    if(!n.dataset.activation){
      if(!n.dataset.branch)card.querySelector('.map-related-operations')?.remove();
      var selectedMembers=new Set(projection.leaves(n.id));selectedMembers.add(n.id);
      var reaching=nodes.filter(function(candidate){if(!candidate.dataset.activation)return false;return Array.from(projection.selection('',candidate.id).active).some(function(id){return selectedMembers.has(id);});});
      var inputs=rmReachingInputs(n,reaching,owner,function(input){select(input,true,null,true);});
      if((rmPage.data(n,'concepts')||[]).length)inputs.appendChild(rmEl('p','meta',rmT('Reaching a part does not by itself establish a change to its entities.')));
      var partReading=card.querySelector('.map-part-reading');
      if(partReading)partReading.appendChild(inputs);
      else if(!n.dataset.branch||n.dataset.branch==='communication'){card.querySelector('.map-card-intro').after(inputs);}
    }
    if(partReading){
      // The part's own reading (31-reading-column.js) replaces its copied
      // connection lists: the input's witness under its description, the
      // inputs reaching it and the calls inside it at its foot.
      var proof=card.querySelector(':scope>.call-path');
      if(proof){(partReading.querySelector(':scope>.map-card-summary')||partReading.querySelector(':scope>.map-part-title')).after(proof);proof.open=true;}
      card.querySelector('.map-card-evidence')?.remove();
      return;
    }
    if(!n.dataset.branch&&!n.dataset.activation&&group?.classList.contains('group')){
      // A part naming no declaration has no reading: its arrows' cards
      // are its connections.
      var witness=card.querySelector('.call-path');
      if(witness){card.querySelector('.map-card-intro').after(witness);witness.open=true;}
      card.querySelector('.map-card-evidence')?.remove();
      return;
    }
    if(n.dataset.branch==='communication'){
      card.querySelector('.map-card-actions')?.remove();
      card.querySelector('.map-related-operations')?.remove();
      var calls=rmEl('section','system-communication-records');
      (n.dataset.children||'').split(/\s+/).filter(Boolean).forEach(function(id){
        var child=byID[id];if(!child)return;var item=rmEl('article');
        var jump=rmEl('button','',child.dataset.title);jump.type='button';jump.addEventListener('click',function(){select(child,true,null,true);});
        item.appendChild(jump);if(child.dataset.summary)item.appendChild(rmEl('p','',child.dataset.summary));calls.appendChild(item);
      });card.querySelector('.map-card-intro').after(calls);
    }
    if(n.dataset.itemKind==='External communication'){
      var proof=card.querySelector('.call-path');if(proof){card.querySelector('.map-card-intro').after(proof);proof.open=true;}
      card.querySelector('.map-card-evidence')?.remove();
    }
    // An input's reading carries the Inputs blue, never core's purple.
    map.querySelector('.map-inspector')?.classList.toggle('map-reading-input',!!n.dataset.activation);
    var inputPath=rmPage.data(n,'inputPath');
    var catalogue=rmPage.data(n,'catalogue');
    if(n.dataset.branch==='inputs'&&n.dataset.launch){
      var launch=rmPage.data(n,'launch');
      if(launch)card.querySelector('.map-card-intro').after(rmLaunchSection(launch,function(id){return byID[id]&&byID[id].dataset.activation?byID[id]:null;},function(input){select(input,true,null,true);},readDeclaration,function(id){return byID[id]&&!byID[id].dataset.activation?byID[id]:null;}));
    }
    if(n.dataset.activation&&n.dataset.declares){
      // The inputs declared on the object this input's own call made.
      var declared=rmEl('div','system-neighbours');declared.appendChild(rmEl('span','meta',rmT('Declares')));
      n.dataset.declares.split(/\s+/).forEach(function(id){var input=byID[id];if(!input)return;var b=rmEl('button','',input.dataset.title);b.type='button';b.addEventListener('click',function(){select(input,true,null,true);});declared.appendChild(b);});
      card.querySelector('.map-card-intro').after(declared);
    }
    if(n.dataset.activation&&catalogue){
      var catalogueSection=rmCatalogueSection(catalogue,n.dataset.title,function(id){return byID[id]&&byID[id].dataset.activation?byID[id]:null;},function(input){select(input,true,null,true);},readDeclaration,function(id){return byID[id]&&!byID[id].dataset.activation?byID[id]:null;});
      catalogueSection.dataset.readingAnchor='';
      card.querySelector('.map-card-intro').after(catalogueSection);
    }
    if(n.dataset.activation&&inputPath){
      // A chosen input's reading opens at its path.
      var pathSection=rmInputPathSection(inputPath,n.dataset.title,function(id){return byID[id]&&!byID[id].dataset.activation?byID[id]:null;},function(id){return byID[id]&&byID[id].dataset.activation?byID[id]:null;},function(part){select(part,true,null,true);},readDeclaration);
      pathSection.dataset.readingAnchor='';
      card.querySelector('.map-card-intro').after(pathSection);
    }
    if(n.dataset.activation){
      // The program's Main flow, after where the input is dispatched from.
      var flowLink=rmInputFlow(document.getElementById(n.dataset.owner));
      if(flowLink){
        var dispatchBoxes=pathSection?pathSection.querySelectorAll(':scope>.system-shared-path'):[];
        if(dispatchBoxes.length)dispatchBoxes[dispatchBoxes.length-1].after(flowLink);
        else if(pathSection)pathSection.querySelector(':scope>h5').after(flowLink);
        else card.querySelector('.map-card-intro').after(flowLink);
      }
    }
    if(n.dataset.activation&&!inputPath){
      var pathState=projection.selection('',n.id),pathParts=rmEl('section','system-input-parts');
      pathParts.appendChild(rmEl('h5','',rmT('Parts on this input path')));
      var pathLinks=rmEl('div','system-neighbours');
      // Nearest the handler first: the saved trace orders the parts by call
      // depth; a part reached only through a matched input follows it.
      var ordered=(n.dataset.inputTrace||'').split(/\s+/).filter(function(id){return pathState.active.has(id);});
      pathState.active.forEach(function(id){if(!ordered.includes(id))ordered.push(id);});
      ordered.forEach(function(id){
        var part=byID[id];if(!part||part.dataset.activation)return;
        var link=rmEl('button','',part.dataset.title);link.type='button';
        link.addEventListener('click',function(){select(part,true,null,true);});pathLinks.appendChild(link);
      });
      if(pathLinks.childElementCount){pathParts.appendChild(pathLinks);card.appendChild(pathParts);}
    }
    if(n.dataset.activation){
      var linkState=projection.selection('',n.id);
      var linkedInputs=new Set();linkState.path.forEach(function(edge){[edge.from,edge.to].forEach(function(id){if(id!==n.id&&byID[id]?.dataset.activation)linkedInputs.add(id);});});
      if(linkedInputs.size){
        var connected=rmEl('section','system-linked-inputs');connected.appendChild(rmEl('h5','',rmT('Connected inputs')));
        linkedInputs.forEach(function(id){var link=rmEl('button','',byID[id].dataset.title);link.type='button';link.addEventListener('click',function(){select(byID[id],true,null,true);});connected.appendChild(link);});card.appendChild(connected);
      }
    }
    var relations=rmEl('div','system-neighbours'),members=new Set(projection.leaves(n.id)),seen=new Set();
    members.add(n.id);rawEdges.filter(function(r){return r.scope==='structure'||r.scope==='component';}).forEach(function(r){
      var outgoing=members.has(r.from),incoming=members.has(r.to);if(outgoing===incoming)return;
      var id=outgoing?r.to:r.from,key=(outgoing?'out:':'in:')+id;if(seen.has(key)||!byID[id])return;seen.add(key);
      // An outside call is named by where it goes: two calls of one client
      // to two services are not one neighbour.
      var destination=nodes.find(function(frame){return frame.dataset.branch==='communication'&&(frame.dataset.children||'').split(/\s+/).includes(id);});
      var peerName=(byID[id].dataset.owner!==n.dataset.owner&&owner(byID[id])?owner(byID[id])+' / ':'')+(destination&&destination.dataset.title!==byID[id].dataset.title?destination.dataset.title+' · ':'')+byID[id].dataset.title;
      var b=rmEl('button','',(outgoing?'→ ':'← ')+peerName);b.type='button';b.addEventListener('click',function(){select(byID[id],true,null,true);});relations.appendChild(b);
    });
    if(relations.childElementCount&&!frameConnections)card.querySelector('.map-card-intro').appendChild(relations);
    var details=document.getElementById(n.dataset.detailsId);
    if(n.dataset.branch==='inputs'&&n.dataset.collection){
      // An Inputs collection is read by its catalogues (31-reading-column.js),
      // its component in its box on top; its records stay on the component's
      // page.
      card.querySelector('.map-card-actions')?.remove();
      card.querySelector('.map-card-intro').after(rmCollectionView(map.readingContext(),n,rmPage.data(n,'collection')));
    }else if(n.dataset.branch==='component'&&details){
      rmComponentReading(map,n,card,details,byID['system-inputs-'+n.dataset.owner]||null,pendingEntry===n.id);
    }else if(details){
      var content=details;
      if(content){
        var copy=content.cloneNode(true);copy.removeAttribute('id');copy.querySelectorAll('[id]').forEach(function(el){el.removeAttribute('id');});
        if(n.dataset.branch==='inputs'){
          copy.querySelector('h3')?.remove();
          copy.querySelectorAll('[data-integration-group]').forEach(function(group){group.remove();});
        }
        if(copy.matches('details'))copy.open=true;
        if(n.dataset.itemKind==='External communication')copy.querySelectorAll('.outbound-call').forEach(function(detail){detail.open=true;});
        copy.querySelectorAll('.outbound-caller-part').forEach(function(link){
          var peer=nodes.find(function(candidate){return candidate.getAttribute('href')===link.getAttribute('href');});
          if(peer)link.addEventListener('click',function(event){event.preventDefault();event.stopPropagation();select(peer,true,null,true);});
        });
        card.insertBefore(copy,card.querySelector('.map-all-members'));
      }
    }
  });
  async function layout(){
    map.setAttribute('aria-busy','true');await Promise.resolve();
    var componentsByOwner=new Map();nodes.forEach(function(n){if(n.dataset.branch==='component'&&n.dataset.owner)componentsByOwner.set(n.dataset.owner,n);});
    var items=nodes.map(function(n){
      var component=n.dataset.activation||n.dataset.branch==='inputs'?componentsByOwner.get(n.dataset.owner):null;
      return {id:n.id,title:n.dataset.title,branch:n.dataset.branch,activation:n.dataset.activation,lane:n.dataset.lane,
        summary:n.dataset.summary,symbols:rmPage.data(n,'symbols')||[],symbolCalls:rmPage.data(n,'symbolCalls')||[],subtitle:n.dataset.subtitle,sourceKind:n.dataset.sourceKind,
        role:n.dataset.role,roleRef:n.dataset.roleRef,language:n.dataset.language,componentKind:n.dataset.componentKind,
        trace:n.dataset.activation?rmInputPath(n,byID):[],displayGroup:n.dataset.displayGroup||'',displayGroupTitle:n.dataset.displayGroupTitle||'',
        componentOwner:component?.id||'',componentName:component?.dataset.title||'',
        children:(n.dataset.children||'').split(/\s+/).filter(function(id){return id&&(n.dataset.branch==='inputs'||!byID[id]?.dataset.activation);}),kind:kind(n),category:category(n)};
    });
    var relations=rawEdges;
    try{
      surface=await rmCreateFlow(map,stage,items,relations,projection.areas,projection.inputOwner,{
        select:function(id,center){select(byID[id],true,null,center?'center':false);},
        // A zoom that brings another frame has the column read it, the
        // camera staying; a pinch leaves an input's path alone, the
        // magnifier reads the part it enters.
        follow:function(id,explicit){var n=byID[id];if(!n||scope===id||!explicit&&operation&&!scope)return;select(n,true,null,false);},
        openConnection:function(id,key){openConnection(id,key);},
        connection:function(group){map.previewConnection?.({from:group.incoming?group.outside:group.area,to:group.incoming?group.area:group.outside,possible:group.relations.some(function(r){return r.possible;}),relations:group.relations});}
      });
      map.visibleEdges=surface.layout.edges;
      if(map.hasAttribute('data-system-map'))rmProgramsTable(map.readingContext(),map.querySelector('[data-programs-table]'),nodes.filter(function(n){return n.dataset.branch==='component';}),
        function(id){return surface.frameConnections(id).map(function(group){return {incoming:group.incoming,title:group.title};});});
      emphasize();
    }catch(error){caption.textContent=rmT('Could not arrange this map. Reload to try again.');console.error(error);}
    map.setAttribute('aria-busy','false');
  }
  map.captureViewport=function(){return surface?.capture()||null;};
  map.restoreViewport=function(saved){surface?.restore(saved);};
  map.exploreNode=function(id){select(byID[id],true);};
  map.displayedNode=function(n){return byID[aliases[n.id]]||n;};
  map.areaDescriptions=function(n){return path(n.id).filter(function(id){return id!==n.id&&byID[id].dataset.branch==='area'&&projection.leaves(id).length===1;}).map(function(id){
    var area=byID[id];return (area.dataset.title!==n.dataset.title?area.dataset.title+': ':'')+area.dataset.summary;
  });};
  map.operationChoices=function(id){var children=new Set(projection.leaves(id));return nodes.filter(function(n){return n.dataset.activation&&((n.dataset.near||'').split(/\s+/).some(function(near){return children.has(near);})||children.has(projection.inputOwner[n.id]));});};
  map.chooseOperation=function(id){return select(byID[id],true);};
  map.explorationPath=function(){return rmExplorationPath(operation,path(scope).map(function(id){return byID[id];}),map.explorerMember);};
  map.explorationLabel=function(){return map.explorationPath().map(function(segment,index){return (index?segment.sep:'')+segment.title;}).join('')||rmT('System map');};
  // A breadcrumb segment goes up to its level, the reading and the camera
  // together: a frame is read and framed, the part named with a declaration
  // is read and centred without it, the declaration is read in its part and
  // its tile centred, the input is entered as its path.
  map.goToLevel=function(segment){
    var n=byID[segment?.id];if(!n)return Promise.resolve(false);
    if(segment.kind==='member')return map.revealNode(n,false,segment.source);
    surface?.clearMember?.();
    return select(n,true,null,segment.kind==='input'?true:'center');
  };
  map.resumeExploration=function(){var n=byID[scope]||operation;if(n)map.showNode(n);else map.clearInspection?.();};
  map.readingState=function(){return {scope:scope,operation:operation?.id||'',away:inputAway,search:searchValue,filter:filterValue,source:map.explorerMember||null,viewport:map.captureViewport?.()||null};};
  map.restoreReadingState=async function(saved){if(!saved)return;var ticket=++selectionRevision;map.readingRestoring=true;try{scope=byID[saved.scope]?saved.scope:'';operation=byID[saved.operation]||null;inputAway=!!operation&&!!saved.away;search.value=searchValue=saved.search||'';filter.value=filterValue=saved.filter||'';updateResults();await ready;if(ticket!==selectionRevision)return;emphasize();map.resumeExploration();if(saved.source)map.explainSource(saved.source);else map.inspectConcept?.(-1);if(saved.viewport)map.restoreViewport(saved.viewport);}finally{if(ticket===selectionRevision){map.readingRestoring=false;emit();}}};
  map.revealNode=async function(n,allUses,source){n=byID[aliases[n?.id]]||byID[n?.dataset?.mapAlias]||byID[n?.id];if(!n)return;if(allUses)operation=null;if(await select(n,true,source,true))rmScrollToReading(map);};
  map.findNode=function(n,source){return map.revealNode(n,true,source);};
  function mapped(node){if(!node)return null;if(byID[node.id])return byID[node.id];if(aliases[node.id])return byID[aliases[node.id]];if(node.dataset.mapAlias)return byID[node.dataset.mapAlias];return byID['system-component-'+node.id]||byID['system-component-'+node.id.replace(/-(parts|inbound|external)$/,'')];}
  function hashChanged(){
    if(map.readingRestoring)return;
    var n=document.getElementById(location.hash.slice(1)),target=mapped(n);
    var saved=history.state?.repomapReading?.map;
    // The page navigator restores complete history visits. A later hashchange
    // must not reinterpret that restoration as a new centred selection.
    if(saved?.page===map.closest('[data-report-page]')?.id&&target&&
      (saved.value?.scope===target.id||saved.value?.operation===target.id))return;
    if(n===document.getElementById('overview')||n===document.getElementById('repository-map')){reset();return;}
    if(target)select(target,n!==target,null,true);
  }
  // The Entrypoints link of a component's details lands on the program's
  // entry, the reading opened at it.
  var pendingEntry=null;
  document.addEventListener('click',function(e){
    var a=e.target.closest&&e.target.closest('a[data-entry-landing]');
    if(!a||e.button||e.ctrlKey||e.metaKey||e.shiftKey||e.altKey)return;
    var landing=rmEntryLanding(a,nodes,function(){return mapped(a.closest('[data-report-page]'));});
    if(!landing)return;
    e.preventDefault();e.stopImmediatePropagation();
    if(landing.entry)pendingEntry=landing.node.id;
    map.revealNode(landing.node,false,landing.source||undefined).finally(function(){pendingEntry=null;});
  },true);
  document.addEventListener('click',function(e){rmCatalogInputClick(e,function(id){var target=mapped(document.getElementById(id));if(target)map.revealNode(target,false);return !!target;});},true);
  document.addEventListener('click',function(e){var a=e.target.closest('a[href^="#"]');if(!a||a.closest('[data-map-explorer]')||a.hasAttribute('data-reading-map-return')||a.hasAttribute('data-open')||e.ctrlKey||e.metaKey||e.shiftKey||e.altKey)return;var n=document.getElementById(a.getAttribute('href').slice(1)),target=mapped(n);if(!target)return;e.preventDefault();e.stopImmediatePropagation();map.revealNode(target,false);},true);
  if(map.hasAttribute('data-system-map'))document.querySelector('.nav-home')?.addEventListener('click',function(e){if(e.ctrlKey||e.metaKey||e.shiftKey||e.altKey)return;e.preventDefault();map.showWholeMap().then(function(){rmScrollToReading(map);});});
  window.addEventListener('hashchange',hashChanged);
  ready=layout();ready.then(function(){hashChanged();});
});})();
