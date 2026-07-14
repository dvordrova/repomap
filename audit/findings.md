# Adversarial findings

Audit target: repomap `ed57fa9b2f5746dd71f594304a83a8ea18db0495`
with an intentionally dirty working tree, and restic
`987caba4089fc4345bb201e62c5a2ba96b168049`.

The current saved FlowProof, slot statuses, rendered report, milestone text, and
existing tests were used only to enumerate claims. Verdicts below come from the
restic source, repomap implementation, raw analyzer facts, and two direct
Pyright 1.1.409 runs (`process`: 12 entities/12 relations; `dynamic_call`: 4/4).

## FlowProof and restic

```yaml
id: FP-001
category: semantic-correctness
severity: high

claim:
  Scanner.Scan is both a synchronous runBackup call and the errgroup.Go callback

evidence:
- ../restic/cmd/restic/cmd_backup.go:652
- internal/gofacts/commandtrace.go:303
- internal/analyzer/golang/gotypes/lifecycle.go:107

actual_semantics:
  runBackup synchronously calls wg.Go;
  Scanner.Scan exists only in the registered function literal and is the asynchronous task body

affected_contract:
  Transition.Invocation; callback traversal boundary

required_test:
  TestBackupScanHasNoDuplicateSynchronousEdge

minimal_fix:
  stop handler traversal at nested function literals and retain Scan only under the task body

scope:
  go-command-reader

do_not_generalize:
  no SSA, VTA, or repository-wide call graph is required
```

```yaml
id: FP-002
category: semantic-correctness
severity: high

claim:
  backup is one linear Scan -> Snapshot -> cancel -> Wait -> return path

evidence:
- ../restic/cmd/restic/cmd_backup.go:642
- ../restic/cmd/restic/cmd_backup.go:652
- ../restic/cmd/restic/cmd_backup.go:698
- internal/report/templates/script.js:410

actual_semantics:
  the main branch runs Snapshot while an optional scanner task may overlap it;
  NoScan removes the scanner branch entirely

affected_contract:
  branch ownership; asynchronous task identity

required_test:
  TestBackupContainsSeparateMainAndScannerBranches

minimal_fix:
  represent one main branch and one optional scanner task instead of one ordered list

scope:
  flowproof-go-lifecycle

do_not_generalize:
  support the bounded inline errgroup shape without introducing a general CFG
```

```yaml
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
  cancel requests cancellation and Wait is a separate join operation

affected_contract:
  Relation.Kind

required_test:
  TestBackupCancelDoesNotJoinWait

minimal_fix:
  represent cancellation, join, and source order as separate relations

scope:
  shared-core

do_not_generalize:
  no general happens-before solver is required
```

```yaml
id: FP-004
category: semantic-correctness
severity: high

claim:
  the current joins edge identifies what wg.Wait joins

evidence:
- ../restic/cmd/restic/cmd_backup.go:638
- ../restic/cmd/restic/cmd_backup.go:652
- ../restic/cmd/restic/cmd_backup.go:704
- internal/analyzer/golang/gotypes/lifecycle.go:126

actual_semantics:
  outer wg.Wait joins tasks registered on that errgroup, concretely Scanner.Scan when NoScan is false;
  the current edge instead runs from cancel to the Wait method anchor

affected_contract:
  RelationJoins endpoints; task and group ownership

required_test:
  TestBackupWaitJoinsScannerTask

minimal_fix:
  give the scanner task identity and make the Wait operation join tasks owned by the same group

scope:
  flowproof-go-lifecycle

do_not_generalize:
  no cross-repository concurrency analysis is required
```

```yaml
id: FP-005
category: semantic-correctness
severity: high

claim:
  lexical order cancel then Wait then return is a joins/returns transition chain

evidence:
- ../restic/cmd/restic/cmd_backup.go:700
- ../restic/cmd/restic/cmd_backup.go:718
- internal/analyzer/golang/gotypes/lifecycle.go:126
- internal/analyzer/golang/gotypes/lifecycle.go:132

actual_semantics:
  same-branch source order is neither a call nor ownership relation;
  three post-Wait handler outcomes remain distinct

affected_contract:
  Relation.Kind; completion outcome representation

required_test:
  TestSourceOrderIsNotRepresentedAsCallEdge

minimal_fix:
  remove fabricated semantic edges and add a minimal precedes relation only if a consumer needs source order

scope:
  shared-core

do_not_generalize:
  do not build a general control-flow or happens-before engine
```

