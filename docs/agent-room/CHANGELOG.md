# Implementation and acceptance journal

This is a concise living log, not another ADR. [CURRENT](CURRENT.md) owns the
current decision/status; [topical contracts](../../AGENTS.md#start-here) own
implementation details. Record a changed decision there in place. Append only
useful implementation/acceptance evidence here; archive superseded detailed
runs instead of growing the entry pages again.

## 2026-09-11 — current correction wave, ordinary acceptance pending

- The Learn/Work switch is removed from the report toolbar: the owner could
  not tell the two entrances apart, and the switch only hid the intro and
  question list or moved the search field. Summary, questions, map and the
  toolbar search are always present; the per-component entrance keeps its
  question links and the component search button; an old `?mode=` link is
  ignored. Print and JavaScript slice tests re-anchored.
- Go handlers registered as method values or method expressions
  (`mux.HandleFunc("/items", s.handleList)`, `http.HandlerFunc(s.handleCreate)`)
  now resolve to their methods: the callable resolver follows a synthetic
  bound-method wrapper or thunk to the method it delegates to. Before, the
  route fact had no handler and the handler no route, so a page listed
  `GET /test` under registrations and a separately labelled operation for
  the same handler. A wrapper around an interface method keeps resolving to
  nothing static; a middleware's returned closure still resolves to that
  closure.
- A sequence cell citing only refs outside its row's options, or nothing
  at all, is an empty selection and no longer refuses the row: an owner's
  symbol window answered `outbound: "c9 c12"` where those refs were context
  calls, not options, and lost its key-symbol and activation decisions with
  the row. Commas now separate refs like spaces. Exceeding the limit still
  refuses the cell.
- The "Where this service connects" catalogue groups communication records by
  destination: one row per destination text (case-insensitive; native label
  or kind when the model named none) with the record count, the shared kind,
  basis and address, and the first sentence of one purpose; every record keeps
  its own row beneath the group. An owner's Go service showed ten
  "Kubernetes API server" paragraphs and twenty "Postgres" ones as separate
  rows. Rendering only: no saved data changes; the first screen and the
  section count show groups.
- An operation label the model left empty or null takes the first sentence of
  the row's own description (`name_from: description`). An owner's Go service
  on another DeepSeek provider received rows with every schema key present and
  `"name": null` beside a complete description, and each such row was refused
  as `cell "name" is empty`; the official API writes the label. An empty
  description still refuses the row. Decoder rule only: requests and memo
  state are unchanged.
- Orientation is bounded by construction again: at most 40 members per group
  (`member_count` keeps the real size), 6 calls and 6 callers per listed
  member's evidence with omitted counts. After `956e5992` removed the
  12-member cap and added evidence for every listed member, the Freqtrade
  orientation request grew from 1.3 MB (accepted at 330,422 tokens on
  20260910-144751) to 22.9 MB, and the provider's context refusal failed
  publication of a 35-minute run (20260911-045158). Bounded on the same saved
  input: 1.82 MB (2,550 listed members, 258 evidence entries, largest 16.6 KB).
  A provider or preparation refusal by size or context now leaves an empty
  orientation with the refusal in `rejected.jsonl`; the report is published.
- Native boundary scopes stay within the targets holding the file, and the
  target projection omits a boundary whose file the target does not hold.
  After `956e5992` widened native scopes to every observing index view,
  Freqtrade `20260911-040254` (one route fact seen from seven views, file held
  by one product) and an owner's Go service failed at publication with
  `atlas: boundary … names unknown box`. Regression tests in `places`,
  `reading`.
- A complete keyless response is read in asked order: the model drops the
  `key` it was told to copy in small answer windows (seven one-question
  windows of `20260911-040254`, then two- and three-question windows of
  `20260911-045158`, ten of 242 windows), and each such window lost its
  answers as `response row has no string key`. One row per asked row and no
  key anywhere leaves the asked order as the only reading; a partial or
  partly keyed response is still refused row by row.
- A closed choice whose only option is `unknown` accepts any answer as
  `unknown`. Freqtrade `20260911-040254` refused 27 outgoing boundary rows
  (7 whole windows) because the model copied the observed path into
  `address` where `address_options` was `["unknown"]`; their kind, line and
  destination were lost with them. An offered `a*` ref still has to be chosen.
- Restoring an empty selected file set no longer fails a run: a Go repository
  whose only Python file is a test script (air) has a Python catalog but no
  required Python target. Both adapter restorers return nothing for an empty
  selection; a non-empty one still fails closed. Regression test in `run`.
- Glossary generation/reduction output allowance is 32,768 tokens instead of
  the shared 128,000. Measured legitimate windows: 1,276–15,278 output tokens
  (Syn, issue-bot, Watchtower); measured loops: two Watchtower windows at
  128,000/128,003 tokens, 318 s and 519 s, 15 of the run's 21 provider
  minutes. The existing resource-refusal split still halves a refused window.
  Saved-window probe before the change: `scratchpad/glossary-cap-probe`.

- Third ordinary series (binary `664e34c`): Syn 224.602 s, issue-bot 574.736 s,
  Watchtower 306.593 s; 30/20/27 questions, all available. Source/prompt and
  provenance receipts are in external `work/small-repo-audit-20260910/next-*`.
  Native and operation controls passed; answer acceptance still exposed the
  listener/worker distinction, a differing GitLab comment argument and a missing
  required template argument. Their current prompts now state those requirements;
  controlled full-window checks and ordinary follow-up remain pending.
- Glossary generation/reduction now use the existing exact-request resource
  refusal memo; current cached/replayed whole answers keep priority. Exhausted
  provider-local deadlines take optional refusal while actual run cancellation
  remains terminal. Focused terminology/LLM/translation tests and vet pass;
  ordinary warm acceptance remains pending. No timeout, output cap, cache type
  or old-journal migration was added. The observed issue-bot delay was a
  309.809 s, 128k-output repetition followed by complete 31/53-row children.
- The third Syn report exposed a display error: two native registrations plus
  an unmatched proxy interpretation were labelled three routes. Route summaries
  now count native registrations; the combined catalogue identifies its records
  and unmatched handlers separately. Existing rows/anchors are unchanged. Focused
  report tests and vet pass; saved rendering/browser verification follows the
  current frozen-binary series.
- Operation v19 states the positive task-and-activation requirement in the
  existing entry decision. Watchtower's actual v18 bytes already prohibited
  listener and dispatcher promotion, but two model rows still did so. The
  clarification adds no new evidence, judge or local role repair; its effect
  needs the next ordinary run.
- Watchtower listener/route projection: graph 14 / saved input 15 / atlas 7
  retain native target origins and the fixed `listen_address` kind. Exact
  paths/methods/columns no longer collapse into one fact. Shared-source context
  follows the compiler-located subject; each target restores its own FactID and
  ObjectID. Focused sealing, owner-context, method/path, fixed-fact and complete
  reading→GroupsIndex→catalogue tests pass. A local projection of compatible
  saved Watchtower native inputs retained its two route observations plus one
  listener, with both targets' exact route identities (0.50 s, no provider).
  Genuine sealed game adapters through current facts/places produced graph
  SHA-256 `ded06f4a626388e0a80db4b12d788d570057c2197802ef702a8e800d4995d19a`.
  This is local validation; existing model wrapper operations and a fresh
  ordinary Watchtower report remain separate acceptance work.
- Boundary v5 distinguishes `remote_client_instance` from an option for a
  later constructor. Two complete saved windows were prepared through the
  current owner and replayed once each: issue-bot retained NewClient and five
  dispatches, WithBaseURL became none (2.733 s); service retained OTLP New and
  HTTP Do, while the local wrapper became none (2.030 s). Current validation
  accepted 7/7 and 3/3 rows without rejection; full original row contexts were
  equal. This is a contract check, not ordinary report acceptance.
- Watchtower's glossary reducer repeated 87,780 source records in 4.73 MB and
  exceeded provider context twice. All 458 accepted variants survived, but the
  catalogue remained partially compared. Reducer v5 factors exact anchors and
  source sets within each independent request. The same full saved window is
  502,076 bytes; one supported replay passed in 15.503 s with 155,669 input and
  5,502 output tokens. Current validation accepted all 458 assignments into 412
  groups, retaining every original variant/source/origin. Full ordinary acceptance
  of the changed reducer remains separate from this saved-window check.
- Syn's actual operation request omitted native receiver/argument context and
  classified middleware as additional incoming work. Operation v18 retains its
  original own-call observations and separates same-request middleware from an
  independently activated endpoint/consumer. Actual cumulative Go contrasts,
  consuming request/decision tests, full reading/lines tests and scoped vet pass;
  corrected model decisions await the ordinary rerun.
- Operation v17 uses own `self`/`none`; fixed-boundary v4 no longer asks a model
  to decide the existence/kind of a native fact. Exact routes and accepted
  worker rows remain independent of captions/setup callers. Focused
  lines/reading tests and vet pass; ordinary classification needs reruns.
- Question-batch v3 validates relevance per selected anchor. Native calls retain
  receiver/source arguments, API and same-line sites in question and boundary
  evidence. Orientation includes native member evidence. Document ancestry is
  carried with author scope (graph 13 / reading input 14), not used as a hard
  file-role exclusion. Integrated ordinary answer quality remains pending.
- Native declared-interface calls retain their original unresolved dispatch
  observations in graph 13 / input 14. Source-ordered Python parameter types
  can supply declared method alternatives, with reassignment/unknown controls.
  Data query/table links are reversible and operation links require existing
  exact model or callable ownership; embedded literals gain no guessed owner.
  Ordinary source-chain quality remains under acceptance.
- Deferred prose source appendices were removed from provider messages.
  Prepared/existing accepted cache v3 retain compact local context once; current
  warm execution, entity memos, original-window question memos and replay keep
  original provenance. Seven consuming package suites/vet pass; race-enabled
  parallel/warm/replay regressions pass. Receipt:
  `work/small-repo-audit-20260910/local-prose-context-fix.md` in the external
  project work directory.
- Glossary generation/reduction now use the shared 128,000-token allowance.
  The saved Watchtower window that stopped at 8,000 completed with 8,903 output
  tokens in 28.803 s (21,040 input; finish=stop). Model/user evidence and p refs
  were unchanged. Of 216 term rows, 211 have exact occurrences; five unsupported
  optional names are discarded. Structure/closed refs/text are checked; original
  source provenance still requires ordinary acceptance.
- The saved issue-bot response accepts 21/21 question rows under per-anchor
  validation (previously 18/21), without a provider call. This is an offline
  decoder check, not a corrected ordinary report.
- The real Python16 Freqtrade native check retains `/trades` →
  `RPC._rpc_trade_history` → `Trade.get_trades_query` as alternatives, including
  the original Trade owner. This closes that observed native frontier, without
  claiming runtime execution or a completed model/report acceptance.
- Orientation preparation retains complete author claims and all validated group
  members with native evidence; the prepared provider envelope remains the actual
  limit. Fresh ordinary orientation quality remains pending.
- Short AGENTS/CURRENT entry pages now route to topical contracts. Full prior
  text is preserved in the [non-normative archive](../archive/2026-09-10/README.md).
  Required native fixtures, executable expectations, package bounds, ordinary
  runs, browser QA, warm/cache-clear checks and constitution supremacy remain.

Next: complete the combined build/check checkpoint, verify the corrected small
ordinary reports, then complete Freqtrade acceptance before full Airflow. Do not
mark those outcomes complete from probes or local tests.
