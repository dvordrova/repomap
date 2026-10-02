// Seeded synthetic graphs for the canvas's invariant checks: page data in
// the exact shape the report hands the canvas (29-operation-view.js layout:
// records, relations, areas, inputOwner), with nothing a run would not
// give. Each is written once to fixtures/synthetic-<name>.json:
//   node visual/synthetic-graphs.mjs
// The browser draws one with the fixture page (/?graph=synthetic-<name>);
// node scene tests read the JSON directly.
import {writeFile} from 'node:fs/promises';
import {sceneOf} from './saved-scene.mjs';

function mulberry32(seed){
  let s=seed>>>0;
  return ()=>{s=(s+0x6D2B79F5)>>>0;let t=s;t=Math.imul(t^(t>>>15),t|1);t^=t+Math.imul(t^(t>>>7),t|61);return ((t^(t>>>14))>>>0)/4294967296;};
}

const words=['account','session','token','ledger','invoice','policy','tenant','report','audit','queue','cache','index','schema','export','import',
  'billing','profile','device','webhook','storage','search','metrics','alert','backup','replica','snapshot','config','plugin','router','gateway'];
const systemNames=['PostgreSQL','MySQL','Redis','Amazon S3','Google Cloud Storage','Azure Blob Storage','Kafka','RabbitMQ','NATS','SQS',
  'Stripe','PayPal','Twilio','SendGrid','Slack','GitHub API','GitLab API','LDAP','Okta','Auth0','Elasticsearch','OpenSearch','etcd','Consul',
  'Vault','Prometheus','OpenTelemetry collector','Sentry','Datadog','Docker Engine','Kubernetes API','SQLite','MongoDB','Cassandra','ClickHouse',
  'git','ffmpeg','ImageMagick','DNS resolver','SMTP server','LLM API','Discord','Telegram','Facebook OAuth','Google OAuth','WeChat','Alipay',
  'Microsoft Graph','Dropbox','Box'];
const kinds=['database','request','sdk','queue','started'];
const activations=['request','command','continuous','interaction','extension'];

