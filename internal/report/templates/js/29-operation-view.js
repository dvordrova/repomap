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
    var focusHistory=[], focusOrigin=null;
    var historyRestoreHash='', historyRestoreKey='', historyRestorePromise=null, historyRevision=0;
    function abandonHistoryRestore(){historyRestoreHash='';historyRestoreKey='';historyRevision++;map.readingRestoring=false;}
    var focusView=document.createElement('section');focusView.className='map-focus';focusView.hidden=true;stage.after(focusView);
    var hoveredPeer='', focusedPeer='';
    function peerAt(target){return target?.closest?.('.map-focus-neighbor')?.querySelector('[data-focus-neighbor]')?.dataset.focusNeighbor||'';}
    function emphasizePeer(){
      var selected=hoveredPeer||focusedPeer;
      focusView.querySelectorAll('.map-focus-neighbor').forEach(function(row){row.classList.toggle('map-focus-peer-emphasis',!!selected&&peerAt(row)===selected);});
    }
    focusView.addEventListener('pointerover',function(e){hoveredPeer=peerAt(e.target);emphasizePeer();});
    focusView.addEventListener('pointerout',function(e){hoveredPeer=focusView.contains(e.relatedTarget)?peerAt(e.relatedTarget):'';emphasizePeer();});
    focusView.addEventListener('focusin',function(e){focusedPeer=peerAt(e.target);emphasizePeer();});
    focusView.addEventListener('focusout',function(e){focusedPeer=focusView.contains(e.relatedTarget)?peerAt(e.relatedTarget):'';emphasizePeer();});
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
    function orient(){
      var inset=parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--toolbar-height'))||0;
      if(bar.getBoundingClientRect().top<inset)map.scrollIntoView({block:'start'});
    }
    function address(node){document.dispatchEvent(new CustomEvent('repomap:navigate',{detail:{destination:node||map.closest('[data-report-page]')}}));}
    function focusDisclosures(){
      if(!scope||focusView.dataset.scope!==scope)return null;
      return {incoming:!!focusView.querySelector('.map-focus-incoming .map-focus-remote')?.open,outgoing:!!focusView.querySelector('.map-focus-outgoing .map-focus-remote')?.open};
    }
    function restoreFocusDisclosures(saved){
      ['incoming','outgoing'].forEach(function(side){var disclosure=focusView.querySelector('.map-focus-'+side+' .map-focus-remote');if(disclosure)disclosure.open=!!saved?.[side];});
    }
    function snapshot(){
      var member=map.explorerMember;
      return {scope:scope,operation:operation,pinned:pinned,mode:mode,search:search.value,
        source:member&&member.owner===scope?{href:member.href,open:member.open,key:member.key}:null,
        label:member&&member.owner===scope?rmT('{0} in {1}',member.name,byID[scope].dataset.title):scope?byID[scope].dataset.title:map.closest('[data-report-page]').dataset.componentName,
        disclosures:focusDisclosures(),viewport:map.captureViewport?.(),scroll:window.scrollY};
    }
    async function restore(saved){
      setScope(saved.scope);operation=saved.operation;pinned=saved.pinned;mode=saved.mode;search.value=saved.search;
      address(byID[scope]);await render();restoreFocusDisclosures(saved.disclosures);if(saved.source)map.explainSource(saved.source);
      requestAnimationFrame(function(){if(saved.viewport)map.restoreViewport?.(saved.viewport);window.scrollTo({top:saved.scroll,behavior:'instant'});});
    }
    function previousFocus(){var previous=focusHistory.pop();if(previous)restore(previous);}
    function collapseFocus(){
      var previous=focusOrigin;focusHistory=[];focusOrigin=null;
      if(previous)restore(previous);else open(trail[trail.length-1]||'');
    }
    async function open(id,neighbor){
      id=displayed(id);if((id&&!byID[id])||scope===id)return;
      if(followForeign(byID[id]))return;
      abandonHistoryRestore();
      var wasFocus=map.classList.contains('map-part-focus'), next=byID[id], willFocus=next&&!next.dataset.branch&&!next.dataset.activation;
      if(neighbor){focusHistory.push(snapshot());}
      else {focusHistory=[];if(willFocus&&!wasFocus)focusOrigin=snapshot();else if(!willFocus)focusOrigin=null;}
      if(!operation)visit=null;
      if(next?.dataset.activation){operation=next;pinned=true;mode='operations';setScope('');}else setScope(id);
      search.value='';address(next);await render();if(!neighbor)orient();
    }
    function updateFocusSelection(){
      var selected=focusView.querySelector('.map-focus-selection');if(!selected)return;
      selected.replaceChildren();var member=map.explorerMember;
      selected.hidden=!member||member.owner!==scope;if(selected.hidden)return;
      var item=repomapMembers.items(byID[scope]).find(function(item){return repomapMembers.sourceKey(item.source)===(member.key||member.href||member.open);});
      if(!item)return;
      var name=document.createElement('strong');name.textContent=item.alias||item.name;selected.appendChild(name);
      if(item.alias&&item.alias!==item.name){var nativeName=document.createElement('code');nativeName.textContent=item.name;selected.appendChild(nativeName);}
      selected.appendChild(repomapMembers.sourceLink(item.source));
    }
    function placeInspection(inFocus){
      var inspector=map.querySelector('.map-inspector'),destination=inFocus?focusView.querySelector('.map-focus-center'):map.querySelector('.map-workspace');
      if(inspector&&destination&&inspector.parentElement!==destination)destination.appendChild(inspector);
    }
    map.addEventListener('repomap:reading',updateFocusSelection);
    function focusRelations(container,relations){
      var list=document.createElement('ul');list.className='map-focus-relations';
      relations.forEach(function(relation){
        var row=document.createElement('li');
        var label=document.createElement('p');label.className='map-focus-relation-label';label.textContent=relation.label;label.dataset.displayRef=relation.labelRef||'';row.appendChild(label);
        if(relation.summary&&relation.summary!==relation.label){var summary=document.createElement('p');summary.textContent=relation.summary;summary.dataset.displayRef=relation.summaryRef||'';row.appendChild(summary);}
        if(relation.possible){var possible=document.createElement('span');possible.className='map-focus-relation-kind';possible.textContent=rmT('Interpreted connection or possible dispatch');row.appendChild(possible);}
        var sources=document.createElement('div');sources.className='map-focus-sources';
        [['fromSource','fromText','fromNoSource'],['toSource','toText','toNoSource']].forEach(function(fields){
          if(!relation[fields[0]]&&!relation[fields[2]])return;
          sources.appendChild(repomapMembers.sourceLink({Href:relation[fields[0]],Text:relation[fields[1]]||rmT('Source'),NoSource:relation[fields[2]]}));
        });
        if(sources.childElementCount)row.appendChild(sources);list.appendChild(row);
      });container.appendChild(list);
    }
    function drawFocus(node,edges){
      var disclosures=focusDisclosures();
      hoveredPeer='';focusedPeer='';
      placeInspection(false);
      focusView.replaceChildren();focusView.dataset.scope=node.id;focusView.setAttribute('aria-label',rmT('Connections of {0}',node.dataset.title));
      var heading=document.createElement('div');heading.className='map-focus-heading';
      var context=document.createElement('span');context.className='map-focus-context';context.textContent=map.closest('[data-report-page]').dataset.componentName+' · '+rmT('Part connections');heading.appendChild(context);
      var navigation=document.createElement('nav');navigation.setAttribute('aria-label',rmT('Map path'));
      if(focusHistory.length)navigation.appendChild(button(rmT('← Back to {0}',focusHistory[focusHistory.length-1].label),previousFocus));
      var originScope=focusOrigin?focusOrigin.scope:trail[trail.length-1];
      var origin=originScope?byID[originScope].dataset.title:map.closest('[data-report-page]').dataset.componentName;
      navigation.appendChild(button(rmT('All parts in {0}',origin),collapseFocus));heading.appendChild(navigation);focusView.appendChild(heading);
      var scene=document.createElement('div');scene.className='map-focus-scene';focusView.appendChild(scene);
      function side(incoming){
        var column=document.createElement('section');column.className='map-focus-side '+(incoming?'map-focus-incoming':'map-focus-outgoing');
        var connections=currentEdges.filter(function(edge){return incoming?edge.to===scope:edge.from===scope;});
        if(!connections.length)return null;
        var total=connections.reduce(function(n,edge){return n+edge.relations.length;},0);
        var h=document.createElement('h3');h.textContent=rmT(incoming?'Incoming connections · {0}':'Outgoing connections · {0}',total);column.appendChild(h);
        var peers=new Map();connections.forEach(function(edge){var id=incoming?edge.from:edge.to;(peers.get(id)||peers.set(id,[]).get(id)).push.apply(peers.get(id),edge.relations);});
        var remote=new Map();
        function neighbor(relations,id){
          var peer=byID[id], item=document.createElement('article');item.className='map-focus-neighbor';
          var kind=document.createElement('span');kind.className='map-focus-peer-kind';kind.textContent=peer.dataset.branch==='component'?rmT('Component'):peer.dataset.branch?rmT('Area'):peer.dataset.activation?rmT('Operation'):rmT('Part');item.appendChild(kind);
          var link=button(peer.dataset.title+' →',function(){
            open(id,true);
          });link.className='map-focus-neighbor-link';link.dataset.focusNeighbor=id;item.appendChild(link);
          if(currentEdges.some(function(edge){return incoming?edge.from===scope&&edge.to===id:edge.to===scope&&edge.from===id;})){
            var reciprocal=document.createElement('span');reciprocal.className='map-focus-reciprocal';reciprocal.textContent=rmT('Also connected in the opposite direction');item.appendChild(reciprocal);
          }
          focusRelations(item,relations);return item;
        }
        peers.forEach(function(relations,id){
          var peer=byID[id];
          if(peer.dataset.remote!=='true'){column.appendChild(neighbor(relations,id));return;}
          var owner=peer.dataset.component||id;
          if(!remote.has(owner))remote.set(owner,[]);remote.get(owner).push({id:id,relations:relations});
        });
        if(remote.size){
          var outside=document.createElement('details');outside.className='map-focus-remote';outside.open=!!disclosures?.[incoming?'incoming':'outgoing'];
          outside.addEventListener('toggle',function(){if(focusView.contains(outside))map.dispatchEvent(new Event('repomap:reading'));});
          var remoteCount=Array.from(remote.values()).reduce(function(count,rows){return count+rows.reduce(function(n,row){return n+row.relations.length;},0);},0);
          var summary=document.createElement('summary');summary.textContent=rmT('Other components · {0} · {1} connections',remote.size,remoteCount);outside.appendChild(summary);
          remote.forEach(function(rows){
            var group=document.createElement('section');group.className='map-focus-remote-owner';
            var peer=byID[rows[0].id],href=peer.getAttribute('href')||'',destination=href[0]==='#'&&document.getElementById(href.slice(1));
            var owner=destination&&destination.closest('[data-report-page]'),native=owner?.querySelector('.target-facts code')?.textContent;
            var ownerTitle=document.createElement('h4');ownerTitle.textContent=partOwner(peer)+(native?' · '+native:'');group.appendChild(ownerTitle);
            rows.forEach(function(row){group.appendChild(neighbor(row.relations,row.id));});outside.appendChild(group);
          });column.appendChild(outside);
        }
        return column;
      }
      var incoming=side(true),outgoing=side(false);
      scene.classList.toggle('has-incoming',!!incoming);scene.classList.toggle('has-outgoing',!!outgoing);
      if(incoming)scene.appendChild(incoming);
      var center=document.createElement('section');center.className='map-focus-center';
      var kind=document.createElement('span');kind.className='map-focus-kind';kind.textContent=rmT('Part')+(node.dataset.remote==='true'?' · '+partOwner(node):'');center.appendChild(kind);
      var title=document.createElement('h3');title.textContent=node.dataset.title;center.appendChild(title);
      if(node.dataset.summary){var purpose=document.createElement('p');purpose.className='map-focus-purpose';purpose.textContent=node.dataset.summary;purpose.dataset.displayRef=node.dataset.summaryRef||'';center.appendChild(purpose);}
      if(!incoming||!outgoing){
        var empty=document.createElement('p');empty.className='map-focus-empty';
        empty.textContent=!incoming&&!outgoing?rmT('No connections recorded.'):rmT(!incoming?'Incoming connections · {0}':'Outgoing connections · {0}',0);
        if(incoming||outgoing)empty.title=rmT(!incoming?'No incoming connections recorded.':'No outgoing connections recorded.');
        center.appendChild(empty);
      }
      var selection=document.createElement('div');selection.className='map-focus-selection';selection.hidden=true;center.appendChild(selection);
      var members=repomapMembers.items(node);
      if(members.length){rmLocalReadingActions(center,center,function(){center.querySelector('.map-member')?.scrollIntoView({block:'nearest'});center.querySelector('.map-member a')?.focus({preventScroll:true});});var label=document.createElement('h4');label.textContent=rmT('Key code')+' · '+members.length;center.appendChild(label);center.appendChild(repomapMembers.grid(map,node));}
      // Only an original self relation belongs here. Folding a group's own
      // contents together must never manufacture an arrow on a code element.
      var self=edges.filter(function(edge){return edge.from===scope&&edge.to===scope;});
      if(self.length){var own=document.createElement('section');own.className='map-focus-self';var ownTitle=document.createElement('h4');ownTitle.textContent=rmT('Connections within this part · {0}',self.length);own.appendChild(ownTitle);focusRelations(own,self);center.appendChild(own);}
      scene.appendChild(center);if(outgoing)scene.appendChild(outgoing);updateFocusSelection();
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
      var previousViewport=operation&&map.explorerOperation===operation?map.captureViewport?.():null;
      var limit=allowed(), scopeLeaves=scope?new Set(leaves(scope)):null;
      var edges=rawEdges.filter(function(e){return operation?e.scope==='operation'&&e.operations.indexOf(operation.id)>=0:e.scope==='structure';});
      var base=scope?(children(byID[scope]).length?children(byID[scope]):[scope]):roots.filter(function(id){return byID[id].dataset.remote!=='true';});
      base=base.filter(function(id){return applicable(id,limit);});
      var term=search.value.trim().toLowerCase();
      if(term)base=groups.filter(function(n){return applicable(n.id,limit)&&(!scopeLeaves||leaves(n.id).some(function(id){return scopeLeaves.has(id);}))&&(n.dataset.title+' '+n.dataset.summary+' '+repomapMembers.items(n).map(repomapMembers.displayName).join(' ')).toLowerCase().indexOf(term)>=0;}).map(function(n){return n.id;});
      base=displayedSet(base);
      var inside=scopeLeaves||(term?new Set(base.flatMap(function(id){return leaves(id);})):null);
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
      // Once a part is open, its immediate neighbours are exact edge
      // endpoints. Folding them back into areas would add an unrelated
      // drill-in between a connection label and the part that owns it.
      var leafFocus=scope&&!term&&!byID[scope].dataset.branch&&!byID[scope].dataset.activation;
      var boundary=leafFocus?Array.from(endpoints).filter(function(id){return byID[id]&&!represented.has(id);}):roots.flatMap(boundaryAt);
      var visible=Array.from(new Set(base.concat(boundary))).filter(function(id){return applicable(id,limit);});
      if(operation&&!term)visible=[operation.id].concat(visible.filter(function(id){return id!==operation.id;}));
      var representatives={};
      visible.forEach(function(id){leaves(id).forEach(function(leaf){
        if(represented.has(leaf)&&base.indexOf(id)<0)return;
        (representatives[leaf]||(representatives[leaf]=[])).push(id);
      });});
      currentEdges=repomapGraph.fold(relevant,representatives,inside);
      // Standalone scope: order incoming peers, the subject, outgoing peers.
      // Areas use a compact grid, retaining their own original child names.
      visible.sort(function(a,b){if(!operation){if(a===scope)return -1;if(b===scope)return 1;}if(a===operation?.id)return -1;if(b===operation?.id)return 1;var ai=base.indexOf(a)>=0,bi=base.indexOf(b)>=0;if(ai!==bi)return ai?-1:1;return byID[a].dataset.title.localeCompare(byID[b].dataset.title);});
      var boxes={};visible.forEach(function(id){boxes[id]={w:220,h:80};});
      var layout;
      // The shared inspector mounts after this script. Its temporary stage
      // width must not turn a wide map into a vertical layout on first open.
      var mapStyle=getComputedStyle(map),availableWidth=map.clientWidth-parseFloat(mapStyle.paddingLeft)-parseFloat(mapStyle.paddingRight);
      var opened=scope&&byID[scope], expanded=!term&&opened&&!opened.dataset.branch&&!opened.dataset.activation?opened:null;
      map.classList.toggle('map-part-focus',!!expanded);map.classList.toggle('map-operation-context',!!operation);focusView.hidden=!expanded;
      if(!expanded)placeInspection(false);
      if(expanded){
        repomapPreview.freeze(map);drawFocus(expanded,edges);
      }
      if(!expanded||operation){
      visible.forEach(function(id){
        var node=byID[id],display=node.style.display,visibility=node.style.visibility;
        node.style.display='';node.style.visibility='hidden';
        node.querySelectorAll('.map-node-title').forEach(function(title){boxes[id].w=Math.max(boxes[id].w,Math.ceil(title.getComputedTextLength())+22);});
        node.style.display=display;node.style.visibility=visibility;
      });
      try{layout=await repomapGraph.layout(boxes,currentEdges,null,availableWidth);}catch(error){
        if(ticket!==revision)return;map.setAttribute('aria-busy','false');
        var message=bar.querySelector('[role="alert"]');if(!message){message=document.createElement('p');message.setAttribute('role','alert');bar.appendChild(message);}message.textContent=rmT('Could not arrange this map. Try another scope.');console.error('Component map layout',error);return;
      }
      if(ticket!==revision)return;
      repomapPreview.freeze(map);boxes=layout.boxes;
      var routed=repomapGraph.draw(map,layout.edges);
      nodes.forEach(function(n){var b=boxes[n.id];n.style.display=b?'':'none';if(!b)return;var origin=origins[n.id];n.setAttribute('transform','translate('+(b.x-origin.x)+' '+(b.y-origin.y)+')');var rect=n.querySelector('rect');rect.setAttribute('width',b.w);rect.setAttribute('height',b.h);n.classList.toggle('map-area',!!n.dataset.branch);n.classList.toggle('operation-selected',n===operation);if(!n.dataset.activation)n.dataset.near=currentEdges.filter(function(e){return e.from===n.id||e.to===n.id;}).map(function(e){return e.from===n.id?e.to:e.from;}).join(' ');});
      nodes.forEach(function(n){
        n.classList.toggle('map-scope-selected',n.id===scope);
        if(n.dataset.activation)return;
        var subtitle=n.querySelector('.map-node-context');
        if(!subtitle){subtitle=document.createElementNS('http://www.w3.org/2000/svg','text');subtitle.setAttribute('x',origins[n.id].x);subtitle.setAttribute('y',origins[n.id].y);subtitle.setAttribute('dx','11');subtitle.setAttribute('dy','56');subtitle.setAttribute('class','map-node-context');n.appendChild(subtitle);}
        subtitle.textContent=n.dataset.branch?rmT('Open parts · {0}',children(n).length):n.id===scope?rmT('Part'):repomapMembers.items(n).length?rmT('Open key code · {0}',repomapMembers.items(n).length):rmT('Explore connections');
      });
      repomapMembers.draw(map,null,null);
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
      if(!expanded&&focusHistory.length)crumbs.prepend(button(rmT('← Back to {0}',focusHistory[focusHistory.length-1].label),previousFocus));
      else if(scope&&!expanded)crumbs.prepend(button(rmT('← Back'),function(){open(trail[trail.length-1]||'');}));
      var selfCount=edges.filter(function(edge){return edge.from===edge.to&&(operation||expanded&&edge.from===scope);}).length;
      var status=document.createElement('span');status.className='explorer-status';status.textContent=rmT('{0} parts · {1} connections',visible.filter(function(id){return !byID[id].dataset.activation;}).length,currentEdges.reduce(function(count,edge){return count+edge.relations.length;},selfCount));crumbs.appendChild(status);
      showReturnPath();
      if(!expanded||operation){
        if(!previousViewport)stage.scrollTo(0,0);
        var focused=base.map(function(id){return boxes[id];}).filter(Boolean);
        map.dispatchEvent(new CustomEvent('repomap:layout',{detail:{focus:focused}}));
        if(previousViewport)map.restoreViewport?.(previousViewport);
      }
      function read(){if(ticket!==revision||!map.showNode)return;placeInspection(!!expanded);if(scope)show(byID[scope]);else if(operation)show(operation);else map.clearInspection();map.dispatchEvent(new Event('repomap:reading'));}
      if(map.showNode)read();else requestAnimationFrame(read);
      map.setAttribute('aria-busy','false');
      return ticket;
    }
    structure.addEventListener('click',function(){mode='structure';operation=null;pinned=false;visit=null;address(byID[scope]);render();});
    operationMode.addEventListener('click',async function(){mode='operations';await render();revealChoice();});
    allUses.addEventListener('click',function(){operation=null;pinned=false;mode='structure';address(byID[scope]);render();});
    search.addEventListener('input',function(){visit=null;render();});
    ops.forEach(function(n){n.addEventListener('click',async function(e){e.preventDefault();e.stopImmediatePropagation();if(operation!==n)visit=null;operation=n;pinned=true;address(n);await render();revealChoice();show(n);orient();});});
    groups.forEach(function(n){n.addEventListener('click',function(e){e.preventDefault();e.stopImmediatePropagation();open(n.id);});});
    async function reveal(n,allUses,source){
      if(!n||nodes.indexOf(n)<0)return;
      if(followForeign(n,allUses,source))return;
      abandonHistoryRestore();visit=null;focusHistory=[];
      if(!map.classList.contains('map-part-focus'))focusOrigin=snapshot();
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
        disclosures:focusDisclosures(),viewport:map.captureViewport?.()||null};
    };
    map.restoreReadingState=function(saved){
      if(!saved||(saved.scope&&!byID[saved.scope])||(saved.operation&&!byID[saved.operation]?.dataset.activation))return Promise.resolve();
      var key=JSON.stringify([location.href,saved]);
      if(key===historyRestoreKey&&(map.readingRestoring||JSON.stringify(map.readingState())===JSON.stringify(saved)))return historyRestorePromise||Promise.resolve();
      var ticket=++historyRevision;historyRestoreKey=key;historyRestoreHash=location.hash;map.readingRestoring=true;
      setScope(saved.scope||'');operation=byID[saved.operation]||null;pinned=!!saved.pinned;mode=saved.mode==='operations'?'operations':'structure';search.value=saved.search||'';
      focusHistory=[];focusOrigin=null;visit=null;map.explorerScope=scope;map.explorerOperation=operation;
      historyRestorePromise=(async function(){
        try{
          await render();if(ticket!==historyRevision)return;restoreFocusDisclosures(saved.disclosures);
          if(saved.source)map.explainSource(saved.source);
          // A previously selected term must also be clearable when the
          // historical visit had no declaration selected.
          else {var picker=map.querySelector('[data-concept-picker]');if(picker&&picker.value){picker.value='';picker.dispatchEvent(new Event('change'));}}
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
    function resetScope(){abandonHistoryRestore();setScope('');operation=null;pinned=false;mode='structure';visit=null;focusHistory=[];focusOrigin=null;search.value='';}
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
