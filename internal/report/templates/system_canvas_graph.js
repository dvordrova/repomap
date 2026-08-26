(function (root) {
  'use strict';

  var AUTHORITIES = Object.freeze({ exact: true, possible: true, runtime: true });
  var LANE_ORDER = Object.freeze(['entry', 'core', 'integration']);

  function fail(message) {
    throw new Error('System canvas graph: ' + message);
  }

  function record(value, label) {
    if (!value || typeof value !== 'object' || Array.isArray(value)) fail(label + ' must be an object.');
    return value;
  }

  function list(value, label) {
    if (!Array.isArray(value)) fail(label + ' must be an array.');
    return value;
  }

  function string(value, label) {
    if (typeof value !== 'string' || !value) fail(label + ' must be a non-empty string.');
    return value;
  }

  function optionalString(value, label) {
    if (value == null || value === '') return null;
    return string(value, label);
  }

  function integer(value, label) {
    if (!Number.isInteger(value) || value < 0) fail(label + ' must be a non-negative integer.');
    return value;
  }

  function freezeLocation(value, label) {
    if (value == null) return null;
    value = record(value, label);
    return Object.freeze({
      path: string(value.path, label + '.path'),
      line: integer(value.line, label + '.line'),
      column: integer(value.column, label + '.column')
    });
  }

  function stableSegment(value) {
    return encodeURIComponent(value).replace(/[!'()*]/g, function (character) {
      return '%' + character.charCodeAt(0).toString(16).toUpperCase();
    });
  }

  function stableEdgeID(sourceID, targetID, authority) {
    var source = stableSegment(sourceID);
    var target = stableSegment(targetID);
    return 'edge:' + String(source.length) + ':' + source + ':' +
      String(target.length) + ':' + target + ':' + authority;
  }

  function displayObjectName(value) {
    var name = string(value.name, 'activity.name');
    var separator = name.lastIndexOf('#');
    if (separator >= 0 && separator + 1 < name.length) return name.slice(separator + 1);
    return name;
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

  function humanIntegrationAuthority(value) {
    if (value === 'exact_external_symbol') return 'exact external symbol';
    if (value === 'syntactic_unresolved') return 'selected callsite; runtime unresolved';
    return value.replace(/_/g, ' ');
  }

  function uniqueStrings(values, label) {
    var seen = Object.create(null);
    var result = [];
    list(values, label).forEach(function (value, index) {
      value = string(value, label + '[' + String(index) + ']');
      if (seen[value]) return;
      seen[value] = true;
      result.push(value);
    });
    return result;
  }

  function mapList(value, key, label) {
    if (value == null || value[key] == null) return [];
    return list(value[key], label + '[' + key + ']');
  }

  function frozenIndex(keys, factory) {
    var result = Object.create(null);
    keys.forEach(function (key) { result[key] = Object.freeze(factory(key)); });
    return Object.freeze(result);
  }

  function buildCanvasGraph(presentationModel, rawOptions) {
    var model = record(presentationModel, 'presentationModel');
    var options = rawOptions == null ? {} : record(rawOptions, 'options');
    var activities = list(model.activities, 'presentationModel.activities');
    var blocks = list(model.blocks, 'presentationModel.blocks');
    var integrations = list(model.integrations, 'presentationModel.integrations');
    var relations = list(model.relations, 'presentationModel.relations');
    var blocksBySymbol = record(model.blocksBySymbol, 'presentationModel.blocksBySymbol');
    var groupsByBlock = model.groupsByBlock == null ? Object.create(null) :
      record(model.groupsByBlock, 'presentationModel.groupsByBlock');
    if (options.complete != null && typeof options.complete !== 'boolean') {
      fail('options.complete must be a boolean.');
    }
    var complete = options.complete === true;

    var allNodes = [];
    var allNodesByID = Object.create(null);
    var blocksByID = Object.create(null);
    var integrationsByID = Object.create(null);
    var activitiesByID = Object.create(null);

    function addNode(node) {
      if (allNodesByID[node.id]) fail('node IDs must be unique: ' + node.id + '.');
      node = Object.freeze(node);
      allNodesByID[node.id] = node;
      allNodes.push(node);
    }

    activities.forEach(function (rawActivity, index) {
      var activity = record(rawActivity, 'presentationModel.activities[' + String(index) + ']');
      var entityID = string(activity.id, 'activity.id');
      if (activitiesByID[entityID]) fail('activity IDs must be unique: ' + entityID + '.');
      activitiesByID[entityID] = activity;
      var activityLocation = freezeLocation(activity.location, 'activity.location');
      var activityName = displayObjectName(activity);
      addNode({
        id: 'entry:' + entityID,
        entityID: entityID,
        kind: 'entrypoint',
        lane: 'entry',
        name: activityName,
        signature: optionalString(activity.signature, 'activity.signature'),
        objectKind: string(activity.kind, 'activity.kind'),
        location: activityLocation,
        data: Object.freeze({
          name: activityName,
          signature: optionalString(activity.signature, 'activity.signature'),
          objectKind: string(activity.kind, 'activity.kind'),
          location: activityLocation
        })
      });
    });

    blocks.forEach(function (rawBlock, index) {
      var block = record(rawBlock, 'presentationModel.blocks[' + String(index) + ']');
      var entityID = string(block.id, 'block.id');
      if (blocksByID[entityID]) fail('block IDs must be unique: ' + entityID + '.');
      blocksByID[entityID] = block;
      var memberships = groupsByBlock[entityID] == null ? [] :
        list(groupsByBlock[entityID], 'presentationModel.groupsByBlock[' + entityID + ']');
      var blockName = string(block.name, 'block.name');
      var blockDescription = string(block.purpose, 'block.purpose');
      var declarationCount = list(block.symbols, 'block.symbols').length;
      var areaCount = memberships.length;
      addNode({
        id: 'core:' + entityID,
        entityID: entityID,
        kind: 'core',
        lane: 'core',
        name: blockName,
        description: blockDescription,
        declarationCount: declarationCount,
        areaCount: areaCount,
        data: Object.freeze({
          name: blockName,
          description: blockDescription,
          declarationCount: declarationCount,
          areaCount: areaCount
        })
      });
    });

    integrations.forEach(function (rawIntegration, index) {
      var integration = record(rawIntegration, 'presentationModel.integrations[' + String(index) + ']');
      var entityID = string(integration.id, 'integration.id');
      if (integrationsByID[entityID]) fail('integration IDs must be unique: ' + entityID + '.');
      integrationsByID[entityID] = integration;
      var integrationName = string(integration.name, 'integration.name');
      var integrationKind = string(integration.kind, 'integration.kind');
      var packagePath = string(integration.packagePath, 'integration.packagePath');
      var modulePath = optionalString(integration.modulePath, 'integration.modulePath');
      var useCount = list(integration.uses, 'integration.uses').length;
      addNode({
        id: 'integration:' + entityID,
        entityID: entityID,
        kind: 'integration',
        lane: 'integration',
        name: integrationName,
        integrationKind: integrationKind,
        packagePath: packagePath,
        modulePath: modulePath,
        useCount: useCount,
        data: Object.freeze({
          name: integrationName,
          integrationKind: integrationKind,
          packagePath: packagePath,
          modulePath: modulePath,
          useCount: useCount
        })
      });
    });

    var requestedActiveBlockIDs = options.activeBlockIDs == null ?
      blocks.map(function (block) { return string(block.id, 'block.id'); }) :
      uniqueStrings(options.activeBlockIDs, 'options.activeBlockIDs');
    requestedActiveBlockIDs.forEach(function (blockID) {
      if (!blocksByID[blockID]) fail('activeBlockIDs cites an unknown block: ' + blockID + '.');
    });
    var activeBlockIDs = Object.freeze(requestedActiveBlockIDs.slice());
    var activeBlocks = Object.create(null);
    activeBlockIDs.forEach(function (blockID) { activeBlocks[blockID] = true; });

    var edgeBuilders = [];
    var edgeByKey = Object.create(null);
    var connectedStarts = Object.create(null);
    var connectedIntegrations = Object.create(null);

    function addEdge(sourceID, targetID, rawBlockIDs, authority, description) {
      string(sourceID, 'edge.sourceID');
      string(targetID, 'edge.targetID');
      if (!allNodesByID[sourceID] || !allNodesByID[targetID]) {
        fail('an edge endpoint is not a known node: ' + sourceID + ' -> ' + targetID + '.');
      }
      if (!AUTHORITIES[authority]) fail('edge authority is not exact, possible, or runtime.');
      description = string(description, 'edge.description');
      var blockIDs = uniqueStrings(rawBlockIDs, 'edge.blockIDs');
      blockIDs.forEach(function (blockID) {
        if (!blocksByID[blockID]) fail('an edge cites an unknown block: ' + blockID + '.');
      });
      var id = stableEdgeID(sourceID, targetID, authority);
      var existing = edgeByKey[id];
      if (existing) {
        blockIDs.forEach(function (blockID) {
          if (existing.blockIDs.indexOf(blockID) < 0) existing.blockIDs.push(blockID);
        });
        return;
      }
      var edge = {
        id: id,
        sourceID: sourceID,
        targetID: targetID,
        authority: authority,
        description: description,
        blockIDs: blockIDs
      };
      edgeByKey[id] = edge;
      edgeBuilders.push(edge);
      if (sourceID.indexOf('entry:') === 0) connectedStarts[sourceID.slice(6)] = true;
      if (targetID.indexOf('integration:') === 0) connectedIntegrations[targetID.slice(12)] = true;
    }

    activities.forEach(function (start) {
      mapList(blocksBySymbol, start.id, 'presentationModel.blocksBySymbol').forEach(function (blockID) {
        blockID = string(blockID, 'entrypoint block ID');
        var block = blocksByID[blockID];
        if (!block) fail('an entrypoint cites an unknown block: ' + blockID + '.');
        addEdge(
          'entry:' + start.id,
          'core:' + blockID,
          [blockID],
          'exact',
          string(start.name, 'activity.name') + ' participates in ' + string(block.name, 'block.name')
        );
      });
    });

    relations.forEach(function (rawRelation, index) {
      var relation = record(rawRelation, 'presentationModel.relations[' + String(index) + ']');
      var kind = string(relation.kind, 'relation.kind');
      var resolution = string(relation.resolution, 'relation.resolution');
      if (['contains', 'imports', 'sources'].indexOf(kind) >= 0 ||
          ['exact', 'alternatives'].indexOf(resolution) < 0) return;
      var fromBlocks = mapList(blocksBySymbol, string(relation.from_id, 'relation.from_id'),
        'presentationModel.blocksBySymbol');
      var toBlocks = [];
      list(relation.to_ids, 'relation.to_ids').forEach(function (targetID) {
        mapList(blocksBySymbol, string(targetID, 'relation target ID'),
          'presentationModel.blocksBySymbol').forEach(function (blockID) {
          if (toBlocks.indexOf(blockID) < 0) toBlocks.push(blockID);
        });
      });
      fromBlocks.forEach(function (fromBlockID) {
        fromBlockID = string(fromBlockID, 'relation source block ID');
        var fromBlock = blocksByID[fromBlockID];
        if (!fromBlock) fail('a relation cites an unknown source block: ' + fromBlockID + '.');
        toBlocks.forEach(function (toBlockID) {
          toBlockID = string(toBlockID, 'relation target block ID');
          var toBlock = blocksByID[toBlockID];
          if (!toBlock) fail('a relation cites an unknown target block: ' + toBlockID + '.');
          if (fromBlockID === toBlockID) return;
          var authority = resolution === 'exact' ? 'exact' : 'possible';
          addEdge(
            'core:' + fromBlockID,
            'core:' + toBlockID,
            [fromBlockID, toBlockID],
            authority,
            fromBlock.name + ' ' + humanRelation(kind) + ' ' + toBlock.name + '; ' + resolution
          );
        });
      });
    });

    integrations.forEach(function (integration) {
      list(integration.uses, 'integration.uses').forEach(function (rawUse) {
        var use = record(rawUse, 'integration use');
        mapList(blocksBySymbol, string(use.callerID, 'integration use.callerID'),
          'presentationModel.blocksBySymbol').forEach(function (blockID) {
          blockID = string(blockID, 'integration use block ID');
          var block = blocksByID[blockID];
          if (!block) fail('an integration use cites an unknown block: ' + blockID + '.');
          var useAuthority = string(use.authority, 'integration use.authority');
          var authority = useAuthority === 'exact_external_symbol' ? 'exact' : 'runtime';
          addEdge(
            'core:' + blockID,
            'integration:' + integration.id,
            [blockID],
            authority,
            block.name + ' has a selected callsite to ' + integration.name + '; ' +
              humanIntegrationAuthority(useAuthority)
          );
        });
      });
    });

    if (model.activityPaths != null) {
      var activityPaths = record(model.activityPaths, 'presentationModel.activityPaths');
      list(activityPaths.routes, 'presentationModel.activityPaths.routes').forEach(function (rawRoute) {
        var route = record(rawRoute, 'activity path route');
        if (!route.activityID || ['exact', 'possible'].indexOf(route.status) < 0) return;
        var activity = activitiesByID[route.activityID];
        if (!activity) fail('an activity path cites an unknown activity: ' + route.activityID + '.');
        var blockSequence = [];
        var objectIDs = [route.activityID];
        list(route.steps, 'activity path route.steps').forEach(function (step) {
          step = record(step, 'activity path step');
          objectIDs.push(string(step.toID, 'activity path step.toID'));
        });
        var callerID = string(route.callerID, 'activity path route.callerID');
        if (objectIDs[objectIDs.length - 1] !== callerID) objectIDs.push(callerID);
        objectIDs.forEach(function (objectID) {
          mapList(blocksBySymbol, objectID, 'presentationModel.blocksBySymbol').forEach(function (blockID) {
            blockID = string(blockID, 'activity path block ID');
            if (!blocksByID[blockID]) fail('an activity path cites an unknown block: ' + blockID + '.');
            if (blockSequence[blockSequence.length - 1] !== blockID) blockSequence.push(blockID);
          });
        });
        var routeAuthority = route.status === 'exact' ? 'exact' : 'possible';
        if (blockSequence.length) {
          addEdge(
            'entry:' + route.activityID,
            'core:' + blockSequence[0],
            blockSequence,
            routeAuthority,
            activity.name + ' reaches ' + blocksByID[blockSequence[0]].name +
              ' through a ' + route.status + ' activity path'
          );
          for (var position = 1; position < blockSequence.length; position++) {
            addEdge(
              'core:' + blockSequence[position - 1],
              'core:' + blockSequence[position],
              blockSequence,
              routeAuthority,
              'Activity path crosses from ' + blocksByID[blockSequence[position - 1]].name +
                ' to ' + blocksByID[blockSequence[position]].name + '; ' + route.status
            );
          }
        }
        list(route.outcomes, 'activity path route.outcomes').forEach(function (rawOutcome) {
          var outcome = record(rawOutcome, 'activity path outcome');
          var dependencyID = string(outcome.dependencyID, 'activity path outcome.dependencyID');
          var integration = integrationsByID[dependencyID];
          if (!integration) fail('an activity path cites an unknown integration: ' + dependencyID + '.');
          var use = record(outcome.use, 'activity path outcome.use');
          var useAuthority = string(use.authority, 'activity path outcome.use.authority');
          var authority = useAuthority === 'exact_external_symbol' ? routeAuthority : 'runtime';
          if (blockSequence.length) {
            var lastBlockID = blockSequence[blockSequence.length - 1];
            addEdge(
              'core:' + lastBlockID,
              'integration:' + dependencyID,
              blockSequence,
              authority,
              blocksByID[lastBlockID].name + ' reaches ' + integration.name +
                ' through a ' + route.status + ' activity path'
            );
          } else {
            addEdge(
              'entry:' + route.activityID,
              'integration:' + dependencyID,
              [],
              authority,
              activity.name + ' reaches ' + integration.name + ' directly; ' + route.status
            );
          }
        });
      });
    }

    var allEdges = edgeBuilders.map(function (edge) {
      edge.blockIDs = Object.freeze(edge.blockIDs.slice());
      return Object.freeze(edge);
    });

    function directFrontier(edge) {
      return edge.blockIDs.length === 0 && edge.sourceID.indexOf('entry:') === 0 &&
        edge.targetID.indexOf('integration:') === 0;
    }

    function endpointVisible(nodeID) {
      return nodeID.indexOf('core:') !== 0 || !!activeBlocks[nodeID.slice(5)];
    }

    var visibleEdges = complete ? allEdges.slice() : allEdges.filter(function (edge) {
      return directFrontier(edge) || (endpointVisible(edge.sourceID) && endpointVisible(edge.targetID));
    });
    var visibleEntries = Object.create(null);
    var visibleIntegrations = Object.create(null);
    visibleEdges.forEach(function (edge) {
      if (edge.sourceID.indexOf('entry:') === 0) visibleEntries[edge.sourceID] = true;
      if (edge.targetID.indexOf('integration:') === 0) visibleIntegrations[edge.targetID] = true;
    });

    var coreNodeIDs = complete ? blocks.map(function (block) { return 'core:' + block.id; }) :
      activeBlockIDs.map(function (blockID) { return 'core:' + blockID; });
    var nodesByLane = {
      entry: activities.map(function (activity) { return allNodesByID['entry:' + activity.id]; })
        .filter(function (node) { return complete || visibleEntries[node.id]; }),
      core: coreNodeIDs.map(function (nodeID) { return allNodesByID[nodeID]; }),
      integration: integrations.map(function (integration) { return allNodesByID['integration:' + integration.id]; })
        .filter(function (node) { return complete || visibleIntegrations[node.id]; })
    };
    LANE_ORDER.forEach(function (lane) { nodesByLane[lane] = Object.freeze(nodesByLane[lane]); });
    nodesByLane = Object.freeze(nodesByLane);
    var nodes = Object.freeze([].concat(nodesByLane.entry, nodesByLane.core, nodesByLane.integration));
    visibleEdges = Object.freeze(visibleEdges);
    var visibleNodeIDSet = Object.create(null);
    nodes.forEach(function (node) { visibleNodeIDSet[node.id] = true; });
    visibleEdges.forEach(function (edge) {
      if (!visibleNodeIDSet[edge.sourceID] || !visibleNodeIDSet[edge.targetID]) {
        fail('focused visibility left an edge without both endpoint nodes: ' + edge.id + '.');
      }
    });

    var nodeIDs = nodes.map(function (node) { return node.id; });
    var edgeIDs = visibleEdges.map(function (edge) { return edge.id; });
    var nodesByID = Object.create(null);
    nodes.forEach(function (node) { nodesByID[node.id] = node; });
    var edgesByID = Object.create(null);
    visibleEdges.forEach(function (edge) { edgesByID[edge.id] = edge; });
    nodesByID = Object.freeze(nodesByID);
    edgesByID = Object.freeze(edgesByID);

    var incomingBuilders = Object.create(null);
    var outgoingBuilders = Object.create(null);
    var incidentBuilders = Object.create(null);
    nodeIDs.forEach(function (nodeID) {
      incomingBuilders[nodeID] = [];
      outgoingBuilders[nodeID] = [];
      incidentBuilders[nodeID] = [];
    });
    visibleEdges.forEach(function (edge) {
      outgoingBuilders[edge.sourceID].push(edge);
      incomingBuilders[edge.targetID].push(edge);
      incidentBuilders[edge.sourceID].push(edge);
      if (edge.targetID !== edge.sourceID) incidentBuilders[edge.targetID].push(edge);
    });
    var incomingByNodeID = frozenIndex(nodeIDs, function (nodeID) { return incomingBuilders[nodeID]; });
    var outgoingByNodeID = frozenIndex(nodeIDs, function (nodeID) { return outgoingBuilders[nodeID]; });
    var incidentEdgesByNodeID = frozenIndex(nodeIDs, function (nodeID) { return incidentBuilders[nodeID]; });

    var shownConnectedStarts = nodesByLane.entry.filter(function (node) {
      return !!connectedStarts[node.entityID];
    }).length;
    var shownConnectedIntegrations = nodesByLane.integration.filter(function (node) {
      return !!connectedIntegrations[node.entityID];
    }).length;
    var connectedStartCount = Object.keys(connectedStarts).length;
    var connectedIntegrationCount = Object.keys(connectedIntegrations).length;
    var visibleNodeIDs = Object.freeze(nodeIDs.slice());
    var visibleEdgeIDs = Object.freeze(edgeIDs.slice());
    var hiddenNodeIDs = Object.freeze(allNodes.filter(function (node) {
      return !visibleNodeIDSet[node.id];
    }).map(function (node) { return node.id; }));
    var visibleEdgeIDSet = Object.create(null);
    edgeIDs.forEach(function (edgeID) { visibleEdgeIDSet[edgeID] = true; });
    var hiddenEdgeIDs = Object.freeze(allEdges.filter(function (edge) {
      return !visibleEdgeIDSet[edge.id];
    }).map(function (edge) { return edge.id; }));

    var visibility = Object.freeze({
      complete: complete,
      activeBlockIDs: activeBlockIDs,
      visibleNodeIDs: visibleNodeIDs,
      visibleEdgeIDs: visibleEdgeIDs,
      hiddenNodeIDs: hiddenNodeIDs,
      hiddenEdgeIDs: hiddenEdgeIDs
    });
    var accounting = Object.freeze({
      connectedStartCount: connectedStartCount,
      connectedIntegrationCount: connectedIntegrationCount,
      hiddenPlacedStarts: Math.max(0, connectedStartCount - shownConnectedStarts),
      hiddenConnectedIntegrations: Math.max(0, connectedIntegrationCount - shownConnectedIntegrations),
      directFrontierEdges: allEdges.filter(directFrontier).length,
      totalEdges: allEdges.length,
      visibleEdges: visibleEdges.length,
      totalNodes: allNodes.length,
      visibleNodes: nodes.length,
      unplacedStarts: activities.filter(function (activity) { return !connectedStarts[activity.id]; }).length,
      unplacedIntegrations: integrations.filter(function (integration) {
        return !connectedIntegrations[integration.id];
      }).length
    });

    return Object.freeze({
      nodes: nodes,
      edges: visibleEdges,
      nodesByID: nodesByID,
      edgesByID: edgesByID,
      incomingByNodeID: incomingByNodeID,
      outgoingByNodeID: outgoingByNodeID,
      incidentEdgesByNodeID: incidentEdgesByNodeID,
      nodesByLane: nodesByLane,
      visibility: visibility,
      accounting: accounting
    });
  }

  var api = Object.freeze({ buildCanvasGraph: buildCanvasGraph });
  if (root.RepomapSystemCanvasGraph) fail('the asset namespace is already installed.');
  Object.defineProperty(root, 'RepomapSystemCanvasGraph', {
    value: api,
    enumerable: false,
    configurable: false,
    writable: false
  });
})(globalThis);
