# Python native authority

Current implementation contract. Read only the sections relevant to the change.
[Constitution](../CONSTITUTION.md) takes precedence; [CURRENT](../agent-room/CURRENT.md)
records the current product decisions and acceptance status. Historical runs and
experiments are in the [non-normative archive](../archive/2026-09-10/README.md).

## Catalogue and shared parsing

The checked Python catalogue validates once when built or decoded and retains
exact native-target, selector and scoped-module lookups. Explicit Validate
still checks a supplied edited catalogue completely; lookups do not rescan it.
Selected targets with the same root/module inventory enter the existing
BuildMany parser core together. It parses each source AST once, then produces
each distinct package/alias view independently. Identical views share one
immutable common input and one stored facts payload. Each target retains its
original scope, launch seeds, complete dependencies and model analysis.
A bad launch projection refuses only that target while its neighbours keep
their already parsed inputs. A failed shared parser preparation retains the
existing exact-target fallback; cancellation stops dispatch immediately.

A module-level function, class or variable is public unless its name starts
with an underscore; a module that declares a literal `__all__` list or tuple
of strings exports exactly the names it lists, so an unlisted module-level
name is internal. Methods and attributes keep the underscore rule. The map of
parts shows the signatures of public names only
(`testdata/repositories/python/src/fixture_app/exports.py`). A class's methods
are always declared in its body: Python has no method outside its class for
the map of parts to move.

## Imports and callable identity

The Python adapter owns package/module scope, import restoration, call and
registration facts, decorators, arguments, target seeds, external origins,
and complete observed/omitted coverage. A name, import, alias, decorator, base
class, read or write that resolves to one known declaration is `exact`; several
known targets are `alternatives`, and a rebound or unknown name is `unresolved`.
Shared stages never repair resolution by matching names.
It maps an exact top-level import root in `sys.stdlib_module_names` to
`platform` and every other external root to `package`; an invalid or missing
authority kind fails the adapter boundary.

Repeated aliases in one import retain one witness for the same declaration
at the same source site; the observed count includes that witness once.
Distinct import statements and calls through each alias retain their own
locations. The 2026-09-09 ordinary Airflow check exposed a false omission:
`BaseFacet`, `BaseFacet as DatasetFacet`, and `BaseFacet as RunFacet` produced
one witness but an observed count of three, refusing the Python dependency
catalog and target. Counting only distinct witnesses fixes the producer while
keeping the complete-coverage check. Cumulative Python, Go, TypeScript and
JavaScript examples exercise native imports, distinct calls and complete
dependency coverage.

The Python adapter also keeps an existing callable candidate consistent
between an argument and the callback transfer that cites that exact argument.
Aliases assigned to a function or lambda and inline lambdas are exact, while
unknown or overwritten aliases, and aliases assigned under a branch, gain no
callback.
The Airflow Edge3, Azure and Vertica libraries exposed the earlier mismatch:
the argument named the assignment variable while the transfer named its callable.
Local native extraction does not establish ordinary full-repository acceptance. Cumulative Python,
Go, TypeScript and JavaScript examples retain their native authority rules.

