import {test} from 'node:test';
import assert from 'node:assert/strict';
import {prepareCards,overviewHeading,groupInputs,wrapText} from './cards.mjs';
import {semanticLayout} from './semantic.mjs';

test('input card labels keep every character across long literals and translated names',()=>{
  for(const title of ['Handle mouse click on board or button','LongUnbrokenInputName'.repeat(8),'Начать симуляцию по нажатию кнопки']){
    const cards=prepareCards([{id:'part',title:'Handler',kind:'Part'},{id:'input',title,activation:'interaction'}],{input:'part'},text=>Array.from(text).length*8,text=>text);
    const label=cards.find(card=>card.id==='input').title;
    assert.ok(label.split('\n').every(line=>Array.from(line).length*8<=228));
    assert.equal(label.replace(/\s/g,''),title.replace(/\s/g,''));
  }
});

test('an external heading fits whole words below its zoom control or beside it',()=>{
  const measure=text=>text.length*7;
  for(const title of ['PostgreSQL','OpenTelemetry collector','Notification gateway']){
    const [card]=prepareCards([{id:'destination',title,branch:'communication',children:['call']}],{},measure,text=>text);
    const heading=overviewHeading(card,card.overviewMinWidth,measure);
    assert.equal(heading.clearZoom,true,`${title}: a narrow frame places the title below the control`);
    assert.ok(title.split(/\s+/).every(word=>measure(word)<=heading.width),'whole heading words fit');
    assert.equal(card.overviewHeightAtWidth(card.overviewMinWidth),16+heading.height,'the extra control row has reserved height');
    assert.equal(overviewHeading(card,card.overviewMinWidth+48,measure).clearZoom,false,'a wider frame keeps the control beside the title');
  }
});

test('a two-line external heading does not force another sizing pass when text and control fit',()=>{
  const [card]=prepareCards([{id:'destination',title:'Redis cache',branch:'communication',children:['call']}],{},text=>text.length*7,text=>text);
  // 64px beside the 49px text column clears the control; the complete heading
  // needs two 17px lines plus two 8px insets, not an unrelated 52px floor.
  assert.equal(card.overviewHeightAtWidth(113),50);
});

test('fractional word measurements do not add an unreserved zoom-control row after fitting',()=>{
  const measure=text=>text.length*7.73;
  const [card]=prepareCards([{id:'backend',title:'Job processing service',branch:'component'}],{},measure,text=>text);
  assert.equal(card.overviewMinWidth,Math.ceil(measure('processing')+64));
  const measured=overviewHeading(card,card.overviewMinWidth,measure);
  const fitted=overviewHeading(card,card.overviewMinWidth-1e-10,measure);
  assert.equal(measured.clearZoom,false);
  assert.equal(fitted.clearZoom,false,'uniform transform round-off cannot move the heading below the control');
  assert.equal(fitted.height,measured.height);
});

test('a short component name reserves the complete words of its actual area inventory',()=>{
  const measure=text=>text.length*7.73;
  const [card]=prepareCards([
    {id:'front',title:'front',branch:'component',children:['area']},
    {id:'area',title:'Playground orchestration',branch:'area'},
  ],{},measure,text=>text);
  assert.equal(card.overviewMinWidth,Math.ceil(measure('orchestration')+32));
  assert.ok(card.overviewMinWidth>Math.ceil(measure('front')+64),'inventory names contribute independently of the short component label');
});

test('short target names leave enough width to scan area names as two-line entries',()=>{
  const measure=text=>text.length*7.73;
  const titles=['Application shell and navigation','Shared domain and utility support','Page and playground views'];
  const [card]=prepareCards([
    {id:'front',title:'front',branch:'component',children:titles.map((_,i)=>`area${i}`)},
    ...titles.map((title,i)=>({id:`area${i}`,title,branch:'area'})),
  ],{},measure,text=>text);
  assert.ok(card.overviewPreferredWidth>card.overviewMinWidth,'a whole-word minimum alone is not readable inventory layout');
  for(const title of titles)assert.ok(wrapText(title,card.overviewPreferredWidth-32,'500 13px system-ui',measure).length<=2);
  const [short]=prepareCards([
    {id:'front',title:'Web application',branch:'component',children:['area']},
    {id:'area',title:'Job editing',branch:'area'},
  ],{},measure,text=>text);
  assert.equal(short.overviewPreferredWidth,short.overviewMinWidth,'short inventories do not enlarge every participant');
});

test('an input collection clears the zoom control before its first input type',()=>{
  const [card]=prepareCards([
    {id:'inputs',title:'front',branch:'inputs',children:['input']},
    {id:'input',title:'Process jobs',activation:'continuous'},
  ],{},text=>text.length*7,text=>text);
  const heading=overviewHeading(card,240,text=>text.length*7);
  assert.equal(heading.height,17);
  assert.equal(card.overviewHeightAtWidth(240),16+32+6+18,'the first type starts after the complete control row');
});