```yaml
id: FP-006
category: semantic-correctness
severity: high

claim:
  openWithAppendLock and Repository.LoadIndex prove an external I/O boundary

evidence:
- ../restic/cmd/restic/cmd_backup.go:542
- ../restic/cmd/restic/cmd_backup.go:577
- ../restic/cmd/restic/lock.go:41
- ../restic/internal/repository/repository.go:713

actual_semantics:
  these are internal repository or persistence-facade calls;
  the current proof does not reach a selected backend read/write or external operation

affected_contract:
  SlotIOBoundary verification criterion

required_test:
  TestInternalRepositoryCallDoesNotVerifyExternalIOBoundary

minimal_fix:
  keep the slot partial or classify the reached fact as internal_persistence_facade

scope:
  shared-core

do_not_generalize:
  no whole-repository boundary discovery is required
```

```yaml
id: FP-007
category: semantic-correctness
severity: high

claim:
  return from runBackup proves CLI process termination

evidence:
- ../restic/cmd/restic/cmd_backup.go:708
- ../restic/cmd/restic/cmd_backup.go:714
- ../restic/cmd/restic/cmd_backup.go:718
- ../restic/cmd/restic/main.go:188
- ../restic/cmd/restic/main.go:243

actual_semantics:
  those returns prove handler completion outcomes;
  process completion occurs only after Cobra returns to main, exit code is selected, and Exit is called

affected_contract:
  SlotTermination scope

required_test:
  TestRunBackupReturnIsHandlerCompletionNotProcessCompletion

minimal_fix:
  separate handler_completion from process_completion and scope the selected proof explicitly

scope:
  shared-core

do_not_generalize:
  follow only the selected CLI chain; no global termination framework is needed
```

```yaml
id: FP-008
category: semantic-correctness
severity: critical

claim:
  statically resolving every selected target is sufficient to verify its slot and stop complete

evidence:
- internal/flowproof/worklist.go:244
- internal/flowproof/worklist.go:263
- internal/flowproof/worklist.go:356
- internal/flowproof/worklist.go:371

actual_semantics:
  target identity does not prove an architectural role;
  core_operation can be verified while its summary still says targets need proof

affected_contract:
  ProofSlot.Status; StopComplete; architectural classification

required_test:
  TestPartialTargetCannotProduceVerifiedOrStopComplete

minimal_fix:
  remove generic partial-to-verified promotion and apply slot-specific satisfaction rules in core

scope:
  shared-core

do_not_generalize:
  no new planner or autonomous loop is required
```

```yaml
id: FP-009
category: semantic-correctness
severity: critical

claim:
  the init handler trace includes its first domain operation

evidence:
- ../restic/cmd/restic/cmd_init.go:84
- ../restic/internal/global/global.go:463
- ../restic/internal/global/global.go:490
- internal/gofacts/commandtrace.go:388

actual_semantics:
  runInit synchronously calls global.CreateRepository, which opens the backend and initializes the repository;
  name scoring drops this call before proof construction

affected_contract:
  CommandTrace handler coverage; SlotCoreOperation

required_test:
  TestInitResolvesGlobalCreateRepository

minimal_fix:
  retain bounded top-level handler calls before role ranking and resolve CreateRepository separately

scope:
  go-command-reader

do_not_generalize:
  do not replace the bounded collector with repository-wide call analysis
```

