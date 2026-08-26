(function (root) {
  'use strict';

  var ENDPOINT_PREFIX = 'canvas-endpoint:';

  function exactText(value, label) {
    if (typeof value !== 'string' || value.trim() !== value || !value) {
      throw new Error(label + ' must be exact non-empty text');
    }
    return value;
  }

  function optionalID(value, label) {
    if (value == null || value === '') return '';
    return exactText(value, label);
  }

  function graphObject(graph) {
    if (!graph || typeof graph !== 'object' || Array.isArray(graph)) {
      throw new Error('canvas graph must be an object');
    }
    if (!Array.isArray(graph.nodes) || !Array.isArray(graph.edges) ||
        !graph.nodesByID || !graph.edgesByID || !graph.incidentEdgesByNodeID) {
      throw new Error('canvas graph must contain nodes, edges, and adjacency indexes');
    }
    return graph;
  }

  function endpointID(edgeID, nodeID, role) {
    edgeID = exactText(edgeID, 'canvas edge id');
    nodeID = exactText(nodeID, 'canvas endpoint node id');
    role = exactText(role, 'canvas endpoint role');
    if (role !== 'source' && role !== 'target') {
      throw new Error('canvas endpoint role is not supported');
    }
    return ENDPOINT_PREFIX + String(edgeID.length) + ':' + edgeID +
      String(nodeID.length) + ':' + nodeID + String(role.length) + ':' + role;
  }

  function parseLengthSegment(value, offset, label) {
    var separator = value.indexOf(':', offset);
    if (separator < 0) throw new Error(label + ' is malformed');
    var rawLength = value.slice(offset, separator);
    if (!/^(0|[1-9][0-9]*)$/.test(rawLength)) throw new Error(label + ' is malformed');
    var length = Number(rawLength);
    if (!Number.isSafeInteger(length) || length <= 0) throw new Error(label + ' is malformed');
    var start = separator + 1;
    var end = start + length;
    if (end > value.length) throw new Error(label + ' is malformed');
    return { value: value.slice(start, end), offset: end };
  }

  function parseEndpointID(value) {
    value = exactText(value, 'canvas endpoint id');
    if (value.indexOf(ENDPOINT_PREFIX) !== 0) throw new Error('canvas endpoint id is malformed');
    var offset = ENDPOINT_PREFIX.length;
    var edge = parseLengthSegment(value, offset, 'canvas endpoint id');
    var node = parseLengthSegment(value, edge.offset, 'canvas endpoint id');
    var role = parseLengthSegment(value, node.offset, 'canvas endpoint id');
    if (role.offset !== value.length || (role.value !== 'source' && role.value !== 'target')) {
      throw new Error('canvas endpoint id is malformed');
    }
    return { id: value, edgeID: edge.value, nodeID: node.value, role: role.value };
  }

  function edgeFor(graph, edgeID) {
    graph = graphObject(graph);
    edgeID = exactText(edgeID, 'canvas edge id');
    var edge = graph.edgesByID[edgeID];
    if (!edge || edge.id !== edgeID) throw new Error('canvas edge is absent from the graph');
    return edge;
  }

  function nodeFor(graph, nodeID) {
    graph = graphObject(graph);
    nodeID = exactText(nodeID, 'canvas node id');
    var node = graph.nodesByID[nodeID];
    if (!node || node.id !== nodeID) throw new Error('canvas node is absent from the graph');
    return node;
  }

  function endpointFor(graph, value) {
    var endpoint = parseEndpointID(value);
    var edge = edgeFor(graph, endpoint.edgeID);
    nodeFor(graph, endpoint.nodeID);
    var expectedNodeID = endpoint.role === 'source' ? edge.sourceID : edge.targetID;
    if (endpoint.nodeID !== expectedNodeID) {
      throw new Error('canvas endpoint does not match its edge role');
    }
    return { endpoint: endpoint, edge: edge };
  }

  function oppositeNodeID(graph, edgeID, nodeID) {
    var edge = edgeFor(graph, edgeID);
    nodeID = nodeFor(graph, nodeID).id;
    if (edge.sourceID === nodeID) return edge.targetID;
    if (edge.targetID === nodeID) return edge.sourceID;
    throw new Error('canvas node is not incident to the edge');
  }

  function endpointNavigationTarget(graph, value) {
    var resolved = endpointFor(graph, value);
    return oppositeNodeID(graph, resolved.edge.id, resolved.endpoint.nodeID);
  }

  function createInteractionState(initialPinnedNodeID) {
    return {
      hoveredNodeID: '',
      focusedNodeID: '',
      pinnedNodeID: optionalID(initialPinnedNodeID, 'initial pinned canvas node id'),
      hoveredEdgeID: '',
      focusedEdgeID: '',
      hoveredEndpointID: '',
      focusedEndpointID: '',
      activatedEndpointID: ''
    };
  }

  function interactionState(value) {
    if (!value || typeof value !== 'object' || Array.isArray(value)) {
      throw new Error('canvas interaction state must be an object');
    }
    return {
      hoveredNodeID: optionalID(value.hoveredNodeID, 'interaction.hoveredNodeID'),
      focusedNodeID: optionalID(value.focusedNodeID, 'interaction.focusedNodeID'),
      pinnedNodeID: optionalID(value.pinnedNodeID, 'interaction.pinnedNodeID'),
      hoveredEdgeID: optionalID(value.hoveredEdgeID, 'interaction.hoveredEdgeID'),
      focusedEdgeID: optionalID(value.focusedEdgeID, 'interaction.focusedEdgeID'),
      hoveredEndpointID: optionalID(value.hoveredEndpointID, 'interaction.hoveredEndpointID'),
      focusedEndpointID: optionalID(value.focusedEndpointID, 'interaction.focusedEndpointID'),
      activatedEndpointID: optionalID(value.activatedEndpointID, 'interaction.activatedEndpointID')
    };
  }

  function actionObject(action) {
    if (!action || typeof action !== 'object' || Array.isArray(action)) {
      throw new Error('canvas interaction action must be an object');
    }
    exactText(action.type, 'canvas interaction action type');
    return action;
  }

  function clearIfCurrent(current, candidate) {
    candidate = optionalID(candidate, 'canvas interaction leave id');
    return !candidate || current === candidate ? '' : current;
  }

  function reduceCanvasInteraction(previous, rawAction, graph) {
    var state = interactionState(previous);
    var action = actionObject(rawAction);
    var next = {
      hoveredNodeID: state.hoveredNodeID,
      focusedNodeID: state.focusedNodeID,
      pinnedNodeID: state.pinnedNodeID,
      hoveredEdgeID: state.hoveredEdgeID,
      focusedEdgeID: state.focusedEdgeID,
      hoveredEndpointID: state.hoveredEndpointID,
      focusedEndpointID: state.focusedEndpointID,
      activatedEndpointID: state.activatedEndpointID
    };

    switch (action.type) {
    case 'NODE_POINTER_ENTER':
      next.hoveredNodeID = nodeFor(graph, action.nodeID).id;
      break;
    case 'NODE_POINTER_LEAVE':
      next.hoveredNodeID = clearIfCurrent(next.hoveredNodeID, action.nodeID);
      break;
    case 'NODE_FOCUS':
      next.focusedNodeID = nodeFor(graph, action.nodeID).id;
      break;
    case 'NODE_BLUR':
      next.focusedNodeID = clearIfCurrent(next.focusedNodeID, action.nodeID);
      break;
    case 'NODE_PIN':
      var pinnedNodeID = nodeFor(graph, action.nodeID).id;
      next.pinnedNodeID = next.pinnedNodeID === pinnedNodeID ? '' : pinnedNodeID;
      break;
    case 'EDGE_POINTER_ENTER':
      next.hoveredEdgeID = edgeFor(graph, action.edgeID).id;
      break;
    case 'EDGE_POINTER_LEAVE':
      next.hoveredEdgeID = clearIfCurrent(next.hoveredEdgeID, action.edgeID);
      break;
    case 'EDGE_FOCUS':
      next.focusedEdgeID = edgeFor(graph, action.edgeID).id;
      break;
    case 'EDGE_BLUR':
      next.focusedEdgeID = clearIfCurrent(next.focusedEdgeID, action.edgeID);
      break;
    case 'ENDPOINT_POINTER_ENTER':
      endpointFor(graph, action.endpointID);
      next.hoveredEndpointID = action.endpointID;
      break;
    case 'ENDPOINT_POINTER_LEAVE':
      next.hoveredEndpointID = clearIfCurrent(next.hoveredEndpointID, action.endpointID);
      break;
    case 'ENDPOINT_FOCUS':
      endpointFor(graph, action.endpointID);
      next.focusedEndpointID = action.endpointID;
      break;
    case 'ENDPOINT_BLUR':
      next.focusedEndpointID = clearIfCurrent(next.focusedEndpointID, action.endpointID);
      break;
    case 'ENDPOINT_CLICK':
      endpointFor(graph, action.endpointID);
      next.activatedEndpointID = action.endpointID;
      break;
    case 'ESCAPE':
      next.pinnedNodeID = '';
      next.activatedEndpointID = '';
      break;
    case 'CLEAR_INTERACTION':
      return createInteractionState();
    default:
      throw new Error('canvas interaction action is not supported');
    }
    return next;
  }

  function pushUnique(values, seen, value) {
    if (seen[value]) return;
    seen[value] = true;
    values.push(value);
  }

  function compareText(left, right) {
    return left < right ? -1 : (left > right ? 1 : 0);
  }

  function stableIncidentEdges(graph, nodeID) {
    var edges = graph.incidentEdgesByNodeID[nodeID] || [];
    return edges.slice().sort(function (left, right) { return compareText(left.id, right.id); });
  }

  function deriveCanvasEmphasis(rawGraph, rawInteraction) {
    var graph = graphObject(rawGraph);
    var interaction = interactionState(rawInteraction);
    var activeEndpointID = interaction.hoveredEndpointID || interaction.focusedEndpointID;
    var activeEdgeID = '';
    var oppositeID = '';
    if (activeEndpointID) {
      var resolvedEndpoint = endpointFor(graph, activeEndpointID);
      activeEdgeID = resolvedEndpoint.edge.id;
      oppositeID = endpointNavigationTarget(graph, activeEndpointID);
    } else {
      activeEdgeID = interaction.hoveredEdgeID || interaction.focusedEdgeID;
      if (activeEdgeID) edgeFor(graph, activeEdgeID);
    }
    var activeNodeID = interaction.hoveredNodeID || interaction.focusedNodeID || interaction.pinnedNodeID;
    if (activeNodeID) nodeFor(graph, activeNodeID);

    var emphasizedNodeIDs = [];
    var emphasizedEdgeIDs = [];
    var visibleEndpointIDs = [];
    var seenNodes = Object.create(null);
    var seenEdges = Object.create(null);
    var seenEndpoints = Object.create(null);

    if (activeEdgeID) {
      var activeEdge = edgeFor(graph, activeEdgeID);
      pushUnique(emphasizedEdgeIDs, seenEdges, activeEdge.id);
      pushUnique(emphasizedNodeIDs, seenNodes, activeEdge.sourceID);
      pushUnique(emphasizedNodeIDs, seenNodes, activeEdge.targetID);
      pushUnique(visibleEndpointIDs, seenEndpoints, endpointID(activeEdge.id, activeEdge.sourceID, 'source'));
      pushUnique(visibleEndpointIDs, seenEndpoints, endpointID(activeEdge.id, activeEdge.targetID, 'target'));
    } else if (activeNodeID) {
      pushUnique(emphasizedNodeIDs, seenNodes, activeNodeID);
      stableIncidentEdges(graph, activeNodeID).forEach(function (edge) {
        pushUnique(emphasizedEdgeIDs, seenEdges, edge.id);
        pushUnique(emphasizedNodeIDs, seenNodes, edge.sourceID);
        pushUnique(emphasizedNodeIDs, seenNodes, edge.targetID);
        pushUnique(visibleEndpointIDs, seenEndpoints, endpointID(edge.id, edge.sourceID, 'source'));
        pushUnique(visibleEndpointIDs, seenEndpoints, endpointID(edge.id, edge.targetID, 'target'));
      });
    }

    return {
      mode: activeEdgeID ? 'edge' : (activeNodeID ? 'node' : 'none'),
      activeNodeID: activeNodeID,
      activeEdgeID: activeEdgeID,
      activeEndpointID: activeEndpointID,
      oppositeNodeID: oppositeID,
      pinnedNodeID: interaction.pinnedNodeID,
      visibleEndpointIDs: visibleEndpointIDs,
      emphasizedNodeIDs: emphasizedNodeIDs,
      emphasizedEdgeIDs: emphasizedEdgeIDs
    };
  }

  var api = Object.freeze({
    createInteractionState: createInteractionState,
    reduceCanvasInteraction: reduceCanvasInteraction,
    deriveCanvasEmphasis: deriveCanvasEmphasis,
    endpointID: endpointID,
    parseEndpointID: parseEndpointID,
    oppositeNodeID: oppositeNodeID,
    endpointNavigationTarget: endpointNavigationTarget
  });
  if (root.RepomapSystemCanvasInteraction) {
    throw new Error('canvas interaction asset namespace is already installed');
  }
  Object.defineProperty(root, 'RepomapSystemCanvasInteraction', {
    value: api,
    enumerable: false,
    configurable: false,
    writable: false
  });
})(globalThis);
