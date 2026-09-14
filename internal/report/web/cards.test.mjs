import {test} from 'node:test';
import assert from 'node:assert/strict';
import {prepareCards} from './cards.mjs';
import {arrange} from './layout.mjs';

test('input card labels keep every character across long literals and translated names',()=>{
  for(const title of ['Handle mouse click on board or button','LongUnbrokenInputName'.repeat(8),'Начать симуляцию по нажатию кнопки']){
    const cards=prepareCards([{id:'part',title:'Handler',kind:'Part'},{id:'input',title,activation:'interaction'}],{input:'part'},text=>Array.from(text).length*8,text=>text);
    const label=cards[0].inputs[0].displayTitle;
    assert.ok(label.split('\n').every(line=>Array.from(line).length*8<=202));
    assert.equal(label.replace(/\s/g,''),title.replace(/\s/g,''));
  }
});

test('every saved item stays visible, bound inputs keep identity and kinds inside their exact owner',async()=>{
  const kinds=['command','request','interaction','scheduled','continuous'];
  const records=[
    {id:'component',title:'Service',branch:'component',category:'component',kind:'Component',summary:'Receives work and sends results.',children:['part','unbound']},
    {id:'part',title:'Handler',category:'part',kind:'Part'},
    ...kinds.map((activation,i)=>({id:`input${i}`,title:i===1?'POST /work/{very_long_literal_parameter_that_must_survive}':`${activation} input`,activation,category:'input',kind:'Input'})),
    {id:'unbound',title:'Unbound request',activation:'request',category:'input',kind:'Input'},
    {id:'out1',title:'Queue',category:'external',kind:'External communication',subtitle:'https://queue.example/work'},
    {id:'out2',title:'Queue',category:'external',kind:'External communication',subtitle:'https://queue.example/work'},
    {id:'failed',title:'Worker',category:'component',kind:'Component',summary:'No compiler'},
  ];
  const owners=Object.fromEntries(kinds.map((_,i)=>[`input${i}`,'part']));
  const cards=prepareCards(records,owners,text=>text.length*8,text=>text);
  const part=cards.find(c=>c.id==='part');
  assert.equal(part.inputs.length,5);
  assert.deepEqual(part.inputs.map(n=>n.activation),kinds);
  assert.equal(part.inputs[1].displayTitle.replace(/\n/g,''),records[3].title.replace(/ /g,''),'literal survives wrapping');
  const visible=cards.flatMap(n=>[n.id,...n.inputs.map(i=>i.id)]);
  assert.deepEqual(visible.sort(),records.map(n=>n.id).sort(),'nothing dropped, merged by name, or added');
  assert.equal(cards.find(n=>n.id==='out1').subtitle.replace(/\n/g,''),'https://queue.example/work');
  assert.equal(cards.find(n=>n.id==='failed').description,'No compiler');
  const layout=await arrange(cards,[],[{id:'component',nodes:['part','unbound']}]);
  const placed=layout.nodes.find(n=>n.id==='part');
  assert.equal(placed.height,part.height,'layout reserves all input cards before selection');
  assert.ok(placed.position.y>=cards.find(c=>c.id==='component').headerHeight,'purpose has reserved space');
});
