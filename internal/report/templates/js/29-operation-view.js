// One repository drawing. Selection changes emphasis and reading, never layout.
// All IDs, containment and operation paths come from the rendered report.
function rmSystemProjection(nodes, edges) {
  var byID={},parents={},inputOwner={};
  nodes.forEach(function(n){byID[n.id]=n;});
  nodes.forEach(function(n){if(n.inputOwner&&byID[n.inputOwner])inputOwner[n.id]=n.inputOwner;});
  nodes.forEach(function(n){(n.children||[]).forEach(function(id){parents[id]=n.id;});});
  edges.forEach(function(e){if(byID[e.from]?.activation && e.label==='implemented in' && byID[e.to] && !byID[e.to].activation)inputOwner[e.from]=e.to;});
  function leaves(id,seen){seen=seen||new Set();if(seen.has(id)||!byID[id])return [];seen.add(id);var n=byID[id];return n.children?.length?n.children.flatMap(function(c){return leaves(c,new Set(seen));}):[id];}
  var visible=nodes.filter(function(n){return !n.children?.length;}).map(function(n){return n.id;});
  var representatives={};nodes.forEach(function(n){representatives[n.id]=[n.id];});
  // Only the input catalogue contains input cards. Implementation and original
  // ownership remain available independently for breadcrumbs and reading.
  var areas=nodes.filter(function(n){return n.children?.length;}).map(function(n){return {id:n.id,nodes:n.children.filter(function(id){return n.branch==='inputs'||!byID[id]?.activation;})};});
  function selection(id,operation){
    var selected=new Set(leaves(id));if(byID[id])selected.add(id);var active=new Set(operation?[]:selected),path=[];
    if(operation){
      path=edges.filter(function(e){return (e.operations||[]).includes(operation);});
      path.forEach(function(e){active.add(e.from);active.add(e.to);});
      active.add(operation);
    }else if(id){edges.forEach(function(e){if(selected.has(e.from)||selected.has(e.to)){active.add(e.from);active.add(e.to);}});}
    return {selected:selected,active:active,path:path,entry:operation||''};
  }
  return {visible:visible,areas:areas,representatives:representatives,parents:parents,inputOwner:inputOwner,leaves:leaves,selection:selection};
}
(function(){document.querySelectorAll('[data-map-explorer]').forEach(function(map){
  var svg=map.querySelector('svg'),stage=map.querySelector('[data-map-stage]');
  var nodes=Array.from(map.querySelectorAll('[data-node]')),byID={},aliases={};
  nodes.forEach(function(n){byID[n.id]=n;});
  map.querySelectorAll('[data-map-alias]').forEach(function(n){aliases[n.id]=n.dataset.mapAlias;});
  var rawEdges=Array.from(svg.querySelectorAll('.map-edge')).filter(function(e){return e.dataset.scope!=='static';}).map(function(e){return {from:e.dataset.from,to:e.dataset.to,scope:e.dataset.scope,summary:e.dataset.summary,summaryRef:e.dataset.summaryRef,labelRef:e.dataset.labelRef,fromSource:e.dataset.fromSource,fromText:e.dataset.fromText,fromNoSource:e.dataset.fromNoSource==='true',toSource:e.dataset.toSource,toText:e.dataset.toText,toNoSource:e.dataset.toNoSource==='true',operations:(e.dataset.operations||'').split(/\s+/).filter(Boolean),possible:e.classList.contains('map-edge-possible'),label:e.querySelector('title')?.textContent||''};});
  var model=nodes.map(function(n){return {id:n.id,branch:n.dataset.branch,children:(n.dataset.children||'').split(/\s+/).filter(Boolean),activation:n.dataset.activation,inputOwner:n.dataset.inputOwner};});
  var projection=rmSystemProjection(model,rawEdges),scope='',operation=null,surface=null,ready=null;
  var searchValue='',filterValue='',selectionRevision=0,visual=null;
  var numbered=true;
  map.querySelector('[data-operation-controls]')?.remove();
  var bar=rmEl('div','system-controls'),search=rmEl('input','system-search'),filter=rmEl('select','system-filter'),clear=rmEl('button','',rmT('Clear selection'));
  search.type='search';search.placeholder=rmT('Find');search.setAttribute('aria-label',rmT('Find'));clear.type='button';
  [['','Everything'],['component','Components'],['part','Parts'],['input','Inputs'],['external','External communication']].forEach(function(item){var option=rmEl('option','',rmT(item[1]));option.value=item[0];filter.appendChild(option);});filter.setAttribute('aria-label',rmT('Show on map'));
  bar.append(search);map.prepend(bar);bar.appendChild(map.querySelector('[data-map-controls]'));
  if(map.hasAttribute('data-system-map'))search.hidden=true;
  var styleChoice=rmEl('button','',rmT('Connection labels'));styleChoice.type='button';styleChoice.setAttribute('aria-pressed','true');
  function syncStyle(){styleChoice.textContent=rmT(numbered?'Connection labels':'Arrows');styleChoice.setAttribute('aria-pressed',String(numbered));map.classList.toggle('system-numbered',numbered);}
  styleChoice.addEventListener('click',function(){numbered=!numbered;syncStyle();emphasize();emit();});syncStyle();
  var results=rmEl('div','system-results');results.hidden=true;bar.after(results);
  var colorKey=rmEl('span','flow-color-key');
  [['','Parts'],['entry','Entrypoints'],['core','Core'],['input','Inputs'],['external','External communication']].filter(function(item){return nodes.some(function(n){return ['core','entry'].includes(item[0])?!n.dataset.branch&&!n.dataset.activation&&n.dataset.lane===(item[0]==='entry'?'triggers':'core'):item[0]?category(n)===item[0]:!n.dataset.branch&&category(n)==='part'&&!['core','triggers'].includes(n.dataset.lane);});}).forEach(function(item){var label=rmEl('span',item[0]);label.append(rmEl('i'),document.createTextNode(rmT(item[1])));colorKey.appendChild(label);});
  var caption=rmEl('div','system-selection');caption.setAttribute('aria-live','polite');results.after(caption);
  var measureControls=new ResizeObserver(function(){map.style.setProperty('--system-controls-height',(bar.offsetHeight+results.offsetHeight+caption.offsetHeight+24)+'px');});
  [bar,results,caption].forEach(function(n){measureControls.observe(n);});
  function kind(n){return n.dataset.itemKind||(n.dataset.activation?'Inputs':n.dataset.branch==='component'?'Component':n.dataset.branch?'Area':n.dataset.lane==='core'?'Core':n.dataset.lane==='dependencies'?'Code dependencies':n.dataset.lane==='triggers'?'Entrypoints':'Part');}
  map.itemKind=kind;
  function category(n){return n.dataset.activation||n.dataset.branch==='inputs'?'input':n.dataset.itemKind==='External communication'?'external':n.dataset.branch==='component'||n.dataset.itemKind==='Component'?'component':'part';}
  function owner(n){return document.getElementById(n.dataset.owner)?.dataset.componentName||'';}
  function emit(){map.dispatchEvent(new Event('repomap:reading'));}
  function address(n,newVisit){document.dispatchEvent(new CustomEvent('repomap:navigate',{detail:{destination:n||document.getElementById('overview'),newVisit:!!newVisit}}));}
  function path(id){var result=[],seen=new Set();while(id&&!seen.has(id)){seen.add(id);result.unshift(id);id=projection.parents[id];}return result;}
  function matches(n){return (!filterValue||category(n)===filterValue)&&(!searchValue||(path(n.id).map(function(id){return byID[id].dataset.title;}).join(' ')+' '+n.dataset.summary+' '+owner(n)).toLowerCase().includes(searchValue.toLowerCase()));}
  function updateResults(){
    results.replaceChildren();results.hidden=!searchValue&&!filterValue;
    if(results.hidden)return;
    nodes.filter(matches).forEach(function(n){var b=rmEl('button','',n.dataset.title);b.type='button';b.dataset.selectNode=n.id;b.appendChild(rmEl('small','',owner(n)+' · '+rmT(kind(n))));b.addEventListener('click',function(){select(n,true,null,true).then(function(selected){if(selected)rmScrollToReading(map);});});results.appendChild(b);});
    if(!results.childElementCount)results.appendChild(rmEl('p','',rmT('No matches.')));
  }
  function emphasize(){
    var state=projection.selection(scope,operation?.id),has=!!scope||!!operation||!!searchValue||!!filterValue;
    var matched=new Set();if(searchValue||filterValue)nodes.filter(matches).forEach(function(n){projection.leaves(n.id).forEach(function(id){matched.add(id);});});
    surface?.update({scope:scope,operation:operation?.id||'',entry:state.entry,selected:state.selected,matched:matched,searching:!!searchValue||!!filterValue,numbered:numbered});
    map.classList.toggle('system-has-selection',has);clear.disabled=!scope&&!operation;
    renderCaption();
    map.explorerScope=scope;map.explorerOperation=operation;map.inspectedOperation=operation;map.dataset.operationPinned=operation?'true':'false';
  }
  function renderCaption(){
    caption.replaceChildren();
    var inspector=map.querySelector('.map-inspector'),controls=map.querySelector('.map-input-context');
    if(inspector&&!controls){controls=rmEl('div','map-input-context');inspector.prepend(controls);}
    if(controls){controls.replaceChildren();controls.hidden=!operation;}
    if(operation){
      var context=rmEl('span','system-reading-context',rmT('Input')+': '+operation.dataset.title);
      var start=rmEl('button','system-input-start',rmT('Show input'));start.type='button';start.addEventListener('click',function(){surface?.clearHover();select(operation,true,null,true);});context.appendChild(start);
      clear.textContent=rmT('Leave input path');controls?.append(context,clear);
    }
    map.querySelector('.map-reading-outside')?.remove();
    if(visual?.readingOutside)map.querySelector('.map-inspector-heading')?.appendChild(rmEl('small','map-reading-outside',rmT('Outside this input path')));
    caption.appendChild(colorKey);
  }
  function focusNode(n,center){surface?.focus(n.id,center);}
  async function select(n,navigate,source,focus){
    if(!n)return;
    var ticket=++selectionRevision;map.explorerMember=null;map.clearMapPreview?.();
    // Search is a chooser. Once a destination is chosen it must not continue
    // highlighting every other result or covering the destination's drawing.
    search.value=searchValue='';filter.value=filterValue='';updateResults();
    if(n.dataset.activation){operation=n;scope='';}else scope=n.id;
    emphasize();if(navigate)address(n,!!focus&&!!map.captureViewport?.()?.overview);
    await ready;if(ticket!==selectionRevision)return false;map.showNode?.(n);if(source)map.explainSource?.(source);
    if(focus)focusNode(n,!!n.dataset.activation||focus==='center');emit();return true;
  }
  function reset(){selectionRevision++;scope='';operation=null;surface?.clearHover();emphasize();map.clearInspection?.();emit();}
  map.showWholeMap=async function(){
    search.value=searchValue='';filter.value=filterValue='';updateResults();reset();
    address(null,true);document.querySelector('.nav .find')?.restoreSearch?.();
    await ready;await surface?.overview();emit();
  };
  map.componentSelection=function(){
    var selected=byID[scope||operation?.id];
    if(selected&&(selected.dataset.activation||selected.dataset.branch==='inputs')){
      var owner=selected.dataset.owner,component=nodes.find(function(n){return owner&&n.dataset.branch==='component'&&n.dataset.owner===owner;});
      return component?.dataset.owner||'';
    }
    if(map.captureViewport?.()?.componentsOpen===false)return '';
    var ids=path(scope||operation?.id),component=ids.map(function(id){return byID[id];}).find(function(n){return n.dataset.branch==='component';});
    return component?.dataset.owner||'';
  };
  map.selectComponent=function(id){return select(byID['system-component-'+id],true,null,'center');};
  map.closeDetails=function(){
    selectionRevision++;scope='';surface?.clearHover();emphasize();map.clearInspection?.();
    address(operation||null);emit();
  };
  clear.addEventListener('click',function(){reset();address(null);});
  function filterChanged(){searchValue=search.value;filterValue=filter.value;updateResults();emphasize();emit();}
  search.addEventListener('input',filterChanged);filter.addEventListener('change',filterChanged);
  nodes.forEach(function(n){n.addEventListener('click',function(e){e.preventDefault();e.stopImmediatePropagation();select(n,true);});});
  var inputWrites=nodes.filter(function(n){return n.dataset.activation;}).map(function(n){return {input:n,writes:JSON.parse(n.dataset.writes||'[]')};});
  var writeReading=null;
  map.addEventListener('repomap:reading',function(){
    if(!writeReading||writeReading.card.hidden)return;
    var key=map.explorerMember?.key||'';if(key===writeReading.key)return;
    writeReading.key=key;entityWrites(writeReading.node,writeReading.card);
  });
  function entityWrites(n,card){
    var entityKeys=new Set(repomapMembers.items(n).map(function(item){return repomapMembers.sourceKey(item.source);}));
    var selectedKey=map.explorerMember?.owner===n.id?map.explorerMember.key:'';
    var rows=inputWrites.flatMap(function(row){return row.writes.filter(function(write){return n.dataset.activation?row.input===n:entityKeys.has(repomapMembers.sourceKey(write.entity))&&(!selectedKey||repomapMembers.sourceKey(write.entity)===selectedKey);}).map(function(write){return {input:row.input,write:write};});});
    card.querySelector('.system-entity-writes')?.remove();
    if(!rows.length)return;
    var section=rmEl('section','system-entity-writes');section.appendChild(rmEl('h5','',rmT('State changes')));
    section.appendChild(rmEl('p','meta',rmT('Writes reachable in code; a call path does not prove they execute on every run.')));
    var entities=new Map();rows.forEach(function(row){var key=repomapMembers.sourceKey(row.write.entity);if(!entities.has(key))entities.set(key,[]);entities.get(key).push(row);});
    entities.forEach(function(changes){
      var heading=rmEl('h6'),entity=changes[0].write;
      var source=repomapMembers.sourceLink(entity.entity);source.textContent=entity.entity_name;heading.appendChild(source);section.appendChild(heading);
      var inputs=new Map();changes.forEach(function(row){if(!inputs.has(row.input.id))inputs.set(row.input.id,[]);inputs.get(row.input.id).push(row);});
      inputs.forEach(function(evidence){
        var input=evidence[0].input;
        if(!n.dataset.activation){var jump=rmEl('button','system-write-input',owner(input)+' / '+input.dataset.title);jump.type='button';jump.addEventListener('click',function(){select(input,true,null,true);});section.appendChild(jump);}
        var fields=rmEl('ul','plain');evidence.forEach(function(row){
          var write=row.write,field=rmEl('li');field.appendChild(rmEl('strong','',write.field));
          if(write.possible)field.appendChild(rmEl('span','possible',' · '+rmT('possible')));
          field.appendChild(document.createElement('br'));field.appendChild(repomapMembers.sourceLink(write.source));
          var path=rmEl('details','call-path');path.appendChild(rmEl('summary','',rmT('Call path')));var steps=rmEl('ol');
          write.steps.forEach(function(step){var li=rmEl('li');li.appendChild(rmEl('strong','',step.name));if(step.possible)li.appendChild(rmEl('span','possible',' · '+rmT(step.integration?'possible integration':'possible call')));li.appendChild(document.createElement('br'));li.appendChild(repomapMembers.sourceLink({Href:step.href,Open:step.open,Text:step.source,NoSource:step.no_source}));steps.appendChild(li);});path.appendChild(steps);field.appendChild(path);fields.appendChild(field);
        });section.appendChild(fields);
      });
    });
    card.querySelector('.map-card-intro').after(section);
  }
  map.addEventListener('repomap:inspect',function(e){
    var n=e.detail.node,card=e.detail.card;
    entityWrites(n,card);
    writeReading={node:n,card:card,key:map.explorerMember?.key||''};
    var group=document.getElementById((n.getAttribute('href')||'').slice(1));
    if(!n.dataset.activation){
      if(!n.dataset.branch)card.querySelector('.map-related-operations')?.remove();
      var selectedMembers=new Set(projection.leaves(n.id));selectedMembers.add(n.id);
      var reaching=nodes.filter(function(candidate){if(!candidate.dataset.activation)return false;return Array.from(projection.selection('',candidate.id).active).some(function(id){return selectedMembers.has(id);});});
      var inputs=rmEl('section','system-reaching-inputs');inputs.appendChild(rmEl('h5','',rmT(n.dataset.itemKind==='External communication'?'Inputs reaching this communication':'Inputs reaching this part')));
      if(reaching.length){
        var types=new Map();reaching.forEach(function(input){var type=input.dataset.activation;if(!types.has(type))types.set(type,[]);types.get(type).push(input);});
        types.forEach(function(choices,type){inputs.appendChild(rmEl('h6','',rmT(({request:'Incoming requests',command:'Commands',interaction:'User interactions',scheduled:'Scheduled tasks',continuous:'Background work'})[type]||'Inputs')));var links=rmEl('div','system-neighbours');choices.forEach(function(input){var b=rmEl('button','',owner(input)+' / '+input.dataset.title);b.type='button';b.addEventListener('click',function(){select(input,true,null,true);});links.appendChild(b);});inputs.appendChild(links);});
      }else inputs.appendChild(rmEl('p','meta',rmT('No input path to this item is recorded.')));
      if(JSON.parse(n.dataset.concepts||'[]').length)inputs.appendChild(rmEl('p','meta',rmT('Reaching a part does not by itself establish a change to its entities.')));
      if(!n.dataset.branch||n.dataset.branch==='communication'){card.querySelector('.map-card-intro').after(inputs);}
    }
    if(!n.dataset.branch&&!n.dataset.activation&&group?.classList.contains('group')){
      // The grouped connection reading replaces only the duplicated inventory.
      // The selected input's source-backed call path is a different explanation.
      var witness=card.querySelector('.call-path');
      if(witness){card.querySelector('.map-card-intro').after(witness);witness.open=true;}
      card.querySelector('.map-card-evidence')?.remove();
      group.querySelectorAll(':scope>.group-connections,:scope>.group-internal-connections,:scope>.group-inventory').forEach(function(section){
        var copy=section.cloneNode(true);copy.removeAttribute('id');copy.querySelectorAll('[id]').forEach(function(el){el.removeAttribute('id');});
        copy.querySelectorAll('.conn-peer a').forEach(function(link){
          var href=link.getAttribute('href'),peer=nodes.find(function(candidate){return candidate.getAttribute('href')===href||'#'+candidate.id===href;});
          if(peer)link.addEventListener('click',function(event){event.preventDefault();event.stopPropagation();select(peer,true,null,true);});
        });if(copy.classList.contains('group-connections'))card.querySelector('.map-card-intro').after(copy);else card.appendChild(copy);
      });
      return;
    }
    if(n.dataset.branch==='communication'){
      card.querySelector('.map-card-actions')?.remove();
      card.querySelector('.map-related-operations')?.remove();
      var calls=rmEl('section','system-communication-records');
      (n.dataset.children||'').split(/\s+/).filter(Boolean).forEach(function(id){
        var child=byID[id];if(!child)return;var item=rmEl('article');
        var jump=rmEl('button','',child.dataset.title);jump.type='button';jump.addEventListener('click',function(){select(child,true,null,true);});
        item.appendChild(jump);if(child.dataset.summary)item.appendChild(rmEl('p','',child.dataset.summary));calls.appendChild(item);
      });card.querySelector('.map-card-intro').after(calls);
    }
    if(n.dataset.itemKind==='External communication'){
      var proof=card.querySelector('.call-path');if(proof){card.querySelector('.map-card-intro').after(proof);proof.open=true;}
      card.querySelector('.map-card-evidence')?.remove();
    }
    if(n.dataset.activation){
      var pathState=projection.selection('',n.id),pathParts=rmEl('section','system-input-parts');
      pathParts.appendChild(rmEl('h5','',rmT('Parts on this input path')));
      var pathLinks=rmEl('div','system-neighbours');
      pathState.active.forEach(function(id){
        var part=byID[id];if(!part||part.dataset.activation)return;
        var link=rmEl('button','',part.dataset.title);link.type='button';
        link.addEventListener('click',function(){select(part,true,null,true);});pathLinks.appendChild(link);
      });
      if(pathLinks.childElementCount){pathParts.appendChild(pathLinks);card.appendChild(pathParts);}
      var linkedInputs=new Set();pathState.path.forEach(function(edge){[edge.from,edge.to].forEach(function(id){if(id!==n.id&&byID[id]?.dataset.activation)linkedInputs.add(id);});});
      if(linkedInputs.size){
        var connected=rmEl('section','system-linked-inputs');connected.appendChild(rmEl('h5','',rmT('Connected inputs')));
        linkedInputs.forEach(function(id){var link=rmEl('button','',byID[id].dataset.title);link.type='button';link.addEventListener('click',function(){select(byID[id],true,null,true);});connected.appendChild(link);});card.appendChild(connected);
      }
    }
    var relations=rmEl('div','system-neighbours'),members=new Set(projection.leaves(n.id)),seen=new Set();
    members.add(n.id);rawEdges.filter(function(r){return r.scope==='structure'||r.scope==='component';}).forEach(function(r){
      var outgoing=members.has(r.from),incoming=members.has(r.to);if(outgoing===incoming)return;
      var id=outgoing?r.to:r.from,key=(outgoing?'out:':'in:')+id;if(seen.has(key)||!byID[id])return;seen.add(key);
      var peerName=(byID[id].dataset.owner!==n.dataset.owner&&owner(byID[id])?owner(byID[id])+' / ':'')+byID[id].dataset.title;
      var b=rmEl('button','',(outgoing?'→ ':'← ')+peerName);b.type='button';b.addEventListener('click',function(){select(byID[id],true,null,true);});relations.appendChild(b);
    });
    if(relations.childElementCount)card.querySelector('.map-card-intro').appendChild(relations);
    var details=document.getElementById(n.dataset.detailsId);
    if(details){
      var content=n.dataset.branch==='component'?details.querySelector('.input-catalog'):details;
      if(content){
        var copy=content.cloneNode(true);copy.removeAttribute('id');copy.querySelectorAll('[id]').forEach(function(el){el.removeAttribute('id');});
        if(n.dataset.branch==='inputs'){
          copy.querySelector('h3')?.remove();
          copy.querySelectorAll('[data-integration-group]').forEach(function(group){group.remove();});
        }
        if(copy.matches('details'))copy.open=true;
        if(n.dataset.itemKind==='External communication')copy.querySelectorAll('.outbound-call').forEach(function(detail){detail.open=true;});
        copy.querySelectorAll('.outbound-caller-part').forEach(function(link){
          var peer=nodes.find(function(candidate){return candidate.getAttribute('href')===link.getAttribute('href');});
          if(peer)link.addEventListener('click',function(event){event.preventDefault();event.stopPropagation();select(peer,true,null,true);});
        });
        card.insertBefore(copy,card.querySelector('.map-all-members'));
      }
      if(n.dataset.branch==='component'){
        card.querySelector('.map-related-operations')?.remove();card.querySelector('.map-card-evidence')?.remove();card.querySelector('.map-all-members')?.remove();
        var readingLinks=rmEl('nav','system-component-reading');
        details.querySelectorAll(':scope>.component-flow>h3,:scope>.component-config>h3,:scope>.data-catalog,:scope>.component-reference>h3,:scope>.component-reference>.evidence-list>h3,:scope>.component-reference>.component-coverage').forEach(function(section){
          if(!section.id)return;
          var title=section.matches('.data-catalog')?rmT('Data'):section.matches('details')?section.querySelector('summary').textContent:section.textContent;
          var link=rmEl('a','map-details-link',title);link.href='#'+section.id;readingLinks.appendChild(link);
        });
        var heading=rmEl('h5',''),all=card.querySelector('.map-card-actions>.map-details-link');
        if(all){all.textContent=rmT('Component details');heading.appendChild(all);}else heading.textContent=rmT('Component details');
        if(readingLinks.childElementCount||all){readingLinks.prepend(heading);card.querySelector('.map-card-intro').after(readingLinks);}
      }
    }
  });
  async function layout(){
    map.setAttribute('aria-busy','true');await Promise.resolve();
    var componentsByOwner=new Map();nodes.forEach(function(n){if(n.dataset.branch==='component'&&n.dataset.owner)componentsByOwner.set(n.dataset.owner,n);});
    var items=nodes.map(function(n){
      var component=n.dataset.activation||n.dataset.branch==='inputs'?componentsByOwner.get(n.dataset.owner):null;
      return {id:n.id,title:n.dataset.title,branch:n.dataset.branch,activation:n.dataset.activation,lane:n.dataset.lane,
        summary:n.dataset.summary,subtitle:n.dataset.subtitle,sourceKind:n.dataset.sourceKind,
        role:n.dataset.role,roleRef:n.dataset.roleRef,language:n.dataset.language,componentKind:n.dataset.componentKind,
        componentOwner:component?.id||'',componentName:component?.dataset.title||'',
        children:(n.dataset.children||'').split(/\s+/).filter(function(id){return id&&(n.dataset.branch==='inputs'||!byID[id]?.dataset.activation);}),kind:kind(n),category:category(n)};
    });
    var relations=rawEdges;
    try{
      surface=await rmCreateFlow(map,stage,items,relations,projection.areas,projection.inputOwner,{
        select:function(id,center){select(byID[id],true,null,center?'center':false);},
        emphasis:function(state){visual=state;renderCaption();},
        connection:function(group){map.previewConnection?.({from:group.incoming?group.outside:group.area,to:group.incoming?group.area:group.outside,possible:group.relations.some(function(r){return r.possible;}),relations:group.relations});}
      });
      map.visibleEdges=surface.layout.edges;
      emphasize();
    }catch(error){caption.textContent=rmT('Could not arrange this map. Reload to try again.');console.error(error);}
    map.setAttribute('aria-busy','false');
  }
  map.captureViewport=function(){return surface?.capture()||null;};
  map.restoreViewport=function(saved){surface?.restore(saved);};
  map.exploreNode=function(id){select(byID[id],true);};
  map.displayedNode=function(n){return byID[aliases[n.id]]||n;};
  map.areaDescriptions=function(n){return path(n.id).filter(function(id){return id!==n.id&&byID[id].dataset.branch==='area'&&projection.leaves(id).length===1;}).map(function(id){
    var area=byID[id];return (area.dataset.title!==n.dataset.title?area.dataset.title+': ':'')+area.dataset.summary;
  });};
  map.operationChoices=function(id){var children=new Set(projection.leaves(id));return nodes.filter(function(n){return n.dataset.activation&&((n.dataset.near||'').split(/\s+/).some(function(near){return children.has(near);})||children.has(projection.inputOwner[n.id]));});};
  map.chooseOperation=function(id){return select(byID[id],true);};
  map.explorationLabel=function(){return [operation?.dataset.title,path(scope).map(function(id){return byID[id].dataset.title;}).join(' / '),map.explorerMember?.name].filter(Boolean).join(' · ')||rmT('System map');};
  map.resumeExploration=function(){var n=byID[scope]||operation;if(n)map.showNode(n);else map.clearInspection?.();};
  map.readingState=function(){return {scope:scope,operation:operation?.id||'',search:searchValue,filter:filterValue,numbered:numbered,source:map.explorerMember||null,viewport:map.captureViewport?.()||null};};
  map.restoreReadingState=async function(saved){if(!saved)return;var ticket=++selectionRevision;map.readingRestoring=true;try{scope=byID[saved.scope]?saved.scope:'';operation=byID[saved.operation]||null;search.value=searchValue=saved.search||'';filter.value=filterValue=saved.filter||'';numbered=saved.numbered!==false;syncStyle();updateResults();await ready;if(ticket!==selectionRevision)return;emphasize();map.resumeExploration();if(saved.source)map.explainSource(saved.source);else map.inspectConcept?.(-1);if(saved.viewport)map.restoreViewport(saved.viewport);}finally{if(ticket===selectionRevision){map.readingRestoring=false;emit();}}};
  map.revealNode=async function(n,allUses,source){n=byID[aliases[n?.id]]||byID[n?.dataset?.mapAlias]||byID[n?.id];if(!n)return;if(allUses)operation=null;if(await select(n,true,source,true))rmScrollToReading(map);};
  map.findNode=function(n,source){return map.revealNode(n,true,source);};
  function mapped(node){if(!node)return null;if(byID[node.id])return byID[node.id];if(aliases[node.id])return byID[aliases[node.id]];if(node.dataset.mapAlias)return byID[node.dataset.mapAlias];return byID['system-component-'+node.id]||byID['system-component-'+node.id.replace(/-(parts|inbound|external)$/,'')];}
  function hashChanged(){
    if(map.readingRestoring)return;
    var n=document.getElementById(location.hash.slice(1)),target=mapped(n);
    var saved=history.state?.repomapReading?.map;
    // The page navigator restores complete history visits. A later hashchange
    // must not reinterpret that restoration as a new centred selection.
    if(saved?.page===map.closest('[data-report-page]')?.id&&target&&
      (saved.value?.scope===target.id||saved.value?.operation===target.id))return;
    if(n===document.getElementById('overview')||n===document.getElementById('repository-map')){reset();return;}
    if(target)select(target,n!==target,null,true);
  }
  document.addEventListener('click',function(e){var a=e.target.closest('a[href^="#"]');if(!a||a.closest('[data-map-explorer]')||a.hasAttribute('data-reading-map-return')||a.hasAttribute('data-open')||e.ctrlKey||e.metaKey||e.shiftKey||e.altKey)return;var n=document.getElementById(a.getAttribute('href').slice(1)),target=mapped(n);if(!target)return;e.preventDefault();e.stopImmediatePropagation();map.revealNode(target,false);},true);
  if(map.hasAttribute('data-system-map'))document.querySelector('.nav-home')?.addEventListener('click',function(e){if(e.ctrlKey||e.metaKey||e.shiftKey||e.altKey)return;e.preventDefault();map.showWholeMap().then(function(){rmScrollToReading(map);});});
  window.addEventListener('hashchange',hashChanged);
  ready=layout();ready.then(function(){hashChanged();});
});})();
