import {mkdir,readFile,writeFile} from 'node:fs/promises';

const escape=value=>String(value).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));

// The normal Playwright report owns comparisons and traces. This additional
// view lets a reader scroll through actual journey PNGs without opening steps.
export default class JourneyReporter {
  journeys=[];
  async onTestEnd(test,result){
    const frames=[];
    for(const attachment of result.attachments){
      if(attachment.contentType!=='image/png'||!attachment.name.startsWith('journey-')||!attachment.name.includes(' — '))continue;
      const png=attachment.body||await readFile(attachment.path);
      frames.push({name:attachment.name,src:'data:image/png;base64,'+png.toString('base64')});
    }
    const failure=result.status!=='passed'&&result.attachments.find(a=>a.name==='screenshot'&&a.contentType==='image/png');
    if(frames.length&&failure){
      const png=failure.body||await readFile(failure.path);
      frames.push({name:'Failure — final captured state',src:'data:image/png;base64,'+png.toString('base64')});
    }
    if(frames.length)this.journeys.push({name:test.parent.project().name,status:result.status,frames});
  }
  async onEnd(){
    if(!this.journeys.length)return;
    this.journeys.sort((a,b)=>a.name.localeCompare(b.name));
    const sections=this.journeys.map(journey=>`<section><h2>${escape(journey.name)} · ${escape(journey.status)}</h2>
      ${journey.frames.map(frame=>`<figure><figcaption>${escape(frame.name)}</figcaption>
        <img src="${frame.src}" alt="${escape(frame.name)}" loading="lazy"></figure>`).join('\n')}</section>`).join('\n');
    const output=new URL('../playwright-report/',import.meta.url);
    await mkdir(output,{recursive:true});
    await writeFile(new URL('journey.html',output),`<!doctype html><html lang="en"><meta charset="utf-8">
      <meta name="viewport" content="width=device-width,initial-scale=1"><title>Canvas screenshot journeys</title>
      <style>body{font:16px/1.5 system-ui;margin:24px auto;padding:0 20px;max-width:1120px;color:#243044;background:#f6f8fa}
      h1{font-size:28px}h2{margin-top:48px}figure{margin:28px 0}figcaption{margin-bottom:8px;font-weight:600}
      img{display:block;width:100%;height:auto;border:1px solid #d4ddda;background:white}a{color:#176f67}</style>
      <h1>Canvas screenshot journeys</h1><p>Actual PNG screenshots, in action order. The pink ring marks the gesture’s aim.
      A failed journey is diagnostic output, not an accepted reference.</p>
      <p><a href="./index.html">Full test report, reference images and differences →</a></p>${sections}</html>`);
  }
}
