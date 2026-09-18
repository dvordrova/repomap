// One function places the camera for "show me all of these": the rectangles
// are whatever the map drew, measured, in the map's coordinates. Nothing here
// knows what a card, a label or a part is, so another layer of the map needs
// no new code.

export function union(rects){
  const list=rects.filter(Boolean);
  if(!list.length)return null;
  const left=Math.min(...list.map(r=>r.x)),top=Math.min(...list.map(r=>r.y));
  const right=Math.max(...list.map(r=>r.x+r.width)),bottom=Math.max(...list.map(r=>r.y+r.height));
  return {x:left,y:top,width:right-left,height:bottom-top};
}

// The viewport {x,y,zoom} that holds box inside a canvas of size with pad
// around it. It never zooms in, and it moves the map as little as it can:
// what already fits is left alone, and anchor, a point of the map, stays
// where it is on the screen whenever the box allows.
export function cameraContaining(box,viewport,size,{pad=24,anchor=null,minZoom=0}={}){
  if(!box||!(box.width>0)||!(box.height>0))return viewport;
  const room={width:Math.max(1,size.width-2*pad),height:Math.max(1,size.height-2*pad)};
  const zoom=Math.max(minZoom,Math.min(viewport.zoom,room.width/box.width,room.height/box.height));
  const held=anchor?{x:anchor.x*viewport.zoom+viewport.x,y:anchor.y*viewport.zoom+viewport.y}:null;
  const place=(origin,start,length,span,pinned)=>{
    // Where the map's origin would stand with the anchor held, or as it is.
    const wanted=pinned===null?origin:pinned;
    // The box's start may not stand before the pad, nor its end past the far
    // pad: the origin lies between these two.
    const least=pad-start*zoom,most=span-pad-(start+length)*zoom;
    // A box larger than the room cannot be held whole: show its beginning.
    return least>most?least:Math.min(most,Math.max(least,wanted));
  };
  return {
    x:place(viewport.x,box.x,box.width,size.width,held?held.x-anchor.x*zoom:null),
    y:place(viewport.y,box.y,box.height,size.height,held?held.y-anchor.y*zoom:null),
    zoom,
  };
}
