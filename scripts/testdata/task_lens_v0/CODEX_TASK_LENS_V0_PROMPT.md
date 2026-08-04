# Repomap Task Lens v0
## Task-conditioned bounded investigation pack + corrected historical evaluation

You are working in the existing `repomap` repository.

This is the next bounded product experiment after the Fuego historical-task
pilot.

Read these existing reports before changing production code:

- `tmp/fuego-historical-benchmark-v0/PRODUCT_FINDINGS.md`
- `tmp/fuego-historical-benchmark-v0/PHASE1_SUPERVISOR_REPORT.md`
- `tmp/fuego-historical-benchmark-v0/PHASE2_SUPERVISOR_REPORT.md`
- `tmp/fuego-historical-benchmark-v0/REPOMAP_VALUE_MATRIX.md`

Also read the independent harness audit if it exists:

- `HARNESS_AUDIT.md`
- or the owner's latest review copy of that file.

Read the accompanying manifests:

- `DEV_SET.json`
- `HOLDOUT_SET.json`

The previous pilot established a useful but narrow result:

- current repomap is a good broad repository guide and first-location finder;
- it sometimes finds representative implementations for extension work;
- it does not reliably assemble the task-specific causal path;
- it did not improve reproduction or verification planning;
- generic accepted Mechanisms can be valid but irrelevant to the user's task;
- Search cannot recover facts omitted before report projection;
- the full onboarding pipeline is wasteful for local negative-control tasks.

The previous A/B harness was not a valid independent answer-quality benchmark:

- assisted answers reused the baseline memo;
- task packets had a ceiling effect;
- task-labelled directory names leaked into Orientation;
- reports recorded the wrong captured revision because a shared Git HEAD was
  used with alternate indexes/worktrees.

Do not repeat those mistakes.

---

# Product hypothesis

Add a small, ephemeral **Task Lens** on top of the existing local evidence
engine.

User job:

> I have a concrete bug report, feature request, operational problem, or
> contribution task. Help me find the smallest relevant repository context,
> form a cited working hypothesis, reproduce or observe the behavior, and decide
> what to inspect or verify next.

Task Lens is not:

- a replacement for Repository Guide;
- a canonical repository truth object;
- a bug-fixing agent;
- a full issue tracker integration;
- a global call graph;
- a license to publish speculative causal stories.

The intended product composition is:

```text
Repository Guide
  reusable repository-level orientation

Task Lens
  ephemeral task-conditioned investigation overlay
```

The Task Lens may link to Brief, Shape, Study Directions, Paved Paths, or
canonical Mechanisms when they are relevant. It must not surface generic
`Start Here` merely because one exists.

---

# Main success question

Can a bounded task-conditioned path produce materially better investigation
context than the generic report while using far less work on local tasks?

The experiment succeeds only if the frozen holdout demonstrates:

1. strong recall of historically necessary files/symbols;
2. a useful causal or obligation-focused hypothesis;
3. grounded reproduction/observation guidance;
4. grounded verification guidance;
5. a cheap exit for local/negative-control tasks;
6. no repository-specific production rules;
7. no unsupported user-visible claims.

Do not claim success because the UI renders or JSON validates.

---

# Required Task Lens output

Create a presentation/research artifact such as `TaskInvestigationPack`.

Do not extract a universal `KnowledgeObject` framework.

The pack is scoped to:

- one repository snapshot;
- one user task;
- one bounded investigation run.

It should contain:

## Task interpretation

- concise restatement;
- task kind:
  - bug
  - feature
  - extension
  - configuration
  - operational
  - compatibility
  - unknown
- observable symptom or requested outcome;
- task terms that were found in repository evidence;
- terms that remain only user-provided.

## Likely area

- 1–3 repository areas/packages;
- why each is relevant;
- exact local target IDs;
- related existing Guide objects only when task-relevant.

## Investigation anchors

Prefer 3–8 exact anchors.

Each anchor includes:

- path;
- enclosing symbol or document section;
- role:
  - symptom_site
  - public_or_cli_entry
  - state_owner
  - state_mutation
  - configuration_source
  - configuration_copy
  - error_creation
  - error_mapping
  - integration_boundary
  - representative_implementation
  - generated_output
  - reproduction_anchor
  - verification_anchor
  - documentation_contract
- bounded source/document excerpt;
- why this anchor matters for the task;
- exact evidence IDs;
- direct relation IDs when locally known.

## Evidence joins

The core product primitive is a bounded join between two or more exact anchors.

Examples:

- write failure ↔ nil request-context dereference;
- group shallow copy ↔ later append into shared slice state;
- public config field ↔ effective engine config assignment;
- type-name parsing ↔ generated OpenAPI component identifier;
- adapter precedent ↔ module/workspace/example/test obligations;
- documented spec version ↔ UI dependency capability.

