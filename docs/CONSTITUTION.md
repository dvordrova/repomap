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
integrations, know where the code runs code it was given, what is dead, and what is missing (no
tests, no README, no CI), and know which questions only a human can answer.

The reader is not the tool's author. Internal vocabulary of the pipeline
(targets, projections, authority, retained relations, selectors) must never
reach the screen.

## Non-goals

* Not a documentation generator. It does not describe everything; it routes
  attention.
* The graph is a means; the answers are the product. The owner-approved map
  supports pan/zoom, short reachable hover cards, operation-path highlighting,
  and jumps between matched endpoints. It routes attention to commands,
  requests and background work with source anchors. A raw import graph or a
  column of equally weighted headings does not satisfy this purpose.
* Not interactive-first. A static report, ≤ 2 screens per target before
  expanding anything.
* Not a place for the model to write essays. One line per group purpose, one
  sentence per connection, anchors everywhere.

## Layers of truth (data model invariant)

1. **facts** — deterministic. Targets and their context (manifests, roots),
   entrypoints found by reachability, imports/calls graph, HTTP boundaries
   with method + path literals (server routes and client calls), cross-target
   portals matched on literals, config reads (env keys), dynamic execution
   (`exec`, `eval`, `subprocess`, `os.system`, `pickle.loads`, …, the places
   where control leaves code the reader can follow), manifest
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

* Stages are separate functions over typed values. Persisting their inputs and
  results is separate from computation, so a stage can be re-run from saved
  inputs without forcing every ordinary handoff through disk. A cached result
  is keyed by the stage contract, parameters and input content: reuse memory
  first, restore disk only when absent, compute on a miss. Deterministic stages
  never depend on LLM stages.
* Validation is a pure function `(model_output, facts) -> (accepted,
  rejected_with_reason)`. It annotates; it never aborts a run. No thresholds
  like "fail if < 80% valid". Rejected items go to `rejected.jsonl` with the
  raw model output and the reason.
* Rate limits, retries, batching, and cost controls live only in the
  orchestrator.
* Raw LLM responses are cached by provider configuration and exact prepared
  request bytes. Every reuse passes the owning stage's current decoder and
  validator. Independent row memos additionally include the stage contract
  and exact input; they resolve the current raw response rather than retaining
  a second accepted value. Model-facing changes belong in the prompt or
  request shape; a local contract-only change does not force another identical
  provider request.
* LLM stages work on aggregates (file with imports + signatures), not per
  symbol, and receive extracted literals (route strings, env keys, ports)
  rather than hunting for them.
* Language adapters extract structure; framework knowledge comes first from
  manifests (`package.json`, `pyproject`/`Pipfile`, `go.mod`) and only then
  from heuristics.
* Every native target receives an explicit independence decision before page
  analysis. Adapters supply launch, package-ownership and import observations;
  source-addressed author commands remain claims. Only a positive, locally
  validated decision can place an incidental launch inside a standalone owner;
  missing or invalid decisions preserve independent analysis and are recorded.
  Shared code, tools and examples retain their complete analyses and source
  links. Presentation roles never substitute for analysis or hide duplicates
  after the fact. This separation was approved by the owner on 2026-09-09.

## Report invariants (UI)

* Analysis and model-generated semantic prose use English through one shared,
  provider-neutral system-prompt instruction. The owner's 2026-09-07/08
  localization design applies only after the English frontend structure is
  built: ordinary code translates our fixed UI vocabulary, and a separate LLM
  cube translates explicitly selected generated display text. Their results
  form the final frontend structure before HTML rendering. `--lang ru` selects
  a Russian report such as `report.etcd.ru.html`. Canonical analysis, IDs,
  topology, code names, source excerpts and source links keep their original
  values. The localized static HTML and server-rendered report use the same
  saved display translation; JavaScript is not required to obtain translation.
  A declaration may additionally carry a short English Alias supplied by its
  existing interpretation request. The alias stays English in every report
  language and appears beside the native code name; neither replaces the source
  identity. Descriptions remain localized. The page never invents an alias from
  a filename, script detection or a truncated description.
  The owner's 2026-09-08 glossary design adds a shared terminology adjunct to
  analytical requests. Each existing call may return source-anchored term
  explanations beside its original result. One aggregate reduction joins
  compatible meanings before the glossary enters the backend report and display
  translation. Original spellings, definitions, sources and request provenance
  survive that reduction; equal names alone never establish equal meanings.
  Translation keeps glossary names and accepted English aliases in their original spelling and translates
  their definitions and surrounding prose. Code looks up complete literal names
  in the final display text; the model does not annotate occurrences or choose
  tooltip positions. Equal spellings may offer separate dictionary definitions;
  a lookup does not establish the meaning of a particular use. Optional
  terminology errors do not invalidate an accepted main answer or translation;
  a refused glossary merge keeps its original accepted definitions separate. Static
  glossary entries remain readable without scripting; JavaScript may reveal
  those same definitions beside the bound words. This adds no semantic graph,
  per-term request or provider call during saved rendering.
