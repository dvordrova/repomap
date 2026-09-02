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
    var edges = map.querySelectorAll('.map-edge');
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
