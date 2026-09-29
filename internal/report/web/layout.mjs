// Group the original relations by outside identity and direction. Numbering
// identifies inner parts, not an inferred execution order.
export function connections(area, members, edges, outsideOf=id=>id) {
  const own = new Set(members), groups = new Map();
  for (const edge of edges) {
    const from = own.has(edge.from), to = own.has(edge.to);
    if(from === to) continue;
    const outside = outsideOf(from ? edge.to : edge.from), key = `${from?'out':'in'}:${outside}`;
    if(!groups.has(key))groups.set(key,{key,area,outside,incoming:!from,insides:new Set(),relations:[],edges:[]});
    const group = groups.get(key);
    group.insides.add(from?edge.from:edge.to);group.relations.push(...edge.relations);group.edges.push(edge.id);
  }
  return [...groups.values()].map(g=>({...g,insides:[...g.insides]}));
}

// Where a drawn arrow crosses a frame's border: an arrow's card stands by
// it there (owner, 2026-09-28). The arrow ends' marks
// had stood at the end of the route nearest the border, which for an arrow
// running on to a part inside was that part's edge: redis-cli's stood on
// Command line client and over Dynamic strings. `points` is the drawn
// polyline, `box` {x,y,width,height}. Of its crossings, the one nearest
// the end inside the frame; an end on the border is one. Null when it
// never meets the border.
export function borderCrossing(points, box, eps=.5) {
  const left=box.x,top=box.y,right=box.x+box.width,bottom=box.y+box.height;
  const inside=p=>p.x>left+eps&&p.x<right-eps&&p.y>top+eps&&p.y<bottom-eps;
  const onBorder=p=>!inside(p)&&p.x>=left-eps&&p.x<=right+eps&&p.y>=top-eps&&p.y<=bottom+eps;
  const crossings=[];
  points.forEach((a,i)=>{
    if(onBorder(a)){crossings.push({x:a.x,y:a.y});return;}
    const b=points[i+1];if(!b||inside(a)===inside(b)||onBorder(b))return;
    // The segment passes the border once: the first border line it meets
    // within the frame's span on the other axis, from the inside end.
    const [from,to]=inside(a)?[a,b]:[b,a];
    let best=null;
    for(const [axis,value] of [['x',left],['x',right],['y',top],['y',bottom]]){
      const span=to[axis]-from[axis];if(Math.abs(span)<1e-9)continue;
      const t=(value-from[axis])/span;if(t<0||t>1)continue;
      const p={x:from.x+(to.x-from.x)*t,y:from.y+(to.y-from.y)*t};
      if(p.x<left-eps||p.x>right+eps||p.y<top-eps||p.y>bottom+eps)continue;
      if(!best||t<best.t)best={t,p};
    }
    if(best)crossings.push(best.p);
  });
  if(!crossings.length)return null;
  return inside(points.at(-1))&&!inside(points[0])?crossings.at(-1):crossings[0];
}

// Where the arrows of a part looked at leave it when no drawn route touches
// it: a route between two areas is drawn from area to area, so Replication,
// read by itself, had no arrow of its own. Each connection gets one short
// arrow out of the side facing what it connects to, spread along that side
// in the order its neighbours stand, pointing out for an outgoing one and in
// for an incoming one; its card stands where it meets the border. `box`
// {x,y,width,height}; `outsides` [{key, box, incoming}]; `length` the
// arrow's length in the same units.
export function stubEnds(box, outsides, length) {
  const cx=box.x+box.width/2,cy=box.y+box.height/2,sides=new Map();
  for(const outside of outsides){
    const dx=outside.box.x+outside.box.width/2-cx,dy=outside.box.y+outside.box.height/2-cy;
    const side=Math.abs(dx)/box.width>=Math.abs(dy)/box.height?(dx<0?'left':'right'):(dy<0?'top':'bottom');
    const along=side==='left'||side==='right'?cy+dy:cx+dx;
    if(!sides.has(side))sides.set(side,[]);
    sides.get(side).push({...outside,along});
  }
  const ends=new Map(),normal={left:{x:-1,y:0},right:{x:1,y:0},top:{x:0,y:-1},bottom:{x:0,y:1}};
  for(const [side,list] of sides){
    list.sort((a,b)=>a.along-b.along||String(a.key).localeCompare(String(b.key)));
    list.forEach((outside,i)=>{
      const t=(i+1)/(list.length+1);
      const point=side==='left'?{x:box.x,y:box.y+box.height*t}:side==='right'?{x:box.x+box.width,y:box.y+box.height*t}
        :side==='top'?{x:box.x+box.width*t,y:box.y}:{x:box.x+box.width*t,y:box.y+box.height};
      const far={x:point.x+normal[side].x*length,y:point.y+normal[side].y*length};
      ends.set(outside.key,{side,point,points:outside.incoming?[far,point]:[point,far]});
    });
  }
  return ends;
}