```yaml
id: FP-010
category: insufficient-contract
severity: high

claim:
  dispatch transitions carry exact callsite evidence

evidence:
- ../restic/cmd/restic/main.go:77
- ../restic/cmd/restic/main.go:188
- ../restic/cmd/restic/cmd_backup.go:60
- internal/gofacts/commandtrace.go:36
- internal/flowproof/cli.go:43

actual_semantics:
  current transition evidence reuses target declaration lines such as main.go:36 and cmd_backup.go:498;
  actual call or binding sites are different locations

affected_contract:
  Transition.callsite_location; Anchor.target_location

required_test:
  TestDispatchStoresCallsiteAndTargetLocationSeparately

minimal_fix:
  retain registration/binding/callsite location independently from resolved declaration location

scope:
  go-command-reader

do_not_generalize:
  no universal event schema is needed
```

```yaml
id: FP-011
category: semantic-correctness
severity: high

claim:
  one package import supports every component relation projected from overlapping components

evidence:
- internal/report/components.go:413
- internal/report/components.go:438
- internal/report/components.go:458

actual_semantics:
  nested loops create a Cartesian set when several components contain either package;
  the package edge provides no component-specific endpoint witness

affected_contract:
  ComponentRelation evidence

required_test:
  TestPackageImportRequiresComponentSpecificWitness

minimal_fix:
  promote only uniquely owned or explicitly witnessed component endpoints; otherwise retain the package edge

scope:
  report-projection

do_not_generalize:
  no graph database or new component inference pass is required
```

```yaml
id: FP-012
category: semantic-correctness
severity: medium

claim:
  overview unknowns reflect later local resolution

evidence:
- internal/orient/proof.go:10
- internal/report/parse.go:608

actual_semantics:
  local proof adds resolved files but unverified paths are copied unchanged into the report

affected_contract:
  orientation unknown reconciliation

required_test:
  TestLocallyResolvedFilesAreRemovedFromOverviewUnknowns

minimal_fix:
  reconcile exact locally resolved paths before confidence and report projection

scope:
  orientation-core

do_not_generalize:
  do not use fuzzy path matching or rescan the repository
```

```yaml
id: FP-013
category: semantic-correctness
severity: medium

claim:
  localVss.DeleteSnapshots is an immediate synchronous handler operation

evidence:
- ../restic/cmd/restic/cmd_backup.go:599
- internal/flowproof/cli.go:225

actual_semantics:
  the call is deferred and executes during handler unwinding on the branch that created localVss

affected_contract:
  Transition.Invocation

required_test:
  TestDeferredCleanupIsNotSynchronous

minimal_fix:
  preserve DeferStmt as deferred invocation and its branch ownership

scope:
  go-command-reader

do_not_generalize:
  no complete defer stack model is required
```

## Shared core and adapter boundaries

```yaml
id: CORE-001
category: hidden-language-coupling
severity: critical

claim:
  Go and Python already feed one shared bounded analysis core

evidence:
- internal/flowproof/assemble/assemble.go:11
- internal/flowproof/assemble/assemble.go:18
- internal/orient/proof.go:10
- internal/analyzer/python/pyright/analyzer.go:53

actual_semantics:
  Pyright and gopls share an exact-symbol evidence shape;
  FlowProof assembly consumes only Go command traces and directly instantiates the Go types executor, while Python never enters its planner, budget, slots, stopping, or confidence evaluator

affected_contract:
  shared planner and normalized fact input

required_test:
  TestGoAndPythonFixturesUseSamePlannerBudgetStoreAndCompletionEvaluator

minimal_fix:
  define one narrow core input of normalized facts and route both fixtures through the existing planner

scope:
  shared-core

do_not_generalize:
  no plugin registry, DI framework, or separate Python proof engine is justified
```

```yaml
id: CORE-002
category: semantic-correctness
severity: critical

claim:
  language adapters emit facts but cannot decide proof completion

evidence:
- internal/flowproof/worklist.go:131
- internal/flowproof/worklist.go:243
- internal/analyzer/golang/gotypes/lifecycle.go:143
- internal/analyzer/golang/gotypes/lifecycle.go:153

actual_semantics:
  the Go executor returns SlotUpdates and directly marks concurrency and termination verified;
  core then accepts those statuses and can convert them into complete/confidence

affected_contract:
  flowproof.Result; ProofSlot.Status ownership

required_test:
  TestAdapterFactsCannotSetFinalSlotStatusOrConfidence

minimal_fix:
  let adapters return normalized lifecycle evidence and let shared core alone evaluate slots

scope:
  shared-core

do_not_generalize:
  keep a small typed result; do not introduce a generic policy engine
```

