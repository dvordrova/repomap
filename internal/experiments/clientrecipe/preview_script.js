(function () {
  'use strict';

  var model = window.__CLIENT_RECIPE_MODEL__;
  var app = document.getElementById('app');
  var overlayRoot = document.getElementById('overlay-root');
  var announcer = document.getElementById('route-announcer');
  var auditTrigger = null;

  function element(tag, className, text) {
    var node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== undefined && text !== null) node.textContent = String(text);
    return node;
  }

  function object(node, name) {
    node.setAttribute('data-object', name);
    return node;
  }

  function action(node, name) {
    node.setAttribute('data-action', name);
    return node;
  }

  function routeLink(label, href, className, actionName) {
    var node = element('a', className || 'quiet-button', label);
    node.href = href;
    if (actionName) action(node, actionName);
    return node;
  }

  function actionButton(label, className, actionName, handler) {
    var node = element('button', className || 'secondary-button', label);
    node.type = 'button';
    if (actionName) action(node, actionName);
    node.addEventListener('click', handler);
    return node;
  }

  function page(state, className) {
    var node = element('section', 'page ' + (className || ''));
    node.setAttribute('data-state', state);
    return node;
  }

  function roleBadge(role) {
    var className = 'role-badge' + (role.task_required ? '' : ' additional');
    return element('span', className, role.label + ' · ' + (role.task_required ? 'Task-required' : 'Additional pattern'));
  }

  function statusLabel(example) {
    return element('span', 'status' + (example.complete ? '' : ' incomplete'), example.status);
  }

  function getStep(id) {
    return model.steps.find(function (row) { return row.id === id; });
  }

  function getExample(id) {
    return model.examples.find(function (row) { return row.id === id; });
  }

  function getSlot(example, stepID) {
    return example && example.slots.find(function (row) { return row.step_id === stepID; });
  }

  function routeParts() {
    var hash = window.location.hash || '#/target';
    return hash.replace(/^#\/?/, '').split('/').filter(Boolean);
  }

  function factCount(value) {
    return value + ' source ' + (value === 1 ? 'fact' : 'facts');
  }

  function announce(text) {
    announcer.textContent = '';
    window.setTimeout(function () { announcer.textContent = text; }, 0);
  }

  function render() {
    closeAudit(false);
    var parts = routeParts();
    if (parts[0] === 'recipe' && parts[1] === 'step' && parts[2]) {
      renderStep(parts[2]);
    } else if (parts[0] === 'recipe' && parts[1] === 'example' && parts[2]) {
      renderExample(parts[2]);
    } else if (parts[0] === 'recipe' && parts[1] === 'evidence') {
      renderEvidence(parts.slice(2));
    } else if (parts[0] === 'recipe') {
      renderOverview();
    } else {
      renderLanding();
    }
    app.focus({ preventScroll: true });
    window.scrollTo({ top: 0, left: 0, behavior: 'auto' });
  }

  function renderLanding() {
    var root = page('target_landing', 'landing');
    var top = element('div', 'landing-top');
    var intro = element('div');
    intro.appendChild(element('p', 'eyebrow', 'Available repository task'));
    var title = element('h1', 'hero-title');
    title.appendChild(document.createTextNode('Plan a source-backed '));
    title.appendChild(element('em', '', 'change.'));
    intro.appendChild(title);
    intro.appendChild(element('p', 'hero-copy', 'This experiment currently supports one concrete task and turns repeated repository evidence into a bounded recipe.'));
    top.appendChild(intro);

    var coverage = object(element('aside', 'coverage-note'), 'coverage_hint');
    var coverageTop = element('div');
    coverageTop.appendChild(element('span', '', 'Experiment scope'));
    coverageTop.appendChild(element('strong', '', model.summary.boundaries + ' boundaries'));
    coverageTop.appendChild(element('span', '', model.summary.complete + ' complete examples · ' + model.summary.excluded + ' decoys excluded'));
    coverage.appendChild(coverageTop);
    var scope = object(element('div', 'scope-status'), 'experiment_scope');
    scope.appendChild(element('span', 'scope-pill', model.scope.evidence));
    scope.appendChild(element('span', 'scope-caveat', model.scope.generalization));
    coverage.appendChild(scope);
    top.appendChild(coverage);
    root.appendChild(top);

    var grid = element('div', 'task-grid');
    model.tasks.forEach(function (task) {
      var card;
      if (task.available) {
        card = actionButton('', 'task-card primary', 'open_recipe', function () { window.location.hash = '#/recipe'; });
        card.appendChild(element('strong', '', task.title));
        card.appendChild(element('p', '', task.description));
        card.appendChild(element('span', 'task-action', 'Open recipe →'));
      } else {
        card = element('article', 'task-card');
        card.appendChild(element('strong', '', task.title));
        card.appendChild(element('p', '', task.description));
      }
      card.setAttribute('data-available', String(task.available));
      object(card, 'task_card');
      grid.appendChild(card);
    });
    root.appendChild(grid);
    object(root, 'target_context');
    app.replaceChildren(root);
    announce('Available repository task');
  }

  function renderOverview() {
    var root = page('recipe_overview', 'overview');
    var routeBar = element('div', 'route-bar');
    routeBar.appendChild(routeLink('← Available task', '#/target', 'back-link', 'return_target'));
    routeBar.appendChild(element('span', 'route-label', model.target.name + ' · ' + model.target.language));
    root.appendChild(routeBar);

    var hero = object(element('section', 'overview-hero'), 'recipe_summary');
    var heroCopy = element('div');
    heroCopy.appendChild(element('p', 'eyebrow', 'Repository-specific recipe'));
    heroCopy.appendChild(element('h1', 'overview-title', 'Add an external client'));
    heroCopy.appendChild(element('p', 'overview-copy', model.steps.length + ' evidence-backed steps connect configuration, a local boundary, live wiring, behavior, verification, and failure handling—without reconstructing the entire codebase.'));
    hero.appendChild(heroCopy);
    var metrics = object(element('div', 'metric-row'), 'coverage');
    [
      [model.summary.boundaries, 'client boundaries'],
      [model.summary.complete, 'complete examples'],
      [model.steps.length, 'recipe steps'],
      [model.summary.excluded, 'decoys excluded']
    ].forEach(function (metric) {
      var card = element('div', 'metric');
      card.appendChild(element('strong', '', metric[0]));
      card.appendChild(element('span', '', metric[1]));
      metrics.appendChild(card);
    });
    hero.appendChild(metrics);
    root.appendChild(hero);

    var heading = element('div', 'section-heading');
    var headingCopy = element('div');
    headingCopy.appendChild(element('span', 'section-kicker', 'Change sequence'));
    headingCopy.appendChild(element('h2', '', 'Follow the local shape'));
    headingCopy.appendChild(element('p', '', 'Each step is reduced from the four production-reachable client boundaries below.'));
    heading.appendChild(headingCopy);
    root.appendChild(heading);

    var recipeLayout = element('div', 'recipe-layout');
    var list = object(element('div', 'step-list'), 'primary_steps');
    model.steps.forEach(function (step) {
      var card = actionButton('', 'step-card', 'open_step', function () { window.location.hash = '#/recipe/step/' + step.id; });
      card.appendChild(element('span', 'step-number', String(step.number).padStart(2, '0')));
      var copy = element('div');
      copy.appendChild(element('h3', '', step.title));
      copy.appendChild(element('p', '', step.purpose));
      card.appendChild(copy);
      var meta = element('div', 'step-meta');
      var coverageCopy = step.complete_coverage + ' fully covered';
      if (step.partial_coverage) coverageCopy += ' · ' + step.partial_coverage + ' partial';
      meta.appendChild(element('div', '', coverageCopy));
      meta.appendChild(element('div', '', factCount(step.evidence_count)));
      card.appendChild(meta);
      card.appendChild(element('span', 'step-arrow', '→'));
      list.appendChild(card);
    });
    recipeLayout.appendChild(list);
    recipeLayout.appendChild(renderMostCompleteExamples());
    root.appendChild(recipeLayout);

    var roleHeading = element('div', 'section-heading');
    var roleCopy = element('div');
    roleCopy.appendChild(element('span', 'section-kicker', 'Role coverage'));
    roleCopy.appendChild(element('h2', '', 'Task contract vs. repository pattern'));
    roleCopy.appendChild(element('p', '', 'Task-required roles determine completeness. Observed frequency only describes the task-complete examples in this controlled fixture.'));
    roleHeading.appendChild(roleCopy);
    root.appendChild(roleHeading);
    var roleCoverage = object(element('div', 'role-coverage'), 'role_coverage');
    model.roles.forEach(function (role) {
      var row = element('article', 'role-coverage-row');
      var name = element('div');
      name.appendChild(element('strong', '', role.label));
      name.appendChild(element('span', 'contract-label' + (role.task_required ? '' : ' additional'), role.task_required ? 'Task-required' : 'Not required by task'));
      row.appendChild(name);
      row.appendChild(element('span', 'role-frequency', role.observed_complete + ' / ' + role.complete_examples + ' complete examples'));
      row.appendChild(element('span', 'observed-pattern', role.observed_necessity));
      roleCoverage.appendChild(row);
    });
    root.appendChild(roleCoverage);

    var examplesHeading = element('div', 'section-heading');
    var examplesCopy = element('div');
    examplesCopy.appendChild(element('span', 'section-kicker', 'Concrete boundaries'));
    examplesCopy.appendChild(element('h2', '', 'Compare complete examples'));
    examplesCopy.appendChild(element('p', '', 'No global copy recommendation is inferred without knowing the kind of boundary you intend to add.'));
    examplesHeading.appendChild(examplesCopy);
    var showAll = actionButton('Show all ' + model.examples.length, 'secondary-button', 'show_all_examples', function () {
      showAll.setAttribute('aria-expanded', 'true');
      showAll.textContent = 'All ' + model.examples.length + ' shown';
      showAll.disabled = true;
      renderExampleCards(exampleGrid, model.examples);
    });
    showAll.setAttribute('aria-expanded', 'false');
    examplesHeading.appendChild(showAll);
    root.appendChild(examplesHeading);
    var exampleGrid = object(element('div', 'example-grid'), 'example_summary');
    renderExampleCards(exampleGrid, model.examples.slice(0, 3));
    root.appendChild(exampleGrid);

    var auditEntry = element('section', 'audit-entry');
    var auditCopy = element('div');
    auditCopy.appendChild(element('span', 'section-kicker', 'Why this is trustworthy'));
    auditCopy.appendChild(element('h2', '', 'Candidate audit'));
    auditCopy.appendChild(element('p', '', 'Inspect the ' + model.summary.excluded + ' generated, test-only, unreachable, prose, and standard-library candidates that did not become production examples.'));
    auditEntry.appendChild(auditCopy);
    auditTrigger = actionButton('Open audit · ' + model.summary.excluded, 'secondary-button', 'open_audit', openAudit);
    auditTrigger.setAttribute('aria-expanded', 'false');
    auditEntry.appendChild(auditTrigger);
    root.appendChild(auditEntry);

    app.replaceChildren(root);
    announce('External client recipe overview');
  }

  function renderMostCompleteExamples() {
    var examples = model.examples.filter(function (row) { return row.most_complete; });
    var card = object(element('aside', 'most-complete'), 'most_complete_examples');
    var head = element('div', 'most-complete-head');
    head.appendChild(element('span', 'most-complete-label', 'Most complete examples'));
    head.appendChild(element('p', '', 'Deterministic full tie set by observed role coverage. This is not a recommendation for every future client.'));
    card.appendChild(head);
    examples.forEach(function (example) {
      var item = object(element('div', 'most-complete-item'), 'most_complete_example');
      item.appendChild(element('h3', '', example.name));
      item.appendChild(element('p', '', example.summary));
      var facts = element('div', 'most-complete-facts');
      facts.appendChild(element('span', '', factCount(example.evidence_count)));
      facts.appendChild(element('span', '', example.role_coverage + ' / ' + model.roles.length + ' observed roles'));
      item.appendChild(facts);
      item.appendChild(routeLink('Inspect ' + example.name + ' →', '#/recipe/example/' + example.id, 'primary-button most-complete-action', 'open_example'));
      card.appendChild(item);
    });
    return card;
  }

  function renderExampleCards(container, examples) {
    container.replaceChildren();
    examples.forEach(function (example) {
      var card = element('article', 'example-card' + (example.complete ? '' : ' incomplete'));
      var top = element('div', 'card-topline');
      top.appendChild(statusLabel(example));
      top.appendChild(element('span', 'route-label', example.verification_kind));
      card.appendChild(top);
      card.appendChild(element('h3', '', example.name));
      card.appendChild(element('p', '', example.summary));
      if (!example.complete) {
        card.appendChild(element('div', 'missing-inline', 'Missing: ' + example.missing.join(', ')));
      }
      card.appendChild(routeLink('Inspect boundary →', '#/recipe/example/' + example.id, 'quiet-button', 'open_example'));
      container.appendChild(card);
    });
  }

  function renderStep(id) {
    var step = getStep(id);
    if (!step) { renderOverview(); return; }
    var root = page('recipe_step', 'detail-shell');
    var routeBar = element('div', 'route-bar');
    routeBar.appendChild(routeLink('← Recipe overview', '#/recipe', 'back-link', 'return_overview'));
    routeBar.appendChild(element('span', 'route-label', 'Step ' + step.number + ' of ' + model.steps.length));
    root.appendChild(routeBar);
    var hero = element('section', 'detail-hero');
    var copy = object(element('div'), 'step_purpose');
    copy.appendChild(element('span', 'detail-number', 'STEP ' + String(step.number).padStart(2, '0')));
    copy.appendChild(element('h1', 'detail-title', step.title));
    copy.appendChild(element('p', 'detail-copy', step.purpose));
    var roles = object(element('div', 'role-row'), 'necessity');
    step.roles.forEach(function (role) { roles.appendChild(roleBadge(role)); });
    copy.appendChild(roles);
    hero.appendChild(copy);
    var stat = object(element('aside', 'detail-stat'), 'evidence_summary');
    stat.appendChild(element('strong', '', step.evidence_count));
    stat.appendChild(element('span', '', 'exact source facts'));
    hero.appendChild(stat);
    root.appendChild(hero);

    var heading = element('div', 'section-heading');
    var headingCopy = element('div');
    headingCopy.appendChild(element('span', 'section-kicker', 'Repository coverage'));
    headingCopy.appendChild(element('h2', '', step.complete_coverage + ' boundaries fully cover this step'));
    heading.appendChild(headingCopy);
    if (step.evidence.length) heading.appendChild(routeLink('View evidence →', '#/recipe/evidence/step/' + step.id, 'primary-button', 'open_evidence'));
    root.appendChild(heading);
    var grid = object(element('div', 'coverage-grid'), 'covered_examples');
    model.examples.forEach(function (example) {
      var slot = getSlot(example, step.id);
      var card = element('article', 'coverage-card ' + slot.status);
      card.appendChild(element('h3', '', example.name));
      if (slot.evidence.length) {
        card.appendChild(element('p', '', factCount(slot.evidence.length) + ' · ' + slot.status));
      }
      if (slot.missing.length) {
        card.appendChild(element('p', '', 'Missing ' + slot.missing.join(', ')));
      }
      if (slot.evidence.length) {
        card.appendChild(routeLink('View evidence →', '#/recipe/evidence/example/' + example.id + '/' + slot.step_id, 'secondary-button', 'open_evidence'));
      }
      card.appendChild(routeLink('Inspect example', '#/recipe/example/' + example.id, 'quiet-button', 'open_example'));
      grid.appendChild(card);
    });
    root.appendChild(grid);
    app.replaceChildren(root);
    announce(step.title + ' recipe step');
  }

  function renderExample(id) {
    var example = getExample(id);
    if (!example) { renderOverview(); return; }
    var root = page('example_instance', 'detail-shell');
    var routeBar = element('div', 'route-bar');
    routeBar.appendChild(routeLink('← Recipe overview', '#/recipe', 'back-link', 'return_overview'));
    routeBar.appendChild(element('span', 'route-label', example.verification_kind));
    root.appendChild(routeBar);
    var hero = element('section', 'detail-hero');
    var copy = element('div');
    var topline = object(element('div', 'example-detail-top'), 'instance_status');
    topline.appendChild(statusLabel(example));
    if (example.most_complete) topline.appendChild(element('span', 'most-complete-chip', 'Most complete set'));
    copy.appendChild(topline);
    copy.appendChild(element('h1', 'detail-title', example.name));
    copy.appendChild(element('p', 'detail-copy', example.summary));
    if (example.missing.length) {
      var missing = object(element('ul', 'missing-list'), 'missing_slots');
      example.missing.forEach(function (label) { missing.appendChild(element('li', '', label)); });
      copy.appendChild(missing);
    }
    hero.appendChild(copy);
    var stat = object(element('aside', 'detail-stat'), 'source_counts');
    stat.appendChild(element('strong', '', example.evidence_count));
    stat.appendChild(element('span', '', 'exact source facts'));
    hero.appendChild(stat);
    root.appendChild(hero);

    var heading = element('div', 'section-heading');
    var headingCopy = element('div');
    headingCopy.appendChild(element('span', 'section-kicker', model.steps.length + ' local steps'));
    headingCopy.appendChild(element('h2', '', 'Boundary slot map'));
    headingCopy.appendChild(element('p', '', example.complete ? 'Every task-required role is grounded; additional repository patterns may still be absent.' : 'Missing task-required roles stay visible; existing evidence remains inspectable.'));
    heading.appendChild(headingCopy);
    root.appendChild(heading);
    var slots = object(element('div', 'slot-map'), 'slot_map');
    example.slots.forEach(function (slot) {
      var card = element('article', 'slot-card ' + slot.status);
      card.setAttribute('data-slot-status', slot.status);
      card.appendChild(element('h3', '', slot.title));
      var roles = element('div', 'slot-role-row');
      slot.roles.forEach(function (role) { roles.appendChild(roleBadge(role)); });
      card.appendChild(roles);
      card.appendChild(element('span', 'slot-state ' + slot.status, slot.status));
      if (slot.evidence.length) {
        card.appendChild(element('p', '', slot.covered_roles.map(function (role) { return role.label; }).join(' + ') + ' · ' + factCount(slot.evidence.length)));
      }
      if (slot.missing.length) {
        card.appendChild(element('p', 'slot-missing', 'Missing: ' + slot.missing.join(', ')));
      }
      if (slot.evidence.length) {
        card.appendChild(routeLink('View evidence →', '#/recipe/evidence/example/' + example.id + '/' + slot.step_id, 'secondary-button', 'open_evidence'));
      }
      slots.appendChild(card);
    });
    root.appendChild(slots);
    app.replaceChildren(root);
    announce(example.name + ' client boundary');
  }

  function renderEvidence(parts) {
    var kind = parts[0];
    var evidence = [];
    var title = 'Exact source evidence';
    var subtitle = '';
    var back = '#/recipe';
    if (kind === 'step') {
      var step = getStep(parts[1]);
      if (!step) { renderOverview(); return; }
      evidence = step.evidence;
      title = step.title;
      subtitle = 'Evidence across production-reachable examples';
      back = '#/recipe/step/' + step.id;
    } else if (kind === 'example') {
      var example = getExample(parts[1]);
      var slot = getSlot(example, parts[2]);
      if (!example || !slot) { renderOverview(); return; }
      evidence = slot.evidence;
      title = example.name + ' · ' + slot.title;
      subtitle = 'Exact facts for this boundary slot';
      back = '#/recipe/example/' + example.id;
    } else {
      renderOverview(); return;
    }

    var root = page('evidence', 'detail-shell');
    var routeBar = element('div', 'route-bar');
    routeBar.appendChild(routeLink('← Back to selection', back, 'back-link', 'return_detail'));
    routeBar.appendChild(element('span', 'route-label', factCount(evidence.length)));
    root.appendChild(routeBar);
    var hero = element('section', 'detail-hero');
    var copy = element('div');
    copy.appendChild(element('p', 'eyebrow', 'Source-backed claim'));
    copy.appendChild(element('h1', 'detail-title', title));
    copy.appendChild(element('p', 'detail-copy', subtitle));
    hero.appendChild(copy);
    root.appendChild(hero);

    var heading = element('div', 'section-heading');
    var headingCopy = element('div');
    headingCopy.appendChild(element('span', 'section-kicker', 'Created on demand'));
    headingCopy.appendChild(element('h2', '', 'Repository locators'));
    headingCopy.appendChild(element('p', '', 'Paths and symbols appear only after you explicitly ask for evidence.'));
    heading.appendChild(headingCopy);
    root.appendChild(heading);
    var list = element('div', 'evidence-list');
    var visible = Math.min(3, evidence.length);
    renderEvidenceCards(list, evidence.slice(0, visible));
    root.appendChild(list);
    if (evidence.length > visible) {
      var showMore = actionButton('Show all ' + evidence.length + ' facts', 'secondary-button', 'show_all_evidence', function () {
        showMore.setAttribute('aria-expanded', 'true');
        showMore.remove();
        renderEvidenceCards(list, evidence);
      });
      showMore.setAttribute('aria-expanded', 'false');
      showMore.style.marginTop = '16px';
      root.appendChild(showMore);
    }
    app.replaceChildren(root);
    announce(title + ' evidence');
  }

  function renderEvidenceCards(container, evidence) {
    container.replaceChildren();
    evidence.forEach(function (row) {
      var card = object(element('article', 'evidence-card'), 'source_locator');
      var header = element('div', 'evidence-header');
      var copy = element('div');
      copy.appendChild(element('span', 'evidence-role', row.example + ' · ' + row.role));
      copy.appendChild(object(element('h3', '', row.symbol), 'symbol'));
      copy.appendChild(element('div', 'locator', row.locator));
      header.appendChild(copy);
      var source = routeLink('Open exact source ↗', row.source_href, 'source-link', 'open_exact_source');
      header.appendChild(source);
      card.appendChild(header);
      var authority = object(element('div', 'authority-row'), 'authority');
      authority.appendChild(element('span', 'authority-chip', row.authority));
      authority.appendChild(object(element('span', 'authority-chip', row.provenance), 'provenance'));
      card.appendChild(authority);
      container.appendChild(card);
    });
  }

  function openAudit() {
    if (overlayRoot.firstChild) return;
    var backdrop = element('div', 'drawer-backdrop');
    backdrop.setAttribute('role', 'presentation');
    backdrop.addEventListener('click', function (event) { if (event.target === backdrop) closeAudit(true); });
    var drawer = element('aside', 'drawer');
    drawer.setAttribute('role', 'dialog');
    drawer.setAttribute('aria-modal', 'true');
    drawer.setAttribute('aria-labelledby', 'audit-title');
    var head = element('div', 'drawer-head');
    var copy = element('div');
    copy.appendChild(element('span', 'section-kicker', 'Separate disclosure'));
    var title = element('h2', '', 'Candidate audit');
    title.id = 'audit-title';
    copy.appendChild(title);
    head.appendChild(copy);
    var close = actionButton('×', 'icon-button', 'close_audit', function () { closeAudit(true); });
    close.setAttribute('aria-label', 'Close candidate audit');
    head.appendChild(close);
    drawer.appendChild(head);
    var ledger = element('div', 'audit-ledger');
    [[model.summary.observed, 'observed'], [model.summary.boundaries, 'admitted'], [model.summary.excluded, 'excluded']].forEach(function (item) {
      var cell = element('div'); cell.appendChild(element('strong', '', item[0])); cell.appendChild(element('span', '', item[1])); ledger.appendChild(cell);
    });
    drawer.appendChild(ledger);
    var list = element('div', 'audit-list');
    model.audit.forEach(function (row) {
      var item = element('article', 'audit-row');
      item.setAttribute('data-audit-row', row.id);
      item.appendChild(element('span', 'section-kicker', row.kind));
      item.appendChild(element('h3', '', row.label));
      item.appendChild(element('p', '', row.reason));
      item.appendChild(element('div', 'locator', row.locator));
      list.appendChild(item);
    });
    drawer.appendChild(list);
    backdrop.appendChild(drawer);
    overlayRoot.appendChild(backdrop);
    if (auditTrigger) auditTrigger.setAttribute('aria-expanded', 'true');
    close.focus();
  }

  function closeAudit(restoreFocus) {
    if (!overlayRoot.firstChild) return;
    overlayRoot.replaceChildren();
    if (auditTrigger) auditTrigger.setAttribute('aria-expanded', 'false');
    if (restoreFocus && auditTrigger) auditTrigger.focus();
  }

  document.addEventListener('keydown', function (event) {
    if (event.key === 'Escape' && overlayRoot.firstChild) {
      event.preventDefault();
      closeAudit(true);
    }
  });
  window.addEventListener('hashchange', render);
  window.__clientRecipePreview = Object.freeze({ render: render, openAudit: openAudit, closeAudit: closeAudit });
  render();
}());
