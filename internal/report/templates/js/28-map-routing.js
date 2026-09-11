// Disclosures change the first view only; the complete original rows stay in HTML.
document.querySelectorAll('[data-input-group],[data-integration-group]').forEach(function(group){
  var items=Array.from(group.querySelectorAll('[data-input-item],[data-integration-item]'));
  var kinds=new Set(items.map(function(item){return item.querySelector('.input-kind')?.textContent||'';}));
  if(kinds.size===1)group.querySelectorAll('.input-kind').forEach(function(kind){kind.hidden=true;});
  var sources=new Set(items.map(function(item){return item.dataset.sourceKind||'fact';}));
  if(sources.size>1)items.forEach(function(item){var label=document.createElement('span');label.className='input-source';label.textContent=rmT(item.dataset.sourceKind==='model'?'Model interpretation':'Observed in source');item.appendChild(label);});
  if(items.length<=5)return;
  var lists=Array.from(group.querySelectorAll('ul.operation-catalog,ul.route-catalog'));
  var preview=document.createElement('ul');preview.className='plain operation-catalog';
  var rest=document.createElement('ul');rest.className=preview.className;
  items.forEach(function(item,index){(index<5?preview:rest).appendChild(item);});
  lists.forEach(function(list){list.remove();});group.querySelectorAll('.input-more').forEach(function(more){more.remove();});
  group.appendChild(preview);
  var more=document.createElement('details');more.className='input-more';
  var summary=document.createElement('summary');summary.textContent=rmT('All {0} →',items.length);
  more.append(summary,rest);group.appendChild(more);
});

