// Find a symbol, file, route or group on the page. The box exists only when
// scripting does, and the page it filters is complete without it: this is a
// reader's convenience over the HTML, never the HTML's dependency. A reader
// who came to find one thing in a hundred and thirty cards had only the
// browser's own search, which does not know a card from a footnote.
(function () {
  var nav = document.querySelector('nav.nav');
  if (!nav) { return; }
  var box = document.createElement('input');
  box.type = 'search';
  box.className = 'find';
  box.placeholder = 'Find a symbol, file, route or group';
  box.setAttribute('aria-label', 'Find on this page');
  var count = document.createElement('span');
  count.className = 'find-count';
  nav.appendChild(box);
  nav.appendChild(count);

  var cards = Array.prototype.slice.call(document.querySelectorAll('.group'));
  var rows = Array.prototype.slice.call(document.querySelectorAll('.routes li'));
  var haystacks = cards.concat(rows).map(function (node) {
    return { node: node, text: node.textContent.toLowerCase() };
  });

  var timer = null;
  function apply() {
    var query = box.value.trim().toLowerCase();
    var shown = 0;
    haystacks.forEach(function (entry) {
      var hit = query === '' || entry.text.indexOf(query) !== -1;
      entry.node.hidden = !hit;
      if (hit && query !== '') { shown++; }
    });
    count.textContent = query === '' ? '' : shown + ' of ' + haystacks.length;
    document.documentElement.classList.toggle('finding', query !== '');
  }
  box.addEventListener('input', function () {
    if (timer) { clearTimeout(timer); }
    timer = setTimeout(apply, 80);
  });
  box.addEventListener('keydown', function (event) {
    if (event.key === 'Escape') { box.value = ''; apply(); box.blur(); }
  });
  document.addEventListener('keydown', function (event) {
    if (event.key === '/' && document.activeElement !== box &&
        !/^(INPUT|TEXTAREA)$/.test(document.activeElement.tagName)) {
      event.preventDefault();
      box.focus();
    }
  });
})();
