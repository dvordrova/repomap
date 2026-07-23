# Task Lens v0 experiment harness

The Task Lens harness is an experiment protocol, not a production analysis
layer. It prepares exact historical repositories, runs the Task Lens CLI,
seals outputs, and keeps holdout gold unavailable until every holdout episode
is complete.

The protocol runner is implemented by `scripts/task_lens_harness.py`; the
post-seal gold boundary and score renderer are exposed separately by
`scripts/task_lens_eval.py`. Their focused regression builds the current
`repomap` binary, runs a synthetic freeze/holdout protocol, and performs one
offline real-binary development smoke:

```bash
make task-lens-harness-check
```

## Inputs

The checked-in owner bundle is byte-for-byte bound by
`scripts/testdata/task_lens_v0/MANIFEST.sha256`:

- `scripts/testdata/task_lens_v0/CODEX_TASK_LENS_V0_PROMPT.md`
- `scripts/testdata/task_lens_v0/DEV_SET.json`
- `scripts/testdata/task_lens_v0/HOLDOUT_SET.json`
- `scripts/testdata/task_lens_v0/README.md`

`scripts/testdata/task_lens_v0/BUDGETS.json` is the machine-readable v0 budget
contract derived from that prompt. Freeze refuses prompt or manifest bytes
that do not match the owner checksum ledger and records every input separately.

Set `TASK_LENS_SOURCE_REPO` to a Fuego clone containing every manifest
revision. Development task packets are read from
`TASK_LENS_DEV_TASKS_DIR`; the harness recognizes either `<id>.md`,
`<id>/task_packet.md`, or `episodes/<id>/task_packet.md` and requires exactly
one match.

## Protocol

Initialize and prepare development:

```bash
make task-lens-init
make task-lens-dev-prepare \
  TASK_LENS_SOURCE_REPO=/path/to/fuego \
  TASK_LENS_DEV_TASKS_DIR=/path/to/development/task-packets
make task-lens-dev-run TASK_LENS_BINARY=.bin/repomap
```

Development may have multiple numbered attempts. Each completed attempt is
validated and sealed independently. Before freeze, complete
`tmp/task-lens-v0/DEV_EVALUATION.md` with findings for every development
episode. Freeze selects the latest attempt for each episode and requires it to
be non-offline, to contain one live provider call, and to be bound to the
candidate binary:

```bash
make task-lens-freeze \
  TASK_LENS_BINARY=.bin/repomap
```

The freeze binds all of the following with SHA-256:

- the executable binary;
- the owner prompt, bundle README, and checksum ledger;
- the active decision;
- the budget policy;
- both task manifests;
- explicitly selected prompt/schema/policy contract source;
- frozen copies of the harness and evaluator used by holdout execution and
  post-seal gold/evaluation commands;
- the implementation `git diff --binary --full-index HEAD`;
- a deterministic tracked-plus-untracked implementation content snapshot and
  inventory, excluding the experiment root;
- the selected sealed live development attempts;
- the completed development evaluation.

Every later freeze verification revalidates the frozen source snapshot, the
selected development attempt trees, and the task/provenance sidecars. Once a
holdout or rendered review exists, its episode sidecars and review links are
checked against the frozen task ledger as well.

After freeze, predeclare the local or negative-control-like tasks expected to
take the cheap exit, while holdout execution and historical gold are both
still unavailable:

```bash
make task-lens-cheap-exits-declare \
  TASK_LENS_CHEAP_EXIT_EPISODES="<episode-1> <episode-2> <episode-3>"
```

The declaration is bound to the freeze and frozen holdout manifest, then
SHA-256 sealed. Holdout preparation refuses to start without it, and the
preparation record and global holdout seal repeat its hash. The post-gold
scorecard inherits its booleans rather than asking the evaluator to choose
them.

Only after freeze and that declaration may the holdout be prepared and run:

```bash
make task-lens-holdout-prepare TASK_LENS_SOURCE_REPO=/path/to/fuego
make task-lens-holdout-run
```

The holdout runner uses the copied frozen binary and frozen
`HOLDOUT_SET.json`. It creates its attempt directory before invoking the CLI,
so even an incomplete or failed invocation consumes the episode's one attempt.
There is no retry flag. A global `holdout/HOLDOUT_SEAL.json` appears only when
all episode pack/status outputs verify and their complete attempt trees have
been hashed.

