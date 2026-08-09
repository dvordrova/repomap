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
 const FIT_MAX_SCALE = 1.35;
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

 function participatingComponentIDs(record, componentByID) {
  const ids = [];
  const seen = new Set();
  array(record && record.participating_component_ids).forEach((value) => {
   const id = text(value);
   if (!id || seen.has(id) || !componentByID.has(id)) return;
   seen.add(id);
   ids.push(id);
  });
  return ids;
 }

 function architectureStepComponentState(step, componentByID) {
  const owner = text(step && step.component_id);
  const exactOwner = owner && componentByID.has(owner) ? owner : "";
  const participants = participatingComponentIDs(step, componentByID);
  const related = new Set(participants);
  if (exactOwner) related.add(exactOwner);
  return {
   owner: exactOwner,
   participants: participants,
   related: Array.from(related),
   lane: exactOwner || (participants.length === 1 ? participants[0] : UNASSIGNED_ID),
   selection: exactOwner || (participants.length === 1 ? participants[0] : ""),
  };
 }

 function productMessage(message, id, params) {
  const api = typeof message === "function"
   ? message
   : global.RepomapUI && typeof global.RepomapUI.message === "function"
    ? global.RepomapUI.message
    : null;
  if (!api) throw new TypeError("Repomap Architecture UI message API is unavailable");
  return api(id, params);
 }

 function architectureProvenanceProductMessageID(provenance) {
  switch (text(provenance && provenance.provider) + "\u0000" + text(provenance && provenance.operation)) {
   case "architecture_grounding\u0000behavior_anchor_file": return "architecture.provenance.behavior_anchor_file";
   case "architecture_grounding\u0000behavior_anchor_member": return "architecture.provenance.behavior_anchor_member";
   case "surface_catalog\u0000exact_process_entry_role": return "architecture.provenance.exact_process_entry_role";
   case "report_repository_graph\u0000saved_package_import": return "architecture.provenance.saved_package_import";
   case "report_repository_graph\u0000saved_package_member": return "architecture.provenance.saved_package_member";
   case "flowproof\u0000saved_proof": return "architecture.provenance.saved_proof";
   case "flowproof\u0000saved_flow_member": return "architecture.provenance.saved_flow_member";
   case "flowproof\u0000anchor_file": return "architecture.provenance.anchor_file";
   case "flowproof\u0000anchor_declaration": return "architecture.provenance.anchor_declaration";
   case "flowproof\u0000bind_anchor_to_exact_member": return "architecture.provenance.bind_anchor_to_exact_member";
   case "flowproof\u0000anchor_flow_participation": return "architecture.provenance.anchor_flow_participation";
   default: return "";
  }
 }

 function architectureScenarioProductMessageID(scenario) {
  if (text(scenario && scenario.id) === "saved-package-graph") {
   return "architecture.scenario.saved_package_graph";
  }
  const build = scenario && scenario.build || {};
  if (text(build.goos) || text(build.goarch) || array(build.build_tags).length > 0) {
   return "architecture.scenario.recorded_go_build";
  }
  return "";
 }

 function architecturePresentationText(lookup, address, fallback) {
  return lookup && Object.prototype.hasOwnProperty.call(lookup, address)
   ? text(lookup[address])
   : fallback;
 }

 function architectureSourceLabel(value, message) {
  switch (text(value)) {
   case "validated_model": return productMessage(message, "architecture.value.validated_model");
   case "partial_model": return productMessage(message, "architecture.value.partial_model");
   case "normalized_model": return productMessage(message, "architecture.value.normalized_model");
   case "local_anchors": return productMessage(message, "architecture.value.local_anchors");
   case "local_packages": return productMessage(message, "architecture.value.local_packages");
   case "package_fallback": return productMessage(message, "architecture.value.package_fallback");
   default: return productMessage(message, "architecture.value.unspecified");
  }
 }

 function architectureGroundingWording(source, mode, message) {
  if (text(source) === "local_anchors") {
   return {
    title: productMessage(message, "architecture.grounding.local_anchors.title"),
    subtitle: productMessage(message, "architecture.grounding.local_anchors.subtitle"),
   };
  }
  if (text(source) === "local_packages") {
   return {
    title: productMessage(message, "architecture.grounding.local_packages.title"),
    subtitle: productMessage(message, "architecture.grounding.local_packages.subtitle"),
   };
  }
  if (text(source) === "package_fallback") {
   return {
    title: productMessage(message, "architecture.grounding.package_fallback.title"),
    subtitle: productMessage(message, "architecture.grounding.package_fallback.subtitle"),
   };
  }
  switch (text(mode)) {
   case "behavior_grounded":
    return {
     title: productMessage(message, "architecture.grounding.behavior.title"),
     subtitle: productMessage(message, "architecture.grounding.behavior.subtitle"),
    };
   case "mixed":
    return {
     title: productMessage(message, "architecture.grounding.mixed.title"),
     subtitle: productMessage(message, "architecture.grounding.mixed.subtitle"),
    };
   default:
    return {
     title: productMessage(message, "architecture.grounding.conceptual.title"),
     subtitle: productMessage(message, "architecture.grounding.conceptual.subtitle"),
    };
  }
 }

 // The backend's package landscape is a closed deterministic projection, not
 // model-authored prose. Its component names are exact repository labels and
 // stay byte-preserved; only the generated package grouping copy is product
 // presentation. The source/level pair is the backend-validated closed type:
 // anchor-first local output is local_anchors/3, while accepted model output
 // has a model source and level 1 or 2 even when its grounding is packages.
 function deterministicPackageLandscape(data) {
  const source = text(data && data.architecture_source);
  return (source === "local_packages" || source === "package_fallback") &&
   Number(data && data.architecture_level) === 4;
 }

 function deterministicPackageComponent(component) {
  return array(component && component.members).some((member) => (
   text(member && member.id && member.id.kind) === "package"
  ));
 }

 function projectArchitectureUserPresentation(data, message) {
  const source = data && typeof data === "object" ? data : {};
  if (!deterministicPackageLandscape(source)) return source;

  const packageComponentIDs = new Set();
  const components = array(source.components).map((component) => {
   if (!deterministicPackageComponent(component)) return component;
   packageComponentIDs.add(text(component.id));
   return Object.assign({}, component, {
    description: productMessage(message, "architecture.fallback.package_group.description"),
   });
  });
  const subsystems = array(source.subsystems).map((subsystem) => {
   const componentIDs = array(subsystem && subsystem.component_ids).map(text).filter(Boolean);
   if (componentIDs.length === 0 || !componentIDs.every((id) => packageComponentIDs.has(id))) {
    return subsystem;
   }
   return Object.assign({}, subsystem, {
    name: productMessage(message, "architecture.fallback.package_group.name"),
    description: productMessage(message, "architecture.fallback.package_group.description"),
   });
  });
  return Object.assign({}, source, { components: components, subsystems: subsystems });
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
  // Decision 229 D1: SVG edge groups are passive visual evidence — no
  // tabindex (keyboard reachability lives on the real HTML buttons: flow
  // step nodes and connection rows). Legacy groups without a tabindex
  // contract never become focus targets.
 }

 function clamp(value, low, high) {
  return Math.min(high, Math.max(low, value));
 }

 function mapStructuralEdges(data) {
  return array(data && data.structural_edges).filter((edge) => (
   text(edge && edge.from_component_id) && text(edge && edge.to_component_id) &&
   text(edge.from_component_id) !== text(edge.to_component_id)
  ));
 }

 function landscapeLayoutMode(projection) {
  if (!projection || !projection.primaryRegion) return "board";
  if (projection.primaryRegion.groupIDs.length === projection.groups.length) return "graph";
  return "hybrid";
 }

 // Decision 236 (v11): the pure lens projection. It answers, from the
 // backend-owned view-model only:
 //   - which principal component IDs a lens emphasizes (exact set), and
 //   - which first-class objects the lens makes visible (entry categories,
 //     exact entry handoff context, and touchpoint families).
 // It never touches the DOM, never reads geometry, and never guesses from
 // renderer state — the backend owns every published identity and join.
 function currentEntrypointLocation(value) {
  return value && typeof value === "object" && !Array.isArray(value) &&
   text(value.path) && Number(value.line) > 0;
 }

 function currentEntrypointTransition(value) {
  return currentEntrypointLocation(value) && Array.isArray(value.component_ids);
 }

 function currentEntrypointHandoffGroup(value) {
  const entry = value && value.entry;
  const frontier = value && value.frontier;
  return Number(value && value.version) === 2 && text(value && value.id) &&
   Array.isArray(value.component_ids) &&
   currentEntrypointTransition(entry) &&
   text(entry.claim_kind) === "process_entry" &&
   Array.isArray(value.entry_handoffs) && value.entry_handoffs.length > 0 &&
   value.entry_handoffs.every((handoff) => (
    currentEntrypointTransition(handoff) &&
    handoff.target && currentEntrypointLocation(handoff.target)
   )) &&
   frontier && typeof frontier === "object" && !Array.isArray(frontier) &&
   text(frontier.ordering) && text(frontier.limitation);
 }

 // Canvas 15/group v2 publishes backend-owned component ownership on the
 // entry and every D210 handoff. This projection consumes those IDs directly.
 // It deliberately has no symbol/path/member fallback: zero or plural endpoint
 // ownership belongs in the side overflow. Participation highlights every
 // exact ID, but only unique→unique authority between distinct components
 // can draw an arrow. The browser never picks a sorted-first owner or fabricates a Cartesian
 // component relation.
 function entryHandoffOverlayProjection(group, knownComponentIDs) {
  const empty = {
   group_id: "", entry: null, component_ids: [], edges: [],
   overflow: [], frontier: null,
  };
  if (!currentEntrypointHandoffGroup(group)) return empty;
  const restrictToKnown = Array.isArray(knownComponentIDs);
  const known = new Set(array(knownComponentIDs).map(text).filter(Boolean));
  const accept = (value) => {
   const componentID = text(value);
   return componentID && (!restrictToKnown || known.has(componentID));
  };
  const exactIDs = (transition) => Array.from(new Set(
   array(transition && transition.component_ids).map(text).filter(accept)
  ));
  const entryIDs = exactIDs(group.entry);
  const componentIDs = [];
  const edges = [];
  const overflow = [];
  const addComponent = (componentID) => {
   if (componentIDs.indexOf(componentID) < 0) componentIDs.push(componentID);
  };
  entryIDs.forEach(addComponent);
  array(group.entry_handoffs).forEach((handoff, handoffIndex) => {
   const targetIDs = exactIDs(handoff);
   targetIDs.forEach(addComponent);
   if (entryIDs.length !== 1 || targetIDs.length !== 1) {
    let reason = "";
    if (entryIDs.length === 0) reason = "entry_unjoined";
    else if (targetIDs.length === 0) reason = "target_unjoined";
    else if (entryIDs.length > 1 && targetIDs.length > 1) reason = "entry_target_plural";
    else if (entryIDs.length > 1) reason = "entry_plural";
    else reason = "target_plural";
    overflow.push({
     id: text(group.id) + ":overflow:" + handoffIndex,
     reason: reason,
     handoff: handoff,
    });
    return;
   }
   const item = {
    id: text(group.id) + ":handoff:" + handoffIndex,
    from_component_id: entryIDs[0],
    to_component_id: targetIDs[0],
    handoff: handoff,
   };
   if (entryIDs[0] === targetIDs[0]) {
    overflow.push({
     id: text(group.id) + ":overflow:" + handoffIndex,
     reason: "same_component",
     handoff: handoff,
    });
    return;
   }
   edges.push(item);
  });
  return {
   group_id: text(group.id),
   entry: group.entry,
   component_ids: componentIDs,
   edges: edges,
   overflow: overflow,
   frontier: group.frontier,
  };
 }

 // D246: one published Study mechanism is already a backend-validated,
 // ordered direct-call path. The browser consumes only its public node/edge
 // projection and exact component_ids. It never repairs identities or picks
 // one owner from a plural set. Every exact participant can be highlighted;
 // only unique -> unique joins between distinct components become arrows.
 function currentStudyMechanism(value) {
  if (!value || typeof value !== "object" || Array.isArray(value) || !text(value.id)) return false;
  const nodes = array(value.nodes);
  const edges = array(value.edges);
  if (nodes.length < 3 || edges.length < 2 || edges.length !== nodes.length - 1) return false;
  const nodeIDs = new Set();
  for (let index = 0; index < nodes.length; index++) {
   const node = nodes[index];
   if (!node || !text(node.id) || !text(node.label) || !Array.isArray(node.component_ids) ||
    nodeIDs.has(text(node.id))) return false;
   nodeIDs.add(text(node.id));
  }
  for (let index = 0; index < edges.length; index++) {
   const edge = edges[index];
   if (!edge || !text(edge.id) || !nodeIDs.has(text(edge.from_node_id)) ||
    !nodeIDs.has(text(edge.to_node_id)) ||
    ["synchronous", "goroutine", "deferred"].indexOf(text(edge.invocation)) < 0) return false;
  }
  return true;
 }

 function studyMechanismOverlayProjection(mechanism, knownComponentIDs) {
  const empty = { mechanism_id: "", component_ids: [], edges: [], side_rows: [] };
  if (!currentStudyMechanism(mechanism)) return empty;
  const restrictToKnown = Array.isArray(knownComponentIDs);
  const known = new Set(array(knownComponentIDs).map(text).filter(Boolean));
  const accept = (value) => {
   const componentID = text(value);
   return componentID && (!restrictToKnown || known.has(componentID));
  };
  const nodeByID = new Map(array(mechanism.nodes).map((node) => [text(node.id), node]));
  const componentIDs = [];
  const exactIDs = (node) => Array.from(new Set(
   array(node && node.component_ids).map(text).filter(Boolean)
  ));
  const joinedIDs = (node) => exactIDs(node).filter(accept);
  array(mechanism.nodes).forEach((node) => {
   joinedIDs(node).forEach((componentID) => {
    if (componentIDs.indexOf(componentID) < 0) componentIDs.push(componentID);
   });
  });
  const edges = [];
  const sideRows = [];
  array(mechanism.edges).forEach((edge) => {
   const fromNode = nodeByID.get(text(edge.from_node_id));
   const toNode = nodeByID.get(text(edge.to_node_id));
   const fromIDs = exactIDs(fromNode);
   const toIDs = exactIDs(toNode);
   const joinedFromIDs = joinedIDs(fromNode);
   const joinedToIDs = joinedIDs(toNode);
   const item = {
    id: text(edge.id), edge: edge, from_node: fromNode, to_node: toNode,
    from_component_ids: fromIDs, to_component_ids: toIDs,
   };
   if (fromIDs.length === 0 || toIDs.length === 0 ||
    joinedFromIDs.length === 0 || joinedToIDs.length === 0) {
    item.reason = "zero_component";
    sideRows.push(item);
    return;
   }
   if (fromIDs.length !== 1 || toIDs.length !== 1) {
    item.reason = "plural_components";
    sideRows.push(item);
    return;
   }
   if (fromIDs[0] === toIDs[0]) {
    item.reason = "same_component";
    sideRows.push(item);
    return;
   }
   item.from_component_id = fromIDs[0];
   item.to_component_id = toIDs[0];
   edges.push(item);
  });
  return {
   mechanism_id: text(mechanism.id),
   component_ids: componentIDs,
   edges: edges,
   side_rows: sideRows,
  };
 }

 function entryHandoffLaneOffset(index) {
  if (!(index > 0)) return 0;
  const distance = Math.ceil(index / 2) * 9;
  return index % 2 ? distance : -distance;
 }

 // Pure geometry over the already-laid-out component boxes. Selecting an
 // entry never asks ELK for a new layout and never changes the viewport.
 function entryHandoffConnectionGeometry(from, to, lane) {
  if (!from || !to) return null;
 const offset = entryHandoffLaneOffset(Number(lane) || 0);
  if (from === to || (
   Number(from.x) === Number(to.x) && Number(from.y) === Number(to.y) &&
   Number(from.width) === Number(to.width) && Number(from.height) === Number(to.height)
  )) return null;
  const fromCenter = {
   x: Number(from.x) + Number(from.width) / 2,
   y: Number(from.y) + Number(from.height) / 2,
  };
  const toCenter = {
   x: Number(to.x) + Number(to.width) / 2,
   y: Number(to.y) + Number(to.height) / 2,
  };
  if (Math.abs(toCenter.x - fromCenter.x) >= Math.abs(toCenter.y - fromCenter.y)) {
   const rightward = toCenter.x >= fromCenter.x;
   const startX = rightward ? Number(from.x) + Number(from.width) : Number(from.x);
   const endX = rightward ? Number(to.x) : Number(to.x) + Number(to.width);
   const startY = clamp(fromCenter.y + offset, Number(from.y) + 12, Number(from.y) + Number(from.height) - 12);
   const endY = clamp(toCenter.y + offset, Number(to.y) + 12, Number(to.y) + Number(to.height) - 12);
   const middleX = (startX + endX) / 2;
   return {
    path: "M" + startX + " " + startY + " L" + middleX + " " + startY +
     " L" + middleX + " " + endY + " L" + endX + " " + endY,
    badge_x: middleX,
    badge_y: (startY + endY) / 2,
   };
  }
  const downward = toCenter.y >= fromCenter.y;
  const startY = downward ? Number(from.y) + Number(from.height) : Number(from.y);
  const endY = downward ? Number(to.y) : Number(to.y) + Number(to.height);
  const startX = clamp(fromCenter.x + offset, Number(from.x) + 12, Number(from.x) + Number(from.width) - 12);
  const endX = clamp(toCenter.x + offset, Number(to.x) + 12, Number(to.x) + Number(to.width) - 12);
  const middleY = (startY + endY) / 2;
  return {
   path: "M" + startX + " " + startY + " L" + startX + " " + middleY +
    " L" + endX + " " + middleY + " L" + endX + " " + endY,
   badge_x: (startX + endX) / 2,
   badge_y: middleY,
  };
 }

 function externalStateAssociation(value) {
  const kind = text(value && value.kind);
  return kind === "boundary" || kind === "resource";
 }

 function mapLensEmphasisProjection(input) {
  const lens = input && input.lens || "landscape";
  const components = array(input && input.components);
  const surfaces = array(input && input.surfaces);
  const associations = array(input && input.associations);
  const entryHandoffGroups = array(input && input.entryHandoffGroups);
  const componentByID = new Map(components.map((component) => [text(component && component.id), component]));
  const emphasized = [];
  const objects = { entrypoints: [], touchpoints: [], entry_handoff_groups: [] };
  const add = (componentID) => {
   if (componentByID.has(componentID) && emphasized.indexOf(componentID) < 0) emphasized.push(componentID);
  };

  if (lens === "entrypoints") {
   // Entry objects are the exact surface catalog entries (backend-joined
   // participation: surface.participating_component_ids /
   // component.participating_surface_ids), grouped by kind. The
   // emphasized set is every component those entries reach.
   const byKind = new Map();
   surfaces.forEach((surface) => {
    const kind = text(surface && surface.kind);
    const entry = {
     id: text(surface && surface.id),
     kind: kind,
     label: text(surface && surface.name),
     component_ids: array(surface && surface.participating_component_ids)
      .filter((id) => componentByID.has(text(id))),
    };
    entry.component_ids = Array.from(new Set(entry.component_ids));
    if (!entry.component_ids.length) return;
    if (!byKind.has(kind)) byKind.set(kind, []);
    byKind.get(kind).push(entry);
   });
   components.forEach((component) => {
    if (array(component && component.participating_surface_ids).length > 0 ||
        array(component && component.owned_surface_ids).length > 0 ||
        array(component && component.entry_surface_ids).length > 0) {
     add(text(component.id));
    }
   });
   // Canvas 15 owns the exact D210→entry join. These groups are context for
   // Entrypoints, not a fourth lens and not a client-side join over grounding.
   entryHandoffGroups.forEach((group) => {
    if (!currentEntrypointHandoffGroup(group)) return;
    const componentIDs = [];
    array(group.component_ids).forEach((value) => {
     const componentID = text(value);
     if (!componentByID.has(componentID) || componentIDs.indexOf(componentID) >= 0) return;
     componentIDs.push(componentID);
     add(componentID);
    });
    objects.entry_handoff_groups.push({
     id: text(group.id),
     kind: "entry_handoff_group",
     component_ids: componentIDs,
     group: group,
    });
   });
   byKind.forEach((entries, kind) => {
    objects.entrypoints.push({ kind: kind, entries: entries });
   });
   return { lens: lens, emphasized: emphasized, objects: objects };
  }

  if (lens === "integrations") {
   // Touchpoint objects are the exact association families observed for a
   // component (backend-owned `family` — the CLOSED generic classification;
   // raw imported_family stays available as evidence detail).
   const families = new Map();
   associations.forEach((association) => {
    // Integrations is deliberately narrower than the canonical association
    // artifact. Operation/surface rows describe local structure; presenting
    // them as a Resource would turn an internal relation into an external or
    // stateful interaction.
    if (!externalStateAssociation(association)) return;
    const componentID = text(association && (association.component_id || association.from_component_id));
    const family = text(association && association.family) ||
     text(association && association.imported_family) || "other";
    if (!componentByID.has(componentID)) return;
    add(componentID);
    const key = componentID + "\u0000" + family;
    if (!families.has(key)) {
     families.set(key, {
      component_id: componentID,
      family: family,
      kind: text(association && association.kind),
      witness_count: Number(association && association.observation_count) || 0,
      paired: association && association.paired === true,
     });
    }
   });
   families.forEach((touchpoint) => objects.touchpoints.push(touchpoint));
   return { lens: lens, emphasized: emphasized, objects: objects };
  }

  // landscape: no emphasis, no extra objects.
  return { lens: "landscape", emphasized: [], objects: objects };
 }

 function associationsForComponent(associations, componentID) {
  componentID = text(componentID);
  return array(associations).filter((association) => (
   text(association && (association.component_id || association.from_component_id)) === componentID ||
   text(association && (association.related_component_id || association.to_component_id)) === componentID
  ));
 }

 // Decision 236 (v11): projectArchitectureLens(reportData, lens) — the
 // DOM-free entry point the workspace and the canvas both use. It reads
 // ONLY backend-owned report view-model fields (architecture_canvas +
 // architecture_associations) and returns plain deterministic values:
 // visible/emphasized/dimmed principal IDs, entry-category objects,
 // touchpoint-family objects, entry-handoff context, counts and
 // omissions. It never touches the DOM, never reads geometry, never mounts.
 function projectArchitectureLens(reportData, lens) {
  const canvas = (reportData && reportData.architecture_canvas) || {};
  const components = array(canvas.components);
  const surfaces = array(canvas.surfaces);
  const projection = mapLensEmphasisProjection({
   lens: lens,
   components: components,
   surfaces: surfaces,
   associations: flatReportAssociations(reportData && reportData.architecture_associations),
   entryHandoffGroups: Number(canvas.version) === 15 ? canvas.entry_handoff_groups : [],
  });
  const componentCount = components.length;
  return {
   lens: projection.lens,
   visible: components.map((component) => text(component && component.id)),
   emphasized: projection.emphasized,
   // With no exact component join the lens is evidence-only. Keep the
   // landscape neutral instead of claiming every component is irrelevant.
   dimmed: projection.emphasized.length > 0
    ? componentCount - projection.emphasized.length : 0,
   entrypoints: projection.objects.entrypoints,
   touchpoints: projection.objects.touchpoints,
   entry_handoff_groups: projection.objects.entry_handoff_groups,
   counts: {
    components: componentCount,
    surfaces: surfaces.length,
    entries: projection.objects.entrypoints.reduce((sum, group) => sum + (group.entries || []).length, 0),
    touchpoints: projection.objects.touchpoints.length,
    entry_handoff_groups: projection.objects.entry_handoff_groups.length,
   },
   omissions: {
    // Honest bounded scope: associations/flows not joinable to a principal
    // component are never shown in a lens.
    unjoined_surfaces: surfaces.filter((surface) =>
     !array(surface && surface.participating_component_ids).some((id) =>
      components.some((component) => text(component && component.id) === text(id))
     )
    ).length,
   },
  };
 }

 // flatReportAssociations normalizes the backend association view-model
 // (architecture_associations.components[].associations[] with the owning
 // component on the parent) into the flat rows the pure projection
 // consumes. A flat array is passed through unchanged.
 function flatReportAssociations(viewModel) {
  if (Array.isArray(viewModel)) return viewModel.slice();
  const components = array(viewModel && viewModel.components);
  const out = [];
  components.forEach((entry) => {
   const componentID = text(entry && entry.component_id);
   array(entry && entry.associations).forEach((row) => {
    if (!row || typeof row !== "object") return;
    out.push(Object.assign({}, row, { component_id: componentID }));
   });
  });
  return out;
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

 // readableFitScale clamps only the UPPER bound (magnification). Decision
 // 234 (F1, fresh review): the LOWER bound must not exist — a huge landscape
 // must fit ENTIRELY inside the viewport (all principal node centers
 // hit-testable), even at very small scales. The data-semantic-scale
 // overview mode keeps tiny scales honest (title/count only).
 function readableFitScale(bounds, viewport, padding) {
  return Math.min(scaleToBounds(bounds, viewport, padding), FIT_MAX_SCALE);
 }

 function componentFocusScale(bounds, viewport, padding) {
  return clamp(scaleToBounds(bounds, viewport, padding), FOCUS_MIN_SCALE, FOCUS_MAX_SCALE);
 }

 function memberLabel(memberID, message) {
  if (!memberID) return productMessage(message, "architecture.fallback.unknown_member");
  if (typeof memberID === "string") return memberID;
  return [text(memberID.kind), text(memberID.value)].filter(Boolean).join(":");
 }

function architecturePartialTruth(data) {
  if (text(data && data.validation_outcome) !== "accepted_partial") return null;
  const remainderComponentID = text(data && data.local_remainder_component_id);
  const component = array(data && data.components).find((candidate) =>
   text(candidate && candidate.id) === remainderComponentID
  );
  const members = component ? array(component.members).map((member) => {
   const name = text(member && member.name);
   const id = member && member.id;
   const label = name || (typeof id === "string" ? text(id) :
    [text(id && id.kind), text(id && id.value)].filter(Boolean).join(":"));
   const locations = new Map();
   array(member && member.facts).forEach((fact) => {
    const location = fact && fact.location;
    if (!location || !text(location.path) || !(Number(location.line) > 0)) return;
    const key = [text(location.path), Number(location.line), Number(location.column) || 0].join("\u0000");
    locations.set(key, {
     path: text(location.path), line: Number(location.line), column: Number(location.column) || 0,
    });
   });
   return { label: label, sources: Array.from(locations.values()) };
  }).filter((member) => member.label) : [];
  return {
   remainderComponentID: remainderComponentID,
   members: members,
  };
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

 function branchLabel(kind, message) {
  const labels = {
   main: "architecture.value.branch_main",
   task: "architecture.value.branch_task",
   shared: "architecture.value.branch_shared",
   unassigned: "architecture.label.unassigned_evidence",
  };
  return productMessage(message, labels[text(kind)] || "architecture.label.unassigned_evidence");
 }

 function proofAreaLabel(kind, message) {
  const labels = {
   trigger: "architecture.value.proof_trigger",
   entrypoint: "architecture.value.proof_entrypoint",
   dispatch: "architecture.value.proof_dispatch",
   application_callable: "architecture.value.proof_application_callable",
   core_operation: "architecture.value.proof_core_operation",
   io_boundary: "architecture.value.proof_io_boundary",
   concurrency: "architecture.value.proof_concurrency",
   termination: "architecture.value.proof_termination",
  };
  const id = labels[text(kind)];
  if (id) return productMessage(message, id);
  return text(kind) || productMessage(message, "architecture.fallback.unknown_proof_area");
 }

 function presentationValueLabel(value) {
  if (value && typeof value === "object") {
   value = value.label || value.name || value.kind || value.id;
  }
  return text(value);
 }

 function semanticClass(relation, invocation) {
  const value = text(relation).toLowerCase();
  if (value.indexOf("cancel") >= 0) return "is-cancel";
  if (value.indexOf("join") >= 0 || value === "waits_for") return "is-join";
  if (value.indexOf("callback") >= 0 || invocation === "callback") return "is-callback";
  if (value.indexOf("start") >= 0 || invocation === "goroutine") return "is-start";
  return "is-call";
 }

 function savedTraceLabel(archetype, message) {
  switch (text(archetype)) {
   case "cli": return productMessage(message, "architecture.value.saved_cli_trace");
   case "process": return productMessage(message, "architecture.value.saved_process_trace");
   default: return productMessage(message, "architecture.label.saved_trace");
  }
 }

 function lifecycleRelationKind(edge) {
  const relation = text(edge && edge.relation).toLowerCase();
  const invocation = text(edge && edge.invocation).toLowerCase();
  if (relation.indexOf("cancel") >= 0) return "cancellation";
  if (relation.indexOf("join") >= 0 || relation === "waits_for") return "join";
  if (relation.indexOf("callback") >= 0 || invocation === "callback") return "callback";
  if (relation.indexOf("start") >= 0 || invocation === "goroutine") return "started_by";
  return "";
 }

 function lifecycleRelationHeading(edge, message) {
  const kind = lifecycleRelationKind(edge);
  switch (kind) {
   case "started_by": return productMessage(message, "architecture.value.lifecycle_started_by");
   case "callback": return productMessage(message, "architecture.value.lifecycle_callback");
   case "cancellation": return productMessage(message, "architecture.value.lifecycle_cancellation");
   case "join": return productMessage(message, "architecture.value.lifecycle_join");
   default: return "";
  }
 }

 function groupLifecycleRelations(flow, edges) {
  const lifecycle = array(edges).filter((edge) => lifecycleRelationKind(edge));
  const groups = [];
  const assigned = new Set();
  const labelOrder = ["started_by", "callback", "cancellation", "join"];
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
      if (lifecycleRelationKind(edge) !== "cancellation") return;
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
     return lifecycleRelationKind(edge) === "cancellation" &&
      (cancellationAnchors.has(from) || cancellationAnchors.has(to));
    });
    relations.sort((a, b) => {
     const labelDifference = labelOrder.indexOf(lifecycleRelationKind(a)) -
      labelOrder.indexOf(lifecycleRelationKind(b));
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

 function relationLabel(relation, message) {
  const value = text(relation);
  const labels = {
   calls: "architecture.relation.calls",
   callback: "architecture.relation.invokes_callback",
   constructs: "architecture.relation.constructs",
   registers: "architecture.relation.registers",
   registers_command: "architecture.relation.registers",
   starts_goroutine: "architecture.relation.starts_task",
   uses_cancellation: "architecture.relation.uses_cancellation_context",
	   cancels: "architecture.relation.invokes_cancellation",
	   joins: "architecture.relation.waits_for_task",
	   waits_for: "architecture.relation.waits_for",
	   dispatches_to: "architecture.relation.dispatches_to",
	   registers_extension_family: "architecture.relation.registers_extension_family",
	   loads_or_adapts_config: "architecture.relation.loads_or_adapts_config",
	   starts_lifecycle: "architecture.relation.starts_lifecycle",
	   exposes_admin_control_plane: "architecture.relation.exposes_admin_control_plane",
	   dispatches_http_request: "architecture.relation.dispatches_http_request",
	   configures_security_boundary: "architecture.relation.configures_security_boundary",
	   static_call_supporting_relation: "architecture.relation.static_call_supporting_relation",
	   package_import: "architecture.relation.package_import",
	   behavior_handoff: "architecture.relation.behavior_handoff",
	  };
	  if (labels[value]) return productMessage(message, labels[value]);
	  return productMessage(message, "architecture.relation.continues");
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

 class ArchitectureCanvasApp {
  constructor(host, data, options) {
   if (!(host instanceof Element)) {
    throw new TypeError("RepomapArchitectureCanvas.mount requires a host Element");
   }
   this.host = host;
   const sourceData = data && typeof data === "object" ? data : {};
   this.options = options && typeof options === "object" ? options : {};
   const message = typeof this.options.message === "function"
    ? this.options.message
    : global.RepomapUI && typeof global.RepomapUI.message === "function"
     ? global.RepomapUI.message
     : null;
   if (!message) {
    throw new TypeError("RepomapArchitectureCanvas.mount requires the typed RepomapUI message API");
   }
   this.message = (id, params) => message(id, params);
   this.userMode = this.options.userMode === true;
   this.data = this.userMode
    ? projectArchitectureUserPresentation(sourceData, this.message)
    : sourceData;
   this.presentationText = this.options.presentationText && typeof this.options.presentationText === "object"
    ? this.options.presentationText
    : {};
   this.componentContexts = this.options.componentContexts && typeof this.options.componentContexts === "object"
    ? this.options.componentContexts
    : {};
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
    this.inspectorVisible = null;

   this.subsystems = array(this.data.subsystems);
   this.components = array(this.data.components);
   this.structuralEdges = mapStructuralEdges(this.data);
   this.entryHandoffGroups = Number(this.data.version) === 15
    ? array(this.data.entry_handoff_groups).filter(currentEntrypointHandoffGroup)
    : [];
   this.entryHandoffGroupByID = new Map(
    this.entryHandoffGroups.map((group) => [text(group.id), group])
   );
   this.selectedEntryHandoffGroupID = "";
   this.studyMechanismOverlay = null;
     this.flows = array(this.data.flows).filter((flow) => (
      !this.userMode || array(flow && flow.steps).length >= 2
     ));
     this.surfaces = array(this.data.surfaces);
     this.suggestions = this.userMode ? [] : array(this.data.suggested_investigations);
    this.candidateDirections = this.userMode ? [] : array(this.options.candidateDirections);
   this.flowEdges = array(this.data.flow_edges);
   this.frontiers = this.userMode ? [] : array(this.data.frontiers);
   this.diagnostics = this.userMode ? [] : array(this.data.diagnostics);
   this.partialTruth = architecturePartialTruth(this.data);
   const guidedTourStory = this.options.guidedTour;
   const activeGuidedTourStory = this.userMode ? null : guidedTourStory;
   const guidedTourSteps = activeGuidedTourStory && typeof activeGuidedTourStory === "object"
    ? array(activeGuidedTourStory.steps).filter((step) => step && typeof step === "object")
    : [];
   this.guidedTourStory = guidedTourSteps.length > 0 ? activeGuidedTourStory : null;
   this.guidedTourSteps = guidedTourSteps;
   this.guidedTour = { active: false, index: 0 };
   this.semanticArtifacts = (this.userMode ? [] : array(this.options.semanticArtifacts)).filter(
    (artifact) => artifact && typeof artifact === "object" && text(artifact.id)
   );
   this.semanticArtifactByID = new Map(
    this.semanticArtifacts.map((artifact) => [text(artifact.id), artifact])
   );
   const requestedStartHereArtifactID = text(this.options.startHereArtifactID);
   const startHereArtifact = this.semanticArtifactByID.get(requestedStartHereArtifactID);
   this.startHereArtifactID = startHereArtifact && text(startHereArtifact.kind) === "mechanism"
    ? requestedStartHereArtifactID
    : "";
   this.semanticNarrative = { artifactID: "", index: 0 };

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
   this.entryHandoffOverlayElements = [];
   this.studyMechanismOverlayElements = [];
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
     architectureStepComponentState(step, this.componentByID).related.forEach((componentID) => {
      componentIDs.add(componentID);
     });
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
     this.landscapeButton = element("button", "rm-arch__flow-button is-active", this.msg("architecture.nav.architecture"));
    this.landscapeButton.type = "button";
    this.listen(this.landscapeButton, "click", () => {
     this.backToArchitecture();
    });
    flowNav.appendChild(this.landscapeButton);
    if (this.startHereArtifactID) {
     this.startHereButton = element("button", "rm-arch__flow-button rm-arch__tour-start", this.msg("architecture.nav.start_here"));
     this.startHereButton.type = "button";
     this.startHereButton.setAttribute("aria-pressed", "false");
     this.startHereButton.title = this.msg("architecture.title.open_primary_mechanism");
     this.listen(this.startHereButton, "click", () => {
      this.openSemanticArtifact(this.startHereArtifactID, 0);
     });
     flowNav.appendChild(this.startHereButton);
    }
    if (this.guidedTourStory) {
     const guidedTourLabel = this.startHereArtifactID
      ? this.msg("architecture.nav.guided_tour")
      : this.msg("architecture.nav.start_here");
     this.guidedTourButton = element(
      "button",
      "rm-arch__flow-button" + (this.startHereArtifactID ? "" : " rm-arch__tour-start"),
      guidedTourLabel
     );
     this.guidedTourButton.type = "button";
     this.guidedTourButton.setAttribute("aria-pressed", "false");
     this.guidedTourButton.title = this.msg("architecture.title.open_guided_tour");
     this.listen(this.guidedTourButton, "click", () => this.startGuidedTour());
     flowNav.appendChild(this.guidedTourButton);
    }
    if (this.flows.length > 0) {
     if (this.userMode) {
      this.pathList = element("div", "rm-arch__path-list");
      this.pathList.appendChild(element("span", "rm-arch__path-list-label", this.msg("architecture.nav.code_paths")));
      this.flows.forEach((flow) => {
       const button = element(
        "button",
        "rm-arch__flow-button rm-arch__path-button",
        flow.name || this.msg("architecture.fallback.code_path")
       );
       button.type = "button";
       if (flow.why_inspect) button.title = flow.why_inspect;
       this.listen(button, "click", () => this.openTrace(flow.id));
       this.flowButtons.set(text(flow.id), button);
       this.pathList.appendChild(button);
      });
      flowNav.appendChild(this.pathList);
     } else {
     this.traceMenu = element("div", "rm-arch__trace-menu");
     this.traceMenuSummary = element(
      "button",
      "rm-arch__flow-button",
      this.userMode
       ? this.msg("architecture.count.code_paths", { count: this.flows.length })
       : this.msg("architecture.count.saved_traces", { count: this.flows.length })
     );
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
       button.appendChild(element("strong", null, flow.name || this.msg("architecture.fallback.code_path")));
       if (flow.why_inspect) button.appendChild(element("small", "rm-arch__trace-purpose", flow.why_inspect));
        const startSurface = this.surfaceByID.get(text(flow.start_surface_id));
        const originName = startSurface ? startSurface.name || startSurface.kind : flow.trigger || flow.command;
        const origin = [
         this.userMode
          ? this.msg("architecture.fallback.code_path")
          : savedTraceLabel(flow.archetype, this.message),
         originName,
        ].filter(Boolean).join(" · ");
       button.appendChild(element(
        "span",
        null,
        [origin, this.userMode ? "" : flow.status]
         .filter(Boolean)
         .join(" · ")
       ));
       button.appendChild(element(
        "span",
        null,
        this.msg("architecture.count.components", { count: array(flow.participating_component_ids).length })
       ));
      if (!this.userMode && flow.status !== "complete" && flow.frontier_summary) {
       button.appendChild(element(
        "small",
        null,
        this.msg("architecture.label.frontier_value", { value: flow.frontier_summary })
       ));
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
    }
   toolbar.appendChild(flowNav);

    const controls = element("div", "rm-arch__controls");
    const diagnosticComponents = this.userMode ? [] : this.diagnosticComponents();
    if (diagnosticComponents.length > 0) {
     diagnosticComponents.forEach((component) => this.diagnosticComponentIDs.add(text(component.id)));
     this.diagnosticButton = this.controlButton(
      this.msg("architecture.count.unassigned_evidence", { count: diagnosticComponents.length }),
      this.msg("architecture.title.inspect_unassigned_evidence"),
      () => this.setSelection({
       flow: "", component: text(diagnosticComponents[0].id), step: "", edge: "",
      }, true)
     );
     this.diagnosticButton.classList.add("rm-arch__diagnostic-control");
     controls.appendChild(this.diagnosticButton);
    }
    this.zoomOutButton = this.controlButton("−", this.msg("architecture.action.zoom_out"), () => this.zoomBy(0.82));
      this.fitButton = this.controlButton(
       this.msg("architecture.action.fit"),
       this.msg("architecture.title.fit_readable"),
       () => this.fit()
      );
   this.zoomInButton = this.controlButton("+", this.msg("architecture.action.zoom_in"), () => this.zoomBy(1.22));
   controls.append(this.zoomOutButton, this.fitButton, this.zoomInButton);
    toolbar.appendChild(controls);
    this.root.appendChild(toolbar);
    if (this.traceList) this.root.appendChild(this.traceList);

    if (this.userMode && this.partialTruth) {
	 // Decision 217: the unassigned-evidence wall becomes a compact
	 // collapsed disclosure — count in the summary, every exact item
	 // preserved behind the expand action.
	 const partial = element("details", "rm-arch__partial-truth");
	 const summary = element("summary", "rm-arch__partial-truth-label");
	 summary.appendChild(element(
	  "strong",
	  "rm-arch__partial-truth-label-text",
	  this.msg("architecture.value.accepted_partial")
	 ));
	 if (this.partialTruth.members.length > 0) {
	  summary.appendChild(element(
	   "span",
	   "rm-arch__partial-count",
	   this.msg("architecture.count.local_remainder_members", {
	    count: this.partialTruth.members.length,
	   })
	  ));
	 }
	 partial.appendChild(summary);
	 partial.appendChild(element(
	  "p",
	  "rm-arch__copy",
	  this.msg("architecture.copy.accepted_partial")
	 ));
	 if (this.partialTruth.members.length > 0) {
	  const members = element("div", "rm-arch__partial-member-list");
	  this.partialTruth.members.forEach((member) => {
	   const sources = array(member.sources);
	   if (sources.length > 0 && typeof this.options.openLocation === "function") {
	    sources.forEach((location) => {
	     const action = element("button", "rm-arch__partial-member-action");
	     action.type = "button";
	     action.appendChild(element("strong", null, member.label));
	     action.appendChild(element("span", null, locationLabel(location)));
	     this.listen(action, "click", () => this.options.openLocation(
	      location.path, location.line, location.column
	     ));
	     members.appendChild(action);
	    });
	    return;
	   }
	   members.appendChild(element("span", "rm-arch__member-id", member.label));
	  });
	  partial.appendChild(members);
	 }
     this.root.appendChild(partial);
    }

   const workspace = element("div", "rm-arch__workspace");
    this.viewport = element("div", "rm-arch__viewport");
    this.loading = element("div", "rm-arch__loading", this.msg("architecture.state.laying_out"));
     this.flowFocus = element("div", "rm-arch__flow-focus");
     this.flowFocus.hidden = true;
     this.viewportHint = element("div", "rm-arch__viewport-hint", this.msg("architecture.hint.drag_to_explore"));
     // Decision 234 (fresh review): the persistent interaction hint is
     // announced as a note with an explicit label naming the exact
     // behavior (wheel zooms, drag pans) — quiet, never occluding.
     this.viewportHint.setAttribute("role", "note");
     this.viewportHint.setAttribute("aria-label", this.msg("architecture.hint.drag_groups_fit", { count: 0 }));
     this.viewport.append(this.loading, this.flowFocus, this.viewportHint);
   workspace.appendChild(this.viewport);

    this.drawerBackdrop = element("button", "rm-arch__drawer-backdrop");
    this.drawerBackdrop.type = "button";
     this.drawerBackdrop.setAttribute("aria-label", this.msg("architecture.aria.close_inspector"));
    // Decision 230 (fresh review task-4 B1): the backdrop is a pointer
    // scrim, never a keyboard tab stop — it must not sit between the
    // last inspector control and the focus-trap wrap.
    this.drawerBackdrop.tabIndex = -1;
    this.drawerBackdrop.hidden = true;
    this.listen(this.drawerBackdrop, "click", () => this.closeInspector());
     this.inspector = element("aside", "rm-arch__inspector");
    this.inspector.setAttribute("aria-label", this.msg("architecture.aria.inspector"));
    // Decision 230 D4: the fixed overlay drawer is a modal dialog —
    // background pointer/keyboard policy is explicit (backdrop blocks
    // pointer; focus trap keeps keyboard inside; Escape closes).
    this.inspector.setAttribute("role", "dialog");
    this.inspector.setAttribute("aria-modal", "true");
    workspace.appendChild(this.inspector);
    this.root.append(workspace, this.drawerBackdrop);

   this.host.appendChild(this.root);
   this.installViewportInteractions();
    if (!this.userMode) this.listen(global, "hashchange", () => this.restoreHash(true));
    this.listen(global, "keydown", (event) => {
     if (event.key !== "Escape") return;
     if (this.semanticArtifact()) {
      this.finishSemanticArtifact(true);
      return;
     }
     if (this.guidedTour.active) {
      this.finishGuidedTour(true);
      return;
     }
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
     if (!this.hasInspectorSelection(this.selection) && !this.guidedTour.active && !this.semanticArtifact()) return;
     const target = event.target;
     if (!target || this.inspector.contains(target)) return;
     if (target === this.drawerBackdrop) return;
     if (typeof target.closest === "function" && target.closest(".rm-explore")) return;
     if (typeof target.closest === "function" && target.closest(".rm-arch__component-card")) return;
     // Decision 236 (v11): the Map lens control is a map-scope control —
     // switching a lens must never close the inspector or drop the
     // selection.
     if (typeof target.closest === "function" && target.closest(".rm-map-lens-control")) return;
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

  msg(id, params) {
   return this.message(id, params);
  }

  presented(address, fallback) {
   return architecturePresentationText(this.presentationText, address, fallback);
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
    .catch(() => {
     if (this.destroyed) return;
     this.loading.textContent = this.msg("architecture.error.layout_failed");
     this.loading.classList.add("is-error");
     this.renderInspector();
    });
   return this.ready;
  }

   layoutOnce() {
    const ELKConstructor = global.ELK;
   if (typeof ELKConstructor !== "function") {
    return Promise.reject(new Error(this.msg("architecture.error.renderer_unavailable")));
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
     if (this.userMode && this.isDiagnosticSubsystem(subsystem)) return;
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
    if (!this.userMode && ungrouped.length > 0) {
     ungrouped.forEach((componentID) => componentOwner.set(componentID, "__ungrouped__"));
     allGroups.push({ id: "__ungrouped__", subsystem: null, componentIDs: ungrouped, diagnostic: true });
    }

    const diagnosticGroups = this.userMode ? [] : allGroups.filter((group) => group.diagnostic);
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
    switch (category) {
     case "primary": return this.msg("architecture.value.category_primary");
     case "external": return this.msg("architecture.value.category_boundary");
     case "support": return this.msg("architecture.value.category_supporting");
     case "diagnostic": return this.msg("architecture.label.unresolved");
     default: return "";
    }
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

  flowStepComponentState(flowID, stepID) {
   const step = this.flowStepsByKey.get(flowStepKey(flowID, stepID));
   return step
    ? architectureStepComponentState(step, this.componentByID)
    : { owner: "", participants: [], related: [], lane: "", selection: "" };
  }

  flowStepLaneComponent(flowID, stepID) {
   return this.flowStepComponentState(flowID, stepID).lane;
  }

  flowStepSelectionComponent(flowID, stepID) {
   return this.flowStepComponentState(flowID, stepID).selection;
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
   this.entryHandoffSVG = this.createSVG("rm-arch__edges rm-arch__edges--entry-handoff");
   this.studyMechanismSVG = this.createSVG("rm-arch__edges rm-arch__edges--study-mechanism");
   this.surface.append(this.structuralSVG, this.flowSVG, this.entryHandoffSVG, this.studyMechanismSVG);

   this.groupLayer = element("div", "rm-arch__groups");
   this.nodeLayer = element("div", "rm-arch__nodes");
   this.stepLayer = element("div", "rm-arch__steps");
   this.entryHandoffBadgeLayer = element("div", "rm-arch__entry-handoff-badges");
   this.surface.append(this.groupLayer, this.nodeLayer, this.stepLayer, this.entryHandoffBadgeLayer);
    this.viewport.appendChild(this.surface);
    if (this.landscapeProjection) {
     const viewportHint = this.msg("architecture.hint.drag_groups_fit", {
      count: this.landscapeProjection.groups.length,
     });
     this.viewportHint.textContent = viewportHint;
     this.viewportHint.setAttribute("aria-label", viewportHint);
    }

   this.renderGroups();
   this.renderComponents();
   if (!this.userMode) this.renderUnassignedRail();
   this.renderStructuralEdges();
   this.renderFlowSteps();
   this.renderFlowEdges();
   this.renderEntrypointHandoffOverlay();
   this.renderStudyMechanismOverlay();
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
     group.title = text(
      subsystem.description || subsystem.name ||
      (this.userMode ? this.msg("architecture.fallback.repository_area") : subsystem.id)
     );
     const header = element("div", "rm-arch__group-header");
     header.appendChild(element("span", "rm-arch__category-marker"));
     header.appendChild(element(
      "h3",
      "rm-arch__group-title",
      subsystem.name || (this.userMode ? this.msg("architecture.fallback.repository_area") : subsystem.id)
     ));
     const categoryLabel = this.semanticCategoryLabel(category);
     if (categoryLabel) header.appendChild(element("span", "rm-arch__group-category", categoryLabel));
     header.appendChild(element(
      "span",
      "rm-arch__group-count",
      this.msg("architecture.count.components", { count: array(subsystem.component_ids).length })
     ));
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
     header.appendChild(element(
      "h3",
      "rm-arch__group-title",
      this.msg("architecture.label.unassigned_subsystem")
     ));
     header.appendChild(element("span", "rm-arch__group-category", this.msg("architecture.label.unresolved")));
     group.appendChild(header);
    this.groupLayer.appendChild(group);
   }
  }

  componentSurfaces(component) {
   // A component may participate in an exact entry surface without owning
   // it. The card and inspector must agree with the Entrypoints lens, whose
   // backend join is represented by participating_surface_ids.
   const ids = Array.from(new Set(
    array(component && component.owned_surface_ids)
     .concat(array(component && component.participating_surface_ids))
     .map(text)
     .filter(Boolean)
   ));
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
      button.title = text(component.name || (this.userMode ? this.msg("architecture.fallback.code_component") : id));
      if (singleton && subsystem) {
       button.appendChild(element(
        "span",
        "rm-arch__component-group",
        subsystem.name || (this.userMode ? this.msg("architecture.fallback.repository_area") : subsystem.id)
       ));
      }
     button.appendChild(element(
      "strong",
      "rm-arch__component-name",
      component.name || (this.userMode ? this.msg("architecture.fallback.code_component") : id)
     ));
     if (component.description) {
      button.appendChild(element("span", "rm-arch__component-description", component.description));
     }
      const associatedSurfaces = this.componentSurfaces(component);
      if (associatedSurfaces.length > 0) {
       const surfaceSummary = element("span", "rm-arch__component-surfaces");
       surfaceSummary.appendChild(element(
        "span",
        "rm-arch__component-surfaces-label",
        this.msg("architecture.label.surfaces")
       ));
       associatedSurfaces.slice(0, 2).forEach((surface) => {
        surfaceSummary.appendChild(element(
         "span",
         "rm-arch__surface-chip",
         surface.name || surface.kind ||
          (this.userMode ? this.msg("architecture.fallback.runtime_surface") : surface.id)
        ));
       });
       if (associatedSurfaces.length > 2) {
        surfaceSummary.appendChild(element("span", "rm-arch__surface-chip is-more", "+" + (associatedSurfaces.length - 2)));
       }
       button.appendChild(surfaceSummary);
      }
       const metadata = [];
       if (associatedSurfaces.length > 0) {
        const allCommands = associatedSurfaces.every((surface) => surface.kind === "cli_command");
        metadata.push(allCommands
         ? this.msg("architecture.count.commands", { count: associatedSurfaces.length })
         : this.msg("architecture.count.surfaces", { count: associatedSurfaces.length }));
       }
       const participatingFlows = array(component.participating_flow_ids).map((flowID) => this.flowByID.get(text(flowID))).filter(Boolean);
       if (participatingFlows.length === 1) {
        metadata.push(
         this.userMode
          ? this.msg("architecture.count.code_paths", { count: 1 })
          : this.msg("architecture.count.trace_with_status", {
           count: 1,
           status: participatingFlows[0].status || this.msg("architecture.value.saved"),
          })
        );
       } else if (participatingFlows.length > 1) {
        metadata.push(this.msg(
         this.userMode ? "architecture.count.code_paths" : "architecture.count.saved_traces",
         { count: participatingFlows.length }
        ));
       }
       const suggestionCount = this.userMode ? 0 : array(component.suggested_investigation_ids).length;
       if (suggestionCount > 0) {
        metadata.push(this.msg("architecture.count.suggested_investigations", { count: suggestionCount }));
       }
       const anchorCount = array(component.anchor_ids).length;
       // Decision 231 (Archive 9): shared participation is visible on the
       // card — the component participates in shared units without
       // exclusive ownership; exact anchors still show.
       const sharedCount = array(component.shared_unit_refs).length;
       if (!this.userMode && sharedCount > 0) {
        metadata.push(this.msg("architecture.count.shared_scope", { count: sharedCount }));
       }
       if (!this.userMode && anchorCount > 0) metadata.push(this.msg("architecture.count.exact_anchors", { count: anchorCount }));
       if (!this.userMode && metadata.length === 0) {
        const memberCount = array(component.members).length;
        if (memberCount > 0) metadata.push(this.msg("architecture.count.exact_members", { count: memberCount }));
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
   this.unassignedRail.appendChild(element(
    "h3",
    "rm-arch__unassigned-title",
    this.msg("architecture.label.unassigned_evidence")
   ));
   this.unassignedRail.appendChild(
    element("p", "rm-arch__unassigned-note", this.msg("architecture.copy.unassigned_evidence"))
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
     this.msg("architecture.aria.structural_relation", {
      relation: text(
       (edge.witness || {}).kind ||
       (this.userMode ? this.msg("architecture.value.between_code_areas") : edge.id)
      ),
     }),
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
     const owner = this.flowStepLaneComponent(flowID, step.id);
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
     if (this.userMode && owner === UNASSIGNED_ID) return;
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
    step.label || step.qualified_name || (this.userMode ? this.msg("architecture.fallback.code_step") : step.id)
   );
   this.listen(button, "click", (event) => {
    event.stopPropagation();
    this.setSelection({
     flow: flowID,
     component: this.flowStepSelectionComponent(flowID, step.id),
     step: text(step.id),
     edge: "",
    }, true);
   });
   this.stepElements.set(flowStepKey(flowID, step.id), button);
  }

  renderFrontierChip(flowID, frontier, index) {
   const geometry = this.flowStepGeometry(UNASSIGNED_ID, index);
   if (!geometry) return;
   const button = this.canvasChip(
    flowID,
    geometry,
    "rm-arch__step is-unassigned is-frontier",
    frontier.kind || this.msg("architecture.fallback.frontier")
   );
   this.listen(button, "click", (event) => {
    event.stopPropagation();
    this.setSelection({ flow: flowID, component: "", step: "", edge: "" }, true);
   });
   this.frontierElements.set(selectionKey(flowID, frontier.id), button);
  }

  renderOverflowChip(flowID, owner, hiddenCount) {
   const geometry = this.overflowGeometry(owner);
   if (!geometry) return;
   const button = this.canvasChip(
    flowID,
    geometry,
    "rm-arch__step is-overflow",
    this.msg("architecture.count.more", { count: hiddenCount })
   );
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
    const owner = this.flowStepLaneComponent(edge.flow_id, edge.from);
    if (owner && owner === this.flowStepLaneComponent(edge.flow_id, edge.to)) local.push({ edge: edge, owner: owner });
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
   const owner = this.flowStepLaneComponent(edge.flow_id, edge.from);
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
    const fromOwner = this.flowStepLaneComponent(edge.flow_id, edge.from);
    const isLocal = fromOwner && fromOwner === this.flowStepLaneComponent(edge.flow_id, edge.to);
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
    const fromStep = this.flowStepsByKey.get(flowStepKey(edge.flow_id, edge.from));
    const toStep = this.flowStepsByKey.get(flowStepKey(edge.flow_id, edge.to));
    const userTransitionFrom = fromStep && (fromStep.label || fromStep.qualified_name) || "";
    const userTransitionTo = toStep && (toStep.label || toStep.qualified_name) || "";
    const group = this.interactiveSVGPath(
     route,
     className,
     this.userMode
      ? this.msg("architecture.aria.code_transition", {
       from: userTransitionFrom,
       to: userTransitionTo,
      })
      : this.msg("architecture.aria.flow_transition", {
       relation: text(edge.relation || edge.id),
       from: text(edge.from),
       to: text(edge.to),
      }),
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

  entryHandoffSourceBadge(item, geometry) {
   const handoff = item && item.handoff;
   const formatted = locationLabel(handoff);
   if (!formatted || !geometry) return null;
   const staticURL = typeof this.options.staticSourceURL === "function"
    ? text(this.options.staticSourceURL(handoff.path, handoff.line || 0, handoff.column || 0))
    : "";
   const actionable = staticURL || typeof this.options.openLocation === "function";
   if (!actionable) return null;
   const control = element(
    staticURL ? "a" : "button",
    "rm-arch__entry-handoff-source",
    "↗ " + formatted
   );
   if (staticURL) {
    control.setAttribute("href", staticURL);
    control.setAttribute("target", "_blank");
    control.setAttribute("rel", "noopener noreferrer");
   } else {
    control.type = "button";
    this.listen(control, "click", (event) => {
     event.stopPropagation();
     this.options.openLocation(handoff.path, handoff.line || 0, handoff.column || 0);
    });
   }
   const target = handoff.target || {};
   control.title = [
    text(target.symbol || target.label || handoff.symbol) || this.msg("main.map.entry.context.direct_call"),
    this.msg("main.map.entry.context.callsite"),
    formatted,
   ].join(" · ");
   control.style.left = geometry.badge_x + "px";
   control.style.top = geometry.badge_y + "px";
   control.setAttribute("data-entry-handoff-source", text(item.id));
   return control;
  }

  renderEntrypointHandoffOverlay() {
   if (!this.entryHandoffSVG || !this.entryHandoffBadgeLayer) return;
   const defs = svgElement("defs");
   defs.appendChild(this.arrowMarker("rm-arch-entry-handoff-arrow", "#0f766e"));
   this.entryHandoffSVG.replaceChildren(defs);
   this.entryHandoffBadgeLayer.replaceChildren();
   this.entryHandoffOverlayElements = [];
   this.componentElements.forEach((node) => {
    node.classList.remove("rm-arch__is-entry-handoff-participant");
   });
   if (this.lens !== "entrypoints" || !this.selectedEntryHandoffGroupID) return;
   const group = this.entryHandoffGroupByID.get(this.selectedEntryHandoffGroupID);
   const projection = entryHandoffOverlayProjection(group, Array.from(this.componentByID.keys()));
   if (!projection.group_id) return;
   projection.component_ids.forEach((componentID) => {
    const node = this.componentElements.get(componentID);
    if (node) node.classList.add("rm-arch__is-entry-handoff-participant");
   });
   const lanes = new Map();
   projection.edges.forEach((item) => {
    const from = this.nodePositions.get(item.from_component_id);
    const to = this.nodePositions.get(item.to_component_id);
    if (!from || !to) return;
    const pair = item.from_component_id + "\u0000" + item.to_component_id;
    const lane = lanes.get(pair) || 0;
    lanes.set(pair, lane + 1);
    const geometry = entryHandoffConnectionGeometry(from, to, lane);
    if (!geometry) return;
    const edge = this.interactiveSVGPath(
     geometry.path,
     "rm-arch__edge rm-arch__edge--entry-handoff",
     [
      text(item.handoff && item.handoff.symbol),
      locationLabel(item.handoff),
     ].filter(Boolean).join(" · "),
     null
    );
    const visible = edge.querySelector(".rm-arch__edge-visible");
    if (visible) visible.setAttribute("marker-end", "url(#rm-arch-entry-handoff-arrow)");
    edge.setAttribute("data-entry-handoff-edge", text(item.id));
    this.entryHandoffSVG.appendChild(edge);
    this.entryHandoffOverlayElements.push(edge);
    const source = this.entryHandoffSourceBadge(item, geometry);
    if (source) this.entryHandoffBadgeLayer.appendChild(source);
   });
  }

  selectEntrypointHandoffGroup(groupID) {
   const selected = text(groupID);
   if (!this.entryHandoffGroupByID.has(selected)) return false;
   this.selectedEntryHandoffGroupID = selected;
   this.renderEntrypointHandoffOverlay();
   return true;
  }

  clearEntrypointHandoffGroup() {
   this.selectedEntryHandoffGroupID = "";
   this.renderEntrypointHandoffOverlay();
  }

  renderStudyMechanismOverlay() {
   if (!this.studyMechanismSVG) return;
   const defs = svgElement("defs");
   defs.appendChild(this.arrowMarker("rm-arch-study-mechanism-arrow", "#7c3aed"));
   this.studyMechanismSVG.replaceChildren(defs);
   this.studyMechanismOverlayElements = [];
   this.componentElements.forEach((node) => {
    node.classList.remove("rm-arch__is-study-mechanism-participant");
   });
   const projection = this.lens === "landscape"
    ? studyMechanismOverlayProjection(
     this.studyMechanismOverlay,
     Array.from(this.componentByID.keys())
    )
    : { mechanism_id: "", component_ids: [], edges: [] };
   if (this.root) {
    this.root.setAttribute(
     "data-study-mechanism-overlay",
     projection.mechanism_id ? "true" : "false"
    );
   }
   if (!projection.mechanism_id) return;
   projection.component_ids.forEach((componentID) => {
    const node = this.componentElements.get(componentID);
    if (node) node.classList.add("rm-arch__is-study-mechanism-participant");
   });
   const lanes = new Map();
   projection.edges.forEach((item) => {
    const from = this.nodePositions.get(item.from_component_id);
    const to = this.nodePositions.get(item.to_component_id);
    if (!from || !to) return;
    const pair = item.from_component_id + "\u0000" + item.to_component_id;
    const lane = lanes.get(pair) || 0;
    lanes.set(pair, lane + 1);
    const geometry = entryHandoffConnectionGeometry(from, to, lane);
    if (!geometry) return;
    const edge = this.interactiveSVGPath(
     geometry.path,
     "rm-arch__edge rm-arch__edge--study-mechanism",
     this.msg("architecture.aria.study_mechanism_transition", {
      from: text(item.from_node && item.from_node.label),
      to: text(item.to_node && item.to_node.label),
     }),
     null
    );
    const visible = edge.querySelector(".rm-arch__edge-visible");
    if (visible) visible.setAttribute("marker-end", "url(#rm-arch-study-mechanism-arrow)");
    edge.setAttribute("data-study-mechanism-edge", text(item.id));
    this.studyMechanismSVG.appendChild(edge);
    this.studyMechanismOverlayElements.push(edge);
   });
  }

  // D246: this is a transient paint operation over existing node positions.
  // It deliberately does not call layout, fit, focus, or applyView, so the
  // user's current Canvas transform is byte-for-byte stable.
  setStudyMechanismOverlay(mechanism) {
   this.studyMechanismOverlay = currentStudyMechanism(mechanism) ? mechanism : null;
   this.renderStudyMechanismOverlay();
   return !!this.studyMechanismOverlay;
  }

  clearStudyMechanismOverlay() {
   this.studyMechanismOverlay = null;
   this.renderStudyMechanismOverlay();
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
   copy.appendChild(element(
    "strong",
    null,
    step.label || step.qualified_name || (this.userMode ? this.msg("architecture.fallback.code_step") : step.id)
   ));
   const state = architectureStepComponentState(step, this.componentByID);
   const componentNames = state.related.map((componentID) => {
    const component = this.componentByID.get(componentID);
    return component && (component.name || (this.userMode ? "" : component.id));
   }).filter(Boolean);
   copy.appendChild(element(
    "span",
    "rm-arch__focus-step-meta",
    [componentNames.join(", "), locationLabel(step.location)].filter(Boolean).join(" · ") ||
     (this.userMode
      ? this.msg("architecture.fallback.implementation_step")
      : this.msg("architecture.fallback.exact_saved_anchor"))
   ));
   button.appendChild(copy);
   this.listen(button, "click", () => this.setSelection({
    flow: text(flow.id),
    component: this.flowStepSelectionComponent(flow.id, step.id),
    step: text(step.id),
    edge: "",
   }, true));
   return button;
  }

  focusedTransitionButton(flow, edge, className) {
   if (!edge) {
    return element("span", "rm-arch__focus-transition is-static", this.msg("architecture.label.then"));
   }
   const button = element(
    "button",
    "rm-arch__focus-transition " + (className || "") + " " + semanticClass(edge.relation, edge.invocation),
    relationLabel(edge.relation, this.message)
   );
   button.type = "button";
   button.dataset.edgeId = text(edge.id);
   button.setAttribute("aria-label", this.msg("architecture.aria.inspect_transition", {
    relation: text(edge.relation) || this.msg("architecture.label.flow"),
   }));
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
   row.appendChild(element(
    "strong",
    "rm-arch__lifecycle-label",
    lifecycleRelationHeading(edge, this.message)
   ));
   const content = element("div", "rm-arch__lifecycle-content");
   const endpoints = element("div", "rm-arch__lifecycle-endpoints");
   const source = this.flowStepsByKey.get(flowStepKey(flow.id, edge.from));
   const target = this.flowStepsByKey.get(flowStepKey(flow.id, edge.to));
   const endpoint = (step) => {
    if (text(step.id) === text(rootAnchorID)) {
     return element("span", "rm-arch__lifecycle-root-reference", this.msg("architecture.label.this_task"));
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
   this.appendLocation(metadata, edge.evidence, this.msg("architecture.label.exact_source"));
   content.appendChild(metadata);
   row.appendChild(content);
   parent.appendChild(row);
  }

  appendFocusedLifecycleCard(parent, flow, group, concurrency, ungroupedTotal) {
   const card = element("article", "rm-arch__lifecycle-card");
   const header = element("header", "rm-arch__lifecycle-card-header");
   const root = group.rootAnchorID && this.flowStepsByKey.get(flowStepKey(flow.id, group.rootAnchorID));
   header.appendChild(element(
    "span",
    "rm-arch__lifecycle-kicker",
    root ? this.msg("architecture.label.task_branch") : this.msg("architecture.label.ungrouped_lifecycle")
   ));
   if (root) {
    header.appendChild(this.focusedStepButton(flow, root, "is-compact is-task", ""));
   } else {
    header.appendChild(element("strong", null, this.msg("architecture.copy.no_exact_task_root")));
   }
   card.appendChild(header);
   const relations = element("div", "rm-arch__lifecycle-relations");
   group.relations.forEach((edge) => this.appendFocusedLifecycleRelation(relations, flow, edge, group.rootAnchorID));
   const limitation = element("div", "rm-arch__lifecycle-limitation");
   limitation.appendChild(element("strong", null, this.msg("architecture.label.limitation")));
   const limitations = [];
   if (concurrency && concurrency.missing) limitations.push(text(concurrency.missing));
   if (!root && ungroupedTotal > group.relations.length) {
    limitations.push(this.msg("architecture.count.showing_lifecycle_relations", {
     shown: group.relations.length,
     total: ungroupedTotal,
    }));
   }
   if (!root) limitations.push(this.msg("architecture.copy.lifecycle_remains_ungrouped"));
   limitations.push(this.msg("architecture.copy.static_relation_limit"));
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
   summary.appendChild(element("strong", null, this.msg("architecture.action.inspect_full_evidence")));
   summary.appendChild(element("span", null, this.msg("architecture.count.anchors_proof_areas", {
    anchors: array(flow.steps).length,
    areas: array(flow.slots).length,
   })));
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
   if (flow.trigger) this.appendKeyValue(parent, this.msg("architecture.label.starts_when"), flow.trigger);
   if (flow.scope) this.appendKeyValue(parent, this.msg("architecture.label.scope"), flow.scope);
   if (flow.command) this.appendKeyValue(parent, this.msg("architecture.label.command"), flow.command);
   if (flow.trace_quality) this.appendKeyValue(parent, this.msg("architecture.label.trace_quality"), flow.trace_quality);
   if (flow.current_frontier) {
    this.appendKeyValue(parent, this.msg("architecture.label.current_frontier"), flow.current_frontier);
   }

   const branches = section(this.msg("architecture.section.branches"));
   array(flow.branches).forEach((branch) => {
    const row = element("div", "rm-arch__branch-row " + branchClass(branch.kind));
    row.appendChild(element("strong", null, branchLabel(branch.kind, this.message)));
    row.appendChild(element(
     "span",
     null,
     this.msg("architecture.count.exact_anchors", { count: array(branch.anchor_ids).length })
    ));
    if (branch.root_anchor_id) {
     const root = this.flowStepsByKey.get(flowStepKey(flow.id, branch.root_anchor_id));
     row.appendChild(element(
      "code",
      null,
      root && (root.label || root.qualified_name) || this.msg("architecture.fallback.task_root")
     ));
    }
    branches.appendChild(row);
   });

   const steps = section(this.msg("architecture.section.exact_steps"));
   array(flow.steps).forEach((step) => steps.appendChild(this.stepJumpButton(flow, step)));

   const slots = section(this.msg("architecture.section.proof_slots"));
   array(flow.slots).forEach((slot) => {
    const card = element("article", "rm-arch__slot is-" + (text(slot.status) || "unknown"));
    const header = element("div", "rm-arch__slot-header");
    header.appendChild(element("strong", null, proofAreaLabel(slot.kind, this.message)));
    header.appendChild(element("span", "rm-arch__badge", text(slot.status)));
    card.appendChild(header);
    if (slot.summary) card.appendChild(element("p", "rm-arch__copy", slot.summary));
    if (slot.missing) card.appendChild(element("p", "rm-arch__notice is-warning", slot.missing));
    if (slot.applicability_reason) {
     this.appendKeyValue(card, this.msg("architecture.label.applicability"), slot.applicability_reason);
    }
    this.appendProvenance(
     card,
     slot.provenance,
     "architecture/flows/" + text(flow.id) + "/slots/" + text(slot.kind)
    );
    slots.appendChild(card);
   });

   const frontiers = section(this.msg("architecture.section.unresolved_frontiers"));
   const flowFrontiers = this.frontiers.filter((frontier) => text(frontier.flow_id) === text(flow.id));
   if (flowFrontiers.length === 0) {
    frontiers.appendChild(element(
     "p",
     "rm-arch__empty",
     this.msg("architecture.copy.no_explicit_frontier")
    ));
   } else {
    flowFrontiers.forEach((frontier) => {
     const row = element("div", "rm-arch__frontier-row");
     row.appendChild(element("strong", null, frontier.kind || frontier.id));
     row.appendChild(element("span", null, frontier.reason));
     this.appendLocation(row, frontier.evidence, this.msg("architecture.label.known_evidence"));
     frontiers.appendChild(row);
    });
   }
   this.appendDiagnostics(section(this.msg("architecture.section.diagnostics")), text(flow.id));
  }

  renderUserFocusedFlow(flow) {
   const flowID = text(flow && flow.id);
   if (!flowID || this.focusFlowID === flowID) return;
   this.focusFlowID = flowID;
   this.flowFocus.replaceChildren();

   const projection = this.primaryFlowSteps(flow);
   const connectedSteps = projection.items;
   const steps = connectedSteps.length > 0 ? connectedSteps : array(flow.steps).slice(0, 8);
   const flowEdges = this.focusedFlowEdges(flowID);
   const header = element("header", "rm-arch__focus-header");
   const back = element(
    "button",
    "rm-arch__focus-back",
    this.msg("architecture.nav.back_to_architecture")
   );
   back.type = "button";
   this.listen(back, "click", () => this.backToArchitecture());
   header.appendChild(back);

   const heading = element("div", "rm-arch__focus-heading");
   heading.appendChild(element("span", "rm-arch__focus-kicker", this.msg("architecture.fallback.code_path")));
   heading.appendChild(element(
    "h3",
    null,
    flow.name || this.msg("architecture.fallback.source_backed_code_path")
   ));
   const explanation = flow.mental_model || flow.goal || flow.why_inspect;
   if (explanation) heading.appendChild(element("p", null, explanation));
   const summary = element("dl", "rm-arch__trace-summary");
   const startSurface = this.surfaceByID.get(text(flow.start_surface_id));
   const trigger = flow.trigger || flow.command || startSurface && (startSurface.name || startSurface.kind);
   this.appendSummaryItem(summary, this.msg("architecture.label.starts_when"), trigger);
   this.appendSummaryItem(summary, this.msg("architecture.label.scope"), flow.scope);
   const participantNames = array(flow.participating_component_ids)
    .map((componentID) => this.componentByID.get(text(componentID)))
    .filter((component) => component && component.name);
   if (participantNames.length > 0) {
    const participating = element("div", "rm-arch__trace-participants");
    participating.appendChild(element("dt", null, this.msg("architecture.label.code_areas")));
    const values = element("dd");
    participantNames.forEach((component) => {
     const button = element("button", null, component.name);
     button.type = "button";
     this.listen(button, "click", () => this.setSelection({
      component: text(component.id), surface: "", step: "", edge: "",
     }, true));
     values.appendChild(button);
    });
    participating.appendChild(values);
    summary.appendChild(participating);
   }
   heading.appendChild(summary);
   header.appendChild(heading);
   if (steps.length > 0) {
    const stats = element("div", "rm-arch__focus-stats");
    stats.appendChild(element(
     "strong",
     null,
     this.msg("architecture.count.code_steps", { count: steps.length })
    ));
    header.appendChild(stats);
   }
   this.flowFocus.appendChild(header);

   if (steps.length > 0) {
    const section = element("section", "rm-arch__focus-section");
    const sectionHeader = element("div", "rm-arch__focus-section-heading");
    sectionHeader.appendChild(element(
     "h4",
     null,
     connectedSteps.length > 0
      ? this.msg("architecture.section.source_linked_sequence")
      : this.msg("architecture.section.implementation_steps")
    ));
    section.appendChild(sectionHeader);
    const path = element("div", "rm-arch__focus-path");
    steps.forEach((step, index) => {
     if (connectedSteps.length > 0 && index > 0) {
      const previous = steps[index - 1];
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
  }

  renderFocusedFlow(flow) {
   if (this.userMode) return this.renderUserFocusedFlow(flow);
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
   const back = element(
    "button",
    "rm-arch__focus-back",
    this.msg("architecture.nav.back_to_architecture")
   );
   back.type = "button";
   this.listen(back, "click", () => this.backToArchitecture());
   header.appendChild(back);
   const heading = element("div", "rm-arch__focus-heading");
   heading.appendChild(element(
    "span",
    "rm-arch__focus-kicker",
    savedTraceLabel(flow.archetype, this.message)
   ));
   heading.appendChild(element("h3", null, flow.name || flow.id));
   heading.appendChild(element(
    "p",
    null,
    flow.why_inspect || flow.mental_model || flow.goal ||
     this.msg("architecture.copy.inspect_static_handoffs")
   ));
   const summary = element("dl", "rm-arch__trace-summary");
   const startSurface = this.surfaceByID.get(text(flow.start_surface_id));
   const trigger = flow.trigger || flow.command || startSurface && (startSurface.name || startSurface.id) ||
    this.msg("architecture.fallback.explicit_investigation");
   const groundedSequence = primary.length > 1 ?
    this.msg("architecture.count.grounded_sequence_many", { count: primary.length }) :
    primary.length === 1
     ? this.msg("architecture.copy.grounded_sequence_one")
     : this.msg("architecture.copy.grounded_sequence_none");
   const concurrentActivities = lifecycle.groups.length > 0 ?
    this.msg("architecture.count.task_branches_grouped", { count: lifecycle.groups.length }) :
    lifecycle.ungroupedTotal > 0
     ? this.msg("architecture.count.lifecycle_without_root", { count: lifecycle.ungroupedTotal })
     : this.msg("architecture.copy.no_concurrent_relation");
   this.appendSummaryItem(summary, this.msg("architecture.label.trigger"), trigger);
   this.appendSummaryItem(
    summary,
    this.msg("architecture.label.what_system_does"),
    flow.mental_model || flow.goal || flow.why_inspect ||
     this.msg("architecture.fallback.bounded_behavior_unresolved")
   );
   const participating = element("div", "rm-arch__trace-participants");
   participating.appendChild(element("dt", null, this.msg("architecture.label.participating_components")));
   const participantValues = element("dd");
   array(flow.participating_component_ids).forEach((componentID) => {
    const component = this.componentByID.get(text(componentID));
    const button = element("button", null, component && (component.name || component.id) || componentID);
    button.type = "button";
    this.listen(button, "click", () => this.setSelection({ component: text(componentID), surface: "", step: "", edge: "" }, true));
    participantValues.appendChild(button);
   });
   if (participantValues.childElementCount === 0) {
    participantValues.appendChild(element("span", null, this.msg("architecture.label.unassigned")));
   }
   participating.appendChild(participantValues);
   summary.appendChild(participating);
   this.appendSummaryItem(summary, this.msg("architecture.label.grounded_sequence"), groundedSequence);
   this.appendSummaryItem(summary, this.msg("architecture.label.concurrent_activities"), concurrentActivities);
    this.appendSummaryItem(
     summary,
     this.msg("architecture.label.current_frontier"),
     flow.frontier_summary || flow.current_frontier || this.msg("architecture.fallback.no_explicit_frontier")
    );
    this.appendSummaryItem(
     summary,
     this.msg("architecture.label.evidence_basis"),
     this.msg("architecture.label.evidence_not_observed", {
      basis: flow.evidence_basis || this.msg("architecture.value.static"),
     })
    );
    this.appendSummaryItem(
     summary,
     this.msg("architecture.label.trace_surfaces"),
     this.msg("architecture.count.trace_evidence_surfaces", {
      count: (flow.seed_surface_id ? 1 : 0) + array(flow.trace_evidence_surface_ids).length,
     })
    );
    this.appendSummaryItem(
     summary,
     this.msg("architecture.label.other_component_surfaces"),
     this.msg("architecture.count.related_not_trace_surfaces", {
      count: array(flow.related_component_surface_ids).length,
     })
    );
   heading.appendChild(summary);
   const proofNode = element("div", "rm-arch__focus-proof is-" + proof.status);
   proofNode.appendChild(element(
    "strong",
    null,
    this.msg("architecture.count.proof_areas_grounded", {
     grounded: proof.grounded,
     total: proof.total,
    })
   ));
   const proofCopy = [];
   if (proof.firstGap) {
    proofCopy.push(
     proofAreaLabel(proof.firstGap.kind, this.message) +
      " " +
      (text(proof.firstGap.status) || this.msg("architecture.label.unknown"))
    );
    if (proof.firstGap.missing) proofCopy.push(text(proof.firstGap.missing));
   }
   proofCopy.push(this.msg("architecture.copy.static_evidence_not_observed"));
   proofNode.appendChild(element("span", null, proofCopy.join(" · ")));
   const proofAction = element(
    "button",
    "rm-arch__focus-proof-action",
    this.msg("architecture.action.inspect_full_evidence")
   );
   proofAction.type = "button";
   this.listen(proofAction, "click", () => {
    evidenceDisclosure.open = true;
    evidenceDisclosure.scrollIntoView({ behavior: "smooth", block: "nearest" });
   });
   proofNode.appendChild(proofAction);
   heading.appendChild(proofNode);
   header.appendChild(heading);
   const stats = element("div", "rm-arch__focus-stats");
   stats.appendChild(element(
    "strong",
    null,
    this.msg("architecture.count.exact_anchors", { count: array(flow.steps).length })
   ));
   stats.appendChild(element(
    "span",
    null,
    this.msg("architecture.count.evidenced_transitions", { count: flowEdges.length })
   ));
   stats.appendChild(element(
    "span",
    null,
    this.msg("architecture.count.trace_lanes", { count: array(flow.branches).length })
   ));
   header.appendChild(stats);
   this.flowFocus.appendChild(header);

   if (primary.length > 0) {
    const section = element("section", "rm-arch__focus-section");
    const sectionHeader = element("div", "rm-arch__focus-section-heading");
    sectionHeader.appendChild(element("h4", null, this.msg("architecture.label.grounded_sequence")));
    const pathNotes = [this.msg("architecture.copy.exact_anchors_static_transitions")];
    if (primaryProjection.total > primary.length) {
     pathNotes.push(this.msg("architecture.count.showing_connected_anchors", {
      shown: primary.length,
      total: primaryProjection.total,
     }));
    }
    if (primaryProjection.disconnected > 0) {
     pathNotes.push(this.msg("architecture.count.role_anchors_unlinked", {
      count: primaryProjection.disconnected,
     }));
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
    sectionHeader.appendChild(element("h4", null, this.msg("architecture.section.key_operations")));
    let operationsCopy = this.msg("architecture.copy.key_operations_limit");
    if (operations.total > operations.items.length) {
     operationsCopy += " " + this.msg("architecture.count.showing_items", {
      shown: operations.items.length,
      total: operations.total,
     });
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
    sectionHeader.appendChild(element("h4", null, this.msg("architecture.label.concurrent_activities")));
    sectionHeader.appendChild(element(
     "p",
     null,
     this.msg("architecture.copy.concurrent_activities_limit")
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
   const group = svgElement("g", { class: className });
   // Decision 229 D1: edges are passive visual evidence — no role, no
   // tabindex, no hitbox, no click/keyboard handler. An optional <title>
   // may describe the line; the same information exists in the
   // selected-node panel (nodes and connection rows are the primary
   // controls).
   if (label) {
    const title = svgElement("title", {});
    title.textContent = label;
    group.appendChild(title);
   }
   group.appendChild(svgElement("path", { class: "rm-arch__edge-visible", d: route }));
   return group;
  }

  installViewportInteractions() {
   // Decision 234 (Archive 9, owner corrective 1): the canvas OWNS the
   // wheel/trackpad input while the pointer is over the map — wheel zooms
   // (one documented stable behavior, no modifier required; Ctrl/Cmd+wheel
   // keeps zooming as a harmless superset). Blank-space drag pans. Pointer
   // outside the canvas: the report page scrolls normally (the handler is
   // installed only on the viewport element, never on the page root). This
   // supersedes the D230 D3 clause that required Ctrl/Cmd+wheel to zoom.
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
   // Decision 230 D3: fit bounds are the union of every group AND every
   // principal node position. A singleton/unassigned principal node must
   // never fall outside the fitted viewport just because it has no group.
   const positions = Array.from(this.groupPositions.values());
   this.nodePositions.forEach((position, id) => {
    if (id === UNASSIGNED_ID) return;
    positions.push(position);
   });
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
    array(flow.steps).some((step) => this.flowStepLaneComponent(flowID, step.id) === UNASSIGNED_ID) ||
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
   // Decision 230 D3 semantic zoom: overview scale shows title/count
   // only; readable scale reveals descriptions and metadata.
   this.root.setAttribute(
    "data-semantic-scale",
    this.view.scale < 0.9 ? "overview" : "readable"
   );
   }

   // Decision 236 (v11): Map lenses are emphasis projections over the
   // SAME landscape layout — one ELK layout, switch by emphasis/dimming,
   // never a relayout and never a view transform write.
   setLens(lens) {
    const value = ["landscape", "entrypoints", "integrations"].indexOf(lens) >= 0
     ? lens : "landscape";
   this.lens = value;
   if (this.root) this.root.setAttribute("data-lens", value);
   if (value !== "entrypoints") this.clearEntrypointHandoffGroup();
   if (value === "landscape") {
     if (this.root) this.root.setAttribute("data-lens-has-emphasis", "false");
     this.components.forEach((component) => {
     const node = this.componentElements.get(text(component && component.id));
     if (node) node.classList.remove("rm-arch__is-lens-emphasized");
     });
     this.renderStudyMechanismOverlay();
     return value;
    }
    const projection = mapLensEmphasisProjection({
     lens: value,
     components: this.components,
     surfaces: this.surfaces,
     associations: this.flatAssociations(),
     entryHandoffGroups: Number(this.data && this.data.version) === 15
      ? this.data.entry_handoff_groups : [],
    });
    const hasEmphasis = projection.emphasized.length > 0;
    if (this.root) this.root.setAttribute("data-lens-has-emphasis", hasEmphasis ? "true" : "false");
    this.components.forEach((component) => {
     const componentID = text(component && component.id);
     const node = this.componentElements.get(componentID);
     if (!node) return;
     const active = projection.emphasized.indexOf(componentID) >= 0;
     node.classList.toggle("rm-arch__is-lens-emphasized", active);
    });
    if (value === "entrypoints") this.renderEntrypointHandoffOverlay();
    this.renderStudyMechanismOverlay();
    return value;
   }

   // Decision 236 (v11): lens emphasis and visible objects are derived by
   // the pure projection below, never guessed from component properties in
   // the renderer — the backend owns entry/touchpoint/context identity.

   // Decision 236 (v11): the DOM-free lens projection is the single source
   // of truth for both the canvas emphasis and the workspace objects panel
   // (projectArchitectureLens on the module) — never a per-instance copy.

   // Decision 236: normalize the backend association view-model
   // (components[].associations[] with the owning component on the parent)
   // into the flat rows the pure projection consumes.
   flatAssociations() {
    const raw = this.options && this.options.associations;
    const out = [];
    if (Array.isArray(raw)) {
     // Flat view-model (component_id on each row) — use as-is.
     raw.forEach((row) => { if (row && typeof row === "object") out.push(row); });
     return out;
    }
    const components = array(raw && raw.components);
    components.forEach((entry) => {
     const componentID = text(entry && entry.component_id);
     array(entry && entry.associations).forEach((row) => {
      if (!row || typeof row !== "object") return;
      out.push(Object.assign({}, row, { component_id: componentID }));
     });
    });
    return out;
   }


   guidedTourStep() {
    if (!this.guidedTour.active) return null;
    return this.guidedTourSteps[this.guidedTour.index] || null;
   }

   guidedTourComponentIDs(step) {
    const ids = new Set();
    array(step && step.component_ids).forEach((componentID) => {
     componentID = text(componentID);
     if (this.componentByID.has(componentID)) ids.add(componentID);
    });
    return Array.from(ids);
   }

   semanticArtifact() {
    return this.semanticArtifactByID.get(text(this.semanticNarrative.artifactID)) || null;
   }

   semanticArtifactSteps(artifact) {
    const steps = array(artifact && artifact.steps).filter((step) => step && typeof step === "object");
    if (steps.length > 0) return steps;
    if (!artifact) return [];
    return [{
     id: text(artifact.id) + ":overview",
     title: artifact.title || this.msg("architecture.fallback.repository_explanation"),
     explanation: artifact.summary || "",
     statement_ids: array(artifact.statements).map((statement) => text(statement && statement.id)).filter(Boolean),
     focus: artifact.focus || {},
     evidence: artifact.evidence || [],
    }];
   }

   semanticArtifactStep() {
    const artifact = this.semanticArtifact();
    if (!artifact) return null;
    return this.semanticArtifactSteps(artifact)[this.semanticNarrative.index] || null;
   }

   semanticArtifactReferenceStep(artifact, step) {
    const artifactFocus = artifact && artifact.focus && typeof artifact.focus === "object" ? artifact.focus : {};
    const stepFocus = step && step.focus && typeof step.focus === "object" ? step.focus : {};
    const focusValues = (key) => {
     const localValues = array(stepFocus[key]).map(text).filter(Boolean);
     return localValues.length > 0 ? localValues : array(artifactFocus[key]).map(text).filter(Boolean);
    };
    return {
     component_ids: focusValues("component_ids"),
     flow_ids: focusValues("flow_ids"),
     surface_ids: focusValues("surface_ids"),
     flow_step_ids: focusValues("flow_step_ids"),
    };
   }

   openSemanticArtifact(artifactID, index) {
    artifactID = text(artifactID);
    if (!this.semanticArtifactByID.has(artifactID)) return;
    if (this.traceList && !this.traceList.hidden) this.toggleTraceMenu(false);
    this.deactivateGuidedTour();
    this.semanticNarrative.artifactID = artifactID;
    this.returnHighlightIDs = new Set();
    this.showSemanticArtifactStep(index, true);
   }

   showSemanticArtifactStep(index, animate) {
    const artifact = this.semanticArtifact();
    const steps = this.semanticArtifactSteps(artifact);
    if (!artifact || steps.length === 0) return;
    const requested = Number(index);
    const nextIndex = clamp(Number.isFinite(requested) ? Math.trunc(requested) : 0, 0, steps.length - 1);
    this.semanticNarrative.index = nextIndex;
    const referenceStep = this.semanticArtifactReferenceStep(artifact, steps[nextIndex]);
    const componentIDs = this.guidedTourComponentIDs(referenceStep);
    const primaryComponentID = componentIDs[0] || "";
    const primaryFlowID = primaryComponentID
     ? ""
     : array(referenceStep.flow_ids).map(text).find((flowID) => this.flowByID.has(flowID)) || "";
    const primarySurfaceID = primaryComponentID || primaryFlowID
     ? ""
     : array(referenceStep.surface_ids).map(text).find((surfaceID) => this.surfaceByID.has(surfaceID)) || "";
    this.setSelection({
     flow: primaryFlowID, component: primaryComponentID, surface: primarySurfaceID, step: "", edge: "",
    }, false, true);
    requestAnimationFrame(() => {
     if (!this.semanticArtifact() || this.semanticNarrative.index !== nextIndex) return;
     if (primaryComponentID) this.focusComponent(primaryComponentID, animate);
     else if (!primaryFlowID && !primarySurfaceID) this.fit();
    });
   }

   moveSemanticArtifact(delta) {
    this.showSemanticArtifactStep(this.semanticNarrative.index + Number(delta || 0), true);
   }

   deactivateSemanticArtifact() {
    this.semanticNarrative.artifactID = "";
    this.semanticNarrative.index = 0;
   }

   finishSemanticArtifact(focusTrigger) {
    if (!this.semanticArtifact()) return;
    this.deactivateSemanticArtifact();
    this.returnHighlightIDs = new Set();
    this.setSelection({ flow: "", component: "", surface: "", step: "", edge: "" }, true);
    requestAnimationFrame(() => {
     this.fit();
     if (focusTrigger && this.startHereButton) this.startHereButton.focus();
     else if (focusTrigger && this.landscapeButton) this.landscapeButton.focus();
    });
   }

   startGuidedTour() {
    this.openGuidedTourStep(0);
   }

   openGuidedTourStep(index) {
    if (!this.guidedTourStory || this.guidedTourSteps.length === 0) return;
    if (this.traceList && !this.traceList.hidden) this.toggleTraceMenu(false);
    this.deactivateSemanticArtifact();
    this.guidedTour.active = true;
    this.returnHighlightIDs = new Set();
    this.showGuidedTourStep(index, true);
   }

   showGuidedTourStep(index, animate) {
    if (!this.guidedTour.active || this.guidedTourSteps.length === 0) return;
    const requested = Number(index);
    const nextIndex = clamp(
     Number.isFinite(requested) ? Math.trunc(requested) : 0,
     0,
     this.guidedTourSteps.length - 1
    );
    this.guidedTour.index = nextIndex;
    const componentIDs = this.guidedTourComponentIDs(this.guidedTourSteps[nextIndex]);
    const primaryComponentID = componentIDs[0] || "";
    this.setSelection({
     flow: "", component: primaryComponentID, surface: "", step: "", edge: "",
    }, false, true);
    requestAnimationFrame(() => {
     if (!this.guidedTour.active || this.guidedTour.index !== nextIndex) return;
     if (primaryComponentID) this.focusComponent(primaryComponentID, animate);
     else this.fit();
    });
   }

   moveGuidedTour(delta) {
    this.showGuidedTourStep(this.guidedTour.index + Number(delta || 0), true);
   }

   deactivateGuidedTour() {
    this.guidedTour.active = false;
   }

   finishGuidedTour(focusTrigger) {
    if (!this.guidedTour.active) return;
    this.deactivateGuidedTour();
    this.returnHighlightIDs = new Set();
    this.setSelection({ flow: "", component: "", surface: "", step: "", edge: "" }, true);
    requestAnimationFrame(() => {
     this.fit();
     if (focusTrigger && this.guidedTourButton) this.guidedTourButton.focus();
    });
   }

   setSelection(patch, writeHash, preserveNarrative) {
     if (!preserveNarrative) {
      if (this.guidedTour.active) this.deactivateGuidedTour();
      if (this.semanticArtifact()) this.deactivateSemanticArtifact();
     }
     const previous = this.selection;
     const next = Object.assign({}, this.selection, patch || {});
     this.selection = this.validateSelection(next);
     if (!previous.flow && this.selection.flow) {
      this.landscapeView = Object.assign({}, this.view);
      this.landscapeComponentID = previous.component;
      this.returnHighlightIDs = new Set();
     }
     if (previous.flow && !this.selection.flow && !this.guidedTour.active && !this.semanticArtifact()) {
      this.returnHighlightIDs = new Set(this.flowComponentIDs.get(previous.flow) || []);
     }
     if (this.selection.flow && this.selection.component) {
      this.landscapeComponentID = this.selection.component;
     }
     if (writeHash && !this.userMode) this.writeHash();
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

   openFlowStep(flowID, stepID) {
    flowID = text(flowID);
    stepID = text(stepID);
    if (!this.flowByID.has(flowID)) return;
    const step = this.flowStepsByKey.get(flowStepKey(flowID, stepID));
    if (!step) return;
   this.setSelection({
     flow: flowID,
     component: this.flowStepSelectionComponent(flowID, stepID),
     surface: "",
     step: stepID,
     edge: "",
    }, true);
    requestAnimationFrame(() => {
     if (this.selection.flow !== flowID || this.selection.step !== stepID) return;
     const target = Array.from(this.flowFocus.querySelectorAll("[data-step-id]")).find(
      (node) => node.dataset.stepId === stepID
     );
     if (!target) return;
     target.scrollIntoView({ behavior: "smooth", block: "nearest" });
     target.focus({ preventScroll: true });
    });
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
    if (this.semanticArtifact()) {
     this.finishSemanticArtifact(false);
     return;
    }
    if (this.guidedTour.active) {
     this.finishGuidedTour(false);
     return;
    }
    const returnComponentID = this.selection.component;
    this.setSelection({ component: "", surface: "", step: "", edge: "" }, true);
    // Decision 230 D4: focus returns to the component that opened the
    // inspector — never to <body>.
    if (returnComponentID) {
     requestAnimationFrame(() => {
      if (this.destroyed) return;
      const card = this.componentElements.get(text(returnComponentID));
      if (card && card.querySelector && card.querySelector(".rm-arch__component-card")) {
       card.querySelector(".rm-arch__component-card").focus({ preventScroll: true });
      } else if (card) {
       card.focus({ preventScroll: true });
      }
     });
    }
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
   if (this.userMode) return { flow: "", component: "", surface: "", step: "", edge: "" };
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
   if (this.userMode) {
    this.selection = this.validateSelection({});
    if (render && this.surface) this.renderSelection();
    return;
   }
   if (render && this.guidedTour.active) this.deactivateGuidedTour();
   if (render && this.semanticArtifact()) this.deactivateSemanticArtifact();
   this.selection = this.validateSelection(this.readHash());
   if (render && this.surface) this.renderSelection();
  }

  writeHash() {
   if (this.userMode) return;
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
   const guidedTourActive = Boolean(this.guidedTour.active && this.guidedTourStep());
   const semanticArtifact = this.semanticArtifact();
   const semanticStep = this.semanticArtifactStep();
   const semanticArtifactActive = Boolean(semanticArtifact && semanticStep);
   this.root.classList.toggle("has-guided-tour", guidedTourActive);
   this.root.classList.toggle("has-semantic-artifact", semanticArtifactActive);
   if (this.startHereButton) {
    const startHereActive = semanticArtifactActive && text(semanticArtifact.id) === this.startHereArtifactID;
    this.startHereButton.classList.toggle("is-active", startHereActive);
    this.startHereButton.setAttribute("aria-pressed", startHereActive ? "true" : "false");
   }
   if (this.guidedTourButton) {
    this.guidedTourButton.classList.toggle("is-active", guidedTourActive);
    this.guidedTourButton.setAttribute("aria-pressed", guidedTourActive ? "true" : "false");
   }
   if (!this.surface) {
    this.renderInspector();
    return;
   }
    const flowID = this.selection.flow;
    const hasFlow = Boolean(flowID);
    const guidedComponentIDs = new Set(
     guidedTourActive ? this.guidedTourComponentIDs(this.guidedTourStep()) : []
    );
    const semanticComponentIDs = new Set(
     semanticArtifactActive
      ? this.guidedTourComponentIDs(this.semanticArtifactReferenceStep(semanticArtifact, semanticStep))
      : []
    );
   this.root.classList.toggle("has-selected-flow", hasFlow);
     this.landscapeButton.classList.toggle("is-active", !hasFlow);
     this.landscapeButton.textContent = hasFlow
      ? this.msg("architecture.nav.back_to_architecture")
      : this.msg("architecture.nav.architecture");
     this.flowButtons.forEach((button, id) => button.classList.toggle("is-active", id === flowID));
     if (this.diagnosticButton) {
      this.diagnosticButton.classList.toggle("is-active", this.diagnosticComponentIDs.has(this.selection.component));
     }
    this.surface.hidden = hasFlow;
    this.flowFocus.hidden = !hasFlow;
    if (hasFlow) this.renderFocusedFlow(this.flowByID.get(flowID));
    this.syncFocusedSelection();

   const relatedComponents = this.flowComponentIDs.get(flowID) || new Set();
    // Decision 229 D1 selected-node focus: one-hop incoming/outgoing exact
    // neighbors stay prominent; unrelated principal nodes dim but remain
    // spatially stable. Dimming is purely visual (is-dimmed), never
    // removes or repositions a node.
    const focusComponentID = this.selection.component && !hasFlow ? this.selection.component : "";
    const focusNeighbors = focusComponentID ? this.structuralNeighborComponentIDs(focusComponentID) : new Set();
    this.componentElements.forEach((node, id) => {
     node.classList.toggle("is-selected", id === this.selection.component);
    node.classList.toggle("is-guided-tour-highlight", guidedComponentIDs.has(id));
    node.classList.toggle("is-semantic-artifact-highlight", semanticComponentIDs.has(id));
    node.classList.toggle("is-unrelated", hasFlow && !relatedComponents.has(id));
    node.classList.toggle("is-flow-related", hasFlow && relatedComponents.has(id));
    node.classList.toggle("is-dimmed", Boolean(focusComponentID) && id !== focusComponentID && !focusNeighbors.has(id));
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
     array(this.flowByID.get(flowID) && this.flowByID.get(flowID).steps).some(
      (step) => this.flowStepLaneComponent(flowID, step.id) === UNASSIGNED_ID
     ) ||
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

  // structuralNeighborComponentIDs returns the exact one-hop incoming and
  // outgoing structural neighbor component IDs of the given component
  // (Decision 229 D1 selected-node focus). Derived only from the local
  // structural edges — never runtime dependency or reachability.
  structuralNeighborComponentIDs(componentID) {
   const ids = new Set();
   this.structuralEdgeByID.forEach((edge) => {
    const from = text(edge && edge.from_component_id);
    const to = text(edge && edge.to_component_id);
    if (from === componentID && to) ids.add(to);
    if (to === componentID && from) ids.add(from);
   });
   return ids;
  }

  renderInspector() {
   const semanticArtifactActive = Boolean(this.semanticArtifact());
   const selectedComponent = this.selection.component && this.componentByID.get(this.selection.component);
   const lowInformationComponent = Boolean(
    this.userMode && selectedComponent && !this.selection.surface && !this.selection.step && !this.selection.edge &&
    !this.userComponentHasInspector(selectedComponent)
   );
   const visible = semanticArtifactActive || this.guidedTour.active ||
    (this.hasInspectorSelection(this.selection) && !lowInformationComponent);
   if (this.inspectorVisible !== visible) {
    this.inspectorVisible = visible;
    if (typeof this.options.onInspectorVisibilityChange === "function") {
     this.options.onInspectorVisibilityChange(visible);
    }
   }
    this.inspector.setAttribute(
     "aria-label",
     semanticArtifactActive
      ? this.msg("architecture.aria.repository_explanation")
      : (this.guidedTour.active
       ? this.msg("architecture.aria.guided_tour")
       : this.msg("architecture.aria.inspector"))
    );
    this.root.classList.toggle("has-detail-inspector", visible);
    this.root.classList.toggle(
     "has-user-compact-inspector",
     Boolean(this.userMode && visible && selectedComponent && !this.selection.surface && !this.selection.step && !this.selection.edge)
    );
    this.inspector.hidden = !visible;
    this.drawerBackdrop.hidden = !visible;
    this.inspector.replaceChildren();
    if (!visible) return;
    const close = element("button", "rm-arch__inspector-close", "×");
    close.type = "button";
    close.setAttribute("aria-label", this.msg("architecture.aria.close_inspector"));
    this.listen(close, "click", () => this.closeInspector());
    this.inspector.appendChild(close);
    // Decision 230 D4: when the inspector opens as a drawer the focus
    // enters the close control (Escape already closes it). Applies to
    // the modal overlay and the compact bottom sheet alike.
    requestAnimationFrame(() => {
     if (this.destroyed || this.inspector.hidden) return;
     close.focus({ preventScroll: true });
    });
    if (!this.userMode) {
     // Keyboard focus stays inside the drawer while it is open: Tab
     // cycles between the first and last focusable element. Background
     // map nodes are never keyboard-reachable through the overlay.
     const trapFocus = (event) => {
      if (this.inspector.hidden) return;
      if (event.key !== "Tab") return;
      // Decision 230 (fresh review task-4 B1): collapsed witness lists and
      // other hidden groups must not be tab stops — filter out elements
      // that are hidden themselves or inside a hidden ancestor so the
      // wrap check fires at the true last VISIBLE control and Tab never
      // falls through the modal into the background.
      const rawFocusables = this.inspector.querySelectorAll(
       'button:not([disabled]), a[href], [tabindex]:not([tabindex="-1"]), input, select, textarea'
      );
      const focusables = Array.from(rawFocusables).filter((candidate) => {
       let node = candidate;
       while (node && node !== this.inspector) {
        if (node.hidden) return false;
        if (node.getAttribute && node.getAttribute("aria-hidden") === "true") return false;
        node = node.parentNode || null;
        if (node && node.parentNode === node) break;
       }
       return true;
      });
      if (!focusables.length) return;
      const first = focusables[0];
      const last = focusables[focusables.length - 1];
      if (event.shiftKey && document.activeElement === first) {
       event.preventDefault();
       last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
       event.preventDefault();
       first.focus();
      }
     };
     this.listen(this.inspector, "keydown", trapFocus);
    }
    if (semanticArtifactActive) return this.renderSemanticArtifactInspector();
    if (this.guidedTour.active) return this.renderGuidedTourInspector();
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

  renderSemanticArtifactInspector() {
   const artifact = this.semanticArtifact();
   const steps = this.semanticArtifactSteps(artifact);
   const step = this.semanticArtifactStep();
   if (!artifact || !step) return;

   this.inspectorHeading(
    artifact.kind
     ? this.guidedTourValueLabel(artifact.kind)
     : this.msg("architecture.fallback.repository_explanation"),
    artifact.title || artifact.question || this.msg("architecture.fallback.repository_explanation"),
    artifact.summary
   );

   const overview = this.inspectorSection(this.msg("architecture.section.what_this_answers"));
   if (artifact.question) overview.appendChild(element("p", "rm-arch__copy", artifact.question));
   this.appendKeyValue(
    overview,
    this.msg("architecture.label.verdict"),
    this.guidedTourValueLabel(artifact.verdict || this.msg("architecture.label.unknown"))
   );
   this.appendKeyValue(
    overview,
    this.msg("architecture.label.confidence"),
    this.guidedTourValueLabel(artifact.confidence || this.msg("architecture.label.unknown"))
   );
   const requiredAspects = array(artifact.required_answer_aspects);
   if (requiredAspects.length > 0) {
    this.appendKeyValue(
     overview,
     this.msg("architecture.label.answer_coverage"),
     this.msg("architecture.count.answer_aspects", {
      covered: array(artifact.covered_answer_aspects).length,
      total: requiredAspects.length,
     })
    );
   }
   if (artifact.verdict === "insufficient_evidence") {
    overview.appendChild(element(
     "p",
     "rm-arch__notice is-warning",
     this.msg("architecture.copy.insufficient_evidence")
    ));
   }

   const stepCard = element("section", "rm-arch__tour-step rm-arch__semantic-step");
   stepCard.appendChild(element(
    "div",
    "rm-arch__tour-progress",
    this.msg("architecture.count.step_progress", {
     current: this.semanticNarrative.index + 1,
     total: steps.length,
    })
   ));
   stepCard.appendChild(element(
    "h4",
    "rm-arch__tour-step-title",
    step.title || this.msg("architecture.fallback.explanation_step")
   ));
   if (step.explanation) stepCard.appendChild(element("p", "rm-arch__copy", step.explanation));

   const actions = element("div", "rm-arch__tour-actions");
   const back = element("button", "rm-arch__tour-action", this.msg("architecture.action.back"));
   back.type = "button";
   back.disabled = this.semanticNarrative.index === 0;
   this.listen(back, "click", () => this.moveSemanticArtifact(-1));
   const next = element("button", "rm-arch__tour-action is-primary", this.msg("architecture.action.next"));
   next.type = "button";
   next.disabled = this.semanticNarrative.index === steps.length - 1;
   this.listen(next, "click", () => this.moveSemanticArtifact(1));
   const evidence = element("button", "rm-arch__tour-action", this.msg("architecture.action.evidence"));
   evidence.type = "button";
   this.listen(evidence, "click", () => {
    const target = this.inspector.querySelector("[data-semantic-artifact-evidence]");
    if (target && typeof target.scrollIntoView === "function") {
     target.scrollIntoView({ behavior: "smooth", block: "start" });
    }
   });
   const fullMap = element("button", "rm-arch__tour-action is-quiet", this.msg("architecture.action.full_map"));
   fullMap.type = "button";
   this.listen(fullMap, "click", () => this.finishSemanticArtifact(true));
   actions.append(back, next, evidence, fullMap);
   stepCard.appendChild(actions);
   this.inspector.appendChild(stepCard);

   const statementByID = new Map(
    array(artifact.statements).filter(Boolean).map((statement) => [text(statement.id), statement])
   );
   const requestedStatementIDs = array(step.statement_ids).map(text).filter(Boolean);
   const statements = (requestedStatementIDs.length > 0
    ? requestedStatementIDs.map((statementID) => statementByID.get(statementID)).filter(Boolean)
    : array(artifact.statements).filter(Boolean));
   const statementsSection = this.inspectorSection(this.msg("architecture.section.claims_in_step"));
   if (statements.length === 0) {
    statementsSection.appendChild(element(
     "p",
     "rm-arch__empty",
     this.msg("architecture.copy.no_materialized_claim")
    ));
   }
   statements.forEach((statement) => {
    const basis = text(statement.basis || "unresolved");
    const card = element(
     "article",
     "rm-arch__notice rm-arch__semantic-statement " +
      (basis === "unresolved" || basis === "interpretive" ? "is-warning" : "is-info")
    );
    card.appendChild(element("span", "rm-arch__badge", this.guidedTourValueLabel(basis)));
    card.appendChild(element(
     "p",
     null,
     statement.text || this.msg("architecture.fallback.untitled_claim")
    ));
    const supportCount = array(statement.support_ids).length;
    const sourceGroupCount = array(statement.source_groups).length;
    if (supportCount || sourceGroupCount) {
     card.appendChild(element(
      "small",
      "rm-arch__semantic-support",
      [
       supportCount ? this.msg("architecture.count.supporting_facts", { count: supportCount }) : "",
       sourceGroupCount ? this.msg("architecture.count.evidence_groups", { count: sourceGroupCount }) : "",
      ].filter(Boolean).join(" · ")
     ));
    }
    statementsSection.appendChild(card);
   });

   const referenceStep = this.semanticArtifactReferenceStep(artifact, step);
   const references = this.inspectorSection(this.msg("architecture.section.related_map_objects"));
   this.appendGuidedTourReferences(references, referenceStep);

   const evidenceSection = this.inspectorSection(this.msg("architecture.section.evidence"));
   evidenceSection.classList.add("rm-arch__tour-evidence");
   evidenceSection.setAttribute("data-semantic-artifact-evidence", "true");
   const evidenceItems = array(step.evidence).length > 0 ? step.evidence : artifact.evidence;
   if (this.appendGuidedTourEvidence(evidenceSection, evidenceItems) === 0) {
    evidenceSection.appendChild(element(
     "p",
     "rm-arch__empty",
     this.msg("architecture.copy.no_exact_evidence_record")
    ));
   }

   const unknowns = array(artifact.unknowns).map(text).filter(Boolean);
   if (unknowns.length > 0) {
    const gaps = this.inspectorSection(this.msg("architecture.section.known_gaps"));
    unknowns.forEach((unknown) => {
     gaps.appendChild(element("p", "rm-arch__notice is-warning", unknown));
    });
   }
  }

  guidedTourValueLabel(value) {
   return presentationValueLabel(value);
  }

  guidedTourUsesEditorialOrder() {
   const basis = text(this.guidedTourStory && this.guidedTourStory.ordering_basis).toLowerCase();
   return basis.indexOf("editorial") >= 0;
  }

  renderGuidedTourInspector() {
   const story = this.guidedTourStory;
   const step = this.guidedTourStep();
   if (!story || !step) return;

   this.inspectorHeading(
    this.msg("architecture.nav.guided_tour"),
    story.title || this.msg("architecture.nav.start_here"),
    story.summary
   );
   const stepCard = element("section", "rm-arch__tour-step");
   stepCard.appendChild(element(
    "div",
    "rm-arch__tour-progress",
    this.msg("architecture.count.step_progress", {
     current: this.guidedTour.index + 1,
     total: this.guidedTourSteps.length,
    })
   ));
   stepCard.appendChild(element(
    "h4",
    "rm-arch__tour-step-title",
    step.title || this.msg("architecture.fallback.orientation_step")
   ));
   if (step.explanation) stepCard.appendChild(element("p", "rm-arch__copy", step.explanation));
   this.appendKeyValue(
    stepCard,
    this.msg("architecture.label.story_kind"),
    this.guidedTourValueLabel(story.candidate_kind)
   );
   this.appendKeyValue(
    stepCard,
    this.msg("architecture.label.ordering"),
    this.guidedTourValueLabel(story.ordering_basis)
   );
   if (story.trigger) this.appendKeyValue(stepCard, this.msg("architecture.label.starts_when"), story.trigger);
   if (this.guidedTourUsesEditorialOrder()) {
    stepCard.appendChild(element(
     "p",
     "rm-arch__notice is-warning rm-arch__tour-disclaimer",
     this.msg("architecture.copy.editorial_order_limit")
    ));
   }

   const actions = element("div", "rm-arch__tour-actions");
   const back = element("button", "rm-arch__tour-action", this.msg("architecture.action.back"));
   back.type = "button";
   back.disabled = this.guidedTour.index === 0;
   this.listen(back, "click", () => this.moveGuidedTour(-1));
   const next = element("button", "rm-arch__tour-action is-primary", this.msg("architecture.action.next"));
   next.type = "button";
   next.disabled = this.guidedTour.index === this.guidedTourSteps.length - 1;
   this.listen(next, "click", () => this.moveGuidedTour(1));
   const evidence = element("button", "rm-arch__tour-action", this.msg("architecture.action.evidence"));
   evidence.type = "button";
   this.listen(evidence, "click", () => {
    const target = this.inspector.querySelector("[data-guided-tour-evidence]");
    if (target && typeof target.scrollIntoView === "function") {
     target.scrollIntoView({ behavior: "smooth", block: "start" });
    }
   });
   const fullMap = element("button", "rm-arch__tour-action is-quiet", this.msg("architecture.action.full_map"));
   fullMap.type = "button";
   this.listen(fullMap, "click", () => this.finishGuidedTour(true));
   actions.append(back, next, evidence, fullMap);
   stepCard.appendChild(actions);
   this.inspector.appendChild(stepCard);

   const references = this.inspectorSection(this.msg("architecture.section.related_map_objects"));
   this.appendGuidedTourReferences(references, step);

   const evidenceSection = this.inspectorSection(this.msg("architecture.section.evidence"));
   evidenceSection.classList.add("rm-arch__tour-evidence");
   evidenceSection.setAttribute("data-guided-tour-evidence", "true");
   if (this.appendGuidedTourEvidence(evidenceSection, step.evidence) === 0) {
    evidenceSection.appendChild(element(
     "p",
     "rm-arch__empty",
     this.msg("architecture.copy.no_exact_evidence_record")
    ));
   }

   const gapSummaries = array(story.gap_summary).filter((summary) => summary && typeof summary === "object");
   if (gapSummaries.length > 0) {
    const gapSection = this.inspectorSection(this.msg("architecture.section.known_gaps"));
    gapSummaries.forEach((summary) => {
     array(summary.gaps).forEach((gap) => {
      if (!gap || typeof gap !== "object") return;
      const card = element("article", "rm-arch__notice is-warning rm-arch__tour-gap");
      card.appendChild(element(
       "strong",
       null,
       gap.label || this.msg("architecture.fallback.unresolved_gap")
      ));
      if (summary.explanation) card.appendChild(element("p", null, summary.explanation));
      if (gap.detail) card.appendChild(element("p", null, gap.detail));
      this.appendGuidedTourEvidence(card, gap.evidence);
      gapSection.appendChild(card);
     });
    });
   }
  }

  appendGuidedTourReferences(parent, step) {
   let count = 0;
   this.guidedTourComponentIDs(step).forEach((componentID) => {
    const component = this.componentByID.get(componentID);
    if (!component) return;
    const button = element("button", "rm-arch__edge-jump");
    button.type = "button";
    button.appendChild(element("strong", null, component.name || component.id));
    button.appendChild(element("span", null, this.msg("architecture.label.component")));
    this.listen(button, "click", () => this.openComponent(componentID));
    parent.appendChild(button);
    count++;
   });

   const surfaceIDs = new Set(array(step && step.surface_ids).map(text));
   surfaceIDs.forEach((surfaceID) => {
    const surface = this.surfaceByID.get(surfaceID);
    if (!surface) return;
    const button = element("button", "rm-arch__edge-jump");
    button.type = "button";
    button.appendChild(element("strong", null, surface.name || surface.id));
    button.appendChild(element(
     "span",
     null,
     [surface.kind, surface.status].filter(Boolean).join(" · ") || this.msg("architecture.label.surface")
    ));
    this.listen(button, "click", () => this.openSurface(surfaceID));
    parent.appendChild(button);
    count++;
   });

   const flowIDs = new Set(array(step && step.flow_ids).map(text));
   flowIDs.forEach((flowID) => {
    const flow = this.flowByID.get(flowID);
    if (!flow) return;
    const button = element("button", "rm-arch__edge-jump");
    button.type = "button";
    button.appendChild(element("strong", null, flow.name || flow.id));
    button.appendChild(element(
     "span",
     null,
     [savedTraceLabel(flow.archetype, this.message), flow.status].filter(Boolean).join(" · ")
    ));
    this.listen(button, "click", () => this.openTrace(flowID));
    parent.appendChild(button);
    count++;

    const flowStepIDs = new Set(array(step.flow_step_ids).map(text));
    flowStepIDs.forEach((stepID) => {
     const flowStep = this.flowStepsByKey.get(flowStepKey(flowID, stepID));
     if (!flowStep) return;
     parent.appendChild(this.stepJumpButton(flow, flowStep));
     count++;
    });
   });

   if (count === 0) {
    parent.appendChild(element(
     "p",
     "rm-arch__empty",
     this.msg("architecture.copy.no_referenced_canvas_object")
    ));
   }
  }

  appendGuidedTourEvidence(parent, evidenceItems) {
   let count = 0;
   array(evidenceItems).forEach((item) => {
    if (!item || typeof item !== "object") return;
    const card = element("article", "rm-arch__evidence-card rm-arch__tour-evidence-card");
    card.appendChild(element(
     "strong",
     "rm-arch__evidence-title",
     item.label || item.kind || item.id || this.msg("architecture.section.evidence")
    ));
    if (item.kind) card.appendChild(element("span", "rm-arch__badge", this.guidedTourValueLabel(item.kind)));
    if (item.id) card.appendChild(element("code", "rm-arch__member-id", item.id));
    this.appendLocation(
     card,
     item.location || (item.path ? item : null),
     this.msg("architecture.label.exact_source")
    );
    parent.appendChild(card);
    count++;
   });
   return count;
  }

  inspectorHeading(kind, title, subtitle) {
   this.inspector.appendChild(element("div", "rm-arch__inspector-kicker", kind));
   this.inspector.appendChild(element("h3", "rm-arch__inspector-title", title));
   if (subtitle) this.inspector.appendChild(element("p", "rm-arch__inspector-summary", subtitle));
  }

  inspectorSection(title, parent) {
   const section = element("section", "rm-arch__inspector-section");
   section.appendChild(element("h4", "rm-arch__inspector-section-title", title));
   (parent || this.inspector).appendChild(section);
   return section;
  }

  userComponentContext(component) {
   if (!component || !component.id) return null;
   const context = this.componentContexts[text(component.id)];
   return context && typeof context === "object" ? context : null;
  }

  userComponentActions(component) {
   const context = this.userComponentContext(component);
   if (!context) return [];
   const actions = [];
   if (typeof this.options.openSourceLocation === "function") {
	const sourceLocations = new Set();
	const appendSource = (source) => {
	 if (!source || source.actionable === false || !locationLabel(source.location)) return;
	 const location = source.location;
	 const key = text(location.path) + "\u0000" + (Number(location.line) || 0) + "\u0000" + (Number(location.column) || 0);
	 if (sourceLocations.has(key)) return;
	 sourceLocations.add(key);
	 actions.push({ kind: "source", value: source });
	};
	array(context.sources).forEach(appendSource);
	if (sourceLocations.size === 0) {
	 array(context.package_targets).forEach((target) => {
	  appendSource({
	   detail: target && target.path,
	   label: target && target.path,
	   location: target && target.location,
	   actionable: target && target.actionable,
	   source_type: "package",
	  });
	 });
	}
   }
   if (typeof this.options.openStudyDirection === "function") {
    array(context.studies).slice(0, 3).forEach((study) => {
     if (!study || !text(study.id) || !text(study.question)) return;
     actions.push({ kind: "study", value: study });
    });
   }
   return actions;
  }

  userComponentHasInspector(component) {
   const context = this.userComponentContext(component);
   return !!(context && (
    this.userComponentActions(component).length > 0 ||
    array(context.surface_starts).length > 0 ||
    array(context.package_paths).length > 0 ||
    array(context.structural_relations).length > 0
   ));
  }

  userComponentTabs(component) {
   const serial = (this.componentInspectorTabSerial || 0) + 1;
   this.componentInspectorTabSerial = serial;
   const prefix = "rm-arch-component-inspector-" + serial + "-" +
    text(component && component.id).replace(/[^A-Za-z0-9_-]/g, "-");
   const definitions = [
    { id: "summary", label: this.msg("architecture.tab.summary") },
    { id: "connections", label: this.msg("architecture.tab.connections") },
    { id: "read-code", label: this.msg("architecture.tab.read_code") },
   ];
   const tablist = element("div", "rm-arch__inspector-tabs");
   tablist.setAttribute("role", "tablist");
   tablist.setAttribute("aria-orientation", "horizontal");
   tablist.setAttribute("aria-label", this.msg("architecture.aria.component_inspector_tabs"));
   const tabs = [];
   const panels = {};
   const activate = (index, moveFocus) => {
    tabs.forEach((tab, candidateIndex) => {
     const active = candidateIndex === index;
     tab.setAttribute("aria-selected", String(active));
     tab.setAttribute("tabindex", active ? "0" : "-1");
     panels[definitions[candidateIndex].id].hidden = !active;
    });
    if (moveFocus && tabs[index]) tabs[index].focus({ preventScroll: true });
   };
   definitions.forEach((definition, index) => {
    const tab = element("button", "rm-arch__inspector-tab", definition.label);
    const tabID = prefix + "-" + definition.id + "-tab";
    const panelID = prefix + "-" + definition.id + "-panel";
    tab.type = "button";
    tab.setAttribute("id", tabID);
    tab.setAttribute("role", "tab");
    tab.setAttribute("aria-controls", panelID);
    const panel = element("div", "rm-arch__inspector-panel rm-arch__inspector-panel--" + definition.id);
    panel.setAttribute("id", panelID);
    panel.setAttribute("role", "tabpanel");
    panel.setAttribute("aria-labelledby", tabID);
    panel.setAttribute("tabindex", "0");
    panels[definition.id] = panel;
    this.listen(tab, "click", () => activate(index, false));
    this.listen(tab, "keydown", (event) => {
     let target = -1;
     if (event.key === "ArrowRight") target = (index + 1) % definitions.length;
     else if (event.key === "ArrowLeft") target = (index + definitions.length - 1) % definitions.length;
     else if (event.key === "Home") target = 0;
     else if (event.key === "End") target = definitions.length - 1;
     if (target < 0) return;
     event.preventDefault();
     activate(target, true);
    });
    tabs.push(tab);
    tablist.appendChild(tab);
   });
   this.inspector.appendChild(tablist);
   definitions.forEach((definition) => this.inspector.appendChild(panels[definition.id]));
   activate(0, false);
   return panels;
  }

  userComponentEntryKindLabel(kind) {
   const ids = {
    async_task: "main.overview.anatomy.surface_kind.async_task",
    cli_command: "main.overview.anatomy.surface_kind.cli_command",
    http_route: "main.overview.anatomy.surface_kind.http_route",
    http_route_descriptor: "main.overview.anatomy.surface_kind.http_route_descriptor",
    http_route_frontier: "main.overview.anatomy.surface_kind.http_route_frontier",
    http_server: "main.overview.anatomy.surface_kind.http_server",
    process_entry: "main.overview.anatomy.surface_kind.process_entry",
    worker: "main.overview.anatomy.surface_kind.worker",
    other: "main.overview.anatomy.surface_kind.other",
   };
   return ids[text(kind)] ? this.msg(ids[text(kind)]) : text(kind) || this.msg(ids.other);
  }

  userComponentEntryGroups(component, surfaceStarts) {
   const startsByID = new Map(array(surfaceStarts).map((start) => [text(start && start.id), start]));
   const groups = new Map();
   const add = (id, kind, label, start) => {
    id = text(id);
    kind = text(kind) || "other";
    if (!groups.has(kind)) groups.set(kind, []);
    const entries = groups.get(kind);
    if (entries.some((entry) => entry.id === id && id)) return;
    entries.push({ id: id, kind: kind, label: text(label), start: start || null });
   };
   this.componentSurfaces(component).forEach((surface) => {
    add(surface.id, surface.kind, surface.name || surface.kind, startsByID.get(text(surface.id)));
   });
   array(surfaceStarts).forEach((start) => {
    const id = text(start && start.id);
    const anchor = this.anchorByID.get(id);
    add(id, anchor && anchor.kind, start && start.label, start);
   });
   return Array.from(groups).map((entry) => ({ kind: entry[0], entries: entry[1] }));
  }

  userComponentReadStarts(context, sourceActions, surfaceStarts) {
   const starts = [];
   const locations = new Set();
   const append = (candidate) => {
    const location = candidate && candidate.location;
    if (!locationLabel(location)) return;
    const key = text(location.path) + "\u0000" + (Number(location.line) || 0) + "\u0000" +
     (Number(location.column) || 0);
    if (locations.has(key)) return;
    locations.add(key);
    starts.push(candidate);
   };
   array(sourceActions).forEach((action) => {
    const source = action && action.value;
    append({
     label: source && (source.detail || source.label),
     location: source && source.location,
     actionable: source && source.actionable !== false && typeof this.options.openSourceLocation === "function",
     source_type: text(source && source.source_type) || text(source && source.member_id && source.member_id.kind) || "source",
     open: () => this.options.openSourceLocation(source.location),
    });
   });
   array(surfaceStarts).forEach((start) => {
    const surface = this.surfaceByID.get(text(start && start.id));
    const anchor = this.anchorByID.get(text(start && start.id));
    const kind = text(surface && surface.kind) || text(anchor && anchor.kind) || "surface";
    append({
     label: start && start.label,
     location: start && start.location,
     actionable: start && start.actionable && typeof this.options.openSourceLocation === "function",
     source_type: kind === "process_entry" ? "process_entry" : "surface",
     open: () => this.options.openSourceLocation(start.location),
    });
   });
   array(context && context.package_targets).forEach((target) => append({
    label: target && target.path,
    location: target && target.location,
    actionable: target && target.actionable && typeof this.options.openSourceLocation === "function",
    source_type: "package",
    open: () => this.options.openSourceLocation(target.location),
   }));
   return starts;
  }

  userComponentSourceReasonID(sourceType) {
   if (sourceType === "symbol") return "architecture.copy.source_reason_symbol";
   if (sourceType === "package") return "architecture.copy.source_reason_package";
   if (sourceType === "process_entry") return "architecture.copy.source_reason_process_entry";
   if (sourceType === "surface") return "architecture.copy.source_reason_surface";
   return "architecture.copy.source_reason_exact";
  }

  appendUserComponentSourceStart(parent, start, primary) {
   const className = "rm-arch__edge-jump rm-arch__compact-action rm-arch__source-start" +
    (primary ? " rm-arch__component-primary-source" : "");
   const node = start.actionable ? element("button", className) : element("div", className);
   if (start.actionable) node.type = "button";
   node.appendChild(element("strong", null, start.label || this.msg("architecture.action.open_code")));
   node.appendChild(element("span", null, locationLabel(start.location)));
   if (!primary) node.appendChild(element(
    "small", "rm-arch__source-reason", this.msg(start.actionable
     ? this.userComponentSourceReasonID(start.source_type)
     : "architecture.copy.source_unavailable")
   ));
   if (start.actionable) this.listen(node, "click", () => start.open());
   parent.appendChild(node);
   return node;
  }

  inspectUserComponent(component) {
   const context = this.userComponentContext(component);
   const actions = this.userComponentActions(component);
   if (!context) return;
   this.inspectorHeading(
    this.msg("architecture.label.component"),
    component.name || this.msg("architecture.fallback.repository_component"),
    component.description
   );
   const panels = this.userComponentTabs(component);
   const summary = panels.summary;
   const connections = panels.connections;
   const readCode = panels["read-code"];
   const surfaceStarts = array(context.surface_starts).filter((start) => (
    start && locationLabel(start.location)
   ));
   const sourceActions = actions.filter((action) => action.kind === "source");
   const studyActions = actions.filter((action) => action.kind === "study");
   const readStarts = this.userComponentReadStarts(context, sourceActions, surfaceStarts);
   const entryGroups = this.userComponentEntryGroups(component, surfaceStarts);
   const neighbors = this.associationEntryFor(component);
   const incoming = array(neighbors && neighbors.incoming);
   const outgoing = array(neighbors && neighbors.outgoing);
   const rawAssociationRows = array(neighbors && neighbors.associations).filter(externalStateAssociation);
   const pairedBoundaryKeys = new Set(rawAssociationRows.filter((row) => (
    row.paired && row.kind === "boundary"
   )).map((row) => String(row.owning_unit || "") + "\u0000" + String(row.imported_family || "")));
   const associationRows = rawAssociationRows.filter((row) => !(
    row.paired && row.kind === "resource" && pairedBoundaryKeys.has(
     String(row.owning_unit || "") + "\u0000" + String(row.imported_family || "")
    )
   ));

   // Summary is deliberately bounded. It is the first-contact view and must
   // not grow with repository cardinality.
   const atAGlance = this.inspectorSection(this.msg("architecture.section.at_a_glance"), summary);
   const authorityValue = context.authority === "validated"
    ? this.msg("architecture.value.authority_validated")
    : context.authority === "partial"
     ? this.msg("architecture.value.authority_partial")
     : this.msg("architecture.value.authority_local");
   const evidenceValue = context.evidence_composition === "exact"
    ? this.msg("architecture.value.evidence_exact")
    : context.evidence_composition === "mixed"
     ? this.msg("architecture.value.evidence_mixed")
     : this.msg("architecture.value.evidence_package");
   this.appendKeyValue(atAGlance, this.msg("architecture.label.grouping_authority"), authorityValue);
   this.appendKeyValue(atAGlance, this.msg("architecture.label.member_evidence"), evidenceValue);
   this.appendKeyValue(atAGlance, this.msg("architecture.label.scope"), this.msg(
    "architecture.label.member_scope", { count: Number(context.member_count) || 0 }
   ));
   const counts = element("div", "rm-arch__summary-counts");
   [
    this.msg("architecture.count.entry_groups", { count: entryGroups.length }),
    this.msg("architecture.count.interactions", { count: associationRows.length }),
    this.msg("architecture.count.source_starts", { count: readStarts.length }),
   ].forEach((label) => counts.appendChild(element("span", "rm-arch__summary-count", label)));
   atAGlance.appendChild(counts);

   const summaryEntries = this.inspectorSection(this.msg("architecture.section.entry_groups"), summary);
   summaryEntries.classList.add("rm-arch__summary-grid");
   if (entryGroups.length === 0) {
    summaryEntries.appendChild(element(
     "p", "rm-arch__copy rm-arch__inspector-empty", this.msg("architecture.copy.no_observed_entry")
    ));
   } else {
    entryGroups.slice(0, 3).forEach((group) => {
     const row = element("div", "rm-arch__compact-reference rm-arch__entry-group-summary");
     row.appendChild(element("strong", null, this.userComponentEntryKindLabel(group.kind)));
     row.appendChild(element(
      "span", "rm-arch__summary-item-count",
      this.msg("architecture.count.entries", { count: group.entries.length })
     ));
     summaryEntries.appendChild(row);
    });
   }

   const interactionSummaries = associationRows.map((row) => ({
    title: text(row.imported_family) || this.msg("architecture.value.interaction_boundary_resource"),
    detail: text(row.owning_unit),
   }));
   array(context.structural_relations).forEach((relation) => interactionSummaries.push({
    title: (text(relation && relation.from_label) || memberLabel(relation && relation.from, this.message)) +
     " → " + (text(relation && relation.to_label) || memberLabel(relation && relation.to, this.message)),
    detail: locationLabel(relation && relation.location),
   }));
   const summaryInteractions = this.inspectorSection(this.msg("architecture.section.key_interactions"), summary);
   summaryInteractions.classList.add("rm-arch__summary-grid");
   if (interactionSummaries.length === 0) {
    summaryInteractions.appendChild(element(
     "p", "rm-arch__copy rm-arch__inspector-empty", this.msg("architecture.copy.no_observed_callsites")
    ));
   } else {
    interactionSummaries.slice(0, 3).forEach((interaction) => {
     const row = element("div", "rm-arch__compact-reference rm-arch__interaction-summary");
     row.appendChild(element("strong", null, interaction.title));
     summaryInteractions.appendChild(row);
    });
   }

   const primaryStudy = studyActions[0];
   const studySummary = this.inspectorSection(this.msg("architecture.section.why_it_matters"), summary);
   if (primaryStudy) {
    const study = primaryStudy.value;
    const button = element("button", "rm-arch__edge-jump rm-arch__compact-action rm-arch__primary-study");
    button.type = "button";
    button.appendChild(element("strong", null, study.question));
    button.appendChild(element("span", null, this.msg("architecture.action.study_this_area")));
    this.listen(button, "click", () => this.options.openStudyDirection(study.id));
    studySummary.appendChild(button);
   } else {
    studySummary.appendChild(element(
     "p", "rm-arch__copy rm-arch__inspector-empty", this.msg("architecture.copy.no_observed_studies")
    ));
   }

   const unknownMessages = [component.hypothesis
    ? this.msg("architecture.value.unknown_hypothesis")
    : this.msg("architecture.value.unknown_not_covered")];
   if (entryGroups.length === 0) unknownMessages.push(this.msg("architecture.copy.no_observed_entry"));
   if (associationRows.length === 0) unknownMessages.push(this.msg("architecture.copy.no_observed_callsites"));
   if (readStarts.length === 0) unknownMessages.push(this.msg("architecture.copy.no_observed_sources"));
   const unknown = this.inspectorSection(this.msg("architecture.section.what_remains_unknown"), summary);
   const unknownList = element("ul", "rm-arch__summary-unknowns");
   unknownMessages.slice(0, 3).forEach((message) => unknownList.appendChild(element("li", null, message)));
   unknown.appendChild(unknownList);

   // One exact source remains on the default panel, preserving the cube →
   // exact source journey in two actions.
   const primarySource = this.inspectorSection(this.msg("architecture.section.start_in_code"), summary);
   if (readStarts.length > 0) this.appendUserComponentSourceStart(primarySource, readStarts[0], true);
   else primarySource.appendChild(element(
    "p", "rm-arch__copy rm-arch__inspector-empty", this.msg("architecture.copy.no_observed_sources")
   ));

   // Connections owns the complete relationship and entry context.
   const usedBySection = this.inspectorSection(this.msg("architecture.section.used_by"), connections);
   if (incoming.length > 0) {
    incoming.forEach((neighbor) => {
     const reference = element("div", "rm-arch__compact-reference");
     reference.appendChild(element(
      "strong", null, neighbor.name || this.msg("architecture.fallback.repository_component")
     ));
     if (this.options.openComponent) {
      const button = element("button", "rm-arch__edge-jump rm-arch__compact-action");
      button.type = "button";
      button.appendChild(element(
       "span", null, this.msg("architecture.action.open_related_component")
      ));
      this.listen(button, "click", () => this.options.openComponent(neighbor.component_id));
      reference.appendChild(button);
     }
     usedBySection.appendChild(reference);
    });
   } else {
    usedBySection.appendChild(element(
     "p", "rm-arch__copy rm-arch__inspector-empty", this.msg("architecture.copy.no_observed_used_by")
    ));
   }
   const usesSection = this.inspectorSection(this.msg("architecture.section.uses"), connections);
   if (outgoing.length > 0) {
    outgoing.forEach((neighbor) => {
     const reference = element("div", "rm-arch__compact-reference");
     reference.appendChild(element(
      "strong", null, neighbor.name || this.msg("architecture.fallback.repository_component")
     ));
     if (this.options.openComponent) {
      const button = element("button", "rm-arch__edge-jump rm-arch__compact-action");
      button.type = "button";
      button.appendChild(element(
       "span", null, this.msg("architecture.action.open_related_component")
      ));
      this.listen(button, "click", () => this.options.openComponent(neighbor.component_id));
      reference.appendChild(button);
     }
     usesSection.appendChild(reference);
    });
   } else {
    usesSection.appendChild(element(
     "p", "rm-arch__copy rm-arch__inspector-empty", this.msg("architecture.copy.no_observed_uses")
    ));
   }

   const entersSection = this.inspectorSection(this.msg("architecture.section.how_work_enters"), connections);
   if (entryGroups.length > 0) {
    entryGroups.forEach((group) => {
     const groupCard = element("div", "rm-arch__entry-group");
     groupCard.appendChild(element("strong", "rm-arch__entry-group-title", this.userComponentEntryKindLabel(group.kind)));
     group.entries.forEach((entry) => {
      const start = entry.start;
      if (start && locationLabel(start.location)) {
       this.appendUserComponentSourceStart(groupCard, {
        label: start.label || entry.label,
        location: start.location,
        actionable: start.actionable && typeof this.options.openSourceLocation === "function",
        source_type: group.kind === "process_entry" ? "process_entry" : "surface",
        open: () => this.options.openSourceLocation(start.location),
       }, false);
      } else {
       groupCard.appendChild(element("span", "rm-arch__entry-group-item", entry.label || entry.id));
      }
     });
     entersSection.appendChild(groupCard);
    });
   } else {
    entersSection.appendChild(element(
     "p", "rm-arch__copy rm-arch__inspector-empty", this.msg("architecture.copy.no_observed_entry")
    ));
   }

   if (array(component.shared_unit_refs).length > 0 || array(component.shared_members).length > 0) {
    const shared = this.inspectorSection(this.msg("architecture.section.shared_scope"), connections);
    shared.appendChild(element(
     "p", "rm-arch__copy", this.msg("architecture.copy.shared_scope_copy", {
      count: array(component.shared_unit_refs).length,
     })
    ));
    array(component.shared_members).forEach((member) => {
     const row = element("div", "rm-arch__compact-reference");
     row.appendChild(element("strong", null, memberLabel(member && member.id, this.message) || text(member && member.name)));
     shared.appendChild(row);
    });
   }

   // Operations: exact code relations observed for this component.
   if (array(context.structural_relations).length > 0) {
    const relations = this.inspectorSection(this.msg("architecture.section.operations"), connections);
    array(context.structural_relations).forEach((relation) => {
     const from = text(relation && relation.from_label) || memberLabel(relation && relation.from, this.message);
     const to = text(relation && relation.to_label) || memberLabel(relation && relation.to, this.message);
     const location = relation && relation.location;
     if (location && locationLabel(location) && typeof this.options.openLocation === "function") {
      const action = element("button", "rm-arch__edge-jump rm-arch__compact-action");
      action.type = "button";
      action.appendChild(element("strong", null, from + " → " + to));
      action.appendChild(element("span", null, locationLabel(location)));
      this.listen(action, "click", () => this.options.openLocation(
       location.path, location.line || 0, location.column || 0
      ));
      relations.appendChild(action);
      return;
     }
     const reference = element("div", "rm-arch__compact-reference");
     reference.appendChild(element("strong", null, from + " → " + to));
     relations.appendChild(reference);
    });
   }

   // Decision 225: observed external/state callsites in exact member scope.
   // Connection rows — not edges — are the primary controls: click a row to
   // highlight the node and expand exact witnesses in place; exact source in
   // at most two actions. Only "an observed boundary/resource callsite occurs
   // in an exact member scope" is stated — never runtime dependency,
   // ownership, reachability, read/write/order or target identity.
   const associationEvidence = element("details", "rm-arch__evidence-limitations");
   associationEvidence.appendChild(element(
    "summary", "rm-arch__evidence-limitations-summary", this.msg("architecture.section.evidence_limitations")
   ));
   connections.appendChild(associationEvidence);
   const associationSection = this.inspectorSection(
    this.msg("architecture.section.observed_callsites"), associationEvidence
   );
   associationSection.appendChild(element(
    "p", "rm-arch__copy", this.msg("architecture.copy.observed_callsites_limit")
   ));
   if (associationRows.length > 0) {
    // Decision 229 D2: Boundary and Resource rows sharing the same exact
    // witness coalesce into ONE visible "Observed external/state
    // interaction" row (canonical records stay distinct). Paired rows
    // render once with every witness retained.
    const rows = associationRows;
    const pairedKeys = new Set();
    rows.forEach((row) => {
     if (!row.paired || row.kind !== "boundary") return;
     const key = String(row.owning_unit || "") + "\u0000" + String(row.imported_family || "");
     if (!pairedKeys.has(key)) pairedKeys.add(key);
    });
    const merged = [];
    const mergedBoundary = new Set();
    rows.forEach((row) => {
     const key = String(row.owning_unit || "") + "\u0000" + String(row.imported_family || "");
     if (row.paired && row.kind === "resource" && mergedBoundary.has(key)) return;
     if (row.paired && row.kind === "boundary") mergedBoundary.add(key);
     merged.push(row);
    });
    // Decision 229 D2 precision tiers: exact symbol/file/narrow-package
    // rows show as normal rows; broad-package observations collapse under
    // "Additional observations from broad package membership · N".
    const broadRows = [];
    const principalRows = [];
    merged.forEach((row) => {
     if (this.associationPrecision(row) === "broad_package_scope") broadRows.push(row);
     else principalRows.push(row);
    });
    principalRows.forEach((row) => {
     // Decision 230 D2: valid disclosure structure — a noninteractive
     // container holding a toggle button (head + meta + chevron) and a
     // sibling witness list. Witness source actions are NEVER nested
     // inside the toggle; a witness click opens the source without
     // toggling the disclosure.
     const rowEl = element("div", "rm-arch__association-row");
     const toggle = element("button", "rm-arch__association-row__toggle");
     toggle.type = "button";
     const head = element("div", "rm-arch__association-row__head");
     const paired = row.paired && row.kind === "boundary";
     head.appendChild(element(
      "strong", null, paired
       ? this.msg("architecture.value.interaction_boundary_resource")
       : this.msg(row.kind === "boundary"
        ? "architecture.value.category_boundary" : "architecture.value.category_resource")
     ));
     if (row.imported_family) head.appendChild(element("span", "rm-arch__association-family", row.imported_family));
     head.appendChild(element("span", "rm-arch__association-unit", row.owning_unit));
     toggle.appendChild(head);
     const meta = element("div", "rm-arch__association-row__meta");
     meta.appendChild(element(
      "span", "rm-arch__association-count",
      this.msg("architecture.label.observation_count", { count: Number(row.observation_count) || 0 })
     ));
     // Decision 229 D2: the visible row states its exact precision tier —
     // never a stronger claim than the evidence supports.
     meta.appendChild(element(
      "span", "rm-arch__association-precision rm-arch__association-precision--" + this.associationPrecision(row),
      this.msg(this.associationPrecisionMessageID(this.associationPrecision(row)))
     ));
     // Goal 05 source roles: closed production/test/tooling split,
     // reconciled to the observation count, always visible.
     const roles = row.source_roles || {};
     const roleParts = [];
     if (Number(roles.production) > 0) roleParts.push(this.msg("architecture.role.production") + " " + roles.production);
     if (Number(roles.test) > 0) roleParts.push(this.msg("architecture.role.test") + " " + roles.test);
     if (Number(roles.tooling) > 0) roleParts.push(this.msg("architecture.role.tooling") + " " + roles.tooling);
     if (roleParts.length) meta.appendChild(element("span", "rm-arch__association-roles", roleParts.join(" · ")));
     if (row.paired) meta.appendChild(element("span", "rm-arch__association-paired", this.msg("architecture.value.paired_boundary_resource")));
     toggle.appendChild(meta);
     const chevron = element("span", "rm-arch__association-chevron");
     chevron.setAttribute("aria-hidden", "true");
     chevron.textContent = "▸";
     toggle.appendChild(chevron);
     rowEl.appendChild(toggle);
     // Exact witnesses expand in place (toggle click). Sibling list —
     // source buttons are not descendants of the toggle button.
     const witnesses = element("div", "rm-arch__association-witnesses");
     witnesses.hidden = true;
     array(row.witnesses).forEach((witness) => {
      const label = [witness.symbol, witness.path + ":" + witness.line].filter(Boolean).join(" · ");
      // Decision 230 (fresh review task-4 minor): in static reports the
      // witness jump is a real link (pinned revision, target=_blank,
      // rel=noopener noreferrer); served mode keeps the exact button.
      const staticURL = typeof this.options.staticSourceURL === "function" && witness.path
       ? this.options.staticSourceURL(witness.path, witness.line || 0)
       : "";
      const jump = staticURL
       ? element("a", "rm-arch__edge-jump rm-arch__compact-action rm-arch__edge-jump--link")
       : element("button", "rm-arch__edge-jump rm-arch__compact-action");
      if (staticURL) {
       jump.href = staticURL;
       jump.target = "_blank";
       jump.rel = "noopener noreferrer";
      } else {
       jump.type = "button";
      }
      jump.appendChild(element("strong", null, witness.symbol || witness.path));
      jump.appendChild(element("span", null, witness.path + (witness.line ? ":" + witness.line : "")));
      if (witness.role === "test") jump.appendChild(element("span", "rm-arch__association-witness-role", this.msg("architecture.role.test")));
      else if (witness.role === "tooling") jump.appendChild(element("span", "rm-arch__association-witness-role", this.msg("architecture.role.tooling")));
      else if (witness.role) jump.appendChild(element("span", "rm-arch__association-witness-role", this.msg("architecture.role.production")));
      if (!staticURL && typeof this.options.openLocation === "function" && witness.path) {
       this.listen(jump, "click", () => this.options.openLocation(witness.path, witness.line || 0, 0));
      }
      witnesses.appendChild(jump);
     });
     rowEl.appendChild(witnesses);
     const witnessCount = array(row.witnesses).length;
     if (witnessCount > 0) {
      // Decision 230 (fresh review task-4 minor): aria-controls names the
      // exact sibling witness list, never an empty value.
      const witnessListID = "rm-arch-assoc-" + text(row.owning_unit) + "-" + text(row.imported_family) + "-" + witnessCount;
      witnesses.setAttribute("id", witnessListID);
      toggle.setAttribute("aria-expanded", "false");
      toggle.setAttribute("aria-controls", witnessListID);
      toggle.setAttribute("title", this.msg("architecture.label.expand_witnesses", { count: witnessCount }));
      this.listen(toggle, "click", () => {
       const willOpen = witnesses.hidden;
       witnesses.hidden = !willOpen;
       toggle.setAttribute("aria-expanded", String(willOpen));
       chevron.textContent = willOpen ? "▾" : "▸";
      });
     } else {
      toggle.setAttribute("aria-disabled", "true");
     }
     associationSection.appendChild(rowEl);
    });
    // Broad-package observations collapse under a bounded disclosure.
    if (broadRows.length) {
     const broad = element("details", "rm-arch__association-broad");
     const summary = element("summary", "rm-arch__association-broad__summary");
     summary.appendChild(element(
      "span", "rm-arch__association-broad__label",
      this.msg("architecture.label.broad_package_observations", { count: broadRows.length })
     ));
     broad.appendChild(summary);
     broadRows.forEach((row) => {
      // Decision 230 D2: same valid disclosure structure as principal rows.
      const rowEl = element("div", "rm-arch__association-row rm-arch__association-row--broad");
      const toggle = element("button", "rm-arch__association-row__toggle");
      toggle.type = "button";
      const head = element("div", "rm-arch__association-row__head");
      head.appendChild(element(
       "strong", null, this.msg(row.kind === "boundary"
        ? "architecture.value.category_boundary" : "architecture.value.category_resource")
      ));
      if (row.imported_family) head.appendChild(element("span", "rm-arch__association-family", row.imported_family));
      head.appendChild(element("span", "rm-arch__association-unit", row.owning_unit));
      toggle.appendChild(head);
      const meta = element("div", "rm-arch__association-row__meta");
      meta.appendChild(element(
       "span", "rm-arch__association-count",
       this.msg("architecture.label.observation_count", { count: Number(row.observation_count) || 0 })
      ));
      const roles = row.source_roles || {};
      const roleParts = [];
      if (Number(roles.production) > 0) roleParts.push(this.msg("architecture.role.production") + " " + roles.production);
      if (Number(roles.test) > 0) roleParts.push(this.msg("architecture.role.test") + " " + roles.test);
      if (Number(roles.tooling) > 0) roleParts.push(this.msg("architecture.role.tooling") + " " + roles.tooling);
      if (roleParts.length) meta.appendChild(element("span", "rm-arch__association-roles", roleParts.join(" · ")));
      toggle.appendChild(meta);
      const chevron = element("span", "rm-arch__association-chevron");
      chevron.setAttribute("aria-hidden", "true");
      chevron.textContent = "▸";
      toggle.appendChild(chevron);
      rowEl.appendChild(toggle);
      const witnesses = element("div", "rm-arch__association-witnesses");
      witnesses.hidden = true;
      array(row.witnesses).forEach((witness) => {
       // Decision 230 (fresh review task-4 minor): static witnesses are
       // real links; served mode keeps exact buttons.
       const staticURL = typeof this.options.staticSourceURL === "function" && witness.path
        ? this.options.staticSourceURL(witness.path, witness.line || 0)
        : "";
       const jump = staticURL
        ? element("a", "rm-arch__edge-jump rm-arch__compact-action rm-arch__edge-jump--link")
        : element("button", "rm-arch__edge-jump rm-arch__compact-action");
       if (staticURL) {
        jump.href = staticURL;
        jump.target = "_blank";
        jump.rel = "noopener noreferrer";
       } else {
        jump.type = "button";
       }
       jump.appendChild(element("strong", null, witness.symbol || witness.path));
       jump.appendChild(element("span", null, witness.path + (witness.line ? ":" + witness.line : "")));
       if (!staticURL && typeof this.options.openLocation === "function" && witness.path) {
        this.listen(jump, "click", () => this.options.openLocation(witness.path, witness.line || 0, 0));
       }
       witnesses.appendChild(jump);
      });
      rowEl.appendChild(witnesses);
      const witnessCount = array(row.witnesses).length;
      if (witnessCount > 0) {
       const witnessListID = "rm-arch-assoc-broad-" + text(row.owning_unit) + "-" + text(row.imported_family) + "-" + witnessCount;
       witnesses.setAttribute("id", witnessListID);
       toggle.setAttribute("aria-expanded", "false");
       toggle.setAttribute("aria-controls", witnessListID);
       toggle.setAttribute("title", this.msg("architecture.label.expand_witnesses", { count: witnessCount }));
       this.listen(toggle, "click", () => {
        const willOpen = witnesses.hidden;
        witnesses.hidden = !willOpen;
        toggle.setAttribute("aria-expanded", String(willOpen));
        chevron.textContent = willOpen ? "▾" : "▸";
       });
       } else {
       toggle.setAttribute("aria-disabled", "true");
       }
       broad.appendChild(rowEl);
     });
     associationSection.appendChild(broad);
     }
    associationSection.appendChild(element(
     "p", "rm-arch__limitation", this.msg("architecture.copy.association_limitations")
    ));
   } else {
    associationSection.appendChild(element(
     "p", "rm-arch__copy rm-arch__inspector-empty", this.msg("architecture.copy.no_observed_callsites")
    ));
   }

   // Read code is exact and deterministically ordered by the backend-owned
   // context: exact symbol starts, entry starts, then package fallbacks.
   const exactStarts = this.inspectorSection(this.msg("architecture.section.exact_starts"), readCode);
   if (readStarts.length === 0) {
    exactStarts.appendChild(element(
     "p", "rm-arch__copy rm-arch__inspector-empty", this.msg("architecture.copy.no_observed_sources")
    ));
   } else {
    readStarts.slice(0, 5).forEach((start) => this.appendUserComponentSourceStart(exactStarts, start, false));
    if (readStarts.length > 5) {
     const allSources = element("details", "rm-arch__source-starts-all");
     allSources.appendChild(element(
      "summary", "rm-arch__source-starts-all-summary",
      this.msg("architecture.action.show_all_sources", { count: readStarts.length })
     ));
     readStarts.slice(5).forEach((start) => this.appendUserComponentSourceStart(allSources, start, false));
     exactStarts.appendChild(allSources);
    }
   }

   const readStudy = this.inspectorSection(this.msg("architecture.section.why_it_matters"), readCode);
   if (primaryStudy) {
    const study = primaryStudy.value;
    const button = element("button", "rm-arch__edge-jump rm-arch__compact-action rm-arch__study-this-area");
    button.type = "button";
    button.appendChild(element("strong", null, this.msg("architecture.action.study_this_area")));
    button.appendChild(element("span", null, study.question));
    this.listen(button, "click", () => this.options.openStudyDirection(study.id));
    readStudy.appendChild(button);
   } else {
    readStudy.appendChild(element(
     "p", "rm-arch__copy rm-arch__inspector-empty", this.msg("architecture.copy.no_observed_studies")
    ));
   }

   const provenance = element("details", "rm-arch__evidence-limitations");
   provenance.appendChild(element(
    "summary", "rm-arch__evidence-limitations-summary", this.msg("architecture.section.evidence_limitations")
   ));
   readCode.appendChild(provenance);
   const truth = this.inspectorSection(this.msg("architecture.section.evidence"), provenance);
   this.appendKeyValue(truth, this.msg("architecture.label.grouping_authority"), authorityValue);
   this.appendKeyValue(truth, this.msg("architecture.label.member_evidence"), evidenceValue);
   array(context.package_paths).forEach((path) => truth.appendChild(element(
    "code", "rm-arch__compact-package", path
   )));
   const alternates = array(component.alternate_names);
   const alternateDescriptions = array(component.alternate_descriptions);
   if (alternates.length > 0 || alternateDescriptions.length > 0) {
    const alternatesSection = this.inspectorSection(
     this.msg("architecture.section.equivalent_components"), provenance
    );
    alternates.forEach((name, index) => {
     const row = element("div", "rm-arch__alternate");
     row.appendChild(element("strong", "rm-arch__alternate-name", name));
     if (alternateDescriptions[index]) row.appendChild(element(
      "span", "rm-arch__alternate-description", alternateDescriptions[index]
     ));
     alternatesSection.appendChild(row);
    });
    alternateDescriptions.slice(alternates.length).forEach((description) => {
     alternatesSection.appendChild(element("p", "rm-arch__alternate-description", description));
    });
   }
  }

  // associationPrecision derives the closed Decision 229 D2 presentation
  // precision tier deterministically from the row's exact evidence — never
  // inferred from prose:
  //   exact_symbol_scope    — every witness has an exact symbol+path+line;
  //   exact_file_scope      — every witness has an exact path+line;
  //   narrow_package_scope  — witnesses share a narrow package scope;
  //   broad_package_scope   — broad imported-family scope (aggregate);
  //   diagnostic_remainder  — unclassified exact scope.
  associationPrecision(row) {
   if (!row) return "diagnostic_remainder";
   const witnesses = array(row.witnesses);
   if (!witnesses.length) return "broad_package_scope";
   let hasSymbol = true;
   let hasLine = true;
   witnesses.forEach((witness) => {
    if (!witness.symbol) hasSymbol = false;
    if (!witness.path || !witness.line) hasLine = false;
   });
   if (hasSymbol) return "exact_symbol_scope";
   if (hasLine) return "exact_file_scope";
   if (row.imported_family) return "broad_package_scope";
   return "narrow_package_scope";
  }

  associationPrecisionMessageID(precision) {
   const ids = {
    exact_symbol_scope: "architecture.precision.exact_symbol_scope",
    exact_file_scope: "architecture.precision.exact_file_scope",
    narrow_package_scope: "architecture.precision.narrow_package_scope",
    broad_package_scope: "architecture.precision.broad_package_scope",
    diagnostic_remainder: "architecture.precision.diagnostic_remainder",
   };
   return ids[precision] || "architecture.precision.narrow_package_scope";
  }

  associationEntryFor(component) {
   const associations = this.options.associations || null;
   if (!associations || !component) return null;
   const componentID = text(component.id);
   return array(associations.components).find((entry) => text(entry.component_id) === componentID) || null;
  }

  appendParticipantComponentLinks(record, excludedComponentID) {
   const componentIDs = participatingComponentIDs(record, this.componentByID)
    .filter((componentID) => componentID !== text(excludedComponentID));
   if (componentIDs.length === 0) return;
   const section = this.inspectorSection(this.msg("architecture.label.participating_components"));
   componentIDs.forEach((componentID) => {
    const component = this.componentByID.get(componentID);
    const button = element("button", "rm-arch__edge-jump", component.name || component.id);
    button.type = "button";
    this.listen(button, "click", () => this.openComponent(componentID));
    section.appendChild(button);
   });
  }

  inspectUserSurface(surface) {
   this.inspectorHeading(
    this.msg("architecture.fallback.runtime_surface"),
    surface.name || surface.kind || this.msg("architecture.fallback.runtime_surface"),
    this.msg("architecture.copy.runtime_surface_entrypoint")
   );
   const owner = this.componentByID.get(text(surface.owning_component_id));
   if (owner && owner.name) {
    const section = this.inspectorSection(this.msg("architecture.label.code_area"));
    const button = element("button", "rm-arch__edge-jump", owner.name);
    button.type = "button";
    this.listen(button, "click", () => this.openComponent(owner.id));
    section.appendChild(button);
   }
   this.appendParticipantComponentLinks(surface, owner && owner.id);
   const locations = array(surface.evidence).filter((location) => locationLabel(location));
   if (locations.length > 0 || surface.owning_executable) {
    const section = this.inspectorSection(this.msg("architecture.section.code"));
    this.appendKeyValue(section, this.msg("architecture.label.executable"), surface.owning_executable);
    locations.forEach((location) => {
     this.appendLocation(section, location, this.msg("architecture.action.open_code"));
    });
   }
   const flow = this.flowByID.get(text(surface.related_saved_trace_id));
   if (flow) {
    const section = this.inspectorSection(this.msg("architecture.fallback.code_path"));
    const button = element(
     "button",
     "rm-arch__edge-jump",
     flow.name || this.msg("architecture.fallback.source_backed_code_path")
    );
    button.type = "button";
    this.listen(button, "click", () => this.openTrace(flow.id));
    section.appendChild(button);
   }
  }

  appendUserFlowEvidence(parent, flow) {
   this.appendKeyValue(parent, this.msg("architecture.label.starts_when"), flow.trigger);
   this.appendKeyValue(parent, this.msg("architecture.label.command"), flow.command);
   this.appendKeyValue(parent, this.msg("architecture.label.scope"), flow.scope);
   const steps = array(flow.steps);
   if (steps.length === 0) return;
   const section = element("section", "rm-arch__inspector-section");
   section.appendChild(element(
    "h4",
    "rm-arch__inspector-section-title",
    this.msg("architecture.section.implementation_steps")
   ));
   steps.forEach((step) => {
    const button = element("button", "rm-arch__edge-jump");
    button.type = "button";
    button.appendChild(element(
     "strong",
     null,
     step.label || step.qualified_name || this.msg("architecture.fallback.code_step")
    ));
    const detail = [step.qualified_name, locationLabel(step.location)].filter(Boolean).join(" · ");
    if (detail) button.appendChild(element("span", null, detail));
    this.listen(button, "click", () => this.setSelection({
     flow: text(flow.id),
     component: this.flowStepSelectionComponent(flow.id, step.id),
     step: text(step.id),
     edge: "",
    }, true));
    section.appendChild(button);
   });
   parent.appendChild(section);
  }

  inspectUserFlow(flow) {
   this.inspectorHeading(
    this.msg("architecture.fallback.code_path"),
    flow.name || this.msg("architecture.fallback.source_backed_code_path"),
    flow.mental_model || flow.goal || flow.why_inspect
   );
   this.appendUserFlowEvidence(this.inspector, flow);
  }

  inspectUserStep(flow, step) {
   const title = step.label || step.qualified_name || this.msg("architecture.fallback.code_step");
   const subtitle = step.qualified_name && step.qualified_name !== title ? step.qualified_name : "";
   this.inspectorHeading(this.msg("architecture.fallback.implementation_step"), title, subtitle);
   const component = this.componentByID.get(text(step.component_id));
   if (component && component.name) {
    const section = this.inspectorSection(this.msg("architecture.label.code_area"));
    const button = element("button", "rm-arch__edge-jump", component.name);
    button.type = "button";
    this.listen(button, "click", () => this.openComponent(component.id));
    section.appendChild(button);
   }
   this.appendParticipantComponentLinks(step, component && component.id);
   const locations = [step.location, step.binding && step.binding.location].filter((location) => locationLabel(location));
   if (locations.length > 0) {
    const source = this.inspectorSection(this.msg("architecture.section.source"));
    this.appendLocation(source, step.location, this.msg("architecture.action.open_code"));
    if (step.binding) {
     this.appendLocation(source, step.binding.location, this.msg("architecture.label.binding_code"));
    }
   }
  }

  inspectUserFlowEdge(edge) {
   const source = this.flowStepsByKey.get(flowStepKey(edge.flow_id, edge.from));
   const target = this.flowStepsByKey.get(flowStepKey(edge.flow_id, edge.to));
   const sourceName = source && (source.label || source.qualified_name);
   const targetName = target && (target.label || target.qualified_name);
   this.inspectorHeading(
    this.msg("architecture.label.code_transition"),
    relationLabel(edge.relation, this.message),
    sourceName && targetName ? sourceName + " → " + targetName : ""
   );
   const locations = [edge.evidence, source && source.location, target && target.location]
    .filter((location) => locationLabel(location));
   if (locations.length > 0) {
    const code = this.inspectorSection(this.msg("architecture.section.source"));
    this.appendLocation(code, edge.evidence, this.msg("architecture.label.call_site"));
    if (source) this.appendLocation(code, source.location, this.msg("architecture.label.from"));
    if (target) this.appendLocation(code, target.location, this.msg("architecture.label.to"));
   }
   if (edge.condition && (edge.condition.expression || locationLabel(edge.condition.location))) {
    const condition = this.inspectorSection(this.msg("architecture.section.condition"));
    if (edge.condition.expression) {
     condition.appendChild(element("code", "rm-arch__condition", edge.condition.expression));
    }
    this.appendLocation(condition, edge.condition.location, this.msg("architecture.action.open_condition"));
   }
  }

  inspectUserStructuralEdge(edge) {
   const from = this.componentByID.get(text(edge.from_component_id));
   const to = this.componentByID.get(text(edge.to_component_id));
   const witness = edge.witness || {};
   const fromName = from && from.name;
   const toName = to && to.name;
	   this.inspectorHeading(
	    this.msg("architecture.label.code_relation"),
	    witness.kind
	     ? relationLabel(witness.kind, this.message)
	     : this.msg("architecture.fallback.source_backed_relation"),
	    fromName && toName ? fromName + " → " + toName : ""
	   );
   if (locationLabel(witness.location)) {
    const source = this.inspectorSection(this.msg("architecture.section.source"));
    this.appendLocation(source, witness.location, this.msg("architecture.action.open_code"));
   }
  }

  inspectLandscape() {
   const grounding = architectureGroundingWording(
    this.data.architecture_source,
    this.data.grounding_mode,
    this.message
   );
   if (this.userMode) {
    this.inspectorHeading(
     this.msg("architecture.nav.architecture"),
     grounding.title,
     grounding.subtitle
    );
    return;
   }
   const hasFlows = this.flows.length > 0;
   this.inspectorHeading(
    this.msg("architecture.nav.architecture"),
    grounding.title,
    grounding.subtitle
   );
    const note = this.inspectorSection(this.msg("architecture.section.evidence_semantics"));
    note.appendChild(element(
     "p",
     "rm-arch__copy",
     this.msg("architecture.copy.evidence_semantics")
    ));
    this.appendKeyValue(
     note,
     this.msg("architecture.label.architecture_source"),
     architectureSourceLabel(this.data.architecture_source, this.message)
    );
    this.appendKeyValue(
     note,
     this.msg("architecture.label.grounding_mode"),
     text(this.data.grounding_mode)
    );
    this.appendKeyValue(
     note,
     this.msg("architecture.label.architecture_anchors"),
     String(array(this.data.behavior_anchors).length)
    );
   if (!hasFlows) {
     const flowState = this.inspectorSection(this.msg("architecture.nav.saved_traces"));
    flowState.appendChild(element(
     "p",
     "rm-arch__notice is-warning",
     this.msg("architecture.copy.no_compatible_flowproof")
    ));
   }
   this.appendDiagnostics(this.inspectorSection(this.msg("architecture.section.diagnostics")), "");
  }

  inspectComponent(component) {
   if (this.userMode) return this.inspectUserComponent(component);
   const subsystem = this.subsystemByID.get(text(component.subsystem_id));
   this.inspectorHeading(this.msg("architecture.label.component"), component.name || component.id, component.description);
   const purpose = this.inspectorSection(this.msg("architecture.section.purpose_grounding"));
   if (component.description) purpose.appendChild(element("p", "rm-arch__copy", component.description));
   if (subsystem) {
    this.appendKeyValue(
     purpose,
     this.msg("architecture.label.subsystem"),
     subsystem.name || subsystem.id
    );
   }
   this.appendKeyValue(
    purpose,
    this.msg("architecture.label.grounding"),
    component.hypothesis
     ? this.msg("architecture.value.conceptual_package_derived")
     : this.msg("architecture.value.exact_local_membership")
   );

   const surfaceIDs = Array.from(new Set(array(component.owned_surface_ids).concat(array(component.participating_surface_ids))));
   const surfaces = this.inspectorSection(this.msg("architecture.section.component_surfaces"));
   surfaces.appendChild(element(
    "p",
    "rm-arch__copy",
    this.msg("architecture.copy.component_surfaces_limit")
   ));
   if (surfaceIDs.length === 0) {
     surfaces.appendChild(element(
      "p",
      "rm-arch__empty",
      this.msg("architecture.copy.no_supported_surface")
     ));
   }
   surfaceIDs.forEach((surfaceID) => {
    const surface = this.surfaceByID.get(text(surfaceID));
    if (!surface) return;
    const button = element("button", "rm-arch__edge-jump");
    button.type = "button";
    button.appendChild(element("strong", null, surface.name || surface.id));
    button.appendChild(element("span", null, [surface.kind, text(surface.category), surface.status].filter(Boolean).join(" · ")));
    this.listen(button, "click", () => this.setSelection({ component: "", surface: text(surface.id), step: "", edge: "" }, true));
    surfaces.appendChild(button);
   });

   const traces = this.inspectorSection(this.msg("architecture.nav.saved_traces"));
   const relatedFlowIDs = array(component.participating_flow_ids).filter((flowID) => this.flowByID.has(text(flowID)));
   if (relatedFlowIDs.length === 0) {
    traces.appendChild(element(
     "p",
     "rm-arch__empty",
     this.msg("architecture.copy.no_trace_crosses_component")
    ));
   }
   relatedFlowIDs.forEach((flowID) => {
    const flow = this.flowByID.get(text(flowID));
    const button = element("button", "rm-arch__edge-jump");
    button.type = "button";
    button.appendChild(element("strong", null, flow.name || flow.id));
    button.appendChild(element("span", null, [
     flow.status,
     this.msg("architecture.count.grounded_areas", {
      grounded: flow.grounded_areas,
      total: flow.total_areas,
     }),
    ].filter(Boolean).join(" · ")));
    this.listen(button, "click", () => this.openTrace(flow.id));
    traces.appendChild(button);
   });

   const suggestions = this.inspectorSection(this.msg("architecture.section.suggested_investigations"));
   const suggestionIDs = array(component.suggested_investigation_ids);
   if (suggestionIDs.length === 0) {
    suggestions.appendChild(element(
     "p",
     "rm-arch__empty",
     this.msg("architecture.copy.no_untraced_suggestion")
    ));
   }
    suggestionIDs.forEach((directionID) => {
     const suggestion = this.suggestionByID.get(text(directionID));
     const direction = this.directionByID.get(text(directionID));
     if (!suggestion && !direction) return;
     const card = element("article", "rm-arch__evidence-card");
     card.appendChild(element("strong", "rm-arch__evidence-title", (suggestion && suggestion.title) || direction.name || direction.id));
     const reason = suggestion && suggestion.reason || direction.why_interesting;
     if (reason) card.appendChild(element("p", "rm-arch__copy", reason));
     this.appendKeyValue(
      card,
      this.msg("architecture.label.status"),
      this.msg("architecture.value.suggested_not_trace")
     );
     if (suggestion) {
      this.appendKeyValue(
       card,
       this.msg("architecture.label.grounding"),
       text(suggestion.current_grounding)
      );
      this.appendKeyValue(
       card,
       this.msg("architecture.label.trace_start"),
       suggestion.can_start_trace
        ? this.msg("architecture.value.available")
        : this.msg("architecture.value.unavailable_local_evidence")
      );
      this.appendKeyValue(
       card,
       this.msg("architecture.label.source_investigation"),
       suggestion.investigation_available
        ? this.msg("architecture.value.exact_source_available")
        : this.msg("architecture.value.unavailable")
      );
      if (!suggestion.can_start_trace) {
       card.appendChild(element(
        "p",
        "rm-arch__notice is-warning",
        this.msg("architecture.label.trace_unavailable_reason", {
         reason: suggestion.trace_unavailable_reason ||
          this.msg("architecture.copy.no_supported_trace_seed"),
        })
       ));
      }
      if (suggestion.investigation_available && suggestion.start_location && typeof this.options.openLocation === "function") {
       const openSource = element(
        "button",
        "rm-arch__edge-jump",
        this.msg("architecture.action.open_starting_source")
       );
       openSource.type = "button";
       this.listen(openSource, "click", () => {
        const location = suggestion.start_location;
        this.options.openLocation(location.path, location.line || 0, location.column || 0);
       });
       card.appendChild(openSource);
      } else {
       const exactSourceUnavailable = suggestion.investigation_available;
       card.appendChild(element(
        "p",
        "rm-arch__notice is-warning",
        exactSourceUnavailable
         ? this.msg("architecture.value.source_action_unavailable")
         : this.msg("architecture.value.investigation_unavailable")
       ));
       const unavailableReason = exactSourceUnavailable ?
        this.msg("architecture.copy.open_static_export") :
        (suggestion.unavailable_reason || this.msg("architecture.copy.no_exact_source_start"));
       this.appendKeyValue(card, this.msg("architecture.label.reason"), unavailableReason);
      }
     }
     suggestions.appendChild(card);
   });

   const members = this.inspectorSection(this.msg("architecture.section.exact_members"));
   if (array(component.members).length === 0) {
    members.appendChild(element(
     "p",
     "rm-arch__empty",
     this.msg("architecture.copy.no_exact_members")
    ));
   }
   array(component.members).forEach((member) => {
    const card = element("article", "rm-arch__evidence-card");
    card.appendChild(element(
     "strong",
     "rm-arch__evidence-title",
     member.name || memberLabel(member.id, this.message)
    ));
    card.appendChild(element("code", "rm-arch__member-id", memberLabel(member.id, this.message)));
    array(member.facts).forEach((fact, factIndex) => this.appendFact(
     card,
     fact,
     "architecture/components/" + text(component.id) +
      "/members/" + text(member.id && member.id.kind) + "/" + text(member.id && member.id.value) +
      "/facts/" + String(factIndex)
    ));
    members.appendChild(card);
   });

   // Decision 231 (Archive 9): shared participation — the exact shared
   // scope renders as its own section so the user sees "participates in
   // these packages" without cloned exclusive ownership.
   const sharedMembers = array(component.shared_members);
   if (sharedMembers.length > 0) {
    const shared = this.inspectorSection(this.msg("architecture.section.shared_scope"));
    shared.appendChild(element(
     "p",
     "rm-arch__empty",
     this.msg("architecture.copy.shared_scope_copy", { count: array(component.shared_unit_refs).length })
    ));
    sharedMembers.forEach((member) => {
     const card = element("article", "rm-arch__evidence-card rm-arch__shared-member");
     card.appendChild(element(
      "strong",
      "rm-arch__evidence-title",
      member.name || memberLabel(member.id, this.message)
     ));
     card.appendChild(element("code", "rm-arch__member-id", memberLabel(member.id, this.message)));
     shared.appendChild(card);
    });
   }

   const evidence = this.inspectorSection(this.msg("architecture.section.evidence"));
   const anchorIDs = array(component.anchor_ids);
   if (anchorIDs.length === 0) {
    evidence.appendChild(element(
     "p",
     "rm-arch__empty",
     this.msg("architecture.copy.no_architecture_anchor")
    ));
   }
   anchorIDs.forEach((anchorID) => {
      const anchor = this.anchorByID.get(text(anchorID));
      const card = element("article", "rm-arch__evidence-card");
      card.appendChild(element("strong", "rm-arch__evidence-title", anchor ? (anchor.label || anchor.kind) : anchorID));
      card.appendChild(element("code", "rm-arch__member-id", text(anchorID)));
      if (anchor) {
       this.appendKeyValue(card, this.msg("architecture.label.kind"), anchor.kind);
       this.appendKeyValue(card, this.msg("architecture.label.certainty"), anchor.certainty);
       this.appendLocation(card, anchor.location, this.msg("architecture.label.anchor_evidence"));
       this.appendProvenance(
        card,
        anchor.producer ? [anchor.producer] : [],
        "architecture/behavior_anchors/" + text(anchor.id) + "/producer"
       );
      }
      evidence.appendChild(card);
   });

   const unknowns = this.inspectorSection(this.msg("architecture.section.unknowns"));
   if (component.hypothesis) {
    unknowns.appendChild(element(
     "p",
     "rm-arch__notice is-warning",
     this.msg("architecture.copy.component_hypothesis")
    ));
   }
   if (surfaceIDs.length === 0) {
    unknowns.appendChild(element(
     "p",
     "rm-arch__notice",
     this.msg("architecture.copy.zero_surfaces_honest")
    ));
   }
   this.appendDiagnostics(unknowns, "");
  }

  inspectSurface(surface) {
   if (this.userMode) return this.inspectUserSurface(surface);
   this.inspectorHeading(
    this.msg("architecture.label.surface"),
    surface.name || surface.id,
    this.msg("architecture.copy.surface_static_evidence")
   );
   const ownership = this.inspectorSection(this.msg("architecture.section.ownership"));
   this.appendKeyValue(
    ownership,
    this.msg("architecture.label.executable"),
    surface.owning_executable || this.msg("architecture.label.unassigned")
   );
   this.appendKeyValue(
    ownership,
    this.msg("architecture.label.catalog_group"),
    text(surface.category)
   );
   const owner = this.componentByID.get(text(surface.owning_component_id));
   if (owner) {
    const button = element("button", "rm-arch__edge-jump", owner.name || owner.id);
    button.type = "button";
    this.listen(button, "click", () => this.openComponent(owner.id));
    ownership.appendChild(button);
   } else {
    ownership.appendChild(element(
     "p",
     "rm-arch__notice is-warning",
     this.msg("architecture.copy.unassigned_surface")
    ));
   }
   this.appendParticipantComponentLinks(surface, owner && owner.id);

   const progression = this.inspectorSection(this.msg("architecture.label.saved_trace"));
   const trace = this.flowByID.get(text(surface.related_saved_trace_id));
   if (trace) {
    const button = element("button", "rm-arch__edge-jump");
    button.type = "button";
    button.appendChild(element("strong", null, this.msg("architecture.action.open_saved_trace")));
    button.appendChild(element("span", null, [trace.name || trace.id, trace.status].filter(Boolean).join(" · ")));
    this.listen(button, "click", () => this.openTrace(trace.id));
    progression.appendChild(button);
   } else {
    progression.appendChild(element(
     "p",
     "rm-arch__notice is-warning",
     this.msg("architecture.label.trace_unavailable_reason", {
      reason: surface.trace_unavailable_reason || this.msg("architecture.copy.surface_seed_unavailable"),
     })
    ));
   }

   const facts = this.inspectorSection(this.msg("architecture.section.evidence"));
   this.appendKeyValue(facts, this.msg("architecture.label.status"), surface.status);
   this.appendKeyValue(facts, this.msg("architecture.label.certainty"), surface.certainty);
   this.appendKeyValue(facts, this.msg("architecture.label.resolution"), surface.resolution);
   array(surface.evidence).forEach((location) => {
    this.appendLocation(facts, location, this.msg("architecture.label.exact_source"));
   });
  }

  inspectFlow(flow) {
   if (this.userMode) return this.inspectUserFlow(flow);
   this.inspectorHeading(
    this.msg("architecture.label.saved_trace"),
    flow.name || flow.id,
    flow.why_inspect || flow.mental_model || flow.goal
   );
   this.appendFlowEvidence(this.inspector, flow);
  }

  inspectStep(flow, step) {
   if (this.userMode) return this.inspectUserStep(flow, step);
   const branch = this.flowBranch(flow.id, step.branch_id);
   this.inspectorHeading(this.msg("architecture.label.flow_step"), step.label || step.id, step.qualified_name);
   this.appendKeyValue(this.inspector, this.msg("architecture.label.flow"), flow.name || flow.id);
   this.appendKeyValue(this.inspector, this.msg("architecture.label.anchor_kind"), step.kind);
   this.appendKeyValue(
    this.inspector,
    this.msg("architecture.label.branch"),
    branch ? branch.kind : this.msg("architecture.label.unassigned")
   );
   this.appendLocation(this.inspector, step.location, this.msg("architecture.label.declaration"));

   if (step.binding) {
    const binding = this.inspectorSection(this.msg("architecture.section.exact_component_binding"));
    this.appendKeyValue(
     binding,
     this.msg("architecture.label.member"),
     memberLabel(step.binding.member_id, this.message)
    );
    this.appendKeyValue(binding, this.msg("architecture.label.certainty"), step.binding.certainty);
    this.appendLocation(binding, step.binding.location, this.msg("architecture.label.binding_evidence"));
    const bindingPresentationBase = "architecture/flows/" + text(flow.id) +
     "/steps/" + (text(step.id) || String(array(flow.steps).indexOf(step))) + "/binding";
    this.appendProvenance(binding, step.binding.provenance, bindingPresentationBase);
    this.appendScenarios(binding, step.binding.scenarios, bindingPresentationBase);
   } else {
    const missing = this.inspectorSection(this.msg("architecture.section.component_binding"));
    missing.appendChild(element(
     "p",
     "rm-arch__notice is-warning",
     this.msg("architecture.copy.no_unique_member_binding")
    ));
   }

  }

  inspectFlowEdge(edge) {
   if (this.userMode) return this.inspectUserFlowEdge(edge);
   this.inspectorHeading(
    this.msg("architecture.label.flow_transition"),
    edge.relation || edge.id,
    text(edge.from) + " → " + text(edge.to)
   );
   this.appendKeyValue(this.inspector, this.msg("architecture.label.flow"), edge.flow_id);
   this.appendKeyValue(this.inspector, this.msg("architecture.label.resolution"), edge.resolution);
   this.appendKeyValue(
    this.inspector,
    this.msg("architecture.label.invocation"),
    edge.invocation || this.msg("architecture.value.unspecified")
   );
   this.appendKeyValue(this.inspector, this.msg("architecture.label.certainty"), edge.certainty);
   this.appendKeyValue(this.inspector, this.msg("architecture.label.provider"), edge.provider);
   this.appendKeyValue(
    this.inspector,
    this.msg("architecture.label.from_branch"),
    edge.from_branch_id || this.msg("architecture.label.unassigned")
   );
   this.appendKeyValue(
    this.inspector,
    this.msg("architecture.label.to_branch"),
    edge.to_branch_id || this.msg("architecture.label.unassigned")
   );
   this.appendKeyValue(
    this.inspector,
    this.msg("architecture.label.cross_branch"),
    edge.cross_branch ? this.msg("architecture.value.yes") : this.msg("architecture.value.no")
   );
   this.appendLocation(this.inspector, edge.evidence, this.msg("architecture.label.callsite_evidence"));
   const source = this.flowStepsByKey.get(flowStepKey(edge.flow_id, edge.from));
   const target = this.flowStepsByKey.get(flowStepKey(edge.flow_id, edge.to));
   if (source) {
    this.appendLocation(this.inspector, source.location, this.msg("architecture.label.source_declaration"));
   }
   if (target) {
    this.appendLocation(this.inspector, target.location, this.msg("architecture.label.target_declaration"));
   }
   if (edge.condition) {
    const condition = this.inspectorSection(this.msg("architecture.section.source_condition"));
    const conditionText = edge.condition.expression || (
     edge.condition.expression_omitted
      ? this.msg("architecture.value.condition_expression_omitted")
      : this.msg("architecture.value.condition_recorded")
    );
    condition.appendChild(element("code", "rm-arch__condition", conditionText));
    this.appendLocation(condition, edge.condition.location, this.msg("architecture.label.condition_location"));
    condition.appendChild(element(
     "p",
     "rm-arch__copy",
     this.msg("architecture.copy.condition_limit")
    ));
   }
  }

  inspectStructuralEdge(edge) {
   if (this.userMode) return this.inspectUserStructuralEdge(edge);
   const from = this.componentByID.get(text(edge.from_component_id));
   const to = this.componentByID.get(text(edge.to_component_id));
   const witness = edge.witness || {};
   this.inspectorHeading(
    this.msg("architecture.label.structural_evidence"),
    witness.kind || edge.id,
    (from ? from.name : edge.from_component_id) + " → " + (to ? to.name : edge.to_component_id)
   );
   this.appendKeyValue(
    this.inspector,
    this.msg("architecture.label.source_member"),
    memberLabel(witness.from, this.message)
   );
   this.appendKeyValue(
    this.inspector,
    this.msg("architecture.label.target_member"),
    memberLabel(witness.to, this.message)
   );
   this.appendKeyValue(this.inspector, this.msg("architecture.label.certainty"), witness.certainty);
   this.appendKeyValue(this.inspector, this.msg("architecture.label.witness_id"), witness.id);
   this.appendLocation(this.inspector, witness.location, this.msg("architecture.label.local_witness"));
   const relationPresentationBase = "architecture/structural_relations/" + text(witness.id);
   this.appendProvenance(this.inspector, witness.provenance, relationPresentationBase);
   this.appendScenarios(this.inspector, witness.scenarios, relationPresentationBase);
   this.inspector.appendChild(element(
    "p",
    "rm-arch__notice",
    this.msg("architecture.copy.structural_evidence_limit")
   ));
  }

  stepJumpButton(flow, step) {
   const button = element("button", "rm-arch__edge-jump");
   button.type = "button";
   const branch = this.flowBranch(flow.id, step.branch_id);
   button.appendChild(element("strong", null, step.label || step.id));
   button.appendChild(element("span", null, branchLabel(branch && branch.kind, this.message)));
   this.listen(button, "click", () => this.setSelection({
    flow: text(flow.id),
    component: this.flowStepSelectionComponent(flow.id, step.id),
    step: text(step.id),
    edge: "",
   }, true));
   return button;
  }

  appendFact(parent, fact, presentationBase) {
   if (!fact) return;
   if (this.userMode) {
    const block = element("div", "rm-arch__fact");
    if (fact.value) block.appendChild(element("span", "rm-arch__fact-value", fact.value));
    this.appendLocation(block, fact.location, this.msg("architecture.action.open_code"));
    if (block.childElementCount > 0) parent.appendChild(block);
    return;
   }
   const block = element("div", "rm-arch__fact");
   const heading = element("div", "rm-arch__fact-heading");
   heading.appendChild(element(
    "span",
    "rm-arch__badge",
    fact.kind || this.msg("architecture.fallback.fact")
   ));
   heading.appendChild(element(
    "span",
    "rm-arch__badge is-muted",
    fact.certainty || this.msg("architecture.label.unknown")
   ));
   block.appendChild(heading);
   if (fact.value) block.appendChild(element("span", "rm-arch__fact-value", fact.value));
   this.appendLocation(block, fact.location, this.msg("architecture.section.evidence"));
   this.appendProvenance(block, fact.provenance, presentationBase);
   parent.appendChild(block);
  }

  appendKeyValue(parent, key, value) {
   if (value == null || value === "") return;
   if (this.userMode) {
    const internalKeys = [
     this.msg("architecture.label.answer_coverage"),
     this.msg("architecture.label.architecture_anchors"),
     this.msg("architecture.label.architecture_source"),
     this.msg("architecture.label.certainty"),
     this.msg("architecture.label.current_frontier"),
     this.msg("architecture.label.derived_verdict"),
     this.msg("architecture.label.evidence_basis"),
     this.msg("architecture.label.grounding"),
     this.msg("architecture.label.grounding_mode"),
     this.msg("architecture.label.model"),
     this.msg("architecture.label.model_verdict"),
     this.msg("architecture.label.provider"),
     this.msg("architecture.label.resolution"),
     this.msg("architecture.label.status"),
     this.msg("architecture.label.trace_quality"),
     this.msg("architecture.label.verdict"),
    ].map((label) => label.toLowerCase());
    if (internalKeys.indexOf(text(key).toLowerCase()) >= 0) return;
   }
   const row = element("div", "rm-arch__key-value");
   row.appendChild(element("span", "rm-arch__key", key));
   row.appendChild(element("span", "rm-arch__value", value));
   parent.appendChild(row);
  }

  appendSummaryItem(parent, key, value) {
   if (value == null || value === "") return;
   if (this.userMode) {
    const internalKeys = [
     this.msg("architecture.label.current_frontier"),
     this.msg("architecture.label.evidence_basis"),
     this.msg("architecture.label.proof_coverage"),
     this.msg("architecture.label.status"),
     this.msg("architecture.label.verdict"),
    ].map((label) => label.toLowerCase());
    if (internalKeys.indexOf(text(key).toLowerCase()) >= 0) return;
   }
   const row = element("div");
   row.appendChild(element("dt", null, key));
   row.appendChild(element("dd", null, value));
   parent.appendChild(row);
  }

  appendLocation(parent, location, label) {
   const formatted = locationLabel(location);
   if (!formatted) return;
   const row = element("div", "rm-arch__location-row");
   row.appendChild(element(
    "span",
    "rm-arch__key",
    label || this.msg("architecture.label.location")
   ));
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
    if (!this.userMode && this.options.stalePaths && this.options.stalePaths.has && this.options.stalePaths.has(text(location.path))) {
     parent.appendChild(element(
      "p",
      "rm-arch__source-warning",
      this.msg("architecture.copy.source_changed")
     ));
    }
  }

  appendProvenance(parent, provenanceItems, presentationBase) {
   if (this.userMode) return;
   const items = array(provenanceItems);
   if (items.length === 0) return;
   const details = element("details", "rm-arch__details");
   details.appendChild(element(
    "summary",
    null,
    this.msg("architecture.count.provenance", { count: items.length })
   ));
   const list = element("ul", "rm-arch__detail-list");
   items.forEach((provenance, provenanceIndex) => {
    const item = element("li");
    const detailMessageID = architectureProvenanceProductMessageID(provenance);
    const detailAddress = text(presentationBase) + "/provenance/" + String(provenanceIndex) + "/detail";
    const detail = detailMessageID
     ? this.msg(detailMessageID)
     : this.presented(detailAddress, provenance.detail);
    const description = [provenance.provider, provenance.operation, provenance.version, detail]
     .filter(Boolean)
     .join(" · ");
    item.appendChild(element(
     "span",
     null,
     description || this.msg("architecture.fallback.unspecified_provenance")
    ));
    this.appendLocation(item, provenance.location, this.msg("architecture.section.source"));
    list.appendChild(item);
   });
   details.appendChild(list);
   parent.appendChild(details);
  }

  appendScenarios(parent, scenarios, presentationBase) {
   if (this.userMode) return;
   const items = array(scenarios);
   if (items.length === 0) return;
   const details = element("details", "rm-arch__details");
   details.appendChild(element(
    "summary",
    null,
    this.msg("architecture.count.scenarios", { count: items.length })
   ));
   const list = element("ul", "rm-arch__detail-list");
   items.forEach((scenario, scenarioIndex) => {
    const item = element("li");
    const scenarioMessageID = architectureScenarioProductMessageID(scenario);
    item.appendChild(element(
     "strong",
     null,
     (scenarioMessageID && this.msg(scenarioMessageID)) ||
      this.presented(
       text(presentationBase) + "/scenarios/" + String(scenarioIndex) + "/name",
       scenario.name
      ) || scenario.id || this.msg("architecture.fallback.scenario")
    ));
    const build = scenario.build || {};
    const context = [build.goos, build.goarch, array(build.build_tags).join(", ")].filter(Boolean).join(" · ");
    if (context) item.appendChild(element("code", null, context));
    list.appendChild(item);
   });
   details.appendChild(list);
   parent.appendChild(details);
  }

  appendDiagnostics(parent, flowID) {
   if (this.userMode) return;
   const diagnostics = this.diagnostics.filter((diagnostic) => !flowID || text(diagnostic.flow_id) === flowID);
   if (diagnostics.length === 0) {
    parent.appendChild(element(
     "p",
     "rm-arch__empty",
     this.msg("architecture.copy.no_saved_diagnostics")
    ));
    return;
   }
   diagnostics.forEach((diagnostic) => {
    const notice = element("article", "rm-arch__notice is-" + (diagnostic.severity || "info"));
    notice.appendChild(element(
     "strong",
     null,
     diagnostic.code || diagnostic.source || this.msg("architecture.fallback.diagnostic")
    ));
    notice.appendChild(element("p", null, diagnostic.message));
    if (diagnostic.member) {
     notice.appendChild(element("code", null, memberLabel(diagnostic.member, this.message)));
    }
    parent.appendChild(notice);
   });
  }

  destroy() {
   if (this.destroyed) return;
   this.destroyed = true;
   if (this.inspectorVisible && typeof this.options.onInspectorVisibilityChange === "function") {
    this.options.onInspectorVisibilityChange(false);
   }
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
   openFlowStep: (flowID, stepID) => app.openFlowStep(flowID, stepID),
   openSurface: (surfaceID) => app.openSurface(surfaceID),
   openComponent: (componentID) => app.openComponent(componentID),
   // Decision 236 (v11): Map lens switching is an emphasis projection
   // over the same layout — exposed so the workspace can switch lenses.
   setLens: (lens) => app.setLens(lens),
   // Canvas 15/group v2: one exact entry group at a time overlays the
   // existing component geometry. Selection never relayouts or pans.
   selectEntrypointHandoffGroup: (groupID) => app.selectEntrypointHandoffGroup(groupID),
   // D246: one backend-published Study path overlays the existing
   // Landscape geometry. It never relayouts, focuses, or pans.
   setStudyMechanismOverlay: (mechanism) => app.setStudyMechanismOverlay(mechanism),
   clearStudyMechanismOverlay: () => app.clearStudyMechanismOverlay(),
   // Decision 236 (v11): the DOM-free lens projection for the workspace
   // objects panel — same function the emphasis uses, so visible objects
   // and dimmed nodes can never disagree.
   projectArchitectureLens: projectArchitectureLens,
   openSemanticArtifact: (artifactID, index) => app.openSemanticArtifact(artifactID, index),
   openGuidedTourStep: (index) => app.openGuidedTourStep(index),
   destroy: () => app.destroy(),
  });
 }

 global.RepomapArchitectureCanvas = Object.freeze({
  mount: mount,
  // Decision 236 (v11): the DOM-free lens projection — a real product
  // consumer (the workspace objects panel) calls it with the report data;
  // the canvas instance uses the same function for emphasis. Never a
  // test-only hook.
  projectArchitectureLens: projectArchitectureLens,
  projectUserPresentation: projectArchitectureUserPresentation,
  projectEntrypointHandoffOverlay: entryHandoffOverlayProjection,
  projectStudyMechanismOverlay: studyMechanismOverlayProjection,
 });
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
   proofAreaLabel: proofAreaLabel,
   presentationValueLabel: presentationValueLabel,
   architectureProvenanceProductMessageID: architectureProvenanceProductMessageID,
   architectureScenarioProductMessageID: architectureScenarioProductMessageID,
   architecturePresentationText: architecturePresentationText,
   architecturePartialTruth: architecturePartialTruth,
   mapStructuralEdges: mapStructuralEdges,
   architectureStepComponentState: (step, componentIDs) => architectureStepComponentState(
    step,
    new Map(array(componentIDs).map((componentID) => [text(componentID), true]))
   ),
   // Decision 236 (v11): the pure lens projection — no DOM, no geometry.
   mapLensEmphasisProjection: mapLensEmphasisProjection,
   currentEntrypointHandoffGroup: currentEntrypointHandoffGroup,
   entryHandoffOverlayProjection: entryHandoffOverlayProjection,
   entryHandoffConnectionGeometry: entryHandoffConnectionGeometry,
   currentStudyMechanism: currentStudyMechanism,
   studyMechanismOverlayProjection: studyMechanismOverlayProjection,
   projectArchitectureLens: projectArchitectureLens,
   associationsForComponent: associationsForComponent,
  });
 }
})(window);
