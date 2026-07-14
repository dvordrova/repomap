# Minimal correction plan

Findings remain atomic and independently falsifiable. Implementation work is
grouped by `scope` and contract so it can be reviewed in small changes rather
than one audit-shaped PR.

In particular, FP-003 and PY-004 are deliberately separate findings but belong
to the same first shared-core work: make relation/slot semantics honest before
adding more analysis.

No implementation begins until the corresponding tests are red for the stated
reason.

## W-00 — accept the audit boundary

Findings: all

Output:

- accepted finding IDs;
- accepted test contracts;
- explicit decision that preserving current restic 8/8 is not a goal.

Gate:

- every proposed edit maps to a failing test in `failing-tests.md`;
- no report, milestone, or current status is used as correctness evidence.

No code or documentation repair belongs in this work.

## W-01 — honest shared relation and slot semantics

Findings:

- FP-003
- FP-005
- FP-008
- PY-004
- CORE-004

Smallest changes:

- add and validate `not_applicable`;
- count `verified` and justified `not_applicable` as satisfied;
- keep missing/partial/unresolved unsatisfied;
- remove generic partial-to-verified promotion;
- reject unknown semantic enum values;
- remove `cancel -> Wait` join and `Wait -> return` source-order edges;
- add `precedes` only if the source-order regression proves a consumer needs it.

Red gates:

- `TestBackupCancelDoesNotJoinWait`
- `TestSourceOrderIsNotRepresentedAsCallEdge`
- `TestPartialTargetCannotProduceVerifiedOrStopComplete`
- `TestInitConcurrencyNotApplicable`
- `TestSynchronousPythonFlowConcurrencyNotApplicable`
- `TestNotApplicableSatisfiesProofMissingDoesNot`
- `TestEvidenceGraphRejectsUnknownSemanticEnums`

Likely edit surface:

- `internal/evidence/evidence.go`
- `internal/flowproof/model.go`
- `internal/flowproof/worklist.go`
- the minimal lifecycle emission lines proven wrong by the tests

Do not add a general happens-before solver, new planner, or schema registry.

## W-02 — correct bounded Go command facts

Findings:

- FP-001
- FP-009
- FP-010
- FP-013

Smallest changes:

- stop handler traversal at nested function literals;
- retain callback body under its callback owner;
- preserve deferred invocation;
- retain bounded top-level handler calls before role ranking so
  `CreateRepository` cannot disappear;
- store dispatch call/binding locations separately from target declarations.

Red gates:

- `TestBackupScanHasNoDuplicateSynchronousEdge`
- `TestDeferredCleanupIsNotSynchronous`
- `TestInitResolvesGlobalCreateRepository`
- `TestDispatchStoresCallsiteAndTargetLocationSeparately`

Likely edit surface:

- `internal/gofacts/commandtrace.go`
- `internal/flowproof/cli.go`

Do not introduce a second framework registry, SSA, VTA, or repository-wide call
graph.

## W-03 — represent one real async task lifecycle

Findings:

- FP-002
- FP-004
- FP-007
- CORE-003

Smallest changes:

- add one scanner-task identity owned by the outer errgroup;
- preserve main and optional scanner branches;
- make Scan the task body only;
- bind cancellation to `cancelCtx`/task scope;
- make Wait join the outer group's scanner task set;
- retain all selected handler outcomes;
- distinguish handler completion from process completion;
- normalize the lifecycle verbs while preserving Go mechanism detail.

Red gates:

- `TestBackupContainsSeparateMainAndScannerBranches`
- `TestBackupWaitJoinsScannerTask`
- `TestRunBackupReturnIsHandlerCompletionNotProcessCompletion`
- `TestGoGoroutineAndPythonTaskShareAsyncLifecycleWithoutLosingMechanism`

Likely edit surface:

- `internal/analyzer/golang/gotypes/lifecycle.go`
- minimal shared evidence fields/enums proven necessary by the cross-language
  lifecycle fixture

Do not analyze arbitrary goroutines, nested runtime groups, or full CFGs. The
NoScan/one-errgroup fixture is the boundary.

## W-04 — move verdict ownership into the existing core

Findings:

- CORE-001
- CORE-002

Smallest changes:

- stop accepting adapter-owned final `SlotUpdates`;
- let adapters return anchors/transitions/scenario/provenance only;
- evaluate slot satisfaction after evidence merge in `internal/flowproof`;
- ensure an adapter cannot influence the next task except through normalized
  facts;
- route one Go and one Python normalized fixture through the existing Session,
  budget, evidence merge, dedup/no-progress, and completion evaluator.

Red gates:

- `TestAdapterFactsCannotSetFinalSlotStatusOrConfidence`
- `TestAdapterCannotEnqueuePlannerTasks`
- `TestRepeatedUnresolvedTasksStopInSharedCore`
- `TestGoAndPythonFixturesUseSamePlannerBudgetStoreAndCompletionEvaluator`

Likely edit surface:

- `internal/flowproof/worklist.go`
- `internal/flowproof/assemble`
- adapter result construction

Prefer tightening `Result` and reusing `Session`. Do not create a plugin
registry, DI layer, generic policy engine, or separate Python proof engine.

## W-05 — preserve Python uncertainty and scenario identity

