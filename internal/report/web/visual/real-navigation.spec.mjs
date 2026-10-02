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
  // The program holding the most areas, its first area, a part of it in
  // sight, by the report's own nodes.
  const {program,area}=await page.evaluate(()=>{
    const nodes=[...document.querySelectorAll('[data-map-explorer] [data-node]')],byID=new Map(nodes.map(n=>[n.id,n]));
    const areasOf=n=>(n.dataset.children||'').split(/\s+/).filter(id=>byID.get(id)?.dataset.branch==='area');
    const program=nodes.filter(n=>n.dataset.branch==='component').sort((a,b)=>areasOf(b).length-areasOf(a).length)[0];
    return {program:program.id,area:areasOf(program)[0]};
  });
  await map.evaluate((map,id)=>map.sceneEnter(id),area);await settle(map);
  const canvas=await page.locator('.flow-root').boundingBox();
  const boxes=await page.locator('.react-flow__node[data-id^="n-"]').evaluateAll(nodes=>nodes.map(n=>n.getBoundingClientRect().toJSON()));
  const box=boxes.find(b=>b.x>canvas.x&&b.y>canvas.y&&b.x+b.width<canvas.x+canvas.width&&b.y+b.height<canvas.y+canvas.height);
  expect(box,'a whole part stands in the canvas').toBeTruthy();
  await page.mouse.move(box.x+box.width/2,box.y+box.height/2,{steps:8});await page.mouse.click(box.x+box.width/2,box.y+box.height/2);await settle(map);
  const titles=await crumbs(page).allTextContents();
  expect(titles.length,`program, area and part are each a link: ${titles}`).toBe(3);
  await expect(page.locator('.reading-map-context')).toHaveText(titles.join(' / '));
  // Up to the area: it is read and entered.
  await crumbs(page).nth(1).click();await settle(map);
  await expect(page.locator('.map-inspector-heading')).toContainText(titles[1]);
  await expect(crumbs(page)).toHaveCount(2);
  expect(await map.evaluate(map=>map.dataset.sceneLevel)).toBe(`${program}/${area}`);
  // Up to the program: read and entered, its areas closed.
  await crumbs(page).first().click();await settle(map);
  await expect(page.locator('.map-inspector-heading')).toContainText(titles[0]);
  await expect(crumbs(page)).toHaveCount(1);
  expect(await map.evaluate(map=>map.dataset.sceneLevel)).toBe(program);
});
