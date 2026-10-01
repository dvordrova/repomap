// The display graph the scene canvas draws (PLAN S1), built once from the
// page's data: what stands in what, the arrows between the things drawn,
// the Outside frames grouped by part (B′) and the markers every box
// carries. It is pure: no layout, no camera, no DOM. The page's records,
// relations and readings are unchanged; this is display containment only.
import {prepareCards} from './cards.mjs';

// An input's kind, as the reading column names its sections.
export const inputKinds=['request','command','setting','scheduled','continuous','interaction','consumer','extension','entry'];
export const inputKindTitles={request:'Incoming requests',command:'Commands',setting:'Settings',scheduled:'Scheduled tasks',
  continuous:'Background work',interaction:'User interactions',consumer:'Queue consumers',extension:'Extension points',entry:'Kind not established'};
export function inputKind(activation){
  if(activation==='background')return 'continuous';
  if(activation==='queue_consumer')return 'consumer';
  return inputKinds.includes(activation)?activation:'entry';
}
// An outside system's kind, as its calls' facts give it
// (page_system_map.go destinationKind); 'other' when they give none.
export const systemKinds=['database','request','sdk','queue','started','other'];
export const systemKind=kind=>systemKinds.includes(kind)&&kind!=='other'?kind:'other';
// Inputs whose handler is not established are taken in where these say.
const takenIn=new Set(['declared in','looked up in']);
// At most this many markers stand on a side of a box (PLAN B).
export const markersPerSide=3;

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

