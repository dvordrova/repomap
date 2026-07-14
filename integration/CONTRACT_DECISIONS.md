# Integration contract decisions

## 1. Slot applicability and satisfaction

`SlotStatus` gains `not_applicable`. Applicability is decided from the proof
archetype and bounded flow facts, never from “the collector found nothing”. A
non-applicable slot carries a machine-readable reason in the existing slot
record.

A proof is satisfied only when every slot is either:

- `verified` by slot-specific evidence rules; or
- `not_applicable` by explicit archetype/scope policy.

`missing`, `partial`, `unresolved`, and future `dynamic_unknown` states are
successful honest outcomes but are not complete. Adapters may emit facts and
warnings; they do not set final slot status, confidence or stop reason.

## 2. Resolution and architectural roles

Resolution proves target identity and provenance only. It does not prove that a
call is a core operation, external I/O boundary, persistence boundary, shutdown
boundary or process completion. Generic “all transition targets resolved” slot
promotion is removed. Each slot’s evaluator names its required evidence.

For this integration, the I/O slot means an external resource or persistence
boundary. `openWithAppendLock` and `Repository.LoadIndex` remain internal
repository calls until a bounded witness crosses that boundary.

## 3. Relations and source locations

The relation vocabulary distinguishes:

- synchronous call;
- callback registration/body;
- asynchronous task start;
- task body;
- cancellation target;
- join target;
- handler return.

Lexical source order is not a call, join or return edge. No general
happens-before solver is introduced.

`Transition.Evidence` is the callsite location. The destination anchor’s
`Location` is the resolved target declaration. Command-trace DTOs expose those
as `callsite_location` and `target_location`; they are never populated from the
same declaration merely to satisfy the schema. A separate binding abstraction
is not introduced unless the focused fixtures prove these two locations are
insufficient.

## 4. Concrete restic task identity

The bounded Go lifecycle must give the scanner callback a stable task identity
owned by the selected `errgroup`. The main handler and scanner task are separate
branches. `cancel()` targets the derived context; `wg.Wait()` joins the scanner
task. They are sibling handler operations. The task body is `sc.Scan`; it is not
also an immediate `runBackup` call.

This decision is deliberately proven for the restic-shaped `errgroup.Go`
fixture. It is not a generic concurrency framework.

## 5. Completion scope

An explicit return from `runBackup` is handler completion. It is not CLI process
termination. Process completion requires evidence through Cobra execution,
`main`, exit-code mapping and `Exit`, which is deferred. Report text and
confidence gates must preserve that scope.

## 6. Shared adapter boundary

The minimum shared boundary is normalized facts plus core policy:

- entities/anchors and exact locations;
- relations with resolution, invocation, certainty and provenance;
- scenarios;
- warnings;
- applicability and slot satisfaction evaluated by the core.

Go collectors may keep Go-specific mechanisms (`errgroup`, build scenario) in
provenance. Python may keep interpreter/project configuration in its scenario.
The shared contract does not require one AST, one LSP or production FlowProof
parity.

## 7. Python uncertainty and scenario identity

Dynamic Python dispatch is structural `dynamic_unknown`, not a static call plus
an easily dropped prose warning. The Pyright scenario identity includes the
inputs that affect analysis: interpreter/tool version, project configuration,
workspace/source roots and relevant execution mode. Secrets and absolute paths
do not enter public IDs.

The accepted Python path remains experimental: one deterministic fixture must
survive the shared DTOs with language, resolution, invocation, warning code and
scenario identity intact. Synchronous Python concurrency is
`not_applicable`.

## 8. Versioning and replay artifacts

FlowProof and evidence semantics are unreleased but persisted in generated
report fixtures. Their versions will be bumped when the JSON meaning changes,
and committed replay/golden fixtures will be migrated in the same increment.
Backward compatibility code is not required. Existing artifacts must fail
clearly on an unsupported version rather than being silently reinterpreted.

## 9. Projection rules

Component relations require an endpoint-specific witness for both selected
components; a package import cannot be multiplied across every component that
overlaps that package. Orientation unknowns are a projection of current state:
files resolved by local proof are removed without deleting unrelated unknowns.

## 10. Test and commit policy

Tests live with the corresponding change. We test each accepted contract change
and each case where the product previously made a confident false claim. We do
not create an audit-sized red-test commit or implement every proposed test.
