// Floating previews protect a pointer moving toward their contents.
// Docked map details follow the hovered node immediately.
var repomapPreview = (function () {
  var active = null, pointer = {x:0,y:0}, waiting = null, bindings = new WeakMap(), restoringFocus = false;
  function ownerOf(target) {
    for(var node=target;node instanceof Element;node=node.parentElement)if(bindings.has(node))return node;
    return null;
  }
  function stationary(map, point) {
    var held=map && map.previewStationaryPoint;
    if(!held) return false;
    if(Math.hypot(held.x-point.x,held.y-point.y)<2) return true;
    delete map.previewStationaryPoint;
    return false;
  }
  function insideTriangle(p,a,b,c) {
    function cross(p,a,b) { return (p.x-b.x)*(a.y-b.y)-(a.x-b.x)*(p.y-b.y); }
    var x=cross(p,a,b), y=cross(p,b,c), z=cross(p,c,a);
    return !((x<0 || y<0 || z<0) && (x>0 || y>0 || z>0));
  }
  function towardCard(point) {
    point=point || pointer;
    if (!active || active.docked || !active.exit) return false;
    var box=active.card.getBoundingClientRect(), a=active.exit, b,c;
    if(point.x>=box.left && point.x<=box.right && point.y>=box.top && point.y<=box.bottom) return true;
    if (a.x < box.left) { b={x:box.left+5,y:box.top-12}; c={x:box.left+5,y:box.bottom+12}; }
    else if (a.x > box.right) { b={x:box.right-5,y:box.top-12}; c={x:box.right-5,y:box.bottom+12}; }
    else if (a.y < box.top) { b={x:box.left-12,y:box.top+5}; c={x:box.right+12,y:box.top+5}; }
    else if (a.y > box.bottom) { b={x:box.left-12,y:box.bottom-5}; c={x:box.right+12,y:box.bottom-5}; }
    else return false;
    return insideTriangle(point,a,b,c);
  }
  document.addEventListener('pointermove',function(event) {
    var next={x:event.clientX,y:event.clientY};
    var map=event.target.closest('[data-map]'), wasHeld=map && map.previewStationaryPoint;
    if(wasHeld && !stationary(map,next)) {
      // A layout can put a different node under a motionless pointer. Resume
      // inspection only on a real move, including within that same new node.
      var binding=bindings.get(event.target.closest('[data-node]'));
      if(binding) binding.show();
    }
    // Test the previous corridor before moving its apex. Testing after the
    // update would always accept the pointer because it is the new apex.
    var protectedPath=towardCard(next);
    if(active && active.exit) active.exit=protectedPath ? next : null;
    pointer=next;
    if(active && active.trigger.contains(event.target)) active.lastInside=pointer;
    if (waiting && !protectedPath && !active?.pinned) { var show=waiting; waiting=null; show(); }
    if(active && !active.pinned && !active.docked && !protectedPath && !active.trigger.contains(event.target) && !active.card.contains(event.target)) active.hide();
  });
  function bind(trigger,card,prepare,options) {
    options=options||{};
    if (!card.parentNode) { card.hidden=true; document.body.appendChild(card); }
    var inspector=card.closest('.map-inspector');
    var timer, item={card:card,trigger:trigger,docked:!!inspector,pinned:false,exit:null,lastInside:null,hide:hide,place:place};
    function hide(returnFocus) {
      clearTimeout(timer);
      if (active !== item) return;
      waiting=null;
      trigger.classList.remove('preview-active'); card.hidden=true; active=null;
      item.pinned=false;card.classList.remove('preview-pinned');
      if(options.pinOnClick)trigger.setAttribute('aria-expanded','false');
      if (inspector) inspector.classList.remove('has-preview');
      trigger.dispatchEvent(new Event('repomap:previewend'));
      if(returnFocus&&options.returnFocus&&trigger.isConnected){restoringFocus=true;trigger.focus({preventScroll:true});restoringFocus=false;}
    }
    function place() {
      if (inspector) return;
      var box=trigger.getBoundingClientRect();
      var left=box.right+10,top=box.top;
      if (left+card.offsetWidth>window.innerWidth-12) left=box.left-card.offsetWidth-10;
      if (left<12) { left=box.left; top=box.bottom+10; }
      card.style.left=Math.max(12,Math.min(left,window.innerWidth-card.offsetWidth-12))+'px';
      card.style.top=Math.max(12,Math.min(top,window.innerHeight-card.offsetHeight-12))+'px';
    }
    function show(force) {
      if(restoringFocus)return;
      if(active&&active!==item&&!force&&(active.pinned||trigger.contains(active.trigger)))return;
      clearTimeout(timer); waiting=null;
      if(active===item&&options.pinOnClick){place();return;}
      if (active && active!==item) active.hide();
      if (prepare) prepare();
      card.hidden=false; active=item; item.exit=null; item.lastInside=pointer;
      if (inspector) inspector.classList.add('has-preview');
      if(options.pinOnClick)trigger.setAttribute('aria-expanded','true');
      trigger.classList.add('preview-active'); place();
      trigger.dispatchEvent(new Event('repomap:preview'));
    }
    function enter(event) {
      if(ownerOf(event.target)!==trigger)return;
      if(active?.pinned&&active!==item)return;
      // Enter/leave may be synthesized after layout, with rounded positions.
      // Only pointermove resumes a hover after layout, never mouseenter alone.
      if(event.type==='mouseenter' && trigger.closest('[data-map]')?.previewStationaryPoint) return;
      if(event.type==='mouseenter') pointer={x:event.clientX,y:event.clientY};
      if (active && active!==item && event.type==='mouseenter' && towardCard()) {
        waiting=function(){ if(trigger.matches(':hover')) show(); };
      } else show(false);
    }
    function leave(event) {
      if (active!==item||item.pinned) return;
      if (inspector) {
        // The description stays readable, but leaving the node ends its
        // transient graph emphasis. Card lifetime must not pin the arrows.
        trigger.dispatchEvent(new Event('repomap:previewend'));
        return;
      }
      if (event.type==='mouseleave' && event.currentTarget===trigger) {
        item.exit=item.lastInside || {x:event.clientX,y:event.clientY};
      }
      clearTimeout(timer);
      timer=setTimeout(function close() {
        if (active!==item || item.pinned || trigger.matches(':hover,:focus-within') || card.matches(':hover,:focus-within')) return;
        if (towardCard()) return;
        hide();
      },220);
    }
    trigger.addEventListener('mouseenter',enter); trigger.addEventListener('mouseleave',leave);
    trigger.addEventListener('focusin',function(event){if(ownerOf(event.target)===trigger)show(false);}); trigger.addEventListener('focusout',leave);
    trigger.addEventListener('click',function(event){
      if(ownerOf(event.target)!==trigger)return;
      waiting=null;
      if(options.pinOnClick){show(true);item.pinned=true;card.classList.add('preview-pinned');place();if(options.focusOnPin)card.focus({preventScroll:true});}
      else if(event.target.closest('a'))hide();else show(true);
    });
    // A shared map card gets these handlers only once.
    if (!card.dataset.previewBound) {
      card.dataset.previewBound='yes';
      card.addEventListener('mouseenter',function(){waiting=null; if(active) active.exit=null;});
      card.addEventListener('mouseleave',function(){ if(inspector) return; var current=active; setTimeout(function(){if(active===current && current && !current.pinned && !current.trigger.matches(':hover,:focus-within') && !card.matches(':hover,:focus-within')) current.hide();},220); });
      card.addEventListener('focusout',function(){var current=active;setTimeout(function(){if(active===current&&current&&!current.pinned&&!current.docked&&!current.trigger.matches(':focus-within,:hover')&&!card.matches(':focus-within,:hover'))current.hide();},0);});
      card.addEventListener('click',function(event){if(active?.card===card&&event.target.closest('a')&&active.pinned)active.hide();});
    }
    var binding={show:show,hide:hide};bindings.set(trigger,binding);return binding;
  }
  document.addEventListener('keydown',function(e){if(e.key==='Escape' && active){waiting=null;active.hide(true);e.preventDefault();e.stopImmediatePropagation();}},true);
  document.addEventListener('pointerdown',function(e){
    pointer={x:e.clientX,y:e.clientY};
    // Map controls keep the current reading; selection or navigation replaces
    // it explicitly. Clicking another surface still dismisses the preview.
    var inMap=active&&active.docked&&e.target.closest('[data-map]')===active.card.closest('[data-map]');
    if(active&&!inMap&&!e.target.closest('.preview-active,.map-card,.map-inspector,.source-card'))active.hide();
  });
  document.addEventListener('scroll',function(e){if(active && !active.pinned && !active.card.closest('.map-inspector') && !(e.target.closest && e.target.closest('.map-card,.source-card')))active.hide();},true);
  window.addEventListener('resize',function(){if(active&&!active.docked){if(active.pinned)active.place();else active.hide();}});
  return {bind:bind,freeze:function(map){map.previewStationaryPoint={x:pointer.x,y:pointer.y};},showFor:function(trigger){var b=bindings.get(trigger);if(b)b.show();return !!b;}};
})();

