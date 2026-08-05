# Decision 214: Locally observed resource boundaries (DB/storage + outbound clients)

## Status

**APPROVED AND ACCEPTED.** Council red-team REVISE→PASS (selection logic sound,
all evidence claims independently verified TRUE; Candidate A confirmed with a
mandatory first-slice bound — this document IS that bound). Implementation
complete; acceptance gates green (go test ./... 74/74, go vet clean, offline
casdoor + service runs exit 0, one live Casdoor RU A/B exit 0).
Approved by: Repository owner via Monster council gate (red-team REVISE with
P1-1 scope bound + P1-2 no-silent-truncation condition, run 20260804T122640Z-
afe3e93, council/20-red-team-214.md; fresh-context read-only red-team).
Implementation accepted 20260805 (live run 20260805-055317-casdoor-d5b0b09e91fc):
Atlas gained 17 boundary + 22 resource entities and 108 exact call-site
observations; Study cited boundaries with exact source (25/33 card anchors
Atlas-proven; scout 10/12, adjudication 10/10, 10 RU theme cards).

## Notes (CURRENT.md style)

Decision 214 builds the first slice of locally observed Boundaries and
Resources on top of the accepted D213 Atlas-first base. The Atlas schema
already defines `EntityBoundary` and `EntityResource` (repositoryatlas/
model.go:34-35) but the Go adapter emits only surface+operation entities
(goadapter/adapter.go:318-321) — the accepted casdoor product proves the gap:
the Atlas contains zero occurrences of http., database/sql, sql., redis,
mysql, postgres, grpc, smtp, os.file, ioutil while the D213 Study cards
correctly claim "DB lifecycle" (object/ormer.go), "SOCKS5 proxy"
(proxy/proxy.go), "TLS certificates" (certificate/) — source-true but not
Atlas-proven. The model layer knows; the local evidence layer is blind.

**Slice bound (red-team P1-1):** exactly two boundary classes in this
decision:
1. **Persistent storage** — databases and object/file storage: `database/sql`,
   `sqlx`, `gorm`, `xorm`, `redis`, `mongo`, `s3`/OSS adapters, `os.OpenFile`/
   `os.WriteFile`/`os.Create` for durable storage.
2. **Outbound network clients** — `http.Client` construction + non-handler
   transport use, `grpc.Dial`/client stubs, and SDK client constructors
   (`NewClient`/`NewXxxClient` returning a client over a network transport).

Message publish/consume, caches/locks, config/secrets, and OS boundaries are
explicitly DEFERRED to a follow-on decision (recorded list, not silently
dropped). This is the red-team's mandatory bound: the full 7-class list is
2–3 decisions, not one.

