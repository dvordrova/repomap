# Restic FlowProof findings

Restic revision: `987caba4089fc4345bb201e62c5a2ba96b168049`.
Generated reports, existing slot statuses, milestone claims, and existing tests
were not used as correctness evidence.

---

id: FP-001
category: semantic-correctness
severity: high

claim:
  Scanner.Scan is both a synchronous runBackup call and the errgroup.Go callback

evidence:
- ../restic/cmd/restic/cmd_backup.go:652
- internal/gofacts/commandtrace.go:303
- internal/gofacts/commandtrace.go:314
- internal/analyzer/golang/gotypes/lifecycle.go:71
- internal/analyzer/golang/gotypes/lifecycle.go:107

actual_semantics:
  Scanner.Scan occurs only inside the function literal passed to wg.Go;
  the handler synchronously invokes Group.Go, while Scan is the asynchronous task body

affected_contract:
  Transition.Invocation; Transition.Relation; handler-call extraction boundary

required_test:
  TestMeaningfulHandlerCalls_ErrgroupCallbackIsNotSynchronousHandlerCall

minimal_fix:
  stop handler-level call collection at nested FuncLit boundaries and retain Scan only as task-body evidence

scope:
  go-command-reader

do_not_generalize:
  no repository-wide call graph or SSA is required

---

id: FP-002
category: control-flow-model
severity: high

claim:
  backup is one linear sequence containing Scan, Snapshot, cancel, Wait and return

evidence:
- ../restic/cmd/restic/cmd_backup.go:642
- ../restic/cmd/restic/cmd_backup.go:652
- ../restic/cmd/restic/cmd_backup.go:655
- ../restic/cmd/restic/cmd_backup.go:698
- internal/analyzer/golang/gotypes/lifecycle.go:112

actual_semantics:
  when NoScan is false the main branch starts an optional scanner task and then runs Snapshot synchronously;
  the scanner task may overlap Snapshot; when NoScan is true no scanner task exists

affected_contract:
  FlowProof branch and task ownership semantics

required_test:
  TestLifecycle_BackupPreservesMainAndScannerBranches

minimal_fix:
  give the scanner task a separate anchor and keep the main and task branches distinct

scope:
  flowproof-go-lifecycle

do_not_generalize:
  no general CFG framework is required; the bounded inline errgroup shape is sufficient

---

id: FP-003
category: semantic-correctness
severity: high

claim:
  cancel joins Wait

evidence:
- ../restic/cmd/restic/cmd_backup.go:701
- ../restic/cmd/restic/cmd_backup.go:704
- internal/analyzer/golang/gotypes/lifecycle.go:126

actual_semantics:
  cancel and Wait are sibling operations owned by runBackup;
  cancel requests cancellation of cancelCtx and Wait joins the outer errgroup task set

affected_contract:
  Relation.Kind

required_test:
  TestLifecycle_CancelAndWaitAreSiblingOperations

minimal_fix:
  represent cancellation, join and source order as separate facts

scope:
  shared-core

do_not_generalize:
  no general happens-before solver is required

---

id: FP-004
category: concurrency-ownership
severity: high

claim:
  the current joins relation identifies what wg.Wait joins

evidence:
- ../restic/cmd/restic/cmd_backup.go:638
- ../restic/cmd/restic/cmd_backup.go:652
- ../restic/cmd/restic/cmd_backup.go:704
- ../restic/internal/archiver/archiver.go:900
- ../restic/internal/archiver/archiver.go:933
- internal/analyzer/golang/gotypes/lifecycle.go:126

actual_semantics:
  outer wg.Wait joins only tasks registered on the outer wg, concretely Scanner.Scan when NoScan is false;
  Snapshot owns and joins a different internal errgroup before Snapshot returns

affected_contract:
  RelationJoins endpoints; task identity; group ownership

required_test:
  TestLifecycle_WaitJoinsScannerTask

minimal_fix:
  make the Wait operation join the scanner-task anchor associated with the same wg identity

scope:
  flowproof-go-lifecycle

do_not_generalize:
  no cross-repository concurrency analysis is required

---

id: FP-005
category: relation-semantics
severity: high

claim:
  lexical order cancel then Wait then return is a joins/returns transition chain

evidence:
- ../restic/cmd/restic/cmd_backup.go:700
- ../restic/cmd/restic/cmd_backup.go:704
- ../restic/cmd/restic/cmd_backup.go:707
- ../restic/cmd/restic/cmd_backup.go:718
- internal/analyzer/golang/gotypes/lifecycle.go:126
- internal/analyzer/golang/gotypes/lifecycle.go:132
- internal/analyzer/golang/gotypes/lifecycle.go:216

actual_semantics:
  source order in the main goroutine is not a call or ownership edge;
  after Wait there are separate snapshot-error, partial-source and scanner-error return branches

