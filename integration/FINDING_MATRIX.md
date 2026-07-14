# Integration finding matrix

This matrix normalizes `audit/` and `audit_restic/` by semantic claim. The two
audits renumber findings after FP-009, so raw IDs are not treated as stable
identity. An accepted finding is still bounded by the increment and test policy
below; acceptance is not permission to implement every proposed abstraction.

| Root cause | Audit findings | Decision | Increment | Regression boundary |
| --- | --- | --- | --- | --- |
| Nested callback/defer bodies are collected as immediate handler calls | broad FP-001, FP-002, FP-005, FP-013; restic FP-001, FP-002, FP-005, FP-012; PRES-001 | accept the callback case; record defer as later work | 1 | no duplicate `Scanner.Scan`; main and scanner branches remain distinct; source order is not encoded as a call/join chain |
| Concurrent work has no stable task identity or ownership | broad/restic FP-003, FP-004 | accept for the concrete restic `errgroup` lifecycle only | 1 | cancel does not join `Wait`; `Wait` identifies the scanner task it joins |
| Target resolution is mistaken for architectural-role verification and completion | broad/restic FP-006, FP-007, FP-008; CORE-002 | accept the shared applicability/satisfaction rule; keep external I/O and process termination honest rather than chasing them | 1, 2 | unresolved/partial role evidence cannot yield verified or `stop=complete`; handler completion is not process completion |
| Optional slots are reported as missing | broad PY-004; restic FP-010 | accept explicit `not_applicable`, decided by archetype/scope policy with a reason | 1, 2 | synchronous init accepts concurrency `not_applicable` and can complete honestly |
| Command facts lose source callsite or omit the first domain operation | broad FP-009, FP-010; restic FP-009, FP-011 | accept | 1 | init contains `global.CreateRepository`; dispatch callsite and target declaration locations are distinct |
| Component projection multiplies one package edge across overlapping components | broad FP-011; restic FP-013; C-011 | accept conservative reconciliation | 3 | no component relation without an endpoint-specific witness |
| Overview unknowns survive later local resolution | broad FP-012; restic FP-014; C-012 | accept pure state reconciliation | 3 | locally resolved files are removed from overview unknowns |
| Shared evidence enums accept invalid values and DTOs drop uncertainty/language | CORE-003, CORE-004, CORE-005 | accept the smallest closed vocabulary and lossless DTO path used by the Go/Python fixtures | 2, 4 | invalid semantic enums fail validation; language, invocation, resolution and warnings survive round trips |
| Python dynamic dispatch is flattened and scenario identity is constant | PY-001, PY-003, PY-004 | accept explicit `dynamic_unknown`, typed scenario identity and honest non-applicability | 4 | dynamic target is not emitted as a static call; scenario changes when relevant project inputs change |
| Python does not yet provide a production FlowProof path | CORE-001, PY-002, PY-005 | accept as a limitation, not as a request for parity | 2, 4 | one experimental Python fixture crosses the shared fact/decision DTOs without claiming production completeness |
| Focused investigation remains Go-shaped | PY-006 | defer | none | tracked in `DEFERRED.md` |
| Pyright output is bounded but total analyzer work is not measured | BOUND-001 | defer | none | tracked in `DEFERRED.md` |
| Report server mixes rendering and collection orchestration | PRES-002 | reject for this integration | none | no failing MVP semantic test requires a service split |
| Documentation claims audited behavior that source does not prove | DOC-001..DOC-007; restic FP-020..FP-025 | accept, but only after behavior and fixtures are final | 5 | documentation states the verified, partial, unknown and experimental boundaries actually produced by tests |

## Consensus blockers

The following are consensus blockers for an honest restic result:

1. `Scanner.Scan` cannot appear as both an immediate handler call and the
   `errgroup.Go` callback body.
2. The main handler and scanner task cannot be rendered as one linear path.
3. cancellation and join must be sibling operations with different targets.
4. the joined scanner task must have stable identity.
5. `global.CreateRepository` must be retained by the init trace.
6. resolving a source target cannot by itself verify an architectural slot.
7. optional concurrency must support justified `not_applicable`.
8. dispatch callsite evidence cannot reuse a target declaration location.

Everything else is either a dependent projection correction, a bounded shared
contract correction, or explicitly deferred.
