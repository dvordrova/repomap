# Implementation map

This is a read-only map from audited behavior to the smallest implementation
surface that can change it.

## Restic source witnesses

| Source | Audited responsibility | Key locations |
| --- | --- | --- |
| `../restic/cmd/restic/main.go` | process entry, command construction/registration, Cobra execution, exit-code mapping | `36`, `76-88`, `161`, `188`, `220-243` |
| `../restic/cmd/restic/cmd_backup.go` | backup RunE binding, handler branches, outer scanner lifecycle, handler outcomes | `35`, `60-61`, `498`, `542`, `577`, `638-655`, `698-718` |
| `../restic/internal/archiver/scanner.go` | scanner task body and cancellation behavior | `23`, `63`, `73`, `101` |
| `../restic/internal/archiver/archiver.go` | synchronous Snapshot operation and its separate internal errgroup/persistence path | `178`, `883`, `900-933`, `946-989` |
| `../restic/cmd/restic/cmd_init.go` | init RunE binding, handler branches and omitted core call | `20`, `37-38`, `58-108`, `111-130` |
| `../restic/internal/global/global.go` | CreateRepository target, backend create and Repository.Init call | `463-495`, `498-521`, `561-590` |
| `../restic/internal/repository/repository.go` | LoadIndex internal facade and repository initialization/persistence | `713-754`, `891-944` |
| `../restic/cmd/restic/cleanup.go` | actual process termination | `41-43` |

## Current transition claim ledger

The current proof replay is used here only to inventory claims. Its statuses are
not evidence of correctness.

### Backup transitions

| Current ID | Current claim | Actual callsite | Current/resolved target | Audit | Evidence needed |
| --- | --- | --- | --- | --- | --- |
| `dispatch-02-main-go-newrootcommand-36` | main calls newRootCommand | `main.go:188`, not current evidence `:36` | `main.go:36` | partial | separate callsite/target |
| `dispatch-03-cmd-backup-go-newbackupcommand-35` | root registers backup constructor | `main.go:77` inside AddCommand `:76` | `cmd_backup.go:35` | partial | constructor call, registration call and declaration |
| `dispatch-04-cmd-backup-go-runbackup-498` | constructor installs runBackup callback | `cmd_backup.go:60-61` | `cmd_backup.go:498` | partial | RunE binding, closure callsite and declaration |
| `handler-01-...-newjsonprogress-504` | synchronous construct | `cmd_backup.go:504`, JSON branch | unresolved | partial | target plus branch |
| `handler-02-...-newtextprogress-506` | synchronous construct | `cmd_backup.go:506`, non-JSON branch | unresolved | partial | target plus mutually exclusive branch |
| `handler-03-...-openwithappendlock-542` | synchronous call | `cmd_backup.go:542` | `lock.go:41` | proven call; partial boundary | concrete backend boundary for I/O role |
| `handler-04-...-newprogress-548` | synchronous construct | `cmd_backup.go:548` | unresolved | partial | target; not core by existence |
| `handler-05-...-findparentsnapshot-559` | synchronous call | `cmd_backup.go:559`, non-stdin branch | `cmd_backup.go:470` | proven target; branch omitted | retain branch |
| `handler-06-...-repo-loadindex-577` | synchronous call | `cmd_backup.go:577` | `repository.go:713` | proven call; partial boundary | backend read witness for I/O role |
| `handler-07-...-fs-newlocal-582` | synchronous construct | `cmd_backup.go:582` | unresolved | partial | target |
| `handler-08-...-fs-newlocalvss-598` | synchronous construct | `cmd_backup.go:598`, Windows/VSS branch | unresolved | partial | target plus guard |
| `handler-09-...-deletesnapshots-599` | synchronous call | `cmd_backup.go:599` | unresolved | unsupported invocation | deferred mode plus target/guard |
| `handler-10-...-newcommandreader-610` | synchronous construct | `cmd_backup.go:610`, stdin-command branch | unresolved | partial | target, branch and child-process lifecycle |
| `handler-11-...-newreader-615` | synchronous construct | `cmd_backup.go:615`, stdin branch | unresolved | partial | target plus branch |
| `handler-12-...-newscanner-643` | synchronous construct | `cmd_backup.go:643`, scanner-enabled branch | `scanner.go:23` | proven under branch | retain NoScan guard |
| `handler-13-...-sc-scan-652` | synchronous runBackup call | `cmd_backup.go:652` inside FuncLit | `scanner.go:73` | unsupported and duplicated | remove handler edge; keep task body only |
| `handler-14-...-wg-go-652` | handler invokes Group.Go in goroutine mode | `cmd_backup.go:652`, scanner-enabled branch | `errgroup.(*Group).Go` | partial | synchronous Go call plus separate task start |
| `handler-15-...-archiver-new-655` | synchronous construct | `cmd_backup.go:655` | `archiver.go:178` | proven | role witness only if needed |
| `handler-16-...-arch-snapshot-698` | synchronous call | `cmd_backup.go:698` | `archiver.go:883` | proven | branch/outcome evidence for persistence completion |
| `go-target-1eed34f6ae705523` | Group.Go callback body calls Scan | `cmd_backup.go:652` | `scanner.go:73` | proven but duplicated | task identity and outer group ownership |
| `go-target-5220eb0b7681e0b9` | runBackup cancels | `cmd_backup.go:701` | cancel operation at `:701` | partial | bind cancel to cancelCtx/scanner scope |
| `go-target-8de62aff43293ed6` | cancel joins Wait | `cmd_backup.go:704` | `errgroup.(*Group).Wait` | unsupported | Wait operation joins scanner task; cancel remains sibling |
| `go-target-da5402d919d2c2ab` | Wait returns from runBackup | `cmd_backup.go:718` | return operation at `:718` | unsupported relation | all post-Wait handler branches; process completion separate |

