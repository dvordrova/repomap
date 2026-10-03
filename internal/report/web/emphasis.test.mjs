import {test} from 'node:test';
import assert from 'node:assert/strict';
import {emphasis,focusAncestors,endEmphasis,recedes,quietFrame} from './emphasis.mjs';

const leaves=id=>id==='area'?['handler','paint']:id==='other'?['test']: [id];
const edges=[
  {id:'hp',from:'handler',to:'paint',relations:[{operations:['input']}]},
  {id:'ht',from:'handler',to:'test',relations:[]},
  {id:'pt',from:'paint',to:'test',relations:[]},
  {id:'elsewhere',from:'test',to:'unrelated',relations:[]},
];
const empty={scope:'',operation:'',entry:'',selected:new Set(),matched:new Set(),searching:false};
test('hovering a child retains its component frame without borrowing sibling arrows',()=>{
  const placed=new Map([['front',{}],['utility',{parentId:'front'}],['area',{parentId:'front'}],['handler',{parentId:'area'}]]);
  const result=emphasis(empty,'utility',id=>[id],[...edges,{id:'own',from:'utility',to:'test',relations:[]}]);
  assert.deepEqual([...focusAncestors(result.focus,placed)],['front']);
  assert.deepEqual([...result.activeEdges],['own']);
  assert.deepEqual([...focusAncestors(new Set(['handler']),placed)],['area','front']);
  assert.deepEqual([...focusAncestors(new Set(),placed)],[]);
});
test('hover names the whole area and darkens only the arrows that cross its border',()=>{
  const result=emphasis(empty,'area',leaves,edges);
  assert.equal(result.mode,'hover');assert.equal(result.subject,'area');
  assert.deepEqual([...result.focus],['area','handler','paint']);
  assert.deepEqual([...result.activeEdges],['ht','pt'],'handler -> paint stays inside the area');
  assert.ok(result.participants.has('test'));
  assert.ok(!result.participants.has('unrelated'));
});
test('hover replaces the drawing emphasis without mixing in a clicked part; leaving restores it',()=>{
  const view={...empty,scope:'paint',selected:new Set(['paint'])};
  const before=emphasis(view,'',leaves,edges),after=emphasis(view,'other',leaves,edges);
  assert.equal(after.mode,'hover');assert.equal(after.subject,'other');
  assert.deepEqual([...after.activeEdges],['ht','pt','elsewhere']);
  assert.ok(!after.activeEdges.has('hp'),'old selected links are not mixed into hover');
  assert.deepEqual([...before.activeEdges],['hp','pt']);
  assert.deepEqual(emphasis(view,'',leaves,edges),before);
});
test('reading an off-path part never colours it as an input participant',()=>{
  const view={...empty,operation:'input',entry:'handler',scope:'test',selected:new Set(['test'])};
  const result=emphasis(view,'',leaves,edges);
  assert.deepEqual([...result.activeEdges],['hp']);
  assert.deepEqual([...result.participants],['handler','paint']);
  const hovered=emphasis(view,'other',leaves,edges);
  assert.equal(hovered.mode,'hover','a pinned input never disables exploring other areas');
  assert.ok(hovered.activeEdges.has('elsewhere'));
  assert.ok(!hovered.activeEdges.has('hp'),'input path is suspended, not combined with hover');
});
test('search matches are not combined with an old input or hover path, including zero matches',()=>{
  const view={...empty,searching:true,operation:'input',entry:'handler',matched:new Set(['test'])};
  const result=emphasis(view,'area',leaves,edges);
  assert.equal(result.mode,'search');
  assert.deepEqual([...result.participants],['test']);assert.equal(result.activeEdges.size,0);
  assert.equal(emphasis({...view,matched:new Set()},'area',leaves,edges).participants.size,0);
});

// Pointing at a declaration in a zoomed part darkens only its own arrows:
// Redis's acceptHandler lit every arrow of Client connections and replies.
test('a declaration pointed at or chosen darkens only the arrows that carry its calls',()=>{
  const part=[
    {id:'in',from:'events',to:'clients',relations:[{calls:[{kind:'calls',caller_name:'aeMain',callee_name:'acceptHandler',caller:'ae.c#L5',callee:'redis.c#L2551',from:'ae.c#L10',to:'redis.c#L2551'}]}]},
    {id:'out',from:'clients',to:'net',relations:[{calls:[{kind:'calls',caller_name:'acceptHandler',callee_name:'anetAccept',caller:'redis.c#L2551',from:'redis.c#L2560',to:'anet.c#L300'}]}]},
    {id:'other',from:'clients',to:'lists',relations:[{calls:[{kind:'calls',caller_name:'createClient',callee_name:'listCreate',caller:'redis.c#L80',from:'redis.c#L90',to:'adlist.c#L40'}]}]},
    // A callable written inline in acceptHandler is a declaration of its
    // own: its name names acceptHandler, and its calls are not
    // acceptHandler's.
    {id:'inline',from:'clients',to:'strings',relations:[{calls:[{kind:'calls',caller_name:'one of two anonymous functions in acceptHandler',callee_name:'sdsnew',caller:'redis.c#L2570',from:'redis.c#L2571',to:'sds.c#L9'}]}]},
  ];
  const member={part:'clients',names:['acceptHandler'],sources:['redis.c#L2551']};
  const hovered=emphasis(empty,'clients',id=>[id],part,member);
  assert.deepEqual([...hovered.activeEdges],['in','out']);
  assert.ok(!hovered.participants.has('lists'));
  const chosen=emphasis({...empty,scope:'clients',selected:new Set(['clients'])},'',id=>[id],part,member);
  assert.deepEqual([...chosen.activeEdges],['in','out']);
  assert.deepEqual([...emphasis(empty,'clients',id=>[id],part).activeEdges],['in','out','other','inline'],'the part itself keeps all its arrows');
  assert.deepEqual([...emphasis(empty,'events',id=>[id],part,member).activeEdges],['in'],'a declaration of another part changes nothing');
});

