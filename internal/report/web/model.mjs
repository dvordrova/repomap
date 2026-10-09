// The display graph the scene canvas draws (PLAN S1), built once from the
// page's data: what stands in what, the arrows between the things drawn,
// the Outside frames grouped by part (B′), the markers every box carries,
// the whole map's arrows and a frame's connections. It is pure: no layout,
// no camera, no DOM. The facts it draws are saved with the report and
// read here as saved (owner, 2026-10-01: "у html должна быть простая
// задача — вот данные, показываю"; REPORT.md's Scene model): where each
// input takes effect, who calls each outside system, each part's calls and
// the pairs of programs. It groups, caps and folds them for drawing and
// derives none. Levels, scene and overlay read only the model.
import {prepareCards} from './cards.mjs';
import {connections} from './layout.mjs';

// An input's kind as saved (scene.go sceneInputKinds), in the order the
// reading column names its sections; an input the saved scene does not
// know is of a kind not established.
export const inputKinds=['request','command','setting','scheduled','continuous','interaction','consumer','extension','entry'];
export const inputKindTitles={request:'Incoming requests',command:'Commands',setting:'Settings',scheduled:'Scheduled tasks',
  continuous:'Background work',interaction:'User interactions',consumer:'Queue consumers',extension:'Extension points',entry:'Kind not established'};
const inputKind=kind=>inputKinds.includes(kind)?kind:'entry';
// An outside system's kind as saved (scene.go sceneSystemKinds); 'other'
// when none is.
export const systemKinds=['database','request','sdk','queue','started','other'];
const systemKind=kind=>systemKinds.includes(kind)?kind:'other';
// At most this many markers stand on a side of a box (PLAN B).
export const markersPerSide=3;

// The saved scene (report.json `scene`, the page's #rm-scene), every field
// present: {inputs, systems, calls, programPairs}.
function savedScene(scene){
  const object=value=>value&&typeof value==='object'&&!Array.isArray(value)?value:{};
  return {inputs:object(scene?.inputs),systems:object(scene?.systems),calls:object(scene?.calls),
    programPairs:Array.isArray(scene?.programPairs)?scene.programPairs:[]};
}

// What a page record is on the canvas.
function kindOf(item){
  if(item.branch==='component')return 'program';
  if(item.branch==='area')return 'area';
  if(item.branch==='inputs')return 'inputs';
  if(item.branch==='outside')return 'outside';
  if(item.branch==='communication')return 'system';
  if(item.activation)return 'input';
  if(item.category==='external')return 'call';
  if(item.category==='component')return 'note';
  return 'part';
}

