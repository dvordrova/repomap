# Decision 189: Repository Atlas canonical core and exact Go adapter

## Status

Approved by the repository owner through the active supervisory instruction as
the immediate provider-free reliability checkpoint after Decision 188.
Decision 188 and earlier historical checkpoints are not rewritten.

## Problem

Repository-local Go facts and runtime-surface discovery already own exact
module, package, build-selected process-entry, source-location, and provenance
facts. Repository Atlas had no small language-neutral canonical contract into
which those facts could be projected without reusing analyzer-specific entity
graphs, presentation DTOs, or model-authored membership.

Without that contract, a future Overview could accidentally promote files and
symbols into top-level architecture entities, collapse module ownership, parse
provider prose, or imply runtime reachability that local facts do not prove.

## Decision

Add a versioned language-neutral Repository Atlas core with one closed Unit
topology and one closed entity vocabulary. Units are repository, module,
service, app, and package. The neutral schema permits app and service under a
repository or module, while package remains module-owned in v1. In this first
adapter, locally authoritative Go facts produce repository to module topology,
with app and package as sibling children of their owning module. The adapter
does not classify a service.

Atlas entities are only Surface, Operation, Boundary, Resource, and Contract.
Files and symbols remain exact Evidence locators and are never Atlas entities.
Observation binds one typed entity reference to evidence in an explicit Unit
scope. Relation binds exact typed source and target entity references to
evidence, one closed Phase, one closed Authority, and an explicit Unit scope.
An Observation or Relation may reference its scope Unit or descendants; a
reference outside that scope fails validation. This permits truthful future
module or repository integrations without allowing unrelated cross-unit refs.

The first relation vocabulary contains only `exposes`. Phase is closed to
runtime, startup, shutdown, scheduled, build, generation, migration, deploy,
test, and development. Authority is closed to observed, resolved, inferred,
partial, conflicted, and unknown. Canonical ordering operates on a deep copy,
and canonical JSON is deterministic without mutating caller-owned slices or
locations.

Add one pure Go adapter seam over existing `gofacts.Facts` and
`surfacediscovery.TriggerCatalog`. It reads no files and runs no analyzer or
provider. Module IDs and ownership come from Go facts. An app Unit requires an
exact versioned build-selected `go_main_function` anchor matched to its exact
package and module. A process slice additionally requires exactly matching
static/exact `process_entry` surface identity, declaration evidence, and
`gofacts` provenance already produced by surface discovery.

That proved input yields one app-scoped Surface, one Operation, two
Observations, their exact evidence locator, and one
`exposes/startup/resolved` Relation. The adapter claims neither runtime
execution nor downstream reachability. Ambiguous or incomplete entrypoint or
surface facts yield absence, not repair or inference. It emits no Boundary,
Resource, Contract, or service without typed proof and contains no framework
branch, prose parser, lexical classifier, LocalProof promotion, or provider
fallback.

The contract and adapter are not wired into report, UI, persistence, manifests,
providers, caches, Study, Architecture, localization, flags, or existing
canonical artifacts in Decision 189. Integration, Flow, Mechanism, Component,
Architecture, and Study remain derived views outside this canonical slice.

## Proof

Provider-free focused tests establish that:

- shuffled authoritative inputs produce identical deterministic IDs, order,
  and canonical JSON without mutating callers;
- two independent fixtures produce the same canonical contract shape;
- mixed multi-module facts retain exact module ownership, keep app and package
  as siblings, and separate their process slices;
- exact process-entry files and symbols occur only in Evidence locators;
- incomplete typed evidence emits no process entities or relation;
- wrong-kind and dangling entity refs fail closed;
- relation and observation refs outside their declared Unit scope fail, while
  a module-scoped relation across descendant Units validates;
- every closed Phase and Authority validates and unknown values fail; and
- the full repository and nearby etcd provider-free checks pass without
  changing existing report or Study integration.