### Init transitions

| Current ID | Current claim | Actual callsite | Current/resolved target | Audit | Evidence needed |
| --- | --- | --- | --- | --- | --- |
| `dispatch-02-main-go-newrootcommand-36` | main calls newRootCommand | `main.go:188` | `main.go:36` | partial | separate callsite/target |
| `dispatch-03-cmd-init-go-newinitcommand-20` | root registers init constructor | `main.go:88` inside AddCommand `:76` | `cmd_init.go:20` | partial | constructor, registration and declaration sites |
| `dispatch-04-cmd-init-go-runinit-58` | constructor installs runInit callback | `cmd_init.go:37-38` | `cmd_init.go:58` | partial | binding/callsite/target separation |
| `handler-01-...-newterminalprinter-63` | synchronous construct | `cmd_init.go:63` | unresolved | partial | target; not core |
| `handler-02-...-maybereadchunkerpolynomial-79` | synchronous call | `cmd_init.go:79` | `cmd_init.go:111` | proven call; not primary boundary | preserve copy/no-copy branches |
| `handler-03-...-json-newencoder-105` | synchronous construct | `cmd_init.go:105`, JSON branch | unresolved | partial | target plus output branch |
| absent | runInit calls CreateRepository | `cmd_init.go:84` | `global.go:463` | unsupported omission | retain and resolve before core/persistence classification |

## Current slot claim ledger

### Backup slots

| Slot | Current status/claim | Audit | Required strengthening |
| --- | --- | --- | --- |
| trigger | verified from constructor declaration | partial | AddCommand callsite, command identity and constructor target |
| entrypoint | verified exact process entrypoint | proven under selected build | retain build scenario |
| dispatch | verified root/subcommand registration | partial | actual callsites and callback binding |
| application_callable | verified RunE callback from handler declaration | partial | RunE binding/callsite plus target |
| core_operation | verified while summary says targets still need proof | partial and contradictory | correct Scan semantics plus role-specific witness |
| io_boundary | verified from Open/Load name matches | unsupported as external boundary | concrete backend/persistence operation or honest partial facade label |
| concurrency | verified callback/cancel/join linked | partial | branches, task/group identity and correct join/cancel endpoints |
| termination | verified joined goroutines reach handler return | partial handler completion; unsupported process completion | all return branches and optional main/Exit chain |

### Init slots

| Slot | Current status/claim | Audit | Required strengthening |
| --- | --- | --- | --- |
| trigger | verified from constructor declaration | partial | AddCommand callsite and command identity |
| entrypoint | verified exact process entrypoint | proven under selected build | retain build scenario |
| dispatch | verified from declarations | partial | actual constructor/registration/callback callsites |
| application_callable | verified from runInit declaration | partial | RunE binding/callsite |
| core_operation | missing | contradicted | CreateRepository callsite and target |
| io_boundary | partial from maybeReadChunkerPolynomial name | unsupported as primary init boundary | CreateRepository to backend create/Repository.Init persistence witness |
| concurrency | missing | not_applicable | explicit N/A within runInit scope |
| termination | missing | partial | branch-aware handler returns; process completion separate |

