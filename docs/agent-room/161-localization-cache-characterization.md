# Decision 161 localization/cache characterization

## Scope

Baseline head: `f92b873` (`feat: add standalone GitHub reports`).

This is a provider-free characterization, not evidence that English and
Russian semantic results are equivalent. The two saved product runs are from
different repositories and are machine-local, so they are useful replay
fixtures but not CI authority.

## Current behavior

`--lang` reaches orientation, zero to two targeted-research rounds,
architecture synthesis, optional Guided Tour, the Study brief, Study direction
selection, and every accepted-direction review. It therefore changes semantic
request bytes throughout the current product.

The data-dependent ordinary call shape is:

```text
1 orientation
+ 0..2 targeted research
+ 1 architecture synthesis
+ 0..1 Guided Tour
+ 2 Study base calls
+ N Study reviews (N is the accepted direction count)
```

There is no single run-level record that accounts for all of those calls.
`metadata.json`, `model_research.json`, architecture status, and Study status
cover different subsets. `provider_request_count` is not a total stage count.

### Cache identity

| Cache | Current identity | Important gap |
|---|---|---|
| shared `.model-research` | repository context, stage, prompt, profile, model, evidence hash, policy, locale; exact request/evidence/response hashes are checked on load | endpoint identity is absent |
| architecture `.component-synthesis` | repository revision, contract/prompt, profile, model, locale, bounded synthesis request hash | endpoint is absent; hash is not the final locale-wrapped provider body |
| Study v3.2 | saved accepted attempts can be replayed | no cross-run cache |
| report UI | locale marker plus post-render English-string translation | no typed message catalog or locale-projection cache |

Invalid historical `.model-research` records are already treated as safe
misses on this head. The older draft-plan claim that they terminate the
ordinary stage is no longer true.

## Field ownership

Initial localization-eligible prose:

- repository/project description written by the model;
- subsystem/component names, responsibilities, and explanations;
- Guided Tour title, summary, step explanation, and gap explanation;
- Study brief prose;
- Study question, motivation, outcome, reading guidance, and search query;
- mechanism title, question, answer, and step explanation.

Always protected:

- repository paths, symbols, line/ranges, packages, modules, URLs;
- product/library/API/protocol names and code identifiers;
- opaque IDs, enum values, JSON keys, hashes, evidence and provenance;
- commands, code, source/document excerpts, and scenarios.

Warnings are mixed-origin and are not eligible wholesale. Static renderer copy
belongs in a typed local message catalog rather than model localization.

Stable field IDs may use existing component, direction, anchor, or artifact
IDs. High-level-map entries and Guided Tour steps still need an explicit
semantic owner ID before production extraction; array index and localized name
are not acceptable substitutes.

## Saved report replay

### English

Fixture:

```text
/Users/dvordrova/Library/Caches/repomap/runs/20260730-082222-chatto-362ae4811bd7
```

Current-head render:

| Artifact | Bytes | SHA-256 |
|---|---:|---|
| `report.json` | 919390 | `901d57d238a394259f3c63143a1b057c468f0d880bb58f91ed2fb85bc423277e` |
| `report.html` | 2792998 | `d675a0c3a3449d43fdcefaa76919250daeeedf6729849a5b79f2c132804b097d` |

The HTML declares `lang=en`.

### Russian

Fixture:

```text
/Users/dvordrova/Library/Caches/repomap/runs/20260730-110319-pglogrepl-5ae56d1362e2
```

Current-head render:

| Artifact | Bytes | SHA-256 |
|---|---:|---|
| `report.json` | 325039 | `ad3321defd59465c803175972c8fe88d63a4882b7097d7a42945a82fcd2a552f` |
| `report.html` | 2456782 | `da53fa442e249a43a63b26c9e273f457931ee2190afb5d54cdf988b1aee7ae8b` |

The HTML declares `lang=ru`.

The exact orientation request/raw response is not retained in either run.
Architecture and some targeted-research request hashes exist, while Study does
not retain a complete exact-request record. These saved runs therefore cannot
serve as the future unified stage manifest.

## Provider-free timing baseline

