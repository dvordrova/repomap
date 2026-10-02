// The frozen journeys and the reading lints, on reports rendered by
// `repomap render` (journeys.spec.mjs). A journey is what a reader of that
// repository came for; it passes or fails as a checklist item and never
// fails the run. A lint is a test: every level reachable by a click (the
// whole map, each program, area, part, Inputs and Outside frame, the
// destinations) and the cards of a few arrows there.

// The journeys, by the repository a report reads (its toolbar's name): the
// answer a newcomer came for, read in the rendered column after the clicks
// a journey allows (choosing the input, and opening its State changes). A
// name merely appearing in a list is no answer (critic, 2026-09-30).
// `check` returns [passed, what it found].
const helperInternals=/^(list|listNode|listIter|dict|dictEntry|dictht|dictIterator|sds|sdshdr|robj|redisObject|zskiplist|zskiplistNode|intset|sharedObjectsStruct|aeEventLoop)$/;
export const journeys=[
  {repo:/^othello/i,input:'key-pressed',says:'each key with the command it maps to (n new game, u undo, h, 1, 2)',
    check:page=>page.evaluate(()=>{
      const want={n:/new|restart/i,u:/undo/i,h:/\p{L}{3}/u,1:/\p{L}{3}/u,2:/\p{L}{3}/u};
      const lines=window.__journey.lines(),pairs=[],bare=[];
      for(const [key,command] of Object.entries(want)){
        // A row starting with the key ("n → new-game", ":n new-game"),
        // the command after it.
        const token=new RegExp(`^[\\s"':\\\\]*${key}["']?(?=$|[\\s:,→=-])`);
        const paired=lines.find(line=>token.test(line)&&command.test(line.replace(token,'')));
        if(paired)pairs.push(key);else bare.push(key);
      }
      return [bare.length===0,bare.length?`no command beside ${bare.join(', ')}`:'every key with its command'];
    })},
  // The replicate input is one case of Main.Run (data 2's design): its
  // handler is named with its type and case, never a bare "Run", and its
  // path reaches ReplicateCommand.Run.
  {repo:/^litestream/i,input:'replicate',says:'handled by Main.Run (case "replicate") reaching ReplicateCommand.Run, and among its State changes the replica, WAL or snapshots',
    check:page=>page.evaluate(()=>{
      const handler=window.__journey.handler(),named=/\w\.\w/.test(handler)&&!/^Run\b/.test(handler)&&window.__journey.visible(/\bReplicateCommand\.Run\b/);
      // A change of the replica, the WAL or snapshots, named by what changes
      // or by how (SQLite changed through checkpointV3, setPersistWAL), not
      // a command's or a config's fields.
      const about=/replica|wal|snapshot|ltx|checkpoint/i;
      const effects=window.__journey.effects().filter(effect=>!/command|config|settings|options/i.test(effect.owner)&&effect.rows.length&&(about.test(effect.owner)||effect.rows.some(row=>about.test(row))));
      return [named&&effects.length>0,`handled by "${handler||'?'}"; ${effects.length?effects.map(effect=>effect.owner).slice(0,4).join(', '):'no replica/WAL/snapshot change'}`];
    })},
  {repo:/^freqtrade/i,input:'trade',says:'FreqtradeBot.process reached, and among its State changes the database (Trade) or exchange orders',
    check:page=>page.evaluate(()=>{
      const reached=window.__journey.visible(/\bFreqtradeBot\.process\b/);
      const effects=window.__journey.effects().filter(effect=>/^(Trade|Order|Database|LocalTrade)$/.test(effect.owner)&&effect.rows.length);
      return [reached&&effects.length>0,`${reached?'process reached':'process not shown'}; ${effects.length?effects.map(effect=>effect.owner).join(', '):'no Trade, Order or Database change'}`];
    })},
  {repo:/^redis/i,input:'set',says:'redisDb.dict among its State changes, and no helper internals (listNode, dict.used)',
    check:page=>page.evaluate(helpers=>{
      const effects=window.__journey.effects(),internals=new RegExp(helpers);
      const dict=effects.some(effect=>effect.owner==='redisDb'&&effect.rows.some(row=>/^dict\b/.test(row)));
      const inner=effects.filter(effect=>internals.test(effect.owner)).map(effect=>`${effect.owner}.${effect.rows[0]?.split(' ')[0]||''}`);
      return [dict&&!inner.length,`${dict?'redisDb.dict':'no redisDb.dict'}${inner.length?`; helper internals: ${inner.join(', ')}`:''}`];
    },helperInternals.source)},
];

