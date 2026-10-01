// The canvas invariants of the accepted rewrite (2026-10-01), checked on a
// rendered report at every level a reader reaches: the whole map, every
// program entered, and every area of the two largest programs. Each level
// gets random pans and random pointer moves, and every arrow is pointed at.
// Nothing here screenshots; every check reads what is drawn.
//
// The DOM contract this reads (old path and `?scene=1` alike):
//   [data-map]                   the canvas host; `flowGeometry()` lists the
//                                placed boxes {id,parentId,branch,frame,shown};
//                                `data-scene-level` (new path) or
//                                `data-entered-program` (old) names the level
//   [data-map-explorer]          `showWholeMap()`, `goToLevel({id,kind:'frame'})`
//                                (a fixture page offers `showWholeMap()` and
//                                `focusNode(id)` instead)
//   .react-flow__node[data-id]   a drawn box
//   g[data-edge-id]              a drawn arrow, `data-edge-ends` = "from to";
//                                its line is `path[data-edge-line]` or the
//                                path that is neither `.flow-edge-hit` nor
//                                `.flow-edge-casing`; its head is that path's
//                                marker-end/marker-start, or `data-edge-head`
//                                = end|start|both|none on the g; it is dark
//                                when the g has `.flow-edge-active` or
//                                `data-edge-dark`
//   [data-edge-hit=ID]           its hit path, which the pointer rests on
//   .flow-floating-card          the card an arrow opens
//   [data-port-end=ID]           a program port item standing for end ID
//   [data-marker]                a marker (new path): `data-marker-box`,
//                                `data-marker-side` = in|out,
//                                `data-marker-end` = the end ids it stands for
//   [data-box-title=ID] or the old title elements ([data-frame-title],
//   [data-summary-area], [data-component-overview], a part's strong)

// The invariants, in table order, with what each one says.
export const invariants=[
  ['pan-level','a pan never changes the level'],
  ['pan-keeps','no element in sight disappears on a pan'],
  ['pan-fixed','a pan keeps every box at its map coordinates'],
  ['point-keeps','no element disappears on a pointer move'],
  ['point-style','pointing changes only classes and order: no box, path or camera moves'],
  ['one-path','each arrow is one continuous polyline'],
  ['own-ends','each arrow starts on its own source and ends on its own target (border, port or marker)'],
  ['head-in','each arrow has its head at its target, its last segment pointing in'],
  ['shared-run','no two arrows share over 6 px unless they are the two directions of one pair'],
  ['in-frame','every arrow end lies in the level frame or on a port or marker'],
  ['off-canvas','no arrow end out of the canvas while the level frame fits it'],
  ['crosses','no arrow runs through a box it does not join'],
  ['step1-gap','parallel lanes stand at least 7.5 screen px apart, as in the approved Step 1 drawing'],
  ['marker-size','ports and markers are 20-28 px at every camera, one size per level'],
  ['markers','at most 3 markers per side and 6 per box, the stack no taller than the box'],
  ['titles','titles at one level within ±10% of their median'],
  ['dark-top','dark arrows are drawn after grey ones'],
  ['cards','every arrow opens its card when pointed at'],
  ['no-labels','no digit, plaque or kind label on the canvas'],
  ['text','no clipped or too small text on the canvas (chip names apart)'],
  ['chip-text','an outside chip\'s name reads at 11 px or more'],
  ['outside-40','the whole map Outside holds at most 40 top-level items'],
  ['page-errors','the page raises no error'],
];

