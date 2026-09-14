import {test,expect} from '@playwright/test';
import {records,manyExternalInventory} from './two-systems-five-externals.mjs';

const roots=records.filter(n=>['component','communication','inputs'].includes(n.branch));
const worldGeometry=page=>page.locator('.react-flow__node').evaluateAll(nodes=>nodes.map(n=>[
  n.dataset.id,n.style.transform,n.style.width,n.style.height,
]));

// Text ranges measure the actual heading, not its potentially much wider block.
const textBounds=locator=>locator.evaluate(el=>{
  const range=document.createRange();range.selectNodeContents(el);
  const box=range.getBoundingClientRect();
  return {x:box.x,y:box.y,width:box.width,height:box.height};
});
const textRects=locator=>locator.evaluate(el=>{
  const range=document.createRange();range.selectNodeContents(el);
  return [...range.getClientRects()].map(r=>({x:r.x,y:r.y,width:r.width,height:r.height}));
});
const overlaps=(a,b)=>Math.min(a.x+a.width,b.x+b.width)-Math.max(a.x,b.x)>.01&&
  Math.min(a.y+a.height,b.y+b.height)-Math.max(a.y,b.y)>.01;
const inside=(box,stage)=>box&&box.width>0&&box.height>0&&
  box.x>=stage.x-.5&&box.y>=stage.y-.5&&
  box.x+box.width<=stage.x+stage.width+.5&&box.y+box.height<=stage.y+stage.height+.5;
async function assertInsideCanvas(page,locator,label,{text=false,container='.flow-root'}={}){
  await expect(locator,label).toBeVisible();
  const box=await(text?textBounds(locator):locator.boundingBox());
  const stage=await page.locator(container).boundingBox();
  expect(inside(box,stage),`${label} is wholly inside the canvas: ${JSON.stringify({box,stage})}`).toBe(true);
}

const componentHeading=(page,id,title)=>page.locator(`[data-component-overview="${id}"] .flow-component-overview-heading>strong`)
  .or(page.locator('.flow-component-title>strong').filter({hasText:new RegExp(`^${title}$`)}));
const areaHeading=(page,id,title)=>page.locator(`[data-summary-area="${id}"] .flow-overview-card>strong`)
  .or(page.locator('.flow-area-title>strong').filter({hasText:new RegExp(`^${title}$`)}));

async function showWholeMap(page){
  await page.getByRole('button',{name:'Show whole map',exact:true}).click();
  // Interrupting an earlier entrance can emit its move-end before this new
  // camera arrives. Wait for the requested, settled overview itself.
  let previous,stable=0;
  await expect.poll(async()=>{
    const camera=await page.locator('[data-map]').evaluate(map=>map.captureViewport());
    const current=JSON.stringify(camera);
    stable=camera?.fit&&camera.componentsOpen===false&&current===previous?stable+1:0;
    previous=current;return stable;
  },{message:'Whole-map camera settles before measuring its labels',intervals:[100]}).toBeGreaterThanOrEqual(2);
  await expect(page.locator('.flow-location')).toHaveText('System map');
}

async function openFixture(page,url='/',{rootCount=roots.length,startupTimeout=10000}={}){
  const errors=[];
  page.on('pageerror',error=>errors.push(error.message));
  await page.goto(url);
  await expect(page.locator('[data-fixture-ready]')).toHaveAttribute('data-fixture-ready','true',{timeout:startupTimeout});
  await expect.poll(()=>page.locator('[data-map]').evaluate(map=>map.captureViewport())).not.toBeNull();
  await expect(page.locator('[data-component-overview]')).toHaveCount(rootCount);
  await expect(page.locator('.flow-location')).toHaveText('System map');
  return errors;
}

