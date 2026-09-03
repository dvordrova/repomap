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
