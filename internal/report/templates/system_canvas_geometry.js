(function (root) {
  'use strict';

  var PORT_SLOT_SPACING = 32;
  var PORT_VERTICAL_INSET = 18;
  var ARROW_TARGET_CLEARANCE = 12;

  function exactText(value, label) {
    if (typeof value !== 'string' || value.trim() !== value || !value) {
      throw new Error(label + ' must be exact non-empty text');
    }
    return value;
  }

  function finiteNumber(value, label) {
    if (typeof value !== 'number' || !Number.isFinite(value)) {
      throw new Error(label + ' must be a finite number');
    }
    return value;
  }

  function compareText(left, right) {
    return left < right ? -1 : (left > right ? 1 : 0);
  }

  function graphObject(graph) {
    if (!graph || typeof graph !== 'object' || Array.isArray(graph) ||
        !Array.isArray(graph.nodes) || !Array.isArray(graph.edges) || !graph.nodesByID) {
      throw new Error('canvas graph must contain nodes, edges, and its node index');
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
    return 'canvas-endpoint:' + String(edgeID.length) + ':' + edgeID +
      String(nodeID.length) + ':' + nodeID + String(role.length) + ':' + role;
  }

  function normalizedRect(rect, label) {
    if (!rect || typeof rect !== 'object' || Array.isArray(rect)) {
      throw new Error(label + ' must be a rectangle');
    }
    var left = finiteNumber(rect.left, label + '.left');
    var top = finiteNumber(rect.top, label + '.top');
    var width = finiteNumber(rect.width, label + '.width');
    var height = finiteNumber(rect.height, label + '.height');
    if (width < 0 || height < 0) throw new Error(label + ' dimensions must be non-negative');
    var right = Object.prototype.hasOwnProperty.call(rect, 'right') ?
      finiteNumber(rect.right, label + '.right') : left + width;
    var bottom = Object.prototype.hasOwnProperty.call(rect, 'bottom') ?
      finiteNumber(rect.bottom, label + '.bottom') : top + height;
    return {
      left: left,
      top: top,
      right: right,
      bottom: bottom,
      width: width,
      height: height,
      centerX: left + width / 2,
      centerY: top + height / 2
    };
  }

  function measureCanvasNodes(host, nodeElements) {
    if (!host || typeof host.getBoundingClientRect !== 'function') {
      throw new Error('canvas measurement host must expose its bounds');
    }
    if (!nodeElements || typeof nodeElements.length !== 'number') {
      throw new Error('canvas node elements must be an explicit array-like collection');
    }
    var hostBounds = normalizedRect(host.getBoundingClientRect(), 'canvas bounds');
    var nodesByID = Object.create(null);
    var nodeIDs = [];
    Array.prototype.forEach.call(nodeElements, function (element) {
      if (!element || typeof element.getAttribute !== 'function' ||
          typeof element.getBoundingClientRect !== 'function') {
        throw new Error('canvas node element must expose identity and bounds');
      }
      var nodeID = exactText(element.getAttribute('data-canvas-node'), 'measured canvas node id');
      if (nodesByID[nodeID]) throw new Error('canvas node measurement identity is duplicated');
      var absolute = normalizedRect(element.getBoundingClientRect(), 'canvas node bounds');
      nodesByID[nodeID] = {
        left: absolute.left - hostBounds.left,
        top: absolute.top - hostBounds.top,
        right: absolute.right - hostBounds.left,
        bottom: absolute.bottom - hostBounds.top,
        width: absolute.width,
        height: absolute.height,
        centerX: absolute.centerX - hostBounds.left,
        centerY: absolute.centerY - hostBounds.top
      };
      nodeIDs.push(nodeID);
    });
    nodeIDs.sort();
    return {
      width: hostBounds.width,
      height: hostBounds.height,
      nodeIDs: nodeIDs,
      nodesByID: nodesByID
    };
  }

  function nodeLane(graph, nodeID) {
    var node = graph.nodesByID[nodeID];
    if (!node) throw new Error('canvas edge endpoint is absent from the graph');
    var lane = exactText(node.lane, 'canvas node lane');
    if (lane !== 'entry' && lane !== 'core' && lane !== 'integration') {
      throw new Error('canvas node lane is not supported');
    }
    return lane;
  }

  function validateEdge(graph, edge) {
    if (!edge || typeof edge !== 'object' || Array.isArray(edge)) {
      throw new Error('canvas edge must be an object');
    }
    var edgeID = exactText(edge.id, 'canvas edge id');
    var sourceID = exactText(edge.sourceID, 'canvas edge source id');
    var targetID = exactText(edge.targetID, 'canvas edge target id');
    nodeLane(graph, sourceID);
    nodeLane(graph, targetID);
    var authority = exactText(edge.authority, 'canvas edge authority');
    if (authority !== 'exact' && authority !== 'possible' && authority !== 'runtime') {
      throw new Error('canvas edge authority is not supported');
    }
    return { id: edgeID, sourceID: sourceID, targetID: targetID, authority: authority };
  }

  function endpointSort(left, right) {
    return compareText(left.edgeID, right.edgeID) ||
      compareText(left.role, right.role) || compareText(left.id, right.id);
  }

  function portLayoutRequirements(rawGraph) {
    var graph = graphObject(rawGraph);
    var countsByNodeID = Object.create(null);
    graph.nodes.forEach(function (node) {
      countsByNodeID[node.id] = { left: 0, right: 0 };
    });
    graph.edges.forEach(function (rawEdge) {
      var edge = validateEdge(graph, rawEdge);
      var sameLane = nodeLane(graph, edge.sourceID) === nodeLane(graph, edge.targetID);
      countsByNodeID[edge.sourceID].right++;
      countsByNodeID[edge.targetID][sameLane ? 'right' : 'left']++;
    });
    var minHeightByNodeID = Object.create(null);
    Object.keys(countsByNodeID).sort().forEach(function (nodeID) {
      var counts = countsByNodeID[nodeID];
      var slots = Math.max(counts.left, counts.right);
      minHeightByNodeID[nodeID] = slots > 1 ?
        PORT_VERTICAL_INSET * 2 + (slots - 1) * PORT_SLOT_SPACING : 0;
    });
    return {
      slotSpacing: PORT_SLOT_SPACING,
      verticalInset: PORT_VERTICAL_INSET,
      countsByNodeID: countsByNodeID,
      minHeightByNodeID: minHeightByNodeID
    };
  }

  function assignStablePorts(rawGraph, measurements) {
    var graph = graphObject(rawGraph);
    if (!measurements || typeof measurements !== 'object' || Array.isArray(measurements) ||
        !measurements.nodesByID) {
      throw new Error('canvas measurements must contain the node index');
    }
    var slots = Object.create(null);
    var ports = [];
    var portsByID = Object.create(null);
    var portsByEdgeID = Object.create(null);
    var portsByNodeID = Object.create(null);

    function addPort(edge, role, nodeID, oppositeNodeID, side) {
      if (!measurements.nodesByID[nodeID]) {
        throw new Error('canvas endpoint has no node measurement');
      }
      var id = endpointID(edge.id, nodeID, role);
      var port = {
        id: id,
        edgeID: edge.id,
        nodeID: nodeID,
        oppositeNodeID: oppositeNodeID,
        role: role,
        side: side,
        index: 0,
        total: 0,
        offset: 0
      };
      if (portsByID[id]) throw new Error('canvas endpoint identity is duplicated');
      portsByID[id] = port;
      ports.push(port);
      if (!portsByEdgeID[edge.id]) portsByEdgeID[edge.id] = Object.create(null);
      portsByEdgeID[edge.id][role] = port;
      if (!portsByNodeID[nodeID]) portsByNodeID[nodeID] = [];
      portsByNodeID[nodeID].push(port);
      var slotKey = nodeID + '\u0000' + side;
      if (!slots[slotKey]) slots[slotKey] = [];
      slots[slotKey].push(port);
    }

    graph.edges.slice().sort(function (left, right) {
      return compareText(exactText(left.id, 'canvas edge id'), exactText(right.id, 'canvas edge id'));
    }).forEach(function (rawEdge) {
      var edge = validateEdge(graph, rawEdge);
      var sameLane = nodeLane(graph, edge.sourceID) === nodeLane(graph, edge.targetID);
      addPort(edge, 'source', edge.sourceID, edge.targetID, 'right');
      addPort(edge, 'target', edge.targetID, edge.sourceID, sameLane ? 'right' : 'left');
    });

    Object.keys(slots).sort().forEach(function (slotKey) {
      var members = slots[slotKey].slice().sort(function (left, right) {
        var leftBounds = normalizedRect(measurements.nodesByID[left.oppositeNodeID],
          'canvas opposite endpoint measurement');
        var rightBounds = normalizedRect(measurements.nodesByID[right.oppositeNodeID],
          'canvas opposite endpoint measurement');
        return leftBounds.centerY - rightBounds.centerY || endpointSort(left, right);
      });
      var total = members.length;
      var nodeBounds = normalizedRect(measurements.nodesByID[members[0].nodeID],
        'canvas port node measurement');
      var availableSpread = Math.max(0, nodeBounds.height - PORT_VERTICAL_INSET * 2);
      var spread = Math.min(availableSpread, Math.max(0, total - 1) * PORT_SLOT_SPACING);
      members.forEach(function (port, index) {
        port.index = index;
        port.total = total;
        port.offset = total === 1 ? 0 : (-spread / 2) + (spread * index / (total - 1));
      });
    });
    Object.keys(portsByNodeID).forEach(function (nodeID) {
      portsByNodeID[nodeID].sort(function (left, right) {
        return compareText(left.side, right.side) || left.index - right.index || endpointSort(left, right);
      });
    });
    ports.sort(function (left, right) { return compareText(left.id, right.id); });
    return {
      ports: ports,
      portsByID: portsByID,
      portsByEdgeID: portsByEdgeID,
      portsByNodeID: portsByNodeID
    };
  }

  function geometryMeasurements(value) {
    if (!value || typeof value !== 'object' || Array.isArray(value) || !value.nodesByID) {
      throw new Error('canvas measurements must contain the node index');
    }
    finiteNumber(value.width, 'canvas measurement width');
    finiteNumber(value.height, 'canvas measurement height');
    return value;
  }

  function buildEdgeGeometry(rawGraph, rawMeasurements, assignment) {
    var graph = graphObject(rawGraph);
    var measurements = geometryMeasurements(rawMeasurements);
    if (!assignment || typeof assignment !== 'object' || Array.isArray(assignment) ||
        !assignment.portsByEdgeID) {
      throw new Error('canvas port assignment must contain its edge index');
    }
    var sameLaneTracksByEdgeID = Object.create(null);
    var sameLaneIntervals = graph.edges.map(function (rawEdge) {
      var edge = validateEdge(graph, rawEdge);
      if (nodeLane(graph, edge.sourceID) !== nodeLane(graph, edge.targetID)) return null;
      var edgePorts = assignment.portsByEdgeID[edge.id];
      if (!edgePorts || !edgePorts.source || !edgePorts.target) {
        throw new Error('canvas edge has incomplete stable port assignment');
      }
      var sourceBounds = normalizedRect(measurements.nodesByID[edge.sourceID], 'canvas source measurement');
      var targetBounds = normalizedRect(measurements.nodesByID[edge.targetID], 'canvas target measurement');
      var sourceY = sourceBounds.centerY + edgePorts.source.offset;
      var targetY = targetBounds.centerY + edgePorts.target.offset;
      return { id: edge.id, start: Math.min(sourceY, targetY), end: Math.max(sourceY, targetY) };
    }).filter(Boolean).sort(function (left, right) {
      return left.start - right.start || left.end - right.end || compareText(left.id, right.id);
    });
    var trackEnds = [];
    sameLaneIntervals.forEach(function (interval) {
      var track = 0;
      while (track < trackEnds.length && interval.start <= trackEnds[track] + PORT_VERTICAL_INSET) track++;
      if (track === trackEnds.length) trackEnds.push(interval.end);
      else trackEnds[track] = interval.end;
      sameLaneTracksByEdgeID[interval.id] = track;
    });

    var edges = [];
    var edgesByID = Object.create(null);
    graph.edges.slice().sort(function (left, right) {
      return compareText(exactText(left.id, 'canvas edge id'), exactText(right.id, 'canvas edge id'));
    }).forEach(function (rawEdge) {
      var edge = validateEdge(graph, rawEdge);
      if (edgesByID[edge.id]) throw new Error('canvas edge geometry identity is duplicated');
      var sourceBounds = normalizedRect(measurements.nodesByID[edge.sourceID], 'canvas source measurement');
      var targetBounds = normalizedRect(measurements.nodesByID[edge.targetID], 'canvas target measurement');
      var edgePorts = assignment.portsByEdgeID[edge.id];
      if (!edgePorts || !edgePorts.source || !edgePorts.target) {
        throw new Error('canvas edge has incomplete stable port assignment');
      }
      var sameLane = nodeLane(graph, edge.sourceID) === nodeLane(graph, edge.targetID);
      var sourceX = sourceBounds.right;
      var sourceY = sourceBounds.centerY + edgePorts.source.offset;
      var targetPortX = sameLane ? targetBounds.right : targetBounds.left;
      var targetY = targetBounds.centerY + edgePorts.target.offset;
      var authorityOffset = edge.authority === 'possible' ? 8 : (edge.authority === 'runtime' ? 16 : 0);
      var horizontalGap = Math.abs(targetPortX - sourceX);
      var crossLaneBend = Math.min(horizontalGap / 2,
        Math.max(26, horizontalGap * 0.4 + authorityOffset));
      var verticalSpan = Math.abs(targetY - sourceY);
      var track = sameLane ? sameLaneTracksByEdgeID[edge.id] || 0 : 0;
      var bend = sameLane ? 42 + track * 22 + Math.min(44, verticalSpan * 0.18) + authorityOffset : crossLaneBend;
      var targetX = targetPortX + (sameLane ? ARROW_TARGET_CLEARANCE : -ARROW_TARGET_CLEARANCE);
      var path = sameLane ?
        'M ' + sourceX + ' ' + sourceY + ' C ' + (sourceX + bend) + ' ' + sourceY + ', ' +
          (targetX + bend) + ' ' + targetY + ', ' + targetX + ' ' + targetY :
        'M ' + sourceX + ' ' + sourceY + ' C ' + (sourceX + bend) + ' ' + sourceY + ', ' +
          (targetX - bend) + ' ' + targetY + ', ' + targetX + ' ' + targetY;
      var geometry = {
        id: edge.id,
        sourceID: edge.sourceID,
        targetID: edge.targetID,
        authority: edge.authority,
        track: track,
        sourcePortID: edgePorts.source.id,
        targetPortID: edgePorts.target.id,
        source: { x: sourceX, y: sourceY },
        targetPort: { x: targetPortX, y: targetY },
        target: { x: targetX, y: targetY },
        path: path
      };
      edgesByID[geometry.id] = geometry;
      edges.push(geometry);
    });
    return {
      width: measurements.width,
      height: measurements.height,
      edges: edges,
      edgesByID: edgesByID
    };
  }

  var api = Object.freeze({
    portLayoutRequirements: portLayoutRequirements,
    measureCanvasNodes: measureCanvasNodes,
    assignStablePorts: assignStablePorts,
    buildEdgeGeometry: buildEdgeGeometry
  });
  if (root.RepomapSystemCanvasGeometry) {
    throw new Error('canvas geometry asset namespace is already installed');
  }
  Object.defineProperty(root, 'RepomapSystemCanvasGeometry', {
    value: api,
    enumerable: false,
    configurable: false,
    writable: false
  });
})(globalThis);
