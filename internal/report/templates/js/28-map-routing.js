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
    var routed=(graph.edges||[]).map(function(e){var original=edges[+e.id.slice(5)],offset=offsets[e.container||'root'],points=[];
      var path=(e.sections||[]).map(function(s){var part=[s.startPoint].concat(s.bendPoints||[],[s.endPoint]).map(function(p){return{x:p.x+offset.x,y:p.y+offset.y};});points.push(...part);return part.map(function(p,i){return(i?'L ':'M ')+p.x+' '+p.y;}).join(' ');}).join(' ');
      return Object.assign({},original,{points:points,path:path});
    });
    return {boxes:placed,edges:routed,areas:frames,width:graph.width,height:graph.height};
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
      title.textContent=edge.relations.length+' connection'+(edge.relations.length===1?'':'s')+' · '+Array.from(new Set(edge.relations.map(function(e){return e.label;}))).join(', ');line.appendChild(title);
      line.setAttribute('tabindex','0');line.setAttribute('role','button');line.setAttribute('aria-label',title.textContent);
      function inspect(){map.dispatchEvent(new CustomEvent('repomap:connection',{detail:edge}));}
      function lift(){layer.appendChild(group);}
      line.addEventListener('mouseenter',lift);line.addEventListener('focus',lift);
      line.addEventListener('click',inspect);line.addEventListener('keydown',function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();inspect();}});
      group.appendChild(line);layer.appendChild(group);
    });
    return routed;
  }
  return {layout:layout,fold:fold,draw:draw};
})();

// The existing role areas are also a hierarchy. Keep the first (primary) area
// open, so applications are directly accessible without expanding every library.
(function(){document.querySelectorAll('.repo-map').forEach(function(map){
  var svg=map.querySelector('svg'),nodes={},origins={},sizes={},revision=0,openArea='';
  var lanes=Array.from(svg.querySelectorAll('.map-lanes text')).map(function(n,i){return{id:'repo-area-'+i,title:n.textContent,y:+n.getAttribute('y'),nodes:[]};});
  svg.querySelectorAll('.repo-node').forEach(function(n,i){var r=n.querySelector('rect'),id=n.id||'repo-unread-'+i,x=+r.getAttribute('x'),y=+r.getAttribute('y');sizes[id]={w:+r.getAttribute('width'),h:+r.getAttribute('height')};nodes[id]=n;origins[id]={x:x,y:y};var lane=lanes.filter(function(l){return l.y<=y;}).pop();if(lane)lane.nodes.push(id);});
  var edges=Array.from(svg.querySelectorAll('.map-edge')).map(function(e){return{from:e.dataset.from,to:e.dataset.to,possible:e.classList.contains('map-edge-possible'),label:(e.querySelector('title')||{}).textContent||'',operations:[]};}).filter(function(e){return nodes[e.from]&&nodes[e.to];});
  lanes=lanes.filter(function(l){return l.nodes.length;});openArea=lanes[0]?.id||'';
  var controls=document.createElement('div');controls.className='repo-area-controls';controls.setAttribute('role','group');controls.setAttribute('aria-label','Repository areas');map.prepend(controls);
  function makeButton(title,id){var b=document.createElement('button');b.type='button';b.textContent=title;b.dataset.area=id;b.addEventListener('click',function(){openArea=id;render();});controls.appendChild(b);}
  lanes.forEach(function(lane){
    makeButton(lane.title,lane.id);
    var n=document.createElementNS('http://www.w3.org/2000/svg','a');n.id=lane.id;n.setAttribute('href','#'+lane.id);n.setAttribute('class','map-node repo-node repo-area-node');
    n.dataset.node=n.id;n.dataset.title=lane.title;n.dataset.branch='repo-area';n.dataset.children=lane.nodes.join(' ');n.dataset.summary=lane.nodes.length+' components. Expand this area to explore them.';
    n.setAttribute('aria-label',lane.title+', '+lane.nodes.length+' components');
    var r=document.createElementNS('http://www.w3.org/2000/svg','rect');r.setAttribute('class','map-node-body');r.setAttribute('width',220);r.setAttribute('height',80);r.setAttribute('rx',7);n.appendChild(r);
    var text=document.createElementNS('http://www.w3.org/2000/svg','text');text.setAttribute('x',13);text.setAttribute('y',26);text.setAttribute('class','map-node-title');text.textContent=lane.title;n.appendChild(text);
    var count=document.createElementNS('http://www.w3.org/2000/svg','text');count.setAttribute('x',13);count.setAttribute('y',60);count.setAttribute('class','map-node-count');count.textContent=lane.nodes.length+' components · explore →';n.appendChild(count);
    n.addEventListener('click',function(e){e.preventDefault();e.stopImmediatePropagation();openArea=lane.id;render();});
    svg.querySelector('.map-nodes').appendChild(n);nodes[n.id]=n;origins[n.id]={x:0,y:0};sizes[n.id]={w:220,h:80};
  });
  makeButton('All components','*');
  svg.querySelector('.map-lanes').replaceChildren();
  async function render(){
    var ticket=++revision,visible=[],reps={},boxes={};map.setAttribute('aria-busy','true');
    lanes.forEach(function(lane){var expanded=openArea==='*'||lane.id===openArea||lane.nodes.length===1;
      if(expanded)visible.push(...lane.nodes);else visible.push(lane.id);
      lane.nodes.forEach(function(id){reps[id]=[expanded?id:lane.id];});
    });
    Object.keys(nodes).filter(function(id){return !lanes.some(function(l){return l.id===id||l.nodes.indexOf(id)>=0;});}).forEach(function(id){visible.push(id);reps[id]=[id];});
    visible.forEach(function(id){boxes[id]=sizes[id];});
    try{
      var folded=repomapGraph.fold(edges,reps),result=await repomapGraph.layout(boxes,folded,null,map.clientWidth-48);
      if(ticket!==revision)return;repomapPreview.freeze(map);
      Object.keys(nodes).forEach(function(id){var b=result.boxes[id];nodes[id].style.display=b?'':'none';if(!b)return;nodes[id].setAttribute('transform','translate('+(b.x-origins[id].x)+' '+(b.y-origins[id].y)+')');nodes[id].dataset.near=folded.filter(function(e){return e.from===id||e.to===id;}).map(function(e){return e.from===id?e.to:e.from;}).join(' ');});
      var opened=lanes.find(function(lane){return lane.id===openArea;});
      var focus=(opened?opened.nodes:visible).map(function(id){return result.boxes[id];}).filter(Boolean);
      repomapGraph.draw(map,result.edges);svg.setAttribute('viewBox','0 0 '+result.width+' '+result.height);svg.setAttribute('width',result.width);svg.setAttribute('height',result.height);map.classList.remove('map-previewing');map.dispatchEvent(new CustomEvent('repomap:layout',{detail:{focus:focus}}));
      controls.querySelectorAll('button').forEach(function(b){b.setAttribute('aria-pressed',b.dataset.area===openArea);});
      if(map.clearInspection)map.clearInspection();
    }catch(error){console.error('Repository map layout',error);}
    if(ticket===revision)map.setAttribute('aria-busy','false');
  }
  render();
});})();
