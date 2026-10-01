// A part's declarations drawn as tiles inside its card (canvas.jsx and
// scene-canvas.jsx draw them alike): by file and in the order the page
// lists them (symbols.mjs tileGrid), each as wide as the longest name, the
// links between them drawn as arrows.
import React from 'react';
import {tileGrid,tileRoom,tileHeader} from './symbols.mjs';

const tileContext=typeof document==='undefined'?null:document.createElement('canvas').getContext('2d');
export const measureTile=(text,font)=>{tileContext.font=font;return tileContext.measureText(text).width;};
const grids=new Map();
export function partGrid(data,box){
  const key=`${data.id}|${box.width}|${box.height}`;
  if(!grids.has(key))grids.set(key,tileGrid(data.symbols||[],data.symbolCalls||[],box,measureTile));
  return grids.get(key);
}
// A marker is drawn in its line's stroke widths, seven of them. Emphasis
// draws a line 2.5px thick instead of 1.5px (canvas.css), so its marker is
// 1.5/2.5 of that and every head stands 10.5px. At seven emphasised strokes
// a head stood 17.5px, nearly three times the area, and covered the plaque
// at the frame.
export const arrowHead=7,emphasisedHead=arrowHead*1.5/2.5;
export function PartSymbols({symbols,calls,width,height,grid,member}){
  const {divisor,inner,tileWidth,blocks,rows,hidden}=grid,{inset,columnGap:gap}=tileRoom;
  const links=calls||[],hot=member?.hot??-1,chosen=member?.chosen??-1;
  const x=column=>inset+column*(tileWidth+gap);
  // A method inside its type's tile needs no line to that type.
  const drawn=links.filter(([from,to])=>rows[from]&&rows[to]&&rows[from].block!==rows[to].block);
  // A declaration with those its links join. Pointing at one darkens its
  // links and recedes nothing; a chosen one recedes what its links do not
  // join, save what the pointer brings forward.
  const joined=at=>new Set(at<0?[]:[at,...drawn.filter(([from,to])=>from===at||to===at).flat().filter(value=>typeof value==='number')]);
  const lit=joined(hot),kept=joined(chosen);
  const tone=i=>chosen>=0&&!kept.has(i)&&!lit.has(i)?'flow-symbol-dim':'';
  // A row reads as a line of a class box: the visibility sign, the name and,
  // in lighter type, what follows it — "(args): Result" or ": Type".
  const mixed=new Set(blocks.filter(block=>{const all=[block.head,...block.rows].filter(i=>symbols[i].kind!=='more');return all.some(i=>symbols[i].inner)&&all.some(i=>!symbols[i].inner);}).flatMap(block=>[block.head,...block.rows]));
  const row=(i,className,first)=>{
    const symbol=symbols[i],kind=symbol.kind==='field'||symbol.kind==='more'?'flow-symbol-field':'';
    // A click chooses the declaration: the reading names it and the camera
    // centres it. Opened with a modifier, a tile's link still opens the code.
    const choose=event=>{
      if(event.button!==0||event.ctrlKey||event.metaKey||event.shiftKey||event.altKey){event.stopPropagation();return;}
      event.preventDefault();event.stopPropagation();member?.choose(i,event);
    };
    const props={className:`${className} ${kind} ${first?'flow-symbol-first-method':''} ${symbol.key?'flow-symbol-key':''} ${symbol.inner&&mixed.has(i)?'flow-symbol-inner':''} ${symbol.quiet?'flow-symbol-quiet':''} ${i===chosen?'flow-symbol-chosen':''} ${tone(i)}`,
      'data-symbol':i,title:symbol.kind==='more'?undefined:`${symbol.full||symbol.name}${symbol.text||''}`,onMouseEnter:()=>member?.point(i),onClick:symbol.kind==='more'?undefined:choose};
    const body=<>{symbol.name}{symbol.text&&<em>{symbol.text}</em>}</>;
    return symbol.href&&symbol.kind!=='more'?<a key={i} href={symbol.code||symbol.href} target="_blank" {...props}>{body}</a>
      :<span key={i} {...props}>{body}</span>;
  };
  // A drag anywhere over the declarations pans, as it does over the part: at
  // their reading scale they fill the canvas, and a drag that moved is no
  // click on the tile it started on.
  const header=tileHeader(divisor);
  return <div className="flow-part-symbols" style={{top:header,width,height:height-header}}>
    <div style={{width:inner.width,height:inner.height,transform:`scale(${1/divisor})`,transformOrigin:'top left'}} onMouseLeave={()=>member?.point(-1)}>
      <svg width={inner.width} height={inner.height}>
        <defs>{[['flow-symbol-arrow',arrowHead],['flow-symbol-arrow-hot',emphasisedHead]].map(([id,size])=>
          <marker key={id} id={id} viewBox="0 0 8 8" refX="7" refY="4" markerWidth={size} markerHeight={size} orient="auto"><path d="M0 0L8 4L0 8z"/></marker>)}</defs>
        {drawn.map(([from,to,kind],i)=>{
          const a=rows[from],b=rows[to],forward=b.column>a.column,ay=inset+a.y+a.height/2,by=inset+b.y+b.height/2;
          // Across columns a link leaves the right edge and enters the left;
          // within a column it bows out to the right of both rows.
          const start={x:x(a.column)+tileWidth,y:ay},end=forward?{x:x(b.column),y:by}:{x:x(b.column)+tileWidth,y:by};
          const bend=forward?Math.max(16,(end.x-start.x)/2):26;
          const on=hot>=0&&(from===hot||to===hot),receded=chosen>=0&&!on&&from!==chosen&&to!==chosen;
          return <path key={i} className={`flow-symbol-${kind||'calls'} ${on?'flow-symbol-call-hot':receded?'flow-symbol-call-dim':''}`}
            d={`M${start.x} ${start.y}C${start.x+bend} ${start.y},${forward?end.x-bend:end.x+bend} ${end.y},${end.x} ${end.y}`} markerEnd={`url(#flow-symbol-arrow${on?'-hot':''})`}>
            <title>{`${symbols[from].name} ${kind||'calls'} ${symbols[to].name}`}</title></path>;
        })}
      </svg>
      {blocks.map(block=><div key={block.head} className={`flow-symbol-block ${block.rows.length?'flow-symbol-type':''}`}
        style={{left:x(block.column),top:inset+block.y,width:tileWidth,height:block.height}}>
        {row(block.head,'flow-symbol-head')}
        {block.rows.map((i,k)=>row(i,'flow-symbol-row',k>0&&symbols[i].kind!=='field'&&symbols[i].kind!=='more'&&['field','more'].includes(symbols[block.rows[k-1]].kind)))}
        {block.more>0&&<span className="flow-symbol-row flow-symbol-rest">… +{block.more}</span>}
      </div>)}
      {hidden>0&&<span className="flow-symbol-more" style={{right:inset,bottom:inset}}>+{hidden}</span>}
    </div>
  </div>;
}
