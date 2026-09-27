import {test,expect} from '@playwright/test';

// Measure the actual CSS-scaled SVG and its paint, not only the nominal SVG
// stroke: vector-effect alone does not cancel React Flow's ancestor transform.
// Every head is an ordinary one, seven 1.5px strokes: emphasis darkens and
// thickens its line, and at seven of its 2.5px strokes a head stood 17.5px.
const head=7*1.5;
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
      const paths=[...document.querySelectorAll('.flow-edge path')].map(path=>{
        const css=getComputedStyle(path),matrix=path.getScreenCTM(),scale=Math.hypot(matrix.a,matrix.b);
        const active=path.parentElement.classList.contains('flow-edge-active'),casing=path.classList.contains('flow-edge-casing');
        const expected=casing?(active?6:5):(active?2.5:1.5),width=parseFloat(css.strokeWidth)*scale;
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
        return {expected,width,dash:css.strokeDasharray==='none'?[]:css.strokeDasharray.split(',').map(value=>parseFloat(value)*scale),
          active,marker:marker?{units:marker.markerUnits.baseVal,width:marker.markerWidth.baseVal.value*width,height:marker.markerHeight.baseVal.value*width}:null};
      });
      return {zoom:document.querySelector('[data-map]').captureViewport().zoom,paths,samples};
      });
      return measured.paths.some(path=>path.marker&&path.active)&&measured.paths.some(path=>path.marker&&!path.active);
    },{message:'The completed camera has committed native route paths, ordinary and emphasised arrowheads'}).toBe(true);
    for(const path of measured.paths){
      expect(path.width,'Painted stroke and casing widths stay fixed in screen pixels').toBeCloseTo(path.expected,3);
      if(path.marker){
        expect(path.marker.units).toBe(2);
        expect(path.marker.width,`${path.active?'An emphasised':'An ordinary'} head is the ordinary size`).toBeCloseTo(head,3);
        expect(path.marker.height).toBeCloseTo(head,3);
      }
      if(path.dash.length){expect(path.dash[0]).toBeCloseTo(7,3);expect(path.dash[1]).toBeCloseTo(5,3);}
    }
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
    measurements.push({step,zoom:measured.zoom,paintedWidths:painted});
    await testInfo.attach(`journey-${measurements.length} — Connection sizes · ${step}`,{body:screenshot,contentType:'image/png'});
  }
  expect(measurements[1].zoom).toBeGreaterThan(measurements[0].zoom);
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
  await part.locator('.flow-symbol-head',{hasText:'processJob'}).hover();
  await expect(part.locator('path.flow-symbol-call-hot').first()).toBeAttached();
  const links=await part.evaluate(part=>[...part.querySelectorAll('.flow-part-symbols svg path[marker-end]')].map(path=>{
    const marker=document.getElementById(path.getAttribute('marker-end').match(/#([^)]+)/)[1]),stroke=parseFloat(getComputedStyle(path).strokeWidth);
    return {hot:path.classList.contains('flow-symbol-call-hot'),stroke,head:marker.markerWidth.baseVal.value*stroke,tall:marker.markerHeight.baseVal.value*stroke};
  }));
  expect(links.filter(link=>link.hot).length).toBeGreaterThan(0);
  for(const link of links){
    expect(link.stroke,'an emphasised link keeps its thicker line').toBe(link.hot?2.5:1.5);
    expect(link.head,`${link.hot?'An emphasised':'An ordinary'} link's head is the ordinary size`).toBeCloseTo(head,3);
    expect(link.tall).toBeCloseTo(head,3);
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
    await testInfo.attach(`journey-${zoom===readableZoom?1:2} — Area frame at zoom ${zoom}`,{body:screenshot,contentType:'image/png'});
  }
  expect(errors).toEqual([]);
});
