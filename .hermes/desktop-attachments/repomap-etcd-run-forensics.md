# Repomap etcd run — evidence-backed Architecture failure analysis

## Evidence identity

- Run archive: `20260805-064730-etcd-4d99f0f8a558.zip`
- Archive SHA-256: `9a8238a9331d8a7f8f97c2d0f617155ec147d4de97a9fcec5a13cf2f85c1beea`
- Repomap `main` reviewed at: `36c75a7a92e70fa7607cb2a3d6d32b4327ed25f7`
- Run ID: `20260805-064730-etcd-4d99f0f8a558`
- Repository: `go.etcd.io/etcd/v3`
- etcd revision label in the run: `4d99f0f8a558`
- Report language: Russian
- Architecture exact request SHA-256:
  `00cb222adab0076fb8bdd346beb742f0be7e776f4a0118e673831ec427c905ab`
- Architecture retained response SHA-256:
  `be4dc6ccaf09dfa82904797301a16680e92d27a119a2348603b76cf0f6b38475`

The raw run remains diagnostic input only. Do not commit it to the repository.

## Proven root cause

This failure is a **visible model generation repetition loop inside one
`member_refs` array**.

The exact request did all of the following correctly:

- model: `deepseek-v4-flash`;
- `response_format: {"type":"json_object"}`;
- `thinking: {"type":"disabled"}`;
- `temperature: 0.1`;
- `max_tokens: 64000`.

Therefore the earlier hidden-reasoning/provider-profile hypothesis is
eliminated for this incident.

The exact response:

1. begins as the requested JSON object;
2. emits one subsystem, “Серверное ядро”;
3. emits one component, “Сервер etcd”;
4. opens that component's `member_refs`;
5. emits only package refs;
6. never closes `member_refs`, never emits `anchor_refs` or `hypothesis`, never
   emits a second component, and never closes the root object.

Measured response shape:

- retained visible response: **201,396 bytes**;
- typed package-ref objects: **6,390**;
- unique refs used: **90**;
- unknown refs: **0**;
- symbol refs used: **0**, despite 68 supplied symbol candidates;
- first duplicate appears at item 91;
- after 34 initial package refs, one exact block of **56 refs / 1,761 bytes**
  repeats **113 complete times**, followed by a partial 114th repetition;
- JSON parse failure: unterminated string near the final byte;
- provider evidence: `finish_reason=length`,
  `input_tokens=42197`, `output_tokens=64000`.

The model violated an already-explicit instruction not to repeat a member
within one component. More prompt emphasis is not an adequate root correction.

## Exact Architecture input pressure

The exact embedded Architecture request contained:

| Collection | Count | Compact JSON bytes |
|---|---:|---:|
| conceptual candidates | 251 | 34,972 |
| package candidates | 183 | 23,740 |
| symbol candidates | 68 | 11,218 |
| required member checklist | 251 | 7,805 |
| behavior anchors | 57 | 10,762 |
| structural file locators | 53 | 8,354 |
| supporting relations | 635 | 77,322 |
| package-import relations | 613 | 74,638 |
| behavior-handoff relations | 22 | 2,660 |

The provider-visible candidate payload was about 139 KB. Raw package-import
relations alone were more than half of it.

This does not prove that 42k input tokens alone always cause a loop, but it
does prove that the current task is a difficult one-shot global partition:
251 raw members plus 635 relations, with model-owned many-to-many membership.

## D214 did not cause the Architecture request expansion

The same etcd run produced a substantial local Repository Atlas:

- 215 units;
- 96 entities:
  - 28 boundary;
  - 32 resource;
  - 18 operation;
  - 18 surface;
- 164 observations;
- 253 evidence records;
- 18 relations.

However, the Architecture synthesis request contains only package and symbol
conceptual candidates. It contains no Atlas `resource` refs and no D214
boundary/resource entity membership catalog. D214 therefore did **not** cause
the 251-member Architecture request or the response loop.

## What is good and weak in D214 data

### Good

- Every observed call site has exact path, line, column, and enclosing symbol.
- There are 64 unique boundary call-site witnesses.
- Each witness is attached both to one boundary observation and one resource
  observation.
- The evidence distinguishes local source locations rather than merely imports.

### Weak / not yet product-ready

1. **“Resource” is currently a library family, not necessarily an external
   system identity.** In this run the evidence detail values are primarily
   `net/http`, `os`, and `google.golang.org/grpc`. That proves a call pattern,
   not a specific endpoint, database, bucket, WAL directory, or reached system.

