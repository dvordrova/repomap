# As-is implementation and claim map

## Audit method

The audit used four evidence classes:

1. restic source at `987caba4089fc4345bb201e62c5a2ba96b168049`;
2. repomap source at `ed57fa9b2f5746dd71f594304a83a8ea18db0495`
   plus the current uncommitted implementation;
3. raw analyzer output from gopls and Pyright;
4. the saved FlowProof JSON only to enumerate the claims being challenged.

Generated HTML, current slot status, milestone prose, and existing tests were not
used to justify any verdict.

Verdicts use the requested vocabulary: **proven**, **partially supported**,
**unsupported**, **duplicated**, and **not applicable**.

## As-is module map

| Module | Actual responsibility and public contract | Layer | Dependencies/coupling | Documentation accuracy |
| --- | --- | --- | --- | --- |
| `internal/analyzer` | Consumer-owned `Provider`, `LocationResolver`, and `ExactSymbolAnalyzer` ports returning evidence types | shared port | `internal/evidence` | Partly accurate; not every adapter implements `Provider` |
| `internal/evidence` | Graph, entities, relations, certainty, provenance, scenarios, validation | intended shared core | none beyond stdlib | Shape is shared, vocabulary/BuildContext still leak Go and validation is incomplete |
| `internal/analyzer/golang/gopls` | Exact Go declaration, direct call hierarchy, implementations/references under active build | Go adapter | gopls CLI | Correctly bounded as static exact-symbol evidence |
| `internal/analyzer/golang/gotypes` | Resolves selected Go callsites and inspects a bounded lifecycle | Go adapter plus proof policy | `go/packages`, `flowproof` | Violates “adapter ends at facts”: implements `flowproof.Executor` and returns slot verdicts |
| `internal/analyzer/python/pyright` | Exact Python declaration, call hierarchy, references via one LSP session | Python adapter | `internal/lspclient`, Pyright | Implemented, but only `LocationResolver`/`ExactSymbolAnalyzer`; no repository or FlowProof provider |
| `internal/lspclient` | Generic bounded stdio JSON-RPC transport/lifecycle | infrastructure | subprocess + JSON-RPC | Appropriate transport boundary; untyped maps remain protocol-local |
| `internal/gofacts` | Go modules/packages/imports/`func main`; Cobra command reader and handler call extraction | Go repository facts/framework recognizer | Go AST, local reader | Framework facts are bounded, but call traversal/ranking changes semantics and drops valid calls |
| `internal/flowproof` | Fixed CLI slots, anchors/transitions, task/worklist/budgets, merge, stopping | intended shared core | `internal/evidence` | Core exists but fixed Go-shaped CLI policy and adapter-owned `SlotUpdates` prevent shared use |
| `internal/flowproof/assemble` | Matches model flows to Go Cobra traces and runs gotypes | orchestration | concrete `gofacts`, `gotypes`, `flowproof` | Go-specific assembly, not language-neutral orchestration |
| `internal/orient/proof.go` | Attaches local FlowProof sessions to candidate flows | orchestration | only `bundle.Go.CommandTraces` | Python cannot enter this path |
| `internal/orient/confidence.go` | Caps model confidence using Go entrypoints/traces and mechanical proof completion | shared policy with Go coupling | `gofacts`, `flowproof` | Core policy exists, but current false completion can raise confidence |
| `internal/snapshot` / `internal/llmbundle` | Language hints, bounded candidate file index, compact orientation DTO | survey/context | Go facts plus filename heuristics | Tracked-file layer is broad; semantic entrypoints/imports remain Go-only |
| `internal/symbol` | Converts one evidence graph into bounded exact-symbol model DTO | shared projection | `internal/evidence` | Accepts both adapters, but drops resolution/invocation and Python warnings/scenario inputs |
| `internal/investigation` | Pure reducer/session for focused symbol/source/test work | shared workflow in intent | symbol/source/test contracts | Separate from FlowProof; downstream fact/test policies remain Go-shaped |
| `internal/freshness` | Repository/fact/claim identity and reconciliation | persistence/core | Go build/gopls collectors | Generic state shell with Go-specific fact context |
| `internal/componentprobe` | Selects component anchor and exact symbol candidates | orchestration/Go adapter | gopls/source cards | Hard-codes selected entity language to Go |
| `internal/report/components.go` | Projects package facts to component relations | presentation projection | report DTO/package edges | Cartesian promotion over overlapping component packages invents relations |
| `internal/reportserver` | Serves report plus invokes local analysis/investigation actions | presentation and orchestration | analyzers, investigation, Go fact capture | Documentation calls presentation passive, but package owns collector invocation |
| `internal/report/templates/script.js` | Client rendering of saved proof | presentation | saved JSON | Reinterprets graph facts as one ordered path |