```yaml
id: CORE-003
category: hidden-language-coupling
severity: high

claim:
  evidence relation and scenario vocabulary is language-neutral

evidence:
- internal/evidence/evidence.go:78
- internal/evidence/evidence.go:112
- internal/evidence/evidence.go:125

actual_semantics:
  shared enums expose starts_goroutine and goroutine, and shared BuildContext contains only GOOS, GOARCH, and build tags;
  Python task mechanisms and Python environment identity have no equivalent typed place

affected_contract:
  RelationKind; InvocationMode; Scenario

required_test:
  TestGoGoroutineAndPythonTaskShareAsyncLifecycleWithoutLosingMechanism

minimal_fix:
  use generic async lifecycle semantics with a typed mechanism/provider detail and language-specific scenario extensions

scope:
  shared-core

do_not_generalize:
  do not invent a shared AST or universal scheduler model
```

```yaml
id: CORE-004
category: insufficient-contract
severity: high

claim:
  evidence.Graph validation prevents invalid semantic values from crossing adapters

evidence:
- internal/evidence/evidence.go:310
- internal/evidence/evidence.go:341

actual_semantics:
  validation checks entity IDs, endpoints, certainty, provenance, and scenario references;
  it does not validate entity kind, relation kind, resolution kind, invocation mode, language/scope consistency, or location rules

affected_contract:
  evidence.Graph.Validate

required_test:
  TestEvidenceGraphRejectsUnknownSemanticEnums

minimal_fix:
  add closed-enum and minimal cross-field validation to the existing graph validator

scope:
  shared-core

do_not_generalize:
  no schema registry or map-based extension mechanism is required
```

```yaml
id: CORE-005
category: hidden-language-coupling
severity: high

claim:
  downstream consumers preserve any adapter's normalized uncertainty and language identity

evidence:
- internal/symbol/bundle.go:54
- internal/symbol/bundle.go:64
- internal/symbol/bundle.go:122
- internal/symbol/bundle.go:333
- internal/componentprobe/collect.go:264

actual_semantics:
  symbol.CallFact drops resolution and invocation, warning filtering accepts only gopls text, Scenario retains only Go BuildContext, and component probing hard-codes language=go

affected_contract:
  symbol.Bundle; warning codes; component selected entity

required_test:
  TestSymbolBundlePreservesDynamicResolutionInvocationAndLanguage

minimal_fix:
  carry typed resolution/invocation/warning codes and selected language through existing DTOs

scope:
  shared-core

do_not_generalize:
  avoid map[string]any language payloads and avoid a plugin registry
```

## Python

```yaml
id: PY-001
category: semantic-correctness
severity: critical

claim:
  Python dynamic dispatch remains an explicit unresolved dynamic boundary

evidence:
- internal/analyzer/python/pyright/analyzer.go:352
- internal/analyzer/python/pyright/analyzer.go:371
- internal/analyzer/python/pyright/analyzer.go:373
- internal/analyzer/python/pyright/analyzer.go:645
- raw run: pyright-playground app/service.py:18 returned 4 entities, 4 relations, and a calls/static edge to getattr

actual_semantics:
  getattr is emitted as a normal calls/static relation with no resolution value;
  unresolved dynamic semantics exist only in a warning that symbol.Build drops

affected_contract:
  ResolutionKind; CallFact.Resolution

required_test:
  TestPythonDynamicDispatchRemainsDynamicUnknown

minimal_fix:
  add dynamic_unknown resolution and preserve it through the symbol bundle without inventing a runtime target

scope:
  shared-core

do_not_generalize:
  do not attempt whole-program Python dynamic dispatch resolution
```

