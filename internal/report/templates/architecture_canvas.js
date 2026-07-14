(function (global) {
 "use strict";

 const SVG_NS = "http://www.w3.org/2000/svg";
 const HASH_KEYS = ["flow", "component", "surface", "step", "edge"];
 const COMPONENT_WIDTH = 268;
 const COMPONENT_HEADER_HEIGHT = 82;
 const CHIP_HEIGHT = 27;
 const CHIP_GAP = 7;
 const CHIP_COLUMNS = 2;
 const MAX_VISIBLE_CHIPS = 4;
 const COMPONENT_HEIGHT = COMPONENT_HEADER_HEIGHT + 38 + 2 * (CHIP_HEIGHT + CHIP_GAP);
 const BRANCH_PRIORITY = ["main", "task", "shared"];
 const SEMANTIC_PRIORITY = ["is-join", "is-start", "is-callback", "is-cancel", "is-frontier", "is-call"];
 const GROUP_PADDING = 26;
 const GROUP_HEADER = 50;
 const UNASSIGNED_ID = "__repomap_unassigned__";
 const MIN_SCALE = 0.18;
 const MAX_SCALE = 2.4;
 const WHEEL_ZOOM_SENSITIVITY = 0.0015;
 const MAX_WHEEL_DELTA = 120;
 const INITIAL_MIN_SCALE = 0.72;
 const INITIAL_MAX_SCALE = 1;
 const FIT_MIN_SCALE = 0.65;
 const FOCUS_MIN_SCALE = 0.88;
 const FOCUS_MAX_SCALE = 1.05;
 const LANDSCAPE_COMPONENT_HEIGHT = 108;
 const LANDSCAPE_COMPONENT_GAP = 16;
 const LANDSCAPE_GROUP_GAP = 32;
 const LANDSCAPE_MARGIN = 28;
 const SINGLETON_GROUP_HEIGHT = 132;
 const LIFECYCLE_FALLBACK_LIMIT = 6;

 function array(value) {
  return Array.isArray(value) ? value : [];
 }

 function text(value) {
  return value == null ? "" : String(value);
 }

 function architectureSourceLabel(value) {
  switch (text(value)) {
   case "validated_model": return "validated model";
   case "normalized_model": return "normalized model";
   case "local_anchors": return "local anchors";
   case "package_fallback": return "package fallback";
   default: return "unspecified";
  }
 }

 function element(tag, className, content) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (content != null) node.textContent = text(content);
  return node;
 }

 function svgElement(tag, attributes) {
  const node = document.createElementNS(SVG_NS, tag);
  Object.keys(attributes || {}).forEach((key) => {
   node.setAttribute(key, text(attributes[key]));
  });
  return node;
 }

 function setSVGVisible(node, visible) {
  node.style.display = visible ? "" : "none";
  node.setAttribute("aria-hidden", visible ? "false" : "true");
  node.setAttribute("tabindex", visible ? "0" : "-1");
 }

 function clamp(value, low, high) {
  return Math.min(high, Math.max(low, value));
 }

 function landscapeLayoutMode(projection) {
  if (!projection || !projection.primaryRegion) return "board";
  if (projection.primaryRegion.groupIDs.length === projection.groups.length) return "graph";
  return "hybrid";
 }

 function boardProfileForWidth(value) {
  const width = Math.max(320, Number(value || 1200));
  if (width >= 1160) return { columns: 4, groupWidth: 300 };
  if (width >= 960) return { columns: 3, groupWidth: 320 };
  if (width >= 720) return { columns: 2, groupWidth: 340 };
  return { columns: 1, groupWidth: Math.min(340, width - 40) };
 }

 function shortestColumnIndex(heights, preference) {
  let column = preference[0];
  preference.forEach((candidate) => {
   if (heights[candidate] < heights[column]) column = candidate;
  });
  return column;
 }

 function childGridColumns(childCount, boardColumns) {
  if (childCount <= 1 || boardColumns <= 1) return 1;
  if (childCount >= 7 && boardColumns >= 5) return 3;
  return 2;
 }

 function childGridShape(childCount, boardColumns) {
  const columns = childGridColumns(childCount, boardColumns);
  return {
   columns: columns,
   span: Math.max(1, Math.min(columns, boardColumns)),
   singleton: childCount === 1,
  };
 }

 function diagnosticSubsystemIDs(subsystems, diagnostics) {
  const ids = new Set();
  array(subsystems).forEach((subsystem) => {
   const category = text(subsystem && (subsystem.category || subsystem.classification || subsystem.role)).toLowerCase();
   if (["unresolved", "unassigned", "partial", "diagnostic"].indexOf(category) >= 0) {
    ids.add(text(subsystem.id));
   }
  });
  const hasPreservedRemainder = array(diagnostics).some((diagnostic) =>
   text(diagnostic && diagnostic.code) === "proposal.omitted_members_preserved"
  );
  if (hasPreservedRemainder && array(subsystems).length > 0) {
   ids.add(text(subsystems[subsystems.length - 1].id));
  }
  return Array.from(ids).sort();
 }

 function shortestCompatiblePlacement(heights, span) {
  const boundedSpan = Math.max(1, Math.min(span, heights.length));
  const candidates = [];
  for (let column = 0; column <= heights.length - boundedSpan; column++) {
   if (boundedSpan > 1 && column % boundedSpan !== 0) continue;
   const y = Math.max.apply(null, heights.slice(column, column + boundedSpan));
   candidates.push({ column: column, y: y });
  }
  candidates.sort((left, right) => left.y - right.y || left.column - right.column);
  return candidates[0];
 }

 function scaleToBounds(bounds, viewport, padding) {
  if (!bounds || !viewport) return 1;
  return Math.min(
   (viewport.width - padding * 2) / Math.max(1, bounds.width),
   (viewport.height - padding * 2) / Math.max(1, bounds.height)
  );
 }

 function centeredTransform(bounds, viewport, scale) {
  return {
   x: (viewport.width - bounds.width * scale) / 2 - bounds.x * scale,
   y: (viewport.height - bounds.height * scale) / 2 - bounds.y * scale,
   scale: scale,
  };
 }

 function readableFitScale(bounds, viewport, padding) {
  return clamp(scaleToBounds(bounds, viewport, padding), FIT_MIN_SCALE, 1.35);
 }

 function componentFocusScale(bounds, viewport, padding) {
  return clamp(scaleToBounds(bounds, viewport, padding), FOCUS_MIN_SCALE, FOCUS_MAX_SCALE);
 }

 function memberLabel(memberID) {
  if (!memberID) return "unknown member";
  if (typeof memberID === "string") return memberID;
  return [text(memberID.kind), text(memberID.value)].filter(Boolean).join(":");
 }

 function locationLabel(location) {
  if (!location || !location.path) return "";
  let label = text(location.path);
  if (location.line) label += ":" + location.line;
  if (location.column) label += ":" + location.column;
  return label;
 }

 function branchClass(kind) {
  if (kind === "task") return "is-task";
  if (kind === "main") return "is-main";
  if (kind === "shared") return "is-shared";
  return "is-unassigned";
 }

 function branchLabel(kind) {
  const labels = {
   main: "Main path",
   task: "Background task",
   shared: "Shared lifecycle context",
   unassigned: "Unassigned evidence",
  };
  return labels[text(kind)] || "Unassigned evidence";
 }

 function proofAreaLabel(kind) {
  const labels = {
   trigger: "Trigger",
   entrypoint: "Entrypoint",
   dispatch: "Dispatch",
   application_callable: "Application callable",
   core_operation: "Core operation",
   io_boundary: "I/O boundary",
   concurrency: "Concurrency",
   termination: "Termination",
  };
  return labels[text(kind)] || text(kind).replace(/_/g, " ") || "Unknown proof area";
 }

 function semanticClass(relation, invocation) {
  const value = text(relation).toLowerCase();
  if (value.indexOf("cancel") >= 0) return "is-cancel";
  if (value.indexOf("join") >= 0 || value === "waits_for") return "is-join";
  if (value.indexOf("callback") >= 0 || invocation === "callback") return "is-callback";
  if (value.indexOf("start") >= 0 || invocation === "goroutine") return "is-start";
  return "is-call";
 }

 function savedTraceLabel(archetype) {
  switch (text(archetype)) {
   case "cli": return "Saved CLI trace";
   case "process": return "Saved process trace";
   default: return "Saved trace";
  }
 }

 function lifecycleRelationHeading(edge) {
  const relation = text(edge && edge.relation).toLowerCase();
  const invocation = text(edge && edge.invocation).toLowerCase();
  if (relation.indexOf("cancel") >= 0) return "Cancellation";
  if (relation.indexOf("join") >= 0 || relation === "waits_for") return "Join";
  if (relation.indexOf("callback") >= 0 || invocation === "callback") return "Callback";
  if (relation.indexOf("start") >= 0 || invocation === "goroutine") return "Started by";
  return "";
 }

 function groupLifecycleRelations(flow, edges) {
  const lifecycle = array(edges).filter((edge) => lifecycleRelationHeading(edge));
  const groups = [];
  const assigned = new Set();
  const labelOrder = ["Started by", "Callback", "Cancellation", "Join"];
  array(flow && flow.branches)
   .filter((branch) => text(branch.kind) === "task" && text(branch.root_anchor_id))
   .forEach((branch) => {
    const branchAnchors = new Set(array(branch.anchor_ids).map(text));
    branchAnchors.add(text(branch.root_anchor_id));
    const cancellationAnchors = new Set(branchAnchors);
    let expanded = true;
    while (expanded) {
     expanded = false;
     lifecycle.forEach((edge) => {
      if (lifecycleRelationHeading(edge) !== "Cancellation") return;
      const from = text(edge.from);
      const to = text(edge.to);
      if (!cancellationAnchors.has(from) && !cancellationAnchors.has(to)) return;
      if (!cancellationAnchors.has(from)) {
       cancellationAnchors.add(from);
       expanded = true;
      }
      if (!cancellationAnchors.has(to)) {
       cancellationAnchors.add(to);
       expanded = true;
      }
     });
    }
    const relations = lifecycle.filter((edge) => {
     const from = text(edge.from);
     const to = text(edge.to);
     if (branchAnchors.has(from) || branchAnchors.has(to)) return true;
     return lifecycleRelationHeading(edge) === "Cancellation" &&
      (cancellationAnchors.has(from) || cancellationAnchors.has(to));
    });
    relations.sort((a, b) => {
     const labelDifference = labelOrder.indexOf(lifecycleRelationHeading(a)) -
      labelOrder.indexOf(lifecycleRelationHeading(b));
     return labelDifference || text(a.id).localeCompare(text(b.id));
    });
    if (relations.length === 0) return;
    relations.forEach((edge) => assigned.add(edge));
    groups.push({
     branchID: text(branch.id),
     rootAnchorID: text(branch.root_anchor_id),
     relations: relations,
    });
   });
  const ungrouped = lifecycle.filter((edge) => !assigned.has(edge));
  return {
   groups: groups,
   total: lifecycle.length,
   ungrouped: ungrouped.slice(0, LIFECYCLE_FALLBACK_LIMIT),
   ungroupedTotal: ungrouped.length,
  };
 }

 function relationLabel(relation) {
  const value = text(relation);
  const labels = {
   calls: "calls",
   callback: "invokes callback",
   constructs: "constructs",
   registers: "registers",
   registers_command: "registers",
   starts_goroutine: "starts task",
   uses_cancellation: "uses cancellation context",
   cancels: "invokes cancellation",
   joins: "waits for task",
   waits_for: "waits for",
  };
  return labels[value] || value.replace(/_/g, " ") || "continues";
 }

 function layoutComponentID(id) {
  return "component:" + text(id);
 }

 function layoutSubsystemID(id) {
  return "subsystem:" + text(id);
 }

 function layoutStructuralEdgeID(id) {
  return "structural:" + text(id);
 }

 function layoutFlowEdgeID(flowID, edgeID) {
  return "flow:" + text(flowID) + ":" + text(edgeID);
 }

 function selectionKey(flowID, itemID) {
  return text(flowID) + "\u0000" + text(itemID);
 }

 function flowStepKey(flowID, stepID) {
  return selectionKey(flowID, stepID);
 }

 function pathFromSections(sections, offsetX, offsetY) {
  const parts = [];
  array(sections).forEach((section) => {
   if (!section || !section.startPoint || !section.endPoint) return;
   const points = [section.startPoint]
    .concat(array(section.bendPoints))
    .concat([section.endPoint]);
   if (points.length < 2) return;
   parts.push(
    points
     .map((point, index) =>
      (index === 0 ? "M" : "L") +
      (Number(point.x || 0) + offsetX) +
      " " +
      (Number(point.y || 0) + offsetY)
     )
     .join(" ")
   );
  });
  return parts.join(" ");
 }

 class ArchitectureCanvasApp {
  constructor(host, data, options) {
   if (!(host instanceof Element)) {
    throw new TypeError("RepomapArchitectureCanvas.mount requires a host Element");
   }
   this.host = host;
   this.data = data && typeof data === "object" ? data : {};
   this.options = options && typeof options === "object" ? options : {};
   this.destroyed = false;
   this.layoutStarted = false;
    this.layoutResult = null;
    this.landscapeProjection = null;
    this.landscapeLayoutMode = "graph";
   this.events = new AbortController();
   this.view = { x: 0, y: 0, scale: 1 };
   this.drag = null;
    this.selection = {
     flow: "",
     component: "",
     surface: "",
     step: "",
     edge: "",
    };

   this.subsystems = array(this.data.subsystems);
   this.components = array(this.data.components);
   this.structuralEdges = array(this.data.structural_edges);
     this.flows = array(this.data.flows);
     this.surfaces = array(this.data.surfaces);
     this.suggestions = array(this.data.suggested_investigations);
    this.candidateDirections = array(this.options.candidateDirections);
   this.flowEdges = array(this.data.flow_edges);
   this.frontiers = array(this.data.frontiers);
   this.diagnostics = array(this.data.diagnostics);

    this.componentByID = new Map();
    this.anchorByID = new Map();
   this.subsystemByID = new Map();
   this.structuralEdgeByID = new Map();
    this.flowByID = new Map();
    this.surfaceByID = new Map();
     this.directionByID = new Map();
     this.suggestionByID = new Map();
   this.flowEdgesByKey = new Map();
   this.flowStepsByKey = new Map();
   this.flowBranchesByKey = new Map();
   this.nodePositions = new Map();
   this.groupPositions = new Map();
   this.edgeRoutes = new Map();
   this.stepGeometry = new Map();
   this.componentElements = new Map();
   this.stepElements = new Map();
   this.frontierElements = new Map();
   this.structuralEdgeElements = new Map();
   this.flowEdgeElements = new Map();
     this.flowComponentIDs = new Map();
     this.flowButtons = new Map();
      this.focusFlowID = "";
      this.landscapeView = null;
      this.landscapeComponentID = "";
      this.returnHighlightIDs = new Set();
     this.diagnosticComponentIDs = new Set();
     this.singletonGroupIDs = new Set();

   this.indexData();
   this.buildShell();
   this.restoreHash(false);
  }

  indexData() {
    array(this.data.behavior_anchors).forEach((anchor) => {
     if (anchor && anchor.id) this.anchorByID.set(text(anchor.id), anchor);
    });
   this.subsystems.forEach((subsystem) => {
    if (subsystem && subsystem.id) this.subsystemByID.set(text(subsystem.id), subsystem);
   });
   this.components.forEach((component) => {
    if (component && component.id) this.componentByID.set(text(component.id), component);
   });
    this.structuralEdges.forEach((edge) => {
     if (edge && edge.id) this.structuralEdgeByID.set(text(edge.id), edge);
    });
    this.surfaces.forEach((surface) => {
     if (surface && surface.id) this.surfaceByID.set(text(surface.id), surface);
    });
     this.candidateDirections.forEach((direction) => {
      if (direction && direction.id) this.directionByID.set(text(direction.id), direction);
     });
     this.suggestions.forEach((suggestion) => {
      if (suggestion && suggestion.id) this.suggestionByID.set(text(suggestion.id), suggestion);
     });

      this.flows.forEach((flow) => {
    if (!flow || !flow.id) return;
    const flowID = text(flow.id);
    this.flowByID.set(flowID, flow);
    const componentIDs = new Set();
    array(flow.branches).forEach((branch) => {
     if (branch && branch.id) {
      this.flowBranchesByKey.set(selectionKey(flowID, branch.id), branch);
     }
    });
    array(flow.steps).forEach((step) => {
     if (!step || !step.id) return;
     this.flowStepsByKey.set(flowStepKey(flowID, step.id), step);
     if (step.component_id) componentIDs.add(text(step.component_id));
    });
    this.flowComponentIDs.set(flowID, componentIDs);
   });

   this.flowEdges.forEach((edge) => {
    if (!edge || !edge.id || !edge.flow_id) return;
    this.flowEdgesByKey.set(selectionKey(edge.flow_id, edge.id), edge);
   });
  }

  buildShell() {
   this.host.replaceChildren();
   this.root = element("section", "rm-arch");

   const toolbar = element("div", "rm-arch__toolbar");
    const flowNav = element("nav", "rm-arch__flows");
     this.landscapeButton = element("button", "rm-arch__flow-button is-active", "Architecture");
    this.landscapeButton.type = "button";
    this.listen(this.landscapeButton, "click", () => {
     this.backToArchitecture();
    });
    flowNav.appendChild(this.landscapeButton);
    if (this.flows.length > 0) {
     this.traceMenu = element("div", "rm-arch__trace-menu");
     this.traceMenuSummary = element("button", "rm-arch__flow-button", "Saved traces (" + this.flows.length + ")");
     this.traceMenuSummary.type = "button";
     this.traceMenuSummary.setAttribute("aria-haspopup", "menu");
     this.traceMenuSummary.setAttribute("aria-expanded", "false");
     this.traceMenu.appendChild(this.traceMenuSummary);
     this.listen(this.traceMenuSummary, "click", (event) => {
      event.stopPropagation();
      this.toggleTraceMenu(this.traceList.hidden);
     });
     this.traceList = element("div", "rm-arch__trace-list");
     this.traceList.setAttribute("role", "menu");
     this.traceList.hidden = true;
     this.flows.forEach((flow) => {
       const button = element("button", "rm-arch__trace-option");
      button.type = "button";
       button.setAttribute("role", "menuitem");
       button.appendChild(element("strong", null, flow.name || flow.id));
       if (flow.why_inspect) button.appendChild(element("small", "rm-arch__trace-purpose", flow.why_inspect));
        const startSurface = this.surfaceByID.get(text(flow.start_surface_id));
        const originName = startSurface ? startSurface.name || startSurface.kind : flow.trigger || flow.command;
        const origin = [savedTraceLabel(flow.archetype), originName].filter(Boolean).join(" · ");
       button.appendChild(element(
        "span",
        null,
        [origin, flow.status]
         .filter(Boolean)
         .join(" · ")
       ));
       button.appendChild(element("span", null, array(flow.participating_component_ids).length + " components"));
      if (flow.status !== "complete" && flow.frontier_summary) {
       button.appendChild(element("small", null, "Frontier: " + flow.frontier_summary));
      }
      this.listen(button, "click", () => {
       this.toggleTraceMenu(false);
       this.openTrace(flow.id);
      });
      this.flowButtons.set(text(flow.id), button);
      this.traceList.appendChild(button);
     });
     flowNav.appendChild(this.traceMenu);
    }
   toolbar.appendChild(flowNav);

    const controls = element("div", "rm-arch__controls");
    const diagnosticComponents = this.diagnosticComponents();
    if (diagnosticComponents.length > 0) {
     diagnosticComponents.forEach((component) => this.diagnosticComponentIDs.add(text(component.id)));
     this.diagnosticButton = this.controlButton(
      "Unassigned evidence · " + diagnosticComponents.length,
      "Inspect exact evidence outside conceptual architecture",
      () => this.setSelection({
       flow: "", component: text(diagnosticComponents[0].id), step: "", edge: "",
      }, true)
     );
     this.diagnosticButton.classList.add("rm-arch__diagnostic-control");
     controls.appendChild(this.diagnosticButton);
    }
    this.zoomOutButton = this.controlButton("−", "Zoom out", () => this.zoomBy(0.82));
      this.fitButton = this.controlButton("Fit", "Fit architecture at readable scale", () => this.fit());
   this.zoomInButton = this.controlButton("+", "Zoom in", () => this.zoomBy(1.22));
   controls.append(this.zoomOutButton, this.fitButton, this.zoomInButton);
    toolbar.appendChild(controls);
    this.root.appendChild(toolbar);
    if (this.traceList) this.root.appendChild(this.traceList);

   const workspace = element("div", "rm-arch__workspace");
    this.viewport = element("div", "rm-arch__viewport");
    this.loading = element("div", "rm-arch__loading", "Laying out the saved architecture…");
     this.flowFocus = element("div", "rm-arch__flow-focus");
     this.flowFocus.hidden = true;
     this.viewportHint = element("div", "rm-arch__viewport-hint", "Drag to explore the architecture");
     this.viewport.append(this.loading, this.flowFocus, this.viewportHint);
   workspace.appendChild(this.viewport);

    this.drawerBackdrop = element("button", "rm-arch__drawer-backdrop");
    this.drawerBackdrop.type = "button";
     this.drawerBackdrop.setAttribute("aria-label", "Close inspector");
     this.drawerBackdrop.hidden = true;
     this.inspector = element("aside", "rm-arch__inspector");
    this.inspector.setAttribute("aria-label", "Architecture inspector");
    workspace.appendChild(this.inspector);
    this.root.append(workspace, this.drawerBackdrop);

   this.host.appendChild(this.root);
   this.installViewportInteractions();
    this.listen(global, "hashchange", () => this.restoreHash(true));
    this.listen(global, "keydown", (event) => {
     if (event.key !== "Escape") return;
     if (this.traceList && !this.traceList.hidden) {
      this.toggleTraceMenu(false);
      this.traceMenuSummary.focus();
      return;
     }
     if (this.hasInspectorSelection(this.selection)) this.closeInspector();
    });
    this.listen(global, "resize", () => {
     if (this.traceList && !this.traceList.hidden) this.positionTraceMenu();
    });
    this.listen(global.document, "click", (event) => {
     if (!this.hasInspectorSelection(this.selection)) return;
     const target = event.target;
     if (!target || this.inspector.contains(target)) return;
     if (typeof target.closest === "function" && target.closest(".rm-arch__component-card")) return;
     this.closeInspector();
    }, { capture: true });
    this.listen(global.document, "click", (event) => {
     if (!this.traceList || this.traceList.hidden) return;
     const target = event.target;
     if (target && (this.traceMenu.contains(target) || this.traceList.contains(target))) return;
     this.toggleTraceMenu(false);
    });
  }

  toggleTraceMenu(open) {
   if (!this.traceList || !this.traceMenuSummary) return;
   this.traceList.hidden = !open;
   this.traceMenuSummary.setAttribute("aria-expanded", open ? "true" : "false");
   this.traceMenuSummary.classList.toggle("is-active", open);
   if (open) requestAnimationFrame(() => this.positionTraceMenu());
  }

  positionTraceMenu() {
   if (!this.traceList || !this.traceMenuSummary || this.traceList.hidden) return;
   const trigger = this.traceMenuSummary.getBoundingClientRect();
   const margin = 12;
   const width = Math.min(420, Math.max(280, global.innerWidth - margin * 2));
   const left = Math.min(Math.max(margin, trigger.left), Math.max(margin, global.innerWidth - width - margin));
   this.traceList.style.left = left + "px";
   this.traceList.style.top = Math.min(trigger.bottom + 7, global.innerHeight - margin) + "px";
   this.traceList.style.width = width + "px";
  }

  controlButton(label, title, handler) {
   const button = element("button", "rm-arch__control", label);
   button.type = "button";
   button.title = title;
   button.setAttribute("aria-label", title);
   this.listen(button, "click", handler);
   return button;
  }

   isDiagnosticSubsystem(subsystem) {
    const id = text(subsystem && subsystem.id);
    return diagnosticSubsystemIDs(this.subsystems, this.diagnostics).indexOf(id) >= 0;
   }

   diagnosticComponents() {
    const componentIDs = new Set();
    this.subsystems.forEach((subsystem) => {
     if (!this.isDiagnosticSubsystem(subsystem)) return;
     array(subsystem.component_ids).forEach((id) => componentIDs.add(text(id)));
    });
    return this.components.filter((component) => componentIDs.has(text(component.id)));
   }

  listen(target, eventName, handler, options) {
   const settings = Object.assign({}, options || {}, { signal: this.events.signal });
   target.addEventListener(eventName, handler, settings);
  }

  start() {
   if (this.layoutStarted) return this.ready;
   this.layoutStarted = true;
   this.ready = this.layoutOnce()
    .then((layout) => {
     if (this.destroyed) return;
      this.layoutResult = layout;
      this.renderPersistentScene();
      this.renderSelection();
       requestAnimationFrame(() => {
        if (this.selection.component && !this.selection.flow) this.focusComponent(this.selection.component, false);
        else this.focusInitialLandscape();
       });
    })
    .catch((error) => {
     if (this.destroyed) return;
     this.loading.textContent = error && error.message ? error.message : text(error);
     this.loading.classList.add("is-error");
     this.renderInspector();
    });
   return this.ready;
  }

   layoutOnce() {
    const ELKConstructor = global.ELK;
   if (typeof ELKConstructor !== "function") {
    return Promise.reject(new Error("Architecture renderer is unavailable."));
   }
    this.landscapeProjection = this.projectLandscapeGraph();
    this.landscapeLayoutMode = this.chooseLandscapeLayoutMode(this.landscapeProjection);
    this.root.dataset.layoutMode = this.landscapeLayoutMode;
    if (this.landscapeLayoutMode === "board") {
     return Promise.resolve(this.layoutArchitectureBoard(this.landscapeProjection));
    }
    const elk = new ELKConstructor();
    return elk.layout(this.buildGroupELKGraph(this.landscapeProjection)).then((layout) =>
     this.composeGraphLandscape(layout, this.landscapeProjection)
    );
   }

   projectLandscapeGraph() {
    const componentIDsInSubsystems = new Set();
    const componentOwner = new Map();
    const allGroups = [];
    this.subsystems.forEach((subsystem) => {
     const id = text(subsystem.id);
     const componentIDs = array(subsystem.component_ids)
      .map(text)
      .filter((componentID) => this.componentByID.has(componentID));
     if (componentIDs.length === 0) return;
     componentIDs.forEach((componentID) => {
      componentIDsInSubsystems.add(componentID);
      componentOwner.set(componentID, id);
     });
     allGroups.push({
      id: id,
      subsystem: subsystem,
      componentIDs: componentIDs,
      diagnostic: this.isDiagnosticSubsystem(subsystem),
     });
    });

    const ungrouped = this.components
     .map((component) => text(component.id))
     .filter((id) => !componentIDsInSubsystems.has(id));
    if (ungrouped.length > 0) {
     ungrouped.forEach((componentID) => componentOwner.set(componentID, "__ungrouped__"));
     allGroups.push({ id: "__ungrouped__", subsystem: null, componentIDs: ungrouped, diagnostic: true });
    }

    const diagnosticGroups = allGroups.filter((group) => group.diagnostic);
    const groups = allGroups.filter((group) => !group.diagnostic);
    const architectureGroupIDs = new Set(groups.map((group) => group.id));
    const architectureComponentIDs = new Set();
    groups.forEach((group) => group.componentIDs.forEach((id) => architectureComponentIDs.add(id)));
    const adjacency = new Map(groups.map((group) => [group.id, new Set()]));
    const componentAdjacency = new Map(Array.from(architectureComponentIDs).map((id) => [id, new Set()]));
    const pairKeys = new Set();
    const componentPairKeys = new Set();
    const edges = [];
    this.structuralEdges.forEach((edge) => {
     const fromComponentID = text(edge.from_component_id);
     const toComponentID = text(edge.to_component_id);
     if (componentAdjacency.has(fromComponentID) && componentAdjacency.has(toComponentID) && fromComponentID !== toComponentID) {
      const componentPairKey = fromComponentID + "\u0000" + toComponentID;
      if (!componentPairKeys.has(componentPairKey)) {
       componentPairKeys.add(componentPairKey);
       componentAdjacency.get(fromComponentID).add(toComponentID);
       componentAdjacency.get(toComponentID).add(fromComponentID);
      }
     }
     const fromGroupID = componentOwner.get(fromComponentID);
     const toGroupID = componentOwner.get(toComponentID);
     if (!architectureGroupIDs.has(fromGroupID) || !architectureGroupIDs.has(toGroupID) || fromGroupID === toGroupID) return;
     const pairKey = fromGroupID + "\u0000" + toGroupID;
     if (pairKeys.has(pairKey)) return;
     pairKeys.add(pairKey);
     edges.push({
      id: pairKey,
      from: fromGroupID,
      to: toGroupID,
     });
     adjacency.get(fromGroupID).add(toGroupID);
     adjacency.get(toGroupID).add(fromGroupID);
    });

    const unseen = new Set(groups.map((group) => group.id));
    const regions = [];
    while (unseen.size > 0) {
     const start = Array.from(unseen).sort()[0];
     const queue = [start];
     const groupIDs = [];
     unseen.delete(start);
     while (queue.length > 0) {
      const current = queue.shift();
      groupIDs.push(current);
      Array.from(adjacency.get(current) || []).sort().forEach((next) => {
       if (!unseen.has(next)) return;
       unseen.delete(next);
       queue.push(next);
      });
     }
     const groupSet = new Set(groupIDs);
     regions.push({
      groupIDs: groupIDs.sort(),
      edgeCount: edges.filter((edge) => groupSet.has(edge.from) && groupSet.has(edge.to)).length,
     });
    }
    regions.sort((left, right) =>
     right.groupIDs.length - left.groupIDs.length ||
     right.edgeCount - left.edgeCount ||
     left.groupIDs[0].localeCompare(right.groupIDs[0])
    );
    const primaryRegion = regions.find((region) =>
     region.groupIDs.length >= 3 && region.edgeCount >= region.groupIDs.length - 1
    ) || null;
    return {
     groups: groups,
     diagnosticGroups: diagnosticGroups,
     edges: edges,
     adjacency: adjacency,
     componentAdjacency: componentAdjacency,
     regions: regions,
     primaryRegion: primaryRegion,
    };
   }

   chooseLandscapeLayoutMode(projection) {
    return landscapeLayoutMode(projection);
   }

   boardProfile() {
    return boardProfileForWidth(this.viewport && this.viewport.clientWidth);
   }

   numericPriority(value) {
    const number = Number(value);
    return Number.isFinite(number) ? number : 0;
   }

   semanticCategory(record, fallback) {
    const value = text(record && (record.category || record.classification || record.role)).toLowerCase();
    if (["application", "primary", "main"].indexOf(value) >= 0) return "primary";
    if (["external", "integration", "boundary"].indexOf(value) >= 0) return "external";
    if (["analysis", "tooling", "support"].indexOf(value) >= 0) return "support";
    if (["unresolved", "unassigned", "partial", "diagnostic"].indexOf(value) >= 0) return "diagnostic";
    return fallback || "neutral";
   }

   semanticCategoryLabel(category) {
    const labels = {
     primary: "Primary",
     external: "Boundary",
     support: "Supporting",
     diagnostic: "Unresolved",
    };
    return labels[category] || "";
   }

   compareLandscapeGroups(projection, left, right) {
    const leftSubsystem = left.subsystem || {};
    const rightSubsystem = right.subsystem || {};
    const importance = this.numericPriority(rightSubsystem.importance || rightSubsystem.rank) -
     this.numericPriority(leftSubsystem.importance || leftSubsystem.rank);
    if (importance !== 0) return importance;
    const participation = this.groupFlowParticipation(right) - this.groupFlowParticipation(left);
    if (participation !== 0) return participation;
    const degree = (projection.adjacency.get(right.id) || new Set()).size -
     (projection.adjacency.get(left.id) || new Set()).size;
    if (degree !== 0) return degree;
    if (right.componentIDs.length !== left.componentIDs.length) {
     return right.componentIDs.length - left.componentIDs.length;
    }
    return left.id.localeCompare(right.id);
   }

   groupFlowParticipation(group) {
    const flowIDs = new Set();
    group.componentIDs.forEach((id) => {
     const component = this.componentByID.get(id);
     array(component && component.participating_flow_ids).forEach((flowID) => flowIDs.add(text(flowID)));
    });
    return flowIDs.size;
   }

   orderedLandscapeGroups(projection) {
    const groupByID = new Map(projection.groups.map((group) => [group.id, group]));
    const ordered = [];
    const seen = new Set();
    const appendRegion = (region) => {
     if (!region) return;
     const remaining = new Set(region.groupIDs);
     const candidates = region.groupIDs.map((id) => groupByID.get(id)).filter(Boolean);
     candidates.sort((left, right) => this.compareLandscapeGroups(projection, left, right));
     const queue = candidates.length > 0 ? [candidates[0].id] : [];
     while (queue.length > 0) {
      const id = queue.shift();
      if (!remaining.has(id)) continue;
      remaining.delete(id);
      seen.add(id);
      ordered.push(groupByID.get(id));
      const neighbors = Array.from(projection.adjacency.get(id) || [])
       .filter((neighborID) => remaining.has(neighborID))
       .map((neighborID) => groupByID.get(neighborID))
       .filter(Boolean)
       .sort((left, right) => this.compareLandscapeGroups(projection, left, right));
      neighbors.forEach((group) => queue.push(group.id));
     }
     Array.from(remaining).sort().forEach((id) => {
      seen.add(id);
      ordered.push(groupByID.get(id));
     });
    };
    appendRegion(projection.primaryRegion);
    projection.regions.forEach((region) => {
     if (region !== projection.primaryRegion) appendRegion(region);
    });
    projection.groups
     .filter((group) => !seen.has(group.id))
     .sort((left, right) => this.compareLandscapeGroups(projection, left, right))
     .forEach((group) => ordered.push(group));
    return ordered;
   }

   orderedBoardGroups(projection) {
    return projection.groups.slice().sort((left, right) => this.compareLandscapeGroups(projection, left, right));
   }

   orderedGroupComponents(group, projection) {
    return group.componentIDs.slice().sort((leftID, rightID) => {
     const left = this.componentByID.get(leftID) || {};
     const right = this.componentByID.get(rightID) || {};
     const importance = this.numericPriority(right.importance || right.rank) -
      this.numericPriority(left.importance || left.rank);
     if (importance !== 0) return importance;
     const participation = array(right.participating_flow_ids).length - array(left.participating_flow_ids).length;
     if (participation !== 0) return participation;
     const degree = (projection.componentAdjacency.get(rightID) || new Set()).size -
      (projection.componentAdjacency.get(leftID) || new Set()).size;
     if (degree !== 0) return degree;
     const size = array(right.members).length - array(left.members).length;
     if (size !== 0) return size;
     return leftID.localeCompare(rightID);
    });
   }

   groupMetrics(group, width, childColumns) {
     const columns = Math.max(1, Math.min(childColumns || 1, group.componentIDs.length));
     if (group.componentIDs.length === 1) {
      return {
       width: width,
       height: SINGLETON_GROUP_HEIGHT,
       columns: 1,
       componentWidth: width,
       singleton: true,
      };
     }
     const componentWidth = (width - GROUP_PADDING * 2 - LANDSCAPE_COMPONENT_GAP * (columns - 1)) / columns;
    const rows = Math.ceil(group.componentIDs.length / columns);
    return {
     width: width,
     height: GROUP_HEADER + GROUP_PADDING + rows * LANDSCAPE_COMPONENT_HEIGHT +
      Math.max(0, rows - 1) * LANDSCAPE_COMPONENT_GAP,
      columns: columns,
      componentWidth: componentWidth,
      singleton: false,
     };
   }

   boardGroupMetrics(group, profile) {
    const shape = childGridShape(group.componentIDs.length, profile.columns);
    const columns = shape.columns;
    const span = shape.span;
    const width = span * profile.groupWidth + Math.max(0, span - 1) * LANDSCAPE_GROUP_GAP;
    const metrics = this.groupMetrics(group, width, columns);
    metrics.span = span;
    return metrics;
   }

   placeLandscapeGroup(group, metrics, x, y, projection) {
    this.groupPositions.set(group.id, {
     x: x,
     y: y,
     width: metrics.width,
     height: metrics.height,
    });
    if (metrics.singleton) this.singletonGroupIDs.add(group.id);
    this.orderedGroupComponents(group, projection).forEach((componentID, index) => {
     if (metrics.singleton) {
      this.nodePositions.set(componentID, {
       x: x,
       y: y,
       width: metrics.width,
       height: metrics.height,
      });
      return;
     }
     const column = index % metrics.columns;
     const row = Math.floor(index / metrics.columns);
     this.nodePositions.set(componentID, {
      x: x + GROUP_PADDING + column * (metrics.componentWidth + LANDSCAPE_COMPONENT_GAP),
      y: y + GROUP_HEADER + row * (LANDSCAPE_COMPONENT_HEIGHT + LANDSCAPE_COMPONENT_GAP),
      width: metrics.componentWidth,
      height: LANDSCAPE_COMPONENT_HEIGHT,
     });
    });
   }

   centeredColumnOrder(columns) {
    const center = (columns - 1) / 2;
    return Array.from({ length: columns }, (_, index) => index).sort((left, right) =>
     Math.abs(left - center) - Math.abs(right - center) || left - right
    );
   }

   packLandscapeGroups(groups, projection, profile, startY) {
     const heights = Array(profile.columns).fill(startY);
     groups.forEach((group) => {
      const metrics = this.boardGroupMetrics(group, profile);
      const placement = shortestCompatiblePlacement(heights, metrics.span);
      const x = LANDSCAPE_MARGIN + placement.column * (profile.groupWidth + LANDSCAPE_GROUP_GAP);
      const y = placement.y;
      this.placeLandscapeGroup(group, metrics, x, y, projection);
      const nextHeight = y + metrics.height + LANDSCAPE_GROUP_GAP;
      for (let index = placement.column; index < placement.column + metrics.span; index++) {
       heights[index] = nextHeight;
      }
     });
    return heights;
   }

   layoutArchitectureBoard(projection) {
    this.nodePositions.clear();
     this.groupPositions.clear();
     this.edgeRoutes.clear();
     this.singletonGroupIDs.clear();
    const profile = this.boardProfile();
     const groups = this.orderedBoardGroups(projection);
    const heights = this.packLandscapeGroups(groups, projection, profile, LANDSCAPE_MARGIN);
     this.primaryGroupIDs = new Set(projection.primaryRegion ? projection.primaryRegion.groupIDs : []);
     this.initialGroupIDs = new Set(groups.slice(0, Math.min(4, groups.length)).map((group) => group.id));
    this.buildLandscapeEdgeRoutes();
    return {
     width: LANDSCAPE_MARGIN * 2 + profile.columns * profile.groupWidth +
      Math.max(0, profile.columns - 1) * LANDSCAPE_GROUP_GAP,
     height: Math.max(LANDSCAPE_MARGIN * 2, Math.max.apply(null, heights) - LANDSCAPE_GROUP_GAP + LANDSCAPE_MARGIN),
    };
   }

   buildGroupELKGraph(projection) {
    const primaryIDs = new Set(projection.primaryRegion ? projection.primaryRegion.groupIDs : []);
    const groups = projection.groups.filter((group) => primaryIDs.has(group.id));
    const children = groups.map((group) => {
      const columns = childGridColumns(group.componentIDs.length, 4);
      const width = columns * COMPONENT_WIDTH + GROUP_PADDING * 2 +
       Math.max(0, columns - 1) * LANDSCAPE_COMPONENT_GAP;
     const metrics = this.groupMetrics(group, width, columns);
     return { id: layoutSubsystemID(group.id), width: metrics.width, height: metrics.height };
    });
    const edges = projection.edges
     .filter((edge) => primaryIDs.has(edge.from) && primaryIDs.has(edge.to))
     .map((edge) => ({
      id: "group-edge:" + edge.id,
      sources: [layoutSubsystemID(edge.from)],
      targets: [layoutSubsystemID(edge.to)],
     }));
    return {
     id: "repomap-architecture-core",
     layoutOptions: {
      "elk.algorithm": "layered",
      "elk.direction": "RIGHT",
      "elk.edgeRouting": "ORTHOGONAL",
      "elk.spacing.nodeNode": "48",
      "elk.layered.spacing.nodeNodeBetweenLayers": "72",
      "elk.spacing.edgeNode": "24",
      "elk.spacing.edgeEdge": "14",
      "elk.layered.considerModelOrder.strategy": "NODES_AND_EDGES",
      "elk.layered.nodePlacement.strategy": "NETWORK_SIMPLEX",
      "elk.padding": "[top=28,left=28,bottom=28,right=28]",
    },
    children: children,
    edges: edges,
    };
   }

   graphLayoutIsReadable(layout) {
    const width = Number(layout.width || 0);
    const height = Number(layout.height || 0);
    if (width <= 0 || height <= 0) return false;
    const aspect = width / height;
    const viewportWidth = Math.max(1, Number(this.viewport.clientWidth || 1));
    return aspect >= 1.05 && aspect <= 2.8 && width <= viewportWidth * 1.8;
   }

  flowStepOwner(flowID, stepID) {
   const step = this.flowStepsByKey.get(flowStepKey(flowID, stepID));
   if (!step) return "";
   const componentID = text(step.component_id);
   return componentID && this.componentByID.has(componentID) ? componentID : UNASSIGNED_ID;
  }

   composeGraphLandscape(layout, projection) {
    if (!this.graphLayoutIsReadable(layout)) {
     this.root.dataset.layoutMode = this.landscapeLayoutMode + "-board";
     return this.layoutArchitectureBoard(projection);
    }
     this.nodePositions.clear();
     this.groupPositions.clear();
     this.edgeRoutes.clear();
     this.singletonGroupIDs.clear();
    const groupByID = new Map(projection.groups.map((group) => [group.id, group]));
    array(layout.children).forEach((node) => {
     const id = text(node.id).replace(/^subsystem:/, "");
     const group = groupByID.get(id);
     if (!group) return;
      const columns = childGridColumns(group.componentIDs.length, 4);
     const metrics = this.groupMetrics(group, Number(node.width || 320), columns);
     this.placeLandscapeGroup(group, metrics, Number(node.x || 0), Number(node.y || 0), projection);
    });
    const primaryIDs = new Set(projection.primaryRegion ? projection.primaryRegion.groupIDs : []);
    const remaining = this.orderedLandscapeGroups(projection).filter((group) => !primaryIDs.has(group.id));
    let width = Math.max(1, Number(layout.width || 1));
    let height = Math.max(1, Number(layout.height || 1));
     if (remaining.length > 0) {
     const profile = this.boardProfile();
     const heights = this.packLandscapeGroups(remaining, projection, profile, height + LANDSCAPE_GROUP_GAP);
     width = Math.max(width, LANDSCAPE_MARGIN * 2 + profile.columns * profile.groupWidth +
      Math.max(0, profile.columns - 1) * LANDSCAPE_GROUP_GAP);
      height = Math.max.apply(null, heights) - LANDSCAPE_GROUP_GAP + LANDSCAPE_MARGIN;
     }
     if (height > width * 1.15) {
      this.root.dataset.layoutMode = this.landscapeLayoutMode + "-board";
      return this.layoutArchitectureBoard(projection);
     }
     this.primaryGroupIDs = primaryIDs;
     this.initialGroupIDs = new Set(this.orderedLandscapeGroups(projection).slice(0, 4).map((group) => group.id));
    this.buildLandscapeEdgeRoutes();
    return {
     width: width,
     height: height,
     };
   }

   buildLandscapeEdgeRoutes() {
    this.structuralEdges.forEach((edge) => {
     const from = this.nodePositions.get(text(edge.from_component_id));
     const to = this.nodePositions.get(text(edge.to_component_id));
     if (!from || !to || from === to) return;
     const fromCenter = { x: from.x + from.width / 2, y: from.y + from.height / 2 };
     const toCenter = { x: to.x + to.width / 2, y: to.y + to.height / 2 };
     let route = "";
     if (Math.abs(toCenter.x - fromCenter.x) >= Math.abs(toCenter.y - fromCenter.y)) {
      const rightward = toCenter.x >= fromCenter.x;
      const startX = rightward ? from.x + from.width : from.x;
      const endX = rightward ? to.x : to.x + to.width;
      const middleX = (startX + endX) / 2;
      route = "M" + startX + " " + fromCenter.y + " L" + middleX + " " + fromCenter.y +
       " L" + middleX + " " + toCenter.y + " L" + endX + " " + toCenter.y;
     } else {
      const downward = toCenter.y >= fromCenter.y;
      const startY = downward ? from.y + from.height : from.y;
      const endY = downward ? to.y : to.y + to.height;
      const middleY = (startY + endY) / 2;
      route = "M" + fromCenter.x + " " + startY + " L" + fromCenter.x + " " + middleY +
       " L" + toCenter.x + " " + middleY + " L" + toCenter.x + " " + endY;
     }
     this.edgeRoutes.set(layoutStructuralEdgeID(edge.id), route);
    });
   }

   renderPersistentScene() {
   this.loading.remove();
   this.surface = element("div", "rm-arch__surface");
   this.surface.style.width = this.layoutResult.width + "px";
   this.surface.style.height = this.layoutResult.height + "px";

   this.structuralSVG = this.createSVG("rm-arch__edges rm-arch__edges--structural");
   this.flowSVG = this.createSVG("rm-arch__edges rm-arch__edges--flow");
   this.surface.append(this.structuralSVG, this.flowSVG);

   this.groupLayer = element("div", "rm-arch__groups");
   this.nodeLayer = element("div", "rm-arch__nodes");
   this.stepLayer = element("div", "rm-arch__steps");
   this.surface.append(this.groupLayer, this.nodeLayer, this.stepLayer);
    this.viewport.appendChild(this.surface);
    if (this.landscapeProjection) {
     this.viewportHint.textContent = "Drag to explore · " + this.landscapeProjection.groups.length +
      " architecture groups · Fit keeps labels readable";
    }

   this.renderGroups();
   this.renderComponents();
   this.renderUnassignedRail();
   this.renderStructuralEdges();
   this.renderFlowSteps();
   this.renderFlowEdges();
   this.applyView();

  }

  createSVG(className) {
   const svg = svgElement("svg", {
    class: className,
    width: this.layoutResult.width,
    height: this.layoutResult.height,
    viewBox: "0 0 " + this.layoutResult.width + " " + this.layoutResult.height,
   });
   svg.style.width = this.layoutResult.width + "px";
   svg.style.height = this.layoutResult.height + "px";
   return svg;
  }

   renderGroups() {
    this.subsystems.forEach((subsystem) => {
     const position = this.groupPositions.get(text(subsystem.id));
     if (!position) return;
      const category = this.semanticCategory(subsystem, "neutral");
      const singleton = this.singletonGroupIDs.has(text(subsystem.id));
      const group = element("section", "rm-arch__group is-" + category + (singleton ? " is-singleton" : ""));
     group.style.left = position.x + "px";
    group.style.top = position.y + "px";
    group.style.width = position.width + "px";
    group.style.height = position.height + "px";
     group.title = text(subsystem.description || subsystem.name || subsystem.id);
     const header = element("div", "rm-arch__group-header");
     header.appendChild(element("span", "rm-arch__category-marker"));
     header.appendChild(element("h3", "rm-arch__group-title", subsystem.name || subsystem.id));
     const categoryLabel = this.semanticCategoryLabel(category);
     if (categoryLabel) header.appendChild(element("span", "rm-arch__group-category", categoryLabel));
     header.appendChild(element("span", "rm-arch__group-count", array(subsystem.component_ids).length + " components"));
     group.appendChild(header);
     this.groupLayer.appendChild(group);
   });

   const ungrouped = this.groupPositions.get("__ungrouped__");
   if (ungrouped) {
     const group = element("section", "rm-arch__group is-ungrouped is-diagnostic");
    group.style.left = ungrouped.x + "px";
    group.style.top = ungrouped.y + "px";
    group.style.width = ungrouped.width + "px";
    group.style.height = ungrouped.height + "px";
     const header = element("div", "rm-arch__group-header");
     header.appendChild(element("span", "rm-arch__category-marker"));
     header.appendChild(element("h3", "rm-arch__group-title", "Unassigned subsystem"));
     header.appendChild(element("span", "rm-arch__group-category", "Unresolved"));
     group.appendChild(header);
    this.groupLayer.appendChild(group);
   }
  }

  componentSurfaces(component) {
   const ids = Array.from(new Set(array(component && component.owned_surface_ids).map(text)));
   return ids.map((id) => this.surfaceByID.get(id)).filter(Boolean);
  }

  renderComponents() {
   this.components.forEach((component) => {
    const id = text(component.id);
     const position = this.nodePositions.get(id);
     if (!position) return;
      const subsystem = this.subsystemByID.get(text(component.subsystem_id));
      const singleton = this.singletonGroupIDs.has(text(component.subsystem_id));
      const category = this.semanticCategory(component, this.semanticCategory(subsystem, "neutral"));
      const shell = element("article", "rm-arch__component is-" + category + (singleton ? " is-singleton" : ""));
    shell.style.left = position.x + "px";
    shell.style.top = position.y + "px";
    shell.style.width = position.width + "px";
    shell.style.height = position.height + "px";

      const button = element("button", "rm-arch__component-card");
      button.type = "button";
      button.title = text(component.name || id);
      if (singleton && subsystem) {
       button.appendChild(element("span", "rm-arch__component-group", subsystem.name || subsystem.id));
      }
     button.appendChild(element("strong", "rm-arch__component-name", component.name || id));
     if (component.description) {
      button.appendChild(element("span", "rm-arch__component-description", component.description));
     }
      const associatedSurfaces = this.componentSurfaces(component);
      if (associatedSurfaces.length > 0) {
       const surfaceSummary = element("span", "rm-arch__component-surfaces");
       surfaceSummary.appendChild(element("span", "rm-arch__component-surfaces-label", "Surfaces"));
       associatedSurfaces.slice(0, 2).forEach((surface) => {
        surfaceSummary.appendChild(element("span", "rm-arch__surface-chip", surface.name || surface.id));
       });
       if (associatedSurfaces.length > 2) {
        surfaceSummary.appendChild(element("span", "rm-arch__surface-chip is-more", "+" + (associatedSurfaces.length - 2)));
       }
       button.appendChild(surfaceSummary);
      }
       const metadata = [];
       if (associatedSurfaces.length > 0) {
        const allCommands = associatedSurfaces.every((surface) => surface.kind === "cli_command");
        metadata.push(associatedSurfaces.length + " " + (allCommands ? "command" : "surface") + (associatedSurfaces.length === 1 ? "" : "s"));
       }
       const participatingFlows = array(component.participating_flow_ids).map((flowID) => this.flowByID.get(text(flowID))).filter(Boolean);
       if (participatingFlows.length === 1) {
        metadata.push("1 " + (participatingFlows[0].status || "saved") + " trace");
       } else if (participatingFlows.length > 1) {
        metadata.push(participatingFlows.length + " saved traces");
       }
       const suggestionCount = array(component.suggested_investigation_ids).length;
       if (suggestionCount > 0) metadata.push(suggestionCount + " suggested investigation" + (suggestionCount === 1 ? "" : "s"));
       const anchorCount = array(component.anchor_ids).length;
       if (anchorCount > 0) metadata.push(anchorCount + " exact anchor" + (anchorCount === 1 ? "" : "s"));
       if (metadata.length === 0) {
        const memberCount = array(component.members).length;
        if (memberCount > 0) metadata.push(memberCount + " exact member" + (memberCount === 1 ? "" : "s"));
       }
       if (metadata.length > 0) button.appendChild(element("span", "rm-arch__component-meta", metadata.join(" · ")));
    this.listen(button, "click", () => {
     const selected = this.selection.component === id && !this.selection.step && !this.selection.edge;
      this.setSelection({ component: selected ? "" : id, surface: "", step: "", edge: "" }, true);
    });
    shell.appendChild(button);
    this.nodeLayer.appendChild(shell);
    this.componentElements.set(id, shell);
   });
  }

  renderUnassignedRail() {
   const position = this.nodePositions.get(UNASSIGNED_ID);
   if (!position) return;
   this.unassignedRail = element("section", "rm-arch__unassigned");
   this.unassignedRail.style.left = position.x + "px";
   this.unassignedRail.style.top = position.y + "px";
   this.unassignedRail.style.width = position.width + "px";
   this.unassignedRail.style.height = position.height + "px";
   this.unassignedRail.appendChild(element("h3", "rm-arch__unassigned-title", "Unassigned evidence"));
   this.unassignedRail.appendChild(
    element("p", "rm-arch__unassigned-note", "Exact saved facts without a unique component binding")
   );
   this.nodeLayer.appendChild(this.unassignedRail);
  }

  renderStructuralEdges() {
   const representedPairs = new Set();
   this.structuralEdges.forEach((edge) => {
    const pair = text(edge.from_component_id) + "\u0000" + text(edge.to_component_id);
    if (representedPairs.has(pair)) return;
    representedPairs.add(pair);
    const route = this.edgeRoutes.get(layoutStructuralEdgeID(edge.id));
    if (!route) return;
    const group = this.interactiveSVGPath(
     route,
     "rm-arch__edge rm-arch__edge--structural",
     "Structural relation " + text((edge.witness || {}).kind || edge.id),
     () => this.setSelection({ edge: text(edge.id), step: "" }, true)
    );
     setSVGVisible(group, true);
    this.structuralSVG.appendChild(group);
    this.structuralEdgeElements.set(text(edge.id), group);
   });
  }

  flowBranch(flowID, branchID) {
   return this.flowBranchesByKey.get(selectionKey(flowID, branchID));
  }

  stepBranchKind(flow, step) {
   const branch = this.flowBranch(flow.id, step.branch_id);
   return branch ? text(branch.kind) : "unassigned";
  }

  stepSemantics(flowID, stepID) {
   const classes = new Set();
   this.flowEdges.forEach((edge) => {
    if (text(edge.flow_id) !== text(flowID)) return;
    if (text(edge.from) !== text(stepID) && text(edge.to) !== text(stepID)) return;
    classes.add(semanticClass(edge.relation, edge.invocation));
   });
   this.frontiers.forEach((frontier) => {
    if (text(frontier.flow_id) === text(flowID) && text(frontier.anchor_id) === text(stepID)) {
     classes.add("is-frontier");
    }
   });
   return Array.from(classes);
  }

  renderFlowSteps() {
   this.flows.forEach((flow) => {
    const flowID = text(flow.id);
    const buckets = new Map();
    array(flow.steps).forEach((step) => {
     const owner = this.flowStepOwner(flowID, step.id);
     if (!owner) return;
     if (!buckets.has(owner)) buckets.set(owner, []);
     buckets.get(owner).push({ kind: "step", value: step });
    });
    this.frontiers
     .filter((frontier) => text(frontier.flow_id) === flowID)
     .forEach((frontier) => {
      if (!buckets.has(UNASSIGNED_ID)) buckets.set(UNASSIGNED_ID, []);
      buckets.get(UNASSIGNED_ID).push({ kind: "frontier", value: frontier });
     });

    buckets.forEach((items, owner) => {
     const visible = this.selectVisibleItems(flow, items);
     visible.forEach((item, index) => {
      if (item.kind === "step") this.renderStepChip(flow, item.value, owner, index);
      else this.renderFrontierChip(flowID, item.value, index);
     });
     if (items.length > visible.length) {
      this.renderOverflowChip(flowID, owner, items.length - visible.length);
     }
    });
   });
  }

  selectVisibleItems(flow, items) {
   const ranked = items.slice().sort((a, b) => this.compareFlowItems(flow, a, b));
   const selected = [];
   BRANCH_PRIORITY.forEach((branch) => {
    const item = ranked.find((candidate) =>
     this.flowItemBranch(flow, candidate) === branch && selected.indexOf(candidate) < 0
    );
    if (item && selected.length < MAX_VISIBLE_CHIPS) selected.push(item);
   });
   ranked.forEach((item) => {
    if (selected.length < MAX_VISIBLE_CHIPS && selected.indexOf(item) < 0) selected.push(item);
   });
   return selected;
  }

  flowItemBranch(flow, item) {
   return item.kind === "frontier" ? "unassigned" : this.stepBranchKind(flow, item.value);
  }

  compareFlowItems(flow, a, b) {
   const rank = (item) => {
    const semantics = item.kind === "frontier" ? ["is-frontier"] : this.stepSemantics(flow.id, item.value.id);
    let best = SEMANTIC_PRIORITY.length;
    semantics.forEach((kind) => {
     const index = SEMANTIC_PRIORITY.indexOf(kind);
     if (index >= 0) best = Math.min(best, index);
    });
    return best;
   };
   const difference = rank(a) - rank(b);
   if (difference !== 0) return difference;
   const left = this.flowItemStableKey(a);
   const right = this.flowItemStableKey(b);
   return left < right ? -1 : left > right ? 1 : 0;
  }

  flowItemStableKey(item) {
   const value = item.value || {};
   const location = value.location || value.evidence || {};
   return text(location.path) + "\u0000" + String(Number(location.line || 0)).padStart(10, "0") + "\u0000" + text(value.id);
  }

  renderStepChip(flow, step, owner, index) {
   const flowID = text(flow.id);
   const geometry = this.flowStepGeometry(owner, index);
   if (!geometry) return;
   this.stepGeometry.set(flowStepKey(flowID, step.id), geometry);
   const kind = this.stepBranchKind(flow, step);
   const button = this.canvasChip(
    flowID,
    geometry,
    "rm-arch__step " + branchClass(kind) + " " + this.stepSemantics(flowID, step.id).join(" "),
    step.label || step.id
   );
   this.listen(button, "click", (event) => {
    event.stopPropagation();
    this.setSelection({ flow: flowID, component: text(step.component_id), step: text(step.id), edge: "" }, true);
   });
   this.stepElements.set(flowStepKey(flowID, step.id), button);
  }

  renderFrontierChip(flowID, frontier, index) {
   const geometry = this.flowStepGeometry(UNASSIGNED_ID, index);
   if (!geometry) return;
   const button = this.canvasChip(flowID, geometry, "rm-arch__step is-unassigned is-frontier", frontier.kind || "frontier");
   this.listen(button, "click", (event) => {
    event.stopPropagation();
    this.setSelection({ flow: flowID, component: "", step: "", edge: "" }, true);
   });
   this.frontierElements.set(selectionKey(flowID, frontier.id), button);
  }

  renderOverflowChip(flowID, owner, hiddenCount) {
   const geometry = this.overflowGeometry(owner);
   if (!geometry) return;
   const button = this.canvasChip(flowID, geometry, "rm-arch__step is-overflow", "+" + hiddenCount + " more");
   this.listen(button, "click", (event) => {
    event.stopPropagation();
    this.setSelection({ flow: flowID, component: "", step: "", edge: "" }, true);
   });
   this.stepElements.set(selectionKey(flowID, "overflow:" + owner), button);
  }

  canvasChip(flowID, geometry, className, label) {
   const button = element("button", className);
   button.type = "button";
   button.hidden = true;
   button.style.left = geometry.x + "px";
   button.style.top = geometry.y + "px";
   button.style.width = geometry.width + "px";
   button.style.height = geometry.height + "px";
   button.appendChild(element("span", "rm-arch__step-dot"));
   button.appendChild(element("span", "rm-arch__step-label", label));
   this.stepLayer.appendChild(button);
   return button;
  }

  flowStepGeometry(owner, index) {
   const position = this.nodePositions.get(owner);
   if (!position) return null;
   const available = position.width - 24 - CHIP_GAP;
   const width = (available - CHIP_GAP) / CHIP_COLUMNS;
   const column = index % CHIP_COLUMNS;
   const row = Math.floor(index / CHIP_COLUMNS);
   const top = owner === UNASSIGNED_ID ? 70 : COMPONENT_HEADER_HEIGHT + 8;
   return {
    x: position.x + 12 + column * (width + CHIP_GAP),
    y: position.y + top + row * (CHIP_HEIGHT + CHIP_GAP),
    width: width,
    height: CHIP_HEIGHT,
   };
  }

  overflowGeometry(owner) {
   const position = this.nodePositions.get(owner);
   if (!position) return null;
   return {
    x: position.x + position.width - 76,
    y: position.y + 48,
    width: 64,
    height: 20,
   };
  }

  localFlowLanes() {
   const local = [];
   this.flowEdges.forEach((edge) => {
    const owner = this.flowStepOwner(edge.flow_id, edge.from);
    if (owner && owner === this.flowStepOwner(edge.flow_id, edge.to)) local.push({ edge: edge, owner: owner });
   });
   local.sort((a, b) => {
    const left = text(a.edge.flow_id) + "\u0000" + a.owner + "\u0000" + text(a.edge.id);
    const right = text(b.edge.flow_id) + "\u0000" + b.owner + "\u0000" + text(b.edge.id);
    return left < right ? -1 : left > right ? 1 : 0;
   });
   const counts = new Map();
   const lanes = new Map();
   local.forEach((item) => {
    const bucket = text(item.edge.flow_id) + "\u0000" + item.owner;
    const lane = counts.get(bucket) || 0;
    counts.set(bucket, lane + 1);
    lanes.set(selectionKey(item.edge.flow_id, item.edge.id), lane);
   });
   return lanes;
  }

  localFlowRoute(edge, lane) {
   const owner = this.flowStepOwner(edge.flow_id, edge.from);
   const position = this.nodePositions.get(owner);
   if (!position) return "";
   const from = this.stepGeometry.get(flowStepKey(edge.flow_id, edge.from));
   const to = this.stepGeometry.get(flowStepKey(edge.flow_id, edge.to));
   const band = Math.floor(lane / 4) % 4;
   const railY = position.y + position.height - 7 - (lane % 4) * 4;
   const fromX = from ? from.x + from.width / 2 : position.x + 18 + band * 8;
   const fromY = from ? from.y + from.height : railY;
   const toX = to ? to.x + to.width / 2 : position.x + position.width - 18 - band * 8;
   const toY = to ? to.y + to.height : railY;
   if (fromX === toX && fromY === toY) {
    const loopX = Math.min(position.x + position.width - 6, fromX + 14 + band * 4);
    return "M" + fromX + " " + fromY + " L" + loopX + " " + fromY + " L" + loopX + " " + railY + " L" + fromX + " " + railY;
   }
   return "M" + fromX + " " + fromY + " L" + fromX + " " + railY + " L" + toX + " " + railY + " L" + toX + " " + toY;
  }

  crossFlowRoute(edge) {
   const from = this.stepGeometry.get(flowStepKey(edge.flow_id, edge.from));
   const to = this.stepGeometry.get(flowStepKey(edge.flow_id, edge.to));
   if (!from || !to) return "";

   const fromCenter = { x: from.x + from.width / 2, y: from.y + from.height / 2 };
   const toCenter = { x: to.x + to.width / 2, y: to.y + to.height / 2 };
   const deltaX = toCenter.x - fromCenter.x;
   const deltaY = toCenter.y - fromCenter.y;
   if (Math.abs(deltaX) >= Math.abs(deltaY)) {
    const rightward = deltaX >= 0;
    const startX = rightward ? from.x + from.width : from.x;
    const endX = rightward ? to.x : to.x + to.width;
    const middleX = (startX + endX) / 2;
    return "M" + startX + " " + fromCenter.y +
     " L" + middleX + " " + fromCenter.y +
     " L" + middleX + " " + toCenter.y +
     " L" + endX + " " + toCenter.y;
   }

   const downward = deltaY >= 0;
   const startY = downward ? from.y + from.height : from.y;
   const endY = downward ? to.y : to.y + to.height;
   const middleY = (startY + endY) / 2;
   return "M" + fromCenter.x + " " + startY +
    " L" + fromCenter.x + " " + middleY +
    " L" + toCenter.x + " " + middleY +
    " L" + toCenter.x + " " + endY;
  }

  renderFlowEdges() {
   const defs = svgElement("defs");
   defs.appendChild(this.arrowMarker("rm-arch-arrow", "#2563eb"));
   defs.appendChild(this.arrowMarker("rm-arch-arrow-cancel", "#dc2626"));
   defs.appendChild(this.arrowMarker("rm-arch-arrow-join", "#7c3aed"));
   this.flowSVG.appendChild(defs);
   const localLanes = this.localFlowLanes();

   this.flowEdges.forEach((edge) => {
    const key = selectionKey(edge.flow_id, edge.id);
    const fromOwner = this.flowStepOwner(edge.flow_id, edge.from);
    const isLocal = fromOwner && fromOwner === this.flowStepOwner(edge.flow_id, edge.to);
    const route = isLocal
      ? this.localFlowRoute(edge, localLanes.get(key) || 0)
      : this.crossFlowRoute(edge) || this.edgeRoutes.get(layoutFlowEdgeID(edge.flow_id, edge.id));
    if (!route) return;
    const semantic = semanticClass(edge.relation, edge.invocation);
    const className = [
     "rm-arch__edge",
     "rm-arch__edge--flow",
     semantic,
     isLocal ? "is-local" : "",
     edge.cross_branch ? "is-cross-branch" : "",
     edge.from_branch_id === edge.to_branch_id ? branchClass(this.branchKind(edge.flow_id, edge.from_branch_id)) : "",
    ].filter(Boolean).join(" ");
    const group = this.interactiveSVGPath(
     route,
     className,
     "Flow transition " + text(edge.relation || edge.id) + " from " + text(edge.from) + " to " + text(edge.to),
     () => this.setSelection({ flow: text(edge.flow_id), edge: text(edge.id), step: "" }, true)
    );
    setSVGVisible(group, false);
    const visible = group.querySelector(".rm-arch__edge-visible");
    if (semantic === "is-cancel") visible.setAttribute("marker-end", "url(#rm-arch-arrow-cancel)");
    else if (semantic === "is-join") visible.setAttribute("marker-end", "url(#rm-arch-arrow-join)");
    else visible.setAttribute("marker-end", "url(#rm-arch-arrow)");
    this.flowSVG.appendChild(group);
    this.flowEdgeElements.set(key, group);
    });
  }

  primaryFlowSteps(flow) {
   const stepByID = new Map(array(flow.steps).map((step) => [text(step.id), step]));
   const candidateIDs = [];
   const seen = new Set();
   ["entrypoint", "dispatch", "application_callable"].forEach((kind) => {
    const slot = array(flow.slots).find((item) => text(item.kind) === kind);
    array(slot && slot.evidence_ids).forEach((id) => {
     id = text(id);
     if (!stepByID.has(id) || seen.has(id)) return;
     seen.add(id);
     candidateIDs.push(id);
    });
   });

   if (candidateIDs.length === 0) {
    array(flow.steps).forEach((step) => {
     if (text(step.kind) !== "entrypoint" && !/^step-\d+-/.test(text(step.id))) return;
     const id = text(step.id);
     if (seen.has(id)) return;
     seen.add(id);
     candidateIDs.push(id);
    });
   }

   const candidateSet = new Set(candidateIDs);
   const edges = this.focusedFlowEdges(flow.id).filter((edge) =>
    candidateSet.has(text(edge.from)) && candidateSet.has(text(edge.to))
   );
   const incoming = new Set(edges.map((edge) => text(edge.to)));
   let current = candidateIDs.find((id) => text((stepByID.get(id) || {}).kind) === "entrypoint" && !incoming.has(id));
   if (!current) current = candidateIDs.find((id) => !incoming.has(id)) || candidateIDs[0];

   const ordered = [];
   const used = new Set();
   while (current && !used.has(current)) {
    used.add(current);
    ordered.push(stepByID.get(current));
    const nextEdges = edges
     .filter((edge) => text(edge.from) === current && !used.has(text(edge.to)))
     .sort((a, b) => text(a.id).localeCompare(text(b.id)));
    current = nextEdges.length === 1 ? text(nextEdges[0].to) : "";
   }

   return {
    items: ordered.slice(0, 8),
    total: ordered.length,
    disconnected: candidateIDs.length - ordered.length,
   };
  }

  focusedStepButton(flow, step, className, sequence) {
   const button = element("button", "rm-arch__focus-step " + (className || ""));
   button.type = "button";
   button.dataset.stepId = text(step.id);
   if (sequence) button.appendChild(element("span", "rm-arch__focus-step-number", sequence));
   const copy = element("span", "rm-arch__focus-step-copy");
   copy.appendChild(element("strong", null, step.label || step.id));
   const component = this.componentByID.get(text(step.component_id));
   copy.appendChild(element(
    "span",
    "rm-arch__focus-step-meta",
    [component && (component.name || component.id), locationLabel(step.location)].filter(Boolean).join(" · ") || "Exact saved anchor"
   ));
   button.appendChild(copy);
   this.listen(button, "click", () => this.setSelection({
    flow: text(flow.id), component: text(step.component_id), step: text(step.id), edge: "",
   }, true));
   return button;
  }

  focusedTransitionButton(flow, edge, className) {
   if (!edge) return element("span", "rm-arch__focus-transition is-static", "then");
   const button = element(
    "button",
    "rm-arch__focus-transition " + (className || "") + " " + semanticClass(edge.relation, edge.invocation),
    relationLabel(edge.relation)
   );
   button.type = "button";
   button.dataset.edgeId = text(edge.id);
   button.setAttribute("aria-label", "Inspect " + text(edge.relation || "flow") + " transition");
   this.listen(button, "click", () => this.setSelection({
    flow: text(flow.id), edge: text(edge.id), step: "", component: "",
   }, true));
   return button;
  }

  focusedFlowEdges(flowID) {
   return this.flowEdges.filter((edge) => text(edge.flow_id) === text(flowID));
  }

  focusedOperationEdges(flow, primary) {
   if (primary.length === 0) return { items: [], total: 0 };
   const source = primary[primary.length - 1];
   const primaryIDs = new Set(primary.map((step) => text(step.id)));
   const seenTargets = new Set();
   const items = this.focusedFlowEdges(flow.id)
    .filter((edge) => text(edge.from) === text(source.id) && !primaryIDs.has(text(edge.to)))
    .map((edge) => ({ edge: edge, target: this.flowStepsByKey.get(flowStepKey(flow.id, edge.to)) }))
    .filter((item) => item.target)
    .filter((item) => {
     const targetComponent = text(item.target.component_id);
     return !targetComponent || targetComponent !== text(source.component_id) ||
      semanticClass(item.edge.relation, item.edge.invocation) !== "is-call";
    })
    .sort((a, b) => {
     const rank = (item) => {
      const targetComponent = text(item.target.component_id);
      if (targetComponent && targetComponent !== text(source.component_id)) return 0;
      if (!targetComponent) return 1;
      const semantic = semanticClass(item.edge.relation, item.edge.invocation);
      return semantic === "is-call" ? 3 : 2;
     };
     const difference = rank(a) - rank(b);
     if (difference !== 0) return difference;
     const left = Number((a.edge.evidence || {}).line || (a.target.location || {}).line || 0);
     const right = Number((b.edge.evidence || {}).line || (b.target.location || {}).line || 0);
     return left - right || text(a.target.id).localeCompare(text(b.target.id));
    })
    .filter((item) => {
     const id = text(item.target.id);
     if (seenTargets.has(id)) return false;
     seenTargets.add(id);
     return true;
    });
   return { items: items.slice(0, 8), total: items.length };
  }

  focusedLifecycleGroups(flow) {
   const edges = this.focusedFlowEdges(flow.id).filter((edge) =>
    this.flowStepsByKey.has(flowStepKey(flow.id, edge.from)) &&
    this.flowStepsByKey.has(flowStepKey(flow.id, edge.to))
   );
   return groupLifecycleRelations(flow, edges);
  }

  appendFocusedLifecycleRelation(parent, flow, edge, rootAnchorID) {
   const row = element("div", "rm-arch__lifecycle-row " + semanticClass(edge.relation, edge.invocation));
   row.appendChild(element("strong", "rm-arch__lifecycle-label", lifecycleRelationHeading(edge)));
   const content = element("div", "rm-arch__lifecycle-content");
   const endpoints = element("div", "rm-arch__lifecycle-endpoints");
   const source = this.flowStepsByKey.get(flowStepKey(flow.id, edge.from));
   const target = this.flowStepsByKey.get(flowStepKey(flow.id, edge.to));
   const endpoint = (step) => {
    if (text(step.id) === text(rootAnchorID)) {
     return element("span", "rm-arch__lifecycle-root-reference", "This task");
    }
    return this.focusedStepButton(
     flow,
     step,
     "is-compact " + branchClass(this.stepBranchKind(flow, step)),
     ""
    );
   };
   endpoints.appendChild(endpoint(source));
   endpoints.appendChild(this.focusedTransitionButton(flow, edge, "is-handoff"));
   endpoints.appendChild(endpoint(target));
   content.appendChild(endpoints);
   const metadata = element("div", "rm-arch__lifecycle-meta");
   metadata.appendChild(element("code", null, text(edge.relation)));
   this.appendLocation(metadata, edge.evidence, "Exact source");
   content.appendChild(metadata);
   row.appendChild(content);
   parent.appendChild(row);
  }

  appendFocusedLifecycleCard(parent, flow, group, concurrency, ungroupedTotal) {
   const card = element("article", "rm-arch__lifecycle-card");
   const header = element("header", "rm-arch__lifecycle-card-header");
   const root = group.rootAnchorID && this.flowStepsByKey.get(flowStepKey(flow.id, group.rootAnchorID));
   header.appendChild(element("span", "rm-arch__lifecycle-kicker", root ? "Task branch" : "Ungrouped lifecycle evidence"));
   if (root) {
    header.appendChild(this.focusedStepButton(flow, root, "is-compact is-task", ""));
   } else {
    header.appendChild(element("strong", null, "No exact task root is available"));
   }
   card.appendChild(header);
   const relations = element("div", "rm-arch__lifecycle-relations");
   group.relations.forEach((edge) => this.appendFocusedLifecycleRelation(relations, flow, edge, group.rootAnchorID));
   const limitation = element("div", "rm-arch__lifecycle-limitation");
   limitation.appendChild(element("strong", null, "Limitation"));
   const limitations = [];
   if (concurrency && concurrency.missing) limitations.push(text(concurrency.missing));
   if (!root && ungroupedTotal > group.relations.length) {
    limitations.push("Showing " + group.relations.length + " of " + ungroupedTotal + " ungrouped lifecycle relations.");
   }
   if (!root) limitations.push("No exact task root was available, so these relations remain ungrouped.");
   limitations.push("Static relation evidence only; execution and runtime ordering were not observed.");
   limitation.appendChild(element("span", null, limitations.join(" ")));
   relations.appendChild(limitation);
   card.appendChild(relations);
   parent.appendChild(card);
  }

  focusedProofSummary(flow) {
   const slots = array(flow.slots);
   const counts = { verified: 0, partial: 0, missing: 0, not_applicable: 0, unknown: 0 };
   slots.forEach((slot) => {
    const status = text(slot.status) || "unknown";
    if (Object.prototype.hasOwnProperty.call(counts, status)) counts[status]++;
    else counts.unknown++;
   });
   const firstGap = slots.find((slot) => {
    const status = text(slot.status);
    return status === "missing" || status === "partial" || !status;
   });
   const status = counts.missing > 0 ? "missing" : counts.partial > 0 || counts.unknown > 0 ? "partial" : "grounded";
   return {
    total: slots.length,
    grounded: counts.verified,
    counts: counts,
    status: status,
    firstGap: firstGap,
   };
  }

  focusedEvidenceDisclosure(flow) {
   const details = element("details", "rm-arch__focus-evidence");
   const summary = element("summary", "rm-arch__focus-evidence-summary");
   summary.appendChild(element("strong", null, "Inspect full evidence"));
   summary.appendChild(element("span", null, array(flow.steps).length + " anchors · " + array(flow.slots).length + " proof areas"));
   details.appendChild(summary);
   const body = element("div", "rm-arch__focus-evidence-body");
   this.appendFlowEvidence(body, flow);
   details.appendChild(body);
   return details;
  }

  appendFlowEvidence(parent, flow) {
   const section = (title) => {
    const node = element("section", "rm-arch__inspector-section");
    node.appendChild(element("h4", "rm-arch__inspector-section-title", title));
    parent.appendChild(node);
    return node;
   };
   if (flow.trigger) this.appendKeyValue(parent, "Starts when", flow.trigger);
   if (flow.scope) this.appendKeyValue(parent, "Scope", flow.scope);
   if (flow.command) this.appendKeyValue(parent, "Command", flow.command);
   if (flow.trace_quality) this.appendKeyValue(parent, "Trace quality", flow.trace_quality);
   if (flow.current_frontier) this.appendKeyValue(parent, "Current frontier", flow.current_frontier);

   const branches = section("Branches");
   array(flow.branches).forEach((branch) => {
    const row = element("div", "rm-arch__branch-row " + branchClass(branch.kind));
    row.appendChild(element("strong", null, branchLabel(branch.kind)));
    row.appendChild(element("span", null, array(branch.anchor_ids).length + " exact anchors"));
    if (branch.root_anchor_id) {
     const root = this.flowStepsByKey.get(flowStepKey(flow.id, branch.root_anchor_id));
     row.appendChild(element("code", null, root && (root.label || root.qualified_name) || "task root"));
    }
    branches.appendChild(row);
   });

   const steps = section("Exact steps");
   array(flow.steps).forEach((step) => steps.appendChild(this.stepJumpButton(flow, step)));

   const slots = section("Proof slots");
   array(flow.slots).forEach((slot) => {
    const card = element("article", "rm-arch__slot is-" + (text(slot.status) || "unknown"));
    const header = element("div", "rm-arch__slot-header");
    header.appendChild(element("strong", null, proofAreaLabel(slot.kind)));
    header.appendChild(element("span", "rm-arch__badge", text(slot.status).replace(/_/g, " ")));
    card.appendChild(header);
    if (slot.summary) card.appendChild(element("p", "rm-arch__copy", slot.summary));
    if (slot.missing) card.appendChild(element("p", "rm-arch__notice is-warning", slot.missing));
    if (slot.applicability_reason) this.appendKeyValue(card, "Applicability", slot.applicability_reason);
    this.appendProvenance(card, slot.provenance);
    slots.appendChild(card);
   });

   const frontiers = section("Unresolved frontiers");
   const flowFrontiers = this.frontiers.filter((frontier) => text(frontier.flow_id) === text(flow.id));
   if (flowFrontiers.length === 0) {
    frontiers.appendChild(element("p", "rm-arch__empty", "No explicit frontier was saved for this flow."));
   } else {
    flowFrontiers.forEach((frontier) => {
     const row = element("div", "rm-arch__frontier-row");
     row.appendChild(element("strong", null, frontier.kind || frontier.id));
     row.appendChild(element("span", null, frontier.reason));
     this.appendLocation(row, frontier.evidence, "Known evidence");
     frontiers.appendChild(row);
    });
   }
   this.appendDiagnostics(section("Diagnostics"), text(flow.id));
  }

  renderFocusedFlow(flow) {
   const flowID = text(flow && flow.id);
   if (!flowID || this.focusFlowID === flowID) return;
   this.focusFlowID = flowID;
   this.flowFocus.replaceChildren();

   const primaryProjection = this.primaryFlowSteps(flow);
   const primary = primaryProjection.items;
   const flowEdges = this.focusedFlowEdges(flowID);
   const operations = this.focusedOperationEdges(flow, primary);
   const lifecycle = this.focusedLifecycleGroups(flow);
   const proof = this.focusedProofSummary(flow);
   const concurrency = array(flow.slots).find((slot) => text(slot.kind) === "concurrency");
   const evidenceDisclosure = this.focusedEvidenceDisclosure(flow);
   const header = element("header", "rm-arch__focus-header");
   const back = element("button", "rm-arch__focus-back", "← Back to architecture");
   back.type = "button";
   this.listen(back, "click", () => this.backToArchitecture());
   header.appendChild(back);
   const heading = element("div", "rm-arch__focus-heading");
   heading.appendChild(element("span", "rm-arch__focus-kicker", savedTraceLabel(flow.archetype)));
   heading.appendChild(element("h3", null, flow.name || flow.id));
   heading.appendChild(element("p", null, flow.why_inspect || flow.mental_model || flow.goal || "Inspect the exact static handoffs and current frontier."));
   const summary = element("dl", "rm-arch__trace-summary");
   const startSurface = this.surfaceByID.get(text(flow.start_surface_id));
   const trigger = flow.trigger || flow.command || startSurface && (startSurface.name || startSurface.id) || "Explicit investigation";
   const groundedSequence = primary.length > 1 ?
    primary.length + " exact anchors linked by explicit static transitions; runtime order is not inferred" :
    primary.length === 1 ? "1 exact anchor; no downstream transition is established" : "No connected static sequence is established";
   const concurrentActivities = lifecycle.groups.length > 0 ?
    lifecycle.groups.length + " task branch" + (lifecycle.groups.length === 1 ? "" : "es") +
     " with typed lifecycle relations grouped by task root" :
    lifecycle.ungroupedTotal > 0 ? lifecycle.ungroupedTotal + " typed lifecycle relations without an exact task root" :
     "No concurrent task relation is established in this bounded trace";
   this.appendSummaryItem(summary, "Trigger", trigger);
   this.appendSummaryItem(summary, "What the system does", flow.mental_model || flow.goal || flow.why_inspect || "Bounded behavior remains unresolved");
   const participating = element("div", "rm-arch__trace-participants");
   participating.appendChild(element("dt", null, "Participating components"));
   const participantValues = element("dd");
   array(flow.participating_component_ids).forEach((componentID) => {
    const component = this.componentByID.get(text(componentID));
    const button = element("button", null, component && (component.name || component.id) || componentID);
    button.type = "button";
    this.listen(button, "click", () => this.setSelection({ component: text(componentID), surface: "", step: "", edge: "" }, true));
    participantValues.appendChild(button);
   });
   if (participantValues.childElementCount === 0) participantValues.appendChild(element("span", null, "Unassigned"));
   participating.appendChild(participantValues);
   summary.appendChild(participating);
   this.appendSummaryItem(summary, "Grounded sequence", groundedSequence);
   this.appendSummaryItem(summary, "Concurrent activities", concurrentActivities);
    this.appendSummaryItem(summary, "Current frontier", flow.frontier_summary || flow.current_frontier || "No explicit frontier was saved");
    this.appendSummaryItem(summary, "Evidence basis", (flow.evidence_basis || "static") + " evidence; execution was not observed");
    this.appendSummaryItem(
     summary,
     "Trace surfaces",
     String((flow.seed_surface_id ? 1 : 0) + array(flow.trace_evidence_surface_ids).length) +
      " exact seed/evidence surface(s)"
    );
    this.appendSummaryItem(
     summary,
     "Other component surfaces",
     String(array(flow.related_component_surface_ids).length) + " related surface(s) — not trace evidence"
    );
   heading.appendChild(summary);
   const proofNode = element("div", "rm-arch__focus-proof is-" + proof.status);
   proofNode.appendChild(element("strong", null, proof.grounded + "/" + proof.total + " proof areas grounded"));
   const proofCopy = [];
   if (proof.firstGap) {
    proofCopy.push(proofAreaLabel(proof.firstGap.kind) + " " + (text(proof.firstGap.status) || "unknown"));
    if (proof.firstGap.missing) proofCopy.push(text(proof.firstGap.missing));
   }
   proofCopy.push("Static evidence; execution was not observed");
   proofNode.appendChild(element("span", null, proofCopy.join(" · ")));
   const proofAction = element("button", "rm-arch__focus-proof-action", "Inspect full evidence");
   proofAction.type = "button";
   this.listen(proofAction, "click", () => {
    evidenceDisclosure.open = true;
    evidenceDisclosure.scrollIntoView({ behavior: "smooth", block: "nearest" });
   });
   proofNode.appendChild(proofAction);
   heading.appendChild(proofNode);
   header.appendChild(heading);
   const stats = element("div", "rm-arch__focus-stats");
   stats.appendChild(element("strong", null, array(flow.steps).length + " exact anchors"));
   stats.appendChild(element("span", null, flowEdges.length + " evidenced transitions"));
   stats.appendChild(element("span", null, array(flow.branches).length + " trace lanes"));
   header.appendChild(stats);
   this.flowFocus.appendChild(header);

   if (primary.length > 0) {
    const section = element("section", "rm-arch__focus-section");
    const sectionHeader = element("div", "rm-arch__focus-section-heading");
    sectionHeader.appendChild(element("h4", null, "Grounded sequence"));
    const pathNotes = ["Exact anchors linked by explicit static transitions. Execution was not observed, and runtime order is not inferred."];
    if (primaryProjection.total > primary.length) {
     pathNotes.push("Showing " + primary.length + " of " + primaryProjection.total + " connected anchors.");
    }
    if (primaryProjection.disconnected > 0) {
     pathNotes.push(primaryProjection.disconnected + " role anchor" + (primaryProjection.disconnected === 1 ? " is" : "s are") + " not linked by the dispatch chain.");
    }
    sectionHeader.appendChild(element("p", null, pathNotes.join(" ")));
    section.appendChild(sectionHeader);
    const path = element("div", "rm-arch__focus-path");
    primary.forEach((step, index) => {
     if (index > 0) {
      const previous = primary[index - 1];
      const edge = flowEdges.find((candidate) =>
       text(candidate.from) === text(previous.id) && text(candidate.to) === text(step.id)
      );
      path.appendChild(this.focusedTransitionButton(flow, edge, "is-path"));
     }
     path.appendChild(this.focusedStepButton(flow, step, "is-primary", String(index + 1).padStart(2, "0")));
    });
    section.appendChild(path);
    this.flowFocus.appendChild(section);
   }

   if (operations.items.length > 0) {
    const section = element("section", "rm-arch__focus-section");
    const sectionHeader = element("div", "rm-arch__focus-section-heading");
    sectionHeader.appendChild(element("h4", null, "Key operations"));
    let operationsCopy = "Important exact calls leaving the application callable; this is fan-out evidence, not runtime order.";
    if (operations.total > operations.items.length) {
     operationsCopy += " Showing " + operations.items.length + " of " + operations.total + ".";
    }
    sectionHeader.appendChild(element("p", null, operationsCopy));
    section.appendChild(sectionHeader);
    const grid = element("div", "rm-arch__focus-operations");
    operations.items.forEach((item) => {
     const card = element("article", "rm-arch__focus-operation " + semanticClass(item.edge.relation, item.edge.invocation));
     card.appendChild(this.focusedTransitionButton(flow, item.edge, "is-operation"));
     card.appendChild(this.focusedStepButton(flow, item.target, branchClass(this.stepBranchKind(flow, item.target)), ""));
     grid.appendChild(card);
    });
    section.appendChild(grid);
    this.flowFocus.appendChild(section);
   }

   if (lifecycle.groups.length > 0 || lifecycle.ungrouped.length > 0) {
    const section = element("section", "rm-arch__focus-section");
    const sectionHeader = element("div", "rm-arch__focus-section-heading");
    sectionHeader.appendChild(element("h4", null, "Concurrent activities"));
    sectionHeader.appendChild(element(
     "p",
     null,
     "Typed start, callback, cancellation, and join rows are grouped by task root. They are not flattened into a runtime sequence."
    ));
    section.appendChild(sectionHeader);
    const cards = element("div", "rm-arch__focus-lifecycle");
    lifecycle.groups.forEach((group) => {
     this.appendFocusedLifecycleCard(cards, flow, group, concurrency, 0);
    });
    if (lifecycle.ungrouped.length > 0) {
     this.appendFocusedLifecycleCard(cards, flow, {
      rootAnchorID: "",
      relations: lifecycle.ungrouped,
     }, concurrency, lifecycle.ungroupedTotal);
    }
    section.appendChild(cards);
    this.flowFocus.appendChild(section);
   }

   this.flowFocus.appendChild(evidenceDisclosure);
  }

  syncFocusedSelection() {
   if (!this.flowFocus) return;
   this.flowFocus.querySelectorAll("[data-step-id]").forEach((node) => {
    node.classList.toggle("is-selected", node.dataset.stepId === this.selection.step);
   });
   this.flowFocus.querySelectorAll("[data-edge-id]").forEach((node) => {
    node.classList.toggle("is-selected", node.dataset.edgeId === this.selection.edge);
   });
  }

  branchKind(flowID, branchID) {
   const branch = this.flowBranch(flowID, branchID);
   return branch ? text(branch.kind) : "unassigned";
  }

  arrowMarker(id, color) {
   const marker = svgElement("marker", {
    id: id,
    viewBox: "0 0 10 10",
    refX: "9",
    refY: "5",
    markerWidth: "7",
    markerHeight: "7",
    orient: "auto-start-reverse",
   });
   marker.appendChild(svgElement("path", { d: "M 0 0 L 10 5 L 0 10 z", fill: color }));
   return marker;
  }

  interactiveSVGPath(route, className, label, handler) {
   const group = svgElement("g", { class: className, role: "button", tabindex: "0", "aria-label": label });
   group.appendChild(svgElement("path", { class: "rm-arch__edge-hit", d: route }));
   group.appendChild(svgElement("path", { class: "rm-arch__edge-visible", d: route }));
   this.listen(group, "click", (event) => {
    event.stopPropagation();
    handler();
   });
   this.listen(group, "keydown", (event) => {
    if (event.key !== "Enter" && event.key !== " ") return;
    event.preventDefault();
    event.stopPropagation();
    handler();
   });
   return group;
  }

  installViewportInteractions() {
   this.listen(this.viewport, "wheel", (event) => {
    if (!this.surface || this.selection.flow) return;
    event.preventDefault();
    let delta = event.deltaY;
    if (event.deltaMode === WheelEvent.DOM_DELTA_LINE) delta *= 16;
    else if (event.deltaMode === WheelEvent.DOM_DELTA_PAGE) delta *= this.viewport.clientHeight;
    delta = clamp(delta, -MAX_WHEEL_DELTA, MAX_WHEEL_DELTA);
    if (Math.abs(delta) < 0.01) return;
    const factor = Math.exp(-delta * WHEEL_ZOOM_SENSITIVITY);
    this.zoomBy(factor);
   }, { passive: false });

   this.listen(this.viewport, "pointerdown", (event) => {
    if (!this.surface || this.selection.flow || event.button !== 0) return;
    if (event.target.closest("button, .rm-arch__edge")) return;
    this.drag = { pointerID: event.pointerId, x: event.clientX, y: event.clientY, originX: this.view.x, originY: this.view.y };
    this.viewport.setPointerCapture(event.pointerId);
    this.viewport.classList.add("is-panning");
   });
   this.listen(this.viewport, "pointermove", (event) => {
    if (!this.drag || event.pointerId !== this.drag.pointerID) return;
    this.view.x = this.drag.originX + event.clientX - this.drag.x;
    this.view.y = this.drag.originY + event.clientY - this.drag.y;
    this.applyView();
   });
   const finishDrag = (event) => {
    if (!this.drag || event.pointerId !== this.drag.pointerID) return;
    this.drag = null;
    this.viewport.classList.remove("is-panning");
   };
   this.listen(this.viewport, "pointerup", finishDrag);
   this.listen(this.viewport, "pointercancel", finishDrag);
  }

  zoomBy(factor) {
   if (!this.surface) return;
   const rect = this.viewport.getBoundingClientRect();
   const localX = rect.width / 2;
   const localY = rect.height / 2;
   const oldScale = this.view.scale;
   const nextScale = clamp(oldScale * factor, MIN_SCALE, MAX_SCALE);
   const graphX = (localX - this.view.x) / oldScale;
   const graphY = (localY - this.view.y) / oldScale;
   this.view.scale = nextScale;
   this.view.x = localX - graphX * nextScale;
   this.view.y = localY - graphY * nextScale;
   this.applyView();
  }

   fit() {
   if (!this.surface || !this.layoutResult) return;
   const bounds = this.selectedFlowBounds() || this.landscapeBounds() || {
     x: 0,
     y: 0,
     width: this.layoutResult.width,
    height: this.layoutResult.height,
   };
    this.fitBounds(bounds);
   }

   primaryLandscapeBounds() {
     const groupIDs = this.initialGroupIDs && this.initialGroupIDs.size > 0
      ? this.initialGroupIDs
      : this.primaryGroupIDs;
     if (!groupIDs || groupIDs.size === 0) return this.landscapeBounds();
     const positions = Array.from(groupIDs)
      .map((id) => this.groupPositions.get(id))
      .filter(Boolean);
     return this.boundsForPositions(positions) || this.landscapeBounds();
    }

   focusInitialLandscape() {
    if (!this.surface || !this.layoutResult) return;
    const bounds = this.primaryLandscapeBounds() || this.landscapeBounds();
    const rect = this.viewport.getBoundingClientRect();
    if (!bounds || rect.width < 10 || rect.height < 10) return;
     const padding = 28;
     const scale = clamp(
      (rect.width - padding * 2) / Math.max(1, bounds.width),
     INITIAL_MIN_SCALE,
      INITIAL_MAX_SCALE
     );
     this.view = centeredTransform(bounds, { width: rect.width, height: rect.height }, scale);
     this.applyView();
    }

   boundsForPositions(positions) {
    if (!positions || positions.length === 0) return null;
    const minX = Math.min.apply(null, positions.map((position) => position.x));
    const minY = Math.min.apply(null, positions.map((position) => position.y));
    const maxX = Math.max.apply(null, positions.map((position) => position.x + position.width));
    const maxY = Math.max.apply(null, positions.map((position) => position.y + position.height));
    return { x: minX, y: minY, width: maxX - minX, height: maxY - minY };
   }

  landscapeBounds() {
   if (this.selection.flow) return null;
   const positions = Array.from(this.groupPositions.values());
   if (positions.length === 0) {
    this.nodePositions.forEach((position, id) => {
     if (id !== UNASSIGNED_ID) positions.push(position);
    });
   }
   if (positions.length === 0) return null;
    return this.boundsForPositions(positions);
   }

   componentContextBounds(componentID) {
    const component = this.componentByID.get(text(componentID));
    if (!component) return null;
    const selectedGroupID = text(component.subsystem_id);
    const neighborGroupIDs = new Set();
    this.structuralEdges.forEach((edge) => {
     let neighborID = "";
     if (text(edge.from_component_id) === text(componentID)) neighborID = text(edge.to_component_id);
     else if (text(edge.to_component_id) === text(componentID)) neighborID = text(edge.from_component_id);
     if (!neighborID || this.diagnosticComponentIDs.has(neighborID)) return;
     const neighbor = this.componentByID.get(neighborID);
     const groupID = text(neighbor && neighbor.subsystem_id);
     if (groupID && groupID !== selectedGroupID && this.groupPositions.has(groupID)) neighborGroupIDs.add(groupID);
    });
    const groupIDs = [selectedGroupID].concat(Array.from(neighborGroupIDs).sort().slice(0, 3));
    const positions = groupIDs.map((id) => this.groupPositions.get(id)).filter(Boolean);
    return this.boundsForPositions(positions) || this.nodePositions.get(text(componentID)) || null;
   }

   focusComponent(componentID, animate) {
    if (!this.surface || this.selection.flow || this.diagnosticComponentIDs.has(text(componentID))) return;
    const target = this.nodePositions.get(text(componentID));
    if (!target) return;
    const context = this.componentContextBounds(componentID) || target;
    const rect = this.viewport.getBoundingClientRect();
    if (rect.width < 10 || rect.height < 10) return;
    const scale = componentFocusScale(context, { width: rect.width, height: rect.height }, 56);
    const centerX = target.x + target.width / 2;
    const centerY = target.y + target.height / 2;
    this.view = {
     x: rect.width / 2 - centerX * scale,
     y: rect.height / 2 - centerY * scale,
     scale: scale,
    };
    this.applyView(animate !== false);
   }

  selectedFlowBounds() {
   const flowID = this.selection.flow;
   if (!flowID) return null;
   const componentIDs = this.flowComponentIDs.get(flowID) || new Set();
   const positions = [];
   componentIDs.forEach((id) => {
    const position = this.nodePositions.get(id);
    if (position) positions.push(position);
   });
   const flow = this.flowByID.get(flowID);
   const hasUnassigned = Boolean(flow) && (
    array(flow.steps).some((step) => !step.component_id) ||
    this.frontiers.some((frontier) => text(frontier.flow_id) === flowID)
   );
   if (hasUnassigned) {
    const unassigned = this.nodePositions.get(UNASSIGNED_ID);
    if (unassigned) positions.push(unassigned);
   }
   if (positions.length === 0) return null;
   const padding = 36;
   const minX = Math.min.apply(null, positions.map((position) => position.x)) - padding;
   const minY = Math.min.apply(null, positions.map((position) => position.y)) - padding;
   const maxX = Math.max.apply(null, positions.map((position) => position.x + position.width)) + padding;
   const maxY = Math.max.apply(null, positions.map((position) => position.y + position.height)) + padding;
   return { x: minX, y: minY, width: maxX - minX, height: maxY - minY };
  }

  fitBounds(bounds) {
   const rect = this.viewport.getBoundingClientRect();
   if (rect.width < 10 || rect.height < 10) return;
   const padding = 28;
    const viewport = { width: rect.width, height: rect.height };
    const scale = readableFitScale(bounds, viewport, padding);
    this.view = centeredTransform(bounds, viewport, scale);
    this.applyView();
   }

   applyView(animate) {
    if (!this.surface) return;
    if (animate) {
     this.surface.classList.add("is-focusing");
     global.setTimeout(() => {
      if (this.surface) this.surface.classList.remove("is-focusing");
     }, 220);
    }
   this.surface.style.transform = "translate(" + this.view.x + "px," + this.view.y + "px) scale(" + this.view.scale + ")";
   this.root.style.setProperty("--rm-arch-scale", this.view.scale);
  }

   setSelection(patch, writeHash) {
     const previous = this.selection;
     const next = Object.assign({}, this.selection, patch || {});
     this.selection = this.validateSelection(next);
     if (!previous.flow && this.selection.flow) {
      this.landscapeView = Object.assign({}, this.view);
      this.landscapeComponentID = previous.component;
      this.returnHighlightIDs = new Set();
     }
     if (previous.flow && !this.selection.flow) {
      this.returnHighlightIDs = new Set(this.flowComponentIDs.get(previous.flow) || []);
     }
     if (this.selection.flow && this.selection.component) {
      this.landscapeComponentID = this.selection.component;
     }
     if (writeHash) this.writeHash();
     this.renderSelection();
     if (previous.flow && !this.selection.flow && this.landscapeView) {
      this.view = Object.assign({}, this.landscapeView);
      this.applyView(false);
     }
    }

   openTrace(flowID) {
    flowID = text(flowID);
    if (!this.flowByID.has(flowID)) return;
    this.setSelection({ flow: flowID, component: "", surface: "", step: "", edge: "" }, true);
   }

   openSurface(surfaceID) {
    surfaceID = text(surfaceID);
    if (!this.surfaceByID.has(surfaceID)) return;
    this.setSelection({ flow: "", component: "", surface: surfaceID, step: "", edge: "" }, true);
   }

   openComponent(componentID) {
    componentID = text(componentID);
    if (!this.componentByID.has(componentID)) return;
    this.setSelection({ flow: "", component: componentID, surface: "", step: "", edge: "" }, true);
   }

   backToArchitecture() {
    const component = this.landscapeComponentID && this.componentByID.has(this.landscapeComponentID)
     ? this.landscapeComponentID
     : "";
    this.setSelection({ flow: "", component: component, surface: "", step: "", edge: "" }, true);
   }

   closeInspector() {
    this.setSelection({ component: "", surface: "", step: "", edge: "" }, true);
   }

  hasInspectorSelection(selection) {
   return Boolean(selection && (selection.component || selection.surface || selection.step || selection.edge));
  }

  validateSelection(selection) {
   const next = {
     flow: text(selection.flow),
     component: text(selection.component),
     surface: text(selection.surface),
    step: text(selection.step),
    edge: text(selection.edge),
   };
   if (next.flow && !this.flowByID.has(next.flow)) next.flow = "";
    if (next.component && !this.componentByID.has(next.component)) next.component = "";
    if (next.surface && !this.surfaceByID.has(next.surface)) next.surface = "";

   if (next.step) {
    if (!next.flow || !this.flowStepsByKey.has(flowStepKey(next.flow, next.step))) next.step = "";
   }
   if (next.edge) {
    const isFlowEdge = next.flow && this.flowEdgesByKey.has(selectionKey(next.flow, next.edge));
    const isStructural = this.structuralEdgeByID.has(next.edge);
    if (!isFlowEdge && !isStructural) next.edge = "";
   }
   return next;
  }

  readHash() {
   const params = new URLSearchParams(global.location.hash.replace(/^#/, ""));
   return {
    flow: params.get("flow") || "",
     component: params.get("component") || "",
     surface: params.get("surface") || "",
    step: params.get("step") || "",
    edge: params.get("edge") || "",
   };
  }

  restoreHash(render) {
   this.selection = this.validateSelection(this.readHash());
   if (render && this.surface) this.renderSelection();
  }

  writeHash() {
   const params = new URLSearchParams(global.location.hash.replace(/^#/, ""));
   HASH_KEYS.forEach((key) => params.delete(key));
   if (this.selection.flow) params.set("flow", this.selection.flow);
    if (this.selection.component) params.set("component", this.selection.component);
    if (this.selection.surface) params.set("surface", this.selection.surface);
   if (this.selection.step) params.set("step", this.selection.step);
   if (this.selection.edge) params.set("edge", this.selection.edge);
   const hash = params.toString();
   const url = global.location.pathname + global.location.search + (hash ? "#" + hash : "");
   global.history.replaceState(null, "", url);
  }

  renderSelection() {
   if (!this.surface) {
    this.renderInspector();
    return;
   }
    const flowID = this.selection.flow;
    const hasFlow = Boolean(flowID);
   this.root.classList.toggle("has-selected-flow", hasFlow);
     this.landscapeButton.classList.toggle("is-active", !hasFlow);
     this.landscapeButton.textContent = hasFlow ? "← Back to architecture" : "Architecture";
     this.flowButtons.forEach((button, id) => button.classList.toggle("is-active", id === flowID));
     if (this.diagnosticButton) {
      this.diagnosticButton.classList.toggle("is-active", this.diagnosticComponentIDs.has(this.selection.component));
     }
    this.surface.hidden = hasFlow;
    this.flowFocus.hidden = !hasFlow;
    if (hasFlow) this.renderFocusedFlow(this.flowByID.get(flowID));
    this.syncFocusedSelection();

   const relatedComponents = this.flowComponentIDs.get(flowID) || new Set();
    this.componentElements.forEach((node, id) => {
     node.classList.toggle("is-selected", id === this.selection.component);
    node.classList.toggle("is-unrelated", hasFlow && !relatedComponents.has(id));
     node.classList.toggle("is-flow-related", hasFlow && relatedComponents.has(id));
     node.classList.toggle("is-return-highlighted", !hasFlow && this.returnHighlightIDs.has(id));
   });

   this.structuralSVG.classList.remove("is-suppressed");
   this.stepElements.forEach((node, key) => {
    const itemFlow = key.split("\u0000", 1)[0];
    node.hidden = itemFlow !== flowID;
    node.classList.toggle("is-selected", key === flowStepKey(flowID, this.selection.step));
   });
   this.frontierElements.forEach((node, key) => {
    const itemFlow = key.split("\u0000", 1)[0];
    node.hidden = itemFlow !== flowID;
   });
   if (this.unassignedRail) {
    const hasUnassigned = hasFlow && (
     array(this.flowByID.get(flowID) && this.flowByID.get(flowID).steps).some((step) => !step.component_id) ||
     this.frontiers.some((frontier) => text(frontier.flow_id) === flowID)
    );
    this.unassignedRail.classList.toggle("is-visible", hasUnassigned);
   }

   this.structuralEdgeElements.forEach((group, id) => {
    const edge = this.structuralEdgeByID.get(id);
    const selectedEdge = id === this.selection.edge && !hasFlow;
    const incident = Boolean(
     !hasFlow &&
     this.selection.component &&
     edge &&
     (text(edge.from_component_id) === this.selection.component || text(edge.to_component_id) === this.selection.component)
    );
     setSVGVisible(group, !hasFlow);
     group.classList.toggle("is-selected", selectedEdge);
     group.classList.toggle("is-highlighted", incident);
     group.classList.toggle("is-muted", Boolean(this.selection.component || this.selection.edge) && !selectedEdge && !incident);
   });
   this.flowEdgeElements.forEach((group, key) => {
    const itemFlow = key.split("\u0000", 1)[0];
    const edge = this.flowEdgesByKey.get(key);
    const endpointsVisible = Boolean(
     edge &&
     this.stepElements.has(flowStepKey(itemFlow, edge.from)) &&
     this.stepElements.has(flowStepKey(itemFlow, edge.to))
    );
    setSVGVisible(group, itemFlow === flowID && endpointsVisible);
    group.classList.toggle("is-selected", key === selectionKey(flowID, this.selection.edge));
   });
   this.renderInspector();
  }

  renderInspector() {
   const visible = this.hasInspectorSelection(this.selection);
    this.root.classList.toggle("has-detail-inspector", visible);
    this.inspector.hidden = !visible;
    this.drawerBackdrop.hidden = !visible;
    this.inspector.replaceChildren();
    if (!visible) return;
    const close = element("button", "rm-arch__inspector-close", "×");
    close.type = "button";
    close.setAttribute("aria-label", "Close inspector");
    this.listen(close, "click", () => this.closeInspector());
    this.inspector.appendChild(close);
   if (this.selection.step && this.selection.flow) {
    const step = this.flowStepsByKey.get(flowStepKey(this.selection.flow, this.selection.step));
    if (step) return this.inspectStep(this.flowByID.get(this.selection.flow), step);
   }
    if (this.selection.edge) {
    const flowEdge = this.selection.flow && this.flowEdgesByKey.get(selectionKey(this.selection.flow, this.selection.edge));
    if (flowEdge) return this.inspectFlowEdge(flowEdge);
    const structuralEdge = this.structuralEdgeByID.get(this.selection.edge);
     if (structuralEdge) return this.inspectStructuralEdge(structuralEdge);
    }
    if (this.selection.surface) {
     const surface = this.surfaceByID.get(this.selection.surface);
     if (surface) return this.inspectSurface(surface);
    }
    if (this.selection.component) {
    const component = this.componentByID.get(this.selection.component);
    if (component) return this.inspectComponent(component);
   }
  }

  inspectorHeading(kind, title, subtitle) {
   this.inspector.appendChild(element("div", "rm-arch__inspector-kicker", kind));
   this.inspector.appendChild(element("h3", "rm-arch__inspector-title", title));
   if (subtitle) this.inspector.appendChild(element("p", "rm-arch__inspector-summary", subtitle));
  }

  inspectorSection(title) {
   const section = element("section", "rm-arch__inspector-section");
   section.appendChild(element("h4", "rm-arch__inspector-section-title", title));
   this.inspector.appendChild(section);
   return section;
  }

  inspectLandscape() {
   const hasFlows = this.flows.length > 0;
   this.inspectorHeading(
    "Architecture",
    "How to read this map",
     hasFlows ? "Select a component, surface, or saved trace." : "Select a component to inspect its exact local members."
   );
    const note = this.inspectorSection("Evidence semantics");
    note.appendChild(element("p", "rm-arch__copy", "Subsystem and component names are conceptual orientation. Quiet lines are witnessed structural relations, not runtime execution."));
    this.appendKeyValue(note, "Architecture source", architectureSourceLabel(this.data.architecture_source));
    this.appendKeyValue(note, "Grounding mode", text(this.data.grounding_mode).replaceAll("_", " "));
    this.appendKeyValue(note, "Architecture anchors", String(array(this.data.behavior_anchors).length));
   if (!hasFlows) {
     const flowState = this.inspectorSection("Saved traces");
    flowState.appendChild(element(
     "p",
     "rm-arch__notice is-warning",
     "No compatible saved FlowProof is available for this run. The landscape remains useful, but no runtime sequence is implied."
    ));
   }
   this.appendDiagnostics(this.inspectorSection("Diagnostics"), "");
  }

  inspectComponent(component) {
   const subsystem = this.subsystemByID.get(text(component.subsystem_id));
   this.inspectorHeading("Component", component.name || component.id, component.description);
   const purpose = this.inspectorSection("Purpose and grounding");
   if (component.description) purpose.appendChild(element("p", "rm-arch__copy", component.description));
   if (subsystem) this.appendKeyValue(purpose, "Subsystem", subsystem.name || subsystem.id);
   this.appendKeyValue(purpose, "Grounding", component.hypothesis ? "Conceptual / package-derived" : "Exact local membership");

   const surfaceIDs = Array.from(new Set(array(component.owned_surface_ids).concat(array(component.participating_surface_ids))));
   const surfaces = this.inspectorSection("Component surfaces");
   surfaces.appendChild(element(
    "p",
    "rm-arch__copy",
    "These surfaces share exact component or executable evidence. Only a saved trace's explicit seed/evidence surfaces participate in that trace."
   ));
   if (surfaceIDs.length === 0) {
     surfaces.appendChild(element("p", "rm-arch__empty", "No supported surface has a unique exact association with this component."));
   }
   surfaceIDs.forEach((surfaceID) => {
    const surface = this.surfaceByID.get(text(surfaceID));
    if (!surface) return;
    const button = element("button", "rm-arch__edge-jump");
    button.type = "button";
    button.appendChild(element("strong", null, surface.name || surface.id));
    button.appendChild(element("span", null, [surface.kind, text(surface.category).replaceAll("_", "/"), surface.status].filter(Boolean).join(" · ")));
    this.listen(button, "click", () => this.setSelection({ component: "", surface: text(surface.id), step: "", edge: "" }, true));
    surfaces.appendChild(button);
   });

   const traces = this.inspectorSection("Saved traces");
   const relatedFlowIDs = array(component.participating_flow_ids).filter((flowID) => this.flowByID.has(text(flowID)));
   if (relatedFlowIDs.length === 0) traces.appendChild(element("p", "rm-arch__empty", "No bounded saved trace crosses this component."));
   relatedFlowIDs.forEach((flowID) => {
    const flow = this.flowByID.get(text(flowID));
    const button = element("button", "rm-arch__edge-jump");
    button.type = "button";
    button.appendChild(element("strong", null, flow.name || flow.id));
    button.appendChild(element("span", null, [flow.status, flow.grounded_areas + "/" + flow.total_areas + " grounded"].filter(Boolean).join(" · ")));
    this.listen(button, "click", () => this.openTrace(flow.id));
    traces.appendChild(button);
   });

   const suggestions = this.inspectorSection("Suggested investigations");
   const suggestionIDs = array(component.suggested_investigation_ids);
   if (suggestionIDs.length === 0) suggestions.appendChild(element("p", "rm-arch__empty", "No untraced suggestion is attached by exact saved evidence."));
    suggestionIDs.forEach((directionID) => {
     const suggestion = this.suggestionByID.get(text(directionID));
     const direction = this.directionByID.get(text(directionID));
     if (!suggestion && !direction) return;
     const card = element("article", "rm-arch__evidence-card");
     card.appendChild(element("strong", "rm-arch__evidence-title", (suggestion && suggestion.title) || direction.name || direction.id));
     const reason = suggestion && suggestion.reason || direction.why_interesting;
     if (reason) card.appendChild(element("p", "rm-arch__copy", reason));
     this.appendKeyValue(card, "Status", "Suggested investigation — not a saved trace");
     if (suggestion) {
      this.appendKeyValue(card, "Grounding", text(suggestion.current_grounding).replaceAll("_", " "));
      this.appendKeyValue(card, "Trace start", suggestion.can_start_trace ? "Available" : "Unavailable from current local evidence");
      this.appendKeyValue(card, "Source investigation", suggestion.investigation_available ? "Exact source available" : "Unavailable");
      if (!suggestion.can_start_trace) {
       card.appendChild(element("p", "rm-arch__notice is-warning", "Trace unavailable: " +
        (suggestion.trace_unavailable_reason || "No singular supported typed trace seed was found.")));
      }
      if (suggestion.investigation_available && suggestion.start_location && typeof this.options.openLocation === "function") {
       const openSource = element("button", "rm-arch__edge-jump", "Open starting source");
       openSource.type = "button";
       this.listen(openSource, "click", () => {
        const location = suggestion.start_location;
        this.options.openLocation(location.path, location.line || 0, location.column || 0);
       });
       card.appendChild(openSource);
      } else {
       const exactSourceUnavailable = suggestion.investigation_available;
       card.appendChild(element("p", "rm-arch__notice is-warning", exactSourceUnavailable ? "Source action unavailable" : "Investigation unavailable"));
       const unavailableReason = exactSourceUnavailable ?
        "Open this static export through repomap <repo> to enable manifest-authorized source actions." :
        (suggestion.unavailable_reason || "No exact source starting point was found for this suggestion.");
       this.appendKeyValue(card, "Reason", unavailableReason);
      }
     }
     suggestions.appendChild(card);
   });

   const members = this.inspectorSection("Exact members");
   if (array(component.members).length === 0) {
    members.appendChild(element("p", "rm-arch__empty", "No exact members were retained."));
   }
   array(component.members).forEach((member) => {
    const card = element("article", "rm-arch__evidence-card");
    card.appendChild(element("strong", "rm-arch__evidence-title", member.name || memberLabel(member.id)));
    card.appendChild(element("code", "rm-arch__member-id", memberLabel(member.id)));
    array(member.facts).forEach((fact) => this.appendFact(card, fact));
    members.appendChild(card);
   });

   const evidence = this.inspectorSection("Evidence");
   const anchorIDs = array(component.anchor_ids);
   if (anchorIDs.length === 0) evidence.appendChild(element("p", "rm-arch__empty", "No exact architecture anchor is attached."));
   anchorIDs.forEach((anchorID) => {
      const anchor = this.anchorByID.get(text(anchorID));
      const card = element("article", "rm-arch__evidence-card");
      card.appendChild(element("strong", "rm-arch__evidence-title", anchor ? (anchor.label || anchor.kind) : anchorID));
      card.appendChild(element("code", "rm-arch__member-id", text(anchorID)));
      if (anchor) {
       this.appendKeyValue(card, "Kind", anchor.kind);
       this.appendKeyValue(card, "Certainty", anchor.certainty);
       this.appendLocation(card, anchor.location, "Anchor evidence");
       this.appendProvenance(card, anchor.producer ? [anchor.producer] : []);
      }
      evidence.appendChild(card);
   });

   const unknowns = this.inspectorSection("Unknowns");
   if (component.hypothesis) unknowns.appendChild(element("p", "rm-arch__notice is-warning", "This component remains a conceptual or package-derived hypothesis."));
   if (surfaceIDs.length === 0) unknowns.appendChild(element("p", "rm-arch__notice", "Zero configured-catalog surfaces is an honest bounded-analysis result, not proof that the component has no runtime surface."));
   this.appendDiagnostics(unknowns, "");
  }

  inspectSurface(surface) {
   this.inspectorHeading("Surface", surface.name || surface.id, "Deterministic registration/start evidence; runtime execution was not observed.");
   const ownership = this.inspectorSection("Ownership");
   this.appendKeyValue(ownership, "Executable", surface.owning_executable || "Unassigned");
   this.appendKeyValue(ownership, "Catalog group", text(surface.category).replaceAll("_", "/"));
   const owner = this.componentByID.get(text(surface.owning_component_id));
   if (owner) {
    const button = element("button", "rm-arch__edge-jump", owner.name || owner.id);
    button.type = "button";
    this.listen(button, "click", () => this.openComponent(owner.id));
    ownership.appendChild(button);
   } else {
    ownership.appendChild(element("p", "rm-arch__notice is-warning", "Unassigned surface — no unique exact component owner was found."));
   }

   const progression = this.inspectorSection("Saved trace");
   const trace = this.flowByID.get(text(surface.related_saved_trace_id));
   if (trace) {
    const button = element("button", "rm-arch__edge-jump");
    button.type = "button";
    button.appendChild(element("strong", null, "Open saved trace"));
    button.appendChild(element("span", null, [trace.name || trace.id, trace.status].filter(Boolean).join(" · ")));
    this.listen(button, "click", () => this.openTrace(trace.id));
    progression.appendChild(button);
   } else {
    progression.appendChild(element("p", "rm-arch__notice is-warning", "Trace unavailable: " + (surface.trace_unavailable_reason || "bounded surface seed is unavailable")));
   }

   const facts = this.inspectorSection("Evidence");
   this.appendKeyValue(facts, "Status", surface.status);
   this.appendKeyValue(facts, "Certainty", surface.certainty);
   this.appendKeyValue(facts, "Resolution", surface.resolution);
   array(surface.evidence).forEach((location) => this.appendLocation(facts, location, "Exact source"));
  }

  inspectFlow(flow) {
   this.inspectorHeading("Saved trace", flow.name || flow.id, flow.why_inspect || flow.mental_model || flow.goal);
   this.appendFlowEvidence(this.inspector, flow);
  }

  inspectStep(flow, step) {
   const branch = this.flowBranch(flow.id, step.branch_id);
   this.inspectorHeading("Flow step", step.label || step.id, step.qualified_name);
   this.appendKeyValue(this.inspector, "Flow", flow.name || flow.id);
   this.appendKeyValue(this.inspector, "Anchor kind", step.kind);
   this.appendKeyValue(this.inspector, "Branch", branch ? branch.kind : "unassigned");
   this.appendLocation(this.inspector, step.location, "Declaration");

   if (step.binding) {
    const binding = this.inspectorSection("Exact component binding");
    this.appendKeyValue(binding, "Member", memberLabel(step.binding.member_id));
    this.appendKeyValue(binding, "Certainty", step.binding.certainty);
    this.appendLocation(binding, step.binding.location, "Binding evidence");
    this.appendProvenance(binding, step.binding.provenance);
    this.appendScenarios(binding, step.binding.scenarios);
   } else {
    const missing = this.inspectorSection("Component binding");
    missing.appendChild(element("p", "rm-arch__notice is-warning", "No unique exact member binds this anchor to a component."));
   }

  }

  inspectFlowEdge(edge) {
   this.inspectorHeading("Flow transition", edge.relation || edge.id, text(edge.from) + " → " + text(edge.to));
   this.appendKeyValue(this.inspector, "Flow", edge.flow_id);
   this.appendKeyValue(this.inspector, "Resolution", edge.resolution);
   this.appendKeyValue(this.inspector, "Invocation", edge.invocation || "unspecified");
   this.appendKeyValue(this.inspector, "Certainty", edge.certainty);
   this.appendKeyValue(this.inspector, "Provider", edge.provider);
   this.appendKeyValue(this.inspector, "From branch", edge.from_branch_id || "unassigned");
   this.appendKeyValue(this.inspector, "To branch", edge.to_branch_id || "unassigned");
   this.appendKeyValue(this.inspector, "Cross branch", edge.cross_branch ? "yes" : "no");
   this.appendLocation(this.inspector, edge.evidence, "Callsite evidence");
   const source = this.flowStepsByKey.get(flowStepKey(edge.flow_id, edge.from));
   const target = this.flowStepsByKey.get(flowStepKey(edge.flow_id, edge.to));
   if (source) this.appendLocation(this.inspector, source.location, "Source declaration");
   if (target) this.appendLocation(this.inspector, target.location, "Target declaration");
   if (edge.condition) {
    const condition = this.inspectorSection("Source condition");
    const conditionText = edge.condition.expression || (
     edge.condition.expression_omitted ? "condition (expression omitted)" : "condition recorded"
    );
    condition.appendChild(element("code", "rm-arch__condition", conditionText));
    this.appendLocation(condition, edge.condition.location, "Condition location");
    condition.appendChild(element("p", "rm-arch__copy", "This preserves syntax; it does not claim the branch ran."));
   }
  }

  inspectStructuralEdge(edge) {
   const from = this.componentByID.get(text(edge.from_component_id));
   const to = this.componentByID.get(text(edge.to_component_id));
   const witness = edge.witness || {};
   this.inspectorHeading(
    "Structural evidence",
    witness.kind || edge.id,
    (from ? from.name : edge.from_component_id) + " → " + (to ? to.name : edge.to_component_id)
   );
   this.appendKeyValue(this.inspector, "Source member", memberLabel(witness.from));
   this.appendKeyValue(this.inspector, "Target member", memberLabel(witness.to));
   this.appendKeyValue(this.inspector, "Certainty", witness.certainty);
   this.appendKeyValue(this.inspector, "Witness ID", witness.id);
   this.appendLocation(this.inspector, witness.location, "Local witness");
   this.appendProvenance(this.inspector, witness.provenance);
   this.appendScenarios(this.inspector, witness.scenarios);
   this.inspector.appendChild(element("p", "rm-arch__notice", "Structural evidence does not imply runtime order or execution."));
  }

  stepJumpButton(flow, step) {
   const button = element("button", "rm-arch__edge-jump");
   button.type = "button";
   const branch = this.flowBranch(flow.id, step.branch_id);
   button.appendChild(element("strong", null, step.label || step.id));
   button.appendChild(element("span", null, branchLabel(branch && branch.kind)));
   this.listen(button, "click", () => this.setSelection({
    flow: text(flow.id), component: text(step.component_id), step: text(step.id), edge: "",
   }, true));
   return button;
  }

  appendFact(parent, fact) {
   if (!fact) return;
   const block = element("div", "rm-arch__fact");
   const heading = element("div", "rm-arch__fact-heading");
   heading.appendChild(element("span", "rm-arch__badge", fact.kind || "fact"));
   heading.appendChild(element("span", "rm-arch__badge is-muted", fact.certainty || "unknown"));
   block.appendChild(heading);
   if (fact.value) block.appendChild(element("span", "rm-arch__fact-value", fact.value));
   this.appendLocation(block, fact.location, "Evidence");
   this.appendProvenance(block, fact.provenance);
   parent.appendChild(block);
  }

  appendKeyValue(parent, key, value) {
   if (value == null || value === "") return;
   const row = element("div", "rm-arch__key-value");
   row.appendChild(element("span", "rm-arch__key", key));
   row.appendChild(element("span", "rm-arch__value", value));
   parent.appendChild(row);
  }

  appendSummaryItem(parent, key, value) {
   if (value == null || value === "") return;
   const row = element("div");
   row.appendChild(element("dt", null, key));
   row.appendChild(element("dd", null, value));
   parent.appendChild(row);
  }

  appendLocation(parent, location, label) {
   const formatted = locationLabel(location);
   if (!formatted) return;
   const row = element("div", "rm-arch__location-row");
   row.appendChild(element("span", "rm-arch__key", label || "Location"));
   const callback = this.options.openLocation;
   if (typeof callback === "function") {
    const button = element("button", "rm-arch__location", formatted + " ↗");
    button.type = "button";
     this.listen(button, "click", () => callback(text(location.path), Number(location.line || 0), Number(location.column || 0)));
     row.appendChild(button);
   } else {
    row.appendChild(element("code", "rm-arch__location-text", formatted));
   }
    parent.appendChild(row);
    if (this.options.stalePaths && this.options.stalePaths.has && this.options.stalePaths.has(text(location.path))) {
     parent.appendChild(element("p", "rm-arch__source-warning", "Source changed since this report was generated"));
    }
  }

  appendProvenance(parent, provenanceItems) {
   const items = array(provenanceItems);
   if (items.length === 0) return;
   const details = element("details", "rm-arch__details");
   details.appendChild(element("summary", null, "Provenance (" + items.length + ")"));
   const list = element("ul", "rm-arch__detail-list");
   items.forEach((provenance) => {
    const item = element("li");
    const description = [provenance.provider, provenance.operation, provenance.version, provenance.detail]
     .filter(Boolean)
     .join(" · ");
    item.appendChild(element("span", null, description || "unspecified provenance"));
    this.appendLocation(item, provenance.location, "Source");
    list.appendChild(item);
   });
   details.appendChild(list);
   parent.appendChild(details);
  }

  appendScenarios(parent, scenarios) {
   const items = array(scenarios);
   if (items.length === 0) return;
   const details = element("details", "rm-arch__details");
   details.appendChild(element("summary", null, "Scenarios (" + items.length + ")"));
   const list = element("ul", "rm-arch__detail-list");
   items.forEach((scenario) => {
    const item = element("li");
    item.appendChild(element("strong", null, scenario.name || scenario.id || "scenario"));
    const build = scenario.build || {};
    const context = [build.goos, build.goarch, array(build.build_tags).join(", ")].filter(Boolean).join(" · ");
    if (context) item.appendChild(element("code", null, context));
    list.appendChild(item);
   });
   details.appendChild(list);
   parent.appendChild(details);
  }

  appendDiagnostics(parent, flowID) {
   const diagnostics = this.diagnostics.filter((diagnostic) => !flowID || text(diagnostic.flow_id) === flowID);
   if (diagnostics.length === 0) {
    parent.appendChild(element("p", "rm-arch__empty", "No saved diagnostics."));
    return;
   }
   diagnostics.forEach((diagnostic) => {
    const notice = element("article", "rm-arch__notice is-" + (diagnostic.severity || "info"));
    notice.appendChild(element("strong", null, diagnostic.code || diagnostic.source || "diagnostic"));
    notice.appendChild(element("p", null, diagnostic.message));
    if (diagnostic.member) notice.appendChild(element("code", null, memberLabel(diagnostic.member)));
    parent.appendChild(notice);
   });
  }

  destroy() {
   if (this.destroyed) return;
   this.destroyed = true;
   this.events.abort();
   this.host.replaceChildren();
  }
 }

 function mount(host, data, options) {
  const app = new ArchitectureCanvasApp(host, data, options);
  const ready = app.start();
  return Object.freeze({
   ready: ready,
   fit: () => app.fit(),
   openTrace: (flowID) => app.openTrace(flowID),
   openSurface: (surfaceID) => app.openSurface(surfaceID),
   openComponent: (componentID) => app.openComponent(componentID),
   destroy: () => app.destroy(),
  });
 }

 global.RepomapArchitectureCanvas = Object.freeze({ mount: mount });
 if (global.__REPOMAP_LAYOUT_TEST__ && typeof global.__REPOMAP_LAYOUT_TEST__ === "object") {
  Object.assign(global.__REPOMAP_LAYOUT_TEST__, {
   landscapeLayoutMode: landscapeLayoutMode,
   boardProfileForWidth: boardProfileForWidth,
   shortestColumnIndex: shortestColumnIndex,
   childGridShape: childGridShape,
   shortestCompatiblePlacement: shortestCompatiblePlacement,
   diagnosticSubsystemIDs: diagnosticSubsystemIDs,
   readableFitScale: readableFitScale,
   componentFocusScale: componentFocusScale,
   centeredTransform: centeredTransform,
   savedTraceLabel: savedTraceLabel,
   lifecycleRelationHeading: lifecycleRelationHeading,
   groupLifecycleRelations: groupLifecycleRelations,
  });
 }
})(window);