## Independent source reconstruction

### Common dispatch

```text
func main declaration main.go:161
  -> constructor callsite main.go:188 -> newRootCommand declaration main.go:36
  -> AddCommand registration main.go:76
       -> backup constructor callsite main.go:77 -> declaration cmd_backup.go:35
       -> init constructor callsite main.go:88 -> declaration cmd_init.go:20
  -> ExecuteContext selects one command
       -> backup RunE binding/call cmd_backup.go:60-61 -> runBackup declaration :498
       -> init RunE binding/call cmd_init.go:37-38 -> runInit declaration :58
handler return
  -> ExecuteContext return main.go:188
  -> exit-code mapping main.go:220-238
  -> Exit main.go:243 -> os.Exit cleanup.go:43
```

### Backup

```text
preflight/target/repository/source branches
  -> outer errgroup wg and cancelCtx cmd_backup.go:638-640
  -> NoScan?
       true: no scanner task
       false: main calls wg.Go at :652
                `-> scanner task body sc.Scan(cancelCtx, targets)
  -> main branch constructs Archiver and synchronously calls Snapshot at :698
       `-> Snapshot owns a separate internal errgroup archiver.go:900-933
  -> Snapshot returns
  -> main invokes cancel at :701 (target: cancelCtx/scanner scope)
  -> main invokes outer wg.Wait at :704 (join target: scanner task if registered)
  -> snapshot-error return :707-708
     or partial-source return :713-714
     or scanner-result return :718
```

happens_before_constraints:
- wg.Go invocation precedes Snapshot invocation on the scanner-enabled main branch; task execution may overlap Snapshot
- Snapshot return precedes manual cancel invocation
- cancel invocation precedes Wait invocation, without a call/join edge between them
- scanner task completion precedes successful outer Wait return
- outer Wait return precedes every post-Wait handler outcome

cancellation_target:
  cancelCtx passed to Scanner.Scan; Snapshot receives the original ctx and is not directly canceled by this CancelFunc

join_target:
  tasks registered on outer wg, concretely Scanner.Scan when NoScan is false; not Snapshot's inner workers

handler_completion:
  any selected runBackup return path

process_completion:
  ExecuteContext return, exit-code mapping and Exit/os.Exit

### Init

```text
runInit
  -> positional-argument error branch
  -> version latest/stable/numeric branches
  -> maybeReadChunkerPolynomial
       -> optional secondary repository read branch
       -> no-copy/no-I/O branch
  -> global.CreateRepository cmd_init.go:84 -> global.go:463
       -> backend create -> Repository.Init -> key/config persistence
  -> text success branch -> return nil
     or JSON Encode branch -> return encode result
```

concurrency:
  not_applicable within the runInit handler scope

handler_completion:
  returns at cmd_init.go:60, :74, :81, :86, :105 or :108

process_completion:
  same Cobra/main/Exit chain as backup

## Analyzer and proof pipeline

