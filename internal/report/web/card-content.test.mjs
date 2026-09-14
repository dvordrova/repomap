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
const {OverviewMembers,InputTypes}=module.exports;

test('the closed input collection lists existing catalogue types without duplicate input buttons',()=>{
  const kinds=['request','command','interaction','scheduled','continuous','interaction'];
  const records=[{id:'part',title:'Handler',kind:'Core'},...kinds.map((activation,i)=>({id:`input-${i}`,title:i>1?'Same label':`${activation} entry`,activation}))];
  const cards=prepareCards(records,Object.fromEntries(kinds.map((_,i)=>[`input-${i}`,'part'])),text=>text.length*7,text=>text);
  const grouped=groupInputs(cards.filter(n=>n.activation));
  assert.equal(grouped.length,4);
  assert.equal(grouped.find(g=>g.kind==='interaction').inputs.length,2);
  assert.equal(grouped.find(g=>g.kind==='background').inputs.length,2);
  const html=renderToStaticMarkup(React.createElement(InputTypes,{groups:grouped}));
  for(const kind of ['request','command','background','interaction'])assert.equal(html.split(`data-input-group-kind="${kind}"`).length-1,1);
  assert.match(html,/Incoming requests/);assert.match(html,/Background work/);assert.match(html,/User interactions/);
  assert.doesNotMatch(html,/Same label|data-input-id/,'named inputs are the original graph children, not summary duplicates');
  const implementation=renderToStaticMarkup(React.createElement(OverviewMembers,{members:cards.filter(n=>!n.activation)}));
  assert.doesNotMatch(implementation,/data-input-id|Same label/,'implementation summaries do not embed the outside inputs');
});
