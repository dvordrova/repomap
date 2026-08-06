# 233 — Study portfolio equivalence and Overview entry-surface prominence

**Status:** ACTIVE (owner-authorized decision C of the Archive 9 semantic-product program)
**Supersedes:** the kind-only Overview entry classification (cli_command →
tooling collapse; exact HTTP routes → `other` collapse) and the
silent-normalKey theme dropping in the Study reducer.

## Problem (owner pack + corrective)

- **Overview entry surfaces are kind-only and repository-blind.** Exact HTTP
  routes/handlers commonly land in `other` and collapse; `cli_command` is
  always tooling. Evidence on Archive 9: etcd
  (modular_platform_server) carries 30 http_route + 18 process_entry + 36
  cli_command + 8 http_server, of which only the process/server entries show
  as primary; restic (daemon_worker_system) carries 21 http_route + 3
  process_entry + 36 cli_command, again mostly collapsed. The user cannot
  see how work enters the repository.
- **Study semantic duplicates are dropped silently.** The reducer omits a
  theme whose normalized question+title matches an earlier one
  (`seenNormal`, reduce.go:105-109) — a valid theme and its readings vanish
  instead of co-projecting.
- **No portfolio-concentration diagnostic.** A single anchor family
  (TLS/certificates, logging, config, metrics, serialization, tests,
  release tooling, ...) may dominate the Study shelf or the Overview spine
  with no family-level concentration control.

## Decision

### 1. Overview entry surfaces: repository-shape + product-role projection

Replace kind-only grouping with an archetype-aware classification computed
client-side from existing report fields (repository_archetype + trigger
kind/role). The archetype label is the closed enum already published
(application, library_framework, modular_platform_server,
daemon_worker_system, cli_tool).

Primary groups (always visible as category summaries when production
evidence exists):

- **primary process entries** — every archetype;
- **CLI command tree** — cli_tool (CLI-shaped) repositories: cli_command
  entries become PRIMARY product entry, never tooling;
- **exact HTTP/API handlers or route groups** — application /
  modular_platform_server / service-shaped repositories: http_route /
  http_server / grpc_server / service entries group as primary;
- **workers/jobs/consumers** — daemon_worker_system: process_entry +
  consumer/job entries group as primary;
- **public constructors/APIs/registration lifecycle** — library_framework:
  library_api / exported_api group as primary;
- **secondary services** — always visible.

Every visible category shows: category name, exact total count,
evidence/coverage state, 1–3 representative exact entries, a "Show all N"
action, and the unresolved count separately. Do not render hundreds of
handler cards above the fold — categories summarize, the full set stays one
disclosure away.

Collapsed by default (still reachable, never hidden):

- test/example/helper surfaces;
- operations/tooling only when they are not the repository's primary
  product (cli_command on non-CLI repositories stays tooling);
- dynamic unresolved route frontiers (unresolved_provisional / no exact
  source);
- value-shaped/local-expression identities (amount, payer,
  application_context, unresolved value, `result of strings.TrimSpace`,
  `result of fmt.Sprintf`) — the existing value-shaped gate is kept and
  extended to these shapes.

Shape-specific priority table (closed, client-side):

| archetype | primary entry family |
|---|---|
| cli_tool | cli_command (CLI = product) |
| application / modular_platform_server | http_route/http_server/grpc_server/service + process_entry |
| library_framework | library_api/exported_api + registration/lifecycle |
| daemon_worker_system | process_entry + jobs/consumers |
| (monorepo) | per-app/library summaries (grouped by app unit when present) |

Acceptance must prove on Casdoor, etcd, Restic, Telebot and Chatto that a
user can see how work enters the repository without opening a generic
disclosure.

### 2. Study reducer: semantic equivalence co-projects

A theme whose normalized question+title matches an earlier accepted theme
(`seenNormal` collision) is NOT dropped. It co-projects into the earlier
card as an **alternate reading set** (same canonical identity, alternate
title/question retained as provenance, readings appended when distinct and
bounded) — exactly the D4-alternates pattern already used by the
Architecture canvas. The complete set stays bound by the existing
MaxFinalThemes cap; when the cap is hit the honest Omitted count grows, the
alternates never silently vanish.

### 3. Portfolio-concentration diagnostic (generic family control)

A repository-independent concentration check, computed in the Study reducer
and projected into the Study report status:

- count accepted themes/readings by source family/kind (anchor family is
  the a*-seed family; kind is the theme_kind);
- detect when one family dominates the principal projection (a threshold
  fraction of principal cards share one family — e.g. more than half of the
  principal shelf from one anchor family while other families hold exact
  evidence);
