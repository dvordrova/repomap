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
// A declaration's name as a tree writes it: a function with "()".
function rmCallableName(decl){return decl.name+(decl.kind==='function'?'()':'');}
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
  if(title)link.title=title;
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
// A name as the column writes it: it breaks only after a dot, never
// inside a word (litestream's "sql.Tx.Rollbac k"); in a flow's row a single
// word longer than the row ends in "…" (43-map-reading.css).
function rmDotBreaks(element){
  var text=element.textContent;if(text.indexOf('.')<0)return element;
  element.textContent='';
  text.split(/(?<=\.)/).forEach(function(piece,i){if(i)element.appendChild(document.createElement('wbr'));element.appendChild(document.createTextNode(piece));});
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
    say.append(document.createTextNode(' → '+rmT(({request:'{0} request handlers',command:'{0} command handlers'})[fan.noun]||'{0} functions',line.ends.length)));
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
// A heading with its count: "12 functions".
function rmCountHeading(tag,key,count){return rmEl(tag,'map-reading-count',rmT(key,count));}
// Who changes a field and who reads it (owner, 2026-09-29;
// page_field_uses.go): "Written by", then "Read by", each its functions by
// the part they stand in, in the part's box; a long side folds under its
// count. No line numbers: a name reads its declaration.
function rmFieldUses(ctx,data,use){
  var box=rmEl('div','map-field-uses');
  [['written','Written by'],['read','Read by']].forEach(function(pair){
    var groups=use[pair[0]]||[];if(!groups.length)return;
    var count=groups.reduce(function(sum,group){return sum+group.decls.length;},0),long=count>12;
    var side=rmEl(long?'details':'div','map-field-side'),head=rmEl(long?'summary':'span','map-field-side-head',rmT(pair[1]));
    if(long)head.appendChild(rmEl('span','map-reading-peer-count',String(count)));
    side.appendChild(head);
    groups.forEach(function(group){
      var line=rmEl('span','map-field-part');
      if(group.part||group.title)line.appendChild(rmPartBox(ctx,group.part,group.title));
      group.decls.forEach(function(at){var decl=data.decls[at];line.appendChild(rmDeclName(decl,rmCallableName(decl),ctx.goDecl(decl),rmEndTitle(ctx,decl)));});
      side.appendChild(line);
    });
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
  if(long)heading.appendChild(rmEl('span','map-reading-peer-count',String(total)));
  section.appendChild(heading);
  data.groups.forEach(function(group){
    var box=rmEl('div','map-reading-peer'),head=rmEl('div','map-reading-peer-head');
    if(group.program)head.appendChild(rmEl('span','map-reading-program',group.program+':'));
    head.append(rmPartBox(ctx,group.part,group.title),rmEl('span','map-reading-peer-count',String(group.decls.length)));box.appendChild(head);
    var list=rmEl('ul','map-reading-ends');group.decls.forEach(function(end){list.appendChild(rmEndItem(ctx,data,end,'in'));});
    box.appendChild(list);section.appendChild(box);
  });
  return section;
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
  body.querySelectorAll('.outbound-chain').forEach(function(chain){
    var steps=kid(chain,tag('OL'));if(!steps)return;
    Array.prototype.slice.call(steps.children).forEach(function(step){
      var name=kid(step,tag('CODE'));if(name)step.replaceChildren(linked(name.textContent,step.querySelector('.anchor')));
    });
  });
  record.replaceChildren(body);
}

// A part's reading (owner, 2026-09-28): the part in its box, the model's
// description and its files; what it is made of first, its declarations by
// kind and name, the keys bold, by file when it holds several; its
// connections (29-operation-view.js mounts them); the parts calling into it,
// each caller with what it calls here, folded when long; then the parts it
// calls into, each with its callees, and the variables it uses there.
function rmPartView(ctx,node,data){
  var view=rmEl('div','map-part-reading');
  var title=rmEl('div','map-part-title');title.appendChild(rmPartBox(ctx,node.getAttribute('href'),node.dataset.title,true));view.appendChild(title);
  if(node.dataset.summary)view.appendChild(rmModelText('p','map-card-summary',node.dataset.summary,node.dataset.summaryRef));
  var files=data.files||[];
  if(files.length)view.appendChild(rmEl('p','map-part-files meta',files.join(', ')));
  // Those reached from outside the part ("Called from": a caller in
  // another part, an input registered at it, a callable handed over) stand
  // first, counted in the heading, the rest after them (owner, 2026-09-29:
  // Persistence's 40 functions had not said which are its ways in).
  function members(into,file){
    (data.members||[]).forEach(function(kind){
      var decls=kind.decls.filter(function(position){return !file||data.decls[position].file===file;});
      if(!decls.length)return;
      var outside=kind.decls.slice(0,kind.outside||0),reached=decls.filter(function(position){return outside.indexOf(position)>=0;});
      var split=reached.length>0&&reached.length<decls.length;
      var list=rmEl('section','map-reading-members'),heading=rmCountHeading('h6',{function:'{0} functions',type:'{0} types',variable:'{0} variables'}[kind.kind],decls.length);
      if(split)heading.appendChild(rmEl('span','map-reading-reached-count',' · '+rmT('{0} reached from outside',reached.length)));
      list.appendChild(heading);
      function names(positions,cls){
        var ul=rmEl('ul','map-reading-names'+(cls?' '+cls:''));
        positions.forEach(function(position){var decl=data.decls[position],item=rmEl('li');item.appendChild(rmDeclName(decl,decl.name,ctx.goDecl(decl),decl.at,true));ul.appendChild(item);});
        return ul;
      }
      if(split)list.append(names(reached,'map-reading-reached'),names(decls.filter(function(position){return outside.indexOf(position)<0;})));
      else list.appendChild(names(decls));
      into.appendChild(list);
    });
  }
  // A part of several files, as Core data structures is of adlist.c,
  // dict.c, sds.c, zipmap.c and zmalloc.c, lists them file by file.
  if(files.length>1)files.forEach(function(file){
    var box=rmEl('section','map-reading-file');box.appendChild(rmEl('h5','map-reading-file-name',file.split('/').pop()));
    members(box,file);if(box.childElementCount>1)view.appendChild(box);
  });
  else members(view,'');
  if(data.in&&data.in.length){
    // Many callers fold the whole list under its count: Server core state
    // had its own declarations below 2,000 px of them.
    var lines=data.in.reduce(function(sum,peer){return sum+peer.lines.length;},0),long=lines>12;
    var incoming=rmEl(long?'details':'section','map-reading-side map-reading-in'),heading=rmEl(long?'summary':'h6','',rmT('Called from'));
    if(long)heading.appendChild(rmEl('span','map-reading-peer-count',String(lines)));
    incoming.appendChild(heading);
    data.in.forEach(function(peer){
      var box=rmEl(long?'details':'div','map-reading-peer'),head=rmEl(long?'summary':'div','map-reading-peer-head');
      head.append(rmPeerBox(ctx,peer),rmEl('span','map-reading-peer-count',String(peer.count)));box.appendChild(head);
      peer.lines.forEach(function(line){box.appendChild(rmCallerLine(ctx,data,line));});
      incoming.appendChild(box);
    });
    view.appendChild(incoming);
  }
  if(data.out&&data.out.length){
    var outgoing=rmEl('section','map-reading-side map-reading-out');outgoing.appendChild(rmEl('h6','',rmT('Calls into')));
    data.out.forEach(function(peer){
      var box=rmEl('div','map-reading-peer'),head=rmEl('div','map-reading-peer-head');
      head.append(rmPeerBox(ctx,peer),rmEl('span','map-reading-peer-count',String(peer.count)));box.appendChild(head);
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

// A function's writes or reads as one line, "Writes: server.dirty" or
// "Reads: redisClient.argv, shared.czero", each field or global variable
// once, its name reading the type declaring the field or the variable.
function rmPathsLine(ctx,data,cls,label,paths){
  var line=rmEl('p','map-reading-paths '+cls);line.appendChild(rmEl('span','map-reading-label',rmT(label)));
  paths.forEach(function(item,i){
    line.append(document.createTextNode(i?', ':' '));
    var decl=item.decl===undefined?null:data.decls[item.decl];
    line.appendChild(decl?rmDeclName(decl,item.path,ctx.goDecl(decl),decl.at):rmEl('span','',item.path));
  });
  return line;
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
      if(group.program)head.appendChild(rmEl('span','map-reading-program',group.program+':'));
      head.append(rmPartBox(ctx,group.part,group.title),rmEl('span','map-reading-peer-count',String(group.decls.length)));box.appendChild(head);
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
  // What it writes, then what else it reads: "Writes: server.dirty",
  // "Reads: redisClient.argv, shared.czero, …".
  if((own.writes||[]).length)view.appendChild(rmPathsLine(ctx,data,'map-reading-writes','Writes:',own.writes));
  if((own.reads||[]).length)view.appendChild(rmPathsLine(ctx,data,'map-reading-reads','Reads:',own.reads));
  // A record type's fields, each with who writes and reads it; a global
  // variable's fields as the code reaches them through it.
  var uses={};(own.fields||[]).forEach(function(use){uses[use.name]=use;});
  function usesRow(grid,use){if(!use)return;var row=rmEl('dd','map-field-uses-row');row.appendChild(rmFieldUses(ctx,data,use));grid.appendChild(row);}
  if(decl.fields&&decl.fields.length){
    var fields=rmEl('section','map-reading-fields');fields.appendChild(rmCountHeading('h6','{0} fields',decl.fields.length));
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
    var paths=rmEl('section','map-reading-fields');paths.appendChild(rmCountHeading('h6','{0} fields',own.fields.length));
    var list=rmEl('dl','map-reading-field-grid');
    own.fields.forEach(function(use){list.appendChild(rmEl('dt','map-field-path',use.name));usesRow(list,use);});
    paths.appendChild(list);view.appendChild(paths);
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
    var flow=rmEl('section','map-reading-flow'),headline=rmEl('div','map-flow-headline');headline.appendChild(rmFlowToggle(null));
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
var rmInputKindTitles={request:'Incoming requests',command:'Commands',setting:'Settings',interaction:'User interactions',scheduled:'Scheduled tasks',continuous:'Background work'};
function rmCollectionView(ctx,node,collection){
  var view=rmEl('div','map-collection-reading');
  var component=ctx.nodeById('system-component-'+node.dataset.owner);
  if(component){var head=rmEl('div','map-part-title');head.appendChild(rmPartBox(ctx,component.getAttribute('href'),component.dataset.title));head.firstChild.classList.add('map-part-box-component');view.appendChild(head);}
  collection.groups.forEach(function(group){
    var section=rmEl('section','map-collection-group'),heading=rmEl('h6','map-reading-count');
    // "37 settings" in the component's reading, or Settings in the
    // collection's frame, lands on its own section: Background work on the
    // first of its scheduled and continuous sections.
    section.dataset.kind=group.kind;
    if(rmPendingKind&&[].concat(rmPendingKind).indexOf(group.kind)>=0){section.dataset.readingAnchor='';rmPendingKind='';}
    heading.append(rmEl('span','',rmT(rmInputKindTitles[group.kind]||'Inputs')),rmEl('span','map-reading-peer-count',String(group.inputs.length)));
    rmLights(ctx,heading,group.inputs);section.appendChild(heading);
    var first=group.catalogue&&ctx.nodeById(group.catalogue),catalogue=first?rmPage.data(first,'catalogue'):null;
    if(catalogue)rmCatalogueLines(ctx,catalogue).forEach(function(line){section.appendChild(line);});
    // Matched by the model to another program's inputs of the same name:
    // one line, the model's.
    var matched=0,programs=[];
    group.inputs.forEach(function(id){
      var input=ctx.nodeById(id),path=input?rmPage.data(input,'inputPath'):null;
      if(path&&(path.sent_to||[]).length){matched++;path.sent_to.forEach(function(entry){if(programs.indexOf(entry.program)<0)programs.push(entry.program);});}
    });
    if(matched)section.appendChild(rmModelText('p','map-collection-matched',rmT('{0} matched to inputs of {1} by name',matched,programs.join(', '))));
    var names=rmEl('ul','map-collection-names');
    group.inputs.forEach(function(id){
      var input=ctx.nodeById(id);if(!input)return;
      // A directive with its values: "appendfsync: always | everysec | no".
      var path=rmPage.data(input,'inputPath'),values=path&&path.values?(path.checks||[]).map(function(check){return check.name;}):[];
      var item=rmEl('li'),button=rmEl('button','',input.dataset.title+(values.length?': '+values.join(' | '):''));button.type='button';
      button.addEventListener('click',function(){ctx.light([]);ctx.readNode(input);});rmLights(ctx,button,[id]);item.appendChild(button);names.appendChild(item);
    });
    section.appendChild(names);view.appendChild(section);
  });
  if(collection.groups.some(function(group){return group.catalogue;}))view.appendChild(rmEl('p','meta',rmT('Where these take effect is not established.')));
  rmPendingKind='';
  return view;
}
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
  function callers(calls){var span=rmEl('span','map-collection-callers');calls.forEach(function(call,i){if(i)span.append(document.createTextNode(', '));span.appendChild(name(call.caller));});return span;}
  function line(key,args){
    var text=rmT.apply(null,[key].concat(args.map(function(_,i){return '\u0001'+i+'\u0002';}))),p=rmEl('p','map-collection-line');
    text.split(/(\u0001\d+\u0002)/).forEach(function(piece){var m=/^\u0001(\d+)\u0002$/.exec(piece);if(m)p.append(args[Number(m[1])]);else if(piece)p.append(document.createTextNode(piece));});
    return p;
  }
  if(catalogue.declarer>=0){
    var declared=line(catalogue.table?'listed in {0}':'declared in {0}',[name(catalogue.declarer)]);
    if((catalogue.calls||[]).length)declared.append(document.createTextNode(' ← '),callers(catalogue.calls));
    lines.push(declared);
  }
  (catalogue.readers||[]).forEach(function(reader){
    var read=line('looked up in {0}',[name(reader.reader)]);
    if((reader.calls||[]).length)read.append(document.createTextNode(' ← '),callers(reader.calls));
    lines.push(read);
  });
  // What else the declaring code uses, one folded line with its count;
  // each variable's other users fold under theirs (owner, 2026-09-29: the
  // Settings catalogue read as walls of "uses server, also used by …").
  if((catalogue.uses||[]).length){
    var usesFold=rmEl('details','map-collection-uses');usesFold.appendChild(rmEl('summary','',rmT('Also uses {0} variables',catalogue.uses.length)));
    catalogue.uses.forEach(function(use){
      var row=line('uses {0}',[name(use.decl)]);
      if((use.users||[]).length){
        var users=rmEl('details','map-collection-users'),head=rmEl('summary','',rmT('also used by {0}',use.users.length));users.appendChild(head);
        var names=rmEl('span','map-collection-callers');use.users.forEach(function(user,i){if(i)names.append(document.createTextNode(', '));names.appendChild(name(user));});
        users.appendChild(names);row.appendChild(users);
      }
      usesFold.appendChild(row);
    });
    lines.push(usesFold);
  }
  return lines;
}
// A component's reading: its role and purpose, the model's; where its
// program starts, one link per entrypoint name; its inputs counted by kind,
// each count lighting its tiles on the canvas while pointed at and reading
// the collection when chosen; its connections (29-operation-view.js); then
// its main flow, what its program never runs, its TODOs and its analysis
// coverage, each a list opening in place, and a link to its whole page.
// The kind whose section a count chosen in a component's reading, or the
// kinds whose first section a kind chosen in a collection's frame, lands
// on, and whether the column's "Main flow" link asked for its Main flow.
var rmPendingKind='',rmPendingFlow=false;
// A section of so few lines stands open in a reading.
var rmShortSection=8;
// A component's areas and parts: each area with its parts under it, each a
// name in its box that reads it, and its description on one line, the
// whole on hover.
function rmOutline(ctx,component){
  function children(node){return (node.dataset.children||'').split(/\s+/).map(ctx.nodeById).filter(function(child){return child&&!child.dataset.activation&&child.dataset.branch!=='inputs';});}
  var top=children(component);if(!top.length)return null;
  function item(node){
    var li=rmEl('li'),box=rmPartBox(ctx,'#'+node.id,node.dataset.title);li.appendChild(box);
    if(node.dataset.summary){var said=rmModelText('span','map-outline-summary',node.dataset.summary,node.dataset.summaryRef);said.title=node.dataset.summary;li.appendChild(said);}
    var inner=children(node);
    if(inner.length){var list=rmEl('ul','map-outline');inner.forEach(function(child){list.appendChild(item(child));});li.appendChild(list);}
    return li;
  }
  var box=rmEl('section','map-component-outline');box.appendChild(rmEl('h6','',rmT('Areas and parts')));
  var list=rmEl('ul','map-outline');top.forEach(function(node){list.appendChild(item(node));});box.appendChild(list);
  return box;
}
var rmInputKindCounts={request:'{0} requests',command:'{0} commands',setting:'{0} settings',interaction:'{0} user interactions',continuous:'{0} continuous',scheduled:'{0} scheduled'};
function rmComponentReading(map,n,card,details,collectionNode,anchorEntry){
  var ctx=map.readingContext(),intro=card.querySelector('.map-card-intro'),page=card.querySelector('.map-card-actions>.map-details-link');
  card.querySelector('.map-related-operations')?.remove();card.querySelector('.map-card-evidence')?.remove();card.querySelector('.map-all-members')?.remove();
  card.querySelector('.map-card-actions')?.remove();
  var after=intro.querySelector(':scope>.map-card-summary');
  function place(item){if(after)after.after(item);else intro.prepend(item);after=item;}
  var entries=rmPage.data(n,'entries')||[];
  if(entries.length){
    var start=rmEl('p','map-component-entry');
    entries.forEach(function(entry,i){
      if(i)start.append(document.createTextNode(' · '));
      var part=entry.part?ctx.nodeByHref(entry.part):null;
      start.appendChild(rmDeclName({name:entry.name,href:entry.href,open:entry.open,key:entry.key},entry.name+(entry.callable?'()':''),part?function(){ctx.readDeclIn(part,entry.key);}:null,''));
    });
    if(anchorEntry)start.dataset.readingAnchor='';
    place(start);
  }
  // A launch point no part holds is named here, with why: the map then
  // draws no entry part.
  details.querySelectorAll(':scope>.component-intro>.component-entry').forEach(function(entry){
    var line=entry.cloneNode(true);if(anchorEntry)line.dataset.readingAnchor='';place(line);
  });
  var collection=collectionNode?rmPage.data(collectionNode,'collection'):null;
  if(collection&&collection.kinds.length){
    var inputs=rmEl('p','map-component-inputs');inputs.appendChild(rmEl('span','map-reading-label',rmT('Inputs')));
    collection.kinds.forEach(function(kind){
      var count=rmEl('button','',rmT(rmInputKindCounts[kind.kind]||'{0} inputs',kind.inputs.length));count.type='button';
      rmLights(ctx,count,kind.inputs);
      count.addEventListener('click',function(){ctx.light([]);rmPendingKind=kind.kind;ctx.readNode(collectionNode);});
      inputs.appendChild(count);
    });
    place(inputs);
  }
  function copy(element){
    var clone=element.cloneNode(true);clone.removeAttribute('id');clone.querySelectorAll('[id]').forEach(function(el){el.removeAttribute('id');});
    clone.querySelectorAll('details').forEach(function(detail){detail.open=true;});
    clone.querySelectorAll('.collapse-label').forEach(function(label){label.remove();});
    return clone;
  }
  // A section of the reading, a list opening in place; a short one stands
  // open (owner, 2026-09-28).
  function section(title,count,parts,open,at){
    if(!parts.length)return null;
    var box=rmEl('details','map-component-section'),summary=rmEl('summary');summary.appendChild(rmEl('span','',title));
    if(count)summary.appendChild(rmEl('span','map-reading-peer-count',String(count)));
    box.appendChild(summary);parts.forEach(function(part){box.appendChild(copy(part));});
    box.open=!!open||box.querySelectorAll('li').length<=rmShortSection;
    if(at)at(box);else card.appendChild(box);
    return box;
  }
  // Its Main flow near the top, open (owner, 2026-09-28), each step's name
  // reading that declaration and showing it on the canvas.
  var flow=details.querySelector(':scope>.component-flow');
  if(flow){
    var steps=section(rmT('Main flow'),0,Array.from(flow.querySelectorAll(':scope>.flow-title,:scope>p.meta,:scope>ol')),true,place);
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
    // A registered callable's step names where it is registered and what
    // runs it, each name reading its declaration.
    rmStepChainNames(ctx,steps);
  }
  // What the program runs on its own, right after its Main flow (owner,
  // 2026-09-29: serverCron was not findable from a flow of client
  // commands; page_flow_steps.go ownWork).
  var own=details.querySelector(':scope>.component-own-work');
  if(own)place(rmOwnWork(ctx,own));
  // Its areas and parts, each a name that reads it, with its description on
  // one line (owner, 2026-09-28).
  var outline=rmOutline(ctx,n);if(outline)place(outline);
  var dead=details.querySelector(':scope>.component-reference h3[id$="-dead"]');
  if(dead){var unreached=[];for(var at=dead.nextElementSibling;at;at=at.nextElementSibling)unreached.push(at);section(rmT('Not reachable from the entrypoints'),0,unreached);}
  var todos=details.querySelector(':scope>.component-reference h3[id$="-todos"]');
  if(todos&&todos.nextElementSibling)section(rmT('TODOs'),todos.nextElementSibling.querySelectorAll('.todo-rows>li').length,Array.from(todos.nextElementSibling.children).filter(function(child){return child.tagName!=='SUMMARY';}));
  var coverage=details.querySelector(':scope>.component-reference .component-coverage');
  if(coverage)section(rmT('Analysis coverage'),coverage.querySelectorAll('li').length,Array.from(coverage.children).filter(function(child){return child.tagName!=='SUMMARY';}));
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
// entry; its inputs by kind; its connections as its arrow ends group them;
// and the files it is built from (page data: for C its link line's units),
// a build fact: a model's summary had said all four Redis programs share
// ae, sds, adlist, dict and anet, which their Makefile does not.
function rmProgramsTable(ctx,holder,components,connections){
  if(!holder||!components.length)return;
  var table=rmEl('dl','system-programs-list');
  function line(label,content){if(!content)return;table.append(rmEl('dt','',rmT(label)),content);}
  components.forEach(function(n){
    var head=rmEl('dt','system-program-name'),name=rmEl('button','',n.dataset.title);name.type='button';
    name.addEventListener('click',function(){ctx.readNode(n);});head.appendChild(name);table.appendChild(head);
    var about=rmEl('dd','system-program-about');
    if(n.dataset.role)about.appendChild(rmModelText('span','',n.dataset.role,n.dataset.roleRef));
    table.appendChild(about);
    var entries=rmPage.data(n,'entries')||[];
    if(entries.length)line('Entry',rmEl('dd','',entries.map(function(entry){return entry.name+(entry.callable?'()':'');}).join(' · ')));
    var collection=ctx.nodeById('system-inputs-'+n.dataset.owner),kinds=(collection&&rmPage.data(collection,'collection')||{}).kinds||[];
    if(kinds.length)line('Inputs',rmEl('dd','',kinds.map(function(kind){return rmT(rmInputKindCounts[kind.kind]||'{0} inputs',kind.inputs.length);}).join(' · ')));
    var ends=connections(n.id);
    if(ends.length)line('Connections',rmEl('dd','',ends.map(function(end){return (end.incoming?'← ':'→ ')+end.title;}).join(' · ')));
    var files=rmPage.data(n,'sources')||[];
    if(files.length){
      var built=rmEl('dd');
      if(files.length<=rmShortSection*2)built.appendChild(rmEl('code','',files.join(' ')));
      else{var fold=rmEl('details');fold.append(rmEl('summary','',rmT('{0} files',files.length)),rmEl('code','',files.join(' ')));built.appendChild(fold);}
      line('Built from',built);
    }
  });
  holder.replaceChildren(rmEl('h4','',rmT('Programs')),table);
}
// </reading-column>
