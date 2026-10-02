// The invariant table never calls an unchecked cell a pass
// (visual/invariant-status.mjs, visual/invariant-table.mjs): a level pass an
// exception stopped is INCOMPLETE in its JSON and its Markdown alike, before
// any check and after a few, and strict acceptance refuses it.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import {execFile} from 'node:child_process';
import {mkdtemp,writeFile,readFile,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {fileURLToPath} from 'node:url';
import {invariants} from './visual/invariants.mjs';
import {cellStatus,legacyStatus,strictProblems,PASS,FAIL,NOT_APPLICABLE,INCOMPLETE} from './visual/invariant-status.mjs';

test('a cell is PASS only for checks that ran and finished; zero checks are not applicable only as declared',()=>{
  assert.equal(cellStatus({checked:3,failed:0},{phaseDone:true}).status,PASS);
  assert.equal(cellStatus({checked:3,failed:1},{phaseDone:true}).status,FAIL);
  assert.equal(cellStatus({checked:3,failed:1},{phaseDone:false,error:'boom'}).status,FAIL,'a found failure stays a failure');
  assert.equal(cellStatus({checked:3,failed:0},{phaseDone:false,error:'boom'}).status,INCOMPLETE,'checks before the stop are no pass');
  assert.equal(cellStatus({checked:0,failed:0},{phaseDone:true}).status,INCOMPLETE,'zero checks, nothing declared');
  assert.equal(cellStatus({checked:0,failed:0,notApplicable:'no marker in sight'},{phaseDone:true}).status,NOT_APPLICABLE);
  assert.equal(cellStatus({checked:0,failed:0,notApplicable:'no marker in sight'},{phaseDone:false}).status,INCOMPLETE,'a declaration counts only once its phase finished');
  assert.ok(invariants.every(([,,declared])=>declared?.phase&&declared.none),'every invariant declares its phase and when it does not apply');
});

test('a table written before statuses calls no zero-check cell a pass',()=>{
  const level={info:{error:'Forced harness failure before checks'}};
  for(const [name] of invariants)assert.notEqual(legacyStatus({checked:0,failed:0},level).status,PASS,name);
  assert.equal(legacyStatus({checked:0,failed:0},{info:{}}).status,INCOMPLETE);
  assert.equal(legacyStatus({checked:2,failed:0},{info:{}}).status,PASS);
});

// A run as the spec writes it when a level pass is stopped: `after` checks
// recorded before the stop (the arrows at rest), none when `after` is 0.
function stoppedRun(after){
  const invariantsOf={};
  for(const [name,,declared] of invariants){
    const checked=name==='page-errors'?1:declared.phase==='arrows'?after:0;
    invariantsOf[name]={checked,failed:0,examples:[],...cellStatus({checked,failed:0},{phaseDone:name==='page-errors',error:'Forced harness failure'})};
  }
  return {repo:`forced-${after}`,path:'canvas',head:'test',complete:false,levels:[{kind:'home',id:'',name:'home',complete:false,phases:['errors'],
    info:{error:'Forced harness failure',errorPhase:'at rest'},invariants:invariantsOf}]};
}

const table=fileURLToPath(new URL('./visual/invariant-table.mjs',import.meta.url));
const run=(file,args)=>new Promise((resolve,reject)=>execFile(process.execPath,[file,...args],(error,stdout,stderr)=>error?reject(new Error(stderr||error.message)):resolve(stdout)));

for(const [when,after] of [['before the first check',0],['after a few successful checks',3]])
  test(`a level stopped ${when} is INCOMPLETE in the JSON and the Markdown, and strict refuses it`,async()=>{
    const result=stoppedRun(after);
    assert.ok(strictProblems(result).some(line=>/INCOMPLETE/.test(line)),'strict refuses the stopped level');
    const dir=await mkdtemp(join(tmpdir(),'repomap-invariant-status-'));
    try{
      await writeFile(join(dir,`${result.repo}.canvas.json`),JSON.stringify(result));
      await run(table,[dir]);
      const json=JSON.parse(await readFile(join(dir,'table.json'),'utf8')),markdown=await readFile(join(dir,'table.md'),'utf8');
      const [level]=json.runs[0].levels,cells=Object.values(level.invariants);
      assert.equal(json.runs[0].complete,false);assert.equal(level.complete,false);
      assert.ok(cells.filter(c=>c.status===INCOMPLETE).length>=invariants.length-1,'every unfinished cell is INCOMPLETE');
      assert.ok(cells.every(c=>c.pass===(c.status===PASS)),'pass says the same as status');
      assert.deepEqual(cells.filter(c=>c.pass).length,1,'only page-errors, which was checked, passes');
      if(after)assert.equal(level.invariants['one-path'].status,INCOMPLETE,'checks before the stop are no pass');
      assert.match(markdown,/\*\*INCOMPLETE\*\*/);
      assert.match(markdown,/### Incomplete/);
      assert.match(markdown,new RegExp(`\\| ${result.repo} \\| home \\(\\*\\*INCOMPLETE\\*\\*: Forced harness failure\\)`));
      if(after)assert.match(markdown,/\*\*INCOMPLETE\*\* 3/,'the Markdown keeps what it checked before the stop');
      const row=markdown.split('### Every level')[1].split('\n').find(line=>line.startsWith(`| ${result.repo} | home`));
      assert.equal((row.match(/ ok \d/g)||[]).length,1,'no cell of the stopped level reads ok but page-errors');
    }finally{await rm(dir,{recursive:true,force:true});}
  });
