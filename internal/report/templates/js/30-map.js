// Shared hover, inspection and pan/zoom behavior. The explorer owns scope and
// layout; without scripting, nodes still link to their code cards.
(function () {
  var maps = document.querySelectorAll('[data-map]');
  for (var index = 0; index < maps.length; index++) {
    bind(maps[index]);
  }

  function bind(map) {
    map.classList.add('map-interactive');
    // Reserve description space before measuring zoom or showing a preview.
    // Opening a card must neither cover a path nor move the pointed-at node.
    var stage = map.querySelector('[data-map-stage]');
    if (!stage) {
      stage = document.createElement('div');
      stage.className = 'map-stage'; stage.setAttribute('data-map-stage', '');
      var svg = map.querySelector('svg');
      svg.before(stage); stage.appendChild(svg);
    }
    var workspace = document.createElement('div');
    workspace.className = 'map-workspace';
    stage.before(workspace);
    if (map.hasAttribute('data-system-map')) {
      // The reading column stands beside the map's own controls and key as
      // well as its canvas, so it takes the height they take: beside the
      // canvas alone it was a box of some 500 px on a laptop screen, and the
      // answer sat at its bottom. The canvas keeps its own size.
      var column = document.createElement('div');
      column.className = 'map-canvas-column';
      map.querySelectorAll(':scope>.system-controls,:scope>.system-results,:scope>.system-selection').forEach(function (part) { column.appendChild(part); });
      column.appendChild(stage); workspace.appendChild(column);
      map.classList.add('map-reading-column');
    } else workspace.appendChild(stage);
    var inspector = document.createElement('aside');
    inspector.className = 'map-inspector';
    inspector.setAttribute('aria-label', rmT('Selected node details'));
    var content = document.createElement('div');
    content.className = 'map-inspector-content';
    content.tabIndex = 0;
    content.setAttribute('role', 'region');
    content.setAttribute('aria-label', rmT('Node description and sources'));
    var hint = document.createElement('div');
    hint.className = 'map-inspector-hint';
    var home = map.hasAttribute('data-system-map') && document.querySelector('template[data-system-reading-home]');
    if(home)hint.appendChild(home.content.cloneNode(true));
    else hint.textContent = rmT(map.hasAttribute('data-map-explorer')?'Click a part or code element to keep its explanation here.':'Hover or focus a node to read about it.');
    // The column is the reading's full height and scrolls as a page does;
    // it has no "More details" button, which moved a small box by a step a
    // reader barely noticed.
    content.appendChild(hint); inspector.appendChild(content); workspace.appendChild(inspector);
    var nodes = map.querySelectorAll('[data-node]');
    var edges = map.querySelectorAll('.map-edge, .map-edge-label');
    for (var index = 0; index < nodes.length; index++) {
      var node = nodes[index];
      node.addEventListener('repomap:preview', preview);
      node.addEventListener('repomap:previewend', clear);
      node.addEventListener('click', select);
    }

    function preview(event) {
      if(map.hasAttribute('data-system-map'))return;
      var node = event.currentTarget;
      edges = map.querySelectorAll('.map-edge, .map-edge-label');
      // An open area's description stays in the inspector while the map
      // shows its children. That hidden parent cannot highlight this view.
      if (!node.getClientRects().length) { clear(); return; }
      if (node.dataset.activation && map.dataset.operationPinned === 'true' && node !== map.inspectedOperation) return;
      var near = {};
      near[node.getAttribute('data-node')] = true;
      var listed = (node.getAttribute('data-near') || '').split(/\s+/);
      for (var index = 0; index < listed.length; index++) {
        if (listed[index]) near[listed[index]] = true;
      }
      for (var position = 0; position < nodes.length; position++) {
        nodes[position].classList.toggle('map-near', !!near[nodes[position].getAttribute('data-node')]);
      }
      for (var edge = 0; edge < edges.length; edge++) {
        var line = edges[edge];
        var paths=(line.getAttribute('data-operations')||'').split(/\s+/);
        var selected=node.getAttribute('data-node');
        var incident = node.getAttribute('data-activation') ? paths.indexOf(selected)>=0 : (line.getAttribute('data-from')===selected || line.getAttribute('data-to')===selected);
        line.classList.toggle('map-near', !!incident);
      }
      map.classList.add('map-previewing');
    }

    function clear() {
      map.classList.remove('map-previewing');
      for (var index = 0; index < nodes.length; index++) {
        nodes[index].classList.remove('map-near');
      }
      for (var edge = 0; edge < edges.length; edge++) {
        edges[edge].classList.remove('map-near');
      }
    }

    // A click follows the link the same way it would without scripting, and
    // additionally marks the group it landed on so the eye can find it.
    function select(event) {
      var href = event.currentTarget.getAttribute('href') || '';
      if (href.charAt(0) !== '#') return;
      var group = document.getElementById(href.slice(1));
      if (!group) return;
      var selected = document.querySelectorAll('.group-selected');
      for (var index = 0; index < selected.length; index++) {
        selected[index].classList.remove('group-selected');
      }
      group.classList.add('group-selected');
      if (group.hasAttribute('data-node')) {
        // Let the URL identify the endpoint, then show it with map context.
        requestAnimationFrame(function () {
          group.scrollIntoView({block:'center', inline:'center'});
          group.focus({preventScroll:true});
        });
      }
    }
  }
})();

// A long quote is clamped to a few lines so it cannot outshout the summary.
// Clicking it opens the whole thing; the anchor beside it always leads to the
// source either way.
(function () {
  var quotes = document.querySelectorAll('.claim p');
  for (var index = 0; index < quotes.length; index++) {
    quotes[index].addEventListener('click', function (event) {
      event.currentTarget.parentNode.classList.toggle('claim-open');
    });
  }
})();

