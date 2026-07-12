# Decision: Behavior-grounded architecture for modular repositories

**Status**: Approved for implementation

## User-visible goal

`repomap <repo>` should distinguish between:

1. a behavior-grounded architecture;
2. a mixed architecture containing both grounded and conceptual areas;
3. a package landscape produced when behavioral grounding is insufficient.

For repositories such as Caddy, the first view should help a user understand:

- how the process starts;
- how configuration enters and is applied;
- how runtime/module lifecycle is managed;
- where the control plane lives;
- where the request/data plane lives;
- how major extension families relate to the runtime.

A package taxonomy remains available as supporting evidence, but must not be
presented as a verified behavioral architecture.

## Problem demonstrated by the Caddy fixture

The current Caddy result is visually readable but primarily groups packages into
categories such as Core, Config, HTTP Module, Testing, and Internal Utilities.

The deterministic surface catalog found insufficient runtime roots.

DeepSeek therefore received package membership as its dominant signal and
produced a renamed package taxonomy rather than a behavioral architecture.

Model-proposed directions such as:

- Server startup and configuration loading;
- Admin API HTTP request handling;
- HTTP request handling for served sites;

are useful hypotheses, but they are not verified merely because the model
returned them.

A generic candidate such as Threshold enforcement must not become a primary
behavior without local process-level evidence.

## Approved implementation scope

### A. Repository archetype

Record a bounded repository archetype:

- application;
- modular_platform_server;
- library_framework;
- cli_tool;
- daemon_worker_system;
- monorepo_mixed.

Use deterministic evidence first.

A bounded model classification may assist but must cite supplied evidence.

### B. Architecture grounding mode

Record one of:

- behavior_grounded;
- mixed;
- package_landscape.

Definitions:

`behavior_grounded`
: Primary architectural pillars are supported by process, dispatch,
  configuration, registry, lifecycle, control-plane, data-plane, or selected
  flow anchors.

`mixed`
: Some primary pillars are locally grounded while others remain conceptual or
  package-derived.

`package_landscape`
: Available grouping is primarily based on packages/imports because sufficient
  behavioral anchors were not recovered.

### C. Behavior anchors

Introduce the smallest necessary contract for exact architecture anchors.

Initial supported kinds may include:

- process_entry;
- command_dispatch;
- config_ingress;
- config_adapter;
- config_apply;
- registry_write;
- registry_lookup;
- lifecycle_interface;
- lifecycle_start;
- admin_control_plane;
- request_dispatch_root;
- application_data_plane;
- tls_or_security_boundary;
- extension_family;
- unresolved_frontier.

Every anchor must retain:

- stable local ID;
- exact source evidence;
- scenario/build identity;
- producer;
- certainty;
- associated local members;
- limitations.

The model may reference anchor IDs but may not create anchors.

### D. Entrypoint-driven composition traversal

Starting from exact process entrypoints, follow bounded repository-local
composition calls to discover:

- command dispatch;
- configuration loading and application;
- server construction;
- control-plane setup;
- request dispatch roots;
- registry setup;
- lifecycle initialization.

Do not build a whole-program call graph.

Use explicit budgets, stable task keys, visited-symbol deduplication, and
no-progress stopping.

Stop before deeply expanding ordinary request or business implementation.

### E. Modular extension architecture

Prototype generic evidence for modular platforms:

- registries storing constructors or implementations by identity;
- configuration-driven registry lookup;
- central interfaces with several implementations;
- core runtime construction of registered implementations;
- lifecycle methods invoked on created implementations;
- registration occurring during initialization.

Do not hard-code repository names.

Small data-driven terminal semantic seeds are allowed when their exact meaning is
verified.

Do not expand every implementation deeply.

### F. Model responsibilities

DeepSeek may:

- name grounded architectural responsibilities;
- group locally supplied IDs;
- choose four to seven primary pillars;
- separate extension families from supporting/tooling packages;
- recommend useful investigation directions;
- explain supplied relationships.

DeepSeek must not:

