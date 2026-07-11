(function () {
  'use strict';

  var DATA = JSON.parse(document.getElementById('rm-report-data').textContent);

  var LABELS = {
    overviewTitle: 'Flows',
    candidateFlows: 'Candidate Flows',
    filesToRead: 'Read Order — Open these files in sequence',
    testsToRead: 'Tests',
    executionChain: 'Execution Chain',
    knownUnknowns: 'Known Unknowns',
    unverified: 'Unverified Paths',
    unknowns: 'Unknowns',
    warnings: 'Warnings',
    bundleStats: 'Technical Details',
    confidence: 'Confidence',
    noFlows: 'No flows identified.',
    collapsedDefault: 'Show details',
    collapsedOpen: 'Hide details',
    startHere: 'Start Here',
    quickStart: 'Quick Start',
    noAIExplanation: 'No AI explanation available — rerun with DEEPSEEK_API_KEY.',
    errorUnavailable: 'Analysis unavailable',
    evidenceFiles: 'Evidence files',
  };

  function esc(s) {
    if (!s) return '';
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  function confClass(c) {
    var v = Math.max(0, Math.min(1, c || 0));
    return v > 0.7 ? 'rm-conf-hi' : v > 0.4 ? 'rm-conf-md' : 'rm-conf-lo';
  }

  function confLabel(c) {
    var v = Math.max(0, Math.min(1, c || 0));
    var label = v >= 0.7 ? 'High' : v >= 0.4 ? 'Medium' : 'Low';
    return label + ' ' + Math.round(v * 100) + '%';
  }

  function chainCircleClass(c) {
    var v = Math.max(0, Math.min(1, c || 0));
    return v >= 0.7 ? 'rm-chain-circle--hi' : v >= 0.4 ? 'rm-chain-circle--md' : 'rm-chain-circle--lo';
  }

  // ── DOM builders ───────────────────────────────────────────────

  function el(tag, cls, attrs) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (attrs) Object.keys(attrs).forEach(function (k) { e.setAttribute(k, attrs[k]); });
    return e;
  }

  function txt(tag, cls, text) {
    var e = el(tag, cls);
    e.textContent = text || '';
    return e;
  }

  // ── Components ──────────────────────────────────────────────────

  function renderConfidenceBadge(confidence) {
    var badge = el('span', 'rm-confidence ' + confClass(confidence));
    badge.textContent = confLabel(confidence);
    return badge;
  }

  function renderPill(text, kind) {
    var pill = el('span', 'rm-pill rm-pill--' + (kind || 'accent'));
    pill.textContent = text;
    return pill;
  }

  function renderFlowCard(flow, isRecommended) {
    var card = el('div', 'rm-ov-flow');
    if (isRecommended) card.classList.add('rm-ov-flow--recommended');

    if (flow.error) {
      card.classList.add('rm-ov-flow--error');
      var header = el('div');
      var h3 = txt('h3', '', flow.name || flow.id);
      header.appendChild(h3);
      header.appendChild(renderPill(LABELS.errorUnavailable, 'error'));
      card.appendChild(header);
      card.onclick = function () { showTab('rm-flow-' + flow.id); };
      return card;
    }

    var header = el('div');
    var h3 = txt('h3', '', flow.name || flow.id);
    header.appendChild(h3);
    if (isRecommended) {
      header.appendChild(renderPill(LABELS.startHere));
    }
    header.appendChild(renderConfidenceBadge(flow.confidence));

    card.appendChild(header);

    if (flow.summary) {
      var truncated = flow.summary.length > 120 ? flow.summary.slice(0, 120) + '...' : flow.summary;
      var sum = txt('div', 'rm-summary-line', truncated);
      card.appendChild(sum);
    }

    if (flow.bundle_stats_label) {
      var meta = txt('div', 'rm-meta', flow.bundle_stats_label);
      card.appendChild(meta);
    }

    card.onclick = function () { showTab('rm-flow-' + flow.id); };
    return card;
  }

  function renderChainSteps(chain) {
    var section = el('div');
    section.appendChild(txt('div', 'rm-section-title', LABELS.executionChain));

    var container = el('div', 'rm-chain');

    chain.forEach(function (s) {
      var step = el('div', 'rm-chain-step');

      var circle = el('div', 'rm-chain-circle ' + chainCircleClass(s.confidence));
      circle.textContent = s.step;
      step.appendChild(circle);

      var body = el('div', 'rm-chain-body');

      var nameRow = el('div', 'rm-chain-name');
      var nameTxt = esc(s.name);
      nameRow.appendChild(document.createTextNode('Step ' + s.step + ': ' + nameTxt));
      body.appendChild(nameRow);

      if (s.what_happens) {
        body.appendChild(txt('div', 'rm-chain-desc', s.what_happens));
      }

      var meta = el('div', 'rm-chain-meta');
      meta.appendChild(renderConfidenceBadge(s.confidence));

      // per-step warnings would come from flow-level warnings -- not implemented yet
      body.appendChild(meta);

      if (s.evidence_files && s.evidence_files.length > 0) {
        var filesDiv = el('div', 'rm-chain-files');
        filesDiv.appendChild(txt('div', '', LABELS.evidenceFiles + ':'));
        s.evidence_files.forEach(function (f) {
          filesDiv.appendChild(txt('span', 'rm-chain-file', f));
        });
        body.appendChild(filesDiv);
      }

      step.appendChild(body);
      container.appendChild(step);
    });

    section.appendChild(container);
    return section;
  }

  function renderReadOrder(files) {
    var section = el('div');
    section.appendChild(txt('div', 'rm-section-title', LABELS.filesToRead));

    var ol = el('div', 'rm-read-order');
    files.forEach(function (fi, i) {
      var item = el('div', 'rm-read-order-item');

      var num = el('div', 'rm-read-order-num');
      if (fi.priority >= 3) num.classList.add('rm-read-order-num--p3');
      else if (fi.priority >= 2) num.classList.add('rm-read-order-num--p2');
      num.textContent = i + 1;
      item.appendChild(num);

      var body = el('div', 'rm-read-order-body');
      body.appendChild(txt('div', 'rm-read-order-path', fi.path));
      if (fi.reason) {
        body.appendChild(txt('div', 'rm-read-order-reason', fi.reason));
      }
      item.appendChild(body);

      ol.appendChild(item);
    });
    section.appendChild(ol);
    return section;
  }

  function renderFileList(title, files) {
    if (!files || !files.length) return null;

    var section = el('div');
    section.appendChild(txt('div', 'rm-section-title', title + ' (' + files.length + ')'));

    var ul = el('ul', 'rm-file-list');
    files.forEach(function (f) {
      var li = el('li');
      li.textContent = f.path;
      ul.appendChild(li);
    });
    section.appendChild(ul);
    return section;
  }

  function renderUnknowns(unknowns) {
    if (!unknowns || !unknowns.length) return null;

    var box = el('div', 'rm-info-box');
    var title = el('strong');
    title.textContent = LABELS.knownUnknowns + ':';
    box.appendChild(title);
    unknowns.forEach(function (u) {
      var p = txt('div', '', '• ' + u);
      p.style.cssText = 'margin-top:.25rem;font-size:.85rem;';
      box.appendChild(p);
    });
    return box;
  }

  function renderWarnings(warnings) {
    if (!warnings || !warnings.length) return null;

    var box = el('div', 'rm-warn-box');
    var title = el('strong');
    title.textContent = LABELS.warnings + ': ';
    box.appendChild(title);
    box.appendChild(document.createTextNode(warnings.join('; ')));
    return box;
  }

  function renderBundleStats(stats) {
    var container = el('div', 'rm-collapsible');

    var toggle = el('button', 'rm-collapsible-toggle');
    toggle.textContent = LABELS.collapsedDefault;
    toggle.onclick = function () {
      container.classList.toggle('rm-collapsible--open');
      toggle.textContent = container.classList.contains('rm-collapsible--open')
        ? LABELS.collapsedOpen : LABELS.collapsedDefault;
    };
    container.appendChild(toggle);

    var body = el('div', 'rm-collapsible-body');
    var list = [
      'Source files selected:   ' + stats.selected_files_count,
      'Test files selected:     ' + stats.selected_tests_count,
      'Docs selected:           ' + stats.selected_docs_count,
      'Packages selected:       ' + stats.selected_packages_count,
      'Related import edges:    ' + stats.related_edges_count,
    ];
    list.forEach(function (line) {
      var div = txt('div', '', line);
      div.style.cssText = 'font-size:.85rem;padding:.15rem 0;font-family:monospace;';
      body.appendChild(div);
    });
    container.appendChild(body);
    return container;
  }

  function renderErrorBox(error) {
    var box = el('div', 'rm-error-box');
    box.textContent = error;
    return box;
  }

  function renderPlaceholder(text) {
    return txt('div', 'rm-placeholder', text);
  }

  function renderFlowPage(flow) {
    var page = el('div');

    // Header
    var header = el('div', 'rm-flow-header');
    header.appendChild(txt('h2', '', flow.name || flow.id));
    header.appendChild(renderConfidenceBadge(flow.confidence));
    page.appendChild(header);

    // Error state
    if (flow.error) {
      page.appendChild(renderErrorBox(flow.error));
      if (flow.bundle_summary) {
        page.appendChild(renderBundleStats(flow.bundle_summary));
      }
      return page;
    }

    // Summary
    if (flow.summary) {
      var summary = txt('div', 'rm-summary', flow.summary);
      page.appendChild(summary);
    }

    // Chain (waterfall timeline)
    if (flow.likely_chain && flow.likely_chain.length > 0) {
      page.appendChild(renderChainSteps(flow.likely_chain));
    }

    // Known Unknowns
    var unknownsSection = renderUnknowns(flow.unknowns);
    if (unknownsSection) page.appendChild(unknownsSection);

    // Read Order -- the most important section
    if (flow.files_to_read_in_order && flow.files_to_read_in_order.length > 0) {
      page.appendChild(renderReadOrder(flow.files_to_read_in_order));
    } else if (!flow.error) {
      // If the flow has no error but also no AI analysis, show placeholder
      if (!flow.summary && !flow.likely_chain) {
        page.appendChild(renderPlaceholder(LABELS.noAIExplanation));
      }
    }

    // Tests
    var testsSection = renderFileList(LABELS.testsToRead, flow.tests_to_read);
    if (testsSection) page.appendChild(testsSection);

    // Warnings
    var warningsSection = renderWarnings(flow.warnings);
    if (warningsSection) page.appendChild(warningsSection);

    // Unverified paths
    if (flow.unverified_paths && flow.unverified_paths.length > 0) {
      var uvDiv = el('div');
      uvDiv.appendChild(txt('div', 'rm-section-title', LABELS.unverified));
      var tags = el('div', 'rm-tags');
      flow.unverified_paths.forEach(function (up) {
        var tag = el('span', 'rm-tag');
        tag.textContent = up.path;
        if (up.reason) tag.title = up.reason;
        tags.appendChild(tag);
      });
      uvDiv.appendChild(tags);
      page.appendChild(uvDiv);
    }

    // Technical Details (collapsed)
    if (flow.bundle_summary) {
      page.appendChild(renderBundleStats(flow.bundle_summary));
    }

    return page;
  }

  // ── Tab management ──────────────────────────────────────────────

  function showTab(id) {
    document.querySelectorAll('.rm-tab, .rm-tab-content').forEach(function (e) {
      e.classList.remove('rm-active');
    });
    var content = document.getElementById(id);
    if (content) content.classList.add('rm-active');
    var tab = document.querySelector('.rm-tab[data-tab="' + id + '"]');
    if (tab) tab.classList.add('rm-active');
  }

  // ── Main render ─────────────────────────────────────────────────

  function render() {
    // Header
    document.getElementById('rm-repo-name').textContent = DATA.repo_name || 'unknown';
    document.getElementById('rm-project-guess').textContent = DATA.project_guess || '';
    if (DATA.artifacts_dir) {
      document.getElementById('rm-artifacts-dir').textContent = 'Artifacts: ' + DATA.artifacts_dir;
    }

    // Tabs
    var tabs = document.getElementById('rm-tabs');
    var overviewTab = el('button', 'rm-tab rm-active');
    overviewTab.textContent = 'Overview';
    overviewTab.setAttribute('data-tab', 'rm-overview');
    overviewTab.onclick = function () { showTab('rm-overview'); };
    tabs.appendChild(overviewTab);

    DATA.flows.forEach(function (f) {
      var tab = el('button', 'rm-tab');
      if (f.error) tab.classList.add('rm-tab--error');
      tab.textContent = f.error ? ('⚠ ' + (f.name || f.id)) : (f.name || f.id);
      tab.setAttribute('data-tab', 'rm-flow-' + f.id);
      tab.onclick = function () { showTab('rm-flow-' + f.id); };
      tabs.appendChild(tab);
    });

    // Overview
    var overview = document.getElementById('rm-overview');
    var overviewHTML = el('div');

    if (!DATA.flows || DATA.flows.length === 0) {
      overviewHTML.appendChild(txt('div', 'rm-card', LABELS.noFlows));
    } else {
      var card = el('div', 'rm-card');
      card.appendChild(txt('h2', '', LABELS.candidateFlows));
      var grid = el('div', 'rm-overview-flows');

      DATA.flows.forEach(function (f) {
        var isRecommended = DATA.recommended_flow && f.id === DATA.recommended_flow;
        grid.appendChild(renderFlowCard(f, isRecommended));
      });

      card.appendChild(grid);
      overviewHTML.appendChild(card);

      // Quick Start
      if (DATA.flows.length > 0) {
        var qs = el('div', 'rm-quick-start');
        qs.appendChild(txt('div', 'rm-section-title', LABELS.quickStart));
        DATA.flows.forEach(function (f) {
          if (f.files_to_read_in_order && f.files_to_read_in_order.length > 0) {
            var item = el('div', 'rm-quick-start-item');
            var flowLink = el('span', 'rm-quick-start-flow');
            flowLink.textContent = f.name || f.id;
            flowLink.onclick = function () { showTab('rm-flow-' + f.id); };
            item.appendChild(flowLink);
            item.appendChild(txt('span', '', '→ ' + f.files_to_read_in_order[0].path));
            qs.appendChild(item);
          }
        });
        overviewHTML.appendChild(qs);
      }
    }

    // Global warnings
    if (DATA.warnings && DATA.warnings.length > 0) {
      overviewHTML.appendChild(renderWarnings(DATA.warnings));
    }

    overview.innerHTML = '';
    overview.appendChild(overviewHTML);

    // Flow pages
    var flowsContainer = document.getElementById('rm-flows-container');
    flowsContainer.innerHTML = '';
    DATA.flows.forEach(function (f) {
      var page = el('div', 'rm-tab-content');
      page.id = 'rm-flow-' + f.id;
      page.appendChild(renderFlowPage(f));
      flowsContainer.appendChild(page);
    });
  }

  window.addEventListener('DOMContentLoaded', render);
})();
