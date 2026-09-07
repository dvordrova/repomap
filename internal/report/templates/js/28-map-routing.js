// Folding retains evidence; ELK owns placement, ports and obstacle-free routing.
// The pinned engine is embedded in the report: no network or analysis is needed.
var repomapGraph = (function () {
  var engine=new ELK();
  async function layout(boxes,edges,areas,availableWidth){
    var children=Object.keys(boxes).map(function(id){return{id:id,width:boxes[id].w,height:boxes[id].h};});
    var options={'elk.algorithm':'layered','elk.direction':'RIGHT','elk.edgeRouting':'ORTHOGONAL','elk.randomSeed':'1','elk.hierarchyHandling':'INCLUDE_CHILDREN','elk.padding':'[top=36,left=36,bottom=36,right=36]','elk.spacing.nodeNode':'44','elk.spacing.edgeNode':'24','elk.spacing.edgeEdge':'12','elk.layered.spacing.nodeNodeBetweenLayers':'90','elk.layered.spacing.edgeNodeBetweenLayers':'28','elk.layered.spacing.edgeEdgeBetweenLayers':'12','elk.layered.mergeEdges':'false','elk.separateConnectedComponents':'true'};
    if(areas && areas.length){
      var assigned=new Set();
      var compound=areas.map(function(area){var members=children.filter(function(n){return area.nodes.indexOf(n.id)>=0;});members.forEach(function(n){assigned.add(n.id);});return{id:area.id,children:members,layoutOptions:Object.assign({},options,{'elk.padding':'[top=44,left=24,bottom=24,right=24]'})};}).filter(function(a){return a.children.length;});
      children=compound.concat(children.filter(function(n){return !assigned.has(n.id);}));
    }
    var input={id:'root',layoutOptions:options,children:children,edges:edges.map(function(e,i){return{id:'edge-'+i,sources:[e.from],targets:[e.to]};})};
    var request=JSON.stringify(input);
    var graph=await engine.layout(JSON.parse(request));
    // Prefer downward flow when it avoids horizontal overflow. Neither choice
    // changes membership or shrinks labels, and ties keep left-to-right flow.
    if(availableWidth && graph.width>availableWidth){
      function downward(n){if(n.layoutOptions)n.layoutOptions['elk.direction']='DOWN';(n.children||[]).forEach(downward);}
      var alternative=JSON.parse(request);downward(alternative);
      var vertical=await engine.layout(alternative);if(vertical.width<graph.width)graph=vertical;
    }
    var placed={}, offsets={root:{x:0,y:0}},frames=[];
    function collect(parent,x,y){(parent.children||[]).forEach(function(n){var px=x+n.x,py=y+n.y;offsets[n.id]={x:px,y:py};if(n.children){frames.push({id:n.id,x:px,y:py,w:n.width,h:n.height});collect(n,px,py);}else placed[n.id]={x:px,y:py,w:n.width,h:n.height};});}
    collect(graph,0,0);
    var routed=(graph.edges||[]).map(function(e){var original=edges[+e.id.slice(5)],offset=offsets[e.container||'root'];
      var segments=(e.sections||[]).map(function(s){return [s.startPoint].concat(s.bendPoints||[],[s.endPoint]).map(function(p){return{x:p.x+offset.x,y:p.y+offset.y};});});
      return Object.assign({},original,{segments:segments,points:segments.flat(),path:pathFor(segments)});
    });
    return {boxes:placed,edges:routed,areas:frames,width:graph.width,height:graph.height};
  }
  function pathFor(segments){return segments.map(function(part){return part.map(function(p,i){return(i?'L ':'M ')+p.x+' '+p.y;}).join(' ');}).join(' ');}
  // Repository components with no path between them are independent pictures.
  // ELK still owns every connected picture and every original arrow; packing
  // their rectangles creates no new parent, relation or semantic grouping.
  async function layoutComponents(boxes,edges,availableWidth){
    var ids=Object.keys(boxes),adjacent={},seen=new Set(),parts=[];
    ids.forEach(function(id){adjacent[id]=[];});
    edges.forEach(function(e){adjacent[e.from].push(e.to);adjacent[e.to].push(e.from);});
    ids.forEach(function(id){
      if(seen.has(id))return;
      var queue=[id],members=[];seen.add(id);
      for(var i=0;i<queue.length;i++){var current=queue[i];members.push(current);adjacent[current].forEach(function(next){if(!seen.has(next)){seen.add(next);queue.push(next);}});}
      parts.push(members);
    });
    var padding=16,gap=20,x=padding,y=padding,rowHeight=0,width=0,result={boxes:{},edges:[],areas:[]};
    for(var members of parts){
      var memberSet=new Set(members),localBoxes={};members.forEach(function(id){localBoxes[id]=boxes[id];});
      var localEdges=edges.filter(function(e){return memberSet.has(e.from)&&memberSet.has(e.to);});
      var picture;
      if(localEdges.length)picture=await layout(localBoxes,localEdges,null,availableWidth-padding*2);
      else{var id=members[0];picture={boxes:{},edges:[]};picture.boxes[id]={x:0,y:0,w:boxes[id].w,h:boxes[id].h};}
      var left=Infinity,top=Infinity,right=-Infinity,bottom=-Infinity;
      Object.values(picture.boxes).forEach(function(b){left=Math.min(left,b.x);top=Math.min(top,b.y);right=Math.max(right,b.x+b.w);bottom=Math.max(bottom,b.y+b.h);});
      picture.edges.forEach(function(e){e.points.forEach(function(p){left=Math.min(left,p.x);top=Math.min(top,p.y);right=Math.max(right,p.x);bottom=Math.max(bottom,p.y);});});
      var w=right-left,h=bottom-top;
      if(x>padding&&x+w>availableWidth-padding){x=padding;y+=rowHeight+gap;rowHeight=0;}
      var dx=x-left,dy=y-top;
      Object.keys(picture.boxes).forEach(function(id){var b=picture.boxes[id];result.boxes[id]={x:b.x+dx,y:b.y+dy,w:b.w,h:b.h};});
      picture.edges.forEach(function(e){var segments=e.segments.map(function(part){return part.map(function(p){return{x:p.x+dx,y:p.y+dy};});});result.edges.push(Object.assign({},e,{segments:segments,points:segments.flat(),path:pathFor(segments)}));});
      width=Math.max(width,x+w);x+=w+gap;rowHeight=Math.max(rowHeight,h);
    }
    result.width=width+padding;result.height=y+rowHeight+padding;return result;
  }
  function fold(edges, representatives, inside) {
    var result=new Map();
    edges.forEach(function(e){
      if(inside && !inside.has(e.from) && !inside.has(e.to))return;
      (representatives[e.from]||[]).forEach(function(from){(representatives[e.to]||[]).forEach(function(to){
        if(from===to)return;
        var key=from+'\x00'+to+'\x00'+e.possible;
        if(!result.has(key))result.set(key,{from:from,to:to,possible:e.possible,relations:[]});
        result.get(key).relations.push(e);
      });});
    });
    return Array.from(result.values()).sort(function(a,b){return (a.from+a.to+a.possible).localeCompare(b.from+b.to+b.possible);});
  }
  function draw(map, routed) {
    var svg=map.querySelector('svg'), layer=svg.querySelector('.map-edges'), marker=svg.querySelector('marker').id;
    layer.replaceChildren();map.visibleEdges=routed;
    routed.forEach(function(edge){
      var group=document.createElementNS('http://www.w3.org/2000/svg','g');group.setAttribute('class','map-edge-group');
      var casing=document.createElementNS('http://www.w3.org/2000/svg','path');casing.setAttribute('d',edge.path);casing.setAttribute('class','map-edge-case');group.appendChild(casing);
      var line=document.createElementNS('http://www.w3.org/2000/svg','path');
      line.setAttribute('d',edge.path);line.setAttribute('class','map-edge'+(edge.possible?' map-edge-possible':''));
      line.dataset.from=edge.from;line.dataset.to=edge.to;
      line.dataset.operations=Array.from(new Set(edge.relations.flatMap(function(e){return e.operations||[];}))).join(' ');
      line.setAttribute('marker-end','url(#'+marker+')');
      var title=document.createElementNS('http://www.w3.org/2000/svg','title');
      title.textContent=rmT('{0} connections',edge.relations.length)+' · '+Array.from(new Set(edge.relations.map(function(e){return e.label;}))).join(', ');line.appendChild(title);
      line.setAttribute('tabindex','0');line.setAttribute('role','button');line.setAttribute('aria-label',title.textContent);
      function inspect(){map.dispatchEvent(new CustomEvent('repomap:connection',{detail:edge}));}
      function lift(){layer.appendChild(group);}
      line.addEventListener('mouseenter',lift);line.addEventListener('focus',lift);
      line.addEventListener('click',inspect);line.addEventListener('keydown',function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();inspect();}});
      group.appendChild(line);layer.appendChild(group);
    });
    return routed;
  }
  return {layout:layout,layoutComponents:layoutComponents,fold:fold,draw:draw};
})();