- create behavioral anchors;
- turn package imports into runtime handoffs;
- mark model directions verified;
- promote generic helpers into primary behaviors without local evidence;
- hide unresolved frontiers;
- assign every package to a primary pillar merely for visual completeness.

### G. Architecture synthesis bundle

The synthesis bundle should prioritize:

- repository archetype;
- project purpose;
- exact behavior anchors;
- locally grounded direction summaries;
- process/config/runtime/control/data relationships;
- registry and lifecycle families;
- unresolved frontiers;
- existing orientation hypotheses.

Packages and member IDs remain supporting membership rather than the dominant
semantic signal.

### H. Relationship presentation

Primary architecture relationships should come from:

- exact semantic handoffs;
- grounded selected-flow transitions;
- registry/lifecycle relationships;
- explicitly marked conceptual hypotheses with supporting anchors.

Package imports remain available in inspector/details or an optional dependency
view.

Do not project every package import as a primary architecture edge.

### I. Product wording

When behavior-grounded:

    Behavioral architecture
    Evidence-backed process and runtime responsibilities

When mixed:

    Architecture hypotheses and grounded behavior
    Some areas remain package-derived

When package-only:

    Package landscape
    Behavioral grounding: low

    Sufficient dispatch, configuration, or lifecycle anchors were not recovered.
    This view groups static packages and imports.

Request/Operational badges describe behavioral category.

They do not indicate proof or execution status.

## Required fixtures

### Caddy

Evaluate:

- modular-platform/server archetype;
- process startup;
- configuration ingress/application;
- module registry/lifecycle;
- admin control plane;
- served-site request/data plane;
- TLS/PKI responsibility;
- extension families;
- rejection or demotion of noisy Threshold enforcement.

A useful behavior-grounded or mixed architecture is preferred.

An honest package-landscape fallback is acceptable when required evidence cannot
be recovered.

### Restic

Preserve:

- accepted Landscape board;
- selected command-flow traces;
- surface inventory;
- existing request/operational classification;
- readable report behavior.

## Validation requirements

Architecture synthesis must be downgraded or rejected when:

- no behavior anchors exist;
- most primary components contain package declarations only;
- descriptions merely restate package names;
- generic utilities/testing dominate a production architecture;
- no process/config/runtime/control/data narrative can be supported;
- primary edges lack supplied relationship evidence;
- more than eight primary pillars are returned without justification;
- noisy model directions have no process-level trigger or bounded investigation
  path.

## Non-goals

Do not implement in this decision:

- full route inventory for every framework;
- complete command discovery for every CLI framework;
- runtime tracing;
- whole-program call graph construction;
- graph database;
- every module implementation;
- every repository archetype;
- a canvas renderer migration;
- a complete FlowProof rewrite;
- Python support;
- automatic troubleshooting or incident-response assistance.

## Implementation sequence

1. Inspect and classify latest Caddy artifacts.
2. Add repository-archetype and grounding-mode contracts.
3. Add behavior-anchor contracts.
4. Expand bounded composition traversal beyond the initial entrypoint.
5. Prototype registry and lifecycle anchors.
6. Build anchor-first architecture synthesis input.
7. Add synthesis validation and package-landscape fallback.
8. Remove package imports from primary architecture edges.
9. Add honest grounding presentation.
10. Validate Caddy and restic through saved fixtures and Playwright.

## Acceptance criteria

The decision is implemented when:

- repository archetype is persisted;
- architecture grounding mode is persisted;
- deterministic behavior anchors are emitted;
- entrypoint composition traversal proceeds beyond the initial function;
- the three plausible Caddy directions are locally evaluated;
- the noisy Threshold enforcement direction is rejected or demoted with reason;
- architecture synthesis receives anchors before package membership;
- primary pillars reference anchors or explicit hypotheses;
- package imports do not dominate primary architecture edges;
- Caddy renders a useful behavioral/mixed architecture or an honest package
  fallback;
- restic remains usable;
- focused offline tests pass;
- Playwright comparison artifacts exist;
- no live model calls are required by automated tests.
