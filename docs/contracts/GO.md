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
Local factory return values can resolve a stored implementation. These are
possible alternatives with an open frontier, never exact instance bindings:
all observed stores to one field do not prove the value of a particular receiver.
Fields with the same name on unrelated types do not share candidates. Both the
call site and the constructor assignment survive projection; an assignment is
support for the call, not a second call at the constructor line. This recovers
`quotaKVServer.Put -> kvServer.Put -> EtcdServer.Put` in etcd without any
framework-specific rule.

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
These are object-registration observations, not callback executions. On etcd,
quotaKVServer.Put retains the RegisterKVServer argument at grpc.go:80 and the
separate local KvServerToKvClient adapter binding at v3client.go:33.

## Receiver fields and bindings

Callable bindings now retain literal assignments to other fields of the same
SSA receiver in that function. Referrer identity keeps two command/worker
objects of the same type separate; conditional or later stores remain separate
anchored observations, not final runtime values. Named and anonymous callbacks
share this mechanism. No field names or framework types drive extraction.
ProgramIndex carries these as `callable_receiver_field` witnesses, and the
atlas attaches them as binding evidence rather than additional registrations.
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

## Owned declarations

Direct TypeScript interface property declarations retain their written type,
optional/readonly modifiers, exact source location and native owner. Go core
objects retain compiler type signatures and explicitly declared struct fields, including tags and embedded
field declarations, under their native type. Both project into the existing
type-owned variable objects used by Python class fields. The same atlas members
and question evidence carry them onward; no field creates a runtime call or an
inherited declaration at a new owner. Comparable count-field examples live in the cumulative TypeScript, Python and Go testdata repositories.

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
