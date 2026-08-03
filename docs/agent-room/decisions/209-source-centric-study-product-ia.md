# Decision 209: Source-centric Study product IA and saved-route recovery

## Status

Approved explicitly by the repository owner in the current session after the
D208 Casdoor product review.

This decision is provider-free. It does not change the Architecture or Atlas
Study provider request wire, prompt or response JSON shape, and it authorizes no
live model call. It does change local Atlas Study response acceptance and the
report projection, so those identities must advance and old identities must
miss closed.

## Problem

The D208 report is truthful but still presents local source evidence as
internal scaffolding. Study repeats file paths and generic headings while the
exact function is the useful object. Route cards hide exact source actions
behind one oversized navigation button. Generic Architecture links duplicate
the primary navigation. The repository Shape renders as one misleading
"important area" when only one of several areas has an exact action. Component
source presentation chooses the first sorted source and makes ordering look
like semantic importance. Partial model coverage is truthful but visually
resembles a broken conceptual component.

The saved D208 Atlas Study response also proves a separate local rejection bug.
The provider returned five bounded directions with reading cardinalities
`[1,3,4,1,1]`, referenced all ten advertised reading targets and completed
normally. The resolver published only three because `naturalQuestion` requires
at least four whitespace-separated words. It rejected the otherwise valid
questions `Как обрабатываются ECC-ключи?` and `Как запускается прокси-сервис?`.
Word count is neither a language-neutral natural-question property nor an
authority boundary.

The input catalog remains narrow before the provider call: its ten exact
targets comprise the process entry, `service.Start`, and eight TLS/certificate
targets. Recovering the two locally rejected routes does not solve that breadth
problem. D209 must not disguise it as a UI problem or tune the product around
Casdoor.

## Outcome A: exact provider-free route recovery

The local question validator accepts a bounded non-empty question that ends in
`?`; it does not impose a token or word-count floor. Existing target-reference,
kind, identity, privacy, collision, cardinality, duplicate and runtime-order
checks remain fail-closed and item-local. Direction and reading order remain
the provider's accepted editorial order; same-file readings are not sorted or
deduplicated by path or line.

The exact saved D208 provider response is replayed through the new validator
without a provider call and must publish all five independently valid
directions. The local response-validator/result, accepted-artifact/cache and
report-projection identities advance wherever they can otherwise reinterpret
old accepted state. Earlier identities miss closed; there is no active artifact
migration or compatibility reader. The provider request bytes and prompt
version do not change.

## Outcome B: source-centric product projection

The product presentation becomes source-centric:

- every Study reading keeps its exact producer-owned target symbol in the
  report projection; the symbol is the primary source action and path:line is
  secondary metadata shown once;
- current reading-label enums are either rendered correctly or omitted where
  the ordinal already carries their only useful meaning;
- a Study detail consists of back navigation, the question and reason, one
  concise outcome, and the exact symbol-first reading list; duplicate headings,
  start buttons and prose about reading order leave the primary surface;
- route cards are articles: the title opens the route and each exact symbol
  opens its own source; the generic textual explore CTA is removed;
- generic Architecture CTAs on Study are removed. A target-specific related
  component or route action is shown only through exact typed IDs. Similar or
  next routes require a shared exact resolved target or component ID; text
  similarity is not an identity seam. Such actions are omitted when the join is
  absent or ambiguous and never claim ownership or equivalence;
- the separate "important code areas" shelf is removed when it would merely
  expose the actionable subset of a larger Shape. Exact related areas may be
  displayed as compact route context without inventing actions;
- Repository Brief and canonical local Atlas remain distinct data/authority
  objects but share one visual Overview: Brief first, compact local topology
  and package inventory beneath it. Routine internal status/count prose is not
  a product section; partial or unavailable states remain visible;
- the unclassified-by-model remainder remains truthful, separate from product
  components and source-accessible, but is rendered as one neutral compact
  coverage disclosure with symbol-first items; and
- component and Overview source actions never select the first sorted source as
  a representative. Exact typed parent ancestry distinguishes conceptual
  package participants from exact symbol sources. One exact source yields one
  source action; several exact sources yield plural actions; no ordering grants
  ownership, primacy or semantic importance; and
- every accepted conceptual Architecture component is navigable to the
  Architecture map even when it has no exact source action. An exact source is
  an independent secondary action, not a prerequisite for showing the
  component and not a substitute for conceptual membership.

No model output is parsed to recover symbols, source identity or semantic
importance. Symbols, paths, lines, targets and component intersections are
restored only from exact local typed data.

## Acceptance

- replaying the exact saved D208 Atlas Study response performs zero provider
  calls and publishes five directions with reading cardinalities
  `[1,3,4,1,1]` in their response order;
- concise Unicode questions such as `Как обрабатываются ECC-ключи?` and
  `Как запускается прокси-сервис?` pass without weakening the existing bounded
  text, terminal `?`, reference, privacy, identity, cardinality and
  runtime-order checks; invalid directions remain item-local;
- an earlier response-validator/result, accepted-cache/artifact or report
  projection identity is not silently read as the new contract;
- the four `certificate/dns.go` readings display four different exact function
  actions and show each path:line once;
- distinct same-file readings preserve accepted editorial order and are not
  collapsed merely because their paths match;
- no reading card falls back to a generic "study step" because of the current
  wire label enum;
- Study detail has no duplicate reading-path/outcome/inspection headings,
  duplicate start action or generic Architecture button;
- route title navigation and independent exact source actions both work with
  keyboard and pointer input in embedded and stripped static reports;
- repository Shape cannot render a singleton "important areas" shelf after
  silently filtering non-actionable siblings;
- Brief remains complete and appears once; Atlas remains canonical local data
  without a second competing hero;
- partial coverage remains visible, compact and source-accessible without
  appearing as a conceptual component;
- all accepted non-remainder conceptual components in the cached product
  fixture remain reachable from Overview or Architecture, including components
  whose only current members are package participants;
- exact symbol sources are derived through typed local ancestry; package
  participants are not presented as component ownership or as a semantic
  source start;
- multiple component sources remain plural; ordering never chooses a primary
  and no `resolved[0]`, sorted-first or equivalent presentation fallback is
  allowed;
- rendered-HTML journey tests cover the Russian product wording and clicks;
- the full Go test suite and `go vet ./...` remain green.

## Explicitly deferred

A diverse typed Study target shelf, deterministic breadth selection, typed
support span (for example process-entry path, repository-global static fact or
surface candidate), route-promise validation, provider request changes and a
new live acceptance run require a separate owner-approved decision. The next
decision must address breadth upstream; five recovered routes from the current
catalog remain startup/security/proxy-skewed.

Observed roles must be producer-owned and repository-shaped; they must not be a
fixed Casdoor/auth/TLS lane checklist. D209 does not expand SSA, infer
reachability from a static call target, add a minimum route count, pad targets,
sort accepted readings, use fuzzy text matching, choose a first source, grant
model ownership, restore raw Orientation, add a legacy reader or migrate active
artifacts.
