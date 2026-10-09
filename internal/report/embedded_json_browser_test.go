package report

import (
	"encoding/json"
	"errors"
	"html/template"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Real Chromium executes the inserted classic script; a throwing replaceWith
// stand-in cannot prove that syntax errors or blocked scripts refuse readiness.
func TestEmbeddedJSONBrowserBootPreservesBytesGlobalsAndRefusesFailures(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node required")
	}
	web, err := filepath.Abs("web")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(web, "node_modules", "playwright")); err != nil {
		t.Skip("Playwright required")
	}
	value := `{"source":"東京 & Москва.cljc:17:4","refs":[` + strings.TrimSuffix(strings.Repeat(`"t16",`, 2000), ",") + `]}`
	payload, err := embeddedJSON("rm-page-data", template.JS(value))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), "gzip-base64") {
		t.Fatal("large complete browser input is not compressed")
	}
	boot, err := embeddedReportBoot()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	config, _ := json.Marshal(map[string]string{"payload": string(payload), "boot": string(boot), "expected": value})
	input, script := filepath.Join(dir, "input.json"), filepath.Join(dir, "boot.mjs")
	if err := os.WriteFile(input, config, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte(embeddedBootBrowserScript), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.CommandContext(t.Context(), node, script, filepath.Join(web, "package.json"), input).CombinedOutput()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 3 {
		t.Skipf("headless Chromium unavailable: %s", out)
	}
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

const embeddedBootBrowserScript = `
import {createRequire} from 'node:module';import http from 'node:http';import fs from 'node:fs';import assert from 'node:assert/strict';import {createHash} from 'node:crypto';
const [pkg,input]=process.argv.slice(2),config=JSON.parse(fs.readFileSync(input));
const {chromium}=createRequire(pkg)('playwright');
let browser;try{browser=await chromium.launch();}catch(e){console.log(String(e).slice(0,200));process.exit(3);}
const applications={normal:'var bootClassicGlobal=(window.bootClassicGlobal||0)+1;window.loadedBeyondLoad=document.readyState==="complete";window.savedDeepHash=location.hash;',throws:'window.attempts=1;throw new Error("startup rejected");',syntax:'var =;',blocked:'window.attempts=1;'};
const bootHash=createHash('sha256').update(config.boot).digest('base64');
const server=http.createServer((q,r)=>{
  const kind=q.url.split('?')[0].slice(1);let data=config.payload;
  if(kind==='corrupt')data=data.replace(/>([^<]*)<\/script>/,'>not-base64!<\/script>');
  if(kind==='crc'||kind==='truncated')data=data.replace(/>([^<]*)<\/script>/,(_,encoded)=>{let bytes=Buffer.from(encoded,'base64');if(kind==='crc')bytes[bytes.length-8]^=1;else bytes=bytes.subarray(0,bytes.length-8);return '>'+bytes.toString('base64')+'<\/script>';});
  const headers={'content-type':'text/html; charset=utf-8'};
  if(kind==='blocked')headers['content-security-policy']="script-src 'sha256-"+bootHash+"'";
  r.writeHead(200,headers).end('<!doctype html><body><p id="rm-load-status" data-error="Could not open report">Opening report</p>'+data+'<script type="application/x-repomap-js" id="rm-report-app-js">'+(applications[kind]||applications.normal)+'</script><script>'+config.boot+'</script>');
});
await new Promise(r=>server.listen(0,'127.0.0.1',r));
try{
  for(const kind of ['normal','corrupt','crc','truncated','unavailable','throws','syntax','blocked']){
    const page=await browser.newPage();
    if(kind==='normal')await page.addInitScript(()=>{
      const Native=DecompressionStream;
      window.DecompressionStream=class{constructor(format){const real=new Native(format);return {writable:real.writable,readable:real.readable.pipeThrough(new TransformStream({async transform(chunk,control){await new Promise(r=>setTimeout(r,100));control.enqueue(chunk);}}))};}};
    });
    if(kind==='unavailable')await page.addInitScript(()=>{window.DecompressionStream=undefined;});
    await page.goto('http://127.0.0.1:'+server.address().port+'/'+kind+'#original-part',{waitUntil:'load'});
    await page.waitForFunction(()=>['ready','failed'].includes(document.body.dataset.reportLoadState));
    const state=await page.evaluate(()=>({state:document.body.dataset.reportLoadState,bytes:document.getElementById('rm-page-data').textContent,hidden:document.getElementById('rm-load-status').hidden,error:document.getElementById('rm-load-status').textContent,count:window.bootClassicGlobal||0,afterLoad:window.loadedBeyondLoad,hash:window.savedDeepHash,attempts:window.attempts||0}));
    if(kind==='normal'){
      assert.equal(state.state,'ready');assert.equal(state.bytes,config.expected);assert.equal(state.count,1);assert.equal(state.afterLoad,true);assert.equal(state.hash,'#original-part');assert.equal(state.hidden,true);
    }else{
      assert.equal(state.state,'failed',kind+' must not claim readiness');assert.equal(state.hidden,false);assert.equal(state.error,'Could not open report');assert.equal(state.count,0);
      if(['blocked','corrupt','crc','truncated','unavailable'].includes(kind))assert.equal(state.attempts,0);
    }
    await page.close();
  }
  console.log('complete bytes, delayed load, classic globals, single execution and seven failure modes passed');
}finally{await browser.close();await new Promise(r=>server.close(r));}
`