// Existing repository roles are filters of one component level. Filtering
// does not create parent nodes or reinterpret connections through hidden nodes.
(function(){document.querySelectorAll('.repo-map').forEach(function(map){
  var svg=map.querySelector('svg'),nodes={},origins={},sizes={},revision=0,selectedRole='';
  var lanes=Array.from(svg.querySelectorAll('.map-lanes text')).map(function(n){return{role:n.dataset.role,title:n.textContent,nodes:[]};});
  svg.querySelectorAll('.repo-node').forEach(function(n,i){
    var r=n.querySelector('rect'),id=n.id||'repo-unread-'+i;
    nodes[id]=n;origins[id]={x:+r.getAttribute('x'),y:+r.getAttribute('y')};sizes[id]={w:+r.getAttribute('width'),h:+r.getAttribute('height')};
    var lane=lanes.find(function(l){return l.role===n.dataset.role;});if(lane)lane.nodes.push(id);
  });
  var edges=Array.from(svg.querySelectorAll('.map-edge')).map(function(e){return{from:e.dataset.from,to:e.dataset.to,possible:e.classList.contains('map-edge-possible'),label:(e.querySelector('title')||{}).textContent||'',operations:[]};}).filter(function(e){return nodes[e.from]&&nodes[e.to];});
  lanes=lanes.filter(function(l){return l.role&&l.nodes.length;});
  var total=Object.keys(nodes).length,controls=document.createElement('div');controls.className='repo-area-controls';controls.setAttribute('role','group');controls.setAttribute('aria-label',rmT('Show components'));map.prepend(controls);
  var count=document.createElement('span');count.className='repo-filter-count';count.setAttribute('role','status');
  function makeButton(title,role,number){
    var b=document.createElement('button');b.type='button';b.textContent=title+' · '+number;b.dataset.role=role;
    b.addEventListener('click',function(){
      if(selectedRole===role)return;selectedRole=role;
      var url=new URL(location.href);if(role)url.searchParams.set('component-role',role);else url.searchParams.delete('component-role');
      history.pushState(history.state,'',url);render();
    });controls.appendChild(b);
  }
  makeButton(rmT('All components'),'',total);
  lanes.forEach(function(lane){makeButton(lane.title,lane.role,lane.nodes.length);});controls.appendChild(count);
  svg.querySelector('.map-lanes').replaceChildren();
  function componentGrid(ids,width){
    var padding=16,gap=20,maxWidth=0,maxHeight=0;
    ids.forEach(function(id){maxWidth=Math.max(maxWidth,sizes[id].w);maxHeight=Math.max(maxHeight,sizes[id].h);});
    if(!ids.length)return{boxes:{},edges:[],areas:[],width:padding*2,height:padding*2};
    var columns=Math.max(1,Math.min(ids.length,Math.floor((width-padding*2+gap)/(maxWidth+gap)))),rows=Math.ceil(ids.length/columns),placed={};
    ids.forEach(function(id,i){placed[id]={x:padding+(i%columns)*(maxWidth+gap),y:padding+Math.floor(i/columns)*(maxHeight+gap),w:sizes[id].w,h:sizes[id].h};});
    return{boxes:placed,edges:[],areas:[],width:padding*2+columns*maxWidth+(columns-1)*gap,height:padding*2+rows*maxHeight+(rows-1)*gap};
  }
  async function render(){
    if(!map.clientWidth)return;
    var ticket=++revision,visible=Object.keys(nodes).filter(function(id){return !selectedRole||nodes[id].dataset.role===selectedRole;}).sort(function(a,b){return Number(nodes[b].dataset.default==='true')-Number(nodes[a].dataset.default==='true');}),reps={},boxes={};map.setAttribute('aria-busy','true');
    visible.forEach(function(id){boxes[id]=sizes[id];reps[id]=[id];});
    controls.querySelectorAll('button').forEach(function(b){b.setAttribute('aria-pressed',b.dataset.role===selectedRole);});
    count.textContent=rmT('Showing {0} of {1} components',visible.length,total);
    try{
      var folded=repomapGraph.fold(edges,reps),stage=map.querySelector('[data-map-stage]'),width=stage?.clientWidth||map.clientWidth-48;
      // Only a repository with no source connections is a component grid.
      // Connected pictures still use the same graph layout engine, then pack
      // side by side at readable scale instead of stacking unrelated pictures.
      var readableWidth=width/(map.readableScale||1);
      var result=edges.length?await repomapGraph.layoutComponents(boxes,folded,readableWidth):componentGrid(visible,readableWidth);
      if(ticket!==revision)return;repomapPreview.freeze(map);
      Object.keys(nodes).forEach(function(id){var b=result.boxes[id];nodes[id].style.display=b?'':'none';if(!b)return;nodes[id].setAttribute('transform','translate('+(b.x-origins[id].x)+' '+(b.y-origins[id].y)+')');nodes[id].dataset.near=folded.filter(function(e){return e.from===id||e.to===id;}).map(function(e){return e.from===id?e.to:e.from;}).join(' ');});
      repomapGraph.draw(map,result.edges);svg.setAttribute('viewBox','0 0 '+result.width+' '+result.height);svg.setAttribute('width',result.width);svg.setAttribute('height',result.height);map.classList.remove('map-previewing');map.dispatchEvent(new CustomEvent('repomap:layout',{detail:{focus:visible.map(function(id){return result.boxes[id];}).filter(Boolean)}}));
      if(map.clearInspection)map.clearInspection();
    }catch(error){console.error('Repository map layout',error);}
    if(ticket===revision)map.setAttribute('aria-busy','false');
  }
  var layoutWidth=0;
  new ResizeObserver(function(){var width=map.clientWidth;if(width===layoutWidth)return;layoutWidth=width;if(width)render();}).observe(map);
  function restore(){var role=new URL(location.href).searchParams.get('component-role')||'';selectedRole=lanes.some(function(l){return l.role===role;})?role:'';requestAnimationFrame(render);}
  window.addEventListener('popstate',restore);restore();
});})();
