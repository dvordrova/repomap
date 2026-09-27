import {test} from 'node:test';
import assert from 'node:assert/strict';
import {emphasis,focusAncestors,endEmphasis} from './emphasis.mjs';

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
  assert.equal(result.readingOutside,true);
  assert.equal(emphasis({...view,scope:'paint'},'',leaves,edges).readingOutside,false);
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
    {id:'in',from:'events',to:'clients',relations:[{calls:[{label:'aeMain calls acceptHandler',from:'ae.c#L10',to:'redis.c#L2551'}]}]},
    {id:'out',from:'clients',to:'net',relations:[{calls:[{label:'acceptHandler calls anetAccept',from:'redis.c#L2560',to:'anet.c#L300'}]}]},
    {id:'other',from:'clients',to:'lists',relations:[{calls:[{label:'createClient calls listCreate',from:'redis.c#L90',to:'adlist.c#L40'}]}]},
  ];
  const member={part:'clients',names:['acceptHandler'],sources:['redis.c#L2551']};
  const hovered=emphasis(empty,'clients',id=>[id],part,member);
  assert.deepEqual([...hovered.activeEdges],['in','out']);
  assert.ok(!hovered.participants.has('lists'));
  const chosen=emphasis({...empty,scope:'clients',selected:new Set(['clients'])},'',id=>[id],part,member);
  assert.deepEqual([...chosen.activeEdges],['in','out']);
  assert.deepEqual([...emphasis(empty,'clients',id=>[id],part).activeEdges],['in','out','other'],'the part itself keeps all its arrows');
  assert.deepEqual([...emphasis(empty,'events',id=>[id],part,member).activeEdges],['in'],'a declaration of another part changes nothing');
});

// An arrow end looked at outlines in place the parts behind it: they are the
// subject, the end's own arrows to them are dark, the parts at the arrows'
// other end stay, and the rest recedes. Its one number pointed at narrows
// it to that number's parts.
test('an arrow end looked at makes the parts behind it the subject',()=>{
  const edges=[{id:'e1',from:'runtime',to:'dict'},{id:'e2',from:'runtime',to:'sds'},{id:'e3',from:'dict',to:'sds'},{id:'e4',from:'cmds',to:'dict'}];
  const label={insides:['dict','sds'],edges:['e1','e2'],byNumber:new Map([[2,{ids:['dict']}],[5,{ids:['sds']}]])};
  const whole=endEmphasis(label,undefined,edges);
  assert.deepEqual([...whole.focus],['dict','sds']);
  assert.deepEqual([...whole.activeEdges].sort(),['e1','e2'],'only the end\'s own arrows are dark');
  assert.ok(whole.participants.has('runtime'),'the other end is involved');
  assert.ok(!whole.participants.has('cmds'),'another frame\'s arrow into the same part is not');
  assert.equal(whole.mode,'hover');
  const one=endEmphasis(label,5,edges);
  assert.deepEqual([...one.focus],['sds']);
  assert.deepEqual([...one.activeEdges],['e2']);
});