function builder(seed){
  const rnd=mulberry32(seed),pick=list=>list[Math.floor(rnd()*list.length)],chance=p=>rnd()<p;
  const records=[],relations=[],inputOwner={},byID=new Map();
  const add=record=>{records.push(record);byID.set(record.id,record);return record;};
  // A call between parts is an operation two times in three (litestream's
  // page: 212 of 327); a call out to a system is structure, as the page's.
  const relation=(from,to,extra={})=>relations.push({from,to,scope:byID.get(to)?.category==='external'||!chance(2/3)?'structure':'operation',fromNoSource:false,toNoSource:false,operations:[],possible:false,init:false,
    calls:[{label:`${byID.get(from)?.title||from} calls ${byID.get(to)?.title||to}`,at:`${from}.go:${1+Math.floor(rnd()*400)}`,callee:''}],label:'',...extra});
  const program=(t,title,role)=>add({id:`system-component-${t}`,title,branch:'component',summary:`${title} ${role}.`,sourceKind:'model',role,
    language:'go',componentKind:'executable',children:[],kind:'Component',category:'component'});
  let g=0;
  const part=(t,holder,lane='')=>{
    g++;const name=`${pick(words)} ${pick(words)}`.replace(/^./,c=>c.toUpperCase());
    const record=add({id:`n-${t}-g${g}`,title:name,summary:`Handles ${name.toLowerCase()} for its program.`,kind:lane==='core'?'Core':lane==='triggers'?'Entrypoints':'Part',
      category:'part',...(lane?{lane}:{})});
    byID.get(holder).children.push(record.id);return record.id;
  };
  let k=0;
  const area=(t,holder,lane='')=>{
    k++;const name=`${pick(words)} ${pick(words)}`.replace(/^./,c=>c.toUpperCase());
    const record=add({id:`${t}-area-k${k}`,title:name,branch:'area',...(lane?{lane}:{}),summary:`Groups the ${name.toLowerCase()} parts.`,children:[],kind:'Area',category:'part'});
    byID.get(holder).children.push(record.id);return record.id;
  };
  let o=0;
  const inputs=(t,program,handlers,count)=>{
    const frame=add({id:`system-inputs-${t}`,title:byID.get(program).title,branch:'inputs',lane:'triggers',componentOwner:program,componentName:byID.get(program).title,
      children:[],kind:'Inputs',category:'input'});
    for(let i=0;i<count;i++){
      o++;const activation=pick(activations),id=`${t}-o${o}`;
      add({id,title:activation==='request'?`${pick(['GET','POST','PUT','DELETE'])} /api/${pick(words)}/${pick(words)}`:activation==='command'?`${pick(words)} ${pick(words)}`:`${pick(words)}-${pick(words)}`,
        activation,lane:'triggers',sourceKind:'fact',trace:[],componentOwner:program,componentName:byID.get(program).title,kind:'Inputs',category:'input',
        ...(chance(.15)?{unestablished:true}:{})});
      frame.children.push(id);
      // Most inputs have a handler; the rest are declared where they stand.
      if(chance(.8)){const handler=pick(handlers);inputOwner[id]=handler;relations.push({from:id,to:handler,scope:'operation',fromNoSource:false,toNoSource:false,
        operations:[id],possible:false,init:false,calls:[],label:'implemented in'});}
    }
    return frame.id;
  };
  let b=0;
  const outside=(id,title,systems,callers)=>{
    const frame=add({id,title,branch:'outside',lane:'dependencies',children:[],kind:'External communication',category:'external'});
    for(const system of systems){
      b++;const destination=add({id:`${id}-b${b}-destination`,title:system.name,branch:'communication',lane:'dependencies',destinationKind:system.kind,children:[],
        kind:'External communication',category:'external'});
      frame.children.push(destination.id);
      const calls=1+Math.floor(rnd()*3);
      for(let c=0;c<calls;c++){
        b++;const call=add({id:`${id}-b${b}`,title:`${pick(words)}${pick(['Get','Put','Query','Send','Open','List'])}`,lane:'dependencies',sourceKind:'fact',kind:'External communication',category:'external'});
        destination.children.push(call.id);
        for(const from of system.callers?.length?system.callers:[pick(callers)])relation(from,call.id);
      }
    }
    return frame.id;
  };
  const finish=(title,about)=>{
    const page={records,relations,areas:records.filter(r=>r.children).map(r=>({id:r.id,nodes:r.children})),inputOwner};
    return {title,about,seed,...page,scene:sceneOf(page)};
  };
  return {rnd,pick,chance,add,relation,program,part,area,inputs,outside,finish,byID};
}

// Two programs, areas and loose parts, outside systems, and no input at all.
function noInputs(){
  const seed=1001,b=builder(seed),parts={t1:[],t2:[]};
  for(const [t,title] of [['t1','api-server'],['t2','worker']]){
    const p=b.program(t,title,t==='t1'?'serves the HTTP API':'runs background jobs');
    for(let a=0;a<4;a++){const area=b.area(t,p.id,a===0?'core':'');for(let i=0;i<3+Math.floor(b.rnd()*3);i++)parts[t].push(b.part(t,area,a===0&&i===0?'triggers':''));}
    for(let i=0;i<2;i++)parts[t].push(b.part(t,p.id));
    for(let i=0;i<parts[t].length*1.5;i++){const from=b.pick(parts[t]),to=b.pick(parts[t]);if(from!==to)b.relation(from,to);}
    b.outside(`system-outside-${t}`,title,systemNames.slice(t==='t1'?0:5,t==='t1'?5:9).map((name,i)=>({name,kind:kinds[i%kinds.length]})),parts[t]);
  }
  b.relation(b.pick(parts.t1),b.pick(parts.t2));
  return b.finish('No inputs · two programs','Two programs with areas, loose parts and outside systems, and no input, Inputs frame or inputOwner entry at all.');
}