Findings:

- PY-001
- PY-003
- CORE-005

Smallest changes:

- encode `dynamic_unknown` structurally for dynamic runtime targets;
- carry resolution/invocation and typed warning codes through `symbol.Bundle`;
- make Pyright scenario identity depend on bounded interpreter/toolchain,
  configuration, and source-root inputs;
- retain language identity instead of an empty Go build context.

Red gates:

- `TestPythonDynamicDispatchRemainsDynamicUnknown`
- `TestPythonScenarioChangesWithInterpreterSourceRootsAndConfig`
- `TestSymbolBundlePreservesDynamicResolutionInvocationAndLanguage`

Likely edit surface:

- `internal/analyzer/python/pyright`
- `internal/evidence`
- `internal/symbol`

Do not resolve dynamic calls, persist arbitrary environment variables, or add
`map[string]any` language extensions.

## W-06 — add the smallest honest Python proof seed

Findings:

- PY-002
- PY-005
- PY-006

Smallest changes:

- replace the current comparison fixture with a source-aligned bounded fixture;
- detect only the fixture's demonstrated package/console entrypoint;
- emit one registration/binding, dispatched callable, direct call, dynamic
  unknown, resource boundary, and synchronous completion fact;
- feed those facts into the same core from W-04;
- make focused test/config policy consume typed language facts rather than Go
  filename rules.

Red gates:

- `TestPythonComparativeFixtureProvidesRequiredRoles`
- `TestPythonDecoratorRegistrationIsNotDirectCall`
- `TestRegistrationKeepsCallsiteAndTargetLocations`
- `TestPythonModuleIdentityDoesNotRequireGoPackage`
- `TestPythonEvidenceCanEnterFocusedInvestigationWithoutGoTestAssumptions`

Likely edit surface:

- one small Python syntax/manifest fact producer;
- the existing fixture;
- the existing shared-core seed adapter.

Choose one concrete registration mechanism for the fixture. Do not generalize
to Django/FastAPI/Celery/Poetry/uv or invent a public recognizer registry.

## W-07 — make role verification conservative

Findings:

- FP-006
- FP-008

Smallest changes:

- keep exact call resolution separate from role classification;
- leave internal repository/facade evidence partial for external I/O;
- add a slot-specific satisfaction predicate only when the fixture provides the
  required witness;
- allow honest partial output.

Red gates:

- `TestInternalRepositoryCallDoesNotVerifyExternalIOBoundary`
- `TestResolvedTargetDoesNotVerifyArchitecturalSlot`
- `TestPartialTargetCannotProduceVerifiedOrStopComplete`

Likely edit surface:

- `internal/flowproof` slot evaluation
- no collector expansion unless the test requires a concrete witness

Do not chase the current output back to 8/8.

## W-08 — repair pure downstream projections

Findings:

- FP-011
- FP-012
- PRES-001
- PRES-002

Smallest changes:

- require component-specific endpoints before package-to-component promotion;
- reconcile exact locally resolved paths with overview unknowns before
  confidence/report projection;
- render branches/relations without constructing runtime order;
- keep rendering pure and make the existing analysis-action seam explicit.

Red gates:

- `TestPackageImportRequiresComponentSpecificWitness`
- `TestLocallyResolvedFilesAreRemovedFromOverviewUnknowns`
- `TestReportDoesNotRenderStaticFactsAsRuntimeSequence`
- `TestReportRendererIsPureAndAnalysisEndpointUsesApplicationService`

Likely edit surface:

- `internal/report/components.go`
- `internal/orient`
- `internal/report/templates/script.js`
- a minimal reportserver boundary only if the behavior test requires it

Do not redesign the browser, canvas, or navigation.

## W-09 — expose Pyright work cost

Findings:

- BOUND-001

Smallest changes:

- record workspace barrier duration/scope;
- preserve the request timeout as the hard limit;
- stop equating bounded result counts with bounded indexing work.

Red gate:

- `TestPythonWorkspaceIndexWorkIsVisibleToBudget`

Do not optimize or persist an index until measurements show a product problem.

## W-10 — update documentation after behavior passes

Findings:

- DOC-001 through DOC-007

Order:

1. replay the corrected restic backup and init facts;
2. replay the Go/Python comparative fixtures through the same core;
3. record proven, partial, dynamic_unknown, missing, and not_applicable outcomes;
4. update `CORE_IDEA.md`, `MILESTONES.md`, `NEXT_SESSION.md`, `SYSTEM_MAP.md`,
   `TECHNICAL_DEBT.md`, and `PYTHON.md`;
5. retain the correct “static calls are not runtime order” rule in
   `DEEPER_RESEARCH.md`.

Gate:

- no document calls mechanical completion semantic proof;
- desired architecture and implemented architecture are labeled separately;
- uncertainty and not_applicable are recorded as successful honest outcomes.

Do not reorder unrelated milestones or turn the audit into product prose.

## Explicitly out of scope

- SSA/VTA or Python equivalents;
- repository-wide semantic call graphs;
- graph database or new persistent index;
- generic DI or public plugin registry;
- long-lived product-wide LSP infrastructure;
- separate Python worklist/proof engine;
- new browser UI or canvas;
- preserving the current restic 8/8 result.
