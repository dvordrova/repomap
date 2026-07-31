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

## Checkpoint B2 handoff

```text
Checkpoint: B2 — provider-free Architecture Canvas adapter
Changed contract: Decision 162 adapts only validated Architecture Canvas subsystem/component names and descriptions; ordinary CLI, providers, caches, persisted artifacts, reports, manifests, HTTP, and browser behavior remain unchanged
J0/J1 evidence: English identity preserves both the complete serialized Canvas and the containing report.json byte-for-byte; a decoded current canonical/input/projection round trip remains exact
Focused tests: go test ./internal/localization -count=1, go test ./internal/report -run ArchitectureLocalization -count=1, focused race tests, focused vet, and git diff --check passed
Provider-free replay: a supplied Russian projection changed only the four allowlisted Architecture prose fields; structured symbols, paths, packages, flow/surface/investigation/anchor/source IDs, Unicode, and CJK text were restored byte-for-byte
Failure behavior: a stale canonical artifact with matching owner IDs is rejected before mutation; invalid individual translations retain canonical English only for that field, with deterministic diagnostics
Adversarial review: passed after binding canonical/input bytes to the current Canvas, preserving the prior subsystem why_it_matters allowlist, completing typed structured protection, and documenting the validated ProjectArchitectureCanvas precondition
Full checks: scripts/check.sh passed, including go test, go vet, and six offline quality tasks, real 47.30s
UI actions actually performed: none; the adapter has no production consumer
Not run and why: no provider call, no external repository, no etcd run, no product binary rebuild, and no EN->RU->EN round trip; no ordinary product path or locale-projection cache changed
Remaining blocker or next checkpoint: persisted canonical/input/identity sidecars require a new numbered decision and a root-confined atomic writer; they must remain non-consumable until bound to verified run state
```

## Checkpoint B3 handoff

```text
Checkpoint: B3 — explicit provider-free Architecture English identity artifacts
Changed contract: Decision 163 adds `make localization-check RUN=<run-dir>` and `repomap dev localization-check <run-dir>` as explicit developer operations; ordinary CLI generation, providers, caches, persisted report artifacts, reports, manifests, HTTP, source authority, and browser behavior remain unchanged
Artifact scope: exactly localization/architecture.canonical.v1.json, localization/architecture.en.input.v1.json, and localization/architecture.en.projection.v1.json; these files cover only allowlisted Architecture Canvas prose and are deliberately non-consumable
Draft-plan departure: whole-report-shaped semantic-report.canonical.json, localization/input.v1.json, and localization/en.projection.json are not written because whole-report semantic ownership and cache authority have not been proven
Eligibility: explicit current v3 output_language=en, accepted or accepted_with_normalization non-fallback synthesis; matching successful/cached accepted v2 status; consistent synthesis/status/canvas metadata; replay against current saved facts; exact English identity replay
Failure behavior: all eligibility, decoding, replay, metadata, language, fallback, secret, bound, symlink, existing-file, and installation conflicts fail closed before publishing a usable partial set
Persistence boundary: fixed root-confined paths, 0700 localization directory, 0600 files, bounded input/output, pre-write secret scan, synced temporary files, complete-set installation, rollback on partial install, and refusal to overwrite conflicting owner data
J0/J1 evidence: both accepted and accepted-with-normalization saved-run shapes produced the exact three-file set; repeated materialization preserved every artifact byte, English identity replay preserved Architecture Canvas bytes, and unrelated report/HTML/manifest/synthesis bytes remained unchanged
Focused tests: make localization-check, go test ./cmd/repomap -run LocalizationCheck -count=1, focused race tests across internal/report and cmd/repomap, focused vet, and git diff --check passed
Provider-free replay: strict saved-status and synthesis replay rebuilt the current Architecture Canvas and applied the persisted English identity projection with zero diagnostics or fallback
Adversarial review: no remaining blockers after passing exact bounded status/synthesis bytes into run replay; root confinement, bounds, strict decoding, stale-facts rejection, symlink refusal, deep secret scanning, exact-set idempotence, conflict refusal, and rollback were reviewed
Full checks: scripts/check.sh passed, including go test, go vet, and six offline quality tasks, real 44.56s; the provider-free etcd snapshot/bundle check also passed, real 17.35s
Commit: this handoff is committed atomically with the B3 implementation
UI actions actually performed: none; these files have no product consumer
Not run and why: no provider call, no Russian projection, no cache read/write, and no EN->RU->EN round trip; B3 is English identity persistence only. The etcd repository was read only by the required provider-free snapshot/bundle check
Remaining blocker or next checkpoint: bind a complete, explicitly scoped semantic artifact to verified run state before any locale projection cache or product consumer is authorized
```

## Checkpoint B4 handoff

```text
Checkpoint: B4 — explicit provider-free Architecture Russian projection replay
Changed contract: Decision 164 adds `make localization-replay RUN=<run-dir> PROJECTION=<projection.json>` and `repomap dev localization-replay <run-dir> <projection.json>` as explicit developer operations; ordinary runs, providers, caches, persisted artifacts, reports, manifests, HTTP, source authority, and browser behavior remain unchanged
Fixture evidence: fixed genuine-Russian projection SHA-256 18a610ea92407f25b4b4429ae51bc371fd0d7b42e2237fa88aeeed024fe36203 is bound to canonical English Architecture SHA-256 eef1c023de5c3c81cf1e992be2dbd22b9f100e847f21a4cece8294b792841771
J0/J1 evidence: accepted and accepted-with-normalization run shapes replayed successfully; identical input produced byte-identical compact JSON, the complete run directory remained byte-identical, protected terms were restored exactly, and resetting only subsystem/component names and descriptions made the complete projected Canvas byte-identical to canonical English
Failure behavior: projection envelope mismatch retains the complete canonical English Canvas atomically; a missing or placeholder-invalid translation retains canonical English for exactly that field while valid Russian fields survive; malformed, unknown, trailing, invalid-UTF-8, oversize, symlink, and credential-like projection input fails closed without echoing its value
Safety boundary: projection bytes are bounded before decode; translation cardinality and IDs are bounded before sort/regex work; the complete replay graph is scalar-scanned and conservatively size-counted before `json.Marshal`; the final encoded bytes are bounded and secret-scanned again; Make stdout contains exactly the replay JSON plus one newline
Adversarial review: initial review found Make recipe leakage, JSON-escape secret-scan bypass, and post-allocation output bounding; all three were fixed and the repeat review passed with no B3 regression, provider, cache, write, or RU-to-EN path
Focused tests: make localization-check, focused internal/report and cmd/repomap tests, focused race tests, focused vet, and git diff --check passed
Full checks: scripts/check.sh passed, including go test, go vet, and six offline quality tasks, real 42.72s; the provider-free etcd snapshot/bundle check also passed, real 16.26s
Commit: this handoff is committed atomically with the B4 implementation
UI actions actually performed: none; replay JSON has no product consumer or browser surface
Not run and why: no provider call, no product binary rebuild, no cache read/write, no ordinary `--lang ru` run, and no RU-to-EN round trip; the only external repository access was the required provider-free etcd snapshot/bundle check
Residual limitation: a same-user replacement race remains possible between projection-file Lstat and Open; both pre-open and opened-file bounds/type checks remain in place, and this explicit dev-only replay does not treat the fixture as cache authority
Remaining blocker or next checkpoint: production Russian output still requires a separately approved localization provider stage, exact projection cache, compatibility boundary, and typed UI message catalog
```
