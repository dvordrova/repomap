import {test,expect} from '@playwright/test';
import {basename,join} from 'node:path';
import {mkdir,writeFile,readFile} from 'node:fs/promises';
import {invariants,invariantKit,compare,random,hash} from './invariants.mjs';
import {lintCanvas} from './geometry.mjs';

// The rewrite's invariant table (invariants.mjs): every invariant at every
// level of each report, on the old path and on `?scene=1`. Reports rendered
// by `repomap render` (no provider call) are named by
// REPOMAP_INVARIANT_REPORTS, a comma list of HTML files the test server
// serves as /invariant-<n>.html; the seeded synthetic graphs of
// fixtures/synthetic-*.json are named by REPOMAP_INVARIANT_GRAPHS (a comma
// list of their names, or "all"), drawn by the fixture page:
//   REPOMAP_INVARIANT_REPORTS=a.html,b.html REPOMAP_INVARIANT_OUT=dir \
//     npx playwright test invariants --workers=1
// REPOMAP_INVARIANT_PATHS picks "old", "scene" or both (default both).
// Each report and path writes <out>/<repo>.<path>.json; the table is
// written by `node visual/invariant-table.mjs <out>`. The run passes unless
// REPOMAP_INVARIANT_STRICT is set: the table is the result.
const reports=(process.env.REPOMAP_INVARIANT_REPORTS||'').split(',').filter(Boolean);
const graphNames=process.env.REPOMAP_INVARIANT_GRAPHS==='all'?['no-inputs','outside-150','cycles','loose-40']:(process.env.REPOMAP_INVARIANT_GRAPHS||'').split(',').filter(Boolean);
const paths=(process.env.REPOMAP_INVARIANT_PATHS||'old,scene').split(',').filter(Boolean);
const out=process.env.REPOMAP_INVARIANT_OUT||new URL('../test-results/invariants/',import.meta.url).pathname;
const pans=Number(process.env.REPOMAP_INVARIANT_PANS||24),moves=Number(process.env.REPOMAP_INVARIANT_MOVES||50);
const strict=!!process.env.REPOMAP_INVARIANT_STRICT;
// REPOMAP_INVARIANT_LEVELS narrows the levels by a pattern on their names.
const only=process.env.REPOMAP_INVARIANT_LEVELS?new RegExp(process.env.REPOMAP_INVARIANT_LEVELS,'i'):null;

// An hour of pointer moves recorded as a trace fills the disk: none here.
test.use({trace:'off',screenshot:'off',video:'off'});

const targets=[
  ...reports.map((file,index)=>({repo:basename(file,'.html'),url:`/invariant-${index}.html`,file})),
  ...graphNames.map(name=>({repo:`synthetic-${name}`,url:`/?graph=synthetic-${name}`,file:`fixtures/synthetic-${name}.json`})),
];

async function settle(page,{first=120,most=6000}={}){
  await page.waitForTimeout(first);
  let previous='',stable=0;const until=Date.now()+most;
  while(stable<3&&Date.now()<until){
    const now=await page.evaluate(()=>window.__inv.signature());
    stable=now===previous?stable+1:0;previous=now;
    await page.waitForTimeout(90);
  }
}
const park=async page=>{await page.mouse.move(3,3);await page.waitForTimeout(60);};