An inline lambda in a store target's receiver or index (the pandas
`df.loc[reduce(lambda …), "exit"] = 1` idiom, also in annotated-assignment and
`for` targets) is declared and passed like any other; Freqtrade's example
strategy once failed its whole target on it. A lambda in a definition header
(a parameter or return annotation such as FastAPI's `Depends(lambda: …)`, a
type-parameter bound, or a lambda's default) belongs to the defining scope,
which passes it where a call receives it.

A call belongs to the scope in which it runs, in every language. A decorator's
arguments, a default and an annotation run once, where the function or class
is defined, so their calls, reads and lambdas belong to the defining scope:
the module for a class decorator or a top-level function's, the class for a
method's decorator and default. The `decorates` relation stays the decorated
declaration's. `LevelRoutes` in `models.py` checks it. TypeScript decorators
and Clojure metadata and attr-maps follow the same rule
([JSTS](JSTS.md#callable-jsx-and-declaration-headers),
[Clojure](CLOJURE.md#calls-that-run-when-a-namespace-loads)); Go and C write
nothing a definition runs.

Nested Python calls now use their complete native AST span in local relation
and argument-pattern identities. A chain such as `push().map(first).map(second)`
shares its starting position but retains two distinct calls, each with its own
arguments and callback candidates. Original source locations are unchanged;
the source-argument consistency check is unchanged. This fixes the three
Airflow task-sdk targets that previously failed with an argument-authority
mismatch. Current acceptance status belongs in [CURRENT](../agent-room/CURRENT.md#acceptance-and-open-work).
Cumulative Python, Go, TypeScript and JavaScript regressions preserve each
language's existing resolution strength and both callbacks. Go and JS/TS
already distinguished the calls and needed no production change.

The Python adapter retains each declared package's native directory in
ProgramIndex. A namespace package without `__init__.py` keeps no source
location; its exact directory supplies the existing workspace dependency row.
Relative named and wildcard imports into another explicitly named portion of
an advertised namespace retain their original external import boundary. A
nearer ordinary package/module does not authorize an unknown child. Importing
an unknown member directly from a known namespace keeps only that known
boundary; it does not invent a declaration or a callable.

## Typed parameters and explicit re-exports

Python HTTP facts follow observed single base-class chains to external methods,
stopping at local overrides and incomplete or multiple bases. TypeScript uses
the compiler-resolved original external class method. These facts do not add
native call edges; Go's promoted embedded methods use existing compiler evidence.
Direct written Python parameter types also retain possible receiver origins,
resolved in the defining scope. This preserves router mounts through typed
parameters and function-local imports. Untyped or reassigned parameters and
unrelated local classes do not acquire framework authority from a method name.
The Python adapter also follows unconditional explicit re-exports through
indexed package/module imports, retaining each written import boundary. A
factory's declared return type may then supply the original instance method
as the callback recipient. Reassigned, conflicting, conditional,
deleted, wildcard or cyclic export bindings remain unresolved; untyped factory
returns and unrelated same-named classes gain no receiver authority. No module
is imported or executed to discover exports. TypeScript uses the compiler's
existing barrel-export and declared-return resolution for the comparable case.

A module-level star import (`from m import *`) may bind any name where it
runs, under a branch too, and its names are not followed. The adapter records
where each star import and each export binding of a module is written, as
statement positions (line, then column). A member of a module with star
imports (`pkg.name`, `from pkg import name`, `import pkg as alias` then
`alias.name`) resolves as it would without the stars only when the module
writes it once, unconditionally, in a statement that starts after its last
star import: a `def`, a `class`, an assignment or an explicit import there is
the module's own. A name only a star could bind, and a name written before a
later star (which may rebind it), stay unresolved, as does a child module
of such a package that the package does not bind itself. pykrx's
`krx/__init__.py` star-imports four subpackages and then defines
`datetime2string` and `get_nearest_business_day_in_a_week`: their 77 calls
through `krx` are exact, while the 75 calls to names only its stars bind stay
unresolved. The cumulative fixture's `import_facades/star_facade` and
`star_consumer.py` pin both sides, beside `star.py`'s `StarOnly`, which stays
unresolved. The native equivalents resolve the same shape already: TypeScript
`export *` plus the module's own export, through `import * as`
(`src/facade-exports/star-*.ts`), and Clojure `:refer :all` plus the
namespace's own `defn`, through an alias (`example.facade`), each with its
expectation; a C header that includes another and declares its own function
is the fixture's `kvd.h` and `kvAssertFail`, called exactly from `kvd.c`
(C contract tests). Go has no wildcard re-export, so a package member is
always a declaration of that package: not applicable.

A source-ordered, directly annotated parameter may supply an existing locally resolved class method as the native target, including an explicitly imported facade class. Annotations and literal values retain distinct provenance. Reassignment or conditional assignment clears that binding; unknown, union and unresolved quoted types remain unresolved. This adds no executed import, body analysis, framework inference or exact runtime dispatch.

Synchronous iteration over a directly annotated homogeneous collection retains
its locally resolved element class as a possible receiver inside the loop body.
This applies to parameters, local annotations and declared receiver fields,
including unshadowed `sorted(collection)` with its ordinary key/reverse options.
Known built-in and typing/collections container annotations supply this evidence;
an arbitrary wrapper, shadowed `sorted`, heterogeneous tuple, union, unknown
collection or replaced binding does not. A loop may run zero times, so its
receiver origin does not escape into the else clause or subsequent statements.
An asynchronous loop does not borrow a synchronous container annotation.
Method calls and direct field writes keep their original source locations and
possible status; this is not runtime dispatch or proof that an input executes
the write.

The cumulative iteration fixture checks these positive cases and negative
controls. Real Go range, TypeScript for-of and JavaScript JSDoc-array examples
retain their existing compiler-resolved method identities, with untyped JS/TS
controls remaining unresolved. Clojure has no corresponding generic receiver
type evidence in its current adapter; Java instance dispatch remains unresolved.

## Variable reads and attribute writes

Underscore is an ordinary Python parameter/local name. Its declaration,
annotation and initializer retain the same source identity as other names;
typed `_` parameters and annotated call results must not abort extraction.
Unannotated receivers remain unresolved. The cumulative JS/TS examples retain
their native underscore parameter authority, and Clojure retains an unresolved
local underscore callback. Go's blank identifier is intentionally different:
the cumulative range example never creates a named `_` variable.

Native variable reads retain the original declared slot and each source site,
including imported aliases, module-qualified values, receiver fields and reads
of receivers/indices on assignment targets. A read names its declaration, not a
runtime value. A callable reading its own parameter or local is not a relation;
that value's origin stays on the patterns that use it. Lexical parameters/locals, comprehensions, nonlocal/global
declarations and class-body versus method scope cannot borrow a same-named
outer value. Unbound with/except/match targets stay unresolved. Replaced or
untyped receivers do not acquire field authority. The cumulative examples test
these controls and preserve read locations through GroupsIndex.

The places graph keeps these reads, with every exact decoration, as the
declaration's `uses` (READING): `TestCumulativePythonMapOfParts` checks that
`read_level_data` uses `levels.py`'s `READ_VALUES` and `READ_LIMIT`, and that
`traced_level` uses `traced`, a decorator written as a bare name, which
leaves no pattern and so no lifted call. The role split's helper question
(READING) shows those constants with the functions that read them, and
counts every exact decoration as a use. In the split check, `exports.py`'s
`format_score`, which `__all__` leaves out and only `render_level` calls, is
a helper, so the file keeps one declaration that is none and stays whole.

Clojure already emits comparable native var reads; its cumulative example now
checks an imported var and a shadowing local. JS/TS emits its compiler-bound
declared value references (JSTS), and C a function's reads of file-scope
variables and tables, one per site (C). Go does not currently emit general
variable reads (GO).

Source-ordered receiver origins may bind a direct attribute write to an existing
native class field. Method receivers, their local aliases, directly annotated
parameters and local constructor results retain the field's exact lexical owner.
Assignments, augmented assignments, annotated writes and deletes keep every
source site. Nested classes keep their own receiver identity; a captured outer
receiver keeps its original owner. Rebinding, an untyped receiver, a static
parameter merely named `self`, nested receiver expressions and dynamic `setattr`
remain unresolved. A resolved field write is exact. No class is executed to infer the result.

The cumulative MutableCounter fixture checks each write and its GroupsIndex
source projection, with read-only and replaced-receiver controls. The current Go,
JS/TS and Clojure adapters do not emit comparable target-bound field-write
relations; their mutation-tracing equivalent remains unavailable rather than
being inferred from call or field-initializer evidence.

## Handler tables and stored callbacks

These are the Python equivalents of the C adapter's command table, its
callbacks stored under a branch and its calls through function-pointer fields.
The cumulative `src/fixture_app/stored_callbacks.py` checks what the adapter
supports:

- A handler stored into one of two attributes under a branch keeps each store
  as an exact write of its attribute at its own line. The calls through
  `self.on_read` and `self.on_write` stay unresolved and gain no handler. Each
  handler keeps its exact callback at its `register` call.
- A local name bound once to a function (`handler = accept_client`) makes
  `handler()` an exact call of that function.
- A name reassigned under a branch leaves the call through it unresolved,
  never its last assignment and never alternatives, as the C adapter does.
  After `handler = flush_replies` and `if readable: handler = accept_client`
  (`run_chosen_handler`), `handler()` names each function stored in the name
  as a `function_value_store` witness at the stored value:
  `flush_replies stored in handler` and
  `accept_client stored in handler under a condition`, each naming its
  function by identity too, so the map draws the call's possible arrows. In `models.py`,
  `register_callback_aliases` passes its parameter `handler` after
  `if replace_handler: handler = handle_delivery`; the argument keeps the
  variable and gains no callback of `handle_delivery`.

A branch is the body of an `if`, a loop, a `try` (not its `finally`), a `with`
or a `match` case, an arm of a conditional expression, an operand of a boolean
operator after the first, or a comprehension, in the name's own scope. The
condition, the subject and the first operand always run, as the C adapter
walks an if's condition and the left of `&&`: after
`if (handler := accept_client) and ready:`, `handler()` stays exact. A function
declared under a branch keeps the exact calls of its own body, and an enclosed
function reading the name sees the same unresolved value. A `def`, `class` or
`import` of the name in that scope is one more witness, and once the name is
also assigned there, one under a branch makes it conditional too
(`handler = flush_replies`, `if readable: def handler(): ...`). A call of an
attribute of such a name is unresolved and names each module or class stored
in it: after `codec = json` and `if flag: codec = pickle`, `codec.dumps()`
names `json stored in codec` and `pickle stored in codec under a condition`,
never `pickle.dumps`.

Missing equivalents, recorded rather than fabricated:

- The fixture has no input of its own, so GroupsIndex's reach is checked by
  probing declarations as inputs (`flowtest.Probe`: `read_level_data` reads
  `READ_VALUES` and `READ_LIMIT`, `traced_level` does not reach `traced`); a
  call through an attribute is never resolved as alternatives (below), so
  there is no dispatch site.
- No Python function is proven `unreachable` (the C adapter's per-program
  fact, PROGRAM_INDEX): `getattr`, `importlib`, entry points, decorators that
  register, special methods the interpreter calls and `eval` reach functions
  no call names. A boundary in a module several targets import stays with
  every target, and a part leaves no target's map as code that target never
  runs (READING). Two targets importing one module therefore list nothing
  either never runs, and no declaration is shown "run by" the other
  (REPORT).
- A dict or list of handlers (`{"get": get_command}`,
  `[("del", del_command, 2)]`), at module level or in a function, keeps no
  binding, key or other relation to its handlers. A call through a looked-up
  entry (`COMMANDS[name](args)`) is unresolved.
- With no table-row registration, the C rule that a row storing two
  callables is one input has nothing to apply to.
- A call through an attribute is never resolved from its stores, even from a
  single store in `__init__`. The C adapter makes one store exact and several
  stores alternatives. The adapter sets no dispatch word, so no call says that
  it runs a function value.
- Assignments without a branch still resolve to the last one in the scope,
  even at a call written before it: after `handler = accept_client`,
  `handler()`, `handler = flush_replies`, the call is an exact call of
  `flush_replies`. The C adapter gives unconditional stores alternatives.
- A function, class or import bound under a branch with no assignment of that
  name (`try: from fast import loads`, `except ImportError: def loads(...)`)
  still resolves to its last binding.

## Generic declarations

Class and function signatures keep PEP 695 type parameters as written after
the name: `class Crate[T]`, `class Keyed[K: str, V: (int, str)](Box[V])`,
`first[T](items: list[T]) -> T`. A `Generic[T]` base already stays in the
class header (`class Box(Generic[T])`). The cumulative Python repository's
`src/fixture_app/generic_types.py` asserts all four, so that fixture needs
Python 3.12 or later; the parser itself still reads older interpreters' trees,
which have no type-parameter field. A PEP 695 `type Pair[T] = ...` statement is
not indexed as a declaration yet; that gap remains open. See
[Go](GO.md#owned-declarations) for the equivalents.

Functions and classes carry `code_lines`: the lines of `lineno..end_lineno`
holding a `tokenize` token that is not a comment, outside every docstring
statement's span (decorator lines are outside the range). A module counts its
whole file; a module or class variable counts its assignment statement. The
same file's `pick` has two `@overload` stubs and an implementation, three
declarations of one name the map of parts reads as one unit; the
implementation's docstring, comment and blank line leave it 3 code lines.

## Framework-neutral registrations

The original AST call site, result identity, positional/keyword arguments and callback targets remain separate. `Thread(target=...)`, async-task and supported schedule registrations preserve their written activation evidence. A later `start`, `join` or liveness check on that same result does not invent a callback call. Lifespan setup and finite retry loops remain negative controls; final scheduled/continuous roles belong to [operation review](READING.md#operation-ownership).

## Inputs a call's words declare, and what Python does not have yet

A call of an outside symbol given words asks that symbol what they become
(READING, the `atlas_api` table's third set), with `result_receives`, the
calls made on what the call returns. The fixture's
`src/fixture_app/tool_cli.py` asks `argparse.ArgumentParser`
(`add_argument ×1`, `add_subparsers ×1`), `add_argument`,
`add_subparsers` (`add_parser ×1`) and `add_parser`; `set_defaults(func=…)`
hands a callable and is asked what it becomes. A subcommand named by one
call and handled through another is two questions and, when both are
accepted, two inputs; joining them is later work. Not recorded yet:

- `sys.argv` carries no argument vector origin, and a comparison of
  `sys.argv` or of a parsed argument (`args.cmd == "init"`) is no fact;
- list and dict tables of names;
- dict registries (`handlers[name] = fn`);
- a callable the repository's own function keeps (S1) is not enabled.

## Programs a call starts

`revision` in `tool_cli.py` runs `subprocess.run(["git", "rev-parse",
"HEAD"], check=True, capture_output=True)`: asked `talks` with the call as
written, the reading's `runs_program` (READING). Not recorded yet: the
strings inside a list or tuple literal are no call words (a call's words
are its literal arguments), so this call gives the program question no
word, is not asked, and its program stays not established. `os.system`
and `subprocess.Popen` given a string are asked like any word-given call.
Missing equivalent (2026-09-28): a name assigned on both branches of an
if/else (`proc = subprocess.Popen(…)` in each) is a reassigned binding,
so its read is `unknown` with the name as text, not the `alternatives` of
both `call_result`s that Go records; `proc.communicate()` on it stays its
own launch boundary instead of folding into both launches (READING).

## Test sources

A resolved pytest table in `pyproject.toml` (`[tool.pytest]` or
`[tool.pytest.ini_options]`) owns the Python files below it. Its
`python_files` patterns select test modules. Without that setting, pytest's
default patterns apply only when pytest is a declared dependency. Every
`conftest.py` under a resolved table is test code too: it is pytest's own
plugin file, the equivalent of a JS runner's config. The nearest table owns a
file, and an unresolved one classifies nothing. The cumulative fixture's
`tests/conftest.py` is test code; `tests/__init__.py` stays unclassified.

Equivalents that are not derived:

- `testpaths` is a collection root, not a test-only directory. Large
  projects point it at their production package (pandas sets
  `testpaths = "pandas"`), so a file under it is not test code by that
  fact. Only the `python_files` matches and `conftest.py` below it are.
- `unittest` discovery has no manifest declaration. Its `discover -s/-p`
  arguments live in Makefiles, tox or CI, which the adapter does not read.
- `pytest.ini`, `tox.ini` and `setup.cfg` pytest sections are not read.

## No execution for inference

No Python module, dynamic setup expression, factory or imported package is executed to infer target ownership or method authority. Unknown, overwritten, conflicting or conditional bindings remain unresolved. All extraction changes require the real cumulative Python fixture and applicable equivalents in the other languages; see [development](DEVELOPMENT.md).
