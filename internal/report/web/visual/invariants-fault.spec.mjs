import {test,expect} from '@playwright/test';
import {spawn,execFile} from 'node:child_process';
import {mkdtemp,readFile,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {fileURLToPath} from 'node:url';

// The invariant table stays honest when its own pass breaks: a harness
// failure forced in invariants.spec.mjs (REPOMAP_INVARIANT_FAULT), with no
// error in the page, before the first check of the whole map and after a
// few of them, fails strict acceptance, and the run's JSON and the table's
// JSON and Markdown all say INCOMPLETE. Each case runs the real spec once on
// the fixture's synthetic graph, beside this suite on the next port.
const web=fileURLToPath(new URL('../',import.meta.url));
const port=Number(process.env.REPOMAP_TEST_PORT||8875)+1;
const playwright=fileURLToPath(new URL('../node_modules/.bin/playwright',import.meta.url));

function run(env){
  return new Promise(resolve=>{
    const child=spawn(playwright,['test','visual/invariants.spec.mjs','--workers=1','--reporter=line'],
      {cwd:web,env:{...process.env,...env,REPOMAP_TEST_PORT:String(port)},stdio:['ignore','pipe','pipe']});
    let output='';child.stdout.on('data',d=>output+=d);child.stderr.on('data',d=>output+=d);
    child.on('close',code=>resolve({code,output}));
  });
}

for(const [fault,when] of [['before-checks','before the first check'],['after-checks','after a few successful checks']])
  test(`a harness failure ${when} fails strict acceptance and reads INCOMPLETE in every output`,async()=>{
    test.setTimeout(240_000);
    const dir=await mkdtemp(join(tmpdir(),'repomap-invariant-fault-'));
    try{
      const {code,output}=await run({REPOMAP_INVARIANT_FAULT:fault,REPOMAP_INVARIANT_STRICT:'1',REPOMAP_INVARIANT_GRAPHS:'no-inputs',
        REPOMAP_INVARIANT_LEVELS:'^home$',REPOMAP_INVARIANT_PANS:'2',REPOMAP_INVARIANT_MOVES:'2',REPOMAP_INVARIANT_OUT:dir,
        REPOMAP_INVARIANT_REPORTS:'',REPOMAP_INVARIANT_PATHS:'canvas',REPOMAP_INVARIANT_HEAD:'fault-test'});
      expect(code,`strict acceptance fails:\n${output.slice(-1500)}`).not.toBe(0);
      expect(output).toMatch(/INCOMPLETE/);
      const saved=JSON.parse(await readFile(join(dir,'synthetic-no-inputs.canvas.json'),'utf8'));
      const [home]=saved.levels;
      expect(saved.complete).toBe(false);
      expect(home.complete).toBe(false);
      expect(home.info.error).toMatch(/Forced harness failure/);
      const statuses=Object.entries(home.invariants).map(([name,cell])=>[name,cell.status]);
      expect(statuses.filter(([,status])=>status==='PASS').map(([name])=>name),'nothing passes but the page-errors check').toEqual(['page-errors']);
      expect(statuses.filter(([,status])=>status==='INCOMPLETE').length).toBeGreaterThan(20);
      if(fault==='after-checks')expect(home.invariants['one-path'].checked,'the arrows at rest were checked before the stop').toBeGreaterThan(0);
      await new Promise((resolve,reject)=>execFile(process.execPath,['visual/invariant-table.mjs',dir],{cwd:web},error=>error?reject(error):resolve()));
      const table=JSON.parse(await readFile(join(dir,'table.json'),'utf8')),markdown=await readFile(join(dir,'table.md'),'utf8');
      const level=table.runs[0].levels[0];
      expect(table.runs[0].complete).toBe(false);expect(level.complete).toBe(false);
      for(const [name,cell] of Object.entries(level.invariants))expect(cell.pass,`${name}: pass says what status does`).toBe(cell.status==='PASS');
      expect(Object.entries(level.invariants).filter(([,cell])=>cell.status==='INCOMPLETE').length).toBeGreaterThan(20);
      expect(markdown).toContain('home (**INCOMPLETE**: Forced harness failure');
      expect(markdown).toContain('### Incomplete');
    }finally{await rm(dir,{recursive:true,force:true});}
  });