affected_contract:
  Relation.Kind; handler completion representation

required_test:
  TestLifecycle_SourceOrderIsNotRepresentedAsCallEdge

minimal_fix:
  remove fabricated joins/returns edges and use a minimal precedes fact only if ordering must be persisted

scope:
  shared-core

do_not_generalize:
  do not build a general happens-before engine

---

id: FP-006
category: role-classification
severity: high

claim:
  openWithAppendLock and Repository.LoadIndex prove the external I/O boundary

evidence:
- ../restic/cmd/restic/cmd_backup.go:542
- ../restic/cmd/restic/cmd_backup.go:577
- ../restic/cmd/restic/lock.go:41
- ../restic/internal/repository/repository.go:713
- internal/flowproof/cli.go:183
- internal/flowproof/cli.go:304

actual_semantics:
  the proof reaches internal repository/persistence facade calls;
  it does not identify the backend read/write call, selected backend implementation, or external I/O operation

affected_contract:
  SlotIOBoundary verification criterion

required_test:
  TestIOBoundary_InternalRepositoryCallRemainsPartial

minimal_fix:
  keep the slot partial until a concrete persistence/external boundary witness is reached or relabel the slot as internal facade

scope:
  shared-core

do_not_generalize:
  no whole-repository boundary discovery is required

---

id: FP-007
category: completion-semantics
severity: high

claim:
  return from runBackup proves CLI process termination

evidence:
- ../restic/cmd/restic/cmd_backup.go:707
- ../restic/cmd/restic/cmd_backup.go:718
- ../restic/cmd/restic/main.go:188
- ../restic/cmd/restic/main.go:220
- ../restic/cmd/restic/main.go:243
- ../restic/cmd/restic/cleanup.go:41
- internal/analyzer/golang/gotypes/lifecycle.go:160

actual_semantics:
  runBackup return proves a handler outcome only;
  process completion occurs after ExecuteContext returns, main maps an exit code, and Exit calls os.Exit

affected_contract:
  SlotTermination scope

required_test:
  TestTermination_HandlerReturnDoesNotProveProcessExit

minimal_fix:
  scope the current slot to handler completion and require the main Exit chain for process completion

scope:
  shared-core

do_not_generalize:
  no interprocedural termination framework is required beyond the selected CLI chain

---

id: FP-008
category: completion-semantics
severity: critical

claim:
  resolving every selected target is sufficient to verify core/I/O slots and stop complete

evidence:
- internal/flowproof/cli.go:180
- internal/flowproof/cli.go:185
- internal/flowproof/worklist.go:263
- internal/flowproof/worklist.go:356
- internal/flowproof/worklist.go:371

actual_semantics:
  target resolution proves callable identity, not architectural role or lifecycle correctness;
  the current core slot becomes verified while its summary still says targets need proof

affected_contract:
  Slot.Status; StopComplete; slot-specific verification criteria

required_test:
  TestSession_PartialTargetCannotBecomeVerifiedOrCompleteByResolutionAlone

minimal_fix:
  remove generic partial-to-verified promotion and let only slot-specific evidence satisfy each slot

scope:
  shared-core

do_not_generalize:
  no new planner or autonomous loop is required

---

id: FP-009
category: extraction-completeness
severity: critical

claim:
  the complete init trace contains its first domain-level operation

evidence:
- ../restic/cmd/restic/cmd_init.go:84
- ../restic/internal/global/global.go:463
- ../restic/internal/global/global.go:480
- ../restic/internal/global/global.go:490
- internal/gofacts/commandtrace.go:336
- internal/gofacts/commandtrace.go:341
- internal/gofacts/commandtrace.go:388

actual_semantics:
  runInit synchronously calls global.CreateRepository, which creates the backend and initializes repository key/config;
  the scorer has no create token, gives the unresolved selector score zero, and drops it

affected_contract:
  CommandTrace.Complete; handler call retention; SlotCoreOperation

required_test:
  TestCommandTrace_InitIncludesCreateRepository

minimal_fix:
  retain the bounded top-level CreateRepository call and resolve its target before role verification

scope:
  go-command-reader

do_not_generalize:
  do not replace the bounded reader with repository-wide call graph construction

---

id: FP-010
category: applicability
severity: high

claim:
  init concurrency is missing evidence

evidence:
- ../restic/cmd/restic/cmd_init.go:58
- ../restic/cmd/restic/cmd_init.go:108
- internal/flowproof/model.go:38
- internal/flowproof/cli.go:187

actual_semantics:
  runInit has no concrete handler-scoped task, goroutine, cancellation or join lifecycle;
  concurrency is not_applicable, while the process-wide signal handler is outside this slot scope