The host's default `go` launcher currently attempts an unavailable toolchain.
All measurements use the installed Go 1.26.4 SDK through a per-command `PATH`
prefix; no persistent PATH change was made.

### T0: focused locale/cache tests

```sh
PATH=/Users/dvordrova/sdk/go1.26.4/bin:$PATH \
  /usr/bin/time -p go test \
  ./internal/deepseek ./internal/modelresearch ./internal/orient \
  ./internal/report ./cmd/repomap \
  -run 'OutputLanguage|Cache|Language' -count=1
```

Result: pass, `real 7.69`.

### T1: saved report assembly replay

```sh
audit_dir=$(mktemp -d /private/tmp/repomap-replay.XXXXXX)
cp -R <fixture> "$audit_dir/run"
env -u DEEPSEEK_API_KEY -u DEEPSEEK_ENDPOINT -u DEEPSEEK_MODEL \
  -u REPOMAP_LLM_API_KEY -u REPOMAP_LLM_ENDPOINT \
  -u REPOMAP_LLM_MODEL GOPROXY=off \
  PATH="$PWD/tmp/d158-tests/bin:/bin:/usr/bin:/usr/sbin:/sbin:/usr/local/bin" \
  go run ./cmd/repomap dev render-report "$audit_dir/run"
shasum -a 256 "$audit_dir/run/report.json" "$audit_dir/run/report.html"
```

Result: pass; repeated English replay was byte-identical, warm render-only
`2.51s`. This proves report assembly only, not semantic-stage replay.

### T2: full repository check

```sh
PATH=/Users/dvordrova/sdk/go1.26.4/bin:$PATH \
  /usr/bin/time -p ./scripts/check.sh
```

Result: pass, including six offline quality tasks, `real 101.63`.

## Python WIP provenance

The dirty root checkout is not an unexplained concurrent rewrite. The intended
D145 Python-focus slice is preserved by stash commit:

```text
27278d9c716c2a3fb2f2a55eac133da4a2231c4f
safety: preserve uncommitted D145 before 0c22a0d fast-forward
```

It must later be ported manually, not by applying the stash wholesale. The
later candidate-scoped `documentSymbol` experiment selected
`get_plugin_names:365` instead of the expected `_get_plugin:406` and is not
production input. Decision 140 and Caddy semantic-map files captured beside the
stash are unrelated debris. The dirty root checkout and stash remain
untouched.

## Checkpoint A handoff

```text
Checkpoint: A — authority and characterization
Changed contract: none; documentation only
J0/J1 evidence: exact saved render hashes above; no cross-repository semantic comparison claimed
Focused tests: T0 passed in 7.69s
Provider-free replays: English and Russian saved report assembly passed; English repeat byte-identical
Full checks: scripts/check.sh passed in 101.63s
UI actions actually performed: none in this checkpoint
Not run and why: no provider call, no external repo, no EN->RU->EN cache round trip; projection cache does not exist
Remaining blocker or next checkpoint: isolated provider-free localization contract; production wiring remains held
```

## Checkpoint B1 handoff

```text
Checkpoint: B1 — isolated canonical/localization contract
Changed contract: added an unconnected internal/localization package and one focused Make target; ordinary CLI, caches, reports, and browser behavior are unchanged
J0/J1 evidence: fixture canonical SHA-256 0916b6136334691462d740449331cbe8596a765178529aeabf0f43aac1197529; repeated canonical bytes and identity English projection are exact
Focused tests: make localization-check, go test -race ./internal/localization -count=1, go vet ./internal/localization, and git diff --check passed
Provider-free replay: supplied Russian projection changed only allowlisted prose; typed protected terms were restored byte-for-byte; invalid fields fell back to canonical English
Adversarial review: passed after bounding projection cardinality, IDs, and translated bytes before sorting or regex processing; canonical placeholder metadata is re-derived before its SHA is trusted
Full checks: scripts/check.sh passed, including go test, go vet, and six offline quality tasks
UI actions actually performed: none; this package has no production consumer
Not run and why: no provider call, no external repository, no etcd run, and no EN->RU->EN round trip; no product path or locale-projection cache changed
Remaining blocker or next checkpoint: a new numbered decision is required before adapting one real model-authored artifact into canonical English plus a Russian projection
```