async function assertOverviewReadable(page,{allowInventoryScroll=false,participants=records}={}){
  const stage=await page.locator('.flow-root').boundingBox();
  for(const item of participants.filter(n=>['component','communication','inputs'].includes(n.branch))){
    const label=page.locator(`[data-component-overview="${item.id}"] .flow-component-overview-heading>strong`);
    await expect(label).toHaveText(item.title);
    const box=await label.boundingBox();
    expect(box,`${item.title} is visible`).not.toBeNull();
    expect(box.x).toBeGreaterThanOrEqual(stage.x);
    expect(box.y).toBeGreaterThanOrEqual(stage.y);
    expect(box.x+box.width).toBeLessThanOrEqual(stage.x+stage.width);
    expect(box.y+box.height).toBeLessThanOrEqual(stage.y+stage.height);
    const frame=await page.locator(`.react-flow__node[data-id="${item.id}"]`).boundingBox();
    expect(box.y+box.height,`${item.title} fits its frame`).toBeLessThanOrEqual(frame.y+frame.height+1);
    const overflow=await label.evaluate(el=>({x:el.scrollWidth>el.clientWidth+1,y:el.scrollHeight>el.clientHeight+1}));
    expect(overflow,`${item.title} is not clipped`).toEqual({x:false,y:false});
    const splitWords=await label.evaluate(el=>{
      const text=el.firstChild,range=document.createRange();
      return [...text.textContent.matchAll(/\S+/g)].filter(word=>{
        range.setStart(text,word.index);range.setEnd(text,word.index+word[0].length);
        return range.getClientRects().length>1;
      }).map(word=>word[0]);
    });
    expect(splitWords,`${item.title} keeps its words readable`).toEqual([]);
    const zoomButton=page.getByRole('button',{name:`Zoom into ${item.branch==='inputs'?'Inputs · ':''}${item.title}`,exact:true});
    await expect(zoomButton).toBeVisible();
    const zoomBox=await zoomButton.boundingBox(),textBox=await textBounds(label);
    expect(inside(textBox,frame),`${item.title}'s full text stays inside its own frame`).toBe(true);
    expect(inside(zoomBox,frame),`${item.title}'s zoom control stays inside its own frame`).toBe(true);
    // Compare rendered lines, not the empty corners of their union. Fractional
    // inverse transforms can put two touching edges 0.00003px apart.
    const overlap=(await textRects(label)).some(line=>overlaps(line,zoomBox));
    expect(overlap,`${item.title} is not covered by its zoom control`).toBe(false);
    if(item.branch==='component'&&!allowInventoryScroll){
      const list=page.locator(`[data-component-overview="${item.id}"] .flow-component-areas`);
      expect(await list.evaluate(el=>el.scrollHeight<=el.clientHeight+1),`${item.title}'s short area list is fully visible`).toBe(true);
    }
    for(const child of participants.filter(n=>!allowInventoryScroll&&n.branch==='area'&&item.children.includes(n.id))){
      const area=page.locator(`[data-component-overview="${item.id}"] [data-overview-area="${child.id}"]`);
      await expect(area).toBeVisible();
      const bounds=await area.boundingBox();
      expect(bounds.y+bounds.height,`${child.title} is visible inside ${item.title}`).toBeLessThanOrEqual(frame.y+frame.height+1);
    }
  }
  for(const collection of participants.filter(n=>n.branch==='inputs')){
    const label=page.locator(`[data-component-overview="${collection.id}"]`);
    const frame=await page.locator(`.react-flow__node[data-id="${collection.id}"]`).boundingBox();
    const zoomBox=await page.locator(`[data-zoom-into="${collection.id}"]`).boundingBox();
    const groups=label.locator('[data-input-group-kind]');
    expect(await groups.count(),'The compact collection lists its existing input types').toBeGreaterThan(0);
    for(const group of await groups.all()){
      await assertInsideCanvas(page,group,'The input type is visible on the whole map',{text:true});
      expect(inside(await textBounds(group),frame),'The input type fits its own collection').toBe(true);
      expect((await textRects(group)).some(line=>overlaps(line,zoomBox)),'The input type is not covered by its zoom control').toBe(false);
    }
    for(const component of participants.filter(n=>n.branch==='component')){
      const componentBox=await page.locator(`.react-flow__node[data-id="${component.id}"]`).boundingBox();
      const overlap=Math.min(frame.x+frame.width,componentBox.x+componentBox.width)>Math.max(frame.x,componentBox.x)&&
        Math.min(frame.y+frame.height,componentBox.y+componentBox.height)>Math.max(frame.y,componentBox.y);
      expect(overlap,`${collection.id} is outside ${component.title}`).toBe(false);
    }
    for(const id of collection.children){
      await expect(page.locator(`.react-flow__node[data-id="${id}"]`),'Individual inputs stay inside their closed collection').toBeHidden();
      await expect(page.locator(`[data-zoom-into="${id}"]`),'An input opens its path, not another hidden container').toHaveCount(0);
    }
  }
}

