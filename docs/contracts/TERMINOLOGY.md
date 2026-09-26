# Optional glossary

Current implementation contract. Read only the sections relevant to the change.
[Constitution](../CONSTITUTION.md) takes precedence; [CURRENT](../agent-room/CURRENT.md)
records the current product decisions and acceptance status. Historical runs and
experiments are in the [non-normative archive](../archive/2026-09-10/README.md).

## Separate generation and reduction

- Shared terminology belongs to `internal/terminology`. The owner-approved
  2026-09-10 correction collects accepted analytical prose and its complete
  source scope, then generates terms in separate aggregate requests. Main
  answers carry only their owning JSON contract, without optional terms or a
  wrapper. Generation and reduction use their own 32,768-token output allowance (legitimate windows produced 1,276–15,278 tokens; two 128,000-token repetition loops were measured on Watchtower) and the actual
  provider request envelope; no ordinary prose byte cap creates extra windows.
  Resource refusals partition complete original prose records. A failed glossary
  request cannot invalidate an accepted analytical answer. Terms must occur in
  accepted prose, found by the same [term lookup](#term-lookup) as the report,
  and select its advertised prose-row refs; they name
  [concepts, not code names](#concepts-not-code-names). Generation
  (`repomap.glossary.generate.v5`) has one `p*` catalogue and no separate
  source-ref namespace; Go restores the complete original source scope of every selected row, plus its analytical
  request and row, including warm reuse. A same-numbered old `g*` ref has no
  authority and is never repaired into a prose ref. Non-table owners declare
  their explanatory response paths; tables use their current prose columns
  and explicit empty-value spellings. Closed refs, states and unused fields
  do not become glossary text, while those same words remain valid in prose.
  Rejected rows contribute no prose or source authority. Exact request bytes and
  provider responses remain the cache/replay authority.
  Generation and reduction remember splittable output, context and response
  resource refusals through the shared exact-request split memo. A warm run
  rebuilds the complete children through the current owner and validates their
  cached or live answers; a cached whole-parent answer or replay takes precedence.
  Changed prepared bytes, provider state or limits do not inherit the refusal.
  NoCache bypasses it, and an indivisible refusal is not a split memo. Old run
  journals are not imported into this cache.
  One aggregate closed-ref reduction joins compatible domain candidates and
  chooses an original definition; when every group is a singleton and no two
  lowercased names meet, the reduction round is skipped and the catalogue is
  sealed as generated. It otherwise joins candidates and chooses a definition, preserving every variant, spelling, source
  and request identity. Each input group chooses one advertised original variant;
  Go joins groups that choose the same variant. Its owner must make that same
  choice, so chains, cycles and conflicting assignments cannot repair themselves
  into a grouping. Every input group still requires an explicit choice.
  A refused reduction window retains its already accepted
  input definitions separately, with incomplete comparison recorded and shown
  in the glossary and run output. Live response refusals record their reason
  in rejected.jsonl beside the exact exchange; provider-error text stays closed.
  An exhausted provider-local timeout remains an optional refusal while the
  owning run context is alive: generation supplies no definitions for that
  window, and reduction keeps its original entries separately. Actual run
  cancellation/deadline, local input, configuration and persistence failures
  remain errors. This does not change the shared transport timeout or retries
  and does not add a glossary attempt deadline. Native code
  concepts keep their existing definitions, exact anchors and destinations and
  enter the final glossary directly; they do not round-trip through the reducer.
  `terminology.json` and `glossary.json` retain domain candidates and the reduced
  catalogue, bound to `ReportData` before translation. Reduction and translation
  use the base provider without recursively collecting terms. Existing display
  translation keeps glossary names literally unchanged, with definitions as
  context, and translates definitions and surrounding prose. Code finds whole
  names in the final text, including English. The model supplies
  no occurrence markers, sense decisions or hint positions. Distinct meanings
  remain dictionary alternatives. [Term lookup](#term-lookup) preserves that distinction.

## Local owning context

Ordinary analytical prompts carry no `REPOMAP_PROSE_SOURCES_V1` appendix and no second `g*` provider namespace. The [shared executor](EXECUTION.md#local-response-context) preserves source/prose ownership locally through cache, entity memo, question memo and exact replay. Generation selects only its p-row catalogue. It never repairs an old g ref or borrows a refused neighbour’s text.

## Reducer completeness

After separate generation,
an aggregate closed-ref reducer joins compatible domain definitions and chooses
one original explanation. Go unions original spellings and sources and retains
all variants and request provenance. A reduction window lists at most six sample observations per variant beside `count`, the real number; the catalog entry keeps every source (Freqtrade `20260911-053911` sent 137 windows of 1.9–3.1 MB, 114 million input tokens, when every anchor of a common term rode into every window). Reduction request v6 asks for one
`{ref, representative}` assignment per input group. The representative is a
closed original variant ref; equal choices identify one output group. The group
owning that variant must make the same choice. Assignments are read row by
row. A missing, malformed, unknown or conflicting choice, and a chain or cycle,
refuse only the groups involved: that group, every group that chose one of its
variants, or exactly the groups of the chain or cycle keep their original
entries, while independent consistent joins in the same window survive. No
transitive repair or local insertion supplies an omitted choice, and a refused
group is recorded as partial comparison. A window with no accepted choice is
refused whole. Identical repeated assignments are
idempotent and unknown input refs are discarded. Earlier accepted groups stay indivisible and all original
variants remain visible to later comparisons. It does not classify translation policies.
Request-local source catalogues encode every distinct path/line once and every
distinct complete source set once. Each original variant references its exact
set; its full spelling and explanation remain present. Every partition builds
its own complete catalogues, without parent/sibling refs or source sampling.
Only the representation changes: output assignments still select original v*
representatives, and local restoration retains all original sources and origins.
Equal names alone never establish equal meanings. Complete groups partition only
when the provider envelope requires it; a nonshrinking round records partial
comparison. A refused model window leaves its already accepted input definitions
separate and stops retrying them in that reduction. Cancellation, invalid local
inputs/configuration and persistence failures remain terminal. Existing native
code concepts enter the final glossary directly, with whole source anchors and
map/question destinations. There is no native-to-candidate-to-native conversion.
Distinct declarations on one line keep their columns and identities; the same
exact declaration can retain memberships in several components.
`terminology.json` holds domain candidates; `glossary.json` and
`ReportData.Glossary` hold the sealed domain catalogue. Reduction and translation
use the base provider without recursively collecting another glossary. Saved
`read` collects candidates without stages beyond the requested stop.

## Concepts, not code names

Generation explains domain and concept terms only. The prompt asks each term
for a concept `kind` (`acronym`, `domain`, `protocol` or `format`). The kind is
neither stored nor shown, so a missing or other kind does not refuse a term;
only a term that declares itself the retired `identifier` kind (in any case) is
dropped alone. A term needs its name, explanation and prose rows; extra members
are ignored and a padded name is trimmed. A missing or null `terms` member is
an empty terms list, and a bare top-level array is the terms list. Self-runs had spent most generated output on `identifier` terms that code
then discarded (121 of 136 and 100 of 182 terms), on a serial 15–30 s step. The
embedded prompt therefore tells the model not to define names that the code
declares or reads: functions, methods, types, variables, constants, packages,
modules, files, paths, environment and configuration keys, command-line flags,
headers, commands and rule or ticket codes. A word, acronym, protocol or format
that code also uses as a name is still a concept; only its code spelling is
skipped.

The reader is an engineer new to the repository (owner decision 2026-09-25):
general engineering, computing and version-control vocabulary (repository,
commit, package, interface, cache, JSON, HTTP) is not explained; terms of the
repository's own field, and ones an engineer would have to look up, are. On
one saved self-run request the prompt without this reader drew 36–618 terms
(6–69 s, four of six draws above 240); with it 66–89 terms in 10–12 s in five
of six draws and 277 terms (41 s) in one.

After validation, Go also drops a valid term whose whole name equals, exactly
and case-sensitively, a code name that the glossary owner already holds and
that is in code spelling:

- ordinary run: every name declared in a published target's ProgramIndex
  (types, functions, methods and variables, which include locals, parameters,
  fields and enum members), package/module names as the index spells them (a Go
  import path, a dotted Python module), environment keys from `config_read`
  facts, and every corpus path and its file name;
- saved `read`: file and symbol declaration names in its places graph, plus its
  source-authority paths and their file names.

Code spelling means a separator or sigil that ordinary words do not carry
(`_ . / \ : $`, and `* ! ? < > =` for Clojure names), or a lower-case letter
directly before an upper-case one: `funding_rate`, `config.json`,
`internal/run`, `ExchangeWS`, `fetchTicker`. A single word, acronym, product
name or hyphenated word is ordinary vocabulary and survives even when code
declares the same spelling. Repositories name their code after their domain,
and an exact match against every declared name had dropped the concepts the
glossary exists for. On the saved freqtrade run (39,015 objects), it dropped
543 distinct names from 1,764 generated terms, including `candle`, `timeframe`,
`stoploss`, `pair` (a local), `ROI` (an enum member) and `RPC`. On ten saved
self-runs it dropped 74 of 586 concept-kind terms (26 names), including `JSON`
(method `Snapshot.JSON`), `API`, `SHA256` and `Python`. With code spelling, freqtrade
drops 393 names, all class, function, configuration-key or file spellings
(`ApiServer`, `stake_amount`), and keeps those words. The self-runs drop one
concept, `ProgramIndex` (a struct field; 4 occurrences). The rule catches 486
of their 1,173 former `identifier` terms rather than 791. Single-word code names
such as `Snapshot`, `Corpus`, `IStrategy` or `caplog` are left to the prompt,
because code cannot tell them from a word.

Lambdas and external symbols are not names this code owns. No package segment,
affix or case variant is inferred for this drop; it stays exact even though
[term lookup](#term-lookup) ignores case. No existing artifact records command-line
flag names, so flags rely on the prompt alone. Each drop is an accepted
decision journaled in `rejected.jsonl` as `glossary_code_name_omitted`, with
the name in its reason and a link to the exact exchange, on live and cached
answers alike. A window of only code names is accepted and publishes nothing.
Code names never enter the provider request. The changed prompt changes the
exact request bytes, so earlier cached generation answers are not reused.

## Visible comparison scope

Term previews put the `(via model)` badge beside `Term explanation`, outside
the definition sentence. The static glossary puts the same provenance in the
term heading. Questions, collapsed separately, and map destinations precede source context. Native
concepts retain their direct declaration anchors. Generated domain definitions
inherit the complete context of their selected analytical prose; the UI labels
that collection `Analysis context`, not direct evidence for the definition.
It starts collapsed, groups references by file, and reveals original locations
only when that file is expanded. Every distinct saved destination remains in
the static HTML, including editor and unavailable-source behavior. Presentation
does not select supposedly relevant lines or change glossary generation.

The ordinary run output and static glossary now expose the saved partial
comparison state. The glossary uses one quiet localized explanation before its
list; successful complete comparisons show no notice. This projects the existing
catalogue flag and introduces no new stored report format or provider request.
Live validation, provider and completion-envelope failures also write a rejection
record with an exact exchange link. Domain validation retains its actual reason;
provider failures use the existing closed error description, never raw transport
error text. Refused responses still cannot enter the accepted response cache.

## Term lookup

Lookup never proves an occurrence's meaning. Owner decision 2026-09-26: a name
  matches in any letter case, alone or followed by an English plural ending
  (`s`, or `es` after a name ending in s, x, z, ch or sh), and only as a whole
  word or phrase. Snapshot finds snapshot, Snapshots and SNAPSHOTS; Class finds
  classes; Go finds neither good nor goes; Snap does not find snapshots. The
  plural ending applies only after a Latin letter. The same rule
  (`internal/terminology/lookup.go`) decides a generated term's source-backed
  occurrence and the report's render-time lookup. Identifier/script boundaries
  stay: a letter, digit, mark or underscore (and `$` in display prose)
  continues a word, while a letter of another concrete script does not, so
  Matchers를 names Matcher. The longest overlapping name wins, a name that needs
  no plural ending wins an equal span (Matchers over Matcher), and highlights
  never nest. Spellings equal but for case are one lookup name: they offer
  their definitions together, and a question's or entry's own sense replaces
  the others for all of them. The translation dictionary carries the names
  found this way under their original spellings, so saved translations of an
  earlier run whose texts now find another name need an ordinary run;
  `repomap render` rejects them rather than adapting them. Commands,
  code, links and source placeholders remain independently protected. Exact
  display refs bind local spans to their prose slots, including dynamic map
  descriptions; equal text alone never identifies a slot. No aliases, other
  morphology (`-ies`, irregular plurals, verb forms), semantic repair or
  classifier call is added when translation changes a name.
  The glossary is static; browser hints reveal those same definitions beside
  bound prose. `repomap render` uses saved report and translation data with zero
  provider calls.

The owner's clarified 2026-09-08 design keeps every glossary name in its original
spelling. A later clarification adds an optional English Alias beside a native
declaration name, including in Russian reports. Symbols and Types request
that short label together with the existing explanation. The accepted cell flows
through Knowledge, atlas and GroupsIndex into the ordinary report. Native
names, IDs, locations and source links are unchanged. The alias is display prose,
not a new observation or another graph. The renderer does not infer a name from
its alphabet or shorten a description into one. Cards show the short alias with
the native code name; the full translated explanation belongs to the selected
detail. Both saved names can lead to one glossary definition by term lookup.

The 2026-09-10 correction extends the same alias binding to an operation that
repeats its native declaration name: cards, map nodes and operation links show
the accepted English alias beside that original name. This includes continuous
and scheduled work. A distinct action label keeps its own meaning; commands and
paths retain their literal spelling. Operation names are excluded from translation, including
remote map references; descriptions still change language. The regression uses
a Korean declaration and checks both English and Russian rendered reports with
the original source and operation links. An ordinary run rebuilds the changed
display catalogue; old saved translations are not silently adapted.
