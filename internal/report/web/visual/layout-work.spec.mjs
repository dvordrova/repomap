import {test,expect} from '@playwright/test';
import {manyExternalInventory} from './two-systems-five-externals.mjs';

const geometry=page=>page.locator('.react-flow__node').evaluateAll(nodes=>nodes.map(node=>{
  const transform=new DOMMatrixReadOnly(node.style.transform);
  return {id:node.dataset.id,x:transform.e,y:transform.f,width:parseFloat(node.style.width),height:parseFloat(node.style.height)};
}));
const camera=page=>page.locator('[data-map]').evaluate(map=>map.captureViewport?.());
const work=page=>page.evaluate(()=>({
  workers:nativeLayoutWork.workers,pending:nativeLayoutWork.pending,
  inner:nativeLayoutWork.requests.filter(request=>request.kind==='inner').length,
  outer:nativeLayoutWork.requests.filter(request=>request.kind==='outer').length,
  unknown:nativeLayoutWork.requests.filter(request=>request.kind==='unknown').length,
}));

async function wholeMap(page){
  const revision=await page.locator('[data-map]').getAttribute('data-camera-revision');
  await page.getByRole('button',{name:'Show whole map',exact:true}).click();
  await expect(page.locator('[data-map]')).not.toHaveAttribute('data-camera-revision',revision);
  await expect(page.locator('.flow-location')).toHaveText('System map');
  await expect.poll(async()=>({fit:(await camera(page))?.fit,open:(await camera(page))?.componentsOpen})).toEqual({fit:true,open:false});
}

function expectAffineInteriors(before,after,records){
  const previous=new Map(before.map(node=>[node.id,node])),next=new Map(after.map(node=>[node.id,node]));
  expect([...next.keys()].sort(),'Resizing preserves every original node').toEqual([...previous.keys()].sort());
  const byID=new Map(records.map(node=>[node.id,node]));
  const descendants=id=>(byID.get(id)?.children||[]).flatMap(child=>[child,...descendants(child)]);
  const childIDs=new Set(records.flatMap(node=>node.children||[]));
  for(const root of records.filter(node=>!childIDs.has(node.id)&&node.children?.length)){
    const ids=descendants(root.id),anchorID=ids.find(id=>!byID.get(id)?.children?.length);
    expect(anchorID,`${root.title} has an actual interior leaf`).toBeTruthy();
    const anchor=previous.get(anchorID),moved=next.get(anchorID),scale=moved.width/anchor.width;
    expect(scale).toBeGreaterThan(0);
    for(const id of ids){
      const a=previous.get(id),b=next.get(id);
      for(const [name,actual,expected] of [
        ['width',b.width,a.width*scale],['height',b.height,a.height*scale],
        ['x',b.x-moved.x,(a.x-anchor.x)*scale],['y',b.y-moved.y,(a.y-anchor.y)*scale],
      ])expect(Math.abs(actual-expected),`${root.title}: ${id} keeps its local ${name} after one scale/translation`).toBeLessThan(.02);
    }
  }
}

for(const inputs of [true,false]){
  test(`${inputs?21:19} roots reuse their interior layouts throughout navigation and resizing`,async({page},testInfo)=>{
    const errors=[];page.on('pageerror',error=>errors.push(error.message));
    await page.addInitScript(()=>{
      const NativeWorker=Worker;
      window.nativeLayoutWork={workers:0,pending:0,requests:[]};
      window.Worker=class extends NativeWorker{
        constructor(...args){
          super(...args);nativeLayoutWork.workers++;this.layouts=new Set();
          this.addEventListener('message',event=>{
            if(this.layouts.delete(event.data.id))nativeLayoutWork.pending--;
          });
        }
        postMessage(message,...args){
          if(message.cmd==='layout'){
            const graph=message.graph,children=graph.children||[];
            const kind=children.length===1&&children[0].children?'inner':children.every(node=>!node.children)?'outer':'unknown';
            const count=node=>(node.children||[]).reduce((total,child)=>{
              const nested=count(child);return {nodes:total.nodes+1+nested.nodes,edges:total.edges+nested.edges};
            },{nodes:0,edges:node.edges?.length||0});
            nativeLayoutWork.requests.push({kind,...count(graph)});
            nativeLayoutWork.pending++;this.layouts.add(message.id);
          }
          return super.postMessage(message,...args);
        }
      };
    });
    await page.goto(`/?many-external${inputs?'':'&no-inputs'}`);
    await expect.poll(()=>camera(page),{message:'The initial world is ready before measuring work'}).toBeTruthy();
    await expect.poll(async()=>(await work(page)).pending).toBe(0);
    const initial=await work(page),placed=await geometry(page),prepared=manyExternalInventory({inputs});
    expect(initial.workers).toBe(1);expect(initial.inner).toBeGreaterThan(0);expect(initial.outer).toBeGreaterThan(0);expect(initial.unknown).toBe(0);
    expect(placed.map(node=>node.id).sort()).toEqual(prepared.records.map(node=>node.id).sort());

    const canvas=await page.locator('.flow-root').boundingBox(),beforePan=await camera(page);
    await page.mouse.move(canvas.x+canvas.width/2,canvas.y+canvas.height-60);await page.mouse.down();
    await page.mouse.move(canvas.x+canvas.width/2-40,canvas.y+canvas.height-100,{steps:4});await page.mouse.up();
    await expect.poll(async()=>(await camera(page)).y).not.toBe(beforePan.y);
    const beforeWheel=await camera(page);await page.mouse.wheel(0,60);
    await expect.poll(async()=>(await camera(page)).y).not.toBe(beforeWheel.y);
    const beforeZoom=await camera(page);await page.getByRole('button',{name:'Zoom in',exact:true}).click();
    await expect.poll(async()=>(await camera(page)).zoom).toBeGreaterThan(beforeZoom.zoom);
    await wholeMap(page);
    await page.locator('[data-zoom-into="front"]').click();
    await expect(page.locator('.flow-location')).toHaveText('Web application');
    await wholeMap(page);
    if(inputs){
      await page.locator('[data-zoom-into="backend-inputs"]').click();
      await page.locator('[data-input-id="create"]').click();
      await expect(page.locator('[data-reading-title]')).toHaveText('POST /api/jobs');
      await wholeMap(page);
    }
    expect(await work(page),'Pan, wheel, zoom, selection and same-size All perform no native layout').toEqual(initial);
    expect(await geometry(page),'Navigation preserves the complete world geometry').toEqual(placed);

    const resizeRevision=await page.locator('[data-map]').getAttribute('data-camera-revision');
    await page.setViewportSize({width:1344,height:900});
    await expect.poll(async()=>(await work(page)).outer,{message:'A real resize lays out the outer frames'}).toBeGreaterThan(initial.outer);
    await expect.poll(async()=>(await work(page)).pending).toBe(0);
    await expect(page.locator('[data-map]')).not.toHaveAttribute('data-camera-revision',resizeRevision);
    const resized=await work(page);
    expect(resized.inner,'Resizing reuses every already placed interior').toBe(initial.inner);
    expect(resized.workers).toBe(1);expect(resized.unknown).toBe(0);
    expectAffineInteriors(placed,await geometry(page),prepared.records);
    expect(errors).toEqual([]);
    await testInfo.attach('Native layout workload',{body:JSON.stringify({initial,resized,requests:await page.evaluate(()=>nativeLayoutWork.requests)},null,2),contentType:'application/json'});
    await testInfo.attach('Reused interiors after resizing',{body:await page.locator('.map-workspace').screenshot(),contentType:'image/png'});
  });
}
