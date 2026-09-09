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
    stage.before(workspace); workspace.appendChild(stage);
    var inspector = document.createElement('aside');
    inspector.className = 'map-inspector';
    inspector.setAttribute('aria-label', rmT('Selected node details'));
    var content = document.createElement('div');
    content.className = 'map-inspector-content';
    content.tabIndex = 0;
    content.setAttribute('role', 'region');
    content.setAttribute('aria-label', rmT('Node description and sources'));
    var hint = document.createElement('p');
    hint.className = 'map-inspector-hint';
    hint.textContent = rmT(map.hasAttribute('data-map-explorer')?'Click a part or code element to keep its explanation here.':'Hover or focus a node to read about it.');
    content.appendChild(hint); inspector.appendChild(content); workspace.appendChild(inspector);
    var continuation = document.createElement('div');
    continuation.className = 'map-inspector-continuation';
    var more = document.createElement('button');
    more.type = 'button'; more.hidden = true;
    continuation.appendChild(more); inspector.appendChild(continuation);
    function updateContinuation() {
      var overflow = content.scrollHeight > content.clientHeight + 1;
      var below = overflow && content.scrollTop + content.clientHeight < content.scrollHeight - 1;
      more.hidden = !overflow;
      more.textContent = below ? rmT('More details ↓') : rmT('↑ Back to top');
      continuation.classList.toggle('has-more', below);
    }
    more.addEventListener('click', function () {
      var below = content.scrollTop + content.clientHeight < content.scrollHeight - 1;
      content.scrollTo({top: below ? content.scrollTop + content.clientHeight * .8 : 0});
    });
    content.addEventListener('scroll', updateContinuation);
    new ResizeObserver(updateContinuation).observe(content);
    new MutationObserver(function () { requestAnimationFrame(updateContinuation); })
      .observe(content, {childList:true, subtree:true, attributes:true, attributeFilter:['hidden','open']});
    var nodes = map.querySelectorAll('[data-node]');
    var edges = map.querySelectorAll('.map-edge, .map-edge-label');
    for (var index = 0; index < nodes.length; index++) {
      var node = nodes[index];
      node.addEventListener('repomap:preview', preview);
      node.addEventListener('repomap:previewend', clear);
      node.addEventListener('click', select);
    }

    function preview(event) {
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
      if(map.classList.contains('map-part-focus'))return;
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
        return;
      }
      stage.scrollTo(Math.max(0,(box.x+box.w/2)*scale-stage.clientWidth/2),
        Math.max(0,(box.y+box.h/2)*scale-stage.clientHeight/2));
    }
    // A report opened at a question may initially hide this map. Frame it once
    // when it becomes visible; later resizing must preserve the reader's pan.
    var resize = new ResizeObserver(function () { updatePan(); focusOpened(); });
    resize.observe(stage);
    resize.observe(svg);

    function readable() {
      scale = readableScale;
      apply();
    }

    function apply() {
      svg.style.width = baseWidth * scale + 'px';
      svg.style.maxWidth = 'none';
      svg.style.minWidth = '0';
      updatePan();
    }
    function updatePan() {
      var canPan = !map.classList.contains('map-part-focus') && stage.clientWidth > 0 && stage.clientHeight > 0 &&
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
    var viewportFrame=0;
    stage.addEventListener('scroll',function(){
      if(viewportFrame)return;
      viewportFrame=requestAnimationFrame(function(){viewportFrame=0;map.dispatchEvent(new Event('repomap:viewport'));});
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
      } else if (button.hasAttribute('data-map-reset')) {
        readable();
        pendingFocus=true;focusOpened();
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
    new ResizeObserver(function () { content.dispatchEvent(new Event('scroll')); }).observe(card);

    function sentences(id) {
      edges = map.querySelectorAll('.map-edge');
      var out = [];
      for (var e = 0; e < edges.length; e++) {
        var edge = edges[e];
        var label = (edge.querySelector('title') || {}).textContent || '';
        var from = edge.getAttribute('data-from'), to = edge.getAttribute('data-to');
        if (from === id && byId[to]) out.push('\u2192 ' + (label ? label + ' \u00b7 ' : '') + titleOf(byId[to]));
        else if (to === id && byId[from]) out.push('\u2190 ' + titleOf(byId[from]) + (label ? ' \u00b7 ' + label : ''));
        if (out.length === 4) break;
      }
      return out;
    }
    var inspectedNode=null, inspectionRevision=0, remembered=new Map();
    function remember(){
      if(!inspectedNode||card.classList.contains('map-card-connection'))return;
      var picker=heading.querySelector('[data-concept-picker]');
      remembered.set(inspectedNode,{concept:picker?.value,scroll:content.scrollTop});
    }
    content.addEventListener('scroll',remember);
    function show(node) {
      remember();inspectedNode=node;
      map.explorerMember=null;
      var saved=remembered.get(node), ticket=++inspectionRevision;
      content.scrollTop = 0;
      card.classList.remove('map-card-connection');
      var id = node.getAttribute('data-node');
      var counts = node.dataset.branch ? rmT('{0} parts',(node.dataset.children||'').split(/\s+/).filter(Boolean).length) : node.dataset.activation ? rmT(node.dataset.activation) : node.dataset.members ? rmT('{0} symbols',Number(node.dataset.members)) : '';
      var html = '<div class="map-card-intro">';
      if(map.exploreNode){
        var current=node.dataset.activation?map.explorerOperation===node&&map.dataset.operationPinned==='true':map.explorerScope===id;
        html+='<span class="map-card-kind">'+(current?'':rmT('Preview')+' · ')+(node.dataset.activation?rmT('Operation')+(counts?' · '+escapeText(counts):''):node.dataset.branch==='component'?rmT('Component'):node.dataset.branch?rmT('Area'):rmT('Part'))+'</span>';
      }
      html += '<b>' + escapeText(titleOf(node)) + '</b>';
      var summary = node.getAttribute('data-summary');
      if (summary) html += '<p class="map-card-summary" data-display-ref="'+escapeText(node.dataset.summaryRef)+'">' + escapeText(summary) + '</p>';
      if (node.dataset.operationGroup) html += '<span class="map-card-meta">' + escapeText(node.dataset.operationGroup) + '</span>';
      var source=node.getAttribute('data-source');
      if(source) html += '<p><a target="_blank" rel="noopener" href="'+escapeText(source)+'">'+escapeText(node.getAttribute('data-source-text')||rmT('Source'))+'</a></p>';
      else if(node.dataset.open) html += '<p><a href="#" data-open="'+escapeText(node.dataset.open)+'">'+escapeText(node.dataset.sourceText||rmT('Source'))+'</a></p>';
      else if(node.dataset.noSource==='true') html += '<p><span title="'+escapeText(rmT('No source'))+'">'+escapeText(node.dataset.sourceText||rmT('Source'))+'</span></p>';
      html += '</div>';
      var concepts = map.exploreNode ? repomapMembers.items(node) : JSON.parse(node.dataset.concepts || '[]');
      card.classList.toggle('map-card-has-concepts', concepts.length > 0);
      if (concepts.length) {
        html += ("<div class=\"map-concepts\" hidden><label><span>"+rmT.html("Code element")+"</span> <select data-concept-picker aria-label=\""+rmT.html("Code element to explain")+"\"><option value=\"\">"+rmT.html("Choose a code element")+"</option>");
        concepts.forEach(function (concept, i) {
          var repeated=concepts.some(function(other,j){return j!==i&&other.name===concept.name;});
          html += '<option value="'+i+'">'+escapeText(repomapMembers.displayName(concept)+(repeated?' · '+concept.source.Text:''))+'</option>';
        });
        html += '</select></label><p data-concept-explanation></p><div class="map-concept-source"><span data-concept-source></span><span class="map-concept-provenance" role="img" title="'+rmT.html('Model explanation')+'" aria-label="'+rmT.html('Model explanation')+'">ⓘ</span></div></div>';
      }
      html += ("<details class=\"map-card-evidence\"><summary>"+rmT.html("Code and connections")+"</summary>");
      if (counts && !node.dataset.activation) html += '<span class="map-card-meta">' + escapeText(counts) + '</span>';
      if(map.areaDescriptions){
        var descriptions=Array.from(new Set(map.areaDescriptions(node))).filter(function(text){return text&&text!==summary;});
        if(descriptions.length){
          html+=("<details><summary>"+rmT.html("Additional model description")+"</summary>");
          descriptions.forEach(function(text){html+='<p>'+escapeText(text)+'</p>';});html+='</details>';
        }
      }
      var operation = map.inspectedOperation;
      var witness = operation && !node.dataset.activation && JSON.parse(operation.dataset.callPaths || '{}')[id];
      if (witness && witness.length) {
        html += '<details class="call-path"><summary>'+rmT.html('Why it appears in {0}',operation.dataset.title)+'</summary><p>'+rmT.html('One shortest static call path:')+'</p><ol>';
        witness.forEach(function (step) {
          html += '<li>'+(step.possible?("<span class=\"possible\">"+rmT.html("possible call")+"</span> "):'')+'<strong>'+escapeText(step.name)+'</strong><br>';
          if (step.href) html += '<a target="_blank" rel="noopener" href="'+escapeText(step.href)+'">'+escapeText(step.source)+'</a>';
          else if (step.open) html += '<a href="#" data-open="'+escapeText(step.open)+'">'+escapeText(step.source)+'</a>';
          else if(step.no_source) html += '<span title="'+escapeText(rmT('No source'))+'">'+escapeText(step.source)+'</span>';
          else html += escapeText(step.source);
          html += '</li>';
        });
        html += '</ol></details>';
      }
      var step = map.traceIndex ? map.traceIndex(node) : -1;
      if (step >= 0) html += '<span class="map-card-meta">'+rmT.html('step {0} of {1} on the main path',step+1,map.traceLength)+'</span>';
      var keys = (node.getAttribute('data-keys') || '').split(' | ').filter(Boolean);
      if(map.exploreNode&&concepts.length&&keys.every(function(key){var name=key.split(' — ')[0];return concepts.filter(function(concept){return concept.name===name;}).length===1;}))keys=[];
      keys=keys.map(escapeText);
      if (keys.length) html += '<ul class="map-card-keys">' + keys.map(function (k) { return '<li><code>' + k.replace(/ — .*$/, '') + '</code>' + (k.indexOf(' — ') > 0 ? ' — ' + k.slice(k.indexOf(' — ') + 3) : '') + '</li>'; }).join('') + '</ul>';
      var arrows = map.classList.contains('repo-map') || map.classList.contains('map-part-focus') || witness ? [] : sentences(id);
      if (arrows.length) html += '<ul>' + arrows.map(function (a) { return '<li>' + escapeText(a) + '</li>'; }).join('') + '</ul>';
      html += '<span class="map-card-hint">'+(node.getAttribute('data-activation')?rmT('Click to keep this operation selected. Open code using the source link.'):rmT('click — explore'))+'</span>';
      card.innerHTML = html+'</details>';
      // Keep the current object and term above the scrolling evidence. This
      // is a reserved row of the inspector, never an overlay on map controls.
      heading.replaceChildren();
      var objectHeading=document.createElement('div'),kindHeading=card.querySelector('.map-card-kind');
      if(kindHeading)objectHeading.appendChild(kindHeading);
      objectHeading.appendChild(card.querySelector('.map-card-intro>b'));heading.appendChild(objectHeading);
      heading.classList.toggle('has-concepts',concepts.length>0);
      if (concepts.length) {
        heading.appendChild(card.querySelector('.map-concepts label'));
        var picker=heading.querySelector('[data-concept-picker]');
        var backToPart=document.createElement('button');backToPart.type='button';backToPart.className='map-concept-back';
        backToPart.textContent=rmT('← Back to part');backToPart.setAttribute('aria-label',rmT('← Back to {0}',titleOf(node)));
        heading.prepend(backToPart);
        backToPart.addEventListener('click',function(){
          var selectedKey=map.explorerMember?.key;
          picker.value='';picker.dispatchEvent(new Event('change'));
          var part=map.querySelector('.map-focus-center');
          var member=part&&Array.from(part.querySelectorAll('[data-member-source]')).find(function(b){return b.dataset.memberSource===selectedKey;});
          if(member){member.focus({preventScroll:true});member.scrollIntoView({block:'nearest'});}else picker.focus({preventScroll:true});
        });
        if(saved?.concept!==undefined)picker.value=saved.concept;
        var explain=function(){
          var panel=card.querySelector('.map-concepts');
          panel.hidden=picker.value==='';card.classList.toggle('map-card-has-concepts',!panel.hidden);
          backToPart.hidden=panel.hidden||!map.classList.contains('map-part-focus');
          if(panel.hidden){map.explorerMember=null;map.querySelectorAll('[data-member-source]').forEach(function(b){b.setAttribute('aria-pressed','false');});map.dispatchEvent(new Event('repomap:reading'));return;}
          var concept=concepts[Number(picker.value)],source=concept.source;
          map.explorerMember={owner:id,name:picker.selectedOptions[0].textContent,source:source.Text,href:source.Href,open:source.Open,key:repomapMembers.sourceKey(source)};
          map.dispatchEvent(new Event('repomap:reading'));
          map.querySelectorAll('[data-member-source]').forEach(function(b){b.setAttribute('aria-pressed',b.dataset.memberSource===repomapMembers.sourceKey(source));});
          var explanation=card.querySelector('[data-concept-explanation]');
          explanation.textContent=concept.explanation||rmT('No explanation saved. Open the source to inspect this element.');
          explanation.dataset.displayRef=concept.explanation?concept.explanation_ref||'':'';
          card.querySelector('[data-concept-source]').replaceChildren(repomapMembers.sourceLink(source));
        };
        picker.addEventListener('change',function(){inspectionRevision++;explain();content.scrollTop=0;remember();});explain();
      }
      var actions=document.createElement('div');actions.className='map-card-actions';card.querySelector('.map-card-intro').appendChild(actions);
      if(map.exploreNode && !node.dataset.activation){
        if(map.explorerScope!==id){var explore=document.createElement('button');explore.type='button';explore.textContent=node.dataset.branch?rmT('Explore these parts'):rmT('Explore connections');explore.addEventListener('click',function(){map.exploreNode(id);});actions.appendChild(explore);}
        var href=node.getAttribute('href')||'';
        if(href.charAt(0)==='#'&&href!=='#'+id){var detail=document.createElement('a');detail.href=href;detail.textContent=node.dataset.branch?rmT('Open component'):rmT('Code and all group details');detail.className='map-details-link';actions.appendChild(detail);}
        var users=map.operationChoices(id);
        if(users.length){var usage=document.createElement('details'),summary=document.createElement('summary');summary.textContent=rmT('Related operations for {0} · {1}',titleOf(node),users.length);usage.appendChild(summary);
          users.forEach(function(op){var b=document.createElement('button');b.type='button';b.textContent=op.dataset.title;
            if(users.some(function(other){return other!==op&&other.dataset.title===op.dataset.title;})&&op.dataset.sourceText)b.textContent+=' · '+op.dataset.sourceText;
            b.addEventListener('click',function(){
            var picker=heading.querySelector('[data-concept-picker]'),concept=picker&&picker.value!==''&&concepts[Number(picker.value)];
            map.chooseOperation(op.id,{node:node,label:concept?rmT('{0} in {1}',picker.selectedOptions[0].textContent,titleOf(node)):titleOf(node),source:concept&&{href:concept.source.Href,open:concept.source.Open,key:repomapMembers.sourceKey(concept.source)}});
          });usage.appendChild(b);});actions.prepend(usage);}
      }
      var evidence=card.querySelector('.map-card-evidence');
      if(node.dataset.activation&&!witness?.length&&step<0&&!keys.length&&!arrows.length)evidence.remove();
      else actions.appendChild(evidence);
      if(!actions.childElementCount)actions.remove();
      requestAnimationFrame(function(){if(inspectionRevision===ticket&&!card.hidden)content.scrollTop=saved?.scroll||0;});
    }
    map.explainSource=function(source){
      if(!inspectedNode)return;
      var concepts=map.exploreNode?repomapMembers.items(inspectedNode):JSON.parse(inspectedNode.dataset.concepts||'[]');
      var index=concepts.findIndex(function(c){return repomapMembers.sourceKey(c.source)===(source.key||source.href||source.open);});
      var picker=heading.querySelector('[data-concept-picker]');
      if(index<0||!picker)return;
      picker.value=String(index);picker.dispatchEvent(new Event('change'));
    };
    map.showNode=function(node){if(map.exploreNode||!repomapPreview.showFor(node)){show(node);card.hidden=false;map.querySelector('.map-inspector').classList.add('has-preview');}};
    map.showMember=function(node,item){
      map.showNode(node);map.explainSource({href:item.source.Href,open:item.source.Open,key:repomapMembers.sourceKey(item.source)});
      var frame=inspector.getBoundingClientRect();
      if(frame.bottom>window.innerHeight||frame.top<0)inspector.scrollIntoView({block:'nearest'});
    };
    map.clearInspection=function(){card.hidden=true;map.explorerMember=null;map.querySelector('.map-inspector').classList.remove('has-preview');map.dispatchEvent(new Event('repomap:reading'));};
    map.addEventListener('repomap:connection',function(event){
      remember();inspectionRevision++;
      content.scrollTop = 0;
      card.classList.add('map-card-connection');
      var edge=event.detail;card.replaceChildren();var title=document.createElement('b');title.textContent=titleOf(byId[edge.from])+' → '+titleOf(byId[edge.to]);heading.replaceChildren(title);heading.classList.remove('has-concepts');
      var kind=document.createElement('p');kind.textContent=edge.possible?rmT('Interpreted connection or possible dispatch'):rmT('Code connections');card.appendChild(kind);
      edge.relations.forEach(function(relation){var row=document.createElement('p');
        [relation.from,relation.to].forEach(function(id,i){if(i)row.appendChild(document.createTextNode(' → '+relation.label+' → '));var n=byId[id];if(!n)return;var a=document.createElement('button');a.type='button';a.textContent=titleOf(n);a.addEventListener('click',function(){if(map.exploreNode)map.exploreNode(id);else n.click();});row.appendChild(a);});card.appendChild(row);
        if(relation.summary){var summary=document.createElement('p');summary.textContent=relation.summary;summary.dataset.displayRef=relation.summaryRef||'';card.appendChild(summary);}
        [['fromSource','fromText'],['toSource','toText']].forEach(function(fields){if(!relation[fields[0]])return;var a=document.createElement('a');a.href=relation[fields[0]];a.textContent=relation[fields[1]];a.target='_blank';a.rel='noopener';a.className='map-details-link';card.appendChild(a);});
      });
      card.hidden=false;map.querySelector('.map-inspector').classList.add('has-preview');
    });
    for (var n = 0; n < nodes.length; n++) {
      // The native title remains in the static HTML for readers without JS.
      var title = nodes[n].querySelector('title');
      if(map.hasAttribute('data-map-explorer')){
        // Hover may emphasize neighbours; only an explicit choice changes
        // the reading panel or the canvas. Keep the native brief preview.
        (function(node){
          node.addEventListener('mouseenter',function(){node.dispatchEvent(new Event('repomap:preview'));});
          node.addEventListener('mouseleave',function(){node.dispatchEvent(new Event('repomap:previewend'));});
          node.addEventListener('focusin',function(){node.dispatchEvent(new Event('repomap:preview'));});
          node.addEventListener('focusout',function(){node.dispatchEvent(new Event('repomap:previewend'));});
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