// `page` {items, relations, areas} as the page hands them to rmCreateFlow,
// and `scene` the saved facts; `measure(text, font)`
// measures text; `t` translates fixed words. Returns the model (see the
// fields at the end).
export function buildModel(page,{measure,t=text=>text}={}){
  const saved=savedScene(page.scene);
  const items=prepareCards(page.items||[],{},measure,t);
  const record=new Map(items.map(item=>[item.id,item]));
  const parentOf=new Map();
  for(const item of items)for(const child of item.children||[])if(record.has(child))parentOf.set(child,item.id);
  for(const area of page.areas||[])for(const child of area.nodes||[])if(record.has(child)&&record.has(area.id))parentOf.set(child,area.id);
  const kind=new Map(items.map(item=>[item.id,kindOf(item)]));
  // A call tile stands for its system: the system is the chip, the call is
  // read in the column (overview.mjs outsideChips).
  const systemOfCall=new Map();
  for(const item of items)if(kind.get(item.id)==='system')for(const tile of item.children||[])if(kind.get(tile)==='call')systemOfCall.set(tile,item.id);
  const shown=id=>systemOfCall.get(id)||id;

  // One edge per directed pair of drawn things and certainty, holding
  // every original relation (split-layout.mjs prepareInteriors did the same).
  // An edge keeps the positions of its relations in the page's list, as
  // the saved pairs of programs name them.
  const folded=new Map(),edgesAt=new Map();
  (page.relations||[]).forEach((relation,position)=>{
    const from=shown(relation.displayFrom||relation.from),to=shown(relation.displayTo||relation.to);
    if(from===to||!record.has(from)||!record.has(to))return;
    if(kind.get(from)==='call'||kind.get(to)==='call')return;
    const key=JSON.stringify([from,to,!!relation.possible]);
    if(!folded.has(key))folded.set(key,{id:`e${folded.size}`,from,to,possible:!!relation.possible,init:true,relations:[]});
    const edge=folded.get(key);edge.relations.push(relation);edge.init&&=!!relation.init;
    edgesAt.set(position,edge.id);
  });
  const edges=[...folded.values()];

  const nodes=new Map();
  const add=node=>{nodes.set(node.id,{children:[],...node});return nodes.get(node.id);};
  const name=item=>item?.name||item?.title||'';
  // Programs, areas, parts and the note keep the page's containment.
  for(const item of items){
    const k=kind.get(item.id);
    if(!['program','area','part','note'].includes(k))continue;
    add({id:item.id,kind:k,name:name(item),item,parent:parentOf.get(item.id)||''});
  }
  for(const node of nodes.values())if(node.parent&&nodes.has(node.parent))nodes.get(node.parent).children.push(node.id);
  // The order a frame lists its children in is the page's.
  for(const node of nodes.values()){
    const order=record.get(node.id)?.children||[];
    const rank=id=>{const at=order.indexOf(id);return at<0?order.length:at;};
    node.children.sort((a,b)=>rank(a)-rank(b));
  }

  // Inputs stand by kind in their program's collection (PLAN B: never by
  // part), each kind a group holding its inputs' names.
  const kindGroupOf=new Map();
  for(const item of items.filter(item=>kind.get(item.id)==='inputs')){
    const collection=add({id:item.id,kind:'inputs',name:t('Inputs'),item,parent:'',program:item.componentOwner||'',children:[]});
    const leaves=id=>{const at=record.get(id);return kind.get(id)==='input'?[id]:(at?.children||[]).flatMap(leaves);};
    const groups=new Map();
    for(const id of (item.children||[]).flatMap(leaves)){
      const k=inputKind(saved.inputs[id]?.kind);
      if(!groups.has(k))groups.set(k,[]);
      if(!groups.get(k).includes(id))groups.get(k).push(id);
    }
    for(const k of inputKinds.filter(k=>groups.has(k))){
      const group=add({id:`${item.id}#${k}`,kind:'kind',inputKind:k,name:t(inputKindTitles[k]),parent:item.id,program:collection.program,children:groups.get(k)});
      collection.children.push(group.id);
      for(const id of group.children){
        kindGroupOf.set(id,group.id);
        add({id,kind:'input',name:name(record.get(id)),item:record.get(id),parent:group.id,inputKind:k,program:saved.inputs[id]?.program||collection.program});
      }
    }
  }
  // An input no collection holds stands alone.
  for(const item of items)if(kind.get(item.id)==='input'&&!nodes.has(item.id))
    add({id:item.id,kind:'input',name:name(item),item,parent:'',inputKind:inputKind(saved.inputs[item.id]?.kind),program:saved.inputs[item.id]?.program||item.componentOwner||''});

  // Who calls each system, as saved: its parts and its programs.
  const callers=new Map(),callingPrograms=new Map();
  for(const [id,system] of Object.entries(saved.systems)){
    if(kind.get(id)!=='system')continue;
    callers.set(id,new Set((system.parts||[]).filter(part=>kind.get(part)==='part')));
    callingPrograms.set(id,new Set((system.programs||[]).filter(program=>kind.get(program)==='program')));
  }
  // Outside frames grouped by part (B′): first the systems two or more
  // parts call, the most called first (casdoor's PostgreSQL); then one
  // bucket per part with two or more systems of its own; then the rest.
  for(const item of items.filter(item=>kind.get(item.id)==='outside')){
    const frame=add({id:item.id,kind:'outside',name:name(item),item,parent:'',children:[]});
    const systems=(item.children||[]).filter(id=>kind.get(id)==='system');
    const order=new Map(systems.map((id,i)=>[id,i]));
    const partsOf=id=>[...(callers.get(id)||[])];
    const shared=systems.filter(id=>partsOf(id).length>=2&&!record.get(id).unestablished)
      .sort((a,b)=>partsOf(b).length-partsOf(a).length||order.get(a)-order.get(b));
    const own=new Map();
    for(const id of systems){
      if(shared.includes(id)||record.get(id).unestablished||partsOf(id).length!==1)continue;
      const part=partsOf(id)[0];
      if(!own.has(part))own.set(part,[]);
      own.get(part).push(id);
    }
    const buckets=[...own].filter(([,list])=>list.length>=2)
      .sort((a,b)=>b[1].length-a[1].length||name(record.get(a[0])).localeCompare(name(record.get(b[0]))));
    const bucketed=new Set(buckets.flatMap(([,list])=>list));
    const rest=systems.filter(id=>!shared.includes(id)&&!bucketed.has(id)).sort((a,b)=>(record.get(a).unestablished?1:0)-(record.get(b).unestablished?1:0)||order.get(a)-order.get(b));
    const chip=(id,parent)=>add({id,kind:'system',name:name(record.get(id)),item:record.get(id),parent,
      systemKind:systemKind(saved.systems[id]?.kind),unestablished:!!record.get(id).unestablished});
    for(const id of shared){chip(id,frame.id);frame.children.push(id);}
    for(const [part,list] of buckets){
      const bucket=add({id:`${frame.id}~${part}`,kind:'bucket',name:name(record.get(part)),part,parent:frame.id,children:list});
      frame.children.push(bucket.id);
      for(const id of list)chip(id,bucket.id);
    }
    for(const id of rest){chip(id,frame.id);frame.children.push(id);}
  }
  // A system no frame holds stands alone.
  for(const item of items)if(kind.get(item.id)==='system'&&!nodes.has(item.id))
    add({id:item.id,kind:'system',name:name(item),item,parent:'',systemKind:systemKind(saved.systems[item.id]?.kind)});

  const roots=[...nodes.values()].filter(node=>!node.parent||!nodes.has(node.parent)).map(node=>node.id);
  for(const id of roots)nodes.get(id).parent='';
  // The page's order: programs, then each program's Inputs and Outside
  // frames, then what else stands alone.
  const rootRank=id=>{const k=nodes.get(id).kind;return ['program','inputs','outside','note','part','system','input'].indexOf(k);};
  const itemRank=new Map(items.map((item,i)=>[item.id,i]));
  roots.sort((a,b)=>rootRank(a)-rootRank(b)||(itemRank.get(a)??0)-(itemRank.get(b)??0));

  const parent=id=>nodes.get(id)?.parent||'';
  const ancestors=id=>{const out=[];for(let at=parent(id);at;at=parent(at))out.push(at);return out;};
  const rootOf=id=>{let at=id;while(parent(at))at=parent(at);return at;};
  const leaves=id=>{const node=nodes.get(id);return node?.children.length?node.children.flatMap(leaves):[id];};
  const within=(id,box)=>id===box||ancestors(id).includes(box);
  const programOf=id=>{
    const node=nodes.get(id);if(!node)return '';
    if(node.kind==='program')return id;
    if(node.program)return node.program;
    const root=rootOf(id);return nodes.get(root)?.kind==='program'?root:'';
  };

  // Where each input takes effect, as saved: the part holding its handler
  // (`handled`, the handler's place), else the parts its code takes it in,
  // else, no part, its program.
  const anchors=new Map();
  for(const node of nodes.values()){
    if(node.kind!=='input')continue;
    const fact=saved.inputs[node.id],parts=[...new Set((fact?.parts||[]).filter(part=>nodes.get(part)?.kind==='part'))];
    anchors.set(node.id,parts.length?{parts,handled:!!fact.handled,handler:fact.handled&&fact.handler||null}:{parts:[],handled:false,program:fact?.program||''});
  }
  // The systems each part calls, as saved, and the places of the
  // declarations making the calls.
  const calls=new Map(),callPlaces=new Map();
  for(const [part,list] of Object.entries(saved.calls)){
    if(nodes.get(part)?.kind!=='part'||!Array.isArray(list))continue;
    const made=list.filter(call=>nodes.get(call?.system)?.kind==='system');
    calls.set(part,new Set(made.map(call=>call.system)));
    callPlaces.set(part,made);
  }
  // A box's markers: the kinds of the inputs taking effect anywhere in it on
  // its left, the kinds of the systems anything in it calls on its right
  // (PLAN B "Scene model"): {kind, members, systems}, members the inputs or
  // the calling parts. At most markersPerSide per side: past it the least
  // held kinds fold into the last marker, its kind the most held of them.
  const markerCache=new Map();
  function markersOf(box){
    if(markerCache.has(box))return markerCache.get(box);
    const inside=new Set(nodes.get(box)?.kind==='part'?[box]:leaves(box).filter(id=>nodes.get(id)?.kind==='part'));
    const byIn=new Map(),byOut=new Map();
    for(const [input,anchor] of anchors){
      const here=anchor.parts.filter(part=>inside.has(part));
      const own=!anchor.parts.length&&anchor.program===box;
      if(!here.length&&!own)continue;
      const k=nodes.get(input).inputKind;
      if(!byIn.has(k))byIn.set(k,{kind:k,members:[],systems:[],handled:[]});
      const marker=byIn.get(k);marker.members.push(input);if(anchor.handled)marker.handled.push(input);
    }
    for(const part of inside)for(const system of calls.get(part)||[]){
      const k=nodes.get(system)?.systemKind||'other';
      if(!byOut.has(k))byOut.set(k,{kind:k,members:[],systems:[]});
      const marker=byOut.get(k);
      if(!marker.members.includes(part))marker.members.push(part);
      if(!marker.systems.includes(system))marker.systems.push(system);
    }
    const result={in:capped([...byIn.values()],m=>m.members.length,inputKinds),out:capped([...byOut.values()],m=>m.systems.length,systemKinds)};
    markerCache.set(box,result);
    return result;
  }
  // At most markersPerSide markers a side: past it the least held kinds
  // fold into the last marker, its kind the most held of them.
  function capped(list,size,order){
    list.sort((a,b)=>order.indexOf(a.kind)-order.indexOf(b.kind));
    if(list.length<=markersPerSide)return list;
    const bySize=[...list].sort((a,b)=>size(b)-size(a)||order.indexOf(a.kind)-order.indexOf(b.kind));
    const kept=bySize.slice(0,markersPerSide-1),folded=bySize.slice(markersPerSide-1);
    const last={kind:folded[0].kind,folded:folded.map(marker=>marker.kind),members:[...new Set(folded.flatMap(m=>m.members))],
      systems:[...new Set(folded.flatMap(m=>m.systems))],handled:[...new Set(folded.flatMap(m=>m.handled||[]))]};
    return [...kept.sort((a,b)=>order.indexOf(a.kind)-order.indexOf(b.kind)),last];
  }
  // Inside a part drawn as its declarations, a marker stands on the
  // declaration handling the input, or making the outside call, by its
  // saved place {path, line}; an input with no known handler, and what
  // names no declaration of the part, stays on the part's edge.
  // {members: Map symbol index → {in, out}, rest: {in, out}}.
  const memberCache=new Map();
  function memberMarkersOf(part){
    if(memberCache.has(part))return memberCache.get(part);
    const symbols=nodes.get(part)?.item?.symbols||[],at=new Map();
    symbols.forEach((symbol,i)=>{const key=symbol.path&&symbol.line?`${symbol.path}:${symbol.line}`:'';if(key&&!at.has(key))at.set(key,i);});
    const placeOf=source=>source?.path&&source.line?at.get(`${source.path}:${source.line}`):undefined;
    const byMember=new Map(),rest={in:new Map(),out:new Map()};
    const side=(i,way)=>{
      if(i===undefined)return rest[way];
      if(!byMember.has(i))byMember.set(i,{in:new Map(),out:new Map()});
      return byMember.get(i)[way];
    };
    for(const [input,anchor] of anchors){
      if(!anchor.parts.includes(part))continue;
      const k=nodes.get(input).inputKind,list=side(anchor.handled?placeOf(anchor.handler):undefined,'in');
      if(!list.has(k))list.set(k,{kind:k,members:[],systems:[],handled:[]});
      const marker=list.get(k);
      if(!marker.members.includes(input)){marker.members.push(input);if(anchor.handled)marker.handled.push(input);}
    }
    for(const call of callPlaces.get(part)||[]){
      const k=nodes.get(call.system).systemKind,list=side(placeOf(call.caller),'out');
      if(!list.has(k))list.set(k,{kind:k,members:[part],systems:[],handled:[]});
      const marker=list.get(k);if(!marker.systems.includes(call.system))marker.systems.push(call.system);
    }
    const finish=sides=>({in:capped([...sides.in.values()],m=>m.members.length,inputKinds),out:capped([...sides.out.values()],m=>m.systems.length,systemKinds)});
    const result={members:new Map([...byMember].map(([i,sides])=>[i,finish(sides)])),rest:finish(rest)};
    memberCache.set(part,result);
    return result;
  }

  const model={
    nodes,roots,edges,record,anchors,calls,callers,callingPrograms,
    shown,parent,ancestors,rootOf,leaves,within,programOf,markersOf,memberMarkersOf,
    kindGroupOf,
  };
  model.homePairs=homePairsOf(model,saved.programPairs,edgesAt);
  model.frameGroups=id=>frameGroupsOf(model,id);
  return model;
}

