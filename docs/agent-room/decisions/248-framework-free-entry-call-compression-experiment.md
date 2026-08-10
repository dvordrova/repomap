# 248 — Framework-free entry call compression experiment

**Status:** ACTIVE EXPERIMENT (owner-authorized, 2026-08-09)

**Preserves:** D220 generic typed discovery and honest frontier accounting,
D243's single repository-local SSA/DirectCallIndex authority, D246's refs-only
provider boundary, and D247 publication health.

**Supersedes:** D220 only for runs explicitly carrying `--no-frameworks`:
third-party framework adapters are disabled while generic local extraction
continues. It changes no default run.

## Product question

Can one bounded generic local call-chain bundle plus one refs-only model
selection recover useful entry-call families such as Casdoor's `web.Router`
without a built-in framework adapter?

This is an experiment, not a new report authority. Its only success criterion
is later evidence from an exact `--no-frameworks` Casdoor run. If the selected
families are not useful, remove the stage and the flag instead of adding
prompting, retries, ranking layers, or UI around a failed substrate.

## Local authority and bounds

- Reuse the existing single repository-source SSA pass. A second package load,
  SSA build, graph expansion, or follow-up query is forbidden.
- Preserve the D243 DirectCallIndex unchanged. During that same pass, retain a
  separate private sidecar containing exact local callers, external static
  callees, invocation kind, witness count, and at most three representative
  local callsites per family.
- Compile from at most four exact process roots to depth three. Keep at most 12
  outgoing families per caller, 32 nodes and 48 families per root, and 128
  nodes and 192 families in aggregate.
- The provider request and response are each limited to 64 KiB. A response may
  select at most 12 connected rooted families per root.
- Every omission and frontier remains explicit local accounting. A model
  selection cannot turn a static call family into a runtime surface, route,
  handler, or endpoint claim.

## Provider contract

The temporary stage is `entry_call_compression` and runs exactly once only
when `--no-frameworks` is set and the run is not offline.

The provider sees only request-local `q*`, `r*`, `n*`, and `f*` refs with
sanitized compact labels, depth, invocation kind, witness counts, and aggregate
frontier counts. It never receives repository paths, source locations, source
bytes, a file tree, raw graph, canonical Atlas/DirectCallIndex identities, or
their digests.

The response contains only a version plus root refs and selected family refs.
The backend rejects unknown, duplicate, disconnected, or non-rooted refs and
reconstructs canonical order and exact local callsite actions. The model emits
no prose, scores, ranks, endpoint claims, or next action. There is no semantic
retry; only the shared transport may replay identical immutable request bytes
after a retryable transport failure.

## Deliberately absent product surface

The experiment may persist bounded, secret-scanned result/status artifacts and
the ordinary semantic exchange journal. It adds no report field, manifest
field, UI, Map edge, TriggerRecord, Mechanism, HTTP inventory, readiness rule,
or report/version bump. A failed optional experiment cannot invalidate an
otherwise publishable report.

Any product projection is a separate owner decision after the Casdoor result
is inspected. Framework plugins are a possible later extension, not part of
this experiment.