(function () {
  document.querySelectorAll('.model').forEach(function (text, index) {
    // Glossary definitions already keep their complete sources beside them.
    if(text.closest('.learn-concept'))return;
    var source = text.querySelector('.model-sources');
    // An empty citation control offers no action. Keep the prose readable and
    // expose actual saved sources only through the dedicated icon.
    if(!source || !source.children.length)return;
    var card = document.createElement('aside');
    card.className = 'source-card';
    card.id = 'model-sources-' + index;
    card.setAttribute('aria-label', rmT('About this model response'));
    var title = document.createElement('strong');
    title.textContent = rmT('Model response');
    card.appendChild(title);
    var note = document.createElement('p');
    note.textContent = rmT('The model cited these sources:');
    card.appendChild(note);
    if (source) {
      var links = source.cloneNode(true);
      links.className = 'source-links';
      card.appendChild(links);
      source.hidden = true;
    }
    var hint = document.createElement('button');
    hint.type = 'button'; hint.className = 'source-hint'; hint.textContent = 'ⓘ';
    hint.setAttribute('aria-label', rmT('About this model response and its sources'));
    hint.setAttribute('aria-controls', card.id);
    var answer = text.classList.contains('answer-copy');
    if (answer) hint.textContent = rmT('Model explanation · sources');
    text.appendChild(hint);
    repomapPreview.bind(hint, card);
    text.classList.add('model-inspectable');
  });
  document.querySelectorAll('.evidence-list').forEach(function (list) {
    var summary = list.querySelector('summary');
    summary.addEventListener('click', function () {
      if (!list.open) return;
      requestAnimationFrame(function () {
        if (summary.getBoundingClientRect().top < 0) summary.scrollIntoView({block:'start'});
        summary.focus({preventScroll:true});
      });
    });
  });
  var picker = document.querySelector('.target-picker');
  if (picker) picker.addEventListener('click', function (event) {
    if (event.target.closest('a')) picker.open = false;
  });
})();
