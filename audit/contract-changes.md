# Minimal contract changes

These are contract deltas implied by the red tests. They are not an
implementation authorization and do not require preserving the current
unreleased, semantically incorrect FlowProof JSON shape.

## C-001 — semantic enums are closed and uncertainty is structural

Findings: CORE-004, PY-001

Current contract:

- `RelationKind`, `ResolutionKind`, and `InvocationMode` are string aliases.
- `Graph.Validate` accepts unknown values.
- Dynamic Python dispatch is a plain static call plus warning prose.

Required contract:

- Add `ResolutionDynamicUnknown` (`dynamic_unknown`).
- Validate entity kind, relation kind, resolution, invocation, and scope.
- A dynamic boundary may preserve the syntactic callee (`getattr`) while leaving
  the runtime target unknown.
- Unknown or dynamic resolution cannot satisfy a target-dependent slot.
- Warnings supplement typed semantics; they never carry the only copy of them.

Acceptance tests:

- `TestPythonDynamicDispatchRemainsDynamicUnknown`
- `TestEvidenceGraphRejectsUnknownSemanticEnums`

## C-002 — callsite, binding, and target declaration are independent

Findings: FP-010, PY-002

Current contract:

- `CommandTraceStep` has one path/line.
- FlowProof often uses a target declaration as transition evidence.

Required contract:

- `Transition.Evidence` has the unambiguous meaning `callsite_location` (or the
  field is renamed accordingly).
- A resolved target anchor carries `target_location` independently.
- Framework registration may additionally retain a binding location when it is
  distinct from both.
- Declaration existence does not verify dispatch.

Acceptance tests:

- `TestDispatchStoresCallsiteAndTargetLocationSeparately`
- `TestRegistrationKeepsCallsiteAndTargetLocations`

The existing transition/anchor pair can express the first two locations; a new
generic location abstraction is not required unless the binding test proves it.

## C-003 — invocation collection stops at control boundaries

Findings: FP-001, FP-013

Current contract:

- Handler AST traversal descends into nested function literals.
- Ordinary calls default to synchronous invocation, including deferred calls.

Required contract:

- Handler synchronous calls exclude nested callback/task bodies.
- Callback/task body facts are owned by their callback/task.
- Deferred calls retain `InvocationDeferred` and branch ownership.
- One source expression cannot be emitted as both synchronous handler work and
  asynchronous task work.

Acceptance tests:

- `TestBackupScanHasNoDuplicateSynchronousEdge`
- `TestDeferredCleanupIsNotSynchronous`

## C-004 — async lifecycle is generic but mechanism is preserved

Findings: FP-002, FP-003, FP-004, FP-005, CORE-003

Current contract:

- Shared enums say `starts_goroutine` and `goroutine`.
- Lifecycle edges connect callback, cancel, Wait, and last return without task or
  group identity.

Required contract:

- Core lifecycle verbs are task start/registration, task body, cancellation,
  join, and completion.
- Adapter facts retain mechanism (`go_goroutine`, `errgroup`, `asyncio_task`,
  thread, process) as typed detail or provenance.
- An async task has identity and an owning group/scope.
- Cancellation targets a cancellation scope or task set.
- Join is an operation whose targets are tasks owned by the joined group.
- Cancel and Wait remain sibling operations.
- Source order is not represented as calls, cancellation, joins, or returns.
- Add a minimal `precedes` relation only if a concrete consumer/test requires
  same-branch order.

Acceptance tests:

- `TestBackupContainsSeparateMainAndScannerBranches`
- `TestBackupCancelDoesNotJoinWait`
- `TestBackupWaitJoinsScannerTask`
- `TestSourceOrderIsNotRepresentedAsCallEdge`
- `TestGoGoroutineAndPythonTaskShareAsyncLifecycleWithoutLosingMechanism`

No general happens-before or CFG solver is part of this contract.

## C-005 — proof slots support honest non-applicability

Findings: PY-004, FP-008

Current contract:

- CLI always has eight slots.
- `SlotStatus` is missing/partial/verified/unresolved.
- Complete means every slot is verified.

Required contract:

- Add `SlotNotApplicable`.
- A slot may be not applicable only within an explicit flow/archetype scope and
  with a bounded reason.
- `verified` and justified `not_applicable` are satisfied.
- `missing`, `partial`, and `unresolved` are not satisfied.
- Completion reports satisfied/total applicable slots rather than preserving an
  8/8 verified presentation.
- Archetypes choose applicable slots; they need not inherit all CLI slots.

Acceptance tests:

- `TestInitConcurrencyNotApplicable`
- `TestSynchronousPythonFlowConcurrencyNotApplicable`
- `TestNotApplicableSatisfiesProofMissingDoesNot`

## C-006 — target resolution and architectural classification are separate

Findings: FP-006, FP-008

Current contract:

- Any partial non-concurrency slot becomes verified when selected targets resolve.
- Name scores seed both discovery and architectural role claims.

Required contract:

- Resolution changes only target identity, resolution kind, certainty, and
  provenance.
- Each slot has an explicit satisfaction predicate owned by core.
- `core_operation` requires an explicit domain-role witness or remains partial.
- `io_boundary` distinguishes internal facade/persistence boundary from external
  resource access; reaching an internal method is insufficient.
- Status, summary, missing criteria, and evidence IDs must be mutually
  consistent.
- Complete is impossible while any applicable criterion is unmet.

Acceptance tests:

- `TestInternalRepositoryCallDoesNotVerifyExternalIOBoundary`
- `TestPartialTargetCannotProduceVerifiedOrStopComplete`
- `TestResolvedTargetDoesNotVerifyArchitecturalSlot`

It is acceptable for current restic output to remain partial.

