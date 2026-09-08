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

// Existing repository roles filter one component level. The overview is a
// direct projection of the same source nodes; only the connections view asks
// ELK for placement. Neither view creates parents or inferred connections.
(function(){document.querySelectorAll('.repo-map').forEach(function(map){
  var svg=map.querySelector('svg'),nodes={},origins={},sizes={},revision=0,selectedRole='',view='components';
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
  var modes=document.createElement('div');modes.className='repo-view-controls';modes.setAttribute('role','group');modes.setAttribute('aria-label',rmT('Repository components and their connections'));controls.before(modes);
  function updateURL(){
    var url=new URL(location.href);if(nodes[url.hash.slice(1)])url.hash='repository-map';
    if(selectedRole)url.searchParams.set('component-role',selectedRole);else url.searchParams.delete('component-role');
    if(view==='connections')url.searchParams.set('component-view',view);else url.searchParams.delete('component-view');
    history.pushState(history.state,'',url);render();
  }
  [['components','Components'],['connections','Connections']].forEach(function(item){
    var b=document.createElement('button');b.type='button';b.textContent=rmT(item[1]);b.dataset.view=item[0];
    b.addEventListener('click',function(){if(view!==item[0]){view=item[0];updateURL();}});modes.appendChild(b);
  });
  function makeButton(title,role,number){
    var b=document.createElement('button');b.type='button';b.textContent=title+' · '+number;b.dataset.role=role;
    b.addEventListener('click',function(){if(selectedRole!==role){selectedRole=role;updateURL();}});controls.appendChild(b);
  }
  makeButton(rmT('All components'),'',total);
  lanes.forEach(function(lane){makeButton(lane.title,lane.role,lane.nodes.length);});controls.appendChild(count);
  var overview=document.createElement('div');overview.className='repo-component-overview';overview.hidden=true;
  var hint=document.createElement('p');hint.className='repo-overview-hint';hint.textContent=rmT('Choose a component to explore its parts.');overview.appendChild(hint);
  var grid=document.createElement('div');grid.className='repo-component-grid';overview.appendChild(grid);svg.after(overview);
  var cards={},componentFacts=Array.from(document.querySelectorAll('#repository-map .cards>.card'));
  function titleOf(id){return nodes[id].dataset.title||nodes[id].querySelector('.repo-card-name').textContent;}
  function highlight(id){
    var near=new Set([id]);edges.forEach(function(e){if(e.from===id)near.add(e.to);if(e.to===id)near.add(e.from);});
    Object.keys(cards).forEach(function(key){cards[key].classList.toggle('repo-component-near',near.has(key)&&key!==id);cards[key].classList.toggle('repo-component-focused',key===id);});
  }
  function clearHighlight(){Object.values(cards).forEach(function(card){card.classList.remove('repo-component-near','repo-component-focused');});}
  Object.keys(nodes).forEach(function(id){
    var node=nodes[id],card=document.createElement('article'),href=node.getAttribute('href');card.className='repo-component-card';card.dataset.component=id;card.dataset.role=node.dataset.role;
    var open=document.createElement(href?'a':'div');open.className='repo-component-open';if(href)open.href=href;
    else{card.classList.add('repo-component-unavailable');open.setAttribute('aria-disabled','true');}
    var content=node.querySelector('.repo-card-content').cloneNode(true),name=content.querySelector('.repo-card-name'),nameText=name.textContent;
    name.replaceChildren();nameText.split(/([./_])/).forEach(function(part){name.appendChild(document.createTextNode(part));if(/^[./_]$/.test(part))name.appendChild(document.createElement('wbr'));});open.appendChild(content);
    if(href){var action=document.createElement('span');action.className='repo-component-action';action.textContent=rmT('Open component')+' →';open.appendChild(action);}
    card.appendChild(open);
    // Target facts already own the manifest and entrypoint anchors. Keep that
    // source context visible beside the component, including unnamed libraries.
    // Clone every source-bearing row, never choose a representative by name.
    var facts=componentFacts.find(function(f){return f.querySelector('h3 a')?.getAttribute('href')===href;}),sourceRows=facts?Array.from(facts.querySelectorAll('.target-facts dl>dd')).filter(function(row){return row.querySelector('.anchor');}):[];
    if(sourceRows.length){
      var sources=document.createElement('dl');sources.className='repo-component-sources';
      sourceRows.forEach(function(row){var label=row.previousElementSibling;if(label&&label.tagName==='DT')sources.appendChild(label.cloneNode(true));sources.appendChild(row.cloneNode(true));});card.appendChild(sources);
    }
    var incident=edges.filter(function(e){return e.from===id||e.to===id;});
    if(incident.length){
      var details=document.createElement('details'),summary=document.createElement('summary'),list=document.createElement('ul');details.className='repo-component-connections';summary.textContent=rmT('Connections')+' · '+incident.length;details.appendChild(summary);
      incident.forEach(function(edge){
        var other=edge.from===id?edge.to:edge.from,row=document.createElement('li'),otherHref=nodes[other].getAttribute('href'),link=document.createElement(otherHref?'a':'span'),direction=document.createElement('span');
        direction.className='repo-connection-direction';direction.textContent=edge.from===id?'→':'←';direction.setAttribute('aria-hidden','true');row.appendChild(direction);
        if(otherHref)link.href=otherHref;link.textContent=titleOf(other);link.setAttribute('aria-label',titleOf(edge.from)+' → '+titleOf(edge.to));row.appendChild(link);
        var label=document.createElement('span');label.className='repo-connection-label';label.textContent=edge.label;row.appendChild(label);
        if(edge.possible){var possible=document.createElement('span');possible.className='repo-connection-label';possible.textContent=rmT('Interpreted connection or possible dispatch');row.appendChild(possible);}
        list.appendChild(row);
      });details.appendChild(list);card.appendChild(details);
    }else{var empty=document.createElement('span');empty.className='repo-component-no-connections';empty.textContent=rmT('No connections recorded.');card.appendChild(empty);}
    card.addEventListener('mouseenter',function(){highlight(id);});card.addEventListener('mouseleave',function(){if(!card.contains(document.activeElement))clearHighlight();});
    card.addEventListener('focusin',function(){highlight(id);});card.addEventListener('focusout',function(event){if(!card.contains(event.relatedTarget))clearHighlight();});
    cards[id]=card;grid.appendChild(card);
  });
  svg.querySelector('.map-lanes').replaceChildren();
  async function render(){
    if(!map.clientWidth)return;
    var ticket=++revision,visible=Object.keys(nodes).filter(function(id){return !selectedRole||nodes[id].dataset.role===selectedRole;}),reps={},boxes={};map.setAttribute('aria-busy','true');
    visible.forEach(function(id){boxes[id]=sizes[id];reps[id]=[id];});
    controls.querySelectorAll('button').forEach(function(b){b.setAttribute('aria-pressed',b.dataset.role===selectedRole);});
    modes.querySelectorAll('button').forEach(function(b){b.setAttribute('aria-pressed',b.dataset.view===view);});
    count.textContent=rmT('Showing {0} of {1} components',visible.length,total);
    map.classList.toggle('repo-map-overview',view==='components');overview.hidden=view!=='components';
    Object.keys(cards).forEach(function(id){cards[id].hidden=!reps[id];});
    if(view==='components'){
      repomapPreview.freeze(map);clearHighlight();map.classList.remove('map-previewing');
      if(map.clearInspection)map.clearInspection();map.setAttribute('aria-busy','false');return;
    }
    try{
      var folded=repomapGraph.fold(edges,reps),stage=map.querySelector('[data-map-stage]'),width=stage?.clientWidth||map.clientWidth-48;
      var result=await repomapGraph.layoutComponents(boxes,folded,width/(map.readableScale||1));
      if(ticket!==revision)return;repomapPreview.freeze(map);
      Object.keys(nodes).forEach(function(id){var b=result.boxes[id];nodes[id].style.display=b?'':'none';if(!b)return;nodes[id].setAttribute('transform','translate('+(b.x-origins[id].x)+' '+(b.y-origins[id].y)+')');nodes[id].dataset.near=folded.filter(function(e){return e.from===id||e.to===id;}).map(function(e){return e.from===id?e.to:e.from;}).join(' ');});
      repomapGraph.draw(map,result.edges);svg.setAttribute('viewBox','0 0 '+result.width+' '+result.height);svg.setAttribute('width',result.width);svg.setAttribute('height',result.height);map.classList.remove('map-previewing');map.dispatchEvent(new CustomEvent('repomap:layout',{detail:{focus:visible.map(function(id){return result.boxes[id];}).filter(Boolean)}}));
      if(map.clearInspection)map.clearInspection();
    }catch(error){console.error('Repository map layout',error);}
    if(ticket===revision)map.setAttribute('aria-busy','false');
  }
  // Existing "on the map" links still address their original SVG identity.
  // In the overview, reveal its visible card rather than a hidden SVG anchor.
  map.revealNode=async function(node){
    var id=node?.id;if(!cards[id])return;
    if(selectedRole&&nodes[id].dataset.role!==selectedRole){
      selectedRole='';var url=new URL(location.href);url.searchParams.delete('component-role');history.replaceState(history.state,'',url);
    }
    await render();
    if(view==='components'){
      cards[id].scrollIntoView({block:'center'});cards[id].querySelector('a')?.focus({preventScroll:true});highlight(id);
    }else{node.scrollIntoView({block:'center',inline:'center'});if(map.showNode)map.showNode(node);}
  };
  var layoutWidth=0;
  new ResizeObserver(function(){var width=map.clientWidth;if(width===layoutWidth)return;layoutWidth=width;if(width)render();}).observe(map);
  function restore(){var params=new URL(location.href).searchParams,role=params.get('component-role')||'';selectedRole=lanes.some(function(l){return l.role===role;})?role:'';view=params.get('component-view')==='connections'?'connections':'components';requestAnimationFrame(function(){var node=nodes[location.hash.slice(1)];if(node)map.revealNode(node);else render();});}
  window.addEventListener('popstate',restore);window.addEventListener('hashchange',restore);restore();
});})();