## Boundary verdict

| Required boundary | Actual result |
| --- | --- |
| Adapter parses language/framework constructs | Go/Cobra does partially; Pyright provides exact symbols/calls but no Python manifests/registration facts |
| Adapter emits normalized facts with provenance/uncertainty | gopls and Pyright emit Graph; gotypes emits FlowProof updates and final slot status |
| Adapter never owns planning/confidence | Violated by gotypes `SlotUpdates`; those statuses influence core stop/confidence |
| Shared core schedules tasks and applies budgets | Exists only for Go FlowProof assembly |
| Shared core is language-agnostic | Violated by goroutine enums, Go BuildContext, fixed CLI concurrency slot, Go trace seed |
| Presentation preserves evidence semantics | Violated by the linear “Guided symbol path” and component Cartesian promotion |
| Presentation does not invoke collectors | Renderer is pure, but `reportserver` combines HTTP/action orchestration with direct collector calls |

Conclusion: the implementation has a useful shared **exact-symbol graph shape**,
not yet one shared bounded **analysis core** for Go and Python.

## Direct Go/Python comparison

### Executed fixtures

```bash
PATH="$HOME/go/bin:$PATH" go run ./cmd/gopls-playground \
  --repo . --location internal/componentmap/role.go:41 \
  --analyze-candidate 1 --max-symbols 4 --max-callers 4 --max-callees 4

make pyright-fixture PYRIGHT_LANGSERVER=/usr/local/bin/pyright-langserver

go run ./cmd/pyright-playground \
  --repo internal/analyzer/python/pyright/testdata/fixture \
  --path app/service.py --line 18 \
  --pyright-langserver /usr/local/bin/pyright-langserver
```

| Input | Tool | Raw result |
| --- | --- | --- |
| `internal/componentmap/role.go:41 Normalize` | gopls | 6 entities, 6 relations; exact resolution plus 4 bounded incoming calls; 1 omitted incoming call warning |
| `app/service.py:8 process` | Pyright 1.1.409 | 12 entities, 12 relations; exact resolution, 2 incoming calls, 3 outgoing calls, 5 references |
| `app/service.py:18 dynamic_call` | Pyright 1.1.409 | 4 entities, 4 relations; exact resolution, 1 reference, 1 `calls/static` edge to stdlib `getattr`, 1 dynamic warning |

Neither target module was imported or executed by repomap. Pyright starts a
language server, reads bounded source files, sends `didOpen`, and queries symbol,
call-hierarchy, and reference methods.

### Shared-contract comparison

| Dimension | Go exact-symbol | Python exact-symbol | Same core? |
| --- | --- | --- | --- |
| Graph shape | `evidence.Graph` | `evidence.Graph` | Yes, at this boundary |
| Namespace | Go package-qualified gopls IDs | Python path/range/name IDs | Graph can store both; FlowProof cannot |
| Scenario | GOOS/GOARCH active build | constant Pyright workspace ID with empty Go build | Same struct, not equivalent identity |
| Direct call | `calls/static` | `calls/static` | Yes |
| Dynamic target | type/call hierarchy limitations as warning/uncertainty | `getattr` incorrectly remains plain static call; dynamic only in warning | No honest shared representation |
| Planner/task execution | Separate restic path uses FlowProof + gotypes | none | No |
| Budget/dedup/stopping | FlowProof session for Go Cobra flows | LSP request limits/timeouts only | No |
| Proof slots | Fixed CLI slots for Go trace | none | No |
| Confidence evaluator | Go trace/local-proof gate | none | No |
| Unresolved frontier | transition resolution/slot missing | warnings and graph omission | No shared frontier |
| `not_applicable` | unavailable | unavailable | No |

The current Python fixture is also insufficient for an end-to-end comparison:
it has no console/package entrypoint declaration, framework registration,
external resource boundary, or proof session. `Repository.save` simply returns a
value and is not external I/O.

## Restic current-claim ledger

The exact status/transition text below is the claim under audit, not proof that
the claim is correct.

### Backup slots