## C-007 — completion claims are scoped

Findings: FP-005, FP-007

Current contract:

- One termination slot is verified from Wait to the lexically last handler return.

Required contract:

- `handler_completion` retains selected reachable handler outcomes.
- `process_completion` requires the selected dispatch return and process exit
  chain.
- A proof states which completion scope it covers.
- A return source location proves a return statement, not process termination.

Acceptance test:

- `TestRunBackupReturnIsHandlerCompletionNotProcessCompletion`

## C-008 — adapters emit facts; shared core owns work and verdicts

Findings: CORE-001, CORE-002

Current contract:

- Go types implements a FlowProof executor and returns `SlotUpdates`.
- FlowProof assembly imports Go collector/executor packages directly.
- Python ends at exact-symbol `evidence.Graph`.

Required contract:

- Adapter output contains normalized entities/relations/scenario/provenance and
  bounded warnings/metrics.
- Adapter output cannot set slot status, stop reason, confidence, or next task.
- Existing shared core merges evidence, plans the next task, enforces budgets,
  deduplicates/no-progress stops, and evaluates satisfaction.
- Go and Python comparative fixtures enter that same core contract.
- Framework recognizers produce registration/dispatch facts and do not schedule
  follow-up work.

Acceptance tests:

- `TestAdapterFactsCannotSetFinalSlotStatusOrConfidence`
- `TestAdapterCannotEnqueuePlannerTasks`
- `TestRepeatedUnresolvedTasksStopInSharedCore`
- `TestGoAndPythonFixturesUseSamePlannerBudgetStoreAndCompletionEvaluator`

The smallest change may retain the current `Session`, `Task`, anchors, and
transitions while removing adapter-owned `SlotUpdates`; no new plugin framework
is implied.

## C-009 — namespace and scenario identity do not assume Go

Findings: CORE-003, PY-003

Current contract:

- `EntityModule` exists, but FlowProof seed identity requires a Go import path.
- Shared `BuildContext` contains only Go fields.
- Pyright scenario ID is constant across interpreter/config/source-root changes.

Required contract:

- Callable identity is scoped by language, namespace/module, location, and
  scenario; Python does not require a Go package field.
- Scenario identity changes when semantically relevant toolchain, configuration,
  build target, or source-root inputs change.
- Scenario details remain typed, bounded, deterministic, and secret-safe.
- Go build inputs and Python configuration inputs remain adapter facts rather
  than a shared AST/config payload.

Acceptance tests:

- `TestPythonModuleIdentityDoesNotRequireGoPackage`
- `TestPythonScenarioChangesWithInterpreterSourceRootsAndConfig`

The test should drive the smallest typed representation; do not add arbitrary
`map[string]any` extensions.

## C-010 — downstream DTOs preserve evidence semantics

Findings: CORE-005, PY-001, PY-006

Current contract:

- `symbol.CallFact` drops resolution/invocation.
- Warning filtering recognizes only gopls message text.
- Scenario projection keeps only Go build context.
- Focused test/freshness policy assumes Go.

Required contract:

- Call facts retain resolution and invocation.
- Warnings have stable typed codes plus safe prose.
- Language and scenario identity survive model-facing compaction without
  sensitive paths.
- Focused investigation asks the adapter/language facts for test/config
  conventions; it does not infer Python from `_test.go` rules.

Acceptance tests:

- `TestSymbolBundlePreservesDynamicResolutionInvocationAndLanguage`
- `TestPythonEvidenceCanEnterFocusedInvestigationWithoutGoTestAssumptions`

## C-011 — component promotion requires endpoint witnesses

Findings: FP-011

Current contract:

- A package edge is projected to every pair of components containing its
  endpoint packages.

Required contract:

- Package imports remain package facts.
- Component promotion requires unique ownership or explicit source and target
  component anchors.
- Ambiguous overlap emits no component arrow.
- Evidence records the endpoint witness as well as the package edge.

Acceptance test:

- `TestPackageImportRequiresComponentSpecificWitness`

## C-012 — unknowns are reconciled after local proof

Findings: FP-012

Current contract:

- Local proof paths become openable while old unverified paths remain unchanged.

Required contract:

- Exact locally resolved files are removed or explicitly reclassified from the
  unknown frontier before confidence calculation and report projection.
- Unrelated, unresolved, and fabricated paths remain unknown/diagnostic.
- Directory-level reconciliation requires an explicit rule; substring matching
  is forbidden.

Acceptance test:

- `TestLocallyResolvedFilesAreRemovedFromOverviewUnknowns`

## C-013 — presentation cannot infer runtime sequence

Findings: PRES-001, PRES-002

Current contract:

- Report JavaScript concatenates relation groups into a guided path.
- reportserver also invokes analysis collectors under a presentation-named
  package.

Required contract:

- Rendering consumes saved state and preserves branches, relation kinds,
  certainty, and unknown order.
- Runtime ordering requires explicit static ordering evidence or runtime
  observation.
- Interactive analysis goes through an explicit action/application seam; the
  renderer remains pure.

Acceptance tests:

- `TestReportDoesNotRenderStaticFactsAsRuntimeSequence`
- `TestReportRendererIsPureAndAnalysisEndpointUsesApplicationService`

No browser redesign is required.

## C-014 — bounded output is not reported as bounded work

Findings: BOUND-001

Current contract:

- Pyright result counts and per-request timeouts are bounded.
- Workspace enumeration cost is invisible.

Required contract:

- Record barrier duration and declared workspace scope in analyzer metrics.
- Keep the existing timeout as the hard guard.
- Do not claim that output limits cap files indexed or bytes read.

Acceptance test:

- `TestPythonWorkspaceIndexWorkIsVisibleToBudget`