for(const target of targets)for(const path of paths){
  test(`invariants of ${target.repo} (${path})`,async({page})=>{
    test.setTimeout(Number(process.env.REPOMAP_INVARIANT_TIMEOUT||4*3600_000));
    const errors=[];page.on('pageerror',error=>errors.push(error.message));
    const sep=target.url.includes('?')?'&':'?';
    await page.goto(path==='scene'?`${target.url}${sep}scene=1`:target.url);
    await page.waitForFunction(()=>{const m=document.querySelector('[data-map]');return m&&m.classList.contains('flow-enabled')&&!m.classList.contains('flow-initializing')&&document.querySelector('.flow-root');},null,{timeout:180_000});
    await page.evaluate(`window.__inv=(${invariantKit.toString()})();window.__lintCanvas=${lintCanvas.toString()}`);
    const explorer=await page.evaluate(()=>!!document.querySelector('[data-map-explorer]')?.goToLevel);
    const go=async id=>{
      await page.evaluate(async([id,explorer])=>{
        const map=document.querySelector('[data-map]');
        await map.showWholeMap?.();
        if(!id)return;
        if(explorer)await document.querySelector('[data-map-explorer]').goToLevel({id,kind:'frame'});
        else await map.focusNode?.(id);
      },[id,explorer]);
      await park(page);await settle(page,{first:400});
    };
    await go('');
    const sceneOn=await page.evaluate(()=>window.__inv.scene());
    // The levels: the whole map, every program, every area of the two
    // largest programs.
    const programs=await page.evaluate(()=>window.__inv.programs());
    const largest=[...programs].sort((a,b)=>b.parts-a.parts).slice(0,2);
    const levels=[{kind:'home',id:'',name:'home'},...programs.map(p=>({kind:'program',id:p.id,name:`program ${p.title}`})),
      ...largest.flatMap(p=>p.areas.map(a=>({kind:'area',id:a.id,frame:a.id,name:`area ${p.title} / ${a.title}`})))];
    const result={repo:target.repo,file:target.file,path,sceneOn,viewport:page.viewportSize(),pans,moves,generated:new Date().toISOString(),levels:[]};
    for(const level of levels.filter(level=>!only||only.test(level.name))){
      const started=Date.now(),errorsBefore=errors.length;
      const r={};for(const [name] of invariants)r[name]={checked:0,failed:0,examples:[]};
      const add=(name,ok,example)=>{r[name].checked++;if(!ok){r[name].failed++;if(r[name].examples.length<8)r[name].examples.push(String(example).slice(0,200));}};
      const merge=(name,part)=>{if(!part)return;r[name].checked+=part.checked;r[name].failed+=part.failed;r[name].examples.push(...part.examples.slice(0,8-r[name].examples.length));};
      const info={};
      try{
        await go(level.id);
        const rng=random(hash(`${target.repo}|${level.name}`));
        const rest=await page.evaluate(()=>window.__inv.snapshot());
        info.level=rest.level;info.zoom=rest.cam.zoom;
        // At rest: the arrows, the titles, the markers and the texts.
        const arrows=await page.evaluate(frame=>window.__inv.arrows(frame),level.kind==='home'?'':level.id);
        for(const name of ['one-path','own-ends','head-in','shared-run','in-frame','off-canvas','crosses','lanes'])merge(name,arrows[name]);
        info.bends=arrows.bends;info.lanes=arrows.lanes?.widest;
        const titles=await page.evaluate(frame=>window.__inv.titles(frame),level.kind==='home'?'':level.id);
        if(titles.length>1){
          const sorted=titles.map(t=>t.px).sort((a,b)=>a-b),median=sorted[Math.floor(sorted.length/2)];
          for(const t of titles)add('titles',t.px>=median*.9-.01&&t.px<=median*1.1+.01,`"${t.text}" ${t.px.toFixed(1)}px against a median ${median.toFixed(1)}px`);
          info.titles={min:sorted[0],median,max:sorted.at(-1),count:titles.length};
        }
        merge('markers',await page.evaluate(()=>window.__inv.markers()));
        const lints=await page.evaluate(([level,whole])=>window.__lintCanvas(level,3,!whole),[level.name,level.kind==='home']);
        const texts=await page.evaluate(()=>window.__inv.textCount());
        r['no-labels'].checked+=texts;r.text.checked+=texts;
        for(const finding of lints){
          if(finding.kind==='label'){r['no-labels'].failed++;if(r['no-labels'].examples.length<8)r['no-labels'].examples.push(finding.element);}
          if(finding.kind==='small'||finding.kind==='clipped'){r.text.failed++;if(r.text.examples.length<8)r.text.examples.push(`${finding.kind} ${finding.element}`);}
        }
        if(level.kind==='home')for(const frame of await page.evaluate(()=>window.__inv.outside()))add('outside-40',frame.items<=40,`${frame.id}: ${frame.items} items`);
        // Pointed at, every port and marker draws its lines: they are
        // arrows too, checked as the ones at rest.
        const restPaths=await page.evaluate(()=>window.__inv.paths());
        const handles=await page.evaluate(()=>window.__inv.handles());
        info.handles=handles.length;
        for(const handle of handles){
          await page.mouse.move(handle.x,handle.y,{steps:2});await page.waitForTimeout(160);
          const pointed=await page.evaluate(([frame,skip])=>window.__inv.arrows(frame,skip),[level.kind==='home'?'':level.id,restPaths]);
          for(const name of ['one-path','own-ends','head-in','shared-run','in-frame','off-canvas','crosses'])
            merge(name,pointed[name]&&{...pointed[name],examples:pointed[name].examples.map(e=>`pointing at ${handle.id}: ${e}`)});
        }
        if(handles.length){await page.keyboard.press('Escape');await park(page);}
        // Ports and markers by camera.
        const sizes=new Map();
        const measure=async where=>{for(const p of await page.evaluate(()=>window.__inv.ports())){if(!sizes.has(p.id))sizes.set(p.id,[]);sizes.get(p.id).push([p.w,where]);}};
        await measure('at rest');
        // Random pans, in pairs that come back, each from an empty spot.
        let vector={dx:0,dy:0};
        for(let i=0;i<pans;i++){
          vector=i%2?{dx:-vector.dx,dy:-vector.dy}:{dx:Math.round((rng()-.5)*500),dy:Math.round((rng()-.5)*360)};
          const spot=await page.evaluate(seed=>window.__inv.emptySpot(seed),Math.floor(rng()*2**31));
          await page.mouse.move(spot.x,spot.y,{steps:2});await page.waitForTimeout(80);
          const before=await page.evaluate(()=>window.__inv.snapshot());
          const how=rng()<.75?'wheel':'drag';
          if(how==='wheel')await page.mouse.wheel(vector.dx,vector.dy);
          else{await page.mouse.down();await page.mouse.move(spot.x-vector.dx,spot.y-vector.dy,{steps:6});await page.mouse.up();}
          await settle(page,{first:300});
          const after=await page.evaluate(()=>window.__inv.snapshot());
          const moved=Math.hypot(after.cam.x-before.cam.x,after.cam.y-before.cam.y),said=`${how} pan ${i+1} by ${moved.toFixed(0)}px`;
          const diff=compare(before,after);
          add('pan-level',after.level===before.level,`${said}: ${before.level} → ${after.level}`);
          add('pan-keeps',!diff.lost.length,`${said}: lost ${diff.lost.slice(0,5).join(', ')}${diff.lost.length>5?` and ${diff.lost.length-5} more`:''}`);
          add('pan-fixed',!diff.moved.length&&Math.abs(after.cam.zoom-before.cam.zoom)<1e-6,`${said}: ${diff.moved.length?`moved ${diff.moved.slice(0,5).join(', ')}`:`zoom ${before.cam.zoom.toFixed(3)} → ${after.cam.zoom.toFixed(3)}`}`);
          await measure(`after ${said}`);
          if(moved<5)info.stuckPans=(info.stuckPans||0)+1;
        }
        // Back where the level stands, the pointer moves at random.
        await go(level.id);
        const still=await page.evaluate(()=>window.__inv.snapshot());
        let dark={checked:0,bad:[]};
        const canvas=still.canvas;
        for(let i=0;i<moves;i++){
          const x=canvas.l+4+rng()*(canvas.r-canvas.l-8),y=canvas.t+4+rng()*(canvas.b-canvas.t-8);
          await page.mouse.move(x,y,{steps:3});await page.waitForTimeout(70);
          const now=await page.evaluate(()=>window.__inv.snapshot());
          const diff=compare(still,now);
          add('point-keeps',!diff.lost.length,`at (${x.toFixed(0)},${y.toFixed(0)}): lost ${diff.lost.slice(0,5).join(', ')}${diff.lost.length>5?` and ${diff.lost.length-5} more`:''}`);
          add('point-style',!diff.moved.length&&!diff.repathed.length&&!diff.cameraMoved&&now.level===still.level,
            `at (${x.toFixed(0)},${y.toFixed(0)}): ${[diff.cameraMoved&&'camera moved',now.level!==still.level&&`level ${still.level} → ${now.level}`,diff.moved.length&&`boxes moved ${diff.moved.slice(0,3).join(', ')}`,diff.repathed.length&&`paths changed ${diff.repathed.slice(0,3).join(', ')}`].filter(Boolean).join('; ')}`);
          const order=await page.evaluate(()=>window.__inv.darkOnTop());dark.checked+=order.checked;dark.bad.push(...order.bad);
          if(diff.cameraMoved)await go(level.id);
        }
        await page.keyboard.press('Escape');await park(page);
        // Every arrow pointed at opens its card.
        await go(level.id);
        const ids=[...new Set(await page.evaluate(()=>window.__inv.hitIds()))];
        info.arrows=ids.length;let outOfSight=0;
        for(const id of ids){
          const point=await page.evaluate(id=>window.__inv.hitPoint(id),id);
          if(point.outOfSight){outOfSight++;continue;}
          if(point.missing)continue;
          if(point.covered){add('cards',false,`${id}: covered wherever it is in sight (by ${point.by})`);continue;}
          await page.mouse.move(point.x-14,point.y-14,{steps:2});await page.mouse.move(point.x,point.y,{steps:4});
          let opened=false;for(let k=0;k<22&&!opened;k++){await page.waitForTimeout(90);opened=await page.evaluate(()=>window.__inv.cardOpen());}
          add('cards',opened,`${id}: no card`);
          const order=await page.evaluate(()=>window.__inv.darkOnTop());dark.checked+=order.checked;dark.bad.push(...order.bad);
          await page.keyboard.press('Escape');await park(page);
          if(!(await page.evaluate(([cam])=>{const now=window.__inv.camera();return Math.abs(now.x-cam.x)<.5&&Math.abs(now.y-cam.y)<.5&&Math.abs(now.zoom-cam.zoom)<1e-6;},[still.cam])))await go(level.id);
        }
        info.arrowsOutOfSight=outOfSight;
        r['dark-top'].checked+=dark.checked;r['dark-top'].failed+=dark.bad.length;r['dark-top'].examples.push(...[...new Set(dark.bad)].slice(0,8));
        // A few zooms about the level: ports and markers keep their size.
        for(const factor of [1.25,.8,1.5]){
          await go(level.id);
          await page.evaluate(f=>{const b=[...document.querySelectorAll('[data-map-zoom]')].find(b=>Number(b.dataset.mapZoom)===(f>1?1.25:.8));b?.click();},factor);
          await settle(page,{first:450});await measure(`zoomed ${factor>1?'in':'out'}`);
        }
        for(const [id,seen] of sizes){
          const sorted=[...seen].sort((a,b)=>a[0]-b[0]),[lo,low]=sorted[0],[hi,high]=sorted.at(-1);
          add('marker-size',lo>=19.5&&hi<=28.5&&hi-lo<=1.01,`${id}: ${lo.toFixed(1)} px ${low}, ${hi.toFixed(1)} px ${high} (${seen.length} cameras)`);
        }
      }catch(error){
        info.error=String(error.message||error).split('\n')[0].slice(0,300);
      }
      const raised=errors.slice(errorsBefore);
      add('page-errors',!raised.length,raised.slice(0,3).join(' | '));
      info.seconds=Math.round((Date.now()-started)/1000);
      result.levels.push({...level,info,invariants:r});
      console.log(`${target.repo} ${path} · ${level.name}: ${invariants.filter(([n])=>r[n].failed).map(([n])=>`${n} ${r[n].failed}/${r[n].checked}`).join(', ')||'all pass'} (${info.seconds}s${info.error?`; error ${info.error}`:''})`);
    }
    await mkdir(out,{recursive:true});
    await writeFile(join(out,`${target.repo}.${path}.json`),JSON.stringify(result,null,1));
    if(strict){
      const failed=result.levels.flatMap(level=>invariants.filter(([n])=>level.invariants[n].failed).map(([n])=>`${level.name} · ${n}: ${level.invariants[n].examples[0]||''}`));
      expect(failed).toEqual([]);
    }
  });
}