// Runs in the page: a kit of read-only probes, installed as window.__inv.
export function invariantKit(){
  const map=()=>document.querySelector('[data-map]'),root=()=>document.querySelector('.flow-root');
  const canvas=()=>{const r=root().getBoundingClientRect();return {l:r.left,t:r.top,r:r.right,b:r.bottom};};
  const camera=()=>{
    const v=root()?.querySelector('.react-flow__viewport'),m=/translate\(([-\d.e]+)px, ?([-\d.e]+)px\) scale\(([-\d.e]+)\)/.exec(v?.style.transform||'');
    return m?{x:+m[1],y:+m[2],zoom:+m[3]}:{x:0,y:0,zoom:1};
  };
  const shown=el=>!!el&&el.isConnected&&(el.checkVisibility?el.checkVisibility({checkOpacity:true,checkVisibilityCSS:true}):true)&&el.getClientRects().length>0;
  const box=r=>({l:r.left,t:r.top,r:r.right,b:r.bottom});
  const meets=(a,b,e=0)=>Math.min(a.r,b.r)-Math.max(a.l,b.l)>e&&Math.min(a.b,b.b)-Math.max(a.t,b.t)>e;
  const level=()=>{const m=map();return m.dataset.sceneLevel||m.dataset.enteredProgram||'whole';};
  const nodeEl=id=>root().querySelector(`.react-flow__node[data-id="${CSS.escape(id)}"]`);
  const line=g=>g.querySelector('path[data-edge-line]')||g.querySelector('path:not(.flow-edge-hit):not(.flow-edge-casing)');
  const edges=()=>[...root().querySelectorAll('g[data-edge-id]')];
  const geometry=()=>{try{return map().flowGeometry?.().nodes||null;}catch{return null;}};
  // The world point of a path's coordinates on the screen.
  const toScreen=(p,cam=camera(),c=canvas())=>({x:c.l+cam.x+p.x*cam.zoom,y:c.t+cam.y+p.y*cam.zoom});
  // A path's subpaths as screen polylines (M and L; a curve by its ends).
  const polylines=(d,cam,c)=>{
    const out=[];let at=null;
    for(const m of String(d||'').matchAll(/([MLHVCQSTZmlhvcqstz])([^MLHVCQSTZmlhvcqstz]*)/g)){
      const op=m[1],n=(m[2].match(/-?[\d.]+(?:e-?\d+)?/g)||[]).map(Number);
      if(op==='M'){at={x:n[0],y:n[1]};out.push([at]);for(let i=2;i+1<n.length;i+=2){at={x:n[i],y:n[i+1]};out.at(-1).push(at);}}
      else if(op==='L')for(let i=0;i+1<n.length;i+=2){at={x:n[i],y:n[i+1]};out.at(-1)?.push(at);}
      else if(op==='H')for(const x of n){at={x,y:at.y};out.at(-1)?.push(at);}
      else if(op==='V')for(const y of n){at={x:at.x,y};out.at(-1)?.push(at);}
      else if('CQST'.includes(op)&&n.length>=2){at={x:n.at(-2),y:n.at(-1)};out.at(-1)?.push(at);}
    }
    return out.map(points=>points.map(p=>toScreen(p,cam,c)));
  };
  const keyed=[
    ['node','.react-flow__node[data-id]',el=>el.dataset.id,'world'],
    ['edge','g[data-edge-id]',el=>el.dataset.edgeId,'world'],
    ['title','[data-frame-title]',el=>el.dataset.frameTitle,'world'],
    ['title','[data-box-title]',el=>el.dataset.boxTitle,'world'],
    ['summary','[data-summary-area]',el=>el.dataset.summaryArea,'world'],
    ['overview','[data-component-overview]',el=>el.dataset.componentOverview,'world'],
    ['mark','[data-zoom-into]',el=>el.dataset.zoomInto,'world'],
    ['port','[data-port-end]',el=>el.dataset.portEnd,'screen'],
    ['marker','[data-marker]',el=>el.dataset.marker,'screen'],
  ];
  const kit={
    camera,level,canvas,
    ready:()=>{const m=map();return !!m&&m.classList.contains('flow-enabled')&&!m.classList.contains('flow-initializing')&&!!root();},
    scene:()=>!!map()?.sceneState||!!document.querySelector('.scene-root'),
    // What is drawn, with its screen box: the elements a reader can lose.
    snapshot(){
      const els={},paths={},seen=new Map();
      for(const [kind,selector,id,anchor] of keyed)for(const el of root().querySelectorAll(selector)){
        const base=`${kind}:${id(el)}`,n=seen.get(base)||0;seen.set(base,n+1);
        const target=kind==='edge'?line(el)||el:el,r=target.getBoundingClientRect(),on=shown(target)&&(kind!=='edge'||shown(el));
        els[n?`${base}#${n}`:base]=[on?1:0,r.left,r.top,r.right,r.bottom,anchor];
        if(kind==='edge')paths[base]=line(el)?.getAttribute('d')||'';
      }
      return {cam:camera(),level:level(),canvas:canvas(),els,paths,card:[...document.querySelectorAll('.flow-floating-card')].some(shown)};
    },
    // The boxes a level holds and the ids of the level's units: the whole
    // map's roots, an entered program's or an open area's children.
    units(frame){
      const nodes=geometry();
      if(nodes)return nodes.filter(n=>(n.parentId||'')===(frame||'')).map(n=>n.id);
      const all=[...root().querySelectorAll('.react-flow__node[data-id]')].filter(shown).map(el=>({id:el.dataset.id,r:box(el.getBoundingClientRect())}));
      const inside=(a,b)=>a.l>=b.l-1&&a.r<=b.r+1&&a.t>=b.t-1&&a.b<=b.b+1&&(a.r-a.l)*(a.b-a.t)<(b.r-b.l)*(b.b-b.t);
      const holder=frame?all.find(n=>n.id===frame):null;
      return all.filter(n=>n.id!==frame&&(!holder||inside(n.r,holder.r))&&!all.some(m=>m.id!==frame&&m!==n&&inside(n.r,m.r)&&(!holder||inside(m.r,holder.r)))).map(n=>n.id);
    },
    // Programs, by their parts, and each program's areas: from the
    // report's explorer nodes or a fixture's page data (the scene path
    // lays out one level at a time), else from what is drawn.
    programs(){
      const explorer=[...document.querySelectorAll('[data-map-explorer] [data-node]')];
      const records=explorer.length?explorer.map(n=>({id:n.id,title:n.dataset.title,branch:n.dataset.branch||'',children:(n.dataset.children||'').split(/\s+/).filter(Boolean)}))
        :map().pageData?.records||null;
      if(records){
        const byID=new Map(records.map(r=>[r.id,r]));
        const parts=(id,seen=new Set())=>{if(seen.has(id))return 0;seen.add(id);const r=byID.get(id);if(!r)return 0;const kids=(r.children||[]).filter(k=>byID.has(k));return kids.length?kids.reduce((s,k)=>s+parts(k,seen),0):1;};
        return records.filter(r=>r.branch==='component').map(r=>({id:r.id,title:r.title||r.id,parts:parts(r.id),
          areas:(r.children||[]).map(k=>byID.get(k)).filter(a=>a?.branch==='area').map(a=>({id:a.id,title:a.title||a.id}))}));
      }
      const nodes=geometry()||[],kids=new Map();
      for(const n of nodes){if(!kids.has(n.parentId||''))kids.set(n.parentId||'',[]);kids.get(n.parentId||'').push(n);}
      const parts=id=>(kids.get(id)||[]).reduce((sum,n)=>sum+(n.frame||kids.has(n.id)?parts(n.id):1),0);
      const title=id=>root().querySelector(`[data-box-title="${CSS.escape(id)}"],[data-frame-title="${CSS.escape(id)}"] strong,[data-component-overview="${CSS.escape(id)}"] strong,[data-summary-area="${CSS.escape(id)}"] strong`)?.textContent?.trim().replace(/\s+/g,' ')||id;
      return nodes.filter(n=>!n.parentId&&['component','program'].includes(n.branch)).map(n=>({id:n.id,title:title(n.id),parts:parts(n.id),
        areas:(kids.get(n.id)||[]).filter(a=>a.branch==='area').map(a=>({id:a.id,title:title(a.id)}))}));
    },
    // The arrows at rest: one path each, from its own source to its own
    // target, its head pointing in, sharing no run, in the level's frame.
    // `skip` maps the arrows already checked to their paths: pointed at a
    // port or marker, only the lines it adds or redraws are checked again.
    arrows(frame,skip=null){
      const cam=camera(),c=canvas(),nodes=geometry(),parent=new Map((nodes||[]).map(n=>[n.id,n.parentId||'']));
      const result={};const add=(name,ok,example)=>{const r=result[name]||={checked:0,failed:0,examples:[]};r.checked++;if(!ok){r.failed++;if(r.examples.length<8)r.examples.push(example);}};
      const rectOf=id=>{const el=nodeEl(id);return el&&shown(el)?box(el.getBoundingClientRect()):null;};
      const endRects=id=>[...root().querySelectorAll(`[data-port-end="${CSS.escape(id)}"]`),...[...root().querySelectorAll('[data-marker-end]')].filter(el=>el.dataset.markerEnd.split(/\s+/).includes(id))]
        .filter(shown).map(el=>box(el.getBoundingClientRect()));
      const ancestors=id=>{const out=new Set();for(let at=id,i=0;at&&i<50;at=parent.get(at),i++)out.add(at);return out;};
      const onBorder=(p,r,e=1.5)=>p.x>=r.l-e&&p.x<=r.r+e&&p.y>=r.t-e&&p.y<=r.b+e&&Math.min(Math.abs(p.x-r.l),Math.abs(p.x-r.r),Math.abs(p.y-r.t),Math.abs(p.y-r.b))<=e;
      const within=(p,r,e=2)=>p.x>=r.l-e&&p.x<=r.r+e&&p.y>=r.t-e&&p.y<=r.b+e;
      const inSight=r=>meets(r,c,0);
      // The level's frame: the scene's own (the entered program's frame at
      // its areas too), else the entered box, else the canvas.
      const sceneFrame=(()=>{try{const f=map().sceneState?.().scene?.frame;if(!f)return null;const a=toScreen({x:f.x,y:f.y},cam,c),b=toScreen({x:f.x+f.width,y:f.y+f.height},cam,c);return {l:a.x,t:a.y,r:b.x,b:b.y};}catch{return null;}})();
      const frameRect=sceneFrame||frame&&rectOf(frame)||c,fits=frameRect&&frameRect.l>=c.l-1&&frameRect.r<=c.r+1&&frameRect.t>=c.t-1&&frameRect.b<=c.b+1;
      const drawn=[];
      for(const g of edges()){
        const path=line(g);if(!path||!shown(g)||!shown(path))continue;
        const r=box(path.getBoundingClientRect());if(!inSight({l:r.l-1,t:r.t-1,r:r.r+1,b:r.b+1}))continue;
        const subs=polylines(path.getAttribute('d'),cam,c).filter(points=>points.length>1);if(!subs.length)continue;
        const ends=(g.dataset.edgeEnds||'').split(/\s+/).filter(Boolean),from=ends[0]||'',to=ends.at(-1)||'';
        const marked=a=>{const v=path.getAttribute(a)||getComputedStyle(path)[a==='marker-end'?'markerEnd':'markerStart'];return !!v&&v!=='none';};
        const head=g.dataset.edgeHead||(marked('marker-end')&&marked('marker-start')?'both':marked('marker-end')?'end':marked('marker-start')?'start':'none');
        const id=g.dataset.edgeId,name=`${id} (${from}→${to})`,fresh=!skip||skip[id]!==path.getAttribute('d');
        drawn.push({id,name,from,to,subs,fresh,points:subs.flat(),dark:g.classList.contains('flow-edge-active')||g.hasAttribute('data-edge-dark')});
        if(!fresh)continue;
        add('one-path',subs.length===1,`${name}: ${subs.length} pieces`);
        const first=subs[0][0],last=subs.at(-1).at(-1);
        const fromRect=rectOf(from),toRect=rectOf(to),fromEnds=endRects(from),toEnds=endRects(to);
        const startOn=fromEnds.some(e=>within(first,e))||!!fromRect&&onBorder(first,fromRect);
        const endOn=toEnds.some(e=>within(last,e))||!!toRect&&onBorder(last,toRect);
        add('own-ends',startOn&&endOn,`${name}: ${startOn?'':`starts off ${from||'nothing'}${fromRect?'':' (not drawn)'}`}${!startOn&&!endOn?'; ':''}${endOn?'':`ends off ${to||'nothing'}${toRect?'':' (not drawn)'}`}`);
        // The head stands at the target end and the last segment enters it.
        let headOk=head==='end'||head==='both',why=headOk?'':`head ${head}`;
        if(headOk&&toRect&&onBorder(last,toRect)&&!toEnds.some(e=>within(last,e))){
          const tail=subs.at(-1),q=tail.at(-2)||tail[0];
          const side=[['l',Math.abs(last.x-toRect.l)],['r',Math.abs(last.x-toRect.r)],['t',Math.abs(last.y-toRect.t)],['b',Math.abs(last.y-toRect.b)]].sort((a,b)=>a[1]-b[1])[0][0];
          const inward={l:q.x<last.x-.25,r:q.x>last.x+.25,t:q.y<last.y-.25,b:q.y>last.y+.25}[side];
          const outside=!(q.x>toRect.l+1&&q.x<toRect.r-1&&q.y>toRect.t+1&&q.y<toRect.b-1);
          if(!inward||!outside){headOk=false;why=`last segment ${outside?'runs along':'comes from inside'} the ${side} side`;}
        }
        add('head-in',headOk,`${name}: ${why}`);
        // Ends in the level frame, or on a port or marker.
        const endsOk=[[first,fromEnds],[last,toEnds]].every(([p,own])=>own.some(e=>within(p,e))||!!frameRect&&within(p,frameRect));
        add('in-frame',endsOk,`${name}: an end outside ${frame||'the canvas'}`);
        if(fits)add('off-canvas',[first,last].every(p=>within(p,c,1)),`${name}: an end beyond the canvas`);
        // No box it does not join lies across it.
        const own=new Set([...ancestors(from),...ancestors(to)]);
        const holds=(outer,id)=>ancestors(id).has(outer);
        const crossed=[];
        for(const el of root().querySelectorAll('.react-flow__node[data-id]')){
          const id=el.dataset.id;if(own.has(id)||holds(from,id)||holds(to,id)||holds(id,from)||holds(id,to)||!shown(el))continue;
          const b=box(el.getBoundingClientRect()),inner={l:b.l+2,t:b.t+2,r:b.r-2,b:b.b-2};
          if(inner.r<=inner.l||inner.b<=inner.t||!inSight(b))continue;
          // A frame drawn open is crossed only where it has no drawn child:
          // its own room, not the boxes inside it.
          if(subs.some(points=>points.slice(1).some((p,i)=>segmentMeets(points[i],p,inner))))crossed.push(id);
        }
        add('crosses',!crossed.length,`${name} through ${crossed.slice(0,3).join(', ')}`);
      }
      function segmentMeets(a,b,r){
        const x1=Math.min(a.x,b.x),x2=Math.max(a.x,b.x),y1=Math.min(a.y,b.y),y2=Math.max(a.y,b.y);
        if(x2<r.l||x1>r.r||y2<r.t||y1>r.b)return false;
        if(Math.abs(a.x-b.x)<.5||Math.abs(a.y-b.y)<.5)return true;
        // A slanted segment: clip it to the box (Liang-Barsky).
        let t0=0,t1=1;const dx=b.x-a.x,dy=b.y-a.y;
        for(const [p,q] of [[-dx,a.x-r.l],[dx,r.r-a.x],[-dy,a.y-r.t],[dy,r.b-a.y]]){
          if(p===0){if(q<0)return false;continue;}
          const t=q/p;if(p<0){if(t>t1)return false;if(t>t0)t0=t;}else{if(t<t0)return false;if(t<t1)t1=t;}
        }
        return t0<t1;
      }
      // Shared runs: two arrows on one line over 6px, unless they are the
      // two directions of one pair.
      const segs=a=>a.subs.flatMap(points=>points.slice(1).map((p,i)=>[points[i],p]));
      const pairs=[];
      for(let i=0;i<drawn.length;i++)for(let j=i+1;j<drawn.length;j++){
        const a=drawn[i],b=drawn[j];
        if(!a.fresh&&!b.fresh)continue;
        if(a.from&&a.to&&((a.from===b.to&&a.to===b.from)||(a.from===b.from&&a.to===b.to)))continue;
        let shared=0;
        for(const [p,q] of segs(a))for(const [r,s] of segs(b)){
          if(Math.abs(p.y-q.y)<.5&&Math.abs(r.y-s.y)<.5&&Math.abs(p.y-r.y)<1)shared+=Math.max(0,Math.min(Math.max(p.x,q.x),Math.max(r.x,s.x))-Math.max(Math.min(p.x,q.x),Math.min(r.x,s.x)));
          else if(Math.abs(p.x-q.x)<.5&&Math.abs(r.x-s.x)<.5&&Math.abs(p.x-r.x)<1)shared+=Math.max(0,Math.min(Math.max(p.y,q.y),Math.max(r.y,s.y))-Math.max(Math.min(p.y,q.y),Math.min(r.y,s.y)));
        }
        pairs.push([a,b,shared]);
      }
      result['shared-run']={checked:pairs.length,failed:0,examples:[]};
      for(const [a,b,shared] of pairs)if(shared>6){result['shared-run'].failed++;if(result['shared-run'].examples.length<8)result['shared-run'].examples.push(`${a.name} and ${b.name}: ${shared.toFixed(0)} px`);}
      // Lanes keep at least the room the owner-approved Step 1 drawing kept
      // between two (7.5 screen pixels, scene.test.mjs): two parallel runs
      // of different arrows, side by side over more than a pixel, with no
      // box between them. Runs on one line are shared runs, not lanes.
      const boxes=[...root().querySelectorAll('.react-flow__node[data-id]')].filter(shown).map(el=>box(el.getBoundingClientRect())).filter(b=>inSight(b));
      const runs=drawn.flatMap(a=>segs(a).map(([p,q])=>({a,p,q,upright:Math.abs(p.x-q.x)<.5,flat:Math.abs(p.y-q.y)<.5}))).filter(r=>(r.upright||r.flat)&&inSight({l:Math.min(r.p.x,r.q.x)-1,r:Math.max(r.p.x,r.q.x)+1,t:Math.min(r.p.y,r.q.y)-1,b:Math.max(r.p.y,r.q.y)+1}));
      let narrowest=Infinity,between='';
      for(let i=0;i<runs.length;i++)for(let j=i+1;j<runs.length;j++){
        const s=runs[i],t=runs[j];if(s.a===t.a||!s.a.fresh&&!t.a.fresh)continue;
        for(const [axis,other] of [['x','y'],['y','x']]){
          if(!(axis==='x'?s.upright&&t.upright:s.flat&&t.flat))continue;
          const lo=Math.max(Math.min(s.p[other],s.q[other]),Math.min(t.p[other],t.q[other])),hi=Math.min(Math.max(s.p[other],s.q[other]),Math.max(t.p[other],t.q[other]));
          if(hi-lo<=1)continue;
          const d=Math.abs(s.p[axis]-t.p[axis]);if(d<.5)continue;
          const a=Math.min(s.p[axis],t.p[axis]),b=Math.max(s.p[axis],t.p[axis]),mid=(lo+hi)/2,[near,far]=axis==='x'?['l','r']:['t','b'],[from,to]=axis==='x'?['t','b']:['l','r'];
          if(boxes.some(r=>r[near]>a&&r[far]<b&&mid>=r[from]&&mid<=r[to]))continue;
          if(d<narrowest){narrowest=d;between=`${s.a.name} and ${t.a.name}`;}
        }
      }
      result['step1-gap']={checked:runs.length>1?1:0,failed:narrowest<7.5?1:0,examples:narrowest<7.5?[`lanes ${narrowest.toFixed(1)} px apart: ${between}`]:[],narrowest:Number.isFinite(narrowest)?narrowest:null};
      const bends=drawn.map(a=>a.points.length-2);
      result.bends={max:bends.length?Math.max(...bends):0,mean:bends.length?bends.reduce((s,b)=>s+b,0)/bends.length:0,arrows:drawn.length};
      return result;
    },
    // Each unit's title, as the font size it is read at on the screen.
    titles(frame){
      const c=canvas(),out=[];
      for(const id of kit.units(frame)){
        const q=CSS.escape(id);
        const el=[...root().querySelectorAll(`[data-box-title="${q}"],[data-frame-title="${q}"] strong,[data-summary-area="${q}"] strong,[data-component-overview="${q}"] strong,.react-flow__node[data-id="${q}"] .flow-part>strong,.react-flow__node[data-id="${q}"] .flow-chip-name`)].find(shown);
        if(!el)continue;
        const r=el.getBoundingClientRect();if(!meets(box(r),c,2))continue;
        const scale=el.offsetHeight?r.height/el.offsetHeight:1,px=parseFloat(getComputedStyle(el).fontSize)*scale;
        out.push({id,px,text:el.textContent.trim().slice(0,30)});
      }
      return out;
    },
    // Ports and markers in sight, by their screen size.
    ports(){
      const c=canvas();
      return [...root().querySelectorAll('[data-port-end],[data-marker]')].filter(shown).map(el=>({id:el.dataset.portEnd||el.dataset.marker,r:box(el.getBoundingClientRect())}))
        .filter(p=>meets(p.r,c,1)).map(p=>({id:p.id,w:p.r.r-p.r.l,h:p.r.b-p.r.t}));
    },
    // Markers on their boxes: at most three a side, six a box, the stack no
    // taller than the box, each touching its side.
    markers(){
      const byBox=new Map(),out={checked:0,failed:0,examples:[]};
      for(const el of root().querySelectorAll('[data-marker]')){
        if(!shown(el))continue;const id=el.dataset.markerBox||'';
        if(!byBox.has(id))byBox.set(id,[]);byBox.get(id).push({side:el.dataset.markerSide||'',r:box(el.getBoundingClientRect())});
      }
      for(const [id,list] of byBox){
        out.checked++;
        const b=nodeEl(id)?.getBoundingClientRect(),why=[];
        const sides={in:list.filter(m=>m.side==='in'),out:list.filter(m=>m.side==='out')};
        if(list.length>6)why.push(`${list.length} markers`);
        for(const [side,ms] of Object.entries(sides)){
          if(ms.length>3)why.push(`${ms.length} on ${side}`);
          if(b&&ms.length){const top=Math.min(...ms.map(m=>m.r.t)),bottom=Math.max(...ms.map(m=>m.r.b));if(bottom-top>b.height+1)why.push(`${side} stack ${Math.round(bottom-top)} px on a ${Math.round(b.height)} px box`);}
          if(b)for(const m of ms){const x=side==='in'?b.left:b.right;if(m.r.l>x+1||m.r.r<x-1)why.push(`${side} marker off its edge`);}
        }
        if(!b)why.push('its box is not drawn');
        if(why.length){out.failed++;if(out.examples.length<8)out.examples.push(`${id}: ${[...new Set(why)].join(', ')}`);}
      }
      return out;
    },
    // The whole map's Outside frames, by their top-level items.
    outside(){
      const nodes=geometry()||[];
      return nodes.filter(n=>!n.parentId&&n.branch==='outside').map(n=>({id:n.id,items:nodes.filter(m=>m.parentId===n.id).length}));
    },
    // Paint order: no dark arrow under a grey one it meets.
    darkOnTop(){
      const list=edges().filter(g=>shown(g)&&line(g)&&shown(line(g))).map((g,i)=>{
        const svg=g.closest('svg'),z=parseFloat(getComputedStyle(svg?.parentElement===root()?svg:svg||g).zIndex)||0;
        return {id:g.dataset.edgeId,dark:g.classList.contains('flow-edge-active')||g.hasAttribute('data-edge-dark'),z,i,r:box(line(g).getBoundingClientRect())};
      });
      const above=(a,b)=>a.z!==b.z?a.z>b.z:a.i>b.i;
      const bad=[];let checked=0;
      for(const d of list.filter(e=>e.dark))for(const g of list.filter(e=>!e.dark&&meets(e.r,d.r,-1))){checked++;if(!above(d,g))bad.push(`${d.id} under ${g.id}`);}
      return {checked,bad};
    },
    // A point on an arrow the pointer reaches it at: the hit path there is
    // its own, inside the canvas.
    hitPoint(id){
      const c=canvas(),path=root().querySelector(`[data-edge-hit="${CSS.escape(id)}"]`);
      if(!path)return {missing:true};
      const length=path.getTotalLength(),ctm=path.getScreenCTM(),r=box(path.getBoundingClientRect());
      if(!meets({l:r.l-1,t:r.t-1,r:r.r+1,b:r.b+1},c,0))return {outOfSight:true};
      // Its middle first, then every few pixels along it.
      const steps=Math.max(12,Math.min(400,Math.ceil(length*Math.hypot(ctm.a,ctm.b)/6)));
      const fractions=[.5,.35,.65,.2,.8,...Array.from({length:steps+1},(_,i)=>i/steps)];
      let seen=0,over=null;
      for(const f of fractions){
        const p=path.getPointAtLength(length*f),x=p.x*ctm.a+p.y*ctm.c+ctm.e,y=p.x*ctm.b+p.y*ctm.d+ctm.f;
        if(x<c.l+6||x>c.r-6||y<c.t+6||y>c.b-6)continue;
        seen++;
        const el=document.elementFromPoint(x,y);
        if(el?.closest?.('[data-edge-hit]')?.dataset.edgeHit===id)return {x,y};
        over||=el;
      }
      if(!seen)return {outOfSight:true};
      const named=el=>{const at=el.closest?.('[data-edge-hit],.react-flow__node,[data-port],.flow-floating-card,[data-frame-title],[data-summary-area],[data-component-overview]')||el;
        return `${at.tagName.toLowerCase()}${at.classList.length?'.'+[...at.classList].slice(0,2).join('.'):''}${at.dataset?.edgeHit?` (arrow ${at.dataset.edgeHit})`:at.dataset?.id?` (${at.dataset.id})`:''}`;};
      return {covered:true,by:over?named(over):'nothing'};
    },
    // The arrows drawn now, by their paths.
    paths(){return Object.fromEntries(edges().map(g=>[g.dataset.edgeId,line(g)?.getAttribute('d')||''])) ;},
    // Every port item and marker in sight, by its middle.
    handles(){
      const c=canvas();
      return [...root().querySelectorAll('[data-port-end],[data-marker]')].filter(shown).map(el=>{const r=el.getBoundingClientRect();return {id:el.dataset.portEnd||el.dataset.marker,x:r.left+r.width/2,y:r.top+r.height/2};})
        .filter(h=>h.x>c.l+2&&h.x<c.r-2&&h.y>c.t+2&&h.y<c.b-2&&document.elementFromPoint(h.x,h.y)?.closest?.('[data-port-end],[data-marker]'));
    },
    hitIds(){return [...root().querySelectorAll('[data-edge-hit]')].filter(path=>shown(path.closest('g[data-edge-id]')||path)).map(path=>path.dataset.edgeHit);},
    // The texts drawn on the canvas in sight, which the label and text
    // lints read.
    textCount(){
      const c=canvas(),walker=document.createTreeWalker(root(),NodeFilter.SHOW_TEXT);let n=0;
      for(let node=walker.nextNode();node;node=walker.nextNode()){
        const el=node.parentElement;if(!node.textContent.trim()||!el||el.closest('.react-flow__attribution,.flow-floating-card,.flow-loading,svg')||!shown(el))continue;
        if(meets(box(el.getBoundingClientRect()),c,0))n++;
      }
      return n;
    },
    cardOpen(){return [...document.querySelectorAll('.flow-floating-card')].some(shown);},
    // A spot on the canvas over nothing a reader points at, for a pan.
    emptySpot(seed){
      const c=canvas();let s=seed>>>0;const rnd=()=>{s=(s+0x6D2B79F5)>>>0;let t=s;t=Math.imul(t^(t>>>15),t|1);t^=t+Math.imul(t^(t>>>7),t|61);return ((t^(t>>>14))>>>0)/4294967296;};
      for(let i=0;i<80;i++){
        const x=c.l+40+rnd()*(c.r-c.l-80),y=c.t+40+rnd()*(c.b-c.t-80),el=document.elementFromPoint(x,y);
        if(el&&root().contains(el)&&!el.closest('.react-flow__node,[data-edge-hit],.flow-floating-card,[data-port],[data-marker],button,a,[data-frame-title],[data-summary-area],[data-component-overview],.flow-location'))return {x,y,empty:true};
      }
      return {x:(c.l+c.r)/2,y:(c.t+c.b)/2,empty:false};
    },
    signature(){const m=map(),r=root();return `${r?.querySelector('.react-flow__viewport')?.style.transform}|${r?.getElementsByTagName('*').length}|${m?.dataset.enteredProgram||''}|${m?.dataset.sceneLevel||''}|${m?.getAttribute('aria-busy')||''}`;},
  };
  return kit;
}

