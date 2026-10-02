// What a cell of the invariant table says (invariants.spec.mjs writes it,
// invariant-table.mjs prints it; both read it here so JSON and Markdown say
// one thing):
//   PASS            its checks ran and none failed;
//   FAIL            a check failed (whether or not its level finished);
//   NOT_APPLICABLE  its phase ran and found nothing of its kind, as the
//                   invariant declares (invariants.mjs `none`), or the
//                   invariant is checked at another kind of level only;
//   INCOMPLETE      its phase did not finish (an exception stopped the
//                   level, or the run), or no check ran and nothing was
//                   declared: zero checks are never a pass.
export const PASS='PASS',FAIL='FAIL',NOT_APPLICABLE='NOT_APPLICABLE',INCOMPLETE='INCOMPLETE';

// `cell` {checked, failed, notApplicable?}; `phaseDone` whether the phase
// holding its checks finished; `error` what stopped the level, if anything.
export function cellStatus(cell,{phaseDone,error=''}={}){
  if((cell?.failed||0)>0)return {status:FAIL};
  if(!phaseDone)return {status:INCOMPLETE,reason:error?`stopped before its checks finished: ${error}`:'its checks did not run'};
  if((cell?.checked||0)>0)return {status:PASS};
  if(cell?.notApplicable)return {status:NOT_APPLICABLE,reason:cell.notApplicable};
  return {status:INCOMPLETE,reason:'no check ran and none was declared not applicable'};
}

// A cell as a table written before statuses has it (the saved tables of
// 21bb81f2 and c5f6163b): a found failure is a FAIL and a check that ran
// with none failing a PASS; zero checks were never declared, so they are
// INCOMPLETE, and so is every unfailed cell of a level an exception stopped.
export function legacyStatus(cell,level){
  if((cell?.failed||0)>0)return {status:FAIL};
  if(level?.info?.error)return {status:INCOMPLETE,reason:`stopped: ${level.info.error}`};
  if((cell?.checked||0)>0)return {status:PASS};
  return {status:INCOMPLETE,reason:'recorded before statuses: no check ran and none was declared not applicable'};
}

// A cell's status as written, else as a legacy table implies.
export const statusOf=(cell,level)=>cell?.status?{status:cell.status,reason:cell.reason||''}:legacyStatus(cell,level);

// A level is complete when no exception stopped it and every cell has a
// status other than INCOMPLETE; a run when every level is and the run
// itself raised nothing.
export const levelComplete=level=>level.complete!==false&&!level.info?.error&&Object.values(level.invariants||{}).every(cell=>statusOf(cell,level).status!==INCOMPLETE);
export const runComplete=run=>run.complete!==false&&!run.error&&(run.levels||[]).every(levelComplete);

// What strict acceptance refuses: every FAIL and INCOMPLETE cell, every
// level an exception stopped and a run that did not finish.
export function strictProblems(run){
  const out=[];
  if(run.error||run.complete===false)out.push(`${run.repo}: the run stopped: ${run.error||'incomplete'}`);
  for(const level of run.levels||[]){
    if(level.info?.error||level.complete===false)out.push(`${level.name}: INCOMPLETE, stopped: ${level.info?.error||'incomplete'}`);
    for(const [name,cell] of Object.entries(level.invariants||{})){
      const {status,reason}=statusOf(cell,level);
      if(status===FAIL)out.push(`${level.name} · ${name}: FAIL ${cell.failed}/${cell.checked}: ${cell.examples?.[0]||''}`);
      else if(status===INCOMPLETE)out.push(`${level.name} · ${name}: INCOMPLETE (${reason})`);
    }
  }
  return out;
}
