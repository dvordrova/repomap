import {test,expect} from '@playwright/test';
import {invariantKit} from './invariants.mjs';
import {writeFile} from 'node:fs/promises';

test.skip(!process.env.REPOMAP_REAL_RUN?.includes('sqlite'),'REPOMAP_REAL_RUN names the completed ordinary SQLite run');

test('keyboard entry cannot scroll screen markers away from the world camera',async({page})=>{
  test.setTimeout(180_000);
  await page.goto('/real-report.html');
  const map=page.locator('[data-map]');
  await expect(map).toHaveClass(/flow-enabled/,{timeout:90_000});
  await expect(map).not.toHaveClass(/flow-initializing/,{timeout:90_000});
  await page.locator('[data-zoom-into="system-component-t1"]').press('Enter');
  await expect(map).toHaveAttribute('data-scene-level','system-component-t1');
  const overlay=page.locator('.scene-overlay');
  const outside=await overlay.locator('button').evaluateAll(buttons=>buttons.find(button=>{
    const r=button.getBoundingClientRect(),p=button.parentElement.getBoundingClientRect();
    return r.left<p.left||r.top<p.top||r.right>p.right||r.bottom>p.bottom;
  })?.getAttribute('data-zoom-into'));
  test.skip(!outside,'this component fits every magnifier in the canvas');
  const before=await overlay.evaluate(e=>[e.scrollLeft,e.scrollTop]);
  const button=page.locator(`[data-zoom-into="${outside}"]`);
  await expect(button).toHaveAttribute('tabindex','-1');
  // Focus alone used to scroll overflow:hidden without moving the camera.
  // A harmless key reaches that focus before the entry action changes it.
  await button.press('Shift');
  expect(await overlay.evaluate(e=>[e.scrollLeft,e.scrollTop])).toEqual(before);
  await button.press('Enter');
  await expect(map).toHaveAttribute('data-scene-level',`system-component-t1/${outside}`);
  expect(await overlay.evaluate(e=>[e.scrollLeft,e.scrollTop])).toEqual([0,0]);
});

// Use the ordinary saved renderer, rather than a second map or model request.
test.skip(!process.env.REPOMAP_REAL_RUN,'set REPOMAP_REAL_RUN to a saved run directory');
test('closed components expose complete named navigation, including their last child',async({page},testInfo)=>{
  test.setTimeout(180_000);
  await page.goto('/real-report.html');
  const map=page.locator('[data-map]');
  await expect(map).toHaveClass(/flow-enabled/,{timeout:90_000});
  await expect(map).not.toHaveClass(/flow-initializing/,{timeout:90_000});
  const ids=await page.locator('.scene-program-card').evaluateAll(cards=>cards.map(card=>card.dataset.componentOverview));
  expect(ids.length).toBeGreaterThan(0);
  for(const id of ids){
    await page.getByRole('link',{name:'Home',exact:true}).click();
    const card=page.locator(`.scene-program-card[data-component-overview="${id}"]`);
    await expect(card).toBeVisible();
    await expect(card.locator('.scene-ghosts')).toHaveCount(0);
    const original=await page.locator(`[data-node][id="${id}"]`).getAttribute('data-children');
    const saved=(original||'').split(/\s+/).filter(Boolean);
    const children=card.locator('[data-program-child]');
    expect(await children.evaluateAll(rows=>rows.map(row=>row.dataset.programChild))).toEqual(saved);
    if(!saved.length)continue;
    const last=children.last(),child=saved.at(-1),childName=(await last.textContent()).replace(/\s+/g,' ').trim();
    // Keyboard focus scrolls the last complete row into view. Enter must
    // open that child, rather than the component hit area behind its button.
    await last.press('Enter');
    await expect(page).toHaveURL(new RegExp(`#${child}$`));
    // Area titles are in the panel heading; part titles are in its prepared
    // reading. Verify the selected panel, which owns both ordinary forms.
    await expect(page.getByRole('complementary',{name:'Selected node details',exact:true})).toContainText(childName);
    await page.getByRole('link',{name:'Home',exact:true}).click();
    await expect(card).toBeVisible();
    const first=card.locator('[data-program-child]').first(),firstID=await first.getAttribute('data-program-child');
    await first.click();
    await expect(page).toHaveURL(new RegExp(`#${firstID}$`));
  }
  await page.getByRole('link',{name:'Home',exact:true}).click();
  await page.screenshot({path:testInfo.outputPath('named-component-overview.png')});
});

