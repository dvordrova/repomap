// What the reader is looking at, for the whole map. One thing at a time has
// the look: a badge, a label, whatever a later layer adds. The rules are the
// same for all of them and live here, not in each component:
//   - leaving does not end the look at once, so the pointer can cross a gap
//     to what the look opened;
//   - while the camera moves for the look, the thing slides under a still
//     pointer and back, and that is not the reader leaving it;
//   - after the move the look ends when the pointer really moves away.
export function createLook({linger=260,slack=10}={}){
  let key='',closeAt=0,hold=null;
  return {
    get key(){return key;},
    get held(){return !!hold;},
    enter(next){closeAt=0;const changed=key!==next;key=next;if(changed)hold=null;return changed;},
    // Returns when to ask again whether the look has ended, or 0.
    leave(which,now){if(key!==which||hold)return 0;closeAt=now+linger;return closeAt;},
    // The camera moves for this look until `until`; the pointer is at x,y.
    cameraMoves(x,y,until){if(key)hold={x,y,until};},
    // A real pointer movement. onIt: the pointer is on the thing or on what
    // the look opened. Returns true when the look ended.
    move(x,y,now,onIt){
      if(!hold)return false;
      if(now<hold.until){hold.x=x;hold.y=y;return false;}
      if(Math.hypot(x-hold.x,y-hold.y)<=slack)return false;
      hold=null;
      if(onIt)return false;
      key='';closeAt=0;return true;
    },
    // Time passing. Returns true when the look ended.
    tick(now){if(!closeAt||hold||now<closeAt)return false;closeAt=0;key='';return true;},
    end(){const had=!!key;key='';closeAt=0;hold=null;return had;},
  };
}