// The whole map's pairs (owner, 2026-10-01, on the skeptic's verdict): one
// arrow per saved pair of programs, both ways on it, each way holding the
// edges of the relations saved for it; then, folded by the boxes the map
// draws, each program's Inputs frame into it and a program into each
// Outside frame it calls. A pair of programs joined only by code use (no
// way `runtime`) is `uses`: drawn only while one of them is pointed at or
// chosen. An edge from an outside system into an input stands only in the
// pairs saved for it, its callers' (connects_to).
function homePairsOf(model,programPairs,edgesAt){
  const {nodes}=model,pairs=new Map(),program=id=>nodes.get(id)?.kind==='program';
  const add=(a,b,id,runtime)=>{
    if(!a||!b||a===b)return;
    const key=a<b?`${a}|${b}`:`${b}|${a}`;
    if(!pairs.has(key))pairs.set(key,{key,from:a,to:b,forward:[],backward:[],operation:false});
    const pair=pairs.get(key),way=pair.from===a?pair.forward:pair.backward;
    if(!way.includes(id))way.push(id);
    pair.operation||=runtime;
  };
  const saved=new Set();
  for(const pair of programPairs){
    if(!program(pair?.from)||!program(pair?.to)||pair.from===pair.to)continue;
    for(const position of pair.relations||[]){
      const id=edgesAt.get(position);
      if(id){saved.add(id);add(pair.from,pair.to,id,!!pair.runtime);}
    }
  }
  for(const edge of model.edges){
    if(saved.has(edge.id)||nodes.get(edge.from)?.kind==='system'&&nodes.get(edge.to)?.kind==='input')continue;
    const from=model.rootOf(edge.from),to=model.rootOf(edge.to);
    if(program(from)&&program(to))continue;
    add(from,to,edge.id,false);
  }
  for(const pair of pairs.values())pair.uses=program(pair.from)&&program(pair.to)&&!pair.operation;
  return [...pairs.values()];
}

