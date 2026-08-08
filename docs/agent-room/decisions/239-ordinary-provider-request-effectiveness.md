# 239 — Ordinary provider-request effectiveness

**Status:** ACTIVE (owner-authorized, 2026-08-08)
**Supersedes:** no product contract. It authorizes a bounded investigation and
only the low-hanging corrective changes supported by its evidence.
**Preserves:** Decisions 235–238, one ordinary product surface, local-first
authority, request-local opaque refs, bounded provider payloads, no raw source
or repository tree/edge dump, exact fallback, Study continuation, historical
artifact replay, and offline behavior.

## 1. Product surface and investigation corpus

The only product workflow under review is the ordinary repository command,
using the remote-specific URL flag that matches the checkout:

`repomap --github-url https://github.com --no-secrets --lang ru`

For a GitLab checkout the same ordinary workflow uses
`--gitlab-url https://gitlab.com`; the GitHub spelling above is an example,
not a distinct or exclusive product surface.

Operational cache clearing and selection of the exact clean binary do not
create another product mode. No task file, experimental command, alternate
semantic pipeline, or hidden feature flag may substitute for this workflow.

Run the exact updated binary with caches cleared on six deliberately different
repositories from the local Go corpus:

- sqlc — strongest known pipeline-failure candidate;
- Gotify — strongest known Map-quality failure;
- Maddy — many facts with weak conceptual understanding;
- PocketBase — likely disagreement between Repomap subsystems;
- Dive — package-tree versus architecture stress case;
- goargs — small negative/control fixture.

Runs may execute concurrently; the owner explicitly authorizes provider calls
and up to seven parallel runs. Each observation is one fresh run, not a retry
until a preferred stochastic answer appears. After corrective work, fresh etcd
and casdoor runs are the final regression gate.

## 2. Required review of every emitted LLM request

For every provider call actually reached by the ordinary command, retain and
inspect the exact safe request, exact response, stage status, semantic exchange,
local input artifacts, and final published projection. Answer with evidence:

1. What exact facts and instructions are sent?
2. Which candidates or fields are filtered, ranked, truncated, summarized, or
   excluded, and on what deterministic grounds?
3. Which already-available local facts are omitted even though a bounded safe
   projection could materially improve the model's task? Opaque IDs alone are
   not treated as useful context.
4. Which output requirements make the model perform deterministic work that the
   backend can do more reliably: deduplication, ordering, ranking, joining,
   identity restoration, enumeration, normalization, or formatting?
5. Could a small deterministic preparation step or a simpler earlier semantic
   question provide materially better bounded context without a broad new
   pipeline or privacy expansion?
6. How is the response decoded, validated, salvaged, cached, and assembled into
   the product? Every hard validation must justify why rejection protects truth
   rather than merely enforcing backend-convenient shape.

Terminal success is not acceptance. Inspect real groupings, themes, summaries,
evidence associations, dropped candidates, local remainders, and disagreement
between stages. Durable diagnostics must make future recurrence diagnosable
without ad-hoc `jq` archaeology, while never persisting provider prose, raw
source, paths, endpoints, credentials, or arbitrary error strings in status or
console metadata.

**Owner clarification (2026-08-08):** operational failures, partial local
projections, and their closed aggregate reasons must explain themselves in the
ordinary console, using `WARN` when product evidence is lost. `report.json` and
`report.html` are user-facing product documentation, not debug containers; do
not add diagnostic-only counters or failure archaeology to the report merely
to make a later investigation easier. A useful diagnostic must remove the need
to open JSON with `jq`, not move the same archaeology to a more convenient
field.

The first systemic Map question is the relationship between the three existing
connectivity stores: `structural_facts`, `structural_edges`, and
`architecture_canvas.entry_handoff_groups[].entry_handoffs`. Establish their
exact semantics, producers, dependencies, and consumers; account for why their
counts may diverge; and verify whether Map selects connectivity from the correct
authoritative layer. Entry handoffs are Entrypoints context, not Landscape
connectivity or complete Mechanisms. Do not assume that sparse Map edges mean
analysis is absent when richer bounded connectivity already exists in another
retained projection.

## 3. Candidate selection and corrective threshold

Rank findings by expected product impact, recurrence across repositories,
implementation simplicity, and strength of a provider-free regression. Test the
simplest high-impact hypotheses first. A production change is allowed only when
the real artifacts identify a root cause and a focused regression can express
the desired contract.

Permitted low-hanging corrections are limited to:

- deterministic candidate preparation, bounded safe context, or truthful
  omission/count logging;
- simplifying the prompt or response grammar and restoring identities locally;
- moving deterministic deduplication/ranking/joining/formatting to the backend;
- relaxing or replacing a hard validation when local authority can retain truth
  through normalization, item-local rejection, remainder, or explicit frontier;
- adding permanent closed diagnostics useful for the same failure class on any
  repository.

No new product flags, shell scripts, one-off debugging utilities, broad model
call, cosmetic-only change, retry loop, repo-specific keyword table, raw-data
dump, or request-bound/privacy expansion is approved. A new production semantic
call requires separate evidence that deterministic preparation and simpler use
of existing calls cannot answer the need; D239 does not approve one by default.

## 4. Verification and final gate

- build one exact clean candidate with `go build -trimpath` and record build
  identity;
- clear semantic caches before each investigation/final batch and run only the
  ordinary command;
- verify process exit, manifest, Atlas, report JSON/HTML, every stage status,
  every semantic exchange, request/response hashes and bytes, and product
  projection for each run;
- compare request candidates with local facts and account for every omission,
  truncation, rejection, normalization, partial result, and fallback;
- add focused provider-free regressions before changing a contract;
- run focused tests, full `go test -count=1 ./...`, `go vet ./...`, and
  `git diff --check`;
- build a new exact clean candidate after corrections;
- clear caches and run fresh ordinary-command acceptance on etcd and casdoor,
  inspecting all model calls and final reports for regressions;
- commit no debug artifacts, provider payloads, binaries, credentials, or
  corpus-specific paths.
