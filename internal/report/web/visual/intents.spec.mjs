import {test,expect} from '@playwright/test';
import {basename} from 'node:path';

// The reader's intents, checked on real reports as the report contract states
// them (REPORT.md "Reader intents and expected screens"): each intent is
// what the screen must and must not show, read from one snapshot of the
// screen, never from a gesture's mechanics. Reports rendered by `repomap
// render` are named as the invariants' are:
//   REPOMAP_INVARIANT_REPORTS=a.html,b.html npx playwright test intents --workers=1
// A mobile version binds other gestures to the same intents; these checks
// stay.
const reports=(process.env.REPOMAP_INVARIANT_REPORTS||'').split(',').filter(Boolean);
test.use({trace:'off',screenshot:'off',video:'off'});

// What the reader sees: the camera, the canvas, every box in sight with its
// rectangle and its name as drawn (screen size, opacity, rectangle), and the
// floating cards and tips.
function screenState(){
  const map=document.querySelector('[data-map]'),root=map.querySelector('.flow-root')||map;
  const c=root.getBoundingClientRect(),canvas={l:c.left,t:c.top,r:c.right,b:c.bottom};
  const v=new DOMMatrixReadOnly(getComputedStyle(map.querySelector('.react-flow__viewport')).transform);
  const opacity=el=>{let o=1;for(let at=el;at&&at!==map;at=at.parentElement)o*=Number(getComputedStyle(at).opacity||1);return o;};
  const screenPx=el=>{const r=el.getBoundingClientRect();return el.offsetWidth?parseFloat(getComputedStyle(el).fontSize)*r.width/el.offsetWidth:0;};
  const rect=el=>{const r=el.getBoundingClientRect();return {l:r.left,t:r.top,r:r.right,b:r.bottom};};
  const boxes=[...map.querySelectorAll('.react-flow__node[data-id]')].map(node=>{
    const names=[...node.querySelectorAll('[data-box-title],.scene-big-name,.flow-chip-name,.scene-tile-lines,.scene-group-title strong')]
      .map(el=>({text:el.textContent.trim().slice(0,60),px:screenPx(el),opacity:opacity(el),rect:rect(el)})).filter(name=>name.text);
    return {id:node.dataset.id,display:[...node.classList].find(c=>c.startsWith('scene-is-'))?.slice(9)||'',rect:rect(node),names};
  });
  const floating=[...document.querySelectorAll('.flow-floating-card,.scene-tip')].filter(el=>el.getClientRects().length).map(rect);
  return {camera:{x:v.e,y:v.f,zoom:v.a},canvas,boxes,floating};
}
const meets=(a,b)=>a.l<b.r&&a.r>b.l&&a.t<b.b&&a.b>b.t;
const inside=(a,b)=>a.l>=b.l-1&&a.r<=b.r+1&&a.t>=b.t-1&&a.b<=b.b+1;
// A name reads from 10.5 screen pixels, an outside system's from 9.5, whole
// in its box's sight and unfaded.
const reads=(name,display)=>name.opacity>.5&&name.px>=(['chip','bucket','tile','kindgroup'].includes(display)?9.5:10.5);

async function open(page,url){
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto(url);
  await expect(page.locator('[data-map]')).toHaveClass(/flow-enabled/,{timeout:120000});
  await expect(page.locator('[data-map]')).not.toHaveClass(/flow-initializing/,{timeout:300000});
  await page.waitForTimeout(800);
  return errors;
}
const snapshot=page=>page.evaluate(screenState);

