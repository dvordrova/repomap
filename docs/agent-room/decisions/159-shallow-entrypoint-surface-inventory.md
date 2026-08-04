# Decision 159: Build a generic shallow entrypoint and surface inventory

## Status

Active product corrective, authorized by the repository owner after review of
the held Decision 158 exact Cobra tree reconstruction.

## Product problem

Ordinary runs omit useful commands when a framework is assembled through
wrappers, globals, `init` registration, or other shapes outside the legacy
package-local reader. Decision 158 tried to solve the omission by reconstructing
an exact rooted Cobra tree. Adversarial review showed that this required
interpreting instance identity, mutation order, aliases, wrappers, and dynamic
slice flow. That is too fragile for the product need.

The useful fact is shallower: a typed command, endpoint, or other externally
invocable surface is worth showing when its declaration and handler are known.
Exact registration and startup relationships improve that fact, but their
absence must not erase it.

The architecture must also avoid making Cobra the product model. Commands are
the first adapter target. HTTP endpoints, event consumers, scheduled jobs, and
outbound client integrations should later be able to contribute the same kinds
of neutral facts.

## Neutral fact contract

Surface adapters emit three independent fact kinds:

- **Descriptor** — a build-selected, typed declaration of an externally
  invocable or observable surface. It carries its adapter kind, stable local
  identity, display label, declaration location, handler location when known,
  and exact framework/type evidence.
- **Binding** — a direct typed relationship that attaches one descriptor to a
  parent, container, router, registry, or executable. A binding is exact only
  when both endpoints and the binding expression are unambiguous.
- **Activation** — a direct typed relationship showing that an executable or
  startup site invokes a known container or descriptor. It does not by itself
  prove a downstream runtime mechanism.

Descriptors publish independently of Binding and Activation reachability.
Binding and Activation enrich the inventory; they are not admission gates.
The ordinary product may label a descriptor as declared, registered, or
activated according to the facts actually available.

Ambiguous or dynamic relationships remain bounded partial frontiers. They may
record the unresolved location and reason, but must not invent an instance,
parent, hierarchy, handler, or executable path.

Descriptor identity is declaration-site identity, not runtime-object identity.
When one constructor-backed descriptor is instantiated at several callsites,
each directly observed typed call may remain an exact local Binding or
Activation fact. Those facts must not be correlated across callsites or
composed into a full command path: the descriptor publishes once with an
explicit instance-correlation frontier, and no arbitrary runtime instance is
selected.

## First adapter: typed Cobra commands

- Reuse the existing build-selected `go/packages` and typed syntax information.
  Do not launch a second package loader and do not scan ignored or
  build-unselected source.
- Recognize only the canonical `github.com/spf13/cobra.Command` type and exact
  Cobra methods. A lookalike `Command`, `AddCommand`, or `Execute` name is not
  evidence.
- Publish every typed Cobra command descriptor with the exact declaration,
  constant `Use` value when available, constructor context, and named or
  literal `Run`/`RunE` handler location when available.
- Emit a Binding for direct, unambiguous `AddCommand` relationships. A unique
  direct constructor or variable binding may be resolved; arbitrary alias,
  instance-flow, reassignment, slice-mutation, or call-order interpretation is
  out of scope.
- Emit an Activation for a direct, unambiguous `Execute` or `ExecuteContext`
  receiver attributable to a build-selected executable startup site.
- Derive a full command path only from a unique chain of exact Binding facts.
  Otherwise keep the known command segment and an explicit partial frontier.
- Keep legacy command facts for old-artifact and no-typed-data compatibility,
  while preferring a matching typed descriptor without duplicating the product
  card.

Cobra-specific recognition stays inside the adapter. Neutral surface records,
report projection, diagnostics, and later path composition must not depend on
Cobra field or method names.

## Boundedness and observability

- Sort neutral facts by stable repository identity before deterministic
  truncation.
- Apply explicit adapter input and output limits without allocating from
  untrusted or repository-sized collection lengths.
- Record counts for descriptors, exact bindings, exact activations, partial
  frontiers, duplicates, and reached limits.
- A partial relationship may reduce structural certainty, but must not silently
  delete an otherwise valid descriptor.
- Descriptor, relationship, handler, executable, counter, frontier, and
  truncation authority is local and provider-free. A later model-assisted
  presentation pass may only name, describe, order, or group supplied opaque
  fact IDs; it may not create, remove, or strengthen inventory facts.

## Acceptance

- a typed Cobra descriptor is published even when no route from `main` is
  proven;
- direct `AddCommand` and `Execute` relationships enrich the matching
  descriptor with exact Binding and Activation facts;
- global wrappers, multiple instances, reassignment, variadic registration,
  and dynamic assembly preserve descriptors while leaving ambiguous edges
  partial rather than guessed;
- an unknown ancestor never produces an exact full command path;
- fake local lookalikes are excluded;
- named and literal handlers retain exact locations;
- typed descriptors win over equivalent legacy records without duplicate
  cards;
- representative etcd commands form a useful inventory even when only a subset
  has a complete exact rooted hierarchy;
- focused tests, `./scripts/check.sh`,
  `./scripts/etcd_check.sh /Users/dvordrova/git/etcd`, and
  `git diff --check` pass.

## Explicit non-goals

This decision does not authorize a deep alias or instance interpreter, complete
framework tree reconstruction, SSA call-order simulation, client or outbound
integration discovery, lifecycle pairing, ordinary Mechanism composition, a
provider prompt, Search, source snapshots, new HTTP routes, or a broad UI
redesign.

Outbound clients and integrations are a later adapter slice. They should map
requests, responses, configuration, and call sites into the same neutral
Descriptor/Binding/Activation vocabulary rather than reusing Cobra-specific
logic or creating another product model.