test('real SQLite closed areas name every original direct child below their heading',async({page})=>{
  test.setTimeout(150_000);
  await page.goto('/real-report.html');
  const map=page.locator('[data-map]');
  await expect(map).toHaveClass(/flow-enabled/,{timeout:90_000});
  await expect(map).not.toHaveClass(/flow-initializing/,{timeout:90_000});
  for(const program of ['system-component-t1','system-component-t2','system-component-t3']){
    await page.getByRole('link',{name:'Home',exact:true}).click();
    await page.locator(`[data-zoom-into="${program}"]`).press('Enter');
    await expect(map).toHaveAttribute('data-scene-level',program);
    const areas=page.locator('[data-summary-area]');
    expect(await areas.count()).toBeGreaterThan(0);
    for(const area of await areas.all()){
      const id=await area.getAttribute('data-summary-area');
      const saved=(await page.locator(`[data-node][id="${id}"]`).getAttribute('data-children')).split(/\s+/).filter(Boolean);
      expect(await area.locator('[data-program-child]').evaluateAll(rows=>rows.map(row=>row.dataset.programChild))).toEqual(saved);
      expect(await area.locator('[data-program-child]').allTextContents()).toHaveLength(saved.length);
      const heading=await area.locator('[data-box-title]').boundingBox(),list=await area.locator('.scene-program-inside').boundingBox();
      const description=await area.locator('.flow-description').count()?await area.locator('.flow-description').boundingBox():heading;
      expect(list.y).toBeGreaterThanOrEqual(description.y+description.height-1);
      const rect=await area.boundingBox();expect(list.y+list.height).toBeLessThanOrEqual(rect.y+rect.height+1);
    }
    await expect(page.locator('.scene-ghosts')).toHaveCount(0);
  }
  // Traverse all four saved architectural levels, then an original leaf.
  await page.getByRole('link',{name:'Home',exact:true}).click();
  await page.locator('[data-zoom-into="system-component-t1"]').press('Enter');
  let chain='system-component-t1';
  for(const id of ['t1-area-k4','t1-area-k14','t1-area-k27','t1-area-k28']){
    await page.locator(`[data-zoom-into="${id}"]`).press('Enter');
    chain+=`/${id}`;await expect(map).toHaveAttribute('data-scene-level',chain);
  }
  // Cards are pointer-transparent: the real canvas pane owns their hit
  // test. A locator click would wait forever for the title to own events.
  let previous='',stable=0;
  for(let i=0;i<60&&stable<3;i++){
    const now=await page.locator('.react-flow__viewport').getAttribute('style');
    stable=now===previous?stable+1:0;previous=now;await page.waitForTimeout(80);
  }
  expect(stable).toBe(3);
  const part=await page.locator('[data-box-title="n-t1-g35"]').boundingBox();
  const canvas=await page.locator('.flow-root').boundingBox();
  expect(part.y).toBeGreaterThanOrEqual(canvas.y);
  expect(part.y+part.height).toBeLessThanOrEqual(canvas.y+canvas.height);
  await page.mouse.click(part.x+part.width/2,part.y+part.height/2);
  await expect(page).toHaveURL(/#n-t1-g35$/);
  await expect(page.getByRole('complementary',{name:'Selected node details',exact:true})).toContainText('Unix shared memory');
});

// Exhaust the saved forest through ordinary keyboard controls, without a new
// model, browser grouping, fixture substitution or sampling of children.
for(const [program,expectedAreas,expectedParts] of [
  ['system-component-t1',28,41],['system-component-t2',17,29],['system-component-t3',19,29],
])test(`complete SQLite UI inventory and reading traversal: ${program}`,async({page},testInfo)=>{
  // Whole-component traversal includes the real report's initial layout and
  // reading every leaf, not just a single navigation scenario.
  test.setTimeout(1_200_000);
  const errors=[];page.on('pageerror',e=>errors.push(e.message));
  page.on('console',msg=>{if(['warning','error'].includes(msg.type()))errors.push(msg.text());});
  await page.goto('/real-report.html');
  const map=page.locator('[data-map]');
  await expect(map).toHaveClass(/flow-enabled/,{timeout:90_000});
  await expect(map).not.toHaveClass(/flow-initializing/,{timeout:90_000});
  const records=await page.locator('[data-node]').evaluateAll(nodes=>nodes.map(n=>{
    const group=document.getElementById((n.getAttribute('href')||'').slice(1)),reading=group&&group.dataset.reading?rmPage.data(group,'reading'):null;
    return {id:n.id,title:n.dataset.title,branch:n.dataset.branch,children:(n.dataset.children||'').split(/\s+/).filter(Boolean),
      declarations:reading?(reading.members||[]).flatMap(kind=>(kind.decls||[]).map(index=>{
        const d=reading.decls[index];return {key:d.key||d.href,name:d.name,at:d.at,href:d.href,open:d.open};
      })):[]};
  }));
  const byID=new Map(records.map(n=>[n.id,n])),expected={areas:new Set(),parts:new Set()};
  function collect(id){for(const child of byID.get(id).children){
    if(byID.get(child).branch==='area'){expected.areas.add(child);collect(child);}else expected.parts.add(child);
  }}
  collect(program);expect(expected.areas.size).toBe(expectedAreas);expect(expected.parts.size).toBe(expectedParts);
  await page.evaluate(`window.__inv=(${invariantKit.toString()})()`);
  const visited={areas:new Set(),parts:new Set()},ledger=[];
  const bare=s=>String(s||'').replace(/\s+/g,'');
  const level=async()=>(await map.getAttribute('data-scene-level'))||'';
  async function settle(){
    // Observe the same stable camera interval in one browser probe. A
    // trace/transport round trip for each 80ms sample distorts its timing.
    const stable=await page.evaluate(async()=>{
      let previous='',stable=0;
      for(let i=0;i<60&&stable<3;i++){
        const now=document.querySelector('.react-flow__viewport')?.getAttribute('style');
        stable=now===previous?stable+1:0;previous=now;
        await new Promise(resolve=>setTimeout(resolve,80));
      }
      return stable;
    });
    expect(stable).toBe(3);
  }
  async function returnTo(parent){
    for(let step=0;await level()!==parent.join('/')&&step<8;step++){
      await page.locator('[data-map-zoom="0.8"]').click();await settle();
    }
    await expect.poll(level).toBe(parent.join('/'));
  }
  async function checkClosed(id){
    const original=byID.get(id),card=page.locator(`[${original.branch==='area'?'data-summary-area':'data-component-overview'}="${id}"]`);
    const observed=await card.evaluate(e=>{
      const r=e.getBoundingClientRect(),h=e.querySelector('[data-box-title]').getBoundingClientRect(),l=e.querySelector('.scene-program-inside')?.getBoundingClientRect(),d=e.querySelector('.flow-description')?.getBoundingClientRect();
      return {rows:[...e.querySelectorAll('[data-program-child]')].map(r=>({id:r.dataset.programChild,title:r.textContent})),title:e.querySelector('[data-box-title]').textContent,ghosts:e.querySelectorAll('.scene-ghosts').length,
        bounds:{box:{top:r.top,bottom:r.bottom,left:r.left,right:r.right},head:{top:h.top,bottom:h.bottom,left:h.left,right:h.right},list:l?{top:l.top,bottom:l.bottom}:null,description:d?{top:d.top,bottom:d.bottom}:null}};
    });
    expect(observed.rows.map(r=>r.id)).toEqual(original.children);
    original.children.forEach((child,i)=>expect(bare(observed.rows[i].title),`${id}: complete name of ${child}`).toBe(bare(byID.get(child).title)));
    expect(bare(observed.title)).toBe(bare(original.title));
    const bounds=observed.bounds;
    expect(bounds.head.left).toBeGreaterThanOrEqual(bounds.box.left-1);expect(bounds.head.right).toBeLessThanOrEqual(bounds.box.right+1);
    if(bounds.list){expect(bounds.list.top).toBeGreaterThanOrEqual((bounds.description||bounds.head).bottom-1);expect(bounds.list.bottom).toBeLessThanOrEqual(bounds.box.bottom+1);}
    expect(observed.ghosts).toBe(0);
    return card;
  }
  async function visit(id,parent){
    await settle();await expect.poll(level).toBe(parent.join('/'));
    const original=byID.get(id);await checkClosed(id);
    // Read every original leaf from the complete named closed inventory.
    for(const child of original.children.filter(child=>byID.get(child).branch!=='area')){
      const card=await checkClosed(id);
      await card.locator(`[data-program-child="${child}"]`).press('Enter');await settle();
      await expect(page).toHaveURL(new RegExp(`#${child}$`));
      const panel=page.getByRole('complementary',{name:'Selected node details',exact:true});
      await expect(panel).toContainText(byID.get(child).title);
      const saved=byID.get(child).declarations;
      const listed=panel.locator('.map-part-reading .map-reading-members .map-reading-name[data-decl-key]');
      expect((await listed.evaluateAll(ds=>ds.map(d=>d.dataset.declKey))).sort(),`${child}: original declaration inventory`).toEqual(saved.map(d=>d.key).sort());
      const decl=listed.filter({visible:true}).first();
      let key=null;
      if(saved.length){
        // An entirely folded original list must still be reachable through
        // its ordinary disclosure before choosing a declaration.
        if(!await decl.count())await panel.locator('.map-part-reading .map-reading-other > summary').click();
        await expect(decl).toBeVisible();
        key=await decl.getAttribute('data-decl-key');
        const original=saved.find(d=>d.key===key);expect(original).toBeDefined();await decl.press('Enter');
        await expect(panel.locator('.map-decl-reading')).toBeVisible();
        const code=panel.locator('.map-decl-reading .map-decl-code');
        expect(bare(await code.textContent())).toBe(bare(original.name));
        if(original.at)expect(await code.getAttribute('title')).toContain(original.at);
        if(original.href)await expect(code).toHaveAttribute('href',original.href);
        if(original.open)await expect(code).toHaveAttribute('data-open',original.open);
        await expect(panel.locator(`.map-part-reading [data-decl-key="${key}"]`).first()).toHaveAttribute('aria-current','true');
        await settle();
      }
      expect(visited.parts.has(child),`${child} visited once`).toBe(false);visited.parts.add(child);
      await returnTo(parent);ledger.push({id:child,kind:'part',parent:id,reading:true,declaration:key,returned:parent});
      console.log(`${program}: read ${child}, declarations ${saved.length}, returned ${parent.join('/')}`);
    }
    await page.locator(`[data-zoom-into="${id}"]`).press('Enter');await settle();
    const chain=[...parent,id];await expect(map).toHaveAttribute('data-scene-level',chain.join('/'));
    for(const child of original.children)await expect(page.locator(`.react-flow__node[data-id="${child}"]`)).toHaveCount(1);
    const words=await page.evaluate(()=>window.__inv.wordAnchor());expect(words.length).toBeGreaterThan(0);expect(words.filter(w=>!w.ok)).toEqual([]);
    const edgeIDs=await page.evaluate(()=>document.querySelector('[data-map]').sceneState().scene.edges.map(e=>e.id));
    const drawnIDs=await page.locator('.flow-root g[data-edge-id]').evaluateAll(es=>es.map(e=>e.dataset.edgeId));
    expect(drawnIDs.sort(),`${id}: all scene arrows rendered`).toEqual(edgeIDs.sort());
    const arrows=await page.evaluate(program=>window.__inv.arrows(program,null,{all:true}),program);
    for(const name of ['one-path','own-ends','head-in','in-frame']){
      expect(arrows[name]?.checked||0,`${id}: ${name} checked every drawn arrow`).toBe(edgeIDs.length);
      expect(arrows[name]?.examples||[],`${id}: ${name}`).toEqual([]);
    }
    if(original.branch==='area'){expect(visited.areas.has(id),`${id} visited once`).toBe(false);visited.areas.add(id);}
    await page.screenshot({path:testInfo.outputPath(`${id}.png`)});
    ledger.push({id,kind:original.branch,chain,children:original.children,wordsChecked:words.length,edgeIDs,arrows});
    console.log(`${program}: opened ${id}, children ${original.children.length}, arrows ${edgeIDs.length}`);
    for(const child of original.children.filter(child=>byID.get(child).branch==='area'))await visit(child,chain);
    await returnTo(parent);await checkClosed(id);
  }
  try{
    await page.getByRole('link',{name:'Home',exact:true}).click();await visit(program,[]);
    expect([...visited.areas].sort()).toEqual([...expected.areas].sort());expect([...visited.parts].sort()).toEqual([...expected.parts].sort());
    expect(errors).toEqual([]);
  }finally{
    const receipt=JSON.stringify({program,expected:{areas:[...expected.areas],parts:[...expected.parts]},visited:{areas:[...visited.areas],parts:[...visited.parts]},ledger,errors},null,2);
    await writeFile(testInfo.outputPath('complete-ui-ledger.json'),receipt);
    await testInfo.attach('complete-ui-ledger',{body:receipt,contentType:'application/json'});
  }
});
