// The map works with scripting off: every node is a link to the group it
// names. Scripting adds one thing on top — pointing at a node previews its
// own neighbourhood, and nothing else. It never changes the layout, the
// geometry or which page you are on.
(function () {
  var maps = document.querySelectorAll('[data-map]');
  for (var index = 0; index < maps.length; index++) {
    bind(maps[index]);
  }

  function bind(map) {
    var nodes = map.querySelectorAll('[data-node]');
    var edges = map.querySelectorAll('.map-edge, .map-edge-label');
    for (var index = 0; index < nodes.length; index++) {
      var node = nodes[index];
      node.addEventListener('mouseenter', preview);
      node.addEventListener('mouseleave', clear);
      node.addEventListener('focus', preview);
      node.addEventListener('blur', clear);
      node.addEventListener('click', select);
    }

    function preview(event) {
      var node = event.currentTarget;
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
        var incident = near[line.getAttribute('data-from')] && near[line.getAttribute('data-to')];
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

// Map controls. The picture is complete and legible before any of this runs:
// it fits its frame, it scrolls, and every box is a link. Scripting only adds
// what a static picture cannot do — moving closer and dragging around.
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
    var baseWidth = svg.getBoundingClientRect().width;
    if (!baseWidth) return;
    controls.hidden = false;
    var scale = 1;

    function apply() {
      svg.style.width = baseWidth * scale + 'px';
      svg.style.maxWidth = 'none';
      svg.style.minWidth = '0';
      stage.classList.toggle('map-zoomed', scale > 1);
    }
    function zoom(factor) {
      scale = Math.min(4, Math.max(0.4, scale * factor));
      apply();
    }
    controls.addEventListener('click', function (event) {
      var button = event.target.closest('button');
      if (!button) return;
      if (button.hasAttribute('data-map-zoom')) {
        zoom(parseFloat(button.getAttribute('data-map-zoom')));
      } else if (button.hasAttribute('data-map-fit')) {
        scale = stage.clientWidth / baseWidth;
        apply();
      } else if (button.hasAttribute('data-map-reset')) {
        scale = 1;
        svg.style.width = '';
        svg.style.maxWidth = '';
        svg.style.minWidth = '';
        stage.classList.remove('map-zoomed');
        stage.scrollTo(0, 0);
      }
    });

    // Dragging pans the stage. It never moves a node: the layout is computed
    // once, in Go, and the picture a reader sees is the picture that was laid
    // out.
    var dragging = false, startX = 0, startY = 0, leftAt = 0, topAt = 0;
    stage.addEventListener('pointerdown', function (event) {
      if (event.target.closest('a')) return;
      dragging = true;
      startX = event.clientX; startY = event.clientY;
      leftAt = stage.scrollLeft; topAt = stage.scrollTop;
      stage.classList.add('map-grabbing');
      stage.setPointerCapture(event.pointerId);
    });
    stage.addEventListener('pointermove', function (event) {
      if (!dragging) return;
      stage.scrollLeft = leftAt - (event.clientX - startX);
      stage.scrollTop = topAt - (event.clientY - startY);
    });
    function endDrag() { dragging = false; stage.classList.remove('map-grabbing'); }
    stage.addEventListener('pointerup', endDrag);
    stage.addEventListener('pointercancel', endDrag);
  }
})();

// The reading layer over the map, written from the journeys a reader
// actually makes, not from what a canvas can do:
//   - "what is this box?" — pointing at a node shows a small card beside it:
//     the summary, the size, and its arrows as sentences. The question is
//     answered without leaving the map, so a jump is for reading in full.
//   - "where is this on the map?" — every group card gets an "on the map"
//     link back to its node, which is lit for a moment; the round trip is one
//     click each way.
//   - "how does a request go through?" — pointing at a node on the main
//     path lights the whole path and its arrows, and the card says which
//     step this is.
// None of it exists without scripting, and none of it moves a node.
(function () {
  var maps = document.querySelectorAll('[data-map]');
  for (var index = 0; index < maps.length; index++) {
    bindReading(maps[index]);
  }
  function titleOf(node) {
    var lines = node.querySelectorAll('.map-node-title');
    var parts = [];
    for (var i = 0; i < lines.length; i++) parts.push(lines[i].textContent);
    return parts.join(' ');
  }
  function bindReading(map) {
    var stage = map.querySelector('[data-map-stage]') || map;
    var nodes = map.querySelectorAll('[data-node]');
    var byId = {};
    for (var i = 0; i < nodes.length; i++) byId[nodes[i].getAttribute('data-node')] = nodes[i];
    var edges = map.querySelectorAll('.map-edge');
    var card = document.createElement('div');
    card.className = 'map-card';
    card.hidden = true;
    stage.appendChild(card);
    stage.style.position = stage.style.position || 'relative';

    function sentences(id) {
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
    function show(node) {
      var id = node.getAttribute('data-node');
      var label = node.getAttribute('aria-label') || '';
      var counts = label.indexOf(', ') > 0 ? label.slice(label.indexOf(', ') + 2) : '';
      var html = '<b>' + titleOf(node) + '</b>';
      var summary = node.getAttribute('data-summary');
      if (summary) html += '<p>' + summary + '</p>';
      if (counts) html += '<span class="map-card-meta">' + counts + '</span>';
      var step = map.traceIndex ? map.traceIndex(node) : -1;
      if (step >= 0) html += '<span class="map-card-meta">step ' + (step + 1) + ' of ' + map.traceLength + ' on the main path</span>';
      var keys = (node.getAttribute('data-keys') || '').split(' | ').filter(Boolean);
      if (keys.length) html += '<ul class="map-card-keys">' + keys.map(function (k) { return '<li><code>' + k.replace(/ — .*$/, '') + '</code>' + (k.indexOf(' — ') > 0 ? ' — ' + k.slice(k.indexOf(' — ') + 3) : '') + '</li>'; }).join('') + '</ul>';
      var arrows = sentences(id);
      if (arrows.length) html += '<ul>' + arrows.map(function (a) { return '<li>' + a + '</li>'; }).join('') + '</ul>';
      html += '<span class="map-card-hint">click \u2014 open its card</span>';
      card.innerHTML = html;
      card.hidden = false;
      var nodeBox = node.getBoundingClientRect(), stageBox = stage.getBoundingClientRect();
      var left = nodeBox.right - stageBox.left + stage.scrollLeft + 8;
      var top = nodeBox.top - stageBox.top + stage.scrollTop;
      if (left + card.offsetWidth > stage.scrollLeft + stage.clientWidth) {
        left = nodeBox.left - stageBox.left + stage.scrollLeft - card.offsetWidth - 8;
      }
      card.style.left = Math.max(0, left) + 'px';
      card.style.top = Math.max(0, top) + 'px';
    }
    function hide() { card.hidden = true; }
    for (var n = 0; n < nodes.length; n++) {
      nodes[n].addEventListener('mouseenter', function (event) { show(event.currentTarget); });
      nodes[n].addEventListener('focus', function (event) { show(event.currentTarget); });
      nodes[n].addEventListener('mouseleave', hide);
      nodes[n].addEventListener('blur', hide);
    }

    // A group card links back to its node on the map.
    for (var k = 0; k < nodes.length; k++) {
      var href = nodes[k].getAttribute('href') || '';
      if (href.charAt(0) !== '#') continue;
      var group = document.getElementById(href.slice(1));
      var head = group && group.querySelector('.group-head');
      if (!head || head.querySelector('.on-map')) continue;
      var link = document.createElement('a');
      link.className = 'on-map';
      link.href = '#' + nodes[k].getAttribute('data-node');
      link.textContent = 'on the map';
      link.addEventListener('click', (function (node) {
        return function (event) {
          event.preventDefault();
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
      trace[u].addEventListener('mouseenter', lightTrace);
      trace[u].addEventListener('focus', lightTrace);
      trace[u].addEventListener('mouseleave', unlightTrace);
      trace[u].addEventListener('blur', unlightTrace);
    }
    map.traceIndex = traceIndex;
    map.traceLength = traceIds.length;
  }
})();
