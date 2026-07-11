(function (global) {
 "use strict";

 const SVG_NS = "http://www.w3.org/2000/svg";
 const HASH_KEYS = ["flow", "component", "step", "edge"];
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
 const MIN_SCALE = 0.28;
 const MAX_SCALE = 2.4;

 function array(value) {
  return Array.isArray(value) ? value : [];
 }

 function text(value) {
  return value == null ? "" : String(value);
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

 function clamp(value, low, high) {
  return Math.min(high, Math.max(low, value));
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

 function semanticClass(relation, invocation) {
  const value = text(relation).toLowerCase();
  if (value.indexOf("cancel") >= 0) return "is-cancel";
  if (value.indexOf("join") >= 0 || value === "waits_for") return "is-join";
  if (value.indexOf("start") >= 0 || invocation === "goroutine") return "is-start";
  if (value.indexOf("callback") >= 0 || invocation === "callback") return "is-callback";
  return "is-call";
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
   this.events = new AbortController();
   this.view = { x: 0, y: 0, scale: 1 };
   this.drag = null;
   this.selection = {
    flow: "",
    component: "",
    step: "",
    edge: "",
   };

   this.subsystems = array(this.data.subsystems);
   this.components = array(this.data.components);
   this.structuralEdges = array(this.data.structural_edges);
   this.flows = array(this.data.flows);
   this.flowEdges = array(this.data.flow_edges);
   this.frontiers = array(this.data.frontiers);
   this.diagnostics = array(this.data.diagnostics);

   this.componentByID = new Map();
   this.subsystemByID = new Map();
   this.structuralEdgeByID = new Map();
   this.flowByID = new Map();
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

   this.indexData();
   this.buildShell();
   this.restoreHash(false);
  }

  indexData() {
   this.subsystems.forEach((subsystem) => {
    if (subsystem && subsystem.id) this.subsystemByID.set(text(subsystem.id), subsystem);
   });
   this.components.forEach((component) => {
    if (component && component.id) this.componentByID.set(text(component.id), component);
   });
   this.structuralEdges.forEach((edge) => {
    if (edge && edge.id) this.structuralEdgeByID.set(text(edge.id), edge);
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
   this.landscapeButton = element("button", "rm-arch__flow-button is-active", "Landscape");
   this.landscapeButton.type = "button";
   this.listen(this.landscapeButton, "click", () => {
    this.setSelection({ flow: "", component: "", step: "", edge: "" }, true);
   });
   flowNav.appendChild(this.landscapeButton);
   this.flows.forEach((flow) => {
    const button = element("button", "rm-arch__flow-button", flow.name || flow.id);
    button.type = "button";
    this.listen(button, "click", () => {
     this.setSelection({ flow: text(flow.id), step: "", edge: "" }, true);
    });
    this.flowButtons.set(text(flow.id), button);
    flowNav.appendChild(button);
   });
   toolbar.appendChild(flowNav);

   const controls = element("div", "rm-arch__controls");
   this.zoomOutButton = this.controlButton("−", "Zoom out", () => this.zoomBy(0.82));
   this.fitButton = this.controlButton("Fit", "Fit architecture in view", () => this.fit());
   this.zoomInButton = this.controlButton("+", "Zoom in", () => this.zoomBy(1.22));
   controls.append(this.zoomOutButton, this.fitButton, this.zoomInButton);
   toolbar.appendChild(controls);
   this.root.appendChild(toolbar);

   const workspace = element("div", "rm-arch__workspace");
   this.viewport = element("div", "rm-arch__viewport");
   this.loading = element("div", "rm-arch__loading", "Laying out the saved architecture…");
   this.viewport.appendChild(this.loading);
   workspace.appendChild(this.viewport);

   this.inspector = element("aside", "rm-arch__inspector");
   workspace.appendChild(this.inspector);
   this.root.appendChild(workspace);

   this.host.appendChild(this.root);
   this.installViewportInteractions();
   this.listen(global, "hashchange", () => this.restoreHash(true));
  }

  controlButton(label, title, handler) {
   const button = element("button", "rm-arch__control", label);
   button.type = "button";
   this.listen(button, "click", handler);
   return button;
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
     requestAnimationFrame(() => this.fit());
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
    return Promise.reject(new Error("ELK.js is unavailable; showing deterministic fallback positions"));
   }
   const elk = new ELKConstructor();
   return elk.layout(this.buildELKGraph()).then((layout) => this.normalizeELKLayout(layout));
  }

  maxUnassignedItems() {
   let maximum = 0;
   this.flows.forEach((flow) => {
    const flowID = text(flow.id);
    const steps = array(flow.steps).filter((step) => !step.component_id).length;
    const frontiers = this.frontiers.filter((frontier) => text(frontier.flow_id) === flowID).length;
    maximum = Math.max(maximum, steps + frontiers);
   });
   return Math.min(MAX_VISIBLE_CHIPS, maximum);
  }

  buildELKGraph() {

   const componentIDsInSubsystems = new Set();
   const children = [];
   this.subsystems.forEach((subsystem) => {
    const subsystemID = text(subsystem.id);
    const componentIDs = array(subsystem.component_ids)
     .map(text)
     .filter((id) => this.componentByID.has(id));
    if (componentIDs.length === 0) return;
    componentIDs.forEach((id) => componentIDsInSubsystems.add(id));
    children.push(this.elkSubsystemNode(subsystemID, componentIDs));
   });

   const ungrouped = this.components
    .map((component) => text(component.id))
    .filter((id) => !componentIDsInSubsystems.has(id));
   if (ungrouped.length > 0) {
    children.push(this.elkSubsystemNode("__ungrouped__", ungrouped));
   }

   const unassignedCount = this.maxUnassignedItems();
   if (unassignedCount > 0) {
    children.push({
     id: UNASSIGNED_ID,
     width: COMPONENT_WIDTH,
     height: COMPONENT_HEIGHT,
     layoutOptions: { "elk.nodeLabels.placement": "INSIDE V_TOP H_LEFT" },
    });
   }

   const edges = [];
   this.structuralEdges.forEach((edge) => {
    const from = text(edge.from_component_id);
    const to = text(edge.to_component_id);
    if (!this.componentByID.has(from) || !this.componentByID.has(to) || from === to) return;
    const id = layoutStructuralEdgeID(edge.id);
    edges.push({ id: id, sources: [layoutComponentID(from)], targets: [layoutComponentID(to)] });
   });

   this.flowEdges.forEach((edge) => {
    const fromOwner = this.flowStepOwner(edge.flow_id, edge.from);
    const toOwner = this.flowStepOwner(edge.flow_id, edge.to);
    if (!fromOwner || !toOwner || fromOwner === toOwner) return;
    const id = layoutFlowEdgeID(edge.flow_id, edge.id);
    edges.push({
     id: id,
     sources: [fromOwner === UNASSIGNED_ID ? UNASSIGNED_ID : layoutComponentID(fromOwner)],
     targets: [toOwner === UNASSIGNED_ID ? UNASSIGNED_ID : layoutComponentID(toOwner)],
    });
   });

   return {
    id: "repomap-architecture-root",
    layoutOptions: {
     "elk.algorithm": "layered",
     "elk.direction": "RIGHT",
     "elk.edgeRouting": "ORTHOGONAL",
     "elk.hierarchyHandling": "INCLUDE_CHILDREN",
     "elk.spacing.nodeNode": "44",
     "elk.layered.spacing.nodeNodeBetweenLayers": "92",
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

  elkSubsystemNode(subsystemID, componentIDs) {
   return {
    id: layoutSubsystemID(subsystemID),
    layoutOptions: {
     "elk.algorithm": "layered",
     "elk.direction": "DOWN",
     "elk.edgeRouting": "ORTHOGONAL",
     "elk.spacing.nodeNode": "24",
     "elk.padding": "[top=" + GROUP_HEADER + ",left=" + GROUP_PADDING + ",bottom=" + GROUP_PADDING + ",right=" + GROUP_PADDING + "]",
    },
    children: componentIDs.map((componentID) => {
     return {
      id: layoutComponentID(componentID),
      width: COMPONENT_WIDTH,
      height: COMPONENT_HEIGHT,
     };
    }),
   };
  }

  flowStepOwner(flowID, stepID) {
   const step = this.flowStepsByKey.get(flowStepKey(flowID, stepID));
   if (!step) return "";
   const componentID = text(step.component_id);
   return componentID && this.componentByID.has(componentID) ? componentID : UNASSIGNED_ID;
  }

  normalizeELKLayout(layout) {
   this.nodePositions.clear();
   this.groupPositions.clear();
   this.edgeRoutes.clear();

   const walk = (node, parentX, parentY) => {
    const x = parentX + Number(node.x || 0);
    const y = parentY + Number(node.y || 0);
    const id = text(node.id);
    if (id.indexOf("component:") === 0) {
     this.nodePositions.set(id.slice("component:".length), {
      x: x,
      y: y,
      width: Number(node.width || COMPONENT_WIDTH),
      height: Number(node.height || 100),
     });
    } else if (id.indexOf("subsystem:") === 0) {
     this.groupPositions.set(id.slice("subsystem:".length), {
      x: x,
      y: y,
      width: Number(node.width || 0),
      height: Number(node.height || 0),
     });
    } else if (id === UNASSIGNED_ID) {
     this.nodePositions.set(UNASSIGNED_ID, {
      x: x,
      y: y,
      width: Number(node.width || COMPONENT_WIDTH),
      height: Number(node.height || 120),
     });
    }

    array(node.edges).forEach((edge) => {
     const route = pathFromSections(edge.sections, x, y);
     if (route) this.edgeRoutes.set(text(edge.id), route);
    });
    array(node.children).forEach((child) => walk(child, x, y));
   };
   walk(layout, 0, 0);

   return {
    width: Math.max(1, Number(layout.width || 1)),
    height: Math.max(1, Number(layout.height || 1)),
   };
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
    const group = element("section", "rm-arch__group");
    group.style.left = position.x + "px";
    group.style.top = position.y + "px";
    group.style.width = position.width + "px";
    group.style.height = position.height + "px";
    const title = element("h3", "rm-arch__group-title", subsystem.name || subsystem.id);
    group.appendChild(title);
    this.groupLayer.appendChild(group);
   });

   const ungrouped = this.groupPositions.get("__ungrouped__");
   if (ungrouped) {
    const group = element("section", "rm-arch__group is-ungrouped");
    group.style.left = ungrouped.x + "px";
    group.style.top = ungrouped.y + "px";
    group.style.width = ungrouped.width + "px";
    group.style.height = ungrouped.height + "px";
    group.appendChild(element("h3", "rm-arch__group-title", "Unassigned subsystem"));
    this.groupLayer.appendChild(group);
   }
  }

  renderComponents() {
   this.components.forEach((component) => {
    const id = text(component.id);
    const position = this.nodePositions.get(id);
    if (!position) return;
    const shell = element("article", "rm-arch__component");
    shell.style.left = position.x + "px";
    shell.style.top = position.y + "px";
    shell.style.width = position.width + "px";
    shell.style.height = position.height + "px";

    const button = element("button", "rm-arch__component-card");
    button.type = "button";
    button.appendChild(element("strong", "rm-arch__component-name", component.name || id));
    this.listen(button, "click", () => {
     this.setSelection({ component: id, step: "", edge: "" }, true);
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
   this.structuralEdges.forEach((edge) => {
    const route = this.edgeRoutes.get(layoutStructuralEdgeID(edge.id));
    if (!route) return;
    const group = this.interactiveSVGPath(
     route,
     "rm-arch__edge rm-arch__edge--structural",
     () => this.setSelection({ edge: text(edge.id), step: "" }, true)
    );
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
     : this.edgeRoutes.get(layoutFlowEdgeID(edge.flow_id, edge.id));
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
     () => this.setSelection({ flow: text(edge.flow_id), edge: text(edge.id), step: "" }, true)
    );
    group.hidden = true;
    const visible = group.querySelector(".rm-arch__edge-visible");
    if (semantic === "is-cancel") visible.setAttribute("marker-end", "url(#rm-arch-arrow-cancel)");
    else if (semantic === "is-join") visible.setAttribute("marker-end", "url(#rm-arch-arrow-join)");
    else visible.setAttribute("marker-end", "url(#rm-arch-arrow)");
    this.flowSVG.appendChild(group);
    this.flowEdgeElements.set(key, group);
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

  interactiveSVGPath(route, className, handler) {
   const group = svgElement("g", { class: className });
   group.appendChild(svgElement("path", { class: "rm-arch__edge-hit", d: route }));
   group.appendChild(svgElement("path", { class: "rm-arch__edge-visible", d: route }));
   this.listen(group, "click", (event) => {
    event.stopPropagation();
    handler();
   });
   return group;
  }

  installViewportInteractions() {
   this.listen(this.viewport, "wheel", (event) => {
    if (!this.surface) return;
    event.preventDefault();
    const factor = event.deltaY > 0 ? 0.88 : 1.14;
    this.zoomBy(factor);
   }, { passive: false });

   this.listen(this.viewport, "pointerdown", (event) => {
    if (!this.surface || event.button !== 0) return;
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
   const rect = this.viewport.getBoundingClientRect();
   if (rect.width < 10 || rect.height < 10) return;
   const padding = 28;
   const scale = clamp(
    Math.min(
     (rect.width - padding * 2) / this.layoutResult.width,
     (rect.height - padding * 2) / this.layoutResult.height
    ),
    MIN_SCALE,
    1.35
   );
   this.view.scale = scale;
   this.view.x = (rect.width - this.layoutResult.width * scale) / 2;
   this.view.y = (rect.height - this.layoutResult.height * scale) / 2;
   this.applyView();
  }

  applyView() {
   if (!this.surface) return;
   this.surface.style.transform = "translate(" + this.view.x + "px," + this.view.y + "px) scale(" + this.view.scale + ")";
   this.root.style.setProperty("--rm-arch-scale", this.view.scale);
  }

  setSelection(patch, writeHash) {
   const next = Object.assign({}, this.selection, patch || {});
   this.selection = this.validateSelection(next);
   if (writeHash) this.writeHash();
   this.renderSelection();
  }

  validateSelection(selection) {
   const next = {
    flow: text(selection.flow),
    component: text(selection.component),
    step: text(selection.step),
    edge: text(selection.edge),
   };
   if (next.flow && !this.flowByID.has(next.flow)) next.flow = "";
   if (next.component && !this.componentByID.has(next.component)) next.component = "";

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
   this.flowButtons.forEach((button, id) => button.classList.toggle("is-active", id === flowID));

   const relatedComponents = this.flowComponentIDs.get(flowID) || new Set();
   this.componentElements.forEach((node, id) => {
    node.classList.toggle("is-selected", id === this.selection.component);
    node.classList.toggle("is-unrelated", hasFlow && !relatedComponents.has(id));
    node.classList.toggle("is-flow-related", hasFlow && relatedComponents.has(id));
   });

   this.structuralSVG.classList.toggle("is-suppressed", hasFlow);
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
    group.classList.toggle("is-selected", id === this.selection.edge && !hasFlow);
   });
   this.flowEdgeElements.forEach((group, key) => {
    const itemFlow = key.split("\u0000", 1)[0];
    group.hidden = itemFlow !== flowID;
    group.classList.toggle("is-selected", key === selectionKey(flowID, this.selection.edge));
   });
   this.renderInspector();
  }

  renderInspector() {
   this.inspector.replaceChildren();
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
   if (this.selection.component) {
    const component = this.componentByID.get(this.selection.component);
    if (component) return this.inspectComponent(component);
   }
   if (this.selection.flow) {
    const flow = this.flowByID.get(this.selection.flow);
    if (flow) return this.inspectFlow(flow);
   }
   this.inspectLandscape();
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
   this.inspectorHeading("Architecture", "How to read this map", "Select a component or choose one saved flow.");
   const note = this.inspectorSection("Evidence semantics");
   note.appendChild(element("p", "rm-arch__copy", "Subsystem and component names are conceptual orientation. Quiet lines are witnessed structural relations, not runtime execution."));
   if (this.data.fallback) {
    const fallback = this.inspectorSection("Fallback state");
    fallback.appendChild(element("p", "rm-arch__notice is-warning", text(this.data.fallback_reason) || "The provider-independent landscape fallback was used."));
   }
   this.appendDiagnostics(this.inspectorSection("Diagnostics"), "");
  }

  inspectComponent(component) {
   const subsystem = this.subsystemByID.get(text(component.subsystem_id));
   this.inspectorHeading("Component", component.name || component.id, component.description);
   if (subsystem) this.appendKeyValue(this.inspector, "Subsystem", subsystem.name || subsystem.id);

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
   this.appendDiagnostics(this.inspectorSection("Diagnostics"), "");
  }

  inspectFlow(flow) {
   this.inspectorHeading("Saved flow", flow.name || flow.id, flow.mental_model || flow.goal);
   if (flow.trigger) this.appendKeyValue(this.inspector, "Trigger", flow.trigger);
   if (flow.scope) this.appendKeyValue(this.inspector, "Scope", flow.scope);
   if (flow.command) this.appendKeyValue(this.inspector, "Command", flow.command);

   const branches = this.inspectorSection("Branches");
   array(flow.branches).forEach((branch) => {
    const row = element("div", "rm-arch__branch-row " + branchClass(branch.kind));
    row.appendChild(element("strong", null, branch.kind || "unassigned"));
    row.appendChild(element("span", null, array(branch.anchor_ids).length + " steps"));
    if (branch.root_anchor_id) row.appendChild(element("code", null, branch.root_anchor_id));
    branches.appendChild(row);
   });

   const steps = this.inspectorSection("Exact steps");
   array(flow.steps).forEach((step) => steps.appendChild(this.stepJumpButton(flow, step)));

   const slots = this.inspectorSection("Proof slots");
   array(flow.slots).forEach((slot) => {
    const card = element("article", "rm-arch__slot is-" + (text(slot.status) || "unknown"));
    const header = element("div", "rm-arch__slot-header");
    header.appendChild(element("strong", null, slot.kind));
    header.appendChild(element("span", "rm-arch__badge", slot.status));
    card.appendChild(header);
    if (slot.summary) card.appendChild(element("p", "rm-arch__copy", slot.summary));
    if (slot.missing) card.appendChild(element("p", "rm-arch__notice is-warning", slot.missing));
    if (slot.applicability_reason) this.appendKeyValue(card, "Applicability", slot.applicability_reason);
    this.appendProvenance(card, slot.provenance);
    slots.appendChild(card);
   });
   this.appendFlowFrontiers(flow.id);
   this.appendDiagnostics(this.inspectorSection("Diagnostics"), text(flow.id));
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
   if (edge.condition) {
    const condition = this.inspectorSection("Source condition");
    condition.appendChild(element("code", "rm-arch__condition", edge.condition.expression || "condition recorded"));
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

  appendFlowFrontiers(flowID) {
   const frontiers = this.frontiers.filter((frontier) => text(frontier.flow_id) === text(flowID));
   const section = this.inspectorSection("Unresolved frontiers");
   if (frontiers.length === 0) {
    section.appendChild(element("p", "rm-arch__empty", "No explicit frontier was saved for this flow."));
    return;
   }
   frontiers.forEach((frontier) => {
    const row = element("div", "rm-arch__frontier-row");
    row.appendChild(element("strong", null, frontier.kind || frontier.id));
    row.appendChild(element("span", null, frontier.reason));
    this.appendLocation(row, frontier.evidence, "Known evidence");
    section.appendChild(row);
   });
  }

  stepJumpButton(flow, step) {
   const button = element("button", "rm-arch__edge-jump");
   button.type = "button";
   const branch = this.flowBranch(flow.id, step.branch_id);
   button.appendChild(element("strong", null, step.label || step.id));
   button.appendChild(element("span", null, branch ? branch.kind + " branch" : "unassigned branch"));
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

  appendLocation(parent, location, label) {
   const formatted = locationLabel(location);
   if (!formatted) return;
   const row = element("div", "rm-arch__location-row");
   row.appendChild(element("span", "rm-arch__key", label || "Location"));
   const callback = this.options.openLocation;
   if (typeof callback === "function") {
    const button = element("button", "rm-arch__location", formatted + " ↗");
    button.type = "button";
    this.listen(button, "click", () => callback(text(location.path), Number(location.line || 0)));
    row.appendChild(button);
   } else {
    row.appendChild(element("code", "rm-arch__location-text", formatted));
   }
   parent.appendChild(row);
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
  destroy: () => app.destroy(),
 });
 }

 global.RepomapArchitectureCanvas = Object.freeze({ mount: mount });
})(window);
