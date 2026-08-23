(function () {
  'use strict';

  try {

  var bundleNode = document.getElementById('rm-standalone-target-bundle');
  var reportNode = document.getElementById('rm-report-data');
  if (!bundleNode || !reportNode) throw new Error('repomap standalone target bundle is incomplete');

  var bundle = JSON.parse(bundleNode.textContent || 'null');
  if (!bundle || bundle.version !== 3 ||
      !Array.isArray(bundle.targets) || !bundle.targets.length) {
    throw new Error('repomap standalone target bundle is invalid');
  }

  function publishSelectionError(message) {
    reportNode.textContent = JSON.stringify({
      standalone_target_error: String(message || 'repomap standalone target selection failed')
    });
  }

  function canonicalTargetIndex() {
    var search = String(window.location && window.location.search || '');
    if (search === '') return null;
    var match = /^\?target=(0|[1-9][0-9]*)$/.exec(search);
    if (!match) {
      publishSelectionError('repomap standalone target query is invalid; use exactly ?target=N with a listed target index');
      return false;
    }
    var value = Number(match[1]);
    if (!Number.isSafeInteger(value)) {
      publishSelectionError('repomap standalone target index is outside the supported integer range');
      return false;
    }
    return value;
  }

  function completeTarget(index) {
    var target = bundle.targets[index];
    return !!(target && target.payload && typeof target.payload === 'object' &&
      target.href === '?target=' + String(index) + '#/program');
  }

  var defaultIndex = bundle.default_target_index;
  if (!Number.isSafeInteger(defaultIndex) || defaultIndex < 0 ||
      defaultIndex >= bundle.targets.length) {
    throw new Error('repomap standalone target default is invalid');
  }
  bundle.targets.forEach(function (_target, index) {
    if (!completeTarget(index)) {
      throw new Error('repomap standalone target bundle contains an incomplete selected target');
    }
  });
  var requestedIndex = canonicalTargetIndex();
  if (requestedIndex === false) return;
  var selectedIndex = requestedIndex == null ? defaultIndex : requestedIndex;
  if (selectedIndex < 0 || selectedIndex >= bundle.targets.length) {
    publishSelectionError('repomap standalone target index ' + String(selectedIndex) +
      ' is not present; choose an index listed by this report');
    return;
  }
  var selected = bundle.targets[selectedIndex];
  var data = selected.payload;
  data.target_navigation = {
    version: 5,
    default_target_index: defaultIndex,
    current_target_index: selectedIndex,
    targets: bundle.targets.map(function (target) {
      return {
        target_id: target.target_id,
        language: target.language,
        kind: target.kind,
        display_name: target.display_name,
        href: target.href,
      };
    }),
  };
  reportNode.textContent = JSON.stringify(data);

  if (document.documentElement) document.documentElement.lang = 'en';
  var title = String(data.repo_name || '');
  if (title) document.title = 'repomap — ' + title;
  } catch (error) {
    var errorReportNode = document.getElementById('rm-report-data');
    if (errorReportNode) {
      errorReportNode.textContent = JSON.stringify({
        standalone_target_error: error && error.message ? error.message : String(error)
      });
    }
  }
})();
