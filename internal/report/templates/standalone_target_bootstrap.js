(function () {
  'use strict';

  var bundleNode = document.getElementById('rm-standalone-target-bundle');
  var reportNode = document.getElementById('rm-report-data');
  if (!bundleNode || !reportNode) throw new Error('repomap standalone target bundle is incomplete');

  var bundle = JSON.parse(bundleNode.textContent || 'null');
  if (!bundle || Number(bundle.version) !== 1 ||
      !Array.isArray(bundle.targets) || !bundle.targets.length) {
    throw new Error('repomap standalone target bundle is invalid');
  }

  function canonicalTargetIndex() {
    var params = new URLSearchParams(String(window.location && window.location.search || ''));
    var values = params.getAll('target');
    if (values.length !== 1 || !/^(0|[1-9][0-9]*)$/.test(values[0])) return -1;
    var value = Number(values[0]);
    return Number.isSafeInteger(value) ? value : -1;
  }

  function readyTarget(index) {
    var target = bundle.targets[index];
    return !!(target && target.available && target.payload && typeof target.payload === 'object');
  }

  var defaultIndex = Number(bundle.default_target_index);
  if (!Number.isSafeInteger(defaultIndex) || defaultIndex < 0 || !readyTarget(defaultIndex)) {
    throw new Error('repomap standalone target default is invalid');
  }
  var selectedIndex = canonicalTargetIndex();
  if (selectedIndex < 0 || selectedIndex >= bundle.targets.length || !readyTarget(selectedIndex)) {
    selectedIndex = defaultIndex;
  }

  var selected = bundle.targets[selectedIndex];
  var data = selected.payload;
  data.target_navigation = {
    version: 3,
    default_target_index: defaultIndex,
    current_target_index: selectedIndex,
    targets: bundle.targets.map(function (target) {
      return {
        target_ref: String(target.target_ref || ''),
        kind: String(target.kind || ''),
        module_path: String(target.module_path || ''),
        module_dir: String(target.module_dir || ''),
        display_path: String(target.display_path || ''),
        available: !!target.available,
        href: target.available ? String(target.href || '') : '',
      };
    }),
  };
  reportNode.textContent = JSON.stringify(data);

  var language = String(data.report_language || '').toLowerCase() === 'ru' ? 'ru' : 'en';
  if (document.documentElement) document.documentElement.lang = language;
  var title = String(data.repo_name || '');
  if (data.project_guess) title += ' — ' + String(data.project_guess);
  if (title) document.title = 'repomap — ' + title;
})();
