# Go native authority

Current implementation contract. Read only the sections relevant to the change.
[Constitution](../CONSTITUTION.md) takes precedence; [CURRENT](../agent-room/CURRENT.md)
records the current product decisions and acceptance status. Historical runs and
experiments are in the [non-normative archive](../archive/2026-09-10/README.md).

## Build-selected scope

- The Go fact and target inventory excludes non-`DepOnly` `go list` root rows
  that have no build-selected `GoFiles` or `CgoFiles`. In particular, a
  directory containing only external `*_test.go` files is not an ordinary
  package when the product loads with `Tests=false`; its raw row may inform
  dependency metadata but must not enter package counts, target identity, or
  the typed ProgramIndex scope. A source-bearing package that fails type
  checking is not filtered and still fails its owning target closed.

The Go fact inventory also retains the complete build-selected package-origin
universe from every `go list -deps` row, including `DepOnly` rows. The Go
tool's exact `Standard` bit maps standard packages to `platform`; every other
known external package maps to `package`, and generated cgo `C` authority maps
to `platform`. An external target absent from that universe fails the adapter
closed rather than being guessed from its import path.

## Traversal and explicit narrowing

- The ordinary Go direct-call traversal is complete for the selected target:
  `--depth 0` and `--edges-limit 0` are the defaults and mean retain every
  exact call and edge across loaded repository declarations, including functions
  outside the launch tree. Positive values are explicit user-requested
  narrowing controls. Dynamic and unresolved call frontiers remain represented separately. Repository scale is neither a warning nor an implicit narrowing option.

## Dynamic receivers and callable transfers

The normal Go call-index pass now visits every loaded repository function,
including callbacks outside the launch call tree, and canonicalizes generic
origins. Anonymous functions retain their compiler signatures through the
Go adapter as well. Explicit depth/edge narrowing remains
explicit. Declaration candidates
include anonymous functions passed as callbacks; incidental closures are not
automatically added. Neutral callable bindings retain the source and destination
names, field/argument detail, invocation, resolution and source location. Calls
to external code retain qualified names, so `context.WithTimeout` does not lose
its identity before interpretation. The dynamic-handoff index also retains source
assignments to interface fields, keyed by the compiler's field declaration.
Local factory return values can resolve a stored implementation. The stores of
a field seen in the analyzed program are its values: one resolved value is an
exact call, several are alternatives, and only a store whose value cannot be
followed leaves an unknown. An implementation outside the repository, such as
`*sql.DB` stored in a sqlc `DBTX` field, is not an unknown: the call becomes an
`invokes_external` fact of that type's method (dispatch `interface`) with the
call site's arguments, and no unresolved `calls` relation is projected beside it.
Fields with the same name on unrelated types do not share candidates. Both the
call site and the constructor assignment survive projection; an assignment is
support for the call, not a second call at the constructor line. This recovers
`quotaKVServer.Put -> kvServer.Put -> EtcdServer.Put` in etcd without any
framework-specific rule.

A store under a branch leaves its field open, as in the C adapter. A branch is
an if, a case of a switch or type switch, or a select clause of the storing
function; a loop body, a store after an early return and the body of a function
literal are not. Every call through an open field is unresolved. Each
repository implementation its stores put there becomes an
`interface_field_assignment` witness of that call at its store (`X stored in
T.field under a condition`, without the last three words for a store no branch
decides), never a target; an implementation outside the repository gives no
`invokes_external` fact there. A store of a parameter joins every caller's
argument whichever field the branch chose, so without this rule a
`register(readable, h)` that stores `h` into `read` or `write` gave each field
both handlers as alternatives.

Dynamic value traversal reuses immutable summaries within one root and exact
interface method. The function key also retains `throughFlow`; factory result
indices remain attached to their own SSA values. Only subtrees that completed
without an active-path cycle enter this local memo. Cyclic results propagate
their dependency on the current path and continue to use the original traversal.
Merging retains child-order evidence, every exact assignment location and the
number of unresolved paths per incoming edge. An integer representation overflow
is a terminal extraction error, including when the same callable resolver feeds
external-call argument facts. This adds no persistent cache, interface-method
cap or inferred candidate, and does not promise linear traversal of cyclic graphs.

Interface-valued arguments now retain the concrete methods of the declared
interface when their implementation is resolved from the actual value, local
factory return, or observed alternatives. The existing transfer slot records
the declared interface and method. An unrelated compatible type cannot supply
an implementation; extra concrete methods outside the interface are excluded.
These are object-registration observations, not callback executions: they are
`binds_implementation` relations, while a callable value passed as an argument
remains `passes_callback`. On etcd,
quotaKVServer.Put retains the RegisterKVServer argument at grpc.go:80 and the
separate local KvServerToKvClient adapter binding at v3client.go:33.

