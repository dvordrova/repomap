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
    (group.rmReading.decls||[]).forEach(function(decl){if(decl.key===undefined)decl.key=decl.href;});
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
// it is a callable written inline, named in words ("anonymous function in
// ReplicateCommand.Run", its anonymous field).
function rmCallableName(decl){return decl.name+(decl.kind==='function'&&!decl.anonymous?'()':'');}
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
  var link=decl.href||decl.open||decl.no_source?repomapMembers.sourceLink({Href:decl.code||decl.href,Open:decl.open,Text:text,NoSource:decl.no_source}):rmEl('span','',text);
  link.classList.add('map-reading-name');rmDotBreaks(link);link.rmDecl=decl;
  if(decl.key)link.dataset.declKey=decl.key;
  if(bold&&decl.bold)link.classList.add('map-reading-key');
  // A clipped name keeps its whole spelling on its hover, above the rest.
  if(title)link.title=link.title&&link.title!==title?link.title+'\n'+title:title;
  if(go){
    link.addEventListener('click',function(event){
      if(event.button||event.metaKey||event.ctrlKey||event.shiftKey||event.altKey)return;
      event.preventDefault();event.stopPropagation();go(decl);
    });
    // The keyboard reads it as a click does: Enter, and Space, on a link as
    // on a name with no source link, which is then a button (control
    // review, 2026-10-02: a page with no remote had its names out of reach).
    var native=link.tagName==='A'||link.tagName==='BUTTON';
    if(!native){link.setAttribute('role','button');link.tabIndex=0;}
    link.addEventListener('keydown',function(event){
      if(event.metaKey||event.ctrlKey||event.altKey||!(event.key===' '||event.key==='Enter'&&!native))return;
      event.preventDefault();event.stopPropagation();go(decl);
    });
  }
  return rmNameIcon(link,code===undefined?{href:decl.code||decl.href,open:decl.open}:code);
}
// Two declarations a list names alike are told apart by where they stand
// (reviewer, 2026-09-30: litestream's "ReplicaClient" had stood twice in one
// list, one per package): each gets its file's folder, or its file when the
// folders are one, quiet after its name. No line is printed.
function rmTellApart(list){
  var byText=new Map();
  Array.prototype.forEach.call(list.children,function(item){
    var name=Array.prototype.find.call(item.children||[],function(child){return child.rmDecl;});if(!name)return;
    var text=name.textContent;if(!byText.has(text))byText.set(text,[]);byText.get(text).push(name);
  });
  byText.forEach(function(names){
    // A declaration nothing where it stands tells apart carries, saved, what
    // only it of those named alike uses (tellDeclsApartByUse).
    if(names.length>1)names.forEach(function(name){var words=rmApartWords(name.rmDecl.apart);if(words)name.after(words);});
    var files=names.map(function(name){return name.rmDecl.file||'';});
    if(names.length<2||new Set(files).size<2)return;
    var folders=files.map(function(file){var at=file.lastIndexOf('/');return at>0?file.slice(0,at):'';}),apart=new Set(folders).size===names.length;
    names.forEach(function(name,i){var where=apart?folders[i]||files[i]:files[i];name.after(rmEl('span','map-reading-where',where));});
  });
  return list;
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
// "redis-" / "benchmark.c"). A piece longer than its line is never cut
// (reviewer, 2026-09-30: thirteen names had ended in "…"): it may also
// break after its underscores, and one with none at its words' humps
// ("zunionInter" / "BlockClient…"), each part of it whole; the whole name
// stays on its hover. A callable written inline comes named in words
// ("anonymous function in ReplicateCommand.Run"; Go's inlineWords, from
// GroupsIndex's fields): the column never reads a name back into parts.
var rmLongPiece=24;
// The words saved beside a same-named input's name (page_apart.go), quiet
// after it, each saying on its hover what it is and where it is written.
var rmApartSays={options:'the commands it is an option of',declared:'the declaration declaring it',key:'its registration\'s own word',handler:'a word its handler declares, at {0}',registered:'the function registering it, at {0}',
  calls:'what only it of those named alike calls, at {0}',reads:'what only it of those named alike reads, at {0}'};
function rmApartTitle(words){
  return words.map(function(word){var says=rmApartSays[word.of];return says?word.word+' — '+(says.indexOf('{0}')>=0?rmT(says,word.at||''):rmT(says)):word.word;}).join('\n');
}
function rmApartWords(words){
  if(!words||!words.length)return null;
  var span=rmEl('span','map-reading-where map-input-apart',words.map(function(word){return word.word;}).join(' · '));
  span.title=rmApartTitle(words);
  return span;
}
// A long piece's words: after each underscore, else at each hump.
function rmPieceWords(piece){
  var words=piece.split(/(?<=_)(?=.)/);
  if(words.length===1)words=piece.split(/(?<=[a-z0-9])(?=[A-Z])/);
  return words;
}
function rmDotBreaks(element){
  var text=element.textContent;
  if(!/[.\/-]/.test(text)&&text.length<=rmLongPiece)return element;
  element.textContent='';
  var long=false;
  // Words break at their spaces; a name within them only after its dots
  // and slashes, a long piece within it after its underscores or humps.
  text.split(/(\s+)/).forEach(function(word){
    if(!word)return;
    if(/^\s+$/.test(word)){element.appendChild(document.createTextNode(word));return;}
    word.split(/(?<=[.\/])/).forEach(function(piece,i){
      if(i)element.appendChild(document.createElement('wbr'));
      if(piece.length<=rmLongPiece){element.appendChild(rmEl('span','map-name-piece',piece));return;}
      long=true;
      rmPieceWords(piece).forEach(function(part,j){
        if(j)element.appendChild(document.createElement('wbr'));
        element.appendChild(rmEl('span','map-name-piece map-name-part',part));
      });
    });
  });
  if(long&&!element.title)element.title=text;
  return element;
}
// A long list said by groups (reviewer, 2026-09-30: lists of up to 336
// rows had stood under one heading): past rmLongList items and with two
// keys or more, each key's items under a closed fold named by the key, in
// the order the keys first come, the items with no key after them; else
// the one list. `render(items)` draws a list's elements.
var rmLongList=40;
function rmFoldBy(items,keyOf,render){
  if(items.length<=rmLongList)return render(items);
  var keys=[],by=new Map(),loose=[];
  items.forEach(function(item){var key=keyOf(item);if(!key){loose.push(item);return;}if(!by.has(key)){by.set(key,[]);keys.push(key);}by.get(key).push(item);});
  if(keys.length<2)return render(items);
  var out=[];
  keys.forEach(function(key){var fold=rmEl('details','map-reading-group');fold.appendChild(rmEl('summary','',key));render(by.get(key)).forEach(function(part){fold.appendChild(part);});out.push(fold);});
  if(loose.length)out=out.concat(render(loose));
  return out;
}
// The group a declaration's name puts it in: a method or a field its type
// ("VFSFile" for VFSFile.Lock), anything else its kind.
var rmKindPlurals={function:'Functions',type:'Types',variable:'Variables',field:'Fields'};
function rmNameGroup(decl){
  var name=String((decl||{}).name||'').replace(/\(\)$/,''),at=name.lastIndexOf('.');
  return at>0?name.slice(0,at):rmT(rmKindPlurals[(decl||{}).kind]||'Declarations');
}
// The file a declaration stands in, by its name.
function rmFileGroup(decl){return String((decl||{}).file||'').split('/').pop();}
// The part a declaration of a page's data stands in, by its title.
function rmPartTitle(ctx,part){var node=part?(part.charAt(0)==='#'?ctx.nodeByHref(part):ctx.nodeById(part)):null;return node?node.dataset.title:'';}
// What a click chose stands marked where the column reads it (owner,
// 2026-09-30: "я в колонке не вижу, что я тыкнул на канвасе"): a short
// pulse, then a steady mark while it stays read; without motion, the
// steady mark alone (43-map-reading.css).
function rmPick(element){if(element&&element.classList)element.classList.add('map-reading-picked');return element;}
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
// A list of ends said by runs, in plain words (reviewer, 2026-09-30: "—
// passed as a callback" had followed each of thirteen names, and Client
// I/O's "Calls into" had listed each command twice, as a possible call and
// as a callback): each name once, with every relation it has here; the
// names of one set of relations stand together, in their order, and a run
// of two or more says its relations once on a quiet line above it
// ("possibly called, passed as callbacks:"); a single name keeps its words
// on its row; plain calls say nothing. A list of callers (`side` "in")
// says it of them ("may call it:"). With `head`, a list that is one run of
// one relation leaves its words (`said`) for its caller's line; `quiet`
// leaves a variable's readers unsaid ("Used by" says it).
var rmRunWords={
  out:{called:'called',possible:'possibly called',passes_callback:'passed as callbacks',binds_implementation:'supplied as implementations',decorates:'decorators',executes:'run',
    imports:'imported',includes:'included',implements:'implemented',reads:'read',writes:'written',sources:'sourced',integration:'receive the connection'},
  in:{called:'call it',possible:'may call it',passes_callback:'pass it as a callback',binds_implementation:'supply it as an implementation',decorates:'decorate it',executes:'run it',
    imports:'import it',includes:'include it',implements:'implement it',reads:'read it',writes:'write it',sources:'source it',integration:'connect to it'}
};
var rmGroupWords={possible:'may call these',passes_callback:'passes these as callbacks',binds_implementation:'supplies these as implementations',decorates:'decorates these',executes:'runs these',
  imports:'imports these',includes:'includes these',implements:'implements these',reads:'reads these',writes:'writes these',sources:'sources these',integration:'connects to these'};
function rmEndRuns(ctx,data,ends,side,head,quiet){
  var names=[],byDecl=new Map();
  ends.forEach(function(end){
    var entry=byDecl.get(end.decl),kind=end.kind||'calls',call=kind==='calls'||kind==='invokes_external';
    if(!entry){entry={end:end,words:[]};byDecl.set(end.decl,entry);names.push(entry);}
    var word=call?(end.possible&&entry.words.indexOf('called')<0?'possible':'called'):kind;
    if(word==='called')entry.words=entry.words.filter(function(other){return other!=='possible';});
    if(entry.words.indexOf(word)<0)entry.words.push(word);
  });
  // A name only called says nothing; called and more, "called" is said.
  names.forEach(function(entry){
    var order=Object.keys(rmRunWords.out);
    if(quiet)entry.words=entry.words.filter(function(word){return word!=='reads';});
    entry.words.sort(function(a,b){return order.indexOf(a)-order.indexOf(b);});
    if(!entry.words.length||entry.words.length===1&&entry.words[0]==='called')entry.words=[];
    entry.key=entry.words.join(' ');
  });
  // Plain calls first, then each run in the order its first name comes.
  var keys=[];names.forEach(function(entry){if(keys.indexOf(entry.key)<0)keys.push(entry.key);});
  if(keys.indexOf('')>0){keys.splice(keys.indexOf(''),1);keys.unshift('');}
  var runs=keys.map(function(key){return names.filter(function(entry){return entry.key===key;});});
  var one=head&&runs.length===1&&runs[0].length>1&&runs[0][0].words.length===1&&rmGroupWords[runs[0][0].words[0]];
  var parts=[];
  runs.forEach(function(run){
    var words=run[0].words,list=rmEl('ul','map-reading-ends');
    if(run.length>1&&words.length&&!one)parts.push(rmEl('p','map-reading-run',words.map(function(word){return rmT(rmRunWords[side][word]);}).join(', ')+':'));
    run.forEach(function(entry){
      var decl=data.decls[entry.end.decl],item=rmEl('li'),site=side==='in'&&entry.end.site;
      item.appendChild(rmDeclName(decl,rmCallableName(decl),ctx.goDecl(decl),rmEndTitle(ctx,decl),false,site?{href:site.href,open:site.open,title:site.at}:undefined));
      if(run.length===1)words.forEach(function(word){
        if(word==='possible')item.appendChild(rmEl('span','possible',rmT('possible')));
        else if(word!=='called'&&rmEndWords[side][word])item.appendChild(rmEl('span','map-reading-relation',rmT(rmEndWords[side][word])));
      });
      list.appendChild(item);
    });
    parts.push(rmTellApart(list));
  });
  return {parts:parts,said:one||''};
}
// One caller and what it reaches in the part, in plain words: a caller
// reaching many of its declarations through one dispatch site is one line,
// "loadAppendOnlyFile() calls one of these request handlers through
// cmdTable", its ends folded under it ("one of these" says they are
// possible); a caller whose ends are one run says it on its own line,
// "cmdTable passes these as callbacks".
var rmFanWords={request:'calls one of these request handlers',command:'calls one of these command handlers'};
function rmCallerLine(ctx,data,line){
  var caller=data.decls[line.caller],row=rmEl(line.fan?'details':'div','map-reading-caller'),head=line.fan?rmEl('summary'):row;
  head.appendChild(rmDeclName(caller,rmCallableName(caller),ctx.goDecl(caller),rmEndTitle(ctx,caller)));
  var runs=rmEndRuns(ctx,data,line.fan?(line.ends||[]).map(function(end){return Object.assign({},end,{possible:false});}):line.ends,'out',!line.fan);
  if(line.fan){
    var fan=line.fan,say=rmEl('span','map-reading-fan'),via=(fan.via||[]).map(function(at){return data.decls[at];}).filter(Boolean);
    // A fan with no dispatch site names none (litestream's r.Client.WriteLTXFile
    // calls one of its eight replica clients): its words take no parameter.
    var key=rmFanWords[fan.noun]||'calls one of these',words=(via.length?rmT(key+' through {0}','\u0001'):rmT(key)).split('\u0001');
    say.append(document.createTextNode(' '+words[0]));
    if(via.length){
      via.forEach(function(decl,i){if(i)say.append(document.createTextNode(', '));say.appendChild(rmDeclName(decl,decl.name,ctx.goDecl(decl),decl.at));});
      say.append(document.createTextNode(words[1]||''));
    }
    head.appendChild(say);row.appendChild(head);
  }else if(runs.said)head.appendChild(rmEl('span','map-reading-fan',' '+rmT(runs.said)));
  runs.parts.forEach(function(part){row.appendChild(part);});
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
    (group.decls||[]).forEach(function(at){var decl=data.decls[at],item=rmEl('li');item.appendChild(rmDeclName(decl,rmCallableName(decl),ctx.goDecl(decl),rmEndTitle(ctx,decl)));list.appendChild(item);});
    peer.appendChild(rmTellApart(list));box.appendChild(peer);
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
    var count=groups.reduce(function(sum,group){return sum+(group.decls||[]).length;},0),long=count>12;
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
// Where the call is written stands before it, "Made in", the same way
// (`made`: the part the canvas stands its destination by; casdoor's Custom
// Logout Endpoint is made in Core data models' callProviderLogoutUrl and
// called from API controllers' ApiController.Logout). Its `calledFrom`
// says whether a "Called from" stands.
function rmReachedFrom(ctx,data){
  if(!data||!((data.groups||[]).length||(data.made||[]).length))return null;
  (data.decls||[]).forEach(function(decl){if(decl.key===undefined)decl.key=decl.href;});
  function side(groups,label,cls){
    var total=groups.reduce(function(sum,group){return sum+(group.decls||[]).length;},0),long=total>12;
    var section=rmEl(long?'details':'section','map-reading-side map-reading-in '+cls),heading=rmEl(long?'summary':'h6','',rmT(label));
    section.appendChild(heading);
    groups.forEach(function(group){
      var box=rmEl('div','map-reading-peer'),head=rmEl('div','map-reading-peer-head');
      if(group.program)head.appendChild(rmDotBreaks(rmEl('span','map-reading-program',group.program+':')));
      head.appendChild(rmPartBox(ctx,group.part,group.title));box.appendChild(head);
      rmEndRuns(ctx,data,group.decls,'in').parts.forEach(function(part){box.appendChild(part);});section.appendChild(box);
    });
    return section;
  }
  var called=(data.groups||[]).length?side(data.groups,'Called from','outbound-reached'):null;
  if(!(data.made||[]).length){called.calledFrom=true;return called;}
  var both=rmEl('div','outbound-where');both.appendChild(side(data.made,'Made in','outbound-made'));
  if(called)both.appendChild(called);
  both.calledFrom=!!called;
  return both;
}
// Where several outside calls are made and reached from, as one "Made in"
// and one "Called from": each record's `reached` (page data) joined by
// program and part, each declaration once.
function rmMergeReached(list){
  var decls=[],at=new Map(),merged={groups:[],made:[]},byPart={groups:new Map(),made:new Map()};
  list.forEach(function(data){
    if(!data)return;
    ['made','groups'].forEach(function(field){
      (data[field]||[]).forEach(function(group){
        var key=(group.program||'')+'\u0000'+(group.part||group.title||''),into=byPart[field].get(key);
        if(!into){into={part:group.part,title:group.title,program:group.program,decls:[]};byPart[field].set(key,into);merged[field].push(into);}
        (group.decls||[]).forEach(function(end){
          var decl=data.decls[end.decl];if(!decl)return;
          var id=decl.key||decl.href||decl.open||decl.name;
          if(!at.has(id)){at.set(id,decls.length);decls.push(decl);}
          var position=at.get(id);
          if(!(into.decls||[]).some(function(other){return other.decl===position;}))into.decls.push(Object.assign({},end,{decl:position}));
        });
      });
    });
  });
  if(!(merged.groups||[]).length&&!merged.made.length)return null;
  var result={decls:decls,groups:merged.groups};if(merged.made.length)result.made=merged.made;
  return result;
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
    if(reached&&reached.calledFrom&&has(part,'outbound-side')||said&&has(part,'outbound-purpose'))return;
    parts.push(part);
    if(has(part,'outbound-source')||has(part,'outbound-runs')||has(part,'outbound-side'))after=parts.length;
  });
  if(reached)parts.splice(after<0?0:after,0,reached);
  body.replaceChildren.apply(body,parts);
  // How many address sources there are is not counted.
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
  (data.members||[]).forEach(function(kind){(kind.decls||[]).forEach(function(position,i){order.push(position);if(i<(kind.outside||0))outside.add(position);});});
  // The keys stand open; a part with none stands its ways in open instead.
  var keys=order.filter(function(position){return data.decls[position].bold;});
  var shown=keys.length?keys:order.filter(function(position){return outside.has(position);});
  var rest=order.filter(function(position){return shown.indexOf(position)<0;});
  function names(positions,cls){
    var ul=rmEl('ul','map-reading-names'+(cls?' '+cls:''));
    positions.forEach(function(position){var decl=data.decls[position],item=rmEl('li');item.appendChild(rmDeclName(decl,decl.name,ctx.goDecl(decl),decl.at,true));ul.appendChild(item);});
    return rmTellApart(ul);
  }
  // A long list by its types and kinds (rmFoldBy).
  function grouped(into,positions,cls){
    rmFoldBy(positions,function(position){return rmNameGroup(data.decls[position]);},function(list){return [names(list,cls)];}).forEach(function(part){into.appendChild(part);});
  }
  // The ways in first, set apart from the rest by a rule.
  function listed(into,positions){
    var reached=positions.filter(function(position){return outside.has(position);}),other=positions.filter(function(position){return !outside.has(position);});
    if(reached.length&&other.length){grouped(into,reached,'map-reading-reached');grouped(into,other);}
    else grouped(into,positions);
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
    var lines=data.in.reduce(function(sum,peer){return sum+(peer.lines||[]).length;},0),long=lines>12;
    var incoming=rmEl(long?'details':'section','map-reading-side map-reading-in'),heading=rmEl(long?'summary':'h6','',rmT('Called from'));
    incoming.appendChild(heading);
    data.in.forEach(function(peer){
      var box=rmEl(long?'details':'div','map-reading-peer'),head=rmEl(long?'summary':'div','map-reading-peer-head');
      head.appendChild(rmPeerBox(ctx,peer));box.appendChild(head);
      // Many callers of one part by the file each stands in.
      rmFoldBy(peer.lines,function(line){return rmFileGroup(data.decls[line.caller]);},function(lines){return lines.map(function(line){return rmCallerLine(ctx,data,line);});}).forEach(function(part){box.appendChild(part);});
      incoming.appendChild(box);
    });
    view.appendChild(incoming);
  }
  if(data.out&&data.out.length){
    // Many callees fold as many callers do: Client I/O's calls into other
    // parts had run to 8,500 px, one name to a line.
    var names=data.out.reduce(function(sum,peer){return sum+(((peer.lines||[])[0]||{}).ends||[]).length;},0),wide=names>12;
    var outgoing=rmEl(wide?'details':'section','map-reading-side map-reading-out');outgoing.appendChild(rmEl(wide?'summary':'h6','',rmT('Calls into')));
    data.out.forEach(function(peer){
      var box=rmEl(wide?'details':'div','map-reading-peer'),head=rmEl(wide?'summary':'div','map-reading-peer-head');
      head.appendChild(rmPeerBox(ctx,peer));box.appendChild(head);
      var ends=peer.lines[0].ends,calls=ends.filter(function(end){return !rmUsesVariable(end.kind);}),uses=rmEl('ul','map-reading-ends');
      ends.forEach(function(end){if(rmUsesVariable(end.kind))uses.appendChild(rmEndItem(ctx,data,end,'out'));});
      // Many callees of one part by the file each stands in; many variables
      // by their type.
      if(calls.length)rmFoldBy(calls,function(end){return rmFileGroup(data.decls[end.decl]);},function(list){return rmEndRuns(ctx,data,list,'out').parts;}).forEach(function(part){box.appendChild(part);});
      if(uses.childElementCount){
        uses.querySelectorAll('.map-reading-relation').forEach(function(word){word.remove();});box.appendChild(rmEl('p','map-reading-uses',rmT('Uses variables')));
        // A variable read and written is one variable used: the list says
        // no relation, so it names it once (beets's Item.path, read and
        // written by Replace plugin, had stood twice).
        var usedDecls=new Set(),usedEnds=ends.filter(function(end){if(!rmUsesVariable(end.kind)||usedDecls.has(end.decl))return false;usedDecls.add(end.decl);return true;});
        rmFoldBy(usedEnds,function(end){return rmNameGroup(data.decls[end.decl]);},function(list){var ul=rmEl('ul','map-reading-ends');list.forEach(function(end){var item=rmEndItem(ctx,data,end,'out');var word=item.querySelector&&item.querySelector('.map-reading-relation');if(word)word.remove();ul.appendChild(item);});return [ul];}).forEach(function(part){box.appendChild(part);});
      }
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
// A declaration's reading: its name, the link into its code, its signature
// and its file, heading it as a type's title heads a type's (final
// journeys, 2026-10-02: lua_gettop, opened from liblua.a's Entry list, had
// read from "Called by" with no name above it); who calls it, by part; the
// comment its author wrote above it; the model's line; what it writes and
// reads; a type's fields and the functions returning or taking it; what it
// calls, by part.
function rmDeclView(ctx,node,data,concept){
  var key=repomapMembers.sourceKey(concept.source),view=rmEl('div','map-decl-reading'),position=(data.decls||[]).findIndex(function(decl){return decl.key===key;});
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
    var total=groups.reduce(function(sum,group){return sum+(group.decls||[]).length;},0);
    groups.forEach(function(group){
      var fold=total>30&&(group.decls||[]).length>5,box=rmEl(fold?'details':'div','map-reading-peer'),head=rmEl(fold?'summary':'div','map-reading-peer-head');
      // Another program's calls into a declaration both hold are named by
      // that program: "redis-cli: [Command line client] cliConnect()".
      if(group.program)head.appendChild(rmDotBreaks(rmEl('span','map-reading-program',group.program+':')));
      head.appendChild(rmPartBox(ctx,group.part,group.title));box.appendChild(head);
      rmEndRuns(ctx,data,group.decls,which,false,variable).parts.forEach(function(part){box.appendChild(part);});section.appendChild(box);
    });
    return section;
  }
  var name=rmEl('div','map-decl-name');
  var link=rmDeclName({name:decl.name,href:decl.href,open:decl.open,code:decl.code},decl.name,null,decl.at,false,null);link.classList.add('map-decl-code');name.appendChild(link);
  symbols=rmPage.data(node,'symbols')||[];
  var symbol=symbols.find(function(s){return repomapMembers.symbolKey(s)===key&&s.kind!=='field';});
  if(symbol&&symbol.text)name.appendChild(rmEl('span','map-decl-signature',symbol.text));
  view.appendChild(name);
  var where=rmEl('p','map-decl-where');where.appendChild(rmEl('span','meta',decl.file||''));
  view.appendChild(where);
  var callers=side(own.callers,variable?'Used by':'Called by','in');if(callers)view.appendChild(callers);
  // Its own program never runs it, while another program does.
  if(own.not_called_in)view.appendChild(rmEl('p','map-reading-not-called meta',rmT('Not called in {0}',own.not_called_in)));
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
  var rest=flowed?(own.callees||[]).map(function(group){return Object.assign({},group,{decls:(group.decls||[]).filter(function(end){return !callKinds[end.kind];})});}).filter(function(group){return (group.decls||[]).length;}):own.callees;
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
  var kinds=[];(collection.groups||[]).forEach(function(group){if(kinds.indexOf(group.kind)<0)kinds.push(group.kind);});
  // Kinds stand as the canvas stands them (owner, 2026-10-01: the canvas
  // stands inputs by kind), in its order with its marks; within a kind, its
  // inputs by the part where each takes effect, as saved (#rm-scene
  // inputs[id].parts, 29-operation-view.js inputPart; owner, 2026-09-28:
  // inputs answer "where it takes effect"), the parts by title, each under
  // its box, the inputs taking effect in no one part after them; a kind of
  // more than twelve inputs in several parts folds each part to its box.
  var unsaid=new Set();
  // Two inputs of one kind sharing a name read apart by the words saved
  // beside each (page_apart.go), never chosen here: the subcommands it is
  // an option of (freqtrade's two "--erase", of download-data and of
  // install-ui), its catalogue's declaration (dataformat_ohlcv in
  // SCHEMA_TRADE_REQUIRED), its key (version_main), a word its handler
  // declares (etcd's POST /v3electionpb.Election/Campaign), the function
  // registering it. Words only, never its code as written (owner's review,
  // 2026-09-30). Inputs of two kinds are not taken for each other: redis's
  // setting save and its command save stand under two headings.
  function ofWhich(input,id){return rmApartWords((collection.apart||{})[id]);}
  kinds.forEach(function(kind){
    var groups=(collection.groups||[]).filter(function(group){return group.kind===kind;}),all=[].concat.apply([],groups.map(function(group){return group.inputs;}));
    var byPart=new Map();
    all.forEach(function(id){var part=ctx.inputPart?ctx.inputPart(id):'';if(!part||!ctx.nodeById(part))return;if(!byPart.has(part))byPart.set(part,[]);if(byPart.get(part).indexOf(id)<0)byPart.get(part).push(id);});
    var inParts=Array.from(byPart).map(function(pair){return {part:pair[0],title:ctx.nodeById(pair[0]).dataset.title||'',inputs:pair[1]};});
    inParts.sort(function(a,b){return a.title.localeCompare(b.title)||a.part.localeCompare(b.part);});
    var fold=inParts.length>1&&all.length>12;
    var section=rmEl('section','map-collection-group'),heading=rmEl('h6','map-reading-count');
    // A kind chosen in the component's reading, or Settings in the
    // collection's frame, lands on its own section: Background work on the
    // first of its scheduled and continuous sections.
    section.dataset.kind=kind;
    if(rmPendingKind&&[].concat(rmPendingKind).indexOf(kind)>=0){section.dataset.readingAnchor='';rmPick(section);rmPendingKind='';}
    var mark=globalThis.rmKindMark?.(kind);if(mark)heading.appendChild(mark);
    heading.appendChild(rmEl('span','',rmT(rmInputKindTitles[kind]||'Inputs')));
    rmLights(ctx,heading,all);section.appendChild(heading);
    // A catalogue's lines stand once, above its first inputs read; the
    // model's match of a catalogue's inputs likewise.
    var said=new Set();
    function catalogueLines(group,into){
      if(said.has(group))return;said.add(group);
      var first=group.catalogue&&ctx.nodeById(group.catalogue),catalogue=first?rmPage.data(first,'catalogue'):null;
      if(catalogue){var where=rmEl('div','map-collection-where');rmCatalogueLines(ctx,catalogue).forEach(function(line){where.appendChild(line);});into.appendChild(where);}
      // Matched by the model to another program's inputs of the same name:
      // one line, the model's.
      var matched=0,programs=[];
      (group.inputs||[]).forEach(function(id){
        var input=ctx.nodeById(id),path=input?rmPage.data(input,'inputPath'):null;
        if(path&&(path.sent_to||[]).length){matched++;path.sent_to.forEach(function(entry){if(programs.indexOf(entry.program)<0)programs.push(entry.program);});}
      });
      if(matched)into.appendChild(rmModelText('p','map-collection-matched',rmT('Matched to inputs of {0} by name',programs.join(', '))));
    }
    function names(ids){
      var list=rmEl('ul','map-collection-names');
      ids.forEach(function(id){
        var input=ctx.nodeById(id);if(!input)return;
        // A directive with its values: "appendfsync: always | everysec | no".
        var path=rmPage.data(input,'inputPath'),values=path&&path.values?(path.checks||[]).map(function(check){return check.name;}):[];
        var title=input.dataset.title+(values.length?': '+values.join(' | '):'');
        var item=rmEl('li'),button=rmDotBreaks(rmEl('button','',title));button.type='button';
        // A name that is a sentence (a query parameter's description) is
        // prose, not code.
        if(rmProse(input.dataset.title))button.classList.add('map-collection-prose');
        // The words telling it apart are part of its name and its link
        // (owner via the coordinator, 2026-10-02: etcd's link had covered
        // "POST" alone).
        var of=ofWhich(input,id);if(of)button.append(' ',of);
        button.addEventListener('click',function(){ctx.light([]);ctx.readNode(input);});rmLights(ctx,button,[id]);item.appendChild(button);
        list.appendChild(item);
      });
      return list;
    }
    // Some inputs of the kind, each catalogue's under its lines.
    function listed(into,among){
      groups.forEach(function(group){
        var ids=(group.inputs||[]).filter(among);if(!ids.length)return;
        var box=rmEl('div','map-collection-catalogue');catalogueLines(group,box);box.appendChild(names(ids));
        // A catalogue none of whose inputs has an established handler says
        // so under its own inputs, once (owner, 2026-09-30: the line had
        // closed the whole reading, under serverCron, whose handler is
        // established).
        if(group.catalogue&&!unsaid.has(group)&&(group.inputs||[]).every(function(id){var input=ctx.nodeById(id);return input&&input.dataset.handlerUnknown==='true';})){
          unsaid.add(group);box.appendChild(rmEl('p','meta map-collection-unknown',rmT('Where these take effect is not established.')));
        }
        into.appendChild(box);
      });
    }
    if(!inParts.length){listed(section,function(){return true;});view.appendChild(section);return;}
    // One group per part, its box its heading (reviewer, 2026-09-30:
    // freqtrade's requests had been headed "declared in
    // freqtrade.rpc.api_server.api_auth", with "REST API server" twice, once
    // per catalogue).
    inParts.forEach(function(part){
      var holder=ctx.nodeById(part.part),peer=rmEl(fold?'details':'div','map-reading-peer map-collection-part'),head=rmEl(fold?'summary':'div','map-reading-peer-head');
      head.appendChild(rmPartBox(ctx,holder?holder.getAttribute('href')||'#'+holder.id:'',part.title,fold));rmLights(ctx,head,(part.inputs||[]).filter(function(id){return all.indexOf(id)>=0;}));
      peer.appendChild(head);listed(peer,function(id){return (part.inputs||[]).indexOf(id)>=0;});section.appendChild(peer);
    });
    listed(section,function(id){return !inParts.some(function(part){return (part.inputs||[]).indexOf(id)>=0;});});
    view.appendChild(section);
  });
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
    var key=repomapMembers.declKey(decl);
    return rmDeclName({name:decl.name,href:decl.href,open:decl.open,code:decl.code,part:node?node.getAttribute('href'):'',key:key},decl.name,node?function(){ctx.readDeclIn(node,key);}:null,decl.source);
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
        rmFoldBy(use.users,function(user){return rmPartTitle(ctx,(decls[user]||{}).part);},function(list){
          var names=rmEl('ul','map-reading-ends map-collection-callers');list.forEach(function(user){var item=rmEl('li');item.appendChild(name(user));names.appendChild(item);});return [names];
        }).forEach(function(part){users.appendChild(part);});
        row.appendChild(users);
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
  (data.decls||[]).forEach(function(decl){if(decl.key===undefined)decl.key=decl.href;});
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
    rmEntriesByPart(ctx,entries,start,function(held){
      var lines=rmEl('div');
      held.forEach(function(entry){
        var part=entry.part?ctx.nodeByHref(entry.part):null,line=rmEl('div');
        line.appendChild(rmDeclName({name:entry.name,href:entry.href,open:entry.open,key:entry.key},entry.name+(entry.callable?'()':''),part?function(){ctx.readDeclIn(part,entry.key);}:null,''));
        lines.appendChild(line);
      });
      return lines;
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
  if(collection&&(collection.kinds||[]).length){
    var inputs=rmEl('div','map-component-inputs');inputs.appendChild(rmEl('p','map-reading-label',rmT('Inputs')));
    var kinds=rmEl('ul','map-component-input-kinds');
    (collection.kinds||[]).forEach(function(kind){
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
    var steps=rmEl('details','map-component-section'),head=rmEl('summary');head.appendChild(rmEl('span','',rmT('Main flow')));steps.appendChild(head);steps.dataset.mainFlow='';
    Array.from(flow.querySelectorAll(':scope>.flow-title,:scope>p.meta,:scope>ol')).forEach(function(part){steps.appendChild(copy(part));});
    steps.open=true;place(steps);
    // A named fork's candidates stay folded under its line: a flow never
    // ends in a wall of names (copy opens every fold).
    steps.querySelectorAll('details.flow-fork').forEach(function(fork){fork.open=false;});
    // A long way of a parted flow shows its first step, the rest folded.
    steps.querySelectorAll('details.flow-way-rest[data-folded]').forEach(function(rest){rest.open=false;});
    // Each run of steps read in one part stands under that part's box, its
    // title alone, its description on hover, a click reading it (review
    // 2026-10-02: the steps had read as a list of calls with no place).
    steps.querySelectorAll('li.flow-part-head').forEach(function(head){
      var node=ctx.nodeByHref(head.dataset.flowPart);
      if(!node){head.remove();return;}
      var box=rmPartBox(ctx,head.dataset.flowPart,'');if(node.dataset.summary)box.title=node.dataset.summary;head.appendChild(box);
    });
    // A type's line reads its first sentence; a click on it says the rest.
    steps.querySelectorAll('.flow-type').forEach(function(line){
      rmTypeLineFold(line);
      line.addEventListener('click',function(event){event.stopPropagation();line.classList.toggle('flow-type-open');});
    });
    // An input a step handles reads that input, its tile lit while pointed
    // at, named as the map names it.
    steps.querySelectorAll('code.flow-input[data-input]').forEach(function(code){
      var input=ctx.nodeById(code.dataset.input);if(!input)return;
      var name=rmEl('button','system-catalogue-member flow-input',input.dataset.title||code.textContent);name.type='button';
      name.addEventListener('click',function(event){event.stopPropagation();ctx.light([]);ctx.readNode(input);});
      rmLights(ctx,name,[code.dataset.input]);code.replaceWith(name);
    });
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
      rmFlowStep(ctx,step,part,step.dataset.stepKey);
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
  var d=element.dataset,part=d.stepPart?ctx.nodeByHref(d.stepPart):null,key=d.stepKey||'';
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
// A program's entries, folded by the part holding them when there are
// more than twelve (a library's exports), as an input kind's list folds:
// each part closed under its box and its count, the entries no part holds
// after them (owner via the coordinator, 2026-10-02: liblua.a's 156 lua_*,
// luaL_* and luaopen_* had stood in one flat list, Lua 5.1.5's 159 in the
// home's list of programs). `list(entries)` draws some entries.
function rmEntriesByPart(ctx,entries,into,list){
  var byPart=new Map(),loose=[];
  entries.forEach(function(entry){var part=entry.part&&ctx.nodeByHref(entry.part);if(!part){loose.push(entry);return;}if(!byPart.has(part))byPart.set(part,[]);byPart.get(part).push(entry);});
  if(entries.length>12&&byPart.size){
    byPart.forEach(function(held,part){
      var fold=rmEl('details','map-reading-peer system-program-entries'),head=rmEl('summary','map-reading-peer-head');
      head.append(rmPartBox(ctx,part.getAttribute('href')||'#'+part.id,part.dataset.title,true),' · '+held.length);
      fold.append(head,list(held));into.appendChild(fold);
    });
  }else loose=entries;
  if(loose.length)into.appendChild(list(loose));
  return into;
}
// The home's table of programs (owner, 2026-09-28), one block each in the
// component's page order: its name, reading it; its role, the model's; its
// entry; the kinds of its inputs, in words; its connections as its arrow
// ends group them;
// and the files it is built from (page data: for C its link line's units),
// a build fact: a model's summary had said all four Redis programs share
// ae, sds, adlist, dict and anet, which their Makefile does not.
// Every name in it does what a newcomer expects (owner, 2026-09-30:
// "ничего не кликабельное"): a program's name reads it; its entry reads
// that function in its part; an input kind reads its inputs of that kind;
// a connection reads its program at that connection, and an Outside frame
// at its other end reads that frame.
function rmProgramsTable(ctx,holder,components,connections){
  if(!holder||!components.length)return;
  var table=rmEl('dl','system-programs-list');
  function line(label,content){if(!content)return;table.append(rmEl('dt','',rmT(label)),content);}
  function button(text,go){var b=rmDotBreaks(rmEl('button','system-program-link',text));b.type='button';b.addEventListener('click',function(event){event.stopPropagation();go();});return b;}
  components.forEach(function(n){
    var head=rmEl('dt','system-program-name'),name=rmDotBreaks(rmEl('button','',n.dataset.title));name.type='button';
    name.addEventListener('click',function(){ctx.readNode(n);});head.appendChild(name);table.appendChild(head);
    var about=rmEl('dd','system-program-about');
    if(n.dataset.role)about.appendChild(rmModelText('span','',n.dataset.role,n.dataset.roleRef));
    table.appendChild(about);
    var entries=rmPage.data(n,'entries')||[];
    if(entries.length){
      var entryItem=function(entry){
        var text=entry.name+(entry.callable?'()':''),item=rmEl('li'),part=entry.part&&ctx.nodeByHref(entry.part);
        if(part&&entry.key)item.appendChild(button(text,function(){ctx.readDeclIn(part,entry.key);}));
        else if(entry.href||entry.open){var link=repomapMembers.sourceLink({Href:entry.href,Open:entry.open,Text:text,NoSource:entry.no_source});item.appendChild(rmDotBreaks(link));}
        else item.appendChild(button(text,function(){ctx.readNode(n);}));
        return item;
      };
      var at=rmEl('dd');rmEntriesByPart(ctx,entries,at,function(held){var list=rmEl('ul','system-program-files');held.forEach(function(entry){list.appendChild(entryItem(entry));});return list;});
      line('Entry',at);
    }
    var collection=ctx.nodeById('system-inputs-'+n.dataset.owner),kinds=(collection&&rmPage.data(collection,'collection')||{}).kinds||[];
    if(kinds.length){
      var kindList=rmEl('dd');
      kinds.forEach(function(kind,i){
        if(i)kindList.append(' · ');
        var b=button(rmT(rmInputKindTitles[kind.kind]||'Inputs'),function(){ctx.light([]);rmPendingKind=kind.kind;ctx.readNode(collection);});
        var mark=globalThis.rmKindMark?.(kind.kind);if(mark)b.prepend(mark);
        rmLights(ctx,b,kind.inputs||[]);kindList.append(b);
      });
      line('Inputs',kindList);
    }
    var ends=connections(n.id);
    // An arrow stays with its name, which breaks only at its dots.
    if(ends.length){
      var peers=rmEl('ul','system-program-files');
      ends.forEach(function(end){
        var item=rmEl('li'),other=end.outside&&ctx.nodeById(end.outside);
        item.appendChild(button((end.incoming?'←\u00a0':'→\u00a0')+end.title,function(){
          if(other&&(other.dataset.branch==='outside'||other.dataset.branch==='communication'))ctx.readNode(other);
          else if(ctx.openConnection&&end.key)ctx.openConnection(n.id,end.key);
          else if(other)ctx.readNode(other);
        }));
        peers.appendChild(item);
      });
      var cell=rmEl('dd');cell.appendChild(peers);line('Connections',cell);
    }
    // Its files by their top folder, each folder closed, opening to its
    // files by name (owner, 2026-09-30: "что будет если файлов много?"); a
    // folder of many files the same way inside; the files at the top first.
    var files=rmPage.data(n,'sources')||[];
    if(files.length){
      var built=rmEl('dd');
      if(files.length<=rmShortSection){var ul=rmEl('ul','system-program-files');files.forEach(function(file){ul.appendChild(rmDotBreaks(rmEl('li','',file)));});built.appendChild(ul);}
      else{var fold=rmEl('details','system-program-built');fold.appendChild(rmEl('summary','',rmT('Files')));rmFileTree(files).forEach(function(part){fold.appendChild(part);});built.appendChild(fold);}
      line('Built from',built);
    }
  });
  holder.replaceChildren(rmEl('h4','',rmT('Programs')),table);
}
// Paths by their top folder: the files at the top one to a line, then each
// folder closed under its name, opening to what is in it the same way when
// it holds more than a screen's worth, else its files by name.
function rmFileTree(files){
  var top=[],folders=new Map();
  files.forEach(function(file){var at=file.indexOf('/');if(at<0){top.push(file);return;}var dir=file.slice(0,at+1);if(!folders.has(dir))folders.set(dir,[]);folders.get(dir).push(file.slice(at+1));});
  var out=[];
  if(top.length){var ul=rmEl('ul','system-program-files');top.forEach(function(file){ul.appendChild(rmDotBreaks(rmEl('li','',file)));});out.push(ul);}
  folders.forEach(function(list,dir){
    var fold=rmEl('details','map-reading-group system-program-folder');fold.appendChild(rmEl('summary','',dir));
    (list.length>rmShortSection&&list.some(function(file){return file.indexOf('/')>=0;})?rmFileTree(list):[(function(){var ul=rmEl('ul','system-program-files');list.forEach(function(file){ul.appendChild(rmDotBreaks(rmEl('li','',file)));});return ul;})()]).forEach(function(part){fold.appendChild(part);});
    out.push(fold);
  });
  return out;
}
// </reading-column>