| Slot | Exact current claim | Source support | Verdict | Evidence required to strengthen |
| --- | --- | --- | --- | --- |
| trigger | `verified`: registered command `backup` | `Use` at `cmd_backup.go:39`; AddCommand callsite `main.go:77`; constructor declaration `cmd_backup.go:35` | partially supported | retain registration callsite and constructor target separately |
| entrypoint | `verified`: exact process entrypoint | `package main`, `func main` at `main.go:161` | proven | keep package/function provenance and build scenario |
| dispatch | `verified`: root command and subcommand registration | execute call `main.go:188`; registration `main.go:77`; declarations at `main.go:36`, `cmd_backup.go:35` | partially supported | store actual call/binding and target locations independently |
| application_callable | `verified`: Run/RunE callback | RunE binding/body `cmd_backup.go:60-61`; target `runBackup` at `:498` | partially supported | retain binding, callsite, and target; framework rule may justify callback relation |
| core_operation | `verified`: exact core-looking callsites; targets still need proof | exact calls include `Snapshot:698`; selected targets resolve | partially supported | an explicit role criterion; resolution alone is insufficient; summary/missing must agree with status |
| io_boundary | `verified`: exact I/O-looking callsites | `openWithAppendLock:542`, `LoadIndex:577` reach internal facades | unsupported as external I/O; partially supported as internal persistence facade | selected backend read/write/resource witness or relabelled internal-facade claim |
| concurrency | `verified`: callback, cancellation, and join linked by exact source | task lifecycle exists, but cancel/join endpoints and branch topology are wrong | partially supported | task/group identity, separate branches, cancellation target, correct join target |
| termination | `verified`: joined goroutines reach explicit handler return paths | handler returns exist; only last is retained; no process exit chain | partially supported for handler completion; unsupported for process completion | all selected handler outcomes and separately scoped process exit chain |

### Init slots

| Slot | Exact current claim | Source support | Verdict | Evidence required to strengthen |
| --- | --- | --- | --- | --- |
| trigger | `verified`: registered command `init` | `Use` at `cmd_init.go:24`; AddCommand callsite `main.go:88`; declaration `cmd_init.go:20` | partially supported | retain registration callsite/target separately |
| entrypoint | `verified`: exact process entrypoint | `func main` at `main.go:161` | proven | keep scenario provenance |
| dispatch | `verified`: root command and subcommand registration | ExecuteContext `main.go:188`; AddCommand `main.go:88`; RunE `cmd_init.go:37-38` | partially supported | exact call/binding and target locations |
| application_callable | `verified`: Run/RunE callback | RunE body calls `runInit` at `cmd_init.go:38`; target at `:58` | partially supported | binding/callsite/target split |
| core_operation | `missing`: first domain-level operation | source has `global.CreateRepository` at `cmd_init.go:84` to `global.go:463` | unsupported | retain and resolve the existing call; then classify role independently |
| io_boundary | `partial`: `maybeReadChunkerPolynomial` | call at `cmd_init.go:79`; target conditionally opens secondary repo at `:118` | partially supported | branch-aware target expansion and concrete backend/resource access witness |
| concurrency | `missing`: goroutine or async lifecycle | no concrete concurrent lifecycle in scoped `runInit` | not applicable | explicit synchronous scope and `not_applicable` status, not more search |
| termination | `missing`: return/shutdown/completion path | returns at `cmd_init.go:81`, `:86`, `:105`, `:108`; process exit remains in main | unsupported as collected; partially supported in source for handler completion | retain handler outcomes and scope process completion separately |

### Backup transitions