// Page side: what a journey reads in the column.
export function journeyHelpers(){
  const column=()=>document.querySelector('.map-inspector');
  // The column's text as a reader sees it: what is rendered (no closed
  // fold's content), a dotted name whole across the pieces its line breaks
  // split it into (rmDotBreaks).
  const read=()=>column().innerText.replace(/[\u200b\u00ad]/g,'');
  const text=el=>el.innerText.replace(/[\u200b\u00ad]/g,'').replace(/\s+/g,' ').trim();
  window.__journey={
    visible:pattern=>pattern.test(read()),
    lines:()=>read().split('\n').map(line=>line.trim()).filter(Boolean),
    // What the input's reading says handles it: "handled by X".
    handler:()=>{const lines=[...column().querySelectorAll('.map-card-handler,p,div')].map(text).filter(said=>/^handled by /.test(said)).sort((a,b)=>a.length-b.length);return lines.length?lines[0].replace(/^handled by /,''):'';},
    // Its State changes, the fold opened (the one click a journey allows):
    // each owner (a type, the database, the files) with its rows.
    effects:()=>{
      const fold=[...column().querySelectorAll('details')].find(details=>text(details.querySelector(':scope>summary')||details)==='State changes');
      if(!fold)return [];
      fold.open=true;
      const out=[];
      for(const el of fold.children){
        if(el.tagName==='H6')out.push({owner:text(el),rows:[]});
        else if(out.length&&(el.tagName==='UL'||el.tagName==='OL'))out.at(-1).rows.push(...[...el.children].map(text));
      }
      return out;
    },
  };
}

