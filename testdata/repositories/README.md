# Cumulative language repositories

This directory contains exactly one small real repository for each language
covered by repository-discovery and ProgramIndex regression tests. A future
repository-dependent regression extends the existing repository for that
language only after owner approval; it does not create one repository per bug.
Every tracked file has an exact inventory entry under `testdata/contracts`.

Every language-cube behavior change must add or extend a source example here
and its executable expected result. Exercise the real extractor and adapter
through the boundary whose behavior changed; a nearby test that does not
exercise the change is insufficient. Reuse an existing example when it covers
that exact behavior. Keep contrasting cases for ownership and resolution so
an invented owner, call or relationship fails the check.

A case discovered in one language triggers the same check for its equivalents
in every other supported language. Add or extend comparable examples and
expectations immediately; no separate request is needed. Respect native
semantics instead of copying syntax mechanically. When an adapter already
handles the case, add the regression example without changing that adapter.
If a language has no equivalent, record that explicitly.

The fixtures deliberately contain no nested `.git` directory, generated
binary, product command, helper script, network requirement, or third-party
runtime dependency. Test harnesses may copy a fixture to a temporary directory
and initialize source-control metadata there when tracked-file behavior is
part of the contract.

These fixtures cover deterministic indexing and preparation of the original
declaration evidence for questions. Model-backed behavior such
as batching, closed-ref normalization, cache validation, and grouping belongs
in smaller cube or executor tests unless a separately approved regression
really depends on repository shape. If a future repository test must cross a
provider boundary, it uses an exact request-bound, fail-closed local preset
with no network access.

These repositories are regression evidence, not product acceptance. Ordinary
online runs against real repositories remain the acceptance path.

Current language repositories:

- `go/` proves the ordinary Go orientation handoff through an imported local
  package. Its unused private receiver method remains a ProgramIndex object but
  never gains a DirectCall node that was not observed.
- `python/` proves the complete PEP 621 `src`-layout script chain from target
  discovery through a validated, round-tripped ProgramIndex with one exact
  `main` script seed.
- `jsts/` proves the selected-package TypeScript compiler path for local and
  JavaScript default-library invocations. DOM canvas calls, `Math`, `console`,
  `Date`, `Promise`, and `Image` retain exact platform authority; a class
  construction retains its exact local constructor, while a repository-local
  value merely typed as a platform constructor remains an unresolved frontier.

Comparable response-field examples:

| Language | Source example | Declaration |
| --- | --- | --- |
| TypeScript | [src/type-members.ts](jsts/src/type-members.ts) | `IGetLevelsResponse` owns `count: number`. |
| Python | [src/fixture_app/models.py](python/src/fixture_app/models.py) | `GetLevelsInfoResponse` owns `count: int`. |
| Go | [internal/storefixture/level_responses.go](go/internal/storefixture/level_responses.go) | `GetLevelsInfoResponse` owns `Count int` with the JSON name `count`. |

The checks follow these declarations through the real language adapter,
ProgramIndex, atlas type members and question evidence. They preserve each
field's signature and exact source location under its own type; a same-named
field in another type cannot replace it. These checks make no model request
and do not assert that a future answer will select or correctly explain the
field.

Go interface methods are covered in
[internal/storefixture/fixtures.go](go/internal/storefixture/fixtures.go#L136):
`TicketContract[T]` declares `Cancel(id T) error` and `Status(id T) string`.
The cumulative Go check retains their original owner and signatures, rejects
new method ownership on `EmbeddedTicket`/`TicketAlias`, and verifies that a
declaration without a body acquires no execution node or runtime relations.
