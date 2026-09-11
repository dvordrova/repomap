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

## Imports and callable identity

The Python adapter owns package/module scope, import restoration, call and
registration facts, decorators, arguments, target seeds, external origins,
and complete observed/omitted coverage. Dynamic dispatch remains alternatives
or unresolved authority; shared stages never repair it by matching names.
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
Aliases assigned to a function or lambda remain alternatives; an inline lambda
retains exact authority, while unknown or overwritten aliases gain no callback.
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
as an alternative callback recipient. Reassigned, conflicting, conditional,
deleted, wildcard or cyclic export bindings remain unresolved; untyped factory
returns and unrelated same-named classes gain no receiver authority. No module
is imported or executed to discover exports. TypeScript uses the compiler's
existing barrel-export and declared-return resolution for the comparable case.

A source-ordered, directly annotated parameter may supply an existing locally resolved class method as an alternative native target, including an explicitly imported facade class. Annotations and literal values retain distinct provenance. Reassignment or conditional assignment clears that binding; unknown, union and unresolved quoted types remain unresolved. This adds no executed import, body analysis, framework inference or exact runtime dispatch.

## Framework-neutral registrations

The original AST call site, result identity, positional/keyword arguments and callback targets remain separate. `Thread(target=...)`, async-task and supported schedule registrations preserve their written activation evidence. A later `start`, `join` or liveness check on that same result does not invent a callback call. Lifespan setup and finite retry loops remain negative controls; final scheduled/continuous roles belong to [operation review](READING.md#operation-ownership).

## No execution for inference

No Python module, dynamic setup expression, factory or imported package is executed to infer target ownership or method authority. Unknown, overwritten, conflicting or conditional bindings remain unresolved. All extraction changes require the real cumulative Python fixture and applicable equivalents in the other languages; see [development](DEVELOPMENT.md).
