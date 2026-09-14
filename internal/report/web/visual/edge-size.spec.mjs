import {test,expect} from '@playwright/test';

// Measure the actual CSS-scaled SVG and its paint, not only the nominal SVG
// stroke: vector-effect alone does not cancel React Flow's ancestor transform.
test('connection strokes and arrowheads keep their screen size while zooming',async({page},testInfo)=>{
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/?many-external');
  await expect.poll(()=>page.locator('[data-map]').evaluate(map=>map.captureViewport?.())).toBeTruthy();
  const map=page.locator('[data-map]'),entranceRevision=await map.getAttribute('data-camera-revision');
  await page.locator('[data-zoom-into="front"]').click();
  await expect(map).not.toHaveAttribute('data-camera-revision',entranceRevision);
  await expect(page.locator('.flow-location')).toHaveText('Web application');
  await expect.poll(()=>page.locator('[data-map]').evaluate(map=>map.captureViewport().componentsOpen)).toBe(true);
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
      const paths=[...document.querySelectorAll('.flow-edge path')].map(path=>{
        const css=getComputedStyle(path),matrix=path.getScreenCTM(),scale=Math.hypot(matrix.a,matrix.b);
        const active=path.parentElement.classList.contains('flow-edge-active'),casing=path.classList.contains('flow-edge-casing');
        const expected=casing?(active?6:5):(active?2.5:1.5),width=parseFloat(css.strokeWidth)*scale;
        const marker=path.getAttribute('marker-end')&&document.getElementById(active?'flow-arrow-active':'flow-arrow');
        if(active&&!casing&&css.strokeDasharray==='none'){
          const numbers=(path.getAttribute('d').match(/-?\d+(?:\.\d+)?(?:e[+-]?\d+)?/gi)||[]).map(Number);
          for(let i=2;i<numbers.length;i+=2){
            const ax=numbers[i-2]*matrix.a+matrix.e,ay=numbers[i-1]*matrix.d+matrix.f;
            const bx=numbers[i]*matrix.a+matrix.e,by=numbers[i+1]*matrix.d+matrix.f;
            const left=Math.max(Math.min(ax,bx),host.left+24),right=Math.min(Math.max(ax,bx),host.right-24);
            if(Math.abs(ay-by)<.01&&right-left>100&&ay>host.top+24&&ay<host.bottom-24)samples.push({x:Math.round((left+right)/2),y:ay});
          }
        }
        return {expected,width,dash:css.strokeDasharray==='none'?[]:css.strokeDasharray.split(',').map(value=>parseFloat(value)*scale),
          marker:marker?{units:marker.markerUnits.baseVal,width:marker.markerWidth.baseVal.value*width,expected:7*expected}:null};
      });
      return {zoom:document.querySelector('[data-map]').captureViewport().zoom,paths,samples};
      });
      return measured.paths.length>0&&measured.paths.some(path=>path.marker);
    },{message:'The completed camera has committed native route paths and arrowheads'}).toBe(true);
    expect(measured.paths.length).toBeGreaterThan(0);
    expect(measured.paths.some(path=>path.marker)).toBe(true);
    for(const path of measured.paths){
      expect(path.width,'Painted stroke and casing widths stay fixed in screen pixels').toBeCloseTo(path.expected,3);
      if(path.marker){expect(path.marker.units).toBe(2);expect(path.marker.width).toBeCloseTo(path.marker.expected,3);}
      if(path.dash.length){expect(path.dash[0]).toBeCloseTo(7,3);expect(path.dash[1]).toBeCloseTo(5,3);}
    }
    const screenshot=await page.screenshot();
    const painted=await page.evaluate(async({png,samples})=>{
      const image=new Image();image.src='data:image/png;base64,'+png;await image.decode();
      const canvas=document.createElement('canvas');canvas.width=image.width;canvas.height=image.height;
      const context=canvas.getContext('2d');context.drawImage(image,0,0);const pixels=context.getImageData(0,0,canvas.width,canvas.height).data;
      return samples.map(({x,y})=>{let count=0;for(let row=Math.floor(y)-12;row<=Math.ceil(y)+12;row++){
        const offset=(row*canvas.width+x)*4;
        if(Math.abs(pixels[offset]-52)<18&&Math.abs(pixels[offset+1]-68)<18&&Math.abs(pixels[offset+2]-91)<18)count++;
      }return count;});
    },{png:screenshot.toString('base64'),samples:measured.samples});
    expect(painted.some(width=>width>=1&&width<=4),'A visible native active route actually paints a thin stroke').toBe(true);
    measurements.push({step,zoom:measured.zoom,paintedWidths:painted});
    await testInfo.attach(`journey-${measurements.length} — Connection sizes · ${step}`,{body:screenshot,contentType:'image/png'});
  }
  expect(measurements[1].zoom).toBeGreaterThan(measurements[0].zoom);
  expect(errors).toEqual([]);
  await testInfo.attach('Connection size measurements',{body:JSON.stringify(measurements,null,2),contentType:'application/json'});
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
  for(const zoom of [10.72,13.4]){
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
        stroke:[...css.boxShadow.matchAll(/(-?[\d.]+)px/g)].map(match=>Number(match[1]))[3],
        x:Math.round((Math.max(box.left+24,host.left+24)+Math.min(box.right-24,host.right-24))/2),y:box.top};
    });
    expect(measured.border,'No border can be rounded up and then magnified').toBe(0);
    expect(measured.stroke*zoom).toBeCloseTo(2,3);
    expect(measured.radius*zoom).toBeCloseTo(12,3);
    expect(await node.evaluate(node=>({transform:node.style.transform,width:node.style.width,height:node.style.height}))).toEqual(world);
    const screenshot=await page.screenshot();
    const painted=await page.evaluate(async({png,x,y})=>{
      const image=new Image();image.src='data:image/png;base64,'+png;await image.decode();
      const canvas=document.createElement('canvas');canvas.width=image.width;canvas.height=image.height;
      const context=canvas.getContext('2d');context.drawImage(image,0,0);const pixels=context.getImageData(0,0,canvas.width,canvas.height).data;
      let count=0;for(let row=Math.floor(y)-4;row<Math.ceil(y)+30;row++){
        const offset=(row*canvas.width+x)*4;
        if(Math.abs(pixels[offset]-82)<18&&Math.abs(pixels[offset+1]-100)<18&&Math.abs(pixels[offset+2]-125)<18)count++;
      }return count;
    },{png:screenshot.toString('base64'),x:measured.x,y:measured.y});
    expect(painted,'The actual frame edge paints a thin stroke').toBeGreaterThanOrEqual(1);
    expect(painted).toBeLessThanOrEqual(3);
    await testInfo.attach(`journey-${zoom===10.72?1:2} — Area frame at zoom ${zoom}`,{body:screenshot,contentType:'image/png'});
  }
  expect(errors).toEqual([]);
});
