// A part's declarations at the deepest zoom. A type is a block: its name and
// under it the methods it owns, each a row. A function or a module's variable
// is a block of one row. Callers stand left of what they call and a callable
// left of the type it returns: a block's column is the longest chain of links
// leading to it. Blocks are placed in the order the page lists them, the
// model's keys first, each in its own column under the blocks before it; a
// column too tall for the card spills into the next one, and what finds no
// room at all is counted, not drawn. Once a key finds no room, nothing after
// the keys is drawn in its place.
export const symbolRow={header:28,row:24,pad:6,gap:10};

export function symbolBlocks(symbols,links,columns,height,rank=i=>i){
  const blocks=[],blockOf=new Array(symbols.length).fill(-1);
  const field=symbol=>symbol.kind==='field'||symbol.kind==='more';
  symbols.forEach((symbol,i)=>{if(!symbol.owner&&symbol.kind!=='skip'){blockOf[i]=blocks.length;blocks.push({head:i,rows:[]});}});
  symbols.forEach((symbol,i)=>{
    if(!symbol.owner||symbol.kind==='skip')return;
    const owner=blockOf[symbol.owner-1];
    if(owner>=0){blocks[owner].rows.push(i);blockOf[i]=owner;}
    else{blockOf[i]=blocks.length;blocks.push({head:i,rows:[]});}
  });
  // As in a class box: the fields first, then the methods.
  for(const block of blocks)block.rows.sort((a,b)=>field(symbols[b])-field(symbols[a])||a-b);
  const before=blocks.map(()=>new Set());
  for(const [from,to] of links){
    const a=blockOf[from],b=blockOf[to];
    if(a>=0&&b>=0&&a!==b)before[b].add(a);
  }
  const depth=new Array(blocks.length).fill(-1),visiting=new Set();
  const column=i=>{
    if(depth[i]>=0)return depth[i];
    if(visiting.has(i))return 0;
    visiting.add(i);
    const d=Math.max(0,...[...before[i]].map(from=>column(from)+1));
    visiting.delete(i);
    return depth[i]=Math.min(columns-1,d);
  };
  // A type taller than the card shows the rows that fit and counts the rest.
  const most=Math.max(1,Math.floor((height-symbolRow.header-symbolRow.pad)/symbolRow.row)-1);
  // A block stands where its first declaration is listed: a type holding a
  // key method is placed among the keys. Placed by link column first, Redis's
  // Data structures had drawn ten zipmap helpers and counted list and five
  // other structs in its "+59".
  const listed=blocks.map(block=>Math.min(...[block.head,...block.rows].map(rank)));
  const key=blocks.map(block=>[block.head,...block.rows].some(i=>symbols[i].key));
  for(const block of blocks)if(block.rows.length>most+1){block.more=block.rows.length-most;block.rows=block.rows.slice(0,most);}
  const tall=block=>symbolRow.header+(block.rows.length?(block.rows.length+(block.more?1:0))*symbolRow.row+symbolRow.pad:0);
  const order=blocks.map((_,i)=>i).sort((a,b)=>listed[a]-listed[b]);
  const filled=new Array(columns).fill(0),placed=new Map();
  let hidden=0,keyLeft=false;
  for(const i of order){
    const need=tall(blocks[i]);
    let c=column(i);
    while(c<columns&&filled[c]&&filled[c]+need>height)c++;
    if(c>=columns)c=filled.indexOf(Math.min(...filled));
    if(filled[c]+need>height||keyLeft&&!key[i]){hidden+=1+blocks[i].rows.length+(blocks[i].more||0);keyLeft||=key[i];continue;}
    placed.set(i,{column:c,y:filled[c],height:need});
    filled[c]+=need+symbolRow.gap;
  }
  // Where each declaration's row stands, for the links drawn between them.
  const rows=new Array(symbols.length).fill(null);
  for(const [i,at] of placed){
    const block=blocks[i];
    rows[block.head]={block:i,column:at.column,y:at.y,height:symbolRow.header};
    block.rows.forEach((symbol,k)=>{rows[symbol]={block:i,column:at.column,y:at.y+symbolRow.header+k*symbolRow.row,height:symbolRow.row};});
  }
  return {blocks:[...placed].map(([i,at])=>({...blocks[i],...at})),rows,hidden};
}

// The tiles of a part as its card draws them. A tile is as wide as the
// longest name among them, so no name is cut; the columns share the card's
// width. Tiles stand by file, the files in the order their first
// declaration is listed and each file's declarations in the page's order:
// the file is a hint, and Redis's linked-list functions stood scattered
// among the dictionary's. The layer is drawn at 1/divisor of the card's
// scale, the smallest divisor from `least` at which every tile fits whole,
// so nothing is counted away: a large part is a larger map to pan when read.
// Cut to 190px, Redis's names read "_dictStringCopyHTKe…" and its "+59" hid
// the list struct.
export const tileRoom={header:20,inset:8,columnGap:36};
// The part's name stands in a band over its tiles, as large on screen as at
// the quarter scale whatever the divisor: 20 card units at a quarter.
export const tileHeader=divisor=>tileRoom.header*4/divisor;
export const tileFont=symbol=>`${symbol.key?750:symbol.owner?400:600} ${symbol.owner?12.5:13}px ui-monospace,SFMono-Regular,Menlo,monospace`;
export function tileGrid(symbols,links,box,measure,{least=4,step=.5,most=64}={}){
  const shown=symbols.map((symbol,i)=>[symbol,i]).filter(([symbol])=>symbol.kind!=='skip');
  // Padding and border as canvas.css draws a head (10px a side) and a row
  // (12px left, 10px right) inside a 1px border.
  const nameWidth=Math.ceil(Math.max(0,...shown.map(([symbol])=>measure(symbol.name||'',tileFont(symbol))+(symbol.owner?22:20)+2)));
  const files=new Map();for(const [symbol] of shown){const file=symbol.path||'';if(!files.has(file))files.set(file,files.size);}
  const rank=i=>(files.get(symbols[i].path||'')??files.size)*symbols.length+i;
  const {inset,columnGap:gap}=tileRoom;
  let grid;
  for(let divisor=least;divisor<=most;divisor+=step){
    const inner={width:box.width*divisor,height:(box.height-tileHeader(divisor))*divisor};
    const columns=Math.max(1,Math.floor((inner.width-2*inset+gap)/(nameWidth+gap)));
    const tileWidth=Math.max(nameWidth,Math.floor((inner.width-2*inset-(columns-1)*gap)/columns));
    grid={divisor,inner,columns,tileWidth,nameWidth,...symbolBlocks(symbols,links,columns,inner.height-2*inset,rank)};
    // Columns enough for a caller, what it calls and what that returns.
    if(grid.hidden===0&&(!links.length||columns>=3))return grid;
  }
  return grid;
}
