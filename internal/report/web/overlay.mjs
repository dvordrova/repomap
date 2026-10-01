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
export function overlayAt(scene,zoom){
  const size=mark.size/zoom,gap=mark.gap/zoom,inset=mark.inset/zoom,clear=size/2+6/zoom;
  const readable=marker=>(marker.text||scene.text)*17*zoom>=mark.readable;
  const items=[];
  // Each side's stack from the top, a marker stepping down past an arrow
  // meeting that side; one that would stand below the box is not shown.
  const stacks=new Map();
  for(const marker of scene.markers){
    const r=marker.rect,key=`${marker.box}:${marker.side}`;
    // A declaration's markers stand in a row beside its tile, outward.
    if(marker.row){
      const out=marker.side==='in'?-1:1,edge=marker.side==='in'?r.x:r.x+r.width;
      items.push({...marker,type:'marker',x:edge+out*(size/2+marker.index*(size+gap)),y:r.y+r.height/2,size,shown:readable(marker)});
      continue;
    }
    let y=stacks.has(key)?stacks.get(key):r.y+inset+size/2;
    for(const at of marker.blocked||[])if(Math.abs(at-y)<clear)y=at+clear;
    stacks.set(key,y+size+gap);
    const shown=readable(marker)&&y+size/2+inset<=r.y+r.height;
    const x=marker.side==='in'?r.x-size/2:r.x+r.width+size/2;
    items.push({...marker,type:'marker',x,y,size,shown});
  }
  for(const port of scene.ports)items.push({...port,type:'port',x:port.point.x,y:port.point.y,size,shown:true});
  // A box that can be entered has its magnifier in its top right corner,
  // once its title reads: the only thing on the canvas that zooms.
  for(const node of scene.nodes){
    if(!node.enter||node.display==='frame'||node.display==='deep'||node.band!==3)continue;
    const r=node.rect;
    items.push({id:`zoom:${node.id}`,type:'zoom',box:node.id,x:r.x+r.width-inset-size/2,y:r.y+inset+size/2,size,
      shown:readable(node)&&r.width*zoom>=4*mark.size&&r.height*zoom>=2*mark.size});
  }
  return items;
}

// Screen places at `camera` {x, y, zoom}: each shown item's centre and its
// size in pixels.
export function project(scene,camera){
  return overlayAt(scene,camera.zoom).filter(item=>item.shown).map(item=>({...item,
    left:item.x*camera.zoom+camera.x,top:item.y*camera.zoom+camera.y,px:mark.size}));
}