// A frame's connections as the reading column groups them (canvas.jsx
// frameConnections): its members' edges by the participant at the other
// end, another program, or the outermost area under a shared parent, and
// the direction. Each group is {key, area, outside, incoming, insides,
// relations, edges}.
function frameMembers(model,id){
  const own=model.leaves(id).filter(leaf=>model.nodes.get(leaf)?.kind!=='input');
  return model.nodes.get(id)?.kind==='program'?[id,...own]:own;
}
function outsideOf(model,frame){
  const boundary=id=>{
    const shared=new Set([frame,...model.ancestors(frame)]);
    let found='';for(const at of model.ancestors(id))if(model.nodes.get(at)?.kind==='area'&&!shared.has(at))found=at;
    return found;
  };
  return id=>model.rootOf(id)!==model.rootOf(frame)?model.rootOf(id):boundary(id)||id;
}
function frameGroupsOf(model,id){
  if(!model.nodes.has(id))return [];
  return connections(id,frameMembers(model,id),model.edges,outsideOf(model,id));
}

// The closed component lists its existing direct areas and loose parts in
// saved order. Names never create a child or merge equal named children.
export const programContents=(model,node)=>(node.children||[]).map(id=>model.nodes.get(id))
  .filter(child=>child&&['area','part'].includes(child.kind)).map(child=>({id:child.id,title:child.name}));
