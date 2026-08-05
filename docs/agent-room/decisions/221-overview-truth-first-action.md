# Decision 221: Overview truth — deterministic Repository Brief, role-ranked entries, independent first action

## Status

ACTIVE — bounded provider-free presentation/view-model corrective
(goal 02 of the Archive 5 review roadmap, approved by the repository owner
2026-08-05: «02 — Overview truth + first action»). Supersedes no durable
decision; narrows D217's Overview projection and D218's hero/glance.

## Proven defects (from goal 02, reproduced on Archive 5 runs)

- All five reports have empty `project_guess`; visible purpose is an
  arbitrary README excerpt (etcd release warning, Telebot marketing quote,
  Casdoor protocol list, Restic OS-support paragraph, Chatto markdown/URL
  residue);
- «Open first» is derived from the first hash-ordered theme card's first
  reading (`Start`, `(*Bot).File`, `startStartupCPUProfile`,
  `errors.IsFatal`, `StartWebhookDeliveryWorker`) — unstable and not
  product-first;
- Overview exposes too much entrypoint taxonomy in the primary path
  (etcd mobile Overview ≈14.6k px tall).

## Required implementation

### A. Deterministic Repository Brief (view-model)

Bounded view-model from existing local data only: repository archetype,
documented purpose/source material, Navigator recommendation when usable,
process/library entries, primary production surfaces, Architecture
component summaries, Study themes, explicit uncertainty. Fields:

```text
what_it_is
normal_use_starts
major_areas
limitations
source_basis
```

- never expose raw model prose as local fact; mark source basis internally
  and render truthful wording;
- deterministic text cleanup: strip Markdown blockquote markers and
  emphasis safely; resolve/remove dangling reference-link syntax; preserve
  product/protocol names; never let a README warning, badge, release note,
  capability list, or joke quote become the sole product purpose when
  stronger bounded source material exists;
- no repository-specific string table in production.

### B. Role-ranked entries

Classify entry surfaces with existing local evidence:
primary product/runtime entry; library construction/consumption entry;
secondary service; operations/tooling; test/example. Default Overview
shows the primary one or two; the complete taxonomy stays under a
collapsed disclosure. Production entries rank before tests/tools/examples.

### C. Independent «Open first» selector

Never derived from theme ordinal. Priority:
1. usable Navigator startup action with exact evidence;
2. primary production process entry;
3. library constructor/start/use entry;
4. a core Study theme's first exact reading;
5. explicit unavailable state.

Returns label, path, line/symbol, reason, source action,
authority/limitation. Clickable and keyboard accessible. Stable canonical
IDs and display rank remain separate.

### D. Fixture acceptance intents (no repo names hardcoded in production)

- etcd: distributed key-value/coordination server; server/main startup
  before migration/tooling;
- Telebot: Go Telegram Bot API library; construct bot, register behavior,
  start/update lifecycle before file transport internals;
- Chatto: self-hosted team/community chat server; server/config/chat
  lifecycle before CPU profiling;
- Restic: backup program; CLI root/backup path before `errors.IsFatal`;
- Casdoor: identity and access management / SSO server; main/authentication
  path before webhook worker.

### E. Information hierarchy and mobile

- one hero/brief, no duplicate purpose blocks;
- one compact entry section; one clickable first action;
- major areas as compact cards;
- full taxonomy and source README under disclosures;
- fix any card whose visual box exceeds 390 px even when document overflow
  is clipped;
- target materially shorter etcd Overview while preserving all facts under
  progressive disclosure.

## Acceptance

Provider-free rebuild/render for all five saved runs. Assert: no empty
primary brief when source/facts exist; no README warning/quote/protocol
list as sole `what_it_is`; first action no longer depends on theme array
order (permuting theme/card order does not change it); primary entries
rank ahead of tests/tooling; exact source action opens; all hidden facts
remain accessible; EN/RU parity; 1440x1000 and 390x844 screenshots; no
clipping, horizontal overflow, or page error; keyboard journey to first
source in ≤2 actions. Full tests, vet, build, asset checks,
manifest/report gates.

## Non-goals

No provider call; no new repository analysis; no Architecture contract
change; no search platform; no frontend framework; no per-repository
production copy; no push (until owner authorizes).

## Ownership

Decision: dvordrova (repository owner) — goal 02 selection.
Implementation: hermes agent session 2026-08-05.

## Implementation record (2026-08-05, per «го» + goal 02)

### Delivered (working tree, not committed)

- **A — deterministic Repository Brief** (`internal/report/onboarding.go`):
  `skipUnsafePurposeSentences` removes README warnings/unstable notes,
  quoted marketing, protocol/capability lists, and build-status badges from
  the thesis purpose deterministically (no repository-specific table).
  Hero renders the filtered thesis purpose or a neutral local fallback
  (`repo name · localized archetype label`), raw README always stays a
  labeled blockquote source. `neutralPurposeFallback` now uses localized
  closed archetype labels (EN/RU).
- **B — role-ranked entries**: glance "Where does it start?" sorts entries
  production-first (primary process, service, library, CLI/tooling);
  Overview shows primary + service groups expanded, tooling/library/other
  under a collapsed `<details>` disclosure with count.
- **C — independent «Open first» selector**: `overviewFirstAction` with
  priority Navigator startup action (usable only when the recommended
  surface is not test_or_helper/tooling — verified by exact location
  against raw triggers), primary production process entry, library entry,
  Study reading (constructor/start symbols ranked ahead, deterministic
  symbol/path/line tie-breaker for permutation invariance), then explicit
  unavailable. Rendered as a clickable, keyboard-accessible button with
  label, path:line · symbol, reason, and authority.
- **E — mobile**: tooling collapse shortens etcd Overview materially;
  verified zero overflow at 390px on all fixtures + RU.

### Per-fixture result (provider-free rebuild of saved Archive 5 runs)

- etcd: «go.etcd.io/etcd/v3 · server platform»; Open first =
  `server/main.go:30` primary process entry (Navigator recommendation
  rejected — it pointed at contrib/lock/storage, role test_or_helper);
- Telebot: «gopkg.in/telebot.v3 · library» (joke quote filtered); no
  exact source action available in saved data (embedded snippets do not
  cover reading lines) — honest unavailable;
- Chatto: «github.com/chattocorp/chatto · application»; honest
  unavailable (no production surfaces in saved data);
- Restic: «github.com/restic/restic · daemon/worker system»; Open first =
  `cmd/restic/main.go:161` via Navigator (production secondary_service);
- Casdoor: «github.com/casdoor/casdoor · application» (protocol list
  filtered); Open first = `main.go:36` via Navigator (primary).

### Gate results

- `go test ./... -count=1`: **74/74 ok, EXIT 0** (`/tmp/d221-full-test2.log`)
- `go vet ./...`: 0; gofmt on touched Go files: 0; node --check assets: OK
- `make quality-check`, `make localization-check`: ok
- `TestWriteReportHTML_Golden` regenerated + passing; typed UI catalog
  green (glance.open_first retired, archetype + first_action keys added)
- Browser: desktop 1440 screenshots (etcd, restic-ru) verified; mobile
  390px zero overflow 6/6 (5 fixtures + RU); first-action click opens the
  exact source drawer (`cmd/restic/main.go:161 main()` + snippet)
- Provider-free: all gates run without credentials
