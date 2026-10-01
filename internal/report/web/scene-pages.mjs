// Pages for the scene's node tests: the seeded synthetic graphs of
// fixtures/ and, when
// REPOMAP_SCENE_PAGES names directories of page JSON, the real reports'
// canvases, each the {items, relations, areas, inputOwner} the page hands
// rmCreateFlow and the `scene` it carries saved (#rm-scene, report.json
// format 94 on). Capture them from rendered reports with
// `node scene-pages.mjs capture REPORT.html... --out DIR` (Playwright,
// headless; the reports are served from a loopback server).
import {readFileSync,readdirSync,existsSync} from 'node:fs';
import {join,basename} from 'node:path';

// The seeded synthetic graphs (visual/synthetic-graphs.mjs writes them):
// no inputs, 150 outside systems, cycles, 40 loose parts.
const fixtures=new URL('./fixtures/',import.meta.url);
export const syntheticPages=(existsSync(fixtures)?readdirSync(fixtures):[]).filter(name=>/^synthetic-.*\.json$/.test(name)).sort().map(name=>{
  const graph=JSON.parse(readFileSync(new URL(name,fixtures),'utf8'));
  return [basename(name,'.json'),{items:graph.records,relations:graph.relations,areas:graph.areas||[],inputOwner:graph.inputOwner||{},scene:graph.scene}];
});

// The real reports' canvases named by REPOMAP_SCENE_PAGES, a list of
// directories (or files) separated by commas.
export function realPages(){
  const named=(process.env.REPOMAP_SCENE_PAGES||'').split(',').filter(Boolean);
  const files=named.flatMap(path=>!existsSync(path)?[]:path.endsWith('.json')?[path]:readdirSync(path).filter(name=>name.endsWith('.json')).sort().map(name=>join(path,name)));
  return files.map(file=>{
    const page=JSON.parse(readFileSync(file,'utf8'));
    // A canvas captured before the scene was saved draws no facts: capture
    // it again from a report of format 94 or later.
    if(!page.scene)throw new Error(`${file}: no saved scene; capture the page again from a report that carries #rm-scene`);
    return [basename(file,'.json'),page];
  });
}

// Text measured as the tests need it: wide enough to wrap as a browser
// would, without one.
export function measure(text,font){
  const size=Number(/(\d+(?:\.\d+)?)px/.exec(font)?.[1]||13),bold=/^(6|7)\d\d/.test(font);
  return String(text??'').length*size*(bold?.6:.55);
}

// Capture: render each report's canvas input to DIR/<name>.json.
if(process.argv[1]&&import.meta.url.endsWith(basename(process.argv[1]))&&process.argv[2]==='capture'){
  const {chromium}=await import('playwright');
  const {createServer}=await import('node:http');
  const {readFile,writeFile}=await import('node:fs/promises');
  const args=process.argv.slice(3),out=args[args.indexOf('--out')+1],reports=args.filter((arg,i)=>arg!=='--out'&&args[i-1]!=='--out');
  const server=createServer(async(request,response)=>{
    const index=Number(new URL(request.url,'http://x').pathname.slice(1));
    try{response.writeHead(200,{'content-type':'text/html'});response.end(await readFile(reports[index]));}catch{response.writeHead(404);response.end();}
  }).listen(0,'127.0.0.1');
  await new Promise(resolve=>server.once('listening',resolve));
  const browser=await chromium.launch();
  try{
    for(const [index,report] of reports.entries()){
      const page=await browser.newPage();
      await page.addInitScript(()=>{let inner;window.__flowArgs=[];Object.defineProperty(window,'rmCreateFlow',{configurable:true,get(){return inner;},set(value){
        inner=async function(map,stage,items,relations,areas,inputOwner){
          const scene=JSON.parse(document.getElementById('rm-scene')?.textContent||'null');
          window.__flowArgs.push(JSON.stringify({items,relations,areas,inputOwner,scene}));return value.apply(this,arguments);};}});});
      await page.goto(`http://127.0.0.1:${server.address().port}/${index}`,{waitUntil:'load',timeout:300000});
      await page.waitForFunction(()=>window.__flowArgs.length>0,null,{timeout:300000});
      const name=basename(report).replace(/\.html$/,'');
      await writeFile(join(out,`${name}.json`),await page.evaluate(()=>window.__flowArgs[0]));
      console.log(name);
      await page.close();
    }
  }finally{await browser.close();server.close();}
}
