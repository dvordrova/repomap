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

## Checkpoint B5 handoff

```text
Checkpoint: B5 — provider-free Architecture localization stage replay
Changed contract: Decision 165 adds `make localization-stage RUN=<run-dir> [RESPONSE=<projection.json>]` and `repomap dev localization-stage <run-dir> [<projection.json>]` as explicit developer operations; ordinary runs, `--lang`, providers, caches, persisted artifacts, reports, manifests, HTTP, source authority, and browser behavior remain unchanged
Direction: exactly canonical English presentation prose to Russian projection; no Russian-to-English translation or semantic round trip exists
Prompt identity: generic contract prompt SHA-256 e399c596ef763d1407d12d108b6168be07503f0d398eab181eedcb67e35a59a7; fixed Architecture saved-run prompt SHA-256 da4a36c6cb0f036bffbad49cc082a92c0fe7c05226631342c03287d3eefb28e4; prompt version localization-projection-json-v1
Prompt boundary: the stage re-derives current eligible canonical/input data, includes only allowlisted prose plus typed placeholders, caps raw collection cardinality and scalar bytes before whole-input validation or marshal, emits deterministic compact JSON bounded to 1 MiB, and secret-scans typed input and encoded bytes before the provider seam
Saved/fake response replay: the unexported injected seam is invoked exactly once; the public adapter reads one explicitly supplied bounded regular local file and makes no network call; accepted bytes are identical to direct Decision 164 replay and the complete run directory remains byte-identical
Failure behavior: cancellation before the seam prevents the call and cancellation immediately after it prevents response processing; provider errors are sanitized; malformed, unknown, trailing, invalid-UTF-8, oversize, symlink, and credential-like output fails closed with no retry; escaped credential-shaped unknown JSON field names cannot leak through strict-decoder errors
Field behavior: envelope mismatch and individual field fallback remain exactly the Decision 164 contract; ordinary in-bound Russian fixture output is byte-identical to direct replay
Focused tests: make localization-check, focused internal/localization, internal/report, and cmd/repomap tests, focused race tests, focused vet, and git diff --check passed
Adversarial review: scope and test-gap reviews found no blocker; security review found a decoded unknown-field error leak, which was fixed with sanitized strict-JSON errors and escaped-field regressions; a secondary pre-validation allocation concern was fixed with raw prompt preflight
Full checks: scripts/check.sh passed, including go test, go vet, and six offline quality tasks, real 25.70s; the provider-free etcd snapshot/bundle check also passed, real 16.91s
Commit: this handoff is committed atomically with the B5 implementation
UI actions actually performed: none; the prompt and replay JSON have no product consumer or browser surface
Not run and why: no live provider, network call, external model, product binary rebuild, cache read/write, ordinary `--lang ru` run, UI localization, or Russian-to-English path; the only external repository access was the required provider-free etcd snapshot/bundle check
Remaining blocker or next checkpoint: an exact projection record/cache needs a separately approved identity binding that includes the exact prompt plus provider/model/endpoint/generation parameters; it must remain provider-free and non-product until its own gate is green
```

## Checkpoint B6 handoff

