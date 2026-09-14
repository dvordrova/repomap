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
const {OverviewMembers,InputCards}=module.exports;

test('both zoom levels render every named input under one heading per saved type and owner',()=>{
  const kinds=['request','command','interaction','scheduled','continuous','interaction'];
  const records=[{id:'part',title:'Handler',kind:'Core'},...kinds.map((activation,i)=>({id:`input-${i}`,title:i>1?'Same label':`${activation} entry`,activation}))];
  const members=prepareCards(records,Object.fromEntries(kinds.map((_,i)=>[`input-${i}`,'part'])),text=>text.length*7,text=>text);
  const grouped=groupInputs(members[0].inputs);
  assert.equal(grouped.length,5);
  assert.equal(grouped.find(g=>g.kind==='interaction').inputs.length,2);
  for(const element of [React.createElement(OverviewMembers,{members,operation:'input-2'}),React.createElement(InputCards,{inputs:members[0].inputs,operation:'input-2'})]){
    const html=renderToStaticMarkup(element);
    for(let i=0;i<kinds.length;i++)assert.equal(html.split(`data-input-id="input-${i}"`).length-1,1,'rendered input identity must not turn into a count');
    assert.equal(html.split('data-input-kind="interaction"').length-1,1,'one type heading, not one per input');
    assert.match(html,/User interactions/);
    assert.match(html,/Same label/);
    assert.match(html,/aria-pressed="true"/);
  }
});
