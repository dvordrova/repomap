# Decision: Chatto Pipeline Reliability Corrective

Status: Active. Approved by the repository owner in the current session after
reviewing the completed Decision 140 run and a fresh Chatto product run.

## Baseline

- commit: `60c1357d4232d5cda88b0b636c54e90f9a3a6701`;
- diagnostic run:
  `/Users/dvordrova/Library/Caches/repomap/runs/20260729-204144-chatto-13f7dbc1276c`;
- preserve the uncommitted Decision 140 record, Python focus microexperiment,
  and owner-authored Caddy semantic-map artifacts.

## Approved corrective scope

1. Surface discovery must load each deterministic Go module root supplied by
   existing Go facts instead of assuming that the repository root is a Go
   module. Nested-module and true two-module regressions must prove that typed
   packages, exact entrypoints, and downstream surfaces are actually
   inspected. SSA value evaluation reached from detached roots has its own
   per-root step, alternative, description, and cycle budgets so one
   pathological value cannot consume unbounded memory or degrade every later
   surface. Reaching those budgets preserves exact callsites and reports a
   neutral unresolved value rather than exposing an internal budget label.
2. Paved-path collection may follow a bounded relative symlink only when every
   resolved target remains inside the repository and the final regular file is
   also present in the tracked operational inventory. Unsafe, broken, cyclic,
   or unauthorized links are skipped without aborting unrelated evidence.
3. Study Map may recover one unambiguous bounded JSON object from provider
   prose or leaked thinking content before applying the existing strict
   envelope, field, ID, cardinality, and semantic validators. Ambiguous
   recoveries remain rejected. Invalid provider prose is not persisted as a
   raw response.
4. Absence of a Guided Tour candidate remains a local no-call guard, but it is
   an expected skipped presentation stage rather than a user-visible warning.
   The existing three-beat publication contract is unchanged.

## Non-goals

- no new provider request shape, parser framework, language adapter, Search
  surface, proof semantics, report format, or browser redesign;
- no relaxation of canonical saved-artifact decoding or opaque-ID validation;
- no provider call solely to verify these corrections.

## Acceptance

- focused tests cover nested and two-module Go repositories, independent
  value-evaluation budgets, internal/external/unauthorized symlinks,
  leaked-thinking JSON recovery and ambiguity, and quiet Guided Tour skipping;
- `./scripts/check.sh` and `./scripts/etcd_check.sh ../etcd` pass;
- the stable PATH binary is rebuilt once after the checkout is green;
- one authorized Chatto rerun confirms that package loading is no longer a
  fast empty pass, the tracked `CLAUDE.md -> AGENTS.md` link does not erase all
  operational evidence, malformed provider wrapping can be recovered when
  unambiguous, and an empty Guided Tour preflight is quiet.
