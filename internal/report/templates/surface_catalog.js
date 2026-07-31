(function (global) {
 "use strict";

 const PAGE_SIZE = 6;
 const KIND_FILTERS = [
  { value: "all", messageID: "surfaces.filter.kind.all" },
  { value: "process_entry", messageID: "surfaces.filter.kind.process_entries" },
  { value: "cli_command", messageID: "surfaces.filter.kind.cli_commands" },
  { value: "http_route", messageID: "surfaces.filter.kind.http_registrations" },
  { value: "http_route_descriptor", messageID: "surfaces.filter.kind.route_descriptors" },
  { value: "http_route_frontier", messageID: "surfaces.filter.kind.route_frontiers" },
  { value: "http_server", messageID: "surfaces.filter.kind.server_start_sites" },
  { value: "worker", messageID: "surfaces.filter.kind.workers" },
  { value: "async_task", messageID: "surfaces.filter.kind.non_worker_tasks" },
 ];
 const EVIDENCE_FILTERS = [
  { value: "all", messageID: "surfaces.filter.evidence.all" },
  { value: "direct", messageID: "surfaces.filter.evidence.direct" },
  { value: "wrapper", messageID: "surfaces.filter.evidence.wrapper" },
  { value: "dynamic", messageID: "surfaces.filter.evidence.dynamic" },
 ];
 const STATUS_LABELS = {
  confirmed_direct_registration: "surfaces.status.confirmed_registration",
  confirmed_through_library_wrapper: "surfaces.status.confirmed_registration",
  confirmed_through_repository_wrapper: "surfaces.status.confirmed_registration",
  confirmed_server_start_call: "surfaces.status.static_start_call",
  confirmed_route_descriptor: "surfaces.status.admin_route_descriptor",
  configured_route_inventory_unresolved: "surfaces.status.configured_routes_unresolved",
  dynamic_unknown: "surfaces.status.dynamic_registration",
  confirmed_async_task_start: "surfaces.status.async_task",
  possible_worker_loop: "surfaces.status.possible_worker",
  confirmed_worker_registration: "surfaces.status.worker_registration",
  confirmed_command_registration: "surfaces.status.confirmed_command_registration",
  partial_command_registration: "surfaces.status.partial_command_registration",
  confirmed_process_entry: "surfaces.status.exact_process_entry",
 };
 const GROUPS = [
  { value: "application", messageID: "surfaces.group.application" },
  { value: "secondary_service", messageID: "surfaces.group.secondary_services" },
  { value: "tooling", messageID: "surfaces.group.tooling" },
  { value: "tests_helpers", messageID: "surfaces.group.tests_helpers" },
  { value: "unassigned", messageID: "surfaces.group.unassigned" },
  { value: "dynamic_unresolved", messageID: "surfaces.group.dynamic_unresolved" },
  { value: "unavailable", messageID: "surfaces.group.unavailable" },
 ];
 const QUALITY_LABELS = {
  identity: "surfaces.quality.identity",
  registration_start: "surfaces.quality.registration_start",
  handler_callback: "surfaces.quality.handler_callback",
  reachability: "surfaces.quality.reachability",
  ownership: "surfaces.quality.ownership",
  traceability: "surfaces.quality.traceability",
 };

 function array(value) {
  return Array.isArray(value) ? value : [];
 }

 function object(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
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

 function appendText(parent, label, value) {
  const rendered = text(value).trim();
  if (!rendered) return;
  const row = element("div", "rm-surface__detail-row");
  row.appendChild(element("dt", "rm-surface__detail-label", label));
  row.appendChild(element("dd", "rm-surface__detail-value", rendered));
  parent.appendChild(row);
 }

 function valueText(value) {
  if (typeof value === "string" || typeof value === "number") return text(value);
  return text(object(value).text);
 }

 function valueKnown(value) {
  if (typeof value === "string" || typeof value === "number") return text(value) !== "";
  const item = object(value);
  return item.known !== false && item.kind !== "dynamic" && valueText(item) !== "";
 }

 function displayValueText(value) {
  if (!valueKnown(value)) return "";
  const item = object(value);
  if (["allocation", "field", "global", "call", "alternatives"].includes(text(item.kind))) return "";
  const rendered = valueText(item);
  if (rendered.includes("$") || rendered.includes("@")) return "";
  return rendered;
 }

 function locationLabel(location) {
  const item = object(location);
  if (!item.path) return "";
  let label = text(item.path);
  if (Number(item.line) > 0) label += ":" + Number(item.line);
  return label;
 }

 function executableOwner(trigger) {
  if (trigger.owning_executable) return text(trigger.owning_executable);
  const entrypoint = object(object(trigger).process_entrypoint);
  const path = text(object(entrypoint.location).path);
  if (path.indexOf("/") >= 0) return path.slice(0, path.lastIndexOf("/"));
  return text(entrypoint.package || entrypoint.name);
 }

function surfaceGroup(trigger, association) {
  if (text(trigger.availability) === "unavailable") return "unavailable";
  switch (text(trigger.executable_role)) {
   case "primary_application": return "application";
   case "secondary_service": return "secondary_service";
   case "tooling":
   case "secondary_tooling": return "tooling";
   case "test_or_helper": return "tests_helpers";
  }
  if (hasDynamicEvidence(trigger)) return "dynamic_unresolved";
  return text(association.category || "unassigned");
 }

 function sentenceLabel(value, message) {
  const source = text(value).trim().replace(/_/g, " ");
  if (!source) return message("surfaces.value.unknown");
  return source.charAt(0).toUpperCase() + source.slice(1);
 }

 function statusLabel(status, message) {
  const messageID = STATUS_LABELS[status];
  return messageID ? message(messageID) : sentenceLabel(status, message);
 }

 function certaintyLabel(certainty, message) {
  if (text(certainty).toLowerCase() === "static") return message("surfaces.certainty.static_not_observed");
  return sentenceLabel(certainty, message);
 }

  function primaryLabel(trigger, message) {
  const identity = object(trigger.identity);
  if (trigger.kind === "http_route") {
   const method = valueText(identity.method).trim().toUpperCase() || message("surfaces.value.http");
   const path = valueKnown(identity.path) ? valueText(identity.path) : message("surfaces.value.dynamic_route");
   return message("surfaces.identity.http_route", { method: method, path: path });
  }
  if (trigger.kind === "http_route_descriptor") {
   const path = valueKnown(identity.path) ? valueText(identity.path) : message("surfaces.value.dynamic_route");
   return message("surfaces.identity.route_descriptor", { path: path });
  }
  if (trigger.kind === "http_route_frontier") return message("surfaces.identity.configured_route_inventory");
   if (trigger.kind === "http_server") return message("surfaces.identity.http_server_start");
   if (trigger.kind === "process_entry") {
    return message("surfaces.identity.process_entry", {
     owner: executableOwner(trigger) || compactSymbolLabel(object(trigger.process_entrypoint).id, message) || "main",
    });
   }
  return compactSymbolLabel(identity.name || trigger.name || valueText(trigger.handler), message) ||
   message("surfaces.identity.unnamed");
 }

 function compactSymbolLabel(value, message) {
  let label = text(value).trim();
  if (!label) return "";
  const slash = label.lastIndexOf("/");
  if (slash >= 0) label = label.slice(slash + 1);
  const task = label.match(/\.([^.]+)\$(\d+)$/);
  if (task) return message("surfaces.identity.task", { name: task[1], number: task[2] });
  const dot = label.lastIndexOf(".");
  return dot >= 0 ? label.slice(dot + 1) : label;
 }

 function triggerID(trigger, index) {
  return text(trigger.id || "surface-" + index);
 }

function hasDynamicEvidence(trigger) {
   const resolution = text(trigger.resolution).toLowerCase();
   const status = text(trigger.status).toLowerCase();
   const identity = object(trigger.identity);
   if (trigger.kind === "http_route_frontier") return true;
   if (status === "dynamic_unknown" || status === "configured_route_inventory_unresolved") return true;
   if (trigger.kind === "http_route" || trigger.kind === "http_route_descriptor") {
    return !valueKnown(identity.path) || (trigger.kind === "http_route_descriptor" && !valueKnown(trigger.handler));
   }
   if (trigger.kind === "http_server") return !object(trigger.server_start_site).path;
   return (
    trigger.provisional_id ||
    resolution === "dynamic" ||
    resolution === "ambiguous" ||
    status === "dynamic_unknown"
   );
  }

 function hasWrapperEvidence(trigger) {
  return array(trigger.wrapper_chain).length > 0 ||
   text(trigger.discovery_basis).toLowerCase().indexOf("wrapper") >= 0;
 }

 function evidenceClass(trigger) {
  if (hasDynamicEvidence(trigger)) return "dynamic";
  if (hasWrapperEvidence(trigger)) return "wrapper";
  return "direct";
 }

 function frontierPresentation(frontier, message) {
  switch (text(frontier.kind)) {
   case "configuration_assembled_route_inventory":
    return {
     label: message("surfaces.frontier.configuration_routes.label"),
     detail: message("surfaces.frontier.configuration_routes.detail"),
    };
   case "unresolved_dispatch_inventory":
    return {
     label: message("surfaces.frontier.route_inventory_unresolved.label"),
     detail: message("surfaces.frontier.route_inventory_unresolved.detail"),
    };
   case "route_provider_dispatch_candidate":
    return {
     label: message("surfaces.frontier.provider_selection_unresolved.label"),
     detail: message("surfaces.frontier.provider_selection_unresolved.detail"),
    };
   case "call_target_limit":
    return {
     label: message("surfaces.frontier.static_dispatch_bounded.label"),
     detail: message("surfaces.frontier.static_dispatch_bounded.detail"),
    };
   case "entrypoint_dispatch_unresolved":
    return {
     label: message("surfaces.frontier.entrypoint_handoff_unresolved.label"),
     detail: message("surfaces.frontier.entrypoint_handoff_unresolved.detail"),
    };
   case "dynamic_route_identity":
    return {
     label: message("surfaces.frontier.route_identity_unresolved.label"),
     detail: message("surfaces.frontier.route_identity_unresolved.detail"),
    };
   case "dynamic_handler_identity":
    return {
     label: message("surfaces.frontier.handler_identity_unresolved.label"),
     detail: message("surfaces.frontier.handler_identity_unresolved.detail"),
    };
   default:
    return { label: sentenceLabel(frontier.kind, message), detail: text(frontier.detail) };
  }
 }

 function countByKind(triggers, kind) {
  return triggers.filter((trigger) => trigger.kind === kind).length;
 }

 function firstNumber(values, fallback) {
  for (let index = 0; index < values.length; index += 1) {
   if (typeof values[index] === "number" && Number.isFinite(values[index])) return values[index];
  }
  return fallback;
 }

 class SurfaceCatalogApp {
  constructor(host, data, options) {
   this.options = object(options);
   if (typeof this.options.message !== "function") {
    throw new TypeError("RepomapSurfaceCatalog.mount requires options.message");
   }
   this.message = this.options.message;
   if (!host || host.nodeType !== 1) {
    throw new TypeError(this.message("surfaces.error.host_element"));
   }
   this.host = host;
   this.data = object(data);
   this.coverage = object(this.data.coverage || this.data.surface_coverage);
    this.architectureSurfaces = new Map(array(this.options.architectureSurfaces).map((surface) => [text(surface.id), surface]));
   this.triggers = array(this.data.triggers).map((trigger) => object(trigger));
   this.kindFilter = "all";
   this.evidenceFilter = "all";
   this.expanded = false;
   this.destroyed = false;
   this.events = new AbortController();
   this.kindButtons = new Map();
   this.evidenceButtons = new Map();
   this.hashEnabled = this.options.hash !== false;
   this.build();
   this.restoreHash();
  }

  listen(target, eventName, handler) {
   target.addEventListener(eventName, handler, { signal: this.events.signal });
  }

  build() {
   const message = this.message;
   this.host.replaceChildren();
   this.root = element("section", "rm-surface");
   this.root.setAttribute("aria-labelledby", "rm-surface-title");

   const heading = element("div", "rm-surface__heading");
   const headingCopy = element("div", "rm-surface__heading-copy");
   const titleLine = element("div", "rm-surface__title-line");
    const title = element("h2", "rm-surface__title", message("surfaces.title"));
   title.id = "rm-surface-title";
   titleLine.appendChild(title);
   titleLine.appendChild(element("span", "rm-surface__source", message("surfaces.source.local_static")));
   headingCopy.appendChild(titleLine);
   headingCopy.appendChild(
    element(
     "p",
     "rm-surface__intro",
      message("surfaces.intro")
    )
   );
   heading.appendChild(headingCopy);
   this.summary = element("div", "rm-surface__counts");
   heading.appendChild(this.summary);
   this.root.appendChild(heading);

   if (this.triggers.length === 0) {
    this.root.classList.add("is-empty");
    this.summary.remove();
    const hasCatalog =
     this.data.analysis_ran !== false &&
     (Object.prototype.hasOwnProperty.call(this.data, "triggers") || this.data.version || this.data.analyzer_version);
     const anchorCount = Number(this.options.architectureAnchorCount || 0);
     const emptyMessage = hasCatalog
      ? message(
       anchorCount > 0 ? "surfaces.empty.catalog_with_anchors" : "surfaces.empty.catalog",
       anchorCount > 0 ? { count: anchorCount } : {}
      )
      : message("surfaces.empty.unavailable");
    this.root.appendChild(this.emptyState(emptyMessage));
    this.root.appendChild(this.renderCoverage());
    this.host.appendChild(this.root);
    return;
   }

   this.renderCounts();
   this.filters = element("div", "rm-surface__filters");
   this.filters.appendChild(this.filterGroup(message("surfaces.filter.kind.label"), KIND_FILTERS, "kind"));
   this.filters.appendChild(this.filterGroup(message("surfaces.filter.evidence.label"), EVIDENCE_FILTERS, "evidence"));
   this.root.appendChild(this.filters);

   this.liveStatus = element("p", "rm-surface__live");
   this.liveStatus.setAttribute("role", "status");
   this.liveStatus.setAttribute("aria-live", "polite");
   this.root.appendChild(this.liveStatus);

   this.list = element("div", "rm-surface__list");
   this.root.appendChild(this.list);
   this.moreButton = element("button", "rm-surface__more");
   this.moreButton.type = "button";
   this.listen(this.moreButton, "click", () => {
    this.expanded = !this.expanded;
    this.renderList();
   });
   this.root.appendChild(this.moreButton);
   this.root.appendChild(this.renderCoverage());
   this.host.appendChild(this.root);

   if (this.hashEnabled) this.listen(global, "hashchange", () => this.restoreHash());
   this.renderList();
  }

  renderCounts() {
   const message = this.message;
   const coverage = this.coverage;
   const frontierFallback = this.triggers.filter((trigger) => evidenceClass(trigger) === "dynamic").length;
    const traceReady = this.triggers.filter((trigger) => text(trigger.trace_readiness) === "trace_ready").length;
    const partialReady = this.triggers.filter((trigger) => text(trigger.trace_readiness) === "partial_trace_ready").length;
    const runtimeActivities = this.triggers.filter((trigger) => text(trigger.surface_role) === "runtime_activity").length;
    const dynamicFrontiers = this.triggers.filter((trigger) => text(trigger.surface_role) === "dynamic_frontier").length;
    const rejectedNoisy = this.triggers.filter((trigger) => ["rejected", "noisy"].includes(text(trigger.surface_role))).length;
     const metrics = [
     {
      messageID: "surfaces.metric.total",
      value: firstNumber([this.data.total_count], this.triggers.length),
     },
      { messageID: "surfaces.metric.trace_ready", value: traceReady },
      { messageID: "surfaces.metric.partial_trace_candidates", value: partialReady },
      { messageID: "surfaces.metric.runtime_activities", value: runtimeActivities },
      { messageID: "surfaces.metric.rejected_noisy", value: rejectedNoisy },
      {
       messageID: "surfaces.metric.cli_commands",
      value: firstNumber([this.data.cli_command_count], countByKind(this.triggers, "cli_command")),
      },
      {
       messageID: "surfaces.metric.process_entries",
       value: firstNumber([this.data.process_entry_count], countByKind(this.triggers, "process_entry")),
      },
     {
      messageID: "surfaces.metric.application",
      value: firstNumber(
       [this.data.application_count],
       this.triggers.filter((trigger) => trigger.executable_role === "primary_application").length
      ),
     },
      {
       messageID: "surfaces.metric.secondary_services",
       value: firstNumber(
        [this.data.secondary_service_count],
        this.triggers.filter((trigger) => trigger.executable_role === "secondary_service").length
       ),
      },
      {
       messageID: "surfaces.metric.tooling",
      value: firstNumber(
       [this.data.tooling_count],
        this.triggers.filter((trigger) => ["tooling", "secondary_tooling"].includes(trigger.executable_role)).length
       ),
      },
      {
       messageID: "surfaces.metric.unavailable",
       value: firstNumber(
        [this.data.unavailable_surface_count],
        this.triggers.filter((trigger) => trigger.availability === "unavailable").length
       ),
      },
      {
       messageID: "surfaces.metric.supporting_dependency",
       value: firstNumber(
        [this.data.supporting_dependency_count],
        this.triggers.filter((trigger) => trigger.application_classification === "supporting_dependency_behavior").length
       ),
      },
      {
       messageID: "surfaces.metric.dependency_only",
       value: firstNumber(
        [this.data.dependency_only_count],
        this.triggers.filter((trigger) => trigger.application_classification === "dependency_only").length
       ),
      },
     {
      messageID: "surfaces.metric.http_registrations",
      value: firstNumber([this.data.http_count, this.data.http_route_count], countByKind(this.triggers, "http_route")),
     },
     {
      messageID: "surfaces.metric.route_descriptors",
      value: firstNumber([this.data.http_route_descriptor_count], countByKind(this.triggers, "http_route_descriptor")),
     },
     {
      messageID: "surfaces.metric.route_frontiers",
      value: firstNumber([this.data.http_route_frontier_count], countByKind(this.triggers, "http_route_frontier")),
     },
     {
      messageID: "surfaces.metric.server_start_sites",
      value: firstNumber([this.data.http_server_count], countByKind(this.triggers, "http_server")),
     },
    {
     messageID: "surfaces.metric.workers",
     value: firstNumber([this.data.worker_count, coverage.workers], countByKind(this.triggers, "worker")),
    },
    {
      messageID: "surfaces.metric.non_worker_async_tasks",
     value: firstNumber(
      [this.data.async_task_count, coverage.async_tasks],
      countByKind(this.triggers, "async_task")
     ),
    },
     {
      messageID: "surfaces.metric.dynamic_frontiers",
      value: firstNumber(
       [this.data.dynamic_frontier_count],
       dynamicFrontiers || array(this.data.dynamic_frontiers || coverage.dynamic_frontiers).length || frontierFallback
      ),
    },
   ];
   metrics.forEach((metric) => {
    const chip = element("span", "rm-surface__count");
    chip.appendChild(element("strong", "", metric.value));
    chip.appendChild(document.createTextNode(" " + message(metric.messageID)));
    this.summary.appendChild(chip);
   });
  }

  filterGroup(label, filters, dimension) {
   const message = this.message;
   const group = element("div", "rm-surface__filter-group");
   group.setAttribute("role", "group");
   group.setAttribute("aria-label", label);
   group.appendChild(element("span", "rm-surface__filter-label", label));
   const buttons = dimension === "kind" ? this.kindButtons : this.evidenceButtons;
   filters.forEach((filter) => {
    const button = element("button", "rm-surface__filter", message(filter.messageID));
    button.type = "button";
    button.setAttribute("aria-pressed", filter.value === "all" ? "true" : "false");
    this.listen(button, "click", () => {
     if (dimension === "kind") this.kindFilter = filter.value;
     else this.evidenceFilter = filter.value;
     this.expanded = false;
     this.updateFilterButtons();
     this.clearHashSelection();
     this.renderList();
    });
    buttons.set(filter.value, button);
    group.appendChild(button);
   });
   return group;
  }

  updateFilterButtons() {
   this.kindButtons.forEach((button, value) => {
    button.setAttribute("aria-pressed", value === this.kindFilter ? "true" : "false");
   });
   this.evidenceButtons.forEach((button, value) => {
    button.setAttribute("aria-pressed", value === this.evidenceFilter ? "true" : "false");
   });
  }

  matchingTriggers() {
   return this.triggers.filter((trigger) => {
    const kindMatches = this.kindFilter === "all" || trigger.kind === this.kindFilter;
    const evidenceMatches = this.evidenceFilter === "all" ||
     (this.evidenceFilter === "wrapper" && hasWrapperEvidence(trigger)) ||
     (this.evidenceFilter === "dynamic" && hasDynamicEvidence(trigger)) ||
     (this.evidenceFilter === "direct" && !hasWrapperEvidence(trigger));
    return kindMatches && evidenceMatches;
   });
  }

  renderList(selectedID) {
   const message = this.message;
   this.list.replaceChildren();
   const matches = this.matchingTriggers();
   const hasCatalog =
    this.data.analysis_ran !== false &&
    (Object.prototype.hasOwnProperty.call(this.data, "triggers") || this.data.version || this.data.analyzer_version);

   if (this.triggers.length === 0) {
     const anchorCount = Number(this.options.architectureAnchorCount || 0);
     const emptyMessage = hasCatalog
      ? message(
       anchorCount > 0 ? "surfaces.empty.catalog_with_anchors" : "surfaces.empty.catalog",
       anchorCount > 0 ? { count: anchorCount } : {}
      )
      : message("surfaces.empty.unavailable");
    this.list.appendChild(this.emptyState(emptyMessage));
    this.liveStatus.textContent = emptyMessage;
    this.moreButton.hidden = true;
    return;
   }
   if (matches.length === 0) {
     const empty = this.emptyState(message("surfaces.empty.filters"));
    const reset = element("button", "rm-surface__reset", message("surfaces.action.reset_filters"));
    reset.type = "button";
    this.listen(reset, "click", () => {
     this.kindFilter = "all";
     this.evidenceFilter = "all";
     this.expanded = false;
     this.updateFilterButtons();
     this.renderList();
    });
    empty.appendChild(reset);
    this.list.appendChild(empty);
     this.liveStatus.textContent = message("surfaces.empty.filters");
    this.moreButton.hidden = true;
    return;
   }

   const visible = this.expanded ? matches : matches.slice(0, PAGE_SIZE);
    GROUPS.forEach((group) => {
      const groupedMatches = matches.filter((trigger) => {
       const association = object(this.architectureSurfaces.get(triggerID(trigger, this.triggers.indexOf(trigger))));
       return surfaceGroup(trigger, association) === group.value;
      });
      const grouped = visible.filter((trigger) => {
      const association = object(this.architectureSurfaces.get(triggerID(trigger, this.triggers.indexOf(trigger))));
       return surfaceGroup(trigger, association) === group.value;
     });
     if (grouped.length === 0) return;
     const section = element("section", "rm-surface__group");
     const heading = element("h3", "rm-surface__group-title", message(group.messageID));
      heading.appendChild(element("span", "rm-surface__group-count", groupedMatches.length));
     section.appendChild(heading);
     grouped.forEach((trigger) => {
      const index = this.triggers.indexOf(trigger);
      const card = this.renderTrigger(trigger, index);
      if (selectedID && triggerID(trigger, index) === selectedID) card.open = true;
      section.appendChild(card);
     });
     this.list.appendChild(section);
    });
   const hiddenCount = matches.length - PAGE_SIZE;
   this.moreButton.hidden = hiddenCount <= 0;
   this.moreButton.textContent = this.expanded
    ? message("surfaces.action.show_less")
    : message("surfaces.action.show_more", { count: hiddenCount });
   this.moreButton.setAttribute("aria-expanded", this.expanded ? "true" : "false");
    this.liveStatus.textContent = message("surfaces.status.showing", {
     visible: visible.length,
     total: matches.length,
    });
  }

  emptyState(message) {
   const empty = element("div", "rm-surface__empty");
   empty.appendChild(element("p", "", message));
   empty.appendChild(
     element("p", "rm-surface__empty-note", message("surfaces.empty.scope_note"))
   );
   return empty;
  }

  renderTrigger(trigger, index) {
   const message = this.message;
   const card = element("details", "rm-surface__item");
   const id = triggerID(trigger, index);
     const association = Object.assign({}, object(trigger), object(this.architectureSurfaces.get(id)));
   card.dataset.surfaceId = id;
   const summary = element("summary", "rm-surface__item-summary");
   const identityBlock = element("span", "rm-surface__identity");
   const primaryText = primaryLabel(trigger, message);
   const primary = element("span", "rm-surface__primary", primaryText);
   primary.title = primary.textContent;
   identityBlock.appendChild(primary);

   const handler = displayValueText(trigger.handler);
     if (trigger.kind !== "http_server" && handler && handler !== primaryText) {
    const callback = element("span", "rm-surface__handler", handler);
    callback.title = handler;
     identityBlock.appendChild(callback);
    }
     const owner = association.owning_executable || executableOwner(trigger);
    if (owner) {
     const ownership = element("span", "rm-surface__owner", message("surfaces.owner.executable", { owner: owner }));
     ownership.title = owner;
     identityBlock.appendChild(ownership);
    }
   summary.appendChild(identityBlock);
   const serverStart = trigger.kind === "http_server";
   const descriptor = trigger.kind === "http_route_descriptor";
   const routeFrontier = trigger.kind === "http_route_frontier";
    const processEntry = trigger.kind === "process_entry";
    const registration = locationLabel(processEntry ? object(trigger.process_entrypoint).location : (serverStart ? trigger.server_start_site : (descriptor ? trigger.descriptor_site : trigger.registration_site)));
    let registrationMessageID = "surfaces.location.registered";
    if (processEntry) registrationMessageID = "surfaces.location.declared";
    else if (serverStart) registrationMessageID = "surfaces.location.start_call";
    else if (descriptor) registrationMessageID = "surfaces.location.descriptor";
    else if (routeFrontier) registrationMessageID = "surfaces.location.assembled";
   summary.appendChild(
    element(
     "span",
     "rm-surface__registration",
     registration ? message(registrationMessageID, { location: registration }) : ""
    )
   );

   const semantics = element("dl", "rm-surface__semantics");
   semantics.appendChild(this.semantic(message("surfaces.field.status"), statusLabel(trigger.status, message)));
   semantics.appendChild(this.semantic(message("surfaces.field.role"), sentenceLabel(trigger.surface_role, message)));
   semantics.appendChild(
    this.semantic(message("surfaces.field.trace_readiness"), sentenceLabel(trigger.trace_readiness, message))
   );
   semantics.appendChild(
    this.semantic(message("surfaces.field.certainty"), certaintyLabel(trigger.certainty, message))
   );
   semantics.appendChild(
    this.semantic(message("surfaces.field.resolution"), sentenceLabel(trigger.resolution, message))
   );
   summary.appendChild(semantics);
   card.appendChild(summary);

   const body = element("div", "rm-surface__item-body");
   const facts = element("dl", "rm-surface__details");
    appendText(facts, message("surfaces.field.kind"), sentenceLabel(trigger.kind, message));
    appendText(facts, message("surfaces.field.framework"), trigger.framework);
       appendText(
        facts,
        message("surfaces.field.executable_role"),
        sentenceLabel(trigger.executable_role, message)
       );
       appendText(facts, message("surfaces.field.availability"), sentenceLabel(trigger.availability, message));
       appendText(
        facts,
        message("surfaces.field.application_ownership"),
        sentenceLabel(trigger.application_classification, message)
       );
       appendText(
        facts,
        message("surfaces.field.terminal_source"),
        sentenceLabel(trigger.terminal_source_scope, message)
       );
       appendText(
        facts,
        message("surfaces.field.promotion_basis"),
        sentenceLabel(trigger.promotion_basis, message)
       );
      appendText(facts, message("surfaces.field.unavailable_reason"), trigger.unavailable_reason);
      appendText(facts, message("surfaces.field.trace_readiness_reason"), trigger.trace_readiness_reason);
      const quality = object(trigger.quality);
      appendText(
       facts,
       message("surfaces.field.quality"),
       ["identity", "registration_start", "handler_callback", "reachability", "ownership", "traceability"]
        .map((dimension) => {
         const value = text(quality[dimension]);
         if (!value) return "";
         return message(QUALITY_LABELS[dimension]) + ": " + value;
        })
        .filter(Boolean)
        .join(" · ")
      );
     appendText(facts, message("surfaces.field.transport"), trigger.transport);
     appendText(facts, message("surfaces.field.handler_callback"), handler);
     appendText(facts, message("surfaces.field.constructor"), text(object(trigger.constructor).name));
    if (trigger.provisional_id && hasDynamicEvidence(trigger)) {
     appendText(facts, message("surfaces.field.identity"), message("surfaces.identity.provisional"));
    }
    body.appendChild(facts);

    const progression = element("section", "rm-surface__progression");
    progression.appendChild(element("h4", "", message("surfaces.progression.title")));
    if (association.owning_component_id && typeof this.options.openComponent === "function") {
     const component = element("button", "rm-surface__action", message("surfaces.action.open_owning_component"));
     component.type = "button";
     this.listen(component, "click", () => this.options.openComponent(association.owning_component_id));
     progression.appendChild(component);
    } else {
     progression.appendChild(element("p", "rm-surface__caveat", message("surfaces.progression.unassigned")));
    }
    if (association.related_saved_trace_id && typeof this.options.openTrace === "function") {
     const trace = element("button", "rm-surface__action is-primary", message("surfaces.action.open_saved_trace"));
     trace.type = "button";
     this.listen(trace, "click", () => this.options.openTrace(association.related_saved_trace_id));
     progression.appendChild(trace);
    } else {
     progression.appendChild(element(
      "p",
      "rm-surface__unavailable",
       message("surfaces.progression.trace_unavailable", {
        reason: association.trace_unavailable_reason || message("surfaces.progression.trace_unavailable_default"),
       })
     ));
    }
    if (typeof this.options.openSurface === "function") {
     const inspect = element("button", "rm-surface__action", message("surfaces.action.view_in_architecture"));
     inspect.type = "button";
     this.listen(inspect, "click", () => this.options.openSurface(id));
     progression.appendChild(inspect);
    }
    body.appendChild(progression);

   const locations = element("div", "rm-surface__locations");
     if (trigger.kind === "http_route") {
      this.appendLocation(locations, message("surfaces.location_label.registration"), trigger.registration_site);
     }
     if (trigger.kind === "http_route_descriptor") {
      this.appendLocation(locations, message("surfaces.location_label.descriptor_source"), trigger.descriptor_site);
      this.appendLocation(locations, message("surfaces.location_label.provider_call"), trigger.registration_site);
     }
     if (trigger.kind === "http_route_frontier") {
      this.appendLocation(locations, message("surfaces.location_label.route_assembly"), trigger.registration_site);
     }
    this.appendLocation(locations, message("surfaces.field.constructor"), object(trigger.constructor).location);
    this.appendLocation(locations, message("surfaces.field.handler_callback"), trigger.handler_location);
   this.appendLocation(locations, message("surfaces.location_label.server_start"), trigger.server_start_site);
   this.appendLocation(
    locations,
    message("surfaces.location_label.process_entrypoint"),
    object(trigger.process_entrypoint).location
   );
   if (locations.childElementCount > 0) {
    body.appendChild(this.detailSection(message("surfaces.section.source_locations"), locations));
   }

   const middleware = array(trigger.middleware);
   if (middleware.length > 0) {
    const content = element("div", "rm-surface__stack");
    content.appendChild(
     element("p", "rm-surface__caveat", message("surfaces.middleware.caveat"))
    );
    middleware.forEach((item) => {
     content.appendChild(
      element("code", "rm-surface__code", valueText(item) || message("surfaces.value.unknown"))
     );
    });
    body.appendChild(this.detailSection(message("surfaces.section.middleware"), content));
   }

   const wrappers = array(trigger.wrapper_chain);
   if (wrappers.length > 0) {
    const content = element("div", "rm-surface__stack");
    content.appendChild(
     element("p", "rm-surface__caveat", message("surfaces.wrapper.caveat"))
    );
    const list = element("ol", "rm-surface__evidence-list");
    wrappers.forEach((wrapper) => {
     const item = element("li", "rm-surface__wrapper");
     const symbol = object(wrapper.symbol);
     item.appendChild(element("strong", "", symbol.name || symbol.id || message("surfaces.value.wrapper")));
     if (wrapper.origin) item.appendChild(element("span", "rm-surface__muted", " · " + wrapper.origin));
     this.appendLocation(item, message("surfaces.location_label.declaration"), symbol.location);
     this.appendLocation(item, message("surfaces.location_label.callsite"), wrapper.callsite);
     list.appendChild(item);
    });
    content.appendChild(list);
    body.appendChild(this.detailSection(message("surfaces.section.wrapper_chain"), content));
   }

   const evidence = array(trigger.evidence);
   if (evidence.length > 0) {
    const content = element("ul", "rm-surface__evidence-list");
    evidence.forEach((fact) => {
     const item = element("li", "rm-surface__evidence");
     item.appendChild(element("strong", "", sentenceLabel(fact.kind, message)));
     if (fact.detail) item.appendChild(element("span", "rm-surface__muted", " — " + fact.detail));
     this.appendLocation(item, message("surfaces.section.evidence"), fact.location);
     content.appendChild(item);
    });
    body.appendChild(this.detailSection(message("surfaces.section.evidence"), content));
   }

   const frontiers = array(trigger.dynamic_frontier);
   if (frontiers.length > 0) {
    body.appendChild(
     this.detailSection(message("surfaces.section.dynamic_frontiers"), this.renderFrontiers(frontiers))
    );
   }

   card.appendChild(body);
   this.listen(card, "toggle", () => {
    if (card.open) this.writeHashSelection(id);
    else this.clearHashSelection(id);
   });
   return card;
  }

  semantic(label, value) {
   const item = element("div", "rm-surface__semantic");
   item.appendChild(element("dt", "", label));
   item.appendChild(element("dd", "", value));
   return item;
  }

  detailSection(label, content) {
   const section = element("section", "rm-surface__detail-section");
   section.appendChild(element("h4", "", label));
   section.appendChild(content);
   return section;
  }

  appendLocation(parent, label, location) {
   const message = this.message;
   const rendered = locationLabel(location);
   if (!rendered) return;
   const row = element("div", "rm-surface__location");
   row.appendChild(element("span", "rm-surface__location-label", label));
   if (typeof this.options.openLocation === "function") {
    const button = element(
     "button",
     "rm-surface__location-button",
     message("surfaces.location.open", { location: rendered })
    );
    button.type = "button";
    button.title = message("surfaces.action.open_in_editor");
    this.listen(button, "click", () => this.openLocation(location));
    row.appendChild(button);
   } else {
    row.appendChild(element("code", "rm-surface__location-text", rendered));
   }
   parent.appendChild(row);
  }

  openLocation(location) {
   const message = this.message;
   try {
    const result = this.options.openLocation(object(location));
    if (result && typeof result.catch === "function") {
     result.catch(() => {
      this.liveStatus.textContent = message("surfaces.error.editor_open", {
       location: locationLabel(location),
      });
     });
    }
   } catch (_error) {
    this.liveStatus.textContent = message("surfaces.error.editor_open", {
     location: locationLabel(location),
    });
   }
  }

  renderFrontiers(frontiers) {
   const message = this.message;
   const list = element("ul", "rm-surface__frontiers");
   frontiers.forEach((frontier) => {
    const item = element("li", "rm-surface__frontier");
     const presentation = frontierPresentation(frontier, message);
     item.appendChild(element("strong", "", presentation.label));
     if (presentation.detail) item.appendChild(element("span", "", " — " + presentation.detail));
    this.appendLocation(item, message("surfaces.location_label.frontier"), frontier.location);
    list.appendChild(item);
   });
   return list;
  }

  renderCoverage() {
   const message = this.message;
   const coverage = Object.keys(this.coverage).length > 0 ? this.coverage : this.data;
   const details = element("details", "rm-surface__coverage");
    details.appendChild(element("summary", "", message("surfaces.coverage.title")));
   const body = element("div", "rm-surface__coverage-body");
   body.appendChild(
    element(
     "p",
     "rm-surface__scope",
     coverage.scope_statement ||
      this.data.scope_statement ||
      message("surfaces.coverage.default_scope")
    )
   );
   body.appendChild(
    element(
     "p",
     "rm-surface__caveat",
      message("surfaces.coverage.caveat")
    )
   );

  const scenario = object(coverage.scenario);
  const facts = element("dl", "rm-surface__details rm-surface__coverage-facts");
  appendText(facts, message("surfaces.coverage.scenario"), this.data.scenario_id || scenario.id);
  appendText(facts, message("surfaces.coverage.analyzer"), this.data.analyzer_version);
  appendText(facts, message("surfaces.coverage.catalog_surfaces"), coverage.total_count);
  if (coverage.truncated === true) {
   appendText(facts, message("surfaces.coverage.surfaces_embedded"), this.triggers.length);
  }
  appendText(facts, message("surfaces.coverage.direct_surfaces"), coverage.direct_count);
  appendText(facts, message("surfaces.coverage.wrapper_derived"), coverage.wrapper_count);
  appendText(facts, message("surfaces.coverage.workers"), coverage.worker_count);
   appendText(facts, message("surfaces.coverage.non_worker_async_tasks"), coverage.async_task_count);
   appendText(
    facts,
    message("surfaces.coverage.process_entries"),
    coverage.process_entry_count == null ? coverage.process_entries : coverage.process_entry_count
   );
   appendText(
    facts,
    message("surfaces.coverage.unavailable_process_entries"),
    coverage.unavailable_process_entries
   );
   appendText(facts, message("surfaces.coverage.unavailable_packages"), coverage.unavailable_package_count);
   appendText(facts, message("surfaces.coverage.package_diagnostics"), coverage.package_diagnostic_count);
  if (coverage.truncated === true) {
   appendText(
    facts,
    message("surfaces.coverage.projection_bound"),
    message("surfaces.coverage.projection_bound_reached")
   );
  }
  appendText(facts, message("surfaces.coverage.packages_inspected"), coverage.packages_inspected);
  appendText(facts, message("surfaces.coverage.functions_inspected"), coverage.functions_inspected);
  if (Array.isArray(coverage.configured_seeds_matched)) {
   appendText(
    facts,
    message("surfaces.coverage.configured_seeds_matched"),
    coverage.configured_seeds_matched.length
   );
  }
  appendText(
   facts,
   message("surfaces.coverage.unresolved_handlers"),
   coverage.unresolved_handler_count == null ? coverage.unresolved_handlers : coverage.unresolved_handler_count
  );
  appendText(
   facts,
   message("surfaces.coverage.possible_registrations"),
   coverage.possible_registration_count == null
    ? coverage.possible_registrations
    : coverage.possible_registration_count
  );
   body.appendChild(facts);

   const loopSignals = array(this.data.loop_signals || coverage.loop_signals);
   if (loopSignals.length > 0) {
    const list = element("ul", "rm-surface__evidence-list");
    loopSignals.forEach((signal) => {
     const item = element("li", "rm-surface__loop");
     item.appendChild(element("strong", "", sentenceLabel(signal.kind, message)));
     if (signal.function_id) item.appendChild(element("span", "rm-surface__muted", " · " + signal.function_id));
     if (signal.detail) item.appendChild(element("p", "", signal.detail));
     this.appendLocation(item, message("surfaces.location_label.signal"), signal.location);
     list.appendChild(item);
    });
    body.appendChild(this.detailSection(message("surfaces.section.loop_signals"), list));
   }

   const frontiers = array(this.data.dynamic_frontiers || coverage.dynamic_frontiers).concat(
    array(coverage.unsupported_dispatch_mechanisms)
   );
   if (frontiers.length > 0) {
    body.appendChild(
     this.detailSection(message("surfaces.section.analysis_frontiers"), this.renderFrontiers(frontiers))
    );
   }

   const unavailablePackages = array(this.data.unavailable_packages || coverage.unavailable_packages);
   if (unavailablePackages.length > 0) {
    const list = element("ul", "rm-surface__evidence-list");
    unavailablePackages.forEach((unavailablePackage) => {
     const item = element("li", "rm-surface__diagnostic");
     item.appendChild(
      element(
       "strong",
       "",
       unavailablePackage.package ||
        unavailablePackage.package_name ||
        message("surfaces.value.unavailable_package")
      )
     );
     if (unavailablePackage.owning_executable) item.appendChild(element("span", "rm-surface__muted", " · " + unavailablePackage.owning_executable));
     if (unavailablePackage.reason) {
      item.appendChild(element("p", "", sentenceLabel(unavailablePackage.reason, message)));
     }
     list.appendChild(item);
    });
    body.appendChild(this.detailSection(message("surfaces.section.unavailable_packages"), list));
   }

   const packageDiagnostics = array(this.data.package_diagnostics || coverage.package_diagnostics);
   if (packageDiagnostics.length > 0) {
    const list = element("ul", "rm-surface__evidence-list");
    packageDiagnostics.forEach((diagnostic) => {
     const item = element("li", "rm-surface__diagnostic");
     item.appendChild(
      element(
       "strong",
       "",
       diagnostic.package || diagnostic.package_name || message("surfaces.value.package_diagnostic")
      )
     );
     if (diagnostic.message) item.appendChild(element("p", "", diagnostic.message));
     this.appendLocation(item, message("surfaces.location_label.diagnostic"), diagnostic.location);
     list.appendChild(item);
    });
    body.appendChild(this.detailSection(message("surfaces.section.package_diagnostics"), list));
   }

   const budgets = array(coverage.budgets_reached);
   if (budgets.length > 0) {
    const list = element("ul", "rm-surface__evidence-list");
    budgets.forEach((budget) => list.appendChild(element("li", "", this.budgetLabel(budget))));
    body.appendChild(this.detailSection(message("surfaces.section.budgets_reached"), list));
   }
   details.appendChild(body);
   return details;
  }

  budgetLabel(budget) {
   const message = this.message;
   const labels = {
    depth: "surfaces.budget.depth",
    targets: "surfaces.budget.targets",
    tasks: "surfaces.budget.tasks",
   };
   const messageID = labels[text(budget)];
   return messageID ? message(messageID) : sentenceLabel(budget, message);
  }

  hashValue() {
   if (!this.hashEnabled || !global.location) return "";
   const params = new URLSearchParams(global.location.hash.replace(/^#/, ""));
   return text(params.get("surface"));
  }

  restoreHash() {
   const selectedID = this.hashValue();
   if (!selectedID) return;
   const index = this.triggers.findIndex((trigger, triggerIndex) => triggerID(trigger, triggerIndex) === selectedID);
   if (index < 0) return;
   this.kindFilter = "all";
   this.evidenceFilter = "all";
   this.expanded = index >= PAGE_SIZE;
   this.updateFilterButtons();
   this.renderList(selectedID);
  }

  writeHashSelection(id) {
   if (!this.hashEnabled || !global.location || !global.history) return;
   const params = new URLSearchParams(global.location.hash.replace(/^#/, ""));
   params.set("surface", id);
   this.replaceHash(params);
  }

  clearHashSelection(expectedID) {
   if (!this.hashEnabled || !global.location || !global.history) return;
   const params = new URLSearchParams(global.location.hash.replace(/^#/, ""));
   if (expectedID && params.get("surface") !== expectedID) return;
   params.delete("surface");
   this.replaceHash(params);
  }

  replaceHash(params) {
   const url = new URL(global.location.href);
   const rendered = params.toString();
   url.hash = rendered ? "#" + rendered : "";
   global.history.replaceState(null, "", url.href);
  }

  destroy() {
   if (this.destroyed) return;
   this.destroyed = true;
   this.events.abort();
   this.host.replaceChildren();
  }
 }

 function mount(host, data, options) {
  return new SurfaceCatalogApp(host, data, options);
 }

 global.RepomapSurfaceCatalog = Object.freeze({ mount: mount });
})(window);
