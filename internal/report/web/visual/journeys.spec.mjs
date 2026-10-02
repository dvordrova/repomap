import {test,expect} from '@playwright/test';
import {basename} from 'node:path';
import {journeys,journeyHelpers,lintLevel,lintOutside,lintPath,collapsedLists,longestFold,inputSample,levels,fileLine} from './journeys.mjs';

// The frozen journey check and the reading lints (journeys.mjs) on reports
// rendered by `repomap render` (no provider call), named by
// REPOMAP_JOURNEY_REPORTS (a comma list of HTML files), which the test
// server serves as /journey-<n>.html:
//   REPOMAP_JOURNEY_REPORTS=a.html,b.html npx playwright test journeys
// A journey prints PASS or FAIL and never fails the run; a lint fails it,
// naming each offender with its level.
const reports=(process.env.REPOMAP_JOURNEY_REPORTS||'').split(',').filter(Boolean);

async function open(page,index){
  await page.goto(`/journey-${index}.html`);
  const map=page.locator('[data-map]');
  await expect(map).toHaveClass(/flow-enabled/,{timeout:120_000});
  await expect(map).not.toHaveClass(/flow-initializing/,{timeout:120_000});
  return map;
}
async function settle(page){
  let previous='',stable=0;
  for(let i=0;i<100&&stable<2;i++){
    // The camera, the reading and the drawing all at rest: a layer opening
    // after the camera stops redraws the cards.
    const v=await page.evaluate(()=>JSON.stringify(document.querySelector('[data-map]').captureViewport?.())+document.querySelector('.map-inspector')?.textContent.length+':'+document.querySelector('.flow-root')?.innerHTML.length);
    stable=v===previous?stable+1:0;previous=v;await page.waitForTimeout(120);
  }
}
const repoOf=page=>page.evaluate(()=>(document.querySelector('.report-toolbar .repo')?.textContent||document.title||'').trim());

