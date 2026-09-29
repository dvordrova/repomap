// The calls behind one arrow end, arranged the way its card and the reading
// column list them: grouped by the part a call is made from, then by the
// part it goes into, each call once, in the order its call sites are
// written. A call the page data marks with a fold (a dispatch site calls one
// of a set, or a declaration hands that whole set over) is one line per
// caller and fold, with its callees under it by part. Nothing is counted
// away: every call stands in a line or in a fold. Inputs that share one
// handler are one line naming each of them ("sinter, smembers →
// sinterCommand"): the line counts the handler, and no input is dropped
// from it. Server runtime → Data type
// commands listed "call → xCommand" 77 times, then "cmdTable passes
// callback xCommand" 77 times, and the answer sat at the bottom.
//   relations: an arrow end's relations, each {from, to, calls, label, …}.
//   nameOf(id): a node's name; groupable(id): whether calls from the node
//   are grouped under it (an input is not a part a call is made from);
//   incoming: whether the calls come in from outside the frame.
// An input whose handler is not established draws its arrow into the part
// its code takes it in (page_operations.go): "declared in" the function
// declaring it, "looked up in" a reader of its table. Its line names its
// inputs as a handler's line does ("-h, -p declared in parseOptions"); it
// never reads as "implemented in".
export const takenIn=new Set(['declared in','looked up in']);
export function callCard(relations,{nameOf=id=>id,groupable=()=>true,incoming=false}={}){
  const seen=new Map(),groups=new Map(),from=new Map(),into=new Map(),kinds=new Map(),inputs=new Set();
  let total=0;
  const count=(map,id)=>map.set(id,(map.get(id)||0)+1);
  for(const relation of relations){
    const calls=relation.calls?.length?relation.calls:[{label:relation.label||relation.summary||'',from:relation.fromSource,to:relation.toSource}];
    const fromID=groupable(relation.from)?relation.from:'',intoID=relation.to;
    // An input's own arrow with no call and no words is its component
    // taking it in (page_system_map.go).
    const member=!relation.calls?.length&&!relation.label&&!relation.summary&&!groupable(relation.from);
    for(const call of calls){
      // A named call's label is its relation's words ("looked up in"),
      // never "caller verb callee".
      const words=!call.name&&String(call.label||'').match(/^(\S+) (\S+) (\S+)$/);
      const key=[fromID,intoID,words?call.label:`${call.label}|${call.at||''}|${call.name||''}`].join('|');
      const kind=words?words[2]:call.name?(takenIn.has(call.label)?call.label:'implemented in'):member?'input':'other';
      // Inputs taken in, not handled, count as inputs, each once.
      if(takenIn.has(kind)||kind==='input'){inputs.add(relation.from);kinds.set('inputs',inputs.size);}
      if(seen.has(key)){
        const row=seen.get(key),input=nameOf(relation.from);
        if(row?.inputs&&!row.inputs.includes(input)){
          row.inputRefs.push({id:relation.from,name:input});row.inputRefs.sort((a,b)=>String(a.name).localeCompare(String(b.name)));
          row.inputs=row.inputRefs.map(ref=>ref.name);row.caller=row.inputs.join(', ');
        }
        continue;
      }
      total++;
      if(!takenIn.has(kind)&&kind!=='input')count(kinds,['calls','passes_callback','reads','implemented in'].includes(kind)?kind:'other');
      if(fromID)count(from,fromID);
      count(into,intoID);
      if(!groups.has(fromID))groups.set(fromID,{id:fromID,name:fromID?nameOf(fromID):'',count:0,folds:new Map(),pairs:new Map()});
      const group=groups.get(fromID);group.count++;
      // The declarations at the call's ends, as the page data keys them,
      // each with the part it is read in: the reading column reads a name
      // there instead of opening its code.
      const end=(part,key)=>part&&key?{part,key}:null;
      const row={kind,site:call.from||'',at:call.at||'',
        caller:words?words[1]:call.name||member?nameOf(relation.from):'',
        callee:words?words[3]:call.name||'',calleeHref:words||call.name?call.to||'':'',
        callerAt:words?end(relation.from,call.caller):null,calleeAt:words||call.name?end(intoID,call.callee):null,
        other:words||call.name?'':nameOf(incoming?relation.from:relation.to),
        otherHref:call.from||call.to||'',
        // A call that leaves its program: each program's side of it, from
        // its own code (page_shared_code.go).
        sides:call.sides||null};
      // A row of inputs names each by its node, so the reading column can
      // read it ("sync, slaveof → syncCommand").
      seen.set(key,!words&&(call.name||member)?Object.assign(row,{inputs:[row.caller],inputRefs:[{id:relation.from,name:row.caller}]}):null);
      if(call.fold&&words){
        const foldKey=`${row.caller}\0${call.fold}`;
        if(!group.folds.has(foldKey))group.folds.set(foldKey,{caller:row.caller,callerAt:row.callerAt,site:row.site,at:row.at,kind,fold:call.fold,of:call.of||0,one:!!call.one,same:call.same||'',count:0,parts:new Map()});
        const fold=group.folds.get(foldKey);fold.count++;
        if(!fold.parts.has(intoID))fold.parts.set(intoID,{id:intoID,name:nameOf(intoID),count:0,rows:[]});
        const part=fold.parts.get(intoID);part.count++;part.rows.push({callee:row.callee,href:row.calleeHref,calleeAt:row.calleeAt});
        continue;
      }
      // Ends of one name are one heading: litestream's SQLite had listed
      // "→ checkpointWithExecutor 1" twice, one per statement it runs there.
      const pairKey=String(nameOf(intoID)??'')||`\0${intoID}`;
      if(!group.pairs.has(pairKey))group.pairs.set(pairKey,{id:intoID,name:nameOf(intoID),count:0,rows:[]});
      const pair=group.pairs.get(pairKey);pair.count++;pair.rows.push(row);
    }
  }
  const byCount=(a,b)=>b.count-a.count||String(a.name).localeCompare(String(b.name));
  const listed=map=>[...map].map(([id,n])=>({id,name:nameOf(id),count:n})).sort(byCount);
  return {
    total,kinds:[...kinds],from:listed(from),into:listed(into),
    groups:[...groups.values()].sort((a,b)=>(a.id?0:1)-(b.id?0:1)||byCount(a,b)).map(group=>({...group,
      folds:[...group.folds.values()].sort((a,b)=>b.count-a.count||a.caller.localeCompare(b.caller)).map(fold=>({...fold,
        parts:[...fold.parts.values()].sort(byCount).map(part=>({...part,rows:part.rows.sort((a,b)=>a.callee.localeCompare(b.callee))}))})),
      pairs:[...group.pairs.values()].sort(byCount).map(pair=>({...pair,rows:pair.rows.sort(bySite)}))})),
  };
}