// `page` {items, relations, areas, inputOwner} as the page hands them to
// rmCreateFlow; `measure(text, font)` measures text; `t` translates fixed
// words. Returns the model (see the fields at the end).
export function buildModel(page,{measure,t=text=>text}={}){
  const items=prepareCards(page.items||[],page.inputOwner||{},measure,t);
  const record=new Map(items.map(item=>[item.id,item]));
  const inputOwner=page.inputOwner||{};
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
  const folded=new Map();
  for(const relation of page.relations||[]){
    const from=shown(relation.displayFrom||relation.from),to=shown(relation.displayTo||relation.to);
    if(from===to||!record.has(from)||!record.has(to))continue;
    if(kind.get(from)==='call'||kind.get(to)==='call')continue;
    const key=JSON.stringify([from,to,!!relation.possible]);
    if(!folded.has(key))folded.set(key,{id:`e${folded.size}`,from,to,possible:!!relation.possible,init:true,relations:[]});
    const edge=folded.get(key);edge.relations.push(relation);edge.init&&=!!relation.init;
  }
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
      const k=inputKind(record.get(id).activation);
      if(!groups.has(k))groups.set(k,[]);
      if(!groups.get(k).includes(id))groups.get(k).push(id);
    }
    for(const k of inputKinds.filter(k=>groups.has(k))){
      const group=add({id:`${item.id}#${k}`,kind:'kind',inputKind:k,name:t(inputKindTitles[k]),parent:item.id,program:collection.program,children:groups.get(k)});
      collection.children.push(group.id);
      for(const id of group.children){
        kindGroupOf.set(id,group.id);
        add({id,kind:'input',name:name(record.get(id)),item:record.get(id),parent:group.id,inputKind:k,program:collection.program});
      }
    }
  }
  // An input no collection holds stands alone.
  for(const item of items)if(kind.get(item.id)==='input'&&!nodes.has(item.id))
    add({id:item.id,kind:'input',name:name(item),item,parent:'',inputKind:inputKind(item.activation),program:item.componentOwner||''});

  const partProgram=id=>{let at=id;while(nodes.get(at)?.parent)at=nodes.get(at).parent;return nodes.get(at)?.kind==='program'?at:'';};
  // Who calls each system: the parts whose arrows go into it, and the
  // programs (a call from a program's own code or another drawn thing).
  const callers=new Map(),callingPrograms=new Map();
  for(const edge of edges){
    if(kind.get(edge.to)!=='system')continue;
    if(!callers.has(edge.to))callers.set(edge.to,new Set());
    if(kind.get(edge.from)==='part')callers.get(edge.to).add(edge.from);
    const program=partProgram(edge.from)||(kind.get(edge.from)==='program'?edge.from:'');
    if(program){if(!callingPrograms.has(edge.to))callingPrograms.set(edge.to,new Set());callingPrograms.get(edge.to).add(program);}
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
      systemKind:systemKind(record.get(id).destinationKind),unestablished:!!record.get(id).unestablished});
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
    add({id:item.id,kind:'system',name:name(item),item,parent:'',systemKind:systemKind(item.destinationKind)});

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

  // Where each input takes effect: the part holding its handler, else the
  // parts its code takes it in (declared in, looked up in), else its
  // program. `handled` says which.
  const anchors=new Map();
  for(const node of nodes.values()){
    if(node.kind!=='input')continue;
    const owner=inputOwner[node.id];
    if(owner&&nodes.get(owner)?.kind==='part'){anchors.set(node.id,{parts:[owner],handled:true});continue;}
    const parts=[...new Set(edges.filter(edge=>edge.from===node.id&&nodes.get(edge.to)?.kind==='part'&&edge.relations.some(r=>takenIn.has(r.label))).map(edge=>edge.to))];
    if(parts.length){anchors.set(node.id,{parts,handled:false});continue;}
    const reached=[...new Set(edges.filter(edge=>edge.from===node.id&&nodes.get(edge.to)?.kind==='part').map(edge=>edge.to))];
    anchors.set(node.id,reached.length?{parts:reached,handled:true}:{parts:[],handled:false,program:node.program||''});
  }
  // The systems each part calls.
  const calls=new Map();
  for(const edge of edges){
    if(kind.get(edge.to)!=='system'||nodes.get(edge.from)?.kind!=='part')continue;
    if(!calls.has(edge.from))calls.set(edge.from,new Set());
    calls.get(edge.from).add(edge.to);
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
  // declaration handling or declaring the input, or making the outside
  // call; what names no declaration of the part stays on the part's edge.
  // {members: Map symbol index → {in, out}, rest: {in, out}}.
  const memberCache=new Map();
  function memberMarkersOf(part){
    if(memberCache.has(part))return memberCache.get(part);
    const symbols=nodes.get(part)?.item?.symbols||[],at=new Map();
    symbols.forEach((symbol,i)=>{for(const key of [symbol.href,symbol.open])if(key&&!at.has(key))at.set(key,i);});
    const byMember=new Map(),rest={in:new Map(),out:new Map()};
    const side=(i,way)=>{
      if(i===undefined)return rest[way];
      if(!byMember.has(i))byMember.set(i,{in:new Map(),out:new Map()});
      return byMember.get(i)[way];
    };
    for(const edge of edges){
      if(edge.to===part&&nodes.get(edge.from)?.kind==='input'){
        const input=nodes.get(edge.from),anchor=anchors.get(input.id);
        // An input with no known handler stays on the part's edge.
        const handled=anchor?.handled&&anchor.parts.includes(part);
        const places=new Set(handled?edge.relations.filter(r=>!takenIn.has(r.label)).flatMap(r=>(r.calls||[]).map(call=>at.get(call.to||call.callee))):[]);
        places.delete(undefined);
        for(const i of places.size?places:[undefined]){
          const list=side(i,'in');
          if(!list.has(input.inputKind))list.set(input.inputKind,{kind:input.inputKind,members:[],systems:[],handled:[]});
          const marker=list.get(input.inputKind);
          if(!marker.members.includes(input.id)){marker.members.push(input.id);if(handled)marker.handled.push(input.id);}
        }
      }
      if(edge.from===part&&kind.get(edge.to)==='system'){
        const k=nodes.get(edge.to)?.systemKind||'other';
        const places=new Set(edge.relations.flatMap(r=>(r.calls||[]).map(call=>at.get(call.caller))));
        places.delete(undefined);
        for(const i of places.size?places:[undefined]){
          const list=side(i,'out');
          if(!list.has(k))list.set(k,{kind:k,members:[part],systems:[],handled:[]});
          const marker=list.get(k);if(!marker.systems.includes(edge.to))marker.systems.push(edge.to);
        }
      }
    }
    const finish=sides=>({in:capped([...sides.in.values()],m=>m.members.length,inputKinds),out:capped([...sides.out.values()],m=>m.systems.length,systemKinds)});
    const result={members:new Map([...byMember].map(([i,sides])=>[i,finish(sides)])),rest:finish(rest)};
    memberCache.set(part,result);
    return result;
  }

  return {
    nodes,roots,edges,record,inputOwner,anchors,calls,callingPrograms,
    shown,parent,ancestors,rootOf,leaves,within,programOf,markersOf,memberMarkersOf,
    kindGroupOf,
  };
}

// The model's things a scene can draw as boxes, by kind: what a box at
// each level stands for.
export const containerKinds=new Set(['program','area','inputs','kind','outside','bucket']);
