# Decision 219: Study content integrity — truthful coverage, no editorial loss (DEFERRED)

## Status

DEFERRED (2026-08-05) — superseded in priority by Decision 218 (report truth
corrective) per the owner's revised risk review. The owner's revised goal
order puts the report truth corrective first, then Surface Discovery v2,
then the Study deep-reading contract. This decision's implementation was
started and then set aside: the full pending change set is preserved at
`/tmp/d218-01-study-content-integrity-pending.patch` (2026-08-05) and the
original goal text is `goals/01-study-content-integrity-goal.txt` in Archive
5. Resume here after D218 and the later Study deep-reading contract.

## Original status

ACTIVE — bounded corrective authorized by the owner goal
"01-study-content-integrity-goal.txt" (Archive 5, 2026-08-05). Supersedes the
D213-era editorial length rejection and the conflated adjudication issue
vocabulary. Does not redesign the report and does not add a third semantic
stage.

## Proven defects (reproduced on the Archive 5 raw responses)

1. **Chatto Scout**: 12 raw candidates → 3 accepted, 9 rejected as
   `prose_too_long`. Rejected themes are structurally valid core topics
   (role administration, configuration validation, TLS, JetStream backup,
   operator API, key management, push notifications, S3) whose provisional
   title exceeds 80 runes or expected_learning exceeds 240 runes by up to
   120 runes. Editorial bounds erase valid source-referenced candidates.
2. **etcd / Casdoor Adjudication**: populated observations rejected as
   `empty_observation` — etcd 6 of 12, Casdoor 3 of 11; every rejected
   observation is non-empty (etcd 250–496 runes, casdoor 248–317 runes).
   The single `empty_observation` code conflates empty text, overlong
   observation, overlong unknown, and too many unknowns.
3. **Reducer**: the source-backed badge ignores the final question's named
   scope; weak/irrelevant anchor removal can leave the badge claiming full
   support; duplicate direct/supporting readings can be re-added after the
   adjudicator excluded them from reading_order (Casdoor webhook shows
   `sendWebhook` twice); dedup is by anchor ref, not by exact public
   identity `(path, line, symbol)`.

## Product goal

Publish a source-grounded learning shelf where strict identity/authority
checks stay fail-closed, editorial length never erases an otherwise valid
source-referenced theme, each reading explains what to inspect, unresolved
gaps remain visible, the support badge matches the final question's actual
coverage, and the default shelf prioritizes core production behavior over
peripheral diagnostics/tooling.

## Required implementation

### A. Separate safety validation from editorial normalization (Scout)

Keep item-local hard rejection for: malformed JSON; unrequested fields;
wrong/unknown/cross-request/duplicate refs; invalid theme kind; invalid
relation claim; invalid anchor cardinality; duplicate candidate identity.

For valid identity/evidence with overlong model prose:

- normalize deterministically to the active field limit (title 80,
  question 200, why/expected 240 runes), preserving Unicode and
  whole-rune boundaries;
- record a typed normalization count per field in the Scout status;
- never silently truncate (the normalization is counted and identity-bound);
- advance Scout result/status artifact versions as required.

Do not repair refs and do not ask the provider again. Scout prose is
provisional because Adjudication may rewrite title/question; a long
provisional `why_it_matters` / `expected_learning` must not erase the
candidate.

### B. Correct adjudication issue vocabulary

Replace the conflated `empty_observation` with distinct closed reasons:

- `empty_observation` — observation text is empty;
- `observation_too_long` — observation exceeds MaxEditorialRunes;
- `unknown_too_long` — an unknown exceeds MaxUnknownRunes;
- `too_many_unknowns` — more than MaxUnknownsPerTheme;
- existing identity/fit/directness reasons unchanged.

Prefer deterministic normalization (same rune-safe truncation, counted)
for overlong observations/unknowns when the anchor identity and fit are
valid. Empty evidence remains a rejection. Advance adjudication
result/status artifact versions.

### C. Project useful reading interpretation

Extend the bounded report card/detail projection (ThemeCard + Reading) with:

- per-reading supported observation (bounded to MaxEditorialRunes);
- user-facing support role derived from fit (direct / supporting);
- bounded unknowns;
- expected learning;
- an explicit final coverage summary.

No source body or raw prompt material enters report JSON/HTML. Keep the
copy honest: supported observation is a model interpretation over supplied
exact source, not a locally proven runtime fact.

### D. Truthful coverage state

Compute coverage against the original/final theme promise and review
result, not merely the surviving readings. A card is fully supported only
when:

- all named/finally retained facets have direct or supporting readings;
- no anchor required for the stated scope was removed as weak/irrelevant;
- no unresolved unknown materially qualifies the question.

Otherwise publish `partial` with a human explanation. When weak/irrelevant
removal narrows the answerable scope, narrow the visible final
question/title deterministically where possible or preserve it with a
partial state; never claim full support.

### E. Reading order and exact deduplication

- honor the adjudicator's reading_order;
- append only distinct remaining direct/supporting readings;
- deduplicate by exact public identity `(path, line, symbol)`;
- preserve at least one direct reading;
- Casdoor webhook must no longer display `sendWebhook` twice.

### F. Portfolio balance without a third semantic call

Deterministic local portfolio view/rank from already accepted themes:

- production core before tests/tooling/diagnostics;
- entry/user journey before peripheral integration where supported;
- retain archetype diversity;
- do not hide cards; default order/shelf and show-all may differ;
- stable canonical identity stays independent from display order.

Required fixture expectations (no repository-specific production rules):
Telebot default shelf includes construction/startup or update/handler
path, not seven transport/file variants first; Chatto retains core
configuration/security/storage/admin themes from the saved Scout response;
Casdoor includes core authentication/identity themes when present; etcd
does not present a one-reading TLS theme as fully supported; Restic
retains its broad portfolio while narrowing incomplete named facets.

### G. UI

Study card: title, question, concise why, expected learning, truthful
coverage, bounded preview of readings. Study detail: each reading has
symbol/path/source action, "what to inspect here", direct/supporting
language, unknowns/limitations; no concatenated symbol+path text. Keep
Coverage and provenance collapsed by default.

## Provider-free acceptance

Replay the exact saved raw Scout and Adjudication responses from Archive 5
(20260805-123011-etcd, -124745-telebot, -124748-chatto, -124749-restic,
-124752-casdoor) through the rebuilt validators without provider calls.
Prove: Chatto no longer loses structurally valid core themes solely for
prose length; etcd/Casdoor populated observations are not called empty;
all normalization is counted and identity-bound; no ref repair or source
invention; no duplicate exact readings; full-support badge matches final
scope; EN/RU catalog parity; report/manifest round trip; desktop and
390 px Study overview/detail for all five fixtures; no horizontal overflow
or page error.

Gates: gofmt on touched Go files; `go test ./...`; `go vet ./...`;
`make build`; `node --check` on touched assets; all report, manifest,
localization, and saved-response replay gates.

## Non-goals

No third semantic critic; no retry; no fuzzy ref repair; no source blocks
on cards; no embeddings/Tree-sitter; no Architecture rewrite; no broad
shell redesign; no push.
