import {test,expect} from '@playwright/test';

// Measure the actual CSS-scaled SVG and its paint, not only the nominal SVG
// stroke: vector-effect alone does not cancel React Flow's ancestor transform.
// Every head is an ordinary one: emphasis darkens and thickens its line and
// leaves its head the size of every other.
const rounded=value=>Math.round(value*1000)/1000;
test('connection strokes and arrowheads keep their screen size while zooming',async({page},testInfo)=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/?many-external');
  await expect.poll(()=>page.locator('[data-map]').evaluate(map=>map.captureViewport?.())).toBeTruthy();
  const map=page.locator('[data-map]'),entranceRevision=await map.getAttribute('data-camera-revision');
  await page.locator('[data-zoom-into="front"]').click();
  await expect(map).not.toHaveAttribute('data-camera-revision',entranceRevision);
  await expect(page.locator('.flow-location')).toHaveText('Web application');
  await expect.poll(()=>page.locator('[data-map]').evaluate(map=>map.captureViewport().componentsOpen)).toBe(true);
  // The component entrance now has boundary-only outer arrows. Inspect the
  // real internal arrow in an opened area rather than a removed continuation.
  await expect.poll(()=>map.evaluate(map=>map.captureViewport().openComponents.includes('front'))).toBe(true);
  // Its zoom mark enters the area and chooses it, so its arrows are dark.
  await page.locator('[data-zoom-into="editing"]').click();
  await expect(page.locator('.react-flow__node[data-id="editor"]')).toBeVisible();
  await page.mouse.move(1430,890);
  const measurements=[];
  for(const step of ['component entrance','closer']){
    if(step==='closer'){
      const zoom=await map.evaluate(map=>map.captureViewport().zoom),revision=await map.getAttribute('data-camera-revision');
      await page.getByRole('button',{name:'Zoom in',exact:true}).click();
      await expect(map).not.toHaveAttribute('data-camera-revision',revision);
      await expect.poll(()=>page.locator('[data-map]').evaluate(map=>map.captureViewport().zoom)).toBeCloseTo(zoom*1.25,5);
    }
    // Camera completion and semantic detail precede React Flow's edge commit.
    // Leaving hover also redraws the routes; wait for that real SVG, not a timer.
    let measured;
    await expect.poll(async()=>{
      measured=await page.evaluate(()=>{
      const host=document.querySelector('.flow-root').getBoundingClientRect(),samples=[];
      const paths=[...document.querySelectorAll('.flow-edge path:not(.flow-edge-hit)')].map(path=>{
        const css=getComputedStyle(path),matrix=path.getScreenCTM(),scale=Math.hypot(matrix.a,matrix.b);
        const active=path.parentElement.classList.contains('flow-edge-active'),casing=path.classList.contains('flow-edge-casing');
        const kind=(active?'active':'ordinary')+(casing?' casing':''),width=parseFloat(css.strokeWidth)*scale;
        const ref=path.getAttribute('marker-end')||path.getAttribute('marker-start'),marker=ref&&document.getElementById(ref.match(/#([^)]+)/)[1]);
        // Straight solid runs in view, crossed at their middle: a level run
        // down its column, an upright one along its row, in its own paint.
        if(!casing&&css.strokeDasharray==='none'){
          const numbers=(path.getAttribute('d').match(/-?\d+(?:\.\d+)?(?:e[+-]?\d+)?/gi)||[]).map(Number),colour=css.stroke.match(/\d+/g).slice(0,3).map(Number);
          for(let i=2;i<numbers.length;i+=2){
            const ax=numbers[i-2]*matrix.a+matrix.e,ay=numbers[i-1]*matrix.d+matrix.f;
            const bx=numbers[i]*matrix.a+matrix.e,by=numbers[i+1]*matrix.d+matrix.f;
            const left=Math.max(Math.min(ax,bx),host.left+24),right=Math.min(Math.max(ax,bx),host.right-24);
            const top=Math.max(Math.min(ay,by),host.top+24),bottom=Math.min(Math.max(ay,by),host.bottom-24);
            if(Math.abs(ay-by)<.01&&right-left>32&&ay>host.top+24&&ay<host.bottom-24)samples.push({x:Math.round((left+right)/2),y:ay,level:true,colour});
            if(Math.abs(ax-bx)<.01&&bottom-top>32&&ax>host.left+24&&ax<host.right-24)samples.push({x:ax,y:Math.round((top+bottom)/2),level:false,colour});
          }
        }
        return {kind,width,dash:css.strokeDasharray==='none'?[]:css.strokeDasharray.split(',').map(value=>parseFloat(value)*scale),
          active,marker:marker?{units:marker.markerUnits.baseVal,width:marker.markerWidth.baseVal.value*width,height:marker.markerHeight.baseVal.value*width}:null};
      });
      return {zoom:document.querySelector('[data-map]').captureViewport().zoom,paths,samples};
      });
      return measured.paths.some(path=>path.marker&&path.active)&&measured.paths.some(path=>path.marker&&!path.active);
    },{message:'The completed camera has committed native route paths, ordinary and emphasised arrowheads'}).toBe(true);
    // Screen sizes by kind of path: one stroke width per kind, one head
    // size for every arrow, emphasised or not, one dash.
    const sizes={widths:{},heads:new Set(),dashes:new Set()};
    for(const path of measured.paths){
      (sizes.widths[path.kind]??=new Set()).add(rounded(path.width));
      if(path.marker)sizes.heads.add(rounded(path.marker.width)+'×'+rounded(path.marker.height));
      if(path.dash.length)sizes.dashes.add(path.dash.map(rounded).join(' '));
    }
    const said={widths:Object.fromEntries(Object.entries(sizes.widths).map(([kind,set])=>[kind,[...set]])),heads:[...sizes.heads],dashes:[...sizes.dashes]};
    for(const [kind,widths] of Object.entries(said.widths))expect(widths,`${kind} strokes share one width`).toHaveLength(1);
    expect(said.widths.active[0],'emphasis thickens the line').toBeGreaterThan(said.widths.ordinary[0]);
    expect(said.heads,'an emphasised head is the ordinary size').toHaveLength(1);
    expect(said.dashes.length).toBeLessThanOrEqual(1);
    const screenshot=await page.screenshot();
    const painted=await page.evaluate(async({png,samples})=>{
      const image=new Image();image.src='data:image/png;base64,'+png;await image.decode();
      const canvas=document.createElement('canvas');canvas.width=image.width;canvas.height=image.height;
      const context=canvas.getContext('2d');context.drawImage(image,0,0);const pixels=context.getImageData(0,0,canvas.width,canvas.height).data;
      const near=(offset,colour)=>colour.every((value,i)=>Math.abs(pixels[offset+i]-value)<18);
      return samples.map(({x,y,level,colour})=>{let count=0;for(let step=-12;step<=12;step++){
        const offset=level?((Math.round(y)+step)*canvas.width+x)*4:(Math.round(y)*canvas.width+Math.round(x)+step)*4;
        if(near(offset,colour))count++;
      }return count;});
    },{png:screenshot.toString('base64'),samples:measured.samples});
    expect(painted.some(width=>width>=1&&width<=4),'A visible native route actually paints a thin stroke').toBe(true);
    measurements.push({step,zoom:measured.zoom,sizes:said,paintedWidths:painted});
    await testInfo.attach(`journey-${measurements.length} — Connection sizes · ${step}`,{body:screenshot,contentType:'image/png'});
  }
  expect(measurements[1].zoom).toBeGreaterThan(measurements[0].zoom);
  // Painted stroke, casing, head and dash sizes stay fixed in screen pixels.
  const stroke=m=>Object.fromEntries(Object.entries(m.sizes.widths).filter(([kind])=>kind in measurements[1].sizes.widths&&kind in measurements[0].sizes.widths));
  expect(stroke(measurements[1]),'stroke widths stay fixed in screen pixels while zooming').toEqual(stroke(measurements[0]));
  expect(measurements[1].sizes.heads,'arrowheads keep their screen size').toEqual(measurements[0].sizes.heads);
  if(measurements[0].sizes.dashes.length&&measurements[1].sizes.dashes.length)expect(measurements[1].sizes.dashes).toEqual(measurements[0].sizes.dashes);
  expect(errors).toEqual([]);
  await testInfo.attach('Connection size measurements',{body:JSON.stringify(measurements,null,2),contentType:'application/json'});
});

// Zoomed into a part, a link between its declarations darkens and thickens
// under the pointer; its head stays the size of every other link's.
test('an emphasised link between declarations keeps an ordinary head',async({page},testInfo)=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/?symbols');
  const map=page.locator('[data-map]');await expect(map).toHaveAttribute('data-fixture-ready','true');
  const revision=await map.getAttribute('data-camera-revision');
  await map.evaluate(map=>map.focusNode('worker'));
  await expect(map).not.toHaveAttribute('data-camera-revision',revision);
  await page.locator('.react-flow__node[data-id="worker"] .flow-part-zoom').click();
  const part=page.locator('.react-flow__node[data-id="worker"]');
  await expect(part.locator('.flow-part-deep')).toBeVisible();
  // Hover sleeps while the camera moves: point at the tile once it has
  // stopped, with a real movement, as a reader does.
  let previous='',stable=0;
  await expect.poll(async()=>{const v=JSON.stringify(await map.evaluate(map=>map.captureViewport()));stable=v===previous?stable+1:0;previous=v;return stable;},{intervals:[100]}).toBeGreaterThanOrEqual(2);
  const tile=await part.locator('.flow-symbol-head',{hasText:'processJob'}).boundingBox();
  await page.mouse.move(tile.x+tile.width/2,tile.y+tile.height/2+40);
  await page.mouse.move(tile.x+tile.width/2,tile.y+tile.height/2,{steps:8});
  await expect(part.locator('path.flow-symbol-call-hot').first()).toBeAttached();
  const links=await part.evaluate(part=>[...part.querySelectorAll('.flow-part-symbols svg path[marker-end]')].map(path=>{
    const marker=document.getElementById(path.getAttribute('marker-end').match(/#([^)]+)/)[1]),stroke=parseFloat(getComputedStyle(path).strokeWidth);
    return {hot:path.classList.contains('flow-symbol-call-hot'),stroke,head:marker.markerWidth.baseVal.value*stroke,tall:marker.markerHeight.baseVal.value*stroke};
  }));
  expect(links.filter(link=>link.hot).length).toBeGreaterThan(0);
  const ordinary=links.find(link=>!link.hot);
  for(const link of links){
    if(link.hot&&ordinary)expect(link.stroke,'an emphasised link keeps its thicker line').toBeGreaterThan(ordinary.stroke);
    expect(link.head,`${link.hot?'An emphasised':'An ordinary'} link's head is the ordinary size`).toBeCloseTo(links[0].head,3);
    expect(link.tall).toBeCloseTo(links[0].tall,3);
  }
  await testInfo.attach('journey-01 — An emphasised declaration link',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  expect(errors).toEqual([]);
});

// CSS borders are rounded to a CSS pixel before a large viewport transform.
// The frame must paint a constant inset stroke without changing its world box.
test('area frames keep thin outlines and small corners at close zoom',async({page},testInfo)=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/?many-external');
  const map=page.locator('[data-map]');
  await expect.poll(()=>map.evaluate(map=>map.captureViewport?.())).toBeTruthy();
  const saved=await map.evaluate(map=>map.captureViewport());
  const node=page.locator('.react-flow__node[data-id="tracking"]'),frame=node.locator('>.flow-area');
  const world=await node.evaluate(node=>({transform:node.style.transform,width:node.style.width,height:node.style.height}));
  const readableZoom=await page.locator('.react-flow__node[data-id="status"]>.flow-part').evaluate(el=>1/new DOMMatrixReadOnly(getComputedStyle(el).transform).a);
  const corners=[];
  for(const zoom of [readableZoom,readableZoom*1.25]){
    await map.evaluate((map,{saved,zoom})=>{
      const node=map.querySelector('.react-flow__node[data-id="tracking"]'),position=new DOMMatrixReadOnly(node.style.transform);
      map.restoreReadingState({scope:'tracking',viewport:{...saved,zoom,x:24-position.e*zoom,y:24-position.f*zoom,fit:false,
        componentsOpen:true,openComponents:['front'],detailAreas:['tracking']}});
    },{saved,zoom});
    await expect.poll(()=>map.evaluate(map=>map.captureViewport().zoom)).toBeCloseTo(zoom,5);
    await expect(frame).toBeVisible();
    await expect(frame).not.toHaveClass(/flow-area-summarized/);
    const measured=await frame.evaluate(frame=>{
      const css=getComputedStyle(frame),box=frame.getBoundingClientRect(),host=document.querySelector('.flow-root').getBoundingClientRect();
      return {border:parseFloat(css.borderTopWidth),radius:parseFloat(css.borderTopLeftRadius),
        stroke:[...css.boxShadow.matchAll(/(-?[\d.]+)px/g)].map(match=>Number(match[1]))[3],width:parseFloat(css.getPropertyValue('--flow-frame-width')),
        colour:css.boxShadow.match(/rgba?\(([^)]*)\)/)[1].split(',').slice(0,3).map(Number),
        x:Math.round((Math.max(box.left+24,host.left+24)+Math.min(box.right-24,host.right-24))/2),y:box.top};
    });
    expect(measured.border,'No border can be rounded up and then magnified').toBe(0);
    // The frame paints its screen width at every zoom, never magnified: the
    // width it is given (1.5px, 2.5px for the frame looked at) is the look's.
    expect(measured.stroke*zoom).toBeCloseTo(measured.width,3);
    expect(measured.width).toBeLessThanOrEqual(3);
    corners.push(measured.radius*zoom);
    expect(await node.evaluate(node=>({transform:node.style.transform,width:node.style.width,height:node.style.height}))).toEqual(world);
    const screenshot=await page.screenshot();
    const painted=await page.evaluate(async({png,x,y,colour})=>{
      const image=new Image();image.src='data:image/png;base64,'+png;await image.decode();
      const canvas=document.createElement('canvas');canvas.width=image.width;canvas.height=image.height;
      const context=canvas.getContext('2d');context.drawImage(image,0,0);const pixels=context.getImageData(0,0,canvas.width,canvas.height).data;
      let count=0;for(let row=Math.floor(y)-4;row<Math.ceil(y)+30;row++){
        const offset=(row*canvas.width+x)*4;
        if(Math.abs(pixels[offset]-colour[0])<18&&Math.abs(pixels[offset+1]-colour[1])<18&&Math.abs(pixels[offset+2]-colour[2])<18)count++;
      }return count;
    },{png:screenshot.toString('base64'),x:measured.x,y:measured.y,colour:measured.colour});
    expect(painted,'The actual frame edge paints a thin stroke').toBeGreaterThanOrEqual(1);
    expect(painted).toBeLessThanOrEqual(3);
    await testInfo.attach(`journey-${zoom===readableZoom?1:2} — Area frame at zoom ${zoom}`,{body:screenshot,contentType:'image/png'});
  }
  expect(corners[1],'its corners keep their screen size').toBeCloseTo(corners[0],3);
  expect(errors).toEqual([]);
});