// One large program and a small one calling 150 outside systems, a few of
// them shared, some used by many parts.
function outside150(){
  const seed=1502,b=builder(seed),parts={t1:[],t2:[]};
  const big=b.program('t1','casdoor-like','serves identity and single sign-on'),small=b.program('t2','sync-tool','synchronises users');
  for(let a=0;a<6;a++){const area=b.area('t1',big.id,a<2?'core':'');for(let i=0;i<4;i++)parts.t1.push(b.part('t1',area,a===0&&i===0?'triggers':''));}
  for(let i=0;i<3;i++)parts.t1.push(b.part('t1',big.id));
  const area=b.area('t2',small.id,'core');for(let i=0;i<3;i++)parts.t2.push(b.part('t2',area,i===0?'triggers':''));
  for(const t of ['t1','t2'])for(let i=0;i<parts[t].length*1.3;i++){const from=b.pick(parts[t]),to=b.pick(parts[t]);if(from!==to)b.relation(from,to);}
  b.inputs('t1',big.id,parts.t1,40);b.inputs('t2',small.id,parts.t2,5);
  // Past the fifty names, a region tells copies apart: a digit would read
  // as a printed number on the canvas.
  const regions=['','EU','US'],names=[];for(let i=0;i<150;i++)names.push(`${systemNames[i%systemNames.length]}${regions[Math.floor(i/systemNames.length)]?` ${regions[Math.floor(i/systemNames.length)]}`:''}`);
  // Names that carry a digit are names, not printed counts: two shared
  // systems the whole map shows at rest, and two copies.
  names[2]='Route 53';names[4]='Python 3';names[60]='Web 2.0';names[61]='Dropbox 3';
  // A few systems used by many parts; most by one; identity providers by
  // one part, many of them.
  const systems=names.map((name,i)=>({name,kind:kinds[i%kinds.length],callers:i<4?parts.t1.filter(()=>b.chance(.5)).slice(0,12):i<20?[b.pick(parts.t1),b.pick(parts.t1)]:i<60?[parts.t1[3]]:[b.pick(parts.t1)]}));
  b.outside('system-outside-t1','casdoor-like',systems.slice(0,146),parts.t1);
  b.outside('system-outside-t2','sync-tool',systems.slice(146).map(s=>({...s,callers:[b.pick(parts.t2)]})),parts.t2);
  b.outside('system-outside-t1-t2','casdoor-like, sync-tool',[{name:'PostgreSQL (shared)',kind:'database',callers:[b.pick(parts.t1),b.pick(parts.t2)]}],parts.t1);
  b.relation(b.pick(parts.t2),b.pick(parts.t1));
  return b.finish('150 outside systems','A large program and a small one calling 150 outside systems: four used by many parts, sixteen by two, forty by one identity-provider part, the rest by one part each; one shared frame.');
}

// Cycles everywhere: programs calling each other round, areas calling
// round, parts in three-cycles and mutual pairs.
function cycles(){
  const seed=2203,b=builder(seed),parts={},areas={};
  for(const [t,title] of [['t1','alpha'],['t2','beta'],['t3','gamma']]){
    const p=b.program(t,title,'calls the next program round');parts[t]=[];areas[t]=[];
    for(let a=0;a<3;a++){const area=b.area(t,p.id,a===0?'core':'');areas[t].push([]);for(let i=0;i<3;i++){const id=b.part(t,area,a===0&&i===0?'triggers':'');parts[t].push(id);areas[t][a].push(id);}}
    // Each area's parts call round; the areas call round; one mutual pair.
    for(const list of areas[t])for(let i=0;i<list.length;i++)b.relation(list[i],list[(i+1)%list.length]);
    for(let a=0;a<3;a++)b.relation(areas[t][a][0],areas[t][(a+1)%3][1]);
    b.relation(areas[t][0][2],areas[t][1][2]);b.relation(areas[t][1][2],areas[t][0][2]);
    b.inputs(t,p.id,parts[t],3);
    b.outside(`system-outside-${t}`,title,[{name:systemNames[parts[t].length%systemNames.length],kind:'database'},{name:'Kafka',kind:'queue'}],parts[t]);
  }
  b.relation(parts.t1[1],parts.t2[4]);b.relation(parts.t2[1],parts.t3[4]);b.relation(parts.t3[1],parts.t1[4]);
  b.relation(parts.t2[7],parts.t1[7]);b.relation(parts.t1[7],parts.t2[7]);
  return b.finish('Cycles','Three programs calling each other round, each with three areas calling round, parts in three-cycles and mutual pairs in and across programs.');
}

