# Required failing regression tests

These are behavioral contracts to add **red before fixes**. They intentionally
avoid rendered-report goldens, live repositories, model calls, network access,
and private helper snapshots. Tiny source fixtures may be deleted or rewritten
with the corresponding adapter.

## Restic-shaped control flow

### `TestBackupScanHasNoDuplicateSynchronousEdge`

Findings: FP-001

Assertions:

- `runBackup -> wg.Go` exists once as synchronous registration/start evidence.
- `scanner task -> Scanner.Scan` exists once as the task body.
- No synchronous `runBackup -> Scanner.Scan` transition exists.

Current failure: handler AST traversal enters the nested function literal and
emits Scan both as a handler call and callback body.

### `TestBackupContainsSeparateMainAndScannerBranches`

Findings: FP-002

Assertions:

- The main branch contains `arch.Snapshot`.
- The optional task branch contains `Scanner.Scan`.
- The two branches may overlap and are not flattened into one sequence.
- The `NoScan` branch has no scanner task.

Current failure: the current proof has no branch/task-owner contract and the
report builds one guided list.

### `TestBackupCancelDoesNotJoinWait`

Findings: FP-003

Assertions:

- No `calls` or `joins` relation runs from `cancel` to `Wait`.
- Both operations are owned by `runBackup`'s main branch.
- Cancellation targets `cancelCtx` or its scoped task set.
- Lexical order, if retained, uses a non-call relation.

Current failure: lifecycle extraction emits `cancel -> Wait` as `joins`.

### `TestBackupWaitJoinsScannerTask`

Findings: FP-004

Assertions:

- `Wait` is the join operation.
- The joined target is the scanner task registered on the same errgroup.
- `NoScan` yields an empty outer joined task set.
- The outer Wait does not claim ownership of Snapshot's internal errgroup.

Current failure: no task/group identity is stored; the join edge ends at the
Wait method and begins at cancel.

### `TestSourceOrderIsNotRepresentedAsCallEdge`

Findings: FP-005

Assertions:

- `Snapshot`, `cancel`, `Wait`, and returns are not connected by fabricated
  `calls`, `joins`, or `returns` ownership edges merely due to source order.
- Same-branch order may be represented only as `precedes`/`happens_before`.
- Snapshot-error, partial-source, and scanner-error outcomes remain distinct.

Current failure: lifecycle extraction turns source order into join/return edges
and retains only the lexically last return.

### `TestDeferredCleanupIsNotSynchronous`

Findings: FP-013

Assertions:

- `localVss.DeleteSnapshots` has deferred invocation.
- It belongs to the branch that constructed `localVss`.
- It is not rendered as an immediate synchronous step.

Current failure: ordinary calls default to synchronous invocation.

### `TestInitResolvesGlobalCreateRepository`

Findings: FP-009

Assertions:

- `runInit` contains one callsite at `cmd/restic/cmd_init.go:84`.
- Its target is independently resolved to
  `internal/global/global.go:463`.
- Retention does not depend on the callee containing `Load`, `Open`, `Save`, or
  `New`.

Current failure: name scoring drops `global.CreateRepository` before proof
construction.

### `TestDispatchStoresCallsiteAndTargetLocationSeparately`

Findings: FP-010

Assertions:

- Root execution stores callsite `cmd/restic/main.go:188` and target declaration
  `cmd/restic/main.go:36`.
- Backup registration stores callsite `cmd/restic/main.go:77` and target
  `cmd/restic/cmd_backup.go:35`.
- Backup handler binding/call stores `cmd/restic/cmd_backup.go:60-61` separately
  from target `cmd/restic/cmd_backup.go:498`.
- Init registration and handler locations are separated the same way.

Current failure: `CommandTraceStep` has one path/line and proof construction
reuses declaration locations as transition evidence.

### `TestRunBackupReturnIsHandlerCompletionNotProcessCompletion`

Findings: FP-007

Assertions:

- The three selected `runBackup` returns are handler outcomes.
- None independently satisfies process completion.
- Process completion requires `ExecuteContext` returning, exit-code mapping,
  and `Exit` at `cmd/restic/main.go:243`.

