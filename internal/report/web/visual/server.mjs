import {createServer} from 'node:http';
import {readFile, readdir, mkdtemp, rm} from 'node:fs/promises';
import {execFile} from 'node:child_process';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {fileURLToPath} from 'node:url';

const here = new URL('./', import.meta.url);
const templates = new URL('../../templates/', here);
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
// This test-only server exposes only this fixture, ordinary report assets and
// that rendered report.
createServer(async(request,response)=>{
  const path=new URL(request.url,'http://127.0.0.1').pathname;
  if(path==='/real-report.html'&&process.env.REPOMAP_REAL_RUN){
    try{response.writeHead(200,{'Content-Type':'text/html','Cache-Control':'no-store'}).end(await renderReal());}
    catch(error){response.writeHead(500).end(String(error));}
    return;
  }
  const asset=routes.get(path);
  if(!asset){response.writeHead(404).end();return;}
  response.writeHead(200,{'Content-Type':asset[0], 'Cache-Control':'no-store'}).end(asset[1]);
}).listen(8875,'127.0.0.1');
