// The frozen journeys and the reading lints, on reports rendered by
// `repomap render` (journeys.spec.mjs). A journey is what a reader of that
// repository came for; it passes or fails as a checklist item and never
// fails the run. A lint is a test: every level reachable by a click (the
// whole map, each program, area, part, Inputs and Outside frame, the
// destinations) and the cards of a few arrows there.

// The journeys, by the repository a report reads (its toolbar's name).
export const journeys=[
  {repo:/^litestream/i,input:'replicate',says:'the column shows the handler ReplicateCommand.Run, not folded',
    check:page=>page.evaluate(()=>window.__journey.visible(/\bReplicateCommand\.Run\b/)?'visible':'')},
  {repo:/^freqtrade/i,input:'trade',says:'FreqtradeBot.process is visible within one expand',
    check:async page=>{
      if(await page.evaluate(()=>window.__journey.visible(/\bFreqtradeBot\.process\b/)))return 'visible';
      const opened=await page.evaluate(()=>window.__journey.expandTo(/\bFreqtradeBot\.process\b/));
      if(!opened)return '';
      await page.waitForTimeout(400);
      return await page.evaluate(()=>window.__journey.visible(/\bFreqtradeBot\.process\b/))?'after one expand':'';
    }},
  {repo:/^redis/i,input:'set',says:'redisDb or dict is named among its State changes',
    check:page=>page.evaluate(()=>window.__journey.inSection('State changes',/\b(redisDb|dict)\b/)?'named':'')},
  {repo:/^othello/i,input:'key-pressed',says:'its keys are listed (n, u, h, 1, 2)',
    check:page=>page.evaluate(()=>{const found=window.__journey.listed(['n','u','h','1','2']);return found.length===5?'all five':found.length?`only ${found.join(', ')}`:'';})},
];

