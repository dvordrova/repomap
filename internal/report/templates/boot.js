// Decode the complete saved data before executing the unchanged classic bundle.
// This is representation decoding; the browser performs no repository analysis.
(function () {
  var status = document.getElementById('rm-load-status');
  document.body.dataset.reportLoadState = 'loading';
  async function decode(node) {
    var binary = atob(node.textContent), bytes = new Uint8Array(binary.length);
    for (var i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
    var stream = new Blob([bytes]).stream().pipeThrough(new DecompressionStream('gzip'));
    var decoded = await new Response(stream).arrayBuffer();
    node.textContent = new TextDecoder('utf-8', {fatal: true}).decode(decoded);
    node.removeAttribute('data-rm-encoding');
  }
  async function prepare() {
    for (var node of document.querySelectorAll('[data-rm-encoding="gzip-base64"]')) await decode(node);
  }
  prepare().then(function () {
    var saved = document.getElementById('rm-report-app-js'), executable = document.createElement('script');
    executable.id = saved.id;
    executable.textContent = saved.textContent + '\n;document.getElementById("rm-report-app-js").dataset.executed="true";\n';
    saved.replaceWith(executable);
    // Browsers report classic-script errors separately rather than throwing
    // from replaceWith. A blocked or failed bundle must never claim readiness.
    if (executable.dataset.executed !== 'true') throw new Error('Report startup failed');
    document.body.dataset.reportLoadState = 'ready';
    status.hidden = true;
  }).catch(function () {
    document.body.dataset.reportLoadState = 'failed';
    status.textContent = status.dataset.error;
    status.hidden = false;
  });
})();
