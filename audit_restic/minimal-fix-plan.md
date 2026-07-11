# Minimal fix plan

No implementation starts until the required tests are introduced red. Each
step is a small reviewable change; no umbrella PR is required.

## 0. Freeze the audit contract

input:
- `audit_restic/findings.md`
- `audit_restic/failing-tests.md`
- `audit_restic/contract-changes.md`

output:
- accepted finding IDs and test names
- no code, test, documentation or report changes

gate:
  every proposed implementation change maps to at least one failing test

## 1. Introduce focused red fixtures

findings:
- FP-001 through FP-014

changes:
- add a tiny backup-shaped Cobra fixture
- add a tiny init-shaped Cobra fixture
- add one in-memory component-overlap fixture
- add one in-memory unknown-reconciliation fixture
- implement the assertions listed in `failing-tests.md`

gate:
- every required regression fails for the stated semantic reason
- failures do not depend on HTML, model calls, network, restic checkout, timing or opaque generated IDs

do_not_generalize:
- no end-to-end golden report suite
- no test of private helper shape when observable contract is sufficient

## 2. Correct command fact collection

findings:
- FP-001
- FP-009
- FP-011
- FP-012

changes:
- stop handler-level traversal at nested FuncLit boundaries
- classify DeferStmt calls as deferred
- retain bounded top-level handler calls before role ranking so CreateRepository is not silently discarded
- store callsite and target declaration locations separately for Cobra dispatch
- split framework-prefix completeness from handler-fact coverage

gate:
- callback duplicate test passes
- CreateRepository test passes
- dispatch location test passes
- deferred invocation test passes

do_not_generalize:
- no second framework plugin API
- no SSA, VTA or repository-wide call graph

## 3. Correct the bounded errgroup lifecycle

findings:
- FP-002
- FP-003
- FP-004
- FP-005
- FP-007

changes:
- add one scanner-task anchor bound to outer wg
- keep main and optional scanner branches separate
- connect Scan only as task body
- bind cancel to cancelCtx/task scope
- make Wait join the scanner task set
- remove cancel-to-Wait and Wait-to-last-return semantic edges
- preserve the three post-Wait handler outcomes
- label handler completion separately from process completion

gate:
- all lifecycle and termination regressions pass
- NoScan yields no scanner task
- Snapshot's internal errgroup is not attributed to outer Wait

do_not_generalize:
- add at most one minimal precedes/happens_before relation if the source-order test proves it necessary
- no general CFG or concurrency solver

## 4. Make slot completion honest

findings:
- FP-006
- FP-008
- FP-010

changes:
- add not_applicable
- remove generic partial-to-verified promotion based only on target resolution
- give core, I/O, concurrency and termination explicit satisfaction criteria
- require every required slot to be verified or not_applicable before stop complete
- keep internal repository calls partial as I/O boundaries until a concrete boundary witness exists

gate:
- partial target cannot produce 8/8 or stop complete
- init concurrency is not_applicable
- slot summary, missing criteria and status cannot contradict one another

do_not_generalize:
- no autonomous proof search
- no requirement to preserve the current restic 8/8 result

## 5. Correct downstream projections

findings:
- FP-013
- FP-014

changes:
- build package ownership sets before component relation promotion
- emit a component arrow only for unique or component-specific endpoint witnesses
- add a pure orientation reconciliation step after local proof and before confidence gating
- retire only unknown paths grounded by semantically verified local proof evidence

gate:
- one package import does not become several component relations
- resolved unknown is removed/reclassified while unrelated and unresolved unknowns remain
- report parser and browser remain passive consumers

do_not_generalize:
- no graph database, clustering, new UI or fuzzy path inference
- no second repository scan

## 6. Replay and document the honest outcome

findings:
- FP-020 through FP-025

changes:
- replay backup and init from raw facts without a model call
- record proven, partial, unknown and not_applicable outcomes
- update CORE_IDEA, MILESTONES, NEXT_SESSION, SYSTEM_MAP and TECHNICAL_DEBT only after behavioral gates pass

gate:
- backup has no duplicate Scan or false cancel-to-Wait edge
- init includes CreateRepository and concurrency not_applicable
- no document calls a mechanically complete status semantic proof
- no generated report is needed to validate the core contracts

do_not_generalize:
- no SSA, VTA, database, new UI or unrelated architecture refactor
