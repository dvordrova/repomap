// The saved scene a report would carry for this page (#rm-scene, report.json
// `scene`), read off it by the rules of internal/report/scene.go sceneOf:
// where each input takes effect, who calls each outside system, each
// part's calls and the directed pairs of programs.
const sceneInputKinds=['request','command','setting','scheduled','continuous','interaction','consumer','extension','entry'];
const sceneSystemKinds=['database','request','sdk','queue','started','other'];
const sceneInputKind=a=>a==='background'?'continuous':a==='queue_consumer'?'consumer':sceneInputKinds.includes(a)?a:'entry';
export function sceneOf({records,relations,inputOwner}){
  const byID=new Map(records.map(r=>[r.id,r]));
  const kindOf=r=>r.branch==='component'?'program':['area','inputs','outside'].includes(r.branch)?r.branch:r.branch==='communication'?'system'
    :r.activation?'input':r.category==='external'?'call':r.category==='component'?'note':'part';
  const kind=new Map(records.map(r=>[r.id,kindOf(r)]));
  const children=new Map(records.map(r=>[r.id,(r.children||[]).filter(id=>r.branch==='inputs'||!byID.get(id)?.activation)]));
  const parentOf=new Map();
  for(const r of records)for(const id of children.get(r.id))if(byID.has(id))parentOf.set(id,r.id);
  const systemOfCall=new Map();
  for(const r of records)if(kind.get(r.id)==='system')for(const id of children.get(r.id))if(kind.get(id)==='call')systemOfCall.set(id,r.id);
  const shown=id=>systemOfCall.get(id)||id;
  const edges=[],folded=new Map();
  relations.forEach((relation,i)=>{
    const from=shown(relation.from),to=shown(relation.to);
    if(from===to||!byID.has(from)||!byID.has(to)||kind.get(from)==='call'||kind.get(to)==='call')return;
    const key=JSON.stringify([from,to,!!relation.possible]);
    if(!folded.has(key)){folded.set(key,{from,to,relations:[]});edges.push(folded.get(key));}
    folded.get(key).relations.push(i);
  });
  const program=r=>r.activation||r.branch==='inputs'?r.componentOwner||'':'';
  const nodes=new Map();
  for(const r of records)if(['program','area','part','note'].includes(kind.get(r.id)))nodes.set(r.id,{kind:kind.get(r.id),parent:parentOf.get(r.id)||''});
  const leaves=id=>kind.get(id)==='input'?[id]:children.get(id)?.flatMap(leaves)||[];
  for(const r of records){
    if(kind.get(r.id)!=='inputs')continue;
    nodes.set(r.id,{kind:'inputs',parent:'',program:program(r)});
    for(const id of leaves(r.id)){if(!byID.has(id))continue;const group=`${r.id}#${sceneInputKind(byID.get(id).activation)}`;
      nodes.set(group,{kind:'kind',parent:r.id,program:program(r)});nodes.set(id,{kind:'input',parent:group,program:program(r)});}
  }
  for(const r of records)if(kind.get(r.id)==='input'&&!nodes.has(r.id))nodes.set(r.id,{kind:'input',parent:'',program:program(r)});
  const up=id=>{let at=id;while(nodes.get(at)?.parent)at=nodes.get(at).parent;return at;};
  const partProgram=id=>{const root=up(id);return nodes.get(root)?.kind==='program'?root:'';};
  const callers=new Map(),calling=new Map(),push=(map,key,value)=>{if(!map.has(key))map.set(key,[]);if(!map.get(key).includes(value))map.get(key).push(value);};
  for(const edge of edges){
    if(kind.get(edge.to)!=='system')continue;
    if(kind.get(edge.from)==='part')push(callers,edge.to,edge.from);
    const by=partProgram(edge.from)||(kind.get(edge.from)==='program'?edge.from:'');
    if(by)push(calling,edge.to,by);
  }
  for(const r of records){
    if(kind.get(r.id)!=='outside')continue;
    nodes.set(r.id,{kind:'outside',parent:''});
    for(const id of children.get(r.id))if(kind.get(id)==='system')nodes.set(id,{kind:'system',parent:r.id});
  }
  for(const r of records)if(kind.get(r.id)==='system'&&!nodes.has(r.id))nodes.set(r.id,{kind:'system',parent:''});
  for(const node of nodes.values())if(!nodes.has(node.parent))node.parent='';
  const kindAt=id=>nodes.get(id)?.kind||'';
  const scene={inputs:{},systems:{},calls:{},programPairs:[],reaching:{}};
  const owner=new Map(Object.entries(inputOwner||{}).filter(([,part])=>byID.has(part)));
  for(const relation of relations)if(byID.get(relation.from)?.activation&&byID.has(relation.to)&&!byID.get(relation.to).activation&&relation.label==='implemented in')owner.set(relation.from,relation.to);
  for(const r of records){
    const node=nodes.get(r.id);if(node?.kind!=='input'||scene.inputs[r.id])continue;
    const input={kind:sceneInputKind(r.activation),program:node.program,parts:[],handled:false},takenIn=[],reached=[];
    for(const edge of edges){
      if(edge.from!==r.id||kindAt(edge.to)!=='part')continue;
      if(!reached.includes(edge.to))reached.push(edge.to);
      if(edge.relations.some(i=>['declared in','looked up in'].includes(relations[i].label))&&!takenIn.includes(edge.to))takenIn.push(edge.to);
    }
    const handler=owner.get(r.id);
    if(handler&&kindAt(handler)==='part'){input.parts=[handler];input.handled=true;}
    else if(takenIn.length)input.parts=takenIn;
    else if(reached.length){input.parts=reached;input.handled=true;}
    scene.inputs[r.id]=input;
  }
  for(const r of records)if(kind.get(r.id)==='system'&&!scene.systems[r.id])
    scene.systems[r.id]={kind:sceneSystemKinds.includes(r.destinationKind)?r.destinationKind:'other',parts:[...(callers.get(r.id)||[])],programs:[...(calling.get(r.id)||[])]};
  // No synthetic call names its caller's declaration: one call per system.
  for(const edge of edges){
    if(kind.get(edge.to)!=='system'||kindAt(edge.from)!=='part')continue;
    const list=scene.calls[edge.from]||=[];
    if(!list.some(call=>call.system===edge.to))list.push({system:edge.to});
  }
  const pairs=new Map();
  const add=(from,to,edge,runtime)=>{
    if(!from||!to||from===to||kindAt(from)!=='program'||kindAt(to)!=='program')return;
    const key=`${from}\0${to}`;
    if(!pairs.has(key)){pairs.set(key,scene.programPairs.length);scene.programPairs.push({from,to,runtime:false,relations:[]});}
    const pair=scene.programPairs[pairs.get(key)];pair.runtime||=runtime;
    for(const i of edge.relations)if(!pair.relations.includes(i))pair.relations.push(i);
  };
  for(const edge of edges){
    const from=nodes.get(edge.from),to=nodes.get(edge.to);
    let sources=[up(edge.from)],target=up(edge.to);
    if(to?.kind==='input'&&to.program&&up(edge.from)!==to.program)target=to.program;
    if(from?.kind==='input'&&from.program&&up(edge.to)!==from.program)sources=[from.program];
    if(from?.kind==='system'&&to?.kind==='input'){target=to.program||target;for(const by of calling.get(edge.from)||[])add(by,target,edge,true);continue;}
    const runtime=edge.relations.some(i=>relations[i].scope!=='structure');
    for(const source of sources)add(source,target,edge,runtime);
  }
  // The inputs reaching each part and outside system along their saved
  // paths: the relations listing an input among their operations
  // (report.json 96).
  const standsFor=new Map(),stand=(id,what)=>{if(!standsFor.has(id))standsFor.set(id,[]);if(!standsFor.get(id).includes(what))standsFor.get(id).push(what);};
  for(const r of records){
    if(kind.get(r.id)==='part')stand(r.id,r.id);
    if(kind.get(r.id)==='system'){stand(r.id,r.id);for(const id of r.children||[])if(byID.has(id))stand(id,r.id);}
  }
  const paths=new Map();
  relations.forEach((relation,i)=>{for(const input of relation.operations||[]){if(!paths.has(input))paths.set(input,[]);paths.get(input).push(i);}});
  for(const r of records){
    if(!r.activation)continue;
    for(const i of paths.get(r.id)||[])for(const end of [relations[i].from,relations[i].to])for(const reached of standsFor.get(end)||[]){
      const list=scene.reaching[reached]||=[];if(list.at(-1)!==r.id)list.push(r.id);
    }
  }
  return scene;
}