- when dominated: co-project the repeated family variants under one primary
  theme with alternate readings, and publish the diagnostic
  `study.portfolio_concentration` with exact before/after counts in the
  Study status;
- the same rule works for logging, metrics, config, serialization, tests,
  release tooling, or any other overrepresented family — **no hard-coded
  TLS string anywhere** (a synthetic non-TLS dominant family test proves
  genericity);
- every suppressed/co-projected anchor remains reachable and counted
  (alternate readings + Omitted accounting; nothing is deleted from the
  artifacts).

Overview: TLS/certificates are not automatically a principal repository
area; cross-cutting concerns show separately from core product areas unless
the repository itself is TLS/security-centric.

### Fresh review verdicts (all applied)

Product: HOLD with 10 bounded defects (D1–D10), all applied:
- D1: closed-enum sentence now lists all 6 published archetypes (including
  monorepo_mixed).
- D2/D4: daemon/telebot/chatto acceptances reworded to shape-level claims
  (the Archive 9 evidence cannot exercise library/daemon entry kinds) plus
  synthetic fixtures.
- D3/F3: value-shaped/unresolved/unavailable entries are COUNTED and
  reachable under a bounded "Unclassified entries (N)" disclosure in the
  grouped Overview path (never deleted, never hidden).
- D5: per-category contract reconciled at the anatomy level
  (entries.total = Σ groups + omitted).
- D6: canonical identity law pinned — the merged card identity is the
  primary theme's; alternates carry provenance only.
- D7/F5/F6: alternates excluded from the badge (supporting/unknown alternate
  ⇒ partial), counted in the browse coverage with the published ⊆
  scout-anchored chain re-verified, and mapped into ThemeRefs ordinals.
- D8/F7: concentration requires OTHER families to hold exact evidence
  (len(counts) ≥ 2), never drops cards, publishes exact before/after counts
  in the Study status; the marker is generic (any source family, proven by
  a synthetic logging shelf).
- D9: monorepo_mixed branch added (http/process promotion).
- D10: StudyThemesVersion 1→2, projection/format bumps executed, pinned
  tests amended.

Red-team: PASS with 8 bounded findings (F1–F8), all applied:
- F1/F3: daemon_worker_system promotes its CLI tree to primary; the omitted
  line renders in the grouped path (counted, not hidden).
- F2: monorepo_mixed handled.
- F4: the hidden full grid builds LAZILY on first click (no eager render of
  hundreds of handler cards).
- F5: badge matches the final visible promise (alternates included).
- F6: alternates count as published (chain re-verified).
- F7: concentration needs other-family evidence + status counts.
- F8: named version bumps executed; new node-runner acceptance test drives
  the REAL script.js with triggers + archetype (service shape: HTTP routes
  primary + Show all N + tooling collapsed; CLI shape: command tree primary).

## Version/cache/replay identities

- StudyThemesVersion v1 → v2 (alternates + concentration diagnostic in the
  reduced portfolio).
- AtlasStudyReportProjectionVersion 10 → 11 and CurrentFormatVersion
  33 → 34 (portfolio concentration + alternates projected).
- Overview entry classification is client-side: no wire change, no new
  artifact; golden report HTML regenerates.

## Acceptance (provider-free)

- Archive 9 replay: etcd/restic overview entry sections show
  primary-category summaries for http routes + process entries (+ CLI tree
  on restic's daemon shape) with exact totals, representatives and Show-all;
  Casdoor shows process + routes; value-shaped and unresolved entries stay
  collapsed; every category count reconciles to the complete surface catalog
  (total = shown + omitted). Library-shaped repositories (Telebot) and
  application repos without available entry triggers (Chatto) are proven by
  SYNTHETIC fixtures instead (their Archive 9 runs carry zero eligible
  entry triggers) — the node-runner classification test covers service,
  CLI, library and daemon shapes from fixtures.
- Study equivalence: two themes with identical normalized question+title but
  distinct readings co-project into one card with alternates; nothing is
  dropped.
- Concentration: a synthetic TLS-heavy Study shelf co-projects to one
  primary TLS theme with alternates + publishes
  `study.portfolio_concentration` with exact counts; the same test with a
  synthetic logging-heavy shelf proves the rule is generic (no TLS string
  in the rule); all anchors remain counted and reachable.
- Fresh code/contract/product reviews, up to two bounded repair cycles.
  Commit implementation and continue automatically.

## Docs

- `PROMPT_VALIDATOR_MATRIX.md` (D231 run dir) — no prompt change in this
  decision; the reducer/classification are backend/local presentation.
- Archive 9 run dirs (tmp/hermes-archive9-semantic-product-20260806-182649)
  hold the entry-kind evidence used above.
