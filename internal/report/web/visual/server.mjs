import {createServer} from 'node:http';
import {readFile, readdir, mkdtemp, rm} from 'node:fs/promises';
import {execFile} from 'node:child_process';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {fileURLToPath} from 'node:url';

const here = new URL('./', import.meta.url);
// REPOMAP_FIXTURE_TEMPLATES draws the fixture with another report's
// templates directory (a clean export of a commit), not this tree's.
const templates = process.env.REPOMAP_FIXTURE_TEMPLATES?new URL(`file://${process.env.REPOMAP_FIXTURE_TEMPLATES.replace(/\/?$/,'/')}`):new URL('../../templates/', here);
const css = (await Promise.all((await readdir(new URL('css/',templates))).sort()
  .filter(n=>n.endsWith('.css')).map(n=>readFile(new URL('css/'+n,templates),'utf8')))).join('\n');
const html = await readFile(new URL('fixture.html',here));
const routes = new Map([
  ['/', ['text/html', html]],
  ['/report.css', ['text/css', css]],
  ['/report-ui.js', ['text/javascript', await readFile(new URL('js/27-report-ui.js',templates))]],
  ['/fixture.mjs', ['text/javascript', await readFile(new URL('fixture.mjs',here))]],
  ['/two-systems-five-externals.mjs', ['text/javascript', await readFile(new URL('two-systems-five-externals.mjs',here))]],
]);
// A saved run named by REPOMAP_REAL_RUN, rendered once by the built binary
// (`repomap render`, no provider call) and served as /real-report.html.
let real;
const renderReal=()=>real||=(async()=>{
  const directory=await mkdtemp(join(tmpdir(),'repomap-real-')),output=join(directory,'report.html');
  try{
    await new Promise((resolve,reject)=>execFile(fileURLToPath(new URL('../../../../.bin/repomap',here)),['render',process.env.REPOMAP_REAL_RUN,'--output',output],
      {maxBuffer:1<<24},error=>error?reject(error):resolve()));
    return await readFile(output);
  }finally{await rm(directory,{recursive:true,force:true});}
})();
// Reports already rendered, named by REPOMAP_GEOMETRY_REPORTS, served as
// /geometry-<n>.html for the geometry checks (geometry.spec.mjs).
const geometryReports=(process.env.REPOMAP_GEOMETRY_REPORTS||'').split(',').filter(Boolean);
// Reports named by REPOMAP_INVARIANT_REPORTS, served as /invariant-<n>.html
// for the invariant table (invariants.spec.mjs).
const invariantReports=(process.env.REPOMAP_INVARIANT_REPORTS||'').split(',').filter(Boolean);
// Reports named by REPOMAP_JOURNEY_REPORTS, served as /journey-<n>.html for
// the journey check and the reading lints (journeys.spec.mjs).
const journeyReports=(process.env.REPOMAP_JOURNEY_REPORTS||'').split(',').filter(Boolean);
// This test-only server exposes only this fixture, its synthetic graphs,
// ordinary report assets and those rendered reports.
createServer(async(request,response)=>{
  const path=new URL(request.url,'http://127.0.0.1').pathname;
  const invariant=/^\/invariant-(\d+)\.html$/.exec(path);
  if(invariant&&invariantReports[Number(invariant[1])]){
    try{response.writeHead(200,{'Content-Type':'text/html','Cache-Control':'no-store'}).end(await readFile(invariantReports[Number(invariant[1])]));}
    catch(error){response.writeHead(500).end(String(error));}
    return;
  }
  // The seeded synthetic graphs (fixtures/synthetic-*.json), drawn by the
  // fixture page as /?graph=<name>.
  const graph=/^\/fixtures\/(synthetic-[a-z0-9-]+)\.json$/.exec(path);
  if(graph){
    try{response.writeHead(200,{'Content-Type':'application/json','Cache-Control':'no-store'}).end(await readFile(new URL(`../fixtures/${graph[1]}.json`,here)));}
    catch{response.writeHead(404).end();}
    return;
  }
  const journey=/^\/journey-(\d+)\.html$/.exec(path);
  if(journey&&journeyReports[Number(journey[1])]){
    try{response.writeHead(200,{'Content-Type':'text/html','Cache-Control':'no-store'}).end(await readFile(journeyReports[Number(journey[1])]));}
    catch(error){response.writeHead(500).end(String(error));}
    return;
  }
  const geometry=/^\/geometry-(\d+)\.html$/.exec(path);
  if(geometry&&geometryReports[Number(geometry[1])]){
    try{response.writeHead(200,{'Content-Type':'text/html','Cache-Control':'no-store'}).end(await readFile(geometryReports[Number(geometry[1])]));}
    catch(error){response.writeHead(500).end(String(error));}
    return;
  }
  if(path==='/real-report.html'&&process.env.REPOMAP_REAL_RUN){
    try{response.writeHead(200,{'Content-Type':'text/html','Cache-Control':'no-store'}).end(await renderReal());}
    catch(error){response.writeHead(500).end(String(error));}
    return;
  }
  const asset=routes.get(path);
  if(!asset){response.writeHead(404).end();return;}
  response.writeHead(200,{'Content-Type':asset[0], 'Cache-Control':'no-store'}).end(asset[1]);
}).listen(Number(process.env.REPOMAP_TEST_PORT||8875),'127.0.0.1');
