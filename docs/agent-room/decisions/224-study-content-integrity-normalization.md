# Decision 224: Study content integrity — editorial normalization, honest coverage, exact readings (resumes D219)

## Status

ACTIVE (Phase 2 of the overnight program
`hermes-repomap-overnight-goal-v3.txt`, approved by the repository owner's
standing goal authorization 2026-08-05). Explicitly resumes the accepted
substance of Decision 219 (DEFERRED) while preserving D219 as history; this
decision supersedes it in priority. Provider-free acceptance; no new
semantic stage, no third call.

## Problem (reproduced on saved raw responses)

1. **Scout erases valid themes for prose length.** Chatto (Archive 5)
   returned 12 raw candidates, 9 rejected `prose_too_long` — structurally
   valid core themes (role administration, config validation, TLS,
   JetStream backup, operator API, key management, push, S3) lost because
   provisional title/question/why/expected_learning exceeded editorial
   limits. Scout prose is provisional: Adjudication may rewrite it.
2. **Adjudication conflates four distinct failures under
   `empty_observation`.** etcd saved raw observations run 250-496 runes
   (limit 240), casdoor 248-317 — populated observations rejected as
   "empty"; overlong unknowns and too-many-unknowns share the same code.
3. **Reducer duplicate exact readings.** publishEntries dedups by anchor
   ref, not by exact public identity `(path,line,symbol)`; Casdoor webhook
   showed sendWebhook twice after the adjudicator excluded it.
4. **Coverage badge ignores final scope.** full-support badge computed from
   surviving readings alone; a narrowed reading set can claim full support
   for a question whose named facets are no longer grounded.

## Scope (exact changes, one vertical slice)

### A. Scout — separate safety validation from editorial normalization
- Keep item-local hard rejection for: malformed JSON, unrequested fields,
  wrong/unknown/cross-request/duplicate refs, invalid theme kind, invalid
  relation claim, invalid anchor cardinality, duplicate candidate identity.
- For valid identity/evidence with overlong prose: normalize
  deterministically to the active field limit (title 80, question 200,
  why/expected 240 runes), preserving Unicode and whole-rune boundaries.
- Record typed normalization counts per field in ScoutStatus
  (`normalized_title/question/why/expected` counters).
- Never silently truncate: normalization is counted and identity-bound.
- Advance Scout result/status artifact versions.

### B. Adjudication — split the issue vocabulary
- Replace conflated `empty_observation` with distinct closed reasons:
  - `empty_observation` — observation text is empty;
  - `observation_too_long` — observation exceeds MaxEditorialRunes;
  - `unknown_too_long` — an unknown exceeds MaxUnknownRunes;
  - `too_many_unknowns` — more than MaxUnknownsPerTheme.
- Prefer deterministic normalization (same rune-safe truncation, counted)
  for overlong observations/unknowns when anchor identity and fit are
  valid; empty evidence remains a rejection; identity/fit/directness
  failures unchanged. Advance adjudication result/status versions.

### C. Reducer — exact reading identity and honest coverage
- Honor the adjudicator's reading_order (already); append only distinct
  remaining direct/supporting readings; deduplicate by exact public
  identity `(path,line,symbol)`; never re-add a reading excluded by
  adjudication; preserve at least one direct reading (already).
- Compute support against the final visible theme promise: full badge only
  when every named/finally-retained facet has a direct or supporting
  reading AND no anchor required for the stated scope was removed as
  weak/irrelevant AND no unresolved unknown materially qualifies the
  question. Otherwise `partial` with a human explanation; narrow the final
  question/title deterministically where possible; never claim full
  support for a broad promise on one reading.

### D. Portfolio balance (no third call)
- Deterministic local portfolio view/rank from already-accepted themes:
  production core before tests/tooling/diagnostics; entry/user journey
  before peripheral integration where supported; retain archetype
  diversity; do not hide cards; default order/shelf and show-all may
  differ; stable canonical identity independent of display order.
- No repository-specific keyword table; ordering by local deterministic
  signals (theme kind role ordering + canonical identity tiebreak).

### E. Product/UI
- Every published theme remains visible. Every reading shows typed kind,
  bare symbol, path:line, what to inspect, direct/supporting role, bounded
  unknowns. No source body on cards. Source action remains exact
  GitHub/GitLab/local jump. Study card: title, question, concise why,
  expected learning, truthful coverage, bounded preview of readings.
- Coverage and provenance collapsed by default.

## Authority and privacy

- Normalization is a bounded local transform of model prose; it never
  creates refs, sources, relations, or acceptance.
- Status counts make every truncation visible (no silent loss).
- No raw source on cards; no canonical IDs on the wire.

## Provider-free acceptance

1. Replay the exact saved raw Scout responses (chatto 12 raw) through the
   rebuilt Scout validator: no valid core theme lost solely to prose
   length; all normalization counted and identity-bound.
2. Replay saved raw Adjudication responses (etcd 12 raw with 250-496-rune
   observations; casdoor 11): populated observations not mislabeled empty;
   distinct issue codes emitted; normalization counted.
3. Reducer: Casdoor webhook no longer shows sendWebhook twice; dedup by
   exact (path,line,symbol); ≥1 direct preserved; excluded readings never
   re-added; one-reading TLS-like etcd theme is partial, not fully
   supporting.
4. Portfolio: telebot emphasizes construction/registration/update before
   repetitive file transport variants; restic retains broad coverage with
   core repository/restore/prune not buried; EN/RU parity.
5. Full gates: gofmt, `go vet ./...`, `go test -count=1 ./...`, `make
   build` → `.bin/repomap`, node --check on touched assets, report/manifest
   round trip, localization parity, golden regen.

## Non-goals

- No third semantic critic; no retry; no fuzzy ref repair; no source blocks
  on cards; no embeddings/Tree-sitter; no Architecture rewrite; no prompt
  redesign beyond version bumps; no push.