// Map controls start at readable text size. Fitting the whole diagram is an
// explicit overview action; reset restores reading size and the starting position.
(function () {
  var maps = document.querySelectorAll('[data-map]');
  for (var index = 0; index < maps.length; index++) {
    bindControls(maps[index]);
  }

  function bindControls(map) {
    if(map.hasAttribute('data-system-map'))return;
    var controls = map.querySelector('[data-map-controls]');
    var stage = map.querySelector('[data-map-stage]');
    var svg = map.querySelector('svg');
    if (!controls || !stage || !svg) return;
    // SVG text scales with the viewBox. Measuring the already fitted picture
    // as the baseline made large maps start with 6–8 px labels.
    var baseWidth = svg.viewBox.baseVal.width;
    if (!baseWidth) return;
    controls.hidden = false;
    var smallestFont = Infinity;
    svg.querySelectorAll('text,[data-map-text]').forEach(function (text) {
      smallestFont = Math.min(smallestFont, parseFloat(getComputedStyle(text).fontSize));
    });
    var minimumText = parseFloat(getComputedStyle(controls.querySelector('.map-hint')).fontSize);
    var readableScale = minimumText / smallestFont;
    map.readableScale = readableScale;
    var scale;
    var homeBoxes = [], pendingFocus = false;
    var panHint = controls.querySelector('.map-hint');
    var dragPointer = null, startX = 0, startY = 0, leftAt = 0, topAt = 0;

    function focusOpened() {
      if (!pendingFocus || !stage.clientWidth || !stage.clientHeight) return;
      pendingFocus = false;
      if (!homeBoxes.length) { stage.scrollTo(0, 0); return; }
      var left=Infinity, top=Infinity, right=-Infinity, bottom=-Infinity;
      homeBoxes.forEach(function(b){left=Math.min(left,b.x);top=Math.min(top,b.y);right=Math.max(right,b.x+b.w);bottom=Math.max(bottom,b.y+b.h);});
      var box = {x:left,y:top,w:right-left,h:bottom-top};
      // An oversized area starts at its first component, without shrinking text
      // or changing ELK's placement. Neighbours remain reachable by panning.
      if (homeBoxes.length===1 || box.w*scale > stage.clientWidth-48 || box.h*scale > stage.clientHeight-48) {
        box=homeBoxes[0];
        // An expanded part can be taller than the viewport. Start at its
        // heading, never halfway through its code cubes.
        stage.scrollTo(Math.max(0,box.x*scale-24),Math.max(0,box.y*scale-24));
        revealInWindow(box);
        return;
      }
      stage.scrollTo(Math.max(0,(box.x+box.w/2)*scale-stage.clientWidth/2),
        Math.max(0,(box.y+box.h/2)*scale-stage.clientHeight/2));
      revealInWindow(box);
    }
    // The stage grows with the map instead of scrolling inside a capped box,
    // so an opened block low on a tall map is brought into the window by the
    // page itself. A stage that does scroll keeps its own framing.
    function revealInWindow(box) {
      if (!map.followOpened) return;
      map.followOpened = false;
      if (stage.scrollHeight > stage.clientHeight + 1) return;
      var inset = parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--toolbar-height')) || 0;
      var top = stage.getBoundingClientRect().top + box.y * scale - 24;
      if (top < inset || top > window.innerHeight - 120) window.scrollBy({top: top - inset - 16, behavior: 'instant'});
    }
    // A report opened at a question may initially hide this map. Frame it once
    // when it becomes visible; later resizing must preserve the reader's pan.
    var resize = new ResizeObserver(function () { updatePan(); focusOpened(); });
    resize.observe(stage);
    resize.observe(svg);

    function readable() {
      scale = map.hasAttribute('data-system-map') ? Math.max(readableScale,stage.clientWidth/baseWidth) : readableScale;
      apply();
    }

    function apply() {
      svg.style.width = baseWidth * scale + 'px';
      svg.style.maxWidth = 'none';
      svg.style.minWidth = '0';
      updatePan();
    }
    function updatePan() {
      var canPan = stage.clientWidth > 0 && stage.clientHeight > 0 &&
        (stage.scrollWidth > stage.clientWidth + 1 || stage.scrollHeight > stage.clientHeight + 1);
      stage.classList.toggle('map-zoomed', canPan);
      if (panHint) panHint.hidden = !canPan;
      if (!canPan) endDrag();
      return canPan;
    }
    function zoom(factor) {
      scale = Math.min(4, Math.max(0.4, scale * factor));
      apply();
    }
    readable();
    map.captureViewport=function(){return {scale:scale,left:stage.scrollLeft,top:stage.scrollTop};};
    map.restoreViewport=function(saved){
      if(!saved)return;pendingFocus=false;
      if(Number.isFinite(saved.scale)){scale=saved.scale;apply();}
      stage.scrollTo(saved.left||0,saved.top||0);
    };
    var viewportTimer=0;
    stage.addEventListener('scroll',function(){
      if(viewportTimer)return;
      viewportTimer=setTimeout(function(){viewportTimer=0;map.dispatchEvent(new Event('repomap:viewport'));},250);
    });
    map.addEventListener('repomap:layout',function(event){
      baseWidth=svg.viewBox.baseVal.width;readable();
      homeBoxes=event.detail?.focus||[];
      if(homeBoxes.length){pendingFocus=true;focusOpened();}
    });
    controls.addEventListener('click', function (event) {
      var button = event.target.closest('button');
      if (!button) return;
      if (button.hasAttribute('data-map-zoom')) {
        zoom(parseFloat(button.getAttribute('data-map-zoom')));
      } else if (button.hasAttribute('data-map-fit')) {
        pendingFocus=false;
        var heightLimit = parseFloat(getComputedStyle(stage).maxHeight);
        var fitHeight = Number.isFinite(heightLimit) ? heightLimit / svg.viewBox.baseVal.height : Infinity;
        scale = Math.min(readableScale, stage.clientWidth / baseWidth, fitHeight);
        apply();
        stage.scrollTo(0, 0);
      }
      map.dispatchEvent(new Event('repomap:viewport'));
    });

    // Dragging pans the stage. Only an explicit scope change lays out nodes.
    stage.addEventListener('pointerdown', function (event) {
      if (event.button !== 0 || event.isPrimary === false || dragPointer !== null ||
          event.target.closest('a,button,input,select,textarea,[role="button"],[contenteditable]') || !updatePan()) return;
      dragPointer = event.pointerId;
      startX = event.clientX; startY = event.clientY;
      leftAt = stage.scrollLeft; topAt = stage.scrollTop;
      stage.classList.add('map-grabbing');
      stage.setPointerCapture(event.pointerId);
      event.preventDefault();
    });
    stage.addEventListener('pointermove', function (event) {
      if (event.pointerId !== dragPointer) return;
      stage.scrollLeft = leftAt - (event.clientX - startX);
      stage.scrollTop = topAt - (event.clientY - startY);
    });
    function endDrag(event) {
      if (event && event.pointerId !== dragPointer) return;
      var captured = dragPointer;
      dragPointer = null;
      stage.classList.remove('map-grabbing');
      if (captured !== null && stage.hasPointerCapture(captured)) stage.releasePointerCapture(captured);
    }
    stage.addEventListener('pointerup', endDrag);
    stage.addEventListener('pointercancel', endDrag);
    stage.addEventListener('lostpointercapture', endDrag);
  }
})();

