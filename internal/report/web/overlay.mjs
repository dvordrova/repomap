// The scene's overlay (PLAN S4): markers and ports keep one screen size at
// every zoom, so where they stand is a function of the camera. `overlayAt`
// gives world places at a zoom (hitTest uses it too); `project` the screen
// places at a camera, run on every camera tick.

// A marker's and a port's screen size, the room between markers and from
// the box's top, and the title size below which markers stay hidden.
export const mark={size:22,gap:3,inset:6,readable:11};

// Where each marker and port stands at `zoom`: a marker touches its box's
// left (inputs) or right (outside) edge from outside, the stack from the
// top under its title, each `mark.size` screen pixels; it shows once the
// box's title reads and the stack fits beside the box. A port is centred on
// its point of the program's border. World units.
// The last zoom's places are kept per scene: the pointer's hit test asks
// at every move, at the camera's one zoom. The list is not to be changed.
const placed=new WeakMap();
export function overlayAt(scene,zoom){
  const kept=placed.get(scene);
  if(kept?.zoom===zoom)return kept.items;
  const items=placeAt(scene,zoom);
  placed.set(scene,{zoom,items});
  return items;
}
function placeAt(scene,zoom){
  const size=mark.size/zoom,gap=mark.gap/zoom,inset=mark.inset/zoom,pad=6/zoom;
  const readable=marker=>(marker.text||scene.text)*17*zoom>=mark.readable;
  const items=[];
  // Each side's stack from the top, a marker stepping down past every
  // arrow running through the column it stands in (an arrow meeting that
  // side, one bending beside the box); one that would stand below the box
  // is not shown.
  const runs=scene.edges.filter(edge=>edge.rest!==false).flatMap(edge=>edge.points.slice(1).map((b,i)=>[edge.points[i],b]));
  const stacks=new Map(),columns=new Map();
  for(const marker of scene.markers){
    const r=marker.rect,key=`${marker.box}:${marker.side}`;
    // A declaration's markers stand in a row beside its tile, outward.
    if(marker.row){
      const out=marker.side==='in'?-1:1,edge=marker.side==='in'?r.x:r.x+r.width;
      items.push({...marker,type:'marker',x:edge+out*(size/2+marker.index*(size+gap)),y:r.y+r.height/2,size,shown:readable(marker)});
      continue;
    }
    if(!columns.has(key))columns.set(key,crossings(runs,marker.side==='in'?r.x-size:r.x+r.width,size));
    let y=stacks.has(key)?stacks.get(key):r.y+inset+size/2;
    for(let moved=true;moved;){
      moved=false;
      for(const [lo,hi] of columns.get(key)){
        const past=hi+pad+size/2;
        if(y+size/2+pad>lo&&past>y+1e-9*size){y=past;moved=true;}
      }
    }
    stacks.set(key,y+size+gap);
    const shown=readable(marker)&&y+size/2+inset<=r.y+r.height;
    const x=marker.side==='in'?r.x-size/2:r.x+r.width+size/2;
    items.push({...marker,type:'marker',x,y,size,shown});
  }
  for(const port of scene.ports)items.push({...port,type:'port',x:port.point.x,y:port.point.y,size,shown:true});
  // A box that can be entered has its magnifier in its top right corner,
  // once its title reads: the only thing on the canvas that zooms. A closed
  // bucket's stands in its bottom right corner, beside its systems' marks,
  // its name's lines the bucket's whole width.
  for(const node of scene.nodes){
    if(!node.enter||node.display==='frame'||node.display==='deep'||node.band!==3)continue;
    const r=node.rect,low=node.display==='bucket',px=Math.max(mark.size,Math.min(32,mark.size*node.text*zoom)),size=px/zoom;
    items.push({id:`zoom:${node.id}`,type:'zoom',box:node.id,x:r.x+r.width-inset-size/2,y:low?r.y+r.height-inset-size/2:r.y+inset+size/2,size,
      shown:readable(node)&&r.width*zoom>=4*px&&r.height*zoom>=2*px});
  }
  return items;
}

// The y-ranges where the segments `runs` cross the column of `width` from
// x `left`, sorted.
function crossings(runs,left,width){
  const right=left+width,out=[];
  for(const [a,b] of runs){
    if(Math.max(a.x,b.x)<left||Math.min(a.x,b.x)>right)continue;
    if(a.x===b.x){out.push([Math.min(a.y,b.y),Math.max(a.y,b.y)]);continue;}
    const y=x=>a.y+(x-a.x)/(b.x-a.x)*(b.y-a.y),from=y(Math.max(left,Math.min(a.x,b.x))),to=y(Math.min(right,Math.max(a.x,b.x)));
    out.push([Math.min(from,to),Math.max(from,to)]);
  }
  return out.sort((p,q)=>p[0]-q[0]);
}

// Screen places at `camera` {x, y, zoom}: each shown item's centre and its
// size in pixels.
export function project(scene,camera){
  return overlayAt(scene,camera.zoom).filter(item=>item.shown).map(item=>({...item,
    left:item.x*camera.zoom+camera.x,top:item.y*camera.zoom+camera.y,px:item.size*camera.zoom}));
}
