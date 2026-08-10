# 285 — Generic scheduled entry surfaces

**Status:** ACTIVE (owner-authorized, 2026-08-11)

**Preserves:** D283's single bounded EntryCall provider request, generic local
candidate reservoir, refs-only response, exact backend restoration and detached
Entrypoints projection; D281's stable Canvas and explicit context switch; every
existing target, privacy, source-authority and item-local salvage boundary.

## Product gap

Fresh reports can recover generic HTTP routes and flat CLI command descriptors,
but exact schedule registrations such as `Add(name, schedule, callback)` are
already present in the same generic candidate reservoir and remain invisible.
Deterministic worker and async-task records are runtime activities rather than
top-level scheduled registrations, so promoting those records would conflate
two authorities. A prompt-only change also cannot work because the request,
reducer and report currently accept only CLI and HTTP kinds.

## Approved correction

- Extend the existing EntryCall kind choices with `scheduled_job`. There is no
  new model call, collector, framework adapter, option, retry or analysis stage.
- Admit only an already-advertised `direct_call` candidate. The provider binds
  one advertised exact string fact as the job identity and may bind one
  advertised repository-local callable fact as the handler. When a candidate
  has callable facts but the handler is omitted, a second exact string fact
  must establish the named schedule shape; a one-string callback stays
  rejected. HTTP method/path slots and CLI hierarchy are forbidden.
- Prefer an exact stable job name. When the registration exposes no separate
  name, the exact schedule string may be the visible identity. The backend
  restores value, callback, registration location, stable identity and order;
  the provider returns refs only and owns none of those fields.
- The prompt may classify a scheduled job only when the advertised candidate
  establishes a time- or schedule-driven callback registration. Generic
  callbacks, workers and async tasks are not scheduled jobs.
- Entrypoints renders a bounded one-column Workers group with exact
  registration and, when selected, handler source actions. Handlerless records
  remain declared descriptors; callback-bound records are exact static
  registrations. Neither gains Explore or Canvas/Atlas participation.

The existing result and report projection shapes are unchanged. Only the
content-derived EntryCall prompt identity and typed UI message catalog advance
where required by their current contracts.

## Acceptance

Provider-free causal tests cover named `Add(name, schedule, func)` and unnamed
`AddFunc(schedule, func)` shapes, incompatible/ambiguous bindings and valid
CLI/HTTP siblings. A fresh uncached Russian PocketBase executable run must show
its exact scheduled registrations in Entrypoints with working revision-pinned
source actions, no fabricated hierarchy, no Canvas selection and no browser
console errors.

Approved by:
    Repository owner after the generic CLI/HTTP recovery review and the explicit
    request to restore schedulers through the same small refs-only mechanism,
    2026-08-11.