// One program of forty loose parts beside two small areas.
function loose40(){
  const seed=4004,b=builder(seed),parts=[];
  const p=b.program('t1','scripts','a script collection');
  for(let a=0;a<2;a++){const area=b.area('t1',p.id,a===0?'core':'');for(let i=0;i<3;i++)parts.push(b.part('t1',area,a===0&&i===0?'triggers':''));}
  const loose=[];for(let i=0;i<40;i++){const id=b.part('t1',p.id);loose.push(id);parts.push(id);}
  for(let i=0;i+1<loose.length;i+=3)b.relation(loose[i],loose[i+1]);
  for(let i=0;i<30;i++){const from=b.pick(parts),to=b.pick(parts);if(from!==to)b.relation(from,to);}
  b.inputs('t1',p.id,parts,12);
  b.outside('system-outside-t1','scripts',systemNames.slice(10,18).map((name,i)=>({name,kind:kinds[i%kinds.length]})),loose);
  return b.finish('Forty loose parts','One program with forty parts in no area beside two small areas, chained and randomly connected, with inputs and outside systems.');
}

export const graphs={'no-inputs':noInputs,'outside-150':outside150,cycles,'loose-40':loose40};

// A graph is page data a run could give: unique ids, every child, relation
// end and input owner a record, every record but a root held once.
export function check(graph){
  const ids=new Set(),problems=[],held=new Map();
  for(const r of graph.records){if(ids.has(r.id))problems.push(`id ${r.id} twice`);ids.add(r.id);}
  for(const r of graph.records)for(const child of r.children||[]){if(!ids.has(child))problems.push(`${r.id} holds missing ${child}`);held.set(child,(held.get(child)||0)+1);}
  for(const [id,n] of held)if(n>1)problems.push(`${id} held ${n} times`);
  for(const r of graph.records)if(!held.has(r.id)&&!['component','inputs','outside'].includes(r.branch))problems.push(`${r.id} held by nothing`);
  for(const e of graph.relations)for(const end of [e.from,e.to])if(!ids.has(end))problems.push(`relation end ${end} missing`);
  for(const [input,owner] of Object.entries(graph.inputOwner))if(!ids.has(input)||!ids.has(owner))problems.push(`inputOwner ${input} → ${owner}`);
  for(const a of graph.areas)if(!ids.has(a.id)||a.nodes.some(n=>!ids.has(n)))problems.push(`area ${a.id}`);
  return problems;
}

if(import.meta.url===`file://${process.argv[1]}`){
  for(const [name,make] of Object.entries(graphs)){
    const graph=make(),file=new URL(`../fixtures/synthetic-${name}.json`,import.meta.url),problems=check(graph);
    if(problems.length)throw new Error(`${name}: ${problems.slice(0,5).join('; ')}`);
    await writeFile(file,JSON.stringify(graph,null,1)+'\n');
    console.log(`${name}: ${graph.records.length} records, ${graph.relations.length} relations, ${graph.areas.length} areas, ${Object.keys(graph.inputOwner).length} owned inputs`);
  }
}
