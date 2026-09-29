import {test,expect} from '@playwright/test';

// Check actual browser paints, including the non-selected neighbours. Pale
// decoration is allowed; the frame/route carrying meaning must stay visible.
test('card roles remain distinct without colour and important lines meet contrast',async({page},testInfo)=>{
  await page.goto('/');
  const map=page.locator('[data-map]');await expect(map).toHaveAttribute('data-fixture-ready','true');
  await page.locator('[data-zoom-into="backend"]').click();
  await expect.poll(()=>map.evaluate(map=>map.captureViewport().openComponents.includes('backend'))).toBe(true);
  // A click reads without moving the camera: the area's magnifier enters it.
  const requests=page.locator('[data-zoom-into="requests"]');
  if(await requests.count())await requests.click();
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
    const symbols=[...document.querySelectorAll('.flow-role-symbol')].map(el=>{const style=getComputedStyle(el);return style.clipPath+' '+style.backgroundImage;});
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

// Cut from a 14px square, the entry arrow's shaft ran along the card's border
// in the border's colour, so only a small head read. It is a wider arrow with
// a halo in the card's colour: the border stops short of it.
test('the entry mark stands clear of the border it sits on and the legend draws the same arrow',async({page},testInfo)=>{
  await page.goto('/');
  const map=page.locator('[data-map]');await expect(map).toHaveAttribute('data-fixture-ready','true');
  await page.locator('[data-zoom-into="backend"]').click();
  await expect.poll(()=>map.evaluate(map=>map.captureViewport().openComponents.includes('backend'))).toBe(true);
  const requests=page.locator('[data-zoom-into="requests"]');
  if(await requests.count())await requests.click();
  const mark=page.locator('.react-flow__node[data-id="routes"] .flow-role-triggers');
  await expect(mark).toBeVisible();
  await page.mouse.move(1430,890);
  // Four screen pixels to a card pixel, the mark in the middle of the canvas.
  const host=await page.locator('.flow-root').boundingBox(),v=await map.evaluate(map=>map.captureViewport());
  const box=await mark.boundingBox(),unit=box.height/await mark.evaluate(el=>el.offsetHeight),zoom=v.zoom*4/unit;
  const world={x:(box.x+box.width/2-host.x-v.x)/v.zoom,y:(box.y+box.height/2-host.y-v.y)/v.zoom};
  await map.evaluate((map,viewport)=>map.restoreReadingState({scope:'',viewport}),{...v,zoom,x:host.width/2-world.x*zoom,y:host.height/2-world.y*zoom,fit:false});
  await expect.poll(()=>map.evaluate(map=>map.captureViewport().zoom)).toBeCloseTo(zoom,5);
  const at=await mark.boundingBox();
  // The mark is centred on the border's inner edge: the 1.5px border's middle
  // is 0.75 card pixels above it.
  const row=Math.round(at.y+at.height/2-.75*4);
  const screenshot=await page.screenshot();
  const paint=await page.evaluate(async({png,points})=>{
    const image=new Image();image.src='data:image/png;base64,'+png;await image.decode();
    const canvas=document.createElement('canvas');canvas.width=image.width;canvas.height=image.height;
    const context=canvas.getContext('2d');context.drawImage(image,0,0);
    return points.map(([x,y])=>[...context.getImageData(x,y,1,1).data.slice(0,3)]);
  },{png:screenshot.toString('base64'),points:[[Math.round(at.x-16),row],[Math.round(at.x+2),row]]});
  await testInfo.attach('journey-01 — The entry mark on its card\'s border, four times closer',{body:await page.screenshot({clip:{x:at.x-60,y:at.y-30,width:at.width+120,height:at.height+60}}),contentType:'image/png'});
  const entry=colour=>[38,114,103].every((value,i)=>Math.abs(colour[i]-value)<24);
  expect(entry(paint[0]),`the card's border runs up to the mark: ${paint[0]}`).toBe(true);
  expect(entry(paint[1]),`the border stops short of the arrow: ${paint[1]}`).toBe(false);
  // The report's legend line, as it builds it: the same arrow.
  const same=await page.evaluate(()=>{
    const key=document.createElement('span');key.className='flow-color-key';key.innerHTML='<span class="entry"><i></i>Entrypoints</span>';
    document.querySelector('[data-map]').append(key);
    const glyph=el=>{const style=getComputedStyle(el),box=el.getBoundingClientRect();return {image:style.backgroundImage,clip:style.clipPath,shape:Math.round(box.width/box.height*100)};};
    return {legend:glyph(key.querySelector('i')),map:glyph(document.querySelector('.react-flow__node[data-id="routes"] .flow-role-triggers'))};
  });
  expect(same.legend).toEqual(same.map);
});
