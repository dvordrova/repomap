(function () {
  'use strict';

  var DATA = JSON.parse(document.getElementById('rm-report-data').textContent);
  var architectureCanvasView = null;

  var LABELS = {
    purpose: 'Project purpose · model orientation',
    systemMap: 'System map · model orientation',
    startFiles: 'Start here',
    terms: 'Important terms',
    questions: 'Questions for a teammate',
    model: 'Model',
    localContext: 'Compact local context',
    externalRequest: 'Provider request bodies',
    providerRequests: 'Provider requests',
    providerLatency: 'Orientation latency',
    directionsFound: 'Directions',
    candidateFlows: 'Expanded directions',
    candidateDirections: 'Directions to explore',
    directionHint: 'Choose one model-suggested direction. A compact local evidence neighborhood is ready for each without another model call.',
    trigger: 'Trigger',
    likelyEntrypoint: 'Likely entrypoint',
    likelyFiles: 'Likely files',
    orientationEvidence: 'Why the model suggested this',
    filesToRead: 'Read order — open these files in sequence',
    testsToRead: 'Tests',
    executionChain: 'Execution chain',
    knownUnknowns: 'Known unknowns',
    unverified: 'Unverified paths',
    unknowns: 'Unknowns',
    warnings: 'Warnings',
    retrievalDetails: 'Retrieval details',
    noFlows: 'No candidate directions were produced.',
    startHere: 'Start here',
    suggestedStart: 'Suggested start',
    quickStart: 'Quick start',
    errorUnavailable: 'Analysis unavailable',
    localEvidence: 'Local evidence',
    openLocalEvidence: 'Explore focused local evidence →',
    localEvidenceIntro: 'Files for this model-suggested direction were selected locally from repository facts. No second model call was made; use them and the import edges as a grounded starting point, not as runtime proof.',
    localEvidenceLegacyIntro: 'This saved artifact contains a focused local evidence neighborhood. No completed model expansion is present; use these files and import edges as a starting point, not as runtime proof.',
    evidenceFiles: 'Evidence files',
    showAll: 'Show all ({count})',
    showMore: 'Show {count} more',
    showLess: 'Show less',
  };

  function esc(s) {
    if (!s) return '';
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  function evidenceClass(c) {
    var v = Math.max(0, c || 0);
    if (v <= 0) return 'rm-ev-none';
    return v >= 0.7 ? 'rm-ev-strong' : v >= 0.4 ? 'rm-ev-medium' : 'rm-ev-weak';
  }

  function evidenceLabel(c) {
    var v = Math.max(0, c || 0);
    if (v <= 0) return 'Model confidence: not estimated';
    if (v >= 0.7) return 'Model confidence: high';
    if (v >= 0.4) return 'Model confidence: medium';
    return 'Model confidence: low';
  }

  function chainCircleClass(c) {
    var v = Math.max(0, c || 0);
    if (v <= 0) return 'rm-chain-circle--none';
    return v >= 0.7 ? 'rm-chain-circle--hi' : v >= 0.4 ? 'rm-chain-circle--md' : 'rm-chain-circle--lo';
  }

  function kindClass(kind) {
    if (!kind) return '';
    return 'rm-kind rm-kind--' + kind;
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

  function formatBytes(value) {
    if (!value || value < 0) return '';
    if (value < 1024) return value + ' B';
    if (value < 1024 * 1024) return (value / 1024).toFixed(1) + ' KiB';
    return (value / (1024 * 1024)).toFixed(1) + ' MiB';
  }

  // ── Components ──────────────────────────────────────────────────

  function renderEvidenceBadge(confidence) {
    var badge = el('span', 'rm-evidence ' + evidenceClass(confidence));
    badge.textContent = evidenceLabel(confidence);
    return badge;
  }

  function renderPill(text, kind) {
    var pill = el('span', 'rm-pill rm-pill--' + (kind || 'accent'));
    pill.textContent = text;
    return pill;
  }

  function renderKindBadge(kind) {
    if (!kind) return null;
    var badge = el('span', 'rm-kind rm-kind--' + kind);
    badge.textContent = kind;
    return badge;
  }

  function renderFlowCard(flow, isRecommended) {
    var card = el('div', 'rm-ov-flow');
    if (isRecommended) card.classList.add('rm-ov-flow--recommended');

    if (flow.error) {
      card.classList.add('rm-ov-flow--error');
      var eh = el('div', 'rm-ov-flow-header');
      var eh3 = txt('h3', '', flow.name || flow.id);
      eh.appendChild(eh3);
      eh.appendChild(renderPill(LABELS.errorUnavailable, 'error'));
      card.appendChild(eh);
      if (flow.bundle_stats_label) {
        var meta = txt('div', 'rm-meta', flow.bundle_stats_label);
        card.appendChild(meta);
      }
      card.onclick = function () { showTab('rm-flow-' + flow.id); };
      return card;
    }

    var header = el('div', 'rm-ov-flow-header');
    var h3 = txt('h3', '', flow.name || flow.id);
    header.appendChild(h3);
    if (isRecommended) header.appendChild(renderPill(LABELS.startHere));
    if (flow.evidence_only) header.appendChild(renderPill(LABELS.localEvidence));
    card.appendChild(header);

    if (flow.summary) {
      var truncated = flow.summary.length > 100 ? flow.summary.slice(0, 100) + '...' : flow.summary;
      card.appendChild(txt('div', 'rm-summary-line', truncated));
    }

    var preview = el('div', 'rm-ov-flow-preview');
    var previewFiles = flow.files_to_read_in_order;
    if ((!previewFiles || previewFiles.length === 0) && flow.bundle_files) {
      previewFiles = flow.bundle_files;
    }
    if (previewFiles && previewFiles.length > 0) {
      previewFiles.slice(0, 3).forEach(function (fi) {
        var p = txt('div', 'rm-ov-flow-file', fi.path);
        preview.appendChild(p);
      });
    }
    card.appendChild(preview);

    var footer = el('div', 'rm-ov-flow-footer');
    if (!flow.evidence_only) footer.appendChild(renderEvidenceBadge(flow.confidence));
    if (flow.warnings && flow.warnings.length > 0) {
      footer.appendChild(txt('span', 'rm-ov-flow-stat', flow.warnings.length + ' warnings'));
    }
    if (flow.tests_to_read && flow.tests_to_read.length > 0) {
      footer.appendChild(txt('span', 'rm-ov-flow-stat', flow.tests_to_read.length + ' tests'));
    }
    card.appendChild(footer);

    card.onclick = function () { showTab('rm-flow-' + flow.id); };
    return card;
  }

  function candidateDirections() {
    if (DATA.candidate_directions && DATA.candidate_directions.length > 0) {
      return DATA.candidate_directions;
    }
    if (!DATA.candidate_flows) return [];
    return DATA.candidate_flows.map(function (candidate, index) {
      if (typeof candidate === 'string') {
        return { id: 'candidate-' + index, name: candidate };
      }
      return candidate;
    });
  }

  function renderDirectionField(label, value, code) {
    if (!value) return null;
    var row = el('div', 'rm-direction-field');
    row.appendChild(txt('div', 'rm-direction-label', label));
    row.appendChild(txt('div', code ? 'rm-direction-code' : 'rm-direction-value', value));
    return row;
  }

  function flowByID(id) {
    if (!id || !DATA.flows) return null;
    for (var i = 0; i < DATA.flows.length; i++) {
      if (DATA.flows[i].id === id) return DATA.flows[i];
    }
    return null;
  }

  function renderCandidateDirectionCard(direction, isSuggestedStart) {
    var card = el('div', 'rm-ov-flow rm-candidate-direction');

    var header = el('div', 'rm-ov-flow-header');
    header.appendChild(txt('h3', '', direction.name || direction.id));
    if (isSuggestedStart) header.appendChild(renderPill(LABELS.suggestedStart));
    header.appendChild(renderEvidenceBadge(direction.confidence));
    card.appendChild(header);

    if (direction.why_interesting) {
      card.appendChild(txt('div', 'rm-summary-line', direction.why_interesting));
    }

    var trigger = renderDirectionField(LABELS.trigger, direction.trigger, false);
    if (trigger) card.appendChild(trigger);

    var entrypoint = renderDirectionField(LABELS.likelyEntrypoint, direction.likely_entrypoint, true);
    if (entrypoint) card.appendChild(entrypoint);

    if (direction.likely_files && direction.likely_files.length > 0) {
      var files = el('div', 'rm-direction-field');
      files.appendChild(txt('div', 'rm-direction-label', LABELS.likelyFiles));
      direction.likely_files.forEach(function (path) {
        files.appendChild(txt('div', 'rm-direction-code', path));
      });
      card.appendChild(files);
    }

    if (direction.evidence && direction.evidence.length > 0) {
      var evidence = el('div', 'rm-direction-evidence');
      evidence.appendChild(txt('div', 'rm-direction-label', LABELS.orientationEvidence));
      direction.evidence.forEach(function (statement) {
        evidence.appendChild(txt('div', 'rm-direction-evidence-item', statement));
      });
      card.appendChild(evidence);
    }

    var focused = flowByID(direction.id);
    if (focused) {
      card.classList.add('rm-candidate-direction--clickable');
      card.setAttribute('role', 'button');
      card.setAttribute('tabindex', '0');
      card.appendChild(txt('div', 'rm-direction-action', LABELS.openLocalEvidence));
      var openFocused = function () { showTab('rm-flow-' + focused.id); };
      card.onclick = openFocused;
      card.onkeydown = function (event) {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault();
          openFocused();
        }
      };
    }

    return card;
  }

  function appendOrientationEvidence(container, evidence) {
    if (!evidence || evidence.length === 0) return;

    var evidenceBlock = el('div', 'rm-direction-evidence');
    evidenceBlock.appendChild(txt('div', 'rm-direction-label', LABELS.orientationEvidence));
    evidence.forEach(function (statement) {
      evidenceBlock.appendChild(txt('div', 'rm-direction-evidence-item', statement));
    });
    container.appendChild(evidenceBlock);
  }

  function renderPurposeCard() {
    var card = el('div', 'rm-card');
    var header = el('div', 'rm-flow-header');
    header.appendChild(txt('h2', '', LABELS.purpose));
    if (DATA.project_guess) {
      header.appendChild(renderEvidenceBadge(DATA.orientation_confidence));
    }
    card.appendChild(header);
    card.appendChild(txt(
      'div',
      DATA.project_guess ? 'rm-summary' : 'rm-placeholder',
      DATA.project_guess || 'No project purpose was produced.'
    ));

    if (DATA.run) {
      var facts = el('div', 'rm-run-facts');
      var addFact = function (label, value) {
        if (!value) return;
        var fact = el('div', 'rm-run-fact');
        fact.appendChild(txt('span', 'rm-run-fact-label', label));
        fact.appendChild(txt('span', 'rm-run-fact-value', value));
        facts.appendChild(fact);
      };
      addFact(LABELS.model, DATA.run.model);
      addFact(LABELS.localContext, formatBytes(DATA.run.compact_context_bytes));
      addFact(LABELS.externalRequest, formatBytes(DATA.run.external_request_bytes));
      if (DATA.run.provider_request_count) {
        addFact(LABELS.providerRequests, String(DATA.run.provider_request_count));
      }
      if (DATA.run.provider_latency_ms !== undefined && DATA.run.provider_latency_ms !== null) {
        addFact(LABELS.providerLatency, DATA.run.provider_latency_ms + ' ms');
      }
      if (DATA.run.candidate_direction_count) {
        addFact(LABELS.directionsFound, String(DATA.run.candidate_direction_count));
      }
      if (facts.children.length > 0) card.appendChild(facts);
    }

    var warnings = renderWarnings(DATA.warnings);
    if (warnings) card.appendChild(warnings);
    return card;
  }

  function renderSystemMapCard(items) {
    if (!items || items.length === 0) return null;

    var card = el('div', 'rm-card');
    card.appendChild(txt('h2', '', LABELS.systemMap));
    var grid = el('div', 'rm-overview-flows');
    items.forEach(function (item) {
      var subsystem = el('div', 'rm-ov-flow rm-candidate-direction');
      subsystem.appendChild(txt('h3', '', item.name));
      if (item.why_it_matters) {
        subsystem.appendChild(txt('div', 'rm-summary-line', item.why_it_matters));
      }
      appendOrientationEvidence(subsystem, item.evidence);
      grid.appendChild(subsystem);
    });
    card.appendChild(grid);
    return card;
  }

  function renderStartHereCard(files) {
    if (!files || files.length === 0) return null;

    var card = el('div', 'rm-card');
    card.appendChild(txt('h2', '', LABELS.startFiles));
    var list = el('div', 'rm-read-order');
    files.forEach(function (file, index) {
      list.appendChild(renderReadOrderItem(file, index + 1, file.priority || index + 1));
    });
    card.appendChild(list);
    return card;
  }

  function renderTermsCard(words) {
    if (!words || words.length === 0) return null;

    var card = el('div', 'rm-card');
    card.appendChild(txt('h2', '', LABELS.terms));
    var grid = el('div', 'rm-overview-flows');
    words.forEach(function (word) {
      var term = el('div', 'rm-ov-flow rm-candidate-direction');
      term.appendChild(txt('h3', '', word.word));
      if (word.guess) {
        term.appendChild(txt('div', 'rm-summary-line', word.guess));
      }
      appendOrientationEvidence(term, word.evidence);
      grid.appendChild(term);
    });
    card.appendChild(grid);
    return card;
  }

  function renderQuestionsCard(questions, unverifiedPaths) {
    var hasQuestions = questions && questions.length > 0;
    var hasUnverified = unverifiedPaths && unverifiedPaths.length > 0;
    if (!hasQuestions && !hasUnverified) return null;

    var card = el('div', 'rm-card');
    card.appendChild(txt('h2', '', LABELS.questions));
    if (hasQuestions) {
      var list = el('ul', 'rm-file-list');
      questions.forEach(function (question) {
        list.appendChild(txt('li', '', question));
      });
      card.appendChild(list);
    }
    var unverified = renderUnverifiedPaths(unverifiedPaths);
    if (unverified) card.appendChild(unverified);
    return card;
  }

  function renderDirectionsCard(directions, flows) {
    var card = el('div', 'rm-card');
    card.appendChild(txt('h2', '', LABELS.candidateDirections));
    if (directions.length === 0) {
      card.appendChild(renderPlaceholder(LABELS.noFlows));
    } else {
      card.appendChild(txt('p', 'rm-direction-hint', LABELS.directionHint));
      var directionsGrid = el('div', 'rm-overview-flows');
      var highestConfidence = Math.max.apply(null, directions.map(function (direction) {
        return direction.confidence || 0;
      }));
      directions.forEach(function (direction) {
        directionsGrid.appendChild(renderCandidateDirectionCard(
          direction,
          (direction.confidence || 0) === highestConfidence
        ));
      });
      card.appendChild(directionsGrid);
    }

    var expandedFlows = flows.filter(function (flow) { return !flow.evidence_only; });
    if (expandedFlows.length > 0) {
      card.appendChild(txt('div', 'rm-section-title', LABELS.candidateFlows));
      var flowsGrid = el('div', 'rm-overview-flows');
      expandedFlows.forEach(function (flow) {
        var isRecommended = DATA.recommended_flow && flow.id === DATA.recommended_flow;
        flowsGrid.appendChild(renderFlowCard(flow, isRecommended));
      });
      card.appendChild(flowsGrid);

      var quickStart = el('div', 'rm-quick-start');
      quickStart.appendChild(txt('div', 'rm-section-title', LABELS.quickStart));
      expandedFlows.forEach(function (flow) {
        if (!flow.files_to_read_in_order || flow.files_to_read_in_order.length === 0) return;

        var item = el('div', 'rm-quick-start-item');
        var flowLink = el('span', 'rm-quick-start-flow');
        flowLink.textContent = flow.name || flow.id;
        flowLink.onclick = function () { showTab('rm-flow-' + flow.id); };
        item.appendChild(flowLink);
        item.appendChild(txt('span', '', '→ ' + flow.files_to_read_in_order[0].path));
        quickStart.appendChild(item);
      });
      card.appendChild(quickStart);
    }

    return card;
  }

  function buildFileReasonIndex(filesToRead) {
    var idx = {};
    if (!filesToRead) return idx;
    filesToRead.forEach(function (fi) {
      if (fi.reason) idx[fi.path] = fi.reason;
    });
    return idx;
  }

  function renderChainSteps(chain, filesToRead) {
    var reasonIndex = buildFileReasonIndex(filesToRead);

    var section = el('div');
    section.appendChild(txt('div', 'rm-section-title', LABELS.executionChain));

    var container = el('div', 'rm-chain');

    chain.forEach(function (s) {
      var hasName = s.name && s.name.length > 0;
      var hasWhat = s.what_happens && s.what_happens.length > 0;
      var hasEvidence = s.evidence_files && s.evidence_files.length > 0;

      var stepWhat = s.what_happens;
      var stepName = s.name;

      if (!hasWhat && hasEvidence) {
        for (var i = 0; i < s.evidence_files.length; i++) {
          var r = reasonIndex[s.evidence_files[i]];
          if (r) { stepWhat = r; break; }
        }
      }
      if (!hasName && hasEvidence) {
        var fname = s.evidence_files[0];
        var last = fname.split('/').pop();
        stepName = last || fname;
      }

      if (!hasEvidence && !stepWhat) return;

      var step = el('div', 'rm-chain-step');

      var circle = el('div', 'rm-chain-circle ' + chainCircleClass(s.confidence));
      circle.textContent = s.step;
      step.appendChild(circle);

      var body = el('div', 'rm-chain-body');

      if (stepName) {
        body.appendChild(txt('div', 'rm-chain-name', 'Step ' + s.step + ': ' + stepName));
      } else {
        body.appendChild(txt('div', 'rm-chain-name', 'Step ' + s.step));
      }

      if (stepWhat) {
        body.appendChild(txt('div', 'rm-chain-desc', stepWhat));
      }

      if (hasEvidence) {
        var filesDiv = el('div', 'rm-chain-files');
        filesDiv.appendChild(txt('span', 'rm-chain-files-label', LABELS.evidenceFiles + ': '));
        s.evidence_files.forEach(function (ef) {
          var fileRow = el('div', 'rm-chain-file-row');
          var pathSpan = txt('span', 'rm-chain-file', ef);
          fileRow.appendChild(pathSpan);
          var eReason = reasonIndex[ef];
          if (eReason) {
            var reasonSpan = txt('span', 'rm-chain-file-reason', eReason);
            fileRow.appendChild(reasonSpan);
          }
          filesDiv.appendChild(fileRow);
        });
        body.appendChild(filesDiv);
      }

      step.appendChild(body);
      container.appendChild(step);
    });

    section.appendChild(container);
    return section;
  }

  function renderReadOrder(files, maxShow) {
    if (!files || !files.length) return null;

    var section = el('div');
    section.appendChild(txt('div', 'rm-section-title', LABELS.filesToRead + ' (' + files.length + ')'));

    var limit = maxShow || files.length;
    var visible = files.slice(0, limit);
    var hidden = files.slice(limit);

    var ol = el('div', 'rm-read-order');
    visible.forEach(function (fi, i) {
      ol.appendChild(renderReadOrderItem(fi, i + 1, fi.priority));
    });
    section.appendChild(ol);

    if (hidden.length > 0) {
      var expandDiv = el('div', 'rm-read-order-expand');
      var btn = el('button', 'rm-expand-btn');
      btn.textContent = LABELS.showMore.replace('{count}', hidden.length);
      var expanded = false;
      btn.onclick = function () {
        if (expanded) {
          while (ol.children.length > limit) ol.removeChild(ol.lastChild);
          btn.textContent = LABELS.showMore.replace('{count}', hidden.length);
        } else {
          hidden.forEach(function (fi, j) {
            ol.appendChild(renderReadOrderItem(fi, limit + j + 1, fi.priority));
          });
          btn.textContent = LABELS.showLess;
        }
        expanded = !expanded;
      };
      expandDiv.appendChild(btn);
      section.appendChild(expandDiv);
    }

    return section;
  }

  function renderReadOrderItem(fi, num, priority) {
    var item = el('div', 'rm-read-order-item');

    var numEl = el('div', 'rm-read-order-num');
    if (priority >= 3) numEl.classList.add('rm-read-order-num--p3');
    else if (priority >= 2) numEl.classList.add('rm-read-order-num--p2');
    numEl.textContent = num;
    item.appendChild(numEl);

    var body = el('div', 'rm-read-order-body');
    var pathRow = el('div', 'rm-read-order-path-row');
    pathRow.appendChild(txt('span', 'rm-read-order-path', fi.path));
    if (fi.kind) pathRow.appendChild(renderKindBadge(fi.kind));
    body.appendChild(pathRow);
    if (fi.reason) {
      body.appendChild(txt('div', 'rm-read-order-reason', fi.reason));
    }
    item.appendChild(body);

    return item;
  }

  function renderFileList(title, files) {
    if (!files || !files.length) return null;

    var section = el('div');
    section.appendChild(txt('div', 'rm-section-title', title + ' (' + files.length + ')'));

    var ul = el('ul', 'rm-file-list');
    files.forEach(function (f) {
      var li = el('li', 'rm-file-list-item');
      var pathSpan = txt('span', 'rm-file-path', f.path);
      li.appendChild(pathSpan);
      if (f.reason) {
        li.appendChild(txt('span', 'rm-file-reason', f.reason));
      }
      ul.appendChild(li);
    });
    section.appendChild(ul);
    return section;
  }

  function renderExpandableList(title, items, boxClass, maxShow) {
    if (!items || !items.length) return null;
    var limit = maxShow || 3;
    var visible = items.slice(0, limit);
    var hidden = items.slice(limit);

    var box = el('div', boxClass);
    var titleEl = el('strong');
    titleEl.textContent = title + ': ';
    box.appendChild(titleEl);

    visible.forEach(function (u, i) {
      var p = el('div');
      p.className = 'rm-exp-item';
      p.textContent = '• ' + u;
      box.appendChild(p);
    });

    if (hidden.length > 0) {
      var hiddenDiv = el('div', 'rm-exp-hidden');
      hidden.forEach(function (u) {
        var p = el('div');
        p.className = 'rm-exp-item';
        p.textContent = '• ' + u;
        hiddenDiv.appendChild(p);
      });
      box.appendChild(hiddenDiv);

      var btn = el('button', 'rm-expand-btn-inline');
      btn.textContent = LABELS.showMore.replace('{count}', hidden.length);
      btn.onclick = function () {
        var showing = hiddenDiv.style.display === 'block';
        hiddenDiv.style.display = showing ? 'none' : 'block';
        btn.textContent = showing ? LABELS.showMore.replace('{count}', hidden.length) : LABELS.showLess;
      };
      box.appendChild(btn);
    }

    return box;
  }

  function renderUnknowns(unknowns) {
    return renderExpandableList(LABELS.knownUnknowns, unknowns, 'rm-info-box', 3);
  }

  function renderWarnings(warnings) {
    return renderExpandableList(LABELS.warnings, warnings, 'rm-warn-box', 3);
  }

  function renderBundleStats(stats) {
    var container = el('div', 'rm-collapsible');

    var toggle = el('button', 'rm-collapsible-toggle');
    toggle.textContent = LABELS.retrievalDetails;
    toggle.onclick = function () {
      container.classList.toggle('rm-collapsible--open');
    };
    container.appendChild(toggle);

    var body = el('div', 'rm-collapsible-body');
    var list = [
      'Source files selected: ' + stats.selected_files_count,
      'Test files selected:   ' + stats.selected_tests_count,
      'Docs selected:         ' + stats.selected_docs_count,
      'Packages selected:     ' + stats.selected_packages_count,
      'Related import edges:  ' + stats.related_edges_count,
    ];
    list.forEach(function (line) {
      var div = txt('div', 'rm-bundle-stat', line);
      body.appendChild(div);
    });
    container.appendChild(body);
    return container;
  }

  function renderStringList(title, items) {
    if (!items || !items.length) return null;

    var section = el('div');
    section.appendChild(txt('div', 'rm-section-title', title + ' (' + items.length + ')'));

    var ul = el('ul', 'rm-file-list');
    items.forEach(function (s) {
      var li = el('li');
      li.textContent = s;
      ul.appendChild(li);
    });
    section.appendChild(ul);
    return section;
  }

  function renderEdgeList(title, edges) {
    if (!edges || !edges.length) return null;

    var section = el('div');
    section.appendChild(txt('div', 'rm-section-title', title + ' (' + edges.length + ')'));

    var ul = el('ul', 'rm-file-list');
    edges.forEach(function (e) {
      var li = el('li');
      li.textContent = e.from + ' → ' + e.to;
      ul.appendChild(li);
    });
    section.appendChild(ul);
    return section;
  }

  function renderErrorBox(error) {
    var box = el('div', 'rm-error-box');
    box.textContent = error;
    return box;
  }

  function renderCollapsedError(error) {
    var container = el('div', 'rm-collapsible');

    var toggle = el('button', 'rm-collapsible-toggle');
    toggle.textContent = 'Show parse error';
    toggle.onclick = function () {
      container.classList.toggle('rm-collapsible--open');
    };
    container.appendChild(toggle);

    var body = el('div', 'rm-collapsible-body');
    body.appendChild(renderErrorBox(error));
    container.appendChild(body);
    return container;
  }

  function renderUnverifiedPaths(paths) {
    if (!paths || !paths.length) return null;

    var section = el('div');
    section.appendChild(txt('div', 'rm-section-title', LABELS.unverified));
    var tags = el('div', 'rm-tags');
    paths.forEach(function (up) {
      var tag = el('span', 'rm-tag');
      tag.textContent = up.path;
      if (up.reason) tag.title = up.reason;
      tags.appendChild(tag);
    });
    section.appendChild(tags);
    return section;
  }

  function renderFlowPage(flow) {
    var page = el('div');

    var header = el('div', 'rm-flow-header');
    header.appendChild(txt('h2', '', flow.name || flow.id));
    if (!flow.error && !flow.evidence_only) {
      header.appendChild(renderEvidenceBadge(flow.confidence));
    }
    if (flow.evidence_only) header.appendChild(renderPill(LABELS.localEvidence));
    page.appendChild(header);

    if (flow.error) {
      page.appendChild(txt('div', 'rm-fallback-heading', 'Flow explanation failed, but local context was collected'));

      var filesSection = renderFileList('Files selected by deterministic retrieval', flow.bundle_files);
      if (filesSection) page.appendChild(filesSection);

      var testsSection = renderFileList('Tests selected by deterministic retrieval', flow.bundle_tests);
      if (testsSection) page.appendChild(testsSection);

      var docsSection = renderFileList('Docs selected', flow.bundle_docs);
      if (docsSection) page.appendChild(docsSection);

      var pkgsSection = renderStringList('Packages selected', flow.bundle_packages);
      if (pkgsSection) page.appendChild(pkgsSection);

      var edgesSection = renderEdgeList('Related import edges', flow.bundle_edges);
      if (edgesSection) page.appendChild(edgesSection);

      if (flow.bundle_summary) {
        page.appendChild(renderBundleStats(flow.bundle_summary));
      }

      page.appendChild(renderCollapsedError(flow.error));
      return page;
    }

    if (flow.evidence_only) {
      page.appendChild(txt(
        'div',
        'rm-summary',
        flow.flow_status === 'local_only' ? LABELS.localEvidenceIntro : LABELS.localEvidenceLegacyIntro
      ));

      if (flow.bundle_files && flow.bundle_files.length > 0) {
        page.appendChild(renderReadOrder(flow.bundle_files, 5));
      }

      var localTests = renderFileList('Tests selected locally', (flow.bundle_tests || []).slice(0, 8));
      if (localTests) page.appendChild(localTests);

      var localDocs = renderFileList('Docs selected locally', (flow.bundle_docs || []).slice(0, 8));
      if (localDocs) page.appendChild(localDocs);

      var localPackages = renderStringList('Packages in this focused neighborhood', (flow.bundle_packages || []).slice(0, 12));
      if (localPackages) page.appendChild(localPackages);

      var localEdges = renderEdgeList('Related import edges', (flow.bundle_edges || []).slice(0, 12));
      if (localEdges) page.appendChild(localEdges);

      if (flow.bundle_summary) page.appendChild(renderBundleStats(flow.bundle_summary));
      return page;
    }

    if (flow.summary) {
      page.appendChild(txt('div', 'rm-summary', flow.summary));
    }

    if (flow.files_to_read_in_order && flow.files_to_read_in_order.length > 0) {
      page.appendChild(renderReadOrder(flow.files_to_read_in_order, 5));
    }

    if (flow.likely_chain && flow.likely_chain.length > 0) {
      var chainSection = renderChainSteps(flow.likely_chain, flow.files_to_read_in_order);
      if (chainSection.children.length > 1) {
        page.appendChild(chainSection);
      }
    }

    var testsSection = renderFileList(LABELS.testsToRead, flow.tests_to_read);
    if (testsSection) page.appendChild(testsSection);

    var unknownsSection = renderUnknowns(flow.unknowns);
    if (unknownsSection) page.appendChild(unknownsSection);

    var warningsSection = renderWarnings(flow.warnings);
    if (warningsSection) page.appendChild(warningsSection);

    var uvSection = renderUnverifiedPaths(flow.unverified_paths);
    if (uvSection) page.appendChild(uvSection);

    if (flow.bundle_summary) {
      page.appendChild(renderBundleStats(flow.bundle_summary));
    }

    return page;
  }

  function renderPlaceholder(text) {
    return txt('div', 'rm-placeholder', text);
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
    DATA.flows = DATA.flows || [];
    document.getElementById('rm-repo-name').textContent = DATA.repo_name || 'unknown';
    if (DATA.artifacts_dir) {
      document.getElementById('rm-artifacts-dir').textContent = 'Artifacts: ' + DATA.artifacts_dir;
    }
    if (DATA.feedback_path) {
      document.getElementById('rm-feedback-path').textContent = 'Feedback notes: ' + DATA.feedback_path;
    }

    var tabs = document.getElementById('rm-tabs');
    var overviewTab = el('button', 'rm-tab rm-active');
    overviewTab.textContent = 'Overview';
    overviewTab.setAttribute('data-tab', 'rm-overview');
    overviewTab.onclick = function () { showTab('rm-overview'); };
    tabs.appendChild(overviewTab);

    DATA.flows.forEach(function (f) {
      var tab = el('button', 'rm-tab');
      if (f.error) {
        tab.classList.add('rm-tab--error');
        tab.textContent = f.name || f.id;
      } else if (f.evidence_only) {
        tab.textContent = (f.name || f.id) + ' · local evidence';
      } else {
        var parts = [f.name || f.id];
        if (f.warnings && f.warnings.length > 0) {
          tab.classList.add('rm-tab--warn');
          parts.push(f.warnings.length + ' warning' + (f.warnings.length > 1 ? 's' : ''));
        }
        if (f.unverified_paths && f.unverified_paths.length > 0) {
          tab.classList.add('rm-tab--partial');
          parts.push('partial');
        }
        tab.textContent = parts.join(' · ');
      }
      tab.setAttribute('data-tab', 'rm-flow-' + f.id);
      tab.onclick = function () { showTab('rm-flow-' + f.id); };
      tabs.appendChild(tab);
    });

    var overview = document.getElementById('rm-overview');
    var overviewHTML = el('div');

    overviewHTML.appendChild(renderPurposeCard());

    var architectureCanvasHost = null;
    if (DATA.architecture_canvas && window.RepomapArchitectureCanvas) {
      var architectureCard = el('section', 'rm-card rm-architecture-canvas-card');
      var architectureHeading = el('div', 'rm-architecture-canvas-heading');
      architectureHeading.appendChild(txt('h2', '', 'Architecture & flows'));
      architectureHeading.appendChild(txt('p', '', 'Select a component or one saved flow, then challenge each step through exact evidence.'));
      architectureCard.appendChild(architectureHeading);
      architectureCanvasHost = el('div', 'rm-architecture-canvas-host');
      architectureCard.appendChild(architectureCanvasHost);
      overviewHTML.appendChild(architectureCard);
    } else {
      var systemMap = renderSystemMapCard(DATA.high_level_map);
      if (systemMap) overviewHTML.appendChild(systemMap);

      var directions = candidateDirections();
      overviewHTML.appendChild(renderDirectionsCard(directions, DATA.flows));
    }

    var startHere = renderStartHereCard(DATA.first_files_to_open);
    if (startHere) overviewHTML.appendChild(startHere);

    var terms = renderTermsCard(DATA.important_domain_words);
    if (terms) overviewHTML.appendChild(terms);

    var questions = renderQuestionsCard(DATA.questions_for_human, DATA.unverified_paths);
    if (questions) overviewHTML.appendChild(questions);

    overview.innerHTML = '';
    overview.appendChild(overviewHTML);

    if (architectureCanvasView) {
      architectureCanvasView.destroy();
      architectureCanvasView = null;
    }
    if (architectureCanvasHost) {
      architectureCanvasView = window.RepomapArchitectureCanvas.mount(
        architectureCanvasHost,
        DATA.architecture_canvas,
        {}
      );
    }

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