* Learn and Work are two entrances to this same report, sharing analysis,
  maps, sources and navigation. Learn is the default on first open: a short
  system explanation and a visible, progressively expanded system map, with
  paths into areas, run instructions and unfamiliar terms. The map is central,
  not hidden behind a catalogue. Work starts with search and the map for a
  concrete investigation. Switching modes preserves the component, scope,
  operation, zoom and inspector. Structure/Operations and contextual/all uses
  remain controls within the map, not additional report modes. With scripting
  disabled all sections and source links remain available in the HTML.
  The repository overview exposes the complete, finite component set as cards
  with existing purposes, source context and incident connections; the same
  graph remains available as an explicit connections view. Learn exposes the
  complete saved question menu and one answer at a time, with position and a
  named return. Work offers search at repository or component scope. Opening
  a part reveals its existing key code inside that part, with the chosen
  explanation below the map. Hover may preview or emphasize neighbours but
  never replaces the selected reading or opens another scope. Member cubes
  do not inherit the group's arrows.
* Learn questions come from a curated, repository-independent set of learning
  intents. The model uses the existing concepts, core parts, integrations,
  README and documentation to turn each intent into zero, one or several
  useful repository-specific questions. There is no question quota. Omit an
  inapplicable intent; lack of an answer is not evidence of inapplicability.
  Answers are short, distinguish original evidence from model interpretation,
  explain the terms needed here, and lead into the same map and exact sources.
  Unresolved relevant questions remain visible as unresolved. This uses the
  existing analysis and shared glossary, not a separate question knowledge graph.
* Overview page: what the repository is (roles with purpose + anchors), the
  targets as cards, the cross-target portals as a table
  (`GET /api/levels: front/src/service/http.ts:12 → backend/app/app.py:19`),
  negatives stated explicitly, and a run recipe with each command anchored to
  the manifest line it came from.
* Target page, in this order: Inbound (routes/triggers) → Entrypoints → Core
  groups → External calls/dependencies, then one main flow as an ordered list
  of steps with anchors, then dynamic execution, config, dead code, TODOs. Evidence is
  collapsed by default, deduplicated by `path:line`, path printed once, max 3
  shown with "+N".
* Connection sentences (the model's one-liners) are visible: on hover and in
  the selected node panel. Never screen-reader-only.
* Each target page shows only its own groups; cross-target edges render as
  stubs pointing to the other target's page.
* Provenance is visible in styling: fact (solid), model (marked as model,
  muted), claim (marked with source and age). A model-written sentence must
  not look like a heading of authoritative documentation.
* Anchors link to a permalink at the captured revision, and to the editor when
  one is installed. A missing editor never withholds the report. The owner's
  2026-09-08 standalone rule also keeps the HTML when an analyzed source is
  absent from that revision or changed locally: show the same path and line as
  ordinary non-clickable text with a `No source` hover explanation. Preserve
  its code cube, explanation and map navigation. Other source links remain
  active; neither committing nor running a server is required to publish.
* Banned on-screen vocabulary: retained, source-bound, authority, projection,
  selector, outcome, target contract, and raw selector strings like
  `python:backend:guard:main`.
* One page with sections, not many routes. Script size is not the constraint
  the owner cares about — "288 KB is very little" (2026-09-03) — but every
  answer must still be in the HTML: scripting may add preview and emphasis and
  nothing else, so the page reads with scripting off. Today it is 145 KB with
  5.4 KB of script.
* The templates are one file per region of the page and one file per style or
  script layer, under `internal/report/templates/{html,css,js}`, concatenated
  in filename order. Adding a region or a layer is a new file and no Go change,
  so working on the page does not mean working on that package.

## Acceptance (fixture)

`testdata/acceptance/python-tutorial-game` (revision `78714d34ee`) is the
canonical fixture. Its `expected.json` lists facts that must
be present with anchors. A report is acceptable only if a reader can answer
these from the report alone, without opening the repo: what is this, how do I
run it, where does the frontend talk to the backend and on which port, what
runs code it was given, what is dead, what is missing, and what is the main
flow from clicking a level to the animation.

## What this tool does not protect you from

repomap trusts the repository it is given and does not scan it for
credentials. Run directories and the model cache contain exactly what the
prompts and responses contained, so a credential committed to the analyzed
repository can reach them like any other repository text. Treat a run
directory as being as sensitive as the repository it came from. The provider
key is read from the environment and is never part of a request body or a
cache record. (Owner's decision, 2026-09-03: the scanner that used to refuse
such content was removed rather than kept and optimised.)

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