| Stage | Current implementation | Current behavior under audit | Findings | Smallest edit surface | Red gate |
| --- | --- | --- | --- | --- | --- |
| Cobra discovery | `internal/gofacts/commandtrace.go:108-153` | finds main/root/constructor/handler declarations and labels four steps complete | FP-009, FP-011 | enrich step facts with actual callsites; split prefix completeness from handler coverage | `TestCommandTrace_InitIncludesCreateRepository`, `TestCobraDispatch_StoresCallsiteAndTargetLocationSeparately` |
| Handler call collection | `internal/gofacts/commandtrace.go:303-373` | recursively inspects nested callbacks; drops zero-score calls | FP-001, FP-009, FP-012 | stop at FuncLit, identify DeferStmt, retain bounded top-level calls before role ranking | callback/deferred/CreateRepository tests |
| Name scoring | `internal/gofacts/commandtrace.go:388-419` | role/discovery depends on tokens; CreateRepository scores zero | FP-009 | separate retention from ranking; the minimal fixture must retain create | `TestCommandTrace_InitIncludesCreateRepository` |
| Initial proof construction | `internal/flowproof/cli.go:43-126` | turns declaration steps into transitions and flattens handler calls under one handler | FP-001, FP-002, FP-011 | consume separate callsite/target facts and task/deferred invocation | dispatch/branch tests |
| Slot selection | `internal/flowproof/cli.go:137-192`, `236-311` | selects core/I/O by name and starts concurrency/termination as missing | FP-006, FP-008, FP-010 | explicit satisfaction criteria; not_applicable; do not equate names with roles | I/O, partial-complete, init-N/A tests |
| Type resolution | `internal/analyzer/golang/gotypes/resolver.go:50-81`, `268-297` | correctly resolves selected callable identity under one package/build, but result is later over-promoted | FP-006, FP-008, FP-011 | keep resolver output limited to target/resolution; preserve target location separately | partial-complete and dispatch tests |
| Lifecycle extraction | `internal/analyzer/golang/gotypes/lifecycle.go:51-167` | identifies Go callback/cancel/Wait/last return but links wrong endpoints | FP-002 through FP-005, FP-007 | add task/group identity, correct cancel/join endpoints, remove lexical-order semantic edges | four lifecycle tests plus termination test |
| Lifecycle tail search | `internal/analyzer/golang/gotypes/lifecycle.go:216-240` | chooses first cancel, first matching Wait and last later return by AST position | FP-003, FP-005, FP-007 | retain bounded same-function branches and all post-Wait outcomes | source-order and all-outcomes tests |
| Worklist refresh | `internal/flowproof/worklist.go:356-375` | promotes partial slots when referenced transitions resolve | FP-008 | remove generic promotion; require slot-specific result update | `TestSession_PartialTargetCannotBecomeVerifiedOrCompleteByResolutionAlone` |
| Completion stop | `internal/flowproof/worklist.go:152-195`, `263-266` | complete means no slot status differs from verified | FP-008, FP-010 | accept verified/not_applicable only; partial/missing/unresolved blocks complete | partial-complete and init-N/A tests |
| Flow attachment | `internal/flowproof/assemble/assemble.go:17-39`, `internal/orient/proof.go:10-13` | attaches proof only to candidate flow | FP-014 | add explicit post-proof reconciliation in orientation layer | unknown reconciliation test |
| Confidence gate | `internal/orient/confidence.go:31-66`, `98-107` | mechanically complete proof suppresses per-flow missing-context item | FP-008, FP-014, FP-020 | consume corrected semantic completion only; do not infer completeness from current enum state | partial-complete and unknown tests |
| Overview unknown projection | `internal/report/parse.go:480-489`, `608-613` | proof paths become openable; unknown list remains unchanged | FP-014 | consume reconciled orientation state; parser should not invent reconciliation | unknown reconciliation test |
| Component package assignment | `internal/report/components.go:24-69`, `469-498` | multiple components can own the same package | FP-013 | retain overlap explicitly; do not assume exclusive ownership | component witness test |
| Component relation projection | `internal/report/components.go:413-467` | Cartesian-promotes one package edge to all matching component pairs | FP-013 | require unique ownership or component-specific witnesses | component witness test |

## Contract ownership after correction

| Contract | Owner | Must not absorb |
| --- | --- | --- |
| command framework callsite/target facts | `internal/gofacts` Cobra reader | slot completion, UI, model prose |
| callable identity and source target | `internal/analyzer/golang/gotypes` | architectural role verification |
| task/cancel/join/handler outcome facts | bounded Go lifecycle executor | generic runtime tracing or whole-repo CFG |
| slot satisfaction and stop reason | `internal/flowproof` | name-based discovery or report rendering |
| unknown reconciliation | `internal/orient` after local proof | fuzzy path guessing |
| package-to-component promotion | `internal/report` projection | new repository graph collection or component inference |

## Documentation surfaces held until behavior passes

| File | Findings | Current inconsistency |
| --- | --- | --- |
| `docs/CORE_IDEA.md` | FP-020, FP-025 | exact dispatch/complete claims are stronger than implementation; component projection certainty is overstated |
| `docs/MILESTONES.md` | FP-021, FP-025 | restic proof is called complete, documents the false cancel to Wait chain, and overstates component relations |
| `docs/NEXT_SESSION.md` | FP-022 | handoff/next blocker is stale and lacks the new audit gate |
| `docs/SYSTEM_MAP.md` | FP-023 | current FlowProof/Cobra/gotypes modules and risks are absent |
| `docs/TECHNICAL_DEBT.md` | FP-024 | unknown reconciliation scope is incomplete and FlowProof records cost but not semantic defects |