| Transition ID | Exact relation claim | Source callsite | Source target, if resolved | Verdict | Evidence required to strengthen |
| --- | --- | --- | --- | --- | --- |
| `dispatch-02-main-go-newrootcommand-36` | `main calls newRootCommand`, static/synchronous | actual call is `main.go:188`; saved evidence says declaration `:36` | `main.go:36` | partially supported | actual callsite plus target declaration |
| `dispatch-03-cmd-backup-go-newbackupcommand-35` | `newRootCommand registers_command newBackupCommand` | `main.go:77` | `cmd_backup.go:35` | partially supported | registration callsite plus framework binding target |
| `dispatch-04-cmd-backup-go-runbackup-498` | `newBackupCommand callback runBackup` | RunE binding/body `cmd_backup.go:60-61` | `cmd_backup.go:498` | partially supported | binding/callsite separated from declaration |
| `handler-01-cmd-backup-go-backup-newjsonprogress-504` | constructs, unresolved, synchronous | `cmd_backup.go:504` | unresolved | partially supported | resolved constructor target and branch (`gopts.JSON`) |
| `handler-02-cmd-backup-go-backup-newtextprogress-506` | constructs, unresolved, synchronous | `cmd_backup.go:506` | unresolved | partially supported | resolved target and else-branch ownership |
| `handler-03-cmd-backup-go-openwithappendlock-542` | calls, static, synchronous | `cmd_backup.go:542` | `cmd/restic/lock.go:41` | proven as a call; no I/O role proven | separate boundary witness |
| `handler-04-cmd-backup-go-backup-newprogress-548` | constructs, unresolved, synchronous | `cmd_backup.go:548` | unresolved | partially supported | resolved constructor target |
| `handler-05-cmd-backup-go-findparentsnapshot-559` | calls, static, synchronous | `cmd_backup.go:559` | `cmd_backup.go:470` | proven as a conditional synchronous call | retain `!opts.Stdin` branch condition |
| `handler-06-cmd-backup-go-repo-loadindex-577` | calls, static, synchronous | `cmd_backup.go:577` | `internal/repository/repository.go:713` | proven as a call; no external boundary proven | concrete resource/backend relation |
| `handler-07-cmd-backup-go-fs-newlocal-582` | constructs, unresolved, synchronous | `cmd_backup.go:582` | unresolved | partially supported | resolved target |
| `handler-08-cmd-backup-go-fs-newlocalvss-598` | constructs, unresolved, synchronous | `cmd_backup.go:598` | unresolved | partially supported | target plus Windows/VSS branch condition |
| `handler-09-cmd-backup-go-localvss-deletesnapshots-599` | calls, unresolved, synchronous | `cmd_backup.go:599` | unresolved | unsupported invocation semantics | mark deferred, resolve target, preserve branch |
| `handler-10-cmd-backup-go-fs-newcommandreader-610` | constructs, unresolved, synchronous | `cmd_backup.go:610` | unresolved | partially supported | target plus stdin-command branch |
| `handler-11-cmd-backup-go-fs-newreader-615` | constructs, unresolved, synchronous | `cmd_backup.go:615` | unresolved | partially supported | target plus stdin branch |
| `handler-12-cmd-backup-go-archiver-newscanner-643` | constructs, static, synchronous | `cmd_backup.go:643` | `internal/archiver/scanner.go:23` | proven on `!NoScan` branch | retain branch condition |
| `handler-13-cmd-backup-go-sc-scan-652` | `runBackup calls Scan`, static/synchronous | inside nested callback at `cmd_backup.go:652` | `internal/archiver/scanner.go:73` | duplicated and wrong owner/invocation | remove handler edge; keep only task-body edge |
| `handler-14-cmd-backup-go-wg-go-652` | `runBackup starts_goroutine Go`, static/goroutine | `cmd_backup.go:652` | external `errgroup.(*Group).Go` | partially supported | distinguish method call, task registration/start, and task identity |
| `handler-15-cmd-backup-go-archiver-new-655` | constructs, static, synchronous | `cmd_backup.go:655` | `internal/archiver/archiver.go:178` | proven | optional constructor role only; no further strengthening needed for call claim |
| `handler-16-cmd-backup-go-arch-snapshot-698` | calls, static, synchronous | `cmd_backup.go:698` | `internal/archiver/archiver.go:883` | proven | retain main-branch ownership and returned outcome |
| `go-target-1eed34f6ae705523` | `Go callback Scan`, static/goroutine | callback body `cmd_backup.go:652` | `internal/archiver/scanner.go:73` | partially supported | explicit scanner-task node owned by outer errgroup |
| `go-target-5220eb0b7681e0b9` | `runBackup cancels cancel`, type-inferred/synchronous | `cmd_backup.go:701` | cancel function created for `cancelCtx` at `:639` | partially supported | target cancellation scope, not merely operation anchor |
| `go-target-8de62aff43293ed6` | `cancel joins Wait`, static/synchronous | Wait call `cmd_backup.go:704` | `errgroup.(*Group).Wait` | unsupported | Wait operation must join scanner task set; cancel remains sibling |
| `go-target-da5402d919d2c2ab` | `Wait returns return-from-runBackup`, static/synchronous | last return `cmd_backup.go:718` | handler return outcome | unsupported | source order is not returns relation; retain all post-Wait branches |

### Init transitions

