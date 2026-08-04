# Decision: saved source windows as semantic facts for chi dispatch

**Status**: Approved for implementation

**Approved by**: Repository owner in the current implementation session

## Product question

Can bounded source windows already saved by Targeted Research become exact,
locally verifiable Semantic Discovery facts and publish one durable Mechanism
for the question “How does Mux dispatch an HTTP request to an endpoint or
404/405?” without rerunning the repository pipeline?

## Approved implementation

1. Use the fixed saved run `20260715-193912-chi`. Project only code visible in
   its saved source windows into deterministic facts with opaque IDs, exact
   evidence, enclosing symbols, local source groups, narrow capabilities, and
   content hashes. Model interpretations and the manual chi audit are not
   facts.
2. Follow only the actionable `routeHTTP` and `FindRoute` frontier. One local
   syntax-only retrieval may read at most `mux.go` and `tree.go`, include only
   a few exact functions, use hard source budgets, and perform no transitive
   expansion, package loading, SSA, call graph, or runtime-surface discovery.
3. Create one owner-assigned `chi-request-dispatch` Mechanism candidate. Freeze
   its question, answer aspects, capability contract, aliases, facts, and
   explicit limitations before synthesis. Do not run opportunity discovery.
4. Allow at most one bounded Golden Mechanism synthesis call over the frozen
   candidate, deterministic facts, and validated leaf. Reuse the existing
   semantic support, capability, temporal, coverage, intent, fan-in,
   materialization, and Mechanism v1 validators without weakening them or
   adding response-specific rules.
5. On acceptance, save the existing canonical Mechanism v1 object and its
   per-object deterministic inputs. Replay must need neither a model nor any
   analyzer and must project through the existing Artifact, report, Search,
   map focus, and evidence paths.
6. Add a small report-local coverage summary for the fixed experiment:
   opportunities attempted, candidates investigated, canonical mechanisms
   published, and whether central routing is confirmed, partial, or unresolved.
   This is secondary and must degrade away with absent experiment artifacts.

## Fixed question and aspects

Question: `How does Mux dispatch an HTTP request to an endpoint or 404/405?`

Aspects: `request_entry`, `route_context_acquisition`,
`computed_handler_invocation`, `route_lookup`, `parameter_context_update`,
`endpoint_invocation`, `not_found_or_method_not_allowed`, and
`known_unknowns`.

## Focused checks

- saved source windows, not research prose, originate the projected facts;
- retrieved functions are limited to the named frontier and two files;
- every fact has stable identity, exact bounded evidence, local source group,
  enclosing symbol, narrow capability set, and content hash;
- the central dispatch mechanism has no unsupported ordering or invented
  dynamic relation, and unresolved wiring stays explicit;
- accepted replay works with the model disabled and no analyzer invocation;
- natural chi routing questions find the mechanism while exact source-path
  queries retain their existing priority;
- steps expose evidence in `mux.go`, `tree.go`, and `context.go` and retain
  non-empty existing-map focus;
- failure or absence of any new artifact leaves the previous report intact.

## Hard exclusions

- route registration, `InsertRoute`, `Route`, `Group`, `Mount`, nested-router
  lifecycle, full middleware composition, panic safety, radix-tree
  performance, DependencyUsage, trace compression, a new renderer, or a new
  orchestration pipeline;
- orientation, architecture synthesis, broad opportunity scan, global package
  loading, SSA, call graph, runtime-surface discovery, or full repository
  analysis;
- provider metrics UI. The known provider-request undercount remains a
  separate bug.

## Stop condition

Stop rather than publish if exact support requires an unapproved source
retrieval, a second model call, weakening a validator, asserting the dynamic
computed-handler wiring without evidence, or creating architecture components
or relations.