// An arrow end looked at outlines in place the parts behind it: they are the
// subject, the end's own arrows to them are dark and the parts at the
// arrows' other end stay.
test('an arrow end looked at makes the parts behind it the subject',()=>{
  const edges=[{id:'e1',from:'runtime',to:'dict'},{id:'e2',from:'runtime',to:'sds'},{id:'e3',from:'dict',to:'sds'},{id:'e4',from:'cmds',to:'dict'}];
  const label={insides:['dict','sds'],edges:['e1','e2']};
  const whole=endEmphasis(label,edges);
  assert.deepEqual([...whole.focus],['dict','sds']);
  assert.deepEqual([...whole.activeEdges].sort(),['e1','e2'],'only the end\'s own arrows are dark');
  assert.ok(whole.participants.has('runtime'),'the other end is involved');
  assert.ok(!whole.participants.has('cmds'),'another frame\'s arrow into the same part is not');
  assert.equal(whole.mode,'hover');
});

// Moving the pointer across Redis's Data type commands dimmed every other
// part on each tile and restored them in the gaps between tiles: the area
// flickered. The pointer highlights and never recedes; only the reader's
// choice does, and the pointer brings forward only what it highlights.
test('the pointer recedes nothing; a choice recedes what it does not involve',()=>{
  const inside=id=>[id,...(id==='area'||id==='other'?leaves(id):[])];
  const pointing=emphasis(empty,'handler',leaves,edges);
  const idle=recedes(emphasis(empty,'',leaves,edges),pointing,new Set(['handler']),inside);
  for(const id of ['handler','paint','test','unrelated','area','other'])assert.equal(idle(id),false,`${id} stays as it is at rest`);
  const view={...empty,scope:'paint',selected:new Set(['paint'])},rest=emphasis(view,'',leaves,edges);
  const chosen=recedes(rest,rest,new Set(['paint']),inside);
  assert.deepEqual(['handler','paint','test','unrelated'].filter(chosen),['unrelated']);
  const {participants}=emphasis(view,'unrelated',leaves,edges);
  assert.ok(participants.has('test'));
  const pointed=recedes(rest,emphasis(view,'unrelated',leaves,edges),new Set(['unrelated']),inside);
  assert.deepEqual(['handler','paint','test','unrelated'].filter(pointed),[],'the pointed part comes forward, nothing recedes further');
  // A frame pointed at comes forward with the parts across its arrows; its
  // own parts stay as the choice left them.
  const far={...empty,scope:'unrelated',selected:new Set(['unrelated'])};
  const framed=recedes(emphasis(far,'',leaves,edges),emphasis(far,'area',leaves,edges),new Set(['area']),inside);
  assert.deepEqual(['area','handler','paint','test','unrelated'].filter(framed),['handler','paint']);
});

// Othello's Game logic, chosen, had drawn none of its quiet arrows: the
// area chosen keeps its parts looked at while the pointer crosses them.
test('quiet arrows are drawn in the area chosen or zoomed into, whatever the pointer does',()=>{
  const frameOf=id=>id==='handler'?'area':id;
  const chosen={scope:'area',operation:'',selected:new Set(['area']),matched:new Set(),searching:false};
  const rest=emphasis(chosen,'',leaves,[]);
  assert.equal(quietFrame(rest,'area','',frameOf),'area');
  assert.equal(quietFrame(emphasis({...chosen,scope:'handler'},'',leaves,[]),'handler','',frameOf),'area','a part chosen is read in its area');
  const idle={scope:'',operation:'',selected:new Set(),matched:new Set(),searching:false};
  assert.equal(quietFrame(emphasis(idle,'',leaves,[]),'','area',frameOf),'area','the one area zoomed into');
  assert.equal(quietFrame(emphasis({...idle,searching:true},'',leaves,[]),'','area',frameOf),'','a search looks at its matches');
});
