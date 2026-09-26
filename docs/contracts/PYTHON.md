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
unknown or overwritten aliases gain no callback. An inline lambda in a store
target's receiver or index (the pandas `df.loc[reduce(lambda …), "exit"] = 1`
idiom, also in annotated-assignment and `for` targets) is declared and passed
like any other; Freqtrade's example strategy once failed its whole target on it.
The Airflow Edge3, Azure and Vertica libraries exposed the earlier mismatch:
the argument named the assignment variable while the transfer named its callable.
Local native extraction does not establish ordinary full-repository acceptance. Cumulative Python,
Go, TypeScript and JavaScript examples retain their native authority rules.

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

Clojure already emits comparable native var reads; its cumulative example now
checks an imported var and a shadowing local. Go and JS/TS do not currently emit
general variable reads. JS/TS retains its narrower compiler contract/type-use
relations; these are not evidence of arbitrary runtime value use.

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

## Framework-neutral registrations

The original AST call site, result identity, positional/keyword arguments and callback targets remain separate. `Thread(target=...)`, async-task and supported schedule registrations preserve their written activation evidence. A later `start`, `join` or liveness check on that same result does not invent a callback call. Lifespan setup and finite retry loops remain negative controls; final scheduled/continuous roles belong to [operation review](READING.md#operation-ownership).

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