Current failure: one last handler return verifies the termination slot.

### `TestInternalRepositoryCallDoesNotVerifyExternalIOBoundary`

Findings: FP-006, FP-008

Assertions:

- Resolving `openWithAppendLock` and `LoadIndex` changes callable identity only.
- An internal persistence facade is classified independently of external I/O.
- The external-I/O claim remains partial without a backend read/write witness.

Current failure: name selection plus target resolution verifies the I/O slot.

### `TestPartialTargetCannotProduceVerifiedOrStopComplete`

Findings: FP-008, CORE-002

Assertions:

- Resolving a partial transition leaves unmet role criteria intact.
- A summary saying “targets still need proof” cannot coexist with `verified`.
- The session cannot report all slots satisfied or `stop=complete`.

Current failure: `refreshResolvedSlots` promotes every partial non-concurrency
slot whose selected transitions resolve.

### `TestInitConcurrencyNotApplicable`

Findings: PY-004

Assertions:

- A scoped synchronous init handler has `concurrency=not_applicable`.
- No lifecycle task is scheduled for that slot.
- `not_applicable` is satisfied; `missing` is not.

Current failure: `SlotStatus` has no `not_applicable` and the fixed CLI planner
requires concurrency.

## Shared core and Python

### `TestSynchronousPythonFlowConcurrencyNotApplicable`

Findings: PY-004

Assertions:

- A synchronous Python dispatch/direct-call/resource flow can complete with
  `concurrency=not_applicable`.
- No Go mechanism name is required to justify the status.
- Missing concurrency evidence is not manufactured as an error.

Current failure: Python cannot enter FlowProof and the only CLI contract requires
the concurrency slot.

### `TestPythonDynamicDispatchRemainsDynamicUnknown`

Findings: PY-001, CORE-005

Assertions:

- `getattr(target, method_name)(value)` retains a syntactic call to `getattr`.
- The runtime target relation has `resolution=dynamic_unknown`.
- No target method is invented or marked verified.
- The dynamic boundary survives `symbol.Build` as typed data, not only prose.

Current failure: Pyright emits a plain `calls/static` relation and the only
dynamic marker is a warning filtered out downstream.

### `TestPythonDecoratorRegistrationIsNotDirectCall`

Findings: PY-002

Assertions:

- A decorator/route/command binding is represented as registration.
- The registration site and application callable declaration are separate.
- The framework's later dispatch is not claimed to be a direct source call.

Current failure: no Python registration facts or relation path exists.

### `TestRegistrationKeepsCallsiteAndTargetLocations`

Findings: FP-010, PY-002

Assertions:

- Both Cobra and one Python registration fixture retain
  `callsite_location`/`binding_location` separately from `target_location`.
- Declaration existence alone does not verify dispatch.

Current failure: Go conflates locations; Python has no registration contract.

### `TestPythonModuleIdentityDoesNotRequireGoPackage`

Findings: CORE-001, CORE-003, PY-002

Assertions:

- A Python module/namespace can seed the same core without an import path,
  `EntrypointPackage`, GOOS, or GOARCH.
- Its entity identity remains stable under the selected scenario.

Current failure: FlowProof assembly seeds `go-default:<EntrypointPackage>` and
only consumes Go command traces.

### `TestGoGoroutineAndPythonTaskShareAsyncLifecycleWithoutLosingMechanism`

Findings: CORE-003

Assertions:

- Both mechanisms normalize to task start/body/cancel/join relations.
- The Go fact retains mechanism `goroutine/errgroup`.
- The Python fact retains mechanism `asyncio` (or the fixture's actual
  mechanism).
- The mechanisms are not collapsed into identical language facts.

Current failure: generic relation/invocation enums encode goroutine directly and
there is no Python lifecycle input.

### `TestAdapterFactsCannotSetFinalSlotStatusOrConfidence`

Findings: CORE-002, FP-008

Assertions:

- Adapter output can add evidence and uncertainty only.
- It cannot submit `SlotVerified`, `StopComplete`, or final confidence.
- Core applies slot-specific criteria after merging facts.

