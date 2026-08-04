# Decision: Semantic Discovery MVP

**Status**: Approved for implementation

**Approved by**: Repository owner in the current implementation session

## User-visible goal

Turn the saved repomap model into a small evidence-backed learning layer. From
already captured artifacts, discover a few different things worth explaining
about the repository, render them as `Explore this repository` cards, connect
their steps to the existing architecture canvas and evidence navigation, and
index them in the existing Super Search.

Search remains enabled by default. `repomap --no-search <repo>` removes only
the search index, field, suggestions, assets, and keyboard shortcut; semantic
discovery, guided onboarding, and the architecture report remain unchanged.

## Options considered

1. **One monolithic repository-documentation call.** This is mechanically
   small, but combines opportunity selection, fact extraction, editorial
   synthesis, and coverage judgment in one response. It is hard to validate or
   partially reuse, and one rejected response removes the entire experiment.
2. **Extend only the existing guided-story editor.** This reuses a strong UI
   path, but forces dependency usage, contribution guidance, and learning
   topics into a single flow-shaped object and cannot test which projection is
   actually useful.
3. **Add one bounded saved-artifact semantic stage with opportunity scan,
   independent evidence leaves, local reduction, and an optional compositional
   story.** This adds a narrow artifact contract and report projection while
   reusing the existing provider, architecture canvas, evidence IDs, and search
   index. Each leaf remains independently useful; missing evidence degrades one
   artifact instead of the report.

Option 3 is approved for the MVP.

## Approved scope

1. Build a deterministic, bounded semantic evidence bundle exclusively from
   artifacts already saved in a run: orientation, compact LLM bundle, coherent
   components and flows, bounded package/import summaries, source signals,
   runtime catalog when present, tests/docs in focused flow bundles, model
   research facts/frontiers, and a replayable guided story. Do not read source
   files or launch repository analysis. Saved model-authored orientation,
   component, research, and guided-story prose may appear only as ID-less
   opportunity-planning context; it is never leaf evidence.
2. Give every model-visible fact an opaque local ID. Keep paths, symbols,
   package names, locations, relations, certainty, and evidence only in the
   local fact catalog. Only deterministically extracted facts and exact local
   proofs receive support IDs. Model responses may refer to exact IDs but may
   not create repository objects or repository-bearing identifiers.
3. Run a bounded opportunity scan that proposes semantic candidates rather
   than facts. Reject unknown IDs and support-free candidates, preserve missing
   information, deduplicate locally, and select at most five candidates across
   distinct user needs.
4. Prefer an experiment set containing a central mechanism, dependency usage,
   contribution pattern/guide, Go-learning topic, and one story when current
   evidence supports them. `insufficient_evidence` is a valid result and must
   remain visible rather than being repaired with model prose.
5. Fan out by one local question or mechanism, not one file or symbol. Every
   leaf sees its own original fact subset and returns atomic observations,
   exact support IDs, candidate connections, contradictions, missing evidence,
   and an optional insufficient-evidence state. No leaf decides whether a
   repository-wide story is proven.
6. Run fan-in whenever at least one validated observation or meaningful
   missing-evidence result exists. Fan-in receives the original bounded local
   facts alongside validated leaf output. It may compose an explanation but
   never use a model summary as the sole evidence for a claim.
7. Materialize accepted artifacts locally. Mark claims as `direct`,
   `compositional`, `interpretive`, or `unresolved`; resolve all component,
   flow, surface, evidence, and artifact references locally; reject dangling
   IDs and unsupported direct/compositional claims.
8. During product iteration, always execute fresh Semantic Discovery provider
   calls; do not read or write the application-level stage-response cache for
   opportunity, leaf, fan-in, or monolithic stages. This does not disable the
   provider's own prompt caching and does not remove local replay of an accepted
   `semantic_artifacts.json` record. Use a stable common instruction prefix for
   leaves, thinking/high for ordinary semantic tasks, and max only for global
   planning/coverage. Record calls, bytes, tokens, provider prompt-cache
   hit/miss tokens, validation outcomes, and wall time in the saved stage
   artifact.
9. Add a small `Explore this repository` card section and artifact detail
   panel. Reuse existing canvas selectors for component/flow/surface focus,
   existing evidence opening, step navigation, and return to the full map. A
   failed or absent stage leaves the old report intact.
10. Add accepted semantic artifacts to Super Search with title, summary,
    aliases, likely questions, kind, exact artifact target, and focus IDs.
    Prefer semantic artifacts for suggested questions without redesigning the
    lexical ranking engine.
11. Add `--no-search` as a presentation-only option. It must not alter the
    semantic model-call plan or remove semantic artifacts from saved report
    data.
12. Provide a saved-run developer entrypoint so the self experiment can reuse
    existing repomap artifacts without repeating runtime surface discovery.
    Compare a bounded monolithic baseline with the decomposed result on useful
    coverage, unsupported claims, tokens, wall time, provider prompt-cache
    reuse, local verifiability, and partial degradation.
13. Generate the repomap self-report from the existing saved run and leave an
    ignored, uncommitted findings file beside it with the requested qualitative
    and quantitative assessment.

## Constraints

- No repository-wide call graph, package reload, SSA build, source scan,
  embeddings, runtime Q&A, MCP, state-machine analyzer, or new universal
  renderer.
- In particular, do not run the known long runtime-surface discovery stage.
- Never send raw `file_tree`, raw `internal_edges`, full repository contents,
  or a path outside the saved provider allowlist to the model.
- Do not force every repository to produce the same artifact kinds and do not
  generate one artifact per package or dependency.
- A model response is editorial interpretation, never the authority for a
  file, symbol, relation, flow, surface, runtime order, certainty, or evidence.
- The semantic artifact contract is an MVP report object, not a parallel
  architecture model. Existing orientation, components, flows, surfaces,
  guided story, canvas, inspector, and report pipeline remain authoritative.
- Missing, rejected, stale, or partial semantic output must degrade locally and
  must not prevent report generation.

## Acceptance

The experiment is complete when a saved-artifact repomap run can produce and
locally replay several distinct semantic artifacts, including honest
insufficient-evidence outcomes; accepted claims resolve to current local IDs;
cards and step navigation reuse the current map and evidence UI; artifacts are
searchable by default; `--no-search` removes the complete search surface while
retaining those artifacts; provider prompt-cache/token/wall-time diagnostics
and the monolithic comparison are saved; the self findings identify the best
and worst artifact kinds plus the highest-value missing deterministic fact;
and repository checks pass without rerunning runtime surface discovery.

## Owner override: application response caching

On 2026-07-15 the repository owner explicitly removed application-level
response caching from Semantic Discovery for the current product-iteration
phase. Repeated Semantic Discovery runs must call the provider again. Existing
orientation, targeted-research, and guided-stage caches are outside this
override. Provider-reported prompt-cache hit/miss tokens and local replay of an
already accepted semantic record remain part of the experiment.
