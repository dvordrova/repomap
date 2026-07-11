# Company engineer trial

repomap's external product direction is:

> A trustable, inspectable Go repository investigation CLI for engineers using
> an OpenAI-compatible company model.

The trial evaluates the real product workflow. It is not a separate demo, prompt
pack, or provider plug-in system. The three user goals share the same local facts,
evidence contracts, investigation reducer, model boundary, and quality harness.

This is an acceptance track across the canonical milestones, not a replacement
roadmap. M3 establishes repeatable cross-repository quality, M5 makes exploration
progressive, M6 delivers the feature `ChangeBrief`, and M7 adds onboarding policy.

## Trial prerequisites

Build the current CLI:

```bash
go build -o ./repomap ./cmd/repomap
```

The first friend calibration uses the current reference model,
`deepseek-v4-flash`. No endpoint or model override is needed:

```bash
export DEEPSEEK_API_KEY=...
./repomap doctor llm --check
./repomap /path/to/a/known/go/repository
```

This performs one bounded orientation call, prints compact-context and exact
request byte counts, writes the retained artifacts outside the analysed
repository, and opens the HTML report. Use `--no-open` to keep the same run in
the terminal only. Running `./repomap` without a repository argument analyses
the current directory.

For a later company-model calibration, configure a full OpenAI-compatible
`chat/completions` endpoint. repomap never loads a repository-controlled `.env`
file.

```bash
export REPOMAP_LLM_ENDPOINT=https://llm.company.example/v1/chat/completions
export REPOMAP_LLM_MODEL=company-code-model
export REPOMAP_LLM_API_KEY=...
export REPOMAP_LLM_AUTH=bearer
export REPOMAP_LLM_TIMEOUT=90s
```

For an explicitly unauthenticated local endpoint:

```bash
export REPOMAP_LLM_ENDPOINT=http://127.0.0.1:11434/v1/chat/completions
export REPOMAP_LLM_MODEL=qwen2.5-coder:1.5b
export REPOMAP_LLM_AUTH=none
```

`DEEPSEEK_*` remains a compatibility alias. New integrations should use
`REPOMAP_LLM_*`. The namespaces are atomic: generic configuration requires an
explicit endpoint and never inherits a legacy DeepSeek key or endpoint.

Check configuration without sending repository content, then optionally send a
tiny synthetic JSON compatibility request:

```bash
./repomap doctor llm
./repomap doctor llm --check
```

Inspect the exact request body before the first repository call:

```bash
./repomap /path/to/repo --preview-request > /tmp/repomap-request.json
wc -c /tmp/repomap-request.json
```

The preview file is the exact provider body, so the engineer can review both its
contents and byte size before authorizing the first repository call.

The normal first call performs one bounded orientation request, presents all
validated candidate directions in the browser report, and stops there.
`--flows N` additionally expands the top N directions.

```bash
./repomap /path/to/repo
./repomap /path/to/repo --flows 1
```

Today `--flows 1` expands the highest-ranked direction. The displayed direction
cards are not yet actions: named direction choice, deterministic symbol
candidates, and the resumable investigation handoff still live outside the main
CLI. The current friend pass is therefore an orientation baseline, not a claim
that the complete progressive journey is wired.

Debug artifacts default to the user cache rather than the analysed repository.
Use `--no-debug` to retain nothing or `--debug-dir` to choose an explicit trusted
location.

## One workflow, three goal policies

```text
goal
  -> deterministic repository survey
  -> bounded model context
  -> validated directions
  -> user-confirmed focus
  -> focused symbol/source/test evidence
  -> supported claims + unknowns
  -> result or another explicit choice
```

The future public goal contract is deliberately small:

```text
onboarding                 known project, standard learning questions
explore                    unfamiliar project, choose and follow one direction
feature + required question  bounded change-surface hypothesis
```

Profiles may change initial questions, evidence ranking, allowed actions, and
stop conditions. They must not fork collectors, evidence types, provider clients,
or investigation state machines.

### 1. Onboarding calibration

Use a clean pinned revision of a project the engineer already knows. The result
should cover purpose, major components, startup/work intake, representative
flows, state boundaries, tests, unknowns, and a suggested learning order.

The engineer records separately:

- useful directions;
- important omissions;
- factual errors;
- unsupported claims;
- whether they would give the result to a new teammate.

Knowing the project makes this a trust calibration, not a memory test for the
model.

### 2. Unfamiliar repository exploration

Use a different Go project the engineer does not know. After choosing one
direction and taking at most two focused steps, they should be able to identify:

- what the project is for;
- one entrypoint and runtime/event flow;
- the first implementation file and a relevant test reference;
- what repomap still has not established.

A maintainer or experienced teammate then checks the explanation.

### 3. Feature counterfactual

Choose an internal feature that has already shipped. Check out the commit before
the change and supply the original ticket text. The result is a `ChangeBrief`,
not a promised patch. It should contain candidate change surfaces, analogous
code, affected flows, relevant tests, risks, unknowns, and the first useful source
location.

Keep the actual diff hidden until evaluation. A useful result reaches at least
one primary changed file, finds a relevant analogue or explicitly reports none,
names a test or preserves a test gap, and does not claim to know the exact patch.

## Current trust controls

- generic and legacy provider credentials cannot be mixed;
- repository reads are confined to tracked regular files under the resolved
  repository root;
- `--offline` disables model calls while preserving the engineer's normal Go
  environment, including an internal module proxy;
- obvious credentials fail closed at the outbound and retained-model-output
  boundaries;
- structured verified paths are allowlisted and normalized before presentation;
- path-like mentions inside orientation evidence are rejected unless the path
  was actually present in the bounded bundle;
- the exact request body can be previewed without a key or network call;
- live debug artifacts record the selected model and endpoint and retain the
  attempted request even when the provider call fails.

When `--dump-llm` is requested, failure to create or write the required request
artifact aborts before the provider call; inspectability is not best-effort.

## Trial completion gates

These are zero-tolerance conditions for all three scenarios:

- no repository-controlled implicit configuration;
- no path outside the local allowlist presented as verified;
- no README or source symlink escape outside the resolved repository root;
- obvious credentials cause remote use to fail closed;
- the exact outbound request is inspectable before sending;
- saved evaluation tasks expose model/provider, prompt/evaluator versions,
  bytes, latency, revision, and build scenario without inventing missing values;
- source-supported claims cite line-addressable evidence;
- `_test.go` references remain navigation evidence until test source is read;
- no runtime claim without a named runtime observation;
- contract adherence, grounding, usefulness, omissions, and trust violations are
  separate dimensions, never one flattering score.

## Current boundary

The secure provider configuration, doctor, request preview, bounded orientation,
source-grounded exact-symbol path, investigation reducer, and etcd offline quality
replay exist. Public `onboarding` and natural-language `feature` commands do not
yet exist, and the investigation reducer is not yet the sole main-CLI workflow.
The trial is complete only after those profiles are wired and replayable; this
document must not be read as a claim that all three commands already work.

## Milestone mapping

1. Finish M3 with the same replayable quality dimensions on k6, Prometheus,
   NATS Server, and golangci-lint; include one company-model compatibility run.
2. Use M4/M5 to retain a session, let the engineer choose a named direction,
   and follow it progressively instead of auto-expanding a ranking.
3. Use M6 for the held-out feature `ChangeBrief`; ticket text changes ranking
   and stop conditions, not the evidence pipeline.
4. Use M7 for onboarding questions over the same workflow and store one
   replayable trial task per user goal.
5. Run the complete three-scenario acceptance exercise with a company engineer.
