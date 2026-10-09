import {test,expect} from '@playwright/test';

// The toolbar's breadcrumb on a real report (REPOMAP_REAL_RUN, rendered by
// .bin/repomap render); the fixture has none. On Redis the breadcrumb was
// one link that went nowhere.
test.skip(!process.env.REPOMAP_REAL_RUN,'set REPOMAP_REAL_RUN to a saved run directory');
test.describe.configure({mode:'serial'});

async function settle(map){
  let previous='',stable=0;
  await expect.poll(async()=>{const v=JSON.stringify(await map.evaluate(map=>map.captureViewport()));stable=v===previous?stable+1:0;previous=v;return stable;},{intervals:[100],timeout:30_000}).toBeGreaterThanOrEqual(2);
}
async function open(page,width,height){
  await page.setViewportSize({width,height});
  await page.goto('/real-report.html');
  const map=page.locator('[data-map]');
  await expect(map).toHaveClass(/flow-enabled/,{timeout:60_000});
  await expect(map).not.toHaveClass(/flow-initializing/,{timeout:60_000});
  await settle(map);
  return map;
}
const crumbs=page=>page.locator('.reading-map-context a');

for(const [width,height] of [[1440,900],[1280,800]])test(`each breadcrumb segment goes up to its level at ${width}×${height}`,async({page})=>{
  test.setTimeout(120_000);
  const map=await open(page,width,height);
  // Follow the original saved area tree to an area containing actual parts;
  // a recursive area's immediate children may themselves all be areas.
  const {program,chain,parts}=await page.evaluate(()=>{
    const nodes=[...document.querySelectorAll('[data-map-explorer] [data-node]')],byID=new Map(nodes.map(n=>[n.id,n]));
    const areasOf=n=>(n.dataset.children||'').split(/\s+/).filter(id=>byID.get(id)?.dataset.branch==='area');
    const program=nodes.filter(n=>n.dataset.branch==='component').sort((a,b)=>areasOf(b).length-areasOf(a).length)[0];
    function leafArea(id,chain){
      const node=byID.get(id),children=(node.dataset.children||'').split(/\s+/).filter(Boolean);
      const parts=children.filter(child=>!byID.get(child)?.dataset.branch);
      if(node.dataset.branch==='area'&&parts.length)return {chain:[...chain,id],parts};
      for(const child of areasOf(node)){const found=leafArea(child,[...chain,id]);if(found)return found;}
    }
    return {program:program.id,...leafArea(program.id,[])};
  });
  expect(chain.length,'an original area containing parts').toBeGreaterThan(1);
  for(const id of chain){await page.locator(`[data-zoom-into="${id}"]`).press('Enter');await settle(map);}
  const canvas=await page.locator('.flow-root').boundingBox();
  const boxes=await page.locator('.react-flow__node[data-id^="n-"]').evaluateAll((nodes,parts)=>nodes.filter(n=>parts.includes(n.dataset.id)).map(n=>n.getBoundingClientRect().toJSON()),parts);
  const box=boxes.find(b=>b.x>canvas.x&&b.y>canvas.y&&b.x+b.width<canvas.x+canvas.width&&b.y+b.height<canvas.y+canvas.height);
  expect(box,'a whole part stands in the canvas').toBeTruthy();
  await page.mouse.move(box.x+box.width/2,box.y+box.height/2,{steps:8});await page.mouse.click(box.x+box.width/2,box.y+box.height/2);await settle(map);
  const titles=await crumbs(page).allTextContents();
  expect(titles.length,`program, each area and part are each a link: ${titles}`).toBe(chain.length+1);
  await expect(page.locator('.reading-map-context')).toHaveText(titles.join(' / '));
  // Up to the area: it is read and entered.
  await crumbs(page).nth(chain.length-1).click();await settle(map);
  await expect(page.locator('.map-inspector-heading')).toContainText(titles[chain.length-1]);
  await expect(crumbs(page)).toHaveCount(chain.length);
  expect(await map.evaluate(map=>map.dataset.sceneLevel)).toBe(chain.join('/'));
  // Up to the program: read and entered, its areas closed.
  await crumbs(page).first().click();await settle(map);
  await expect(page.locator('.map-inspector-heading')).toContainText(titles[0]);
  await expect(crumbs(page)).toHaveCount(1);
  expect(await map.evaluate(map=>map.dataset.sceneLevel)).toBe(program);
});
