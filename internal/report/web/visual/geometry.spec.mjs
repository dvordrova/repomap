import {test,expect} from '@playwright/test';
import {basename} from 'node:path';
import {lintReport,summary} from './geometry.mjs';

// The canvas's geometry at every level (visual/geometry.mjs): on the fixture
// always; on rendered reports named by REPOMAP_GEOMETRY_REPORTS (a comma
// list of report HTML files, rendered with `.bin/repomap render`), which the
// test server serves as /geometry-<n>.html. No provider call:
//   REPOMAP_GEOMETRY_REPORTS=a.html,b.html npx playwright test geometry
async function ready(page){
  const map=page.locator('[data-map]');
  await expect(map).toHaveClass(/flow-enabled/,{timeout:60_000});
  await expect(map).not.toHaveClass(/flow-initializing/,{timeout:60_000});
}

test('the fixture canvas keeps its texts whole, its boxes in their frames and its arrows clear at every level',async({page})=>{
  test.setTimeout(120_000);
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('/?described');
  await expect(page.locator('[data-fixture-ready]')).toHaveAttribute('data-fixture-ready','true');
  await ready(page);
  const findings=await lintReport(page,{repo:'fixture'});
  expect(errors,'the page raises no error').toEqual([]);
  expect(findings,summary(findings)).toEqual([]);
});

const reports=(process.env.REPOMAP_GEOMETRY_REPORTS||'').split(',').filter(Boolean);
for(const [index,file] of reports.entries())test(`the geometry of ${basename(file)}`,async({page})=>{
  test.setTimeout(600_000);
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto(`/geometry-${index}.html`);
  await ready(page);
  const findings=await lintReport(page,{repo:basename(file,'.html')});
  console.log(`${basename(file)}: ${summary(findings)}`);
  expect(errors,'the page raises no error').toEqual([]);
  expect(findings,summary(findings)).toEqual([]);
});
