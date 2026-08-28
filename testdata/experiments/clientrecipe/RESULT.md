# Add an external client — experiment result

The experiment answers one repository task: how a newcomer can add an outbound
client without first reconstructing the whole service.

## Result

| Layer | Verdict | What it established |
| --- | --- | --- |
| H0 dependency/import baseline | PARTIAL | Found all four real boundaries, but also admitted two critical false positives and could not establish roles, completeness, evidence, or a best example. |
| H1 typed structural footprint | FEASIBILITY PASS | Recovered 4 production boundaries, 32 of 32 expected role assignments, 43 of 43 evidence locators, all 6 exclusions, and exact `10 = 4 + 6` accounting on the controlled repository. |
| H1 controlled robustness | 3 / 3 PASS | Preserved the same exact score under rename, layout, and irrelevant-noise mutations of that repository. This is not a second-repository generalization result. |
| H1 reducer leave-one-out | DOMAIN FAIL | The eight task-required roles stayed present, but they were supplied by the task contract. The only learned repository-pattern role failed every held-out fold: `TP 0 / FP 1 / FN 2`, exact footprint `0 / 3`. |
| H2 presentation copy | Non-authoritative | Adds bounded titles, purposes, and summaries for six short step refs and four short example refs. It cannot change roles, examples, completeness, recommendation, evidence, or exclusions. |

Three boundaries are complete. Kubernetes and Vault are tied as the two most
complete examples; neither is a globally recommended example without a
requested boundary kind. Notifier remains incomplete with exactly three missing
roles: verification, observability, and failure policy.

The word `PASS` is deliberately scoped:

| Claim | Status |
| --- | --- |
| Structural feasibility on the controlled semantic scene | ESTABLISHED |
| Generalization to an independently authored or real repository | NOT ESTABLISHED |
| Improvement to a user's change plan | NOT TESTED |
| Readiness for the ordinary repomap product path | NOT READY |

## Controlled robustness scorecard

| Dimension | Mutation | Asserted invariant | Outcome |
| --- | --- | --- | --- |
| Noise | Added irrelevant lookalike functions, a wrong-signature fake, and an unrelated helper call | Noise does not alter admitted boundaries, roles, evidence, exclusions, or callback accounting | PASS — instances `4/4`, roles `32/32`, evidence `43/43`, exclusions `7/7` |
| Rename | Renamed constructors, configuration helpers, wrapper types, helper variables, and `internal/clients` | Names and the client directory label are not semantic admission authority | PASS — instances `4/4`, roles `32/32`, evidence `43/43`, exclusions `7/7` |
| Layout | Moved ClickHouse verification from an integration package to a package-local unit test | Verification remains grounded and its kind changes exactly with the layout | PASS — instances `4/4`, roles `32/32`, evidence `43/43`, exclusions `7/7` |

The canonical machine-readable scorecard is
`golden/05-robustness.json`. Each row also records ledger, callback frontier,
production-load count, and the incomplete Notifier roles rather than collapsing
the result to one overall word.

## Reducer leave-one-out: negative result

This gate hides each complete boundary from the role-frequency reducer in turn.
It does **not** hide the boundary from the frozen extractor: `ExtractH1` already
saw the whole controlled repository. Candidate construction receives only the
two training H1 instances, the closed role universe, and the task-contract role
set. The evaluator alone joins the held-out boundary to the frozen oracle.

| Held out | Training examples | Contract core | Predicted repository pattern | Held-out truth | Result |
| --- | --- | --- | --- | --- | --- |
| Kubernetes | ClickHouse + Vault | `8 / 8` — `TASK_SUPPLIED_NOT_LEARNED` | none | Failure policy | `FN 1` — FAIL |
| ClickHouse | Kubernetes + Vault | `8 / 8` — `TASK_SUPPLIED_NOT_LEARNED` | Failure policy | none | `FP 1` — FAIL |
| Vault | Kubernetes + ClickHouse | `8 / 8` — `TASK_SUPPLIED_NOT_LEARNED` | none | Failure policy | `FN 1` — FAIL |

The separate contract total is `24 / 24`; it is descriptive and never enters
the predictive metric or verdict. The actual repository-pattern signal is
`TP 0 / FP 1 / FN 2`, with exact held-out pattern match `0 / 3`. Therefore the
domain verdict is **FAIL**. Generalization remains **NOT ESTABLISHED**, user
utility **NOT TESTED**, and production readiness **NOT READY**.

The canonical scorecard is `golden/07-reducer-leave-one-out.json`, bound to
frozen H1 `8d2614…` and freeze receipt `934a4b8b…`. Two executions produce
identical bytes. An evaluator-only oracle mutation changes evaluation while the
candidate bytes remain identical, so held-out truth does not leak into the
candidate. This result rejects productizing the current required/common
frequency reducer as predictive guidance; it does not invalidate the exact
descriptive boundary facts.

## H1 freeze receipt

`golden/05-robustness.json` also carries the deterministic freeze identity for
the next blind and real-repository gates. It binds:

- the sealed baseline Authority → H0 → H1 identity chain, the validated
  `oracle.json` byte identity, and the canonical decoded evaluation identity;
- the Authority, H0, H1, and evaluator schema versions;
- an auto-discovered exact set of every non-test Go source in the experiment
  package, so adding or removing a package source changes the receipt;
- repository-relative SHA-256s for `internal/programindex/index.go`,
  `internal/dependencies/catalog.go`, `go.mod`, `go.sum`, the task contract,
  and the negative-invariant inputs;
- the current admissibility, clustering, role extraction, completeness,
  exclusion, reduction, and ranking rules.

Validation rediscovers the canonical surface and recomputes every file digest,
the aggregate source digest, all baseline identities, the rule surface, and the
receipt digest. An omitted, added, or changed input fails the checked-in golden
until the owner deliberately re-freezes it. The receipt contains no absolute
paths, binaries, generated report HTML, recursive dependency-source tree, Go
toolchain runtime identity, or other build artifacts.

## Demo

Serve this experiment directory on loopback and open
`preview/report.html`. The standalone file contains all CSS, JavaScript, and
validated browser data; it has no external assets or network calls.

The UI starts with the task picker. Open **Add an external client** to see six
repository-specific steps and three complete examples. **Show all 4** reveals
Notifier. Source paths are not created in the DOM until **View evidence** is
chosen. **Candidate audit** is a separate disclosure with the six rejected
candidates and their closed reasons.

## Honest limits

- Evidence comes from one deliberately crafted Go service, not a corpus of real repositories.
- All three robustness mutations reuse that same service and oracle shape. They show resistance to these named perturbations, not extractor generalization.
- Leave-one-out uses only three complete boundaries, does not isolate extraction, and finds no predictive non-contract pattern; the task-defined `24 / 24` coverage is not learned evidence.
- The frozen completeness rule currently supplies mandatory roles before role frequency is reduced over complete instances. The experiment does not yet establish a clean separation between task-required and repository-inferred necessity.
- H1 is a test-only extraction hypothesis and is not integrated into the ordinary repomap pipeline.
- H2 is presentation-only and is tested with an injected deterministic provider; there is no live-provider acceptance claim.
- The HTML is a task prototype, not a change to the production report, schema, CLI, or current ADR.
- No user study was run. The manual protocol records expected observable behavior only.
