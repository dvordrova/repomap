import {test,expect} from '@playwright/test';
import {records} from './two-systems-five-externals.mjs';

const roots=records.filter(n=>['component','communication'].includes(n.branch));
const worldGeometry=page=>page.locator('.react-flow__node').evaluateAll(nodes=>nodes.map(n=>[
  n.dataset.id,n.style.transform,n.style.width,n.style.height,
]));

// Text ranges measure the actual heading, not its potentially much wider block.
const textBounds=locator=>locator.evaluate(el=>{
  const range=document.createRange();range.selectNodeContents(el);
  const box=range.getBoundingClientRect();
  return {x:box.x,y:box.y,width:box.width,height:box.height};
});
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

async function openFixture(page){
  const errors=[];
  page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/');
  await expect(page.locator('[data-fixture-ready]')).toHaveAttribute('data-fixture-ready','true');
  await expect(page.locator('[data-component-overview]')).toHaveCount(7);
  await expect(page.locator('.flow-location')).toHaveText('System map');
  return errors;
}

async function assertOverviewReadable(page){
  const stage=await page.locator('.flow-root').boundingBox();
  for(const item of roots){
    const label=page.locator(`[data-component-overview="${item.id}"] strong`);
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
    await expect(page.getByRole('button',{name:`Zoom into ${item.title}`,exact:true})).toBeVisible();
    for(const child of records.filter(n=>n.branch==='area'&&item.children.includes(n.id))){
      const area=page.locator(`[data-component-overview="${item.id}"] [data-overview-area="${child.id}"]`);
      await expect(area).toBeVisible();
      const bounds=await area.boundingBox();
      expect(bounds.y+bounds.height,`${child.title} is visible inside ${item.title}`).toBeLessThanOrEqual(frame.y+frame.height+1);
    }
  }
}

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

test('external zoom reveals calls and returns to the same overview',async({page})=>{
  const errors=await openFixture(page);
  const geometry=await worldGeometry(page);
  await page.getByRole('button',{name:'Zoom into Backend API',exact:true}).click();
  await expect(page.locator('[data-reading-title]')).toHaveText('Backend API');
  for(const id of ['post','get','download']){
    const call=page.locator(`.react-flow__node[data-id="${id}"]`);
    await expect(call).toBeInViewport();
    await expect.poll(()=>call.locator('strong').evaluate(el=>
      parseFloat(getComputedStyle(el).fontSize)*el.getBoundingClientRect().width/el.offsetWidth
    ),{message:'Zoom makes call text readable'}).toBeGreaterThanOrEqual(14);
  }
  await page.getByRole('heading',{name:'Two systems · five external participants'}).hover();
  await expect(page.locator('.map-workspace')).toHaveScreenshot('external.png');
  await page.getByRole('button',{name:'Show whole map',exact:true}).click();
  await expect(page.locator('.flow-location')).toHaveText('System map');
  await assertOverviewReadable(page);
  await expect(page.locator('.map-workspace')).toHaveScreenshot('overview.png');
  expect(await worldGeometry(page)).toEqual(geometry);
  expect(errors).toEqual([]);
});

test('visual journey: aim, zoom through both detail levels, return',async({page},testInfo)=>{
  const errors=await openFixture(page);
  const geometry=await worldGeometry(page);
  const workspace=page.locator('.map-workspace');
  const camera=()=>page.locator('.react-flow__viewport').evaluate(el=>{
    const m=new DOMMatrixReadOnly(getComputedStyle(el).transform);return {x:m.e,y:m.f,zoom:m.a};
  });
  let pointer=null,gesture=0;
  async function capture(action){
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
    if(inside(box,stage))return;
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
        if(inside(current,stage))break;
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
      try{await page.mouse.wheel(0,-24);}finally{await page.keyboard.up('Control');}
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
    await page.getByRole('button',{name:'Show whole map',exact:true}).click();
    await expect(page.locator('.flow-location')).toHaveText('System map');
    await expect(page.locator('[data-component-overview]')).toHaveCount(7);
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
  await expect(page.locator('.map-workspace')).toHaveScreenshot('component.png');
  expect(errors).toEqual([]);
});
