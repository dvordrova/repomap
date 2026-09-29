import {test,expect} from '@playwright/test';
import {manyExternalInventory} from './two-systems-five-externals.mjs';

const roots=manyExternalInventory().records.filter(n=>['component','inputs'].includes(n.branch)).map(n=>n.id);

// In a 1280×720 window Redis's whole map could not give its summaries their
// reserved room: "TCP endpoint" was cut below its frame and "Background" out
// of its input list. A summary short of its room is scaled down whole.
for(const [label,viewport] of [['a short window',{width:1000,height:620}],['an ordinary window',{width:1440,height:900}]]){
  test(`every whole-map summary reads whole inside its own frame in ${label}`,async({page})=>{
    await page.setViewportSize(viewport);
    const errors=[];page.on('pageerror',error=>errors.push(error.message));
    await page.goto('/?many-external');
    await expect(page.locator('[data-map]')).toHaveAttribute('data-fixture-ready','true');
    // A frame too narrow for its text at full size keeps a smaller summary,
    // not a blank tile that reads like its display group's plain tiles.
    await expect(page.locator('[data-component-overview]')).toHaveCount(roots.length);
    for(const id of roots)await expect(page.locator(`[data-component-overview="${id}"] .flow-component-overview-heading>strong`),`${id} names itself`).not.toBeEmpty();
    const cut=await page.locator('[data-component-overview]').evaluateAll(cards=>cards.flatMap(card=>{
      const id=card.dataset.componentOverview,frame=document.querySelector(`.react-flow__node[data-id="${id}"]`).getBoundingClientRect();
      const range=document.createRange(),found=[];
      for(const text of card.querySelectorAll('strong,li')){
        // A scrolling list may hide its later entries below; nothing may be
        // cut across, and no heading below its frame.
        const scrolling=text.closest('.flow-component-areas,.flow-input-types');
        const walker=document.createTreeWalker(text,NodeFilter.SHOW_TEXT);let node;
        while((node=walker.nextNode()))for(const word of node.textContent.matchAll(/\S+/g)){
          range.setStart(node,word.index);range.setEnd(node,word.index+word[0].length);
          const rects=[...range.getClientRects()].filter(r=>r.width>0);
          if(rects.length>1)found.push(`${id}: "${word[0]}" is split`);
          const clip=(scrolling||card).getBoundingClientRect();
          for(const r of rects){
            if(r.left<Math.max(frame.left,clip.left)-.5||r.right>Math.min(frame.right,clip.right)+.5)found.push(`${id}: "${word[0]}" is cut at the side`);
            if(!scrolling&&(r.top<frame.top-.5||r.bottom>Math.min(frame.bottom,clip.bottom)+.5))found.push(`${id}: "${word[0]}" is cut below`);
          }
        }
      }
      return found;
    }));
    expect(cut,'every summary word is whole and inside its frame').toEqual([]);
    expect(errors).toEqual([]);
  });
}