```yaml
id: PY-002
category: insufficient-contract
severity: high

claim:
  current Python onboarding finds semantic entrypoints, registrations, imports, and resource boundaries

evidence:
- internal/llmbundle/llmbundle.go:630
- internal/llmbundle/llmbundle.go:636
- internal/analyzer/python/pyright/analyzer.go:115
- internal/analyzer/python/pyright/testdata/fixture/main.py:1

actual_semantics:
  ordinary onboarding scores conventional filenames;
  focused Pyright starts from an exact user-selected callable and exposes calls/references but no manifests, module/import graph, decorator or framework registration, CLI/route dispatch, or external resource classification

affected_contract:
  Python normalized seed facts

required_test:
  TestPythonComparativeFixtureProvidesRequiredRoles

minimal_fix:
  add only the smallest bounded Python syntax/manifest facts demonstrated by the comparative fixture

scope:
  python-adapter

do_not_generalize:
  do not add framework-specific registries before one concrete fixture requires each recognizer
```

```yaml
id: PY-003
category: insufficient-contract
severity: high

claim:
  Pyright scenario identity captures the Python environment and project configuration

evidence:
- internal/analyzer/python/pyright/analyzer.go:183
- internal/analyzer/python/pyright/analyzer.go:428
- raw run: pyright-playground app/service.py:8 returned a constant pyright-workspace scenario with empty build inputs

actual_semantics:
  scenario identity records only a constant ID, diagnostic mode, and index barrier;
  it omits interpreter/Python version, source roots, pyrightconfig content, execution environments, and configuration digest

affected_contract:
  evidence.Scenario identity

required_test:
  TestPythonScenarioChangesWithInterpreterSourceRootsAndConfig

minimal_fix:
  include a bounded typed toolchain/config/source-root identity or digest in Scenario

scope:
  python-adapter

do_not_generalize:
  do not persist absolute environment paths or arbitrary environment variables
```

```yaml
id: PY-004
category: hidden-language-coupling
severity: high

claim:
  concurrency is a mandatory proof slot

evidence:
- internal/flowproof/model.go:16
- internal/flowproof/model.go:38
- internal/flowproof/worklist.go:163
- synchronous Python fixture cannot complete

actual_semantics:
  a fully synchronous Python or Go flow has no concrete async lifecycle to prove;
  absence within the scoped flow should be an honest satisfied not_applicable outcome

affected_contract:
  ProofSlot.Status

required_test:
  TestSynchronousPythonFlowConcurrencyNotApplicable

minimal_fix:
  add not_applicable status and count it as satisfied while missing remains unsatisfied

scope:
  shared-core

do_not_generalize:
  do not make every archetype share all eight CLI slots
```

```yaml
id: PY-005
category: semantic-correctness
severity: high

claim:
  the tracked Python fixture demonstrates the same end-to-end proof contract as Go

evidence:
- internal/analyzer/python/pyright/testdata/fixture/main.py:1
- internal/analyzer/python/pyright/testdata/fixture/app/service.py:8
- internal/analyzer/python/pyright/testdata/fixture/app/repository.py:1
- internal/analyzer/python/pyright/analyzer_test.go:77
- internal/analyzer/python/pyright/analyzer_test.go:89

actual_semantics:
  the fixture has no console/package entrypoint declaration, registration, external resource, or proof session;
  its fake analyzer test injects a getattr outgoing record while selecting process in a different function

affected_contract:
  comparative fixture validity

required_test:
  TestPythonComparativeFixtureProvidesRequiredRoles

minimal_fix:
  replace cross-function fake facts with a tiny source-aligned fixture that exercises required roles

scope:
  python-adapter

do_not_generalize:
  keep one small fixture rather than an expensive repository golden test
```

```yaml
id: PY-006
category: hidden-language-coupling
severity: medium

claim:
  focused investigation support is language-neutral after evidence.Graph

evidence:
- internal/reportserver/investigation.go:115
- internal/reportserver/investigation.go:466

actual_semantics:
  investigation freshness captures Go fact context and public test discovery accepts only _test.go;
  a Python graph cannot reuse these downstream actions without losing Python test/config semantics

affected_contract:
  focused investigation fact context and test reference policy

required_test:
  TestPythonEvidenceCanEnterFocusedInvestigationWithoutGoTestAssumptions

minimal_fix:
  make the existing context and test-reference policies consume typed language facts supplied by adapters

scope:
  shared-core

do_not_generalize:
  no universal test framework model is required
```

