// The scene canvas's one store (PLAN S3), read by React through
// useSyncExternalStore: the level is reducer state, entered by an action or
// by a zoom gesture with hysteresis, never by a pan. The camera moves on
// every tick and has its own small store, so a pan repaints only the
// overlay.
import {levelAfterZoom,chainOf} from './scene.mjs';

export function createStore(reducer,initial){
  let state=initial;
  const listeners=new Set();
  return {
    getState:()=>state,
    dispatch(action){
      const next=reducer(state,action);
      if(next===state)return;
      state=next;
      for(const listener of [...listeners])listener();
    },
    subscribe(listener){listeners.add(listener);return ()=>listeners.delete(listener);},
  };
}

export const initialState={
  level:[],
  // What the pointer is on (scene.hitTest's answer), or null.
  pointer:null,
  // The host's choice: {scope, operation, entry, selected, matched, searching}.
  view:{scope:'',operation:'',entry:'',selected:new Set(),matched:new Set(),searching:false},
  // A declaration pointed at and one chosen, {part, index}.
  member:{hover:null,chosen:null},
  // Cards kept open by a click, by their arrow's id.
  pinned:[],
  // The arrow or marker whose card the look opened (look.mjs), or ''.
  look:'',
  // The inputs the column points at.
  lit:[],
  version:0,
};

const same=(a,b)=>a===b||a&&b&&a.type===b.type&&a.id===b.id;
// `context` gives the reducer what a zoom needs: {model, geometry, scene()}.
export function sceneReducer(context){
  return (state,action)=>{
    switch(action.type){
    case 'enter':{
      const level=Array.isArray(action.level)?action.level:chainOf(context.model,action.id);
      return level.join('/')===state.level.join('/')?state:{...state,level,pointer:null};
    }
    case 'zoom':{
      const level=levelAfterZoom(context.model,context.geometry(),context.scene(),state.level,action.zoom,action.aim,action.zoomingIn);
      return level.join('/')===state.level.join('/')?state:{...state,level,pointer:null};
    }
    case 'point':
      return same(state.pointer,action.target)?state:{...state,pointer:action.target};
    case 'view':
      return {...state,view:{...initialState.view,...action.view}};
    case 'member':
      return {...state,member:{...state.member,...action.member}};
    case 'look':
      return action.key===state.look?state:{...state,look:action.key};
    case 'pin':
      return state.pinned.includes(action.key)?state:{...state,pinned:[...state.pinned,action.key]};
    case 'unpin':
      return action.key?{...state,pinned:state.pinned.filter(key=>key!==action.key)}:state.pinned.length?{...state,pinned:[]}:state;
    case 'lit':{
      const ids=[...(action.ids||[])];
      return ids.length===state.lit.length&&ids.every(id=>state.lit.includes(id))?state:{...state,lit:ids};
    }
    case 'relayout':
      return {...state,version:state.version+1,pointer:null};
    default:
      return state;
    }
  };
}

// The camera's own store: {x, y, zoom}, written on every move.
export function createCamera(initial={x:0,y:0,zoom:1}){
  let camera=initial;
  const listeners=new Set();
  return {
    get:()=>camera,
    set(next){
      if(next.x===camera.x&&next.y===camera.y&&next.zoom===camera.zoom)return;
      camera={x:next.x,y:next.y,zoom:next.zoom};
      for(const listener of [...listeners])listener();
    },
    subscribe(listener){listeners.add(listener);return ()=>listeners.delete(listener);},
  };
}

// The action a camera move asks for: a zoom gesture's, at the zoom it
// brings and the world point it aims at; null for a pan, which keeps the
// zoom and never changes the level.
export function zoomAction(previous,viewport,aim){
  if(Math.abs(viewport.zoom-previous)<1e-9)return null;
  return {type:'zoom',zoom:viewport.zoom,aim,zoomingIn:viewport.zoom>previous};
}
