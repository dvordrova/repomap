import React from 'react';
import {countsHandlers, countsInputs, headingRows, briefCard} from './call-card.mjs';

const t=(...args)=>window.rmT(...args);
const stop=event=>event.stopPropagation();
// A name breaks only after a dot or a slash, never inside a word
// ("sql.Tx.Rollbac k"; 43-map-reading.css makes it a unit): a route
// ("/api/v1/backtest/history") had been clipped in its row.
// A callable written inline reads "anonymous function in
// ReplicateCommand.Run" (GroupsIndex names it "ReplicateCommand.Run
// (inline)").
const inline=text=>{const said=typeof text==='string'&&/^(.+) \(inline\)$/.exec(text);return said?t('anonymous function in {0}',said[1]):text;};
const dotted=text=>{text=inline(text);return typeof text==='string'&&/[./]/.test(text)?text.split(/(?<=[./])/).map((piece,i)=><React.Fragment key={i}>{i>0&&<wbr/>}{piece}</React.Fragment>):text;};
function Link({href,title,children}){
  children=dotted(children);
  return href?<a href={href} title={title||undefined} target="_blank" onClick={stop}>{children}</a>:<span title={title||undefined}>{children}</span>;
}
const modified=event=>event.metaKey||event.ctrlKey||event.shiftKey||event.altKey||event.button!==0;
// A declaration's name in the reading column reads that declaration in the
// report, as a click on its tile does; a modifier-click still opens the code
// it linked to. A name the report has no declaration for is only named.
// Without a chooser (the canvas's own card) a name stays the link it was.
// Redis's "anetTcpGeneri…" opened GitHub for a reader who meant to read it.
function Name({at,href,title,choose,children}){
  if(!choose)return <Link href={href} title={title}>{children}</Link>;
  children=dotted(children);
  if(!at||!choose.can(at.part,at.key))return <span title={title||undefined}>{children}</span>;
  const read=event=>{event.stopPropagation();if(href&&modified(event))return;event.preventDefault();choose.go(at.part,at.key);};
  return href?<a href={href} title={title||undefined} target="_blank" onClick={read}>{children}</a>
    :<button type="button" className="flow-card-name" title={title||undefined} onClick={read}>{children}</button>;
}
// The inputs of a row, each a name reading that input in the reading
// column (owner, 2026-09-29: "sync" and "slaveof" had been plain text).
function InputNames({refs,choose}){
  return <>{refs.map((ref,i)=><React.Fragment key={ref.id}>{i>0&&', '}{choose?.input
    ?<button type="button" className="flow-card-name" onClick={event=>{event.stopPropagation();choose.input(ref.id);}}>{ref.name}</button>
    :ref.name}</React.Fragment>)}</>;
}
// The words between a caller and its callee: an arrow for a call, the
// relation's own words for anything else ("passes callback"). Inputs taken
// in, not handled, say where ("declared in parseOptions"); an input its
// component takes in names no callee.
const verb=kind=>kind==='calls'||kind==='implemented in'?'→':kind==='declared in'?t('declared in'):kind==='looked up in'?t('looked up in'):kind==='input'?'':kind.replace(/_/g,' ');

// A card's total, with its unit where it counts handlers or inputs, not
// calls.
export function cardTotal(card){
  const inputs=countsInputs(card);
  return countsHandlers(card)?t('{0} handlers',card.total):inputs?t('{0} inputs',inputs):card.total;
}

// A fold says what its set is: one of the set a dispatch site calls, or the
// same set handed over whole, and how many of it this card holds. On the
// canvas (`counts` off) it says so without its numbers.
function foldWords(fold,counts=true){
  if(!counts)return fold.one?t('one of a set'):fold.same?t('the same set as {0}',fold.same):t('the same set');
  const here=fold.count<fold.of?` · ${t('{0} here',fold.count)}`:'';
  if(fold.one)return t('one of {0}',fold.of)+here;
  return (fold.same?t('the same {0} as {1}',fold.of,fold.same):t('the same {0}',fold.of))+here;
}

