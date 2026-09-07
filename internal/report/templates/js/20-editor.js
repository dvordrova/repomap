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

  function say(message, failed, detail) {
    if (!status) return;
    status.replaceChildren(document.createTextNode(message));
    if(detail){var details=document.createElement('details'),summary=document.createElement('summary'),original=document.createElement('pre');summary.textContent=rmT('Technical details');original.textContent=detail;details.append(summary,original);status.appendChild(details);}
    status.className = failed ? 'status bad' : 'status';
    status.hidden = false;
  }

  function uiError(message,detail) { var error=new Error(message);error.repomapMessage=true;error.detail=detail;return error; }

  function open(spec) {
    var parts = spec.split(':');
    var column = parts.length > 2 ? parseInt(parts.pop(), 10) || 0 : 0;
    var line = parts.length > 1 ? parseInt(parts.pop(), 10) || 0 : 0;
    var sourceID = sourceIDs[parts.join(':')];
    if (typeof sourceID !== 'string' || !sourceID) {
      say(rmT('This source is not openable from this report.'), true);
      return;
    }
    fetch(base + '/api/open', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Repomap-Action': 'open-file' },
      body: JSON.stringify({ run_id: runID, source_id: sourceID, line: line, column: column })
    }).then(function (response) {
      return response.json().catch(function () {
        throw uiError(rmT('The editor response was not valid JSON.'));
      }).then(function (body) {
        if (!response.ok) throw uiError(rmT('Could not open this source in the editor.'),body&&body.error?String(body.error):'');
        if (!body || body.status !== 'opened') throw uiError(rmT('The editor did not confirm the source action.'));
        say(rmT('Opened {0} in your editor.',spec), false);
      });
    }).catch(function (error) {
      say(error && error.repomapMessage ? error.message : rmT('Could not open this source in the editor.'), true, error && error.repomapMessage ? error.detail : String(error));
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