## Boundedness and presentation

```yaml
id: BOUND-001
category: performance-boundedness
severity: medium

claim:
  Pyright analysis is bounded in both output and work

evidence:
- internal/analyzer/python/pyright/analyzer.go:56
- internal/analyzer/python/pyright/analyzer.go:241
- internal/analyzer/python/pyright/analyzer.go:359

actual_semantics:
  result counts and request timeouts are bounded;
  the workspace/symbol barrier can enumerate the whole workspace, so work volume is not represented by the output limits

affected_contract:
  analyzer budget and metrics

required_test:
  TestPythonWorkspaceIndexWorkIsVisibleToBudget

minimal_fix:
  expose barrier duration/work scope in metrics and enforce the existing timeout; optimize only after measurement

scope:
  python-adapter

do_not_generalize:
  do not build a repository index or persistent LSP service yet
```

```yaml
id: PRES-001
category: presentation-only-issue
severity: high

claim:
  the report renders proof relations without reinterpreting runtime order

evidence:
- internal/report/templates/script.js:410
- internal/report/templates/script.js:425
- internal/report/templates/script.js:431
- internal/report/templates/script.js:441

actual_semantics:
  presentation appends dispatch, core, concurrency, and termination facts into one Guided symbol path;
  static facts and sibling branches become a visual sequence

affected_contract:
  report projection of branches and relations

required_test:
  TestReportDoesNotRenderStaticFactsAsRuntimeSequence

minimal_fix:
  render explicit relation/branch groups from saved state and label unknown order

scope:
  presentation

do_not_generalize:
  no new canvas or UI redesign is required
```

```yaml
id: PRES-002
category: insufficient-contract
severity: medium

claim:
  presentation only renders saved state and never invokes collectors

evidence:
- internal/reportserver/analysis.go:184
- internal/reportserver/investigation.go:115
- docs/SYSTEM_MAP.md:575

actual_semantics:
  reportserver owns HTTP presentation and directly calls location/symbol analyzers plus Go fact capture;
  it is therefore also orchestration, despite documentation describing presentation as passive

affected_contract:
  reportserver package boundary

required_test:
  TestReportRendererIsPureAndAnalysisEndpointUsesApplicationService

minimal_fix:
  keep renderer pure and name or isolate the existing action orchestration seam only when the test requires it

scope:
  orchestration

do_not_generalize:
  do not add DI or one interface per package
```

## Documentation drift

```yaml
id: DOC-001
category: documentation-drift
severity: high

claim:
  CORE_IDEA accurately states a complete eight-slot proof and a common Provider/evidence path

evidence:
- docs/CORE_IDEA.md:102
- docs/CORE_IDEA.md:106
- docs/CORE_IDEA.md:167
- docs/CORE_IDEA.md:234

actual_semantics:
  eight mandatory slots lack not_applicable, restic completion is semantically false, Pyright does not implement Provider, gotypes bypasses evidence.Graph, and a long-lived Pyright LSP session now exists

affected_contract:
  architecture and completion documentation

required_test:
  DocumentationAudit_COREIDEAReflectsVerifiedContracts

minimal_fix:
  update only after behavioral tests establish the corrected contracts

scope:
  documentation

do_not_generalize:
  do not rewrite the product vision during the correction
```

```yaml
id: DOC-002
category: documentation-drift
severity: critical

claim:
  MILESTONES correctly declares the bounded restic proof complete

evidence:
- docs/MILESTONES.md:399
- docs/MILESTONES.md:405
- docs/MILESTONES.md:406

actual_semantics:
  the documented cancel -> Wait -> return chain is the central false relation and 8/8 is produced by invalid completion rules

affected_contract:
  milestone completion claim

required_test:
  DocumentationAudit_MilestoneDoesNotClaimInvalidResticCompletion

minimal_fix:
  replace completion language with audited proven/partial/unknown outcomes after fixes land

scope:
  documentation

do_not_generalize:
  do not reorder unrelated milestones
```