```text
Checkpoint: B6 — immutable provider-free Architecture localization projection record
Changed contract: Decision 166 adds `make localization-record RUN=<run-dir> [RESPONSE=<projection.json>]` and `repomap dev localization-record <run-dir> [<projection.json>]` as explicit developer operations; ordinary runs, `--lang`, live providers, retries, shared caches, reports, manifests, HTTP, source authority, and browser behavior remain unchanged
Direction: exactly canonical English Architecture presentation prose to one complete accepted Russian projection; there is no Russian-to-English request or round trip
Request identity: localization-openai-request-v1 binds the exact Decision 165 System/User messages, canonical endpoint, auth mode without credentials, model, temperature, max tokens, JSON response mode, DeepSeek thinking mode, reasoning effort, exact request bytes, prompt/input/canonical hashes, projector/schema versions, and locale direction
Bounds: provider identity scalars are capped at 4 KiB before parsing; exact provider request bytes use one shared 2 MiB builder/record limit; normalized projection remains capped at 1 MiB; canonical encoded record/result is capped at 5 MiB, including base64 expansion
Record path: `.localization-projections/v1/architecture-<64 lowercase hex>.json`; lookup-only miss writes nothing; accepted first publication is immutable and no-replace; an exact hit does not read the optional response and reruns current Decision 164 replay before returning
Filesystem boundary: the real run-directory identity and nested record-directory identities are pinned with `os.Root` and `os.SameFile`; fixed root-confined paths refuse symlinks/non-regular files; bounded reads verify the same leaf before and after open; writes use 0600 random O_EXCL temporary files, file sync, hard-link no-replace publication, cleanup, and directory sync; concurrent same-key writers leave one validated winner
Failure behavior: corrupt, noncanonical, duplicate-key, tampered, signed-zero-alias, stale, secret-bearing, symlinked, oversized, or fallback-bearing expected content fails closed and is not replaced; cancellation before lookup, after response, and before the no-replace commit does not publish; provider/configuration errors are sanitized and do not echo malformed secret-bearing environment values
Secret boundary: mandatory DetectAlways scanning protects prompts, requests, identity artifacts, projections, records, and replay results even when the explicitly unsafe process-wide `--no-secrets` override is active
Focused tests: make localization-check; focused internal/secretscan, internal/deepseek, internal/report, and cmd/repomap tests; focused race tests; focused vet; and git diff --check all passed. Near-boundary tests prove a valid request above the former 1 MiB artifact cap crosses the shared builder/record identity boundary
Adversarial review: independent scope review found no blocker; final security review found and then verified fixes for actual-CLI environment error redaction and the former 2 MiB builder/1 MiB record mismatch; exact identity comparison was additionally hardened against canonical signed-zero aliasing
Full checks: scripts/check.sh passed, including go test, go vet, and six offline quality tasks, real 60.02s; the provider-free etcd snapshot/bundle check passed, real 14.04s
Real saved-run smoke: a copied WAL run with output_language=ru was correctly rejected with `synthesis record is not explicit current English`; no `.localization-projections` root was created. No current explicit-English saved run exists on this host, so positive store/hit evidence comes from the full report-layer and CLI test fixtures rather than a live model run
Generated verification artifacts: tmp/quality, tmp/etcd-snapshot.json, and tmp/etcd-llm-bundle.json were moved to Trash after verification; the stable .bin directory and unrelated tmp contents were untouched
UI actions actually performed: none; this record has no product consumer or browser surface
Not run and why: no live provider, network call, external model, ordinary repository analysis, product binary rebuild, UI localization, shared cache, or Russian-to-English path; the only external repository access was the required provider-free etcd snapshot/bundle check
Residual limitations: publication requires a local filesystem with hard-link and directory-sync support; no total projection-cache eviction policy exists; a post-link cleanup/sync failure may return an error even though the next lookup sees the valid immutable record
Remaining blocker or next checkpoint: ordinary product use still needs a separately approved shared provider executor and cache policy; Decision 166 is replay/storage proof only and must not become the default implicitly
```

## Checkpoint B7 handoff

```text
Checkpoint: B7 — typed report UI message catalog
Changed contract: Decision 167 replaces the report's post-render exact-string/regex/TreeWalker/MutationObserver translator with one versioned fail-closed English/Russian product-message catalog shared by the main renderer, Architecture Canvas, surface catalog, and fixed template chrome; report JSON, manifests, HTTP, source authority, semantic artifacts, providers, caches, and ordinary --lang propagation are unchanged
Catalog boundary: 1052 English and 1052 Russian message IDs have exact ID and declared-parameter parity; product copy is selected at its render/update site, while repository/model prose, paths, symbols, packages, source, IDs, links, and line numbers are opaque parameters and are never translated or scanned
Failure behavior: duplicate source keys are rejected before JavaScript evaluation; unsupported locales, unknown IDs, missing parameters (including explicitly undefined values), undeclared parameters, and EN/RU schema drift fail explicitly; English is not a Russian fallback and there is no Russian-to-English path
Acceptance evidence: Russian plural behavior covers 1, 2, 5, 11, and 21; Unicode path/symbol/package parameters remain byte-exact; every literal message ID used by script.js, architecture_canvas.js, surface_catalog.js, and report.html is declared and no catalog ID is orphaned; targeted regression checks forbid the former hardcoded dynamic count/status/surface-label branches; RU_UI, translateUI, TreeWalker, and MutationObserver are absent
Provider-free real replay: an existing Russian Casdoor run and an English etcd run rendered successfully with DeepSeek and REPOMAP_LLM variables removed and GOPROXY=off; the outputs declare the requested html lang and required no provider or network call
Canonical A/B evidence: parent HEAD and Decision 167 rendered the same copied Russian Casdoor saved run; raw report.json differs only in run-local absolute artifacts_dir and feedback_path, and jq-sorted JSON with those two fields removed has the identical SHA-256 1a4639ea9bdc47db87a8c70f31521b686a26c3eb7b2565d028d8324d971ecb05
UI actions actually performed: opened Overview; opened Study overview; opened the first Study detail; opened an exact-source drawer; opened Architecture; selected a component and inspected its source/study links; mounted the debug surface catalog; selected the process-entry filter and observed the dynamic Russian status «Показано точек запуска: 1 из 1.»
Supervisor verdict: COMMIT D167, blockers none; legacy explicitly cataloged English-valued Russian copy belongs to one later ordinary RU-copy slice, while model-authored English labels remain intentionally opaque
Adversarial review: the first pass found six dynamic script.js product-copy bypasses despite green broad tests; commit was held, every branch was migrated to typed named-parameter messages, a targeted bare-copy regression was added, and the focused re-review confirmed all reported bypasses closed. The review also established and accepted the sole bounded server-rendered noscript exception
Focused tests: node --check for all four JavaScript assets; focused UIMessage/Language, Architecture/Surface/UIMessage/Language, full internal/report, and Russian saved-report CLI tests all passed
Full checks: go test ./..., go vet ./..., scripts/check.sh (including six offline quality tasks), scripts/etcd_check.sh /Users/dvordrova/git/etcd, and git diff --check all passed with Go 1.26.4 selected through a per-command PATH prefix
Not run and why: no live provider, no semantic regeneration, no external repository mutation, no cache read/write change, and no English-to-Russian-to-English round trip; Decision 167 changes only product-owned renderer messages
Residual limitation: some Russian catalog entries deliberately retain the prior English copy, and model-authored labels may be English; neither is a fallback or an ownership ambiguity. The one fixed no-parameter noscript notice is server-rendered in exact EN/RU because the JavaScript catalog cannot execute when JavaScript is disabled; tests forbid any second template-language conditional
Remaining blocker or next checkpoint: replace only Russian catalog values that still preserve canonical English wording, excluding every model-owned value
```