for(const [index,file] of reports.entries()){
  test(`journeys of ${basename(file)}`,async({page})=>{
    test.setTimeout(300_000);
    await open(page,index);
    const repo=await repoOf(page),lines=[];
    for(const journey of journeys.filter(journey=>journey.repo.test(repo))){
      await page.evaluate(`(${journeyHelpers.toString()})()`);
      const id=await page.evaluate(title=>{
        const n=[...document.querySelectorAll('[data-map-explorer] [data-node]')].find(n=>n.dataset.activation&&n.dataset.title===title);
        if(n)document.querySelector('[data-map-explorer]').exploreNode(n.id);
        return n?.id||'';
      },journey.input);
      let [passed,found]=[false,'no such input'];
      if(id){await settle(page);[passed,found]=await journey.check(page);}
      lines.push(`${passed?'PASS':'FAIL'}  ${repo} · ${journey.input} → ${journey.says} (${found})`);
    }
    console.log(lines.join('\n'));
    test.info().annotations.push(...lines.map(line=>({type:'journey',description:line})));
  });

  test(`reading lints of ${basename(file)}`,async({page})=>{
    test.setTimeout(900_000);
    const errors=[];page.on('pageerror',error=>errors.push(error.message));
    await open(page,index);
    const repo=await repoOf(page),findings=[];
    await page.evaluate(`window.__fileLine=${fileLine.toString()};window.__lintLevel=${lintLevel.toString()};window.__lintPath=${lintPath.toString()};window.__collapsedLists=${collapsedLists.toString()}`);
    const lint=async(level,cards=2)=>{
      findings.push(...await page.evaluate(level=>window.__lintLevel(level),`${repo} ${level}`));
      findings.push(...await page.evaluate(level=>window.__lintPath(level),`${repo} ${level}`));
      findings.push(...(await page.evaluate(()=>window.__collapsedLists())).filter(list=>list.rows>longestFold)
        .map(list=>({kind:'long fold',element:`"${list.fold}" folds ${list.rows} rows (${list.kind}) under no named fold`,level:`${repo} ${level}`})));
      // The cards of a few arrows in sight, opened as the pointer opens them.
      const hits=await page.evaluate(()=>{
        const canvas=document.querySelector('.flow-root').getBoundingClientRect();
        return [...document.querySelectorAll('[data-edge-hit]')].map(path=>{
          const length=path.getTotalLength(),ctm=path.getScreenCTM(),p=path.getPointAtLength(length*.5);
          return {x:p.x*ctm.a+p.y*ctm.c+ctm.e,y:p.x*ctm.b+p.y*ctm.d+ctm.f};
        }).filter(p=>p.x>canvas.left+20&&p.x<canvas.right-20&&p.y>canvas.top+20&&p.y<canvas.bottom-20);
      });
      for(const point of hits.slice(0,cards)){
        await page.mouse.move(point.x,point.y,{steps:4});
        if(await page.locator('.flow-floating-card').first().waitFor({timeout:1200}).then(()=>true,()=>false))
          findings.push(...await page.evaluate(level=>window.__lintLevel(level),`${repo} ${level} · arrow card`));
        await page.keyboard.press('Escape');await page.mouse.move(2,2);await page.waitForTimeout(80);
      }
    };
    findings.push(...await page.evaluate(lintOutside));
    await lint('whole map');
    const named=journeys.map(journey=>journey.input);
    for(const level of [...await page.evaluate(levels),...await page.evaluate(`(${inputSample.toString()})(${JSON.stringify(named)})`)]){
      await page.evaluate(id=>document.querySelector('[data-map-explorer]').goToLevel({id,kind:'frame'}),level.id);
      await settle(page);
      await lint(`${level.branch} ${level.title} [${level.id}]`,level.branch==='part'?0:2);
    }
    const seen=new Set(),unique=findings.filter(f=>{const key=`${f.kind}|${f.element}`;if(seen.has(key))return false;seen.add(key);return true;});
    const counts={};for(const f of unique)counts[f.kind]=(counts[f.kind]||0)+1;
    console.log(`${basename(file)}: ${unique.length} offenders ${JSON.stringify(counts)}\n`+unique.map(f=>`  ${f.kind.padEnd(11)} ${f.level} · ${f.element}`).join('\n'));
    expect(errors,'the page raises no error').toEqual([]);
    expect(unique.map(f=>`${f.kind} · ${f.level} · ${f.element}`)).toEqual([]);
  });

  // Leaving an input's path shows on the canvas what the column then reads:
  // opened by a link, or by one search after another, "Leave input path"
  // had read System map while the canvas stayed on the old path, nothing
  // marked (reviewer, 2026-09-30).
  test(`leaving an input path of ${basename(file)}`,async({page})=>{
    test.setTimeout(300_000);
    const errors=[];page.on('pageerror',error=>errors.push(error.message));
    await open(page,index);
    const inputs=await page.evaluate(()=>[...document.querySelectorAll('[data-map-explorer] [data-node][data-activation]')].filter(n=>n.dataset.inputPath).map(n=>n.id).slice(0,2));
    expect(inputs.length,'inputs with a path').toBeGreaterThan(1);
    const agree=async how=>{
      await settle(page);
      const state=await page.evaluate(()=>{const map=document.querySelector('[data-map]');return {fit:!!map.captureViewport().fit,pinned:map.dataset.operationPinned};});
      expect(state,`${how}: the column reads the whole map and the canvas shows it`).toEqual({fit:true,pinned:'false'});
    };
    const leave=async()=>{await page.locator('.map-input-context button:not(.system-input-start)').last().click();};
    // By a link followed in the page, and by the page reloaded at it.
    for(const how of ['a link','a reload']){
      if(how==='a reload')await page.goto('about:blank');
      await page.goto(`/journey-${index}.html#${inputs[0]}`);
      await expect(page.locator('[data-map]')).toHaveClass(/flow-enabled/,{timeout:120_000});
      await expect(page.locator('[data-map]')).not.toHaveClass(/flow-initializing/,{timeout:120_000});
      await settle(page);
      expect(await page.evaluate(()=>document.querySelector('[data-map]').dataset.operationPinned)).toBe('true');
      await leave();await agree(`opened by ${how}`);
    }
    // By one search after another, from the whole map.
    for(const id of inputs){await page.evaluate(id=>document.querySelector('[data-map]').findNode(document.getElementById(id)),id);await settle(page);}
    await leave();await agree('two searches in a row');
    expect(errors).toEqual([]);
  });

  // Every name the home's programs list says does something, and a kind
  // chosen on the canvas is marked where the column reads it (owner,
  // 2026-09-30: "ничего не кликабельное", "я в колонке не вижу, что я
  // тыкнул на канвасе"). Each row naming a program, an entry, an input kind
  // or a connection is a link or a button with a name; a few of each, one
  // per kind, clicked, change the column; a kind clicked on the canvas's
  // Inputs card reads "Inputs · {kind}" with its section marked in sight.
  test(`clickable rows of ${basename(file)}`,async({page})=>{
    test.setTimeout(300_000);
    const errors=[];page.on('pageerror',error=>errors.push(error.message));
    await open(page,index);await settle(page);
    const rows=await page.evaluate(()=>{
      const out=[];const list=document.querySelector('.map-inspector .system-programs-list');if(!list)return [{kind:'list',text:'no programs list',ok:false}];
      let label='';
      for(const el of list.children){
        if(el.tagName==='DT'){label=el.classList.contains('system-program-name')?'program':el.textContent.trim();if(label==='program'){const b=el.querySelector('button,a[href]');out.push({kind:'program',text:el.textContent.trim(),ok:!!b&&!!(b.getAttribute('aria-label')||b.textContent.trim())});}continue;}
        const kind=({Entry:'entry',Inputs:'input kind',Connections:'connection'})[label];if(!kind)continue;
        const items=el.tagName==='DD'&&el.querySelector('ul')?[...el.querySelectorAll(':scope>ul>li')]:[el];
        if(kind==='input kind'){const named=[...el.querySelectorAll('button,a[href]')];out.push({kind,text:el.textContent.trim().slice(0,60),ok:named.length>0&&named.length===el.textContent.split('·').length});continue;}
        for(const item of items){const b=item.querySelector('button,a[href]');out.push({kind,text:item.textContent.trim().slice(0,60),ok:!!b&&!!b.textContent.trim()});}
      }
      return out;
    });
    const dead=rows.filter(row=>!row.ok);
    expect(dead.map(row=>`${row.kind}: "${row.text}" is no link`),'every row of the programs list does something').toEqual([]);
    // One of each kind, clicked from the home, changes the column.
    for(const kind of ['entry','input kind','connection']){
      await page.evaluate(()=>document.querySelector('[data-map-explorer]').showWholeMap());await settle(page);
      const before=await page.evaluate(()=>document.querySelector('.map-inspector').innerText.length+':'+(document.querySelector('.map-inspector .map-card-kind')?.textContent||''));
      const clicked=await page.evaluate(label=>{
        let at='';for(const el of document.querySelector('.map-inspector .system-programs-list').children){if(el.tagName==='DT'){at=el.textContent.trim();continue;}
          if(({Entry:'entry',Inputs:'input kind',Connections:'connection'})[at]===label){const b=el.querySelector('button,a[href]:not([target])');if(b){b.click();return true;}}}
        return false;
      },kind);
      if(!clicked)continue;
      await settle(page);
      const after=await page.evaluate(()=>document.querySelector('.map-inspector').innerText.length+':'+(document.querySelector('.map-inspector .map-card-kind')?.textContent||''));
      expect(after,`a ${kind} clicked in the programs list changes the column`).not.toEqual(before);
    }
    // A kind chosen on the canvas's Inputs card: its section marked in sight.
    await page.evaluate(()=>document.querySelector('[data-map-explorer]').showWholeMap());await settle(page);
    const kind=page.locator('.flow-root .flow-input-kind').first();
    if(await kind.count()){
      await kind.click();await settle(page);
      const said=await page.evaluate(()=>{
        const column=document.querySelector('.map-inspector-content'),box=column.getBoundingClientRect(),picked=column.querySelector('.map-reading-picked');
        const r=picked?.getBoundingClientRect();
        return {heading:document.querySelector('.map-inspector .map-card-kind')?.textContent||'',marked:!!picked,inSight:!!r&&r.top<box.bottom&&r.bottom>box.top};
      });
      expect(said.heading,'the heading says which kind was clicked').toMatch(/^Inputs · \S/);
      expect(said.marked&&said.inSight,'the clicked kind is marked in sight').toBe(true);
      const inputs=await page.evaluate(()=>[...document.querySelectorAll('.map-inspector-content .map-collection-names>li')].filter(li=>!li.querySelector('button,a[href]')).map(li=>li.textContent.trim().slice(0,60)));
      expect(inputs.map(text=>`input "${text}" is no link`),'every input of the collection reading does something').toEqual([]);
    }
    // A program's reading: its entry and its input kinds do something.
    await page.evaluate(()=>document.querySelector('[data-map-explorer]').showWholeMap());await settle(page);
    const program=page.locator('.map-inspector .system-programs-list dt.system-program-name button').first();
    if(await program.count()){
      await program.click();await settle(page);
      const deadRows=await page.evaluate(()=>[...document.querySelectorAll('.map-inspector-content .map-component-entry>div,.map-inspector-content .map-component-input-kinds>li')].filter(row=>!row.querySelector('button,a[href]')).map(row=>row.textContent.trim().slice(0,60)));
      expect(deadRows.map(text=>`"${text}" is no link`),'every entry and input kind of a program reading does something').toEqual([]);
    }
    expect(errors).toEqual([]);
  });
}
