(function (global) {
  'use strict';

  var SVG_NAMESPACE = 'http://www.w3.org/2000/svg';
  var mountedByHost = new WeakMap();
  var mountSequence = 0;

  function requireNamespace(name) {
    var value = global[name];
    if (!value) throw new Error('repomap System canvas requires ' + name);
    return value;
  }

  function element(tagName, className, text) {
    var node = document.createElement(tagName);
    if (className) node.className = className;
    if (text != null) node.textContent = String(text);
    return node;
  }

  function svgElement(tagName, className) {
    var node = document.createElementNS(SVG_NAMESPACE, tagName);
    if (className) node.setAttribute('class', className);
    return node;
  }

  function setClass(node, className, enabled) {
    if (!node) return;
    if (node.classList) {
      node.classList.toggle(className, !!enabled);
      return;
    }
    var classes = String(node.getAttribute('class') || '').split(/\s+/).filter(Boolean);
    var position = classes.indexOf(className);
    if (enabled && position < 0) classes.push(className);
    if (!enabled && position >= 0) classes.splice(position, 1);
    node.setAttribute('class', classes.join(' '));
  }

  function lookup(values) {
    var result = Object.create(null);
    (values || []).forEach(function (value) { result[value] = true; });
    return result;
  }

  function closestWithin(target, selector, root) {
    while (target && target !== root) {
      if (target.matches && target.matches(selector)) return target;
      target = target.parentNode;
    }
    return root && root.matches && root.matches(selector) ? root : null;
  }

  function movedWithin(event, container) {
    return !!(event.relatedTarget && container && container.contains(event.relatedTarget));
  }

  function authority(edge) {
    return edge.authority;
  }

  function displayKind(node) {
    return node.lane;
  }

  function nodeName(node) {
    return node.name;
  }

  function nodeData(node) {
    return node.data;
  }

  function laneForNode(node) {
    return node.lane;
  }

  function defaultNodeMeta(node, graph) {
    var value = nodeData(node);
    if (node.kind === 'entrypoint') {
      var location = value.location || {};
      if (location.path) return location.path + (location.line ? ':' + String(location.line) : '');
      return '';
    }
    if (node.kind === 'integration') {
      var uses = value.useCount;
      return String(uses) + ' selected use' + (uses === 1 ? '' : 's');
    }
    var symbols = value.declarationCount;
    var areaCount = value.areaCount;
    var result = String(symbols) + ' declaration' + (symbols === 1 ? '' : 's');
    if (areaCount) {
      result += ' · ' + String(areaCount) + ' area' + (areaCount === 1 ? '' : 's');
    }
    return result;
  }

  function defaultCoreFlow(node, graph) {
    if (node.lane !== 'core') return '';
    var incoming = graph.incomingByNodeID[node.id] || [];
    var outgoing = graph.outgoingByNodeID[node.id] || [];
    var counts = { incoming: 0, outgoing: 0, possibleIncoming: 0, possibleOutgoing: 0 };
    incoming.forEach(function (edge) {
      var source = graph.nodesByID[edge.sourceID];
      if (!source || source.lane !== 'core') return;
      if (authority(edge) === 'exact') counts.incoming++;
      else if (authority(edge) === 'possible') counts.possibleIncoming++;
    });
    outgoing.forEach(function (edge) {
      var target = graph.nodesByID[edge.targetID];
      if (!target || target.lane !== 'core') return;
      if (authority(edge) === 'exact') counts.outgoing++;
      else if (authority(edge) === 'possible') counts.possibleOutgoing++;
    });
    var parts = [];
    if (counts.outgoing) parts.push('→ ' + String(counts.outgoing) + ' exact downstream');
    if (counts.incoming) parts.push('← ' + String(counts.incoming) + ' exact upstream');
    if (counts.possibleOutgoing) parts.push('⇢ ' + String(counts.possibleOutgoing) + ' possible downstream');
    if (counts.possibleIncoming) parts.push('⇠ ' + String(counts.possibleIncoming) + ' possible upstream');
    return parts.join(' · ');
  }

  function nodeHref(node, options, callbacks) {
    var resolve = callbacks.hrefForNode;
    return typeof resolve === 'function' ? String(resolve(node) || '') : '';
  }

  function renderNode(node, graph, options, callbacks) {
    var kind = displayKind(node);
    var name = nodeName(node);
    var wrapper = element('div', 'rm-canvas-node-wrap rm-canvas-node-wrap--' + kind);
    wrapper.setAttribute('data-canvas-node-wrap', node.id);
    var href = nodeHref(node, options, callbacks);
    var control = element(href ? 'a' : 'button', 'rm-canvas-node rm-canvas-node--' + kind);
    if (href) control.setAttribute('href', href);
    else control.type = 'button';
    control.setAttribute('data-canvas-node', node.id);
    control.setAttribute('data-canvas-label', name);
    if (options.selectedNodeID === node.id) control.setAttribute('aria-current', 'true');
    control.appendChild(element('span', 'rm-canvas-node__name', name));
    var meta = typeof options.nodeMeta === 'function' ? options.nodeMeta(node, graph) : defaultNodeMeta(node, graph);
    if (meta) control.appendChild(element('span', 'rm-canvas-node__meta', meta));
    var flow = typeof options.nodeFlow === 'function' ? options.nodeFlow(node, graph) : defaultCoreFlow(node, graph);
    if (flow) control.appendChild(element('span', 'rm-canvas-node__flow', flow));
    var action = typeof options.nodeAction === 'function' ? options.nodeAction(node, graph) :
      (kind === 'core' ? 'Inspect responsibility ↓' : '');
    if (action) control.appendChild(element('span', 'rm-canvas-node__action', action));
    wrapper.appendChild(control);
    return wrapper;
  }

  function renderLaneHeader(title, summary) {
    var header = element('header', 'rm-canvas-lane__header');
    header.appendChild(element('h3', '', title));
    header.appendChild(element('span', '', summary || ''));
    return header;
  }

  function laneTitle(lane) {
    if (lane === 'entry') return 'Entrypoints';
    if (lane === 'integration') return 'Integrations';
    return 'Core';
  }

  function defaultLaneSummary(lane, nodes, graph) {
    if (lane === 'core') {
      var coreEdges = graph.edges.filter(function (edge) {
        var source = graph.nodesByID[edge.sourceID];
        var target = graph.nodesByID[edge.targetID];
        return source && target && source.lane === 'core' && target.lane === 'core';
      });
      return String(nodes.length) + ' responsibilities · ' + String(coreEdges.length) +
        ' directional core link' + (coreEdges.length === 1 ? '' : 's');
    }
    return String(nodes.length) + ' shown';
  }

  function renderCoreGroup(nodes, graph, options, callbacks) {
    var group = options.coreGroup;
    if (!group) {
      var simple = element('div', 'rm-canvas-node-list');
      nodes.forEach(function (node) { simple.appendChild(renderNode(node, graph, options, callbacks)); });
      return simple;
    }
    var list = element('div', 'rm-canvas-node-list rm-core-group-list');
    var section = element('section', 'rm-core-group rm-core-group--active');
    if (group.authority === 'local_unassigned') section.className += ' rm-core-group--local-unassigned';
    var headingID = 'rm-core-group-' + encodeURIComponent(group.id);
    section.setAttribute('aria-labelledby', headingID);
    var header = element('header', 'rm-core-group__header');
    var copy = element('div');
    if (group.eyebrow) copy.appendChild(element('p', 'rm-eyebrow', group.eyebrow));
    var heading = element('h4', '', group.name);
    heading.id = headingID;
    copy.appendChild(heading);
    if (group.purpose) copy.appendChild(element('p', '', group.purpose));
    header.appendChild(copy);
    header.appendChild(element('span', '', group.summary || (String(nodes.length) + ' responsibilities')));
    section.appendChild(header);
    var groupNodes = element('div', 'rm-core-group__nodes');
    nodes.forEach(function (node) { groupNodes.appendChild(renderNode(node, graph, options, callbacks)); });
    section.appendChild(groupNodes);
    list.appendChild(section);
    return list;
  }

  function renderCanvas(graph, options, callbacks) {
    var canvas = element('div', 'rm-flow-canvas');
    canvas.setAttribute('data-system-canvas', 'true');
    var svg = svgElement('svg', 'rm-canvas-edges');
    svg.setAttribute('aria-hidden', 'true');
    canvas.appendChild(svg);
    var lanes = element('div', 'rm-canvas-lanes');
    ['entry', 'core', 'integration'].forEach(function (lane) {
      var laneNodes = graph.nodes.filter(function (node) { return laneForNode(node) === lane; });
      var laneNode = element('section', 'rm-canvas-lane rm-canvas-lane--' + lane);
      var summaries = options.laneSummaries || {};
      laneNode.appendChild(renderLaneHeader(laneTitle(lane), summaries[lane] || defaultLaneSummary(lane, laneNodes, graph)));
      if (lane === 'core' && options.coreLeadNote) {
        laneNode.appendChild(element('p', 'rm-canvas-core-note', options.coreLeadNote));
      }
      var nodeList = lane === 'core' ? renderCoreGroup(laneNodes, graph, options, callbacks) :
        element('div', 'rm-canvas-node-list');
      if (lane !== 'core') {
        laneNodes.forEach(function (node) { nodeList.appendChild(renderNode(node, graph, options, callbacks)); });
      }
      var emptyMessages = options.laneEmptyMessages || {};
      if (!laneNodes.length && emptyMessages[lane]) {
        nodeList.appendChild(element('p', 'rm-canvas-empty', emptyMessages[lane]));
      }
      var notes = options.laneNotes && options.laneNotes[lane];
      (Array.isArray(notes) ? notes : notes ? [notes] : []).forEach(function (note) {
        nodeList.appendChild(element('p', 'rm-canvas-frontier', note));
      });
      laneNode.appendChild(nodeList);
      lanes.appendChild(laneNode);
    });
    canvas.appendChild(lanes);
    var accessibleEdges = element('ul', 'rm-sr-only');
    graph.edges.forEach(function (edge) {
      accessibleEdges.appendChild(element('li', '', edge.description));
    });
    canvas.appendChild(accessibleEdges);
    return canvas;
  }

  function appendMarkers(svg, markerPrefix) {
    var definitions = svgElement('defs');
    ['exact', 'possible', 'runtime'].forEach(function (kind) {
      var marker = svgElement('marker');
      marker.setAttribute('id', markerPrefix + '-' + kind);
      marker.setAttribute('viewBox', '0 0 8 8');
      marker.setAttribute('refX', '7');
      marker.setAttribute('refY', '4');
      marker.setAttribute('markerWidth', '6');
      marker.setAttribute('markerHeight', '6');
      marker.setAttribute('orient', 'auto');
      var arrow = svgElement('path', 'rm-canvas-arrow rm-canvas-arrow--' + kind);
      arrow.setAttribute('d', 'M 0 0 L 8 4 L 0 8 z');
      marker.appendChild(arrow);
      definitions.appendChild(marker);
    });
    svg.appendChild(definitions);
  }

  function mountSystemCanvas(host, graph, suppliedOptions, suppliedCallbacks) {
    if (!host || host.nodeType !== 1) throw new Error('repomap System canvas host is required');
    if (!graph || !Array.isArray(graph.nodes) || !Array.isArray(graph.edges) ||
        !graph.nodesByID || !graph.edgesByID || !graph.incidentEdgesByNodeID) {
      throw new Error('repomap System canvas graph is incomplete');
    }
    unmountSystemCanvas(host);
    var interaction = requireNamespace('RepomapSystemCanvasInteraction');
    var geometry = requireNamespace('RepomapSystemCanvasGeometry');
    var options = suppliedOptions || {};
    var callbacks = suppliedCallbacks || {};
    var canvas = renderCanvas(graph, options, callbacks);
    host.replaceChildren(canvas);
    var svg = canvas.querySelector('.rm-canvas-edges');
    var markerPrefix = 'rm-canvas-arrow-' + String(++mountSequence);
    var diagnostics = { geometryBuildCount: 0 };
    var interactionState = interaction.createInteractionState(options.initialPinnedNodeID || null);
    var geometryFrame = 0;
    var destroyed = false;
    var resizeObserver = null;
    var lastLayoutSignature = '';
    var nodeElementsByID = Object.create(null);
    var edgeElementsByID = Object.create(null);
    var portElementsByID = Object.create(null);
    var previousEmphasis = interaction.deriveCanvasEmphasis(graph, interactionState);
    Array.prototype.forEach.call(canvas.querySelectorAll('[data-canvas-node]'), function (node) {
      nodeElementsByID[node.getAttribute('data-canvas-node')] = node;
    });

    function dispatch(action) {
      interactionState = interaction.reduceCanvasInteraction(interactionState, action, graph);
      applyEmphasis(interaction.deriveCanvasEmphasis(graph, interactionState));
    }

    function affectedIDs(previous, current, listName, scalarNames, all) {
      var result = Object.create(null);
      if (all) Object.keys(all).forEach(function (id) { result[id] = true; });
      [previous, current].forEach(function (value) {
        (value[listName] || []).forEach(function (id) { result[id] = true; });
        scalarNames.forEach(function (name) {
          if (value[name]) result[value[name]] = true;
        });
      });
      return Object.keys(result);
    }

    function applyEmphasis(emphasis, force) {
      var emphasizedNodes = lookup(emphasis.emphasizedNodeIDs);
      var emphasizedEdges = lookup(emphasis.emphasizedEdgeIDs);
      var visibleEndpoints = lookup(emphasis.visibleEndpointIDs);
      var activeNodeID = emphasis.activeNodeID || '';
      var activeEdgeID = emphasis.activeEdgeID || '';
      var activeEndpointID = emphasis.activeEndpointID || '';
      var oppositeNodeID = emphasis.oppositeNodeID || '';
      if (emphasis.mode && emphasis.mode !== 'none') canvas.setAttribute('data-canvas-highlight', emphasis.mode);
      else canvas.removeAttribute('data-canvas-highlight');
      affectedIDs(previousEmphasis, emphasis, 'emphasizedEdgeIDs', ['activeEdgeID'],
        force ? edgeElementsByID : null).forEach(function (edgeID) {
        var group = edgeElementsByID[edgeID];
        if (!group) return;
        var highlighted = !!emphasizedEdges[edgeID];
        setClass(group, 'rm-canvas-edge-group--related', highlighted);
        setClass(group, 'rm-canvas-edge-group--active', edgeID === activeEdgeID);
      });
      affectedIDs(previousEmphasis, emphasis, 'emphasizedNodeIDs', ['activeNodeID', 'oppositeNodeID'],
        force ? nodeElementsByID : null).forEach(function (nodeID) {
        var node = nodeElementsByID[nodeID];
        if (!node) return;
        setClass(node, 'rm-canvas-node--edge-active', nodeID === activeNodeID);
        setClass(node, 'rm-canvas-node--edge-related', !!emphasizedNodes[nodeID] && nodeID !== activeNodeID);
        setClass(node, 'rm-canvas-node--edge-opposite', nodeID === oppositeNodeID);
      });
      affectedIDs(previousEmphasis, emphasis, 'visibleEndpointIDs', ['activeEndpointID'],
        force ? portElementsByID : null).forEach(function (endpointID) {
        var port = portElementsByID[endpointID];
        if (!port) return;
        var visible = !!visibleEndpoints[endpointID];
        setClass(port, 'rm-canvas-edge-port--related', visible);
        setClass(port, 'rm-canvas-edge-port--active', endpointID === activeEndpointID);
        port.setAttribute('tabindex', visible ? '0' : '-1');
        port.setAttribute('aria-hidden', visible ? 'false' : 'true');
      });
      previousEmphasis = emphasis;
    }

    function nodeControl(nodeID) {
      return nodeElementsByID[nodeID] || null;
    }

    function focusAndCenterNode(nodeID) {
      var target = nodeControl(nodeID);
      if (!target) return;
      if (typeof target.scrollIntoView === 'function') {
        var reduced = typeof global.matchMedia === 'function' &&
          global.matchMedia('(prefers-reduced-motion: reduce)').matches;
        target.scrollIntoView({ behavior: reduced ? 'auto' : 'smooth', block: 'center', inline: 'nearest' });
      }
      if (typeof target.focus === 'function') target.focus({ preventScroll: true });
    }

    function appendPort(port) {
      var control = nodeControl(port.nodeID);
      if (!control || !control.parentNode) return;
      var opposite = graph.nodesByID[port.oppositeNodeID];
      var edge = graph.edgesByID[port.edgeID];
      var button = element('button', 'rm-canvas-edge-port rm-canvas-edge-port--' + port.side +
        ' rm-canvas-edge-port--' + authority(edge));
      button.type = 'button';
      button.style.setProperty('--rm-canvas-edge-offset', String(port.offset || 0) + 'px');
      button.setAttribute('data-canvas-edge-port', port.id);
      button.setAttribute('data-canvas-edge-id', port.edgeID);
      button.setAttribute('data-canvas-node-id', port.nodeID);
      button.setAttribute('data-canvas-opposite-node-id', port.oppositeNodeID);
      button.setAttribute('tabindex', '-1');
      button.setAttribute('aria-hidden', 'true');
      var oppositeName = nodeName(opposite);
      button.setAttribute('aria-label', 'Follow this connection to ' + oppositeName);
      button.title = edge ? edge.description : oppositeName;
      button.appendChild(element('span', 'rm-canvas-edge-port__label', oppositeName));
      control.parentNode.appendChild(button);
      portElementsByID[port.id] = button;
    }

    function measureGeometry() {
      return geometry.measureCanvasNodes(canvas, canvas.querySelectorAll('[data-canvas-node]'));
    }

    function measurementSignature(measurements) {
      var parts = [measurements.width, measurements.height];
      (measurements.nodeIDs || Object.keys(measurements.nodesByID).sort()).forEach(function (nodeID) {
        var bounds = measurements.nodesByID[nodeID];
        parts.push(nodeID, bounds.left, bounds.top, bounds.width, bounds.height);
      });
      return parts.map(function (value) {
        return typeof value === 'number' ? String(Math.round(value * 100) / 100) : String(value);
      }).join('|');
    }

    function buildGeometry(measurements) {
      if (destroyed) return;
      diagnostics.geometryBuildCount++;
      measurements = measurements || measureGeometry();
      var assignment = geometry.assignStablePorts(graph, measurements);
      var built = geometry.buildEdgeGeometry(graph, measurements, assignment);
      svg.replaceChildren();
      appendMarkers(svg, markerPrefix);
      svg.setAttribute('viewBox', '0 0 ' + String(built.width) + ' ' + String(built.height));
      svg.setAttribute('width', String(built.width));
      svg.setAttribute('height', String(built.height));
      Object.keys(portElementsByID).forEach(function (endpointID) {
        var port = portElementsByID[endpointID];
        if (port.parentNode) port.parentNode.removeChild(port);
      });
      edgeElementsByID = Object.create(null);
      portElementsByID = Object.create(null);
      built.edges.forEach(function (edgeGeometry) {
        var kind = edgeGeometry.authority || authority(graph.edgesByID[edgeGeometry.id]);
        var group = svgElement('g', 'rm-canvas-edge-group');
        group.setAttribute('data-canvas-edge-id', edgeGeometry.id);
        group.setAttribute('data-canvas-edge-from', edgeGeometry.sourceID);
        group.setAttribute('data-canvas-edge-to', edgeGeometry.targetID);
        var path = svgElement('path', 'rm-canvas-edge rm-canvas-edge--' + kind);
        path.setAttribute('d', edgeGeometry.path);
        path.setAttribute('marker-end', 'url(#' + markerPrefix + '-' + kind + ')');
        group.appendChild(path);
        svg.appendChild(group);
        edgeElementsByID[edgeGeometry.id] = group;
      });
      assignment.ports.forEach(appendPort);
      applyEmphasis(interaction.deriveCanvasEmphasis(graph, interactionState), true);
    }

    var geometryForce = false;
    function scheduleGeometryBuild(force) {
      if (destroyed) return;
      geometryForce = geometryForce || force === true;
      if (geometryFrame) return;
      var schedule = typeof global.requestAnimationFrame === 'function' ? global.requestAnimationFrame :
        function (callback) { return global.setTimeout(callback, 0); };
      geometryFrame = schedule(function () {
        geometryFrame = 0;
        if (destroyed) return;
        var measurements = measureGeometry();
        var signature = measurementSignature(measurements);
        var forced = geometryForce;
        geometryForce = false;
        if (!forced && signature === lastLayoutSignature) return;
        lastLayoutSignature = signature;
        buildGeometry(measurements);
      });
    }

    function endpointFromEvent(event) {
      return closestWithin(event.target, '[data-canvas-edge-port]', canvas);
    }

    function nodeWrapFromEvent(event) {
      return closestWithin(event.target, '[data-canvas-node-wrap]', canvas);
    }

    function handlePointerOver(event) {
      var endpoint = endpointFromEvent(event);
      if (endpoint && !movedWithin(event, endpoint)) {
        dispatch({ type: 'ENDPOINT_POINTER_ENTER', endpointID: endpoint.getAttribute('data-canvas-edge-port') });
      }
      var wrapper = nodeWrapFromEvent(event);
      if (wrapper && !movedWithin(event, wrapper)) {
        dispatch({ type: 'NODE_POINTER_ENTER', nodeID: wrapper.getAttribute('data-canvas-node-wrap') });
      }
    }

    function handlePointerOut(event) {
      var endpoint = endpointFromEvent(event);
      if (endpoint && !movedWithin(event, endpoint)) {
        dispatch({ type: 'ENDPOINT_POINTER_LEAVE', endpointID: endpoint.getAttribute('data-canvas-edge-port') });
      }
      var wrapper = nodeWrapFromEvent(event);
      if (wrapper && !movedWithin(event, wrapper)) {
        dispatch({ type: 'NODE_POINTER_LEAVE', nodeID: wrapper.getAttribute('data-canvas-node-wrap') });
      }
    }

    function handleFocusIn(event) {
      var endpoint = endpointFromEvent(event);
      if (endpoint) {
        dispatch({ type: 'ENDPOINT_FOCUS', endpointID: endpoint.getAttribute('data-canvas-edge-port') });
        return;
      }
      var wrapper = nodeWrapFromEvent(event);
      if (wrapper) dispatch({ type: 'NODE_FOCUS', nodeID: wrapper.getAttribute('data-canvas-node-wrap') });
    }

    function handleFocusOut(event) {
      var endpoint = endpointFromEvent(event);
      if (endpoint && !movedWithin(event, endpoint)) {
        dispatch({ type: 'ENDPOINT_BLUR', endpointID: endpoint.getAttribute('data-canvas-edge-port') });
      }
      var wrapper = nodeWrapFromEvent(event);
      if (wrapper && !movedWithin(event, wrapper)) {
        dispatch({ type: 'NODE_BLUR', nodeID: wrapper.getAttribute('data-canvas-node-wrap') });
      }
    }

    function handleClick(event) {
      var endpoint = endpointFromEvent(event);
      if (endpoint) {
        event.preventDefault();
        event.stopPropagation();
        var endpointID = endpoint.getAttribute('data-canvas-edge-port');
        dispatch({ type: 'ENDPOINT_CLICK', endpointID: endpointID });
        var targetID = interaction.endpointNavigationTarget(graph, endpointID);
        focusAndCenterNode(targetID);
        if (typeof callbacks.endpointActivated === 'function') {
          callbacks.endpointActivated(endpointID, targetID);
        }
        return;
      }
      var wrapper = nodeWrapFromEvent(event);
      if (!wrapper) return;
      var nodeID = wrapper.getAttribute('data-canvas-node-wrap');
      var node = graph.nodesByID[nodeID];
      var navigate = callbacks.navigateNode;
      if (typeof navigate === 'function') navigate(node, event);
    }

    function handleKeyDown(event) {
      if (event.key !== 'Escape') return;
      dispatch({ type: 'ESCAPE' });
      if (typeof callbacks.interactionCleared === 'function') callbacks.interactionCleared();
    }

    canvas.addEventListener('pointerover', handlePointerOver);
    canvas.addEventListener('pointerout', handlePointerOut);
    canvas.addEventListener('focusin', handleFocusIn);
    canvas.addEventListener('focusout', handleFocusOut);
    canvas.addEventListener('click', handleClick);
    canvas.addEventListener('keydown', handleKeyDown);

    if (typeof global.ResizeObserver === 'function') {
      resizeObserver = new global.ResizeObserver(function () { scheduleGeometryBuild(false); });
      resizeObserver.observe(canvas);
      Array.prototype.forEach.call(canvas.querySelectorAll('[data-canvas-node-wrap]'), function (node) {
        resizeObserver.observe(node);
      });
    } else {
      global.addEventListener('resize', scheduleGeometryBuild);
    }

    var controller = {
      host: host,
      canvas: canvas,
      diagnostics: diagnostics,
      rebuildGeometry: function () { scheduleGeometryBuild(true); },
      pinNode: function (nodeID) { dispatch({ type: 'NODE_PIN', nodeID: nodeID }); },
      clearInteraction: function () { dispatch({ type: 'CLEAR_INTERACTION' }); },
      unmount: function () {
        if (destroyed) return;
        destroyed = true;
        if (resizeObserver) resizeObserver.disconnect();
        else global.removeEventListener('resize', scheduleGeometryBuild);
        if (geometryFrame) {
          if (typeof global.cancelAnimationFrame === 'function') global.cancelAnimationFrame(geometryFrame);
          else global.clearTimeout(geometryFrame);
          geometryFrame = 0;
        }
        canvas.removeEventListener('pointerover', handlePointerOver);
        canvas.removeEventListener('pointerout', handlePointerOut);
        canvas.removeEventListener('focusin', handleFocusIn);
        canvas.removeEventListener('focusout', handleFocusOut);
        canvas.removeEventListener('click', handleClick);
        canvas.removeEventListener('keydown', handleKeyDown);
        if (canvas.parentNode === host) host.removeChild(canvas);
        mountedByHost.delete(host);
      }
    };
    mountedByHost.set(host, controller);
    scheduleGeometryBuild(true);
    return controller;
  }

  function unmountSystemCanvas(host) {
    if (!host) return;
    var existing = mountedByHost.get(host);
    if (existing) existing.unmount();
  }

  function canvasDiagnostics(host) {
    var existing = mountedByHost.get(host);
    return existing ? existing.diagnostics : null;
  }

  global.RepomapSystemCanvasRenderer = Object.freeze({
    mountSystemCanvas: mountSystemCanvas,
    unmountSystemCanvas: unmountSystemCanvas,
    diagnostics: canvasDiagnostics
  });
})(globalThis);