// Node side: a seeded random stream.
export function random(seed){
  let s=seed>>>0;
  return ()=>{s=(s+0x6D2B79F5)>>>0;let t=s;t=Math.imul(t^(t>>>15),t|1);t^=t+Math.imul(t^(t>>>7),t|61);return ((t^(t>>>14))>>>0)/4294967296;};
}
export const hash=text=>{let h=2166136261;for(const ch of String(text)){h^=ch.charCodeAt(0);h=Math.imul(h,16777619);}return h>>>0;};

// Node side: what a reader lost between two snapshots. A world element in
// sight before stays drawn where the camera shows its place in sight; a
// port or marker stays while the level does.
export function compare(before,after,{camera=true}={}){
  const lost=[],moved=[],repathed=[];
  const dx=after.cam.x-before.cam.x,dy=after.cam.y-before.cam.y,same=Math.abs(after.cam.zoom-before.cam.zoom)<1e-6;
  const c=before.canvas,inSight=(l,t,r,b,e)=>Math.min(r,c.r)-Math.max(l,c.l)>e&&Math.min(b,c.b)-Math.max(t,c.t)>e;
  for(const [key,[on,l,t,r,b,anchor]] of Object.entries(before.els)){
    if(!on||!inSight(l,t,r,b,2))continue;
    const now=after.els[key];
    if(anchor==='screen'){if(before.level===after.level&&!(now&&now[0]))lost.push(key);continue;}
    if(!same)continue;
    if(!inSight(l+dx,t+dy,r+dx,b+dy,8))continue;
    if(!(now&&now[0])){lost.push(key);continue;}
    if(key.startsWith('node:')&&(Math.abs(now[1]-(l+dx))>1||Math.abs(now[2]-(t+dy))>1||Math.abs(now[3]-(r+dx))>1||Math.abs(now[4]-(b+dy))>1))moved.push(key);
  }
  for(const [key,d] of Object.entries(before.paths))if(after.paths[key]!==undefined&&after.paths[key]!==d)repathed.push(key);
  const cameraMoved=camera&&(Math.abs(dx)>.5||Math.abs(dy)>.5||!same);
  return {lost,moved,repathed,cameraMoved};
}
