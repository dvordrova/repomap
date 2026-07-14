# MVP test matrix

Tests are selected by root cause. Increment 1 is capped at eight focused
semantic tests. A separate knowingly-red repository commit is not required;
the increment report records the pre-fix failure and the test ships with its
fix.

## Increment 1: required focused tests (maximum eight)

| Test | Protects | Pre-fix failure |
| --- | --- | --- |
| `TestBackupScanHasNoDuplicateSynchronousEdge` | a nested callback body is not collected as an immediate handler call | `Scanner.Scan` appears in handler calls and again as the `errgroup.Go` callback |
| `TestBackupContainsMainAndScannerBranches` | handler and asynchronous task are not flattened into one sequence | every retained operation is attached to the handler |
| `TestBackupCancelDoesNotJoinWait` | cancellation and join are sibling semantics | current `joins` edge is `cancel -> Wait` |
| `TestBackupWaitJoinsScannerTask` | join has stable task identity | current edge names `Wait`, not the joined scanner task |
| `TestInitTraceIncludesCreateRepository` | first init domain operation is retained | scoring omits `global.CreateRepository` |
| `TestPartialTargetCannotCompleteProof` | resolution cannot verify a role or produce `stop=complete` | generic refresh promotes partial slots after target resolution |
| `TestDispatchKeepsCallsiteAndTargetLocationsSeparate` | registration evidence is a callsite and declaration is a target | current dispatch evidence reuses declaration locations |
| `TestInitConcurrencyIsNotApplicable` | an optional slot can be honestly satisfied | init concurrency remains missing forever |

The branch and cancellation tests also assert that lexical source order is not
encoded as a synchronous call/join chain. No ninth source-order symptom test is
added.

## Later increments

| Test | Classification | Increment |
| --- | --- | --- |
| `TestEvidenceGraphRejectsUnknownSemanticEnums` | required after shared contract change | 2 |
| `TestCoreOwnsApplicabilityAndSlotVerdicts` | required after adapter/core boundary change | 2 |
| `TestComponentRelationRequiresEndpointWitnesses` | required projection regression | 3 |
| `TestResolvedFilesAreRemovedFromOverviewUnknowns` | required state reconciliation regression | 3 |
| `TestPythonDynamicDispatchRemainsUnknown` | required confident-lie regression | 4 |
| `TestPythonScenarioIdentityTracksProjectInputs` | required shared scenario regression | 4 |
| `TestPythonFactRoundTripPreservesUncertainty` | required shared DTO regression | 4 |
| `TestSynchronousPythonConcurrencyIsNotApplicable` | superseded by the shared applicability rule plus one Python fixture assertion | 4 |
| deferred cleanup invocation test | useful but deferred | future |
| external I/O role test | useful but deferred until a concrete boundary witness exists | future |
| handler-return-versus-process-exit test | required only if process termination becomes an approved slot goal | future |
| Pyright total-work budget test | useful but deferred | future |
| reportserver service-boundary test | not required for MVP | none |
| documentation grep tests | not required; behavior fixtures and final review are the authority | none |

## Verification tiers

### Tier 1 — during an increment

- changed packages only;
- new semantic regressions;
- warm target under two minutes.

### Tier 2 — before each increment commit

- `go test ./...` or the smallest repository-wide offline command covering the
  changed contract;
- one deterministic restic or Python fixture relevant to the increment;
- warm target under five minutes.

### Tier 3 — integration checkpoints

Run the full offline quality/replay suite only after a shared contract change,
after the final increment, or when a focused test exposes unexpected
cross-package breakage. Live provider calls and repeated cold scans are never
mandatory verification.

If a command exceeds ten minutes, stop it when safe, record the completed
portion, run the smallest relevant deterministic subset, and continue unless
the omitted check is directly required by the increment.