| Transition ID | Exact relation claim | Source callsite | Source target, if resolved | Verdict | Evidence required to strengthen |
| --- | --- | --- | --- | --- | --- |
| `dispatch-02-main-go-newrootcommand-36` | `main calls newRootCommand`, static/synchronous | actual call `main.go:188`; saved evidence says `:36` | `main.go:36` | partially supported | actual callsite plus target |
| `dispatch-03-cmd-init-go-newinitcommand-20` | `newRootCommand registers_command newInitCommand` | `main.go:88` | `cmd_init.go:20` | partially supported | registration callsite plus target |
| `dispatch-04-cmd-init-go-runinit-58` | `newInitCommand callback runInit` | RunE binding/body `cmd_init.go:37-38` | `cmd_init.go:58` | partially supported | binding/callsite plus target |
| `handler-01-cmd-init-go-progress-newterminalprinter-63` | constructs, unresolved, synchronous | `cmd_init.go:63` | unresolved | partially supported | resolved target if relevant; not a core role |
| `handler-02-cmd-init-go-maybereadchunkerpolynomial-79` | calls, static, synchronous | `cmd_init.go:79` | `cmd_init.go:111` | proven as a call; boundary role unproven | branch expansion for optional secondary repository access |
| `handler-03-cmd-init-go-json-newencoder-105` | constructs, unresolved, synchronous | `cmd_init.go:105` | unresolved | partially supported | resolved target and JSON branch; still not the omitted core operation |

## Independent source control-flow reconstruction

This reconstruction was made from restic source before evaluating the guided
path. Branches are intentionally not flattened.

### Backup

```text
process main branch
  main:188 calls newRootCommand(...).ExecuteContext(ctx)
    Cobra registration/binding
      main:77 registers newBackupCommand
      cmd_backup:60 binds RunE callback
      cmd_backup:61 callback calls runBackup

runBackup main branch
  ... validation/open/parent/index/target setup branches ...
  638 create outer errgroup wg, wgCtx
  639 create cancelCtx and cancel
  640 defer cancel for handler unwinding

  branch !opts.NoScan
    643 construct Scanner
    652 call wg.Go(callback)             [task registration/start]
      scanner task body
        652 call Scanner.Scan(cancelCtx, targets)
        Scan return -> scanner task completion

  main branch continues after wg.Go returns
    655 construct Archiver
    ... configure archiver ...
    698 call Snapshot synchronously
    Snapshot returns before line 701
    701 call cancel                       [cancels cancelCtx]
    704 call wg.Wait                      [joins tasks registered on outer wg]
    Wait returns only after registered tasks complete

    branch err != nil
      708 return fatal snapshot error     [handler completion outcome]
    branch !success
      714 return ErrInvalidSourceData     [handler completion outcome]
    otherwise
      718 return werr                     [handler completion outcome]

back in process main branch
  ExecuteContext returns after Cobra/handler completion
  main maps error/context to exit code
  main:243 calls Exit(exitCode)            [process completion path]
```

Ordering facts justified within the selected source:

- The `wg.Go` call returns before the main branch reaches `Snapshot`; this does
  not order the scanner task body against `Snapshot`.
- `Snapshot` returns before `cancel`; `cancel` returns before `Wait` is called.
- `Wait` returns after every task registered on the outer group completes.
- The scanner task may overlap the main branch's Snapshot call.
- `NoScan` creates no scanner task, so the outer join set is empty.
- Snapshot has its own internal concurrency; it is not owned by the outer `wg`.

### Init

```text
process main branch
  main:188 calls newRootCommand(...).ExecuteContext(ctx)
    main:88 registers newInitCommand
    cmd_init:37 binds RunE callback
    cmd_init:38 callback calls runInit

runInit main branch
  branch args present -> return validation error at 60
  select repository version (with parse-error return at 74)
  79 call maybeReadChunkerPolynomial
    branch error -> return at 81
  84 call global.CreateRepository
    target global.go:463
      reads configuration/password
      opens/creates selected backend at global.go:480
      initializes repository at global.go:490
    branch error -> return at 86
  branch !JSON -> print success; fall through
  branch JSON -> Encode and return at 105
  otherwise return nil at 108

handler completion returns through Cobra to process main
process completion still requires main's exit-code mapping and Exit at main.go:243
```

There is no concrete concurrent lifecycle in scoped `runInit`; concurrency is
`not_applicable`, not missing.

## Mandatory contradiction results

