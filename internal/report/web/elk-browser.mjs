import ELK from 'elkjs/lib/elk-api.js';
import packedWorker from 'elkjs/lib/elk-worker.min.js';

// build.mjs imports the worker once into the self-contained report, gzipped
// and in base64. The worker unpacks its own source and runs it; the layout
// requests posted meanwhile wait and are handed to it in order. Keep its URL
// for this document: later diagrams can start their own worker without
// another source copy or a network asset.
const unpack=`const waiting=[];self.onmessage=event=>waiting.push(event);
(async()=>{
  const bytes=Uint8Array.from(atob(${JSON.stringify(packedWorker)}),c=>c.charCodeAt(0));
  const source=await new Response(new Blob([bytes]).stream().pipeThrough(new DecompressionStream('gzip'))).text();
  importScripts(URL.createObjectURL(new Blob([source],{type:'text/javascript'})));
  for(const event of waiting)self.onmessage(event);
})();`;
let workerURL;
function createWorker() {
  workerURL ||= URL.createObjectURL(new Blob([unpack], {type:'text/javascript'}));
  return new Worker(workerURL, {name:'repomap-layout'});
}

export default class BrowserELK extends ELK {
  constructor(options = {}) {
    let worker;
    super({...options, workerFactory:()=>worker=createWorker()});
    this.pending=new Set();
    const fail=event=>{
      event.preventDefault();
      if(this.failed)return;
      this.failed=event.error instanceof Error ? event.error
        :new Error(event.message||'The map layout worker failed.');
      worker.terminate();
      for(const reject of this.pending)reject(this.failed);
      this.pending.clear();
    };
    worker.addEventListener('error',fail);
    worker.addEventListener('messageerror',fail);
  }

  layout(...args) {
    if(this.failed)return Promise.reject(this.failed);
    return new Promise((resolve,reject)=>{
      this.pending.add(reject);
      super.layout(...args).then(
        result=>{this.pending.delete(reject);resolve(result);},
        error=>{this.pending.delete(reject);reject(error);},
      );
    });
  }
}
