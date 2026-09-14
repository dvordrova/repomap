import {test,expect} from '@playwright/test';

// Check actual browser paints, including the non-selected neighbours. Pale
// decoration is allowed; the frame/route carrying meaning must stay visible.
test('card roles remain distinct without colour and important lines meet contrast',async({page},testInfo)=>{
  await page.goto('/');
  const map=page.locator('[data-map]');await expect(map).toHaveAttribute('data-fixture-ready','true');
  await page.locator('[data-zoom-into="backend"]').click();
  await expect.poll(()=>map.evaluate(map=>map.captureViewport().openComponents.includes('backend'))).toBe(true);
  await page.locator('[data-summary-area="requests"] strong,[data-frame-title="requests"]>strong').click();
  await expect(page.locator('.flow-role-core').first()).toBeVisible();
  await expect(page.locator('.flow-role-triggers').first()).toBeVisible();
  await expect(page.locator('.flow-part>.flow-kind').filter({hasText:/^(Core|Entrypoints)$/})).toHaveCount(0);
  const colours=await page.evaluate(()=>{
    const rgb=value=>value.match(/[\d.]+/g).slice(0,3).map(Number);
    const lum=c=>c.map(v=>v/255).map(v=>v<=.04045?v/12.92:((v+.055)/1.055)**2.4).reduce((a,v,i)=>a+v*[.2126,.7152,.0722][i],0);
    const ratio=(a,b)=>{a=lum(rgb(a));b=lum(rgb(b));return (Math.max(a,b)+.05)/(Math.min(a,b)+.05);};
    const parts=[...document.querySelectorAll('.react-flow__node>.flow-part')].map(el=>{
      const style=getComputedStyle(el),title=getComputedStyle(el.querySelector('strong'));
      const parent=el.closest('.react-flow__node').dataset.id;
      return {id:parent,border:ratio(style.borderColor,style.backgroundColor),text:ratio(title.color,style.backgroundColor)};
    });
    const edges=[...document.querySelectorAll('.flow-edge path:not(.flow-edge-casing)')].map(el=>ratio(getComputedStyle(el).stroke,'rgb(248,250,252)'));
    const frames=[...document.querySelectorAll('.flow-area')].map(el=>{const s=getComputedStyle(el);return ratio(s.boxShadow.match(/rgba?\([^)]+\)/)[0],s.backgroundColor);});
    const symbols=[...document.querySelectorAll('.flow-role-symbol')].map(el=>getComputedStyle(el).clipPath);
    return {parts,edges,frames,symbols:[...new Set(symbols)]};
  });
  expect(colours.parts.length).toBeGreaterThan(5);expect(colours.edges.length).toBeGreaterThan(0);
  for(const card of colours.parts){expect(card.border,card.id+' boundary').toBeGreaterThanOrEqual(3);expect(card.text,card.id+' text').toBeGreaterThanOrEqual(4.5);}
  for(const value of colours.edges)expect(value,'Even neighbouring arrows remain legible').toBeGreaterThanOrEqual(3);
  for(const value of colours.frames)expect(value,'Participant and group frames stay distinguishable').toBeGreaterThanOrEqual(3);
  expect(colours.symbols).toHaveLength(2);
  await testInfo.attach('contrast-measurements',{body:JSON.stringify(colours,null,2),contentType:'application/json'});
  await testInfo.attach('journey-01 — Entry and internal parts without repeated category captions',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  await page.addStyleTag({content:'.map-workspace{filter:grayscale(1)}'});
  await testInfo.attach('journey-02 — The same roles and connections in grayscale',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
});
