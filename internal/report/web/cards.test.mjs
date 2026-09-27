import {test} from 'node:test';
import assert from 'node:assert/strict';
import {prepareCards,overviewHeading,overviewScale,groupInputs,wrapText} from './cards.mjs';
import {semanticLayout} from './semantic.mjs';

// canvas.css draws a card 260px wide inside a 1.5px border and 16px padding.
const column=260-2*1.5-2*16;

test('input card labels keep every character across long literals and translated names',()=>{
  for(const title of ['Handle mouse click on board or button','LongUnbrokenInputName'.repeat(8),'Начать симуляцию по нажатию кнопки']){
    const cards=prepareCards([{id:'part',title:'Handler',kind:'Part'},{id:'input',title,activation:'interaction'}],{input:'part'},text=>Array.from(text).length*8,text=>text);
    const label=cards.find(card=>card.id==='input').title;
    assert.ok(label.split('\n').every(line=>Array.from(line).length*8<=column));
    assert.equal(label.replace(/\s/g,''),title.replace(/\s/g,''));
  }
});

// Redis's input collection read "redis-server (executable" above a lone ")".
test('a title breaks only between words and never starts a line with closing punctuation',()=>{
  const measure=text=>Array.from(text).length*8;
  assert.deepEqual(wrapText('redis-server (executable)',120,'',measure),['redis-server','(executable)']);
  assert.deepEqual(wrapText('Set commands (SADD, SREM )',150,'',measure),['Set commands','(SADD, SREM )'],'a spaced bracket stays with its word');
  assert.deepEqual(wrapText('internal/report/templates',96,'',measure),['internal/','report/','templates'],'a path breaks after its separators');
  assert.deepEqual(wrapText('getHostByNameAndPort',96,'',measure),['getHostBy','NameAndPort'],'an identifier breaks between its words');
  // Only a word wider than the whole line breaks inside, keeping every
  // character, and never so that a line starts with ")" or ends with "(".
  for(const title of ['(executable)','redis-server (executable)','call(argument)']){
    const lines=wrapText(title,88,'',measure);
    assert.equal(lines.join('').replace(/\s/g,''),title.replace(/\s/g,''),`${title}: every character kept`);
    for(const line of lines){
      assert.ok(measure(line)<=88,`${title}: "${line}" fits`);
      assert.ok(!/^[)\]},.;:]/.test(line)&&!/[(\[{]$/.test(line),`${title}: "${line}" splits a bracket from its word`);
    }
  }
});

// The browser re-broke a part's title measured across the zoom button's room,
// and one measured 194px wide beside a button in a 191px column.
test('a part\'s title lines leave room for its zoom button',()=>{
  const measure=text=>Array.from(String(text)).length*8;
  for(const title of ['Command dispatch table and replies','Sorted set skiplist code']){
    const [part]=prepareCards([{id:'part',title,symbols:[{name:'call'}]}],{},measure,text=>text);
    assert.ok(part.title.split('\n').every(line=>measure(line)<=column-34),part.title);
  }
});

// Pre-broken at 228px, "Implements Redis set commands and" (225.84px) broke
// again in the 225px column and left "and" alone on a line.
test('a description is wrapped by the browser and counted at the card\'s text column',()=>{
  const measure=text=>Array.from(String(text)).length*7.6;
  const summary='Keeps the replica data in step with the master over networks.';
  assert.equal(wrapText(summary,228,'',measure).length,2,'two lines at the old width');
  assert.equal(wrapText(summary,column,'',measure).length,3,'three in the real column');
  const [part]=prepareCards([{id:'part',title:'Replication',category:'part',kind:'Part',summary}],{},measure,text=>text);
  assert.equal(part.description,summary,'no line break is inserted into the text');
  assert.equal(part.height,66+22+3*18,'the card has room for the three lines the browser draws');
  const long=summary+' '+summary;
  const [capped]=prepareCards([{id:'part',title:'Replication',category:'part',kind:'Part',summary:long}],{},measure,text=>text);
  assert.equal(capped.description,long);
  assert.equal(capped.height,66+22+3*18,'a part keeps at most three lines of its description');
});

// A destination frame fitted a pixel narrower than its reservation read "DNS
// resolve" over a clipped "r". Short of room, a heading shrinks and stays whole.
test('an overview heading short of its longest word shrinks rather than breaking it',()=>{
  const measure=(text,font='13px')=>Array.from(String(text)).length*Number(/(\d+)px/.exec(font)[1])*.55;
  for(const [title,branch] of [['DNS resolver','communication'],['Background work (Inputs)','communication'],['redis-benchmark (executable)','component']]){
    const [card]=prepareCards([{id:'frame',title,branch,children:['inside']},{id:'inside',title:'inside'}],{},measure,text=>text);
    for(const width of [card.overviewMinWidth,card.overviewMinWidth-.6,card.overviewMinWidth*.6]){
      const heading=overviewHeading(card,width,measure),font=`700 ${branch==='component'?18:13}px system-ui`;
      assert.deepEqual(heading.lines.join(' ').split(' '),title.split(' '),`${title} at ${width}px keeps every word whole`);
      assert.ok(heading.lines.every(line=>measure(line,font)*heading.scale<=heading.width+1e-9),`${title} at ${width}px fits its room`);
      if(width>=card.overviewMinWidth)assert.equal(heading.scale,1,`${title} keeps its size at its reserve`);
      else if(width<card.overviewMinWidth*.7)assert.ok(heading.scale<1,`${title} shrinks well short of its reserve`);
    }
  }
});

