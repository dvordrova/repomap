# Optional glossary

Current implementation contract. Read only the sections relevant to the change.
[Constitution](../CONSTITUTION.md) takes precedence; [CURRENT](../agent-room/CURRENT.md)
records the current product decisions and acceptance status. Historical runs and
experiments are in the [non-normative archive](../archive/2026-09-10/README.md).

## Separate generation and reduction

- Shared terminology belongs to `internal/terminology`. The owner-approved
  2026-09-10 correction collects accepted analytical prose and its complete
  source scope, then makes the glossary in separate aggregate requests. Main
  answers carry only their owning JSON contract, without optional terms or a
  wrapper. Generation is [three steps, each one decision](#three-glossary-steps).
  Generation and reduction use their own 32,768-token output allowance (legitimate windows produced 1,276–15,278 tokens; two 128,000-token repetition loops were measured on Watchtower) and the actual
  provider request envelope; no ordinary prose byte cap creates extra windows.
  Resource refusals partition complete original prose records or names. A failed glossary
  request cannot invalidate an accepted analytical answer. A name must occur in
  accepted prose, found by the same [term lookup](#term-lookup) as the report;
  terms are [concepts, not code names](#concepts-not-code-names). No step has
  a source-ref namespace: Go restores the complete original source scope of
  every row that writes the name, plus its analytical request and row,
  including warm reuse. An old `g*` ref has no authority and is never repaired
  into a prose ref. Non-table owners declare
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
  The same name with the same explanation is one definition: code joins it,
  with the union of its sources and origins, before reduction.
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
  remain errors. Persistence means the glossary's required artifacts
  (`terminology.json`, `glossary_names.json`, `glossary.json` and the run's
  `rejected.jsonl`, whose failed append is the executor's observer issue). The
  response cache and the term-decision memos are not among them: a cache read,
  write or eviction that failed, or a memo that could not be saved, leaves the
  executor's answer as it is and is printed as a Glossary notice ("cache read
  failed", "cache write failed", "cache eviction failed"), with the same
  `llm.Issue.Recoverable` line the atlas reading draws (review A7, 2026-10-03:
  such an issue once stopped the report before translation and HTML). A split
  memo that cannot be read or saved is still an error here, as in every other
  adaptive owner. This does not change the shared transport timeout or retries
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

## Three glossary steps

Owner decision 2026-09-28. The earlier single request ("define the unfamiliar
terms in these rows") expressed each inclusion only by leaving other names
out, so one draw decided the glossary's size: on the same saved litestream
window its answers fell into about 40–47 terms or 263–288, one draw looped to
the output ceiling, and most of the extra terms were general words, parts of
the code in plain words and paraphrases. The glossary is now three steps, each
one decision, and code joins them.

1. **Names** (`repomap.glossary.names.v1`, text model,
   `prompts/names.md`): the prose rows as one `p*` catalogue; the answer is
   `{"names":[…]}`, names as the prose writes them, with no explanation and no
   rows. A missing or null member is no names, a bare array or one string is
   the list, and a `{name}` object is its name. Code keeps a name only when
   the term lookup finds it in that window's prose; an identical repeat is one
   name, and a window none of whose names the prose writes is refused. Code
   then makes one term of the names the lookup treats as one (case, English
   plural), keeps the spelling the prose writes most often, and attaches every
   collected prose row that writes it, from every window. The model never
   selects rows, so a name is never answered once per row.
2. **Decision** (`repomap.glossary.term.v1`, the categorizer): one closed
   question per name, with three options and criteria for each
   (`prompts/term_options.md`): `domain_concept`, a concept of the program's
   field or of a technology it is built around that a newcomer would look
   up; `general_vocabulary`, words any engineer knows, or a phrase whose
   meaning is its known words put together; `code_element`, a name whose
   meaning only this program's own source or report gives. The state
   (`prompts/term.md`) says what the glossary is for, the shared context is
   the report's orientation summary (none in saved `read`), and the item is
   the name with every prose text that writes it, as written.
   `FitClassifierWindows` packs whole items; nothing is left out, and an item
   larger than one request goes alone for the provider to accept or refuse.
   The categorizer margin decides; a near-tie is `undecided`, and a name that
   no accepted answer reached is `unanswered`. Every decided or undecided
   answer is remembered per name and item, so a warm run asks nothing and a
   near-tie is not re-asked for a clearer draw. Only a decided
   `domain_concept` goes on. Every name's outcome is saved in
   `glossary_names.json`; declined and undecided names are journaled as
   `glossary_name_declined` and `glossary_name_undecided`.
3. **Explanations** (`repomap.glossary.explain.v1`, text model,
   `prompts/explain.md`): one closed `t*` ref per accepted name, with its name
   and the `p*` refs of its rows; the prose catalogue holds exactly those rows.
   The answer is one `{ref, explanation}` per ref. An unknown ref is dropped;
   a missing, empty, `none` or conflicting explanation leaves only that name
   out; an identical repeat, a padded or upper-case ref and extra members are
   harmless; a window that explains no name is refused.

Measured on the two saved litestream generation windows (`61235e8f…` and
`40bf2f66…`, five draws each): the single request drew 45–248 and 49–270
distinct terms (union 264 and 276; 42 and 34 in all five draws; up to 14,989
output tokens). The names step drew 143–181 and 155–271 names in 553–1,667
output tokens; 54–58 and 47–61 of them were accepted (union 62 and 74; 52 and
39 in all five), among them LTX file, TXID range, lock page, shadow WAL,
storage class, WAL segments, salt and multi-level compaction. What still
varies is whether a draw names a concept at all. Two independent decision
passes over the same names changed 13 of 207 and 28 of 344 outcomes, every
one between a decided option and `undecided`, never between two decided
options. No term cap, stop-list, smaller output allowance, render filter or
retry was added.

## Local owning context

Ordinary analytical prompts carry no `REPOMAP_PROSE_SOURCES_V1` appendix and no second `g*` provider namespace. The [shared executor](EXECUTION.md#local-response-context) preserves source/prose ownership locally through cache, entity memo, question memo and exact replay. Generation reads only its p-row catalogue. It never repairs an old g ref or borrows a refused neighbour’s text.

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
Equal names alone never establish equal meanings, and sources never decide a
join (owner decision 2026-09-28): groups with the same name come from
different places of the report and join when their explanations describe the
same concept, whatever their sources; different names join only as spellings
or aliases of one concept. Complete groups partition only
when the provider envelope requires it; a nonshrinking round records partial
comparison. A refused model window leaves its already accepted input definitions
separate and stops retrying them in that reduction. Cancellation, invalid local
inputs/configuration and persistence failures of required artifacts remain
terminal; cache diagnostics are notices (above). Existing native
code concepts enter the final glossary directly, with whole source anchors and
map/question destinations. There is no native-to-candidate-to-native conversion.
Distinct declarations on one line keep their columns and identities; the same
exact declaration can retain memberships in several components.
`terminology.json` holds domain candidates; `glossary.json` and
`ReportData.Glossary` hold the sealed domain catalogue. Reduction and translation
use the base provider without recursively collecting another glossary. Saved
`read` collects candidates without stages beyond the requested stop.

## Concepts, not code names

The names step asks for concepts only, and the decision's `code_element`
option leaves out a part of the program written in words. The earlier single
request asked each term for a concept `kind` (`acronym`, `domain`, `protocol`
or `format`) after self-runs had spent most generated output on `identifier`
terms that code then discarded (121 of 136 and 100 of 182 terms); the kind is
retired with that request. A word, acronym, protocol or format that code also
uses as a name is still a concept; only its code spelling is skipped.

The reader is an engineer new to the repository (owner decision 2026-09-25):
general engineering, computing and version-control vocabulary (repository,
commit, package, interface, cache, JSON, HTTP) is not explained; terms of the
repository's own field, and ones an engineer would have to look up, are. On
one saved self-run request the single-request prompt without this reader drew
36–618 terms (6–69 s, four of six draws above 240); with it 66–89 terms in
10–12 s in five of six draws and 277 terms (41 s) in one.

After the names step, Go also drops a found name whose whole name equals,
exactly and case-sensitively, a code name that the glossary owner already
holds and that is in code spelling:

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
such as `Snapshot`, `Corpus`, `IStrategy` or `caplog` are left to the names
step and the decision, because code cannot tell them from a word.

Lambdas and external symbols are not names this code owns. No package segment,
affix or case variant is inferred for this drop; it stays exact even though
[term lookup](#term-lookup) ignores case. No existing artifact records command-line
flag names, so flags rely on the names step and the decision. Each drop
is an accepted decision journaled in `rejected.jsonl` as
`glossary_code_name_omitted`, with the name in its reason ("name is a code
declaration: …") and a link to the exact exchange, on live and cached
answers alike. A window of only code names is accepted and keeps nothing.
Code names never enter the provider request.

## Visible comparison scope

Term previews put the `(via model)` badge beside `Term explanation`, outside
the definition sentence. The static glossary puts the same provenance in the
term heading. Questions, collapsed separately, and map destinations precede source context. Native
concepts retain their direct declaration anchors. Generated domain definitions
inherit the complete context of their selected analytical prose; that context
stays in `terminology.json`/`glossary.json` and in the linked questions, and
it is not the list of files a term appears in: the glossary once listed
eighteen files "in which" event loop appeared, lzf.h and solarisfixes.h among
them. The page lists `Files in which this term appears`: every line of the
corpus's readable text files that writes the spelling, found by the shared
[term lookup](#term-lookup) after reduction in the ordinary run and saved as
`glossary_occurrences` in `report.json` (a code fact, not evidence for the
definition). It starts collapsed, groups lines by file and reveals them when
that file is expanded, with editor and unavailable-source behavior. A spelling
no read file writes lists no files, and a saved report without that record
lists none. Presentation does not change glossary generation.

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
  plural ending applies only after a Latin letter, and after an acronym (a
  name written in capitals only) it must be written in lower case: API finds
  APIs, HTTP does not find HTTPS. A native code declaration's name is code and
  matches only as written, with no plural: Result opens on Result, never on
  "result" or "Results" (a self-run otherwise went from 132 to 404 code-name
  underlines, 240 of them offering several definitions). The same rule
  (`internal/terminology/lookup.go`) decides a generated term's source-backed
  occurrence and the report's render-time lookup. Identifier/script boundaries
  stay: a letter, digit, mark or underscore (and `$` in display prose)
  continues a word, while a letter of another concrete script does not, so
  Matchers를 names Matcher. The longest overlapping name wins, a name that needs
  no plural ending wins an equal span (Matchers over Matcher), and highlights
  never nest. Spellings equal but for case are one lookup name: they offer
  their definitions together, and a question's or entry's own sense replaces
  the others for all of them. A reduced entry that joined such spellings
  (Zipkin, zipkin) has one identical definition per spelling; the name offers
  it once. The translation dictionary carries the names
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
that short label in their existing tables, only for a name with a letter
outside the Latin script (owner decision 2026-09-26; see
[Reading](READING.md#type-and-concept-descriptions)). The accepted cell flows
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