Then, and only then, unlock historical gold:

```bash
make task-lens-gold-unlock TASK_LENS_GOLD_DIR=/path/to/holdout-gold
```

This copies the gold into `evaluation/gold`, records its complete inventory,
and creates `evaluation/SCORECARD.json`. A supervisor fills the ten separate
0–4 dimensions plus failure lists and outcome fields, then renders the review:

```bash
make task-lens-evaluate
make task-lens-review
```

The evaluator never computes an average. It reports the approved thresholds
and preserves the individual scores. It also requires
`answer_A_B_not_run`; answer-quality A/B is outside this harness unless truly
independent sessions are separately implemented and sealed. An overall
`passed` outcome is accepted only when every computed holdout target passes,
including the zero-major-unsupported-claim gate. Those targets are necessary,
not sufficient: a supervisor may still report `partial` or `failed` for a
qualitative product blocker. `recommended_next_step` and
`final_recommendation` must be the same one of the three approved next steps.

## Revision and isolation guarantees

For each episode the harness:

1. rejects identity/worktree-affecting ambient `GIT_*` variables and removes
   every `GIT_*` variable from child environments;
2. resolves the full 40-hex commit and requires it to be an ancestor of the
   source comparison revision;
3. creates a real detached worktree named exactly `repo`;
4. verifies HEAD, tree hash, detached state, and a completely clean status;
5. proves paths added after the base revision are absent, plus any explicit
   manifest absence assertions;
6. creates a separate Git-archive source export, also named `repo`, and rejects
   any `.git` entry in that export;
7. checks emitted bundle, pack, and status revision/tree identity before
   sealing.

It also validates artifact semantics before accepting an attempt. In
particular, the manifest and evidence file reads share one total budget;
anchor, evidence, relation, join, hypothesis, guidance, and probe references
must resolve; and skipped stages must use the exact ordered v0 list. A
zero-anchor result is allowed only for an explicitly insufficient broad or
negative-control task. Attempt status records provider provenance, while
`sufficient` is derived independently from the resulting pack, so a local
partial result can still be sufficient.
Extension-contribution packs additionally need integration and verification
anchors, at least two likely areas, and a document-supported or substantive
locally observed join; a model-only or shared-task-term join is not enough.
Verification must cite task-provided evidence, an anchor participating in such
a strong join, or an anchor matching a non-generic grounded task term. Generic
words such as `test`, `error`, `config`, or `package` cannot make an unrelated
verification anchor sufficient.

Accepted model attempts retain the exact raw response and bind its byte count
and SHA-256 into the status. Responses that are oversized, secret-bearing, or
otherwise unusable are rejected and fall back to the local pack. The validator
copies each artifact quartet to a temporary directory and invokes the exact
launched candidate or frozen binary through `dev render-report`. That replay
recomputes the stable prompt, proposal reduction, finalization, role-hint and
task-observation rules, sufficiency, and the complete user-facing workspace;
the temporary copy is discarded afterward. The saved report must equal that
complete Go projection, and both report and run manifest must bind the exact
bundle, attempt, pack, and status SHA-256 values. The validator also rejects
legacy generic report artifacts and checks the report hash, repository state,
material inputs, and freshness. Clean repository-state hashes are recomputed
from location-independent content.

The focused check includes current-binary adversarial mutations: a retained
accepted response whose restatement and response accounting are consistently
rehashed, a changed report projection with a recomputed report hash, a swapped
artifact hash in both report and manifest, and a rehashed incorrect stable
prompt identity. Each must be rejected by the seal path.

Holdout preparation rejects source, manifest, or output paths under Downloads
or the legacy Fuego benchmark. Gold paths are accepted only by the post-seal
`unlock-gold` command.

## Stable review convention

The generated supervisor report always ends with:

```text
Supervisor report:
<absolute path>/SUPERVISOR_REPORT.md

User review:
python3 -m http.server 8767 --bind 127.0.0.1 --directory <absolute root>
http://127.0.0.1:8767/review/
```

The static review is an experiment index over saved artifacts. It is not a
second production frontend.
