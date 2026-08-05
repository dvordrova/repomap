# D215 acceptance record (2026-08-05)

Decision: docs/agent-room/decisions/215-etcd-architecture-output-exhaustion-isolation.md
CURRENT.md: points at D215 (active)

## Root cause (exact evidence)

- Run `20260805-064730-etcd-4d99f0f8a558`, request SHA
  `00cb222a...`, retained response SHA `be4dc6cc...`.
- Model `deepseek-v4-flash`, JSON-object mode, `thinking: disabled`,
  input 42,197 tokens, output 64,000 tokens, `finish_reason=length`,
  201,396 visible bytes.
- Visible generation repetition loop: one subsystem, one component, open
  `member_refs`, 6,390 package-ref objects, 90 unique refs, one exact
  56-ref / 1,761-byte block repeated 113 full times + partial 114th, no
  closing JSON. First duplicate already made the proposal invalid.
- NOT capacity shortage → the 64,000 global ceiling is retained (no 128k,
  no 8,192 stage cap). NOT hidden reasoning (thinking disabled). D214
  entities were not members of the Architecture request.

## What shipped (D215, narrow D194 delta)

- Typed `architectureOutputResourceExhausted` failure (preserves the
  underlying ResourceLimitError). Publishable only when the provider call was
  attempted, kind is output tokens or response bytes, cancellation is off,
  no accepted record/cache exists, and failed status + accounting are durable.
- Architecture status version 8→9: `state=failed`, closed
  `error_code=provider_output_limit`, exact request bytes, known partial
  response bytes, provider_request_count=1, transport attempts,
  usage_reported, input/output tokens, configured_max_tokens,
  observed_output_tokens, `finish_reason=length`, `response_complete=false`,
  local/requested/structural/anchor counts. No proposal, no membership, no
  model source, no fallback, no provider prose.
- Exact failed-call model-research accounting once (`resource_limited`,
  semantic calls +1, request/response bytes, latency, tokens). Pre-call
  rejections never counted.
- D192: one redacted exchange preserved; no second exchange; no read-back.
- Continuation: repository authority reconciliation, Theme Scout, local
  expansion, Theme Adjudication, report/manifest generation, static export,
  serving/opening. Visible Architecture is the canonical local Canvas.
- Localized EN/RU disclosure, compact and secondary:
  EN "Conceptual grouping is unavailable because the model exceeded its
  response budget. The partial response was not used; exact local
  Architecture remains available."
  RU «Концептуальная группировка недоступна: модель исчерпала лимит
  ответа. Частичный ответ не использован; точная локальная архитектура
  остаётся доступна.»
- Hard safety: no `architecture_synthesis.json`, no accepted cache write, no
  response-prefix parsing, no membership recovery, no semantic retry, no
  later model Architecture claim, local Canvas unchanged.

## Verification (all real)

- `go test ./...` 74/74 packages; `go vet ./...` clean; `make build` clean;
  `node --check` on touched JS assets; golden HTML regenerated.
- New tests: v9 status validation (valid length shape, byte-overflow shape,
  partial-response rejection, v8 rejects v9 fields), resource-limit stage
  outcome (failed status + accounting once, no record/cache), one-redacted
  exchange, classification (publishable only after durable status+accounting,
  status-write failure terminal, accounting-write failure terminal,
  cancellation terminal, pre-call limit terminal), full CLI continuation
  through both D213 stages + report, and the etcd acceptance replay.
- etcd provider-free replay (real `/Users/dvordrova/git/etcd` repo, real
  CLI, deterministic generated loop fixture, no network): Architecture
  output exhaustion → failed status durable (provider_output_limit, length,
  64k/64k) → no synthesis record → Study executes and accepts → report.json +
  report.html + manifest publish → exit 0. Canonical local Canvas bound in
  report.json.

## Live run

At most one fresh live etcd run is authorized if credentials are available
(owner zsh session). Not performed in this acceptance record.
