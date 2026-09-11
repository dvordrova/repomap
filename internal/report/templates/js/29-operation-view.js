// Structure and operation context are two views of the same retained graph.
// Opening a scope changes the projection, never the underlying evidence.
(function () {
  document.querySelectorAll('[data-map-explorer]').forEach(function(map){
    var svg=map.querySelector('svg'), stage=map.querySelector('[data-map-stage]');
    var nodes=Array.from(map.querySelectorAll('[data-node]')), byID={}, origins={}, parents={};
    nodes.forEach(function(n){byID[n.id]=n;var r=n.querySelector('rect');origins[n.id]={x:+r.getAttribute('x'),y:+r.getAttribute('y')};children(n).forEach(function(id){(parents[id]||(parents[id]=[])).push(n.id);});});
    function children(n){return (n.dataset.children||'').split(/\s+/).filter(Boolean);}
    function leaves(id,seen){seen=seen||new Set();if(seen.has(id))return [];seen.add(id);var n=byID[id];if(!n)return [];var list=children(n);return list.length?list.flatMap(function(c){return leaves(c,new Set(seen));}):[id];}
    // An identically named area with one part adds no navigable subdivision.
    // Keep its original node and description, but open that part directly.
    // A remote area may contain only the subset seen from this component, so
    // its one visible member is not enough to establish a single-part area.
    var directParts={},areaDescriptions={};
    nodes.forEach(function(n){
      var ids=children(n),part=ids.length===1&&byID[ids[0]];
      if(n.dataset.branch==='area'&&n.dataset.remote!=='true'&&part&&!part.dataset.branch&&!part.dataset.activation&&n.dataset.canonicalTitle===part.dataset.canonicalTitle){
        directParts[n.id]=part.id;(areaDescriptions[part.id]||(areaDescriptions[part.id]=[])).push(n.dataset.summary);
      }
    });
    function displayed(id){return directParts[id]||id;}
    function displayedSet(ids){return Array.from(new Set(ids.map(displayed)));}
    var nearOf={};nodes.forEach(function(n){nearOf[n.id]=(n.dataset.near||'').split(/\s+/).filter(Boolean);});
    var ops=nodes.filter(function(n){return n.dataset.activation;}), groups=nodes.filter(function(n){return !n.dataset.activation;});
    var roots=displayedSet(groups.filter(function(n){return !parents[n.id];}).map(function(n){return n.id;}));
    var rawEdges=Array.from(svg.querySelectorAll('.map-edge')).map(function(e){return {from:e.dataset.from,to:e.dataset.to,scope:e.dataset.scope,summary:e.dataset.summary,summaryRef:e.dataset.summaryRef,labelRef:e.dataset.labelRef,fromSource:e.dataset.fromSource,fromText:e.dataset.fromText,fromNoSource:e.dataset.fromNoSource==='true',toSource:e.dataset.toSource,toText:e.dataset.toText,toNoSource:e.dataset.toNoSource==='true',operations:(e.dataset.operations||'').split(/\s+/).filter(Boolean),possible:e.classList.contains('map-edge-possible'),label:(e.querySelector('title')||{}).textContent||''};});
    svg.querySelector('.map-frames').replaceChildren();svg.querySelector('.map-lanes').replaceChildren();svg.querySelector('.map-edge-labels').replaceChildren();
    var oldControls=map.querySelector('[data-operation-controls]');if(oldControls)oldControls.remove();
    var bar=document.createElement('div');bar.className='explorer-controls';
    bar.innerHTML=("<div class=\"explorer-modes\" role=\"group\" aria-label=\""+rmT.html("Explore by")+"\"><button type=\"button\" data-structure>"+rmT.html("Structure")+"</button><button type=\"button\" data-operations>"+rmT.html("Operations")+"</button></div><label class=\"explorer-search\">"+rmT.html("Find a part")+" <input type=\"search\" placeholder=\""+rmT.html("Name or description")+"\" aria-label=\""+rmT.html("Find a part")+"\"></label><button type=\"button\" data-all-uses>"+rmT.html("Clear selection")+"</button><nav class=\"explorer-breadcrumbs\" aria-label=\""+rmT.html("Map path")+"\"></nav>");
    map.prepend(bar);
    // Fit, zoom and reset belong with the other view controls, not on a row
    // of their own above the map.
    var zoomControls=map.querySelector('[data-map-controls]'),crumbRow=bar.querySelector('.explorer-breadcrumbs');
    if(zoomControls){if(crumbRow)bar.insertBefore(zoomControls,crumbRow);else bar.appendChild(zoomControls);}
    var structure=bar.querySelector('[data-structure]'), operationMode=bar.querySelector('[data-operations]'), search=bar.querySelector('input'), allUses=bar.querySelector('[data-all-uses]'), crumbs=bar.querySelector('nav');
    operationMode.hidden=!ops.length;
    var operationPicker=document.createElement('div');operationPicker.className='operation-picker';operationPicker.hidden=true;
    operationPicker.innerHTML=("<label>"+rmT.html("Find an operation")+" <input type=\"search\" placeholder=\""+rmT.html("Command, endpoint, activity")+"\" aria-label=\""+rmT.html("Find an operation")+"\"></label><div class=\"operation-choices\"></div>");
    bar.after(operationPicker);
    var choices=operationPicker.querySelector('.operation-choices'), opSearch=operationPicker.querySelector('input');
    var choiceCount=document.createElement('span');choiceCount.className='operation-choice-count';
    operationPicker.querySelector('label').appendChild(choiceCount);
    var visit=null, returnPath=document.createElement('div');returnPath.className='explorer-return';returnPath.hidden=true;bar.appendChild(returnPath);
    var trail=[], operation=null, pinned=false, mode='structure', scope='', visibleIDs=[], currentEdges=[], revision=0;
    var scopeVisits=new Map();
    var historyRestoreHash='', historyRestoreKey='', historyRestorePromise=null, historyRevision=0;
    function abandonHistoryRestore(){historyRestoreHash='';historyRestoreKey='';historyRevision++;map.readingRestoring=false;}
    function button(text,fn){var b=document.createElement('button');b.type='button';b.textContent=text;b.addEventListener('click',fn);return b;}
    function show(n){if(map.showNode)map.showNode(n);}
    function partOwner(node){
      var href=node.getAttribute('href')||'',destination=href[0]==='#'&&document.getElementById(href.slice(1));
      var owner=destination&&destination.closest('[data-report-page]');
      return (owner||map.closest('[data-report-page]')).dataset.componentName;
    }
    function followForeign(node,allUses,source){
      if(!node||node.dataset.remote!=='true'||node.dataset.branch)return false;
      var href=node.getAttribute('href')||'',destination=href[0]==='#'&&document.getElementById(href.slice(1));
      var owner=destination&&destination.closest('[data-report-page]'),other=owner&&owner.querySelector('[data-map-explorer]');
      if(!owner||owner===map.closest('[data-report-page]'))return false;
      // Match only the existing exact destination, never a symbol name.
      // A cross-target stub opens the owning page, not another copy here.
      var canonical=other&&Array.from(other.querySelectorAll('[data-node]')).find(function(candidate){return candidate.dataset.remote!=='true'&&(candidate===destination||candidate.getAttribute('href')===href);});
      if(canonical&&other.revealNode)other.revealNode(canonical,allUses,source);
      else {document.dispatchEvent(new CustomEvent('repomap:navigate',{detail:{destination:destination}}));destination.scrollIntoView({block:'start'});}
      return true;
    }
    ops.forEach(function(op){
      var b=button(op.dataset.title,async function(){var same=operation===op;operation=op;pinned=true;if(!same){scope='';trail=[];visit=null;}search.value='';address(op);await render();revealChoice();show(op);orient();});b.dataset.operationChoice=op.id;
      b.title=op.dataset.summary||'';
      var desc=document.createElement('span');desc.textContent=rmT(op.dataset.activation)+' · '+op.dataset.operationGroup;b.appendChild(desc);choices.appendChild(b);
      if(ops.some(function(other){return other!==op&&other.dataset.title===op.dataset.title;})&&op.dataset.sourceText){
        var source=document.createElement('span');source.className='operation-choice-source';source.textContent=op.dataset.sourceText;b.appendChild(source);
      }
    });
    function updateChoices(){
      var matching=Array.from(choices.children).filter(function(b){return !b.hidden;}).length;
      choiceCount.textContent=matching===ops.length?rmT('{0} operations',matching):rmT('{0} of {1} operations',matching,ops.length);
    }
    function filterChoices(){var term=opSearch.value.toLowerCase();choices.querySelectorAll('button').forEach(function(b){var n=byID[b.dataset.operationChoice];b.hidden=(n.dataset.title+' '+n.dataset.summary+' '+n.dataset.activation+' '+n.dataset.sourceText).toLowerCase().indexOf(term)<0;});updateChoices();}
    function revealChoice(keepFilter){
      if(operation&&!keepFilter){opSearch.value='';filterChoices();}
    }
    opSearch.addEventListener('input',filterChoices);
    function allowed(){return operation?new Set(nearOf[operation.id].concat(operation.id)):null;}
    function applicable(id,limit){return !limit||leaves(id).some(function(leaf){return limit.has(leaf);});}
    function scopePath(id,seen){
      seen=seen||new Set();if(!id||seen.has(id))return [];seen.add(id);
      var parent=(parents[id]||[]).find(function(p){return !seen.has(p);});
      return scopePath(parent,seen).concat(directParts[id]?[]:[id]);
    }
    function setScope(id){scope=displayed(id);trail=scopePath(scope).slice(0,-1);}
    // The explanation stands beside the map, so a click never needs the page
    // to move; only a map that is entirely out of view (a reveal from a
    // question or a search) is brought back.
    function stageWidth(){
      var stage=map.querySelector('.map-stage');
      if(stage&&stage.clientWidth)return stage.clientWidth;
      var mapStyle=getComputedStyle(map),width=map.clientWidth-parseFloat(mapStyle.paddingLeft)-parseFloat(mapStyle.paddingRight);
      if(window.innerWidth>1100)width-=Math.min(24*16,Math.max(17*16,width*.32))+16;
      return width;
    }
    function orient(){
      var inset=parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--toolbar-height'))||0;
      var rect=(map.querySelector('.map-stage')||bar).getBoundingClientRect();
      if(rect.bottom<inset||rect.top>window.innerHeight)map.scrollIntoView({block:'start'});
    }
    function address(node){document.dispatchEvent(new CustomEvent('repomap:navigate',{detail:{destination:node||map.closest('[data-report-page]')}}));}
    function readingDisclosures(){return {members:!!map.querySelector('.map-all-members')?.open};}
    function restoreDisclosures(saved){var members=map.querySelector('.map-all-members');if(members)members.open=!!saved?.members;}
    function snapshot(){
      var member=map.explorerMember;
      return {scope:scope,operation:operation,pinned:pinned,mode:mode,search:search.value,
        source:member&&member.owner===scope?{href:member.href,open:member.open,key:member.key}:null,
        label:member&&member.owner===scope?rmT('{0} in {1}',member.name,byID[scope].dataset.title):scope?byID[scope].dataset.title:map.closest('[data-report-page]').dataset.componentName,
        disclosures:readingDisclosures(),viewport:map.captureViewport?.(),scroll:window.scrollY};
    }
    async function restore(saved){
      setScope(saved.scope);operation=saved.operation;pinned=saved.pinned;mode=saved.mode;search.value=saved.search;
      address(byID[scope]);await render();restoreDisclosures(saved.disclosures);if(saved.source)map.explainSource(saved.source);
      if(saved.viewport)map.restoreViewport?.(saved.viewport);window.scrollTo({top:saved.scroll,behavior:'instant'});
    }
    function scopeKey(id){return [mode,operation?.id||'',id].join('\0');}
    async function open(id){
      id=displayed(id);if((id&&!byID[id])||scope===id)return;
      if(followForeign(byID[id]))return;
      abandonHistoryRestore();
      scopeVisits.set(scopeKey(scope),snapshot());
      var previous=(!id||trail.indexOf(id)>=0)&&scopeVisits.get(scopeKey(id));
      if(previous){await restore(previous);return;}
      var next=byID[id];
      if(!operation)visit=null;
      if(next?.dataset.activation){operation=next;pinned=true;mode='operations';setScope('');}else setScope(id);
      search.value='';address(next);await render();orient();
    }
    function showReturnPath(){
      returnPath.replaceChildren();returnPath.hidden=!visit;if(!visit)return;
      var previous=visit;
      returnPath.append(rmT('Opened')+' ',button(previous.operation.dataset.title,async function(){operation=previous.operation;pinned=true;mode='operations';address(operation);await render();revealChoice();show(operation);orient();}),' '+rmT('from')+' ',button(previous.origin.label,async function(){
        operation=null;pinned=false;mode='structure';setScope(previous.origin.node.id);search.value='';address(byID[scope]);await render();
        if(previous.origin.source)map.explainSource(previous.origin.source);orient();
      }));
    }
    async function render(){
      var ticket=++revision;map.setAttribute('aria-busy','true');
      // A root view whose only own node is one area would show a single box
      // reading "Open parts · N". Its parts are the first thing worth seeing,
      // so that area is open from the start; the crumbs still name it.
      if(!scope&&!operation&&!search.value.trim()){
        var own=roots.filter(function(id){return byID[id].dataset.remote!=='true';});
        if(own.length===1&&byID[own[0]].dataset.branch==='area')setScope(own[0]);
      }
      var previousViewport=operation&&map.explorerOperation===operation?map.captureViewport?.():null;
      var area=!operation&&!search.value.trim()?scopePath(scope).map(function(id){return byID[id];}).filter(function(n){return n?.dataset.branch==='area';}).pop():null;
      var viewScope=area?area.id:scope;
      var limit=allowed(), scopeLeaves=viewScope?new Set(leaves(viewScope)):null;
      var edges=rawEdges.filter(function(e){return operation?e.scope==='operation'&&e.operations.indexOf(operation.id)>=0:e.scope==='structure';});
      var base=viewScope&&children(byID[viewScope]).length?children(byID[viewScope]):roots.filter(function(id){return byID[id].dataset.remote!=='true';});
      base=base.filter(function(id){return applicable(id,limit);});
      var term=search.value.trim().toLowerCase();
      if(term)base=groups.filter(function(n){return applicable(n.id,limit)&&(!scopeLeaves||leaves(n.id).some(function(id){return scopeLeaves.has(id);}))&&(n.dataset.title+' '+n.dataset.summary+' '+repomapMembers.items(n).map(repomapMembers.displayName).join(' ')).toLowerCase().indexOf(term)>=0;}).map(function(n){return n.id;});
      base=displayedSet(base);
      var inside=area?scopeLeaves:term?new Set(base.flatMap(function(id){return leaves(id);})):null;
      if(operation&&!term){
        base=groups.filter(function(n){return !children(n).length&&applicable(n.id,limit);}).map(function(n){return n.id;});
        inside=null;
      }
      var relevant=edges.filter(function(e){return !inside||inside.has(e.from)||inside.has(e.to);});
      var endpoints=new Set(relevant.flatMap(function(e){return [e.from,e.to];}));
      // A boundary is folded at the nearest unopened area/component. Edges
      // between those boundary nodes stay in their own scope, not this one.
      var represented=new Set(base.flatMap(function(id){return leaves(id);}));
      function boundaryAt(id){
        var members=leaves(id);
        if(!members.some(function(leaf){return endpoints.has(leaf)&&!represented.has(leaf);}))return [];
        // A peer cannot contain the part already open. Descend shared
        // ancestors until the peer and the current scope are disjoint.
        if(members.some(function(leaf){return represented.has(leaf);}))return children(byID[id]).flatMap(boundaryAt);
        return [id];
      }
      var boundary=roots.flatMap(boundaryAt);
      var visible=Array.from(new Set(base.concat(boundary))).filter(function(id){return applicable(id,limit);});
      if(operation&&!term)visible=[operation.id].concat(visible.filter(function(id){return id!==operation.id;}));
      var representatives={};
      visible.forEach(function(id){leaves(id).forEach(function(leaf){
        if(represented.has(leaf)&&base.indexOf(id)<0)return;
        (representatives[leaf]||(representatives[leaf]=[])).push(id);
      });});
      currentEdges=repomapGraph.fold(relevant,representatives,inside);
      if(area&&!term){
        var outer={};Object.keys(representatives).forEach(function(id){outer[id]=inside.has(id)?[area.id]:representatives[id];});
        currentEdges=repomapGraph.fold(relevant.filter(function(e){return inside.has(e.from)&&inside.has(e.to);}),representatives,inside)
          .concat(repomapGraph.fold(relevant.filter(function(e){return !inside.has(e.from)||!inside.has(e.to);}),outer,inside));
      }
      // Standalone scope: order incoming peers, the subject, outgoing peers.
      // Areas use a compact grid, retaining their own original child names.
      visible.sort(function(a,b){if(!operation){if(a===scope)return -1;if(b===scope)return 1;}if(a===operation?.id)return -1;if(b===operation?.id)return 1;var ai=base.indexOf(a)>=0,bi=base.indexOf(b)>=0;if(ai!==bi)return ai?-1:1;return byID[a].dataset.title.localeCompare(byID[b].dataset.title);});
      var boxes={};visible.forEach(function(id){boxes[id]={w:220,h:80};});
      var layout;
      // The explanation panel takes the right third of the workspace on wide
      // screens, so the map's width is the stage's. The first render starts
      // before the shared inspector mounts the stage; one microtask later the
      // whole bundle has run and the stage can be measured. A hidden page
      // takes the same split from the figure's width.
      await Promise.resolve();if(ticket!==revision)return;
      var availableWidth=stageWidth();
      var opened=scope&&byID[scope], expanded=!term&&opened&&!opened.dataset.branch&&!opened.dataset.activation?opened:null;
      map.classList.toggle('map-part-expanded',!!expanded);map.classList.toggle('map-operation-context',!!operation);
      if(expanded&&boxes[expanded.id])boxes[expanded.id]=repomapMembers.size(expanded);
      {
      visible.forEach(function(id){
        var node=byID[id],display=node.style.display,visibility=node.style.visibility;
        node.style.display='';node.style.visibility='hidden';
        node.querySelectorAll('.map-node-title').forEach(function(title){boxes[id].w=Math.max(boxes[id].w,Math.ceil(title.getComputedTextLength())+22);});
        node.style.display=display;node.style.visibility=visibility;
      });
      try{layout=await repomapGraph.layout(boxes,currentEdges,area?[{id:area.id,nodes:base}]:null,availableWidth);}catch(error){
        if(ticket!==revision)return;map.setAttribute('aria-busy','false');
        var message=bar.querySelector('[role="alert"]');if(!message){message=document.createElement('p');message.setAttribute('role','alert');bar.appendChild(message);}message.textContent=rmT('Could not arrange this map. Try another scope.');console.error('Component map layout',error);return;
      }
      if(ticket!==revision)return;
      repomapPreview.freeze(map);boxes=layout.boxes;
      var frameLayer=svg.querySelector('.map-frames');frameLayer.replaceChildren();
      (layout.areas||[]).forEach(function(frame){
        var group=document.createElementNS('http://www.w3.org/2000/svg','g');group.classList.add('map-expanded-area');group.dataset.areaFrame=frame.id;
        var rect=document.createElementNS('http://www.w3.org/2000/svg','rect');
        Object.entries({x:frame.x,y:frame.y,width:frame.w,height:frame.h,rx:12}).forEach(function(pair){rect.setAttribute(pair[0],pair[1]);});group.appendChild(rect);
        var label=document.createElementNS('http://www.w3.org/2000/svg','text');label.setAttribute('x',frame.x+20);label.setAttribute('y',frame.y+27);label.textContent=byID[frame.id].dataset.title;group.appendChild(label);
        frameLayer.appendChild(group);
      });
      var routed=repomapGraph.draw(map,layout.edges);
      nodes.forEach(function(n){var b=boxes[n.id];n.style.display=b?'':'none';if(!b)return;var origin=origins[n.id];n.setAttribute('transform','translate('+(b.x-origin.x)+' '+(b.y-origin.y)+')');var rect=n.querySelector('rect');rect.setAttribute('width',b.w);rect.setAttribute('height',b.h);n.classList.toggle('map-area',!!n.dataset.branch);n.classList.toggle('operation-selected',n===operation);if(!n.dataset.activation)n.dataset.near=currentEdges.filter(function(e){return e.from===n.id||e.to===n.id;}).map(function(e){return e.from===n.id?e.to:e.from;}).join(' ');});
      nodes.forEach(function(n){
        n.classList.toggle('map-scope-selected',n.id===scope);n.classList.toggle('map-context-node',!!area&&base.indexOf(n.id)<0);
        if(n.dataset.activation)return;
        var subtitle=n.querySelector('.map-node-context');
        if(!subtitle){subtitle=document.createElementNS('http://www.w3.org/2000/svg','text');subtitle.setAttribute('x',origins[n.id].x);subtitle.setAttribute('y',origins[n.id].y);subtitle.setAttribute('dx','11');subtitle.setAttribute('dy','56');subtitle.setAttribute('class','map-node-context');n.appendChild(subtitle);}
        subtitle.textContent=n.dataset.branch?rmT('Open parts · {0}',children(n).length):n.id===scope?rmT('Part'):repomapMembers.items(n).length?rmT('Open key code · {0}',repomapMembers.items(n).length):rmT('Explore connections');
      });
      repomapMembers.draw(map,expanded,expanded&&boxes[expanded.id]);
      var width=Math.max(300,layout.width),height=Math.max(160,layout.height);
      svg.setAttribute('viewBox','0 0 '+width+' '+height);svg.setAttribute('width',width);svg.setAttribute('height',height);svg.style.width=width+'px';svg.style.minWidth='0';svg.style.maxWidth='none';
      }
      visibleIDs=visible;map.inspectedOperation=operation;map.dataset.operationPinned=pinned?'true':'false';map.classList.remove('map-previewing');
      map.explorerScope=scope;map.explorerOperation=operation;
      structure.setAttribute('aria-pressed',mode==='structure');operationMode.setAttribute('aria-pressed',mode==='operations');operationPicker.hidden=mode!=='operations'||!!operation;
      allUses.hidden=!operation;allUses.textContent=rmT('Clear selection');
      allUses.title=operation?rmT('Clear selection: {0}',operation.dataset.title):'';
      choices.querySelectorAll('button').forEach(function(b){var selected=operation&&b.dataset.operationChoice===operation.id;b.setAttribute('aria-pressed',!!selected);});
      updateChoices();
      crumbs.replaceChildren();
      var repository=document.createElement('a');repository.href='#repository-map';repository.className='explorer-repository';repository.textContent=rmT('← Repository');crumbs.appendChild(repository);
      crumbs.appendChild(button(map.closest('[data-report-page]').dataset.componentName,function(){visit=null;open('');}));
      if(operation){
        var choose=document.createElement('a');choose.href='#'+map.closest('[data-report-page]').id+'-inbound';choose.textContent=rmT('Choose another operation');crumbs.appendChild(choose);
        var label=document.createElement('strong');label.textContent='→ '+operation.dataset.title+' · '+rmT(pinned?'selected':'preview');crumbs.appendChild(label);}
      trail.concat(scope?[scope]:[]).forEach(function(id){
        var item=button(byID[id].dataset.title,function(){open(id);});
        item.className='explorer-crumb';
        var kind=document.createElement('span');kind.className='explorer-crumb-kind';kind.textContent=byID[id].dataset.branch==='component'?rmT('Component'):byID[id].dataset.branch?rmT('Area'):rmT('Part');if(byID[id].dataset.remote==='true'&&!byID[id].dataset.branch)kind.textContent+=' · '+partOwner(byID[id]);item.prepend(kind);
        if(id===scope){item.setAttribute('aria-current','location');item.disabled=true;}
        crumbs.appendChild(item);
      });
      if(scope)crumbs.prepend(button(rmT('← Back'),function(){open(trail[trail.length-1]||'');}));
      if(area){var collapse=button(rmT('Collapse details'),function(){open(scopePath(area.id).slice(0,-1).pop()||'');});collapse.className='explorer-collapse';crumbs.appendChild(collapse);}
      var selfCount=edges.filter(function(edge){return edge.from===edge.to&&(operation||expanded&&edge.from===scope);}).length;
      var status=document.createElement('span');status.className='explorer-status';status.textContent=rmT('{0} parts · {1} connections',visible.filter(function(id){return !byID[id].dataset.activation;}).length,currentEdges.reduce(function(count,edge){return count+edge.relations.length;},selfCount));crumbs.appendChild(status);
      showReturnPath();
      {
        if(!previousViewport)stage.scrollTo(0,0);
        var focused=expanded&&boxes[expanded.id]?[boxes[expanded.id]]:base.map(function(id){return boxes[id];}).filter(Boolean);
        map.dispatchEvent(new CustomEvent('repomap:layout',{detail:{focus:focused}}));
        if(previousViewport)map.restoreViewport?.(previousViewport);
      }
      function read(){if(ticket!==revision||!map.showNode)return;if(scope)show(byID[scope]);else if(operation)show(operation);else map.clearInspection();map.dispatchEvent(new Event('repomap:reading'));}
      if(map.showNode)read();else requestAnimationFrame(read);
      map.setAttribute('aria-busy','false');
      return ticket;
    }
    structure.addEventListener('click',function(){mode='structure';operation=null;pinned=false;visit=null;address(byID[scope]);render();});
    operationMode.addEventListener('click',async function(){mode='operations';await render();revealChoice();});
    allUses.addEventListener('click',function(){operation=null;pinned=false;mode='structure';address(byID[scope]);render();});
    var searchTimer=0;search.addEventListener('input',function(){visit=null;clearTimeout(searchTimer);searchTimer=setTimeout(render,200);});
    ops.forEach(function(n){n.addEventListener('click',async function(e){e.preventDefault();e.stopImmediatePropagation();if(operation!==n)visit=null;operation=n;pinned=true;address(n);await render();revealChoice();show(n);orient();});});
    groups.forEach(function(n){n.addEventListener('click',function(e){e.preventDefault();e.stopImmediatePropagation();open(n.id);});});
    async function reveal(n,allUses,source){
      if(!n||nodes.indexOf(n)<0)return;
      if(followForeign(n,allUses,source))return;
      abandonHistoryRestore();visit=null;scopeVisits.clear();
      var destination=n;n=byID[displayed(n.id)];
      if(allUses){mode='structure';operation=null;pinned=false;}
      if(n.dataset.activation){mode='operations';operation=n;pinned=true;scope='';trail=[];}
      else setScope(n.id);
      search.value='';
      // Record the destination state before layout can yield, and reveal its
      // page before the map measures the space available to it.
      document.dispatchEvent(new CustomEvent('repomap:navigate',{detail:{destination:destination}}));
      var rendered=await render();
      if(rendered!==revision)return;
      if(n.dataset.activation)revealChoice();
      rmScrollToReading(map);show(n);
      if(source)map.explainSource(source);
    }
    map.exploreNode=function(id){open(id);};
    map.explorationLabel=function(){
      var labels=[mode==='operations'?rmT('Operations'):rmT('Structure')];
      if(operation)labels.push(operation.dataset.title+' · '+rmT(pinned?'selected':'preview'));
      trail.concat(scope?[scope]:[]).forEach(function(id){var n=byID[id];labels.push((n.dataset.branch==='component'?rmT('Component'):n.dataset.branch?rmT('Area'):rmT('Part'))+(n.dataset.remote==='true'&&!n.dataset.branch?' · '+partOwner(n):'')+' '+n.dataset.title);});
      if(map.explorerMember&&map.explorerMember.owner===scope)labels.push(rmT('Code element')+' '+map.explorerMember.name);
      return labels.join(' › ');
    };
    map.resumeExploration=function(){if(scope)show(byID[scope]);else if(operation)show(operation);};
    // The reading navigator owns the single browser-history entry. Supply
    // only serializable UI state, retaining the exact declaration source.
    map.readingState=function(){
      var member=map.explorerMember;
      return {scope:scope,operation:operation?.id||'',pinned:pinned,mode:mode,search:search.value,
        source:member&&member.owner===scope?{href:member.href||'',open:member.open||'',key:member.key}:null,
        disclosures:readingDisclosures(),viewport:map.captureViewport?.()||null};
    };
    map.restoreReadingState=function(saved){
      if(!saved||(saved.scope&&!byID[saved.scope])||(saved.operation&&!byID[saved.operation]?.dataset.activation))return Promise.resolve();
      var key=JSON.stringify([location.href,saved]);
      if(key===historyRestoreKey&&(map.readingRestoring||JSON.stringify(map.readingState())===JSON.stringify(saved)))return historyRestorePromise||Promise.resolve();
      var ticket=++historyRevision;historyRestoreKey=key;historyRestoreHash=location.hash;map.readingRestoring=true;
      setScope(saved.scope||'');operation=byID[saved.operation]||null;pinned=!!saved.pinned;mode=saved.mode==='operations'?'operations':'structure';search.value=saved.search||'';
      scopeVisits.clear();visit=null;map.explorerScope=scope;map.explorerOperation=operation;
      historyRestorePromise=(async function(){
        try{
          await render();if(ticket!==historyRevision)return;restoreDisclosures(saved.disclosures);
          if(saved.source)map.explainSource(saved.source);
          // A previously selected term must also be clearable when the
          // historical visit had no declaration selected.
          else map.inspectConcept?.(-1);
          await new Promise(requestAnimationFrame);if(ticket!==historyRevision)return;
          if(saved.viewport)map.restoreViewport?.(saved.viewport);
        }finally{
          if(ticket===historyRevision){map.readingRestoring=false;map.dispatchEvent(new Event('repomap:reading'));}
        }
      })();return historyRestorePromise;
    };
    map.displayedNode=function(node){return byID[displayed(node.id)];};
    map.areaDescriptions=function(node){return areaDescriptions[node.id]||[];};
    map.revealNode=reveal;
    map.findNode=function(n,source){return reveal(n,true,source);};
    map.operationChoices=function(id){var members=new Set(leaves(id));return ops.filter(function(op){return nearOf[op.id].some(function(near){return members.has(near);});});};
    map.chooseOperation=async function(id,origin){var op=byID[id];if(!op)return;visit=origin?{origin:origin,operation:op}:null;operation=op;pinned=true;mode='operations';setScope('');search.value='';address(op);await render();revealChoice();orient();};
    function resetScope(){abandonHistoryRestore();setScope('');operation=null;pinned=false;mode='structure';visit=null;scopeVisits.clear();search.value='';}
    function hashChanged(){
      if(historyRestoreHash===location.hash){historyRestoreHash='';return;}
      historyRestoreHash='';
      var node=document.getElementById(location.hash.slice(1));
      if(node===map.closest('[data-report-page]')){resetScope();render();return;}
      reveal(node,true);
    }
    window.addEventListener('hashchange',hashChanged);
    document.addEventListener('click',function(e){
      var a=e.target.closest('a[href^="#"]');if(!a||a.closest('[data-map-explorer]')||a.hasAttribute('data-reading-map-return')||e.ctrlKey||e.metaKey||e.shiftKey||e.altKey)return;
      var destination=document.getElementById(a.getAttribute('href').slice(1));
      // A component card/picker promises that component's entrance. A named
      // return to an existing reading uses its exact scope through 45-modes.
      if(destination===map.closest('[data-report-page]')||destination===map.closest('.component-parts')){resetScope();address(destination);render().then(function(){destination.scrollIntoView({block:'start'});});return;}
      var source=a.closest('.learn-concept,.answer-term')?.querySelector('.model-sources a');
      reveal(destination,!!a.closest('#learn-parts,.concept-library,.answer-term'),source&&{href:source.getAttribute('href'),open:source.dataset.open});
    },true);
    render();hashChanged();
  });
})();