test('bound inputs retain their exact identity outside their implementation',()=>{
  const records=[{id:'part',title:'Handler',kind:'Part'},{id:'input',title:'Submit a job',activation:'interaction',componentName:'Web application'}];
  const cards=prepareCards(records,{input:'part'},text=>text.length*7,text=>text);
  assert.deepEqual(cards.map(n=>n.id),['part','input']);
  assert.equal(cards.find(n=>n.id==='input').activation,'interaction');
  assert.equal(cards.find(n=>n.id==='input').componentName,'Web application');
  assert.equal(cards.find(n=>n.id==='part').inputs,undefined,'implementation does not embed a duplicate input');
});

test('the input catalogue preserves original kinds and combines only scheduled and continuous display types',()=>{
  const inputs=['request','command','scheduled','continuous','interaction','unclassified'].map((activation,i)=>({id:'input'+i,activation}));
  const groups=groupInputs(inputs,text=>'translated '+text);
  assert.deepEqual(groups.map(g=>g.kind),['request','command','background','interaction','other']);
  assert.deepEqual(groups[2].inputs.map(n=>n.activation),['scheduled','continuous']);
  assert.deepEqual(groups.flatMap(g=>g.inputs.map(n=>n.id)),inputs.map(n=>n.id));
  assert.equal(groups[4].title,'translated Other operations');
  assert.deepEqual(groupInputs([{id:'one',activation:'request'}]).map(g=>g.kind),['request'],'absent types are not invented');
});

test('one root input collection retains every original input and implementation edge',async()=>{
  const kinds=['command','request','interaction','scheduled','continuous'];
  const inputIDs=kinds.map((_,i)=>`input${i}`);
  const records=[
    {id:'component',title:'Service',branch:'component',category:'component',kind:'Component',summary:'Receives work and sends results.',children:['part']},
    {id:'part',title:'Handler',category:'part',kind:'Part'},
    {id:'inputs',title:'Service',branch:'inputs',kind:'Inputs',children:inputIDs},
    ...kinds.map((activation,i)=>({id:inputIDs[i],title:i===1?'POST /work/{very_long_literal_parameter_that_must_survive}':`${activation} input`,activation,category:'input',kind:'Input'})),
    {id:'unbound',title:'Unbound request',activation:'request',category:'input',kind:'Input'},
    {id:'out1',title:'Queue',category:'external',kind:'External communication',subtitle:'https://queue.example/work'},
    {id:'out2',title:'Queue',category:'external',kind:'External communication',subtitle:'https://queue.example/work'},
    {id:'failed',title:'Worker',category:'component',kind:'Component',summary:'No compiler'},
  ];
  const owners=Object.fromEntries(inputIDs.map(id=>[id,'part']));
  const cards=prepareCards(records,owners,text=>text.length*8,text=>text);
  assert.equal(cards.find(c=>c.id==='part').inputs,undefined,'the implementation has no embedded duplicates');
  assert.deepEqual(cards.filter(c=>owners[c.id]).map(n=>n.activation),kinds);
  assert.equal(cards.find(c=>c.id==='input1').title.replace(/\s/g,''),records.find(c=>c.id==='input1').title.replace(/\s/g,''),'complete literal survives wrapping');
  assert.deepEqual(cards.map(n=>n.id),records.map(n=>n.id),'no item is dropped, merged by name, or added');
  const collection=cards.find(c=>c.id==='inputs');
  assert.deepEqual(collection.inputGroups.map(g=>g.kind),['request','command','background','interaction']);
  assert.ok(collection.overviewHeightAtWidth(collection.overviewMinWidth)>80,'the whole type list has reserved height');
  assert.equal(cards.find(n=>n.id==='out1').subtitle.replace(/\n/g,''),'https://queue.example/work');
  assert.equal(cards.find(n=>n.id==='failed').description,'No compiler');
  const relations=inputIDs.map(id=>({from:id,to:'part',label:'implemented in',fromSource:id+':12'}));
  const {layout}=await semanticLayout(cards,relations,[{id:'component',nodes:['part']},{id:'inputs',nodes:inputIDs}],1200,700);
  const placed=new Map(layout.nodes.map(n=>[n.id,n]));
  assert.equal(placed.get('inputs').parentId,undefined,'the input collection is outside the target');
  for(const id of inputIDs)assert.equal(placed.get(id).parentId,'inputs');
  assert.equal(placed.get('part').parentId,'component');
  assert.equal(placed.get('unbound').parentId,undefined,'an unowned input remains visible');
  assert.deepEqual(layout.edges.flatMap(e=>e.relations),relations,'original endpoints and evidence are preserved');
});
