import {createServer} from 'node:http';
import {readFile, readdir} from 'node:fs/promises';

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
// This test-only server exposes only this fixture and ordinary report assets.
createServer((request,response)=>{
  const asset=routes.get(new URL(request.url,'http://127.0.0.1').pathname);
  if(!asset){response.writeHead(404).end();return;}
  response.writeHead(200,{'Content-Type':asset[0], 'Cache-Control':'no-store'}).end(asset[1]);
}).listen(8875,'127.0.0.1');