| Check | Result |
| --- | --- |
| Is `Scanner.Scan` emitted both synchronously and as callback? | Yes. `handler-13...` is the duplicate synchronous edge; `go-target-1eed...` is callback/task-body evidence. |
| Does cancel call or join Wait? | No. They are sibling operations in `runBackup`; current `cancel -> Wait joins` is unsupported. |
| What does Wait join? | Tasks registered on the outer errgroup, concretely the optional Scanner.Scan task. It does not join Snapshot's internal group. |
| What is the current I/O boundary? | At best an internal repository/persistence facade. External backend I/O is not proven. |
| Does `runBackup` return prove process termination? | No. It proves handler completion; `main.go:243` is on the process exit path. |
| Can core operation verify while summary says targets need proof? | Yes, due generic `refreshResolvedSlots`; this is an internal contradiction. |
| Should concurrency be N/A without lifecycle? | Yes, for scoped synchronous init/Python flows. |
| Does `runInit` call `global.CreateRepository`? | Yes, at `cmd_init.go:84`; it is dropped because name scoring does not retain `CreateRepository`. |
| Are dispatch callsites distinct from declarations? | Yes: e.g. `main.go:188 -> main.go:36`, `main.go:77 -> cmd_backup.go:35`, `cmd_backup.go:61 -> :498`. |
| Are component edges duplicated by overlapping packages? | Yes; nested component loops Cartesian-promote a package edge without endpoint witness. |
| Are overview unknowns stale after local resolution? | Yes; proof paths are added/openable while `UnverifiedPaths` are copied unchanged. |

## Documentation consistency audit

No documents were repaired during this audit.

| Document and location | Stale or contradictory statement |
| --- | --- |
| `docs/CORE_IDEA.md:102-108` | Describes a complete fixed eight-slot proof without `not_applicable`; current 8/8 can be mechanically but not semantically complete. |
| `docs/CORE_IDEA.md:113-115` | Correctly limits Go package loading, but omits that the Go executor also owns slot-verification policy. |
| `docs/CORE_IDEA.md:161-168` | Says language adapters implement the same `Provider` and graph; Pyright implements two focused ports, and gotypes bypasses Graph into FlowProof. |
| `docs/CORE_IDEA.md:234` | Says there is no long-lived LSP client; tracked Pyright now keeps a reusable LSP session. |
| `docs/MILESTONES.md:399-408` | Calls restic proof complete, records the false `cancel -> Wait -> return` chain, and treats all eight slots as completed. |
| `docs/NEXT_SESSION.md:3` | Last-updated date predates Python and FlowProof work. |
| `docs/NEXT_SESSION.md:7-17` | Product/support boundary is stated as Go-only despite tracked Python onboarding/playground. |
| `docs/NEXT_SESSION.md:26-35` | Recommended next work and six M3 tasks are stale relative to current implementation/audit. |
| `docs/NEXT_SESSION.md:83-108` | Names selected-flow to exact-symbol wiring as the next blocker although the component/symbol slice exists; omits semantic audit as gate. |
| `docs/SYSTEM_MAP.md:199-205` | Calls evidence vocabulary language-neutral without noting Go-only relations/build context or downstream semantic loss. |
| `docs/SYSTEM_MAP.md:292` | Says only the Go/gopls adapter exists; Pyright exists. |
| `docs/SYSTEM_MAP.md:566-568` | Says language adapters end at facts; gotypes returns FlowProof slot updates. |
| `docs/SYSTEM_MAP.md:575-576` | Says presentation does not invoke collectors; reportserver directly invokes analyzers/Go fact capture. |
| `docs/TECHNICAL_DEBT.md:490-507` | Records only hidden package-loading cost for FlowProof, not demonstrated semantic defects or false completion. |
| `PYTHON.md:13-34` | Describes a good language-neutral foundation/no core rewrite before showing that Python cannot enter FlowProof. |
| `PYTHON.md:114-120` | Specifies structural unresolved dynamic behavior that the adapter currently stores only as warning prose. |
| `PYTHON.md:251` | Requires Python/Pyright/config scenario inputs that current scenario omits. |
| `PYTHON.md:257` | Says registry/getattr remains unresolved dynamic; downstream graph/bundle semantics do not preserve that state. |
| `PYTHON.md:318-330` | Correctly limits ordinary Python onboarding to filenames/directions; any stronger semantic-onboarding interpretation is unsupported. |
| `PYTHON.md:399-411` | Correctly admits the missing Python facts provider; this contradicts earlier broad “same core” implications and should remain the honest boundary. |
| `docs/DEEPER_RESEARCH.md:115-116` | Says static calls are not runtime order; current report linearizes them into “Guided symbol path”. |
| `docs/OPEN_QUESTIONS.md` | No contradiction found for this slice; it labels questions as open and should stay that way. |