An interface value stored through a repository constructor parameter retains
the concrete implementations supplied by every actual static repository call
to that constructor. A non-call use, missing argument or caller outside the
selected repository scope keeps the frontier unresolved. The declared method
still limits eligible observed implementations; type compatibility alone never
creates a dispatch edge. The cumulative Go fixture requires the
constructor-injected facade to retain `storedEngine.Put` as an observed target.

## Interface implementation matching

The exact Go analysis input owns `MatchInterfaceImplementations`. When enabled,
the core-object pass uses the already loaded `go/types` universe to enumerate
every selected-repository named non-interface type whose value or pointer method
set satisfies a selected-repository interface. An inverted method-name index
narrows candidates before `types.Implements` performs the authoritative check;
this adds no source read, package load, SSA build or call-graph traversal. The
ordinary Go adapter enables the option for every selected target, including
module libraries; direct cube callers may enable or disable it explicitly.

Each accepted type pair projects one exact `implements` relation from concrete
type to interface and each directly owned matching method projects another
exact `implements` relation from concrete method to interface method. Value and
pointer method-set evidence remains explicit. These compatibility facts do not
claim construction, assignment or runtime dispatch and therefore never replace
the separately observed binding and call relations. The cumulative fixture
requires `compatibleOnlyEngine` and its `Put` method to match `FieldStore`
despite never being assigned to that interface.

## Receiver fields and bindings

Callable bindings now retain literal assignments to other fields of the same
SSA receiver in that function. Referrer identity keeps two command/worker
objects of the same type separate; conditional or later stores remain separate
anchored observations, not final runtime values. Named and anonymous callbacks
share this mechanism. No field names or framework types drive extraction.
ProgramIndex carries these as `callable_receiver_field` witnesses, and the
atlas attaches them as binding evidence rather than additional registrations.
A callable bound into a field of a value whose type another package declares
(`&cobra.Command{Use: "serve", RunE: run}`) is also projected as the
construction of that value: an `invokes_external` relation to the type with
the `construct` invocation, whose one call pattern carries the string literals
stored beside the callable as keyword arguments and the bound field as the
keyword argument the `passes_callback` crosses by. Go constructs where other
languages call a constructor, and the facts stage reads both as one
registration shape. A value of a repository type is no such construction.
The own callback sees its object's fields; a neighbouring caller's registration
retains just the binding shape and source, so a shared error helper does not
inherit every command's help text. Canonical sealing, independent copies,
source anchors and a two-object fixture verify the underlying facts.
The atlas now canonically orders and exactly deduplicates each binding's
source-evidence set before deriving its identity key. The same Freqtrade Thread
registration occurred four times solely because target views ordered its
start/join/is_alive observations differently. Canonicalization produces one
binding while retaining every source anchor and the literal thread name.
Argument order, different field values, callback targets and source sites are
unchanged. A real cumulative Python extraction across four target views and
generic contrasting cases cover this correction. It changes affected graph
and exact-request hashes, independently of the byte-preserving loading change.

## Handler tables and stored callbacks

These are the Go equivalents of the C adapter's command table, its callbacks
stored under a branch and its calls through function-pointer fields. The
cumulative fixture's `internal/storefixture/command_table.go` checks them:

- A table of named handlers built inside a function
  (`[]commandRow{{Name: "get", Arity: 2, Run: getCommand}, ...}`) gives each
  row its own exact `passes_callback` binding. Its `go_ssa_dynamic_handoff`
  witness makes the binding row, and its `callable_receiver_field` witnesses
  carry that row's literals and no other row's (`Name = "get"`, `Arity = 2`).
  A value of a repository type is no construction, so a row stays a binding.
- A call through a function-typed field is exact when the value it reads was
  allocated in that function (or returned by a repository factory) with one
  store to the field, in the allocating block, before the call
  (`RunSingleHandler`). A field filled through a parameter under a branch
  (`eventLoop.fire`) and a store under a branch (`RunChosenHandler`) leave the
  call unresolved, as the C adapter does. The Go call lists no candidates,
  where the C adapter names each stored function as a witness. Each handler
  keeps its exact callback at its `register` call.
- The same loop with interface-typed fields (`readyLoop`): `register` stores
  its handler into `read` or `write` under a branch, and `RunChosenReady`
  stores one into `read` under a branch. Every call through either field is
  unresolved, with the C adapter's witnesses: each handler a store put into
  that field, at the store (`(acceptReady).Handle stored in readyLoop.read
  under a condition`), never the handler registered for the other field. Each
  handler keeps its exact `binds_implementation` at its `register` call.

Missing equivalents, recorded rather than fabricated:

- A call through the field of a row found by a lookup (`DispatchCommand`)
  stays unresolved: a function-typed field is followed only on one allocated
  value. The C adapter gives such a call every function stored into that field
  as its alternatives, so it reaches each handler of the table.
