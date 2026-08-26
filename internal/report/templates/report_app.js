(function () {
  'use strict';

  var state = null;
  var toastTimer = 0;
  var MAX_REVISION_ABBREVIATION_CHARS = 10;
  var MAX_SURVEY_PREVIEW_ITEMS = 3;

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

  function projectionCoverage(value, label) {
    value = object(value, label);
    var coverage = {
      eligible: integer(value.eligible, label + '.eligible'),
      shown: integer(value.shown, label + '.shown'),
      omitted: integer(value.omitted, label + '.omitted')
    };
    if (coverage.eligible !== coverage.shown + coverage.omitted) {
      throw new Error(label + ' does not account for every eligible item.');
    }
    return coverage;
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

  function compactDisplayText(value, maxCharacters) {
    var normalized = String(value || '').replace(/\s+/g, ' ').trim();
    var characters = Array.from(normalized);
    if (characters.length <= maxCharacters) return normalized;
    return characters.slice(0, maxCharacters - 1).join('').trimEnd() + '…';
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

  function closedText(value, allowed, label) {
    value = text(value, label);
    if (allowed.indexOf(value) < 0) throw new Error(label + ' is not supported');
    return value;
  }

  function buildTargetDirectory(data, currentTarget) {
    var byID = Object.create(null);
    var targets = [];
    var raw = data.target_navigation;
    if (raw == null) {
      var only = {
        id: currentTarget.id,
        language: currentTarget.language,
        kind: currentTarget.kind,
        displayName: currentTarget.name,
        href: '#/program'
      };
      byID[only.id] = only;
      return { targets: [only], byID: byID, currentID: only.id, defaultID: only.id };
    }

    raw = object(raw, 'target_navigation');
    array(raw.targets, 'target_navigation.targets').forEach(function (rawTarget) {
      rawTarget = object(rawTarget, 'target navigation item');
      var target = {
        id: text(rawTarget.target_id, 'target navigation target_id'),
        language: text(rawTarget.language, 'target navigation language'),
        kind: text(rawTarget.kind, 'target navigation kind'),
        displayName: text(rawTarget.display_name, 'target navigation display_name'),
        href: text(rawTarget.href, 'target navigation href')
      };
      if (byID[target.id]) throw new Error('Target navigation identities are not unique.');
      byID[target.id] = target;
      targets.push(target);
    });
    if (!targets.length) throw new Error('Target navigation has no targets.');

    var currentID = optionalText(raw.current_target_id, 'target_navigation.current_target_id');
    if (Object.prototype.hasOwnProperty.call(raw, 'current_target_index')) {
      var currentIndex = integer(raw.current_target_index, 'target_navigation.current_target_index');
      if (currentIndex >= targets.length) throw new Error('The current target navigation index is absent.');
      if (currentID && currentID !== targets[currentIndex].id) {
        throw new Error('The current target navigation authorities disagree.');
      }
      currentID = targets[currentIndex].id;
    }
    if (!currentID || !byID[currentID]) throw new Error('The current target navigation identity is absent.');
    var defaultID = optionalText(raw.default_target_id, 'target_navigation.default_target_id');
    if (Object.prototype.hasOwnProperty.call(raw, 'default_target_index')) {
      var defaultIndex = integer(raw.default_target_index, 'target_navigation.default_target_index');
      if (defaultIndex >= targets.length) throw new Error('The default target navigation index is absent.');
      if (defaultID && defaultID !== targets[defaultIndex].id) {
        throw new Error('The default target navigation authorities disagree.');
      }
      defaultID = targets[defaultIndex].id;
    }
    if (!defaultID || !byID[defaultID]) throw new Error('The default target navigation identity is absent.');
    var current = byID[currentID];
    if (current.id !== currentTarget.id || current.language !== currentTarget.language ||
        current.kind !== currentTarget.kind || current.displayName !== currentTarget.name) {
      throw new Error('Target navigation does not match the current ProgramTarget.');
    }
    return { targets: targets, byID: byID, currentID: currentID, defaultID: defaultID };
  }

  function buildRuntimePortfolio(data, targetDirectory, openable) {
    var raw = object(data.runtime_portfolio, 'runtime_portfolio');
    if (integer(raw.version, 'runtime_portfolio.version') !== 3) {
      throw new Error('The runtime portfolio version is not supported.');
    }
    var roleIDs = Object.create(null);
    var mappedTargets = Object.create(null);
    var roles = array(raw.roles, 'runtime_portfolio.roles').map(function (rawRole) {
      rawRole = object(rawRole, 'runtime role');
      var id = text(rawRole.id, 'runtime role.id');
      if (roleIDs[id]) throw new Error('Runtime role identities are not unique.');
      roleIDs[id] = true;
      var seenImplementations = Object.create(null);
      var implementations = array(rawRole.implementations, 'runtime role.implementations').map(function (rawImplementation) {
        rawImplementation = object(rawImplementation, 'runtime implementation');
        var targetID = text(rawImplementation.program_target_id, 'runtime implementation.program_target_id');
        var target = targetDirectory.byID[targetID];
        if (!target) throw new Error('A runtime implementation cites an unknown ProgramTarget.');
        var mode = optionalText(rawImplementation.mode, 'runtime implementation.mode');
        var key = targetID + '\u0000' + mode;
        if (seenImplementations[key]) throw new Error('A runtime role repeats one target and mode mapping.');
        seenImplementations[key] = true;
        mappedTargets[targetID] = true;
        return { target: target, mode: mode };
      });
      var evidence = array(rawRole.evidence, 'runtime role.evidence').map(function (rawEvidence) {
        rawEvidence = object(rawEvidence, 'runtime role evidence');
        var location = exactLocation(rawEvidence.location, 'runtime role evidence.location');
        if (!openable[location.path]) throw new Error('Runtime role evidence is outside publication authority.');
        return { label: text(rawEvidence.label, 'runtime role evidence.label'), location: location };
      });
      return {
        id: id,
        name: text(rawRole.name, 'runtime role.name'),
        purpose: text(rawRole.purpose, 'runtime role.purpose'),
        prominence: closedText(rawRole.prominence, ['primary', 'supporting', 'unknown'], 'runtime role.prominence'),
        roleKind: closedText(rawRole.role_kind,
          ['library', 'service', 'daemon', 'worker', 'cli', 'example', 'supporting_tool', 'unknown'], 'runtime role.role_kind'),
        requiredness: closedText(rawRole.requiredness,
          ['required', 'optional', 'experimental', 'unknown'], 'runtime role.requiredness'),
        confidence: closedText(rawRole.confidence, ['high', 'medium', 'low', 'unknown'], 'runtime role.confidence'),
        implementations: implementations,
        evidence: evidence
      };
    });
    var unclassifiedIDs = Object.create(null);
    var unclassified = array(raw.unclassified_targets, 'runtime_portfolio.unclassified_targets').map(function (rawTarget) {
      rawTarget = object(rawTarget, 'unclassified runtime target');
      var targetID = text(rawTarget.program_target_id, 'unclassified runtime target.program_target_id');
      var target = targetDirectory.byID[targetID];
      if (!target) throw new Error('An unclassified runtime target cites an unknown ProgramTarget.');
      if (mappedTargets[targetID]) throw new Error('A mapped runtime target is also marked unclassified.');
      if (unclassifiedIDs[targetID]) throw new Error('Unclassified runtime target identities are not unique.');
      unclassifiedIDs[targetID] = true;
      return { target: target, reason: text(rawTarget.reason, 'unclassified runtime target.reason') };
    });
    return { roles: roles, unclassified: unclassified };
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

  function buildJSTSFactDirectory(rawFacts, label, openable) {
    var facts = [];
    var byRef = Object.create(null);
    array(rawFacts, label + '.facts').forEach(function (rawFact) {
      rawFact = object(rawFact, label + ' fact');
      var ref = text(rawFact.ref, label + ' fact.ref');
      if (byRef[ref]) throw new Error(label + ' fact identities are not unique.');
      var location = rawFact.location == null ? null : exactLocation(rawFact.location, label + ' fact.location');
      if (location && !openable[location.path]) throw new Error(label + ' fact is outside publication authority.');
      var fact = {
        ref: ref,
        category: closedText(rawFact.category,
          ['file', 'declaration', 'import', 'export', 'call', 'surface', 'route', 'http_use', 'contract', 'resource'],
          label + ' fact.category'),
        kind: text(rawFact.kind, label + ' fact.kind'),
        label: text(rawFact.label, label + ' fact.label'),
        location: location
      };
      byRef[ref] = fact;
      facts.push(fact);
    });
    return { facts: facts, byRef: byRef };
  }

  function buildJSTSSurfaceCatalog(data, target, indexSHA256, openable) {
    if (data.js_ts_surface_catalog_view == null) return null;
    var raw = object(data.js_ts_surface_catalog_view, 'js_ts_surface_catalog_view');
    if (integer(raw.version, 'js_ts_surface_catalog_view.version') !== 2 ||
        text(raw.program_target_id, 'js_ts_surface_catalog_view.program_target_id') !== target.id ||
        text(raw.program_index_sha256, 'js_ts_surface_catalog_view.program_index_sha256') !== indexSHA256) {
      throw new Error('The JavaScript/TypeScript surface catalog does not bind the default program.');
    }
    var facts = buildJSTSFactDirectory(raw.facts, 'js_ts_surface_catalog_view', openable);
    var project = object(raw.project, 'js_ts_surface_catalog_view.project');
    var surfaceByID = Object.create(null);
    var surfaces = array(raw.surfaces, 'js_ts_surface_catalog_view.surfaces').map(function (rawSurface) {
      rawSurface = object(rawSurface, 'JavaScript/TypeScript surface');
      var id = text(rawSurface.surface_id, 'JavaScript/TypeScript surface.surface_id');
      if (surfaceByID[id]) throw new Error('JavaScript/TypeScript surface identities are not unique.');
      var role = closedText(rawSurface.role, ['product', 'supporting', 'script', 'unknown'],
        'JavaScript/TypeScript surface.role');
      var disposition = closedText(rawSurface.disposition,
        ['product_surface', 'supporting_code', 'tool', 'unknown'],
        'JavaScript/TypeScript surface.disposition');
      var expectedDisposition = { product: 'product_surface', supporting: 'supporting_code', script: 'tool', unknown: 'unknown' }[role];
      if (disposition !== expectedDisposition) throw new Error('JavaScript/TypeScript surface role and disposition disagree.');
      var location = exactLocation(rawSurface.location, 'JavaScript/TypeScript surface.location');
      if (!openable[location.path]) throw new Error('A JavaScript/TypeScript surface is outside publication authority.');
      function references(values, referenceLabel) {
        return array(values, referenceLabel).map(function (ref) {
          ref = text(ref, referenceLabel + '[]');
          if (!facts.byRef[ref]) throw new Error('A JavaScript/TypeScript surface cites an unknown fact.');
          return ref;
        });
      }
      var surface = {
        id: id,
        kind: closedText(rawSurface.kind,
          ['browser_application', 'node_server', 'command_line_application', 'shared_contracts', 'tool', 'unknown'],
          'JavaScript/TypeScript surface.kind'),
        role: role,
        disposition: disposition,
        name: text(rawSurface.name, 'JavaScript/TypeScript surface.name'),
        entryRefs: references(rawSurface.entry_refs, 'JavaScript/TypeScript surface.entry_refs'),
        evidenceRefs: references(rawSurface.evidence_refs, 'JavaScript/TypeScript surface.evidence_refs'),
        location: location
      };
      surfaceByID[id] = surface;
      return surface;
    });
    return {
      project: {
        name: text(project.name, 'js_ts_surface_catalog_view.project.name'),
        manifestPath: text(project.manifest_path, 'js_ts_surface_catalog_view.project.manifest_path'),
        configPath: optionalText(project.config_path, 'js_ts_surface_catalog_view.project.config_path'),
        moduleResolution: text(project.module_resolution, 'js_ts_surface_catalog_view.project.module_resolution')
      },
      facts: facts.facts,
      factsByRef: facts.byRef,
      surfaces: surfaces,
      surfacesByID: surfaceByID
    };
  }

  function buildCrossSurfacePaths(data, target, indexSHA256, openable, surfaceCatalog) {
    if (data.cross_surface_path_view == null) return null;
    var raw = object(data.cross_surface_path_view, 'cross_surface_path_view');
    if (integer(raw.version, 'cross_surface_path_view.version') !== 1 ||
        text(raw.program_target_id, 'cross_surface_path_view.program_target_id') !== target.id ||
        text(raw.program_index_sha256, 'cross_surface_path_view.program_index_sha256') !== indexSHA256 || !surfaceCatalog) {
      throw new Error('The cross-surface path inventory does not bind the exact surface catalog and default program.');
    }
    var facts = buildJSTSFactDirectory(raw.facts, 'cross_surface_path_view', openable);
    var pathByID = Object.create(null);
    var paths = array(raw.paths, 'cross_surface_path_view.paths').map(function (rawPath) {
      rawPath = object(rawPath, 'cross-surface path');
      var id = text(rawPath.path_id, 'cross-surface path.path_id');
      if (pathByID[id]) throw new Error('Cross-surface path identities are not unique.');
      var steps = array(rawPath.steps, 'cross-surface path.steps').map(function (rawStep, position) {
        rawStep = object(rawStep, 'cross-surface path step');
        if (integer(rawStep.ordinal, 'cross-surface path step.ordinal') !== position + 1) {
          throw new Error('Cross-surface path step ordinals are not canonical.');
        }
        var sourceRef = text(rawStep.source_ref, 'cross-surface path step.source_ref');
        if (!facts.byRef[sourceRef]) throw new Error('A cross-surface step cites an unknown source fact.');
        var targetRefs = array(rawStep.target_refs, 'cross-surface path step.target_refs').map(function (ref) {
          ref = text(ref, 'cross-surface path step.target_refs[]');
          if (!facts.byRef[ref]) throw new Error('A cross-surface step cites an unknown target fact.');
          return ref;
        });
        var location = exactLocation(rawStep.location, 'cross-surface path step.location');
        if (!openable[location.path]) throw new Error('A cross-surface path step is outside publication authority.');
        var resolution = closedText(rawStep.resolution, ['exact', 'alternatives', 'unresolved'],
          'cross-surface path step.resolution');
        var authority = closedText(rawStep.authority,
          ['exact_static', 'resolved_indirect', 'possible', 'unresolved_frontier'],
          'cross-surface path step.authority');
        var expectedResolution = {
          exact_static: 'exact', resolved_indirect: 'exact',
          possible: 'alternatives', unresolved_frontier: 'unresolved'
        }[authority];
        if (resolution !== expectedResolution) {
          throw new Error('A cross-surface path step has incompatible authority and resolution.');
        }
        return {
          ordinal: position + 1,
          kind: closedText(rawStep.kind,
            ['page_route', 'render_target', 'mutation_site', 'program_call', 'client_http_use',
              'http_method_path_match', 'server_route', 'middleware', 'handler_factory', 'handler',
              'contract_validation', 'storage_call', 'resource_boundary'], 'cross-surface path step.kind'),
          label: text(rawStep.label, 'cross-surface path step.label'),
          sourceRef: sourceRef,
          targetRefs: targetRefs,
          resolution: resolution,
          authority: authority,
          location: location
        };
      });
      if (!steps.length) throw new Error('A cross-surface path has no steps.');
      var path = {
        id: id,
        name: text(rawPath.name, 'cross-surface path.name'),
        outcome: text(rawPath.outcome, 'cross-surface path.outcome'),
        steps: steps,
        frontier: optionalText(rawPath.frontier, 'cross-surface path.frontier')
      };
      pathByID[id] = path;
      return path;
    });
    var coverage = object(raw.coverage, 'cross_surface_path_view.coverage');
    ['routes_observed', 'http_uses_observed', 'paths_projected', 'steps_projected', 'exact_steps',
      'alternative_steps', 'unresolved_steps', 'exact_static_steps', 'resolved_indirect_steps',
      'possible_steps', 'unresolved_frontier_steps', 'frontiers'].forEach(function (field) {
      integer(coverage[field], 'cross_surface_path_view.coverage.' + field);
    });
    if (coverage.paths_projected !== paths.length) throw new Error('Cross-surface path coverage does not match the path inventory.');
    facts.facts.forEach(function (fact) {
      if (fact.category !== 'surface') return;
      var surface = surfaceCatalog.surfacesByID[fact.ref];
      if (!surface || fact.kind !== surface.kind || fact.label !== surface.name || !fact.location ||
          fact.location.path !== surface.location.path || fact.location.line !== surface.location.line ||
          fact.location.column !== surface.location.column) {
        throw new Error('A cross-surface surface fact does not match the exact surface catalog.');
      }
    });
    paths.forEach(function (path) {
      var citesBrowser = false;
      var citesServer = false;
      function citeSurface(ref) {
        if (facts.byRef[ref].category !== 'surface') return;
        var surface = surfaceCatalog.surfacesByID[ref];
        if (!surface || surface.role !== 'product') return;
        if (surface.kind === 'browser_application') citesBrowser = true;
        if (surface.kind === 'node_server') citesServer = true;
      }
      path.steps.forEach(function (step) {
        citeSurface(step.sourceRef);
        step.targetRefs.forEach(citeSurface);
      });
      if (!citesBrowser || !citesServer) {
        throw new Error('A cross-surface path must cite exact browser and server product surfaces.');
      }
    });
    return { facts: facts.facts, factsByRef: facts.byRef, paths: paths, pathsByID: pathByID, coverage: coverage };
  }

  function activityPathUseKey(value) {
    return value.dependencyID + '\u0000' + value.relationID + '\u0000' +
      String(value.witnessIndex) + '\u0000' + value.externalSymbolID;
  }

  function buildActivityPaths(data, target, indexSHA256, activitiesByID, integrationUsesByKey) {
    if (data.activity_path_view == null) return null;
    var raw = object(data.activity_path_view, 'activity_path_view');
    if (integer(raw.version, 'activity_path_view.version') !== 1 ||
        text(raw.program_target_id, 'activity_path_view.program_target_id') !== target.id ||
        text(raw.program_index_sha256, 'activity_path_view.program_index_sha256') !== indexSHA256) {
      throw new Error('Activity paths do not bind the default program.');
    }
    var objectsByID = Object.create(null);
    array(raw.objects, 'activity path objects').forEach(function (rawObject) {
      rawObject = object(rawObject, 'activity path object');
      var id = text(rawObject.object_id, 'activity path object.object_id');
      if (objectsByID[id]) throw new Error('Activity path object identities are not unique.');
      objectsByID[id] = {
        id: id,
        kind: text(rawObject.kind, 'activity path object.kind'),
        name: text(rawObject.name, 'activity path object.name'),
        signature: optionalText(rawObject.signature, 'activity path object.signature'),
        location: rawObject.location == null ? null : exactLocation(rawObject.location, 'activity path object.location')
      };
    });
    var routes = [];
    var routesByID = Object.create(null);
    array(raw.routes, 'activity path routes').forEach(function (rawRoute) {
      rawRoute = object(rawRoute, 'activity path route');
      var id = text(rawRoute.route_id, 'activity path route.route_id');
      if (routesByID[id]) throw new Error('Activity path route identities are not unique.');
      var callerID = text(rawRoute.caller_id, 'activity path route.caller_id');
      if (!objectsByID[callerID]) throw new Error('An activity path caller is absent from its object dictionary.');
      var activityID = optionalText(rawRoute.activity_id, 'activity path route.activity_id');
      if (activityID && (!objectsByID[activityID] || !activitiesByID[activityID])) {
        throw new Error('An activity path starts outside selected entrypoint authority.');
      }
      var steps = array(rawRoute.steps, 'activity path route.steps').map(function (rawStep) {
        rawStep = object(rawStep, 'activity path step');
        var fromID = text(rawStep.from_id, 'activity path step.from_id');
        var toID = text(rawStep.to_id, 'activity path step.to_id');
        if (!objectsByID[fromID] || !objectsByID[toID]) {
          throw new Error('An activity path step is outside its object dictionary.');
        }
        return {
          relationID: text(rawStep.relation_id, 'activity path step.relation_id'),
          fromID: fromID,
          toID: toID,
          kind: text(rawStep.kind, 'activity path step.kind'),
          resolution: closedText(rawStep.resolution, ['exact', 'alternatives'], 'activity path step.resolution'),
          authority: closedText(rawStep.authority, ['exact', 'possible'], 'activity path step.authority'),
          invocation: optionalText(rawStep.invocation, 'activity path step.invocation'),
          location: rawStep.location == null ? null : exactLocation(rawStep.location, 'activity path step.location')
        };
      });
      var route = {
        id: id,
        callerID: callerID,
        status: closedText(rawRoute.status, ['exact', 'possible', 'frontier', 'unconnected'], 'activity path route.status'),
        activityID: activityID,
        steps: steps,
        distance: integer(rawRoute.distance, 'activity path route.distance'),
        possibleSteps: integer(rawRoute.possible_steps, 'activity path route.possible_steps'),
        callbackHandoffs: integer(rawRoute.callback_handoffs, 'activity path route.callback_handoffs'),
        frontier: array(rawRoute.frontier, 'activity path route.frontier').map(function (reason) {
          return text(reason, 'activity path route.frontier[]');
        }),
        outcomes: []
      };
      if (route.distance !== steps.length) throw new Error('Activity path distance does not match its exact steps.');
      routesByID[id] = route;
      routes.push(route);
    });
    var outcomesByUseKey = Object.create(null);
    array(raw.outcomes, 'activity path outcomes').forEach(function (rawOutcome) {
      rawOutcome = object(rawOutcome, 'activity path outcome');
      var outcome = {
        dependencyID: text(rawOutcome.dependency_id, 'activity path outcome.dependency_id'),
        relationID: text(rawOutcome.relation_id, 'activity path outcome.relation_id'),
        witnessIndex: integer(rawOutcome.witness_index, 'activity path outcome.witness_index'),
        externalSymbolID: text(rawOutcome.external_symbol_id, 'activity path outcome.external_symbol_id'),
        routeID: text(rawOutcome.route_id, 'activity path outcome.route_id')
      };
      var route = routesByID[outcome.routeID];
      if (!route) throw new Error('An activity path outcome cites an unknown route.');
      var key = activityPathUseKey(outcome);
      var use = integrationUsesByKey[key];
      if (!use || outcomesByUseKey[key]) {
        throw new Error('Activity path outcomes do not cover exact integration uses once.');
      }
      outcome.use = use;
      outcome.route = route;
      use.route = route;
      route.outcomes.push(outcome);
      outcomesByUseKey[key] = outcome;
    });
    if (Object.keys(outcomesByUseKey).length !== Object.keys(integrationUsesByKey).length) {
      throw new Error('Activity paths do not cover every exact integration use.');
    }
    return {
      objectsByID: objectsByID,
      routes: routes,
      routesByID: routesByID,
      outcomesByUseKey: outcomesByUseKey,
      coverage: object(raw.coverage, 'activity path coverage')
    };
  }

  function buildPresentationModel(data) {
    text(data.repo_name, 'repo_name');
    if (integer(data.format_version, 'format_version') !== 67) {
      throw new Error('The report format version is not supported.');
    }
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
    var indexSHA256 = text(view.index_sha256, 'default program view.index_sha256');

    var objectsByID = Object.create(null);
    array(view.objects, 'program view.objects').forEach(function (rawObject) {
      rawObject = object(rawObject, 'program object');
      var id = text(rawObject.id, 'program object.id');
      if (objectsByID[id]) throw new Error('Program object identities are not unique.');
      objectsByID[id] = rawObject;
    });
    var relations = array(view.relations, 'program view.relations');
    var rawProjection = object(view.projection, 'program view.projection');
    var programProjection = {
      seeds: projectionCoverage(rawProjection.seeds, 'program view.projection.seeds'),
      objects: projectionCoverage(rawProjection.objects, 'program view.projection.objects'),
      relations: projectionCoverage(rawProjection.relations, 'program view.projection.relations'),
      witnessesOmitted: relations.reduce(function (total, rawRelation) {
        rawRelation = object(rawRelation, 'program relation');
        return total + integer(rawRelation.witnesses_projection_omitted, 'program relation.witnesses_projection_omitted');
      }, 0)
    };

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
    var integrationsByID = Object.create(null);
    var integrationUsesByKey = Object.create(null);
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
          var use = {
            dependencyID: dependencyID,
            relationID: text(rawUse.relation_id, 'integration use.relation_id'),
            witnessIndex: integer(rawUse.witness_index, 'integration use.witness_index'),
            externalSymbolID: text(rawUse.external_symbol_id, 'integration use.external_symbol_id'),
            callerID: text(rawUse.caller_id, 'integration use.caller_id'),
            callerName: text(rawUse.caller_name, 'integration use.caller_name'),
            callsite: exactLocation(rawUse.callsite, 'integration use.callsite'),
            callee: text(rawUse.canonical_callee, 'integration use.canonical_callee'),
            label: text(rawUse.label, 'integration use.label'),
            mechanism: text(rawUse.mechanism, 'integration use.mechanism'),
            authority: text(rawUse.authority, 'integration use.authority')
          };
          use.key = activityPathUseKey(use);
          if (integrationUsesByKey[use.key]) throw new Error('Integration use identities are not unique.');
          integrationUsesByKey[use.key] = use;
          return use;
        });
        var integration = {
          id: dependencyID,
          name: text(rawDependency.name, 'integration dependency.name'),
          kind: text(rawDependency.kind, 'integration dependency.kind'),
          packagePath: text(rawDependency.package_path, 'integration dependency.package_path'),
          modulePath: optionalText(rawDependency.module_path, 'integration dependency.module_path'),
          uses: uses
        };
        integrationsByID[integration.id] = integration;
        integrations.push(integration);
      });
    }

    var activityPaths = buildActivityPaths(
      data, target, indexSHA256, activityByID, integrationUsesByKey
    );

    var blocksByID = Object.create(null);
    var blocksBySymbol = Object.create(null);
    blocks.forEach(function (block) {
      blocksByID[block.id] = block;
      block.symbols.forEach(function (symbol) {
        if (!blocksBySymbol[symbol.id]) blocksBySymbol[symbol.id] = [];
        blocksBySymbol[symbol.id].push(block.id);
      });
    });

    var groups = [];
    var groupByBlock = Object.create(null);
    var groupsByBlock = Object.create(null);
    if (core) {
      var groupedBlocks = Object.create(null);
	  var modelGroupedBlocks = Object.create(null);
	  var modelGroupCount = 0;
	  var localGroupCount = 0;
	  var rawGroups = array(core.refined_groups, 'core_map_view.refined_groups');
      rawGroups.forEach(function (rawGroup, groupPosition) {
        rawGroup = object(rawGroup, 'refined core group');
        var groupID = text(rawGroup.id, 'refined core group.id');
		var authority = text(rawGroup.authority, 'refined core group.authority');
		if (authority === 'model') {
		  modelGroupCount++;
		} else if (authority === 'local_unassigned') {
		  localGroupCount++;
		  if (localGroupCount !== 1 || groupPosition !== rawGroups.length - 1 ||
		      rawGroup.name !== 'Not placed by grouping model' ||
		      rawGroup.purpose !== 'Accounts for responsibilities the grouping model did not place in a supported orientation area.') {
		    throw new Error('The local unassigned responsibility group is invalid.');
		  }
		} else {
		  throw new Error('A refined core group has unknown authority.');
		}
        var insideGroup = Object.create(null);
        var blockIDs = array(rawGroup.core_block_ids, 'refined core group.core_block_ids').map(function (blockID) {
          blockID = text(blockID, 'refined core group.core_block_ids[]');
          if (!blocksByID[blockID]) throw new Error('A refined core group cites an unknown responsibility.');
          if (insideGroup[blockID]) throw new Error('A refined core group repeats a responsibility.');
          insideGroup[blockID] = true;
          groupedBlocks[blockID] = true;
		  if (authority === 'model') {
		    modelGroupedBlocks[blockID] = true;
		  } else if (modelGroupedBlocks[blockID]) {
		    throw new Error('The local unassigned group overlaps a model-owned group.');
		  }
          return blockID;
        });
        if (!blockIDs.length) throw new Error('A refined core group has no responsibilities.');
        var group = {
          id: groupID,
		  authority: authority,
          name: text(rawGroup.name, 'refined core group.name'),
          purpose: text(rawGroup.purpose, 'refined core group.purpose'),
          blockIDs: blockIDs
        };
        groups.push(group);
        blockIDs.forEach(function (blockID) {
          if (!groupsByBlock[blockID]) groupsByBlock[blockID] = [];
          groupsByBlock[blockID].push(group);
          if (!groupByBlock[blockID]) groupByBlock[blockID] = group;
        });
      });
      if (groups.length && Object.keys(groupedBlocks).length !== blocks.length) {
		throw new Error('Refined core groups do not account for every responsibility.');
	  }
	  if (groups.length && modelGroupCount === 0) {
		throw new Error('A local unassigned group has no model-owned grouping.');
	  }
	  var unassignedCount = blocks.length - Object.keys(modelGroupedBlocks).length;
	  if (groups.length && ((unassignedCount > 0 && localGroupCount !== 1) ||
	      (unassignedCount === 0 && localGroupCount !== 0))) {
		throw new Error('Local unassigned accounting does not match the model-owned memberships.');
      }
    }

    var openable = Object.create(null);
    array(data.openable_paths, 'openable_paths').forEach(function (path) {
      path = text(path, 'openable path');
      openable[path] = true;
    });

    var currentTarget = {
      id: text(target.id, 'program target.id'),
      language: text(target.language, 'program target.language'),
      kind: text(target.kind, 'program target.kind'),
      name: text(target.name, 'program target.name'),
      selector: text(target.selector, 'program target.selector'),
      indexSHA256: indexSHA256,
      sources: array(target.sources, 'program target.sources')
    };
    var targetDirectory = buildTargetDirectory(data, currentTarget);
    var surfaceCatalog = buildJSTSSurfaceCatalog(data, currentTarget, indexSHA256, openable);
    var crossSurfacePaths = buildCrossSurfacePaths(
      data, currentTarget, indexSHA256, openable, surfaceCatalog
    );
    if (!!surfaceCatalog !== !!crossSurfacePaths) {
      throw new Error('JavaScript/TypeScript surface and path authority must be published together.');
    }
    var isJSTS = currentTarget.language === 'javascript' || currentTarget.language === 'typescript';
    if (isJSTS !== !!surfaceCatalog) {
      throw new Error('JavaScript/TypeScript semantic targets require exact surface and path authority.');
    }
    var runtimePortfolio = data.runtime_portfolio == null ? null :
      buildRuntimePortfolio(data, targetDirectory, openable);

    return {
      raw: data,
      repoName: data.repo_name,
      revision: text(data.captured_revision, 'captured_revision'),
      capturedInputs: integer(data.captured_input_count, 'captured_input_count'),
      target: currentTarget,
      targets: targetDirectory.targets,
      targetsByID: targetDirectory.byID,
      defaultTargetID: targetDirectory.defaultID,
      runtimePortfolio: runtimePortfolio,
      surfaceCatalog: surfaceCatalog,
      crossSurfacePaths: crossSurfacePaths,
      blocks: blocks,
      blocksByID: blocksByID,
      blocksBySymbol: blocksBySymbol,
      groups: groups,
	  modelGroupCount: groups.filter(function (group) { return group.authority === 'model'; }).length,
	  unassignedBlockCount: groups.reduce(function (count, group) {
		return count + (group.authority === 'local_unassigned' ? group.blockIDs.length : 0);
	  }, 0),
      groupByBlock: groupByBlock,
      groupsByBlock: groupsByBlock,
      activityByID: activityByID,
      activities: activities,
      integrations: integrations,
      integrationsByID: integrationsByID,
      integrationUsesByKey: integrationUsesByKey,
      activityPaths: activityPaths,
      objectsByID: objectsByID,
      relations: relations,
      programProjection: programProjection,
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

  function sourceAction(label, rawLocation, display) {
    var location = exactLocation(rawLocation, 'source action location');
    display = display || {};
    if (!state.model.openable[location.path]) throw new Error('Source evidence is outside publication authority.');
    var control;
    if (state.source.mode === 'static') {
      control = element('a', 'rm-source-action' + (display.compact ? ' rm-source-action--compact' : ''));
      control.href = staticSourceURL(location);
      control.target = '_blank';
      control.rel = 'noopener noreferrer';
      control.title = 'Open the exact captured revision in ' + state.source.host;
    } else {
      var sourceID = state.source.ids[location.path];
      if (typeof sourceID !== 'string' || !sourceID) throw new Error('Source evidence lacks a manifest source ID.');
      control = element('button', 'rm-source-action' + (display.compact ? ' rm-source-action--compact' : ''));
      control.type = 'button';
      control.addEventListener('click', function () {
        requestOpenSource(sourceID, location).catch(function (error) {
          showToast(error && error.message ? error.message : String(error), true);
        });
      });
    }
    appendText(control, 'span', 'rm-source-action__name', label);
    var locationLabel = Object.prototype.hasOwnProperty.call(display, 'locationLabel') ?
      String(display.locationLabel || '') : formatLocation(location);
    if (locationLabel) appendText(control, 'span', 'rm-source-action__location', locationLabel);
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
      invokes_external: 'uses a resolved external/runtime API',
      implements: 'implements',
      decorates: 'decorates',
      passes_callback: 'passes control to',
      executes: 'executes',
      reads: 'reads',
      writes: 'writes'
    };
    return labels[kind] || kind.replace(/_/g, ' ');
  }

  function isJavaScriptPlatformObject(value) {
    return !!value && value.kind === 'external_symbol' && value.external &&
      value.external.package_path === 'platform:javascript';
  }

  function displayProgramObjectName(value) {
    var name = text(value.name, 'program object.name');
    if (isJavaScriptPlatformObject(value)) {
      var receiver = value.external.receiver || '';
      var member = value.external.name || '';
      var platformName = receiver && member ? receiver + '.' + member : receiver || member;
      return 'JavaScript ' + (platformName || name.replace(/^platform:javascript\.?/, ''));
    }
    var declarationSeparator = name.lastIndexOf('#');
    if (declarationSeparator >= 0 && declarationSeparator + 1 < name.length) {
      return name.slice(declarationSeparator + 1);
    }
    return name;
  }

  function displaySurfaceFactLabel(fact) {
    if (!fact.location || fact.category !== 'declaration') return fact.label;
    return displayProgramObjectName({ name: fact.label, kind: fact.kind });
  }

  function humanConnectionRelation(connection) {
    if (connection.kind !== 'invokes_external') return humanRelation(connection.kind);
    if (connection.platformTarget) {
      return connection.invocation === 'construct' ?
        'creates a JavaScript platform value' : 'uses a JavaScript platform API';
    }
    return connection.invocation === 'construct' ?
      'constructs through a resolved external/runtime API' : 'uses a resolved external/runtime API';
  }

  function humanIntegrationAuthority(value) {
    if (value === 'exact_external_symbol') return 'exact external symbol';
    if (value === 'syntactic_unresolved') return 'selected callsite; runtime unresolved';
    return value.replace(/_/g, ' ');
  }

  function relationLocations(relation) {
    var locations = [];
    function add(rawLocation, label) {
      if (!rawLocation) return;
      var location = exactLocation(rawLocation, label);
      if (locations.some(function (existing) {
        return existing.path === location.path && existing.line === location.line &&
          existing.column === location.column;
      })) return;
      locations.push(location);
    }
    add(relation.location, 'relation.location');
    var witnesses = Array.isArray(relation.witnesses) ? relation.witnesses : [];
    for (var index = 0; index < witnesses.length; index++) {
      if (witnesses[index]) add(witnesses[index].location, 'relation witness location');
    }
    return locations;
  }

  function relationWitnessAccounting(relation) {
    var witnesses = array(relation.witnesses, 'program relation.witnesses');
    var accounting = {
      observed: integer(relation.witnesses_observed, 'program relation.witnesses_observed'),
      indexed: integer(relation.witnesses_indexed, 'program relation.witnesses_indexed'),
      omitted: integer(relation.witnesses_omitted, 'program relation.witnesses_omitted'),
      projectedOmitted: integer(
        relation.witnesses_projection_omitted,
        'program relation.witnesses_projection_omitted'
      ),
      shown: witnesses.length
    };
    if (accounting.observed !== accounting.indexed + accounting.omitted ||
        accounting.indexed !== accounting.shown + accounting.projectedOmitted) {
      throw new Error('Program relation witness accounting is incomplete.');
    }
    return accounting;
  }

  function relationWitnessExpression(relation) {
    var witnesses = Array.isArray(relation.witnesses) ? relation.witnesses : [];
    for (var index = 0; index < witnesses.length; index++) {
      if (!witnesses[index]) continue;
      var expression = optionalText(witnesses[index].source_expression, 'relation witness source_expression');
      if (expression) return expression;
    }
    return '';
  }

  function connectionsFor(block) {
    var selected = Object.create(null);
    block.symbols.forEach(function (symbol) { selected[symbol.id] = symbol; });
    var connections = [];
    var grouped = Object.create(null);
    state.model.relations.forEach(function (rawRelation) {
      var relation = object(rawRelation, 'program relation');
      if (!selected[relation.from_id] || ['contains', 'imports', 'sources'].indexOf(relation.kind) >= 0) return;
      var from = state.model.objectsByID[relation.from_id];
      if (!from) return;
      var targetObjects = array(relation.to_ids, 'program relation.to_ids').map(function (id) {
        var target = state.model.objectsByID[id];
        return target || null;
      }).filter(Boolean);
      var targetNames = targetObjects.map(displayProgramObjectName);
      var relationID = text(relation.id, 'program relation.id');
      var kind = text(relation.kind, 'program relation.kind');
      var resolution = text(relation.resolution, 'program relation.resolution');
      var invocation = optionalText(relation.invocation, 'program relation.invocation');
      if (invocation && ['call', 'construct'].indexOf(invocation) < 0) {
        throw new Error('Program relation invocation is not supported.');
      }
      var key = relation.from_id + '\\u0000' + kind + '\\u0000' + resolution + '\\u0000' +
        invocation + '\\u0000' + relation.to_ids.join('\\u0000');
      if (!relation.to_ids.length) key += '\\u0000' + relationID;
      var locations = relationLocations(relation);
      var witnessAccounting = relationWitnessAccounting(relation);
      var expression = relationWitnessExpression(relation);
      var connection = grouped[key];
      if (connection) {
        connection.relationIDs.push(relationID);
        locations.forEach(function (location) {
          if (connection.locations.some(function (existing) {
            return existing.path === location.path && existing.line === location.line &&
              existing.column === location.column;
          })) return;
          connection.locations.push(location);
          if (!connection.location) connection.location = location;
        });
        connection.witnessesObserved += witnessAccounting.observed;
        connection.witnessesIndexed += witnessAccounting.indexed;
        connection.witnessesOmitted += witnessAccounting.omitted;
        connection.witnessesProjectionOmitted += witnessAccounting.projectedOmitted;
        return;
      }
      connection = {
        id: relationID,
        relationIDs: [relationID],
        fromID: relation.from_id,
        from: displayProgramObjectName(from),
        to: targetNames.length ? targetNames.join(' / ') :
          (expression ? 'unresolved call: ' + expression : 'runtime target unresolved'),
        kind: kind,
        invocation: invocation,
        platformTarget: targetObjects.length > 0 && targetObjects.every(isJavaScriptPlatformObject),
        externalTarget: targetObjects.length > 0 && targetObjects.every(function (target) {
          return target.kind === 'external_symbol';
        }),
        resolution: resolution,
        location: locations.length ? locations[0] : null,
        locations: locations,
        witnessesObserved: witnessAccounting.observed,
        witnessesIndexed: witnessAccounting.indexed,
        witnessesOmitted: witnessAccounting.omitted,
        witnessesProjectionOmitted: witnessAccounting.projectedOmitted,
        targetIDs: relation.to_ids
      };
      grouped[key] = connection;
      connections.push(connection);
    });
    var rank = { exact: 0, alternatives: 1, unresolved: 2 };
    connections.sort(function (left, right) {
      return (rank[left.resolution] || 0) - (rank[right.resolution] || 0) ||
        left.from.localeCompare(right.from) || left.to.localeCompare(right.to);
    });
    return connections;
  }

  function connectionSourceObject(connection) {
    var source = state.model.objectsByID[text(connection.fromID, 'connection.fromID')];
    if (!source) throw new Error('A displayed connection has no exact source object.');
    return source;
  }

  function connectionOwnerObject(connection) {
    var source = connectionSourceObject(connection);
    var ownerID = optionalText(source.owner_id, 'program object.owner_id');
    if (ownerID) {
      var owner = state.model.objectsByID[ownerID];
      if (!owner) throw new Error('A displayed connection owner is absent from the ProgramView.');
      return owner;
    }
    var containerID = optionalText(source.container_id, 'program object.container_id');
    if (!containerID) return source;
    var container = state.model.objectsByID[containerID];
    if (!container) throw new Error('A displayed connection container is absent from the ProgramView.');
    return container.kind === 'module' || container.kind === 'package' ? source : container;
  }

  function connectionBucket(connection) {
    if (connection.resolution === 'unresolved' || !connection.targetIDs.length) return 'unresolved';
    if (connection.platformTarget) return 'platform';
    if (connection.externalTarget) return 'external';
    return 'local';
  }

  function connectionPosition(connection) {
    var source = connectionSourceObject(connection);
    var location = connection.location || source.location || null;
    return {
      path: location && location.path ? location.path : '',
      line: location && location.line ? location.line : 0,
      column: location && location.column ? location.column : 0
    };
  }

  function compareConnections(left, right) {
    var leftPosition = connectionPosition(left);
    var rightPosition = connectionPosition(right);
    return leftPosition.path.localeCompare(rightPosition.path) ||
      leftPosition.line - rightPosition.line || leftPosition.column - rightPosition.column ||
      left.from.localeCompare(right.from) || left.to.localeCompare(right.to) || left.id.localeCompare(right.id);
  }

  function groupConnectionsByOwner(connections) {
    var groups = [];
    var groupsByID = Object.create(null);
    connections.forEach(function (connection) {
      var owner = connectionOwnerObject(connection);
      var group = groupsByID[owner.id];
      if (!group) {
        group = {
          id: owner.id,
          owner: owner,
          local: [],
          platform: [],
          external: [],
          unresolved: []
        };
        groupsByID[owner.id] = group;
        groups.push(group);
      }
      group[connectionBucket(connection)].push(connection);
    });
    groups.forEach(function (group) {
      ['local', 'platform', 'external', 'unresolved'].forEach(function (bucket) {
        group[bucket].sort(compareConnections);
      });
    });
    groups.sort(function (left, right) {
      var leftLocation = left.owner.location || null;
      var rightLocation = right.owner.location || null;
      var leftPath = leftLocation && leftLocation.path ? leftLocation.path : '';
      var rightPath = rightLocation && rightLocation.path ? rightLocation.path : '';
      return leftPath.localeCompare(rightPath) ||
        ((leftLocation && leftLocation.line) || 0) - ((rightLocation && rightLocation.line) || 0) ||
        displayProgramObjectName(left.owner).localeCompare(displayProgramObjectName(right.owner)) ||
        left.id.localeCompare(right.id);
    });
    return groups;
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
    return Object.keys(ids).map(function (id) { return state.model.blocksByID[id]; }).filter(Boolean);
  }

  function buildSystemCanvasGraph(activeBlockIDs, complete) {
    var graphAPI = globalThis.RepomapSystemCanvasGraph;
    if (!graphAPI || typeof graphAPI.buildCanvasGraph !== 'function') {
      throw new Error('The System canvas graph projection is unavailable.');
    }
    return graphAPI.buildCanvasGraph(state.model, {
      activeBlockIDs: activeBlockIDs,
      complete: complete
    });
  }

  function canvasNodeHref(node) {
    if (node.kind === 'entrypoint') return routeForActivity(node.entityID);
    if (node.kind === 'core') return routeForBlock(node.entityID);
    if (node.kind === 'integration') return routeForIntegration(node.entityID);
    throw new Error('The System canvas node kind is not supported.');
  }

  function navigateCanvasNode(node, event, activeGroup) {
    if (!node || node.kind !== 'core') return;
    if (event && (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey)) return;
    if (event && event.preventDefault) event.preventDefault();
    var block = state.model.blocksByID[node.entityID];
    if (!block) throw new Error('The System canvas responsibility is unavailable.');
    navigateToBlock(block, activeGroup, true);
  }

  function renderAreaSwitcher(selected, activeGroup) {
    var navigation = element('nav', 'rm-area-switcher');
    navigation.setAttribute('aria-label', 'Architecture areas and grouping coverage');
    var heading = element('div', 'rm-area-switcher__heading');
    appendText(heading, 'p', 'rm-eyebrow', 'Architecture areas and grouping coverage');
    var groupingSummary = String(state.model.modelGroupCount) + ' model-owned group' +
      (state.model.modelGroupCount === 1 ? '' : 's');
    if (state.model.unassignedBlockCount) {
      groupingSummary += ' · ' + String(state.model.unassignedBlockCount) + ' not placed by model';
    }
    appendText(heading, 'span', '', groupingSummary);
    navigation.appendChild(heading);
    var grid = element('div', 'rm-area-switcher__grid');
    state.model.groups.forEach(function (group) {
      var button = element('button', 'rm-area-switcher__item');
      button.type = 'button';
      button.title = group.purpose;
      var containsSelected = group.blockIDs.indexOf(selected.id) >= 0;
      if (containsSelected) button.className += ' rm-area-switcher__item--membership';
      if (activeGroup && group.id === activeGroup.id) button.setAttribute('aria-current', 'true');
      button.setAttribute('data-area-group', group.id);
      appendText(button, 'span', '', group.name);
      var groupMeta = String(group.blockIDs.length) + (containsSelected ? ' · member' : '');
      if (group.authority === 'local_unassigned') groupMeta += ' · local accounting';
      appendText(button, 'small', '', groupMeta);
      button.addEventListener('click', function () {
        state.pendingAreaGroupFocusID = group.id;
        navigateToBlock(state.model.blocksByID[group.blockIDs[0]], group, false);
      });
      if (activeGroup && group.id === activeGroup.id && state.pendingAreaGroupFocusID === group.id) {
        state.pendingAreaGroupFocusID = '';
        window.requestAnimationFrame(function () { button.focus({ preventScroll: true }); });
      }
      grid.appendChild(button);
    });
    navigation.appendChild(grid);
    return navigation;
  }

  function canvasLanePresentation(graph, activeGroup) {
    var accounting = graph.accounting;
    var coreEdges = graph.edges.filter(function (edge) {
      return edge.sourceID.indexOf('core:') === 0 && edge.targetID.indexOf('core:') === 0;
    });
    var visibleCoreBlocks = graph.nodesByLane.core.length;
    var completeGroupSummary = String(state.model.modelGroupCount) + ' model groups';
    if (state.model.unassignedBlockCount) {
      completeGroupSummary += ' · ' + String(state.model.unassignedBlockCount) + ' not placed';
    }
    var coreSummary = !state.model.groups.length ? String(visibleCoreBlocks) + ' responsibilities' :
      graph.visibility.complete ? String(visibleCoreBlocks) + ' responsibilities / ' + completeGroupSummary :
        String(visibleCoreBlocks) + ' in current grouping selection / ' +
          String(state.model.blocks.length) + ' responsibilities';
    coreSummary += ' · ' + String(coreEdges.length) + ' directional core link' +
      (coreEdges.length === 1 ? '' : 's');

    var notes = { entry: [], integration: [] };
    if (!graph.visibility.complete && accounting.hiddenPlacedStarts > 0) {
      notes.entry.push(String(accounting.hiddenPlacedStarts) +
        ' placed starts connect to other grouping selections.');
    }
    if (accounting.unplacedStarts > 0) {
      notes.entry.push(String(accounting.unplacedStarts) +
        ' selected starts have no exact representative-member binding.');
    }
    if (!graph.visibility.complete && accounting.hiddenConnectedIntegrations > 0) {
      notes.integration.push(String(accounting.hiddenConnectedIntegrations) +
        ' connected integrations connect only through other grouping selections.');
    }
    if (accounting.unplacedIntegrations > 0) {
      notes.integration.push(String(accounting.unplacedIntegrations) +
        ' selected integrations have no exact representative-caller binding.');
    }
    return {
      laneSummaries: {
        entry: String(graph.nodesByLane.entry.length) + ' shown / ' +
          String(accounting.connectedStartCount) + ' connected / ' +
          String(state.model.activities.length) + ' selected',
        core: coreSummary,
        integration: String(graph.nodesByLane.integration.length) + ' shown / ' +
          String(accounting.connectedIntegrationCount) + ' connected / ' +
          String(state.model.integrations.length) + ' selected'
      },
      laneEmptyMessages: {
        entry: 'No selected entrypoint is an exact representative member of a responsibility.',
        integration: 'No model-selected integration operations for this target.'
      },
      laneNotes: notes,
      coreLeadNote: coreEdges.length ? '' :
        'No exact or possible program relations connect the representative declarations in this view.',
      coreGroup: !graph.visibility.complete && activeGroup ? {
        id: activeGroup.id,
        authority: activeGroup.authority,
        eyebrow: activeGroup.authority === 'local_unassigned' ? 'Local grouping coverage' : '',
        name: activeGroup.name,
        purpose: activeGroup.purpose,
        summary: String(activeGroup.blockIDs.length) + ' responsibilit' +
          (activeGroup.blockIDs.length === 1 ? 'y' : 'ies')
      } : null
    };
  }

  function renderFlowCanvas(selected) {
    var memberships = state.model.groupsByBlock[selected.id] || [];
    var activeGroup = memberships.find(function (group) { return group.id === state.focusGroupID; }) ||
      state.model.groupByBlock[selected.id] || null;
    state.focusGroupID = activeGroup ? activeGroup.id : null;
    var activeBlockIDs = activeGroup ? activeGroup.blockIDs :
      (state.model.groups.length ? [selected.id] : state.model.blocks.map(function (block) { return block.id; }));
    var graph = buildSystemCanvasGraph(activeBlockIDs, state.completeCanvas);
    var section = element('section', 'rm-flow-section');
    section.setAttribute('aria-labelledby', 'rm-flow-title');
    var header = element('header', 'rm-flow-section__header');
    var copy = element('div');
    appendText(copy, 'p', 'rm-eyebrow', 'System canvas');
    var title = appendText(copy, 'h2', '', 'Repository flow');
    title.id = 'rm-flow-title';
    var focusIntro = state.model.groups.length ?
      'Focused on ' + (activeGroup ? activeGroup.name : selected.name) +
        '. Other grouping selections remain available in the switcher below.' :
      'Focused detail: ' + selected.name + '. All responsibility cards remain visible.';
    if (activeGroup && activeGroup.authority === 'local_unassigned') {
      focusIntro = 'Focused on responsibilities not placed by the grouping model. ' +
        'Model-owned architecture areas remain available in the switcher below.';
    }
    if (graph.accounting.directFrontierEdges > 0) {
      focusIntro += ' ' + String(graph.accounting.directFrontierEdges) +
        ' direct entrypoint-to-integration ' +
        (graph.accounting.directFrontierEdges === 1 ? 'path remains' : 'paths remain') +
        ' visible as a direct frontier outside core grouping.';
    }
    appendText(copy, 'p', 'rm-flow-section__intro', state.completeCanvas ?
      'The complete selected map is visible. Exact, possible, and unresolved runtime authority remain distinct.' :
      focusIntro);
    header.appendChild(copy);
    var controls = element('div', 'rm-canvas-controls');
    var modeLabel = state.completeCanvas ? 'Focus current grouping selection' :
      'Show complete map · ' + String(graph.accounting.totalNodes) + ' nodes / ' +
        String(graph.accounting.totalEdges) + ' bindings';
    var mode = element('button', 'rm-canvas-mode', modeLabel);
    mode.type = 'button';
    mode.setAttribute('aria-pressed', state.completeCanvas ? 'true' : 'false');
    mode.addEventListener('click', function () {
      state.completeCanvas = !state.completeCanvas;
      renderRoute();
      window.requestAnimationFrame(function () {
        var nextMode = document.querySelector('.rm-canvas-mode');
        if (nextMode) nextMode.focus();
      });
    });
    controls.appendChild(mode);
    var legend = element('div', 'rm-canvas-legend');
    appendText(legend, 'span', 'rm-canvas-legend__exact', 'Exact local binding');
    appendText(legend, 'span', 'rm-canvas-legend__possible', 'Possible local path');
    appendText(legend, 'span', 'rm-canvas-legend__runtime',
      'Selected callsite; runtime unresolved');
    controls.appendChild(legend);
    header.appendChild(controls);
    section.appendChild(header);
    if (state.model.groups.length) section.appendChild(renderAreaSwitcher(selected, activeGroup));

    var canvasHost = element('div', 'rm-system-canvas-host');
    section.appendChild(canvasHost);
    var presentation = canvasLanePresentation(graph, activeGroup);
    return {
      element: section,
      mount: function () {
        var renderer = globalThis.RepomapSystemCanvasRenderer;
        if (!renderer || typeof renderer.mountSystemCanvas !== 'function') {
          throw new Error('The System canvas renderer is unavailable.');
        }
        return renderer.mountSystemCanvas(canvasHost, graph, {
          selectedNodeID: 'core:' + selected.id,
          laneSummaries: presentation.laneSummaries,
          laneEmptyMessages: presentation.laneEmptyMessages,
          laneNotes: presentation.laneNotes,
          coreLeadNote: presentation.coreLeadNote,
          coreGroup: presentation.coreGroup
        }, {
          hrefForNode: canvasNodeHref,
          navigateNode: function (node, event) {
            navigateCanvasNode(node, event, activeGroup);
          }
        });
      }
    };
  }

  function unmountSystemCanvas() {
    if (!state || !state.canvasMount) return;
    state.canvasMount.unmount();
    state.canvasMount = null;
  }
  function routeForBlock(id) {
    return '#/program/responsibility/' + encodePathSegment(id);
  }

  function routeForActivity(id) {
    return '#/program/entrypoint/' + encodePathSegment(id);
  }

  function routeForIntegration(id) {
    return '#/program/integration/' + encodePathSegment(id);
  }

  function routeForSurface(id) {
    return '#/program/surface/' + encodePathSegment(id);
  }

  function routeForCrossSurfacePath(id) {
    return '#/program/path/' + encodePathSegment(id);
  }

  function decodeRouteIdentity(value) {
    try {
      var result = decodeURIComponent(value);
      return text(result, 'report route identity');
    } catch (error) {
      throw new Error('The requested report route is malformed.');
    }
  }

  function selectedReportRoute() {
    var hash = window.location.hash || '';
    if (hash === '' || hash === '#/') {
      return repositoryOverviewKind() ? { kind: 'repository' } : { kind: 'program' };
    }
    if (hash === '#/repository') return { kind: 'repository' };
    if (hash === '#/program') return { kind: 'program' };
    var match = hash.match(/^#\/program\/(responsibility|entrypoint|integration|surface|path)\/([^/?#]+)$/);
    if (!match) throw new Error('The requested report route is not supported.');
    var id = decodeRouteIdentity(match[2]);
    if (match[1] === 'responsibility') {
      var block = state.model.blocksByID[id];
      if (!block) throw new Error('The requested responsibility is not part of this report.');
      return { kind: 'responsibility', block: block };
    }
    if (match[1] === 'entrypoint') {
      var activity = state.model.activityByID[id];
      if (!activity) throw new Error('The requested entrypoint is not part of this report.');
      return { kind: 'entrypoint', activity: activity };
    }
    if (match[1] === 'integration') {
      var integration = state.model.integrationsByID[id];
      if (!integration) throw new Error('The requested integration is not part of this report.');
      return { kind: 'integration', integration: integration };
    }
    if (match[1] === 'surface') {
      var catalog = state.model.surfaceCatalog;
      if (!catalog || !catalog.surfacesByID[id]) throw new Error('The requested surface is not part of this report.');
      return { kind: 'surface', surface: catalog.surfacesByID[id] };
    }
    var paths = state.model.crossSurfacePaths;
    if (!paths || !paths.pathsByID[id]) throw new Error('The requested cross-surface path is not part of this report.');
    return { kind: 'path', path: paths.pathsByID[id] };
  }

  function repositoryOverviewKind() {
    if (state.model.runtimePortfolio &&
        (state.model.runtimePortfolio.roles.length || state.model.runtimePortfolio.unclassified.length)) {
      return 'runtime';
    }
    return '';
  }

  function scheduleResponsibilityScroll(blockID) {
    window.requestAnimationFrame(function () {
      var detail = document.querySelector('.rm-focus[data-responsibility-detail]');
      if (!detail || detail.getAttribute('data-responsibility-detail') !== blockID) return;
      if (typeof detail.scrollIntoView === 'function') {
        var reducedMotion = typeof window.matchMedia === 'function' &&
          window.matchMedia('(prefers-reduced-motion: reduce)').matches;
        detail.scrollIntoView({ behavior: reducedMotion ? 'auto' : 'smooth', block: 'start' });
      }
      if (typeof detail.focus === 'function') detail.focus({ preventScroll: true });
    });
  }

  function navigateToBlock(block, group, scrollToDetail) {
    state.focusGroupID = group ? group.id : null;
    if (group) state.completeCanvas = false;
    state.pendingResponsibilityID = scrollToDetail === false ? '' : block.id;
    var route = routeForBlock(block.id);
    if (window.location.hash === route) {
      renderRoute();
      return;
    }
    window.location.hash = route;
  }

  function shortRepositoryName(value) {
    var parts = String(value || '').split('/').filter(Boolean);
    return parts.length > 1 ? parts.slice(-2).join('/') : String(value || 'repository');
  }

  function reportRouteContext(route) {
    if (route.kind === 'repository') return 'Repository overview';
    if (route.kind === 'surface') return 'Surface';
    if (route.kind === 'path') return 'Full-stack path';
    if (route.kind === 'entrypoint') return 'Entrypoint';
    if (route.kind === 'integration') return 'Integration';
    if (route.kind === 'responsibility') return 'Responsibility';
    return 'Program map';
  }

  function updateHeaderContext(route) {
    var scope = document.getElementById('rm-page-context');
    var revision = state.model.revision.slice(0, MAX_REVISION_ABBREVIATION_CHARS);
    var repositoryRoute = route.kind === 'repository';
    if (scope) {
      scope.textContent = repositoryRoute ?
        reportRouteContext(route) + ' · ' + String(state.model.targets.length) +
          (state.model.targets.length === 1 ? ' target' : ' targets') + ' · ' + revision :
        reportRouteContext(route) + ' · ' + humanRuntimeToken(state.model.target.language) + ' · ' +
          humanRuntimeToken(state.model.target.kind) + ' · ' + revision;
    }
    var current = document.getElementById('rm-target-current');
    if (current) current.textContent = repositoryRoute ? 'All targets' : 'Target · ' + state.model.target.name;
    (state.headerTargetLinks || []).forEach(function (entry) {
      var currentPage = route.kind === 'program' && entry.target.id === state.model.target.id;
      entry.link.setAttribute('aria-current', currentPage ? 'page' : 'false');
      if (entry.currentBadge) {
        entry.currentBadge.hidden = !currentPage;
      }
    });
  }

  function renderHeader() {
    document.getElementById('rm-target-repository').textContent = shortRepositoryName(state.model.repoName);
    document.getElementById('rm-target-count').textContent = String(state.model.targets.length);
    document.getElementById('rm-target-panel-count').textContent =
      String(state.model.targets.length) + (state.model.targets.length === 1 ? ' target' : ' targets');
    var switcher = document.getElementById('rm-target-switcher');
    var navigation = document.getElementById('rm-target-navigation');
    state.headerTargetLinks = [];
    state.model.targets.forEach(function (target) {
      var link = element('a', 'rm-target-switcher__target');
      link.href = target.href;
      var copy = element('span');
      appendText(copy, 'strong', '', target.displayName);
      appendText(copy, 'small', '', humanRuntimeToken(target.language) + ' · ' + humanRuntimeToken(target.kind));
      link.appendChild(copy);
      var badges = element('span', 'rm-target-switcher__badges');
      var currentBadge = null;
      if (target.id === state.model.target.id) {
        currentBadge = element('small', 'rm-target-switcher__badge rm-target-switcher__badge--current', 'Current');
        currentBadge.hidden = true;
        badges.appendChild(currentBadge);
      }
      if (target.id === state.model.defaultTargetID) {
        appendText(badges, 'small', 'rm-target-switcher__badge', 'Default');
      }
      link.appendChild(badges);
      link.addEventListener('click', function () { switcher.open = false; });
      navigation.appendChild(link);
      state.headerTargetLinks.push({ target: target, link: link, currentBadge: currentBadge });
    });
    switcher.addEventListener('keydown', function (event) {
      if (event.key !== 'Escape' || !switcher.open) return;
      switcher.open = false;
      var summary = switcher.querySelector('summary');
      if (summary) summary.focus();
    });
    updateHeaderContext(selectedReportRoute());
  }

  function humanRuntimeToken(value) {
    if (value === 'cli') return 'CLI';
    if (value === 'go') return 'Go';
    if (value === 'javascript') return 'JavaScript';
    if (value === 'typescript') return 'TypeScript';
    if (value === 'python') return 'Python';
    return value.replace(/_/g, ' ').replace(/^./, function (character) { return character.toUpperCase(); });
  }

  function runtimeBadge(parent, value, kind) {
    appendText(parent, 'span', 'rm-runtime-badge rm-runtime-badge--' + kind, humanRuntimeToken(value));
  }

  function runtimeEvidenceGroups(evidence) {
    var byPath = Object.create(null);
    var groups = [];
    evidence.forEach(function (fact) {
      var path = fact.location.path;
      var group = byPath[path];
      if (!group) {
        group = { path: path, factCount: 0, locations: [], locationsByKey: Object.create(null) };
        byPath[path] = group;
        groups.push(group);
      }
      group.factCount++;
      var key = String(fact.location.line) + ':' + String(fact.location.column);
      if (!group.locationsByKey[key]) {
        group.locationsByKey[key] = true;
        group.locations.push(fact.location);
      }
    });
    groups.sort(function (left, right) { return left.path.localeCompare(right.path); });
    groups.forEach(function (group) {
      group.locations.sort(function (left, right) {
        return left.line - right.line || left.column - right.column;
      });
    });
    return groups;
  }

  function renderRuntimeEvidence(role) {
    var groups = runtimeEvidenceGroups(role.evidence);
    var disclosure = element('details', 'rm-runtime-evidence');
    appendText(disclosure, 'summary', '', 'Evidence · ' + String(role.evidence.length) +
      (role.evidence.length === 1 ? ' fact' : ' facts') + ' · ' + String(groups.length) +
      (groups.length === 1 ? ' file' : ' files'));
    var files = element('div', 'rm-runtime-evidence__files');
    groups.forEach(function (group) {
      var file = element('details', 'rm-runtime-evidence-file');
      var summary = element('summary');
      appendText(summary, 'span', '', group.path);
      appendText(summary, 'small', '', String(group.factCount) +
        (group.factCount === 1 ? ' fact' : ' facts') + ' · ' + String(group.locations.length) +
        (group.locations.length === 1 ? ' location' : ' locations'));
      file.appendChild(summary);
      var locations = element('div', 'rm-runtime-evidence-file__locations');
      group.locations.forEach(function (location) {
        var label = location.line > 0 ? 'L' + String(location.line) +
          (location.column > 0 ? ':' + String(location.column) : '') : 'File';
        locations.appendChild(sourceAction(label, location, { compact: true, locationLabel: '' }));
      });
      file.appendChild(locations);
      files.appendChild(file);
    });
    disclosure.appendChild(files);
    return disclosure;
  }

  function renderRuntimeRole(role) {
    var card = element('article', 'rm-runtime-card');
    var heading = element('div', 'rm-runtime-card__heading');
    appendText(heading, 'h3', '', role.name);
    var badges = element('div', 'rm-runtime-card__badges');
    runtimeBadge(badges, role.roleKind, 'kind');
    if (role.roleKind === 'library' && role.prominence !== 'primary') {
      runtimeBadge(badges, role.prominence === 'unknown' ? 'unknown prominence' : role.prominence,
        'prominence-' + role.prominence);
    }
    if (role.requiredness !== 'optional') {
      runtimeBadge(badges, role.requiredness === 'required' ? 'required role' : role.requiredness,
        'requiredness-' + role.requiredness);
    }
    if (role.confidence !== 'high') {
      runtimeBadge(badges, role.confidence + ' confidence', 'confidence-' + role.confidence);
    }
    if (badges.children.length) heading.appendChild(badges);
    card.appendChild(heading);
    appendText(card, 'p', 'rm-runtime-card__purpose', role.purpose);

    if (!role.implementations.length) {
      appendText(card, 'p', 'rm-runtime-card__unresolved', 'Target mapping unresolved.');
    } else {
      var implementations = element('div', 'rm-runtime-card__implementations');
      role.implementations.forEach(function (implementation) {
        var link = element('a', 'rm-runtime-target');
        link.href = implementation.target.href;
        appendText(link, 'span', '', implementation.target.displayName + ' →');
        appendText(link, 'small', '', implementation.mode ?
          humanRuntimeToken(implementation.mode) + ' · ' + humanRuntimeToken(implementation.target.language) + ' ' +
            humanRuntimeToken(implementation.target.kind) :
          humanRuntimeToken(implementation.target.language) + ' ' + humanRuntimeToken(implementation.target.kind));
        implementations.appendChild(link);
      });
      card.appendChild(implementations);
    }

    if (role.evidence.length) {
      card.appendChild(renderRuntimeEvidence(role));
    }
    return card;
  }

  function renderRuntimeRoleSection(host, sectionKind, title, intro, roles) {
    if (!roles.length) return;
    var section = element('section', 'rm-runtime-section rm-runtime-section--' + sectionKind);
    var heading = element('header', 'rm-runtime-section__header');
    appendText(heading, 'h2', '', title);
    appendText(heading, 'p', '', intro);
    section.appendChild(heading);
    var grid = element('div', 'rm-runtime-grid');
    roles.forEach(function (role) { grid.appendChild(renderRuntimeRole(role)); });
    section.appendChild(grid);
    host.appendChild(section);
  }

  function renderUnclassifiedRuntimeTargets(host) {
    var targets = state.model.runtimePortfolio.unclassified;
    if (!targets.length) return;
    var section = element('section', 'rm-runtime-section rm-runtime-section--unclassified');
    var heading = element('header', 'rm-runtime-section__header');
    appendText(heading, 'h2', '', 'Unclassified targets');
    appendText(heading, 'p', '',
      'These selected targets have exact target-local reports, but the repository portfolio did not assign them a supported role.');
    section.appendChild(heading);
    var list = element('div', 'rm-runtime-unclassified');
    targets.forEach(function (unclassified) {
      var link = element('a', 'rm-runtime-unclassified__item');
      link.href = unclassified.target.href;
      var copy = element('span');
      appendText(copy, 'strong', '', unclassified.target.displayName);
      appendText(copy, 'small', '', unclassified.reason);
      link.appendChild(copy);
      appendText(link, 'span', 'rm-runtime-unclassified__action', 'Open target map →');
      list.appendChild(link);
    });
    section.appendChild(list);
    host.appendChild(section);
  }

  function renderRuntimePortfolio(host) {
    var runtime = state.model.runtimePortfolio;
    var survey = element('section', 'rm-survey rm-runtime-survey');
    var copy = element('div');
    appendText(copy, 'p', 'rm-eyebrow', 'Repository overview');
    appendText(copy, 'h1', '', state.model.repoName);
    var summary = 'Understand ' + String(runtime.roles.length) +
      (runtime.roles.length === 1 ? ' repository role' : ' repository roles') + ' across ' +
      String(state.model.targets.length) + (state.model.targets.length === 1 ? ' selected target.' : ' selected targets.');
    if (runtime.unclassified.length) {
      summary += ' ' + String(runtime.unclassified.length) +
        (runtime.unclassified.length === 1 ? ' target remains unclassified.' : ' targets remain unclassified.');
    }
    appendText(copy, 'p', 'rm-survey__summary', summary);
    survey.appendChild(copy);
    var facts = element('dl', 'rm-survey__facts');
    [['Roles', runtime.roles.length], ['Targets', state.model.targets.length],
      ['Revision', state.model.revision]].forEach(function (row) {
      var wrapper = element('div');
      appendText(wrapper, 'dt', '', row[0]);
      appendText(wrapper, 'dd', row[0] === 'Revision' ? 'rm-runtime-revision' : '', row[1]);
      facts.appendChild(wrapper);
    });
    survey.appendChild(facts);
    host.appendChild(survey);

    var libraryRoles = runtime.roles.filter(function (role) { return role.roleKind === 'library'; });
    var exampleRoles = runtime.roles.filter(function (role) { return role.roleKind === 'example'; });
    var toolRoles = runtime.roles.filter(function (role) { return role.roleKind === 'supporting_tool'; });
    var runnableRoles = runtime.roles.filter(function (role) {
      return ['library', 'example', 'supporting_tool', 'unknown'].indexOf(role.roleKind) < 0;
    });
    var uncertainRoles = runtime.roles.filter(function (role) {
      return role.roleKind === 'unknown' ||
        (['library', 'example', 'supporting_tool'].indexOf(role.roleKind) < 0 && role.prominence === 'unknown');
    });
    renderRuntimeRoleSection(host, 'library', 'Libraries and product APIs',
      'Reusable packages and public APIs that form the product this repository delivers.', libraryRoles);
    renderRuntimeRoleSection(host, 'primary', 'Primary runtime roles',
      'The central services, daemons, workers, and command surfaces that explain how this repository runs.',
      runnableRoles.filter(function (role) { return role.prominence === 'primary'; }));
    renderRuntimeRoleSection(host, 'example', 'Examples',
      'Runnable demonstrations and tutorials that show how the repository product is used.', exampleRoles);
    renderRuntimeRoleSection(host, 'tool', 'Supporting tools',
      'Build, release, migration, generator, and operational utilities.', toolRoles);
    renderRuntimeRoleSection(host, 'supporting', 'Other supporting roles',
      'Secondary runnable or operational roles that are neither examples nor tools.',
      runnableRoles.filter(function (role) { return role.prominence === 'supporting'; }));
    renderRuntimeRoleSection(host, 'unknown', 'Uncertain roles',
      'Evidence supports these roles, but their kind or repository prominence remains uncertain.', uncertainRoles);
    renderUnclassifiedRuntimeTargets(host);
  }

  function humanSurfaceToken(value) {
    if (value === 'node_server') return 'Node server';
    if (value === 'command_line_application') return 'Command-line application';
    if (value === 'shared_contracts') return 'Shared contracts';
    if (value === 'browser_application') return 'Browser application';
    if (value === 'http_method_path_match') return 'HTTP method/path match';
    return value.replace(/_/g, ' ').replace(/^./, function (character) { return character.toUpperCase(); });
  }

  function renderJSTSSurfaceCard(surface) {
    var card = element('article', 'rm-surface-card rm-surface-card--' + surface.disposition);
    var heading = element('div', 'rm-surface-card__heading');
    appendText(heading, 'p', 'rm-eyebrow', humanSurfaceToken(surface.kind));
    var link = element('a', 'rm-surface-card__link', surface.name);
    link.href = routeForSurface(surface.id);
    heading.appendChild(link);
    card.appendChild(heading);
    appendText(card, 'p', 'rm-surface-card__meta', humanSurfaceToken(surface.role) + ' · ' +
      String(surface.entryRefs.length) + (surface.entryRefs.length === 1 ? ' entry' : ' entries') + ' · ' +
      String(surface.evidenceRefs.length) + ' evidence refs');
    card.appendChild(sourceAction('Open surface evidence', surface.location));
    return card;
  }

  function renderJSTSSurfaceSection(host, disposition, title, intro) {
    var surfaces = state.model.surfaceCatalog.surfaces.filter(function (surface) {
      return surface.disposition === disposition;
    });
    if (!surfaces.length) return;
    var section = element('section', 'rm-surface-section rm-surface-section--' + disposition);
    var heading = element('header', 'rm-runtime-section__header');
    appendText(heading, 'h2', '', title);
    appendText(heading, 'p', '', intro);
    section.appendChild(heading);
    var grid = element('div', 'rm-surface-grid');
    surfaces.forEach(function (surface) { grid.appendChild(renderJSTSSurfaceCard(surface)); });
    section.appendChild(grid);
    host.appendChild(section);
  }

  function renderCrossSurfacePathCards(host) {
    var paths = state.model.crossSurfacePaths.paths;
    var section = element('section', 'rm-surface-section rm-cross-path-section');
    var heading = element('header', 'rm-runtime-section__header');
    appendText(heading, 'h2', '', 'Full-stack paths');
    appendText(heading, 'p', '',
      'Deterministic routes across browser code, the explicit HTTP method/path boundary, server handling, contracts, and resources.');
    section.appendChild(heading);
    if (!paths.length) {
      appendText(section, 'p', 'rm-runtime-empty', crossSurfaceEmptyReason(
        state.model.surfaceCatalog, state.model.crossSurfacePaths.coverage
      ));
    } else {
      var grid = element('div', 'rm-cross-path-grid');
      paths.forEach(function (path) {
        var card = element('a', 'rm-cross-path-card');
        card.href = routeForCrossSurfacePath(path.id);
        appendText(card, 'p', 'rm-eyebrow', String(path.steps.length) + ' grounded steps');
        appendText(card, 'h3', '', path.name);
        appendText(card, 'p', '', path.outcome);
        if (path.frontier) appendText(card, 'small', 'rm-cross-path-card__frontier', path.frontier);
        grid.appendChild(card);
      });
      section.appendChild(grid);
    }
    host.appendChild(section);
  }

  function renderTargetSurfaceInventory(host) {
    if (!state.model.surfaceCatalog || !state.model.crossSurfacePaths) return;
    renderJSTSSurfaceSection(host, 'product_surface', 'Product surfaces',
      'Runnable browser, server, and command-line surfaces established by exact project evidence.');
    renderJSTSSurfaceSection(host, 'supporting_code', 'Supporting code',
      'Shared contracts and other exact supporting boundaries that are not independent runtime processes.');
    renderJSTSSurfaceSection(host, 'tool', 'Tools and scripts',
      'Project scripts and tooling surfaces kept separate from product runtime roles.');
    renderJSTSSurfaceSection(host, 'unknown', 'Unclassified surfaces',
      'Exact surface evidence is present, but the producer could not assign a supported disposition.');
    renderCrossSurfacePathCards(host);
  }

  function crossSurfaceEmptyReason(catalog, coverage) {
    var productKinds = Object.create(null);
    catalog.surfaces.forEach(function (surface) {
      if (surface.disposition === 'product_surface') productKinds[surface.kind] = true;
    });
    var browser = !!productKinds.browser_application;
    var server = !!productKinds.node_server;
    if (!browser && !server) {
      return 'No eligible full-stack path: this target has neither a product browser surface nor a product Node server. Tooling and development servers do not count as product runtime surfaces.';
    }
    if (!browser) {
      return 'No eligible full-stack path: this target has no product browser surface. Tooling and development clients do not count as product runtime surfaces.';
    }
    if (!server) {
      return 'No eligible full-stack path: this target has no product Node server. Tooling and development servers do not count as product runtime surfaces.';
    }
    if (coverage.http_uses_observed === 0) {
      return 'No eligible full-stack path: the product browser has no retained client HTTP use.';
    }
    if (coverage.routes_observed === 0) {
      return 'No eligible full-stack path: the product Node server has no retained server route.';
    }
    return 'No complete full-stack path: the product browser and Node server have no retained explicit HTTP method/path match with program reachability on both sides.';
  }

  function renderRepositoryFallback(host) {
    var survey = element('section', 'rm-survey');
    var copy = element('div');
    appendText(copy, 'p', 'rm-eyebrow', 'Repository overview');
    appendText(copy, 'h1', '', state.model.repoName);
    appendText(copy, 'p', 'rm-survey__summary',
      'Open the selected program map to inspect its exact structural and semantic evidence.');
    var link = element('a', 'rm-survey__overview-link', 'Open program map →');
    link.href = '#/program';
    copy.appendChild(link);
    survey.appendChild(copy);
    host.appendChild(survey);
  }

  function renderSurfaceFacts(parent, title, refs, factsByRef) {
    var section = element('section', 'rm-focus-section');
    appendText(section, 'h3', '', title);
    if (!refs.length) {
      appendText(section, 'p', 'rm-empty', 'No exact references were recorded for this group.');
    } else {
      var list = element('ul', 'rm-evidence-list rm-surface-fact-list');
      refs.forEach(function (ref) {
        var fact = factsByRef[ref];
        var item = element('li');
        if (fact.location) item.appendChild(sourceAction(displaySurfaceFactLabel(fact), fact.location));
        else appendText(item, 'code', '', fact.label);
        appendText(item, 'small', '', humanSurfaceToken(fact.category) + ' · ' + humanSurfaceToken(fact.kind));
        list.appendChild(item);
      });
      section.appendChild(list);
    }
    parent.appendChild(section);
  }

  function pathsForSurface(surface) {
    var relevant = Object.create(null);
    relevant[surface.id] = true;
    surface.entryRefs.forEach(function (ref) { relevant[ref] = true; });
    surface.evidenceRefs.forEach(function (ref) { relevant[ref] = true; });
    return state.model.crossSurfacePaths.paths.filter(function (path) {
      return path.steps.some(function (step) {
        if (relevant[step.sourceRef]) return true;
        return step.targetRefs.some(function (ref) { return !!relevant[ref]; });
      });
    });
  }

  function renderSurfaceDetail(host, surface) {
    var hero = element('section', 'rm-detail-hero');
    var back = element('a', 'rm-survey__overview-link', '← Back to target overview');
    back.href = '#/program';
    hero.appendChild(back);
    appendText(hero, 'p', 'rm-eyebrow', humanSurfaceToken(surface.kind));
    appendText(hero, 'h1', '', surface.name);
    appendText(hero, 'p', 'rm-detail-hero__summary',
      humanSurfaceToken(surface.disposition) + ' · ' + humanSurfaceToken(surface.role));
    hero.appendChild(sourceAction('Open surface evidence', surface.location));
    host.appendChild(hero);
    var detail = element('article', 'rm-surface-detail');
    renderSurfaceFacts(detail, 'Entries', surface.entryRefs, state.model.surfaceCatalog.factsByRef);
    renderSurfaceFacts(detail, 'Source evidence', surface.evidenceRefs, state.model.surfaceCatalog.factsByRef);
    var semantic = element('section', 'rm-focus-section');
    appendText(semantic, 'h3', '', 'Responsibilities and integrations');
    appendText(semantic, 'p', 'rm-focus-section__intro',
      'Continue into the semantic program map for the complete responsibility and selected integration views bound to this same target.');
    var programLink = element('a', 'rm-survey__overview-link', 'Open semantic program map →');
    programLink.href = '#/program';
    semantic.appendChild(programLink);
    detail.appendChild(semantic);
    var related = pathsForSurface(surface);
    var section = element('section', 'rm-focus-section');
    appendText(section, 'h3', '', 'Full-stack paths');
    if (!related.length) {
      appendText(section, 'p', 'rm-empty', 'No exact cross-surface path cites this surface or its bound facts.');
    } else {
      var list = element('div', 'rm-cross-path-grid');
      related.forEach(function (path) {
        var link = element('a', 'rm-cross-path-card');
        link.href = routeForCrossSurfacePath(path.id);
        appendText(link, 'h4', '', path.name);
        appendText(link, 'p', '', path.outcome);
        list.appendChild(link);
      });
      section.appendChild(list);
    }
    detail.appendChild(section);
    host.appendChild(detail);
  }

  function renderCrossSurfacePathDetail(host, path) {
    var hero = element('section', 'rm-detail-hero');
    var back = element('a', 'rm-survey__overview-link', '← Back to target overview');
    back.href = '#/program';
    hero.appendChild(back);
    appendText(hero, 'p', 'rm-eyebrow', 'Full-stack path');
    appendText(hero, 'h1', '', path.name);
    appendText(hero, 'p', 'rm-detail-hero__summary', path.outcome);
    if (path.frontier) appendText(hero, 'p', 'rm-path-frontier', 'Frontier: ' + path.frontier);
    host.appendChild(hero);
    var timeline = element('ol', 'rm-path-timeline');
    path.steps.forEach(function (step) {
      var fact = state.model.crossSurfacePaths.factsByRef[step.sourceRef];
      var item = element('li', 'rm-path-step rm-path-step--' + step.authority +
        (step.kind === 'http_method_path_match' ? ' rm-path-step--http-boundary' : ''));
      var heading = element('div', 'rm-path-step__heading');
      appendText(heading, 'span', 'rm-path-step__ordinal', String(step.ordinal).padStart(2, '0'));
      var copy = element('div');
      appendText(copy, 'p', 'rm-eyebrow', humanSurfaceToken(step.kind));
      appendText(copy, 'h2', '', step.label);
      heading.appendChild(copy);
      var badges = element('div', 'rm-runtime-card__badges');
      runtimeBadge(badges, step.authority, 'authority-' + step.authority);
      runtimeBadge(badges, step.resolution, 'resolution-' + step.resolution);
      heading.appendChild(badges);
      item.appendChild(heading);
      appendText(item, 'p', 'rm-path-step__fact', displaySurfaceFactLabel(fact) + ' · ' +
        humanSurfaceToken(fact.category));
      item.appendChild(sourceAction('Open exact step', step.location));
      if (step.targetRefs.length) {
        var targets = element('ul', 'rm-path-step__targets');
        step.targetRefs.forEach(function (ref) {
          var target = state.model.crossSurfacePaths.factsByRef[ref];
          var targetItem = element('li');
          if (target.location) {
            targetItem.appendChild(sourceAction(displaySurfaceFactLabel(target), target.location));
          }
          else appendText(targetItem, 'code', '', target.label);
          targets.appendChild(targetItem);
        });
        item.appendChild(targets);
      }
      timeline.appendChild(item);
    });
    host.appendChild(timeline);
  }

  function renderActivityRouteCard(route) {
    var card = element('article', 'rm-program-route-card rm-program-route-card--' + route.status);
    var heading = element('div', 'rm-program-route-card__heading');
    var copy = element('div');
    appendText(copy, 'p', 'rm-eyebrow', route.status + ' activity path');
    appendText(copy, 'h3', '', route.distance === 0 ? 'Direct caller identity' :
      String(route.distance) + (route.distance === 1 ? ' program step' : ' program steps'));
    heading.appendChild(copy);
    appendText(heading, 'span', 'rm-resolution rm-resolution--' +
      (route.status === 'possible' ? 'alternatives' : route.status), route.status);
    card.appendChild(heading);
    if (route.steps.length) {
      var steps = element('ol', 'rm-program-route-steps');
      route.steps.forEach(function (step) {
        var from = state.model.activityPaths.objectsByID[step.fromID];
        var to = state.model.activityPaths.objectsByID[step.toID];
        var item = element('li');
        appendText(item, 'span', 'rm-program-route-step__path',
          displayProgramObjectName(from) + ' → ' + displayProgramObjectName(to));
        appendText(item, 'small', '', humanRelation(step.kind) + ' · ' + step.authority);
        if (step.location) item.appendChild(sourceAction('Open exact step', step.location));
        steps.appendChild(item);
      });
      card.appendChild(steps);
    }
    if (route.frontier.length) {
      appendText(card, 'p', 'rm-path-frontier', 'Authority frontier: ' + route.frontier.join(', '));
    }
    if (route.outcomes.length) {
      var outcomes = element('div', 'rm-program-route-outcomes');
      route.outcomes.forEach(function (outcome) {
        var integration = state.model.integrationsByID[outcome.dependencyID];
        var link = element('a', '', integration.name + ' — ' + outcome.use.label + ' →');
        link.href = routeForIntegration(integration.id);
        outcomes.appendChild(link);
      });
      card.appendChild(outcomes);
    }
    return card;
  }

  function activityResponsibilityImpacts(activity, routes) {
    var impactsByID = Object.create(null);
    function impactFor(blockID) {
      var block = state.model.blocksByID[blockID];
      if (!block) return null;
      if (!impactsByID[blockID]) {
        impactsByID[blockID] = {
          block: block, startsHere: false, exactRelations: 0, possibleRelations: 0,
          exactPath: false, possiblePath: false
        };
      }
      return impactsByID[blockID];
    }

    (state.model.blocksBySymbol[activity.id] || []).forEach(function (blockID) {
      impactFor(blockID).startsHere = true;
    });
    state.model.relations.forEach(function (rawRelation) {
      var relation = object(rawRelation, 'program relation');
      if (relation.from_id !== activity.id || ['contains', 'imports', 'sources'].indexOf(relation.kind) >= 0 ||
          ['exact', 'alternatives'].indexOf(relation.resolution) < 0) return;
      var touched = Object.create(null);
      array(relation.to_ids, 'program relation.to_ids').forEach(function (targetID) {
        (state.model.blocksBySymbol[targetID] || []).forEach(function (blockID) {
          if (touched[blockID]) return;
          touched[blockID] = true;
          var impact = impactFor(blockID);
          if (relation.resolution === 'exact') impact.exactRelations++;
          else impact.possibleRelations++;
        });
      });
    });
    routes.forEach(function (route) {
      if (['exact', 'possible'].indexOf(route.status) < 0) return;
      var objectIDs = [route.activityID, route.callerID];
      route.steps.forEach(function (step) {
        objectIDs.push(step.fromID, step.toID);
      });
      objectIDs.forEach(function (objectID) {
        (state.model.blocksBySymbol[objectID] || []).forEach(function (blockID) {
          var impact = impactFor(blockID);
          if (route.status === 'exact') impact.exactPath = true;
          else impact.possiblePath = true;
        });
      });
    });
    return state.model.blocks.map(function (block) { return impactsByID[block.id]; }).filter(Boolean);
  }

  function renderActivityDetail(host, activity) {
    var routes = state.model.activityPaths ? state.model.activityPaths.routes.filter(function (route) {
      return route.activityID === activity.id;
    }) : [];
    var impacts = activityResponsibilityImpacts(activity, routes);
    var hero = element('section', 'rm-entrypoint-header');
    var back = element('a', 'rm-survey__overview-link', '← Back to program map');
    back.href = '#/program';
    hero.appendChild(back);
    var header = element('div', 'rm-entrypoint-header__body');
    var copy = element('div');
    appendText(copy, 'p', 'rm-eyebrow', 'Selected entrypoint');
    appendText(copy, 'h1', '', displayProgramObjectName(activity));
    appendText(copy, 'code', 'rm-entrypoint-header__signature', compactDisplayText(
      activity.signature || humanSurfaceToken(activity.kind), 240));
    appendText(copy, 'p', 'rm-entrypoint-header__facts', String(impacts.length) + ' core touchpoint' +
      (impacts.length === 1 ? '' : 's') + ' · ' + String(routes.length) + ' selected integration path' +
      (routes.length === 1 ? '' : 's'));
    header.appendChild(copy);
    var source = element('div', 'rm-entrypoint-header__source');
    source.appendChild(sourceAction('Open declaration', activity.location));
    header.appendChild(source);
    hero.appendChild(header);
    host.appendChild(hero);

    var detail = element('section', 'rm-entrypoint-detail');
    var responsibilities = element('section', 'rm-focus-section');
    appendText(responsibilities, 'h2', '', 'Core responsibility touchpoints');
    appendText(responsibilities, 'p', 'rm-focus-section__intro',
      'Where this entrypoint starts and which responsibilities its direct program relations or selected paths reach.');
    if (!impacts.length) {
      appendText(responsibilities, 'p', 'rm-empty',
        'No exact responsibility membership or selected program path is bound to this entrypoint.');
    } else {
      var links = element('div', 'rm-entrypoint-responsibilities');
      impacts.forEach(function (impact) {
        var link = element('a', 'rm-entrypoint-responsibility');
        link.href = routeForBlock(impact.block.id);
        link.addEventListener('click', function (event) {
          if (event && (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey)) return;
          if (event && event.preventDefault) event.preventDefault();
          navigateToBlock(impact.block, null, true);
        });
        var labels = [];
        if (impact.startsHere) labels.push('Starts here');
        if (impact.exactRelations) labels.push('→ ' + String(impact.exactRelations) + ' exact direct relation' +
          (impact.exactRelations === 1 ? '' : 's'));
        if (impact.possibleRelations) labels.push('⇢ ' + String(impact.possibleRelations) + ' possible direct relation' +
          (impact.possibleRelations === 1 ? '' : 's'));
        if (impact.exactPath) labels.push('Exact selected path');
        else if (impact.possiblePath) labels.push('Possible selected path');
        appendText(link, 'span', 'rm-entrypoint-responsibility__meta', labels.join(' · '));
        appendText(link, 'strong', '', impact.block.name);
        appendText(link, 'span', 'rm-entrypoint-responsibility__purpose', impact.block.purpose);
        appendText(link, 'span', 'rm-entrypoint-responsibility__action', 'Inspect responsibility ↓');
        links.appendChild(link);
      });
      responsibilities.appendChild(links);
    }
    detail.appendChild(responsibilities);

    var paths = element('section', 'rm-focus-section');
    appendText(paths, 'h2', '', 'Paths to selected integrations');
    if (!routes.length) {
      appendText(paths, 'p', 'rm-empty',
        'No selected external-package integration route starts at this entrypoint. Platform runtime operations are not integrations.');
    } else {
      var cards = element('div', 'rm-program-route-list');
      routes.forEach(function (route) { cards.appendChild(renderActivityRouteCard(route)); });
      paths.appendChild(cards);
    }
    detail.appendChild(paths);
    host.appendChild(detail);
  }

  function renderIntegrationDetail(host, integration) {
    var hero = element('section', 'rm-detail-hero');
    var back = element('a', 'rm-survey__overview-link', '← Back to program map');
    back.href = '#/program';
    hero.appendChild(back);
    appendText(hero, 'p', 'rm-eyebrow', 'Selected integration');
    appendText(hero, 'h1', '', integration.name);
    appendText(hero, 'p', 'rm-detail-hero__summary', integration.packagePath + ' · ' + integration.kind);
    host.appendChild(hero);

    var detail = element('section', 'rm-surface-detail');
    var uses = element('section', 'rm-focus-section');
    appendText(uses, 'h2', '', 'Selected operations');
    var list = element('div', 'rm-integration-use-list');
    integration.uses.forEach(function (use) {
      var card = element('article', 'rm-integration-use-card');
      var heading = element('div', 'rm-integration-use-card__heading');
      appendText(heading, 'h3', '', use.label);
      appendText(heading, 'span', 'rm-resolution rm-resolution--' +
        (use.authority === 'exact_external_symbol' ? 'exact' : 'unresolved'),
        humanIntegrationAuthority(use.authority));
      card.appendChild(heading);
      var caller = state.model.objectsByID[use.callerID];
      var callerLabel = caller ? displayProgramObjectName(caller) : use.callerName;
      appendText(card, 'p', '', callerLabel + ' → ' + use.callee);
      appendText(card, 'p', 'rm-integration-use-card__meta', 'Mechanism: ' + use.mechanism);
      card.appendChild(sourceAction('Open selected callsite', use.callsite));
      if (use.route) {
        var routeCopy = 'Activity path: ' + use.route.status;
        if (use.route.activityID) {
          var activity = state.model.activityByID[use.route.activityID];
          var routeLink = element('a', 'rm-integration-use-card__route', routeCopy + ' from ' + activity.name + ' →');
          routeLink.href = routeForActivity(activity.id);
          card.appendChild(routeLink);
        } else {
          appendText(card, 'p', 'rm-path-frontier', routeCopy +
            (use.route.frontier.length ? ' · ' + use.route.frontier.join(', ') : ''));
        }
      }
      list.appendChild(card);
    });
    uses.appendChild(list);
    detail.appendChild(uses);
    host.appendChild(detail);
  }

  function surveySummary(blocks, groups) {
    if (!blocks.length) return 'This target has exact structural evidence, but no semantic responsibility map.';
	var modelGroups = groups.filter(function (group) { return group.authority === 'model'; });
	var unassigned = groups.reduce(function (count, group) {
	  return count + (group.authority === 'local_unassigned' ? group.blockIDs.length : 0);
	}, 0);
    if (modelGroups.length) {
      var groupNames = modelGroups.slice(0, MAX_SURVEY_PREVIEW_ITEMS).map(function (group) { return group.name; });
      var finalGroup = groupNames.pop();
      var groupList = groupNames.length ? groupNames.join(', ') + ', and ' + finalGroup : finalGroup;
	  var summary = 'Explore ' + String(blocks.length) + ' responsibilities across ' + String(modelGroups.length) +
		' model-owned architecture areas: ' + groupList + '.';
	  if (unassigned) summary += ' ' + String(unassigned) + ' responsibilities were not placed by the grouping model.';
	  return summary;
    }
    var names = blocks.slice(0, MAX_SURVEY_PREVIEW_ITEMS).map(function (block) { return block.name; });
    var last = names.pop();
    var joined = names.length ? names.join(', ') + ', and ' + last : last;
    return 'Explore ' + String(blocks.length) + ' model-selected responsibilities, beginning with ' + joined + '.';
  }

  function targetOverviewCounts(model) {
    return [
      String(model.blocks.length) + (model.blocks.length === 1 ? ' responsibility' : ' responsibilities'),
      String(model.activities.length) + (model.activities.length === 1 ? ' entrypoint' : ' entrypoints'),
      String(model.integrations.length) + (model.integrations.length === 1 ? ' integration' : ' integrations')
    ].join(' · ');
  }

  function renderSurvey(host) {
    var survey = element('details', 'rm-target-overview');
    var summary = element('summary', 'rm-target-overview__summary');
    var identity = element('span', 'rm-target-overview__identity');
    appendText(identity, 'strong', '', state.model.target.name);
    appendText(identity, 'small', '', targetOverviewCounts(state.model));
    summary.appendChild(identity);
    appendText(summary, 'span', 'rm-target-overview__kind',
      humanRuntimeToken(state.model.target.language) + ' · ' + humanRuntimeToken(state.model.target.kind));
    survey.appendChild(summary);

    var body = element('div', 'rm-target-overview__body');
    var copy = element('div', 'rm-target-overview__copy');
    appendText(copy, 'p', '', surveySummary(state.model.blocks, state.model.groups));
    if (repositoryOverviewKind()) {
      var overview = element('a', 'rm-survey__overview-link', '← Repository overview');
      overview.href = '#/repository';
      copy.appendChild(overview);
    }
    body.appendChild(copy);

    var facts = element('dl', 'rm-target-overview__facts');
    [['Target', state.model.target.kind], ['Language', state.model.target.language],
      ['Selector', state.model.target.selector]].forEach(function (row) {
      var wrapper = element('div');
      appendText(wrapper, 'dt', '', row[0]);
      appendText(wrapper, 'dd', '', row[1]);
      facts.appendChild(wrapper);
    });
    body.appendChild(facts);
    survey.appendChild(body);
    host.appendChild(survey);
  }

  function renderStarts(parent, block) {
    var starts = block.symbols.map(function (symbol) { return state.model.activityByID[symbol.id]; }).filter(Boolean);
    if (!starts.length) return;
    var section = element('section', 'rm-focus-section');
    appendText(section, 'h3', '', state.model.target.kind === 'library' ? 'Ways into this area' : 'Execution starts here');
    appendText(section, 'p', 'rm-focus-section__intro',
      state.model.target.kind === 'library' ? 'Selected public or internal entrypoints that participate in this responsibility.' :
        'Selected activity entrypoints that participate in this responsibility.');
    var disclosure = element('details', 'rm-disclosure');
    disclosure.open = true;
    appendText(disclosure, 'summary', '', String(starts.length) + (starts.length === 1 ? ' selected entrypoint' : ' selected entrypoints'));
    var list = element('ul', 'rm-start-list');
    starts.forEach(function (start) {
      var item = element('li', 'rm-start');
      var route = element('a', 'rm-start__name', displayProgramObjectName(start) + ' →');
      route.href = routeForActivity(start.id);
      item.appendChild(route);
      appendText(item, 'code', 'rm-start__signature', compactDisplayText(start.signature || start.kind, 72));
      item.appendChild(sourceAction('Open declaration', start.location));
      list.appendChild(item);
    });
    disclosure.appendChild(list);
    section.appendChild(disclosure);
    parent.appendChild(section);
  }

  function connectionRecordCount(connections) {
    return connections.reduce(function (total, connection) {
      return total + connection.relationIDs.length;
    }, 0);
  }

  function connectionMemberName(connection, owner) {
    var source = connectionSourceObject(connection);
    var sourceName = displayProgramObjectName(source);
    if (source.id === owner.id) return sourceName;
    var ownerName = displayProgramObjectName(owner);
    var ownedPrefix = ownerName + '.';
    return sourceName.indexOf(ownedPrefix) === 0 ? sourceName.slice(ownedPrefix.length) : sourceName;
  }

  function groupOwnerConnectionsByMember(group) {
    var members = [];
    var membersByID = Object.create(null);
    ['local', 'platform', 'external', 'unresolved'].forEach(function (bucketName) {
      group[bucketName].forEach(function (connection) {
        var source = connectionSourceObject(connection);
        var member = membersByID[source.id];
        if (!member) {
          member = {
            id: source.id,
            source: source,
            local: [],
            platform: [],
            external: [],
            unresolved: []
          };
          membersByID[source.id] = member;
          members.push(member);
        }
        member[bucketName].push(connection);
      });
    });
    members.sort(function (left, right) {
      var leftConnections = left.local.concat(left.platform, left.external, left.unresolved);
      var rightConnections = right.local.concat(right.platform, right.external, right.unresolved);
      return compareConnections(leftConnections[0], rightConnections[0]) || left.id.localeCompare(right.id);
    });
    return members;
  }

  function connectionTargetNames(connection, owner) {
    if (!connection.targetIDs.length) return [connection.to];
    var ownerName = displayProgramObjectName(owner);
    var ownedPrefix = ownerName + '.';
    return connection.targetIDs.map(function (targetID) {
      var target = state.model.objectsByID[targetID];
      if (!target) throw new Error('A displayed connection target is absent from the ProgramView.');
      var name = displayProgramObjectName(target);
      return name.indexOf(ownedPrefix) === 0 ? name.slice(ownedPrefix.length) : name;
    });
  }

  function connectionTargetLabel(connection, owner) {
    var names = connectionTargetNames(connection, owner);
    if (connection.kind === 'calls' && connection.targetIDs.length) {
      return names.map(function (name) { return /\)$/.test(name) ? name : name + '()'; }).join(' / ');
    }
    if (connection.kind === 'invokes_external' && connection.targetIDs.length) {
      var targets = names.map(function (name) { return /\)$/.test(name) ? name : name + '()'; }).join(' / ');
      return connection.invocation === 'construct' ? 'new ' + targets : targets;
    }
    if (connection.kind === 'calls') return connection.to;
    return humanConnectionRelation(connection) + ' → ' + names.join(' / ');
  }

  function orderedConnectionLocations(connection) {
    return connection.locations.slice().sort(function (left, right) {
      return left.path.localeCompare(right.path) || left.line - right.line || left.column - right.column;
    });
  }

  function renderConnectionTarget(connection, owner) {
    var item = element('li', 'rm-connection rm-connection-target');
    var body = element('div');
    var label = connectionTargetLabel(connection, owner);
    var locations = orderedConnectionLocations(connection);
    var target;
    if (locations.length) {
      target = sourceAction(label, locations[0], { compact: true, locationLabel: '' });
      target.className += ' rm-connection__path rm-connection-target__link';
    } else {
      target = element('div', 'rm-connection-target__label', label);
    }
    body.appendChild(target);
    if (locations.length > 1) {
      var sites = element('details', 'rm-connection-sites');
      var siteLabel = connection.kind === 'calls' ? 'callsites' : 'source sites';
      appendText(sites, 'summary', 'rm-connection-sites__summary', String(locations.length) + ' ' + siteLabel);
      var siteList = element('div', 'rm-connection-sites__list');
      locations.forEach(function (location) {
        siteList.appendChild(sourceAction(formatLocation(location), location, {
          compact: true,
          locationLabel: ''
        }));
      });
      sites.appendChild(siteList);
      body.appendChild(sites);
    }
    item.appendChild(body);
    if (connection.resolution !== 'exact') {
      var resolutionLabel = connection.resolution === 'alternatives' ? 'possible' : connection.resolution;
      appendText(item, 'span', 'rm-resolution rm-resolution--' + connection.resolution, resolutionLabel);
    }
    return item;
  }

  function renderConnectionBucket(owner, bucket, label, modifier) {
    if (!bucket.length) return null;
    var details = element('details', 'rm-connection-runtime rm-connection-runtime--' + modifier);
    var records = connectionRecordCount(bucket);
    var summary = label + ' · ' + String(bucket.length) +
      (bucket.length === 1 ? ' relation group' : ' relation groups');
    if (records !== bucket.length) summary += ' · ' + String(records) + ' relation records';
    appendText(details, 'summary', 'rm-connection-runtime__summary', summary);
    var body = element('div', 'rm-connection-runtime__body');
    var list = element('ul', 'rm-connection-member__targets');
    bucket.forEach(function (connection) {
      list.appendChild(renderConnectionTarget(connection, owner));
    });
    body.appendChild(list);
    details.appendChild(body);
    return details;
  }

  function renderConnectionMemberGroup(member, owner) {
    var item = element('li', 'rm-connection-member-group');
    if (member.source.id !== owner.id) {
      var memberLabel = connectionMemberName(
        member.local.concat(member.platform, member.external, member.unresolved)[0], owner
      );
      var heading;
      if (member.source.location && state.model.openable[member.source.location.path]) {
        heading = sourceAction(memberLabel, member.source.location, { compact: true, locationLabel: '' });
        heading.className += ' rm-connection-member-group__heading';
      } else {
        heading = element('div', 'rm-connection-member-group__heading', memberLabel);
      }
      item.appendChild(heading);
    } else {
      item.className += ' rm-connection-member-group--owner';
    }
    if (member.local.length) {
      var local = element('ul', 'rm-connection-member__targets');
      member.local.forEach(function (connection) {
        local.appendChild(renderConnectionTarget(connection, owner));
      });
      item.appendChild(local);
    }
    [
      [member.platform, 'JavaScript platform APIs', 'platform'],
      [member.external, 'External APIs', 'external'],
      [member.unresolved, 'Unresolved runtime calls', 'unresolved']
    ].forEach(function (entry) {
      var bucket = renderConnectionBucket(owner, entry[0], entry[1], entry[2]);
      if (bucket) item.appendChild(bucket);
    });
    return item;
  }

  function renderConnectionOwner(group) {
    var item = element('li', 'rm-connection-owner');
    var heading = element('div', 'rm-connection-owner__heading');
    var ownerLabel = displayProgramObjectName(group.owner);
    var title;
    if (group.owner.location && state.model.openable[group.owner.location.path]) {
      title = sourceAction(ownerLabel, group.owner.location, { compact: true, locationLabel: '' });
      title.className += ' rm-connection-owner__title';
    } else {
      title = element('div', 'rm-connection-owner__title');
      appendText(title, 'strong', '', ownerLabel);
    }
    heading.appendChild(title);
    var all = group.local.concat(group.platform, group.external, group.unresolved);
    var records = connectionRecordCount(all);
    appendText(heading, 'span', 'rm-connection-owner__count', String(all.length) +
      (all.length === 1 ? ' group' : ' groups') +
      (records === all.length ? '' : ' · ' + String(records) + ' records'));
    item.appendChild(heading);

    var members = element('ul', 'rm-connection-owner__members');
    groupOwnerConnectionsByMember(group).forEach(function (member) {
      members.appendChild(renderConnectionMemberGroup(member, group.owner));
    });
    item.appendChild(members);
    return item;
  }

  function renderConnections(parent, block, connections) {
    var section = element('section', 'rm-focus-section');
    appendText(section, 'h3', '', 'How the code connects');
    appendText(section, 'p', 'rm-focus-section__intro',
      'Exact local relations come first. Relations with the same resolved targets and authority are condensed; unresolved relation records stay separate.');
    if (!connections.length) {
      appendText(section, 'p', 'rm-empty', 'No focused call or handoff relation is available for the representative symbols.');
      parent.appendChild(section);
      return;
    }
    var disclosure = element('details', 'rm-disclosure');
    disclosure.open = true;
    var relationCount = connectionRecordCount(connections);
    var summary = String(connections.length) + (connections.length === 1 ? ' relation group' : ' relation groups');
    if (relationCount !== connections.length) summary += ' · ' + String(relationCount) + ' relation records';
    appendText(disclosure, 'summary', '', summary);
    var list = element('ul', 'rm-connection-owner-list');
    groupConnectionsByOwner(connections).forEach(function (group) {
      list.appendChild(renderConnectionOwner(group));
    });
    var omittedWitnessDetails = connections.reduce(function (total, connection) {
      return total + connection.witnessesOmitted + connection.witnessesProjectionOmitted;
    }, 0);
    disclosure.appendChild(list);
    if (omittedWitnessDetails > 0) {
      appendText(disclosure, 'p', 'rm-connection-warning', 'Evidence detail limit · ' +
        String(omittedWitnessDetails) + ' relation witness detail' +
        (omittedWitnessDetails === 1 ? ' is' : 's are') +
        ' not shown; displayed endpoints and source locations remain exact.');
    }
    section.appendChild(disclosure);
    parent.appendChild(section);
  }

  function renderFocus(block, index, connections, related) {
    var article = element('article', 'rm-focus');
    article.tabIndex = -1;
    article.setAttribute('data-responsibility-detail', block.id);
    appendText(article, 'p', 'rm-focus__index', 'Responsibility ' + String(index + 1).padStart(2, '0'));
    appendText(article, 'h2', '', block.name);
    appendText(article, 'p', 'rm-focus__purpose', block.purpose);
	var memberships = state.model.groupsByBlock[block.id] || [];
	var modelMemberships = memberships.filter(function (group) { return group.authority === 'model'; });
    if (modelMemberships.length) {
      appendText(article, 'p', 'rm-focus__membership', 'Architecture areas: ' + modelMemberships.map(function (group) {
        return group.name;
      }).join(', '));
	} else if (memberships.some(function (group) { return group.authority === 'local_unassigned'; })) {
	  appendText(article, 'p', 'rm-focus__membership', 'Grouping status: not placed by grouping model.');
    }
    renderStarts(article, block);
    renderConnections(article, block, connections);

    if (related.length) {
      var relatedHost = element('details', 'rm-related rm-disclosure');
      appendText(relatedHost, 'summary', '', 'Related responsibilities · ' + String(related.length));
      var relatedList = element('div', 'rm-related__list');
      related.forEach(function (candidate) {
        var button = element('button', '', candidate.name);
        button.type = 'button';
        button.addEventListener('click', function () { navigateToBlock(candidate); });
        relatedList.appendChild(button);
      });
      relatedHost.appendChild(relatedList);
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

  function evidenceFileGroups(block) {
    var groups = [];
    var byPath = Object.create(null);
    function groupFor(path) {
      if (byPath[path]) return byPath[path];
      var group = { path: path, symbols: [] };
      byPath[path] = group;
      groups.push(group);
      return group;
    }
    block.files.forEach(function (file) { groupFor(file.path); });
    block.symbols.forEach(function (symbol) { groupFor(symbol.location.path).symbols.push(symbol); });
    return groups;
  }

  function renderEvidence(block) {
    var aside = element('aside', 'rm-evidence');
    aside.setAttribute('aria-label', 'Exact source evidence');
    var sticky = element('div', 'rm-evidence__sticky');
    appendText(sticky, 'h2', '', 'Verify in code');
    var groups = evidenceFileGroups(block);
    var disclosure = element('details', 'rm-evidence-disclosure');
    disclosure.open = true;
    appendText(disclosure, 'summary', '', 'Code · ' + String(groups.length) +
      (groups.length === 1 ? ' file' : ' files') + ' · ' + String(block.symbols.length) +
      (block.symbols.length === 1 ? ' declaration' : ' declarations'));
    var files = element('ul', 'rm-evidence-files');
    groups.forEach(function (group) {
      var file = element('li', 'rm-evidence-file');
      var fileAction = sourceAction(group.path, { path: group.path, line: 0, column: 0 }, {
        compact: true,
        locationLabel: ''
      });
      fileAction.className += ' rm-evidence-file__path';
      file.appendChild(fileAction);
      if (group.symbols.length) {
        var symbols = element('ul', 'rm-evidence-symbols');
        group.symbols.forEach(function (symbol) {
          var item = element('li');
          item.appendChild(sourceAction(displayProgramObjectName(symbol), symbol.location, {
            locationLabel: symbol.location.line > 0 ? 'L' + String(symbol.location.line) : ''
          }));
          symbols.appendChild(item);
        });
        file.appendChild(symbols);
      }
      files.appendChild(file);
    });
    disclosure.appendChild(files);
    sticky.appendChild(disclosure);

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
    var projection = state.model.programProjection;
    ['seeds', 'objects', 'relations'].forEach(function (kind) {
      var coverage = projection[kind];
      if (coverage.omitted > 0) {
        notes.push(String(coverage.omitted) + ' of ' + String(coverage.eligible) +
          ' eligible ProgramIndex ' + kind + ' are outside this compact browser projection.');
      }
    });
    if (projection.witnessesOmitted > 0) {
      notes.push(String(projection.witnessesOmitted) +
        ' exact relation witnesses are outside this compact browser projection.');
    }
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
    unmountSystemCanvas();
    var host = document.getElementById('rm-app');
    var orientation = element('div', 'rm-orientation');
    var route = selectedReportRoute();
    updateHeaderContext(route);
    if (route.kind === 'repository') {
      var overviewKind = repositoryOverviewKind();
      if (overviewKind === 'runtime') renderRuntimePortfolio(orientation);
      else renderRepositoryFallback(orientation);
      renderDiagnostics(orientation);
      host.replaceChildren(orientation);
      document.title = state.model.repoName + ' — repository overview — repomap';
      return;
    }
    if (route.kind === 'surface') {
      renderSurfaceDetail(orientation, route.surface);
      renderDiagnostics(orientation);
      host.replaceChildren(orientation);
      document.title = state.model.repoName + ' — ' + route.surface.name + ' — repomap';
      return;
    }
    if (route.kind === 'path') {
      renderCrossSurfacePathDetail(orientation, route.path);
      renderDiagnostics(orientation);
      host.replaceChildren(orientation);
      document.title = state.model.repoName + ' — ' + route.path.name + ' — repomap';
      return;
    }
    if (route.kind === 'entrypoint') {
      renderActivityDetail(orientation, route.activity);
      renderDiagnostics(orientation);
      host.replaceChildren(orientation);
      document.title = state.model.repoName + ' — ' + route.activity.name + ' — repomap';
      return;
    }
    if (route.kind === 'integration') {
      renderIntegrationDetail(orientation, route.integration);
      renderDiagnostics(orientation);
      host.replaceChildren(orientation);
      document.title = state.model.repoName + ' — ' + route.integration.name + ' — repomap';
      return;
    }
    orientation.className += ' rm-orientation--program';
    renderSurvey(orientation);
    if (!state.model.blocks.length) {
      renderStructuralOnly(orientation);
      renderTargetSurfaceInventory(orientation);
      renderDiagnostics(orientation);
      host.replaceChildren(orientation);
      return;
    }
    var selected = route.kind === 'responsibility' ? route.block : state.model.blocks[0];
    var index = state.model.blocks.findIndex(function (block) { return block.id === selected.id; });
    var connections = connectionsFor(selected);
    var related = relatedBlocksFor(selected, connections);
    var canvasView = renderFlowCanvas(selected);
    orientation.appendChild(canvasView.element);
    var workspace = element('section', 'rm-workspace');
    workspace.appendChild(renderFocus(selected, index, connections, related));
    workspace.appendChild(renderEvidence(selected));
    orientation.appendChild(workspace);
    renderTargetSurfaceInventory(orientation);
    renderDiagnostics(orientation);
    host.replaceChildren(orientation);
    state.canvasMount = canvasView.mount();
    if (state.pendingResponsibilityID === selected.id) {
      state.pendingResponsibilityID = '';
      scheduleResponsibilityScroll(selected.id);
    }
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
      state = {
        model: model, source: buildSourceAuthority(raw, model), canvasMount: null,
        completeCanvas: false, focusGroupID: null, pendingResponsibilityID: '', pendingAreaGroupFocusID: ''
      };
      renderHeader();
      renderRoute();
      window.addEventListener('hashchange', function () {
        try { renderRoute(); } catch (error) { renderFatal(error); }
      });
    } catch (error) {
      renderFatal(error);
    }
  }

  boot();
})();
