# repomap — product constitution

Read this before touching anything. It defines what the product is, who it is
for, and the invariants that every change must preserve. If a change conflicts
with this file, the change is wrong, not the file — unless the human explicitly
updates the file.

## What the product is

repomap takes a repository and produces a static HTML report that lets a
person who has just been handed that repository jump into the code fast. It
does this by answering the first-day questions with claims anchored to exact
source locations, so that any claim can be verified in seconds by opening the
anchor.

The truth is the code. Everything the report shows is either a fact extracted
from the code deterministically, a claim quoted from human-written artifacts
(README, comments, commit messages — dated, possibly stale), or a hypothesis
produced by a model. The three are never mixed without labeling. A hypothesis
with a good pointer is useful even when wrong; a claim without a pointer is a
bug.

Pointer quality > narrative quality. Missing the single most important fact is
worse than ten small errors.

## Who it is for

A developer (or a tester/PM moving toward code) who did not write this repo and
needs to: understand what it is, run it, find the entry points, follow one main
flow end to end, locate the data model and configuration, see the
integrations, know what is dangerous, what is dead, and what is missing (no
tests, no README, no CI), and know which questions only a human can answer.

The reader is not the tool's author. Internal vocabulary of the pipeline
(targets, projections, authority, retained relations, selectors) must never
reach the screen.

## Non-goals

* Not a documentation generator. It does not describe everything; it routes
  attention.
* Not a graph explorer. The graph is a means; the answers are the product.
* Not interactive-first. A static report, ≤ 2 screens per target before
  expanding anything.
* Not a place for the model to write essays. One line per group purpose, one
  sentence per connection, anchors everywhere.

## Layers of truth (data model invariant)

1. **facts** — deterministic. Targets and their context (manifests, roots),
   entrypoints found by reachability, imports/calls graph, HTTP boundaries
   with method + path literals (server routes and client calls), cross-target
   portals matched on literals, config reads (env keys), risk patterns
   (`exec`, `eval`, `subprocess`, `os.system`, `pickle.loads`, …), manifest
   facts (`scripts`, `proxy`, `engines`, pinned versions, committed `.env`
   keys without values), TODO/FIXME, dead modules (unreachable from real
   entrypoints), and negatives (README below N bytes, zero test files, no
   Dockerfile, no CI).
2. **claims** — extracted from README, comments, docstrings, commit messages.
   Always carry source path and, when available, age.
3. **model** — LLM output: group membership and purposes, connection
   sentences, role descriptions, run recipe, step explanations of the main
   flow. Every model artifact references facts by id. Any referenced id that
   does not exist in the fact layer is rejected (recorded, not silently
   dropped).
4. **human** — confirmations/annotations (future). Ids must be stable enough
   to attach them later: target-scoped `path + symbol + content-hash`.

The group graph is a layer on top of the fact features, never a replacement
for them. Facts survive any rewrite of the model stage because they live in
separate stage outputs.

## Pipeline invariants

* Stages are separate, each reads files and writes files (JSONL/JSON). Any
  stage can be re-run from its inputs alone. Deterministic stages never depend
  on LLM stages.
* Validation is a pure function `(model_output, facts) -> (accepted,
  rejected_with_reason)`. It annotates; it never aborts a run. No thresholds
  like "fail if < 80% valid". Rejected items go to `rejected.jsonl` with the
  raw model output and the reason.
* Rate limits, retries, batching, and cost controls live only in the
  orchestrator.
* LLM calls are cached by `(stage, prompt_version, input_content_hash)`.
* LLM stages work on aggregates (file with imports + signatures), not per
  symbol, and receive extracted literals (route strings, env keys, ports)
  rather than hunting for them.
* Language adapters extract structure; framework knowledge comes first from
  manifests (`package.json`, `pyproject`/`Pipfile`, `go.mod`) and only then
  from heuristics.

## Report invariants (UI)

* Overview page: what the repository is (roles with purpose + anchors), the
  targets as cards, the cross-target portals as a table
  (`GET /api/levels: front/src/service/http.ts:12 → backend/app/app.py:19`),
  negatives stated explicitly, and a run recipe with each command anchored to
  the manifest line it came from.
* Target page, in this order: Inbound (routes/triggers) → Entrypoints → Core
  groups → External calls/dependencies, then one main flow as an ordered list
  of steps with anchors, then risks, config, dead code, TODOs. Evidence is
  collapsed by default, deduplicated by `path:line`, path printed once, max 3
  shown with "+N".
* Connection sentences (the model's one-liners) are visible: on hover and in
  the selected node panel. Never screen-reader-only.
* Each target page shows only its own groups; cross-target edges render as
  stubs pointing to the other target's page.
* Provenance is visible in styling: fact (solid), model (marked as model,
  muted), claim (marked with source and age). A model-written sentence must
  not look like a heading of authoritative documentation.
* Anchors link to a permalink at the captured revision (and to the editor if
  available).
* Banned on-screen vocabulary: retained, source-bound, authority, projection,
  selector, outcome, target contract, and raw selector strings like
  `python:backend:guard:main`.
* The report JS stays small (tens of KB, not hundreds). Prefer one page with
  sections over many routes.

## Acceptance (fixture)

`fixtures/python-tutorial-game` (revision `78714d34ee`) is the canonical
fixture. `fixtures/python-tutorial-game/expected.json` lists facts that must
be present with anchors. A report is acceptable only if a reader can answer
these from the report alone, without opening the repo: what is this, how do I
run it, where does the frontend talk to the backend and on which port, what is
dangerous, what is dead, what is missing, and what is the main flow from
clicking a level to the animation.

## Working rules for agents

* Before changing architecture, generate the report for the fixture from the
  current state, open it, and try to answer the acceptance questions as the
  newcomer. Do this again after the change. Dogfood is the test.
* Remove more than you add. The previous UI rewrite grew to 288 KB of JS and
  12 routes; the one after it dropped the whole fact layer. Both were failures
  of scope, not code.
* When unsure whether something belongs to facts or model, it is facts if a
  grep can find it.
* Do not guess product decisions. Collect open questions and ask the human at
  the end.
