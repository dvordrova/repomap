(function () {
  'use strict';

  var state = null;
  var toastTimer = 0;

  function object(value, label) {
    if (!value || typeof value !== 'object' || Array.isArray(value)) {
      throw new Error(label + ' must be an object');
    }
    return value;
  }

  function array(value, label) {
    if (!Array.isArray(value)) throw new Error(label + ' must be an array');
    return value;
  }

  function text(value, label) {
    if (typeof value !== 'string' || value.trim() !== value || !value) {
      throw new Error(label + ' must be exact non-empty text');
    }
    return value;
  }

  function optionalText(value, label) {
    if (value == null || value === '') return '';
    return text(value, label);
  }

  function integer(value, label) {
    if (!Number.isSafeInteger(value) || value < 0) throw new Error(label + ' must be a non-negative integer');
    return value;
  }

  function element(tag, className, value) {
    var node = document.createElement(tag);
    if (className) node.className = className;
    if (value != null) node.textContent = String(value);
    return node;
  }

  function appendText(parent, tag, className, value) {
    var node = element(tag, className, value);
    parent.appendChild(node);
    return node;
  }

  function readPayload() {
    var node = document.getElementById('rm-report-data');
    if (!node) throw new Error('The embedded report payload is missing.');
    var value;
    try { value = JSON.parse(node.textContent || ''); } catch (error) {
      throw new Error('The embedded report payload is not valid JSON.');
    }
    return object(value, 'report');
  }

  function exactLocation(value, label) {
    value = object(value, label);
    var path = text(value.path, label + '.path');
    var line = Object.prototype.hasOwnProperty.call(value, 'line') ? integer(value.line, label + '.line') : 0;
    var column = Object.prototype.hasOwnProperty.call(value, 'column') ? integer(value.column, label + '.column') : 0;
    return { path: path, line: line, column: column };
  }

  function flattenBlocks(values, depth, result, seen) {
    array(values, 'core blocks').forEach(function (raw) {
      var block = object(raw, 'core block');
      var id = text(block.id, 'core block.id');
      if (seen[id]) throw new Error('Core responsibility identities are not unique.');
      seen[id] = true;
      var files = array(block.files, 'core block.files').map(function (file) {
        file = object(file, 'core file');
        return { fileRef: text(file.file_ref, 'core file.file_ref'), path: text(file.path, 'core file.path') };
      });
      var symbols = array(block.representative_symbols, 'core block.representative_symbols').map(function (rawSymbol) {
        rawSymbol = object(rawSymbol, 'representative symbol');
        var symbol = object(rawSymbol.symbol, 'representative symbol.symbol');
        return {
          id: text(symbol.node_id, 'representative symbol.node_id'),
          name: text(symbol.name, 'representative symbol.name'),
          packageName: text(symbol.package, 'representative symbol.package'),
          location: exactLocation(symbol.location, 'representative symbol.location'),
          kind: text(rawSymbol.kind, 'representative symbol.kind'),
          visibility: text(rawSymbol.visibility, 'representative symbol.visibility'),
          unresolvedOutgoing: integer(rawSymbol.unresolved_outgoing, 'representative symbol.unresolved_outgoing')
        };
      });
      var projected = {
        id: id,
        name: text(block.name, 'core block.name'),
        purpose: text(block.purpose, 'core block.purpose'),
        files: files,
        symbols: symbols,
        depth: depth
      };
      result.push(projected);
      flattenBlocks(array(block.children, 'core block.children'), depth + 1, result, seen);
    });
  }

  function buildPresentationModel(data) {
    text(data.repo_name, 'repo_name');
    integer(data.format_version, 'format_version');
    var portfolio = object(data.program_portfolio, 'program_portfolio');
    var entries = array(portfolio.entries, 'program_portfolio.entries');
    var defaultID = text(portfolio.default_target_id, 'program_portfolio.default_target_id');
    var defaultEntry = null;
    entries.forEach(function (rawEntry) {
      var entry = object(rawEntry, 'program portfolio entry');
      var target = object(entry.target, 'program target');
      if (target.id === defaultID) defaultEntry = entry;
    });
    if (!defaultEntry) throw new Error('The default program target is missing.');

    var target = object(defaultEntry.target, 'default program target');
    var view = object(defaultEntry.view, 'default program view');
    if (view.target_id !== target.id) throw new Error('The default program view does not match its target.');

    var objectsByID = Object.create(null);
    array(view.objects, 'program view.objects').forEach(function (rawObject) {
      rawObject = object(rawObject, 'program object');
      var id = text(rawObject.id, 'program object.id');
      if (objectsByID[id]) throw new Error('Program object identities are not unique.');
      objectsByID[id] = rawObject;
    });
    var relations = array(view.relations, 'program view.relations');

    var core = data.core_map_view == null ? null : object(data.core_map_view, 'core_map_view');
    var blocks = [];
    if (core) {
      if (core.program_target_id !== target.id) throw new Error('CoreMap does not match the default target.');
      flattenBlocks(array(core.refined_core, 'core_map_view.refined_core'), 0, blocks, Object.create(null));
    }

    var activityByID = Object.create(null);
    var activities = [];
    if (data.activity_entrypoint_view != null) {
      var activity = object(data.activity_entrypoint_view, 'activity_entrypoint_view');
      if (activity.program_target_id !== target.id) throw new Error('Activity entrypoints do not match the default target.');
      array(activity.entrypoints, 'activity entrypoints').forEach(function (rawStart) {
        rawStart = object(rawStart, 'activity entrypoint');
        var id = text(rawStart.object_id, 'activity entrypoint.object_id');
        if (activityByID[id]) throw new Error('Activity entrypoint identities are not unique.');
        var start = {
          id: id,
          name: text(rawStart.name, 'activity entrypoint.name'),
          signature: optionalText(rawStart.signature, 'activity entrypoint.signature'),
          kind: text(rawStart.kind, 'activity entrypoint.kind'),
          location: exactLocation(rawStart.location, 'activity entrypoint.location')
        };
        activityByID[id] = start;
        activities.push(start);
      });
    }

    var integrations = [];
    if (data.integration_usage_view != null) {
      var usageView = object(data.integration_usage_view, 'integration_usage_view');
      if (usageView.program_target_id !== target.id) throw new Error('Integration usage does not match the default target.');
      var integrationIDs = Object.create(null);
      array(usageView.dependencies, 'integration dependencies').forEach(function (rawDependency) {
        rawDependency = object(rawDependency, 'integration dependency');
        var dependencyID = text(rawDependency.dependency_id, 'integration dependency.dependency_id');
        if (integrationIDs[dependencyID]) throw new Error('Integration dependency identities are not unique.');
        integrationIDs[dependencyID] = true;
        var uses = array(rawDependency.uses, 'integration dependency.uses').map(function (rawUse) {
          rawUse = object(rawUse, 'integration use');
          return {
            callerID: text(rawUse.caller_id, 'integration use.caller_id'),
            callerName: text(rawUse.caller_name, 'integration use.caller_name'),
            callsite: exactLocation(rawUse.callsite, 'integration use.callsite'),
            callee: text(rawUse.canonical_callee, 'integration use.canonical_callee'),
            label: text(rawUse.label, 'integration use.label'),
            mechanism: text(rawUse.mechanism, 'integration use.mechanism'),
            authority: text(rawUse.authority, 'integration use.authority')
          };
        });
        integrations.push({
          id: dependencyID,
          name: text(rawDependency.name, 'integration dependency.name'),
          kind: text(rawDependency.kind, 'integration dependency.kind'),
          packagePath: text(rawDependency.package_path, 'integration dependency.package_path'),
          modulePath: optionalText(rawDependency.module_path, 'integration dependency.module_path'),
          uses: uses
        });
      });
    }

    var blocksByID = Object.create(null);
    var blocksBySymbol = Object.create(null);
    blocks.forEach(function (block) {
      blocksByID[block.id] = block;
      block.symbols.forEach(function (symbol) {
        if (!blocksBySymbol[symbol.id]) blocksBySymbol[symbol.id] = [];
        blocksBySymbol[symbol.id].push(block.id);
      });
    });

    var openable = Object.create(null);
    array(data.openable_paths, 'openable_paths').forEach(function (path) {
      path = text(path, 'openable path');
      openable[path] = true;
    });

    return {
      raw: data,
      repoName: data.repo_name,
      revision: text(data.captured_revision, 'captured_revision'),
      capturedInputs: integer(data.captured_input_count, 'captured_input_count'),
      target: {
        id: text(target.id, 'program target.id'),
        language: text(target.language, 'program target.language'),
        kind: text(target.kind, 'program target.kind'),
        name: text(target.name, 'program target.name'),
        selector: text(target.selector, 'program target.selector'),
        sources: array(target.sources, 'program target.sources')
      },
      blocks: blocks,
      blocksByID: blocksByID,
      blocksBySymbol: blocksBySymbol,
      activityByID: activityByID,
      activities: activities,
      integrations: integrations,
      objectsByID: objectsByID,
      relations: relations,
      openable: openable,
      coreCoverage: core ? object(core.coverage, 'core_map_view.coverage') : null,
      warnings: data.warnings == null ? [] : array(data.warnings, 'warnings')
    };
  }

  function encodePathSegment(value) {
    return encodeURIComponent(value).replace(/[!'()*]/g, function (character) {
      return '%' + character.charCodeAt(0).toString(16).toUpperCase();
    });
  }

  function serverCoordinates() {
    var baseMatch = window.location.pathname.match(/^(\/_repomap\/[A-Za-z0-9_-]+)(?:\/|$)/);
    var loopback = window.location.hostname === '127.0.0.1' || window.location.hostname === 'localhost' ||
      window.location.hostname === '::1';
    if (!loopback || window.location.protocol !== 'http:' || !baseMatch) return null;
    var escaped = baseMatch[1].replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    var runMatch = window.location.pathname.match(new RegExp(
      '^' + escaped + '/runs/([A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?)/report\\.html$'
    ));
    if (!runMatch) return null;
    return { base: baseMatch[1], runID: runMatch[1] };
  }

  function buildSourceAuthority(data, model) {
    var github = data.github_source_links == null ? null : object(data.github_source_links, 'github_source_links');
    var gitlab = data.gitlab_source_links == null ? null : object(data.gitlab_source_links, 'gitlab_source_links');
    if (github && gitlab) throw new Error('The report has multiple source authorities.');
    var links = github || gitlab;
    if (links) {
      var repositoryURL = text(links.repository_url, 'repository_url').replace(/\/+$/g, '');
      var parsed = new URL(repositoryURL);
      if ((parsed.protocol !== 'https:' && parsed.protocol !== 'http:') || parsed.username || parsed.password ||
          parsed.search || parsed.hash) throw new Error('The source repository URL is unsafe.');
      var revision = text(links.revision, 'source revision');
      if (revision !== model.revision) throw new Error('The source authority revision does not match the report.');
      return {
        mode: 'static',
        host: github ? 'GitHub' : 'GitLab',
        repositoryURL: repositoryURL,
        revision: revision,
        pathPrefix: links.path_prefix || ''
      };
    }
    var ids = data.source_ids == null ? null : object(data.source_ids, 'source_ids');
    var server = serverCoordinates();
    if (!ids || !server) throw new Error('No exact source-opening authority is available.');
    return { mode: 'served', ids: ids, server: server };
  }

  function staticSourceURL(location) {
    var authority = state.source;
    var parts = [];
    var prefix = authority.pathPrefix.replace(/^\/+|\/+$/g, '');
    if (prefix) parts = parts.concat(prefix.split('/'));
    parts = parts.concat(location.path.split('/'));
    if (parts.some(function (part) { return !part || part === '.' || part === '..'; })) {
      throw new Error('The source path is not canonical.');
    }
    var blob = authority.host === 'GitHub' ? '/blob/' : '/-/blob/';
    var url = authority.repositoryURL + blob + encodePathSegment(authority.revision) + '/' +
      parts.map(encodePathSegment).join('/');
    if (location.line > 0) url += '#L' + String(location.line);
    return url;
  }

  function sourceAction(label, rawLocation) {
    var location = exactLocation(rawLocation, 'source action location');
    if (!state.model.openable[location.path]) throw new Error('Source evidence is outside publication authority.');
    var control;
    if (state.source.mode === 'static') {
      control = element('a', 'rm-source-action');
      control.href = staticSourceURL(location);
      control.target = '_blank';
      control.rel = 'noopener noreferrer';
      control.title = 'Open the exact captured revision in ' + state.source.host;
    } else {
      var sourceID = state.source.ids[location.path];
      if (typeof sourceID !== 'string' || !sourceID) throw new Error('Source evidence lacks a manifest source ID.');
      control = element('button', 'rm-source-action');
      control.type = 'button';
      control.addEventListener('click', function () {
        requestOpenSource(sourceID, location).catch(function (error) {
          showToast(error && error.message ? error.message : String(error), true);
        });
      });
    }
    appendText(control, 'span', 'rm-source-action__name', label);
    appendText(control, 'span', 'rm-source-action__location', formatLocation(location));
    return control;
  }

  function requestOpenSource(sourceID, location) {
    return fetch(state.source.server.base + '/api/open', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Repomap-Action': 'open-file' },
      body: JSON.stringify({
        run_id: state.source.server.runID,
        source_id: sourceID,
        line: location.line,
        column: location.column
      })
    }).then(function (response) {
      return response.json().catch(function () { throw new Error('The editor response was not valid JSON.'); })
        .then(function (body) {
          body = object(body, 'open-file response');
          if (!response.ok) throw new Error(text(body.error, 'open-file error'));
          if (body.status !== 'opened') throw new Error('The editor did not confirm the source action.');
          showToast('Opened the exact source location in VS Code.', false);
        });
    });
  }

  function formatLocation(location) {
    return location.path + (location.line > 0 ? ':' + String(location.line) : '');
  }

  function humanRelation(kind) {
    var labels = {
      calls: 'calls',
      invokes_external: 'crosses an external boundary to',
      implements: 'implements',
      decorates: 'decorates',
      passes_callback: 'passes control to',
      executes: 'executes',
      reads: 'reads',
      writes: 'writes'
    };
    return labels[kind] || kind.replace(/_/g, ' ');
  }

  function humanIntegrationAuthority(value) {
    if (value === 'exact_external_symbol') return 'exact external symbol';
    if (value === 'syntactic_unresolved') return 'selected callsite; runtime unresolved';
    return value.replace(/_/g, ' ');
  }

  function relationLocation(relation) {
    if (relation.location) return exactLocation(relation.location, 'relation.location');
    var witnesses = Array.isArray(relation.witnesses) ? relation.witnesses : [];
    for (var index = 0; index < witnesses.length; index++) {
      if (witnesses[index] && witnesses[index].location) {
        return exactLocation(witnesses[index].location, 'relation witness location');
      }
    }
    return null;
  }

  function connectionsFor(block) {
    var selected = Object.create(null);
    block.symbols.forEach(function (symbol) { selected[symbol.id] = symbol; });
    var connections = [];
    state.model.relations.forEach(function (rawRelation) {
      var relation = object(rawRelation, 'program relation');
      if (!selected[relation.from_id] || ['contains', 'imports', 'sources'].indexOf(relation.kind) >= 0) return;
      var from = state.model.objectsByID[relation.from_id];
      if (!from) return;
      var targetNames = array(relation.to_ids, 'program relation.to_ids').map(function (id) {
        var target = state.model.objectsByID[id];
        return target ? target.name : '';
      }).filter(Boolean);
      connections.push({
        id: text(relation.id, 'program relation.id'),
        from: text(from.name, 'connection source name'),
        to: targetNames.length ? targetNames.join(' / ') : 'runtime target unresolved',
        kind: text(relation.kind, 'program relation.kind'),
        resolution: text(relation.resolution, 'program relation.resolution'),
        location: relationLocation(relation),
        targetIDs: relation.to_ids
      });
    });
    var rank = { exact: 0, alternatives: 1, unresolved: 2 };
    connections.sort(function (left, right) {
      return (rank[left.resolution] || 0) - (rank[right.resolution] || 0) ||
        left.from.localeCompare(right.from) || left.to.localeCompare(right.to);
    });
    return connections.slice(0, 7);
  }

  function relatedBlocksFor(block, connections) {
    var ids = Object.create(null);
    block.symbols.forEach(function (symbol) {
      (state.model.blocksBySymbol[symbol.id] || []).forEach(function (blockID) {
        if (blockID !== block.id) ids[blockID] = true;
      });
    });
    connections.forEach(function (connection) {
      connection.targetIDs.forEach(function (targetID) {
        (state.model.blocksBySymbol[targetID] || []).forEach(function (blockID) {
          if (blockID !== block.id) ids[blockID] = true;
        });
      });
    });
    return Object.keys(ids).map(function (id) { return state.model.blocksByID[id]; }).filter(Boolean).slice(0, 4);
  }

  function canvasTopology() {
    var starts = state.model.activities.filter(function (start) {
      return (state.model.blocksBySymbol[start.id] || []).length > 0;
    });
    var edges = [];
    var edgeKeys = Object.create(null);

    starts.forEach(function (start) {
      state.model.blocksBySymbol[start.id].forEach(function (blockID) {
        var key = 'entry:' + start.id + '->core:' + blockID;
        if (edgeKeys[key]) return;
        edgeKeys[key] = true;
        edges.push({
          from: 'entry:' + start.id,
          to: 'core:' + blockID,
          resolution: 'exact',
          description: start.name + ' participates in ' + state.model.blocksByID[blockID].name
        });
      });
    });

    state.model.integrations.forEach(function (integration) {
      integration.uses.forEach(function (use) {
        (state.model.blocksBySymbol[use.callerID] || []).forEach(function (blockID) {
          var key = 'core:' + blockID + '->integration:' + integration.id;
          var existing = edgeKeys[key];
          var resolution = use.authority === 'exact_external_symbol' ? 'exact' : 'runtime';
          if (existing) {
            if (resolution === 'runtime') {
              existing.resolution = 'runtime';
              existing.description = state.model.blocksByID[blockID].name +
                ' has selected callsites to ' + integration.name + '; includes runtime-unresolved evidence';
            }
            return;
          }
          var edge = {
            from: 'core:' + blockID,
            to: 'integration:' + integration.id,
            resolution: resolution,
            description: state.model.blocksByID[blockID].name + ' has a selected callsite to ' +
              integration.name + '; ' + humanIntegrationAuthority(use.authority)
          };
          edgeKeys[key] = edge;
          edges.push(edge);
        });
      });
    });

    return {
      starts: starts,
      integrations: state.model.integrations,
      edges: edges,
      unplacedStarts: state.model.activities.length - starts.length,
      unplacedIntegrations: state.model.integrations.filter(function (integration) {
        return !integration.uses.some(function (use) {
          return (state.model.blocksBySymbol[use.callerID] || []).length > 0;
        });
      }).length
    };
  }

  function appendCanvasFact(parent, label, value) {
    var row = element('div', 'rm-canvas-popover__fact');
    appendText(row, 'span', '', label);
    appendText(row, 'strong', '', value);
    parent.appendChild(row);
  }

  function compactCanvasValues(values, limit) {
    var seen = Object.create(null);
    var distinct = [];
    values.forEach(function (value) {
      if (!value || seen[value]) return;
      seen[value] = true;
      distinct.push(value);
    });
    var visible = distinct.slice(0, limit);
    return visible.join(', ') + (distinct.length > limit ? ' +' + String(distinct.length - limit) : '');
  }

  function canvasNode(kind, id, name, meta, selected, activate) {
    var wrapper = element('div', 'rm-canvas-node-wrap rm-canvas-node-wrap--' + kind);
    var control = element('button', 'rm-canvas-node rm-canvas-node--' + kind);
    control.type = 'button';
    if (activate) control.addEventListener('click', activate);
    control.setAttribute('data-canvas-node', kind + ':' + id);
    if (selected) control.setAttribute('aria-current', 'true');
    appendText(control, 'span', 'rm-canvas-node__name', name);
    appendText(control, 'span', 'rm-canvas-node__meta', meta);
    wrapper.appendChild(control);
    return { wrapper: wrapper, control: control };
  }

  function activityCanvasNode(start) {
    var rendered = canvasNode('entry', start.id, start.name, formatLocation(start.location), false, null);
    var popover = element('div', 'rm-canvas-popover');
    popover.setAttribute('role', 'tooltip');
    appendText(popover, 'p', 'rm-canvas-popover__eyebrow', 'Selected entrypoint');
    appendText(popover, 'h4', '', start.name);
    appendCanvasFact(popover, 'Kind', start.kind);
    var responsibilityNames = (state.model.blocksBySymbol[start.id] || []).map(function (blockID) {
      return state.model.blocksByID[blockID].name;
    });
    if (responsibilityNames.length) {
      appendCanvasFact(popover, 'Responsibilities', compactCanvasValues(responsibilityNames, 3));
    }
    if (start.signature) appendText(popover, 'code', 'rm-canvas-popover__code', start.signature);
    popover.appendChild(sourceAction('Open declaration', start.location));
    rendered.wrapper.appendChild(popover);
    return rendered.wrapper;
  }

  function coreCanvasNode(block, selected) {
    var rendered = canvasNode('core', block.id, block.name,
      String(block.symbols.length) + ' representative declaration' + (block.symbols.length === 1 ? '' : 's'),
      selected, function () { navigateToBlock(block); });
    var popover = element('div', 'rm-canvas-popover');
    popover.setAttribute('role', 'tooltip');
    appendText(popover, 'p', 'rm-canvas-popover__eyebrow', 'Core responsibility');
    appendText(popover, 'h4', '', block.name);
    appendText(popover, 'p', 'rm-canvas-popover__copy', block.purpose);
    if (block.symbols.length) {
      appendCanvasFact(popover, 'Declarations', block.symbols.slice(0, 4).map(function (symbol) {
        return symbol.name;
      }).join(', ') + (block.symbols.length > 4 ? ' +' + String(block.symbols.length - 4) : ''));
      popover.appendChild(sourceAction('Open ' + block.symbols[0].name, block.symbols[0].location));
    }
    rendered.wrapper.appendChild(popover);
    return rendered.wrapper;
  }

  function integrationCanvasNode(integration) {
    var rendered = canvasNode('integration', integration.id, integration.name,
      String(integration.uses.length) + ' selected use' + (integration.uses.length === 1 ? '' : 's'), false, null);
    var popover = element('div', 'rm-canvas-popover');
    popover.setAttribute('role', 'tooltip');
    appendText(popover, 'p', 'rm-canvas-popover__eyebrow', 'Selected integration');
    appendText(popover, 'h4', '', integration.name);
    appendCanvasFact(popover, 'Kind', integration.kind);
    appendCanvasFact(popover, 'Package', integration.packagePath);
    if (integration.uses.length) {
      var use = integration.uses[0];
      appendText(popover, 'p', 'rm-canvas-popover__copy', use.label + ' — ' + use.callee);
      appendCanvasFact(popover, 'Used from', compactCanvasValues(integration.uses.map(function (item) {
        return item.callerName;
      }), 4));
      appendCanvasFact(popover, 'Operations', compactCanvasValues(integration.uses.map(function (item) {
        return item.label;
      }), 3));
      appendCanvasFact(popover, 'Mechanism', compactCanvasValues(integration.uses.map(function (item) {
        return item.mechanism;
      }), 3));
      appendCanvasFact(popover, 'Authority', compactCanvasValues(integration.uses.map(function (item) {
        return humanIntegrationAuthority(item.authority);
      }), 3));
      popover.appendChild(sourceAction(
        integration.uses.length === 1 ? 'Open selected callsite' : 'Open first selected callsite', use.callsite
      ));
    }
    rendered.wrapper.appendChild(popover);
    return rendered.wrapper;
  }

  function renderCanvasLane(label, count, className) {
    var lane = element('section', 'rm-canvas-lane rm-canvas-lane--' + className);
    var header = element('header', 'rm-canvas-lane__header');
    appendText(header, 'h3', '', label);
    appendText(header, 'span', '', count);
    lane.appendChild(header);
    return lane;
  }

  function renderFlowCanvas(selected) {
    var topology = canvasTopology();
    state.canvasEdges = topology.edges;
    var section = element('section', 'rm-flow-section');
    section.setAttribute('aria-labelledby', 'rm-flow-title');
    var header = element('header', 'rm-flow-section__header');
    var copy = element('div');
    appendText(copy, 'p', 'rm-eyebrow', 'System canvas');
    var title = appendText(copy, 'h2', '', 'Repository flow');
    title.id = 'rm-flow-title';
    appendText(copy, 'p', 'rm-flow-section__intro',
      'Only exact selected facts are connected. Missing bindings remain visible instead of being inferred.');
    header.appendChild(copy);
    var legend = element('div', 'rm-canvas-legend');
    appendText(legend, 'span', 'rm-canvas-legend__exact', 'Exact local binding');
    appendText(legend, 'span', 'rm-canvas-legend__runtime', 'Selected callsite; runtime unresolved');
    header.appendChild(legend);
    section.appendChild(header);

    var canvas = element('div', 'rm-flow-canvas');
    var svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('class', 'rm-canvas-edges');
    svg.setAttribute('aria-hidden', 'true');
    canvas.appendChild(svg);
    var lanes = element('div', 'rm-canvas-lanes');

    var entryLane = renderCanvasLane('Entrypoints', String(topology.starts.length) + ' placed / ' +
      String(state.model.activities.length) + ' selected', 'entry');
    var entryNodes = element('div', 'rm-canvas-node-list');
    topology.starts.forEach(function (start) { entryNodes.appendChild(activityCanvasNode(start)); });
    if (!topology.starts.length) appendText(entryNodes, 'p', 'rm-canvas-empty', 'No selected entrypoint is an exact representative member of a responsibility.');
    if (topology.unplacedStarts > 0) appendText(entryNodes, 'p', 'rm-canvas-frontier',
      String(topology.unplacedStarts) + ' selected starts have no exact representative-member binding.');
    entryLane.appendChild(entryNodes);
    lanes.appendChild(entryLane);

    var coreLane = renderCanvasLane('Core', String(state.model.blocks.length) + ' responsibilities', 'core');
    var coreNodes = element('div', 'rm-canvas-node-list');
    state.model.blocks.forEach(function (block) { coreNodes.appendChild(coreCanvasNode(block, block.id === selected.id)); });
    coreLane.appendChild(coreNodes);
    lanes.appendChild(coreLane);

    var integrationLane = renderCanvasLane('Integrations', String(topology.integrations.length) + ' selected', 'integration');
    var integrationNodes = element('div', 'rm-canvas-node-list');
    topology.integrations.forEach(function (integration) { integrationNodes.appendChild(integrationCanvasNode(integration)); });
    if (!topology.integrations.length) appendText(integrationNodes, 'p', 'rm-canvas-empty',
      'No model-selected integration operations for this target.');
    if (topology.unplacedIntegrations > 0) appendText(integrationNodes, 'p', 'rm-canvas-frontier',
      String(topology.unplacedIntegrations) + ' selected integrations have no exact representative-caller binding.');
    integrationLane.appendChild(integrationNodes);
    lanes.appendChild(integrationLane);

    canvas.appendChild(lanes);
    section.appendChild(canvas);
    var accessibleEdges = element('ul', 'rm-sr-only');
    topology.edges.forEach(function (edge) { appendText(accessibleEdges, 'li', '', edge.description); });
    section.appendChild(accessibleEdges);
    return section;
  }

  function drawCanvasEdges() {
    var canvas = document.querySelector('.rm-flow-canvas');
    if (!canvas || !state || !Array.isArray(state.canvasEdges)) return;
    var svg = canvas.querySelector('.rm-canvas-edges');
    if (!svg) return;
    svg.replaceChildren();
    var bounds = canvas.getBoundingClientRect();
    svg.setAttribute('viewBox', '0 0 ' + String(bounds.width) + ' ' + String(bounds.height));
    svg.setAttribute('width', String(bounds.width));
    svg.setAttribute('height', String(bounds.height));
    var nodes = Object.create(null);
    Array.prototype.forEach.call(canvas.querySelectorAll('[data-canvas-node]'), function (node) {
      nodes[node.getAttribute('data-canvas-node')] = node;
    });
    state.canvasEdges.forEach(function (edge) {
      var source = nodes[edge.from];
      var target = nodes[edge.to];
      if (!source || !target) return;
      var sourceBounds = source.getBoundingClientRect();
      var targetBounds = target.getBoundingClientRect();
      var x1 = sourceBounds.right - bounds.left;
      var y1 = sourceBounds.top + sourceBounds.height / 2 - bounds.top;
      var x2 = targetBounds.left - bounds.left;
      var y2 = targetBounds.top + targetBounds.height / 2 - bounds.top;
      var bend = Math.max(34, Math.abs(x2 - x1) * 0.45);
      var path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
      path.setAttribute('d', 'M ' + x1 + ' ' + y1 + ' C ' + (x1 + bend) + ' ' + y1 + ', ' +
        (x2 - bend) + ' ' + y2 + ', ' + x2 + ' ' + y2);
      path.setAttribute('class', 'rm-canvas-edge rm-canvas-edge--' + edge.resolution);
      svg.appendChild(path);
    });
  }

  function scheduleCanvasDraw() {
    window.requestAnimationFrame(drawCanvasEdges);
  }

  function routeForBlock(id) {
    return '#/program/responsibility/' + encodePathSegment(id);
  }

  function selectedBlockFromRoute() {
    var hash = window.location.hash || '#/program';
    if (hash === '#/program' || hash === '#/' || hash === '') return state.model.blocks[0] || null;
    var match = hash.match(/^#\/program\/responsibility\/([^/?#]+)$/);
    if (!match) throw new Error('The requested report route is not supported.');
    var id;
    try { id = decodeURIComponent(match[1]); } catch (error) { throw new Error('The requested report route is malformed.'); }
    var block = state.model.blocksByID[id];
    if (!block) throw new Error('The requested responsibility is not part of this report.');
    return block;
  }

  function navigateToBlock(block) {
    window.location.hash = routeForBlock(block.id);
  }

  function renderHeader() {
    var scope = document.getElementById('rm-target-scope');
    scope.textContent = state.model.target.language + ' ' + state.model.target.kind + ' · ' +
      state.model.revision.slice(0, 10);
    var navigation = document.getElementById('rm-target-navigation');
    var raw = state.model.raw.target_navigation;
    if (!raw || !Array.isArray(raw.targets) || raw.targets.length < 2) return;
    raw.targets.forEach(function (target, index) {
      target = object(target, 'target navigation item');
      var link = element('a', '', text(target.display_name, 'target navigation display name'));
      link.href = text(target.href, 'target navigation href');
      if ((raw.current_target_id && target.target_id === raw.current_target_id) ||
          Number.isSafeInteger(raw.current_target_index) && index === raw.current_target_index) {
        link.setAttribute('aria-current', 'page');
      }
      navigation.appendChild(link);
    });
    navigation.hidden = false;
  }

  function surveySummary(blocks) {
    if (!blocks.length) return 'This target has exact structural evidence, but no semantic responsibility map.';
    var names = blocks.slice(0, 3).map(function (block) { return block.name; });
    var last = names.pop();
    var joined = names.length ? names.join(', ') + ', and ' + last : last;
    return 'Explore ' + String(blocks.length) + ' model-selected responsibilities, beginning with ' + joined + '.';
  }

  function renderSurvey(host) {
    var survey = element('section', 'rm-survey');
    var copy = element('div');
    appendText(copy, 'p', 'rm-eyebrow', 'Repository orientation');
    appendText(copy, 'h1', '', state.model.repoName);
    appendText(copy, 'p', 'rm-survey__summary', surveySummary(state.model.blocks));
    survey.appendChild(copy);

    var facts = element('dl', 'rm-survey__facts');
    [['Target', state.model.target.kind], ['Language', state.model.target.language],
      ['Selector', state.model.target.selector]].forEach(function (row) {
      var wrapper = element('div');
      appendText(wrapper, 'dt', '', row[0]);
      appendText(wrapper, 'dd', '', row[1]);
      facts.appendChild(wrapper);
    });
    survey.appendChild(facts);
    host.appendChild(survey);
  }

  function renderDirections(selected) {
    var aside = element('aside', 'rm-directions');
    aside.setAttribute('aria-label', 'Repository responsibilities');
    appendText(aside, 'h2', '', 'Choose a direction');
    appendText(aside, 'p', 'rm-directions__hint', 'Each direction is a model-selected responsibility grounded in exact code evidence.');
    var list = element('div', 'rm-direction-list');
    state.model.blocks.forEach(function (block, index) {
      var button = element('button', 'rm-direction');
      button.type = 'button';
      button.setAttribute('aria-current', block.id === selected.id ? 'true' : 'false');
      appendText(button, 'span', 'rm-direction__number', String(index + 1).padStart(2, '0'));
      appendText(button, 'span', 'rm-direction__name', block.name);
      button.addEventListener('click', function () { navigateToBlock(block); });
      list.appendChild(button);
    });
    aside.appendChild(list);
    return aside;
  }

  function renderStarts(parent, block) {
    var starts = block.symbols.map(function (symbol) { return state.model.activityByID[symbol.id]; }).filter(Boolean);
    if (!starts.length) return;
    var section = element('section', 'rm-focus-section');
    appendText(section, 'h3', '', state.model.target.kind === 'library' ? 'Ways into this area' : 'Execution starts here');
    appendText(section, 'p', 'rm-focus-section__intro',
      state.model.target.kind === 'library' ? 'Selected public or internal entrypoints that participate in this responsibility.' :
        'Selected activity entrypoints that participate in this responsibility.');
    var list = element('ul', 'rm-start-list');
    starts.slice(0, 5).forEach(function (start) {
      var item = element('li', 'rm-start');
      var action = sourceAction(start.name, start.location);
      action.className += ' rm-start__name';
      item.appendChild(action);
      appendText(item, 'code', 'rm-start__signature', start.signature || start.kind);
      list.appendChild(item);
    });
    section.appendChild(list);
    parent.appendChild(section);
  }

  function renderConnections(parent, block, connections) {
    var section = element('section', 'rm-focus-section');
    appendText(section, 'h3', '', 'How the code connects');
    appendText(section, 'p', 'rm-focus-section__intro',
      'Exact local relations come first. Dynamic joints remain visibly unresolved.');
    if (!connections.length) {
      appendText(section, 'p', 'rm-empty', 'No focused call or handoff relation is available for the representative symbols.');
      parent.appendChild(section);
      return;
    }
    var list = element('ul', 'rm-connection-list');
    connections.forEach(function (connection) {
      var item = element('li', 'rm-connection');
      var body = element('div');
      var path = element('div', 'rm-connection__path');
      appendText(path, 'span', '', connection.from);
      appendText(path, 'span', 'rm-connection__arrow', '→');
      appendText(path, 'span', '', connection.to);
      body.appendChild(path);
      var meta = humanRelation(connection.kind);
      if (connection.location) meta += ' · ' + formatLocation(connection.location);
      appendText(body, 'div', 'rm-connection__meta', meta);
      item.appendChild(body);
      appendText(item, 'span', 'rm-resolution rm-resolution--' + connection.resolution, connection.resolution);
      list.appendChild(item);
    });
    section.appendChild(list);
    parent.appendChild(section);
  }

  function renderFocus(block, index, connections, related) {
    var article = element('article', 'rm-focus');
    article.tabIndex = -1;
    appendText(article, 'p', 'rm-focus__index', 'Responsibility ' + String(index + 1).padStart(2, '0'));
    appendText(article, 'h2', '', block.name);
    appendText(article, 'p', 'rm-focus__purpose', block.purpose);
    renderStarts(article, block);
    renderConnections(article, block, connections);

    if (related.length) {
      var relatedHost = element('div', 'rm-related');
      appendText(relatedHost, 'span', 'rm-focus-section__intro', 'Related responsibilities');
      related.forEach(function (candidate) {
        var button = element('button', '', candidate.name);
        button.type = 'button';
        button.addEventListener('click', function () { navigateToBlock(candidate); });
        relatedHost.appendChild(button);
      });
      article.appendChild(relatedHost);
    }

    var next = state.model.blocks[(index + 1) % state.model.blocks.length];
    if (next && next.id !== block.id) {
      var nextHost = element('div', 'rm-next');
      var button = element('button');
      button.type = 'button';
      button.appendChild(document.createTextNode('Next: ' + next.name));
      appendText(button, 'span', '', '→');
      button.addEventListener('click', function () { navigateToBlock(next); });
      nextHost.appendChild(button);
      article.appendChild(nextHost);
    }
    return article;
  }

  function renderEvidence(block) {
    var aside = element('aside', 'rm-evidence');
    aside.setAttribute('aria-label', 'Exact source evidence');
    var sticky = element('div', 'rm-evidence__sticky');
    appendText(sticky, 'h2', '', 'Verify in code');
    appendText(sticky, 'p', 'rm-evidence__intro',
      'Representative declarations and files restored against the captured revision.');

    appendText(sticky, 'h3', '', 'Declarations');
    var symbols = element('ul', 'rm-evidence-list');
    block.symbols.forEach(function (symbol) {
      var item = element('li');
      item.appendChild(sourceAction(symbol.name, symbol.location));
      symbols.appendChild(item);
    });
    sticky.appendChild(symbols);

    appendText(sticky, 'h3', '', 'Files');
    var files = element('ul', 'rm-evidence-list');
    block.files.forEach(function (file) {
      var item = element('li');
      item.appendChild(sourceAction(file.path, { path: file.path, line: 0, column: 0 }));
      files.appendChild(item);
    });
    sticky.appendChild(files);

    var unresolved = block.symbols.reduce(function (total, symbol) { return total + symbol.unresolvedOutgoing; }, 0);
    if (unresolved > 0) {
      appendText(sticky, 'p', 'rm-evidence-note',
        'Some outgoing runtime dispatch from these declarations is unresolved. Treat the focused explanation as grounded orientation, not a complete runtime trace.');
    }
    aside.appendChild(sticky);
    return aside;
  }

  function renderDiagnostics(host) {
    var notes = state.model.warnings.slice();
    var coverage = state.model.coreCoverage;
    if (coverage && coverage.program_objects_omitted > 0) {
      notes.push(String(coverage.program_objects_omitted) +
        ' ProgramIndex objects were outside the retained semantic index; selected responsibility evidence remains validated.');
    }
    if (!notes.length) return;
    var details = element('details', 'rm-diagnostics');
    appendText(details, 'summary', '', 'Evidence limits');
    var list = element('ul');
    notes.forEach(function (note) { appendText(list, 'li', '', String(note)); });
    details.appendChild(list);
    host.appendChild(details);
  }

  function renderStructuralOnly(host) {
    var section = element('section', 'rm-focus');
    appendText(section, 'p', 'rm-eyebrow', 'Structural evidence only');
    appendText(section, 'h2', '', 'A semantic responsibility map is not available for this target.');
    appendText(section, 'p', 'rm-focus__purpose',
      'The report retains exact target and source evidence, but the current pipeline did not produce the focused semantic layer required by this interface.');
    host.appendChild(section);
  }

  function renderRoute() {
    var host = document.getElementById('rm-app');
    var orientation = element('div', 'rm-orientation');
    renderSurvey(orientation);
    if (!state.model.blocks.length) {
      renderStructuralOnly(orientation);
      renderDiagnostics(orientation);
      host.replaceChildren(orientation);
      return;
    }
    var selected = selectedBlockFromRoute();
    var index = state.model.blocks.findIndex(function (block) { return block.id === selected.id; });
    var connections = connectionsFor(selected);
    var related = relatedBlocksFor(selected, connections);
    orientation.appendChild(renderFlowCanvas(selected));
    var workspace = element('section', 'rm-workspace');
    workspace.appendChild(renderDirections(selected));
    workspace.appendChild(renderFocus(selected, index, connections, related));
    workspace.appendChild(renderEvidence(selected));
    orientation.appendChild(workspace);
    renderDiagnostics(orientation);
    host.replaceChildren(orientation);
    scheduleCanvasDraw();
    document.title = state.model.repoName + ' — ' + selected.name + ' — repomap';
  }

  function showToast(message, error) {
    var toast = document.getElementById('rm-toast');
    toast.textContent = String(message || '');
    toast.className = 'rm-toast' + (error ? ' rm-toast--error' : '');
    toast.hidden = false;
    if (toastTimer) window.clearTimeout(toastTimer);
    toastTimer = window.setTimeout(function () { toast.hidden = true; }, 5500);
  }

  function renderFatal(error) {
    document.getElementById('rm-app').hidden = true;
    var fatal = document.getElementById('rm-fatal');
    fatal.hidden = false;
    document.getElementById('rm-fatal-message').textContent = error && error.message ? error.message : String(error);
  }

  function boot() {
    try {
      var raw = readPayload();
      var model = buildPresentationModel(raw);
      state = { model: model, source: buildSourceAuthority(raw, model), canvasEdges: [] };
      renderHeader();
      renderRoute();
      window.addEventListener('resize', scheduleCanvasDraw);
      window.addEventListener('hashchange', function () {
        try { renderRoute(); } catch (error) { renderFatal(error); }
      });
    } catch (error) {
      renderFatal(error);
    }
  }

  boot();
})();
