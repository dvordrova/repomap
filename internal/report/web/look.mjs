// What the reader is looking at, for the whole map. One thing at a time has
// the look: a badge, a label, whatever a later layer adds. The rules are the
// same for all of them and live here, not in each component:
//   - a look opens on intent: the pointer rests on its handle for `dwell`
//     milliseconds, so a pointer crossing a handle on its way elsewhere opens
//     nothing;
//   - leaving does not end the look at once, so the pointer can cross a gap
//     to what the look opened;
//   - on the way from the handle to its card the pointer is safe: while it
//     stays in the triangle between where it left the handle and the card,
//     the look lasts and no other handle it crosses takes it;
//   - while the camera moves for the look, the thing slides under a still
//     pointer and back, and that is not the reader leaving it;
//   - after the move the look ends when the pointer really moves away.
// The timings are interaction timing, tuned on recorded pointer paths.
export function createLook({linger=260,slack=10,dwell=120}={}){
  let key='',closeAt=0,hold=null,pending=null,safe=null,pointer=null;
  const inSafe=()=>!!safe&&!!pointer&&insideHull(pointer,safe.hull);
  return {
    get key(){return key;},
    get held(){return !!hold;},
    get pending(){return pending?.key||'';},
    get safe(){return inSafe();},
    // When time passing may next open or end the look, or 0.
    get due(){return pending?.at||closeAt||0;},
    enter(next){closeAt=0;pending=null;safe=null;const changed=key!==next;key=next;if(changed)hold=null;return changed;},
    // The pointer came onto a handle. Returns when to ask again whether the
    // look opened, or 0 when nothing is pending: the handle already has the
    // look, or the pointer is on its way to another handle's card.
    aim(next,now){
      if(next===key){closeAt=0;safe=null;pending=null;return 0;}
      if(inSafe())return 0;
      pending={key:next,at:now+dwell};return pending.at;
    },
    // The pointer left a handle before the look opened.
    abandon(which){if(pending?.key===which)pending=null;},
    // Returns when to ask again whether the look has ended, or 0. `from` is
    // where the pointer left and `card` the screen box of what the look
    // opened; the triangle between them is safe.
    leave(which,now,from=null,card=null){
      if(pending?.key===which)pending=null;
      if(key!==which||hold)return 0;
      closeAt=now+linger;
      safe=from&&card?{hull:hull([from,{x:card.left,y:card.top},{x:card.right,y:card.top},{x:card.right,y:card.bottom},{x:card.left,y:card.bottom}])}:null;
      if(safe)pointer={...from};
      return closeAt;
    },
    // The camera moves for this look until `until`; the pointer is at x,y.
    cameraMoves(x,y,until){if(key)hold={x,y,until};},
    // A real pointer movement. onIt: the pointer is on the thing or on what
    // the look opened. Returns true when the look ended.
    move(x,y,now,onIt){
      pointer={x,y};
      if(safe){
        if(inSafe()){if(closeAt)closeAt=now+linger;}
        else safe=null;
      }
      if(!hold)return false;
      if(now<hold.until){hold.x=x;hold.y=y;return false;}
      if(Math.hypot(x-hold.x,y-hold.y)<=slack)return false;
      hold=null;
      if(onIt)return false;
      key='';closeAt=0;return true;
    },
    // Time passing. Returns true when the look opened or ended.
    tick(now){
      if(pending&&now>=pending.at){key=pending.key;pending=null;closeAt=0;hold=null;safe=null;return true;}
      if(!closeAt||hold||now<closeAt)return false;
      closeAt=0;key='';safe=null;return true;
    },
    end(){const had=!!key;key='';closeAt=0;hold=null;pending=null;safe=null;return had;},
  };
}

// The convex hull of a few points, counter-clockwise.
export function hull(points){
  const sorted=[...points].sort((a,b)=>a.x-b.x||a.y-b.y);
  const cross=(o,a,b)=>(a.x-o.x)*(b.y-o.y)-(a.y-o.y)*(b.x-o.x);
  const half=list=>{const out=[];for(const p of list){while(out.length>=2&&cross(out.at(-2),out.at(-1),p)<=0)out.pop();out.push(p);}out.pop();return out;};
  return [...half(sorted),...half(sorted.slice().reverse())];
}
export function insideHull(p,polygon,tolerance=1){
  if(polygon.length<3)return false;
  return polygon.every((a,i)=>{
    const b=polygon[(i+1)%polygon.length],length=Math.hypot(b.x-a.x,b.y-a.y)||1;
    return ((b.x-a.x)*(p.y-a.y)-(b.y-a.y)*(p.x-a.x))/length>=-tolerance;
  });
}
