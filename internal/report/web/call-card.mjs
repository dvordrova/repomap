// The calls behind one arrow end, arranged the way its card and the reading
// column list them: grouped by the part a call is made from, then by the
// part it goes into, each call once, in the order its call sites are
// written. A call the page data marks with a fold (a dispatch site calls one
// of a set, or a declaration hands that whole set over) is one line per
// caller and fold, with its callees under it by part. Nothing is counted
// away: every call stands in a line or in a fold. Server runtime → Data type
// commands listed "call → xCommand" 77 times, then "cmdTable passes
// callback xCommand" 77 times, and the answer sat at the bottom.
//   relations: an arrow end's relations, each {from, to, calls, label, …}.
//   nameOf(id): a node's name; groupable(id): whether calls from the node
//   are grouped under it (an input is not a part a call is made from);
//   incoming: whether the calls come in from outside the frame.
export function callCard(relations,{nameOf=id=>id,groupable=()=>true,incoming=false}={}){
  const seen=new Set(),groups=new Map(),from=new Map(),into=new Map(),kinds=new Map();
  let total=0;
  const count=(map,id)=>map.set(id,(map.get(id)||0)+1);
  for(const relation of relations){
    const calls=relation.calls?.length?relation.calls:[{label:relation.label||relation.summary||'',from:relation.fromSource,to:relation.toSource}];
    const fromID=groupable(relation.from)?relation.from:'',intoID=relation.to;
    for(const call of calls){
      const words=String(call.label||'').match(/^(\S+) (\S+) (\S+)$/);
      const key=[fromID,intoID,words?call.label:`${call.label}|${call.at||''}|${call.name||''}`].join('|');
      if(seen.has(key))continue;seen.add(key);total++;
      const kind=words?words[2]:call.name?'implemented in':'other';
      count(kinds,['calls','passes_callback','reads','implemented in'].includes(kind)?kind:'other');
      if(fromID)count(from,fromID);
      count(into,intoID);
      if(!groups.has(fromID))groups.set(fromID,{id:fromID,name:fromID?nameOf(fromID):'',count:0,folds:new Map(),pairs:new Map()});
      const group=groups.get(fromID);group.count++;
      const row={kind,site:call.from||'',at:call.at||'',
        caller:words?words[1]:call.name?nameOf(relation.from):'',
        callee:words?words[3]:call.name||'',calleeHref:words||call.name?call.to||'':'',
        other:words||call.name?'':nameOf(incoming?relation.from:relation.to),
        otherHref:call.from||call.to||''};
      if(call.fold&&words){
        const foldKey=`${row.caller}\0${call.fold}`;
        if(!group.folds.has(foldKey))group.folds.set(foldKey,{caller:row.caller,site:row.site,kind,fold:call.fold,of:call.of||0,one:!!call.one,same:call.same||'',count:0,parts:new Map()});
        const fold=group.folds.get(foldKey);fold.count++;
        if(!fold.parts.has(intoID))fold.parts.set(intoID,{id:intoID,name:nameOf(intoID),count:0,rows:[]});
        const part=fold.parts.get(intoID);part.count++;part.rows.push({callee:row.callee,href:row.calleeHref});
        continue;
      }
      if(!group.pairs.has(intoID))group.pairs.set(intoID,{id:intoID,name:nameOf(intoID),count:0,rows:[]});
      const pair=group.pairs.get(intoID);pair.count++;pair.rows.push(row);
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
