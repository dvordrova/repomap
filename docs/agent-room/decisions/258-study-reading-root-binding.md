# 258 — Exact Study reading root binding

**Status:** APPROVED IMPLEMENTATION SLICE (owner-authorized, 2026-08-09)

**Preserves:** Decisions 243, 246, 256 and 257; one private
`DirectCallIndex`; final direct Study readings; refs-only mechanism requests;
the existing graph depth and cardinality bounds; and exact fail-closed local
validation.

## Product defect

A final Study reading retains a backend-owned `CanonicalSpanID`, exact
repository-relative source location and user-facing symbol. Focused readings
may point at an exact line inside a function body rather than at its declaration.
The user-facing Go spelling can consequently be `NewBot` or `(*Bot).Raw`, while
the direct-call substrate owns canonical identities such as
`gopkg.in/telebot.v3.NewBot` and
`gopkg.in/telebot.v3.(*Bot).Raw`.

The mechanism compiler previously asked `ResolveRoot` to match the user-facing
symbol against canonical producer identities. Exact Telebot readings therefore
closed as `no_exact_function` even though their source locations each belonged
to one exact indexed function. This is a loss of producer identity between two
local stages, not missing graph depth and not a reason to add suffix or fuzzy
symbol matching.

## Approved binding

Before compiling a Study card graph, the backend builds one bounded private
`StudyReadingRootBindings` value from the final Study artifact and the current
private `DirectCallIndex`:

1. Only final direct readings admitted by the existing eight-card/five-reading
   compiler bounds participate.
2. A reading is eligible only when it carries a non-empty backend-owned
   `CanonicalSpanID`, a repository-relative path and a positive focus line.
3. The binder first looks for exactly one indexed function whose declaration is
   at that exact path and line. When none exists, it looks for exactly one
   indexed function body containing that exact path and line.
4. Zero or multiple matches produce no binding. Slice order, symbol spelling,
   suffixes, package guesses, source prose and model prose never choose a node.
5. A resolved entry binds `CanonicalSpanID` to the exact DirectCall node ID.
   Entries are canonical-sorted by span identity.

The binding envelope carries its own version, Study repository revision,
DirectCallIndex SHA-256 and complete Go scenario. Compilation rejects drift in
any of those identities and rechecks that every supplied node is still the
unique exact locator match in that index.

`Compile` remains the ordinary convenience entrypoint and performs binding
before graph compilation. `BindStudyReadingRoots` and
`CompileWithStudyReadingRoots` are the explicit producer/root-wiring seam for
the AnalysisTarget integration. The original Study reading symbol and artifact
are never mutated. A reading with `CanonicalSpanID` never falls back to symbol
resolution when its exact locator is unresolved or ambiguous. Readings without
that producer binding retain the existing strict canonical/equivalent-symbol
lookup.

## Identity and boundaries

- `StudyReadingRootBindings`: v1, private and in-memory only;
- mechanism compilation/request/result, persisted investigation artifacts,
  prompt, report, manifest and UI identities remain unchanged because the
  existing compilation digest already binds the resolved root node, current
  index SHA/scenario and repository revision/freshness;
- no second package load or SSA build;
- no full index persistence;
- no symbol suffix, basename, receiver-name or fuzzy lookup;
- no depth, request, provider, report or presentation change;
- no `CURRENT.md` change.

## Acceptance

1. Telebot-shaped focused readings for `NewBot`, `(*Bot).Raw`,
   `(*Bot).Download` and `(*Bot).File` bind their interior focus lines to the
   exact canonical DirectCall nodes while preserving the Study artifact.
2. Wrong paths/lines and zero matches remain unbound.
3. Overlapping or nested function bodies remain ambiguous even when one short
   symbol appears attractive.
4. A misleading short symbol and another same-named declaration cannot affect
   an otherwise unique locator binding.
5. Study input permutation produces the same canonical root-binding entries.
6. Binding version, revision, index SHA, scenario, span and node tampering fail
   closed before graph compilation.

Approved by:
    Repository owner after the fresh Telebot report showed four exact Study
    readings becoming prepared-only with `no_exact_function`, 2026-08-09.