affected_contract:
  SlotStatus; SlotConcurrency

required_test:
  TestBuildCLI_InitConcurrencyIsNotApplicable

minimal_fix:
  add not_applicable and count it as an honest satisfied outcome

scope:
  shared-core

do_not_generalize:
  do not search nested callees for concurrency merely to fill the slot

---

id: FP-011
category: evidence-provenance
severity: high

claim:
  dispatch transition evidence locations are exact callsites

evidence:
- ../restic/cmd/restic/main.go:188
- ../restic/cmd/restic/main.go:77
- ../restic/cmd/restic/main.go:88
- ../restic/cmd/restic/cmd_backup.go:61
- ../restic/cmd/restic/cmd_init.go:38
- internal/gofacts/commandtrace.go:130
- internal/gofacts/commandtrace.go:422
- internal/flowproof/cli.go:76

actual_semantics:
  current dispatch steps store target declaration locations as Transition.Evidence;
  callsites and declarations are distinct facts

affected_contract:
  CommandTraceStep; Transition.Evidence; Anchor.Location

required_test:
  TestCobraDispatch_StoresCallsiteAndTargetLocationSeparately

minimal_fix:
  store callsite_location and target_location separately for constructor, registration and callback edges

scope:
  command-framework-contract

do_not_generalize:
  no universal framework plugin API is required

---

id: FP-012
category: invocation-semantics
severity: medium

claim:
  localVss.DeleteSnapshots is invoked synchronously at its source line

evidence:
- ../restic/cmd/restic/cmd_backup.go:598
- ../restic/cmd/restic/cmd_backup.go:599
- internal/flowproof/cli.go:225

actual_semantics:
  the call is registered with defer on the Windows VSS branch and executes during handler unwinding

affected_contract:
  Transition.Invocation

required_test:
  TestCommandTrace_DeferredCleanupIsNotSynchronous

minimal_fix:
  detect DeferStmt ownership and emit deferred invocation

scope:
  go-command-reader

do_not_generalize:
  no general resource-lifecycle analysis is required

---

id: FP-013
category: component-projection
severity: high

claim:
  every component relation has a component-specific static import witness

evidence:
- internal/report/components.go:57
- internal/report/components.go:413
- internal/report/components.go:436
- internal/report/components.go:446

actual_semantics:
  components may overlap the same Go package and each package edge is projected through all matching source/target component pairs;
  one raw cmd/restic to internal/archiver edge can become several component relations

affected_contract:
  ComponentRelation.Evidence; package-to-component promotion

required_test:
  TestComponentRelations_RequireComponentSpecificWitness

minimal_fix:
  promote only uniquely owned or anchor-witnessed component pairs and otherwise retain the package edge without component arrows

scope:
  report-projection

do_not_generalize:
  no graph database, clustering engine or new canvas is required

---

id: FP-014
category: state-reconciliation
severity: high

claim:
  overview unknowns reflect files resolved by the later local proof

evidence:
- internal/orient/proof.go:10
- internal/flowproof/assemble/assemble.go:21
- internal/flowproof/assemble/assemble.go:38
- internal/report/parse.go:480
- internal/report/parse.go:608

actual_semantics:
  FlowProof only attaches to candidate flows;
  proof paths become openable but original overview unverified paths are copied unchanged

affected_contract:
  orientationPart.UnverifiedPaths; ReportData.OrientationUnverifiedPaths

required_test:
  TestOrientation_ReconcilesLocallyResolvedUnknownFiles

minimal_fix:
  reconcile exact resolved files and explicitly supported directory descendants while preserving unrelated and fabricated diagnostics

scope:
  orientation-report-boundary

do_not_generalize:
  no persistent knowledge base or fuzzy path resolver is required

---

id: FP-020
category: documentation-consistency
severity: high

claim:
  CORE_IDEA accurately states that exact dispatch evidence and complete FlowProof safely scope confidence

evidence:
- docs/CORE_IDEA.md:96
- docs/CORE_IDEA.md:103
- docs/CORE_IDEA.md:106
- docs/CORE_IDEA.md:113

actual_semantics:
  dispatch evidence currently uses declarations as callsites;
  complete is a mechanical status condition and can contain false invocation/join relations and unresolved architectural roles

affected_contract:
  documented FlowProof and confidence-gate contract

required_test:
  TestSession_PartialTargetCannotBecomeVerifiedOrCompleteByResolutionAlone

minimal_fix:
  update documentation only after FP-001 through FP-011 tests pass and describe partial/unknown/not_applicable outcomes

scope:
  documentation

do_not_generalize:
  do not rewrite the project vision during the semantic fix

---

id: FP-021
category: documentation-consistency
severity: critical

claim:
  MILESTONES correctly declares the restic proof complete, its dispatch prefix exact, and its lifecycle cancel to Wait to return