A join records:

- left/right anchor IDs;
- relation kind;
- support type:
  - locally_observed
  - document_supported
  - model_hypothesis
  - unresolved
- exact supporting evidence;
- scope/non-guarantees.

Do not turn model prose into a locally observed relation.

## Working hypothesis

A short causal or implementation hypothesis.

Every clause is labeled:

- supported;
- plausible;
- unresolved.

Do not use unsupported sequence or runtime language.

## Reproduce or observe

Use only:

- task-provided reproduction;
- exact repository docs;
- exact examples/tests;
- exact commands/config/endpoints already in repository evidence.

Do not invent commands.

Do not execute arbitrary target-repository commands.

When no grounded reproduction exists, say what evidence is missing and offer
the smallest safe observation step.

## Verify

Identify:

- effect to observe;
- exact existing test/example/fixture when useful;
- likely regression-test location;
- generated output or response status when grounded;
- non-destructive command only when repository-owned evidence exists.

Tests are optional evidence, not a publication gate.

## Next probes

At most 1–3 concrete next actions:

- inspect exact symbol;
- resolve one caller/callee/reference;
- compare two config copies;
- inspect one generated fixture;
- inspect one sibling implementation.

No vague “read the codebase”.

---

# Epistemic contract

Keep the existing core rule:

```text
LOCAL selects facts and assigns opaque IDs
  → LLM proposes semantic organization
    → LOCAL validates IDs, claims, and scope
      → Task Investigation Pack
```

The Task Lens may generate a hypothesis, but must preserve the distinction
between:

- repository fact;
- document claim;
- task-provided symptom;
- model inference;
- unresolved question.

The task text is not repository truth.

The task text must not alter:

- canonical repository identity;
- Repository Brief;
- generic Orientation;
- canonical Mechanism identity;
- existing report artifacts outside the task overlay.

---

# Bounded local retrieval

Build the smallest generic retrieval path that can work across the corpus.

Allowed deterministic sources:

- repository snapshot and manifests;
- package/module index;
- exact symbol declarations;
- exact textual references;
- AST calls/selectors/assignments/branches;
- existing saved source windows;
- docs/README/config/scripts;
- existing canonical Guide/Mechanism IDs;
- bounded gopls symbol/reference/caller/callee queries;
- generated fixture files as read-only evidence.

Do not add:

- global SSA;
- pointer analysis;
- repository-wide call graph;
- runtime-surface discovery;
- arbitrary command execution;
- repository-specific rules.

Suggested hard budgets per task:

- initial lexical/symbol candidates: ≤40;
- retained anchors before review: ≤16;
- visible anchors: 3–8;
- source/document files read: ≤12;
- retained source/document bytes: ≤128 KiB;
- gopls symbol/reference/call queries: ≤12 total;
- named frontier expansions: ≤2;
- deterministic local time: ≤10 seconds, excluding gopls startup;
- synthesis calls: normally 0–2;
- no semantic retry after substantive rejection.

These are experiment defaults, not universal product laws. Record when they
bind.

---

# Cheap local exit

The negative controls showed that the full onboarding pipeline is wasteful.

Before any broad/global editorial stage, classify task locality.

## Local exact task

Examples:

- exact config field omitted from copier;
- nil reflection panic in one validator;
- wrong error type at one negotiation fallthrough;
- docs dependency/import mismatch.

Expected path:

```text
task terms
→ exact symbol/reference retrieval
→ 2–5 anchors
→ optional one compact synthesis call
→ Task Pack
```

Do not run Orientation, Architecture, Study Map, Mechanism Opportunity, or
full Paved Paths merely to answer this task.

## Bounded cross-file task

Use:

- local symbol/reference/call expansion;
- at most two frontiers;
- one synthesis call after facts exist.

## Extension/contribution task

Use:

- sibling implementations;
- public interfaces;
- module/workspace wiring;
- examples;
- repository-owned verification commands.

May reuse existing Shape/Study artifacts if already available, but do not
require a full fresh generic report.

## Broad/dynamic task

If bounded evidence is insufficient:

- return a useful partial Task Pack;
- list exact unresolved probes;
- do not silently fall back to a generic repository report as the answer.

Record task classification and stages skipped.

---

# Model-call policy

The task-conditioned path should not inherit the current theoretical 25-call
onboarding ceiling.

Target:

- local exact tasks: 0–1 calls;
- bounded cross-file tasks: 1–2 calls;
- extension tasks: 1–3 calls;
- hard maximum: 4 calls per task.

Calls should have narrow roles:

1. optional task interpretation/candidate selection;
2. bounded evidence synthesis/review;
3. optional contribution/operational compression.

No leaf fan-out over unrelated global topics.

No provider/model change.

No model-routing framework.

---

# CLI and artifacts

