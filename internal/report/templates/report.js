(function () {
  'use strict';
  // Served mode only: the local report server embeds a path -> source id map
  // and an /api/open endpoint that opens the exact location in the editor.
  var idsNode = document.getElementById('rm-source-ids');
  if (!idsNode) return;
  var sourceIDs;
  try { sourceIDs = JSON.parse(idsNode.textContent); } catch (error) { return; }
  var route = /^(.*)\/runs\/([^\/]+)\/report\.html$/.exec(window.location.pathname);
  if (!route) return;
  var base = route[1];
  var runID = decodeURIComponent(route[2]);
  var status = document.getElementById('rm-status');

  function say(message, failed) {
    if (!status) return;
    status.textContent = message;
    status.className = failed ? 'status bad' : 'status';
    status.hidden = false;
  }

  function open(spec) {
    var parts = spec.split(':');
    var column = parts.length > 2 ? parseInt(parts.pop(), 10) || 0 : 0;
    var line = parts.length > 1 ? parseInt(parts.pop(), 10) || 0 : 0;
    var sourceID = sourceIDs[parts.join(':')];
    if (typeof sourceID !== 'string' || !sourceID) {
      say('This source is not openable from this report.', true);
      return;
    }
    fetch(base + '/api/open', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Repomap-Action': 'open-file' },
      body: JSON.stringify({ run_id: runID, source_id: sourceID, line: line, column: column })
    }).then(function (response) {
      return response.json().catch(function () {
        throw new Error('The editor response was not valid JSON.');
      }).then(function (body) {
        if (!response.ok) throw new Error(body && body.error ? String(body.error) : 'open-file failed');
        if (!body || body.status !== 'opened') throw new Error('The editor did not confirm the source action.');
        say('Opened ' + spec + ' in VS Code.', false);
      });
    }).catch(function (error) {
      say(error && error.message ? error.message : String(error), true);
    });
  }

  document.addEventListener('click', function (event) {
    var node = event.target;
    while (node && node.nodeType === 1 && !node.hasAttribute('data-open')) node = node.parentNode;
    if (!node || node.nodeType !== 1) return;
    event.preventDefault();
    open(node.getAttribute('data-open'));
  });
})();
