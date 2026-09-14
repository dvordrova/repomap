// Moving the camera or revealing the map can put a different card under a
// stationary pointer. Preview resumes only when the reader moves the pointer.
export class HoverGate {
  point=null;
  origin=null;
  remember(x,y){this.point={x,y};}
  pause(){this.origin=this.point?{...this.point}:true;}
  get allowed(){return this.origin===null;}
  move(x,y){
    this.remember(x,y);
    if(this.origin===true){this.origin={x,y};return false;}
    if(this.origin&&Math.abs(x-this.origin.x)+Math.abs(y-this.origin.y)<3)return false;
    this.origin=null;return true;
  }
}
