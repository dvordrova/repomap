// The invariant table: repo × level × invariant, pass or fail with counts,
// from the <repo>.<path>.json files invariants.spec.mjs writes (a run's
// `note`, when set, is printed with it), each cell marked with the commit it
// ran on.
//   node visual/invariant-table.mjs DIR [--title TEXT] [--not-run NAMES] [--why TEXT]
//     [--head SHA] [--overlay DIR2 --overlay-cells red,home:text]
// writes DIR/table.json and DIR/table.md; --not-run lists the targets left
// out (a comma list), --why says why; --head names the commit of runs that
// do not say theirs. --overlay lays a later pass over the table: its
// repositories missing from DIR join whole, and in the others only the
// named cells of the levels it ran are replaced (`red`: the cells failing in
// DIR; `LEVEL:INVARIANT`: that cell, `*` for any level), each keeping the
// commit it ran on.
import {readdir,readFile,writeFile} from 'node:fs/promises';
import {join} from 'node:path';
import {invariants} from './invariants.mjs';

const [dir,...rest]=process.argv.slice(2);
if(!dir){console.error('usage: node visual/invariant-table.mjs DIR [--title TEXT] [--not-run NAMES] [--why TEXT]');process.exit(2);}
const flag=name=>{const at=rest.indexOf(name);if(at<0)return '';const out=[];for(let i=at+1;i<rest.length&&!rest[i].startsWith('--');i++)out.push(rest[i]);return out.join(' ');};
const title=flag('--title')||'Canvas invariants',notRun=flag('--not-run').split(',').map(n=>n.trim()).filter(Boolean),why=flag('--why')||'not run';
const read=async from=>{const out=[];for(const name of (await readdir(from)).filter(n=>/\.(old|scene|canvas)\.json$/.test(n)).sort())out.push(JSON.parse(await readFile(join(from,name),'utf8')));return out;};
const runs=await read(dir);
const baseHead=flag('--head');
for(const run of runs){run.head||=baseHead;for(const level of run.levels)for(const cell of Object.values(level.invariants))cell.head||=run.head;}
// A later pass laid over the table.
const overlayDir=flag('--overlay'),overlayCells=flag('--overlay-cells').split(',').map(t=>t.trim()).filter(Boolean);
if(overlayDir)for(const later of await read(overlayDir)){
  later.head||='';
  for(const level of later.levels)for(const cell of Object.values(level.invariants))cell.head||=later.head;
  const base=runs.find(run=>run.repo===later.repo&&run.path===later.path);
  if(!base){runs.push(later);continue;}
  base.overlays=(base.overlays||0);
  for(const level of later.levels){
    const at=base.levels.find(l=>l.name===level.name);if(!at)continue;
    for(const [name] of invariants){
      const wanted=overlayCells.some(token=>token==='red'?at.invariants[name]?.failed>0:(([where,what])=>(where==='*'||where===level.name)&&what===name)(token.split(':')));
      if(!wanted||!level.invariants[name])continue;
      at.invariants[name]={...level.invariants[name],was:at.invariants[name]};base.overlays++;
    }
  }
}
runs.sort((a,b)=>a.repo.localeCompare(b.repo));

// A cell run on another commit than its repository's is marked †.
const cell=(r,head)=>(!r||!r.checked?'–':r.failed?`**${r.failed}**/${r.checked}`:`ok ${r.checked}`)+(r&&r.head&&head&&r.head!==head?' †':'');
const lines=[`# ${title}`,'',`Generated ${new Date().toISOString().slice(0,16).replace('T',' ')} UTC from ${runs.length} report runs.`,
  'A cell is `ok N` (N checks, all passed), `**F**/N` (F of N failed) or `–` (nothing to check at that level).','',
  '## Invariants','',...invariants.map(([name,says])=>`- \`${name}\`: ${says}`),''];
const table={generated:new Date().toISOString(),invariants:Object.fromEntries(invariants),runs:[]};
for(const path of ['old','scene','canvas']){
  const set=runs.filter(run=>run.path===path);if(!set.length)continue;
  lines.push(`## ${({old:'Old path',scene:'`?scene=1`',canvas:'Canvas (no flag)'})[path]}`,'');
  const notRead=set.filter(run=>path!=='old'&&!run.sceneOn).map(run=>run.repo);
  if(notRead.length)lines.push(`The page did not draw the scene canvas for ${notRead.join(', ')}: its bundle drew the old path.`,'');
  // Summary: failing levels per repo and invariant.
  lines.push('### Levels failing, by repository','',`| repo | levels | ${invariants.map(([n])=>n).join(' | ')} |`,`|---|---|${invariants.map(()=>'---').join('|')}|`);
  for(const run of set){
    const row=invariants.map(([n])=>{const checked=run.levels.filter(l=>l.invariants[n]?.checked).length,failed=run.levels.filter(l=>l.invariants[n]?.failed).length;return !checked?'–':failed?`**${failed}**/${checked}`:`ok ${checked}`;});
    lines.push(`| ${run.repo} | ${run.levels.length} | ${row.join(' | ')} |`);
  }
  for(const name of notRun)lines.push(`| ${name} | – | ${invariants.map(()=>'–').join(' | ')} |`);
  lines.push('');
  for(const run of set.filter(run=>run.head))lines.push(`- ${run.repo}: ${run.levels.length} levels on ${run.head}${run.overlays?`; ${run.overlays} cells marked † re-run on ${[...new Set(run.levels.flatMap(l=>Object.values(l.invariants).map(c=>c.head)).filter(h=>h&&h!==run.head))].join(', ')}`:''}`);
  lines.push('');
  if(notRun.length)lines.push(`Not run: ${notRun.join(', ')} (${why}).`,'');
  for(const run of set.filter(run=>run.note))lines.push(`${run.repo}: ${run.note}`,'');
  lines.push('### Every level','',`| repo | level | ${invariants.map(([n])=>n).join(' | ')} |`,`|---|---|${invariants.map(()=>'---').join('|')}|`);
  for(const run of set)for(const level of run.levels)
    lines.push(`| ${run.repo} | ${level.name.replace(/\|/g,'\\|')}${level.info?.error?` (error: ${level.info.error.replace(/\|/g,'\\|').slice(0,80)})`:''} | ${invariants.map(([n])=>cell(level.invariants[n],run.head)).join(' | ')} |`);
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
  for(const run of set)table.runs.push({repo:run.repo,path,head:run.head||'',sceneOn:run.sceneOn,file:run.file,note:run.note||'',levels:run.levels.map(l=>({name:l.name,kind:l.kind,id:l.id,info:l.info,
    invariants:Object.fromEntries(invariants.map(([n])=>[n,{checked:l.invariants[n]?.checked||0,failed:l.invariants[n]?.failed||0,pass:!l.invariants[n]?.failed,head:l.invariants[n]?.head||run.head||'',
      examples:l.invariants[n]?.examples||[],...l.invariants[n]?.was?{was:{checked:l.invariants[n].was.checked,failed:l.invariants[n].was.failed,head:l.invariants[n].was.head,examples:l.invariants[n].was.examples}}:{}}]))}))});
}
table.notRun=notRun.map(repo=>({repo,why}));
await writeFile(join(dir,'table.json'),JSON.stringify(table,null,1));
await writeFile(join(dir,'table.md'),lines.join('\n')+'\n');
console.log(`${join(dir,'table.md')}: ${table.runs.length} runs, ${table.runs.reduce((s,r)=>s+r.levels.length,0)} levels`);
