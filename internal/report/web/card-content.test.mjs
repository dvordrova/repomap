import {test} from 'node:test';
import assert from 'node:assert/strict';
import {build} from 'esbuild';
import {createRequire} from 'node:module';
import React from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import {prepareCards,groupInputs} from './cards.mjs';

const bundle=await build({entryPoints:[new URL('./card-content.jsx',import.meta.url).pathname],bundle:true,write:false,format:'cjs',packages:'external',jsx:'transform'});
const module={exports:{}};
new Function('require','module','exports',bundle.outputFiles[0].text)(createRequire(import.meta.url),module,module.exports);
const {InputTypes}=module.exports;

test('the closed input collection lists existing catalogue types without duplicate input buttons',()=>{
  const kinds=['request','command','interaction','scheduled','continuous','interaction'];
  const records=[{id:'part',title:'Handler',kind:'Core'},...kinds.map((activation,i)=>({id:`input-${i}`,title:i>1?'Same label':`${activation} entry`,activation}))];
  const cards=prepareCards(records,Object.fromEntries(kinds.map((_,i)=>[`input-${i}`,'part'])),text=>text.length*7,text=>text);
  const grouped=groupInputs(cards.filter(n=>n.activation));
  assert.equal(grouped.length,5);
  assert.equal(grouped.find(g=>g.kind==='interaction').inputs.length,2);
  const html=renderToStaticMarkup(React.createElement(InputTypes,{groups:grouped}));
  for(const kind of ['request','command','scheduled','continuous','interaction'])assert.equal(html.split(`data-input-group-kind="${kind}"`).length-1,1);
  assert.match(html,/Incoming requests/);assert.match(html,/Background work/);assert.match(html,/User interactions/);
  assert.doesNotMatch(html,/Same label|data-input-id/,'named inputs are the original graph children, not summary duplicates');
  // A kind chosen reads the collection at that kind's section (owner,
  // 2026-09-29).
  const chosen=[];
  const list=InputTypes({groups:grouped,choose:kinds=>chosen.push(kinds)});
  for(const item of list.props.children)item.props.onClick({stopPropagation(){}});
  assert.deepEqual(chosen,[['request'],['command'],['scheduled'],['continuous'],['interaction']]);
});

// A call that leaves its program reads from each program's own code, not
// only the shared anet pair (owner, 2026-09-28); an outside call has its
// program's side alone, the program not named again ("cmd/litestream:" had
// begun every row of cmd/litestream's reading).
test('a call between programs names each side from its own code, and an outside call says it is outgoing',async()=>{
  globalThis.window={rmT:(key,...values)=>values.reduce((s,v,i)=>s.replace(`{${i}}`,v),key)};
  const view=await build({entryPoints:[new URL('./call-card-view.jsx',import.meta.url).pathname],bundle:true,write:false,format:'cjs',packages:'external',jsx:'transform'});
  const loaded={exports:{}};
  new Function('require','module','exports',view.outputFiles[0].text)(createRequire(import.meta.url),loaded,loaded.exports);
  const {CallRows}=loaded.exports;
  const {callCard}=await import('./call-card.mjs');
  const step=(name,part)=>({name,key:'k#'+name,part});
  const card=callCard([
    {from:'cli',to:'server',calls:[{label:'anetTcpGenericConnect connects_to anetAccept',from:'a#158',at:'anet.c:158',
      sides:[{program:'redis-cli',path:[step('cliConnect','cli'),step('anetTcpConnect','net'),step('anetTcpGenericConnect','net')]},{program:'redis-server',path:[step('acceptHandler','srv'),step('anetAccept','snet')]}]}]},
    {from:'net',to:'tcp',calls:[{label:'anetTcpGenericConnect calls socket.h.connect',from:'a#128',to:'a#158',at:'anet.c:158',
      sides:[{program:'redis-server',path:[step('syncWithMaster','repl'),step('anetTcpConnect','net'),step('anetTcpGenericConnect','net')]}]}]},
  ],{nameOf:id=>id});
  const text=renderToStaticMarkup(React.createElement(CallRows,{card})).replace(/<wbr\/?>/g,'').replace(/<[^>]+>/g,' ').replace(/\s+/g,' ');
  assert.match(text,/redis-cli: cliConnect → anetTcpConnect → anetTcpGenericConnect ⇢ redis-server: acceptHandler → anetAccept/);
  assert.match(text,/ syncWithMaster → anetTcpConnect → anetTcpGenericConnect → socket\.h\.connect outgoing/);
  assert.equal(text.match(/redis-server:/g).length,1,'the outside call names no program');
});