Current failure: gotypes returns `SlotUpdates` marking concurrency and
termination verified; core confidence later trusts mechanical completion.

### `TestRepeatedUnresolvedTasksStopInSharedCore`

Findings: CORE-001

Assertions:

- The same unresolved fact from either adapter produces the same task key.
- Repetition triggers the shared duplicate/no-progress stop.
- No adapter-specific loop or retry policy is used.

Current failure: Python cannot submit facts to the FlowProof planner, so the
cross-language shared-core contract cannot be exercised.

### `TestAdapterCannotEnqueuePlannerTasks`

Findings: CORE-002

Assertions:

- Adapter results contain facts, not requested next-task kinds.
- Adapter-provided slot updates cannot indirectly choose the next planner task.
- Core alone derives work from current evidence and goal.

Current failure: while no explicit `NextTasks` field exists, arbitrary
`SlotUpdates` alter `PlanNext` and can force or skip work indirectly.

### `TestGoAndPythonFixturesUseSamePlannerBudgetStoreAndCompletionEvaluator`

Findings: CORE-001

Assertions:

- Both normalized fixtures enter the same `Session`/planner implementation.
- Both use the same budget, evidence merge, dedup/no-progress, and completion
  evaluator.
- Only adapter fact production differs.

Current failure: Go uses `flowproof/assemble` plus gotypes; Python ends at an
exact-symbol evidence graph.

### `TestNotApplicableSatisfiesProofMissingDoesNot`

Findings: PY-004, FP-008

Assertions:

- `verified` and justified `not_applicable` are satisfied terminal statuses.
- `missing`, `partial`, `unresolved`, and `dynamic_unknown` are not.
- Complete status does not require preserving an 8/8 verified count.

Current failure: no `not_applicable` status exists and complete requires every
slot to be verified.

### `TestResolvedTargetDoesNotVerifyArchitecturalSlot`

Findings: FP-006, FP-008

Assertions:

- Resolution updates target identity/certainty only.
- `core_operation`, `io_boundary`, and completion roles require separate
  classification evidence.

Current failure: generic refresh promotes partial slots from target resolution.

### `TestPythonScenarioChangesWithInterpreterSourceRootsAndConfig`

Findings: PY-003

Assertions:

- Scenario identity changes when interpreter/toolchain, source roots, or
  pyrightconfig changes.
- Sensitive absolute paths and arbitrary environment variables are not retained.
- Identical bounded inputs produce the same identity.

Current failure: every run uses constant `pyright-workspace` with only two env
labels.

### `TestPythonComparativeFixtureProvidesRequiredRoles`

Findings: PY-002, PY-005

Assertions:

- The fixture includes a package/console entrypoint, registration, dispatched
  callable, direct call, unresolved dynamic call, external resource boundary,
  and synchronous completion.
- Analyzer facts refer to the selected source function only.
- Optional async facts, if present, form a separate branch.

Current failure: the current fixture has calls/references only and its fake test
injects `getattr` while selecting a different function.

### `TestPythonEvidenceCanEnterFocusedInvestigationWithoutGoTestAssumptions`

Findings: PY-006

Assertions:

- A Python exact-symbol fact can enter the existing focused investigation state.
- Freshness identity uses Python scenario/tool inputs rather than requiring a Go
  build context.
- Python test references are selected from adapter-provided conventions rather
  than `_test.go`.
- The shared reducer remains unchanged.

Current failure: reportserver investigation captures Go fact context and filters
public test references exclusively by the Go `_test.go` suffix.

### `TestEvidenceGraphRejectsUnknownSemanticEnums`

Findings: CORE-004

Assertions:

- Unknown entity, relation, resolution, invocation, and scope enum values fail
  validation.
- Repository-scoped source entities require usable repository-relative
  locations.
- External entities do not require repository locations.

Current failure: validation accepts arbitrary semantic strings except certainty.

### `TestSymbolBundlePreservesDynamicResolutionInvocationAndLanguage`

Findings: CORE-005, PY-001

Assertions:

