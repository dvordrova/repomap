import {test,expect} from '@playwright/test';
import {basename} from 'node:path';
import {journeys,journeyHelpers,lintLevel,lintOutside,levels} from './journeys.mjs';

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
    const v=await page.evaluate(()=>JSON.stringify(document.querySelector('[data-map]').captureViewport?.())+document.querySelector('.map-inspector')?.textContent.length);
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
      let result='';
      if(id){await settle(page);result=await journey.check(page);}
      lines.push(`${result?'PASS':'FAIL'}  ${repo} · ${journey.input} → ${journey.says}${result?` (${result})`:id?'':' (no such input)'}`);
    }
    console.log(lines.join('\n'));
    test.info().annotations.push(...lines.map(line=>({type:'journey',description:line})));
  });

  test(`reading lints of ${basename(file)}`,async({page})=>{
    test.setTimeout(900_000);
    const errors=[];page.on('pageerror',error=>errors.push(error.message));
    await open(page,index);
    const repo=await repoOf(page),findings=[];
    await page.evaluate(`window.__lintLevel=${lintLevel.toString()}`);
    const lint=async(level,cards=2)=>{
      findings.push(...await page.evaluate(level=>window.__lintLevel(level),`${repo} ${level}`));
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
    for(const level of await page.evaluate(levels)){
      await page.evaluate(id=>document.querySelector('[data-map-explorer]').goToLevel({id,kind:'frame'}),level.id);
      await settle(page);
      await lint(`${level.branch} ${level.title}`,level.branch==='part'?0:2);
    }
    const seen=new Set(),unique=findings.filter(f=>{const key=`${f.kind}|${f.element}`;if(seen.has(key))return false;seen.add(key);return true;});
    const counts={};for(const f of unique)counts[f.kind]=(counts[f.kind]||0)+1;
    console.log(`${basename(file)}: ${unique.length} offenders ${JSON.stringify(counts)}\n`+unique.map(f=>`  ${f.kind.padEnd(11)} ${f.level} · ${f.element}`).join('\n'));
    expect(errors,'the page raises no error').toEqual([]);
    expect(unique.map(f=>`${f.kind} · ${f.level} · ${f.element}`)).toEqual([]);
  });
}
