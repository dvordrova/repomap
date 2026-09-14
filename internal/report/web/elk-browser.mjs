import ELK from 'elkjs/lib/elk-api.js';
import workerSource from 'elkjs/lib/elk-worker.min.js';

// build.mjs imports the worker as text, once, into the self-contained report.
// Keep its URL for this document: later diagrams can start their own worker
// without another source copy or a network asset.
let workerURL;
function createWorker() {
  workerURL ||= URL.createObjectURL(new Blob([workerSource], {type:'text/javascript'}));
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