- A package-level table (`var commands = []commandRow{...}`) is filled by the
  synthetic package initializer, which the binding capture skips. A map of
  handlers (`map[string]func(){"ping": ping}`) is filled by map updates, not
  field stores. Neither keeps a binding or any other relation to its handlers,
  so neither gives the registration the C adapter makes of a table row that
  names its function by a string literal (the owner's decision of
  2026-09-26).
- An open interface field is decided at its stores, not at the calls that
  reach them. A helper that stores its parameter unconditionally
  (`func (l *L) setRead(h H) { l.read = h }`), called under a branch with a
  parameter its caller was given (`if readable { l.setRead(h) } else {
  l.setWrite(h) }`), still joins every argument of that caller's callers: the
  call through `l.read` keeps both handlers as false alternatives. The C
  adapter leaves a parameter passed on unresolved; Go follows it through its
  callers, as it does for the values a constructor chain hands to a field.
  A method of an interface declared outside the repository keeps only its
  `invokes_external` fact at a call through an open field, without the
  witnesses.

## Owned declarations

Direct TypeScript interface property declarations retain their written type,
optional/readonly modifiers, exact source location and native owner. Go core
objects retain short type signatures and explicitly declared struct fields, including embedded
field declarations, under their native type. A struct tag is not signature text: each named format
becomes an object alias (`json:"count_label,omitempty"` is alias `json`/`count_label`; options and `-`
are not names). Both project into the existing
type-owned variable objects used by Python class fields. The same atlas members
and question evidence carry them onward; no field creates a runtime call or an
inherited declaration at a new owner. Comparable count-field examples live in the cumulative TypeScript, Python and Go testdata repositories.

A generic type's short signature keeps its whole type-parameter list, whose
constraints may hold spaces and brackets: `[T any] struct`, `[K comparable, V
map[string]int] struct`, `[T interface{~int | ~string}] interface`, and
`[T storefixture.Labeled[int]] []T` with the constraint's package path
shortened. Its fields and struct tags never enter that text; before this, the
form began at the first space, inside the parameters, and leaked every field
and tag. The header is written from go/types, not parsed back out of a
printed type: the type parameters with their constraints (consecutive ones
with one constraint share it, `[Item, Value any]`), then `struct`,
`interface`, the defined type, or `= T` for an alias, with packages named as
the code names them. A generic function's signature keeps its parameters
(`func[T any](items []T) T`). The cumulative Go repository's
`internal/storefixture/generic_types.go` asserts both. Equivalents: TypeScript
class headers and function signatures keep their parameters; a generic type
alias is written from its source parameters (`Keyed<K extends string, V =
number>`), because the compiler's rendering (`Keyed<K, V>`) drops constraints
and defaults. Regression examples are in `src/type-members.ts`. Python writes
PEP 695 parameters since the same change
([Python](PYTHON.md#generic-declarations)); Clojure declares no type parameters
and has no equivalent.

A Go method may be declared in another file than its type. Its native owner
is the type, so on the map of parts it goes with its type's file part, and
a file holding only such methods has no unit of its own
(`internal/localstore/ledger_append.go` in the cumulative Go fixture). A
closure (`f$1`) has no native parent in the index (its container is the
package); the map of parts finds it as a lexical child by source range.

## Source-aware diagnostics

The etcd report exposed a shared-root ownership defect: the first target at a
root took every file from its library/executable sibling. Places now retains
all indexed owners tied at the deepest root; a nested target still owns its
own subtree. Input order does not decide ownership. Configuration reads no
longer put an entire directory in the outbound integration lane, and entry
seeds are checked against the current target's file membership.

Go TODO extraction scans comment tokens, preserving physical source lines;
`context.TODO()` and string literals are not comment markers. Other languages
still use their existing line matching. Every readable text file is scanned;
the former whole-file 1 MiB cutoff silently lost all markers, even at the start
of an otherwise ordinary source file. Cumulative Go, Python and TypeScript
examples now retain source-distinct markers on both sides of that former size
boundary, including CRLF and Go physical-line anchors. Dependency facts retain the complete
imported package path instead of only the short package name.
Claims likewise read each eligible README/source file completely through the
existing corpus reader; a whole file larger than 1 MiB no longer silently
loses all its quotes. The existing quote selection, UTF-8 policy and physical
anchors are unchanged. Cumulative README/Go/Python/TypeScript regressions keep
their complete original claim sets, dates, ownership and seals after padding
moves the same text past that former byte boundary.

Listen address facts parse bracketed IPv6 host/port pairs, including scoped
addresses such as `[fe80::1%eth0]:8080`, with the same existing port rules.
Unix socket paths keep their existing handling. Native Go `net.Listen`
examples preserve distinct IPv4/IPv6/Unix call sites and exact source anchors.
The Python tuple/host-port and JS numeric-listen APIs do not use this string
address parser; this fix adds no new API recognition.
