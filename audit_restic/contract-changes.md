# Required contract changes

This file defines the smallest contract deltas implied by the failing tests. It
does not authorize implementation yet.

## C-001: callsite and target locations are independent

findings:
- FP-011

current_contract:
  CommandTraceStep has one path/line and BuildCLI uses it both as transition evidence and target anchor location

required_contract:
- every dispatch/call transition has callsite_location
- a resolved target anchor has target_location
- callback binding location may be distinct from callback-body callsite and target declaration
- declaration existence alone cannot verify dispatch

acceptance_test:
  TestCobraDispatch_StoresCallsiteAndTargetLocationSeparately

## C-002: invocation scope stops at callback/deferred boundaries

findings:
- FP-001
- FP-012

current_contract:
  nested FuncLit calls are collected as handler calls and ordinary calls default to synchronous

required_contract:
- handler synchronous calls exclude nested callback bodies
- callback/task bodies are collected under their callback/task owner
- DeferStmt calls use deferred invocation
- the same source call cannot appear once synchronous and once asynchronous

acceptance_tests:
- TestMeaningfulHandlerCalls_ErrgroupCallbackIsNotSynchronousHandlerCall
- TestCommandTrace_DeferredCleanupIsNotSynchronous

## C-003: concurrent tasks have identity and group ownership

findings:
- FP-002
- FP-003
- FP-004
- FP-005

current_contract:
  lifecycle is represented as start target to callback, handler to cancel, cancel to Wait, Wait to last return

required_contract:
- an asynchronous task has an anchor independent of Group.Go and Scanner.Scan
- task registration/start identifies the owning errgroup
- task body identifies Scanner.Scan
- cancellation identifies cancelCtx or the task scope it affects
- Wait is the join operation and identifies tasks registered on the same group
- cancel and Wait remain sibling operations
- source order is not encoded as calls, joins, callbacks or returns
- if persisted, same-branch order uses a minimal precedes/happens_before relation
- NoScan branch has no scanner task

acceptance_tests:
- TestLifecycle_BackupPreservesMainAndScannerBranches
- TestLifecycle_CancelAndWaitAreSiblingOperations
- TestLifecycle_WaitJoinsScannerTask
- TestLifecycle_SourceOrderIsNotRepresentedAsCallEdge

## C-004: handler completion and process completion are scoped claims

findings:
- FP-005
- FP-007

current_contract:
  one termination slot is verified by Wait to the lexically last handler return

required_contract:
- handler_completion retains all reachable selected handler return outcomes
- process_completion requires the selected dispatch return and Exit/os.Exit chain
- a proof may verify handler completion while process completion remains partial or out of scope
- a return source location proves a return statement, not process termination

acceptance_tests:
- TestTermination_HandlerReturnDoesNotProveProcessExit
- TestLifecycle_BackupKeepsAllPostWaitOutcomes

## C-005: slot status includes not_applicable

findings:
- FP-010

current_contract:
  SlotStatus contains missing, partial, verified and unresolved

required_contract:
- add not_applicable
- not_applicable requires a stated flow scope and evidence that the event kind is absent from that scope
- not_applicable is a successful honest terminal outcome
- missing continues to mean required evidence is absent

acceptance_test:
  TestBuildCLI_InitConcurrencyIsNotApplicable

## C-006: transition resolution cannot verify architectural roles

findings:
- FP-006
- FP-008

current_contract:
  every partial non-concurrency slot with resolved evidence transitions becomes verified

required_contract:
- target resolution updates only transition target/resolution/certainty
- each slot owns an explicit satisfaction criterion
- core operation requires a domain-role witness, not a name score alone
- I/O boundary requires a concrete persistence/external operation witness or remains partial
- verified requires empty missing criteria and a summary consistent with verified state
- stop complete requires every required slot verified or not_applicable
- partial, missing or unresolved prevents stop complete

acceptance_tests:
- TestIOBoundary_InternalRepositoryCallRemainsPartial
- TestSession_PartialTargetCannotBecomeVerifiedOrCompleteByResolutionAlone

## C-007: command trace completeness covers retained handler semantics

findings:
- FP-009

current_contract:
  CommandTrace.Complete means four framework declaration steps exist and Missing is empty

required_contract:
- framework_prefix_complete and handler_fact_coverage are separate claims
- an important top-level handler call cannot disappear solely because its name scores zero
- CreateRepository is retained as a callsite before role classification
- unresolved retained calls remain explicit rather than absent

acceptance_test:
  TestCommandTrace_InitIncludesCreateRepository

## C-008: local proof reconciles orientation unknowns

findings:
- FP-014

current_contract:
  local proof paths are appended to OpenablePaths while orientation unknowns are copied unchanged

required_contract:
- locally resolved exact files are removed or reclassified from overview unknowns
- a directory unknown may be reclassified only with an explicit descendant/package rule
- unrelated unknowns remain
- nonexistent/model-fabricated paths remain diagnostics and are never promoted

acceptance_test:
  TestOrientation_ReconcilesLocallyResolvedUnknownFiles

## C-009: component relations require component-specific witnesses

findings:
- FP-013

current_contract:
  package edges are projected to every source/target component pair containing the edge packages

required_contract:
- a package edge remains a package fact
- promotion to a component relation requires unique package ownership or explicit source and target component anchors
- relation evidence records the component-specific witness in addition to the package edge
- ambiguous overlap yields no component arrow

acceptance_test:
  TestComponentRelations_RequireComponentSpecificWitness

## Compatibility decision

The affected FlowProof/report contracts are unreleased. No compatibility layer
for the semantically incorrect shape is required. Saved local audit/replay input
may be regenerated; no migration framework is justified by these tests.