Inspect the current CLI and choose the smallest reversible experimental
surface.

Preferred shape, unless existing conventions suggest a safer equivalent:

```bash
repomap investigate /path/to/repository --task-file task.md
```

or:

```bash
repomap /path/to/repository --task-file task.md
```

Required options:

- `--no-open`
- `--no-serve`
- `--debug-dir`

Do not make Task Lens the default `./repomap` behavior in this experiment.

Persist:

- `task_investigation_bundle.json`
- `task_investigation_attempt.json`
- `task_investigation.json`
- `task_investigation_status.json`
- report projection when accepted/partially useful.

The canonical repository report must remain usable without task mode.

---

# User-facing report

Add one optional Task Lens workspace to the existing renderer.

Suggested route:

```text
#/investigate/<task-id>
```

The first viewport should show:

- task interpretation;
- likely area;
- working hypothesis with support labels;
- anchor map/list;
- reproduce/observe;
- verify;
- next probes.

Source should be progressively disclosed.

Do not lead with a generic Repository `Start Here`.

Do not show irrelevant canonical Mechanisms.

Related objects must pass a local task-relevance check.

Architecture is optional context, not a fallback destination.

Do not expose:

- model verdict jargon;
- candidate funnels;
- raw IDs;
- unsupported hidden attempts;
- generic gaps unrelated to the task.

---

# Harness repair — mandatory before product evaluation

The previous historical harness had three material defects.

## 1. Real revision identity

For every episode:

- create a real detached Git worktree at the exact base revision;
- run repomap inside that worktree;
- use a neutral directory basename such as `repo`;
- create a separate `.git`-free export for any source-only solving condition;
- assert:
  - `captured_revision == expected base revision`;
  - tree hash matches the base commit;
  - no later files are present.

Do not use a shared `GIT_DIR` HEAD with alternate indexes as the report
repository.

## 2. Repository basename is not semantic evidence

Fix generically:

- basename may remain display copy;
- canonical repository identity should prefer module path, remote, or manifest;
- task-labelled temporary directory names must not influence model-visible
  purpose, Orientation, Brief, or claims;
- add a regression with two identical repositories under different directory
  names and require semantically identical bundles/outputs aside from display
  labels and authorized paths.

## 3. Independent conditions

Do not compare an assisted memo created by copying the baseline memo.

For this experiment, the primary machine gate is **Task Pack versus historical
gold**, not a same-session A/B memo.

Optional answer-quality A/B may run only when:

- separate fresh Codex/model sessions are available;
- neither condition can read the other's answer;
- both use neutral source directories;
- outputs are sealed before gold is opened.

If independent sessions are unavailable, prepare condition packets and report
`answer_A_B_not_run`. Do not fake independence.

---

# Development and frozen holdout

Use the accompanying manifests.

## Development set

`DEV_SET.json`

You may use the existing historical gold and previous benchmark reports for
these six episodes while designing the generic Task Lens.

Iterate locally until the design is stable.

Do not add episode-specific production branches, aliases, paths, or prompt
examples.

A generic test fixture may encode an episode's saved facts/expected contract,
but production behavior must remain repository-agnostic.

## Freeze

Before opening any holdout historical gold:

1. freeze code;
2. freeze prompts/schemas;
3. freeze budgets;
4. record binary SHA;
5. record git diff;
6. write `FREEZE_MANIFEST.json`;
7. ensure holdout gold is not present in the active workspace.

## Holdout set

`HOLDOUT_SET.json`

Generate one Task Pack per holdout episode from:

- prompt-safe task text;
- exact base revision;
- current frozen implementation.

One attempt per episode.

No semantic retry.

No code/prompt/validator changes after the first holdout output.

Seal every pack and status file with SHA-256.

Only after all holdout outputs are sealed may the supervisor gold be opened for
evaluation.

If the gold package is already present in the working directory, move it out
or stop with `holdout_contaminated`.

---

# Evaluation against historical gold

After holdout sealing, compare Task Packs—not revised agent memos—to gold.

Score separately 0–4:

- task interpretation;
- subsystem localization;
- must-read file recall;
- key-symbol recall;
- causal/evidence-join quality;
- reproduction/observation usefulness;
- verification usefulness;
- uncertainty calibration;
- task relevance of linked Guide objects;
- cost appropriateness.

Also record:

- unsupported claims;
- irrelevant anchors;
- important missing anchors;
- stages/calls skipped by cheap exit;
- wall time;
- input/output/cache tokens;
- local retrieval actions.

Do not compute one opaque average.

## Holdout target

A successful bounded result should satisfy at least:

- 4 of 6 holdout packs score ≥3 for subsystem localization;
- 4 of 6 score ≥3 for must-read file recall;
- 3 of 6 score ≥3 for causal/evidence-join quality;
- 4 of 6 provide useful verification guidance;
- all three local/negative-control-like tasks use ≤1 model call where possible;
- zero major unsupported user-visible claims;
- no task-labelled basename leakage;
- every report revision matches the expected base revision.