**Not import classification (owner's explicit bar).** Import-level
classification already exists (gofacts.go:32 ExternalImportsTop,
llmbundle.go:76). The differentiator is **exact call-site AST observation**
using the existing go/ast machinery (the same pattern as
gofacts/entrypoints.go:48 findMainFunctionAnchors): for each observed call
site, emit an exact `(path, line, column, enclosing symbol)` evidence anchor
bound to the boundary entity. The producer never reads import graphs onto any
wire; it emits compact typed refs only.

**Emission.** The existing Go adapter (goadapter/adapter.go) gains additive
emission: `Entity{Kind: EntityResource}` for the storage/network target and
`Entity{Kind: EntityBoundary}` for the typed operation class (e.g.
`persistent_storage`, `outbound_http`), each with exact evidence observations
bound to the call site (Observation{Subject, EvidenceRefs} — the schema
already supports this). No new artifact family, no new semantic stage, no
provider call. The D213 Study Scout consumes the new entities as additional
exact seeds (its seed catalog already accepts Atlas observations), so the
visible consumer is the Atlas entity/relation surface AND the Study shelf.

**No silent truncation (red-team P1-2, owner doctrine).** The accepted D213
pipeline carries a latent wart the boundary producer must not inherit:
theme_scout_request seed packs report `omissions: seed_budget, count=5`
(a32/a34/a35/a36/a38) while the wire tells the model `omitted: 0`. The
boundary producer emits evidence COMPLETE: every observed boundary call site
is either published as an entity+evidence or recorded in a closed,
wire-visible omission with an explicit reason and count. No budget-capped
evidence (red-team contract condition).

**Fixed detector list in code (red-team contract condition).** The boundary
detector is a fixed, bounded list of import-path + call-pattern matchers in
the Go adapter — NOT a plugin/detector registry, NOT a generic analyzer
framework. Adding a boundary class is a code change in a future decision.

**Privacy/contract soundness (red-team verified).** Producer is local and
deterministic (exact-process-evidence pattern, adapter.go:354-386); the
import graph stays in gofacts and never reaches any wire; canonical Atlas IDs
(trigger-*, operation-*) never reach the model — the Study wire continues to
carry only its own advertised request-local refs (route-span-*, a*, f*, t*);
no legacy adapter, no migration, additive emission only.

**Scope exclusions (explicit, not silent).** No new Analysis/UI layer beyond
the Atlas surface; no Overview boundary canvas rendering in this decision
(red-team P2-3 — the canvas currently renders boundaries only as
model-synthesis anchors with honest "runtime reachability is not implied";
a typed Atlas boundary upgrades those in a later decision); no detector
plugin registry; no message/cache/lock/config/secrets/OS classes (deferred
list); no changes to the D213 two-stage pipeline, manifest v12, report
projection v8, or wire contracts.

**Versions/identity.** Atlas artifact version advances (new entity kinds
emitted); the accepted-cache and run-manifest identities that bind Atlas
digests re-derive; old Atlas versions fail closed; no compatibility reader.

**Acceptance.** Provider-free first: (a) go test ./... and go vet ./... green;
(b) a bounded unit/contract test for each detector class with exact
path:line:symbol evidence assertions; (c) offline runs on casdoor and the
service fixture exit 0 and the Atlas now contains typed resource/boundary
entities with exact evidence; (d) the D213 Study shelf still renders with the
new seeds; (e) built binary `make build` → .bin/repomap (PATH). Then exactly
ONE fresh live Casdoor semantic A/B run (owner credentials in the zsh
session, `repomap cache clear && repomap --github-url ... --no-secrets
--lang ru`), judged by M1–M9 material improvement: the Atlas must show
observable storage/network boundaries with exact source, and the Study must
cite them. Acceptance, never a tuning loop, no second live calibration.

## 1. Problem and evidence

### 1.1 Verified gap (independently re-verified by fresh red-team)

Accepted D213 casdoor run (20260804-232816, /tmp/d213-live-ru/): repository
atlas has 37 units, 2 entities (1 operation, 1 surface), 1 relation
(exposes), 2 observations. Zero occurrences of http., ListenAndServe,
database/sql, sql., redis, mysql, postgres, grpc, smtp, os.file, ioutil in
the Atlas JSON. Yet D213 Study cards 4/5/6/9 claim TLS, proxy, DB lifecycle,
API routing, and the anchor files provably contain boundary stdlib usage:
service/proxy.go→http., object/token_oauth_util.go→redis, proxy/proxy.go→http.,
object/ormer.go→sql. (verified against /Users/dvordrova/git/casdoor source).
All 20/20 theme anchors verified exact (object/ormer.go:91 InitAdapter,
proxy/proxy.go:34 InitHttpClient, certificate/account.go:50, ldap/server.go:30).

### 1.2 Why this wins the five-question test

Q1 (enters): unchanged. Q2 (next operation): unchanged — D213's strength.
**Q3 (external system/resource reached): FAILS today — this decision fixes
it with exact local evidence.** Q4 (exact code): already strong. Q5
(disconnected mechanisms): partially — the boundary producer makes the
model's boundary claims provable and grounds new ones.

## 2. Deferred list (explicit, recorded)

- Message publish/consume operations
- Caches and locks
- Configuration, secrets and OS boundaries
- Overview boundary canvas rendering (typed boundary → canvas upgrade)
- Rejected-candidate auditability in the D213 adjudication result artifact
  (red-team P2-4: the 3 rejected of 12 live only in the 482KB request
  artifact, not the result — separate auditability decision)
- D213 seed-pack omission surfacing on the wire (P1-2 related: producer must
  not inherit it; fixing the D213 seed wire is a separate corrective)

## 3. Acceptance criteria (M1–M9)

M1: Atlas gains typed resource/boundary entities with exact evidence on
casdoor offline run. M2: Study shelf still renders (D213 re-based). M3:
provider-free gates green. M4: no silent truncation in boundary evidence. M5:
no import-graph leak to any wire. M6: no legacy adapter/migration. M7: one
live Casdoor A/B shows observable boundaries + Study cites them. M8:
EN/RU both render. M9: manifest/report gate passes.