// A declaration's name as its tile writes it: the name and what follows it
// in a class box, "(c: redisClient *)" for a function, ": int" for a field.
function rmDeclarationText(node,concept){
  var symbols=rmPage.data(node,'symbols')||[];
  var href=concept.source&&concept.source.Href,symbol=href&&symbols.find(function(s){return s.href===href&&s.kind!=='field';});
  return symbol?symbol.name+(symbol.text||''):concept.name;
}
// The declaration's code: the existing link, and where it stands.
function rmOpenCode(source){
  var fragment=document.createDocumentFragment();
  if(source.Href||source.Open){var link=repomapMembers.sourceLink(source);link.textContent=rmT('Open code ↗');fragment.append(link,document.createTextNode(' '));}
  var place=rmEl('span','meta',source.Text);if(source.NoSource)place.title=rmT('No source');fragment.appendChild(place);
  return fragment;
}
// A pinned input's entry into a part, from its saved reading: the calls
// entering the part from a part reached earlier, the other calls counted,
// and the declarations they name. Null when the path does not enter it.
function rmInputPart(operation,id){
  var path=rmPage.data(operation,'inputPath');
  var part=path&&(path.parts||[]).find(function(entry){return entry.part===id;});
  if(!part||!(part.entered||[]).length&&!part.others)return null;
  return {entered:part.entered||[],others:part.others||0,decls:path.decls||[]};
}
// The inputs dispatched at a site, by name, each with its handler, and a box
// that filters them by either (owner, 2026-09-28: "one of 94 handlers"
// opened nothing). The list is part of the column's own scroll, folded
// when long, as a side of more than twelve names is, and
// opened in place (owner, 2026-09-29: its own scroller had taken the wheel
// and the page scrolled under it). An input's name reads the input, its
// handler's name the handler's declaration.
function rmDispatchedList(map,site,decls){
  var ctx=map.readingContext?map.readingContext():null;
  var fold=rmEl('details','map-dispatched');fold.open=site.dispatched.length<=12;
  var head=rmEl('summary');head.appendChild(rmEl('span','',rmT('Its handlers by input')));fold.appendChild(head);
  var filter=rmEl('input','map-dispatched-filter');filter.type='search';filter.placeholder=rmT('Filter');filter.setAttribute('aria-label',rmT('Filter'));
  var list=rmEl('ul','map-dispatched-list');
  site.dispatched.forEach(function(entry){
    var input=document.getElementById(entry.input),item=rmEl('li'),name=rmEl('button','',input?input.dataset.title:entry.input);name.type='button';
    name.addEventListener('click',function(){map.chooseOperation?.(entry.input);});
    item.append(name,document.createTextNode(' → '),rmSiteDeclName(ctx,decls[entry.handler]||{name:''}));
    item.dataset.filter=(item.textContent||'').toLowerCase();list.appendChild(item);
  });
  filter.addEventListener('input',function(){var words=filter.value.trim().toLowerCase();list.querySelectorAll('li').forEach(function(item){item.hidden=!!words&&item.dataset.filter.indexOf(words)<0;});});
  fold.append(filter,list);
  return fold;
}
// What a dispatch site's "one of N" counts: the handlers it chooses
// between, or its functions and how many of them are handlers when some
// are not (page_input_path.go).
function rmSiteHandlers(site){
  return site.handlers<site.of?rmT('one of {0} functions, {1} of them handlers',site.of,site.handlers):rmT('one of {0} handlers',site.of);
}
// A declaration a site's reading names: a link into all of its code whose
// plain click reads it in its part, its part named on hover, as every name
// in the column is; a plain name when it has neither.
function rmSiteDeclName(ctx,decl){
  var part=ctx?rmFlowPart(ctx,decl.part):null,key=decl.href||decl.open||'';
  if(!key)return rmEl('span','',decl.name||'');
  return rmDeclName({name:decl.name,href:decl.href,open:decl.open,code:decl.code,key:key},decl.name||'',part?function(){ctx.readDeclIn(part,key);}:null,part?part.dataset.title:'');
}
// Each handler several inputs dispatched at a site share, with those inputs:
// why the site's inputs outnumber its handlers.
function rmSharedHandlers(site,decls,title){
  return (site.shared||[]).map(function(shared){
    return rmT('{0} handles {1} of these inputs: {2}',(decls[shared.handler]||{}).name||'',shared.inputs.length,shared.inputs.map(title).join(', '));
  });
}
// A dispatch site read with its declaration (page_input_path.go): how many
// it chooses between and how many inputs are dispatched there, then the
// inputs whose own code reaches it, each with its calls to it, or that no
// input does. Which of them, if any, leads to an input dispatched there is
// not established: that list stands here, never in a dispatched input's
// reading, where a reader took it for GET's route.
function rmSiteReading(map,node,key){
  var box=rmEl('div','map-concept-dispatch'),readings=null,ctx=map.readingContext?map.readingContext():null;
  readings=rmPage.data(node,'dispatch');
  if(!readings||!key)return box;
  var decls=readings.decls||[];
  function name(index){return rmSiteDeclName(ctx,decls[index]||{name:''});}
  (readings.sites||[]).forEach(function(site){
    var decl=decls[site.site];if(!decl||(decl.href||decl.open)!==key)return;
    // Named, not counted (owner, 2026-09-29: no digits in the column); the
    // counts stay on its hover.
    var heading=rmEl('h6','',rmT('Dispatch site'));
    heading.title=[rmT('Dispatch site · {0} · {1} inputs dispatched here',rmSiteHandlers(site),site.inputs)].concat(rmSharedHandlers(site,decls,function(id){return document.getElementById(id)?.dataset.title||id;})).join('\n');
    box.appendChild(heading);
    if((site.dispatched||[]).length)box.appendChild(rmDispatchedList(map,site,decls));
    if(!(site.reached_from||[]).length){box.appendChild(rmEl('p','meta',rmT('No input reaches {0} by calls',decl.name)));return;}
    box.appendChild(rmEl('p','',rmT('{0} is reached from these inputs:',decl.name)));
    site.reached_from.forEach(function(entry){
      var input=document.getElementById(entry.input),row=rmEl('div','map-concept-reached');
      var button=rmEl('button','',input?input.dataset.title:entry.input);button.type='button';
      button.addEventListener('click',function(){map.chooseOperation?.(entry.input);});row.appendChild(button);
      var list=rmEl('ul','plain');
      (entry.calls||[]).forEach(function(call){var item=rmEl('li');item.append(name(call[0]),document.createTextNode(' → '),name(call[1]));if(call[2]&1)item.appendChild(rmEl('span','possible',' · '+rmT('possible')));list.appendChild(item);});
      row.appendChild(list);box.appendChild(row);
    });
    box.appendChild(rmEl('p','meta',rmT('Which of these, if any, leads to an input dispatched here is not established.')));
  });
  return box;
}
// Who calls a declaration and what it calls, read from the relation rows
// its part already lists, grouped by the part at the other end. The
// declaration at the other end is one line, its name alone however many
// places the call is written (no line numbers), and choosing it reads that
// declaration in its part, so a chain is followed one call at a time. A
// relation other than a call keeps its own words. Nothing is inferred: a
// relation the part does not list is not here.
function rmDeclarationRelations(map,node,key,nodes){
  var box=rmEl('div','map-concept-relations'),group=document.getElementById((node.getAttribute('href')||'').slice(1));
  var sides={in:new Map(),out:new Map()};
  if(group&&key)group.querySelectorAll('.conn[data-kind]').forEach(function(row){
    var d=row.dataset,peer=row.closest('.conn-group'),link=peer&&peer.querySelector('.conn-peer a'),label=peer&&peer.querySelector('.conn-peer .lbl');
    var part=peer?{href:link?link.getAttribute('href'):'',title:(link||label||{}).textContent||''}:{href:node.getAttribute('href')||'',title:node.dataset.title||'',own:true};
    [['in',rmPage.link(d.toDecl)===key,rmPage.link(d.fromDecl),d.fromName],['out',rmPage.link(d.fromDecl)===key,rmPage.link(d.toDecl),d.toName]].forEach(function(end){
      if(!end[1])return;
      var parts=sides[end[0]],at=part.href||part.title;
      if(!parts.has(at))parts.set(at,{part:part,decls:new Map()});
      var decls=parts.get(at).decls,other=(d.kind==='calls'||d.kind==='invokes_external'?'':d.kind)+'\0'+(end[2]||end[3]);
      if(!decls.has(other))decls.set(other,{key:end[2],name:end[3],kind:d.kind,sentence:row.querySelector(':scope>p')?.firstChild?.textContent||'',rows:[]});
      decls.get(other).rows.push(row);
    });
  });
  function peerNode(part){return Array.prototype.find.call(nodes,function(n){return n.getAttribute('href')===part.href||'#'+n.id===part.href;});}
  function read(part,declKey){
    if(part.own){map.explainSource?.({key:declKey});return;}
    var peer=peerNode(part);if(peer&&map.revealNode)map.revealNode(peer,false,declKey?{key:declKey}:null);
  }
  [['in','Called by'],['out','Calls']].forEach(function(side){
    // The declaration's own part first: its neighbours in the code it is read in.
    var parts=new Map(Array.from(sides[side[0]]).sort(function(a,b){return (b[1].part.own?1:0)-(a[1].part.own?1:0);}));if(!parts.size)return;
    var section=rmEl('section','map-concept-side');section.appendChild(rmEl('h6','',rmT(side[1])));
    parts.forEach(function(entry){
      var head=rmEl('div','map-concept-part'),peer=entry.part.own?null:peerNode(entry.part);
      if(peer){var go=rmEl('button','',entry.part.title);go.type='button';go.addEventListener('click',function(){read(entry.part,'');});head.appendChild(go);}
      else head.textContent=entry.part.title;
      section.appendChild(head);
      var list=rmEl('ul','plain');
      entry.decls.forEach(function(decl){
        var item=rmEl('li'),words=decl.kind==='calls'||decl.kind==='invokes_external'?decl.name:decl.sentence.trim();
        var name=rmEl(entry.part.own||peer?'button':'span','map-concept-decl',words);
        if(name.tagName==='BUTTON'){name.type='button';name.addEventListener('click',function(){read(entry.part,decl.key);});}
        item.appendChild(name);
        if(decl.rows.every(function(row){return row.querySelector(':scope>p>.possible');}))item.appendChild(rmEl('span','possible',rmT('possible')));
        list.appendChild(item);
      });
      section.appendChild(list);
    });
    box.appendChild(section);
  });
  if(!box.childElementCount)box.appendChild(rmEl('p','meta',rmT("No call to or from it is listed among this part's connections.")));
  return box;
}

