# Decision: Chatto End-to-End Acceptance Run

Status: Completed verification. Approved and reviewed by the product
supervisor and repository owner in the current session.

## Baseline and purpose

The verification baseline is commit
`dd460eb323680ea5c2d6d9f6232fdf4226df8d3b`. The stable PATH binary must be:

- path: `/Users/dvordrova/git/repomap/.bin/repomap`
- SHA-256:
  `7050d5e4fa3548019f0284cb4ef71f95c1a8863ca61f6751fe5b606804b1f2f2`

Decisions 130 through 132 corrected and provider-free verified the missing
Chatto topic projection, unconditional normal-report Search removal, and the
retained repository Brief `domain_terms` placement rejection. A fresh normal
Chatto run is now required to verify orchestration, later Study Map stages,
authority-bound serving, and the actual product surface.

## Authorized invocation

Record the Chatto revision and dirty state, then run exactly once from:

`/Users/dvordrova/git/chatto`

```sh
repomap "$PWD"
```

The command must resolve to the stable binary above. This is one normal
provider-bearing invocation under existing stage timeouts. It is not a replay
and may not use `go -C`, another repomap checkout, a retry, timeout inflation,
or a second repository.

## Evidence to collect

Preserve:

- full stdout and stderr;
- exact debug run path;
- total and per-stage latency;
- request, response, and token metrics;
- every rejection, downgrade, warning, and fallback with exact stage/reason;
- whether expensive later stages continue after a decisive failure;
- exact report publication and authority-bound serving outcome;
- attempt and status artifacts for failed or reduced stages; and
- a comparison with saved run
  `20260728-234147-chatto-3da86b716518`.

Product evidence must establish:

1. discovery is attempted and produces concrete mechanisms or topics;
2. Study Map passes the corrected Brief decoder and records its later-stage
   outcome;
3. Overview exposes no Search for the actual shelf outcome;
4. one useful item opens within two clicks with exact starting symbols;
5. an authorized source-open succeeds; and
6. screenshots capture Overview, one useful detail, Study when published, and
   suspicious Run notes.

## Authorized file budget

Decision activation and result recording are limited to:

- `docs/agent-room/decisions/133-chatto-end-to-end-acceptance-run.md`
- `docs/agent-room/CURRENT.md`

Ignored logs and screenshots may be written beneath `tmp/chatto-d133/`. The
normal repomap command may write its ordinary debug run and report artifacts
to the configured user cache. No production or test source change is
authorized.

## Stop conditions

After the one invocation reaches terminal publication or failure, stop for
supervisor review. Do not implement a corrective, retry, replay, run another
repository, change a timeout, prompt, schema, gate, renderer, source authority,
or UI behavior. Stop immediately on binary/hash mismatch or if the command
would use a different checkout.

## Completion condition

This decision completes only after exact artifact/log inspection, browser
evidence, screenshot delivery, comparison with the prior Chatto run, and
concrete product-supervisor review. It authorizes no subsequent corrective or
target run.

## Completed verification result

The stable binary path and SHA-256 matched, and the clean Chatto revision was
`e81f585f147eaeafaf8a3b226e28c2599c3bdb2c`. The one authorized invocation
saved run `20260729-030442-chatto-d18b2df079a9`. Its Overview exposed exactly
three grounded incomplete topics and no Search. A topic opened in one click,
and its authorized exact source opened in the second click. Architecture
remained available.

The accepted Brief proved Decision 132. The later Study candidate call took
113,220 milliseconds and returned eleven ordered candidates, but the stage
failed after 137,354 milliseconds because positions 8 and 10 contained only
two anchors. Nine siblings satisfied the unchanged three-to-five-anchor rule.
The atomic decoder stopped at the first invalid item and discarded the whole
collection. This attributable failure was reviewed by the product supervisor
and became Decision 134; no retry occurred under this decision.

The full log is retained beneath `tmp/chatto-d133/run.log`, and the Overview,
topic-detail, and source-open screenshots are retained beside it. The run
continued to terminal report publication with a coherent code-first topic
fallback rather than a richer Study page.
