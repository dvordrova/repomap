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

SQL statement facts need statement structure. Each language hands one SQL
statement and one ordinary message that starts with an SQL verb to calls
outside the repository; only the statement becomes a `sql_query` fact:

| Language | Statement | Ordinary text |
| --- | --- | --- |
| Go | [handoff_flow.go](go/internal/storefixture/handoff_flow.go) `QueryRowContext` | [cmd/app/sql_literals.go](go/cmd/app/sql_literals.go) `fmt.Errorf("create %s dir: %w", ...)` |
| Python | [data_sources.py](python/src/fixture_app/data_sources.py) `connection.execute` | [sql_literals.py](python/src/fixture_app/sql_literals.py) `logging.error("create %s dir", path)` |
| TypeScript | [data-sources.ts](jsts/src/data-sources.ts) `connection.execute` | [sql-literals.ts](jsts/src/sql-literals.ts) `console.error("create %s dir", path)` |
| Clojure | [core.clj](clojure/src/example/core.clj) `(query! "SELECT ...")` | [core.clj](clojure/src/example/core.clj) `(format "create %s dir" dir)` |

A statement whose table the source fills in stays a statement with no listed
table. Go (`fmt.Sprintf("DROP TABLE IF EXISTS %s", table)`) and Clojure
(`(format "DROP TABLE IF EXISTS %s" table)`) hand it to a call outside the
repository, so it is also a `sql_query` fact. Python's `%` operator and a
TypeScript template literal are not call arguments, so their equivalents in
`sql_literals.py` and `sql-literals.ts` are partial source SQL only.

Calls chained or nested on one line keep their own positions. Each language
writes different calls with the same value, and the same call twice, on one
line; every call is its own registration fact at its own column and its own
boundary place, observed once by each of two targets sharing the file:

| Language | Source example | A call's position |
| --- | --- | --- |
| TypeScript | [platform.ts](jsts/src/platform.ts) `chainedPlatformCalls`, [server.ts](jsts/src/server.ts) `registerChainedOrderConsumers` | the called member's name |
| Python | [events.py](python/src/fixture_app/events.py) `subscribe_chained`, `chained_text_calls` | the attribute name |
| Go | [http_registrations.go](go/internal/storefixture/http_registrations.go) `registerStrippedFiles` | the opening parenthesis |
| Clojure | [core.clj](clojure/src/example/core.clj) `chained-paths`, `nested-paths` | the form's opening parenthesis |

Go's standard library has no fluent registration chain, so nesting stands in
for it. A Clojure Java instance chain (`(.. s (replace "/" "-"))`) carries no
call pattern and becomes no fact. Python knows no type for an untyped
parameter or for what `subscribe` returns, so those calls name no external
symbol; they are still separate facts.

Test code comes from runner facts, never from a file name alone:

| Language | Test code | Stays production or unclassified |
| --- | --- | --- |
| JS/TS | `node --test *.test.mjs` matches, the Playwright config, its `testDir`, reporter and the stub API only its `webServer` starts in [packages/canvas-ui](jsts/packages/canvas-ui); the Vitest config and its matches | the application server that `start` runs and `webServer` starts too, a draft test the script glob does not select, [src/excluded/retained.test.ts](jsts/src/excluded/retained.test.ts) |
| Python | pytest `python_files` matches and [tests/conftest.py](python/tests/conftest.py) | `tests/__init__.py` |
| Go | build-selected `_test.go` files | [internal/testhelper/helper.go](go/internal/testhelper/helper.go) |
| Clojure | namespaces that require `clojure.test` | none in the fixture |

Runner-configured test directories have no derived equivalent in Python
(`testpaths` often names the production package) or Clojure (no manifest is
read). The Python and Clojure contracts record both gaps.

A C command table, a callback stored under a branch and a call through a
function-pointer field have these equivalents:

| Language | Table of named handlers | Callback stored under a branch | Call through a stored function value |
| --- | --- | --- | --- |
| Go | [command_table.go](go/internal/storefixture/command_table.go) `commandTable`: one exact binding per row, with that row's `Name` and `Arity` | `eventLoop.register`, `RunChosenHandler`: the calls through the fields stay unresolved | `RunSingleHandler` is exact; a looked-up row (`DispatchCommand`) is unresolved, where C gives the table's handlers as alternatives |
| Python | missing | [stored_callbacks.py](python/src/fixture_app/stored_callbacks.py) `EventLoop.register`: the calls through the attributes stay unresolved | `run_single_handler` (a local name) is exact; through an attribute, missing |
| TypeScript | missing | [stored-callbacks.ts](jsts/src/stored-callbacks.ts) `EventLoop.register`: the calls through the properties stay unresolved | missing |
| Clojure | missing | missing | [core.clj](clojure/src/example/core.clj) `with-shadow` is unresolved |

Each handler keeps its exact callback at the call that registers it. The
language contracts record every missing equivalent, including two that give a
store under a branch a wrong answer: Go's interface-typed fields (false
alternatives) and Python's names reassigned under a branch (a false exact
call).

A C call written through a macro and a C function address cast to an integer
have no equivalent in Go, Python or TypeScript, which have neither macros nor
such casts. Clojure has macros: a call written in a macro's argument keeps its
own place and caller (`ensured-limit` in
[core.clj](clojure/src/example/core.clj)), while the use of the macro itself
leaves no call, which the Clojure contract records.
