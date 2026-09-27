// Where a floating card stands, in screen pixels. A card explains its handle
// (a part's number, a label on a frame's border), so it stands flush with
// it; it stands outside the frame being read, so it covers none of that
// frame's parts; on the side with room; and wholly inside the canvas. Placed
// by the canvas halves alone, Redis's cards covered the parts they were
// about and ran off the canvas edge.
//   handle, frame, canvas: {left,top,right,bottom}; frame may be null. A
//   handle may stand outside its frame, as an arrowhead does.
//   size: {width,height} of the card on screen.
//   side: the frame side a label stands on ('left', 'right', 'top',
//   'bottom'); its card goes out through that side. A part's number has none.
export function placeCard({handle,frame,canvas,size,side='',gap=8,margin=8}){
  const {width,height}=size,room={left:canvas.left+margin,top:canvas.top+margin,right:canvas.right-margin,bottom:canvas.bottom-margin};
  // An arrowhead stands outside its frame's border: the card clears it too,
  // or it opened over the head the pointer rested on.
  const outer=frame?{left:Math.min(frame.left,handle.left),top:Math.min(frame.top,handle.top),right:Math.max(frame.right,handle.right),bottom:Math.max(frame.bottom,handle.bottom)}:handle;
  const clamp=(value,low,high)=>high<low?low:Math.min(Math.max(value,low),high);
  const vertical=side==='top'||side==='bottom';
  if(vertical){
    const above=outer.top-gap-height,below=outer.bottom+gap;
    const fitsAbove=above>=room.top,fitsBelow=below+height<=room.bottom;
    const preferAbove=side==='top';
    let y=preferAbove?(fitsAbove?above:fitsBelow?below:null):(fitsBelow?below:fitsAbove?above:null);
    if(y===null){
      const up=handle.top-room.top,down=room.bottom-handle.bottom;
      y=down>=up?handle.bottom+gap:handle.top-gap-height;
    }
    const x=handle.left+width<=room.right?handle.left:handle.right-width;
    return {x:clamp(x,room.left,room.right-width),y:clamp(y,room.top,room.bottom-height)};
  }
  const left=outer.left-gap-width,right=outer.right+gap;
  const fitsLeft=left>=room.left,fitsRight=right+width<=room.right;
  const preferLeft=side?side==='left':outer.left-room.left>room.right-outer.right;
  let x=preferLeft?(fitsLeft?left:fitsRight?right:null):(fitsRight?right:fitsLeft?left:null);
  if(x===null){
    // No room outside the frame: beside the handle, toward the roomier side.
    const toLeft=handle.left-room.left,toRight=room.right-handle.right;
    x=toRight>=toLeft?handle.right+gap:handle.left-gap-width;
  }
  const y=handle.top+height<=room.bottom?handle.top:handle.bottom-height;
  return {x:clamp(x,room.left,room.right-width),y:clamp(y,room.top,room.bottom-height)};
}
