// Callers stand left of what they call: a declaration's column is the longest
// chain of calls leading to it. Declarations no call joins fill the cells
// left over. A column too tall for the card gives the plain order back.
export function symbolCells(count,calls,columns,rows){
  const callers=Array.from({length:count},()=>[]),joined=new Set();
  for(const [from,to] of calls){callers[to].push(from);joined.add(from);joined.add(to);}
  const depth=Array(count).fill(-1),visiting=new Set();
  const column=i=>{
    if(depth[i]>=0)return depth[i];
    if(visiting.has(i))return 0;
    visiting.add(i);
    const d=Math.max(0,...callers[i].map(from=>column(from)+1));
    visiting.delete(i);
    return depth[i]=Math.min(columns-1,d);
  };
  const cells=Array(count).fill(null),used=new Set(),height=Array(columns).fill(0);
  for(let i=0;i<count;i++)if(joined.has(i)){
    const c=column(i);
    if(height[c]>=rows)return Array.from({length:count},(_,k)=>({column:k%columns,row:Math.floor(k/columns)}));
    cells[i]={column:c,row:height[c]++};used.add(`${c}:${cells[i].row}`);
  }
  let next=0;
  for(let i=0;i<count;i++)if(!cells[i]){
    while(used.has(`${next%columns}:${Math.floor(next/columns)}`))next++;
    cells[i]={column:next%columns,row:Math.floor(next/columns)};used.add(`${cells[i].column}:${cells[i].row}`);
  }
  return cells;
}