2. **Boundary → resource is not an explicit Atlas relation.** The run has only
   `surface --exposes--> operation` relations. The 32 boundary/resource pairs
   can be reconstructed because their observations share the same evidence
   ref, but the relationship is implicit.

3. **Evidence roles are mixed.** Of the 64 unique boundary call sites:
   - 39 are production/other;
   - 18 are tests/integration;
   - 7 are contrib/tooling.

   A default Overview or Study shelf must not present all three groups with the
   same weight.

4. **Entities themselves are opaque IDs plus `unit_id`.** The useful semantics
   live in evidence provenance. A report projection must compile a truthful
   human label and role rather than exposing raw entities directly.

Recommended visible wording:

> Observed boundary call pattern

not:

> External system reached

until stronger evidence exists.

## Why the run aborted completely

The run successfully persisted Navigator and the local Atlas, then stopped on
Architecture. It contains:

- no `architecture_synthesis.json`;
- no `architecture_synthesis_status.json`;
- no Theme Scout artifacts;
- no Theme Adjudication artifacts;
- no Study themes;
- no `report.json`;
- no `report.html`;
- no manifest.

This is consistent with Decision 194: every typed semantic resource limit
terminates the whole ordinary run. The no-parse/no-cache rule is correct; the
whole-product termination rule is now too broad for optional Architecture
enrichment because the exact local Canvas already exists independently.

## Corrective sequence

### D215 — failure isolation, first

D215 should narrowly supersede only the whole-run termination clause of D194
for an **attempted Architecture output/response resource exhaustion**.

Keep all safety rules:

- one semantic call;
- no retry;
- no truncated JSON salvage;
- no synthesis record;
- no accepted cache;
- no partial membership;
- no model Architecture source.

Add:

- durable failed Architecture status with bounded resource evidence;
- exact failed-call accounting;
- publishable optional-stage classification only after status and accounting
  are durable;
- continue to Study and final report with canonical local Architecture;
- small localized warning.

Do **not** change the global 64,000-token contract in D215. A stage-specific
8,192 cap was suggested before the exact response and current membership
contract were inspected. It is not yet justified: the current accepted schema
allows up to 2,048 conceptual memberships and up to eight participations per
member. Pick a smaller output envelope only after the response contract is
made compact enough to prove that every legal answer fits.

### D216 — root Architecture correction, second

Replace raw-member global partitioning with locally compiled Architecture
units:

```text
251 exact package/symbol members
+ 613 package-import relations
+ behavior anchors
        ↓ local deterministic compiler
bounded Architecture units (target 24–64)
+ aggregated unit relations
+ representative labels / counts / exact private expansion map
        ↓ one model call
subsystems + components containing unit refs
        ↓ local validation and expansion
exact member memberships + local unclassified remainder
```

The local compiler should use:

- package/module and top-level path structure;
- exact package graph communities;
- process-entry and behavior-anchor neighborhoods;
- test/tooling/production role separation;
- symbols attached to their exact owning package/unit;
- D214 boundaries/resources as overlays or unit summaries, not raw new members.

The etcd package graph already demonstrates feasibility: simple provider-free
community analysis over the 183 package nodes and 613 import edges yields five
large meaningful regions (roughly client/integration, server/storage, raft/WAL,
e2e/robustness, and test utilities) plus bounded smaller islands. Production
code should use a deterministic repository-aware compiler, not this exploratory
algorithm verbatim.

The model should group `u*` unit refs, not enumerate 251 raw member refs.
Backend expansion restores exact members. Then the legal maximum output can be
computed and a smaller stage envelope can be justified.

### UI/UX after D215

Use the completed etcd report as an additional real fixture. Show:

- the Architecture fallback warning as secondary provenance;
- exact local Architecture even when conceptual grouping failed;
- D214 evidence split into production, test, and tooling;
- “observed call pattern” rather than “resource reached”;
- collapsed raw diagnostics;
- no mobile horizontal overflow.

## Rejected fixes

Do not:

- increase 64k to 128k;
- retry the same request;
- repair or parse the truncated prefix;
- blame hidden reasoning (it was explicitly disabled);
- blame D214 boundary/resource entities (they were not in the Architecture
  membership wire);
- merely add stronger “do not repeat” prose;
- silently drop candidates or relations;
- turn every semantic resource limit in every stage into a warning.

## Acceptance target

After D215, the same response loop may still occur, but the product outcome
must be:

```text
Architecture model enrichment: failed/resource-limited
partial response: diagnostic only
exact local Architecture: available
Study: executed
report + manifest: published
command: successful
warning: truthful and secondary
```

After D216, etcd should normally produce a complete bounded conceptual
Architecture without enumerating raw members.
