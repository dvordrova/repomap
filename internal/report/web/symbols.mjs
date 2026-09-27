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

export function symbolBlocks(symbols,links,columns,height){
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
  const listed=blocks.map(block=>Math.min(block.head,...block.rows));
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