// A place written as file:line: a path-like token with a file extension (a
// letter first) and a line, never an address and port ("8.8.8.8:80",
// "localhost:2379", "example.com:443"). Pure, so the page and a node test
// share it (window.__fileLine).
export function fileLine(text){
  const hosts=/^(localhost|com|org|net|io|dev|app|ai|co|cloud|edu|gov|info|biz|me|tv|xyz|local|internal|us|uk|de|ru|cn|jp|fr|eu)$/i;
  for(const m of String(text).matchAll(/(?:^|[\s("'`])((?:[\w@~+.-]+[\/\\])*[\w@~+-]+(?:\.[\w-]+)*\.([A-Za-z][A-Za-z0-9]{0,9})):(\d+)(?::\d+)?(?=$|[\s)"'`,;.])/g)){
    const [,token,extension]=m;
    // An address: a dotted host with no path, its last label a top-level
    // domain or a host name, or numbers only.
    if(!/[\/\\]/.test(token)&&(hosts.test(extension)||/^\d+(\.\d+){3}$/.test(token)))continue;
    return `${token}:${m[3]}`;
  }
  return '';
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
    const at=window.__fileLine?window.__fileLine(words):(/\S+\.\w+:\d+/.exec(words)||[''])[0];if(at)add('file:line',`"${at}" in "${words}"`);
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
  for(const el of texts){const why=cut(el);if(why)add('cut',`"${text(el)}" ${why} (${where(el)}${el.closest('.react-flow__node')?` in ${el.closest('.react-flow__node').dataset.id}`:''})`);}
  // No empty box in sight: a card with no words, or a frame with neither
  // its title nor anything in it.
  if(canvas){
    const nodes=[...document.querySelectorAll('.react-flow__node')].filter(el=>shown(el)&&inSight(el));
    const boxes=nodes.map(el=>({el,id:el.dataset.id,rect:el.getBoundingClientRect()}));
    // A frame holds other drawn boxes (an entered program, area or Inputs
    // frame, a kind's group): its content is them, not its words.
    const within=(a,b)=>a.left>=b.left-1&&a.right<=b.right+1&&a.top>=b.top-1&&a.bottom<=b.bottom+1&&a.width*a.height<b.width*b.height;
    for(const box of boxes)box.frame=box.el.classList.contains('react-flow__node-area')||boxes.some(other=>other!==box&&within(other.rect,box.rect));
    for(const box of boxes){
      const {width,height}=box.rect;if(width<40||height<30)continue;
      if(!box.frame){
        // Its words fill less than the top 45% of a card two lines tall or
        // more: litestream's "Not analysed" had stood as its title over an
        // empty box. Marks and outlines drawn in it count with its words
        // (an Inputs card's kind marks, a closed area's parts outlined).
        const words=[...box.el.querySelectorAll('*')].filter(el=>(own(el)||el.matches('svg,img,[data-kind-mark],.scene-ghosts>*,.scene-program-ghosts>*,.scene-bucket-marks>*'))&&shown(el)).flatMap(el=>[...el.getClientRects()]);
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

// Page side: "Parts on this path" names parts only, each once: no raw
// calls, no "init" four times (critic, 2026-09-30).
export function lintPath(level){
  const out=[];
  const parts=new Set([...document.querySelectorAll('[data-map-explorer] [data-node]')].filter(n=>n.id.startsWith('n-')&&!n.dataset.activation&&!n.dataset.branch).map(n=>n.dataset.title));
  for(const summary of document.querySelectorAll('.map-inspector details>summary')){
    const said=/^Parts on this path:\s*(.*)$/.exec(summary.innerText.replace(/[​­]/g,'').replace(/\s+/g,' ').trim());
    if(!said)continue;
    const count=new Map();
    for(const name of said[1].split(/,\s*/).map(name=>name.trim()).filter(Boolean))count.set(name,(count.get(name)||0)+1);
    const raw=[...count.keys()].filter(name=>!parts.has(name)),twice=[...count].filter(([,n])=>n>1).map(([name,n])=>`${name} ×${n}`);
    if(raw.length)out.push({kind:'path parts',element:`not parts: ${raw.slice(0,6).join(', ')}${raw.length>6?` and ${raw.length-6} more`:''}`,level});
    if(twice.length)out.push({kind:'path parts',element:`more than once: ${twice.slice(0,6).join(', ')}`,level});
  }
  return out;
}

// A folded list longer than this is a wall behind its fold: the 95th
// percentile of the 2,592 distinct lists folded away in the four reports
// of 2026-09-30 10:17 (redis, litestream, freqtrade, othello; rows of li,
// div, p or button runs at every level and a sample of inputs: p50 6, p90
// 37, p95 67, p99 136, the longest 336 call rows under one heading).
export const longestFold=67;

// Page side: the lists folded away as the column is rendered (inside a
// closed fold, not inside a fold of their own within it): each run of
// sibling rows of one kind, and its fold's name.
export function collapsedLists(){
  const out=[];
  for(const details of document.querySelectorAll('.map-inspector details:not([open])')){
    const name=(details.querySelector(':scope>summary')?.textContent||'').replace(/\s+/g,' ').trim().slice(0,50);
    for(const container of [details,...details.querySelectorAll('*')]){
      if(container!==details&&container.closest('details')!==details)continue;
      const runs=new Map();
      for(const row of container.children){if(row.tagName==='SUMMARY')continue;const key=`${row.tagName}.${row.className}`;runs.set(key,(runs.get(key)||0)+1);}
      for(const [kind,rows] of runs)if(rows>=2&&/^(LI|DIV|P|BUTTON)\./.test(kind))out.push({fold:name,kind,rows});
    }
  }
  return out;
}

// Page side: a few inputs to read, spread over the report, the journeys'
// among them.
export function inputSample(named,count=6){
  const inputs=[...document.querySelectorAll('[data-map-explorer] [data-node]')].filter(n=>n.dataset.activation);
  const chosen=inputs.filter(n=>named.includes(n.dataset.title));
  for(let i=0;i<count&&inputs.length;i++){const n=inputs[Math.floor(i*inputs.length/count)];if(!chosen.includes(n))chosen.push(n);}
  return chosen.map(n=>({id:n.id,title:n.dataset.title,branch:'input'}));
}
