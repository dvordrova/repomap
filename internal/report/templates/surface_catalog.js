(function (global) {
 "use strict";

 const PAGE_SIZE = 6;
 const KIND_FILTERS = [
  { value: "all", label: "All" },
  { value: "process_entry", label: "Process entries" },
  { value: "cli_command", label: "CLI commands" },
  { value: "http_route", label: "HTTP registrations" },
  { value: "http_route_descriptor", label: "Route descriptors" },
  { value: "http_route_frontier", label: "Route frontiers" },
  { value: "http_server", label: "Server start sites" },
  { value: "worker", label: "Workers" },
  { value: "async_task", label: "Non-worker tasks" },
 ];
 const EVIDENCE_FILTERS = [
  { value: "all", label: "All evidence" },
  { value: "direct", label: "Direct" },
  { value: "wrapper", label: "Through wrapper" },
  { value: "dynamic", label: "Dynamic / unresolved" },
 ];
 const STATUS_LABELS = {
  confirmed_direct_registration: "Confirmed registration",
  confirmed_through_library_wrapper: "Confirmed registration",
  confirmed_through_repository_wrapper: "Confirmed registration",
  confirmed_server_start_call: "Static start call found",
  confirmed_route_descriptor: "Admin route descriptor found",
  configured_route_inventory_unresolved: "Configured routes unresolved",
  dynamic_unknown: "Dynamic registration",
  confirmed_async_task_start: "Async task",
  possible_worker_loop: "Possible worker",
  confirmed_worker_registration: "Worker registration",
  confirmed_command_registration: "Confirmed command registration",
  partial_command_registration: "Partial command registration",
  confirmed_process_entry: "Exact process entry",
 };
 const GROUPS = [
  { value: "application", label: "Application" },
  { value: "secondary_service", label: "Secondary services" },
  { value: "tooling", label: "Tooling" },
  { value: "tests_helpers", label: "Tests/helpers" },
  { value: "unassigned", label: "Unassigned" },
  { value: "dynamic_unresolved", label: "Dynamic/unresolved" },
  { value: "unavailable", label: "Unavailable" },
 ];

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

 function sentenceLabel(value) {
  const source = text(value).trim().replace(/_/g, " ");
  if (!source) return "Unknown";
  return source.charAt(0).toUpperCase() + source.slice(1);
 }

 function statusLabel(status) {
  return STATUS_LABELS[status] || sentenceLabel(status);
 }

 function certaintyLabel(certainty) {
  if (text(certainty).toLowerCase() === "static") return "Static · not observed";
  return sentenceLabel(certainty);
 }

  function primaryLabel(trigger) {
  const identity = object(trigger.identity);
  if (trigger.kind === "http_route") {
   const method = valueText(identity.method).trim().toUpperCase() || "HTTP";
   const path = valueKnown(identity.path) ? valueText(identity.path) : "<dynamic route>";
   return method + " " + path;
  }
  if (trigger.kind === "http_route_descriptor") {
   const path = valueKnown(identity.path) ? valueText(identity.path) : "<dynamic route>";
   return "Route descriptor " + path;
  }
  if (trigger.kind === "http_route_frontier") return "Configured route inventory";
   if (trigger.kind === "http_server") return "HTTP server start call";
   if (trigger.kind === "process_entry") {
    return "Process entry " + (executableOwner(trigger) || compactSymbolLabel(object(trigger.process_entrypoint).id) || "main");
   }
  return compactSymbolLabel(identity.name || trigger.name || valueText(trigger.handler)) || "Unnamed surface";
 }

 function compactSymbolLabel(value) {
  let label = text(value).trim();
  if (!label) return "";
  const slash = label.lastIndexOf("/");
  if (slash >= 0) label = label.slice(slash + 1);
  const task = label.match(/\.([^.]+)\$(\d+)$/);
  if (task) return task[1] + " · task " + task[2];
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

 function frontierPresentation(frontier) {
  switch (text(frontier.kind)) {
   case "configuration_assembled_route_inventory":
    return { label: "Configuration-built routes", detail: "Runtime configuration assembles these routes; static analysis did not invent or enumerate them." };
   case "unresolved_dispatch_inventory":
    return { label: "Route inventory unresolved", detail: "No supported route registration was correlated with this static start call." };
   case "route_provider_dispatch_candidate":
    return { label: "Provider selection unresolved", detail: "The returned descriptor is exact, but runtime provider selection and consumer registration were not observed." };
   case "call_target_limit":
    return { label: "Static dispatch bounded", detail: "Some possible call targets were intentionally left unresolved." };
   case "entrypoint_dispatch_unresolved":
    return { label: "Entrypoint handoff unresolved", detail: "The callback is build-selected, but its runtime handoff from the process entrypoint was not observed." };
   case "dynamic_route_identity":
    return { label: "Route identity unresolved", detail: "The route path depends on values unavailable to bounded static analysis." };
   case "dynamic_handler_identity":
    return { label: "Handler identity unresolved", detail: "The handler depends on values unavailable to bounded static analysis." };
   default:
    return { label: sentenceLabel(frontier.kind), detail: text(frontier.detail) };
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
   if (!host || host.nodeType !== 1) {
    throw new TypeError("RepomapSurfaceCatalog.mount requires a host Element");
   }
   this.host = host;
   this.data = object(data);
   this.coverage = object(this.data.coverage || this.data.surface_coverage);
    this.options = object(options);
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
   this.host.replaceChildren();
   this.root = element("section", "rm-surface");
   this.root.setAttribute("aria-labelledby", "rm-surface-title");

   const heading = element("div", "rm-surface__heading");
   const headingCopy = element("div", "rm-surface__heading-copy");
   const titleLine = element("div", "rm-surface__title-line");
    const title = element("h2", "rm-surface__title", "All surfaces");
   title.id = "rm-surface-title";
   titleLine.appendChild(title);
   titleLine.appendChild(element("span", "rm-surface__source", "Local static analysis"));
   headingCopy.appendChild(titleLine);
   headingCopy.appendChild(
    element(
     "p",
     "rm-surface__intro",
      "Bounded static catalog of supported registrations, route descriptors, route frontiers, and start call sites. This is not a runtime trace or a complete route inventory; configuration-built routes may remain unresolved."
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
     const message = hasCatalog
      ? "No supported runtime registrations were cataloged." +
       (anchorCount > 0 ? " " + anchorCount + " architecture-anchor families remain available." : "")
      : "No surface catalog is available for this saved run.";
    this.root.appendChild(this.emptyState(message));
    this.root.appendChild(this.renderCoverage());
    this.host.appendChild(this.root);
    return;
   }

   this.renderCounts();
   this.filters = element("div", "rm-surface__filters");
   this.filters.appendChild(this.filterGroup("Surface kind", KIND_FILTERS, "kind"));
   this.filters.appendChild(this.filterGroup("Evidence", EVIDENCE_FILTERS, "evidence"));
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
   const coverage = this.coverage;
   const frontierFallback = this.triggers.filter((trigger) => evidenceClass(trigger) === "dynamic").length;
    const traceReady = this.triggers.filter((trigger) => text(trigger.trace_readiness) === "trace_ready").length;
    const partialReady = this.triggers.filter((trigger) => text(trigger.trace_readiness) === "partial_trace_ready").length;
    const runtimeActivities = this.triggers.filter((trigger) => text(trigger.surface_role) === "runtime_activity").length;
    const dynamicFrontiers = this.triggers.filter((trigger) => text(trigger.surface_role) === "dynamic_frontier").length;
    const rejectedNoisy = this.triggers.filter((trigger) => ["rejected", "noisy"].includes(text(trigger.surface_role))).length;
     const metrics = [
     {
      label: "total",
      value: firstNumber([this.data.total_count], this.triggers.length),
     },
      { label: "trace-ready", value: traceReady },
      { label: "partial-trace candidates", value: partialReady },
      { label: "runtime activities", value: runtimeActivities },
      { label: "rejected/noisy", value: rejectedNoisy },
      {
       label: "CLI commands",
      value: firstNumber([this.data.cli_command_count], countByKind(this.triggers, "cli_command")),
      },
      {
       label: "process entries",
       value: firstNumber([this.data.process_entry_count], countByKind(this.triggers, "process_entry")),
      },
     {
      label: "application",
      value: firstNumber(
       [this.data.application_count],
       this.triggers.filter((trigger) => trigger.executable_role === "primary_application").length
      ),
     },
      {
       label: "secondary services",
       value: firstNumber(
        [this.data.secondary_service_count],
        this.triggers.filter((trigger) => trigger.executable_role === "secondary_service").length
       ),
      },
      {
       label: "tooling",
      value: firstNumber(
       [this.data.tooling_count],
        this.triggers.filter((trigger) => ["tooling", "secondary_tooling"].includes(trigger.executable_role)).length
       ),
      },
      {
       label: "unavailable",
       value: firstNumber(
        [this.data.unavailable_surface_count],
        this.triggers.filter((trigger) => trigger.availability === "unavailable").length
       ),
      },
     {
      label: "HTTP registrations",
      value: firstNumber([this.data.http_count, this.data.http_route_count], countByKind(this.triggers, "http_route")),
     },
     {
      label: "route descriptors",
      value: firstNumber([this.data.http_route_descriptor_count], countByKind(this.triggers, "http_route_descriptor")),
     },
     {
      label: "route frontiers",
      value: firstNumber([this.data.http_route_frontier_count], countByKind(this.triggers, "http_route_frontier")),
     },
     {
      label: "server start sites",
      value: firstNumber([this.data.http_server_count], countByKind(this.triggers, "http_server")),
     },
    {
     label: "workers",
     value: firstNumber([this.data.worker_count, coverage.workers], countByKind(this.triggers, "worker")),
    },
    {
      label: "non-worker async tasks",
     value: firstNumber(
      [this.data.async_task_count, coverage.async_tasks],
      countByKind(this.triggers, "async_task")
     ),
    },
     {
      label: "dynamic frontiers",
      value: firstNumber(
       [this.data.dynamic_frontier_count],
       dynamicFrontiers || array(this.data.dynamic_frontiers || coverage.dynamic_frontiers).length || frontierFallback
      ),
    },
   ];
   metrics.forEach((metric) => {
    const chip = element("span", "rm-surface__count");
    chip.appendChild(element("strong", "", metric.value));
    chip.appendChild(document.createTextNode(" " + metric.label));
    this.summary.appendChild(chip);
   });
  }

  filterGroup(label, filters, dimension) {
   const group = element("div", "rm-surface__filter-group");
   group.setAttribute("role", "group");
   group.setAttribute("aria-label", label);
   group.appendChild(element("span", "rm-surface__filter-label", label));
   const buttons = dimension === "kind" ? this.kindButtons : this.evidenceButtons;
   filters.forEach((filter) => {
    const button = element("button", "rm-surface__filter", filter.label);
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
   this.list.replaceChildren();
   const matches = this.matchingTriggers();
   const hasCatalog =
    this.data.analysis_ran !== false &&
    (Object.prototype.hasOwnProperty.call(this.data, "triggers") || this.data.version || this.data.analyzer_version);

   if (this.triggers.length === 0) {
     const anchorCount = Number(this.options.architectureAnchorCount || 0);
     const message = hasCatalog
      ? "No supported runtime registrations were cataloged." +
       (anchorCount > 0 ? " " + anchorCount + " architecture-anchor families remain available." : "")
      : "No surface catalog is available for this saved run.";
    this.list.appendChild(this.emptyState(message));
    this.liveStatus.textContent = message;
    this.moreButton.hidden = true;
    return;
   }
   if (matches.length === 0) {
     const empty = this.emptyState("No surfaces match these filters.");
    const reset = element("button", "rm-surface__reset", "Reset filters");
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
     this.liveStatus.textContent = "No surfaces match these filters.";
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
     const heading = element("h3", "rm-surface__group-title", group.label);
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
   this.moreButton.textContent = this.expanded ? "Show less" : "Show " + hiddenCount + " more";
   this.moreButton.setAttribute("aria-expanded", this.expanded ? "true" : "false");
    this.liveStatus.textContent = "Showing " + visible.length + " of " + matches.length + " surfaces.";
  }

  emptyState(message) {
   const empty = element("div", "rm-surface__empty");
   empty.appendChild(element("p", "", message));
   empty.appendChild(
     element("p", "rm-surface__empty-note", "Architecture anchors and the supported surface catalog have different scopes; absence here does not prove runtime absence.")
   );
   return empty;
  }

  renderTrigger(trigger, index) {
   const card = element("details", "rm-surface__item");
   const id = triggerID(trigger, index);
     const association = Object.assign({}, object(trigger), object(this.architectureSurfaces.get(id)));
   card.dataset.surfaceId = id;
   const summary = element("summary", "rm-surface__item-summary");
   const identityBlock = element("span", "rm-surface__identity");
   const primaryText = primaryLabel(trigger);
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
     const ownership = element("span", "rm-surface__owner", "Executable · " + owner);
     ownership.title = owner;
     identityBlock.appendChild(ownership);
    }
   summary.appendChild(identityBlock);
   const serverStart = trigger.kind === "http_server";
   const descriptor = trigger.kind === "http_route_descriptor";
   const routeFrontier = trigger.kind === "http_route_frontier";
    const processEntry = trigger.kind === "process_entry";
    const registration = locationLabel(processEntry ? object(trigger.process_entrypoint).location : (serverStart ? trigger.server_start_site : (descriptor ? trigger.descriptor_site : trigger.registration_site)));
    const locationVerb = processEntry ? "declared " : (serverStart ? "start call " : (descriptor ? "descriptor " : (routeFrontier ? "assembled " : "registered ")));
   summary.appendChild(
    element("span", "rm-surface__registration", registration ? locationVerb + registration : "")
   );

   const semantics = element("dl", "rm-surface__semantics");
   semantics.appendChild(this.semantic("Status", statusLabel(trigger.status)));
   semantics.appendChild(this.semantic("Role", sentenceLabel(trigger.surface_role)));
   semantics.appendChild(this.semantic("Trace readiness", sentenceLabel(trigger.trace_readiness)));
   semantics.appendChild(this.semantic("Certainty", certaintyLabel(trigger.certainty)));
   semantics.appendChild(this.semantic("Resolution", sentenceLabel(trigger.resolution)));
   summary.appendChild(semantics);
   card.appendChild(summary);

   const body = element("div", "rm-surface__item-body");
   const facts = element("dl", "rm-surface__details");
    appendText(facts, "Kind", sentenceLabel(trigger.kind));
    appendText(facts, "Framework", trigger.framework);
      appendText(facts, "Executable role", sentenceLabel(trigger.executable_role));
      appendText(facts, "Availability", sentenceLabel(trigger.availability));
      appendText(facts, "Unavailable reason", trigger.unavailable_reason);
      appendText(facts, "Trace readiness reason", trigger.trace_readiness_reason);
      const quality = object(trigger.quality);
      appendText(
       facts,
       "Quality",
       ["identity", "registration_start", "handler_callback", "reachability", "ownership", "traceability"]
        .map((dimension) => dimension.replace(/_/g, " ") + ": " + text(quality[dimension]))
        .filter((dimension) => !dimension.endsWith(": "))
        .join(" · ")
      );
     appendText(facts, "Transport", trigger.transport);
     appendText(facts, "Handler / callback", handler);
     appendText(facts, "Constructor", text(object(trigger.constructor).name));
    if (trigger.provisional_id && hasDynamicEvidence(trigger)) appendText(facts, "Identity", "Provisional; unresolved values may change it");
    body.appendChild(facts);

    const progression = element("section", "rm-surface__progression");
    progression.appendChild(element("h4", "", "Architecture progression"));
    if (association.owning_component_id && typeof this.options.openComponent === "function") {
     const component = element("button", "rm-surface__action", "Open owning component");
     component.type = "button";
     this.listen(component, "click", () => this.options.openComponent(association.owning_component_id));
     progression.appendChild(component);
    } else {
     progression.appendChild(element("p", "rm-surface__caveat", "Unassigned surface — no unique exact component owner was found."));
    }
    if (association.related_saved_trace_id && typeof this.options.openTrace === "function") {
     const trace = element("button", "rm-surface__action is-primary", "Open saved trace");
     trace.type = "button";
     this.listen(trace, "click", () => this.options.openTrace(association.related_saved_trace_id));
     progression.appendChild(trace);
    } else {
     progression.appendChild(element(
      "p",
      "rm-surface__unavailable",
       "Trace unavailable: " + (association.trace_unavailable_reason || "no compatible saved trace was found")
     ));
    }
    if (typeof this.options.openSurface === "function") {
     const inspect = element("button", "rm-surface__action", "View in Architecture");
     inspect.type = "button";
     this.listen(inspect, "click", () => this.options.openSurface(id));
     progression.appendChild(inspect);
    }
    body.appendChild(progression);

   const locations = element("div", "rm-surface__locations");
     if (trigger.kind === "http_route") this.appendLocation(locations, "Registration", trigger.registration_site);
     if (trigger.kind === "http_route_descriptor") {
      this.appendLocation(locations, "Descriptor source", trigger.descriptor_site);
      this.appendLocation(locations, "Provider call", trigger.registration_site);
     }
     if (trigger.kind === "http_route_frontier") this.appendLocation(locations, "Route assembly", trigger.registration_site);
    this.appendLocation(locations, "Constructor", object(trigger.constructor).location);
    this.appendLocation(locations, "Handler / callback", trigger.handler_location);
   this.appendLocation(locations, "Server start", trigger.server_start_site);
   this.appendLocation(locations, "Process entrypoint", object(trigger.process_entrypoint).location);
   if (locations.childElementCount > 0) body.appendChild(this.detailSection("Source locations", locations));

   const middleware = array(trigger.middleware);
   if (middleware.length > 0) {
    const content = element("div", "rm-surface__stack");
    content.appendChild(
     element("p", "rm-surface__caveat", "Registered middleware identities; execution order was not observed.")
    );
    middleware.forEach((item) => content.appendChild(element("code", "rm-surface__code", valueText(item) || "unknown")));
    body.appendChild(this.detailSection("Middleware", content));
   }

   const wrappers = array(trigger.wrapper_chain);
   if (wrappers.length > 0) {
    const content = element("div", "rm-surface__stack");
    content.appendChild(
     element("p", "rm-surface__caveat", "Derived registration chain; runtime execution order was not observed.")
    );
    const list = element("ol", "rm-surface__evidence-list");
    wrappers.forEach((wrapper) => {
     const item = element("li", "rm-surface__wrapper");
     const symbol = object(wrapper.symbol);
     item.appendChild(element("strong", "", symbol.name || symbol.id || "wrapper"));
     if (wrapper.origin) item.appendChild(element("span", "rm-surface__muted", " · " + wrapper.origin));
     this.appendLocation(item, "Declaration", symbol.location);
     this.appendLocation(item, "Callsite", wrapper.callsite);
     list.appendChild(item);
    });
    content.appendChild(list);
    body.appendChild(this.detailSection("Wrapper chain", content));
   }

   const evidence = array(trigger.evidence);
   if (evidence.length > 0) {
    const content = element("ul", "rm-surface__evidence-list");
    evidence.forEach((fact) => {
     const item = element("li", "rm-surface__evidence");
     item.appendChild(element("strong", "", sentenceLabel(fact.kind)));
     if (fact.detail) item.appendChild(element("span", "rm-surface__muted", " — " + fact.detail));
     this.appendLocation(item, "Evidence", fact.location);
     content.appendChild(item);
    });
    body.appendChild(this.detailSection("Evidence", content));
   }

   const frontiers = array(trigger.dynamic_frontier);
   if (frontiers.length > 0) body.appendChild(this.detailSection("Dynamic frontiers", this.renderFrontiers(frontiers)));

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
   const rendered = locationLabel(location);
   if (!rendered) return;
   const row = element("div", "rm-surface__location");
   row.appendChild(element("span", "rm-surface__location-label", label));
   if (typeof this.options.openLocation === "function") {
    const button = element("button", "rm-surface__location-button", rendered + " ↗");
    button.type = "button";
    button.title = "Open in editor";
    this.listen(button, "click", () => this.openLocation(location));
    row.appendChild(button);
   } else {
    row.appendChild(element("code", "rm-surface__location-text", rendered));
   }
   parent.appendChild(row);
  }

  openLocation(location) {
   try {
    const result = this.options.openLocation(object(location));
    if (result && typeof result.catch === "function") {
     result.catch(() => {
      this.liveStatus.textContent = "The editor could not open " + locationLabel(location) + ".";
     });
    }
   } catch (_error) {
    this.liveStatus.textContent = "The editor could not open " + locationLabel(location) + ".";
   }
  }

  renderFrontiers(frontiers) {
   const list = element("ul", "rm-surface__frontiers");
   frontiers.forEach((frontier) => {
    const item = element("li", "rm-surface__frontier");
     const presentation = frontierPresentation(frontier);
     item.appendChild(element("strong", "", presentation.label));
     if (presentation.detail) item.appendChild(element("span", "", " — " + presentation.detail));
    this.appendLocation(item, "Frontier", frontier.location);
    list.appendChild(item);
   });
   return list;
  }

  renderCoverage() {
   const coverage = Object.keys(this.coverage).length > 0 ? this.coverage : this.data;
   const details = element("details", "rm-surface__coverage");
    details.appendChild(element("summary", "", "View coverage and limits"));
   const body = element("div", "rm-surface__coverage-body");
   body.appendChild(
    element(
     "p",
     "rm-surface__scope",
     coverage.scope_statement ||
      this.data.scope_statement ||
      "Configured terminal seeds and bounded static propagation under the recorded build scenario."
    )
   );
   body.appendChild(
    element(
     "p",
     "rm-surface__caveat",
      "Static registration evidence does not prove callback execution, middleware order, branch choice, or process lifetime. Worker and non-worker async-task counts are exclusive catalog classifications and are independent of selected FlowProof coverage."
    )
   );

  const scenario = object(coverage.scenario);
  const facts = element("dl", "rm-surface__details rm-surface__coverage-facts");
  appendText(facts, "Scenario", this.data.scenario_id || scenario.id);
  appendText(facts, "Analyzer", this.data.analyzer_version);
  appendText(facts, "Catalog surfaces", coverage.total_count);
  if (coverage.truncated === true) appendText(facts, "Surfaces embedded", this.triggers.length);
  appendText(facts, "Direct surfaces", coverage.direct_count);
  appendText(facts, "Wrapper-derived", coverage.wrapper_count);
  appendText(facts, "Workers", coverage.worker_count);
   appendText(facts, "Non-worker async tasks", coverage.async_task_count);
   appendText(facts, "Process entries", coverage.process_entry_count == null ? coverage.process_entries : coverage.process_entry_count);
   appendText(facts, "Unavailable process entries", coverage.unavailable_process_entries);
   appendText(facts, "Unavailable packages", coverage.unavailable_package_count);
   appendText(facts, "Package diagnostics", coverage.package_diagnostic_count);
  if (coverage.truncated === true) appendText(facts, "Projection bound", "Reached; additional triggers were not embedded");
  appendText(facts, "Packages inspected", coverage.packages_inspected);
  appendText(facts, "Functions inspected", coverage.functions_inspected);
  if (Array.isArray(coverage.configured_seeds_matched)) {
   appendText(facts, "Configured seeds matched", coverage.configured_seeds_matched.length);
  }
  appendText(
   facts,
   "Unresolved handlers",
   coverage.unresolved_handler_count == null ? coverage.unresolved_handlers : coverage.unresolved_handler_count
  );
  appendText(
   facts,
   "Possible registrations",
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
     item.appendChild(element("strong", "", sentenceLabel(signal.kind)));
     if (signal.function_id) item.appendChild(element("span", "rm-surface__muted", " · " + signal.function_id));
     if (signal.detail) item.appendChild(element("p", "", signal.detail));
     this.appendLocation(item, "Signal", signal.location);
     list.appendChild(item);
    });
    body.appendChild(this.detailSection("Loop signals", list));
   }

   const frontiers = array(this.data.dynamic_frontiers || coverage.dynamic_frontiers).concat(
    array(coverage.unsupported_dispatch_mechanisms)
   );
   if (frontiers.length > 0) body.appendChild(this.detailSection("Analysis frontiers", this.renderFrontiers(frontiers)));

   const unavailablePackages = array(this.data.unavailable_packages || coverage.unavailable_packages);
   if (unavailablePackages.length > 0) {
    const list = element("ul", "rm-surface__evidence-list");
    unavailablePackages.forEach((unavailablePackage) => {
     const item = element("li", "rm-surface__diagnostic");
     item.appendChild(element("strong", "", unavailablePackage.package || unavailablePackage.package_name || "Unavailable package"));
     if (unavailablePackage.owning_executable) item.appendChild(element("span", "rm-surface__muted", " · " + unavailablePackage.owning_executable));
     if (unavailablePackage.reason) item.appendChild(element("p", "", sentenceLabel(unavailablePackage.reason)));
     list.appendChild(item);
    });
    body.appendChild(this.detailSection("Unavailable packages", list));
   }

   const packageDiagnostics = array(this.data.package_diagnostics || coverage.package_diagnostics);
   if (packageDiagnostics.length > 0) {
    const list = element("ul", "rm-surface__evidence-list");
    packageDiagnostics.forEach((diagnostic) => {
     const item = element("li", "rm-surface__diagnostic");
     item.appendChild(element("strong", "", diagnostic.package || diagnostic.package_name || "Package diagnostic"));
     if (diagnostic.message) item.appendChild(element("p", "", diagnostic.message));
     this.appendLocation(item, "Diagnostic", diagnostic.location);
     list.appendChild(item);
    });
    body.appendChild(this.detailSection("Package diagnostics", list));
   }

   const budgets = array(coverage.budgets_reached);
   if (budgets.length > 0) {
    const list = element("ul", "rm-surface__evidence-list");
    budgets.forEach((budget) => list.appendChild(element("li", "", this.budgetLabel(budget))));
    body.appendChild(this.detailSection("Budgets reached", list));
   }
   details.appendChild(body);
   return details;
  }

  budgetLabel(budget) {
   const labels = {
    depth: "Call-depth bound reached",
    targets: "Dynamic call-target bound reached",
    tasks: "Analysis work-item bound reached",
   };
   return labels[text(budget)] || sentenceLabel(budget);
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
