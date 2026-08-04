# Decision: User Report v2.3 — relevant answer and code

**Status**: Approved for implementation

**Approved by**: Repository owner in the current implementation session

## Product outcome

A reader first receives the accepted whole-Mechanism answer, then walks short
action-oriented steps whose primary source card shows one dominant behavioral
fragment. Secondary references, ranked fallback code, and navigation remain
available without duplicating the same code or exposing provenance metadata.

## Approved implementation

1. Extend the existing User Report read model and renderer. Do not create a
   second frontend, change the architecture canvas, or change Search ranking.
2. Project the whole-Mechanism answer from the already accepted canonical
   `Artifact.Summary`. Never substitute the first supported statement. If the
   summary is absent, gap-only, or merely repeats the first step, omit the
   answer instead of inventing semantic prose.
3. Project short action-oriented step titles from accepted canonical step and
   statement text. Keep canonical titles, statement IDs, LogicalIDs, content,
   and semantic hashes unchanged.
4. Group saved supporting locations by path, enclosing symbol, and proximity.
   Pick one dominant executable cluster for the primary source excerpt, while
   retaining every original location under exact references. A directly useful
   second symbol may remain related source; distant support within the primary
   function does not need to be mixed into the compact excerpt.
5. Project at most two short source-shaped callouts from exact highlighted
   behavioral lines. Omit `What to notice` when no safe line-scoped callout is
   available; never repeat a whole accepted statement against several ranges.
6. Deduplicate primary, inline related, and whole-Mechanism source cards. Keep
   the whole-Mechanism file list collapsed behind one `All files` disclosure
   and omit the current step's already visible sources from that list.
7. Rank no-Mechanism saved source windows using deterministic presentation
   landmarks: CLI entrypoint, public API, quickstart/example, orientation start
   file, constructor/handler, representative test, then core code. Anchor each
   card on useful saved code and label the selection reason. If no strong
   primary exists, show several code-bearing starting points rather than a
   random large card.
8. Add an explicit `Open code path →` action to every Mechanism card. Remove the
   Overview action that silently chooses the first Mechanism.
9. Hide saved-snapshot metadata in default User View. It may remain in the
   source drawer and debug/provenance presentation.

## Truth boundary

- No model call, probe, analyzer, repository-wide analysis, fact, claim,
  validator, provider stage, Search ranking, or architecture graph change.
- No Mechanism v1 identity, LogicalID, canonical title, statement, summary,
  evidence binding, or semantic hash change.
- Dominant clusters, short titles, callouts, reason labels, and fallback ranking
  are presentation-only projections of already saved and validated inputs.
- Every omitted primary range remains available through the unchanged exact
  references and canonical artifact.

## Focused checks

- Caddy and chi answers equal their accepted canonical summaries and differ
  from the first step explanation.
- Caddy step 1 opens on the browse guard/call around lines 364–374; its function
  signature and index branch remain in exact references.
- chi step 2 keeps `Mux.routeHTTP` primary and shows `node.FindRoute` only once.
- Callouts are short, line-scoped, and absent when exact mapping is unsafe.
- No-Mechanism Caddy does not lead with `AdminPermissions`; ranked cards contain
  real saved code and reason labels.
- Existing routes, Back/Forward, Search, drawer, and mobile controls do not
  regress.
- Canonical Caddy and chi Mechanism bytes and content hashes remain unchanged.

## Relationship to prior decisions

This decision extends decisions 108 and 109. It does not delete, rewrite,
narrow, or invalidate them or any earlier approved decision. Their truth,
replay, source-authority, navigation, and safety boundaries remain in force.