// Page side: what a journey looks for in the reading column.
export function journeyHelpers(){
  const column=()=>document.querySelector('.map-inspector');
  const shown=el=>el.checkVisibility?.({checkOpacity:true,checkVisibilityCSS:true})!==false&&el.getClientRects().length>0;
  const leaves=()=>[...column().querySelectorAll('*')].filter(el=>![...el.children].some(child=>child.textContent.trim()));
  // The column's text as a reader sees it: what is rendered (no closed
  // fold's content), a dotted name whole across the pieces its line breaks
  // split it into (rmDotBreaks).
  const read=()=>column().innerText.replace(/[\u200b\u00ad]/g,'');
  window.__journey={
    visible:pattern=>pattern.test(read()),
    // One click: the closed fold whose content names it.
    expandTo:pattern=>{
      const fold=[...column().querySelectorAll('details:not([open])')].find(details=>pattern.test(details.textContent));
      if(!fold)return false;
      fold.querySelector(':scope>summary')?.click();return true;
    },
    inSection:(title,pattern)=>{
      const head=[...column().querySelectorAll('h4,h5,h6,summary,strong')].find(el=>el.textContent.trim()===title);
      const section=head?.closest('section,details')||head?.parentElement;
      return !!section&&pattern.test(section.textContent);
    },
    listed:keys=>keys.filter(key=>leaves().some(el=>shown(el)&&el.textContent.trim().replace(/^[:\\"'`]+|["'`]+$/g,'')===key)),
  };
}

// Page side: the lints of what is read at one level, over the reading
// column and the arrow cards open, and the canvas in sight.
export function lintLevel(level){
  const out=[],add=(kind,element)=>out.push({kind,element:String(element).slice(0,140),level});
  const shown=el=>el.checkVisibility?.({checkOpacity:true,checkVisibilityCSS:true})!==false&&el.getClientRects().length>0;
  const canvas=document.querySelector('.flow-root')?.getBoundingClientRect();
  const inSight=el=>{const r=el.getBoundingClientRect();return canvas&&r.width>0&&r.right>canvas.left&&r.left<canvas.right&&r.bottom>canvas.top&&r.top<canvas.bottom;};
  const readings=[document.querySelector('.map-inspector'),...document.querySelectorAll('.flow-floating-card')].filter(Boolean);
  const own=el=>[...el.childNodes].filter(node=>node.nodeType===3).map(node=>node.textContent).join('').trim();
  const text=el=>el.textContent.replace(/\s+/g,' ').trim();
  const where=el=>[el,el.parentElement].filter(Boolean).map(at=>`${at.tagName.toLowerCase()}${at.classList.length?'.'+[...at.classList].join('.'):''}`).reverse().join('>');
  // No place written as file:line in a row.
  for(const root of readings)for(const el of root.querySelectorAll('*')){
    const words=own(el);if(!words||!shown(el))continue;
    const at=/\S+\.\w+:\d+/.exec(words);if(at)add('file:line',`"${at[0]}" in "${words}"`);
  }
  // No entry repeated within one list: a list's items, or a card's rows.
  for(const root of readings)for(const list of root.querySelectorAll('ul,ol,.flow-card-rows')){
    if(!shown(list))continue;
    const seen=new Set();
    for(const row of [...list.children].filter(shown)){
      const item=text(row);if(!item)continue;
      if(seen.has(item)){add('repeated',`"${item}" twice in one list (${where(list)})`);break;}
      seen.add(item);
    }
  }
  // No word cut in the middle: an ellipsis before a letter of the whole
  // name its hover gives, or a line the browser clips.
  const cut=el=>{
    const style=getComputedStyle(el);
    if(style.textOverflow==='ellipsis'&&el.scrollWidth>el.clientWidth+1)return 'clipped at its end by the browser';
    if(style.webkitLineClamp&&style.webkitLineClamp!=='none'&&el.scrollHeight>el.clientHeight+1)return 'clamped by the browser';
    const words=text(el);
    if(words.endsWith('…')){
      const whole=(el.closest('[title]')?.getAttribute('title')||'').replace(/\s+/g,' ').trim(),before=words.slice(0,-1).trim();
      const at=whole.indexOf(before.split(' ').slice(-3).join(' '));
      if(whole&&at>=0){const next=whole[at+before.split(' ').slice(-3).join(' ').length];if(next&&/[\p{L}\p{N}]/u.test(next))return 'cut inside a word';}
    }
    return '';
  };
  const texts=[...readings.flatMap(root=>[...root.querySelectorAll('*')]),...(canvas?document.querySelectorAll('.flow-root *'):[])]
    .filter(el=>own(el)&&shown(el)&&(!el.closest('.flow-root')||inSight(el)));
  for(const el of texts){const why=cut(el);if(why)add('cut',`"${text(el)}" ${why} (${where(el)})`);}
  // No empty box in sight: a card with no words, or a frame with neither
  // its title nor anything in it.
  if(canvas){
    const nodes=[...document.querySelectorAll('.react-flow__node')].filter(el=>shown(el)&&inSight(el));
    const boxes=nodes.map(el=>({el,id:el.dataset.id,rect:el.getBoundingClientRect(),frame:el.classList.contains('react-flow__node-area')}));
    for(const box of boxes){
      const {width,height}=box.rect;if(width<40||height<30)continue;
      if(!box.frame){
        // Its words fill less than the top 45% of a card two lines tall or
        // more: litestream's "Not analysed" had stood as its title over an
        // empty box.
        const words=[...box.el.querySelectorAll('*')].filter(el=>own(el)&&shown(el)).flatMap(el=>[...el.getClientRects()]);
        const bottom=Math.max(box.rect.top,...words.map(rect=>rect.bottom));
        if(!text(box.el))add('empty box',box.id);
        else if(height>=40&&bottom-box.rect.top<height*.45)add('empty box',`${box.id} "${text(box.el).slice(0,40)}": its words fill ${Math.round(100*(bottom-box.rect.top)/height)}% of it`);
        continue;
      }
      const named=document.querySelector(`[data-frame-title="${CSS.escape(box.id)}"],[data-summary-area="${CSS.escape(box.id)}"],[data-component-overview="${CSS.escape(box.id)}"]`);
      const holds=boxes.some(other=>other!==box&&other.rect.left>=box.rect.left-1&&other.rect.right<=box.rect.right+1&&other.rect.top>=box.rect.top-1&&other.rect.bottom<=box.rect.bottom+1);
      if(!(named&&text(named))&&!holds)add('empty frame',box.id);
    }
  }
  return out;
}

// Page side, once per report: an Outside frame naming its own program.
export function lintOutside(){
  const out=[],byID=new Map([...document.querySelectorAll('[data-map-explorer] [data-node]')].map(n=>[n.id,n]));
  const name=text=>String(text||'').toLowerCase().replace(/\.[a-z]+$/,'').replace(/[^\p{L}\p{N}]+/gu,' ').trim();
  for(const frame of byID.values()){
    if(frame.dataset.branch!=='outside')continue;
    const program=byID.get(frame.id.replace(/^system-outside-/,'system-component-'));if(!program)continue;
    const own=new Set([name(program.dataset.title),name(program.dataset.title.split('/').filter(Boolean).at(-1))]);
    for(const id of (frame.dataset.children||'').split(/\s+/)){
      const chip=byID.get(id);
      if(chip&&own.has(name(chip.dataset.title)))out.push({kind:'own program',element:`"${chip.dataset.title}" in ${program.dataset.title}'s Outside`,level:'whole map'});
    }
  }
  return out;
}

// Every level a click reaches, as the explorer reads it: its frames and
// parts by their map ids, in the order a reader goes down.
export function levels(){
  const nodes=[...document.querySelectorAll('[data-map-explorer] [data-node]')];
  const of=test=>nodes.filter(test).map(n=>({id:n.id,title:n.dataset.title,branch:n.dataset.branch||(n.dataset.activation?'input':n.id.startsWith('n-')?'part':n.dataset.itemKind||'')}));
  return [...of(n=>n.dataset.branch==='component'),...of(n=>n.dataset.branch==='inputs'),...of(n=>n.dataset.branch==='outside'),
    ...of(n=>n.dataset.branch==='communication'),...of(n=>n.dataset.branch==='area'),...of(n=>!n.dataset.branch&&!n.dataset.activation&&n.id.startsWith('n-'))];
}