for(const [index,file] of reports.entries()){
  const repo=basename(file,'.html');
  test(`${repo}: the reader's intents`,async({page})=>{
    test.setTimeout(600000);
    const errors=await open(page,`/invariant-${index}.html`);

    // OVERVIEW: every box in sight large enough to hold a name names itself;
    // two boxes' names do not overlap.
    const home=await snapshot(page);
    const seen=home.boxes.filter(box=>inside(box.rect,home.canvas)&&box.rect.r-box.rect.l>=48&&box.rect.b-box.rect.t>=24);
    for(const box of seen)expect.soft(box.names.some(name=>reads(name,box.display)),`OVERVIEW ${repo}: ${box.id} (${box.display}) names itself`).toBe(true);
    const named=seen.flatMap(box=>box.names.filter(name=>reads(name,box.display)).map(name=>({box:box.id,rect:name.rect})));
    for(let i=0;i<named.length;i++)for(let j=i+1;j<named.length;j++)
      if(named[i].box!==named[j].box)expect.soft(meets(named[i].rect,named[j].rect),`OVERVIEW ${repo}: ${named[i].box} and ${named[j].box} names overlap`).toBe(false);

    // PEEK: pointing at a box changes nothing but emphasis: the camera and
    // every box stand where they stood.
    for(const box of seen.slice(0,5)){
      await page.mouse.move((box.rect.l+box.rect.r)/2,(box.rect.t+box.rect.b)/2,{steps:3});await page.waitForTimeout(250);
      const after=await snapshot(page);
      expect.soft(after.camera,`PEEK ${repo}: pointing at ${box.id} keeps the camera`).toEqual(home.camera);
      const moved=after.boxes.filter(other=>{const was=home.boxes.find(b=>b.id===other.id);return was&&Math.max(...['l','t','r','b'].map(k=>Math.abs(was.rect[k]-other.rect[k])))>1;});
      expect.soft(moved.map(b=>b.id),`PEEK ${repo}: pointing at ${box.id} moves no box`).toEqual([]);
    }
    await page.mouse.move(home.canvas.l+2,home.canvas.b-2);await page.waitForTimeout(200);

    // OPEN: a program opened keeps its name where and as large as its card
    // drew it (in world units), and its drawing stands below its header.
    const program=await page.locator('.scene-zoom[data-zoom-into^="system-component-"]').first();
    if(await program.count()){
      const id=await program.getAttribute('data-zoom-into');
      const closed=home.boxes.find(box=>box.id===id),title=closed?.names.find(name=>name.opacity>.5);
      // The canvas hit-tests its marks itself: a click at the magnifier.
      const at=await program.boundingBox();await page.mouse.click(at.x+at.width/2,at.y+at.height/2);await page.waitForTimeout(1500);
      const opened=await snapshot(page),frame=opened.boxes.find(box=>box.id===id)||opened.boxes.find(box=>title&&box.names.some(name=>name.text===title.text));
      // A program that is one part opens as that part, named by the program
      // (REPORT.md); any other as its frame.
      expect.soft(['frame','card','deep'].includes(frame?.display),`OPEN ${repo}: ${id} opens in its place (${frame?.display})`).toBe(true);
      const header=frame?.names[0];
      if(title&&header){
        const world=(px,zoom)=>px/zoom;
        // A frame keeps its card's header exactly; a program that is one
        // part opens as that part's card, drawn at its own level, its name
        // within a tenth of its card's.
        const share=frame.display==='frame'?.02:.1;
        expect.soft(Math.abs(world(header.px,opened.camera.zoom)-world(title.px,home.camera.zoom))<=share*world(title.px,home.camera.zoom),
          `OPEN ${repo}: ${id}'s name keeps its size (${title.px.toFixed(1)} px at ${home.camera.zoom.toFixed(3)} → ${header.px.toFixed(1)} px at ${opened.camera.zoom.toFixed(3)})`).toBe(true);
        expect.soft(reads(header,'frame'),`OPEN ${repo}: ${id}'s name reads when opened`).toBe(true);
      }
      expect.soft(opened.camera.zoom>=home.camera.zoom-1e-9,`OPEN ${repo}: opening never zooms out`).toBe(true);
    }
    expect(errors,`${repo}: page errors`).toEqual([]);
  });
}