// Folding retains evidence; ELK owns placement, ports and obstacle-free routing.
// The pinned engine is embedded in the report: no network or analysis is needed.
var repomapGraph = (function () {
  var engine=new ELK();
  async function layout(boxes,edges,areas,availableWidth){
    var children=Object.keys(boxes).map(function(id){return{id:id,width:boxes[id].w,height:boxes[id].h};});
    var options={'elk.algorithm':'layered','elk.direction':'RIGHT','elk.edgeRouting':'ORTHOGONAL','elk.randomSeed':'1','elk.hierarchyHandling':'INCLUDE_CHILDREN','elk.padding':'[top=36,left=36,bottom=36,right=36]','elk.spacing.nodeNode':'44','elk.spacing.edgeNode':'24','elk.spacing.edgeEdge':'12','elk.layered.spacing.nodeNodeBetweenLayers':'90','elk.layered.spacing.edgeNodeBetweenLayers':'28','elk.layered.spacing.edgeEdgeBetweenLayers':'12','elk.layered.mergeEdges':'false','elk.separateConnectedComponents':'true'};
    if(areas && areas.length){
      var assigned=new Set();
      var compound=areas.map(function(area){var members=children.filter(function(n){return area.nodes.indexOf(n.id)>=0;});members.forEach(function(n){assigned.add(n.id);});return{id:area.id,children:members,layoutOptions:Object.assign({},options,{'elk.padding':'[top=64,left=24,bottom=24,right=24]'})};}).filter(function(a){return a.children.length;});
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
    var allEdges=[];function collectEdges(parent){(parent.edges||[]).forEach(function(e){allEdges.push({edge:e,container:parent.id});});(parent.children||[]).forEach(collectEdges);}collectEdges(graph);
    var routed=allEdges.map(function(item){var e=item.edge,original=edges[+e.id.slice(5)],offset=offsets[e.container||item.container];
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
        var key=from+'\x00'+to;
        if(!result.has(key))result.set(key,{from:from,to:to,possible:e.possible,relations:[]});
        var folded=result.get(key);folded.possible=folded.possible||e.possible;folded.relations.push(e);
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

function rmEl(tag,cls,text){var item=document.createElement(tag);if(cls)item.className=cls;if(text!==undefined)item.textContent=text;return item;}
// The product's counted inventory links: inputs, parts and communication.
function rmProductCatalog(page,href){
  var catalog=rmEl('ul','product-catalog');
  [['Inputs','inputCount','inbound'],['Parts','partCount','parts'],['External communication','integrationCount','external']].forEach(function(item){
    var number=Number(page?.dataset[item[1]]||0);if(!number)return;
    var value=rmEl('li',''),link=rmEl('a','');link.href=href+(item[2]?'-'+item[2]:'');link.setAttribute('aria-label',rmT(item[0])+': '+number);
    link.appendChild(rmEl('span','product-catalog-label',rmT(item[0])));link.appendChild(rmEl('span','product-catalog-count',String(number)));value.appendChild(link);catalog.appendChild(value);
  });
  return catalog.childNodes.length?catalog:null;
}
// The first-screen entrance of one product: its input, activity and
// communication groups, five rows each, copied from the component page.
function rmBuildEntrance(inputs){
  var entrance=rmEl('div','repo-inputs');
  inputs.querySelectorAll('[data-input-group],[data-integration-group]').forEach(function(group){
    var block=rmEl('section',''),heading=group.querySelector('h4');
    block.appendChild(rmEl('h5','',heading.textContent));
    var list=rmEl('ul','');
    var items=Array.from(group.querySelectorAll('[data-input-item],[data-integration-item]'));
    items.slice(0,5).forEach(function(item){
      var row=rmEl('li','');row.appendChild(item.querySelector('.input-title').cloneNode(true));
      if(item.hasAttribute('data-integration-item')){
        ['.outbound-purpose','.outbound-address','.outbound-basis','.outbound-records'].forEach(function(selector){var detail=item.querySelector(selector);if(!detail)return;var copy=detail.cloneNode(true);copy.querySelectorAll('[id]').forEach(function(node){node.removeAttribute('id');});row.appendChild(copy);});
      }
      row.dataset.sourceKind=item.dataset.sourceKind||'fact';
      list.appendChild(row);
    });block.appendChild(list);
    var provenance=group.querySelector('.input-provenance');if(provenance)block.appendChild(provenance.cloneNode(true));
    if(items.length>5){var more=rmEl('a','input-all',rmT('All {0} →',items.length));more.href='#'+inputs.id;more.addEventListener('click',function(){group.querySelector('.input-more')?.setAttribute('open','');});block.appendChild(more);}
    entrance.appendChild(block);
  });
  return entrance;
}

// The repository is one addressable component space. Existing roles only
// arrange its nodes; selecting one opens its saved description and neighbours.
(function(){document.querySelectorAll('.repo-map').forEach(function(map,mapIndex){
  var svg=map.querySelector('svg'),nodes={},cards={},facts={},neighbourOpen={},selectedRole='',selected='',pointed='',selectedConnection=-1,frame=0;
  var lanes=Array.from(svg.querySelectorAll('.map-lanes text')).map(function(n){return{role:n.dataset.role,title:n.textContent,nodes:[]};});
  svg.querySelectorAll('.repo-node').forEach(function(node,index){
    var id=node.id||'repo-unread-'+index;nodes[id]=node;
    var lane=lanes.find(function(item){return item.role===node.dataset.role;});
    if(!lane){lane={role:node.dataset.role||'',title:node.querySelector('.repo-card-role')?.textContent||'',nodes:[]};lanes.push(lane);}
    lane.nodes.push(id);
  });
  lanes=lanes.filter(function(lane){return lane.nodes.length;});
  var ids=Object.keys(nodes),edges=Array.from(svg.querySelectorAll('.map-edge')).map(function(edge){
    return{from:edge.dataset.from,to:edge.dataset.to,possible:edge.classList.contains('map-edge-possible'),label:edge.querySelector('title')?.textContent||''};
  }).filter(function(edge){return nodes[edge.from]&&nodes[edge.to];});
  var componentFacts=Array.from(document.querySelectorAll('#repository-map .cards>.card'));
  ids.forEach(function(id){
    var href=nodes[id].getAttribute('href'),card=componentFacts.find(function(item){return item.querySelector('h3 a')?.getAttribute('href')===href;});
    facts[id]=card?Array.from(card.querySelectorAll('.target-facts dl>dd')).filter(function(row){return row.querySelector('.anchor');}):[];
  });
  function el(tag,cls,text){var item=document.createElement(tag);if(cls)item.className=cls;if(text!==undefined)item.textContent=text;return item;}
  function nameOf(id){return nodes[id].querySelector('.repo-card-name').textContent;}
  function breakName(item){var text=item.textContent;item.replaceChildren();text.split(/([./_])/).forEach(function(part){item.appendChild(document.createTextNode(part));if(/^[./_]$/.test(part))item.appendChild(document.createElement('wbr'));});}
  function incident(id,direction){return edges.filter(function(edge){return direction==='in'?edge.to===id:edge.from===id;});}
  function fullSources(id){
    var list=el('dl','repo-component-sources');
    facts[id].forEach(function(row){var label=row.previousElementSibling;if(label&&label.tagName==='DT')list.appendChild(label.cloneNode(true));list.appendChild(row.cloneNode(true));});
    return list;
  }
  var controls=el('div','repo-area-controls');controls.setAttribute('role','group');controls.setAttribute('aria-label',rmT('Show components'));
  var count=el('span','repo-filter-count');count.setAttribute('role','status');
  function roleButton(title,role,number){
    var button=el('button','',title+' · '+number);button.type='button';button.dataset.role=role;
    button.addEventListener('click',function(){if(selectedRole===role)return;selectedRole=role;selected='';pointed='';selectedConnection=-1;record();render();});controls.appendChild(button);
  }
  roleButton(rmT('All components'),'',ids.length);lanes.forEach(function(lane){if(lane.role)roleButton(lane.title,lane.role,lane.nodes.length);});controls.appendChild(count);map.prepend(controls);
  var overview=el('div','repo-component-overview');
  var space=el('div','repo-component-space');overview.appendChild(space);
  var links=document.createElementNS('http://www.w3.org/2000/svg','svg');links.classList.add('repo-space-links');links.setAttribute('role','group');links.setAttribute('aria-label',rmT('Repository components and their connections'));space.appendChild(links);
  var focus=el('section','repo-component-focus');focus.id='repo-component-focus-'+mapIndex;focus.hidden=true;focus.tabIndex=-1;overview.appendChild(focus);
  // The unmodified original SVG remains the source and the no-script map.
  svg.after(overview);map.classList.add('repo-map-space');
  lanes.forEach(function(lane){
    var auxiliary=!['Applications','Libraries'].includes(lane.role);
    var section=el(auxiliary?'details':'section','repo-role-space');section.dataset.role=lane.role;
    if(lane.title)section.appendChild(el(auxiliary?'summary':'h4','repo-role-heading',lane.title+' · '+lane.nodes.length));
    if(auxiliary)section.addEventListener('toggle',queueDraw);
    var grid=el('div','repo-component-grid');section.appendChild(grid);space.appendChild(section);lane.section=section;
    lane.nodes.forEach(function(id){
      var node=nodes[id],href=node.getAttribute('href'),card=el('article','repo-component-card');card.dataset.component=id;card.dataset.role=node.dataset.role;
      var button=el(href?'a':'div','repo-component-open');
      if(href)button.href=href;
      else{card.classList.add('repo-component-unavailable');button.setAttribute('aria-disabled','true');}
      var name=node.querySelector('.repo-card-name').cloneNode(true);breakName(name);button.appendChild(name);
      var meta=node.querySelector('.repo-card-meta')?.cloneNode(true);if(meta)button.appendChild(meta);
      if(!href){var note=node.querySelector('.repo-card-note');if(note)button.appendChild(note.cloneNode(true));}
      card.appendChild(button);
      if(href){
        var purpose=node.querySelector('.repo-card-purpose');if(purpose)card.appendChild(purpose.cloneNode(true));
        var page=document.getElementById(href.slice(1)),catalog=rmProductCatalog(page,href);
        if(catalog)card.appendChild(catalog);
        var inputs=page?.querySelector('.input-catalog');
        if(inputs)card.appendChild(rmBuildEntrance(inputs));
        var connectionCount=incident(id,'in').length+incident(id,'out').length;
        var inspect=el(connectionCount?'button':'span','repo-row-connections'+(connectionCount?'':' repo-row-connections-empty'));inspect.setAttribute('aria-label',rmT('Connections')+': '+connectionCount);
        inspect.appendChild(el('span','product-catalog-label',rmT('Connections')));inspect.appendChild(el('span','product-catalog-count',String(connectionCount)));inspect.hidden=!connectionCount;
        if(connectionCount){inspect.type='button';inspect.setAttribute('aria-controls',focus.id);inspect.addEventListener('click',function(){choose(id,true);});}card.appendChild(inspect);
      }
      var sources=el('div','repo-component-source-links'),seen=new Set();
      facts[id].forEach(function(row){row.querySelectorAll('.anchor').forEach(function(anchor){
        var key=(anchor.getAttribute('href')||'')+'|'+(anchor.dataset.open||'')+'|'+anchor.textContent;
        if(seen.has(key))return;seen.add(key);sources.appendChild(anchor.cloneNode(true));
      });});
      if(sources.childNodes.length)card.appendChild(sources);
      else{var path=node.querySelector('.repo-card-path');if(path)card.appendChild(path.cloneNode(true));}
      card.addEventListener('mouseenter',function(){pointed=id;emphasize();});
      card.addEventListener('mouseleave',function(){pointed='';emphasize();});
      card.addEventListener('focusin',function(){pointed=id;emphasize();});
      card.addEventListener('focusout',function(event){if(!card.contains(event.relatedTarget)){pointed='';emphasize();}});
      cards[id]=card;grid.appendChild(card);
    });
  });
  function record(){
    var url=new URL(location.href);url.hash=selected||'repository-map';
    if(selectedRole)url.searchParams.set('component-role',selectedRole);else url.searchParams.delete('component-role');
    // Old links to either presentation now address this same component space.
    url.searchParams.delete('component-view');
    if(selectedConnection>=0)url.searchParams.set('component-edge',selectedConnection);else url.searchParams.delete('component-edge');
    if(url.href!==location.href)history.pushState(history.state,'',url);
  }
  function choose(id,reveal){
    if(!cards[id]||!nodes[id].getAttribute('href'))return;
    if(selectedRole&&nodes[id].dataset.role!==selectedRole)selectedRole='';
    selected=id;pointed='';selectedConnection=-1;record();render();
    if(reveal)requestAnimationFrame(function(){focus.scrollIntoView({block:'nearest'});focus.focus({preventScroll:true});});
  }
  function relationList(id,direction){
    var box=el('section','repo-component-relations'),rows=incident(id,direction);
    box.appendChild(el('h5','',rmT(direction==='in'?'Uses this component':'This component uses')+' · '+rows.length));
    if(!rows.length){box.appendChild(el('p','repo-component-no-connections',rmT('No connections recorded.')));return box;}
    lanes.forEach(function(lane){
      var matching=rows.filter(function(edge){return lane.nodes.indexOf(direction==='in'?edge.from:edge.to)>=0;});if(!matching.length)return;
      var group=el('details','repo-neighbour-role'),summary=el('summary','',(lane.title||rmT('Components'))+' · '+matching.length),list=el('ul','repo-neighbour-list');
      var groupKey=id+'|'+direction+'|'+lane.role;
      group.appendChild(summary);group.open=!!neighbourOpen[groupKey]||matching.some(function(edge){return edges.indexOf(edge)===selectedConnection;});
      group.addEventListener('toggle',function(){if(group.isConnected)neighbourOpen[groupKey]=group.open;});
      matching.forEach(function(edge){
        var other=direction==='in'?edge.from:edge.to,row=el('li','repo-neighbour'),available=nodes[other].getAttribute('href');
        row.classList.toggle('repo-neighbour-selected',edges.indexOf(edge)===selectedConnection);
        var button=el(available?'button':'span','repo-neighbour-name',nameOf(other)+(available?' →':''));
        if(available){button.type='button';button.addEventListener('click',function(){choose(other,true);});}row.appendChild(button);
        row.appendChild(el('p','repo-connection-label',edge.label));
        if(edge.possible)row.appendChild(el('span','repo-connection-possible',rmT('Interpreted connection or possible dispatch')));
        list.appendChild(row);
      });group.appendChild(list);box.appendChild(group);
    });return box;
  }
  function describe(){
    focus.hidden=!selected;focus.replaceChildren();if(!selected)return;
    var id=selected,node=nodes[id],head=el('div','repo-focus-heading'),titles=el('div','');
    titles.appendChild(el('span','repo-focus-context',rmT('Selected component')));
    var title=el('h4','',nameOf(id));breakName(title);titles.appendChild(title);head.appendChild(titles);
    var close=el('button','repo-focus-close',rmT('Back to components'));close.type='button';
    close.addEventListener('click',function(){selected='';selectedConnection=-1;record();render();cards[id].scrollIntoView({block:'nearest'});cards[id].querySelector('button')?.focus({preventScroll:true});});head.appendChild(close);focus.appendChild(head);
    if(selectedConnection>=0){var edge=edges[selectedConnection],connection=el('p','repo-selected-connection');connection.appendChild(el('strong','',nameOf(edge.from)+' → '+nameOf(edge.to)));connection.appendChild(el('span','',edge.label));if(edge.possible)connection.appendChild(el('span','',rmT('Interpreted connection or possible dispatch')));focus.appendChild(connection);}
    var body=el('div','repo-focus-body'),description=el('div','repo-focus-description'),purpose=node.querySelector('.repo-card-purpose');
    if(purpose){var copy=purpose.cloneNode(true);copy.classList.add('model');copy.title=rmT('Model explanation');description.appendChild(copy);}
    var open=el('a','repo-open-parts',rmT('Open parts')+' →');open.href=node.getAttribute('href');description.appendChild(open);body.appendChild(description);
    if(facts[id].length)body.appendChild(fullSources(id));focus.appendChild(body);
    var relations=el('div','repo-focus-relations');relations.appendChild(relationList(id,'in'));relations.appendChild(relationList(id,'out'));focus.appendChild(relations);
  }
  function emphasize(){
    var subject=pointed||selected,near=new Set();
    if(subject)edges.forEach(function(edge){if(edge.from===subject)near.add(edge.to);if(edge.to===subject)near.add(edge.from);});
    space.classList.toggle('repo-space-emphasized',!!subject);
    ids.forEach(function(id){cards[id].classList.toggle('repo-component-selected',id===selected);cards[id].classList.toggle('repo-component-pointed',id===pointed);cards[id].classList.toggle('repo-component-near',near.has(id)&&id!==subject);});
    links.querySelectorAll('path[data-from]').forEach(function(path){path.classList.toggle('repo-space-edge-near',path.dataset.from===subject||path.dataset.to===subject);path.classList.toggle('repo-space-edge-selected',+path.dataset.edge===selectedConnection);});
  }
  function draw(){
    frame=0;if(!space.clientWidth)return;
    var bounds=space.getBoundingClientRect(),width=space.clientWidth,height=space.clientHeight,marker='repo-space-arrow-'+mapIndex;
    links.replaceChildren();links.setAttribute('viewBox','0 0 '+width+' '+height);
    var defs=document.createElementNS(links.namespaceURI,'defs'),arrow=document.createElementNS(links.namespaceURI,'marker');
    arrow.id=marker;arrow.setAttribute('viewBox','0 0 10 10');arrow.setAttribute('refX','9');arrow.setAttribute('refY','5');arrow.setAttribute('markerWidth','5');arrow.setAttribute('markerHeight','5');arrow.setAttribute('orient','auto');
    var tip=document.createElementNS(links.namespaceURI,'path');tip.setAttribute('d','M0 0 L10 5 L0 10 z');arrow.appendChild(tip);defs.appendChild(arrow);links.appendChild(defs);
    edges.forEach(function(edge,index){
      var from=cards[edge.from],to=cards[edge.to];if(from.hidden||to.hidden||from.closest('[hidden],details:not([open])')||to.closest('[hidden],details:not([open])'))return;
      var a=from.getBoundingClientRect(),b=to.getBoundingClientRect(),sx,sy,tx,ty,path;
      if(Math.abs(a.top-b.top)<Math.min(a.height,b.height)/2){
        var right=a.left<b.left;sx=(right?a.right:a.left)-bounds.left;tx=(right?b.left:b.right)-bounds.left;sy=a.top+a.height/2-bounds.top;ty=b.top+b.height/2-bounds.top;
        var bend=Math.max(18,Math.abs(tx-sx)/2);path='M'+sx+' '+sy+' C'+(sx+(right?bend:-bend))+' '+sy+' '+(tx+(right?-bend:bend))+' '+ty+' '+tx+' '+ty;
      }else{
        var down=a.top<b.top;sx=a.left+a.width/2-bounds.left;tx=b.left+b.width/2-bounds.left;sy=(down?a.bottom:a.top)-bounds.top;ty=(down?b.top:b.bottom)-bounds.top;
        var bend=Math.max(24,Math.abs(ty-sy)/2);path='M'+sx+' '+sy+' C'+sx+' '+(sy+(down?bend:-bend))+' '+tx+' '+(ty+(down?-bend:bend))+' '+tx+' '+ty;
      }
      var line=document.createElementNS(links.namespaceURI,'path');line.setAttribute('d',path);line.setAttribute('class','repo-space-edge'+(edge.possible?' repo-space-edge-possible':''));line.dataset.from=edge.from;line.dataset.to=edge.to;line.dataset.edge=index;line.setAttribute('marker-end','url(#'+marker+')');
      line.setAttribute('tabindex','0');line.setAttribute('role','button');line.setAttribute('aria-label',nameOf(edge.from)+' → '+nameOf(edge.to)+': '+edge.label);
      function inspect(){selected=selected===edge.to?selected:edge.from;selectedConnection=index;pointed='';record();render();requestAnimationFrame(function(){focus.scrollIntoView({block:'nearest'});focus.focus({preventScroll:true});});}
      var hit=document.createElementNS(links.namespaceURI,'path');hit.setAttribute('d',path);hit.setAttribute('class','repo-space-edge-hit');hit.setAttribute('aria-hidden','true');hit.addEventListener('click',inspect);links.appendChild(hit);
      line.addEventListener('click',inspect);line.addEventListener('keydown',function(event){if(event.key==='Enter'||event.key===' '){event.preventDefault();inspect();}});
      var title=document.createElementNS(links.namespaceURI,'title');title.textContent=edge.label;line.appendChild(title);links.appendChild(line);
    });emphasize();
  }
  function queueDraw(){if(!frame)frame=requestAnimationFrame(draw);}
  function render(){
    ids.forEach(function(id){cards[id].hidden=!!selectedRole&&nodes[id].dataset.role!==selectedRole;var button=cards[id].querySelector('button');if(button)button.setAttribute('aria-expanded',id===selected?'true':'false');});
    lanes.forEach(function(lane){lane.section.hidden=!!selectedRole&&lane.role!==selectedRole;});
    controls.querySelectorAll('button').forEach(function(button){button.setAttribute('aria-pressed',button.dataset.role===selectedRole?'true':'false');});
    count.hidden=!selectedRole;
    count.textContent=rmT('Showing {0} of {1} components',ids.filter(function(id){return !cards[id].hidden;}).length,ids.length);
    describe();emphasize();queueDraw();
    map.dataset.readingLabel=selected?nameOf(selected):'';map.dispatchEvent(new CustomEvent('repomap:reading',{bubbles:true}));
  }
  map.revealNode=async function(node){
    var id=node?.id;if(!cards[id])return;
    selected=id;pointed='';selectedConnection=-1;
    var url=new URL(location.href);url.searchParams.delete('component-edge');
    if(selectedRole&&nodes[id].dataset.role!==selectedRole){selectedRole='';url.searchParams.delete('component-role');}
    if(url.href!==location.href)history.replaceState(history.state,'',url);
    render();requestAnimationFrame(function(){var lane=cards[id].closest('details');if(lane)lane.open=true;cards[id].scrollIntoView({block:'start'});cards[id].querySelector('a')?.focus({preventScroll:true});});
  };
  new ResizeObserver(queueDraw).observe(space);
  function restore(){
    var url=new URL(location.href),role=url.searchParams.get('component-role')||'',id=url.hash.slice(1);
    selectedRole=lanes.some(function(lane){return lane.role===role;})?role:'';
    selected=nodes[id]&&nodes[id].getAttribute('href')?id:'';pointed='';
    var edgeIndex=url.searchParams.has('component-edge')?Number(url.searchParams.get('component-edge')):-1;
    selectedConnection=Number.isInteger(edgeIndex)&&edges[edgeIndex]&&(edges[edgeIndex].from===selected||edges[edgeIndex].to===selected)?edgeIndex:-1;
    if(selected&&selectedRole&&nodes[selected].dataset.role!==selectedRole){selectedRole='';url.searchParams.delete('component-role');history.replaceState(history.state,'',url);}
    if(selected){var lane=cards[selected].closest('details');if(lane)lane.open=true;}
    render();
  }
  window.addEventListener('popstate',restore);window.addEventListener('hashchange',restore);restore();
});})();

// A report with one component has no repository map. Its product still opens
// the first screen the same way: name, purpose, counts and the inputs,
// commands and communication entrance, ahead of the question list.
(function(){
  if(document.querySelector('.repo-map'))return;
  var panel=document.getElementById('repository-map');if(!panel)return;
  var sources=Array.from(panel.querySelectorAll('.cards>.card'));if(!sources.length)return;
  var grid=rmEl('div','repo-component-grid repo-single-product');
  sources.forEach(function(source){
    var link=source.querySelector('h3 a');if(!link)return;
    var href=link.getAttribute('href'),page=document.getElementById(href.slice(1));
    var card=rmEl('article','repo-component-card');
    var open=rmEl('a','repo-component-open');open.href=href;open.appendChild(rmEl('span','repo-card-name',link.textContent));
    var meta=source.querySelector('.meta');if(meta)open.appendChild(rmEl('span','repo-card-meta',meta.textContent));
    card.appendChild(open);
    var purpose=source.querySelector('p.model');if(purpose){var text=purpose.cloneNode(true);text.className='repo-card-purpose';card.appendChild(text);}
    var catalog=rmProductCatalog(page,href);if(catalog)card.appendChild(catalog);
    var inputs=page?.querySelector('.input-catalog');if(inputs)card.appendChild(rmBuildEntrance(inputs));
    grid.appendChild(card);
  });
  panel.insertBefore(grid,panel.querySelector('.evidence-list'));
})();