// Redis's first screen showed "DNS resolver" three times side by side.
test('a frame whose display group carries its destination text draws no heading of its own',()=>{
  const measure=text=>String(text).length*7;
  const [tile,,titled]=prepareCards([
    {id:'dns-cli',title:'DNS resolver',branch:'communication',children:['call'],displayGroup:'dns',displayGroupTitle:'DNS resolver'},
    {id:'call',title:'gethostbyname'},
    {id:'dns-alone',title:'DNS resolver',branch:'communication',children:['other']},
  ],{},measure,text=>text);
  assert.deepEqual(overviewHeading(tile,tile.overviewMinWidth,measure).lines,[],'the tile repeats nothing');
  assert.equal(overviewHeading(titled,titled.overviewMinWidth,measure).lines.join(' '),'DNS resolver','a frame alone keeps its heading');
  assert.equal(tile.displayGroupHeadingAt(3*tile.overviewMinWidth).lines.join(' '),'DNS resolver','the group says it once');
  assert.equal(titled.displayGroupHeadingAt,undefined);
  // Open, the tile's calls stand under no title row: the group names them.
  assert.ok(tile.headerHeight<titled.headerHeight,`the open tile reserves ${tile.headerHeight} above its calls, a titled frame ${titled.headerHeight}`);
});

// In a 1280×720 window Redis's map could not give its summaries their reserved
// room: "TCP endpoint" was cut below its frame and "Background" out of the
// input list. A summary short of its room is scaled down whole instead.
test('a summary the whole-map fit cannot give its reserve is scaled down whole, never cut',()=>{
  const measure=(text,font='13px')=>Array.from(String(text)).length*Number(/(\d+)px/.exec(font)?.[1]||13)*.58;
  const cards=prepareCards([
    {id:'tcp',title:'TCP endpoint',branch:'communication',children:['connect']},{id:'connect',title:'connect'},
    {id:'inputs',title:'redis-server (executable)',branch:'inputs',children:['get','cron']},
    {id:'get',title:'GET',activation:'request'},{id:'cron',title:'serverCron',activation:'background'},
    {id:'server',title:'redis-server (executable)',branch:'component',children:['net','commands']},
    {id:'net',title:'Networking',branch:'area',children:['n1']},{id:'commands',title:'Data type commands',branch:'area',children:['c1']},
    {id:'n1',title:'n1'},{id:'c1',title:'c1'},
  ],{get:'n1',cron:'c1'},measure,text=>text);
  for(const card of cards.filter(card=>card.overviewHeightAtWidth)){
    const width=card.overviewMinWidth,height=card.overviewHeightAtWidth(width);
    assert.equal(overviewScale(card,width,height),1,`${card.id} at its reserve keeps its size`);
    for(const [screenWidth,screenHeight] of [[width*.8,height],[width,height*.7],[width*.5,height*.6]]){
      const fit=overviewScale(card,screenWidth,screenHeight);
      assert.ok(fit<1,`${card.id} in ${screenWidth}×${screenHeight} shrinks`);
      assert.ok(screenWidth/fit>=card.overviewMinWidth-1e-6,`${card.id}: every whole word has its width`);
      assert.ok(screenHeight/fit>=card.overviewHeightAtWidth(screenWidth/fit)-1e-6,`${card.id}: the complete summary has its height`);
      assert.ok(fit>.95*Math.min(screenWidth/width,screenHeight/height),`${card.id} shrinks no more than it must`);
    }
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

// "Request" stood on 97 of Redis's 98 input tiles, repeating the collection's
// own "Incoming requests". A tile names its kind only when it differs.
test('an input tile names its kind only when it is not its collection\'s common kind',()=>{
  const records=[{id:'inputs',branch:'inputs',children:['group','thread']},{id:'group',branch:'inputs-part',title:'String commands',children:['get','set']},
    {id:'get',title:'get',activation:'request'},{id:'set',title:'set',activation:'request'},{id:'thread',title:'IOThreadEntryPoint',activation:'continuous'}];
  const cards=new Map(prepareCards(records,{},text=>String(text).length*7,text=>text).map(card=>[card.id,card]));
  assert.equal(cards.get('get').kindLabel,'');assert.equal(cards.get('set').kindLabel,'');
  assert.equal(cards.get('thread').kindLabel,'Background activity');
  assert.ok(cards.get('get').height<cards.get('thread').height,'a tile without its kind row is shorter');
  assert.deepEqual(cards.get('inputs').inputGroups.map(group=>group.kind),['request','background'],'the collection still lists every kind it holds');
});

// Titled with its component's name, Redis's input collection read as a
// second redis-server beside the programs. It is headed by the word the
// glyph key uses for inputs; its component stays in its accessible name.
test('an input collection is headed Inputs, whatever the page titles it',()=>{
  const measure=(text,font='13px')=>Array.from(String(text)).length*Number(/(\d+)px/.exec(font)[1])*.55;
  const [card]=prepareCards([{id:'inputs',title:'redis-server (executable)',branch:'inputs',children:['get']},{id:'get',title:'get',activation:'request'}],{},measure,text=>text==='Inputs'?'Входы':text);
  assert.equal(card.heading,'Входы');
  assert.equal(card.name,'redis-server (executable)');
  assert.deepEqual(overviewHeading(card,card.overviewMinWidth,measure).lines,['Входы']);
});
