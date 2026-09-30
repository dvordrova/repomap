package report

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The reading column prints no place in the code (owner, 2026-09-29: "a
// person will see the code"; reviewer, 2026-09-30: State changes had
// printed "adlist.c:86" under each field) and cuts no sentence mid-word
// (an area's part descriptions had ended "parses comman…"). Every reading
// of the fixture page — its component, frames, parts, inputs and each
// declaration of each part — is read in headless Chromium with every fold
// open; no line of the column's text reads "file.ext:line", and no element
// holding words is cut by an ellipsis. REPOMAP_COLUMN_REPORTS names
// rendered reports to read the same way, the first declarations of each
// part (`repomap render` of saved runs; the fixture page draws no part
// reading of its own). Skipped without Node and the report's Playwright
// (internal/report/web).
func TestTheColumnPrintsNoPlaceAndCutsNoWords(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node required")
	}
	web, err := filepath.Abs("web")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(web, "node_modules", "playwright")); err != nil {
		t.Skip("Playwright required (npm ci in internal/report/web)")
	}
	data := reportProgramShellDataFixture(t, "fixture")
	html, err := RenderHTMLWithOptions(&data, reportSingleTargetRenderOptionsFixture(t, &data))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	page, script := filepath.Join(dir, "report.html"), filepath.Join(dir, "walk.mjs")
	if err := os.WriteFile(page, html, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte(columnWalkScript), 0o600); err != nil {
		t.Fatal(err)
	}
	pages := map[string]string{"fixture": page}
	for _, path := range strings.Split(os.Getenv("REPOMAP_COLUMN_REPORTS"), ",") {
		if path != "" {
			pages[filepath.Base(path)] = path
		}
	}
	for name, path := range pages {
		t.Run(name, func(t *testing.T) {
			limit := "40"
			if name != "fixture" {
				limit = "3"
			}
			out, err := exec.CommandContext(t.Context(), node, script, filepath.Join(web, "package.json"), path, limit).CombinedOutput()
			var exit *exec.ExitError
			if errors.As(err, &exit) && exit.ExitCode() == 3 {
				t.Skipf("headless Chromium unavailable: %s", out)
			}
			if err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
			if said := string(out); !strings.Contains(said, "readings ") || strings.Contains(said, "readings 0 ") {
				t.Fatalf("the walk read nothing:\n%s", said)
			}
		})
	}
}

// columnWalkScript reads every reading of a report page and exits 1 on a
// place printed in the column or words cut by an ellipsis, 3 when Chromium
// cannot start.
const columnWalkScript = `
import {createRequire} from 'node:module';import http from 'node:http';import fs from 'node:fs';
const [pkg,file,limit]=process.argv.slice(2);
const {chromium}=createRequire(pkg)('playwright');
let browser;try{browser=await chromium.launch();}catch(e){console.log(String(e).slice(0,200));process.exit(3);}
const server=http.createServer((q,r)=>{r.writeHead(200,{'content-type':'text/html; charset=utf-8'});fs.createReadStream(file).pipe(r);});
await new Promise(r=>server.listen(0,'127.0.0.1',r));
const page=await browser.newPage({viewport:{width:1440,height:900}});
const errors=[];page.on('pageerror',e=>errors.push(e.message));
await page.goto('http://127.0.0.1:'+server.address().port+'/report.html');
page.setDefaultTimeout(600000);
await page.waitForFunction(()=>{const m=document.querySelector('[data-map]');return m&&m.classList.contains('flow-enabled')&&!m.classList.contains('flow-initializing');},null,{timeout:60000});
const found=await page.evaluate(async limit=>{
  const wait=ms=>new Promise(r=>setTimeout(r,ms));
  const map=document.querySelector('[data-map-explorer]'),nodes=[...map.querySelectorAll('[data-node]')];
  const bad=[];let readings=0;
  function scan(where){
    const column=document.querySelector('.map-inspector-content');if(!column)return;
    for(let i=0;i<3;i++)column.querySelectorAll('details:not([open])').forEach(d=>d.open=true);
    readings++;
    // A file and a line ("adlist.c:86"), never an address ("127.0.0.1:4222").
    column.innerText.split('\n').forEach(line=>{if(/\S+\.[A-Za-z]\w*:\d+/.test(line))bad.push(where+': a place: '+line.trim());});
    for(const e of column.querySelectorAll('*')){
      if(!e.getClientRects().length)continue;
      if(getComputedStyle(e).textOverflow==='ellipsis'&&e.scrollWidth>e.clientWidth+1&&/\S\s+\S/.test(e.textContent.trim()))bad.push(where+': words cut: '+e.textContent.trim());
    }
  }
  for(const n of nodes){
    const where=(n.dataset.branch||(n.dataset.activation?'input':'part'))+' '+n.dataset.title;
    if(n.dataset.branch==='component')map.selectComponent(n.dataset.owner);
    else if(n.dataset.activation&&map.chooseOperation)map.chooseOperation(n.id);
    else map.goToLevel({id:n.id,kind:'frame'});
    await wait(150);scan(where);
    if(n.dataset.branch||n.dataset.activation)continue;
    const keys=[...document.querySelectorAll('.map-inspector-content .map-part-reading .map-reading-name')].map(a=>a.dataset.declKey).filter(Boolean);
    for(const key of keys.slice(0,limit)){
      map.goToLevel({id:n.id,kind:'frame'});await wait(80);
      const name=[...document.querySelectorAll('.map-inspector-content .map-part-reading .map-reading-name')].find(a=>a.dataset.declKey===key);
      if(!name)continue;name.click();await wait(100);scan(where+' / '+name.textContent);
    }
  }
  return {readings,bad};
},Number(limit||40));
console.log('readings '+found.readings+' ');
await browser.close();server.close();
if(errors.length){console.log('page errors: '+errors.join('\n'));process.exit(1);}
if(found.bad.length){console.log(found.bad.slice(0,20).join('\n'));process.exit(1);}
`