evidence:
- docs/MILESTONES.md:4
- docs/MILESTONES.md:389
- docs/MILESTONES.md:395
- docs/MILESTONES.md:399
- docs/MILESTONES.md:401
- docs/MILESTONES.md:405
- docs/MILESTONES.md:406
- docs/MILESTONES.md:410
- docs/MILESTONES.md:415

actual_semantics:
  the prefix conflates declarations with callsites;
  cancel and Wait are siblings; Scan is duplicated; 8/8 is mechanical; semantic correction is required before only friend evaluation remains

affected_contract:
  milestone completion and evidence-trust claims

required_test:
  TestLifecycle_CancelAndWaitAreSiblingOperations; TestCobraDispatch_StoresCallsiteAndTargetLocationSeparately; TestSession_PartialTargetCannotBecomeVerifiedOrCompleteByResolutionAlone

minimal_fix:
  retract complete and record audited outcomes only after the regression gates pass

scope:
  documentation

do_not_generalize:
  do not redefine all historical milestone completion criteria

---

id: FP-022
category: documentation-consistency
severity: medium

claim:
  NEXT_SESSION describes the current implementation and next blocker

evidence:
- docs/NEXT_SESSION.md:3
- docs/NEXT_SESSION.md:26
- docs/NEXT_SESSION.md:56
- docs/NEXT_SESSION.md:85
- docs/NEXT_SESSION.md:104
- docs/NEXT_SESSION.md:137

actual_semantics:
  the file still says exact-symbol handoff is absent and next even though it is implemented elsewhere;
  it calls onboarding complete and omits FlowProof semantic gates from verification

affected_contract:
  current handoff and next-session entrypoint

required_test:
  n/a; gate the rewrite on the semantic regression suite

minimal_fix:
  replace stale next-work and boundary statements after implementation behavior is corrected

scope:
  documentation

do_not_generalize:
  do not turn the handoff into another roadmap

---

id: FP-023
category: documentation-consistency
severity: medium

claim:
  SYSTEM_MAP describes current code and independently challengeable FlowProof seams

evidence:
- docs/SYSTEM_MAP.md:3
- docs/SYSTEM_MAP.md:30
- docs/SYSTEM_MAP.md:184
- docs/SYSTEM_MAP.md:195
- docs/SYSTEM_MAP.md:256
- docs/SYSTEM_MAP.md:302

actual_semantics:
  current tables omit internal/flowproof, flowproof/assemble, the Cobra command reader and analyzer/golang/gotypes;
  component-relation overlap and FlowProof semantic correctness have no visible seam or red scorecard cell

affected_contract:
  system module map and replaceability scorecard

required_test:
  n/a; verify manually against the post-fix module boundary

minimal_fix:
  add only the actual bounded collectors/resolver/worklist boundaries and their limitations

scope:
  documentation

do_not_generalize:
  do not invent a generic analyzer registry or graph platform

---

id: FP-024
category: documentation-consistency
severity: medium

claim:
  TECHNICAL_DEBT covers the demonstrated FlowProof and unknown-reconciliation gaps

evidence:
- docs/TECHNICAL_DEBT.md:394
- docs/TECHNICAL_DEBT.md:406
- docs/TECHNICAL_DEBT.md:490
- docs/TECHNICAL_DEBT.md:492
- docs/TECHNICAL_DEBT.md:499
- docs/TECHNICAL_DEBT.md:509

actual_semantics:
  TD-016 only covers tracked-inventory reconciliation, not paths later resolved by local proof;
  TD-021 records accounting cost but omits false invocation, join, termination and role semantics

affected_contract:
  demonstrated-debt inventory

required_test:
  TestOrientation_ReconcilesLocallyResolvedUnknownFiles; TestLifecycle_CancelAndWaitAreSiblingOperations

minimal_fix:
  update debt only after the audit tests are red and the minimal correction scope is accepted

scope:
  documentation

do_not_generalize:
  do not add speculative feature debt or optimization work

---

id: FP-025
category: documentation-consistency
severity: medium

claim:
  component arrows are documented as explicit exact static component relations

evidence:
- docs/CORE_IDEA.md:133
- docs/CORE_IDEA.md:135
- docs/MILESTONES.md:347
- docs/MILESTONES.md:348
- docs/MILESTONES.md:350

actual_semantics:
  the underlying package import is exact, but overlapping package membership can promote it into several component pairs without a component-specific witness

affected_contract:
  documented component relation certainty and evidence meaning

required_test:
  TestComponentRelations_RequireComponentSpecificWitness

minimal_fix:
  document package facts separately from promoted component relations after FP-013 passes

scope:
  documentation

do_not_generalize:
  do not remove the component canvas or add model-invented edges