- `CallFact` retains resolution and invocation.
- Typed analyzer warnings survive without provider-specific text allowlists.
- Python language/scenario identity is not replaced by empty Go build fields.

Current failure: the bundle drops resolution/invocation and accepts only gopls
warning text.

### `TestPythonWorkspaceIndexWorkIsVisibleToBudget`

Findings: BOUND-001

Assertions:

- Workspace barrier duration and scope are visible in analyzer metrics.
- Existing request timeout bounds the operation.
- Output count limits are not reported as total work limits.

Current failure: output is bounded but workspace enumeration cost is implicit.

## Downstream projection and presentation

### `TestLocallyResolvedFilesAreRemovedFromOverviewUnknowns`

Findings: FP-012

Assertions:

- An exact locally resolved file is removed or explicitly reclassified from
  overview unknowns.
- Unrelated and unresolved paths remain unchanged.
- Model-fabricated paths are never promoted.

Current failure: report parsing copies all prior unverified paths unchanged.

### `TestPackageImportRequiresComponentSpecificWitness`

Findings: FP-011

Assertions:

- One `P -> Q` package import is not projected to every component pair that
  overlaps P and Q.
- Promotion requires unique ownership or explicit endpoint witnesses.
- Ambiguity retains a package fact and emits no invented component arrow.

Current failure: nested loops produce the Cartesian component relation set.

### `TestReportDoesNotRenderStaticFactsAsRuntimeSequence`

Findings: PRES-001, DOC-007

Assertions:

- Static sibling facts are not placed into a runtime-ordered list.
- Main/task branches remain visibly distinct.
- Unknown order is labeled rather than inferred.
- Runtime order requires observed or explicitly supported ordering evidence.

Current failure: JavaScript concatenates transition groups into “Guided symbol
path”.

### `TestReportRendererIsPureAndAnalysisEndpointUsesApplicationService`

Findings: PRES-002

Assertions:

- Rendering consumes saved application state only.
- Analyzer invocation belongs to an explicit action/application seam.
- The test does not prescribe DI or package count.

Current failure: reportserver combines HTTP presentation, action orchestration,
and collector invocation under one undocumented boundary.

## Documentation consistency checks

These are acceptance checks, not brittle prose snapshots. They should fail on
the current documents and become true only after behavioral corrections land.

| Check name | Assertion | Current failure |
| --- | --- | --- |
| `DocumentationAudit_COREIDEAReflectsVerifiedContracts` | CORE_IDEA distinguishes current exact-symbol graph sharing from a future shared planner; documents `not_applicable` and honest partial completion | It claims all adapters implement Provider/the same graph path, fixed eight-slot completion, and no long-lived LSP |
| `DocumentationAudit_MilestoneDoesNotClaimInvalidResticCompletion` | MILESTONES does not call the false cancel/Wait chain or mechanical 8/8 semantically complete | Lines 399-408 do both |
| `DocumentationAudit_NextSessionNamesCurrentAuditGate` | NEXT_SESSION identifies the accepted semantic audit/correction as the active gate and reflects current Go/Python slices | It is dated 2026-07-10 and names already-implemented exact-symbol wiring as next blocker |
| `DocumentationAudit_SystemMapMatchesImplementationMap` | SYSTEM_MAP lists Pyright, gotypes/FlowProof policy coupling, and reportserver orchestration honestly | It says only gopls exists and all adapters/presentation respect desired boundaries |
| `DocumentationAudit_TechnicalDebtIncludesAcceptedSemanticFindings` | TECHNICAL_DEBT records accepted unresolved semantic issues after tests define them | It records FlowProof cost only |
| `DocumentationAudit_PythonClaimsMatchComparativeFixture` | PYTHON.md states exactly which graph facts and shared-core stages the real fixture reaches | It overstates scenario/dynamic/shared-core guarantees in earlier sections |
| `DocumentationAudit_StaticEvidenceIsNotPresentedAsRuntimeOrder` | DEEPER_RESEARCH's correct evidence rule matches report behavior | The document rejects runtime ordering while current JS creates a guided sequence |
