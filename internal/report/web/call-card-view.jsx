import React from 'react';
import {reach} from './call-card.mjs';

const t=(...args)=>window.rmT(...args);
const stop=event=>event.stopPropagation();
function Link({href,title,children}){
  return href?<a href={href} title={title||undefined} target="_blank" rel="noopener" onClick={stop}>{children}</a>:<span title={title||undefined}>{children}</span>;
}
// The words between a caller and its callee: an arrow for a call, the
// relation's own words for anything else ("passes callback").
const verb=kind=>kind==='calls'||kind==='implemented in'?'→':kind.replace(/_/g,' ');

// What a card's calls are, in one line: how many of each kind, then from
// how many parts into how many of the frames at its ends.
export function cardCount(card,fromTotal,intoTotal){
  const words={calls:'{0} calls',passes_callback:'{0} callbacks',reads:'{0} reads','implemented in':'{0} inputs',other:'{0} other connections'};
  const said=card.kinds.map(([kind,n])=>t(words[kind]||words.other,n));
  const from=card.from.length?reach(card.from.length,fromTotal):null,into=reach(card.into.length,intoTotal);
  const ends=[from&&(from.all?t('from all {0} parts',from.count):t('from {0} of {1} parts',from.count,from.of)),
    into&&(into.all?t('into all {0}',into.count):t('into {0} of {1}',into.count,into.of))].filter(Boolean).join(' ');
  return [said.join(', '),ends].filter(Boolean).join(', ');
}

// A fold says what its set is: one of the set a dispatch site calls, or the
// same set handed over whole, and how many of it this card holds.
function foldWords(fold){
  const here=fold.count<fold.of?` · ${t('{0} here',fold.count)}`:'';
  if(fold.one)return t('one of {0}',fold.of)+here;
  return (fold.same?t('the same {0} as {1}',fold.of,fold.same):t('the same {0}',fold.of))+here;
}

// The body of a call card, or of a connection in the reading column: its
// calls by the part they are made from, then by the part they go into,
// under headings that stay at the top while the list scrolls. A caller is
// written once for its run of calls; a fold is one line, its callees by
// part under it, each part opening to their names.
// A part's own card leaves out the heading of calls made from the part
// itself (`own`), and a relation with no call of its own that would only
// name the heading above it again says nothing more.
export function CallRows({card,sticky=true,own=''}){
  return <div className={`flow-card-groups ${sticky?'flow-card-sticky':''}`}>
    {card.groups.map(group=><section key={group.id||'-'} data-call-group={group.id}>
      {group.id&&group.id!==own&&<h4 className="flow-card-group"><span>{group.name}</span><b>{group.count}</b></h4>}
      {group.folds.map(fold=><div key={fold.caller+fold.fold} className="flow-card-fold">
        <p className="flow-card-row"><Link href={fold.site}>{fold.caller}</Link><i>{verb(fold.kind)}</i><span className="flow-card-say">{foldWords(fold)}</span></p>
        <div className="flow-card-fold-parts">{fold.parts.map(part=><details key={part.id} onClick={stop}>
          <summary><span>{part.name}</span><b>{part.count}</b></summary>
          <p>{part.rows.map((row,i)=><React.Fragment key={i}>{i>0&&' '}<Link href={row.href}>{row.callee}</Link></React.Fragment>)}</p>
        </details>)}</div>
      </div>)}
      {group.pairs.map(pair=><div key={pair.id} className="flow-card-pair-rows">
        <h5 className="flow-card-pair"><span><i>→</i> {pair.name}</span><b>{pair.count}</b></h5>
        <div className="flow-card-rows">{pair.rows.map((row,i)=>{
          if(row.kind==='other'){
            if(!row.at&&!row.otherHref&&(row.other===pair.name||row.other===group.name))return null;
            return <p key={i} className="flow-card-row flow-card-other"><Link href={row.otherHref}>{row.other}</Link>{row.at&&<em>{row.at}</em>}</p>;
          }
          const again=i>0&&pair.rows[i-1].caller===row.caller&&pair.rows[i-1].kind!=='other';
          return <p key={i} className="flow-card-row"><span className={again?'flow-card-again':''}>{row.kind==='implemented in'?row.caller:<Link href={row.site} title={row.at}>{row.caller}</Link>}</span>
            <i>{verb(row.kind)}</i><Link href={row.calleeHref}>{row.callee}</Link></p>;
        })}</div>
      </div>)}
    </section>)}
  </div>;
}

// A frame's connections in the reading column: one line per frame or
// participant at the other end and direction, its calls' count and the
// parts they are made from, opening to the same rows as its card.
export function FrameConnections({groups,open}){
  return <section className="map-frame-connections">
    <h5>{t('Connections')}</h5>
    {groups.map(group=><details key={group.key} data-connection-key={group.key} open={group.key===open} data-reading-anchor={group.key===open?'':undefined}>
      <summary><span className="map-connection-peer">{group.incoming?'←':'→'} {group.title}</span><b>{group.card.total}</b>
        {group.card.from.length>0&&<small>{group.card.from.map(part=>`${part.name} ${part.count}`).join(' · ')}</small>}</summary>
      <CallRows card={group.card} sticky={false}/>
    </details>)}
  </section>;
}