```yaml
id: DOC-003
category: documentation-drift
severity: medium

claim:
  NEXT_SESSION describes the current blocker and supported language boundary

evidence:
- docs/NEXT_SESSION.md:3
- docs/NEXT_SESSION.md:7
- docs/NEXT_SESSION.md:26
- docs/NEXT_SESSION.md:104

actual_semantics:
  the handoff is dated 2026-07-10, remains Go-only, and calls exact-symbol selection the next blocker even though that slice and Python playground now exist;
  it omits the FlowProof semantic audit gate

affected_contract:
  session handoff

required_test:
  DocumentationAudit_NextSessionNamesCurrentAuditGate

minimal_fix:
  refresh the handoff only after the minimal correction outcome is known

scope:
  documentation

do_not_generalize:
  retain concise operational handoff rather than an audit transcript
```

```yaml
id: DOC-004
category: documentation-drift
severity: high

claim:
  SYSTEM_MAP accurately states that only gopls exists and all language adapters end at evidence.Graph

evidence:
- docs/SYSTEM_MAP.md:199
- docs/SYSTEM_MAP.md:201
- docs/SYSTEM_MAP.md:292
- docs/SYSTEM_MAP.md:566

actual_semantics:
  Pyright exists, gotypes implements the FlowProof executor directly, shared evidence types contain Go-only fields, and reportserver invokes collectors

affected_contract:
  as-is system map and dependency rules

required_test:
  DocumentationAudit_SystemMapMatchesImplementationMap

minimal_fix:
  update module state and distinguish desired rules from current violations after correction

scope:
  documentation

do_not_generalize:
  do not present intended modularity as implemented modularity
```

```yaml
id: DOC-005
category: documentation-drift
severity: high

claim:
  TECHNICAL_DEBT records the demonstrated FlowProof risks

evidence:
- docs/TECHNICAL_DEBT.md:490
- docs/TECHNICAL_DEBT.md:499

actual_semantics:
  TD-021 records package-loading cost but not duplicate calls, false joins, role promotion, missing not_applicable, or adapter-owned completion policy

affected_contract:
  technical debt inventory

required_test:
  DocumentationAudit_TechnicalDebtIncludesAcceptedSemanticFindings

minimal_fix:
  add accepted unresolved findings after correction scope is agreed

scope:
  documentation

do_not_generalize:
  do not use technical debt text as a substitute for failing tests
```

```yaml
id: DOC-006
category: documentation-drift
severity: high

claim:
  PYTHON.md acceptance claims match the tracked adapter

evidence:
- PYTHON.md:251
- PYTHON.md:257
- PYTHON.md:318
- PYTHON.md:399

actual_semantics:
  scenario identity lacks Python/config inputs, dynamic_unknown is not represented structurally, ordinary onboarding is filename-oriented, and the same planner/proof core is not used;
  the later section correctly admits that a Python facts provider is still missing

affected_contract:
  Python implementation status

required_test:
  DocumentationAudit_PythonClaimsMatchComparativeFixture

minimal_fix:
  keep the admitted limitations and remove stronger acceptance claims after tests define the boundary

scope:
  documentation

do_not_generalize:
  do not promise framework parity from Pyright alone
```

```yaml
id: DOC-007
category: documentation-drift
severity: high

claim:
  DEEPER_RESEARCH evidence rules are respected by presentation

evidence:
- docs/DEEPER_RESEARCH.md:115
- internal/report/templates/script.js:410
- internal/report/templates/script.js:441

actual_semantics:
  documentation says static calls are not runtime order while the report constructs one guided sequence from unrelated proof relations

affected_contract:
  evidence ladder versus report semantics

required_test:
  DocumentationAudit_StaticEvidenceIsNotPresentedAsRuntimeOrder

minimal_fix:
  align the report behavior first, then retain the existing documentation rule

scope:
  presentation

do_not_generalize:
  no documentation change is needed if the implementation is corrected to match it
```

`docs/OPEN_QUESTIONS.md` contains open questions rather than a contradictory
implementation claim for this slice. It needs no audit finding yet; uncertainty
there is honest.