// The body of a call card, or of a connection in the reading column: its
// calls by the part they are made from, then by the part they go into,
// each heading a row of the list: sticky, the "→ VFS core" heading had been
// drawn over the rows scrolling under it (owner, 2026-09-29). A caller is
// written once for its run of calls; a fold is one line, its callees by
// part under it, each part opening to their names.
// A relation with no call of its own that would only name the heading above
// it again says nothing more in the column. On the canvas (`counts` off) no
// heading prints a count: the canvas says nothing in digits.
export function CallRows({card,choose=null,counts=true}){
  return <div className={`flow-card-groups ${choose?'flow-card-reading':''}`}>
    {card.groups.map(group=><section key={group.id||'-'} data-call-group={group.id}>
      {group.id&&<h4 className="flow-card-group"><span>{group.name}</span>{counts&&<b>{group.count}</b>}</h4>}
      {group.folds.map(fold=><div key={fold.caller+fold.fold} className="flow-card-fold">
        <p className="flow-card-row"><Name at={fold.callerAt} href={fold.site} choose={choose}>{fold.caller}</Name><i>{verb(fold.kind)}</i><span className="flow-card-say">{foldWords(fold,counts)}</span></p>
        <div className="flow-card-fold-parts">{fold.parts.map(part=><details key={part.id} onClick={stop}>
          <summary><span>{part.name}</span>{counts&&<b>{part.count}</b>}</summary>
          <p>{part.rows.map((row,i)=><React.Fragment key={i}>{i>0&&' '}<Name at={row.calleeAt} href={row.href} choose={choose}>{row.callee}</Name></React.Fragment>)}</p>
        </details>)}</div>
      </div>)}
      {group.pairs.map(pair=>{
        // In the column a row that would only name its heading again is not
        // repeated (headingRows).
        const rows=choose?headingRows(pair,group):pair.rows;
        return <div key={pair.id} className="flow-card-pair-rows">
          <h5 className="flow-card-pair"><span><i>→</i> {pair.name}</span>{counts&&<b>{pair.count}</b>}</h5>
          {rows.length>0&&<div className="flow-card-rows">{rows.map((row,i)=>{
            if(row.kind==='other'){
              if(!row.at&&!row.otherHref&&(row.other===pair.name||row.other===group.name))return null;
              return <p key={i} className="flow-card-row flow-card-other">{choose?<span>{dotted(row.other)}</span>:<Link href={row.otherHref}>{row.other}</Link>}{!choose&&row.at&&<em>{row.at}</em>}</p>;
            }
            if(row.sides)return <SidesRow key={i} row={row} choose={choose}/>;
            const again=i>0&&rows[i-1].caller===row.caller&&rows[i-1].kind!=='other';
            return <p key={i} className="flow-card-row"><span className={again?'flow-card-again':''}>{row.inputs?<InputNames refs={row.inputRefs} choose={choose}/>:<Name at={row.callerAt} href={row.site} title={row.at} choose={choose}>{row.caller}</Name>}</span>
              {verb(row.kind)&&<i>{verb(row.kind)}</i>}{row.callee&&<Name at={row.calleeAt} href={row.calleeHref} choose={choose}>{row.callee}</Name>}</p>;
          })}</div>}
        </div>;
      })}
    </section>)}
  </div>;
}

// A call that leaves its program reads from each program's own code: "redis-cli:
// cliConnect → anetTcpConnect → anetTcpGenericConnect ⇢ redis-server:
// acceptHandler → anetAccept"; a call to an outside endpoint is the
// program's one side, then what it calls, outgoing, the program not named
// ("cmd/litestream:" had begun every row of cmd/litestream's own reading).
function SidesRow({row,choose}){
  const [first]=row.sides;
  return <p className="flow-card-row flow-card-sides">
    {row.sides.map((side,s)=><React.Fragment key={s}>{s>0&&<i className="flow-card-joint">⇢</i>}{row.sides.length>1&&<b className="flow-card-program">{side.program}:</b>}
      {side.path.map((step,k)=><React.Fragment key={k}>{k>0&&<i>→</i>}<Name at={step.part&&step.key?{part:step.part,key:step.key}:null} href={k===side.path.length-1&&s===0?row.site:''} choose={choose}>{step.name}</Name></React.Fragment>)}
    </React.Fragment>)}
    {row.sides.length===1&&first&&<><i>→</i><span>{dotted(row.callee)}</span><em>{t('outgoing')}</em></>}
  </p>;
}

// A frame's connections in the reading column: one line per frame or
// participant at the other end and direction, its calls' count and the
// parts they are made from, opening to the same rows as its card. Of one
// part (`single`) the line names the parts at the other end instead:
// "→ Core infrastructure · Network sockets 2 · Dynamic strings 1".
// The ends written only in tests (`apart`) stand last, under one closed
// "Tests" (owner, 2026-09-30: an area's connections had opened on a test
// fixture's forty rows).
export function FrameConnections({groups,open,choose=null,single=false}){
  const line=group=><details key={group.key} data-connection-key={group.key} open={group.key===open} data-reading-anchor={group.key===open?'':undefined}>
      <summary><span className="map-connection-peer">{group.incoming?'←':'→'} {group.title}</span><b>{cardTotal(group.card)}</b>
        {(single&&!group.incoming?group.card.into:group.card.from).length>0&&<small>{(single&&!group.incoming?group.card.into:group.card.from).map(part=>`${part.name} ${part.count}`).join(' · ')}</small>}</summary>
      <CallRows card={group.card} choose={choose}/>
    </details>;
  const apart=groups.filter(group=>group.apart);
  return <section className="map-frame-connections">
    <h5>{t('Connections')}</h5>
    {groups.filter(group=>!group.apart).map(line)}
    {apart.length>0&&<details className="map-connections-apart" open={apart.some(group=>group.key===open)}><summary>{t('Tests')}</summary>{apart.map(line)}</details>}
  </section>;
}

// An arrow's card on the canvas (briefCard): the parts it goes into, under
// each the names it reaches there, one name to a line. A part its heading
// names already (`into`) is not named again: "Server lifecycle and cron →
// Virtual memory" had stood over "→ Virtual memory" and its names in two
// columns (reviewer, 2026-09-30).
// A part's first dozen names say what it is; a click on the arrow reads
// every call in the column.
const briefMost=12;
export function BriefRows({card,into=''}){
  const parts=briefCard(card);
  return <div className="flow-card-brief">{parts.map(part=><section key={part.name}>
    {part.name!==into&&<h5 className="flow-card-pair"><span><i>→</i> {part.name}</span></h5>}
    {part.names.length>0&&<ul>
      {part.names.slice(0,briefMost).map(entry=><li key={entry.name}><Link href={entry.href}>{entry.name}</Link></li>)}
      {part.names.length>briefMost&&<li className="flow-card-more" aria-hidden="true">…</li>}</ul>}
  </section>)}</div>;
}
