// The reading column as the owner chose it on 2026-09-28 (the designer's
// variant A, with his changes): a part, a declaration, a component and an
// Inputs collection, each rendered from the page data Go prepared and
// sorted (page_reading.go, page_system_map.go). Nothing here sorts or
// repairs that data. Each name read here is shown on the canvas as well: a
// click reads it in the column and marks it there, and the camera moves
// only when it is out of sight (29-operation-view.js). A modifier-click on
// a name still opens its code. Lists are plain names, a key in bold: no
// squares or chips before them, and no line numbers after them (owner,
// 2026-09-29: "человек будет видеть код"; the name is the link to it).
// <reading-column>

// The reading of a part (page_reading.go), from its card; null when the
// part names no declaration.
function rmGroupReading(node){
  var group=document.getElementById((node.getAttribute('href')||'').slice(1));
  if(!group||!group.dataset.reading)return null;
  if(!group.rmReading){
    // A declaration's key is left out where it is its link.
    group.rmReading=rmPage.data(group,'reading');
    group.rmReading.decls.forEach(function(decl){if(decl.key===undefined)decl.key=decl.href;});
  }
  return group.rmReading;
}
// What a relation other than a call says of its end: of a callee ("passed
// as a callback") and of a caller ("passes it as a callback").
var rmEndWords={
  out:{passes_callback:'passed as a callback',binds_implementation:'supplied as an implementation',decorates:'decorator',executes:'run',
    imports:'imported',includes:'included',implements:'implemented',reads:'read',writes:'written',sources:'sourced',integration:'receives the connection'},
  in:{passes_callback:'passes it as a callback',binds_implementation:'supplies it as an implementation',decorates:'decorated by it',executes:'runs it',
    imports:'imports it',includes:'includes it',implements:'implements it',reads:'reads it',writes:'writes it',sources:'sources it',integration:'connects to it'}
};
function rmUsesVariable(kind){return kind==='reads'||kind==='writes';}
// A declaration's name as a tree writes it: a function with "()", unless
// its name already closes a parenthesis ("ReplicateCommand.Run (inline)").
function rmCallableName(decl){return decl.name+(decl.kind==='function'&&!/\)$/.test(decl.name||'')?'()':'');}
// Model text is told apart by its style alone: italic, with a hover saying
// who wrote it; no chip opens it.
function rmModelText(tag,cls,text,ref){
  var item=rmEl(tag,(cls?cls+' ':'')+'model',text);item.title=rmT('written by the model');
  if(ref)item.dataset.displayRef=ref;
  return item;
}
// A declaration's name: a link into its code, all of its lines (`code`),
// whose plain click reads it (`go`), and whose hover says where it stands;
// then the one slot for opening its code (rmNameIcon): `code` names what it
// opens, the declaration's code by default, a caller's call line on a
// "Called by" row, null for none (the reading's own title).
function rmDeclName(decl,text,go,title,bold,code){
  var link=decl.href||decl.open?repomapMembers.sourceLink({Href:decl.code||decl.href,Open:decl.open,Text:text}):rmEl('span','',text);
  link.classList.add('map-reading-name');rmDotBreaks(link);
  if(decl.key)link.dataset.declKey=decl.key;
  if(bold&&decl.bold)link.classList.add('map-reading-key');
  // A clipped name keeps its whole spelling on its hover, above the rest.
  if(title)link.title=link.title&&link.title!==title?link.title+'\n'+title:title;
  if(go)link.addEventListener('click',function(event){
    if(event.button||event.metaKey||event.ctrlKey||event.shiftKey||event.altKey)return;
    event.preventDefault();event.stopPropagation();go(decl);
  });
  return rmNameIcon(link,code===undefined?{href:decl.code||decl.href,open:decl.open}:code);
}
// The one slot beside a name for opening its code: an icon, being chosen
// (owner, 2026-09-29), will open `code` ({href, open, title}). Until then a
// name is its link alone, and with no code to open it is always the name.
function rmNameIcon(name,code){return name;}
// A part named in a reading stands in a small box as the canvas draws it:
// its colour and frame by lane (core, entry). A drawn part is a button that
// reads it.
function rmPartBox(ctx,href,title,counted){
  var node=href?ctx.nodeByHref(href):null,box=rmEl(node&&!counted?'button':'span','map-part-box',node?node.dataset.title:title);
  if(node){
    if(node.dataset.lane==='core')box.classList.add('map-part-box-core');
    else if(node.dataset.lane==='triggers')box.classList.add('map-part-box-entry');
    if(node.dataset.activation||node.dataset.branch==='inputs')box.classList.add('map-part-box-input');
    else if(node.dataset.itemKind==='External communication')box.classList.add('map-part-box-external');
    if(box.tagName==='BUTTON'){box.type='button';box.addEventListener('click',function(event){event.stopPropagation();ctx.readNode(node);});}
  }
  return box;
}
// A neighbour of a part: a part in its box, or the inputs registered at it,
// counted as one.
function rmPeerBox(ctx,peer){
  if(!peer.inputs)return rmPartBox(ctx,peer.part,peer.title);
  return rmEl('span','map-part-box map-part-box-input',rmT('Inputs'));
}
// The hover of a name in another part: that part.
function rmEndTitle(ctx,decl){
  var node=decl.part?ctx.nodeByHref(decl.part):null;
  return node?node.dataset.title:'';
}
// A name as the column writes it: it breaks only after a dot or a slash,
// never inside a word, at an underscore or at a hyphen (litestream's
// "sql.Tx.Rollbac k", freqtrade's "process_open_trade / _positions",
// "redis-" / "benchmark.c"); a piece longer than its line ends in "…", the
// whole name on its hover (43-map-reading.css). A callable written inline
// reads "anonymous function in ReplicateCommand.Run" (GroupsIndex names it
// "ReplicateCommand.Run (inline)").
var rmLongPiece=24;
function rmInlineText(text){return String(text).replace(/([^\s→(]+) \(inline\)/g,function(_,home){return rmT('anonymous function in {0}',home);});}
function rmDotBreaks(element){
  var said=element.textContent,text=rmInlineText(said),inline=text!==said;
  if(!/[.\/-]/.test(text)&&text.length<=rmLongPiece){if(inline)element.textContent=text;return element;}
  element.textContent='';
  var long=false;
  // Words break at their spaces; a name within them only after its dots
  // and slashes.
  text.split(/(\s+)/).forEach(function(word){
    if(!word)return;
    if(/^\s+$/.test(word)){element.appendChild(document.createTextNode(word));return;}
    word.split(/(?<=[.\/])/).forEach(function(piece,i){
      if(i)element.appendChild(document.createElement('wbr'));
      var span=rmEl('span','map-name-piece',piece);if(piece.length>rmLongPiece){span.classList.add('map-name-long');long=true;}
      element.appendChild(span);
    });
  });
  if(long&&!element.title)element.title=text;
  return element;
}
// One end of a relation: its name, and what the relation says of it when it
// is not a call (a variable's readers say only who writes it: "Used by"
// says the rest). A name is its link: the page prints no separate code
// marks (owner, 2026-09-29); a caller's name keeps where it makes the call
// for its code slot.
function rmEndItem(ctx,data,end,side,quiet){
  var decl=data.decls[end.decl],item=rmEl('li'),site=side==='in'&&end.site;
  item.appendChild(rmDeclName(decl,rmCallableName(decl),ctx.goDecl(decl),rmEndTitle(ctx,decl),false,site?{href:site.href,open:site.open,title:site.at}:undefined));
  var words=quiet&&end.kind==='reads'?'':rmEndWords[side][end.kind];
  if(words)item.appendChild(rmEl('span','map-reading-relation',rmT(words)));
  if(end.possible)item.appendChild(rmEl('span','possible',rmT('possible')));
  return item;
}
// The heading's kind line: its word, then the frame holding what is read
// as a link up to it ("Part · Core infrastructure ↑"). Up from a
// declaration is its part (`member`).
function rmHeadingKind(kind,word){kind.replaceChildren(document.createTextNode(word));}
function rmHeadingUp(map,kind,frame,member){
  if(!kind||!frame)return;
  var up=rmEl('button','map-reading-up',frame.dataset.title+' ↑');up.type='button';
  up.addEventListener('click',function(){map.readUp(frame,!!member);});
  kind.append(document.createTextNode(' · '),up);
}
// One caller and what it calls in the part. A caller reaching many of its
// declarations through one dispatch site is one line: "loadAppendOnlyFile()
// → 17 request handlers, possible, via cmdTable", its ends folded under it.
function rmCallerLine(ctx,data,line){
  var caller=data.decls[line.caller],row=rmEl(line.fan?'details':'div','map-reading-caller'),head=line.fan?rmEl('summary'):row;
  head.appendChild(rmDeclName(caller,rmCallableName(caller),ctx.goDecl(caller),rmEndTitle(ctx,caller)));
  if(line.fan){
    var fan=line.fan,say=rmEl('span','map-reading-fan');
    say.append(document.createTextNode(' → '+rmT(({request:'request handlers',command:'command handlers'})[fan.noun]||'functions')));
    if(line.ends.every(function(end){return end.possible;}))say.append(document.createTextNode(', '),rmEl('span','possible',rmT('possible')));
    if((fan.via||[]).length){
      say.append(document.createTextNode(', '+rmT('via')+' '));
      fan.via.forEach(function(at,i){if(i)say.append(document.createTextNode(', '));var via=data.decls[at];say.appendChild(rmDeclName(via,via.name,ctx.goDecl(via),via.at));});
    }
    head.appendChild(say);row.appendChild(head);
  }
  var ends=rmEl('ul','map-reading-ends');line.ends.forEach(function(end){ends.appendChild(rmEndItem(ctx,data,end,'out'));});
  row.appendChild(ends);
  return row;
}
// Functions by the part they stand in: the part in its box on a line of
// its own, its functions under it one to a line (owner, 2026-09-29:
// several names to a line had read as a wall). A name reads its
// declaration; no line numbers.
function rmNamesByPart(ctx,data,groups){
  var box=rmEl('div','map-names-by-part');
  groups.forEach(function(group){
    var peer=rmEl('div','map-reading-peer');
    if(group.part||group.title){var head=rmEl('div','map-reading-peer-head');head.appendChild(rmPartBox(ctx,group.part,group.title));peer.appendChild(head);}
    var list=rmEl('ul','map-reading-ends');
    group.decls.forEach(function(at){var decl=data.decls[at],item=rmEl('li');item.appendChild(rmDeclName(decl,rmCallableName(decl),ctx.goDecl(decl),rmEndTitle(ctx,decl)));list.appendChild(item);});
    peer.appendChild(list);box.appendChild(peer);
  });
  return box;
}
// Who changes a field and who reads it (owner, 2026-09-29;
// page_field_uses.go): "Written by", then "Read by", each its functions by
// the part they stand in; a long side folds under its words, no count.
function rmFieldUses(ctx,data,use){
  var box=rmEl('div','map-field-uses');
  [['written','Written by'],['read','Read by']].forEach(function(pair){
    var groups=use[pair[0]]||[];if(!groups.length)return;
    var count=groups.reduce(function(sum,group){return sum+group.decls.length;},0),long=count>12;
    var side=rmEl(long?'details':'div','map-field-side');side.appendChild(rmEl(long?'summary':'span','map-field-side-head',rmT(pair[1])));
    side.appendChild(rmNamesByPart(ctx,data,groups));
    box.appendChild(side);
  });
  return box;
}

// Where an outside call is reached from (page_outbound.go, GroupsIndex
// ReachedFrom): the first callers outside the part making it, each part in
// its box with its callers by name, another program's part named with its
// program; a path starting inside that part at a seed or an input's
// handler stands under the part's own box. Redis's connect, written in
// anet.c, reads syncWithMaster, cliConnect and createClient, not the
// wrapper one hop away. A long list folds under its count (freqtrade's
// exchange calls are reached from 39 functions). No line numbers: a name
// reads its function in the column and on the canvas.
function rmReachedFrom(ctx,data){
  if(!data||!(data.groups||[]).length)return null;
  data.decls.forEach(function(decl){if(decl.key===undefined)decl.key=decl.href;});
  var total=data.groups.reduce(function(sum,group){return sum+group.decls.length;},0),long=total>12;
  var section=rmEl(long?'details':'section','map-reading-side map-reading-in outbound-reached'),heading=rmEl(long?'summary':'h6','',rmT('Called from'));
  section.appendChild(heading);
  data.groups.forEach(function(group){
    var box=rmEl('div','map-reading-peer'),head=rmEl('div','map-reading-peer-head');
    if(group.program)head.appendChild(rmDotBreaks(rmEl('span','map-reading-program',group.program+':')));
    head.appendChild(rmPartBox(ctx,group.part,group.title));box.appendChild(head);
    var list=rmEl('ul','map-reading-ends');group.decls.forEach(function(end){list.appendChild(rmEndItem(ctx,data,end,'in'));});
    box.appendChild(list);section.appendChild(box);
  });
  return section;
}
// Where several outside calls are reached from, as one "Called from": each
// record's `reached` (page data) joined by program and part, each caller
// once.
function rmMergeReached(list){
  var decls=[],at=new Map(),groups=[],byPart=new Map();
  list.forEach(function(data){
    if(!data||!(data.groups||[]).length)return;
    data.groups.forEach(function(group){
      var key=(group.program||'')+'\u0000'+(group.part||group.title||''),into=byPart.get(key);
      if(!into){into={part:group.part,title:group.title,program:group.program,decls:[]};byPart.set(key,into);groups.push(into);}
      group.decls.forEach(function(end){
        var decl=data.decls[end.decl];if(!decl)return;
        var id=decl.key||decl.href||decl.open||decl.name;
        if(!at.has(id)){at.set(id,decls.length);decls.push(decl);}
        var position=at.get(id);
        if(!into.decls.some(function(other){return other.decl===position;}))into.decls.push(Object.assign({},end,{decl:position}));
      });
    });
  });
  return groups.length?{decls:decls,groups:groups}:null;
}
// An outside call's record as the column reads it (owner, 2026-09-29), from
// its row on the component's page (target.html "outbound-row"): what the
// call is, once, by its outside name ("netdb.h.gethostbyname"), a link into
// the line making it with that place said on hover; its kind and address;
// then "Called from" (`reached`, rmReachedFrom) and the names its address
// passes through, each a link. No place is printed. The model's note stands
// once, in the card's intro (`said`), and the run from the program's own
// code ("redis-server connects out from syncWithMaster → anetTcpConnect →
// …") only when no "Called from" says it by part.
function rmOutboundRecord(record,reached,title,said){
  function kid(parent,test){return parent&&parent.children?Array.prototype.find.call(parent.children,test)||null:null;}
  function has(element,cls){return !!element&&!!element.classList&&element.classList.contains(cls);}
  function tag(name){return function(element){return element.tagName===name;};}
  // A name with the place its anchor printed: the name, a link to that
  // place, which its hover says.
  function linked(name,anchor){
    var href=anchor&&anchor.getAttribute('href'),at=anchor?anchor.textContent:'';
    return rmDeclName({name:name,href:href&&href!=='#'?href:'',open:anchor&&anchor.dataset.open||''},name,null,at,false,null);
  }
  var call=kid(record,function(element){return has(element,'outbound-call');}),body=call&&kid(call,function(element){return has(element,'outbound-call-body');});
  if(!body)return;
  var line=kid(call,tag('SUMMARY')),source=kid(body,function(element){return has(element,'outbound-source');});
  if(source){
    var callable=kid(source,tag('CODE'))||kid(line,tag('CODE')),statement=linked(callable?callable.textContent:title,source.querySelector('.anchor'));
    statement.classList.add('outbound-callable');
    var words=line&&line.querySelector('.outbound-words');
    source.replaceChildren(statement);if(words)source.append(document.createTextNode(' '),words);
  }
  // A note the card's intro does not say stays first, unless the full
  // purpose below says it.
  var note=!said&&line&&line.querySelector('.outbound-note'),parts=[],after=-1;
  if(note&&!body.querySelector('.outbound-purpose')){var purpose=rmEl('p','outbound-purpose');purpose.appendChild(note);parts.push(purpose);}
  Array.prototype.slice.call(body.children).forEach(function(part){
    if(reached&&has(part,'outbound-side')||said&&has(part,'outbound-purpose'))return;
    parts.push(part);
    if(has(part,'outbound-source')||has(part,'outbound-runs')||has(part,'outbound-side'))after=parts.length;
  });
  if(reached)parts.splice(after<0?0:after,0,reached);
  body.replaceChildren.apply(body,parts);
  // Where the call is made from reads its callables as the column names
  // them; how many address sources there are is not counted.
  body.querySelectorAll('.outbound-side code').forEach(function(code){code.textContent=rmInlineText(code.textContent);});
  body.querySelectorAll(':scope>p.meta').forEach(function(meta){meta.textContent=meta.textContent.replace(/\s*·\s*\d+\s*$/,'');});
  body.querySelectorAll('.outbound-chain').forEach(function(chain){
    var steps=kid(chain,tag('OL'));if(!steps)return;
    Array.prototype.slice.call(steps.children).forEach(function(step){
      var name=kid(step,tag('CODE'));if(name)step.replaceChildren(linked(name.textContent,step.querySelector('.anchor')));
    });
  });
  record.replaceChildren(body);
}

// A part's reading (owner, 2026-09-28): the part in its box, the model's
// description and its files, one to a line; its key declarations, bold,
// one to a line, and every other declaration under one closed fold, file
// by file, those reached from outside first (owner, 2026-09-29: "Made of
// 123 functions" and its names several to a line had been a wall); its
// connections (29-operation-view.js mounts them); the parts calling into
// it, each caller with what it calls here, folded when long; then the
// parts it calls into, each with its callees, and the variables it uses
// there. No counts.
function rmPartView(ctx,node,data){
  var view=rmEl('div','map-part-reading');
  var title=rmEl('div','map-part-title');title.appendChild(rmPartBox(ctx,node.getAttribute('href'),node.dataset.title,true));view.appendChild(title);
  if(node.dataset.summary)view.appendChild(rmModelText('p','map-card-summary',node.dataset.summary,node.dataset.summaryRef));
  var files=data.files||[];
  if(files.length){var fileList=rmEl('ul','map-part-files meta');files.forEach(function(file){fileList.appendChild(rmDotBreaks(rmEl('li','',file)));});view.appendChild(fileList);}
  // Its declarations in Go's order (by kind, then name); those reached from
  // outside the part ("Called from": a caller in another part, an input
  // registered at it, a callable handed over) are its ways in.
  var order=[],outside=new Set();
  (data.members||[]).forEach(function(kind){kind.decls.forEach(function(position,i){order.push(position);if(i<(kind.outside||0))outside.add(position);});});
  // The keys stand open; a part with none stands its ways in open instead.
  var keys=order.filter(function(position){return data.decls[position].bold;});
  var shown=keys.length?keys:order.filter(function(position){return outside.has(position);});
  var rest=order.filter(function(position){return shown.indexOf(position)<0;});
  function names(positions,cls){
    var ul=rmEl('ul','map-reading-names'+(cls?' '+cls:''));
    positions.forEach(function(position){var decl=data.decls[position],item=rmEl('li');item.appendChild(rmDeclName(decl,decl.name,ctx.goDecl(decl),decl.at,true));ul.appendChild(item);});
    return ul;
  }
  // The ways in first, set apart from the rest by a rule.
  function listed(into,positions){
    var reached=positions.filter(function(position){return outside.has(position);}),other=positions.filter(function(position){return !outside.has(position);});
    if(reached.length&&other.length)into.append(names(reached,'map-reading-reached'),names(other));
    else into.appendChild(names(positions));
  }
  if(shown.length){var members=rmEl('section','map-reading-members');members.appendChild(names(shown));view.appendChild(members);}
  if(rest.length){
    var fold=rmEl('details','map-reading-members map-reading-other');fold.appendChild(rmEl('summary','',rmT(shown.length?'Other declarations':'Declarations')));
    // A part of several files, as Core data structures is of adlist.c,
    // dict.c, sds.c, zipmap.c and zmalloc.c, lists them file by file.
    if(files.length>1)files.forEach(function(file){
      var here=rest.filter(function(position){return data.decls[position].file===file;});if(!here.length)return;
      var box=rmEl('section','map-reading-file');box.appendChild(rmDotBreaks(rmEl('h5','map-reading-file-name',file.split('/').pop())));
      listed(box,here);fold.appendChild(box);
    });
    else listed(fold,rest);
    view.appendChild(fold);
  }
  if(data.in&&data.in.length){
    // Many callers fold the whole list: Server core state had its own
    // declarations below 2,000 px of them.
    var lines=data.in.reduce(function(sum,peer){return sum+peer.lines.length;},0),long=lines>12;
    var incoming=rmEl(long?'details':'section','map-reading-side map-reading-in'),heading=rmEl(long?'summary':'h6','',rmT('Called from'));
    incoming.appendChild(heading);
    data.in.forEach(function(peer){
      var box=rmEl(long?'details':'div','map-reading-peer'),head=rmEl(long?'summary':'div','map-reading-peer-head');
      head.appendChild(rmPeerBox(ctx,peer));box.appendChild(head);
      peer.lines.forEach(function(line){box.appendChild(rmCallerLine(ctx,data,line));});
      incoming.appendChild(box);
    });
    view.appendChild(incoming);
  }
  if(data.out&&data.out.length){
    // Many callees fold as many callers do: Client I/O's calls into other
    // parts had run to 8,500 px, one name to a line.
    var names=data.out.reduce(function(sum,peer){return sum+peer.lines[0].ends.length;},0),wide=names>12;
    var outgoing=rmEl(wide?'details':'section','map-reading-side map-reading-out');outgoing.appendChild(rmEl(wide?'summary':'h6','',rmT('Calls into')));
    data.out.forEach(function(peer){
      var box=rmEl(wide?'details':'div','map-reading-peer'),head=rmEl(wide?'summary':'div','map-reading-peer-head');
      head.appendChild(rmPeerBox(ctx,peer));box.appendChild(head);
      var ends=peer.lines[0].ends,calls=rmEl('ul','map-reading-ends'),uses=rmEl('ul','map-reading-ends');
      ends.forEach(function(end){(rmUsesVariable(end.kind)?uses:calls).appendChild(rmEndItem(ctx,data,end,'out'));});
      if(calls.childElementCount)box.appendChild(calls);
      if(uses.childElementCount){uses.querySelectorAll('.map-reading-relation').forEach(function(word){word.remove();});box.append(rmEl('p','map-reading-uses',rmT('Uses variables')),uses);}
      outgoing.appendChild(box);
    });
    view.appendChild(outgoing);
  }
  return {view:view};
}

// A function's writes or reads, "Writes:" then each field or global
// variable once, one to a line, its name reading the type declaring the
// field or the variable.
function rmPathsList(ctx,data,cls,label,paths){
  var box=rmEl('div','map-reading-paths '+cls);box.appendChild(rmEl('p','map-reading-label',rmT(label)));
  var list=rmEl('ul','map-reading-ends');
  paths.forEach(function(item){
    var decl=item.decl===undefined?null:data.decls[item.decl],li=rmEl('li');
    li.appendChild(decl?rmDeclName(decl,item.path,ctx.goDecl(decl),decl.at):rmEl('span','',item.path));list.appendChild(li);
  });
  box.appendChild(list);
  return box;
}
// A declaration's reading: who calls it, by part; its name, the link into
// its code, with its file and the comment its author wrote above it; the
// model's line; what it writes and reads; a type's fields and the functions
// returning or taking it; what it calls, by part.
function rmDeclView(ctx,node,data,concept){
  var key=repomapMembers.sourceKey(concept.source),view=rmEl('div','map-decl-reading'),position=data.decls.findIndex(function(decl){return decl.key===key;});
  // A declaration the part's reading does not list (neither a function, a
  // type nor a module's variable) is read by its name and code alone.
  var decl=position>=0?data.decls[position]:{name:concept.name,key:key,href:concept.source.Href,open:concept.source.Open,at:concept.source.Text,file:concept.source.Path};
  var own=(data.own||[]).find(function(entry){return entry.decl===position;})||{},symbols=[],explanation={text:concept.explanation,ref:concept.explanation_ref};
  var variable=decl.kind==='variable'||decl.kind==='type';
  function side(groups,heading,which){
    if(!groups||!groups.length)return null;
    var section=rmEl('section','map-reading-side');section.appendChild(rmEl('h6','',rmT(heading)));
    // A variable a hundred functions read folds each larger part to its
    // line: redisServer's server had a reading 32,000 px tall.
    var total=groups.reduce(function(sum,group){return sum+group.decls.length;},0);
    groups.forEach(function(group){
      var fold=total>30&&group.decls.length>5,box=rmEl(fold?'details':'div','map-reading-peer'),head=rmEl(fold?'summary':'div','map-reading-peer-head');
      // Another program's calls into a declaration both hold are named by
      // that program: "redis-cli: [Command line client] cliConnect()".
      if(group.program)head.appendChild(rmDotBreaks(rmEl('span','map-reading-program',group.program+':')));
      head.appendChild(rmPartBox(ctx,group.part,group.title));box.appendChild(head);
      var list=rmEl('ul','map-reading-ends');group.decls.forEach(function(end){list.appendChild(rmEndItem(ctx,data,end,which,variable));});
      box.appendChild(list);section.appendChild(box);
    });
    return section;
  }
  var callers=side(own.callers,variable?'Used by':'Called by','in');if(callers)view.appendChild(callers);
  // Its own program never runs it, while another program does.
  if(own.not_called_in)view.appendChild(rmEl('p','map-reading-not-called meta',rmT('Not called in {0}',own.not_called_in)));
  var name=rmEl('div','map-decl-name');
  var link=rmDeclName({name:decl.name,href:decl.href,open:decl.open,code:decl.code},decl.name,null,decl.at,false,null);link.classList.add('map-decl-code');name.appendChild(link);
  symbols=rmPage.data(node,'symbols')||[];
  var symbol=symbols.find(function(s){return (s.href||s.open)===key&&s.kind!=='field';});
  if(symbol&&symbol.text)name.appendChild(rmEl('span','map-decl-signature',symbol.text));
  view.appendChild(name);
  var where=rmEl('p','map-decl-where');where.appendChild(rmEl('span','meta',decl.file||''));
  view.appendChild(where);
  // The author's comment above it, quoted as written in its reading and
  // marked as theirs (owner, 2026-09-28: it had waited behind a "comment"
  // hover).
  if(decl.doc){
    var comment=rmEl('blockquote','map-author-comment');comment.setAttribute('role','note');
    comment.append(rmEl('small','',rmT("The author's comment in the code")),rmEl('q','',decl.doc));
    view.appendChild(comment);
  }
  if(explanation&&explanation.text)view.appendChild(rmModelText('p','map-decl-explanation',explanation.text,explanation.ref));
  // What it writes, then what else it reads, under one closed "Reads and
  // writes" (owner, 2026-09-29: processCommand's twenty fields had stood
  // between its comment and its calls).
  if((own.writes||[]).length||(own.reads||[]).length){
    var paths=rmEl('details','map-reading-rw');paths.appendChild(rmEl('summary','',rmT('Reads and writes')));
    if((own.writes||[]).length)paths.appendChild(rmPathsList(ctx,data,'map-reading-writes','Writes:',own.writes));
    if((own.reads||[]).length)paths.appendChild(rmPathsList(ctx,data,'map-reading-reads','Reads:',own.reads));
    view.appendChild(paths);
  }
  // A record type's fields, each with who writes and reads it; a global
  // variable's fields as the code reaches them through it.
  var uses={};(own.fields||[]).forEach(function(use){uses[use.name]=use;});
  function usesRow(grid,use){if(!use)return;var row=rmEl('dd','map-field-uses-row');row.appendChild(rmFieldUses(ctx,data,use));grid.appendChild(row);}
  if(decl.fields&&decl.fields.length){
    var fields=rmEl('section','map-reading-fields');fields.appendChild(rmEl('h6','map-reading-count',rmT('Fields')));
    var grid=rmEl('dl','map-reading-field-grid');
    decl.fields.forEach(function(field){
      var term=rmEl('dt');term.appendChild(rmDeclName({href:field.href,open:field.open,name:field.name},field.name,null,field.at));
      // A field's type reads that type when it is the repository's.
      var typeDecl=field.type_decl===undefined?null:data.decls[field.type_decl],typed=rmEl('dd');
      if(typeDecl)typed.appendChild(rmDeclName(typeDecl,field.type||typeDecl.name,ctx.goDecl(typeDecl),typeDecl.at));else typed.textContent=field.type||'';
      grid.append(term,typed);usesRow(grid,uses[field.name]);
    });
    fields.appendChild(grid);view.appendChild(fields);
  }else if(decl.kind==='variable'&&(own.fields||[]).length){
    var through=rmEl('section','map-reading-fields');through.appendChild(rmEl('h6','map-reading-count',rmT('Fields')));
    var list=rmEl('dl','map-reading-field-grid');
    own.fields.forEach(function(use){list.appendChild(rmEl('dt','map-field-path',use.name));usesRow(list,use);});
    through.appendChild(list);view.appendChild(through);
  }
  [['returns','Returned by'],['takes','Taken by']].forEach(function(pair){
    if(!(own[pair[0]]||[]).length)return;
    var line=rmEl('section','map-reading-side');line.appendChild(rmEl('h6','',rmT(pair[1])));
    var list=rmEl('ul','map-reading-ends');
    own[pair[0]].forEach(function(at){var item=rmEl('li');item.appendChild(rmDeclName(data.decls[at],rmCallableName(data.decls[at]),ctx.goDecl(data.decls[at]),data.decls[at].at));list.appendChild(item);});
    line.appendChild(list);view.appendChild(line);
  });
  // A function's calls read as its flow, in the order they are written
  // (32-flow.js); what else it relates to stays by part.
  var flowed=(own.flow||[]).length>0,callKinds={calls:1,passes_callback:1,executes:1,invokes_external:1};
  if(flowed){
    var flow=rmEl('section','map-reading-flow'),headline=rmEl('div','map-flow-headline');headline.append(rmEl('h6','',rmT('Calls')),rmFlowToggle(null));
    flow.append(headline,rmFlowTree(ctx,data,own));view.appendChild(flow);
  }
  var rest=flowed?(own.callees||[]).map(function(group){return Object.assign({},group,{decls:group.decls.filter(function(end){return !callKinds[end.kind];})});}).filter(function(group){return group.decls.length;}):own.callees;
  var callees=side(rest,variable?'Uses':'Calls','out');if(callees)view.appendChild(callees);
  return view;
}

// An Inputs collection's reading: its component first, in its box, then
// each catalogue under a short heading with where its inputs are declared
// and read, and its inputs as a grid of names; then the inputs no
// catalogue holds, by kind. A name reads its input.
// A component's language as its reading names it.
var rmLanguageNames={c:'C',go:'Go',python:'Python',javascript:'JavaScript',typescript:'TypeScript',clojure:'Clojure'};
// An operation's kind as the reading names it: every kind GroupsIndex gives
// (groupindex.OperationKind), an extension point and a queue consumer
// included; an entry whose kind the reading did not establish (entry) says
// so, never "Inputs" (litestream's vfs had read "Inputs: … · Inputs").
var rmInputKindTitles={request:'Incoming requests',command:'Commands',setting:'Settings',interaction:'User interactions',scheduled:'Scheduled tasks',continuous:'Background work',consumer:'Queue consumers',extension:'Extension points',entry:'Kind not established'};
function rmCollectionView(ctx,node,collection){
  var view=rmEl('div','map-collection-reading');
  var component=ctx.nodeById('system-component-'+node.dataset.owner);
  if(component){var head=rmEl('div','map-part-title');head.appendChild(rmPartBox(ctx,component.getAttribute('href'),component.dataset.title));head.firstChild.classList.add('map-part-box-component');view.appendChild(head);}
  // One section per kind, in the order the kinds come (owner, 2026-09-30:
  // litestream's inputs had read "Incoming requests" once per declaring
  // function, eight times); each catalogue of it under a quiet line saying
  // where its inputs are declared, then its inputs.
  var kinds=[];collection.groups.forEach(function(group){if(kinds.indexOf(group.kind)<0)kinds.push(group.kind);});
  kinds.forEach(function(kind){
    var groups=collection.groups.filter(function(group){return group.kind===kind;}),all=[].concat.apply([],groups.map(function(group){return group.inputs;}));
    var section=rmEl('section','map-collection-group'),heading=rmEl('h6','map-reading-count');
    // A kind chosen in the component's reading, or Settings in the
    // collection's frame, lands on its own section: Background work on the
    // first of its scheduled and continuous sections.
    section.dataset.kind=kind;
    if(rmPendingKind&&[].concat(rmPendingKind).indexOf(kind)>=0){section.dataset.readingAnchor='';rmPendingKind='';}
    heading.appendChild(rmEl('span','',rmT(rmInputKindTitles[kind]||'Inputs')));
    rmLights(ctx,heading,all);section.appendChild(heading);
    groups.forEach(function(group){
      var box=rmEl('div','map-collection-catalogue');
      var first=group.catalogue&&ctx.nodeById(group.catalogue),catalogue=first?rmPage.data(first,'catalogue'):null;
      if(catalogue){var where=rmEl('div','map-collection-where');rmCatalogueLines(ctx,catalogue).forEach(function(line){where.appendChild(line);});box.appendChild(where);}
      // Matched by the model to another program's inputs of the same name:
      // one line, the model's.
      var matched=0,programs=[];
      group.inputs.forEach(function(id){
        var input=ctx.nodeById(id),path=input?rmPage.data(input,'inputPath'):null;
        if(path&&(path.sent_to||[]).length){matched++;path.sent_to.forEach(function(entry){if(programs.indexOf(entry.program)<0)programs.push(entry.program);});}
      });
      if(matched)box.appendChild(rmModelText('p','map-collection-matched',rmT('Matched to inputs of {0} by name',programs.join(', '))));
      var names=rmEl('ul','map-collection-names');
      group.inputs.forEach(function(id){
        var input=ctx.nodeById(id);if(!input)return;
        // A directive with its values: "appendfsync: always | everysec | no".
        var path=rmPage.data(input,'inputPath'),values=path&&path.values?(path.checks||[]).map(function(check){return check.name;}):[];
        var title=input.dataset.title+(values.length?': '+values.join(' | '):'');
        var item=rmEl('li'),button=rmDotBreaks(rmEl('button','',title));button.type='button';
        // A name that is a sentence (a query parameter's description) is
        // prose, not code.
        if(rmProse(input.dataset.title))button.classList.add('map-collection-prose');
        button.addEventListener('click',function(){ctx.light([]);ctx.readNode(input);});rmLights(ctx,button,[id]);item.appendChild(button);names.appendChild(item);
      });
      box.appendChild(names);section.appendChild(box);
    });
    view.appendChild(section);
  });
  if(collection.groups.some(function(group){return group.catalogue;}))view.appendChild(rmEl('p','meta',rmT('Where these take effect is not established.')));
  rmPendingKind='';
  return view;
}
// A name of three words or more with no path, brace, dot or underscore in
// it is a sentence ("Number of months to fetch data for").
function rmProse(text){return /\S\s+\S+\s+\S/.test(text||'')&&!/[\/{}._$\\]/.test(text);}
// Pointing at inputs in the column lights their tiles on the canvas, and
// nothing recedes.
function rmLights(ctx,element,ids){
  function on(){ctx.light(ids);}function off(){ctx.light([]);}
  element.addEventListener('mouseenter',on);element.addEventListener('focus',on);element.addEventListener('mouseleave',off);element.addEventListener('blur',off);
}
// Where a catalogue's inputs are declared and read, one line each: "listed
// in cmdTable", "looked up in lookupCommand ← cliSendCommand, main", "uses
// config, also used by main, repl". Names read their declarations.
function rmCatalogueLines(ctx,catalogue){
  var decls=catalogue.decls||[],lines=[];
  function name(index){
    var decl=decls[index]||{name:''},node=decl.part?ctx.nodeById(decl.part):null;
    return rmDeclName({name:decl.name,href:decl.href,open:decl.open,code:decl.code,part:node?node.getAttribute('href'):'',key:decl.href||decl.open},decl.name,node?function(){ctx.readDeclIn(node,decl.href||decl.open);}:null,decl.source);
  }
  // Its callers one to a line under the line naming what they call.
  function callers(calls){var list=rmEl('ul','map-reading-ends map-collection-callers');calls.forEach(function(call){var item=rmEl('li');item.appendChild(name(call.caller));list.appendChild(item);});return list;}
  function line(key,args){
    var text=rmT.apply(null,[key].concat(args.map(function(_,i){return '\u0001'+i+'\u0002';}))),p=rmEl('p','map-collection-line');
    text.split(/(\u0001\d+\u0002)/).forEach(function(piece){var m=/^\u0001(\d+)\u0002$/.exec(piece);if(m)p.append(args[Number(m[1])]);else if(piece)p.append(document.createTextNode(piece));});
    return p;
  }
  if(catalogue.declarer>=0){
    var declared=line(catalogue.table?'listed in {0}':'declared in {0}',[name(catalogue.declarer)]);
    if((catalogue.calls||[]).length)declared.append(document.createTextNode(' ←'));
    lines.push(declared);
    if((catalogue.calls||[]).length)lines.push(callers(catalogue.calls));
  }
  (catalogue.readers||[]).forEach(function(reader){
    var read=line('looked up in {0}',[name(reader.reader)]);
    if((reader.calls||[]).length)read.append(document.createTextNode(' ←'));
    lines.push(read);
    if((reader.calls||[]).length)lines.push(callers(reader.calls));
  });
  // What else the declaring code uses, one folded line; each variable's
  // other users fold under theirs, one to a line (owner, 2026-09-29: the
  // Settings catalogue read as walls of "uses server, also used by …").
  if((catalogue.uses||[]).length){
    var usesFold=rmEl('details','map-collection-uses');usesFold.appendChild(rmEl('summary','',rmT('Also uses variables')));
    catalogue.uses.forEach(function(use){
      var row=line('uses {0}',[name(use.decl)]);
      if((use.users||[]).length){
        var users=rmEl('details','map-collection-users'),head=rmEl('summary','',rmT('also used by'));users.appendChild(head);
        var names=rmEl('ul','map-reading-ends map-collection-callers');use.users.forEach(function(user){var item=rmEl('li');item.appendChild(name(user));names.appendChild(item);});
        users.appendChild(names);row.appendChild(users);
      }
      usesFold.appendChild(row);
    });
    lines.push(usesFold);
  }
  return lines;
}
// A component's reading (owner, 2026-09-29, after the critic: nine
// sections had run to ten screens): its role and purpose, the model's;
// where its program starts, one link per entrypoint name; the kinds of its
// inputs, each lighting its tiles on the canvas while pointed at and
// reading the collection when chosen; its Main flow with what it runs on
// its own; the files it reaches; its connections (29-operation-view.js);
// then one line of links, its whole page first. Its areas and parts are
// the canvas's; what its entrypoints do not reach, its TODOs and its
// analysis coverage are the "What is missing" page's. No counts.
// The kind whose section a kind chosen in a component's reading, or the
// kinds whose first section a kind chosen in a collection's frame, lands
// on, and whether the column's "Main flow" link asked for its Main flow.
var rmPendingKind='',rmPendingFlow=false;
// A section of so few lines stands open in a reading.
var rmShortSection=8;
// The files a component's program reaches by their paths (owner,
// 2026-09-29; page_data_files.go): each by its path as written, or the
// paths its field's writes store and the field (the field in braces when
// they store none), with what else sets it (a setting whose branch writes
// it, a function writing what is not established); each opening to the
// functions whose calls reach it, by part. A file whose path is not
// established is not listed. No role and no line numbers: a name reads its
// function, a path links to where it is written, its place said on hover.
function rmComponentFiles(ctx,data){
  if(!data||!(data.files||[]).length)return null;
  data.decls.forEach(function(decl){if(decl.key===undefined)decl.key=decl.href;});
  function also(term,value){
    if(value.setting){
      term.append(document.createTextNode('; '+rmT('set by the setting')+' '));
      var input=value.input?ctx.nodeById(value.input):null,name=rmEl(input?'button':'code','map-file-setting',value.setting);
      if(input){name.type='button';name.addEventListener('click',function(event){event.stopPropagation();ctx.readNode(input);});}
      term.appendChild(name);
    }else if(value.decl!==undefined){
      var decl=data.decls[value.decl];
      term.append(document.createTextNode('; '+rmT('set in')+' '),rmDeclName(decl,rmCallableName(decl),ctx.goDecl(decl),rmEndTitle(ctx,decl)));
    }
  }
  var named=data.files.filter(function(file){return file.path||file.field;});
  if(!named.length)return null;
  var box=rmEl('details','map-component-section map-component-files'),summary=rmEl('summary');
  summary.appendChild(rmEl('span','',rmT('Files')));box.appendChild(summary);
  named.forEach(function(file){
    var by=file.by||[],entry=rmEl(by.length?'details':'div','map-file'),term=rmEl(by.length?'summary':'p','map-field-path');
    if(file.path)term.appendChild(rmDotBreaks(rmEl('code','',file.path)));
    else{
      var paths=(file.values||[]).filter(function(value){return value.value;});
      paths.forEach(function(value,i){
        if(i)term.append(document.createTextNode(' · '));
        term.appendChild(rmDeclName({name:value.value,href:value.href,open:value.open},value.value,null,value.at));
      });
      // With no path its writes store, the field stands for it in braces.
      if(paths.length)term.append(document.createTextNode(' '+rmT('from')+' '),rmEl('code','',file.field));
      else term.appendChild(rmEl('code','','{'+file.field+'}'));
      (file.values||[]).forEach(function(value){if(!value.value)also(term,value);});
    }
    entry.appendChild(term);
    if(by.length)entry.appendChild(rmNamesByPart(ctx,data,by));
    box.appendChild(entry);
  });
  box.open=named.length<=rmShortSection;
  return box;
}
function rmComponentReading(map,n,card,details,collectionNode,anchorEntry){
  var ctx=map.readingContext(),intro=card.querySelector('.map-card-intro'),page=card.querySelector('.map-card-actions>.map-details-link');
  card.querySelector('.map-related-operations')?.remove();card.querySelector('.map-card-evidence')?.remove();card.querySelector('.map-all-members')?.remove();
  card.querySelector('.map-card-actions')?.remove();
  var after=intro.querySelector(':scope>.map-card-summary');
  function place(item){if(after)after.after(item);else intro.prepend(item);after=item;}
  // Where its program starts, one entry to a line.
  var entries=rmPage.data(n,'entries')||[];
  if(entries.length){
    var start=rmEl('div','map-component-entry');
    entries.forEach(function(entry){
      var part=entry.part?ctx.nodeByHref(entry.part):null,line=rmEl('div');
      line.appendChild(rmDeclName({name:entry.name,href:entry.href,open:entry.open,key:entry.key},entry.name+(entry.callable?'()':''),part?function(){ctx.readDeclIn(part,entry.key);}:null,''));
      start.appendChild(line);
    });
    if(anchorEntry)start.dataset.readingAnchor='';
    place(start);
  }
  // A launch point no part holds is named here, with why: the map then
  // draws no entry part.
  details.querySelectorAll(':scope>.component-intro>.component-entry').forEach(function(entry){
    var line=entry.cloneNode(true);if(anchorEntry)line.dataset.readingAnchor='';place(line);
  });
  // The kinds of its inputs, one to a line, in words.
  var collection=collectionNode?rmPage.data(collectionNode,'collection'):null;
  if(collection&&collection.kinds.length){
    var inputs=rmEl('div','map-component-inputs');inputs.appendChild(rmEl('p','map-reading-label',rmT('Inputs')));
    var kinds=rmEl('ul','map-component-input-kinds');
    collection.kinds.forEach(function(kind){
      var item=rmEl('li'),choice=rmEl('button','',rmT(rmInputKindTitles[kind.kind]||'Inputs'));choice.type='button';
      var choiceMark=globalThis.rmKindMark?.(kind.kind);if(choiceMark)choice.prepend(choiceMark);
      rmLights(ctx,choice,kind.inputs);
      choice.addEventListener('click',function(){ctx.light([]);rmPendingKind=kind.kind;ctx.readNode(collectionNode);});
      item.appendChild(choice);kinds.appendChild(item);
    });
    inputs.appendChild(kinds);place(inputs);
  }
  function copy(element){
    var clone=element.cloneNode(true);clone.removeAttribute('id');clone.querySelectorAll('[id]').forEach(function(el){el.removeAttribute('id');});
    clone.querySelectorAll('details').forEach(function(detail){detail.open=true;});
    clone.querySelectorAll('.collapse-label').forEach(function(label){label.remove();});
    return clone;
  }
  // Its Main flow near the top, open (owner, 2026-09-28), each step's name
  // reading that declaration and showing it on the canvas; what the program
  // runs on its own closes it (owner, 2026-09-29: serverCron was not
  // findable from a flow of client commands; page_flow_steps.go ownWork).
  var flow=details.querySelector(':scope>.component-flow'),own=details.querySelector(':scope>.component-own-work');
  if(flow){
    var steps=rmEl('details','map-component-section'),head=rmEl('summary');head.appendChild(rmEl('span','',rmT('Main flow')));steps.appendChild(head);
    Array.from(flow.querySelectorAll(':scope>.flow-title,:scope>p.meta,:scope>ol')).forEach(function(part){steps.appendChild(copy(part));});
    steps.open=true;place(steps);
    // Each step opens in place to its code flow (32-flow.js), the model's
    // sentence kept in its style above it.
    var toggle=rmEl('div','map-flow-headline');toggle.appendChild(rmFlowToggle(null));steps.insertBefore(toggle,steps.children[1]||null);
    if(rmPendingFlow){steps.dataset.readingAnchor='';rmPendingFlow=false;}
    // A step's name is a link into all of its declaration's code, a plain
    // click reading it; the step's line link goes (owner, 2026-09-29: it
    // had opened the line registering the callable, inside another
    // function). Its calls open in place from the step's own twist.
    steps.querySelectorAll('li[data-step-part]').forEach(function(step){
      var part=ctx.nodeByHref(step.dataset.stepPart),code=step.querySelector('.flow-what>code');
      if(!part||!code)return;
      code.replaceChildren(rmStepName(ctx,step,code.textContent));
      step.querySelector(':scope>.anchor')?.remove();
      rmFlowStep(ctx,step,part,rmPage.link(step.dataset.stepKey));
    });
    // A step no part holds is still its name, the link into its code, with
    // no line printed after it.
    steps.querySelectorAll('li.flow-step:not([data-step-part])').forEach(function(step){
      var code=step.querySelector('.flow-what>code'),anchor=step.querySelector(':scope>a.anchor');
      if(!code||!anchor)return;
      code.replaceChildren(rmStepName(ctx,step,code.textContent));
      anchor.remove();
    });
    // A program no model flow passes reads forward from its one entry: its
    // calls stand open under it, in the order they are written (owner,
    // 2026-09-29: redis-cli's Main flow had read only "main in Command line
    // client").
    var entryTwists=steps.querySelectorAll('ol.fact>li>.map-flow-step-twist');
    if(entryTwists.length===1)entryTwists[0].click();
    // Its calls standing open, the entry's line of what its part reaches says
    // them again and goes (owner, 2026-09-30: redis-cli's had run on as
    // "cliConnect calls anetTcpConnect Network sockets · …"); with several
    // entries each reach stands on a line of its own.
    if(entryTwists.length===1)steps.querySelectorAll('ol.fact .flow-reaches').forEach(function(line){line.remove();});
    // A registered callable's step names where it is registered and what
    // runs it, each name reading its declaration.
    rmStepChainNames(ctx,steps);
    if(own)steps.appendChild(rmOwnWork(ctx,own));
  }else if(own)place(rmOwnWork(ctx,own));
  // The files its program reaches, after its flow (rmComponentFiles).
  var files=rmComponentFiles(ctx,rmPage.data(n,'files'));if(files)place(files);
  // Its whole page leads the reading's one line of links (30-map.js).
  if(page){page.textContent=rmT('Component details');var foot=rmEl('p','map-component-page');foot.appendChild(page);card.appendChild(foot);}
}
// A Main flow step's declaration, or one it names beside it: a link into
// all of its code (the step's `data-step-code`, else its line link), whose
// plain click reads the declaration in its part, the part named on hover.
function rmStepName(ctx,element,text){
  var d=element.dataset,part=d.stepPart?ctx.nodeByHref(d.stepPart):null,key=d.stepKey?rmPage.link(d.stepKey):'';
  var anchor=element.querySelector?element.querySelector(':scope>.anchor'):null;
  var code=d.stepCode?rmPage.link(d.stepCode):anchor&&anchor.getAttribute('href')!=='#'?anchor.getAttribute('href')||'':'';
  var open=d.stepOpen||(anchor&&anchor.dataset.open)||'';
  var name=rmDeclName({name:text,href:code,code:code,open:open,key:key},text,part&&key?function(){ctx.readDeclIn(part,key);}:null,part?part.dataset.title:'');
  name.classList.add('map-flow-step-name');
  return name;
}
// The names a step's registration and runners are said with, each read so.
function rmStepChainNames(ctx,holder){
  holder.querySelectorAll('.flow-chain-name').forEach(function(code){
    if(!code.dataset.stepKey&&!code.dataset.stepCode&&!code.dataset.stepOpen)return;
    code.replaceChildren(rmStepName(ctx,code,code.textContent));
  });
}
// "Also runs on its own:", one line each: the callable, where it is
// registered ("registers it" linking the registering call) and what runs
// it, each name read as a step's is; pointing at a line lights its
// input's tile.
function rmOwnWork(ctx,source){
  var box=rmEl('section','map-component-own');box.appendChild(rmEl('p','map-reading-label',rmT('Also runs on its own:')));
  var list=rmEl('ul','map-component-own-list');
  source.querySelectorAll('li').forEach(function(from){list.appendChild(rmOwnWorkLine(ctx,from));});
  box.appendChild(list);
  return box;
}
function rmOwnWorkLine(ctx,from){
  var item=from.cloneNode(true),code=item.querySelector('.flow-what>code');
  if(code&&(item.dataset.stepKey||item.dataset.stepCode||item.dataset.stepOpen))code.replaceChildren(rmStepName(ctx,item,code.textContent));
  rmStepChainNames(ctx,item);
  if(item.dataset.input&&ctx.nodeById(item.dataset.input))rmLights(ctx,item,[item.dataset.input]);
  return item;
}
// The home's table of programs (owner, 2026-09-28), one block each in the
// component's page order: its name, reading it; its role, the model's; its
// entry; the kinds of its inputs, in words; its connections as its arrow
// ends group them;
// and the files it is built from (page data: for C its link line's units),
// a build fact: a model's summary had said all four Redis programs share
// ae, sds, adlist, dict and anet, which their Makefile does not.
function rmProgramsTable(ctx,holder,components,connections){
  if(!holder||!components.length)return;
  var table=rmEl('dl','system-programs-list');
  function line(label,content){if(!content)return;table.append(rmEl('dt','',rmT(label)),content);}
  components.forEach(function(n){
    var head=rmEl('dt','system-program-name'),name=rmDotBreaks(rmEl('button','',n.dataset.title));name.type='button';
    name.addEventListener('click',function(){ctx.readNode(n);});head.appendChild(name);table.appendChild(head);
    var about=rmEl('dd','system-program-about');
    if(n.dataset.role)about.appendChild(rmModelText('span','',n.dataset.role,n.dataset.roleRef));
    table.appendChild(about);
    var entries=rmPage.data(n,'entries')||[];
    if(entries.length){var starts=rmEl('ul','system-program-files');entries.forEach(function(entry){starts.appendChild(rmDotBreaks(rmEl('li','',entry.name+(entry.callable?'()':''))));});var at=rmEl('dd');at.appendChild(starts);line('Entry',at);}
    var collection=ctx.nodeById('system-inputs-'+n.dataset.owner),kinds=(collection&&rmPage.data(collection,'collection')||{}).kinds||[];
    if(kinds.length){
      var kindList=rmEl('dd');
      kinds.forEach(function(kind,i){if(i)kindList.append(' · ');var mark=globalThis.rmKindMark?.(kind.kind);if(mark)kindList.append(mark);kindList.append(rmT(rmInputKindTitles[kind.kind]||'Inputs'));});
      line('Inputs',kindList);
    }
    var ends=connections(n.id);
    // An arrow stays with its name, which breaks only at its dots.
    if(ends.length){var peers=rmEl('ul','system-program-files');ends.forEach(function(end){var item=rmEl('li','',end.incoming?'←\u00a0':'→\u00a0');item.appendChild(rmDotBreaks(rmEl('span','',end.title)));peers.appendChild(item);});var cell=rmEl('dd');cell.appendChild(peers);line('Connections',cell);}
    // Its files one to a line, folded when many; no count.
    var files=rmPage.data(n,'sources')||[];
    if(files.length){
      var built=rmEl('dd'),list=rmEl('ul','system-program-files');
      files.forEach(function(file){list.appendChild(rmDotBreaks(rmEl('li','',file)));});
      if(files.length<=rmShortSection)built.appendChild(list);
      else{var fold=rmEl('details');fold.append(rmEl('summary','',rmT('Files')),list);built.appendChild(fold);}
      line('Built from',built);
    }
  });
  holder.replaceChildren(rmEl('h4','',rmT('Programs')),table);
}
// </reading-column>