## Checkpoint B8 handoff

```text
Checkpoint: B8 — complete Russian report UI copy
Changed contract: Decision 168 replaces product-owned English copy that Decision 167 deliberately preserved inside the Russian typed UI catalog; English catalog values, message IDs, parameter contracts, report JSON, manifests, HTTP, source authority, navigation, providers, semantic stages, and caches are unchanged
Inventory: the Decision 167 baseline had 750 EN/RU byte-identical renderers: 363 main, 254 Architecture, and 133 surface-catalog entries. Decision 168 translates 747 product-owned renderers and leaves exactly three complete opaque technical values byte-identical: surfaces.identity.http_route, surfaces.location.open, and surfaces.value.http
Catalog evidence: English catalog section is byte-identical to Decision 167 (SHA-256 f0d797cc5c340b697a18c5af4e4e1938f8197d9612f60cb387c2376de4cbadf9); both locales expose 1052 IDs with exact declared-parameter parity; Unicode paths, symbols, packages, routes, locations, and other dynamic parameters remain byte-exact
Semantic-copy review: independent review found and closed two meaning-loss defects in repository workspace and trace-readiness-reason labels; both now have exact representative assertions. Previously mixed product labels such as Study/Overview, endpoint, commit, surfaces, wrapper, worker, callback, and runtime trace/order were replaced with Russian copy while explicit technical names and commands remain exact
Generated golden: report.golden.html was mechanically regenerated because it embeds the complete catalog; after removing that embedded script, the golden is byte-identical to Decision 167, proving no surrounding report-format drift
Provider-free replay: copied Russian Casdoor and English etcd saved runs were rendered with all DeepSeek and REPOMAP_LLM variables removed and GOPROXY=off. Canonical report JSON remained byte-identical after removing only artifacts_dir and feedback_path: Russian SHA-256 1a4639ea9bdc47db87a8c70f31521b686a26c3eb7b2565d028d8324d971ecb05; English SHA-256 4000b180c41dee1cd526ec71e209e4989b1ef80fcf5c98e6c74a7c26e3bb395f. Final HTML SHA-256 values are 401f7c48e82816b9a9453a7576fdb577032462df16d01dc344daee6f9fdce719 (Russian) and dc943011a8b1f6fdafe6611cc90a63e1fb28784ff5db3001cb96ae96befb807d (English)
Focused tests: node --check internal/report/templates/ui_messages.js; go test ./internal/report -run 'UIMessage|Language' -count=1; go test ./internal/report -run TestWriteReportHTML_Golden -update -count=1; and go test ./internal/report -count=1 passed. The catalog test enforces the exact three-ID allowlist, representative main/Study/Task Lens/Architecture/surface copy, semantic regression labels, Russian plurals, and opaque Unicode values
Full checks: scripts/check.sh, scripts/etcd_check.sh /Users/dvordrova/git/etcd, and git diff --check passed with Go 1.26.4 selected through a per-command PATH prefix
Independent review: PASS with no blockers after the two semantic-copy fixes; scope contains only the decision pointer/document, typed catalog, generated golden, catalog test, and this handoff
UI actions actually performed: a fresh local report server started and served both copied runs, but the in-app browser blocked localhost and file navigation by browser policy, so no new manual click-through is claimed. Decision 167's green Overview → Study → detail → exact source → Architecture component/source/Study → surface-filter journey remains the last actual click evidence; Decision 168 changes only catalog values used at those already-tested render sites
Not run and why: no live provider, semantic regeneration, external-repository mutation, cache read/write change, English-to-Russian-to-English round trip, or alternate browser-policy workaround
Residual limitation: model- and repository-authored prose can remain English by design; only product-owned UI copy is covered. The sole fixed noscript notice remains server-rendered because the JavaScript catalog cannot execute when JavaScript is disabled
Remaining blocker or next checkpoint: none for Decision 168; any localization of model-authored prose or projection-cache wiring requires a separately approved decision
```