// The reading layer over the map, written from the journeys a reader
// actually makes, not from what a canvas can do:
//   - "what is this box?" — pointing at a node fills the map's details panel:
//     the summary, the size, and its arrows as sentences. The question is
//     answered without leaving the map, so a jump is for reading in full.
//   - "where is this on the map?" — every group card gets an rmT("on the map")
//     link back to its node, which is lit for a moment; the round trip is one
//     click each way.
//   - "how does a request go through?" — pointing at a node on the main
//     path lights the whole path and its arrows, and the card says which
//     step this is.
// Hover does not change the layout.
(function () {
  var maps = document.querySelectorAll('[data-map]');
  for (var index = 0; index < maps.length; index++) {
    bindReading(maps[index]);
  }
  // These fragments contain both text and quoted attributes. Text-node HTML
  // serialization alone leaves quotes intact and cannot protect data-open.
  function escapeText(text) {return String(text||'').replace(/[&<>"']/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c];});}
  function titleOf(node) {
    if (node.dataset.title) return node.dataset.title;
    var lines = node.querySelectorAll('.map-node-title');
    var parts = [];
    for (var i = 0; i < lines.length; i++) parts.push(lines[i].textContent);
    return parts.join(' ');
  }
  function bindReading(map) {
    var nodes = map.querySelectorAll('[data-node]');
    var hoverCard = document.createElement('div');
    hoverCard.className = 'map-card map-hover-card';
    hoverCard.setAttribute('role', 'tooltip');
    var byId = {};
    for (var i = 0; i < nodes.length; i++) byId[nodes[i].getAttribute('data-node')] = nodes[i];
    var edges = map.querySelectorAll('.map-edge');
    var card = document.createElement('div');
    card.className = 'map-card map-card-docked';
    card.hidden = true;
    var content = map.querySelector('.map-inspector-content');
    var inspector=content.closest('.map-inspector'),heading=document.createElement('div');
    heading.className='map-inspector-heading';content.before(heading);
    content.appendChild(card);
    if(map.hasAttribute('data-system-map')){
      // Only connection evidence has a hover preview. Selected card details
      // remain mounted and usable while crossing the canvas to reach them.
      var mapPreview=document.createElement('div');mapPreview.className='system-map-preview';mapPreview.hidden=true;inspector.appendChild(mapPreview);
      map.clearMapPreview=function(){mapPreview.hidden=true;inspector.classList.remove('has-map-preview');};
      map.previewConnection=function(edge){mapPreview.replaceChildren();var title=document.createElement('strong'),body=document.createElement('div');connectionReading(edge,title,body);mapPreview.append(title,body);mapPreview.hidden=false;inspector.classList.add('has-map-preview');};
    }
    new ResizeObserver(function () { content.dispatchEvent(new Event('scroll')); }).observe(card);
    // What a reader opens in the reading column comes into view: Redis's
    // "Source details · 11" under Persistence opened at the column's foot
    // with three of its five lines below it, and Open all put four of them
    // there. Only the reader's own click scrolls; a reading restored with
    // its evidence open keeps its place.
    content.addEventListener('click',function(event){
      var summary=event.target.closest&&event.target.closest('summary'),opener=event.target.closest&&event.target.closest('[data-open-all]');
      var block=summary&&summary.parentElement.tagName==='DETAILS'&&!summary.parentElement.open?summary.parentElement:opener&&opener.closest('details');
      if(!block||!content.contains(block))return;
      requestAnimationFrame(function(){if(block.open&&(!opener||rmOpenAllWord(opener.closest('.connection-evidence')||block)==='Close all'))rmRevealOpened(content,block);});
    });

    function sentences(id) {
      edges = map.querySelectorAll('.map-edge');
      var out = [];
      for (var e = 0; e < edges.length; e++) {
        var edge = edges[e];
        var label = edge.dataset.label || '';
        var from = edge.getAttribute('data-from'), to = edge.getAttribute('data-to');
        if (from === id && byId[to]) out.push('\u2192 ' + (label ? label + ' \u00b7 ' : '') + titleOf(byId[to]));
        else if (to === id && byId[from]) out.push('\u2190 ' + titleOf(byId[from]) + (label ? ' \u00b7 ' + label : ''));
        if (out.length === 4) break;
      }
      return out;
    }
    var inspectedNode=null, inspectionKey='', inspectionRevision=0, inspectionPending=false, remembered=new Map();
    // The declaration a returning reading had, and whether one newly chosen
    // with it reads from the top instead (owner, 2026-09-29: a tile chosen
    // in the part being read kept the column where the last one stood, and
    // a click meant for a step hit another name).
    var savedConcept='',freshDeclaration=false;
    function remember(){
      if(inspectionPending||!inspectedNode||card.classList.contains('map-card-connection'))return;
      remembered.set(inspectionKey,{concept:map.explorerMember?.key,scroll:content.scrollTop,expanded:rmOpenFolds(card)});
    }
    content.addEventListener('scroll',remember);
    // A fold opened or closed is remembered at once: returning to the
    // reading by a click, not only by Back, finds it as it was left.
    card.addEventListener('toggle',function(){if(!inspectionPending)remember();rmExpandAllWord(card);},true);
    function show(node) {
      // A new selection reads from its top. Its remembered scroll and open
      // evidence come back only when the reader returns to it: Back, or
      // the same item shown again.
      var restoring=map.readingRestoring,returning=restoring||inspectedNode===node;
      remember();inspectedNode=node;inspectionKey=node.id+'\0'+(map.inspectedOperation?.id||'');inspectionPending=true;
      map.explorerMember=null;
      // Its folds, scroll and declaration come back when the reader returns
      // to it (Back, or the same item shown again); reached anew, a reading
      // opens in its default state (owner, 2026-09-29: after "Expand all"
      // redis-server's column stood 9,877 px tall on every later visit).
      var memory=remembered.get(inspectionKey),saved=returning?memory:null, ticket=++inspectionRevision;
      savedConcept=saved?.concept||'';freshDeclaration=false;
      content.scrollTop = 0;
      card.classList.remove('map-card-connection');
      var id = node.getAttribute('data-node');
      // A part of the system map is read from its prepared reading
      // (31-reading-column.js): its title in its box, its members by kind
      // and name, its callers above and its callees below.
      var reading = map.hasAttribute('data-system-map') && !node.dataset.branch && !node.dataset.activation ? rmGroupReading(node) : null;
      var counts = node.dataset.branch ? rmT('{0} parts',(node.dataset.children||'').split(/\s+/).filter(Boolean).length) : node.dataset.activation ? rmT(node.dataset.activation) : node.dataset.members ? rmT('{0} symbols',Number(node.dataset.members)) : '';
      var html = '<div class="map-card-intro">';
      if(map.exploreNode){
        var current=node.dataset.activation?map.explorerOperation===node&&map.dataset.operationPinned==='true':map.explorerScope===id;
        html+='<span class="map-card-kind">'+(current?'':rmT('Preview')+' · ')+(node.dataset.itemKind?rmT(node.dataset.itemKind):node.dataset.activation?rmT('Operation')+(counts?' · '+escapeText(counts):''):node.dataset.branch==='component'?rmT('Component'):node.dataset.branch?rmT('Area'):rmT(map.itemKind?map.itemKind(node):'Part'))+'</span>';
      }
      html += '<b>' + escapeText(titleOf(node)) + '</b>';
      var summary = node.getAttribute('data-summary');
      // The model's words, told apart by their style alone (a component's
      // role before its purpose); a part's stand under its box.
      var modelTitle=' title="'+escapeText(rmT('written by the model'))+'"';
      if (node.dataset.branch==='component'&&map.hasAttribute('data-system-map')&&(summary||node.dataset.role)) html += '<p class="map-card-summary model"'+modelTitle+'>'+(node.dataset.role?'<strong data-display-ref="'+escapeText(node.dataset.roleRef)+'">'+escapeText(node.dataset.role)+'</strong>'+(summary?' — ':''):'')+(summary?'<span data-display-ref="'+escapeText(node.dataset.summaryRef)+'">'+escapeText(summary)+'</span>':'')+'</p>';
      else if (summary && !reading) html += '<p class="map-card-summary model"'+modelTitle+' data-display-ref="'+escapeText(node.dataset.summaryRef)+'">' + escapeText(summary) + '</p>';
      // An input is read by its handler: the declaration its registration
      // hands over, a link into the code.
      if (node.dataset.handler && node.dataset.handler !== titleOf(node)) {
        var handler = node.dataset.handlerSource ? '<a target="_blank" href="'+escapeText(node.dataset.handlerSource)+'">'+escapeText(node.dataset.handler)+'</a>'
          : node.dataset.handlerOpen ? '<a href="#" data-open="'+escapeText(node.dataset.handlerOpen)+'">'+escapeText(node.dataset.handler)+'</a>'
          : '<span'+(node.dataset.handlerNoSource==='true'?' title="'+escapeText(rmT('No source'))+'"':'')+'>'+escapeText(node.dataset.handler)+'</span>';
        html += '<p class="map-card-handler">' + rmT.html('handled by') + ' ' + handler + '</p>';
      }
      // An input whose handler is not established is read where its call
      // declares it: the part is where it is declared, not what handles it.
      // A catalogued input reads its catalogue instead (rmCatalogueSection).
      if (node.dataset.handlerUnknown==='true' && !node.dataset.catalogue) html += '<p class="map-card-handler">' + rmT.html('handler not established') + '</p>';
      // Its registration as the code wrote it: a command table's row says
      // its arity and flags ({"rpush",rpushCommand,3,REDIS_CMD_BULK|…}).
      // A setting's comparison as written links its line (owner, 2026-09-29).
      if (node.dataset.written) html += '<p class="map-card-written">' + (node.dataset.source ? '<a target="_blank" href="'+escapeText(node.dataset.source)+'"><code>' + escapeText(node.dataset.written) + '</code></a>' : '<code>' + escapeText(node.dataset.written) + '</code>') + '</p>';
      if (node.dataset.operationGroup && !node.dataset.catalogue) html += '<span class="map-card-meta">' + (node.dataset.handlerUnknown==='true' ? rmT.html('declared in') + ' ' : '') + escapeText(node.dataset.operationGroup) + '</span>';
      var source=node.getAttribute('data-source');
      // An input with no handler established says where it is parsed.
      var parsed=node.dataset.activation&&node.dataset.handlerUnknown==='true'?rmT.html('parsed at')+' ':'';
      if(source) html += '<p class="map-card-source">'+parsed+'<a target="_blank" href="'+escapeText(source)+'">'+escapeText(node.getAttribute('data-source-text')||rmT('Source'))+'</a></p>';
      else if(node.dataset.open) html += '<p class="map-card-source"><a href="#" data-open="'+escapeText(node.dataset.open)+'">'+escapeText(node.dataset.sourceText||rmT('Source'))+'</a></p>';
      else if(node.dataset.noSource==='true') html += '<p class="map-card-source"><span title="'+escapeText(rmT('No source'))+'">'+escapeText(node.dataset.sourceText||rmT('Source'))+'</span></p>';
      html += '</div>';
      var concepts = map.exploreNode ? repomapMembers.items(node) : rmPage.data(node,'concepts') || [];
      card.classList.toggle('map-card-has-concepts', concepts.length > 0);
      if (concepts.length && !reading) {
        html+='<div class="map-concepts" hidden><strong data-concept-name></strong><code class="map-concept-declaration" data-concept-declaration></code><div class="map-member-fields" data-concept-fields></div><p class="model" data-concept-explanation></p><p class="map-concept-source" data-concept-source></p><div data-concept-dispatch></div><div data-concept-relations></div></div>';
      }
      if(map.areaDescriptions){
        var descriptions=Array.from(new Set(map.areaDescriptions(node))).filter(function(text){return text&&text!==summary;});
        if(descriptions.length){
          html+=("<details><summary>"+rmT.html("Additional model description")+"</summary>");
          descriptions.forEach(function(text){html+='<p>'+escapeText(text)+'</p>';});html+='</details>';
        }
      }
      html += ("<details class=\"map-card-evidence\"><summary>"+rmT.html("Code and connections")+"</summary>");
      if (counts && !node.dataset.activation) html += '<span class="map-card-meta">' + escapeText(counts) + '</span>';
      var operation = map.inspectedOperation;
      var witness = operation && !node.dataset.activation && rmInputPart(operation, id);
      if (witness) {
        // Every call entering this part on the pinned input's path from a
        // part reached earlier, and how many others enter it: no shortest
        // route is chosen.
        html += '<details class="call-path"><summary>'+rmT.html('Why it appears in {0}',operation.dataset.title)+'</summary><ol>';
        witness.entered.forEach(function (entry) {
          var caller = witness.decls[entry[0]] || {name:''}, callee = witness.decls[entry[1]] || {name:''};
          var mark = entry[2]&4 ? 'possible integration' : entry[2]&2 ? (entry[2]&1 ? 'possible read' : 'read') : entry[2]&1 ? 'possible call' : '';
          html += '<li>'+(mark?("<span class=\"possible\">"+rmT.html(mark)+"</span> "):'')+escapeText(caller.name)+' → <strong>'+escapeText(callee.name)+'</strong><br>';
          if (callee.href) html += '<a target="_blank" href="'+escapeText(callee.href)+'">'+escapeText(callee.source)+'</a>';
          else if (callee.open) html += '<a href="#" data-open="'+escapeText(callee.open)+'">'+escapeText(callee.source)+'</a>';
          else if (callee.no_source) html += '<span title="'+escapeText(rmT('No source'))+'">'+escapeText(callee.source)+'</span>';
          else html += escapeText(callee.source||'');
          html += '</li>';
        });
        html += '</ol>';
        if (witness.others) html += '<p class="meta">'+rmT.html('{0} more calls into this part come from other code on this path',witness.others)+'</p>';
        html += '</details>';
      }
      var step = map.traceIndex ? map.traceIndex(node) : -1;
      if (step >= 0) html += '<span class="map-card-meta">'+rmT.html('step {0} of {1} on the main path',step+1,map.traceLength)+'</span>';
      var keys = (node.getAttribute('data-keys') || '').split(' | ').filter(Boolean);
      if(map.exploreNode&&concepts.length&&keys.every(function(key){var name=key.split(' — ')[0];return concepts.filter(function(concept){return concept.name===name;}).length===1;}))keys=[];
      keys=keys.map(escapeText);
      if (keys.length) html += '<ul class="map-card-keys">' + keys.map(function (k) { return '<li><code>' + k.replace(/ — .*$/, '') + '</code>' + (k.indexOf(' — ') > 0 ? ' — ' + k.slice(k.indexOf(' — ') + 3) : '') + '</li>'; }).join('') + '</ul>';
      var arrows = map.classList.contains('repo-map') || witness ? [] : sentences(id);
      if (arrows.length) html += '<ul>' + arrows.map(function (a) { return '<li>' + escapeText(a) + '</li>'; }).join('') + '</ul>';
      card.innerHTML = html+'</details>';
      // Keep the current object and term above the scrolling evidence. This
      // is a reserved row of the inspector, never an overlay on map controls.
      heading.replaceChildren();
      var objectHeading=document.createElement('div'),kindHeading=card.querySelector('.map-card-kind'),titleHeading=card.querySelector('.map-card-intro>b');
      if(kindHeading)objectHeading.appendChild(kindHeading);
      // A part, a declaration and an Inputs collection name what they are
      // in their heading, with the frame holding them as a link up; their
      // own name stands in the reading, in its box. A component states its
      // language and kind beside the word.
      var systemMap=map.hasAttribute('data-system-map');
      if(kindHeading&&systemMap&&node.dataset.branch==='component'&&node.dataset.language)kindHeading.textContent=rmT('Component')+' · '+rmLanguageNames[node.dataset.language]+(node.dataset.componentKind?' '+rmT(node.dataset.componentKind):'');
      if(reading||systemMap&&node.dataset.branch==='inputs')titleHeading.remove();
      else objectHeading.appendChild(titleHeading);
      var kindWord=kindHeading?kindHeading.textContent:'';
      if(reading)rmHeadingUp(map,kindHeading,map.parentFrame(node));
      // One way back to the program's Main flow while any of its component
      // is read (owner, 2026-09-29: it had taken a reader five actions).
      var ownerPage=systemMap&&map.readMainFlow&&node.dataset.owner?document.getElementById(node.dataset.owner):null;
      if(ownerPage&&ownerPage.querySelector(':scope>.component-flow')){
        var toFlow=document.createElement('button');toFlow.type='button';toFlow.className='map-main-flow-link';toFlow.textContent=rmT('Main flow');
        toFlow.addEventListener('click',function(){map.readMainFlow(node.dataset.owner);});heading.appendChild(toFlow);
      }
      heading.appendChild(objectHeading);
      if(map.closeDetails){
        objectHeading.className='map-object-heading';
        var close=document.createElement('button');close.type='button';close.className='map-close-details';
        close.setAttribute('aria-label',rmT('Close details'));close.innerHTML='<svg viewBox="0 0 20 20" width="20" height="20" aria-hidden="true"><path d="m5 12 5-5 5 5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>';
        close.addEventListener('click',function(){map.closeDetails();});objectHeading.appendChild(close);
        // Every fold of the reading at once.
        var expand=document.createElement('button');expand.type='button';expand.className='map-expand-all';expand.dataset.expandAll='';
        expand.addEventListener('click',function(){var open=expand.dataset.state!=='open';rmFolds(card).forEach(function(detail){detail.open=open;});rmExpandAllWord(card);remember();});
        objectHeading.insertBefore(expand,close);
      }
      heading.classList.toggle('has-concepts',concepts.length>0&&!reading);
      var partView=null,declHolder=null;
      if(reading){
        var ctx=map.readingContext(),built=rmPartView(ctx,node,reading);partView=built.view;
        declHolder=document.createElement('div');declHolder.className='map-decl-holder';declHolder.hidden=true;
        card.querySelector('.map-card-intro').after(partView,declHolder);
      }
      // A chosen declaration is read by itself: its name as its tile writes
      // it, the model's line when there is one, its code, and who calls it
      // and what it calls, from the relation rows the part already lists.
      // Without a line it says nothing about one: "No explanation saved"
      // had answered a click on every tile but the keys.
      map.inspectConcept=function(index){
        if(reading){readDeclaration(index);return;}
        var panel=card.querySelector('.map-concepts');if(!panel)return;
        panel.hidden=index<0;card.classList.toggle('map-card-has-concepts',index>=0);
        card.querySelectorAll('.map-member-name[aria-current]').forEach(function(name){name.removeAttribute('aria-current');});
        if(index<0){map.explorerMember=null;map.dispatchEvent(new Event('repomap:reading'));return;}
        var concept=concepts[index],source=concept.source,key=repomapMembers.sourceKey(source);
        map.explorerMember={owner:id,name:repomapMembers.displayName(concept),source:source.Text,href:source.Href,open:source.Open,key:key};
        panel.querySelector('[data-concept-name]').textContent=repomapMembers.displayName(concept);
        var declaration=panel.querySelector('[data-concept-declaration]'),text=rmDeclarationText(node,concept);
        declaration.textContent=text;declaration.hidden=!text||text===concept.name;
        var fields=panel.querySelector('[data-concept-fields]');fields.replaceChildren();fields.hidden=!concept.fields?.length;
        (concept.fields||[]).forEach(function(field,at){if(at)fields.appendChild(document.createTextNode(' · '));var link=repomapMembers.sourceLink(field.source);link.textContent=field.name;fields.appendChild(link);});
        var explanation=panel.querySelector('[data-concept-explanation]');
        explanation.textContent=concept.explanation||'';explanation.hidden=!concept.explanation;
        explanation.dataset.displayRef=concept.explanation?concept.explanation_ref||'':'';
        panel.querySelector('[data-concept-source]').replaceChildren(rmOpenCode(source));
        panel.querySelector('[data-concept-dispatch]').replaceChildren(rmSiteReading(map,node,key));
        panel.querySelector('[data-concept-relations]').replaceChildren(rmDeclarationRelations(map,node,key,nodes));
        card.querySelectorAll('.map-member-name').forEach(function(name){if(name.dataset.memberSource===key)name.setAttribute('aria-current','true');});
        if(!map.readingRestoring&&!inspectionPending)content.scrollTop+=panel.getBoundingClientRect().top-content.getBoundingClientRect().top;
        map.dispatchEvent(new Event('repomap:reading'));remember();
      };
      // A declaration of a prepared part is read by itself: the part's
      // reading steps aside, and the heading names its kind and the part,
      // a link up to it.
      function readDeclaration(index){
        card.querySelectorAll('.map-reading-name[aria-current]').forEach(function(name){name.removeAttribute('aria-current');});
        if(index<0){
          map.explorerMember=null;declHolder.hidden=true;declHolder.replaceChildren();partView.hidden=false;
          rmHeadingKind(kindHeading,kindWord);rmHeadingUp(map,kindHeading,map.parentFrame(node));
          map.dispatchEvent(new Event('repomap:reading'));return;
        }
        var concept=concepts[index],source=concept.source,key=repomapMembers.sourceKey(source);
        map.explorerMember={owner:id,name:repomapMembers.displayName(concept),source:source.Text,href:source.Href,open:source.Open,key:key};
        var view=rmDeclView(map.readingContext(),node,reading,concept);
        view.appendChild(rmSiteReading(map,node,key));
        declHolder.replaceChildren(view);declHolder.hidden=false;partView.hidden=true;
        var kind=(reading.decls.find(function(decl){return decl.key===key;})||{}).kind;
        rmHeadingKind(kindHeading,rmT(({function:'Function',type:'Type',variable:'Variable'})[kind]||'Declaration'));
        rmHeadingUp(map,kindHeading,node,true);
        partView.querySelectorAll('.map-reading-name').forEach(function(name){if(name.dataset.declKey===key)name.setAttribute('aria-current','true');});
        if(!map.readingRestoring&&!inspectionPending)content.scrollTop=0;
        else if(!map.readingRestoring&&key!==savedConcept)freshDeclaration=true;
        map.dispatchEvent(new Event('repomap:reading'));remember();
      }
      if(concepts.length&&!reading){
        var list=document.createElement('section');list.className='map-all-members';
        // The part's composition first, then its code: "Made of 18
        // functions", the files once beside it (owner's 3a).
        var made=repomapMembers.composition(node),label=document.createElement('h5');label.className='map-made-of';
        label.appendChild(rmEl('span','',rmT('Made of {0}',made.counts)));if(made.files.length)label.appendChild(rmEl('small','',made.files.join(', ')));list.appendChild(label);
        list.appendChild(repomapMembers.grid(map,node));card.appendChild(list);
        if(saved?.concept){var selected=concepts.findIndex(function(c){return repomapMembers.sourceKey(c.source)===saved.concept;});map.inspectConcept(selected);}
      }
      if(reading&&saved?.concept){var chosen=concepts.findIndex(function(c){return repomapMembers.sourceKey(c.source)===saved.concept;});if(chosen>=0)readDeclaration(chosen);}
      var actions=document.createElement('div');actions.className='map-card-actions';card.querySelector('.map-card-intro').appendChild(actions);
      if(map.exploreNode && !node.dataset.activation){
        if(!map.hasAttribute('data-system-map')&&map.explorerScope!==id){var explore=document.createElement('button');explore.type='button';explore.textContent=node.dataset.branch?rmT('Explore these parts'):rmT('Explore connections');explore.addEventListener('click',function(){map.exploreNode(id);});actions.appendChild(explore);}
        var href=node.getAttribute('href')||'';
        var destination=href.charAt(0)==='#'&&document.getElementById(href.slice(1));
        if(destination&&node.dataset.branch&&href!=='#'+id&&destination!==map.closest('[data-report-page]')){var detail=document.createElement('a');detail.href=href;detail.textContent=rmT('Open component');detail.className='map-details-link';actions.appendChild(detail);}
        var users=map.operationChoices(id);
        if(users.length){var usage=document.createElement('details'),summary=document.createElement('summary');usage.className='map-related-operations';summary.textContent=rmT('Related operations for {0}',titleOf(node));usage.appendChild(summary);
          users.forEach(function(op){var b=document.createElement('button');b.type='button';b.textContent=op.dataset.title;
            if(users.some(function(other){return other!==op&&other.dataset.title===op.dataset.title;})&&op.dataset.sourceText)b.textContent+=' · '+op.dataset.sourceText;
            b.addEventListener('click',function(){
            var concept=concepts.find(function(c){return repomapMembers.sourceKey(c.source)===map.explorerMember?.key;});
            map.chooseOperation(op.id,{node:node,label:concept?rmT('{0} in {1}',repomapMembers.displayName(concept),titleOf(node)):titleOf(node),source:concept&&{href:concept.source.Href,open:concept.source.Open,key:repomapMembers.sourceKey(concept.source)}});
          });usage.appendChild(b);});actions.prepend(usage);}
      }
      var evidence=card.querySelector('.map-card-evidence');
      if(node.dataset.activation&&!witness?.length&&step<0&&!keys.length&&!arrows.length)evidence.remove();
      else actions.appendChild(evidence);
      if(!actions.childElementCount)actions.remove();
      requestAnimationFrame(function(){if(inspectionRevision===ticket&&!card.hidden){
        // A reading opened at one of its sections (an arrow end's connection,
        // an input's path) starts there, that section open.
        var anchor=card.querySelector('[data-reading-anchor]');
        if(saved&&!freshDeclaration)rmRestoreFolds(card,saved.expanded);
        if(anchor?.matches('details'))anchor.open=true;
        content.scrollTop=freshDeclaration?0:saved?.scroll||0;
        // A declaration newly chosen with its part is read from its own
        // reading; one the reader returns to keeps the place they left.
        // An arrow end opened from the canvas opens that connection alone.
        if(anchor&&!restoring)card.querySelectorAll('.map-frame-connections>details[open]').forEach(function(detail){if(detail!==anchor)detail.open=false;});
        var chosen=card.querySelector('.map-concepts:not([hidden])');
        if(chosen&&(!saved||!restoring&&map.explorerMember?.key!==saved.concept))content.scrollTop+=chosen.getBoundingClientRect().top-content.getBoundingClientRect().top;
        if(anchor&&!restoring){content.scrollTop+=anchor.getBoundingClientRect().top-content.getBoundingClientRect().top-8;anchor.removeAttribute('data-reading-anchor');}
        inspectionPending=false;rmExpandAllWord(card);
      }});
    }
    map.explainSource=function(source){
      if(!inspectedNode)return;
      var concepts=map.exploreNode?repomapMembers.items(inspectedNode):rmPage.data(inspectedNode,'concepts')||[];
      var index=concepts.findIndex(function(c){return repomapMembers.sourceKey(c.source)===(source.key||source.href||source.open);});
      if(index>=0)map.inspectConcept(index);
    };
    map.showAllMembers=function(node){map.showNode(node);var list=card.querySelector('.map-all-members');if(list){list.open=true;list.scrollIntoView({block:'nearest'});}};
    map.showNode=function(node){if(map.exploreNode||!repomapPreview.showFor(node)){show(node);card.hidden=false;map.querySelector('.map-inspector').classList.add('has-preview');map.dispatchEvent(new CustomEvent('repomap:inspect',{detail:{node:node,card:card}}));var foot=homeLinks(),page=card.querySelector(':scope>.map-component-page');if(foot&&page&&page.firstChild){foot.prepend(page.firstChild,document.createTextNode(' · '));page.remove();}if(foot)card.appendChild(foot);}};
    // The home's text pages stay one small line away at the foot of every
    // reading (owner, 2026-09-29: "How do I run it?" and the glossary had
    // gone from the column once a component was read, and only the home
    // icon brought them back); a component's whole page leads that line.
    var homeNav=map.hasAttribute('data-system-map')&&document.querySelector('template[data-system-reading-home]')?.content.querySelector('nav');
    function homeLinks(){
      if(!homeNav)return null;
      var line=document.createElement('p');line.className='map-reading-home-links';
      Array.from(homeNav.querySelectorAll('a[href]')).forEach(function(link,i){
        if(i)line.appendChild(document.createTextNode(' · '));
        var a=document.createElement('a');a.href=link.getAttribute('href');a.textContent=link.textContent.replace(/\s*→\s*$/,'');line.appendChild(a);
      });
      return line.childElementCount?line:null;
    }
    map.showMember=function(node,item){
      map.showNode(node);map.explainSource({href:item.source.Href,open:item.source.Open,key:repomapMembers.sourceKey(item.source)});
      var frame=inspector.getBoundingClientRect();
      if(frame.bottom>window.innerHeight||frame.top<0)inspector.scrollIntoView({block:'nearest'});
    };
    map.clearInspection=function(){inspectionRevision++;inspectionPending=false;card.hidden=true;heading.replaceChildren();content.scrollTop=0;inspectedNode=null;map.explorerMember=null;map.querySelector('.map-inspector').classList.remove('has-preview');map.dispatchEvent(new Event('repomap:reading'));};
    function connectionReading(edge,title,body){
      title.textContent=titleOf(byId[edge.from])+' → '+titleOf(byId[edge.to]);
      var kind=document.createElement('p');kind.textContent=edge.possible?rmT('Interpreted connection or possible dispatch'):rmT('Code connections');body.appendChild(kind);
      edge.relations.forEach(function(relation){var row=document.createElement('p');
        [relation.from,relation.to].forEach(function(id,i){if(i)row.appendChild(document.createTextNode(' → '+relation.label+' → '));var n=byId[id];if(!n)return;var a=document.createElement('button');a.type='button';a.textContent=titleOf(n);a.addEventListener('click',function(){if(map.revealNode)map.revealNode(n,false);else if(map.exploreNode)map.exploreNode(id);else n.click();});row.appendChild(a);});body.appendChild(row);
        if(relation.summary){var summary=document.createElement('p');summary.textContent=relation.summary;summary.dataset.displayRef=relation.summaryRef||'';body.appendChild(summary);}
        var evidenceKind=document.createElement('span');evidenceKind.className='map-card-meta';evidenceKind.textContent=relation.possible?rmT('Interpreted connection or possible dispatch'):rmT('Code connections');row.appendChild(evidenceKind);
        [['fromSource','fromText','fromNoSource'],['toSource','toText','toNoSource']].forEach(function(fields){if(!relation[fields[0]]&&!relation[fields[2]])return;var link=repomapMembers.sourceLink({Href:relation[fields[0]],Text:relation[fields[1]],NoSource:relation[fields[2]]});link.className='map-details-link';body.appendChild(link);});
      });
    }
    map.addEventListener('repomap:connection',function(event){
      remember();inspectionRevision++;map.clearMapPreview?.();content.scrollTop=0;card.classList.add('map-card-connection');card.replaceChildren();
      var title=document.createElement('b');heading.replaceChildren(title);heading.classList.remove('has-concepts');connectionReading(event.detail,title,card);
      var foot=homeLinks();if(foot)card.appendChild(foot);
      card.hidden=false;map.querySelector('.map-inspector').classList.add('has-preview');
    });
    for (var n = 0; n < nodes.length; n++) {
      // The native title remains in the static HTML for readers without JS.
      var title = nodes[n].querySelector('title');
      if(map.hasAttribute('data-system-map')){
        if(title)title.remove();
      }else if(map.hasAttribute('data-map-explorer')){
        // Hover may emphasize neighbours and answers "what is this" in a
        // readable card beside the pointer; only an explicit choice changes
        // the reading panel or the canvas. The native title tooltip, which
        // cannot be read at leisure or selected, is replaced by that card.
        if(title)title.remove();
        (function(node){
          repomapPreview.bind(node,hoverCard,function(){
            var kind=node.dataset.activation||(node.dataset.branch==='area'?'Area':node.dataset.branch==='component'?'Component':node.dataset.branch==='components'?'Connected components':'Part');
            var label=document.createElement('span');label.className='map-card-kind';label.textContent=rmT(kind);
            var name=document.createElement('b');name.textContent=node.dataset.title||'';
            var summary=document.createElement('p');summary.textContent=node.dataset.summary||'';
            hoverCard.replaceChildren(label,name,summary);
          });
        })(nodes[n]);
      }else{
        if(title)title.remove();
        (function (node) { repomapPreview.bind(node, card, function () { show(node); }); })(nodes[n]);
      }
      nodes[n].addEventListener('dragstart',function(event){event.preventDefault();});
    }

    // A group card links back to its node on the map.
    for (var k = 0; k < nodes.length; k++) {
      if(nodes[k].dataset.remote==='true')continue;
      var href = nodes[k].getAttribute('href') || '';
      if (href.charAt(0) !== '#') continue;
      var group = document.getElementById(href.slice(1));
      var head = group && group.matches('.group') && group.querySelector('.group-head');
      if (!head || head.querySelector('.on-map')) continue;
      var link = document.createElement('a');
      link.className = 'on-map';
      link.href = '#' + nodes[k].getAttribute('data-node');
      link.textContent = rmT('on the map');
      link.addEventListener('click', (function (node) {
        return function (event) {
          event.preventDefault();
          if(map.revealNode){map.revealNode(node);return;}
          node.scrollIntoView({ block: 'center', inline: 'center' });
          node.classList.remove('map-flash');
          void node.getBoundingClientRect();
          node.classList.add('map-flash');
          setTimeout(function () { node.classList.remove('map-flash'); }, 1600);
        };
      })(nodes[k]));
      head.appendChild(link);
    }

    // The main path through the target. Pointing at a node on it lights the
    // whole path and its arrows, so "how does a request go through?" is
    // answered on the map; the card says which step this is.
    var traceIds = (map.getAttribute('data-trace') || '').split(/\s+/).filter(Boolean);
    var trace = [];
    for (var t = 0; t < traceIds.length; t++) {
      var traced = map.querySelector('[data-node][href="#' + traceIds[t] + '"]');
      if (traced) trace.push(traced);
    }
    // A step is counted along the whole path, drawn or left to the cards
    // below, so "step 2 of 5" is true of the path and not of the picture.
    function traceIndex(node) {
      var href = node.getAttribute('href') || '';
      for (var i = 0; i < traceIds.length; i++) if ('#' + traceIds[i] === href) return i;
      return -1;
    }
    function lightTrace() {
      var on = {};
      for (var i = 0; i < trace.length; i++) { on[trace[i].getAttribute('data-node')] = true; trace[i].classList.add('map-near', 'map-traced'); }
      var lines = map.querySelectorAll('.map-edge, .map-edge-label');
      for (var j = 0; j < lines.length; j++) {
        if (on[lines[j].getAttribute('data-from')] && on[lines[j].getAttribute('data-to')]) lines[j].classList.add('map-near');
      }
      map.classList.add('map-previewing');
    }
    function unlightTrace() {
      for (var i = 0; i < trace.length; i++) trace[i].classList.remove('map-traced');
    }
    for (var u = 0; u < trace.length; u++) {
      trace[u].addEventListener('repomap:preview', lightTrace);
      trace[u].addEventListener('focus', lightTrace);
      trace[u].addEventListener('repomap:previewend', unlightTrace);
      trace[u].addEventListener('blur', unlightTrace);
    }
    map.traceIndex = traceIndex;
    map.traceLength = traceIds.length;
  }
})();

// An opened block of a scrolling column is brought into view: the column
// scrolls by what the block overflows at its foot, and never further than
// bringing the block's own top (its summary) to the column's top.
function rmRevealOpened(content,block,room){
  room=room===undefined?8:room;
  var box=content.getBoundingClientRect(),at=block.getBoundingClientRect();
  var shift=Math.min(at.bottom-(box.bottom-room),at.top-(box.top+room));
  if(shift>0)content.scrollTop+=shift;
}
// An evidence list opens every folded line at once, and closes them again;
// each fold still opens alone, and without scripting. The button says what
// it will do next, also after folds are opened by hand.
function rmOpenAllWord(list){
  var folds=list.querySelectorAll(':scope>.conn-fold');
  return Array.prototype.every.call(folds,function(fold){return fold.open;})?'Close all':'Open all';
}
(function(){
  document.querySelectorAll('[data-open-all]').forEach(function(button){button.hidden=false;});
  document.addEventListener('click',function(event){
    var button=event.target.closest?.('[data-open-all]');if(!button)return;
    var list=button.closest('.connection-evidence');if(!list)return;
    var open=rmOpenAllWord(list)==='Open all';
    list.querySelectorAll(':scope>.conn-fold').forEach(function(fold){fold.open=open;});
    button.textContent=rmT(rmOpenAllWord(list));
  });
  document.addEventListener('toggle',function(event){
    var fold=event.target;if(!fold.matches?.('.conn-fold'))return;
    var list=fold.parentElement,button=list&&list.querySelector(':scope>[data-open-all]');
    if(button)button.textContent=rmT(rmOpenAllWord(list));
  },true);
})();

// A reading's folds, each known by its summary's words and how many folds
// with the same words stand before it: the same fold whatever else the
// reading shows (a declaration read in its part adds folds above others).
function rmFolds(card){return Array.from(card.querySelectorAll('details')).filter(function(detail){return !detail.closest('[hidden]');});}
function rmFoldKeys(card){
  var seen={};
  return Array.from(card.querySelectorAll('details')).map(function(detail){
    var words=(detail.querySelector(':scope>summary')?.textContent||'').trim(),n=seen[words]=(seen[words]||0)+1;
    return {detail:detail,key:words+'#'+n};
  });
}
function rmOpenFolds(card){return rmFoldKeys(card).filter(function(fold){return fold.detail.open;}).map(function(fold){return fold.key;});}
function rmRestoreFolds(card,keys){var open=new Set(keys||[]);rmFoldKeys(card).forEach(function(fold){fold.detail.open=open.has(fold.key);});}
// "Expand all" opens every fold of the reading; with all open it closes them.
function rmExpandAllWord(card){
  var button=card.closest('.map-inspector')?.querySelector('[data-expand-all]');if(!button)return;
  var folds=rmFolds(card),open=folds.length>0&&folds.every(function(detail){return detail.open;});
  button.hidden=!folds.length;button.dataset.state=open?'open':'';button.textContent=rmT(open?'Collapse all':'Expand all');
}