test('input collections stay outside both systems and reveal their saved implementation paths',async({page},testInfo)=>{
  const errors=await openFixture(page);
  const geometry=await worldGeometry(page);
  await assertOverviewReadable(page);
  await testInfo.attach('journey-01 — One input collection outside each system',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  for(const [id,owner] of [['submit','submission'],['create','routes'],['consume','worker']]){
    const edge=await page.locator('[data-map]').evaluate((map,{id,owner})=>map.visibleEdges.find(edge=>edge.from===id&&edge.to===owner),{id,owner});
    expect(edge,`${id} retains its exact implementation relation`).toBeTruthy();
    const collection=records.find(n=>n.branch==='inputs'&&n.children.includes(id));
    await page.locator(`[data-zoom-into="${collection.id}"]`).click();
    const input=page.locator(`[data-input-id="${id}"]`);
    await assertInsideCanvas(page,input.locator('[data-input-name]'),'The collection opens on a named input',{text:true});
    await expect.poll(()=>input.locator('[data-input-name]').evaluate(el=>parseFloat(getComputedStyle(el).fontSize)*el.getBoundingClientRect().width/el.offsetWidth),{message:'Input names are readable after entering their collection'}).toBeGreaterThanOrEqual(14);
    await input.click();
    await expect(page.locator('[data-reading-title]')).toHaveText(records.find(n=>n.id===id).title);
    await expect(page.locator(`[data-edge-ids~="${edge.id}"]`).first()).toHaveClass(/flow-edge-active/);
    await testInfo.attach(`journey-0${['submit','create','consume'].indexOf(id)+2} — Open ${records.find(n=>n.id===id).title}`,{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
    await showWholeMap(page);
    await assertOverviewReadable(page);
    expect(await worldGeometry(page),'Input selection and return preserve the placed world').toEqual(geometry);
  }
  expect(errors).toEqual([]);
});

test('overview keeps two systems and five external participants readable',async({page})=>{
  const errors=await openFixture(page);
  await assertOverviewReadable(page);
  await expect(page.locator('.map-workspace')).toHaveScreenshot('overview.png');
  const before=await page.locator('.map-stage').boundingBox();
  await page.getByRole('button',{name:'Zoom into Web application',exact:true}).hover();
  expect(await page.locator('.map-stage').boundingBox()).toEqual(before);
  await page.getByRole('heading',{name:'Two systems · five external participants'}).hover();
  await expect(page.locator('.map-workspace')).toHaveScreenshot('overview.png');
  expect(errors).toEqual([]);
});

test('dense internal inventory keeps whole-map headings and zoom controls readable',async({page},testInfo)=>{
  // This journey includes initial compound layout, screenshots, scrolling both
  // complete inventories, opening a part and returning. Keep the same total
  // budget as the two-level zoom journey; individual assertions stay bounded.
  test.setTimeout(60000);
  await page.addInitScript(()=>{
    const original=CanvasRenderingContext2D.prototype.measureText;
    window.panTextMeasurements=0;
    CanvasRenderingContext2D.prototype.measureText=function(...args){window.panTextMeasurements++;return original.apply(this,args);};
  });
  const errors=await openFixture(page,'/?dense',{startupTimeout:30000});
  const workspace=page.locator('.map-workspace');
  const geometry=await worldGeometry(page);
  await testInfo.attach('journey-01 — Dense inventory: default overview',{body:await workspace.screenshot(),contentType:'image/png'});
  await assertOverviewReadable(page,{allowInventoryScroll:true});
  const buttons=[];
  for(const item of roots){
    const button=page.getByRole('button',{name:`Zoom into ${item.branch==='inputs'?'Inputs · ':''}${item.title}`,exact:true});
    const bounds=await button.boundingBox();
    const frame=await page.locator(`.react-flow__node[data-id="${item.id}"]`).boundingBox();
    expect(inside(bounds,frame),`${item.title} zoom control fits its frame`).toBe(true);
    for(const other of buttons){
      const overlap=Math.min(bounds.x+bounds.width,other.x+other.width)>Math.max(bounds.x,other.x)&&
        Math.min(bounds.y+bounds.height,other.y+other.height)>Math.max(bounds.y,other.y);
      expect(overlap,'Zoom controls for different participants do not overlap').toBe(false);
    }
    buttons.push(bounds);
  }
  await expect(workspace).toHaveScreenshot('dense-overview.png');
  const camera=()=>page.locator('.react-flow__viewport').evaluate(el=>{
    const m=new DOMMatrixReadOnly(getComputedStyle(el).transform);return {x:m.e,y:m.f,zoom:m.a};
  });
  const stage=await page.locator('.flow-root').boundingBox();
  await page.mouse.move(stage.x+8,stage.y+stage.height/2);
  const beforePan=await camera();
  await page.evaluate(()=>{window.panTextMeasurements=0;});
  await page.mouse.wheel(0,80);
  await expect.poll(async()=>(await camera()).y,'The ordinary wheel gesture pans the canvas').not.toBe(beforePan.y);
  await page.evaluate(()=>new Promise(requestAnimationFrame));
  expect(await page.evaluate(()=>window.panTextMeasurements),'Panning does not remeasure unchanged text').toBe(0);
  expect((await camera()).zoom,'Panning preserves zoom').toBe(beforePan.zoom);
  expect(await worldGeometry(page),'Panning preserves world geometry').toEqual(geometry);
  await showWholeMap(page);
  await assertOverviewReadable(page,{allowInventoryScroll:true});
  for(const id of ['front','backend']){
    await expect(page.locator(`[data-component-overview="${id}"] [data-overview-area]`)).toHaveCount(42);
    const area=page.locator(`[data-component-overview="${id}"] [data-overview-area="${id}-workflow-40"]`);
    await expect(area).toHaveText('Additional workflow 40');
    await area.scrollIntoViewIfNeeded();
    await assertInsideCanvas(page,area,'The final area stays reachable in its frame',{text:true,container:`.react-flow__node[data-id="${id}"]`});
  }
  await testInfo.attach('journey-02 — Scroll to the final entries',{body:await workspace.screenshot(),contentType:'image/png'});
  await page.locator('[data-overview-area="backend-workflow-40"]').click();
  await expect(page.locator('[data-reading-title]')).toHaveText('Additional workflow 40');
  await assertInsideCanvas(page,page.locator('.react-flow__node[data-id="backend-workflow-40-part"]'),'The final workflow opens its actual part');
  await testInfo.attach('journey-03 — Open the final workflow and its part',{body:await workspace.screenshot(),contentType:'image/png'});
  await showWholeMap(page);
  await assertOverviewReadable(page,{allowInventoryScroll:true});
  expect(await worldGeometry(page),'Dense overview return keeps the same fixed world').toEqual(geometry);
  await testInfo.attach('journey-04 — Return to the same whole map',{body:await workspace.screenshot(),contentType:'image/png'});
  expect(errors).toEqual([]);
});

test('external zoom reveals calls and returns to the same overview',async({page})=>{
  const errors=await openFixture(page);
  const geometry=await worldGeometry(page);
  await page.getByRole('button',{name:'Zoom into Backend API',exact:true}).click();
  await expect(page.locator('[data-reading-title]')).toHaveText('Backend API');
  await expect(page.locator('.flow-location')).toHaveText('Backend API');
  for(const id of ['post','get','download']){
    const call=page.locator(`.react-flow__node[data-id="${id}"]`);
    await expect(call).toBeInViewport();
    await expect.poll(()=>call.locator('strong').evaluate(el=>
      parseFloat(getComputedStyle(el).fontSize)*el.getBoundingClientRect().width/el.offsetWidth
    ),{message:'Zoom makes call text readable'}).toBeGreaterThanOrEqual(14);
  }
  await page.getByRole('heading',{name:'Two systems · five external participants'}).hover();
  await expect(page.locator('.map-workspace')).toHaveScreenshot('external.png');
  await showWholeMap(page);
  await expect(page.locator('.flow-location')).toHaveText('System map');
  await assertOverviewReadable(page);
  await expect(page.locator('.map-workspace')).toHaveScreenshot('overview.png');
  expect(await worldGeometry(page)).toEqual(geometry);
  expect(errors).toEqual([]);
});

for(const inputs of [true,false])test(`seventeen external participants remain readable through entry and zoom out${inputs?'':' without inputs'}`,async({page},testInfo)=>{
  test.setTimeout(90000);
  const prepared=manyExternalInventory({inputs}),rootCount=inputs?21:19;
  await page.addInitScript(()=>{
    window.mapStartup={visibleBeforeReady:0,readyAt:null};
    const inspect=()=>{
      const map=document.querySelector('[data-map]'),flow=map?.querySelector('.react-flow');
      const ready=!!map?.captureViewport?.();
      if(flow&&!ready&&getComputedStyle(flow).opacity!=='0')window.mapStartup.visibleBeforeReady++;
      if(ready){window.mapStartup.readyAt=performance.now();return;}
      requestAnimationFrame(inspect);
    };
    requestAnimationFrame(inspect);
  });
  // The larger real compound layout has its own startup budget; record its
  // actual duration below, separately from the subsequent interaction checks.
  const errors=await openFixture(page,`/?many-external${inputs?'':'&no-inputs'}`,{rootCount,startupTimeout:30000});
  const workspace=page.locator('.map-workspace');
  const geometry=await worldGeometry(page);
  const overviewCamera=await page.locator('[data-map]').evaluate(map=>{const {x,y,zoom}=map.captureViewport();return {x,y,zoom};});
  await testInfo.attach('journey-01 — Two systems and seventeen destinations',{body:await workspace.screenshot(),contentType:'image/png'});
  await assertOverviewReadable(page,{participants:prepared.records});
  expect(await page.evaluate(()=>window.mapStartup.visibleBeforeReady),'Initial layout and camera stay concealed until ready').toBe(0);
  await testInfo.attach('Initial placement timing',{body:JSON.stringify(await page.evaluate(()=>window.mapStartup)),contentType:'application/json'});
  await expect(workspace).toHaveScreenshot(inputs?'many-external-overview.png':'many-external-without-inputs.png');
  if(inputs){
  await page.locator('[data-zoom-into="backend-inputs"]').click();
  for(const id of ['create','consume']){
    const name=page.locator(`[data-input-id="${id}"] [data-input-name]`);
    await assertInsideCanvas(page,name,'Backend input remains wholly readable on the large map',{text:true});
    await expect.poll(()=>name.evaluate(el=>parseFloat(getComputedStyle(el).fontSize)*el.getBoundingClientRect().width/el.offsetWidth)).toBeGreaterThanOrEqual(14);
  }
  await page.locator('[data-input-id="create"]').click();
  await expect(page.locator('[data-reading-title]')).toHaveText('POST /api/jobs');
  const implementation=await page.locator('[data-map]').evaluate(map=>map.visibleEdges.find(e=>e.from==='create'&&e.to==='routes'));
  await expect(page.locator(`[data-edge-ids~="${implementation.id}"]`).first()).toHaveClass(/flow-edge-active/);
  await testInfo.attach('journey-01b — Reveal backend inputs and follow the HTTP entry',{body:await workspace.screenshot(),contentType:'image/png'});
  await showWholeMap(page);
  expect(await worldGeometry(page)).toEqual(geometry);
  expect(await page.locator('[data-map]').evaluate(map=>{const {x,y,zoom}=map.captureViewport();return {x,y,zoom};})).toEqual(overviewCamera);
  }
  await page.getByRole('button',{name:'Zoom into Backend API',exact:true}).click();
  await expect(page.locator('[data-reading-title]')).toHaveText('Backend API');
  const api=prepared.records.find(n=>n.id==='api');
  for(const id of api.children){
    const title=page.locator(`.react-flow__node[data-id="${id}"] .flow-part>strong`);
    await expect(title).toBeVisible();
    await expect.poll(()=>title.evaluate(el=>parseFloat(getComputedStyle(el).fontSize)*el.getBoundingClientRect().width/el.offsetWidth),{message:'External calls have readable text after entry'}).toBeGreaterThanOrEqual(14);
  }
  const visibleCalls=await page.locator(api.children.map(id=>`.react-flow__node[data-id="${id}"] .flow-part>strong`).join(',')).evaluateAll(headings=>{
    const canvas=document.querySelector('.flow-root').getBoundingClientRect();
    return headings.filter(heading=>{
      const range=document.createRange();range.selectNodeContents(heading);const box=range.getBoundingClientRect();
      return box.left>=canvas.left&&box.top>=canvas.top&&box.right<=canvas.right&&box.bottom<=canvas.bottom;
    }).length;
  });
  expect(visibleCalls,'External entrance starts at an actual call').toBeGreaterThan(0);
  await testInfo.attach('journey-02 — Enter the API and its six calls',{body:await workspace.screenshot(),contentType:'image/png'});
  await showWholeMap(page);
  await expect(page.locator('[data-component-overview]')).toHaveCount(rootCount);
  await page.getByRole('button',{name:'Zoom into Web application',exact:true}).click();
  await expect(page.locator('[data-reading-title]')).toHaveText('Web application');
  await expect(page.locator('.flow-location')).toHaveText('Web application');
  await testInfo.attach('journey-03 — Enter the component and its areas',{body:await workspace.screenshot(),contentType:'image/png'});
  for(let step=0;step<3;step++){
    const previousZoom=await page.locator('[data-map]').evaluate(map=>map.captureViewport().zoom);
    await page.getByRole('button',{name:'Zoom out',exact:true}).click();
    await expect.poll(()=>page.locator('[data-map]').evaluate(map=>map.captureViewport().zoom)).toBeLessThan(previousZoom);
    const areaIDs=prepared.records.find(n=>n.id==='front').children;
    const headings=page.locator([
      '[data-component-overview="front"] .flow-component-overview-heading>strong',
      '[data-frame-title="front"]>strong',
      ...areaIDs.flatMap(id=>[`[data-summary-area="${id}"] .flow-part>strong`,`[data-frame-title="${id}"]>strong`]),
    ].join(','));
    await expect.poll(()=>headings.evaluateAll(elements=>{
      const canvas=document.querySelector('.flow-root').getBoundingClientRect();
      return elements.filter(el=>{
        const style=getComputedStyle(el),bounds=el.getBoundingClientRect();
        if(style.visibility==='hidden'||parseFloat(style.fontSize)*bounds.width/el.offsetWidth<12)return false;
        const range=document.createRange();range.selectNodeContents(el);const box=range.getBoundingClientRect();
        return box.width>0&&box.height>0&&box.left>=canvas.left&&box.top>=canvas.top&&box.right<=canvas.right&&box.bottom<=canvas.bottom;
      }).length;
    }),{message:'Zooming out retains a readable component or area heading on the map'}).toBeGreaterThan(0);
    await expect.poll(()=>page.locator('[data-component-overview="front"]').evaluateAll((summaries,areaIDs)=>{
      if(!summaries.length)return false;
      return areaIDs.some(id=>{
        const el=document.querySelector(`.react-flow__node[data-id="${CSS.escape(id)}"]`);
        if(!el)return false;
        const style=getComputedStyle(el),box=el.getBoundingClientRect();
        return style.visibility!=='hidden'&&style.display!=='none'&&box.width>0&&box.height>0;
      });
    },areaIDs),{message:'The component summary must not cover its still-visible interior'}).toBe(false);
    await testInfo.attach(`journey-0${step+4} — Zoom out toward the component overview`,{body:await workspace.screenshot(),contentType:'image/png'});
    expect(await worldGeometry(page),'Zoom-out preserves the placed world').toEqual(geometry);
  }
  await showWholeMap(page);
  await expect(page.locator('[data-component-overview]')).toHaveCount(rootCount);
  await assertOverviewReadable(page,{participants:prepared.records});
  await testInfo.attach(`journey-07 — Return to all participants${inputs?' and both input collections':''}`,{body:await workspace.screenshot(),contentType:'image/png'});
  expect(errors).toEqual([]);
});

test('whole map remeasures readable headings after a desktop resize',async({page},testInfo)=>{
  const errors=await openFixture(page);
  const savedOverview=await page.locator('[data-map]').evaluate(map=>map.captureViewport());
  await page.getByRole('button',{name:'Zoom into Backend API',exact:true}).click();
  await expect(page.locator('[data-reading-title]')).toHaveText('Backend API');
  let previousCamera,stable=0;
  await expect.poll(async()=>{
    const current=JSON.stringify(await page.locator('[data-map]').evaluate(map=>map.captureViewport()));
    stable=current===previousCamera?stable+1:0;previousCamera=current;return stable;
  },{message:'The reading camera finishes its entrance before resizing',intervals:[100]}).toBeGreaterThanOrEqual(2);
  const before=await worldGeometry(page);
  const readingCamera=await page.locator('[data-map]').evaluate(map=>map.captureViewport());
  await page.setViewportSize({width:1680,height:1050});
  // Resizing while reading must not replace the reader's world or camera.
  expect(await worldGeometry(page)).toEqual(before);
  expect(await page.locator('[data-map]').evaluate(map=>map.captureViewport())).toEqual(readingCamera);
  await showWholeMap(page);
  await expect.poll(()=>page.locator('[data-map]').evaluate(map=>map.captureViewport()),'Whole-map camera fits the changed viewport').not.toEqual(savedOverview);
  await expect(page.locator('[data-component-overview]')).toHaveCount(roots.length);
  await testInfo.attach('journey-01 — Whole map after enlarging the desktop window',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  await assertOverviewReadable(page);
  for(const item of roots){
    const button=await page.getByRole('button',{name:`Zoom into ${item.branch==='inputs'?'Inputs · ':''}${item.title}`,exact:true}).boundingBox();
    const frame=await page.locator(`.react-flow__node[data-id="${item.id}"]`).boundingBox();
    expect(inside(button,frame),`${item.title} zoom control stays in its resized frame`).toBe(true);
  }
  await expect(page.locator('.map-workspace')).toHaveScreenshot('resized-overview.png');
  await page.getByRole('button',{name:'Zoom into Backend API',exact:true}).click();
  await expect(page.locator('[data-component-overview="api"]')).toHaveCount(0);
  await assertInsideCanvas(page,page.locator('.react-flow__node[data-id="download"] .flow-part>strong'),'The resized API entrance reveals its actual call',{text:true});
  await page.locator('[data-map]').evaluate((map,viewport)=>map.restoreReadingState({scope:'',viewport}),savedOverview);
  await expect(page.locator('[data-component-overview]')).toHaveCount(roots.length);
  await assertOverviewReadable(page);
  await expect(page.locator('.map-workspace')).toHaveScreenshot('resized-overview.png');
  const enlarged=await page.locator('[data-map]').evaluate(map=>map.captureViewport());
  await page.setViewportSize({width:1440,height:900});
  await expect.poll(()=>page.locator('[data-map]').evaluate(map=>map.captureViewport()),'Resizing the whole map updates its camera without requiring a different arrangement').not.toEqual(enlarged);
  await expect.poll(()=>page.locator('[data-map]').evaluate(map=>map.captureViewport().fit)).toBe(true);
  await assertOverviewReadable(page);
  expect(errors).toEqual([]);
});

test('visual journey: aim, zoom through both detail levels, return',async({page},testInfo)=>{
  test.setTimeout(60000);
  const errors=await openFixture(page);
  const geometry=await worldGeometry(page);
  const workspace=page.locator('.map-workspace');
  const camera=()=>page.locator('.react-flow__viewport').evaluate(el=>{
    const m=new DOMMatrixReadOnly(getComputedStyle(el).transform);return {x:m.e,y:m.f,zoom:m.a};
  });
  let pointer=null,gesture=0;
  async function capture(action){
    await expect.poll(()=>page.evaluate(()=>{
      const canvas=document.querySelector('.flow-root').getBoundingClientRect();
      const nodes=[...document.querySelectorAll('.react-flow__node')];
      const overlapping=[];
      const intersect=(a,b)=>Math.min(a.right,b.right,canvas.right)-Math.max(a.left,b.left,canvas.left)>1&&
        Math.min(a.bottom,b.bottom,canvas.bottom)-Math.max(a.top,b.top,canvas.top)>1;
      for(const summary of document.querySelectorAll('[data-component-overview]')){
        const frame=nodes.find(node=>node.dataset.id===summary.dataset.componentOverview)?.getBoundingClientRect();
        if(!frame)continue;
        for(const card of document.querySelectorAll('.react-flow__node>.flow-part')){
          if(getComputedStyle(card).visibility==='hidden')continue;
          const box=card.getBoundingClientRect();
          if(box.left>=frame.left&&box.right<=frame.right&&box.top>=frame.top&&box.bottom<=frame.bottom&&intersect(box,summary.getBoundingClientRect()))
            overlapping.push({summary:summary.dataset.componentOverview,child:card.parentElement.dataset.id});
        }
      }
      return overlapping;
    }),{message:'A revealed child card must not cover the component summary'}).toEqual([]);
    // Unlike toHaveScreenshot, a raw buffer comparison has no stability wait.
    let image=await workspace.screenshot({animations:'disabled'});
    for(let attempt=0;attempt<5;attempt++){
      const next=await workspace.screenshot({animations:'disabled'});
      if(image.equals(next))return {image,metadata:{action,gesture,pointer,camera:await camera()}};
      image=next;
    }
    await testInfo.attach('Unstable canvas — '+action,{body:image,contentType:'image/png'});
    throw new Error('Canvas did not settle before '+action);
  }
  async function attach(name,state){
    await testInfo.attach(name+' — '+state.metadata.action,{body:state.image,contentType:'image/png'});
    await testInfo.attach(name+' — camera and aim',{body:JSON.stringify(state.metadata,null,2),contentType:'application/json'});
  }
  async function frame(name,action,check=async()=>{},state){
    const actual=state?{...state,metadata:{...state.metadata,action}}:await capture(action);
    await attach(name,actual);
    await check();
    // Keep collecting the journey after a visual difference, while the test
    // remains failed. This never accepts or rewrites a reference image.
    expect.soft(actual.image).toMatchSnapshot(name+'.png',{maxDiffPixels:0});
  }
  async function aim(locator,label){
    await attach('Before aiming at '+label,await capture('Locate the visible '+label+' heading'));
    await assertInsideCanvas(page,locator,label,{text:true});
    const box=await textBounds(locator);
    pointer={x:box.x+box.width/2,y:box.y+box.height/2};
    await page.mouse.move(pointer.x,pointer.y);
    await page.locator('#visual-aim').evaluate((el,p)=>{
      el.hidden=false;el.style.left=p.x+'px';el.style.top=p.y+'px';
    },pointer);
  }
  async function panToHeading(locator,label){
    const stage=await page.locator('.flow-root').boundingBox();
    const box=await textBounds(locator);
    // Leave room below the heading for the parts the next pinch will reveal.
    const ready=box=>inside(box,stage)&&box.y<=stage.y+stage.height/3;
    if(ready(box))return;
    await test.step('Pan to the '+label+' heading',async()=>{
      await attach('journey-04a-before-pan',await capture('The '+label+' heading needs a pan'));
      await page.locator('#visual-aim').evaluate(el=>{el.hidden=true;});
      pointer=null;
      const before=await camera();
      // An ordinary wheel pan is a user action; no camera API or hidden teleport.
      await page.mouse.move(stage.x+stage.width/2,stage.y+stage.height/2);
      const destination={x:stage.x+stage.width/2,y:stage.y+Math.min(120,stage.height/3)};
      // The canvas may apply scroll sensitivity, so inspect each real movement
      // instead of assuming a wheel delta equals a screen-pixel displacement.
      for(let movement=0;movement<8;movement++){
        const current=await textBounds(locator);
        if(ready(current))break;
        const revision=await page.locator('[data-map]').getAttribute('data-camera-revision');
        await page.mouse.wheel(current.x+current.width/2-destination.x,current.y+current.height/2-destination.y);
        await expect.poll(()=>page.locator('[data-map]').getAttribute('data-camera-revision')).not.toBe(revision);
      }
      await attach('journey-04b-after-pan',await capture('Pan until '+label+' is visible'));
      expect((await camera()).zoom,'Panning does not change the zoom').toBeCloseTo(before.zoom,6);
      await assertInsideCanvas(page,locator,label,{text:true});
      expect(await worldGeometry(page)).toEqual(geometry);
    });
  }
  async function pinchUntil(revealed,beforeName,afterName,description,heading,checkRevealed=async()=>{},afterAction){
    for(let step=0;step<24;step++){
      const before=await capture('Before zooming to '+description);
      const stage=await page.locator('.flow-root').boundingBox();
      const worldPoint={x:(pointer.x-stage.x-before.metadata.camera.x)/before.metadata.camera.zoom,
        y:(pointer.y-stage.y-before.metadata.camera.y)/before.metadata.camera.zoom};
      const revision=await page.locator('[data-map]').getAttribute('data-camera-revision');
      await page.keyboard.down('Control');
      try{await page.mouse.wheel(0,-8);}finally{await page.keyboard.up('Control');}
      await expect.poll(()=>page.locator('[data-map]').getAttribute('data-camera-revision')).not.toBe(revision);
      gesture++;
      const after=await capture('Zoom gesture '+gesture+' toward '+description);
      await attach('Gesture '+gesture,after);
      const next=after.metadata.camera;
      expect(next.zoom,'The pinch actually zooms in').toBeGreaterThan(before.metadata.camera.zoom);
      expect(Math.abs(stage.x+next.x+worldPoint.x*next.zoom-pointer.x),'The point under the pointer keeps its horizontal position').toBeLessThanOrEqual(.75);
      expect(Math.abs(stage.y+next.y+worldPoint.y*next.zoom-pointer.y),'The point under the pointer keeps its vertical position').toBeLessThanOrEqual(.75);
      const isRevealed=await revealed();
      if(!isRevealed)await assertInsideCanvas(page,heading,'The heading remains readable before detail opens',{text:true});
      expect(await worldGeometry(page),'Revealing detail does not rearrange the world').toEqual(geometry);
      if(isRevealed){
        // Accept neither reference until the revealed content has passed its checks.
        await checkRevealed();
        await frame(beforeName,'Last gesture before '+description,async()=>{},before);
        await frame(afterName,afterAction||'First gesture revealing '+description,checkRevealed,after);
        return;
      }
    }
    throw new Error('Pinch gestures did not reveal '+description);
  }
  const frontHeading=componentHeading(page,'front','Web application');
  const editingHeading=areaHeading(page,'editing','Job editing');
  await test.step('01 · Default view',async()=>frame('journey-01-overview','Default view',()=>assertOverviewReadable(page)));
  await test.step('02 · Aim inside Web application',async()=>{
    await aim(frontHeading,'Web application');
    await frame('journey-02-aim-system','Aim at Web application (pink ring)');
  });
  await test.step('03–04 · Pinch until the system opens',async()=>{
    await pinchUntil(()=>page.locator('[data-summary-area="editing"]').isVisible(),
      'journey-03-before-areas','journey-04-areas-visible','the system’s areas',frontHeading,async()=>{
        const location=page.locator('.flow-location');
        await expect(location).toHaveText('Web application');
        await assertInsideCanvas(page,location,'The component context remains visible after detail opens',{text:true,container:'.map-stage'});
        // Either actual area can enter the viewport first; native layout need
        // not put Job editing ahead of Progress and results under the pointer.
        const children=page.locator(records.find(n=>n.id==='front').children.flatMap(id=>[
          `[data-summary-area="${id}"] .flow-overview-card>strong`,
          `[data-frame-title="${id}"]>strong`,
        ]).join(','));
        const readableChildren=await children.evaluateAll(elements=>{
          const canvas=document.querySelector('.flow-root').getBoundingClientRect();
          return elements.filter(el=>{
            const style=getComputedStyle(el),bounds=el.getBoundingClientRect();
            if(style.visibility==='hidden'||parseFloat(style.fontSize)*bounds.width/el.offsetWidth<12)return false;
            const range=document.createRange();range.selectNodeContents(el);const box=range.getBoundingClientRect();
            return box.width>0&&box.height>0&&box.left>=canvas.left&&box.top>=canvas.top&&box.right<=canvas.right&&box.bottom<=canvas.bottom;
          }).length;
        });
        if(!readableChildren){
          await assertInsideCanvas(page,frontHeading,'The component heading remains visible until its interior arrives',{text:true});
          await assertInsideCanvas(page,page.locator('[data-component-overview="front"] [data-overview-area="editing"]'),'The actual Job editing entrance remains visible',{text:true});
        }
      },
      'Component detail threshold crossed; pan to the area follows');
  });
  await test.step('05 · Aim inside Job editing',async()=>{
    await panToHeading(editingHeading,'Job editing');
    await aim(editingHeading,'Job editing');
    await frame('journey-05-aim-area','Aim at Job editing (pink ring)');
  });
  await test.step('06–07 · Pinch until the parts appear',async()=>{
    await pinchUntil(()=>page.locator('.react-flow__node[data-id="editor"]').isVisible(),
      'journey-06-before-parts','journey-07-parts-visible','the actual parts',editingHeading,async()=>{
        await assertInsideCanvas(page,editingHeading,'Job editing heading',{text:true});
        await assertInsideCanvas(page,page.locator('.react-flow__node[data-id="editor"]'),'Job editor card');
        await assertInsideCanvas(page,page.locator('.react-flow__node[data-id="editor"] .flow-part>strong'),'Job editor heading',{text:true});
      });
  });
  await test.step('08 · Return to the whole map',async()=>{
    await page.locator('#visual-aim').evaluate(el=>{el.hidden=true;});
    await showWholeMap(page);
    await expect(page.locator('.flow-location')).toHaveText('System map');
    await expect(page.locator('[data-component-overview]')).toHaveCount(roots.length);
    await frame('journey-08-return','Return to the whole map',async()=>{
      await assertOverviewReadable(page);
      expect(await worldGeometry(page)).toEqual(geometry);
    });
  });
  await testInfo.attach('Final camera',{body:JSON.stringify(await camera(),null,2),contentType:'application/json'});
  expect(errors).toEqual([]);
});

test('component zoom reveals its named areas without losing the heading',async({page})=>{
  const errors=await openFixture(page);
  await page.getByRole('button',{name:'Zoom into Web application',exact:true}).click();
  await expect(page.locator('[data-reading-title]')).toHaveText('Web application');
  for(const [id,title] of [['editing','Job editing'],['tracking','Progress and results']]){
    await expect(page.locator(`[data-summary-area="${id}"]`).or(page.locator('.flow-area-title').filter({hasText:title}))).toBeVisible();
  }
  await expect(page.locator('.flow-component-title').filter({hasText:'Web application'})).toBeInViewport();
  await page.getByRole('heading',{name:'Two systems · five external participants'}).hover();
  // Repeated local Chromium captures differ at up to three glyph-edge pixels.
  await expect(page.locator('.map-workspace')).toHaveScreenshot('component.png',{maxDiffPixels:3});
  expect(errors).toEqual([]);
});

test('restoring a close part view does not cover it with the component summary',async({page},testInfo)=>{
  const errors=await openFixture(page);
  await page.getByRole('button',{name:'Zoom into Web application',exact:true}).click();
  await expect(page.locator('.flow-location')).toHaveText('Web application');
  await page.locator('[data-map]').evaluate(map=>{
    const saved=map.captureViewport();
    const editor=map.querySelector('.react-flow__node[data-id="editor"]');
    const position=new DOMMatrixReadOnly(editor.style.transform);
    const scale=new DOMMatrixReadOnly(getComputedStyle(editor.querySelector('.flow-part')).transform).a;
    const zoom=1.5/scale;
    map.restoreReadingState({scope:'editing',viewport:{...saved,
      x:-position.e*zoom+50,y:-position.f*zoom+60,zoom,
      detailAreas:['editing','tracking'],componentsOpen:true,fit:false}});
  });
  const heading=page.locator('.react-flow__node[data-id="editor"] .flow-part>strong');
  await assertInsideCanvas(page,heading,'The restored view shows its actual part heading',{text:true});
  const ancestor=await page.locator('[data-frame-title="editing"]>strong').boundingBox();
  const canvas=await page.locator('.flow-root').boundingBox();
  expect(ancestor.y+ancestor.height,'The area title has left the top of the viewport').toBeLessThan(canvas.y);
  await expect(page.locator('[data-component-overview="front"]'),'A missing area title must not put the component summary over a visible part').toHaveCount(0);
  await testInfo.attach('journey-01 — Return to a part with its area heading above the viewport',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  expect(errors).toEqual([]);
});

for(const phase of ['first initial layout','final initial reflow','later resize']){
  test(`worker failure during ${phase} leaves no pending map`,async({page},testInfo)=>{
    const errors=[];page.on('pageerror',error=>errors.push(error.message));
    await page.addInitScript(({phase})=>{
      const NativeWorker=Worker;
      window.workerFailure={workers:0,terminations:0,layouts:0};
      window.Worker=class extends NativeWorker{
        constructor(...args){super(...args);workerFailure.workers++;}
        postMessage(message,...args){
          if(message.cmd==='layout'){
            workerFailure.layouts++;
            const stage=document.querySelector('.map-stage');
            // Change the actual host size after its first measurement. The
            // next layout after React mounts is the final initial reflow.
            if(phase==='final initial reflow'&&workerFailure.layouts===1)stage.style.width=(stage.clientWidth-96)+'px';
            const fail=phase==='first initial layout'||phase==='final initial reflow'&&document.querySelector('.react-flow')||phase==='later resize'&&workerFailure.armed;
            if(fail&&!workerFailure.failed){
              workerFailure.failed=true;
              workerFailure.beforeReady=!document.querySelector('[data-map]')?.captureViewport?.();
              queueMicrotask(()=>this.dispatchEvent(new ErrorEvent('error',{cancelable:true,message:'Forced map worker failure'})));
              return;
            }
          }
          return super.postMessage(message,...args);
        }
        terminate(){workerFailure.terminations++;return super.terminate();}
      };
      // The fixture calls the renderer directly. Observe its initial rejection
      // as the ordinary report's caller does, without hiding browser errors.
      let create;
      Object.defineProperty(window,'rmCreateFlow',{get:()=>create,set:value=>{
        create=(...args)=>value(...args).catch(error=>{
          workerFailure.initialError=error.message;
          return {layout:{edges:[]},capture:()=>null};
        });
      }});
    },{phase});
    await page.goto('/');
    const map=page.locator('[data-map]');
    let camera,geometry;
    if(phase==='later resize'){
      await expect.poll(()=>map.evaluate(map=>map.captureViewport?.())).toBeTruthy();
      camera=await map.evaluate(map=>map.captureViewport());geometry=await worldGeometry(page);
      await page.evaluate(()=>{workerFailure.armed=true;});
      await page.setViewportSize({width:1344,height:900});
    }
    await expect.poll(()=>page.evaluate(()=>workerFailure.failed)).toBe(true);
    if(phase==='first initial layout'){
      await expect.poll(()=>page.evaluate(()=>workerFailure.initialError)).toBe('Forced map worker failure');
      await expect(page.locator('.flow-root')).toHaveCount(0);
      await expect(page.locator('.map-stage>svg')).not.toHaveCSS('display','none');
    }else{
      await expect.poll(()=>map.evaluate(map=>map.captureViewport?.())).toBeTruthy();
      await expect(page.locator('.flow-location')).toHaveText('Could not arrange this map. Reload to try again.');
      await expect(page.locator('.react-flow__node').first()).toBeVisible();
      await expect(page.locator('.flow-root')).not.toHaveAttribute('inert','');
      expect(await page.evaluate(()=>workerFailure.initialError)).toBeUndefined();
      if(phase==='later resize'){
        expect(await worldGeometry(page)).toEqual(geometry);
        const current=await map.evaluate(map=>map.captureViewport());
        for(const key of ['x','y','zoom','layoutKey'])expect(current[key],`Failed reflow preserves ${key}`).toEqual(camera[key]);
        const box=await page.locator('.flow-root').boundingBox();
        await page.mouse.move(box.x+box.width/2,box.y+box.height-60);await page.mouse.down();
        await page.mouse.move(box.x+box.width/2-40,box.y+box.height-100,{steps:4});await page.mouse.up();
        await expect.poll(()=>map.evaluate(map=>map.captureViewport().y)).not.toBe(camera.y);
        expect((await map.evaluate(map=>map.captureViewport())).zoom).toBe(camera.zoom);
        expect(await worldGeometry(page)).toEqual(geometry);
      }
    }
    await expect(map).not.toHaveClass(/flow-initializing/);
    await expect(page.locator('.flow-loading')).toHaveCount(0);
    expect(await page.evaluate(()=>({workers:workerFailure.workers,terminations:workerFailure.terminations,beforeReady:workerFailure.beforeReady})))
      .toEqual({workers:1,terminations:1,beforeReady:phase!=='later resize'});
    expect(errors).toEqual([]);
    await testInfo.attach(`Worker failure — ${phase}`,{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  });
}