// In the reading column a row names its declarations and ends in its code
// mark, with no place printed. A row with no call of its own names only the
// end it goes to or comes from, which its heading already names, so there it
// says nothing but its code: its mark joins the heading and the row is not
// repeated (litestream's "→ acquireReadLock", then "acquireReadLock </>").
// A row with no code to link says nothing at all there.
export function headingMarks(pair,group){
  const repeats=row=>row.kind==='other'&&(row.other===pair.name||row.other===group.name);
  return {marks:pair.rows.filter(row=>repeats(row)&&row.otherHref),rows:pair.rows.filter(row=>!repeats(row))};
}

// What a card's counts count, by kind of call. An arrow from inputs counts
// the handlers they are implemented in (inputs sharing one stand in its
// line), so its total says handlers: "← Inputs 94" had stood beside
// "Inputs 96" and "95 inputs dispatched here" with nothing telling them
// apart. Inputs whose handler is not established are counted as inputs:
// the part their arrow enters takes them in and handles none of them.
export const countWords={calls:'{0} calls',passes_callback:'{0} callbacks',reads:'{0} reads','implemented in':'{0} handlers',inputs:'{0} inputs',other:'{0} other connections'};
export function countsHandlers(card){
  return card.kinds.length>0&&card.kinds.every(([kind])=>kind==='implemented in');
}
// The number of inputs a card of inputs taken in counts, or 0 when it
// counts anything else too.
export function countsInputs(card){
  return card.kinds.length===1&&card.kinds[0][0]==='inputs'?card.kinds[0][1]:0;
}

// Rows stand in the order their call sites are written: file, then line.
function bySite(a,b){
  const at=row=>{const m=/^(.*):(\d+)(?::\d+)?$/.exec(row.at);return m?[m[1],Number(m[2])]:[row.at,0];};
  const [pa,la]=at(a),[pb,lb]=at(b);
  return pa<pb?-1:pa>pb?1:la-lb||a.caller.localeCompare(b.caller)||a.callee.localeCompare(b.callee);
}

// How many parts of a frame a side of the card reaches: every one, some of
// them, or nothing to say when the side is a single part.
export function reach(count,of){
  if(!(of>1))return null;
  return count>=of?{all:true,count:of}:{all:false,count,of};
}