This is an experiment threshold, not a release promise.

---

# Review bundle

Create:

```text
tmp/task-lens-v0/
```

Required:

- `PLAN.md`
- `RUN_LOG.md`
- `EXPERIMENTS.jsonl`
- `FREEZE_MANIFEST.json`
- `HARNESS_AUDIT.md`
- `DEV_EVALUATION.md`
- `HOLDOUT_EVALUATION.md`
- `PRODUCT_FINDINGS.md`
- `SUPERVISOR_REPORT.md`
- `WALKTHROUGH.md`
- `review/index.html`
- `dev/`
- `holdout/`
- `screenshots/`

Per episode:

- exact task packet;
- source/base provenance;
- Task Lens artifacts;
- report;
- metrics;
- sealed hashes;
- gold comparison only after unlock.

The static review should let the owner compare:

- task;
- Task Lens;
- exact anchors/source;
- reproduce/verify guidance;
- historical gold;
- costs;
- failure notes.

Do not build a second production frontend.

---

# Focused verification

Run:

- tests for task bundle and validation;
- tests for task-text versus repository-truth separation;
- basename leakage regression;
- captured-revision/worktree regression;
- cheap-exit classification tests;
- bounded retrieval tests;
- report route/projection tests;
- browser/JS smoke;
- `git diff --check`.

Do not require:

- full external repository tests;
- runtime-surface discovery;
- repository-wide SSA/call graph;
- full release suite;
- broad five-repository fresh sweep.

Do not commit or push.

---

# Stop conditions

Stop and report honestly when:

- holdout gold contamination cannot be excluded;
- task relevance requires repository-specific rules;
- causal output requires a global call graph;
- model hypotheses cannot be clearly separated from facts;
- cheap local tasks still require the full generic pipeline;
- report identity cannot be tied to the actual base revision;
- source-only and assisted conditions cannot be made independent;
- the implementation starts becoming an issue tracker or autonomous fixer.

Do not respond by:

- weakening validation;
- adding semantic retries;
- increasing all context limits;
- hand-editing holdout packs;
- promoting an irrelevant generic Mechanism;
- switching repositories after failure.

---

# Mandatory supervisor report

At the end create:

```text
tmp/task-lens-v0/SUPERVISOR_REPORT.md
```

It must begin with:

- outcome: `passed | partial | failed`;
- technical result;
- product result;
- investment result;
- development episodes used;
- holdout episodes evaluated;
- single strongest demonstrated value;
- single main blocker;
- exactly one recommended next step.

Then include:

## Previous benchmark defects

For each:

- reproduced;
- fixed;
- regression test;
- residual risk.

Defects:

- wrong captured revision;
- task-labelled basename leakage;
- dependent A/B answers;
- ceiling-effect prompts;
- excessive generic call cost.

## Task Lens contract

- object scope;
- evidence sources;
- hypothesis labels;
- relation/evidence joins;
- reproduction/verification authority;
- replay/staleness;
- user-facing projection.

## Cheap exit

Per episode:

- task locality class;
- stages skipped;
- model calls;
- local actions;
- wall time;
- whether the result was sufficient.

## Development set

- what changed during development;
- generic versus fixture-only changes;
- failures that shaped the contract;
- no episode-specific production rules.

## Holdout results

Separate scorecard per episode:

- interpretation;
- subsystem;
- files;
- symbols;
- joins;
- reproduce;
- verify;
- calibration;
- linked-object relevance;
- cost.

No average.

## Product comparison

Compare current generic report and Task Lens at the artifact level:

- relevant anchors;
- irrelevant generic objects;
- causal pair coverage;
- reproduction/verification;
- call/token cost;
- user next action.

Do not claim independent answer-quality A/B unless it actually ran in separate
sessions.

## Model/resource accounting

- calls;
- tokens;
- cache;
- request/response bytes;
- provider latency;
- wall time;
- gopls/local retrieval count;
- budget binding.

## Product findings

Answer:

1. Which real task classes benefit?
2. Which tasks should cheap-exit locally?
3. Does task conditioning improve evidence joining?
4. Which Guide surfaces remain useful?
5. Which generic artifacts are often irrelevant?
6. Is a production Task Lens justified?
7. What is the one most important remaining gap?

## Verification

List exact focused checks.

List broad/slow checks skipped.

## Final recommendation

Recommend exactly one:

- integrate Task Lens as an experimental product surface;
- run one cross-repository task holdout;
- stop/redesign task-conditioned investigation.

Do not begin the next experiment.

At terminal completion print only:

Supervisor report:
<exact path>

User review:
<exact command>
<exact URL>
