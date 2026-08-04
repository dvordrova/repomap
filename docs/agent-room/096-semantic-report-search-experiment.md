# Decision: Semantic report search experiment

**Status**: Approved for implementation

**Approved by**: Repository owner in the current implementation session

## User-visible goal

Add a persistent `What do you want to understand?` entry point and an offline
`Cmd+K` / `Ctrl+K` palette to the existing HTML report. Search should lead a
new reader to an already saved story, architecture object, explanation, known
gap, or exact source reference instead of behaving like a source-code grep or
an online chat.

## Options considered

1. **Search only inside the architecture canvas.** This is the smallest UI
   patch and can reuse canvas maps directly, but it omits orientation concepts,
   known gaps, and reports that gracefully fell back without a rich canvas.
2. **Build an ad-hoc index from `DATA` in browser JavaScript.** This keeps the
   saved schema unchanged, but duplicates coherence rules in presentation code
   and makes target validation and deterministic regression tests weaker.
3. **Build one bounded report projection after existing coherence joins, then
   render it with a small report-level palette.** This adds a narrow optional
   view-model field and UI asset, but reuses the current artifacts and exact
   canvas actions, is locally testable, works offline, and can be removed
   without changing any analyzer.

Option 3 is approved for the experiment.

## Approved scope

1. Derive a bounded deterministic search index from the coherent `ReportData`
   assembled after component, surface, saved-flow, suggestion, and optional
   guided-story replay. Do not read repository files or run a new analyzer.
2. Prefer already explained semantic objects: guided story and steps,
   architecture map, components, accepted mechanisms and saved flows, runtime
   surfaces, existing domain concepts, and known gaps. Exact flow-step names,
   packages, evidence locations, and allowlisted file paths are a secondary
   search layer.
3. Store only existing titles, summaries, IDs, paths, and bounded aliases. A
   search item may have a run-scoped UI identity, but its action must resolve
   to an exact current story step, component, flow, surface, overview, or
   allowlisted location.
4. Validate all action targets locally while building the index. Invalid or
   unavailable targets are omitted; index construction failure must leave the
   old report usable.
5. Add a persistent top-level search field, five to eight locally selected
   questions, an offline palette, grouped results, keyboard navigation,
   `Escape`, `Enter`, and the platform shortcut. Reuse the existing canvas and
   source-opening authority; add only small public canvas selectors needed for
   guided and flow steps.
6. Use deterministic lexical scoring with bounded Russian/English retrieval
   aliases. Aliases may improve discovery but never add claim text or change
   the selected object's displayed explanation.
7. Keep exact symbol, package, endpoint, state, and file matches visibly
   accessible even when a higher-level explanation ranks first.
8. Generate the self-report from the already saved repomap run, evaluate the
   eight approved natural-language queries, and leave an ignored, uncommitted
   findings file beside the run.

## Constraints

- No runtime LLM, embeddings, vector database, RAG pipeline, backend search,
  chat interface, or full-source indexing.
- No new repository-wide call graph, package load, SSA build, source scan, or
  runtime-surface discovery. In particular, the known long discovery run must
  not be started for this experiment.
- Search is a presentation projection, not another repository model. It cannot
  create components, files, symbols, flows, relations, surfaces, evidence,
  states, or explanations.
- Model-authored prose is rendered only from already locally accepted report
  artifacts and only with its validated existing references.
- Source actions remain limited to existing `OpenablePaths` and server-issued
  source authority. Static HTML falls back to an owning semantic object or an
  honest visible path; search grants no new filesystem capability.
- Reports without a usable search index continue to render and navigate as
  before.

## Acceptance

The experiment is complete when a static report exposes the persistent field,
suggested questions, and keyboard palette; the palette can focus existing
story steps, components, flows, and surfaces; exact names remain reachable;
no-result states make no unsupported claim; all targets are locally validated;
the eight self-report queries have recorded outcomes; the old guided/full-map
experience remains intact; and repository checks pass without rerunning the
long runtime-surface discovery stage.
