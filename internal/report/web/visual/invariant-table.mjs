// The invariant table: repo × level × invariant, pass or fail with counts,
// from the <repo>.<path>.json files invariants.spec.mjs writes (a run's
// `note`, when set, is printed with it).
//   node visual/invariant-table.mjs DIR [--title TEXT] [--not-run NAMES] [--why TEXT]
// writes DIR/table.json and DIR/table.md; --not-run lists the targets left
// out (a comma list), --why says why.
import {readdir,readFile,writeFile} from 'node:fs/promises';
import {join} from 'node:path';
import {invariants} from './invariants.mjs';

const [dir,...rest]=process.argv.slice(2);
if(!dir){console.error('usage: node visual/invariant-table.mjs DIR [--title TEXT] [--not-run NAMES] [--why TEXT]');process.exit(2);}
const flag=name=>{const at=rest.indexOf(name);if(at<0)return '';const out=[];for(let i=at+1;i<rest.length&&!rest[i].startsWith('--');i++)out.push(rest[i]);return out.join(' ');};
const title=flag('--title')||'Canvas invariants',notRun=flag('--not-run').split(',').map(n=>n.trim()).filter(Boolean),why=flag('--why')||'not run';
const runs=[];
for(const name of (await readdir(dir)).filter(n=>/\.(old|scene)\.json$/.test(n)).sort())runs.push(JSON.parse(await readFile(join(dir,name),'utf8')));

const cell=r=>!r||!r.checked?'–':r.failed?`**${r.failed}**/${r.checked}`:`ok ${r.checked}`;
const lines=[`# ${title}`,'',`Generated ${new Date().toISOString().slice(0,16).replace('T',' ')} UTC from ${runs.length} report runs.`,
  'A cell is `ok N` (N checks, all passed), `**F**/N` (F of N failed) or `–` (nothing to check at that level).','',
  '## Invariants','',...invariants.map(([name,says])=>`- \`${name}\`: ${says}`),''];
const table={generated:new Date().toISOString(),invariants:Object.fromEntries(invariants),runs:[]};
for(const path of ['old','scene']){
  const set=runs.filter(run=>run.path===path);if(!set.length)continue;
  lines.push(`## ${path==='old'?'Old path':'`?scene=1`'}`,'');
  const notRead=set.filter(run=>path==='scene'&&!run.sceneOn).map(run=>run.repo);
  if(notRead.length)lines.push(`The page did not report a scene level for ${notRead.join(', ')}: the bundle drew its old path under \`?scene=1\`.`,'');
  // Summary: failing levels per repo and invariant.
  lines.push('### Levels failing, by repository','',`| repo | levels | ${invariants.map(([n])=>n).join(' | ')} |`,`|---|---|${invariants.map(()=>'---').join('|')}|`);
  for(const run of set){
    const row=invariants.map(([n])=>{const checked=run.levels.filter(l=>l.invariants[n]?.checked).length,failed=run.levels.filter(l=>l.invariants[n]?.failed).length;return !checked?'–':failed?`**${failed}**/${checked}`:`ok ${checked}`;});
    lines.push(`| ${run.repo} | ${run.levels.length} | ${row.join(' | ')} |`);
  }
  for(const name of path==='old'?notRun:[])lines.push(`| ${name} | – | ${invariants.map(()=>'–').join(' | ')} |`);
  lines.push('');
  if(path==='old'&&notRun.length)lines.push(`Not run: ${notRun.join(', ')} (${why}).`,'');
  for(const run of set.filter(run=>run.note))lines.push(`${run.repo}: ${run.note}`,'');
  lines.push('### Every level','',`| repo | level | ${invariants.map(([n])=>n).join(' | ')} |`,`|---|---|${invariants.map(()=>'---').join('|')}|`);
  for(const run of set)for(const level of run.levels)
    lines.push(`| ${run.repo} | ${level.name.replace(/\|/g,'\\|')}${level.info?.error?` (error: ${level.info.error.replace(/\|/g,'\\|').slice(0,80)})`:''} | ${invariants.map(([n])=>cell(level.invariants[n])).join(' | ')} |`);
  lines.push('');
  // The first examples of each failing invariant, per repo.
  lines.push('### Examples','');
  for(const run of set){
    const failing=invariants.filter(([n])=>run.levels.some(l=>l.invariants[n]?.failed));
    if(!failing.length)continue;
    lines.push(`#### ${run.repo}`,'');
    for(const [n] of failing){
      const examples=run.levels.filter(l=>l.invariants[n]?.failed).flatMap(l=>l.invariants[n].examples.slice(0,2).map(e=>`${l.name}: ${e}`)).slice(0,4);
      lines.push(`- \`${n}\`: ${examples.map(e=>e.replace(/\|/g,'\\|')).join(' · ')}`);
    }
    lines.push('');
  }
  for(const run of set)table.runs.push({repo:run.repo,path,sceneOn:run.sceneOn,file:run.file,note:run.note||'',levels:run.levels.map(l=>({name:l.name,kind:l.kind,id:l.id,info:l.info,
    invariants:Object.fromEntries(invariants.map(([n])=>[n,{checked:l.invariants[n]?.checked||0,failed:l.invariants[n]?.failed||0,pass:!l.invariants[n]?.failed,examples:l.invariants[n]?.examples||[]}]))}))});
}
table.notRun=notRun.map(repo=>({repo,why}));
await writeFile(join(dir,'table.json'),JSON.stringify(table,null,1));
await writeFile(join(dir,'table.md'),lines.join('\n')+'\n');
console.log(`${join(dir,'table.md')}: ${table.runs.length} runs, ${table.runs.reduce((s,r)=>s+r.levels.length,0)} levels`);
